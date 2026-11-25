import { useCallback, useEffect, useState } from "react";
import { api } from "../../../shared/api";
import { bytes } from "../../../shared/format";
import { printerStatusLabel } from "../../../shared/printInfo";
import { Icon } from "../../../shared/icons";
import { frameDisplayName } from "../../../shared/frameInfo";
import { backendLabel, kindLabel } from "../../../shared/sourceInfo";
import { useApp, useT } from "../../../app/state";
import type { FramesResult, LibrarySummary, PrintersResult } from "../../../shared/types";
import { PageHeader, Fact, PrepCard, StateBlock, StatusPill } from "./parts";
import { QuickSettings } from "./QuickSettings";
import { groupView, itemsOf, readinessHeadline, readinessTranslation, VIEW_LABEL, viewOf, type View } from "./status";
import { TestPrint } from "../../printer/operator/TestPrint";
import { useLatest } from "./latest";

/** Prepare Booth: the daily start-of-day check. Camera, printer, frames and storage on one screen, built on the single
 *  BoothReadiness() answer (no second readiness engine); every fix is one tap away on the page that owns the setting. */
export function PreparePage({ go, onOpenBooth, opening }: { go: (page: string) => void; onOpenBooth: () => void; opening?: boolean }) {
  const { camera, settings, readiness, refreshReadiness, location, frameUrl, boot } = useApp();
  const { t, tf } = useT();
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
    if (p.status === "fulfilled") setPrinters(p.value); else setErr(t("preparePrinterReadError"));
    if (f.status === "fulfilled") setFrames(f.value);
    if (l.status === "fulfilled") setLib(l.value);
    refreshReadiness?.();
    setBusy(false);
  }, [latest, refreshReadiness, t]);
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
      <PageHeader title={t("prepareBoothTitle")} sub={t("prepareBoothHint")}>
        <button onClick={() => { void refresh(); }} disabled={busy}><Icon name="refresh" size={16} /> {busy ? t("checkingStatus") : t("refreshHardware")}</button>
      </PageHeader>

      <section className={`prep-hero status-${status}`} aria-label={t("boothReadiness")}>
        <div>
          <h2 aria-live="polite">{readinessHeadline(readiness, t)}</h2>
          <p>{readiness?.detail || (readiness?.canStart ? t("guestsCanTakePhotos") : "")}</p>
          {demo && <p className="demo-note" role="note"><StatusPill view="demo" /> {t("notHardwareCheck")}</p>}
        </div>
        <button className="primary alt cta" onClick={onOpenBooth} disabled={opening}><Icon name="play" size={18} />{opening ? t("checkingStatus") : t("openBooth")}</button>
      </section>
      {err && <StateBlock kind="error" action={<button onClick={() => { void refresh(); }}>{t("tryAgain")}</button>}>{err}</StateBlock>}
      {!readiness && <StateBlock kind="loading">{t("boothStatusRead")}</StateBlock>}

      <div className="prep-grid">
        <PrepCard id="camera" icon="camera" title={t("camera")} view={camView}
          actions={<><button onClick={() => go("camera")}>{camera.device ? t("cameraSetup") : t("chooseCamera")}</button><button onClick={() => go("camera")} disabled={!camera.device || camera.reconnecting}>{t("testCamera")}</button></>}>
          <dl className="facts">
            <Fact label={t("photoCamera")}>{camera.device ? <>{camera.device.name}{demo && <> <StatusPill view="demo" label={t("simulated").toUpperCase()} /></>}</> : <span className="muted">{t("noneConnected")}</span>}</Fact>
            <Fact label={t("connection")}>{camera.reconnecting ? t("cameraReconnectingStatus") : camera.device ? (camera.state === "error" ? t("problem") : t("connected")) : t("notConnected")}</Fact>
            <Fact label={t("typeDriver")}>{camera.device ? `${kindLabel(camera.device.kind, settings.language)} · ${backendLabel(camera.device.driver, settings.language)}` : "—"}</Fact>
            <Fact label={t("preferredCamera")}>{settings.photoCamera.name || <span className="muted">{t("noneChosenConnectedUsed")}</span>}</Fact>
            <Fact label={t("preview")}>{camera.liveView ? t("liveViewAvailable") : camera.device ? t("previewOffFirstGuest") : "—"}</Fact>
            <Fact label={t("stillCapture")}>{caps ? t(caps.stillCapture === "supported" ? "captureSupported" : caps.stillCapture === "unverified" ? "captureUnverified" : "captureUnsupported") : "—"}</Fact>
            <Fact label={t("reconnection")}>{camera.reconnecting ? t("inProgress") : settings.autoReconnect ? t("automaticAfterUSB") : t("off")}</Fact>
          </dl>
          {uvc && <p className="muted small">{t("webcamFrameGrab")}</p>}
          <Lines items={cam} />
        </PrepCard>

        <PrepCard id="printer" icon="printer" title={t("printer")} view={prnView} pillLabel={digital ? t("digitalOnly") : undefined}
          actions={<><button onClick={() => go("printer")}>{digital ? t("printWithPrinter") : t("printerSetup")}</button>
            {!digital && <TestPrint printer={settings.printerName} paper={settings.printPaper} disabled={printer?.status !== "ready"} />}</>}>
          <dl className="facts">
            <Fact label={t("printing")}>{digital ? t("digitalOnlyNormalChoice") : t("printingOn")}</Fact>
            {!digital && <>
              <Fact label={t("printer")}>{settings.printerName}</Fact>
              <Fact label={t("windowsReports")}>{printer ? printerStatusLabel(printer.status, settings.language) : printers ? <span className="warn-text">{t("notFoundLower")}</span> : "…"}</Fact>
              <Fact label={t("paperSize")}>{settings.printPaper || <span className="muted">{t("notSetChoosePrinter")}</span>}</Fact>
            </>}
          </dl>
          <p className="muted small">{digital ? t("printingOptional") : t("spoolerNotVerified")}</p>
          {!digital && readiness?.printing === "unavailable" && <p className="note warn inline-note">{t("printingUnavailableDigital")}</p>}
          <Lines items={prn} />
        </PrepCard>

        <PrepCard id="frames" icon="frames" title={t("frames")} view={groupView(frm)}
          actions={<button onClick={() => go("frames")}>{t("manageFrames")}</button>}>
          <div className="frames-row">
            {def && <img className="def-frame" src={frameUrl(def.id, "", 240)} alt={`${t("defaultFrame")}: ${frameDisplayName(def, settings.language)}`} loading="lazy" />}
            <dl className="facts">
              <Fact label={t("offeredToGuests")}>{frames ? tf("framesOfferedCount", { count: frames.frames.length, label: t(frames.frames.length === 1 ? "frame" : "framesCount") }) : "…"}</Fact>
              <Fact label={t("preselected")}>{def ? <>{frameDisplayName(def, settings.language)} <span className="muted small">{def.format} · {tf("photoCountCompact", { count: def.photos, label: t(def.photos === 1 ? "photoWord" : "photosWord") })}</span></> : <span className="muted">{t("noSelection")}</span>}</Fact>
            </dl>
          </div>
          <Lines items={frm} />
        </PrepCard>

        <PrepCard id="storage" icon="storage" title={t("storage")} view={groupView(sto)}
          actions={<><button onClick={() => go("settings")}>{t("librarySettings")}</button><button onClick={() => { void api.openLibraryFolder(); }}><Icon name="folder" size={16} /> {t("openMediaFolder")}</button></>}>
          <dl className="facts">
            <Fact label={t("mediaFolder")}><code className="path">{location.path}</code></Fact>
            <Fact label={t("freeSpace")}>{location.freeBytes >= 0 ? bytes(location.freeBytes, settings.language) : t("unknown")}</Fact>
            <Fact label={t("writable")}>{sto.find((i) => i.key === "storage")?.state === "problem" ? <span className="warn-text">{t("no")}</span> : t("yes")}</Fact>
            <Fact label={t("interruptedSessions")}>{lib.interrupted.length > 0 ? <span className="warn-text">{tf("interruptedRecovered", { count: lib.interrupted.length })}</span> : t("none")}</Fact>
          </dl>
          <Lines items={sto} />
        </PrepCard>
      </div>

      <QuickSettings />
      <nav className="shortcuts" aria-label={t("shortcuts")}>
        <button onClick={() => go("camera")}>{t("cameraSetup")}</button>
        <button onClick={() => go("printer")}>{t("printerSetup")}</button>
        <button onClick={() => go("frames")}>{t("manageGuestFrames")}</button>
        <button onClick={() => { void api.openLibraryFolder(); }}>{t("openMediaFolder")}</button>
        <button onClick={() => go("diagnostics")}>{t("diagnostics")}</button>
      </nav>
    </div>
  );
}

/** The readiness lines that need a word: problems, warnings, demo and unverified, each with what to do. */
function Lines({ items }: { items: ReturnType<typeof itemsOf> }) {
  const { settings } = useApp();
  const { t } = useT();
  const shown = items.filter((i) => viewOf(i) !== "ready" && viewOf(i) !== "info");
  if (shown.length === 0) return null;
  return (
    <ul className="lines">
      {shown.map((i) => {
        const v = viewOf(i);
        return <li key={i.key} className={`v-${v}`}><StatusPill view={v} /><span>{settings.language === "vi" ? <><strong>{readinessTranslation(i, t).label}.</strong> {readinessTranslation(i, t).detail} <em>{readinessTranslation(i, t).action}</em><details><summary>{t("technicalDetails")}</summary>{i.detail} {i.action}</details></> : <><strong>{i.label}.</strong> {i.detail} {i.action && <em>{i.action}</em>}</>}</span><span className="sr-only">{VIEW_LABEL[v]}</span></li>;
      })}
    </ul>
  );
}
