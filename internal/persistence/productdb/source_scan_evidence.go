package productdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/stashapp/stash/internal/archivecheck"
	"github.com/stashapp/stash/internal/sourcescan"
)

const sourceScanEvidenceVersion = 1

type archiveScanEvidence struct {
	Size          int64
	ModifiedAtUTC string
	LimitsJSON    string
}

func inspectArchiveEvidence(path string, limits archivecheck.Limits) (archiveScanEvidence, os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return archiveScanEvidence{}, nil, err
	}
	if !info.Mode().IsRegular() {
		return archiveScanEvidence{}, nil, errors.New("archive GallerySource must be a regular file, not a symbolic link")
	}
	encoded, err := json.Marshal(limits)
	if err != nil {
		return archiveScanEvidence{}, nil, err
	}
	modified := ""
	if !info.ModTime().IsZero() {
		modified = info.ModTime().UTC().Format(time.RFC3339Nano)
	}
	return archiveScanEvidence{Size: info.Size(), ModifiedAtUTC: modified, LimitsJSON: string(encoded)}, info, nil
}

func (s *ScanStore) reusableArchiveEvidence(ctx context.Context, sourceID int64, current archiveScanEvidence) (bool, map[string]sourcescan.Observation, error) {
	if current.ModifiedAtUTC == "" {
		return false, nil, nil
	}
	var size int64
	var modified, limits string
	var version int
	err := s.db.QueryRowContext(ctx, `SELECT container_size,container_modified_at_utc,
		archive_limits_json,scanner_version FROM gallery_source_scan_evidence WHERE source_id=?`, sourceID).
		Scan(&size, &modified, &limits, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	if size != current.Size || modified != current.ModifiedAtUTC || limits != current.LimitsJSON || version != sourceScanEvidenceVersion {
		return false, nil, nil
	}
	prior, err := s.reusableDirectoryEvidence(ctx, sourceID)
	if err != nil || prior == nil {
		return false, nil, err
	}
	var availableCount int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gallery_items
		WHERE source_id=? AND availability_state='AVAILABLE'`, sourceID).Scan(&availableCount); err != nil {
		return false, nil, err
	}
	if availableCount != len(prior) {
		return false, nil, nil
	}
	for _, observation := range prior {
		if !sourcescan.ValidContentFingerprints(observation) ||
			observation.MediaKind == "" || observation.ContentFormat == "" {
			return false, nil, nil
		}
	}
	return true, prior, nil
}
