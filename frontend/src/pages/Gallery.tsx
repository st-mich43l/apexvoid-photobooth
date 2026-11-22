import { useEffect, useMemo, useState } from "react";
import { api } from "../api";
import { bytes, clock, when } from "../format";
import { useApp } from "../state";
import { Badge, Confirm, ErrorNote, Modal, useAction } from "../ui";
import type { Media, Session } from "../types";
import { Editor } from "./Editor";
import { PrintPanel } from "../PrintPanel";
import { FrameSelection } from "../FrameSelection";
import type { FrameChoice } from "../types";

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

  const shown = items.filter((m) => filter === "all" || m.kind === filter || (filter === "strip" && m.kind === "print"));
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
          {(["all", "photo", "strip", "video"] as Filter[]).map((f) => <button key={f} className={filter === f ? "on" : ""} onClick={() => setFilter(f)}>{f === "all" ? "All" : f === "photo" ? "Photos" : f === "strip" ? "Strips & prints" : "Videos"}</button>)}
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
  const [reframe, setReframe] = useState(false);
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
          {media.frameName && <><dt>Frame</dt><dd>{media.frameName}{media.variantId ? ` · ${media.variantId}` : ""}</dd></>}
          <dt>Source</dt><dd>{media.source || "—"} {media.simulated && <Badge kind="sim">SIMULATED — not a real photo</Badge>} {media.status === "partial" && <Badge kind="warn">partial recording</Badge>}</dd>
        </dl>
      </div>
      <footer>
        <button className="danger" onClick={onDelete}>Delete…</button>
        <button onClick={() => run(() => api.revealMedia(media.id))}>Show in folder</button>
        <button onClick={() => run(async () => { const p = await api.exportMedia(media.id, "", false); toast(`Original exported to ${p}`); })}>Export original</button>
        {media.editedRel && <button onClick={() => run(async () => { const p = await api.exportMedia(media.id, "", true); toast(`Edited copy exported to ${p}`); })}>Export edited</button>}
        {media.editedRel && <button onClick={() => setShowOriginal((v) => !v)}>{showOriginal ? "Show edited" : "Show original"}</button>}
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
