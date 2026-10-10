package mediaaccess

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/stashapp/stash/internal/archivefile"
	"github.com/stashapp/stash/internal/gallery"
	"golang.org/x/sys/unix"
)

var (
	ErrArchiveSourceUnsafe  = errors.New("archive source is not a safe regular file")
	ErrArchiveSourceChanged = errors.New("archive source changed")
	ErrArchiveMemberRange   = errors.New("archive member seek is outside its bounds")
	ErrArchiveMemberClosed  = errors.New("archive member is closed")
)

// ArchiveEvidence is an optional prior scan observation. Passing it ensures
// direct access cannot silently serve a known different container revision.
// It is not a content hash: same-size/same-mtime replacements still need the
// existing explicit deep scan. None of this evidence is portable metadata.
type ArchiveEvidence struct {
	Size     int64
	Modified time.Time
}

// DirectArchiveMember is a bounded, seekable view of the original descriptor.
// It creates no extracted file. Paths and offsets remain private to this layer.
type DirectArchiveMember struct {
	section  *io.SectionReader
	input    *observedArchive
	size     int64
	once     sync.Once
	closeErr error
}

func OpenArchiveMember(ctx context.Context, source Source, limits archivefile.DirectLimits, expected *ArchiveEvidence) (*DirectArchiveMember, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if source.Type != gallery.SourceTypeArchive || validateRelative(source.RelativePath) != nil {
		return nil, ErrArchiveSourceUnsafe
	}
	format, ok := archivefile.Detect(source.Path)
	if !ok {
		return nil, archivefile.ErrDirectLayout
	}
	file, info, err := openArchiveDescriptor(ctx, source.Path)
	if err != nil {
		return nil, err
	}
	input := &observedArchive{ctx: ctx, file: file, path: source.Path, info: info}
	success := false
	defer func() {
		if !success {
			_ = file.Close()
		}
	}()
	if expected != nil && (info.Size() != expected.Size || !info.ModTime().Equal(expected.Modified)) {
		return nil, ErrArchiveSourceChanged
	}
	member, err := archivefile.LocateDirectMember(ctx, format, input, info.Size(), source.RelativePath, limits)
	if err != nil {
		return nil, err
	}
	if err := input.check(); err != nil {
		return nil, err
	}
	success = true
	return &DirectArchiveMember{section: io.NewSectionReader(input, member.Offset, member.Size), input: input, size: member.Size}, nil
}

func (m *DirectArchiveMember) Size() int64        { return m.size }
func (m *DirectArchiveMember) ModTime() time.Time { return m.input.info.ModTime() }
func (m *DirectArchiveMember) Read(p []byte) (int, error) {
	if err := m.input.check(); err != nil {
		return 0, err
	}
	return m.section.Read(p)
}
func (m *DirectArchiveMember) ReadAt(p []byte, offset int64) (int, error) {
	if err := m.input.check(); err != nil {
		return 0, err
	}
	return m.section.ReadAt(p, offset)
}
func (m *DirectArchiveMember) Seek(offset int64, whence int) (int64, error) {
	if err := m.input.check(); err != nil {
		return 0, err
	}
	base := int64(0)
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		base, _ = m.section.Seek(0, io.SeekCurrent)
	case io.SeekEnd:
		base = m.size
	default:
		return 0, ErrArchiveMemberRange
	}
	if offset < -base || offset > m.size-base {
		return 0, ErrArchiveMemberRange
	}
	return m.section.Seek(base+offset, io.SeekStart)
}
func (m *DirectArchiveMember) Close() error {
	m.once.Do(func() { m.input.closed.Store(true); m.closeErr = m.input.file.Close() })
	return m.closeErr
}

type observedArchive struct {
	ctx    context.Context
	file   *os.File
	path   string
	info   os.FileInfo
	closed atomic.Bool
}

func (r *observedArchive) check() error {
	if r.closed.Load() {
		return ErrArchiveMemberClosed
	}
	if err := r.ctx.Err(); err != nil {
		return err
	}
	current, err := r.file.Stat()
	if err != nil {
		return ErrArchiveSourceChanged
	}
	// Lstat detects path replacement, while all byte reads keep using the
	// original descriptor. No file is reopened through a changed path.
	pathInfo, err := os.Lstat(r.path)
	if err != nil || !pathInfo.Mode().IsRegular() || !os.SameFile(r.info, pathInfo) ||
		!os.SameFile(r.info, current) || current.Size() != r.info.Size() || !current.ModTime().Equal(r.info.ModTime()) {
		return ErrArchiveSourceChanged
	}
	return nil
}
func (r *observedArchive) ReadAt(p []byte, offset int64) (int, error) {
	if err := r.check(); err != nil {
		return 0, err
	}
	n, err := r.file.ReadAt(p, offset)
	if checkErr := r.check(); checkErr != nil {
		return 0, checkErr
	}
	return n, err
}

// Open all path components with no-follow and retain each parent's descriptor
// until its child is open. Nonblocking prevents a raced FIFO from hanging setup.
func openArchiveDescriptor(ctx context.Context, value string) (*os.File, os.FileInfo, error) {
	if !filepath.IsAbs(value) || filepath.Clean(value) != value {
		return nil, nil, ErrArchiveSourceUnsafe
	}
	parts := strings.Split(strings.TrimPrefix(value, string(filepath.Separator)), string(filepath.Separator))
	fd, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, ErrArchiveSourceUnsafe
	}
	for i, part := range parts {
		if err := ctx.Err(); err != nil {
			unix.Close(fd)
			return nil, nil, err
		}
		flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK
		if i < len(parts)-1 {
			flags |= unix.O_DIRECTORY
		}
		next, err := unix.Openat(fd, part, flags, 0)
		unix.Close(fd)
		if err != nil {
			return nil, nil, ErrArchiveSourceUnsafe
		}
		fd = next
	}
	file := os.NewFile(uintptr(fd), value)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, nil, ErrArchiveSourceUnsafe
	}
	return file, info, nil
}
