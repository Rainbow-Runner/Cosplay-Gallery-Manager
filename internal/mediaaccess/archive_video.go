package mediaaccess

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/stashapp/stash/internal/archivefile"
	"github.com/stashapp/stash/internal/gallery"
)

// OpenArchiveVideo is for background probe/poster/date work ONLY. Playback
// must use OpenArchiveMember, never this extraction-capable method.
func (m Materializer) OpenArchiveVideo(ctx context.Context, source Source, limits archivefile.DirectLimits, expected *ArchiveEvidence) (Materialized, error) {
	member, err := OpenArchiveMember(ctx, source, limits, expected)
	if err == nil {
		input, err := serveProcessingMember(ctx, member)
		if err != nil {
			member.Close()
		}
		return input, err
	}
	if !errors.Is(err, archivefile.ErrDirectCompressed) && !errors.Is(err, archivefile.ErrDirectLayout) {
		return Materialized{}, err
	}
	if source.Type != gallery.SourceTypeArchive || validateRelative(source.RelativePath) != nil {
		return Materialized{}, ErrArchiveSourceUnsafe
	}
	file, info, err := openArchiveDescriptor(ctx, source.Path)
	if err != nil {
		return Materialized{}, err
	}
	success := false
	defer func() {
		if !success {
			_ = file.Close()
		}
	}()
	input := &observedArchive{ctx: ctx, file: file, path: source.Path, info: info}
	if expected != nil && (expected.Size != info.Size() || !expected.Modified.Equal(info.ModTime())) {
		return Materialized{}, ErrArchiveSourceChanged
	}
	root, err := os.Lstat(m.TemporaryRoot)
	if err != nil || !root.IsDir() || root.Mode()&os.ModeSymlink != 0 {
		return Materialized{}, ErrArchiveSourceUnsafe
	}
	format, _ := archivefile.Detect(source.Path)
	maximum := limits.MaxMemberBytes
	if m.MaximumBytes > 0 && uint64(m.MaximumBytes) < maximum {
		maximum = uint64(m.MaximumBytes)
	}
	var filename string
	defer func() {
		if !success && filename != "" {
			_ = os.Remove(filename)
		}
	}()
	seen, count, total := map[string]bool{}, 0, uint64(0)
	matched := false
	err = archivefile.WalkReaderAt(format, input, info.Size(), func(entry archivefile.Entry) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := entry.Name
		if entry.IsDir() {
			name = strings.TrimSuffix(name, "/")
		}
		if validateRelative(name) != nil || strings.ContainsRune(name, '\x00') || (len(name) >= 2 && name[1] == ':') || (!entry.IsDir() && !entry.Mode.IsRegular()) || entry.Encrypted || archivefile.IsArchiveLikePath(name) || seen[strings.ToLower(name)] {
			return archivefile.ErrDirectUnsafe
		}
		seen[strings.ToLower(name)] = true
		count++
		if count > limits.MaxEntries || entry.UncompressedSize > maximum || total > limits.MaxTotalBytes || entry.UncompressedSize > limits.MaxTotalBytes-total {
			return archivefile.ErrDirectLimit
		}
		total += entry.UncompressedSize
		if name != source.RelativePath || entry.IsDir() {
			return nil
		}
		matched = true
		part, err := entry.Open()
		if err != nil {
			return err
		}
		defer part.Close()
		output, err := os.CreateTemp(m.TemporaryRoot, "cgm-video-*"+filepath.Ext(name))
		if err != nil {
			return err
		}
		filename = output.Name()
		defer output.Close()
		// +1 detects streams that lie about their advertised size.
		written, err := io.Copy(output, io.LimitReader(videoContextReader{ctx: ctx, input: part}, int64(maximum)+1))
		if err != nil {
			return err
		}
		if uint64(written) != entry.UncompressedSize || uint64(written) > maximum {
			return archivefile.ErrDirectInvalid
		}
		return output.Sync()
	})
	if err != nil {
		return Materialized{}, err
	}
	if !matched {
		return Materialized{}, archivefile.ErrDirectMemberMissing
	}
	if err := input.check(); err != nil {
		return Materialized{}, err
	}
	// Retain the descriptor until the worker publishes (or discards) results.
	success = true
	var once sync.Once
	return Materialized{Path: filename, validate: input.check, cleanup: func() error {
		var err error
		once.Do(func() { _ = file.Close(); err = os.Remove(filename) })
		return err
	}}, nil
}

type videoContextReader struct {
	ctx   context.Context
	input io.Reader
}

func (r videoContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.input.Read(p)
}

func serveProcessingMember(ctx context.Context, member *DirectArchiveMember) (Materialized, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Materialized{}, err
	}
	var token [24]byte
	if _, err := rand.Read(token[:]); err != nil {
		listener.Close()
		return Materialized{}, err
	}
	endpoint := "/" + hex.EncodeToString(token[:])
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, MaxHeaderBytes: 8192, BaseContext: func(net.Listener) context.Context { return ctx }}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != endpoint || r.URL.RawQuery != "" || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Cache-Control", "no-store")
		if err := member.input.check(); err != nil {
			http.Error(w, "source unavailable", http.StatusConflict)
			return
		}
		http.ServeContent(w, r, "", member.ModTime(), io.NewSectionReader(member, 0, member.Size()))
	})
	go func() { _ = server.Serve(listener) }()
	var once sync.Once
	closeInput := func() error { once.Do(func() { _ = server.Close(); _ = member.Close() }); return nil }
	stop := context.AfterFunc(ctx, func() { _ = closeInput() })
	return Materialized{Path: "http://" + listener.Addr().String() + endpoint, validate: member.input.check, cleanup: func() error { stop(); return closeInput() }}, nil
}
