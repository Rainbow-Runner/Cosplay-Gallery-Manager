// Package entitymetadata defines the optional boundary for importing names for
// Work and Character entities from an external metadata provider.
//
// The package deliberately contains no provider registration, remote host,
// HTML selector, database write, or product-server dependency. An empty
// Registry is a complete offline CGM build.
package entitymetadata

import (
	"context"
	"errors"
)

var (
	ErrProviderNotFound = errors.New("entity metadata provider is not available")
	ErrPreviewNotFound  = errors.New("entity metadata preview is unavailable or expired")
)

type EntityKind string

const (
	KindWork      EntityKind = "WORK"
	KindCharacter EntityKind = "CHARACTER"
)

func (k EntityKind) Valid() bool { return k == KindWork || k == KindCharacter }

type ProviderInfo struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type SearchRequest struct {
	Kind    EntityKind
	Query   string
	Context string
}

type Candidate struct {
	Ref            string `json:"ref"`
	DisplayName    string `json:"display_name"`
	Context        string `json:"context"`
	SourceURL      string `json:"source_url"`
	MatchQuality   int    `json:"match_quality"`
	TypeConfidence string `json:"type_confidence"`
}

type AliasSuggestion struct {
	Value           string `json:"value"`
	Category        string `json:"category"`
	LanguageHint    string `json:"language_hint"`
	Evidence        string `json:"evidence"`
	DefaultSelected bool   `json:"default_selected"`
}

type NameProfile struct {
	DisplayName string
	SourceURL   string
	PageID      string
	RevisionID  string
	Suggestions []AliasSuggestion
}

// Provider owns all source-specific knowledge. Candidate refs are opaque
// outside the adapter and must be validated again by it.
type Provider interface {
	Info() ProviderInfo
	Search(context.Context, SearchRequest) ([]Candidate, error)
	FetchNames(context.Context, EntityKind, string) (NameProfile, error)
}
