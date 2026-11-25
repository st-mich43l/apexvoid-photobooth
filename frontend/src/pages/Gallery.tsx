import { useEffect, useRef, useState } from "react";
import { api } from "../api";
import { bytes, clock, when } from "../format";
import { useApp, useT } from "../state";
import { Badge, Confirm, ErrorNote, Modal, useAction } from "../ui";
import type { Media, PrintJob, SessionCard, SessionQuery } from "../types";
import { Editor } from "./Editor";
import { PrintPanel } from "../PrintPanel";
import { FrameSelection } from "../FrameSelection";
import type { FrameChoice } from "../types";
import { jobActive } from "../printInfo";
import type { TranslationKey } from "../i18n";

const initialQuery: SessionQuery = { search: "", mode: "", status: "", simulated: "all", from: "", to: "", cursor: "", limit: 40 };
const jobStatusKey = { submitting: "jobStatusSubmitting", queued: "jobStatusQueued", printing: "jobStatusPrinting", completed: "jobStatusCompleted", removed: "jobStatusRemoved", failed: "jobStatusFailed", unknown: "jobStatusUnknown" } as const;
const sessionStatusKeys: Record<string, TranslationKey> = {
  active: "sessionActive", completed: "sessionCompleted", cancelled: "statusSessionCancelled", failed: "sessionFailed",
  interrupted: "sessionInterrupted", trashing: "sessionTrashing", trashed: "sessionTrashed",
};

export function Gallery() {
  const { mediaUrl, libraryVersion, bumpLibrary, toast, settings } = useApp();
  const { t, tf } = useT();
  const [query, setQuery] = useState(initialQuery);
  const [sessions, setSessions] = useState<SessionCard[]>([]);
  const [cursor, setCursor] = useState("");
  const [pageCursor, setPageCursor] = useState("");
  const [history, setHistory] = useState<string[]>([]);
  const [more, setMore] = useState(false);
  const [loading, setLoading] = useState(false);
  const [selected, setSelected] = useState<SessionCard | null>(null);
  const [items, setItems] = useState<Media[]>([]);
  const [mediaOffset, setMediaOffset] = useState(0);
  const [mediaStart, setMediaStart] = useState(0);
  const [mediaMore, setMediaMore] = useState(false);
  const [mediaLoading, setMediaLoading] = useState(false);
  const [open, setOpen] = useState<Media | null>(null);
  const [confirm, setConfirm] = useState<Media | null>(null);
  const [trashSession, setTrashSession] = useState(false);
  const [includeEdited, setIncludeEdited] = useState(true);
  const [printJobs, setPrintJobs] = useState<PrintJob[]>([]);
  const [jobsError, setJobsError] = useState(false);
  const [confirmPrint, setConfirmPrint] = useState<PrintJob | null>(null);
  const request = useRef(0); const mediaRequest = useRef(0); const pendingViewer = useRef<"first" | "last" | null>(null);
  const { error, run, clear } = useAction();

  useEffect(() => {
    const id = ++request.current; setLoading(true); setSessions([]); setCursor(""); setMore(false);
    const timer = window.setTimeout(() => { api.querySessions({ ...query, cursor: pageCursor }).then((p) => {
      if (request.current !== id) return; setSessions(p.items); setCursor(p.nextCursor); setMore(p.hasMore);
    }).catch((e) => { if (request.current === id) void run(async () => { throw e; }); }).finally(() => { if (request.current === id) setLoading(false); }); }, 150);
    return () => { window.clearTimeout(timer); request.current++; };
  }, [query.search, query.mode, query.status, query.simulated, query.from, query.to, pageCursor, libraryVersion]); // eslint-disable-line react-hooks/exhaustive-deps
  const nextPage = () => { setHistory((h) => [...h, pageCursor]); setPageCursor(cursor); };
  const previousPage = () => { const previous = history.at(-1) ?? ""; setHistory((h) => h.slice(0, -1)); setPageCursor(previous); };
  useEffect(() => {
    const id = ++mediaRequest.current; setItems([]); setMediaOffset(0); setMediaMore(false);
    if (!selected) { setMediaLoading(false); return; }
    setMediaLoading(true); setPrintJobs([]); setJobsError(false);
    api.listSessionPrintJobs(selected.id).then((jobs) => { if (id === mediaRequest.current) setPrintJobs(jobs); }).catch(() => { if (id === mediaRequest.current) setJobsError(true); });
    api.querySessionMedia(selected.id, 50, mediaStart).then((p) => { if (id === mediaRequest.current) { setItems(p.items); setMediaOffset(p.nextOffset); setMediaMore(p.hasMore); if (pendingViewer.current) { setOpen(p.items[pendingViewer.current === "first" ? 0 : p.items.length - 1] ?? null); pendingViewer.current = null; } } })
      .catch((e) => { if (id === mediaRequest.current) void run(async () => { throw e; }); }).finally(() => { if (id === mediaRequest.current) setMediaLoading(false); });
    return () => { mediaRequest.current++; };
  }, [selected?.id, mediaStart, libraryVersion]); // eslint-disable-line react-hooks/exhaustive-deps
  const update = (patch: Partial<SessionQuery>) => { setPageCursor(""); setHistory([]); setQuery((q) => ({ ...q, ...patch, cursor: "" })); };
  const del = (m: Media) => run(async () => { await api.trashMedia(m.id); setConfirm(null); setOpen(null); bumpLibrary(); toast(t("mediaMovedToTrash"), "ok", () => { void run(async () => { await api.restoreMedia(m.id); bumpLibrary(); }); }); });
  const exportSession = () => selected && run(async () => { const folder = await api.pickFolder(); if (!folder) return; const p = await api.exportSession(selected.id, folder, includeEdited); toast(tf("sessionExported", { path: p })); });
  const trash = () => selected && run(async () => { const id = selected.id; try { await api.trashSession(id); } catch (e) { setTrashSession(false); setSelected(null); bumpLibrary(); throw e; } setTrashSession(false); setSelected(null); bumpLibrary(); toast(t("sessionMovedToTrash"), "ok", () => { void run(async () => { await api.restoreSession(id); bumpLibrary(); }); }); });
  const activeIndex = open ? items.findIndex((m) => m.id === open.id) : -1;
  const sessionStatusLabel = (status: string) => sessionStatusKeys[status] ? t(sessionStatusKeys[status]) : status;

  return <div className="page">
    <header className="row"><h1>{t("gallery")}</h1>{selected && <button onClick={() => { setSelected(null); setOpen(null); }}>{t("allSessions")}</button>}</header>
    <ErrorNote error={error} onClose={clear} />
    {!selected ? <>
      <div className="gallery-filters" aria-label={t("galleryFilters")}>
        <label>{t("searchSession")} <input value={query.search} onChange={(e) => update({ search: e.target.value })} placeholder={t("sessionIdPlaceholder")} /></label>
        <label>{t("mode")} <select value={query.mode} onChange={(e) => update({ mode: e.target.value })}><option value="">{t("allModes")}</option><option value="photo">{t("photoMode")}</option><option value="video">{t("videoMode")}</option></select></label>
        <label>{t("status")} <select value={query.status} onChange={(e) => update({ status: e.target.value })}><option value="">{t("allStatuses")}</option>{["active", "completed", "cancelled", "failed", "interrupted", "trashing", "trashed"].map((x) => <option key={x} value={x}>{sessionStatusLabel(x)}</option>)}</select></label>
        <label>{t("sessions")} <select value={query.simulated} onChange={(e) => update({ simulated: e.target.value })}><option value="all">{t("all")}</option><option value="real">{t("real")}</option><option value="simulated">{t("simulated")}</option><option value="test">{t("operatorTests")}</option></select></label>
        <label>{t("fromUtc")} <input type="date" value={query.from} onChange={(e) => update({ from: e.target.value })} /></label><label>{t("toUtc")} <input type="date" value={query.to} onChange={(e) => update({ to: e.target.value })} /></label>
        <button onClick={() => { setPageCursor(""); setHistory([]); setQuery({ ...initialQuery }); }}>{t("clearFilters")}</button>
      </div>
      {!loading && sessions.length === 0 && <p className="muted empty-state">{t("noSessions")}</p>}
      <div className="session-cards">{sessions.map((s) => <button className="session-card" key={s.id} onClick={() => { pendingViewer.current = null; setMediaStart(0); setSelected(s); }}>
        <div className="session-card-image">{s.thumbnailId ? <Thumbnail src={mediaUrl(s.thumbnailId, "thumb", s.thumbnailVersion)} fallback={t("noThumbnail")} /> : <span className="muted">{t("noThumbnail")}</span>}</div>
        <strong>{when(s.createdAt, settings?.language)}</strong><span>{t(s.mode === "photo" ? "photoMode" : "videoMode")} · {tf(s.photoCount === 1 ? "onePhoto" : "photosCount", { count: s.photoCount })} {s.hasStrip ? `· ${t("composition")}` : `· ${t("compositionMissing")}`}</span><span>{s.frameName || t("noFrame")} · {sessionStatusLabel(s.status)}</span><small>{s.id}</small>{s.simulated && <Badge kind="sim">{t("simulatedBadge")}</Badge>}{s.note === "Operator camera test" && <Badge kind="info">{t("testBadge")}</Badge>}
      </button>)}</div>
      {loading && <p role="status">{t("loadingSessions")}</p>}<div className="row">{history.length > 0 && <button onClick={previousPage} disabled={loading}>{t("previousSessions")}</button>}{more && <button onClick={nextPage} disabled={loading}>{t("nextSessions")}</button>}</div>
    </> : <>
      <section className="card session-summary"><h2>{when(selected.createdAt, settings?.language)} {selected.simulated && <Badge kind="sim">{t("simulatedBadge")}</Badge>}</h2>
        <p>{selected.id} · {t(selected.mode === "photo" ? "photoMode" : "videoMode")} · {sessionStatusLabel(selected.status)} · {selected.frameName || t("noFrame")} · {tf(selected.photoCount === 1 ? "onePhoto" : "photosCount", { count: selected.photoCount })}</p>
        {selected.note && <p className="note warn">{selected.note}</p>}
        <div className="row"><label className="inline"><input type="checkbox" checked={includeEdited} onChange={(e) => setIncludeEdited(e.target.checked)} /> {t("includeEdits")}</label><button onClick={exportSession} disabled={selected.status === "active" || selected.status === "trashing" || selected.status === "trashed"}>{t("exportSession")}</button>
          {(selected.status === "trashing" || selected.status === "trashed") ? <button onClick={() => run(async () => { await api.restoreSession(selected.id); setSelected(null); bumpLibrary(); toast(t("sessionRestored")); })}>{t("restoreSession")}</button> : <button className="danger" disabled={selected.status === "active"} onClick={() => setTrashSession(true)}>{t("trashSession")}</button>}</div>
      </section>
      <h3>{t("sessionMedia")}</h3><p className="muted small">{t("originalsOrder")}</p>
      <div className="gallery">{items.map((m) => <button key={m.id} className={`tile ${m.kind}`} onClick={() => setOpen(m)} aria-label={tf("mediaFrom", { kind: m.kind === "photo" ? t("photo").toLowerCase() : m.kind === "video" ? t("video").toLowerCase() : t("photoStrip").toLowerCase(), date: when(m.capturedAt, settings?.language) })}>
        {m.status === "missing" ? <div className="no-thumb err">{t("fileMissing")}</div> : m.thumbRel ? <Thumbnail src={mediaUrl(m.id, "thumb", m.editedRel || m.thumbRel)} fallback={t("thumbUnavailable")} /> : <div className="no-thumb">{m.kind}</div>}
        <span className="tag">{m.kind === "video" ? `▶ ${clock(m.durationMs)}` : m.kind === "photo" ? tf("photoSlot", { number: m.slot + 1 }) : m.kind}</span>{m.editedRel && <span className="edit-tag">{t("editedTag")}</span>}{m.status === "partial" && <span className="sim-tag warn">{t("partialRecording")}</span>}
      </button>)}</div>
      <section className="card">
        <h3>{t("recentJobs")}</h3>
        {printJobs.length > 0 ? (
          <>
            {printJobs.map((j) => (
              <div className="row" key={j.id}>
                <span>{when(j.createdAt, settings?.language)} · {j.printer} · {j.paperName} · {t(jobStatusKey[j.status])}{j.parentId ? ` · ${tf("reprintNumber", { number: j.attempt - 1 })}` : ""}</span>
                {!jobActive(j.status) && j.status !== "unknown" && <button onClick={() => setConfirmPrint(j)}>{t("reprint")}</button>}
              </div>
            ))}
          </>
        ) : <p className="muted">{jobsError ? t("jobHistoryError") : t("noJobs")}</p>}
        <p className="muted small">{t("jobHistoryLimit")}</p>
      </section>
      {mediaLoading && <p role="status">{t("loadingMedia")}</p>}{!mediaLoading && !items.length && <p className="muted">{t("noMedia")}</p>}<div className="row">{mediaStart > 0 && <button onClick={() => setMediaStart(Math.max(0, mediaStart - 50))} disabled={mediaLoading}>{t("previousMedia")}</button>}{mediaMore && <button onClick={() => setMediaStart(mediaOffset)} disabled={mediaLoading}>{t("nextMedia")}</button>}</div>
    </>}
    {open && <Viewer media={open} onClose={() => { pendingViewer.current = null; setOpen(null); }} onDelete={() => setConfirm(open)} onChanged={(m) => { setOpen(m); bumpLibrary(); }}
      onPrevious={activeIndex > 0 ? () => setOpen(items[activeIndex - 1]) : mediaStart > 0 ? () => { pendingViewer.current = "last"; setMediaStart(Math.max(0, mediaStart - 50)); } : undefined} onNext={activeIndex >= 0 && activeIndex < items.length - 1 ? () => setOpen(items[activeIndex + 1]) : mediaMore ? () => { pendingViewer.current = "first"; setMediaStart(mediaOffset); } : undefined} />}
    {confirm && <Confirm title={t("deleteQuestion")} danger confirmLabel={t("deleteToTrash")} onNo={() => setConfirm(null)} onYes={() => del(confirm)} body={<p>{tf("trashItemHelp", { kind: confirm.kind })}</p>} />}
    {confirmPrint && <Confirm title={t("reprintImageQuestion")} confirmLabel={t("reprintImage")} onNo={() => setConfirmPrint(null)} onYes={() => { const j = confirmPrint; setConfirmPrint(null); void run(async () => { await api.reprint(j.id, true); if (selected) setPrintJobs(await api.listSessionPrintJobs(selected.id)); toast(t("reprintToWindowsQueue")); }); }} body={<p>{tf("reprintPrompt", { count: confirmPrint.copies, copyLabel: t(confirmPrint.copies === 1 ? "copySingular" : "copyPlural"), printer: confirmPrint.printer, paper: confirmPrint.paperName })}</p>} />}
    {trashSession && selected && <Confirm title={t("trashQuestion")} danger confirmLabel={t("trashConfirm")} onNo={() => setTrashSession(false)} onYes={trash} body={<p>{t("trashHelp")}</p>} />}
  </div>;
}

function Thumbnail({ src, fallback }: { src: string; fallback: string }) {
  const [failedSrc, setFailedSrc] = useState<string | null>(null);
  return failedSrc === src ? <span className="no-thumb">{fallback}</span> : <img loading="lazy" decoding="async" src={src} alt="" onError={() => setFailedSrc(src)} />;
}

function Viewer({ media, onClose, onDelete, onChanged, onPrevious, onNext }: { media: Media; onClose: () => void; onDelete: () => void; onChanged: (m: Media) => void; onPrevious?: () => void; onNext?: () => void }) {
  const { mediaUrl, toast, settings } = useApp();
  const { t, tf } = useT();
  const [editing, setEditing] = useState(false);
  const [printing, setPrinting] = useState(false);
  const [reframe, setReframe] = useState(false);
  const [showOriginal, setShowOriginal] = useState(false);
  const [imageFailed, setImageFailed] = useState(false);
  const [zoom, setZoom] = useState(1);
  const { error, run } = useAction();
  useEffect(() => { setShowOriginal(false); setImageFailed(false); setZoom(1); }, [media.id]);
  useEffect(() => { if (editing || printing || reframe) return; const key = (e: KeyboardEvent) => { if (e.target instanceof HTMLElement && (e.target.matches("input, textarea, select, video") || e.target.isContentEditable)) return; if (e.key === "ArrowLeft" && onPrevious) { e.preventDefault(); onPrevious(); } if (e.key === "ArrowRight" && onNext) { e.preventDefault(); onNext(); } }; window.addEventListener("keydown", key); return () => window.removeEventListener("keydown", key); }, [editing, printing, reframe, onPrevious, onNext]);
  const isVideo = media.kind === "video";
  const ver = media.editedRel || media.id;

  return (
    <Modal title={isVideo ? t("video") : media.kind === "strip" ? t("photoStrip") : t("photo")} onClose={onClose} wide>
      <div className="viewer">
        <ErrorNote error={error} />
        {media.status === "missing" ? <div className="note err">{t("viewerMissingFile")}</div>
          : isVideo ? <video controls playsInline autoPlay src={mediaUrl(media.id, "original")} className="player" />
          : imageFailed ? <div className="note err">{t("viewerPreviewFailed")}</div>
          : <div className="viewer-image-viewport"><img style={{ zoom }} src={mediaUrl(media.id, showOriginal ? "original" : "preview", ver + (showOriginal ? "o" : ""))} alt={t("selectedPhoto")} onError={() => setImageFailed(true)} /></div>}
        <dl className="meta">
          <dt>{t("capturedAt")}</dt><dd>{when(media.capturedAt, settings?.language)}</dd>
          <dt>{t("size")}</dt><dd>{media.width}×{media.height}{isVideo ? ` · ${clock(media.durationMs)}` : ""} · {bytes(media.sizeBytes, settings?.language)}</dd>
          {media.frameName && <><dt>{t("frame")}</dt><dd>{media.frameName}{media.variantId ? ` · ${media.variantId}` : ""}</dd></>}
          <dt>{t("sourceLabel")}</dt><dd>{media.source || "—"} {media.simulated && <Badge kind="sim">{t("simulatedNotPhoto")}</Badge>} {media.status === "partial" && <Badge kind="warn">{t("partialRecording")}</Badge>}</dd>
        </dl>
      </div>
      <footer>
        <button disabled={!onPrevious} onClick={onPrevious}>{t("previousLabel")}</button><button disabled={!onNext} onClick={onNext}>{t("nextLabel")}</button>
        {!isVideo && <><button onClick={() => setZoom((z) => Math.max(1, z - 0.5))} disabled={zoom <= 1}>{t("zoomOut")}</button><span>{Math.round(zoom * 100)}%</span><button onClick={() => setZoom((z) => Math.min(3, z + 0.5))} disabled={zoom >= 3}>{t("zoomIn")}</button></>}
        <button className="danger" onClick={onDelete}>{t("deleteEllipsis")}</button>
        <button onClick={() => run(() => api.revealMedia(media.id))}>{t("showInFolder")}</button>
        <button onClick={() => run(async () => { const p = await api.exportMedia(media.id, "", false); toast(tf("exportOriginalToast", { path: p })); })}>{t("exportOriginal")}</button>
        {media.editedRel && <button onClick={() => run(async () => { const p = await api.exportMedia(media.id, "", true); toast(tf("exportEditedToast", { path: p })); })}>{t("exportEdited")}</button>}
        {media.editedRel && <button onClick={() => { setImageFailed(false); setShowOriginal((v) => !v); }}>{showOriginal ? t("showEdited") : t("showOriginal")}</button>}
        {media.kind === "strip" && <button onClick={() => setReframe(true)}>{t("changeFrameEllipsis")}</button>}
        {!isVideo && <button onClick={() => setPrinting(true)}>{t("printEllipsis")}</button>}
        {media.kind === "photo" && <button className="primary" onClick={() => setEditing(true)}>{t("edit")}</button>}
      </footer>
      {editing && <Editor media={media} onClose={() => setEditing(false)} onSaved={onChanged} />}
      {printing && <PrintDialog media={media} onClose={() => setPrinting(false)} />}
      {reframe && <ReframeDialog media={media} onClose={() => setReframe(false)} onDone={(m) => { setReframe(false); onChanged(m); }} />}
    </Modal>
  );
}

function PrintDialog({ media, onClose }: { media: Media; onClose: () => void }) {
  const { t } = useT();
  return (
    <Modal title={t("print")} onClose={onClose} wide>
      <div className="modal-body">
        {media.kind === "photo" && <p className="muted small">{t("singlePhotoLayoutHint")}</p>}
        <PrintPanel mediaId={media.id} onPrinted={onClose} />
      </div>
    </Modal>
  );
}

/** Give a finished session another frame, using its original photos (no retake). */
function ReframeDialog({ media, onClose, onDone }: { media: Media; onClose: () => void; onDone: (m: Media) => void }) {
  const { toast } = useApp();
  const { t, tf } = useT();
  const [photos, setPhotos] = useState<number | null>(null);
  const { busy, error, run } = useAction();
  useEffect(() => {
    api.listMedia(media.sessionId, 200, 0).then((ms) => setPhotos(ms.filter((m) => m.kind === "photo").length)).catch(() => setPhotos(0));
  }, [media.sessionId]);
  const apply = (c: FrameChoice, d: { useFirst: boolean; restart: boolean }) => {
    if (d.restart) { toast(tf("reframeNewSessionHint", { frame: c.name, count: c.photos })); onClose(); return; }
    run(async () => { const m = await api.composeSession(media.sessionId, c.id, c.variantId, c.caption, d.useFirst); toast(tf("newImageMadeWith", { frame: c.name })); onDone(m); });
  };
  return (
    <Modal title={t("reframeTitle")} onClose={onClose} wide>
      <div className="modal-body">
        <ErrorNote error={error} />
        <p className="muted small">{t("originalPhotosUntouched")}</p>
        {photos === null ? <div className="spinner" /> : photos === 0
          ? <div className="note err">{t("noPhotosCompose")}</div>
          : <FrameSelection variant="studio" takenPhotos={photos} initial={{ id: media.frameId, variantId: media.variantId }} confirmLabel={t("makeImage")} busy={busy} title={t("chooseFrameForReframe")} onConfirm={apply} />}
      </div>
    </Modal>
  );
}
