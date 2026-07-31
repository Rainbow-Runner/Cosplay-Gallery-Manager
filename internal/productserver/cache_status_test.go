package productserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/persistence/productdb"
)

func TestCacheStorageStatusCountsRegularFilesWithoutFollowingSymlinks(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "items", "ab")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "card-480.jpg"), []byte("1234"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "temporary"), []byte("123456"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("must not count"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}

	database, err := productdb.Open(context.Background(), filepath.Join(t.TempDir(), "product.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	server := &Server{Config: Config{CachePath: root}, Database: database}
	status, err := server.CacheStorageStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Path != root || status.ByteSize != 10 || status.FileCount != 2 || status.BaseByteSize != 0 || status.EnhancedByteSize != 0 {
		t.Fatalf("cache status = %#v", status)
	}
}
