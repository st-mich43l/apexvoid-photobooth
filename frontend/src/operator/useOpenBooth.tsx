import { useCallback, useRef, useState, type ReactNode } from "react";
import { api, toAppError } from "../api";
import { translate } from "../i18n";
import { useLocale } from "../locale";
import { LocalizedDiagnostic, Modal } from "../ui";
import { useApp } from "../state";
import type { ReadinessItem } from "../types";
import { blockers, FIX, readinessTranslation } from "./status";

/**
 * "Open Booth Mode" for the operator, with a fresh check first.
 *  - one launch at a time: repeated clicks get the first answer, never a second readiness run or a second launch;
 *  - required checks are re-read from the backend (not from a screen that may be stale);
 *  - if something required fails, the failing items are listed with where to fix them — nothing is launched, and
 *    no other camera is ever chosen here (switching a camera stays an explicit operator action on the Camera page).
 * Booth Mode itself still asks the backend before every guest session (startCheck), so this is a convenience, not the guard.
 */
export function useOpenBooth(launch: () => void, go: (page: string) => void): { open: () => void; busy: boolean; dialog: ReactNode } {
  const { refreshReadiness, settings } = useApp();
  const language = useLocale(settings?.language);
  const t = (key: Parameters<typeof translate>[1]) => translate(language, key);
  const fixLabels: Record<string, Parameters<typeof translate>[1]> = {
    camera: "cameraSetup", capture: "cameraSetup", storage: "librarySettings", library: "librarySettings", space: "librarySettings",
    frames: "manageFrames", printer: "printerSetup", session: "openSession", interrupted: "openGallery",
  };
  const [busy, setBusy] = useState(false);
  const [failing, setFailing] = useState<ReadinessItem[] | null>(null);
  const [error, setError] = useState("");
  const flight = useRef(false);

  const open = useCallback(() => {
    if (flight.current) return;
    flight.current = true; setBusy(true); setError("");
    void (async () => {
      try {
        const r = await api.boothReadiness();
        refreshReadiness?.();
        const bad = blockers(r);
        if (r.canStart && bad.length === 0) { setFailing(null); launch(); } else setFailing(bad);
      } catch (e) {
        const err = toAppError(e);
        setError(err.message); setFailing([]);
      } finally { flight.current = false; setBusy(false); }
    })();
  }, [launch, refreshReadiness]);

  const dialog = failing && (
    <Modal title={t("readinessBlockedTitle")} onClose={() => setFailing(null)}>
      <div className="modal-body">
        <p>{t("readinessBlockedBody")}</p>
        {error && <div className="note err" role="alert"><div><LocalizedDiagnostic text={error} summary="errorCamera" /></div></div>}
        <ul className="blockers">
          {failing.map((it) => (
            <li key={it.key}>
              {language === "vi" ? <><strong>{readinessTranslation(it, t).label}</strong><span>{readinessTranslation(it, t).detail}</span><em>{readinessTranslation(it, t).action}</em><details><summary>{t("technicalDetails")}</summary>{it.detail} {it.action}</details></> : <><strong>{it.label}</strong><span>{it.detail}</span>{it.action && <em>{it.action}</em>}</>}
              {FIX[it.key] && <button onClick={() => { setFailing(null); go(FIX[it.key][0]); }}>{language === "vi" ? t(fixLabels[it.key] ?? "settings") : FIX[it.key][1]}</button>}
            </li>
          ))}
        </ul>
      </div>
      <footer>
        <button onClick={() => setFailing(null)}>{t("close")}</button>
        <button className="primary" disabled={busy} onClick={open}>{busy ? t("checkingStatus") : t("checkAgain")}</button>
      </footer>
    </Modal>
  );
  return { open, busy, dialog };
}
