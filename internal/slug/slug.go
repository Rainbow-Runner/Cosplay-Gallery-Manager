// Package slug creates readable route keys. Portable UUIDs remain the entity
// identity; slugs are stable presentation addresses with short UUID suffixes.
package slug

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

func FromName(name string, portableUUID string) string {
	var builder strings.Builder
	lastSeparator := false
	for _, character := range strings.ToLower(norm.NFC.String(strings.TrimSpace(name))) {
		switch {
		case unicode.IsLetter(character) || unicode.IsDigit(character):
			builder.WriteRune(character)
			lastSeparator = false
		case !lastSeparator && builder.Len() > 0:
			builder.WriteByte('-')
			lastSeparator = true
		}
	}
	base := strings.Trim(builder.String(), "-")
	if base == "" {
		base = "item"
	}
	suffix := portableUUID
	if len(suffix) > 8 {
		suffix = suffix[:8]
	}
	return base + "-" + suffix
}
