package productdb

import (
	"context"
	"database/sql"
	"errors"
	"math"

	"github.com/stashapp/stash/internal/manage"
)

type ManageStore struct{ db *sql.DB }

func (db *Database) Manage() *ManageStore { return &ManageStore{db: db.DB} }

func (s *ManageStore) GalleryPage(ctx context.Context, page int) (manage.GalleryPage, error) {
	if page < 1 || page > 1_000_000 {
		return manage.GalleryPage{}, errors.New("Manage page is out of range")
	}
	result := manage.GalleryPage{Page: page, PageSize: 24}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN gallery.state='DRAFT' THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN source.over_limit=1 THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN source.availability_state<>'AVAILABLE' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN EXISTS(SELECT 1 FROM gallery_source_issues issue WHERE issue.source_id=source.id AND issue.severity='BLOCKING' AND issue.resolved_at_utc IS NULL) THEN 1 ELSE 0 END),0),
		COALESCE(SUM((SELECT COUNT(*) FROM gallery_items item WHERE item.gallery_id=gallery.id AND item.availability_state='MISSING')),0)
		FROM galleries gallery LEFT JOIN gallery_sources source ON source.gallery_id=gallery.id`).Scan(&result.TotalItems, &result.Summary.Draft, &result.Summary.OverLimit, &result.Summary.Unavailable, &result.Summary.Blocking, &result.Summary.MissingItem); err != nil {
		return manage.GalleryPage{}, err
	}
	result.TotalPages = int(math.Ceil(float64(result.TotalItems) / 24))
	rows, err := s.db.QueryContext(ctx, manageGalleryRowSelect+` ORDER BY CASE WHEN gallery.state='DRAFT' THEN 0 WHEN source.over_limit=1 OR source.availability_state<>'AVAILABLE' THEN 1 ELSE 2 END,gallery.updated_at_utc DESC,gallery.id DESC LIMIT ? OFFSET ?`, 24, (page-1)*24)
	if err != nil {
		return manage.GalleryPage{}, err
	}
	defer rows.Close()
	for rows.Next() {
		row, err := scanManageGalleryRow(rows)
		if err != nil {
			return manage.GalleryPage{}, err
		}
		result.Items = append(result.Items, row)
	}
	return result, rows.Err()
}

func (s *ManageStore) GalleryDetail(ctx context.Context, setID string) (manage.GalleryDetail, error) {
	rows, err := s.db.QueryContext(ctx, manageGalleryRowSelect+` WHERE gallery.set_id=?`, setID)
	if err != nil {
		return manage.GalleryDetail{}, err
	}
	if !rows.Next() {
		rows.Close()
		return manage.GalleryDetail{}, ErrGalleryNotFound
	}
	row, err := scanManageGalleryRow(rows)
	if closeErr := rows.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return manage.GalleryDetail{}, err
	}
	result := manage.GalleryDetail{Row: row}
	if err := s.db.QueryRowContext(ctx, `SELECT description,COALESCE(shoot_date,''),COALESCE(shoot_date_precision,''),photographer_name,studio_name FROM galleries WHERE set_id=?`, setID).Scan(&result.Description, &result.ShootDate, &result.ShootDatePrecision, &result.PhotographerName, &result.StudioName); err != nil {
		return manage.GalleryDetail{}, err
	}
	var galleryID int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM galleries WHERE set_id=?`, setID).Scan(&galleryID); err != nil {
		return manage.GalleryDetail{}, err
	}
	result.Aliases, err = loadGalleryAliases(ctx, s.db, galleryID)
	if err != nil {
		return manage.GalleryDetail{}, err
	}
	items, err := s.db.QueryContext(ctx, `SELECT item_uuid,relative_path,media_kind,content_format,COALESCE(image_category,''),position,caption,excluded,availability_state,processing_state,byte_size
		FROM gallery_items WHERE gallery_id=? ORDER BY CASE WHEN media_kind='STATIC_IMAGE' AND image_category='PHOTO' THEN 0 WHEN media_kind='STATIC_IMAGE' THEN 1 WHEN media_kind='ANIMATED_IMAGE' THEN 2 ELSE 3 END,position,item_uuid`, galleryID)
	if err != nil {
		return manage.GalleryDetail{}, err
	}
	for items.Next() {
		var item manage.GalleryItem
		var excluded int
		if err := items.Scan(&item.UUID, &item.RelativePath, &item.MediaKind, &item.ContentFormat, &item.ImageCategory, &item.Position, &item.Caption, &excluded, &item.Availability, &item.ProcessingState, &item.ByteSize); err != nil {
			return manage.GalleryDetail{}, err
		}
		item.Excluded = excluded == 1
		result.Items = append(result.Items, item)
	}
	if err := items.Err(); err != nil {
		items.Close()
		return manage.GalleryDetail{}, err
	}
	if err := items.Close(); err != nil {
		return manage.GalleryDetail{}, err
	}
	credits, err := s.db.QueryContext(ctx, `SELECT credit.id,credit.coser_uuid,coser.name,credit.position FROM gallery_credits credit JOIN cosers coser ON coser.uuid=credit.coser_uuid WHERE credit.gallery_id=? ORDER BY credit.position,credit.id`, galleryID)
	if err != nil {
		return manage.GalleryDetail{}, err
	}
	type creditRecord struct {
		id    int64
		value manage.GalleryCredit
	}
	var creditRecords []creditRecord
	for credits.Next() {
		var record creditRecord
		if err := credits.Scan(&record.id, &record.value.CoserUUID, &record.value.CoserName, &record.value.Position); err != nil {
			credits.Close()
			return manage.GalleryDetail{}, err
		}
		creditRecords = append(creditRecords, record)
	}
	if err := credits.Close(); err != nil {
		return manage.GalleryDetail{}, err
	}
	for _, record := range creditRecords {
		castRows, err := s.db.QueryContext(ctx, `SELECT cast_item.character_uuid,character.name,character.work_uuid,work.name,cast_item.position FROM gallery_cast cast_item JOIN characters character ON character.uuid=cast_item.character_uuid JOIN works work ON work.uuid=character.work_uuid WHERE cast_item.gallery_credit_id=? ORDER BY cast_item.position,cast_item.id`, record.id)
		if err != nil {
			return manage.GalleryDetail{}, err
		}
		for castRows.Next() {
			var cast manage.GalleryCast
			if err := castRows.Scan(&cast.CharacterUUID, &cast.CharacterName, &cast.WorkUUID, &cast.WorkName, &cast.Position); err != nil {
				castRows.Close()
				return manage.GalleryDetail{}, err
			}
			record.value.Cast = append(record.value.Cast, cast)
		}
		if err := castRows.Close(); err != nil {
			return manage.GalleryDetail{}, err
		}
		result.Credits = append(result.Credits, record.value)
	}
	tagRows, err := s.db.QueryContext(ctx, `SELECT relation.tag_uuid,tag.name,relation.position FROM gallery_tags relation JOIN tags tag ON tag.uuid=relation.tag_uuid WHERE relation.gallery_id=? ORDER BY relation.position,relation.tag_uuid`, galleryID)
	if err != nil {
		return manage.GalleryDetail{}, err
	}
	for tagRows.Next() {
		var tag manage.GalleryTag
		if err := tagRows.Scan(&tag.UUID, &tag.Name, &tag.Position); err != nil {
			tagRows.Close()
			return manage.GalleryDetail{}, err
		}
		result.Tags = append(result.Tags, tag)
	}
	if err := tagRows.Close(); err != nil {
		return manage.GalleryDetail{}, err
	}
	linkRows, err := s.db.QueryContext(ctx, `SELECT link_uuid,link_type,label,url,position FROM gallery_external_links WHERE gallery_id=? ORDER BY position,link_uuid`, galleryID)
	if err != nil {
		return manage.GalleryDetail{}, err
	}
	defer linkRows.Close()
	for linkRows.Next() {
		var link manage.GalleryExternalLink
		if err := linkRows.Scan(&link.UUID, &link.Type, &link.Label, &link.URL, &link.Position); err != nil {
			return manage.GalleryDetail{}, err
		}
		result.ExternalLinks = append(result.ExternalLinks, link)
	}
	return result, linkRows.Err()
}

const manageGalleryRowSelect = `SELECT gallery.set_id,gallery.slug,gallery.state,gallery.title,COALESCE(gallery.content_rating,''),gallery.metadata_revision,gallery.scan_revision,
	CASE WHEN gallery.state='ACTIVE' AND source.availability_state='AVAILABLE' AND source.over_limit=0 AND NOT EXISTS(SELECT 1 FROM gallery_source_issues issue WHERE issue.source_id=source.id AND issue.severity='BLOCKING' AND issue.resolved_at_utc IS NULL) THEN 1 ELSE 0 END,
	COALESCE(source.source_type,''),COALESCE(source.source_path,''),COALESCE(source.availability_state,'MISSING'),COALESCE(source.reconcile_state,'NEVER_SCANNED'),COALESCE(source.over_limit,0),
	(SELECT COUNT(*) FROM gallery_items item WHERE item.gallery_id=gallery.id),(SELECT COUNT(*) FROM gallery_items item WHERE item.gallery_id=gallery.id AND item.availability_state='MISSING'),
	(SELECT COUNT(*) FROM gallery_items item WHERE item.gallery_id=gallery.id AND item.processing_state IN ('PENDING','PROCESSING')),(SELECT COUNT(*) FROM gallery_items item WHERE item.gallery_id=gallery.id AND item.processing_state='ERROR'),
	(SELECT COUNT(*) FROM gallery_source_issues issue WHERE issue.source_id=source.id AND issue.severity='BLOCKING' AND issue.resolved_at_utc IS NULL)
	FROM galleries gallery LEFT JOIN gallery_sources source ON source.gallery_id=gallery.id`

type manageRowScanner interface{ Scan(...any) error }

func scanManageGalleryRow(scanner manageRowScanner) (manage.GalleryRow, error) {
	var row manage.GalleryRow
	var browsable, overLimit int
	err := scanner.Scan(&row.SetID, &row.Slug, &row.State, &row.Title, &row.ContentRating, &row.MetadataRevision, &row.ScanRevision, &browsable, &row.SourceType, &row.SourcePath, &row.SourceAvailability, &row.ReconcileState, &overLimit, &row.ItemCount, &row.MissingCount, &row.PendingCount, &row.ErrorCount, &row.BlockingIssues)
	row.Browsable = browsable == 1
	row.OverLimit = overLimit == 1
	return row, err
}

func (s *ManageStore) GalleryID(ctx context.Context, setID string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM galleries WHERE set_id=?`, setID).Scan(&id)
	return id, err
}

// GalleryItemID resolves a member only inside the requested Gallery aggregate.
// Manage mutations use this boundary so an Item UUID can never modify a
// different Gallery through a stale editor tab.
func (s *ManageStore) GalleryItemID(ctx context.Context, setID, itemUUID string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT item.id FROM gallery_items item JOIN galleries gallery ON gallery.id=item.gallery_id WHERE gallery.set_id=? AND item.item_uuid=?`, setID, itemUUID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrGalleryItemNotFound
	}
	return id, err
}

func (s *ManageStore) GallerySourceID(ctx context.Context, setID string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT source.id FROM gallery_sources source JOIN galleries gallery ON gallery.id=source.gallery_id WHERE gallery.set_id=?`, setID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrGalleryNotFound
	}
	return id, err
}
