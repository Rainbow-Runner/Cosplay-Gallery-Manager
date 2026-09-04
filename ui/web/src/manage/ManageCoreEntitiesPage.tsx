import { useMutation, useQuery } from "@apollo/client/react";
import { type FormEvent, type KeyboardEvent, type ReactNode, useEffect, useId, useRef, useState } from "react";
import { useIntl } from "react-intl";
import { Link, useSearchParams } from "react-router-dom";
import { ADD_COSER_SOCIAL_ACCOUNT, CREATE_CORE_ENTITY, MANAGE_CORE_ENTITIES, MANAGE_CORE_ENTITY, MANAGE_CORE_ENTITY_NAME_CONFLICTS, MANAGE_COSER_MANIFEST, MANAGE_COSER_NAME_CONFLICTS, MANAGE_WORK_CHARACTERS, PULL_COSER_MANIFEST, PUSH_COSER_MANIFEST, REPLACE_TAG_PARENTS, RESOLVE_COSER_MANIFEST, UPDATE_CORE_ENTITY } from "../api/manage";
import { Icon } from "../ui/Icon";
import { ManageEntitySelector } from "./ManageEntitySelector";
import { ManageEntityLifecyclePanel } from "./ManageEntityLifecyclePanel";
import { ManageCoserAssetsPanel } from "./ManageCoserAssetsPanel";
import { ManageCoserMetadataImport } from "./ManageCoserMetadataImport";
import { ManageEntityMetadataImport } from "./ManageEntityMetadataImport";
import type { ManageCoreEntity, ManageCoreEntityNameConflict, ManageCoreEntityPage, ManageCoserManifestState, ManageCoserNameConflict } from "./types";

type Kind = ManageCoreEntity["kind"];
type CoserAssetFilter = "ALL" | "MISSING_AVATAR" | "MISSING_BANNER" | "INCOMPLETE" | "COMPLETE";
const kinds: Kind[] = ["WORK", "CHARACTER", "TAG"];
const coserAssetFilters: CoserAssetFilter[] = ["ALL", "MISSING_AVATAR", "MISSING_BANNER", "INCOMPLETE", "COMPLETE"];
const entityPageSizes = [30, 60, 100] as const;
const blank = (kind: Kind): ManageCoreEntity => ({ kind, uuid: "", name: "", sortName: "", aliases: [], slug: "", metadataRevision: 0, workName: "", profileSummary: "", biography: "", countryOrRegion: "", useInRecommendation: true, socialAccounts: [], parents: [] });
export const parseAliases = (value: string) => value.split("/").map((alias) => alias.trim()).filter(Boolean);
export const socialPlatformOptions = [
  ["instagram", "Instagram"], ["twitter", "Twitter / X"], ["weibo", "Weibo"],
  ["bilibili", "Bilibili"], ["xiaohongshu", "Xiaohongshu"], ["douyin", "Douyin"],
  ["tiktok", "TikTok"], ["youtube", "YouTube"], ["pixiv", "Pixiv"],
  ["facebook", "Facebook"], ["bluesky", "Bluesky"], ["patreon", "Patreon"],
  ["fanbox", "FANBOX"], ["website", "Website"],
] as const;

function AliasChipInput({ aliases, onChange, draft, setDraft }: { aliases: string[]; onChange: (value: string[]) => void; draft: string; setDraft: (value: string) => void }) {
  const intl = useIntl();
  const inputID = useId();
  const helpID = `${inputID}-help`;
  const inputRef = useRef<HTMLInputElement>(null);
  const [error, setError] = useState("");
  const hasDraft = draft.trim() !== "";
  const normalized = (text: string) => text.trim().normalize("NFC").toLocaleLowerCase();
  function replaceAliases(next: string[]) { onChange(next); setError(""); }
  function commit(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key !== "Enter" || event.nativeEvent.isComposing) return;
    event.preventDefault();
    const alias = draft.trim();
    if (!alias) return;
    if (aliases.some((value) => normalized(value) === normalized(alias))) {
      setError(intl.formatMessage({ id: "manage.aliasChips.duplicate" }));
      return;
    }
    if (aliases.length >= 100) {
      setError(intl.formatMessage({ id: "manage.aliasChips.limit" }));
      return;
    }
    replaceAliases([...aliases, alias]);
    setDraft("");
  }
  function edit(alias: string, index: number) {
    const remaining = aliases.filter((_, position) => position !== index);
    const pending = draft.trim();
    if (pending && normalized(pending) === normalized(alias)) {
      replaceAliases(remaining);
      setDraft(pending);
      inputRef.current?.focus();
      return;
    }
    if (pending && remaining.some((value) => normalized(value) === normalized(pending))) {
      setError(intl.formatMessage({ id: "manage.aliasChips.duplicate" }));
      inputRef.current?.focus();
      return;
    }
    replaceAliases(pending ? [...remaining, pending] : remaining);
    setDraft(alias);
    inputRef.current?.focus();
  }
  return <div className="span-2 alias-chip-field">
    <label htmlFor={inputID}>{intl.formatMessage({ id: "manage.aliasChips.label" })}</label>
    <div className="alias-chip-input" role="group" aria-describedby={helpID}>
      {aliases.map((alias, index) => <span className="alias-chip" key={`${alias}-${index}`}>
        <button type="button" className="alias-chip__text" aria-label={intl.formatMessage({ id: "manage.aliasChips.edit" }, { alias })} onClick={() => edit(alias, index)}>{alias}</button>
        <button type="button" className="alias-chip__remove" aria-label={intl.formatMessage({ id: "manage.aliasChips.remove" }, { alias })} onClick={() => replaceAliases(aliases.filter((_, position) => position !== index))}>×</button>
      </span>)}
      <input ref={inputRef} id={inputID} maxLength={300} value={draft} placeholder={aliases.length ? "" : intl.formatMessage({ id: "manage.aliasChips.placeholder" })} onChange={(event) => { setDraft(event.target.value); setError(""); }} onKeyDown={commit} />
    </div>
    <small id={helpID} className={error ? "manage-error" : ""} role={error ? "alert" : undefined}>{error || (hasDraft ? intl.formatMessage({ id: "manage.aliasChips.pending" }) : intl.formatMessage({ id: "manage.aliasChips.help" }))}</small>
  </div>;
}

function ManageCoserListItem({ item }: { item: ManageCoreEntity }) {
  const intl = useIntl();
  const [avatarFailed, setAvatarFailed] = useState(false);
  const avatarState = avatarFailed ? "unavailable" : item.avatarURL ? "set" : "missing";
  const avatarLabel = intl.formatMessage({ id: `manage.coserList.avatar.${avatarState}` }, { name: item.name });
  const bannerLabel = intl.formatMessage({ id: item.bannerURL ? "manage.coserList.banner.set" : "manage.coserList.banner.missing" }, { name: item.name });
  return <>
    <span className={`entity-manage-list__avatar${avatarFailed ? " is-error" : ""}`} aria-hidden="true">
      {item.avatarURL && !avatarFailed ? <img src={item.avatarURL} alt="" width="40" height="40" loading="lazy" decoding="async" onError={() => setAvatarFailed(true)} /> : null}
    </span>
    <span className="entity-manage-list__copy"><strong>{item.name}</strong><span>{item.aliases.join(" / ") || item.uuid}</span></span>
    <span className="entity-manage-list__asset-status" aria-label={intl.formatMessage({ id: "manage.coserList.assetStatus" }, { name: item.name })}>
      <span data-asset="avatar" className={`entity-manage-list__asset${avatarState === "set" ? " is-set" : avatarState === "unavailable" ? " is-error" : ""}`} aria-label={avatarLabel} title={avatarLabel}><Icon name="user" /></span>
      <span data-asset="banner" className={`entity-manage-list__asset${item.bannerURL ? " is-set" : ""}`} aria-label={bannerLabel} title={bannerLabel}><Icon name="gallery" /></span>
    </span>
  </>;
}

function CoserDuplicateReview({ name, checkedName, loading, error, conflicts, confirmed, setConfirmed, openExisting }: {
  name: string; checkedName: string; loading: boolean; error: boolean; conflicts: ManageCoserNameConflict[];
  confirmed: boolean; setConfirmed: (value: boolean) => void; openExisting: (coser: ManageCoreEntity) => void;
}) {
  const intl = useIntl();
  if (!name) return null;
  const checking = checkedName !== name || loading;
  return <section className={`coser-duplicate-review${conflicts.length ? " has-conflicts" : ""}`} aria-live="polite">
    <h4>{intl.formatMessage({ id: "manage.coserDuplicate.heading" })}</h4>
    {checking ? <p>{intl.formatMessage({ id: "manage.coserDuplicate.checking" })}</p> : error ? <>
      <p className="manage-error">{intl.formatMessage({ id: "manage.coserDuplicate.failed" })}</p>
      <label className="check"><input type="checkbox" checked={confirmed} onChange={(event) => setConfirmed(event.target.checked)} /> {intl.formatMessage({ id: "manage.coserDuplicate.continueAfterFailure" })}</label>
    </> : conflicts.length ? <>
      <p>{intl.formatMessage({ id: "manage.coserDuplicate.matches" }, { count: conflicts.length })}</p>
      <div className="coser-duplicate-review__list">{conflicts.map((conflict) => <article key={conflict.coser.uuid}>
        <span className="coser-duplicate-review__avatar" aria-hidden="true">{conflict.coser.avatarURL ? <img src={conflict.coser.avatarURL} alt="" width="40" height="40" loading="lazy" decoding="async" /> : null}</span>
        <div><strong>{conflict.coser.name}</strong><small>{conflict.coser.aliases.join(" / ") || intl.formatMessage({ id: "manage.coserDuplicate.noAliases" })}</small><small>{intl.formatMessage({ id: "manage.coserDuplicate.matchReason" }, { values: conflict.matchedValues.join(" / "), uuid: conflict.coser.uuid.slice(-8), count: conflict.galleryCount })}</small></div>
        <button type="button" onClick={() => openExisting(conflict.coser)}>{intl.formatMessage({ id: "manage.coserDuplicate.open" })}</button>
      </article>)}</div>
      <label className="check"><input type="checkbox" checked={confirmed} onChange={(event) => setConfirmed(event.target.checked)} /> {intl.formatMessage({ id: "manage.coserDuplicate.confirmDifferent" })}</label>
    </> : <p className="coser-duplicate-review__clear">{intl.formatMessage({ id: "manage.coserDuplicate.clear" })}</p>}
  </section>;
}

function CoreEntityDuplicateReview({ kind, name, checkedName, loading, error, conflicts, sameWorkBlocked, confirmed, setConfirmed, openExisting }: {
  kind: "WORK" | "CHARACTER"; name: string; checkedName: string; loading: boolean; error: boolean; conflicts: ManageCoreEntityNameConflict[];
  sameWorkBlocked: boolean; confirmed: boolean; setConfirmed: (value: boolean) => void; openExisting: (entity: ManageCoreEntity) => void;
}) {
  const intl = useIntl();
  if (!name) return null;
  const checking = checkedName !== name || loading;
  const kindLabel = intl.formatMessage({ id: `manage.entityDuplicate.kind.${kind.toLowerCase()}` });
  return <section className={`coser-duplicate-review${conflicts.length ? " has-conflicts" : ""}`} aria-live="polite">
    <h4>{intl.formatMessage({ id: "manage.entityDuplicate.heading" }, { kind: kindLabel })}</h4>
    {checking ? <p>{intl.formatMessage({ id: "manage.entityDuplicate.checking" })}</p> : error ? <>
      <p className="manage-error">{intl.formatMessage({ id: "manage.entityDuplicate.failed" })}</p>
      <label className="check"><input type="checkbox" checked={confirmed} onChange={(event) => setConfirmed(event.target.checked)} /> {intl.formatMessage({ id: "manage.entityDuplicate.continueAfterFailure" }, { kind: kindLabel })}</label>
    </> : conflicts.length ? <>
      <p>{intl.formatMessage({ id: "manage.entityDuplicate.matches" }, { count: conflicts.length, kind: kindLabel })}</p>
      <div className="coser-duplicate-review__list">{conflicts.map((conflict) => <article key={conflict.entity.uuid}>
        <span className="coser-duplicate-review__avatar" aria-hidden="true"><Icon name={kind === "WORK" ? "book" : "character"} /></span>
        <div><strong>{conflict.entity.name}</strong><small>{conflict.entity.aliases.join(" / ") || intl.formatMessage({ id: "manage.entityDuplicate.noAliases" })}</small>{kind === "CHARACTER" ? <small>{intl.formatMessage({ id: "manage.entityDuplicate.work" }, { work: conflict.workName })}</small> : null}<small>{intl.formatMessage({ id: "manage.entityDuplicate.matchReason" }, { values: conflict.matchedValues.join(" / "), uuid: conflict.entity.uuid.slice(-8), count: conflict.galleryCount })}</small></div>
        <button type="button" onClick={() => openExisting(conflict.entity)}>{intl.formatMessage({ id: "manage.entityDuplicate.open" })}</button>
      </article>)}</div>
      {sameWorkBlocked ? <p className="manage-error">{intl.formatMessage({ id: "manage.entityDuplicate.sameWorkBlocked" })}</p> : <label className="check"><input type="checkbox" checked={confirmed} onChange={(event) => setConfirmed(event.target.checked)} /> {intl.formatMessage({ id: "manage.entityDuplicate.confirmDifferent" }, { kind: kindLabel })}</label>}
    </> : <p className="coser-duplicate-review__clear">{intl.formatMessage({ id: "manage.entityDuplicate.clear" })}</p>}
  </section>;
}

function WorkCharactersEditor({ work, openStandalone }: { work: ManageCoreEntity; openStandalone: (entity: ManageCoreEntity) => void }) {
  const intl = useIntl();
  const newCharacter = () => ({ ...blank("CHARACTER"), workUUID: work.uuid, workName: work.name });
  const [character, setCharacter] = useState<ManageCoreEntity>(newCharacter);
  const [aliases, setAliases] = useState<string[]>([]);
  const [aliasDraft, setAliasDraft] = useState("");
  const [checkedName, setCheckedName] = useState("");
  const [duplicateConfirmed, setDuplicateConfirmed] = useState(false);
  const [message, setMessage] = useState("");
  const charactersQuery = useQuery<{ manageWorkCharacters: ManageCoreEntity[] }>(MANAGE_WORK_CHARACTERS, { variables: { workUUID: work.uuid }, fetchPolicy: "network-only" });
  const duplicateQuery = useQuery<{ manageCoreEntityNameConflicts: ManageCoreEntityNameConflict[] }>(MANAGE_CORE_ENTITY_NAME_CONFLICTS, {
    variables: { kind: "CHARACTER", name: checkedName || "_", limit: 10 }, skip: character.uuid !== "" || checkedName === "", fetchPolicy: "network-only",
  });
  const [create, createState] = useMutation<{ createCoreEntity: ManageCoreEntity }>(CREATE_CORE_ENTITY);
  const [update, updateState] = useMutation<{ updateCoreEntity: ManageCoreEntity }>(UPDATE_CORE_ENTITY);
  const items = charactersQuery.data?.manageWorkCharacters || [];
  const enteredName = character.uuid ? "" : character.name.trim();

  useEffect(() => {
    setCharacter({ ...blank("CHARACTER"), workUUID: work.uuid, workName: work.name });
    setAliases([]); setAliasDraft(""); setCheckedName(""); setDuplicateConfirmed(false); setMessage("");
  }, [work.uuid, work.name]);
  useEffect(() => { setAliases([...character.aliases]); setAliasDraft(""); }, [character.uuid, character.metadataRevision]);
  useEffect(() => {
    setDuplicateConfirmed(false);
    if (!enteredName) { setCheckedName(""); return; }
    const timeout = window.setTimeout(() => setCheckedName(enteredName), 350);
    return () => window.clearTimeout(timeout);
  }, [enteredName, work.uuid]);

  const conflictsCurrent = checkedName === enteredName && !duplicateQuery.loading;
  const conflicts = conflictsCurrent ? duplicateQuery.data?.manageCoreEntityNameConflicts || [] : [];
  const sameWorkBlocked = conflicts.some((conflict) => conflict.primaryNameMatch && conflict.entity.workUUID === work.uuid);
  const duplicateReady = !enteredName || (conflictsCurrent && !duplicateQuery.error && conflicts.length === 0) ||
    (conflictsCurrent && !duplicateQuery.error && conflicts.length > 0 && !sameWorkBlocked && duplicateConfirmed) ||
    (conflictsCurrent && Boolean(duplicateQuery.error) && duplicateConfirmed);
  const valid = character.name.trim() !== "" && !aliasDraft.trim() && duplicateReady;
  const saved = items.find((item) => item.uuid === character.uuid);
  const dirty = Boolean(saved && (saved.name !== character.name || saved.sortName !== character.sortName || JSON.stringify(saved.aliases) !== JSON.stringify(aliases) || aliasDraft.trim()));

  function edit(value: ManageCoreEntity) {
    setCharacter(value); setAliases([...value.aliases]); setAliasDraft(""); setDuplicateConfirmed(false); setMessage("");
  }
  function reset() {
    setCharacter(newCharacter()); setAliases([]); setAliasDraft(""); setDuplicateConfirmed(false); setMessage("");
  }
  async function save(event: FormEvent) {
    event.preventDefault();
    if (!valid) return;
    setMessage("");
    const input = { kind: "CHARACTER", name: character.name, sortName: character.sortName, aliases, workUUID: work.uuid, profileSummary: "", biography: "", countryOrRegion: "", useInRecommendation: true };
    try {
      let savedCharacter: ManageCoreEntity | undefined;
      if (character.uuid) {
        const result = await update({ variables: { uuid: character.uuid, expectedMetadataRevision: character.metadataRevision, input } });
        savedCharacter = result.data?.updateCoreEntity;
      } else {
        const result = await create({ variables: { input } });
        savedCharacter = result.data?.createCoreEntity;
      }
      if (!savedCharacter) throw new Error("Server did not return the saved Character");
      setCharacter(savedCharacter);
      await charactersQuery.refetch();
      setMessage(intl.formatMessage({ id: "manage.workCharacters.saved" }));
    } catch (error) {
      setMessage(error instanceof Error ? error.message : intl.formatMessage({ id: "manage.workCharacters.saveFailed" }));
    }
  }
  async function refreshSelected() {
    const result = await charactersQuery.refetch();
    const refreshed = result.data?.manageWorkCharacters.find((item) => item.uuid === character.uuid);
    if (refreshed) setCharacter(refreshed);
  }

  return <section className="manage-panel work-character-editor">
    <header><div><p>{intl.formatMessage({ id: "manage.workCharacters.context" })}</p><h3>{work.name}</h3></div><button type="button" onClick={reset}>＋ {intl.formatMessage({ id: "manage.workCharacters.new" })}</button></header>
    <div className="work-character-editor__collection">
      <div><strong>{intl.formatMessage({ id: "manage.workCharacters.associated" })}</strong><span>{intl.formatMessage({ id: "manage.workCharacters.count" }, { count: items.length })}</span></div>
      {charactersQuery.loading && !charactersQuery.data ? <p>{intl.formatMessage({ id: "manage.workCharacters.loading" })}</p> : charactersQuery.error ? <p className="manage-error">{intl.formatMessage({ id: "manage.workCharacters.loadFailed" })}</p> : items.length ? <ul className="work-character-editor__chips" aria-label={intl.formatMessage({ id: "manage.workCharacters.associated" })}>{items.map((item) => <li key={item.uuid}><button type="button" className={`work-character-chip${character.uuid === item.uuid ? " is-active" : ""}`} onClick={() => edit(item)}>{item.name}</button></li>)}</ul> : <p>{intl.formatMessage({ id: "manage.workCharacters.empty" })}</p>}
    </div>
    {message ? <p className="manage-message">{message}</p> : null}
    <EntityForm draft={character} aliasText="" setAliasText={() => {}} aliasChips={aliases} setAliasChips={setAliases} aliasChipDraft={aliasDraft} setAliasChipDraft={setAliasDraft} setDraft={setCharacter} submit={save} saving={createState.loading || updateState.loading} valid={valid} fixedCharacterWorkName={work.name} nameReview={enteredName ? <CoreEntityDuplicateReview kind="CHARACTER" name={enteredName} checkedName={checkedName} loading={duplicateQuery.loading} error={Boolean(duplicateQuery.error)} conflicts={conflicts} sameWorkBlocked={sameWorkBlocked} confirmed={duplicateConfirmed} setConfirmed={setDuplicateConfirmed} openExisting={(entity) => entity.workUUID === work.uuid ? edit(entity) : openStandalone(entity)} /> : null} />
    {character.uuid ? <><ManageEntityMetadataImport entity={character} editorDirty={dirty} onUpdated={refreshSelected} /><ManageEntityLifecyclePanel source={character} onMerged={async (target) => { await charactersQuery.refetch(); if (target.workUUID === work.uuid) edit(target); else openStandalone(target); }} onDeleted={async () => { await charactersQuery.refetch(); reset(); }} /></> : null}
  </section>;
}

function ManageEntityPagination({ data, kindLabel, onPage }: { data: ManageCoreEntityPage; kindLabel: string; onPage: (page: number) => void }) {
  const intl = useIntl();
  const [jumpPage, setJumpPage] = useState(String(data.page));
  useEffect(() => setJumpPage(String(data.page)), [data.page]);
  const firstItem = data.totalItems === 0 ? 0 : (data.page - 1) * data.pageSize + 1;
  const lastItem = Math.min(data.totalItems, data.page * data.pageSize);
  function jump(event: FormEvent) {
    event.preventDefault();
    const requested = Math.floor(Number(jumpPage));
    const target = Number.isFinite(requested) ? Math.min(Math.max(requested, 1), Math.max(data.totalPages, 1)) : data.page;
    setJumpPage(String(target));
    onPage(target);
  }
  return <nav className="manage-pagination entity-list-pagination" aria-label={intl.formatMessage({ id: "manage.entityList.pagination" }, { kind: kindLabel })}>
    <span className="entity-list-pagination__range">{intl.formatMessage({ id: "manage.entityList.range" }, { first: firstItem, last: lastItem, total: data.totalItems })}</span>
    <div>
      <button type="button" disabled={data.page <= 1} aria-label={intl.formatMessage({ id: "manage.entityList.firstPage" })} onClick={() => onPage(1)}>«</button>
      <button type="button" disabled={data.page <= 1} aria-label={intl.formatMessage({ id: "pagination.previous" })} onClick={() => onPage(data.page - 1)}>‹</button>
      <form onSubmit={jump}>
        <label>{intl.formatMessage({ id: "manage.entityList.page" })}<input type="number" min="1" max={Math.max(data.totalPages, 1)} value={jumpPage} onChange={(event) => setJumpPage(event.target.value)} /></label>
        <span>/ {Math.max(data.totalPages, 1)}</span>
      </form>
      <button type="button" disabled={data.page >= data.totalPages} aria-label={intl.formatMessage({ id: "pagination.next" })} onClick={() => onPage(data.page + 1)}>›</button>
      <button type="button" disabled={data.page >= data.totalPages} aria-label={intl.formatMessage({ id: "manage.entityList.lastPage" })} onClick={() => onPage(data.totalPages)}>»</button>
    </div>
  </nav>;
}

function isAbsoluteHTTPURL(value: string) {
  try {
    const parsed = new URL(value);
    return (parsed.protocol === "http:" || parsed.protocol === "https:") &&
      parsed.host !== "" && parsed.username === "" && parsed.password === "";
  } catch {
    return false;
  }
}

export function ManageCoreEntitiesPage({ coserOnly = false }: { coserOnly?: boolean }) {
  const intl = useIntl();
  const [parameters, setParameters] = useSearchParams(); const kind: Kind = coserOnly ? "COSER" : kinds.includes(parameters.get("kind") as Kind) ? parameters.get("kind") as Kind : "WORK"; const page = Math.max(1, Number(parameters.get("page")) || 1); const selected = parameters.get("uuid") || "";
  const query = (parameters.get("q") || "").trim();
  const requestedPageSize = Number(parameters.get("pageSize"));
  const defaultPageSize = coserOnly ? 30 : 60;
  const pageSize = entityPageSizes.includes(requestedPageSize as typeof entityPageSizes[number]) ? requestedPageSize : defaultPageSize;
  const requestedAssetFilter = parameters.get("assets") as CoserAssetFilter;
  const assetFilter: CoserAssetFilter = coserOnly && coserAssetFilters.includes(requestedAssetFilter) ? requestedAssetFilter : "ALL";
  const [listSearch, setListSearch] = useState(query);
  const [draft, setDraft] = useState<ManageCoreEntity>(blank(kind)); const [aliasText, setAliasText] = useState(""); const [aliasChips, setAliasChips] = useState<string[]>([]); const [aliasChipDraft, setAliasChipDraft] = useState(""); const [message, setMessage] = useState(""); const [coserTab, setCoserTab] = useState("profile"); const [workTab, setWorkTab] = useState<"details" | "characters">("details");
  const [checkedEntityName, setCheckedEntityName] = useState(""); const [duplicateConfirmed, setDuplicateConfirmed] = useState(false);
  const [account, setAccount] = useState({ platformKey: "", label: "", handle: "", url: "https://", status: "ACTIVE", visible: true, position: "1024" });
  const [conflictChoices, setConflictChoices] = useState<Record<string, "DATABASE" | "FILE">>({});
  const kindLabel = intl.formatMessage({ id: `manage.entityList.kind.${kind.toLowerCase()}` });
  const listQuery = useQuery<{ manageCoreEntities: ManageCoreEntityPage }>(MANAGE_CORE_ENTITIES, { variables: { kind, page, pageSize, query, coserAssetFilter: assetFilter } }); const detailQuery = useQuery<{ manageCoreEntity: ManageCoreEntity }>(MANAGE_CORE_ENTITY, { variables: { kind, uuid: selected }, skip: !selected, fetchPolicy: "network-only" });
  const [create, createState] = useMutation<{ createCoreEntity: ManageCoreEntity }>(CREATE_CORE_ENTITY); const [update, updateState] = useMutation<{ updateCoreEntity: ManageCoreEntity }>(UPDATE_CORE_ENTITY); const [addSocial, addSocialState] = useMutation<{ addCoserSocialAccount: ManageCoreEntity }>(ADD_COSER_SOCIAL_ACCOUNT);
  const [replaceTagParents, replaceTagParentsState] = useMutation<{ replaceTagParents: ManageCoreEntity }>(REPLACE_TAG_PARENTS);
  const duplicateReviewKind = !draft.uuid && (draft.kind === "COSER" || draft.kind === "WORK" || draft.kind === "CHARACTER") ? draft.kind : null;
  const newEntityName = duplicateReviewKind ? draft.name.trim() : "";
  const coserDuplicateQuery = useQuery<{ manageCoserNameConflicts: ManageCoserNameConflict[] }>(MANAGE_COSER_NAME_CONFLICTS, { variables: { name: checkedEntityName || "_", limit: 10 }, skip: checkedEntityName === "" || duplicateReviewKind !== "COSER", fetchPolicy: "network-only" });
  const coreEntityDuplicateQuery = useQuery<{ manageCoreEntityNameConflicts: ManageCoreEntityNameConflict[] }>(MANAGE_CORE_ENTITY_NAME_CONFLICTS, { variables: { kind: duplicateReviewKind === "CHARACTER" ? "CHARACTER" : "WORK", name: checkedEntityName || "_", limit: 10 }, skip: checkedEntityName === "" || (duplicateReviewKind !== "WORK" && duplicateReviewKind !== "CHARACTER"), fetchPolicy: "network-only" });
  const manifestQuery = useQuery<{ manageCoserManifest: ManageCoserManifestState }>(MANAGE_COSER_MANIFEST, { variables: { coserUUID: draft?.uuid || selected }, skip: kind !== "COSER" || coserTab !== "manifest" || !(draft?.uuid || selected), fetchPolicy: "network-only" });
  const [pushManifest, pushManifestState] = useMutation(PUSH_COSER_MANIFEST); const [pullManifest, pullManifestState] = useMutation(PULL_COSER_MANIFEST); const [resolveManifest, resolveManifestState] = useMutation(RESOLVE_COSER_MANIFEST);
  useEffect(() => { setDraft(detailQuery.data?.manageCoreEntity ?? blank(kind)); }, [detailQuery.data, kind, selected]);
  useEffect(() => { setAliasText(draft.aliases.join(" / ")); setAliasChips([...draft.aliases]); setAliasChipDraft(""); }, [draft.kind, draft.metadataRevision, draft.uuid]);
  useEffect(() => setListSearch(query), [query]);
  useEffect(() => {
    if (listSearch.trim() === query) return;
    const timeout = window.setTimeout(() => setParameters((current) => {
      const next = new URLSearchParams(current);
      const value = listSearch.trim();
      if (value) next.set("q", value); else next.delete("q");
      next.set("page", "1");
      return next;
    }), 300);
    return () => window.clearTimeout(timeout);
  }, [listSearch, query, setParameters]);
  useEffect(() => {
    setDuplicateConfirmed(false);
    if (!newEntityName) { setCheckedEntityName(""); return; }
    const timeout = window.setTimeout(() => setCheckedEntityName(newEntityName), 350);
    return () => window.clearTimeout(timeout);
  }, [newEntityName, duplicateReviewKind, draft.kind === "CHARACTER" ? draft.workUUID : ""]);
  const data = listQuery.data?.manageCoreEntities;
  function updateListParameters(changes: Record<string, string | number | null>) {
    setParameters((current) => {
      const next = new URLSearchParams(current);
      if (coserOnly) next.delete("kind"); else next.set("kind", kind);
      for (const [key, value] of Object.entries(changes)) {
        if (value === null || value === "") next.delete(key); else next.set(key, String(value));
      }
      return next;
    });
  }
  function input(value = draft) { return { kind: value.kind, name: value.name, sortName: value.sortName, aliases: value.kind === "WORK" || value.kind === "CHARACTER" ? aliasChips : parseAliases(aliasText), workUUID: value.kind === "CHARACTER" ? value.workUUID || null : null, profileSummary: value.profileSummary, biography: value.biography, countryOrRegion: value.countryOrRegion, useInRecommendation: value.useInRecommendation }; }
  const activeDuplicateQuery = duplicateReviewKind === "COSER" ? coserDuplicateQuery : coreEntityDuplicateQuery;
  const conflictsCurrent = checkedEntityName === newEntityName && !activeDuplicateQuery.loading;
  const coserDuplicateConflicts = conflictsCurrent && duplicateReviewKind === "COSER" ? coserDuplicateQuery.data?.manageCoserNameConflicts || [] : [];
  const coreEntityDuplicateConflicts = conflictsCurrent && (duplicateReviewKind === "WORK" || duplicateReviewKind === "CHARACTER") ? coreEntityDuplicateQuery.data?.manageCoreEntityNameConflicts || [] : [];
  const sameWorkCharacterConflict = duplicateReviewKind === "CHARACTER" && Boolean(draft.workUUID) && coreEntityDuplicateConflicts.some((conflict) => conflict.primaryNameMatch && conflict.entity.workUUID === draft.workUUID);
  const duplicateConflictCount = duplicateReviewKind === "COSER" ? coserDuplicateConflicts.length : coreEntityDuplicateConflicts.length;
  const duplicateReady = !newEntityName || (conflictsCurrent && !activeDuplicateQuery.error && duplicateConflictCount === 0) || (conflictsCurrent && !activeDuplicateQuery.error && duplicateConflictCount > 0 && !sameWorkCharacterConflict && duplicateConfirmed) || (conflictsCurrent && Boolean(activeDuplicateQuery.error) && duplicateConfirmed);
  const entityValid = draft.name.trim() !== "" && (draft.kind !== "CHARACTER" || Boolean(draft.workUUID)) && !((draft.kind === "WORK" || draft.kind === "CHARACTER") && aliasChipDraft.trim()) && duplicateReady;
  const savedEntity = detailQuery.data?.manageCoreEntity;
  const entityEditorDirty = Boolean(savedEntity && (draft.name !== savedEntity.name || draft.sortName !== savedEntity.sortName ||
    draft.workUUID !== savedEntity.workUUID || aliasChipDraft.trim() || JSON.stringify(aliasChips) !== JSON.stringify(savedEntity.aliases)));
  const socialAccountValid = /^[a-z0-9][a-z0-9_-]{0,63}$/.test(account.platformKey) && isAbsoluteHTTPURL(account.url);
  async function save(event: FormEvent) { event.preventDefault(); if (!entityValid) return; setMessage(""); try { if (draft.uuid) { const result = await update({ variables: { uuid: draft.uuid, expectedMetadataRevision: draft.metadataRevision, input: input() } }); if (!result.data) throw new Error("Server did not return the saved entity"); setDraft(result.data.updateCoreEntity); } else { const result = await create({ variables: { input: input() } }); if (!result.data) throw new Error("Server did not return the created entity"); setDraft(result.data.createCoreEntity); updateListParameters({ uuid: result.data.createCoreEntity.uuid }); } await listQuery.refetch(); setMessage("Saved"); } catch (error) { setMessage(error instanceof Error ? error.message : "Unable to save entity"); } }
  async function addAccount(event: FormEvent) { event.preventDefault(); if (!draft.uuid || !socialAccountValid) return; setMessage(""); try { const result = await addSocial({ variables: { coserUUID: draft.uuid, expectedMetadataRevision: draft.metadataRevision, input: account } }); if (!result.data) throw new Error("Server did not return the saved social account"); setDraft(result.data.addCoserSocialAccount); setAccount({ ...account, platformKey: "", label: "", handle: "", url: "https://", position: String((draft.socialAccounts.length + 2) * 1024) }); } catch (error) { setMessage(error instanceof Error ? error.message : "Unable to add social account"); } }
  async function runCoserManifest(action: "PUSH" | "PULL") { if (!draft.uuid) return; setMessage(""); try { const call = action === "PUSH" ? pushManifest : pullManifest; await call({ variables: { coserUUID: draft.uuid, expectedMetadataRevision: draft.metadataRevision } }); await detailQuery.refetch(); await manifestQuery.refetch(); setMessage(`Coser Manifest ${action} completed`); } catch (error) { setMessage(error instanceof Error ? error.message : `Coser Manifest ${action} failed`); } }
  async function resolveCoserManifest() { const conflicts = manifestQuery.data?.manageCoserManifest.conflicts || []; if (!draft.uuid || conflicts.some((conflict) => !conflictChoices[conflict.path])) { setMessage("Choose Database or File for every conflict"); return; } setMessage(""); try { await resolveManifest({ variables: { coserUUID: draft.uuid, expectedMetadataRevision: draft.metadataRevision, choices: conflicts.map((conflict) => ({ path: conflict.path, choice: conflictChoices[conflict.path] })) } }); setConflictChoices({}); await detailQuery.refetch(); await manifestQuery.refetch(); setMessage("Coser Manifest conflicts resolved and pulled"); } catch (error) { setMessage(error instanceof Error ? error.message : "Coser Manifest conflict resolution failed"); } }
  async function saveTagParents() { if (draft.kind !== "TAG" || !draft.uuid) return; const expected = new Map<string, number>(); for (const parent of detailQuery.data?.manageCoreEntity.parents || []) expected.set(parent.uuid, parent.metadataRevision); for (const parent of draft.parents) expected.set(parent.uuid, parent.metadataRevision); setMessage(""); try { const result = await replaceTagParents({ variables: { childUUID: draft.uuid, expectedChildRevision: draft.metadataRevision, parents: draft.parents.map((parent, index) => ({ uuid: parent.uuid, position: String((index + 1) * 1024) })), expectedParents: Array.from(expected, ([uuid, metadataRevision]) => ({ uuid, metadataRevision })) } }); if (result.data) setDraft(result.data.replaceTagParents); await detailQuery.refetch(); await listQuery.refetch(); setMessage("Tag parent DAG saved"); } catch (error) { setMessage(error instanceof Error ? error.message : "Unable to save Tag parents"); } }
  async function merged(target: ManageCoreEntity, completionWarning?: string | null) { setDraft(target); updateListParameters({ uuid: target.uuid }); await listQuery.refetch(); setMessage(completionWarning ? `Merged. ${completionWarning}` : "Merged into the selected target; the source UUID is now a permanent alias."); }
  async function deleted() { setDraft(blank(kind)); updateListParameters({ uuid: null }); await listQuery.refetch(); setMessage("Entity deleted from the database and UUID permanently tombstoned; files were not removed."); }
  return <main className="manage-page"><header className="manage-heading"><div><p>PORTABLE UUID ENTITIES</p><h2>{coserOnly ? "Coser" : "Core entities"}</h2></div><button type="button" onClick={() => { updateListParameters({ uuid: null }); setDraft(blank(kind)); setAliasText(""); setAliasChips([]); setAliasChipDraft(""); }}>＋ New</button></header>{!coserOnly ? <nav className="task-filters">{kinds.map((value) => <button key={value} type="button" className={kind === value ? "is-active" : ""} onClick={() => setParameters({ kind: value })}>{value}</button>)}</nav> : null}{message ? <p className="manage-message">{message}</p> : null}
    <section className="entity-manage-layout"><aside className="entity-manage-list"><div className="entity-list-tools">
      <label className="entity-list-tools__search">{intl.formatMessage({ id: "manage.entityList.search" }, { kind: kindLabel })}<input type="search" maxLength={300} value={listSearch} placeholder={intl.formatMessage({ id: "manage.entityList.searchPlaceholder" })} onChange={(event) => setListSearch(event.target.value)} /></label>
      <p className="entity-list-tools__sort-note">{intl.formatMessage({ id: "manage.entityList.sortNote" })}</p>
      {coserOnly ? <label>{intl.formatMessage({ id: "manage.coserList.assets" })}<select value={assetFilter} onChange={(event) => updateListParameters({ assets: event.target.value === "ALL" ? null : event.target.value, page: 1 })}>{coserAssetFilters.map((value) => <option value={value} key={value}>{intl.formatMessage({ id: `manage.coserList.assets.${value.toLowerCase()}` })}</option>)}</select></label> : null}
      <label>{intl.formatMessage({ id: "manage.entityList.pageSize" })}<select value={pageSize} onChange={(event) => updateListParameters({ pageSize: Number(event.target.value) === defaultPageSize ? null : event.target.value, page: 1 })}>{entityPageSizes.map((value) => <option value={value} key={value}>{value}</option>)}</select></label>
      {query || assetFilter !== "ALL" ? <button type="button" className="entity-list-tools__clear" onClick={() => { setListSearch(""); updateListParameters({ q: null, assets: null, page: 1 }); }}>{intl.formatMessage({ id: "manage.entityList.clear" })}</button> : null}
    </div>{listQuery.loading && !data ? <p className="entity-list-state">{intl.formatMessage({ id: "manage.entityList.loading" }, { kind: kindLabel })}</p> : listQuery.error ? <p className="entity-list-state manage-error">{intl.formatMessage({ id: "manage.entityList.failed" }, { kind: kindLabel })}</p> : data?.items.length === 0 ? <p className="entity-list-state">{intl.formatMessage({ id: "manage.entityList.empty" }, { kind: kindLabel })}</p> : null}{data?.items.map((item) => <button type="button" className={`${item.uuid === selected ? "is-active" : ""}${item.kind === "COSER" ? " is-coser" : ""}`} key={item.uuid} onClick={() => updateListParameters({ uuid: item.uuid, page })}>{item.kind === "COSER" ? <ManageCoserListItem item={item} /> : <><strong>{item.name}</strong><span>{item.aliases.join(" / ") || item.uuid}</span></>}</button>)}{data ? <ManageEntityPagination data={data} kindLabel={kindLabel} onPage={(value) => updateListParameters({ page: value })} /> : null}</aside>
      <div className="entity-editor">{kind === "COSER" && draft.uuid ? <nav className="editor-tabs">{["profile", "social", "galleries", "manifest"].map((tab) => <button type="button" key={tab} className={coserTab === tab ? "is-active" : ""} onClick={() => setCoserTab(tab)}>{tab}</button>)}</nav> : null}
        {kind === "WORK" && draft.uuid ? <nav className="editor-tabs work-editor-tabs" aria-label={intl.formatMessage({ id: "manage.workCharacters.tabs" })}><button type="button" className={workTab === "details" ? "is-active" : ""} onClick={() => setWorkTab("details")}>{intl.formatMessage({ id: "manage.workCharacters.detailsTab" })}</button><button type="button" className={workTab === "characters" ? "is-active" : ""} onClick={() => setWorkTab("characters")}>{intl.formatMessage({ id: "manage.workCharacters.charactersTab" })}</button></nav> : null}
        {(kind !== "COSER" || coserTab === "profile" || !draft.uuid) && (kind !== "WORK" || workTab === "details" || !draft.uuid) ? <><EntityForm draft={draft} aliasText={aliasText} setAliasText={setAliasText} aliasChips={aliasChips} setAliasChips={setAliasChips} aliasChipDraft={aliasChipDraft} setAliasChipDraft={setAliasChipDraft} setDraft={setDraft} submit={save} saving={createState.loading || updateState.loading} valid={entityValid} nameReview={newEntityName ? duplicateReviewKind === "COSER" ? <CoserDuplicateReview name={newEntityName} checkedName={checkedEntityName} loading={coserDuplicateQuery.loading} error={Boolean(coserDuplicateQuery.error)} conflicts={coserDuplicateConflicts} confirmed={duplicateConfirmed} setConfirmed={setDuplicateConfirmed} openExisting={(coser) => updateListParameters({ uuid: coser.uuid })} /> : duplicateReviewKind === "WORK" || duplicateReviewKind === "CHARACTER" ? <CoreEntityDuplicateReview kind={duplicateReviewKind} name={newEntityName} checkedName={checkedEntityName} loading={coreEntityDuplicateQuery.loading} error={Boolean(coreEntityDuplicateQuery.error)} conflicts={coreEntityDuplicateConflicts} sameWorkBlocked={sameWorkCharacterConflict} confirmed={duplicateConfirmed} setConfirmed={setDuplicateConfirmed} openExisting={(entity) => updateListParameters({ uuid: entity.uuid })} /> : null : null} />{kind === "COSER" && draft.uuid ? <><ManageCoserAssetsPanel coser={draft} onUpdated={async () => { const result = await detailQuery.refetch(); if (result.data) setDraft(result.data.manageCoreEntity); await listQuery.refetch(); }} /><ManageCoserMetadataImport coser={draft} onUpdated={async () => { const result = await detailQuery.refetch(); if (result.data) setDraft(result.data.manageCoreEntity); await listQuery.refetch(); }} /></> : null}{(kind === "WORK" || kind === "CHARACTER") && draft.uuid ? <ManageEntityMetadataImport entity={draft} editorDirty={entityEditorDirty} onUpdated={async () => { const result = await detailQuery.refetch(); if (result.data) setDraft(result.data.manageCoreEntity); await listQuery.refetch(); }} /> : null}</> : null}
        {kind === "WORK" && draft.uuid && workTab === "characters" ? <WorkCharactersEditor work={draft} openStandalone={(entity) => setParameters({ kind: "CHARACTER", uuid: entity.uuid })} /> : null}
        {kind === "TAG" && draft.uuid ? <section className="manage-panel tag-parent-editor"><h3>Direct parent DAG</h3><p>A Tag may have multiple parents. The batch save checks every affected Tag revision and the database rejects cycles.</p><div className="manage-inline-toolbar"><span>{draft.parents.length} direct parents</span><button type="button" onClick={() => setDraft({ ...draft, parents: [...draft.parents, { uuid: "", name: "", metadataRevision: 0 }] })}>Add parent</button></div>{draft.parents.map((parent, index) => <div className="manage-tag-row" key={`${parent.uuid}-${index}`}><ManageEntitySelector kind="TAG" label="Parent Tag" uuid={parent.uuid} name={parent.name} onSelect={(entity) => setDraft({ ...draft, parents: draft.parents.map((value, position) => position === index ? { uuid: entity.uuid, name: entity.name, metadataRevision: entity.metadataRevision } : value) })} /><button type="button" onClick={() => setDraft({ ...draft, parents: draft.parents.filter((_, position) => position !== index) })}>Remove</button></div>)}<div className="manage-panel-actions"><button type="button" disabled={replaceTagParentsState.loading || draft.parents.some((parent) => !parent.uuid)} onClick={saveTagParents}>{replaceTagParentsState.loading ? "Saving…" : "Save parent DAG"}</button></div></section> : null}
        {draft.uuid && (kind !== "COSER" || coserTab === "profile") && (kind !== "WORK" || workTab === "details") ? <ManageEntityLifecyclePanel source={draft} onMerged={merged} onDeleted={deleted} /> : null}
        {kind === "COSER" && coserTab === "social" ? <section className="manage-panel"><h3>Social accounts</h3><div className="social-manage-list">{draft.socialAccounts.map((item) => <div key={item.uuid}><strong>{item.label || item.platformKey}</strong><span>{item.handle} · {item.status}</span><a href={item.url} target="_blank" rel="noreferrer">{item.url}</a></div>)}</div><form className="rule-form" onSubmit={addAccount}><label>Platform key<input required list="social-platform-keys" maxLength={64} pattern="[a-z0-9][a-z0-9_-]{0,63}" value={account.platformKey} onChange={(event) => setAccount({ ...account, platformKey: event.target.value })} /><small>Choose a common platform or type a custom lowercase key.</small></label><datalist id="social-platform-keys">{socialPlatformOptions.map(([value, label]) => <option key={value} value={value}>{label}</option>)}</datalist><label>Label<input maxLength={100} value={account.label} onChange={(event) => setAccount({ ...account, label: event.target.value })} /></label><label>Handle<input maxLength={200} value={account.handle} onChange={(event) => setAccount({ ...account, handle: event.target.value })} /></label><label>HTTP(S) URL<input type="url" required value={account.url} onChange={(event) => setAccount({ ...account, url: event.target.value })} /></label><label>Status<select value={account.status} onChange={(event) => setAccount({ ...account, status: event.target.value })}><option>ACTIVE</option><option>INACTIVE</option></select></label><label className="check"><input type="checkbox" checked={account.visible} onChange={(event) => setAccount({ ...account, visible: event.target.checked })} /> Visible</label><button type="submit" disabled={!socialAccountValid || addSocialState.loading}>{addSocialState.loading ? "Adding…" : "Add account"}</button></form></section> : null}
        {kind === "COSER" && coserTab === "galleries" ? <section className="manage-panel"><h3>Associated galleries</h3><p>Gallery relations are edited from each Gallery aggregate.</p><Link to={`/coser/${draft.slug}`}>Open unified Coser detail</Link></section> : null}
        {kind === "COSER" && coserTab === "manifest" ? <section className="manage-panel manage-manifest"><h3>Coser Manifest</h3><p>The independent Coser profile snapshot is optional and stored below the single configured metadata root. Pull and Push are always explicit.</p>
          {manifestQuery.loading ? <p>Checking Manifest…</p> : manifestQuery.error ? <p className="manage-error">Manifest inspection failed: {manifestQuery.error.message}</p> : manifestQuery.data ? <><dl><dt>Status</dt><dd><strong className={`manifest-status is-${manifestQuery.data.manageCoserManifest.status.toLowerCase()}`}>{manifestQuery.data.manageCoserManifest.status}</strong></dd><dt>Path</dt><dd>{manifestQuery.data.manageCoserManifest.path}</dd><dt>Manifest revision</dt><dd>{manifestQuery.data.manageCoserManifest.manifestRevision}</dd><dt>Database revision</dt><dd>{manifestQuery.data.manageCoserManifest.metadataRevision}</dd></dl>
            {manifestQuery.data.manageCoserManifest.conflicts.length ? <div className="manifest-conflicts"><h4>Conflicts require an explicit decision for every field</h4>{manifestQuery.data.manageCoserManifest.conflicts.map((conflict) => <article key={conflict.path}><code>{conflict.path}</code><div><label>Database<pre>{conflict.databaseJSON}</pre></label><label>Manifest file<pre>{conflict.fileJSON}</pre></label></div><select aria-label={`Resolution for ${conflict.path}`} value={conflictChoices[conflict.path] || ""} onChange={(event) => setConflictChoices({ ...conflictChoices, [conflict.path]: event.target.value as "DATABASE" | "FILE" })}><option value="">Choose…</option><option value="DATABASE">Keep database</option><option value="FILE">Use Manifest file</option></select></article>)}<button type="button" disabled={resolveManifestState.loading} onClick={resolveCoserManifest}>{resolveManifestState.loading ? "Resolving…" : "Resolve all and Pull"}</button></div> : null}</> : null}
          <div className="manage-panel-actions"><button type="button" onClick={() => manifestQuery.refetch()}>Check again</button><button type="button" disabled={pushManifestState.loading} onClick={() => runCoserManifest("PUSH")}>{pushManifestState.loading ? "Pushing…" : "Push database → Manifest"}</button><button type="button" disabled={pullManifestState.loading} onClick={() => runCoserManifest("PULL")}>{pullManifestState.loading ? "Pulling…" : "Pull Manifest → database"}</button></div>
        </section> : null}</div></section></main>;
}

function EntityForm({ draft, aliasText, setAliasText, aliasChips, setAliasChips, aliasChipDraft, setAliasChipDraft, setDraft, submit, saving, valid, nameReview, fixedCharacterWorkName }: { draft: ManageCoreEntity; aliasText: string; setAliasText: (value: string) => void; aliasChips: string[]; setAliasChips: (value: string[]) => void; aliasChipDraft: string; setAliasChipDraft: (value: string) => void; setDraft: (value: ManageCoreEntity) => void; submit: (event: FormEvent) => void; saving: boolean; valid: boolean; nameReview?: ReactNode; fixedCharacterWorkName?: string }) {
  const chipAliases = draft.kind === "WORK" || draft.kind === "CHARACTER";
  return <form className="metadata-form" onSubmit={submit}><label>Name (required)<input required maxLength={300} value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} /></label><label>Sort name<input maxLength={300} value={draft.sortName} onChange={(event) => setDraft({ ...draft, sortName: event.target.value })} /></label>{nameReview ? <div className="span-2">{nameReview}</div> : null}{chipAliases ? <AliasChipInput key={`${draft.kind}:${draft.uuid}:${draft.metadataRevision}`} aliases={aliasChips} onChange={setAliasChips} draft={aliasChipDraft} setDraft={setAliasChipDraft} /> : <label className="span-2">Aliases<input aria-describedby="entity-aliases-help" value={aliasText} onChange={(event) => setAliasText(event.target.value)} /><small id="entity-aliases-help">Separate multiple aliases with /, for example: Komachi / こまち / 小丁.</small></label>}{draft.kind === "CHARACTER" ? <div className="span-2 required-entity-field"><strong>Primary Work (required)</strong>{fixedCharacterWorkName ? <span className="fixed-character-work">{fixedCharacterWorkName}</span> : <ManageEntitySelector kind="WORK" label="Primary Work" uuid={draft.workUUID || ""} name={draft.workName || ""} onSelect={(entity) => setDraft({ ...draft, workUUID: entity.uuid, workName: entity.name })} />}{!draft.workUUID ? <small>Select an existing Work before creating the Character.</small> : null}</div> : null}{draft.kind === "TAG" ? <label className="span-2 settings-check"><input type="checkbox" checked={draft.useInRecommendation} onChange={(event) => setDraft({ ...draft, useInRecommendation: event.target.checked })} /> Use in recommendations</label> : null}{draft.kind === "COSER" ? <><label className="span-2">Profile summary<textarea maxLength={20000} value={draft.profileSummary} onChange={(event) => setDraft({ ...draft, profileSummary: event.target.value })} /></label><label className="span-2">Biography (restricted Markdown)<textarea maxLength={20000} value={draft.biography} onChange={(event) => setDraft({ ...draft, biography: event.target.value })} /></label><label>Country / region<input maxLength={100} value={draft.countryOrRegion} onChange={(event) => setDraft({ ...draft, countryOrRegion: event.target.value })} /></label></> : null}<footer className="span-2"><button type="submit" disabled={!valid || saving}>{saving ? "Saving…" : draft.uuid ? `Save revision ${draft.metadataRevision}` : "Create"}</button></footer></form>;
}
