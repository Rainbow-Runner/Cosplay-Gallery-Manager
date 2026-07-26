package productdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/portableid"
	"github.com/stashapp/stash/internal/slug"
	"github.com/stashapp/stash/internal/textsafe"
)

var (
	ErrGalleryNotFound          = errors.New("Gallery not found")
	ErrMetadataRevisionConflict = errors.New("Gallery metadata revision conflict")
)

type GalleryStore struct {
	db *sql.DB
}

func (db *Database) Galleries() *GalleryStore {
	return &GalleryStore{db: db.DB}
}

type CreateGalleryInput struct {
	Title            string
	Aliases          []string
	Description      string
	ContentRating    gallery.ContentRating
	PhotographerName string
	StudioName       string
}

type UpdateGalleryMetadataInput struct {
	Title              string
	Aliases            []string
	Description        string
	ShootDate          string
	ShootDatePrecision gallery.ShootDatePrecision
	ContentRating      gallery.ContentRating
	PhotographerName   string
	StudioName         string
}

type CreateSourceInput struct {
	LibraryID    *int64
	Type         gallery.SourceType
	Path         string
	Availability gallery.AvailabilityState
}

type CreateItemInput struct {
	RelativePath    string
	MediaKind       gallery.MediaKind
	ContentFormat   gallery.ContentFormat
	ImageCategory   gallery.ImageCategory
	Position        int64
	Caption         string
	Availability    gallery.AvailabilityState
	ProcessingState gallery.ProcessingState
}

type ActivationError struct {
	Blockers []gallery.ActivationBlocker
}

func (e *ActivationError) Error() string {
	return fmt.Sprintf("Gallery activation blocked by %d issue(s)", len(e.Blockers))
}

func (s *GalleryStore) Create(
	ctx context.Context,
	input CreateGalleryInput,
	now time.Time,
) (gallery.Gallery, error) {
	if err := validateGalleryMetadata(input.Title, input.Description, "", "", input.ContentRating, input.PhotographerName, input.StudioName); err != nil {
		return gallery.Gallery{}, err
	}
	if err := validateAliases(input.Aliases); err != nil {
		return gallery.Gallery{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return gallery.Gallery{}, err
	}
	defer func() { _ = tx.Rollback() }()

	setID := portableid.New()
	createdAt := normalisedTime(now)
	if _, err := registerPortableUUID(ctx, tx, setID, portableid.KindGallery, createdAt); err != nil {
		return gallery.Gallery{}, err
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO galleries (
			set_id, slug, state, title, description, content_rating,
			photographer_name, studio_name, created_at_utc, updated_at_utc
		) VALUES (?, ?, 'DRAFT', ?, ?, NULLIF(?, ''), ?, ?, ?, ?)
	`,
		setID,
		slug.FromName(input.Title, setID),
		input.Title,
		input.Description,
		input.ContentRating,
		input.PhotographerName,
		input.StudioName,
		formatTime(createdAt),
		formatTime(createdAt),
	)
	if err != nil {
		return gallery.Gallery{}, fmt.Errorf("creating Gallery: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return gallery.Gallery{}, err
	}
	if err := insertGalleryAliases(ctx, tx, id, input.Aliases); err != nil {
		return gallery.Gallery{}, err
	}

	created, err := findGallery(ctx, tx, id)
	if err != nil {
		return gallery.Gallery{}, err
	}
	created.Aliases, err = loadGalleryAliases(ctx, tx, id)
	if err != nil {
		return gallery.Gallery{}, err
	}
	if err := tx.Commit(); err != nil {
		return gallery.Gallery{}, err
	}
	return created, nil
}

func (s *GalleryStore) Find(ctx context.Context, id int64) (gallery.Gallery, error) {
	result, err := findGallery(ctx, s.db, id)
	if err != nil {
		return gallery.Gallery{}, err
	}
	result.Aliases, err = loadGalleryAliases(ctx, s.db, id)
	if err != nil {
		return gallery.Gallery{}, err
	}
	facts, err := activationFacts(ctx, s.db, id)
	if err != nil {
		return gallery.Gallery{}, err
	}
	result.Browsable = gallery.DeriveBrowsable(result.State, facts)
	return result, nil
}

func (s *GalleryStore) UpdateMetadata(
	ctx context.Context,
	id int64,
	expectedRevision int64,
	input UpdateGalleryMetadataInput,
	now time.Time,
) (gallery.Gallery, error) {
	if err := validateGalleryMetadata(
		input.Title,
		input.Description,
		input.ShootDate,
		input.ShootDatePrecision,
		input.ContentRating,
		input.PhotographerName,
		input.StudioName,
	); err != nil {
		return gallery.Gallery{}, err
	}
	if err := validateAliases(input.Aliases); err != nil {
		return gallery.Gallery{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return gallery.Gallery{}, err
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
		UPDATE galleries SET
			title = ?, description = ?, shoot_date = NULLIF(?, ''),
			shoot_date_precision = NULLIF(?, ''), content_rating = NULLIF(?, ''),
			photographer_name = ?, studio_name = ?, updated_at_utc = ?,
			metadata_revision = metadata_revision + 1
		WHERE id = ? AND metadata_revision = ?
	`,
		input.Title,
		input.Description,
		input.ShootDate,
		input.ShootDatePrecision,
		input.ContentRating,
		input.PhotographerName,
		input.StudioName,
		formatTime(normalisedTime(now)),
		id,
		expectedRevision,
	)
	if err != nil {
		return gallery.Gallery{}, fmt.Errorf("updating Gallery metadata: %w", err)
	}
	if err := requireOneRevisionRow(result); err != nil {
		return gallery.Gallery{}, err
	}
	if err := markGalleryManifestDBDirty(ctx, tx, id); err != nil {
		return gallery.Gallery{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM gallery_aliases WHERE gallery_id = ?`, id); err != nil {
		return gallery.Gallery{}, err
	}
	if err := insertGalleryAliases(ctx, tx, id, input.Aliases); err != nil {
		return gallery.Gallery{}, err
	}
	if err := demoteInvalidActiveGallery(ctx, tx, id); err != nil {
		return gallery.Gallery{}, err
	}
	updated, err := findGallery(ctx, tx, id)
	if err != nil {
		return gallery.Gallery{}, err
	}
	updated.Aliases, err = loadGalleryAliases(ctx, tx, id)
	if err != nil {
		return gallery.Gallery{}, err
	}
	if err := deriveGalleryBrowsable(ctx, tx, &updated); err != nil {
		return gallery.Gallery{}, err
	}
	if err := tx.Commit(); err != nil {
		return gallery.Gallery{}, err
	}
	return updated, nil
}

func (s *GalleryStore) AddSource(
	ctx context.Context,
	galleryID int64,
	input CreateSourceInput,
	now time.Time,
) (gallery.Source, error) {
	if input.Type != gallery.SourceTypeDirectory && input.Type != gallery.SourceTypeArchive {
		return gallery.Source{}, fmt.Errorf("unsupported GallerySource type %q", input.Type)
	}
	if input.Path == "" || len([]rune(input.Path)) > 4096 {
		return gallery.Source{}, errors.New("GallerySource path must contain 1 to 4096 characters")
	}
	if !validAvailability(input.Availability) {
		return gallery.Source{}, fmt.Errorf("unsupported source availability %q", input.Availability)
	}
	if input.LibraryID != nil {
		mediaLibrary, err := findLibrary(ctx, s.db, *input.LibraryID)
		if err != nil {
			return gallery.Source{}, err
		}
		if !pathWithin(mediaLibrary.RootPath, input.Path) {
			return gallery.Source{}, fmt.Errorf(
				"GallerySource path %q is outside media library %q",
				input.Path,
				mediaLibrary.RootPath,
			)
		}
	}

	timestamp := normalisedTime(now)
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO gallery_sources (
			gallery_id, library_id, source_type, source_path, availability_state,
			reconcile_state, created_at_utc, updated_at_utc
		) VALUES (?, ?, ?, ?, ?, 'NEVER_SCANNED', ?, ?)
	`, galleryID, input.LibraryID, input.Type, input.Path, input.Availability, formatTime(timestamp), formatTime(timestamp))
	if err != nil {
		return gallery.Source{}, fmt.Errorf("creating GallerySource: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return gallery.Source{}, err
	}
	return findSource(ctx, s.db, id)
}

func (s *GalleryStore) AddItem(
	ctx context.Context,
	galleryID int64,
	sourceID int64,
	input CreateItemInput,
	now time.Time,
) (gallery.Item, error) {
	if err := validateItemInput(input); err != nil {
		return gallery.Item{}, err
	}
	input.ContentFormat = effectiveContentFormat(input.MediaKind, input.ContentFormat)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return gallery.Item{}, err
	}
	defer func() { _ = tx.Rollback() }()

	itemUUID := portableid.New()
	timestamp := normalisedTime(now)
	if _, err := registerPortableUUID(ctx, tx, itemUUID, portableid.KindGalleryItem, timestamp); err != nil {
		return gallery.Item{}, err
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO gallery_items (
			item_uuid, gallery_id, source_id, relative_path, media_kind,
			content_format, image_category, position, caption, availability_state,
			processing_state, created_at_utc, updated_at_utc
		) VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?)
	`,
		itemUUID,
		galleryID,
		sourceID,
		input.RelativePath,
		input.MediaKind,
		input.ContentFormat,
		input.ImageCategory,
		input.Position,
		input.Caption,
		input.Availability,
		input.ProcessingState,
		formatTime(timestamp),
		formatTime(timestamp),
	)
	if err != nil {
		return gallery.Item{}, fmt.Errorf("creating GalleryItem: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return gallery.Item{}, err
	}
	item, err := findItem(ctx, tx, id)
	if err != nil {
		return gallery.Item{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE galleries SET scan_revision = scan_revision + 1, scrubber_revision = scrubber_revision + 1 WHERE id = ?
	`, galleryID); err != nil {
		return gallery.Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return gallery.Item{}, err
	}
	return item, nil
}

func (s *GalleryStore) AddCredit(
	ctx context.Context,
	galleryID int64,
	coserUUID string,
	position int64,
	expectedRevision int64,
	now time.Time,
) (int64, error) {
	if position <= 0 {
		return 0, errors.New("GalleryCredit position must be positive")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := requireActivePortableKind(ctx, tx, coserUUID, portableid.KindCoser); err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO gallery_credits (gallery_id, coser_uuid, position)
		VALUES (?, ?, ?)
	`, galleryID, coserUUID, position)
	if err != nil {
		return 0, fmt.Errorf("creating GalleryCredit: %w", err)
	}
	creditID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := touchGalleryMetadata(ctx, tx, galleryID, expectedRevision, now); err != nil {
		return 0, err
	}
	if err := demoteInvalidActiveGallery(ctx, tx, galleryID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return creditID, nil
}

func (s *GalleryStore) AddCast(
	ctx context.Context,
	galleryID int64,
	creditID int64,
	characterUUID string,
	position int64,
	expectedRevision int64,
	now time.Time,
) error {
	if position <= 0 {
		return errors.New("GalleryCast position must be positive")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := requireActivePortableKind(ctx, tx, characterUUID, portableid.KindCharacter); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO gallery_cast (
			gallery_id, gallery_credit_id, character_uuid, position
		) VALUES (?, ?, ?, ?)
	`, galleryID, creditID, characterUUID, position)
	if err != nil {
		return fmt.Errorf("creating GalleryCast: %w", err)
	}
	if err := touchGalleryMetadata(ctx, tx, galleryID, expectedRevision, now); err != nil {
		return err
	}
	if err := demoteInvalidActiveGallery(ctx, tx, galleryID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *GalleryStore) SetState(
	ctx context.Context,
	id int64,
	expectedRevision int64,
	target gallery.State,
	now time.Time,
) (gallery.Gallery, error) {
	if target != gallery.StateDraft && target != gallery.StateActive && target != gallery.StateArchived {
		return gallery.Gallery{}, fmt.Errorf("unsupported Gallery state %q", target)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return gallery.Gallery{}, err
	}
	defer func() { _ = tx.Rollback() }()

	current, err := findGallery(ctx, tx, id)
	if err != nil {
		return gallery.Gallery{}, err
	}
	if current.MetadataRevision != expectedRevision {
		return gallery.Gallery{}, fmt.Errorf("%w: expected %d, found %d", ErrMetadataRevisionConflict, expectedRevision, current.MetadataRevision)
	}

	if target == gallery.StateActive {
		facts, err := activationFacts(ctx, tx, id)
		if err != nil {
			return gallery.Gallery{}, err
		}
		if blockers := current.ActivationBlockers(facts); len(blockers) > 0 {
			if current.State == gallery.StateArchived {
				if _, err := tx.ExecContext(ctx, `
					UPDATE galleries SET state = 'DRAFT', updated_at_utc = ?,
						metadata_revision = metadata_revision + 1
					WHERE id = ? AND metadata_revision = ?
				`, formatTime(normalisedTime(now)), id, expectedRevision); err != nil {
					return gallery.Gallery{}, err
				}
				if err := tx.Commit(); err != nil {
					return gallery.Gallery{}, err
				}
			}
			return gallery.Gallery{}, &ActivationError{Blockers: blockers}
		}
	}

	timestamp := formatTime(normalisedTime(now))
	result, err := tx.ExecContext(ctx, `
		UPDATE galleries SET
			state = ?,
			added_at_utc = CASE
				WHEN ? = 'ACTIVE' THEN COALESCE(added_at_utc, ?)
				ELSE added_at_utc
			END,
			updated_at_utc = ?, metadata_revision = metadata_revision + 1
		WHERE id = ? AND metadata_revision = ?
	`, target, target, timestamp, timestamp, id, expectedRevision)
	if err != nil {
		return gallery.Gallery{}, fmt.Errorf("transitioning Gallery state: %w", err)
	}
	if err := requireOneRevisionRow(result); err != nil {
		return gallery.Gallery{}, err
	}
	updated, err := findGallery(ctx, tx, id)
	if err != nil {
		return gallery.Gallery{}, err
	}
	if err := deriveGalleryBrowsable(ctx, tx, &updated); err != nil {
		return gallery.Gallery{}, err
	}
	if err := tx.Commit(); err != nil {
		return gallery.Gallery{}, err
	}
	return updated, nil
}

func (s *GalleryStore) SetSourceHealth(
	ctx context.Context,
	sourceID int64,
	availability gallery.AvailabilityState,
	reconcile gallery.ReconcileState,
	overLimit bool,
	now time.Time,
) error {
	if !validAvailability(availability) || !validReconcile(reconcile) {
		return errors.New("invalid GallerySource health state")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		UPDATE gallery_sources SET availability_state = ?, reconcile_state = ?,
			over_limit = ?, updated_at_utc = ?
		WHERE id = ?
	`, availability, reconcile, overLimit, formatTime(normalisedTime(now)), sourceID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("GallerySource not found")
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE galleries SET scan_revision = scan_revision + 1, scrubber_revision = scrubber_revision + 1
		WHERE id = (SELECT gallery_id FROM gallery_sources WHERE id = ?)
	`, sourceID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *GalleryStore) PutSourceIssue(
	ctx context.Context,
	sourceID int64,
	code string,
	severity gallery.IssueSeverity,
	message string,
	now time.Time,
) error {
	if code == "" || len([]rune(code)) > 100 || len([]rune(message)) > 2000 {
		return errors.New("invalid GallerySourceIssue text length")
	}
	if severity != gallery.IssueSeverityInfo && severity != gallery.IssueSeverityWarning && severity != gallery.IssueSeverityBlocking {
		return fmt.Errorf("unsupported issue severity %q", severity)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO gallery_source_issues (
			source_id, code, severity, message, created_at_utc, resolved_at_utc
		) VALUES (?, ?, ?, ?, ?, NULL)
		ON CONFLICT(source_id, code) DO UPDATE SET
			severity = excluded.severity, message = excluded.message,
			created_at_utc = excluded.created_at_utc, resolved_at_utc = NULL
	`, sourceID, code, severity, message, formatTime(normalisedTime(now)))
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE galleries SET scan_revision = scan_revision + 1, scrubber_revision = scrubber_revision + 1
		WHERE id = (SELECT gallery_id FROM gallery_sources WHERE id = ?)
	`, sourceID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *GalleryStore) ResolveSourceIssue(ctx context.Context, sourceID int64, code string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
		UPDATE gallery_source_issues SET resolved_at_utc = ?
		WHERE source_id = ? AND code = ? AND resolved_at_utc IS NULL
	`, formatTime(normalisedTime(now)), sourceID, code)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE galleries SET scan_revision = scan_revision + 1, scrubber_revision = scrubber_revision + 1
		WHERE id = (SELECT gallery_id FROM gallery_sources WHERE id = ?)
	`, sourceID); err != nil {
		return err
	}
	return tx.Commit()
}

func touchGalleryMetadata(
	ctx context.Context,
	tx *sql.Tx,
	galleryID int64,
	expectedRevision int64,
	now time.Time,
) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE galleries SET metadata_revision = metadata_revision + 1,
			updated_at_utc = ?
		WHERE id = ? AND metadata_revision = ?
	`, formatTime(normalisedTime(now)), galleryID, expectedRevision)
	if err != nil {
		return err
	}
	if err := requireOneRevisionRow(result); err != nil {
		return err
	}
	return markGalleryManifestDBDirty(ctx, tx, galleryID)
}

func markGalleryManifestDBDirty(ctx context.Context, tx *sql.Tx, galleryID int64) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE gallery_manifest_sync SET status = CASE
			WHEN status = 'CLEAN' THEN 'DB_DIRTY'
			ELSE status END
		WHERE gallery_id = ?
	`, galleryID)
	return err
}

func demoteInvalidActiveGallery(ctx context.Context, tx *sql.Tx, galleryID int64) error {
	current, err := findGallery(ctx, tx, galleryID)
	if err != nil {
		return err
	}
	if current.State != gallery.StateActive {
		return nil
	}
	facts, err := activationFacts(ctx, tx, galleryID)
	if err != nil {
		return err
	}
	if len(current.ActivationBlockers(facts)) == 0 {
		return nil
	}
	_, err = tx.ExecContext(ctx, `UPDATE galleries SET state = 'DRAFT' WHERE id = ?`, galleryID)
	return err
}

func deriveGalleryBrowsable(ctx context.Context, queryer galleryQueryer, target *gallery.Gallery) error {
	facts, err := activationFacts(ctx, queryer, target.ID)
	if err != nil {
		return err
	}
	target.Browsable = gallery.DeriveBrowsable(target.State, facts)
	return nil
}

type galleryQueryer interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

func findGallery(ctx context.Context, queryer galleryQueryer, id int64) (gallery.Gallery, error) {
	var (
		result         gallery.Gallery
		shootDate      sql.NullString
		shootPrecision sql.NullString
		contentRating  sql.NullString
		createdAt      string
		updatedAt      string
		addedAt        sql.NullString
	)
	err := queryer.QueryRowContext(ctx, `
		SELECT id, set_id, slug, state, title, description, shoot_date,
			shoot_date_precision, content_rating, photographer_name,
			studio_name, created_at_utc, updated_at_utc, added_at_utc,
			metadata_revision, scan_revision, scrubber_revision
		FROM galleries WHERE id = ?
	`, id).Scan(
		&result.ID, &result.SetID, &result.Slug, &result.State, &result.Title,
		&result.Description, &shootDate, &shootPrecision, &contentRating,
		&result.PhotographerName, &result.StudioName, &createdAt, &updatedAt,
		&addedAt, &result.MetadataRevision, &result.ScanRevision, &result.ScrubberRevision,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return gallery.Gallery{}, ErrGalleryNotFound
	}
	if err != nil {
		return gallery.Gallery{}, err
	}
	result.ShootDate = shootDate.String
	result.ShootDatePrecision = gallery.ShootDatePrecision(shootPrecision.String)
	result.ContentRating = gallery.ContentRating(contentRating.String)
	if result.CreatedAtUTC, err = parseTime(createdAt); err != nil {
		return gallery.Gallery{}, err
	}
	if result.UpdatedAtUTC, err = parseTime(updatedAt); err != nil {
		return gallery.Gallery{}, err
	}
	if addedAt.Valid {
		parsed, err := parseTime(addedAt.String)
		if err != nil {
			return gallery.Gallery{}, err
		}
		result.AddedAtUTC = &parsed
	}
	return result, nil
}

func activationFacts(ctx context.Context, queryer galleryQueryer, galleryID int64) (gallery.ActivationFacts, error) {
	var facts gallery.ActivationFacts
	var sourceAvailable, sourceOverLimit int
	err := queryer.QueryRowContext(ctx, `
		SELECT
			EXISTS(SELECT 1 FROM gallery_sources WHERE gallery_id = ?),
			EXISTS(SELECT 1 FROM gallery_sources WHERE gallery_id = ? AND availability_state = 'AVAILABLE'),
			EXISTS(SELECT 1 FROM gallery_sources WHERE gallery_id = ? AND over_limit = 1),
			(SELECT COUNT(*) FROM gallery_source_issues issue
				JOIN gallery_sources source ON source.id = issue.source_id
				WHERE source.gallery_id = ? AND issue.severity = 'BLOCKING'
					AND issue.resolved_at_utc IS NULL),
			(SELECT COUNT(*) FROM gallery_items WHERE gallery_id = ?
				AND excluded = 0 AND availability_state = 'AVAILABLE'
				AND processing_state = 'READY'),
			(SELECT COUNT(*) FROM gallery_credits WHERE gallery_id = ?),
			(SELECT COUNT(*) FROM gallery_cast WHERE gallery_id = ?),
			(SELECT COUNT(*) FROM gallery_credits credit
				WHERE credit.gallery_id = ? AND NOT EXISTS (
					SELECT 1 FROM gallery_cast cast_item
					WHERE cast_item.gallery_credit_id = credit.id
				)),
			(SELECT COUNT(*) FROM gallery_identity_suggestions
				WHERE gallery_id = ? AND status = 'PENDING')
	`, galleryID, galleryID, galleryID, galleryID, galleryID, galleryID, galleryID, galleryID, galleryID).Scan(
		&facts.HasSource,
		&sourceAvailable,
		&sourceOverLimit,
		&facts.BlockingIssueCount,
		&facts.DisplayableItemCount,
		&facts.CreditCount,
		&facts.CastCount,
		&facts.CreditsWithoutCharacterCount,
		&facts.UnresolvedIdentitySuggestions,
	)
	if err != nil {
		return gallery.ActivationFacts{}, err
	}
	facts.SourceAvailable = sourceAvailable == 1
	facts.SourceOverLimit = sourceOverLimit == 1
	return facts, nil
}

func requireActivePortableKind(ctx context.Context, queryer portableUUIDQueryer, value string, kind portableid.Kind) error {
	record, err := lookupPortableUUID(ctx, queryer, value)
	if err != nil {
		return err
	}
	if record.Kind != kind {
		return &PortableUUIDKindConflictError{UUID: value, Found: record.Kind, Requested: kind}
	}
	if record.State != PortableUUIDActive {
		return fmt.Errorf("%w: %s is %s", ErrPortableUUIDNotActive, value, record.State)
	}
	return nil
}

func validateGalleryMetadata(
	title string,
	description string,
	shootDate string,
	precision gallery.ShootDatePrecision,
	rating gallery.ContentRating,
	photographer string,
	studio string,
) error {
	if len([]rune(title)) > 300 || len([]rune(description)) > 20000 ||
		len([]rune(photographer)) > 200 || len([]rune(studio)) > 200 {
		return errors.New("Gallery metadata exceeds a field length limit")
	}
	if err := textsafe.ValidateMarkdown(description); err != nil {
		return fmt.Errorf("invalid Gallery description: %w", err)
	}
	if rating != "" && rating != gallery.ContentRatingNonAdult && rating != gallery.ContentRatingAdult {
		return fmt.Errorf("unsupported content rating %q", rating)
	}
	if shootDate == "" && precision != "" || shootDate != "" && precision == "" {
		return errors.New("shoot date and precision must be provided together")
	}
	if shootDate != "" {
		if precision != gallery.ShootDatePrecisionMonth && precision != gallery.ShootDatePrecisionDay {
			return fmt.Errorf("unsupported shoot date precision %q", precision)
		}
		layout := "2006-01"
		if precision == gallery.ShootDatePrecisionDay {
			layout = "2006-01-02"
		}
		if parsed, err := time.Parse(layout, shootDate); err != nil || parsed.Format(layout) != shootDate {
			return fmt.Errorf("invalid shoot date %q for precision %s", shootDate, precision)
		}
	}
	return nil
}

func validateAliases(aliases []string) error {
	return validateNamedInput(CreateNamedEntityInput{Name: "placeholder", Aliases: aliases})
}

func insertGalleryAliases(ctx context.Context, tx *sql.Tx, galleryID int64, aliases []string) error {
	for index, alias := range aliases {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO gallery_aliases (gallery_id, alias, normalized_alias, position)
			VALUES (?, ?, ?, ?)
		`, galleryID, normalizedDisplay(alias), normalizedKey(alias), int64(index+1)*1024); err != nil {
			return err
		}
	}
	return nil
}

func loadGalleryAliases(ctx context.Context, queryer rowsQueryer, galleryID int64) ([]string, error) {
	rows, err := queryer.QueryContext(ctx, `
		SELECT alias FROM gallery_aliases WHERE gallery_id = ? ORDER BY position
	`, galleryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var aliases []string
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			return nil, err
		}
		aliases = append(aliases, alias)
	}
	return aliases, rows.Err()
}

func validateItemInput(input CreateItemInput) error {
	input.ContentFormat = effectiveContentFormat(input.MediaKind, input.ContentFormat)
	if input.RelativePath == "" || len([]rune(input.RelativePath)) > 4096 ||
		strings.Contains(input.RelativePath, "\\") || path.Clean(input.RelativePath) != input.RelativePath ||
		strings.HasPrefix(input.RelativePath, "/") || input.RelativePath == "." || input.RelativePath == ".." {
		return errors.New("GalleryItem relative path must be a canonical forward-slash path within its source")
	}
	if input.Position <= 0 {
		return errors.New("GalleryItem position must be positive")
	}
	if len([]rune(input.Caption)) > 1000 {
		return errors.New("GalleryItem caption exceeds 1000 characters")
	}
	if !validAvailability(input.Availability) {
		return fmt.Errorf("unsupported item availability %q", input.Availability)
	}
	if input.ProcessingState != gallery.ProcessingPending && input.ProcessingState != gallery.ProcessingProcessing &&
		input.ProcessingState != gallery.ProcessingReady && input.ProcessingState != gallery.ProcessingError {
		return fmt.Errorf("unsupported item processing state %q", input.ProcessingState)
	}
	switch input.MediaKind {
	case gallery.MediaKindStaticImage:
		if input.ImageCategory != gallery.ImageCategoryPhoto && input.ImageCategory != gallery.ImageCategorySelfie {
			return errors.New("static images require PHOTO or SELFIE category")
		}
	case gallery.MediaKindAnimatedImage, gallery.MediaKindVideo:
		if input.ImageCategory != "" {
			return errors.New("animated images and videos cannot have an image category")
		}
	default:
		return fmt.Errorf("unsupported media kind %q", input.MediaKind)
	}
	if (input.ContentFormat == gallery.ContentFormatRAW && input.MediaKind != gallery.MediaKindStaticImage) ||
		(input.ContentFormat == gallery.ContentFormatVideo && input.MediaKind != gallery.MediaKindVideo) ||
		(input.ContentFormat == gallery.ContentFormatImage && input.MediaKind == gallery.MediaKindVideo) {
		return errors.New("GalleryItem content format does not match media kind")
	}
	return nil
}

func effectiveContentFormat(kind gallery.MediaKind, value gallery.ContentFormat) gallery.ContentFormat {
	if value != "" {
		return value
	}
	if kind == gallery.MediaKindVideo {
		return gallery.ContentFormatVideo
	}
	return gallery.ContentFormatImage
}

func validAvailability(value gallery.AvailabilityState) bool {
	return value == gallery.AvailabilityAvailable || value == gallery.AvailabilityMissing || value == gallery.AvailabilityUnreadable
}

func validReconcile(value gallery.ReconcileState) bool {
	return value == gallery.ReconcileNeverScanned || value == gallery.ReconcileScanning ||
		value == gallery.ReconcileInSync || value == gallery.ReconcileNeedsRescan || value == gallery.ReconcileError
}

func requireOneRevisionRow(result sql.Result) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrMetadataRevisionConflict
	}
	return nil
}

func formatTime(value time.Time) string {
	return value.UTC().Round(0).Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing database UTC time: %w", err)
	}
	return parsed, nil
}
