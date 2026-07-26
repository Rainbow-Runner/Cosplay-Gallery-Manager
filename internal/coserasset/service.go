package coserasset

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/stashapp/stash/internal/coreentity"
	"github.com/stashapp/stash/internal/manifest"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/portableid"
	_ "golang.org/x/image/webp"
)

const (
	MaxUploadBytes = int64(20 * 1024 * 1024)
	MaxImagePixels = uint64(50_000_000)
)

type Service struct {
	Database *productdb.Database
	Root     string
	Now      func() time.Time
}

type UploadInput struct {
	CoserUUID        string
	ExpectedRevision int64
	Kind             productdb.CoserAssetKind
	Reader           io.Reader
	AvatarCrop       *coreentity.AvatarCrop
	BannerFocalPoint *coreentity.FocalPoint
}

func (s Service) Upload(ctx context.Context, input UploadInput) (coreentity.Coser, error) {
	if s.Database == nil || input.Reader == nil || input.ExpectedRevision <= 0 {
		return coreentity.Coser{}, errors.New("invalid Coser asset upload")
	}
	if _, err := portableid.Parse(input.CoserUUID); err != nil {
		return coreentity.Coser{}, err
	}
	if input.Kind != productdb.CoserAssetAvatar && input.Kind != productdb.CoserAssetBanner {
		return coreentity.Coser{}, errors.New("unsupported Coser asset kind")
	}
	if _, err := s.Database.CoreEntities().FindCoser(ctx, input.CoserUUID); err != nil {
		return coreentity.Coser{}, err
	}
	assets, err := ensureAssetsDirectory(s.Root, input.CoserUUID)
	if err != nil {
		return coreentity.Coser{}, err
	}
	temporary, err := os.CreateTemp(assets, ".coser-upload-*.tmp")
	if err != nil {
		return coreentity.Coser{}, err
	}
	temporaryPath := temporary.Name()
	publishedOriginal := false
	defer func() {
		_ = temporary.Close()
		if !publishedOriginal {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return coreentity.Coser{}, err
	}
	written, err := io.Copy(temporary, io.LimitReader(input.Reader, MaxUploadBytes+1))
	if err != nil {
		return coreentity.Coser{}, err
	}
	if written == 0 || written > MaxUploadBytes {
		return coreentity.Coser{}, fmt.Errorf("Coser managed image must contain 1 to %d bytes", MaxUploadBytes)
	}
	if err := temporary.Sync(); err != nil {
		return coreentity.Coser{}, err
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return coreentity.Coser{}, err
	}
	config, format, err := image.DecodeConfig(temporary)
	if err != nil {
		return coreentity.Coser{}, errors.New("Coser managed asset is not a supported image")
	}
	extension, err := acceptedExtension(format)
	if err != nil {
		return coreentity.Coser{}, err
	}
	if config.Width <= 0 || config.Height <= 0 || uint64(config.Width) > MaxImagePixels/uint64(config.Height) {
		return coreentity.Coser{}, errors.New("Coser managed image exceeds 50 megapixels")
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return coreentity.Coser{}, err
	}
	if animated, err := isAnimated(temporary, format); err != nil {
		return coreentity.Coser{}, err
	} else if animated {
		return coreentity.Coser{}, errors.New("Coser managed image must be static")
	}
	if err := temporary.Close(); err != nil {
		return coreentity.Coser{}, err
	}

	identifier := portableid.New()
	prefix := strings.ToLower(string(input.Kind))
	originalName := prefix + "-" + identifier + "." + extension
	originalPath := filepath.Join(assets, originalName)
	if err := os.Rename(temporaryPath, originalPath); err != nil {
		return coreentity.Coser{}, err
	}
	publishedOriginal = true
	relativeOriginal := filepath.ToSlash(filepath.Join("assets", originalName))

	decoded, err := imaging.Open(originalPath, imaging.AutoOrientation(true))
	if err != nil {
		return coreentity.Coser{}, errors.New("Coser managed asset could not be decoded")
	}
	switch input.Kind {
	case productdb.CoserAssetAvatar:
		if err := writeJPEGAtomic(filepath.Join(assets, derivativeBase(originalName)+"-480.jpg"), avatarDerivative(decoded, input.AvatarCrop, 480)); err != nil {
			return coreentity.Coser{}, err
		}
	case productdb.CoserAssetBanner:
		focal := input.BannerFocalPoint
		if focal == nil {
			focal = &coreentity.FocalPoint{X: 0.5, Y: 0.5}
		}
		for _, width := range []int{960, 1600} {
			if err := writeJPEGAtomic(filepath.Join(assets, fmt.Sprintf("%s-%d.jpg", derivativeBase(originalName), width)), bannerDerivative(decoded, focal, width)); err != nil {
				return coreentity.Coser{}, err
			}
		}
	}
	if err := syncDirectory(assets); err != nil {
		return coreentity.Coser{}, err
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	return s.Database.CoreEntities().SetCoserManagedAsset(ctx, input.CoserUUID, input.ExpectedRevision, productdb.CoserManagedAssetInput{
		Kind: input.Kind, RelativePath: relativeOriginal, AvatarCrop: input.AvatarCrop, BannerFocalPoint: input.BannerFocalPoint,
	}, now)
}

func acceptedExtension(format string) (string, error) {
	switch format {
	case "jpeg":
		return "jpg", nil
	case "png":
		return "png", nil
	case "webp":
		return "webp", nil
	default:
		return "", errors.New("Coser managed assets allow only JPEG, PNG, or static WebP")
	}
}

func ensureAssetsDirectory(root, coserUUID string) (string, error) {
	manifestPath, err := manifest.EnsureCoserDirectory(root, coserUUID)
	if err != nil {
		return "", err
	}
	assets := filepath.Join(filepath.Dir(manifestPath), "assets")
	if err := os.Mkdir(assets, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	info, err := os.Lstat(assets)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", errors.New("Coser assets directory must be a real directory")
	}
	return assets, nil
}

func derivativeBase(originalName string) string {
	return strings.TrimSuffix(originalName, filepath.Ext(originalName))
}

func DerivativeRelative(originalRelative, variant string) (string, error) {
	if err := manifest.ValidateManagedRelativeAsset(originalRelative); err != nil {
		return "", err
	}
	base := derivativeBase(filepath.Base(originalRelative))
	switch variant {
	case "avatar-480":
		return filepath.ToSlash(filepath.Join("assets", base+"-480.jpg")), nil
	case "banner-960":
		return filepath.ToSlash(filepath.Join("assets", base+"-960.jpg")), nil
	case "banner-1600":
		return filepath.ToSlash(filepath.Join("assets", base+"-1600.jpg")), nil
	default:
		return "", errors.New("unsupported Coser asset variant")
	}
}

func OpenManaged(root, coserUUID, relative string) (*os.File, os.FileInfo, error) {
	if _, err := portableid.Parse(coserUUID); err != nil {
		return nil, nil, err
	}
	if err := manifest.ValidateManagedRelativeAsset(relative); err != nil {
		return nil, nil, err
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, err
	}
	target := filepath.Join(absoluteRoot, coserUUID, filepath.FromSlash(relative))
	within, err := filepath.Rel(absoluteRoot, target)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return nil, nil, errors.New("Coser managed asset escaped metadata root")
	}
	for current := filepath.Dir(target); ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return nil, nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return nil, nil, errors.New("Coser managed asset parent is not a real directory")
		}
		if current == absoluteRoot {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil, nil, errors.New("Coser managed asset parent escaped root")
		}
	}
	info, err := os.Lstat(target)
	if err != nil {
		return nil, nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, nil, errors.New("Coser managed asset is not a regular file")
	}
	file, err := os.Open(target)
	if err != nil {
		return nil, nil, err
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		_ = file.Close()
		if err == nil {
			err = errors.New("Coser managed asset changed while opening")
		}
		return nil, nil, err
	}
	return file, opened, nil
}

func avatarDerivative(source image.Image, crop *coreentity.AvatarCrop, size int) image.Image {
	bounds := source.Bounds()
	selected := source
	if crop != nil {
		x := bounds.Min.X + int(crop.X*float64(bounds.Dx()))
		y := bounds.Min.Y + int(crop.Y*float64(bounds.Dy()))
		width := max(1, int(crop.Size*float64(bounds.Dx())))
		height := max(1, int(crop.Size*float64(bounds.Dy())))
		if x+width > bounds.Max.X {
			width = bounds.Max.X - x
		}
		if y+height > bounds.Max.Y {
			height = bounds.Max.Y - y
		}
		selected = imaging.Crop(source, image.Rect(x, y, x+width, y+height))
	}
	return imaging.Fill(selected, size, size, imaging.Center, imaging.Lanczos)
}

func bannerDerivative(source image.Image, focal *coreentity.FocalPoint, width int) image.Image {
	height := max(1, width/3)
	bounds := source.Bounds()
	targetRatio := float64(width) / float64(height)
	cropWidth, cropHeight := bounds.Dx(), bounds.Dy()
	if float64(cropWidth)/float64(cropHeight) > targetRatio {
		cropWidth = int(float64(cropHeight) * targetRatio)
	} else {
		cropHeight = int(float64(cropWidth) / targetRatio)
	}
	centerX := int(focal.X * float64(bounds.Dx()))
	centerY := int(focal.Y * float64(bounds.Dy()))
	left := min(max(0, centerX-cropWidth/2), bounds.Dx()-cropWidth)
	top := min(max(0, centerY-cropHeight/2), bounds.Dy()-cropHeight)
	cropped := imaging.Crop(source, image.Rect(bounds.Min.X+left, bounds.Min.Y+top, bounds.Min.X+left+cropWidth, bounds.Min.Y+top+cropHeight))
	return imaging.Resize(cropped, width, height, imaging.Lanczos)
}

func writeJPEGAtomic(target string, value image.Image) error {
	temporary, err := os.CreateTemp(filepath.Dir(target), ".coser-derivative-*.tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(name)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if err := imaging.Encode(temporary, value, imaging.JPEG, imaging.JPEGQuality(90)); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, target)
}

func syncDirectory(directory string) error {
	value, err := os.Open(directory)
	if err != nil {
		return err
	}
	err = value.Sync()
	closeErr := value.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func isAnimated(reader io.ReadSeeker, format string) (bool, error) {
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	switch format {
	case "png":
		return animatedPNG(reader)
	case "webp":
		return animatedWebP(reader)
	default:
		return false, nil
	}
}

func animatedPNG(reader io.Reader) (bool, error) {
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
		if length > uint32(MaxUploadBytes) {
			return false, errors.New("invalid PNG chunk")
		}
		if _, err := io.CopyN(io.Discard, buffered, int64(length)+4); err != nil {
			return false, err
		}
		if chunk == "IDAT" || chunk == "IEND" {
			return false, nil
		}
	}
}

func animatedWebP(reader io.Reader) (bool, error) {
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
		if length > uint32(MaxUploadBytes) {
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
}
