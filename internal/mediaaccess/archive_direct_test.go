package mediaaccess

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stashapp/stash/internal/archivefile"
	"github.com/stashapp/stash/internal/gallery"
)

func TestArchiveMemberRandomReadsStayInsideMemberAndCreateNoExtractedFile(t *testing.T) {
	for _, extension := range []string{".zip", ".cbz", ".tar"} {
		t.Run(extension, func(t *testing.T) {
			source := writeDirectArchive(t, extension, zip.Store)
			info, err := os.Stat(source.Path)
			if err != nil {
				t.Fatal(err)
			}
			member, err := OpenArchiveMember(context.Background(), source, archivefile.DefaultDirectLimits(), &ArchiveEvidence{info.Size(), info.ModTime()})
			if err != nil {
				t.Fatal(err)
			}
			defer member.Close()
			if member.Size() != 10 || !member.ModTime().Equal(info.ModTime()) {
				t.Fatal("incorrect member metadata")
			}
			got := make([]byte, 3)
			if n, err := member.ReadAt(got, 3); err != nil || n != 3 || string(got) != "345" {
				t.Fatalf("ReadAt: %q %d %v", got, n, err)
			}
			if _, err := member.Seek(-2, io.SeekEnd); err != nil {
				t.Fatal(err)
			}
			last, err := io.ReadAll(member)
			if err != nil || string(last) != "89" {
				t.Fatalf("suffix: %q %v", last, err)
			}
			if n, err := member.ReadAt(make([]byte, 8), 8); n != 2 || !errors.Is(err, io.EOF) {
				t.Fatalf("bounded ReadAt: %d %v", n, err)
			}
			for _, seek := range [][2]int64{{-1, io.SeekStart}, {11, io.SeekStart}, {1, io.SeekEnd}, {math.MaxInt64, io.SeekCurrent}, {math.MinInt64, io.SeekEnd}, {0, 99}} {
				if _, err := member.Seek(seek[0], int(seek[1])); !errors.Is(err, ErrArchiveMemberRange) {
					t.Fatalf("seek %v: %v", seek, err)
				}
			}
			if n, err := member.ReadAt(make([]byte, 1), -1); n != 0 || err == nil {
				t.Fatalf("negative ReadAt: %d %v", n, err)
			}
			entries, err := os.ReadDir(filepath.Dir(source.Path))
			if err != nil || len(entries) != 1 {
				t.Fatalf("unexpected extracted files: %v %v", entries, err)
			}
			if err := member.Close(); err != nil {
				t.Fatal(err)
			}
			if err := member.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := member.Seek(0, io.SeekStart); !errors.Is(err, ErrArchiveMemberClosed) {
				t.Fatalf("closed seek: %v", err)
			}
			if _, err := member.Read(make([]byte, 1)); !errors.Is(err, ErrArchiveMemberClosed) {
				t.Fatalf("closed read at EOF: %v", err)
			}
			if _, err := member.ReadAt(make([]byte, 1), member.Size()); !errors.Is(err, ErrArchiveMemberClosed) {
				t.Fatalf("closed ReadAt at EOF: %v", err)
			}
		})
	}
}

func TestArchiveMemberSupportsHTTPRangeWithoutNeighbourBytes(t *testing.T) {
	source := writeDirectArchive(t, ".zip", zip.Store)
	for _, check := range []struct {
		method, rangeHeader string
		status              int
		body                string
	}{
		{http.MethodGet, "", 200, "0123456789"}, {http.MethodHead, "", 200, ""},
		{http.MethodGet, "bytes=2-5", 206, "2345"}, {http.MethodGet, "bytes=-3", 206, "789"},
		{http.MethodGet, "bytes=8-99", 206, "89"}, {http.MethodGet, "bytes=10-20", 416, "invalid range: failed to overlap\n"},
	} {
		request := httptest.NewRequest(check.method, "/internal-test", nil)
		if check.rangeHeader != "" {
			request.Header.Set("Range", check.rangeHeader)
		}
		response := httptest.NewRecorder()
		member, err := OpenArchiveMember(request.Context(), source, archivefile.DefaultDirectLimits(), nil)
		if err != nil {
			t.Fatal(err)
		}
		http.ServeContent(response, request, "", member.ModTime(), member)
		member.Close()
		if response.Code != check.status || response.Body.String() != check.body {
			t.Fatalf("%s %s: %d %q", check.method, check.rangeHeader, response.Code, response.Body.String())
		}
	}
}

func TestArchiveMemberRejectsSourceReplacementMutationAndStaleEvidence(t *testing.T) {
	for _, action := range []string{"replacement", "resize", "mtime", "stale-size", "stale-mtime", "cancel"} {
		t.Run(action, func(t *testing.T) {
			source := writeDirectArchive(t, ".tar", zip.Store)
			info, err := os.Stat(source.Path)
			if err != nil {
				t.Fatal(err)
			}
			expected := &ArchiveEvidence{info.Size(), info.ModTime()}
			if action == "stale-size" {
				expected.Size++
			}
			if action == "stale-mtime" {
				expected.Modified = expected.Modified.Add(time.Second)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			member, err := OpenArchiveMember(ctx, source, archivefile.DefaultDirectLimits(), expected)
			if strings.HasPrefix(action, "stale-") {
				if !errors.Is(err, ErrArchiveSourceChanged) {
					t.Fatalf("stale source: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer member.Close()
			want := ErrArchiveSourceChanged
			switch action {
			case "replacement":
				if err := os.Rename(source.Path, source.Path+".old"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(source.Path, []byte("not the archive"), 0600); err != nil {
					t.Fatal(err)
				}
			case "resize":
				if err := os.Truncate(source.Path, info.Size()-1); err != nil {
					t.Fatal(err)
				}
			case "mtime":
				if err := os.Chtimes(source.Path, time.Now(), info.ModTime().Add(time.Second)); err != nil {
					t.Fatal(err)
				}
			case "cancel":
				cancel()
				want = context.Canceled
			}
			if n, err := member.ReadAt(make([]byte, 3), 0); n != 0 || !errors.Is(err, want) {
				t.Fatalf("changed source read: %d %v", n, err)
			}
		})
	}
}

func TestArchiveMemberRejectsSourceSymlinksAndInvalidContexts(t *testing.T) {
	source := writeDirectArchive(t, ".zip", zip.Store)
	link := filepath.Join(t.TempDir(), "linked.zip")
	if err := os.Symlink(source.Path, link); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(t.TempDir(), "parent")
	if err := os.Symlink(filepath.Dir(source.Path), parent); err != nil {
		t.Fatal(err)
	}
	for _, filename := range []string{link, filepath.Join(parent, filepath.Base(source.Path)), filepath.Dir(source.Path), "relative.zip"} {
		copy := source
		copy.Path = filename
		if _, err := OpenArchiveMember(context.Background(), copy, archivefile.DefaultDirectLimits(), nil); !errors.Is(err, ErrArchiveSourceUnsafe) && !errors.Is(err, archivefile.ErrDirectLayout) {
			t.Fatalf("unsafe source %s: %v", filename, err)
		}
	}
	for _, relative := range []string{"../clip.mp4", "/clip.mp4", "a\\b", ""} {
		copy := source
		copy.RelativePath = relative
		if _, err := OpenArchiveMember(context.Background(), copy, archivefile.DefaultDirectLimits(), nil); !errors.Is(err, ErrArchiveSourceUnsafe) {
			t.Fatalf("relative path %q: %v", relative, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := OpenArchiveMember(ctx, source, archivefile.DefaultDirectLimits(), nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled open: %v", err)
	}
	source.Type = gallery.SourceTypeDirectory
	if _, err := OpenArchiveMember(context.Background(), source, archivefile.DefaultDirectLimits(), nil); !errors.Is(err, ErrArchiveSourceUnsafe) {
		t.Fatalf("directory source: %v", err)
	}
}

func TestCompressedArchiveMemberNeverCreatesPlaybackExtraction(t *testing.T) {
	source := writeDirectArchive(t, ".zip", zip.Deflate)
	if _, err := OpenArchiveMember(context.Background(), source, archivefile.DefaultDirectLimits(), nil); !errors.Is(err, archivefile.ErrDirectCompressed) {
		t.Fatalf("compressed open: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(source.Path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("playback extraction created: %v %v", entries, err)
	}
}

func TestRealSevenZIPStoreDirectMember(t *testing.T) {
	if os.Getenv("CGM_REAL_ARCHIVE_MEDIA") != "1" {
		t.Skip("opt-in real 7z gate")
	}
	command, err := exec.LookPath("7z")
	if err != nil {
		t.Fatal("real 7z gate requires 7z")
	}
	root := t.TempDir()
	for _, name := range []string{"clip.mp4", "second.jpg"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("0123456789"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, option := range []string{"-mhc=off", "-mhc=on"} {
		filename := filepath.Join(root, strings.TrimPrefix(option, "-mhc=")+".7z")
		cmd := exec.Command(command, "a", "-t7z", "-mx=0", option, filename, "clip.mp4", "second.jpg")
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("7z fixture generation: %v %s", err, output)
		}
		member, err := OpenArchiveMember(context.Background(), Source{Type: gallery.SourceTypeArchive, Path: filename, RelativePath: "clip.mp4"}, archivefile.DefaultDirectLimits(), nil)
		if option == "-mhc=on" && errors.Is(err, archivefile.ErrDirectLayout) {
			continue
		} // Metadata encoding is not guessed.
		if err != nil {
			t.Fatalf("real Store %s: %v", option, err)
		}
		got, err := io.ReadAll(member)
		member.Close()
		if err != nil || string(got) != "0123456789" {
			t.Fatalf("real Store bytes: %q %v", got, err)
		}
	}
}

func writeDirectArchive(t *testing.T, extension string, method uint16) Source {
	t.Helper()
	var buffer bytes.Buffer
	names := []string{"before.jpg", "folder/clip.mp4", "after.jpg"}
	payload := func(name string) []byte {
		if name == "folder/clip.mp4" {
			return []byte("0123456789")
		}
		return []byte("NEIGHBOUR!")
	}
	if extension == ".tar" {
		w := tar.NewWriter(&buffer)
		for _, name := range names {
			if err := w.WriteHeader(&tar.Header{Name: name, Size: 10, Mode: 0600}); err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write(payload(name)); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		w := zip.NewWriter(&buffer)
		for _, name := range names {
			part, err := w.CreateHeader(&zip.FileHeader{Name: name, Method: method})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write(payload(name)); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}
	filename := filepath.Join(t.TempDir(), "gallery"+extension)
	if err := os.WriteFile(filename, buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return Source{Type: gallery.SourceTypeArchive, Path: filename, RelativePath: "folder/clip.mp4"}
}
