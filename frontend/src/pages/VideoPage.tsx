import { useEffect, useState } from "react";
import { api } from "../api";
import { useApp, useT } from "../state";
import { Badge, ErrorNote, useAction } from "../ui";
import { VideoFlow } from "../VideoFlow";
import { usePrepareMode } from "../usePrepareMode";
import type { VDevices } from "../types";

export function VideoPage() {
  const { camera, settings, setSettings, toast } = useApp();
  const { t, tf } = useT();
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
      setSettings(r.settings ?? { ...settings, video: { ...v, ...patch } }); toast(t("videoSaved"));
    });

  return (
    <div className="page">
      <h1>{t("video")}</h1>
      {isCanonSDK && (
          <div className="note warn">
          <div><strong>{t("canonVideoUnavailable")}</strong>
            <div className="action">{t("canonVideoGuidance")}</div></div>
        </div>
      )}
      <ErrorNote error={q.error ?? save.error} />
      <section className="card">
        <h3>{t("videoSettingsTitle")}</h3>
        <div className="form">
          <label>{t("videoResolutionFPS")}
            {modes.length > 0 ? (
              <select value={`${v.width}x${v.height}@${v.fps}`} onChange={(e) => {
                const [wh, fps] = e.target.value.split("@"); const [w, h] = wh.split("x").map(Number); update({ width: w, height: h, fps: Number(fps) });
              }}>
                {!modes.some((m) => m.width === v.width && m.height === v.height) && <option value={`${v.width}x${v.height}@${v.fps}`}>{v.width}×{v.height} @ {v.fps} (requested)</option>}
                {modes.map((m, i) => { const fps = Math.min(30, Math.round(m.fpsMax || 30)); return (
                  <option key={i} value={`${m.width}x${m.height}@${fps}`}>{m.width}×{m.height} @ {fps} fps ({m.format})</option>); })}
              </select>
            ) : <span className="muted">{uvcSupported ? t("modeListHint") : t("connectUVCModes")} {tf("requestedMode", { width: v.width, height: v.height, fps: v.fps })}</span>}
          </label>
          <label>{t("microphone")}
            <select value={v.audioInput} onChange={(e) => update({ audioInput: e.target.value })}>
              <option value="">{t("noAudio")}</option>
              {dev?.audio.map((a) => <option key={a.id} value={a.name}>{a.name}</option>)}
            </select>
          </label>
          <label>{t("maximumDuration")}
            <input type="number" min={1} max={3600} value={v.maxSeconds} onChange={(e) => update({ maxSeconds: Math.max(1, Number(e.target.value) || 1) })} />
          </label>
        </div>
        <p className="muted small">{t("videoOutputHint")}
          {" "}<Badge kind="info">{t("previewPauseHint")}</Badge></p>
        <button onClick={scan} disabled={q.busy}>{t("rescanDevices")}</button>
      </section>
      {modeError && <div className="note err" role="alert"><div>{modeError}</div></div>}
      {ready ? <VideoFlow variant="studio" /> : !modeError && <p className="muted">{t("preparingVideoCamera")}</p>}
      {dialog}
    </div>
  );
}
