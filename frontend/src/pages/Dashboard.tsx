import { useEffect, useState } from "react";
import { api } from "../api";
import { bytes } from "../format";
import { useApp, useT } from "../state";
import { Icon } from "../icons";
import type { Media, PrintersResult, ReadinessItem } from "../types";

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
  const { camera, boot, settings, mediaUrl, libraryVersion, readiness, refreshReadiness } = useApp();
  const { t, tf } = useT();
  const [recent, setRecent] = useState<Media[]>([]);
  const [lib, setLib] = useState(boot.library);
  const [printers, setPrinters] = useState<PrintersResult | null>(null);

  useEffect(() => {
    api.listMedia("", 8, 0).then(setRecent).catch(() => {});
    api.librarySummary().then(setLib).catch(() => {});
  }, [libraryVersion]);
  // the printer list is asked again now and then (and on demand): a printer switched off after the page opened must show
  const loadPrinters = () => api.listPrinters().then(setPrinters).catch(() => setPrinters(null));
  useEffect(() => { loadPrinters(); const t = window.setInterval(loadPrinters, 10_000); return () => window.clearInterval(t); }, []);
  useEffect(() => { refreshReadiness?.(); }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const connected = !!camera.device && camera.state !== "error" && !camera.reconnecting;
  const printer = printers?.printers.find((p) => p.name === settings.printerName);
  const lowDisk = lib.freeBytes >= 0 && lib.freeBytes < 2 * 1024 ** 3;
  const interrupted = lib.interrupted.length;

  const camTone: Tone = connected ? (camera.device!.simulated ? "warn" : "ok") : camera.reconnecting ? "warn" : "err";
  const camStatus = connected ? (camera.device!.simulated ? t("demoSimulated") : t("connected")) : camera.reconnecting ? t("cameraReconnectingStatus") : t("notConnected");
  const prTone: Tone = printer ? (printer.status === "ready" ? "ok" : "warn") : settings.printerName ? "warn" : "ok";

  // where each readiness line is fixed
  const FIX: Record<string, [string, string]> = {
    camera: ["camera", t("cameraSetup")], capture: ["camera", t("cameraSetup")], storage: ["settings", t("librarySettings")], library: ["settings", t("librarySettings")],
    space: ["settings", t("librarySettings")], frames: ["frames", t("frames")], printer: ["printer", t("printerSetup")], session: ["photo", t("openSession")], interrupted: ["gallery", t("openGallery")],
  };
  const todo: ReadinessItem[] = (readiness?.items ?? []).filter((it) => it.state === "problem" || it.state === "warn");
  const status = readiness?.status ?? (connected ? "ready" : "setup_required");
  const headline = readiness?.headline ?? (connected ? t("readyForGuests") : t("setupRequired"));
  const sub = readiness?.detail || (connected ? t("guestReadyHint") : t("connectCameraHint"));

  return (
    <div className="page dash">
      <header className={`dash-hero status-${status}`}>
        <div className="hero-copy">
          <span className="eyebrow">{t("studioOperator")}</span>
          <h1 aria-live="polite">{headline}</h1>
          <p>{sub}</p>
          <div className="hero-actions">
            {onBooth && <button className="primary alt cta" onClick={onBooth}><Icon name="play" size={18} />{t("openBooth")}</button>}
            <button onClick={() => go("prepare")}><Icon name="prepare" size={18} />{t("prepare")}</button>
            <button onClick={() => go("photo")}><Icon name="photo" size={18} />{t("photoSession")}</button>
            <button onClick={() => go("video")}><Icon name="video" size={18} />{t("recordVideo")}</button>
          </div>
        </div>
        <dl className="hero-stats" aria-label={t("libraryTotals")}>
          <div><dd>{lib.photos}</dd><dt>{t("photoCountLabel")}</dt></div>
          <div><dd>{lib.videos}</dd><dt>{t("video")}</dt></div>
          <div><dd>{lib.sessions}</dd><dt>{t("sessions")}</dt></div>
        </dl>
      </header>

      {todo.length > 0 && (
        <section className="todo" aria-label={t("needsAttention")}>
          <h2>{t("needsAttention")}</h2>
          <ul>
            {todo.map((it) => (
              <li key={it.key} className={it.state}>
                <span className="todo-dot" aria-hidden="true" />
                <div><strong>{it.label}</strong><span>{it.detail}</span>{it.action && <em>{it.action}</em>}</div>
                {FIX[it.key] && <button onClick={() => go(FIX[it.key][0])}>{FIX[it.key][1]}</button>}
              </li>
            ))}
          </ul>
        </section>
      )}

      <div className="stat-grid">
        <StatCard icon="camera" label={t("camera")} tone={camTone} status={camStatus}
          value={connected ? camera.device!.name : camera.reconnecting ? t("cameraReconnectingStatus") : t("noCamera")}
          sub={connected ? camera.device!.driver : camera.lastError ? `${camera.lastError}${camera.lastAction ? ` — ${camera.lastAction}` : ""}` : t("plugCameraHint")}
          action={t("cameraSetup")} onAction={() => go("camera")} />
        <StatCard icon="printer" label={t("printer")} tone={prTone}
          status={printer ? (printer.status === "ready" ? t("windowsReady") : printer.status) : settings.printerName ? t("notFound") : t("digitalOnly")}
          value={printer ? printer.name : settings.printerName ? `${settings.printerName} ${t("notFound").toLowerCase()}` : t("noPrinterSelected")}
          sub={printer || settings.printerName ? t("spoolerNotice") : t("choosePrinterHint")}
          action={t("printerSetup")} onAction={() => go("printer")} />
        <StatCard icon="storage" label={t("storage")} tone={lowDisk ? "err" : interrupted ? "warn" : "ok"}
          status={lowDisk ? t("lowSpace") : interrupted ? t("checkSessions") : t("healthy")}
          value={lib.freeBytes >= 0 ? `${bytes(lib.freeBytes)} ${t("freeBytes")}` : t("unknownPrintHelp")}
          sub={interrupted ? tf("interruptedSessions", { count: interrupted }) : tf("savedMediaCount", { photos: lib.photos, videos: lib.videos })}
          action={t("openGallery")} onAction={() => go("gallery")} />
      </div>

      <div className="section-head"><h2>{t("recent")}</h2>{recent.length > 0 && <button className="ghost" onClick={() => go("gallery")}>{t("viewAll")}</button>}</div>
      {recent.length === 0 ? (
        <div className="empty-state"><Icon name="photo" size={28} /><b>{t("noPhotosYet")}</b><span>{t("sessionsAppearHere")}</span></div>
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
      <p className="muted small path-line">{t("mediaFolder")} {boot.dataDir}</p>
    </div>
  );
}
