package productdb

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/stashapp/stash/internal/browse"
)

type TimelineEntry struct {
	Month string
	Card  browse.GalleryCard
}

type TimelineConnection struct {
	Items       []TimelineEntry
	EndCursor   string
	HasNextPage bool
}

type timelineCursor struct {
	Version   int
	Scope     browse.Scope
	Coser     string
	Date      browse.TimelineDate
	Primary   string
	Secondary string
	ID        int64
}

// CoserTimeline uses bounded keyset reads, not a full-history response or OFFSET.
func (s *BrowseStore) CoserTimeline(ctx context.Context, scope browse.Scope, coser string, date browse.TimelineDate, first int, after string) (TimelineConnection, error) {
	result := TimelineConnection{Items: []TimelineEntry{}}
	if coser == "" || len(coser) > 128 || first < 1 || first > browseGalleryPageSize || len(after) > 2048 {
		return result, errors.New("invalid timeline request")
	}
	extra, primary, err := timelineDateSQL(date)
	if err != nil {
		return result, err
	}
	scopeSQL, scopeArg, err := browseScopePredicate(scope)
	if err != nil {
		return result, err
	}
	args := []any{}
	if scopeArg != "" {
		args = append(args, scopeArg)
	}
	extra += ` AND EXISTS(SELECT 1 FROM gallery_credits credit WHERE credit.gallery_id=gallery.id AND credit.coser_uuid=?)`
	args = append(args, coser)
	if after != "" {
		raw, err := base64.RawURLEncoding.DecodeString(after)
		if err != nil {
			return result, errors.New("invalid timeline cursor")
		}
		var cursor timelineCursor
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&cursor) != nil || decoder.Decode(new(any)) != io.EOF || cursor.Version != 1 || cursor.Scope != scope || cursor.Coser != coser || cursor.Date != date || cursor.ID < 1 || len(cursor.Primary) < 10 || len(cursor.Primary) > 64 || len(cursor.Secondary) < 10 || len(cursor.Secondary) > 64 {
			return result, errors.New("invalid timeline cursor")
		}
		extra += ` AND (` + primary + `,` + timelineSecondaryOrder + `,gallery.id) < (?,?,?)`
		args = append(args, cursor.Primary, cursor.Secondary, cursor.ID)
	}
	args = append(args, first+1)
	rows, err := s.db.QueryContext(ctx, `SELECT gallery.id,gallery.set_id,`+primary+`,`+timelineSecondaryOrder+`
		FROM galleries gallery JOIN gallery_sources source ON source.gallery_id=gallery.id
		LEFT JOIN gallery_personal_states personal ON personal.gallery_id=gallery.id
		WHERE `+browseVisibleGalleryPredicate+scopeSQL+extra+` ORDER BY `+primary+` DESC,`+timelineSecondaryOrder+` DESC,gallery.id DESC LIMIT ?`, args...)
	if err != nil {
		return result, err
	}
	type row struct {
		id                        int64
		setID, primary, secondary string
	}
	entries := []row{}
	for rows.Next() {
		var entry row
		if err := rows.Scan(&entry.id, &entry.setID, &entry.primary, &entry.secondary); err != nil {
			rows.Close()
			return result, err
		}
		entries = append(entries, entry)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	result.HasNextPage = len(entries) > first
	if result.HasNextPage {
		entries = entries[:first]
	}
	if len(entries) == 0 {
		return result, nil
	}
	ids, placeholders := []any{}, []string{}
	for _, entry := range entries {
		ids = append(ids, entry.id)
		placeholders = append(placeholders, "?")
	}
	// Hydrate only this bounded selection through the existing visibility/card path.
	page, err := s.galleryPage(ctx, scope, 1, browse.GallerySortShootDate, ` AND gallery.id IN (`+strings.Join(placeholders, ",")+`)`, ids, primary+` DESC,`+timelineSecondaryOrder+` DESC,gallery.id DESC`)
	if err != nil {
		return result, err
	}
	months := map[string]string{}
	for _, entry := range entries {
		months[entry.setID] = entry.primary[:7]
	}
	for _, card := range page.Items {
		result.Items = append(result.Items, TimelineEntry{Month: months[card.SetID], Card: card})
	}
	last := entries[len(entries)-1]
	raw, err := json.Marshal(timelineCursor{Version: 1, Scope: scope, Coser: coser, Date: date, Primary: last.primary, Secondary: last.secondary, ID: last.id})
	if err != nil {
		return result, err
	}
	result.EndCursor = base64.RawURLEncoding.EncodeToString(raw)
	return result, nil
}
