package entitymetadata

import (
	"context"
	"testing"
	"time"
)

type fixtureProvider struct{}

func (fixtureProvider) Info() ProviderInfo { return ProviderInfo{Key: "fixture", Label: "Fixture"} }
func (fixtureProvider) Search(context.Context, SearchRequest) ([]Candidate, error) {
	return nil, nil
}

type invalidProvider struct{ fixtureProvider }

func (invalidProvider) Info() ProviderInfo { return ProviderInfo{Key: "invalid", Label: "Invalid"} }
func (invalidProvider) Search(context.Context, SearchRequest) ([]Candidate, error) {
	return []Candidate{{Ref: "ref", DisplayName: "Example", MatchQuality: 101}}, nil
}
func (invalidProvider) FetchNames(context.Context, EntityKind, string) (NameProfile, error) {
	return NameProfile{DisplayName: "Example", Suggestions: []AliasSuggestion{{Value: ""}}}, nil
}
func (fixtureProvider) FetchNames(context.Context, EntityKind, string) (NameProfile, error) {
	return NameProfile{DisplayName: "Example", Suggestions: []AliasSuggestion{{Value: "別名", Evidence: "本名"}}}, nil
}

func TestServiceRejectsInvalidProviderOutputAtTheBoundary(t *testing.T) {
	registry, err := NewRegistry(invalidProvider{})
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{Registry: registry}
	if _, err := service.Search(context.Background(), "invalid", SearchRequest{Kind: KindWork, Query: "Example"}); err == nil {
		t.Fatal("invalid candidate was accepted")
	}
	if _, err := service.Prepare(context.Background(), "invalid", KindWork, "uuid", "ref"); err == nil {
		t.Fatal("invalid suggestion was accepted")
	}
}

func TestServiceUsesNonNullCollectionsAndExpiringBoundPreview(t *testing.T) {
	registry, err := NewRegistry(fixtureProvider{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	service := &Service{Registry: registry, Now: func() time.Time { return now }}
	values, err := service.Search(context.Background(), "fixture", SearchRequest{Kind: KindWork, Query: "Example"})
	if err != nil || values == nil || len(values) != 0 {
		t.Fatalf("search = %#v, %v", values, err)
	}
	preview, err := service.Prepare(context.Background(), "fixture", KindWork, "uuid", "ref")
	if err != nil {
		t.Fatal(err)
	}
	if preview.TargetUUID != "" || len(preview.Suggestions) != 1 {
		t.Fatalf("public preview = %#v", preview)
	}
	internal, err := service.InternalPreview(preview.Token)
	if err != nil || internal.TargetUUID != "uuid" {
		t.Fatalf("internal preview = %#v, %v", internal, err)
	}
	now = now.Add(16 * time.Minute)
	if _, err := service.InternalPreview(preview.Token); err != ErrPreviewNotFound {
		t.Fatalf("expired error = %v", err)
	}
}
