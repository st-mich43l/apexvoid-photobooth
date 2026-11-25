import { useEffect, useState } from "react";
import { api, on } from "./api";
import { bytes } from "./format";
import { useApp, useT } from "./state";
import { ErrorNote, Modal, useAction } from "./ui";
import type { MigrationReport } from "./types";

/** Offers to copy the old %LOCALAPPDATA% library to Documents\apexvoid-studio. Nothing happens without a click. */
export function MigrationDialog({ onClose }: { onClose: () => void }) {
  const { location, toast, settings } = useApp();
  const { t, tf } = useT();
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null);
  const [report, setReport] = useState<MigrationReport | null>(null);
  const { busy, error, run } = useAction();

  useEffect(() => on<{ done: number; total: number }>("migration:progress", setProgress), []);

  const pct = progress && progress.total > 0 ? Math.round((progress.done / progress.total) * 100) : 0;
  return (
    <Modal title={report ? t("copiedLibrary") : t("moveLibraryDocuments")} onClose={busy ? () => {} : onClose}>
      <div className="modal-body">
        {!report && (<>
          <p>{t("currentMediaAt")}</p>
          <p><code>{location.legacyPath}</code></p>
          <p>{t("newMediaAt")} <code>{location.defaultPath}</code>. {t("copyExistingLibrary")}</p>
          <ul className="small muted">
            <li>{t("oldFolderUntouched")}</li>
            <li>{t("copyVerified")}</li>
            <li>{t("copyResume")}</li>
            <li>{t("restartRequired")}</li>
            <li>{tf("freeSpaceNow", { space: bytes(location.freeBytes, settings?.language) })}</li>
          </ul>
          <ErrorNote error={error} />
          {busy && <div role="status"><div className="progress"><i style={{ width: `${pct}%` }} /></div><p className="muted small">{tf("copyingPercent", { percent: pct })} {progress ? `(${bytes(progress.done, settings?.language)} / ${bytes(progress.total, settings?.language)})` : ""}</p></div>}
        </>)}
        {report && (<>
          <div className="note warn"><div><strong>{t("restartNow")}</strong> {t("toStartNewLibrary")}</div></div>
          <p>{tf("copiedSummary", { files: report.files + report.skipped, bytes: bytes(report.bytes, settings?.language), sessions: report.sessions, media: report.mediaRows })}</p>
          <p className="muted small">{t("oldLibraryAt")} <code>{report.source}</code>. {t("deleteAfterCheck")}</p>
          {report.symlinks.length > 0 && <p className="muted small">{tf("skippedShortcuts", { count: report.symlinks.length })}</p>}
        </>)}
      </div>
      <footer>
        {!report && <>
          <button disabled={busy} onClick={onClose}>{t("notNow")}</button>
          <button disabled={busy} onClick={() => run(async () => { await api.keepLegacyLibrary(); toast(t("keepingLibrary")); onClose(); })}>{t("keepOldFolder")}</button>
          <button className="primary" disabled={busy} onClick={() => run(async () => { setReport(await api.migrateLibrary()); })}>{busy ? t("copying") : t("copyLibraryDocuments")}</button>
        </>}
        {report && <button className="primary" onClick={onClose}>{t("ok")}</button>}
      </footer>
    </Modal>
  );
}
