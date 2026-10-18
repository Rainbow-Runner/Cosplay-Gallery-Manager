package productdb

import (
	"context"
	"database/sql"
	"time"
)

// Technical/poster terminal failures must not leave newly imported archive
// videos permanently PENDING. Existing current posters remain displayable.
func updateArchiveVideoJobState(ctx context.Context, tx *sql.Tx, jobID int64, state string, now time.Time) error {
	_, err := tx.ExecContext(ctx, `UPDATE gallery_items SET processing_state=?,updated_at_utc=?
		WHERE item_uuid=(SELECT item_uuid FROM processing_jobs WHERE id=?)
		AND media_kind='VIDEO' AND excluded=0 AND availability_state='AVAILABLE'
		AND source_id IN (SELECT id FROM gallery_sources WHERE source_type='ARCHIVE')
		AND EXISTS(SELECT 1 FROM processing_jobs job WHERE job.id=? AND job.item_uuid=gallery_items.item_uuid
			AND job.content_revision=gallery_items.content_revision
			AND ((job.job_kind='ITEM_TECHNICAL_METADATA' AND job.variant='') OR (job.job_kind='ITEM_DERIVATIVE' AND job.variant='STATIC_POSTER')))
		AND NOT EXISTS(SELECT 1 FROM media_derivatives derivative WHERE derivative.item_uuid=gallery_items.item_uuid
			AND derivative.variant='STATIC_POSTER' AND derivative.content_revision=gallery_items.content_revision
			AND derivative.is_current=1 AND derivative.state IN ('READY','STALE'))`, state, formatTime(now), jobID, jobID)
	return err
}
