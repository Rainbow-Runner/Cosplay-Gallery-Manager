package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/stashapp/stash/internal/build"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portablecatalog"
	"github.com/stashapp/stash/internal/product"
	"github.com/stashapp/stash/internal/productlog"
	"github.com/stashapp/stash/internal/productserver"
	"golang.org/x/term"
)

func main() {
	configPath := flag.String("config", "cgm.json", "startup configuration JSON")
	showVersion := flag.Bool("version", false, "print the product version")
	setupTicket := flag.Bool("setup-ticket", false, "generate a one-time 15-minute Docker Setup ticket")
	createBackup := flag.Bool("create-backup", false, "create a consistent full backup package and exit")
	restoreBackup := flag.String("restore-backup", "", "restore a backup UUID through maintenance mode and exit")
	mapRestoredPaths := flag.Bool("map-restored-paths", false, "map or disable every restored media library and exit")
	resumeMaintenance := flag.Bool("resume-maintenance", false, "validate the environment, resume schedules and exit")
	exportPortable := flag.String("export-portable", "", "export portable core metadata to a new .zip path and exit")
	importPortable := flag.String("import-portable", "", "import portable core metadata into an empty business database and exit")
	mapPortableLibrariesFlag := flag.String("map-portable-libraries", "", "map every logical library for a portable import UUID and exit")
	preflightPortableRebuild := flag.String("preflight-portable-rebuild", "", "validate mapped Gallery sources for a portable import UUID and exit")
	rebuildPortableGalleries := flag.String("rebuild-portable-galleries", "", "rebuild mapped Galleries for a portable import UUID and exit")
	preflightPortableMerge := flag.String("preflight-portable-merge", "", "compare a portable metadata .zip with this database without changing it")
	preparePortableMerge := flag.String("prepare-portable-merge", "", "retain a portable metadata .zip and persist its merge conflicts")
	decidePortableMerge := flag.String("decide-portable-merge", "", "record every review decision for a prepared portable merge UUID")
	applyPortableMerge := flag.String("apply-portable-merge", "", "apply a fully reviewed portable merge UUID")
	abortPortableMerge := flag.String("abort-portable-merge", "", "abort a portable merge before it has changed business data")
	recoverPortableMerge := flag.String("recover-portable-merge", "", "recover interrupted Coser asset publication for a portable merge UUID")
	recoverPortable := flag.Bool("recover-portable-import", false, "recover an interrupted portable metadata import and exit")
	preflightPortable := flag.Bool("preflight-portable", false, "validate portable export readiness without creating a package")
	allowIncompletePortable := flag.Bool("allow-incomplete-portable", false, "allow a portable export whose Gallery manifest index has warnings")
	inspectPortable := flag.String("inspect-portable", "", "verify a portable metadata .zip without opening the product database")
	flag.Parse()
	if *showVersion {
		version, gitHash, _ := build.Version()
		if gitHash != "" {
			fmt.Printf("%s %s (%s)\n", product.WorkingName, product.CurrentVersions(version).Product, gitHash)
		} else {
			fmt.Println(product.WorkingName, product.CurrentVersions(version).Product)
		}
		return
	}
	if *inspectPortable != "" {
		if *setupTicket || *createBackup || *restoreBackup != "" || *mapRestoredPaths || *resumeMaintenance || *exportPortable != "" || *importPortable != "" || *mapPortableLibrariesFlag != "" || *preflightPortableRebuild != "" || *rebuildPortableGalleries != "" || *preflightPortableMerge != "" || *preparePortableMerge != "" || *decidePortableMerge != "" || *applyPortableMerge != "" || *abortPortableMerge != "" || *recoverPortableMerge != "" || *recoverPortable || *preflightPortable || *allowIncompletePortable {
			fatal("CGM_CLI_ACTION_CONFLICT")
		}
		inspection, err := portablecatalog.InspectFile(context.Background(), *inspectPortable)
		if err != nil {
			fatal("CGM_PORTABLE_INSPECT_FAILED")
		}
		manifest := inspection.Manifest
		fmt.Printf("portable export %s verified: %d identities, %d cosers, %d works, %d characters, %d tags, %d accounts, %d galleries, %d assets\n",
			manifest.ExportID, manifest.IdentityCount, manifest.CoserCount, manifest.WorkCount, manifest.CharacterCount, manifest.TagCount, manifest.AccountCount, manifest.GalleryCount, manifest.AssetCount)
		return
	}
	if *allowIncompletePortable && *exportPortable == "" {
		fatal("CGM_CLI_ACTION_CONFLICT")
	}
	config, err := productserver.LoadConfig(*configPath)
	if err != nil {
		fatal("CGM_CONFIG_INVALID")
	}
	if err := productlog.Configure(config.LogLevel, os.Stderr); err != nil {
		fatal("CGM_LOG_CONFIG_INVALID")
	}
	server, err := productserver.OpenWithProviders(config, configuredCoserMetadataProviders(), configuredEntityMetadataProviders())
	if err != nil {
		fatal("CGM_DATABASE_OPEN_FAILED")
	}
	defer server.Close()
	actionCount := 0
	for _, selected := range []bool{*setupTicket, *createBackup, *restoreBackup != "", *mapRestoredPaths, *resumeMaintenance, *exportPortable != "", *importPortable != "", *mapPortableLibrariesFlag != "", *preflightPortableRebuild != "", *rebuildPortableGalleries != "", *preflightPortableMerge != "", *preparePortableMerge != "", *decidePortableMerge != "", *applyPortableMerge != "", *abortPortableMerge != "", *recoverPortableMerge != "", *recoverPortable, *preflightPortable} {
		if selected {
			actionCount++
		}
	}
	if actionCount > 1 {
		fatal("CGM_CLI_ACTION_CONFLICT")
	}
	if *setupTicket {
		ticket, expires, err := server.Auth.CreateSetupTicket(context.Background())
		if err != nil {
			fatal("CGM_SETUP_TICKET_FAILED")
		}
		fmt.Printf("%s\nexpires %s\n", ticket, expires.Local().Format(time.RFC3339))
		return
	}
	if *createBackup || *restoreBackup != "" || *mapRestoredPaths || *resumeMaintenance || *exportPortable != "" || *importPortable != "" || *mapPortableLibrariesFlag != "" || *preflightPortableRebuild != "" || *rebuildPortableGalleries != "" || *preflightPortableMerge != "" || *preparePortableMerge != "" || *decidePortableMerge != "" || *applyPortableMerge != "" || *abortPortableMerge != "" || *recoverPortableMerge != "" || *recoverPortable || *preflightPortable {
		if err := authenticateOwner(server); err != nil {
			fatal("CGM_OWNER_REAUTH_FAILED")
		}
	}
	if *createBackup {
		record, err := server.CreateFullBackup(context.Background())
		if err != nil {
			fatal("CGM_BACKUP_CREATE_FAILED")
		}
		fmt.Printf("backup %s ready (%s)\n", record.ID, record.FileName)
		return
	}
	if *preflightPortable {
		result, err := server.PreflightPortableMetadata(context.Background())
		if err != nil {
			fatal("CGM_PORTABLE_PREFLIGHT_FAILED")
		}
		fmt.Printf("portable preflight: %d identities, %d core entities, %d galleries (%d incomplete), %d assets (%d bytes)\n",
			result.IdentityCount, result.CoreEntityCount, result.GalleryCount, result.IncompleteGalleryCount, result.AssetCount, result.AssetBytes)
		for _, kind := range []string{"GALLERY", "GALLERY_ITEM", "COSER", "WORK", "CHARACTER", "TAG", "EXTERNAL_LINK", "SOCIAL_ACCOUNT"} {
			if count := result.IdentityByKind[kind]; count > 0 {
				fmt.Printf("identity kind %s: %d\n", kind, count)
			}
		}
		for _, state := range []string{"ACTIVE", "ALIAS", "TOMBSTONE"} {
			if count := result.IdentityByState[state]; count > 0 {
				fmt.Printf("identity state %s: %d\n", state, count)
			}
		}
		for _, issue := range result.Issues {
			fmt.Printf("%s %s: %d\n", issue.Severity, issue.Code, issue.Count)
		}
		if result.BlockingCount() > 0 {
			fatal("CGM_PORTABLE_PREFLIGHT_BLOCKED")
		}
		return
	}
	if *exportPortable != "" {
		target, err := filepath.Abs(*exportPortable)
		if err != nil {
			fatal("CGM_PORTABLE_EXPORT_TARGET_INVALID")
		}
		result, err := server.ExportPortableMetadata(context.Background(), productserver.PortableExportOptions{TargetPath: target, AllowIncompleteGallery: *allowIncompletePortable})
		if err != nil {
			fatal("CGM_PORTABLE_EXPORT_FAILED")
		}
		fmt.Printf("portable export %s ready (%s, %d bytes, %d warnings)\n", result.ExportID, result.FileName, result.ByteSize, result.WarningCount)
		return
	}
	if *importPortable != "" {
		source, err := filepath.Abs(*importPortable)
		if err != nil {
			fatal("CGM_PORTABLE_IMPORT_SOURCE_INVALID")
		}
		fmt.Fprint(os.Stderr, "Type IMPORT to import core metadata and reserve Gallery identities in this empty database: ")
		confirmation, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.TrimSpace(confirmation) != "IMPORT" {
			fatal("CGM_PORTABLE_IMPORT_NOT_CONFIRMED")
		}
		result, err := server.ImportPortableMetadata(context.Background(), productserver.PortableImportOptions{SourcePath: source})
		if err != nil {
			fatal("CGM_PORTABLE_IMPORT_FAILED")
		}
		fmt.Printf("portable import %s completed from export %s: %d core entities, %d pending Gallery identities, %d assets; safety backup %s\n", result.ImportID, result.ExportID, result.CoreEntityCount, result.ClaimCount, result.AssetCount, result.SafetyBackupID)
		return
	}
	if *recoverPortable {
		result, err := server.RecoverPortableMetadataImport(context.Background())
		if err != nil {
			fatal("CGM_PORTABLE_IMPORT_RECOVERY_FAILED")
		}
		fmt.Printf("portable import %s recovery completed for export %s\n", result.ImportID, result.ExportID)
		return
	}
	if *mapPortableLibrariesFlag != "" {
		if err := mapPortableLibraries(server, *mapPortableLibrariesFlag); err != nil {
			fatal("CGM_PORTABLE_LIBRARY_MAP_FAILED")
		}
		fmt.Println("portable media library decisions saved; no media scan was started")
		return
	}
	if *preflightPortableRebuild != "" {
		report, err := server.PreflightPortableGalleryRebuild(context.Background(), *preflightPortableRebuild)
		if err != nil {
			fatal("CGM_PORTABLE_GALLERY_PREFLIGHT_FAILED")
		}
		printPortableGalleryRebuildReport(report)
		if report.Blocked != 0 {
			fatal("CGM_PORTABLE_GALLERY_PREFLIGHT_BLOCKED")
		}
		return
	}
	if *rebuildPortableGalleries != "" {
		preflight, err := server.PreflightPortableGalleryRebuild(context.Background(), *rebuildPortableGalleries)
		if err != nil {
			fatal("CGM_PORTABLE_GALLERY_PREFLIGHT_FAILED")
		}
		printPortableGalleryRebuildReport(preflight)
		if preflight.Blocked != 0 {
			fatal("CGM_PORTABLE_GALLERY_PREFLIGHT_BLOCKED")
		}
		fmt.Fprint(os.Stderr, "Type REBUILD to create DRAFT Galleries and claim their portable identities: ")
		confirmation, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.TrimSpace(confirmation) != "REBUILD" {
			fatal("CGM_PORTABLE_GALLERY_REBUILD_NOT_CONFIRMED")
		}
		report, err := server.RebuildPortableGalleries(context.Background(), *rebuildPortableGalleries)
		if err != nil {
			fatal("CGM_PORTABLE_GALLERY_REBUILD_FAILED")
		}
		printPortableGalleryRebuildReport(report)
		return
	}
	if *preflightPortableMerge != "" {
		source, err := filepath.Abs(*preflightPortableMerge)
		if err != nil {
			fatal("CGM_PORTABLE_MERGE_SOURCE_INVALID")
		}
		report, err := server.PreflightPortableMerge(context.Background(), source)
		if err != nil {
			fatal("CGM_PORTABLE_MERGE_PREFLIGHT_FAILED")
		}
		fmt.Printf("portable merge preflight %s: %d identities add, %d reuse; %d core entities add, %d reuse; %d hard blockers, %d review decisions\n",
			report.ExportID, report.IdentityAdd, report.IdentityReuse, report.EntityAdd, report.EntityReuse, report.HardBlockingCount(), report.ReviewCount())
		for _, issue := range report.Issues {
			fmt.Printf("%s %s %s incoming=%s local=%s\n", issue.Severity, issue.Code, issue.EntityKind, issue.IncomingUUID, issue.LocalUUID)
		}
		if report.HardBlockingCount() != 0 {
			fatal("CGM_PORTABLE_MERGE_PREFLIGHT_BLOCKED")
		}
		return
	}
	if *preparePortableMerge != "" {
		source, err := filepath.Abs(*preparePortableMerge)
		if err != nil {
			fatal("CGM_PORTABLE_MERGE_SOURCE_INVALID")
		}
		preview, err := server.PreflightPortableMerge(context.Background(), source)
		if err != nil {
			fatal("CGM_PORTABLE_MERGE_PREFLIGHT_FAILED")
		}
		fmt.Printf("portable merge candidate %s: %d hard blockers, %d review decisions\n", preview.ExportID, preview.HardBlockingCount(), preview.ReviewCount())
		fmt.Fprint(os.Stderr, "Type PREPARE to retain this exact package and persist its conflict identities: ")
		confirmation, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.TrimSpace(confirmation) != "PREPARE" {
			fatal("CGM_PORTABLE_MERGE_PREPARE_NOT_CONFIRMED")
		}
		prepared, err := server.PreparePortableMerge(context.Background(), source)
		if err != nil {
			fatal("CGM_PORTABLE_MERGE_PREPARE_FAILED")
		}
		fmt.Printf("portable merge %s prepared in state %s: %d hard blockers, %d review decisions\n", prepared.MergeID, prepared.State, prepared.Report.HardBlockingCount(), prepared.Report.ReviewCount())
		return
	}
	if *decidePortableMerge != "" {
		if err := decidePreparedPortableMerge(server, *decidePortableMerge); err != nil {
			fatal("CGM_PORTABLE_MERGE_DECISION_FAILED")
		}
		fmt.Println("portable merge decisions saved; no business data was imported")
		return
	}
	if *applyPortableMerge != "" {
		fmt.Fprint(os.Stderr, "Type MERGE to create a full safety backup and atomically apply the reviewed portable metadata merge: ")
		confirmation, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.TrimSpace(confirmation) != "MERGE" {
			fatal("CGM_PORTABLE_MERGE_APPLY_NOT_CONFIRMED")
		}
		result, err := server.ApplyPortableMerge(context.Background(), *applyPortableMerge)
		if err != nil {
			fatal("CGM_PORTABLE_MERGE_APPLY_FAILED")
		}
		fmt.Printf("portable merge %s applied: %d new core identities, %d exact identity reuses, %d pending Gallery identities; safety backup %s\n", result.MergeID, result.NewCoreIdentities, result.ReusedIdentities, result.PendingGalleryClaims, result.SafetyBackupID)
		return
	}
	if *abortPortableMerge != "" {
		fmt.Fprint(os.Stderr, "Type ABORT to permanently close this not-yet-applied merge session: ")
		confirmation, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.TrimSpace(confirmation) != "ABORT" {
			fatal("CGM_PORTABLE_MERGE_ABORT_NOT_CONFIRMED")
		}
		if err := server.AbortPortableMerge(context.Background(), *abortPortableMerge); err != nil {
			fatal("CGM_PORTABLE_MERGE_ABORT_FAILED")
		}
		fmt.Println("portable merge aborted; retained package and conflict audit were preserved")
		return
	}
	if *recoverPortableMerge != "" {
		if err := server.RecoverPortableMerge(context.Background(), *recoverPortableMerge); err != nil {
			fatal("CGM_PORTABLE_MERGE_RECOVERY_FAILED")
		}
		fmt.Println("portable merge asset publication recovered")
		return
	}
	if *restoreBackup != "" {
		fmt.Fprint(os.Stderr, "Type RESTORE to create a safety snapshot and replace the live database: ")
		confirmation, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.TrimSpace(confirmation) != "RESTORE" {
			fatal("CGM_RESTORE_NOT_CONFIRMED")
		}
		state, err := server.RestoreBackup(context.Background(), *restoreBackup)
		if err != nil {
			fatal("CGM_RESTORE_FAILED")
		}
		fmt.Printf("restore completed; maintenance state %s; sign in again, run -map-restored-paths, then validate before resuming\n", state.Mode)
		return
	}
	if *mapRestoredPaths {
		if err := mapRestoredMediaLibraries(server); err != nil {
			fatal("CGM_RESTORE_PATH_MAPPING_FAILED")
		}
		fmt.Println("restored media library decisions saved; no media scan was started")
		return
	}
	if *resumeMaintenance {
		if err := server.ResumeMaintenance(context.Background()); err != nil {
			fatal("CGM_MAINTENANCE_RESUME_FAILED")
		}
		fmt.Println("environment validated; automatic schedules resumed")
		return
	}
	httpServer := server.HTTPServer()
	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := server.RunWorkers(stop); err != nil {
		fatal("CGM_WORKER_START_FAILED")
	}
	go func() {
		<-stop.Done()
		ctx, release := context.WithTimeout(context.Background(), 15*time.Second)
		defer release()
		_ = httpServer.Shutdown(ctx)
	}()
	slog.Info("CGM_SERVICE_STARTED", "product", product.WorkingName, "listen", config.Listen, "log_level", config.LogLevel)
	if err := httpServer.ListenAndServe(); err != nil && err.Error() != "http: Server closed" {
		fatal("CGM_HTTP_SERVER_FAILED")
	}
	slog.Info("CGM_SERVICE_STOPPED")
}

func mapRestoredMediaLibraries(server *productserver.Server) error {
	ctx := context.Background()
	state, err := server.Database.Operations().Maintenance(ctx)
	if err != nil {
		return err
	}
	if state.Mode != productdb.MaintenanceWaitingValidation || state.LastErrorCode != productdb.RestorePathMappingRequired {
		return fmt.Errorf("restore path mapping is not pending")
	}
	libraries, err := server.Database.Libraries().List(ctx)
	if err != nil {
		return err
	}
	reader := bufio.NewReader(os.Stdin)
	mappings := make([]productdb.RestorePathMapping, 0, len(libraries))
	for _, mediaLibrary := range libraries {
		fmt.Fprintf(os.Stderr, "\n%s\nRestored root: %s\nNew absolute root on this machine (leave empty to disable): ",
			mediaLibrary.Name, mediaLibrary.RootPath)
		value, err := reader.ReadString('\n')
		if err != nil && len(value) == 0 {
			return err
		}
		value = strings.TrimSpace(value)
		mappings = append(mappings, productdb.RestorePathMapping{
			LibraryID: mediaLibrary.ID, ExpectedRootPath: mediaLibrary.RootPath,
			RootPath: value, Disable: value == "",
		})
	}
	fmt.Fprint(os.Stderr, "Type MAP to confirm every restored media library decision: ")
	confirmation, err := reader.ReadString('\n')
	if err != nil && len(confirmation) == 0 {
		return err
	}
	if strings.TrimSpace(confirmation) != "MAP" {
		return fmt.Errorf("restore path mapping was not confirmed")
	}
	return server.ApplyRestorePathMappings(ctx, mappings)
}

func mapPortableLibraries(server *productserver.Server, importID string) error {
	ctx := context.Background()
	mappings, err := server.Database.ListPortableLibraryMappings(ctx, importID)
	if err != nil {
		return err
	}
	libraries, err := server.Database.Libraries().List(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Enabled target media libraries:")
	valid := map[int64]bool{}
	for _, mediaLibrary := range libraries {
		if mediaLibrary.Enabled {
			valid[mediaLibrary.ID] = true
			fmt.Fprintf(os.Stderr, "  %d: %s (%s)\n", mediaLibrary.ID, mediaLibrary.Name, mediaLibrary.RootPath)
		}
	}
	reader := bufio.NewReader(os.Stdin)
	decisions := make([]productdb.PortableLibraryDecision, 0, len(mappings))
	for _, mapping := range mappings {
		fmt.Fprintf(os.Stderr, "\nLogical library %s (%s)\nTarget library ID (leave empty to skip): ", mapping.LibraryName, mapping.LibraryKey)
		value, err := reader.ReadString('\n')
		if err != nil && len(value) == 0 {
			return err
		}
		value = strings.TrimSpace(value)
		decision := productdb.PortableLibraryDecision{LibraryKey: mapping.LibraryKey}
		if value != "" {
			var targetID int64
			if _, err := fmt.Sscan(value, &targetID); err != nil || !valid[targetID] {
				return fmt.Errorf("target media library ID is invalid")
			}
			decision.TargetLibraryID = &targetID
		}
		decisions = append(decisions, decision)
	}
	fmt.Fprint(os.Stderr, "Type MAP to confirm every logical media library decision: ")
	confirmation, err := reader.ReadString('\n')
	if err != nil && len(confirmation) == 0 {
		return err
	}
	if strings.TrimSpace(confirmation) != "MAP" {
		return fmt.Errorf("portable media library mapping was not confirmed")
	}
	return server.MapPortableLibraries(ctx, importID, decisions)
}

func printPortableGalleryRebuildReport(report productserver.PortableGalleryRebuildReport) {
	fmt.Printf("portable Gallery rebuild %s: %d ready, %d blocked, %d skipped, %d rebuilt\n", report.ImportID, report.Ready, report.Blocked, report.Skipped, report.Rebuilt)
	for _, entry := range report.Entries {
		fmt.Printf("%s %s %s %s %s\n", entry.State, entry.SetID, entry.SourceType, entry.LibraryKey, entry.RelativeSource)
		if entry.IssueCode != "" {
			fmt.Printf("  issue: %s\n", entry.IssueCode)
		}
	}
}

func decidePreparedPortableMerge(server *productserver.Server, mergeID string) error {
	ctx := context.Background()
	session, err := server.Database.FindPortableMergeSession(ctx, mergeID)
	if err != nil {
		return err
	}
	if session.State != "DECISIONS_PENDING" {
		return fmt.Errorf("portable merge is not awaiting review decisions")
	}
	conflicts, err := server.Database.ListPortableMergeConflicts(ctx, mergeID)
	if err != nil {
		return err
	}
	reader := bufio.NewReader(os.Stdin)
	decisions := make([]productdb.PortableMergeDecision, 0, session.ReviewCount)
	for _, conflict := range conflicts {
		if conflict.Severity != "REVIEW" {
			continue
		}
		allowed := "KEEP_LOCAL or USE_INCOMING"
		if conflict.IssueCode == "PORTABLE_CORE_NAME_MATCH_REVIEW" || conflict.IssueCode == "PORTABLE_SOCIAL_ACCOUNT_URL_REVIEW" {
			allowed = "KEEP_SEPARATE or MAP_TO_LOCAL"
		}
		fmt.Fprintf(os.Stderr, "\n%s %s incoming=%s local=%s\nDecision (%s): ", conflict.IssueCode, conflict.EntityKind, conflict.IncomingUUID, conflict.LocalUUID, allowed)
		value, err := reader.ReadString('\n')
		if err != nil && len(value) == 0 {
			return err
		}
		decisions = append(decisions, productdb.PortableMergeDecision{IssueKey: conflict.IssueKey, Decision: strings.TrimSpace(value)})
	}
	fmt.Fprint(os.Stderr, "Type DECIDE to bind all decisions to the retained package and current target fingerprint: ")
	confirmation, err := reader.ReadString('\n')
	if err != nil && len(confirmation) == 0 {
		return err
	}
	if strings.TrimSpace(confirmation) != "DECIDE" {
		return fmt.Errorf("portable merge decisions were not confirmed")
	}
	return server.SetPortableMergeDecisions(ctx, mergeID, decisions)
}

func authenticateOwner(server *productserver.Server) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("owner reauthentication requires an interactive terminal")
	}
	fmt.Fprint(os.Stderr, "Owner password: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return err
	}
	return server.Auth.VerifyPassword(context.Background(), string(password))
}

func fatal(code string) {
	slog.Error(code)
	os.Exit(1)
}
