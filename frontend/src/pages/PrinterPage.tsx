import { useEffect, useState } from "react";
import { api, on } from "../api";
import { jobActive, jobKind, paperLabel, printerStatusLabel } from "../printInfo";
import { PrintPanel } from "../PrintPanel";
import { useApp, useT } from "../state";
import { Badge, Confirm, ErrorNote, useAction } from "../ui";
import { Fact, PageHeader, StatusPill } from "../operator/parts";
import { TestPrint } from "../operator/TestPrint";
import { when } from "../format";
import type { Media, PrintJob, PrinterCaps, PrintersResult } from "../types";

const jobStatusKey = { submitting: "jobStatusSubmitting", queued: "jobStatusQueued", printing: "jobStatusPrinting", completed: "jobStatusCompleted", removed: "jobStatusRemoved", failed: "jobStatusFailed", unknown: "jobStatusUnknown" } as const;

/** Printer page: installed printers, defaults, test print, preview + print, queue and reprint history. */
export function PrinterPage() {
  const { settings, setSettings, mediaUrl, libraryVersion, toast } = useApp();
  const { t, tf } = useT();
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
    const r = await api.patchSettings(patch); setSettings(r.settings ?? { ...settings, ...patch }); toast(t("printerSaved"));
  });
  const cur = pr?.printers.find((p) => p.name === printer);
  const lastTest = jobs.find((j) => j.kind === "test");
  const paper = caps?.papers.some((p) => p.name === settings.printPaper) ? settings.printPaper : "";

  return (
    <div className="page">
      <PageHeader title={t("printer")} sub={t("printerSub")}>
        <button onClick={refreshPrinters} disabled={load.busy}>{load.busy ? t("scanning") : t("refreshPrinters")}</button>
      </PageHeader>
      <ErrorNote error={load.error ?? act.error} onClose={() => { load.clear(); act.clear(); }} />
      <section className="card print-mode" aria-label={t("printingMode")}>
        <h3>{t("printingMode")}</h3>
        <div className="mode-choice" role="radiogroup" aria-label={t("printingMode")}>
          <button role="radio" aria-checked={digital} className={digital ? "on" : ""} disabled={act.busy} onClick={() => { if (!digital) save({ printerName: "" }); }}>
            <strong>{t("digitalOnly")}</strong><span>{t("digitalHint")}</span></button>
          <button role="radio" aria-checked={!digital} className={!digital ? "on" : ""} disabled={act.busy}
            onClick={() => { if (digital) { if (pr?.default) save({ printerName: pr.default }); else toast(t("choosePrinterList")); } }}>
            <strong>{t("printWithPrinter")}</strong><span>{digital ? t("choosePrinterBelow") : t("on")}</span></button>
        </div>
        <dl className="facts">
          <Fact label={t("printer")}>{printer || <span className="muted">{t("noPrinter")}</span>}</Fact>
          {!digital && <>
            <Fact label={t("printerWindowsReports")}>{cur ? <StatusPill view={cur.status === "ready" ? "unverified" : "unavailable"} label={cur.status === "ready" ? t("notVerified") : printerStatusLabel(cur.status, settings.language)} /> : <span className="warn-text">{t("printerNotFound")}</span>}</Fact>
            <Fact label={t("paperSize")}>{settings.printPaper || <span className="muted">{t("paperNotSet")}</span>}</Fact>
            <Fact label={t("testPrint")}>{lastTest ? <>{lastTest.detail || t(jobStatusKey[lastTest.status])} <span className="muted small">{when(lastTest.createdAt, settings.language)}</span></> : <span className="muted">{t("testNotRequested")}</span>}</Fact>
          </>}
        </dl>
        <p className="muted small">{digital ? t("digitalPickPrinter") : !cur ? t("printerRemovedDigital") : cur.status !== "ready" ? t("printerNotReadyDigital") : t("printerReadyCaveat")}</p>
        {!digital && <div className="row"><TestPrint printer={printer} paper={paper} disabled={!cur || cur.status !== "ready" || !paper} onSent={() => { void api.printJobs().then(setJobs); }} /></div>}
      </section>
      <div className="note warn"><div>{pr?.note}</div></div>

      <div className="two-col">
        <section className="card">
          <h3>{t("installedPrinters")} <span className="muted small">{pr?.driver}</span></h3>
          {pr && pr.printers.length === 0 && <p className="muted">{t("noPrinters")}</p>}
          <ul className="list">
            {pr?.printers.map((p) => (
              <li key={p.name}>
                <div>
                  <strong>{p.name}</strong>{" "}
                  <Badge kind={p.status === "ready" ? "ok" : p.status === "offline" ? "warn" : "err"}>{printerStatusLabel(p.status, settings.language)}</Badge>
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
          <h3>{t("printerDefaults")}{printer ? ` — ${printer}` : ""}</h3>
          {!printer && <p className="muted">{t("choosePrinterFirst")}</p>}
          {printer && (<>
            <div className="form">
              <label>{t("defaultPaperSize")}
                <select value={paper} onChange={(e) => save({ printPaper: e.target.value })} disabled={!caps}>
                  <option value="">{t("choosePaper")}</option>
                  {caps?.papers.map((p) => <option key={`${p.id}-${p.name}`} value={p.name}>{paperLabel(p)}</option>)}
                </select>
              </label>
              <label>{t("defaultOrientation")}
                <select value={settings.printOrientation || "auto"} disabled={!caps} onChange={(e) => save({ printOrientation: e.target.value })}>
                  <option value="auto">{t("automatic")}</option>
                  {caps?.orientations.map((o) => <option key={o} value={o}>{o}</option>)}
                </select>
              </label>
              <label>{t("defaultPlacement")}
                <select value={settings.printFit || "fit"} onChange={(e) => save({ printFit: e.target.value })}>
                  <option value="fit">{t("fitArea")}</option><option value="fill">{t("fillSheet")}</option><option value="actual">{t("trueSize")}</option>
                </select>
              </label>
              <label>{t("defaultCopies")}
                <input type="number" min={1} max={Math.min(10, caps?.maxCopies || 1)} value={settings.printCopies || 1} disabled={!caps}
                  onChange={(e) => { const n = Number(e.target.value); if (n >= 1 && n <= Math.min(10, caps?.maxCopies || 1)) save({ printCopies: n }); }} />
              </label>
              <label>{t("twoUpStripLabel")}
                <select value={settings.printTwoUp || "auto"} onChange={(e) => save({ printTwoUp: e.target.value })}>
                  <option value="auto">{t("twoStrips")}</option><option value="off">{t("singleStrip")}</option>
                </select>
              </label>
            </div>
            {caps && <p className="muted small">{tf("printCapabilitySummary", { papers: caps.papers.length, orientations: caps.orientations.join(", "), copies: caps.maxCopies, colorMode: t(caps.color ? "colorMode" : "monochromeMode") })}
              {caps.resolutions && caps.resolutions.length > 0 ? ` · ${caps.resolutions.map((r) => `${r[0]}×${r[1]} dpi`).join(", ")}` : ""}{caps.note ? ` · ${caps.note}` : ""}</p>}
            <p className="muted small">{t("testPageMeasurementHint")}</p>
          </>)}
        </section>
      </div>

      <details className="advanced">
      <summary>{t("advancedPrint")}</summary>
      <section className="card">
        <h3>{t("previewPrint")}</h3>
        {recent.length === 0 ? <p className="muted">{t("previewTakeSessionFirst")}</p> : (<>
          <div className="strip-row">
            {recent.map((m) => (
              <button key={m.id} className={`thumb-btn ${m.kind} ${pick === m.id ? "picked" : ""}`} onClick={() => setPick(m.id)} aria-label={tf("mediaNamed", { kind: m.kind === "photo" ? t("photo").toLowerCase() : m.kind === "video" ? t("video").toLowerCase() : t("photoStrip").toLowerCase(), id: m.id })}>
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
        <h3>{t("printHistory")}</h3>
        {jobs.length === 0 ? <p className="muted">{t("noJobsYet")}</p> : (
          <table className="table jobs"><tbody>
            {jobs.map((j) => (
              <tr key={j.id}>
                <td><strong>{j.kind === "test" ? t("testPage") : j.mediaId.slice(0, 8)}</strong>{j.parentId && <span className="muted small"> · {tf("reprintNumber", { number: j.attempt - 1 })}</span>}
                  <div className="muted small">{when(j.createdAt, settings.language)}</div></td>
                <td>{j.printer}<div className="muted small">{j.paperName} · {j.copies}× · {j.orientation}/{j.fit}</div></td>
                <td><Badge kind={jobKind(j.status)}>{t(jobStatusKey[j.status])}</Badge>{jobActive(j.status) && <span className="spinner tiny" />}
                  <div className="muted small">{j.detail}</div>{j.status === "unknown" && <div className="muted small">{t("unknownPrintHelp")}</div>}</td>
                <td>{j.kind === "print" && !jobActive(j.status) && j.status !== "unknown" && <button className="ghost" disabled={act.busy} onClick={() => setConfirmReprint(j)}>{t("reprint")}</button>}</td>
              </tr>
            ))}
          </tbody></table>
        )}
      </section>

      {confirmReprint && <Confirm title={t("confirmPrintAgain")} confirmLabel={t("reprintImage")} onNo={() => setConfirmReprint(null)}
        body={<div>{recent.some((m) => m.id === (confirmReprint.renderedMediaId || confirmReprint.mediaId)) && <img style={{ maxHeight: 180, maxWidth: "100%" }} src={mediaUrl(confirmReprint.renderedMediaId || confirmReprint.mediaId, "thumb", "")} alt={t("imageSelectedReprint")} />}<p>{t("reprintSameFinishedImage")} <strong>{confirmReprint.printer}</strong> {tf("againCopiesOn", { copies: confirmReprint.copies, copyLabel: t(confirmReprint.copies > 1 ? "copyPlural" : "copySingular"), paper: confirmReprint.paperName, status: t(jobStatusKey[confirmReprint.status]) })}</p></div>}
        onYes={() => { const j = confirmReprint; setConfirmReprint(null); act.run(async () => { const r = await api.reprint(j.id, true); toast(tf("reprintSent", { id: r.id })); setJobs(await api.printJobs()); }); }} />}
    </div>
  );
}
