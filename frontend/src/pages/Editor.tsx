import { useEffect, useMemo, useRef, useState } from "react";
import { api } from "../api";
import { applyColor, cropRect, type CropUI } from "../edits";
import { useApp, useT } from "../state";
import { ErrorNote, Modal, useAction } from "../ui";
import type { Edits, Media } from "../types";

const ASPECTS: Array<[string, number | null]> = [["Free", null], ["1:1", 1], ["4:3", 4 / 3], ["3:2", 3 / 2], ["16:9", 16 / 9], ["2:3", 2 / 3], ["3:4", 3 / 4]];

function initial(m: Media): { rot: number; b: number; c: number; s: number } {
  try { const e = JSON.parse(m.edits || "{}"); return { rot: e.rotate ?? 0, b: e.brightness ?? 0, c: e.contrast ?? 0, s: e.saturation ?? 0 }; } catch { return { rot: 0, b: 0, c: 0, s: 0 }; }
}

/** Non-destructive editor: rotate, crop, brightness/contrast/saturation.
 *  The preview is drawn from the *original* preview image with the same maths
 *  as the Go renderer; saving stores a recipe + render, never touching the original. */
export function Editor({ media, onClose, onSaved }: { media: Media; onClose: () => void; onSaved: (m: Media) => void }) {
  const { mediaUrl, toast } = useApp();
  const { t } = useT();
  const init = useMemo(() => initial(media), [media]);
  const [rot, setRot] = useState(init.rot);
  const [b, setB] = useState(init.b);
  const [c, setC] = useState(init.c);
  const [s, setS] = useState(init.s);
  const [crop, setCrop] = useState<CropUI>({ aspect: null, zoom: 1, cx: 0.5, cy: 0.5 });
  const [img, setImg] = useState<HTMLImageElement | null>(null);
  const canvas = useRef<HTMLCanvasElement>(null);
  const { busy, error, run } = useAction();

  useEffect(() => {
    const i = new Image();
    i.onload = () => setImg(i);
    // the preview of the ORIGINAL (not the edited render): edits are applied on top of it
    i.src = mediaUrl(media.id, "basepreview");
  }, [media, mediaUrl]);

  const rotated = useMemo(() => {
    if (!img) return null;
    const swap = rot % 180 !== 0;
    const w = swap ? img.naturalHeight : img.naturalWidth, h = swap ? img.naturalWidth : img.naturalHeight;
    const cv = document.createElement("canvas");
    cv.width = w; cv.height = h;
    const g = cv.getContext("2d")!;
    g.translate(w / 2, h / 2); g.rotate((rot * Math.PI) / 180);
    g.drawImage(img, -img.naturalWidth / 2, -img.naturalHeight / 2);
    return cv;
  }, [img, rot]);

  const rect = rotated ? cropRect(crop, rotated.width, rotated.height) : null;

  useEffect(() => {
    if (!rotated || !canvas.current) return;
    const sx = rect ? rect.x * rotated.width : 0, sy = rect ? rect.y * rotated.height : 0;
    const sw = rect ? rect.w * rotated.width : rotated.width, sh = rect ? rect.h * rotated.height : rotated.height;
    const scale = Math.min(1, 900 / sw, 600 / sh);
    const cv = canvas.current;
    cv.width = Math.max(1, Math.round(sw * scale)); cv.height = Math.max(1, Math.round(sh * scale));
    const g = cv.getContext("2d")!;
    g.drawImage(rotated, sx, sy, sw, sh, 0, 0, cv.width, cv.height);
    const d = g.getImageData(0, 0, cv.width, cv.height);
    applyColor(d.data, { brightness: b, contrast: c, saturation: s });
    g.putImageData(d, 0, 0);
  }, [rotated, rect?.x, rect?.y, rect?.w, rect?.h, b, c, s]); // eslint-disable-line react-hooks/exhaustive-deps

  const recipe = (): Edits => ({ rotate: rot, crop: rect, brightness: b, contrast: c, saturation: s });
  const reset = () => { setRot(0); setB(0); setC(0); setS(0); setCrop({ aspect: null, zoom: 1, cx: 0.5, cy: 0.5 }); };
  const slider = (label: string, v: number, set: (n: number) => void) => (
    <label className="slider">{label}<input type="range" min={-1} max={1} step={0.02} value={v} onChange={(e) => set(Number(e.target.value))} /><output>{v.toFixed(2)}</output></label>
  );

  return (
    <Modal title={t("editPhoto")} onClose={onClose} wide>
      <div className="editor">
        <div className="canvas-wrap"><canvas ref={canvas} aria-label={t("editPreview")} /></div>
        <div className="panel">
          <ErrorNote error={error} />
          <h4>{t("rotatePhoto")}</h4>
          <div className="row">
            <button onClick={() => setRot((r) => (r + 270) % 360)}>↺ 90°</button>
            <button onClick={() => setRot((r) => (r + 90) % 360)}>↻ 90°</button>
            <span className="muted">{rot}°</span>
          </div>
          <h4>{t("cropPhoto")}</h4>
          <div className="seg wrap">
            {ASPECTS.map(([n, a]) => <button key={n} className={crop.aspect === a ? "on" : ""} onClick={() => setCrop({ ...crop, aspect: a, zoom: a === null ? 1 : Math.min(crop.zoom, 1) })}>{n === "Free" ? t("freeCrop") : n}</button>)}
          </div>
          <label className="slider">{t("zoom")}<input type="range" min={0.2} max={1} step={0.01} value={crop.zoom} onChange={(e) => setCrop({ ...crop, zoom: Number(e.target.value) })} /><output>{Math.round(crop.zoom * 100)}%</output></label>
          <label className="slider">{t("leftRight")}<input type="range" min={0} max={1} step={0.01} value={crop.cx} onChange={(e) => setCrop({ ...crop, cx: Number(e.target.value) })} /><output /></label>
          <label className="slider">{t("upDown")}<input type="range" min={0} max={1} step={0.01} value={crop.cy} onChange={(e) => setCrop({ ...crop, cy: Number(e.target.value) })} /><output /></label>
          <h4>{t("color")}</h4>
          {slider(t("brightness"), b, setB)}
          {slider(t("contrast"), c, setC)}
          {slider(t("saturation"), s, setS)}
          <p className="muted small">{t("originalUnchanged")}</p>
        </div>
      </div>
      <footer>
        <button onClick={reset}>{t("reset")}</button>
        <button disabled={busy} onClick={() => run(async () => {
          const m = await api.saveEdits(media.id, { rotate: 0, crop: null, brightness: 0, contrast: 0, saturation: 0 });
          toast(t("revertedOriginal")); onSaved(m); onClose();
        })}>{t("revertOriginal")}</button>
        <button className="primary" disabled={busy} onClick={() => run(async () => {
          const m = await api.saveEdits(media.id, recipe()); toast(t("editsSaved")); onSaved(m); onClose();
        })}>{t("save")}</button>
      </footer>
    </Modal>
  );
}
