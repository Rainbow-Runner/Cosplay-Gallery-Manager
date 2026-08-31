import { useMutation, useQuery } from "@apollo/client/react";
import { type FormEvent, type RefObject, useEffect, useRef, useState } from "react";
import { useIntl } from "react-intl";
import { useNavigate } from "react-router-dom";
import { CANCEL_LIBRARY_AUTOMATION, CREATE_MEDIA_LIBRARY, CREATE_RECOGNITION_RULE, DELETE_RECOGNITION_RULE, DISCOVER_MEDIA_LIBRARY, IMPORT_GALLERY_CANDIDATE, MANAGE_DISCOVERY, MANAGE_LIBRARIES, MANAGE_LIBRARY_AUTOMATION, RUN_LIBRARY_AUTOMATION, SAVE_LIBRARY_AUTOMATION_POLICY, UPDATE_RECOGNITION_RULE } from "../api/manage";
import { MediaClassificationRules } from "./MediaClassificationRules";
import { MediaExclusionRules } from "./MediaExclusionRules";
import type { ManageDiscoverySnapshot, ManageGalleryDetail, ManageLibrary, ManageLibraryAutomation, ManageLibraryAutomationPolicy, ManageLibraryAutomationRun, ManageRecognitionRule } from "./types";

const emptyLibrary = { name: "", rootPath: "", enabled: true, readOnly: true, captureTimezone: "UTC" };
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
        <details><summary>{f("manage.library.add")}</summary><form onSubmit={addLibrary}><label>{f("manage.library.nameRequired")}<input required value={libraryDraft.name} onChange={(event) => setLibraryDraft({ ...libraryDraft, name: event.target.value })} /></label><label>{f("manage.library.pathRequired")}<input required value={libraryDraft.rootPath} onChange={(event) => setLibraryDraft({ ...libraryDraft, rootPath: event.target.value })} /></label><label>{f("manage.library.timezoneRequired")}<input required value={libraryDraft.captureTimezone} onChange={(event) => setLibraryDraft({ ...libraryDraft, captureTimezone: event.target.value })} /></label><label className="check"><input type="checkbox" checked={libraryDraft.readOnly} onChange={(event) => setLibraryDraft({ ...libraryDraft, readOnly: event.target.checked })} /> {f("manage.library.readOnly")}</label><button type="submit" disabled={!libraryValid || createLibraryState.loading}>{createLibraryState.loading ? f("manage.library.creating") : f("manage.library.create")}</button></form></details>
      </aside>
      <div className="library-workspace">{selectedLibrary === null ? <p className="state-message">{f("manage.library.createFirst")}</p> : <>
        <LibraryAutomationPanel key={selectedLibrary.id} libraryID={selectedLibrary.id} report={setMessage} />
        <LibraryRules sectionRef={rulesSectionRef} library={selectedLibrary} draft={ruleDraft} setDraft={setRuleDraft} submit={saveRule} saving={createRuleState.loading || updateRuleState.loading} editingRuleID={editingRuleID} deletingRuleID={deletingRuleID} editRule={editRule} cancelEdit={resetRuleEditor} requestDelete={setDeletingRuleID} deleteRule={removeRule} />
        <section className="candidate-section"><header><h3>{f("manage.library.latestDiscovery")}</h3><span>{snapshot?.completedAt || f("manage.library.notScanned")}</span></header>{snapshot?.candidates.map((candidate) => <article className={`candidate-card ${candidate.hasConflict || candidate.overLimit ? "has-issue" : ""}`} key={candidate.id}><div><strong>{candidate.rootPath}</strong><p>{candidate.sourceType} · {candidate.method} · {f("manage.library.mediaCount", { count: candidate.mediaCount })}</p>{candidate.suggestions.length ? <ul>{candidate.suggestions.map((suggestion) => <li key={`${suggestion.field}:${suggestion.value}`}>{suggestion.field}: {suggestion.value}</li>)}</ul> : null}</div><div><span>{candidate.status}</span><button type="button" disabled={candidate.status !== "PENDING" || candidate.hasConflict || candidate.overLimit} onClick={() => createDraft(candidate.id)}>{f("manage.library.createDraft")}</button></div></article>)}
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
  </main>;
}

function LibraryAutomationPanel({ libraryID, report }: { libraryID: number; report: (message: string) => void }) {
  const intl = useIntl();
  const f = (id: string, values?: Record<string, string | number>) => intl.formatMessage({ id }, values);
  const query = useQuery<{ manageLibraryAutomation: ManageLibraryAutomation }>(MANAGE_LIBRARY_AUTOMATION, { variables: { libraryID } });
  const [save, saveState] = useMutation<{ saveLibraryAutomationPolicy: ManageLibraryAutomation }>(SAVE_LIBRARY_AUTOMATION_POLICY);
  const [run, runState] = useMutation<{ runLibraryAutomation: ManageLibraryAutomationRun }>(RUN_LIBRARY_AUTOMATION);
  const [cancel, cancelState] = useMutation(CANCEL_LIBRARY_AUTOMATION);
  const [draft, setDraft] = useState<ManageLibraryAutomationPolicy | null>(null);
  useEffect(() => {
    if (query.data?.manageLibraryAutomation.policy) setDraft(query.data.manageLibraryAutomation.policy);
  }, [query.data]);
  const activeRun = query.data?.manageLibraryAutomation.recentRuns.find((value) => value.status === "QUEUED" || value.status === "RUNNING") ?? null;
  useEffect(() => {
    if (activeRun) query.startPolling(1500);
    else query.stopPolling();
    return () => query.stopPolling();
  }, [activeRun?.id, activeRun?.status, query.startPolling, query.stopPolling]);
  if (query.loading || draft === null) return <section className="rule-section automation-section"><p>{f("manage.automation.loading")}</p></section>;
  if (query.error) return <section className="rule-section automation-section"><p role="alert">{query.error.message}</p></section>;
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
      if (result.data) setDraft(result.data.saveLibraryAutomationPolicy.policy);
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
  const canRun = draft.revision > 0 && draft.mode !== "MANUAL" && activeRun === null;
  const autoActivationValid = draft.mode !== "TRUSTED" || !draft.autoActivate || Boolean(draft.defaultContentRating);
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
