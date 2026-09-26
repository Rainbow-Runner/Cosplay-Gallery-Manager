package manifestidentity

import (
	"testing"

	"github.com/stashapp/stash/internal/manifest"
	"github.com/stashapp/stash/internal/portableid"
)

const (
	galleryUUID   = "11111111-1111-4111-8111-111111111111"
	itemUUID      = "22222222-2222-4222-8222-222222222222"
	linkUUID      = "33333333-3333-4333-8333-333333333333"
	coserUUID     = "44444444-4444-4444-8444-444444444444"
	workUUID      = "55555555-5555-4555-8555-555555555555"
	characterUUID = "66666666-6666-4666-8666-666666666666"
	tagUUID       = "77777777-7777-4777-8777-777777777777"
)

func TestExtractNormalizesOwnedAndCoreIdentities(t *testing.T) {
	document := identityFixture()
	declaration := Extract(document)
	if len(declaration.Identities) != 3 {
		t.Fatalf("owned identities = %d, want 3", len(declaration.Identities))
	}
	if len(declaration.CoreReferences) != 4 {
		t.Fatalf("core references = %d, want 4", len(declaration.CoreReferences))
	}
	if len(declaration.InternalIssues) != 0 {
		t.Fatalf("internal issues = %#v", declaration.InternalIssues)
	}
}

func TestExtractDetectsCrossKindReuseInsideManifest(t *testing.T) {
	document := identityFixture()
	document.Items.Value[0].ItemUUID = galleryUUID
	declaration := Extract(document)
	if len(declaration.InternalIssues) != 1 || declaration.InternalIssues[0].Code != ClassUUIDKindConflict {
		t.Fatalf("internal issues = %#v", declaration.InternalIssues)
	}
}

func TestInspectClassifiesUnclaimedManifest(t *testing.T) {
	compatible := true
	occupancies := map[string]Occupancy{}
	for _, value := range Extract(identityFixture()).CoreReferences {
		occupancies[value.UUID] = Occupancy{UUID: value.UUID, Kind: value.Kind, State: OccupancyActive, CoreCompatible: &compatible}
	}
	result := Inspect(Extract(identityFixture()), occupancies)
	if result.Class != ClassUnclaimed || len(result.Issues) != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestInspectUsesStableConflictPriority(t *testing.T) {
	compatible := true
	occupancies := map[string]Occupancy{
		galleryUUID:   {UUID: galleryUUID, Kind: portableid.KindWork, State: OccupancyActive},
		itemUUID:      {UUID: itemUUID, Kind: portableid.KindGalleryItem, State: OccupancyTombstone},
		linkUUID:      {UUID: linkUUID, Kind: portableid.KindExternalLink, State: OccupancyPending, ClaimImportID: "import-1"},
		coserUUID:     {UUID: coserUUID, Kind: portableid.KindCoser, State: OccupancyActive, CoreCompatible: &compatible},
		workUUID:      {UUID: workUUID, Kind: portableid.KindWork, State: OccupancyActive, CoreCompatible: &compatible},
		characterUUID: {UUID: characterUUID, Kind: portableid.KindCharacter, State: OccupancyActive, CoreCompatible: &compatible},
		tagUUID:       {UUID: tagUUID, Kind: portableid.KindTag, State: OccupancyActive, CoreCompatible: &compatible},
	}
	result := Inspect(Extract(identityFixture()), occupancies)
	if result.Class != ClassUUIDKindConflict {
		t.Fatalf("class = %q, want %q", result.Class, ClassUUIDKindConflict)
	}
	if len(result.Issues) != 3 || result.Issues[0].Code != ClassUUIDKindConflict || result.Issues[1].Code != ClassUUIDRetired || result.Issues[2].Code != ClassPendingPortableClaim {
		t.Fatalf("issues = %#v", result.Issues)
	}
}

func TestInspectRejectsForeignLocalItemAndIncompatibleCoreReference(t *testing.T) {
	compatible, incompatible := true, false
	occupancies := map[string]Occupancy{
		itemUUID:      {UUID: itemUUID, Kind: portableid.KindGalleryItem, State: OccupancyActive, OwnerSetID: "88888888-8888-4888-8888-888888888888"},
		coserUUID:     {UUID: coserUUID, Kind: portableid.KindCoser, State: OccupancyActive, CoreCompatible: &compatible},
		workUUID:      {UUID: workUUID, Kind: portableid.KindWork, State: OccupancyActive, CoreCompatible: &compatible},
		characterUUID: {UUID: characterUUID, Kind: portableid.KindCharacter, State: OccupancyActive, CoreCompatible: &incompatible},
		tagUUID:       {UUID: tagUUID, Kind: portableid.KindTag, State: OccupancyActive, CoreCompatible: &compatible},
	}
	result := Inspect(Extract(identityFixture()), occupancies)
	if result.Class != ClassLocalItemOrLinkConflict || len(result.Issues) != 2 {
		t.Fatalf("result = %#v", result)
	}
}

func identityFixture() manifest.GalleryDocument {
	return manifest.GalleryDocument{
		SetID:         galleryUUID,
		Items:         manifest.Optional[[]manifest.Item]{Present: true, Value: []manifest.Item{{ItemUUID: itemUUID, Path: "01.jpg"}}},
		ExternalLinks: manifest.Optional[[]manifest.ExternalLink]{Present: true, Value: []manifest.ExternalLink{{LinkUUID: linkUUID}}},
		Credits:       manifest.Optional[[]manifest.Credit]{Present: true, Value: []manifest.Credit{{Coser: manifest.EntityReference{UUID: coserUUID, NameHint: "Coser"}}}},
		Cast:          manifest.Optional[[]manifest.Cast]{Present: true, Value: []manifest.Cast{{Coser: manifest.EntityReference{UUID: coserUUID}, Work: manifest.EntityReference{UUID: workUUID}, Character: manifest.EntityReference{UUID: characterUUID}}}},
		Tags:          manifest.Optional[[]manifest.EntityReference]{Present: true, Value: []manifest.EntityReference{{UUID: tagUUID}}},
	}
}
