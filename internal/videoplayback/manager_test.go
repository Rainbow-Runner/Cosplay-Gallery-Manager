package videoplayback

import (
	"archive/tar"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/archivecheck"
	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaaccess"
	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/settings"
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

func TestHLSExecutionPlanSupportsReviewedHardwareBackends(t *testing.T) {
	metadata := mediaprocessing.VideoTechnicalMetadata{VideoCodec: "hevc", PixelFormat: "yuv420p", DisplayWidth: 2160, DisplayHeight: 3840}
	plan := mediaprocessing.PlaybackPlanFromMetadata(metadata)
	hardware := mediaprocessing.HardwareAccelerationStatus{ProbeState: mediaprocessing.HardwareProbeCompleted, Backends: []mediaprocessing.HardwareBackendStatus{
		{Backend: "NVENC", State: mediaprocessing.HardwareProbeAvailable, Device: "nvidia0", DecodeCodecs: []string{"h264_cuvid", "hevc_cuvid"}},
		{Backend: "VAAPI", State: mediaprocessing.HardwareProbeAvailable, Device: "renderD128", DecodeCodecs: []string{"h264", "hevc"}},
	}}
	nvenc, code := hlsExecutionPlan(plan, metadata, mediaprocessing.VideoHardwarePreference{Mode: "AUTO", AllowSoftwareFallback: true}, hardware)
	if code != "" || nvenc.EffectiveBackend != "NVENC" {
		t.Fatalf("NVENC plan=%#v code=%q", nvenc, code)
	}
	vaapiOnly := hardware
	vaapiOnly.Backends = vaapiOnly.Backends[1:]
	vaapi, code := hlsExecutionPlan(plan, metadata, mediaprocessing.VideoHardwarePreference{Mode: "AUTO", AllowSoftwareFallback: true}, vaapiOnly)
	if code != "" || vaapi.EffectiveBackend != "VAAPI" || vaapi.Device != "renderD128" || vaapi.Decoder != "hevc" {
		t.Fatalf("VAAPI plan=%#v code=%q", vaapi, code)
	}
	explicit, code := hlsExecutionPlan(plan, metadata, mediaprocessing.VideoHardwarePreference{Mode: "VAAPI"}, vaapiOnly)
	if code != "" || explicit.EffectiveBackend != "VAAPI" {
		t.Fatalf("VAAPI explicit=%#v code=%q", explicit, code)
	}
	metadata.PixelFormat = "yuv420p10le"
	unsupported, code := hlsExecutionPlan(plan, metadata, mediaprocessing.VideoHardwarePreference{Mode: "NVENC"}, hardware)
	if code != "VIDEO_HARDWARE_UNAVAILABLE" || unsupported.Executable {
		t.Fatalf("10-bit plan=%#v code=%q", unsupported, code)
	}
}

func TestHLSFailureCodePreventsImplicitHardwareFallback(t *testing.T) {
	if got := hlsFailureCode(mediaprocessing.VideoTranscodeExecutionPlan{EffectiveBackend: "NVENC"}); got != "VIDEO_HARDWARE_HLS_FAILED" {
		t.Fatalf("NVENC failure code = %q", got)
	}
	if got := hlsFailureCode(mediaprocessing.VideoTranscodeExecutionPlan{EffectiveBackend: "SOFTWARE"}); got != "VIDEO_HLS_FAILED" {
		t.Fatalf("software failure code = %q", got)
	}
	if got := hlsFailureCode(mediaprocessing.VideoTranscodeExecutionPlan{EffectiveBackend: "VAAPI"}); got != "VIDEO_HARDWARE_HLS_FAILED" {
		t.Fatalf("VAAPI failure code = %q", got)
	}
}

func TestHA05PlaylistKeepsFullTimelineWithShortLeadSegment(t *testing.T) {
	value := playlist(9)
	for _, expected := range []string{"#EXT-X-TARGETDURATION:4", "#EXT-X-PLAYLIST-TYPE:VOD", "#EXTINF:2.000000,\nsegment-000000.ts", "#EXTINF:4.000000,\nsegment-000001.ts", "#EXTINF:3.000000,\nsegment-000002.ts", "#EXT-X-ENDLIST"} {
		if !strings.Contains(value, expected) {
			t.Fatalf("playlist lacks %q:\n%s", expected, value)
		}
	}
	if strings.Count(value, "segment-") != 3 {
		t.Fatalf("unexpected playlist segment count:\n%s", value)
	}
}

func TestHLSHardwareFallbackCleansPartialOutputSwitchesProfileAndRunsOnce(t *testing.T) {
	directory := t.TempDir()
	for name, data := range map[string]string{"segment-000000.ts": "partial", "segment-000001.ts.tmp": "pending", "internal.m3u8": "playlist"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	metadata := mediaprocessing.VideoTechnicalMetadata{VideoCodec: "hevc", PixelFormat: "yuv420p", VideoStreamIndex: 0, DisplayWidth: 2160, DisplayHeight: 3840}
	plan := mediaprocessing.PlaybackPlanFromMetadata(metadata)
	hardware := mediaprocessing.HardwareAccelerationStatus{ProbeState: mediaprocessing.HardwareProbeCompleted, Backends: []mediaprocessing.HardwareBackendStatus{{Backend: "NVENC", State: mediaprocessing.HardwareProbeAvailable, Device: "nvidia0", DecodeCodecs: []string{"hevc_cuvid"}}}}
	execution := mediaprocessing.PlanVideoTranscode(plan, metadata, mediaprocessing.VideoHardwarePreference{Mode: "NVENC", AllowSoftwareFallback: true}, hardware, mediaprocessing.VideoTranscodeHLS)
	s := &session{dir: directory, descriptor: productdb.DirectVideoDescriptor{ItemUUID: "12345678-1234-1234-1234-123456789012", ContentRevision: 3, Metadata: metadata, Source: mediaaccess.Source{Type: gallery.SourceTypeDirectory}}, execution: execution,
		profile: "hardware-profile", state: State{Backend: "NVENC", Status: "PROCESSING", ErrorCode: "old"}}
	failures := []string{}
	m := &Manager{Version: "7.1", RecordHardwareFailure: func(backend string) { failures = append(failures, backend) }}
	applied, err := m.applyHLSFallback(s, "NVENC", "HARDWARE_DEVICE_OR_DRIVER_FAILED")
	if err != nil || !applied {
		t.Fatalf("fallback = %v, %v", applied, err)
	}
	if len(failures) != 1 || failures[0] != "NVENC" || s.execution.EffectiveBackend != "SOFTWARE" || s.state.Backend != "SOFTWARE" || s.state.Status != "PENDING" || s.state.ErrorCode != "" || s.profile == "hardware-profile" {
		t.Fatalf("session=%#v failures=%v", s, failures)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("partial output remains: %v %v", entries, err)
	}
	if applied, err := m.applyHLSFallback(s, "NVENC", "HARDWARE_DEVICE_OR_DRIVER_FAILED"); err != nil || applied || len(failures) != 1 {
		t.Fatalf("second fallback = %v, %v failures=%v", applied, err, failures)
	}
}

func TestHLSVAAPIPoolFailureFallsBackThroughHybridBeforeSoftware(t *testing.T) {
	directory := t.TempDir()
	metadata := mediaprocessing.VideoTechnicalMetadata{VideoCodec: "hevc", PixelFormat: "yuv420p", VideoStreamIndex: 0, DisplayWidth: 2160, DisplayHeight: 3840}
	plan := mediaprocessing.PlaybackPlanFromMetadata(metadata)
	hardware := mediaprocessing.HardwareAccelerationStatus{ProbeState: mediaprocessing.HardwareProbeCompleted, Backends: []mediaprocessing.HardwareBackendStatus{{Backend: "VAAPI", State: mediaprocessing.HardwareProbeAvailable, Device: "renderD128", DecodeCodecs: []string{"h264", "hevc"}}}}
	execution := mediaprocessing.PlanVideoTranscode(plan, metadata, mediaprocessing.VideoHardwarePreference{Mode: "VAAPI", AllowSoftwareFallback: true}, hardware, mediaprocessing.VideoTranscodeHLS)
	s := &session{dir: directory, descriptor: productdb.DirectVideoDescriptor{ItemUUID: "12345678-1234-1234-1234-123456789012", ContentRevision: 3, Metadata: metadata, Source: mediaaccess.Source{Type: gallery.SourceTypeDirectory}}, execution: execution,
		profile: "full-profile", state: State{Backend: "VAAPI", Status: "PROCESSING"}}
	failures := []string{}
	m := &Manager{Version: "6.1.1", RecordHardwareFailure: func(backend string) { failures = append(failures, backend) }}
	applied, err := m.applyHLSFallback(s, "VAAPI", "HARDWARE_FRAME_POOL_EXHAUSTED")
	if err != nil || !applied || s.execution.FilterStrategy != "VAAPI_CPU_SCALE" || s.fallbackLevel != 1 || len(failures) != 0 || s.profile == "full-profile" {
		t.Fatalf("hybrid fallback session=%#v failures=%v applied=%v err=%v", s, failures, applied, err)
	}
	applied, err = m.applyHLSFallback(s, "VAAPI", "HARDWARE_DEVICE_OR_DRIVER_FAILED")
	if err != nil || !applied || s.execution.EffectiveBackend != "SOFTWARE" || s.fallbackLevel != 2 || len(failures) != 1 {
		t.Fatalf("software fallback session=%#v failures=%v applied=%v err=%v", s, failures, applied, err)
	}
}

// Opt-in host gate: this exercises the complete HA-03 path through persisted
// runtime settings, Browse authorization, directory/archive source proof,
// progressive session handling, NVDEC/CUDA/NVENC, seek and cleanup.
func TestNVENCProgressiveSessionEndToEnd(t *testing.T) {
	if os.Getenv("CGM_TEST_NVENC") != "1" {
		t.Skip("set CGM_TEST_NVENC=1 on an NVIDIA test host")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Fatal(err)
	}
	fixtureRoot := t.TempDir()
	clip := filepath.Join(fixtureRoot, "clip.mp4")
	command := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=12", "-t", "36", "-an", "-c:v", "libx265", "-preset", "ultrafast", "-x265-params", "log-level=error:pools=1:frame-threads=1", "-pix_fmt", "yuv420p", clip)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("HEVC fixture: %v: %s", err, output)
	}
	for _, scenario := range []struct {
		name, sourceKind string
		seek             bool
	}{{"directory-complete", "directory", false}, {"tar-range-seek", "tar", true}, {"sevenzip-range-seek", "7z", true}} {
		t.Run(scenario.name, func(t *testing.T) {
			testHardwareProgressiveSource(t, ffmpeg, ffprobe, clip, scenario.sourceKind, scenario.seek, "NVENC", "nvidia0")
		})
	}
}

func TestVAAPIProgressiveSessionEndToEnd(t *testing.T) {
	device := os.Getenv("CGM_TEST_VAAPI_DEVICE")
	if device == "" {
		t.Skip("set CGM_TEST_VAAPI_DEVICE to a render node such as renderD128")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Fatal(err)
	}
	fixtureRoot := t.TempDir()
	clip := filepath.Join(fixtureRoot, "clip.mp4")
	command := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=12", "-t", "36", "-an", "-c:v", "libx265", "-preset", "ultrafast", "-x265-params", "log-level=error:pools=1:frame-threads=1", "-pix_fmt", "yuv420p", clip)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("HEVC fixture: %v: %s", err, output)
	}
	for _, scenario := range []struct {
		name, sourceKind string
		seek             bool
	}{{"directory-complete", "directory", false}, {"tar-range-seek", "tar", true}, {"sevenzip-range-seek", "7z", true}} {
		t.Run(scenario.name, func(t *testing.T) {
			testHardwareProgressiveSource(t, ffmpeg, ffprobe, clip, scenario.sourceKind, scenario.seek, "VAAPI", device)
		})
	}
}

func testHardwareProgressiveSource(t *testing.T, ffmpeg, ffprobe, clip, sourceKind string, seek bool, backend, device string) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	db, err := productdb.Open(ctx, filepath.Join(root, "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	created, err := db.Galleries().Create(ctx, productdb.CreateGalleryInput{Title: backend + " progressive", ContentRating: gallery.ContentRatingNonAdult}, now)
	if err != nil {
		t.Fatal(err)
	}
	var itemUUID string
	var revision int64
	if sourceKind != "directory" {
		archivePath := filepath.Join(root, "source."+sourceKind)
		memberPath := "video/clip.mp4"
		if sourceKind == "7z" {
			memberPath = writeStoredSevenZIPVideo(t, archivePath, clip)
		} else {
			writeStoredTarVideo(t, archivePath, clip)
		}
		source, err := db.Galleries().AddSource(ctx, created.ID, productdb.CreateSourceInput{Type: gallery.SourceTypeArchive, Path: archivePath, Availability: gallery.AvailabilityAvailable}, now)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Scans().RunWithOptions(ctx, source.ID, archivecheck.DefaultLimits(), productdb.ScanOptions{ExcludeNewRootMedia: false}, now); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT item_uuid,content_revision FROM gallery_items WHERE gallery_id=? AND relative_path=?`, created.ID, memberPath).Scan(&itemUUID, &revision); err != nil {
			t.Fatal(err)
		}
	} else {
		sourceRoot := filepath.Join(root, "source")
		if err := os.Mkdir(sourceRoot, 0700); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(sourceRoot, "clip.mp4")
		if err := copyTestFile(clip, target); err != nil {
			t.Fatal(err)
		}
		source, err := db.Galleries().AddSource(ctx, created.ID, productdb.CreateSourceInput{Type: gallery.SourceTypeDirectory, Path: sourceRoot, Availability: gallery.AvailabilityAvailable}, now)
		if err != nil {
			t.Fatal(err)
		}
		item, err := db.Galleries().AddItem(ctx, created.ID, source.ID, productdb.CreateItemInput{RelativePath: "clip.mp4", MediaKind: gallery.MediaKindVideo, ContentFormat: gallery.ContentFormatVideo, Position: 1024, Availability: gallery.AvailabilityAvailable, ProcessingState: gallery.ProcessingReady}, now)
		if err != nil {
			t.Fatal(err)
		}
		itemUUID, revision = item.UUID, item.ContentRevision
	}
	if _, err := db.ExecContext(ctx, `UPDATE galleries SET state='ACTIVE',added_at_utc=? WHERE id=?`, now.Format(time.RFC3339Nano), created.ID); err != nil {
		t.Fatal(err)
	}
	profile := strings.ToLower(backend) + "-e2e"
	if err := db.VideoMetadata().MarkPending(ctx, itemUUID, revision, profile); err != nil {
		t.Fatal(err)
	}
	metadata := mediaprocessing.VideoTechnicalMetadata{ItemUUID: itemUUID, ContentRevision: revision, ProbeProfileHash: profile, Container: "mp4", VideoCodec: "hevc", PixelFormat: "yuv420p", VideoProfile: "Main", VideoStreamIndex: 0, DisplayWidth: 320, DisplayHeight: 180, DurationSeconds: 36}
	if err := db.VideoMetadata().PublishReady(ctx, metadata, now); err != nil {
		t.Fatal(err)
	}
	runtime, err := db.Settings().Find(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if backend == "VAAPI" {
		runtime.VideoHardwareMode = settings.VideoHardwareVAAPI
	} else {
		runtime.VideoHardwareMode = settings.VideoHardwareNVENC
	}
	runtime.VideoHardwareFallbackEnabled = false
	runtime.VideoHardwareDevice = device
	if _, err := db.Settings().Update(ctx, runtime.Revision, runtime, now); err != nil {
		t.Fatal(err)
	}
	m, err := New(db, filepath.Join(root, "cache"), ffmpeg, profile, func(r *http.Request) bool {
		cookie, err := r.Cookie("cgm_session")
		return err == nil && cookie.Value == "owner"
	})
	if err != nil {
		t.Fatal(err)
	}
	m.HardwareStatus = func() mediaprocessing.HardwareAccelerationStatus {
		decodeCodecs := []string{"h264_cuvid", "hevc_cuvid"}
		scaleFilter := "scale_cuda"
		if backend == "VAAPI" {
			decodeCodecs = []string{"h264", "hevc"}
			scaleFilter = mediaprocessing.VAAPIFilterHybrid
		}
		return mediaprocessing.HardwareAccelerationStatus{ProbeState: mediaprocessing.HardwareProbeCompleted, Backends: []mediaprocessing.HardwareBackendStatus{{Backend: backend, State: mediaprocessing.HardwareProbeAvailable, Device: device, DecodeCodecs: decodeCodecs, ScaleFilter: scaleFilter}}}
	}
	closed := false
	defer func() {
		if !closed {
			m.Close()
		}
	}()
	request := func(method, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		r.AddCookie(&http.Cookie{Name: "cgm_session", Value: "owner"})
		w := httptest.NewRecorder()
		m.ServeHTTP(w, r)
		return w
	}
	response := request(http.MethodPost, Prefix+"video/"+itemUUID)
	if response.Code != http.StatusOK {
		t.Fatalf("start = %d: %s", response.Code, response.Body.String())
	}
	var state State
	if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Mode != "HLS_SESSION" || state.Backend != backend || state.Lease == "" {
		t.Fatalf("session = %#v", state)
	}
	m.mu.Lock()
	var activeBackend string
	var sessionDirectory string
	for _, active := range m.sessions {
		activeBackend = active.execution.EffectiveBackend
		sessionDirectory = active.dir
	}
	m.mu.Unlock()
	if activeBackend != backend {
		t.Fatalf("effective backend = %q", activeBackend)
	}
	if seek {
		segment := request(http.MethodGet, Prefix+"session/"+state.Lease+"/segment-000007.ts")
		if segment.Code != http.StatusOK || segment.Body.Len() == 0 {
			t.Fatalf("seek segment = %d, %d bytes: %s", segment.Code, segment.Body.Len(), segment.Body.String())
		}
		part := filepath.Join(root, "seek.ts")
		if err := os.WriteFile(part, segment.Body.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		output, err := exec.Command(ffprobe, "-v", "error", "-show_entries", "format=start_time", "-of", "default=noprint_wrappers=1:nokey=1", part).Output()
		startTime, parseErr := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
		if err != nil || parseErr != nil || startTime < 20 {
			t.Fatalf("seek timestamp = %q, %v", output, err)
		}
	} else {
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			response = request(http.MethodGet, Prefix+"session/"+state.Lease+"/status")
			if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
				t.Fatal(err)
			}
			if state.Status == "ERROR" || state.Status == "READY" {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if state.Status != "READY" {
			entries, _ := os.ReadDir(sessionDirectory)
			names := make([]string, 0, len(entries))
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			t.Fatalf("%s session did not complete: %#v files=%v", backend, state, names)
		}
		manifest, err := os.ReadFile(filepath.Join(sessionDirectory, "internal.m3u8"))
		if err != nil || !strings.Contains(string(manifest), "#EXTINF:2.000000") || !strings.Contains(string(manifest), "#EXTINF:4.000000") {
			t.Fatalf("HA-05 %s schedule manifest = %q, %v", backend, manifest, err)
		}
	}
	if released := request(http.MethodPost, Prefix+"session/"+state.Lease+"/release"); released.Code != http.StatusNoContent {
		t.Fatalf("release = %d", released.Code)
	}
	m.Close()
	closed = true
	entries, err := os.ReadDir(filepath.Join(root, "cache", "tmp", "hls"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("playback temporary directories retained: %v %v", entries, err)
	}
}

func writeStoredTarVideo(t *testing.T, destination, clip string) {
	t.Helper()
	input, err := os.Open(clip)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		t.Fatal(err)
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(output)
	if err := writer.WriteHeader(&tar.Header{Name: "video/clip.mp4", Mode: 0600, Size: info.Size(), ModTime: info.ModTime()}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(writer, input); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeStoredSevenZIPVideo(t *testing.T, destination, clip string) string {
	t.Helper()
	tool, err := exec.LookPath("7z")
	if err != nil {
		t.Fatal("HA-03 7z host gate requires 7z")
	}
	member := filepath.Base(clip)
	command := exec.Command(tool, "a", "-t7z", "-mx=0", "-mhc=on", destination, member)
	command.Dir = filepath.Dir(clip)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("7z fixture: %v: %s", err, output)
	}
	return member
}

func copyTestFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
}
