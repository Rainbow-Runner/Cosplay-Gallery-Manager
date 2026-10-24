package productdb

import "github.com/stashapp/stash/internal/mediaprocessing"

// Prefer a valid current-contract 960 base; old galleries retain the
// legacy 480 fallback. itemUUIDSQL is an internal SQL expression, never input.
func staticBaseVariantSQL(itemUUIDSQL string) string {
	return `CASE WHEN EXISTS(SELECT 1 FROM media_derivatives base960 JOIN gallery_items base_item ON base_item.item_uuid=base960.item_uuid
		WHERE base960.item_uuid=` + itemUUIDSQL + ` AND base_item.media_kind='STATIC_IMAGE' AND base_item.content_format IN ('IMAGE','RAW')
		AND base960.variant='CARD_DISPLAY_960' AND base960.profile_hash='` + mediaprocessing.DisplayProfileHash() + `'
		AND base960.content_revision=base_item.content_revision AND base960.is_current=1 AND base960.state='READY')
		THEN 'CARD_DISPLAY_960' ELSE 'CARD_480' END`
}
