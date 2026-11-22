import { useState } from "react";
import { api, toAppError } from "../api";
import { sessionGate } from "../sessionGate";
import { useApp } from "../state";
import { Confirm } from "../ui";
import type { Frame, ShootStateName } from "../types";

const LOCKED: ShootStateName[] = ["COUNTDOWN", "CAPTURING", "REVIEW", "SAVING"];
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

/** Waits until the engine reports one of `want` (or ERROR/CANCELLED, which end the wait with an explanation). */
async function until(want: ShootStateName[], ms: number) {
  const end = Date.now() + ms;
  for (;;) {
    const s = await api.shootState();
    if (want.includes(s.state)) return s;
    if (s.state === "ERROR" || s.state === "CANCELLED") throw Object.assign(new Error(s.error || "The camera test was cancelled."), { action: s.errorAction });
    if (Date.now() > end) throw new Error("The camera did not respond in time.");
    await sleep(150);
  }
}

type Result = { kind: "ok"; mediaId: string; sim: boolean; grab: boolean; sessionId: string } | { kind: "fail"; message: string; action: string };

/**
 * An explicit camera test. It uses the normal capture lifecycle (preview → shoot → review → save) with the real
 * shutter, so the operator is told first what will happen. The saved session is labelled "Operator camera test" in the
 * library and is kept: nothing here deletes it. A simulated camera produces a test pattern, which is reported as a demo,
 * never as a hardware check.
 */
export function CameraTest() {
  const { camera, shoot, mediaUrl } = useApp();
  const [ask, setAsk] = useState<Frame | "none" | null>(null);
  const [running, setRunning] = useState(false);
  const [res, setRes] = useState<Result | null>(null);
  const sim = !!camera.device?.simulated;
  const locked = LOCKED.includes(shoot?.state);
  const ready = !!camera.device && !camera.reconnecting && camera.state !== "error";
  const reason = !camera.device ? "Connect a camera first." : camera.reconnecting ? "The camera is reconnecting." : locked ? "A photo session is running — wait for it to finish." : camera.state === "error" ? "The camera reported an error." : "";

  const begin = async () => {
    const { frames } = await api.listFrames(false);
    const pick = frames.find((f) => f.photos === 1) ?? frames[0];
    setAsk(pick ?? "none");
  };

  const run = async (frame: Frame) => {
    setAsk(null); setRes(null); setRunning(true);
    const liveBefore = camera.liveView;
    let sessionId = "";
    try {
      await sessionGate.ready();
      if ((await api.shootState()).state === "IDLE") await api.startPreview();
      await until(["PREVIEW"], 15_000);
      await api.startShooting(frame.id, "", "");
      let s = await until(["REVIEW"], 60_000);
      sessionId = s.sessionId;
      await api.labelTestSession(sessionId); // marked before anything else can go wrong
      await api.accept();
      s = await until(["COMPLETED"], 30_000);
      setRes({ kind: "ok", mediaId: s.resultId || s.taken[0]?.mediaId || "", sim, grab: camera.device?.driver === "uvc", sessionId });
    } catch (e) {
      const err = toAppError(e);
      setRes({ kind: "fail", message: err.message, action: err.action || "Check the cable and that the camera is in photo mode, then try again." });
    } finally {
      await sessionGate.reset();           // the one safe way back to IDLE; the photos already taken stay in the library
      if (liveBefore) void api.startLiveView().catch(() => {});
      setRunning(false);
    }
  };

  return (
    <section className="card camera-test" aria-label="Camera test">
      <h3>Camera test <span className="muted small">optional</span></h3>
      <p className="muted small">Checks that the shutter fires and the photo reaches the PC. It takes {sim ? "a simulated" : "a real"} photo, which is saved in the Gallery as “Operator camera test” and kept.</p>
      <div className="row">
        <button className="primary" disabled={!ready || locked || running} onClick={() => { void begin(); }}>{running ? "Testing…" : "Run camera test…"}</button>
        {reason && <span className="muted small">{reason}</span>}
      </div>
      {res?.kind === "ok" && (
        <div className="test-result ok" role="status">
          {res.mediaId && <img src={mediaUrl(res.mediaId, "preview")} alt="Camera test photo" />}
          <div>
            <strong>{res.sim ? "Demo test finished" : "Test photo saved"}</strong>
            <p>{res.sim ? "This was the SIMULATED camera: a test pattern. It does not verify real hardware." : res.grab ? "The camera delivered a photo. Photos from a webcam are frame grabs from the video stream, not full-resolution originals." : "The camera delivered a photo."} It is in the Gallery as “Operator camera test”.</p>
          </div>
        </div>
      )}
      {res?.kind === "fail" && (
        <div className="test-result fail" role="alert"><div><strong>The camera test failed.</strong><p>{res.message}</p><p className="action">{res.action}</p></div>
          <button onClick={() => { void begin(); }} disabled={!ready || running}>Try again</button></div>
      )}
      {ask === "none" && <Confirm title="No frame to test with" body={<p>Switch on at least one frame on the Frames page first. The test uses the normal photo session.</p>} confirmLabel="OK" onYes={() => setAsk(null)} onNo={() => setAsk(null)} />}
      {ask && ask !== "none" && (
        <Confirm title={sim ? "Run a demo camera test?" : "Take a real test photo?"} confirmLabel={sim ? "Run demo test" : "Take test photo"} onNo={() => setAsk(null)} onYes={() => { void run(ask); }}
          body={<>
            <p>{sim ? "The SIMULATED camera will produce a test pattern. This does not test real hardware." : <>The camera <strong>will fire its shutter</strong> {ask.photos === 1 ? "once" : `${ask.photos} times`} after a countdown.</>}</p>
            <p>It uses the “{ask.name}” frame and the normal photo session. The result is saved to the library as <strong>Operator camera test</strong> and is not deleted automatically.</p>
          </>} />
      )}
    </section>
  );
}
