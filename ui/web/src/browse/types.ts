export type Scope = "LIST" | "MAGIC" | "ALL";
export type ContentRating = "NON_ADULT" | "ADULT";
export type CollectionType = "COSPLAY" | "ALBUM";

export interface ResourceIdentity {
  itemUUID: string;
  contentRevision: number;
  profileHash: string;
  variant: string;
  mimeType: string;
}

export interface EntitySummary {
  uuid: string;
  name: string;
}

export interface PersonSummary extends EntitySummary {
  avatarURL?: string | null;
}

export interface BrowseGalleryCard {
  setID: string;
  slug: string;
  title: string;
  collectionType: CollectionType;
  contentRating: ContentRating;
  cover: {
    kind: string;
    revision: number;
    managed: boolean;
    resource?: ResourceIdentity | null;
    warning: boolean;
  };
  credits: PersonSummary[];
  creditCount: number;
  characters: EntitySummary[];
  characterCount: number;
  works: EntitySummary[];
  workCount: number;
  shootDate?: string | null;
  shootDatePrecision?: "MONTH" | "DAY" | null;
  addedAtUTC: string;
  media: { photo: number; selfie: number; gif: number; video: number };
  favorite: boolean;
  ratingHalfSteps?: number | null;
  scrubberCount: number;
  scrubberRevision: number;
}

export interface GalleryPage {
  items: BrowseGalleryCard[];
  page: number;
  pageSize: number;
  totalItems: number;
  totalPages: number;
}

export interface BrowseUISettings {
  settingsRevision: number;
  galleryScrubberEnabled: boolean;
  detailMediaFilterEnabled: boolean;
  galleryAnimatedPlaybackLimit: number;
  galleryAnimatedLockIntervalMS: number;
  cardFavoriteControlVisible: boolean;
  cardRatingSummaryVisible: boolean;
  detailRatingControlVisible: boolean;
}

export interface GalleryCreditDetail {
  coser: PersonSummary;
  characters: EntitySummary[];
  works: EntitySummary[];
}

export interface GalleryDetail {
  card: BrowseGalleryCard;
  description: string;
  photographerName: string;
  studioName: string;
  availableBytes: number;
  mediaParentDirectories: string[];
  credits: GalleryCreditDetail[];
  tags: EntitySummary[];
  externalLinks: Array<{ uuid: string; type: string; label: string; url: string }>;
  redirected: boolean;
}

export interface GalleryMember {
  itemUUID: string;
  mediaKind: "STATIC_IMAGE" | "ANIMATED_IMAGE" | "VIDEO";
  contentFormat: "IMAGE" | "RAW" | "VIDEO";
  imageCategory?: "PHOTO" | "SELFIE" | null;
  position: string;
  caption: string;
  processingState: "PENDING" | "PROCESSING" | "READY" | "ERROR";
  cardResource?: ResourceIdentity | null;
  largeResource?: ResourceIdentity | null;
  favorite: boolean;
  ratingHalfSteps?: number | null;
}

export interface GalleryMemberIndex {
  setID: string;
  metadataRevision: number;
  scanRevision: number;
  items: GalleryMember[];
}

export interface OnDemandResource {
  status: GalleryMember["processingState"];
  resource?: ResourceIdentity | null;
  errorCode: string;
}

export interface VideoPlaybackStatus {
  itemUUID: string;
  mode: "" | "DIRECT" | "REMUX" | "TRANSCODE";
  status: GalleryMember["processingState"];
  contentRevision: number;
  resource?: ResourceIdentity | null;
  errorCode: string;
}

export type SearchEntityKind = "GALLERY" | "COSER" | "WORK" | "CHARACTER" | "TAG";
export interface EntityIndexItem { kind: SearchEntityKind; uuid: string; slug: string; name: string; aliases: string[]; avatarURL?: string | null }
export interface EntityPage { items: EntityIndexItem[]; page: number; pageSize: number; totalItems: number; totalPages: number }
export interface SearchHit { kind: SearchEntityKind; uuid: string; slug: string; name: string; matchLevel: number }
export interface SearchPreview { scope: Scope; query: string; galleries: SearchHit[]; cosers: SearchHit[]; works: SearchHit[]; characters: SearchHit[]; tags: SearchHit[] }
export interface RandomMediaItem {
  itemUUID: string; mediaKind: GalleryMember["mediaKind"]; imageCategory?: GalleryMember["imageCategory"];
  resource: ResourceIdentity; gallerySetID: string; gallerySlug: string; characters: EntitySummary[]; cosers: EntitySummary[];
  favorite: boolean; ratingHalfSteps?: number | null;
}
export interface SocialAccount { uuid: string; platformKey: string; label: string; handle: string; url: string; status: "ACTIVE" | "INACTIVE"; position: string }
export interface CoserDetail { entity: EntityIndexItem; profileSummary: string; biography: string; countryOrRegion: string; bannerURL?: string | null; socialAccounts: SocialAccount[]; galleries: GalleryPage; redirected: boolean }
export interface WorkDetail { entity: EntityIndexItem; characters: EntityIndexItem[]; redirected: boolean }
export interface CharacterDetail { entity: EntityIndexItem; work: EntityIndexItem; galleries: GalleryPage; redirected: boolean }
export interface TagDetail { entity: EntityIndexItem; galleries: GalleryPage; redirected: boolean }
export interface MediaInformation { state: "READY" | "ERROR"; errorCode: string; entries: { key: string; visibilityKey: string; label: string; group: string; value: string }[] }
export interface MediaDetail { item: GalleryMember; displayResource?: ResourceIdentity | null; gallery: BrowseGalleryCard; metadataRevision: number; videoTechnical?: { probeState: string; errorCode: string; container: string; durationSeconds: number; width: number; height: number; frameRate: number; videoCodec: string; audioCodec: string } | null }
export interface MediaPage { items: RandomMediaItem[]; page: number; pageSize: number; totalItems: number; totalPages: number }
