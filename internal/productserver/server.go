package productserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/stashapp/stash/internal/mediaaccess"
	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/mediaresource"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/processingworker"
	"github.com/stashapp/stash/internal/productapi"
	"github.com/stashapp/stash/internal/productauth"
	"github.com/stashapp/stash/pkg/ffmpeg"
	productweb "github.com/stashapp/stash/ui/web"
)

type Config struct {
	Listen       string `json:"listen"`
	DatabasePath string `json:"database_path"`
	CachePath    string `json:"cache_path"`
	WebRoot      string `json:"web_root"`
	FFmpegPath   string `json:"ffmpeg_path"`
	LibRawPath   string `json:"libraw_path"`
	WorkerCount  int    `json:"worker_count"`
}

func DefaultConfig() Config {
	return Config{Listen: "127.0.0.1:9999", DatabasePath: "cosplay-gallery-manager.sqlite", CachePath: "cache", WorkerCount: 1}
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
	return nil
}

type Server struct {
	Config   Config
	Database *productdb.Database
	Auth     *productauth.Service
	Handler  http.Handler

	handlerSwitch *switchHandler
	operationMu   sync.Mutex
	workerMu      sync.Mutex
	workerCancel  context.CancelFunc
	workerDone    sync.WaitGroup
	workerRoot    context.Context

	restoreAfterDatabaseSwapHook func() error
}

func Open(config Config) (*Server, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	database, err := productdb.Open(context.Background(), config.DatabasePath)
	if err != nil {
		return nil, err
	}
	server := New(config, database, productauth.New(database))
	return server, nil
}

func New(config Config, database *productdb.Database, auth *productauth.Service) *Server {
	server := &Server{Config: config, Database: database, Auth: auth, handlerSwitch: &switchHandler{}}
	server.Handler = server.handlerSwitch
	server.rebuildHandler()
	return server
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
	mux.Handle("/maintenance/status", s.maintenanceStatusHandler(database, auth))
	mux.Handle("/maintenance/path-mappings", sameOrigin(s.maintenancePathMappingsHandler()))
	mux.Handle("/maintenance/resume", sameOrigin(s.maintenanceResumeHandler()))
	resourceHandler := mediaresource.Handler{Database: database, Cache: mediaprocessing.CacheWriter{Root: s.Config.CachePath},
		Access: func(request *http.Request) (productdb.ResourceAccess, error) {
			return productdb.ResourceAccess{Authenticated: auth.AuthorizeRequest(request), Mode: productdb.ResourceBrowse,
				Scope: productdb.ResourceScopeAll}, nil
		}}
	mux.Handle("/resource/", resourceHandler)
	if s.Config.WebRoot != "" {
		mux.Handle("/", spaHandler(s.Config.WebRoot))
	} else if embeddedWeb, ok := productweb.FileSystem(); ok {
		mux.Handle("/", spaHandlerFS(embeddedWeb))
	}
	s.handlerSwitch.Set(securityHeaders(s.maintenanceGate(database, mux)))
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
	if s.Config.FFmpegPath != "" {
		generators = append(generators, mediaprocessing.FFmpegPosterGenerator{Encoder: ffmpeg.NewEncoder(s.Config.FFmpegPath)})
	}
	worker := processingworker.Worker{Database: s.Database, Materializer: mediaaccess.Materializer{TemporaryRoot: temporaryRoot}, Cache: mediaprocessing.CacheWriter{Root: s.Config.CachePath}, Generators: generators}
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
		_, err := worker.RunOne(ctx, owner, 30*time.Second, time.Now())
		if err == nil {
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
			log.Print("CGM_MEDIA_JOB_FAILED")
		}
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
