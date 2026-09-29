package archivefile

import (
	"archive/tar"
	"archive/zip"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"math"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

var (
	ErrDirectCompressed    = errors.New("archive member requires decompression")
	ErrDirectLayout        = errors.New("archive layout cannot be proven directly readable")
	ErrDirectUnsafe        = errors.New("unsafe archive structure")
	ErrDirectInvalid       = errors.New("invalid archive structure")
	ErrDirectLimit         = errors.New("archive direct access limit exceeded")
	ErrDirectMemberMissing = errors.New("archive member not found")
)

// DirectLimits bounds metadata work independently of member size. Locating a
// member never decompresses or hashes its payload. These are internal values,
// not physical offsets or capabilities exposed to Browse.
type DirectLimits struct {
	MaxEntries     int
	MaxHeaderBytes int64
	MaxMemberBytes uint64
	MaxTotalBytes  uint64
}

func DefaultDirectLimits() DirectLimits {
	return DirectLimits{20_000, 16 << 20, 2 << 30, 100 << 30}
}

type DirectMember struct{ Offset, Size int64 }

// LocateDirectMember proves a contiguous member slice on the caller's already
// open descriptor. The caller must authorize first and retain that descriptor.
func LocateDirectMember(ctx context.Context, format Format, input io.ReaderAt, size int64, name string, limits DirectLimits) (DirectMember, error) {
	if err := ctx.Err(); err != nil {
		return DirectMember{}, err
	}
	if size < 0 || !safeDirectName(name) {
		return DirectMember{}, ErrDirectUnsafe
	}
	if limits.MaxEntries <= 0 || limits.MaxEntries > 100_000 || limits.MaxHeaderBytes < 1024 || limits.MaxHeaderBytes > 64<<20 || limits.MaxMemberBytes == 0 || limits.MaxMemberBytes > math.MaxInt64 || limits.MaxTotalBytes == 0 {
		return DirectMember{}, ErrDirectLimit
	}
	bounded := &metadataReader{ctx: ctx, input: input, remaining: limits.MaxHeaderBytes*3 + 1<<20}
	switch format {
	case FormatZIP:
		return locateZIP(bounded, size, name, limits)
	case FormatTAR:
		return locateTAR(bounded, size, name, limits)
	case FormatSevenZIP:
		return locateSevenZIP(bounded, size, name, limits)
	case FormatTARGZIP:
		return DirectMember{}, ErrDirectCompressed
	default:
		return DirectMember{}, ErrDirectLayout
	}
}

type metadataReader struct {
	ctx       context.Context
	input     io.ReaderAt
	remaining int64
}

func (r *metadataReader) ReadAt(p []byte, offset int64) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		return 0, ErrDirectLimit
	}
	r.remaining -= int64(len(p))
	return r.input.ReadAt(p, offset)
}

func safeDirectName(value string) bool {
	return value != "" && utf8.ValidString(value) && norm.NFC.String(value) == value &&
		!strings.ContainsAny(value, "\\\x00") && !strings.HasPrefix(value, "/") &&
		!(len(value) >= 2 && value[1] == ':') && path.Clean(value) == value &&
		value != "." && value != ".." && !strings.HasPrefix(value, "../")
}

type directValidator struct {
	limits DirectLimits
	seen   map[string]bool
	count  int
	total  uint64
}

func (v *directValidator) entry(name string, mode fs.FileMode, size uint64) error {
	if mode.IsDir() {
		name = strings.TrimSuffix(name, "/")
	}
	if !safeDirectName(name) || (!mode.IsDir() && !mode.IsRegular()) || IsArchiveLikePath(name) {
		return ErrDirectUnsafe
	}
	key := strings.ToLower(name)
	if v.seen[key] {
		return ErrDirectUnsafe
	}
	v.seen[key] = true
	v.count++
	if v.count > v.limits.MaxEntries || size > v.limits.MaxMemberBytes || size > v.limits.MaxTotalBytes-v.total {
		return ErrDirectLimit
	}
	v.total += size
	return nil
}

func inside(offset, length, size int64) bool {
	return offset >= 0 && length >= 0 && offset <= size && length <= size-offset
}

// Preflight the central directory before archive/zip allocates using the
// untrusted record count (including ZIP64 counts). SFX/multi-volume layouts are
// deliberately not accepted by this direct-access capability.
func zipDirectory(r io.ReaderAt, size int64, limits DirectLimits) (int64, error) {
	if size < 22 {
		return 0, ErrDirectInvalid
	}
	tail := make([]byte, min(size, 65535+22))
	if _, err := r.ReadAt(tail, size-int64(len(tail))); err != nil {
		return 0, err
	}
	i := len(tail) - 22
	for ; i >= 0; i-- {
		if binary.LittleEndian.Uint32(tail[i:]) == 0x06054b50 && i+22+int(binary.LittleEndian.Uint16(tail[i+20:])) == len(tail) {
			break
		}
	}
	if i < 0 {
		return 0, ErrDirectInvalid
	}
	e := tail[i:]
	if binary.LittleEndian.Uint16(e[4:]) != 0 || binary.LittleEndian.Uint16(e[6:]) != 0 {
		return 0, ErrDirectLayout
	}
	count := uint64(binary.LittleEndian.Uint16(e[10:]))
	length, offset := uint64(binary.LittleEndian.Uint32(e[12:])), uint64(binary.LittleEndian.Uint32(e[16:]))
	eocdOffset := size - int64(len(tail)) + int64(i)
	boundary := eocdOffset
	if count == 0xffff || length == 0xffffffff || offset == 0xffffffff {
		var locator [20]byte
		if !inside(eocdOffset-20, 20, size) {
			return 0, ErrDirectInvalid
		}
		if _, err := r.ReadAt(locator[:], eocdOffset-20); err != nil {
			return 0, err
		}
		if binary.LittleEndian.Uint32(locator[:]) != 0x07064b50 || binary.LittleEndian.Uint32(locator[4:]) != 0 || binary.LittleEndian.Uint32(locator[16:]) != 1 {
			return 0, ErrDirectLayout
		}
		start := binary.LittleEndian.Uint64(locator[8:])
		if start > uint64(eocdOffset-20) || eocdOffset-20-int64(start) < 56 {
			return 0, ErrDirectInvalid
		}
		var record [56]byte
		if _, err := r.ReadAt(record[:], int64(start)); err != nil {
			return 0, err
		}
		if binary.LittleEndian.Uint32(record[:]) != 0x06064b50 || binary.LittleEndian.Uint64(record[4:]) != uint64(eocdOffset-20-int64(start)-12) || binary.LittleEndian.Uint32(record[16:]) != 0 || binary.LittleEndian.Uint32(record[20:]) != 0 {
			return 0, ErrDirectLayout
		}
		count, length, offset = binary.LittleEndian.Uint64(record[32:]), binary.LittleEndian.Uint64(record[40:]), binary.LittleEndian.Uint64(record[48:])
		if binary.LittleEndian.Uint64(record[24:]) != count {
			return 0, ErrDirectInvalid
		}
		boundary = int64(start)
	} else if uint64(binary.LittleEndian.Uint16(e[8:])) != count {
		return 0, ErrDirectLayout
	}
	if count > uint64(limits.MaxEntries) || length > uint64(limits.MaxHeaderBytes) {
		return 0, ErrDirectLimit
	}
	if offset > uint64(boundary) || length != uint64(boundary)-offset {
		return 0, ErrDirectLayout
	}
	// Count real headers too: a forged small EOCD count must not evade bounds.
	position := int64(offset)
	for n := uint64(0); position < boundary; n++ {
		if n >= count || !inside(position, 46, boundary) {
			return 0, ErrDirectInvalid
		}
		var header [46]byte
		if _, err := r.ReadAt(header[:], position); err != nil {
			return 0, err
		}
		if binary.LittleEndian.Uint32(header[:]) != 0x02014b50 || binary.LittleEndian.Uint16(header[34:]) != 0 {
			return 0, ErrDirectLayout
		}
		step := int64(46) + int64(binary.LittleEndian.Uint16(header[28:])) + int64(binary.LittleEndian.Uint16(header[30:])) + int64(binary.LittleEndian.Uint16(header[32:]))
		if !inside(position, step, boundary) {
			return 0, ErrDirectInvalid
		}
		position += step
		if position == boundary && n+1 != count {
			return 0, ErrDirectInvalid
		}
	}
	return int64(offset), nil
}

func locateZIP(r io.ReaderAt, size int64, name string, limits DirectLimits) (DirectMember, error) {
	central, err := zipDirectory(r, size, limits)
	if err != nil {
		return DirectMember{}, err
	}
	z, err := zip.NewReader(r, size)
	if err != nil {
		return DirectMember{}, err
	}
	v := directValidator{limits: limits, seen: map[string]bool{}}
	type span struct{ start, end int64 }
	var spans []span
	var result DirectMember
	found, compressed := false, false
	centralPosition := central
	for _, entry := range z.File {
		if err := v.entry(entry.Name, entry.Mode(), entry.UncompressedSize64); err != nil {
			return DirectMember{}, err
		}
		if entry.Flags&(1|0x40) != 0 {
			return DirectMember{}, ErrDirectUnsafe
		}
		offset, err := entry.DataOffset()
		if err != nil {
			return DirectMember{}, err
		}
		if entry.CompressedSize64 > uint64(central) || !inside(offset, int64(entry.CompressedSize64), central) {
			return DirectMember{}, ErrDirectInvalid
		}
		start, err := verifyZIPLocalHeader(r, centralPosition, central, offset, entry)
		if err != nil {
			return DirectMember{}, err
		}
		centralPosition += int64(46 + len(entry.Name) + len(entry.Extra) + len(entry.Comment))
		if entry.Name == name && !entry.FileInfo().IsDir() {
			found = true
			compressed = entry.Method != zip.Store
			if !compressed && entry.CompressedSize64 != entry.UncompressedSize64 {
				return DirectMember{}, ErrDirectInvalid
			}
			result = DirectMember{offset, int64(entry.UncompressedSize64)}
		}
		spans = append(spans, span{start, offset + int64(entry.CompressedSize64)})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	for i := 1; i < len(spans); i++ {
		if spans[i].start < spans[i-1].end {
			return DirectMember{}, ErrDirectUnsafe
		}
	}
	if !found {
		return DirectMember{}, ErrDirectMemberMissing
	}
	if compressed {
		return DirectMember{}, ErrDirectCompressed
	}
	return result, nil
}

func zipExtraField(extra []byte, id uint16) ([]byte, bool) {
	for len(extra) >= 4 {
		length := int(binary.LittleEndian.Uint16(extra[2:]))
		if length > len(extra)-4 {
			return nil, false
		}
		if binary.LittleEndian.Uint16(extra) == id {
			return extra[4 : 4+length], true
		}
		extra = extra[4+length:]
	}
	return nil, false
}

func verifyZIPLocalHeader(r io.ReaderAt, position, central, expectedOffset int64, entry *zip.File) (int64, error) {
	var header [46]byte
	if _, err := r.ReadAt(header[:], position); err != nil {
		return 0, err
	}
	start := uint64(binary.LittleEndian.Uint32(header[42:]))
	if start == 0xffffffff {
		extra, ok := zipExtraField(entry.Extra, 1)
		if binary.LittleEndian.Uint32(header[24:]) == 0xffffffff {
			if len(extra) < 8 {
				return 0, ErrDirectInvalid
			}
			extra = extra[8:]
		}
		if binary.LittleEndian.Uint32(header[20:]) == 0xffffffff {
			if len(extra) < 8 {
				return 0, ErrDirectInvalid
			}
			extra = extra[8:]
		}
		if !ok || len(extra) < 8 {
			return 0, ErrDirectInvalid
		}
		start = binary.LittleEndian.Uint64(extra)
	}
	if start > uint64(central) || !inside(int64(start), 30, central) {
		return 0, ErrDirectInvalid
	}
	var local [30]byte
	if _, err := r.ReadAt(local[:], int64(start)); err != nil {
		return 0, err
	}
	if binary.LittleEndian.Uint32(local[:]) != 0x04034b50 || binary.LittleEndian.Uint16(local[6:]) != entry.Flags || binary.LittleEndian.Uint16(local[8:]) != entry.Method {
		return 0, ErrDirectUnsafe
	}
	names, extras := int64(binary.LittleEndian.Uint16(local[26:])), int64(binary.LittleEndian.Uint16(local[28:]))
	if !inside(int64(start)+30, names+extras, central) || int64(start)+30+names+extras != expectedOffset || names != int64(len(entry.Name)) {
		return 0, ErrDirectInvalid
	}
	fields := make([]byte, names+extras)
	if _, err := r.ReadAt(fields, int64(start)+30); err != nil {
		return 0, err
	}
	if string(fields[:names]) != entry.Name {
		return 0, ErrDirectUnsafe
	}
	if entry.Flags&8 == 0 {
		packed, unpacked := uint64(binary.LittleEndian.Uint32(local[18:])), uint64(binary.LittleEndian.Uint32(local[22:]))
		if packed == 0xffffffff || unpacked == 0xffffffff {
			extra, ok := zipExtraField(fields[names:], 1)
			if !ok {
				return 0, ErrDirectInvalid
			}
			if unpacked == 0xffffffff {
				if len(extra) < 8 {
					return 0, ErrDirectInvalid
				}
				unpacked = binary.LittleEndian.Uint64(extra)
				extra = extra[8:]
			}
			if packed == 0xffffffff {
				if len(extra) < 8 {
					return 0, ErrDirectInvalid
				}
				packed = binary.LittleEndian.Uint64(extra)
			}
		}
		if packed != entry.CompressedSize64 || unpacked != entry.UncompressedSize64 || binary.LittleEndian.Uint32(local[14:]) != entry.CRC32 {
			return 0, ErrDirectInvalid
		}
	}
	return int64(start), nil
}

func locateTAR(r io.ReaderAt, size int64, name string, limits DirectLimits) (DirectMember, error) {
	source := io.NewSectionReader(r, 0, size)
	t := tar.NewReader(source)
	v := directValidator{limits: limits, seen: map[string]bool{}}
	var result DirectMember
	found := false
	for {
		header, err := t.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return DirectMember{}, err
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA && header.Typeflag != tar.TypeDir {
			return DirectMember{}, ErrDirectUnsafe
		}
		for key := range header.PAXRecords {
			if strings.HasPrefix(key, "GNU.sparse") || strings.HasPrefix(key, "SCHILY.") {
				return DirectMember{}, ErrDirectLayout
			}
		}
		if header.Size < 0 {
			return DirectMember{}, ErrDirectInvalid
		}
		if err := v.entry(header.Name, header.FileInfo().Mode(), uint64(header.Size)); err != nil {
			return DirectMember{}, err
		}
		offset, err := source.Seek(0, io.SeekCurrent)
		if err != nil {
			return DirectMember{}, err
		}
		if !inside(offset, header.Size, size) {
			return DirectMember{}, ErrDirectInvalid
		}
		if header.Name == name && header.Typeflag != tar.TypeDir {
			result, found = DirectMember{offset, header.Size}, true
		}
	}
	if !found {
		return DirectMember{}, ErrDirectMemberMissing
	}
	return result, nil
}
