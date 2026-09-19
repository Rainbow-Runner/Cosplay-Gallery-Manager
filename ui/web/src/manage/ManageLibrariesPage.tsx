import { useLazyQuery, useMutation, useQuery } from "@apollo/client/react";
import { type FormEvent, type RefObject, useEffect, useRef, useState } from "react";
import { useIntl } from "react-intl";
import { useNavigate } from "react-router-dom";
import { APPLY_MEDIA_LIBRARY_CHANGE, CANCEL_LIBRARY_AUTOMATION, CONFIRM_GALLERY_SOURCE_REBIND, CREATE_MEDIA_LIBRARY, CREATE_RECOGNITION_RULE, DELETE_RECOGNITION_RULE, DISCOVER_MEDIA_LIBRARY, IMPORT_GALLERY_CANDIDATE, MANAGE_DISCOVERY, MANAGE_IGNORED_SOURCES, MANAGE_LIBRARIES, MANAGE_LIBRARY_AUTOMATION, PREVIEW_IGNORED_SOURCE_REMOVAL, PREVIEW_MEDIA_LIBRARY_CHANGE, REVOKE_IGNORED_SOURCE, RUN_LIBRARY_AUTOMATION, SAVE_LIBRARY_AUTOMATION_POLICY, SET_MEDIA_LIBRARY_METADATA_WRITEBACK, TRANSFER_MEDIA_LIBRARY_SOURCE, UPDATE_RECOGNITION_RULE } from "../api/manage";
import { MediaClassificationRules } from "./MediaClassificationRules";
import { MediaExclusionRules } from "./MediaExclusionRules";
import type { ManageDiscoverySnapshot, ManageGalleryDetail, ManageIgnoredSourcePage, ManageIgnoredSourceRemovalPreview, ManageLibrary, ManageLibraryAutomation, ManageLibraryAutomationPolicy, ManageLibraryAutomationRun, ManageLibraryChangePreview, ManageRecognitionRule } from "./types";

const emptyLibrary = { name: "", rootPath: "", enabled: true, metadataWritebackEnabled: true, captureTimezone: "UTC" };
type RuleDraft = Omit<ManageRecognitionRule, "id">;
const emptyRule = (name: string): RuleDraft => ({ name, kind: "FIXED_DEPTH", enabled: false, autoCreateDraft: false, order: 100, pattern: "", fixedDepth: 1 });

export function ManageLibrariesPage() {
  const intl = useIntl();
  const f = (id: string, values?: Record<string, string | number>) => intl.formatMessage({ id }, values);
  const defaultRecognitionName = f("manage.library.defaultRuleName");
  const navigate = useNavigate();
  const librariesQuery = useQuery<{ manageLibraries: ManageLibrary[] }>(MANAGE_LIBRARIES);
  const libraries = librariesQuery.data?.manageLibraries ?? [];
  const [selectedID, setSelectedID] = useState<number | null>(null);
  const [libraryDraft, setLibraryDraft] = useState(emptyLibrary);
  const [ruleDraft, setRuleDraft] = useState(() => emptyRule(defaultRecognitionName));
  const [editingRuleID, setEditingRuleID] = useState<number | null>(null);
  const [deletingRuleID, setDeletingRuleID] = useState<number | null>(null);
  const rulesSectionRef = useRef<HTMLElement | null>(null);
  const [message, setMessage] = useState("");
  const discoveryQuery = useQuery<{ manageDiscovery: ManageDiscoverySnapshot }>(MANAGE_DISCOVERY, { variables: { libraryID: selectedID ?? 0 }, skip: selectedID === null });
  const [createLibrary, createLibraryState] = useMutation<{ createMediaLibrary: ManageLibrary }>(CREATE_MEDIA_LIBRARY);
  const [createRule, createRuleState] = useMutation(CREATE_RECOGNITION_RULE);
  const [updateRule, updateRuleState] = useMutation(UPDATE_RECOGNITION_RULE);
  const [deleteRule] = useMutation(DELETE_RECOGNITION_RULE);
  const [discover, discoveryState] = useMutation<{ discoverMediaLibrary: ManageDiscoverySnapshot }>(DISCOVER_MEDIA_LIBRARY);
  const [importCandidate] = useMutation<{ importGalleryCandidate: ManageGalleryDetail }>(IMPORT_GALLERY_CANDIDATE);
  const [confirmRebind, confirmRebindState] = useMutation<{ confirmGallerySourceRebind: ManageGalleryDetail }>(CONFIRM_GALLERY_SOURCE_REBIND);

  useEffect(() => {
    if (selectedID === null && libraries[0]) setSelectedID(libraries[0].id);
  }, [libraries, selectedID]);

  const snapshot = discoveryState.data?.discoverMediaLibrary ?? discoveryQuery.data?.manageDiscovery;
  const selectedLibrary = libraries.find((value) => value.id === selectedID) ?? null;
  const libraryValid = libraryDraft.name.trim() !== "" && libraryDraft.rootPath.startsWith("/") && libraryDraft.captureTimezone.trim() !== "";

  async function addLibrary(event: FormEvent) {
    event.preventDefault();
    if (!libraryValid) return;
    setMessage("");
    try {
      const result = await createLibrary({ variables: { input: libraryDraft } });
      if (!result.data) throw new Error(f("manage.library.error.noCreatedLibrary"));
      setSelectedID(result.data.createMediaLibrary.id);
      setLibraryDraft(emptyLibrary);
      await librariesQuery.refetch();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : f("manage.library.error.create"));
    }
  }
  function resetRuleEditor() {
    setRuleDraft(emptyRule(defaultRecognitionName));
    setEditingRuleID(null);
  }
  async function saveRule(event: FormEvent) {
    event.preventDefault();
    if (selectedID === null) return;
    setMessage("");
    const normalized = { ...ruleDraft, pattern: ruleDraft.kind === "PATH_TEMPLATE" ? ruleDraft.pattern : "", fixedDepth: ruleDraft.kind === "FIXED_DEPTH" ? ruleDraft.fixedDepth : 0 };
    try {
      if (editingRuleID === null) await createRule({ variables: { input: { ...normalized, libraryID: selectedID } } });
      else await updateRule({ variables: { input: { ...normalized, id: editingRuleID } } });
      resetRuleEditor();
      await librariesQuery.refetch();
      setMessage(f(editingRuleID === null ? "manage.library.recognitionAdded" : "manage.library.recognitionUpdated"));
    } catch (error) {
      setMessage(error instanceof Error ? error.message : f("manage.library.error.saveRecognition"));
    }
  }
  function editRule(rule: ManageRecognitionRule) {
    setDeletingRuleID(null);
    setEditingRuleID(rule.id);
    setRuleDraft({ name: rule.name, kind: rule.kind, enabled: rule.enabled, autoCreateDraft: rule.autoCreateDraft, order: rule.order, pattern: rule.pattern, fixedDepth: rule.fixedDepth || 1 });
  }
  async function removeRule(id: number) {
    setMessage("");
    try {
      await deleteRule({ variables: { id } });
      if (editingRuleID === id) resetRuleEditor();
      setDeletingRuleID(null);
      await librariesQuery.refetch();
      setMessage(f("manage.library.recognitionDeleted"));
    } catch (error) {
      setMessage(error instanceof Error ? error.message : f("manage.library.error.deleteRecognition"));
    }
  }
  async function runDiscovery() {
    if (selectedID === null) return;
    setMessage("");
    try {
      await discover({ variables: { libraryID: selectedID } });
    } catch (error) {
      setMessage(error instanceof Error ? error.message : f("manage.library.error.discovery"));
    }
  }
  async function createDraft(candidateID: number) {
    setMessage("");
    try {
      const result = await importCandidate({ variables: { candidateID } });
      const setID = result.data?.importGalleryCandidate.row.setID;
      if (setID) navigate(`/manage/gallery/${setID}`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : f("manage.library.error.createDraft"));
    }
  }
	async function rebindSource(candidate: ManageDiscoverySnapshot["candidates"][number]) {
		const warning = candidate.hasConflict
			? "The old and new sources are both accessible. Confirm that the new path must replace the old Gallery source."
			: "Rebind this existing Gallery to the discovered path? A complete source scan is still required afterwards.";
		if (!window.confirm(warning)) return;
		setMessage("");
		try {
			const result = await confirmRebind({ variables: { candidateID: candidate.id, allowAccessibleDuplicate: candidate.hasConflict } });
			const target = result.data?.confirmGallerySourceRebind.row.setID;
			if (target) navigate(`/manage/gallery/${target}?tab=source`);
		} catch (error) { setMessage(error instanceof Error ? error.message : "Unable to rebind Gallery source"); }
	}

  function prepareExactDirectoryRule(parentPath: string) {
    if (!selectedLibrary) return;
    const root = selectedLibrary.rootPath.replace(/\/+$/, "");
    if (!parentPath.startsWith(`${root}/`)) return;
    const relative = parentPath.slice(root.length + 1).replace(/\\/g, "/");
    const segments = relative.split("/").filter(Boolean);
    if (segments.length === 0) return;
    const escapeRE2 = (value: string) => value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    const title = segments.pop() as string;
    const prefix = segments.length ? `${segments.map(escapeRE2).join("/")}/` : "";
    setEditingRuleID(null);
    setDeletingRuleID(null);
    setRuleDraft({ name: `${f("manage.library.defaultRuleName")} · ${title}`, kind: "PATH_TEMPLATE", enabled: true,
      autoCreateDraft: false, order: 100, pattern: `${prefix}(?P<title>${escapeRE2(title)})`, fixedDepth: 0 });
    requestAnimationFrame(() => {
      const section = rulesSectionRef.current;
      const details = section?.querySelector("details");
      if (details) details.open = true;
      section?.scrollIntoView?.({ behavior: "smooth", block: "start" });
      section?.querySelector<HTMLInputElement>("input")?.focus();
    });
  }

  return <main className="manage-page">
    <header className="manage-heading"><div><p>{f("manage.library.eyebrow")}</p><h2>{f("manage.libraries")}</h2></div><button type="button" disabled={selectedID === null || discoveryState.loading} onClick={runDiscovery}>{discoveryState.loading ? f("manage.library.scanning") : f("manage.library.scanSelected")}</button></header>
    {message ? <p className="manage-message" role="status">{message}</p> : null}
    <section className="library-layout">
      <aside className="library-list"><h3>{f("manage.library.mediaLibraries")}</h3>{libraries.map((library) => <button type="button" className={library.id === selectedID ? "is-active" : ""} key={library.id} onClick={() => { setSelectedID(library.id); resetRuleEditor(); setDeletingRuleID(null); }}><strong>{library.name}</strong><span>{library.rootPath}</span></button>)}
        <details><summary>{f("manage.library.add")}</summary><form onSubmit={addLibrary}><label>{f("manage.library.nameRequired")}<input required value={libraryDraft.name} onChange={(event) => setLibraryDraft({ ...libraryDraft, name: event.target.value })} /></label><label>{f("manage.library.pathRequired")}<input required value={libraryDraft.rootPath} onChange={(event) => setLibraryDraft({ ...libraryDraft, rootPath: event.target.value })} /></label><label>{f("manage.library.timezoneRequired")}<input required value={libraryDraft.captureTimezone} onChange={(event) => setLibraryDraft({ ...libraryDraft, captureTimezone: event.target.value })} /></label><label className="check"><input type="checkbox" checked={libraryDraft.metadataWritebackEnabled} onChange={(event) => setLibraryDraft({ ...libraryDraft, metadataWritebackEnabled: event.target.checked })} /> {f("manage.library.metadataWriteback")}</label><p>{f("manage.library.mediaReadOnlyHelp")}</p><button type="submit" disabled={!libraryValid || createLibraryState.loading}>{createLibraryState.loading ? f("manage.library.creating") : f("manage.library.create")}</button></form></details>
      </aside>
      <div className="library-workspace">{selectedLibrary === null ? <p className="state-message">{f("manage.library.createFirst")}</p> : <>
        <LibraryWritebackPanel library={selectedLibrary} onUpdated={() => librariesQuery.refetch()} />
        <LibraryChangeWorkbench key={`change-${selectedLibrary.id}`} library={selectedLibrary} libraries={libraries} onApplied={async (deleted) => { const refreshed = await librariesQuery.refetch(); if (deleted) setSelectedID(refreshed.data?.manageLibraries[0]?.id ?? null); }} />
        <LibraryAutomationPanel key={selectedLibrary.id} libraryID={selectedLibrary.id} report={setMessage} />
        <LibraryRules sectionRef={rulesSectionRef} library={selectedLibrary} draft={ruleDraft} setDraft={setRuleDraft} submit={saveRule} saving={createRuleState.loading || updateRuleState.loading} editingRuleID={editingRuleID} deletingRuleID={deletingRuleID} editRule={editRule} cancelEdit={resetRuleEditor} requestDelete={setDeletingRuleID} deleteRule={removeRule} />
        <section className="candidate-section"><header><h3>{f("manage.library.latestDiscovery")}</h3><span>{snapshot?.completedAt || f("manage.library.notScanned")}</span></header>{snapshot?.candidates.map((candidate) => <article className={`candidate-card ${candidate.hasConflict || candidate.overLimit || candidate.status === "SOURCE_REBIND_CANDIDATE" ? "has-issue" : ""}`} key={candidate.id}><div><strong>{candidate.rootPath}</strong><p>{candidate.sourceType} · {candidate.method} · {f("manage.library.mediaCount", { count: candidate.mediaCount })}</p>{candidate.suggestions.length ? <ul>{candidate.suggestions.map((suggestion) => <li key={`${suggestion.field}:${suggestion.value}`}>{suggestion.field}: {suggestion.value}</li>)}</ul> : null}</div><div><span>{candidate.status}</span>{candidate.status === "SOURCE_REBIND_CANDIDATE" ? <button type="button" disabled={candidate.overLimit || confirmRebindState.loading} onClick={() => rebindSource(candidate)}>确认重新绑定 / Rebind</button> : <button type="button" disabled={candidate.status !== "PENDING" || candidate.hasConflict || candidate.overLimit} onClick={() => createDraft(candidate.id)}>{f("manage.library.createDraft")}</button>}</div></article>)}
          {snapshot && snapshot.candidates.length === 0 ? <p className="state-message">{f("manage.library.noCandidates")}</p> : null}
          {snapshot ? <section className="coverage-report"><header><div><h4>{f("manage.library.coverage.title")}</h4><p>{f("manage.library.coverage.help")}</p></div><strong>{f("manage.library.coverage.issues", { count: snapshot.coverageSummary.actionableIssueCount })}</strong></header><dl>
            <div><dt>{f("manage.library.coverage.regular")}</dt><dd>{snapshot.coverageSummary.regularFileCount}</dd></div>
            <div><dt>{f("manage.library.coverage.media")}</dt><dd>{snapshot.coverageSummary.supportedMediaCount}</dd></div>
            <div><dt>{f("manage.library.coverage.archives")}</dt><dd>{snapshot.coverageSummary.supportedArchiveCount}</dd></div>
            <div><dt>{f("manage.library.coverage.unsupported")}</dt><dd>{snapshot.coverageSummary.unsupportedArchiveCount}</dd></div>
            <div><dt>{f("manage.library.coverage.control")}</dt><dd>{snapshot.coverageSummary.controlFileCount}</dd></div>
            <div><dt>{f("manage.library.coverage.other")}</dt><dd>{snapshot.coverageSummary.ignoredOtherCount}</dd></div>
            <div><dt>{f("manage.library.coverage.sources")}</dt><dd>{snapshot.coverageSummary.registeredSourceCount}</dd></div>
            <div><dt>{f("manage.library.coverage.indexed")}</dt><dd>{snapshot.coverageSummary.indexedItemCount}</dd></div>
            <div><dt>{f("manage.library.coverage.needsScan")}</dt><dd>{snapshot.coverageSummary.sourceNeedsScanCount}</dd></div>
          </dl>{snapshot.coverageDiagnostics.length ? <details open><summary>{f("manage.library.coverage.review")}</summary>{snapshot.coverageDiagnostics.map((item) => <article key={`${item.reasonCode}:${item.path}`}><div><strong>{f(`manage.library.coverage.reason.${item.reasonCode}`)}</strong><span>{item.path}</span></div><b>{item.fileCount}</b></article>)}</details> : <p className="state-message">{f("manage.library.coverage.clear")}</p>}</section> : null}
          {snapshot?.unassigned.length ? <details className="unassigned"><summary>{f("manage.library.unassigned", { count: snapshot.unassigned.length })}</summary><p className="unassigned-help">{f("manage.library.unassignedHelp")}</p>{snapshot.unassigned.map((item) => <article key={item.parentPath}><span>{item.parentPath}</span><strong>{item.mediaCount}</strong><button type="button" onClick={() => prepareExactDirectoryRule(item.parentPath)}>{f("manage.library.createExactRule")}</button></article>)}</details> : null}
        </section>
      </>}</div>
    </section>
    <MediaClassificationRules library={selectedLibrary ? { id: selectedLibrary.id, name: selectedLibrary.name } : null} />
    <MediaExclusionRules library={selectedLibrary ? { id: selectedLibrary.id, name: selectedLibrary.name } : null} />
    <IgnoredSourcesWorkbench selectedLibrary={selectedLibrary} libraries={libraries} />
  </main>;
}

function LibraryWritebackPanel({ library, onUpdated }: { library: ManageLibrary; onUpdated: () => Promise<unknown> }) {
  const intl = useIntl();
  const f = (id: string, values?: Record<string, string | number>) => intl.formatMessage({ id }, values);
  const [save, state] = useMutation(SET_MEDIA_LIBRARY_METADATA_WRITEBACK);
  const [error, setError] = useState("");
  async function change() {
    const enabled = !library.metadataWritebackEnabled;
    if (!window.confirm(f(enabled ? "manage.library.writebackEnableConfirm" : "manage.library.writebackDisableConfirm", { count: library.boundGalleryCount }))) return;
    setError("");
    try {
      await save({ variables: { libraryID: library.id, enabled, expectedUpdatedAt: library.updatedAt } });
      await onUpdated();
    } catch (value) { setError(value instanceof Error ? value.message : f("manage.library.writebackFailed")); }
  }
  return <section className="manage-panel"><h3>{f("manage.library.metadataWriteback")}</h3>
    <p>{f("manage.library.writebackStatus", { status: f(library.metadataWritebackEnabled ? "manage.library.enabled" : "manage.library.disabled"), count: library.boundGalleryCount })}</p>
    <p>{f("manage.library.mediaReadOnlyHelp")}</p>
    <button type="button" disabled={state.loading} onClick={change}>{f(library.metadataWritebackEnabled ? "manage.library.disableWriteback" : "manage.library.enableWriteback")}</button>
    {error ? <p role="alert" className="manage-error">{error}</p> : null}
  </section>;
}

function IgnoredSourcesWorkbench({ selectedLibrary, libraries }: { selectedLibrary: ManageLibrary | null; libraries: ManageLibrary[] }) {
  const [scope, setScope] = useState<"global" | "selected">("global");
  const [search, setSearch] = useState("");
  const [loadedKey, setLoadedKey] = useState("");
  const [review, setReview] = useState<ManageIgnoredSourceRemovalPreview | null>(null);
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [message, setMessage] = useState("");
  const [fetchItems, itemsState] = useLazyQuery<{ manageIgnoredSources: ManageIgnoredSourcePage }>(MANAGE_IGNORED_SOURCES, { fetchPolicy: "network-only" });
  const [fetchReview, reviewState] = useLazyQuery<{ previewIgnoredSourceRemoval: ManageIgnoredSourceRemovalPreview }>(PREVIEW_IGNORED_SOURCE_REMOVAL, { fetchPolicy: "network-only" });
  const [revoke, revokeState] = useMutation<{ revokeIgnoredSource: boolean }>(REVOKE_IGNORED_SOURCE);
  const libraryID = scope === "global" ? null : selectedLibrary?.id ?? null;
  const keyFor = (page: number) => `${scope}:${libraryID ?? "global"}:${search.trim()}:${page}`;
  const page = itemsState.data?.manageIgnoredSources;
  const visible = page && loadedKey === keyFor(page.page) ? page : null;
  async function load(pageNumber = 1) {
    if (scope === "selected" && libraryID === null) return;
    setMessage(""); setReview(null); setPassword(""); setConfirmation(""); setLoadedKey("");
    try {
      const result = await fetchItems({ variables: { libraryID, page: pageNumber, query: search.trim() } });
      if (!result.data) throw new Error("Unable to load ignored sources");
      setLoadedKey(keyFor(pageNumber));
    } catch (error) { setMessage(error instanceof Error ? error.message : "Unable to load ignored sources"); }
  }
  async function previewRemoval(id: number) {
    setMessage(""); setReview(null); setPassword(""); setConfirmation("");
    try {
      const result = await fetchReview({ variables: { id } });
      if (!result.data) throw new Error("Unable to preview removal");
      setReview(result.data.previewIgnoredSourceRemoval);
    } catch (error) { setMessage(error instanceof Error ? error.message : "Unable to preview removal"); }
  }
  async function remove(event: FormEvent) {
    event.preventDefault();
    if (!review || confirmation !== "REVEAL" || !password || review.activeRunCount > 0) return;
    setMessage("");
    try {
      const result = await revoke({ variables: { id: review.record.id, revisionToken: review.revisionToken, password, confirmation } });
      if (!result.data?.revokeIgnoredSource) throw new Error("Unable to revoke ignored source");
      setReview(null); setPassword(""); setConfirmation("");
      await load(page?.page ?? 1);
      setMessage("忽略记录已撤销；下次发现扫描可能重新发现该路径。/ Ignore removed; the next discovery may see this path.");
    } catch (error) {
      const detail = error instanceof Error ? error.message : "Unable to revoke ignored source";
      setMessage(detail.includes("IGNORED_SOURCE_PREVIEW_STALE") ? "预览已过期，请重新预览。/ Preview is stale; review again." : detail.includes("IGNORED_SOURCE_AUTOMATION_ACTIVE") ? "请先停止受影响媒体库的自动化任务。/ Stop affected automation first." : detail);
    }
  }
  return <section className="rule-section ignored-source-section">
    <header><h3>忽略路径管理 / Ignored source paths</h3></header>
    <p>默认查看全局忽略；也可查看当前媒体库的专属忽略。撤销仅删除 CGM 的忽略记录，不会立即扫描或改动媒体文件。</p>
    <label>作用范围 / Scope<select value={scope} onChange={(event) => { setScope(event.target.value as "global" | "selected"); setLoadedKey(""); setReview(null); }}><option value="global">全局 / Global</option><option value="selected" disabled={!selectedLibrary}>当前媒体库 / Selected library</option></select></label>
    <label>路径搜索 / Path search<input value={search} onChange={(event) => { setSearch(event.target.value); setLoadedKey(""); setReview(null); }} maxLength={300} /></label>
    <button type="button" disabled={itemsState.loading || scope === "selected" && !selectedLibrary} onClick={() => load(1)}>加载忽略记录 / Load ignores</button>
    {message ? <p role="status">{message}</p> : null}
    {visible ? <><p>{visible.total} records · page {visible.page}</p>{visible.items.map((item) => <article className="candidate-card" key={item.id}><div><strong><code>{item.path}</code></strong><p>#{item.id} · {item.libraryID === null ? "Global" : libraries.find((lib) => lib.id === item.libraryID)?.name ?? item.libraryID} · {item.reason || "—"}</p>{item.setID ? <p>Set ID: {item.setID}</p> : null}</div><button type="button" onClick={() => previewRemoval(item.id)}>预览撤销 / Preview removal</button></article>)}
      {visible.total === 0 ? <p className="state-message">没有匹配的忽略记录 / No matching ignores</p> : null}
      <div><button type="button" disabled={visible.page <= 1 || itemsState.loading} onClick={() => load(visible.page - 1)}>上一页 / Previous</button><button type="button" disabled={visible.page * visible.pageSize >= visible.total || itemsState.loading} onClick={() => load(visible.page + 1)}>下一页 / Next</button></div>
    </> : null}
    {reviewState.loading ? <p>正在加载撤销预览 / Loading removal preview</p> : null}
    {review ? <form onSubmit={remove}><h4>撤销忽略 / Revoke ignore #{review.record.id}</h4><p><code>{review.record.path}</code> · {review.record.reason || "—"}</p>
      <p>受影响媒体库 / Affected libraries: {review.affectedLibraryIDs.map((id) => libraries.find((lib) => lib.id === id)?.name ?? id).join(", ") || "none"} · 活动任务 / Active runs: {review.activeRunCount} · 同路径 Gallery Source: {review.boundSourceCount}</p>
      <p>下次发现扫描可能重新出现；若有已删除 Gallery 的 Set ID，身份 Tombstone 仍可能阻止导入。不会自动扫描或修改媒体文件。</p>
      <label>所有者密码 / Owner password<input type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} /></label>
      <label>输入 REVEAL 确认 / Type REVEAL<input value={confirmation} onChange={(event) => setConfirmation(event.target.value)} /></label>
      <button type="submit" disabled={revokeState.loading || review.activeRunCount > 0 || confirmation !== "REVEAL" || !password}>撤销忽略 / Revoke ignore</button>
    </form> : null}
  </section>;
}

function LibraryChangeWorkbench({ library, libraries, onApplied }: { library: ManageLibrary; libraries: ManageLibrary[]; onApplied: (deleted: boolean) => Promise<void> }) {
  const [newRoot, setNewRoot] = useState("");
  const [previewedInput, setPreviewedInput] = useState<string | null>(null);
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [targets, setTargets] = useState<Record<number, string>>({});
  const [message, setMessage] = useState("");
  const [preview, previewState] = useLazyQuery<{ previewMediaLibraryChange: ManageLibraryChangePreview }>(PREVIEW_MEDIA_LIBRARY_CHANGE, { fetchPolicy: "network-only" });
  const [transfer, transferState] = useMutation(TRANSFER_MEDIA_LIBRARY_SOURCE);
  const [applyChange, applyState] = useMutation(APPLY_MEDIA_LIBRARY_CHANGE);
  const impact = previewState.data?.previewMediaLibraryChange;
  async function loadPreview() {
    setMessage("");
    setPreviewedInput(null);
    try { await preview({ variables: { libraryID: library.id, newRoot: newRoot.trim() } }); setPreviewedInput(newRoot.trim()); }
    catch (error) { setMessage(error instanceof Error ? error.message : "Unable to preview library impact"); }
  }
  const deleting = previewedInput === "";
  const phrase = deleting ? "DELETE LIBRARY" : "MOVE ROOT";
  const blocked = !impact || previewedInput === null || previewedInput !== newRoot.trim() || impact.activeRunCount > 0 || impact.scanningSourceCount > 0 || impact.portableMappingCount > 0 || (deleting && impact.impacts.length > 0) || (!deleting && (impact.currentRoot === impact.proposedRoot || impact.childRoots.length > 0 || impact.proposedBoundaryConflicts.length > 0));
  async function applyReviewedChange(event: FormEvent) {
    event.preventDefault();
    if (!impact || blocked || confirmation !== phrase || !password) return;
    setMessage("");
    try {
      await applyChange({ variables: { libraryID: library.id, newRoot: previewedInput, revisionToken: impact.revisionToken, password, confirmation } });
      setPassword(""); setConfirmation(""); setPreviewedInput(null);
      await onApplied(deleting);
      setMessage("Media library change applied / 媒体库变更已执行");
    } catch (error) { setMessage(error instanceof Error ? error.message : "Unable to apply library change; refresh preview"); }
  }
  async function transferSource(sourceID: number) {
    const selected = targets[sourceID];
    if (!selected) return;
    const targetID = selected === "unassign" ? null : Number(selected);
    const targetName = targetID === null ? "unassigned / 未分配" : libraries.find((item) => item.id === targetID)?.name;
    if (!window.confirm(`Transfer only Source #${sourceID} to ${targetName}? No media files will be moved.\n仅更改 Source 归属，不移动媒体文件。`)) return;
    setMessage("");
    try {
      await transfer({ variables: { sourceID, expectedLibraryID: library.id, targetLibraryID: targetID } });
      setTargets((previous) => ({ ...previous, [sourceID]: "" }));
      await loadPreview();
      setMessage("Source binding updated / Source 归属已更新");
    } catch (error) { setMessage(error instanceof Error ? error.message : "Unable to transfer Source"); }
  }
  return <section className="rule-section library-change-section">
    <header><h3>媒体库变更影响 / Library change impact</h3></header>
    <p>先预览改根或删除的全部影响；变更须输入所有者密码及确认词。操作只改变 CGM 记录，不移动或删除媒体文件。改根后 Source 需重新扫描。</p>
    <label>拟议的新根路径（留空预览删除） / Proposed root (blank for deletion)
      <input value={newRoot} onChange={(event) => setNewRoot(event.target.value)} placeholder="/media/new-root" />
    </label>
    <button type="button" disabled={previewState.loading || !!newRoot.trim() && !newRoot.startsWith("/")} onClick={loadPreview}>预览影响 / Preview impact</button>
    {message ? <p role="status">{message}</p> : null}
    {impact ? <div>
      <p>当前根 / Current root: <code>{impact.currentRoot}</code>{impact.proposedRoot ? <> → <code>{impact.proposedRoot}</code></> : " → deletion preview"}</p>
      <p>Source: {impact.impacts.length} · 忽略的 Source / Ignored sources: {impact.ignoredSourceCount} · 子媒体库 / Child libraries: {impact.childRoots.length}</p>
      <p>识别规则 / Recognition: {impact.recognitionRules.length} · 分类规则 / Classification: {impact.classificationRules.length} · 排除规则 / Exclusion: {impact.exclusionRules.length}</p>
      <p>自动化 / Automation: {impact.automationMode} (revision {impact.automationPolicyRevision}) · 历史任务 / Runs: {impact.automationRunCount} · 活动任务 / Active: {impact.activeRunCount} · 正在扫描 / Scanning: {impact.scanningSourceCount} · 迁移映射 / Migration mappings: {impact.portableMappingCount}</p>
      {impact.ignoredSources.length ? <details><summary>被忽略的 Source / Ignored source paths</summary>{impact.ignoredSources.map((item) => <p key={item.id}><code>{item.path}</code> · {item.reason}</p>)}</details> : null}
      {impact.unassignedSourcePaths.length ? <details><summary>库根下未分配的 Source / Unassigned sources ({impact.unassignedSourcePaths.length})</summary><p>{deleting ? "删除时会为这些精确路径建立全局忽略，防止父库重新导入。" : "这些Source不属于本库；改根不会映射其路径，之后仍需人工处理。"}</p>{impact.unassignedSourcePaths.map((path) => <p key={path}><code>{path}</code></p>)}</details> : null}
      {([['Recognition',impact.recognitionRules],['Classification',impact.classificationRules],['Exclusion',impact.exclusionRules]] as const).map(([name,rules]) => rules.length ? <details key={name}><summary>{name} rules ({rules.length})</summary>{rules.map((rule) => <p key={rule.id}>#{rule.id} {rule.name}</p>)}</details> : null)}
      {impact.childRoots.map((root) => <p key={root}><code>{root}</code></p>)}
      {impact.proposedBoundaryConflicts.map((root) => <p key={`conflict-${root}`}>目标根包含其他媒体库 / Proposed root contains library: <code>{root}</code></p>)}
      {impact.impacts.map((source) => <article className="candidate-card" key={source.sourceID}>
        <div><strong>{source.galleryTitle}</strong><p>Source #{source.sourceID} · <code>{source.sourcePath}</code></p><p>建议归属 / Suggested owner: {source.suggestedOwnerID === null ? "unassigned" : libraries.find((item) => item.id === source.suggestedOwnerID)?.name ?? source.suggestedOwnerID}</p></div>
        <div><label>显式转移到 / Transfer to
          <select aria-label={`Transfer Source ${source.sourceID} to`} value={targets[source.sourceID] ?? ""} onChange={(event) => setTargets((previous) => ({ ...previous, [source.sourceID]: event.target.value }))}>
            <option value="">选择目标 / Select target</option><option value="unassign">未分配 / Unassigned</option>
            {libraries.filter((item) => item.id !== library.id).map((item) => <option key={item.id} value={item.id}>{item.name} · {item.rootPath}</option>)}
          </select>
        </label><button type="button" disabled={!targets[source.sourceID] || transferState.loading} onClick={() => transferSource(source.sourceID)}>确认转移 / Transfer</button></div>
      </article>)}
      <form onSubmit={applyReviewedChange}>
        <p>{deleting ? "删除会保留该库的 ignored Source 为全局精确路径忽略，防止父库静默重新导入；专属规则、自动化策略和历史任务会删除。已绑定的 Gallery Source 必须先逐项转移。子媒体库仍保留。" : "改根会按相对路径映射 Source、ignored Source 和 Manifest 路径，保留规则/自动化策略，并把 Source 标为待重新扫描。子媒体库须先单独处理。"}</p>
        {blocked ? <p role="status">当前变更不可执行：请刷新预览并解决 Source、活动任务、正在扫描、迁移映射或媒体库边界门禁。</p> : null}
        <label>所有者密码 / Owner password<input type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} /></label>
        <label>输入确认词 / Type {phrase}<input value={confirmation} onChange={(event) => setConfirmation(event.target.value)} /></label>
        <button type="submit" disabled={blocked || confirmation !== phrase || !password || applyState.loading}>{deleting ? "删除媒体库 / Delete library" : "更改媒体库根 / Move root"}</button>
      </form>
    </div> : null}
  </section>;
}

function LibraryAutomationPanel({ libraryID, report }: { libraryID: number; report: (message: string) => void }) {
  const intl = useIntl();
  const f = (id: string, values?: Record<string, string | number>) => intl.formatMessage({ id }, values);
  const query = useQuery<{ manageLibraryAutomation: ManageLibraryAutomation }>(MANAGE_LIBRARY_AUTOMATION, { variables: { libraryID } });
  const [save, saveState] = useMutation<{ saveLibraryAutomationPolicy: ManageLibraryAutomation }>(SAVE_LIBRARY_AUTOMATION_POLICY);
  const [run, runState] = useMutation<{ runLibraryAutomation: ManageLibraryAutomationRun }>(RUN_LIBRARY_AUTOMATION);
  const [cancel, cancelState] = useMutation(CANCEL_LIBRARY_AUTOMATION);
  const [draft, setDraft] = useState<ManageLibraryAutomationPolicy | null>(null);
  const [savedPolicy, setSavedPolicy] = useState<ManageLibraryAutomationPolicy | null>(null);
  const loadedPolicyRevision = useRef<number | null>(null);
  useEffect(() => {
    const policy = query.data?.manageLibraryAutomation.policy;
    if (policy && loadedPolicyRevision.current !== policy.revision) {
      loadedPolicyRevision.current = policy.revision;
      setDraft(policy);
      setSavedPolicy(policy);
    }
  }, [query.data?.manageLibraryAutomation.policy]);
  const activeRun = query.data?.manageLibraryAutomation.recentRuns.find((value) => value.status === "QUEUED" || value.status === "RUNNING") ?? null;
  useEffect(() => {
    if (activeRun) query.startPolling(1500);
    else query.stopPolling();
    return () => query.stopPolling();
  }, [activeRun?.id, activeRun?.status, query.startPolling, query.stopPolling]);
  if (draft === null && query.loading) return <section className="rule-section automation-section"><p>{f("manage.automation.loading")}</p></section>;
  if (draft === null && query.error) return <section className="rule-section automation-section"><p role="alert">{query.error.message}</p></section>;
  if (draft === null) return null;
  const state = query.data?.manageLibraryAutomation;
  async function savePolicy(event: FormEvent) {
    event.preventDefault();
    if (!draft) return;
    report("");
    try {
      const input = { mode: draft.mode, defaultContentRating: draft.defaultContentRating || null, excludeNewRootMedia: draft.excludeNewRootMedia,
		autoImportArchives: draft.autoImportArchives,
        autoAcceptUniqueEntities: draft.autoAcceptUniqueEntities, autoAcceptMediaClassification: draft.autoAcceptMediaClassification,
        autoActivate: draft.mode === "TRUSTED" ? draft.autoActivate : false };
      const result = await save({ variables: { libraryID, expectedRevision: draft.revision, input } });
      if (result.data) {
        loadedPolicyRevision.current = result.data.saveLibraryAutomationPolicy.policy.revision;
        setDraft(result.data.saveLibraryAutomationPolicy.policy);
        setSavedPolicy(result.data.saveLibraryAutomationPolicy.policy);
      }
      report(f("manage.automation.saved"));
    } catch (error) { report(error instanceof Error ? error.message : f("manage.automation.saveFailed")); }
  }
  async function runNow() {
    report("");
    try {
      const result = await run({ variables: { libraryID } });
      const value = result.data?.runLibraryAutomation;
      if (value) report(f("manage.automation.queued", { id: value.id }));
      await query.refetch();
    } catch (error) { report(error instanceof Error ? error.message : f("manage.automation.runFailed")); }
  }
  async function cancelRun() {
    if (!activeRun) return;
    report("");
    try {
      await cancel({ variables: { runID: activeRun.id } });
      report(f("manage.automation.cancelRequested"));
      await query.refetch();
    } catch (error) { report(error instanceof Error ? error.message : f("manage.automation.cancelFailed")); }
  }
  const comparablePolicy = (policy: ManageLibraryAutomationPolicy) => JSON.stringify({
    mode: policy.mode,
    defaultContentRating: policy.defaultContentRating,
    excludeNewRootMedia: policy.excludeNewRootMedia,
    autoImportArchives: policy.autoImportArchives,
    autoAcceptUniqueEntities: policy.autoAcceptUniqueEntities,
    autoAcceptMediaClassification: policy.autoAcceptMediaClassification,
    autoActivate: policy.autoActivate,
  });
  const hasUnsavedChanges = savedPolicy === null || comparablePolicy(draft) !== comparablePolicy(savedPolicy);
  const canRun = draft.revision > 0 && draft.mode !== "MANUAL" && !hasUnsavedChanges && activeRun === null;
  const autoActivationValid = draft.mode !== "TRUSTED" || !draft.autoActivate || Boolean(draft.defaultContentRating);
  const progressTotal = activeRun?.totalTargets ?? 0;
  const progressProcessed = activeRun?.processedTargets ?? 0;
  const progressPercent = progressTotal > 0 ? Math.min(100, Math.round(progressProcessed * 100 / progressTotal)) : 0;
  return <section className="rule-section automation-section">
    <header><div><h3>{f("manage.automation.title")}</h3><p>{f("manage.automation.help")}</p></div><div className="automation-actions"><button type="button" disabled={!canRun || runState.loading} onClick={runNow}>{runState.loading ? f("manage.automation.queueing") : f("manage.automation.run")}</button>{activeRun ? <button type="button" disabled={cancelState.loading || activeRun.cancellationRequested} onClick={cancelRun}>{activeRun.cancellationRequested ? f("manage.automation.cancelling") : f("manage.automation.cancel")}</button> : null}</div></header>
    <form className="automation-form" onSubmit={savePolicy}>
      <label>{f("manage.automation.mode")}<select value={draft.mode} onChange={(event) => setDraft({ ...draft, mode: event.target.value as ManageLibraryAutomationPolicy["mode"], autoActivate: event.target.value === "TRUSTED" ? draft.autoActivate : false })}><option value="MANUAL">{f("manage.automation.manual")}</option><option value="ASSISTED">{f("manage.automation.assisted")}</option><option value="TRUSTED">{f("manage.automation.trusted")}</option></select></label>
      <label>{f("manage.automation.rating")}<select value={draft.defaultContentRating ?? ""} onChange={(event) => setDraft({ ...draft, defaultContentRating: (event.target.value || null) as ManageLibraryAutomationPolicy["defaultContentRating"] })}><option value="">{f("manage.automation.noDefault")}</option><option value="NON_ADULT">LIST</option><option value="ADULT">MAGIC</option></select></label>
      <label className="check"><input type="checkbox" checked={draft.excludeNewRootMedia} onChange={(event) => setDraft({ ...draft, excludeNewRootMedia: event.target.checked })} /> {f("manage.automation.excludeRoot")}</label>
	  <label className="check"><input type="checkbox" disabled={draft.mode === "MANUAL"} checked={draft.autoImportArchives} onChange={(event) => setDraft({ ...draft, autoImportArchives: event.target.checked })} /> {f("manage.automation.importArchives")}</label>
      <label className="check"><input type="checkbox" checked={draft.autoAcceptUniqueEntities} onChange={(event) => setDraft({ ...draft, autoAcceptUniqueEntities: event.target.checked })} /> {f("manage.automation.entities")}</label>
      <label className="check"><input type="checkbox" checked={draft.autoAcceptMediaClassification} onChange={(event) => setDraft({ ...draft, autoAcceptMediaClassification: event.target.checked })} /> {f("manage.automation.classification")}</label>
      <label className="check"><input type="checkbox" disabled={draft.mode !== "TRUSTED"} checked={draft.autoActivate} onChange={(event) => setDraft({ ...draft, autoActivate: event.target.checked })} /> {f("manage.automation.activate")}</label>
      {!autoActivationValid ? <p className="automation-warning">{f("manage.automation.ratingRequired")}</p> : null}
      <button type="submit" disabled={!autoActivationValid || saveState.loading}>{saveState.loading ? f("manage.library.saving") : f("manage.automation.save")}</button>
    </form>
    <p className="automation-last-run">{hasUnsavedChanges ? f("manage.automation.unsaved") : draft.mode === "MANUAL" ? f("manage.automation.manualRunDisabled") : f("manage.automation.explicitRun")}</p>
    {activeRun ? <div className="automation-progress" aria-live="polite">
      <div className="automation-progress__heading"><strong>{f(`manage.automation.phase.${activeRun.phase || activeRun.status}`)}</strong><span>{progressTotal > 0 ? f("manage.automation.progressCount", { processed: progressProcessed, total: progressTotal }) : f("manage.automation.progressPreparing")}</span></div>
      <div className={`automation-progress__track${progressTotal === 0 ? " is-indeterminate" : ""}`} role="progressbar" aria-label={f("manage.automation.progressLabel")} aria-valuemin={0} aria-valuemax={progressTotal || undefined} aria-valuenow={progressTotal > 0 ? progressProcessed : undefined}><span style={progressTotal > 0 ? { width: `${progressPercent}%` } : undefined} /></div>
      {activeRun.currentGalleryTitle ? <p>{f("manage.automation.currentGallery", { title: activeRun.currentGalleryTitle })}</p> : null}
    </div> : null}
    {state ? <div className="automation-summary"><span>{f("manage.automation.candidates", { count: state.preview.candidateCount })}</span><span>{f("manage.automation.drafts", { count: state.preview.draftCount })}</span><span>{f("manage.automation.ready", { count: state.preview.activationReady })}</span><span>{f("manage.automation.review", { count: state.preview.needsReview })}</span></div> : null}
    {state?.recentRuns[0] ? <p className="automation-last-run" role="status">{f("manage.automation.lastRun", { status: state.recentRuns[0].status, scanned: state.recentRuns[0].scanned, activated: state.recentRuns[0].activated, review: state.recentRuns[0].needsReview })}</p> : null}
  </section>;
}

function LibraryRules({ sectionRef, library, draft, setDraft, submit, saving, editingRuleID, deletingRuleID, editRule, cancelEdit, requestDelete, deleteRule }: {
  sectionRef: RefObject<HTMLElement | null>;
  library: ManageLibrary;
  draft: RuleDraft;
  setDraft: (value: RuleDraft) => void;
  submit: (event: FormEvent) => void;
  saving: boolean;
  editingRuleID: number | null;
  deletingRuleID: number | null;
  editRule: (rule: ManageRecognitionRule) => void;
  cancelEdit: () => void;
  requestDelete: (id: number | null) => void;
  deleteRule: (id: number) => void;
}) {
  const intl = useIntl();
  const f = (id: string, values?: Record<string, string | number>) => intl.formatMessage({ id }, values);
  const ruleValid = draft.name.trim() !== "" &&
    (draft.kind !== "PATH_TEMPLATE" || draft.pattern.trim() !== "") &&
    (draft.kind !== "FIXED_DEPTH" || (draft.fixedDepth >= 1 && draft.fixedDepth <= 64));

  return <section className="rule-section" ref={sectionRef}><header><div><h3>{library.name}</h3><p>{library.rootPath}</p></div><button type="button" onClick={() => navigator.clipboard?.writeText(library.rootPath)}>{f("manage.library.copyPath")}</button></header><div className="rule-list">{library.rules.map((rule) => <div key={rule.id}><div><strong>{rule.name}</strong><span>{rule.kind} · {rule.enabled ? f("manage.classification.on") : f("manage.classification.off")} · auto DRAFT {rule.autoCreateDraft ? f("manage.classification.on") : f("manage.classification.off")}</span></div><div className="rule-actions">{deletingRuleID === rule.id ? <><button type="button" onClick={() => requestDelete(null)}>{f("manage.library.cancel")}</button><button type="button" className="danger" onClick={() => deleteRule(rule.id)}>{f("manage.library.confirmDelete")}</button></> : <><button type="button" onClick={() => editRule(rule)}>{f("manage.classification.edit")}</button><button type="button" onClick={() => requestDelete(rule.id)}>{f("manage.classification.delete")}</button></>}</div></div>)}</div>
    <details key={editingRuleID ?? "new"} open={editingRuleID !== null ? true : undefined}><summary>{editingRuleID === null ? f("manage.library.addRecognition") : f("manage.library.editRecognition", { id: editingRuleID })}</summary><form className="rule-form" onSubmit={submit}><label>{f("manage.library.nameRequired")}<input value={draft.name} required onChange={(event) => setDraft({ ...draft, name: event.target.value })} /></label><label>{f("manage.library.mode")}<select value={draft.kind} onChange={(event) => setDraft({ ...draft, kind: event.target.value as RuleDraft["kind"] })}><option value="FIXED_DEPTH">DIRECT_CHILD / FIXED_DEPTH</option><option value="MARKER">.cosplay-root MARKER</option><option value="PATH_TEMPLATE">PATH_TEMPLATE (RE2)</option></select></label><label>{f("manage.classification.order")}<input type="number" value={draft.order} onChange={(event) => setDraft({ ...draft, order: Number(event.target.value) })} /></label>{draft.kind === "FIXED_DEPTH" ? <label>{f("manage.library.depth")}<input type="number" min="1" max="64" value={draft.fixedDepth} onChange={(event) => setDraft({ ...draft, fixedDepth: Number(event.target.value) })} /></label> : null}{draft.kind === "PATH_TEMPLATE" ? <label className="wide">{f("manage.library.relativeRe2Required")}<input value={draft.pattern} required onChange={(event) => setDraft({ ...draft, pattern: event.target.value })} /></label> : null}<label className="check"><input type="checkbox" checked={draft.enabled} onChange={(event) => setDraft({ ...draft, enabled: event.target.checked })} /> {f("manage.library.enableRule")}</label><label className="check"><input type="checkbox" checked={draft.autoCreateDraft} onChange={(event) => setDraft({ ...draft, autoCreateDraft: event.target.checked })} /> {f("manage.library.autoDraft")}</label><div className="rule-form-actions"><button type="submit" disabled={!ruleValid || saving}>{saving ? f("manage.library.saving") : editingRuleID === null ? f("manage.library.addRule") : f("manage.library.saveRule")}</button>{editingRuleID !== null ? <button type="button" onClick={cancelEdit}>{f("manage.library.cancelEdit")}</button> : null}</div></form></details>
  </section>;
}
