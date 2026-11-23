import { useEffect, useState } from "react";
import { api } from "../api";
import { Icon } from "../icons";
import { PageHeader, StateBlock, StatusPill } from "../operator/parts";
import type { View } from "../operator/status";
import { useApp } from "../state";
import { ErrorNote, useAction } from "../ui";
import type { Check, Diagnostics } from "../types";

/** Each check answers: what is wrong, does it stop guests, what to do, can I retry. */
const viewOfCheck = (c: Check): View => (c.state === "problem" ? "setup" : c.state === "warn" ? "warning" : c.state === "info" ? "info" : "ready");
const pillLabel = (c: Check) => (c.state === "problem" ? (c.blocksGuests ? "Stops guests" : "Problem") : c.state === "warn" ? "Warning" : c.state === "info" ? "Not needed" : "OK");

export function DiagnosticsPage() {
  const { toast } = useApp();
  const [d, setD] = useState<Diagnostics | null>(null);
  const { busy, error, run } = useAction();
  const sum = useAction();
  const bundle = useAction();
  const [summary, setSummary] = useState("");
  const load = () => run(async () => setD(await api.diagnostics()));
  useEffect(() => { load(); }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const copy = () => sum.run(async () => {
    const text = await api.diagnosticSummary();
    setSummary(text);
    try { await navigator.clipboard.writeText(text); toast("Support summary copied"); } catch { toast("Select the text below and copy it (the clipboard is not available)", "err"); }
  });

  const exportBundle = () => bundle.run(async () => {
    const folder = await api.pickFolder();
    if (!folder) return;
    const path = await api.exportSupportBundle(folder);
    toast(`Support bundle saved to ${path}`);
  });

  const blocking = d?.checks.filter((c) => c.blocksGuests) ?? [];
  const order = (c: Check) => (c.blocksGuests ? 0 : c.state === "problem" ? 1 : c.state === "warn" ? 2 : c.state === "info" ? 4 : 3);
  const checks = [...(d?.checks ?? [])].sort((a, b) => order(a) - order(b));
  return (
    <div className="page">
      <PageHeader title="Diagnostics" sub="What is wrong, whether it stops guests, and what to do.">
        <button onClick={exportBundle} disabled={bundle.busy}>Export support bundle</button>
        <button onClick={copy} disabled={sum.busy}><Icon name="upload" size={16} /> Copy support summary</button>
        <button onClick={load} disabled={busy}>{busy ? "Checking…" : "Run again"}</button>
      </PageHeader>
      <ErrorNote error={error ?? sum.error ?? bundle.error} />
      {!d && !error && <StateBlock kind="loading">Checking this computer…</StateBlock>}
      {d && (<>
        <div className={`note ${blocking.length ? "err" : "info"}`} role="status"><div>{blocking.length ? `${blocking.length} problem${blocking.length > 1 ? "s" : ""} stop${blocking.length > 1 ? "" : "s"} guests from taking photos.` : "Nothing here stops guests from taking photos."} Items marked “Not needed” do not apply to your current camera set-up.</div></div>
        <section className="card"><h3>Checks <span className="muted small">{d.platform}</span></h3>
          <ul className="diag-list">
            {checks.map((c, i) => (
              <li key={i} className={`diag ${c.state}`}>
                <StatusPill view={viewOfCheck(c)} label={pillLabel(c)} />
                <div><strong>{c.name}</strong><span className="muted"> {c.detail}</span>
                  {c.state !== "ok" && c.action && <div className="action">{c.action}{c.state === "problem" || c.state === "warn" ? " It is safe to run the check again." : ""}</div>}</div>
              </li>))}
          </ul>
        </section>
        <section className="card"><h3>Canon EOS R50 — what is and is not verified</h3>
          <ul>{d.movieFindings.map((m, i) => <li key={i}>{m}</li>)}</ul>
          <p className="muted small">Verification levels: <b>automated</b> (tests, no hardware) · <b>hardware verified</b> (a real camera did it) · <b>unverified</b> (reported but never proven). Hardware acceptance is documented in docs/HARDWARE_TESTS.md and must be run on a Windows PC with the camera and Canon's SDK.</p>
        </section>
        <section className="card"><h3>Privacy &amp; offline</h3><p>{d.network}</p><p className="muted small">Logs: {d.logDir}</p>
          <p className="muted small">The support bundle includes readiness and sanitized event counts, without raw logs or media. The support summary contains status and counts only — no PIN, no photos or file names, no folder paths, printer name or user/computer names.</p></section>
        {summary && <section className="card"><h3>Support summary</h3><textarea className="summary" readOnly value={summary} rows={14} aria-label="Support summary" onFocus={(e) => e.currentTarget.select()} /></section>}
      </>)}
    </div>
  );
}
