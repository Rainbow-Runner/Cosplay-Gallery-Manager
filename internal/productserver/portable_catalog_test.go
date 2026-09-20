package productserver

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/archivecheck"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portablecatalog"
	"github.com/stashapp/stash/internal/portableid"
)

func TestPortableMetadataExportIncludesCurrentCoserAssetAndVerifies(t *testing.T) {
	server := testServer(t)
	ctx := context.Background()
	root := t.TempDir()
	coserRoot, backupRoot := filepath.Join(root, "cosers"), filepath.Join(root, "backups")
	if err := os.MkdirAll(coserRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Database.ExecContext(ctx, `UPDATE product_setup SET complete=1,coser_metadata_root=?,backup_root=?,completed_at_utc=? WHERE id=1`, coserRoot, backupRoot, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	coser, err := server.Database.CoreEntities().CreateCoser(ctx, productdb.CreateCoserInput{CreateNamedEntityInput: productdb.CreateNamedEntityInput{Name: "Portable Coser"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	relative := "assets/avatar-11111111-1111-4111-8111-111111111111.png"
	assetDirectory := filepath.Join(coserRoot, coser.UUID, "assets")
	if err := os.MkdirAll(assetDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	imageValue := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	imageValue.Set(0, 0, color.NRGBA{R: 255, A: 255})
	if err := png.Encode(&encoded, imageValue); err != nil {
		t.Fatal(err)
	}
	assetBytes := encoded.Bytes()
	if err := os.WriteFile(filepath.Join(coserRoot, coser.UUID, filepath.FromSlash(relative)), assetBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Database.CoreEntities().SetCoserManagedAsset(ctx, coser.UUID, coser.MetadataRevision, productdb.CoserManagedAssetInput{Kind: productdb.CoserAssetAvatar, RelativePath: relative}, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	preflight, err := server.PreflightPortableMetadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if preflight.BlockingCount() != 0 || preflight.WarningCount() != 0 || preflight.AssetCount != 1 || preflight.AssetBytes != int64(len(assetBytes)) {
		t.Fatalf("unexpected portable preflight: %+v", preflight)
	}
	target := filepath.Join(root, "portable.cgm-portable.zip")
	result, err := server.ExportPortableMetadata(ctx, PortableExportOptions{TargetPath: target})
	if err != nil {
		t.Fatal(err)
	}
	if result.AssetCount != 1 || result.CoreEntityCount != 1 || result.WarningCount != 0 || result.ArchiveSHA256 == "" {
		t.Fatalf("unexpected export result: %+v", result)
	}
	inspection, err := portablecatalog.InspectFile(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Bundle.Catalog.Cosers) != 1 || inspection.Bundle.Catalog.Cosers[0].Avatar == nil {
		t.Fatal("portable package omitted current Coser avatar")
	}
	if _, err := os.Stat(filepath.Join(assetDirectory, "avatar-11111111-1111-4111-8111-111111111111-480.jpg")); !os.IsNotExist(err) {
		t.Fatal("portable export generated a derivative")
	}
	if err := os.WriteFile(filepath.Join(coserRoot, coser.UUID, filepath.FromSlash(relative)), []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}
	invalid, err := server.PreflightPortableMetadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if invalid.BlockingCount() != 1 || len(invalid.Issues) != 1 || invalid.Issues[0].Code != "PORTABLE_COSER_ASSET_INVALID" {
		t.Fatalf("invalid managed asset was not reported: %+v", invalid)
	}
}

func TestPortableMetadataExportRequiresExplicitIncompleteGalleryOverride(t *testing.T) {
	server := testServer(t)
	ctx := context.Background()
	root := t.TempDir()
	coserRoot, backupRoot := filepath.Join(root, "cosers"), filepath.Join(root, "backups")
	if err := os.MkdirAll(coserRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Database.ExecContext(ctx, `UPDATE product_setup SET complete=1,coser_metadata_root=?,backup_root=?,completed_at_utc=? WHERE id=1`, coserRoot, backupRoot, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Database.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Missing manifest"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	preflight, err := server.PreflightPortableMetadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if preflight.IncompleteGalleryCount != 1 || preflight.WarningCount() != 2 || preflight.BlockingCount() != 0 {
		t.Fatalf("unexpected incomplete Gallery preflight: %+v", preflight)
	}
	strictTarget := filepath.Join(root, "strict.zip")
	if _, err := server.ExportPortableMetadata(ctx, PortableExportOptions{TargetPath: strictTarget}); err == nil {
		t.Fatal("incomplete Gallery index was exported without an override")
	}
	if _, err := os.Stat(strictTarget); !os.IsNotExist(err) {
		t.Fatal("blocked export created a target file")
	}
	overrideTarget := filepath.Join(root, "override.zip")
	result, err := server.ExportPortableMetadata(ctx, PortableExportOptions{TargetPath: overrideTarget, AllowIncompleteGallery: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.WarningCount != 1 {
		t.Fatalf("warning count = %d", result.WarningCount)
	}
}

func TestPortableMetadataImportRestoresCoreAssetsAndReservesGallery(t *testing.T) {
	ctx := context.Background()
	source := testServer(t)
	sourceRoot := t.TempDir()
	sourceCosers := filepath.Join(sourceRoot, "cosers")
	if err := os.MkdirAll(sourceCosers, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Database.ExecContext(ctx, `UPDATE product_setup SET complete=1,coser_metadata_root=?,backup_root=?,completed_at_utc=? WHERE id=1`, sourceCosers, filepath.Join(sourceRoot, "backups"), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	coser, err := source.Database.CoreEntities().CreateCoser(ctx, productdb.CreateCoserInput{CreateNamedEntityInput: productdb.CreateNamedEntityInput{Name: "Imported Coser", Aliases: []string{"Alias"}}}, now)
	if err != nil {
		t.Fatal(err)
	}
	assetRelative := "assets/avatar-11111111-1111-4111-8111-111111111111.png"
	assetDirectory := filepath.Join(sourceCosers, coser.UUID, "assets")
	if err := os.MkdirAll(assetDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	assetBytes := encoded.Bytes()
	if err := os.WriteFile(filepath.Join(sourceCosers, coser.UUID, filepath.FromSlash(assetRelative)), assetBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Database.CoreEntities().SetCoserManagedAsset(ctx, coser.UUID, coser.MetadataRevision, productdb.CoserManagedAssetInput{Kind: productdb.CoserAssetAvatar, RelativePath: assetRelative}, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	gallery, err := source.Database.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Rebuild Later"}, now)
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(sourceRoot, "portable.zip")
	if _, err := source.ExportPortableMetadata(ctx, PortableExportOptions{TargetPath: archivePath, AllowIncompleteGallery: true}); err != nil {
		t.Fatal(err)
	}

	target := testServer(t)
	targetRoot := t.TempDir()
	targetCosers := filepath.Join(targetRoot, "cosers")
	targetBackups := filepath.Join(targetRoot, "backups")
	if _, err := target.Database.ExecContext(ctx, `UPDATE product_setup SET complete=1,coser_metadata_root=?,backup_root=?,completed_at_utc=? WHERE id=1`, targetCosers, targetBackups, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	result, err := target.ImportPortableMetadata(ctx, PortableImportOptions{SourcePath: archivePath})
	if err != nil {
		t.Fatal(err)
	}
	if result.CoreEntityCount != 1 || result.ClaimCount != 1 || result.AssetCount != 1 || result.SafetyBackupID == "" {
		t.Fatalf("unexpected import result: %+v", result)
	}
	var name, avatarPath string
	if err := target.Database.QueryRowContext(ctx, `SELECT name,avatar_path FROM cosers WHERE uuid=?`, coser.UUID).Scan(&name, &avatarPath); err != nil {
		t.Fatal(err)
	}
	if name != "Imported Coser" || avatarPath != assetRelative {
		t.Fatalf("imported Coser=%q avatar=%q", name, avatarPath)
	}
	gotAsset, err := os.ReadFile(filepath.Join(targetCosers, coser.UUID, filepath.FromSlash(avatarPath)))
	if err != nil || !bytes.Equal(gotAsset, assetBytes) {
		t.Fatalf("imported asset mismatch: bytes=%d err=%v", len(gotAsset), err)
	}
	if _, err := os.Stat(filepath.Join(targetCosers, coser.UUID, "assets", "avatar-11111111-1111-4111-8111-111111111111-480.jpg")); err != nil {
		t.Fatalf("avatar derivative was not rebuilt: %v", err)
	}
	var claimState string
	if err := target.Database.QueryRowContext(ctx, `SELECT claim_state FROM portable_identity_claims WHERE uuid=? AND entity_kind='GALLERY'`, gallery.SetID).Scan(&claimState); err != nil || claimState != "PENDING" {
		t.Fatalf("Gallery claim=%q err=%v", claimState, err)
	}
	var galleryCount int
	if err := target.Database.QueryRowContext(ctx, `SELECT COUNT(*) FROM galleries`).Scan(&galleryCount); err != nil || galleryCount != 0 {
		t.Fatalf("Gallery rows=%d err=%v", galleryCount, err)
	}
	if _, err := target.ImportPortableMetadata(ctx, PortableImportOptions{SourcePath: archivePath}); !errors.Is(err, productdb.ErrPortableImportTargetNotEmpty) {
		t.Fatalf("second import error=%v", err)
	}
	marker := filepath.Join(targetCosers, coser.UUID, portableImportOwnerMarker)
	if err := os.WriteFile(marker, []byte(result.ImportID+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := target.Database.Operations().SetMaintenance(ctx, productdb.MaintenancePortableImporting, result.ImportID, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	recovered, err := target.RecoverPortableMetadataImport(ctx)
	if err != nil || recovered.ImportID != result.ImportID {
		t.Fatalf("recovery=%+v err=%v", recovered, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("recovery did not remove ownership marker: %v", err)
	}
	maintenance, err := target.Database.Operations().Maintenance(ctx)
	if err != nil || maintenance.Mode != productdb.MaintenanceNormal {
		t.Fatalf("maintenance=%+v err=%v", maintenance, err)
	}
	if _, err := os.Stat(filepath.Join(targetBackups, "portable-imports", result.ImportID, "package.zip")); err != nil {
		t.Fatalf("quarantined package was not retained: %v", err)
	}
	var backupStatus string
	if err := target.Database.QueryRowContext(ctx, `SELECT status FROM backup_records WHERE backup_id=?`, result.SafetyBackupID).Scan(&backupStatus); err != nil || backupStatus != "READY" {
		t.Fatalf("safety backup status=%q err=%v", backupStatus, err)
	}
}

func TestInterruptedPortableAssetCleanupRequiresOwnershipMarker(t *testing.T) {
	root := t.TempDir()
	importID := "99999999-9999-4999-8999-999999999999"
	owned := "11111111-1111-4111-8111-111111111111"
	unowned := "22222222-2222-4222-8222-222222222222"
	if err := os.MkdirAll(filepath.Join(root, owned), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, owned, portableImportOwnerMarker), []byte(importID+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeInterruptedPortableAssets(root, []string{owned}, importID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, owned)); !os.IsNotExist(err) {
		t.Fatalf("owned directory was not removed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, unowned), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := removeInterruptedPortableAssets(root, []string{unowned}, importID); err == nil {
		t.Fatal("unowned directory was accepted for cleanup")
	}
	if _, err := os.Stat(filepath.Join(root, unowned)); err != nil {
		t.Fatalf("unowned directory was changed: %v", err)
	}
}

func TestPortableGalleryRebuildPreservesDirectoryIdentitiesAtNewRoot(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	source := testServer(t)
	sourceBase := t.TempDir()
	configurePortableTestServer(t, source, sourceBase)
	sourceMedia := filepath.Join(sourceBase, "old-media")
	setPath := filepath.Join(sourceMedia, "Portable Coser", "Moved Set")
	if err := os.MkdirAll(setPath, 0o700); err != nil {
		t.Fatal(err)
	}
	writePortableTestPNG(t, filepath.Join(setPath, "photo.png"))
	sourceLibrary, err := source.Database.Libraries().Create(ctx, productdb.CreateLibraryInput{Name: "Old logical library", RootPath: sourceMedia, Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	created, err := source.Database.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Moved Set", ContentRating: gallery.ContentRatingNonAdult}, now)
	if err != nil {
		t.Fatal(err)
	}
	gallerySource, err := source.Database.Galleries().AddSource(ctx, created.ID, productdb.CreateSourceInput{LibraryID: &sourceLibrary.ID, Type: gallery.SourceTypeDirectory, Path: setPath, Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Database.Scans().Run(ctx, gallerySource.ID, archivecheck.DefaultLimits(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Database.ExecContext(ctx, `UPDATE gallery_items SET excluded=0 WHERE gallery_id=?`, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Database.ExecContext(ctx, `UPDATE galleries SET shoot_date='2024-05-20',shoot_date_precision='DAY' WHERE id=?`, created.ID); err != nil {
		t.Fatal(err)
	}
	var sourceItemUUID string
	if err := source.Database.QueryRowContext(ctx, `SELECT item_uuid FROM gallery_items WHERE gallery_id=?`, created.ID).Scan(&sourceItemUUID); err != nil {
		t.Fatal(err)
	}
	current, err := source.Database.Galleries().Find(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	link, err := source.Database.Galleries().AddExternalLink(ctx, created.ID, gallery.ExternalLinkSource, "Source", "https://example.test/moved", 1024, current.MetadataRevision, now)
	if err != nil {
		t.Fatal(err)
	}
	current, _ = source.Database.Galleries().Find(ctx, created.ID)
	if _, err := source.Database.Manifests().PushGallery(ctx, created.ID, current.MetadataRevision, now); err != nil {
		t.Fatal(err)
	}
	packagePath := filepath.Join(sourceBase, "portable.zip")
	exportPreflight, err := source.PreflightPortableMetadata(ctx)
	if err != nil || exportPreflight.IncompleteGalleryCount != 0 {
		t.Fatalf("source preflight=%+v err=%v", exportPreflight, err)
	}
	if _, err := source.ExportPortableMetadata(ctx, PortableExportOptions{TargetPath: packagePath}); err != nil {
		t.Fatal(err)
	}

	target := testServer(t)
	targetBase := t.TempDir()
	configurePortableTestServer(t, target, targetBase)
	targetMedia := filepath.Join(targetBase, "new-media")
	targetSetPath := filepath.Join(targetMedia, "Portable Coser", "Moved Set")
	if err := os.MkdirAll(targetSetPath, 0o700); err != nil {
		t.Fatal(err)
	}
	var targetManifest []byte
	for _, name := range []string{"photo.png", ".cosplay.json"} {
		data, err := os.ReadFile(filepath.Join(setPath, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(targetSetPath, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
		if name == ".cosplay.json" {
			targetManifest = append([]byte(nil), data...)
		}
	}
	targetLibrary, err := target.Database.Libraries().Create(ctx, productdb.CreateLibraryInput{Name: "New machine library", RootPath: targetMedia, Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := target.ImportPortableMetadata(ctx, PortableImportOptions{SourcePath: packagePath})
	if err != nil {
		t.Fatal(err)
	}
	mappings, err := target.Database.ListPortableLibraryMappings(ctx, imported.ImportID)
	if err != nil || len(mappings) != 1 || mappings[0].LibraryName != sourceLibrary.Name {
		t.Fatalf("logical mappings=%+v err=%v", mappings, err)
	}
	if err := target.MapPortableLibraries(ctx, imported.ImportID, []productdb.PortableLibraryDecision{{LibraryKey: mappings[0].LibraryKey, TargetLibraryID: &targetLibrary.ID}}); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(targetSetPath, ".cosplay.json")
	if err := os.WriteFile(manifestPath, append(targetManifest, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := target.PreflightPortableGalleryRebuild(ctx, imported.ImportID)
	if err != nil || changed.Blocked != 1 || len(changed.Entries) != 1 || changed.Entries[0].IssueCode != "PORTABLE_MANIFEST_CHANGED" {
		t.Fatalf("changed manifest preflight=%+v err=%v", changed, err)
	}
	if _, err := target.RebuildPortableGalleries(ctx, imported.ImportID); err == nil {
		t.Fatal("blocked portable Gallery rebuild returned success")
	}
	var changedGalleryCount int
	if err := target.Database.QueryRowContext(ctx, `SELECT COUNT(*) FROM galleries`).Scan(&changedGalleryCount); err != nil || changedGalleryCount != 0 {
		t.Fatalf("blocked preflight created %d galleries: %v", changedGalleryCount, err)
	}
	if err := os.WriteFile(manifestPath, targetManifest, 0o600); err != nil {
		t.Fatal(err)
	}
	preflight, err := target.PreflightPortableGalleryRebuild(ctx, imported.ImportID)
	if err != nil || preflight.Ready != 1 || preflight.Blocked != 0 {
		t.Fatalf("preflight=%+v err=%v", preflight, err)
	}
	var before int
	if err := target.Database.QueryRowContext(ctx, `SELECT COUNT(*) FROM galleries`).Scan(&before); err != nil || before != 0 {
		t.Fatalf("read-only preflight created %d galleries: %v", before, err)
	}
	rebuilt, err := target.RebuildPortableGalleries(ctx, imported.ImportID)
	if err != nil || rebuilt.Rebuilt != 1 || rebuilt.Blocked != 0 {
		t.Fatalf("rebuild=%+v err=%v", rebuilt, err)
	}
	var targetGalleryID int64
	var state, title string
	if err := target.Database.QueryRowContext(ctx, `SELECT id,state,title FROM galleries WHERE set_id=?`, created.SetID).Scan(&targetGalleryID, &state, &title); err != nil {
		t.Fatal(err)
	}
	if state != "DRAFT" || title != "Moved Set" {
		t.Fatalf("rebuilt Gallery state/title=%q/%q", state, title)
	}
	var migratedDate, origin string
	if err := target.Database.QueryRowContext(ctx, `SELECT shoot_date,shoot_date_origin FROM galleries WHERE id=?`, targetGalleryID).Scan(&migratedDate, &origin); err != nil || migratedDate != "2024-05-20" || origin != "MANUAL" {
		t.Fatalf("migrated shoot date=%q origin=%q err=%v", migratedDate, origin, err)
	}
	var dateJobs, primaryJobs int
	if err := target.Database.QueryRowContext(ctx, `SELECT COUNT(*) FROM processing_jobs WHERE gallery_id=? AND variant='CAPTURE_DATE'`, targetGalleryID).Scan(&dateJobs); err != nil || dateJobs != 0 {
		t.Fatalf("date backfill should defer to primary processing: jobs=%d err=%v", dateJobs, err)
	}
	if err := target.Database.QueryRowContext(ctx, `SELECT COUNT(*) FROM processing_jobs WHERE gallery_id=? AND variant='CARD_480' AND status IN ('PENDING','RUNNING','RETRY_WAIT')`, targetGalleryID).Scan(&primaryJobs); err != nil || primaryJobs != 1 {
		t.Fatalf("target first-derivative jobs=%d err=%v", primaryJobs, err)
	}
	var targetItemUUID, targetLinkUUID, targetSourcePath string
	if err := target.Database.QueryRowContext(ctx, `SELECT item_uuid FROM gallery_items WHERE gallery_id=?`, targetGalleryID).Scan(&targetItemUUID); err != nil {
		t.Fatal(err)
	}
	if err := target.Database.QueryRowContext(ctx, `SELECT link_uuid FROM gallery_external_links WHERE gallery_id=?`, targetGalleryID).Scan(&targetLinkUUID); err != nil {
		t.Fatal(err)
	}
	if err := target.Database.QueryRowContext(ctx, `SELECT source_path FROM gallery_sources WHERE gallery_id=?`, targetGalleryID).Scan(&targetSourcePath); err != nil {
		t.Fatal(err)
	}
	if targetItemUUID != sourceItemUUID || targetLinkUUID != link.UUID || targetSourcePath != targetSetPath {
		t.Fatalf("identity/path changed: item=%q link=%q source=%q", targetItemUUID, targetLinkUUID, targetSourcePath)
	}
	var targetContentRevision int64
	if err := target.Database.QueryRowContext(ctx, `SELECT content_revision FROM gallery_items WHERE item_uuid=?`, targetItemUUID).Scan(&targetContentRevision); err != nil {
		t.Fatal(err)
	}
	if err := target.Database.CaptureDates().Publish(ctx, targetItemUUID, targetContentRevision, "2024-05-12", "exif.DateTimeOriginal", now); err != nil {
		t.Fatal(err)
	}
	dateSummary, err := target.Database.CaptureDates().Summary(ctx, targetGalleryID)
	if err != nil || dateSummary.ReviewStatus != "PENDING" || dateSummary.Manual != "2024-05-20" || dateSummary.Candidate != "2024-05-12" {
		t.Fatalf("rebuilt date review=%+v err=%v", dateSummary, err)
	}
	var sessionState string
	if err := target.Database.QueryRowContext(ctx, `SELECT state FROM portable_import_sessions WHERE import_id=?`, imported.ImportID).Scan(&sessionState); err != nil || sessionState != "GALLERIES_REBUILT" {
		t.Fatalf("session state=%q err=%v", sessionState, err)
	}
}

func TestPortableGalleryRebuildPreservesArchiveIdentityAtNewRoot(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 9, 13, 0, 0, 0, time.UTC)
	source := testServer(t)
	sourceBase := t.TempDir()
	configurePortableTestServer(t, source, sourceBase)
	sourceMedia := filepath.Join(sourceBase, "old-archives")
	if err := os.MkdirAll(filepath.Join(sourceMedia, "sets"), 0o700); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(sourceMedia, "sets", "archive-set.zip")
	writePortableTestZIP(t, archivePath)
	sourceLibrary, err := source.Database.Libraries().Create(ctx, productdb.CreateLibraryInput{Name: "Archive library", RootPath: sourceMedia, Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	created, err := source.Database.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Archive Set", ContentRating: gallery.ContentRatingNonAdult}, now)
	if err != nil {
		t.Fatal(err)
	}
	gallerySource, err := source.Database.Galleries().AddSource(ctx, created.ID, productdb.CreateSourceInput{LibraryID: &sourceLibrary.ID, Type: gallery.SourceTypeArchive, Path: archivePath, Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Database.Scans().Run(ctx, gallerySource.ID, archivecheck.DefaultLimits(), now); err != nil {
		t.Fatal(err)
	}
	var itemUUID string
	if err := source.Database.QueryRowContext(ctx, `SELECT item_uuid FROM gallery_items WHERE gallery_id=?`, created.ID).Scan(&itemUUID); err != nil {
		t.Fatal(err)
	}
	current, _ := source.Database.Galleries().Find(ctx, created.ID)
	manifestState, err := source.Database.Manifests().PushGallery(ctx, created.ID, current.MetadataRevision, now)
	if err != nil {
		t.Fatal(err)
	}
	packagePath := filepath.Join(sourceBase, "archive-portable.zip")
	if _, err := source.ExportPortableMetadata(ctx, PortableExportOptions{TargetPath: packagePath}); err != nil {
		t.Fatal(err)
	}

	target := testServer(t)
	targetBase := t.TempDir()
	configurePortableTestServer(t, target, targetBase)
	targetMedia := filepath.Join(targetBase, "new-archives")
	targetArchive := filepath.Join(targetMedia, "sets", "archive-set.zip")
	if err := os.MkdirAll(filepath.Dir(targetArchive), 0o700); err != nil {
		t.Fatal(err)
	}
	for sourcePath, targetPath := range map[string]string{
		archivePath:        targetArchive,
		manifestState.Path: filepath.Join(filepath.Dir(targetArchive), filepath.Base(manifestState.Path)),
	} {
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(targetPath, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	targetLibrary, err := target.Database.Libraries().Create(ctx, productdb.CreateLibraryInput{Name: "Archive target", RootPath: targetMedia, Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := target.ImportPortableMetadata(ctx, PortableImportOptions{SourcePath: packagePath})
	if err != nil {
		t.Fatal(err)
	}
	mappings, err := target.Database.ListPortableLibraryMappings(ctx, imported.ImportID)
	if err != nil || len(mappings) != 1 {
		t.Fatalf("mappings=%+v err=%v", mappings, err)
	}
	if err := target.MapPortableLibraries(ctx, imported.ImportID, []productdb.PortableLibraryDecision{{LibraryKey: mappings[0].LibraryKey, TargetLibraryID: &targetLibrary.ID}}); err != nil {
		t.Fatal(err)
	}
	report, err := target.RebuildPortableGalleries(ctx, imported.ImportID)
	if err != nil || report.Rebuilt != 1 {
		t.Fatalf("archive rebuild=%+v err=%v", report, err)
	}
	var rebuiltItem, rebuiltSource, rebuiltType string
	if err := target.Database.QueryRowContext(ctx, `SELECT item.item_uuid,source.source_path,source.source_type
		FROM gallery_items item JOIN gallery_sources source ON source.id=item.source_id
		JOIN galleries gallery ON gallery.id=item.gallery_id WHERE gallery.set_id=?`, created.SetID).Scan(&rebuiltItem, &rebuiltSource, &rebuiltType); err != nil {
		t.Fatal(err)
	}
	if rebuiltItem != itemUUID || rebuiltSource != targetArchive || rebuiltType != string(gallery.SourceTypeArchive) {
		t.Fatalf("archive identity/source changed: %q %q %q", rebuiltItem, rebuiltSource, rebuiltType)
	}
}

func TestPortableMergePreflightReportsIdentityAndNameConflictsWithoutBusinessWrites(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 9, 16, 0, 0, 0, time.UTC)
	const (
		coserUUID = "11111111-aaaa-4aaa-8aaa-111111111111"
		workUUID  = "22222222-bbbb-4bbb-8bbb-222222222222"
		tagUUID   = "33333333-cccc-4ccc-8ccc-333333333333"
		localUUID = "44444444-dddd-4ddd-8ddd-444444444444"
		reuseUUID = "55555555-eeee-4eee-8eee-555555555555"
	)
	source := testServer(t)
	sourceRoot := t.TempDir()
	configurePortableTestServer(t, source, sourceRoot)
	if _, err := source.Database.CoreEntities().CreateCoser(ctx, productdb.CreateCoserInput{CreateNamedEntityInput: productdb.CreateNamedEntityInput{UUID: coserUUID, Name: "Alice"}}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Database.CoreEntities().CreateWork(ctx, productdb.CreateNamedEntityInput{UUID: workUUID, Name: "Incoming Work"}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Database.CoreEntities().CreateTag(ctx, productdb.CreateTagInput{CreateNamedEntityInput: productdb.CreateNamedEntityInput{UUID: tagUUID, Name: "Incoming Tag"}, UseInRecommendation: true}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Database.CoreEntities().CreateCoser(ctx, productdb.CreateCoserInput{CreateNamedEntityInput: productdb.CreateNamedEntityInput{UUID: reuseUUID, Name: "Exact Reuse"}}, now); err != nil {
		t.Fatal(err)
	}
	packagePath := filepath.Join(sourceRoot, "merge-source.zip")
	if _, err := source.ExportPortableMetadata(ctx, PortableExportOptions{TargetPath: packagePath}); err != nil {
		t.Fatal(err)
	}

	target := testServer(t)
	targetRoot := t.TempDir()
	configurePortableTestServer(t, target, targetRoot)
	if _, err := target.Database.CoreEntities().CreateCoser(ctx, productdb.CreateCoserInput{CreateNamedEntityInput: productdb.CreateNamedEntityInput{UUID: localUUID, Name: "alice"}}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := target.Database.CoreEntities().CreateWork(ctx, productdb.CreateNamedEntityInput{UUID: workUUID, Name: "Local Work"}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := target.Database.CoreEntities().CreateCoser(ctx, productdb.CreateCoserInput{CreateNamedEntityInput: productdb.CreateNamedEntityInput{UUID: tagUUID, Name: "Other Identity"}}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := target.Database.CoreEntities().CreateCoser(ctx, productdb.CreateCoserInput{CreateNamedEntityInput: productdb.CreateNamedEntityInput{UUID: reuseUUID, Name: "Exact Reuse"}}, now); err != nil {
		t.Fatal(err)
	}
	var beforeRegistry, beforeCore int
	if err := target.Database.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM portable_uuid_registry),(SELECT COUNT(*) FROM cosers)+(SELECT COUNT(*) FROM works)+(SELECT COUNT(*) FROM tags)`).Scan(&beforeRegistry, &beforeCore); err != nil {
		t.Fatal(err)
	}
	report, err := target.PreflightPortableMerge(ctx, packagePath)
	if err != nil {
		t.Fatal(err)
	}
	if report.IdentityAdd != 1 || report.IdentityReuse != 2 || report.EntityAdd != 2 || report.EntityReuse != 1 || report.HardBlockingCount() != 1 || report.ReviewCount() != 2 {
		t.Fatalf("unexpected merge preflight: %+v", report)
	}
	wanted := map[string]bool{
		"PORTABLE_UUID_KIND_CONFLICT":           false,
		"PORTABLE_CORE_NAME_MATCH_REVIEW":       false,
		"PORTABLE_CORE_ENTITY_CONTENT_CONFLICT": false,
	}
	for _, issue := range report.Issues {
		if _, ok := wanted[issue.Code]; ok {
			wanted[issue.Code] = true
		}
		if issue.IncomingUUID == "" || issue.LocalUUID == "" {
			t.Fatalf("merge issue leaked an unusable identity: %+v", issue)
		}
	}
	for code, found := range wanted {
		if !found {
			t.Fatalf("merge preflight omitted %s: %+v", code, report.Issues)
		}
	}
	var afterRegistry, afterCore, mergeSessions int
	if err := target.Database.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM portable_uuid_registry),(SELECT COUNT(*) FROM cosers)+(SELECT COUNT(*) FROM works)+(SELECT COUNT(*) FROM tags),(SELECT COUNT(*) FROM portable_import_sessions)`).Scan(&afterRegistry, &afterCore, &mergeSessions); err != nil {
		t.Fatal(err)
	}
	if afterRegistry != beforeRegistry || afterCore != beforeCore || mergeSessions != 0 {
		t.Fatalf("merge preflight changed business/import state: registry %d/%d core %d/%d sessions=%d", beforeRegistry, afterRegistry, beforeCore, afterCore, mergeSessions)
	}
	prepared, err := target.PreparePortableMerge(ctx, packagePath)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.State != "BLOCKED" || prepared.MergeID == "" || prepared.Report.PackageSHA256 != report.PackageSHA256 || prepared.Report.TargetFingerprint != report.TargetFingerprint {
		t.Fatalf("prepared merge=%+v", prepared)
	}
	var storedFormatVersion int
	if err := target.Database.QueryRowContext(ctx, `SELECT format_version FROM portable_merge_sessions WHERE merge_id=?`, prepared.MergeID).Scan(&storedFormatVersion); err != nil || storedFormatVersion != portablecatalog.FormatVersion {
		t.Fatalf("stored merge format version=%d err=%v", storedFormatVersion, err)
	}
	conflicts, err := target.Database.ListPortableMergeConflicts(ctx, prepared.MergeID)
	if err != nil || len(conflicts) != len(report.Issues) {
		t.Fatalf("stored conflicts=%+v err=%v", conflicts, err)
	}
	for _, conflict := range conflicts {
		if conflict.Decision != "UNRESOLVED" || conflict.IssueKey == "" {
			t.Fatalf("stored conflict is not unresolved and keyed: %+v", conflict)
		}
	}
	retained := filepath.Join(targetRoot, "backups", "portable-merges", prepared.MergeID, "package.zip")
	retainedBytes, err := os.ReadFile(retained)
	if err != nil {
		t.Fatal(err)
	}
	sourceBytes, err := os.ReadFile(packagePath)
	if err != nil || !bytes.Equal(retainedBytes, sourceBytes) {
		t.Fatalf("retained merge package mismatch: %v", err)
	}
}

func TestPortableMergeDecisionRejectsChangedTargetFingerprint(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 9, 18, 0, 0, 0, time.UTC)
	source := testServer(t)
	sourceRoot := t.TempDir()
	configurePortableTestServer(t, source, sourceRoot)
	if _, err := source.Database.CoreEntities().CreateCoser(ctx, productdb.CreateCoserInput{CreateNamedEntityInput: productdb.CreateNamedEntityInput{UUID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Name: "Same Name"}}, now); err != nil {
		t.Fatal(err)
	}
	packagePath := filepath.Join(sourceRoot, "review-only.zip")
	if _, err := source.ExportPortableMetadata(ctx, PortableExportOptions{TargetPath: packagePath}); err != nil {
		t.Fatal(err)
	}
	target := testServer(t)
	targetRoot := t.TempDir()
	configurePortableTestServer(t, target, targetRoot)
	if _, err := target.Database.CoreEntities().CreateCoser(ctx, productdb.CreateCoserInput{CreateNamedEntityInput: productdb.CreateNamedEntityInput{UUID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", Name: "same name"}}, now); err != nil {
		t.Fatal(err)
	}
	prepared, err := target.PreparePortableMerge(ctx, packagePath)
	if err != nil || prepared.State != "DECISIONS_PENDING" || prepared.Report.ReviewCount() != 1 {
		t.Fatalf("prepared=%+v err=%v", prepared, err)
	}
	conflicts, err := target.Database.ListPortableMergeConflicts(ctx, prepared.MergeID)
	if err != nil || len(conflicts) != 1 {
		t.Fatalf("conflicts=%+v err=%v", conflicts, err)
	}
	if _, err := target.Database.CoreEntities().CreateWork(ctx, productdb.CreateNamedEntityInput{Name: "Target changed after review"}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	decision := productdb.PortableMergeDecision{IssueKey: conflicts[0].IssueKey, Decision: "MAP_TO_LOCAL"}
	if err := target.SetPortableMergeDecisions(ctx, prepared.MergeID, []productdb.PortableMergeDecision{decision}); err == nil {
		t.Fatal("changed target fingerprint accepted stale portable merge decision")
	}
	stored, err := target.Database.ListPortableMergeConflicts(ctx, prepared.MergeID)
	if err != nil || stored[0].Decision != "UNRESOLVED" {
		t.Fatalf("stale decision changed stored conflict: %+v err=%v", stored, err)
	}
	session, err := target.Database.FindPortableMergeSession(ctx, prepared.MergeID)
	if err != nil || session.State != "STALE" {
		t.Fatalf("changed target did not stale merge session: %+v err=%v", session, err)
	}
}

func TestConflictFreePortableMergeCreatesSafetyBackupAndReservesGallery(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 9, 19, 0, 0, 0, time.UTC)
	source := testServer(t)
	sourceRoot := t.TempDir()
	configurePortableTestServer(t, source, sourceRoot)
	work, err := source.Database.CoreEntities().CreateWork(ctx, productdb.CreateNamedEntityInput{Name: "Merge Work"}, now)
	if err != nil {
		t.Fatal(err)
	}
	character, err := source.Database.CoreEntities().CreateCharacter(ctx, work.UUID, productdb.CreateNamedEntityInput{Name: "Merge Character"}, now)
	if err != nil {
		t.Fatal(err)
	}
	coser, err := source.Database.CoreEntities().CreateCoser(ctx, productdb.CreateCoserInput{CreateNamedEntityInput: productdb.CreateNamedEntityInput{Name: "Merge Coser"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	account, err := source.Database.CoreEntities().AddSocialAccount(ctx, coser.UUID, "x", "X", "merge", "https://example.test/merge", "ACTIVE", true, 1024, coser.MetadataRevision, now)
	if err != nil {
		t.Fatal(err)
	}
	tag, err := source.Database.CoreEntities().CreateTag(ctx, productdb.CreateTagInput{CreateNamedEntityInput: productdb.CreateNamedEntityInput{Name: "Merge Tag"}, UseInRecommendation: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	galleryValue, err := source.Database.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Merge Gallery"}, now)
	if err != nil {
		t.Fatal(err)
	}
	packagePath := filepath.Join(sourceRoot, "conflict-free-merge.zip")
	if _, err := source.ExportPortableMetadata(ctx, PortableExportOptions{TargetPath: packagePath, AllowIncompleteGallery: true}); err != nil {
		t.Fatal(err)
	}

	target := testServer(t)
	targetRoot := t.TempDir()
	configurePortableTestServer(t, target, targetRoot)
	if _, err := target.Database.CoreEntities().CreateCoser(ctx, productdb.CreateCoserInput{CreateNamedEntityInput: productdb.CreateNamedEntityInput{Name: "Existing Local Coser"}}, now); err != nil {
		t.Fatal(err)
	}
	prepared, err := target.PreparePortableMerge(ctx, packagePath)
	if err != nil || prepared.State != "READY" || len(prepared.Report.Issues) != 0 {
		t.Fatalf("prepared=%+v err=%v", prepared, err)
	}
	applied, err := target.ApplyConflictFreePortableMerge(ctx, prepared.MergeID)
	if err != nil {
		t.Fatal(err)
	}
	if applied.NewCoreIdentities != 5 || applied.ReusedIdentities != 0 || applied.PendingGalleryClaims != 1 || applied.SafetyBackupID == "" {
		t.Fatalf("applied=%+v", applied)
	}
	for table, uuid := range map[string]string{"works": work.UUID, "characters": character.UUID, "cosers": coser.UUID, "tags": tag.UUID, "coser_social_accounts": account.UUID} {
		column := "uuid"
		if table == "coser_social_accounts" {
			column = "account_uuid"
		}
		var count int
		if err := target.Database.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE `+column+`=?`, uuid).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s %s count=%d err=%v", table, uuid, count, err)
		}
	}
	var claimState, sessionState, backupStatus string
	if err := target.Database.QueryRowContext(ctx, `SELECT claim_state FROM portable_identity_claims WHERE import_id=? AND uuid=?`, prepared.MergeID, galleryValue.SetID).Scan(&claimState); err != nil || claimState != "PENDING" {
		t.Fatalf("Gallery claim=%q err=%v", claimState, err)
	}
	if err := target.Database.QueryRowContext(ctx, `SELECT state FROM portable_merge_sessions WHERE merge_id=?`, prepared.MergeID).Scan(&sessionState); err != nil || sessionState != "APPLIED" {
		t.Fatalf("merge state=%q err=%v", sessionState, err)
	}
	if err := target.Database.QueryRowContext(ctx, `SELECT status FROM backup_records WHERE backup_id=?`, applied.SafetyBackupID).Scan(&backupStatus); err != nil || backupStatus != "READY" {
		t.Fatalf("backup=%q err=%v", backupStatus, err)
	}
	if _, err := target.Database.UUIDRegistry().Register(ctx, galleryValue.SetID, portableid.KindGallery, now); err == nil {
		t.Fatal("pending Merge Gallery identity did not block ordinary UUID allocation")
	}
	if _, err := target.ApplyConflictFreePortableMerge(ctx, prepared.MergeID); err == nil {
		t.Fatal("applied portable merge was not idempotently refused")
	}
	repeated, err := target.PreflightPortableMerge(ctx, packagePath)
	if err != nil || repeated.HardBlockingCount() == 0 {
		t.Fatalf("pending Gallery claim was not reported on repeated merge preflight: %+v err=%v", repeated, err)
	}
}

func TestReviewedPortableMergeMapsIdentityAndUsesIncomingContent(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 9, 20, 0, 0, 0, time.UTC)
	const incomingCoser = "aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa"
	const localCoser = "bbbbbbbb-2222-4222-8222-bbbbbbbbbbbb"
	const workUUID = "cccccccc-3333-4333-8333-cccccccccccc"
	source := testServer(t)
	sourceRoot := t.TempDir()
	configurePortableTestServer(t, source, sourceRoot)
	if _, err := source.Database.CoreEntities().CreateCoser(ctx, productdb.CreateCoserInput{CreateNamedEntityInput: productdb.CreateNamedEntityInput{UUID: incomingCoser, Name: "Same Coser"}}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Database.CoreEntities().CreateWork(ctx, productdb.CreateNamedEntityInput{UUID: workUUID, Name: "Incoming Work"}, now); err != nil {
		t.Fatal(err)
	}
	packagePath := filepath.Join(sourceRoot, "reviewed.zip")
	if _, err := source.ExportPortableMetadata(ctx, PortableExportOptions{TargetPath: packagePath}); err != nil {
		t.Fatal(err)
	}

	target := testServer(t)
	configurePortableTestServer(t, target, t.TempDir())
	if _, err := target.Database.CoreEntities().CreateCoser(ctx, productdb.CreateCoserInput{CreateNamedEntityInput: productdb.CreateNamedEntityInput{UUID: localCoser, Name: "same coser"}}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := target.Database.CoreEntities().CreateWork(ctx, productdb.CreateNamedEntityInput{UUID: workUUID, Name: "Local Work"}, now); err != nil {
		t.Fatal(err)
	}
	prepared, err := target.PreparePortableMerge(ctx, packagePath)
	if err != nil || prepared.State != "DECISIONS_PENDING" || prepared.Report.ReviewCount() != 2 {
		t.Fatalf("prepared=%+v err=%v", prepared, err)
	}
	conflicts, err := target.Database.ListPortableMergeConflicts(ctx, prepared.MergeID)
	if err != nil {
		t.Fatal(err)
	}
	decisions := make([]productdb.PortableMergeDecision, 0, len(conflicts))
	for _, conflict := range conflicts {
		decision := "USE_INCOMING"
		if conflict.IssueCode == "PORTABLE_CORE_NAME_MATCH_REVIEW" {
			decision = "MAP_TO_LOCAL"
		}
		decisions = append(decisions, productdb.PortableMergeDecision{IssueKey: conflict.IssueKey, Decision: decision})
	}
	if err := target.SetPortableMergeDecisions(ctx, prepared.MergeID, decisions); err != nil {
		t.Fatal(err)
	}
	if _, err := target.ApplyPortableMerge(ctx, prepared.MergeID); err != nil {
		t.Fatal(err)
	}
	var workName string
	if err := target.Database.QueryRowContext(ctx, `SELECT name FROM works WHERE uuid=?`, workUUID).Scan(&workName); err != nil || workName != "Incoming Work" {
		t.Fatalf("work=%q err=%v", workName, err)
	}
	var targetUUID string
	if err := target.Database.QueryRowContext(ctx, `SELECT target_uuid FROM portable_uuid_aliases WHERE alias_uuid=?`, incomingCoser).Scan(&targetUUID); err != nil || targetUUID != localCoser {
		t.Fatalf("alias target=%q err=%v", targetUUID, err)
	}
	var incomingRows int
	if err := target.Database.QueryRowContext(ctx, `SELECT COUNT(*) FROM cosers WHERE uuid=?`, incomingCoser).Scan(&incomingRows); err != nil || incomingRows != 0 {
		t.Fatalf("mapped Coser rows=%d err=%v", incomingRows, err)
	}
}

func TestPortableMergePreflightProjectsCharacterScopeThroughWorkCandidate(t *testing.T) {
	const incomingWork = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	const localWork = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	const incomingCharacter = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	const localCharacter = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	named := func(uuid, name, slug string) portablecatalog.NamedEntity {
		return portablecatalog.NamedEntity{UUID: uuid, Name: name, Aliases: []string{}, Slug: slug, MetadataRevision: 1, CreatedAt: "2026-09-09T00:00:00Z", UpdatedAt: "2026-09-09T00:00:00Z"}
	}
	local := portablecatalog.CoreCatalog{Works: []portablecatalog.Work{{NamedEntity: named(localWork, "Same Work", "local-work")}}, Characters: []portablecatalog.Character{{NamedEntity: named(localCharacter, "Hero", "local-hero"), WorkUUID: localWork}}}
	incoming := portablecatalog.CoreCatalog{Works: []portablecatalog.Work{{NamedEntity: named(incomingWork, "same work", "incoming-work")}}, Characters: []portablecatalog.Character{{NamedEntity: named(incomingCharacter, "hero", "incoming-hero"), WorkUUID: incomingWork}}}
	report := PortableMergePreflightReport{}
	comparePortableCoreCatalog(local, incoming, &report)
	found := false
	for _, issue := range report.Issues {
		if issue.Code == "PORTABLE_CORE_NAME_MATCH_REVIEW" && issue.IncomingUUID == incomingCharacter && issue.LocalUUID == localCharacter {
			found = true
		}
	}
	if !found {
		t.Fatalf("projected Character identity review missing: %+v", report.Issues)
	}
}

func TestPortableMergePublishesAssetsForNewCoser(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 9, 21, 0, 0, 0, time.UTC)
	source := testServer(t)
	sourceRoot := t.TempDir()
	configurePortableTestServer(t, source, sourceRoot)
	coser, err := source.Database.CoreEntities().CreateCoser(ctx, productdb.CreateCoserInput{CreateNamedEntityInput: productdb.CreateNamedEntityInput{Name: "Asset Merge Coser"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	relative := "assets/avatar-11111111-1111-4111-8111-111111111111.png"
	directory := filepath.Join(sourceRoot, "cosers", coser.UUID, "assets")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	writePortableTestPNG(t, filepath.Join(sourceRoot, "cosers", coser.UUID, filepath.FromSlash(relative)))
	if _, err := source.Database.CoreEntities().SetCoserManagedAsset(ctx, coser.UUID, coser.MetadataRevision, productdb.CoserManagedAssetInput{Kind: productdb.CoserAssetAvatar, RelativePath: relative}, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	packagePath := filepath.Join(sourceRoot, "asset-merge.zip")
	if _, err := source.ExportPortableMetadata(ctx, PortableExportOptions{TargetPath: packagePath}); err != nil {
		t.Fatal(err)
	}

	target := testServer(t)
	targetRoot := t.TempDir()
	configurePortableTestServer(t, target, targetRoot)
	if _, err := target.Database.CoreEntities().CreateWork(ctx, productdb.CreateNamedEntityInput{Name: "Existing target data"}, now); err != nil {
		t.Fatal(err)
	}
	prepared, err := target.PreparePortableMerge(ctx, packagePath)
	if err != nil || prepared.State != "READY" {
		t.Fatalf("prepared=%+v err=%v", prepared, err)
	}
	if _, err := target.ApplyPortableMerge(ctx, prepared.MergeID); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := target.Database.QueryRowContext(ctx, `SELECT avatar_path FROM cosers WHERE uuid=?`, coser.UUID).Scan(&stored); err != nil || stored != relative {
		t.Fatalf("avatar=%q err=%v", stored, err)
	}
	if _, err := os.Stat(filepath.Join(targetRoot, "cosers", coser.UUID, filepath.FromSlash(relative))); err != nil {
		t.Fatalf("original asset missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetRoot, "cosers", coser.UUID, "assets", "avatar-11111111-1111-4111-8111-111111111111-480.jpg")); err != nil {
		t.Fatalf("derivative missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetRoot, "cosers", coser.UUID, portableImportOwnerMarker)); !os.IsNotExist(err) {
		t.Fatalf("merge ownership marker remained: %v", err)
	}
	marker := filepath.Join(targetRoot, "cosers", coser.UUID, portableImportOwnerMarker)
	if err := os.WriteFile(marker, []byte(prepared.MergeID+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := target.Database.Operations().SetMaintenance(ctx, productdb.MaintenancePortableMerging, prepared.MergeID, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := target.RecoverPortableMerge(ctx, prepared.MergeID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("merge recovery did not clear marker: %v", err)
	}
}

func TestPortableMergeCanOnlyAbortBeforeApply(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 9, 21, 30, 0, 0, time.UTC)
	source := testServer(t)
	sourceRoot := t.TempDir()
	configurePortableTestServer(t, source, sourceRoot)
	if _, err := source.Database.CoreEntities().CreateWork(ctx, productdb.CreateNamedEntityInput{Name: "Abort Work"}, now); err != nil {
		t.Fatal(err)
	}
	packagePath := filepath.Join(sourceRoot, "abort.zip")
	if _, err := source.ExportPortableMetadata(ctx, PortableExportOptions{TargetPath: packagePath}); err != nil {
		t.Fatal(err)
	}
	target := testServer(t)
	configurePortableTestServer(t, target, t.TempDir())
	if _, err := target.Database.CoreEntities().CreateCoser(ctx, productdb.CreateCoserInput{CreateNamedEntityInput: productdb.CreateNamedEntityInput{Name: "Existing"}}, now); err != nil {
		t.Fatal(err)
	}
	prepared, err := target.PreparePortableMerge(ctx, packagePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.AbortPortableMerge(ctx, prepared.MergeID); err != nil {
		t.Fatal(err)
	}
	if _, err := target.ApplyPortableMerge(ctx, prepared.MergeID); err == nil {
		t.Fatal("aborted merge was applied")
	}
	session, err := target.Database.FindPortableMergeSession(ctx, prepared.MergeID)
	if err != nil || session.State != "ABORTED" {
		t.Fatalf("session=%+v err=%v", session, err)
	}
}

func TestReviewedPortableMergeReplacesExistingCoserAssets(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 9, 21, 45, 0, 0, time.UTC)
	const coserUUID = "dddddddd-4444-4444-8444-dddddddddddd"
	makeCoser := func(t *testing.T, server *Server, root, relative string, colour color.NRGBA) {
		t.Helper()
		coser, err := server.Database.CoreEntities().CreateCoser(ctx, productdb.CreateCoserInput{CreateNamedEntityInput: productdb.CreateNamedEntityInput{UUID: coserUUID, Name: "Replace Asset"}}, now)
		if err != nil {
			t.Fatal(err)
		}
		directory := filepath.Join(root, "cosers", coserUUID, "assets")
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		value := image.NewNRGBA(image.Rect(0, 0, 2, 2))
		value.Set(0, 0, colour)
		file, err := os.OpenFile(filepath.Join(root, "cosers", coserUUID, filepath.FromSlash(relative)), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, value); err != nil {
			file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := server.Database.CoreEntities().SetCoserManagedAsset(ctx, coserUUID, coser.MetadataRevision, productdb.CoserManagedAssetInput{Kind: productdb.CoserAssetAvatar, RelativePath: relative}, now.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	source := testServer(t)
	sourceRoot := t.TempDir()
	configurePortableTestServer(t, source, sourceRoot)
	incomingRelative := "assets/avatar-aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa.png"
	makeCoser(t, source, sourceRoot, incomingRelative, color.NRGBA{R: 255, A: 255})
	packagePath := filepath.Join(sourceRoot, "replace-assets.zip")
	if _, err := source.ExportPortableMetadata(ctx, PortableExportOptions{TargetPath: packagePath}); err != nil {
		t.Fatal(err)
	}
	target := testServer(t)
	targetRoot := t.TempDir()
	configurePortableTestServer(t, target, targetRoot)
	oldRelative := "assets/avatar-bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb.png"
	makeCoser(t, target, targetRoot, oldRelative, color.NRGBA{B: 255, A: 255})
	prepared, err := target.PreparePortableMerge(ctx, packagePath)
	if err != nil || prepared.State != "DECISIONS_PENDING" {
		t.Fatalf("prepared=%+v err=%v", prepared, err)
	}
	conflicts, err := target.Database.ListPortableMergeConflicts(ctx, prepared.MergeID)
	if err != nil || len(conflicts) != 1 {
		t.Fatalf("conflicts=%+v err=%v", conflicts, err)
	}
	if err := target.SetPortableMergeDecisions(ctx, prepared.MergeID, []productdb.PortableMergeDecision{{IssueKey: conflicts[0].IssueKey, Decision: "USE_INCOMING"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := target.ApplyPortableMerge(ctx, prepared.MergeID); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := target.Database.QueryRowContext(ctx, `SELECT avatar_path FROM cosers WHERE uuid=?`, coserUUID).Scan(&stored); err != nil || stored != incomingRelative {
		t.Fatalf("avatar=%q err=%v", stored, err)
	}
	if _, err := os.Stat(filepath.Join(targetRoot, "cosers", coserUUID, filepath.FromSlash(incomingRelative))); err != nil {
		t.Fatalf("incoming asset missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetRoot, "cosers", coserUUID, filepath.FromSlash(oldRelative))); !os.IsNotExist(err) {
		t.Fatalf("old active asset directory was not replaced: %v", err)
	}
}

func TestPortableMergeRebuildsGalleryAtMappedRoot(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 9, 22, 0, 0, 0, time.UTC)
	source := testServer(t)
	sourceRoot := t.TempDir()
	configurePortableTestServer(t, source, sourceRoot)
	mediaRoot := filepath.Join(sourceRoot, "old-media")
	setPath := filepath.Join(mediaRoot, "set")
	if err := os.MkdirAll(setPath, 0o700); err != nil {
		t.Fatal(err)
	}
	writePortableTestPNG(t, filepath.Join(setPath, "photo.png"))
	library, err := source.Database.Libraries().Create(ctx, productdb.CreateLibraryInput{Name: "Logical merge library", RootPath: mediaRoot, Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	galleryValue, err := source.Database.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Merge Rebuild", ContentRating: gallery.ContentRatingNonAdult}, now)
	if err != nil {
		t.Fatal(err)
	}
	sourceValue, err := source.Database.Galleries().AddSource(ctx, galleryValue.ID, productdb.CreateSourceInput{LibraryID: &library.ID, Type: gallery.SourceTypeDirectory, Path: setPath, Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.Database.Scans().Run(ctx, sourceValue.ID, archivecheck.DefaultLimits(), now); err != nil {
		t.Fatal(err)
	}
	var itemUUID string
	if err := source.Database.QueryRowContext(ctx, `SELECT item_uuid FROM gallery_items WHERE gallery_id=?`, galleryValue.ID).Scan(&itemUUID); err != nil {
		t.Fatal(err)
	}
	current, _ := source.Database.Galleries().Find(ctx, galleryValue.ID)
	if _, err := source.Database.Manifests().PushGallery(ctx, galleryValue.ID, current.MetadataRevision, now); err != nil {
		t.Fatal(err)
	}
	packagePath := filepath.Join(sourceRoot, "merge-rebuild.zip")
	if _, err := source.ExportPortableMetadata(ctx, PortableExportOptions{TargetPath: packagePath}); err != nil {
		t.Fatal(err)
	}

	target := testServer(t)
	targetRoot := t.TempDir()
	configurePortableTestServer(t, target, targetRoot)
	if _, err := target.Database.CoreEntities().CreateWork(ctx, productdb.CreateNamedEntityInput{Name: "Existing target catalog"}, now); err != nil {
		t.Fatal(err)
	}
	newMedia := filepath.Join(targetRoot, "new-media")
	newSet := filepath.Join(newMedia, "set")
	if err := os.MkdirAll(newSet, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"photo.png", ".cosplay.json"} {
		data, err := os.ReadFile(filepath.Join(setPath, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(newSet, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	targetLibrary, err := target.Database.Libraries().Create(ctx, productdb.CreateLibraryInput{Name: "Mapped merge library", RootPath: newMedia, Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := target.PreparePortableMerge(ctx, packagePath)
	if err != nil || prepared.State != "READY" {
		t.Fatalf("prepared=%+v err=%v", prepared, err)
	}
	if _, err := target.ApplyPortableMerge(ctx, prepared.MergeID); err != nil {
		t.Fatal(err)
	}
	mappings, err := target.Database.ListPortableLibraryMappings(ctx, prepared.MergeID)
	if err != nil || len(mappings) != 1 {
		t.Fatalf("mappings=%+v err=%v", mappings, err)
	}
	if err := target.MapPortableLibraries(ctx, prepared.MergeID, []productdb.PortableLibraryDecision{{LibraryKey: mappings[0].LibraryKey, TargetLibraryID: &targetLibrary.ID}}); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := target.RebuildPortableGalleries(ctx, prepared.MergeID)
	if err != nil || rebuilt.Rebuilt != 1 {
		t.Fatalf("rebuilt=%+v err=%v", rebuilt, err)
	}
	var gotItem, state string
	if err := target.Database.QueryRowContext(ctx, `SELECT item.item_uuid,gallery.state FROM gallery_items item JOIN galleries gallery ON gallery.id=item.gallery_id WHERE gallery.set_id=?`, galleryValue.SetID).Scan(&gotItem, &state); err != nil {
		t.Fatal(err)
	}
	if gotItem != itemUUID || state != "DRAFT" {
		t.Fatalf("rebuilt item/state=%q/%q", gotItem, state)
	}
	var importState string
	if err := target.Database.QueryRowContext(ctx, `SELECT state FROM portable_import_sessions WHERE import_id=?`, prepared.MergeID).Scan(&importState); err != nil || importState != "GALLERIES_REBUILT" {
		t.Fatalf("rebuild workflow=%q err=%v", importState, err)
	}
}

func configurePortableTestServer(t *testing.T, server *Server, root string) {
	t.Helper()
	coserRoot := filepath.Join(root, "cosers")
	if err := os.MkdirAll(coserRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Database.ExecContext(context.Background(), `UPDATE product_setup SET complete=1,coser_metadata_root=?,backup_root=?,completed_at_utc=? WHERE id=1`, coserRoot, filepath.Join(root, "backups"), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
}

func writePortableTestPNG(t *testing.T, path string) {
	t.Helper()
	var encoded bytes.Buffer
	value := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	value.Set(0, 0, color.NRGBA{R: 255, G: 64, A: 255})
	if err := png.Encode(&encoded, value); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writePortableTestZIP(t *testing.T, path string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("folder/photo.jpg")
	if err == nil {
		_, err = entry.Write([]byte("\xff\xd8\xff portable archive image"))
	}
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
}
