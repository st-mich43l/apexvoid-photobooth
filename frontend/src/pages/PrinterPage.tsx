import { useEffect, useState } from "react";
import { api, on } from "../api";
import { jobActive, jobKind, jobLabel, paperLabel, pickPaper } from "../printInfo";
import { PrintPanel } from "../PrintPanel";
import { useApp } from "../state";
import { Badge, Confirm, ErrorNote, useAction } from "../ui";
import { when } from "../format";
import type { Media, PrintJob, PrinterCaps, PrintersResult } from "../types";

/** Printer page: installed printers, defaults, test print, preview + print, queue and reprint history. */
export function PrinterPage() {
  const { settings, setSettings, mediaUrl, libraryVersion, toast } = useApp();
  const [pr, setPr] = useState<PrintersResult | null>(null);
  const [caps, setCaps] = useState<PrinterCaps | null>(null);
  const [jobs, setJobs] = useState<PrintJob[]>([]);
  const [recent, setRecent] = useState<Media[]>([]);
  const [pick, setPick] = useState("");
  const [confirmTest, setConfirmTest] = useState(false);
  const [confirmReprint, setConfirmReprint] = useState<PrintJob | null>(null);
  const load = useAction();
  const act = useAction();

  const refreshPrinters = () => load.run(async () => setPr(await api.listPrinters()));
  useEffect(() => { refreshPrinters(); api.printJobs().then(setJobs).catch(() => {}); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => on<PrintJob>("print:job", (j) => setJobs((cur) => [j, ...cur.filter((x) => x.id !== j.id)].sort((a, b) => b.createdAt.localeCompare(a.createdAt)))), []);
  useEffect(() => {
    api.listMedia("", 60, 0).then((m) => { const c = m.filter((x) => x.kind !== "video"); setRecent(c.slice(0, 16)); setPick((p) => p || c.find((x) => x.kind === "strip" || x.kind === "print")?.id || c[0]?.id || ""); }).catch(() => {});
  }, [libraryVersion]);
  useEffect(() => { const t = setInterval(() => api.printJobs().then(setJobs).catch(() => {}), 4000); return () => clearInterval(t); }, []);

  const printer = settings.printerName || pr?.default || "";
  useEffect(() => { if (printer) api.printerCapabilities(printer).then(setCaps).catch(() => setCaps(null)); }, [printer]);

  const save = (patch: Partial<typeof settings>) => act.run(async () => {
    const n = { ...settings, ...patch }; await api.saveSettings(n); setSettings(n); toast("Saved");
  });
  const cur = pr?.printers.find((p) => p.name === printer);
  const paper = pickPaper(caps?.papers ?? [], settings.printPaper);

  return (
    <div className="page">
      <header className="row"><h1>Printer</h1><button onClick={refreshPrinters} disabled={load.busy}>{load.busy ? "Scanning…" : "Refresh printers"}</button></header>
      <ErrorNote error={load.error ?? act.error} onClose={() => { load.clear(); act.clear(); }} />
      <div className="note warn"><div>{pr?.note}</div></div>

      <div className="two-col">
        <section className="card">
          <h3>Installed printers <span className="muted small">{pr?.driver}</span></h3>
          {pr && pr.printers.length === 0 && <p className="muted">No printers are installed. Add one in Windows Settings → Printers &amp; scanners, then press Refresh.</p>}
          <ul className="list">
            {pr?.printers.map((p) => (
              <li key={p.name}>
                <div>
                  <strong>{p.name}</strong>{" "}
                  <Badge kind={p.status === "ready" ? "ok" : p.status === "offline" ? "warn" : "err"}>{p.status}</Badge>
                  {p.default && <Badge kind="info">Windows default</Badge>}
                  {printer === p.name && <Badge kind="info">used by the booth</Badge>}
                  {p.detail && <div className="muted small">{p.detail}</div>}
                  {p.driver && <div className="muted small">{p.driver}{p.port ? ` · ${p.port}` : ""}</div>}
                </div>
                <button disabled={printer === p.name || act.busy} onClick={() => save({ printerName: p.name })}>Use this printer</button>
              </li>
            ))}
          </ul>
        </section>

        <section className="card">
          <h3>Defaults{printer ? ` — ${printer}` : ""}</h3>
          {!printer && <p className="muted">Choose a printer first.</p>}
          {printer && (<>
            <div className="form">
              <label>Default paper size
                <select value={paper} onChange={(e) => save({ printPaper: e.target.value })} disabled={!caps}>
                  {caps?.papers.map((p) => <option key={`${p.id}-${p.name}`} value={p.name}>{paperLabel(p)}</option>)}
                </select>
              </label>
            </div>
            {caps && <p className="muted small">{caps.papers.length} paper sizes · orientations: {caps.orientations.join(", ")} · up to {caps.maxCopies} copies · {caps.color ? "colour" : "monochrome"}
              {caps.resolutions && caps.resolutions.length > 0 ? ` · ${caps.resolutions.map((r) => `${r[0]}×${r[1]} dpi`).join(", ")}` : ""}{caps.note ? ` · ${caps.note}` : ""}</p>}
            <button disabled={!cur || cur.status !== "ready" || act.busy} onClick={() => setConfirmTest(true)}>Test print…</button>
            <p className="muted small">The test page has a 10 mm grid, 25 mm corner squares and the paper size, so margins and scale can be checked on paper.</p>
          </>)}
        </section>
      </div>

      <section className="card">
        <h3>Preview and print</h3>
        {recent.length === 0 ? <p className="muted">Take a photo session first, then print its strip from here or from the Gallery.</p> : (<>
          <div className="strip-row">
            {recent.map((m) => (
              <button key={m.id} className={`thumb-btn ${m.kind} ${pick === m.id ? "picked" : ""}`} onClick={() => setPick(m.id)} aria-label={`${m.kind} ${m.id}`}>
                {m.thumbRel ? <img src={mediaUrl(m.id, "thumb", m.editedRel || m.thumbRel)} alt="" /> : <div className="no-thumb">{m.kind}</div>}
                <span className="tag">{m.kind}</span>
              </button>
            ))}
          </div>
          {pick && <PrintPanel key={pick} mediaId={pick} />}
        </>)}
      </section>

      <section className="card">
        <h3>Print queue and history</h3>
        {jobs.length === 0 ? <p className="muted">No print jobs yet.</p> : (
          <table className="table jobs"><tbody>
            {jobs.map((j) => (
              <tr key={j.id}>
                <td><strong>{j.kind === "test" ? "Test page" : j.mediaId.slice(0, 8)}</strong>{j.parentId && <span className="muted small"> · reprint #{j.attempt - 1}</span>}
                  <div className="muted small">{when(j.createdAt)}</div></td>
                <td>{j.printer}<div className="muted small">{j.paperName} · {j.copies}× · {j.orientation}/{j.fit}</div></td>
                <td><Badge kind={jobKind(j.status)}>{jobLabel[j.status]}</Badge>{jobActive(j.status) && <span className="spinner tiny" />}
                  <div className="muted small">{j.detail}</div></td>
                <td>{j.kind === "print" && !jobActive(j.status) && <button className="ghost" disabled={act.busy} onClick={() => setConfirmReprint(j)}>Reprint…</button>}</td>
              </tr>
            ))}
          </tbody></table>
        )}
      </section>

      {confirmTest && <Confirm title="Print a test page?" confirmLabel="Print test page" onNo={() => setConfirmTest(false)}
        body={<p>One calibration page will be sent to <strong>{printer}</strong> on <strong>{paper}</strong>. It uses paper (and ink or ribbon).</p>}
        onYes={() => { setConfirmTest(false); act.run(async () => { const j = await api.testPrint(printer, paper, "portrait"); toast(`Test page sent (${j.id})`); setJobs(await api.printJobs()); }); }} />}
      {confirmReprint && <Confirm title="Print this again?" confirmLabel="Reprint" onNo={() => setConfirmReprint(null)}
        body={<p>This sends the same finished image to <strong>{confirmReprint.printer}</strong> again ({confirmReprint.copies} {confirmReprint.copies > 1 ? "copies" : "copy"} on {confirmReprint.paperName}). Check the output tray first: the spooler cannot tell whether the earlier print came out.</p>}
        onYes={() => { const j = confirmReprint; setConfirmReprint(null); act.run(async () => { const r = await api.reprint(j.id, true); toast(`Reprint sent (${r.id})`); setJobs(await api.printJobs()); }); }} />}
    </div>
  );
}
