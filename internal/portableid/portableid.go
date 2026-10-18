// Package portableid defines the portable UUID namespace shared by manifests
// and the product database.
package portableid

import (
	"fmt"

	"github.com/google/uuid"
)

// Kind identifies the entity namespace occupying a portable UUID.
type Kind string

const (
	KindGallery       Kind = "GALLERY"
	KindGalleryItem   Kind = "GALLERY_ITEM"
	KindCoser         Kind = "COSER"
	KindWork          Kind = "WORK"
	KindCharacter     Kind = "CHARACTER"
	KindTag           Kind = "TAG"
	KindExternalLink  Kind = "EXTERNAL_LINK"
	KindSocialAccount Kind = "SOCIAL_ACCOUNT"
)

var validKinds = map[Kind]struct{}{
	KindGallery:       {},
	KindGalleryItem:   {},
	KindCoser:         {},
	KindWork:          {},
	KindCharacter:     {},
	KindTag:           {},
	KindExternalLink:  {},
	KindSocialAccount: {},
}

// ValidateKind rejects values that cannot participate in the global UUID
// registry. Adding a future portable entity requires an explicit schema and
// code change.
func ValidateKind(kind Kind) error {
	if _, ok := validKinds[kind]; !ok {
		return fmt.Errorf("unsupported portable UUID kind %q", kind)
	}
	return nil
}

// Parse validates the canonical portable form. Version 4 and version 7 UUIDs
// are accepted; non-canonical spellings are rejected so manifests and the
// database have one deterministic representation.
func Parse(value string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parsing portable UUID: %w", err)
	}
	if parsed.String() != value {
		return uuid.Nil, fmt.Errorf("portable UUID %q is not canonical lowercase form", value)
	}
	if version := parsed.Version(); version != 4 && version != 7 {
		return uuid.Nil, fmt.Errorf("portable UUID %q uses unsupported version %d", value, version)
	}
	return parsed, nil
}

// New returns a canonical lowercase UUIDv4 for a newly created entity.
func New() string {
	return uuid.NewString()
}
