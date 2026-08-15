import { useQuery } from "@apollo/client/react";
import { useEffect, type ReactNode } from "react";
import { useIntl } from "react-intl";
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { CHARACTER_DETAIL, COSER_DETAIL, TAG_DETAIL, WORK_DETAIL } from "../api/browse";
import { Icon } from "../ui/Icon";
import { SocialAccounts } from "./SocialAccounts";
import { Breadcrumbs, Pagination } from "../ui/Patterns";
import { GalleryCard } from "./GalleryCard";
import { ScopeSelector } from "./ScopeSelector";
import { TimelinePage } from "./TimelinePage";
import type { CharacterDetail, CollectionType, CoserDetail, GalleryPage, Scope, TagDetail, WorkDetail } from "./types";

function useDetailRoute() {
  const { slug = "" } = useParams(); const navigate = useNavigate(); const [parameters, setParameters] = useSearchParams();
  const scope = (parameters.get("scope") as Scope) || "ALL"; const page = Math.max(1, Number(parameters.get("page")) || 1);
  return {
    slug, navigate, scope, page,
    setScope: (next: Scope) => setParameters({ scope: next, page: "1" }),
    setPage: (next: number) => setParameters({ scope, page: String(next) }),
  };
}

export function CoserDetailPage() {
  return <PersonDetailPage collectionType="COSPLAY" routeKind="coser" indexPath="/cosers" indexTitleID="nav.cosers" profileVisible />;
}

export function ModelDetailPage() {
  return <PersonDetailPage collectionType="ALBUM" routeKind="model" indexPath="/models" indexTitleID="nav.models" />;
}

function PersonDetailPage({ collectionType, routeKind, indexPath, indexTitleID, profileVisible = false }: {
  collectionType: CollectionType; routeKind: "coser" | "model"; indexPath: string; indexTitleID: string; profileVisible?: boolean;
}) {
  const intl = useIntl(); const route = useDetailRoute();
  const effectiveScope = collectionType === "ALBUM" ? "ALL" : route.scope;
  const query = useQuery<{ coserDetail: CoserDetail }>(COSER_DETAIL, { variables: { slug: route.slug, scope: effectiveScope, page: route.page, collectionType } });
  const detail = query.data?.coserDetail;
  useCanonicalDetail(detail, route.slug, routeKind, route.navigate);
  if (query.loading) return <Loading />; if (query.error || !detail) return <ErrorState />;
  return <main className={`browse-main ${profileVisible ? "coser-detail" : "model-detail"}`}>
    <Breadcrumbs>
      <li><Link to="/">{intl.formatMessage({ id: "nav.home" })}</Link></li>
      <li><Link to={indexPath}>{intl.formatMessage({ id: indexTitleID })}</Link></li>
      <li aria-current="page">{detail.entity.name}</li>
    </Breadcrumbs>
    {profileVisible ? <section className="coser-profile">
      <div className={`coser-banner${detail.bannerURL ? "" : " coser-banner--empty"}`}>{detail.bannerURL ? <img src={detail.bannerURL} alt="" /> : null}</div>
      <div className="coser-hero">
        <Link
          className="coser-avatar"
          to={`/manage/cosers?uuid=${encodeURIComponent(detail.entity.uuid)}`}
          aria-label={intl.formatMessage({ id: "coser.manage" }, { name: detail.entity.name })}
          title={intl.formatMessage({ id: "coser.manage" }, { name: detail.entity.name })}
        >
          {detail.entity.avatarURL ? <img src={detail.entity.avatarURL} alt="" /> : detail.entity.name.slice(0, 1)}
        </Link>
        <div className="coser-identity">
          <div className="coser-identity__title"><h1>{detail.entity.name}</h1>{detail.countryOrRegion ? <span>{detail.countryOrRegion}</span> : null}</div>
          <SocialAccounts accounts={detail.socialAccounts} />
        </div>
      </div>
    </section> : <h1 className="sr-only">{detail.entity.name}</h1>}
    {profileVisible && (detail.profileSummary || detail.biography) ? <section className="coser-supporting">
      {detail.profileSummary ? <details><summary>{intl.formatMessage({ id: "coser.profile" })}</summary><p>{detail.profileSummary}</p></details> : null}
      {detail.biography ? <details><summary>{intl.formatMessage({ id: "coser.biography" })}</summary><p>{detail.biography}</p></details> : null}
    </section> : null}
    <GalleryResults
      className="coser-gallery-results"
      hideHeading
      title={intl.formatMessage({ id: "coser.galleries" })}
      scope={effectiveScope}
      onScope={profileVisible ? route.setScope : undefined}
      onPage={route.setPage}
      page={detail.galleries}
      toolbarAction={profileVisible ? <Link className="coser-timeline-link" to={`/coser/${detail.entity.slug}/timeline`}><span>{intl.formatMessage({ id: "coser.timeline" })}</span><Icon name="chevron-right" /></Link> : undefined}
      compactCards={profileVisible}
    />
  </main>;
}

export function CoserTimelinePage() {
  const { slug = "" } = useParams(); const query = useQuery<{ coserDetail: CoserDetail }>(COSER_DETAIL, { variables: { slug, scope: "ALL", page: 1, collectionType: "COSPLAY" } });
  if (!query.data) return <Loading />;
  return <TimelinePage coserUUID={query.data.coserDetail.entity.uuid} />;
}

export function WorkDetailPage() {
  const intl = useIntl();
  const route = useDetailRoute(); const query = useQuery<{ workDetail: WorkDetail }>(WORK_DETAIL, { variables: { slug: route.slug, scope: route.scope } }); const detail = query.data?.workDetail;
  useCanonicalDetail(detail, route.slug, "work", route.navigate); if (query.loading) return <Loading />; if (!detail || query.error) return <ErrorState />;
  return <main className="browse-main entity-detail-index"><Breadcrumbs><li><Link to="/">{intl.formatMessage({ id: "nav.home" })}</Link></li><li><Link to="/works">{intl.formatMessage({ id: "nav.parodies" })}</Link></li><li aria-current="page">{detail.entity.name}</li></Breadcrumbs><h1 className="sr-only">{detail.entity.name}</h1><ScopeSelector value={route.scope} onChange={route.setScope} />
    <section className="character-text-index">{detail.characters.map((character) => <Link key={character.uuid} to={`/character/${character.slug}`}>{character.name}</Link>)}</section></main>;
}

export function CharacterDetailPage() {
  const intl = useIntl(); const route = useDetailRoute(); const result = useQuery<{ characterDetail: CharacterDetail }>(CHARACTER_DETAIL, { variables: { slug: route.slug, scope: route.scope, page: route.page } }); const detail = result.data?.characterDetail;
  useCanonicalDetail(detail, route.slug, "character", route.navigate); if (result.loading) return <Loading />; if (!detail || result.error) return <ErrorState />;
  return <main className="browse-main character-detail"><Breadcrumbs><li><Link to="/">{intl.formatMessage({ id: "nav.home" })}</Link></li><li><Link to="/works">{intl.formatMessage({ id: "nav.parodies" })}</Link></li><li><Link to={`/work/${detail.work.slug}`}>{detail.work.name}</Link></li><li aria-current="page">{detail.entity.name}</li></Breadcrumbs><h1 className="sr-only">{detail.entity.name}</h1>
    <GalleryResults title={detail.entity.name} hideHeading scope={route.scope} onScope={route.setScope} onPage={route.setPage} page={detail.galleries} /></main>;
}
export function TagDetailPage() { return <GalleryEntityDetail />; }

function GalleryEntityDetail() {
  const route = useDetailRoute(); const result = useQuery<{ tagDetail: TagDetail }>(TAG_DETAIL, { variables: { slug: route.slug, scope: route.scope, page: route.page } });
  const detail = result.data?.tagDetail;
  useCanonicalDetail(detail, route.slug, "tag", route.navigate); if (result.loading) return <Loading />; if (!detail || result.error) return <ErrorState />;
  return <main className="browse-main"><TextDetailHeading scope={route.scope} name={detail.entity.name} />
    <GalleryResults title={detail.entity.name} scope={route.scope} onScope={route.setScope} page={detail.galleries} /></main>;
}

function GalleryResults({ title, scope, onScope, onPage, page, className = "", hideHeading = false, toolbarAction, compactCards = false }: {
  title: string; scope: Scope; onScope?: (scope: Scope) => void; onPage?: (page: number) => void; page: GalleryPage;
  className?: string; hideHeading?: boolean; toolbarAction?: ReactNode; compactCards?: boolean;
}) {
  const intl = useIntl();
  return <section className={`entity-galleries ${className}`.trim()}>
    <header className={hideHeading ? "sr-only" : "section-heading"}><h2>{title}</h2><span>{page.totalItems}</span></header>
    {onScope || toolbarAction ? <div className={toolbarAction ? "entity-gallery-controls" : "entity-gallery-scope"}>{onScope ? <ScopeSelector value={scope} onChange={onScope} /> : null}{toolbarAction}</div> : null}
    <div className="gallery-grid">{page.items.map((card) => <GalleryCard key={card.setID} card={card} scrubberEnabled={false} peopleVisible={!compactCards} ratingSummaryVisible={!compactCards} />)}</div>
    {onPage && page.totalPages > 1 ? <Pagination page={page.page} totalPages={page.totalPages} previousLabel={intl.formatMessage({ id: "pagination.previous" })} nextLabel={intl.formatMessage({ id: "pagination.next" })} onPageChange={onPage} /> : null}
  </section>;
}
function TextDetailHeading({ name, scope }: { name: string; scope: Scope }) { return <header className="page-heading"><p>{scope}</p><h1>{name}</h1></header>; }
function Loading() { const intl = useIntl(); return <main className="browse-main"><p className="state-message">{intl.formatMessage({ id: "state.loading" })}</p></main>; }
function ErrorState() { const intl = useIntl(); return <main className="browse-main"><p className="state-message" role="alert">{intl.formatMessage({ id: "state.error" })}</p></main>; }
function useCanonicalDetail(detail: { entity: { slug: string }; redirected: boolean } | undefined, slug: string, kind: string, navigate: ReturnType<typeof useNavigate>) {
  useEffect(() => { if (detail?.redirected && detail.entity.slug !== slug) navigate(`/${kind}/${detail.entity.slug}`, { replace: true }); }, [detail, kind, navigate, slug]);
}
