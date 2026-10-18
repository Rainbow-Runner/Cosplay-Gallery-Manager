package productdb

import (
	"context"
	"database/sql/driver"
	"sync/atomic"

	"github.com/mattn/go-sqlite3"
)

// Connection-local triggers cover all writers (including transactions/cascade
// deletion), without changing the on-disk schema. Access-time touches and the
// maintenance cursor are deliberately not events. Only a buffered wakeup is
// sent from SQLite callbacks: database work happens after the transaction.
type cacheMaintenanceSignal struct {
	events   chan struct{}
	revision atomic.Uint64
	ready    atomic.Bool
}

func (s *cacheMaintenanceSignal) changed() int {
	s.revision.Add(1)
	select {
	case s.events <- struct{}{}:
	default:
	}
	return 1
}

type cacheConnector struct {
	dsn    string
	sqlite *sqlite3.SQLiteDriver
}

func (c *cacheConnector) Connect(context.Context) (driver.Conn, error) { return c.sqlite.Open(c.dsn) }
func (c *cacheConnector) Driver() driver.Driver                        { return c.sqlite }

func installCacheMaintenanceTriggers(c *sqlite3.SQLiteConn) error {
	_, err := c.Exec(`
	CREATE TEMP TRIGGER IF NOT EXISTS cgm_cache_insert AFTER INSERT ON main.media_derivatives BEGIN SELECT cgm_cache_changed(); END;
	CREATE TEMP TRIGGER IF NOT EXISTS cgm_cache_delete AFTER DELETE ON main.media_derivatives BEGIN SELECT cgm_cache_changed(); END;
	CREATE TEMP TRIGGER IF NOT EXISTS cgm_cache_change AFTER UPDATE OF content_revision,profile_hash,cache_relative_path,cache_tier,state,is_current,byte_size ON main.media_derivatives
	WHEN OLD.content_revision<>NEW.content_revision OR OLD.profile_hash<>NEW.profile_hash OR OLD.cache_relative_path<>NEW.cache_relative_path OR OLD.cache_tier<>NEW.cache_tier OR OLD.state<>NEW.state OR OLD.is_current<>NEW.is_current OR OLD.byte_size<>NEW.byte_size
	BEGIN SELECT cgm_cache_changed(); END;
	CREATE TEMP TRIGGER IF NOT EXISTS cgm_cache_item_change AFTER UPDATE OF content_revision,excluded,availability_state,content_format,media_kind ON main.gallery_items
	WHEN OLD.content_revision<>NEW.content_revision OR OLD.excluded<>NEW.excluded OR OLD.availability_state<>NEW.availability_state OR OLD.content_format<>NEW.content_format OR OLD.media_kind<>NEW.media_kind
	BEGIN SELECT cgm_cache_changed(); END;
	CREATE TEMP TRIGGER IF NOT EXISTS cgm_cache_source_change AFTER UPDATE OF availability_state,over_limit ON main.gallery_sources
	WHEN OLD.availability_state<>NEW.availability_state OR OLD.over_limit<>NEW.over_limit BEGIN SELECT cgm_cache_changed(); END;
	CREATE TEMP TRIGGER IF NOT EXISTS cgm_cache_job_terminal AFTER UPDATE OF status ON main.processing_jobs
	WHEN NEW.job_kind='ITEM_DERIVATIVE' AND OLD.status<>NEW.status AND NEW.status IN ('COMPLETED','FAILED','CANCELLED') BEGIN SELECT cgm_cache_changed(); END;
	CREATE TEMP TRIGGER IF NOT EXISTS cgm_cache_job_delete AFTER DELETE ON main.processing_jobs
	WHEN OLD.job_kind='ITEM_DERIVATIVE' BEGIN SELECT cgm_cache_changed(); END;
	CREATE TEMP TRIGGER IF NOT EXISTS cgm_cache_outbox_insert AFTER INSERT ON main.cache_cleanup_outbox BEGIN SELECT cgm_cache_changed(); END;
	CREATE TEMP TRIGGER IF NOT EXISTS cgm_cache_outbox_earlier AFTER UPDATE OF not_before_utc ON main.cache_cleanup_outbox
	WHEN NEW.not_before_utc<OLD.not_before_utc BEGIN SELECT cgm_cache_changed(); END;
	CREATE TEMP TRIGGER IF NOT EXISTS cgm_cache_pressure_change AFTER UPDATE OF enhanced_cache_maximum_bytes,minimum_free_bytes,minimum_free_percent ON main.runtime_settings
	WHEN OLD.enhanced_cache_maximum_bytes<>NEW.enhanced_cache_maximum_bytes OR OLD.minimum_free_bytes<>NEW.minimum_free_bytes OR OLD.minimum_free_percent<>NEW.minimum_free_percent
	BEGIN SELECT cgm_cache_changed(); END;
	`, nil)
	return err
}

func (db *Database) CacheMaintenanceEvents() <-chan struct{} { return db.cacheSignal.events }
func (db *Database) CacheMaintenanceRevision() uint64        { return db.cacheSignal.revision.Load() }
