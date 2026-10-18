import { type PointerEvent as ReactPointerEvent, useEffect, useRef, useState } from "react";
import { useIntl } from "react-intl";

import { Dialog } from "../ui/Patterns";

export type ImageCropKind = "avatar" | "banner";

type CropTransform = { zoom: number; x: number; y: number };
type CropMetrics = { naturalWidth: number; naturalHeight: number; frameWidth: number; frameHeight: number };
export type CropSourceRect = { x: number; y: number; width: number; height: number };

const initialTransform: CropTransform = { zoom: 1, x: 0, y: 0 };
const maxSourcePixels = 50_000_000;
const clamp = (value: number, minimum: number, maximum: number) => Math.min(maximum, Math.max(minimum, value));

function fittedScale(metrics: CropMetrics, zoom: number) {
  return Math.max(metrics.frameWidth / metrics.naturalWidth, metrics.frameHeight / metrics.naturalHeight) * zoom;
}

export function clampCropTransform(transform: CropTransform, metrics: CropMetrics): CropTransform {
  const zoom = clamp(transform.zoom, 1, 3);
  const scale = fittedScale(metrics, zoom);
  const maximumX = Math.max(0, (metrics.naturalWidth * scale - metrics.frameWidth) / 2);
  const maximumY = Math.max(0, (metrics.naturalHeight * scale - metrics.frameHeight) / 2);
  return { zoom, x: clamp(transform.x, -maximumX, maximumX), y: clamp(transform.y, -maximumY, maximumY) };
}

export function cropSourceRect(metrics: CropMetrics, transform: CropTransform): CropSourceRect {
  const current = clampCropTransform(transform, metrics);
  const scale = fittedScale(metrics, current.zoom);
  const width = metrics.frameWidth / scale;
  const height = metrics.frameHeight / scale;
  const centreX = metrics.naturalWidth / 2 - current.x / scale;
  const centreY = metrics.naturalHeight / 2 - current.y / scale;
  return {
    x: clamp(centreX - width / 2, 0, metrics.naturalWidth - width),
    y: clamp(centreY - height / 2, 0, metrics.naturalHeight - height),
    width,
    height,
  };
}

function croppedName(file: File, mimeType: string) {
  const extension = mimeType === "image/png" ? "png" : mimeType === "image/webp" ? "webp" : "jpg";
  const stem = file.name.replace(/\.[^.]+$/, "") || "image";
  return `${stem}-cropped.${extension}`;
}

async function createCroppedFile(file: File, image: HTMLImageElement, metrics: CropMetrics, transform: CropTransform, kind: ImageCropKind) {
  const source = cropSourceRect(metrics, transform);
  const width = kind === "avatar" ? 960 : 1600;
  const height = kind === "avatar" ? 960 : Math.round(width / 3);
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const context = canvas.getContext("2d");
  if (!context) throw new Error("canvas unavailable");
  context.drawImage(image, source.x, source.y, source.width, source.height, 0, 0, width, height);
  const mimeType = file.type === "image/png" || file.type === "image/webp" ? file.type : "image/jpeg";
  const blob = await new Promise<Blob>((resolve, reject) => canvas.toBlob((value) => value ? resolve(value) : reject(new Error("crop encoding failed")), mimeType, 0.92));
  return new File([blob], croppedName(file, mimeType), { type: mimeType, lastModified: Date.now() });
}

export function ImageCropperDialog({ file, kind, onCancel, onConfirm }: {
  file: File;
  kind: ImageCropKind;
  onCancel: () => void;
  onConfirm: (file: File) => void;
}) {
  const intl = useIntl();
  const t = (id: string) => intl.formatMessage({ id });
  const [imageURL] = useState(() => URL.createObjectURL(file));
  const imageRef = useRef<HTMLImageElement>(null);
  const frameRef = useRef<HTMLDivElement>(null);
  const pointers = useRef(new Map<number, { x: number; y: number }>());
  const pointerStarts = useRef(new Map<number, { x: number; y: number }>());
  const gesture = useRef({ centreX: 0, centreY: 0, distance: 0 });
  const lastTouchTap = useRef(0);
  const transformRef = useRef<CropTransform>(initialTransform);
  const [transform, setTransform] = useState<CropTransform>(initialTransform);
  const [metrics, setMetrics] = useState<CropMetrics | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const aspect = kind === "avatar" ? 1 : 3;

  useEffect(() => () => URL.revokeObjectURL(imageURL), [imageURL]);

  useEffect(() => {
    function measure() {
      const image = imageRef.current;
      const frame = frameRef.current;
      if (!image?.naturalWidth || !image.naturalHeight || !frame) return;
      const bounds = frame.getBoundingClientRect();
      if (!bounds.width || !bounds.height) return;
      const next = { naturalWidth: image.naturalWidth, naturalHeight: image.naturalHeight, frameWidth: bounds.width, frameHeight: bounds.height };
      setMetrics(next);
      const clamped = clampCropTransform(transformRef.current, next);
      transformRef.current = clamped;
      setTransform(clamped);
    }
    window.addEventListener("resize", measure);
    measure();
    return () => window.removeEventListener("resize", measure);
  }, []);

  function apply(next: CropTransform) {
    const value = metrics ? clampCropTransform(next, metrics) : next;
    transformRef.current = value;
    setTransform(value);
  }

  function reset() { apply(initialTransform); }

  function pointerSummary() {
    const values = [...pointers.current.values()];
    const centreX = values.reduce((sum, value) => sum + value.x, 0) / values.length;
    const centreY = values.reduce((sum, value) => sum + value.y, 0) / values.length;
    const distance = values.length > 1 ? Math.hypot(values[0].x - values[1].x, values[0].y - values[1].y) : 0;
    return { centreX, centreY, distance };
  }

  function pointerDown(event: ReactPointerEvent<HTMLDivElement>) {
    event.preventDefault();
    event.currentTarget.setPointerCapture?.(event.pointerId);
    pointers.current.set(event.pointerId, { x: event.clientX, y: event.clientY });
    pointerStarts.current.set(event.pointerId, { x: event.clientX, y: event.clientY });
    gesture.current = pointerSummary();
  }

  function pointerMove(event: ReactPointerEvent<HTMLDivElement>) {
    if (!pointers.current.has(event.pointerId)) return;
    event.preventDefault();
    pointers.current.set(event.pointerId, { x: event.clientX, y: event.clientY });
    const next = pointerSummary();
    const previous = gesture.current;
    let zoom = transformRef.current.zoom;
    if (pointers.current.size > 1 && previous.distance > 0 && next.distance > 0) zoom *= next.distance / previous.distance;
    apply({ zoom, x: transformRef.current.x + next.centreX - previous.centreX, y: transformRef.current.y + next.centreY - previous.centreY });
    gesture.current = next;
  }

  function pointerEnd(event: ReactPointerEvent<HTMLDivElement>) {
    const start = pointerStarts.current.get(event.pointerId);
    pointers.current.delete(event.pointerId);
    pointerStarts.current.delete(event.pointerId);
    if (event.pointerType === "touch" && start && Math.hypot(event.clientX - start.x, event.clientY - start.y) < 8) {
      const now = Date.now();
      if (now - lastTouchTap.current < 320) { reset(); lastTouchTap.current = 0; }
      else lastTouchTap.current = now;
    }
    if (pointers.current.size) gesture.current = pointerSummary();
  }

  async function confirm() {
    if (!metrics || !imageRef.current || busy) return;
    setBusy(true); setError("");
    try { onConfirm(await createCroppedFile(file, imageRef.current, metrics, transformRef.current, kind)); }
    catch { setError(t("manage.coserCrop.failed")); setBusy(false); }
  }

  const scale = metrics ? fittedScale(metrics, transform.zoom) : 1;
  return <Dialog titleID="coser-image-crop-title" onClose={onCancel} className="image-cropper-dialog" dismissible={!busy}>
    <div className="image-cropper" onKeyDown={(event) => {
      if (event.key === "Enter" && !(event.target instanceof HTMLButtonElement)) { event.preventDefault(); void confirm(); }
    }}>
      <header>
        <h3 id="coser-image-crop-title">{t(kind === "avatar" ? "manage.coserCrop.avatarTitle" : "manage.coserCrop.bannerTitle")}</h3>
        <p>{t("manage.coserCrop.help")}</p>
      </header>
      <div
        autoFocus
        className="image-cropper__stage"
        role="application"
        tabIndex={0}
        aria-label={t("manage.coserCrop.canvas")}
        onDoubleClick={reset}
        onPointerDown={pointerDown}
        onPointerMove={pointerMove}
        onPointerUp={pointerEnd}
        onPointerCancel={pointerEnd}
        onWheel={(event) => { event.preventDefault(); apply({ ...transformRef.current, zoom: transformRef.current.zoom * Math.exp(-event.deltaY * 0.0015) }); }}
      >
        <img
          ref={imageRef}
          src={imageURL}
          alt=""
          draggable={false}
          onLoad={(event) => {
            const image = event.currentTarget;
            if (image.naturalWidth * image.naturalHeight > maxSourcePixels) {
              setMetrics(null);
              setError(t("manage.coserCrop.tooManyPixels"));
              return;
            }
            setError("");
            window.dispatchEvent(new Event("resize"));
          }}
          onError={() => { setMetrics(null); setError(t("manage.coserCrop.invalidImage")); }}
          style={metrics ? { width: metrics.naturalWidth * scale, height: metrics.naturalHeight * scale, transform: `translate(-50%, -50%) translate(${transform.x}px, ${transform.y}px)` } : { visibility: "hidden" }}
        />
        <div ref={frameRef} className={`image-cropper__frame image-cropper__frame--${kind}`} style={{ aspectRatio: String(aspect) }} aria-hidden="true" />
        <output className="image-cropper__zoom" aria-label={t("manage.coserCrop.zoom")}>{transform.zoom.toFixed(2)}×</output>
      </div>
      {error ? <p className="manage-error" role="alert">{error}</p> : null}
      <footer>
        <button type="button" disabled={busy} onClick={onCancel}>{t("manage.coserCrop.cancel")}</button>
        <button type="button" disabled={busy} onClick={reset}>{t("manage.coserCrop.reset")}</button>
        <button type="button" disabled={!metrics || busy} onClick={() => void confirm()}>{busy ? t("manage.coserCrop.processing") : t("manage.coserCrop.confirm")}</button>
      </footer>
    </div>
  </Dialog>;
}
