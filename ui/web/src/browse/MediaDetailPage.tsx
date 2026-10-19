import { useMutation, useQuery } from "@apollo/client/react";
import { useEffect, useState } from "react";
import { useIntl } from "react-intl";
import { useLocation, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { BROWSE_UI_SETTINGS, MEDIA_DETAIL, RECORD_GALLERY_VIEW, SET_ITEM_FAVORITE, SET_ITEM_RATING } from "../api/browse";
import { Icon } from "../ui/Icon";
import type { BrowseUISettings, MediaDetail } from "./types";
import { MediaViewer } from "./MediaViewer";
import { viewerBackgroundLocation } from "./mediaViewerRoute";
import { MediaViewerPlaceholder } from "./MediaViewerPlaceholder";

export function MediaDetailPage() {
  const intl=useIntl(); const t=(key:string)=>intl.formatMessage({id:key});
  const {uuid=""}=useParams(); const location=useLocation(); const navigate=useNavigate(); const [parameters,setParameters]=useSearchParams();
  const query=useQuery<{mediaDetail:MediaDetail}>(MEDIA_DETAIL,{variables:{itemUUID:uuid},fetchPolicy:"cache-and-network"});
  const [recordView]=useMutation(RECORD_GALLERY_VIEW);
  const [setFavorite,favoriteState]=useMutation(SET_ITEM_FAVORITE); const [setRating,ratingState]=useMutation(SET_ITEM_RATING);
  const settings=useQuery<{browseUISettings:BrowseUISettings}>(BROWSE_UI_SETTINGS);
  const [favorite,updateFavorite]=useState(false); const [rating,updateRating]=useState<number|null>(null); const [revision,updateRevision]=useState(0);
  const detail=query.data?.mediaDetail;
  useEffect(()=>{if(detail)void recordView({variables:{setID:detail.gallery.setID,itemUUID:uuid}});},[detail?.gallery.setID,uuid,recordView]);
  useEffect(()=>{if(detail){updateFavorite(detail.item.favorite);updateRating(detail.item.ratingHalfSteps??null);updateRevision(detail.metadataRevision);}},[detail]);
  const background=viewerBackgroundLocation(location);
  function close(){if(background)navigate(-1);else navigate(detail ? `/gallery/${encodeURIComponent(detail.gallery.slug)}` : "/",{replace:true});}
  if(!detail)return <MediaViewerPlaceholder failed={!!query.error} onClose={close}/>;
  const informationAllowed=!(parameters.get("from")==="random" && detail.item.mediaKind==="STATIC_IMAGE");
  const favoriteLabel=t(favorite ? "gallery.itemUnfavorite" : "gallery.itemFavorite");
  const busy=favoriteState.loading||ratingState.loading;
  return <MediaViewer item={detail.item} informationAllowed={informationAllowed} informationOpen={informationAllowed && parameters.get("info")!=="0"}
    onInformationChange={value=>{const next=new URLSearchParams(parameters);next.set("info",value?"1":"0");setParameters(next,{replace:true,state:location.state});}}
    onClose={close} galleryHref={`/gallery/${encodeURIComponent(detail.gallery.slug)}`}
    actions={settings.data?.browseUISettings.detailRatingControlVisible ? <>
      <button type="button" disabled={busy} aria-label={favoriteLabel} title={favoriteLabel} aria-pressed={favorite} className={favorite?"is-active":""} onClick={async()=>{const next=!favorite;updateFavorite(next);try{await setFavorite({variables:{itemUUID:uuid,favorite:next}});}catch{updateFavorite(!next);}}}><Icon name="heart"/></button>
      <select disabled={busy} aria-label={t("gallery.rating")} value={rating??0} onChange={async event=>{const next=Number(event.target.value)||null;const previous=rating;updateRating(next);try{await setRating({variables:{itemUUID:uuid,ratingHalfSteps:next,expectedMetadataRevision:revision}});updateRevision(current=>current+1);}catch{updateRating(previous);}}}><option value={0}>—</option>{Array.from({length:10},(_,index)=>index+1).map(value=><option key={value} value={value}>{value/2} / 5</option>)}</select>
    </> : null} />;
}
