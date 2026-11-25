import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api } from "../api";
import { filterFrames, photoCounts, photosLabel } from "../frameInfo";
import { useApp, useT } from "../state";
import { Badge, ErrorNote, useAction } from "../ui";
import { FrameBuilder } from "../builder/FrameBuilder";
import { ImageImport } from "../builder/Panels";
import { Icon } from "../icons";
import { Confirm } from "../ui";
import type { AssetInfo, Frame, FrameDef } from "../types";

/** Studio: every frame (built-in and custom). The operator chooses which ones guests can pick in Booth Mode. */
export function Frames() {
  const { settings, setSettings, toast, frameUrl } = useApp();
  const { t, tf } = useT();
  const [frames, setFrames] = useState<Frame[] | null>(null);
  const [categories, setCategories] = useState<string[]>([]);
  const [problems, setProblems] = useState<string[]>([]);
  const [dir, setDir] = useState("");
  const [category, setCategory] = useState("All");
  const [count, setCount] = useState(0);
  const [visibility, setVisibility] = useState("all");
  const [variants, setVariants] = useState<Record<string, string>>({});
  const [search, setSearch] = useState("");
  const [layoutFilter, setLayoutFilter] = useState("");
  const [source, setSource] = useState("all");
  const [onlyFavorites, setOnlyFavorites] = useState(false);
  const [pendingHide, setPendingHide] = useState<Frame | "bulk" | null>(null);
  const [bulkError, setBulkError] = useState("");
  const { error, run, busy } = useAction();
  const catalogueSaving = useRef(false);
  const [building, setBuilding] = useState<{ def: FrameDef; isNew: boolean; notes?: string[]; assets?: AssetInfo[] } | null>(null);
  const [importing, setImporting] = useState(false);
  const [removing, setRemoving] = useState<Frame | null>(null);
  const [layout, setLayout] = useState("plain.strip4");

  const load = useCallback(() => run(async () => {
    const r = await api.listFrames(true);
    setFrames(r.frames); setCategories(r.categories); setProblems(r.errors); setDir(r.dir);
  }), []); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { load(); }, [load]);

  const shown = useMemo(() => filterFrames(frames ?? [], { category, photos: count }).filter((f) => (visibility === "all" || (visibility === "offered" ? f.enabled : !f.enabled)) && (!search || f.name.toLowerCase().includes(search.toLowerCase())) && (!layoutFilter || f.format === layoutFilter) && (source === "all" || (source === "builtin" ? f.builtin : !f.builtin)) && (!onlyFavorites || f.favorite)), [frames, category, count, visibility, search, layoutFilter, source, onlyFavorites]);
  const enabledCount = (frames ?? []).filter((f) => f.enabled).length;

  const toggle = (f: Frame, on: boolean) => run(async () => {
    await api.setFrameEnabled(f.id, on);
    setFrames((cur) => cur && cur.map((x) => (x.id === f.id ? { ...x, enabled: on } : x)));
  });
  const bulk = (on: boolean) => run(async () => {
    setBulkError("");
    const failures: string[] = []; let changed = 0;
    for (const f of shown) if (f.enabled !== on) { try { await api.setFrameEnabled(f.id, on); changed++; } catch { failures.push(`${f.name} (${f.id})`); } }
    const r = await api.listFrames(true); setFrames(r.frames); setCategories(r.categories); setProblems(r.errors);
    if (failures.length) { setBulkError(tf("bulkUpdateFailed", { changed, frames: failures.join(", ") })); return; }
    toast(tf(on ? "framesOfferedToast" : "framesHiddenToast", { count: changed }));
  });
  const saveCatalogue = (next: Frame[]) => {
    if (catalogueSaving.current) return;
    catalogueSaving.current = true;
    void run(async () => { try {
      const favorites = Object.fromEntries(next.filter((f) => f.favorite).map((f) => [f.id, true]));
      await api.setFrameCatalogue(next.map((f) => f.id), favorites); setFrames(next);
    } finally { catalogueSaving.current = false; } });
  };
  const move = (f: Frame, delta: number) => { if (!frames) return; const i = frames.findIndex((x) => x.id === f.id), j = i + delta; if (j < 0 || j >= frames.length) return; const next = [...frames]; [next[i], next[j]] = [next[j], next[i]]; saveCatalogue(next); };
  const favorite = (f: Frame) => { if (frames) saveCatalogue(frames.map((x) => x.id === f.id ? { ...x, favorite: !x.favorite } : x)); };
  const layouts = useMemo(() => (frames ?? []).filter((f) => f.design === "plain"), [frames]);
  // #frames/new opens the builder straight away (support and screenshots)
  const auto = useRef(false);
  useEffect(() => { if (!auto.current && window.location.hash === "#frames/new" && layouts.length) { auto.current = true; startNew(); } }); // eslint-disable-line react-hooks/exhaustive-deps
  const startNew = () => run(async () => setBuilding({ def: await api.newFrameDraft(layout, t("myFrame")), isNew: true }));
  // Import PNG as frame: pick a picture with transparent windows; photo slots are placed under them
  const importPng = async (file: File) => {
    if (file.size > 15 * 1024 * 1024) { toast(t("oversizedFrameImage"), "err"); return; }
    setImporting(true);
    await run(async () => {
      const url = await new Promise<string>((res, rej) => { const r = new FileReader(); r.onload = () => res(String(r.result)); r.onerror = () => rej(new Error(t("fileReadError"))); r.readAsDataURL(file); });
      const r = await api.importPngFrame(url, file.name);
      setBuilding({ def: r.draft, isNew: true, notes: r.notes, assets: [r.asset] });
    });
    setImporting(false);
  };
  const edit = (f: Frame) => run(async () => setBuilding({ def: await api.getFrame(f.id), isNew: false }));
  const duplicate = (f: Frame) => run(async () => setBuilding({ def: await api.newFrameDraft(f.id), isNew: true }));
  const remove = (f: Frame) => run(async () => { await api.deleteFrame(f.id); setRemoving(null); await load(); toast(tf("frameDeletedToast", { name: f.name })); });
  const makeDefault = (f: Frame) => run(async () => {
    const r = await api.patchSettings({ templateId: f.id }); setSettings(r.settings ?? { ...settings, templateId: f.id }); toast(tf("framePreselectedToast", { name: f.name }));
  });

  return (
    <div className="page">
      <h1>{t("frames")}</h1>
      <ErrorNote error={error} />
      <p className="muted">{t("framesIntro")} {dir && <>{t("ownFrameFilesAt")} <code>{dir}</code>.</>}</p>
      {problems.map((e, i) => <div key={i} className="note warn">{e}</div>)}

      <div className="frames-toolbar">
        <div className="make-grid" aria-label={t("addFrame")}>
          <section className="make-card" aria-label={t("createFrameLabel")}>
            <span className="make-icon" aria-hidden="true"><Icon name="sparkle" size={24} /></span>
            <div className="make-text">
              <h3>{t("designYourOwn")}</h3>
              <p>{t("designHint")}</p>
            </div>
            <div className="make-actions">
              <select aria-label="Layout" value={layout} onChange={(e) => setLayout(e.target.value)}>
                {layouts.map((f) => <option key={f.id} value={f.id}>{f.format} · {photosLabel(f.photos)}</option>)}
              </select>
              <button className="primary" onClick={startNew} disabled={!layouts.length}>{t("createFrame")}</button>
            </div>
          </section>
          <section className="make-card make-import" aria-label={t("havePng")}>
            <span className="make-icon" aria-hidden="true"><Icon name="upload" size={24} /></span>
            <div className="make-text">
              <h3>{t("havePng")}</h3>
              <p>{t("pngHint")}</p>
            </div>
            <div className="make-actions">
              <ImageImport className="primary alt" label={t("importPngFrame")} hint={t("fastestArtwork")} busy={importing} onPick={importPng} />
            </div>
          </section>
        </div>
        <div className="gallery-filters">
          <label>{t("searchFrames")} <input value={search} onChange={(e) => setSearch(e.target.value)} /></label>
          <label>{t("layout")} <select value={layoutFilter} onChange={(e) => setLayoutFilter(e.target.value)}><option value="">{t("allLayouts")}</option>{[...new Set((frames ?? []).map((f) => f.format))].map((f) => <option key={f} value={f}>{f}</option>)}</select></label>
          <label>{t("source")} <select value={source} onChange={(e) => setSource(e.target.value)}><option value="all">{t("allFrames")}</option><option value="builtin">{t("builtIn")}</option><option value="custom">{t("custom")}</option></select></label>
          <label className="inline"><input type="checkbox" checked={onlyFavorites} onChange={(e) => setOnlyFavorites(e.target.checked)} /> {t("favoritesOnly")}</label>
        </div>
        <div className="filters" role="group" aria-label={t("categoriesLabel")}>
          {["All", ...categories].map((c) => <button key={c} className={`chip ${category === c ? "on" : ""}`} aria-pressed={category === c} onClick={() => setCategory(c)}>{c}</button>)}
        </div>
        <div className="filters" role="group" aria-label={t("photoCountFilter")}>
          <button className={`chip ${count === 0 ? "on" : ""}`} aria-pressed={count === 0} onClick={() => setCount(0)}>{t("anyPhotos")}</button>
          {photoCounts(frames ?? []).map((n) => <button key={n} className={`chip ${count === n ? "on" : ""}`} aria-pressed={count === n} onClick={() => setCount(n)}>{photosLabel(n)}</button>)}
        </div>
        <div className="row">
          <label>{t("visibility")} <select value={visibility} onChange={(e) => setVisibility(e.target.value)}><option value="all">{t("all")}</option><option value="offered">{t("offered")}</option><option value="hidden">{t("hidden")}</option></select></label>
          <span className="muted small">{tf("offeredSummary", { enabled: enabledCount, total: frames?.length ?? 0, shown: shown.length })}</span>
          <button onClick={() => bulk(true)} disabled={busy || !shown.length}>{t("offerAll")}</button>
          <button onClick={() => { if (shown.filter((f) => f.enabled).length === enabledCount && enabledCount > 0) setPendingHide("bulk"); else void bulk(false); }} disabled={busy || !shown.length}>{t("hideAll")}</button>
        </div>
        {bulkError && <div className="note err" role="alert">{bulkError}</div>}
      </div>

      {!frames && !error && <div className="center-msg"><div className="spinner" />{t("loadingFrames")}</div>}
      <div className="grid templates">
        {shown.map((f) => (
          <section key={f.id} className={`card tpl ${settings.templateId === f.id ? "selected" : ""} ${f.enabled ? "" : "off"}`}>
            <div className="tpl-preview"><img src={frameUrl(f.id, variants[f.id] ?? "", 220)} alt={tf("framePreviewAlt", { name: f.name })} loading="lazy" /></div>
            <h3>{f.name} {f.editable ? <Badge kind="info">{t("customBadge")}</Badge> : !f.builtin ? <Badge kind="warn">{t("fileBadge")}</Badge> : null}</h3>
            <p className="muted small">{f.category} · {f.format} · {photosLabel(f.photos)} · {f.pixelW}×{f.pixelH} px @ {f.dpi} dpi</p>
            {f.variants && f.variants.length > 1 && <label>{t("previewColor")} <select value={variants[f.id] ?? ""} onChange={(e) => setVariants((cur) => ({ ...cur, [f.id]: e.target.value }))}><option value="">{t("defaultLabel")}</option>{f.variants.map((v) => <option key={v.id} value={v.id}>{v.name}</option>)}</select></label>}
            <label className="inline"><input type="checkbox" checked={f.enabled} disabled={busy} onChange={(e) => { if (!e.target.checked && enabledCount === 1 && f.enabled) setPendingHide(f); else void toggle(f, e.target.checked); }} aria-label={`Offer ${f.name} ${f.format} in Booth Mode`} /> {t("offeredInBooth")}</label>
            <div className="row">
              <button aria-label={tf(f.favorite ? "unpinFrame" : "pinFrame", { name: f.name })} onClick={() => favorite(f)} disabled={busy}>{f.favorite ? `★ ${t("favorite")}` : `☆ ${t("favorite")}`}</button>
              <button aria-label={tf("moveFrameEarlier", { name: f.name })} onClick={() => move(f, -1)} disabled={busy || frames?.[0]?.id === f.id}>↑</button>
              <button aria-label={tf("moveFrameLater", { name: f.name })} onClick={() => move(f, 1)} disabled={busy || frames?.at(-1)?.id === f.id}>↓</button>
              <button className={settings.templateId === f.id ? "" : "primary"} disabled={settings.templateId === f.id || !f.enabled} onClick={() => makeDefault(f)}>
                {settings.templateId === f.id ? t("framePreselected") : t("preselect")}
              </button>
              {f.editable && <button onClick={() => edit(f)} aria-label={`Edit ${f.name}`}>{t("edit")}</button>}
              <button onClick={() => duplicate(f)} aria-label={`Duplicate ${f.name}`}>{t("duplicateAction")}</button>
              {f.editable && <button className="danger" onClick={() => setRemoving(f)} aria-label={`Delete ${f.name}`}>{t("deleteAction")}</button>}
            </div>
          </section>
        ))}
      </div>
      {building && (
        <FrameBuilder initial={building.def} isNew={building.isNew} notes={building.notes} knownAssets={building.assets} onClose={() => setBuilding(null)}
          onSaved={(saved) => { setBuilding(null); load(); toast(`${tf("frameSavedToast", { name: saved.name })}${t(saved.enabled ? "savedAndOffered" : "savedNotOffered")}`); }} />
      )}
      {pendingHide && <Confirm title={t("hideLastTitle")} danger confirmLabel={t("hideFrame")} onNo={() => setPendingHide(null)} onYes={() => { const pending = pendingHide; setPendingHide(null); if (pending === "bulk") void bulk(false); else void toggle(pending, false); }} body={<p>{t("hideLastHint")}</p>} />}
      {removing && (
        <Confirm title={t("deleteFrameTitle")} danger confirmLabel={t("confirmDeleteFrame")} onNo={() => setRemoving(null)} onYes={() => remove(removing)}
          body={<p>{tf("deleteFrameHelp", { name: removing.name })}</p>} />
      )}
    </div>
  );
}
