package productdb

import (
	"context"
	"errors"
	"time"

	"github.com/stashapp/stash/internal/portableid"
)

type ReplaceTagParentInput struct {
	UUID     string
	Position int64
}

type ExpectedTagRevision struct {
	UUID             string
	MetadataRevision int64
}

// ReplaceTagParents changes every direct parent in one transaction. Revisions
// are required for the child and for the union of added, retained and removed
// parents, so a concurrent DAG edit can never be silently overwritten.
func (s *CoreEntityStore) ReplaceTagParents(ctx context.Context, childUUID string, expectedChildRevision int64, parents []ReplaceTagParentInput, expectedParents []ExpectedTagRevision, now time.Time) error {
	if len(parents) > 50 {
		return errors.New("Tag cannot have more than 50 direct parents")
	}
	requested := make(map[string]ReplaceTagParentInput, len(parents))
	positions := make(map[int64]struct{}, len(parents))
	for _, parent := range parents {
		if parent.UUID == "" || parent.UUID == childUUID || parent.Position <= 0 {
			return errors.New("invalid Tag parent")
		}
		if _, duplicate := requested[parent.UUID]; duplicate {
			return errors.New("duplicate Tag parent")
		}
		if _, duplicate := positions[parent.Position]; duplicate {
			return errors.New("duplicate Tag parent position")
		}
		requested[parent.UUID] = parent
		positions[parent.Position] = struct{}{}
	}
	expected := make(map[string]int64, len(expectedParents))
	for _, parent := range expectedParents {
		if parent.UUID == "" || parent.MetadataRevision <= 0 || parent.UUID == childUUID {
			return errors.New("invalid expected Tag parent revision")
		}
		if _, duplicate := expected[parent.UUID]; duplicate {
			return errors.New("duplicate expected Tag parent revision")
		}
		expected[parent.UUID] = parent.MetadataRevision
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := requireActivePortableKind(ctx, tx, childUUID, portableid.KindTag); err != nil {
		return err
	}
	var childRevision int64
	if err := tx.QueryRowContext(ctx, `SELECT metadata_revision FROM tags WHERE uuid=?`, childUUID).Scan(&childRevision); err != nil {
		return err
	}
	if childRevision != expectedChildRevision {
		return ErrCoreMetadataRevisionConflict
	}
	rows, err := tx.QueryContext(ctx, `SELECT parent_uuid FROM tag_edges WHERE child_uuid=?`, childUUID)
	if err != nil {
		return err
	}
	impacted := make(map[string]struct{}, len(requested)+len(expected))
	for rows.Next() {
		var uuid string
		if err := rows.Scan(&uuid); err != nil {
			rows.Close()
			return err
		}
		impacted[uuid] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for uuid := range requested {
		impacted[uuid] = struct{}{}
		if err := requireActivePortableKind(ctx, tx, uuid, portableid.KindTag); err != nil {
			return err
		}
	}
	if len(expected) != len(impacted) {
		return errors.New("expected parent revisions must exactly cover all affected Tags")
	}
	for uuid := range impacted {
		revision, ok := expected[uuid]
		if !ok {
			return errors.New("missing expected Tag parent revision")
		}
		result, err := tx.ExecContext(ctx, `UPDATE tags SET metadata_revision=metadata_revision+1,updated_at_utc=? WHERE uuid=? AND metadata_revision=?`, formatTime(normalisedTime(now)), uuid, revision)
		if err != nil {
			return err
		}
		if count, err := result.RowsAffected(); err != nil || count != 1 {
			return ErrCoreMetadataRevisionConflict
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE tags SET metadata_revision=metadata_revision+1,updated_at_utc=? WHERE uuid=? AND metadata_revision=?`, formatTime(normalisedTime(now)), childUUID, expectedChildRevision)
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return ErrCoreMetadataRevisionConflict
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tag_edges WHERE child_uuid=?`, childUUID); err != nil {
		return err
	}
	for _, parent := range parents {
		if _, err := tx.ExecContext(ctx, `INSERT INTO tag_edges(parent_uuid,child_uuid,position) VALUES(?,?,?)`, parent.UUID, childUUID, parent.Position); err != nil {
			return err
		}
	}
	return tx.Commit()
}
