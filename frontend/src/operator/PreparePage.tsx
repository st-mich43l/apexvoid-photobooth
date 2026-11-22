import { useCallback, useEffect, useState } from "react";
import { api } from "../api";
import { bytes } from "../format";
import { Icon } from "../icons";
import { backendLabel, kindLabel } from "../sourceInfo";
import { useApp } from "../state";
import type { FramesResult, LibrarySummary, PrintersResult } from "../types";
import { PageHeader, Fact, PrepCard, StateBlock, StatusPill } from "./parts";
import { QuickSettings } from "./QuickSettings";
import { groupView, itemsOf, VIEW_LABEL, viewOf, type View } from "./status";
import { TestPrint } from "./TestPrint";
import { useLatest } from "./latest";

const CAP_TEXT = { supported: "Works — a photo was taken in this session", unverified: "Not verified yet — the first photo will confirm it", unsupported: "Not supported by this camera/driver" } as const;

/** Prepare Booth: the daily start-of-day check. Camera, printer, frames and storage on one screen, built on the single
 *  BoothReadiness() answer (no second readiness engine); every fix is one tap away on the page that owns the setting. */
export function PreparePage({ go, onOpenBooth, opening }: { go: (page: string) => void; onOpenBooth: () => void; opening?: boolean }) {
  const { camera, settings, readiness, refreshReadiness, location, frameUrl, boot } = useApp();
  const [printers, setPrinters] = useState<PrintersResult | null>(null);
  const [frames, setFrames] = useState<FramesResult | null>(null);
  const [lib, setLib] = useState<LibrarySummary>(boot.library);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const latest = useLatest();

  /** Re-reads the hardware lists and readiness. Concurrent refreshes are safe: only the newest answer is applied. */
  const refresh = useCallback(async () => {
    const id = latest.next();
    setBusy(true); setErr("");
    const [p, f, l] = await Promise.allSettled([api.listPrinters(), api.listFrames(false), api.librarySummary(), api.listCameraSources()]);
    if (!latest.isCurrent(id)) return;
    if (p.status === "fulfilled") setPrinters(p.value); else setErr("The printer list could not be read.");
    if (f.status === "fulfilled") setFrames(f.value);
    if (l.status === "fulfilled") setLib(l.value);
    refreshReadiness?.();
    setBusy(false);
  }, [latest, refreshReadiness]);
  useEffect(() => { void refresh(); }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const cam = itemsOf(readiness, "camera"), sto = itemsOf(readiness, "storage"), frm = itemsOf(readiness, "frames"), prn = itemsOf(readiness, "printer");
  const camView = groupView(cam);
  const digital = (readiness?.printing ?? (settings.printerName ? "unverified" : "digital_only")) === "digital_only";
  const prnView: View = digital ? "info" : groupView(prn);
  const printer = printers?.printers.find((p) => p.name === settings.printerName);
  const def = frames?.frames.find((f) => f.id === (settings.templateId || frames.default)) ?? frames?.frames[0];
  const status = readiness?.status ?? "setup_required";
  const caps = camera.capabilities;
  const demo = !!camera.device?.simulated;
  const uvc = camera.device?.driver === "uvc";

  return (
    <div className="page prepare">
      <PageHeader title="Prepare Booth" sub="Check everything once, then open Booth Mode for guests.">
        <button onClick={() => { void refresh(); }} disabled={busy}><Icon name="refresh" size={16} /> {busy ? "Checking…" : "Refresh hardware status"}</button>
      </PageHeader>

      <section className={`prep-hero status-${status}`} aria-label="Booth readiness">
        <div>
          <h2 aria-live="polite">{readiness?.headline ?? "Checking…"}</h2>
          <p>{readiness?.detail || (readiness?.canStart ? "Guests can take photos." : "")}</p>
          {demo && <p className="demo-note" role="note"><StatusPill view="demo" /> Not a hardware check — photos are test patterns.</p>}
        </div>
        <button className="primary alt cta" onClick={onOpenBooth} disabled={opening}><Icon name="play" size={18} />{opening ? "Checking…" : "Open Booth Mode"}</button>
      </section>
      {err && <StateBlock kind="error" action={<button onClick={() => { void refresh(); }}>Try again</button>}>{err}</StateBlock>}
      {!readiness && <StateBlock kind="loading">Reading the booth status…</StateBlock>}

      <div className="prep-grid">
        <PrepCard id="camera" icon="camera" title="Camera" view={camView}
          actions={<><button onClick={() => go("camera")}>{camera.device ? "Camera setup" : "Choose a camera"}</button><button onClick={() => go("camera")} disabled={!camera.device || camera.reconnecting}>Test camera…</button></>}>
          <dl className="facts">
            <Fact label="Photo camera">{camera.device ? <>{camera.device.name}{demo && <> <StatusPill view="demo" label="SIMULATED" /></>}</> : <span className="muted">none connected</span>}</Fact>
            <Fact label="Connection">{camera.reconnecting ? "Reconnecting automatically…" : camera.device ? (camera.state === "error" ? "Error" : "Connected") : "Not connected"}</Fact>
            <Fact label="Type · driver">{camera.device ? `${kindLabel(camera.device.kind)} · ${backendLabel(camera.device.driver)}` : "—"}</Fact>
            <Fact label="Preferred camera">{settings.photoCamera.name || <span className="muted">none chosen — the connected camera is used</span>}</Fact>
            <Fact label="Preview">{camera.liveView ? "Live view available" : camera.device ? "Off (starts with the first guest session)" : "—"}</Fact>
            <Fact label="Still capture">{caps ? CAP_TEXT[caps.stillCapture] : "—"}</Fact>
            <Fact label="Reconnection">{camera.reconnecting ? "In progress" : settings.autoReconnect ? "Automatic after a USB drop" : "Off"}</Fact>
          </dl>
          {uvc && <p className="muted small">Photos from a webcam are frame grabs from the video stream.</p>}
          <Lines items={cam} />
        </PrepCard>

        <PrepCard id="printer" icon="printer" title="Printer" view={prnView} pillLabel={digital ? "Digital only" : undefined}
          actions={<><button onClick={() => go("printer")}>{digital ? "Set up printing" : "Printer setup"}</button>
            {!digital && <TestPrint printer={settings.printerName} paper={settings.printPaper} disabled={printer?.status !== "ready"} />}</>}>
          <dl className="facts">
            <Fact label="Printing">{digital ? "Off — guests get digital photos only (a normal choice)" : "On"}</Fact>
            {!digital && <>
              <Fact label="Printer">{settings.printerName}</Fact>
              <Fact label="Windows reports">{printer ? printer.status : printers ? <span className="warn-text">not found</span> : "…"}</Fact>
              <Fact label="Paper size">{settings.printPaper || <span className="muted">not set — choose one on the Printer page</span>}</Fact>
            </>}
          </dl>
          <p className="muted small">{digital ? "Nothing is required here. Printing can be switched on at any time." : "“Ready” is what Windows says about the queue. Paper output is not verified — use a test print to check it."}</p>
          {!digital && readiness?.printing === "unavailable" && <p className="note warn inline-note">Printing is unavailable right now. Guests can still take photos; they are saved digitally.</p>}
          <Lines items={prn} />
        </PrepCard>

        <PrepCard id="frames" icon="frames" title="Frames" view={groupView(frm)}
          actions={<button onClick={() => go("frames")}>Manage frames</button>}>
          <div className="frames-row">
            {def && <img className="def-frame" src={frameUrl(def.id, "", 240)} alt={`Default frame: ${def.name}`} loading="lazy" />}
            <dl className="facts">
              <Fact label="Offered to guests">{frames ? `${frames.frames.length} ${frames.frames.length === 1 ? "frame" : "frames"}` : "…"}</Fact>
              <Fact label="Preselected">{def ? <>{def.name} <span className="muted small">{def.format} · {def.photos} {def.photos === 1 ? "photo" : "photos"}</span></> : <span className="muted">none</span>}</Fact>
            </dl>
          </div>
          <Lines items={frm} />
        </PrepCard>

        <PrepCard id="storage" icon="storage" title="Storage" view={groupView(sto)}
          actions={<><button onClick={() => go("settings")}>Library settings</button><button onClick={() => { void api.openLibraryFolder(); }}><Icon name="folder" size={16} /> Open media folder</button></>}>
          <dl className="facts">
            <Fact label="Media folder"><code className="path">{location.path}</code></Fact>
            <Fact label="Free space">{location.freeBytes >= 0 ? bytes(location.freeBytes) : "unknown"}</Fact>
            <Fact label="Writable">{sto.find((i) => i.key === "storage")?.state === "problem" ? <span className="warn-text">No</span> : "Yes"}</Fact>
            <Fact label="Interrupted sessions">{lib.interrupted.length > 0 ? <span className="warn-text">{lib.interrupted.length} — originals recovered, check the Gallery</span> : "None"}</Fact>
          </dl>
          <Lines items={sto} />
        </PrepCard>
      </div>

      <QuickSettings />
      <nav className="shortcuts" aria-label="Shortcuts">
        <button onClick={() => go("camera")}>Camera setup</button>
        <button onClick={() => go("printer")}>Printer setup</button>
        <button onClick={() => go("frames")}>Manage guest frames</button>
        <button onClick={() => { void api.openLibraryFolder(); }}>Open media folder</button>
        <button onClick={() => go("diagnostics")}>Diagnostics</button>
      </nav>
    </div>
  );
}

/** The readiness lines that need a word: problems, warnings, demo and unverified, each with what to do. */
function Lines({ items }: { items: ReturnType<typeof itemsOf> }) {
  const shown = items.filter((i) => viewOf(i) !== "ready" && viewOf(i) !== "info");
  if (shown.length === 0) return null;
  return (
    <ul className="lines">
      {shown.map((i) => {
        const v = viewOf(i);
        return <li key={i.key} className={`v-${v}`}><StatusPill view={v} /><span><strong>{i.label}.</strong> {i.detail} {i.action && <em>{i.action}</em>}</span><span className="sr-only">{VIEW_LABEL[v]}</span></li>;
      })}
    </ul>
  );
}
