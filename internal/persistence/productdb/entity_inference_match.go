package productdb

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	// Sequence markers and media summaries are presentation metadata, not
	// identity evidence. Keep the expressions deliberately narrow so ordinary
	// names containing numbers are left intact.
	entitySequenceNoisePattern = regexp.MustCompile(`(?i)\b(?:no|vol)\.?\s*\d+(?:\.\d+)?\b`)
	entityMediaNoisePattern    = regexp.MustCompile(`(?i)\b\d+\s*p(?:\s*\d+\s*[gv])*(?:\s*(?:-\s*|\s+)\d+(?:\.\d+)?\s*(?:[kmgt]b?|bytes?))?\b`)
)

const (
	entityMatchNone = iota
	entityMatchWeak
	entityMatchStrong
)

func entityInferenceText(value string) string {
	value = entitySequenceNoisePattern.ReplaceAllString(value, " ")
	return entityMediaNoisePattern.ReplaceAllString(value, " ")
}

func splitMultiCoserToken(value string) []string {
	runes := []rune(value)
	var parts []string
	start := 0
	foundSeparator := false
	for index, r := range runes {
		separator := r == '&' || r == '+' || r == '×'
		if (r == 'x' || r == 'X') && index > 0 && index+1 < len(runes) {
			// A Latin x inside an ordinary name (for example "Maxine") is not a
			// separator. Spaced " x " and the common Han-name form "甲x乙" are.
			separator = unicode.IsSpace(runes[index-1]) || unicode.IsSpace(runes[index+1]) ||
				(unicode.Is(unicode.Han, runes[index-1]) && unicode.Is(unicode.Han, runes[index+1]))
		}
		if !separator {
			continue
		}
		if part := strings.TrimSpace(string(runes[start:index])); part != "" {
			parts = append(parts, part)
		}
		start = index + 1
		foundSeparator = true
	}
	if !foundSeparator {
		return nil
	}
	if part := strings.TrimSpace(string(runes[start:])); part != "" {
		parts = append(parts, part)
	}
	if len(parts) < 2 {
		return nil
	}
	return parts
}

// characterTokenMatch rejects matches in the middle of an English word. A
// Han name joined directly to a longer Han phrase remains a weak fallback,
// while punctuation/whitespace-delimited occurrences are strong evidence.
func characterTokenMatch(text, token string) int {
	if token == "" || len([]rune(token)) < 2 {
		return entityMatchNone
	}
	if isDecimalEntityToken(token) {
		return decimalCharacterTokenMatch(text, token)
	}
	allowWeak := containsHanRune(token)
	best := entityMatchNone
	for offset := 0; offset <= len(text); {
		relative := strings.Index(text[offset:], token)
		if relative < 0 {
			break
		}
		start := offset + relative
		end := start + len(token)
		leftBoundary := start == 0 || !isEntityWordRune(lastRune(text[:start]))
		rightBoundary := end == len(text) || !isEntityWordRune(firstRune(text[end:]))
		if leftBoundary && rightBoundary {
			return entityMatchStrong
		}
		if allowWeak {
			best = entityMatchWeak
		}
		offset = start + len(token)
	}
	return best
}

func isDecimalEntityToken(value string) bool {
	for _, r := range value {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return value != ""
}

func decimalCharacterTokenMatch(text, token string) int {
	expanded := strings.NewReplacer(" - ", "\x00", " – ", "\x00", " — ", "\x00", " | ", "\x00", " ｜ ", "\x00").Replace(text)
	for _, segment := range strings.Split(expanded, "\x00") {
		if strings.TrimSpace(segment) == token {
			return entityMatchStrong
		}
	}
	return entityMatchNone
}

func containsHanRune(value string) bool {
	for _, r := range value {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func isEntityWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func firstRune(value string) rune {
	r, _ := utf8.DecodeRuneInString(value)
	return r
}

func lastRune(value string) rune {
	r, _ := utf8.DecodeLastRuneInString(value)
	return r
}
