package productdb

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/stashapp/stash/internal/browse"
)

var (
	albumSequence = regexp.MustCompile(`(?i)(?:no|vol)\.?\s*\d+(?:\.\d+)?`)
	// Statistics are removed only when the entire original [] block is made
	// of counts/capacities. Never extract numbers from explanatory annotations.
	albumSquareBlocks = regexp.MustCompile(`\[[^\]]*\]`)
	albumPureStats    = regexp.MustCompile(`(?i)^\s*(?:\d+\s*(?:gif|[pvg])|\d+(?:\.\d+)?\s*(?:[kmgt](?:i?b)?|bytes?))(?:\s*[-－–—+,/&:：]?\s*(?:\d+\s*(?:gif|[pvg])|\d+(?:\.\d+)?\s*(?:[kmgt](?:i?b)?|bytes?)))*\s*$`)
	// Delete only empty/separator-only bracket pairs left after removing identity
	// metadata. Do not strip brackets around meaningful description text.
	albumEmptyBrackets = regexp.MustCompile(`\([\s\p{Z}\-–—_·|｜/\\,:：;；&+×.]*\)|（[\s\p{Z}\-–—_·|｜/\\,:：;；&+×.]*）|\[[\s\p{Z}\-–—_·|｜/\\,:：;；&+×.]*\]|【[\s\p{Z}\-–—_·|｜/\\,:：;；&+×.]*】|\{[\s\p{Z}\-–—_·|｜/\\,:：;；&+×.]*\}|「[\s\p{Z}\-–—_·|｜/\\,:：;；&+×.]*」|『[\s\p{Z}\-–—_·|｜/\\,:：;；&+×.]*』`)
)

// Display-only derivation: never changes identity, source names or searchable aliases.
func albumDisplayTitle(title string, aliases, names []string) string {
	for _, alias := range aliases {
		if value := strings.TrimSpace(alias); value != "" {
			return value
		}
	}
	text := normalizedDisplay(title)
	text = albumSquareBlocks.ReplaceAllStringFunc(text, func(block string) string {
		if albumPureStats.MatchString(block[1 : len(block)-1]) {
			return " "
		}
		return block
	})
	names = append([]string(nil), names...)
	sort.SliceStable(names, func(i, j int) bool { return runeLength(names[i]) > runeLength(names[j]) })
	for _, name := range names {
		name = normalizedDisplay(name)
		if name == "" {
			continue
		}
		pattern := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(name))
		spans := pattern.FindAllStringIndex(text, -1)
		for i := len(spans) - 1; i >= 0; i-- {
			start, end := spans[i][0], spans[i][1]
			if (start == 0 || !isEntityWordRune(lastRune(text[:start]))) && (end == len(text) || !isEntityWordRune(firstRune(text[end:]))) {
				text = text[:start] + " " + text[end:]
			}
		}
	}
	spans := albumSequence.FindAllStringIndex(text, -1)
	for i := len(spans) - 1; i >= 0; i-- {
		start, end := spans[i][0], spans[i][1]
		// Do not erase embedded words (e.g. SuperNo.123 or Vol.12abc).
		if (start == 0 || !isEntityWordRune(lastRune(text[:start]))) && (end == len(text) || !isEntityWordRune(firstRune(text[end:]))) {
			text = text[:start] + " " + text[end:]
		}
	}
	for {
		cleaned := albumEmptyBrackets.ReplaceAllString(text, " ")
		if cleaned == text {
			break
		}
		text = cleaned // Repeating also clears nested pairs that become empty.
	}
	text = strings.Trim(strings.Join(strings.Fields(text), " "), " -–—_·|｜/\\,:：;；&+×.")
	if text == "" {
		return title
	}
	return text
}

func (s *BrowseStore) populateCardDisplayTitle(ctx context.Context, galleryID int64, card *browse.GalleryCard) error {
	card.DisplayTitle = card.Title
	if card.CollectionType != browse.CollectionAlbum {
		return nil
	}
	aliases, err := loadGalleryAliases(ctx, s.db, galleryID)
	if err != nil {
		return err
	}
	for _, alias := range aliases {
		if strings.TrimSpace(alias) != "" {
			card.DisplayTitle = albumDisplayTitle(card.Title, aliases, nil)
			return nil
		}
	}
	// Include every linked Coser, not just the three visible card credits.
	rows, err := s.db.QueryContext(ctx, `SELECT coser.name FROM gallery_credits credit JOIN cosers coser ON coser.uuid=credit.coser_uuid WHERE credit.gallery_id=?
		UNION SELECT alias.alias FROM gallery_credits credit JOIN coser_aliases alias ON alias.coser_uuid=credit.coser_uuid WHERE credit.gallery_id=?`, galleryID, galleryID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	card.DisplayTitle = albumDisplayTitle(card.Title, aliases, names)
	return nil
}
