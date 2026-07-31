package productapi

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/stashapp/stash/internal/browse"
	"github.com/stashapp/stash/internal/discovery"
	"github.com/stashapp/stash/internal/library"
	"github.com/stashapp/stash/internal/manage"
	"github.com/stashapp/stash/internal/persistence/productdb"
	"github.com/stashapp/stash/internal/settings"
)

func publicError(err error) error {
	if errors.Is(err, productdb.ErrBrowseGalleryNotVisible) {
		return errors.New("not found")
	}
	return errors.New("internal server error")
}

func galleryPage(value browse.GalleryPage) *GalleryPage {
	result := &GalleryPage{Page: value.Page, PageSize: value.PageSize, TotalItems: value.TotalItems, TotalPages: value.TotalPages}
	for _, item := range value.Items {
		result.Items = append(result.Items, galleryCard(item))
	}
	return result
}

func galleryCard(value browse.GalleryCard) *BrowseGalleryCard {
	result := &BrowseGalleryCard{
		SetID: value.SetID, Slug: value.Slug, Title: value.Title,
		CollectionType: CollectionType(value.CollectionType), ContentRating: ContentRating(value.ContentRating),
		Cover: &GalleryCover{Kind: string(value.Cover.Kind), Revision: value.Cover.Revision, Managed: value.Cover.Managed,
			Warning: value.Cover.Warning, Resource: resourceIdentity(value.Cover.Resource)},
		Credits: entitySummaries(value.Credits), CreditCount: value.CreditCount,
		Characters: entitySummaries(value.Characters), CharacterCount: value.CharacterCount,
		Works: entitySummaries(value.Works), WorkCount: value.WorkCount,
		ShootDate: value.ShootDate, AddedAtUtc: value.AddedAtUTC.UTC().Format("2006-01-02T15:04:05Z"),
		Media:    &MediaCounts{Photo: value.Media.Photo, Selfie: value.Media.Selfie, Gif: value.Media.GIF, Video: value.Media.Video},
		Favorite: value.Favorite, RatingHalfSteps: value.RatingHalfSteps,
		ScrubberCount: value.ScrubberCount, ScrubberRevision: value.ScrubberRevision,
	}
	if value.ShootDatePrecision == "" {
		result.ShootDatePrecision = ShootDatePrecisionUnknown
	} else {
		result.ShootDatePrecision = ShootDatePrecision(value.ShootDatePrecision)
	}
	return result
}

func resourceIdentity(value *browse.ResourceIdentity) *ResourceIdentity {
	if value == nil {
		return nil
	}
	return &ResourceIdentity{ItemUUID: value.ItemUUID, ContentRevision: value.ContentRevision, ProfileHash: value.ProfileHash,
		Variant: value.Variant, MimeType: value.MIMEType}
}

func onDemandResource(value browse.OnDemandResource) *OnDemandResource {
	return &OnDemandResource{Status: ProcessingState(value.Status), Resource: resourceIdentity(value.Resource), ErrorCode: value.ErrorCode}
}

func entitySummary(value browse.EntitySummary) *EntitySummary {
	return &EntitySummary{UUID: value.UUID, Name: value.Name}
}

func entitySummaries(values []browse.EntitySummary) []*EntitySummary {
	result := make([]*EntitySummary, 0, len(values))
	for _, value := range values {
		result = append(result, entitySummary(value))
	}
	return result
}

func galleryDetail(value browse.GalleryDetail) *GalleryDetail {
	result := &GalleryDetail{Card: galleryCard(value.Card), Description: value.Description, PhotographerName: value.PhotographerName,
		StudioName: value.StudioName, AvailableBytes: value.AvailableBytes, Tags: entitySummaries(value.Tags), Redirected: value.Redirected}
	for _, credit := range value.Credits {
		result.Credits = append(result.Credits, &GalleryCreditDetail{Coser: entitySummary(credit.Coser),
			Characters: entitySummaries(credit.Characters), Works: entitySummaries(credit.Works)})
	}
	for _, link := range value.ExternalLinks {
		result.ExternalLinks = append(result.ExternalLinks, &ExternalLink{UUID: link.UUID, Type: string(link.Type), Label: link.Label, URL: link.URL})
	}
	return result
}

func galleryMemberIndex(value browse.GalleryMemberIndex) *GalleryMemberIndex {
	result := &GalleryMemberIndex{SetID: value.SetID, MetadataRevision: value.MetadataRevision, ScanRevision: value.ScanRevision}
	for _, item := range value.Items {
		result.Items = append(result.Items, galleryMemberModel(item))
	}
	return result
}

func galleryMemberModel(item browse.GalleryMember) *GalleryMember {
	var category *ImageCategory
	if item.ImageCategory != "" {
		converted := ImageCategory(item.ImageCategory)
		category = &converted
	}
	return &GalleryMember{ItemUUID: item.ItemUUID, MediaKind: MediaKind(item.MediaKind), ContentFormat: ContentFormat(item.ContentFormat),
		ImageCategory: category, Position: strconv.FormatInt(item.Position, 10), Caption: item.Caption,
		ProcessingState: ProcessingState(item.ProcessingState), CardResource: resourceIdentity(item.CardResource),
		LargeResource: resourceIdentity(item.LargeResource), Favorite: item.Favorite, RatingHalfSteps: item.RatingHalfSteps}
}

func mergeRecommendations(strong, tags []browse.GalleryRecommendation) []*GalleryRecommendation {
	bySetID := make(map[string]*GalleryRecommendation, len(strong)+len(tags))
	for _, value := range append(strong, tags...) {
		existing := bySetID[value.Card.SetID]
		if existing == nil {
			existing = &GalleryRecommendation{Card: galleryCard(value.Card), Score: value.Score}
			bySetID[value.Card.SetID] = existing
		} else if value.Score > existing.Score {
			existing.Score = value.Score
		}
		for _, reason := range value.Reasons {
			existing.Reasons = append(existing.Reasons, string(reason))
		}
	}
	result := make([]*GalleryRecommendation, 0, len(bySetID))
	for _, value := range bySetID {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Score != result[j].Score {
			return result[i].Score > result[j].Score
		}
		return result[i].Card.SetID < result[j].Card.SetID
	})
	return result
}

func randomMediaItem(value browse.RandomMediaItem) *RandomMediaItem {
	var category *ImageCategory
	if value.ImageCategory != "" {
		converted := ImageCategory(value.ImageCategory)
		category = &converted
	}
	return &RandomMediaItem{ItemUUID: value.ItemUUID, MediaKind: MediaKind(value.MediaKind), ImageCategory: category,
		Resource: resourceIdentity(&value.Resource), GallerySetID: value.GallerySetID, GallerySlug: value.GallerySlug,
		Characters: entitySummaries(value.Characters), Cosers: entitySummaries(value.Cosers), Favorite: value.Favorite,
		RatingHalfSteps: value.RatingHalfSteps}
}

func entityPage(value browse.EntityPage) *EntityPage {
	result := &EntityPage{Page: value.Page, PageSize: value.PageSize, TotalItems: value.TotalItems, TotalPages: value.TotalPages}
	for _, item := range value.Items {
		result.Items = append(result.Items, &EntityIndexItem{Kind: SearchEntityKind(item.Kind), UUID: item.UUID,
			Slug: item.Slug, Name: item.Name, Aliases: item.Aliases})
	}
	return result
}

func searchPreviewModel(value browse.SearchPreview) *SearchPreview {
	return &SearchPreview{Scope: BrowseScope(value.Scope), Query: value.Query, Galleries: searchHits(value.Galleries),
		Cosers: searchHits(value.Cosers), Works: searchHits(value.Works), Characters: searchHits(value.Characters), Tags: searchHits(value.Tags)}
}

func searchHits(values []browse.SearchHit) []*SearchHit {
	result := make([]*SearchHit, 0, len(values))
	for _, value := range values {
		result = append(result, &SearchHit{Kind: SearchEntityKind(value.Kind), UUID: value.UUID, Slug: value.Slug,
			Name: value.Name, MatchLevel: value.MatchLevel})
	}
	return result
}

func entityIndexItem(value browse.EntityIndexItem) *EntityIndexItem {
	result := &EntityIndexItem{Kind: SearchEntityKind(value.Kind), UUID: value.UUID, Slug: value.Slug, Name: value.Name, Aliases: value.Aliases}
	if value.AvatarAvailable {
		url := coserAssetResourceURL(value.UUID, value.AssetRevision, "avatar-480")
		result.AvatarURL = &url
	}
	return result
}

func coserDetailModel(value browse.CoserDetail) *CoserDetail {
	result := &CoserDetail{Entity: entityIndexItem(value.Entity), ProfileSummary: value.ProfileSummary, Biography: value.Biography,
		CountryOrRegion: value.CountryOrRegion, Galleries: galleryPage(value.Galleries), Redirected: value.Redirected}
	if value.BannerAvailable {
		url := coserAssetResourceURL(value.Entity.UUID, value.Entity.AssetRevision, "banner-1600")
		result.BannerURL = &url
	}
	for _, account := range value.SocialAccounts {
		result.SocialAccounts = append(result.SocialAccounts, &SocialAccount{UUID: account.UUID, PlatformKey: account.PlatformKey,
			Label: account.Label, Handle: account.Handle, URL: account.URL, Status: account.Status, Position: strconv.FormatInt(account.Position, 10)})
	}
	return result
}

func workDetailModel(value browse.WorkDetail) *WorkDetail {
	result := &WorkDetail{Entity: entityIndexItem(value.Entity), Redirected: value.Redirected}
	for _, character := range value.Characters {
		result.Characters = append(result.Characters, entityIndexItem(character))
	}
	return result
}

func characterDetailModel(value browse.CharacterDetail) *CharacterDetail {
	return &CharacterDetail{Entity: entityIndexItem(value.Entity), Work: entityIndexItem(value.Work), Galleries: galleryPage(value.Galleries), Redirected: value.Redirected}
}

func tagDetailModel(value browse.TagDetail) *TagDetail {
	return &TagDetail{Entity: entityIndexItem(value.Entity), Galleries: galleryPage(value.Galleries), Redirected: value.Redirected}
}

func mediaPage(value browse.MediaPage) *MediaPage {
	result := &MediaPage{Page: value.Page, PageSize: value.PageSize, TotalItems: value.TotalItems, TotalPages: value.TotalPages}
	for _, item := range value.Items {
		result.Items = append(result.Items, randomMediaItem(item))
	}
	return result
}

func manageError(err error) error {
	var activation *productdb.ActivationError
	if errors.As(err, &activation) {
		codes := make([]string, 0, len(activation.Blockers))
		for _, blocker := range activation.Blockers {
			codes = append(codes, blocker.Code)
		}
		return errors.New("activation blocked: " + strings.Join(codes, ","))
	}
	if errors.Is(err, productdb.ErrMetadataRevisionConflict) {
		return errors.New("metadata revision conflict")
	}
	if errors.Is(err, productdb.ErrGalleryDeleteRequiresArchived) {
		return errors.New("Gallery must be ARCHIVED before permanent deletion")
	}
	if errors.Is(err, productdb.ErrCoreMetadataRevisionConflict) {
		return errors.New("core entity metadata revision conflict")
	}
	if errors.Is(err, productdb.ErrCoreEntityMergeConflict) {
		return errors.New("core entity merge has unresolved conflicts")
	}
	if errors.Is(err, productdb.ErrCoreEntityReferenced) {
		return errors.New("core entity is still referenced")
	}
	if errors.Is(err, productdb.ErrPortableUUIDNotActive) {
		return errors.New("core entity is no longer active")
	}
	return publicError(err)
}

func manageGalleryPage(value manage.GalleryPage) *ManageGalleryPage {
	result := &ManageGalleryPage{Page: value.Page, PageSize: value.PageSize, TotalItems: value.TotalItems, TotalPages: value.TotalPages,
		Summary: &ManageIssueSummary{Draft: value.Summary.Draft, OverLimit: value.Summary.OverLimit, Unavailable: value.Summary.Unavailable, Blocking: value.Summary.Blocking, MissingItem: value.Summary.MissingItem}}
	for _, row := range value.Items {
		result.Items = append(result.Items, manageGalleryRow(row))
	}
	return result
}
func manageGalleryRow(value manage.GalleryRow) *ManageGalleryRow {
	var rating *ContentRating
	if value.ContentRating != "" {
		converted := ContentRating(value.ContentRating)
		rating = &converted
	}
	return &ManageGalleryRow{SetID: value.SetID, Slug: value.Slug, State: GalleryState(value.State), Title: value.Title, ContentRating: rating,
		MetadataRevision: value.MetadataRevision, ScanRevision: value.ScanRevision, Browsable: value.Browsable, SourceType: string(value.SourceType), SourcePath: value.SourcePath,
		SourceAvailability: string(value.SourceAvailability), ReconcileState: string(value.ReconcileState), OverLimit: value.OverLimit, ItemCount: value.ItemCount,
		MissingCount: value.MissingCount, PendingCount: value.PendingCount, ErrorCount: value.ErrorCount, BlockingIssues: value.BlockingIssues}
}
func manageGalleryDetail(value manage.GalleryDetail) *ManageGalleryDetail {
	precision := ShootDatePrecisionUnknown
	if value.ShootDatePrecision != "" {
		precision = ShootDatePrecision(value.ShootDatePrecision)
	}
	result := &ManageGalleryDetail{Row: manageGalleryRow(value.Row), Aliases: value.Aliases, Description: value.Description, ShootDate: value.ShootDate,
		ShootDatePrecision: precision, PhotographerName: value.PhotographerName, StudioName: value.StudioName}
	for _, item := range value.Items {
		var category *ImageCategory
		if item.ImageCategory != "" {
			converted := ImageCategory(item.ImageCategory)
			category = &converted
		}
		result.Items = append(result.Items, &ManageGalleryItem{UUID: item.UUID, RelativePath: item.RelativePath, MediaKind: MediaKind(item.MediaKind), ContentFormat: ContentFormat(item.ContentFormat),
			ImageCategory: category, Position: strconv.FormatInt(item.Position, 10), Caption: item.Caption, Excluded: item.Excluded, Availability: string(item.Availability),
			ProcessingState: ProcessingState(item.ProcessingState), ByteSize: item.ByteSize})
	}
	for _, credit := range value.Credits {
		convertedCredit := &ManageGalleryCredit{CoserUUID: credit.CoserUUID, CoserName: credit.CoserName, Position: strconv.FormatInt(credit.Position, 10)}
		for _, cast := range credit.Cast {
			convertedCredit.Cast = append(convertedCredit.Cast, &ManageGalleryCast{
				CharacterUUID: cast.CharacterUUID, CharacterName: cast.CharacterName,
				WorkUUID: cast.WorkUUID, WorkName: cast.WorkName, Position: strconv.FormatInt(cast.Position, 10),
			})
		}
		result.Credits = append(result.Credits, convertedCredit)
	}
	for _, match := range value.FolderMatches {
		result.FolderMatches = append(result.FolderMatches, &ManageGalleryFolderMatch{
			Kind: SearchEntityKind(match.Kind), UUID: match.UUID, Name: match.Name,
			MatchedName: match.MatchedName, WorkUUID: match.WorkUUID, WorkName: match.WorkName,
		})
	}
	for _, tag := range value.Tags {
		result.Tags = append(result.Tags, &ManageGalleryTag{UUID: tag.UUID, Name: tag.Name, Position: strconv.FormatInt(tag.Position, 10)})
	}
	for _, link := range value.ExternalLinks {
		result.ExternalLinks = append(result.ExternalLinks, &ManageGalleryExternalLink{
			UUID: link.UUID, Type: link.Type, Label: link.Label, URL: link.URL, Position: strconv.FormatInt(link.Position, 10),
		})
	}
	return result
}

func manageLibrary(value library.Library, rules []discovery.Rule) *ManageLibrary {
	result := &ManageLibrary{ID: value.ID, Name: value.Name, RootPath: value.RootPath, Enabled: value.Enabled, ReadOnly: value.ReadOnly, CaptureTimezone: value.CaptureTimezone}
	for _, rule := range rules {
		result.Rules = append(result.Rules, manageRecognitionRule(rule))
	}
	return result
}

func manageRecognitionRule(value discovery.Rule) *ManageRecognitionRule {
	return &ManageRecognitionRule{ID: value.ID, Name: value.Name, Kind: string(value.Kind), Enabled: value.Enabled, AutoCreateDraft: value.AutoCreateDraft, Order: value.Order, Pattern: value.Pattern, FixedDepth: value.FixedDepth}
}

func manageDiscoverySnapshot(value productdb.DiscoverySnapshot) *ManageDiscoverySnapshot {
	result := &ManageDiscoverySnapshot{ID: value.ID, LibraryID: value.LibraryID}
	if !value.CompletedAt.IsZero() {
		result.CompletedAt = value.CompletedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	for _, candidate := range value.Candidates {
		converted := &ManageCandidate{ID: candidate.ID, RootPath: candidate.RootPath, SourceType: string(candidate.SourceType), Method: candidate.Method, Status: candidate.Status, AutoCreateDraft: candidate.AutoCreateDraft, HasConflict: candidate.HasConflict, OverLimit: candidate.OverLimit, MediaCount: candidate.MediaCount}
		if candidate.ManifestSetID != "" {
			converted.ManifestSetID = &candidate.ManifestSetID
		}
		for _, suggestion := range candidate.Suggestions {
			converted.Suggestions = append(converted.Suggestions, &ManageSuggestion{Field: suggestion.Field, Value: suggestion.Value})
		}
		result.Candidates = append(result.Candidates, converted)
	}
	for _, diagnostic := range value.Unassigned {
		result.Unassigned = append(result.Unassigned, &ManageUnassignedDiagnostic{ParentPath: diagnostic.ParentPath, MediaCount: diagnostic.MediaCount})
	}
	return result
}

func manageRuntimeSettings(value settings.Runtime) *ManageRuntimeSettings {
	return &ManageRuntimeSettings{SettingsRevision: value.Revision, HomeScope: BrowseScope(value.HomeScope), GalleryCardScrubberEnabled: value.GalleryCardScrubberEnabled,
		GalleryDetailMediaFilterEnabled: value.GalleryDetailMediaFilterEnabled, GalleryCardControlsVisible: value.GalleryCardControlsVisible,
		MediaCardControlsVisible: value.MediaCardControlsVisible, DetailPersonalControlsVisible: value.DetailPersonalControlsVisible,
		RelatedLimit: value.RelatedLimit, TagParentWeight: value.TagParentWeight, TagMinimumScore: value.TagMinimumScore, TagMaximumDepth: value.TagMaximumDepth,
		RandomLimit: value.RandomLimit, RandomStaticQuota: value.RandomStaticQuota, RandomGIFQuota: value.RandomGIFQuota, RandomVideoQuota: value.RandomVideoQuota,
		RandomGalleryRepeatDecay: value.RandomGalleryRepeatDecay, EnhancedCacheMaximumBytes: value.EnhancedCacheMaximumBytes, MinimumFreeBytes: value.MinimumFreeBytes,
		MinimumFreePercent: value.MinimumFreePercent, AutomaticScanEnabled: value.AutomaticScanEnabled, AutomaticSchedulesSuspended: value.AutomaticSchedulesSuspended,
		DailyBackupEnabled: value.DailyBackupEnabled, DailyBackupRetention: value.DailyBackupRetention,
		ArchiveMaxEntries: value.ArchiveMaxEntries, ArchiveMaxEntryBytes: value.ArchiveMaxEntryBytes, ArchiveMaxTotalBytes: value.ArchiveMaxTotalBytes,
		ArchiveMaxCompressionRatio: value.ArchiveMaxCompressionRatio, ArchiveMaxImagePixels: value.ArchiveMaxImagePixels}
}

func manageProcessingJobPage(value productdb.ProcessingJobPage) *ManageProcessingJobPage {
	result := &ManageProcessingJobPage{Page: value.Page, PageSize: value.PageSize, TotalItems: value.TotalItems, TotalPages: value.TotalPages}
	for _, job := range value.Items {
		converted := &ManageProcessingJob{ID: job.ID, Kind: string(job.Kind), GalleryID: job.GalleryID, Variant: job.Variant, Status: string(job.Status), Priority: job.Priority,
			AttemptCount: job.AttemptCount, MaxAttempts: job.MaxAttempts, LastErrorCode: job.LastErrorCode, StructuralFailure: job.StructuralFailure,
			CreatedAt: job.CreatedAtUTC.UTC().Format("2006-01-02T15:04:05Z"), UpdatedAt: job.UpdatedAtUTC.UTC().Format("2006-01-02T15:04:05Z")}
		if job.ItemUUID != "" {
			converted.ItemUUID = &job.ItemUUID
		}
		result.Items = append(result.Items, converted)
	}
	return result
}

func manageBackupRecord(value productdb.BackupRecord) *ManageBackupRecord {
	result := &ManageBackupRecord{
		ID: value.ID, Kind: string(value.Kind), FileName: value.FileName, Status: value.Status, ByteSize: value.ByteSize, ArchiveSha256: value.ArchiveSHA256,
		ProductVersion: value.ProductVersion, DatabaseSchemaVersion: value.DatabaseSchemaVersion,
		ManifestSchemaVersion: value.ManifestSchemaVersion, MediaProcessingVersion: value.MediaProcessingVersion,
		CreatedAt: value.CreatedAtUTC.UTC().Format("2006-01-02T15:04:05Z"), LastErrorCode: value.LastErrorCode,
	}
	if value.CompletedAtUTC != nil {
		completed := value.CompletedAtUTC.UTC().Format("2006-01-02T15:04:05Z")
		result.CompletedAt = &completed
	}
	return result
}

func manageMaintenanceState(value productdb.MaintenanceState) *ManageMaintenanceState {
	result := &ManageMaintenanceState{
		State: string(value.Mode), LastErrorCode: value.LastErrorCode,
		UpdatedAt: value.UpdatedAtUTC.UTC().Format("2006-01-02T15:04:05Z"),
	}
	if value.RestoreBackupID != "" {
		result.RestoreBackupID = &value.RestoreBackupID
	}
	return result
}

func manageAuditPage(value productdb.AuditPage) *ManageAuditPage {
	result := &ManageAuditPage{Page: value.Page, PageSize: value.PageSize, TotalItems: value.TotalItems, TotalPages: value.TotalPages}
	for _, event := range value.Items {
		result.Items = append(result.Items, &ManageAuditEvent{
			ID: event.ID, EventCode: event.EventCode, TargetKind: event.TargetKind, TargetID: event.TargetID,
			Outcome: event.Outcome, ErrorCode: event.ErrorCode, SummaryJSON: event.SummaryJSON,
			CreatedAt: event.CreatedAtUTC.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	return result
}

func manageCoreEntity(value productdb.ManageCoreEntity) *ManageCoreEntity {
	result := &ManageCoreEntity{Kind: SearchEntityKind(value.Kind), UUID: value.UUID, Name: value.Name, SortName: value.SortName, Aliases: value.Aliases, Slug: value.Slug,
		MetadataRevision: value.MetadataRevision, ProfileSummary: value.ProfileSummary, Biography: value.Biography, CountryOrRegion: value.CountryOrRegion, UseInRecommendation: value.UseInRecommendation}
	if value.AvatarPath != "" {
		url := coserAssetResourceURL(value.UUID, value.MetadataRevision, "avatar-480")
		result.AvatarURL = &url
	}
	if value.BannerPath != "" {
		url := coserAssetResourceURL(value.UUID, value.MetadataRevision, "banner-1600")
		result.BannerURL = &url
	}
	if value.AvatarCrop != nil {
		result.AvatarCrop = &ManageAvatarCrop{X: value.AvatarCrop.X, Y: value.AvatarCrop.Y, Size: value.AvatarCrop.Size}
	}
	if value.BannerFocalPoint != nil {
		result.BannerFocalPoint = &ManageFocalPoint{X: value.BannerFocalPoint.X, Y: value.BannerFocalPoint.Y}
	}
	if value.WorkUUID != "" {
		result.WorkUUID = &value.WorkUUID
	}
	for _, account := range value.SocialAccounts {
		result.SocialAccounts = append(result.SocialAccounts, &ManageSocialAccount{UUID: account.UUID, PlatformKey: account.PlatformKey, Label: account.Label, Handle: account.Handle, URL: account.URL, Status: account.Status, Visible: account.Visible, Position: strconv.FormatInt(account.Position, 10)})
	}
	for _, parent := range value.Parents {
		result.Parents = append(result.Parents, &ManageCoreEntityRef{UUID: parent.UUID, Name: parent.Name, MetadataRevision: parent.MetadataRevision})
	}
	return result
}

func coserAssetResourceURL(uuid string, revision int64, variant string) string {
	return "/resource/coser/" + uuid + "/" + strconv.FormatInt(revision, 10) + "/" + variant
}

func manageCoreEntityPage(value productdb.ManageCoreEntityPage) *ManageCoreEntityPage {
	result := &ManageCoreEntityPage{Page: value.Page, PageSize: value.PageSize, TotalItems: value.TotalItems, TotalPages: value.TotalPages}
	for _, item := range value.Items {
		result.Items = append(result.Items, manageCoreEntity(item))
	}
	return result
}

func manageCoreEntityMergePreview(value productdb.CoreEntityMergePreview) *ManageCoreEntityMergePreview {
	result := &ManageCoreEntityMergePreview{
		Kind: SearchEntityKind(value.Kind), SourceUUID: value.SourceUUID, TargetUUID: value.TargetUUID,
		SourceRevision: value.SourceRevision, TargetRevision: value.TargetRevision,
		AffectedGalleryIDs: value.AffectedGalleryIDs, CanMerge: len(value.Conflicts) == 0,
	}
	for _, conflict := range value.Conflicts {
		result.Conflicts = append(result.Conflicts, &ManageCoreEntityMergeConflict{Code: conflict.Code, Details: conflict.Details})
	}
	return result
}

func manageCoreEntityDeletePreview(value productdb.CoreEntityDeletePreview) *ManageCoreEntityDeletePreview {
	result := &ManageCoreEntityDeletePreview{
		Kind: SearchEntityKind(value.Kind), UUID: value.UUID, MetadataRevision: value.MetadataRevision,
		ReferenceCount: value.ReferenceCount, CanDelete: value.ReferenceCount == 0,
	}
	for _, blocker := range value.Blockers {
		result.Blockers = append(result.Blockers, &ManageCoreEntityDeleteBlocker{Code: blocker.Code, ReferenceCount: blocker.ReferenceCount})
	}
	return result
}
