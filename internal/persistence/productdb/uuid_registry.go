package productdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/stashapp/stash/internal/portableid"
)

var (
	ErrPortableUUIDNotFound   = errors.New("portable UUID is not registered")
	ErrPortableUUIDOccupied   = errors.New("portable UUID is permanently occupied")
	ErrPortableUUIDNotActive  = errors.New("portable UUID is not active")
	ErrPortableUUIDTombstoned = errors.New("portable UUID is tombstoned")
)

// PortableUUIDState is derived from immutable registry, alias and tombstone
// records. Registry rows are never removed or repurposed.
type PortableUUIDState string

const (
	PortableUUIDActive    PortableUUIDState = "ACTIVE"
	PortableUUIDAlias     PortableUUIDState = "ALIAS"
	PortableUUIDTombstone PortableUUIDState = "TOMBSTONE"
)

// PortableUUIDRecord describes one permanently occupied UUID.
type PortableUUIDRecord struct {
	UUID         string
	Kind         portableid.Kind
	State        PortableUUIDState
	TargetUUID   string
	CreatedAtUTC time.Time
	RetiredAtUTC *time.Time
	Reason       string
}

// PortableUUIDKindConflictError reports an attempt to reuse an occupied UUID
// for a different entity type.
type PortableUUIDKindConflictError struct {
	UUID      string
	Found     portableid.Kind
	Requested portableid.Kind
}

func (e *PortableUUIDKindConflictError) Error() string {
	return fmt.Sprintf(
		"portable UUID %q belongs to %s, not %s",
		e.UUID,
		e.Found,
		e.Requested,
	)
}

// UUIDRegistry performs durable allocation and retirement operations against
// the global portable UUID namespace.
type UUIDRegistry struct {
	db *sql.DB
}

// UUIDRegistry returns the registry bound to this product database.
func (db *Database) UUIDRegistry() *UUIDRegistry {
	return &UUIDRegistry{db: db.DB}
}

// Register imports an existing canonical UUIDv4/v7 into the global namespace.
func (r *UUIDRegistry) Register(
	ctx context.Context,
	value string,
	kind portableid.Kind,
	now time.Time,
) (PortableUUIDRecord, error) {
	if _, err := portableid.Parse(value); err != nil {
		return PortableUUIDRecord{}, err
	}
	if err := portableid.ValidateKind(kind); err != nil {
		return PortableUUIDRecord{}, err
	}

	createdAt := normalisedTime(now)
	return registerPortableUUID(ctx, r.db, value, kind, createdAt)
}

type portableUUIDExecer interface {
	portableUUIDQueryer
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

func registerPortableUUID(
	ctx context.Context,
	execer portableUUIDExecer,
	value string,
	kind portableid.Kind,
	createdAt time.Time,
) (PortableUUIDRecord, error) {
	_, err := execer.ExecContext(ctx, `
		INSERT INTO portable_uuid_registry (uuid, entity_kind, created_at_utc)
		VALUES (?, ?, ?)
	`, value, kind, createdAt.Format(time.RFC3339Nano))
	if err != nil {
		existing, lookupErr := lookupPortableUUID(ctx, execer, value)
		if lookupErr == nil {
			if existing.Kind != kind {
				return PortableUUIDRecord{}, &PortableUUIDKindConflictError{
					UUID:      value,
					Found:     existing.Kind,
					Requested: kind,
				}
			}
			return PortableUUIDRecord{}, fmt.Errorf("%w: %s", ErrPortableUUIDOccupied, value)
		}
		return PortableUUIDRecord{}, fmt.Errorf("registering portable UUID: %w", err)
	}

	return PortableUUIDRecord{
		UUID:         value,
		Kind:         kind,
		State:        PortableUUIDActive,
		CreatedAtUTC: createdAt,
	}, nil
}

// New allocates and registers a new canonical UUIDv4.
func (r *UUIDRegistry) New(
	ctx context.Context,
	kind portableid.Kind,
	now time.Time,
) (PortableUUIDRecord, error) {
	if err := portableid.ValidateKind(kind); err != nil {
		return PortableUUIDRecord{}, err
	}

	// A collision is cryptographically negligible, but retrying keeps the API
	// correct even under a deterministic UUID source used by future tests.
	for attempts := 0; attempts < 4; attempts++ {
		record, err := r.Register(ctx, portableid.New(), kind, now)
		if err == nil {
			return record, nil
		}
		var kindConflict *PortableUUIDKindConflictError
		if !errors.Is(err, ErrPortableUUIDOccupied) && !errors.As(err, &kindConflict) {
			return PortableUUIDRecord{}, err
		}
	}

	return PortableUUIDRecord{}, errors.New("allocating portable UUID after repeated collisions")
}

// Lookup returns the lifecycle record without resolving aliases.
func (r *UUIDRegistry) Lookup(ctx context.Context, value string) (PortableUUIDRecord, error) {
	return lookupPortableUUID(ctx, r.db, value)
}

// Resolve follows permanent merge aliases and returns the current active UUID.
// A chain ending in a Tombstone remains unavailable and cannot be recreated.
func (r *UUIDRegistry) Resolve(
	ctx context.Context,
	value string,
) (PortableUUIDRecord, error) {
	seen := make(map[string]struct{})
	current := value

	for {
		if _, duplicate := seen[current]; duplicate {
			return PortableUUIDRecord{}, fmt.Errorf("portable UUID alias cycle detected at %s", current)
		}
		seen[current] = struct{}{}

		record, err := r.Lookup(ctx, current)
		if err != nil {
			return PortableUUIDRecord{}, err
		}
		switch record.State {
		case PortableUUIDActive:
			return record, nil
		case PortableUUIDAlias:
			current = record.TargetUUID
		case PortableUUIDTombstone:
			return PortableUUIDRecord{}, fmt.Errorf("%w: %s", ErrPortableUUIDTombstoned, current)
		default:
			return PortableUUIDRecord{}, fmt.Errorf("unknown portable UUID state %q", record.State)
		}
	}
}

// Alias records a permanent source-to-target merge. This standalone operation
// establishes registry semantics; aggregate entity merges will use the same
// checks through a transaction-bound helper when those tables are introduced.
func (r *UUIDRegistry) Alias(
	ctx context.Context,
	sourceUUID string,
	targetUUID string,
	kind portableid.Kind,
	now time.Time,
) error {
	if _, err := portableid.Parse(sourceUUID); err != nil {
		return err
	}
	if _, err := portableid.Parse(targetUUID); err != nil {
		return err
	}
	if err := portableid.ValidateKind(kind); err != nil {
		return err
	}
	if sourceUUID == targetUUID {
		return errors.New("portable UUID cannot be aliased to itself")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	for _, endpoint := range []string{sourceUUID, targetUUID} {
		record, err := lookupPortableUUID(ctx, tx, endpoint)
		if err != nil {
			return err
		}
		if record.Kind != kind {
			return &PortableUUIDKindConflictError{
				UUID:      endpoint,
				Found:     record.Kind,
				Requested: kind,
			}
		}
		if record.State != PortableUUIDActive {
			return fmt.Errorf("%w: %s is %s", ErrPortableUUIDNotActive, endpoint, record.State)
		}
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO portable_uuid_aliases (
			alias_uuid, target_uuid, entity_kind, merged_at_utc
		) VALUES (?, ?, ?, ?)
	`, sourceUUID, targetUUID, kind, normalisedTime(now).Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("recording portable UUID alias: %w", err)
	}

	return tx.Commit()
}

// Tombstone permanently retires an active UUID after its owning entity is
// deleted. It never removes the allocation record.
func (r *UUIDRegistry) Tombstone(
	ctx context.Context,
	value string,
	kind portableid.Kind,
	reason string,
	now time.Time,
) error {
	if _, err := portableid.Parse(value); err != nil {
		return err
	}
	if err := portableid.ValidateKind(kind); err != nil {
		return err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	record, err := lookupPortableUUID(ctx, tx, value)
	if err != nil {
		return err
	}
	if record.Kind != kind {
		return &PortableUUIDKindConflictError{
			UUID:      value,
			Found:     record.Kind,
			Requested: kind,
		}
	}
	if record.State != PortableUUIDActive {
		return fmt.Errorf("%w: %s is %s", ErrPortableUUIDNotActive, value, record.State)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO portable_uuid_tombstones (
			uuid, entity_kind, deleted_at_utc, reason
		) VALUES (?, ?, ?, ?)
	`, value, kind, normalisedTime(now).Format(time.RFC3339Nano), reason)
	if err != nil {
		return fmt.Errorf("recording portable UUID tombstone: %w", err)
	}

	return tx.Commit()
}

type portableUUIDQueryer interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

func lookupPortableUUID(
	ctx context.Context,
	queryer portableUUIDQueryer,
	value string,
) (PortableUUIDRecord, error) {
	var (
		record      PortableUUIDRecord
		createdAt   string
		aliasTarget sql.NullString
		mergedAt    sql.NullString
		deletedAt   sql.NullString
		reason      sql.NullString
	)

	err := queryer.QueryRowContext(ctx, `
		SELECT
			r.uuid,
			r.entity_kind,
			r.created_at_utc,
			a.target_uuid,
			a.merged_at_utc,
			t.deleted_at_utc,
			t.reason
		FROM portable_uuid_registry r
		LEFT JOIN portable_uuid_aliases a ON a.alias_uuid = r.uuid
		LEFT JOIN portable_uuid_tombstones t ON t.uuid = r.uuid
		WHERE r.uuid = ?
	`, value).Scan(
		&record.UUID,
		&record.Kind,
		&createdAt,
		&aliasTarget,
		&mergedAt,
		&deletedAt,
		&reason,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return PortableUUIDRecord{}, fmt.Errorf("%w: %s", ErrPortableUUIDNotFound, value)
	}
	if err != nil {
		return PortableUUIDRecord{}, err
	}

	record.CreatedAtUTC, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return PortableUUIDRecord{}, fmt.Errorf("parsing portable UUID creation time: %w", err)
	}

	switch {
	case aliasTarget.Valid:
		record.State = PortableUUIDAlias
		record.TargetUUID = aliasTarget.String
		retiredAt, err := time.Parse(time.RFC3339Nano, mergedAt.String)
		if err != nil {
			return PortableUUIDRecord{}, fmt.Errorf("parsing portable UUID merge time: %w", err)
		}
		record.RetiredAtUTC = &retiredAt
	case deletedAt.Valid:
		record.State = PortableUUIDTombstone
		retiredAt, err := time.Parse(time.RFC3339Nano, deletedAt.String)
		if err != nil {
			return PortableUUIDRecord{}, fmt.Errorf("parsing portable UUID deletion time: %w", err)
		}
		record.RetiredAtUTC = &retiredAt
		record.Reason = reason.String
	default:
		record.State = PortableUUIDActive
	}

	return record, nil
}

func normalisedTime(value time.Time) time.Time {
	return value.UTC().Round(0)
}
