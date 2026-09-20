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

func TestProbeWithCaptureDateUsesOneFFprobeCall(t *testing.T) {
	root := t.TempDir()
	probe := filepath.Join(root, "ffprobe")
	count := filepath.Join(root, "count")
	script := "#!/bin/sh\nprintf x >> '" + count + "'\nprintf '%s\\n' '{\"format\":{\"format_name\":\"mp4\",\"tags\":{\"creation_time\":\"2024-05-12T23:30:00Z\"}},\"streams\":[{\"index\":0,\"codec_type\":\"video\",\"codec_name\":\"h264\",\"width\":1280,\"height\":720}]}'\n"
	if err := os.WriteFile(probe, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	metadata, date, tag, dateErr, err := (ProbeAdapter{Executable: probe}).ProbeWithCaptureDate(context.Background(), filepath.Join(root, "source.mp4"), "Asia/Shanghai")
	if err != nil || dateErr != nil || metadata.VideoCodec != "h264" || date != "2024-05-13" || tag != "ffprobe.creation_time" {
		t.Fatalf("metadata=%#v date=%q tag=%q dateErr=%v err=%v", metadata, date, tag, dateErr, err)
	}
	invocations, err := os.ReadFile(count)
	if err != nil || string(invocations) != "x" {
		t.Fatalf("FFprobe invocations=%q err=%v", invocations, err)
	}
}
