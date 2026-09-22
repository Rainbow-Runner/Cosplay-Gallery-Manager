import { type FormEvent, useEffect, useState } from "react";
import { useIntl } from "react-intl";

import type { ManageCoreEntity } from "./types";

export function ManageCoserAssetsPanel({ coser, onUpdated }: { coser: ManageCoreEntity; onUpdated: () => Promise<void> }) {
  const intl = useIntl();
  const t = (id: string) => intl.formatMessage({ id });
  const [avatar, setAvatar] = useState<File | null>(null);
  const [banner, setBanner] = useState<File | null>(null);
  const [crop, setCrop] = useState(coser.avatarCrop || { x: 0, y: 0, size: 1 });
  const [focal, setFocal] = useState(coser.bannerFocalPoint || { x: 0.5, y: 0.5 });
  const [busy, setBusy] = useState<"avatar" | "banner" | null>(null);
  const [message, setMessage] = useState("");

  useEffect(() => {
    setCrop(coser.avatarCrop || { x: 0, y: 0, size: 1 });
    setFocal(coser.bannerFocalPoint || { x: 0.5, y: 0.5 });
  }, [coser.avatarCrop, coser.bannerFocalPoint, coser.uuid]);

  async function upload(event: FormEvent, kind: "avatar" | "banner") {
    event.preventDefault();
    const file = kind === "avatar" ? avatar : banner;
    if (!file) return;
    const body = new FormData();
    body.append("expected_metadata_revision", String(coser.metadataRevision));
    body.append("file", file);
    if (kind === "avatar") {
      body.append("crop_x", String(crop.x)); body.append("crop_y", String(crop.y)); body.append("crop_size", String(crop.size));
    } else {
      body.append("focal_x", String(focal.x)); body.append("focal_y", String(focal.y));
    }
    setBusy(kind); setMessage("");
    try {
      const response = await fetch(`/manage/coser-assets/${coser.uuid}/${kind}`, { method: "POST", body, credentials: "same-origin" });
      if (!response.ok) throw new Error((await response.text()).trim() || t("manage.coserAssets.failed"));
      if (kind === "avatar") setAvatar(null); else setBanner(null);
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
    <div className="coser-assets__grid">
      <form onSubmit={(event) => upload(event, "avatar")}>
        <h4>{t("manage.coserAssets.avatar")}</h4>
        <div className="coser-assets__avatar">{coser.avatarURL ? <img src={coser.avatarURL} alt="" /> : <span>{coser.name.slice(0, 1)}</span>}</div>
        <label>{t("manage.coserAssets.file")}<input required type="file" accept="image/jpeg,image/png,image/webp" onChange={(event) => setAvatar(event.target.files?.[0] || null)} /></label>
        <div className="coser-assets__framing">
          <label>X<input aria-label="Avatar crop X" type="number" min="0" max={1 - crop.size} step="0.01" value={crop.x} onChange={(event) => setCrop({ ...crop, x: Number(event.target.value) })} /></label>
          <label>Y<input aria-label="Avatar crop Y" type="number" min="0" max={1 - crop.size} step="0.01" value={crop.y} onChange={(event) => setCrop({ ...crop, y: Number(event.target.value) })} /></label>
          <label>{t("manage.coserAssets.size")}<input aria-label="Avatar crop size" type="number" min="0.01" max={Math.min(1 - crop.x, 1 - crop.y)} step="0.01" value={crop.size} onChange={(event) => setCrop({ ...crop, size: Number(event.target.value) })} /></label>
        </div>
        <button disabled={!avatar || busy !== null} type="submit">{busy === "avatar" ? t("manage.coserAssets.uploading") : t("manage.coserAssets.uploadAvatar")}</button>
      </form>
      <form onSubmit={(event) => upload(event, "banner")}>
        <h4>{t("manage.coserAssets.banner")}</h4>
        <div className="coser-assets__banner">{coser.bannerURL ? <img src={coser.bannerURL} alt="" style={{ objectPosition: `${focal.x * 100}% ${focal.y * 100}%` }} /> : <span>{t("manage.coserAssets.noBanner")}</span>}</div>
        <label>{t("manage.coserAssets.file")}<input required type="file" accept="image/jpeg,image/png,image/webp" onChange={(event) => setBanner(event.target.files?.[0] || null)} /></label>
        <div className="coser-assets__framing">
          <label>X<input aria-label="Banner focal X" type="number" min="0" max="1" step="0.01" value={focal.x} onChange={(event) => setFocal({ ...focal, x: Number(event.target.value) })} /></label>
          <label>Y<input aria-label="Banner focal Y" type="number" min="0" max="1" step="0.01" value={focal.y} onChange={(event) => setFocal({ ...focal, y: Number(event.target.value) })} /></label>
        </div>
        <button disabled={!banner || busy !== null} type="submit">{busy === "banner" ? t("manage.coserAssets.uploading") : t("manage.coserAssets.uploadBanner")}</button>
      </form>
    </div>
  </section>;
}
