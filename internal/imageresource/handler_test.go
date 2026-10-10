package imageresource

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaaccess"
	"github.com/stashapp/stash/internal/persistence/productdb"
)

func TestHandlerServesEligibleStaticAndExactAnimatedOriginals(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	db, err := productdb.Open(ctx, filepath.Join(t.TempDir(), "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	root := t.TempDir()
	staticBytes := jpegBytes(t, 32, 24)
	animatedBytes := gifBytes(t)
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), staticBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "motion.gif"), animatedBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	record, source := activeSource(t, db, gallery.SourceTypeDirectory, root, now)
	staticItem := addImageItem(t, db, record.ID, source.ID, "photo.jpg", gallery.MediaKindStaticImage, now)
	animatedItem := addImageItem(t, db, record.ID, source.ID, "motion.gif", gallery.MediaKindAnimatedImage, now)
	temporaryRoot := t.TempDir()
	handler := Handler{Database: db, Materializer: mediaaccess.Materializer{TemporaryRoot: temporaryRoot}, Access: testAccess}

	staticResponse := serve(t, handler, http.MethodHead, staticItem.UUID, staticItem.ContentRevision)
	if staticResponse.Code != http.StatusOK || staticResponse.Body.Len() != 0 || staticResponse.Header().Get("Content-Type") != "image/jpeg" ||
		staticResponse.Header().Get("Content-Length") != strconv.Itoa(len(staticBytes)) {
		t.Fatalf("static HEAD = %d %q %#v", staticResponse.Code, staticResponse.Body.String(), staticResponse.Header())
	}
	animatedResponse := serve(t, handler, http.MethodGet, animatedItem.UUID, animatedItem.ContentRevision)
	if animatedResponse.Code != http.StatusOK || !bytes.Equal(animatedResponse.Body.Bytes(), animatedBytes) || animatedResponse.Header().Get("Content-Type") != "image/gif" {
		t.Fatalf("animated original = %d bytes=%d want=%d %#v", animatedResponse.Code, animatedResponse.Body.Len(), len(animatedBytes), animatedResponse.Header())
	}
}

func TestHandlerRejectsOversizedStaticAndServesArchiveEntry(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 16, 13, 0, 0, 0, time.UTC)
	db, err := productdb.Open(ctx, filepath.Join(t.TempDir(), "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	root := t.TempDir()
	oversized := jpegBytes(t, 4097, 1)
	if err := os.WriteFile(filepath.Join(root, "wide.jpg"), oversized, 0o600); err != nil {
		t.Fatal(err)
	}
	record, source := activeSource(t, db, gallery.SourceTypeDirectory, root, now)
	wide := addImageItem(t, db, record.ID, source.ID, "wide.jpg", gallery.MediaKindStaticImage, now)
	handler := Handler{Database: db, Materializer: mediaaccess.Materializer{TemporaryRoot: t.TempDir()}, Access: testAccess}
	if response := serve(t, handler, http.MethodHead, wide.UUID, wide.ContentRevision); response.Code != http.StatusNotFound {
		t.Fatalf("oversized static status = %d", response.Code)
	}

	archiveBytes := gifBytes(t)
	archivePath := filepath.Join(root, "gallery.cbz")
	archive, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(archive)
	entry, err := writer.Create("nested/motion.gif")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(archiveBytes); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	archiveRecord, archiveSource := activeSource(t, db, gallery.SourceTypeArchive, archivePath, now.Add(time.Minute))
	archiveItem := addImageItem(t, db, archiveRecord.ID, archiveSource.ID, "nested/motion.gif", gallery.MediaKindAnimatedImage, now)
	response := serve(t, handler, http.MethodGet, archiveItem.UUID, archiveItem.ContentRevision)
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), archiveBytes) {
		t.Fatalf("archive original = %d bytes=%d", response.Code, response.Body.Len())
	}
}

func activeSource(t *testing.T, db *productdb.Database, sourceType gallery.SourceType, sourcePath string, now time.Time) (gallery.Gallery, gallery.Source) {
	t.Helper()
	record, err := db.Galleries().Create(context.Background(), productdb.CreateGalleryInput{Title: "Original image"}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(context.Background(), record.ID, productdb.CreateSourceInput{Type: sourceType, Path: sourcePath, Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(), `UPDATE galleries SET state='ACTIVE',content_rating='NON_ADULT',added_at_utc=? WHERE id=?`, now.Format(time.RFC3339Nano), record.ID); err != nil {
		t.Fatal(err)
	}
	return record, source
}

func addImageItem(t *testing.T, db *productdb.Database, galleryID, sourceID int64, relative string, kind gallery.MediaKind, now time.Time) gallery.Item {
	t.Helper()
	category := gallery.ImageCategory("")
	position := int64(2048)
	if kind == gallery.MediaKindStaticImage {
		category = gallery.ImageCategoryPhoto
		position = 1024
	}
	item, err := db.Galleries().AddItem(context.Background(), galleryID, sourceID, productdb.CreateItemInput{RelativePath: relative, MediaKind: kind,
		ContentFormat: gallery.ContentFormatImage, ImageCategory: category, Position: position, Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingReady}, now)
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func serve(t *testing.T, handler Handler, method, uuid string, revision int64) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, RoutePrefix+uuid+"/"+strconv.FormatInt(revision, 10)+"/original", nil)
	request.Header.Set("Authorization", "session")
	request.Header.Set("X-Scope", string(productdb.ResourceScopeList))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func testAccess(request *http.Request) (productdb.ResourceAccess, error) {
	return productdb.ResourceAccess{Authenticated: request.Header.Get("Authorization") == "session", Mode: productdb.ResourceBrowse, Scope: productdb.ResourceScope(request.Header.Get("X-Scope"))}, nil
}

func jpegBytes(t *testing.T, width, height int) []byte {
	t.Helper()
	var output bytes.Buffer
	if err := jpeg.Encode(&output, image.NewRGBA(image.Rect(0, 0, width, height)), nil); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func gifBytes(t *testing.T) []byte {
	t.Helper()
	palette := color.Palette{color.Black, color.White}
	first := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	second := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	second.Pix[0] = 1
	var output bytes.Buffer
	if err := gif.EncodeAll(&output, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{7, 11}, LoopCount: 0}); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
