package productdb

import (
	"context"
	"errors"
	"os"
	"reflect"
	"time"

	"github.com/stashapp/stash/internal/manifest"
)

type GalleryManifestBatchPreview struct {
	GalleryID              int64
	SetID                  string
	Title                  string
	Status                 ManifestStatus
	MetadataRevision       int64
	Path                   string
	FileHash               string
	DatabaseContentChanged bool
	LocalFileChanged       bool
	BlockReason            string
}

// PreviewGalleryBatchPush performs a fresh per-Gallery inspection. It is
// deliberately bounded and never modifies a Manifest file.
func (s *ManifestStore) PreviewGalleryBatchPush(ctx context.Context, galleryID int64, now time.Time) (GalleryManifestBatchPreview, error) {
	galleryValue, err := findGallery(ctx, s.db, galleryID)
	if err != nil {
		return GalleryManifestBatchPreview{}, err
	}
	result := GalleryManifestBatchPreview{GalleryID: galleryID, SetID: galleryValue.SetID, Title: galleryValue.Title, MetadataRevision: galleryValue.MetadataRevision}
	source, err := findGallerySourceByGallery(ctx, s.db, galleryID)
	if err != nil {
		result.Status, result.BlockReason = ManifestNone, "NO_SOURCE"
		return result, nil
	}
	available, readOnly := 1, 0
	if source.LibraryID != nil {
		if err := s.db.QueryRowContext(ctx, `SELECT source.availability_state='AVAILABLE',library.read_only FROM gallery_sources source JOIN media_libraries library ON library.id=source.library_id WHERE source.id=?`, source.ID).Scan(&available, &readOnly); err != nil {
			return result, err
		}
	}
	if available == 0 {
		result.Status, result.BlockReason = ManifestSourceUnavailable, "SOURCE_UNAVAILABLE"
		return result, nil
	}
	if readOnly != 0 {
		result.BlockReason = "READ_ONLY_LIBRARY"
	}
	state, checkErr := s.CheckGallery(ctx, galleryID, now)
	result.Status, result.Path, result.FileHash = state.Status, state.Path, state.FileHash
	if checkErr != nil {
		result.Status, result.BlockReason = ManifestError, "MANIFEST_CHECK_FAILED"
		return result, nil
	}
	baselineState, baseline, _, found, err := loadGalleryManifestState(ctx, s.db, galleryID)
	if err != nil {
		return result, err
	}
	if !found {
		result.DatabaseContentChanged = true
	} else {
		current, err := buildGalleryBusinessSnapshot(ctx, s.db, galleryID)
		if err != nil {
			return result, err
		}
		result.DatabaseContentChanged = !reflect.DeepEqual(current, baseline)
		result.LocalFileChanged = state.FileHash != "" && state.FileHash != baselineState.FileHash
	}
	if result.Status == ManifestFileDirty && !found {
		result.LocalFileChanged = true
	}
	if result.LocalFileChanged {
		data, _, readErr := manifest.ReadFile(result.Path, manifest.MaxGalleryBytes)
		if readErr != nil {
			result.Status, result.BlockReason = ManifestError, "MANIFEST_READ_FAILED"
		} else if document, parseErr := manifest.ParseGallery(bytesReader(data)); parseErr != nil || document.SetID != result.SetID {
			result.Status, result.BlockReason = ManifestError, "INVALID_OR_OTHER_MANIFEST"
		}
	}
	return result, nil
}

type GalleryManifestBatchDecision struct {
	GalleryID        int64
	ExpectedRevision int64
	ExpectedPath     string
	ExpectedFileHash string
}

type GalleryManifestBatchResult struct {
	GalleryID int64
	SetID     string
	Outcome   string
	Reason    string
}

// PushGalleryBatchItem rechecks everything shown in the preview. A changed
// source, database revision or file hash fails closed, even in overwrite mode.
func (s *ManifestStore) PushGalleryBatchItem(ctx context.Context, input GalleryManifestBatchDecision, overwriteLocal bool, now time.Time) (GalleryManifestBatchResult, error) {
	preview, err := s.PreviewGalleryBatchPush(ctx, input.GalleryID, now)
	if err != nil {
		return GalleryManifestBatchResult{}, err
	}
	result := GalleryManifestBatchResult{GalleryID: input.GalleryID, SetID: preview.SetID}
	if preview.MetadataRevision != input.ExpectedRevision || preview.Path != input.ExpectedPath || preview.FileHash != input.ExpectedFileHash {
		result.Outcome, result.Reason = "FAILED", "PREVIEW_STALE"
		return result, nil
	}
	if preview.BlockReason != "" || preview.Status == ManifestError || preview.Status == ManifestSourceUnavailable {
		result.Outcome, result.Reason = "SKIPPED", preview.BlockReason
		return result, nil
	}
	if !preview.DatabaseContentChanged && !preview.LocalFileChanged && preview.Status != ManifestMissing {
		if preview.Status == ManifestDBDirty {
			if err := s.acknowledgeNoContentChange(ctx, input.GalleryID, input.ExpectedRevision, input.ExpectedFileHash, now); err != nil {
				result.Outcome, result.Reason = "FAILED", "PREVIEW_STALE"
				return result, nil
			}
			_, _ = s.CheckGallery(ctx, input.GalleryID, now)
		}
		result.Outcome, result.Reason = "SKIPPED", "NO_CONTENT_CHANGE"
		return result, nil
	}
	if preview.LocalFileChanged && !overwriteLocal {
		result.Outcome, result.Reason = "SKIPPED", "LOCAL_FILE_CHANGED"
		return result, nil
	}
	if preview.Status == ManifestMissing {
		if _, err := os.Lstat(preview.Path); !errors.Is(err, os.ErrNotExist) {
			result.Outcome, result.Reason = "FAILED", "PREVIEW_STALE"
			return result, nil
		}
	}
	if preview.LocalFileChanged {
		_, err = s.PushGalleryOverwritingFile(ctx, input.GalleryID, input.ExpectedRevision, input.ExpectedFileHash, now)
	} else {
		_, err = s.PushGallery(ctx, input.GalleryID, input.ExpectedRevision, now)
	}
	if err != nil {
		result.Outcome, result.Reason = "FAILED", "PUSH_FAILED"
		return result, nil
	}
	result.Outcome = "PUSHED"
	_, _ = s.CheckGallery(ctx, input.GalleryID, now)
	return result, nil
}

func (s *ManifestStore) acknowledgeNoContentChange(ctx context.Context, galleryID, expectedRevision int64, expectedHash string, now time.Time) error {
	state, baseline, _, found, err := loadGalleryManifestState(ctx, s.db, galleryID)
	if err != nil || !found || state.FileHash != expectedHash {
		return ErrManifestFileDirty
	}
	if _, currentHash, err := manifest.ReadFile(state.Path, manifest.MaxGalleryBytes); err != nil || currentHash != expectedHash {
		return ErrManifestFileDirty
	}
	current, err := buildGalleryBusinessSnapshot(ctx, s.db, galleryID)
	if err != nil || !reflect.DeepEqual(current, baseline) {
		return ErrMetadataRevisionConflict
	}
	result, err := s.db.ExecContext(ctx, `UPDATE gallery_manifest_sync SET status='CLEAN',baseline_metadata_revision=?,checked_at_utc=?
		WHERE gallery_id=? AND file_hash=? AND EXISTS(SELECT 1 FROM galleries WHERE id=? AND metadata_revision=?)`,
		expectedRevision, formatTime(normalisedTime(now)), galleryID, expectedHash, galleryID, expectedRevision)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return ErrMetadataRevisionConflict
	}
	return nil
}
