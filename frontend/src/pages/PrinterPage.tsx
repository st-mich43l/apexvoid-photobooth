import { translate } from "../i18n";
import { useEffect, useState } from "react";
import { api, on } from "../api";
import { jobActive, jobKind, jobLabel, paperLabel } from "../printInfo";
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
  const t = (key: Parameters<typeof translate>[1]) => translate(settings?.language, key);
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
  useEffect(() => {
    let active = true;
    setCaps(null);
    if (printer) api.printerCapabilities(printer).then((c) => { if (active) setCaps(c); }).catch(() => { if (active) setCaps(null); });
    return () => { active = false; };
  }, [printer]);

  const save = (patch: Partial<typeof settings>) => act.run(async () => {
    const r = await api.patchSettings(patch); setSettings(r.settings ?? { ...settings, ...patch }); toast("Saved");
  });
  const cur = pr?.printers.find((p) => p.name === printer);
  const lastTest = jobs.find((j) => j.kind === "test");
  const paper = caps?.papers.some((p) => p.name === settings.printPaper) ? settings.printPaper : "";

  return (
    <div className="page">
      <PageHeader title={translate(settings?.language, "printer")} sub={t("printerSub")}>
        <button onClick={refreshPrinters} disabled={load.busy}>{load.busy ? t("scanning") : t("refreshPrinters")}</button>
      </PageHeader>
      <ErrorNote error={load.error ?? act.error} onClose={() => { load.clear(); act.clear(); }} />
      <section className="card print-mode" aria-label="Printing mode">
        <h3>{translate(settings?.language, "printingMode")}</h3>
        <div className="mode-choice" role="radiogroup" aria-label="Printing mode">
          <button role="radio" aria-checked={digital} className={digital ? "on" : ""} disabled={act.busy} onClick={() => { if (!digital) save({ printerName: "" }); }}>
            <strong>{t("digitalOnly")}</strong><span>{t("digitalHint")}</span></button>
          <button role="radio" aria-checked={!digital} className={!digital ? "on" : ""} disabled={act.busy}
            onClick={() => { if (digital) { if (pr?.default) save({ printerName: pr.default }); else toast("Choose a printer from the list below."); } }}>
            <strong>{t("printWithPrinter")}</strong><span>{digital ? t("choosePrinterBelow") : t("on")}</span></button>
        </div>
        <dl className="facts">
          <Fact label={t("printer")}>{printer || <span className="muted">{t("noPrinter")}</span>}</Fact>
          {!digital && <>
            <Fact label="Windows reports">{cur ? <StatusPill view={cur.status === "ready" ? "unverified" : "unavailable"} label={cur.status === "ready" ? t("notVerified") : cur.status} /> : <span className="warn-text">{t("printerNotFound")}</span>}</Fact>
            <Fact label={t("paperSize")}>{settings.printPaper || <span className="muted">not set</span>}</Fact>
            <Fact label={t("testPrint")}>{lastTest ? <>{lastTest.detail || jobLabel[lastTest.status]} <span className="muted small">{when(lastTest.createdAt)}</span></> : <span className="muted">{t("testNotRequested")}</span>}</Fact>
          </>}
        </dl>
        <p className="muted small">{digital ? "Pick a printer below to offer prints." : !cur ? "This printer is not installed any more. Photos are still saved digitally; choose another printer, or switch to Digital only." : cur.status !== "ready" ? "Windows reports the printer is not ready. Guests can still take photos — they are saved digitally. Check power, paper and the cable, then Refresh printers." : "Windows says the printer is ready; that does not prove paper will come out. Send a test print to check the tray."}</p>
        {!digital && <div className="row"><TestPrint printer={printer} paper={paper} disabled={!cur || cur.status !== "ready" || !paper} onSent={() => { void api.printJobs().then(setJobs); }} /></div>}
      </section>
      <div className="note warn"><div>{pr?.note}</div></div>

      <div className="two-col">
        <section className="card">
          <h3>{translate(settings?.language, "installedPrinters")} <span className="muted small">{pr?.driver}</span></h3>
          {pr && pr.printers.length === 0 && <p className="muted">{t("noPrinters")}</p>}
          <ul className="list">
            {pr?.printers.map((p) => (
              <li key={p.name}>
                <div>
                  <strong>{p.name}</strong>{" "}
                  <Badge kind={p.status === "ready" ? "ok" : p.status === "offline" ? "warn" : "err"}>{p.status}</Badge>
                  {p.default && <Badge kind="info">{t("windowsDefault")}</Badge>}
                  {printer === p.name && <Badge kind="info">{t("usedByBooth")}</Badge>}
                  {p.detail && <div className="muted small">{p.detail}</div>}
                  {p.driver && <div className="muted small">{p.driver}{p.port ? ` · ${p.port}` : ""}</div>}
                </div>
                <button disabled={printer === p.name || act.busy} onClick={() => save({ printerName: p.name })}>{t("usePrinter")}</button>
              </li>
            ))}
          </ul>
        </section>

        <section className="card">
          <h3>Defaults{printer ? ` — ${printer}` : ""}</h3>
          {!printer && <p className="muted">{t("choosePrinterFirst")}</p>}
          {printer && (<>
            <div className="form">
              <label>Default paper size
                <select value={paper} onChange={(e) => save({ printPaper: e.target.value })} disabled={!caps}>
                  <option value="">{t("choosePaper")}</option>
                  {caps?.papers.map((p) => <option key={`${p.id}-${p.name}`} value={p.name}>{paperLabel(p)}</option>)}
                </select>
              </label>
              <label>Default orientation
                <select value={settings.printOrientation || "auto"} disabled={!caps} onChange={(e) => save({ printOrientation: e.target.value })}>
                  <option value="auto">{t("automatic")}</option>
                  {caps?.orientations.map((o) => <option key={o} value={o}>{o}</option>)}
                </select>
              </label>
              <label>Default placement
                <select value={settings.printFit || "fit"} onChange={(e) => save({ printFit: e.target.value })}>
                  <option value="fit">{t("fitArea")}</option><option value="fill">{t("fillSheet")}</option><option value="actual">{t("trueSize")}</option>
                </select>
              </label>
              <label>Default copies
                <input type="number" min={1} max={Math.min(10, caps?.maxCopies || 1)} value={settings.printCopies || 1} disabled={!caps}
                  onChange={(e) => { const n = Number(e.target.value); if (n >= 1 && n <= Math.min(10, caps?.maxCopies || 1)) save({ printCopies: n }); }} />
              </label>
              <label>Two-up strips
                <select value={settings.printTwoUp || "auto"} onChange={(e) => save({ printTwoUp: e.target.value })}>
                  <option value="auto">{t("twoStrips")}</option><option value="off">{t("singleStrip")}</option>
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
      <summary>{t("advancedPrint")}</summary>
      <section className="card">
        <h3>{translate(settings?.language, "previewPrint")}</h3>
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
        <h3>{translate(settings?.language, "printHistory")}</h3>
        {jobs.length === 0 ? <p className="muted">{t("noJobsYet")}</p> : (
          <table className="table jobs"><tbody>
            {jobs.map((j) => (
              <tr key={j.id}>
                <td><strong>{j.kind === "test" ? "Test page" : j.mediaId.slice(0, 8)}</strong>{j.parentId && <span className="muted small"> · reprint #{j.attempt - 1}</span>}
                  <div className="muted small">{when(j.createdAt)}</div></td>
                <td>{j.printer}<div className="muted small">{j.paperName} · {j.copies}× · {j.orientation}/{j.fit}</div></td>
                <td><Badge kind={jobKind(j.status)}>{jobLabel[j.status]}</Badge>{jobActive(j.status) && <span className="spinner tiny" />}
                  <div className="muted small">{j.detail}</div>{j.status === "unknown" && <div className="muted small">{t("unknownPrintHelp")}</div>}</td>
                <td>{j.kind === "print" && !jobActive(j.status) && j.status !== "unknown" && <button className="ghost" disabled={act.busy} onClick={() => setConfirmReprint(j)}>{t("reprint")}</button>}</td>
              </tr>
            ))}
          </tbody></table>
        )}
      </section>

      {confirmReprint && <Confirm title="Print this again?" confirmLabel="Reprint" onNo={() => setConfirmReprint(null)}
        body={<div>{recent.some((m) => m.id === (confirmReprint.renderedMediaId || confirmReprint.mediaId)) && <img style={{ maxHeight: 180, maxWidth: "100%" }} src={mediaUrl(confirmReprint.renderedMediaId || confirmReprint.mediaId, "thumb", "")} alt="Image selected for reprint" />}<p>This sends the same finished image to <strong>{confirmReprint.printer}</strong> again ({confirmReprint.copies} {confirmReprint.copies > 1 ? "copies" : "copy"} on {confirmReprint.paperName}). Previous state: {jobLabel[confirmReprint.status]}. Check the output tray first: the spooler cannot tell whether the earlier print came out.</p></div>}
        onYes={() => { const j = confirmReprint; setConfirmReprint(null); act.run(async () => { const r = await api.reprint(j.id, true); toast(`Reprint sent (${r.id})`); setJobs(await api.printJobs()); }); }} />}
    </div>
  );
}
