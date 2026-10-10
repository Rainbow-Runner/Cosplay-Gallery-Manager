package productdb

import (
	"context"
	"database/sql"
	"fmt"
)

var requiredAutomationSchemaV6Tables = []string{"library_automation_policies", "library_automation_run_issues", "library_automation_runs"}

func createAutomationSchemaV6(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE library_automation_policies (
			library_id INTEGER NOT NULL PRIMARY KEY REFERENCES media_libraries(id) ON DELETE CASCADE,
			mode TEXT NOT NULL DEFAULT 'MANUAL' CHECK(mode IN ('MANUAL','ASSISTED','TRUSTED')),
			default_content_rating TEXT CHECK(default_content_rating IN ('NON_ADULT','ADULT')),
			exclude_new_root_media INTEGER NOT NULL DEFAULT 1 CHECK(exclude_new_root_media IN (0,1)),
			auto_accept_unique_entities INTEGER NOT NULL DEFAULT 0 CHECK(auto_accept_unique_entities IN (0,1)),
			auto_accept_media_classification INTEGER NOT NULL DEFAULT 0 CHECK(auto_accept_media_classification IN (0,1)),
			auto_activate INTEGER NOT NULL DEFAULT 0 CHECK(auto_activate IN (0,1)),
			revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
			created_at_utc TEXT NOT NULL,
			updated_at_utc TEXT NOT NULL
		);
		CREATE TABLE library_automation_runs (
			id INTEGER NOT NULL PRIMARY KEY,
			library_id INTEGER NOT NULL REFERENCES media_libraries(id) ON DELETE CASCADE,
			policy_revision INTEGER NOT NULL CHECK(policy_revision > 0),
			mode TEXT NOT NULL CHECK(mode IN ('ASSISTED','TRUSTED')),
			default_content_rating TEXT CHECK(default_content_rating IN ('NON_ADULT','ADULT')),
			exclude_new_root_media INTEGER NOT NULL CHECK(exclude_new_root_media IN (0,1)),
			auto_accept_unique_entities INTEGER NOT NULL CHECK(auto_accept_unique_entities IN (0,1)),
			auto_accept_media_classification INTEGER NOT NULL CHECK(auto_accept_media_classification IN (0,1)),
			auto_activate INTEGER NOT NULL CHECK(auto_activate IN (0,1)),
			status TEXT NOT NULL CHECK(status IN ('QUEUED','RUNNING','COMPLETED','FAILED','CANCELLED')),
			discovery_completed INTEGER NOT NULL DEFAULT 0 CHECK(discovery_completed IN (0,1)),
			cursor_gallery_id INTEGER NOT NULL DEFAULT 0 CHECK(cursor_gallery_id >= 0),
			cancellation_requested INTEGER NOT NULL DEFAULT 0 CHECK(cancellation_requested IN (0,1)),
			candidates_seen INTEGER NOT NULL DEFAULT 0 CHECK(candidates_seen >= 0),
			drafts_created INTEGER NOT NULL DEFAULT 0 CHECK(drafts_created >= 0),
			scanned INTEGER NOT NULL DEFAULT 0 CHECK(scanned >= 0),
			activated INTEGER NOT NULL DEFAULT 0 CHECK(activated >= 0),
			needs_review INTEGER NOT NULL DEFAULT 0 CHECK(needs_review >= 0),
			error_code TEXT NOT NULL DEFAULT '' CHECK(length(error_code) <= 100),
			started_at_utc TEXT NOT NULL,
			completed_at_utc TEXT,
			lease_owner TEXT,
			lease_expires_at_utc TEXT,
			last_heartbeat_at_utc TEXT
		);
		CREATE INDEX library_automation_runs_recent ON library_automation_runs(library_id,id DESC);
		CREATE INDEX library_automation_runs_claim ON library_automation_runs(status,id);
		CREATE UNIQUE INDEX library_automation_runs_one_active ON library_automation_runs(library_id) WHERE status IN ('QUEUED','RUNNING');
		CREATE TABLE library_automation_run_issues (
			id INTEGER NOT NULL PRIMARY KEY,
			run_id INTEGER NOT NULL REFERENCES library_automation_runs(id) ON DELETE CASCADE,
			gallery_id INTEGER REFERENCES galleries(id) ON DELETE SET NULL,
			source_id INTEGER REFERENCES gallery_sources(id) ON DELETE SET NULL,
			stage TEXT NOT NULL CHECK(stage IN ('SCAN','POLICY','ACTIVATION')),
			error_code TEXT NOT NULL CHECK(error_code <> '' AND length(error_code) <= 100),
			created_at_utc TEXT NOT NULL,
			UNIQUE(run_id,gallery_id,stage)
		);
		CREATE INDEX library_automation_run_issues_run ON library_automation_run_issues(run_id,id);
	`); err != nil {
		return fmt.Errorf("creating automation schema version 6: %w", err)
	}
	return nil
}

func validateAutomationSchemaV6(ctx context.Context, db *sql.DB) error {
	for _, table := range requiredAutomationSchemaV6Tables {
		exists, err := schemaObjectExists(ctx, db, "table", table)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w: required table %q is missing", ErrInvalidProductSchema, table)
		}
	}
	return nil
}

func validateSchemaV6(ctx context.Context, db *sql.DB) error {
	if err := validateSchemaV5(ctx, db); err != nil {
		return err
	}
	return validateAutomationSchemaV6(ctx, db)
}
