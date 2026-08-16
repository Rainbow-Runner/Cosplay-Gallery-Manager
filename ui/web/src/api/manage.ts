import { gql } from "@apollo/client";

const MANAGE_GALLERY_DETAIL = gql`
  fragment ManageGalleryDetailFields on ManageGalleryDetail {
    row { setID slug state title contentRating metadataRevision scanRevision browsable sourceType sourcePath sourceAvailability reconcileState overLimit itemCount missingCount pendingCount errorCount blockingIssues }
    aliases description shootDate shootDatePrecision photographerName studioName
    items { uuid relativePath mediaKind contentFormat imageCategory position caption excluded availability processingState byteSize videoProbeState videoErrorCode videoContainer videoDurationSeconds videoWidth videoHeight videoCodec audioCodec }
    credits { coserUUID coserName position cast { characterUUID characterName workUUID workName position } }
    tags { uuid name position }
    externalLinks { uuid type label url position }
    folderMatches { kind uuid name matchedName workUUID workName }
  }
`;

export const MANAGE_GALLERIES = gql`
  query ManageGalleries($page: Int!) { manageGalleries(page: $page) {
    page pageSize totalItems totalPages summary { draft overLimit unavailable blocking missingItem }
    items { setID slug state title contentRating metadataRevision scanRevision browsable sourceType sourcePath sourceAvailability reconcileState overLimit itemCount missingCount pendingCount errorCount blockingIssues }
  } }
`;
export const MANAGE_GALLERY = gql`
  ${MANAGE_GALLERY_DETAIL}
  query ManageGallery($setID: ID!) { manageGallery(setID: $setID) {
    ...ManageGalleryDetailFields
  } }
`;
export const UPDATE_GALLERY_METADATA = gql`
  ${MANAGE_GALLERY_DETAIL}
  mutation UpdateGalleryMetadata($setID: ID!, $expectedMetadataRevision: Int64!, $input: UpdateGalleryMetadataInput!) {
    updateGalleryMetadata(setID: $setID, expectedMetadataRevision: $expectedMetadataRevision, input: $input) {
      ...ManageGalleryDetailFields
    }
  }
`;
export const SET_GALLERY_STATE = gql`
  ${MANAGE_GALLERY_DETAIL}
  mutation SetGalleryState($setID: ID!, $expectedMetadataRevision: Int64!, $state: GalleryState!) {
    setGalleryState(setID: $setID, expectedMetadataRevision: $expectedMetadataRevision, state: $state) {
      ...ManageGalleryDetailFields
    }
  }
`;
export const UPDATE_GALLERY_ITEM = gql`
  ${MANAGE_GALLERY_DETAIL}
  mutation UpdateGalleryItem($setID: ID!, $itemUUID: ID!, $expectedMetadataRevision: Int64!, $input: UpdateGalleryItemInput!) {
    updateGalleryItem(setID: $setID, itemUUID: $itemUUID, expectedMetadataRevision: $expectedMetadataRevision, input: $input) { ...ManageGalleryDetailFields }
  }
`;
export const SET_GALLERY_ITEM_EXCLUDED = gql`
  ${MANAGE_GALLERY_DETAIL}
  mutation SetGalleryItemExcluded($setID: ID!, $itemUUID: ID!, $excluded: Boolean!, $expectedMetadataRevision: Int64!) {
    setGalleryItemExcluded(setID: $setID, itemUUID: $itemUUID, excluded: $excluded, expectedMetadataRevision: $expectedMetadataRevision) { ...ManageGalleryDetailFields }
  }
`;
export const MOVE_GALLERY_ITEM = gql`
  ${MANAGE_GALLERY_DETAIL}
  mutation MoveGalleryItem($setID: ID!, $itemUUID: ID!, $beforeItemUUID: ID, $expectedMetadataRevision: Int64!) {
    moveGalleryItem(setID: $setID, itemUUID: $itemUUID, beforeItemUUID: $beforeItemUUID, expectedMetadataRevision: $expectedMetadataRevision) { ...ManageGalleryDetailFields }
  }
`;
export const REORDER_GALLERY_ITEMS = gql`
  ${MANAGE_GALLERY_DETAIL}
  mutation ReorderGalleryItems($setID: ID!, $itemUUIDs: [ID!]!, $expectedMetadataRevision: Int64!) {
    reorderGalleryItems(setID: $setID, itemUUIDs: $itemUUIDs, expectedMetadataRevision: $expectedMetadataRevision) { ...ManageGalleryDetailFields }
  }
`;
export const SET_GALLERY_COVER_ITEM = gql`
  ${MANAGE_GALLERY_DETAIL}
  mutation SetGalleryCoverItem($setID: ID!, $itemUUID: ID!, $expectedMetadataRevision: Int64!) {
    setGalleryCoverItem(setID: $setID, itemUUID: $itemUUID, expectedMetadataRevision: $expectedMetadataRevision) { ...ManageGalleryDetailFields }
  }
`;
export const RESET_GALLERY_COVER = gql`
  ${MANAGE_GALLERY_DETAIL}
  mutation ResetGalleryCover($setID: ID!, $expectedMetadataRevision: Int64!) {
    resetGalleryCover(setID: $setID, expectedMetadataRevision: $expectedMetadataRevision) { ...ManageGalleryDetailFields }
  }
`;
export const MANAGE_LIBRARIES = gql`
  query ManageLibraries { manageLibraries { id name rootPath enabled readOnly captureTimezone rules { id name kind enabled autoCreateDraft order pattern fixedDepth } } }
`;
export const MANAGE_DISCOVERY = gql`
  query ManageDiscovery($libraryID: Int64!) { manageDiscovery(libraryID: $libraryID) { id libraryID completedAt candidates { id rootPath sourceType method manifestSetID status autoCreateDraft hasConflict overLimit mediaCount suggestions { field value } } unassigned { parentPath mediaCount } } }
`;
export const CREATE_MEDIA_LIBRARY = gql`
  mutation CreateMediaLibrary($input: CreateMediaLibraryInput!) { createMediaLibrary(input: $input) { id name rootPath enabled readOnly captureTimezone rules { id } } }
`;
export const CREATE_RECOGNITION_RULE = gql`
  mutation CreateRecognitionRule($input: CreateRecognitionRuleInput!) { createRecognitionRule(input: $input) { id name kind enabled autoCreateDraft order pattern fixedDepth } }
`;
export const UPDATE_RECOGNITION_RULE = gql`
  mutation UpdateRecognitionRule($input: UpdateRecognitionRuleInput!) { updateRecognitionRule(input: $input) { id name kind enabled autoCreateDraft order pattern fixedDepth } }
`;
export const DELETE_RECOGNITION_RULE = gql`
  mutation DeleteRecognitionRule($id: Int64!) { deleteRecognitionRule(id: $id) }
`;
export const DISCOVER_MEDIA_LIBRARY = gql`
  mutation DiscoverMediaLibrary($libraryID: Int64!) { discoverMediaLibrary(libraryID: $libraryID) { id libraryID completedAt candidates { id rootPath sourceType method manifestSetID status autoCreateDraft hasConflict overLimit mediaCount suggestions { field value } } unassigned { parentPath mediaCount } } }
`;
export const IMPORT_GALLERY_CANDIDATE = gql`
  mutation ImportGalleryCandidate($candidateID: Int64!) { importGalleryCandidate(candidateID: $candidateID) { row { setID } } }
`;
export const SCAN_GALLERY_SOURCE = gql`
  ${MANAGE_GALLERY_DETAIL}
  mutation ScanGallerySource($setID: ID!, $excludeNewRootMedia: Boolean! = true) {
    scanGallerySource(setID: $setID, excludeNewRootMedia: $excludeNewRootMedia) { ...ManageGalleryDetailFields }
  }
`;
export const REPLACE_GALLERY_RELATIONS = gql`
  ${MANAGE_GALLERY_DETAIL}
  mutation ReplaceGalleryRelations($setID: ID!, $expectedMetadataRevision: Int64!, $input: ReplaceGalleryRelationsInput!) {
    replaceGalleryRelations(setID: $setID, expectedMetadataRevision: $expectedMetadataRevision, input: $input) { ...ManageGalleryDetailFields }
  }
`;
export const ADD_GALLERY_EXTERNAL_LINK = gql`
  ${MANAGE_GALLERY_DETAIL}
  mutation AddGalleryExternalLink($setID: ID!, $expectedMetadataRevision: Int64!, $input: GalleryExternalLinkInput!) {
    addGalleryExternalLink(setID: $setID, expectedMetadataRevision: $expectedMetadataRevision, input: $input) { ...ManageGalleryDetailFields }
  }
`;
const MANAGE_GALLERY_MANIFEST_FIELDS = gql`fragment ManageGalleryManifestFields on ManageGalleryManifestState { status path manifestRevision metadataRevision conflicts { path baselineJSON databaseJSON fileJSON } }`;
export const MANAGE_GALLERY_MANIFEST = gql`${MANAGE_GALLERY_MANIFEST_FIELDS} query ManageGalleryManifest($setID: ID!) { manageGalleryManifest(setID: $setID) { ...ManageGalleryManifestFields } }`;
export const PUSH_GALLERY_MANIFEST = gql`${MANAGE_GALLERY_MANIFEST_FIELDS} mutation PushGalleryManifest($setID: ID!, $expectedMetadataRevision: Int64!) { pushGalleryManifest(setID: $setID, expectedMetadataRevision: $expectedMetadataRevision) { ...ManageGalleryManifestFields } }`;
export const PULL_GALLERY_MANIFEST = gql`${MANAGE_GALLERY_MANIFEST_FIELDS} mutation PullGalleryManifest($setID: ID!, $expectedMetadataRevision: Int64!) { pullGalleryManifest(setID: $setID, expectedMetadataRevision: $expectedMetadataRevision) { ...ManageGalleryManifestFields } }`;
export const RESOLVE_GALLERY_MANIFEST = gql`${MANAGE_GALLERY_MANIFEST_FIELDS} mutation ResolveGalleryManifest($setID: ID!, $expectedMetadataRevision: Int64!, $choices: [ManifestConflictChoiceInput!]!) { resolveGalleryManifest(setID: $setID, expectedMetadataRevision: $expectedMetadataRevision, choices: $choices) { ...ManageGalleryManifestFields } }`;
const MANAGE_COSER_MANIFEST_FIELDS = gql`fragment ManageCoserManifestFields on ManageCoserManifestState { status path manifestRevision metadataRevision conflicts { path baselineJSON databaseJSON fileJSON } }`;
export const MANAGE_COSER_MANIFEST = gql`${MANAGE_COSER_MANIFEST_FIELDS} query ManageCoserManifest($coserUUID: ID!) { manageCoserManifest(coserUUID: $coserUUID) { ...ManageCoserManifestFields } }`;
export const PUSH_COSER_MANIFEST = gql`${MANAGE_COSER_MANIFEST_FIELDS} mutation PushCoserManifest($coserUUID: ID!, $expectedMetadataRevision: Int64!) { pushCoserManifest(coserUUID: $coserUUID, expectedMetadataRevision: $expectedMetadataRevision) { ...ManageCoserManifestFields } }`;
export const PULL_COSER_MANIFEST = gql`${MANAGE_COSER_MANIFEST_FIELDS} mutation PullCoserManifest($coserUUID: ID!, $expectedMetadataRevision: Int64!) { pullCoserManifest(coserUUID: $coserUUID, expectedMetadataRevision: $expectedMetadataRevision) { ...ManageCoserManifestFields } }`;
export const RESOLVE_COSER_MANIFEST = gql`${MANAGE_COSER_MANIFEST_FIELDS} mutation ResolveCoserManifest($coserUUID: ID!, $expectedMetadataRevision: Int64!, $choices: [ManifestConflictChoiceInput!]!) { resolveCoserManifest(coserUUID: $coserUUID, expectedMetadataRevision: $expectedMetadataRevision, choices: $choices) { ...ManageCoserManifestFields } }`;
const RUNTIME_SETTINGS_FIELDS = gql`fragment RuntimeSettingsFields on ManageRuntimeSettings { settingsRevision homeScope galleryCardScrubberEnabled galleryDetailMediaFilterEnabled galleryCardControlsVisible mediaCardControlsVisible detailPersonalControlsVisible galleryAnimatedPlaybackLimit galleryAnimatedLockIntervalMS relatedLimit tagParentWeight tagMinimumScore tagMaximumDepth randomLimit randomStaticQuota randomGIFQuota randomVideoQuota randomGalleryRepeatDecay enhancedCacheMaximumBytes minimumFreeBytes minimumFreePercent automaticScanEnabled automaticSchedulesSuspended dailyBackupEnabled dailyBackupRetention archiveMaxEntries archiveMaxEntryBytes archiveMaxTotalBytes archiveMaxCompressionRatio archiveMaxImagePixels }`;
export const MANAGE_RUNTIME_SETTINGS = gql`${RUNTIME_SETTINGS_FIELDS} query ManageRuntimeSettings { manageRuntimeSettings { ...RuntimeSettingsFields } manageCacheStorage { path byteSize fileCount baseByteSize enhancedByteSize } manageVideoDependencyStatus { ffmpegAvailable ffmpegSource ffmpegVersion ffmpegErrorCode ffprobeAvailable ffprobeSource ffprobeVersion ffprobeErrorCode } }`;
export const UPDATE_RUNTIME_SETTINGS = gql`${RUNTIME_SETTINGS_FIELDS} mutation UpdateRuntimeSettings($expectedSettingsRevision: Int64!, $input: RuntimeSettingsInput!) { updateRuntimeSettings(expectedSettingsRevision: $expectedSettingsRevision, input: $input) { ...RuntimeSettingsFields } }`;
const PROCESSING_JOB_PAGE_FIELDS = gql`fragment ProcessingJobPageFields on ManageProcessingJobPage { page pageSize totalItems totalPages items { id kind galleryID itemUUID variant status priority attemptCount maxAttempts lastErrorCode structuralFailure createdAt updatedAt } }`;
export const MANAGE_PROCESSING_JOBS = gql`${PROCESSING_JOB_PAGE_FIELDS} query ManageProcessingJobs($status: String!, $page: Int!) { manageProcessingJobs(status: $status, page: $page) { ...ProcessingJobPageFields } }`;
export const CANCEL_PROCESSING_JOB = gql`${PROCESSING_JOB_PAGE_FIELDS} mutation CancelProcessingJob($id: Int64!) { cancelProcessingJob(id: $id) { ...ProcessingJobPageFields } }`;
export const RETRY_PROCESSING_JOB = gql`${PROCESSING_JOB_PAGE_FIELDS} mutation RetryProcessingJob($id: Int64!) { retryProcessingJob(id: $id) { ...ProcessingJobPageFields } }`;
export const RETRY_GALLERY_ITEM_VIDEO = gql`mutation RetryGalleryItemVideo($itemUUID: ID!) { retryGalleryItemVideo(itemUUID: $itemUUID) }`;
const BACKUP_FIELDS = gql`fragment BackupFields on ManageBackupRecord { id kind fileName status byteSize archiveSHA256 productVersion databaseSchemaVersion manifestSchemaVersion mediaProcessingVersion createdAt completedAt lastErrorCode }`;
export const MANAGE_OPERATIONS = gql`${BACKUP_FIELDS} query ManageOperations($page: Int!) {
  manageBackups { ...BackupFields }
  manageMaintenance { state restoreBackupID lastErrorCode updatedAt }
  manageAudit(page: $page) { page pageSize totalItems totalPages items { id eventCode targetKind targetID outcome errorCode summaryJSON createdAt } }
}`;
export const CREATE_FULL_BACKUP = gql`${BACKUP_FIELDS} mutation CreateFullBackup { createFullBackup { ...BackupFields } }`;
export const RESTORE_BACKUP = gql`mutation RestoreBackup($backupID: ID!) { restoreBackup(backupID: $backupID) { state restoreBackupID lastErrorCode updatedAt } }`;
const MANAGE_CORE_ENTITY_FIELDS = gql`fragment ManageCoreEntityFields on ManageCoreEntity { kind uuid name sortName aliases slug metadataRevision workUUID profileSummary biography countryOrRegion useInRecommendation avatarURL bannerURL avatarCrop { x y size } bannerFocalPoint { x y } socialAccounts { uuid platformKey label handle url status visible position } parents { uuid name metadataRevision } }`;
export const MANAGE_CORE_ENTITIES = gql`${MANAGE_CORE_ENTITY_FIELDS} query ManageCoreEntities($kind: SearchEntityKind!, $page: Int!) { manageCoreEntities(kind: $kind, page: $page) { page pageSize totalItems totalPages items { ...ManageCoreEntityFields } } }`;
export const MANAGE_CORE_ENTITY = gql`${MANAGE_CORE_ENTITY_FIELDS} query ManageCoreEntity($kind: SearchEntityKind!, $uuid: ID!) { manageCoreEntity(kind: $kind, uuid: $uuid) { ...ManageCoreEntityFields } }`;
export const MANAGE_CORE_ENTITY_OPTIONS = gql`query ManageCoreEntityOptions($kind: SearchEntityKind!, $query: String!, $limit: Int!) { manageCoreEntityOptions(kind: $kind, query: $query, limit: $limit) { kind uuid name aliases workUUID metadataRevision } }`;
export const CREATE_CORE_ENTITY = gql`${MANAGE_CORE_ENTITY_FIELDS} mutation CreateCoreEntity($input: CoreEntityInput!) { createCoreEntity(input: $input) { ...ManageCoreEntityFields } }`;
export const UPDATE_CORE_ENTITY = gql`${MANAGE_CORE_ENTITY_FIELDS} mutation UpdateCoreEntity($uuid: ID!, $expectedMetadataRevision: Int64!, $input: CoreEntityInput!) { updateCoreEntity(uuid: $uuid, expectedMetadataRevision: $expectedMetadataRevision, input: $input) { ...ManageCoreEntityFields } }`;
export const ADD_COSER_SOCIAL_ACCOUNT = gql`${MANAGE_CORE_ENTITY_FIELDS} mutation AddCoserSocialAccount($coserUUID: ID!, $expectedMetadataRevision: Int64!, $input: SocialAccountInput!) { addCoserSocialAccount(coserUUID: $coserUUID, expectedMetadataRevision: $expectedMetadataRevision, input: $input) { ...ManageCoreEntityFields } }`;
export const REPLACE_TAG_PARENTS = gql`${MANAGE_CORE_ENTITY_FIELDS} mutation ReplaceTagParents($childUUID: ID!, $expectedChildRevision: Int64!, $parents: [ReplaceTagParentInput!]!, $expectedParents: [ExpectedTagRevisionInput!]!) { replaceTagParents(childUUID: $childUUID, expectedChildRevision: $expectedChildRevision, parents: $parents, expectedParents: $expectedParents) { ...ManageCoreEntityFields } }`;
const CORE_ENTITY_MERGE_PREVIEW_FIELDS = gql`fragment CoreEntityMergePreviewFields on ManageCoreEntityMergePreview { kind sourceUUID targetUUID sourceRevision targetRevision affectedGalleryIDs conflicts { code details } canMerge }`;
export const PREVIEW_CORE_ENTITY_MERGE = gql`${CORE_ENTITY_MERGE_PREVIEW_FIELDS} query PreviewCoreEntityMerge($kind: SearchEntityKind!, $sourceUUID: ID!, $targetUUID: ID!) { previewCoreEntityMerge(kind: $kind, sourceUUID: $sourceUUID, targetUUID: $targetUUID) { ...CoreEntityMergePreviewFields } }`;
export const MERGE_CORE_ENTITIES = gql`${MANAGE_CORE_ENTITY_FIELDS} ${CORE_ENTITY_MERGE_PREVIEW_FIELDS} mutation MergeCoreEntities($kind: SearchEntityKind!, $sourceUUID: ID!, $targetUUID: ID!, $expectedSourceRevision: Int64!, $expectedTargetRevision: Int64!) { mergeCoreEntities(kind: $kind, sourceUUID: $sourceUUID, targetUUID: $targetUUID, expectedSourceRevision: $expectedSourceRevision, expectedTargetRevision: $expectedTargetRevision) { target { ...ManageCoreEntityFields } preview { ...CoreEntityMergePreviewFields } completionWarning } }`;
export const PREVIEW_CORE_ENTITY_DELETE = gql`query PreviewCoreEntityDelete($kind: SearchEntityKind!, $uuid: ID!) { previewCoreEntityDelete(kind: $kind, uuid: $uuid) { kind uuid metadataRevision referenceCount blockers { code referenceCount } canDelete } }`;
export const DELETE_CORE_ENTITY = gql`mutation DeleteCoreEntity($kind: SearchEntityKind!, $uuid: ID!, $expectedMetadataRevision: Int64!) { deleteCoreEntity(kind: $kind, uuid: $uuid, expectedMetadataRevision: $expectedMetadataRevision) }`;
export const PREVIEW_GALLERY_DELETE = gql`query PreviewGalleryDelete($setID: ID!) { previewGalleryDelete(setID: $setID) { setID state metadataRevision itemCount externalLinkCount executableJobCount ignoredSourceWillBeCreated canDelete } }`;
export const DELETE_GALLERY = gql`mutation DeleteGallery($setID: ID!, $expectedMetadataRevision: Int64!, $password: String!, $confirmation: String!) { deleteGallery(setID: $setID, expectedMetadataRevision: $expectedMetadataRevision, password: $password, confirmation: $confirmation) }`;
