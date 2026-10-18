import { useLazyQuery, useMutation, useQuery } from "@apollo/client/react";
import { useEffect, useState } from "react";
import { useIntl } from "react-intl";

import { DELETE_CORE_ENTITY, MERGE_CORE_ENTITIES, PREVIEW_CORE_ENTITY_DELETE, PREVIEW_CORE_ENTITY_MERGE } from "../api/manage";
import { Dialog } from "../ui/Patterns";
import { ManageEntitySelector } from "./ManageEntitySelector";
import type {
  ManageCoreEntity,
  ManageCoreEntityDeletePreview,
  ManageCoreEntityMergePreview,
  ManageCoreEntityMergeResult,
} from "./types";

type Target = Pick<ManageCoreEntity, "uuid" | "name" | "metadataRevision" | "workUUID" | "workName">;

const emptyTarget = (): Target => ({ uuid: "", name: "", metadataRevision: 0, workUUID: "", workName: "" });

export function ManageEntityLifecyclePanel({
  source,
  onMerged,
  onDeleted,
}: {
  source: ManageCoreEntity;
  onMerged: (target: ManageCoreEntity, completionWarning?: string | null) => void;
  onDeleted: () => void;
}) {
  const intl = useIntl();
  const t = (id: string, values?: Record<string, string | number>) => intl.formatMessage({ id }, values);
  const [target, setTarget] = useState<Target>(emptyTarget);
  const [confirmation, setConfirmation] = useState<"MERGE" | "DELETE" | null>(null);
  const [confirmationText, setConfirmationText] = useState("");
  const [message, setMessage] = useState("");
  const [loadMergePreview, mergePreviewState] = useLazyQuery<{ previewCoreEntityMerge: ManageCoreEntityMergePreview }>(PREVIEW_CORE_ENTITY_MERGE, { fetchPolicy: "network-only" });
  const deletePreviewState = useQuery<{ previewCoreEntityDelete: ManageCoreEntityDeletePreview }>(PREVIEW_CORE_ENTITY_DELETE, {
    variables: { kind: source.kind, uuid: source.uuid },
    fetchPolicy: "network-only",
  });
  const [mergeEntities, mergeState] = useMutation<{ mergeCoreEntities: ManageCoreEntityMergeResult }>(MERGE_CORE_ENTITIES);
  const [deleteEntity, deleteState] = useMutation<{ deleteCoreEntity: boolean }>(DELETE_CORE_ENTITY);
  const loadedMergePreview = mergePreviewState.data?.previewCoreEntityMerge;
  const mergePreview = loadedMergePreview?.sourceUUID === source.uuid && loadedMergePreview.targetUUID === target.uuid ? loadedMergePreview : undefined;
  const deletePreview = deletePreviewState.data?.previewCoreEntityDelete;

  useEffect(() => {
    setTarget(emptyTarget());
    setConfirmation(null);
    setConfirmationText("");
    setMessage("");
  }, [source.kind, source.uuid]);

  function selectTarget(entity: Target) {
    setTarget(entity.uuid === source.uuid ? emptyTarget() : entity);
    setMessage(entity.uuid === source.uuid ? t("manage.lifecycle.sameSource") : "");
  }

  async function previewMerge() {
    if (!target.uuid || target.uuid === source.uuid) return;
    setMessage("");
    try {
      await loadMergePreview({ variables: { kind: source.kind, sourceUUID: source.uuid, targetUUID: target.uuid } });
    } catch (error) {
      setMessage(error instanceof Error ? error.message : t("manage.lifecycle.previewFailed"));
    }
  }

  async function merge() {
    if (!mergePreview?.canMerge || confirmationText !== "MERGE") return;
    setMessage("");
    try {
      const result = await mergeEntities({
        variables: {
          kind: source.kind,
          sourceUUID: source.uuid,
          targetUUID: mergePreview.targetUUID,
          expectedSourceRevision: mergePreview.sourceRevision,
          expectedTargetRevision: mergePreview.targetRevision,
        },
      });
      if (result.data) {
        setConfirmation(null);
        onMerged(result.data.mergeCoreEntities.target, result.data.mergeCoreEntities.completionWarning);
      }
    } catch (error) {
      setConfirmation(null);
      setMessage(error instanceof Error ? error.message : t("manage.lifecycle.mergeFailed"));
    }
  }

  async function remove() {
    if (!deletePreview?.canDelete || confirmationText !== "DELETE") return;
    setMessage("");
    try {
      await deleteEntity({
        variables: {
          kind: source.kind,
          uuid: source.uuid,
          expectedMetadataRevision: deletePreview.metadataRevision,
        },
      });
      setConfirmation(null);
      onDeleted();
    } catch (error) {
      setConfirmation(null);
      setMessage(error instanceof Error ? error.message : t("manage.lifecycle.deleteFailed"));
      await deletePreviewState.refetch();
    }
  }

  return <section className="manage-panel entity-lifecycle">
    <h3>{t("manage.lifecycle.title")}</h3>
    <p>{t("manage.lifecycle.summary")}</p>
    {message ? <p className="manage-error">{message}</p> : null}
    <article>
      <h4>{t("manage.lifecycle.mergeHeading")}</h4>
      <ManageEntitySelector kind={source.kind} label={t("manage.lifecycle.targetLabel", { kind: source.kind })} uuid={target.uuid} name={target.name} onSelect={selectTarget} />
      <div className="manage-panel-actions"><button type="button" disabled={!target.uuid || mergePreviewState.loading} onClick={previewMerge}>{mergePreviewState.loading ? t("manage.lifecycle.checking") : t("manage.lifecycle.previewMerge")}</button></div>
      {mergePreview ? <div className="entity-lifecycle__preview" aria-live="polite">
        <dl><dt>{t("manage.lifecycle.sourceRevision")}</dt><dd>{mergePreview.sourceRevision}</dd><dt>{t("manage.lifecycle.targetRevision")}</dt><dd>{mergePreview.targetRevision}</dd><dt>{t("manage.lifecycle.affectedGalleries")}</dt><dd>{mergePreview.affectedGalleryIDs.length ? mergePreview.affectedGalleryIDs.join(", ") : t("manage.lifecycle.none")}</dd></dl>
        {source.kind === "CHARACTER" ? <p>{t("manage.lifecycle.characterMergeWork", { source: source.workName || source.workUUID || t("manage.lifecycle.none"), target: target.workName || target.workUUID || t("manage.lifecycle.none") })}</p> : null}
        {mergePreview.conflicts.length ? <><strong>{t("manage.lifecycle.resolveBlockers")}</strong><ul>{mergePreview.conflicts.map((conflict) => <li key={`${conflict.code}-${conflict.details}`}><code>{conflict.code}</code> — {conflict.details}</li>)}</ul></> : <p>{t("manage.lifecycle.noMergeConflicts")}</p>}
        <button className="danger" type="button" disabled={!mergePreview.canMerge} onClick={() => { setConfirmationText(""); setConfirmation("MERGE"); }}>{t("manage.lifecycle.mergePermanently")}</button>
      </div> : null}
    </article>
    <article>
      <h4>{t("manage.lifecycle.deleteHeading")}</h4>
      {deletePreviewState.loading ? <p>{t("manage.lifecycle.checkingReferences")}</p> : deletePreviewState.error ? <p className="manage-error">{t("manage.lifecycle.inspectDeleteFailed")}</p> : deletePreview ? <div className="entity-lifecycle__preview">
        {deletePreview.blockers.length ? <><strong>{t("manage.lifecycle.deleteBlocked", { count: deletePreview.referenceCount })}</strong><ul>{deletePreview.blockers.map((blocker) => <li key={blocker.code}><code>{blocker.code}</code> — {blocker.referenceCount}</li>)}</ul></> : <p>{t("manage.lifecycle.deleteSafe")}</p>}
        <button className="danger" type="button" disabled={!deletePreview.canDelete} onClick={() => { setConfirmationText(""); setConfirmation("DELETE"); }}>{t("manage.lifecycle.deletePermanently")}</button>
      </div> : null}
    </article>
    {confirmation ? <Dialog titleID="entity-lifecycle-confirm-title" dismissible={!mergeState.loading && !deleteState.loading} onClose={() => setConfirmation(null)}>
      <p>{t("manage.lifecycle.permanentChange")}</p><h3 id="entity-lifecycle-confirm-title">{confirmation === "MERGE" ? t("manage.lifecycle.mergeConfirm", { source: source.name, target: target.name }) : t("manage.lifecycle.deleteConfirm", { source: source.name })}</h3>
      <ul>{confirmation === "MERGE" ? <><li>{t("manage.lifecycle.mergeEffectTarget")}</li>{source.kind === "CHARACTER" ? <li>{t("manage.lifecycle.characterMergeEffectWork", { work: target.workName || target.workUUID || t("manage.lifecycle.none") })}</li> : null}<li>{t("manage.lifecycle.mergeEffectAlias")}</li><li>{t("manage.lifecycle.noMediaChange")}</li></> : <><li>{t("manage.lifecycle.deleteEffectReferences")}</li><li>{t("manage.lifecycle.deleteEffectTombstone")}</li><li>{t("manage.lifecycle.deleteEffectAssets")}</li></>}</ul>
      <label>{t("manage.lifecycle.typeToContinue", { phrase: confirmation })}<input autoFocus value={confirmationText} onChange={(event) => setConfirmationText(event.target.value)} /></label>
      <footer><button type="button" onClick={() => setConfirmation(null)}>{t("manage.lifecycle.cancel")}</button><button className="danger" type="button" disabled={confirmationText !== confirmation || mergeState.loading || deleteState.loading} onClick={confirmation === "MERGE" ? merge : remove}>{mergeState.loading || deleteState.loading ? t("manage.lifecycle.committing") : confirmation === "MERGE" ? t("manage.lifecycle.mergeCommit") : t("manage.lifecycle.deleteCommit")}</button></footer>
    </Dialog> : null}
  </section>;
}
