package productserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/stashapp/stash/internal/build"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portablecatalog"
	"github.com/stashapp/stash/internal/portableid"
)

type PortableMergePrepareResult struct {
	MergeID string
	State   string
	Report  PortableMergePreflightReport
}

type PortableMergeApplyResult struct {
	MergeID, SafetyBackupID                                   string
	NewCoreIdentities, ReusedIdentities, PendingGalleryClaims int
}

// PreparePortableMerge retains an already preflighted package and its opaque
// conflict identities. It does not import an identity, entity, asset, or
// Gallery and intentionally does not create a restore snapshot yet.
func (s *Server) PreparePortableMerge(ctx context.Context, sourcePath string) (result PortableMergePrepareResult, returnErr error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	mergeID := portableid.New()
	result.MergeID = mergeID
	defer func() {
		outcome, code := "SUCCESS", ""
		if returnErr != nil {
			outcome, code = "FAILURE", "PORTABLE_MERGE_PREPARE_FAILED"
		}
		_ = s.Database.Operations().Audit(context.Background(), "PORTABLE_MERGE_PREPARE", "PORTABLE_MERGE", mergeID, outcome, code, map[string]any{"hard_blocking": result.Report.HardBlockingCount(), "review": result.Report.ReviewCount()}, time.Now())
	}()
	report, err := s.preflightPortableMerge(ctx, sourcePath)
	if err != nil {
		return result, err
	}
	roots, err := s.Database.Operations().StorageRoots(ctx)
	if err != nil {
		return result, err
	}
	mergeRoot := filepath.Join(roots.BackupRoot, "portable-merges", mergeID)
	if err := os.MkdirAll(mergeRoot, 0o700); err != nil {
		return result, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(mergeRoot)
		}
	}()
	packagePath := filepath.Join(mergeRoot, "package.zip")
	digest, err := copyPortableImportArchive(sourcePath, packagePath)
	if err != nil {
		return result, err
	}
	if digest != report.PackageSHA256 {
		return result, errors.New("portable merge source changed before retention")
	}
	// Bind the stored conflicts to a fresh comparison of the retained bytes and
	// the current target, not to the caller-controlled source path.
	report, err = s.preflightPortableMerge(ctx, packagePath)
	if err != nil {
		return result, err
	}
	if report.PackageSHA256 != digest {
		return result, errors.New("retained portable merge package digest changed")
	}
	conflicts := make([]productdb.PortableMergeConflict, 0, len(report.Issues))
	for _, issue := range report.Issues {
		conflicts = append(conflicts, productdb.PortableMergeConflict{IssueKey: issue.IssueKey, IssueCode: issue.Code, Severity: issue.Severity, EntityKind: issue.EntityKind, IncomingUUID: issue.IncomingUUID, LocalUUID: issue.LocalUUID, FieldKey: issue.FieldKey})
	}
	relativePackage := filepath.ToSlash(filepath.Join("portable-merges", mergeID, "package.zip"))
	if err := s.Database.CreatePortableMergeSession(ctx, productdb.PortableMergeSessionInput{
		MergeID: mergeID, ExportID: report.ExportID, PackageSHA256: digest, PackageRelativePath: relativePackage,
		TargetFingerprint: report.TargetFingerprint, FormatVersion: 1,
		IdentityAdd: report.IdentityAdd, IdentityReuse: report.IdentityReuse, EntityAdd: report.EntityAdd, EntityReuse: report.EntityReuse, Issues: conflicts,
	}, time.Now()); err != nil {
		return result, err
	}
	session, err := s.Database.FindPortableMergeSession(ctx, mergeID)
	if err != nil {
		return result, err
	}
	cleanup = false
	result.State, result.Report = session.State, report
	return result, nil
}

func (s *Server) SetPortableMergeDecisions(ctx context.Context, mergeID string, decisions []productdb.PortableMergeDecision) error {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if _, err := portableid.Parse(mergeID); err != nil {
		return err
	}
	session, err := s.Database.FindPortableMergeSession(ctx, mergeID)
	if err != nil {
		return err
	}
	roots, err := s.Database.Operations().StorageRoots(ctx)
	if err != nil {
		return err
	}
	packagePath := filepath.Join(roots.BackupRoot, filepath.FromSlash(session.PackageRelativePath))
	report, err := s.preflightPortableMerge(ctx, packagePath)
	if err != nil {
		return err
	}
	if report.PackageSHA256 != session.PackageSHA256 || report.TargetFingerprint != session.TargetFingerprint {
		_ = s.Database.MarkPortableMergeStale(ctx, mergeID, time.Now())
		return errors.New("portable merge package or target changed after preflight")
	}
	if err := s.Database.SetPortableMergeDecisions(ctx, mergeID, report.TargetFingerprint, decisions, time.Now()); err != nil {
		return err
	}
	_ = s.Database.Operations().Audit(ctx, "PORTABLE_MERGE_DECISIONS", "PORTABLE_MERGE", mergeID, "SUCCESS", "", map[string]any{"decision_count": len(decisions)}, time.Now())
	return nil
}

func (s *Server) ApplyPortableMerge(ctx context.Context, mergeID string) (result PortableMergeApplyResult, returnErr error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	result.MergeID = mergeID
	defer func() {
		outcome, code := "SUCCESS", ""
		if returnErr != nil {
			outcome, code = "FAILURE", "PORTABLE_MERGE_APPLY_FAILED"
		}
		_ = s.Database.Operations().Audit(context.Background(), "PORTABLE_MERGE_APPLY", "PORTABLE_MERGE", mergeID, outcome, code, map[string]any{"new_core_identities": result.NewCoreIdentities, "reused_identities": result.ReusedIdentities, "pending_gallery_claims": result.PendingGalleryClaims}, time.Now())
	}()
	if _, err := portableid.Parse(mergeID); err != nil {
		return result, err
	}
	maintenance, err := s.Database.Operations().Maintenance(ctx)
	if err != nil {
		return result, err
	}
	if maintenance.Mode != productdb.MaintenanceNormal {
		return result, errors.New("portable merge is unavailable during maintenance")
	}
	session, err := s.Database.FindPortableMergeSession(ctx, mergeID)
	if err != nil {
		return result, err
	}
	if session.State != "READY" || session.HardBlockingCount != 0 {
		return result, errors.New("only a fully reviewed portable merge can be applied")
	}
	roots, err := s.Database.Operations().StorageRoots(ctx)
	if err != nil {
		return result, err
	}
	packagePath := filepath.Join(roots.BackupRoot, filepath.FromSlash(session.PackageRelativePath))
	report, err := s.preflightPortableMerge(ctx, packagePath)
	if err != nil {
		return result, err
	}
	if report.PackageSHA256 != session.PackageSHA256 || report.TargetFingerprint != session.TargetFingerprint {
		_ = s.Database.MarkPortableMergeStale(ctx, mergeID, time.Now())
		return result, errors.New("portable merge package or target changed before apply")
	}
	conflicts, err := s.Database.ListPortableMergeConflicts(ctx, mergeID)
	if err != nil {
		return result, err
	}
	if err := validatePortableMergeReviewBinding(report.Issues, conflicts); err != nil {
		return result, err
	}
	inspection, err := portablecatalog.InspectFile(ctx, packagePath)
	if err != nil {
		return result, err
	}
	version, _, _ := build.Version()
	safety, err := s.Database.Backups().CreateSafetyFull(ctx, productdb.FullBackupOptions{BackupRoot: roots.BackupRoot, CoserMetadataRoot: roots.CoserMetadataRoot, ProductVersion: version, StartupConfig: s.Config}, time.Now())
	if err != nil {
		return result, err
	}
	result.SafetyBackupID = safety.ID
	if err := s.Database.Operations().SetMaintenance(ctx, productdb.MaintenancePortableMerging, mergeID, "", time.Now()); err != nil {
		return result, err
	}
	maintenanceSet := true
	defer func() {
		if returnErr != nil && maintenanceSet {
			_ = s.Database.Operations().SetMaintenance(context.Background(), productdb.MaintenanceNormal, "", "PORTABLE_MERGE_APPLY_FAILED", time.Now())
		}
	}()
	stageRoot, assetCosers, err := stagePortableCoserAssets(ctx, packagePath, roots.CoserMetadataRoot, mergeID, inspection)
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(stageRoot)
	contentDecision := map[string]string{}
	mapped := map[string]bool{}
	for _, conflict := range conflicts {
		if conflict.IssueCode == "PORTABLE_CORE_ENTITY_CONTENT_CONFLICT" {
			contentDecision[conflict.IncomingUUID] = conflict.Decision
		}
		if (conflict.IssueCode == "PORTABLE_CORE_NAME_MATCH_REVIEW" || conflict.IssueCode == "PORTABLE_SOCIAL_ACCOUNT_URL_REVIEW") && conflict.Decision == "MAP_TO_LOCAL" {
			mapped[conflict.IncomingUUID] = true
		}
	}
	publishCosers := assetCosers[:0]
	replaceCosers := []string{}
	for _, coserUUID := range assetCosers {
		var count int
		if err := s.Database.QueryRowContext(ctx, `SELECT COUNT(*) FROM cosers WHERE uuid=?`, coserUUID).Scan(&count); err != nil {
			return result, err
		}
		if count != 0 || mapped[coserUUID] {
			if contentDecision[coserUUID] == "USE_INCOMING" {
				replaceCosers = append(replaceCosers, coserUUID)
				continue
			}
			if err := os.RemoveAll(filepath.Join(stageRoot, coserUUID)); err != nil {
				return result, err
			}
			continue
		}
		publishCosers = append(publishCosers, coserUUID)
	}
	publication := &portableMergeAssetPublication{mergeID: mergeID, coserRoot: roots.CoserMetadataRoot, stageRoot: stageRoot,
		rollbackRoot: filepath.Join(roots.BackupRoot, "portable-merges", mergeID, "asset-rollback"), newCosers: publishCosers, replaceCosers: replaceCosers}
	committed := false
	defer func() {
		if !committed {
			publication.Rollback()
		}
	}()
	merged, err := s.Database.MergePortableCore(ctx, mergeID, report.TargetFingerprint, safety.ID, inspection.Bundle, conflicts, func(yield func(portablecatalog.IdentityRecord) error) (int, error) {
		return portablecatalog.StreamFileIdentities(ctx, packagePath, yield)
	}, publication.Publish, time.Now())
	if err != nil {
		return result, err
	}
	committed = true
	if err := publication.Finalize(); err != nil {
		maintenanceSet = false
		return result, errors.New("portable merge completed but asset publication could not be finalized")
	}
	if err := s.Database.Operations().SetMaintenance(ctx, productdb.MaintenanceNormal, "", "", time.Now()); err != nil {
		maintenanceSet = false
		return result, errors.New("portable merge completed but maintenance mode could not be resumed")
	}
	maintenanceSet = false
	result.NewCoreIdentities, result.ReusedIdentities, result.PendingGalleryClaims = merged.NewCoreIdentities, merged.ReusedIdentities, merged.PendingGalleryClaims
	return result, nil
}

// Retained compatibility for callers/tests created during the conflict-free slice.
func (s *Server) ApplyConflictFreePortableMerge(ctx context.Context, mergeID string) (PortableMergeApplyResult, error) {
	return s.ApplyPortableMerge(ctx, mergeID)
}

func validatePortableMergeReviewBinding(issues []PortableMergeIssue, conflicts []productdb.PortableMergeConflict) error {
	if len(issues) != len(conflicts) {
		return errors.New("portable merge findings changed after decisions")
	}
	stored := make(map[string]productdb.PortableMergeConflict, len(conflicts))
	for _, value := range conflicts {
		stored[value.IssueKey] = value
	}
	for _, issue := range issues {
		value, ok := stored[issue.IssueKey]
		if !ok || value.IssueCode != issue.Code || value.Severity != issue.Severity || value.IncomingUUID != issue.IncomingUUID || value.LocalUUID != issue.LocalUUID || value.FieldKey != issue.FieldKey {
			return errors.New("portable merge findings changed after decisions")
		}
		if value.Severity == "REVIEW" && value.Decision == "UNRESOLVED" {
			return errors.New("portable merge review decision is unresolved")
		}
	}
	return nil
}
