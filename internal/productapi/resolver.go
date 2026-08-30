package productapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/stashapp/stash/internal/mediaclassification"
	"github.com/stashapp/stash/internal/mediaexclusion"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portableid"
	"github.com/stashapp/stash/internal/productlog"
)

// Resolver is intentionally backed only by the product database Browse stores.
// It cannot reach legacy Scene/Image models or physical source paths.
type Resolver struct {
	Database      *productdb.Database
	Operations    OperationsService
	OwnerPassword OwnerPasswordVerifier
}

func (r *Resolver) ffmpegStatus(ctx context.Context) (string, string, error) {
	if r.Operations == nil {
		return "", "FFMPEG_UNAVAILABLE", nil
	}
	dependency, err := r.Operations.VideoDependencyStatus(ctx)
	if err != nil {
		return "", "", err
	}
	if dependency.FFmpegAvailable {
		return dependency.FFmpegVersion, "", nil
	}
	return "", dependency.FFmpegErrorCode, nil
}

func (r *Resolver) auditManage(ctx context.Context, eventCode, targetKind, targetID, failureCode string, operationErr error, summary map[string]any) {
	outcome, errorCode := "SUCCESS", ""
	if operationErr != nil {
		outcome, errorCode = "FAILURE", failureCode
	}
	_ = r.Database.Operations().Audit(ctx, eventCode, targetKind, targetID, outcome, errorCode, summary, time.Now())
	attributes := []any{
		"request_id", productlog.RequestID(ctx),
		"event_code", eventCode,
		"target_kind", targetKind,
		"target_id", targetID,
		"outcome", outcome,
	}
	if operationErr != nil {
		slog.Error("CGM_MANAGE_OPERATION", append(attributes, "error_code", errorCode)...)
	} else {
		slog.Info("CGM_MANAGE_OPERATION", attributes...)
	}
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

func (r *Resolver) manageLibraryAutomation(ctx context.Context, libraryID int64) (*ManageLibraryAutomation, error) {
	policy, err := r.Database.Automation().FindPolicy(ctx, libraryID)
	if err != nil {
		return nil, manageError(err)
	}
	preview, err := r.Database.Automation().Preview(ctx, libraryID)
	if err != nil {
		return nil, manageError(err)
	}
	runs, err := r.Database.Automation().RecentRuns(ctx, libraryID, 20)
	if err != nil {
		return nil, manageError(err)
	}
	result := &ManageLibraryAutomation{Policy: manageAutomationPolicy(policy), Preview: manageAutomationPreview(preview)}
	for _, run := range runs {
		result.RecentRuns = append(result.RecentRuns, manageAutomationRun(run))
	}
	return result, nil
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

func mediaClassificationRuleInput(input MediaClassificationRuleInput) productdb.MediaClassificationRule {
	return productdb.MediaClassificationRule{LibraryID: input.LibraryID, Name: input.Name, Enabled: input.Enabled, Order: input.Order, Subject: mediaclassification.Subject(input.Subject), Operator: mediaclassification.Operator(input.Operator), Pattern: input.Pattern, CaseSensitive: input.CaseSensitive, Category: mediaclassification.Category(input.ResultCategory), Revision: 1}
}

func mediaExclusionRuleInput(input MediaExclusionRuleInput) productdb.MediaExclusionRule {
	result := productdb.MediaExclusionRule{LibraryID: input.LibraryID, Name: input.Name, Enabled: input.Enabled, Order: input.Order,
		Subject: mediaexclusion.Subject(input.Subject), Operator: mediaexclusion.Operator(input.Operator), Pattern: input.Pattern,
		CaseSensitive: input.CaseSensitive, MediaKind: mediaexclusion.MediaKind(input.MediaKind), Decision: mediaexclusion.Decision(input.Decision), Revision: 1}
	if input.ID != nil {
		result.ID = *input.ID
	}
	return result
}

func optionalInt64Target(value *int64) string {
	if value == nil {
		return "GLOBAL"
	}
	return fmt.Sprintf("%d", *value)
}
