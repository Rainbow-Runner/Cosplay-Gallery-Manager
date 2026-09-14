package productdb

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"path/filepath"
	"sort"
	"strings"

	"github.com/stashapp/stash/internal/gallery"
	"github.com/stashapp/stash/internal/manage"
)

type ManageStore struct{ db *sql.DB }

func (db *Database) Manage() *ManageStore { return &ManageStore{db: db.DB} }

func (s *ManageStore) GalleryPage(ctx context.Context, page int, issue string) (manage.GalleryPage, error) {
	if page < 1 || page > 1_000_000 {
		return manage.GalleryPage{}, errors.New("Manage page is out of range")
	}
	condition := "1=1"
	switch issue {
	case "", "ALL":
	case "DRAFT":
		condition = "gallery.state='DRAFT'"
	case "UNAVAILABLE":
		condition = "source.id IS NULL OR source.availability_state<>'AVAILABLE'"
	case "MISSING":
		condition = "EXISTS(SELECT 1 FROM gallery_items filtered_item WHERE filtered_item.gallery_id=gallery.id AND filtered_item.availability_state='MISSING')"
	case "PROCESSING_ERROR":
		condition = "EXISTS(SELECT 1 FROM gallery_items filtered_item WHERE filtered_item.gallery_id=gallery.id AND filtered_item.processing_state='ERROR')"
	case "BLOCKING":
		condition = "EXISTS(SELECT 1 FROM gallery_source_issues filtered_issue WHERE filtered_issue.source_id=source.id AND filtered_issue.severity='BLOCKING' AND filtered_issue.resolved_at_utc IS NULL)"
	case "OVER_LIMIT":
		condition = "source.over_limit=1"
	default:
		return manage.GalleryPage{}, errors.New("unsupported Manage Gallery issue filter")
	}
	result := manage.GalleryPage{Page: page, PageSize: 24}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN gallery.state='DRAFT' THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN source.over_limit=1 THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN source.id IS NULL OR source.availability_state<>'AVAILABLE' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN EXISTS(SELECT 1 FROM gallery_source_issues issue WHERE issue.source_id=source.id AND issue.severity='BLOCKING' AND issue.resolved_at_utc IS NULL) THEN 1 ELSE 0 END),0),
		COALESCE(SUM((SELECT COUNT(*) FROM gallery_items item WHERE item.gallery_id=gallery.id AND item.availability_state='MISSING')),0)
		FROM galleries gallery LEFT JOIN gallery_sources source ON source.gallery_id=gallery.id`).Scan(&result.TotalItems, &result.Summary.Draft, &result.Summary.OverLimit, &result.Summary.Unavailable, &result.Summary.Blocking, &result.Summary.MissingItem); err != nil {
		return manage.GalleryPage{}, err
	}
	if condition != "1=1" {
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM galleries gallery LEFT JOIN gallery_sources source ON source.gallery_id=gallery.id WHERE `+condition).Scan(&result.TotalItems); err != nil {
			return manage.GalleryPage{}, err
		}
	}
	result.TotalPages = int(math.Ceil(float64(result.TotalItems) / 24))
	rows, err := s.db.QueryContext(ctx, manageGalleryRowSelect+` WHERE `+condition+` ORDER BY CASE WHEN gallery.state='DRAFT' THEN 0 WHEN source.over_limit=1 OR source.availability_state<>'AVAILABLE' THEN 1 WHEN EXISTS(SELECT 1 FROM gallery_items priority_item WHERE priority_item.gallery_id=gallery.id AND priority_item.availability_state='MISSING') THEN 2 ELSE 3 END,gallery.updated_at_utc DESC,gallery.id DESC LIMIT ? OFFSET ?`, 24, (page-1)*24)
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
	items, err := s.db.QueryContext(ctx, `SELECT item.item_uuid,item.relative_path,item.media_kind,item.content_format,COALESCE(item.image_category,''),item.position,item.caption,item.excluded,item.availability_state,item.processing_state,item.byte_size,
		COALESCE(video.probe_state,''),COALESCE(video.last_error_code,''),COALESCE(video.container,''),COALESCE(video.duration_seconds,0),COALESCE(video.display_width,0),COALESCE(video.display_height,0),COALESCE(video.video_codec,''),COALESCE(video.audio_codec,'')
		FROM gallery_items item LEFT JOIN video_technical_metadata video ON video.item_uuid=item.item_uuid WHERE item.gallery_id=? ORDER BY CASE WHEN item.media_kind='STATIC_IMAGE' AND item.image_category='PHOTO' THEN 0 WHEN item.media_kind='STATIC_IMAGE' THEN 1 WHEN item.media_kind='ANIMATED_IMAGE' THEN 2 ELSE 3 END,item.position,item.item_uuid`, galleryID)
	if err != nil {
		return manage.GalleryDetail{}, err
	}
	for items.Next() {
		var item manage.GalleryItem
		var excluded int
		if err := items.Scan(&item.UUID, &item.RelativePath, &item.MediaKind, &item.ContentFormat, &item.ImageCategory, &item.Position, &item.Caption, &excluded, &item.Availability, &item.ProcessingState, &item.ByteSize,
			&item.VideoProbeState, &item.VideoErrorCode, &item.VideoContainer, &item.VideoDurationSeconds, &item.VideoWidth, &item.VideoHeight, &item.VideoCodec, &item.AudioCodec); err != nil {
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
	scanRuns, err := s.db.QueryContext(ctx, `SELECT CAST(run.id AS TEXT),run.status,run.started_at_utc,
		COALESCE(run.completed_at_utc,''),run.error_code FROM gallery_scan_runs run
		JOIN gallery_sources source ON source.id=run.source_id WHERE source.gallery_id=? ORDER BY run.id DESC LIMIT 20`, galleryID)
	if err != nil {
		return manage.GalleryDetail{}, err
	}
	for scanRuns.Next() {
		var run manage.GalleryScanRun
		if err := scanRuns.Scan(&run.ID, &run.Status, &run.StartedAt, &run.CompletedAt, &run.ErrorCode); err != nil {
			scanRuns.Close()
			return manage.GalleryDetail{}, err
		}
		result.ScanRuns = append(result.ScanRuns, run)
	}
	if err := scanRuns.Close(); err != nil {
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
	for linkRows.Next() {
		var link manage.GalleryExternalLink
		if err := linkRows.Scan(&link.UUID, &link.Type, &link.Label, &link.URL, &link.Position); err != nil {
			linkRows.Close()
			return manage.GalleryDetail{}, err
		}
		result.ExternalLinks = append(result.ExternalLinks, link)
	}
	if err := linkRows.Err(); err != nil {
		linkRows.Close()
		return manage.GalleryDetail{}, err
	}
	if err := linkRows.Close(); err != nil {
		return manage.GalleryDetail{}, err
	}
	var markerImport int
	if err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM gallery_candidates
			WHERE gallery_id = ? AND recognition_method = 'MARKER' AND status = 'IMPORTED'
		)
	`, galleryID).Scan(&markerImport); err != nil {
		return manage.GalleryDetail{}, err
	}
	if markerImport == 1 && row.SourceType == gallery.SourceTypeDirectory {
		result.FolderMatches, err = s.folderEntityMatches(ctx, filepath.Base(filepath.Clean(row.SourcePath)))
		if err != nil {
			return manage.GalleryDetail{}, err
		}
	}
	return result, nil
}

type folderEntityToken struct {
	UUID, Name, Token, WorkUUID, WorkName string
	tokenKey                              string
}

func (s *ManageStore) folderEntityMatches(ctx context.Context, folderName string) ([]manage.GalleryFolderMatch, error) {
	folderKey := normalizedKey(folderName)
	if folderKey == "" {
		return nil, nil
	}
	cosers, err := s.matchFolderEntityKind(ctx, folderKey, "COSER", `
		SELECT uuid,name,name,'','' FROM cosers
		UNION ALL
		SELECT coser.uuid,coser.name,alias.alias,'',''
		FROM coser_aliases alias JOIN cosers coser ON coser.uuid=alias.coser_uuid
	`, nil)
	if err != nil {
		return nil, err
	}
	works, err := s.matchFolderEntityKind(ctx, folderKey, "WORK", `
		SELECT uuid,name,name,uuid,name FROM works
		UNION ALL
		SELECT work.uuid,work.name,alias.alias,work.uuid,work.name
		FROM work_aliases alias JOIN works work ON work.uuid=alias.work_uuid
	`, nil)
	if err != nil {
		return nil, err
	}
	workUUIDs := make(map[string]struct{}, len(works))
	for _, match := range works {
		workUUIDs[match.UUID] = struct{}{}
	}
	characters, err := s.matchFolderEntityKind(ctx, folderKey, "CHARACTER", `
		SELECT character.uuid,character.name,character.name,work.uuid,work.name
		FROM characters character JOIN works work ON work.uuid=character.work_uuid
		UNION ALL
		SELECT character.uuid,character.name,alias.alias,work.uuid,work.name
		FROM character_aliases alias
		JOIN characters character ON character.uuid=alias.character_uuid
		JOIN works work ON work.uuid=character.work_uuid
	`, workUUIDs)
	if err != nil {
		return nil, err
	}
	result := append(cosers, works...)
	result = append(result, characters...)
	return result, nil
}

func (s *ManageStore) matchFolderEntityKind(
	ctx context.Context,
	folderKey string,
	kind string,
	query string,
	allowedWorkUUIDs map[string]struct{},
) ([]manage.GalleryFolderMatch, error) {
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ownersByToken := map[string]map[string]folderEntityToken{}
	for rows.Next() {
		var token folderEntityToken
		if err := rows.Scan(&token.UUID, &token.Name, &token.Token, &token.WorkUUID, &token.WorkName); err != nil {
			return nil, err
		}
		if kind == "CHARACTER" && len(allowedWorkUUIDs) > 0 {
			if _, allowed := allowedWorkUUIDs[token.WorkUUID]; !allowed {
				continue
			}
		}
		token.tokenKey = normalizedKey(token.Token)
		if token.tokenKey == "" || !strings.Contains(folderKey, token.tokenKey) {
			continue
		}
		if len([]rune(token.tokenKey)) < 2 && folderKey != token.tokenKey {
			continue
		}
		if ownersByToken[token.tokenKey] == nil {
			ownersByToken[token.tokenKey] = map[string]folderEntityToken{}
		}
		ownersByToken[token.tokenKey][token.UUID] = token
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	bestByUUID := map[string]folderEntityToken{}
	for _, owners := range ownersByToken {
		if len(owners) != 1 {
			continue
		}
		for uuid, token := range owners {
			current, exists := bestByUUID[uuid]
			if !exists || len([]rune(token.tokenKey)) > len([]rune(current.tokenKey)) {
				bestByUUID[uuid] = token
			}
		}
	}
	tokens := make([]folderEntityToken, 0, len(bestByUUID))
	for _, token := range bestByUUID {
		tokens = append(tokens, token)
	}
	result := make([]manage.GalleryFolderMatch, 0, len(tokens))
	for index, token := range tokens {
		suppressed := false
		for otherIndex, other := range tokens {
			if index == otherIndex || len([]rune(other.tokenKey)) <= len([]rune(token.tokenKey)) {
				continue
			}
			if strings.Contains(other.tokenKey, token.tokenKey) {
				suppressed = true
				break
			}
		}
		if suppressed {
			continue
		}
		result = append(result, manage.GalleryFolderMatch{
			Kind: kind, UUID: token.UUID, Name: token.Name, MatchedName: token.Token,
			WorkUUID: token.WorkUUID, WorkName: token.WorkName,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].UUID < result[j].UUID
	})
	return result, nil
}

const manageGalleryRowSelect = `SELECT gallery.set_id,gallery.slug,gallery.state,gallery.title,COALESCE(gallery.content_rating,''),gallery.metadata_revision,gallery.scan_revision,
	CASE WHEN gallery.state='ACTIVE' AND source.availability_state='AVAILABLE' AND source.over_limit=0 AND NOT EXISTS(SELECT 1 FROM gallery_source_issues issue WHERE issue.source_id=source.id AND issue.severity='BLOCKING' AND issue.resolved_at_utc IS NULL) THEN 1 ELSE 0 END,
	COALESCE(source.source_type,''),COALESCE(source.source_path,''),COALESCE(source.availability_state,'MISSING'),COALESCE(source.reconcile_state,'NEVER_SCANNED'),COALESCE(source.over_limit,0),
	(SELECT COUNT(*) FROM gallery_items item WHERE item.gallery_id=gallery.id),(SELECT COUNT(*) FROM gallery_items item WHERE item.gallery_id=gallery.id AND item.availability_state='MISSING'),
	(SELECT COUNT(*) FROM gallery_items item WHERE item.gallery_id=gallery.id AND item.processing_state IN ('PENDING','PROCESSING')),(SELECT COUNT(*) FROM gallery_items item WHERE item.gallery_id=gallery.id AND item.processing_state='ERROR'),
	(SELECT COUNT(*) FROM gallery_source_issues issue WHERE issue.source_id=source.id AND issue.severity='BLOCKING' AND issue.resolved_at_utc IS NULL),
	COALESCE((SELECT run.error_code FROM gallery_scan_runs run WHERE run.source_id=source.id ORDER BY run.id DESC LIMIT 1),''),
	COALESCE((SELECT run.completed_at_utc FROM gallery_scan_runs run WHERE run.source_id=source.id ORDER BY run.id DESC LIMIT 1),'')
	FROM galleries gallery LEFT JOIN gallery_sources source ON source.gallery_id=gallery.id`

type manageRowScanner interface{ Scan(...any) error }

func scanManageGalleryRow(scanner manageRowScanner) (manage.GalleryRow, error) {
	var row manage.GalleryRow
	var browsable, overLimit int
	err := scanner.Scan(&row.SetID, &row.Slug, &row.State, &row.Title, &row.ContentRating, &row.MetadataRevision, &row.ScanRevision, &browsable, &row.SourceType, &row.SourcePath, &row.SourceAvailability, &row.ReconcileState, &overLimit, &row.ItemCount, &row.MissingCount, &row.PendingCount, &row.ErrorCount, &row.BlockingIssues, &row.LastScanErrorCode, &row.LastScanCompleted)
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
