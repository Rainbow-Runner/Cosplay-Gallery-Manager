package manifestidentity

import "sort"

// SourceResolution is the stable result of locating one exported Gallery in a
// complete discovery snapshot. The exported relative source remains audit
// evidence; only ResolvedRelativeSource may be used to rebuild a target.
type SourceResolution string

const (
	ResolutionExact                     SourceResolution = "EXACT"
	ResolutionRelocatedUnique           SourceResolution = "RELOCATED_UNIQUE"
	ResolutionDuplicateAccessibleSource SourceResolution = "DUPLICATE_ACCESSIBLE_SOURCE"
	ResolutionUnresolved                SourceResolution = "UNRESOLVED"
)

type ManifestComparison string

const (
	ManifestExact   ManifestComparison = "EXACT"
	ManifestChanged ManifestComparison = "CHANGED"
)

type ExpectedSource struct {
	SetID                  string
	SourceType             string
	ExportedRelativeSource string
	ManifestSchema         int
	ManifestRevision       int64
	ManifestHash           string
}

// ObservedSource contains only evidence captured by a completed discovery
// snapshot. Invalid Manifests and candidates outside mapped, enabled target
// libraries must be filtered by the caller and never passed here.
type ObservedSource struct {
	SnapshotID     string
	LibraryID      string
	SetID          string
	SourceType     string
	RelativeSource string
	ManifestSchema int
	Revision       int64
	ManifestHash   string
}

type ResolutionResult struct {
	Resolution             SourceResolution
	ManifestComparison     ManifestComparison
	SnapshotID             string
	ResolvedLibraryID      string
	ExportedRelativeSource string
	ResolvedRelativeSource string
	Candidates             []ObservedSource
}

// ResolveSource considers all mapped target libraries as one identity domain.
// A second accessible copy always wins over an exact old path and requires
// review; silently preferring the old path would hide a duplicate identity.
func ResolveSource(expected ExpectedSource, observed []ObservedSource) ResolutionResult {
	matches := make([]ObservedSource, 0)
	for _, candidate := range observed {
		if candidate.SetID == expected.SetID && candidate.SourceType == expected.SourceType {
			matches = append(matches, candidate)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].LibraryID != matches[j].LibraryID {
			return matches[i].LibraryID < matches[j].LibraryID
		}
		return matches[i].RelativeSource < matches[j].RelativeSource
	})
	result := ResolutionResult{Resolution: ResolutionUnresolved, ExportedRelativeSource: expected.ExportedRelativeSource, Candidates: matches}
	if len(matches) == 0 {
		return result
	}
	if len(matches) > 1 {
		result.Resolution = ResolutionDuplicateAccessibleSource
		return result
	}
	candidate := matches[0]
	result.SnapshotID = candidate.SnapshotID
	result.ResolvedLibraryID = candidate.LibraryID
	result.ResolvedRelativeSource = candidate.RelativeSource
	if candidate.RelativeSource == expected.ExportedRelativeSource {
		result.Resolution = ResolutionExact
	} else {
		result.Resolution = ResolutionRelocatedUnique
	}
	if candidate.ManifestSchema == expected.ManifestSchema && candidate.Revision == expected.ManifestRevision && candidate.ManifestHash == expected.ManifestHash {
		result.ManifestComparison = ManifestExact
	} else {
		result.ManifestComparison = ManifestChanged
	}
	return result
}
