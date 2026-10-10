package cosermetadata

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

type fakeProvider struct{}

func (fakeProvider) Info() ProviderInfo { return ProviderInfo{Key: "fixture", Label: "Fixture"} }
func (fakeProvider) Search(context.Context, string) ([]Candidate, error) {
	return []Candidate{{Ref: "1", DisplayName: "Example"}}, nil
}
func (fakeProvider) FetchProfile(context.Context, string) (Profile, error) {
	return Profile{DisplayName: "Example", Avatar: &RemoteAsset{Ref: "avatar"}, Accounts: []SocialAccount{{PlatformKey: "website", URL: "https://example.test"}}}, nil
}
func (fakeProvider) OpenAsset(context.Context, string) (Asset, error) {
	data := "\xff\xd8\xff\xdb"
	return Asset{Reader: io.NopCloser(strings.NewReader(data)), ContentType: "image/jpeg", ByteSize: int64(len(data))}, nil
}

func TestEmptyRegistryKeepsCoreOperational(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil || len(registry.Infos()) != 0 {
		t.Fatalf("empty registry = %#v, %v", registry.Infos(), err)
	}
	if _, err := registry.Provider("galleryepic"); err != ErrProviderNotFound {
		t.Fatalf("missing provider error = %v", err)
	}
}

func TestPreviewIsBoundedAndExpires(t *testing.T) {
	registry, _ := NewRegistry(fakeProvider{})
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	service := &Service{Registry: registry, Now: func() time.Time { return now }}
	preview, err := service.Prepare(context.Background(), "fixture", "coser-1", "1")
	if err != nil {
		t.Fatal(err)
	}
	if !preview.HasAvatar || preview.CoserUUID != "" || preview.Token == "" {
		t.Fatalf("public preview leaked internal state: %#v", preview)
	}
	asset, err := service.Asset(preview.Token, "avatar")
	if err != nil || len(asset.Bytes) != 4 {
		t.Fatalf("asset = %#v, %v", asset, err)
	}
	now = now.Add(16 * time.Minute)
	if _, err := service.Preview(preview.Token); err != ErrPreviewNotFound {
		t.Fatalf("expired preview error = %v", err)
	}
}
