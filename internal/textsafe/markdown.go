// Package textsafe contains write-time validation for user-authored text.
// Rendering must still escape and sanitize output; this validator keeps the
// persisted Markdown subset portable and rejects unsupported constructs early.
package textsafe

import (
	"errors"
	"regexp"
	"strings"
)

var (
	inlineLinkPattern    = regexp.MustCompile(`\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	referenceLinkPattern = regexp.MustCompile(`(?m)^\s*\[[^]]+\]:\s*(\S+)`)
	rawHTMLPattern       = regexp.MustCompile(`(?i)<\s*/?\s*[a-z!]`)
)

func ValidateMarkdown(value string) error {
	if rawHTMLPattern.MatchString(value) || strings.Contains(strings.ToLower(value), "<http") {
		return errors.New("raw HTML and angle-bracket autolinks are not allowed in Markdown")
	}
	if strings.Contains(value, "![") {
		return errors.New("Markdown images are not allowed")
	}
	for _, line := range strings.Split(value, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") || trimmed == "#" {
			return errors.New("Markdown headings must start at level 2")
		}
	}
	for _, matches := range inlineLinkPattern.FindAllStringSubmatch(value, -1) {
		if !safeHTTPURL(matches[1]) {
			return errors.New("Markdown links must use absolute HTTP(S) URLs")
		}
	}
	for _, matches := range referenceLinkPattern.FindAllStringSubmatch(value, -1) {
		if !safeHTTPURL(matches[1]) {
			return errors.New("Markdown reference links must use absolute HTTP(S) URLs")
		}
	}
	lower := strings.ToLower(value)
	for _, protocol := range []string{"javascript:", "data:", "file:", "vbscript:"} {
		if strings.Contains(lower, protocol) {
			return errors.New("dangerous URL protocol is not allowed in Markdown")
		}
	}
	return nil
}

func safeHTTPURL(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://")
}
