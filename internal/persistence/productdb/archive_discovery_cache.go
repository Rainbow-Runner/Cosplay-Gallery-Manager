package productdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/stashapp/stash/internal/archivecheck"
	"github.com/stashapp/stash/internal/gallery"
)

// Change this when discovery safety checks, supported media or counting change.
const archiveDiscoveryVersion = 1

type archiveDiscoveryEntry struct {
	size                           int64
	modified, limits, data, reason string
	version                        int
}

func loadArchiveDiscoveryCache(ctx context.Context, db *sql.DB, libraryID int64) (map[string]archiveDiscoveryEntry, error) {
	rows, err := db.QueryContext(ctx, `SELECT path,byte_size,modified_at,limits_json,version,observation_json,reason FROM archive_discovery_cache WHERE library_id=?`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]archiveDiscoveryEntry{}
	for rows.Next() {
		var path string
		var entry archiveDiscoveryEntry
		if err := rows.Scan(&path, &entry.size, &entry.modified, &entry.limits, &entry.version, &entry.data, &entry.reason); err != nil {
			return nil, err
		}
		result[path] = entry
	}
	return result, rows.Err()
}

func (s *CandidateDiscoveryStore) observeCachedArchive(ctx context.Context, libraryID int64, root, path string, info os.FileInfo, limits archivecheck.Limits, prior archiveDiscoveryEntry, force bool) (ObservedDirectory, string, bool, error) {
	encoded, err := json.Marshal(limits)
	if err != nil {
		return ObservedDirectory{}, "", false, err
	}
	modified := info.ModTime().UTC().Format(time.RFC3339Nano)
	var observation ObservedDirectory
	if !force && !info.ModTime().IsZero() && prior.version == archiveDiscoveryVersion && prior.size == info.Size() && prior.modified == modified && prior.limits == string(encoded) && json.Unmarshal([]byte(prior.data), &observation) == nil && observation.SourceType == gallery.SourceTypeArchive && observation.MediaCount >= 0 {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return ObservedDirectory{}, "", false, err
		}
		observation.RelativePath = filepath.ToSlash(relative)
		return withArchiveManifest(observation, path), prior.reason, true, nil
	}
	observation, reason := observeArchiveCandidateBase(root, path, limits)
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(info, current) || current.Size() != info.Size() || !current.ModTime().Equal(info.ModTime()) {
		return ObservedDirectory{}, "", false, errors.New("archive changed during discovery")
	}
	// Never cache read errors. A repaired mount must be inspected again.
	if reason != "UNREADABLE_ARCHIVE" {
		data, err := json.Marshal(observation)
		if err != nil {
			return observation, reason, false, err
		}
		_, err = s.db.ExecContext(ctx, `INSERT INTO archive_discovery_cache(library_id,path,byte_size,modified_at,limits_json,version,observation_json,reason) VALUES(?,?,?,?,?,?,?,?)
		 ON CONFLICT(library_id,path) DO UPDATE SET byte_size=excluded.byte_size,modified_at=excluded.modified_at,limits_json=excluded.limits_json,version=excluded.version,observation_json=excluded.observation_json,reason=excluded.reason`, libraryID, path, info.Size(), modified, string(encoded), archiveDiscoveryVersion, string(data), reason)
		if err != nil {
			return observation, reason, false, err
		}
	}
	return withArchiveManifest(observation, path), reason, false, nil
}

// Sidecar identity is always read live, never trusted from archive cache.
func withArchiveManifest(observation ObservedDirectory, filename string) ObservedDirectory {
	if observation.SourceType != gallery.SourceTypeArchive {
		return observation
	}
	if evidence := readManifestIdentity(filename + ".cosplay.json"); evidence.Exists {
		observation.HasValidManifest, observation.ManifestSetID = evidence.Valid, evidence.SetID
		observation.ManifestSchema, observation.ManifestRevision, observation.ManifestHash, observation.ManifestDocument = evidence.Schema, evidence.Revision, evidence.Hash, evidence.Document
		observation.HasConflict = observation.HasConflict || !evidence.Valid
	}
	return observation
}
