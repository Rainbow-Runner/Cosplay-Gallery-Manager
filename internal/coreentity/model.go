package coreentity

import "time"

type Coser struct {
	UUID             string
	Name             string
	SortName         string
	Aliases          []string
	Slug             string
	ProfileSummary   string
	Biography        string
	CountryOrRegion  string
	AvatarPath       string
	BannerPath       string
	AvatarCrop       *AvatarCrop
	BannerFocalPoint *FocalPoint
	MetadataRevision int64
	CreatedAtUTC     time.Time
	UpdatedAtUTC     time.Time
}

type AvatarCrop struct {
	X    float64
	Y    float64
	Size float64
}

type FocalPoint struct {
	X float64
	Y float64
}

type Work struct {
	UUID             string
	Name             string
	SortName         string
	Aliases          []string
	Slug             string
	MetadataRevision int64
	CreatedAtUTC     time.Time
	UpdatedAtUTC     time.Time
}

type Character struct {
	UUID             string
	WorkUUID         string
	Name             string
	SortName         string
	Aliases          []string
	Slug             string
	MetadataRevision int64
	CreatedAtUTC     time.Time
	UpdatedAtUTC     time.Time
}

type Tag struct {
	UUID                  string
	Name                  string
	SortName              string
	Aliases               []string
	Slug                  string
	UseInRecommendation   bool
	AllowDirectAssignment bool
	MetadataRevision      int64
	CreatedAtUTC          time.Time
	UpdatedAtUTC          time.Time
}

type SocialAccount struct {
	UUID        string
	CoserUUID   string
	PlatformKey string
	Label       string
	Handle      string
	URL         string
	Status      string
	Visible     bool
	Position    int64
}
