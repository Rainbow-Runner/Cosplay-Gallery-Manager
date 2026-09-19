package mediaprocessing

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestVideoCaptureDateUsesContainerTagAndLibraryTimezone(t *testing.T) {
	probe := filepath.Join(t.TempDir(), "ffprobe")
	script := "#!/bin/sh\nprintf '%s\\n' '{\"format\":{\"tags\":{\"creation_time\":\"2024-05-12T23:30:00Z\"}}}'\n"
	if err := os.WriteFile(probe, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	date, tag, err := (ProbeAdapter{Executable: probe}).CaptureDate(context.Background(), "/tmp/unused.mp4", "Asia/Shanghai")
	if err != nil || date != "2024-05-13" || tag != "ffprobe.creation_time" {
		t.Fatalf("date=%q tag=%q err=%v", date, tag, err)
	}
}

func TestVideoCaptureDateDoesNotFallBackToFileTime(t *testing.T) {
	probe := filepath.Join(t.TempDir(), "ffprobe")
	if err := os.WriteFile(probe, []byte("#!/bin/sh\nprintf '%s\\n' '{\"format\":{\"tags\":{}}}'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	date, tag, err := (ProbeAdapter{Executable: probe}).CaptureDate(context.Background(), "/tmp/unused.mp4", "UTC")
	if err != nil || date != "" || tag != "" {
		t.Fatalf("date=%q tag=%q err=%v", date, tag, err)
	}
}
