import { useEffect, useState } from "react";
import { api } from "../../shared/api";
import { formatTranslation, translate } from "../../shared/i18n";
import { useLocale } from "../../shared/locale";
import { useApp } from "../../app/state";
import { Badge } from "../../shared/ui";
import type { CanonStatus, ReadyState } from "../../shared/types";

const STATE_TEXT = { ok: "canonWorking", problem: "canonProblem", not_tried: "canonNotTried" } as const;
const STATE_KIND: Record<ReadyState, "ok" | "err" | "info"> = { ok: "ok", problem: "err", not_tried: "info" };

/** Canon camera setup and readiness: what is working, what is not, what to do next.
 *  "working" is shown only for things that actually succeeded on the connected camera in this session. */
export function CanonPanel() {
  const { camera, settings } = useApp();
  const language = useLocale(settings?.language);
  const t = (key: Parameters<typeof translate>[1]) => translate(language, key);
  const tf = (key: Parameters<typeof translate>[1], values: Record<string, string | number>) => formatTranslation(language, key, values);
  const itemKeys = { sdk: "canonSdkLabel", init: "canonInitLabel", conflicts: "canonConflictsLabel", detected: "canonDetectedLabel", connected: "canonConnectedLabel", liveview: "canonLiveLabel", still: "canonStillLabel", original: "canonOriginalLabel", lasterror: "canonLastErrorLabel" } as const;
  const guideKeys = { usb_mode: "canonGuideUsb", photo_mode: "canonGuideMode", image_quality: "canonGuideQuality", close_apps: "canonGuideApps", usb_cable: "canonGuideCable", sdk: "canonGuideSdk" } as const;
  const [st, setSt] = useState<CanonStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState("");
  const load = async () => {
    setBusy(true);
    try { setSt(await api.canonStatus()); setFailed(""); } catch (e) { setFailed(e instanceof Error ? e.message : String(e)); } finally { setBusy(false); }
  };
  // re-read when the camera connects, disconnects or reports something new
  useEffect(() => { load(); }, [camera.device?.id, camera.state, camera.reconnecting, camera.lastError, camera.liveView]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <section className="card canon-panel" aria-label={t("canonSetup")}>
      <header className="row">
        <h3>{t("canonSetup")}</h3>
        <button onClick={load} disabled={busy}>{busy ? t("checkingStatus") : t("checkAgain")}</button>
      </header>
      {failed && <div className="note err"><div>{failed}</div></div>}
      {st && (<>
        {st.recommendation && <div className="note warn"><div><strong>{t("canonNextStep")}</strong> {language === "vi" ? t("readinessCameraHelp") : st.recommendation}{language === "vi" && <details><summary>{t("technicalDetails")}</summary>{st.recommendation}</details>}</div></div>}
        <table className="table"><tbody>
          {st.items.map((it) => (
            <tr key={it.key} data-state={it.state}>
              <td>{it.key in itemKeys ? t(itemKeys[it.key as keyof typeof itemKeys]) : it.label}</td>
              <td><Badge kind={STATE_KIND[it.state]}>{t(STATE_TEXT[it.state])}</Badge></td>
              <td>
                {language === "vi" ? <>{it.state === "problem" && <span>{t("readinessCameraHelp")}</span>}{(it.detail || it.action) && <details><summary>{t("technicalDetails")}</summary>{it.detail} {it.action}</details>}</> : <>{it.detail && <span className="muted">{it.detail}</span>}{it.state === "problem" && it.action && <div className="action">{it.action}</div>}</>}
              </td>
            </tr>
          ))}
        </tbody></table>
        {st.active?.counters && (
          <p className="muted small">{t("canonThisSession")}: {tf("canonReceived", { count: st.active.counters.captures })}
            {st.active.counters.lateTransfers > 0 && <>, {tf("canonLateIgnored", { count: st.active.counters.lateTransfers })}</>}
            {st.active.counters.invalidTransfers > 0 && <>, {tf("canonIncompleteRejected", { count: st.active.counters.invalidTransfers })}</>}
            {st.active.counters.lastFile && <>; {tf("canonLastFile", { name: st.active.counters.lastFile })}</>}.</p>
        )}
        <details className="canon-guide">
          <summary>{t("canonGuideTitle")}</summary>
          <ol>{st.guide.map((g, i) => { const code = st.guideCodes?.[i]; return <li key={i}>{code && code in guideKeys ? t(guideKeys[code as keyof typeof guideKeys]) : g}</li>; })}</ol>
        </details>
        <p className="muted small">{language === "vi" ? t("canonVerificationNote") : st.verification}</p>
      </>)}
    </section>
  );
}
