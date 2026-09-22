package videoresource

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/persistence/productdb"
)

func TestDirectVideoHandlerAuthorizesIdentityAndServesBrowserRanges(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 15, 15, 0, 0, 0, time.UTC)
	db, err := productdb.Open(ctx, filepath.Join(t.TempDir(), "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	root := t.TempDir()
	content := []byte("0123456789abcdef")
	if err := os.WriteFile(filepath.Join(root, "clip.mp4"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	record, err := db.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Direct video"}, now)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Galleries().AddSource(ctx, record.ID, productdb.CreateSourceInput{Type: gallery.SourceTypeDirectory, Path: root, Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	item, err := db.Galleries().AddItem(ctx, record.ID, source.ID, productdb.CreateItemInput{RelativePath: "clip.mp4", MediaKind: gallery.MediaKindVideo,
		ContentFormat: gallery.ContentFormatVideo, Position: 1024, Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingReady}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE galleries SET state='ACTIVE',content_rating='NON_ADULT',added_at_utc=? WHERE id=?`, now.Format(time.RFC3339Nano), record.ID); err != nil {
		t.Fatal(err)
	}
	profile := mediaprocessing.VideoProbeProfileHash("6.1")
	if err := db.VideoMetadata().MarkPending(ctx, item.UUID, item.ContentRevision, profile); err != nil {
		t.Fatal(err)
	}
	audioIndex := 1
	if err := db.VideoMetadata().PublishReady(ctx, mediaprocessing.VideoTechnicalMetadata{ItemUUID: item.UUID, ContentRevision: item.ContentRevision, ProbeProfileHash: profile,
		Container: "mp4", VideoStreamIndex: 0, VideoCodec: "h264", AudioStreamIndex: &audioIndex, AudioCodec: "aac", DisplayWidth: 1280, DisplayHeight: 720}, now); err != nil {
		t.Fatal(err)
	}
	handler := Handler{Database: db, Access: func(request *http.Request) (productdb.ResourceAccess, error) {
		return productdb.ResourceAccess{Authenticated: request.Header.Get("Authorization") == "session", Mode: productdb.ResourceBrowse, Scope: productdb.ResourceScope(request.Header.Get("X-Scope"))}, nil
	}}
	resourceURL := RoutePrefix + item.UUID + "/" + strconv.FormatInt(item.ContentRevision, 10) + "/direct"

	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, resourceURL, nil))
	if unauthenticated.Code != http.StatusNotFound || unauthenticated.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unauthenticated response = %d %#v", unauthenticated.Code, unauthenticated.Header())
	}

	rangeRequest := httptest.NewRequest(http.MethodGet, resourceURL, nil)
	rangeRequest.Header.Set("Authorization", "session")
	rangeRequest.Header.Set("X-Scope", string(productdb.ResourceScopeList))
	rangeRequest.Header.Set("Range", "bytes=2-5")
	ranged := httptest.NewRecorder()
	handler.ServeHTTP(ranged, rangeRequest)
	if ranged.Code != http.StatusPartialContent || ranged.Body.String() != "2345" || ranged.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("range response = %d %q %#v", ranged.Code, ranged.Body.String(), ranged.Header())
	}
	if ranged.Header().Get("Accept-Ranges") != "bytes" || ranged.Header().Get("X-Content-Type-Options") != "nosniff" || ranged.Header().Get("Content-Disposition") != "inline" {
		t.Fatalf("direct headers = %#v", ranged.Header())
	}

	headRequest := httptest.NewRequest(http.MethodHead, resourceURL, nil)
	headRequest.Header.Set("Authorization", "session")
	headRequest.Header.Set("X-Scope", string(productdb.ResourceScopeList))
	head := httptest.NewRecorder()
	handler.ServeHTTP(head, headRequest)
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") != strconv.Itoa(len(content)) {
		t.Fatalf("HEAD response = %d %q %#v", head.Code, head.Body.String(), head.Header())
	}

	invalidRangeRequest := httptest.NewRequest(http.MethodGet, resourceURL, nil)
	invalidRangeRequest.Header.Set("Authorization", "session")
	invalidRangeRequest.Header.Set("X-Scope", string(productdb.ResourceScopeList))
	invalidRangeRequest.Header.Set("Range", "bytes=100-200")
	invalidRange := httptest.NewRecorder()
	handler.ServeHTTP(invalidRange, invalidRangeRequest)
	if invalidRange.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("invalid range response = %d", invalidRange.Code)
	}

	multiRangeRequest := httptest.NewRequest(http.MethodGet, resourceURL, nil)
	multiRangeRequest.Header.Set("Authorization", "session")
	multiRangeRequest.Header.Set("X-Scope", string(productdb.ResourceScopeList))
	multiRangeRequest.Header.Set("Range", "bytes=0-1,4-5")
	multiRange := httptest.NewRecorder()
	handler.ServeHTTP(multiRange, multiRangeRequest)
	if multiRange.Code != http.StatusPartialContent || !strings.HasPrefix(multiRange.Header().Get("Content-Type"), "multipart/byteranges;") {
		t.Fatalf("multi-range response = %d %#v", multiRange.Code, multiRange.Header())
	}

	staleRequest := httptest.NewRequest(http.MethodGet, RoutePrefix+item.UUID+"/2/direct", nil)
	staleRequest.Header.Set("Authorization", "session")
	staleRequest.Header.Set("X-Scope", string(productdb.ResourceScopeList))
	stale := httptest.NewRecorder()
	handler.ServeHTTP(stale, staleRequest)
	if stale.Code != http.StatusNotFound {
		t.Fatalf("stale revision response = %d", stale.Code)
	}

	if _, err := db.ExecContext(ctx, `INSERT INTO gallery_personal_states(gallery_id,hidden) VALUES(?,1)`, record.ID); err != nil {
		t.Fatal(err)
	}
	hiddenRequest := httptest.NewRequest(http.MethodGet, resourceURL, nil)
	hiddenRequest.Header.Set("Authorization", "session")
	hiddenRequest.Header.Set("X-Scope", string(productdb.ResourceScopeAll))
	hidden := httptest.NewRecorder()
	handler.ServeHTTP(hidden, hiddenRequest)
	if hidden.Code != http.StatusNotFound {
		t.Fatalf("hidden response = %d", hidden.Code)
	}
	if strings.Contains(ranged.Body.String(), root) || strings.Contains(ranged.Body.String(), "clip.mp4") {
		t.Fatal("direct response leaked a physical path")
	}
}
