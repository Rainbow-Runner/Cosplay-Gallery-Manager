import { useMutation } from "@apollo/client/react";
import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";

import { GalleryCardPosterScrubber } from "./GalleryCardPosterScrubber";
import { SET_GALLERY_FAVORITE } from "../api/browse";
import { galleryPreviewURL, itemResourceURL } from "./resourceUrl";
import type { BrowseGalleryCard } from "./types";

interface Props {
  card: BrowseGalleryCard;
  scrubberEnabled: boolean;
  favoriteControlVisible?: boolean;
  ratingSummaryVisible?: boolean;
}

function names(values: { name: string }[]) {
  return values.map((value) => value.name).join(" · ");
}

export function GalleryCard({ card, scrubberEnabled, favoriteControlVisible = true, ratingSummaryVisible = true }: Props) {
  const coverURL = itemResourceURL(card.cover.resource);
  const previewURL = useCallback((ordinal: number) => galleryPreviewURL(card, ordinal), [card]);
  const subtitle = card.collectionType === "ALBUM" ? names(card.credits) : names(card.characters);
  const rating = card.ratingHalfSteps ? (card.ratingHalfSteps / 2).toFixed(1) : null;
  const [favorite, setFavorite] = useState(card.favorite);
  const [saveFavorite] = useMutation(SET_GALLERY_FAVORITE);
  useEffect(() => setFavorite(card.favorite), [card.favorite]);
  async function toggleFavorite() {
    const next = !favorite; setFavorite(next);
    try { await saveFavorite({ variables: { setID: card.setID, favorite: next } }); } catch { setFavorite(!next); }
  }

  return (
    <article className="gallery-card" data-rating={card.contentRating}>
      <div className="gallery-card__poster-frame"><Link className="gallery-card__poster" to={`/gallery/${encodeURIComponent(card.slug)}`} aria-label={card.title}>
          <GalleryCardPosterScrubber coverURL={coverURL} previewCount={card.scrubberCount} previewURL={previewURL} enabled={scrubberEnabled} alt={card.title} />
          {card.contentRating === "ADULT" ? <span className="gallery-card__r18" aria-label="R-18">R-18</span> : null}
        </Link>{favoriteControlVisible ? <button className="gallery-card__favorite" type="button" aria-label="Favourite" onClick={toggleFavorite}>{favorite ? "♥" : "♡"}</button> : null}</div>
      <div className="gallery-card__body">
        <p className="gallery-card__kind">{card.collectionType}</p>
        <h2><Link to={`/gallery/${encodeURIComponent(card.slug)}`}>{card.title}</Link></h2>
        <p className="gallery-card__relation" title={subtitle}>{subtitle || "\u00a0"}</p>
        <div className="gallery-card__meta">
          <span>P{card.media.photo} / S{card.media.selfie} / G{card.media.gif} / V{card.media.video}</span>
          {rating && ratingSummaryVisible ? <span aria-label={`Rating ${rating} of 5`}>★ {rating}</span> : null}
        </div>
      </div>
    </article>
  );
}
