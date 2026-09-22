package coserasset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portableid"
)

const MaxCleanupGroups = 100

var (
	ErrCleanupSelectionInvalid = errors.New("invalid Coser managed asset cleanup selection")
	ErrCleanupReviewStale      = errors.New("Coser managed asset cleanup review is stale")
)

type ReviewReason string

const (
	ReviewReasonReplaced     ReviewReason = "REPLACED"
	ReviewReasonMergedCoser  ReviewReason = "MERGED_COSER"
	ReviewReasonDeletedCoser ReviewReason = "DELETED_COSER"
)

type ReviewGroup struct {
	ID        string       `json:"id"`
	CoserUUID string       `json:"coser_uuid"`
	Kind      string       `json:"kind"`
	Reason    ReviewReason `json:"reason"`
	FileCount int          `json:"file_count"`
	ByteSize  int64        `json:"byte_size"`
	Modified  time.Time    `json:"modified_at"`

	files []reviewFile
}

type Review struct {
	Groups            []ReviewGroup `json:"groups"`
	TotalFileCount    int           `json:"total_file_count"`
	TotalByteSize     int64         `json:"total_byte_size"`
	IgnoredEntryCount int           `json:"ignored_entry_count"`
}

type CleanupResult struct {
	DeletedGroupCount int    `json:"deleted_group_count"`
	DeletedFileCount  int    `json:"deleted_file_count"`
	DeletedByteSize   int64  `json:"deleted_byte_size"`
	Review            Review `json:"review"`
}

type reviewFile struct {
	relative string
	size     int64
}

type managedGroupKey struct {
	kind      string
	assetUUID string
}

// ReviewUnreferenced lists only files whose names exactly match the
// application-generated avatar/banner layout. Unknown files and links are
// counted but never exposed as cleanup candidates.
func (s Service) ReviewUnreferenced(ctx context.Context) (Review, error) {
	if s.Database == nil || strings.TrimSpace(s.Root) == "" {
		return Review{}, errors.New("Coser managed asset review is unavailable")
	}
	owners, err := s.Database.CoreEntities().CoserManagedAssetOwners(ctx)
	if err != nil {
		return Review{}, err
	}
	return s.reviewWithOwners(owners)
}

// CleanupUnreferenced re-runs the review while holding an immediate database
// transaction, then removes only selected groups that remain unreferenced.
func (s Service) CleanupUnreferenced(ctx context.Context, selectedIDs []string) (CleanupResult, error) {
	if s.Database == nil || strings.TrimSpace(s.Root) == "" || len(selectedIDs) == 0 || len(selectedIDs) > MaxCleanupGroups {
		return CleanupResult{}, ErrCleanupSelectionInvalid
	}
	selected := make(map[string]struct{}, len(selectedIDs))
	for _, id := range selectedIDs {
		if len(id) != sha256.Size*2 {
			return CleanupResult{}, ErrCleanupSelectionInvalid
		}
		if _, err := hex.DecodeString(id); err != nil {
			return CleanupResult{}, ErrCleanupSelectionInvalid
		}
		if _, duplicate := selected[id]; duplicate {
			return CleanupResult{}, ErrCleanupSelectionInvalid
		}
		selected[id] = struct{}{}
	}

	var result CleanupResult
	err := s.Database.CoreEntities().WithCoserManagedAssetOwners(ctx, func(owners []productdb.CoserManagedAssetOwner) error {
		review, err := s.reviewWithOwners(owners)
		if err != nil {
			return err
		}
		candidates := make(map[string]ReviewGroup, len(review.Groups))
		for _, group := range review.Groups {
			candidates[group.ID] = group
		}
		for id := range selected {
			if _, ok := candidates[id]; !ok {
				return ErrCleanupReviewStale
			}
		}
		for _, group := range review.Groups {
			if _, ok := selected[group.ID]; !ok {
				continue
			}
			for _, file := range group.files {
				if err := removeManagedFile(s.Root, group.CoserUUID, file.relative); err != nil {
					return err
				}
				result.DeletedFileCount++
				result.DeletedByteSize += file.size
			}
			if err := syncDirectory(filepath.Join(s.Root, group.CoserUUID, "assets")); err != nil {
				return err
			}
			result.DeletedGroupCount++
		}
		result.Review, err = s.reviewWithOwners(owners)
		return err
	})
	return result, err
}

func (s Service) reviewWithOwners(owners []productdb.CoserManagedAssetOwner) (Review, error) {
	result := Review{Groups: []ReviewGroup{}}
	root, err := filepath.Abs(s.Root)
	if err != nil {
		return Review{}, err
	}
	rootInfo, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return Review{}, err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return Review{}, errors.New("Coser metadata root must be a real directory")
	}

	for _, owner := range owners {
		ownerDirectory := filepath.Join(root, owner.UUID)
		assetsDirectory := filepath.Join(ownerDirectory, "assets")
		if safe, exists, err := realDirectory(ownerDirectory); err != nil {
			return Review{}, err
		} else if !exists {
			continue
		} else if !safe {
			result.IgnoredEntryCount++
			continue
		}
		if safe, exists, err := realDirectory(assetsDirectory); err != nil {
			return Review{}, err
		} else if !exists {
			continue
		} else if !safe {
			result.IgnoredEntryCount++
			continue
		}
		referenced := referencedGroups(owner)
		entries, err := os.ReadDir(assetsDirectory)
		if err != nil {
			return Review{}, err
		}
		groups := make(map[managedGroupKey]*ReviewGroup)
		for _, entry := range entries {
			if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
				result.IgnoredEntryCount++
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return Review{}, err
			}
			if !info.Mode().IsRegular() {
				result.IgnoredEntryCount++
				continue
			}
			key, ok := parseManagedFilename(entry.Name())
			if !ok {
				result.IgnoredEntryCount++
				continue
			}
			if _, current := referenced[key]; current {
				continue
			}
			group := groups[key]
			if group == nil {
				group = &ReviewGroup{
					ID: opaqueGroupID(owner.UUID, key), CoserUUID: owner.UUID, Kind: strings.ToUpper(key.kind),
					Reason: reviewReason(owner.State),
				}
				groups[key] = group
			}
			group.files = append(group.files, reviewFile{relative: filepath.ToSlash(filepath.Join("assets", entry.Name())), size: info.Size()})
			group.FileCount++
			group.ByteSize += info.Size()
			if info.ModTime().After(group.Modified) {
				group.Modified = info.ModTime().UTC()
			}
		}
		for _, group := range groups {
			sort.Slice(group.files, func(i, j int) bool { return group.files[i].relative < group.files[j].relative })
			result.Groups = append(result.Groups, *group)
			result.TotalFileCount += group.FileCount
			result.TotalByteSize += group.ByteSize
		}
	}
	sort.Slice(result.Groups, func(i, j int) bool {
		if result.Groups[i].Reason != result.Groups[j].Reason {
			return result.Groups[i].Reason < result.Groups[j].Reason
		}
		if result.Groups[i].CoserUUID != result.Groups[j].CoserUUID {
			return result.Groups[i].CoserUUID < result.Groups[j].CoserUUID
		}
		if result.Groups[i].Kind != result.Groups[j].Kind {
			return result.Groups[i].Kind < result.Groups[j].Kind
		}
		return result.Groups[i].ID < result.Groups[j].ID
	})
	return result, nil
}

func realDirectory(path string) (safe, exists bool, err error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return info.Mode()&os.ModeSymlink == 0 && info.IsDir(), true, nil
}

func referencedGroups(owner productdb.CoserManagedAssetOwner) map[managedGroupKey]struct{} {
	result := make(map[managedGroupKey]struct{}, 2)
	if owner.State != productdb.CoserManagedAssetOwnerActive {
		return result
	}
	for _, relative := range []string{owner.AvatarPath, owner.BannerPath} {
		if relative == "" {
			continue
		}
		if key, ok := parseManagedFilename(filepath.Base(filepath.FromSlash(relative))); ok {
			result[key] = struct{}{}
		}
	}
	return result
}

func parseManagedFilename(name string) (managedGroupKey, bool) {
	for _, kind := range []string{"avatar", "banner"} {
		prefix := kind + "-"
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		remainder := strings.TrimPrefix(name, prefix)
		var identifier string
		switch {
		case kind == "avatar" && strings.HasSuffix(remainder, "-480.jpg"):
			identifier = strings.TrimSuffix(remainder, "-480.jpg")
		case kind == "banner" && strings.HasSuffix(remainder, "-960.jpg"):
			identifier = strings.TrimSuffix(remainder, "-960.jpg")
		case kind == "banner" && strings.HasSuffix(remainder, "-1600.jpg"):
			identifier = strings.TrimSuffix(remainder, "-1600.jpg")
		default:
			extension := filepath.Ext(remainder)
			if extension != ".jpg" && extension != ".png" && extension != ".webp" {
				return managedGroupKey{}, false
			}
			identifier = strings.TrimSuffix(remainder, extension)
		}
		if _, err := portableid.Parse(identifier); err != nil {
			return managedGroupKey{}, false
		}
		return managedGroupKey{kind: kind, assetUUID: identifier}, true
	}
	return managedGroupKey{}, false
}

func reviewReason(state productdb.CoserManagedAssetOwnerState) ReviewReason {
	switch state {
	case productdb.CoserManagedAssetOwnerMerged:
		return ReviewReasonMergedCoser
	case productdb.CoserManagedAssetOwnerDeleted:
		return ReviewReasonDeletedCoser
	default:
		return ReviewReasonReplaced
	}
}

func opaqueGroupID(coserUUID string, key managedGroupKey) string {
	value := sha256.Sum256([]byte(coserUUID + "\x00" + key.kind + "\x00" + key.assetUUID))
	return hex.EncodeToString(value[:])
}

func removeManagedFile(root, coserUUID, relative string) error {
	file, expected, err := OpenManaged(root, coserUUID, relative)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	target := filepath.Join(root, coserUUID, filepath.FromSlash(relative))
	quarantine := filepath.Join(filepath.Dir(target), ".coser-clean-"+portableid.New()+".tmp")
	if err := os.Rename(target, quarantine); err != nil {
		return err
	}
	restore := func() {
		if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
			_ = os.Rename(quarantine, target)
		}
	}
	moved, err := os.Lstat(quarantine)
	if err != nil {
		restore()
		return err
	}
	if !os.SameFile(expected, moved) || moved.Mode()&os.ModeSymlink != 0 || !moved.Mode().IsRegular() {
		restore()
		return errors.New("Coser managed asset changed before cleanup")
	}
	if err := os.Remove(quarantine); err != nil {
		restore()
		return fmt.Errorf("removing Coser managed asset: %w", err)
	}
	return nil
}
