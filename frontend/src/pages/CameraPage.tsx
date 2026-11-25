import { useEffect, useState } from "react";
import { api } from "../api";
import { backendLabel, canRecordVideo, guidance, kindLabel, modeOptions, resolutions } from "../sourceInfo";
import { CanonPanel } from "../CanonPanel";
import { useApp, useT } from "../state";
import { Badge, ErrorNote, LivePreview, SupportBadge, useAction } from "../ui";
import { CameraTest } from "../operator/CameraTest";
import { useLatest } from "../operator/latest";
import { PageHeader, StateBlock } from "../operator/parts";
import { Icon } from "../icons";
import type { CameraSources, DeviceInfo } from "../types";

const CAP_LABELS = [
  ["liveView", "liveViewLabel"], ["stillCapture", "stillCapture"], ["originalTransfer", "originalTransferLabel"],
  ["movieRecord", "movieRecordLabel"], ["movieTransfer", "movieTransferLabel"], ["streamedVideo", "streamedVideoLabel"],
] as const;

/** Camera setup. Primary view: choose the photo camera, connect, see it live, run an optional test. Everything else
 *  (video camera, resolution, backends, Canon diagnostics, capabilities) is under "Advanced". */
export function CameraPage() {
  const { camera, settings, toast, shoot } = useApp();
  const { t, tf } = useT();
  const [src, setSrc] = useState<CameraSources | null>(null);
  const scan = useAction();
  const act = useAction();
  const latest = useLatest();

  // concurrent refreshes (button + a camera event) are safe: only the newest answer is shown
  const refresh = () => scan.run(async () => { const id = latest.next(); const r = await api.listCameraSources(); if (latest.isCurrent(id)) setSrc(r); });
  useEffect(() => { void refresh(); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { if (src) { const id = latest.next(); api.listCameraSources().then((r) => { if (latest.isCurrent(id)) setSrc(r); }).catch(() => {}); } }, [camera.device?.id, camera.reconnecting, settings.photoCamera.id, settings.videoCamera.id]); // eslint-disable-line react-hooks/exhaustive-deps

  const active = camera.device?.id;
  const caps = camera.capabilities;
  const activeDev = src?.devices.find((d) => d.id === active);
  const err = act.error ?? scan.error;
  const locked = ["COUNTDOWN", "CAPTURING", "REVIEW", "SAVING"].includes(shoot?.state); // the backend refuses too; the UI says why
  const sim = !!camera.device?.simulated;

  const status = (d: DeviceInfo) => {
    if (d.duplicateOf) return <Badge kind="warn">{t("duplicate")}</Badge>;
    if (active === d.id) return camera.reconnecting ? <Badge kind="warn">{t("cameraReconnecting")}</Badge> : d.simulated ? <Badge kind="sim">{t("demoConnected")}</Badge> : <Badge kind="ok">{t("connected")}</Badge>;
    return <Badge kind="info">{t("available")}</Badge>;
  };
  const isPref = (d: DeviceInfo, p: "photo" | "video") => (p === "photo" ? src?.photoMatchId : src?.videoMatchId) === d.id;

  return (
    <div className="page">
      <PageHeader title={t("camera")} sub={t("cameraSub")}>
        <button onClick={() => { void refresh(); }} disabled={scan.busy}><Icon name="refresh" size={16} /> {scan.busy ? t("scanning") : t("refreshDevices")}</button>
      </PageHeader>
      {src?.restoreNote && <div className="note warn" role="alert"><div>{src.restoreNote}</div></div>}
      {camera.reconnecting && <div className="note warn" role="alert"><div>{t("cameraDisconnected")}</div></div>}
      {locked && <div className="note info" role="status"><div>{t("cameraLocked")}</div></div>}
      {sim && <div className="note info" role="status"><div><strong>{t("demoModeTitle")}</strong> {t("cameraDemoNote")}</div></div>}
      <ErrorNote error={err} onClose={() => { act.clear(); scan.clear(); }} />
      {!err && camera.lastError && <ErrorNote error={{ name: "", message: camera.lastError, action: camera.lastAction || guidance(activeDev, camera.lastError, settings.language) }} />}
      {src && !src.ffmpegFound && <div className="note warn"><div>{t("cameraFFmpegMissing")}</div></div>}

      <div className="two-col sources">
        <section className="card" aria-label={t("photoCamera")}>
          <h3>{t("photoCamera")}</h3>
          {scan.busy && !src && <StateBlock kind="loading">{t("lookingCameras")}</StateBlock>}
          {src && src.devices.length === 0 && <StateBlock kind="empty">{t("noCameras")}</StateBlock>}
          <ul className="sources-list">
            {src?.devices.map((d) => (
              <li key={d.id} className={`source ${active === d.id ? "active" : ""} ${d.duplicateOf ? "dup" : ""}`}>
                <div className="source-head">
                  <div>
                    <strong>{d.name}</strong> {status(d)}
                    {isPref(d, "photo") && <Badge kind="info">{t("photoDefault")}</Badge>}
                    <div className="muted small">{kindLabel(d.kind, settings.language)} · {backendLabel(d.driver, settings.language)}{d.usbId ? ` · USB ${d.usbId}` : ""}</div>
                  </div>
                  {active === d.id
                    ? <button disabled={act.busy || locked} onClick={() => act.run(() => api.disconnectCamera())}>{t("disconnect")}</button>
                    : <button className="primary" disabled={act.busy || locked || !!d.duplicateOf} onClick={() => act.run(() => api.connectCamera(d.id))}>{t("connect")}</button>}
                </div>
                {d.note && <p className="muted small">{d.note}</p>}
                {!d.duplicateOf && !isPref(d, "photo") && (
                  <div className="row tight">
                    <button className="ghost" disabled={act.busy || locked} onClick={() => act.run(async () => { await api.setPreferredCamera("photo", d.id); toast(tf("preferredCameraSet", { name: d.name, camera: t("photoCamera") })); })}>{t("usePhotoCamera")}</button>
                  </div>
                )}
              </li>
            ))}
          </ul>
          {settings.photoCamera.name && (
            <p className="muted small">{t("photoCamera")}: <strong>{settings.photoCamera.name}</strong> — {t("photoCameraRestoreHint")}
              <button className="ghost" disabled={locked} onClick={() => act.run(() => api.clearPreferredCamera("photo"))}>{t("clearPreference")}</button></p>
          )}
          {camera.device?.driver === "uvc" && <p className="note warn inline-note"><span>{t("webcamFrameGrabDetail")}</span></p>}
        </section>

        <section className="card" aria-label={t("livePreview")}>
          <header className="row"><h3>{t("livePreview")}{camera.device ? ` — ${camera.device.name}` : ""}</h3>
            {camera.device && (camera.liveView
              ? <button onClick={() => api.stopLiveView()}>{t("stopPreview")}</button>
              : <button className="primary" disabled={locked} onClick={() => act.run(() => api.startLiveView())}>{t("startPreview")}</button>)}
          </header>
          <LivePreview className="big-live" />
          <p className="muted small">{t("checkPreviewHint")} {settings.previewMirror ? t("previewMirroredSavedNot") : ""}</p>
        </section>
      </div>

      <CameraTest />

      <details className="advanced">
        <summary>{t("cameraAdvanced")}</summary>
        <section className="card">
          <h3>{t("videoCamera")}</h3>
          <p className="muted small">{t("videoOwnCamera")}</p>
          <ul className="sources-list">
            {src?.devices.filter((d) => canRecordVideo(d) || isPref(d, "video")).map((d) => (
              <li key={d.id} className="source"><div className="source-head"><div><strong>{d.name}</strong> {isPref(d, "video") && <Badge kind="info">{t("videoDefault")}</Badge>}</div>
                <button className="ghost" disabled={act.busy || locked || !canRecordVideo(d)} onClick={() => act.run(async () => { await api.setPreferredCamera("video", d.id); toast(tf("preferredCameraSet", { name: d.name, camera: t("videoCamera") })); })}>{t("useVideoCamera")}</button></div>
                {d.modes && d.modes.length > 0 && <p className="muted small">{t("resolutionsLabel")}: {resolutions(d.modes).join(" · ")}</p>}</li>
            ))}
          </ul>
          {settings.videoCamera.name && <p className="muted small">{t("videoDefault")}: {settings.videoCamera.name} <button className="ghost" onClick={() => act.run(() => api.clearPreferredCamera("video"))}>{t("clearPreference")}</button></p>}
        </section>
        {camera.device && activeDev?.modes && activeDev.modes.length > 0 && (
          <section className="card">
            <h3>{t("streamResolution")}</h3>
            <label className="form-line">{t("resolutionAndRate")}
              <select value={currentKey(camera.mode)} disabled={act.busy || locked} onChange={(e) => {
                const o = modeOptions(activeDev.modes).find((x) => x.key === e.target.value);
                if (o) act.run(async () => { await api.setCameraMode(activeDev.id, { width: o.width, height: o.height, fps: o.fps, format: o.format }); toast(tf("streamConfigured", { mode: o.label })); });
              }}>
                {!modeOptions(activeDev.modes).some((o) => o.key === currentKey(camera.mode)) && <option value={currentKey(camera.mode)}>{currentKey(camera.mode).replace("x", "×").replace("@", " @ ")} ({t("requestedModeLabel")})</option>}
                {modeOptions(activeDev.modes).map((o) => <option key={o.key} value={o.key}>{o.label}</option>)}
              </select>
            </label>
            {camera.mode && camera.mode.width > 0 && <p className="muted small">{tf("streamingMode", { width: camera.mode.width, height: camera.mode.height, fps: camera.mode.fpsMax ? ` @ ${Math.round(camera.mode.fpsMax)} fps` : "" })}. {t("streamChangeReconnect")}</p>}
          </section>
        )}
        <section className="card">
          <h3>{t("cameraDrivers")}</h3>
          <p className="muted small">{t("driverInfo")}</p>
          <ul className="list compact">
            {camera.drivers.map((d) => (
              <li key={d.name}><div><strong>{backendLabel(d.name, settings.language)}</strong> {d.available ? <Badge kind="ok">{t("cameraReady")}</Badge> : <Badge kind="info">{t("cameraUnavailable")}</Badge>}
                {d.problem && <div className="muted small">{d.problem}</div>}</div></li>
            ))}
          </ul>
        </section>
        <CanonPanel />
        {camera.device && caps && (
          <section className="card">
            <h3>{tf("capabilitiesFor", { camera: camera.device.name })}</h3>
            <p className="muted small">{t("capabilityRuntimeHint")}</p>
            <table className="table"><tbody>
              {CAP_LABELS.map(([k, label]) => <tr key={k}><td>{t(label)}</td><td><SupportBadge v={caps[k]} /></td></tr>)}
            </tbody></table>
            {caps.notes?.map((n, i) => <p key={i} className="muted small">• {n}</p>)}
          </section>
        )}
      </details>
    </div>
  );
}

function currentKey(m?: { width: number; height: number; fpsMax: number }): string {
  return m && m.width > 0 ? `${m.width}x${m.height}@${Math.min(30, Math.round(m.fpsMax || 30))}` : "";
}
