import { useCallback, useEffect, useMemo, useState } from "react";
import { api, toAppError } from "./api";
import { choiceOf, explainFit, filterFrames, fitFor, photoCounts, photosLabel, swatch } from "./frameInfo";
import { useApp } from "./state";
import type { Frame, FrameChoice } from "./types";

export interface FrameDecision { useFirst: boolean; restart: boolean }

interface Props {
  /** "booth": large guest UI. "studio": the same screen inside the operator pages. */
  variant: "booth" | "studio";
  initial?: { id: string; variantId: string; caption?: string };
  /** After capture: how many photos exist. Frames for another number are explained, never silently swapped in. */
  takenPhotos?: number;
  title?: string;
  confirmLabel?: string;
  backLabel?: string;
  busy?: boolean;
  onConfirm: (c: FrameChoice, d: FrameDecision) => void;
  onBack?: () => void;
}

/** Frame selection: browse, preview, pick a colour, confirm. Used before the camera starts. */
export function FrameSelection({ variant, initial, takenPhotos, title, confirmLabel, backLabel, busy, onConfirm, onBack }: Props) {
  const { settings, frameUrl } = useApp();
  const [frames, setFrames] = useState<Frame[] | null>(null);
  const [categories, setCategories] = useState<string[]>([]);
  const [error, setError] = useState("");
  const [category, setCategory] = useState("All");
  const [count, setCount] = useState(0);
  const [pickId, setPickId] = useState(initial?.id ?? "");
  const [variantId, setVariantId] = useState(initial?.variantId ?? "");
  const [caption, setCaption] = useState(initial?.caption ?? "");

  const load = useCallback(() => {
    setError(""); setFrames(null);
    api.listFrames(false).then((r) => {
      setFrames(r.frames); setCategories(r.categories);
      setPickId((cur) => {
        if (cur && r.frames.some((f) => f.id === cur)) return cur;
        const pre = r.frames.find((f) => f.id === (settings.templateId || r.default)) ?? r.frames.find((f) => takenPhotos ? f.photos === takenPhotos : true);
        return pre?.id ?? "";
      });
    }).catch((e) => setError(toAppError(e).message));
  }, [settings.templateId, takenPhotos]);
  useEffect(load, [load]);

  const shown = useMemo(() => filterFrames(frames ?? [], { category, photos: count }), [frames, category, count]);
  const picked = frames?.find((f) => f.id === pickId);
  const fit = picked ? fitFor(picked, takenPhotos) : { kind: "ok" as const };
  const booth = variant === "booth";

  const select = (f: Frame) => {
    if (f.id !== pickId) { setPickId(f.id); setVariantId(""); }
  };
  const confirm = (d: Partial<FrameDecision>) => picked && onConfirm(choiceOf(picked, variantId, caption), { useFirst: false, restart: false, ...d });

  return (
    <div className={`frames ${booth ? "booth" : "studio"}`} data-testid="frame-selection">
      <header className="frames-head">
        {onBack && <button className="back" onClick={onBack} disabled={busy}>← {backLabel ?? "Back"}</button>}
        <h1>{title ?? "Choose your frame"}</h1>
      </header>

      {error && (
        <div className="note err" role="alert"><div><strong>The frames could not be loaded.</strong><div className="action">{error}</div></div><button onClick={load}>Try again</button></div>
      )}
      {!error && !frames && <div className="center-msg" role="status"><div className="spinner" />Loading frames…</div>}
      {frames && frames.length === 0 && (
        <div className="note warn" role="status"><div>No frames are available{booth ? ". Please ask the host." : ". Enable some on the Frames page."}</div></div>
      )}

      {frames && frames.length > 0 && (
        <div className="frames-body">
          <div className="frames-list">
            <div className="filters" role="group" aria-label="Frame categories">
              {["All", ...categories].map((c) => (
                <button key={c} className={`chip ${category === c ? "on" : ""}`} aria-pressed={category === c} onClick={() => setCategory(c)}>{c}</button>
              ))}
            </div>
            {photoCounts(frames).length > 1 && (
              <div className="filters" role="group" aria-label="Number of photos">
                <button className={`chip ${count === 0 ? "on" : ""}`} aria-pressed={count === 0} onClick={() => setCount(0)}>Any photos</button>
                {photoCounts(frames).map((n) => (
                  <button key={n} className={`chip ${count === n ? "on" : ""}`} aria-pressed={count === n} onClick={() => setCount(n)}>{photosLabel(n)}</button>
                ))}
              </div>
            )}
            {shown.length === 0 && <p className="muted">No frames match this filter.</p>}
            <div className="frame-grid" role="radiogroup" aria-label="Frames">
              {shown.map((f) => {
                const mis = fitFor(f, takenPhotos).kind !== "ok";
                return (
                  <button key={f.id} role="radio" aria-checked={f.id === pickId} className={`frame-card ${f.id === pickId ? "on" : ""} ${mis ? "mismatch" : ""}`} onClick={() => select(f)}>
                    <img src={frameUrl(f.id, "", 360)} alt={`${f.name} frame`} loading="lazy" />
                    <span className="fname">{f.name}</span>
                    <span className="fmeta">{f.format} · {photosLabel(f.photos)}</span>
                    {mis && <span className="fwarn">needs {photosLabel(f.photos)}</span>}
                  </button>
                );
              })}
            </div>
          </div>

          <aside className="frame-detail" aria-label="Selected frame">
            {picked ? (
              <>
                <div className="big-preview"><img src={frameUrl(picked.id, variantId, 640, picked.captionEnabled ? caption.trim() : "")} alt={`${picked.name} preview`} /></div>
                <h2>{picked.name}</h2>
                <p className="muted">{picked.format} · {photosLabel(picked.photos)} · {picked.pixelW}×{picked.pixelH} px</p>
                {picked.variants && picked.variants.length > 1 && (
                  <div className="swatches" role="radiogroup" aria-label="Colour">
                    {picked.variants.map((v, i) => {
                      const on = variantId === v.id || (!variantId && i === 0);
                      return (
                        <button key={v.id} role="radio" aria-checked={on} aria-label={v.name} title={v.name} className={`swatch ${on ? "on" : ""}`}
                          style={{ background: i === 0 ? picked.background : swatch(v, picked) }} onClick={() => setVariantId(i === 0 ? "" : v.id)} />
                      );
                    })}
                  </div>
                )}
                {picked.captionEnabled && (
                  <label className="caption-line">Short caption (optional)
                    <input value={caption} maxLength={40} placeholder="e.g. Mai & Nam" onChange={(e) => setCaption(e.target.value)} aria-label="Caption" />
                  </label>
                )}
                {fit.kind !== "ok" && <div className="note warn" role="status"><div>{explainFit(fit, picked.name)}</div></div>}
                <div className="frame-actions">
                  {fit.kind === "ok" && (
                    <button className="primary big" disabled={busy} onClick={() => confirm({})}>{busy ? "Please wait…" : (confirmLabel ?? "Continue")}</button>
                  )}
                  {fit.kind === "fewer" && <button className="primary big" disabled={busy} onClick={() => confirm({ useFirst: true })}>Use the first {fit.need} photos</button>}
                  {fit.kind === "more" && <button className="primary big" disabled={busy} onClick={() => confirm({ restart: true })}>Start over with this frame</button>}
                </div>
              </>
            ) : <p className="muted">Tap a frame to see it here.</p>}
          </aside>
        </div>
      )}
    </div>
  );
}
