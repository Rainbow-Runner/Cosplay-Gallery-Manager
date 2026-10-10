package productlog

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	for _, value := range []string{"DEBUG", "info", " Warn ", "ERROR"} {
		if _, err := ParseLevel(value); err != nil {
			t.Fatalf("ParseLevel(%q): %v", value, err)
		}
	}
	if _, err := ParseLevel("TRACE"); err == nil {
		t.Fatal("TRACE log level unexpectedly accepted")
	}
}

func TestConfigureFiltersDebugAndKeepsStructuredAttributes(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })

	var output bytes.Buffer
	if err := Configure("INFO", &output); err != nil {
		t.Fatal(err)
	}
	slog.Debug("CGM_DEBUG_HIDDEN")
	slog.Info("CGM_TEST_EVENT", "request_id", "request-1")
	if strings.Contains(output.String(), "CGM_DEBUG_HIDDEN") ||
		!strings.Contains(output.String(), "CGM_TEST_EVENT") ||
		!strings.Contains(output.String(), "request_id=request-1") {
		t.Fatalf("unexpected log output %q", output.String())
	}
}
