import { useEffect, useId, useMemo, useRef, useState, type CSSProperties, type KeyboardEvent as ReactKeyboardEvent } from "react";
import { api } from "../../../shared/api";
import { MediaImage } from "../../../shared/MediaImage";
import { bytes, clock, when } from "../../../shared/format";
import { useApp, useT } from "../../../app/state";
import { Badge, ErrorNote, Modal, useAction } from "../../../shared/ui";
import { Icon } from "../../../shared/icons";
import { jobActive, jobKind } from "../../../shared/printInfo";
import type { Media, PrintJob, SessionCard } from "../../../shared/types";
import type { TranslationKey } from "../../../shared/i18n";
import { aspectOf, compositionFormat, defaultComposition, dominantOrientation, groupSessionMedia, photoGridBand, sessionActions, sessionStatusBadge } from "../sessionMedia";
import { PrintDialog, ReframeDialog } from "./MediaViewer";

const jobStatusKey = { submitting: "jobStatusSubmitting", queued: "jobStatusQueued", printing: "jobStatusPrinting", completed: "jobStatusCompleted", removed: "jobStatusRemoved", failed: "jobStatusFailed", unknown: "jobStatusUnknown" } as const;
/** Large tiles use the 1600 px preview; bigger grids keep to the 420 px thumbnails so memory stays bounded. */
const PREVIEW_TILE_LIMIT = 4;

export interface SessionDetailProps {
  session: SessionCard;
  items: Media[];
  mediaLoading: boolean;
  mediaStart: number;
  mediaMore: boolean;
  onPreviousMedia: () => void;
  onNextMedia: () => void;
  jobs: PrintJob[];
  jobsState: "loading" | "error" | "ready";
  onBack: () => void;
  onOpen: (m: Media) => void;
  onExport: (includeEdited: boolean) => void;
  onTrash: () => void;
  onRestore: () => void;
  onReprint: (j: PrintJob) => void;
  onComposed: (m: Media) => void;
}

export function SessionDetail(p: SessionDetailProps) {
  const { session, items, mediaLoading } = p;
  const { t, tf } = useT();
  const { settings } = useApp();
  const language = settings?.language;
  const groups = useMemo(() => groupSessionMedia(items), [items]);
  const [compositionId, setCompositionId] = useState<string | undefined>();
  const [videoId, setVideoId] = useState<string | undefined>();
  const [exporting, setExporting] = useState(false);
  const [reframe, setReframe] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  useEffect(() => {
    setCompositionId(undefined); setVideoId(undefined);
    // the Studio scroll area keeps the list's position; a newly opened session starts at its header
    let el = root.current?.parentElement ?? null;
    while (el && el.scrollHeight <= el.clientHeight) el = el.parentElement;
    if (el && typeof el.scrollTo === "function") el.scrollTo({ top: 0 });
  }, [session.id]);
  const composition = defaultComposition(groups.compositions, compositionId);
  const videoOnly = groups.originals.length === 0 && groups.compositions.length === 0 && groups.videos.length > 0;
  const activeVideo = groups.videos.find((v) => v.id === videoId) ?? groups.videos[0] ?? null;
  const actions = sessionActions(session.status);
  const status = sessionStatusBadge[session.status];
  const format = composition ? formatLabel(composition, t, tf) : "";
  const frameName = composition?.frameName || session.frameName;
  const paged = p.mediaStart > 0 || p.mediaMore;
  const pageRange = paged && items.length ? tf("sdMediaRange", { from: p.mediaStart + 1, to: p.mediaStart + items.length }) : "";
  const showAside = !mediaLoading || items.length > 0;
  const copyId = useCopySessionId();

  return <div className="session-detail" ref={root}>
    <nav className="sd-breadcrumb" aria-label={t("sdBreadcrumb")}>
      <ol>
        <li><button type="button" className="sd-crumb" onClick={p.onBack}><Icon name="back" size={16} />{t("gallery")}</button></li>
        <li aria-current="page"><Icon name="chevright" size={14} className="sd-crumb-sep" />{t("sdSessionDetails")}</li>
      </ol>
    </nav>

    <header className="sd-header">
      <div className="sd-heading">
        <h1><SessionDate iso={session.createdAt} language={language} /></h1>
        <ul className="sd-facts" aria-label={t("sdSessionDetails")}>
          <li><Badge kind={status?.kind ?? "info"}>{status ? t(status.key) : session.status}</Badge></li>
          <li className="sd-fact"><Icon name={session.mode === "video" ? "video" : "photo"} size={15} />{session.mode === "video" ? t("videoMode") : tf(session.photoCount === 1 ? "onePhoto" : "photosCount", { count: session.photoCount })}</li>
          {frameName && <li className="sd-fact"><Icon name="frames" size={15} />{frameName}</li>}
          {format && <li className="sd-fact">{format}</li>}
          {session.simulated && <li><Badge kind="sim">{t("simulatedBadge")}</Badge></li>}
          {session.note === "Operator camera test" && <li><Badge kind="info">{t("testBadge")}</Badge></li>}
        </ul>
      </div>
      <div className="sd-actions" role="group" aria-label={t("sdSessionActions")}>
        {actions.canRestore && <button type="button" onClick={p.onRestore}><Icon name="refresh" size={17} />{t("restoreSession")}</button>}
        <button type="button" className="primary" disabled={!actions.canExport} onClick={() => setExporting(true)}><Icon name="export" size={17} />{t("sdExportSession")}</button>
        <OverflowMenu label={t("sdMoreActions")} items={[
          { id: "copy", label: t("sdCopyId"), icon: "copy", onSelect: () => void copyId(session.id) },
          ...(actions.canRestore ? [] : [{ id: "trash", label: t("trashSession"), icon: "trash", danger: true, disabled: !actions.canTrash, onSelect: p.onTrash }]),
        ]} />
      </div>
    </header>
    <SessionIdLine id={session.id} onCopy={() => void copyId(session.id)} />
    {session.note && session.note !== "Operator camera test" && <p className="note warn sd-note">{session.note}</p>}

    <div className={`sd-workspace${showAside ? "" : " single"}`}>
      <section className="sd-panel sd-main" aria-labelledby="sd-main-title">
        <header className="sd-panel-head">
          <h2 id="sd-main-title">{videoOnly ? t("sdRecordings") : t("sdOriginalPhotos")}</h2>
          <span className="sd-count">{videoOnly ? groups.videos.length : groups.originals.length}</span>
          {pageRange && <span className="sd-range">{pageRange}</span>}
        </header>
        {mediaLoading && items.length === 0 ? <p role="status" className="sd-loading">{t("loadingMedia")}</p>
          : !mediaLoading && items.length === 0 ? <p className="sd-empty">{t("noMedia")}</p>
          : videoOnly ? <VideoGrid fill videos={groups.videos} activeId={activeVideo?.id} onOpen={(v) => { setVideoId(v.id); p.onOpen(v); }} />
          : groups.originals.length > 0 ? <PhotoGrid photos={groups.originals} onOpen={p.onOpen} />
          : <p className="sd-empty">{t("sdNoOriginals")}</p>}
        {!videoOnly && groups.videos.length > 0 && <>
          <header className="sd-panel-head sd-subhead"><h3>{t("sdVideos")}</h3><span className="sd-count">{groups.videos.length}</span></header>
          <VideoGrid videos={groups.videos} onOpen={p.onOpen} />
        </>}
        {mediaLoading && items.length > 0 && <p role="status" className="sd-loading">{t("loadingMedia")}</p>}
        {(p.mediaStart > 0 || p.mediaMore) && <div className="sd-pager">
          <button type="button" onClick={p.onPreviousMedia} disabled={mediaLoading || p.mediaStart === 0}>{t("previousMedia")}</button>
          <button type="button" onClick={p.onNextMedia} disabled={mediaLoading || !p.mediaMore}>{t("nextMedia")}</button>
        </div>}
      </section>
      {showAside && (videoOnly
        ? <RecordingPanel video={activeVideo} number={activeVideo ? groups.videos.indexOf(activeVideo) + 1 : 1} onOpen={p.onOpen} />
        : <CompositionPanel session={session} compositions={groups.compositions} active={composition} hasPhotos={groups.originals.length > 0 || session.photoCount > 0}
          elsewhere={session.hasStrip && groups.compositions.length === 0 && paged} onSelect={setCompositionId} onOpen={p.onOpen} onReframe={() => setReframe(true)} />)}
    </div>

    <PrintHistory jobs={p.jobs} state={p.jobsState} onReprint={p.onReprint} />

    {exporting && <ExportDialog onClose={() => setExporting(false)} onExport={(edited) => { setExporting(false); p.onExport(edited); }} />}
    {reframe && <ReframeDialog sessionId={session.id} frameId={composition?.frameId || session.frameId} variantId={composition?.variantId || session.variantId}
      onClose={() => setReframe(false)} onDone={(m) => { setReframe(false); setCompositionId(m.id); p.onComposed(m); }} />}
  </div>;
}

function formatLabel(m: Media, t: (k: TranslationKey) => string, tf: (k: TranslationKey, v: Record<string, string | number>) => string): string {
  const f = compositionFormat(m.width, m.height);
  if (!f) return "";
  const base = f.kind ? tf(f.kind === "strip" ? "sdFormatStrip" : "sdFormatSheet", { size: f.size }) : tf("sdPixels", { width: m.width, height: m.height });
  return f.orientation === "landscape" ? `${base} · ${t("sdLandscape")}` : base;
}

function SessionDate({ iso, language }: { iso: string; language?: string }) {
  const d = new Date(iso);
  if (isNaN(d.getTime())) return <>{iso}</>;
  const locale = language === "vi" ? "vi-VN" : "en-US";
  return <>
    <span>{d.toLocaleDateString(locale, { weekday: "short", day: "numeric", month: "short", year: "numeric" })}</span>
    <span className="sd-time">{d.toLocaleTimeString(locale, { hour: "numeric", minute: "2-digit" })}</span>
  </>;
}

async function copyText(text: string): Promise<boolean> {
  try { await navigator.clipboard.writeText(text); return true; } catch { /* fall back below */ }
  try {
    const area = document.createElement("textarea");
    area.value = text; area.setAttribute("readonly", ""); area.style.position = "fixed"; area.style.opacity = "0";
    document.body.appendChild(area); area.select();
    const ok = document.execCommand("copy");
    area.remove();
    return ok;
  } catch { return false; }
}

function useCopySessionId() {
  const { toast } = useApp();
  const { t } = useT();
  return async (value: string) => { const ok = await copyText(value); toast(t(ok ? "sdCopied" : "sdCopyFailed"), ok ? "ok" : "err"); };
}

function SessionIdLine({ id, onCopy }: { id: string; onCopy: () => void }) {
  const { t } = useT();
  return <p className="sd-id">
    <span className="sd-id-label">{t("sdSessionId")}</span>
    <code>{id}</code>
    <button type="button" className="sd-ghost" onClick={onCopy} aria-label={t("sdCopyId")}><Icon name="copy" size={15} />{t("sdCopy")}</button>
  </p>;
}

interface MenuItem { id: string; label: string; icon?: string; danger?: boolean; disabled?: boolean; onSelect: () => void }

/** A small menu button: Arrow keys move, Escape and Tab close, focus returns to the trigger. */
export function OverflowMenu({ label, items }: { label: string; items: MenuItem[] }) {
  const [open, setOpen] = useState(false);
  const menuId = useId();
  const trigger = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const enabled = () => Array.from(menu.current?.querySelectorAll<HTMLButtonElement>("[role=menuitem]:not(:disabled)") ?? []);
  useEffect(() => {
    if (!open) return;
    enabled()[0]?.focus();
    const away = (e: PointerEvent) => { if (!menu.current?.contains(e.target as Node) && !trigger.current?.contains(e.target as Node)) setOpen(false); };
    document.addEventListener("pointerdown", away);
    return () => document.removeEventListener("pointerdown", away);
  }, [open]);
  const close = (focus = true) => { setOpen(false); if (focus) trigger.current?.focus(); };
  const keys = (e: ReactKeyboardEvent) => {
    const list = enabled(), at = list.indexOf(document.activeElement as HTMLButtonElement);
    if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); close(); }
    else if (e.key === "Tab") close(false);
    else if (e.key === "ArrowDown") { e.preventDefault(); list[(at + 1) % list.length]?.focus(); }
    else if (e.key === "ArrowUp") { e.preventDefault(); list[(at - 1 + list.length) % list.length]?.focus(); }
    else if (e.key === "Home") { e.preventDefault(); list[0]?.focus(); }
    else if (e.key === "End") { e.preventDefault(); list.at(-1)?.focus(); }
  };
  return <div className="sd-menu">
    <button ref={trigger} type="button" className="sd-icon-btn" aria-label={label} title={label} aria-haspopup="menu" aria-expanded={open} aria-controls={open ? menuId : undefined}
      onClick={() => setOpen((o) => !o)} onKeyDown={(e) => { if (e.key === "ArrowDown" && !open) { e.preventDefault(); setOpen(true); } }}>
      <Icon name="more" size={20} />
    </button>
    {open && <div ref={menu} id={menuId} role="menu" aria-label={label} className="sd-menu-list" onKeyDown={keys}>
      {items.map((it) => <button key={it.id} type="button" role="menuitem" className={it.danger ? "danger-item" : ""} disabled={it.disabled}
        onClick={() => { close(); it.onSelect(); }}>{it.icon && <Icon name={it.icon} size={17} />}<span>{it.label}</span></button>)}
    </div>}
  </div>;
}

function ExportDialog({ onClose, onExport }: { onClose: () => void; onExport: (includeEdited: boolean) => void }) {
  const { t } = useT();
  const [edited, setEdited] = useState(true);
  return <Modal title={t("sdExportSession")} onClose={onClose}>
    <div className="modal-body sd-export">
      <p>{t("sdExportHelp")}</p>
      <label className="sd-check"><input type="checkbox" checked={edited} onChange={(e) => setEdited(e.target.checked)} />
        <span><b>{t("sdIncludeEdited")}</b><small>{t("sdIncludeEditedHint")}</small></span></label>
    </div>
    <footer>
      <button type="button" onClick={onClose}>{t("cancel")}</button>
      <button type="button" className="primary" autoFocus onClick={() => onExport(edited)}><Icon name="folder" size={17} />{t("sdChooseFolder")}</button>
    </footer>
  </Modal>;
}

function PhotoGrid({ photos, onOpen }: { photos: Media[]; onOpen: (m: Media) => void }) {
  const large = photos.length <= PREVIEW_TILE_LIMIT;
  return <div className="sd-photo-grid" data-band={photoGridBand(photos.length)} data-orientation={dominantOrientation(photos)}>
    {photos.map((m) => <PhotoTile key={m.id} media={m} large={large} onOpen={onOpen} />)}
  </div>;
}

function PhotoTile({ media, large, onOpen }: { media: Media; large: boolean; onOpen: (m: Media) => void }) {
  const { mediaUrl, settings } = useApp();
  const { t, tf } = useT();
  const [decoded, setDecoded] = useState<number | null>(null);
  useEffect(() => setDecoded(null), [media.id, media.editedRel]);
  // the decoded file wins: an edit can crop or rotate, which changes the shape the stored dimensions describe
  const ar = decoded ?? aspectOf(media.width, media.height) ?? 4 / 3;
  const number = media.slot + 1;
  const version = media.editedRel || (large ? media.id : media.thumbRel);
  return <button type="button" className={`sd-photo${media.status === "missing" ? " missing" : ""}`} style={{ "--ar": ar } as CSSProperties}
    onClick={() => onOpen(media)} aria-label={tf("sdOpenPhoto", { number, date: when(media.capturedAt, settings?.language) })}>
    <span className="sd-photo-media">
      {media.status === "missing" ? <span className="sd-placeholder err">{t("fileMissing")}</span>
        : (large || media.thumbRel) ? <MediaImage diagnosticId={media.id} src={mediaUrl(media.id, large ? "preview" : "thumb", version)} alt="" manualRetry={false}
          onDecoded={(w, h) => setDecoded(aspectOf(w, h))} />
        : <span className="sd-placeholder">{t("thumbUnavailable")}</span>}
    </span>
    <span className="sd-photo-foot">
      <span className="sd-num">{tf("photoSlot", { number })}</span>
      {media.editedRel && <span className="sd-chip edited">{t("editedTag")}</span>}
      {media.status === "partial" && <span className="sd-chip warn">{t("partialRecording")}</span>}
      {media.status === "missing" && <span className="sd-chip err">{t("fileMissing")}</span>}
    </span>
  </button>;
}

function VideoGrid({ videos, activeId, fill, onOpen }: { videos: Media[]; activeId?: string; fill?: boolean; onOpen: (m: Media) => void }) {
  return <div className={`sd-video-grid${fill ? " fill" : ""}`}>{videos.map((v, i) => <VideoTile key={v.id} video={v} number={i + 1} active={v.id === activeId} onOpen={onOpen} />)}</div>;
}

function VideoTile({ video, number, active, onOpen }: { video: Media; number: number; active?: boolean; onOpen: (m: Media) => void }) {
  const { mediaUrl } = useApp();
  const { t, tf } = useT();
  const ar = aspectOf(video.width, video.height) ?? 16 / 9;
  return <button type="button" className={`sd-video${active ? " active" : ""}`} style={{ "--ar": ar } as CSSProperties} onClick={() => onOpen(video)}
    aria-label={tf("sdPlayVideo", { number, duration: clock(video.durationMs) })}>
    <span className="sd-photo-media">
      {video.status === "missing" ? <span className="sd-placeholder err">{t("fileMissing")}</span>
        : video.thumbRel ? <MediaImage diagnosticId={video.id} src={mediaUrl(video.id, "thumb", video.thumbRel)} alt="" manualRetry={false} />
        : <span className="sd-placeholder"><Icon name="video" size={28} /></span>}
      <span className="sd-play" aria-hidden="true"><Icon name="play" size={20} /></span>
    </span>
    <span className="sd-photo-foot">
      <span className="sd-num">{tf("sdVideoNumber", { number })}</span>
      <span className="sd-chip">{clock(video.durationMs)}</span>
      {video.status === "partial" && <span className="sd-chip warn">{t("partialRecording")}</span>}
    </span>
  </button>;
}

function CompositionPanel({ session, compositions, active, hasPhotos, elsewhere, onSelect, onOpen, onReframe }: {
  session: SessionCard; compositions: Media[]; active: Media | null; hasPhotos: boolean; elsewhere: boolean;
  onSelect: (id: string) => void; onOpen: (m: Media) => void; onReframe: () => void;
}) {
  const { mediaUrl, toast, settings } = useApp();
  const { t, tf } = useT();
  const { error, run, clear } = useAction();
  const [printing, setPrinting] = useState(false);
  const index = active ? compositions.indexOf(active) + 1 : 0;
  const busy = session.status === "active" || session.status === "trashing" || session.status === "trashed";
  return <aside className="sd-panel sd-composition" aria-labelledby="sd-comp-title">
    <header className="sd-panel-head">
      <h2 id="sd-comp-title">{t("sdFinalComposition")}</h2>
      {compositions.length > 1 && <span className="sd-range">{tf("sdCompositionPosition", { index, count: compositions.length })}</span>}
    </header>
    <ErrorNote error={error} onClose={clear} />
    {!active ? <div className="sd-comp-empty">
      <Icon name="frames" size={30} />
      <b>{elsewhere ? t("sdFinalComposition") : t("sdNoComposition")}</b>
      <span>{elsewhere ? t("sdCompositionElsewhere") : hasPhotos ? t("sdNoCompositionHint") : t("sdNoCompositionNoPhotos")}</span>
      {!elsewhere && hasPhotos && <button type="button" disabled={busy} onClick={onReframe}>{t("sdChooseDesign")}</button>}
    </div> : <>
      <button type="button" className="sd-comp-stage" onClick={() => onOpen(active)} aria-label={tf("sdOpenComposition", { index })}>
        <span className="sd-comp-figure" style={{ "--ar": aspectOf(active.width, active.height) ?? 2 / 3 } as CSSProperties}>
          {active.status === "missing" ? <span className="sd-placeholder err">{t("fileMissing")}</span>
            : <MediaImage diagnosticId={active.id} revision={active.editedRel || active.id} src={mediaUrl(active.id, "preview", active.editedRel || active.id)} alt={active.frameName || t("sdFinalComposition")} />}
        </span>
      </button>
      {compositions.length > 1 && <div className="sd-comp-picker" role="group" aria-label={t("sdCompositions")}>
        {compositions.map((c, i) => <button key={c.id} type="button" aria-pressed={c.id === active.id} onClick={() => onSelect(c.id)}
          aria-label={tf("sdSelectComposition", { index: i + 1, format: formatLabel(c, t, tf) || c.frameName })} title={c.frameName}>
          <span className="sd-pick-figure" style={{ "--ar": aspectOf(c.width, c.height) ?? 2 / 3 } as CSSProperties}>
            {c.thumbRel ? <MediaImage diagnosticId={c.id} src={mediaUrl(c.id, "thumb", c.thumbRel)} alt="" manualRetry={false} /> : <span className="sd-placeholder"><Icon name="frames" size={18} /></span>}
          </span>
        </button>)}
      </div>}
      <div className="sd-comp-actions">
        <button type="button" className="primary" disabled={active.status === "missing"} onClick={() => setPrinting(true)}><Icon name="printer" size={17} />{t("printEllipsis")}</button>
        <button type="button" disabled={active.status === "missing"} onClick={() => run(async () => { const path = await api.exportMedia(active.id, "", false); toast(tf("sdImageExported", { path })); })}><Icon name="export" size={17} />{t("sdExportImage")}</button>
        <button type="button" disabled={busy} onClick={onReframe}><Icon name="swap" size={17} />{t("sdChangeDesign")}</button>
      </div>
      <dl className="sd-meta">
        <div><dt>{t("sdFrame")}</dt><dd>{active.frameName || t("noFrame")}</dd></div>
        <div><dt>{t("sdFormat")}</dt><dd>{formatLabel(active, t, tf) || "—"}</dd></div>
        <div><dt>{t("sdResolution")}</dt><dd>{tf("sdPixels", { width: active.width, height: active.height })}</dd></div>
        <div><dt>{t("sdCreated")}</dt><dd>{when(active.capturedAt, settings?.language)}</dd></div>
      </dl>
    </>}
    {printing && active && <PrintDialog media={active} onClose={() => setPrinting(false)} />}
  </aside>;
}

function RecordingPanel({ video, number, onOpen }: { video: Media | null; number: number; onOpen: (m: Media) => void }) {
  const { mediaUrl, toast, settings } = useApp();
  const { t, tf } = useT();
  const { error, run, clear } = useAction();
  if (!video) return null;
  return <aside className="sd-panel sd-composition sd-recording" aria-labelledby="sd-rec-title">
    <header className="sd-panel-head"><h2 id="sd-rec-title">{t("sdRecording")}</h2></header>
    <ErrorNote error={error} onClose={clear} />
    <button type="button" className="sd-comp-stage" onClick={() => onOpen(video)} aria-label={tf("sdPlayVideo", { number, duration: clock(video.durationMs) })}>
      <span className="sd-comp-figure" style={{ "--ar": aspectOf(video.width, video.height) ?? 16 / 9 } as CSSProperties}>
        {video.thumbRel ? <MediaImage diagnosticId={video.id} src={mediaUrl(video.id, "thumb", video.thumbRel)} alt="" manualRetry={false} /> : <span className="sd-placeholder"><Icon name="video" size={32} /></span>}
        <span className="sd-play big" aria-hidden="true"><Icon name="play" size={26} /></span>
      </span>
    </button>
    {video.status === "partial" && <p className="sd-inline-note"><Badge kind="warn">{t("partialRecording")}</Badge></p>}
    <div className="sd-comp-actions">
      <button type="button" className="primary" disabled={video.status === "missing"} onClick={() => onOpen(video)}><Icon name="play" size={17} />{t("sdPlay")}</button>
      <button type="button" disabled={video.status === "missing"} onClick={() => run(async () => { const path = await api.exportMedia(video.id, "", false); toast(tf("sdVideoExported", { path })); })}><Icon name="export" size={17} />{t("sdExportVideo")}</button>
    </div>
    <dl className="sd-meta">
      <div><dt>{t("sdDuration")}</dt><dd>{clock(video.durationMs)}</dd></div>
      <div><dt>{t("sdResolution")}</dt><dd>{tf("sdPixels", { width: video.width, height: video.height })}</dd></div>
      <div><dt>{t("sdFileSize")}</dt><dd>{bytes(video.sizeBytes, settings?.language)}</dd></div>
      <div><dt>{t("sdCreated")}</dt><dd>{when(video.capturedAt, settings?.language)}</dd></div>
    </dl>
  </aside>;
}

function PrintHistory({ jobs, state, onReprint }: { jobs: PrintJob[]; state: "loading" | "error" | "ready"; onReprint: (j: PrintJob) => void }) {
  const { settings } = useApp();
  const { t, tf } = useT();
  return <section className="sd-history" aria-labelledby="sd-history-title">
    <header className="sd-panel-head">
      <Icon name="printer" size={18} className="sd-history-icon" />
      <h2 id="sd-history-title">{t("sdPrintHistory")}</h2>
      {state === "ready" && jobs.length > 0 && <span className="sd-count">{jobs.length === 1 ? t("sdOneJob") : tf("sdJobCount", { count: jobs.length })}</span>}
    </header>
    {state === "loading" ? <p role="status" className="sd-history-empty">{t("sdLoadingJobs")}</p>
      : state === "error" ? <p role="alert" className="sd-history-empty err">{t("jobHistoryError")}</p>
      : jobs.length === 0 ? <p className="sd-history-empty">{t("noJobs")}</p>
      : <ol className="sd-jobs">{jobs.map((j) => <li key={j.id}>
        <span className="sd-job-when">{when(j.createdAt, settings?.language)}</span>
        <span className="sd-job-where"><b>{j.printer}</b><span>{j.paperName}</span><span>{tf("sdCopies", { count: j.copies, copyLabel: t(j.copies === 1 ? "copySingular" : "copyPlural") })}</span>
          {j.parentId && <span>{tf("reprintNumber", { number: j.attempt - 1 })}</span>}</span>
        <Badge kind={jobKind(j.status)}>{t(jobStatusKey[j.status])}</Badge>
        <span className="sd-job-action">{!jobActive(j.status) && j.status !== "unknown" && <button type="button" onClick={() => onReprint(j)}>{t("reprint")}</button>}</span>
      </li>)}</ol>}
    <p className="sd-hint"><Icon name="info" size={15} />{t("sdSpoolerNote")}</p>
  </section>;
}
