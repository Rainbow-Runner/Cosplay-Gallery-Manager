import { useMutation, useQuery } from "@apollo/client/react";
import { type FormEvent, useEffect, useMemo, useState } from "react";
import { useIntl } from "react-intl";
import { CREATE_MEDIA_CLASSIFICATION_RULE, DELETE_MEDIA_CLASSIFICATION_RULE, EVALUATE_MEDIA_CLASSIFICATION_RULES, MANAGE_MEDIA_CLASSIFICATION_RULES, MANAGE_MEDIA_CLASSIFICATION_SUGGESTIONS, PREVIEW_MEDIA_CLASSIFICATION_RULE, RESOLVE_MEDIA_CLASSIFICATION_SUGGESTION, RESTORE_DEFAULT_MEDIA_CLASSIFICATION_RULES, TEST_MEDIA_CLASSIFICATION_RULE, UPDATE_MEDIA_CLASSIFICATION_RULE, VALIDATE_MEDIA_CLASSIFICATION_RULE } from "../api/manage";
import type { ManageMediaClassificationPreview, ManageMediaClassificationRule, ManageMediaClassificationSuggestion } from "./types";

type Draft = Omit<ManageMediaClassificationRule, "id" | "revision" | "systemDefault">;
type LibraryTarget = { id: number; name: string } | null;

const emptyDraft = (name: string): Draft => ({ libraryID: null, name, enabled: true, order: 100, subject: "PARENT_FOLDER", operator: "EXACT", pattern: "selfie\n自拍", caseSensitive: false, resultCategory: "SELFIE" });

export function MediaClassificationRules({ library }: { library: LibraryTarget }) {
  const intl = useIntl();
  const f = (id: string, values?: Record<string, string | number>) => intl.formatMessage({ id }, values);
  const defaultRuleName = f("manage.classification.defaultRuleName");
  const libraryID = library?.id ?? null;
  const rulesQuery = useQuery<{ manageMediaClassificationRules: ManageMediaClassificationRule[] }>(MANAGE_MEDIA_CLASSIFICATION_RULES, { variables: { libraryID } });
  const suggestionsQuery = useQuery<{ manageMediaClassificationSuggestions: ManageMediaClassificationSuggestion[] }>(MANAGE_MEDIA_CLASSIFICATION_SUGGESTIONS, { variables: { libraryID, status: "PENDING" }, skip: libraryID === null });
  const [createRule, createState] = useMutation(CREATE_MEDIA_CLASSIFICATION_RULE);
  const [updateRule, updateState] = useMutation(UPDATE_MEDIA_CLASSIFICATION_RULE);
  const [deleteRule] = useMutation(DELETE_MEDIA_CLASSIFICATION_RULE);
  const [restoreDefaults, restoreState] = useMutation(RESTORE_DEFAULT_MEDIA_CLASSIFICATION_RULES);
  const [validateRule, validateState] = useMutation<{ validateMediaClassificationRule: { valid: boolean; errorCode: string; message: string } }>(VALIDATE_MEDIA_CLASSIFICATION_RULE);
  const [testRule] = useMutation<{ testMediaClassificationRule: { matched: boolean; resultCategory?: string | null; subject: string; matchedValue: string } }>(TEST_MEDIA_CLASSIFICATION_RULE);
  const [previewRule, previewState] = useMutation<{ previewMediaClassificationRule: ManageMediaClassificationPreview }>(PREVIEW_MEDIA_CLASSIFICATION_RULE);
  const [evaluateRules, evaluateState] = useMutation<{ evaluateMediaClassificationRules: { evaluated: number; matched: number; pending: number; superseded: number } }>(EVALUATE_MEDIA_CLASSIFICATION_RULES);
  const [resolveSuggestion] = useMutation<{ resolveMediaClassificationSuggestion: { id: number; galleryID: number; galleryRevision: number; status: string } }>(RESOLVE_MEDIA_CLASSIFICATION_SUGGESTION);
  const [draft, setDraft] = useState<Draft>(() => emptyDraft(defaultRuleName));
  const [editingID, setEditingID] = useState<number | null>(null);
  const [validatedKey, setValidatedKey] = useState<string | null>(null);
  const [validation, setValidation] = useState<{ valid: boolean; errorCode: string; message: string } | null>(null);
  const [samplePath, setSamplePath] = useState("selfie/IMG_0001.jpg");
  const [testResult, setTestResult] = useState("");
  const [preview, setPreview] = useState<ManageMediaClassificationPreview | null>(null);
  const [message, setMessage] = useState("");

  useEffect(() => {
    setDraft(emptyDraft(defaultRuleName));
    setEditingID(null);
    setValidatedKey(null);
    setValidation(null);
    setPreview(null);
  }, [defaultRuleName, libraryID]);

  const input = useMemo(() => ({ libraryID: draft.libraryID ?? null, name: draft.name, enabled: draft.enabled, order: draft.order, subject: draft.subject, operator: draft.operator, pattern: draft.pattern, caseSensitive: draft.caseSensitive, resultCategory: draft.resultCategory }), [draft]);
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
    setDraft(emptyDraft(defaultRuleName));
    setEditingID(null);
    setValidatedKey(null);
    setValidation(null);
    setPreview(null);
  }
  function edit(rule: ManageMediaClassificationRule) {
    setDraft({ libraryID: rule.libraryID, name: rule.name, enabled: rule.enabled, order: rule.order, subject: rule.subject, operator: rule.operator, pattern: rule.pattern, caseSensitive: rule.caseSensitive, resultCategory: rule.resultCategory });
    setEditingID(rule.id);
    setValidatedKey(null);
    setValidation(null);
    setPreview(null);
    setMessage("");
  }
  async function runValidation() {
    setMessage("");
    try {
      const result = await validateRule({ variables: { input } });
      const checked = result.data?.validateMediaClassificationRule;
      if (!checked) throw new Error(f("manage.classification.error.noValidation"));
      setValidation(checked);
      setValidatedKey(validationKey);
    } catch (error) {
      setValidation(null);
      setValidatedKey(null);
      setMessage(error instanceof Error ? error.message : f("manage.classification.error.validation"));
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
      setMessage(f("manage.classification.saved"));
    } catch (error) {
      setMessage(error instanceof Error ? error.message : f("manage.classification.error.save"));
    }
  }
  async function remove(id: number) {
    setMessage("");
    try {
      await deleteRule({ variables: { id } });
      await rulesQuery.refetch();
      if (editingID === id) reset();
      setMessage(f("manage.classification.deleted"));
    } catch (error) {
      setMessage(error instanceof Error ? error.message : f("manage.classification.error.delete"));
    }
  }
  async function restore() {
    setMessage("");
    try {
      await restoreDefaults();
      await rulesQuery.refetch();
      setMessage(f("manage.classification.restored"));
    } catch (error) {
      setMessage(error instanceof Error ? error.message : f("manage.classification.error.restore"));
    }
  }
  async function testSample() {
    setMessage("");
    try {
      const result = await testRule({ variables: { input, relativePath: samplePath } });
      const match = result.data?.testMediaClassificationRule;
      setTestResult(match?.matched ? f("manage.classification.testMatched", { value: match.matchedValue, category: match.resultCategory ?? "" }) : f("manage.classification.noMatch"));
    } catch (error) {
      setTestResult("");
      setMessage(error instanceof Error ? error.message : f("manage.classification.error.test"));
    }
  }
  async function runPreview() {
    if (libraryID === null) return;
    setMessage("");
    try {
      const result = await previewRule({ variables: { input, libraryID } });
      setPreview(result.data?.previewMediaClassificationRule ?? null);
    } catch (error) {
      setPreview(null);
      setMessage(error instanceof Error ? error.message : f("manage.classification.error.preview"));
    }
  }
  async function evaluate() {
    if (libraryID === null) return;
    setMessage("");
    try {
      const result = await evaluateRules({ variables: { libraryID } });
      const value = result.data?.evaluateMediaClassificationRules;
      await suggestionsQuery.refetch();
      setMessage(value ? f("manage.classification.evaluated", value) : f("manage.classification.evaluationComplete"));
    } catch (error) {
      setMessage(error instanceof Error ? error.message : f("manage.classification.error.evaluate"));
    }
  }
  async function resolve(item: ManageMediaClassificationSuggestion, accept: boolean) {
    setMessage("");
    try {
      await resolveSuggestion({ variables: { id: item.id, accept, expectedGalleryRevision: item.galleryRevision } });
      await suggestionsQuery.refetch();
      setMessage(f(accept ? "manage.classification.accepted" : "manage.classification.rejected"));
    } catch (error) {
      setMessage(error instanceof Error ? error.message : f("manage.classification.error.resolve"));
    }
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
      await suggestionsQuery.refetch();
      setMessage(f(accept ? "manage.classification.acceptedAll" : "manage.classification.rejectedAll", { count: pending.length }));
    } catch (error) {
      await suggestionsQuery.refetch();
      setMessage(error instanceof Error ? error.message : f("manage.classification.error.resolveAll"));
    }
  }

  const rules = rulesQuery.data?.manageMediaClassificationRules ?? [];
  const globalRules = rules.filter((rule) => rule.libraryID == null);
  const libraryRules = rules.filter((rule) => rule.libraryID != null);
  const suggestions = suggestionsQuery.data?.manageMediaClassificationSuggestions ?? [];
  const subjectLabel = (subject: Draft["subject"]) => f(`manage.classification.subject.${subject}`);
  const operatorLabel = (operator: Draft["operator"]) => f(`manage.classification.operator.${operator}`);

  function ruleGroup(title: string, description: string, items: ManageMediaClassificationRule[], scope: "global" | "library") {
    return <section className="classification-rule-group" aria-label={title}>
      <header><div><h4>{title}</h4><p>{description}</p></div>{scope === "global" ? <button type="button" disabled={restoreState.loading} onClick={restore}>{restoreState.loading ? f("manage.classification.restoring") : f("manage.classification.restore")}</button> : null}</header>
      <div className="rule-list">{items.map((rule) => <div key={rule.id}><div><strong>{rule.name}</strong><span><span className={`classification-scope-badge is-${scope}`}>{scope === "global" ? f("manage.classification.globalBadge") : f("manage.classification.libraryBadge")}</span> · {subjectLabel(rule.subject)} / {operatorLabel(rule.operator)} · {rule.resultCategory} · {f("manage.classification.revision", { revision: rule.revision })} · {rule.enabled ? f("manage.classification.on") : f("manage.classification.off")}</span></div><div className="rule-actions"><button type="button" onClick={() => edit(rule)}>{f("manage.classification.edit")}</button><button type="button" onClick={() => remove(rule.id)}>{f("manage.classification.delete")}</button></div></div>)}</div>
      {items.length === 0 ? <p className="classification-empty">{f(scope === "global" ? "manage.classification.noGlobalRules" : "manage.classification.noLibraryRules")}</p> : null}
    </section>;
  }

  return <section className="rule-section media-classification">
    <header><div><p className="manage-eyebrow">{f("manage.classification.eyebrow")}</p><h3>{f("manage.classification.title")}</h3><p>{f("manage.classification.summary")}</p></div></header>
    <div className="classification-scope-guide"><strong>{f("manage.classification.howScopeWorks")}</strong><p>{library ? f("manage.classification.scopeFormula", { library: library.name }) : f("manage.classification.scopeNoLibrary")}</p></div>
    {message ? <p className="manage-message" role="status">{message}</p> : null}
    <div className="classification-rule-groups">
      {ruleGroup(f("manage.classification.globalTitle"), f("manage.classification.globalSummary"), globalRules, "global")}
      {ruleGroup(library ? f("manage.classification.overrideTitle", { library: library.name }) : f("manage.classification.overrideTitleEmpty"), library ? f("manage.classification.overrideSummary", { library: library.name }) : f("manage.classification.overrideSummaryEmpty"), libraryRules, "library")}
    </div>
    <details key={editingID ?? "new-classification"} open={editingID !== null ? true : undefined}><summary>{editingID === null ? f("manage.classification.add") : f("manage.classification.editNumber", { id: editingID })}</summary><form className="rule-form" onSubmit={save}>
      <label>{f("manage.classification.nameRequired")}<input required value={draft.name} onChange={(event) => change({ name: event.target.value })} /></label>
      <label>{f("manage.classification.scope")}<select value={draft.libraryID == null ? "GLOBAL" : "LIBRARY"} onChange={(event) => change({ libraryID: event.target.value === "GLOBAL" ? null : libraryID })}><option value="GLOBAL">{f("manage.classification.scopeGlobal")}</option><option value="LIBRARY" disabled={libraryID === null}>{library ? f("manage.classification.scopeLibrary", { library: library.name }) : f("manage.classification.scopeLibraryUnavailable")}</option></select></label>
      <label>{f("manage.classification.matchSubject")}<select value={draft.subject} onChange={(event) => change({ subject: event.target.value as Draft["subject"] })}><option value="PARENT_FOLDER">{subjectLabel("PARENT_FOLDER")}</option><option value="FILE_NAME">{subjectLabel("FILE_NAME")}</option><option value="FILE_STEM">{subjectLabel("FILE_STEM")}</option><option value="RELATIVE_PATH">{subjectLabel("RELATIVE_PATH")}</option></select></label>
      <label>{f("manage.classification.operator")}<select value={draft.operator} onChange={(event) => change({ operator: event.target.value as Draft["operator"] })}><option value="EXACT">{operatorLabel("EXACT")}</option><option value="GLOB">{operatorLabel("GLOB")}</option><option value="RE2">{operatorLabel("RE2")}</option></select></label>
      <label>{f("manage.classification.result")}<select value={draft.resultCategory} onChange={(event) => change({ resultCategory: event.target.value as Draft["resultCategory"] })}><option value="SELFIE">SELFIE</option><option value="PHOTO">{f("manage.classification.photoStop")}</option></select></label>
      <label>{f("manage.classification.order")}<input type="number" value={draft.order} onChange={(event) => change({ order: Number(event.target.value) })} /></label>
      <label className="wide">{draft.operator === "RE2" ? f("manage.classification.re2Expression") : f("manage.classification.patterns")}<textarea required maxLength={4000} rows={5} value={draft.pattern} onChange={(event) => change({ pattern: event.target.value })} /></label>
      <label className="check"><input type="checkbox" checked={draft.caseSensitive} onChange={(event) => change({ caseSensitive: event.target.checked })} /> {f("manage.classification.caseSensitive")}</label><label className="check"><input type="checkbox" checked={draft.enabled} onChange={(event) => change({ enabled: event.target.checked })} /> {f("manage.classification.enabled")}</label>
      {draft.operator === "RE2" ? <div className="wide rule-validation"><button type="button" disabled={!baseValid || validateState.loading} onClick={runValidation}>{validateState.loading ? f("manage.classification.validating") : f("manage.classification.validate")}</button><span className={validation?.valid ? "is-valid" : "is-invalid"}>{validatedKey !== validationKey ? f("manage.classification.validationRequired") : validation?.valid ? f("manage.classification.validRe2") : `${validation?.errorCode}: ${validation?.message}`}</span></div> : null}
      <div className="wide rule-test"><input aria-label={f("manage.classification.samplePath")} value={samplePath} onChange={(event) => setSamplePath(event.target.value)} /><button type="button" disabled={!baseValid || !samplePath.trim()} onClick={testSample}>{f("manage.classification.testPath")}</button><button type="button" disabled={libraryID === null || !baseValid || previewState.loading} onClick={runPreview}>{previewState.loading ? f("manage.classification.previewing") : library ? f("manage.classification.previewIn", { library: library.name }) : f("manage.classification.previewUnavailable")}</button><span>{testResult}</span></div>
      {preview ? <div className="wide rule-preview"><strong>{f("manage.classification.previewCount", { count: preview.totalMatches })}</strong>{preview.samples.slice(0, 20).map((sample) => <p key={sample.itemUUID}>{sample.galleryTitle || sample.gallerySetID} · {sample.relativePath} · {sample.currentCategory} → {sample.proposedCategory}</p>)}</div> : null}
      <div className="rule-form-actions"><button type="submit" disabled={!saveEnabled}>{editingID === null ? f("manage.classification.addRule") : f("manage.classification.saveRule")}</button>{editingID !== null ? <button type="button" onClick={reset}>{f("manage.classification.cancelEdit")}</button> : null}</div>
    </form></details>
    <section className="classification-target"><header><div><h4>{f("manage.classification.evaluationTitle")}</h4><p>{library ? f("manage.classification.evaluationSummary", { library: library.name }) : f("manage.classification.evaluationNoLibrary")}</p></div><button type="button" disabled={libraryID === null || evaluateState.loading} onClick={evaluate}>{evaluateState.loading ? f("manage.classification.evaluating") : library ? f("manage.classification.evaluateLibrary", { library: library.name }) : f("manage.classification.evaluateUnavailable")}</button></header></section>
    <div className="classification-review"><header><div><h4>{f("manage.classification.pending")}</h4><p>{library ? f("manage.classification.awaiting", { count: suggestions.length, library: library.name }) : f("manage.classification.selectLibrary")}</p></div>{suggestions.length ? <div className="rule-actions"><button type="button" onClick={() => resolveAll(true)}>{f("manage.classification.acceptAll")}</button><button type="button" onClick={() => resolveAll(false)}>{f("manage.classification.rejectAll")}</button></div> : null}</header>{suggestions.map((item) => <article key={item.id}><div><strong>{item.galleryTitle || item.gallerySetID}</strong><span>{item.relativePath}</span><small>{f("manage.classification.suggestionDetail", { rule: item.ruleName, revision: item.ruleRevision, subject: item.matchedSubject, value: item.matchedValue, category: item.proposedCategory })}</small></div><div className="rule-actions"><button type="button" onClick={() => resolve(item, true)}>{f("manage.classification.accept")}</button><button type="button" onClick={() => resolve(item, false)}>{f("manage.classification.reject")}</button></div></article>)}</div>
  </section>;
}
