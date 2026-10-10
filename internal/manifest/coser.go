package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"time"

	"github.com/stashapp/stash/internal/portableid"
	"github.com/stashapp/stash/internal/textsafe"
	"golang.org/x/text/unicode/norm"
)

var manifestPlatformKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

type CoserDocument struct {
	SchemaVersion    int                        `json:"schema_version"`
	Revision         int64                      `json:"revision"`
	CoserUUID        string                     `json:"coser_uuid"`
	UpdatedAt        string                     `json:"updated_at"`
	Name             Optional[string]           `json:"name,omitempty"`
	SortName         Optional[string]           `json:"sort_name,omitempty"`
	Aliases          Optional[[]string]         `json:"aliases,omitempty"`
	ProfileSummary   Optional[string]           `json:"profile_summary,omitempty"`
	Biography        Optional[string]           `json:"biography,omitempty"`
	CountryOrRegion  Optional[string]           `json:"country_or_region,omitempty"`
	Avatar           Optional[string]           `json:"avatar,omitempty"`
	Banner           Optional[string]           `json:"banner,omitempty"`
	AvatarCrop       Optional[AvatarCrop]       `json:"avatar_crop,omitempty"`
	BannerFocalPoint Optional[FocalPoint]       `json:"banner_focal_point,omitempty"`
	SocialAccounts   Optional[[]SocialAccount]  `json:"social_accounts,omitempty"`
	Extensions       map[string]json.RawMessage `json:"extensions,omitempty"`
}

type AvatarCrop struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Size float64 `json:"size"`
}

type FocalPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type SocialAccount struct {
	AccountUUID string `json:"account_uuid,omitempty"`
	PlatformKey string `json:"platform_key"`
	Label       string `json:"label,omitempty"`
	Handle      string `json:"handle,omitempty"`
	URL         string `json:"url"`
	Status      string `json:"status"`
	Visible     bool   `json:"visible"`
	Position    int64  `json:"position"`
}

func ParseCoser(reader io.Reader) (CoserDocument, error) {
	data, err := readLimited(reader, MaxCoserBytes)
	if err != nil {
		return CoserDocument{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var document CoserDocument
	if err := decoder.Decode(&document); err != nil {
		return CoserDocument{}, fmt.Errorf("decoding Coser Manifest v1: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return CoserDocument{}, err
	}
	if err := document.Validate(); err != nil {
		return CoserDocument{}, err
	}
	return document, nil
}

func (document CoserDocument) Validate() error {
	if document.SchemaVersion != GallerySchemaVersion || document.Revision < 0 {
		return errors.New("invalid Coser Manifest schema_version or revision")
	}
	if _, err := portableid.Parse(document.CoserUUID); err != nil {
		return fmt.Errorf("invalid Coser Manifest coser_uuid: %w", err)
	}
	if parsed, err := time.Parse(time.RFC3339, document.UpdatedAt); err != nil || parsed.Format(time.RFC3339) != document.UpdatedAt {
		return errors.New("Coser Manifest updated_at must be canonical RFC3339")
	}
	for field, value := range map[string]Optional[string]{
		"name": document.Name, "sort_name": document.SortName,
	} {
		if value.Present && !value.Null {
			length := len([]rune(norm.NFC.String(value.Value)))
			if (field == "name" && length == 0) || length > 300 {
				return fmt.Errorf("%s must contain 1 to 300 characters", field)
			}
		}
	}
	if document.Aliases.Present && !document.Aliases.Null {
		if len(document.Aliases.Value) > 100 {
			return errors.New("aliases exceeds 100 entries")
		}
		seen := map[string]struct{}{}
		for index, alias := range document.Aliases.Value {
			normalized := norm.NFC.String(alias)
			if length := len([]rune(normalized)); length == 0 || length > 300 {
				return fmt.Errorf("aliases[%d] must contain 1 to 300 characters", index)
			}
			if _, duplicate := seen[normalized]; duplicate {
				return fmt.Errorf("aliases[%d] is duplicated", index)
			}
			seen[normalized] = struct{}{}
		}
	}
	if document.ProfileSummary.Present && !document.ProfileSummary.Null && len([]rune(document.ProfileSummary.Value)) > 20000 {
		return errors.New("profile_summary exceeds 20000 characters")
	}
	if document.Biography.Present && !document.Biography.Null {
		if len([]rune(document.Biography.Value)) > 20000 {
			return errors.New("biography exceeds 20000 characters")
		}
		if err := textsafe.ValidateMarkdown(document.Biography.Value); err != nil {
			return fmt.Errorf("biography: %w", err)
		}
	}
	if document.CountryOrRegion.Present && !document.CountryOrRegion.Null && len([]rune(document.CountryOrRegion.Value)) > 100 {
		return errors.New("country_or_region exceeds 100 characters")
	}
	if document.Avatar.Present && !document.Avatar.Null {
		if err := validateRelativePath(document.Avatar.Value); err != nil {
			return fmt.Errorf("avatar: %w", err)
		}
	}
	if document.Banner.Present && !document.Banner.Null {
		if err := validateRelativePath(document.Banner.Value); err != nil {
			return fmt.Errorf("banner: %w", err)
		}
	}
	if document.AvatarCrop.Present && !document.AvatarCrop.Null {
		crop := document.AvatarCrop.Value
		if crop.X < 0 || crop.X > 1 || crop.Y < 0 || crop.Y > 1 || crop.Size <= 0 || crop.Size > 1 ||
			crop.X+crop.Size > 1 || crop.Y+crop.Size > 1 {
			return errors.New("avatar_crop must be a normalized in-bounds square")
		}
	}
	if document.BannerFocalPoint.Present && !document.BannerFocalPoint.Null {
		point := document.BannerFocalPoint.Value
		if point.X < 0 || point.X > 1 || point.Y < 0 || point.Y > 1 {
			return errors.New("banner_focal_point must be normalized")
		}
	}
	accounts := optionalSlice(document.SocialAccounts)
	if len(accounts) > 50 {
		return errors.New("social_accounts exceeds 50 entries")
	}
	seenUUIDs := map[string]struct{}{}
	seenURLs := map[string]struct{}{}
	seenPositions := map[int64]struct{}{}
	for index, account := range accounts {
		if account.AccountUUID != "" {
			if _, err := portableid.Parse(account.AccountUUID); err != nil {
				return fmt.Errorf("social_accounts[%d].account_uuid: %w", index, err)
			}
			if _, duplicate := seenUUIDs[account.AccountUUID]; duplicate {
				return fmt.Errorf("social_accounts[%d] duplicates account_uuid", index)
			}
			seenUUIDs[account.AccountUUID] = struct{}{}
		}
		if !manifestPlatformKeyPattern.MatchString(account.PlatformKey) || account.URL == "" || account.Position <= 0 ||
			(account.Status != "ACTIVE" && account.Status != "INACTIVE") {
			return fmt.Errorf("social_accounts[%d] has invalid required fields", index)
		}
		if len([]rune(account.Label)) > 100 || len([]rune(account.Handle)) > 200 || len([]rune(account.URL)) > 2048 {
			return fmt.Errorf("social_accounts[%d] exceeds a field limit", index)
		}
		parsed, err := url.Parse(account.URL)
		if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("social_accounts[%d].url must be absolute HTTP(S)", index)
		}
		if _, duplicate := seenURLs[account.URL]; duplicate {
			return fmt.Errorf("social_accounts[%d] duplicates URL", index)
		}
		if _, duplicate := seenPositions[account.Position]; duplicate {
			return fmt.Errorf("social_accounts[%d] duplicates position", index)
		}
		seenURLs[account.URL] = struct{}{}
		seenPositions[account.Position] = struct{}{}
	}
	return nil
}
