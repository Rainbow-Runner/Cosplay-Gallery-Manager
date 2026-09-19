import { useQuery } from "@apollo/client/react";
import { type FormEvent, useState } from "react";
import { useIntl } from "react-intl";
import { Link } from "react-router-dom";
import { SEARCH_PREVIEW } from "../api/browse";
import { Icon, type IconName } from "../ui/Icon";
import { itemResourceURL } from "./resourceUrl";
import { ScopeSelector } from "./ScopeSelector";
import type { Scope, SearchHit, SearchPreview } from "./types";

export function SearchPage() {
  const intl = useIntl(); const [scope, setScope] = useState<Scope>("LIST"); const [input, setInput] = useState(""); const [query, setQuery] = useState("");
  const result = useQuery<{ searchPreview: SearchPreview }>(SEARCH_PREVIEW, { variables: { query, scope }, skip: !query });
  function submit(event: FormEvent) { event.preventDefault(); setQuery(input.trim()); }
  return <main className="browse-main search-page"><header className="page-heading"><p>{intl.formatMessage({ id: "search.scope" })}: {scope}</p><h1>{intl.formatMessage({ id: "page.search" })}</h1></header>
    <form className="search-form" onSubmit={submit}><input aria-label={intl.formatMessage({ id: "nav.search" })} value={input} onChange={(event) => setInput(event.target.value)} maxLength={300} autoFocus /><button type="submit">{intl.formatMessage({ id: "nav.search" })}</button></form>
    <ScopeSelector value={scope} onChange={setScope} />{result.loading ? <p className="state-message">{intl.formatMessage({ id: "state.loading" })}</p> : result.error ? <p className="state-message" role="alert">{intl.formatMessage({ id: "state.error" })}</p> : result.data ? <SearchGroups value={result.data.searchPreview} /> : null}</main>;
}
function SearchGroups({ value }: { value: SearchPreview }) {
  const intl = useIntl();
  const groups: Array<[string, SearchHit[]]> = [["search.galleries", value.galleries], ["search.cosers", value.cosers], ["search.works", value.works], ["search.characters", value.characters], ["search.tags", value.tags]];
  const visible = groups.filter(([, items]) => items.length);
  if (!visible.length) return <p className="state-message">{intl.formatMessage({ id: "search.noResults" })}</p>;
  const icons: Record<SearchHit["kind"], IconName> = { GALLERY: "gallery", COSER: "user", WORK: "book", CHARACTER: "character", TAG: "tags" };
  const labels: Record<SearchHit["kind"], string> = { GALLERY: "search.gallery", COSER: "search.coser", WORK: "search.work", CHARACTER: "search.character", TAG: "search.tag" };
  return <div className="search-results">{visible.map(([name, items]) => <section className="search-results__group" key={name}><header><h2>{intl.formatMessage({ id: name })}</h2><span>{items.length}</span></header><div className="search-results__list">{items.map((item) => {
    const coverURL = item.kind === "GALLERY" ? itemResourceURL(item.coverResource) : null;
    const route = item.kind === "GALLERY" ? `/gallery/${item.slug}` : `/${item.kind.toLowerCase()}/${item.slug}`;
    return <Link className="search-result" key={`${item.kind}:${item.uuid}`} to={route}><span className={`search-result__visual${item.kind === "GALLERY" ? " is-gallery" : ""}`}>{coverURL ? <img src={coverURL} alt="" loading="lazy" /> : <Icon name={icons[item.kind]} aria-hidden="true" />}</span><span className="search-result__content"><strong>{item.name}</strong><small>{intl.formatMessage({ id: labels[item.kind] })}</small></span><Icon className="search-result__arrow" name="chevron-right" aria-hidden="true" /></Link>;
  })}</div></section>)}</div>;
}
