// Package library defines media-library roots and deterministic path ownership.
package library

import "time"

type Library struct {
	ID              int64
	Name            string
	RootPath        string
	Enabled         bool
	ReadOnly        bool
	CaptureTimezone string
	CreatedAtUTC    time.Time
	UpdatedAtUTC    time.Time
}

type SourceImpact struct {
	SourceID       int64
	GalleryID      int64
	SourcePath     string
	CurrentLibrary int64
	SuggestedOwner *int64
}

type ChangePreview struct {
	LibraryID int64
	NewRoot   string
	Impacts   []SourceImpact
}
