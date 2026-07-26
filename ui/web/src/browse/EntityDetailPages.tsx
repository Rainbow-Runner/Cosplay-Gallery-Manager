import { useQuery } from "@apollo/client/react";
import { useEffect } from "react";
import { useIntl } from "react-intl";
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { CHARACTER_DETAIL, COSER_DETAIL, TAG_DETAIL, WORK_DETAIL } from "../api/browse";
import { GalleryCard } from "./GalleryCard";
import { ScopeSelector } from "./ScopeSelector";
import { TimelinePage } from "./TimelinePage";
import type { CharacterDetail, CoserDetail, GalleryPage, Scope, TagDetail, WorkDetail } from "./types";

function useDetailRoute() {
  const { slug = "" } = useParams(); const navigate = useNavigate(); const [parameters, setParameters] = useSearchParams();
  const scope = (parameters.get("scope") as Scope) || "ALL"; const page = Math.max(1, Number(parameters.get("page")) || 1);
  return { slug, navigate, scope, page, setScope: (next: Scope) => setParameters({ scope: next, page: "1" }) };
}

export function CoserDetailPage() {
  const intl = useIntl(); const route = useDetailRoute();
  const query = useQuery<{ coserDetail: CoserDetail }>(COSER_DETAIL, { variables: { slug: route.slug, scope: route.scope, page: route.page } });
  const detail = query.data?.coserDetail;
  useCanonicalDetail(detail, route.slug, "coser", route.navigate);
  if (query.loading) return <Loading />; if (query.error || !detail) return <ErrorState />;
  return <main className="browse-main">{detail.bannerURL ? <div className="coser-banner"><img src={detail.bannerURL} alt="" /></div> : null}<section className="coser-hero"><div className="coser-avatar">{detail.entity.avatarURL ? <img src={detail.entity.avatarURL} alt="" /> : detail.entity.name.slice(0, 1)}</div><div><p>COSER</p><h1>{detail.entity.name}</h1>
    {detail.countryOrRegion ? <span>{detail.countryOrRegion}</span> : null}{detail.profileSummary ? <details><summary>{intl.formatMessage({ id: "coser.profile" })}</summary><p>{detail.profileSummary}</p></details> : null}</div></section>
    {detail.biography ? <section className="coser-biography"><h2>{intl.formatMessage({ id: "coser.biography" })}</h2><p>{detail.biography}</p></section> : null}
    {detail.socialAccounts.length ? <section className="social-accounts">{detail.socialAccounts.map((account) => <a key={account.uuid} className={account.status === "INACTIVE" ? "is-inactive" : ""} href={account.url} target="_blank" rel="noreferrer"><span>{account.platformKey}</span>{account.label || account.handle}</a>)}</section> : null}
    <div className="entity-actions"><Link to={`/coser/${detail.entity.slug}/timeline`}>{intl.formatMessage({ id: "coser.timeline" })}</Link></div>
    <GalleryResults title={intl.formatMessage({ id: "coser.galleries" })} scope={route.scope} onScope={route.setScope} page={detail.galleries} />
  </main>;
}

export function CoserTimelinePage() {
  const { slug = "" } = useParams(); const query = useQuery<{ coserDetail: CoserDetail }>(COSER_DETAIL, { variables: { slug, scope: "ALL", page: 1 } });
  if (!query.data) return <Loading />;
  return <TimelinePage coserUUID={query.data.coserDetail.entity.uuid} />;
}

export function WorkDetailPage() {
  const route = useDetailRoute(); const query = useQuery<{ workDetail: WorkDetail }>(WORK_DETAIL, { variables: { slug: route.slug, scope: route.scope } }); const detail = query.data?.workDetail;
  useCanonicalDetail(detail, route.slug, "work", route.navigate); if (query.loading) return <Loading />; if (!detail || query.error) return <ErrorState />;
  return <main className="browse-main"><TextDetailHeading scope={route.scope} name={detail.entity.name} /><ScopeSelector value={route.scope} onChange={route.setScope} />
    <section className="character-text-index">{detail.characters.map((character) => <Link key={character.uuid} to={`/character/${character.slug}`}>{character.name}</Link>)}</section></main>;
}

export function CharacterDetailPage() { return <GalleryEntityDetail kind="character" queryDocument={CHARACTER_DETAIL} />; }
export function TagDetailPage() { return <GalleryEntityDetail kind="tag" queryDocument={TAG_DETAIL} />; }

function GalleryEntityDetail({ kind, queryDocument }: { kind: "character" | "tag"; queryDocument: typeof CHARACTER_DETAIL }) {
  const route = useDetailRoute(); const result = useQuery<{ characterDetail?: CharacterDetail; tagDetail?: TagDetail }>(queryDocument, { variables: { slug: route.slug, scope: route.scope, page: route.page } });
  const detail = kind === "character" ? result.data?.characterDetail : result.data?.tagDetail;
  useCanonicalDetail(detail, route.slug, kind, route.navigate); if (result.loading) return <Loading />; if (!detail || result.error) return <ErrorState />;
  return <main className="browse-main"><TextDetailHeading scope={route.scope} name={detail.entity.name} />
    {kind === "character" ? <Link className="detail-parent" to={`/work/${(detail as CharacterDetail).work.slug}`}>{(detail as CharacterDetail).work.name}</Link> : null}
    <GalleryResults title={detail.entity.name} scope={route.scope} onScope={route.setScope} page={detail.galleries} /></main>;
}

function GalleryResults({ title, scope, onScope, page }: { title: string; scope: Scope; onScope: (scope: Scope) => void; page: GalleryPage }) {
  return <section className="entity-galleries"><header className="section-heading"><h2>{title}</h2><span>{page.totalItems}</span></header><ScopeSelector value={scope} onChange={onScope} />
    <div className="gallery-grid">{page.items.map((card) => <GalleryCard key={card.setID} card={card} scrubberEnabled={false} />)}</div></section>;
}
function TextDetailHeading({ name, scope }: { name: string; scope: Scope }) { return <header className="page-heading"><p>{scope}</p><h1>{name}</h1></header>; }
function Loading() { const intl = useIntl(); return <main className="browse-main"><p className="state-message">{intl.formatMessage({ id: "state.loading" })}</p></main>; }
function ErrorState() { const intl = useIntl(); return <main className="browse-main"><p className="state-message" role="alert">{intl.formatMessage({ id: "state.error" })}</p></main>; }
function useCanonicalDetail(detail: { entity: { slug: string }; redirected: boolean } | undefined, slug: string, kind: string, navigate: ReturnType<typeof useNavigate>) {
  useEffect(() => { if (detail?.redirected && detail.entity.slug !== slug) navigate(`/${kind}/${detail.entity.slug}`, { replace: true }); }, [detail, kind, navigate, slug]);
}
