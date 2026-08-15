package cosermetadata

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	MaxAssetBytes   = int64(20 * 1024 * 1024)
	maxPreviewBytes = int64(96 * 1024 * 1024)
	maxPreviews     = 8
)

type PreparedPreview struct {
	Token        string          `json:"token"`
	ProviderKey  string          `json:"provider_key"`
	CoserUUID    string          `json:"-"`
	CandidateRef string          `json:"-"`
	DisplayName  string          `json:"display_name"`
	SourceURL    string          `json:"source_url"`
	HasAvatar    bool            `json:"has_avatar"`
	HasBanner    bool            `json:"has_banner"`
	Accounts     []SocialAccount `json:"accounts"`
	CreatedAt    time.Time       `json:"-"`
	ExpiresAt    time.Time       `json:"expires_at"`

	avatarType string
	avatar     []byte
	bannerType string
	banner     []byte
}

type PreviewAsset struct {
	ContentType string
	Bytes       []byte
}

type Service struct {
	Registry *Registry
	Now      func() time.Time

	mu       sync.Mutex
	previews map[string]PreparedPreview
	opMu     sync.Mutex
}

func (s *Service) Providers() []ProviderInfo {
	if s == nil || s.Registry == nil {
		return nil
	}
	return s.Registry.Infos()
}

func (s *Service) Search(ctx context.Context, providerKey, query string) ([]Candidate, error) {
	query = strings.TrimSpace(query)
	if query == "" || len([]rune(query)) > 300 {
		return nil, errors.New("Coser metadata search query must contain 1 to 300 characters")
	}
	if s == nil || s.Registry == nil {
		return nil, ErrProviderNotFound
	}
	provider, err := s.Registry.Provider(providerKey)
	if err != nil {
		return nil, err
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return provider.Search(ctx, query)
}

func (s *Service) Prepare(ctx context.Context, providerKey, coserUUID, candidateRef string) (PreparedPreview, error) {
	if s == nil || s.Registry == nil {
		return PreparedPreview{}, ErrProviderNotFound
	}
	provider, err := s.Registry.Provider(providerKey)
	if err != nil {
		return PreparedPreview{}, err
	}
	if strings.TrimSpace(coserUUID) == "" || strings.TrimSpace(candidateRef) == "" || len(candidateRef) > 512 {
		return PreparedPreview{}, errors.New("invalid Coser metadata preview request")
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	profile, err := provider.FetchProfile(ctx, candidateRef)
	if err != nil {
		return PreparedPreview{}, err
	}
	prepared := PreparedPreview{ProviderKey: providerKey, CoserUUID: coserUUID, CandidateRef: candidateRef,
		DisplayName: profile.DisplayName, SourceURL: profile.SourceURL, Accounts: append([]SocialAccount(nil), profile.Accounts...)}
	if profile.Avatar != nil {
		prepared.avatar, prepared.avatarType, err = readAsset(ctx, provider, profile.Avatar.Ref)
		if err != nil {
			return PreparedPreview{}, err
		}
		prepared.HasAvatar = true
	}
	if profile.Banner != nil {
		prepared.banner, prepared.bannerType, err = readAsset(ctx, provider, profile.Banner.Ref)
		if err != nil {
			return PreparedPreview{}, err
		}
		prepared.HasBanner = true
	}
	prepared.Token, err = previewToken()
	if err != nil {
		return PreparedPreview{}, err
	}
	prepared.CreatedAt = s.now()
	prepared.ExpiresAt = prepared.CreatedAt.Add(15 * time.Minute)
	s.store(prepared)
	return publicPreview(prepared), nil
}

func readAsset(ctx context.Context, provider Provider, ref string) ([]byte, string, error) {
	asset, err := provider.OpenAsset(ctx, ref)
	if err != nil {
		return nil, "", err
	}
	if asset.Reader == nil {
		return nil, "", errors.New("metadata provider returned an empty asset")
	}
	defer asset.Reader.Close()
	if asset.ByteSize > MaxAssetBytes {
		return nil, "", errors.New("remote Coser asset exceeds 20 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(asset.Reader, MaxAssetBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 || int64(len(data)) > MaxAssetBytes {
		return nil, "", errors.New("remote Coser asset must contain 1 to 20 MiB")
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(asset.ContentType, ";")[0]))
	detected := strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(data), ";")[0]))
	allowed := map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true}
	if !strings.HasPrefix(contentType, "image/") || !allowed[detected] {
		return nil, "", errors.New("remote Coser asset must contain JPEG, PNG, or WebP image bytes")
	}
	// Some image CDNs negotiate or cache a generic image Content-Type that does
	// not match the stored bytes. The managed asset pipeline validates and
	// decodes actual bytes, so use the detected safe type for same-origin preview.
	return data, detected, nil
}

func (s *Service) Preview(token string) (PreparedPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeExpiredLocked(s.now())
	value, ok := s.previews[token]
	if !ok {
		return PreparedPreview{}, ErrPreviewNotFound
	}
	return publicPreview(value), nil
}

func (s *Service) InternalPreview(token string) (PreparedPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeExpiredLocked(s.now())
	value, ok := s.previews[token]
	if !ok {
		return PreparedPreview{}, ErrPreviewNotFound
	}
	value.Accounts = append([]SocialAccount(nil), value.Accounts...)
	value.avatar = append([]byte(nil), value.avatar...)
	value.banner = append([]byte(nil), value.banner...)
	return value, nil
}

func (s *Service) Asset(token, kind string) (PreviewAsset, error) {
	value, err := s.InternalPreview(token)
	if err != nil {
		return PreviewAsset{}, err
	}
	switch kind {
	case "avatar":
		if !value.HasAvatar {
			return PreviewAsset{}, ErrPreviewNotFound
		}
		return PreviewAsset{ContentType: value.avatarType, Bytes: value.avatar}, nil
	case "banner":
		if !value.HasBanner {
			return PreviewAsset{}, ErrPreviewNotFound
		}
		return PreviewAsset{ContentType: value.bannerType, Bytes: value.banner}, nil
	default:
		return PreviewAsset{}, ErrPreviewNotFound
	}
}

func (s *Service) Delete(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.previews, token)
}

func (s *Service) store(value PreparedPreview) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.previews == nil {
		s.previews = make(map[string]PreparedPreview)
	}
	s.removeExpiredLocked(s.now())
	s.previews[value.Token] = value
	for len(s.previews) > maxPreviews || previewBytes(s.previews) > maxPreviewBytes {
		oldest := value.Token
		for token, candidate := range s.previews {
			if candidate.CreatedAt.Before(s.previews[oldest].CreatedAt) {
				oldest = token
			}
		}
		delete(s.previews, oldest)
	}
}

func (s *Service) removeExpiredLocked(now time.Time) {
	for token, value := range s.previews {
		if !value.ExpiresAt.After(now) {
			delete(s.previews, token)
		}
	}
}

func (s *Service) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func previewBytes(values map[string]PreparedPreview) int64 {
	var result int64
	for _, value := range values {
		result += int64(len(value.avatar) + len(value.banner))
	}
	return result
}

func publicPreview(value PreparedPreview) PreparedPreview {
	value.CoserUUID = ""
	value.CandidateRef = ""
	value.avatar = nil
	value.banner = nil
	value.avatarType = ""
	value.bannerType = ""
	value.Accounts = append([]SocialAccount(nil), value.Accounts...)
	sort.SliceStable(value.Accounts, func(i, j int) bool {
		if value.Accounts[i].PlatformKey == value.Accounts[j].PlatformKey {
			return value.Accounts[i].URL < value.Accounts[j].URL
		}
		return value.Accounts[i].PlatformKey < value.Accounts[j].PlatformKey
	})
	return value
}

func previewToken() (string, error) {
	data := make([]byte, 24)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func AssetReader(value []byte) io.ReadCloser {
	return io.NopCloser(bytes.NewReader(value))
}
