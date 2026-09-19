export interface ManageGalleryRow {
  setID: string; slug: string; state: "DRAFT" | "ACTIVE" | "ARCHIVED"; title: string; contentRating?: "NON_ADULT" | "ADULT" | null;
  metadataRevision: number; scanRevision: number; browsable: boolean; sourceType: string; sourcePath: string; sourceAvailability: string;
  reconcileState: string; overLimit: boolean; itemCount: number; missingCount: number; pendingCount: number; errorCount: number; blockingIssues: number; lastScanErrorCode: string; lastScanCompleted: string; manifestStatus: string; manifestCheckedAt: string; captureDateReviewStatus: string;
}
export interface ManageGalleryPage { items: ManageGalleryRow[]; summary: { all: number; draft: number; overLimit: number; unavailable: number; blocking: number; processingError: number; missingGallery: number; manifestAttention: number; captureDateAttention: number }; page: number; pageSize: number; totalItems: number; totalPages: number }
export interface ManageGalleryManifestBatchPreview { setID: string; title: string; status: string; metadataRevision: number; path: string; fileHash: string; databaseContentChanged: boolean; localFileChanged: boolean; blockReason: string }
export interface ManageGalleryManifestBatchResult { setID: string; outcome: string; reason: string }
export interface ManageGalleryItem { uuid: string; relativePath: string; mediaKind: string; contentFormat: string; imageCategory?: string | null; position: string; caption: string; excluded: boolean; availability: string; processingState: string; byteSize: number; videoProbeState: string; videoErrorCode: string; videoContainer: string; videoDurationSeconds: number; videoWidth: number; videoHeight: number; videoCodec: string; audioCodec: string }
export interface ManageGalleryCast { characterUUID: string; characterName: string; workUUID: string; workName: string; position: string }
export interface ManageGalleryCredit { coserUUID: string; coserName: string; position: string; cast: ManageGalleryCast[] }
export interface ManageGalleryTag { uuid: string; name: string; position: string }
export interface ManageGalleryExternalLink { uuid: string; type: "SOURCE" | "PROFILE" | "REFERENCE"; label: string; url: string; position: string }
export interface ManageGalleryFolderMatch { kind: "COSER" | "WORK" | "CHARACTER"; uuid: string; name: string; matchedName: string; workUUID: string; workName: string }
export interface ManageGalleryManifestState { status: "NONE" | "CLEAN" | "DB_DIRTY" | "FILE_DIRTY" | "CONFLICT" | "MISSING" | "ERROR"; path: string; manifestRevision: number; metadataRevision: number; pushAdded: number; pushRemoved: number; pushRetained: number; pushUpdated: number; conflicts: { path: string; baselineJSON: string; databaseJSON: string; fileJSON: string }[] }
export type ManageCoserManifestState = ManageGalleryManifestState;
export interface ManageGalleryDetail {
  row: ManageGalleryRow; aliases: string[]; description: string; shootDate: string; shootDatePrecision: "DAY" | "MONTH" | "UNKNOWN";
  imageCaptureStart: string; imageCaptureEnd: string; videoCaptureStart: string; videoCaptureEnd: string;
  captureDateCandidate: string; captureDateReviewStatus: string;
  photographerName: string; studioName: string; items: ManageGalleryItem[]; credits: ManageGalleryCredit[]; tags: ManageGalleryTag[]; externalLinks: ManageGalleryExternalLink[];
  folderMatches: ManageGalleryFolderMatch[];
  scanRuns: { id: string; status: string; startedAt: string; completedAt: string; errorCode: string }[];
}
export interface ManageGalleryDeletePreview {
  setID: string; state: "DRAFT" | "ACTIVE" | "ARCHIVED"; metadataRevision: number; itemCount: number;
  externalLinkCount: number; executableJobCount: number; ignoredSourceWillBeCreated: boolean; canDelete: boolean;
}
export interface ManageRecognitionRule { id: number; name: string; kind: "MARKER" | "PATH_TEMPLATE" | "FIXED_DEPTH"; enabled: boolean; autoCreateDraft: boolean; order: number; pattern: string; fixedDepth: number }
export interface ManageLibrary { id: number; name: string; rootPath: string; enabled: boolean; metadataWritebackEnabled: boolean; boundGalleryCount: number; captureTimezone: string; updatedAt: string; rules: ManageRecognitionRule[] }
export interface ManageLibraryChangePreview { libraryID: number; currentRoot: string; proposedRoot: string; revisionToken: string; ignoredSourceCount: number; ignoredSources: { id: number; path: string; reason: string }[]; unassignedSourcePaths: string[]; recognitionRules: { id: number; name: string }[]; classificationRules: { id: number; name: string }[]; exclusionRules: { id: number; name: string }[]; automationMode: string; automationPolicyRevision: number; automationRunCount: number; activeRunCount: number; portableMappingCount: number; scanningSourceCount: number; childRoots: string[]; proposedBoundaryConflicts: string[]; impacts: { sourceID: number; galleryID: number; galleryTitle: string; sourcePath: string; currentLibraryID: number; suggestedOwnerID: number | null }[] }
export interface ManageIgnoredSourceRecord { id: number; libraryID: number | null; setID: string | null; path: string; reason: string; createdAt: string }
export interface ManageIgnoredSourcePage { page: number; pageSize: number; total: number; items: ManageIgnoredSourceRecord[] }
export interface ManageIgnoredSourceRemovalPreview { record: ManageIgnoredSourceRecord; affectedLibraryIDs: number[]; activeRunCount: number; boundSourceCount: number; revisionToken: string }
export interface ManageMediaClassificationRule { id: number; libraryID?: number | null; name: string; enabled: boolean; order: number; subject: "PARENT_FOLDER" | "FILE_NAME" | "FILE_STEM" | "RELATIVE_PATH"; operator: "EXACT" | "GLOB" | "RE2"; pattern: string; caseSensitive: boolean; resultCategory: "PHOTO" | "SELFIE"; revision: number; systemDefault: boolean }
export interface ManageMediaClassificationSuggestion { id: number; galleryID: number; galleryRevision: number; gallerySetID: string; galleryTitle: string; itemUUID: string; relativePath: string; ruleID: number; ruleRevision: number; ruleName: string; proposedCategory: "PHOTO" | "SELFIE"; matchedSubject: string; matchedValue: string; status: string }
export interface ManageMediaClassificationPreview { totalMatches: number; samples: { gallerySetID: string; galleryTitle: string; itemUUID: string; relativePath: string; currentCategory: "PHOTO" | "SELFIE"; proposedCategory: "PHOTO" | "SELFIE"; matchedValue: string }[] }
export interface ManageMediaExclusionRule { id: number; libraryID?: number | null; name: string; enabled: boolean; order: number; subject: "PARENT_FOLDER" | "PARENT_PATH" | "FILE_NAME" | "FILE_STEM" | "RELATIVE_PATH"; operator: "EXACT" | "GLOB" | "RE2"; pattern: string; caseSensitive: boolean; mediaKind: "ALL" | "STATIC_IMAGE" | "ANIMATED_IMAGE" | "VIDEO"; decision: "EXCLUDE" | "INCLUDE"; revision: number; systemDefault: boolean }
export interface ManageMediaExclusionDecision { id: number; galleryID: number; galleryRevision: number; gallerySetID: string; galleryTitle: string; itemUUID: string; relativePath: string; ruleID: number; ruleRevision: number; ruleName: string; decision: "EXCLUDE" | "INCLUDE"; matchedSubject: string; matchedValue: string; status: string }
export interface ManageMediaExclusionPreview { totalMatches: number; samples: { gallerySetID: string; galleryTitle: string; itemUUID: string; relativePath: string; mediaKind: string; currentlyExcluded: boolean; proposedDecision: "EXCLUDE" | "INCLUDE"; winningRuleName: string; matchedValue: string }[] }
export interface ManageCandidate { id: number; rootPath: string; sourceType: string; method: string; manifestSetID?: string | null; status: string; autoCreateDraft: boolean; hasConflict: boolean; overLimit: boolean; mediaCount: number; suggestions: { field: string; value: string }[] }
export interface ManageDiscoverySnapshot {
  id: number; libraryID: number; completedAt: string; candidates: ManageCandidate[]; unassigned: { parentPath: string; mediaCount: number }[];
  coverageSummary: { regularFileCount: number; supportedMediaCount: number; supportedArchiveCount: number; unsupportedArchiveCount: number; controlFileCount: number; ignoredOtherCount: number; actionableIssueCount: number; registeredSourceCount: number; indexedItemCount: number; sourceNeedsScanCount: number };
  coverageDiagnostics: { path: string; entryKind: "DIRECTORY" | "ARCHIVE"; reasonCode: string; fileCount: number; byteSize: number }[];
}
export interface ManageLibraryAutomationPolicy { libraryID: number; mode: "MANUAL" | "ASSISTED" | "TRUSTED"; defaultContentRating?: "NON_ADULT" | "ADULT" | null; excludeNewRootMedia: boolean; autoImportArchives: boolean; autoAcceptUniqueEntities: boolean; autoAcceptMediaClassification: boolean; autoActivate: boolean; revision: number }
export interface ManageLibraryAutomationRun { id: number; libraryID: number; policyRevision: number; mode: string; status: string; phase: string; processedTargets: number; totalTargets: number; currentGalleryTitle: string; cancellationRequested: boolean; candidatesSeen: number; draftsCreated: number; scanned: number; activated: number; needsReview: number; issueCount: number; errorCode: string; startedAt: string; completedAt?: string | null }
export interface ManageLibraryAutomation { policy: ManageLibraryAutomationPolicy; preview: { candidateCount: number; autoCreateEligible: number; draftCount: number; activationReady: number; needsReview: number }; recentRuns: ManageLibraryAutomationRun[] }
export interface ManageRuntimeSettings {
  settingsRevision: number; homeScope: "LIST" | "MAGIC" | "ALL"; galleryCardScrubberEnabled: boolean; galleryDetailMediaFilterEnabled: boolean;
  galleryCardControlsVisible: boolean; mediaCardControlsVisible: boolean; detailPersonalControlsVisible: boolean;
  galleryAnimatedPlaybackLimit: number; galleryAnimatedLockIntervalMS: number;
  relatedLimit: number; tagParentWeight: number; tagMinimumScore: number; tagMaximumDepth: number;
  randomLimit: number; randomStaticQuota: number; randomGIFQuota: number; randomVideoQuota: number; randomGalleryRepeatDecay: number;
  enhancedCacheMaximumBytes: number; minimumFreeBytes: number; minimumFreePercent: number; automaticScanEnabled: boolean; automaticScanOnStartup: boolean; automaticScanIntervalMinutes: number; automaticSchedulesSuspended: boolean;
  dailyBackupEnabled: boolean; dailyBackupRetention: number;
  archiveMaxEntries: number; archiveMaxEntryBytes: number; archiveMaxTotalBytes: number; archiveMaxCompressionRatio: number; archiveMaxImagePixels: number;
}

export interface ManageVideoDependencyStatus {
  ffmpegAvailable: boolean;
  ffmpegSource: string;
  ffmpegVersion: string;
  ffmpegErrorCode: string;
  ffprobeAvailable: boolean;
  ffprobeSource: string;
  ffprobeVersion: string;
  ffprobeErrorCode: string;
}
export interface ManageCacheStorage { path: string; byteSize: number; fileCount: number; baseByteSize: number; enhancedByteSize: number }
export interface ManageProcessingJob { id: number; kind: string; galleryID?: number | null; itemUUID?: string | null; variant: string; status: string; priority: number; attemptCount: number; maxAttempts: number; lastErrorCode: string; structuralFailure: boolean; createdAt: string; updatedAt: string }
export interface ManageProcessingJobPage { items: ManageProcessingJob[]; page: number; pageSize: number; totalItems: number; totalPages: number }
export interface ManageBackupRecord {
  id: string; kind: "DAILY_SNAPSHOT" | "MANUAL_FULL" | "SAFETY_SNAPSHOT"; fileName: string; status: string; byteSize: number; archiveSHA256: string;
  productVersion: string; databaseSchemaVersion: number; manifestSchemaVersion: number; mediaProcessingVersion: number;
  createdAt: string; completedAt?: string | null; lastErrorCode: string;
}
export interface ManageMaintenanceState { state: "NORMAL" | "RESTORING" | "WAITING_VALIDATION" | "PORTABLE_IMPORTING" | "PORTABLE_MERGING"; restoreBackupID?: string | null; lastErrorCode: string; updatedAt: string }
export interface ManageAuditEvent { id: number; eventCode: string; targetKind: string; targetID: string; outcome: string; errorCode: string; summaryJSON: string; createdAt: string }
export interface ManageAuditPage { items: ManageAuditEvent[]; page: number; pageSize: number; totalItems: number; totalPages: number }
export interface ManagePortableImportSession { importID: string; exportID: string; state: string; formatVersion: number; identityCount: number; coreEntityCount: number; galleryClaimCount: number; itemClaimCount: number; linkClaimCount: number; assetCount: number; errorCode: string; createdAt: string; updatedAt: string }
export interface ManagePortableMergeSession { mergeID: string; exportID: string; state: string; hardBlockingCount: number; reviewCount: number; identityAddCount: number; identityReuseCount: number; entityAddCount: number; entityReuseCount: number; errorCode: string; safetyBackupID?: string | null; createdAt: string; updatedAt: string }
export interface ManagePortableMergeConflict { issueKey: string; issueCode: string; severity: string; entityKind: string; incomingUUID: string; localUUID?: string | null; fieldKey: string; decision: string }
export interface ManagePortableLibraryMapping { libraryKey: string; libraryName: string; decision: string; targetLibraryID?: number | null; targetName: string; targetRoot: string }
export interface ManagePortableGalleryRebuild { setID: string; libraryKey: string; sourceType: string; relativeSource: string; locatorStatus: string; manifestStatus: string; state: string; issueCode: string }
export interface ManagePortableOwnerContinuity { available: boolean; galleryLifecycle: boolean; personalFlags: boolean; galleryCount: number; itemCount: number }
export interface ManagePortablePreflight { identityCount: number; coreEntityCount: number; galleryCount: number; incompleteGalleryCount: number; assetCount: number; warningCount: number; blockingCount: number; issues: { code: string; severity: string; count: number }[] }
export interface ManagePortableMigrationSnapshot { imports: ManagePortableImportSession[]; merges: ManagePortableMergeSession[]; conflicts: ManagePortableMergeConflict[]; mappings: ManagePortableLibraryMapping[]; rebuilds: ManagePortableGalleryRebuild[]; owner?: ManagePortableOwnerContinuity | null }
export interface ManageCoserAssetReviewGroup {
  id: string; coser_uuid: string; kind: "AVATAR" | "BANNER"; reason: "REPLACED" | "MERGED_COSER" | "DELETED_COSER";
  file_count: number; byte_size: number; modified_at: string;
}
export interface ManageCoserAssetReview {
  groups: ManageCoserAssetReviewGroup[]; total_file_count: number; total_byte_size: number; ignored_entry_count: number;
}
export interface ManageCoserAssetCleanupResult {
  deleted_group_count: number; deleted_file_count: number; deleted_byte_size: number; review: ManageCoserAssetReview;
}
export interface ManageCoreEntity { kind: "COSER" | "WORK" | "CHARACTER" | "TAG"; uuid: string; name: string; sortName: string; aliases: string[]; slug: string; metadataRevision: number; workUUID?: string | null; workName?: string; profileSummary: string; biography: string; countryOrRegion: string; useInRecommendation: boolean; allowDirectAssignment?: boolean; avatarURL?: string | null; bannerURL?: string | null; avatarCrop?: { x: number; y: number; size: number } | null; bannerFocalPoint?: { x: number; y: number } | null; socialAccounts: { uuid: string; platformKey: string; label: string; handle: string; url: string; status: string; visible: boolean; position: string }[]; parents: { uuid: string; name: string; metadataRevision: number }[] }
export interface ManageCoserNameConflict { coser: ManageCoreEntity; matchedValues: string[]; galleryCount: number }
export interface ManageCoreEntityNameConflict { entity: ManageCoreEntity; matchedValues: string[]; galleryCount: number; workName: string; primaryNameMatch: boolean }
export interface ManageCoreEntityPage { items: ManageCoreEntity[]; page: number; pageSize: number; totalItems: number; totalPages: number }
export interface ManageTagTreeItem { uuid: string; name: string; aliases: string[]; parentUUIDs: string[]; metadataRevision: number; childCount: number; galleryCount: number; allowDirectAssignment?: boolean }
export interface ManageCoreEntityMergePreview {
  kind: ManageCoreEntity["kind"]; sourceUUID: string; targetUUID: string; sourceRevision: number; targetRevision: number;
  affectedGalleryIDs: number[]; conflicts: { code: string; details: string }[]; canMerge: boolean;
}
export interface ManageCoreEntityMergeResult { target: ManageCoreEntity; preview: ManageCoreEntityMergePreview; completionWarning?: string | null }
export interface ManageCoreEntityDeletePreview {
  kind: ManageCoreEntity["kind"]; uuid: string; metadataRevision: number; referenceCount: number;
  blockers: { code: string; referenceCount: number }[]; canDelete: boolean;
}
