import { useEffect, useMemo, useState } from "react";
import { api } from "../api";
import { bytes, clock, when } from "../format";
import { useApp } from "../state";
import { Badge, Confirm, ErrorNote, Modal, useAction } from "../ui";
import type { Media, PrintersResult, Session, TemplatesResult } from "../types";
import { Editor } from "./Editor";

type Filter = "all" | "photo" | "video" | "strip";

export function Gallery() {
  const { mediaUrl, libraryVersion, bumpLibrary, toast } = useApp();
  const [items, setItems] = useState<Media[]>([]);
  const [sessions, setSessions] = useState<Record<string, Session>>({});
  const [filter, setFilter] = useState<Filter>("all");
  const [open, setOpen] = useState<Media | null>(null);
  const [confirm, setConfirm] = useState<Media | null>(null);
  const [loaded, setLoaded] = useState(false);
  const { error, run, clear } = useAction();

  useEffect(() => {
    run(async () => {
      const [m, s] = await Promise.all([api.listMedia("", 1000, 0), api.listSessions(500, 0)]);
      setItems(m); setSessions(Object.fromEntries(s.map((x) => [x.id, x]))); setLoaded(true);
    });
  }, [libraryVersion]); // eslint-disable-line react-hooks/exhaustive-deps

  const shown = items.filter((m) => filter === "all" || m.kind === filter);
  const groups = useMemo(() => {
    const g = new Map<string, Media[]>();
    for (const m of shown) g.set(m.sessionId, [...(g.get(m.sessionId) ?? []), m]);
    return [...g.entries()];
  }, [shown]);

  const del = (m: Media) => run(async () => {
    await api.trashMedia(m.id);
    setConfirm(null); setOpen(null); bumpLibrary();
    toast("Moved to trash", "ok", () => { api.restoreMedia(m.id).then(bumpLibrary).catch(() => {}); });
  });

  return (
    <div className="page">
      <header className="row"><h1>Gallery</h1>
        <div className="seg">
          {(["all", "photo", "strip", "video"] as Filter[]).map((f) => <button key={f} className={filter === f ? "on" : ""} onClick={() => setFilter(f)}>{f === "all" ? "All" : f === "photo" ? "Photos" : f === "strip" ? "Strips" : "Videos"}</button>)}
        </div>
      </header>
      <ErrorNote error={error} onClose={clear} />
      {loaded && shown.length === 0 && <p className="muted">No media yet. Start a photo session or record a video.</p>}
      {groups.map(([sid, ms]) => {
        const s = sessions[sid];
        return (
          <section key={sid} className="session-group">
            <h3>{s ? when(s.createdAt) : sid} {s?.simulated && <Badge kind="sim">SIMULATED</Badge>} {s && s.status !== "completed" && <Badge kind="warn">{s.status}</Badge>}</h3>
            <div className="gallery">
              {ms.map((m) => (
                <button key={m.id} className={`tile ${m.kind}`} onClick={() => setOpen(m)} aria-label={`${m.kind} from ${when(m.capturedAt)}`}>
                  {m.status === "missing" ? <div className="no-thumb err">file missing</div>
                    : m.thumbRel ? <img loading="lazy" src={mediaUrl(m.id, "thumb", m.editedRel || m.thumbRel)} alt="" /> : <div className="no-thumb">{m.kind}</div>}
                  <span className="tag">{m.kind === "video" ? `▶ ${clock(m.durationMs)}` : m.kind}</span>
                  {m.simulated && <span className="sim-tag">SIM</span>}
                  {m.status === "partial" && <span className="sim-tag warn">partial</span>}
                  {m.editedRel && <span className="edit-tag">edited</span>}
                </button>
              ))}
            </div>
          </section>
        );
      })}
      {open && <Viewer media={open} onClose={() => setOpen(null)} onDelete={() => setConfirm(open)}
        onChanged={(m) => { setOpen(m); bumpLibrary(); }} />}
      {confirm && (
        <Confirm title="Move to trash?" danger confirmLabel="Move to trash" onNo={() => setConfirm(null)} onYes={() => del(confirm)}
          body={<p>This {confirm.kind} will be removed from the gallery and moved to the trash folder. You can undo right after, and the files are only erased permanently when the trash is emptied in Settings.</p>} />
      )}
    </div>
  );
}

function Viewer({ media, onClose, onDelete, onChanged }: { media: Media; onClose: () => void; onDelete: () => void; onChanged: (m: Media) => void }) {
  const { mediaUrl, toast } = useApp();
  const [editing, setEditing] = useState(false);
  const [printing, setPrinting] = useState(false);
  const [showOriginal, setShowOriginal] = useState(false);
  const { error, run } = useAction();
  const isVideo = media.kind === "video";
  const ver = media.editedRel || media.id;

  return (
    <Modal title={isVideo ? "Video" : media.kind === "strip" ? "Photo strip" : "Photo"} onClose={onClose} wide>
      <div className="viewer">
        <ErrorNote error={error} />
        {media.status === "missing" ? <div className="note err">The file is missing from the media folder.</div>
          : isVideo ? <video controls playsInline autoPlay src={mediaUrl(media.id, "original")} className="player" />
          : <img src={mediaUrl(media.id, showOriginal ? "original" : "preview", ver + (showOriginal ? "o" : ""))} alt="" />}
        <dl className="meta">
          <dt>Captured</dt><dd>{when(media.capturedAt)}</dd>
          <dt>Size</dt><dd>{media.width}×{media.height}{isVideo ? ` · ${clock(media.durationMs)}` : ""} · {bytes(media.sizeBytes)}</dd>
          <dt>Source</dt><dd>{media.source || "—"} {media.simulated && <Badge kind="sim">SIMULATED — not a real photo</Badge>} {media.status === "partial" && <Badge kind="warn">partial recording</Badge>}</dd>
        </dl>
      </div>
      <footer>
        <button className="danger" onClick={onDelete}>Delete…</button>
        <button onClick={() => run(() => api.revealMedia(media.id))}>Show in folder</button>
        <button onClick={() => run(async () => { const p = await api.exportMedia(media.id, "", false); toast(`Original exported to ${p}`); })}>Export original</button>
        {media.editedRel && <button onClick={() => run(async () => { const p = await api.exportMedia(media.id, "", true); toast(`Edited copy exported to ${p}`); })}>Export edited</button>}
        {media.editedRel && <button onClick={() => setShowOriginal((v) => !v)}>{showOriginal ? "Show edited" : "Show original"}</button>}
        {media.kind === "strip" && <button onClick={() => setPrinting(true)}>Print…</button>}
        {media.kind === "photo" && <button className="primary" onClick={() => setEditing(true)}>Edit</button>}
      </footer>
      {editing && <Editor media={media} onClose={() => setEditing(false)} onSaved={onChanged} />}
      {printing && <PrintDialog media={media} onClose={() => setPrinting(false)} />}
    </Modal>
  );
}

function PrintDialog({ media, onClose }: { media: Media; onClose: () => void }) {
  const { settings, toast } = useApp();
  const [pr, setPr] = useState<PrintersResult | null>(null);
  const [tpls, setTpls] = useState<TemplatesResult | null>(null);
  const [printer, setPrinter] = useState(settings.printerName);
  const [paper, setPaper] = useState("4x6");
  const [copies, setCopies] = useState(1);
  const [tpl, setTpl] = useState(settings.templateId);
  const { busy, error, run } = useAction();
  useEffect(() => { api.listPrinters().then(setPr).catch(() => {}); api.listTemplates().then((t) => { setTpls(t); }).catch(() => {}); }, []);
  return (
    <Modal title="Print" onClose={onClose}>
      <div className="modal-body">
        <div className="note warn"><div>Printing is not validated on any physical printer yet. {pr?.note}</div></div>
        <ErrorNote error={error} />
        <div className="form">
          <label>Printer<select value={printer} onChange={(e) => setPrinter(e.target.value)}><option value="">Choose…</option>{pr?.printers.map((p) => <option key={p.name}>{p.name}</option>)}</select></label>
          <label>Layout of this image<select value={tpl} onChange={(e) => setTpl(e.target.value)}>{tpls?.templates.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}</select></label>
          <label>Paper<select value={paper} onChange={(e) => setPaper(e.target.value)}>{pr?.papers.map((p) => <option key={p.name}>{p.name}</option>)}</select></label>
          <label>Copies<input type="number" min={1} max={10} value={copies} onChange={(e) => setCopies(Number(e.target.value))} /></label>
        </div>
      </div>
      <footer><button onClick={onClose}>Close</button>
        <button className="primary" disabled={busy || !printer} onClick={() => run(async () => { const j = await api.printMedia(media.id, printer, paper, copies, tpl); toast(`Print job ${j.id} submitted`); onClose(); })}>Print</button></footer>
    </Modal>
  );
}
