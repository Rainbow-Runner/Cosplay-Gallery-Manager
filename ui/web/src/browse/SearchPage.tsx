import { useQuery } from "@apollo/client/react";
import { type FormEvent, useState, useEffect, useRef, useCallback } from "react";
import { useIntl } from "react-intl";
import { Link } from "react-router-dom";
import { SEARCH_PREVIEW, SEARCH_RESULTS } from "../api/browse";
import { Icon, type IconName } from "../ui/Icon";
import { itemResourceURL } from "./resourceUrl";
import { ScopeSelector } from "./ScopeSelector";
import type { Scope, SearchHit, SearchPreview, SearchEntityKind } from "./types";

export function SearchPage() {
  const intl = useIntl(); const [scope, setScope] = useState<Scope>("LIST"); const [input, setInput] = useState(""); const [query, setQuery] = useState("");
  const [allKind,setAllKind]=useState<SearchEntityKind|null>(null);
  const result = useQuery<{ searchPreview: SearchPreview }>(SEARCH_PREVIEW, { variables: { query, scope }, skip: !query });
  function submit(event: FormEvent) { event.preventDefault(); setQuery(input.trim()); setAllKind(null); }
  return <main className="browse-main search-page"><header className="page-heading"><p>{intl.formatMessage({ id: "search.scope" })}: {scope}</p><h1>{intl.formatMessage({ id: "page.search" })}</h1></header>
    <form className="search-form" onSubmit={submit}><input aria-label={intl.formatMessage({ id: "nav.search" })} value={input} onChange={(event) => setInput(event.target.value)} maxLength={300} autoFocus /><button type="submit">{intl.formatMessage({ id: "nav.search" })}</button></form>
    <ScopeSelector value={scope} onChange={(value)=>{setScope(value);setAllKind(null)}} />{allKind ? <SearchAll key={`${query}:${scope}:${allKind}`} query={query} scope={scope} kind={allKind} onBack={()=>setAllKind(null)} /> : result.loading ? <p className="state-message">{intl.formatMessage({ id: "state.loading" })}</p> : result.error ? <p className="state-message" role="alert">{intl.formatMessage({ id: "state.error" })}</p> : result.data ? <SearchGroups value={result.data.searchPreview} onViewAll={setAllKind} /> : null}</main>;
}
function SearchGroups({ value,onViewAll }: { value: SearchPreview; onViewAll?: (kind:SearchEntityKind)=>void }) {
  const intl = useIntl();
  const groups: Array<[string, SearchHit[]]> = [["search.galleries", value.galleries], ["search.cosers", value.cosers], ["search.works", value.works], ["search.characters", value.characters], ["search.tags", value.tags]];
  const visible = groups.filter(([, items]) => items.length);
  if (!visible.length) return <p className="state-message">{intl.formatMessage({ id: "search.noResults" })}</p>;
  const icons: Record<SearchHit["kind"], IconName> = { GALLERY: "gallery", COSER: "user", WORK: "book", CHARACTER: "character", TAG: "tags" };
  const labels: Record<SearchHit["kind"], string> = { GALLERY: "search.gallery", COSER: "search.coser", WORK: "search.work", CHARACTER: "search.character", TAG: "search.tag" };
  const totals:Record<string,number>={"search.galleries":value.galleriesTotal,"search.cosers":value.cosersTotal,"search.works":value.worksTotal,"search.characters":value.charactersTotal,"search.tags":value.tagsTotal};
  return <div className="search-results">{visible.map(([name, items]) => <section className="search-results__group" key={name}><header><h2>{intl.formatMessage({ id: name })}</h2><span>{totals[name]}</span>{onViewAll && totals[name]>items.length ? <button type="button" onClick={()=>onViewAll(items[0].kind)} aria-label={`${intl.formatMessage({id:"search.viewAll"})} ${intl.formatMessage({id:name})}`}>{intl.formatMessage({id:"search.viewAll"})}</button>:null}</header><div className="search-results__list">{items.map((item) => {
    const coverURL = item.kind === "GALLERY" ? itemResourceURL(item.coverResource) : null;
    const route = item.kind === "GALLERY" ? `/gallery/${item.slug}` : `/${item.kind.toLowerCase()}/${item.slug}`;
    const missing = "--:--:--";
    const added = item.mediaAddedStartUTC ? `${item.mediaAddedStartUTC.slice(0, 10)}${item.mediaAddedEndUTC && item.mediaAddedEndUTC.slice(0, 10) !== item.mediaAddedStartUTC.slice(0, 10) ? ` – ${item.mediaAddedEndUTC.slice(0, 10)}` : ""}` : missing;
    return <Link className="search-result" key={`${item.kind}:${item.uuid}`} to={route}><span className={`search-result__visual${item.kind === "GALLERY" ? " is-gallery" : ""}`}>{coverURL ? <img src={coverURL} alt="" loading="lazy" /> : <Icon name={icons[item.kind]} aria-hidden="true" />}</span><span className="search-result__content"><strong>{item.name}</strong><small>{intl.formatMessage({ id: labels[item.kind] })}</small>{item.kind === "GALLERY" ? <span className="search-result__dates"><span>{intl.formatMessage({ id: "gallery.addedDate" })}: {added}</span><span>{intl.formatMessage({ id: "gallery.shootDate" })}: {item.shootDate || missing}</span><span>{intl.formatMessage({ id: "gallery.publishDate" })}: {item.publishDate || missing}</span></span> : null}</span><Icon className="search-result__arrow" name="chevron-right" aria-hidden="true" /></Link>;
  })}</div></section>)}</div>;
}

interface ResultBatch {items:SearchHit[];total:number;endCursor:string;hasNext:boolean}
function SearchAll({query,scope,kind,onBack}:{query:string;scope:Scope;kind:SearchEntityKind;onBack:()=>void}) {
 const intl=useIntl();const sentinel=useRef<HTMLDivElement>(null);const pending=useRef(false);const [failed,setFailed]=useState(false);
 const result=useQuery<{searchResults:ResultBatch}>(SEARCH_RESULTS,{variables:{query,scope,kind,first:24,after:""}});
 const batch=result.data?.searchResults;
 const load=useCallback(async()=>{
  if(pending.current||!batch?.hasNext||result.loading)return;
  pending.current=true;setFailed(false);
  try { await result.fetchMore({variables:{after:batch.endCursor},updateQuery:(previous,{fetchMoreResult})=>{
   if(!fetchMoreResult)return previous;
   const next=fetchMoreResult.searchResults;const items=[...new Map([...previous.searchResults.items,...next.items].map(item=>[`${item.kind}:${item.uuid}`,item])).values()];
   return {searchResults:{...next,items,hasNext:next.hasNext&&next.endCursor!==previous.searchResults.endCursor}};
  }}); }catch {setFailed(true)}finally{pending.current=false}
 },[batch,result.loading,result.fetchMore]);
 useEffect(()=>{
  if(!sentinel.current||!batch?.hasNext||failed||result.loading||typeof IntersectionObserver==="undefined")return;
  const observer=new IntersectionObserver(entries=>{if(entries.some(entry=>entry.isIntersecting))void load()},{rootMargin:"300px"});observer.observe(sentinel.current);return()=>observer.disconnect();
 },[batch,failed,result.loading,load]);
 const value:SearchPreview={scope,query,galleries:[],cosers:[],works:[],characters:[],tags:[],galleriesTotal:0,cosersTotal:0,worksTotal:0,charactersTotal:0,tagsTotal:0};
 const key=({GALLERY:"galleries",COSER:"cosers",WORK:"works",CHARACTER:"characters",TAG:"tags"} as const)[kind];
 value[key]=batch?.items||[];value[`${key}Total`]=batch?.total||0;
 return <section className="search-all"><button type="button" onClick={onBack}>{intl.formatMessage({id:"search.back"})}</button>
 {batch?<SearchGroups value={value}/>:null}
 {result.error&&!batch?<p role="alert">{intl.formatMessage({id:"state.error"})}<button type="button" onClick={()=>void result.refetch().catch(()=>{})}>{intl.formatMessage({id:"search.retry"})}</button></p>:null}
 {result.loading?<p role="status">{intl.formatMessage({id:"state.loading"})}</p>:null}
 {failed?<p role="alert">{intl.formatMessage({id:"state.error"})}</p>:null}
 <div ref={sentinel} className="search-all__footer">{batch?.hasNext?<button type="button" disabled={result.loading} onClick={()=>void load()}>{intl.formatMessage({id:failed?"search.retry":"search.loadMore"})}</button>:batch?.items.length?<p>{intl.formatMessage({id:"search.finished"})}</p>:null}</div>
 </section>;
}
