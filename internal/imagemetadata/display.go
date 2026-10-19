package imagemetadata

// Display extraction is intentionally separate from ExtractPath: capture-date
// jobs must retain their EXIF-only evidence and original parsing semantics.
import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rwcarlsen/goexif/exif"
	"github.com/stashapp/stash/internal/settings"
)

const metadataBudget = 8 << 20
const historyLimit = 100
const (
	nsXMP   = "http://ns.adobe.com/xap/1.0/"
	nsMM    = "http://ns.adobe.com/xap/1.0/mm/"
	nsEvent = "http://ns.adobe.com/xap/1.0/sType/ResourceEvent#"
	nsRDF   = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
)

func ExtractDisplayPath(filename string) (Result, error) {
	result, err := ExtractPath(filename)
	if err != nil {
		return result, err
	}
	for i := range result.Entries {
		result.Entries[i].Source = "EXIF"
		result.Entries[i].RawValue = result.Entries[i].Value
	}
	enrichEXIFDates(result.Entries)
	f, err := os.Open(filename)
	if err != nil {
		return result, err
	}
	defer f.Close()
	reader := &displayReader{result: &result, remaining: metadataBudget, extended: make(map[string]*extendedPacket)}
	var header [12]byte
	n, _ := io.ReadFull(f, header[:])
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return result, err
	}
	limited := io.LimitReader(f, maximumMetadataReadBytes)
	switch {
	case n >= 2 && header[0] == 0xff && header[1] == 0xd8:
		reader.jpeg(limited)
	case n >= 8 && bytes.Equal(header[:8], []byte("\x89PNG\r\n\x1a\n")):
		reader.png(limited)
	case n == 12 && string(header[:4]) == "RIFF" && string(header[8:]) == "WEBP":
		reader.webp(limited)
	default:
		reader.warn("METADATA_FORMAT_UNSUPPORTED")
	}
	for _, packet := range reader.extended {
		sort.Slice(packet.parts, func(i, j int) bool { return packet.parts[i].offset < packet.parts[j].offset })
		var data []byte
		for _, part := range packet.parts {
			if part.offset != len(data) {
				break
			}
			data = append(data, part.data...)
		}
		if len(data) == packet.total {
			reader.xmp(data)
		} else {
			reader.warn("XMP_INCOMPLETE")
		}
	}
	if len(result.Entries) > maximumEntries {
		result.Entries = result.Entries[:maximumEntries]
		result.Truncated = true
	}
	sort.SliceStable(result.Entries, func(i, j int) bool {
		a, b := metadataGroupOrder[result.Entries[i].Group], metadataGroupOrder[result.Entries[j].Group]
		if a != b {
			return a < b
		}
		return result.Entries[i].Key < result.Entries[j].Key
	})
	return result, nil
}

func enrichEXIFDates(entries []Entry, extra ...map[string]string) {
	values := make(map[string]string)
	for _, entry := range entries {
		values[entry.Key] = entry.Value
	}
	for _, set := range extra {
		for key, value := range set {
			values[key] = value
		}
	}
	for i := range entries {
		entry := &entries[i]
		suffix, offset := "", "OffsetTime"
		switch entry.Key {
		case "exif.DateTime":
		case "exif.DateTimeOriginal":
			suffix, offset = "Original", "OffsetTimeOriginal"
		case "exif.DateTimeDigitized":
			suffix, offset = "Digitized", "OffsetTimeDigitized"
		default:
			continue
		}
		value := strings.Replace(entry.Value, ":", "-", 2)
		if fraction := values["exif.SubSecTime"+suffix]; fraction != "" && digitsOnly(fraction) {
			value += "." + fraction
		}
		if zone := values["exif."+offset]; validOffset(zone) {
			value += " " + zone
		}
		entry.Value, entry.Timestamp = displayTime(value)
		entry.RawValue = values[entry.Key]
		if fraction := values["exif.SubSecTime"+suffix]; fraction != "" {
			entry.RawValue += "; SubSecTime" + suffix + "=" + fraction
		}
		if zone := values["exif."+offset]; zone != "" {
			entry.RawValue += "; " + offset + "=" + zone
		}
	}
}

func validOffset(s string) bool {
	if len(s) != 6 || (s[0] != '+' && s[0] != '-') || s[3] != ':' {
		return false
	}
	_, err := time.Parse("-07:00", s)
	return err == nil
}
func digitsOnly(s string) bool {
	if len(s) > 9 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}
func displayTime(value string) (string, string) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999 -07:00", "2006-01-02 15:04:05.999999999-0700"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("2006-01-02 15:04:05.999999999 -07:00"), parsed.Format(time.RFC3339Nano)
		}
	}
	// No inferred timezone: an unzoned timestamp is not an absolute instant.
	return strings.Replace(value, "T", " ", 1), ""
}

type extendedPart struct {
	offset int
	data   []byte
}
type extendedPacket struct {
	total int
	parts []extendedPart
}
type displayReader struct {
	result    *Result
	remaining int
	extended  map[string]*extendedPacket
}

func (r *displayReader) warn(code string) {
	for _, old := range r.result.Warnings {
		if old == code {
			return
		}
	}
	r.result.Warnings = append(r.result.Warnings, code)
}
func (r *displayReader) take(in io.Reader, n int) []byte {
	if n < 0 || n > r.remaining {
		r.result.Truncated = true
		return nil
	}
	r.remaining -= n
	value := make([]byte, n)
	if _, err := io.ReadFull(in, value); err != nil {
		r.warn("METADATA_READ_INCOMPLETE")
		return nil
	}
	return value
}
func (r *displayReader) add(key, visibility, label, group, value, source string) {
	if strings.Contains(visibility, "filename") {
		value = path.Base(strings.ReplaceAll(value, "\\", "/"))
	}
	if utf8.RuneCountInString(value) > maximumValueRunes {
		r.result.Truncated = true
	}
	raw := cleanValue(value)
	if raw == "" {
		return
	}
	if len(r.result.Entries) >= maximumEntries {
		r.result.Truncated = true
		return
	}
	for _, entry := range r.result.Entries {
		if entry.Key == key && entry.Value == raw {
			return
		}
	}
	display, stamp := raw, ""
	if group == "DATES" {
		display, stamp = displayTime(raw)
	}
	if strings.Contains(visibility, "filename") {
		display = path.Base(strings.ReplaceAll(raw, "\\", "/"))
		raw = display
		if display == "." || display == "/" {
			return
		}
	}
	keyBase := key
	for count := 1; ; count++ {
		found := false
		for _, entry := range r.result.Entries {
			if entry.Key == key {
				found = true
				break
			}
		}
		if !found {
			break
		}
		key = fmt.Sprintf("%s.%d", keyBase, count)
	}
	r.result.Entries = append(r.result.Entries, Entry{Key: key, VisibilityKey: visibility, Label: label, Group: group, Value: display, RawValue: raw, Source: source, Timestamp: stamp})
}

func (r *displayReader) jpeg(in io.Reader) {
	var soi [2]byte
	_, _ = io.ReadFull(in, soi[:])
	for count := 0; count < 1024; count++ {
		var marker [2]byte
		if _, err := io.ReadFull(in, marker[:]); err != nil {
			return
		}
		if marker[0] != 0xff {
			r.warn("METADATA_READ_INCOMPLETE")
			return
		}
		for marker[1] == 0xff {
			if _, err := io.ReadFull(in, marker[1:]); err != nil {
				return
			}
		}
		if marker[1] == 0xda || marker[1] == 0xd9 {
			return
		}
		if marker[1] == 0x01 || marker[1] >= 0xd0 && marker[1] <= 0xd8 {
			continue
		}
		var length [2]byte
		if _, err := io.ReadFull(in, length[:]); err != nil {
			return
		}
		size := int(binary.BigEndian.Uint16(length[:])) - 2
		if size < 0 {
			r.warn("METADATA_READ_INCOMPLETE")
			return
		}
		if marker[1] != 0xe1 && marker[1] != 0xed && marker[1] != 0xe2 {
			if _, err := io.CopyN(io.Discard, in, int64(size)); err != nil {
				return
			}
			continue
		}
		data := r.take(in, size)
		if data == nil {
			return
		}
		switch {
		case bytes.HasPrefix(data, []byte("Exif\x00\x00")):
			r.embeddedEXIF(data)
		case bytes.HasPrefix(data, []byte("http://ns.adobe.com/xap/1.0/\x00")):
			r.xmp(data[len("http://ns.adobe.com/xap/1.0/\x00"):])
		case bytes.HasPrefix(data, []byte("http://ns.adobe.com/xmp/extension/\x00")):
			payload := data[len("http://ns.adobe.com/xmp/extension/\x00"):]
			if len(payload) < 40 {
				r.warn("XMP_INCOMPLETE")
				continue
			}
			guid := string(payload[:32])
			total, offset := int(binary.BigEndian.Uint32(payload[32:36])), int(binary.BigEndian.Uint32(payload[36:40]))
			if total > metadataBudget || offset > total || len(payload)-40 > total-offset {
				r.result.Truncated = true
				continue
			}
			packet := r.extended[guid]
			if packet == nil {
				packet = &extendedPacket{total: total}
				r.extended[guid] = packet
			}
			if packet.total != total {
				r.warn("XMP_INCOMPLETE")
				continue
			}
			packet.parts = append(packet.parts, extendedPart{offset, payload[40:]})
		case marker[1] == 0xed:
			r.photoshop(data)
		case marker[1] == 0xe2 && bytes.HasPrefix(data, []byte("ICC_PROFILE\x00")):
			r.icc(data)
		}
	}
	r.result.Truncated = true
}

func (r *displayReader) png(in io.Reader) {
	var signature [8]byte
	_, _ = io.ReadFull(in, signature[:])
	for count := 0; count < 2048; count++ {
		var header [8]byte
		if _, err := io.ReadFull(in, header[:]); err != nil {
			return
		}
		size := int64(binary.BigEndian.Uint32(header[:4]))
		kind := string(header[4:])
		if kind == "IEND" {
			return
		}
		wanted := kind == "iTXt" || kind == "tEXt" || kind == "zTXt" || kind == "eXIf"
		if !wanted {
			if _, err := io.CopyN(io.Discard, in, size+4); err != nil {
				r.result.Truncated = true
				r.warn("METADATA_READ_INCOMPLETE")
				return
			}
			continue
		}
		data := r.take(in, int(size))
		if data == nil {
			return
		}
		if kind == "eXIf" {
			r.embeddedEXIF(data)
		} else {
			r.pngText(kind, data)
		}
		if _, err := io.CopyN(io.Discard, in, 4); err != nil {
			return
		}
	}
	r.result.Truncated = true
}
func (r *displayReader) inflate(data []byte) []byte {
	decoder, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		r.warn("METADATA_PARSE_FAILED")
		return nil
	}
	defer decoder.Close()
	decoded, err := io.ReadAll(io.LimitReader(decoder, int64(r.remaining)+1))
	if err != nil {
		r.warn("METADATA_PARSE_FAILED")
		return nil
	}
	if len(decoded) > r.remaining {
		r.result.Truncated = true
		return nil
	}
	r.remaining -= len(decoded)
	return decoded
}
func (r *displayReader) pngText(kind string, data []byte) {
	split := bytes.IndexByte(data, 0)
	if split < 0 {
		return
	}
	key := string(data[:split])
	value := data[split+1:]
	switch kind {
	case "iTXt":
		if len(value) < 2 {
			return
		}
		compressed, method := value[0], value[1]
		value = value[2:]
		for i := 0; i < 2; i++ {
			at := bytes.IndexByte(value, 0)
			if at < 0 {
				return
			}
			value = value[at+1:]
		}
		if compressed == 1 {
			if method != 0 {
				return
			}
			value = r.inflate(value)
		} else if compressed != 0 {
			return
		}
	case "zTXt":
		if len(value) < 1 || value[0] != 0 {
			return
		}
		value = r.inflate(value[1:])
	}
	if key == "XML:com.adobe.xmp" {
		r.xmp(value)
		return
	}
	fields := map[string][2]string{"Title": {settings.MetadataTitle, "Title"}, "Description": {settings.MetadataDescription, "Description"}, "Author": {settings.MetadataAuthor, "Author"}, "Copyright": {settings.MetadataCopyright, "Copyright"}, "Software": {settings.MetadataSoftware, "Software"}}
	if definition, ok := fields[key]; ok && utf8.Valid(value) {
		r.add("png."+key, definition[0], definition[1], "DESCRIPTIVE", string(value), "PNG")
	}
}
func (r *displayReader) webp(in io.Reader) {
	var header [12]byte
	_, _ = io.ReadFull(in, header[:])
	for count := 0; count < 2048; count++ {
		var chunk [8]byte
		if _, err := io.ReadFull(in, chunk[:]); err != nil {
			return
		}
		size := int64(binary.LittleEndian.Uint32(chunk[4:]))
		kind := string(chunk[:4])
		if kind == "XMP " || kind == "EXIF" {
			data := r.take(in, int(size))
			if data == nil {
				return
			}
			if kind == "XMP " {
				r.xmp(data)
			} else {
				r.embeddedEXIF(data)
			}
		} else {
			if _, err := io.CopyN(io.Discard, in, size); err != nil {
				r.result.Truncated = true
				r.warn("METADATA_READ_INCOMPLETE")
				return
			}
		}
		if size%2 != 0 {
			if _, err := io.CopyN(io.Discard, in, 1); err != nil {
				return
			}
		}
	}
	r.result.Truncated = true
}
func (r *displayReader) embeddedEXIF(data []byte) {
	decoded, err := exif.Decode(bytes.NewReader(data))
	if err != nil {
		r.warn("EXIF_PARSE_FAILED")
		return
	}
	walker := &metadataWalker{}
	if decoded.Walk(walker) != nil {
		return
	}
	offsets := displayOffsets(decoded)
	for i := range walker.entries {
		walker.entries[i].Source = "EXIF"
		walker.entries[i].RawValue = walker.entries[i].Value
	}
	enrichEXIFDates(walker.entries, offsets)
	// Replace legacy EXIF entries, not XMP/IPTC already read from other segments.
	kept := r.result.Entries[:0]
	for _, entry := range r.result.Entries {
		if entry.Source != "EXIF" {
			kept = append(kept, entry)
		}
	}
	r.result.Entries = kept
	sort.Slice(walker.entries, func(i, j int) bool { return walker.entries[i].Key < walker.entries[j].Key })
	for _, entry := range walker.entries {
		if len(r.result.Entries) >= maximumEntries {
			r.result.Truncated = true
			break
		}
		r.result.Entries = append(r.result.Entries, entry)
	}
}

// The legacy goexif dictionary predates OffsetTime tags. Inspect only these
// three bounded ASCII tags locally; never register a global EXIF parser.
func displayOffsets(decoded *exif.Exif) map[string]string {
	result := make(map[string]string)
	raw := decoded.Raw
	order := decoded.Tiff.Order
	if len(raw) < 8 {
		return result
	}
	offsets := []uint32{order.Uint32(raw[4:8])}
	if pointer, err := decoded.Get(exif.ExifIFDPointer); err == nil {
		if offset, err := pointer.Int64(0); err == nil && offset >= 0 && offset < int64(len(raw)) {
			offsets = append(offsets, uint32(offset))
		}
	}
	names := map[uint16]string{0x9010: "OffsetTime", 0x9011: "OffsetTimeOriginal", 0x9012: "OffsetTimeDigitized"}
	for _, offset := range offsets {
		pos := int(offset)
		if pos < 0 || pos > len(raw)-2 {
			continue
		}
		count := int(order.Uint16(raw[pos : pos+2]))
		if count > 1024 || count > (len(raw)-pos-2)/12 {
			continue
		}
		for i := 0; i < count; i++ {
			tag := raw[pos+2+i*12 : pos+2+(i+1)*12]
			name := names[order.Uint16(tag[:2])]
			n := int(order.Uint32(tag[4:8]))
			if name == "" || order.Uint16(tag[2:4]) != 2 || n < 1 || n > 8 {
				continue
			}
			value := tag[8:]
			if n > 4 {
				at := int(order.Uint32(tag[8:12]))
				if at < 0 || at > len(raw)-n {
					continue
				}
				value = raw[at : at+n]
			}
			zone := strings.TrimRight(string(value[:n]), "\x00")
			if validOffset(zone) {
				result["exif."+name] = zone
			}
		}
	}
	return result
}

// XML is tokenized with strict byte/depth/token budgets. encoding/xml does
// not resolve external entities; DTDs/directives are explicitly rejected.
type xmlNode struct {
	name     xml.Name
	text     *strings.Builder
	language string
}

func (r *displayReader) xmp(data []byte) {
	if len(data) == 0 {
		return
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var stack []xmlNode
	historyDepth, eventDepth := 0, 0
	var event map[string]string
	staged := *r.result
	staged.Entries = append([]Entry(nil), r.result.Entries...)
	staged.History = append([]HistoryEvent(nil), r.result.History...)
	original := r.result
	r.result = &staged
	success := false
	defer func() {
		r.result = original
		if success {
			*original = staged
		} else {
			original.Truncated = original.Truncated || staged.Truncated
			r.warn("XMP_PARSE_FAILED")
		}
	}()
	for count := 0; count < 100000; count++ {
		token, err := decoder.Token()
		if err == io.EOF {
			if len(stack) != 0 {
				return
			}
			success = true
			return
		}
		if err != nil {
			return
		}
		switch item := token.(type) {
		case xml.Directive:
			return
		case xml.StartElement:
			if len(stack) >= 32 {
				r.result.Truncated = true
				return
			}
			stack = append(stack, xmlNode{name: item.Name, text: &strings.Builder{}})
			if item.Name.Space == nsMM && item.Name.Local == "History" {
				historyDepth = len(stack)
			}
			if historyDepth > 0 && item.Name.Space == nsRDF && item.Name.Local == "li" {
				event = make(map[string]string)
				eventDepth = len(stack)
			}
			for _, attr := range item.Attr {
				if attr.Name.Space == nsEvent && event != nil {
					event[attr.Name.Local] = cleanValue(attr.Value)
				} else if historyDepth == 0 {
					r.xmpProperty(attr.Name, attr.Value)
				}
			}
		case xml.CharData:
			if len(stack) > 0 {
				node := &stack[len(stack)-1]
				if node.text.Len()+len(item) > 16384 {
					r.result.Truncated = true
				} else {
					node.text.Write(item)
				}
			}
		case xml.EndElement:
			if len(stack) == 0 {
				return
			}
			node := stack[len(stack)-1]
			text := strings.TrimSpace(node.text.String())
			if node.name.Space == nsEvent && event != nil {
				event[node.name.Local] = cleanValue(text)
			} else if historyDepth == 0 && text != "" {
				if node.name.Space == nsRDF && node.name.Local == "li" {
					for i := len(stack) - 2; i >= 0; i-- {
						if stack[i].name.Space != nsRDF {
							r.xmpProperty(stack[i].name, text)
							break
						}
					}
				} else {
					r.xmpProperty(node.name, text)
				}
			}
			if eventDepth == len(stack) && event != nil {
				r.history(event)
				event = nil
				eventDepth = 0
			}
			if historyDepth == len(stack) {
				historyDepth = 0
			}
			stack = stack[:len(stack)-1]
		}
	}
	r.result.Truncated = true
}

func (r *displayReader) xmpProperty(name xml.Name, value string) {
	prefix := ""
	visibility, label, group := "", "", "DESCRIPTIVE"
	switch name.Space {
	case nsXMP:
		prefix = "xmp"
		switch name.Local {
		case "CreateDate":
			visibility, label, group = settings.MetadataDateCreated, "Creation time", "DATES"
		case "ModifyDate":
			visibility, label, group = settings.MetadataDateModified, "Modified time", "DATES"
		case "MetadataDate":
			visibility, label, group = settings.MetadataDateMetadata, "Metadata modified time", "DATES"
		case "CreatorTool":
			visibility, label = settings.MetadataSoftware, "Creator tool"
		}
	case nsMM:
		prefix = "xmpMM"
		switch name.Local {
		case "PreservedFileName":
			visibility, label, group = settings.MetadataOriginalFilename, "Preserved original filename", "FILE"
		case "DocumentID", "OriginalDocumentID", "InstanceID":
			visibility, label, group = settings.MetadataDeviceIdentifiers, name.Local, "SENSITIVE"
		}
	case "http://ns.adobe.com/photoshop/1.0/":
		prefix = "photoshop"
		switch name.Local {
		case "DateCreated":
			visibility, label, group = settings.MetadataDateCreated, "Content creation time", "DATES"
		case "ICCProfile":
			visibility, label, group = settings.MetadataICC, "ICC profile", "IMAGE"
		case "Headline":
			visibility, label = settings.MetadataTitle, "Headline"
		case "Credit", "AuthorsPosition":
			visibility, label = settings.MetadataAuthor, name.Local
		case "Source":
			visibility, label = settings.MetadataSource, "Source"
		}
	case "http://purl.org/dc/elements/1.1/":
		prefix = "dc"
		switch name.Local {
		case "title":
			visibility, label = settings.MetadataTitle, "Title"
		case "description":
			visibility, label = settings.MetadataDescription, "Description"
		case "creator":
			visibility, label = settings.MetadataAuthor, "Author"
		case "rights":
			visibility, label = settings.MetadataCopyright, "Copyright"
		case "subject":
			visibility, label = settings.MetadataKeywords, "Keywords"
		case "source":
			visibility, label = settings.MetadataSource, "Source"
		}
	case "http://ns.adobe.com/exif/1.0/", "http://ns.adobe.com/tiff/1.0/", "http://ns.adobe.com/exif/1.0/aux/", "http://cipa.jp/exif/1.0/":
		prefix = "exif"
		if strings.Contains(name.Space, "tiff") {
			prefix = "tiff"
		} else if strings.Contains(name.Space, "aux") {
			prefix = "aux"
		} else if strings.Contains(name.Space, "cipa") {
			prefix = "exifEX"
		}
		switch name.Local {
		case "DateTimeOriginal":
			visibility, label, group = settings.MetadataDateOriginal, "Date taken", "DATES"
		case "DateTimeDigitized":
			visibility, label, group = settings.MetadataDateDigitized, "Digitized time", "DATES"
		case "DateTime":
			visibility, label, group = settings.MetadataDateModified, "Modified time", "DATES"
		case "Make":
			visibility, label, group = settings.MetadataCameraMake, "Camera manufacturer", "CAMERA"
		case "Model":
			visibility, label, group = settings.MetadataCameraModel, "Camera model", "CAMERA"
		case "Lens", "LensModel":
			visibility, label, group = settings.MetadataLens, "Lens", "CAMERA"
		case "Software":
			visibility, label = settings.MetadataSoftware, "Software"
		case "SerialNumber", "LensSerialNumber", "ImageUniqueID":
			visibility, label, group = settings.MetadataDeviceIdentifiers, name.Local, "SENSITIVE"
		case "GPSLatitude", "GPSLongitude", "GPSAltitude":
			visibility, label, group = settings.MetadataGPS, name.Local, "SENSITIVE"
		case "Artist":
			visibility, label = settings.MetadataAuthor, "Author"
		case "Copyright":
			visibility, label = settings.MetadataCopyright, "Copyright"
		case "ImageDescription":
			visibility, label = settings.MetadataDescription, "Description"
		}
	}
	if visibility != "" {
		r.add("xmp."+prefix+":"+name.Local, visibility, label, group, value, "XMP")
	}
}

var conversionPattern = regexp.MustCompile(`(?i)(?:converted\s+)?from\s+([a-z0-9.+-]+/[a-z0-9.+-]+)\s+to\s+([a-z0-9.+-]+/[a-z0-9.+-]+)`)

func (r *displayReader) history(value map[string]string) {
	if len(value) == 0 {
		return
	}
	if len(r.result.History) >= historyLimit {
		r.result.Truncated = true
		return
	}
	action := value["action"]
	switch action {
	case "saved", "created", "converted", "derived", "edited":
	default:
		action = "other"
	}
	parameters := ""
	if match := conversionPattern.FindStringSubmatch(value["parameters"]); len(match) == 3 {
		parameters = match[1] + " → " + match[2]
	}
	when, stamp := displayTime(value["when"])
	r.result.History = append(r.result.History, HistoryEvent{Index: len(r.result.History) + 1, Action: action, Software: cleanValue(value["softwareAgent"]), When: when, Timestamp: stamp, Parameters: parameters})
}

func (r *displayReader) photoshop(data []byte) {
	if !bytes.HasPrefix(data, []byte("Photoshop 3.0\x00")) {
		return
	}
	data = data[len("Photoshop 3.0\x00"):]
	for count := 0; len(data) >= 12 && count < 1024; count++ {
		if string(data[:4]) != "8BIM" {
			return
		}
		id := binary.BigEndian.Uint16(data[4:6])
		nameLen := int(data[6]) + 1
		if nameLen%2 != 0 {
			nameLen++
		}
		offset := 6 + nameLen
		if offset+4 > len(data) {
			r.warn("IPTC_PARSE_FAILED")
			return
		}
		size := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		offset += 4
		if size < 0 || size > len(data)-offset {
			r.warn("IPTC_PARSE_FAILED")
			return
		}
		if id == 0x0404 {
			r.iptc(data[offset : offset+size])
		}
		offset += size
		if size%2 != 0 {
			offset++
		}
		if offset > len(data) {
			return
		}
		data = data[offset:]
	}
}
func (r *displayReader) iptc(data []byte) {
	values := make(map[byte][]string)
	for count := 0; len(data) > 0 && count < 2048; count++ {
		// Photoshop may pad the IIM resource with zero bytes inside its
		// declared length. Padding is not a malformed dataset.
		if data[0] == 0 && len(bytes.Trim(data, "\x00")) == 0 {
			break
		}
		if len(data) < 5 || data[0] != 0x1c {
			r.warn("IPTC_PARSE_FAILED")
			return
		}
		record, dataset := data[1], data[2]
		size := int(binary.BigEndian.Uint16(data[3:5]))
		offset := 5
		if size&0x8000 != 0 {
			n := size & 0x7fff
			if n < 1 || n > 4 || len(data) < 5+n {
				r.warn("IPTC_PARSE_FAILED")
				return
			}
			size = 0
			for _, b := range data[5 : 5+n] {
				size = size<<8 | int(b)
			}
			offset += n
		}
		if size > len(data)-offset {
			r.warn("IPTC_PARSE_FAILED")
			return
		}
		value := data[offset : offset+size]
		data = data[offset+size:]
		if record != 2 {
			continue
		}
		if !utf8.Valid(value) {
			r.warn("IPTC_TEXT_ENCODING_UNSUPPORTED")
			continue
		}
		values[dataset] = append(values[dataset], string(value))
	}
	first := func(key byte) string {
		if len(values[key]) > 0 {
			return values[key][0]
		}
		return ""
	}
	for _, pair := range [][4]string{{"55", "60", settings.MetadataDateCreated, "Content creation time"}, {"62", "63", settings.MetadataDateDigitized, "Digitized time"}} {
		dateID, _ := strconv.Atoi(pair[0])
		timeID, _ := strconv.Atoi(pair[1])
		date, clock := first(byte(dateID)), first(byte(timeID))
		if parsed, err := time.Parse("20060102", date); err == nil {
			value := parsed.Format("2006-01-02")
			if len(clock) >= 6 && digitsOnly(clock[:6]) {
				value += " " + clock[:2] + ":" + clock[2:4] + ":" + clock[4:6]
				if len(clock) == 11 {
					value += " " + clock[6:9] + ":" + clock[9:]
				}
			}
			r.add("iptc."+pair[0]+"+"+pair[1], pair[2], pair[3], "DATES", value, "IPTC")
		}
	}
	fields := map[byte][2]string{5: {settings.MetadataTitle, "Title"}, 25: {settings.MetadataKeywords, "Keywords"}, 80: {settings.MetadataAuthor, "Author"}, 110: {settings.MetadataAuthor, "Credit"}, 115: {settings.MetadataSource, "Source"}, 116: {settings.MetadataCopyright, "Copyright"}, 120: {settings.MetadataDescription, "Description"}}
	for key, definition := range fields {
		for _, value := range values[key] {
			r.add(fmt.Sprintf("iptc.2:%d", key), definition[0], definition[1], "DESCRIPTIVE", value, "IPTC")
		}
	}
}

func (r *displayReader) icc(data []byte) {
	// Only inspect a complete, single-segment ICC profile, never binary payloads.
	if len(data) < 14 || data[12] != 1 || data[13] != 1 {
		return
	}
	profile := data[14:]
	if len(profile) < 132 {
		return
	}
	count := int(binary.BigEndian.Uint32(profile[128:132]))
	if count > 256 {
		return
	}
	for i := 0; i < count; i++ {
		pos := 132 + i*12
		if pos+12 > len(profile) {
			return
		}
		if string(profile[pos:pos+4]) != "desc" {
			continue
		}
		offset, size := int(binary.BigEndian.Uint32(profile[pos+4:pos+8])), int(binary.BigEndian.Uint32(profile[pos+8:pos+12]))
		if offset > len(profile) || size > len(profile)-offset || size < 12 {
			return
		}
		tag := profile[offset : offset+size]
		if string(tag[:4]) != "desc" {
			return
		}
		length := int(binary.BigEndian.Uint32(tag[8:12]))
		if length > 0 && length <= len(tag)-12 {
			r.add("icc.description", settings.MetadataICC, "ICC profile", "IMAGE", string(bytes.TrimRight(tag[12:12+length], "\x00")), "ICC")
		}
	}
}
