// Package videoplayback owns short-lived, authenticated progressive playback.
// Durable output is an ordinary VIDEO_PLAYBACK MP4, not a new media identity.
package videoplayback

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/mediaaccess"
	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portableid"
	"github.com/stashapp/stash/pkg/ffmpeg"
	"golang.org/x/sys/unix"
)

const Prefix = "/playback/"
const segmentSeconds = 4
const idleTimeout = 45 * time.Second

var tokenPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
var segmentPattern = regexp.MustCompile(`^segment-[0-9]{6}\.ts$`)
var segmentTemporaryPattern = regexp.MustCompile(`^segment-[0-9]{6}\.ts\.tmp$`)

type State struct {
	Mode      string                        `json:"mode"`
	Backend   string                        `json:"backend,omitempty"`
	Status    string                        `json:"status"`
	URL       string                        `json:"url,omitempty"`
	Lease     string                        `json:"lease,omitempty"`
	ErrorCode string                        `json:"errorCode,omitempty"`
	Progress  mediaprocessing.VideoProgress `json:"progress"`
}
type session struct {
	key, dir, profile string
	descriptor        productdb.DirectVideoDescriptor
	execution         mediaprocessing.VideoTranscodeExecutionPlan
	owner             [32]byte
	request           *http.Request
	ctx               context.Context
	cancel            context.CancelFunc
	seek              chan int
	state             State
	start, produced   int
	started           time.Time
	firstSegment      bool
	fallbackAttempted bool
	leases            map[string]time.Time
}
type Manager struct {
	DB                    *productdb.Database
	Cache                 mediaprocessing.CacheWriter
	Encoder               *ffmpeg.FFMpeg
	Version               string
	Authorize             func(*http.Request) bool
	HardwareStatus        func() mediaprocessing.HardwareAccelerationStatus
	RecordHardwareFailure func(string)
	mu                    sync.Mutex
	sessions              map[string]*session
	leases                map[string]*session
	ctx                   context.Context
	cancel                context.CancelFunc
	wg                    sync.WaitGroup
	root                  string
}

func New(db *productdb.Database, root, executable, version string, authorize func(*http.Request) bool) (*Manager, error) {
	m := &Manager{DB: db, Cache: mediaprocessing.CacheWriter{Root: root}, Version: version, Authorize: authorize, sessions: map[string]*session{}, leases: map[string]*session{}}
	if executable != "" {
		m.Encoder = ffmpeg.NewEncoder(executable)
	}
	m.root = filepath.Join(root, "tmp", "hls")
	for _, p := range []string{root, filepath.Join(root, "tmp"), m.root} {
		if err := os.MkdirAll(p, 0700); err != nil {
			return nil, err
		}
		info, err := os.Lstat(p)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("unsafe playback cache")
		}
	}
	entries, err := os.ReadDir(m.root)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "cgm-hls-") && tokenPattern.MatchString(strings.TrimPrefix(e.Name(), "cgm-hls-")) {
			if err := os.RemoveAll(filepath.Join(m.root, e.Name())); err != nil {
				return nil, err
			}
		}
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.wg.Add(1)
	go m.monitor()
	return m, nil
}

func (m *Manager) CancelAll() {
	m.mu.Lock()
	for _, s := range m.sessions {
		s.cancel()
	}
	m.mu.Unlock()
}
func (m *Manager) Close() { m.cancel(); m.CancelAll(); m.wg.Wait() }
func owner(r *http.Request) [32]byte {
	cookie, err := r.Cookie("cgm_session")
	if err == nil {
		return sha256.Sum256([]byte(cookie.Value))
	}
	// Trusted local mode has no login cookie; authorization is still checked
	// on every request before this key is used.
	return sha256.Sum256(nil)
}
func randomToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
func access() productdb.ResourceAccess {
	return productdb.ResourceAccess{Authenticated: true, Mode: productdb.ResourceBrowse, Scope: productdb.ResourceScopeAll}
}
func (m *Manager) valid(s *session) bool {
	r := s.request.Clone(m.ctx)
	if !m.Authorize(r) {
		return false
	}
	state, err := m.DB.Operations().Maintenance(m.ctx)
	if err != nil || state.Mode != productdb.MaintenanceNormal {
		return false
	}
	_, err = m.DB.MediaResources().AuthorizeVideoPlaybackInput(m.ctx, access(), s.descriptor.ItemUUID, s.descriptor.ContentRevision)
	return err == nil
}

func (m *Manager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if !m.Authorize(r) {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, Prefix), "/")
	if len(parts) == 2 && parts[0] == "video" && r.Method == http.MethodPost {
		if _, err := portableid.Parse(parts[1]); err != nil {
			http.NotFound(w, r)
			return
		}
		state, err := m.start(r, parts[1])
		if err != nil {
			http.Error(w, "VIDEO_PLAYBACK_UNAVAILABLE", http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(state)
		return
	}
	if len(parts) != 3 || parts[0] != "session" || !tokenPattern.MatchString(parts[1]) {
		http.NotFound(w, r)
		return
	}
	m.mu.Lock()
	s := m.leases[parts[1]]
	if s == nil || s.owner != owner(r) {
		m.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	s.leases[parts[1]] = time.Now()
	m.mu.Unlock()
	if !m.valid(s) {
		s.cancel()
		http.NotFound(w, r)
		return
	}
	if parts[2] == "release" && r.Method == http.MethodPost {
		m.mu.Lock()
		delete(s.leases, parts[1])
		delete(m.leases, parts[1])
		if len(s.leases) == 0 {
			s.cancel()
		}
		m.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if parts[2] == "status" {
		m.mu.Lock()
		state := s.state
		m.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(state)
		return
	}
	if parts[2] == "index.m3u8" {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		if r.Method == http.MethodGet {
			_, _ = io.WriteString(w, playlist(s.descriptor.Metadata.DurationSeconds))
		}
		return
	}
	if !segmentPattern.MatchString(parts[2]) {
		http.NotFound(w, r)
		return
	}
	index, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(parts[2], "segment-"), ".ts"))
	if index >= int(math.Ceil(s.descriptor.Metadata.DurationSeconds/segmentSeconds)) {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		file, info, err := m.Cache.OpenGenerated(filepath.ToSlash(filepath.Join("tmp", "hls", filepath.Base(s.dir), parts[2])))
		if err == nil {
			defer file.Close()
			if !m.valid(s) {
				http.NotFound(w, r)
				return
			}
			m.mu.Lock()
			if s.state.Status == "PROCESSING" {
				s.state.Status = "STREAMING"
			}
			m.mu.Unlock()
			w.Header().Set("Content-Type", "video/mp2t")
			http.ServeContent(w, r, "", info.ModTime(), file)
			return
		}
		m.mu.Lock()
		failed := s.state.Status == "ERROR"
		start, produced := s.start, s.produced
		m.mu.Unlock()
		if failed {
			http.Error(w, "VIDEO_HLS_FAILED", http.StatusConflict)
			return
		}
		if index < start || index > produced+6 {
			select {
			case s.seek <- index:
			default:
			}
		}
		select {
		case <-ctx.Done():
			http.Error(w, "VIDEO_HLS_WAIT_TIMEOUT", http.StatusGatewayTimeout)
			return
		case <-s.ctx.Done():
			http.NotFound(w, r)
			return
		case <-ticker.C:
		}
	}
}

func playlist(duration float64) string {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:4\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-INDEPENDENT-SEGMENTS\n")
	for i := 0; i < int(math.Ceil(duration/segmentSeconds)); i++ {
		fmt.Fprintf(&b, "#EXTINF:%.6f,\nsegment-%06d.ts\n", math.Min(segmentSeconds, duration-float64(i*segmentSeconds)), i)
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	return b.String()
}

func (m *Manager) start(r *http.Request, item string) (State, error) {
	began := time.Now()
	defer mediaprocessing.VideoStage(item, "PLAYBACK_REQUEST", began)
	runtimeSettings, err := m.DB.Settings().Find(r.Context())
	if err != nil {
		return State{}, err
	}
	hardware := mediaprocessing.HardwareAccelerationStatus{}
	if m.HardwareStatus != nil {
		hardware = m.HardwareStatus()
	}
	if runtimeSettings.VideoHardwareMode != "SOFTWARE" && hardware.ProbeState != mediaprocessing.HardwareProbeCompleted {
		return State{Status: "ERROR", ErrorCode: "VIDEO_HARDWARE_PROBE_PENDING"}, nil
	}
	preference := mediaprocessing.VideoHardwarePreference{Mode: string(runtimeSettings.VideoHardwareMode), Device: runtimeSettings.VideoHardwareDevice, AllowSoftwareFallback: runtimeSettings.VideoHardwareFallbackEnabled}
	playbackRuntime := productdb.VideoPlaybackRuntime{FFmpegVersion: m.Version, FFmpegUnavailableCode: mediaprocessing.ErrorFFmpegUnavailable, Preference: preference, Hardware: hardware}
	status, err := m.DB.Browse().VideoPlaybackStatusWithRuntime(r.Context(), item, playbackRuntime)
	if err != nil {
		return State{}, err
	}
	if status.Status == gallery.ProcessingError {
		if shouldRetryCompleteProxy(status.ErrorCode) {
			return State{}, errors.New("previous proxy must be retried through the existing job")
		}
		return State{Status: "ERROR", ErrorCode: status.ErrorCode}, nil
	}
	if status.Status == gallery.ProcessingReady && status.Mode == string(mediaprocessing.PlaybackDirect) {
		return State{Status: "READY", Mode: "DIRECT", URL: fmt.Sprintf("/resource/video/%s/%d/direct", item, status.ContentRevision)}, nil
	}
	if status.Resource != nil {
		if cached, err := m.DB.Derivatives().Current(r.Context(), item, mediaprocessing.VariantVideoPlayback, time.Now()); err == nil && cached.ContentRevision == status.ContentRevision && cached.ProfileHash == status.Resource.ProfileHash {
			if file, _, err := m.Cache.OpenGenerated(cached.CacheRelativePath); err == nil {
				_ = file.Close()
				return State{Status: "READY", Mode: "MP4_PROXY", URL: fmt.Sprintf("/resource/item/%s/%d/%s/VIDEO_PLAYBACK", item, status.ContentRevision, url.PathEscape(status.Resource.ProfileHash))}, nil
			}
		}
	}
	d, err := m.DB.MediaResources().AuthorizeVideoPlaybackInput(r.Context(), access(), item, status.ContentRevision)
	if err != nil {
		return State{}, err
	}
	plan := mediaprocessing.PlaybackPlanFromMetadata(d.Metadata)
	if d.Source.Type == gallery.SourceTypeArchive {
		plan = mediaprocessing.ArchivePlaybackPlanFromMetadata(d.Metadata)
	}
	if plan.Mode == mediaprocessing.PlaybackRemux {
		return State{}, errors.New("remux uses complete MP4 proxy")
	}
	execution, executionError := hlsExecutionPlan(plan, d.Metadata, preference, hardware)
	if executionError != "" {
		return State{Status: "ERROR", ErrorCode: executionError}, nil
	}
	if preference.Mode != "SOFTWARE" && execution.EffectiveBackend == "SOFTWARE" {
		slog.Info("CGM_VIDEO_HARDWARE_PLANNING_FALLBACK", "item", shortItem(d.ItemUUID), "requested_backend", preference.Mode, "reason", execution.ReasonCode)
	}
	profile := mediaprocessing.ProgressiveVideoProfileHash(d.Metadata, plan, m.Version, execution, segmentSeconds, d.ContentRevision)
	if cached, err := m.DB.Derivatives().Current(r.Context(), item, mediaprocessing.VariantVideoPlayback, time.Now()); err == nil && cached.ContentRevision == d.ContentRevision && cached.ProfileHash == profile {
		file, _, err := m.Cache.OpenGenerated(cached.CacheRelativePath)
		if err == nil {
			file.Close()
			return State{Status: "READY", Mode: "MP4_PROXY", URL: fmt.Sprintf("/resource/item/%s/%d/%s/VIDEO_PLAYBACK", item, d.ContentRevision, url.PathEscape(profile))}, nil
		}
	}
	if m.Encoder == nil {
		return State{Status: "ERROR", ErrorCode: mediaprocessing.ErrorFFmpegUnavailable}, nil
	}
	if d.Metadata.DurationSeconds <= 0 || d.Metadata.DurationSeconds > 6*3600 || math.IsNaN(d.Metadata.DurationSeconds) || math.IsInf(d.Metadata.DurationSeconds, 0) {
		return State{}, errors.New("unsupported duration")
	}
	// Existing complete-file work must finish rather than duplicating the input.
	var jobs int
	if err := m.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM processing_jobs WHERE item_uuid=? AND content_revision=? AND variant='VIDEO_PLAYBACK' AND status IN ('PENDING','RUNNING','RETRY_WAIT')`, item, d.ContentRevision).Scan(&jobs); err != nil {
		return State{}, err
	}
	if jobs > 0 {
		return State{}, errors.New("complete proxy already running")
	}
	key := fmt.Sprintf("%x/%s/%d/%s", owner(r), item, d.ContentRevision, profile)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ctx.Err() != nil {
		return State{}, errors.New("playback service stopped")
	}
	s := m.sessions[key]
	if s != nil && s.ctx.Err() != nil {
		return State{}, errors.New("playback closing")
	}
	lease, err := randomToken()
	if err != nil {
		return State{}, err
	}
	if m.leases[lease] != nil {
		return State{}, errors.New("playback lease collision")
	}
	if s == nil {
		if len(m.sessions) >= 8 {
			return State{}, errors.New("playback queue full")
		}
		dirToken, err := randomToken()
		if err != nil {
			return State{}, err
		}
		dir := filepath.Join(m.root, "cgm-hls-"+dirToken)
		if err := os.Mkdir(dir, 0700); err != nil {
			return State{}, err
		}
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Minute)
		s = &session{key: key, dir: dir, profile: profile, descriptor: d, execution: execution, owner: owner(r), request: r.Clone(m.ctx), ctx: ctx, cancel: cancel, seek: make(chan int, 1), state: State{Mode: "HLS_SESSION", Backend: execution.EffectiveBackend, Status: "PENDING"}, leases: map[string]time.Time{}, started: time.Now()}
		m.sessions[key] = s
		m.wg.Add(1)
		go m.run(s)
	}
	s.leases[lease] = time.Now()
	m.leases[lease] = s
	state := s.state
	state.Lease = lease
	state.URL = Prefix + "session/" + lease + "/index.m3u8"
	return state, nil
}

func hlsExecutionPlan(plan mediaprocessing.VideoPlaybackPlan, metadata mediaprocessing.VideoTechnicalMetadata, preference mediaprocessing.VideoHardwarePreference, hardware mediaprocessing.HardwareAccelerationStatus) (mediaprocessing.VideoTranscodeExecutionPlan, string) {
	execution := mediaprocessing.PlanVideoTranscode(plan, metadata, preference, hardware, mediaprocessing.VideoTranscodeHLS)
	// VAAPI remains a diagnosed/plannable HA-02 backend. HA-03 executes only
	// NVENC; an explicit no-fallback VAAPI selection must not run silently.
	if execution.EffectiveBackend == "VAAPI" {
		if !preference.AllowSoftwareFallback {
			return execution, "VIDEO_HARDWARE_BACKEND_NOT_IMPLEMENTED"
		}
		execution = mediaprocessing.PlanVideoTranscode(plan, metadata, mediaprocessing.VideoHardwarePreference{Mode: "SOFTWARE", AllowSoftwareFallback: true}, hardware, mediaprocessing.VideoTranscodeHLS)
		execution.ReasonCode = "HARDWARE_EXECUTOR_NOT_IMPLEMENTED"
	}
	if !execution.Executable {
		return execution, "VIDEO_HARDWARE_UNAVAILABLE"
	}
	return execution, ""
}

func shouldRetryCompleteProxy(code string) bool {
	if code == "" || strings.HasPrefix(code, "ARCHIVE_VIDEO_") {
		return false
	}
	switch code {
	case mediaprocessing.ErrorFFmpegUnavailable, mediaprocessing.ErrorFFmpegVersionUnsupported,
		mediaprocessing.ErrorFFprobeUnavailable, mediaprocessing.ErrorFFprobeVersionUnsupported,
		mediaprocessing.ErrorVideoTrackMissing, mediaprocessing.ErrorVideoProbeFailed:
		return false
	default:
		return true
	}
}

func (m *Manager) run(s *session) {
	defer m.wg.Done()
	defer func() {
		m.mu.Lock()
		for token := range s.leases {
			delete(m.leases, token)
		}
		delete(m.sessions, s.key)
		m.mu.Unlock()
		if err := os.RemoveAll(s.dir); err != nil {
			slog.Warn("CGM_VIDEO_HLS_CLEANUP_FAILED", "item", shortItem(s.descriptor.ItemUUID))
		} else {
			slog.Info("CGM_VIDEO_HLS_CLEANED", "item", shortItem(s.descriptor.ItemUUID), "elapsed_ms", time.Since(s.started).Milliseconds())
		}
	}()
	start := 0

encodeLoop:
	for {
		ctx, cancel := context.WithCancel(s.ctx)
		finished := make(chan error, 1)
		m.mu.Lock()
		s.start = start
		s.produced = start
		s.state.Status = "PROCESSING"
		m.mu.Unlock()
		go func(offset int) { finished <- m.generate(ctx, s, offset) }(start)
		select {
		case <-s.ctx.Done():
			cancel()
			<-finished
			return
		case next := <-s.seek:
			cancel()
			<-finished
			start = next
			continue
		case err := <-finished:
			cancel()
			if backend, hardwareFailure := mediaprocessing.HardwareExecutionFailure(err); hardwareFailure {
				fallback, fallbackErr := m.applyHLSFallback(s, backend)
				if fallbackErr != nil {
					err = fallbackErr
				} else if fallback {
					continue encodeLoop
				}
			}
			m.mu.Lock()
			if err != nil {
				s.state.Status = "ERROR"
				s.state.ErrorCode = hlsFailureCode(s.execution)
			} else {
				s.state.Status = "STREAMING"
				if completeSegments(s.dir, s.descriptor.Metadata.DurationSeconds) {
					s.state.Status = "READY"
				}
			}
			m.mu.Unlock()
			if err != nil {
				slog.Warn("CGM_VIDEO_HLS_FAILED", "item", s.descriptor.ItemUUID[:8], "backend", s.execution.EffectiveBackend)
			}
		}
		select {
		case <-s.ctx.Done():
			return
		case start = <-s.seek:
		}
	}
}

func (m *Manager) applyHLSFallback(s *session, backend string) (bool, error) {
	if !s.execution.AllowSoftwareFallback || s.fallbackAttempted {
		return false, nil
	}
	s.fallbackAttempted = true
	if m.RecordHardwareFailure != nil {
		m.RecordHardwareFailure(backend)
	}
	if err := clearHLSOutput(s.dir); err != nil {
		return false, err
	}
	plan := mediaprocessing.PlaybackPlanFromMetadata(s.descriptor.Metadata)
	if s.descriptor.Source.Type == gallery.SourceTypeArchive {
		plan = mediaprocessing.ArchivePlaybackPlanFromMetadata(s.descriptor.Metadata)
	}
	s.execution = mediaprocessing.SoftwareVideoTranscodePlan(plan, s.descriptor.Metadata, mediaprocessing.VideoTranscodeHLS)
	s.execution.ReasonCode = "HARDWARE_RUNTIME_FALLBACK"
	s.profile = mediaprocessing.ProgressiveVideoProfileHash(s.descriptor.Metadata, plan, m.Version, s.execution, segmentSeconds, s.descriptor.ContentRevision)
	m.mu.Lock()
	s.state.Backend, s.state.Status, s.state.ErrorCode, s.state.Progress = "SOFTWARE", "PENDING", "", mediaprocessing.VideoProgress{}
	m.mu.Unlock()
	slog.Warn("CGM_VIDEO_HARDWARE_RUNTIME_FALLBACK", "item", shortItem(s.descriptor.ItemUUID), "failed_backend", backend, "fallback_backend", "SOFTWARE")
	return true, nil
}

func clearHLSOutput(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || (!segmentPattern.MatchString(entry.Name()) && !segmentTemporaryPattern.MatchString(entry.Name()) && entry.Name() != "internal.m3u8" && entry.Name() != "internal.m3u8.tmp") {
			return errors.New("unexpected HLS temporary entry")
		}
		if err := os.Remove(filepath.Join(directory, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func hlsFailureCode(execution mediaprocessing.VideoTranscodeExecutionPlan) string {
	if execution.EffectiveBackend == "NVENC" {
		return "VIDEO_HARDWARE_HLS_FAILED"
	}
	return "VIDEO_HLS_FAILED"
}

func completeSegments(directory string, duration float64) bool {
	for i := 0; i < int(math.Ceil(duration/segmentSeconds)); i++ {
		info, err := os.Stat(filepath.Join(directory, fmt.Sprintf("segment-%06d.ts", i)))
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return false
		}
	}
	return true
}

func (m *Manager) generate(ctx context.Context, s *session, start int) error {
	queued := time.Now()
	release, err := mediaprocessing.AcquirePlaybackSlot(ctx)
	if err != nil {
		return err
	}
	defer release()
	mediaprocessing.VideoStage(s.descriptor.ItemUUID, "HLS_SLOT_WAIT", queued)
	if !m.valid(s) {
		return errors.New("source unavailable")
	}
	capacityStarted := time.Now()
	if err := m.capacity(ctx); err != nil {
		return err
	}
	mediaprocessing.VideoStage(s.descriptor.ItemUUID, "CAPACITY_CHECK", capacityStarted)
	began := time.Now()
	d := s.descriptor
	materializer := mediaaccess.Materializer{TemporaryRoot: filepath.Join(m.Cache.Root, "tmp")}
	var input mediaaccess.Materialized
	if d.Source.Type == gallery.SourceTypeArchive {
		input, err = materializer.OpenDirectArchiveVideo(ctx, d.Source, d.ArchiveLimits, d.ArchiveEvidence)
	} else {
		input, err = mediaaccess.OpenVerifiedDirectoryVideo(d.Source)
	}
	if err != nil {
		return err
	}
	defer input.Close()
	mediaprocessing.VideoStage(d.ItemUUID, "SOURCE_OPEN_AND_PROOF", began)
	plan := mediaprocessing.PlaybackPlanFromMetadata(d.Metadata)
	if d.Source.Type == gallery.SourceTypeArchive {
		plan = mediaprocessing.ArchivePlaybackPlanFromMetadata(d.Metadata)
	}
	// Fixed keyframe boundaries require encoding even when MP4 could remux.
	execution := s.execution
	args, err := mediaprocessing.ProgressiveVideoArgsForExecution(input.Path, s.dir, plan, execution, start, segmentSeconds)
	if err != nil {
		return err
	}
	slog.Info("CGM_VIDEO_HLS_EXECUTION_STARTED", "item", shortItem(d.ItemUUID), "backend", execution.EffectiveBackend, "decoder", execution.Decoder, "filter", execution.FilterStrategy, "encoder", execution.Encoder, "reason", execution.ReasonCode)
	err = mediaprocessing.RunVideoCommand(ctx, m.Encoder, args, d.ItemUUID, "HLS_ENCODE", d.Metadata.DurationSeconds, func(p mediaprocessing.VideoProgress) {
		m.mu.Lock()
		if start > 0 {
			p.Seconds = math.Max(float64(start*segmentSeconds), p.Seconds)
		}
		s.state.Progress = p
		s.produced = int(p.Seconds / segmentSeconds)
		m.mu.Unlock()
	})
	if err != nil {
		if execution.EffectiveBackend == "NVENC" && ctx.Err() == nil && mediaprocessing.HardwareVideoCommandFailure(err) {
			return &mediaprocessing.VideoHardwareExecutionError{Backend: execution.EffectiveBackend, Err: err}
		}
		return err
	}
	if err := input.Validate(); err != nil {
		return err
	}
	if start != 0 {
		return nil
	}
	segmentCheckStarted := time.Now()
	// Promote only a complete uninterrupted output. A seeked session remains
	// temporary until all required segments can be produced by a later run.
	for i := 0; i < int(math.Ceil(d.Metadata.DurationSeconds/segmentSeconds)); i++ {
		if _, err := os.Stat(filepath.Join(s.dir, fmt.Sprintf("segment-%06d.ts", i))); err != nil {
			return err
		}
	}
	mediaprocessing.VideoStage(d.ItemUUID, "HLS_SEGMENT_CHECK", segmentCheckStarted)
	if !m.valid(s) {
		return errors.New("authorization changed")
	}
	path, err := m.Cache.RelativePath(d.ItemUUID, d.ContentRevision, mediaprocessing.VariantVideoPlayback, s.profile, "mp4")
	if err != nil {
		return err
	}
	began = time.Now()
	_, size, err := m.Cache.WriteAtomicPath(path, func(destination string) error {
		args := []string{"-hide_banner", "-loglevel", "error", "-y", "-i", filepath.Join(s.dir, "internal.m3u8"), "-map", "0:v:0", "-map", "0:a:0?", "-c", "copy", "-movflags", "+faststart", "-f", "mp4", destination}
		return mediaprocessing.RunVideoCommand(ctx, m.Encoder, args, d.ItemUUID, "HLS_MP4_REMUX", d.Metadata.DurationSeconds, nil)
	})
	if err != nil {
		return err
	}
	mediaprocessing.VideoStage(d.ItemUUID, "HLS_TO_MP4_AND_SYNC", began)
	if !m.valid(s) || input.Validate() != nil {
		_ = m.Cache.RemoveUnpublished(path)
		return errors.New("source changed before publishing")
	}
	width, height := d.Metadata.DisplayWidth, d.Metadata.DisplayHeight
	ratio := math.Min(1, math.Min(float64(plan.MaximumWidth)/float64(width), float64(plan.MaximumHeight)/float64(height)))
	publishStarted := time.Now()
	_, err = m.DB.Derivatives().Publish(ctx, productdb.PublishDerivativeInput{ItemUUID: d.ItemUUID, Variant: mediaprocessing.VariantVideoPlayback, CacheTier: mediaprocessing.CacheEnhanced, ContentRevision: d.ContentRevision, ProfileHash: s.profile, CacheRelativePath: path, MIMEType: "video/mp4", ByteSize: size, Width: int(float64(width) * ratio), Height: int(float64(height) * ratio)}, time.Now())
	if err != nil {
		_ = m.Cache.RemoveUnpublished(path)
	} else {
		mediaprocessing.VideoStage(d.ItemUUID, "CACHE_REGISTER", publishStarted)
		slog.Info("CGM_VIDEO_HLS_CACHE_COMPLETED", "item", d.ItemUUID[:8], "elapsed_ms", time.Since(s.started).Milliseconds(), "bytes", size)
	}
	return err
}

func (m *Manager) capacity(ctx context.Context) error {
	settings, err := m.DB.Settings().Find(ctx)
	if err != nil {
		return err
	}
	_, enhanced, err := m.DB.Derivatives().CacheTierBytes(ctx)
	if err != nil {
		return err
	}
	var temporary int64
	err = filepath.WalkDir(m.root, func(p string, e os.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !e.IsDir() {
			info, err := e.Info()
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			temporary += info.Size()
		}
		return nil
	})
	if err != nil {
		return err
	}
	var stat unix.Statfs_t
	if err := unix.Statfs(m.Cache.Root, &stat); err != nil {
		return err
	}
	pressure := mediaprocessing.PlanCacheCleanup(mediaprocessing.CachePressure{EnhancedBytes: enhanced + temporary, MaximumEnhancedBytes: settings.EnhancedCacheMaximumBytes, AvailableBytes: int64(stat.Bavail) * int64(stat.Bsize), TotalBytes: int64(stat.Blocks) * int64(stat.Bsize), MinimumFreeBytes: settings.MinimumFreeBytes, MinimumFreePercent: settings.MinimumFreePercent})
	if temporary > 2<<30 || pressure.BytesToFree > 0 || pressure.PauseNewProcessing {
		return errors.New("playback cache capacity exhausted")
	}
	return nil
}

func (m *Manager) monitor() {
	defer m.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	tick := 0
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
		}
		tick++
		m.mu.Lock()
		list := make([]*session, 0, len(m.sessions))
		for _, s := range m.sessions {
			list = append(list, s)
		}
		m.mu.Unlock()
		for _, s := range list {
			m.mu.Lock()
			for token, t := range s.leases {
				if time.Since(t) > idleTimeout {
					delete(m.leases, token)
					delete(s.leases, token)
				}
			}
			idle := len(s.leases) == 0
			m.mu.Unlock()
			if idle {
				slog.Info("CGM_VIDEO_HLS_IDLE_CANCEL", "item", shortItem(s.descriptor.ItemUUID))
				s.cancel()
				continue
			}
			if tick%5 == 0 && !m.valid(s) {
				slog.Info("CGM_VIDEO_HLS_ACCESS_CANCEL", "item", shortItem(s.descriptor.ItemUUID))
				s.cancel()
				continue
			}
			if tick%5 == 0 {
				if err := m.capacity(s.ctx); err != nil {
					s.cancel()
					slog.Warn("CGM_VIDEO_HLS_CAPACITY_STOP", "item", shortItem(s.descriptor.ItemUUID))
					continue
				}
			}
			m.mu.Lock()
			first := s.firstSegment
			start := s.start
			m.mu.Unlock()
			if !first {
				if info, err := os.Stat(filepath.Join(s.dir, fmt.Sprintf("segment-%06d.ts", start))); err == nil && info.Size() > 0 {
					m.mu.Lock()
					s.firstSegment = true
					if s.state.Status == "PROCESSING" {
						s.state.Status = "STREAMING"
					}
					m.mu.Unlock()
					slog.Info("CGM_VIDEO_HLS_FIRST_SEGMENT_READY", "item", shortItem(s.descriptor.ItemUUID), "backend", s.execution.EffectiveBackend, "elapsed_ms", time.Since(s.started).Milliseconds(), "bytes", info.Size())
				}
			}
		}
	}
}

func shortItem(value string) string {
	if len(value) > 8 {
		return value[:8]
	}
	return value
}
