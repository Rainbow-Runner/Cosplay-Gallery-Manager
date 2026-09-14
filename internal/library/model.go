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
	GalleryTitle   string
	SourcePath     string
	CurrentLibrary int64
	SuggestedOwner *int64
}

type ChangePreview struct {
	LibraryID                 int64
	CurrentRoot               string
	NewRoot                   string
	RevisionToken             string
	IgnoredSourceCount        int
	IgnoredSources            []IgnoredSourceImpact
	UnassignedSourcePaths     []string
	RecognitionRules          []RuleImpact
	ClassificationRules       []RuleImpact
	ExclusionRules            []RuleImpact
	AutomationMode            string
	AutomationPolicyRevision  int64
	AutomationRunCount        int
	ActiveRunCount            int
	PortableMappingCount      int
	ScanningSourceCount       int
	ChildRoots                []string
	ProposedBoundaryConflicts []string
	Impacts                   []SourceImpact
}

type IgnoredSourceImpact struct {
	ID     int64
	Path   string
	Reason string
}
type RuleImpact struct {
	ID       int64
	Name     string
	Revision string
}
