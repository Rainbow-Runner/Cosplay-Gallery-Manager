package productdb

import (
	"context"
	"regexp"
	"strings"

	"github.com/stashapp/stash/internal/archivefile"
)

// A narrowly scoped naming convention, not a general claim that Ft. identifies
// character ownership. Main Coser plays role 1; featured Coser plays role 2.
var featuredCastTitle = regexp.MustCompile(`(?i)^\s*(.+?)\s+[-–—]\s+(.+?)\s*&\s*(.+?)\s*[（(]\s*(?:ft\.?|feat\.?|featuring)\s+(.+?)\s*[）)]\s*(?:\[[^\]]*\]\s*)*$`)
var featuredCastMarker = regexp.MustCompile(`(?i)[（(]\s*(?:ft\.|feat\.|ft\b|feat\b|featuring\b)`)

func featuredCastParts(title string) ([]string, bool) {
	title = normalizedDisplay(title)
	if archivefile.IsSupportedPath(title) {
		title = archivefile.BaseName(title)
	}
	featured := featuredCastMarker.MatchString(title)
	parts := featuredCastTitle.FindStringSubmatch(title)
	return parts, featured
}

func featuredCastOwners(ctx context.Context, db discoveryQueryer, title string, cosers []string, works map[string]bool) (map[string]string, bool, error) {
	parts, featured := featuredCastParts(title)
	if len(parts) == 0 || len(cosers) != 2 {
		return nil, featured, nil
	}
	var ids [4]string
	for i, value := range parts[1:] {
		table, aliases, owner := "cosers", "coser_aliases", "coser_uuid"
		if i == 1 || i == 2 {
			table, aliases, owner = "characters", "character_aliases", "character_uuid"
		}
		matches, err := exactEntityUUIDs(ctx, db, table, aliases, owner, strings.TrimSpace(value), "")
		if err != nil {
			return nil, featured, err
		}
		if table == "characters" && len(works) > 0 {
			var contextual []string
			for _, uuid := range matches {
				rows, err := db.QueryContext(ctx, `SELECT work_uuid FROM characters WHERE uuid=?`, uuid)
				if err != nil {
					return nil, featured, err
				}
				var work string
				if rows.Next() {
					err = rows.Scan(&work)
				}
				rowErr := rows.Err()
				rows.Close()
				if err != nil {
					return nil, featured, err
				}
				if rowErr != nil {
					return nil, featured, rowErr
				}
				if works[work] {
					contextual = append(contextual, uuid)
				}
			}
			matches = contextual
		}
		if len(matches) != 1 {
			return nil, featured, nil
		}
		ids[i] = matches[0]
	}
	if ids[0] == ids[3] || ids[1] == ids[2] {
		return nil, featured, nil
	}
	if !(cosers[0] == ids[0] && cosers[1] == ids[3] || cosers[1] == ids[0] && cosers[0] == ids[3]) {
		return nil, featured, nil
	}
	return map[string]string{ids[1]: ids[0], ids[2]: ids[3]}, featured, nil
}
