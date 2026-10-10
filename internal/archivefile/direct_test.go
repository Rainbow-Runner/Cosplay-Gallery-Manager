package archivefile

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"math"
	"os"
	"testing"
	"unicode/utf16"

	"github.com/bodgit/sevenzip"
)

func TestDirectMemberStoredFormatsAndBoundedPayload(t *testing.T) {
	for _, format := range []Format{FormatZIP, FormatTAR, FormatSevenZIP} {
		t.Run(string(format), func(t *testing.T) {
			body := directFixture(t, format, []string{"before.jpg", "folder/clip.mp4", "after.jpg"}, zip.Store)
			member, err := LocateDirectMember(context.Background(), format, bytes.NewReader(body), int64(len(body)), "folder/clip.mp4", DefaultDirectLimits())
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(io.NewSectionReader(bytes.NewReader(body), member.Offset, member.Size))
			if err != nil || string(got) != "payload-1" {
				t.Fatalf("member: %q %v", got, err)
			}
			if format == FormatSevenZIP {
				z, err := sevenzip.NewReader(bytes.NewReader(body), int64(len(body)))
				if err != nil {
					t.Fatalf("fixture rejected by independent reader: %v", err)
				}
				for i, file := range z.File {
					r, err := file.Open()
					if err != nil {
						t.Fatal(err)
					}
					got, err := io.ReadAll(r)
					r.Close()
					if err != nil || string(got) != string(directPayload(i)) {
						t.Fatalf("7z member %d: %q %v", i, got, err)
					}
				}
			}
		})
	}
}

func TestDirectMemberRejectsUnsafePathsAndDuplicateNames(t *testing.T) {
	for _, format := range []Format{FormatZIP, FormatTAR, FormatSevenZIP} {
		for _, names := range [][]string{{"clip.mp4", "clip.mp4"}, {"clip.mp4", "CLIP.mp4"}, {"clip.mp4", "../escape"}, {"clip.mp4", "/escape"}, {"clip.mp4", "C:/escape"}, {"clip.mp4", "a\\b"}, {"clip.mp4", "e\u0301.jpg"}, {"clip.mp4", "nested.zip"}} {
			body := directFixture(t, format, names, zip.Store)
			_, err := LocateDirectMember(context.Background(), format, bytes.NewReader(body), int64(len(body)), "clip.mp4", DefaultDirectLimits())
			if !errors.Is(err, ErrDirectUnsafe) {
				t.Fatalf("%s %q: %v", format, names, err)
			}
		}
	}
}

func TestDirectMemberRejectsCompressedMissingCancelledAndOverLimit(t *testing.T) {
	body := directFixture(t, FormatZIP, []string{"clip.mp4"}, zip.Deflate)
	if _, err := LocateDirectMember(context.Background(), FormatZIP, bytes.NewReader(body), int64(len(body)), "clip.mp4", DefaultDirectLimits()); !errors.Is(err, ErrDirectCompressed) {
		t.Fatalf("compressed ZIP: %v", err)
	}
	if _, err := LocateDirectMember(context.Background(), FormatTARGZIP, bytes.NewReader(nil), 0, "clip.mp4", DefaultDirectLimits()); !errors.Is(err, ErrDirectCompressed) {
		t.Fatalf("gzip TAR: %v", err)
	}
	for _, format := range []Format{FormatZIP, FormatTAR, FormatSevenZIP} {
		body := directFixture(t, format, []string{"clip.mp4", "next.jpg"}, zip.Store)
		for _, option := range []string{"missing", "cancelled", "entries", "member", "total", "huge-limits"} {
			limits, name, ctx := DefaultDirectLimits(), "clip.mp4", context.Background()
			want := ErrDirectLimit
			switch option {
			case "missing":
				name, want = "missing.mp4", ErrDirectMemberMissing
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
				want = context.Canceled
			case "entries":
				limits.MaxEntries = 1
			case "member":
				limits.MaxMemberBytes = 1
			case "total":
				limits.MaxTotalBytes = 1
			case "huge-limits":
				limits.MaxHeaderBytes = math.MaxInt64
			}
			if _, err := LocateDirectMember(ctx, format, bytes.NewReader(body), int64(len(body)), name, limits); !errors.Is(err, want) {
				t.Fatalf("%s/%s: %v want %v", format, option, err, want)
			}
		}
	}
}

func TestDirectZIPLocalIdentityAndEncryptionMustAgree(t *testing.T) {
	for _, option := range []string{"name", "method", "encrypted", "size", "count"} {
		body := directFixture(t, FormatZIP, []string{"clip.mp4"}, zip.Store)
		central := bytes.Index(body, []byte{0x50, 0x4b, 0x01, 0x02})
		want := ErrDirectUnsafe
		switch option {
		case "name":
			body[30] = 'X'
		case "method":
			binary.LittleEndian.PutUint16(body[8:], zip.Deflate)
		case "encrypted":
			body[central+8] |= 1
		case "size":
			binary.LittleEndian.PutUint32(body[central+20:], 0xffff)
			want = ErrDirectInvalid
		case "count":
			binary.LittleEndian.PutUint16(body[len(body)-12:], 0xffff)
			want = ErrDirectLayout
		}
		_, err := LocateDirectMember(context.Background(), FormatZIP, bytes.NewReader(body), int64(len(body)), "clip.mp4", DefaultDirectLimits())
		if !errors.Is(err, want) {
			t.Fatalf("%s: %v want %v", option, err, want)
		}
	}
}

func TestDirectZIP64SmallPayload(t *testing.T) {
	body := directFixture(t, FormatZIP, []string{"clip.mp4"}, zip.Store)
	end := len(body) - 22
	eocd := append([]byte(nil), body[end:]...)
	record := make([]byte, 56)
	binary.LittleEndian.PutUint32(record, 0x06064b50)
	binary.LittleEndian.PutUint64(record[4:], 44)
	binary.LittleEndian.PutUint64(record[24:], 1)
	binary.LittleEndian.PutUint64(record[32:], 1)
	binary.LittleEndian.PutUint64(record[40:], uint64(binary.LittleEndian.Uint32(eocd[12:])))
	binary.LittleEndian.PutUint64(record[48:], uint64(binary.LittleEndian.Uint32(eocd[16:])))
	locator := make([]byte, 20)
	binary.LittleEndian.PutUint32(locator, 0x07064b50)
	binary.LittleEndian.PutUint64(locator[8:], uint64(end))
	binary.LittleEndian.PutUint32(locator[16:], 1)
	binary.LittleEndian.PutUint16(eocd[8:], 0xffff)
	binary.LittleEndian.PutUint16(eocd[10:], 0xffff)
	binary.LittleEndian.PutUint32(eocd[12:], 0xffffffff)
	binary.LittleEndian.PutUint32(eocd[16:], 0xffffffff)
	body = append(body[:end], record...)
	body = append(body, locator...)
	body = append(body, eocd...)
	member, err := LocateDirectMember(context.Background(), FormatZIP, bytes.NewReader(body), int64(len(body)), "clip.mp4", DefaultDirectLimits())
	if err != nil || string(body[member.Offset:member.Offset+member.Size]) != "payload-0" {
		t.Fatalf("ZIP64: %#v %v", member, err)
	}
}

func TestDirectTARRejectsLinksAndSpecialFiles(t *testing.T) {
	for _, flag := range []byte{tar.TypeSymlink, tar.TypeLink, tar.TypeFifo, tar.TypeChar} {
		var buffer bytes.Buffer
		w := tar.NewWriter(&buffer)
		if err := w.WriteHeader(&tar.Header{Name: "clip.mp4", Typeflag: flag, Linkname: "elsewhere", Mode: 0600}); err != nil {
			t.Fatal(err)
		}
		w.Close()
		_, err := LocateDirectMember(context.Background(), FormatTAR, bytes.NewReader(buffer.Bytes()), int64(buffer.Len()), "clip.mp4", DefaultDirectLimits())
		if !errors.Is(err, ErrDirectUnsafe) {
			t.Fatalf("TAR type %c: %v", flag, err)
		}
	}
}

func TestDirectTARRejectsAlternativePAXLayout(t *testing.T) {
	var buffer bytes.Buffer
	w := tar.NewWriter(&buffer)
	if err := w.WriteHeader(&tar.Header{Name: "clip.mp4", Mode: 0600, Size: 3, Format: tar.FormatPAX, PAXRecords: map[string]string{"SCHILY.realsize": "9"}}); err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("end"))
	w.Close()
	if _, err := LocateDirectMember(context.Background(), FormatTAR, bytes.NewReader(buffer.Bytes()), int64(buffer.Len()), "clip.mp4", DefaultDirectLimits()); !errors.Is(err, ErrDirectLayout) {
		t.Fatalf("alternative PAX size must not become a physical slice: %v", err)
	}
}

func TestDirectSevenZIPRepeatedPadding(t *testing.T) {
	body := directSevenFixture([]string{"clip.mp4", "second.jpg"})
	start := 32 + int(binary.LittleEndian.Uint64(body[12:]))
	header := append([]byte(nil), body[start:len(body)-2]...)
	header = append(header, 0x19, 2, 0, 0, 0x19, 1, 0, 0, 0)
	body = append(body[:start], header...)
	binary.LittleEndian.PutUint64(body[20:], uint64(len(header)))
	binary.LittleEndian.PutUint32(body[28:], crc32.ChecksumIEEE(header))
	binary.LittleEndian.PutUint32(body[8:], crc32.ChecksumIEEE(body[12:32]))
	member, err := LocateDirectMember(context.Background(), FormatSevenZIP, bytes.NewReader(body), int64(len(body)), "clip.mp4", DefaultDirectLimits())
	if err != nil || string(body[member.Offset:member.Offset+member.Size]) != "payload-0" {
		t.Fatalf("repeated legal padding: %#v %v", member, err)
	}
}

type countingDirectReader struct {
	input  *bytes.Reader
	read   int
	cancel context.CancelFunc
}

func (r *countingDirectReader) ReadAt(p []byte, offset int64) (int, error) {
	n, err := r.input.ReadAt(p, offset)
	r.read += n
	if r.cancel != nil {
		r.cancel()
	}
	return n, err
}

func TestDirectTARSkipsPayloadAndBoundsMetadata(t *testing.T) {
	var buffer bytes.Buffer
	w := tar.NewWriter(&buffer)
	if err := w.WriteHeader(&tar.Header{Name: "large.jpg", Mode: 0600, Size: 2 << 20}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(make([]byte, 2<<20)); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteHeader(&tar.Header{Name: "clip.mp4", Mode: 0600, Size: 3}); err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("end"))
	w.Close()
	r := &countingDirectReader{input: bytes.NewReader(buffer.Bytes())}
	member, err := LocateDirectMember(context.Background(), FormatTAR, r, int64(buffer.Len()), "clip.mp4", DefaultDirectLimits())
	if err != nil || string(buffer.Bytes()[member.Offset:member.Offset+member.Size]) != "end" || r.read > 4096 {
		t.Fatalf("must skip payload: member %#v, bytes read %d, error %v", member, r.read, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	r = &countingDirectReader{input: bytes.NewReader(buffer.Bytes()), cancel: cancel}
	if _, err := LocateDirectMember(ctx, FormatTAR, r, int64(buffer.Len()), "clip.mp4", DefaultDirectLimits()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel during metadata reads: %v", err)
	}
	r = &countingDirectReader{input: bytes.NewReader(buffer.Bytes())}
	budget := &metadataReader{ctx: context.Background(), input: r, remaining: 1}
	if _, err := budget.ReadAt(make([]byte, 2), 0); !errors.Is(err, ErrDirectLimit) || r.read != 0 {
		t.Fatalf("metadata budget must reject before read: %v, %d bytes", err, r.read)
	}
}

func TestDirectSevenZIPFailsClosedForUnprovenHeaders(t *testing.T) {
	for _, option := range []string{"encoded", "compressed", "crc", "start-crc", "truncated", "huge-files"} {
		body := directFixture(t, FormatSevenZIP, []string{"clip.mp4"}, zip.Store)
		headerStart := 32 + int(binary.LittleEndian.Uint64(body[12:]))
		want := ErrDirectInvalid
		switch option {
		case "encoded":
			body[headerStart] = 0x17
			want = ErrDirectLayout
		case "compressed":
			body[headerStart+14] = 0x21
			want = ErrDirectCompressed
		case "crc":
			body[len(body)-1] ^= 1
		case "start-crc":
			body[8] ^= 1
		case "truncated":
			body = body[:len(body)-2]
		case "huge-files":
			body[headerStart+7] = 0xff
			want = ErrDirectLayout
		}
		if option == "encoded" || option == "compressed" || option == "huge-files" {
			binary.LittleEndian.PutUint32(body[28:], crc32.ChecksumIEEE(body[headerStart:]))
			binary.LittleEndian.PutUint32(body[8:], crc32.ChecksumIEEE(body[12:32]))
		}
		_, err := LocateDirectMember(context.Background(), FormatSevenZIP, bytes.NewReader(body), int64(len(body)), "clip.mp4", DefaultDirectLimits())
		if !errors.Is(err, want) {
			t.Fatalf("%s: %v want %v", option, err, want)
		}
	}
}

// Opt-in business-media smoke gate: no payload extraction or mutation.
func TestDirectSevenZIPEncodedBusinessHeader(t *testing.T) {
	filename, memberName := os.Getenv("CGM_ARCHIVE_SMOKE_PATH"), os.Getenv("CGM_ARCHIVE_SMOKE_MEMBER")
	if filename == "" || memberName == "" {
		t.Skip("requires a read-only 7z path and member name")
	}
	file, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	member, err := LocateDirectMember(context.Background(), FormatSevenZIP, file, info.Size(), memberName, DefaultDirectLimits())
	if err != nil {
		t.Fatal(err)
	}
	if member.Offset < 32 || member.Size < 16 || member.Offset+member.Size > info.Size() {
		t.Fatalf("invalid member bounds %#v", member)
	}
	var prefix [12]byte
	if _, err := file.ReadAt(prefix[:], member.Offset); err != nil || !bytes.Contains(prefix[:], []byte("ftyp")) {
		t.Fatalf("incorrect MP4 member prefix %x: %v", prefix, err)
	}
}

// Tests may synthesize files; fixture bytes are not user media.
func directPayload(i int) []byte { return []byte("payload-" + string(rune('0'+i))) }
func directFixture(t testing.TB, format Format, names []string, method uint16) []byte {
	t.Helper()
	var buffer bytes.Buffer
	switch format {
	case FormatZIP:
		w := zip.NewWriter(&buffer)
		for i, name := range names {
			part, err := w.CreateHeader(&zip.FileHeader{Name: name, Method: method})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write(directPayload(i)); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	case FormatTAR:
		w := tar.NewWriter(&buffer)
		for i, name := range names {
			data := directPayload(i)
			if err := w.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(data))}); err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write(data); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	case FormatSevenZIP:
		return directSevenFixture(names)
	default:
		t.Fatal("unsupported fixture")
	}
	return buffer.Bytes()
}

func directSevenFixture(names []string) []byte {
	var body bytes.Buffer
	for i := range names {
		body.Write(directPayload(i))
	}
	header := []byte{1, 4, 6, 0, 1, 9}
	header = append(header, sevenNumber(uint64(body.Len()))...)
	header = append(header, 0, 7, 0x0b, 1, 0, 1, 1, 0, 0x0c)
	header = append(header, sevenNumber(uint64(body.Len()))...)
	header = append(header, 0, 8, 0x0d)
	header = append(header, sevenNumber(uint64(len(names)))...)
	header = append(header, 9)
	for i := 0; i < len(names)-1; i++ {
		header = append(header, sevenNumber(uint64(len(directPayload(i))))...)
	}
	header = append(header, 0x0a, 1)
	for i := range names {
		header = binary.LittleEndian.AppendUint32(header, crc32.ChecksumIEEE(directPayload(i)))
	}
	header = append(header, 0, 0, 5)
	header = append(header, sevenNumber(uint64(len(names)))...)
	property := []byte{0}
	for _, name := range names {
		for _, v := range utf16.Encode([]rune(name)) {
			property = binary.LittleEndian.AppendUint16(property, v)
		}
		property = append(property, 0, 0)
	}
	header = append(header, 0x11)
	header = append(header, sevenNumber(uint64(len(property)))...)
	header = append(header, property...)
	header = append(header, 0, 0)
	signature := make([]byte, 32)
	copy(signature, []byte{'7', 'z', 0xbc, 0xaf, 0x27, 0x1c, 0, 4})
	binary.LittleEndian.PutUint64(signature[12:], uint64(body.Len()))
	binary.LittleEndian.PutUint64(signature[20:], uint64(len(header)))
	binary.LittleEndian.PutUint32(signature[28:], crc32.ChecksumIEEE(header))
	binary.LittleEndian.PutUint32(signature[8:], crc32.ChecksumIEEE(signature[12:]))
	return append(append(signature, body.Bytes()...), header...)
}
func sevenNumber(n uint64) []byte {
	for i := uint(0); i < 8; i++ {
		if n < uint64(1)<<(7+7*i) {
			first := byte(n>>(8*i)) | byte(0xff<<(8-i))
			p := []byte{first}
			for j := uint(0); j < i; j++ {
				p = append(p, byte(n>>(8*j)))
			}
			return p
		}
	}
	p := []byte{0xff}
	return binary.LittleEndian.AppendUint64(p, n)
}

func FuzzLocateDirectMember(f *testing.F) {
	f.Add(byte(0), directSevenFixture([]string{"clip.mp4"}))
	f.Add(byte(1), directFixture(f, FormatZIP, []string{"clip.mp4"}, zip.Store))
	f.Add(byte(2), directFixture(f, FormatTAR, []string{"clip.mp4"}, zip.Store))
	f.Fuzz(func(t *testing.T, choice byte, body []byte) {
		if len(body) > 1<<20 {
			t.Skip()
		}
		format := []Format{FormatSevenZIP, FormatZIP, FormatTAR}[choice%3]
		_, _ = LocateDirectMember(context.Background(), format, bytes.NewReader(body), int64(len(body)), "clip.mp4", DefaultDirectLimits())
	})
}
