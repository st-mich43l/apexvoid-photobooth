import { useEffect, useState } from "react";
import { api } from "../api";
import { backendLabel, canRecordVideo, guidance, kindLabel, modeOptions, resolutions } from "../sourceInfo";
import { CanonPanel } from "../CanonPanel";
import { useApp } from "../state";
import { Badge, ErrorNote, LivePreview, SimulatedRibbon, SupportBadge, useAction } from "../ui";
import type { CameraSources, DeviceInfo } from "../types";

const CAP_LABELS = [
  ["liveView", "Live view"], ["stillCapture", "Still capture"], ["originalTransfer", "Full-resolution original to PC"],
  ["movieRecord", "Camera-native movie start/stop"], ["movieTransfer", "Movie file transfer"], ["streamedVideo", "Video streamed to PC"],
] as const;

/** Camera Sources: every discovered source, its backend and resolutions, connect/disconnect, preferred sources, live preview. */
export function CameraPage() {
  const { camera, settings, toast } = useApp();
  const [src, setSrc] = useState<CameraSources | null>(null);
  const scan = useAction();
  const act = useAction();

  const refresh = () => scan.run(async () => setSrc(await api.listCameraSources()));
  useEffect(() => { refresh(); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  // refresh the list after the camera set changes (replug, reconnect, preferences)
  useEffect(() => { if (src) api.listCameraSources().then(setSrc).catch(() => {}); }, [camera.device?.id, camera.reconnecting, settings.photoCamera.id, settings.videoCamera.id]); // eslint-disable-line react-hooks/exhaustive-deps

  const active = camera.device?.id;
  const caps = camera.capabilities;
  const activeDev = src?.devices.find((d) => d.id === active);
  const err = act.error ?? scan.error;

  const status = (d: DeviceInfo) => {
    if (d.duplicateOf) return <Badge kind="warn">duplicate</Badge>;
    if (active === d.id) return camera.reconnecting ? <Badge kind="warn">reconnecting</Badge> : <Badge kind="ok">connected</Badge>;
    return <Badge kind="info">available</Badge>;
  };
  const isPref = (d: DeviceInfo, p: "photo" | "video") => (p === "photo" ? src?.photoMatchId : src?.videoMatchId) === d.id;

  return (
    <div className="page">
      <header className="row"><h1>Camera sources</h1>
        <button onClick={refresh} disabled={scan.busy}>{scan.busy ? "Scanning…" : "Refresh devices"}</button></header>
      <SimulatedRibbon />
      {src?.restoreNote && <div className="note warn" role="alert"><div>{src.restoreNote}</div></div>}
      {camera.reconnecting && <div className="note warn" role="alert"><div>Camera disconnected. Check the USB cable — reconnecting automatically.</div></div>}
      <ErrorNote error={err} onClose={() => { act.clear(); scan.clear(); }} />
      {!err && camera.lastError && <ErrorNote error={{ name: "", message: camera.lastError, action: camera.lastAction || guidance(activeDev, camera.lastError) }} />}
      {src && !src.ffmpegFound && <div className="note warn"><div>FFmpeg was not found. Webcams are listed, but preview, photos and video need <code>ffmpeg.exe</code> next to the app or in Settings.</div></div>}

      <div className="two-col sources">
        <section className="card">
          <h3>Available sources</h3>
          {src && src.devices.length === 0 && <p className="muted">No camera found. Connect a camera (Canon body, webcam, capture card) and press Refresh.</p>}
          <ul className="sources-list">
            {src?.devices.map((d) => (
              <li key={d.id} className={`source ${active === d.id ? "active" : ""} ${d.duplicateOf ? "dup" : ""}`}>
                <div className="source-head">
                  <div>
                    <strong>{d.name}</strong> {status(d)}
                    {isPref(d, "photo") && <Badge kind="info">photo default</Badge>}
                    {isPref(d, "video") && <Badge kind="info">video default</Badge>}
                    <div className="muted small">{kindLabel(d.kind)} · {backendLabel(d.driver)}{d.usbId ? ` · USB ${d.usbId}` : ""}</div>
                  </div>
                  {active === d.id
                    ? <button disabled={act.busy} onClick={() => act.run(() => api.disconnectCamera())}>Disconnect</button>
                    : <button className="primary" disabled={act.busy || !!d.duplicateOf} onClick={() => act.run(() => api.connectCamera(d.id))}>Connect</button>}
                </div>
                {d.note && <p className="muted small">{d.note}</p>}
                {d.modes && d.modes.length > 0 && <p className="muted small">Resolutions: {resolutions(d.modes).join(" · ")}</p>}
                {!d.duplicateOf && (
                  <div className="row tight">
                    <button className="ghost" disabled={act.busy} onClick={() => act.run(async () => { await api.setPreferredCamera("photo", d.id); toast(`${d.name} is now the Photo camera`); })}>Use for Photo</button>
                    <button className="ghost" disabled={act.busy || !canRecordVideo(d)} title={canRecordVideo(d) ? "" : "Video needs a webcam/UVC source"}
                      onClick={() => act.run(async () => { await api.setPreferredCamera("video", d.id); toast(`${d.name} is now the Video camera`); })}>Use for Video</button>
                  </div>
                )}
              </li>
            ))}
          </ul>
          {(settings.photoCamera.name || settings.videoCamera.name) && (
            <p className="muted small">
              Photo default: {settings.photoCamera.name || "—"}{settings.photoCamera.name && <button className="ghost" onClick={() => act.run(() => api.clearPreferredCamera("photo"))}>clear</button>}
              {" · "}Video default: {settings.videoCamera.name || "—"}{settings.videoCamera.name && <button className="ghost" onClick={() => act.run(() => api.clearPreferredCamera("video"))}>clear</button>}
            </p>
          )}
          <h4>Backends</h4>
          <ul className="list compact">
            {camera.drivers.map((d) => (
              <li key={d.name}><div><strong>{backendLabel(d.name)}</strong> {d.available ? <Badge kind="ok">ready</Badge> : <Badge kind="warn">unavailable</Badge>}
                {d.problem && <div className="muted small">{d.problem}</div>}</div></li>
            ))}
          </ul>
        </section>

        <section className="card">
          <header className="row"><h3>Live preview{camera.device ? ` — ${camera.device.name}` : ""}</h3>
            {camera.device && (camera.liveView
              ? <button onClick={() => api.stopLiveView()}>Stop preview</button>
              : <button className="primary" onClick={() => act.run(() => api.startLiveView())}>Start preview</button>)}
          </header>
          <LivePreview />
          {camera.device && activeDev?.modes && activeDev.modes.length > 0 && (
            <label className="form-line">Resolution and frame rate
              <select value={currentKey(camera.mode)} disabled={act.busy} onChange={(e) => {
                const o = modeOptions(activeDev.modes).find((x) => x.key === e.target.value);
                if (o) act.run(async () => { await api.setCameraMode(activeDev.id, { width: o.width, height: o.height, fps: o.fps, format: o.format }); toast(`Stream set to ${o.label}`); });
              }}>
                {!modeOptions(activeDev.modes).some((o) => o.key === currentKey(camera.mode)) && <option value={currentKey(camera.mode)}>{currentKey(camera.mode).replace("x", "×").replace("@", " @ ")} (requested)</option>}
                {modeOptions(activeDev.modes).map((o) => <option key={o.key} value={o.key}>{o.label}</option>)}
              </select>
            </label>
          )}
          {camera.device && camera.mode && camera.mode.width > 0 && <p className="muted small">Streaming {camera.mode.width}×{camera.mode.height}{camera.mode.fpsMax ? ` @ ${Math.round(camera.mode.fpsMax)} fps` : ""}. Changing it reconnects the camera.</p>}
        </section>
      </div>

      <CanonPanel />

      {camera.device && caps && (
        <section className="card">
          <h3>Capabilities — {camera.device.name}</h3>
          <p className="muted small">Reported by the backend at runtime. “Unverified” means the function exists but has not succeeded on this camera yet; it is promoted to “supported” only after a real success.</p>
          <table className="table"><tbody>
            {CAP_LABELS.map(([k, label]) => <tr key={k}><td>{label}</td><td><SupportBadge v={caps[k]} /></td></tr>)}
          </tbody></table>
          {camera.device.driver === "uvc" && <p className="note warn"><span>Photos from this source are <strong>frame grabs</strong> from the video stream at the stream resolution — not full-resolution camera originals.</span></p>}
          {caps.notes?.map((n, i) => <p key={i} className="muted small">• {n}</p>)}
        </section>
      )}
    </div>
  );
}

function currentKey(m?: { width: number; height: number; fpsMax: number }): string {
  return m && m.width > 0 ? `${m.width}x${m.height}@${Math.min(30, Math.round(m.fpsMax || 30))}` : "";
}
