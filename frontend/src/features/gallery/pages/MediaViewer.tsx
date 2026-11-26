import { useEffect, useState } from "react";
import { api } from "../../../shared/api";
import { MediaImage } from "../../../shared/MediaImage";
import { bytes, clock, when } from "../../../shared/format";
import { useApp, useT } from "../../../app/state";
import { Badge, ErrorNote, Modal, useAction } from "../../../shared/ui";
import type { FrameChoice, Media } from "../../../shared/types";
import { Editor } from "./Editor";
import { PrintPanel } from "../../printer/PrintPanel";
import { FrameSelection } from "../../frames/FrameSelection";

export function Viewer({ media, onClose, onDelete, onChanged, onPrevious, onNext }: { media: Media; onClose: () => void; onDelete: () => void; onChanged: (m: Media) => void; onPrevious?: () => void; onNext?: () => void }) {
  const { mediaUrl, toast, settings } = useApp();
  const { t, tf } = useT();
  const [editing, setEditing] = useState(false);
  const [printing, setPrinting] = useState(false);
  const [reframe, setReframe] = useState(false);
  const [showOriginal, setShowOriginal] = useState(false);
  const [zoom, setZoom] = useState(1);
  const { error, run } = useAction();
  useEffect(() => { setShowOriginal(false); setZoom(1); }, [media.id]);
  useEffect(() => { if (editing || printing || reframe) return; const key = (e: KeyboardEvent) => { if (e.target instanceof HTMLElement && (e.target.matches("input, textarea, select, video") || e.target.isContentEditable)) return; if (e.key === "ArrowLeft" && onPrevious) { e.preventDefault(); onPrevious(); } if (e.key === "ArrowRight" && onNext) { e.preventDefault(); onNext(); } }; window.addEventListener("keydown", key); return () => window.removeEventListener("keydown", key); }, [editing, printing, reframe, onPrevious, onNext]);
  const isVideo = media.kind === "video";
  const ver = media.editedRel || media.id;

  return (
    <Modal title={isVideo ? t("video") : media.kind === "strip" ? t("photoStrip") : t("photo")} onClose={onClose} wide>
      <div className="viewer">
        <ErrorNote error={error} />
        {media.status === "missing" ? <div className="note err">{t("viewerMissingFile")}</div>
          : isVideo ? <video controls playsInline autoPlay src={mediaUrl(media.id, "original")} className="player" />
          : <div className="viewer-image-viewport" style={{ zoom }}><MediaImage diagnosticId={media.id} revision={ver + (showOriginal ? "o" : "")} src={mediaUrl(media.id, showOriginal ? "original" : "preview", ver + (showOriginal ? "o" : ""))} alt={t("selectedPhoto")} /></div>}
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
        {media.editedRel && <button onClick={() => setShowOriginal((v) => !v)}>{showOriginal ? t("showEdited") : t("showOriginal")}</button>}
        {media.kind === "strip" && <button onClick={() => setReframe(true)}>{t("changeFrameEllipsis")}</button>}
        {!isVideo && <button onClick={() => setPrinting(true)}>{t("printEllipsis")}</button>}
        {media.kind === "photo" && <button className="primary" onClick={() => setEditing(true)}>{t("edit")}</button>}
      </footer>
      {editing && <Editor media={media} onClose={() => setEditing(false)} onSaved={onChanged} />}
      {printing && <PrintDialog media={media} onClose={() => setPrinting(false)} />}
      {reframe && <ReframeDialog sessionId={media.sessionId} frameId={media.frameId} variantId={media.variantId} onClose={() => setReframe(false)} onDone={(m) => { setReframe(false); onChanged(m); }} />}
    </Modal>
  );
}

/** Print confirmation and duplicate-print protection live in PrintPanel; every entry point goes through it. */
export function PrintDialog({ media, onClose }: { media: Media; onClose: () => void }) {
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
export function ReframeDialog({ sessionId, frameId, variantId, onClose, onDone }: { sessionId: string; frameId: string; variantId: string; onClose: () => void; onDone: (m: Media) => void }) {
  const { toast } = useApp();
  const { t, tf } = useT();
  const [photos, setPhotos] = useState<number | null>(null);
  const { busy, error, run } = useAction();
  useEffect(() => {
    api.listMedia(sessionId, 200, 0).then((ms) => setPhotos(ms.filter((m) => m.kind === "photo").length)).catch(() => setPhotos(0));
  }, [sessionId]);
  const apply = (c: FrameChoice, d: { useFirst: boolean; restart: boolean }) => {
    if (d.restart) { toast(tf("reframeNewSessionHint", { frame: c.name, count: c.photos })); onClose(); return; }
    run(async () => { const m = await api.composeSession(sessionId, c.id, c.variantId, c.caption, d.useFirst); toast(tf("newImageMadeWith", { frame: c.name })); onDone(m); });
  };
  return (
    <Modal title={t("reframeTitle")} onClose={onClose} wide>
      <div className="modal-body">
        <ErrorNote error={error} />
        <p className="muted small">{t("originalPhotosUntouched")}</p>
        {photos === null ? <div className="spinner" /> : photos === 0
          ? <div className="note err">{t("noPhotosCompose")}</div>
          : <FrameSelection variant="studio" takenPhotos={photos} initial={{ id: frameId, variantId }} confirmLabel={t("makeImage")} busy={busy} title={t("chooseFrameForReframe")} onConfirm={apply} />}
      </div>
    </Modal>
  );
}
