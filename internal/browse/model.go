// Package browse contains the Gallery-level DTO contract used by the new React
// application. It is path-free except for the authenticated single-owner
// Gallery detail directory summary explicitly exposed by GalleryDetail.
package browse

import (
	"time"

	"github.com/stashapp/stash/internal/gallery"
)

type Scope string

const (
	ScopeList  Scope = "LIST"
	ScopeMagic Scope = "MAGIC"
	ScopeAll   Scope = "ALL"
)

type GallerySort string

const (
	GallerySortRecentlyAdded GallerySort = "RECENTLY_ADDED"
	GallerySortName          GallerySort = "NAME"
	GallerySortShootDate     GallerySort = "SHOOT_DATE"
	GallerySortRating        GallerySort = "RATING"
)

type CollectionType string

const (
	CollectionAlbum   CollectionType = "ALBUM"
	CollectionCosplay CollectionType = "COSPLAY"
)

type EntitySummary struct {
	UUID string
	Name string
}

// PersonSummary augments the path-free entity identity with enough managed
// asset state for the API layer to issue an authenticated avatar URL.
type PersonSummary struct {
	UUID            string
	Name            string
	AvatarAvailable bool
	AssetRevision   int64
}

// ResourceIdentity is sufficient to build an authenticated resource URL but
// cannot reveal a physical source or cache path.
type ResourceIdentity struct {
	ItemUUID        string
	ContentRevision int64
	ProfileHash     string
	Variant         string
	MIMEType        string
}

type Cover struct {
	Kind     gallery.CoverKind
	Revision int64
	Managed  bool
	Resource *ResourceIdentity
	Warning  bool
}

type MediaCounts struct {
	Photo  int
	Selfie int
	GIF    int
	Video  int
}

type GalleryCard struct {
	SetID              string
	Slug               string
	Title              string
	CollectionType     CollectionType
	ContentRating      gallery.ContentRating
	Cover              Cover
	Credits            []PersonSummary
	CreditCount        int
	Characters         []EntitySummary
	CharacterCount     int
	Works              []EntitySummary
	WorkCount          int
	ShootDate          string
	ShootDatePrecision gallery.ShootDatePrecision
	AddedAtUTC         time.Time
	Media              MediaCounts
	Favorite           bool
	RatingHalfSteps    *int
	ScrubberCount      int
	ScrubberRevision   int64
}

type GalleryPage struct {
	Items      []GalleryCard
	Page       int
	PageSize   int
	TotalItems int
	TotalPages int
}

type RecommendationReason string

const (
	ReasonCharacter RecommendationReason = "SHARED_CHARACTER"
	ReasonWork      RecommendationReason = "SHARED_WORK"
	ReasonCoser     RecommendationReason = "SHARED_COSER"
	ReasonType      RecommendationReason = "SAME_COLLECTION_TYPE"
	ReasonTags      RecommendationReason = "TAG_SIMILARITY"
)

type GalleryRecommendation struct {
	Card    GalleryCard
	Score   float64
	Reasons []RecommendationReason
}

type RandomMediaFilter string

const (
	RandomAll    RandomMediaFilter = "ALL"
	RandomPhoto  RandomMediaFilter = "PHOTO"
	RandomSelfie RandomMediaFilter = "SELFIE"
	RandomGIF    RandomMediaFilter = "GIF"
	RandomVideo  RandomMediaFilter = "VIDEO"
)

type RandomMediaItem struct {
	ItemUUID        string
	MediaKind       gallery.MediaKind
	ImageCategory   gallery.ImageCategory
	Resource        ResourceIdentity
	GallerySetID    string
	GallerySlug     string
	Characters      []EntitySummary
	Cosers          []EntitySummary
	Favorite        bool
	RatingHalfSteps *int
}

type GalleryMember struct {
	ItemUUID        string
	MediaKind       gallery.MediaKind
	ContentFormat   gallery.ContentFormat
	ImageCategory   gallery.ImageCategory
	Position        int64
	Caption         string
	ProcessingState gallery.ProcessingState
	CardResource    *ResourceIdentity
	LargeResource   *ResourceIdentity
	Favorite        bool
	RatingHalfSteps *int
}

type GalleryMemberIndex struct {
	SetID            string
	MetadataRevision int64
	ScanRevision     int64
	Items            []GalleryMember
}

type SearchEntityKind string

const (
	SearchGallery   SearchEntityKind = "GALLERY"
	SearchCoser     SearchEntityKind = "COSER"
	SearchWork      SearchEntityKind = "WORK"
	SearchCharacter SearchEntityKind = "CHARACTER"
	SearchTag       SearchEntityKind = "TAG"
)

type SearchHit struct {
	Kind          SearchEntityKind
	UUID          string
	Slug          string
	Name          string
	MatchLevel    int
	CoverResource *ResourceIdentity
}

type SearchPreview struct {
	Scope      Scope
	Query      string
	Galleries  []SearchHit
	Cosers     []SearchHit
	Works      []SearchHit
	Characters []SearchHit
	Tags       []SearchHit
}

type EntitySort string

const (
	EntitySortName          EntitySort = "NAME"
	EntitySortRecentlyAdded EntitySort = "RECENTLY_ADDED"
)

type EntityIndexItem struct {
	Kind            SearchEntityKind
	UUID            string
	Slug            string
	Name            string
	Aliases         []string
	AvatarAvailable bool
	AssetRevision   int64
}

type EntityPage struct {
	Items      []EntityIndexItem
	Page       int
	PageSize   int
	TotalItems int
	TotalPages int
}

type CreditDetail struct {
	Coser      PersonSummary
	Characters []EntitySummary
	Works      []EntitySummary
}

type ExternalLink struct {
	UUID  string
	Type  gallery.ExternalLinkType
	Label string
	URL   string
}

type GalleryDetail struct {
	Card                   GalleryCard
	MetadataRevision       int64
	Description            string
	PhotographerName       string
	StudioName             string
	AvailableBytes         int64
	MediaParentDirectories []string
	Credits                []CreditDetail
	Tags                   []EntitySummary
	ExternalLinks          []ExternalLink
	Redirected             bool
}

type SocialAccount struct {
	UUID        string
	PlatformKey string
	Label       string
	Handle      string
	URL         string
	Status      string
	Position    int64
}

type CoserDetail struct {
	Entity          EntityIndexItem
	ProfileSummary  string
	Biography       string
	CountryOrRegion string
	SocialAccounts  []SocialAccount
	Galleries       GalleryPage
	BannerAvailable bool
	Redirected      bool
}

type WorkDetail struct {
	Entity     EntityIndexItem
	Characters []EntityIndexItem
	Redirected bool
}

type CharacterDetail struct {
	Entity     EntityIndexItem
	Work       EntityIndexItem
	Galleries  GalleryPage
	Redirected bool
}

type TagDetail struct {
	Entity     EntityIndexItem
	Galleries  GalleryPage
	Redirected bool
}

type MediaDetail struct {
	Item             GalleryMember
	DisplayResource  *ResourceIdentity
	Gallery          GalleryCard
	MetadataRevision int64
	VideoTechnical   *VideoTechnicalSummary
}

type MediaInformationEntry struct {
	Key           string
	VisibilityKey string
	Label         string
	Group         string
	Value         string
}

type MediaInformationSummary struct {
	State     string
	ErrorCode string
	Entries   []MediaInformationEntry
}

type VideoTechnicalSummary struct {
	ProbeState      string
	ErrorCode       string
	Container       string
	DurationSeconds float64
	Width           int
	Height          int
	FrameRate       float64
	VideoCodec      string
	AudioCodec      string
}

// OnDemandResource reports the lifecycle of an optional generated Browse
// resource without exposing a source or cache path.
type OnDemandResource struct {
	Status    gallery.ProcessingState
	Resource  *ResourceIdentity
	ErrorCode string
}

type VideoPlaybackStatus struct {
	ItemUUID        string
	Mode            string
	Status          gallery.ProcessingState
	ContentRevision int64
	Resource        *ResourceIdentity
	ErrorCode       string
}

type MediaPage struct {
	Items      []RandomMediaItem
	Page       int
	PageSize   int
	TotalItems int
	TotalPages int
}
