import { useEffect, useState } from "react";
import { api } from "../api";
import { bytes } from "../format";
import { useApp } from "../state";
import { Icon } from "../icons";
import type { Media, PrintersResult } from "../types";

type Tone = "ok" | "warn" | "err";

function StatCard({ icon, label, tone, status, value, sub, action, onAction, children }:
  { icon: string; label: string; tone: Tone; status: string; value: string; sub?: string; action: string; onAction: () => void; children?: React.ReactNode }) {
  return (
    <section className={`stat-card ${tone}`} aria-label={label}>
      <div className="stat-top">
        <span className="stat-icon"><Icon name={icon} size={22} /></span>
        <span className="stat-label">{label}</span>
        <span className="stat-status"><i className="dot" />{status}</span>
      </div>
      <div className="stat-value">{value}</div>
      {sub && <p className="stat-sub">{sub}</p>}
      {children}
      <button className="stat-link" onClick={onAction}>{action}<span aria-hidden="true">→</span></button>
    </section>
  );
}

export function Dashboard({ go, onBooth }: { go: (p: string) => void; onBooth?: () => void }) {
  const { camera, boot, settings, mediaUrl, libraryVersion } = useApp();
  const [recent, setRecent] = useState<Media[]>([]);
  const [lib, setLib] = useState(boot.library);
  const [printers, setPrinters] = useState<PrintersResult | null>(null);

  useEffect(() => {
    api.listMedia("", 8, 0).then(setRecent).catch(() => {});
    api.librarySummary().then(setLib).catch(() => {});
  }, [libraryVersion]);
  useEffect(() => { api.listPrinters().then(setPrinters).catch(() => setPrinters(null)); }, []);

  const connected = !!camera.device && camera.state !== "error" && !camera.reconnecting;
  const printer = printers?.printers.find((p) => p.name === settings.printerName);
  const lowDisk = lib.freeBytes >= 0 && lib.freeBytes < 2 * 1024 ** 3;
  const interrupted = lib.interrupted.length;

  const camTone: Tone = connected ? "ok" : camera.reconnecting ? "warn" : "err";
  const camStatus = connected ? (camera.device!.simulated ? "Simulated" : "Connected") : camera.reconnecting ? "Reconnecting" : "Not connected";
  const prTone: Tone = printer ? (printer.status === "ready" ? "ok" : "warn") : "warn";

  return (
    <div className="page dash">
      <header className="dash-hero">
        <div className="hero-copy">
          <span className="eyebrow">Studio</span>
          <h1>{connected ? "Ready to shoot" : "Almost ready"}</h1>
          <p>{connected ? "Guests pick a frame, the camera counts down, and the strip is ready to print."
            : "Connect a camera first. Booth Mode needs one before guests can shoot."}</p>
          <div className="hero-actions">
            {onBooth && <button className="primary alt cta" onClick={onBooth}><Icon name="play" size={18} />Open Booth Mode</button>}
            <button onClick={() => go("photo")}><Icon name="photo" size={18} />Photo session</button>
            <button onClick={() => go("video")}><Icon name="video" size={18} />Record video</button>
          </div>
        </div>
        <dl className="hero-stats" aria-label="Library totals">
          <div><dd>{lib.photos}</dd><dt>Photos</dt></div>
          <div><dd>{lib.videos}</dd><dt>Videos</dt></div>
          <div><dd>{lib.sessions}</dd><dt>Sessions</dt></div>
        </dl>
      </header>

      <div className="stat-grid">
        <StatCard icon="camera" label="Camera" tone={camTone} status={camStatus}
          value={connected ? camera.device!.name : camera.reconnecting ? "Reconnecting…" : "No camera"}
          sub={connected ? camera.device!.driver : camera.lastError ? `${camera.lastError}${camera.lastAction ? ` — ${camera.lastAction}` : ""}` : "Plug in a webcam or camera and open Camera settings."}
          action="Camera settings" onAction={() => go("camera")} />
        <StatCard icon="printer" label="Printer" tone={prTone}
          status={printer ? (printer.status === "ready" ? "Ready" : printer.status) : "Not set"}
          value={printer ? printer.name : settings.printerName ? `${settings.printerName} not found` : "No printer selected"}
          sub="Jobs go through the Windows spooler."
          action={printer ? "Printer settings" : "Choose a printer"} onAction={() => go("printer")} />
        <StatCard icon="storage" label="Storage" tone={lowDisk ? "err" : interrupted ? "warn" : "ok"}
          status={lowDisk ? "Low space" : interrupted ? "Check sessions" : "Healthy"}
          value={lib.freeBytes >= 0 ? `${bytes(lib.freeBytes)} free` : "Unknown"}
          sub={interrupted ? `${interrupted} interrupted session(s) — originals recovered` : `${lib.photos} photos · ${lib.videos} videos saved`}
          action="Open gallery" onAction={() => go("gallery")} />
      </div>

      <div className="section-head"><h2>Recent</h2>{recent.length > 0 && <button className="ghost" onClick={() => go("gallery")}>View all →</button>}</div>
      {recent.length === 0 ? (
        <div className="empty-state"><Icon name="photo" size={28} /><b>No photos yet</b><span>Sessions you shoot will show up here.</span></div>
      ) : (
        <div className="strip-row">
          {recent.map((m) => (
            <button key={m.id} className="thumb-btn" onClick={() => go("gallery")} aria-label={`${m.kind} ${m.id}`}>
              {m.thumbRel ? <img src={mediaUrl(m.id, "thumb", m.editedRel)} alt="" /> : <div className="no-thumb">{m.kind}</div>}
              {m.simulated && <span className="sim-tag">SIM</span>}
            </button>
          ))}
        </div>
      )}
      <p className="muted small path-line">Media folder: {boot.dataDir}</p>
    </div>
  );
}
