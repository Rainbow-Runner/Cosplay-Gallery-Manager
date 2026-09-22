import { Component, type ErrorInfo, type ReactNode, useEffect, useMemo, useState } from "react";
import { useIntl } from "react-intl";

import type { ManageCoreEntity } from "./types";

interface Provider { key: string; label: string }
interface Candidate { ref: string; display_name: string; context: string; source_url: string; match_quality: number; type_confidence: string }
interface Suggestion { value: string; category: string; language_hint: string; evidence: string; default_selected: boolean }
interface Preview {
  token: string; provider_key: string; kind: "WORK" | "CHARACTER"; display_name: string; source_url: string;
  page_id: string; revision_id: string; suggestions: Suggestion[]; expires_at: string;
}

class EntityMetadataErrorBoundary extends Component<{
  children: ReactNode; resetKey: string; failureMessage: string; retryLabel: string;
}, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() { return { failed: true }; }
  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("CGM_ENTITY_METADATA_RENDER_FAILED", error, info.componentStack);
  }
  componentDidUpdate(previous: Readonly<{ resetKey: string }>) {
    if (this.state.failed && previous.resetKey !== this.props.resetKey) this.setState({ failed: false });
  }
  render() {
    if (this.state.failed) return <section className="manage-panel entity-metadata-import" role="alert">
      <p className="manage-error">{this.props.failureMessage}</p>
      <button type="button" onClick={() => this.setState({ failed: false })}>{this.props.retryLabel}</button>
    </section>;
    return this.props.children;
  }
}

async function metadataRequest<T>(path: string, body?: unknown): Promise<T> {
  const response = await fetch(`/manage/entity-metadata/${path}`, body === undefined ? { credentials: "same-origin" } : {
    method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body),
  });
  if (!response.ok) throw new Error((await response.text()).trim() || "Entity metadata request failed");
  return response.json() as Promise<T>;
}

const normalized = (value: string) => value.trim().normalize("NFC").toLocaleLowerCase();

export function ManageEntityMetadataImport(props: { entity: ManageCoreEntity; editorDirty: boolean; onUpdated: () => Promise<void> }) {
  const intl = useIntl();
  return <EntityMetadataErrorBoundary resetKey={`${props.entity.uuid}:${props.entity.metadataRevision}`} failureMessage={intl.formatMessage({ id: "manage.entityMetadata.renderFailed" })} retryLabel={intl.formatMessage({ id: "manage.entityMetadata.retry" })}>
    <ManageEntityMetadataImportInner {...props} />
  </EntityMetadataErrorBoundary>;
}

function ManageEntityMetadataImportInner({ entity, editorDirty, onUpdated }: { entity: ManageCoreEntity; editorDirty: boolean; onUpdated: () => Promise<void> }) {
  const intl = useIntl(); const t = (id: string) => intl.formatMessage({ id });
  const [providers, setProviders] = useState<Provider[]>([]); const [loadingProviders, setLoadingProviders] = useState(true);
  const [providerKey, setProviderKey] = useState(""); const [query, setQuery] = useState(entity.name); const [context, setContext] = useState("");
  const [candidates, setCandidates] = useState<Candidate[]>([]); const [preview, setPreview] = useState<Preview | null>(null);
  const [selected, setSelected] = useState<string[]>([]); const [searchedQuery, setSearchedQuery] = useState(""); const [searchCompleted, setSearchCompleted] = useState(false);
  const [busy, setBusy] = useState<"search" | "prepare" | "apply" | null>(null); const [message, setMessage] = useState("");

  useEffect(() => {
    let active = true;
    metadataRequest<{ providers: Provider[] }>("providers").then((value) => {
      if (!active) return; const next = Array.isArray(value.providers) ? value.providers : []; setProviders(next); setProviderKey(next[0]?.key || "");
    }).catch(() => { if (active) setProviders([]); }).finally(() => { if (active) setLoadingProviders(false); });
    return () => { active = false; };
  }, []);
  useEffect(() => {
    setQuery(entity.name); setContext(""); setCandidates([]); setPreview(null); setSelected([]); setSearchedQuery(""); setSearchCompleted(false); setMessage("");
  }, [entity.uuid, entity.name]);
  const occupied = useMemo(() => new Set([entity.name, ...entity.aliases].map(normalized)), [entity.name, entity.aliases]);

  async function search() {
    const name = query.trim(); if (!providerKey || !name) return;
    setBusy("search"); setMessage(""); setPreview(null); setCandidates([]); setSearchCompleted(false);
    try {
      const value = await metadataRequest<{ candidates: Candidate[] | null }>("search", { provider_key: providerKey, kind: entity.kind, query: name, context: context.trim() });
      setCandidates(Array.isArray(value.candidates) ? value.candidates : []); setSearchedQuery(name); setSearchCompleted(true);
    } catch (error) { setMessage(error instanceof Error ? error.message : t("manage.entityMetadata.failed")); }
    finally { setBusy(null); }
  }
  async function prepare(candidate: Candidate) {
    setBusy("prepare"); setMessage("");
    try {
      const value = await metadataRequest<Preview & { suggestions: Suggestion[] | null }>("prepare", { provider_key: providerKey, kind: entity.kind, entity_uuid: entity.uuid, candidate_ref: candidate.ref });
      const suggestions = Array.isArray(value.suggestions) ? value.suggestions : [];
      setPreview({ ...value, suggestions }); setSelected(suggestions.filter((item) => item.default_selected && !occupied.has(normalized(item.value))).map((item) => item.value));
    } catch (error) { setMessage(error instanceof Error ? error.message : t("manage.entityMetadata.failed")); }
    finally { setBusy(null); }
  }
  async function apply() {
    if (!preview || editorDirty || selected.length === 0) return;
    setBusy("apply"); setMessage("");
    try {
      await metadataRequest("apply", { token: preview.token, expected_metadata_revision: entity.metadataRevision, aliases: selected });
      await onUpdated(); setPreview(null); setCandidates([]); setSelected([]); setSearchCompleted(false); setMessage(t("manage.entityMetadata.saved"));
    } catch (error) { setMessage(error instanceof Error ? error.message : t("manage.entityMetadata.failed")); }
    finally { setBusy(null); }
  }

  if (!loadingProviders && providers.length === 0) return null;
  return <section className="manage-panel entity-metadata-import">
    <h3>{t("manage.entityMetadata.heading")}</h3><p>{t("manage.entityMetadata.summary")}</p>
    {editorDirty ? <p className="manage-message" role="status">{t("manage.entityMetadata.saveFirst")}</p> : null}
    {message ? <p className="manage-message" role="status">{message}</p> : null}
    <div className="manage-inline-toolbar">
      <label>{t("manage.entityMetadata.provider")}<select value={providerKey} disabled={loadingProviders || busy !== null} onChange={(event) => { setProviderKey(event.target.value); setCandidates([]); setPreview(null); setSearchCompleted(false); }}>{providers.map((provider) => <option key={provider.key} value={provider.key}>{provider.label}</option>)}</select></label>
      <label>{t("manage.entityMetadata.name")}<input maxLength={300} value={query} onChange={(event) => { setQuery(event.target.value); setCandidates([]); setPreview(null); setSearchCompleted(false); }} /></label>
      {entity.kind === "CHARACTER" ? <label>{t("manage.entityMetadata.context")}<input maxLength={300} value={context} placeholder={t("manage.entityMetadata.contextPlaceholder")} onChange={(event) => { setContext(event.target.value); setCandidates([]); setPreview(null); setSearchCompleted(false); }} /></label> : null}
      <button type="button" disabled={!providerKey || !query.trim() || busy !== null} onClick={search}>{busy === "search" ? t("manage.entityMetadata.searching") : t("manage.entityMetadata.search")}</button>
    </div>
    {searchCompleted ? <p className={`entity-metadata-search-status${candidates.length === 0 ? " is-empty" : ""}`} role="status" aria-live="polite">{candidates.length === 0
      ? intl.formatMessage({ id: "manage.entityMetadata.noResults" }, { query: searchedQuery })
      : intl.formatMessage({ id: "manage.entityMetadata.resultCount" }, { count: candidates.length })}</p> : null}
    {candidates.length ? <div className="entity-metadata-candidates">{candidates.map((candidate) => <button type="button" key={candidate.ref} disabled={busy !== null} onClick={() => prepare(candidate)}><span><strong>{candidate.display_name}</strong>{candidate.context ? <small>{candidate.context}</small> : null}</span><span>{t("manage.entityMetadata.match")} {candidate.match_quality}%</span></button>)}</div> : null}
    {preview ? <div className="entity-metadata-preview"><header><div><strong>{preview.display_name}</strong><a href={preview.source_url} target="_blank" rel="noreferrer">{t("manage.entityMetadata.source")}</a></div><small>{t("manage.entityMetadata.review")}</small></header>
      {preview.suggestions.length === 0 ? <p className="entity-metadata-search-status is-empty">{t("manage.entityMetadata.noAliases")}</p> : <div className="entity-metadata-suggestions">{preview.suggestions.map((suggestion) => { const exists = occupied.has(normalized(suggestion.value)); return <label key={`${suggestion.category}:${suggestion.value}`}><input type="checkbox" disabled={exists || busy !== null} checked={!exists && selected.includes(suggestion.value)} onChange={(event) => setSelected(event.target.checked ? [...selected, suggestion.value] : selected.filter((value) => value !== suggestion.value))} /><span><strong>{suggestion.value}</strong><small>{suggestion.evidence}{suggestion.language_hint ? ` · ${suggestion.language_hint}` : ""}{exists ? ` · ${t("manage.entityMetadata.exists")}` : ""}</small></span></label>; })}</div>}
      <button type="button" disabled={editorDirty || busy !== null || selected.length === 0} onClick={apply}>{busy === "apply" ? t("manage.entityMetadata.applying") : t("manage.entityMetadata.apply")}</button>
    </div> : null}
  </section>;
}
