import type { GalleryMember } from "./types";
export type GalleryMediaOrder="DEFAULT"|"CAPTURE_ASC"|"CAPTURE_DESC";
export function readGalleryMediaOrder(value: string|null): GalleryMediaOrder { return value==="CAPTURE_ASC"||value==="CAPTURE_DESC"?value:"DEFAULT"; }
export function sortGalleryMedia(items: GalleryMember[],order: GalleryMediaOrder) {
 if(order==="DEFAULT")return items;
 return items.map((item,index)=>({item,index})).sort((a,b)=>{
  const x=a.item.captureTimeUTC??"",y=b.item.captureTimeUTC??"";
  const foundX=!!x&&Number.isFinite(Date.parse(x)),foundY=!!y&&Number.isFinite(Date.parse(y));
  if(foundX!==foundY)return foundX?-1:1;
  const comparison=foundX&&foundY?(x<y?-1:x>y?1:0):0;
  return (order==="CAPTURE_DESC"?-comparison:comparison)||a.index-b.index;
 }).map(entry=>entry.item);
}
