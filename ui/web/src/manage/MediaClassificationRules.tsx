import { useMutation, useQuery } from "@apollo/client/react";
import { type FormEvent, useEffect, useMemo, useState } from "react";
import { CREATE_MEDIA_CLASSIFICATION_RULE, DELETE_MEDIA_CLASSIFICATION_RULE, EVALUATE_MEDIA_CLASSIFICATION_RULES, MANAGE_MEDIA_CLASSIFICATION_RULES, MANAGE_MEDIA_CLASSIFICATION_SUGGESTIONS, PREVIEW_MEDIA_CLASSIFICATION_RULE, RESOLVE_MEDIA_CLASSIFICATION_SUGGESTION, RESTORE_DEFAULT_MEDIA_CLASSIFICATION_RULES, TEST_MEDIA_CLASSIFICATION_RULE, UPDATE_MEDIA_CLASSIFICATION_RULE, VALIDATE_MEDIA_CLASSIFICATION_RULE } from "../api/manage";
import type { ManageMediaClassificationPreview, ManageMediaClassificationRule, ManageMediaClassificationSuggestion } from "./types";

type Draft = Omit<ManageMediaClassificationRule, "id" | "revision" | "systemDefault">;
const emptyDraft = (libraryID: number): Draft => ({ libraryID, name: "Selfie folders", enabled: true, order: 100, subject: "PARENT_FOLDER", operator: "EXACT", pattern: "selfie\n自拍", caseSensitive: false, resultCategory: "SELFIE" });

export function MediaClassificationRules({ libraryID }: { libraryID: number }) {
  const rulesQuery = useQuery<{ manageMediaClassificationRules: ManageMediaClassificationRule[] }>(MANAGE_MEDIA_CLASSIFICATION_RULES, { variables: { libraryID } });
  const suggestionsQuery = useQuery<{ manageMediaClassificationSuggestions: ManageMediaClassificationSuggestion[] }>(MANAGE_MEDIA_CLASSIFICATION_SUGGESTIONS, { variables: { libraryID, status: "PENDING" } });
  const [createRule, createState] = useMutation(CREATE_MEDIA_CLASSIFICATION_RULE);
  const [updateRule, updateState] = useMutation(UPDATE_MEDIA_CLASSIFICATION_RULE);
  const [deleteRule] = useMutation(DELETE_MEDIA_CLASSIFICATION_RULE);
  const [restoreDefaults, restoreState] = useMutation(RESTORE_DEFAULT_MEDIA_CLASSIFICATION_RULES);
  const [validateRule, validateState] = useMutation<{ validateMediaClassificationRule: { valid: boolean; errorCode: string; message: string } }>(VALIDATE_MEDIA_CLASSIFICATION_RULE);
  const [testRule] = useMutation<{ testMediaClassificationRule: { matched: boolean; resultCategory?: string | null; subject: string; matchedValue: string } }>(TEST_MEDIA_CLASSIFICATION_RULE);
  const [previewRule, previewState] = useMutation<{ previewMediaClassificationRule: ManageMediaClassificationPreview }>(PREVIEW_MEDIA_CLASSIFICATION_RULE);
  const [evaluateRules, evaluateState] = useMutation<{ evaluateMediaClassificationRules: { evaluated: number; matched: number; pending: number; superseded: number } }>(EVALUATE_MEDIA_CLASSIFICATION_RULES);
  const [resolveSuggestion] = useMutation<{ resolveMediaClassificationSuggestion: { id: number; galleryID: number; galleryRevision: number; status: string } }>(RESOLVE_MEDIA_CLASSIFICATION_SUGGESTION);
  const [draft, setDraft] = useState<Draft>(() => emptyDraft(libraryID));
  const [editingID, setEditingID] = useState<number | null>(null);
  const [validatedKey, setValidatedKey] = useState<string | null>(null);
  const [validation, setValidation] = useState<{ valid: boolean; errorCode: string; message: string } | null>(null);
  const [samplePath, setSamplePath] = useState("selfie/IMG_0001.jpg");
  const [testResult, setTestResult] = useState("");
  const [preview, setPreview] = useState<ManageMediaClassificationPreview | null>(null);
  const [message, setMessage] = useState("");
  useEffect(() => { setDraft(emptyDraft(libraryID)); setEditingID(null); setValidatedKey(null); setValidation(null); setPreview(null); }, [libraryID]);

  const input = useMemo(() => ({ libraryID: draft.libraryID ?? null, name: draft.name, enabled: draft.enabled, order: draft.order, subject: draft.subject, operator: draft.operator, pattern: draft.pattern, caseSensitive: draft.caseSensitive, resultCategory: draft.resultCategory }), [draft]);
  const validationKey = JSON.stringify(input);
  const baseValid = draft.name.trim() !== "" && draft.pattern.trim() !== "" && draft.pattern.length <= 4000;
  const regexValidated = draft.operator !== "RE2" || (validatedKey === validationKey && validation?.valid === true);
  const saveEnabled = baseValid && regexValidated && !createState.loading && !updateState.loading;

  function change(patch: Partial<Draft>) {
    setDraft((current) => ({ ...current, ...patch }));
    setValidatedKey(null); setValidation(null); setPreview(null); setTestResult("");
  }
  function reset() { setDraft(emptyDraft(libraryID)); setEditingID(null); setValidatedKey(null); setValidation(null); setPreview(null); }
  function edit(rule: ManageMediaClassificationRule) {
    setDraft({ libraryID: rule.libraryID, name: rule.name, enabled: rule.enabled, order: rule.order, subject: rule.subject, operator: rule.operator, pattern: rule.pattern, caseSensitive: rule.caseSensitive, resultCategory: rule.resultCategory });
    setEditingID(rule.id); setValidatedKey(null); setValidation(null); setPreview(null); setMessage("");
  }
  async function runValidation() {
    setMessage("");
    try {
      const result = await validateRule({ variables: { input } });
      const checked = result.data?.validateMediaClassificationRule;
      if (!checked) throw new Error("Rule validator returned no result");
      setValidation(checked); setValidatedKey(validationKey);
    } catch (error) { setValidation(null); setValidatedKey(null); setMessage(error instanceof Error ? error.message : "Validation failed"); }
  }
  async function save(event: FormEvent) {
    event.preventDefault(); if (!saveEnabled) return; setMessage("");
    try {
      if (editingID === null) await createRule({ variables: { input } });
      else await updateRule({ variables: { input: { ...input, id: editingID } } });
      await rulesQuery.refetch(); reset(); setMessage("Media classification rule saved. Existing media is unchanged until evaluation runs.");
    } catch (error) { setMessage(error instanceof Error ? error.message : "Unable to save media classification rule"); }
  }
  async function remove(id: number) {
    setMessage("");
    try { await deleteRule({ variables: { id } }); await rulesQuery.refetch(); if (editingID === id) reset(); setMessage("Rule deleted. Existing media categories are unchanged."); }
    catch (error) { setMessage(error instanceof Error ? error.message : "Unable to delete rule"); }
  }
  async function restore() {
    setMessage("");
    try { await restoreDefaults(); await rulesQuery.refetch(); setMessage("Default media classification rules restored. Existing media is unchanged until evaluation runs."); }
    catch (error) { setMessage(error instanceof Error ? error.message : "Unable to restore defaults"); }
  }
  async function testSample() {
    setMessage("");
    try { const result = await testRule({ variables: { input, relativePath: samplePath } }); const match = result.data?.testMediaClassificationRule; setTestResult(match?.matched ? `Matched ${match.matchedValue} → ${match.resultCategory}` : "No match"); }
    catch (error) { setTestResult(""); setMessage(error instanceof Error ? error.message : "Test failed"); }
  }
  async function runPreview() {
    setMessage("");
    try { const result = await previewRule({ variables: { input, libraryID } }); setPreview(result.data?.previewMediaClassificationRule ?? null); }
    catch (error) { setPreview(null); setMessage(error instanceof Error ? error.message : "Preview failed"); }
  }
  async function evaluate() {
    setMessage("");
    try { const result = await evaluateRules({ variables: { libraryID } }); const value = result.data?.evaluateMediaClassificationRules; await suggestionsQuery.refetch(); setMessage(value ? `Evaluated ${value.evaluated}; matched ${value.matched}; created ${value.pending}; superseded ${value.superseded}.` : "Evaluation completed."); }
    catch (error) { setMessage(error instanceof Error ? error.message : "Evaluation failed"); }
  }
  async function resolve(item: ManageMediaClassificationSuggestion, accept: boolean) {
    setMessage("");
    try { await resolveSuggestion({ variables: { id: item.id, accept, expectedGalleryRevision: item.galleryRevision } }); await suggestionsQuery.refetch(); setMessage(accept ? "Suggestion accepted." : "Suggestion rejected."); }
    catch (error) { setMessage(error instanceof Error ? error.message : "Unable to resolve suggestion"); }
  }
  async function resolveAll(accept: boolean) {
    const pending = suggestionsQuery.data?.manageMediaClassificationSuggestions ?? [];
    const revisions = new Map<number, number>();
    setMessage("");
    try {
      for (const item of pending) {
        const expected = revisions.get(item.galleryID) ?? item.galleryRevision;
        const result = await resolveSuggestion({ variables: { id: item.id, accept, expectedGalleryRevision: expected } });
        const resolved = result.data?.resolveMediaClassificationSuggestion;
        if (resolved) revisions.set(item.galleryID, resolved.galleryRevision);
      }
      await suggestionsQuery.refetch(); setMessage(`${pending.length} suggestion(s) ${accept ? "accepted" : "rejected"}.`);
    } catch (error) { await suggestionsQuery.refetch(); setMessage(error instanceof Error ? error.message : "Bulk resolution stopped"); }
  }

  const rules = rulesQuery.data?.manageMediaClassificationRules ?? [];
  const suggestions = suggestionsQuery.data?.manageMediaClassificationSuggestions ?? [];
  return <section className="rule-section media-classification"><header><div><h3>Media classification</h3><p>Static-image PHOTO/SELFIE suggestions. Rules never rewrite media until accepted.</p></div><div className="rule-actions"><button type="button" disabled={restoreState.loading} onClick={restore}>{restoreState.loading ? "Restoring…" : "Restore defaults"}</button><button type="button" disabled={evaluateState.loading} onClick={evaluate}>{evaluateState.loading ? "Evaluating…" : "Evaluate existing media"}</button></div></header>
    {message ? <p className="manage-message" role="status">{message}</p> : null}
    <div className="rule-list">{rules.map((rule) => <div key={rule.id}><div><strong>{rule.name}</strong><span>{rule.libraryID == null ? "GLOBAL" : "THIS LIBRARY"} · {rule.subject} / {rule.operator} · {rule.resultCategory} · rev {rule.revision} · {rule.enabled ? "ON" : "OFF"}</span></div><div className="rule-actions"><button type="button" onClick={() => edit(rule)}>Edit</button><button type="button" onClick={() => remove(rule.id)}>Delete</button></div></div>)}</div>
    <details key={editingID ?? "new-classification"} open={editingID !== null ? true : undefined}><summary>{editingID === null ? "Add classification rule" : `Edit classification rule #${editingID}`}</summary><form className="rule-form" onSubmit={save}>
      <label>Name (required)<input required value={draft.name} onChange={(event) => change({ name: event.target.value })} /></label>
      <label>Scope<select value={draft.libraryID == null ? "GLOBAL" : "LIBRARY"} onChange={(event) => change({ libraryID: event.target.value === "GLOBAL" ? null : libraryID })}><option value="LIBRARY">Selected library</option><option value="GLOBAL">All libraries</option></select></label>
      <label>Match subject<select value={draft.subject} onChange={(event) => change({ subject: event.target.value as Draft["subject"] })}><option value="PARENT_FOLDER">Parent folder</option><option value="FILE_NAME">File name</option><option value="FILE_STEM">File stem</option><option value="RELATIVE_PATH">Full relative path</option></select></label>
      <label>Operator<select value={draft.operator} onChange={(event) => change({ operator: event.target.value as Draft["operator"] })}><option value="EXACT">Exact (one per line)</option><option value="GLOB">Glob (one per line)</option><option value="RE2">RE2 expression</option></select></label>
      <label>Result<select value={draft.resultCategory} onChange={(event) => change({ resultCategory: event.target.value as Draft["resultCategory"] })}><option value="SELFIE">SELFIE</option><option value="PHOTO">PHOTO (stop/exclude)</option></select></label>
      <label>Order<input type="number" value={draft.order} onChange={(event) => change({ order: Number(event.target.value) })} /></label>
      <label className="wide">{draft.operator === "RE2" ? "RE2 expression" : "Patterns (one per line)"}<textarea required maxLength={4000} rows={5} value={draft.pattern} onChange={(event) => change({ pattern: event.target.value })} /></label>
      <label className="check"><input type="checkbox" checked={draft.caseSensitive} onChange={(event) => change({ caseSensitive: event.target.checked })} /> Case-sensitive</label><label className="check"><input type="checkbox" checked={draft.enabled} onChange={(event) => change({ enabled: event.target.checked })} /> Enabled</label>
      {draft.operator === "RE2" ? <div className="wide rule-validation"><button type="button" disabled={!baseValid || validateState.loading} onClick={runValidation}>{validateState.loading ? "Validating…" : "Validate RE2"}</button><span className={validation?.valid ? "is-valid" : "is-invalid"}>{validatedKey !== validationKey ? "Validation required before saving." : validation?.valid ? "Valid RE2 expression." : `${validation?.errorCode}: ${validation?.message}`}</span></div> : null}
      <div className="wide rule-test"><input aria-label="Sample relative path" value={samplePath} onChange={(event) => setSamplePath(event.target.value)} /><button type="button" disabled={!baseValid || !samplePath.trim()} onClick={testSample}>Test path</button><button type="button" disabled={!baseValid || previewState.loading} onClick={runPreview}>{previewState.loading ? "Previewing…" : "Preview existing"}</button><span>{testResult}</span></div>
      {preview ? <div className="wide rule-preview"><strong>{preview.totalMatches} match(es); showing up to 200</strong>{preview.samples.slice(0, 20).map((sample) => <p key={sample.itemUUID}>{sample.galleryTitle || sample.gallerySetID} · {sample.relativePath} · {sample.currentCategory} → {sample.proposedCategory}</p>)}</div> : null}
      <div className="rule-form-actions"><button type="submit" disabled={!saveEnabled}>{editingID === null ? "Add rule" : "Save rule"}</button>{editingID !== null ? <button type="button" onClick={reset}>Cancel edit</button> : null}</div>
    </form></details>
    <div className="classification-review"><header><div><h4>Pending suggestions</h4><p>{suggestions.length} awaiting explicit review</p></div>{suggestions.length ? <div className="rule-actions"><button type="button" onClick={() => resolveAll(true)}>Accept all</button><button type="button" onClick={() => resolveAll(false)}>Reject all</button></div> : null}</header>{suggestions.map((item) => <article key={item.id}><div><strong>{item.galleryTitle || item.gallerySetID}</strong><span>{item.relativePath}</span><small>{item.ruleName} rev {item.ruleRevision} · {item.matchedSubject}: {item.matchedValue} → {item.proposedCategory}</small></div><div className="rule-actions"><button type="button" onClick={() => resolve(item, true)}>Accept</button><button type="button" onClick={() => resolve(item, false)}>Reject</button></div></article>)}</div>
  </section>;
}
