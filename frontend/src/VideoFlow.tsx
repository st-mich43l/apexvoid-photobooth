import { useEffect } from "react";
import { api } from "./api";
import { clock } from "./format";
import { useApp } from "./state";
import { SourceChip } from "./SourceChip";
import { ErrorNote, LivePreview, SimulatedRibbon, useAction } from "./ui";

/** Video recording workflow shared by Studio and Booth. */
export function VideoFlow({ variant, onDone }: { variant: "studio" | "booth"; onDone?: () => void }) {
  const { video, camera, mediaUrl } = useApp();
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
        <div className="note err"><div><strong>{video.error}</strong></div></div>
      )}
      {video.notice && <div className="note warn">{video.notice}</div>}
      <div className="stage">
        <LivePreview className="stage-preview" />
        {rec && <div className="rec-pill"><i /> REC {clock(video.elapsedMs)} / {clock(video.maxSeconds * 1000)}</div>}
      </div>
      <div className="controls">
        {!rec && video.state !== "saving" && (
          <button className="shutter rec" disabled={busy || !camera.liveView} onClick={() => run(() => api.startRecording())}>
            {booth ? "TAP TO RECORD" : "Record"}
          </button>
        )}
        {rec && <button className="shutter stop" disabled={busy} onClick={() => run(() => api.stopRecording())}>Stop</button>}
        {video.state === "saving" && <div className="center-msg"><div className="spinner" />Saving video…</div>}
      </div>
      {video.lastMediaId && video.state === "idle" && (
        <div className="done">
          <h2>{booth ? "Your video" : "Last recording"}</h2>
          <video key={video.lastMediaId} controls playsInline src={mediaUrl(video.lastMediaId, "original")} className="player" />
          {booth && <div className="controls"><button className="primary big" onClick={onDone}>Finish</button></div>}
        </div>
      )}
    </div>
  );
}
