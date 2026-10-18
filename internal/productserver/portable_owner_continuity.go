package productserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/portablecatalog"
)

type PortableOwnerContinuityResult struct {
	ImportID      string
	GalleryCount  int
	ItemCount     int
	ActiveCount   int
	ArchivedCount int
}

func (s *Server) ApplyPortableOwnerContinuity(ctx context.Context, importID string) (result PortableOwnerContinuityResult, returnErr error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	result.ImportID = importID
	defer func() {
		outcome, code := "SUCCESS", ""
		if returnErr != nil {
			outcome, code = "FAILURE", "PORTABLE_OWNER_CONTINUITY_APPLY_FAILED"
		}
		_ = s.Database.Operations().Audit(context.Background(), "PORTABLE_OWNER_CONTINUITY_APPLY", "PORTABLE_IMPORT", importID, outcome, code, map[string]any{"galleries": result.GalleryCount, "items": result.ItemCount, "active": result.ActiveCount, "archived": result.ArchivedCount}, time.Now())
	}()
	session, err := s.Database.FindPortableImportSession(ctx, importID)
	if err != nil {
		return result, err
	}
	roots, err := s.Database.Operations().StorageRoots(ctx)
	if err != nil {
		return result, err
	}
	packagePath := filepath.Join(roots.BackupRoot, filepath.FromSlash(session.PackageRelativePath))
	cleanRoot := filepath.Clean(roots.BackupRoot) + string(os.PathSeparator)
	if !strings.HasPrefix(filepath.Clean(packagePath), cleanRoot) {
		return result, errors.New("portable owner continuity package escaped backup root")
	}
	digest, err := portableFileSHA256(packagePath)
	if err != nil || digest != session.PackageSHA256 {
		return result, errors.New("portable owner continuity package digest is invalid")
	}
	inspection, err := portablecatalog.InspectFile(ctx, packagePath)
	if err != nil {
		return result, err
	}
	if inspection.Bundle.Owner == nil {
		return result, errors.New("portable package does not contain owner continuity")
	}
	applied, err := s.Database.ApplyPortableOwnerContinuity(ctx, importID, *inspection.Bundle.Owner, time.Now())
	if err != nil {
		return result, err
	}
	result = PortableOwnerContinuityResult{ImportID: importID, GalleryCount: applied.GalleryCount, ItemCount: applied.ItemCount, ActiveCount: applied.ActiveCount, ArchivedCount: applied.ArchivedCount}
	return result, nil
}
