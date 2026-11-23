import { translate } from "../i18n";
import { useEffect, useRef, useState } from "react";
import { api } from "../api";
import { bytes, clock, when } from "../format";
import { useApp } from "../state";
import { Badge, Confirm, ErrorNote, Modal, useAction } from "../ui";
import type { Media, PrintJob, SessionCard, SessionQuery } from "../types";
import { Editor } from "./Editor";
import { PrintPanel } from "../PrintPanel";
import { FrameSelection } from "../FrameSelection";
import type { FrameChoice } from "../types";
import { jobActive, jobLabel } from "../printInfo";

const initialQuery: SessionQuery = { search: "", mode: "", status: "", simulated: "all", from: "", to: "", cursor: "", limit: 40 };

export function Gallery() {
  const { mediaUrl, libraryVersion, bumpLibrary, toast, settings } = useApp();
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
  const del = (m: Media) => run(async () => { await api.trashMedia(m.id); setConfirm(null); setOpen(null); bumpLibrary(); toast("Moved to trash", "ok", () => { void run(async () => { await api.restoreMedia(m.id); bumpLibrary(); }); }); });
  const exportSession = () => selected && run(async () => { const folder = await api.pickFolder(); if (!folder) return; const p = await api.exportSession(selected.id, folder, includeEdited); toast(`Session exported to ${p}`); });
  const trash = () => selected && run(async () => { const id = selected.id; try { await api.trashSession(id); } catch (e) { setTrashSession(false); setSelected(null); bumpLibrary(); throw e; } setTrashSession(false); setSelected(null); bumpLibrary(); toast("Session moved to trash", "ok", () => { void run(async () => { await api.restoreSession(id); bumpLibrary(); }); }); });
  const activeIndex = open ? items.findIndex((m) => m.id === open.id) : -1;

  return <div className="page">
    <header className="row"><h1>{translate(settings?.language, "gallery")}</h1>{selected && <button onClick={() => { setSelected(null); setOpen(null); }}>← All sessions</button>}</header>
    <ErrorNote error={error} onClose={clear} />
    {!selected ? <>
      <div className="gallery-filters" aria-label="Session filters">
        <label>{translate(settings?.language, "searchSession")} <input value={query.search} onChange={(e) => update({ search: e.target.value })} placeholder="Session ID" /></label>
        <label>Mode <select value={query.mode} onChange={(e) => update({ mode: e.target.value })}><option value="">All modes</option><option value="photo">Photo</option><option value="video">Video</option></select></label>
        <label>Status <select value={query.status} onChange={(e) => update({ status: e.target.value })}><option value="">All statuses</option>{["active", "completed", "cancelled", "failed", "interrupted", "trashing", "trashed"].map((x) => <option key={x} value={x}>{x}</option>)}</select></label>
        <label>Sessions <select value={query.simulated} onChange={(e) => update({ simulated: e.target.value })}><option value="all">All</option><option value="real">Real</option><option value="simulated">Simulated</option><option value="test">Operator tests</option></select></label>
        <label>From (UTC) <input type="date" value={query.from} onChange={(e) => update({ from: e.target.value })} /></label><label>To (UTC) <input type="date" value={query.to} onChange={(e) => update({ to: e.target.value })} /></label>
        <button onClick={() => { setPageCursor(""); setHistory([]); setQuery({ ...initialQuery }); }}>Clear filters</button>
      </div>
      {!loading && sessions.length === 0 && <p className="muted empty-state">No sessions match these filters.</p>}
      <div className="session-cards">{sessions.map((s) => <button className="session-card" key={s.id} onClick={() => { pendingViewer.current = null; setMediaStart(0); setSelected(s); }}>
        <div className="session-card-image">{s.thumbnailId ? <Thumbnail src={mediaUrl(s.thumbnailId, "thumb", s.thumbnailVersion)} fallback="No thumbnail" /> : <span className="muted">No thumbnail</span>}</div>
        <strong>{when(s.createdAt)}</strong><span>{s.mode} · {s.photoCount} {s.photoCount === 1 ? "photo" : "photos"} {s.hasStrip ? "· composition" : ""}</span><span>{s.frameName || "No frame"} · {s.status}</span><small>{s.id}</small>{s.simulated && <Badge kind="sim">SIMULATED</Badge>}{s.note === "Operator camera test" && <Badge kind="info">TEST</Badge>}
      </button>)}</div>
      {loading && <p role="status">Loading sessions…</p>}<div className="row">{history.length > 0 && <button onClick={previousPage} disabled={loading}>Previous sessions</button>}{more && <button onClick={nextPage} disabled={loading}>Next sessions</button>}</div>
    </> : <>
      <section className="card session-summary"><h2>{when(selected.createdAt)} {selected.simulated && <Badge kind="sim">SIMULATED</Badge>}</h2>
        <p>{selected.id} · {selected.mode} · {selected.status} · {selected.frameName || "No frame"} · {selected.photoCount} {selected.photoCount === 1 ? "photo" : "photos"}</p>
        {selected.note && <p className="note warn">{selected.note}</p>}
        <div className="row"><label className="inline"><input type="checkbox" checked={includeEdited} onChange={(e) => setIncludeEdited(e.target.checked)} /> Include edited copies in export</label><button onClick={exportSession} disabled={selected.status === "active" || selected.status === "trashing" || selected.status === "trashed"}>Export session…</button>
          {(selected.status === "trashing" || selected.status === "trashed") ? <button onClick={() => run(async () => { await api.restoreSession(selected.id); setSelected(null); bumpLibrary(); toast("Session restored"); })}>Restore session</button> : <button className="danger" disabled={selected.status === "active"} onClick={() => setTrashSession(true)}>Move session to trash…</button>}</div>
      </section>
      <h3>{translate(settings?.language, "sessionMedia")}</h3><p className="muted small">Original captures appear in shot order, followed by compositions and videos.</p>
      <div className="gallery">{items.map((m) => <button key={m.id} className={`tile ${m.kind}`} onClick={() => setOpen(m)} aria-label={`${m.kind} from ${when(m.capturedAt)}`}>
        {m.status === "missing" ? <div className="no-thumb err">file missing</div> : m.thumbRel ? <Thumbnail src={mediaUrl(m.id, "thumb", m.editedRel || m.thumbRel)} fallback="Thumbnail unavailable" /> : <div className="no-thumb">{m.kind}</div>}
        <span className="tag">{m.kind === "video" ? `▶ ${clock(m.durationMs)}` : m.kind === "photo" ? `photo ${m.slot + 1}` : m.kind}</span>{m.editedRel && <span className="edit-tag">edited</span>}{m.status === "partial" && <span className="sim-tag warn">partial</span>}
      </button>)}</div>
      <section className="card"><h3>{translate(settings?.language, "recentJobs")}</h3>{printJobs.length ? printJobs.map((j) => <div className="row" key={j.id}><span>{when(j.createdAt)} · {j.printer} · {j.paperName} · {jobLabel[j.status]}{j.parentId ? ` · reprint #${j.attempt - 1}` : ""}</span>{!jobActive(j.status) && j.status !== "unknown" && <button onClick={() => setConfirmPrint(j)}>Reprint…</button>}</div>) : <p className="muted">{jobsError ? "Print history could not be loaded." : "No print jobs for this session."}</p>}<p className="muted small">Showing up to 50 recent jobs. Windows queue status does not prove physical delivery.</p></section>
      {mediaLoading && <p role="status">Loading media…</p>}{!mediaLoading && !items.length && <p className="muted">No visible media in this session.</p>}<div className="row">{mediaStart > 0 && <button onClick={() => setMediaStart(Math.max(0, mediaStart - 50))} disabled={mediaLoading}>Previous media</button>}{mediaMore && <button onClick={() => setMediaStart(mediaOffset)} disabled={mediaLoading}>Next media</button>}</div>
    </>}
    {open && <Viewer media={open} onClose={() => { pendingViewer.current = null; setOpen(null); }} onDelete={() => setConfirm(open)} onChanged={(m) => { setOpen(m); bumpLibrary(); }}
      onPrevious={activeIndex > 0 ? () => setOpen(items[activeIndex - 1]) : mediaStart > 0 ? () => { pendingViewer.current = "last"; setMediaStart(Math.max(0, mediaStart - 50)); } : undefined} onNext={activeIndex >= 0 && activeIndex < items.length - 1 ? () => setOpen(items[activeIndex + 1]) : mediaMore ? () => { pendingViewer.current = "first"; setMediaStart(mediaOffset); } : undefined} />}
    {confirm && <Confirm title="Move to trash?" danger confirmLabel="Move to trash" onNo={() => setConfirm(null)} onYes={() => del(confirm)} body={<p>This {confirm.kind} will be moved to recoverable trash.</p>} />}
    {confirmPrint && <Confirm title="Reprint this image?" confirmLabel="Reprint" onNo={() => setConfirmPrint(null)} onYes={() => { const j = confirmPrint; setConfirmPrint(null); void run(async () => { await api.reprint(j.id, true); if (selected) setPrintJobs(await api.listSessionPrintJobs(selected.id)); toast("Reprint sent to Windows queue"); }); }} body={<p>Send {confirmPrint.copies} copy to {confirmPrint.printer} on {confirmPrint.paperName}? Check the output tray first; Windows cannot confirm physical delivery.</p>} />}
    {trashSession && selected && <Confirm title="Move session to trash?" danger confirmLabel="Move session to trash" onNo={() => setTrashSession(false)} onYes={trash} body={<p>All session media will move to recoverable trash. This cannot interrupt an active session. Use Undo to restore it.</p>} />}
  </div>;
}

function Thumbnail({ src, fallback }: { src: string; fallback: string }) {
  const [failed, setFailed] = useState(false);
  useEffect(() => setFailed(false), [src]);
  return failed ? <span className="no-thumb">{fallback}</span> : <img loading="lazy" decoding="async" src={src} alt="" onError={() => setFailed(true)} />;
}

function Viewer({ media, onClose, onDelete, onChanged, onPrevious, onNext }: { media: Media; onClose: () => void; onDelete: () => void; onChanged: (m: Media) => void; onPrevious?: () => void; onNext?: () => void }) {
  const { mediaUrl, toast } = useApp();
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
    <Modal title={isVideo ? "Video" : media.kind === "strip" ? "Photo strip" : "Photo"} onClose={onClose} wide>
      <div className="viewer">
        <ErrorNote error={error} />
        {media.status === "missing" ? <div className="note err">The file is missing from the media folder.</div>
          : isVideo ? <video controls playsInline autoPlay src={mediaUrl(media.id, "original")} className="player" />
          : imageFailed ? <div className="note err">The image preview could not be loaded.</div>
          : <div className="viewer-image-viewport"><img style={{ zoom }} src={mediaUrl(media.id, showOriginal ? "original" : "preview", ver + (showOriginal ? "o" : ""))} alt="Selected photo" onError={() => setImageFailed(true)} /></div>}
        <dl className="meta">
          <dt>Captured</dt><dd>{when(media.capturedAt)}</dd>
          <dt>Size</dt><dd>{media.width}×{media.height}{isVideo ? ` · ${clock(media.durationMs)}` : ""} · {bytes(media.sizeBytes)}</dd>
          {media.frameName && <><dt>Frame</dt><dd>{media.frameName}{media.variantId ? ` · ${media.variantId}` : ""}</dd></>}
          <dt>Source</dt><dd>{media.source || "—"} {media.simulated && <Badge kind="sim">SIMULATED — not a real photo</Badge>} {media.status === "partial" && <Badge kind="warn">partial recording</Badge>}</dd>
        </dl>
      </div>
      <footer>
        <button disabled={!onPrevious} onClick={onPrevious}>Previous</button><button disabled={!onNext} onClick={onNext}>Next</button>
        {!isVideo && <><button onClick={() => setZoom((z) => Math.max(1, z - 0.5))} disabled={zoom <= 1}>Zoom out</button><span>{Math.round(zoom * 100)}%</span><button onClick={() => setZoom((z) => Math.min(3, z + 0.5))} disabled={zoom >= 3}>Zoom in</button></>}
        <button className="danger" onClick={onDelete}>Delete…</button>
        <button onClick={() => run(() => api.revealMedia(media.id))}>Show in folder</button>
        <button onClick={() => run(async () => { const p = await api.exportMedia(media.id, "", false); toast(`Original exported to ${p}`); })}>Export original</button>
        {media.editedRel && <button onClick={() => run(async () => { const p = await api.exportMedia(media.id, "", true); toast(`Edited copy exported to ${p}`); })}>Export edited</button>}
        {media.editedRel && <button onClick={() => { setImageFailed(false); setShowOriginal((v) => !v); }}>{showOriginal ? "Show edited" : "Show original"}</button>}
        {media.kind === "strip" && <button onClick={() => setReframe(true)}>Change frame…</button>}
        {!isVideo && <button onClick={() => setPrinting(true)}>Print…</button>}
        {media.kind === "photo" && <button className="primary" onClick={() => setEditing(true)}>Edit</button>}
      </footer>
      {editing && <Editor media={media} onClose={() => setEditing(false)} onSaved={onChanged} />}
      {printing && <PrintDialog media={media} onClose={() => setPrinting(false)} />}
      {reframe && <ReframeDialog media={media} onClose={() => setReframe(false)} onDone={(m) => { setReframe(false); onChanged(m); }} />}
    </Modal>
  );
}

function PrintDialog({ media, onClose }: { media: Media; onClose: () => void }) {
  return (
    <Modal title="Print" onClose={onClose} wide>
      <div className="modal-body">
        {media.kind === "photo" && <p className="muted small">A single photo is printed through a one-photo layout (its edited version if you edited it) — never as the raw original.</p>}
        <PrintPanel mediaId={media.id} onPrinted={onClose} />
      </div>
    </Modal>
  );
}

/** Give a finished session another frame, using its original photos (no retake). */
function ReframeDialog({ media, onClose, onDone }: { media: Media; onClose: () => void; onDone: (m: Media) => void }) {
  const { toast } = useApp();
  const [photos, setPhotos] = useState<number | null>(null);
  const { busy, error, run } = useAction();
  useEffect(() => {
    api.listMedia(media.sessionId, 200, 0).then((ms) => setPhotos(ms.filter((m) => m.kind === "photo").length)).catch(() => setPhotos(0));
  }, [media.sessionId]);
  const apply = (c: FrameChoice, d: { useFirst: boolean; restart: boolean }) => {
    if (d.restart) { toast(`“${c.name}” needs ${c.photos} photos: start a new photo session and choose it there.`); onClose(); return; }
    run(async () => { const m = await api.composeSession(media.sessionId, c.id, c.variantId, c.caption, d.useFirst); toast(`New image made with “${c.name}”`); onDone(m); });
  };
  return (
    <Modal title="Change frame" onClose={onClose} wide>
      <div className="modal-body">
        <ErrorNote error={error} />
        <p className="muted small">The original photos are not touched. A new image is made; the earlier one stays in the gallery.</p>
        {photos === null ? <div className="spinner" /> : photos === 0
          ? <div className="note err">This session has no photos to compose.</div>
          : <FrameSelection variant="studio" takenPhotos={photos} initial={{ id: media.frameId, variantId: media.variantId }} confirmLabel="Make image" busy={busy} title="Choose a frame" onConfirm={apply} />}
      </div>
    </Modal>
  );
}
