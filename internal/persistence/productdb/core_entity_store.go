package productdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/coreentity"
	"github.com/stashapp/stash/internal/manifest"
	"github.com/stashapp/stash/internal/portableid"
	"github.com/stashapp/stash/internal/slug"
	"github.com/stashapp/stash/internal/textsafe"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

var (
	ErrCoreMetadataRevisionConflict = errors.New("core entity metadata revision conflict")
	ErrTagNameAmbiguous             = errors.New("Tag name or alias is already occupied")
	ErrCoreEntityReferenced         = errors.New("core entity is still referenced")
)

type CoreEntityStore struct {
	db *sql.DB
}

func (db *Database) CoreEntities() *CoreEntityStore {
	return &CoreEntityStore{db: db.DB}
}

type CreateNamedEntityInput struct {
	UUID     string
	Name     string
	SortName string
	Aliases  []string
}

type UpdateNamedEntityInput struct {
	Name     string
	SortName string
	Aliases  []string
}

type CreateCoserInput struct {
	CreateNamedEntityInput
	ProfileSummary  string
	Biography       string
	CountryOrRegion string
}

type UpdateCoserInput struct {
	Name             string
	SortName         string
	Aliases          []string
	ProfileSummary   string
	Biography        string
	CountryOrRegion  string
	AvatarPath       string
	BannerPath       string
	AvatarCrop       *coreentity.AvatarCrop
	BannerFocalPoint *coreentity.FocalPoint
}

func (s *CoreEntityStore) CreateCoser(ctx context.Context, input CreateCoserInput, now time.Time) (coreentity.Coser, error) {
	if err := validateNamedInput(input.CreateNamedEntityInput); err != nil {
		return coreentity.Coser{}, err
	}
	if runeLength(input.ProfileSummary) > 20000 || runeLength(input.Biography) > 20000 || runeLength(input.CountryOrRegion) > 100 {
		return coreentity.Coser{}, errors.New("Coser profile field exceeds its length limit")
	}
	if err := textsafe.ValidateMarkdown(input.Biography); err != nil {
		return coreentity.Coser{}, fmt.Errorf("invalid Coser biography: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return coreentity.Coser{}, err
	}
	defer func() { _ = tx.Rollback() }()
	entityUUID, err := allocateEntityUUID(ctx, tx, input.UUID, portableid.KindCoser, now)
	if err != nil {
		return coreentity.Coser{}, err
	}
	timestamp := formatTime(normalisedTime(now))
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO cosers (uuid, name, sort_name, slug, profile_summary, biography,
			country_or_region, created_at_utc, updated_at_utc)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, entityUUID, normalizedDisplay(input.Name), normalizedDisplay(input.SortName),
		slug.FromName(input.Name, entityUUID), input.ProfileSummary, input.Biography,
		input.CountryOrRegion, timestamp, timestamp); err != nil {
		return coreentity.Coser{}, err
	}
	if err := insertAliases(ctx, tx, "coser_aliases", "coser_uuid", entityUUID, input.Aliases); err != nil {
		return coreentity.Coser{}, err
	}
	result, err := findCoser(ctx, tx, entityUUID)
	if err != nil {
		return coreentity.Coser{}, err
	}
	if err := tx.Commit(); err != nil {
		return coreentity.Coser{}, err
	}
	return result, nil
}

func (s *CoreEntityStore) UpdateCoser(
	ctx context.Context,
	uuid string,
	expectedRevision int64,
	input UpdateCoserInput,
	now time.Time,
) (coreentity.Coser, error) {
	if err := validateNamedInput(CreateNamedEntityInput{Name: input.Name, SortName: input.SortName, Aliases: input.Aliases}); err != nil {
		return coreentity.Coser{}, err
	}
	if runeLength(input.ProfileSummary) > 20000 || runeLength(input.Biography) > 20000 || runeLength(input.CountryOrRegion) > 100 {
		return coreentity.Coser{}, errors.New("Coser profile field exceeds its length limit")
	}
	if err := textsafe.ValidateMarkdown(input.Biography); err != nil {
		return coreentity.Coser{}, fmt.Errorf("invalid Coser biography: %w", err)
	}
	for _, asset := range []string{input.AvatarPath, input.BannerPath} {
		if asset != "" {
			if err := manifest.ValidateManagedRelativeAsset(asset); err != nil {
				return coreentity.Coser{}, err
			}
		}
	}
	if err := validateCoserCrop(input.AvatarCrop, input.BannerFocalPoint); err != nil {
		return coreentity.Coser{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return coreentity.Coser{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := findCoser(ctx, tx, uuid)
	if err != nil {
		return coreentity.Coser{}, err
	}
	avatarCrop := input.AvatarCrop
	if current.AvatarPath != input.AvatarPath {
		avatarCrop = nil
	}
	var cropX, cropY, cropSize, focalX, focalY any
	if avatarCrop != nil {
		cropX, cropY, cropSize = avatarCrop.X, avatarCrop.Y, avatarCrop.Size
	}
	if input.BannerFocalPoint != nil {
		focalX, focalY = input.BannerFocalPoint.X, input.BannerFocalPoint.Y
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE cosers SET name = ?, sort_name = ?, profile_summary = ?, biography = ?,
			country_or_region = ?, avatar_path = ?, banner_path = ?,
			avatar_crop_x = ?, avatar_crop_y = ?, avatar_crop_size = ?,
			banner_focal_x = ?, banner_focal_y = ?,
			metadata_revision = metadata_revision + 1, updated_at_utc = ?
		WHERE uuid = ? AND metadata_revision = ?
	`, normalizedDisplay(input.Name), normalizedDisplay(input.SortName), input.ProfileSummary,
		input.Biography, normalizedDisplay(input.CountryOrRegion), input.AvatarPath,
		input.BannerPath, cropX, cropY, cropSize, focalX, focalY,
		formatTime(normalisedTime(now)), uuid, expectedRevision)
	if err != nil {
		return coreentity.Coser{}, err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return coreentity.Coser{}, ErrCoreMetadataRevisionConflict
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM coser_aliases WHERE coser_uuid = ?`, uuid); err != nil {
		return coreentity.Coser{}, err
	}
	if err := insertAliases(ctx, tx, "coser_aliases", "coser_uuid", uuid, input.Aliases); err != nil {
		return coreentity.Coser{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE coser_manifest_sync SET status = CASE WHEN status = 'CLEAN' THEN 'DB_DIRTY' ELSE status END
		WHERE coser_uuid = ?
	`, uuid); err != nil {
		return coreentity.Coser{}, err
	}
	updated, err := findCoser(ctx, tx, uuid)
	if err != nil {
		return coreentity.Coser{}, err
	}
	if err := tx.Commit(); err != nil {
		return coreentity.Coser{}, err
	}
	return updated, nil
}

func validateCoserCrop(crop *coreentity.AvatarCrop, focal *coreentity.FocalPoint) error {
	if crop != nil && (crop.X < 0 || crop.X > 1 || crop.Y < 0 || crop.Y > 1 || crop.Size <= 0 || crop.Size > 1 || crop.X+crop.Size > 1 || crop.Y+crop.Size > 1) {
		return errors.New("Coser avatar crop must be a normalized in-bounds square")
	}
	if focal != nil && (focal.X < 0 || focal.X > 1 || focal.Y < 0 || focal.Y > 1) {
		return errors.New("Coser banner focal point must be normalized")
	}
	return nil
}

func (s *CoreEntityStore) CreateWork(ctx context.Context, input CreateNamedEntityInput, now time.Time) (coreentity.Work, error) {
	if err := validateNamedInput(input); err != nil {
		return coreentity.Work{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return coreentity.Work{}, err
	}
	defer func() { _ = tx.Rollback() }()
	entityUUID, err := allocateEntityUUID(ctx, tx, input.UUID, portableid.KindWork, now)
	if err != nil {
		return coreentity.Work{}, err
	}
	timestamp := formatTime(normalisedTime(now))
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO works (uuid, name, sort_name, slug, created_at_utc, updated_at_utc)
		VALUES (?, ?, ?, ?, ?, ?)
	`, entityUUID, normalizedDisplay(input.Name), normalizedDisplay(input.SortName),
		slug.FromName(input.Name, entityUUID), timestamp, timestamp); err != nil {
		return coreentity.Work{}, err
	}
	if err := insertAliases(ctx, tx, "work_aliases", "work_uuid", entityUUID, input.Aliases); err != nil {
		return coreentity.Work{}, err
	}
	result, err := findWork(ctx, tx, entityUUID)
	if err != nil {
		return coreentity.Work{}, err
	}
	if err := tx.Commit(); err != nil {
		return coreentity.Work{}, err
	}
	return result, nil
}

func (s *CoreEntityStore) UpdateWork(ctx context.Context, uuid string, expectedRevision int64, input UpdateNamedEntityInput, now time.Time) (coreentity.Work, error) {
	if err := validateNamedInput(CreateNamedEntityInput{Name: input.Name, SortName: input.SortName, Aliases: input.Aliases}); err != nil {
		return coreentity.Work{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return coreentity.Work{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := updateNamedEntity(ctx, tx, "works", "work_aliases", "work_uuid", uuid, expectedRevision, input, now, false); err != nil {
		return coreentity.Work{}, err
	}
	updated, err := findWork(ctx, tx, uuid)
	if err != nil {
		return coreentity.Work{}, err
	}
	if err := tx.Commit(); err != nil {
		return coreentity.Work{}, err
	}
	return updated, nil
}

func (s *CoreEntityStore) CreateCharacter(
	ctx context.Context,
	workUUID string,
	input CreateNamedEntityInput,
	now time.Time,
) (coreentity.Character, error) {
	if err := validateNamedInput(input); err != nil {
		return coreentity.Character{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return coreentity.Character{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := requireActivePortableKind(ctx, tx, workUUID, portableid.KindWork); err != nil {
		return coreentity.Character{}, err
	}
	entityUUID, err := allocateEntityUUID(ctx, tx, input.UUID, portableid.KindCharacter, now)
	if err != nil {
		return coreentity.Character{}, err
	}
	timestamp := formatTime(normalisedTime(now))
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO characters (uuid, work_uuid, name, normalized_name, sort_name,
			slug, created_at_utc, updated_at_utc)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, entityUUID, workUUID, normalizedDisplay(input.Name), normalizedKey(input.Name),
		normalizedDisplay(input.SortName), slug.FromName(input.Name, entityUUID), timestamp, timestamp); err != nil {
		return coreentity.Character{}, err
	}
	if err := insertAliases(ctx, tx, "character_aliases", "character_uuid", entityUUID, input.Aliases); err != nil {
		return coreentity.Character{}, err
	}
	result, err := findCharacter(ctx, tx, entityUUID)
	if err != nil {
		return coreentity.Character{}, err
	}
	if err := tx.Commit(); err != nil {
		return coreentity.Character{}, err
	}
	return result, nil
}

func (s *CoreEntityStore) UpdateCharacter(ctx context.Context, uuid string, expectedRevision int64, input UpdateNamedEntityInput, now time.Time) (coreentity.Character, error) {
	if err := validateNamedInput(CreateNamedEntityInput{Name: input.Name, SortName: input.SortName, Aliases: input.Aliases}); err != nil {
		return coreentity.Character{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return coreentity.Character{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := updateNamedEntity(ctx, tx, "characters", "character_aliases", "character_uuid", uuid, expectedRevision, input, now, true); err != nil {
		return coreentity.Character{}, err
	}
	updated, err := findCharacter(ctx, tx, uuid)
	if err != nil {
		return coreentity.Character{}, err
	}
	if err := tx.Commit(); err != nil {
		return coreentity.Character{}, err
	}
	return updated, nil
}

type CreateTagInput struct {
	CreateNamedEntityInput
	UseInRecommendation bool
}

func (s *CoreEntityStore) CreateTag(ctx context.Context, input CreateTagInput, now time.Time) (coreentity.Tag, error) {
	if err := validateNamedInput(input.CreateNamedEntityInput); err != nil {
		return coreentity.Tag{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return coreentity.Tag{}, err
	}
	defer func() { _ = tx.Rollback() }()
	allNames := append([]string{input.Name}, input.Aliases...)
	localNames := make(map[string]struct{}, len(allNames))
	for _, name := range allNames {
		key := normalizedKey(name)
		if _, duplicate := localNames[key]; duplicate {
			return coreentity.Tag{}, ErrTagNameAmbiguous
		}
		localNames[key] = struct{}{}
		if occupied, err := tagNameOccupied(ctx, tx, key); err != nil {
			return coreentity.Tag{}, err
		} else if occupied {
			return coreentity.Tag{}, ErrTagNameAmbiguous
		}
	}
	entityUUID, err := allocateEntityUUID(ctx, tx, input.UUID, portableid.KindTag, now)
	if err != nil {
		return coreentity.Tag{}, err
	}
	timestamp := formatTime(normalisedTime(now))
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tags (uuid, name, normalized_name, sort_name, slug,
			use_in_recommendation, created_at_utc, updated_at_utc)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, entityUUID, normalizedDisplay(input.Name), normalizedKey(input.Name),
		normalizedDisplay(input.SortName), slug.FromName(input.Name, entityUUID),
		input.UseInRecommendation, timestamp, timestamp); err != nil {
		return coreentity.Tag{}, err
	}
	if err := insertAliases(ctx, tx, "tag_aliases", "tag_uuid", entityUUID, input.Aliases); err != nil {
		return coreentity.Tag{}, err
	}
	result, err := findTag(ctx, tx, entityUUID)
	if err != nil {
		return coreentity.Tag{}, err
	}
	if err := tx.Commit(); err != nil {
		return coreentity.Tag{}, err
	}
	return result, nil
}

type UpdateTagInput struct {
	UpdateNamedEntityInput
	UseInRecommendation bool
}

func (s *CoreEntityStore) UpdateTag(ctx context.Context, uuid string, expectedRevision int64, input UpdateTagInput, now time.Time) (coreentity.Tag, error) {
	if err := validateNamedInput(CreateNamedEntityInput{Name: input.Name, SortName: input.SortName, Aliases: input.Aliases}); err != nil {
		return coreentity.Tag{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return coreentity.Tag{}, err
	}
	defer func() { _ = tx.Rollback() }()
	allNames := append([]string{input.Name}, input.Aliases...)
	local := map[string]struct{}{}
	for _, value := range allNames {
		key := normalizedKey(value)
		if _, duplicate := local[key]; duplicate {
			return coreentity.Tag{}, ErrTagNameAmbiguous
		}
		local[key] = struct{}{}
		occupied, err := tagNameOccupiedExcept(ctx, tx, key, uuid)
		if err != nil {
			return coreentity.Tag{}, err
		}
		if occupied {
			return coreentity.Tag{}, ErrTagNameAmbiguous
		}
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE tags SET name = ?, normalized_name = ?, sort_name = ?, use_in_recommendation = ?,
			metadata_revision = metadata_revision + 1, updated_at_utc = ?
		WHERE uuid = ? AND metadata_revision = ?
	`, normalizedDisplay(input.Name), normalizedKey(input.Name), normalizedDisplay(input.SortName),
		input.UseInRecommendation, formatTime(normalisedTime(now)), uuid, expectedRevision)
	if err != nil {
		return coreentity.Tag{}, err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return coreentity.Tag{}, ErrCoreMetadataRevisionConflict
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tag_aliases WHERE tag_uuid = ?`, uuid); err != nil {
		return coreentity.Tag{}, err
	}
	if err := insertAliases(ctx, tx, "tag_aliases", "tag_uuid", uuid, input.Aliases); err != nil {
		return coreentity.Tag{}, err
	}
	if err := markEntityGalleriesManifestDirty(ctx, tx, portableid.KindTag, uuid); err != nil {
		return coreentity.Tag{}, err
	}
	updated, err := findTag(ctx, tx, uuid)
	if err != nil {
		return coreentity.Tag{}, err
	}
	if err := tx.Commit(); err != nil {
		return coreentity.Tag{}, err
	}
	return updated, nil
}

func updateNamedEntity(ctx context.Context, tx *sql.Tx, table, aliasesTable, ownerColumn, uuid string, expectedRevision int64, input UpdateNamedEntityInput, now time.Time, updateNormalizedName bool) error {
	setClause := "name = ?, sort_name = ?, metadata_revision = metadata_revision + 1, updated_at_utc = ?"
	args := []any{normalizedDisplay(input.Name), normalizedDisplay(input.SortName), formatTime(normalisedTime(now)), uuid, expectedRevision}
	if updateNormalizedName {
		setClause = "name = ?, normalized_name = ?, sort_name = ?, metadata_revision = metadata_revision + 1, updated_at_utc = ?"
		args = []any{normalizedDisplay(input.Name), normalizedKey(input.Name), normalizedDisplay(input.SortName), formatTime(normalisedTime(now)), uuid, expectedRevision}
	}
	result, err := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET %s WHERE uuid = ? AND metadata_revision = ?`, table, setClause), args...)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil || rows != 1 {
		return ErrCoreMetadataRevisionConflict
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE %s = ?`, aliasesTable, ownerColumn), uuid); err != nil {
		return err
	}
	if err := insertAliases(ctx, tx, aliasesTable, ownerColumn, uuid, input.Aliases); err != nil {
		return err
	}
	kind := portableid.KindWork
	if table == "characters" {
		kind = portableid.KindCharacter
	}
	return markEntityGalleriesManifestDirty(ctx, tx, kind, uuid)
}

func (s *CoreEntityStore) AddTagParent(
	ctx context.Context,
	childUUID string,
	parentUUID string,
	position int64,
	expectedChildRevision int64,
	expectedParentRevision int64,
	now time.Time,
) error {
	if position <= 0 {
		return errors.New("Tag parent position must be positive")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, check := range []struct {
		uuid     string
		revision int64
	}{{childUUID, expectedChildRevision}, {parentUUID, expectedParentRevision}} {
		result, err := tx.ExecContext(ctx, `
			UPDATE tags SET metadata_revision = metadata_revision + 1, updated_at_utc = ?
			WHERE uuid = ? AND metadata_revision = ?
		`, formatTime(normalisedTime(now)), check.uuid, check.revision)
		if err != nil {
			return err
		}
		if rows, err := result.RowsAffected(); err != nil || rows != 1 {
			return ErrCoreMetadataRevisionConflict
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tag_edges (parent_uuid, child_uuid, position) VALUES (?, ?, ?)
	`, parentUUID, childUUID, position); err != nil {
		return err
	}
	return tx.Commit()
}

func allocateEntityUUID(ctx context.Context, tx *sql.Tx, requested string, kind portableid.Kind, now time.Time) (string, error) {
	if requested == "" {
		requested = portableid.New()
	}
	if _, err := portableid.Parse(requested); err != nil {
		return "", err
	}
	if _, err := registerPortableUUID(ctx, tx, requested, kind, normalisedTime(now)); err != nil {
		return "", err
	}
	return requested, nil
}

func validateNamedInput(input CreateNamedEntityInput) error {
	if normalizedDisplay(input.Name) == "" || runeLength(input.Name) > 300 || runeLength(input.SortName) > 300 {
		return errors.New("entity name must contain 1 to 300 characters")
	}
	if len(input.Aliases) > 100 {
		return errors.New("entity alias count exceeds 100")
	}
	seen := make(map[string]struct{}, len(input.Aliases))
	for _, alias := range input.Aliases {
		key := normalizedKey(alias)
		if key == "" || runeLength(alias) > 300 {
			return errors.New("entity alias must contain 1 to 300 characters")
		}
		if _, duplicate := seen[key]; duplicate {
			return errors.New("entity aliases contain a normalized duplicate")
		}
		seen[key] = struct{}{}
	}
	return nil
}

func insertAliases(ctx context.Context, tx *sql.Tx, table string, ownerColumn string, ownerUUID string, aliases []string) error {
	// table and ownerColumn are internal constants selected by callers above.
	query := fmt.Sprintf(`INSERT INTO %s (%s, alias, normalized_alias, position) VALUES (?, ?, ?, ?)`, table, ownerColumn)
	for index, alias := range aliases {
		if _, err := tx.ExecContext(ctx, query, ownerUUID, normalizedDisplay(alias), normalizedKey(alias), int64(index+1)*1024); err != nil {
			return err
		}
	}
	return nil
}

func tagNameOccupied(ctx context.Context, tx *sql.Tx, key string) (bool, error) {
	var occupied int
	err := tx.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM tags WHERE normalized_name = ?)
			OR EXISTS(SELECT 1 FROM tag_aliases WHERE normalized_alias = ?)
	`, key, key).Scan(&occupied)
	return occupied == 1, err
}

func tagNameOccupiedExcept(ctx context.Context, tx *sql.Tx, key string, uuid string) (bool, error) {
	var occupied int
	err := tx.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM tags WHERE normalized_name = ? AND uuid <> ?)
			OR EXISTS(SELECT 1 FROM tag_aliases WHERE normalized_alias = ? AND tag_uuid <> ?)
	`, key, uuid, key, uuid).Scan(&occupied)
	return occupied == 1, err
}

func normalizedDisplay(value string) string {
	return norm.NFC.String(strings.TrimSpace(value))
}

func normalizedKey(value string) string {
	return cases.Fold().String(normalizedDisplay(value))
}

func runeLength(value string) int {
	return len([]rune(norm.NFC.String(value)))
}
