package cosermetadata

import (
	"errors"
	"sort"
	"strings"
)

type Registry struct {
	providers map[string]Provider
}

func NewRegistry(providers ...Provider) (*Registry, error) {
	registry := &Registry{providers: make(map[string]Provider, len(providers))}
	for _, provider := range providers {
		if provider == nil {
			return nil, errors.New("nil Coser metadata provider")
		}
		info := provider.Info()
		key := strings.TrimSpace(info.Key)
		if key == "" || strings.TrimSpace(info.Label) == "" || key != strings.ToLower(key) {
			return nil, errors.New("invalid Coser metadata provider identity")
		}
		if _, exists := registry.providers[key]; exists {
			return nil, errors.New("duplicate Coser metadata provider key")
		}
		registry.providers[key] = provider
	}
	return registry, nil
}

func (r *Registry) Infos() []ProviderInfo {
	if r == nil {
		return nil
	}
	result := make([]ProviderInfo, 0, len(r.providers))
	for _, provider := range r.providers {
		result = append(result, provider.Info())
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return result
}

func (r *Registry) Provider(key string) (Provider, error) {
	if r != nil {
		if provider := r.providers[key]; provider != nil {
			return provider, nil
		}
	}
	return nil, ErrProviderNotFound
}
