import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api, toAppError, AppError } from "../api";
import { ErrorNote, Modal } from "../ui";
import type { AssetInfo, Frame, FrameDef, LayerKind } from "../types";
import { Canvas } from "./Canvas";
import { addLayer, addSlot, duplicateLayer, duplicateSlot, moveLayer, removeLayer, removeSlot, type Selection } from "./draft";
import { canRedo, canUndo, newHistory, push, redo, undo } from "./history";
import { ImageImport, Properties } from "./Panels";

interface Props {
  /** The draft to edit (a saved frame, or a fresh copy from NewFrameDraft). */
  initial: FrameDef;
  isNew: boolean;
  onClose: () => void;
  onSaved: (f: Frame) => void;
}

const ELEMENTS: Array<[LayerKind, string]> = [["text", "Text"], ["rect", "Rectangle"], ["ellipse", "Ellipse"], ["shape", "Shape"], ["pattern", "Pattern"]];
const typing = (t: EventTarget | null) => t instanceof HTMLElement && (["INPUT", "TEXTAREA", "SELECT"].includes(t.tagName) || t.isContentEditable);
const readAsDataUrl = (f: File) => new Promise<string>((res, rej) => { const r = new FileReader(); r.onload = () => res(String(r.result)); r.onerror = () => rej(new Error("The file could not be read.")); r.readAsDataURL(f); });

/** Studio Frame Builder: layers on the left, the canvas in the middle, properties on the right. */
export function FrameBuilder({ initial, isNew, onClose, onSaved }: Props) {
  const [hist, setHist] = useState(() => newHistory(initial));
  const draft = hist.present;
  const [sel, setSel] = useState<Selection>({ type: "frame" });
  const [zoom, setZoom] = useState(1);
  const [snap, setSnap] = useState(true);
  const [grid, setGrid] = useState(false);
  const [variant, setVariant] = useState("");
  const [sample, setSample] = useState("");
  const [image, setImage] = useState("");
  const [rendering, setRendering] = useState(false);
  const [previewError, setPreviewError] = useState("");
  const [error, setError] = useState<AppError | null>(null);
  const [saving, setSaving] = useState(false);
  const [importing, setImporting] = useState(false);
  const [assets, setAssets] = useState<Record<string, AssetInfo>>({});
  const [full, setFull] = useState("");
  const [fullOpen, setFullOpen] = useState(false);
  const [askClose, setAskClose] = useState(false);
  const dirty = hist.past.length > 0;

  const change = useCallback((next: FrameDef, key?: string) => { setHist((h) => push(h, next, key)); setError(null); }, []);

  // ---- live preview from the Go renderer (debounced; stale answers are dropped) ----
  const req = useRef(0);
  useEffect(() => {
    const id = ++req.current;
    setRendering(true);
    const t = window.setTimeout(() => {
      api.previewDraft(draft, variant, sample, 560).then((url) => { if (id === req.current) { setImage(url); setPreviewError(""); } })
        .catch((e) => { if (id === req.current) setPreviewError(toAppError(e).message); })
        .finally(() => { if (id === req.current) setRendering(false); });
    }, 160);
    return () => window.clearTimeout(t);
  }, [draft, variant, sample]);

  // ---- editing actions ----
  const select = (s: Selection) => setSel(s);
  const addEl = (kind: LayerKind) => { const r = addLayer(draft, kind); change(r.draft); setSel({ type: "layer", id: r.id }); };
  const addPhotoSlot = () => { const d = addSlot(draft); change(d); setSel({ type: "slot", index: d.slots.length - 1 }); };
  const del = useCallback(() => {
    if (sel.type === "layer") { change(removeLayer(draft, sel.id)); setSel({ type: "frame" }); }
    else if (sel.type === "slot") { const d = removeSlot(draft, sel.index); if (d !== draft) { change(d); setSel({ type: "frame" }); } }
  }, [sel, draft, change]);
  const dup = useCallback(() => {
    if (sel.type === "layer") { const r = duplicateLayer(draft, sel.id); change(r.draft); setSel({ type: "layer", id: r.id }); }
    else if (sel.type === "slot") { const d = duplicateSlot(draft, sel.index); change(d); setSel({ type: "slot", index: d.slots.length - 1 }); }
  }, [sel, draft, change]);
  const nudge = useCallback((dx: number, dy: number) => {
    if (sel.type === "slot") { const s = draft.slots[sel.index]; if (s && !s.locked) change({ ...draft, slots: draft.slots.map((x, i) => (i === sel.index ? { ...x, x: Math.min(Math.max(0, x.x + dx), draft.widthIn - x.w), y: Math.min(Math.max(0, x.y + dy), draft.heightIn - x.h) } : x)) }, "nudge"); }
    else if (sel.type === "layer") { const id = sel.id; change({ ...draft, layers: (draft.layers ?? []).map((l) => (l.id === id && !l.locked ? { ...l, x: l.x + dx, y: l.y + dy } : l)) }, "nudge"); }
  }, [sel, draft, change]);

  useEffect(() => {
    const f = (e: KeyboardEvent) => {
      if (typing(e.target) || fullOpen || askClose) return;
      const mod = e.ctrlKey || e.metaKey;
      if (mod && e.key.toLowerCase() === "z") { e.preventDefault(); setHist((h) => (e.shiftKey ? redo(h) : undo(h))); }
      else if (mod && e.key.toLowerCase() === "y") { e.preventDefault(); setHist(redo); }
      else if (mod && e.key.toLowerCase() === "d") { e.preventDefault(); dup(); }
      else if (e.key === "Delete" || e.key === "Backspace") { e.preventDefault(); del(); }
      else if (e.key === "Escape") { e.preventDefault(); setSel({ type: "frame" }); }
      else if (e.key.startsWith("Arrow")) {
        e.preventDefault();
        const step = e.shiftKey ? 0.1 : 0.01;
        nudge(e.key === "ArrowLeft" ? -step : e.key === "ArrowRight" ? step : 0, e.key === "ArrowUp" ? -step : e.key === "ArrowDown" ? step : 0);
      }
    };
    window.addEventListener("keydown", f);
    return () => window.removeEventListener("keydown", f);
  }, [dup, del, nudge, fullOpen, askClose]);

  // ---- importing images (validated and copied by the backend) ----
  const doImport = (use: "overlay" | "background" | "layer", layerId?: string) => async (file: File) => {
    if (file.size > 15 * 1024 * 1024) { setError(new AppError("That image is larger than 15 MB.", "Choose a smaller PNG or JPEG.")); return; }
    setImporting(true); setError(null);
    try {
      const info = await api.importFrameImage(await readAsDataUrl(file));
      setAssets((a) => ({ ...a, [info.name]: info }));
      if (use === "overlay") change({ ...draft, overlay: info.name });
      else if (use === "background") change({ ...draft, backgroundImage: info.name });
      else if (layerId) change({ ...draft, layers: (draft.layers ?? []).map((l) => (l.id === layerId ? { ...l, asset: info.name } : l)) });
      else {
        const r = addLayer(draft, "image", { asset: info.name, name: "Image", w: Math.min(1, draft.widthIn - 0.2), h: Math.min(1, draft.widthIn - 0.2) * (info.height / info.width) });
        change(r.draft); setSel({ type: "layer", id: r.id });
      }
    } catch (e) { setError(toAppError(e)); } finally { setImporting(false); }
  };

  // ---- save ----
  const save = async (asCopy = false) => {
    setSaving(true); setError(null);
    try {
      let d = draft;
      if (asCopy) {
        const c = await api.newFrameDraft(draft.id, `${draft.name} copy`).catch(() => null);
        d = { ...draft, id: c?.id ?? `${draft.id}-copy`, name: `${draft.name} copy`, revision: 0, enabled: false };
      }
      const saved = await api.saveFrame(d);
      onSaved(saved);
    } catch (e) { setError(toAppError(e)); } finally { setSaving(false); }
  };
  const showFull = async () => {
    setFullOpen(true); setFull("");
    try { setFull(await api.previewDraft(draft, variant, sample, 900)); } catch (e) { setPreviewError(toAppError(e).message); }
  };
  const reset = () => { setHist(newHistory(initial)); setSel({ type: "frame" }); setVariant(""); };
  const close = () => (dirty ? setAskClose(true) : onClose());

  const layers = useMemo(() => [...(draft.layers ?? [])].reverse(), [draft.layers]);
  const selLayer = sel.type === "layer" ? (draft.layers ?? []).find((l) => l.id === sel.id) : undefined;
  const slotSel = (i: number) => sel.type === "slot" && sel.index === i;

  return (
    <div className="builder" role="dialog" aria-modal="true" aria-label="Frame Builder">
      <header className="fb-top">
        <button onClick={close}>← Frames</button>
        <input className="fb-title" aria-label="Frame name (title)" value={draft.name} maxLength={60} onChange={(e) => change({ ...draft, name: e.target.value }, "name")} />
        <span className="fb-spacer" />
        <button onClick={() => setHist(undo)} disabled={!canUndo(hist)} aria-label="Undo" title="Undo (Ctrl+Z)">↶</button>
        <button onClick={() => setHist(redo)} disabled={!canRedo(hist)} aria-label="Redo" title="Redo (Ctrl+Y)">↷</button>
        <label className="check" title="Snap to guides and a 0.05 in grid"><input type="checkbox" checked={snap} onChange={(e) => setSnap(e.target.checked)} /> Snap</label>
        <label className="check"><input type="checkbox" checked={grid} onChange={(e) => setGrid(e.target.checked)} /> Grid</label>
        <div className="seg" role="group" aria-label="Zoom">
          <button onClick={() => setZoom((z) => Math.max(0.4, +(z - 0.2).toFixed(2)))} aria-label="Zoom out">−</button>
          <button className={zoom === 1 ? "on" : ""} onClick={() => setZoom(1)}>Fit</button>
          <button onClick={() => setZoom((z) => Math.min(3, +(z + 0.2).toFixed(2)))} aria-label="Zoom in">+</button>
        </div>
        <button onClick={showFull}>Preview</button>
        <button onClick={reset} disabled={!dirty}>Reset</button>
        {!isNew && <button onClick={() => save(true)} disabled={saving}>Save as copy</button>}
        <button className="primary" onClick={() => save(false)} disabled={saving} aria-label="Save frame">{saving ? "Saving…" : "Save"}</button>
      </header>
      <div className="fb-notes">
        <ErrorNote error={error} onClose={() => setError(null)} />
        {previewError && <div className="note warn" role="status"><div>Preview: {previewError}</div></div>}
      </div>

      <div className="fb-body">
        <aside className="fb-left" aria-label="Layers and elements">
          <h3>Add</h3>
          <div className="fb-add">
            <button onClick={addPhotoSlot}>＋ Photo slot</button>
            {ELEMENTS.map(([k, n]) => <button key={k} onClick={() => addEl(k)}>＋ {n}</button>)}
            <ImageImport label="＋ Image…" busy={importing} onPick={doImport("layer")} />
          </div>
          <h3>Photo slots</h3>
          <ul className="fb-list" aria-label="Photo slots">
            {draft.slots.map((s, i) => (
              <li key={i} className={slotSel(i) ? "on" : ""}>
                <button className="ghost" onClick={() => select({ type: "slot", index: i })}>Photo {s.photo + 1}{s.locked ? " 🔒" : ""}</button>
              </li>
            ))}
          </ul>
          <h3>Layers <small>(front first)</small></h3>
          <ul className="fb-list" aria-label="Layers">
            {layers.length === 0 && <li className="muted small">No layers yet.</li>}
            {layers.map((l) => (
              <li key={l.id} className={sel.type === "layer" && sel.id === l.id ? "on" : ""}>
                <button className="ghost" onClick={() => select({ type: "layer", id: l.id })}>{l.hidden ? "◌ " : ""}{l.name || l.kind}{l.locked ? " 🔒" : ""}{l.under ? " ↓" : ""}</button>
              </li>
            ))}
          </ul>
          <div className="fb-actions">
            <button onClick={() => selLayer && change(moveLayer(draft, selLayer.id, "up"))} disabled={!selLayer} aria-label="Bring forward">Forward</button>
            <button onClick={() => selLayer && change(moveLayer(draft, selLayer.id, "down"))} disabled={!selLayer} aria-label="Send backward">Backward</button>
            <button onClick={dup} disabled={sel.type === "frame"} aria-label="Duplicate element">Duplicate</button>
            <button onClick={del} disabled={sel.type === "frame" || (sel.type === "slot" && draft.slots.length <= 1)} aria-label="Delete element">Delete</button>
          </div>
          {(draft.variants?.length ?? 0) > 1 && (
            <label className="fb-field"><span>Preview colour</span>
              <select aria-label="Preview colour" value={variant} onChange={(e) => setVariant(e.target.value)}>
                {draft.variants!.map((v, i) => <option key={v.id} value={i === 0 ? "" : v.id}>{v.name}</option>)}
              </select></label>
          )}
          {draft.captionEnabled && <label className="fb-field"><span>Sample caption</span><input aria-label="Sample caption" value={sample} maxLength={40} onChange={(e) => setSample(e.target.value)} /></label>}
        </aside>

        <main className="fb-center">
          <Canvas draft={draft} imageSrc={image} zoom={zoom} selection={sel} snap={snap} showGrid={grid} busy={rendering} onSelect={select} onChange={change} />
          <p className="muted small fb-hint">Drag to move, drag the handles to resize. Arrow keys nudge (Shift = 0.1 in). Del removes, Ctrl+Z / Ctrl+Y undo / redo, Ctrl+D duplicates. Previews use stand-in portraits and the same renderer as the final print.</p>
        </main>

        <aside className="fb-right" aria-label="Properties">
          <Properties draft={draft} selection={sel} assets={assets} importing={importing} onChange={change} onImport={doImport} />
        </aside>
      </div>

      {fullOpen && (
        <Modal title="Preview" onClose={() => setFullOpen(false)} wide>
          <div className="viewer">{full ? <img src={full} alt="Full preview" style={{ maxHeight: "74vh" }} /> : <div className="center-msg"><div className="spinner" /></div>}
            <p className="muted small">{draft.widthIn}×{draft.heightIn} in · {Math.round(draft.widthIn * draft.dpi)}×{Math.round(draft.heightIn * draft.dpi)} px at {draft.dpi} dpi. Stand-in portraits, not photographs.</p></div>
        </Modal>
      )}
      {askClose && (
        <Modal title="Discard changes?" onClose={() => setAskClose(false)}>
          <div className="modal-body"><p>This frame has changes that are not saved.</p></div>
          <footer><button onClick={() => setAskClose(false)}>Keep editing</button><button className="danger" onClick={onClose}>Discard</button></footer>
        </Modal>
      )}
    </div>
  );
}
