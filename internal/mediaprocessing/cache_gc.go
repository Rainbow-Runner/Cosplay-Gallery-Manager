package mediaprocessing

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/portableid"
	"golang.org/x/sys/unix"
)

var generatedCachePattern = regexp.MustCompile(`^items/([0-9a-f]{2})/([0-9a-f-]{36})/r([1-9][0-9]*)/([0-9a-f]{16})/(card-480|card-960|card-1600|lightbox-4096|static-poster|animated-preview|video-playback)\.(jpg|jpeg|png|webp|mp4)$`)

type CacheFileIdentity struct {
	ItemUUID, Variant, ProfileKey string
	ContentRevision               int64
}

func ParseGeneratedCachePath(relative string) (CacheFileIdentity, bool) {
	if strings.HasPrefix(filepathBase(relative), ".gc-") {
		relative = strings.TrimSuffix(relative, filepathBase(relative)) + strings.TrimPrefix(filepathBase(relative), ".gc-")
	}
	m := generatedCachePattern.FindStringSubmatch(relative)
	if len(m) == 0 || m[1] != m[2][:2] {
		return CacheFileIdentity{}, false
	}
	if _, err := portableid.Parse(m[2]); err != nil {
		return CacheFileIdentity{}, false
	}
	revision, err := strconv.ParseInt(m[3], 10, 64)
	if err != nil {
		return CacheFileIdentity{}, false
	}
	variant := strings.ToUpper(strings.ReplaceAll(m[5], "-", "_"))
	if _, known := RequiredCacheTier(variant); !known {
		return CacheFileIdentity{}, false
	}
	return CacheFileIdentity{ItemUUID: m[2], Variant: variant, ProfileKey: m[4], ContentRevision: revision}, true
}

func filepathBase(relative string) string {
	parts := strings.Split(relative, "/")
	return parts[len(parts)-1]
}

// Open each directory component with O_NOFOLLOW and keep its descriptor. A
// replaced parent cannot redirect the final unlink into a media library.
func (writer CacheWriter) cacheParent(relative string) (int, string, error) {
	if _, ok := ParseGeneratedCachePath(relative); !ok {
		return -1, "", errors.New("unknown generated cache path")
	}
	fd, err := unix.Open(writer.Root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, "", err
	}
	parts := strings.Split(relative, "/")
	for _, component := range parts[:len(parts)-1] {
		next, e := unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(fd)
		if e != nil {
			return -1, "", e
		}
		fd = next
	}
	return fd, parts[len(parts)-1], nil
}

func (writer CacheWriter) InspectForCleanup(relative string) (fs.FileInfo, error) {
	fd, name, err := writer.cacheParent(relative)
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)
	fileFD, err := unix.Openat(fd, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if errors.Is(err, os.ErrNotExist) && !strings.HasPrefix(name, ".gc-") {
		fileFD, err = unix.Openat(fd, ".gc-"+name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	}
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fileFD), name)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	var raw unix.Stat_t
	if err := unix.Fstat(fileFD, &raw); err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || raw.Nlink != 1 {
		return nil, errors.New("cache cleanup requires a regular non-hardlinked file")
	}
	return info, nil
}

// RemoveReviewedGenerated is for DB-authorized retired resources, not ordinary
// BASE eviction. File identity is rechecked before unlink; no recursive delete.
func (writer CacheWriter) RemoveReviewedGenerated(relative string, expected fs.FileInfo) error {
	fd, name, err := writer.cacheParent(relative)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	fileFD, err := unix.Openat(fd, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	quarantine := ".gc-" + name
	resuming := strings.HasPrefix(name, ".gc-")
	if resuming {
		quarantine = name
	}
	if errors.Is(err, os.ErrNotExist) {
		fileFD, err = unix.Openat(fd, quarantine, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		resuming = true
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fileFD), name)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	var raw unix.Stat_t
	if err := unix.Fstat(fileFD, &raw); err != nil {
		return err
	}
	if expected == nil || !info.Mode().IsRegular() || raw.Nlink != 1 || !os.SameFile(expected, info) || expected.Size() != info.Size() || !expected.ModTime().Equal(info.ModTime()) {
		return errors.New("cache file changed after review")
	}
	// Quarantine first so a rename race can never unlink the replacement file.
	if !resuming {
		if err := unix.Renameat2(fd, name, fd, quarantine, unix.RENAME_NOREPLACE); err != nil {
			return err
		}
	}
	var moved unix.Stat_t
	if err := unix.Fstatat(fd, quarantine, &moved, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if moved.Dev != raw.Dev || moved.Ino != raw.Ino || moved.Size != raw.Size || moved.Mtim != raw.Mtim || moved.Nlink != 1 {
		// Do not overwrite a concurrently created new file when restoring.
		_ = unix.Renameat2(fd, quarantine, fd, name, unix.RENAME_NOREPLACE)
		return errors.New("cache file changed during quarantine")
	}
	if err := unix.Unlinkat(fd, quarantine, 0); err != nil {
		return err
	}
	writer.pruneEmptyCacheParents(relative)
	return nil
}

// Remove only empty known cache directories, never recursively or via links.
func (writer CacheWriter) pruneEmptyCacheParents(relative string) {
	parts := strings.Split(relative, "/")
	for depth := len(parts) - 2; depth >= 2; depth-- {
		fd, err := unix.Open(writer.Root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return
		}
		for _, component := range parts[:depth] {
			next, e := unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			unix.Close(fd)
			if e != nil {
				return
			}
			fd = next
		}
		err = unix.Unlinkat(fd, parts[depth], unix.AT_REMOVEDIR)
		unix.Close(fd)
		if err != nil {
			return
		}
	}
}

var ErrCacheWalkBound = errors.New("cache walk budget reached")

// WalkCleanupShard is lexically resumable and skips completed subtrees. It
// traverses only one generated UUID prefix, never follows directory symlinks.
func (writer CacheWriter) WalkCleanupShard(shard int, cursor string, budget int, deadline time.Time, visit func(string, fs.FileInfo) error) (last string, complete bool, ignored int, err error) {
	root, err := os.OpenRoot(writer.Root)
	if err != nil {
		return "", false, 0, err
	}
	defer root.Close()
	start := fmt.Sprintf("items/%02x", shard)
	// Root.FS is confined even if a parent is maliciously swapped mid-walk.
	for _, p := range []string{"items", start} {
		info, e := root.Lstat(p)
		if errors.Is(e, os.ErrNotExist) {
			return "", true, 0, nil
		}
		if e != nil {
			return "", false, 0, e
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", true, 1, nil
		}
	}
	count := 0
	last = cursor
	err = fs.WalkDir(root.FS(), start, func(relative string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil // Another cleanup may have pruned an empty directory.
			}
			return walkErr
		}
		if time.Now().After(deadline) {
			return ErrCacheWalkBound
		}
		if entry.IsDir() {
			if relative < cursor && !strings.HasPrefix(cursor, relative+"/") {
				return fs.SkipDir
			}
			if count >= budget {
				return ErrCacheWalkBound
			}
			count++
			if relative > cursor {
				last = relative
			}
			if relative != start {
				info, e := root.Lstat(relative)
				if e != nil {
					if errors.Is(e, os.ErrNotExist) {
						return fs.SkipDir
					}
					return e
				}
				if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
					ignored++
					return fs.SkipDir
				}
			}
			return nil
		}
		if relative <= cursor {
			return nil
		}
		if count >= budget {
			return ErrCacheWalkBound
		}
		count++
		if entry.Type()&os.ModeSymlink != 0 {
			last = relative
			ignored++
			return nil
		}
		if _, ok := ParseGeneratedCachePath(relative); !ok {
			last = relative
			ignored++
			return nil
		}
		info, e := writer.InspectForCleanup(relative)
		if e != nil {
			last = relative
			ignored++
			return nil
		}
		if err := visit(relative, info); err != nil {
			return err
		}
		last = relative
		return nil
	})
	if errors.Is(err, ErrCacheWalkBound) {
		return last, false, ignored, nil
	}
	return last, err == nil, ignored, err
}
