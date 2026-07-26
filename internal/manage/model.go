package manage

import "github.com/stashapp/stash/internal/gallery"

type IssueSummary struct {
	Draft       int
	OverLimit   int
	Unavailable int
	Blocking    int
	MissingItem int
}

type GalleryRow struct {
	SetID              string
	Slug               string
	State              gallery.State
	Title              string
	ContentRating      gallery.ContentRating
	MetadataRevision   int64
	ScanRevision       int64
	Browsable          bool
	SourceType         gallery.SourceType
	SourcePath         string
	SourceAvailability gallery.AvailabilityState
	ReconcileState     gallery.ReconcileState
	OverLimit          bool
	ItemCount          int
	MissingCount       int
	PendingCount       int
	ErrorCount         int
	BlockingIssues     int
}

type GalleryPage struct {
	Items      []GalleryRow
	Summary    IssueSummary
	Page       int
	PageSize   int
	TotalItems int
	TotalPages int
}

type GalleryItem struct {
	UUID            string
	RelativePath    string
	MediaKind       gallery.MediaKind
	ContentFormat   gallery.ContentFormat
	ImageCategory   gallery.ImageCategory
	Position        int64
	Caption         string
	Excluded        bool
	Availability    gallery.AvailabilityState
	ProcessingState gallery.ProcessingState
	ByteSize        int64
}

type GalleryDetail struct {
	Row                GalleryRow
	Aliases            []string
	Description        string
	ShootDate          string
	ShootDatePrecision gallery.ShootDatePrecision
	PhotographerName   string
	StudioName         string
	Items              []GalleryItem
	Credits            []GalleryCredit
	Tags               []GalleryTag
	ExternalLinks      []GalleryExternalLink
}

type GalleryCast struct {
	CharacterUUID, CharacterName, WorkUUID, WorkName string
	Position                                         int64
}
type GalleryCredit struct {
	CoserUUID, CoserName string
	Position             int64
	Cast                 []GalleryCast
}
type GalleryTag struct {
	UUID, Name string
	Position   int64
}
type GalleryExternalLink struct {
	UUID, Type, Label, URL string
	Position               int64
}
