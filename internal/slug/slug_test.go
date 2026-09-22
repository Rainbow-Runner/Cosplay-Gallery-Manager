package slug

import "testing"

func TestFromNameKeepsUnicodeReadableAndStableSuffix(t *testing.T) {
	got := FromName("  初音 ミク / Hatsune Miku  ", "12345678-1234-4123-8123-123456789abc")
	if got != "初音-ミク-hatsune-miku-12345678" {
		t.Fatalf("slug = %q", got)
	}
}
