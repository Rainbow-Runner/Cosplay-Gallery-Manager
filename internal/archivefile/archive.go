// Package archivefile exposes one sequential, read-only view over the archive
// formats accepted as Gallery sources. It never extracts into a media library.
package archivefile

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bodgit/sevenzip"
)

type Format string

const (
	FormatZIP      Format = "ZIP"
	FormatTAR      Format = "TAR"
	FormatTARGZIP  Format = "TAR_GZIP"
	FormatSevenZIP Format = "SEVEN_ZIP"
)

type Entry struct {
	Name             string
	Mode             fs.FileMode
	UncompressedSize uint64
	// CompressedSize is zero when a format does not expose a meaningful
	// per-member compressed size (solid 7z and tar streams).
	CompressedSize uint64
	Encrypted      bool
	OpenReader     func() (io.ReadCloser, error)
}

func (entry Entry) IsDir() bool { return entry.Mode.IsDir() }

func (entry Entry) Open() (io.ReadCloser, error) {
	if entry.OpenReader == nil || entry.IsDir() {
		return nil, errors.New("archive entry is not a readable regular file")
	}
	return entry.OpenReader()
}

func Detect(filename string) (Format, bool) {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return FormatTARGZIP, true
	case strings.HasSuffix(lower, ".tar"):
		return FormatTAR, true
	case strings.HasSuffix(lower, ".zip"), strings.HasSuffix(lower, ".cbz"):
		return FormatZIP, true
	case strings.HasSuffix(lower, ".7z"):
		return FormatSevenZIP, true
	default:
		return "", false
	}
}

func IsSupportedPath(filename string) bool {
	_, ok := Detect(filename)
	return ok
}

func BaseName(filename string) string {
	name := filepath.Base(filepath.Clean(filename))
	lower := strings.ToLower(name)
	for _, suffix := range []string{".tar.gz", ".tgz", ".tar", ".zip", ".cbz", ".7z"} {
		if strings.HasSuffix(lower, suffix) {
			return name[:len(name)-len(suffix)]
		}
	}
	return strings.TrimSuffix(name, filepath.Ext(name))
}

func IsArchiveLikePath(filename string) bool {
	if IsSupportedPath(filename) {
		return true
	}
	lower := strings.ToLower(filename)
	for _, suffix := range []string{".rar", ".cbr", ".zipx", ".tar.bz2", ".tbz", ".tbz2", ".tar.xz", ".txz", ".tar.zst", ".tzst", ".gz", ".bz2", ".xz", ".zst", ".lz", ".lzma", ".cab"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

// Walk opens an archive once and visits entries in storage order. Entry.Open
// must be consumed and closed before the callback returns; this preserves the
// solid-stream optimisation used by 7z and the forward-only TAR contract.
func Walk(filename string, visit func(Entry) error) error {
	format, ok := Detect(filename)
	if !ok {
		return fmt.Errorf("unsupported archive extension %q", filepath.Ext(filename))
	}
	info, err := os.Lstat(filename)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("archive source must be a regular non-symlink file")
	}
	switch format {
	case FormatZIP:
		return walkZIP(filename, visit)
	case FormatTAR, FormatTARGZIP:
		return walkTAR(filename, format == FormatTARGZIP, visit)
	case FormatSevenZIP:
		return walkSevenZIP(filename, visit)
	default:
		return errors.New("unsupported archive format")
	}
}

func walkZIP(filename string, visit func(Entry) error) error {
	reader, err := zip.OpenReader(filename)
	if err != nil {
		return fmt.Errorf("opening ZIP/CBZ: %w", err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		current := file
		if err := visit(Entry{Name: current.Name, Mode: current.Mode(),
			UncompressedSize: current.UncompressedSize64, CompressedSize: current.CompressedSize64,
			Encrypted: current.Flags&0x1 != 0, OpenReader: current.Open}); err != nil {
			return err
		}
	}
	return nil
}

func walkTAR(filename string, compressed bool, visit func(Entry) error) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	var input io.Reader = file
	var compressedReader *gzip.Reader
	if compressed {
		compressedReader, err = gzip.NewReader(file)
		if err != nil {
			return fmt.Errorf("opening gzip-compressed TAR: %w", err)
		}
		defer compressedReader.Close()
		input = compressedReader
	}
	reader := tar.NewReader(input)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading TAR header: %w", err)
		}
		mode := header.FileInfo().Mode()
		size := uint64(0)
		if header.Size > 0 {
			size = uint64(header.Size)
		}
		entry := Entry{Name: header.Name, Mode: mode, UncompressedSize: size}
		if mode.IsRegular() {
			entry.OpenReader = func() (io.ReadCloser, error) {
				return io.NopCloser(io.LimitReader(reader, header.Size)), nil
			}
		}
		if err := visit(entry); err != nil {
			return err
		}
	}
}

func walkSevenZIP(filename string, visit func(Entry) error) error {
	reader, err := sevenzip.OpenReader(filename)
	if err != nil {
		return fmt.Errorf("opening 7z: %w", err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		current := file
		if err := visit(Entry{Name: current.Name, Mode: current.Mode(),
			UncompressedSize: current.UncompressedSize, OpenReader: current.Open}); err != nil {
			return err
		}
	}
	return nil
}

func IsEncryptedError(err error) bool {
	var readError *sevenzip.ReadError
	return errors.As(err, &readError) && readError.Encrypted
}
