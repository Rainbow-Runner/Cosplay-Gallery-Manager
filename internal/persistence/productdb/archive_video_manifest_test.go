package productdb

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/archivecheck"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/manifest"
	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/portablecatalog"
	"github.com/stashapp/stash/internal/product"
)

func TestArchiveVideoManifestAndPortableCatalogExcludeLocalAccessEvidence(t *testing.T) {
	ctx, now := context.Background(), time.Now().UTC()
	db, _ := openTestDatabaseAndRegistry(t)
	filename := filepath.Join(t.TempDir(), "video.zip")
	var body bytes.Buffer
	w := zip.NewWriter(&body)
	part, err := w.CreateHeader(&zip.FileHeader{Name: "video/clip.mp4", Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	part.Write(append([]byte("\x00\x00\x00\x18ftypisom"), make([]byte, 20)...))
	w.Close()
	if err := os.WriteFile(filename, body.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	g, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Portable video", ContentRating: gallery.ContentRatingNonAdult}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, g.ID, CreateSourceInput{Type: gallery.SourceTypeArchive, Path: filename, Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().RunWithOptions(ctx, source.ID, archivecheck.DefaultLimits(), ScanOptions{}, now); err != nil {
		t.Fatal(err)
	}
	var uuid string
	var revision int64
	if err := db.QueryRowContext(ctx, `SELECT item_uuid,content_revision FROM gallery_items WHERE gallery_id=?`, g.ID).Scan(&uuid, &revision); err != nil {
		t.Fatal(err)
	}
	if err := db.VideoMetadata().PublishReady(ctx, mediaprocessing.VideoTechnicalMetadata{ItemUUID: uuid, ContentRevision: revision, ProbeProfileHash: mediaprocessing.DefaultProfileHash(), Container: "mp4", VideoCodec: "h264", DisplayWidth: 64, DisplayHeight: 48}, now); err != nil {
		t.Fatal(err)
	}
	current, _ := db.Galleries().Find(ctx, g.ID)
	state, err := db.Manifests().PushGallery(ctx, g.ID, current.MetadataRevision, now)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _, err := manifest.ReadFile(state.Path, manifest.MaxGalleryBytes)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := manifest.ParseGallery(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Items.Value) != 1 || doc.Items.Value[0].ItemUUID != uuid || doc.Items.Value[0].Path != "video/clip.mp4" || doc.SetID != g.SetID {
		t.Fatalf("portable identities %#v", doc)
	}
	packageManifest := portablecatalog.PackageManifest{Format: portablecatalog.Format, FormatVersion: portablecatalog.FormatVersion, ProductID: product.ID, ExportID: "77777777-7777-4777-8777-777777777777", CreatedAt: now.Format(time.RFC3339), Versions: portablecatalog.Versions(product.CurrentVersions("1.5.0-test"))}
	snapshot, err := db.PortableCatalogSnapshot(ctx, packageManifest)
	if err != nil {
		t.Fatal(err)
	}
	portableBytes, err := json.Marshal(snapshot.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range [][]byte{encoded, portableBytes} {
		for _, forbidden := range []string{"container_size", "container_modified_at_utc", "probe_profile_hash", "probe_state", "direct_offset", "http://127.0.0.1", filename} {
			if strings.Contains(string(content), forbidden) {
				t.Fatalf("local evidence leaked: %s", forbidden)
			}
		}
	}
}
