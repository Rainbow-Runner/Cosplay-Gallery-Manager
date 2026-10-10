package portablecatalog

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

const maxIdentityCount = 20_000_000

// inspectIdentityLedger decodes records one at a time. A short-lived disk
// index provides uniqueness, target and graph checks without retaining a
// million-item registry in Go memory.
func inspectIdentityLedger(ctx context.Context, entry *zip.File, yield func(IdentityRecord) error) (IdentityLedger, int, error) {
	reader, err := entry.Open()
	if err != nil {
		return IdentityLedger{}, 0, err
	}
	defer reader.Close()
	temporary, err := os.CreateTemp("", ".cgm-portable-identities-*.sqlite")
	if err != nil {
		return IdentityLedger{}, 0, err
	}
	name := temporary.Name()
	if err := temporary.Close(); err != nil {
		_ = os.Remove(name)
		return IdentityLedger{}, 0, err
	}
	defer os.Remove(name)
	index, err := sql.Open("sqlite3", name)
	if err != nil {
		return IdentityLedger{}, 0, err
	}
	defer index.Close()
	index.SetMaxOpenConns(1)
	if _, err := index.ExecContext(ctx, `PRAGMA journal_mode=OFF; PRAGMA synchronous=OFF; PRAGMA temp_store=FILE;
		CREATE TABLE identities(uuid TEXT NOT NULL PRIMARY KEY,kind TEXT NOT NULL,state TEXT NOT NULL,target_uuid TEXT NOT NULL)`); err != nil {
		return IdentityLedger{}, 0, err
	}
	tx, err := index.BeginTx(ctx, nil)
	if err != nil {
		return IdentityLedger{}, 0, err
	}
	defer tx.Rollback()
	insert, err := tx.PrepareContext(ctx, `INSERT INTO identities(uuid,kind,state,target_uuid) VALUES(?,?,?,?)`)
	if err != nil {
		return IdentityLedger{}, 0, err
	}
	defer insert.Close()
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return IdentityLedger{}, 0, errors.New("identity ledger must be a JSON object")
	}
	result := IdentityLedger{Identities: []IdentityRecord{}}
	seenSchema, seenIdentities := false, false
	count := 0
	previous := ""
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return IdentityLedger{}, 0, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return IdentityLedger{}, 0, errors.New("identity ledger contains an invalid field")
		}
		switch key {
		case "schema_version":
			if seenSchema {
				return IdentityLedger{}, 0, errors.New("identity ledger repeats schema_version")
			}
			seenSchema = true
			if err := decoder.Decode(&result.SchemaVersion); err != nil {
				return IdentityLedger{}, 0, err
			}
		case "identities":
			if seenIdentities {
				return IdentityLedger{}, 0, errors.New("identity ledger repeats identities")
			}
			seenIdentities = true
			opening, err := decoder.Token()
			if err != nil || opening != json.Delim('[') {
				return IdentityLedger{}, 0, errors.New("identity ledger identities must be an array")
			}
			for decoder.More() {
				if err := ctx.Err(); err != nil {
					return IdentityLedger{}, 0, err
				}
				var identity IdentityRecord
				if err := decoder.Decode(&identity); err != nil {
					return IdentityLedger{}, 0, fmt.Errorf("decoding identity: %w", err)
				}
				if err := validateStreamIdentity(identity); err != nil {
					return IdentityLedger{}, 0, err
				}
				if previous != "" && identity.UUID <= previous {
					return IdentityLedger{}, 0, errors.New("portable identities are not strictly ordered by UUID")
				}
				previous = identity.UUID
				if _, err := insert.ExecContext(ctx, identity.UUID, identity.Kind, identity.State, identity.TargetUUID); err != nil {
					return IdentityLedger{}, 0, fmt.Errorf("indexing portable identity: %w", err)
				}
				if yield != nil {
					if err := yield(identity); err != nil {
						return IdentityLedger{}, 0, err
					}
				}
				if portableObjectIdentityKind(identity.Kind) {
					result.Identities = append(result.Identities, identity)
				}
				count++
				if count > maxIdentityCount {
					return IdentityLedger{}, 0, errors.New("portable metadata package has too many identities")
				}
			}
			closing, err := decoder.Token()
			if err != nil || closing != json.Delim(']') {
				return IdentityLedger{}, 0, errors.New("identity ledger array is not closed")
			}
		default:
			return IdentityLedger{}, 0, fmt.Errorf("identity ledger contains unknown field %q", key)
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') || !seenSchema || !seenIdentities || result.SchemaVersion != 1 {
		return IdentityLedger{}, 0, errors.New("identity ledger is incomplete or unsupported")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return IdentityLedger{}, 0, errors.New("identity ledger contains trailing JSON")
	}
	if err := insert.Close(); err != nil {
		return IdentityLedger{}, 0, err
	}
	if err := tx.Commit(); err != nil {
		return IdentityLedger{}, 0, err
	}
	if err := validateIdentityIndex(ctx, index); err != nil {
		return IdentityLedger{}, 0, err
	}
	return result, count, nil
}

// StreamFileIdentities revalidates the identity ledger and yields every
// record without retaining Item/Link identities. Callers importing a package
// should still run InspectFile first so every non-identity entry is verified.
func StreamFileIdentities(ctx context.Context, filename string, yield func(IdentityRecord) error) (int, error) {
	if yield == nil {
		return 0, errors.New("portable identity consumer is required")
	}
	archive, err := zip.OpenReader(filename)
	if err != nil {
		return 0, fmt.Errorf("opening portable metadata package: %w", err)
	}
	defer archive.Close()
	var identityEntry *zip.File
	for _, entry := range archive.File {
		if entry.Name == "identity-ledger.json" {
			if identityEntry != nil {
				return 0, errors.New("portable metadata package repeats identity-ledger.json")
			}
			identityEntry = entry
		}
	}
	if identityEntry == nil {
		return 0, errors.New("portable metadata package is missing identity-ledger.json")
	}
	_, count, err := inspectIdentityLedger(ctx, identityEntry, yield)
	return count, err
}

func portableObjectIdentityKind(kind string) bool {
	switch kind {
	case "COSER", "WORK", "CHARACTER", "TAG", "SOCIAL_ACCOUNT", "GALLERY":
		return true
	default:
		return false
	}
}

func validateIdentityIndex(ctx context.Context, index *sql.DB) error {
	var invalid int
	if err := index.QueryRowContext(ctx, `SELECT COUNT(*) FROM identities alias
		LEFT JOIN identities target ON target.uuid=alias.target_uuid
		WHERE alias.state='ALIAS' AND (target.uuid IS NULL OR target.kind<>alias.kind OR target.state='TOMBSTONE')`).Scan(&invalid); err != nil {
		return err
	}
	if invalid != 0 {
		return errors.New("portable identity Alias has a missing, mismatched, or retired target")
	}
	if err := index.QueryRowContext(ctx, `WITH RECURSIVE resolved(uuid) AS (
		SELECT uuid FROM identities WHERE state='ACTIVE'
		UNION
		SELECT alias.uuid FROM identities alias JOIN resolved ON alias.target_uuid=resolved.uuid WHERE alias.state='ALIAS'
	) SELECT COUNT(*) FROM identities WHERE state='ALIAS' AND uuid NOT IN (SELECT uuid FROM resolved)`).Scan(&invalid); err != nil {
		return err
	}
	if invalid != 0 {
		return errors.New("portable identity Alias graph contains a cycle or unresolved chain")
	}
	return nil
}
