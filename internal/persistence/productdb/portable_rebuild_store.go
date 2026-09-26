package productdb

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/portableid"
	"github.com/stashapp/stash/internal/slug"
)

type PortableLibraryMapping struct {
	ImportID        string
	LibraryKey      string
	LibraryName     string
	Decision        string
	TargetLibraryID *int64
	TargetName      string
	TargetRoot      string
}

type PortableLibraryDecision struct {
	LibraryKey      string
	TargetLibraryID *int64
}

func (db *Database) ConfigurePortableRebuildAutomation(ctx context.Context, importID string, autoAdopt, autoActivate bool, now time.Time) error {
	if autoActivate && !autoAdopt {
		return errors.New("portable automatic activation requires automatic adoption")
	}
	result, err := db.ExecContext(ctx, `UPDATE portable_import_sessions SET auto_adopt_enabled=?,auto_activate_enabled=?,
		automation_authorized_at_utc=?,updated_at_utc=? WHERE import_id=? AND package_profile<>'CORE_CATALOG'
		AND state='LIBRARIES_MAPPED'`, boolInt(autoAdopt), boolInt(autoActivate), formatTime(normalisedTime(now)),
		formatTime(normalisedTime(now)), importID)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return errors.New("portable import is not ready for rebuild automation")
	}
	return nil
}

type PortableGalleryRebuild struct {
	ImportID                string
	SetID                   string
	LibraryKey              string
	SourceType              gallery.SourceType
	RelativeSource          string
	LocatorStatus           string
	ManifestStatus          string
	ManifestSchema          int
	ManifestRevision        int64
	ManifestHash            string
	State                   string
	IssueCode               string
	GalleryID               *int64
	SourceID                *int64
	ExportedRelativeSource  string
	ResolvedTargetLibraryID *int64
	ResolvedRelativeSource  string
	SourceResolution        string
	ResolutionSnapshotID    *int64
	ResolutionManifestHash  string
	ResolutionTokenHash     string
	AdoptedManifestSchema   int
	AdoptedManifestRevision int64
	AdoptedManifestHash     string
	AdoptedFromStatus       string
	AdoptedAt               string
	AdoptionTokenHash       string
}

type PortableSourceCandidate struct {
	SnapshotID       int64
	LibraryID        int64
	SetID            string
	SourceType       gallery.SourceType
	RelativeSource   string
	ManifestSchema   int
	ManifestRevision int64
	ManifestHash     string
}

func (db *Database) ListPortableLibraryMappings(ctx context.Context, importID string) ([]PortableLibraryMapping, error) {
	rows, err := db.QueryContext(ctx, `SELECT mapping.import_id,mapping.library_key,mapping.library_name,mapping.decision,
		mapping.target_library_id,COALESCE(library.name,''),COALESCE(library.root_path,'')
		FROM portable_library_mappings mapping LEFT JOIN media_libraries library ON library.id=mapping.target_library_id
		WHERE mapping.import_id=? ORDER BY mapping.library_key`, importID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []PortableLibraryMapping
	for rows.Next() {
		var value PortableLibraryMapping
		var target sql.NullInt64
		if err := rows.Scan(&value.ImportID, &value.LibraryKey, &value.LibraryName, &value.Decision, &target, &value.TargetName, &value.TargetRoot); err != nil {
			return nil, err
		}
		if target.Valid {
			id := target.Int64
			value.TargetLibraryID = &id
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

// SetPortableLibraryMappings records only local bindings. It never starts
// discovery, reads media, or copies an old machine's absolute path.
func (db *Database) SetPortableLibraryMappings(ctx context.Context, importID string, decisions []PortableLibraryDecision, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var sessionState string
	if err := tx.QueryRowContext(ctx, `SELECT state FROM portable_import_sessions WHERE import_id=?`, importID).Scan(&sessionState); err != nil {
		return err
	}
	if sessionState != "CORE_IMPORTED" && sessionState != "LIBRARIES_MAPPED" {
		return errors.New("portable import is not ready for media library mapping")
	}
	var expected int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM portable_library_mappings WHERE import_id=?`, importID).Scan(&expected); err != nil {
		return err
	}
	if len(decisions) != expected {
		return errors.New("every portable media library requires one mapping decision")
	}
	seen := map[string]bool{}
	timestamp := formatTime(normalisedTime(now))
	for _, decision := range decisions {
		if decision.LibraryKey == "" || seen[decision.LibraryKey] {
			return errors.New("portable media library mapping is duplicate or invalid")
		}
		seen[decision.LibraryKey] = true
		kind := "SKIPPED"
		if decision.TargetLibraryID != nil {
			var enabled int
			if err := tx.QueryRowContext(ctx, `SELECT enabled FROM media_libraries WHERE id=?`, *decision.TargetLibraryID).Scan(&enabled); err != nil {
				return err
			}
			if enabled != 1 {
				return errors.New("portable media library target must be enabled")
			}
			kind = "MAPPED"
		}
		result, err := tx.ExecContext(ctx, `UPDATE portable_library_mappings SET decision=?,target_library_id=?,updated_at_utc=?
			WHERE import_id=? AND library_key=?`, kind, decision.TargetLibraryID, timestamp, importID, decision.LibraryKey)
		if err != nil {
			return err
		}
		if changed, err := result.RowsAffected(); err != nil || changed != 1 {
			return errors.New("portable media library mapping key is unknown")
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE portable_import_sessions SET state='LIBRARIES_MAPPED',updated_at_utc=?
		WHERE import_id=? AND state IN ('CORE_IMPORTED','LIBRARIES_MAPPED')`, timestamp, importID); err != nil {
		return err
	}
	return tx.Commit()
}

func (db *Database) ListPortableGalleryRebuilds(ctx context.Context, importID string) ([]PortableGalleryRebuild, error) {
	rows, err := db.QueryContext(ctx, `SELECT import_id,set_id,COALESCE(library_key,''),source_type,relative_source,locator_status,
		manifest_status,manifest_schema,manifest_revision,manifest_hash,state,issue_code,gallery_id,source_id,
		exported_relative_source,resolved_target_library_id,resolved_relative_source,source_resolution,resolution_snapshot_id,
		resolution_manifest_hash,resolution_token_hash,adopted_manifest_schema,adopted_manifest_revision,adopted_manifest_hash,
		adopted_from_status,adopted_at_utc,adoption_token_hash
		FROM portable_gallery_rebuilds WHERE import_id=? ORDER BY set_id`, importID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []PortableGalleryRebuild
	for rows.Next() {
		var value PortableGalleryRebuild
		var galleryID, sourceID, resolvedLibraryID, resolutionSnapshotID sql.NullInt64
		if err := rows.Scan(&value.ImportID, &value.SetID, &value.LibraryKey, &value.SourceType, &value.RelativeSource,
			&value.LocatorStatus, &value.ManifestStatus, &value.ManifestSchema, &value.ManifestRevision, &value.ManifestHash,
			&value.State, &value.IssueCode, &galleryID, &sourceID, &value.ExportedRelativeSource, &resolvedLibraryID,
			&value.ResolvedRelativeSource, &value.SourceResolution, &resolutionSnapshotID, &value.ResolutionManifestHash,
			&value.ResolutionTokenHash, &value.AdoptedManifestSchema, &value.AdoptedManifestRevision, &value.AdoptedManifestHash,
			&value.AdoptedFromStatus, &value.AdoptedAt, &value.AdoptionTokenHash); err != nil {
			return nil, err
		}
		if galleryID.Valid {
			id := galleryID.Int64
			value.GalleryID = &id
		}
		if sourceID.Valid {
			id := sourceID.Int64
			value.SourceID = &id
		}
		if resolvedLibraryID.Valid {
			id := resolvedLibraryID.Int64
			value.ResolvedTargetLibraryID = &id
		}
		if resolutionSnapshotID.Valid {
			id := resolutionSnapshotID.Int64
			value.ResolutionSnapshotID = &id
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

// PortableSourceCandidates returns only valid Manifest observations from the
// latest completed discovery snapshot of enabled libraries mapped by this
// import. It never traverses the filesystem or mutates a candidate.
func (db *Database) PortableSourceCandidates(ctx context.Context, importID, setID string) ([]PortableSourceCandidate, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT candidate.snapshot_id,candidate.library_id,candidate.manifest_set_id,candidate.source_type,
			candidate.root_path,library.root_path,candidate.manifest_schema,candidate.manifest_revision,candidate.manifest_hash
		FROM portable_library_mappings mapping
		JOIN media_libraries library ON library.id=mapping.target_library_id AND library.enabled=1
		JOIN gallery_candidates candidate ON candidate.library_id=library.id
		JOIN (SELECT library_id,MAX(id) snapshot_id FROM discovery_snapshots GROUP BY library_id) latest
			ON latest.library_id=candidate.library_id AND latest.snapshot_id=candidate.snapshot_id
		WHERE mapping.import_id=? AND mapping.decision='MAPPED' AND candidate.manifest_set_id=?
			AND candidate.manifest_hash<>''
		ORDER BY candidate.library_id,candidate.root_path`, importID, setID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []PortableSourceCandidate
	for rows.Next() {
		var value PortableSourceCandidate
		var rootPath, libraryRoot string
		if err := rows.Scan(&value.SnapshotID, &value.LibraryID, &value.SetID, &value.SourceType, &rootPath, &libraryRoot,
			&value.ManifestSchema, &value.ManifestRevision, &value.ManifestHash); err != nil {
			return nil, err
		}
		relative, err := filepath.Rel(filepath.Clean(libraryRoot), filepath.Clean(rootPath))
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		if relative == "." {
			relative = ""
		}
		value.RelativeSource = filepath.ToSlash(relative)
		result = append(result, value)
	}
	return result, rows.Err()
}

func (db *Database) PortableMappingsHaveCompleteDiscovery(ctx context.Context, importID string) (bool, error) {
	var mapped, discovered int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(CASE WHEN EXISTS(
		SELECT 1 FROM discovery_snapshots snapshot WHERE snapshot.library_id=mapping.target_library_id
	) THEN 1 ELSE 0 END),0) FROM portable_library_mappings mapping
	WHERE mapping.import_id=? AND mapping.decision='MAPPED'`, importID).Scan(&mapped, &discovered)
	if err != nil {
		return false, err
	}
	return mapped > 0 && mapped == discovered, nil
}

func (db *Database) SetPortableSourceResolution(ctx context.Context, importID, setID, resolution string, targetLibraryID, snapshotID *int64, relativeSource, manifestHash, tokenHash string, now time.Time) error {
	result, err := db.ExecContext(ctx, `UPDATE portable_gallery_rebuilds SET
		adopted_manifest_schema=CASE WHEN resolution_token_hash=? THEN adopted_manifest_schema ELSE 0 END,
		adopted_manifest_revision=CASE WHEN resolution_token_hash=? THEN adopted_manifest_revision ELSE 0 END,
		adopted_manifest_hash=CASE WHEN resolution_token_hash=? THEN adopted_manifest_hash ELSE '' END,
		adopted_from_status=CASE WHEN resolution_token_hash=? THEN adopted_from_status ELSE '' END,
		adopted_at_utc=CASE WHEN resolution_token_hash=? THEN adopted_at_utc ELSE '' END,
		adoption_token_hash=CASE WHEN resolution_token_hash=? THEN adoption_token_hash ELSE '' END,
		resolved_target_library_id=?,resolved_relative_source=?,source_resolution=?,resolution_snapshot_id=?,
		resolution_manifest_hash=?,resolution_token_hash=?,updated_at_utc=?
		WHERE import_id=? AND set_id=? AND state NOT IN ('REBUILT','SKIPPED')`, tokenHash, tokenHash, tokenHash, tokenHash, tokenHash, tokenHash,
		targetLibraryID, relativeSource, resolution, snapshotID, manifestHash, tokenHash, formatTime(normalisedTime(now)), importID, setID)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return errors.New("portable source resolution target is no longer available")
	}
	return nil
}

func (db *Database) AdoptPortableGalleryManifest(ctx context.Context, importID, setID, expectedResolutionToken string, schema int, revision int64, manifestHash, adoptionToken string, now time.Time) error {
	result, err := db.ExecContext(ctx, `UPDATE portable_gallery_rebuilds SET adopted_manifest_schema=?,adopted_manifest_revision=?,
		adopted_manifest_hash=?,adopted_from_status=manifest_status,adopted_at_utc=?,adoption_token_hash=?,state='PENDING',issue_code='',updated_at_utc=?
		WHERE import_id=? AND set_id=? AND resolution_token_hash=? AND source_resolution IN ('EXACT','RELOCATED_UNIQUE')
			AND state NOT IN ('REBUILT','SKIPPED')`, schema, revision, manifestHash, formatTime(normalisedTime(now)), adoptionToken,
		formatTime(normalisedTime(now)), importID, setID, expectedResolutionToken)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return errors.New("portable Manifest adoption evidence is stale")
	}
	return nil
}

func (db *Database) SetPortableGalleryInspection(ctx context.Context, importID, setID, state, issueCode string, now time.Time) error {
	if state != "READY" && state != "BLOCKED" && state != "SKIPPED" {
		return errors.New("invalid portable Gallery inspection state")
	}
	if state == "READY" || state == "SKIPPED" {
		issueCode = ""
	}
	result, err := db.ExecContext(ctx, `UPDATE portable_gallery_rebuilds SET state=?,issue_code=?,updated_at_utc=?
		WHERE import_id=? AND set_id=? AND state IN ('PENDING','READY','BLOCKED','SKIPPED')`, state, issueCode, formatTime(normalisedTime(now)), importID, setID)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return errors.New("portable Gallery rebuild is no longer inspectable")
	}
	return nil
}

func (db *Database) SetPortableGalleryRebuildIssue(ctx context.Context, importID, setID, issueCode string, now time.Time) error {
	if issueCode == "" || len(issueCode) > 100 {
		return errors.New("portable Gallery rebuild issue code is invalid")
	}
	result, err := db.ExecContext(ctx, `UPDATE portable_gallery_rebuilds SET issue_code=?,updated_at_utc=?
		WHERE import_id=? AND set_id=? AND state='REBUILDING'`, issueCode, formatTime(normalisedTime(now)), importID, setID)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return errors.New("portable Gallery rebuild is not in progress")
	}
	return nil
}

func (db *Database) FinalizePortableGalleryRebuilds(ctx context.Context, importID string, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var remaining, pendingActive int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM portable_gallery_rebuilds WHERE import_id=? AND state NOT IN ('REBUILT','SKIPPED')`, importID).Scan(&remaining); err != nil {
		return err
	}
	if remaining != 0 {
		return nil
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM portable_identity_claims WHERE import_id=? AND identity_state='ACTIVE' AND claim_state<>'CLAIMED'`, importID).Scan(&pendingActive); err != nil {
		return err
	}
	// A skipped or incomplete Gallery keeps its ACTIVE identities reserved and
	// the session resumable. It must not be advertised as fully rebuilt.
	if pendingActive != 0 {
		return nil
	}
	if err := publishPortableLifecycleClaims(ctx, tx, importID, now); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE portable_import_sessions SET state='GALLERIES_REBUILT',updated_at_utc=?
		WHERE import_id=? AND state IN ('LIBRARIES_MAPPED','GALLERIES_REBUILT')`, formatTime(normalisedTime(now)), importID)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return errors.New("portable import session cannot complete Gallery rebuild")
	}
	return tx.Commit()
}

func publishPortableLifecycleClaims(ctx context.Context, tx *sql.Tx, importID string, now time.Time) error {
	type lifecycleClaim struct {
		uuid, kind, state, target, created, retired, reason string
	}
	rows, err := tx.QueryContext(ctx, `SELECT uuid,entity_kind,identity_state,target_uuid,created_at_utc,retired_at_utc,reason
		FROM portable_identity_claims WHERE import_id=? AND identity_state<>'ACTIVE' AND claim_state='PENDING'
		ORDER BY CASE identity_state WHEN 'ALIAS' THEN 0 ELSE 1 END,uuid`, importID)
	if err != nil {
		return err
	}
	var claims []lifecycleClaim
	for rows.Next() {
		var value lifecycleClaim
		if err := rows.Scan(&value.uuid, &value.kind, &value.state, &value.target, &value.created, &value.retired, &value.reason); err != nil {
			rows.Close()
			return err
		}
		claims = append(claims, value)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, value := range claims {
		kind := portableid.Kind(value.kind)
		result, err := tx.ExecContext(ctx, `UPDATE portable_identity_claims SET claim_state='CLAIMING'
			WHERE import_id=? AND uuid=? AND claim_state='PENDING'`, importID, value.uuid)
		if err != nil {
			return err
		}
		if changed, err := result.RowsAffected(); err != nil || changed != 1 {
			return errors.New("portable lifecycle UUID claim is no longer pending")
		}
		created, err := parseTime(value.created)
		if err != nil {
			return err
		}
		if _, err := registerPortableUUID(ctx, tx, value.uuid, kind, created); err != nil {
			return err
		}
		switch value.state {
		case "ALIAS":
			if _, err := tx.ExecContext(ctx, `INSERT INTO portable_uuid_aliases(alias_uuid,target_uuid,entity_kind,merged_at_utc)
				VALUES(?,?,?,?)`, value.uuid, value.target, value.kind, value.retired); err != nil {
				return err
			}
		case "TOMBSTONE":
			if _, err := tx.ExecContext(ctx, `INSERT INTO portable_uuid_tombstones(uuid,entity_kind,deleted_at_utc,reason)
				VALUES(?,?,?,?)`, value.uuid, value.kind, value.retired, value.reason); err != nil {
				return err
			}
		default:
			return errors.New("portable lifecycle claim state is invalid")
		}
		result, err = tx.ExecContext(ctx, `UPDATE portable_identity_claims SET claim_state='CLAIMED',claimed_at_utc=?
			WHERE import_id=? AND uuid=? AND claim_state='CLAIMING'`, formatTime(normalisedTime(now)), importID, value.uuid)
		if err != nil {
			return err
		}
		if changed, err := result.RowsAffected(); err != nil || changed != 1 {
			return errors.New("portable lifecycle UUID claim cannot be completed")
		}
	}
	return nil
}

func claimPortableUUID(ctx context.Context, tx *sql.Tx, importID, uuid string, kind portableid.Kind, now time.Time) error {
	var claimedKind, identityState, claimState string
	if err := tx.QueryRowContext(ctx, `SELECT entity_kind,identity_state,claim_state FROM portable_identity_claims
		WHERE import_id=? AND uuid=?`, importID, uuid).Scan(&claimedKind, &identityState, &claimState); err != nil {
		return err
	}
	if claimedKind != string(kind) || identityState != "ACTIVE" || claimState != "PENDING" {
		return errors.New("portable UUID claim is not an active pending identity of the required kind")
	}
	result, err := tx.ExecContext(ctx, `UPDATE portable_identity_claims SET claim_state='CLAIMING' WHERE import_id=? AND uuid=? AND claim_state='PENDING'`, importID, uuid)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return errors.New("portable UUID claim is no longer pending")
	}
	if _, err := registerPortableUUID(ctx, tx, uuid, kind, normalisedTime(now)); err != nil {
		return err
	}
	result, err = tx.ExecContext(ctx, `UPDATE portable_identity_claims SET claim_state='CLAIMED',claimed_at_utc=?
		WHERE import_id=? AND uuid=? AND claim_state='CLAIMING'`, formatTime(normalisedTime(now)), importID, uuid)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return errors.New("portable UUID claim cannot be completed")
	}
	return nil
}

func (db *Database) CreatePortableGalleryDraft(ctx context.Context, importID, setID string, libraryID int64, sourceType gallery.SourceType, sourcePath, title string, now time.Time) (gallery.Gallery, gallery.Source, error) {
	if sourceType != gallery.SourceTypeDirectory && sourceType != gallery.SourceTypeArchive {
		return gallery.Gallery{}, gallery.Source{}, errors.New("portable Gallery source type is invalid")
	}
	if err := validateGalleryMetadata(title, "", "", "", "", "", ""); err != nil {
		return gallery.Gallery{}, gallery.Source{}, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return gallery.Gallery{}, gallery.Source{}, err
	}
	defer tx.Rollback()
	var expectedLibraryID int64
	var expectedType gallery.SourceType
	var state string
	if err := tx.QueryRowContext(ctx, `SELECT mapping.target_library_id,rebuild.source_type,rebuild.state
		FROM portable_gallery_rebuilds rebuild JOIN portable_library_mappings mapping
		ON mapping.import_id=rebuild.import_id AND mapping.library_key=rebuild.library_key
		WHERE rebuild.import_id=? AND rebuild.set_id=? AND mapping.decision='MAPPED'`, importID, setID).Scan(&expectedLibraryID, &expectedType, &state); err != nil {
		return gallery.Gallery{}, gallery.Source{}, err
	}
	if state != "READY" || expectedLibraryID != libraryID || expectedType != sourceType {
		return gallery.Gallery{}, gallery.Source{}, errors.New("portable Gallery rebuild is not ready for this source")
	}
	mediaLibrary, err := findLibrary(ctx, tx, libraryID)
	if err != nil {
		return gallery.Gallery{}, gallery.Source{}, err
	}
	if !pathWithin(mediaLibrary.RootPath, sourcePath) {
		return gallery.Gallery{}, gallery.Source{}, errors.New("portable Gallery source is outside its mapped media library")
	}
	if err := claimPortableUUID(ctx, tx, importID, setID, portableid.KindGallery, now); err != nil {
		return gallery.Gallery{}, gallery.Source{}, err
	}
	timestamp := formatTime(normalisedTime(now))
	created, err := tx.ExecContext(ctx, `INSERT INTO galleries(set_id,slug,state,title,created_at_utc,updated_at_utc)
		VALUES(?,?,'DRAFT',?,?,?)`, setID, slug.FromName(title, setID), title, timestamp, timestamp)
	if err != nil {
		return gallery.Gallery{}, gallery.Source{}, err
	}
	galleryID, err := created.LastInsertId()
	if err != nil {
		return gallery.Gallery{}, gallery.Source{}, err
	}
	created, err = tx.ExecContext(ctx, `INSERT INTO gallery_sources(gallery_id,library_id,source_type,source_path,availability_state,reconcile_state,created_at_utc,updated_at_utc)
		VALUES(?,?,?,?,'AVAILABLE','NEVER_SCANNED',?,?)`, galleryID, libraryID, sourceType, sourcePath, timestamp, timestamp)
	if err != nil {
		return gallery.Gallery{}, gallery.Source{}, err
	}
	sourceID, err := created.LastInsertId()
	if err != nil {
		return gallery.Gallery{}, gallery.Source{}, err
	}
	created, err = tx.ExecContext(ctx, `UPDATE portable_gallery_rebuilds SET state='REBUILDING',gallery_id=?,source_id=?,issue_code='',updated_at_utc=?
		WHERE import_id=? AND set_id=? AND state='READY'`, galleryID, sourceID, timestamp, importID, setID)
	if err != nil {
		return gallery.Gallery{}, gallery.Source{}, err
	}
	if changed, err := created.RowsAffected(); err != nil || changed != 1 {
		return gallery.Gallery{}, gallery.Source{}, errors.New("portable Gallery rebuild cursor cannot start")
	}
	galleryValue, err := findGallery(ctx, tx, galleryID)
	if err != nil {
		return gallery.Gallery{}, gallery.Source{}, err
	}
	sourceValue, err := findSource(ctx, tx, sourceID)
	if err != nil {
		return gallery.Gallery{}, gallery.Source{}, err
	}
	if err := tx.Commit(); err != nil {
		return gallery.Gallery{}, gallery.Source{}, err
	}
	return galleryValue, sourceValue, nil
}

func (db *Database) FinishPortableGalleryRebuild(ctx context.Context, importID, setID string, galleryID, sourceID int64, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE portable_gallery_rebuilds SET state='REBUILT',issue_code='',updated_at_utc=?
		WHERE import_id=? AND set_id=? AND gallery_id=? AND source_id=? AND state='REBUILDING'`, formatTime(normalisedTime(now)), importID, setID, galleryID, sourceID)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return errors.New("portable Gallery rebuild cannot be completed")
	}
	return tx.Commit()
}
