import { useLocale } from "../../../shared/locale";
import { translate } from "../../../shared/i18n";
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import type { FrameDef } from "../../../shared/types";
import type { Selection } from "./draft";
import { applyDrag, guidesFor, HANDLES, pct, pxToIn, type Box, type Handle } from "./geometry";

interface Props {
  draft: FrameDef;
  imageSrc: string;
  zoom: number;          // 1 = fit the stage
  selection: Selection;
  snap: boolean;
  showGrid: boolean;
  busy: boolean;         // the preview is being re-rendered
  onSelect: (s: Selection) => void;
  /** Called once when a drag finishes (and for keyboard nudges). */
  onChange: (next: FrameDef, key?: string) => void;
}

const GRID = 0.25; // inches (display grid); snapping uses a finer 0.05 step

interface Item { key: string; sel: Selection; box: Box; kind: "slot" | "layer"; label: string; locked: boolean; hidden: boolean }

function items(d: FrameDef): Item[] {
  const out: Item[] = d.slots.map((s, i) => ({ key: `s${i}`, sel: { type: "slot", index: i }, box: { x: s.x, y: s.y, w: s.w, h: s.h }, kind: "slot", label: `${s.photo + 1}`, locked: !!s.locked, hidden: false }));
  for (const l of d.layers ?? []) out.push({ key: `l${l.id}`, sel: { type: "layer", id: l.id }, box: { x: l.x, y: l.y, w: l.w, h: l.h }, kind: "layer", label: l.name || l.kind, locked: !!l.locked, hidden: !!l.hidden });
  return out;
}

const same = (a: Selection, b: Selection) => a.type === b.type && (a.type === "frame" || (a.type === "slot" && b.type === "slot" && a.index === b.index) || (a.type === "layer" && b.type === "layer" && a.id === b.id));

/** The editing surface. The picture is rendered by the Go renderer; boxes and handles are drawn on top in percent of the canvas, so screen pixels always map exactly to print inches. */
export function Canvas({ draft, imageSrc, zoom, selection, snap, showGrid, busy, onSelect, onChange }: Props) {
  const language = useLocale();
  const stageRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLDivElement>(null);
  const [stage, setStage] = useState({ w: 600, h: 600 });
  const [live, setLive] = useState<{ item: string; box: Box; gx?: number; gy?: number } | null>(null);
  const drag = useRef<{ key: string; handle: Handle; sx: number; sy: number; start: Box; sel: Selection; moved: boolean } | null>(null);

  useLayoutEffect(() => {
    const el = stageRef.current; if (!el) return;
    const read = () => setStage({ w: el.clientWidth, h: el.clientHeight });
    read();
    const ro = typeof ResizeObserver !== "undefined" ? new ResizeObserver(read) : null;
    ro?.observe(el);
    return () => ro?.disconnect();
  }, []);

  const W = draft.widthIn, H = draft.heightIn;
  const fitW = Math.max(60, Math.min(stage.w - 32, (stage.h - 32) * (W / H)));
  const shownW = Math.round(fitW * zoom), shownH = Math.round((fitW * zoom * H) / W);
  const list = useMemo(() => items(draft), [draft]);

  const move = (e: PointerEvent) => {
    const d = drag.current, cv = canvasRef.current; if (!d || !cv) return;
    const rect = cv.getBoundingClientRect();
    const dx = pxToIn(e.clientX - d.sx, rect.width, W), dy = pxToIn(e.clientY - d.sy, rect.width, W);
    if (!d.moved && Math.hypot(e.clientX - d.sx, e.clientY - d.sy) < 3) return;
    d.moved = true;
    const others = list.filter((i) => i.key !== d.key && !i.hidden).map((i) => i.box);
    const g = guidesFor(W, H, others);
    const r = applyDrag(d.start, d.handle, dx, dy, { W, H, snap, grid: 0.05, threshold: 0.06, guidesX: g.x, guidesY: g.y, clamp: d.sel.type === "slot" });
    setLive({ item: d.key, box: r.box, gx: r.guideX, gy: r.guideY });
  };
  const up = () => {
    const d = drag.current; drag.current = null;
    setLive((cur) => {
      if (d && d.moved && cur) {
        const b = cur.box;
        if (d.sel.type === "slot") onChange({ ...draft, slots: draft.slots.map((s, i) => (d.sel.type === "slot" && i === d.sel.index ? { ...s, ...b } : s)) });
        else if (d.sel.type === "layer") { const id = d.sel.id; onChange({ ...draft, layers: (draft.layers ?? []).map((l) => (l.id === id ? { ...l, ...b } : l)) }); }
      }
      return null;
    });
  };
  // window listeners must be stable functions that call the latest closures
  const latest = useRef({ move, up });
  latest.current = { move, up };
  const onMove = useRef((e: PointerEvent) => latest.current.move(e)).current;
  const onUp = useRef(() => { window.removeEventListener("pointermove", onMove); window.removeEventListener("pointerup", onUp); latest.current.up(); }).current;
  useEffect(() => () => { window.removeEventListener("pointermove", onMove); window.removeEventListener("pointerup", onUp); }, [onMove, onUp]);

  const begin = (e: React.PointerEvent, it: Item, handle: Handle) => {
    e.stopPropagation();
    onSelect(it.sel);
    if (it.locked || e.button !== 0) return;
    drag.current = { key: it.key, handle, sx: e.clientX, sy: e.clientY, start: it.box, sel: it.sel, moved: false };
    window.addEventListener("pointermove", onMove); window.addEventListener("pointerup", onUp);
  };

  return (
    <div className="fb-stage" ref={stageRef} onPointerDown={() => onSelect({ type: "frame" })} data-testid="fb-stage">
      <div className="fb-canvas" ref={canvasRef} style={{ width: shownW, height: shownH }} data-testid="fb-canvas"
        data-w={shownW} data-h={shownH}>
        {imageSrc ? <img src={imageSrc} alt={translate(language, "builderFramePreview")} draggable={false} className={busy ? "busy" : ""} /> : <div className="fb-loading"><div className="spinner" /></div>}
        {showGrid && <div className="fb-grid" style={{ backgroundSize: `${(GRID / W) * 100}% ${(GRID / H) * 100}%` }} />}
        {list.filter((i) => !i.hidden).map((it) => {
          const isSel = same(it.sel, selection);
          const box = live && live.item === it.key ? live.box : it.box;
          return (
            <div key={it.key} data-testid={`fb-${it.key}`} className={`fb-box ${it.kind} ${isSel ? "sel" : ""} ${it.locked ? "locked" : ""}`} style={pct(box, W, H)}
              onPointerDown={(e) => begin(e, it, "move")} title={it.label}>
              {it.kind === "slot" && <span className="fb-badge">{it.label}{it.locked ? " 🔒" : ""}</span>}
              {isSel && !it.locked && HANDLES.map((h) => (
                <i key={h} className={`fb-handle h-${h}`} data-handle={h} onPointerDown={(e) => begin(e, it, h)} />
              ))}
            </div>
          );
        })}
        {live?.gx !== undefined && <div className="fb-guide v" style={{ left: `${(live.gx / W) * 100}%` }} />}
        {live?.gy !== undefined && <div className="fb-guide h" style={{ top: `${(live.gy / H) * 100}%` }} />}
      </div>
    </div>
  );
}
