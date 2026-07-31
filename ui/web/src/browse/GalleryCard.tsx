import { useMutation } from "@apollo/client/react";
import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";

import { GalleryCardPosterScrubber } from "./GalleryCardPosterScrubber";
import { SET_GALLERY_FAVORITE } from "../api/browse";
import { formatGalleryMediaCount } from "./galleryMediaCount";
import { galleryCardPresentation } from "./galleryCardPresentation";
import { galleryPreviewURL, itemResourceURL } from "./resourceUrl";
import type { BrowseGalleryCard } from "./types";
import { Avatar, CardMedia } from "../ui/Media";
import { Icon } from "../ui/Icon";

interface Props {
  card: BrowseGalleryCard;
  scrubberEnabled: boolean;
  favoriteControlVisible?: boolean;
  ratingSummaryVisible?: boolean;
  peopleVisible?: boolean;
}

export function GalleryCard({ card, scrubberEnabled, favoriteControlVisible = true, ratingSummaryVisible = true, peopleVisible = true }: Props) {
  const coverURL = itemResourceURL(card.cover.resource);
  const previewURL = useCallback((ordinal: number) => galleryPreviewURL(card, ordinal), [card]);
  const presentation = galleryCardPresentation(card);
  const mediaCount = formatGalleryMediaCount(card.media);
  const rating = card.ratingHalfSteps ? (card.ratingHalfSteps / 2).toFixed(1) : null;
  const personRoute = card.collectionType === "ALBUM" ? "model" : "coser";
  const [favorite, setFavorite] = useState(card.favorite);
  const [saveFavorite] = useMutation(SET_GALLERY_FAVORITE);
  useEffect(() => setFavorite(card.favorite), [card.favorite]);
  async function toggleFavorite() {
    const next = !favorite; setFavorite(next);
    try { await saveFavorite({ variables: { setID: card.setID, favorite: next } }); } catch { setFavorite(!next); }
  }

  return (
    <article className="gallery-card" data-rating={card.contentRating}>
      <CardMedia className="gallery-card__poster-frame"><Link className="gallery-card__poster" to={`/gallery/${encodeURIComponent(card.slug)}`} aria-label={card.title}>
          <GalleryCardPosterScrubber coverURL={coverURL} previewCount={card.scrubberCount} previewURL={previewURL} enabled={scrubberEnabled} alt={card.title} />
          {card.contentRating === "ADULT" ? <span className="gallery-card__r18" aria-label="R-18">R-18</span> : null}
          {mediaCount ? <span className="gallery-card__count" aria-label={`Media count ${mediaCount}`}>{mediaCount}</span> : null}
        </Link>{favoriteControlVisible ? <button className={`gallery-card__favorite${favorite ? " is-active" : ""}`} type="button" aria-label="Favourite" aria-pressed={favorite} onClick={toggleFavorite}><Icon name="heart" /></button> : null}</CardMedia>
      <div className="gallery-card__body">
        <h2 title={card.title}><Link to={`/gallery/${encodeURIComponent(card.slug)}`}>{presentation.primary}</Link></h2>
        <p className="gallery-card__work" title={presentation.secondary}>{presentation.secondary || "\u00a0"}</p>
        {peopleVisible ? <div className="gallery-card__people">
          <span className="gallery-card__cosers" title={presentation.cosers.map((coser) => coser.name).join(" · ")}>
            {presentation.cosers.map((coser) => (
              <Link className="gallery-card__coser-link" key={coser.uuid} to={`/${personRoute}/${encodeURIComponent(coser.uuid)}`} aria-label={coser.name}>
                <Avatar name={coser.name} size="small" />
                <span>{coser.name}</span>
              </Link>
            ))}
            {presentation.cosers.length === 0 ? "\u00a0" : null}
          </span>
          {rating && ratingSummaryVisible ? <span aria-label={`Rating ${rating} of 5`}>★ {rating}</span> : null}
        </div> : null}
      </div>
    </article>
  );
}
