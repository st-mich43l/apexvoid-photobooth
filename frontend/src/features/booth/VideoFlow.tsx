import { useEffect } from "react";
import { api } from "../../shared/api";
import { clock } from "../../shared/format";
import { useApp, useT } from "../../app/state";
import { SourceChip } from "./SourceChip";
import { ErrorNote, LocalizedDiagnostic, LivePreview, SimulatedRibbon, useAction } from "../../shared/ui";

/** Video recording workflow shared by Studio and Booth. */
export function VideoFlow({ variant, onDone }: { variant: "studio" | "booth"; onDone?: () => void }) {
  const { video, camera, mediaUrl } = useApp();
  const { t } = useT();
  const { busy, error, run, clear } = useAction();
  const booth = variant === "booth";

  // live view must be on to record; leave: stop an unfinished recording so the file is finalised
  useEffect(() => {
    if (camera.device && !camera.liveView && camera.state !== "error") api.startLiveView().catch(() => {});
  }, [camera.device, camera.liveView, camera.state]);
  useEffect(() => () => { api.videoState().then(async (v) => { if (v.state === "recording") await api.stopRecording(); }).catch(() => {}); }, []);

  const rec = video.state === "recording";
  return (
    <div className={`shoot ${booth ? "booth" : ""}`}>
      {!booth && <SimulatedRibbon />}
      <SourceChip />
      <ErrorNote error={error} onClose={clear} />
      {video.state === "error" && (
        <div className="note err"><div>{video.error && <LocalizedDiagnostic text={video.error} summary="recordingWarning" />}</div></div>
      )}
      {video.notice && <div className="note warn"><LocalizedDiagnostic text={video.notice} summary="recordingWarning" /></div>}
      <div className="stage">
        <LivePreview className="stage-preview" />
        {rec && <div className="rec-pill"><i /> REC {clock(video.elapsedMs)} / {clock(video.maxSeconds * 1000)}</div>}
      </div>
      <div className="controls">
        {!rec && video.state !== "saving" && (
          <button className="shutter rec" disabled={busy || !camera.liveView} onClick={() => run(() => api.startRecording())}>
            {booth ? t("tapRecord") : t("record")}
          </button>
        )}
        {rec && <button className="shutter stop" disabled={busy} onClick={() => run(() => api.stopRecording())}>{t("stop")}</button>}
        {video.state === "saving" && <div className="center-msg"><div className="spinner" />{t("savingVideo")}</div>}
      </div>
      {video.lastMediaId && video.state === "idle" && (
        <div className="done">
          <h2>{booth ? t("yourVideo") : t("lastRecording")}</h2>
          <video key={video.lastMediaId} controls playsInline src={mediaUrl(video.lastMediaId, "original")} className="player" />
          {booth && <div className="controls"><button className="primary big" onClick={onDone}>{t("finish")}</button></div>}
        </div>
      )}
    </div>
  );
}
