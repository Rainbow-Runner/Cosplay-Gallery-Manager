import { type FormEvent, useEffect, useRef, useState } from "react";
import { useIntl } from "react-intl";

import type { ManageCoreEntity } from "./types";

type AvatarCrop = { x: number; y: number; size: number };
type FocalPoint = { x: number; y: number };
type SourceRect = { x: number; y: number; width: number; height: number };

const defaultCrop: AvatarCrop = { x: 0, y: 0, size: 1 };
const defaultFocal: FocalPoint = { x: 0.5, y: 0.5 };
const acceptedImageTypes = new Set(["image/jpeg", "image/png", "image/webp"]);
const maxUploadBytes = 20 * 1024 * 1024;

const clamp = (value: number, minimum: number, maximum: number) => Math.min(maximum, Math.max(minimum, value));
const normalized = (value: number) => Math.round(value * 10_000) / 10_000;

export function avatarPreviewSourceRect(width: number, height: number, crop: AvatarCrop): SourceRect {
  let x = clamp(crop.x, 0, 1) * width;
  let y = clamp(crop.y, 0, 1) * height;
  let selectedWidth = Math.max(1, Math.min(crop.size * width, width - x));
  let selectedHeight = Math.max(1, Math.min(crop.size * height, height - y));
  if (selectedWidth > selectedHeight) {
    x += (selectedWidth - selectedHeight) / 2;
    selectedWidth = selectedHeight;
  } else if (selectedHeight > selectedWidth) {
    y += (selectedHeight - selectedWidth) / 2;
    selectedHeight = selectedWidth;
  }
  return { x, y, width: selectedWidth, height: selectedHeight };
}

export function bannerPreviewSourceRect(width: number, height: number, focal: FocalPoint): SourceRect {
  const targetRatio = 3;
  let cropWidth = width;
  let cropHeight = height;
  if (cropWidth / cropHeight > targetRatio) cropWidth = cropHeight * targetRatio;
  else cropHeight = cropWidth / targetRatio;
  const x = clamp(focal.x * width - cropWidth / 2, 0, width - cropWidth);
  const y = clamp(focal.y * height - cropHeight / 2, 0, height - cropHeight);
  return { x, y, width: cropWidth, height: cropHeight };
}

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

function AssetPreviewCanvas({ src, kind, crop, focal, label, onDimensions }: {
  src: string;
  kind: "avatar" | "banner";
  crop: AvatarCrop;
  focal: FocalPoint;
  label: string;
  onDimensions: (value: string) => void;
}) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [image, setImage] = useState<HTMLImageElement | null>(null);

  useEffect(() => {
    const next = new Image();
    let active = true;
    next.onload = () => {
      if (!active) return;
      setImage(next);
      onDimensions(`${next.naturalWidth} × ${next.naturalHeight}`);
    };
    next.onerror = () => { if (active) onDimensions(""); };
    next.src = src;
    return () => { active = false; };
  }, [onDimensions, src]);

  useEffect(() => {
    const canvas = canvasRef.current;
    const context = canvas?.getContext("2d");
    if (!canvas || !context || !image) return;
    const source = kind === "avatar"
      ? avatarPreviewSourceRect(image.naturalWidth, image.naturalHeight, crop)
      : bannerPreviewSourceRect(image.naturalWidth, image.naturalHeight, focal);
    context.clearRect(0, 0, canvas.width, canvas.height);
    context.drawImage(image, source.x, source.y, source.width, source.height, 0, 0, canvas.width, canvas.height);
  }, [crop, focal, image, kind]);

  return <canvas ref={canvasRef} width={kind === "avatar" ? 480 : 960} height={kind === "avatar" ? 480 : 320} aria-label={label} />;
}

export function ManageCoserAssetsPanel({ coser, onUpdated }: { coser: ManageCoreEntity; onUpdated: () => Promise<void> }) {
  const intl = useIntl();
  const t = (id: string) => intl.formatMessage({ id });
  const [avatar, setAvatar] = useState<File | null>(null);
  const [banner, setBanner] = useState<File | null>(null);
  const [crop, setCrop] = useState<AvatarCrop>(coser.avatarCrop || defaultCrop);
  const [focal, setFocal] = useState<FocalPoint>(coser.bannerFocalPoint || defaultFocal);
  const [avatarDimensions, setAvatarDimensions] = useState("");
  const [bannerDimensions, setBannerDimensions] = useState("");
  const [busy, setBusy] = useState<"avatar" | "banner" | null>(null);
  const [message, setMessage] = useState("");
  const [fileError, setFileError] = useState("");
  const avatarInput = useRef<HTMLInputElement>(null);
  const bannerInput = useRef<HTMLInputElement>(null);
  const avatarPreview = useFilePreview(avatar);
  const bannerPreview = useFilePreview(banner);

  useEffect(() => {
    setAvatar(null); setBanner(null); setAvatarDimensions(""); setBannerDimensions(""); setFileError("");
    setCrop(coser.avatarCrop || defaultCrop);
    setFocal(coser.bannerFocalPoint || defaultFocal);
    if (avatarInput.current) avatarInput.current.value = "";
    if (bannerInput.current) bannerInput.current.value = "";
  }, [coser.avatarCrop, coser.bannerFocalPoint, coser.uuid]);

  function chooseFile(kind: "avatar" | "banner", selected: File | null) {
    let file = selected;
    setFileError(""); setMessage("");
    if (file && ((file.type !== "" && !acceptedImageTypes.has(file.type)) || file.size > maxUploadBytes)) {
      setFileError(t(file.size > maxUploadBytes ? "manage.coserAssets.fileTooLarge" : "manage.coserAssets.fileType"));
      file = null;
    }
    if (kind === "avatar") {
      setAvatar(file); setAvatarDimensions(""); setCrop(defaultCrop);
      if (!file && avatarInput.current) avatarInput.current.value = "";
    } else {
      setBanner(file); setBannerDimensions(""); setFocal(defaultFocal);
      if (!file && bannerInput.current) bannerInput.current.value = "";
    }
  }

  function setAvatarZoom(value: number) {
    const size = normalized(1 / value);
    const centerX = crop.x + crop.size / 2;
    const centerY = crop.y + crop.size / 2;
    setCrop({ x: normalized(clamp(centerX - size / 2, 0, 1 - size)), y: normalized(clamp(centerY - size / 2, 0, 1 - size)), size });
  }

  function setAvatarPosition(axis: "x" | "y", center: number) {
    const start = normalized(clamp(center - crop.size / 2, 0, 1 - crop.size));
    setCrop({ ...crop, [axis]: start });
  }

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
    setBusy(kind); setMessage(""); setFileError("");
    try {
      const response = await fetch(`/manage/coser-assets/${coser.uuid}/${kind}`, { method: "POST", body, credentials: "same-origin" });
      if (!response.ok) throw new Error((await response.text()).trim() || t("manage.coserAssets.failed"));
      if (kind === "avatar") { setAvatar(null); setAvatarDimensions(""); if (avatarInput.current) avatarInput.current.value = ""; }
      else { setBanner(null); setBannerDimensions(""); if (bannerInput.current) bannerInput.current.value = ""; }
      await onUpdated();
      setMessage(t("manage.coserAssets.saved"));
    } catch (error) {
      setMessage(error instanceof Error ? error.message : t("manage.coserAssets.failed"));
    } finally {
      setBusy(null);
    }
  }

  const cropCenterX = normalized(crop.x + crop.size / 2);
  const cropCenterY = normalized(crop.y + crop.size / 2);
  return <section className="manage-panel coser-assets">
    <h3>{t("manage.coserAssets.heading")}</h3>
    <p>{t("manage.coserAssets.summary")}</p>
    {message ? <p className="manage-message" role="status">{message}</p> : null}
    {fileError ? <p className="manage-error" role="alert">{fileError}</p> : null}
    <div className="coser-assets__grid">
      <form onSubmit={(event) => upload(event, "avatar")}>
        <h4>{t("manage.coserAssets.avatar")}</h4>
        <div className="coser-assets__avatar">
          {avatarPreview ? <AssetPreviewCanvas src={avatarPreview} kind="avatar" crop={crop} focal={focal} label={t("manage.coserAssets.avatarPreview")} onDimensions={setAvatarDimensions} />
            : coser.avatarURL ? <img src={coser.avatarURL} alt="" /> : <span>{coser.name.slice(0, 1)}</span>}
        </div>
        <label>{t("manage.coserAssets.file")}<input ref={avatarInput} required type="file" accept="image/jpeg,image/png,image/webp" onChange={(event) => chooseFile("avatar", event.target.files?.[0] || null)} /></label>
        {avatar ? <fieldset className="coser-assets__editor">
          <legend>{t("manage.coserAssets.avatarFraming")}</legend>
          <p>{t("manage.coserAssets.avatarHint")}{avatarDimensions ? ` · ${avatarDimensions}` : ""}</p>
          <label>{t("manage.coserAssets.zoom")}<input aria-label={t("manage.coserAssets.avatarZoom")} type="range" min="1" max="8" step="0.05" value={normalized(1 / crop.size)} onChange={(event) => setAvatarZoom(Number(event.target.value))} /><output>{(1 / crop.size).toFixed(2)}×</output></label>
          <label>{t("manage.coserAssets.horizontal")}<input aria-label={t("manage.coserAssets.avatarHorizontal")} type="range" min={crop.size / 2} max={1 - crop.size / 2} step="0.001" value={cropCenterX} disabled={crop.size === 1} onChange={(event) => setAvatarPosition("x", Number(event.target.value))} /></label>
          <label>{t("manage.coserAssets.vertical")}<input aria-label={t("manage.coserAssets.avatarVertical")} type="range" min={crop.size / 2} max={1 - crop.size / 2} step="0.001" value={cropCenterY} disabled={crop.size === 1} onChange={(event) => setAvatarPosition("y", Number(event.target.value))} /></label>
          <button type="button" onClick={() => setCrop(defaultCrop)}>{t("manage.coserAssets.reset")}</button>
        </fieldset> : <p className="coser-assets__hint">{t("manage.coserAssets.chooseAvatar")}</p>}
        <button disabled={!avatar || busy !== null} type="submit">{busy === "avatar" ? t("manage.coserAssets.uploading") : t("manage.coserAssets.uploadAvatar")}</button>
      </form>
      <form onSubmit={(event) => upload(event, "banner")}>
        <h4>{t("manage.coserAssets.banner")}</h4>
        <div className="coser-assets__banner">
          {bannerPreview ? <AssetPreviewCanvas src={bannerPreview} kind="banner" crop={crop} focal={focal} label={t("manage.coserAssets.bannerPreview")} onDimensions={setBannerDimensions} />
            : coser.bannerURL ? <img src={coser.bannerURL} alt="" /> : <span>{t("manage.coserAssets.noBanner")}</span>}
        </div>
        <label>{t("manage.coserAssets.file")}<input ref={bannerInput} required type="file" accept="image/jpeg,image/png,image/webp" onChange={(event) => chooseFile("banner", event.target.files?.[0] || null)} /></label>
        {banner ? <fieldset className="coser-assets__editor">
          <legend>{t("manage.coserAssets.bannerFraming")}</legend>
          <p>{t("manage.coserAssets.bannerHint")}{bannerDimensions ? ` · ${bannerDimensions}` : ""}</p>
          <label>{t("manage.coserAssets.horizontal")}<input aria-label={t("manage.coserAssets.bannerHorizontal")} type="range" min="0" max="1" step="0.001" value={focal.x} onChange={(event) => setFocal({ ...focal, x: normalized(Number(event.target.value)) })} /></label>
          <label>{t("manage.coserAssets.vertical")}<input aria-label={t("manage.coserAssets.bannerVertical")} type="range" min="0" max="1" step="0.001" value={focal.y} onChange={(event) => setFocal({ ...focal, y: normalized(Number(event.target.value)) })} /></label>
          <button type="button" onClick={() => setFocal(defaultFocal)}>{t("manage.coserAssets.reset")}</button>
        </fieldset> : <p className="coser-assets__hint">{t("manage.coserAssets.chooseBanner")}</p>}
        <button disabled={!banner || busy !== null} type="submit">{busy === "banner" ? t("manage.coserAssets.uploading") : t("manage.coserAssets.uploadBanner")}</button>
      </form>
    </div>
  </section>;
}
