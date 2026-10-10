package manifestidentity

import "testing"

func TestResolveSourceFindsExactAndRelocatedManifest(t *testing.T) {
	expected := ExpectedSource{SetID: galleryUUID, SourceType: "DIRECTORY", ExportedRelativeSource: "old/set", ManifestSchema: 1, ManifestRevision: 3, ManifestHash: "sha256:abc"}
	exact := ResolveSource(expected, []ObservedSource{{SnapshotID: "snapshot-1", LibraryID: "library-1", SetID: galleryUUID, SourceType: "DIRECTORY", RelativeSource: "old/set", ManifestSchema: 1, Revision: 3, ManifestHash: "sha256:abc"}})
	if exact.Resolution != ResolutionExact || exact.ManifestComparison != ManifestExact {
		t.Fatalf("exact result = %#v", exact)
	}
	relocated := ResolveSource(expected, []ObservedSource{{SnapshotID: "snapshot-2", LibraryID: "library-1", SetID: galleryUUID, SourceType: "DIRECTORY", RelativeSource: "imported/set", ManifestSchema: 1, Revision: 3, ManifestHash: "sha256:abc"}})
	if relocated.Resolution != ResolutionRelocatedUnique || relocated.ResolvedRelativeSource != "imported/set" || relocated.ExportedRelativeSource != "old/set" {
		t.Fatalf("relocated result = %#v", relocated)
	}
}

func TestResolveSourceDoesNotHideDuplicateAtExportedPath(t *testing.T) {
	expected := ExpectedSource{SetID: galleryUUID, SourceType: "DIRECTORY", ExportedRelativeSource: "old/set"}
	result := ResolveSource(expected, []ObservedSource{
		{SnapshotID: "snapshot-1", LibraryID: "library-1", SetID: galleryUUID, SourceType: "DIRECTORY", RelativeSource: "old/set"},
		{SnapshotID: "snapshot-1", LibraryID: "library-2", SetID: galleryUUID, SourceType: "DIRECTORY", RelativeSource: "copy/set"},
	})
	if result.Resolution != ResolutionDuplicateAccessibleSource || len(result.Candidates) != 2 || result.ResolvedRelativeSource != "" {
		t.Fatalf("duplicate result = %#v", result)
	}
}

func TestResolveSourceRequiresSeparateAdoptionWhenManifestChanged(t *testing.T) {
	expected := ExpectedSource{SetID: galleryUUID, SourceType: "ARCHIVE", ExportedRelativeSource: "set.zip", ManifestSchema: 1, ManifestRevision: 2, ManifestHash: "sha256:old"}
	result := ResolveSource(expected, []ObservedSource{{SnapshotID: "snapshot-1", LibraryID: "library-1", SetID: galleryUUID, SourceType: "ARCHIVE", RelativeSource: "moved/set.zip", ManifestSchema: 1, Revision: 3, ManifestHash: "sha256:new"}})
	if result.Resolution != ResolutionRelocatedUnique || result.ManifestComparison != ManifestChanged {
		t.Fatalf("changed result = %#v", result)
	}
}

func TestResolveSourceRejectsSourceTypeConversion(t *testing.T) {
	expected := ExpectedSource{SetID: galleryUUID, SourceType: "DIRECTORY", ExportedRelativeSource: "set"}
	result := ResolveSource(expected, []ObservedSource{{SnapshotID: "snapshot-1", LibraryID: "library-1", SetID: galleryUUID, SourceType: "ARCHIVE", RelativeSource: "set.zip"}})
	if result.Resolution != ResolutionUnresolved || len(result.Candidates) != 0 {
		t.Fatalf("type-change result = %#v", result)
	}
}
