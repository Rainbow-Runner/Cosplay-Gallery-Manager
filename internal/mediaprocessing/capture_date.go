package mediaprocessing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// CaptureDate reads only embedded video container/stream tags. File timestamps
// and names are intentionally not used as evidence of a shooting date.
func (adapter ProbeAdapter) CaptureDate(parent context.Context, sourcePath, timezone string) (string, string, error) {
	if adapter.Executable == "" {
		return "", "", errors.New("ffprobe is unavailable")
	}
	timeout := adapter.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	command := exec.CommandContext(ctx, adapter.Executable, "-v", "error", "-show_entries", "format_tags:stream_tags", "-of", "json", sourcePath)
	var stdout, stderr limitedBuffer
	stdout.Maximum, stderr.Maximum = 4<<20, 64<<10
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return "", "", err
	}
	var document struct {
		Format struct {
			Tags map[string]string `json:"tags"`
		} `json:"format"`
		Streams []struct {
			Tags map[string]string `json:"tags"`
		} `json:"streams"`
	}
	if err := json.NewDecoder(bytes.NewReader(stdout.Bytes())).Decode(&document); err != nil {
		return "", "", err
	}
	groups := []map[string]string{document.Format.Tags}
	for _, stream := range document.Streams {
		groups = append(groups, stream.Tags)
	}
	return captureDateFromTags(groups, timezone)
}

func captureDateFromTags(groups []map[string]string, timezone string) (string, string, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return "", "", err
	}
	for _, key := range []string{"com.apple.quicktime.creationdate", "creation_time"} {
		for _, tags := range groups {
			for name, raw := range tags {
				if !strings.EqualFold(name, key) {
					continue
				}
				for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05-0700", "2006-01-02 15:04:05-07:00", "2006-01-02 15:04:05-0700", "2006-01-02 15:04:05", "2006-01-02"} {
					var parsed time.Time
					var parseErr error
					if layout == "2006-01-02 15:04:05" || layout == "2006-01-02" {
						parsed, parseErr = time.ParseInLocation(layout, raw, location)
					} else {
						parsed, parseErr = time.Parse(layout, raw)
					}
					if parseErr == nil && parsed.Year() >= 1900 && parsed.Year() <= 2100 {
						return parsed.In(location).Format("2006-01-02"), "ffprobe." + key, nil
					}
				}
			}
		}
	}
	return "", "", nil
}
