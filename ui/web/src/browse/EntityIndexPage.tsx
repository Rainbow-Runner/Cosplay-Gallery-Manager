import { useQuery } from "@apollo/client/react";
import { type FormEvent } from "react";
import { useIntl } from "react-intl";
import { Link, useSearchParams } from "react-router-dom";
import { ENTITY_INDEX } from "../api/browse";
import { Icon } from "../ui/Icon";
import { Breadcrumbs, Pagination } from "../ui/Patterns";
import { ScopeSelector } from "./ScopeSelector";
import type { CollectionType, EntityPage, Scope, SearchEntityKind } from "./types";

const routeNames: Record<SearchEntityKind, string> = { GALLERY: "gallery", COSER: "coser", WORK: "work", CHARACTER: "character", TAG: "tag" };
export function EntityIndexPage({ kind, titleID, collectionType, routeName, fixedScope, searchable = false }: {
  kind: Exclude<SearchEntityKind, "GALLERY">; titleID: string; collectionType?: CollectionType; routeName?: string; fixedScope?: Scope; searchable?: boolean;
}) {
  const intl = useIntl(); const [parameters, setParameters] = useSearchParams(); const scope = fixedScope ?? (parameters.get("scope") as Scope) ?? "LIST";
  const page = Math.max(1, Number(parameters.get("page")) || 1); const sort = kind === "COSER" && parameters.get("sort") === "RECENTLY_ADDED" ? "RECENTLY_ADDED" : "NAME";
  const query = parameters.get("query")?.trim() ?? ""; const title = intl.formatMessage({ id: titleID });
  const { data, loading, error } = useQuery<{ entityIndex: EntityPage }>(ENTITY_INDEX, { variables: { kind, scope, page, sort, collectionType, query } });
  function updatePage(next: number) { const updated = new URLSearchParams(parameters); updated.set("page", String(next)); setParameters(updated); }
  function submitSearch(event: FormEvent<HTMLFormElement>) { event.preventDefault(); const value = String(new FormData(event.currentTarget).get("query") ?? "").trim(); const updated = new URLSearchParams(parameters); value ? updated.set("query", value) : updated.delete("query"); updated.set("page", "1"); setParameters(updated); }
  return <main className="browse-main entity-index-page"><Breadcrumbs><li><Link to="/">{intl.formatMessage({ id: "nav.home" })}</Link></li><li aria-current="page">{title}</li></Breadcrumbs><h1 className="sr-only">{title}</h1>
    {searchable ? <form className="entity-index-search" onSubmit={submitSearch}><input key={query} name="query" defaultValue={query} maxLength={300} aria-label={intl.formatMessage({ id: "nav.search" })} /><button type="submit" aria-label={intl.formatMessage({ id: "nav.search" })}><Icon name="search" /></button></form> : null}
    {!fixedScope ? <ScopeSelector value={scope} onChange={(next) => setParameters({ scope: next, page: "1", sort, query })} /> : null}
    {loading ? <p className="state-message">{intl.formatMessage({ id: "state.loading" })}</p> : null}{error ? <p className="state-message" role="alert">{intl.formatMessage({ id: "state.error" })}</p> : null}
    {data?.entityIndex.items.length ? <section className={`entity-index entity-index--${kind.toLowerCase()}`}>{data.entityIndex.items.map((item) =>
      <Link key={item.uuid} to={`/${routeName ?? routeNames[kind]}/${item.slug}`} aria-label={item.name}>{kind === "COSER" ? <div className="entity-index__avatar" aria-hidden="true">{item.avatarURL ? <img src={item.avatarURL} alt="" /> : item.name.slice(0, 1)}</div> : null}<strong>{item.name}</strong>{kind !== "COSER" && item.aliases.length ? <span>{item.aliases.join(" / ")}</span> : null}</Link>)}</section> : null}
    {data && data.entityIndex.totalPages > 1 ? <Pagination page={page} totalPages={data.entityIndex.totalPages} previousLabel={intl.formatMessage({ id: "pagination.previous" })} nextLabel={intl.formatMessage({ id: "pagination.next" })} onPageChange={updatePage} /> : null}
  </main>;
}
