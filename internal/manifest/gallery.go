package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/portableid"
	"github.com/stashapp/stash/internal/textsafe"
	"golang.org/x/text/unicode/norm"
)

const (
	GallerySchemaVersion = 1
	MaxGalleryBytes      = 16 * 1024 * 1024
	MaxCoserBytes        = 2 * 1024 * 1024
)

type GalleryDocument struct {
	SchemaVersion    int                         `json:"schema_version"`
	Revision         int64                       `json:"revision"`
	SetID            string                      `json:"set_id"`
	UpdatedAt        string                      `json:"updated_at"`
	Title            Optional[string]            `json:"title,omitempty"`
	Description      Optional[string]            `json:"description,omitempty"`
	ShootDate        Optional[ShootDate]         `json:"shoot_date,omitempty"`
	ContentRating    Optional[string]            `json:"content_rating,omitempty"`
	Rating           Optional[float64]           `json:"rating,omitempty"`
	PhotographerName Optional[string]            `json:"photographer_name,omitempty"`
	StudioName       Optional[string]            `json:"studio_name,omitempty"`
	Credits          Optional[[]Credit]          `json:"credits,omitempty"`
	Cast             Optional[[]Cast]            `json:"cast,omitempty"`
	Tags             Optional[[]EntityReference] `json:"tags,omitempty"`
	ExternalLinks    Optional[[]ExternalLink]    `json:"external_links,omitempty"`
	Cover            Optional[Cover]             `json:"cover,omitempty"`
	Items            Optional[[]Item]            `json:"items,omitempty"`
	ExcludedItems    Optional[[]Exclusion]       `json:"excluded_items,omitempty"`
	Extensions       map[string]json.RawMessage  `json:"extensions,omitempty"`
}

type ShootDate struct {
	Value     string `json:"value"`
	Precision string `json:"precision"`
}

type EntityReference struct {
	UUID     string `json:"uuid"`
	NameHint string `json:"name_hint,omitempty"`
}

type Credit struct {
	Coser    EntityReference `json:"coser"`
	Position int64           `json:"position"`
}

type Cast struct {
	Coser     EntityReference `json:"coser"`
	Character EntityReference `json:"character"`
	Work      EntityReference `json:"work"`
	Position  int64           `json:"position"`
}

type ExternalLink struct {
	LinkUUID string `json:"link_uuid,omitempty"`
	Type     string `json:"type"`
	Label    string `json:"label,omitempty"`
	URL      string `json:"url"`
	Position int64  `json:"position"`
}

type Cover struct {
	Kind     string `json:"kind"`
	ItemUUID string `json:"item_uuid,omitempty"`
	Path     string `json:"path,omitempty"`
}

type Item struct {
	ItemUUID string   `json:"item_uuid,omitempty"`
	Path     string   `json:"path"`
	Category string   `json:"category,omitempty"`
	Caption  string   `json:"caption,omitempty"`
	Position int64    `json:"position,omitempty"`
	Rating   *float64 `json:"rating,omitempty"`
}

type Exclusion struct {
	ItemUUID    string `json:"item_uuid,omitempty"`
	Path        string `json:"path,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

func ParseGallery(reader io.Reader) (GalleryDocument, error) {
	data, err := readLimited(reader, MaxGalleryBytes)
	if err != nil {
		return GalleryDocument{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var document GalleryDocument
	if err := decoder.Decode(&document); err != nil {
		return GalleryDocument{}, fmt.Errorf("decoding Gallery Manifest v1: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return GalleryDocument{}, err
	}
	if err := document.Validate(); err != nil {
		return GalleryDocument{}, err
	}
	return document, nil
}

func (document GalleryDocument) Validate() error {
	if document.SchemaVersion != GallerySchemaVersion {
		return fmt.Errorf("unsupported Gallery Manifest schema_version %d", document.SchemaVersion)
	}
	if document.Revision < 0 {
		return errors.New("Gallery Manifest revision cannot be negative")
	}
	if _, err := portableid.Parse(document.SetID); err != nil {
		return fmt.Errorf("invalid Gallery Manifest set_id: %w", err)
	}
	if parsed, err := time.Parse(time.RFC3339, document.UpdatedAt); err != nil || parsed.Format(time.RFC3339) != document.UpdatedAt {
		return errors.New("Gallery Manifest updated_at must be canonical RFC3339")
	}
	if document.Rating.Present && !document.Rating.Null && !validHalfStar(document.Rating.Value) {
		return errors.New("Gallery Manifest rating must be 0.5 to 5.0 in half-star steps")
	}
	if document.ShootDate.Present && !document.ShootDate.Null {
		if err := validateShootDate(document.ShootDate.Value); err != nil {
			return err
		}
	}
	if document.Title.Present && !document.Title.Null && len([]rune(norm.NFC.String(document.Title.Value))) > 300 {
		return errors.New("Gallery Manifest title exceeds 300 characters")
	}
	if document.Description.Present && !document.Description.Null {
		if len([]rune(norm.NFC.String(document.Description.Value))) > 20000 {
			return errors.New("Gallery Manifest description exceeds 20000 characters")
		}
		if err := textsafe.ValidateMarkdown(document.Description.Value); err != nil {
			return fmt.Errorf("Gallery Manifest description: %w", err)
		}
	}
	if document.ContentRating.Present && !document.ContentRating.Null &&
		document.ContentRating.Value != "NON_ADULT" && document.ContentRating.Value != "ADULT" {
		return errors.New("Gallery Manifest content_rating must be NON_ADULT or ADULT")
	}
	if document.PhotographerName.Present && !document.PhotographerName.Null && len([]rune(document.PhotographerName.Value)) > 200 {
		return errors.New("photographer_name exceeds 200 characters")
	}
	if document.StudioName.Present && !document.StudioName.Null && len([]rune(document.StudioName.Value)) > 200 {
		return errors.New("studio_name exceeds 200 characters")
	}
	if len(optionalSlice(document.Credits)) > 100 || len(optionalSlice(document.Cast)) > 500 ||
		len(optionalSlice(document.Tags)) > 200 || len(optionalSlice(document.ExternalLinks)) > 50 ||
		len(optionalSlice(document.Items)) > 1000 {
		return errors.New("Gallery Manifest collection exceeds its hard limit")
	}
	seenCredits := map[string]struct{}{}
	for index, credit := range optionalSlice(document.Credits) {
		if err := validateReference(credit.Coser, "credits", index); err != nil {
			return err
		}
		if credit.Position <= 0 {
			return fmt.Errorf("credits[%d].position must be positive", index)
		}
		if _, duplicate := seenCredits[credit.Coser.UUID]; duplicate {
			return fmt.Errorf("credits[%d] duplicates Coser UUID", index)
		}
		seenCredits[credit.Coser.UUID] = struct{}{}
	}
	creditUUIDs := make(map[string]struct{})
	for _, credit := range optionalSlice(document.Credits) {
		creditUUIDs[credit.Coser.UUID] = struct{}{}
	}
	seenCast := map[string]struct{}{}
	for index, cast := range optionalSlice(document.Cast) {
		for field, reference := range map[string]EntityReference{
			"coser": cast.Coser, "character": cast.Character, "work": cast.Work,
		} {
			if err := validateReference(reference, "cast."+field, index); err != nil {
				return err
			}
		}
		if _, exists := creditUUIDs[cast.Coser.UUID]; document.Credits.Present && !document.Credits.Null && !exists {
			return fmt.Errorf("cast[%d] references a Coser not present in credits", index)
		}
		if cast.Position <= 0 {
			return fmt.Errorf("cast[%d].position must be positive", index)
		}
		key := cast.Coser.UUID + "|" + cast.Character.UUID
		if _, duplicate := seenCast[key]; duplicate {
			return fmt.Errorf("cast[%d] duplicates Coser/Character pair", index)
		}
		seenCast[key] = struct{}{}
	}
	seenTags := map[string]struct{}{}
	for index, tag := range optionalSlice(document.Tags) {
		if err := validateReference(tag, "tags", index); err != nil {
			return err
		}
		if _, duplicate := seenTags[tag.UUID]; duplicate {
			return fmt.Errorf("tags[%d] duplicates UUID", index)
		}
		seenTags[tag.UUID] = struct{}{}
	}
	seenLinks := map[string]struct{}{}
	seenURLs := map[string]struct{}{}
	for index, link := range optionalSlice(document.ExternalLinks) {
		if link.LinkUUID != "" {
			if _, err := portableid.Parse(link.LinkUUID); err != nil {
				return fmt.Errorf("external_links[%d].link_uuid: %w", index, err)
			}
			if _, duplicate := seenLinks[link.LinkUUID]; duplicate {
				return fmt.Errorf("external_links[%d] duplicates link_uuid", index)
			}
			seenLinks[link.LinkUUID] = struct{}{}
		}
		if link.Type != "SOURCE" && link.Type != "PROFILE" && link.Type != "REFERENCE" {
			return fmt.Errorf("external_links[%d].type is invalid", index)
		}
		parsed, err := url.Parse(link.URL)
		if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("external_links[%d].url must be absolute HTTP(S)", index)
		}
		if len([]rune(link.Label)) > 100 || len([]rune(link.URL)) > 2048 || link.Position <= 0 {
			return fmt.Errorf("external_links[%d] exceeds a field limit", index)
		}
		if _, duplicate := seenURLs[link.URL]; duplicate {
			return fmt.Errorf("external_links[%d] duplicates URL", index)
		}
		seenURLs[link.URL] = struct{}{}
	}
	seenItems := map[string]struct{}{}
	for index, item := range optionalSlice(document.Items) {
		if item.ItemUUID != "" {
			if _, err := portableid.Parse(item.ItemUUID); err != nil {
				return fmt.Errorf("items[%d].item_uuid: %w", index, err)
			}
		}
		if err := validateRelativePath(item.Path); err != nil {
			return fmt.Errorf("items[%d].path: %w", index, err)
		}
		if item.Rating != nil && !validHalfStar(*item.Rating) {
			return fmt.Errorf("items[%d].rating is not a half-star value", index)
		}
		if len([]rune(item.Caption)) > 1000 {
			return fmt.Errorf("items[%d].caption exceeds 1000 characters", index)
		}
		key := item.ItemUUID
		if key == "" {
			key = "path:" + item.Path
		}
		if _, duplicate := seenItems[key]; duplicate {
			return fmt.Errorf("items[%d] duplicates stable key", index)
		}
		seenItems[key] = struct{}{}
	}
	for index, exclusion := range optionalSlice(document.ExcludedItems) {
		if exclusion.ItemUUID == "" && exclusion.Path == "" && exclusion.Fingerprint == "" {
			return fmt.Errorf("excluded_items[%d] requires item_uuid, fingerprint or path", index)
		}
		if exclusion.ItemUUID != "" {
			if _, err := portableid.Parse(exclusion.ItemUUID); err != nil {
				return fmt.Errorf("excluded_items[%d].item_uuid: %w", index, err)
			}
		}
		if exclusion.Path != "" {
			if err := validateRelativePath(exclusion.Path); err != nil {
				return fmt.Errorf("excluded_items[%d].path: %w", index, err)
			}
		}
	}
	if document.Cover.Present && !document.Cover.Null {
		if document.Cover.Value.ItemUUID != "" {
			if _, err := portableid.Parse(document.Cover.Value.ItemUUID); err != nil {
				return fmt.Errorf("cover.item_uuid: %w", err)
			}
		}
		if document.Cover.Value.Path != "" {
			if err := validateRelativePath(document.Cover.Value.Path); err != nil {
				return fmt.Errorf("cover.path: %w", err)
			}
		}
		switch document.Cover.Value.Kind {
		case "AUTO_RANDOM", "ITEM":
			if document.Cover.Value.ItemUUID == "" || document.Cover.Value.Path != "" {
				return errors.New("cover AUTO_RANDOM/ITEM requires item_uuid and forbids path")
			}
		case "MANAGED":
			if document.Cover.Value.Path == "" || document.Cover.Value.ItemUUID != "" {
				return errors.New("cover MANAGED requires path and forbids item_uuid")
			}
		default:
			return errors.New("cover.kind must be AUTO_RANDOM, ITEM or MANAGED")
		}
	}
	return nil
}

func readLimited(reader io.Reader, maximum int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("Manifest exceeds %d byte limit", maximum)
	}
	return data, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("Manifest contains multiple JSON values")
		}
		return fmt.Errorf("reading Manifest trailing data: %w", err)
	}
	return nil
}

func validateReference(reference EntityReference, field string, index int) error {
	if _, err := portableid.Parse(reference.UUID); err != nil {
		return fmt.Errorf("%s[%d].uuid: %w", field, index, err)
	}
	if len([]rune(norm.NFC.String(reference.NameHint))) > 300 {
		return fmt.Errorf("%s[%d].name_hint exceeds 300 characters", field, index)
	}
	return nil
}

func validateShootDate(value ShootDate) error {
	layout := "2006-01"
	if value.Precision == "DAY" {
		layout = "2006-01-02"
	} else if value.Precision != "MONTH" {
		return errors.New("shoot_date.precision must be MONTH or DAY")
	}
	parsed, err := time.Parse(layout, value.Value)
	if err != nil || parsed.Format(layout) != value.Value {
		return errors.New("shoot_date.value does not match its precision")
	}
	return nil
}

func validateRelativePath(value string) error {
	if value == "" || value != norm.NFC.String(value) || strings.Contains(value, "\\") ||
		strings.HasPrefix(value, "/") || path.Clean(value) != value || value == "." || value == ".." ||
		strings.HasPrefix(value, "../") {
		return errors.New("path must be an NFC forward-slash relative path without traversal")
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return errors.New("path contains a control character")
		}
	}
	return nil
}

func validHalfStar(value float64) bool {
	return value >= 0.5 && value <= 5 && value*2 == float64(int(value*2))
}

func optionalSlice[T any](value Optional[[]T]) []T {
	if !value.Present || value.Null {
		return nil
	}
	return value.Value
}
