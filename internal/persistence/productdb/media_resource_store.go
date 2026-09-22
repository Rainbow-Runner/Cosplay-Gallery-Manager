package productdb

import (
	"context"
	"database/sql"
	"errors"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

var (
	ErrMediaResourceForbidden   = errors.New("media resource is not visible in the requested access scope")
	ErrScrubberRevisionConflict = errors.New("Gallery Scrubber revision has changed")
)

type ResourceAccessMode string

const (
	ResourceBrowse ResourceAccessMode = "BROWSE"
	ResourceManage ResourceAccessMode = "MANAGE"
)

type ResourceScope string

const (
	ResourceScopeList  ResourceScope = "LIST"
	ResourceScopeMagic ResourceScope = "MAGIC"
	ResourceScopeAll   ResourceScope = "ALL"
)

type ResourceAccess struct {
	Authenticated bool
	Mode          ResourceAccessMode
	Scope         ResourceScope
}

// MediaResourceDescriptor contains only opaque identity plus an
// application-cache-relative path. It never contains a user media path.
type MediaResourceDescriptor struct {
	ItemUUID          string
	GallerySetID      string
	Variant           string
	ContentRevision   int64
	ProfileHash       string
	MIMEType          string
	ByteSize          int64
	CacheRelativePath string
	State             mediaprocessing.DerivativeState
}

type MediaResourceStore struct{ db *sql.DB }

func (db *Database) MediaResources() *MediaResourceStore { return &MediaResourceStore{db: db.DB} }

func (s *MediaResourceStore) AuthorizeDerivative(ctx context.Context, access ResourceAccess, itemUUID, variant string, contentRevision int64, profileHash string) (MediaResourceDescriptor, error) {
	if !access.Authenticated {
		return MediaResourceDescriptor{}, ErrMediaResourceForbidden
	}
	if access.Mode != ResourceBrowse && access.Mode != ResourceManage {
		return MediaResourceDescriptor{}, ErrMediaResourceForbidden
	}
	var result MediaResourceDescriptor
	var galleryState gallery.State
	var contentRating sql.NullString
	var excluded, overLimit, hidden int
	var sourceAvailability, itemAvailability string
	var blocking int
	err := s.db.QueryRowContext(ctx, `SELECT item.item_uuid,gallery.set_id,derivative.variant,derivative.content_revision,
		derivative.profile_hash,derivative.mime_type,derivative.byte_size,derivative.cache_relative_path,derivative.state,
		gallery.state,gallery.content_rating,item.excluded,source.over_limit,COALESCE(personal.hidden,0),source.availability_state,
		item.availability_state,(SELECT COUNT(*) FROM gallery_source_issues issue WHERE issue.source_id=source.id AND issue.severity='BLOCKING' AND issue.resolved_at_utc IS NULL)
		FROM media_derivatives derivative JOIN gallery_items item ON item.item_uuid=derivative.item_uuid
		JOIN galleries gallery ON gallery.id=item.gallery_id JOIN gallery_sources source ON source.id=item.source_id
		LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id
		WHERE derivative.item_uuid=? AND derivative.variant=? AND derivative.content_revision=? AND derivative.profile_hash=?`,
		itemUUID, variant, contentRevision, profileHash).Scan(&result.ItemUUID, &result.GallerySetID, &result.Variant, &result.ContentRevision,
		&result.ProfileHash, &result.MIMEType, &result.ByteSize, &result.CacheRelativePath, &result.State, &galleryState, &contentRating,
		&excluded, &overLimit, &hidden, &sourceAvailability, &itemAvailability, &blocking)
	if errors.Is(err, sql.ErrNoRows) {
		return MediaResourceDescriptor{}, ErrMediaResourceForbidden
	}
	if err != nil {
		return MediaResourceDescriptor{}, err
	}
	if result.State == mediaprocessing.DerivativeHardInvalid {
		return MediaResourceDescriptor{}, ErrMediaResourceForbidden
	}
	if access.Mode == ResourceManage {
		return result, nil
	}
	if galleryState != gallery.StateActive || sourceAvailability != string(gallery.AvailabilityAvailable) || itemAvailability != string(gallery.AvailabilityAvailable) || excluded == 1 || overLimit == 1 || hidden == 1 || blocking > 0 {
		return MediaResourceDescriptor{}, ErrMediaResourceForbidden
	}
	switch access.Scope {
	case ResourceScopeList:
		if contentRating.String != string(gallery.ContentRatingNonAdult) {
			return MediaResourceDescriptor{}, ErrMediaResourceForbidden
		}
	case ResourceScopeMagic:
		if contentRating.String != string(gallery.ContentRatingAdult) {
			return MediaResourceDescriptor{}, ErrMediaResourceForbidden
		}
	case ResourceScopeAll:
	default:
		return MediaResourceDescriptor{}, ErrMediaResourceForbidden
	}
	return result, nil
}

// AuthorizeGalleryScrubber resolves an ordinal from the complete safe preview
// index. It never accepts OFFSET or a physical path, and a stale immutable
// revision cannot silently resolve to a different member.
func (s *MediaResourceStore) AuthorizeGalleryScrubber(ctx context.Context, access ResourceAccess, setID string, revision int64, ordinal int) (MediaResourceDescriptor, error) {
	if !access.Authenticated || access.Mode != ResourceBrowse || revision < 0 || ordinal < 0 {
		return MediaResourceDescriptor{}, ErrMediaResourceForbidden
	}
	var galleryID int64
	var currentRevision int64
	var contentRating sql.NullString
	var state gallery.State
	var sourceAvailability string
	var overLimit, hidden, blocking int
	err := s.db.QueryRowContext(ctx, `SELECT gallery.id,gallery.scrubber_revision,gallery.content_rating,gallery.state,
		source.availability_state,source.over_limit,COALESCE(personal.hidden,0),
		(SELECT COUNT(*) FROM gallery_source_issues issue WHERE issue.source_id=source.id AND issue.severity='BLOCKING' AND issue.resolved_at_utc IS NULL)
		FROM galleries gallery JOIN gallery_sources source ON source.gallery_id=gallery.id
		LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id WHERE gallery.set_id=?`, setID).Scan(
		&galleryID, &currentRevision, &contentRating, &state, &sourceAvailability, &overLimit, &hidden, &blocking)
	if errors.Is(err, sql.ErrNoRows) {
		return MediaResourceDescriptor{}, ErrMediaResourceForbidden
	}
	if err != nil {
		return MediaResourceDescriptor{}, err
	}
	if state != gallery.StateActive || sourceAvailability != string(gallery.AvailabilityAvailable) || overLimit == 1 || hidden == 1 || blocking > 0 || !resourceRatingInScope(contentRating.String, access.Scope) {
		return MediaResourceDescriptor{}, ErrMediaResourceForbidden
	}
	if currentRevision != revision {
		return MediaResourceDescriptor{}, ErrScrubberRevisionConflict
	}
	index, err := (&DerivativeStore{db: s.db}).GalleryScrubberIndex(ctx, galleryID)
	if err != nil {
		return MediaResourceDescriptor{}, err
	}
	if ordinal >= len(index) {
		return MediaResourceDescriptor{}, ErrMediaResourceForbidden
	}
	entry := index[ordinal]
	return s.AuthorizeDerivative(ctx, access, entry.ItemUUID, entry.Variant, entry.ContentRevision, entry.ProfileHash)
}

func resourceRatingInScope(rating string, scope ResourceScope) bool {
	switch scope {
	case ResourceScopeList:
		return rating == string(gallery.ContentRatingNonAdult)
	case ResourceScopeMagic:
		return rating == string(gallery.ContentRatingAdult)
	case ResourceScopeAll:
		return true
	default:
		return false
	}
}
