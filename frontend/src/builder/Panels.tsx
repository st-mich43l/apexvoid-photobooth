import { useRef, useState, type ReactNode } from "react";
import { formatTranslation, type TranslationKey } from "../i18n";
import { categoryLabel } from "../frameInfo";
import { useLocale } from "../locale";
import { Categories } from "./categories";
import { addVariant, imageWarnings, removeVariant, type Selection } from "./draft";
import type { AssetInfo, FrameDef, Layer, Slot, Variant } from "../types";
import { useT } from "../state";

const translateSlots = (count: number, language: string) => formatTranslation(language, "photoSlots", { count });
const SHAPES = ["heart", "star", "sparkle", "flower", "cloud", "triangle", "dot", "square", "sprocket", "diamond"] as const;
const SHAPE_LABELS: Record<(typeof SHAPES)[number], TranslationKey> = {
  heart: "shapeHeart", star: "shapeStar", sparkle: "shapeSparkle", flower: "shapeFlower", cloud: "shapeCloud",
  triangle: "shapeTriangle", dot: "shapeDot", square: "shapeSquare", sprocket: "shapeSprocket", diamond: "shapeDiamond",
};
const FONTS = [["sans", "builderRegular"], ["bold", "builderBold"], ["italic", "builderItalic"], ["mono", "builderMono"]] as const;
const TOKENS = [["", "builderCustom"], ["accent", "builderAccent"], ["text", "builderTextToken"], ["border", "builderBorderToken"], ["bg", "builderBackground"]] as const;

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return <label className="fb-field"><span>{label}</span>{children}</label>;
}
export function Num({ label, value, onChange, step = 0.01, min, max, unit = "in" }: { label: string; value: number; onChange: (v: number) => void; step?: number; min?: number; max?: number; unit?: string }) {
  return (
    <Field label={`${label}${unit ? ` (${unit})` : ""}`}>
      <input type="number" aria-label={label} value={Number.isFinite(value) ? Math.round(value * 1000) / 1000 : 0} step={step} min={min} max={max}
        onChange={(e) => { const v = parseFloat(e.target.value); if (Number.isFinite(v)) onChange(v); }} />
    </Field>
  );
}
const hex = (s?: string) => (s && /^#[0-9a-fA-F]{6}$/.test(s) ? s : "#000000");
export function ColorInput({ label, value, onChange, tokens }: { label: string; value?: string; onChange: (v: string) => void; tokens?: boolean }) {
  const { t } = useT();
  const useLanguage = useLocale();
  const isToken = !!value && !value.startsWith("#");
  return (
    <Field label={label}>
      <span className="fb-color">
        {tokens && <select aria-label={formatTranslation(useLanguage, "labelSource", { label })} value={isToken ? value : ""} onChange={(e) => onChange(e.target.value || hex(undefined))}>{TOKENS.map(([v, k]) => <option key={v} value={v}>{t(k)}</option>)}</select>}
        {!isToken && <>
          <input type="color" aria-label={label} value={hex(value)} onChange={(e) => onChange(e.target.value)} />
          <input type="text" aria-label={formatTranslation(useLanguage, "labelHex", { label })} value={value ?? ""} maxLength={7} onChange={(e) => onChange(e.target.value)} />
        </>}
      </span>
    </Field>
  );
}

/** Choose a PNG/JPEG; the file is validated and copied by the backend, never referenced by path. */
export function ImageImport({ label, onPick, busy, className, hint }: { label: string; onPick: (file: File) => void; busy?: boolean; className?: string; hint?: string }) {
  const { t } = useT();
  const ref = useRef<HTMLInputElement>(null);
  return (
    <>
      <button type="button" className={className} disabled={busy} onClick={() => ref.current?.click()}>
        {busy ? t("builderImporting") : label}
        {hint && <small aria-hidden="true">{hint}</small>}
      </button>
      <input ref={ref} type="file" accept="image/png,image/jpeg" hidden data-testid="file-input" aria-label={label}
        onChange={(e) => { const f = e.target.files?.[0]; if (f) onPick(f); e.target.value = ""; }} />
    </>
  );
}

interface PanelProps {
  draft: FrameDef;
  selection: Selection;
  assets: Record<string, AssetInfo>;
  importing: boolean;
  onChange: (d: FrameDef, key?: string) => void;
  onImport: (use: "overlay" | "background" | "layer", layerId?: string) => (f: File) => void;
}

export function Properties({ draft, selection, assets, importing, onChange, onImport }: PanelProps) {
  const { t, tf } = useT();
  if (selection.type === "slot") {
    const s = draft.slots[selection.index];
    if (!s) return null;
    const set = (p: Partial<Slot>, key?: string) => onChange({ ...draft, slots: draft.slots.map((x, i) => (i === (selection as { index: number }).index ? { ...x, ...p } : x)) }, key);
    return (
      <div className="fb-props" aria-label={t("builderSlotProperties")}>
        <h3>{tf("builderSlotNumber", { number: s.photo + 1 })}</h3>
        <div className="fb-grid2">
          <Num label={t("builderX")} value={s.x} onChange={(v) => set({ x: v }, "sx")} /><Num label={t("builderY")} value={s.y} onChange={(v) => set({ y: v }, "sy")} />
          <Num label={t("builderWidth")} value={s.w} onChange={(v) => set({ w: v }, "sw")} /><Num label={t("builderHeight")} value={s.h} onChange={(v) => set({ h: v }, "sh")} />
        </div>
        <Num label={t("builderPhotoNumber")} unit="" step={1} min={1} value={s.photo + 1} onChange={(v) => set({ photo: Math.max(0, Math.round(v) - 1) }, "sp")} />
        <p className="muted small">{t("builderRepeatPhoto")}</p>
        <Num label={t("builderCropFocusX")} unit="0–1" step={0.05} min={0} max={1} value={s.focusX || 0.5} onChange={(v) => set({ focusX: v }, "fx")} />
        <Num label={t("builderCropFocusY")} unit="0–1" step={0.05} min={0} max={1} value={s.focusY || 0.5} onChange={(v) => set({ focusY: v }, "fy")} />
        <label className="check"><input type="checkbox" checked={!!s.locked} onChange={(e) => set({ locked: e.target.checked })} /> {t("builderLockPosition")}</label>
      </div>
    );
  }
  if (selection.type === "layer") {
    const id = selection.id;
    const l = (draft.layers ?? []).find((x) => x.id === id);
    if (!l) return null;
    const set = (p: Partial<Layer>, key?: string) => onChange({ ...draft, layers: (draft.layers ?? []).map((x) => (x.id === id ? { ...x, ...p } : x)) }, key);
    return (
      <div className="fb-props" aria-label={t("builderLayerProperties")}>
        <h3>{l.name || l.kind}</h3>
        <Field label={t("builderLayerName")}><input aria-label={t("builderLayerName")} value={l.name ?? ""} maxLength={40} onChange={(e) => set({ name: e.target.value }, `n:${id}`)} /></Field>
        <div className="fb-grid2">
          <Num label={t("builderX")} value={l.x} onChange={(v) => set({ x: v }, `x:${id}`)} /><Num label={t("builderY")} value={l.y} onChange={(v) => set({ y: v }, `y:${id}`)} />
          <Num label={t("builderWidth")} value={l.w} onChange={(v) => set({ w: v }, `w:${id}`)} /><Num label={t("builderHeight")} value={l.h} onChange={(v) => set({ h: v }, `h:${id}`)} />
        </div>
        {l.kind === "text" && <>
          <Field label={t("builderText")}><input aria-label={t("builderText")} value={l.text ?? ""} maxLength={200} onChange={(e) => set({ text: e.target.value }, `t:${id}`)} /></Field>
          <p className="muted small">{t("builderCaptionHelp")}</p>
          <Field label={t("builderFont")}><select aria-label={t("builderFont")} value={l.font || "sans"} onChange={(e) => set({ font: e.target.value })}>{FONTS.map(([v, k]) => <option key={v} value={v}>{t(k)}</option>)}</select></Field>
          <Num label={t("builderTextSize")} value={l.sizeIn ?? 0.15} step={0.01} min={0.02} onChange={(v) => set({ sizeIn: v }, `ts:${id}`)} />
          <Field label={t("builderAlign")}><select aria-label={t("builderAlign")} value={l.align || "center"} onChange={(e) => set({ align: e.target.value })}><option value="left">{t("alignLeft")}</option><option value="center">{t("alignCenter")}</option><option value="right">{t("alignRight")}</option></select></Field>
        </>}
        {(l.kind === "shape" || l.kind === "pattern") && (
          <Field label={t(l.kind === "shape" ? "builderShape" : "builderElement")}><select aria-label={t("builderShape")} value={l.shape || "heart"} onChange={(e) => set({ shape: e.target.value })}>{SHAPES.map((s) => <option key={s} value={s}>{t(SHAPE_LABELS[s])}</option>)}</select></Field>
        )}
        {l.kind === "pattern" && <>
          <Num label={t("builderElementSize")} value={l.size ?? 0.1} step={0.01} min={0.02} onChange={(v) => set({ size: v }, `ps:${id}`)} />
          <Num label={t("builderSpacing")} value={l.gap ?? 0.2} step={0.01} min={0} onChange={(v) => set({ gap: v }, `pg:${id}`)} />
          <label className="check"><input type="checkbox" checked={!!l.spread} onChange={(e) => set({ spread: e.target.checked })} /> {t("builderScatter")}</label>
        </>}
        {(l.kind === "rect" || l.kind === "ellipse") && <Num label={t("builderOutlineWidth")} value={l.strokeIn ?? 0} step={0.005} min={0} onChange={(v) => set({ strokeIn: v }, `st:${id}`)} />}
        {l.kind === "rect" && <Num label={t("builderCornerRadius")} value={l.radiusIn ?? 0} step={0.01} min={0} onChange={(v) => set({ radiusIn: v }, `rr:${id}`)} />}
        {l.kind === "image" && <>
          <Field label={t("builderFitMode")}><select aria-label={t("builderFitMode")} value={l.fit || "contain"} onChange={(e) => set({ fit: e.target.value })}><option value="contain">{t("builderContain")}</option><option value="cover">{t("builderCover")}</option></select></Field>
          <ImageImport label={t("builderReplaceImage")} busy={importing} onPick={onImport("layer", id)} />
          {l.asset && assets[l.asset] && <p className="muted small">{assets[l.asset].width}×{assets[l.asset].height} px{assets[l.asset].hasAlpha ? ` · ${t("transparent")}` : ""}</p>}
        </>}
        {l.kind !== "image" && <ColorInput label={t("builderColor")} tokens value={l.color} onChange={(v) => set({ color: v }, `c:${id}`)} />}
        <Num label={t("builderOpacity")} unit="0–1" step={0.05} min={0} max={1} value={l.opacity ?? 1} onChange={(v) => set({ opacity: v }, `o:${id}`)} />
        <label className="check"><input type="checkbox" checked={!!l.under} onChange={(e) => set({ under: e.target.checked })} /> {t("builderBehindPhotos")}</label>
        <label className="check"><input type="checkbox" checked={!!l.hidden} onChange={(e) => set({ hidden: e.target.checked })} /> {t("builderHidden")}</label>
        <label className="check"><input type="checkbox" checked={!!l.locked} onChange={(e) => set({ locked: e.target.checked })} /> {t("builderLockPosition")}</label>
      </div>
    );
  }
  return <FrameProps draft={draft} assets={assets} importing={importing} onChange={onChange} onImport={onImport} />;
}

function Asset({ name, assets, draft, use }: { name?: string; assets: Record<string, AssetInfo>; draft: FrameDef; use: "overlay" | "background" }) {
  const { t } = useT();
  const language = useLocale();
  if (!name) return <p className="muted small">{t("builderNoAsset")}</p>;
  const a = assets[name];
  return (
    <div className="muted small">
      {a ? `${a.width}×${a.height} px · ${a.format.toUpperCase()}${a.hasAlpha ? ` · ${t("transparent")}` : ""}` : name}
      {a && imageWarnings(a, draft, use, language).map((w, i) => <div key={i} className="fb-warn">⚠ {w}</div>)}
    </div>
  );
}

function FrameProps({ draft, assets, importing, onChange, onImport }: Omit<PanelProps, "selection">) {
  const { t } = useT();
  const language = useLocale();
  const set = (p: Partial<FrameDef>, key?: string) => onChange({ ...draft, ...p }, key);
  const [open, setOpen] = useState<string | null>(draft.variants?.[0]?.id ?? null);
  const setVariant = (id: string, p: Partial<Variant>, key?: string) => onChange({ ...draft, variants: (draft.variants ?? []).map((v) => (v.id === id ? { ...v, ...p } : v)) }, key);
  return (
    <div className="fb-props" aria-label={t("builderFrameProperties")}>
      <h3>{t("builderFrame")}</h3>
      <Field label={t("builderLayerName")}><input aria-label={t("builderFrameName")} value={draft.name} maxLength={60} onChange={(e) => set({ name: e.target.value }, "name")} /></Field>
      <Field label={t("builderCategory")}><select aria-label={t("builderCategory")} value={draft.category} onChange={(e) => set({ category: e.target.value })}>{Categories.map((c) => <option key={c} value={c}>{categoryLabel(c, language)}</option>)}</select></Field>
      <Field label={t("builderDescription")}><input aria-label={t("builderDescription")} value={draft.description ?? ""} maxLength={300} onChange={(e) => set({ description: e.target.value }, "desc")} /></Field>
      <label className="check"><input type="checkbox" checked={draft.enabled} onChange={(e) => set({ enabled: e.target.checked })} /> {t("builderOfferInBooth")}</label>
      <label className="check"><input type="checkbox" checked={!!draft.captionEnabled} onChange={(e) => set({ captionEnabled: e.target.checked })} /> {t("builderAllowCaption")}</label>
      <p className="muted small">{draft.widthIn}×{draft.heightIn} in · {translateSlots(draft.slots.length, language)} · {Math.round(draft.widthIn * draft.dpi)}×{Math.round(draft.heightIn * draft.dpi)} px</p>
      <Field label={t("builderPrintResolution")}><select aria-label={t("builderPrintResolution")} value={draft.dpi} onChange={(e) => set({ dpi: Number(e.target.value) })}>{[150, 200, 300, 600].map((d) => <option key={d} value={d}>{d} dpi{d === 300 ? ` ${t("builderRecommended")}` : ""}</option>)}</select></Field>

      <h4>{t("builderColorsBorders")}</h4>
      <ColorInput label={t("builderBackground")} value={draft.background} onChange={(v) => set({ background: v }, "bg")} />
      <Num label={t("builderPhotoBorderWidth")} value={draft.borderIn ?? 0} step={0.005} min={0} max={1} onChange={(v) => set({ borderIn: v }, "bw")} />
      <ColorInput label={t("builderPhotoBorderColor")} value={draft.borderColor} onChange={(v) => set({ borderColor: v }, "bc")} />
      <Num label={t("builderPhotoCornerRadius")} value={draft.cornerIn ?? 0} step={0.01} min={0} max={2} onChange={(v) => set({ cornerIn: v }, "cr")} />
      <ColorInput label={t("builderAccentColor")} value={draft.accent} onChange={(v) => set({ accent: v }, "ac")} />
      <ColorInput label={t("builderTextColor")} value={draft.textColor} onChange={(v) => set({ textColor: v }, "tc")} />

      <h4>{t("builderArtwork")}</h4>
      <div className="fb-art">
        <b>{t("builderBackgroundImage")}</b>
        <Asset name={draft.backgroundImage} assets={assets} draft={draft} use="background" />
        <div className="row"><ImageImport label={t("builderImportBackground")} busy={importing} onPick={onImport("background")} />{draft.backgroundImage && <button onClick={() => set({ backgroundImage: "" })}>{t("builderRemove")}</button>}</div>
      </div>
      <div className="fb-art">
        <b>{t("builderTransparentOverlay")}</b>
        <Asset name={draft.overlay} assets={assets} draft={draft} use="overlay" />
        <div className="row"><ImageImport label={t("builderImportOverlay")} busy={importing} onPick={onImport("overlay")} />{draft.overlay && <button onClick={() => set({ overlay: "" })}>{t("builderRemove")}</button>}</div>
        <p className="muted small">{t("builderOverlayHint")}</p>
      </div>

      <h4>{t("builderColorOptions")}</h4>
      {(draft.variants ?? []).map((v, i) => (
        <div key={v.id} className="fb-variant">
          <button className="ghost" onClick={() => setOpen(open === v.id ? null : v.id)} aria-expanded={open === v.id}>{v.name}{i === 0 ? " (default)" : ""}</button>
          {open === v.id && <>
            <Field label={t("builderOptionName")}><input aria-label={t("builderOptionName")} value={v.name} maxLength={30} onChange={(e) => setVariant(v.id, { name: e.target.value }, `vn:${v.id}`)} /></Field>
            <ColorInput label={t("builderBackground")} value={v.background} onChange={(c) => setVariant(v.id, { background: c }, `vb:${v.id}`)} />
            <ColorInput label={t("builderBorder")} value={v.borderColor} onChange={(c) => setVariant(v.id, { borderColor: c }, `vr:${v.id}`)} />
            <ColorInput label={t("builderAccent")} value={v.accent} onChange={(c) => setVariant(v.id, { accent: c }, `va:${v.id}`)} />
            <ColorInput label={t("builderText")} value={v.textColor} onChange={(c) => setVariant(v.id, { textColor: c }, `vt:${v.id}`)} />
            <button onClick={() => onChange(removeVariant(draft, v.id))}>{t("builderRemoveOption")}</button>
          </>}
        </div>
      ))}
      <button onClick={() => onChange(addVariant(draft))}>{t("builderAddOption")}</button>
    </div>
  );
}
