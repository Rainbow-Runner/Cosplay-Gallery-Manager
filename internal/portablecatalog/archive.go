package portablecatalog

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/stashapp/stash/internal/portableid"
	"github.com/stashapp/stash/internal/product"
	_ "golang.org/x/image/webp"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	maxEntries          = 100005
	maxUncompressedSize = uint64(10 * 1024 * 1024 * 1024)
	maxJSONSize         = uint64(128 * 1024 * 1024)
	maxAssetSize        = int64(20 * 1024 * 1024)
	maxCompressionRatio = uint64(1000)
)

var requiredPayload = []string{"identity-ledger.json", "core-catalog.json", "gallery-index.json"}

type AssetSource struct {
	Path string
	Kind string
	Size int64
	Open func() (io.ReadCloser, error)
}

type IdentitySource struct {
	Count  int
	Stream func(func(IdentityRecord) error) error
}

type Inspection struct {
	Manifest  PackageManifest
	Checksums ChecksumManifest
	// Bundle.Identity retains only identity kinds referenced by the in-memory
	// core and Gallery documents. Manifest.IdentityCount is the authoritative
	// count for the fully validated streamed ledger.
	Bundle Bundle
}

func Write(writer io.Writer, bundle Bundle, assets []AssetSource) error {
	bundle.Normalize()
	identities := append([]IdentityRecord{}, bundle.Identity.Identities...)
	return writePackage(writer, bundle, assets, IdentitySource{Count: len(identities), Stream: func(yield func(IdentityRecord) error) error {
		for _, identity := range identities {
			if err := yield(identity); err != nil {
				return err
			}
		}
		return nil
	}}, true)
}

// WriteStreaming writes the identity ledger incrementally. The source must be
// ordered by UUID and repeatable only for this single invocation.
func WriteStreaming(writer io.Writer, bundle Bundle, assets []AssetSource, identities IdentitySource) error {
	return writePackage(writer, bundle, assets, identities, false)
}

func writePackage(writer io.Writer, bundle Bundle, assets []AssetSource, identities IdentitySource, validateInMemory bool) error {
	bundle.Normalize()
	bundle.Manifest.Format = Format
	bundle.Manifest.FormatVersion = FormatVersion
	if validateInMemory {
		if err := bundle.Validate(); err != nil {
			return err
		}
	} else {
		if identities.Count < 0 || identities.Stream == nil || bundle.Manifest.ProductID != product.ID || bundle.Identity.SchemaVersion != 1 || bundle.Catalog.SchemaVersion != 1 || bundle.Gallery.SchemaVersion != 1 || !canonicalTime(bundle.Manifest.CreatedAt) {
			return errors.New("invalid streaming portable metadata source")
		}
		if _, err := portableid.Parse(bundle.Manifest.ExportID); err != nil {
			return fmt.Errorf("invalid streaming portable export ID: %w", err)
		}
	}
	if len(assets)+len(requiredPayload)+2 > maxEntries {
		return errors.New("portable metadata package has too many entries")
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].Path < assets[j].Path })
	seen := map[string]bool{}
	requiredAssets := map[string]string{}
	for _, coser := range bundle.Catalog.Cosers {
		for _, asset := range []*AssetRef{coser.Avatar, coser.Banner} {
			if asset != nil {
				requiredAssets[asset.PackagePath] = "COSER_" + asset.Kind + "_ORIGINAL"
			}
		}
	}
	for _, asset := range assets {
		if !safeArchiveName(asset.Path) || !strings.HasPrefix(asset.Path, "coser-assets/") || asset.Size < 1 || asset.Size > maxAssetSize || asset.Open == nil {
			return fmt.Errorf("invalid portable asset %q", asset.Path)
		}
		folded := cases.Fold().String(asset.Path)
		if seen[folded] {
			return fmt.Errorf("duplicate portable asset %q", asset.Path)
		}
		if requiredAssets[asset.Path] != asset.Kind {
			return fmt.Errorf("portable asset %q is unreferenced or has the wrong kind", asset.Path)
		}
		seen[folded] = true
		delete(requiredAssets, asset.Path)
	}
	if len(requiredAssets) != 0 {
		return errors.New("portable metadata package is missing a referenced Coser asset")
	}

	archive := zip.NewWriter(writer)
	closed := false
	defer func() {
		if !closed {
			_ = archive.Close()
		}
	}()
	checksums := ChecksumManifest{Algorithm: "SHA-256", Files: []ChecksumEntry{}}
	createdAt, _ := time.Parse(time.RFC3339, bundle.Manifest.CreatedAt)

	identityEntry, err := writeIdentityLedger(archive, identities, createdAt)
	if err != nil {
		return err
	}
	checksums.Files = append(checksums.Files, identityEntry)
	for _, item := range []struct {
		name, kind string
		value      any
	}{
		{"core-catalog.json", "CORE_CATALOG", bundle.Catalog},
		{"gallery-index.json", "GALLERY_INDEX", bundle.Gallery},
	} {
		data, err := canonicalJSON(item.value)
		if err != nil {
			return err
		}
		if uint64(len(data)) > maxJSONSize {
			return fmt.Errorf("%s exceeds portable JSON limit", item.name)
		}
		if err := writeBytes(archive, item.name, data, createdAt); err != nil {
			return err
		}
		checksums.Files = append(checksums.Files, checksumEntry(item.name, item.kind, data))
	}
	for _, asset := range assets {
		entry, err := writeAsset(archive, asset, createdAt)
		if err != nil {
			return err
		}
		checksums.Files = append(checksums.Files, entry)
	}

	checksumsData, err := canonicalJSON(checksums)
	if err != nil {
		return err
	}
	if err := writeBytes(archive, "checksums.json", checksumsData, createdAt); err != nil {
		return err
	}
	bundle.Manifest.ChecksumsSHA256 = sha256Hex(checksumsData)
	bundle.Manifest.IdentityCount = identities.Count
	bundle.Manifest.CoserCount = len(bundle.Catalog.Cosers)
	bundle.Manifest.WorkCount = len(bundle.Catalog.Works)
	bundle.Manifest.CharacterCount = len(bundle.Catalog.Characters)
	bundle.Manifest.TagCount = len(bundle.Catalog.Tags)
	bundle.Manifest.AccountCount = len(bundle.Catalog.Accounts)
	bundle.Manifest.GalleryCount = len(bundle.Gallery.Galleries)
	bundle.Manifest.AssetCount = len(assets)
	manifestData, err := canonicalJSON(bundle.Manifest)
	if err != nil {
		return err
	}
	if err := writeBytes(archive, "package.json", manifestData, createdAt); err != nil {
		return err
	}
	if err := archive.Close(); err != nil {
		return err
	}
	closed = true
	return nil
}

func InspectFile(ctx context.Context, filename string) (Inspection, error) {
	archive, err := zip.OpenReader(filename)
	if err != nil {
		return Inspection{}, fmt.Errorf("opening portable metadata package: %w", err)
	}
	defer archive.Close()
	if len(archive.File) > maxEntries {
		return Inspection{}, errors.New("portable metadata package has too many entries")
	}
	entries := make(map[string]*zip.File, len(archive.File))
	folded := map[string]string{}
	var total uint64
	for _, entry := range archive.File {
		if err := ctx.Err(); err != nil {
			return Inspection{}, err
		}
		if entry.FileInfo().IsDir() || !entry.Mode().IsRegular() || !safeArchiveName(entry.Name) {
			return Inspection{}, fmt.Errorf("unsafe portable metadata entry %q", entry.Name)
		}
		key := cases.Fold().String(entry.Name)
		if prior := folded[key]; prior != "" {
			return Inspection{}, fmt.Errorf("portable metadata entry collision %q and %q", prior, entry.Name)
		}
		folded[key] = entry.Name
		if entry.UncompressedSize64 > maxUncompressedSize || total > maxUncompressedSize-entry.UncompressedSize64 {
			return Inspection{}, errors.New("portable metadata package exceeds size limit")
		}
		total += entry.UncompressedSize64
		if entry.UncompressedSize64 > 1024*1024 && (entry.CompressedSize64 == 0 || entry.UncompressedSize64/entry.CompressedSize64 > maxCompressionRatio) {
			return Inspection{}, fmt.Errorf("portable metadata entry %q has unsafe compression ratio", entry.Name)
		}
		if entry.Name != "package.json" && entry.Name != "checksums.json" && !contains(requiredPayload, entry.Name) && !strings.HasPrefix(entry.Name, "coser-assets/") {
			return Inspection{}, fmt.Errorf("unknown portable metadata entry %q", entry.Name)
		}
		entries[entry.Name] = entry
	}
	for _, name := range append([]string{"package.json", "checksums.json"}, requiredPayload...) {
		if entries[name] == nil {
			return Inspection{}, fmt.Errorf("portable metadata package is missing %s", name)
		}
	}

	var result Inspection
	manifestData, err := readEntry(entries["package.json"], maxJSONSize)
	if err != nil {
		return Inspection{}, err
	}
	if err := strictJSON(manifestData, &result.Manifest); err != nil {
		return Inspection{}, fmt.Errorf("decoding package.json: %w", err)
	}
	checksumsData, err := readEntry(entries["checksums.json"], maxJSONSize)
	if err != nil {
		return Inspection{}, err
	}
	if sha256Hex(checksumsData) != result.Manifest.ChecksumsSHA256 {
		return Inspection{}, errors.New("checksums.json digest mismatch")
	}
	if err := strictJSON(checksumsData, &result.Checksums); err != nil {
		return Inspection{}, fmt.Errorf("decoding checksums.json: %w", err)
	}
	if result.Checksums.Algorithm != "SHA-256" {
		return Inspection{}, errors.New("unsupported checksum algorithm")
	}

	listed := make(map[string]bool, len(result.Checksums.Files))
	for _, expected := range result.Checksums.Files {
		if !safeArchiveName(expected.Path) || expected.Path == "package.json" || expected.Path == "checksums.json" || listed[expected.Path] {
			return Inspection{}, errors.New("invalid checksum manifest entry")
		}
		entry := entries[expected.Path]
		if entry == nil || int64(entry.UncompressedSize64) != expected.Size {
			return Inspection{}, fmt.Errorf("portable metadata entry %q size mismatch", expected.Path)
		}
		actual, err := hashEntry(ctx, entry)
		if err != nil {
			return Inspection{}, err
		}
		if actual != expected.SHA256 {
			return Inspection{}, fmt.Errorf("portable metadata entry %q digest mismatch", expected.Path)
		}
		if strings.HasPrefix(expected.Path, "coser-assets/") {
			if err := validateCoserAssetEntry(entry); err != nil {
				return Inspection{}, fmt.Errorf("portable metadata entry %q: %w", expected.Path, err)
			}
		}
		listed[expected.Path] = true
	}
	for name := range entries {
		if name != "package.json" && name != "checksums.json" && !listed[name] {
			return Inspection{}, fmt.Errorf("portable metadata entry %q is absent from checksums", name)
		}
	}

	catalogData, err := readEntry(entries["core-catalog.json"], maxJSONSize)
	if err != nil {
		return Inspection{}, err
	}
	galleryData, err := readEntry(entries["gallery-index.json"], maxJSONSize)
	if err != nil {
		return Inspection{}, err
	}
	if err := strictJSON(catalogData, &result.Bundle.Catalog); err != nil {
		return Inspection{}, fmt.Errorf("decoding core catalog: %w", err)
	}
	if err := strictJSON(galleryData, &result.Bundle.Gallery); err != nil {
		return Inspection{}, fmt.Errorf("decoding Gallery index: %w", err)
	}
	identityCount := 0
	result.Bundle.Identity, identityCount, err = inspectIdentityLedger(ctx, entries["identity-ledger.json"], nil)
	if err != nil {
		return Inspection{}, fmt.Errorf("decoding identity ledger: %w", err)
	}
	result.Bundle.Manifest = result.Manifest
	if err := result.Bundle.Validate(); err != nil {
		return Inspection{}, err
	}
	assetChecksums := map[string]string{}
	for _, value := range result.Checksums.Files {
		if strings.HasPrefix(value.Path, "coser-assets/") {
			assetChecksums[value.Path] = value.Kind
		}
	}
	referencedAssets := map[string]string{}
	for _, coser := range result.Bundle.Catalog.Cosers {
		for _, asset := range []*AssetRef{coser.Avatar, coser.Banner} {
			if asset == nil {
				continue
			}
			expectedKind := "COSER_" + asset.Kind + "_ORIGINAL"
			if assetChecksums[asset.PackagePath] != expectedKind {
				return Inspection{}, fmt.Errorf("Coser asset %q is missing or has the wrong kind", asset.PackagePath)
			}
			if referencedAssets[asset.PackagePath] != "" {
				return Inspection{}, fmt.Errorf("Coser asset %q is referenced more than once", asset.PackagePath)
			}
			referencedAssets[asset.PackagePath] = expectedKind
		}
	}
	if len(referencedAssets) != len(assetChecksums) {
		return Inspection{}, errors.New("portable metadata package contains an unreferenced Coser asset")
	}
	if result.Manifest.IdentityCount != identityCount || result.Manifest.CoserCount != len(result.Bundle.Catalog.Cosers) || result.Manifest.WorkCount != len(result.Bundle.Catalog.Works) || result.Manifest.CharacterCount != len(result.Bundle.Catalog.Characters) || result.Manifest.TagCount != len(result.Bundle.Catalog.Tags) || result.Manifest.AccountCount != len(result.Bundle.Catalog.Accounts) || result.Manifest.GalleryCount != len(result.Bundle.Gallery.Galleries) || result.Manifest.AssetCount != len(result.Checksums.Files)-len(requiredPayload) {
		return Inspection{}, errors.New("portable metadata package count mismatch")
	}
	return result, nil
}

func validateCoserAssetEntry(entry *zip.File) error {
	if entry.UncompressedSize64 == 0 || entry.UncompressedSize64 > uint64(maxAssetSize) {
		return errors.New("Coser asset exceeds size limit")
	}
	reader, err := entry.Open()
	if err != nil {
		return err
	}
	defer reader.Close()
	return ValidateCoserAsset(reader, int64(entry.UncompressedSize64))
}

// ValidateCoserAsset applies the same bounded image checks used by the offline
// inspector without generating derivatives or retaining the source.
func ValidateCoserAsset(reader io.Reader, size int64) error {
	if reader == nil || size < 1 || size > maxAssetSize {
		return errors.New("Coser asset exceeds size limit")
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxAssetSize+1))
	if err != nil {
		return err
	}
	if int64(len(data)) != size {
		return errors.New("Coser asset size changed while reading")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return errors.New("Coser asset is not a supported image")
	}
	if format != "jpeg" && format != "png" && format != "webp" {
		return errors.New("Coser asset must be JPEG, PNG, or static WebP")
	}
	if config.Width <= 0 || config.Height <= 0 || uint64(config.Width) > 50_000_000/uint64(config.Height) {
		return errors.New("Coser asset exceeds 50 megapixels")
	}
	animated, err := portableAnimatedImage(bytes.NewReader(data), format)
	if err != nil {
		return err
	}
	if animated {
		return errors.New("Coser asset must be static")
	}
	return nil
}

func portableAnimatedImage(reader io.Reader, format string) (bool, error) {
	switch format {
	case "png":
		buffered := bufio.NewReader(reader)
		signature := make([]byte, 8)
		if _, err := io.ReadFull(buffered, signature); err != nil || !bytes.Equal(signature, []byte("\x89PNG\r\n\x1a\n")) {
			return false, errors.New("invalid PNG")
		}
		for {
			var header [8]byte
			if _, err := io.ReadFull(buffered, header[:]); err != nil {
				return false, err
			}
			length := binary.BigEndian.Uint32(header[:4])
			chunk := string(header[4:])
			if chunk == "acTL" {
				return true, nil
			}
			if length > uint32(maxAssetSize) {
				return false, errors.New("invalid PNG chunk")
			}
			if _, err := io.CopyN(io.Discard, buffered, int64(length)+4); err != nil {
				return false, err
			}
			if chunk == "IDAT" || chunk == "IEND" {
				return false, nil
			}
		}
	case "webp":
		var header [12]byte
		if _, err := io.ReadFull(reader, header[:]); err != nil || string(header[:4]) != "RIFF" || string(header[8:]) != "WEBP" {
			return false, errors.New("invalid WebP")
		}
		for {
			var chunkHeader [8]byte
			if _, err := io.ReadFull(reader, chunkHeader[:]); errors.Is(err, io.EOF) {
				return false, nil
			} else if err != nil {
				return false, err
			}
			length := binary.LittleEndian.Uint32(chunkHeader[4:])
			chunk := string(chunkHeader[:4])
			if chunk == "ANIM" || chunk == "ANMF" {
				return true, nil
			}
			if length > uint32(maxAssetSize) {
				return false, errors.New("invalid WebP chunk")
			}
			skip := int64(length)
			if skip%2 == 1 {
				skip++
			}
			if _, err := io.CopyN(io.Discard, reader, skip); err != nil {
				return false, err
			}
		}
	default:
		return false, nil
	}
}

func writeAsset(archive *zip.Writer, asset AssetSource, createdAt time.Time) (ChecksumEntry, error) {
	header := &zip.FileHeader{Name: asset.Path, Method: zip.Deflate}
	header.SetMode(0o600)
	header.SetModTime(createdAt)
	destination, err := archive.CreateHeader(header)
	if err != nil {
		return ChecksumEntry{}, err
	}
	source, err := asset.Open()
	if err != nil {
		return ChecksumEntry{}, err
	}
	defer source.Close()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(destination, hash), io.LimitReader(source, maxAssetSize+1))
	if err != nil {
		return ChecksumEntry{}, err
	}
	if written != asset.Size || written > maxAssetSize {
		return ChecksumEntry{}, fmt.Errorf("portable asset %q changed while exporting", asset.Path)
	}
	one := make([]byte, 1)
	if count, readErr := source.Read(one); count != 0 || readErr != io.EOF {
		return ChecksumEntry{}, fmt.Errorf("portable asset %q changed while exporting", asset.Path)
	}
	return ChecksumEntry{Path: asset.Path, Size: written, SHA256: hex.EncodeToString(hash.Sum(nil)), Kind: asset.Kind}, nil
}

type countingWriter struct {
	writer io.Writer
	count  int64
}

func (w *countingWriter) Write(data []byte) (int, error) {
	written, err := w.writer.Write(data)
	w.count += int64(written)
	return written, err
}

func writeIdentityLedger(archive *zip.Writer, source IdentitySource, createdAt time.Time) (ChecksumEntry, error) {
	header := &zip.FileHeader{Name: "identity-ledger.json", Method: zip.Deflate}
	header.SetMode(0o600)
	header.SetModTime(createdAt)
	destination, err := archive.CreateHeader(header)
	if err != nil {
		return ChecksumEntry{}, err
	}
	hash := sha256.New()
	counter := &countingWriter{writer: io.MultiWriter(destination, hash)}
	if _, err := io.WriteString(counter, "{\n  \"schema_version\": 1,\n  \"identities\": [\n"); err != nil {
		return ChecksumEntry{}, err
	}
	count := 0
	previous := ""
	err = source.Stream(func(identity IdentityRecord) error {
		if err := validateStreamIdentity(identity); err != nil {
			return err
		}
		if previous != "" && identity.UUID <= previous {
			return errors.New("portable identities must be strictly ordered by UUID")
		}
		previous = identity.UUID
		data, err := json.Marshal(identity)
		if err != nil {
			return err
		}
		if count > 0 {
			if _, err := io.WriteString(counter, ",\n"); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(counter, "    "); err != nil {
			return err
		}
		if _, err := counter.Write(data); err != nil {
			return err
		}
		count++
		if uint64(counter.count) > maxUncompressedSize {
			return errors.New("identity-ledger.json exceeds portable size limit")
		}
		return nil
	})
	if err != nil {
		return ChecksumEntry{}, err
	}
	if count != source.Count {
		return ChecksumEntry{}, fmt.Errorf("identity source count mismatch: expected %d, wrote %d", source.Count, count)
	}
	if _, err := io.WriteString(counter, "\n  ]\n}\n"); err != nil {
		return ChecksumEntry{}, err
	}
	return ChecksumEntry{Path: "identity-ledger.json", Size: counter.count, SHA256: hex.EncodeToString(hash.Sum(nil)), Kind: "IDENTITY_LEDGER"}, nil
}

func validateStreamIdentity(value IdentityRecord) error {
	if _, err := portableid.Parse(value.UUID); err != nil {
		return fmt.Errorf("invalid identity UUID: %w", err)
	}
	if err := portableid.ValidateKind(portableid.Kind(value.Kind)); err != nil {
		return err
	}
	if !canonicalNanoTime(value.CreatedAt) || !norm.NFC.IsNormalString(value.Reason) {
		return fmt.Errorf("identity %s has invalid metadata", value.UUID)
	}
	switch value.State {
	case "ACTIVE":
		if value.TargetUUID != "" || value.RetiredAt != "" || value.Reason != "" {
			return fmt.Errorf("active identity %s has retirement data", value.UUID)
		}
	case "ALIAS":
		if _, err := portableid.Parse(value.TargetUUID); err != nil || !canonicalNanoTime(value.RetiredAt) || value.Reason != "" {
			return fmt.Errorf("alias identity %s is invalid", value.UUID)
		}
	case "TOMBSTONE":
		if value.TargetUUID != "" || !canonicalNanoTime(value.RetiredAt) {
			return fmt.Errorf("tombstone identity %s is invalid", value.UUID)
		}
	default:
		return fmt.Errorf("identity %s has unsupported state", value.UUID)
	}
	return nil
}

func writeBytes(archive *zip.Writer, name string, data []byte, createdAt time.Time) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(0o600)
	header.SetModTime(createdAt)
	destination, err := archive.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = destination.Write(data)
	return err
}

func checksumEntry(name, kind string, data []byte) ChecksumEntry {
	return ChecksumEntry{Path: name, Size: int64(len(data)), SHA256: sha256Hex(data), Kind: kind}
}
func sha256Hex(data []byte) string { value := sha256.Sum256(data); return hex.EncodeToString(value[:]) }
func canonicalJSON(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
func safeArchiveName(name string) bool {
	if name == "" || !utf8.ValidString(name) || !norm.NFC.IsNormalString(name) || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || path.Clean(name) != name || name == "." || name == ".." || strings.HasPrefix(name, "../") || len([]rune(name)) > 4096 {
		return false
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || len([]rune(segment)) > 255 || strings.HasSuffix(segment, " ") || strings.HasSuffix(segment, ".") {
			return false
		}
		for _, character := range segment {
			if character < 0x20 || character == 0x7f || strings.ContainsRune(`<>:"|?*`, character) {
				return false
			}
		}
		base := strings.ToUpper(strings.SplitN(segment, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
			return false
		}
	}
	return true
}
func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func strictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func readEntry(entry *zip.File, limit uint64) ([]byte, error) {
	if entry.UncompressedSize64 > limit {
		return nil, fmt.Errorf("portable metadata entry %q exceeds limit", entry.Name)
	}
	reader, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if uint64(len(data)) > limit {
		return nil, fmt.Errorf("portable metadata entry %q exceeds limit", entry.Name)
	}
	return data, nil
}

func hashEntry(ctx context.Context, entry *zip.File) (string, error) {
	reader, err := entry.Open()
	if err != nil {
		return "", err
	}
	defer reader.Close()
	hash := sha256.New()
	buffer := make([]byte, 128*1024)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		count, readErr := reader.Read(buffer)
		if count > 0 {
			_, _ = hash.Write(buffer[:count])
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func WriteFileAtomic(filename string, bundle Bundle, assets []AssetSource) error {
	return writeFileAtomic(filename, func(writer io.Writer) error {
		return Write(writer, bundle, assets)
	}, len(bundle.Identity.Identities))
}

func WriteFileAtomicStreaming(filename string, bundle Bundle, assets []AssetSource, identities IdentitySource) error {
	return writeFileAtomic(filename, func(writer io.Writer) error {
		return WriteStreaming(writer, bundle, assets, identities)
	}, identities.Count)
}

func writeFileAtomic(filename string, write func(io.Writer) error, identityCount int) error {
	if filename == "" {
		return errors.New("portable metadata target is required")
	}
	if write == nil || identityCount < 0 {
		return errors.New("portable metadata writer is invalid")
	}
	if _, err := os.Stat(filename); err == nil {
		return errors.New("portable metadata target already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(filename), ".cgm-portable-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryName)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if err := write(temporary); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	// A same-directory hard link publishes the completed inode atomically and
	// fails if another process created the requested target after our preflight.
	// Unlike os.Rename on POSIX it never overwrites that competing file.
	if err := os.Link(temporaryName, filename); err != nil {
		return err
	}
	_ = os.Remove(temporaryName)
	if directory, err := os.Open(filepath.Dir(filename)); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}
