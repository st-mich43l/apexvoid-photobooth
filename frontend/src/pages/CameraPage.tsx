import { useEffect, useState } from "react";
import { api } from "../api";
import { backendLabel, canRecordVideo, guidance, kindLabel, modeOptions, resolutions } from "../sourceInfo";
import { CanonPanel } from "../CanonPanel";
import { useApp } from "../state";
import { Badge, ErrorNote, LivePreview, SupportBadge, useAction } from "../ui";
import { CameraTest } from "../operator/CameraTest";
import { useLatest } from "../operator/latest";
import { PageHeader, StateBlock } from "../operator/parts";
import { Icon } from "../icons";
import type { CameraSources, DeviceInfo } from "../types";

const CAP_LABELS = [
  ["liveView", "Live view"], ["stillCapture", "Still capture"], ["originalTransfer", "Full-resolution original to PC"],
  ["movieRecord", "Camera-native movie start/stop"], ["movieTransfer", "Movie file transfer"], ["streamedVideo", "Video streamed to PC"],
] as const;

/** Camera setup. Primary view: choose the photo camera, connect, see it live, run an optional test. Everything else
 *  (video camera, resolution, backends, Canon diagnostics, capabilities) is under "Advanced". */
export function CameraPage() {
  const { camera, settings, toast, shoot } = useApp();
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
    if (d.duplicateOf) return <Badge kind="warn">duplicate</Badge>;
    if (active === d.id) return camera.reconnecting ? <Badge kind="warn">reconnecting</Badge> : d.simulated ? <Badge kind="sim">demo · connected</Badge> : <Badge kind="ok">connected</Badge>;
    return <Badge kind="info">available</Badge>;
  };
  const isPref = (d: DeviceInfo, p: "photo" | "video") => (p === "photo" ? src?.photoMatchId : src?.videoMatchId) === d.id;

  return (
    <div className="page">
      <PageHeader title="Camera" sub="Choose the photo camera, check the picture, and test it if you like.">
        <button onClick={() => { void refresh(); }} disabled={scan.busy}><Icon name="refresh" size={16} /> {scan.busy ? "Scanning…" : "Refresh devices"}</button>
      </PageHeader>
      {src?.restoreNote && <div className="note warn" role="alert"><div>{src.restoreNote}</div></div>}
      {camera.reconnecting && <div className="note warn" role="alert"><div>Camera disconnected. Check the USB cable — reconnecting automatically. Photos already taken are safe.</div></div>}
      {locked && <div className="note info" role="status"><div>A photo session is running, so the camera cannot be changed right now. Finish or cancel the session first.</div></div>}
      {sim && <div className="note info" role="status"><div><strong>Demo / test mode.</strong> The SIMULATED camera produces test patterns. It is useful for trying the screens and is never a hardware check.</div></div>}
      <ErrorNote error={err} onClose={() => { act.clear(); scan.clear(); }} />
      {!err && camera.lastError && <ErrorNote error={{ name: "", message: camera.lastError, action: camera.lastAction || guidance(activeDev, camera.lastError) }} />}
      {src && !src.ffmpegFound && <div className="note warn"><div>FFmpeg was not found. Webcams are listed, but preview and photos need <code>ffmpeg.exe</code> next to the app or its path in Settings. Canon cameras do not need it.</div></div>}

      <div className="two-col sources">
        <section className="card" aria-label="Photo camera">
          <h3>Photo camera</h3>
          {scan.busy && !src && <StateBlock kind="loading">Looking for cameras…</StateBlock>}
          {src && src.devices.length === 0 && <StateBlock kind="empty">No camera found. Connect a camera (Canon body, webcam, capture card) and press Refresh devices.</StateBlock>}
          <ul className="sources-list">
            {src?.devices.map((d) => (
              <li key={d.id} className={`source ${active === d.id ? "active" : ""} ${d.duplicateOf ? "dup" : ""}`}>
                <div className="source-head">
                  <div>
                    <strong>{d.name}</strong> {status(d)}
                    {isPref(d, "photo") && <Badge kind="info">photo default</Badge>}
                    <div className="muted small">{kindLabel(d.kind)} · {backendLabel(d.driver)}{d.usbId ? ` · USB ${d.usbId}` : ""}</div>
                  </div>
                  {active === d.id
                    ? <button disabled={act.busy || locked} onClick={() => act.run(() => api.disconnectCamera())}>Disconnect</button>
                    : <button className="primary" disabled={act.busy || locked || !!d.duplicateOf} onClick={() => act.run(() => api.connectCamera(d.id))}>Connect</button>}
                </div>
                {d.note && <p className="muted small">{d.note}</p>}
                {!d.duplicateOf && !isPref(d, "photo") && (
                  <div className="row tight">
                    <button className="ghost" disabled={act.busy || locked} onClick={() => act.run(async () => { await api.setPreferredCamera("photo", d.id); toast(`${d.name} is now the Photo camera`); })}>Use as the Photo camera</button>
                  </div>
                )}
              </li>
            ))}
          </ul>
          {settings.photoCamera.name && (
            <p className="muted small">Photo camera: <strong>{settings.photoCamera.name}</strong> — restored when the app starts. If it is missing the app asks before using another camera.
              <button className="ghost" disabled={locked} onClick={() => act.run(() => api.clearPreferredCamera("photo"))}>clear</button></p>
          )}
          {camera.device?.driver === "uvc" && <p className="note warn inline-note"><span>Photos from this webcam are <strong>frame grabs</strong> from the video stream, at the stream resolution.</span></p>}
        </section>

        <section className="card" aria-label="Live preview">
          <header className="row"><h3>Live preview{camera.device ? ` — ${camera.device.name}` : ""}</h3>
            {camera.device && (camera.liveView
              ? <button onClick={() => api.stopLiveView()}>Stop preview</button>
              : <button className="primary" disabled={locked} onClick={() => act.run(() => api.startLiveView())}>Start preview</button>)}
          </header>
          <LivePreview className="big-live" />
          <p className="muted small">Check that the picture is sharp, framed and not mirrored the wrong way. {settings.previewMirror ? "The preview is mirrored; saved photos are not." : ""}</p>
        </section>
      </div>

      <CameraTest />

      <details className="advanced">
        <summary>Advanced — video camera, resolution, drivers and diagnostics</summary>
        <section className="card">
          <h3>Video camera</h3>
          <p className="muted small">Video recording has its own default camera, separate from the Photo camera.</p>
          <ul className="sources-list">
            {src?.devices.filter((d) => canRecordVideo(d) || isPref(d, "video")).map((d) => (
              <li key={d.id} className="source"><div className="source-head"><div><strong>{d.name}</strong> {isPref(d, "video") && <Badge kind="info">video default</Badge>}</div>
                <button className="ghost" disabled={act.busy || locked || !canRecordVideo(d)} onClick={() => act.run(async () => { await api.setPreferredCamera("video", d.id); toast(`${d.name} is now the Video camera`); })}>Use for Video</button></div>
                {d.modes && d.modes.length > 0 && <p className="muted small">Resolutions: {resolutions(d.modes).join(" · ")}</p>}</li>
            ))}
          </ul>
          {settings.videoCamera.name && <p className="muted small">Video default: {settings.videoCamera.name} <button className="ghost" onClick={() => act.run(() => api.clearPreferredCamera("video"))}>clear</button></p>}
        </section>
        {camera.device && activeDev?.modes && activeDev.modes.length > 0 && (
          <section className="card">
            <h3>Stream resolution</h3>
            <label className="form-line">Resolution and frame rate
              <select value={currentKey(camera.mode)} disabled={act.busy || locked} onChange={(e) => {
                const o = modeOptions(activeDev.modes).find((x) => x.key === e.target.value);
                if (o) act.run(async () => { await api.setCameraMode(activeDev.id, { width: o.width, height: o.height, fps: o.fps, format: o.format }); toast(`Stream set to ${o.label}`); });
              }}>
                {!modeOptions(activeDev.modes).some((o) => o.key === currentKey(camera.mode)) && <option value={currentKey(camera.mode)}>{currentKey(camera.mode).replace("x", "×").replace("@", " @ ")} (requested)</option>}
                {modeOptions(activeDev.modes).map((o) => <option key={o.key} value={o.key}>{o.label}</option>)}
              </select>
            </label>
            {camera.mode && camera.mode.width > 0 && <p className="muted small">Streaming {camera.mode.width}×{camera.mode.height}{camera.mode.fpsMax ? ` @ ${Math.round(camera.mode.fpsMax)} fps` : ""}. Changing it reconnects the camera.</p>}
          </section>
        )}
        <section className="card">
          <h3>Camera drivers</h3>
          <p className="muted small">A driver that is not available only matters if you use a camera that needs it (Canon needs its SDK; webcams need FFmpeg).</p>
          <ul className="list compact">
            {camera.drivers.map((d) => (
              <li key={d.name}><div><strong>{backendLabel(d.name)}</strong> {d.available ? <Badge kind="ok">ready</Badge> : <Badge kind="info">not available</Badge>}
                {d.problem && <div className="muted small">{d.problem}</div>}</div></li>
            ))}
          </ul>
        </section>
        <CanonPanel />
        {camera.device && caps && (
          <section className="card">
            <h3>Capabilities — {camera.device.name}</h3>
            <p className="muted small">Reported by the backend at runtime. “Unverified” means the function exists but has not succeeded on this camera yet; it is promoted to “supported” only after a real success.</p>
            <table className="table"><tbody>
              {CAP_LABELS.map(([k, label]) => <tr key={k}><td>{label}</td><td><SupportBadge v={caps[k]} /></td></tr>)}
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
