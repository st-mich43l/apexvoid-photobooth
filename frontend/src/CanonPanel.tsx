import { useEffect, useState } from "react";
import { api } from "./api";
import { useApp } from "./state";
import { Badge } from "./ui";
import type { CanonStatus, ReadyState } from "./types";

const STATE_TEXT: Record<ReadyState, string> = { ok: "working", problem: "problem", not_tried: "not tried yet" };
const STATE_KIND: Record<ReadyState, "ok" | "err" | "info"> = { ok: "ok", problem: "err", not_tried: "info" };

/** Canon camera setup and readiness: what is working, what is not, what to do next.
 *  "working" is shown only for things that actually succeeded on the connected camera in this session. */
export function CanonPanel() {
  const { camera } = useApp();
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
    <section className="card canon-panel" aria-label="Canon camera setup">
      <header className="row">
        <h3>Canon camera setup</h3>
        <button onClick={load} disabled={busy}>{busy ? "Checking…" : "Check again"}</button>
      </header>
      {failed && <div className="note err"><div>{failed}</div></div>}
      {st && (<>
        {st.recommendation && <div className="note warn"><div><strong>Next step:</strong> {st.recommendation}</div></div>}
        <table className="table"><tbody>
          {st.items.map((it) => (
            <tr key={it.key} data-state={it.state}>
              <td>{it.label}</td>
              <td><Badge kind={STATE_KIND[it.state]}>{STATE_TEXT[it.state]}</Badge></td>
              <td>
                {it.detail && <span className="muted">{it.detail}</span>}
                {it.state === "problem" && it.action && <div className="action">{it.action}</div>}
              </td>
            </tr>
          ))}
        </tbody></table>
        {st.active?.counters && (
          <p className="muted small">This session: {st.active.counters.captures} photo(s) received
            {st.active.counters.lateTransfers > 0 && <>, {st.active.counters.lateTransfers} late transfer(s) ignored</>}
            {st.active.counters.invalidTransfers > 0 && <>, {st.active.counters.invalidTransfers} incomplete transfer(s) rejected</>}
            {st.active.counters.lastFile && <>; last file {st.active.counters.lastFile}</>}.</p>
        )}
        <details className="canon-guide">
          <summary>Setup guide for the Canon EOS R50 (Windows)</summary>
          <ol>{st.guide.map((g, i) => <li key={i}>{g}</li>)}</ol>
        </details>
        <p className="muted small">{st.verification}</p>
      </>)}
    </section>
  );
}
