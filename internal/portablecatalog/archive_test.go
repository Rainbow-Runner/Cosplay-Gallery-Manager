package portablecatalog

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stashapp/stash/internal/product"
)

const (
	testCoserUUID     = "11111111-1111-4111-8111-111111111111"
	testWorkUUID      = "22222222-2222-4222-8222-222222222222"
	testCharacterUUID = "33333333-3333-4333-8333-333333333333"
	testTagUUID       = "44444444-4444-4444-8444-444444444444"
	testAccountUUID   = "55555555-5555-4555-8555-555555555555"
	testGalleryUUID   = "66666666-6666-4666-8666-666666666666"
	testExportUUID    = "77777777-7777-4777-8777-777777777777"
	testTime          = "2026-09-07T12:00:00Z"
)

func TestPortablePackageRoundTripIsDeterministic(t *testing.T) {
	bundle := validBundle()
	assetBytes := testPNG(t)
	assets := func() []AssetSource {
		return []AssetSource{{Path: "coser-assets/" + testCoserUUID + "/avatar-original.png", Kind: "COSER_AVATAR_ORIGINAL", Size: int64(len(assetBytes)), Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(assetBytes)), nil }}}
	}
	first, second := filepath.Join(t.TempDir(), "first.cgm-portable.zip"), filepath.Join(t.TempDir(), "second.cgm-portable.zip")
	if err := WriteFileAtomic(first, bundle, assets()); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(second, bundle, assets()); err != nil {
		t.Fatal(err)
	}
	firstBytes, _ := os.ReadFile(first)
	secondBytes, _ := os.ReadFile(second)
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatal("identical portable snapshots produced different archives")
	}
	inspection, err := InspectFile(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Manifest.CoserCount != 1 || inspection.Manifest.AssetCount != 1 || len(inspection.Bundle.Catalog.Accounts) != 1 {
		t.Fatalf("unexpected package counts: %+v", inspection.Manifest)
	}
	if inspection.Bundle.Catalog.Cosers[0].Avatar == nil || inspection.Bundle.Catalog.Cosers[0].Avatar.PackagePath == "" {
		t.Fatal("avatar reference was not preserved")
	}
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	imageValue := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	imageValue.Set(0, 0, color.NRGBA{R: 255, A: 255})
	if err := png.Encode(&output, imageValue); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestPortablePackageRejectsChangedPayloadAndUnsafeNames(t *testing.T) {
	bundle := validBundle()
	valid := filepath.Join(t.TempDir(), "valid.cgm-portable.zip")
	if err := WriteFileAtomic(valid, bundle, nil); err == nil {
		t.Fatal("missing referenced asset was accepted by writer")
	}
	bundle.Catalog.Cosers[0].Avatar = nil
	if err := WriteFileAtomic(valid, bundle, nil); err != nil {
		t.Fatal(err)
	}
	tampered := filepath.Join(t.TempDir(), "tampered.cgm-portable.zip")
	if err := rewriteArchive(valid, tampered, func(name string, data []byte) (string, []byte) {
		if name == "core-catalog.json" {
			return name, append(data, ' ')
		}
		return name, data
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectFile(context.Background(), tampered); err == nil || !strings.Contains(err.Error(), "size mismatch") {
		t.Fatalf("tampered payload error = %v", err)
	}
	unsafe := filepath.Join(t.TempDir(), "unsafe.cgm-portable.zip")
	if err := rewriteArchive(valid, unsafe, func(name string, data []byte) (string, []byte) {
		if name == "gallery-index.json" {
			return "coser-assets/CON.jpg", data
		}
		return name, data
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectFile(context.Background(), unsafe); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("unsafe path error = %v", err)
	}
}

func TestPortablePackageRejectsUnknownIdentityKind(t *testing.T) {
	bundle := validBundle()
	bundle.Identity.Identities[0].Kind = "UNKNOWN"
	var output bytes.Buffer
	if err := Write(&output, bundle, nil); err == nil || !strings.Contains(err.Error(), "unsupported portable UUID kind") {
		t.Fatalf("identity kind error = %v", err)
	}
}

func TestPortablePackageStreamsLargeItemLedgerWithoutRetainingItems(t *testing.T) {
	bundle := validBundle()
	bundle.Catalog.Cosers[0].Avatar = nil
	identities := append([]IdentityRecord{}, bundle.Identity.Identities...)
	for index := 1; index <= 20_000; index++ {
		identities = append(identities, IdentityRecord{UUID: fmt.Sprintf("80000000-0000-4000-8000-%012d", index), Kind: "GALLERY_ITEM", State: "ACTIVE", CreatedAt: testTime})
	}
	target := filepath.Join(t.TempDir(), "streamed.cgm-portable.zip")
	source := IdentitySource{Count: len(identities), Stream: func(yield func(IdentityRecord) error) error {
		for _, identity := range identities {
			if err := yield(identity); err != nil {
				return err
			}
		}
		return nil
	}}
	if err := WriteFileAtomicStreaming(target, bundle, nil, source); err != nil {
		t.Fatal(err)
	}
	inspection, err := InspectFile(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Manifest.IdentityCount != len(identities) {
		t.Fatalf("identity count = %d", inspection.Manifest.IdentityCount)
	}
	if len(inspection.Bundle.Identity.Identities) != len(bundle.Identity.Identities) {
		t.Fatalf("inspector retained %d identities instead of only %d portable objects", len(inspection.Bundle.Identity.Identities), len(bundle.Identity.Identities))
	}
}

func TestPortableStreamingInspectorRejectsAliasCycle(t *testing.T) {
	inspectionTemp := t.TempDir()
	t.Setenv("TMPDIR", inspectionTemp)
	bundle := validBundle()
	bundle.Catalog.Cosers[0].Avatar = nil
	identities := append([]IdentityRecord{}, bundle.Identity.Identities...)
	identities = append(identities,
		IdentityRecord{UUID: "80000000-0000-4000-8000-000000000001", Kind: "GALLERY_ITEM", State: "ALIAS", CreatedAt: testTime, TargetUUID: "80000000-0000-4000-8000-000000000002", RetiredAt: testTime},
		IdentityRecord{UUID: "80000000-0000-4000-8000-000000000002", Kind: "GALLERY_ITEM", State: "ALIAS", CreatedAt: testTime, TargetUUID: "80000000-0000-4000-8000-000000000001", RetiredAt: testTime},
	)
	target := filepath.Join(t.TempDir(), "alias-cycle.cgm-portable.zip")
	if err := WriteFileAtomicStreaming(target, bundle, nil, IdentitySource{Count: len(identities), Stream: func(yield func(IdentityRecord) error) error {
		for _, identity := range identities {
			if err := yield(identity); err != nil {
				return err
			}
		}
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectFile(context.Background(), target); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("Alias cycle inspection error = %v", err)
	}
	leftovers, err := filepath.Glob(filepath.Join(inspectionTemp, ".cgm-portable-identities-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(leftovers) != 0 {
		t.Fatalf("Inspector left temporary identity indexes: %v", leftovers)
	}
}

func validBundle() Bundle {
	identities := []IdentityRecord{
		{UUID: testCoserUUID, Kind: "COSER", State: "ACTIVE", CreatedAt: testTime}, {UUID: testWorkUUID, Kind: "WORK", State: "ACTIVE", CreatedAt: testTime},
		{UUID: testCharacterUUID, Kind: "CHARACTER", State: "ACTIVE", CreatedAt: testTime}, {UUID: testTagUUID, Kind: "TAG", State: "ACTIVE", CreatedAt: testTime},
		{UUID: testAccountUUID, Kind: "SOCIAL_ACCOUNT", State: "ACTIVE", CreatedAt: testTime}, {UUID: testGalleryUUID, Kind: "GALLERY", State: "ACTIVE", CreatedAt: testTime},
	}
	named := func(uuid, name, slug string) NamedEntity {
		return NamedEntity{UUID: uuid, Name: name, SortName: "", Aliases: []string{}, Slug: slug, MetadataRevision: 1, CreatedAt: testTime, UpdatedAt: testTime}
	}
	return Bundle{
		Manifest: PackageManifest{Format: Format, FormatVersion: FormatVersion, ProductID: product.ID, ExportID: testExportUUID, CreatedAt: testTime, Versions: Versions(product.CurrentVersions("1.5.0-test"))},
		Identity: IdentityLedger{SchemaVersion: 1, Identities: identities},
		Catalog: CoreCatalog{SchemaVersion: 1,
			Cosers: []Coser{{NamedEntity: named(testCoserUUID, "Alice", "alice"), Avatar: &AssetRef{PackagePath: "coser-assets/" + testCoserUUID + "/avatar-original.png", Kind: "AVATAR"}}},
			Works:  []Work{{NamedEntity: named(testWorkUUID, "Work", "work")}}, Characters: []Character{{NamedEntity: named(testCharacterUUID, "Hero", "hero"), WorkUUID: testWorkUUID}},
			Tags: []Tag{{NamedEntity: named(testTagUUID, "Tag", "tag"), UseInRecommendation: true}}, TagEdges: []TagEdge{},
			Accounts: []SocialAccount{{UUID: testAccountUUID, CoserUUID: testCoserUUID, PlatformKey: "x", URL: "https://example.com/alice", Status: "ACTIVE", Visible: true, Position: 1024}}, SlugRedirects: []SlugRedirect{},
		},
		Gallery: GalleryIndex{SchemaVersion: 1, Libraries: []LibraryLocator{{Key: "library-000001", Name: "Library"}}, Galleries: []GalleryLocator{{SetID: testGalleryUUID, LibraryKey: "library-000001", SourceType: "DIRECTORY", RelativeSource: "Alice/Set", LocatorStatus: "MAPPED", ManifestStatus: "CLEAN", ManifestSchema: 1, ManifestRevision: 1, ManifestHash: "abc"}}},
	}
}

func rewriteArchive(source, target string, transform func(string, []byte) (string, []byte)) error {
	input, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.Create(target)
	if err != nil {
		return err
	}
	archive := zip.NewWriter(output)
	for _, entry := range input.File {
		reader, err := entry.Open()
		if err != nil {
			return err
		}
		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			return err
		}
		name, data := transform(entry.Name, data)
		writer, err := archive.Create(name)
		if err != nil {
			return err
		}
		if _, err := writer.Write(data); err != nil {
			return err
		}
	}
	if err := archive.Close(); err != nil {
		return err
	}
	return output.Close()
}
