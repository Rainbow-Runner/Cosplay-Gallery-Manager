import { useMutation, useQuery } from "@apollo/client/react";
import { useState } from "react";
import { useIntl } from "react-intl";

import { DELETE_GALLERY, PREVIEW_GALLERY_DELETE } from "../api/manage";
import { Dialog } from "../ui/Patterns";
import type { ManageGalleryDeletePreview } from "./types";

export function ManageGalleryDeletePanel({
  setID,
  title,
  onDeleted,
}: {
  setID: string;
  title: string;
  onDeleted: () => void;
}) {
  const intl = useIntl();
  const t = (id: string, values?: Record<string, string | number>) => intl.formatMessage({ id }, values);
  const previewState = useQuery<{ previewGalleryDelete: ManageGalleryDeletePreview }>(PREVIEW_GALLERY_DELETE, {
    variables: { setID },
    fetchPolicy: "network-only",
  });
  const [deleteGallery, deleteState] = useMutation<{ deleteGallery: boolean }>(DELETE_GALLERY);
  const [confirming, setConfirming] = useState(false);
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [message, setMessage] = useState("");
  const preview = previewState.data?.previewGalleryDelete;

  async function remove() {
    if (!preview?.canDelete || confirmation !== "DELETE" || !password) return;
    setMessage("");
    try {
      const result = await deleteGallery({
        variables: {
          setID,
          expectedMetadataRevision: preview.metadataRevision,
          password,
          confirmation,
        },
      });
      if (result.data?.deleteGallery) onDeleted();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : t("manage.galleryDelete.failed"));
      setPassword("");
      await previewState.refetch();
    }
  }

  return <section className="manage-panel entity-lifecycle gallery-delete">
    <h3>{t("manage.galleryDelete.heading")}</h3>
    <p>{t("manage.galleryDelete.summary")}</p>
    {message ? <p className="manage-error" role="alert">{message}</p> : null}
    {previewState.loading ? <p>{t("manage.galleryDelete.checking")}</p> : previewState.error ? <p className="manage-error">{t("manage.galleryDelete.previewFailed")}</p> : preview ? <div className="entity-lifecycle__preview">
      <dl>
        <dt>{t("manage.galleryDelete.state")}</dt><dd>{preview.state}</dd>
        <dt>{t("manage.galleryDelete.items")}</dt><dd>{preview.itemCount}</dd>
        <dt>{t("manage.galleryDelete.links")}</dt><dd>{preview.externalLinkCount}</dd>
        <dt>{t("manage.galleryDelete.jobs")}</dt><dd>{preview.executableJobCount}</dd>
      </dl>
      <p>{preview.ignoredSourceWillBeCreated ? t("manage.galleryDelete.ignoreSource") : t("manage.galleryDelete.noSource")}</p>
      {!preview.canDelete ? <p>{t("manage.galleryDelete.archiveFirst")}</p> : <p>{t("manage.galleryDelete.ready")}</p>}
      <button className="danger" type="button" disabled={!preview.canDelete} onClick={() => { setPassword(""); setConfirmation(""); setMessage(""); setConfirming(true); }}>{t("manage.galleryDelete.open")}</button>
    </div> : null}
    {confirming ? <Dialog titleID="gallery-delete-confirm-title" dismissible={!deleteState.loading} onClose={() => setConfirming(false)}>
      <p>{t("manage.galleryDelete.permanent")}</p>
      <h3 id="gallery-delete-confirm-title">{t("manage.galleryDelete.confirmTitle", { title })}</h3>
      <ul>
        <li>{t("manage.galleryDelete.effectDatabase")}</li>
        <li>{t("manage.galleryDelete.effectUUIDs")}</li>
        <li>{t("manage.galleryDelete.effectFiles")}</li>
        <li>{t("manage.galleryDelete.effectRecovery")}</li>
      </ul>
      <label>{t("manage.galleryDelete.password")}<input autoFocus type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} /></label>
      <label>{t("manage.galleryDelete.typeDelete")}<input value={confirmation} onChange={(event) => setConfirmation(event.target.value)} /></label>
      <footer>
        <button type="button" onClick={() => setConfirming(false)}>{t("manage.galleryDelete.cancel")}</button>
        <button className="danger" type="button" disabled={!password || confirmation !== "DELETE" || deleteState.loading} onClick={remove}>{deleteState.loading ? t("manage.galleryDelete.deleting") : t("manage.galleryDelete.commit")}</button>
      </footer>
    </Dialog> : null}
  </section>;
}
