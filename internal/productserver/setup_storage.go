package productserver

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/stashapp/stash/internal/productauth"
)

const (
	dockerCoserRoot  = "/var/lib/cgm/cosers"
	dockerBackupRoot = "/var/lib/cgm/backups"
)

func (s *Server) setupOptions() productauth.SetupOptions {
	runtime := s.Config.RuntimeEnvironment
	if runtime == "" {
		runtime = "NATIVE"
	}
	options := productauth.SetupOptions{RuntimeEnvironment: runtime,
		AllowDockerLocal: s.Config.LocalDockerSetup, ValidateStorage: s.validateSetupStorage}
	if runtime == "DOCKER" {
		options.CoserMetadataRoot = dockerCoserRoot
		options.BackupRoot = dockerBackupRoot
	}
	return options
}

func (s *Server) validateSetupStorage(input productauth.SetupInput) error {
	if input.CoserMetadataRoot == input.BackupRoot {
		return errors.New("Coser and backup roots must differ")
	}
	if s.Config.RuntimeEnvironment == "DOCKER" && (filepath.Clean(input.CoserMetadataRoot) != dockerCoserRoot || filepath.Clean(input.BackupRoot) != dockerBackupRoot) {
		return errors.New("Docker storage roots must use the mounted state volume")
	}
	if s.Config.RuntimeEnvironment == "DOCKER" {
		mounts, err := os.ReadFile("/proc/self/mountinfo")
		if err != nil {
			return errors.New("Docker mount information is unavailable")
		}
		for _, path := range []string{"/var/lib/cgm", "/var/cache/cgm", "/media", "/transfer"} {
			if !hasMountPoint(string(mounts), path) {
				return errors.New("required Docker volume is not mounted: " + path)
			}
		}
	}
	for _, root := range []string{input.CoserMetadataRoot, input.BackupRoot} {
		if !filepath.IsAbs(root) {
			return errors.New("storage root must be absolute")
		}
		if err := os.MkdirAll(root, 0o700); err != nil {
			return err
		}
		if info, err := os.Lstat(root); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("storage root must be a real directory")
		}
		file, err := os.CreateTemp(root, ".cgm-setup-check-*")
		if err != nil {
			return err
		}
		name := file.Name()
		closeErr := file.Close()
		removeErr := os.Remove(name)
		if closeErr != nil {
			return closeErr
		}
		if removeErr != nil {
			return removeErr
		}
	}
	return nil
}

func hasMountPoint(mountInfo, path string) bool {
	for _, line := range strings.Split(mountInfo, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 5 && fields[4] == path {
			return true
		}
	}
	return false
}

// ValidateMediaLibraryPath checks the container-visible mount contract before
// a new Docker library can be registered. Existing libraries are untouched.
func (s *Server) ValidateMediaLibraryPath(path string) error {
	if s.Config.RuntimeEnvironment != "DOCKER" {
		return nil
	}
	mounts, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil || !hasMountPoint(string(mounts), "/media") {
		return errors.New("Docker media volume is not mounted at /media")
	}
	if !filepath.IsAbs(path) {
		return errors.New("Docker media library path must be absolute")
	}
	clean := filepath.Clean(path)
	relative, err := filepath.Rel("/media", clean)
	if err != nil || relative == ".." || len(relative) >= 3 && relative[:3] == "../" {
		return errors.New("Docker media library path must be under /media")
	}
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return errors.New("Docker media library path is not mounted or readable")
	}
	relative, err = filepath.Rel("/media", resolved)
	if err != nil || relative == ".." || len(relative) >= 3 && relative[:3] == "../" {
		return errors.New("Docker media library path escapes /media")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return errors.New("Docker media library path is not a directory")
	}
	directory, err := os.Open(resolved)
	if err != nil {
		return errors.New("Docker media library path is not readable")
	}
	return directory.Close()
}
