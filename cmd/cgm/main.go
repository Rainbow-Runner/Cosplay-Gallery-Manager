package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/stashapp/stash/internal/build"
	"github.com/stashapp/stash/internal/persistence/productdb"
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
	for _, selected := range []bool{*setupTicket, *createBackup, *restoreBackup != "", *mapRestoredPaths, *resumeMaintenance} {
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
	if *createBackup || *restoreBackup != "" || *mapRestoredPaths || *resumeMaintenance {
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
