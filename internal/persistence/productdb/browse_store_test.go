package productdb

import (
	"context"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
)

func TestBrowseGalleryCardScopeCountsRelationsAndOpaqueCover(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 18, 0, 0, 0, time.UTC)
	cardGallery, source := createBrowseGallery(t, db, "Cosplay", gallery.ContentRatingNonAdult, now)

	coser, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Coser A"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE cosers SET avatar_path='assets/avatar.webp',metadata_revision=2 WHERE uuid=?`, coser.UUID); err != nil {
		t.Fatal(err)
	}
	work, err := db.CoreEntities().CreateWork(ctx, CreateNamedEntityInput{Name: "Work A"}, now)
	if err != nil {
		t.Fatal(err)
	}
	character, err := db.CoreEntities().CreateCharacter(ctx, work.UUID, CreateNamedEntityInput{Name: "Character A"}, now)
	if err != nil {
		t.Fatal(err)
	}
	credit, err := db.Galleries().AddCredit(ctx, cardGallery.ID, coser.UUID, 1024, cardGallery.MetadataRevision, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Galleries().AddCast(ctx, cardGallery.ID, credit, character.UUID, 1024, cardGallery.MetadataRevision+1, now); err != nil {
		t.Fatal(err)
	}

	photo := addBrowseItem(t, db, cardGallery.ID, source.ID, "photo.jpg", gallery.MediaKindStaticImage, gallery.ImageCategoryPhoto, gallery.AvailabilityAvailable, gallery.ProcessingReady, 1024, now)
	addBrowseItem(t, db, cardGallery.ID, source.ID, "selfie.jpg", gallery.MediaKindStaticImage, gallery.ImageCategorySelfie, gallery.AvailabilityAvailable, gallery.ProcessingPending, 2048, now)
	addBrowseItem(t, db, cardGallery.ID, source.ID, "animation.gif", gallery.MediaKindAnimatedImage, "", gallery.AvailabilityAvailable, gallery.ProcessingError, 3072, now)
	addBrowseItem(t, db, cardGallery.ID, source.ID, "video.mp4", gallery.MediaKindVideo, "", gallery.AvailabilityMissing, gallery.ProcessingError, 4096, now)
	profile := mediaprocessing.DefaultProfileHash()
	if _, err := db.Derivatives().Publish(ctx, PublishDerivativeInput{ItemUUID: photo.UUID, Variant: mediaprocessing.VariantCard480,
		CacheTier: mediaprocessing.CacheBase, ContentRevision: photo.ContentRevision, ProfileHash: profile,
		CacheRelativePath: "browse/cover.jpg", MIMEType: "image/jpeg", ByteSize: 10, Width: 480, Height: 640}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Covers().Initialize(ctx, cardGallery.ID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO gallery_personal_states(gallery_id,favorite,favorited_at_utc,rating_half_steps,rated_at_utc) VALUES(?,1,?,9,?)`, cardGallery.ID, formatTime(now), formatTime(now)); err != nil {
		t.Fatal(err)
	}
	activateBrowseFixture(t, db, cardGallery.ID, now)

	magic, _ := createBrowseGallery(t, db, "Adult", gallery.ContentRatingAdult, now.Add(time.Minute))
	activateBrowseFixture(t, db, magic.ID, now.Add(time.Minute))
	hidden, _ := createBrowseGallery(t, db, "Hidden", gallery.ContentRatingNonAdult, now.Add(2*time.Minute))
	activateBrowseFixture(t, db, hidden.ID, now.Add(2*time.Minute))
	if _, err := db.ExecContext(ctx, `INSERT INTO gallery_personal_states(gallery_id,hidden) VALUES(?,1)`, hidden.ID); err != nil {
		t.Fatal(err)
	}

	list, err := db.Browse().Galleries(ctx, browse.ScopeList, 1, browse.GallerySortRecentlyAdded)
	if err != nil {
		t.Fatal(err)
	}
	if list.TotalItems != 1 || len(list.Items) != 1 || list.PageSize != 24 || list.TotalPages != 1 {
		t.Fatalf("LIST page = %#v", list)
	}
	card := list.Items[0]
	if card.SetID != cardGallery.SetID || card.Slug == "" || card.CollectionType != browse.CollectionCosplay || card.ContentRating != gallery.ContentRatingNonAdult {
		t.Fatalf("card identity = %#v", card)
	}
	if card.Media.Photo != 1 || card.Media.Selfie != 1 || card.Media.GIF != 1 || card.Media.Video != 0 {
		t.Fatalf("AVAILABLE P/S/G/V = %#v", card.Media)
	}
	if card.CreditCount != 1 || card.CharacterCount != 1 || card.WorkCount != 1 || card.Credits[0].UUID != coser.UUID || card.Characters[0].UUID != character.UUID {
		t.Fatalf("card summaries = %#v", card)
	}
	if !card.Credits[0].AvatarAvailable || card.Credits[0].AssetRevision != 2 {
		t.Fatalf("card Coser avatar summary = %#v", card.Credits[0])
	}
	if !card.Favorite || card.RatingHalfSteps == nil || *card.RatingHalfSteps != 9 || card.ScrubberCount != 1 {
		t.Fatalf("card personal/scrubber state = %#v", card)
	}
	if card.Cover.Resource == nil || card.Cover.Resource.ItemUUID != photo.UUID || card.Cover.Resource.Variant != mediaprocessing.VariantCard480 {
		t.Fatalf("opaque cover = %#v", card.Cover)
	}
	magicPage, err := db.Browse().Galleries(ctx, browse.ScopeMagic, 1, browse.GallerySortRecentlyAdded)
	if err != nil || len(magicPage.Items) != 1 || magicPage.Items[0].SetID != magic.SetID {
		t.Fatalf("MAGIC page = %#v, %v", magicPage, err)
	}
	all, err := db.Browse().Galleries(ctx, browse.ScopeAll, 1, browse.GallerySortRecentlyAdded)
	if err != nil || all.TotalItems != 2 || all.Items[0].SetID != magic.SetID {
		t.Fatalf("ALL page = %#v, %v", all, err)
	}
	cosplay, err := db.Browse().GalleriesByCollection(ctx, browse.ScopeAll, 1, browse.GallerySortRecentlyAdded, browse.CollectionCosplay)
	if err != nil || cosplay.TotalItems != 1 || cosplay.Items[0].SetID != cardGallery.SetID {
		t.Fatalf("COSPLAY page = %#v, %v", cosplay, err)
	}
	album, err := db.Browse().GalleriesByCollection(ctx, browse.ScopeAll, 1, browse.GallerySortRecentlyAdded, browse.CollectionAlbum)
	if err != nil || album.TotalItems != 1 || album.Items[0].SetID != magic.SetID {
		t.Fatalf("ALBUM page = %#v, %v", album, err)
	}
}

func TestTimelineNormalizesMonthPrecisionSkipsUnknownAndFiltersCoser(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 7, 23, 19, 0, 0, 0, time.UTC)
	day17, _ := createBrowseGallery(t, db, "Day 17", gallery.ContentRatingNonAdult, now)
	monthOnly, _ := createBrowseGallery(t, db, "Month", gallery.ContentRatingNonAdult, now)
	day1, _ := createBrowseGallery(t, db, "Day 1", gallery.ContentRatingNonAdult, now)
	unknown, _ := createBrowseGallery(t, db, "Unknown", gallery.ContentRatingNonAdult, now)
	for id, values := range map[int64][2]string{
		day17.ID: {"2024-05-17", "DAY"}, monthOnly.ID: {"2024-05", "MONTH"}, day1.ID: {"2024-05-01", "DAY"},
	} {
		if _, err := db.ExecContext(ctx, `UPDATE galleries SET shoot_date=?,shoot_date_precision=? WHERE id=?`, values[0], values[1], id); err != nil {
			t.Fatal(err)
		}
	}
	activateBrowseFixture(t, db, day17.ID, now)
	activateBrowseFixture(t, db, day1.ID, now.Add(time.Minute))
	activateBrowseFixture(t, db, monthOnly.ID, now.Add(2*time.Minute))
	activateBrowseFixture(t, db, unknown.ID, now.Add(3*time.Minute))

	page, err := db.Browse().Timeline(ctx, browse.ScopeList, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 3 || page.Items[0].SetID != day17.SetID || page.Items[1].SetID != monthOnly.SetID || page.Items[2].SetID != day1.SetID {
		t.Fatalf("timeline order = %#v", page.Items)
	}
	coser, err := db.CoreEntities().CreateCoser(ctx, CreateCoserInput{CreateNamedEntityInput: CreateNamedEntityInput{Name: "Timeline Coser"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Galleries().AddCredit(ctx, day17.ID, coser.UUID, 1024, day17.MetadataRevision, now); err != nil {
		t.Fatal(err)
	}
	activateBrowseFixture(t, db, day17.ID, now)
	coserPage, err := db.Browse().Timeline(ctx, browse.ScopeAll, 1, coser.UUID)
	if err != nil || len(coserPage.Items) != 1 || coserPage.Items[0].SetID != day17.SetID {
		t.Fatalf("Coser timeline = %#v, %v", coserPage, err)
	}
}

func TestTimelineDateModesOmitUnknownAndSortDeterministically(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	first, _ := createBrowseGallery(t, db, "First", gallery.ContentRatingNonAdult, now)
	second, _ := createBrowseGallery(t, db, "Second", gallery.ContentRatingNonAdult, now)
	unknown, _ := createBrowseGallery(t, db, "Unknown", gallery.ContentRatingNonAdult, now)
	for _, value := range []gallery.Gallery{first, second, unknown} {
		activateBrowseFixture(t, db, value.ID, now)
	}
	for _, row := range []struct {
		id                                                      int64
		shoot, shootPrecision, publish, publishPrecision, added string
	}{
		{first.ID, "2024-05", "MONTH", "2025-01", "MONTH", "2023-01-01T00:00:00Z"},
		{second.ID, "2024-06-01", "DAY", "2024-12-31", "DAY", "2024-01-01T00:00:00Z"},
	} {
		if _, err := db.ExecContext(ctx, `UPDATE galleries SET shoot_date=?,shoot_date_precision=?,publish_date=?,publish_date_precision=?,media_added_status='COMPLETE',media_added_start_at_utc=? WHERE id=?`, row.shoot, row.shootPrecision, row.publish, row.publishPrecision, row.added, row.id); err != nil {
			t.Fatal(err)
		}
	}
	for _, check := range []struct {
		mode     browse.TimelineDate
		expected []string
	}{
		{browse.TimelineShoot, []string{second.SetID, first.SetID}},
		{browse.TimelineMediaAdded, []string{second.SetID, first.SetID}},
		{browse.TimelinePublish, []string{first.SetID, second.SetID}},
	} {
		page, err := db.Browse().TimelineByDate(ctx, browse.ScopeList, 1, "", check.mode)
		if err != nil {
			t.Fatal(err)
		}
		if page.TotalItems != 2 || len(page.Items) != 2 || page.Items[0].SetID != check.expected[0] || page.Items[1].SetID != check.expected[1] {
			t.Fatalf("mode %s: %#v", check.mode, page)
		}
	}
	if _, err := db.Browse().TimelineByDate(ctx, browse.ScopeList, 1, "", "INVALID"); err == nil {
		t.Fatal("unknown timeline mode accepted")
	}
}

func TestCombinedTimelineChoosesFirstAvailableDateAndOmitsUnknown(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDatabaseAndRegistry(t)
	now := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	type fixture struct {
		title, publish, publishPrecision, shoot, shootPrecision, added, addedStatus string
	}
	fixtures := []fixture{
		// The newer shoot and media dates must not override publication.
		{"published", "2025-02", "MONTH", "2026-01-01", "DAY", "2026-02-01T12:00:00Z", "COMPLETE"},
		{"shot", "", "UNKNOWN", "2025-02-03", "DAY", "2026-03-01T12:00:00Z", "COMPLETE"},
		{"added", "", "UNKNOWN", "", "UNKNOWN", "2025-02-02T23:59:59Z", "COMPLETE"},
		{"partial", "", "UNKNOWN", "", "UNKNOWN", "2025-03-01T00:00:00Z", "PARTIAL"},
		{"unknown", "", "UNKNOWN", "", "UNKNOWN", "", "NONE"},
	}
	var setIDs []string
	for _, item := range fixtures {
		created, _ := createBrowseGallery(t, db, item.title, gallery.ContentRatingNonAdult, now)
		activateBrowseFixture(t, db, created.ID, now)
		if _, err := db.ExecContext(ctx, `UPDATE galleries SET publish_date=NULLIF(?,''),publish_date_precision=NULLIF(?,'UNKNOWN'),
			shoot_date=NULLIF(?,''),shoot_date_precision=NULLIF(?,'UNKNOWN'),media_added_start_at_utc=?,media_added_status=? WHERE id=?`,
			item.publish, item.publishPrecision, item.shoot, item.shootPrecision, item.added, item.addedStatus, created.ID); err != nil {
			t.Fatal(err)
		}
		setIDs = append(setIDs, created.SetID)
	}
	page, err := db.Browse().TimelineByDate(ctx, browse.ScopeList, 1, "", browse.TimelineCombined)
	if err != nil {
		t.Fatal(err)
	}
	if page.TotalItems != 3 || len(page.Items) != 3 {
		t.Fatalf("combined timeline included missing or incomplete dates: %#v", page)
	}
	for index, expected := range []string{setIDs[1], setIDs[2], setIDs[0]} {
		if page.Items[index].SetID != expected {
			t.Fatalf("combined timeline order at %d: got %s, want %s", index, page.Items[index].SetID, expected)
		}
	}
	// Existing modes retain their own inclusion and ordering rules.
	shoot, err := db.Browse().TimelineByDate(ctx, browse.ScopeList, 1, "", browse.TimelineShoot)
	if err != nil || shoot.TotalItems != 2 || shoot.Items[0].SetID != setIDs[0] {
		t.Fatalf("shoot mode changed: page=%#v err=%v", shoot, err)
	}
}

func createBrowseGallery(t *testing.T, db *Database, title string, rating gallery.ContentRating, now time.Time) (gallery.Gallery, gallery.Source) {
	t.Helper()
	ctx := context.Background()
	created, err := db.Galleries().Create(ctx, CreateGalleryInput{Title: title, ContentRating: rating}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, created.ID, CreateSourceInput{Type: gallery.SourceTypeDirectory, Path: t.TempDir(), Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	return created, source
}

func addBrowseItem(t *testing.T, db *Database, galleryID, sourceID int64, relative string, kind gallery.MediaKind,
	category gallery.ImageCategory, availability gallery.AvailabilityState, processing gallery.ProcessingState, position int64, now time.Time) gallery.Item {
	t.Helper()
	format := gallery.ContentFormatImage
	if kind == gallery.MediaKindVideo {
		format = gallery.ContentFormatVideo
	}
	item, err := db.Galleries().AddItem(context.Background(), galleryID, sourceID, CreateItemInput{RelativePath: relative,
		MediaKind: kind, ContentFormat: format, ImageCategory: category, Availability: availability,
		ProcessingState: processing, Position: position}, now)
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func activateBrowseFixture(t *testing.T, db *Database, galleryID int64, added time.Time) {
	t.Helper()
	if _, err := db.Exec(`UPDATE galleries SET state='ACTIVE',added_at_utc=? WHERE id=?`, formatTime(added), galleryID); err != nil {
		t.Fatal(err)
	}
}
