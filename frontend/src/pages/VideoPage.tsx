import { useEffect, useState } from "react";
import { api } from "../api";
import { useApp } from "../state";
import { Badge, ErrorNote, useAction } from "../ui";
import { VideoFlow } from "../VideoFlow";
import { usePrepareMode } from "../usePrepareMode";
import type { VDevices } from "../types";

export function VideoPage() {
  const { camera, settings, setSettings, toast } = useApp();
  const [dev, setDev] = useState<VDevices | null>(null);
  const q = useAction();
  const { prepare, dialog, error: modeError } = usePrepareMode();
  const [ready, setReady] = useState(false);
  useEffect(() => { prepare("video").then(setReady); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  const save = useAction();

  const scan = () => q.run(async () => setDev(await api.videoDevices()));
  useEffect(() => { scan(); }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const v = settings.video;
  const mine = dev?.video.find((d) => d.name === camera.device?.name);
  const modes = mine?.modes ?? [];
  const isCanonSDK = camera.device?.driver === "canon-edsdk";
  const uvcSupported = camera.capabilities?.streamedVideo === "supported";

  const update = (patch: Partial<typeof v>) =>
    save.run(async () => {
      const r = await api.patchSettings({ video: { ...v, ...patch } });
      setSettings(r.settings ?? { ...settings, video: { ...v, ...patch } }); toast("Video settings saved");
    });

  return (
    <div className="page">
      <h1>Video</h1>
      {isCanonSDK && (
        <div className="note warn">
          <div><strong>Camera-native recording through the Canon EDSDK is not available.</strong>
            <div className="action">Movie start/stop and movie file transfer on the EOS R50 are unverified and not implemented. Put the camera in USB streaming (UVC) mode and connect it from the UVC list on the Camera page — the app then records the streamed video on the PC.</div></div>
        </div>
      )}
      <ErrorNote error={q.error ?? save.error} />
      <section className="card">
        <h3>Recording settings</h3>
        <div className="form">
          <label>Resolution / frame rate
            {modes.length > 0 ? (
              <select value={`${v.width}x${v.height}@${v.fps}`} onChange={(e) => {
                const [wh, fps] = e.target.value.split("@"); const [w, h] = wh.split("x").map(Number); update({ width: w, height: h, fps: Number(fps) });
              }}>
                {!modes.some((m) => m.width === v.width && m.height === v.height) && <option value={`${v.width}x${v.height}@${v.fps}`}>{v.width}×{v.height} @ {v.fps} (requested)</option>}
                {modes.map((m, i) => { const fps = Math.min(30, Math.round(m.fpsMax || 30)); return (
                  <option key={i} value={`${m.width}x${m.height}@${fps}`}>{m.width}×{m.height} @ {fps} fps ({m.format})</option>); })}
              </select>
            ) : <span className="muted">{uvcSupported ? "Mode list is available once the device is listed by FFmpeg." : "Connect a UVC/webcam device to choose modes."} Requested: {v.width}×{v.height} @ {v.fps}</span>}
          </label>
          <label>Microphone
            <select value={v.audioInput} onChange={(e) => update({ audioInput: e.target.value })}>
              <option value="">No audio</option>
              {dev?.audio.map((a) => <option key={a.id} value={a.name}>{a.name}</option>)}
            </select>
          </label>
          <label>Maximum duration (seconds)
            <input type="number" min={1} max={3600} value={v.maxSeconds} onChange={(e) => update({ maxSeconds: Math.max(1, Number(e.target.value) || 1) })} />
          </label>
        </div>
        <p className="muted small">Output: MP4 (H.264 + AAC) written as fragmented MP4, so a recording survives a crash or unplugged camera.
          {" "}<Badge kind="info">preview pauses ~1 s when recording starts and stops</Badge></p>
        <button onClick={scan} disabled={q.busy}>Rescan devices</button>
      </section>
      {modeError && <div className="note err" role="alert"><div>{modeError}</div></div>}
      {ready ? <VideoFlow variant="studio" /> : !modeError && <p className="muted">Preparing the video camera…</p>}
      {dialog}
    </div>
  );
}
