package productdb

import (
	"context"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

type entityNameSpan struct{ start, end, strength int }

// Match complete catalog names in source text; never tokenize catalog names.
func entityNameSpans(text, name string, tags []string) []entityNameSpan {
	if name == "" {
		return nil
	}
	var result []entityNameSpan
	for offset := 0; offset < len(text); {
		i := strings.Index(text[offset:], name)
		if i < 0 {
			break
		}
		start, end := offset+i, offset+i+len(name)
		left := start == 0 || !isEntityWordRune(lastRune(text[:start]))
		right := end == len(text) || !isEntityWordRune(firstRune(text[end:]))
		strength := entityMatchNone
		if left && right {
			strength = entityMatchStrong
		} else if containsHanRune(name) {
			strength = entityMatchWeak
			if (left || entityTagSuffix(text[:start], tags)) && (right || entityTagPrefix(text[end:], tags)) {
				strength = entityMatchStrong
			}
		}
		if isDecimalEntityToken(name) && decimalCharacterTokenMatch(text, name) != entityMatchStrong {
			strength = entityMatchNone
		}
		if strength != entityMatchNone {
			result = append(result, entityNameSpan{start, end, strength})
		}
		offset = end
	}
	return result
}

func entityTagPrefix(text string, tags []string) bool {
	if text == "" || !isEntityWordRune(firstRune(text)) {
		return true
	}
	for _, tag := range tags {
		if strings.HasPrefix(text, tag) && entityTagPrefix(text[len(tag):], tags) {
			return true
		}
	}
	return false
}

func entityTagSuffix(text string, tags []string) bool {
	if text == "" || !isEntityWordRune(lastRune(text)) {
		return true
	}
	for _, tag := range tags {
		if strings.HasSuffix(text, tag) && entityTagSuffix(text[:len(text)-len(tag)], tags) {
			return true
		}
	}
	return false
}

func entityDescriptionTags(ctx context.Context, db discoveryQueryer) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM tags UNION SELECT alias FROM tag_aliases`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if key := normalizedKey(name); key != "" {
			tags = append(tags, key)
		}
	}
	return tags, rows.Err()
}

func entitySpanDominated(span entityNameSpan, others []entityNameSpan) bool {
	for _, other := range others {
		if other.start <= span.start && other.end >= span.end && other.end-other.start > span.end-span.start && other.strength >= span.strength {
			return true
		}
	}
	return false
}

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
