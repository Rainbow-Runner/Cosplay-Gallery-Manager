package productserver

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stashapp/stash/internal/archivefile"
	"github.com/stashapp/stash/internal/library"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/sourcescan"
)

const libraryMonitorQuiet = 10 * time.Second

type monitoredChange struct {
	libraryID int64
	path      string
	at        time.Time
}

func monitorPathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Linux watches are per directory. Never follow symlinks; a new directory is
// registered recursively, but an ordinary file event does not walk the library.
func addMonitorTree(w *fsnotify.Watcher, root string, boundaries ...string) error {
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("monitor root must be a real directory")
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		for _, boundary := range boundaries {
			if path == boundary && path != root && entry.IsDir() {
				return filepath.SkipDir
			}
		}
		if entry.IsDir() {
			return w.Add(path)
		}
		return nil
	})
}

func (s *Server) runLibraryMonitor(ctx context.Context) {
	s.runLibraryMonitorWithQuiet(ctx, libraryMonitorQuiet)
}

func (s *Server) runLibraryMonitorWithQuiet(ctx context.Context, quiet time.Duration) {
	var watcher *fsnotify.Watcher
	var events <-chan fsnotify.Event
	var failures <-chan error
	defer func() {
		if watcher != nil {
			watcher.Close()
		}
	}()
	libraries := []library.Library{}
	allLibraries := []library.Library{}
	pending := map[string]monitoredChange{}
	signature := ""
	degraded := false
	retryAfter := time.Time{}
	refresh := func() {
		if time.Now().Before(retryAfter) {
			return
		}
		settings, err := s.Database.Settings().Find(ctx)
		if err != nil {
			return
		}
		maintenance, err := s.Database.Operations().Maintenance(ctx)
		if err != nil {
			return
		}
		var next []library.Library
		var all []library.Library
		nextSignature := "off"
		if settings.LibraryMonitorEnabled && !settings.AutomaticSchedulesSuspended && maintenance.Mode == productdb.MaintenanceNormal {
			all, err = s.Database.Libraries().List(ctx)
			if err != nil {
				return
			}
			for _, library := range all {
				nextSignature += fmt.Sprintf("|%d:%t:%s", library.ID, library.Enabled, library.RootPath)
				if library.Enabled {
					next = append(next, library)
				}
			}
		}
		if signature == nextSignature {
			return
		}
		signature = nextSignature
		degraded = false
		if watcher != nil {
			watcher.Close()
			watcher = nil
		}
		events = nil
		failures = nil
		libraries = next
		allLibraries = all
		clear(pending)
		if len(next) == 0 {
			return
		}
		for _, library := range next {
			pending[library.RootPath] = monitoredChange{library.ID, library.RootPath, time.Now()}
		}
		watcher, err = fsnotify.NewWatcher()
		if err != nil {
			degraded = true
			slog.Warn("CGM_LIBRARY_MONITOR_UNAVAILABLE")
			signature = ""
			retryAfter = time.Now().Add(5 * time.Minute)
			return
		}
		events = watcher.Events
		failures = watcher.Errors
		for _, library := range next {
			var boundaries []string
			for _, child := range all {
				if child.ID != library.ID && monitorPathWithin(library.RootPath, child.RootPath) {
					boundaries = append(boundaries, child.RootPath)
				}
			}
			if err := addMonitorTree(watcher, library.RootPath, boundaries...); err != nil {
				degraded = true
				slog.Warn("CGM_LIBRARY_MONITOR_PARTIAL", "library_id", library.ID)
			}
			// Re-enablement/restart closes event gaps using one discovery pass.
			pending[library.RootPath] = monitoredChange{library.ID, library.RootPath, time.Now()}
		}
		slog.Info("CGM_LIBRARY_MONITOR_STARTED", "library_count", len(next), "watch_count", len(watcher.WatchList()), "degraded", degraded)
	}
	first := true
	recovery := time.NewTicker(time.Hour)
	defer recovery.Stop()
	for ctx.Err() == nil {
		wake := s.Database.BackgroundEvents()
		if first {
			refresh()
			first = false
		}
		var timer *time.Timer
		var timerC <-chan time.Time
		if len(pending) > 0 {
			var deadline time.Time
			for _, change := range pending {
				deadline = earliestBackgroundTime(deadline, change.at.Add(quiet))
			}
			delay := time.Until(deadline)
			if delay < 0 {
				delay = 0
			}
			timer = time.NewTimer(delay)
			timerC = timer.C
		}
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case <-wake:
			refresh()
		case <-recovery.C:
			refresh()
			if degraded {
				for _, library := range libraries {
					pending[library.RootPath] = monitoredChange{library.ID, library.RootPath, time.Now()}
				}
			}
		case event, ok := <-events:
			if !ok {
				events = nil
				break
			}
			if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 {
				break
			}
			if event.Op&(fsnotify.Remove|fsnotify.Rename) == 0 {
				if info, err := os.Lstat(event.Name); err == nil && !info.IsDir() && !sourcescan.IsSupportedMediaPath(event.Name) && !archivefile.IsSupportedPath(event.Name) && filepath.Base(event.Name) != ".cosplay-root" && !strings.HasSuffix(event.Name, ".cosplay.json") {
					break
				}
			}
			for _, library := range libraries {
				if !monitorPathWithin(library.RootPath, event.Name) {
					continue
				}
				// Nested library ownership: only the deepest enabled root handles it.
				owned := true
				for _, child := range allLibraries {
					if child.ID != library.ID && monitorPathWithin(library.RootPath, child.RootPath) && monitorPathWithin(child.RootPath, event.Name) {
						owned = false
						break
					}
				}
				if !owned {
					continue
				}
				path := filepath.Clean(event.Name)
				if event.Op&fsnotify.Create != 0 {
					if info, err := os.Lstat(path); err == nil && info.IsDir() {
						var boundaries []string
						for _, child := range allLibraries {
							if child.ID != library.ID && monitorPathWithin(path, child.RootPath) {
								boundaries = append(boundaries, child.RootPath)
							}
						}
						if err := addMonitorTree(watcher, path, boundaries...); err != nil {
							degraded = true
							slog.Warn("CGM_LIBRARY_MONITOR_PARTIAL", "library_id", library.ID)
						}
					}
				}
				// Coalesce media siblings only after new-directory registration.
				// Ordinary photo events must not recursively re-register a tree.
				if sourcescan.IsSupportedMediaPath(path) {
					if info, err := os.Lstat(path); err != nil || !info.IsDir() {
						path = filepath.Dir(path)
					}
				}
				if len(pending) >= 4096 {
					for key, change := range pending {
						if change.libraryID == library.ID {
							delete(pending, key)
						}
					}
					path = library.RootPath
				}
				pending[path] = monitoredChange{library.ID, path, time.Now()}
			}
		case _, ok := <-failures:
			if !ok {
				failures = nil
				break
			}
			slog.Warn("CGM_LIBRARY_MONITOR_EVENT_LOSS")
			degraded = true
			for _, library := range libraries {
				pending[library.RootPath] = monitoredChange{library.ID, library.RootPath, time.Now()}
			}
		case <-timerC:
			for key, change := range pending {
				if time.Since(change.at) < quiet {
					continue
				}
				if err := s.Database.QueueLibraryPathChange(ctx, change.libraryID, change.path, change.at); err == nil {
					delete(pending, key)
				} else {
					change.at = time.Now()
					pending[key] = change
				}
			}
		}
		if timer != nil {
			timer.Stop()
		}
	}
}

func (s *Server) runLibraryPathChanges(ctx context.Context, now time.Time) bool {
	settings, err := s.Database.Settings().Find(ctx)
	if err != nil || !settings.LibraryMonitorEnabled || settings.AutomaticSchedulesSuspended {
		return false
	}
	maintenance, err := s.Database.Operations().Maintenance(ctx)
	if err != nil || maintenance.Mode != productdb.MaintenanceNormal {
		return false
	}
	changes, err := s.Database.PendingLibraryPathChanges(ctx, now.Add(-libraryMonitorQuiet), 100)
	if err != nil {
		return true
	}
	discover := map[int64]bool{}
	valid := []productdb.LibraryPathChange{}
	libraries, err := s.Database.Libraries().List(ctx)
	if err != nil {
		return true
	}
	for _, change := range changes {
		sources, err := s.Database.ChangedPathSourceIDs(ctx, change, now)
		if err != nil {
			continue
		}
		if len(sources) == 0 {
			discover[change.LibraryID] = true
		}
		for _, library := range libraries {
			if library.ID == change.LibraryID && change.Path == library.RootPath {
				discover[change.LibraryID] = true
			}
		}
		valid = append(valid, change)
	}
	for libraryID := range discover {
		if _, err := s.Database.CandidateDiscovery().DiscoverFilesystem(ctx, libraryID, now); err != nil {
			slog.Warn("CGM_LIBRARY_MONITOR_DISCOVERY_FAILED", "library_id", libraryID)
			for _, change := range valid {
				if change.LibraryID == libraryID {
					_ = s.Database.QueueLibraryPathChange(ctx, libraryID, change.Path, now.Add(time.Hour))
				}
			}
			continue
		}
	}
	// Source reconciliation uses the normal bounded worker below. Discovery
	// only proposes new Galleries; it does not silently activate or replace Cast.
	for _, change := range valid {
		_ = s.Database.CompleteLibraryPathChange(ctx, change)
	}
	return len(changes) >= 100
}
