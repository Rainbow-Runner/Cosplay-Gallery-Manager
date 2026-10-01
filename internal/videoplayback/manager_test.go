package videoplayback

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/persistence/productdb"
)

func TestProgressiveSessionAuthorizationAndPromotion(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("FFmpeg unavailable")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(source, "clip.mp4")
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=12", "-t", "9", "-an", "-c:v", "mpeg4", video)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v: %s", err, output)
	}
	ctx := context.Background()
	db, err := productdb.Open(ctx, filepath.Join(root, "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	created, err := db.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: "Progressive", ContentRating: gallery.ContentRatingNonAdult}, now)
	if err != nil {
		t.Fatal(err)
	}
	src, err := db.Galleries().AddSource(ctx, created.ID, productdb.CreateSourceInput{Type: gallery.SourceTypeDirectory, Path: source, Availability: gallery.AvailabilityAvailable}, now)
	if err != nil {
		t.Fatal(err)
	}
	item, err := db.Galleries().AddItem(ctx, created.ID, src.ID, productdb.CreateItemInput{RelativePath: "clip.mp4", MediaKind: gallery.MediaKindVideo, ContentFormat: gallery.ContentFormatVideo, Position: 1024, Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingReady}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE galleries SET state='ACTIVE',added_at_utc=? WHERE id=?`, now.Format("2006-01-02T15:04:05Z"), created.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.VideoMetadata().MarkPending(ctx, item.UUID, item.ContentRevision, "test-probe"); err != nil {
		t.Fatal(err)
	}
	metadata := mediaprocessing.VideoTechnicalMetadata{ItemUUID: item.UUID, ContentRevision: item.ContentRevision, ProbeProfileHash: "test-probe", Container: "mp4", VideoCodec: "mpeg4", VideoStreamIndex: 0, DisplayWidth: 320, DisplayHeight: 180, DurationSeconds: 9}
	if err := db.VideoMetadata().PublishReady(ctx, metadata, now); err != nil {
		t.Fatal(err)
	}
	m, err := New(db, filepath.Join(root, "cache"), ffmpeg, "test-ffmpeg", func(r *http.Request) bool {
		cookie, err := r.Cookie("cgm_session")
		return err == nil && (cookie.Value == "owner" || cookie.Value == "other")
	})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	request := func(method, path, cookie string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: "cgm_session", Value: cookie})
		}
		w := httptest.NewRecorder()
		m.ServeHTTP(w, r)
		return w
	}
	path := Prefix + "video/" + item.UUID
	if got := request(http.MethodPost, path, "").Code; got != http.StatusUnauthorized {
		t.Fatalf("unauthenticated start = %d", got)
	}
	w := request(http.MethodPost, path, "owner")
	if w.Code != http.StatusOK {
		t.Fatalf("start = %d: %s", w.Code, w.Body.String())
	}
	var state State
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Mode != "HLS_SESSION" || state.Lease == "" {
		t.Fatalf("session = %+v", state)
	}
	firstLease := state.Lease
	if got := request(http.MethodGet, state.URL, "other").Code; got != http.StatusNotFound {
		t.Fatalf("foreign playlist = %d", got)
	}
	if got := request(http.MethodGet, state.URL, "owner").Code; got != http.StatusOK {
		t.Fatalf("owner playlist = %d", got)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		w = request(http.MethodGet, Prefix+"session/"+state.Lease+"/status", "owner")
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d", w.Code)
		}
		if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
			t.Fatal(err)
		}
		if state.Status == "ERROR" {
			t.Fatalf("processing error %+v", state)
		}
		if state.Status == "READY" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if state.Status != "READY" {
		t.Fatalf("session never completed: %+v", state)
	}
	segment := Prefix + "session/" + firstLease + "/segment-000000.ts"
	w = request(http.MethodGet, segment, "owner")
	if w.Code != http.StatusOK || w.Body.Len() == 0 || w.Header().Get("Content-Type") != "video/mp2t" {
		t.Fatalf("authenticated segment = %d, %d bytes, %q", w.Code, w.Body.Len(), w.Header().Get("Content-Type"))
	}
	if got := request(http.MethodGet, segment, "other").Code; got != http.StatusNotFound {
		t.Fatalf("foreign segment = %d", got)
	}
	w = request(http.MethodPost, path, "owner")
	if w.Code != http.StatusOK {
		t.Fatalf("cache reuse = %d", w.Code)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Mode != "MP4_PROXY" {
		t.Fatalf("cache not promoted: %+v", state)
	}
	if _, err := db.ExecContext(ctx, `UPDATE gallery_items SET content_revision=content_revision+1 WHERE item_uuid=?`, item.UUID); err != nil {
		t.Fatal(err)
	}
	if got := request(http.MethodGet, Prefix+"session/"+firstLease+"/index.m3u8", "owner").Code; got != http.StatusNotFound {
		t.Fatalf("old session after source revision change = %d", got)
	}
}

func TestStartupRemovesOnlyOwnedPlaybackTemporaryDirectories(t *testing.T) {
	root := t.TempDir()
	hls := filepath.Join(root, "tmp", "hls")
	if err := os.MkdirAll(hls, 0700); err != nil {
		t.Fatal(err)
	}
	owned := filepath.Join(hls, "cgm-hls-"+strings.Repeat("a", 32))
	unknown := filepath.Join(hls, "unrelated")
	for _, path := range []string{owned, unknown} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "keep"), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m, err := New(nil, root, "", "", func(*http.Request) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	m.Close()
	if _, err := os.Stat(owned); !os.IsNotExist(err) {
		t.Fatalf("owned temporary directory retained: %v", err)
	}
	if _, err := os.Stat(filepath.Join(unknown, "keep")); err != nil {
		t.Fatalf("unknown directory changed: %v", err)
	}
}
