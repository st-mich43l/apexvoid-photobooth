import { useState } from "react";
import { api, toAppError } from "../api";
import { sessionGate } from "../sessionGate";
import { formatTranslation, translate } from "../i18n";
import { useLocale } from "../locale";
import { useApp } from "../state";
import { Confirm, LocalizedDiagnostic } from "../ui";
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
  const { camera, shoot, mediaUrl, settings } = useApp();
  const language = useLocale(settings?.language);
  const t = (key: Parameters<typeof translate>[1]) => translate(language, key);
  const tf = (key: Parameters<typeof translate>[1], values: Record<string, string | number>) => formatTranslation(language, key, values);
  const [ask, setAsk] = useState<Frame | "none" | null>(null);
  const [running, setRunning] = useState(false);
  const [res, setRes] = useState<Result | null>(null);
  const sim = !!camera.device?.simulated;
  const locked = LOCKED.includes(shoot?.state);
  const ready = !!camera.device && !camera.reconnecting && camera.state !== "error";
  const reason = !camera.device ? t("cameraTestConnect") : camera.reconnecting ? t("cameraTestReconnecting") : locked ? t("cameraTestBusy") : camera.state === "error" ? t("cameraTestCameraError") : "";

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
      setRes({ kind: "fail", message: err.message, action: err.action || t("cameraTestRecovery") });
    } finally {
      await sessionGate.reset();           // the one safe way back to IDLE; the photos already taken stay in the library
      if (liveBefore) void api.startLiveView().catch(() => {});
      setRunning(false);
    }
  };

  return (
    <section className="card camera-test" aria-label={t("cameraTestTitle")}>
      <h3>{t("cameraTestTitle")} <span className="muted small">{t("optional")}</span></h3>
      <p className="muted small">{tf("cameraTestIntro", { kind: t(sim ? "cameraTestSimulated" : "cameraTestReal") })}</p>
      <div className="row">
        <button className="primary" disabled={!ready || locked || running} onClick={() => { void begin(); }}>{running ? t("cameraTesting") : t("cameraTestRun")}</button>
        {reason && <span className="muted small">{reason}</span>}
      </div>
      {res?.kind === "ok" && (
        <div className="test-result ok" role="status">
          {res.mediaId && <img src={mediaUrl(res.mediaId, "preview")} alt={t("cameraTestAlt")} />}
          <div>
            <strong>{t(res.sim ? "cameraTestDemoDone" : "cameraTestSaved")}</strong>
            <p>{t(res.sim ? "cameraTestDemoDetail" : res.grab ? "cameraTestWebcamDetail" : "cameraTestDelivered")} {t("cameraTestGallery")}</p>
          </div>
        </div>
      )}
      {res?.kind === "fail" && (
        <div className="test-result fail" role="alert"><div><strong>{t("cameraTestFailed")}</strong><div><LocalizedDiagnostic text={`${res.message} ${res.action}`} summary="cameraTestDetails" /></div></div>
          <button onClick={() => { void begin(); }} disabled={!ready || running}>{t("tryAgain")}</button></div>
      )}
      {ask === "none" && <Confirm title={t("cameraTestNoFrame")} body={<p>{t("cameraTestNoFrameHint")}</p>} confirmLabel={t("ok")} onYes={() => setAsk(null)} onNo={() => setAsk(null)} />}
      {ask && ask !== "none" && (
        <Confirm title={t(sim ? "cameraTestDemoTitle" : "cameraTestRealTitle")} confirmLabel={t(sim ? "cameraTestDemoAction" : "cameraTestRealAction")} onNo={() => setAsk(null)} onYes={() => { void run(ask); }}
          body={<>
            <p>{sim ? t("cameraTestDemoWarning") : (ask.photos === 1 ? t("cameraTestShutterOne") : tf("cameraTestShutter", { count: ask.photos }))}</p>
            <p>{tf("cameraTestFrame", { name: ask.name })}</p>
          </>} />
      )}
    </section>
  );
}
