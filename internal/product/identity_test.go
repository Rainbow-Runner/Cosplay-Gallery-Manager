package product

import (
	"regexp"
	"testing"
)

func TestStableIdentity(t *testing.T) {
	if ID == "" || ID == "stash" {
		t.Fatalf("product ID must be non-empty and independent from Stash: %q", ID)
	}

	if !regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`).MatchString(ID) {
		t.Fatalf("product ID must be a stable lowercase machine key: %q", ID)
	}

	if WorkingName == "" || DefaultConfigDirectoryName == ".stash" || DefaultDatabaseFileName == "stash-go.sqlite" {
		t.Fatal("product-facing defaults must not reuse the original Stash identity")
	}
}

func TestCurrentVersions(t *testing.T) {
	tests := []struct {
		name         string
		buildVersion string
		wantProduct  string
	}{
		{name: "development fallback", wantProduct: DevelopmentVersion},
		{name: "injected release", buildVersion: "0.4.2", wantProduct: "0.4.2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CurrentVersions(tt.buildVersion)
			if got.Product != tt.wantProduct {
				t.Fatalf("product version = %q, want %q", got.Product, tt.wantProduct)
			}
			if got.DatabaseSchema == 0 || got.ManifestSchema == 0 || got.MediaProcessing == 0 {
				t.Fatalf("compatibility versions must be positive: %#v", got)
			}
		})
	}
}

func TestSourceCodeURL(t *testing.T) {
	if got := SourceCodeURL("0123abc"); got != SourceRepositoryURL+"/tree/0123abc" {
		t.Fatalf("commit source URL = %q", got)
	}
	for _, invalid := range []string{"", "123456", "0123ABC", "not-a-commit"} {
		if got := SourceCodeURL(invalid); got != SourceRepositoryURL {
			t.Fatalf("invalid hash %q source URL = %q", invalid, got)
		}
	}
}
