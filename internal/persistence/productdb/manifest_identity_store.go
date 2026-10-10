package productdb

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/stashapp/stash/internal/manifest"
	"github.com/stashapp/stash/internal/manifestidentity"
	"github.com/stashapp/stash/internal/portableid"
)

// inspectManifestIdentity is the read-only database adapter for the shared
// inspector. Source-location policy is deliberately handled by discovery.
func inspectManifestIdentity(ctx context.Context, db portableUUIDQueryer, document manifest.GalleryDocument) (manifestidentity.Result, error) {
	declaration := manifestidentity.Extract(document)
	occupancies := make(map[string]manifestidentity.Occupancy, len(declaration.Identities)+len(declaration.CoreReferences))
	for _, identity := range declaration.Identities {
		occupancy, err := manifestUUIDOccupancy(ctx, db, identity.UUID, identity.Kind)
		if err != nil {
			return manifestidentity.Result{}, err
		}
		occupancies[identity.UUID] = occupancy
	}
	for _, reference := range declaration.CoreReferences {
		occupancy, err := manifestUUIDOccupancy(ctx, db, reference.UUID, reference.Kind)
		if err != nil {
			return manifestidentity.Result{}, err
		}
		compatible := false
		if occupancy.State == manifestidentity.OccupancyActive && occupancy.Kind == reference.Kind {
			compatible, err = manifestCoreReferenceCompatible(ctx, db, reference)
			if err != nil {
				return manifestidentity.Result{}, err
			}
		}
		occupancy.CoreCompatible = &compatible
		occupancies[reference.UUID] = occupancy
	}
	if document.Cast.Present && !document.Cast.Null {
		for _, cast := range document.Cast.Value {
			var workUUID string
			err := db.QueryRowContext(ctx, `SELECT work_uuid FROM characters WHERE uuid=?`, cast.Character.UUID).Scan(&workUUID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return manifestidentity.Result{}, err
			}
			if err != nil || workUUID != cast.Work.UUID {
				occupancy := occupancies[cast.Character.UUID]
				compatible := false
				occupancy.CoreCompatible = &compatible
				occupancies[cast.Character.UUID] = occupancy
			}
		}
	}
	return manifestidentity.Inspect(declaration, occupancies), nil
}

// ManifestCoreReferencesValid reuses the shared read-only inspector while
// ignoring Gallery-local PENDING claims owned by a portable import. Migration
// still validates those claims separately against its import session.
func (db *Database) ManifestCoreReferencesValid(ctx context.Context, document manifest.GalleryDocument) (bool, error) {
	result, err := inspectManifestIdentity(ctx, db.DB, document)
	if err != nil {
		return false, err
	}
	for _, issue := range result.Issues {
		if issue.Code == manifestidentity.ClassCoreReferenceReview {
			return false, nil
		}
	}
	return true, nil
}

func manifestUUIDOccupancy(ctx context.Context, db portableUUIDQueryer, uuid string, expected portableid.Kind) (manifestidentity.Occupancy, error) {
	result := manifestidentity.Occupancy{UUID: uuid, Kind: expected, State: manifestidentity.OccupancyUnclaimed}
	record, err := lookupPortableUUID(ctx, db, uuid)
	if err == nil {
		result.Kind = record.Kind
		switch record.State {
		case PortableUUIDActive:
			result.State = manifestidentity.OccupancyActive
		case PortableUUIDAlias:
			result.State = manifestidentity.OccupancyAlias
		case PortableUUIDTombstone:
			result.State = manifestidentity.OccupancyTombstone
		}
		if record.State == PortableUUIDActive {
			switch record.Kind {
			case portableid.KindGalleryItem:
				_ = db.QueryRowContext(ctx, `SELECT gallery.set_id FROM gallery_items item JOIN galleries gallery ON gallery.id=item.gallery_id WHERE item.item_uuid=?`, uuid).Scan(&result.OwnerSetID)
			case portableid.KindExternalLink:
				_ = db.QueryRowContext(ctx, `SELECT gallery.set_id FROM gallery_external_links link JOIN galleries gallery ON gallery.id=link.gallery_id WHERE link.link_uuid=?`, uuid).Scan(&result.OwnerSetID)
			}
		}
		return result, nil
	}
	if !errors.Is(err, ErrPortableUUIDNotFound) {
		return result, err
	}
	var kind, importID string
	err = db.QueryRowContext(ctx, `SELECT entity_kind,import_id FROM portable_identity_claims WHERE uuid=? AND claim_state<>'CLAIMED'`, uuid).Scan(&kind, &importID)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	result.Kind = portableid.Kind(kind)
	result.State = manifestidentity.OccupancyPending
	result.ClaimImportID = importID
	return result, nil
}

func manifestCoreReferenceCompatible(ctx context.Context, db portableUUIDQueryer, reference manifestidentity.CoreReference) (bool, error) {
	if reference.NameHint == "" {
		return true, nil
	}
	var table, aliasTable, uuidColumn string
	switch reference.Kind {
	case portableid.KindCoser:
		table, aliasTable, uuidColumn = "cosers", "coser_aliases", "coser_uuid"
	case portableid.KindWork:
		table, aliasTable, uuidColumn = "works", "work_aliases", "work_uuid"
	case portableid.KindCharacter:
		table, aliasTable, uuidColumn = "characters", "character_aliases", "character_uuid"
	case portableid.KindTag:
		table, aliasTable, uuidColumn = "tags", "tag_aliases", "tag_uuid"
	default:
		return false, nil
	}
	var count int
	query := `SELECT COUNT(*) FROM (` +
		`SELECT name AS value FROM ` + table + ` WHERE uuid=? ` +
		`UNION ALL SELECT alias AS value FROM ` + aliasTable + ` WHERE ` + uuidColumn + `=?` +
		`) WHERE lower(trim(value))=lower(trim(?))`
	if err := db.QueryRowContext(ctx, query, reference.UUID, reference.UUID, strings.TrimSpace(reference.NameHint)).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}
