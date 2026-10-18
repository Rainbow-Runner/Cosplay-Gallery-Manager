package imagemetadata

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/rwcarlsen/goexif/exif"
	"github.com/rwcarlsen/goexif/tiff"
	"github.com/stashapp/stash/internal/settings"
)

const (
	maximumMetadataReadBytes = 32 << 20
	maximumEntries           = 192
	maximumTagCount          = 64
	maximumValueRunes        = 1024
)

type fieldDefinition struct {
	visibility string
	label      string
	group      string
}

var knownFields = map[exif.FieldName]fieldDefinition{
	exif.XPTitle:               {settings.MetadataTitle, "Title", "DESCRIPTIVE"},
	exif.ImageDescription:      {settings.MetadataDescription, "Description", "DESCRIPTIVE"},
	exif.XPSubject:             {settings.MetadataDescription, "Subject", "DESCRIPTIVE"},
	exif.Artist:                {settings.MetadataAuthor, "Author", "DESCRIPTIVE"},
	exif.XPAuthor:              {settings.MetadataAuthor, "Author", "DESCRIPTIVE"},
	exif.FileSource:            {settings.MetadataSource, "File source", "DESCRIPTIVE"},
	exif.Software:              {settings.MetadataSoftware, "Software", "DESCRIPTIVE"},
	exif.Copyright:             {settings.MetadataCopyright, "Copyright", "DESCRIPTIVE"},
	exif.XPKeywords:            {settings.MetadataKeywords, "Keywords", "DESCRIPTIVE"},
	exif.UserComment:           {settings.MetadataComment, "Comment", "DESCRIPTIVE"},
	exif.XPComment:             {settings.MetadataComment, "Comment", "DESCRIPTIVE"},
	exif.DateTime:              {settings.MetadataDateModified, "Modified date", "DATES"},
	exif.DateTimeOriginal:      {settings.MetadataDateOriginal, "Date taken", "DATES"},
	exif.DateTimeDigitized:     {settings.MetadataDateDigitized, "Digitized date", "DATES"},
	exif.Make:                  {settings.MetadataCameraMake, "Camera manufacturer", "CAMERA"},
	exif.Model:                 {settings.MetadataCameraModel, "Camera model", "CAMERA"},
	exif.LensMake:              {settings.MetadataLens, "Lens manufacturer", "CAMERA"},
	exif.LensModel:             {settings.MetadataLens, "Lens model", "CAMERA"},
	exif.ExposureTime:          {settings.MetadataExposureTime, "Exposure time", "CAMERA"},
	exif.FNumber:               {settings.MetadataAperture, "Aperture", "CAMERA"},
	exif.ISOSpeedRatings:       {settings.MetadataISO, "ISO speed", "CAMERA"},
	exif.FocalLength:           {settings.MetadataFocalLength, "Focal length", "CAMERA"},
	exif.FocalLengthIn35mmFilm: {settings.MetadataFocalLength, "35 mm focal length", "CAMERA"},
	exif.ExposureBiasValue:     {settings.MetadataExposureBias, "Exposure bias", "CAMERA"},
	exif.Flash:                 {settings.MetadataFlash, "Flash", "CAMERA"},
	exif.MeteringMode:          {settings.MetadataMeteringMode, "Metering mode", "CAMERA"},
	exif.WhiteBalance:          {settings.MetadataWhiteBalance, "White balance", "CAMERA"},
	exif.Orientation:           {settings.MetadataOrientation, "Orientation", "IMAGE"},
	exif.ColorSpace:            {settings.MetadataColorSpace, "Color space", "IMAGE"},
	exif.XResolution:           {settings.MetadataResolution, "Horizontal resolution", "IMAGE"},
	exif.YResolution:           {settings.MetadataResolution, "Vertical resolution", "IMAGE"},
	exif.ResolutionUnit:        {settings.MetadataResolution, "Resolution unit", "IMAGE"},
	exif.ImageUniqueID:         {settings.MetadataDeviceIdentifiers, "Image unique ID", "SENSITIVE"},
}

var excludedFields = map[exif.FieldName]bool{
	exif.ExifIFDPointer: true, exif.GPSInfoIFDPointer: true, exif.InteroperabilityIFDPointer: true,
	exif.MakerNote: true, exif.ThumbJPEGInterchangeFormat: true, exif.ThumbJPEGInterchangeFormatLength: true,
}

// ExtractPath reads only the materialized, read-only source path supplied by
// the media-access boundary. Malformed or absent EXIF is non-fatal: basic
// dimensions are still returned when the image decoder supports the format.
func ExtractPath(path string) (Result, error) {
	var result Result
	file, err := os.Open(path)
	if err != nil {
		return result, err
	}
	config, _, configErr := image.DecodeConfig(io.LimitReader(file, maximumMetadataReadBytes))
	_ = file.Close()
	if configErr == nil {
		result.Width, result.Height = config.Width, config.Height
	}

	file, err = os.Open(path)
	if err != nil {
		return result, err
	}
	decoded, decodeErr := exif.Decode(io.LimitReader(file, maximumMetadataReadBytes))
	_ = file.Close()
	if decodeErr != nil {
		return result, nil
	}
	walker := &metadataWalker{}
	if err := decoded.Walk(walker); err != nil {
		return result, nil
	}
	sort.Slice(walker.entries, func(i, j int) bool {
		left, right := metadataGroupOrder[walker.entries[i].Group], metadataGroupOrder[walker.entries[j].Group]
		if left != right {
			return left < right
		}
		return walker.entries[i].Key < walker.entries[j].Key
	})
	if len(walker.entries) > maximumEntries {
		walker.entries = walker.entries[:maximumEntries]
	}
	result.Entries = walker.entries
	return result, nil
}

var metadataGroupOrder = map[string]int{"DESCRIPTIVE": 1, "DATES": 2, "CAMERA": 3, "IMAGE": 4, "SENSITIVE": 5, "OTHER": 6}

type metadataWalker struct{ entries []Entry }

func (walker *metadataWalker) Walk(name exif.FieldName, tag *tiff.Tag) error {
	if tag == nil || tag.Count == 0 || tag.Count > maximumTagCount || excludedFields[name] || strings.HasPrefix(string(name), exif.UnknownPrefix) {
		return nil
	}
	definition, known := knownFields[name]
	if !known {
		if strings.HasPrefix(string(name), "GPS") {
			definition = fieldDefinition{settings.MetadataGPS, humanise(string(name)), "SENSITIVE"}
		} else {
			definition = fieldDefinition{settings.MetadataOtherEXIF, humanise(string(name)), "OTHER"}
		}
	}
	value := formatValue(name, tag)
	if value == "" {
		return nil
	}
	walker.entries = append(walker.entries, Entry{Key: "exif." + string(name), VisibilityKey: definition.visibility,
		Label: definition.label, Group: definition.group, Value: value})
	return nil
}

func formatValue(name exif.FieldName, tag *tiff.Tag) string {
	if tag.Format() == tiff.UndefVal {
		switch name {
		case exif.XPTitle, exif.XPComment, exif.XPAuthor, exif.XPKeywords, exif.XPSubject:
			return cleanValue(decodeUTF16LE(tag.Val))
		case exif.UserComment:
			return cleanValue(decodeUserComment(tag.Val))
		case exif.FileSource:
			if len(tag.Val) == 1 {
				if tag.Val[0] == 3 {
					return "Digital still camera (3)"
				}
				return strconv.Itoa(int(tag.Val[0]))
			}
		case exif.ExifVersion, exif.FlashpixVersion:
			return cleanValue(string(tag.Val))
		case exif.GPSProcessingMethod, exif.GPSAreaInformation:
			return cleanValue(decodeUserComment(tag.Val))
		default:
			return ""
		}
		return ""
	}
	if tag.Count == 1 && tag.Format() == tiff.RatVal {
		numerator, denominator, err := tag.Rat2(0)
		if err == nil && denominator != 0 {
			decimal := float64(numerator) / float64(denominator)
			switch name {
			case exif.ExposureTime:
				if numerator == 1 {
					return fmt.Sprintf("1/%d s", denominator)
				}
				return fmt.Sprintf("%s s", trimFloat(decimal))
			case exif.FNumber:
				return "f/" + trimFloat(decimal)
			case exif.FocalLength:
				return trimFloat(decimal) + " mm"
			case exif.ExposureBiasValue:
				return fmt.Sprintf("%+.2g EV", decimal)
			}
		}
	}
	value := tag.String()
	if unquoted, err := strconv.Unquote(value); err == nil {
		value = unquoted
	} else if strings.HasPrefix(value, "[") {
		var decoded any
		if json.Unmarshal([]byte(value), &decoded) == nil {
			value = fmt.Sprint(decoded)
		}
	}
	if name == exif.FocalLengthIn35mmFilm && value != "" {
		value += " mm"
	}
	return cleanValue(value)
}

func decodeUTF16LE(value []byte) string {
	units := make([]uint16, 0, len(value)/2)
	for index := 0; index+1 < len(value); index += 2 {
		unit := uint16(value[index]) | uint16(value[index+1])<<8
		if unit == 0 {
			break
		}
		units = append(units, unit)
	}
	return string(utf16.Decode(units))
}

func decodeUserComment(value []byte) string {
	if len(value) >= 8 {
		prefix := string(value[:8])
		value = value[8:]
		if strings.HasPrefix(prefix, "UNICODE") {
			return decodeUTF16LE(value)
		}
	}
	return string(value)
}

func cleanValue(value string) string {
	value = strings.TrimSpace(strings.TrimRight(value, "\x00"))
	value = strings.Map(func(character rune) rune {
		if unicode.IsControl(character) && character != '\n' && character != '\t' {
			return -1
		}
		return character
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	if value == "" || strings.HasPrefix(value, "ERROR:") {
		return ""
	}
	runes := []rune(value)
	if len(runes) > maximumValueRunes {
		value = string(runes[:maximumValueRunes]) + "…"
	}
	return value
}

func trimFloat(value float64) string { return strconv.FormatFloat(value, 'f', -1, 64) }

func humanise(value string) string {
	var result []rune
	for index, character := range value {
		if index > 0 && unicode.IsUpper(character) {
			result = append(result, ' ')
		}
		result = append(result, character)
	}
	return string(result)
}
