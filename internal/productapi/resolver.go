package productapi

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portableid"
)

// Resolver is intentionally backed only by the product database Browse stores.
// It cannot reach legacy Scene/Image models or physical source paths.
type Resolver struct {
	Database   *productdb.Database
	Operations OperationsService
}

func (r *Resolver) auditManage(ctx context.Context, eventCode, targetKind, targetID, failureCode string, operationErr error, summary map[string]any) {
	outcome, errorCode := "SUCCESS", ""
	if operationErr != nil {
		outcome, errorCode = "FAILURE", failureCode
	}
	_ = r.Database.Operations().Audit(ctx, eventCode, targetKind, targetID, outcome, errorCode, summary, time.Now())
}

func (r *Resolver) coserMetadataRoot(ctx context.Context) (string, error) {
	roots, err := r.Database.Operations().StorageRoots(ctx)
	if err != nil {
		return "", manageError(err)
	}
	return roots.CoserMetadataRoot, nil
}

func (r *mutationResolver) loadManageGalleryDetail(ctx context.Context, setID string) (*ManageGalleryDetail, error) {
	value, err := r.Database.Manage().GalleryDetail(ctx, setID)
	if err != nil {
		return nil, manageError(err)
	}
	return manageGalleryDetail(value), nil
}

func (r *Resolver) manageGalleryManifestState(ctx context.Context, state productdb.GalleryManifestState) (*ManageGalleryManifestState, error) {
	var metadataRevision int64
	if err := r.Database.QueryRowContext(ctx, `SELECT metadata_revision FROM galleries WHERE id=?`, state.GalleryID).Scan(&metadataRevision); err != nil {
		return nil, manageError(err)
	}
	result := &ManageGalleryManifestState{
		Status: string(state.Status), Path: state.Path, ManifestRevision: int(state.ManifestRevision), MetadataRevision: metadataRevision,
	}
	for _, conflict := range state.Conflicts {
		result.Conflicts = append(result.Conflicts, &ManageManifestConflict{
			Path: conflict.Path, BaselineJSON: manifestConflictJSON(conflict.Baseline, conflict.BaselinePresent),
			DatabaseJSON: manifestConflictJSON(conflict.Database, conflict.DatabasePresent),
			FileJSON:     manifestConflictJSON(conflict.File, conflict.FilePresent),
		})
	}
	return result, nil
}

func (r *Resolver) manageCoserManifestState(ctx context.Context, state productdb.CoserManifestState) (*ManageCoserManifestState, error) {
	var metadataRevision int64
	if err := r.Database.QueryRowContext(ctx, `SELECT metadata_revision FROM cosers WHERE uuid=?`, state.CoserUUID).Scan(&metadataRevision); err != nil {
		return nil, manageError(err)
	}
	result := &ManageCoserManifestState{
		Status: string(state.Status), Path: state.Path, ManifestRevision: int(state.ManifestRevision), MetadataRevision: metadataRevision,
	}
	for _, conflict := range state.Conflicts {
		result.Conflicts = append(result.Conflicts, &ManageManifestConflict{
			Path: conflict.Path, BaselineJSON: manifestConflictJSON(conflict.Baseline, conflict.BaselinePresent),
			DatabaseJSON: manifestConflictJSON(conflict.Database, conflict.DatabasePresent),
			FileJSON:     manifestConflictJSON(conflict.File, conflict.FilePresent),
		})
	}
	return result, nil
}

func manifestConflictJSON(value any, present bool) string {
	if !present {
		return "<missing>"
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "<unavailable>"
	}
	return string(data)
}

func portableCoreKind(kind SearchEntityKind) (portableid.Kind, error) {
	converted := portableid.Kind(kind)
	switch converted {
	case portableid.KindCoser, portableid.KindWork, portableid.KindCharacter, portableid.KindTag:
		return converted, nil
	default:
		return "", errors.New("unsupported core entity kind")
	}
}
