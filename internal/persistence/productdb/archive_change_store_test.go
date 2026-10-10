package productdb

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/archivecheck"
	"github.com/stashapp/stash/internal/gallery"
)

func TestLibraryDiscoveryQueuesSamePathArchiveReplacement(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 30, 5, 0, 0, 0, time.UTC)
	root := t.TempDir()
	library, err := db.Libraries().Create(ctx, CreateLibraryInput{Name: "Replacement", RootPath: root, Enabled: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "set.cbz")
	writeArchive := func(body string, modified time.Time) {
		t.Helper()
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		writer := zip.NewWriter(file)
		part, err := writer.CreateHeader(&zip.FileHeader{Name: "photo.jpg", Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte("\xff\xd8\xff " + body)); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	writeArchive("first", now)
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: "Replacement"}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{
		LibraryID: &library.ID, Type: gallery.SourceTypeArchive, Path: path, Availability: gallery.AvailabilityAvailable,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Scans().Run(ctx, source.ID, archivecheck.DefaultLimits(), now); err != nil {
		t.Fatal(err)
	}
	before := loadGalleryItemsForTest(t, db, created.ID)
	if len(before) != 1 {
		t.Fatalf("items before replacement = %d", len(before))
	}
	if _, err := db.CandidateDiscovery().DiscoverFilesystem(ctx, library.ID, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	assertArchiveTargets(t, db, ctx, nil)
	writeArchive("second-longer", now.Add(2*time.Minute))
	if _, err := db.CandidateDiscovery().DiscoverFilesystem(ctx, library.ID, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	assertArchiveTargets(t, db, ctx, []int64{source.ID})
	if err := db.Scans().Run(ctx, source.ID, archivecheck.DefaultLimits(), now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	assertArchiveTargets(t, db, ctx, nil)
	after := loadGalleryItemsForTest(t, db, created.ID)
	if len(after) != 1 || after[0].FullFingerprint == before[0].FullFingerprint || after[0].ContentRevision != before[0].ContentRevision+1 {
		t.Fatalf("replacement was not reconciled: before=%#v after=%#v", before, after)
	}
}

func assertArchiveTargets(t *testing.T, db *Database, ctx context.Context, want []int64) {
	t.Helper()
	got, err := db.ChangedArchiveSourceTargets(ctx, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("changed archive targets = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("changed archive targets = %v, want %v", got, want)
		}
	}
}
