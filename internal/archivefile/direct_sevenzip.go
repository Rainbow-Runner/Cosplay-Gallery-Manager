package archivefile

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"unicode/utf16"

	"github.com/bodgit/sevenzip"
	"github.com/ulikunitz/xz/lzma"
)

// A deliberately narrow proof reader, not another general 7z decompressor.
// Only plain headers or a single bounded LZMA encoded header, then one Copy
// coder per member folder, are accepted. Crypto, filters, external properties
// and alternative graphs fail closed. Packed and unpacked sizes and CRCs
// must agree. No private sevenzip fields or unsafe/reflection are used.
// Format reference: https://github.com/ip7z/7zip/blob/main/DOC/7zFormat.txt
func locateSevenZIP(ctx context.Context, input io.ReaderAt, size int64, name string, limits DirectLimits) (DirectMember, error) {
	var signature [32]byte
	if size < 32 {
		return DirectMember{}, ErrDirectInvalid
	}
	if _, err := input.ReadAt(signature[:], 0); err != nil {
		return DirectMember{}, err
	}
	if !bytes.Equal(signature[:6], []byte{'7', 'z', 0xbc, 0xaf, 0x27, 0x1c}) || signature[6] != 0 {
		return DirectMember{}, ErrDirectLayout
	}
	if crc32.ChecksumIEEE(signature[12:]) != binary.LittleEndian.Uint32(signature[8:]) {
		return DirectMember{}, ErrDirectInvalid
	}
	offset, length := binary.LittleEndian.Uint64(signature[12:]), binary.LittleEndian.Uint64(signature[20:])
	if offset > uint64(size-32) || length == 0 || length > uint64(size-32)-offset {
		return DirectMember{}, ErrDirectInvalid
	}
	if length > uint64(limits.MaxHeaderBytes) {
		return DirectMember{}, ErrDirectLimit
	}
	header := make([]byte, length)
	if _, err := input.ReadAt(header, 32+int64(offset)); err != nil {
		return DirectMember{}, err
	}
	if crc32.ChecksumIEEE(header) != binary.LittleEndian.Uint32(signature[28:]) {
		return DirectMember{}, ErrDirectInvalid
	}
	dataEnd := offset
	if header[0] == 0x17 {
		var err error
		header, dataEnd, err = decodeSevenZIPHeader(ctx, input, header, offset, limits)
		if err != nil {
			return DirectMember{}, err
		}
	}
	c := sevenCursor{data: header}
	if c.byte() != 1 {
		return DirectMember{}, ErrDirectLayout
	}
	if c.byte() != 4 {
		return DirectMember{}, ErrDirectLayout
	}
	position, folders := c.copyStreams(limits, dataEnd)
	if c.err != nil {
		return DirectMember{}, fmt.Errorf("7z streams: %w", c.err)
	}
	if c.byte() != 5 {
		return DirectMember{}, ErrDirectLayout
	}
	files := c.count(limits.MaxEntries)
	names := []string(nil)
	empty, emptyFile := []bool(nil), []bool(nil)
	attributes := make([]uint32, files)
	seen := map[byte]bool{}
	for id := c.byte(); id != 0 && c.err == nil; id = c.byte() {
		if seen[id] && id != 0x19 { // Dummy padding can occur between multiple properties.
			return DirectMember{}, ErrDirectInvalid
		}
		seen[id] = true
		property := sevenCursor{data: c.take(c.number())}
		if c.err != nil {
			return DirectMember{}, c.err
		}
		switch id {
		case 0x0e:
			empty = property.bits(files)
		case 0x0f:
			if empty == nil {
				return DirectMember{}, ErrDirectInvalid
			}
			n := 0
			for _, v := range empty {
				if v {
					n++
				}
			}
			emptyFile = property.bits(n)
		case 0x10:
			return DirectMember{}, ErrDirectUnsafe // Anti items can delete when extracted; never accept.
		case 0x11:
			if property.byte() != 0 {
				return DirectMember{}, ErrDirectLayout
			}
			names = property.names(files)
		case 0x15:
			defined := property.defined(files)
			if property.byte() != 0 {
				return DirectMember{}, ErrDirectLayout
			}
			for i, ok := range defined {
				if ok {
					p := property.take(4)
					if len(p) == 4 {
						attributes[i] = binary.LittleEndian.Uint32(p)
					}
				}
			}
		case 0x12, 0x13, 0x14: // Times do not affect physical member offsets.
			defined := property.defined(files)
			if property.byte() != 0 {
				return DirectMember{}, ErrDirectLayout
			}
			for _, ok := range defined {
				if ok {
					property.take(8)
				}
			}
		case 0x19:
			property.take(uint64(len(property.data))) // Padding.
		default:
			return DirectMember{}, ErrDirectLayout
		}
		if property.err != nil || len(property.data) != 0 {
			return DirectMember{}, fmt.Errorf("7z property %x: %w", id, ErrDirectInvalid)
		}
	}
	if c.byte() != 0 || c.err != nil || len(c.data) != 0 || len(names) != files {
		return DirectMember{}, fmt.Errorf("7z files end (%d/%d, remaining %d): %w", len(names), files, len(c.data), ErrDirectInvalid)
	}
	if empty == nil {
		empty = make([]bool, files)
	}
	validator := directValidator{limits: limits, seen: map[string]bool{}}
	folderIndex, subIndex, emptyIndex := 0, 0, 0
	currentOffset := int64(32 + position)
	var found bool
	var result DirectMember
	for i, filename := range names {
		head := sevenzip.FileHeader{Name: filename, Attributes: attributes[i]}
		mode := head.Mode()
		memberSize := uint64(0)
		if empty[i] {
			if emptyFile == nil || !emptyFile[emptyIndex] {
				mode |= fs.ModeDir
			}
			emptyIndex++
		} else {
			if folderIndex >= len(folders) || subIndex >= len(folders[folderIndex]) {
				return DirectMember{}, fmt.Errorf("7z member stream: %w", ErrDirectInvalid)
			}
			memberSize = folders[folderIndex][subIndex]
		}
		if err := validator.entry(filename, mode, memberSize); err != nil {
			return DirectMember{}, err
		}
		if mode.IsDir() && memberSize != 0 {
			return DirectMember{}, ErrDirectInvalid
		}
		if filename == name && !mode.IsDir() {
			result, found = DirectMember{currentOffset, int64(memberSize)}, true
		}
		if !empty[i] {
			currentOffset += int64(memberSize)
			subIndex++
			if subIndex == len(folders[folderIndex]) {
				folderIndex++
				subIndex = 0
			}
		}
	}
	if folderIndex != len(folders) || subIndex != 0 {
		return DirectMember{}, ErrDirectInvalid
	}
	if !found {
		return DirectMember{}, ErrDirectMemberMissing
	}
	return result, nil
}

// decodeSevenZIPHeader accepts only one independent LZMA header stream. The
// compressed bytes must precede NextHeader, and both sizes and the decoded
// CRC are checked before the ordinary Copy-member proof sees the header.
func decodeSevenZIPHeader(ctx context.Context, input io.ReaderAt, header []byte, nextOffset uint64, limits DirectLimits) ([]byte, uint64, error) {
	c := sevenCursor{data: header}
	c.expect(0x17)
	c.expect(0x06) // PackInfo
	position := c.number()
	if c.number() != 1 {
		return nil, 0, ErrDirectLayout
	}
	c.expect(0x09)
	packedSize := c.number()
	if c.byte() != 0 || c.byte() != 0x07 || c.byte() != 0x0b || c.number() != 1 || c.byte() != 0 {
		return nil, 0, ErrDirectLayout
	}
	if c.number() != 1 || c.byte() != 0x23 || !bytes.Equal(c.take(3), []byte{3, 1, 1}) || c.number() != 5 {
		return nil, 0, ErrDirectLayout
	}
	properties := c.take(5)
	c.expect(0x0c)
	unpackedSize := c.number()
	c.expect(0x0a)
	if c.byte() != 1 {
		return nil, 0, ErrDirectLayout
	}
	checksumBytes := c.take(4)
	if c.byte() != 0 || c.byte() != 0 || c.err != nil || len(c.data) != 0 || len(properties) != 5 || len(checksumBytes) != 4 {
		return nil, 0, ErrDirectLayout
	}
	if packedSize == 0 || packedSize > uint64(limits.MaxHeaderBytes) || unpackedSize == 0 || unpackedSize > uint64(limits.MaxHeaderBytes) {
		return nil, 0, ErrDirectLimit
	}
	if position > nextOffset || packedSize > nextOffset-position {
		return nil, 0, ErrDirectInvalid
	}
	// LZMA properties encode the dictionary in bytes 1..4. Reject expensive
	// dictionaries even when a tiny output size is claimed.
	dictionary := binary.LittleEndian.Uint32(properties[1:])
	if properties[0] >= 225 || dictionary > 16<<20 {
		return nil, 0, ErrDirectLimit
	}
	packed := make([]byte, packedSize)
	if _, err := input.ReadAt(packed, 32+int64(position)); err != nil {
		return nil, 0, err
	}
	stream := make([]byte, 0, len(properties)+8+len(packed))
	stream = append(stream, properties...)
	var sizeBytes [8]byte
	binary.LittleEndian.PutUint64(sizeBytes[:], unpackedSize)
	stream = append(stream, sizeBytes[:]...)
	stream = append(stream, packed...)
	reader, err := lzma.NewReader(bytes.NewReader(stream))
	if err != nil {
		return nil, 0, ErrDirectInvalid
	}
	decoded := make([]byte, 0, unpackedSize)
	var chunk [4096]byte
	for uint64(len(decoded)) < unpackedSize {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		n, readErr := reader.Read(chunk[:min(uint64(len(chunk)), unpackedSize-uint64(len(decoded)))])
		decoded = append(decoded, chunk[:n]...)
		if readErr != nil {
			return nil, 0, ErrDirectInvalid
		}
	}
	if len(decoded) == 0 || decoded[0] != 1 || crc32.ChecksumIEEE(decoded) != binary.LittleEndian.Uint32(checksumBytes) {
		return nil, 0, ErrDirectInvalid
	}
	return decoded, position, nil
}

type sevenCursor struct {
	data []byte
	err  error
}

func (c *sevenCursor) take(n uint64) []byte {
	if c.err != nil {
		return nil
	}
	if n > uint64(len(c.data)) {
		c.err = ErrDirectInvalid
		return nil
	}
	p := c.data[:n]
	c.data = c.data[n:]
	return p
}
func (c *sevenCursor) byte() byte {
	p := c.take(1)
	if len(p) == 0 {
		return 0
	}
	return p[0]
}
func (c *sevenCursor) number() uint64 {
	first, value := c.byte(), uint64(0)
	mask := byte(0x80)
	for i := uint(0); i < 8; i++ {
		if first&mask == 0 {
			return value | uint64(first&(mask-1))<<(8*i)
		}
		value |= uint64(c.byte()) << (8 * i)
		mask >>= 1
	}
	return value
}
func (c *sevenCursor) count(maximum int) int {
	n := c.number()
	if n == 0 || n > uint64(maximum) {
		c.err = ErrDirectLimit
		return 0
	}
	return int(n)
}
func (c *sevenCursor) bits(n int) []bool {
	p := c.take(uint64((n + 7) / 8))
	if c.err != nil {
		return nil
	}
	result := make([]bool, n)
	for i := range result {
		result[i] = p[i/8]&(0x80>>(i%8)) != 0
	}
	return result
}
func (c *sevenCursor) defined(n int) []bool {
	all := c.byte()
	if all == 0 {
		return c.bits(n)
	}
	if all != 1 {
		c.err = ErrDirectInvalid
		return nil
	}
	result := make([]bool, n)
	for i := range result {
		result[i] = true
	}
	return result
}
func (c *sevenCursor) digests(n int) []bool {
	defined := c.defined(n)
	for _, ok := range defined {
		if ok {
			c.take(4)
		}
	}
	return defined
}
func (c *sevenCursor) expect(id byte) {
	if c.byte() != id && c.err == nil {
		c.err = ErrDirectLayout
	}
}
func (c *sevenCursor) names(n int) []string {
	var result []string
	var units []uint16
	for len(c.data) > 0 && c.err == nil {
		p := c.take(2)
		if len(p) != 2 {
			break
		}
		v := binary.LittleEndian.Uint16(p)
		if v != 0 {
			units = append(units, v)
			continue
		}
		decoded := utf16.Decode(units)
		if !bytes.Equal(utf16Bytes(utf16.Encode(decoded)), utf16Bytes(units)) {
			c.err = ErrDirectUnsafe
			return nil
		}
		result = append(result, string(decoded))
		units = nil
		if len(result) > n {
			c.err = ErrDirectInvalid
		}
	}
	if len(units) != 0 || len(result) != n {
		c.err = ErrDirectInvalid
	}
	return result
}
func utf16Bytes(units []uint16) []byte {
	p := make([]byte, 2*len(units))
	for i, v := range units {
		binary.LittleEndian.PutUint16(p[2*i:], v)
	}
	return p
}

func (c *sevenCursor) copyStreams(limits DirectLimits, dataEnd uint64) (uint64, [][]uint64) {
	c.expect(6)
	position := c.number()
	count := c.count(limits.MaxEntries)
	c.expect(9)
	packed := make([]uint64, count)
	for i := range packed {
		packed[i] = c.number()
	}
	id := c.byte()
	if id == 0x0a {
		c.digests(count)
		id = c.byte()
	}
	if id != 0 {
		c.err = ErrDirectLayout
	}
	c.expect(7)
	c.expect(0x0b)
	folderCount := c.count(limits.MaxEntries)
	if folderCount != count || c.byte() != 0 {
		c.err = ErrDirectLayout
	}
	for i := 0; i < folderCount && c.err == nil; i++ {
		if c.number() != 1 || c.byte() != 1 {
			c.err = ErrDirectLayout
			break
		}
		if c.byte() != 0 {
			c.err = ErrDirectCompressed
			break
		}
	}
	if c.err != nil {
		return position, nil
	}
	c.expect(0x0c)
	unpacked := make([]uint64, count)
	end := position
	for i := range unpacked {
		unpacked[i] = c.number()
		if packed[i] != unpacked[i] || end > dataEnd || packed[i] > dataEnd-end {
			c.err = ErrDirectInvalid
			break
		}
		end += packed[i]
	}
	crcDefined := make([]bool, count)
	id = c.byte()
	if id == 0x0a {
		crcDefined = c.digests(count)
		id = c.byte()
	}
	if id != 0 {
		c.err = ErrDirectLayout
	}
	if c.err != nil {
		return position, nil
	}
	id = c.byte()
	counts := make([]int, count)
	for i := range counts {
		counts[i] = 1
	}
	folders := make([][]uint64, count)
	if id == 8 {
		id = c.byte()
		if id == 0x0d {
			total := 0
			for i := range counts {
				counts[i] = c.count(limits.MaxEntries)
				total += counts[i]
				if total > limits.MaxEntries {
					c.err = ErrDirectLimit
					break
				}
			}
			id = c.byte()
		}
		if c.err != nil {
			return position, nil
		}
		for i := range folders {
			folders[i] = make([]uint64, counts[i])
			sum := uint64(0)
			for j := 0; j < counts[i]-1 && c.err == nil; j++ {
				if id != 9 {
					c.err = ErrDirectLayout
					break
				}
				n := c.number()
				if sum > unpacked[i] || n > unpacked[i]-sum {
					c.err = ErrDirectInvalid
					break
				}
				folders[i][j] = n
				sum += n
			}
			if sum <= unpacked[i] && counts[i] > 0 {
				folders[i][counts[i]-1] = unpacked[i] - sum
			}
		}
		if id == 9 {
			id = c.byte()
		}
		if id == 0x0a {
			n := 0
			for i, v := range counts {
				if v != 1 || !crcDefined[i] {
					n += v
				}
			}
			c.digests(n)
			id = c.byte()
		}
		if id != 0 {
			c.err = ErrDirectLayout
		}
		id = c.byte()
	} else {
		for i := range folders {
			folders[i] = []uint64{unpacked[i]}
		}
	}
	if id != 0 {
		c.err = ErrDirectLayout
	}
	return position, folders
}
