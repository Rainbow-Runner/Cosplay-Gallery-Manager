import { type FormEvent, useEffect, useRef, useState } from "react";
import { useIntl } from "react-intl";

import { ImageCropperDialog, type ImageCropKind } from "./ImageCropperDialog";
import type { ManageCoreEntity } from "./types";

const acceptedImageTypes = new Set(["image/jpeg", "image/png", "image/webp"]);
const maxUploadBytes = 20 * 1024 * 1024;

function useFilePreview(file: File | null) {
  const [url, setURL] = useState("");
  useEffect(() => {
    if (!file) { setURL(""); return; }
    const next = URL.createObjectURL(file);
    setURL(next);
    return () => URL.revokeObjectURL(next);
  }, [file]);
  return url;
}

export function ManageCoserAssetsPanel({ coser, onUpdated }: { coser: ManageCoreEntity; onUpdated: () => Promise<void> }) {
  const intl = useIntl();
  const t = (id: string) => intl.formatMessage({ id });
  const [avatar, setAvatar] = useState<File | null>(null);
  const [banner, setBanner] = useState<File | null>(null);
  const [cropRequest, setCropRequest] = useState<{ kind: ImageCropKind; file: File } | null>(null);
  const [busy, setBusy] = useState<ImageCropKind | null>(null);
  const [message, setMessage] = useState("");
  const [fileError, setFileError] = useState("");
  const avatarInput = useRef<HTMLInputElement>(null);
  const bannerInput = useRef<HTMLInputElement>(null);
  const avatarPreview = useFilePreview(avatar);
  const bannerPreview = useFilePreview(banner);

  useEffect(() => {
    setAvatar(null); setBanner(null); setCropRequest(null); setFileError("");
    if (avatarInput.current) avatarInput.current.value = "";
    if (bannerInput.current) bannerInput.current.value = "";
  }, [coser.uuid]);

  function inputFor(kind: ImageCropKind) { return kind === "avatar" ? avatarInput.current : bannerInput.current; }

  function chooseFile(kind: ImageCropKind, selected: File | null) {
    setFileError(""); setMessage("");
    if (!selected) return;
    if ((selected.type !== "" && !acceptedImageTypes.has(selected.type)) || selected.size > maxUploadBytes) {
      setFileError(t(selected.size > maxUploadBytes ? "manage.coserAssets.fileTooLarge" : "manage.coserAssets.fileType"));
      const input = inputFor(kind);
      if (input) input.value = "";
      return;
    }
    setCropRequest({ kind, file: selected });
  }

  function cancelCrop() {
    if (cropRequest) {
      const input = inputFor(cropRequest.kind);
      if (input) input.value = "";
    }
    setCropRequest(null);
  }

  function confirmCrop(file: File) {
    if (!cropRequest) return;
    if (cropRequest.kind === "avatar") setAvatar(file); else setBanner(file);
    setCropRequest(null);
  }

  async function upload(event: FormEvent, kind: ImageCropKind) {
    event.preventDefault();
    const file = kind === "avatar" ? avatar : banner;
    if (!file) return;
    const body = new FormData();
    body.append("expected_metadata_revision", String(coser.metadataRevision));
    body.append("file", file);
    if (kind === "avatar") {
      body.append("crop_x", "0"); body.append("crop_y", "0"); body.append("crop_size", "1");
    } else {
      body.append("focal_x", "0.5"); body.append("focal_y", "0.5");
    }
    setBusy(kind); setMessage(""); setFileError("");
    try {
      const response = await fetch(`/manage/coser-assets/${coser.uuid}/${kind}`, { method: "POST", body, credentials: "same-origin" });
      if (!response.ok) throw new Error((await response.text()).trim() || t("manage.coserAssets.failed"));
      if (kind === "avatar") { setAvatar(null); if (avatarInput.current) avatarInput.current.value = ""; }
      else { setBanner(null); if (bannerInput.current) bannerInput.current.value = ""; }
      await onUpdated();
      setMessage(t("manage.coserAssets.saved"));
    } catch (error) {
      setMessage(error instanceof Error ? error.message : t("manage.coserAssets.failed"));
    } finally {
      setBusy(null);
    }
  }

  return <section className="manage-panel coser-assets">
    <h3>{t("manage.coserAssets.heading")}</h3>
    <p>{t("manage.coserAssets.summary")}</p>
    {message ? <p className="manage-message" role="status">{message}</p> : null}
    {fileError ? <p className="manage-error" role="alert">{fileError}</p> : null}
    <div className="coser-assets__grid">
      <form onSubmit={(event) => upload(event, "avatar")}>
        <h4>{t("manage.coserAssets.avatar")}</h4>
        <div className="coser-assets__avatar">{avatarPreview ? <img src={avatarPreview} alt="" /> : coser.avatarURL ? <img src={coser.avatarURL} alt="" /> : <span>{coser.name.slice(0, 1)}</span>}</div>
        <label>{t("manage.coserAssets.file")}<input ref={avatarInput} required type="file" accept="image/jpeg,image/png,image/webp" onChange={(event) => chooseFile("avatar", event.target.files?.[0] || null)} /></label>
        <p className="coser-assets__hint">{avatar ? t("manage.coserAssets.cropReady") : t("manage.coserAssets.chooseAvatar")}</p>
        <button disabled={!avatar || busy !== null} type="submit">{busy === "avatar" ? t("manage.coserAssets.uploading") : t("manage.coserAssets.uploadAvatar")}</button>
      </form>
      <form onSubmit={(event) => upload(event, "banner")}>
        <h4>{t("manage.coserAssets.banner")}</h4>
        <div className="coser-assets__banner">{bannerPreview ? <img src={bannerPreview} alt="" /> : coser.bannerURL ? <img src={coser.bannerURL} alt="" /> : <span>{t("manage.coserAssets.noBanner")}</span>}</div>
        <label>{t("manage.coserAssets.file")}<input ref={bannerInput} required type="file" accept="image/jpeg,image/png,image/webp" onChange={(event) => chooseFile("banner", event.target.files?.[0] || null)} /></label>
        <p className="coser-assets__hint">{banner ? t("manage.coserAssets.cropReady") : t("manage.coserAssets.chooseBanner")}</p>
        <button disabled={!banner || busy !== null} type="submit">{busy === "banner" ? t("manage.coserAssets.uploading") : t("manage.coserAssets.uploadBanner")}</button>
      </form>
    </div>
    {cropRequest ? <ImageCropperDialog key={`${cropRequest.kind}-${cropRequest.file.name}-${cropRequest.file.lastModified}`} file={cropRequest.file} kind={cropRequest.kind} onCancel={cancelCrop} onConfirm={confirmCrop} /> : null}
  </section>;
}
