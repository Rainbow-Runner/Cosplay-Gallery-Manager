package productdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/coreentity"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/portableid"
)

var platformKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func (s *GalleryStore) AddTag(
	ctx context.Context,
	galleryID int64,
	tagUUID string,
	position int64,
	expectedRevision int64,
	now time.Time,
) error {
	if position <= 0 {
		return errors.New("Gallery Tag position must be positive")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := requireActivePortableKind(ctx, tx, tagUUID, portableid.KindTag); err != nil {
		return err
	}
	if err := requireDirectAssignableTag(ctx, tx, tagUUID); err != nil {
		return err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM gallery_tags WHERE gallery_id = ?`, galleryID).Scan(&count); err != nil {
		return err
	}
	if count >= 200 {
		return errors.New("Gallery direct Tag limit exceeds 200")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO gallery_tags (gallery_id, tag_uuid, position) VALUES (?, ?, ?)
	`, galleryID, tagUUID, position); err != nil {
		return err
	}
	if err := touchGalleryMetadata(ctx, tx, galleryID, expectedRevision, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *GalleryStore) AddExternalLink(
	ctx context.Context,
	galleryID int64,
	linkType gallery.ExternalLinkType,
	label string,
	rawURL string,
	position int64,
	expectedRevision int64,
	now time.Time,
) (gallery.ExternalLink, error) {
	if linkType != gallery.ExternalLinkSource && linkType != gallery.ExternalLinkProfile && linkType != gallery.ExternalLinkReference {
		return gallery.ExternalLink{}, errors.New("unsupported GalleryExternalLink type")
	}
	if runeLength(label) > 100 || runeLength(rawURL) > 2048 || position <= 0 {
		return gallery.ExternalLink{}, errors.New("invalid GalleryExternalLink field")
	}
	normalizedURL, err := validateExternalHTTPURL(rawURL)
	if err != nil {
		return gallery.ExternalLink{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return gallery.ExternalLink{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM gallery_external_links WHERE gallery_id = ?`, galleryID).Scan(&count); err != nil {
		return gallery.ExternalLink{}, err
	}
	if count >= 50 {
		return gallery.ExternalLink{}, errors.New("Gallery ExternalLink limit exceeds 50")
	}
	linkUUID, err := allocateEntityUUID(ctx, tx, "", portableid.KindExternalLink, now)
	if err != nil {
		return gallery.ExternalLink{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO gallery_external_links (link_uuid, gallery_id, link_type, label, url, position)
		VALUES (?, ?, ?, ?, ?, ?)
	`, linkUUID, galleryID, linkType, normalizedDisplay(label), normalizedURL, position); err != nil {
		return gallery.ExternalLink{}, err
	}
	if err := touchGalleryMetadata(ctx, tx, galleryID, expectedRevision, now); err != nil {
		return gallery.ExternalLink{}, err
	}
	if err := tx.Commit(); err != nil {
		return gallery.ExternalLink{}, err
	}
	return gallery.ExternalLink{
		UUID: linkUUID, GalleryID: galleryID, Type: linkType,
		Label: normalizedDisplay(label), URL: normalizedURL, Position: position,
	}, nil
}

func (s *GalleryStore) UpdateItemMetadata(
	ctx context.Context,
	itemID int64,
	category gallery.ImageCategory,
	caption string,
	expectedRevision int64,
	now time.Time,
) (gallery.Item, error) {
	if runeLength(caption) > 1000 {
		return gallery.Item{}, errors.New("GalleryItem Caption exceeds 1000 characters")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return gallery.Item{}, err
	}
	defer func() { _ = tx.Rollback() }()
	item, err := findItem(ctx, tx, itemID)
	if err != nil {
		return gallery.Item{}, err
	}
	if item.MediaKind == gallery.MediaKindStaticImage {
		if category != gallery.ImageCategoryPhoto && category != gallery.ImageCategorySelfie {
			return gallery.Item{}, errors.New("static GalleryItem requires PHOTO or SELFIE")
		}
	} else if category != "" {
		return gallery.Item{}, errors.New("animated and video GalleryItems cannot have an image category")
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE gallery_items SET image_category = NULLIF(?, ''), caption = ?, updated_at_utc = ?
		WHERE id = ?
	`, category, normalizedDisplay(caption), formatTime(normalisedTime(now)), itemID); err != nil {
		return gallery.Item{}, err
	}
	if err := touchGalleryMetadata(ctx, tx, item.GalleryID, expectedRevision, now); err != nil {
		return gallery.Item{}, err
	}
	updated, err := findItem(ctx, tx, itemID)
	if err != nil {
		return gallery.Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return gallery.Item{}, err
	}
	return updated, nil
}

func (s *CoreEntityStore) AddSocialAccount(
	ctx context.Context,
	coserUUID string,
	platformKey string,
	label string,
	handle string,
	rawURL string,
	status string,
	visible bool,
	position int64,
	expectedRevision int64,
	now time.Time,
) (coreentity.SocialAccount, error) {
	if !platformKeyPattern.MatchString(platformKey) || runeLength(label) > 100 || runeLength(handle) > 200 || position <= 0 {
		return coreentity.SocialAccount{}, errors.New("invalid SocialAccount field")
	}
	if status != "ACTIVE" && status != "INACTIVE" {
		return coreentity.SocialAccount{}, errors.New("SocialAccount status must be ACTIVE or INACTIVE")
	}
	normalizedURL, err := validateExternalHTTPURL(rawURL)
	if err != nil {
		return coreentity.SocialAccount{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return coreentity.SocialAccount{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM coser_social_accounts WHERE coser_uuid = ?`, coserUUID).Scan(&count); err != nil {
		return coreentity.SocialAccount{}, err
	}
	if count >= 50 {
		return coreentity.SocialAccount{}, errors.New("Coser SocialAccount limit exceeds 50")
	}
	accountUUID, err := allocateEntityUUID(ctx, tx, "", portableid.KindSocialAccount, now)
	if err != nil {
		return coreentity.SocialAccount{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO coser_social_accounts (
			account_uuid, coser_uuid, platform_key, label, handle, url, status, visible, position
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, accountUUID, coserUUID, platformKey, normalizedDisplay(label), normalizedDisplay(handle), normalizedURL, status, visible, position); err != nil {
		return coreentity.SocialAccount{}, err
	}
	if err := touchCoreEntity(ctx, tx, "cosers", coserUUID, expectedRevision, now); err != nil {
		return coreentity.SocialAccount{}, err
	}
	if err := tx.Commit(); err != nil {
		return coreentity.SocialAccount{}, err
	}
	return coreentity.SocialAccount{
		UUID: accountUUID, CoserUUID: coserUUID, PlatformKey: platformKey, Label: normalizedDisplay(label),
		Handle: normalizedDisplay(handle), URL: normalizedURL, Status: status,
		Visible: visible, Position: position,
	}, nil
}

type SocialAccountInput struct {
	PlatformKey string
	Label       string
	Handle      string
	URL         string
	Status      string
	Visible     bool
}

func validateExternalHTTPURL(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", errors.New("URL must be an absolute HTTP(S) URL without user information")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	if parsed.Host == "" || parsed.User != nil ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("URL must be an absolute HTTP(S) URL without user information")
	}
	normalized := parsed.String()
	if runeLength(normalized) > 2048 {
		return "", errors.New("URL exceeds 2048 characters")
	}
	return normalized, nil
}

func touchCoreEntity(ctx context.Context, tx *sql.Tx, table string, uuid string, expectedRevision int64, now time.Time) error {
	allowed := map[string]bool{"cosers": true, "works": true, "characters": true, "tags": true}
	if !allowed[table] {
		return fmt.Errorf("unsupported core entity table %q", table)
	}
	result, err := tx.ExecContext(ctx, `UPDATE `+table+`
		SET metadata_revision = metadata_revision + 1, updated_at_utc = ?
		WHERE uuid = ? AND metadata_revision = ?`,
		formatTime(normalisedTime(now)), uuid, expectedRevision)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrCoreMetadataRevisionConflict
	}
	if table == "cosers" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE coser_manifest_sync SET status = CASE
				WHEN status = 'CLEAN' THEN 'DB_DIRTY' ELSE status END
			WHERE coser_uuid = ?
		`, uuid); err != nil {
			return err
		}
	}
	return nil
}
