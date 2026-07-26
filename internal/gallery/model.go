// Package gallery defines the Gallery aggregate without depending on the
// original Stash Gallery/Image/Scene business models.
package gallery

import "time"

type State string

const (
	StateDraft    State = "DRAFT"
	StateActive   State = "ACTIVE"
	StateArchived State = "ARCHIVED"
)

type ContentRating string

const (
	ContentRatingNonAdult ContentRating = "NON_ADULT"
	ContentRatingAdult    ContentRating = "ADULT"
)

type ShootDatePrecision string

const (
	ShootDatePrecisionMonth ShootDatePrecision = "MONTH"
	ShootDatePrecisionDay   ShootDatePrecision = "DAY"
)

type SourceType string

const (
	SourceTypeDirectory SourceType = "DIRECTORY"
	SourceTypeArchive   SourceType = "ARCHIVE"
)

type AvailabilityState string

const (
	AvailabilityAvailable  AvailabilityState = "AVAILABLE"
	AvailabilityMissing    AvailabilityState = "MISSING"
	AvailabilityUnreadable AvailabilityState = "UNREADABLE"
)

type ReconcileState string

const (
	ReconcileNeverScanned ReconcileState = "NEVER_SCANNED"
	ReconcileScanning     ReconcileState = "SCANNING"
	ReconcileInSync       ReconcileState = "IN_SYNC"
	ReconcileNeedsRescan  ReconcileState = "NEEDS_RESCAN"
	ReconcileError        ReconcileState = "ERROR"
)

type IssueSeverity string

const (
	IssueSeverityInfo     IssueSeverity = "INFO"
	IssueSeverityWarning  IssueSeverity = "WARNING"
	IssueSeverityBlocking IssueSeverity = "BLOCKING"
)

type MediaKind string

const (
	MediaKindStaticImage   MediaKind = "STATIC_IMAGE"
	MediaKindAnimatedImage MediaKind = "ANIMATED_IMAGE"
	MediaKindVideo         MediaKind = "VIDEO"
)

// ContentFormat records the actual-content family independently from the
// presentation MediaKind. RAW remains a STATIC_IMAGE but always requires a
// decoded proxy.
type ContentFormat string

const (
	ContentFormatImage ContentFormat = "IMAGE"
	ContentFormatRAW   ContentFormat = "RAW"
	ContentFormatVideo ContentFormat = "VIDEO"
)

type ImageCategory string

const (
	ImageCategoryPhoto  ImageCategory = "PHOTO"
	ImageCategorySelfie ImageCategory = "SELFIE"
)

type ProcessingState string

const (
	ProcessingPending    ProcessingState = "PENDING"
	ProcessingProcessing ProcessingState = "PROCESSING"
	ProcessingReady      ProcessingState = "READY"
	ProcessingError      ProcessingState = "ERROR"
)

type Gallery struct {
	ID                 int64
	SetID              string
	Slug               string
	State              State
	Title              string
	Aliases            []string
	Description        string
	ShootDate          string
	ShootDatePrecision ShootDatePrecision
	ContentRating      ContentRating
	PhotographerName   string
	StudioName         string
	CreatedAtUTC       time.Time
	UpdatedAtUTC       time.Time
	AddedAtUTC         *time.Time
	MetadataRevision   int64
	ScanRevision       int64
	ScrubberRevision   int64
	Browsable          bool
}

type Source struct {
	ID             int64
	GalleryID      int64
	LibraryID      *int64
	Type           SourceType
	Path           string
	Availability   AvailabilityState
	ReconcileState ReconcileState
	OverLimit      bool
	CreatedAtUTC   time.Time
	UpdatedAtUTC   time.Time
}

type Item struct {
	ID               int64
	UUID             string
	GalleryID        int64
	SourceID         int64
	RelativePath     string
	MediaKind        MediaKind
	ContentFormat    ContentFormat
	ImageCategory    ImageCategory
	Position         int64
	Caption          string
	Excluded         bool
	Availability     AvailabilityState
	ProcessingState  ProcessingState
	ByteSize         int64
	QuickFingerprint string
	FullFingerprint  string
	ContentRevision  int64
	CreatedAtUTC     time.Time
	UpdatedAtUTC     time.Time
}

type ExternalLinkType string

const (
	ExternalLinkSource    ExternalLinkType = "SOURCE"
	ExternalLinkProfile   ExternalLinkType = "PROFILE"
	ExternalLinkReference ExternalLinkType = "REFERENCE"
)

type ExternalLink struct {
	UUID      string
	GalleryID int64
	Type      ExternalLinkType
	Label     string
	URL       string
	Position  int64
}

type CoverKind string

const (
	CoverNone        CoverKind = "NONE"
	CoverAutoRandom  CoverKind = "AUTO_RANDOM"
	CoverItem        CoverKind = "ITEM"
	CoverManaged     CoverKind = "MANAGED"
	CoverVideoPoster CoverKind = "VIDEO_POSTER"
)

type CoverState struct {
	GalleryID         int64
	PreferredKind     CoverKind
	PreferredItemUUID string
	PreferredPath     string
	FallbackItemUUID  string
	EffectiveKind     CoverKind
	EffectiveItemUUID string
	EffectivePath     string
	WarningCode       string
	Revision          int64
	CanUndo           bool
	UpdatedAtUTC      time.Time
}

type ActivationFacts struct {
	HasSource                     bool
	SourceAvailable               bool
	SourceOverLimit               bool
	BlockingIssueCount            int
	DisplayableItemCount          int
	CreditCount                   int
	CastCount                     int
	CreditsWithoutCharacterCount  int
	UnresolvedIdentitySuggestions int
}

type ActivationBlocker struct {
	Code    string
	Message string
}

func (g Gallery) ActivationBlockers(facts ActivationFacts) []ActivationBlocker {
	var blockers []ActivationBlocker
	add := func(code string, message string) {
		blockers = append(blockers, ActivationBlocker{Code: code, Message: message})
	}

	if !facts.HasSource {
		add("SOURCE_REQUIRED", "Gallery must have exactly one physical source")
	} else if !facts.SourceAvailable {
		add("SOURCE_UNAVAILABLE", "Gallery source must be available")
	}
	if facts.SourceOverLimit {
		add("SOURCE_OVER_LIMIT", "Gallery source exceeds the 1000 member hard limit")
	}
	if facts.BlockingIssueCount > 0 {
		add("BLOCKING_SOURCE_ISSUE", "Gallery source has unresolved blocking issues")
	}
	if facts.DisplayableItemCount == 0 {
		add("DISPLAYABLE_ITEM_REQUIRED", "Gallery must have at least one displayable member")
	}
	if g.Title == "" {
		add("TITLE_REQUIRED", "Gallery title is required")
	}
	if g.ContentRating == "" {
		add("CONTENT_RATING_REQUIRED", "Gallery content rating is required")
	}
	if facts.CreditCount == 0 {
		add("CREDIT_REQUIRED", "Gallery must have at least one Coser credit")
	}
	if facts.CastCount > 0 && facts.CreditsWithoutCharacterCount > 0 {
		add("CAST_REQUIRED_FOR_EACH_CREDIT", "Every Cosplay credit must have at least one Character")
	}
	if facts.UnresolvedIdentitySuggestions > 0 {
		add("IDENTITY_SUGGESTION_UNRESOLVED", "Coser, Work or Character suggestions must be resolved")
	}

	return blockers
}

func DeriveBrowsable(state State, facts ActivationFacts) bool {
	return state == StateActive && facts.HasSource && facts.SourceAvailable &&
		!facts.SourceOverLimit && facts.BlockingIssueCount == 0 &&
		facts.DisplayableItemCount > 0
}
