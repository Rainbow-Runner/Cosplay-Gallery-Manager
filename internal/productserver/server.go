package productserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/stashapp/stash/internal/cosermetadata"
	"github.com/stashapp/stash/internal/imageresource"
	"github.com/stashapp/stash/internal/mediaaccess"
	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/mediaresource"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/processingworker"
	"github.com/stashapp/stash/internal/productapi"
	"github.com/stashapp/stash/internal/productauth"
	"github.com/stashapp/stash/internal/productlog"
	"github.com/stashapp/stash/internal/videoresource"
	"github.com/stashapp/stash/pkg/ffmpeg"
	productweb "github.com/stashapp/stash/ui/web"
)

type Config struct {
	Listen                  string `json:"listen"`
	DatabasePath            string `json:"database_path"`
	CachePath               string `json:"cache_path"`
	WebRoot                 string `json:"web_root"`
	FFmpegPath              string `json:"ffmpeg_path"`
	FFprobePath             string `json:"ffprobe_path"`
	LibRawPath              string `json:"libraw_path"`
	WorkerCount             int    `json:"worker_count"`
	LogLevel                string `json:"log_level"`
	MetadataScrapingEnabled bool   `json:"metadata_scraping_enabled"`
}

func DefaultConfig() Config {
	return Config{Listen: "127.0.0.1:9999", DatabasePath: "cosplay-gallery-manager.sqlite", CachePath: "cache", WorkerCount: 1, LogLevel: "INFO"}
}

func LoadConfig(path string) (Config, error) {
	value := DefaultConfig()
	file, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return Config{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Config{}, errors.New("startup configuration must contain exactly one JSON object")
	}
	if err := value.Validate(); err != nil {
		return Config{}, err
	}
	return value, nil
}

func (c Config) Validate() error {
	host, port, err := net.SplitHostPort(c.Listen)
	if err != nil || host == "" || port == "" {
		return errors.New("listen must be an explicit host:port address")
	}
	if c.DatabasePath == "" || c.CachePath == "" {
		return errors.New("database_path and cache_path are required")
	}
	if c.WorkerCount < 1 || c.WorkerCount > 8 {
		return errors.New("worker_count must be between 1 and 8")
	}
	if _, err := productlog.ParseLevel(c.LogLevel); err != nil {
		return err
	}
	return nil
}

type Server struct {
	Config        Config
	Database      *productdb.Database
	Auth          *productauth.Service
	Handler       http.Handler
	CoserMetadata *cosermetadata.Service
	VideoTools    mediaprocessing.VideoToolchain

	handlerSwitch      *switchHandler
	operationMu        sync.Mutex
	workerMu           sync.Mutex
	workerCancel       context.CancelFunc
	workerDone         sync.WaitGroup
	workerRoot         context.Context
	mediaMetadataOnce  sync.Once
	mediaMetadataSlots chan struct{}

	restoreAfterDatabaseSwapHook func() error
}

func Open(config Config) (*Server, error) {
	return OpenWithMetadata(config)
}

func OpenWithMetadata(config Config, providers ...cosermetadata.Provider) (*Server, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	database, err := productdb.Open(context.Background(), config.DatabasePath)
	if err != nil {
		return nil, err
	}
	server, err := NewWithMetadata(config, database, productauth.New(database), providers...)
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	return server, nil
}

func New(config Config, database *productdb.Database, auth *productauth.Service) *Server {
	server, err := NewWithMetadata(config, database, auth)
	if err != nil {
		panic(err)
	}
	return server
}

func NewWithMetadata(config Config, database *productdb.Database, auth *productauth.Service, providers ...cosermetadata.Provider) (*Server, error) {
	if err := os.MkdirAll(filepath.Join(config.CachePath, "tmp"), 0o700); err != nil {
		return nil, err
	}
	registry, err := cosermetadata.NewRegistry(providers...)
	if err != nil {
		return nil, err
	}
	tools := mediaprocessing.ResolveVideoToolchain(context.Background(), config.FFmpegPath, config.FFprobePath)
	server := &Server{Config: config, Database: database, Auth: auth, CoserMetadata: &cosermetadata.Service{Registry: registry}, VideoTools: tools, handlerSwitch: &switchHandler{}}
	server.Handler = server.handlerSwitch
	server.rebuildHandler()
	return server, nil
}

func (s *Server) rebuildHandler() {
	database, auth := s.Database, s.Auth
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(response http.ResponseWriter, _ *http.Request) { response.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("/readyz", func(response http.ResponseWriter, request *http.Request) {
		if database.PingContext(request.Context()) != nil {
			http.Error(response, "not ready", http.StatusServiceUnavailable)
			return
		}
		response.WriteHeader(http.StatusNoContent)
	})
	mux.Handle("/about.json", aboutHandler())
	mux.Handle("/session/login", sameOrigin(auth.LoginHandler()))
	mux.Handle("/session/logout", sameOrigin(auth.LogoutHandler()))
	mux.Handle("/session/status", auth.SessionStatusHandler())
	mux.Handle("/setup/status", auth.SetupStatusHandler())
	mux.Handle("/setup/ticket/exchange", sameOrigin(auth.SetupTicketExchangeHandler()))
	listenHost, _, _ := net.SplitHostPort(s.Config.Listen)
	directLoopbackSetup := net.ParseIP(listenHost) != nil && net.ParseIP(listenHost).IsLoopback()
	mux.Handle("/setup/complete", sameOrigin(auth.CompleteSetupHandler(directLoopbackSetup)))
	mux.Handle("/graphql", sameOrigin(productapi.NewHandlerWithServices(database, auth.AuthorizeRequest, s, auth)))
	mux.Handle(coserAssetReviewPath, sameOrigin(s.coserAssetReviewHandler(database)))
	mux.Handle(coserAssetUploadPrefix, sameOrigin(s.coserAssetUploadHandler(database)))
	mux.Handle(coserAssetResourcePrefix, s.coserAssetResourceHandler(database))
	mux.Handle(coserMetadataPrefix, sameOrigin(s.coserMetadataHandler(database)))
	mux.Handle("/maintenance/status", s.maintenanceStatusHandler(database, auth))
	mux.Handle("/maintenance/path-mappings", sameOrigin(s.maintenancePathMappingsHandler()))
	mux.Handle("/maintenance/resume", sameOrigin(s.maintenanceResumeHandler()))
	resourceHandler := mediaresource.Handler{Database: database, Cache: mediaprocessing.CacheWriter{Root: s.Config.CachePath},
		Access: func(request *http.Request) (productdb.ResourceAccess, error) {
			return productdb.ResourceAccess{Authenticated: auth.AuthorizeRequest(request), Mode: productdb.ResourceBrowse,
				Scope: productdb.ResourceScopeAll}, nil
		}}
	mux.Handle("/resource/", resourceHandler)
	directImageHandler := imageresource.Handler{Database: database, Materializer: mediaaccess.Materializer{TemporaryRoot: filepath.Join(s.Config.CachePath, "tmp")}, Access: func(request *http.Request) (productdb.ResourceAccess, error) {
		return productdb.ResourceAccess{Authenticated: auth.AuthorizeRequest(request), Mode: productdb.ResourceBrowse, Scope: productdb.ResourceScopeAll}, nil
	}}
	mux.Handle(imageresource.RoutePrefix, directImageHandler)
	directVideoHandler := videoresource.Handler{Database: database, Access: func(request *http.Request) (productdb.ResourceAccess, error) {
		return productdb.ResourceAccess{Authenticated: auth.AuthorizeRequest(request), Mode: productdb.ResourceBrowse, Scope: productdb.ResourceScopeAll}, nil
	}}
	mux.Handle(videoresource.RoutePrefix, directVideoHandler)
	if s.Config.WebRoot != "" {
		mux.Handle("/", spaHandler(s.Config.WebRoot))
	} else if embeddedWeb, ok := productweb.FileSystem(); ok {
		mux.Handle("/", spaHandlerFS(embeddedWeb))
	}
	s.handlerSwitch.Set(requestLogger(securityHeaders(s.maintenanceGate(database, mux))))
}

func (s *Server) HTTPServer() *http.Server {
	return &http.Server{Addr: s.Config.Listen, Handler: s.Handler, ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout: 30 * time.Second, WriteTimeout: 2 * time.Minute, IdleTimeout: 2 * time.Minute}
}

func (s *Server) Close() error {
	s.stopWorkers()
	return s.Database.Close()
}

// RunWorkers starts local derivative workers. They consume only internal
// Gallery jobs and never accept a shell command or perform network access.
func (s *Server) RunWorkers(ctx context.Context) error {
	s.workerMu.Lock()
	s.workerRoot = ctx
	s.workerMu.Unlock()
	state, err := s.Database.Operations().Maintenance(ctx)
	if err != nil {
		return err
	}
	if state.Mode != productdb.MaintenanceNormal {
		return nil
	}
	if err := s.Database.Derivatives().AdoptOnDemandLightboxPolicy(ctx); err != nil {
		return err
	}
	return s.startWorkers(ctx)
}

func (s *Server) startWorkers(ctx context.Context) error {
	s.workerMu.Lock()
	defer s.workerMu.Unlock()
	if s.workerCancel != nil {
		return nil
	}
	temporaryRoot := filepath.Join(s.Config.CachePath, "tmp")
	if err := os.MkdirAll(temporaryRoot, 0o700); err != nil {
		return err
	}
	generators := []mediaprocessing.Generator{mediaprocessing.ImageGenerator{}}
	if s.Config.LibRawPath != "" {
		generators = append(generators, mediaprocessing.LibRawGenerator{Executable: s.Config.LibRawPath})
	}
	if s.VideoTools.FFmpeg.Available {
		encoder := ffmpeg.NewEncoder(s.VideoTools.FFmpeg.Path)
		generators = append(generators, mediaprocessing.FFmpegPosterGenerator{Encoder: encoder}, mediaprocessing.AnimatedPreviewGenerator{Encoder: encoder}, mediaprocessing.VideoPlaybackGenerator{Encoder: encoder})
	}
	slog.Info("CGM_WORKERS_STARTED",
		"worker_count", s.Config.WorkerCount,
		"libraw_enabled", s.Config.LibRawPath != "",
		"ffmpeg_enabled", s.VideoTools.FFmpeg.Available,
		"ffprobe_enabled", s.VideoTools.FFprobe.Available,
	)
	worker := processingworker.Worker{Database: s.Database, Materializer: mediaaccess.Materializer{TemporaryRoot: temporaryRoot}, Cache: mediaprocessing.CacheWriter{Root: s.Config.CachePath}, Generators: generators,
		ProbeProfileHash: mediaprocessing.VideoProbeProfileHash(s.VideoTools.FFprobe.Version), PosterProfileHash: mediaprocessing.VideoPosterProfileHash(s.VideoTools.FFmpeg.Version),
		ProbeUnavailableCode: s.VideoTools.FFprobe.ErrorCode, FFmpegUnavailableCode: s.VideoTools.FFmpeg.ErrorCode}
	if s.VideoTools.FFprobe.Available {
		worker.VideoProbe = mediaprocessing.ProbeAdapter{Executable: s.VideoTools.FFprobe.Path, Version: s.VideoTools.FFprobe.Version}
	}
	workerContext, cancel := context.WithCancel(ctx)
	s.workerCancel = cancel
	for index := 0; index < s.Config.WorkerCount; index++ {
		owner := "local-worker-" + string(rune('1'+index))
		s.workerDone.Add(1)
		go func() {
			defer s.workerDone.Done()
			runWorkerLoop(workerContext, worker, owner)
		}()
	}
	s.workerDone.Add(1)
	go func() {
		defer s.workerDone.Done()
		s.runSchedulerLoop(workerContext)
	}()
	return nil
}

func (s *Server) stopWorkers() {
	s.workerMu.Lock()
	cancel := s.workerCancel
	s.workerCancel = nil
	s.workerMu.Unlock()
	if cancel != nil {
		cancel()
		s.workerDone.Wait()
	}
}

type switchHandler struct {
	mu      sync.RWMutex
	current http.Handler
}

func (h *switchHandler) Set(value http.Handler) {
	h.mu.Lock()
	h.current = value
	h.mu.Unlock()
}

func (h *switchHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	h.mu.RLock()
	current := h.current
	h.mu.RUnlock()
	if current == nil {
		http.Error(response, "not ready", http.StatusServiceUnavailable)
		return
	}
	current.ServeHTTP(response, request)
}

func runWorkerLoop(ctx context.Context, worker processingworker.Worker, owner string) {
	for ctx.Err() == nil {
		started := time.Now()
		job, err := worker.RunOne(ctx, owner, 30*time.Second, time.Now())
		if err == nil {
			logMediaJobResult(job, true, "", time.Since(started))
			continue
		}
		if errors.Is(err, productdb.ErrJobNotClaimable) {
			timer := time.NewTimer(500 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
			case <-timer.C:
			}
			continue
		}
		if ctx.Err() == nil {
			logMediaJobResult(job, false, processingworker.ErrorCode(err), time.Since(started))
		}
	}
}

func logMediaJobResult(job mediaprocessing.Job, success bool, errorCode string, elapsed time.Duration) {
	event := "CGM_MEDIA_JOB_COMPLETED"
	if !success {
		event = "CGM_MEDIA_JOB_FAILED"
	}
	if job.Kind == mediaprocessing.JobItemTechnicalMetadata {
		event = "CGM_VIDEO_PROBE_COMPLETED"
		if !success {
			event = "CGM_VIDEO_PROBE_FAILED"
		}
	} else if job.Variant == mediaprocessing.VariantStaticPoster {
		event = "CGM_VIDEO_POSTER_COMPLETED"
		if !success {
			event = "CGM_VIDEO_POSTER_FAILED"
		}
	} else if job.Variant == mediaprocessing.VariantVideoPlayback {
		event = "CGM_VIDEO_PROXY_COMPLETED"
		if !success {
			event = "CGM_VIDEO_PROXY_FAILED"
		}
	}
	item := job.ItemUUID
	if len(item) > 8 {
		item = item[:8]
	}
	attributes := []any{"job_id", job.ID, "item", item, "variant", job.Variant, "elapsed_ms", elapsed.Milliseconds()}
	if errorCode != "" {
		attributes = append(attributes, "error_code", errorCode)
	}
	if success {
		slog.Info(event, attributes...)
	} else {
		slog.Error(event, attributes...)
	}
}

var requestSequence atomic.Uint64

type responseMetrics struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *responseMetrics) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseMetrics) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	count, err := w.ResponseWriter.Write(data)
	w.bytes += count
	return count, err
}

func (w *responseMetrics) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requestID := fmt.Sprintf("req-%016x", requestSequence.Add(1))
		response.Header().Set("X-Request-ID", requestID)
		metrics := &responseMetrics{ResponseWriter: response}
		started := time.Now()
		next.ServeHTTP(metrics, request.WithContext(productlog.WithRequestID(request.Context(), requestID)))
		status := metrics.status
		if status == 0 {
			status = http.StatusOK
		}
		attributes := []any{
			"request_id", requestID,
			"endpoint", endpointCategory(request.URL.Path),
			"method", request.Method,
			"status", status,
			"duration_ms", time.Since(started).Milliseconds(),
			"response_bytes", metrics.bytes,
		}
		if status >= http.StatusInternalServerError {
			slog.Error("CGM_HTTP_REQUEST", attributes...)
		} else {
			slog.Debug("CGM_HTTP_REQUEST", attributes...)
		}
	})
}

func endpointCategory(path string) string {
	switch {
	case path == "/graphql":
		return "GRAPHQL"
	case strings.HasPrefix(path, "/resource/"):
		return "MEDIA_RESOURCE"
	case strings.HasPrefix(path, "/manage/coser-assets/"):
		return "COSER_ASSET"
	case strings.HasPrefix(path, "/session/"):
		return "SESSION"
	case strings.HasPrefix(path, "/setup/"):
		return "SETUP"
	case strings.HasPrefix(path, "/maintenance/"):
		return "MAINTENANCE"
	case path == "/healthz" || path == "/readyz":
		return "HEALTH"
	case path == "/about.json":
		return "ABOUT"
	case strings.HasPrefix(path, "/assets/"):
		return "WEB_ASSET"
	default:
		return "WEB_ROUTE"
	}
}

func sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead && request.Method != http.MethodOptions {
			origin := request.Header.Get("Origin")
			if origin != "" {
				expectedHTTP := "http://" + request.Host
				expectedHTTPS := "https://" + request.Host
				if origin != expectedHTTP && origin != expectedHTTPS {
					http.Error(response, "cross-origin request rejected", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(response, request)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("X-Content-Type-Options", "nosniff")
		response.Header().Set("X-Frame-Options", "DENY")
		response.Header().Set("Referrer-Policy", "no-referrer")
		response.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' blob:; style-src 'self'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'")
		next.ServeHTTP(response, request)
	})
}

func spaHandler(root string) http.Handler {
	return spaHandlerFS(os.DirFS(root))
}

func spaHandlerFS(fileSystem fs.FS) http.Handler {
	files := http.FileServer(http.FS(fileSystem))
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		clean := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(request.URL.Path)), "/")
		if clean == "." || clean == "" {
			clean = "index.html"
		}
		if info, err := fs.Stat(fileSystem, clean); err == nil && !info.IsDir() {
			if strings.HasPrefix(clean, "assets/") {
				response.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else if strings.HasSuffix(clean, ".html") {
				response.Header().Set("Cache-Control", "no-cache")
			}
			files.ServeHTTP(response, request)
			return
		}
		index, err := fs.ReadFile(fileSystem, "index.html")
		if err != nil {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Content-Type", "text/html; charset=utf-8")
		response.Header().Set("Cache-Control", "no-cache")
		_, _ = response.Write(index)
	})
}
