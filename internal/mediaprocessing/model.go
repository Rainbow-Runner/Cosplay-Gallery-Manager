package mediaprocessing

import "time"

type JobKind string

const (
	JobLibraryScan           JobKind = "LIBRARY_SCAN"
	JobGalleryProcessing     JobKind = "GALLERY_PROCESSING"
	JobItemDerivative        JobKind = "ITEM_DERIVATIVE"
	JobItemTechnicalMetadata JobKind = "ITEM_TECHNICAL_METADATA"
	JobManifest              JobKind = "MANIFEST"
	JobCache                 JobKind = "CACHE"
	JobBackup                JobKind = "BACKUP"
)

// RequiredCacheTier is a product invariant: resources needed for a Gallery to
// remain browsable survive LRU cleanup, while richer optional resources may be
// regenerated. Callers cannot downgrade a BASE variant into the evictable
// tier through job payloads.
func RequiredCacheTier(variant string) (CacheTier, bool) {
	switch variant {
	case VariantCard480, VariantStaticPoster:
		return CacheBase, true
	case VariantCard960, VariantCard1600, VariantLightbox4096, VariantAnimatedPreview, VariantVideoPlayback:
		return CacheEnhanced, true
	default:
		return "", false
	}
}

type VideoProbeState string

const (
	VideoProbePending VideoProbeState = "PENDING"
	VideoProbeReady   VideoProbeState = "READY"
	VideoProbeError   VideoProbeState = "ERROR"
)

type VideoTechnicalMetadata struct {
	ItemUUID         string
	ContentRevision  int64
	ProbeProfileHash string
	ProbeState       VideoProbeState
	LastErrorCode    string
	Container        string
	DurationSeconds  float64
	StartTimeSeconds float64
	TotalBitrate     int64
	VideoBitrate     int64
	VideoStreamIndex int
	VideoCodec       string
	VideoProfile     string
	PixelFormat      string
	CodedWidth       int
	CodedHeight      int
	DisplayWidth     int
	DisplayHeight    int
	FrameRate        float64
	Rotation         int
	ColorRange       string
	ColorSpace       string
	ColorPrimaries   string
	ColorTransfer    string
	HDR              bool
	AudioStreamIndex *int
	AudioCodec       string
	AudioChannels    int
	AudioSampleRate  int
	FFprobeVersion   string
	CompletedAtUTC   *time.Time
}

type JobStatus string

const (
	JobPending   JobStatus = "PENDING"
	JobRunning   JobStatus = "RUNNING"
	JobRetryWait JobStatus = "RETRY_WAIT"
	JobPaused    JobStatus = "PAUSED"
	JobCompleted JobStatus = "COMPLETED"
	JobFailed    JobStatus = "FAILED"
	JobCancelled JobStatus = "CANCELLED"
)

type CacheTier string

const (
	CacheBase     CacheTier = "BASE"
	CacheEnhanced CacheTier = "ENHANCED"
)

type DerivativeState string

const (
	DerivativeReady       DerivativeState = "READY"
	DerivativeStale       DerivativeState = "STALE"
	DerivativeHardInvalid DerivativeState = "HARD_INVALID"
)

const (
	VariantCard480         = "CARD_480"
	VariantCard960         = "CARD_960"
	VariantCard1600        = "CARD_1600"
	VariantLightbox4096    = "LIGHTBOX_4096"
	VariantStaticPoster    = "STATIC_POSTER"
	VariantAnimatedPreview = "ANIMATED_PREVIEW"
	VariantVideoPlayback   = "VIDEO_PLAYBACK"
)

type Job struct {
	ID                int64
	Key               string
	Kind              JobKind
	GalleryID         *int64
	ItemUUID          string
	Variant           string
	ContentRevision   *int64
	ProfileHash       string
	PayloadJSON       []byte
	Status            JobStatus
	Priority          int
	AttemptCount      int
	MaxAttempts       int
	NotBeforeUTC      time.Time
	LeaseOwner        string
	LeaseExpiresUTC   *time.Time
	LastErrorCode     string
	StructuralFailure bool
	CreatedAtUTC      time.Time
	UpdatedAtUTC      time.Time
}

type Derivative struct {
	ID                int64
	ItemUUID          string
	Variant           string
	CacheTier         CacheTier
	ContentRevision   int64
	ProfileHash       string
	State             DerivativeState
	Current           bool
	CacheRelativePath string
	MIMEType          string
	ByteSize          int64
	Width             int
	Height            int
	CreatedAtUTC      time.Time
	LastAccessedUTC   time.Time
}
