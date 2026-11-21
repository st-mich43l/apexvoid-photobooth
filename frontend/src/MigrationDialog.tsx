import { useEffect, useState } from "react";
import { api, on } from "./api";
import { bytes } from "./format";
import { useApp } from "./state";
import { ErrorNote, Modal, useAction } from "./ui";
import type { MigrationReport } from "./types";

/** Offers to copy the old %LOCALAPPDATA% library to Documents\apexvoid-studio. Nothing happens without a click. */
export function MigrationDialog({ onClose }: { onClose: () => void }) {
  const { location, toast } = useApp();
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null);
  const [report, setReport] = useState<MigrationReport | null>(null);
  const { busy, error, run } = useAction();

  useEffect(() => on<{ done: number; total: number }>("migration:progress", setProgress), []);

  const pct = progress && progress.total > 0 ? Math.round((progress.done / progress.total) * 100) : 0;
  return (
    <Modal title={report ? "Library copied" : "Move your media library to Documents?"} onClose={busy ? () => {} : onClose}>
      <div className="modal-body">
        {!report && (<>
          <p>Your photos and videos are currently stored in:</p>
          <p><code>{location.legacyPath}</code></p>
          <p>New photos and videos are stored in <code>{location.defaultPath}</code>. The app can <strong>copy</strong> your existing library there.</p>
          <ul className="small muted">
            <li>The old folder is <strong>not changed or deleted</strong>; it stays as a backup.</li>
            <li>The copy is checked file by file and the database is verified before anything switches over.</li>
            <li>An interrupted copy can be resumed; files are never overwritten.</li>
            <li>You must restart the app afterwards. Until then, capturing is paused.</li>
            <li>Free space now: {bytes(location.freeBytes)}.</li>
          </ul>
          <ErrorNote error={error} />
          {busy && <div role="status"><div className="progress"><i style={{ width: `${pct}%` }} /></div><p className="muted small">Copying… {pct}% {progress ? `(${bytes(progress.done)} of ${bytes(progress.total)})` : ""}</p></div>}
        </>)}
        {report && (<>
          <div className="note warn"><div><strong>Restart ApexVoid Photobooth now</strong> to start using the new library.</div></div>
          <p>{report.files + report.skipped} files ({bytes(report.bytes)}), {report.sessions} sessions and {report.mediaRows} media records were copied and verified.</p>
          <p className="muted small">The old library was left untouched at <code>{report.source}</code>. Delete it yourself only after you have checked the new one.</p>
          {report.symlinks.length > 0 && <p className="muted small">{report.symlinks.length} shortcut file(s) were skipped.</p>}
        </>)}
      </div>
      <footer>
        {!report && <>
          <button disabled={busy} onClick={onClose}>Not now</button>
          <button disabled={busy} onClick={() => run(async () => { await api.keepLegacyLibrary(); toast("Keeping the library where it is"); onClose(); })}>Keep the old folder</button>
          <button className="primary" disabled={busy} onClick={() => run(async () => { setReport(await api.migrateLibrary()); })}>{busy ? "Copying…" : "Copy library to Documents"}</button>
        </>}
        {report && <button className="primary" onClick={onClose}>OK</button>}
      </footer>
    </Modal>
  );
}
