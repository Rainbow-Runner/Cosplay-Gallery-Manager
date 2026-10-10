package productserver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/productauth"
)

func TestSetupStorageChecksWritableRootsAndDockerContract(t *testing.T) {
	root := t.TempDir()
	server := &Server{Config: Config{RuntimeEnvironment: "NATIVE"}}
	input := productauth.SetupInput{CoserMetadataRoot: filepath.Join(root, "cosers"), BackupRoot: filepath.Join(root, "backups")}
	if err := server.validateSetupStorage(input); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{input.CoserMetadataRoot, input.BackupRoot} {
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			t.Fatalf("storage path %s = %v %v", path, info, err)
		}
	}
	server.Config.RuntimeEnvironment = "DOCKER"
	if err := server.validateSetupStorage(input); err == nil {
		t.Fatal("Docker accepted unmounted arbitrary roots")
	}
}

func TestMountPointMatcherUsesExactContainerPath(t *testing.T) {
	info := "101 1 0:55 / /var/lib/cgm rw - ext4 /dev/sda rw\n102 1 0:56 / /var/cache/cgm rw - ext4 /dev/sda rw\n"
	if !hasMountPoint(info, "/var/lib/cgm") || hasMountPoint(info, "/var/lib/cgm/subdir") || hasMountPoint(info, "/media") {
		t.Fatal("mount matcher accepted a missing or nested mount")
	}
}
