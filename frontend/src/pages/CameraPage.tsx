import { useEffect, useState } from "react";
import { api } from "../api";
import { useApp } from "../state";
import { Badge, ErrorNote, LivePreview, SimulatedRibbon, SupportBadge, useAction } from "../ui";
import type { DeviceInfo } from "../types";

const CAP_LABELS: Array<[keyof NonNullable<ReturnType<typeof useApp>["camera"]["capabilities"]>, string]> = [
  ["liveView", "Live view"], ["stillCapture", "Still capture"], ["originalTransfer", "Full-resolution original to PC"],
  ["movieRecord", "Camera-native movie start/stop"], ["movieTransfer", "Movie file transfer"], ["streamedVideo", "Video streamed to PC"],
];

export function CameraPage() {
  const { camera } = useApp();
  const [devices, setDevices] = useState<DeviceInfo[]>([]);
  const [scanned, setScanned] = useState(false);
  const scan = useAction();
  const conn = useAction();

  const refresh = () => scan.run(async () => { setDevices(await api.listCameras()); setScanned(true); });
  useEffect(() => { refresh(); }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const active = camera.device?.id;
  const caps = camera.capabilities;
  return (
    <div className="page">
      <h1>Camera</h1>
      <SimulatedRibbon />
      {camera.reconnecting && <div className="note warn">Camera disconnected. Check the USB cable — reconnecting automatically.</div>}
      <ErrorNote error={conn.error ?? scan.error} onClose={() => { conn.clear(); scan.clear(); }} />
      {!conn.error && !scan.error && camera.lastError && <ErrorNote error={{ name: "", message: camera.lastError, action: camera.lastAction ?? "" }} />}

      <div className="two-col">
        <section className="card">
          <header className="row"><h3>Devices</h3><button onClick={refresh} disabled={scan.busy}>{scan.busy ? "Scanning…" : "Refresh"}</button></header>
          {scanned && devices.length === 0 && <p className="muted">No cameras found. Connect the camera by USB and switch it on.</p>}
          <ul className="list">
            {devices.map((d) => (
              <li key={d.id}>
                <div>
                  <strong>{d.name}</strong>{" "}
                  <Badge kind={d.simulated ? "sim" : "info"}>{d.simulated ? "SIMULATED" : d.driver}</Badge>
                  {active === d.id && <Badge kind="ok">connected</Badge>}
                </div>
                {active === d.id
                  ? <button disabled={conn.busy} onClick={() => conn.run(() => api.disconnectCamera())}>Disconnect</button>
                  : <button className="primary" disabled={conn.busy} onClick={() => conn.run(() => api.connectCamera(d.id))}>Connect</button>}
              </li>
            ))}
          </ul>
          <h4>Drivers</h4>
          <ul className="list compact">
            {camera.drivers.map((d) => (
              <li key={d.name}><div><strong>{d.name}</strong> {d.available ? <Badge kind="ok">ready</Badge> : <Badge kind="warn">unavailable</Badge>}
                {d.problem && <div className="muted small">{d.problem}</div>}</div></li>
            ))}
          </ul>
        </section>

        <section className="card">
          <header className="row"><h3>Live view</h3>
            {camera.device && (camera.liveView
              ? <button onClick={() => api.stopLiveView()}>Stop</button>
              : <button className="primary" onClick={() => conn.run(() => api.startLiveView())}>Start</button>)}
          </header>
          <LivePreview />
        </section>
      </div>

      {camera.device && caps && (
        <section className="card">
          <h3>Capabilities — {camera.device.name}</h3>
          <p className="muted small">Reported by the driver at runtime. “Unverified” means the API exists but has not succeeded on this camera yet; it is promoted to “supported” only after a real success.</p>
          <table className="table"><tbody>
            {CAP_LABELS.map(([k, label]) => <tr key={k}><td>{label}</td><td><SupportBadge v={caps[k] as never} /></td></tr>)}
          </tbody></table>
          {caps.notes?.map((n, i) => <p key={i} className="muted small">• {n}</p>)}
        </section>
      )}
    </div>
  );
}
