package mediaprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/imagemetadata"
)

// Separate from CaptureDate: preserving timestamp precision must not change
// the existing calendar-day shooting-date/Manifest algorithm.
func (adapter ProbeAdapter) CaptureTime(parent context.Context, path, zone string) (string, string, string, error) {
	if adapter.Executable == "" {
		return "", "", "", errors.New("ffprobe unavailable")
	}
	timeout := adapter.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, adapter.Executable, "-v", "error", "-show_entries", "format_tags:stream_tags", "-of", "json", path)
	var stdout, stderr limitedBuffer
	stdout.Maximum = 4 << 20
	stderr.Maximum = 64 << 10
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", "", "", err
	}
	var doc struct {
		Format struct {
			Tags map[string]string `json:"tags"`
		} `json:"format"`
		Streams []struct {
			Tags map[string]string `json:"tags"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		return "", "", "", err
	}
	groups := []map[string]string{doc.Format.Tags}
	for _, stream := range doc.Streams {
		groups = append(groups, stream.Tags)
	}
	for _, key := range []string{"com.apple.quicktime.creationdate", "creation_time"} {
		for _, tags := range groups {
			for name, raw := range tags {
				if !strings.EqualFold(name, key) {
					continue
				}
				if stamp, precision, source, ok := imagemetadata.ParseEvidenceTime(raw, zone); ok {
					return stamp.UTC().Format(imagemetadata.EvidenceTimeLayout), precision, source, nil
				}
			}
		}
	}
	return "", "", "", nil
}
