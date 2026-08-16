import { useMutation, useQuery } from "@apollo/client/react";
import { type FormEvent, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { CREATE_MEDIA_LIBRARY, CREATE_RECOGNITION_RULE, DELETE_RECOGNITION_RULE, DISCOVER_MEDIA_LIBRARY, IMPORT_GALLERY_CANDIDATE, MANAGE_DISCOVERY, MANAGE_LIBRARIES, UPDATE_RECOGNITION_RULE } from "../api/manage";
import type { ManageDiscoverySnapshot, ManageGalleryDetail, ManageLibrary, ManageRecognitionRule } from "./types";
import { MediaClassificationRules } from "./MediaClassificationRules";

const emptyLibrary = { name: "", rootPath: "", enabled: true, readOnly: true, captureTimezone: "UTC" };
type RuleDraft = Omit<ManageRecognitionRule, "id">;
const emptyRule: RuleDraft = { name: "Direct child", kind: "FIXED_DEPTH", enabled: false, autoCreateDraft: false, order: 100, pattern: "", fixedDepth: 1 };

export function ManageLibrariesPage() {
  const navigate = useNavigate();
  const librariesQuery = useQuery<{ manageLibraries: ManageLibrary[] }>(MANAGE_LIBRARIES);
  const libraries = librariesQuery.data?.manageLibraries ?? [];
  const [selectedID, setSelectedID] = useState<number | null>(null);
  const [libraryDraft, setLibraryDraft] = useState(emptyLibrary);
  const [ruleDraft, setRuleDraft] = useState(emptyRule);
  const [editingRuleID, setEditingRuleID] = useState<number | null>(null);
  const [deletingRuleID, setDeletingRuleID] = useState<number | null>(null);
  const [message, setMessage] = useState("");
  const discoveryQuery = useQuery<{ manageDiscovery: ManageDiscoverySnapshot }>(MANAGE_DISCOVERY, { variables: { libraryID: selectedID ?? 0 }, skip: selectedID === null });
  const [createLibrary, createLibraryState] = useMutation<{ createMediaLibrary: ManageLibrary }>(CREATE_MEDIA_LIBRARY);
  const [createRule, createRuleState] = useMutation(CREATE_RECOGNITION_RULE);
  const [updateRule, updateRuleState] = useMutation(UPDATE_RECOGNITION_RULE);
  const [deleteRule] = useMutation(DELETE_RECOGNITION_RULE);
  const [discover, discoveryState] = useMutation<{ discoverMediaLibrary: ManageDiscoverySnapshot }>(DISCOVER_MEDIA_LIBRARY);
  const [importCandidate] = useMutation<{ importGalleryCandidate: ManageGalleryDetail }>(IMPORT_GALLERY_CANDIDATE);
  useEffect(() => { if (selectedID === null && libraries[0]) setSelectedID(libraries[0].id); }, [libraries, selectedID]);
  const snapshot = discoveryState.data?.discoverMediaLibrary ?? discoveryQuery.data?.manageDiscovery;
  const libraryValid = libraryDraft.name.trim() !== "" && libraryDraft.rootPath.startsWith("/") && libraryDraft.captureTimezone.trim() !== "";

  async function addLibrary(event: FormEvent) {
    event.preventDefault(); if (!libraryValid) return; setMessage("");
    try { const result = await createLibrary({ variables: { input: libraryDraft } }); if (!result.data) throw new Error("Server did not return the created media library"); setSelectedID(result.data.createMediaLibrary.id); setLibraryDraft(emptyLibrary); await librariesQuery.refetch(); }
    catch (error) { setMessage(error instanceof Error ? error.message : "Unable to create media library"); }
  }
  function resetRuleEditor() {
    setRuleDraft(emptyRule);
    setEditingRuleID(null);
  }
  async function saveRule(event: FormEvent) {
    event.preventDefault(); if (selectedID === null) return; setMessage("");
    const normalized = { ...ruleDraft, pattern: ruleDraft.kind === "PATH_TEMPLATE" ? ruleDraft.pattern : "", fixedDepth: ruleDraft.kind === "FIXED_DEPTH" ? ruleDraft.fixedDepth : 0 };
    try {
      if (editingRuleID === null) await createRule({ variables: { input: { ...normalized, libraryID: selectedID } } });
      else await updateRule({ variables: { input: { ...normalized, id: editingRuleID } } });
      resetRuleEditor(); await librariesQuery.refetch(); setMessage(editingRuleID === null ? "Recognition rule added. Scan the library to refresh discovery." : "Recognition rule updated. Scan the library to refresh discovery.");
    }
    catch (error) { setMessage(error instanceof Error ? error.message : "Unable to save recognition rule"); }
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
      setDeletingRuleID(null); await librariesQuery.refetch(); setMessage("Recognition rule deleted. Existing Galleries are unchanged; scan the library to refresh discovery.");
    }
    catch (error) { setMessage(error instanceof Error ? error.message : "Unable to delete recognition rule"); }
  }
  async function runDiscovery() {
    if (selectedID === null) return; setMessage("");
    try { await discover({ variables: { libraryID: selectedID } }); }
    catch (error) { setMessage(error instanceof Error ? error.message : "Discovery failed"); }
  }
  async function createDraft(candidateID: number) {
    setMessage("");
    try { const result = await importCandidate({ variables: { candidateID } }); const setID = result.data?.importGalleryCandidate.row.setID; if (setID) navigate(`/manage/gallery/${setID}`); }
    catch (error) { setMessage(error instanceof Error ? error.message : "Candidate cannot create a DRAFT"); }
  }

  return <main className="manage-page"><header className="manage-heading"><div><p>STORAGE ROOTS</p><h2>Libraries & import</h2></div><button type="button" disabled={selectedID === null || discoveryState.loading} onClick={runDiscovery}>{discoveryState.loading ? "Scanning…" : "Scan selected library"}</button></header>
    {message ? <p className="manage-message" role="status">{message}</p> : null}
    <section className="library-layout"><aside className="library-list"><h3>Media libraries</h3>{libraries.map((library) => <button type="button" className={library.id === selectedID ? "is-active" : ""} key={library.id} onClick={() => { setSelectedID(library.id); resetRuleEditor(); setDeletingRuleID(null); }}><strong>{library.name}</strong><span>{library.rootPath}</span></button>)}
      <details><summary>Add media library</summary><form onSubmit={addLibrary}><label>Name (required)<input required value={libraryDraft.name} onChange={(event) => setLibraryDraft({ ...libraryDraft, name: event.target.value })} /></label><label>Absolute path (required)<input required value={libraryDraft.rootPath} onChange={(event) => setLibraryDraft({ ...libraryDraft, rootPath: event.target.value })} /></label><label>Capture timezone (required)<input required value={libraryDraft.captureTimezone} onChange={(event) => setLibraryDraft({ ...libraryDraft, captureTimezone: event.target.value })} /></label><label className="check"><input type="checkbox" checked={libraryDraft.readOnly} onChange={(event) => setLibraryDraft({ ...libraryDraft, readOnly: event.target.checked })} /> Read-only source</label><button type="submit" disabled={!libraryValid || createLibraryState.loading}>{createLibraryState.loading ? "Creating…" : "Create"}</button></form></details></aside>
      <div className="library-workspace">{selectedID === null ? <p className="state-message">Create a media library first.</p> : <><LibraryRules library={libraries.find((value) => value.id === selectedID)} draft={ruleDraft} setDraft={setRuleDraft} submit={saveRule} saving={createRuleState.loading || updateRuleState.loading} editingRuleID={editingRuleID} deletingRuleID={deletingRuleID} editRule={editRule} cancelEdit={resetRuleEditor} requestDelete={setDeletingRuleID} deleteRule={removeRule} />
        <MediaClassificationRules libraryID={selectedID} />
        <section className="candidate-section"><header><h3>Latest discovery</h3><span>{snapshot?.completedAt || "Not scanned"}</span></header>{snapshot?.candidates.map((candidate) => <article className={`candidate-card ${candidate.hasConflict || candidate.overLimit ? "has-issue" : ""}`} key={candidate.id}><div><strong>{candidate.rootPath}</strong><p>{candidate.sourceType} · {candidate.method} · {candidate.mediaCount} media</p>{candidate.suggestions.length ? <ul>{candidate.suggestions.map((suggestion) => <li key={`${suggestion.field}:${suggestion.value}`}>{suggestion.field}: {suggestion.value}</li>)}</ul> : null}</div><div><span>{candidate.status}</span><button type="button" disabled={candidate.status !== "PENDING" || candidate.hasConflict || candidate.overLimit} onClick={() => createDraft(candidate.id)}>Create DRAFT</button></div></article>)}
          {snapshot && snapshot.candidates.length === 0 ? <p className="state-message">No deterministic candidates.</p> : null}
          {snapshot?.unassigned.length ? <details className="unassigned"><summary>Unassigned media ({snapshot.unassigned.length})</summary>{snapshot.unassigned.map((item) => <p key={item.parentPath}><span>{item.parentPath}</span><strong>{item.mediaCount}</strong></p>)}</details> : null}</section></>}</div></section></main>;
}

function LibraryRules({ library, draft, setDraft, submit, saving, editingRuleID, deletingRuleID, editRule, cancelEdit, requestDelete, deleteRule }: {
  library?: ManageLibrary; draft: RuleDraft; setDraft: (value: RuleDraft) => void; submit: (event: FormEvent) => void;
  saving: boolean;
  editingRuleID: number | null; deletingRuleID: number | null; editRule: (rule: ManageRecognitionRule) => void;
  cancelEdit: () => void; requestDelete: (id: number | null) => void; deleteRule: (id: number) => void;
}) {
  if (!library) return null;
  const ruleValid = draft.name.trim() !== "" &&
    (draft.kind !== "PATH_TEMPLATE" || draft.pattern.trim() !== "") &&
    (draft.kind !== "FIXED_DEPTH" || (draft.fixedDepth >= 1 && draft.fixedDepth <= 64));
  return <section className="rule-section"><header><div><h3>{library.name}</h3><p>{library.rootPath}</p></div><button type="button" onClick={() => navigator.clipboard?.writeText(library.rootPath)}>Copy path</button></header><div className="rule-list">{library.rules.map((rule) => <div key={rule.id}><div><strong>{rule.name}</strong><span>{rule.kind} · {rule.enabled ? "ON" : "OFF"} · auto DRAFT {rule.autoCreateDraft ? "ON" : "OFF"}</span></div><div className="rule-actions">{deletingRuleID === rule.id ? <><button type="button" onClick={() => requestDelete(null)}>Cancel</button><button type="button" className="danger" onClick={() => deleteRule(rule.id)}>Confirm delete</button></> : <><button type="button" onClick={() => editRule(rule)}>Edit</button><button type="button" onClick={() => requestDelete(rule.id)}>Delete</button></>}</div></div>)}</div>
    <details key={editingRuleID ?? "new"} open={editingRuleID !== null ? true : undefined}><summary>{editingRuleID === null ? "Add deterministic rule" : `Edit rule #${editingRuleID}`}</summary><form className="rule-form" onSubmit={submit}><label>Name (required)<input value={draft.name} required onChange={(event) => setDraft({ ...draft, name: event.target.value })} /></label><label>Mode<select value={draft.kind} onChange={(event) => setDraft({ ...draft, kind: event.target.value as RuleDraft["kind"] })}><option value="FIXED_DEPTH">DIRECT_CHILD / FIXED_DEPTH</option><option value="MARKER">.cosplay-root MARKER</option><option value="PATH_TEMPLATE">PATH_TEMPLATE (RE2)</option></select></label><label>Order<input type="number" value={draft.order} onChange={(event) => setDraft({ ...draft, order: Number(event.target.value) })} /></label>{draft.kind === "FIXED_DEPTH" ? <label>Depth<input type="number" min="1" max="64" value={draft.fixedDepth} onChange={(event) => setDraft({ ...draft, fixedDepth: Number(event.target.value) })} /></label> : null}{draft.kind === "PATH_TEMPLATE" ? <label className="wide">Full relative path RE2 (required)<input value={draft.pattern} required onChange={(event) => setDraft({ ...draft, pattern: event.target.value })} /></label> : null}<label className="check"><input type="checkbox" checked={draft.enabled} onChange={(event) => setDraft({ ...draft, enabled: event.target.checked })} /> Enable rule</label><label className="check"><input type="checkbox" checked={draft.autoCreateDraft} onChange={(event) => setDraft({ ...draft, autoCreateDraft: event.target.checked })} /> Auto-create DRAFT</label><div className="rule-form-actions"><button type="submit" disabled={!ruleValid || saving}>{saving ? "Saving…" : editingRuleID === null ? "Add rule" : "Save rule"}</button>{editingRuleID !== null ? <button type="button" onClick={cancelEdit}>Cancel edit</button> : null}</div></form></details></section>;
}
