import { useEffect, useState } from "react";
import { api } from "../api";
import { bytes } from "../format";
import { useApp } from "../state";
import { Badge } from "../ui";
import type { Media, PrintersResult } from "../types";

export function Dashboard({ go }: { go: (p: string) => void }) {
  const { camera, boot, settings, mediaUrl, libraryVersion } = useApp();
  const [recent, setRecent] = useState<Media[]>([]);
  const [lib, setLib] = useState(boot.library);
  const [printers, setPrinters] = useState<PrintersResult | null>(null);

  useEffect(() => {
    api.listMedia("", 8, 0).then(setRecent).catch(() => {});
    api.librarySummary().then(setLib).catch(() => {});
  }, [libraryVersion]);
  useEffect(() => { api.listPrinters().then(setPrinters).catch(() => setPrinters(null)); }, []);

  const connected = camera.device && camera.state !== "error" && !camera.reconnecting;
  const printer = printers?.printers.find((p) => p.name === settings.printerName);

  return (
    <div className="page">
      <h1>Dashboard</h1>
      <div className="grid cards">
        <section className="card">
          <h3>Camera</h3>
          {connected ? (
            <>
              <p className="big-line">{camera.device!.name}</p>
              <p><Badge kind={camera.device!.simulated ? "sim" : "ok"}>{camera.device!.simulated ? "SIMULATED" : camera.state}</Badge>{" "}
                <span className="muted">{camera.device!.driver}</span></p>
            </>
          ) : camera.reconnecting ? <p><Badge kind="warn">disconnected — reconnecting</Badge></p>
            : <p><Badge kind="err">not connected</Badge></p>}
          {camera.lastError && <p className="muted small">{camera.lastError}{camera.lastAction ? ` — ${camera.lastAction}` : ""}</p>}
          <button onClick={() => go("camera")}>Camera settings</button>
        </section>

        <section className="card">
          <h3>Printer</h3>
          {printer ? <p><span className="big-line">{printer.name}</span> <Badge kind={printer.status === "ready" ? "ok" : "warn"}>{printer.status}</Badge></p>
            : <p className="muted">{settings.printerName ? `${settings.printerName} not found` : "No printer selected"}</p>}
          <p className="muted small">Discovery only — print submission is not validated on real printers yet.</p>
          <button onClick={() => go("templates")}>Layouts &amp; printing</button>
        </section>

        <section className="card">
          <h3>Library</h3>
          <p className="big-line">{lib.photos} photos · {lib.videos} videos</p>
          <p className="muted">{lib.sessions} sessions · {bytes(lib.freeBytes)} free</p>
          {lib.interrupted.length > 0 && <p><Badge kind="warn">{lib.interrupted.length} session(s) were interrupted — originals recovered</Badge></p>}
          {lib.freeBytes >= 0 && lib.freeBytes < 2 * 1024 ** 3 && <p><Badge kind="err">Low disk space</Badge></p>}
          <button onClick={() => go("gallery")}>Open gallery</button>
        </section>

        <section className="card actions">
          <h3>Shoot</h3>
          <button className="primary" onClick={() => go("photo")}>Photo session</button>
          <button className="primary alt" onClick={() => go("video")}>Record video</button>
        </section>
      </div>

      <h2>Recent</h2>
      {recent.length === 0 ? <p className="muted">Nothing captured yet.</p> : (
        <div className="strip-row">
          {recent.map((m) => (
            <button key={m.id} className="thumb-btn" onClick={() => go("gallery")} aria-label={`${m.kind} ${m.id}`}>
              {m.thumbRel ? <img src={mediaUrl(m.id, "thumb", m.editedRel)} alt="" /> : <div className="no-thumb">{m.kind}</div>}
              {m.simulated && <span className="sim-tag">SIM</span>}
            </button>
          ))}
        </div>
      )}
      <p className="muted small">Media folder: {boot.dataDir}</p>
    </div>
  );
}
