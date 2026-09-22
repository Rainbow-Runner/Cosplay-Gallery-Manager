package portableid

import "testing"

func TestParseAcceptsCanonicalV4AndV7(t *testing.T) {
	tests := []string{
		"550e8400-e29b-41d4-a716-446655440000",
		"01890f5c-7b2a-7cc0-98c4-dc0c0c07398f",
	}

	for _, value := range tests {
		if _, err := Parse(value); err != nil {
			t.Errorf("Parse(%q): %v", value, err)
		}
	}
}

func TestParseRejectsNonCanonicalAndUnsupportedVersions(t *testing.T) {
	tests := []string{
		"550E8400-E29B-41D4-A716-446655440000",
		"550e8400e29b41d4a716446655440000",
		"550e8400-e29b-11d4-a716-446655440000",
		"not-a-uuid",
	}

	for _, value := range tests {
		if _, err := Parse(value); err == nil {
			t.Errorf("Parse(%q) unexpectedly succeeded", value)
		}
	}
}

func TestNewReturnsValidV4(t *testing.T) {
	value := New()
	parsed, err := Parse(value)
	if err != nil {
		t.Fatalf("Parse(New()): %v", err)
	}
	if parsed.Version() != 4 {
		t.Fatalf("New() version = %d, want 4", parsed.Version())
	}
}

func TestValidateKind(t *testing.T) {
	if err := ValidateKind(KindGallery); err != nil {
		t.Fatalf("Gallery kind rejected: %v", err)
	}
	if err := ValidateKind(Kind("STUDIO")); err == nil {
		t.Fatal("removed Studio entity kind unexpectedly accepted")
	}
}
