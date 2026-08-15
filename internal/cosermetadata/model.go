// Package cosermetadata defines the optional boundary for importing Coser
// profile assets and social accounts from an external metadata provider.
//
// The package deliberately contains no provider registration, site URL, HTML
// selector, database write, or product-server dependency. A build with an empty
// Registry remains a complete offline CGM build.
package cosermetadata

import (
	"context"
	"errors"
	"io"
)

var (
	ErrProviderNotFound = errors.New("Coser metadata provider is not available")
	ErrPreviewNotFound  = errors.New("Coser metadata preview is unavailable or expired")
)

type ProviderInfo struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type Candidate struct {
	Ref          string `json:"ref"`
	DisplayName  string `json:"display_name"`
	SourceURL    string `json:"source_url"`
	MatchQuality int    `json:"match_quality"`
}

type RemoteAsset struct {
	Ref string
}

type SocialAccount struct {
	PlatformKey string `json:"platform_key"`
	Label       string `json:"label"`
	Handle      string `json:"handle"`
	URL         string `json:"url"`
}

type Profile struct {
	DisplayName string
	SourceURL   string
	Avatar      *RemoteAsset
	Banner      *RemoteAsset
	Accounts    []SocialAccount
}

type Asset struct {
	Reader      io.ReadCloser
	ContentType string
	ByteSize    int64
}

// Provider owns all source-specific knowledge. Candidate refs and remote asset
// refs are opaque outside the adapter and must be validated again by it.
type Provider interface {
	Info() ProviderInfo
	Search(context.Context, string) ([]Candidate, error)
	FetchProfile(context.Context, string) (Profile, error)
	OpenAsset(context.Context, string) (Asset, error)
}
