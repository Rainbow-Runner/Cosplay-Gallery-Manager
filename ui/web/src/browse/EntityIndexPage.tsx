import { useQuery } from "@apollo/client/react";
import { useIntl } from "react-intl";
import { Link, useSearchParams } from "react-router-dom";
import { ENTITY_INDEX } from "../api/browse";
import { ScopeSelector } from "./ScopeSelector";
import type { EntityPage, Scope, SearchEntityKind } from "./types";

const routeNames: Record<SearchEntityKind, string> = { GALLERY: "gallery", COSER: "coser", WORK: "work", CHARACTER: "character", TAG: "tag" };
export function EntityIndexPage({ kind, titleID }: { kind: Exclude<SearchEntityKind, "GALLERY">; titleID: string }) {
  const intl = useIntl(); const [parameters, setParameters] = useSearchParams(); const scope = (parameters.get("scope") as Scope) || "LIST";
  const page = Math.max(1, Number(parameters.get("page")) || 1); const sort = kind === "COSER" && parameters.get("sort") === "RECENTLY_ADDED" ? "RECENTLY_ADDED" : "NAME";
  const { data, loading, error } = useQuery<{ entityIndex: EntityPage }>(ENTITY_INDEX, { variables: { kind, scope, page, sort } });
  return <main className="browse-main"><header className="page-heading"><p>{scope}</p><h1>{intl.formatMessage({ id: titleID })}</h1></header>
    <ScopeSelector value={scope} onChange={(next) => setParameters({ scope: next, page: "1", sort })} />
    {loading ? <p className="state-message">{intl.formatMessage({ id: "state.loading" })}</p> : null}{error ? <p className="state-message" role="alert">{intl.formatMessage({ id: "state.error" })}</p> : null}
    {data?.entityIndex.items.length ? <section className={`entity-index entity-index--${kind.toLowerCase()}`}>{data.entityIndex.items.map((item) =>
      <Link key={item.uuid} to={`/${routeNames[kind]}/${item.slug}`}>{kind === "COSER" ? <div className="entity-index__avatar">{item.avatarURL ? <img src={item.avatarURL} alt="" /> : item.name.slice(0, 1)}</div> : null}<strong>{item.name}</strong>{item.aliases.length ? <span>{item.aliases.join(" / ")}</span> : null}</Link>)}</section> : null}
  </main>;
}
