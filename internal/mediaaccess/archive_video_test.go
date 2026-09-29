package mediaaccess

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/archivefile"
)

func TestArchiveBackgroundInputDirectFirstAndTemporaryFallback(t *testing.T) {
	for _, method := range []uint16{zip.Store, zip.Deflate} {
		t.Run(map[uint16]string{zip.Store: "direct", zip.Deflate: "extracted"}[method], func(t *testing.T) {
			source := writeDirectArchive(t, ".zip", method)
			root := t.TempDir()
			info, _ := os.Stat(source.Path)
			input, err := (Materializer{TemporaryRoot: root}).OpenArchiveVideo(context.Background(), source, archivefile.DefaultDirectLimits(), &ArchiveEvidence{info.Size(), info.ModTime()})
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			var body []byte
			if method == zip.Store {
				if !strings.HasPrefix(input.Path, "http://127.0.0.1:") {
					t.Fatalf("not a private direct input: %s", input.Path)
				}
				client := &http.Client{Timeout: time.Second}
				request, _ := http.NewRequest(http.MethodGet, input.Path, nil)
				request.Header.Set("Range", "bytes=3-5")
				response, err := client.Do(request)
				if err != nil {
					t.Fatal(err)
				}
				body, err = io.ReadAll(response.Body)
				response.Body.Close()
				if err != nil || response.StatusCode != 206 || string(body) != "345" {
					t.Fatalf("direct range: %q %v", body, err)
				}
				response, err = client.Get(input.Path + "/other")
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				if response.StatusCode != 404 {
					t.Fatal("processing token boundary not enforced")
				}
			} else {
				body, err = os.ReadFile(input.Path)
				if err != nil || string(body) != "0123456789" {
					t.Fatalf("temporary payload: %q %v", body, err)
				}
			}
			entries, _ := os.ReadDir(root)
			if (method == zip.Store && len(entries) != 0) || (method == zip.Deflate && len(entries) != 1) {
				t.Fatalf("temporary entries: %v", entries)
			}
			if err := input.Validate(); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(source.Path, time.Now(), info.ModTime().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			if !errors.Is(input.Validate(), ErrArchiveSourceChanged) {
				t.Fatal("changed source accepted after processing")
			}
			if err := input.Close(); err != nil {
				t.Fatal(err)
			}
			entries, _ = os.ReadDir(root)
			if len(entries) != 0 {
				t.Fatalf("temporary source retained: %v", entries)
			}
		})
	}
}

func TestArchiveBackgroundInputCancellationClosesPrivateEndpoint(t *testing.T) {
	source := writeDirectArchive(t, ".tar", zip.Store)
	ctx, cancel := context.WithCancel(context.Background())
	input, err := (Materializer{TemporaryRoot: t.TempDir()}).OpenArchiveVideo(ctx, source, archivefile.DefaultDirectLimits(), nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	defer input.Close()
	cancel()
	if input.Validate() == nil {
		t.Fatal("cancelled input accepted")
	}
}

func TestArchiveVideoStartupCleanupProtectsUnknownFilesLinksAndDirectories(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"cgm-video-12345.mp4", "cgm-video-23456.MOV", "cgm-media-123.mp4", "original.mp4"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "cgm-video-34567.mp4"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "original.mp4"), filepath.Join(root, "cgm-video-45678.mp4")); err != nil {
		t.Fatal(err)
	}
	count, err := CleanupArchiveVideoTemporary(root)
	if err != nil || count != 2 {
		t.Fatalf("startup cleanup %d %v", count, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 4 {
		t.Fatalf("protected entries %v %v", entries, err)
	}
}
