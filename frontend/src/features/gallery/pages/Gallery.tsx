import { useEffect, useRef, useState } from "react";
import { api } from "../../../shared/api";
import { MediaImage } from "../../../shared/MediaImage";
import { when } from "../../../shared/format";
import { useApp, useT } from "../../../app/state";
import { Badge, Confirm, ErrorNote, useAction } from "../../../shared/ui";
import type { Media, PrintJob, SessionCard, SessionQuery } from "../../../shared/types";
import type { TranslationKey } from "../../../shared/i18n";
import { Viewer } from "./MediaViewer";
import { SessionDetail } from "./SessionDetail";

const initialQuery: SessionQuery = { search: "", mode: "", status: "", simulated: "all", from: "", to: "", cursor: "", limit: 40 };
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
  const [printJobs, setPrintJobs] = useState<PrintJob[]>([]);
  const [jobsState, setJobsState] = useState<"loading" | "error" | "ready">("loading");
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
    setMediaLoading(true); setPrintJobs([]); setJobsState("loading");
    api.listSessionPrintJobs(selected.id).then((jobs) => { if (id === mediaRequest.current) { setPrintJobs(jobs); setJobsState("ready"); } }).catch(() => { if (id === mediaRequest.current) setJobsState("error"); });
    api.querySessionMedia(selected.id, 50, mediaStart).then((p) => { if (id === mediaRequest.current) { setItems(p.items); setMediaOffset(p.nextOffset); setMediaMore(p.hasMore); if (pendingViewer.current) { setOpen(p.items[pendingViewer.current === "first" ? 0 : p.items.length - 1] ?? null); pendingViewer.current = null; } } })
      .catch((e) => { if (id === mediaRequest.current) void run(async () => { throw e; }); }).finally(() => { if (id === mediaRequest.current) setMediaLoading(false); });
    return () => { mediaRequest.current++; };
  }, [selected?.id, mediaStart, libraryVersion]); // eslint-disable-line react-hooks/exhaustive-deps
  // keep the open session's card (status, frame, composition flag) in step with the refreshed list
  useEffect(() => { if (selected) { const fresh = sessions.find((s) => s.id === selected.id); if (fresh && fresh !== selected) setSelected(fresh); } }, [sessions]); // eslint-disable-line react-hooks/exhaustive-deps
  const update = (patch: Partial<SessionQuery>) => { setPageCursor(""); setHistory([]); setQuery((q) => ({ ...q, ...patch, cursor: "" })); };
  const del = (m: Media) => run(async () => { await api.trashMedia(m.id); setConfirm(null); setOpen(null); bumpLibrary(); toast(t("mediaMovedToTrash"), "ok", () => { void run(async () => { await api.restoreMedia(m.id); bumpLibrary(); }); }); });
  const exportSession = (includeEdited: boolean) => selected && run(async () => { const folder = await api.pickFolder(); if (!folder) return; const p = await api.exportSession(selected.id, folder, includeEdited); toast(tf("sessionExported", { path: p })); });
  const restoreSession = () => selected && run(async () => { await api.restoreSession(selected.id); setSelected(null); bumpLibrary(); toast(t("sessionRestored")); });
  const trash = () => selected && run(async () => { const id = selected.id; try { await api.trashSession(id); } catch (e) { setTrashSession(false); setSelected(null); bumpLibrary(); throw e; } setTrashSession(false); setSelected(null); bumpLibrary(); toast(t("sessionMovedToTrash"), "ok", () => { void run(async () => { await api.restoreSession(id); bumpLibrary(); }); }); });
  const activeIndex = open ? items.findIndex((m) => m.id === open.id) : -1;
  const sessionStatusLabel = (status: string) => sessionStatusKeys[status] ? t(sessionStatusKeys[status]) : status;

  return <div className="page">
    {!selected && <header className="row"><h1>{t("gallery")}</h1></header>}
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
    </> : <SessionDetail session={selected} items={items} mediaLoading={mediaLoading} mediaStart={mediaStart} mediaMore={mediaMore}
      onPreviousMedia={() => setMediaStart(Math.max(0, mediaStart - 50))} onNextMedia={() => setMediaStart(mediaOffset)}
      jobs={printJobs} jobsState={jobsState} onBack={() => { setSelected(null); setOpen(null); }} onOpen={setOpen}
      onExport={(edited) => void exportSession(edited)} onTrash={() => setTrashSession(true)} onRestore={() => void restoreSession()}
      onReprint={setConfirmPrint} onComposed={() => bumpLibrary()} />}
    {open && <Viewer media={open} onClose={() => { pendingViewer.current = null; setOpen(null); }} onDelete={() => setConfirm(open)} onChanged={(m) => { setOpen(m); bumpLibrary(); }}
      onPrevious={activeIndex > 0 ? () => setOpen(items[activeIndex - 1]) : mediaStart > 0 ? () => { pendingViewer.current = "last"; setMediaStart(Math.max(0, mediaStart - 50)); } : undefined} onNext={activeIndex >= 0 && activeIndex < items.length - 1 ? () => setOpen(items[activeIndex + 1]) : mediaMore ? () => { pendingViewer.current = "first"; setMediaStart(mediaOffset); } : undefined} />}
    {confirm && <Confirm title={t("deleteQuestion")} danger confirmLabel={t("deleteToTrash")} onNo={() => setConfirm(null)} onYes={() => del(confirm)} body={<p>{tf("trashItemHelp", { kind: confirm.kind })}</p>} />}
    {confirmPrint && <Confirm title={t("reprintImageQuestion")} confirmLabel={t("reprintImage")} onNo={() => setConfirmPrint(null)} onYes={() => { const j = confirmPrint; setConfirmPrint(null); void run(async () => { await api.reprint(j.id, true); if (selected) { setPrintJobs(await api.listSessionPrintJobs(selected.id)); setJobsState("ready"); } toast(t("reprintToWindowsQueue")); }); }} body={<p>{tf("reprintPrompt", { count: confirmPrint.copies, copyLabel: t(confirmPrint.copies === 1 ? "copySingular" : "copyPlural"), printer: confirmPrint.printer, paper: confirmPrint.paperName })}</p>} />}
    {trashSession && selected && <Confirm title={t("trashQuestion")} danger confirmLabel={t("trashConfirm")} onNo={() => setTrashSession(false)} onYes={trash} body={<p>{t("trashHelp")}</p>} />}
  </div>;
}

function Thumbnail({ src, fallback }: { src: string; fallback: string }) {
  return <MediaImage src={src} alt={fallback} manualRetry={false} />;
}
