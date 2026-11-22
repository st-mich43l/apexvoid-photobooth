import { useRef, useState, type ReactNode } from "react";
import { Categories } from "./categories";
import { addVariant, imageWarnings, removeVariant, type Selection } from "./draft";
import type { AssetInfo, FrameDef, Layer, Slot, Variant } from "../types";

const SHAPES = ["heart", "star", "sparkle", "flower", "cloud", "triangle", "dot", "square", "sprocket", "diamond"];
const FONTS = [["sans", "Regular"], ["bold", "Bold"], ["italic", "Italic"], ["mono", "Mono"]];
const TOKENS = [["", "Custom"], ["accent", "Accent"], ["text", "Text colour"], ["border", "Border colour"], ["bg", "Background"]];

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
  const isToken = !!value && !value.startsWith("#");
  return (
    <Field label={label}>
      <span className="fb-color">
        {tokens && <select aria-label={`${label} source`} value={isToken ? value : ""} onChange={(e) => onChange(e.target.value || hex(undefined))}>{TOKENS.map(([v, n]) => <option key={v} value={v}>{n}</option>)}</select>}
        {!isToken && <>
          <input type="color" aria-label={label} value={hex(value)} onChange={(e) => onChange(e.target.value)} />
          <input type="text" aria-label={`${label} hex`} value={value ?? ""} maxLength={7} onChange={(e) => onChange(e.target.value)} />
        </>}
      </span>
    </Field>
  );
}

/** Choose a PNG/JPEG; the file is validated and copied by the backend, never referenced by path. */
export function ImageImport({ label, onPick, busy }: { label: string; onPick: (file: File) => void; busy?: boolean }) {
  const ref = useRef<HTMLInputElement>(null);
  return (
    <>
      <button type="button" disabled={busy} onClick={() => ref.current?.click()}>{busy ? "Importing…" : label}</button>
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
  if (selection.type === "slot") {
    const s = draft.slots[selection.index];
    if (!s) return null;
    const set = (p: Partial<Slot>, key?: string) => onChange({ ...draft, slots: draft.slots.map((x, i) => (i === (selection as { index: number }).index ? { ...x, ...p } : x)) }, key);
    return (
      <div className="fb-props" aria-label="Photo slot properties">
        <h3>Photo slot {s.photo + 1}</h3>
        <div className="fb-grid2">
          <Num label="X" value={s.x} onChange={(v) => set({ x: v }, "sx")} /><Num label="Y" value={s.y} onChange={(v) => set({ y: v }, "sy")} />
          <Num label="Width" value={s.w} onChange={(v) => set({ w: v }, "sw")} /><Num label="Height" value={s.h} onChange={(v) => set({ h: v }, "sh")} />
        </div>
        <Num label="Photo number" unit="" step={1} min={1} value={s.photo + 1} onChange={(v) => set({ photo: Math.max(0, Math.round(v) - 1) }, "sp")} />
        <p className="muted small">Repeat a photo number to show the same photo twice (two-up sheets).</p>
        <Num label="Crop focus X" unit="0–1" step={0.05} min={0} max={1} value={s.focusX || 0.5} onChange={(v) => set({ focusX: v }, "fx")} />
        <Num label="Crop focus Y" unit="0–1" step={0.05} min={0} max={1} value={s.focusY || 0.5} onChange={(v) => set({ focusY: v }, "fy")} />
        <label className="check"><input type="checkbox" checked={!!s.locked} onChange={(e) => set({ locked: e.target.checked })} /> Lock position</label>
      </div>
    );
  }
  if (selection.type === "layer") {
    const id = selection.id;
    const l = (draft.layers ?? []).find((x) => x.id === id);
    if (!l) return null;
    const set = (p: Partial<Layer>, key?: string) => onChange({ ...draft, layers: (draft.layers ?? []).map((x) => (x.id === id ? { ...x, ...p } : x)) }, key);
    return (
      <div className="fb-props" aria-label="Layer properties">
        <h3>{l.name || l.kind}</h3>
        <Field label="Name"><input aria-label="Layer name" value={l.name ?? ""} maxLength={40} onChange={(e) => set({ name: e.target.value }, `n:${id}`)} /></Field>
        <div className="fb-grid2">
          <Num label="X" value={l.x} onChange={(v) => set({ x: v }, `x:${id}`)} /><Num label="Y" value={l.y} onChange={(v) => set({ y: v }, `y:${id}`)} />
          <Num label="Width" value={l.w} onChange={(v) => set({ w: v }, `w:${id}`)} /><Num label="Height" value={l.h} onChange={(v) => set({ h: v }, `h:${id}`)} />
        </div>
        {l.kind === "text" && <>
          <Field label="Text"><input aria-label="Text" value={l.text ?? ""} maxLength={200} onChange={(e) => set({ text: e.target.value }, `t:${id}`)} /></Field>
          <p className="muted small">{"{caption}"} = the guest's caption, {"{date}"} = session date, {"{caption|date}"} = caption, else date.</p>
          <Field label="Font"><select aria-label="Font" value={l.font || "sans"} onChange={(e) => set({ font: e.target.value })}>{FONTS.map(([v, n]) => <option key={v} value={v}>{n}</option>)}</select></Field>
          <Num label="Text size" value={l.sizeIn ?? 0.15} step={0.01} min={0.02} onChange={(v) => set({ sizeIn: v }, `ts:${id}`)} />
          <Field label="Align"><select aria-label="Align" value={l.align || "center"} onChange={(e) => set({ align: e.target.value })}><option>left</option><option>center</option><option>right</option></select></Field>
        </>}
        {(l.kind === "shape" || l.kind === "pattern") && (
          <Field label={l.kind === "shape" ? "Shape" : "Element"}><select aria-label="Shape" value={l.shape || "heart"} onChange={(e) => set({ shape: e.target.value })}>{SHAPES.map((s) => <option key={s}>{s}</option>)}</select></Field>
        )}
        {l.kind === "pattern" && <>
          <Num label="Element size" value={l.size ?? 0.1} step={0.01} min={0.02} onChange={(v) => set({ size: v }, `ps:${id}`)} />
          <Num label="Spacing" value={l.gap ?? 0.2} step={0.01} min={0} onChange={(v) => set({ gap: v }, `pg:${id}`)} />
          <label className="check"><input type="checkbox" checked={!!l.spread} onChange={(e) => set({ spread: e.target.checked })} /> Scatter (confetti)</label>
        </>}
        {(l.kind === "rect" || l.kind === "ellipse") && <Num label="Outline width (0 = filled)" value={l.strokeIn ?? 0} step={0.005} min={0} onChange={(v) => set({ strokeIn: v }, `st:${id}`)} />}
        {l.kind === "rect" && <Num label="Corner radius" value={l.radiusIn ?? 0} step={0.01} min={0} onChange={(v) => set({ radiusIn: v }, `rr:${id}`)} />}
        {l.kind === "image" && <>
          <Field label="Fit"><select aria-label="Fit" value={l.fit || "contain"} onChange={(e) => set({ fit: e.target.value })}><option value="contain">Contain</option><option value="cover">Cover</option></select></Field>
          <ImageImport label="Replace image…" busy={importing} onPick={onImport("layer", id)} />
          {l.asset && assets[l.asset] && <p className="muted small">{assets[l.asset].width}×{assets[l.asset].height} px{assets[l.asset].hasAlpha ? " · transparent" : ""}</p>}
        </>}
        {l.kind !== "image" && <ColorInput label="Colour" tokens value={l.color} onChange={(v) => set({ color: v }, `c:${id}`)} />}
        <Num label="Opacity" unit="0–1" step={0.05} min={0} max={1} value={l.opacity ?? 1} onChange={(v) => set({ opacity: v }, `o:${id}`)} />
        <label className="check"><input type="checkbox" checked={!!l.under} onChange={(e) => set({ under: e.target.checked })} /> Behind the photos</label>
        <label className="check"><input type="checkbox" checked={!!l.hidden} onChange={(e) => set({ hidden: e.target.checked })} /> Hidden</label>
        <label className="check"><input type="checkbox" checked={!!l.locked} onChange={(e) => set({ locked: e.target.checked })} /> Lock position</label>
      </div>
    );
  }
  return <FrameProps draft={draft} assets={assets} importing={importing} onChange={onChange} onImport={onImport} />;
}

function Asset({ name, assets, draft, use }: { name?: string; assets: Record<string, AssetInfo>; draft: FrameDef; use: "overlay" | "background" }) {
  if (!name) return <p className="muted small">None</p>;
  const a = assets[name];
  return (
    <div className="muted small">
      {a ? `${a.width}×${a.height} px · ${a.format.toUpperCase()}${a.hasAlpha ? " · transparent" : ""}` : name}
      {a && imageWarnings(a, draft, use).map((w, i) => <div key={i} className="fb-warn">⚠ {w}</div>)}
    </div>
  );
}

function FrameProps({ draft, assets, importing, onChange, onImport }: Omit<PanelProps, "selection">) {
  const set = (p: Partial<FrameDef>, key?: string) => onChange({ ...draft, ...p }, key);
  const [open, setOpen] = useState<string | null>(draft.variants?.[0]?.id ?? null);
  const setVariant = (id: string, p: Partial<Variant>, key?: string) => onChange({ ...draft, variants: (draft.variants ?? []).map((v) => (v.id === id ? { ...v, ...p } : v)) }, key);
  return (
    <div className="fb-props" aria-label="Frame properties">
      <h3>Frame</h3>
      <Field label="Name"><input aria-label="Frame name" value={draft.name} maxLength={60} onChange={(e) => set({ name: e.target.value }, "name")} /></Field>
      <Field label="Category"><select aria-label="Category" value={draft.category} onChange={(e) => set({ category: e.target.value })}>{Categories.map((c) => <option key={c}>{c}</option>)}</select></Field>
      <Field label="Description"><input aria-label="Description" value={draft.description ?? ""} maxLength={300} onChange={(e) => set({ description: e.target.value }, "desc")} /></Field>
      <label className="check"><input type="checkbox" checked={draft.enabled} onChange={(e) => set({ enabled: e.target.checked })} /> Offer in Booth Mode</label>
      <label className="check"><input type="checkbox" checked={!!draft.captionEnabled} onChange={(e) => set({ captionEnabled: e.target.checked })} /> Let guests type a short caption</label>
      <p className="muted small">{draft.widthIn}×{draft.heightIn} in · {draft.slots.length} photo slot(s) · {Math.round(draft.widthIn * draft.dpi)}×{Math.round(draft.heightIn * draft.dpi)} px</p>
      <Field label="Print resolution"><select aria-label="Print resolution" value={draft.dpi} onChange={(e) => set({ dpi: Number(e.target.value) })}>{[150, 200, 300, 600].map((d) => <option key={d} value={d}>{d} dpi{d === 300 ? " (recommended)" : ""}</option>)}</select></Field>

      <h4>Colours and borders</h4>
      <ColorInput label="Background" value={draft.background} onChange={(v) => set({ background: v }, "bg")} />
      <Num label="Photo border width" value={draft.borderIn ?? 0} step={0.005} min={0} max={1} onChange={(v) => set({ borderIn: v }, "bw")} />
      <ColorInput label="Photo border colour" value={draft.borderColor} onChange={(v) => set({ borderColor: v }, "bc")} />
      <Num label="Photo corner radius" value={draft.cornerIn ?? 0} step={0.01} min={0} max={2} onChange={(v) => set({ cornerIn: v }, "cr")} />
      <ColorInput label="Accent colour" value={draft.accent} onChange={(v) => set({ accent: v }, "ac")} />
      <ColorInput label="Text colour" value={draft.textColor} onChange={(v) => set({ textColor: v }, "tc")} />

      <h4>Artwork</h4>
      <div className="fb-art">
        <b>Background image</b>
        <Asset name={draft.backgroundImage} assets={assets} draft={draft} use="background" />
        <div className="row"><ImageImport label="Import background…" busy={importing} onPick={onImport("background")} />{draft.backgroundImage && <button onClick={() => set({ backgroundImage: "" })}>Remove</button>}</div>
      </div>
      <div className="fb-art">
        <b>Transparent overlay (PNG)</b>
        <Asset name={draft.overlay} assets={assets} draft={draft} use="overlay" />
        <div className="row"><ImageImport label="Import overlay…" busy={importing} onPick={onImport("overlay")} />{draft.overlay && <button onClick={() => set({ overlay: "" })}>Remove</button>}</div>
        <p className="muted small">Drawn over the photos. Transparent areas show them.</p>
      </div>

      <h4>Colour options for guests</h4>
      {(draft.variants ?? []).map((v, i) => (
        <div key={v.id} className="fb-variant">
          <button className="ghost" onClick={() => setOpen(open === v.id ? null : v.id)} aria-expanded={open === v.id}>{v.name}{i === 0 ? " (default)" : ""}</button>
          {open === v.id && <>
            <Field label="Name"><input aria-label="Option name" value={v.name} maxLength={30} onChange={(e) => setVariant(v.id, { name: e.target.value }, `vn:${v.id}`)} /></Field>
            <ColorInput label="Background" value={v.background} onChange={(c) => setVariant(v.id, { background: c }, `vb:${v.id}`)} />
            <ColorInput label="Border" value={v.borderColor} onChange={(c) => setVariant(v.id, { borderColor: c }, `vr:${v.id}`)} />
            <ColorInput label="Accent" value={v.accent} onChange={(c) => setVariant(v.id, { accent: c }, `va:${v.id}`)} />
            <ColorInput label="Text" value={v.textColor} onChange={(c) => setVariant(v.id, { textColor: c }, `vt:${v.id}`)} />
            <button onClick={() => onChange(removeVariant(draft, v.id))}>Remove option</button>
          </>}
        </div>
      ))}
      <button onClick={() => onChange(addVariant(draft))}>Add colour option</button>
    </div>
  );
}
