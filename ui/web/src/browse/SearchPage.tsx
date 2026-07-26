import { useQuery } from "@apollo/client/react";
import { type FormEvent, useState } from "react";
import { useIntl } from "react-intl";
import { Link } from "react-router-dom";
import { SEARCH_PREVIEW } from "../api/browse";
import { ScopeSelector } from "./ScopeSelector";
import type { Scope, SearchHit, SearchPreview } from "./types";

export function SearchPage() {
  const intl = useIntl(); const [scope, setScope] = useState<Scope>("LIST"); const [input, setInput] = useState(""); const [query, setQuery] = useState("");
  const result = useQuery<{ searchPreview: SearchPreview }>(SEARCH_PREVIEW, { variables: { query, scope }, skip: !query });
  function submit(event: FormEvent) { event.preventDefault(); setQuery(input.trim()); }
  return <main className="browse-main"><header className="page-heading"><p>{intl.formatMessage({ id: "search.scope" })}: {scope}</p><h1>{intl.formatMessage({ id: "page.search" })}</h1></header>
    <form className="search-form" onSubmit={submit}><input value={input} onChange={(event) => setInput(event.target.value)} maxLength={300} autoFocus /><button type="submit">{intl.formatMessage({ id: "nav.search" })}</button></form>
    <ScopeSelector value={scope} onChange={setScope} />{result.data ? <SearchGroups value={result.data.searchPreview} /> : null}</main>;
}
function SearchGroups({ value }: { value: SearchPreview }) {
  const groups: Array<[string, SearchHit[]]> = [["Galleries", value.galleries], ["Cosers", value.cosers], ["Works", value.works], ["Characters", value.characters], ["Tags", value.tags]];
  return <div className="search-results">{groups.filter(([, items]) => items.length).map(([name, items]) => <section key={name}><h2>{name}</h2>{items.map((item) =>
    <Link key={`${item.kind}:${item.uuid}`} to={item.kind === "GALLERY" ? `/gallery/${item.slug}` : `/${item.kind.toLowerCase()}/${item.slug}`}><strong>{item.name}</strong><span>{item.matchLevel}</span></Link>)}</section>)}</div>;
}
