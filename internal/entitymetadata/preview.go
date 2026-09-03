package entitymetadata

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"time"
)

const maxPreviews = 32

type PreparedPreview struct {
	Token        string            `json:"token"`
	ProviderKey  string            `json:"provider_key"`
	Kind         EntityKind        `json:"kind"`
	TargetUUID   string            `json:"-"`
	CandidateRef string            `json:"-"`
	DisplayName  string            `json:"display_name"`
	SourceURL    string            `json:"source_url"`
	PageID       string            `json:"page_id"`
	RevisionID   string            `json:"revision_id"`
	Suggestions  []AliasSuggestion `json:"suggestions"`
	CreatedAt    time.Time         `json:"-"`
	ExpiresAt    time.Time         `json:"expires_at"`
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
		return []ProviderInfo{}
	}
	return s.Registry.Infos()
}

func (s *Service) Search(ctx context.Context, providerKey string, request SearchRequest) ([]Candidate, error) {
	request.Query, request.Context = strings.TrimSpace(request.Query), strings.TrimSpace(request.Context)
	if !request.Kind.Valid() || request.Query == "" || len([]rune(request.Query)) > 300 || len([]rune(request.Context)) > 300 {
		return nil, errors.New("invalid entity metadata search request")
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
	result, err := provider.Search(ctx, request)
	if err == nil {
		if len(result) > 50 {
			return nil, errors.New("entity metadata provider returned too many candidates")
		}
		for _, candidate := range result {
			if strings.TrimSpace(candidate.Ref) == "" || len(candidate.Ref) > 512 || strings.TrimSpace(candidate.DisplayName) == "" ||
				len([]rune(candidate.DisplayName)) > 300 || len([]rune(candidate.Context)) > 500 || len(candidate.SourceURL) > 2048 ||
				candidate.MatchQuality < 0 || candidate.MatchQuality > 100 {
				return nil, errors.New("entity metadata provider returned an invalid candidate")
			}
		}
	}
	if result == nil {
		result = []Candidate{}
	}
	return result, err
}

func (s *Service) Prepare(ctx context.Context, providerKey string, kind EntityKind, targetUUID, candidateRef string) (PreparedPreview, error) {
	if s == nil || s.Registry == nil {
		return PreparedPreview{}, ErrProviderNotFound
	}
	if !kind.Valid() || strings.TrimSpace(targetUUID) == "" || strings.TrimSpace(candidateRef) == "" || len(candidateRef) > 512 {
		return PreparedPreview{}, errors.New("invalid entity metadata preview request")
	}
	provider, err := s.Registry.Provider(providerKey)
	if err != nil {
		return PreparedPreview{}, err
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	profile, err := provider.FetchNames(ctx, kind, candidateRef)
	if err != nil {
		return PreparedPreview{}, err
	}
	prepared := PreparedPreview{ProviderKey: providerKey, Kind: kind, TargetUUID: targetUUID, CandidateRef: candidateRef,
		DisplayName: strings.TrimSpace(profile.DisplayName), SourceURL: profile.SourceURL, PageID: profile.PageID, RevisionID: profile.RevisionID,
		Suggestions: append([]AliasSuggestion{}, profile.Suggestions...)}
	if prepared.DisplayName == "" || len([]rune(prepared.DisplayName)) > 300 || len(prepared.SourceURL) > 2048 ||
		len(prepared.PageID) > 100 || len(prepared.RevisionID) > 100 || len(prepared.Suggestions) > 100 {
		return PreparedPreview{}, errors.New("entity metadata provider returned an invalid preview")
	}
	for _, suggestion := range prepared.Suggestions {
		if strings.TrimSpace(suggestion.Value) == "" || len([]rune(suggestion.Value)) > 300 || len(suggestion.Category) > 100 ||
			len(suggestion.LanguageHint) > 32 || len([]rune(suggestion.Evidence)) > 300 {
			return PreparedPreview{}, errors.New("entity metadata provider returned an invalid suggestion")
		}
	}
	prepared.Token, err = entityPreviewToken()
	if err != nil {
		return PreparedPreview{}, err
	}
	prepared.CreatedAt = s.now()
	prepared.ExpiresAt = prepared.CreatedAt.Add(15 * time.Minute)
	s.store(prepared)
	return publicPreview(prepared), nil
}

func (s *Service) InternalPreview(token string) (PreparedPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeExpiredLocked(s.now())
	value, ok := s.previews[token]
	if !ok {
		return PreparedPreview{}, ErrPreviewNotFound
	}
	value.Suggestions = append([]AliasSuggestion{}, value.Suggestions...)
	return value, nil
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
	for len(s.previews) > maxPreviews {
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

func publicPreview(value PreparedPreview) PreparedPreview {
	value.TargetUUID, value.CandidateRef = "", ""
	value.Suggestions = append([]AliasSuggestion{}, value.Suggestions...)
	return value
}

func entityPreviewToken() (string, error) {
	data := make([]byte, 24)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}
