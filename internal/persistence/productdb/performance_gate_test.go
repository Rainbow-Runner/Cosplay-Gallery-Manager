package productdb

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/portablecatalog"
	"github.com/stashapp/stash/internal/product"
)

const (
	performanceGalleryCount = 10_000
	performanceItemCount    = 1_000_000
	performanceIterations   = 20
)

func TestPerformanceReleaseGate(t *testing.T) {
	if os.Getenv("CGM_PERFORMANCE_GATE") != "1" {
		t.Skip("set CGM_PERFORMANCE_GATE=1 to build and measure the release-scale synthetic database")
	}
	ctx := context.Background()
	root := t.TempDir()
	database, err := Open(ctx, filepath.Join(root, "performance.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.ExecContext(ctx, `PRAGMA synchronous=OFF`); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	populatePerformanceFixture(t, database)
	if _, err := database.ExecContext(ctx, `PRAGMA optimize`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(database.Path())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("fixture galleries=%d items=%d database=%d bytes build=%s",
		performanceGalleryCount, performanceItemCount, info.Size(), time.Since(started).Round(time.Millisecond))

	runtime.GC()
	var memoryBefore, memoryAfter runtime.MemStats
	runtime.ReadMemStats(&memoryBefore)
	preflightStarted := time.Now()
	preflight, err := database.PortableCatalogPreflight(ctx)
	preflightDuration := time.Since(preflightStarted)
	runtime.ReadMemStats(&memoryAfter)
	if err != nil {
		t.Fatal(err)
	}
	expectedIdentities := performanceItemCount + performanceGalleryCount + 400
	if preflight.IdentityCount != expectedIdentities || preflight.GalleryCount != performanceGalleryCount {
		t.Fatalf("portable preflight counts = %d identities/%d galleries", preflight.IdentityCount, preflight.GalleryCount)
	}
	if preflight.BlockingCount() != 0 {
		t.Fatalf("portable preflight blocking findings = %d", preflight.BlockingCount())
	}
	allocated := memoryAfter.TotalAlloc - memoryBefore.TotalAlloc
	if preflightDuration > 5*time.Second {
		t.Fatalf("portable preflight = %s, target <= 5s", preflightDuration)
	}
	if allocated > 64*1024*1024 {
		t.Fatalf("portable preflight allocated %d bytes, target <= 64 MiB", allocated)
	}
	t.Logf("portable-preflight=%s allocated=%d bytes target<=5s/64MiB", preflightDuration.Round(time.Millisecond), allocated)

	type portableRoundTripResult struct {
		count               int
		identityPayloadSize uint64
		err                 error
	}
	runtime.GC()
	runtime.ReadMemStats(&memoryBefore)
	peakHeap := memoryBefore.HeapAlloc
	roundTripStarted := time.Now()
	roundTripDone := make(chan portableRoundTripResult, 1)
	go func() {
		manifest := portablecatalog.PackageManifest{Format: portablecatalog.Format, FormatVersion: portablecatalog.FormatVersion, ProductID: product.ID, ExportID: "90000000-0000-4000-8000-000000000001", CreatedAt: "2026-09-08T00:00:00Z", Versions: portablecatalog.Versions(product.CurrentVersions("1.5.0-test"))}
		reader, readErr := database.BeginPortableCatalogRead(ctx, manifest)
		if readErr != nil {
			roundTripDone <- portableRoundTripResult{err: readErr}
			return
		}
		defer reader.Close()
		target := filepath.Join(root, "million-identities.cgm-portable.zip")
		source := portablecatalog.IdentitySource{Count: reader.IdentityCount(), Stream: func(yield func(portablecatalog.IdentityRecord) error) error {
			return reader.StreamIdentities(ctx, yield)
		}}
		if writeErr := portablecatalog.WriteFileAtomicStreaming(target, reader.Snapshot().Bundle, nil, source); writeErr != nil {
			roundTripDone <- portableRoundTripResult{err: writeErr}
			return
		}
		if commitErr := reader.Commit(); commitErr != nil {
			roundTripDone <- portableRoundTripResult{err: commitErr}
			return
		}
		inspection, inspectErr := portablecatalog.InspectFile(ctx, target)
		if inspectErr != nil {
			roundTripDone <- portableRoundTripResult{err: inspectErr}
			return
		}
		archive, openErr := zip.OpenReader(target)
		if openErr != nil {
			roundTripDone <- portableRoundTripResult{err: openErr}
			return
		}
		var identityPayloadSize uint64
		for _, entry := range archive.File {
			if entry.Name == "identity-ledger.json" {
				identityPayloadSize = entry.UncompressedSize64
				break
			}
		}
		_ = archive.Close()
		roundTripDone <- portableRoundTripResult{count: inspection.Manifest.IdentityCount, identityPayloadSize: identityPayloadSize}
	}()
	ticker := time.NewTicker(20 * time.Millisecond)
	var roundTrip portableRoundTripResult
	for waiting := true; waiting; {
		select {
		case roundTrip = <-roundTripDone:
			waiting = false
		case <-ticker.C:
			runtime.ReadMemStats(&memoryAfter)
			if memoryAfter.HeapAlloc > peakHeap {
				peakHeap = memoryAfter.HeapAlloc
			}
		}
	}
	ticker.Stop()
	if roundTrip.err != nil {
		t.Fatal(roundTrip.err)
	}
	if roundTrip.count != expectedIdentities {
		t.Fatalf("portable round trip identities = %d", roundTrip.count)
	}
	if roundTrip.identityPayloadSize <= 120*1024*1024 {
		t.Fatalf("portable identity payload = %d bytes; expected a release-scale payload above 120 MiB", roundTrip.identityPayloadSize)
	}
	roundTripDuration := time.Since(roundTripStarted)
	peakGrowth := peakHeap - memoryBefore.HeapAlloc
	if roundTripDuration > 90*time.Second {
		t.Fatalf("portable million-identity round trip = %s, target <= 90s", roundTripDuration)
	}
	if peakGrowth > 128*1024*1024 {
		t.Fatalf("portable million-identity peak Go heap growth = %d bytes, target <= 128 MiB", peakGrowth)
	}
	t.Logf("portable-million-round-trip=%s identity-payload=%d bytes peak-go-heap-growth=%d bytes target<=90s/128MiB", roundTripDuration.Round(time.Millisecond), roundTrip.identityPayloadSize, peakGrowth)

	store := database.Browse()
	setID := performanceUUID("00000000", 1)
	manifestRoot := filepath.Join(root, "manifest-gallery")
	if err := os.MkdirAll(manifestRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE gallery_sources SET source_path=? WHERE gallery_id=1`, manifestRoot); err != nil {
		t.Fatal(err)
	}
	var revision int64
	if err := database.QueryRowContext(ctx, `SELECT metadata_revision FROM galleries WHERE id=1`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Manifests().PushGallery(ctx, 1, revision, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE galleries SET title='Manifest dirty title',metadata_revision=metadata_revision+1 WHERE id=1`); err != nil {
		t.Fatal(err)
	}

	tagColdStarted := time.Now()
	if _, err := store.TagDetail(ctx, "tag-1", browse.ScopeList, 1); err != nil {
		t.Fatal(err)
	}
	tagCold := time.Since(tagColdStarted)
	if tagCold > 800*time.Millisecond {
		t.Fatalf("tag detail uncached = %s, target <= 800ms", tagCold)
	}
	t.Logf("tag-detail-uncached=%s target<=800ms", tagCold.Round(time.Microsecond))

	gates := []struct {
		name      string
		threshold time.Duration
		run       func() error
	}{
		{name: "home", threshold: 500 * time.Millisecond, run: func() error {
			page, _, err := store.Home(ctx, 1)
			if err == nil && len(page.Items) != 24 {
				return fmt.Errorf("home items = %d", len(page.Items))
			}
			return err
		}},
		{name: "gallery-list", threshold: 500 * time.Millisecond, run: func() error {
			page, err := store.Galleries(ctx, browse.ScopeList, 200, browse.GallerySortRecentlyAdded)
			if err == nil && len(page.Items) != 24 {
				return fmt.Errorf("gallery list items = %d", len(page.Items))
			}
			return err
		}},
		{name: "gallery-detail", threshold: 500 * time.Millisecond, run: func() error {
			_, err := store.GalleryDetailBySlug(ctx, browse.ScopeList, "gallery-1")
			return err
		}},
		{name: "member-index-100", threshold: 500 * time.Millisecond, run: func() error {
			index, err := store.GalleryMemberIndex(ctx, setID, browse.ScopeList)
			if err == nil && len(index.Items) != 100 {
				return fmt.Errorf("member index items = %d", len(index.Items))
			}
			return err
		}},
		{name: "entity-index", threshold: 500 * time.Millisecond, run: func() error {
			page, err := store.EntityIndex(ctx, browse.SearchCoser, browse.ScopeList, 1, browse.EntitySortName)
			if err == nil && len(page.Items) == 0 {
				return errors.New("entity index is empty")
			}
			return err
		}},
		{name: "search", threshold: 500 * time.Millisecond, run: func() error {
			result, err := store.SearchPreview(ctx, browse.ScopeList, "Gallery 09999")
			if err == nil && len(result.Galleries) == 0 {
				return errors.New("search result is empty")
			}
			return err
		}},
		{name: "timeline", threshold: 500 * time.Millisecond, run: func() error {
			page, err := store.Timeline(ctx, browse.ScopeList, 200, "")
			if err == nil && len(page.Items) != 24 {
				return fmt.Errorf("timeline items = %d", len(page.Items))
			}
			return err
		}},
		{name: "strong-recommendations", threshold: 500 * time.Millisecond, run: func() error {
			result, err := store.StrongRecommendations(ctx, setID, browse.ScopeList)
			if err == nil && len(result) == 0 {
				return errors.New("strong recommendations are empty")
			}
			return err
		}},
		{name: "tag-recommendations", threshold: 800 * time.Millisecond, run: func() error {
			result, err := store.TagRecommendations(ctx, setID, browse.ScopeList)
			if err == nil && len(result) == 0 {
				return errors.New("tag recommendations are empty")
			}
			return err
		}},
		{name: "tag-detail-cached", threshold: 300 * time.Millisecond, run: func() error {
			_, err := store.TagDetail(ctx, "tag-1", browse.ScopeList, 1)
			return err
		}},
		{name: "random", threshold: 800 * time.Millisecond, run: func() error {
			result, err := store.RandomItems(ctx, browse.ScopeList, browse.RandomAll)
			if err == nil && len(result) != 24 {
				return fmt.Errorf("random items = %d", len(result))
			}
			return err
		}},
		{name: "manifest-diff", threshold: time.Second, run: func() error {
			_, err := database.Manifests().CheckGallery(ctx, 1, time.Now())
			return err
		}},
	}
	for _, gate := range gates {
		t.Run(gate.name, func(t *testing.T) {
			p95 := measureP95(t, gate.run)
			t.Logf("p95=%s target<=%s", p95.Round(time.Microsecond), gate.threshold)
			if p95 > gate.threshold {
				t.Fatalf("%s p95 = %s, target <= %s", gate.name, p95, gate.threshold)
			}
		})
	}
}

func measureP95(t *testing.T, run func() error) time.Duration {
	t.Helper()
	for index := 0; index < 3; index++ {
		if err := run(); err != nil {
			t.Fatal(err)
		}
	}
	values := make([]time.Duration, performanceIterations)
	for index := range values {
		started := time.Now()
		if err := run(); err != nil {
			t.Fatal(err)
		}
		values[index] = time.Since(started)
	}
	sort.Slice(values, func(left, right int) bool { return values[left] < values[right] })
	return values[(95*len(values)+99)/100-1]
}

func populatePerformanceFixture(t *testing.T, database *Database) {
	t.Helper()
	ctx := context.Background()
	statements := []string{
		`WITH RECURSIVE n(value) AS (SELECT 1 UNION ALL SELECT value+1 FROM n WHERE value<10000)
		 INSERT INTO portable_uuid_registry(uuid,entity_kind,created_at_utc)
		 SELECT printf('00000000-0000-4000-8000-%012d',value),'GALLERY','2026-01-01T00:00:00Z' FROM n`,
		`WITH RECURSIVE n(value) AS (SELECT 1 UNION ALL SELECT value+1 FROM n WHERE value<100)
		 INSERT INTO portable_uuid_registry(uuid,entity_kind,created_at_utc)
		 SELECT printf('20000000-0000-4000-8000-%012d',value),'COSER','2026-01-01T00:00:00Z' FROM n`,
		`WITH RECURSIVE n(value) AS (SELECT 1 UNION ALL SELECT value+1 FROM n WHERE value<100)
		 INSERT INTO portable_uuid_registry(uuid,entity_kind,created_at_utc)
		 SELECT printf('30000000-0000-4000-8000-%012d',value),'TAG','2026-01-01T00:00:00Z' FROM n`,
		`WITH RECURSIVE n(value) AS (SELECT 1 UNION ALL SELECT value+1 FROM n WHERE value<100)
		 INSERT INTO portable_uuid_registry(uuid,entity_kind,created_at_utc)
		 SELECT printf('40000000-0000-4000-8000-%012d',value),'WORK','2026-01-01T00:00:00Z' FROM n`,
		`WITH RECURSIVE n(value) AS (SELECT 1 UNION ALL SELECT value+1 FROM n WHERE value<100)
		 INSERT INTO portable_uuid_registry(uuid,entity_kind,created_at_utc)
		 SELECT printf('50000000-0000-4000-8000-%012d',value),'CHARACTER','2026-01-01T00:00:00Z' FROM n`,
		`WITH RECURSIVE d(value) AS (SELECT 0 UNION ALL SELECT value+1 FROM d WHERE value<999),
		 n(value) AS (SELECT left_digit.value*1000+right_digit.value+1 FROM d left_digit CROSS JOIN d right_digit)
		 INSERT INTO portable_uuid_registry(uuid,entity_kind,created_at_utc)
		 SELECT printf('10000000-0000-4000-8000-%012d',value),'GALLERY_ITEM','2026-01-01T00:00:00Z' FROM n`,
		`WITH RECURSIVE n(value) AS (SELECT 1 UNION ALL SELECT value+1 FROM n WHERE value<100)
		 INSERT INTO cosers(uuid,name,sort_name,slug,created_at_utc,updated_at_utc)
		 SELECT printf('20000000-0000-4000-8000-%012d',value),printf('Coser %03d',value),
		 printf('Coser %03d',value),printf('coser-%d',value),'2026-01-01T00:00:00Z','2026-01-01T00:00:00Z' FROM n`,
		`WITH RECURSIVE n(value) AS (SELECT 1 UNION ALL SELECT value+1 FROM n WHERE value<100)
		 INSERT INTO tags(uuid,name,normalized_name,sort_name,slug,created_at_utc,updated_at_utc)
		 SELECT printf('30000000-0000-4000-8000-%012d',value),printf('Tag %03d',value),
		 printf('tag %03d',value),printf('Tag %03d',value),printf('tag-%d',value),
		 '2026-01-01T00:00:00Z','2026-01-01T00:00:00Z' FROM n`,
		`WITH RECURSIVE n(value) AS (SELECT 1 UNION ALL SELECT value+1 FROM n WHERE value<100)
		 INSERT INTO works(uuid,name,sort_name,slug,created_at_utc,updated_at_utc)
		 SELECT printf('40000000-0000-4000-8000-%012d',value),printf('Work %03d',value),
		 printf('Work %03d',value),printf('work-%d',value),'2026-01-01T00:00:00Z','2026-01-01T00:00:00Z' FROM n`,
		`WITH RECURSIVE n(value) AS (SELECT 1 UNION ALL SELECT value+1 FROM n WHERE value<100)
		 INSERT INTO characters(uuid,work_uuid,name,normalized_name,sort_name,slug,created_at_utc,updated_at_utc)
		 SELECT printf('50000000-0000-4000-8000-%012d',value),printf('40000000-0000-4000-8000-%012d',value),
		 printf('Character %03d',value),printf('character %03d',value),printf('Character %03d',value),printf('character-%d',value),
		 '2026-01-01T00:00:00Z','2026-01-01T00:00:00Z' FROM n`,
		`WITH RECURSIVE n(value) AS (SELECT 1 UNION ALL SELECT value+1 FROM n WHERE value<10000)
		 INSERT INTO galleries(id,set_id,slug,state,title,shoot_date,shoot_date_precision,content_rating,
		 created_at_utc,updated_at_utc,added_at_utc)
		 SELECT value,printf('00000000-0000-4000-8000-%012d',value),printf('gallery-%d',value),'ACTIVE',
		 printf('Gallery %05d Needle',value),printf('2025-%02d-%02d',1+(value%12),1+(value%27)),'DAY','NON_ADULT',
		 '2026-01-01T00:00:00Z','2026-01-01T00:00:00Z',
		 printf('2026-01-%02dT00:00:00Z',1+(value%27)) FROM n`,
		`WITH RECURSIVE n(value) AS (SELECT 1 UNION ALL SELECT value+1 FROM n WHERE value<10000)
		 INSERT INTO gallery_sources(id,gallery_id,source_type,source_path,availability_state,reconcile_state,created_at_utc,updated_at_utc)
		 SELECT value,value,'DIRECTORY',printf('/synthetic/library/gallery-%d',value),'AVAILABLE','IN_SYNC',
		 '2026-01-01T00:00:00Z','2026-01-01T00:00:00Z' FROM n`,
		`WITH RECURSIVE n(value) AS (SELECT 1 UNION ALL SELECT value+1 FROM n WHERE value<10000)
		 INSERT INTO gallery_credits(id,gallery_id,coser_uuid,position)
		 SELECT value,value,printf('20000000-0000-4000-8000-%012d',1+((value-1)%100)),1024 FROM n`,
		`WITH RECURSIVE n(value) AS (SELECT 1 UNION ALL SELECT value+1 FROM n WHERE value<10000)
		 INSERT INTO gallery_cast(id,gallery_id,gallery_credit_id,character_uuid,position)
		 SELECT value,value,value,printf('50000000-0000-4000-8000-%012d',1+((value-1)%100)),1024 FROM n`,
		`WITH RECURSIVE n(value) AS (SELECT 1 UNION ALL SELECT value+1 FROM n WHERE value<10000)
		 INSERT INTO gallery_tags(gallery_id,tag_uuid,position)
		 SELECT value,printf('30000000-0000-4000-8000-%012d',1+((value-1)%100)),1024 FROM n`,
		`WITH RECURSIVE n(value) AS (SELECT 1 UNION ALL SELECT value+1 FROM n WHERE value<9)
		 INSERT INTO tag_edges(parent_uuid,child_uuid,position)
		 SELECT printf('30000000-0000-4000-8000-%012d',value),
		 printf('30000000-0000-4000-8000-%012d',value+1),1024 FROM n`,
		`WITH RECURSIVE d(value) AS (SELECT 0 UNION ALL SELECT value+1 FROM d WHERE value<999),
		 n(value) AS (SELECT left_digit.value*1000+right_digit.value+1 FROM d left_digit CROSS JOIN d right_digit)
		 INSERT INTO gallery_items(id,item_uuid,gallery_id,source_id,relative_path,media_kind,content_format,image_category,
		 position,availability_state,processing_state,byte_size,quick_fingerprint,full_fingerprint,created_at_utc,updated_at_utc)
		 SELECT value,printf('10000000-0000-4000-8000-%012d',value),1+((value-1)/100),1+((value-1)/100),
		 printf('photo-%03d.jpg',1+((value-1)%100)),'STATIC_IMAGE','IMAGE','PHOTO',(1+((value-1)%100))*1024,
		 'AVAILABLE','READY',1048576,printf('quick-%d',value),printf('full-%d',value),
		 '2026-01-01T00:00:00Z','2026-01-01T00:00:00Z' FROM n`,
		`WITH RECURSIVE n(value) AS (SELECT 1 UNION ALL SELECT value+1 FROM n WHERE value<10000)
		 INSERT INTO media_derivatives(id,item_uuid,variant,cache_tier,content_revision,profile_hash,state,is_current,
		 cache_relative_path,mime_type,byte_size,width,height,created_at_utc,last_accessed_at_utc)
		 SELECT value,printf('10000000-0000-4000-8000-%012d',(value-1)*100+1),'CARD_480','BASE',1,'performance-profile',
		 'READY',1,printf('items/%d/card.jpg',value),'image/jpeg',4096,480,320,
		 '2026-01-01T00:00:00Z','2026-01-01T00:00:00Z' FROM n`,
	}
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for index, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			t.Fatalf("fixture statement %d: %v", index+1, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func performanceUUID(prefix string, value int) string {
	return fmt.Sprintf("%s-0000-4000-8000-%012d", prefix, value)
}
