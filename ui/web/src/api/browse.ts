import { gql } from "@apollo/client";

export const GALLERY_CARD_FIELDS = gql`
  fragment GalleryCardFields on BrowseGalleryCard {
    setID slug title collectionType contentRating
    cover {
      kind revision managed warning
      resource { itemUUID contentRevision profileHash variant mimeType }
    }
    credits { uuid name avatarURL }
    creditCount
    characters { uuid name }
    characterCount
    works { uuid name }
    workCount
    shootDate
    shootDatePrecision
    addedAtUTC
    media { photo selfie gif video }
    favorite
    ratingHalfSteps
    scrubberCount
    scrubberRevision
  }
`;

export const BROWSE_GALLERIES = gql`
  ${GALLERY_CARD_FIELDS}
  query BrowseGalleries($scope: BrowseScope!, $page: Int!, $sort: GallerySort!, $collectionType: CollectionType) {
    browseGalleries(scope: $scope, page: $page, sort: $sort, collectionType: $collectionType) {
      page pageSize totalItems totalPages
      items { ...GalleryCardFields }
    }
  }
`;

export const HOME_GALLERIES = gql`
  ${GALLERY_CARD_FIELDS}
  query HomeGalleries($page: Int!) {
    homeGalleries(page: $page) {
      scope
      page {
        page pageSize totalItems totalPages
        items { ...GalleryCardFields }
      }
    }
  }
`;

export const BROWSE_UI_SETTINGS = gql`
  query BrowseUISettings {
    browseUISettings {
      settingsRevision
      galleryScrubberEnabled
      detailMediaFilterEnabled
      galleryAnimatedPlaybackLimit
      galleryAnimatedLockIntervalMS
      cardFavoriteControlVisible
      cardRatingSummaryVisible
      detailRatingControlVisible
    }
  }
`;

export const GALLERY_DETAIL = gql`
  ${GALLERY_CARD_FIELDS}
  query GalleryDetail($slug: String!) {
    galleryDetail(slug: $slug, scope: ALL) {
      card { ...GalleryCardFields }
      metadataRevision description photographerName studioName availableBytes mediaParentDirectories redirected
      credits { coser { uuid name avatarURL } characters { uuid name } works { uuid name } }
      tags { uuid name }
      externalLinks { uuid type label url }
    }
  }
`;

export const BROWSE_TAG_OPTIONS = gql`
  query BrowseTagOptions($query: String!, $limit: Int!) {
    manageCoreEntityOptions(kind: TAG, query: $query, limit: $limit, assignableOnly: true) {
      uuid name aliases
    }
  }
`;

export const REPLACE_GALLERY_TAGS = gql`
  mutation ReplaceGalleryTags($setID: ID!, $expectedMetadataRevision: Int64!, $tags: [ReplaceGalleryTagInput!]!) {
    replaceGalleryTags(setID: $setID, expectedMetadataRevision: $expectedMetadataRevision, tags: $tags) {
      metadataRevision
      tags { uuid name }
    }
  }
`;

export const GALLERY_MEMBER_INDEX = gql`
  query GalleryMemberIndex($setID: ID!) {
    galleryMemberIndex(setID: $setID) {
      setID metadataRevision scanRevision
      items {
        itemUUID mediaKind contentFormat imageCategory position caption processingState favorite ratingHalfSteps
        cardResource { itemUUID contentRevision profileHash variant mimeType }
        largeResource { itemUUID contentRevision profileHash variant mimeType }
      }
    }
  }
`;

export const ITEM_LIGHTBOX_STATUS = gql`
  query ItemLightboxStatus($itemUUID: ID!) {
    itemLightboxStatus(itemUUID: $itemUUID) {
      status errorCode
      resource { itemUUID contentRevision profileHash variant mimeType }
    }
  }
`;

export const REQUEST_ITEM_LIGHTBOX = gql`
  mutation RequestItemLightbox($itemUUID: ID!) {
    requestItemLightbox(itemUUID: $itemUUID) {
      status errorCode
      resource { itemUUID contentRevision profileHash variant mimeType }
    }
  }
`;

export const ITEM_ANIMATED_PREVIEW_STATUS = gql`
  query ItemAnimatedPreviewStatus($itemUUID: ID!) {
    itemAnimatedPreviewStatus(itemUUID: $itemUUID) {
      status errorCode
      resource { itemUUID contentRevision profileHash variant mimeType }
    }
  }
`;

export const REQUEST_ITEM_ANIMATED_PREVIEW = gql`
  mutation RequestItemAnimatedPreview($itemUUID: ID!) {
    requestItemAnimatedPreview(itemUUID: $itemUUID) {
      status errorCode
      resource { itemUUID contentRevision profileHash variant mimeType }
    }
  }
`;

export const ITEM_VIDEO_PLAYBACK_STATUS = gql`
  query ItemVideoPlaybackStatus($itemUUID: ID!) {
    itemVideoPlaybackStatus(itemUUID: $itemUUID) {
      itemUUID mode status contentRevision errorCode
      resource { itemUUID contentRevision profileHash variant mimeType }
    }
  }
`;

export const REQUEST_ITEM_VIDEO_PLAYBACK = gql`
  mutation RequestItemVideoPlayback($itemUUID: ID!) {
    requestItemVideoPlayback(itemUUID: $itemUUID) {
      itemUUID mode status contentRevision errorCode
      resource { itemUUID contentRevision profileHash variant mimeType }
    }
  }
`;

export const RELATED_GALLERIES = gql`
  ${GALLERY_CARD_FIELDS}
  query RelatedGalleries($setID: ID!) {
    relatedGalleries(setID: $setID, scope: ALL) {
      score reasons card { ...GalleryCardFields }
    }
  }
`;

export const TIMELINE_GALLERIES = gql`
  ${GALLERY_CARD_FIELDS}
  query TimelineGalleries($scope: BrowseScope!, $page: Int!, $coserUUID: ID) {
    timelineGalleries(scope: $scope, page: $page, coserUUID: $coserUUID) {
      page pageSize totalItems totalPages items { ...GalleryCardFields }
    }
  }
`;

export const RANDOM_MEDIA = gql`
  query RandomMedia($scope: BrowseScope!, $filter: RandomMediaFilter!) {
    randomMedia(scope: $scope, filter: $filter) {
      itemUUID mediaKind imageCategory gallerySetID gallerySlug favorite ratingHalfSteps
      resource { itemUUID contentRevision profileHash variant mimeType }
      characters { uuid name }
      cosers { uuid name }
    }
  }
`;

export const ENTITY_INDEX = gql`
  query EntityIndex($kind: SearchEntityKind!, $scope: BrowseScope!, $page: Int!, $sort: EntitySort!, $collectionType: CollectionType, $query: String!) {
    entityIndex(kind: $kind, scope: $scope, page: $page, sort: $sort, collectionType: $collectionType, query: $query) {
      page pageSize totalItems totalPages
      items { kind uuid slug name aliases avatarURL }
    }
  }
`;

export const SEARCH_PREVIEW = gql`
  query SearchPreview($query: String!, $scope: BrowseScope!) {
    searchPreview(query: $query, scope: $scope) {
      scope query
      galleries { kind uuid slug name matchLevel }
      cosers { kind uuid slug name matchLevel }
      works { kind uuid slug name matchLevel }
      characters { kind uuid slug name matchLevel }
      tags { kind uuid slug name matchLevel }
    }
  }
`;

export const COSER_DETAIL = gql`
  ${GALLERY_CARD_FIELDS}
  query CoserDetail($slug: String!, $scope: BrowseScope!, $page: Int!, $collectionType: CollectionType) {
    coserDetail(slug: $slug, scope: $scope, page: $page, collectionType: $collectionType) {
      entity { kind uuid slug name aliases avatarURL }
      profileSummary biography countryOrRegion bannerURL redirected
      socialAccounts { uuid platformKey label handle url status position }
      galleries { page pageSize totalItems totalPages items { ...GalleryCardFields } }
    }
  }
`;

export const WORK_DETAIL = gql`
  query WorkDetail($slug: String!, $scope: BrowseScope!) {
    workDetail(slug: $slug, scope: $scope) {
      entity { kind uuid slug name aliases }
      characters { kind uuid slug name aliases }
      redirected
    }
  }
`;

export const CHARACTER_DETAIL = gql`
  ${GALLERY_CARD_FIELDS}
  query CharacterDetail($slug: String!, $scope: BrowseScope!, $page: Int!) {
    characterDetail(slug: $slug, scope: $scope, page: $page) {
      entity { kind uuid slug name aliases }
      work { kind uuid slug name aliases }
      galleries { page pageSize totalItems totalPages items { ...GalleryCardFields } }
      redirected
    }
  }
`;

export const TAG_DETAIL = gql`
  ${GALLERY_CARD_FIELDS}
  query TagDetail($slug: String!, $scope: BrowseScope!, $page: Int!) {
    tagDetail(slug: $slug, scope: $scope, page: $page) {
      entity { kind uuid slug name aliases }
      galleries { page pageSize totalItems totalPages items { ...GalleryCardFields } }
      redirected
    }
  }
`;

export const MEDIA_DETAIL = gql`
  ${GALLERY_CARD_FIELDS}
  query MediaDetail($itemUUID: ID!) {
    mediaDetail(itemUUID: $itemUUID) {
      item { itemUUID mediaKind contentFormat imageCategory position caption processingState favorite ratingHalfSteps
        cardResource { itemUUID contentRevision profileHash variant mimeType }
        largeResource { itemUUID contentRevision profileHash variant mimeType } }
      displayResource { itemUUID contentRevision profileHash variant mimeType }
      videoTechnical { probeState errorCode container durationSeconds width height frameRate videoCodec audioCodec }
      metadataRevision
      gallery { ...GalleryCardFields }
    }
  }
`;

export const MEDIA_EMBEDDED_METADATA = gql`
  query MediaEmbeddedMetadata($itemUUID: ID!, $visibleFields: [String!]!) {
    mediaEmbeddedMetadata(itemUUID: $itemUUID, visibleFields: $visibleFields) {
      state errorCode entries { key visibilityKey label group value }
    }
  }
`;

export const FAVORITE_GALLERIES = gql`
  ${GALLERY_CARD_FIELDS}
  query FavoriteGalleries($scope: BrowseScope!, $page: Int!) {
    favoriteGalleries(scope: $scope, page: $page) { page pageSize totalItems totalPages items { ...GalleryCardFields } }
  }
`;
export const GALLERY_HISTORY = gql`
  ${GALLERY_CARD_FIELDS}
  query GalleryHistory($scope: BrowseScope!, $page: Int!) {
    galleryHistory(scope: $scope, page: $page) { page pageSize totalItems totalPages items { ...GalleryCardFields } }
  }
`;
export const FAVORITE_MEDIA = gql`
  query FavoriteMedia($scope: BrowseScope!, $page: Int!, $ratingSort: Boolean!) {
    favoriteMedia(scope: $scope, page: $page, ratingSort: $ratingSort) { page pageSize totalItems totalPages items {
      itemUUID mediaKind imageCategory gallerySetID gallerySlug favorite ratingHalfSteps
      resource { itemUUID contentRevision profileHash variant mimeType } characters { uuid name } cosers { uuid name }
    } }
  }
`;
export const SET_GALLERY_FAVORITE = gql`mutation SetGalleryFavorite($setID: ID!, $favorite: Boolean!) { setGalleryFavorite(setID: $setID, favorite: $favorite) { metadataRevision } }`;
export const SET_ITEM_FAVORITE = gql`mutation SetItemFavorite($itemUUID: ID!, $favorite: Boolean!) { setItemFavorite(itemUUID: $itemUUID, favorite: $favorite) { metadataRevision } }`;
export const SET_ITEM_RATING = gql`mutation SetItemRating($itemUUID: ID!, $ratingHalfSteps: Int, $expectedMetadataRevision: Int64!) { setItemRating(itemUUID: $itemUUID, ratingHalfSteps: $ratingHalfSteps, expectedMetadataRevision: $expectedMetadataRevision) { metadataRevision } }`;
export const SET_BROWSE_GALLERY_COVER_ITEM = gql`
  mutation SetBrowseGalleryCoverItem($setID: ID!, $itemUUID: ID!, $expectedMetadataRevision: Int64!) {
    setGalleryCoverItem(setID: $setID, itemUUID: $itemUUID, expectedMetadataRevision: $expectedMetadataRevision) {
      row { metadataRevision }
    }
  }
`;
export const RECORD_GALLERY_VIEW = gql`mutation RecordGalleryView($setID: ID!, $itemUUID: ID) { recordGalleryView(setID: $setID, itemUUID: $itemUUID) }`;
