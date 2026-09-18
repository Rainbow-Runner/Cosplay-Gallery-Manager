// Package portablecatalog defines the site-independent portable metadata
// archive used to move identity and core catalog data between CGM installs.
package portablecatalog

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/portableid"
	"github.com/stashapp/stash/internal/product"
	"golang.org/x/text/unicode/norm"
)

const (
	Format        = "cgm-portable-metadata"
	FormatVersion = 2
)

type VersionSet struct {
	Product         string `json:"product"`
	DatabaseSchema  uint   `json:"database_schema"`
	ManifestSchema  uint   `json:"manifest_schema"`
	MediaProcessing uint   `json:"media_processing"`
}

func Versions(value product.Versions) VersionSet {
	return VersionSet{Product: value.Product, DatabaseSchema: value.DatabaseSchema, ManifestSchema: value.ManifestSchema, MediaProcessing: value.MediaProcessing}
}

type PackageManifest struct {
	Format                string     `json:"format"`
	FormatVersion         int        `json:"format_version"`
	ProductID             string     `json:"product_id"`
	ExportID              string     `json:"export_id"`
	CreatedAt             string     `json:"created_at"`
	Versions              VersionSet `json:"versions"`
	ChecksumsSHA256       string     `json:"checksums_sha256"`
	IdentityCount         int        `json:"identity_count"`
	CoserCount            int        `json:"coser_count"`
	WorkCount             int        `json:"work_count"`
	CharacterCount        int        `json:"character_count"`
	TagCount              int        `json:"tag_count"`
	AccountCount          int        `json:"account_count"`
	GalleryCount          int        `json:"gallery_count"`
	AssetCount            int        `json:"asset_count"`
	OwnerContinuity       bool       `json:"owner_continuity"`
	OwnerGalleryLifecycle bool       `json:"owner_gallery_lifecycle"`
	OwnerPersonalFlags    bool       `json:"owner_personal_flags"`
	OwnerGalleryCount     int        `json:"owner_gallery_count"`
	OwnerItemCount        int        `json:"owner_item_count"`
}

type ChecksumManifest struct {
	Algorithm string          `json:"algorithm"`
	Files     []ChecksumEntry `json:"files"`
}

type ChecksumEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Kind   string `json:"kind"`
}

type IdentityLedger struct {
	SchemaVersion int              `json:"schema_version"`
	Identities    []IdentityRecord `json:"identities"`
}

type IdentityRecord struct {
	UUID       string `json:"uuid"`
	Kind       string `json:"kind"`
	State      string `json:"state"`
	CreatedAt  string `json:"created_at"`
	TargetUUID string `json:"target_uuid,omitempty"`
	RetiredAt  string `json:"retired_at,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type CoreCatalog struct {
	SchemaVersion int             `json:"schema_version"`
	Cosers        []Coser         `json:"cosers"`
	Works         []Work          `json:"works"`
	Characters    []Character     `json:"characters"`
	Tags          []Tag           `json:"tags"`
	TagEdges      []TagEdge       `json:"tag_edges"`
	Accounts      []SocialAccount `json:"social_accounts"`
	SlugRedirects []SlugRedirect  `json:"slug_redirects"`
}

type NamedEntity struct {
	UUID             string   `json:"uuid"`
	Name             string   `json:"name"`
	SortName         string   `json:"sort_name"`
	Aliases          []string `json:"aliases"`
	Slug             string   `json:"slug"`
	MetadataRevision int64    `json:"metadata_revision"`
	CreatedAt        string   `json:"created_at"`
	UpdatedAt        string   `json:"updated_at"`
}

type Coser struct {
	NamedEntity
	ProfileSummary  string      `json:"profile_summary"`
	Biography       string      `json:"biography"`
	CountryOrRegion string      `json:"country_or_region"`
	Avatar          *AssetRef   `json:"avatar,omitempty"`
	Banner          *AssetRef   `json:"banner,omitempty"`
	AvatarCrop      *AvatarCrop `json:"avatar_crop,omitempty"`
	BannerFocal     *FocalPoint `json:"banner_focal_point,omitempty"`
}

type Work struct{ NamedEntity }

type Character struct {
	NamedEntity
	WorkUUID string `json:"work_uuid"`
}

type Tag struct {
	NamedEntity
	UseInRecommendation bool `json:"use_in_recommendation"`
	// Missing in older packages means the former directly assignable behavior.
	AllowDirectAssignment *bool `json:"allow_direct_assignment,omitempty"`
}

func (t Tag) DirectAssignmentAllowed() bool {
	return t.AllowDirectAssignment == nil || *t.AllowDirectAssignment
}

type AssetRef struct {
	PackagePath string `json:"package_path"`
	Kind        string `json:"kind"`
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
	UUID        string `json:"uuid"`
	CoserUUID   string `json:"coser_uuid"`
	PlatformKey string `json:"platform_key"`
	Label       string `json:"label"`
	Handle      string `json:"handle"`
	URL         string `json:"url"`
	Status      string `json:"status"`
	Visible     bool   `json:"visible"`
	Position    int64  `json:"position"`
}

type TagEdge struct {
	ParentUUID string `json:"parent_uuid"`
	ChildUUID  string `json:"child_uuid"`
	Position   int64  `json:"position"`
}

type SlugRedirect struct {
	Kind       string `json:"kind"`
	OldSlug    string `json:"old_slug"`
	TargetUUID string `json:"target_uuid"`
	CreatedAt  string `json:"created_at"`
}

type GalleryIndex struct {
	SchemaVersion int              `json:"schema_version"`
	Libraries     []LibraryLocator `json:"libraries"`
	Galleries     []GalleryLocator `json:"galleries"`
}

type LibraryLocator struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type GalleryLocator struct {
	SetID            string `json:"set_id"`
	LibraryKey       string `json:"library_key,omitempty"`
	SourceType       string `json:"source_type,omitempty"`
	RelativeSource   string `json:"relative_source,omitempty"`
	LocatorStatus    string `json:"locator_status"`
	ManifestStatus   string `json:"manifest_status"`
	ManifestSchema   int    `json:"manifest_schema"`
	ManifestRevision int64  `json:"manifest_revision"`
	ManifestHash     string `json:"manifest_hash,omitempty"`
}

// OwnerContinuity is deliberately separate from Gallery manifests. Ratings
// remain manifest-owned; Slugs and browsing history are intentionally not
// portable. Every reference uses a portable identity rather than a database ID.
type OwnerContinuity struct {
	SchemaVersion            int                      `json:"schema_version"`
	IncludesGalleryLifecycle bool                     `json:"includes_gallery_lifecycle"`
	IncludesPersonalFlags    bool                     `json:"includes_personal_flags"`
	Galleries                []GalleryOwnerContinuity `json:"galleries"`
}

type GalleryOwnerContinuity struct {
	SetID       string                `json:"set_id"`
	State       string                `json:"state,omitempty"`
	AddedAt     string                `json:"added_at,omitempty"`
	Favorite    bool                  `json:"favorite"`
	FavoritedAt string                `json:"favorited_at,omitempty"`
	Hidden      bool                  `json:"hidden"`
	Items       []ItemOwnerContinuity `json:"items"`
}

type ItemOwnerContinuity struct {
	ItemUUID    string `json:"item_uuid"`
	Favorite    bool   `json:"favorite"`
	FavoritedAt string `json:"favorited_at,omitempty"`
}

type Bundle struct {
	Manifest PackageManifest
	Identity IdentityLedger
	Catalog  CoreCatalog
	Gallery  GalleryIndex
	Owner    *OwnerContinuity
}

func (b *Bundle) Normalize() {
	for index := range b.Identity.Identities {
		b.Identity.Identities[index].Reason = norm.NFC.String(b.Identity.Identities[index].Reason)
	}
	sort.Slice(b.Identity.Identities, func(i, j int) bool { return b.Identity.Identities[i].UUID < b.Identity.Identities[j].UUID })
	for i := range b.Catalog.Cosers {
		normalizeNamed(&b.Catalog.Cosers[i].NamedEntity)
		b.Catalog.Cosers[i].ProfileSummary = norm.NFC.String(b.Catalog.Cosers[i].ProfileSummary)
		b.Catalog.Cosers[i].Biography = norm.NFC.String(b.Catalog.Cosers[i].Biography)
		b.Catalog.Cosers[i].CountryOrRegion = norm.NFC.String(b.Catalog.Cosers[i].CountryOrRegion)
	}
	for i := range b.Catalog.Works {
		normalizeNamed(&b.Catalog.Works[i].NamedEntity)
	}
	for i := range b.Catalog.Characters {
		normalizeNamed(&b.Catalog.Characters[i].NamedEntity)
	}
	for i := range b.Catalog.Tags {
		normalizeNamed(&b.Catalog.Tags[i].NamedEntity)
	}
	for index := range b.Catalog.Accounts {
		value := &b.Catalog.Accounts[index]
		value.PlatformKey = norm.NFC.String(value.PlatformKey)
		value.Label = norm.NFC.String(value.Label)
		value.Handle = norm.NFC.String(value.Handle)
		value.URL = norm.NFC.String(value.URL)
	}
	for index := range b.Catalog.SlugRedirects {
		b.Catalog.SlugRedirects[index].OldSlug = norm.NFC.String(b.Catalog.SlugRedirects[index].OldSlug)
	}
	for index := range b.Gallery.Libraries {
		b.Gallery.Libraries[index].Name = norm.NFC.String(b.Gallery.Libraries[index].Name)
	}
	for index := range b.Gallery.Galleries {
		b.Gallery.Galleries[index].RelativeSource = norm.NFC.String(b.Gallery.Galleries[index].RelativeSource)
	}
	sort.Slice(b.Catalog.Cosers, func(i, j int) bool { return b.Catalog.Cosers[i].UUID < b.Catalog.Cosers[j].UUID })
	sort.Slice(b.Catalog.Works, func(i, j int) bool { return b.Catalog.Works[i].UUID < b.Catalog.Works[j].UUID })
	sort.Slice(b.Catalog.Characters, func(i, j int) bool { return b.Catalog.Characters[i].UUID < b.Catalog.Characters[j].UUID })
	sort.Slice(b.Catalog.Tags, func(i, j int) bool { return b.Catalog.Tags[i].UUID < b.Catalog.Tags[j].UUID })
	sort.Slice(b.Catalog.Accounts, func(i, j int) bool { return b.Catalog.Accounts[i].UUID < b.Catalog.Accounts[j].UUID })
	sort.Slice(b.Catalog.TagEdges, func(i, j int) bool {
		if b.Catalog.TagEdges[i].ChildUUID != b.Catalog.TagEdges[j].ChildUUID {
			return b.Catalog.TagEdges[i].ChildUUID < b.Catalog.TagEdges[j].ChildUUID
		}
		if b.Catalog.TagEdges[i].Position != b.Catalog.TagEdges[j].Position {
			return b.Catalog.TagEdges[i].Position < b.Catalog.TagEdges[j].Position
		}
		return b.Catalog.TagEdges[i].ParentUUID < b.Catalog.TagEdges[j].ParentUUID
	})
	sort.Slice(b.Catalog.SlugRedirects, func(i, j int) bool {
		if b.Catalog.SlugRedirects[i].Kind != b.Catalog.SlugRedirects[j].Kind {
			return b.Catalog.SlugRedirects[i].Kind < b.Catalog.SlugRedirects[j].Kind
		}
		return b.Catalog.SlugRedirects[i].OldSlug < b.Catalog.SlugRedirects[j].OldSlug
	})
	sort.Slice(b.Gallery.Libraries, func(i, j int) bool { return b.Gallery.Libraries[i].Key < b.Gallery.Libraries[j].Key })
	sort.Slice(b.Gallery.Galleries, func(i, j int) bool { return b.Gallery.Galleries[i].SetID < b.Gallery.Galleries[j].SetID })
	if b.Owner != nil {
		for index := range b.Owner.Galleries {
			if b.Owner.Galleries[index].Items == nil {
				b.Owner.Galleries[index].Items = []ItemOwnerContinuity{}
			}
			sort.Slice(b.Owner.Galleries[index].Items, func(i, j int) bool {
				return b.Owner.Galleries[index].Items[i].ItemUUID < b.Owner.Galleries[index].Items[j].ItemUUID
			})
		}
		sort.Slice(b.Owner.Galleries, func(i, j int) bool { return b.Owner.Galleries[i].SetID < b.Owner.Galleries[j].SetID })
	}
}

func normalizeNamed(value *NamedEntity) {
	value.Name = norm.NFC.String(value.Name)
	value.SortName = norm.NFC.String(value.SortName)
	value.Slug = norm.NFC.String(value.Slug)
	if value.Aliases == nil {
		value.Aliases = []string{}
	}
	for index := range value.Aliases {
		value.Aliases[index] = norm.NFC.String(value.Aliases[index])
	}
}

func (b Bundle) Validate() error {
	if b.Manifest.Format != Format || (b.Manifest.FormatVersion != 1 && b.Manifest.FormatVersion != FormatVersion) || b.Manifest.ProductID != product.ID {
		return errors.New("unsupported portable metadata package")
	}
	if _, err := portableid.Parse(b.Manifest.ExportID); err != nil {
		return fmt.Errorf("invalid export_id: %w", err)
	}
	if !canonicalTime(b.Manifest.CreatedAt) {
		return errors.New("created_at must be canonical RFC3339")
	}
	if b.Identity.SchemaVersion != 1 || b.Catalog.SchemaVersion != 1 || b.Gallery.SchemaVersion != 1 {
		return errors.New("unsupported portable metadata document schema")
	}
	identities := make(map[string]IdentityRecord, len(b.Identity.Identities))
	for _, value := range b.Identity.Identities {
		if _, err := portableid.Parse(value.UUID); err != nil {
			return fmt.Errorf("invalid identity UUID: %w", err)
		}
		if err := portableid.ValidateKind(portableid.Kind(value.Kind)); err != nil {
			return err
		}
		if _, duplicate := identities[value.UUID]; duplicate {
			return fmt.Errorf("duplicate identity UUID %s", value.UUID)
		}
		if !canonicalNanoTime(value.CreatedAt) {
			return fmt.Errorf("identity %s has invalid created_at", value.UUID)
		}
		switch value.State {
		case "ACTIVE":
			if value.TargetUUID != "" || value.RetiredAt != "" || value.Reason != "" {
				return fmt.Errorf("active identity %s has retirement data", value.UUID)
			}
		case "ALIAS":
			if _, err := portableid.Parse(value.TargetUUID); err != nil || !canonicalNanoTime(value.RetiredAt) {
				return fmt.Errorf("alias identity %s is invalid", value.UUID)
			}
		case "TOMBSTONE":
			if value.TargetUUID != "" || !canonicalNanoTime(value.RetiredAt) {
				return fmt.Errorf("tombstone identity %s is invalid", value.UUID)
			}
		default:
			return fmt.Errorf("identity %s has unsupported state", value.UUID)
		}
		identities[value.UUID] = value
	}
	active := func(uuid, kind string) bool {
		value, ok := identities[uuid]
		return ok && value.State == "ACTIVE" && value.Kind == kind
	}
	for _, identity := range b.Identity.Identities {
		if identity.State != "ALIAS" {
			continue
		}
		seen := map[string]bool{identity.UUID: true}
		current := identity
		for current.State == "ALIAS" {
			target, ok := identities[current.TargetUUID]
			if !ok || target.Kind != identity.Kind || seen[target.UUID] {
				return fmt.Errorf("identity Alias chain from %s is invalid", identity.UUID)
			}
			seen[target.UUID] = true
			current = target
		}
	}
	activeObjects := map[string]bool{}
	recordObject := func(uuid string) error {
		if activeObjects[uuid] {
			return fmt.Errorf("portable object %s is duplicated", uuid)
		}
		activeObjects[uuid] = true
		return nil
	}
	for _, value := range b.Catalog.Cosers {
		if err := validateNamed(value.NamedEntity, "COSER", active); err != nil {
			return err
		}
		if err := recordObject(value.UUID); err != nil {
			return err
		}
		if err := validateAsset(value.UUID, value.Avatar, "AVATAR"); err != nil {
			return err
		}
		if err := validateAsset(value.UUID, value.Banner, "BANNER"); err != nil {
			return err
		}
	}
	for _, value := range b.Catalog.Works {
		if err := validateNamed(value.NamedEntity, "WORK", active); err != nil {
			return err
		}
		if err := recordObject(value.UUID); err != nil {
			return err
		}
	}
	for _, value := range b.Catalog.Characters {
		if err := validateNamed(value.NamedEntity, "CHARACTER", active); err != nil {
			return err
		}
		if err := recordObject(value.UUID); err != nil {
			return err
		}
		if !active(value.WorkUUID, "WORK") {
			return fmt.Errorf("character %s references inactive Work", value.UUID)
		}
	}
	for _, value := range b.Catalog.Tags {
		if err := validateNamed(value.NamedEntity, "TAG", active); err != nil {
			return err
		}
		if err := recordObject(value.UUID); err != nil {
			return err
		}
	}
	for _, value := range b.Catalog.Accounts {
		if !active(value.UUID, "SOCIAL_ACCOUNT") || !active(value.CoserUUID, "COSER") {
			return fmt.Errorf("social account %s has invalid identity", value.UUID)
		}
		if err := recordObject(value.UUID); err != nil {
			return err
		}
		if value.PlatformKey == "" || value.URL == "" || value.Position <= 0 || (value.Status != "ACTIVE" && value.Status != "INACTIVE") {
			return fmt.Errorf("social account %s has invalid fields", value.UUID)
		}
	}
	for _, value := range b.Catalog.TagEdges {
		if !active(value.ParentUUID, "TAG") || !active(value.ChildUUID, "TAG") || value.Position <= 0 {
			return errors.New("invalid Tag edge")
		}
	}
	libraries := map[string]bool{}
	for _, value := range b.Gallery.Libraries {
		if value.Key == "" || value.Name == "" || libraries[value.Key] {
			return errors.New("invalid or duplicate Gallery index library")
		}
		libraries[value.Key] = true
	}
	for _, value := range b.Gallery.Galleries {
		if _, err := portableid.Parse(value.SetID); err != nil {
			return fmt.Errorf("invalid Gallery set_id: %w", err)
		}
		if !active(value.SetID, "GALLERY") {
			return fmt.Errorf("Gallery %s has invalid identity", value.SetID)
		}
		if err := recordObject(value.SetID); err != nil {
			return err
		}
		if value.RelativeSource != "" && !safeRelative(value.RelativeSource) {
			return fmt.Errorf("Gallery %s has unsafe relative source", value.SetID)
		}
		if value.LibraryKey != "" && !libraries[value.LibraryKey] {
			return fmt.Errorf("Gallery %s references an unknown library", value.SetID)
		}
		switch value.LocatorStatus {
		case "MAPPED":
			if value.LibraryKey == "" || value.RelativeSource == "" {
				return fmt.Errorf("Gallery %s has an incomplete mapped locator", value.SetID)
			}
		case "LIBRARY_ROOT":
			if value.LibraryKey == "" || value.RelativeSource != "" {
				return fmt.Errorf("Gallery %s has an invalid library-root locator", value.SetID)
			}
		case "UNBOUND", "OUTSIDE_LIBRARY":
			if value.RelativeSource != "" {
				return fmt.Errorf("Gallery %s has an invalid unresolved locator", value.SetID)
			}
		default:
			return fmt.Errorf("Gallery %s has an unsupported locator status", value.SetID)
		}
		if value.SourceType != "" && value.SourceType != "DIRECTORY" && value.SourceType != "ARCHIVE" {
			return fmt.Errorf("Gallery %s has an unsupported source type", value.SetID)
		}
		switch value.ManifestStatus {
		case "CLEAN", "DB_DIRTY", "FILE_DIRTY", "CONFLICT", "MISSING", "ERROR":
		default:
			return fmt.Errorf("Gallery %s has an unsupported Manifest status", value.SetID)
		}
	}
	for _, value := range b.Catalog.SlugRedirects {
		if value.OldSlug == "" || !active(value.TargetUUID, value.Kind) || !canonicalNanoTime(value.CreatedAt) {
			return errors.New("invalid core entity Slug redirect")
		}
	}
	for _, identity := range b.Identity.Identities {
		if identity.State != "ACTIVE" {
			continue
		}
		switch identity.Kind {
		case "COSER", "WORK", "CHARACTER", "TAG", "SOCIAL_ACCOUNT", "GALLERY":
			if !activeObjects[identity.UUID] {
				return fmt.Errorf("active %s identity %s has no portable object", identity.Kind, identity.UUID)
			}
		}
	}
	if hasTagCycle(b.Catalog.TagEdges) {
		return errors.New("portable Tag graph contains a cycle")
	}
	if err := validateOwnerContinuity(b.Owner, active); err != nil {
		return err
	}
	return nil
}

func validateOwnerContinuity(value *OwnerContinuity, active func(string, string) bool) error {
	if value == nil {
		return nil
	}
	if value.SchemaVersion != 1 {
		return errors.New("unsupported owner continuity schema")
	}
	if !value.IncludesGalleryLifecycle && !value.IncludesPersonalFlags {
		return errors.New("owner continuity has no selected data")
	}
	previousGallery := ""
	seenItems := map[string]bool{}
	for _, gallery := range value.Galleries {
		if gallery.SetID <= previousGallery || !active(gallery.SetID, "GALLERY") {
			return errors.New("invalid owner continuity Gallery identity")
		}
		previousGallery = gallery.SetID
		if value.IncludesGalleryLifecycle {
			if gallery.State != "DRAFT" && gallery.State != "ACTIVE" && gallery.State != "ARCHIVED" {
				return errors.New("invalid owner continuity Gallery state")
			}
			if gallery.AddedAt != "" && !canonicalNanoTime(gallery.AddedAt) {
				return errors.New("invalid owner continuity added_at")
			}
			if gallery.State == "ACTIVE" && gallery.AddedAt == "" {
				return errors.New("active owner continuity Gallery lacks added_at")
			}
		} else if gallery.State != "" || gallery.AddedAt != "" {
			return errors.New("unselected Gallery lifecycle is present")
		}
		if value.IncludesPersonalFlags {
			if gallery.Favorite != (gallery.FavoritedAt != "") || gallery.FavoritedAt != "" && !canonicalNanoTime(gallery.FavoritedAt) {
				return errors.New("invalid owner continuity Gallery favorite")
			}
		} else if gallery.Favorite || gallery.FavoritedAt != "" || gallery.Hidden || len(gallery.Items) != 0 {
			return errors.New("unselected personal flags are present")
		}
		previousItem := ""
		for _, item := range gallery.Items {
			if item.ItemUUID <= previousItem || seenItems[item.ItemUUID] || !active(item.ItemUUID, "GALLERY_ITEM") {
				return errors.New("invalid owner continuity Item identity")
			}
			previousItem, seenItems[item.ItemUUID] = item.ItemUUID, true
			if !value.IncludesPersonalFlags || !item.Favorite || item.FavoritedAt == "" || !canonicalNanoTime(item.FavoritedAt) {
				return errors.New("invalid owner continuity Item favorite")
			}
		}
	}
	return nil
}

func hasTagCycle(edges []TagEdge) bool {
	children := map[string][]string{}
	for _, edge := range edges {
		children[edge.ParentUUID] = append(children[edge.ParentUUID], edge.ChildUUID)
	}
	state := map[string]uint8{}
	var visit func(string) bool
	visit = func(uuid string) bool {
		if state[uuid] == 1 {
			return true
		}
		if state[uuid] == 2 {
			return false
		}
		state[uuid] = 1
		for _, child := range children[uuid] {
			if visit(child) {
				return true
			}
		}
		state[uuid] = 2
		return false
	}
	for uuid := range children {
		if visit(uuid) {
			return true
		}
	}
	return false
}

func validateNamed(value NamedEntity, kind string, active func(string, string) bool) error {
	if !active(value.UUID, kind) || value.Name == "" || value.Slug == "" || value.MetadataRevision <= 0 || !canonicalNanoTime(value.CreatedAt) || !canonicalNanoTime(value.UpdatedAt) {
		return fmt.Errorf("invalid %s %s", kind, value.UUID)
	}
	return nil
}

func validateAsset(coserUUID string, value *AssetRef, kind string) error {
	if value == nil {
		return nil
	}
	if value.Kind != kind || !safeRelative(value.PackagePath) || !strings.HasPrefix(value.PackagePath, "coser-assets/"+coserUUID+"/") {
		return fmt.Errorf("invalid %s asset for Coser %s", kind, coserUUID)
	}
	return nil
}

func safeRelative(value string) bool {
	return value != "" && norm.NFC.IsNormalString(value) && !strings.Contains(value, "\\") && !strings.HasPrefix(value, "/") && path.Clean(value) == value && value != "." && value != ".." && !strings.HasPrefix(value, "../")
}

func canonicalTime(value string) bool {
	parsed, err := time.Parse(time.RFC3339, value)
	return err == nil && parsed.Format(time.RFC3339) == value
}

func canonicalNanoTime(value string) bool {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && parsed.Format(time.RFC3339Nano) == value
}
