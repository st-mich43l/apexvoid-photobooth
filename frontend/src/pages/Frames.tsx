import { useCallback, useEffect, useMemo, useState } from "react";
import { api } from "../api";
import { filterFrames, photoCounts, photosLabel } from "../frameInfo";
import { useApp } from "../state";
import { Badge, ErrorNote, useAction } from "../ui";
import type { Frame } from "../types";

/** Studio: every frame (built-in and custom). The operator chooses which ones guests can pick in Booth Mode. */
export function Frames() {
  const { settings, setSettings, toast, frameUrl } = useApp();
  const [frames, setFrames] = useState<Frame[] | null>(null);
  const [categories, setCategories] = useState<string[]>([]);
  const [problems, setProblems] = useState<string[]>([]);
  const [dir, setDir] = useState("");
  const [category, setCategory] = useState("All");
  const [count, setCount] = useState(0);
  const [onlyOn, setOnlyOn] = useState(false);
  const { error, run } = useAction();

  const load = useCallback(() => run(async () => {
    const r = await api.listFrames(true);
    setFrames(r.frames); setCategories(r.categories); setProblems(r.errors); setDir(r.dir);
  }), []); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { load(); }, [load]);

  const shown = useMemo(() => filterFrames(frames ?? [], { category, photos: count }).filter((f) => !onlyOn || f.enabled), [frames, category, count, onlyOn]);
  const enabledCount = (frames ?? []).filter((f) => f.enabled).length;

  const toggle = (f: Frame, on: boolean) => run(async () => {
    await api.setFrameEnabled(f.id, on);
    setFrames((cur) => cur && cur.map((x) => (x.id === f.id ? { ...x, enabled: on } : x)));
  });
  const bulk = (on: boolean) => run(async () => {
    for (const f of shown) if (f.enabled !== on) await api.setFrameEnabled(f.id, on);
    await load();
    toast(on ? `${shown.length} frames offered in Booth Mode` : `${shown.length} frames hidden from Booth Mode`);
  });
  const makeDefault = (f: Frame) => run(async () => {
    const n = { ...settings, templateId: f.id }; await api.saveSettings(n); setSettings(n); toast(`“${f.name}” is preselected in Booth Mode`);
  });

  return (
    <div className="page">
      <h1>Frames</h1>
      <ErrorNote error={error} />
      <p className="muted">
        A frame is a layout (where the photos go) and a design (colours, borders, artwork). Guests choose one <b>before</b> their photos are taken; it sets how many photos are shot.
        Switch frames on to offer them in Booth Mode. Previews use generated stand-in portraits, not photographs.
        {dir && <> Your own frame files live in <code>{dir}</code>.</>}
      </p>
      {problems.map((e, i) => <div key={i} className="note warn">{e}</div>)}

      <div className="frames-toolbar">
        <div className="filters" role="group" aria-label="Categories">
          {["All", ...categories].map((c) => <button key={c} className={`chip ${category === c ? "on" : ""}`} aria-pressed={category === c} onClick={() => setCategory(c)}>{c}</button>)}
        </div>
        <div className="filters" role="group" aria-label="Number of photos">
          <button className={`chip ${count === 0 ? "on" : ""}`} aria-pressed={count === 0} onClick={() => setCount(0)}>Any photos</button>
          {photoCounts(frames ?? []).map((n) => <button key={n} className={`chip ${count === n ? "on" : ""}`} aria-pressed={count === n} onClick={() => setCount(n)}>{photosLabel(n)}</button>)}
        </div>
        <div className="row">
          <label className="inline"><input type="checkbox" checked={onlyOn} onChange={(e) => setOnlyOn(e.target.checked)} /> Offered only</label>
          <span className="muted small">{enabledCount} of {frames?.length ?? 0} offered · showing {shown.length}</span>
          <button onClick={() => bulk(true)} disabled={!shown.length}>Offer all shown</button>
          <button onClick={() => bulk(false)} disabled={!shown.length}>Hide all shown</button>
        </div>
      </div>

      {!frames && !error && <div className="center-msg"><div className="spinner" />Loading frames…</div>}
      <div className="grid templates">
        {shown.map((f) => (
          <section key={f.id} className={`card tpl ${settings.templateId === f.id ? "selected" : ""} ${f.enabled ? "" : "off"}`}>
            <div className="tpl-preview"><img src={frameUrl(f.id, "", 220)} alt={`${f.name} preview`} loading="lazy" /></div>
            <h3>{f.name} {!f.builtin && <Badge kind="info">custom</Badge>}</h3>
            <p className="muted small">{f.category} · {f.format} · {photosLabel(f.photos)} · {f.pixelW}×{f.pixelH} px @ {f.dpi} dpi</p>
            <label className="inline"><input type="checkbox" checked={f.enabled} onChange={(e) => toggle(f, e.target.checked)} aria-label={`Offer ${f.name} ${f.format} in Booth Mode`} /> Offered in Booth</label>
            <button className={settings.templateId === f.id ? "" : "primary"} disabled={settings.templateId === f.id || !f.enabled} onClick={() => makeDefault(f)}>
              {settings.templateId === f.id ? "Preselected" : "Preselect"}
            </button>
          </section>
        ))}
      </div>
    </div>
  );
}
