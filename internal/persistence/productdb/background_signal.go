package productdb

import (
	"strings"
	"sync"

	"github.com/mattn/go-sqlite3"
)

// Closing a generation broadcasts to every worker. Subscribers take a snapshot
// before checking durable work, so a change during that check cannot be lost.
// Notifications are hints, not transaction commits or authorization to process.
type workSignal struct {
	mu   sync.Mutex
	wake chan struct{}
}

func newWorkSignal() *workSignal { return &workSignal{wake: make(chan struct{})} }
func (s *workSignal) snapshot() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.wake
}
func (s *workSignal) changed() int {
	s.mu.Lock()
	close(s.wake)
	s.wake = make(chan struct{})
	s.mu.Unlock()
	return 1
}

func (db *Database) ProcessingEvents() <-chan struct{}  { return db.processingSignal.snapshot() }
func (db *Database) AutomationEvents() <-chan struct{}  { return db.automationSignal.snapshot() }
func (db *Database) BackgroundEvents() <-chan struct{}  { return db.backgroundSignal.snapshot() }
func (db *Database) BackgroundScheduleRevision() uint64 { return db.scheduleRevision.Load() }

// TEMP triggers also cover store-internal transactions and cascading writes.
// Lease heartbeats, access timestamps and progress-only writes are not events.
func installBackgroundTriggers(c *sqlite3.SQLiteConn) error {
	for _, rule := range []struct{ table, columns, callback string }{
		{"processing_jobs", "status,not_before_utc", "cgm_processing_changed"},
		{"media_derivatives", "state,is_current,content_revision,profile_hash", "cgm_processing_changed"},
		{"cover_similarity_intents", "status,next_check_at_utc", "cgm_processing_changed"},
		{"library_automation_runs", "status,cancellation_requested", "cgm_automation_changed"},
		{"library_automation_runs", "status,cancellation_requested", "cgm_background_changed"},
		{"gallery_items", "content_revision,excluded,availability_state,media_kind,source_modified_status", "cgm_background_changed"},
		{"gallery_sources", "availability_state,over_limit,reconcile_state", "cgm_background_changed"},
		{"library_path_changes", "changed_at_utc", "cgm_background_changed"},
		{"gallery_source_issues", "resolved_at_utc,severity", "cgm_background_changed"},
		{"media_libraries", "enabled,capture_timezone", "cgm_background_changed"},
		{"runtime_settings", "library_monitor_enabled,automatic_scan_enabled,automatic_scan_on_startup,automatic_scan_interval_minutes,automatic_schedules_suspended,daily_backup_enabled,daily_backup_retention,archive_max_entries,archive_max_entry_bytes,archive_max_total_bytes,archive_max_compression_ratio,archive_max_image_pixels", "cgm_schedule_changed"},
		{"runtime_settings", "gallery_cover_1600_enabled", "cgm_processing_changed"},
		{"maintenance_state", "state", "cgm_schedule_changed"},
		{"product_setup", "complete,backup_root", "cgm_schedule_changed"},
	} {
		prefix := "cgm_wake_" + rule.callback + "_" + rule.table + "_"
		conditions := []string{}
		for _, column := range strings.Split(rule.columns, ",") {
			conditions = append(conditions, "OLD."+column+" IS NOT NEW."+column)
		}
		query := "CREATE TEMP TRIGGER IF NOT EXISTS " + prefix + "insert AFTER INSERT ON main." + rule.table + " BEGIN SELECT " + rule.callback + "(); END;" +
			"CREATE TEMP TRIGGER IF NOT EXISTS " + prefix + "delete AFTER DELETE ON main." + rule.table + " BEGIN SELECT " + rule.callback + "(); END;" +
			"CREATE TEMP TRIGGER IF NOT EXISTS " + prefix + "update AFTER UPDATE OF " + rule.columns + " ON main." + rule.table + " WHEN " + strings.Join(conditions, " OR ") + " BEGIN SELECT " + rule.callback + "(); END;"
		if _, err := c.Exec(query, nil); err != nil {
			return err
		}
	}
	_, err := c.Exec(`CREATE TEMP TRIGGER IF NOT EXISTS cgm_background_job_finished AFTER UPDATE OF status ON main.processing_jobs
		WHEN OLD.status<>NEW.status AND NEW.status IN ('COMPLETED','FAILED','CANCELLED') BEGIN SELECT cgm_background_changed(); END;`, nil)
	return err
}
