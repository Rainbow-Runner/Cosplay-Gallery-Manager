package settings

import "fmt"

// Media metadata visibility keys are stable API/settings identifiers. The
// extractor may discover many concrete EXIF tags, but every returned value is
// assigned to one of these bounded controls before it can reach Browse.
const (
	MetadataFileType          = "file.type"
	MetadataFileSize          = "file.size"
	MetadataFileDimensions    = "file.dimensions"
	MetadataFileAddedAt       = "file.added_at"
	MetadataTitle             = "descriptive.title"
	MetadataDescription       = "descriptive.description"
	MetadataAuthor            = "descriptive.author"
	MetadataSource            = "descriptive.source"
	MetadataSoftware          = "descriptive.software"
	MetadataCopyright         = "descriptive.copyright"
	MetadataKeywords          = "descriptive.keywords"
	MetadataComment           = "descriptive.comment"
	MetadataDateModified      = "date.modified"
	MetadataDateOriginal      = "date.original"
	MetadataDateDigitized     = "date.digitized"
	MetadataCameraMake        = "camera.make"
	MetadataCameraModel       = "camera.model"
	MetadataLens              = "camera.lens"
	MetadataExposureTime      = "camera.exposure_time"
	MetadataAperture          = "camera.aperture"
	MetadataISO               = "camera.iso"
	MetadataFocalLength       = "camera.focal_length"
	MetadataExposureBias      = "camera.exposure_bias"
	MetadataFlash             = "camera.flash"
	MetadataMeteringMode      = "camera.metering_mode"
	MetadataWhiteBalance      = "camera.white_balance"
	MetadataOrientation       = "image.orientation"
	MetadataColorSpace        = "image.color_space"
	MetadataResolution        = "image.resolution"
	MetadataVideoDuration     = "video.duration"
	MetadataVideoContainer    = "video.container"
	MetadataVideoCodec        = "video.codec"
	MetadataVideoFrameRate    = "video.frame_rate"
	MetadataAudioCodec        = "video.audio_codec"
	MetadataGPS               = "sensitive.gps"
	MetadataDeviceIdentifiers = "sensitive.device_identifiers"
	MetadataOtherEXIF         = "other.exif"
)

// MediaMetadataVisibilityCatalog is ordered for the Manage settings UI.
// GPS and unique device/image identifiers are supported but intentionally not
// enabled by default.
var MediaMetadataVisibilityCatalog = []string{
	MetadataFileType, MetadataFileSize, MetadataFileDimensions, MetadataFileAddedAt,
	MetadataTitle, MetadataDescription, MetadataAuthor, MetadataSource, MetadataSoftware,
	MetadataCopyright, MetadataKeywords, MetadataComment,
	MetadataDateModified, MetadataDateOriginal, MetadataDateDigitized,
	MetadataCameraMake, MetadataCameraModel, MetadataLens, MetadataExposureTime,
	MetadataAperture, MetadataISO, MetadataFocalLength, MetadataExposureBias,
	MetadataFlash, MetadataMeteringMode, MetadataWhiteBalance,
	MetadataOrientation, MetadataColorSpace, MetadataResolution,
	MetadataVideoDuration, MetadataVideoContainer, MetadataVideoCodec,
	MetadataVideoFrameRate, MetadataAudioCodec, MetadataOtherEXIF,
	MetadataGPS, MetadataDeviceIdentifiers,
}

var DefaultMediaMetadataVisibleFields = []string{
	MetadataFileType, MetadataFileSize, MetadataFileDimensions, MetadataFileAddedAt,
	MetadataTitle, MetadataDescription, MetadataAuthor, MetadataSource, MetadataSoftware,
	MetadataCopyright, MetadataKeywords, MetadataComment,
	MetadataDateModified, MetadataDateOriginal, MetadataDateDigitized,
	MetadataCameraMake, MetadataCameraModel, MetadataLens, MetadataExposureTime,
	MetadataAperture, MetadataISO, MetadataFocalLength, MetadataExposureBias,
	MetadataFlash, MetadataMeteringMode, MetadataWhiteBalance,
	MetadataOrientation, MetadataColorSpace, MetadataResolution,
	MetadataVideoDuration, MetadataVideoContainer, MetadataVideoCodec,
	MetadataVideoFrameRate, MetadataAudioCodec, MetadataOtherEXIF,
}

func ValidateMediaMetadataVisibleFields(values []string) error {
	if len(values) > len(MediaMetadataVisibilityCatalog) {
		return fmt.Errorf("too many media metadata visibility fields")
	}
	allowed := make(map[string]bool, len(MediaMetadataVisibilityCatalog))
	for _, key := range MediaMetadataVisibilityCatalog {
		allowed[key] = true
	}
	seen := make(map[string]bool, len(values))
	for _, key := range values {
		if !allowed[key] {
			return fmt.Errorf("unsupported media metadata visibility field %q", key)
		}
		if seen[key] {
			return fmt.Errorf("duplicate media metadata visibility field %q", key)
		}
		seen[key] = true
	}
	return nil
}

func MediaMetadataVisibleSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}
