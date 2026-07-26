export interface ManageGalleryRow {
  setID: string; slug: string; state: "DRAFT" | "ACTIVE" | "ARCHIVED"; title: string; contentRating?: "NON_ADULT" | "ADULT" | null;
  metadataRevision: number; scanRevision: number; browsable: boolean; sourceType: string; sourcePath: string; sourceAvailability: string;
  reconcileState: string; overLimit: boolean; itemCount: number; missingCount: number; pendingCount: number; errorCount: number; blockingIssues: number;
}
export interface ManageGalleryPage { items: ManageGalleryRow[]; summary: { draft: number; overLimit: number; unavailable: number; blocking: number; missingItem: number }; page: number; pageSize: number; totalItems: number; totalPages: number }
export interface ManageGalleryItem { uuid: string; relativePath: string; mediaKind: string; contentFormat: string; imageCategory?: string | null; position: string; caption: string; excluded: boolean; availability: string; processingState: string; byteSize: number }
export interface ManageGalleryCast { characterUUID: string; characterName: string; workUUID: string; workName: string; position: string }
export interface ManageGalleryCredit { coserUUID: string; coserName: string; position: string; cast: ManageGalleryCast[] }
export interface ManageGalleryTag { uuid: string; name: string; position: string }
export interface ManageGalleryExternalLink { uuid: string; type: "SOURCE" | "PROFILE" | "REFERENCE"; label: string; url: string; position: string }
export interface ManageGalleryManifestState { status: "NONE" | "CLEAN" | "DB_DIRTY" | "FILE_DIRTY" | "CONFLICT" | "MISSING" | "ERROR"; path: string; manifestRevision: number; metadataRevision: number; conflicts: { path: string; baselineJSON: string; databaseJSON: string; fileJSON: string }[] }
export type ManageCoserManifestState = ManageGalleryManifestState;
export interface ManageGalleryDetail {
  row: ManageGalleryRow; aliases: string[]; description: string; shootDate: string; shootDatePrecision: "DAY" | "MONTH" | "UNKNOWN";
  photographerName: string; studioName: string; items: ManageGalleryItem[]; credits: ManageGalleryCredit[]; tags: ManageGalleryTag[]; externalLinks: ManageGalleryExternalLink[];
}
export interface ManageRecognitionRule { id: number; name: string; kind: "MARKER" | "PATH_TEMPLATE" | "FIXED_DEPTH"; enabled: boolean; autoCreateDraft: boolean; order: number; pattern: string; fixedDepth: number }
export interface ManageLibrary { id: number; name: string; rootPath: string; enabled: boolean; readOnly: boolean; captureTimezone: string; rules: ManageRecognitionRule[] }
export interface ManageCandidate { id: number; rootPath: string; sourceType: string; method: string; manifestSetID?: string | null; status: string; autoCreateDraft: boolean; hasConflict: boolean; overLimit: boolean; mediaCount: number; suggestions: { field: string; value: string }[] }
export interface ManageDiscoverySnapshot { id: number; libraryID: number; completedAt: string; candidates: ManageCandidate[]; unassigned: { parentPath: string; mediaCount: number }[] }
export interface ManageRuntimeSettings {
  settingsRevision: number; homeScope: "LIST" | "MAGIC" | "ALL"; galleryCardScrubberEnabled: boolean; galleryDetailMediaFilterEnabled: boolean;
  galleryCardControlsVisible: boolean; mediaCardControlsVisible: boolean; detailPersonalControlsVisible: boolean;
  relatedLimit: number; tagParentWeight: number; tagMinimumScore: number; tagMaximumDepth: number;
  randomLimit: number; randomStaticQuota: number; randomGIFQuota: number; randomVideoQuota: number; randomGalleryRepeatDecay: number;
  enhancedCacheMaximumBytes: number; minimumFreeBytes: number; minimumFreePercent: number; automaticScanEnabled: boolean; automaticSchedulesSuspended: boolean;
  dailyBackupEnabled: boolean; dailyBackupRetention: number;
  archiveMaxEntries: number; archiveMaxEntryBytes: number; archiveMaxTotalBytes: number; archiveMaxCompressionRatio: number; archiveMaxImagePixels: number;
}
export interface ManageProcessingJob { id: number; kind: string; galleryID?: number | null; itemUUID?: string | null; variant: string; status: string; priority: number; attemptCount: number; maxAttempts: number; lastErrorCode: string; structuralFailure: boolean; createdAt: string; updatedAt: string }
export interface ManageProcessingJobPage { items: ManageProcessingJob[]; page: number; pageSize: number; totalItems: number; totalPages: number }
export interface ManageBackupRecord {
  id: string; kind: "DAILY_SNAPSHOT" | "MANUAL_FULL" | "SAFETY_SNAPSHOT"; fileName: string; status: string; byteSize: number; archiveSHA256: string;
  productVersion: string; databaseSchemaVersion: number; manifestSchemaVersion: number; mediaProcessingVersion: number;
  createdAt: string; completedAt?: string | null; lastErrorCode: string;
}
export interface ManageMaintenanceState { state: "NORMAL" | "RESTORING" | "WAITING_VALIDATION"; restoreBackupID?: string | null; lastErrorCode: string; updatedAt: string }
export interface ManageAuditEvent { id: number; eventCode: string; targetKind: string; targetID: string; outcome: string; errorCode: string; summaryJSON: string; createdAt: string }
export interface ManageAuditPage { items: ManageAuditEvent[]; page: number; pageSize: number; totalItems: number; totalPages: number }
export interface ManageCoreEntity { kind: "COSER" | "WORK" | "CHARACTER" | "TAG"; uuid: string; name: string; sortName: string; aliases: string[]; slug: string; metadataRevision: number; workUUID?: string | null; profileSummary: string; biography: string; countryOrRegion: string; useInRecommendation: boolean; socialAccounts: { uuid: string; platformKey: string; label: string; handle: string; url: string; status: string; visible: boolean; position: string }[]; parents: { uuid: string; name: string; metadataRevision: number }[] }
export interface ManageCoreEntityPage { items: ManageCoreEntity[]; page: number; pageSize: number; totalItems: number; totalPages: number }
export interface ManageCoreEntityMergePreview {
  kind: ManageCoreEntity["kind"]; sourceUUID: string; targetUUID: string; sourceRevision: number; targetRevision: number;
  affectedGalleryIDs: number[]; conflicts: { code: string; details: string }[]; canMerge: boolean;
}
export interface ManageCoreEntityMergeResult { target: ManageCoreEntity; preview: ManageCoreEntityMergePreview; completionWarning?: string | null }
export interface ManageCoreEntityDeletePreview {
  kind: ManageCoreEntity["kind"]; uuid: string; metadataRevision: number; referenceCount: number;
  blockers: { code: string; referenceCount: number }[]; canDelete: boolean;
}
