import { useEffect, useState } from "react";
import { api, on } from "../api";
import { jobActive, jobKind, jobLabel, paperLabel, pickPaper } from "../printInfo";
import { PrintPanel } from "../PrintPanel";
import { useApp } from "../state";
import { Badge, Confirm, ErrorNote, useAction } from "../ui";
import { Fact, PageHeader, StatusPill } from "../operator/parts";
import { TestPrint } from "../operator/TestPrint";
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

  const printer = settings.printerName; // empty = Digital only: the Windows default printer is never used silently
  const digital = !printer;
  useEffect(() => { if (printer) api.printerCapabilities(printer).then(setCaps).catch(() => setCaps(null)); }, [printer]);

  const save = (patch: Partial<typeof settings>) => act.run(async () => {
    const r = await api.patchSettings(patch); setSettings(r.settings ?? { ...settings, ...patch }); toast("Saved");
  });
  const cur = pr?.printers.find((p) => p.name === printer);
  const lastTest = jobs.find((j) => j.kind === "test");
  const paper = pickPaper(caps?.papers ?? [], settings.printPaper);

  return (
    <div className="page">
      <PageHeader title="Printer" sub="Printing is optional. Digital only works without a printer.">
        <button onClick={refreshPrinters} disabled={load.busy}>{load.busy ? "Scanning…" : "Refresh printers"}</button>
      </PageHeader>
      <ErrorNote error={load.error ?? act.error} onClose={() => { load.clear(); act.clear(); }} />
      <section className="card print-mode" aria-label="Printing mode">
        <h3>Printing mode</h3>
        <div className="mode-choice" role="radiogroup" aria-label="Printing mode">
          <button role="radio" aria-checked={digital} className={digital ? "on" : ""} disabled={act.busy} onClick={() => { if (!digital) save({ printerName: "" }); }}>
            <strong>Digital only</strong><span>Guests get digital photos. No printer needed.</span></button>
          <button role="radio" aria-checked={!digital} className={!digital ? "on" : ""} disabled={act.busy}
            onClick={() => { if (digital) { if (pr?.default) save({ printerName: pr.default }); else toast("Choose a printer from the list below."); } }}>
            <strong>Print with a printer</strong><span>{digital ? "Choose a printer below." : "On."}</span></button>
        </div>
        <dl className="facts">
          <Fact label="Printer">{printer || <span className="muted">none — digital only</span>}</Fact>
          {!digital && <>
            <Fact label="Windows reports">{cur ? <StatusPill view={cur.status === "ready" ? "unverified" : "unavailable"} label={cur.status === "ready" ? "ready (not verified)" : cur.status} /> : <span className="warn-text">printer not found</span>}</Fact>
            <Fact label="Paper size">{settings.printPaper || <span className="muted">not set</span>}</Fact>
            <Fact label="Test print">{lastTest ? <>{lastTest.detail || jobLabel[lastTest.status]} <span className="muted small">{when(lastTest.createdAt)}</span></> : <span className="muted">not requested — it is never sent automatically</span>}</Fact>
          </>}
        </dl>
        <p className="muted small">{digital ? "Pick a printer below to offer prints." : !cur ? "This printer is not installed any more. Photos are still saved digitally; choose another printer, or switch to Digital only." : cur.status !== "ready" ? "Windows reports the printer is not ready. Guests can still take photos — they are saved digitally. Check power, paper and the cable, then Refresh printers." : "Windows says the printer is ready; that does not prove paper will come out. Send a test print to check the tray."}</p>
        {!digital && <div className="row"><TestPrint printer={printer} paper={paper} disabled={!cur || cur.status !== "ready"} onSent={() => { void api.printJobs().then(setJobs); }} /></div>}
      </section>
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
            <p className="muted small">The test page has a 10 mm grid, 25 mm corner squares and the paper size, so margins and scale can be checked on paper (use “Test print…” above).</p>
          </>)}
        </section>
      </div>

      <details className="advanced">
      <summary>Advanced — preview, placement, rotation and calibration</summary>
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
      </details>

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

      {confirmReprint && <Confirm title="Print this again?" confirmLabel="Reprint" onNo={() => setConfirmReprint(null)}
        body={<p>This sends the same finished image to <strong>{confirmReprint.printer}</strong> again ({confirmReprint.copies} {confirmReprint.copies > 1 ? "copies" : "copy"} on {confirmReprint.paperName}). Check the output tray first: the spooler cannot tell whether the earlier print came out.</p>}
        onYes={() => { const j = confirmReprint; setConfirmReprint(null); act.run(async () => { const r = await api.reprint(j.id, true); toast(`Reprint sent (${r.id})`); setJobs(await api.printJobs()); }); }} />}
    </div>
  );
}
