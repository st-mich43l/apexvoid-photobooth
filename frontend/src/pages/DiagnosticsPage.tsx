import { useEffect, useState } from "react";
import { api } from "../api";
import { Badge, ErrorNote, useAction } from "../ui";
import type { Diagnostics } from "../types";

export function DiagnosticsPage() {
  const [d, setD] = useState<Diagnostics | null>(null);
  const { busy, error, run } = useAction();
  const load = () => run(async () => setD(await api.diagnostics()));
  useEffect(() => { load(); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  return (
    <div className="page">
      <header className="row"><h1>Hardware diagnostics</h1><button onClick={load} disabled={busy}>{busy ? "Checking…" : "Run again"}</button></header>
      <ErrorNote error={error} />
      {d && (<>
        <section className="card"><h3>Checks <span className="muted small">{d.platform}</span></h3>
          <table className="table"><tbody>
            {d.checks.map((c, i) => (
              <tr key={i}><td>{c.name}</td><td>{c.ok ? <Badge kind="ok">ok</Badge> : <Badge kind="err">problem</Badge>}</td>
                <td><span className="muted">{c.detail}</span>{!c.ok && c.action && <div className="action">{c.action}</div>}</td></tr>))}
          </tbody></table>
        </section>
        <section className="card"><h3>Canon EOS R50 — what is and is not verified</h3>
          <ul>{d.movieFindings.map((m, i) => <li key={i}>{m}</li>)}</ul>
          <p className="muted small">Hardware acceptance (detect, live view, shutter, JPEG download, USB-drop recovery, clean exit) is documented in docs/HARDWARE_TESTS.md and must be run on a Windows PC with the camera and Canon's SDK.</p>
        </section>
        <section className="card"><h3>Privacy &amp; offline</h3><p>{d.network}</p><p className="muted small">Logs: {d.logDir}</p></section>
      </>)}
    </div>
  );
}
