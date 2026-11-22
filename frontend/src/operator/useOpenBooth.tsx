import { useCallback, useRef, useState, type ReactNode } from "react";
import { api, toAppError } from "../api";
import { Modal } from "../ui";
import { useApp } from "../state";
import type { ReadinessItem } from "../types";
import { blockers, FIX } from "./status";

/**
 * "Open Booth Mode" for the operator, with a fresh check first.
 *  - one launch at a time: repeated clicks get the first answer, never a second readiness run or a second launch;
 *  - required checks are re-read from the backend (not from a screen that may be stale);
 *  - if something required fails, the failing items are listed with where to fix them — nothing is launched, and
 *    no other camera is ever chosen here (switching a camera stays an explicit operator action on the Camera page).
 * Booth Mode itself still asks the backend before every guest session (startCheck), so this is a convenience, not the guard.
 */
export function useOpenBooth(launch: () => void, go: (page: string) => void): { open: () => void; busy: boolean; dialog: ReactNode } {
  const { refreshReadiness } = useApp();
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
    <Modal title="Booth Mode is not ready yet" onClose={() => setFailing(null)}>
      <div className="modal-body">
        <p>Guests cannot take photos until this is fixed:</p>
        {error && <div className="note err" role="alert"><div>{error}</div></div>}
        <ul className="blockers">
          {failing.map((it) => (
            <li key={it.key}>
              <strong>{it.label}</strong><span>{it.detail}</span>{it.action && <em>{it.action}</em>}
              {FIX[it.key] && <button onClick={() => { setFailing(null); go(FIX[it.key][0]); }}>{FIX[it.key][1]}</button>}
            </li>
          ))}
        </ul>
      </div>
      <footer>
        <button onClick={() => setFailing(null)}>Close</button>
        <button className="primary" disabled={busy} onClick={open}>{busy ? "Checking…" : "Check again"}</button>
      </footer>
    </Modal>
  );
  return { open, busy, dialog };
}
