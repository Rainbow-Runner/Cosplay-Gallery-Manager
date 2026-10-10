import { useMutation, useQuery } from "@apollo/client/react";
import { type FormEvent, useEffect, useMemo, useState } from "react";
import { useIntl } from "react-intl";
import { CREATE_MEDIA_EXCLUSION_RULE, DELETE_MEDIA_EXCLUSION_RULE, EVALUATE_MEDIA_EXCLUSION_RULES, MANAGE_MEDIA_EXCLUSION_DECISIONS, MANAGE_MEDIA_EXCLUSION_RULES, PREVIEW_MEDIA_EXCLUSION_RULE, RESOLVE_MEDIA_EXCLUSION_DECISION, TEST_MEDIA_EXCLUSION_RULE, UPDATE_MEDIA_EXCLUSION_RULE, VALIDATE_MEDIA_EXCLUSION_RULE } from "../api/manage";
import type { ManageMediaExclusionDecision, ManageMediaExclusionPreview, ManageMediaExclusionRule } from "./types";

type Draft = Omit<ManageMediaExclusionRule, "id" | "revision" | "systemDefault">;
type LibraryTarget = { id: number; name: string } | null;

const emptyDraft = (name: string): Draft => ({ libraryID: null, name, enabled: true, order: 100, subject: "PARENT_FOLDER", operator: "EXACT", pattern: "", caseSensitive: false, mediaKind: "ALL", decision: "EXCLUDE" });

export function MediaExclusionRules({ library }: { library: LibraryTarget }) {
  const intl = useIntl();
  const f = (id: string, values?: Record<string, string | number>) => intl.formatMessage({ id }, values);
  const libraryID = library?.id ?? null;
  const defaultName = f("manage.exclusion.defaultRuleName");
  const rulesQuery = useQuery<{ manageMediaExclusionRules: ManageMediaExclusionRule[] }>(MANAGE_MEDIA_EXCLUSION_RULES, { variables: { libraryID } });
  const decisionsQuery = useQuery<{ manageMediaExclusionDecisions: ManageMediaExclusionDecision[] }>(MANAGE_MEDIA_EXCLUSION_DECISIONS, { variables: { libraryID, status: "PENDING" }, skip: libraryID === null });
  const [createRule, createState] = useMutation(CREATE_MEDIA_EXCLUSION_RULE);
  const [updateRule, updateState] = useMutation(UPDATE_MEDIA_EXCLUSION_RULE);
  const [deleteRule] = useMutation(DELETE_MEDIA_EXCLUSION_RULE);
  const [validateRule, validateState] = useMutation<{ validateMediaExclusionRule: { valid: boolean; errorCode: string; message: string } }>(VALIDATE_MEDIA_EXCLUSION_RULE);
  const [testRule] = useMutation<{ testMediaExclusionRule: { matched: boolean; decision: string; matchedValue: string } }>(TEST_MEDIA_EXCLUSION_RULE);
  const [previewRule, previewState] = useMutation<{ previewMediaExclusionRule: ManageMediaExclusionPreview }>(PREVIEW_MEDIA_EXCLUSION_RULE);
  const [evaluateRules, evaluateState] = useMutation<{ evaluateMediaExclusionRules: { evaluated: number; matched: number; pending: number; superseded: number } }>(EVALUATE_MEDIA_EXCLUSION_RULES);
  const [resolveDecision] = useMutation<{ resolveMediaExclusionDecision: { id: number; galleryID: number; galleryRevision: number; status: string } }>(RESOLVE_MEDIA_EXCLUSION_DECISION);
  const [draft, setDraft] = useState<Draft>(() => emptyDraft(defaultName));
  const [editingID, setEditingID] = useState<number | null>(null);
  const [deletingID, setDeletingID] = useState<number | null>(null);
  const [validatedKey, setValidatedKey] = useState<string | null>(null);
  const [validation, setValidation] = useState<{ valid: boolean; errorCode: string; message: string } | null>(null);
  const [samplePath, setSamplePath] = useState("extras/cover.jpg");
  const [sampleKind, setSampleKind] = useState<Draft["mediaKind"]>("STATIC_IMAGE");
  const [testResult, setTestResult] = useState("");
  const [preview, setPreview] = useState<ManageMediaExclusionPreview | null>(null);
  const [message, setMessage] = useState("");

  useEffect(() => {
    setDraft(emptyDraft(defaultName));
    setEditingID(null);
    setDeletingID(null);
    setValidatedKey(null);
    setValidation(null);
    setPreview(null);
  }, [defaultName, libraryID]);

  const input = useMemo(() => ({ libraryID: draft.libraryID ?? null, name: draft.name, enabled: draft.enabled, order: draft.order, subject: draft.subject, operator: draft.operator, pattern: draft.pattern, caseSensitive: draft.caseSensitive, mediaKind: draft.mediaKind, decision: draft.decision }), [draft]);
  const validationKey = JSON.stringify(input);
  const baseValid = draft.name.trim() !== "" && draft.pattern.trim() !== "" && draft.pattern.length <= 4000;
  const regexValidated = draft.operator !== "RE2" || (validatedKey === validationKey && validation?.valid === true);
  const saveEnabled = baseValid && regexValidated && !createState.loading && !updateState.loading;

  function change(patch: Partial<Draft>) {
    setDraft((current) => ({ ...current, ...patch }));
    setValidatedKey(null);
    setValidation(null);
    setPreview(null);
    setTestResult("");
  }
  function reset() {
    setDraft(emptyDraft(defaultName));
    setEditingID(null);
    setDeletingID(null);
    setValidatedKey(null);
    setValidation(null);
    setPreview(null);
  }
  function edit(rule: ManageMediaExclusionRule) {
    setDraft({ libraryID: rule.libraryID, name: rule.name, enabled: rule.enabled, order: rule.order, subject: rule.subject, operator: rule.operator, pattern: rule.pattern, caseSensitive: rule.caseSensitive, mediaKind: rule.mediaKind, decision: rule.decision });
    setEditingID(rule.id);
    setDeletingID(null);
    setValidatedKey(null);
    setValidation(null);
    setPreview(null);
    setMessage("");
  }
  async function runValidation() {
    setMessage("");
    try {
      const result = await validateRule({ variables: { input } });
      const checked = result.data?.validateMediaExclusionRule;
      if (!checked) throw new Error(f("manage.exclusion.error.noValidation"));
      setValidation(checked);
      setValidatedKey(validationKey);
    } catch (error) {
      setValidation(null);
      setValidatedKey(null);
      setMessage(error instanceof Error ? error.message : f("manage.exclusion.error.validation"));
    }
  }
  async function save(event: FormEvent) {
    event.preventDefault();
    if (!saveEnabled) return;
    setMessage("");
    try {
      if (editingID === null) await createRule({ variables: { input } });
      else await updateRule({ variables: { input: { ...input, id: editingID } } });
      await rulesQuery.refetch();
      reset();
      setMessage(f("manage.exclusion.saved"));
    } catch (error) {
      setMessage(error instanceof Error ? error.message : f("manage.exclusion.error.save"));
    }
  }
  async function remove(id: number) {
    setMessage("");
    try {
      await deleteRule({ variables: { id } });
      await rulesQuery.refetch();
      setDeletingID(null);
      if (editingID === id) reset();
      setMessage(f("manage.exclusion.deleted"));
    } catch (error) {
      setMessage(error instanceof Error ? error.message : f("manage.exclusion.error.delete"));
    }
  }
  async function testSample() {
    setMessage("");
    try {
      const result = await testRule({ variables: { input, relativePath: samplePath, mediaKind: sampleKind } });
      const match = result.data?.testMediaExclusionRule;
      setTestResult(match?.matched ? f("manage.exclusion.testMatched", { value: match.matchedValue, decision: match.decision }) : f("manage.exclusion.noMatch"));
    } catch (error) {
      setTestResult("");
      setMessage(error instanceof Error ? error.message : f("manage.exclusion.error.test"));
    }
  }
  async function runPreview() {
    if (libraryID === null) return;
    setMessage("");
    try {
      const result = await previewRule({ variables: { input: { ...input, id: editingID }, libraryID } });
      setPreview(result.data?.previewMediaExclusionRule ?? null);
    } catch (error) {
      setPreview(null);
      setMessage(error instanceof Error ? error.message : f("manage.exclusion.error.preview"));
    }
  }
  async function evaluate() {
    if (libraryID === null) return;
    setMessage("");
    try {
      const result = await evaluateRules({ variables: { libraryID } });
      const value = result.data?.evaluateMediaExclusionRules;
      await decisionsQuery.refetch();
      setMessage(value ? f("manage.exclusion.evaluated", value) : f("manage.exclusion.evaluationComplete"));
    } catch (error) {
      setMessage(error instanceof Error ? error.message : f("manage.exclusion.error.evaluate"));
    }
  }
  async function resolve(item: ManageMediaExclusionDecision, accept: boolean) {
    setMessage("");
    try {
      await resolveDecision({ variables: { id: item.id, accept, expectedGalleryRevision: item.galleryRevision } });
      await decisionsQuery.refetch();
      setMessage(f(accept ? "manage.exclusion.accepted" : "manage.exclusion.rejected"));
    } catch (error) {
      setMessage(error instanceof Error ? error.message : f("manage.exclusion.error.resolve"));
    }
  }
  async function resolveAll(accept: boolean) {
    const pending = decisionsQuery.data?.manageMediaExclusionDecisions ?? [];
    const revisions = new Map<number, number>();
    setMessage("");
    try {
      for (const item of pending) {
        const expected = revisions.get(item.galleryID) ?? item.galleryRevision;
        const result = await resolveDecision({ variables: { id: item.id, accept, expectedGalleryRevision: expected } });
        const resolved = result.data?.resolveMediaExclusionDecision;
        if (resolved) revisions.set(item.galleryID, resolved.galleryRevision);
      }
      await decisionsQuery.refetch();
      setMessage(f(accept ? "manage.exclusion.acceptedAll" : "manage.exclusion.rejectedAll", { count: pending.length }));
    } catch (error) {
      await decisionsQuery.refetch();
      setMessage(error instanceof Error ? error.message : f("manage.exclusion.error.resolveAll"));
    }
  }

  const rules = rulesQuery.data?.manageMediaExclusionRules ?? [];
  const globalRules = rules.filter((rule) => rule.libraryID == null);
  const libraryRules = rules.filter((rule) => rule.libraryID != null);
  const decisions = decisionsQuery.data?.manageMediaExclusionDecisions ?? [];
  const subjectLabel = (value: Draft["subject"]) => f(`manage.exclusion.subject.${value}`);
  const operatorLabel = (value: Draft["operator"]) => f(`manage.exclusion.operator.${value}`);
  const mediaKindLabel = (value: Draft["mediaKind"]) => f(`manage.exclusion.mediaKind.${value}`);
  const decisionLabel = (value: Draft["decision"]) => f(`manage.exclusion.decision.${value}`);

  function ruleGroup(title: string, description: string, items: ManageMediaExclusionRule[], scope: "global" | "library") {
    return <section className="classification-rule-group" aria-label={title}>
      <header><div><h4>{title}</h4><p>{description}</p></div></header>
      <div className="rule-list">{items.map((rule) => <div key={rule.id}><div><strong>{rule.name}</strong><span><span className={`classification-scope-badge is-${scope}`}>{f(scope === "global" ? "manage.exclusion.globalBadge" : "manage.exclusion.libraryBadge")}</span> · {decisionLabel(rule.decision)} · {subjectLabel(rule.subject)} / {operatorLabel(rule.operator)} · {mediaKindLabel(rule.mediaKind)} · {f("manage.exclusion.revision", { revision: rule.revision })} · {f(rule.enabled ? "manage.exclusion.on" : "manage.exclusion.off")}</span></div><div className="rule-actions"><button type="button" onClick={() => edit(rule)}>{f("manage.exclusion.edit")}</button>{deletingID === rule.id ? <><button type="button" className="danger" onClick={() => remove(rule.id)}>{f("manage.exclusion.confirmDelete")}</button><button type="button" onClick={() => setDeletingID(null)}>{f("manage.exclusion.cancelDelete")}</button></> : <button type="button" onClick={() => setDeletingID(rule.id)}>{f("manage.exclusion.delete")}</button>}</div></div>)}</div>
      {items.length === 0 ? <p className="classification-empty">{f(scope === "global" ? "manage.exclusion.noGlobalRules" : "manage.exclusion.noLibraryRules")}</p> : null}
    </section>;
  }

  return <section className="rule-section media-classification media-exclusion">
    <header><div><p className="manage-eyebrow">{f("manage.exclusion.eyebrow")}</p><h3>{f("manage.exclusion.title")}</h3><p>{f("manage.exclusion.summary")}</p></div></header>
    <div className="classification-scope-guide"><strong>{f("manage.exclusion.howScopeWorks")}</strong><p>{library ? f("manage.exclusion.scopeFormula", { library: library.name }) : f("manage.exclusion.scopeNoLibrary")}</p><p>{f("manage.exclusion.firstMatch")}</p></div>
    {message ? <p className="manage-message" role="status">{message}</p> : null}
    <div className="classification-rule-groups">
      {ruleGroup(f("manage.exclusion.globalTitle"), f("manage.exclusion.globalSummary"), globalRules, "global")}
      {ruleGroup(library ? f("manage.exclusion.overrideTitle", { library: library.name }) : f("manage.exclusion.overrideTitleEmpty"), library ? f("manage.exclusion.overrideSummary", { library: library.name }) : f("manage.exclusion.overrideSummaryEmpty"), libraryRules, "library")}
    </div>
    <details key={editingID ?? "new-exclusion"} open={editingID !== null ? true : undefined}><summary>{f(editingID === null ? "manage.exclusion.add" : "manage.exclusion.editNumber", editingID === null ? undefined : { id: editingID })}</summary><form className="rule-form" onSubmit={save}>
      <label>{f("manage.exclusion.nameRequired")}<input required value={draft.name} onChange={(event) => change({ name: event.target.value })} /></label>
      <label>{f("manage.exclusion.scope")}<select value={draft.libraryID == null ? "GLOBAL" : "LIBRARY"} onChange={(event) => change({ libraryID: event.target.value === "GLOBAL" ? null : libraryID })}><option value="GLOBAL">{f("manage.exclusion.scopeGlobal")}</option><option value="LIBRARY" disabled={libraryID === null}>{library ? f("manage.exclusion.scopeLibrary", { library: library.name }) : f("manage.exclusion.scopeLibraryUnavailable")}</option></select></label>
      <label>{f("manage.exclusion.matchSubject")}<select value={draft.subject} onChange={(event) => change({ subject: event.target.value as Draft["subject"] })}>{(["PARENT_FOLDER", "PARENT_PATH", "FILE_NAME", "FILE_STEM", "RELATIVE_PATH"] as Draft["subject"][]).map((value) => <option key={value} value={value}>{subjectLabel(value)}</option>)}</select></label>
      <label>{f("manage.exclusion.operator")}<select value={draft.operator} onChange={(event) => change({ operator: event.target.value as Draft["operator"] })}>{(["EXACT", "GLOB", "RE2"] as Draft["operator"][]).map((value) => <option key={value} value={value}>{operatorLabel(value)}</option>)}</select></label>
      <label>{f("manage.exclusion.mediaKind")}<select value={draft.mediaKind} onChange={(event) => change({ mediaKind: event.target.value as Draft["mediaKind"] })}>{(["ALL", "STATIC_IMAGE", "ANIMATED_IMAGE", "VIDEO"] as Draft["mediaKind"][]).map((value) => <option key={value} value={value}>{mediaKindLabel(value)}</option>)}</select></label>
      <label>{f("manage.exclusion.decision")}<select value={draft.decision} onChange={(event) => change({ decision: event.target.value as Draft["decision"] })}><option value="EXCLUDE">{decisionLabel("EXCLUDE")}</option><option value="INCLUDE">{decisionLabel("INCLUDE")}</option></select></label>
      <label>{f("manage.exclusion.order")}<input type="number" value={draft.order} onChange={(event) => change({ order: Number(event.target.value) })} /></label>
      <label className="wide">{f(draft.operator === "RE2" ? "manage.exclusion.re2Expression" : "manage.exclusion.patterns")}<textarea required maxLength={4000} rows={5} value={draft.pattern} onChange={(event) => change({ pattern: event.target.value })} /></label>
      <label className="check"><input type="checkbox" checked={draft.caseSensitive} onChange={(event) => change({ caseSensitive: event.target.checked })} /> {f("manage.exclusion.caseSensitive")}</label><label className="check"><input type="checkbox" checked={draft.enabled} onChange={(event) => change({ enabled: event.target.checked })} /> {f("manage.exclusion.enabled")}</label>
      {draft.operator === "RE2" ? <div className="wide rule-validation"><button type="button" disabled={!baseValid || validateState.loading} onClick={runValidation}>{f(validateState.loading ? "manage.exclusion.validating" : "manage.exclusion.validate")}</button><span className={validation?.valid ? "is-valid" : "is-invalid"}>{validatedKey !== validationKey ? f("manage.exclusion.validationRequired") : validation?.valid ? f("manage.exclusion.validRe2") : `${validation?.errorCode}: ${validation?.message}`}</span></div> : null}
      <div className="wide rule-test"><input aria-label={f("manage.exclusion.samplePath")} value={samplePath} onChange={(event) => setSamplePath(event.target.value)} /><select aria-label={f("manage.exclusion.sampleKind")} value={sampleKind} onChange={(event) => setSampleKind(event.target.value as Draft["mediaKind"])}>{(["STATIC_IMAGE", "ANIMATED_IMAGE", "VIDEO"] as Draft["mediaKind"][]).map((value) => <option key={value} value={value}>{mediaKindLabel(value)}</option>)}</select><button type="button" disabled={!baseValid || !samplePath.trim()} onClick={testSample}>{f("manage.exclusion.testPath")}</button><button type="button" disabled={libraryID === null || !baseValid || previewState.loading} onClick={runPreview}>{previewState.loading ? f("manage.exclusion.previewing") : library ? f("manage.exclusion.previewIn", { library: library.name }) : f("manage.exclusion.previewUnavailable")}</button><span>{testResult}</span></div>
      {preview ? <div className="wide rule-preview"><strong>{f("manage.exclusion.previewCount", { count: preview.totalMatches })}</strong>{preview.samples.slice(0, 20).map((sample) => <p key={sample.itemUUID}>{sample.galleryTitle || sample.gallerySetID} · {sample.relativePath} · {sample.currentlyExcluded ? f("manage.exclusion.currentExcluded") : f("manage.exclusion.currentIncluded")} → {decisionLabel(sample.proposedDecision)} · {f("manage.exclusion.winningRule", { rule: sample.winningRuleName })}</p>)}</div> : null}
      <div className="rule-form-actions"><button type="submit" disabled={!saveEnabled}>{f(editingID === null ? "manage.exclusion.addRule" : "manage.exclusion.saveRule")}</button>{editingID !== null ? <button type="button" onClick={reset}>{f("manage.exclusion.cancelEdit")}</button> : null}</div>
    </form></details>
    <section className="classification-target"><header><div><h4>{f("manage.exclusion.evaluationTitle")}</h4><p>{library ? f("manage.exclusion.evaluationSummary", { library: library.name }) : f("manage.exclusion.evaluationNoLibrary")}</p></div><button type="button" disabled={libraryID === null || evaluateState.loading} onClick={evaluate}>{evaluateState.loading ? f("manage.exclusion.evaluating") : library ? f("manage.exclusion.evaluateLibrary", { library: library.name }) : f("manage.exclusion.evaluateUnavailable")}</button></header></section>
    <div className="classification-review"><header><div><h4>{f("manage.exclusion.pending")}</h4><p>{library ? f("manage.exclusion.awaiting", { count: decisions.length, library: library.name }) : f("manage.exclusion.selectLibrary")}</p></div>{decisions.length ? <div className="rule-actions"><button type="button" onClick={() => resolveAll(true)}>{f("manage.exclusion.acceptAll")}</button><button type="button" onClick={() => resolveAll(false)}>{f("manage.exclusion.rejectAll")}</button></div> : null}</header>{decisions.map((item) => <article key={item.id}><div><strong>{item.galleryTitle || item.gallerySetID}</strong><span>{item.relativePath}</span><small>{f("manage.exclusion.decisionDetail", { rule: item.ruleName, revision: item.ruleRevision, subject: item.matchedSubject, value: item.matchedValue })}</small></div><div className="rule-actions"><button type="button" onClick={() => resolve(item, true)}>{f("manage.exclusion.accept")}</button><button type="button" onClick={() => resolve(item, false)}>{f("manage.exclusion.reject")}</button></div></article>)}</div>
  </section>;
}
