package productserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/stashapp/stash/internal/mediaprocessing"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/processingworker"
)

const cacheCleanupReviewPath = "/manage/cache/review"

func (s *Server) cacheCleanupHandler(database *productdb.Database) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !s.Auth.AuthorizeRequest(r) {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		service := processingworker.CacheLifecycleService{Database: database, Cache: mediaprocessing.CacheWriter{Root: s.Config.CachePath}}
		if r.Method == http.MethodGet {
			review, err := service.Review(r.Context(), time.Now())
			if err != nil {
				http.Error(w, "CACHE_REVIEW_FAILED", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(review)
			return
		}
		audit := func(outcome, code string, summary map[string]any) {
			_ = database.Operations().Audit(r.Context(), "CACHE_CLEANUP", "SYSTEM", "", outcome, code, summary, time.Now())
		}
		r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
		var input struct {
			IDs          []string `json:"ids"`
			Password     string   `json:"password"`
			Confirmation string   `json:"confirmation"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var trailing any
		if err := decoder.Decode(&input); err != nil || !errors.Is(decoder.Decode(&trailing), io.EOF) || len(input.IDs) == 0 || len(input.IDs) > 100 {
			audit("FAILURE", "CACHE_CLEANUP_INPUT_FAILED", nil)
			http.Error(w, "CACHE_CLEANUP_INPUT_FAILED", http.StatusBadRequest)
			return
		}
		if input.Confirmation != "CLEAN" {
			audit("FAILURE", "CACHE_CLEANUP_CONFIRMATION_FAILED", nil)
			http.Error(w, "CACHE_CLEANUP_CONFIRMATION_FAILED", http.StatusBadRequest)
			return
		}
		if err := s.Auth.VerifyPassword(r.Context(), input.Password); err != nil {
			audit("FAILURE", "CACHE_CLEANUP_PASSWORD_FAILED", nil)
			http.Error(w, "CACHE_CLEANUP_PASSWORD_FAILED", http.StatusForbidden)
			return
		}
		result, err := service.CleanupSelected(r.Context(), input.IDs, time.Now())
		summary := map[string]any{"selected": len(input.IDs), "removed": result.Removed, "freed_bytes": result.FreedBytes, "failed": result.Failed, "skipped": result.Skipped}
		if err != nil {
			status, code := http.StatusInternalServerError, "CACHE_CLEANUP_FAILED"
			if errors.Is(err, productdb.ErrCacheReviewStale) {
				status, code = http.StatusConflict, "CACHE_REVIEW_STALE"
			}
			audit("FAILURE", code, summary)
			http.Error(w, code, status)
			return
		}
		outcome, code := "SUCCESS", ""
		if result.Failed > 0 {
			outcome, code = "FAILURE", "CACHE_FILE_DELETE_FAILED"
		}
		audit(outcome, code, summary)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
}
