import { useEffect, useRef, useState } from "react";
import { api, toAppError, type AppError } from "../../shared/api";
import { describeMargins, newRequestId, paperLabel, pickPaper, printerStatusLabel } from "../../shared/printInfo";
import { formatTranslation, translate } from "../../shared/i18n";
import { useLocale } from "../../shared/locale";
import { useApp } from "../../app/state";
import { Badge, ErrorNote, LocalizedDiagnostic, useAction } from "../../shared/ui";
import type { PrintJob, PrinterCaps, PrintersResult, PrintPreview } from "../../shared/types";

/** Choose printer/paper/orientation/fit/copies, see the real placement, print. Shared by the gallery and the Printer page. */
export function PrintPanel({ mediaId, templateId, onPrinted }: { mediaId: string; templateId?: string; onPrinted?: (j: PrintJob) => void }) {
  const { settings, toast } = useApp();
  const language = useLocale(settings?.language);
  const t = (key: Parameters<typeof translate>[1]) => translate(language, key);
  const tf = (key: Parameters<typeof translate>[1], values: Record<string, string | number>) => formatTranslation(language, key, values);
  const [pr, setPr] = useState<PrintersResult | null>(null);
  const [caps, setCaps] = useState<PrinterCaps | null>(null);
  const [printer, setPrinter] = useState(settings.printerName);
  const [paper, setPaper] = useState(settings.printPaper);
  const [orientation, setOrientation] = useState(settings.printOrientation || "auto");
  const [fit, setFit] = useState(settings.printFit || "fit");
  const [copies, setCopies] = useState(settings.printCopies || 1);
  const [twoUp, setTwoUp] = useState<boolean | null>(settings.printTwoUp === "off" ? false : null); // null = follow the offer for this paper
  const [preview, setPreview] = useState<PrintPreview | null>(null);
  const [previewError, setPreviewError] = useState<AppError | null>(null);
  const [previewKey, setPreviewKey] = useState("");
  const { busy, error, run, clear } = useAction();
  // one id per user action: a double tap or a retry can never print twice
  const requestId = useRef(newRequestId());

  useEffect(() => {
    api.listPrinters().then((r) => { setPr(r); setPrinter((p) => p || r.default || r.printers.find((x) => x.status === "ready")?.name || ""); }).catch(() => {});
  }, []);
  useEffect(() => {
    let active = true;
    setCaps(null); setPreview(null); setPreviewKey("");
    if (!printer) { setPaper(""); return () => { active = false; }; }
    api.printerCapabilities(printer).then((c) => {
      if (!active) return;
      setCaps(c); setPaper((p) => pickPaper(c.papers, p));
      setOrientation((o) => o === "auto" || c.orientations.includes(o) ? o : "auto");
      setCopies((n) => Math.min(n, Math.max(1, c.maxCopies)));
    }).catch(() => { if (active) { setCaps(null); setPaper(""); } });
    return () => { active = false; };
  }, [printer]);

  const opts = (): import("../../shared/types").PrintOptions => ({ requestId: requestId.current, mediaId, templateId: templateId ?? "", printer, paper, orientation, fit, copies, twoUp: twoUp ?? false });
  const selectionKey = JSON.stringify([mediaId, templateId, printer, paper, orientation, fit, copies, twoUp]);
  useEffect(() => {
    if (!printer || !paper || !caps || !caps.papers.some((p) => p.name === paper)) { setPreview(null); setPreviewKey(""); return; }
    let alive = true;
    setPreview(null); setPreviewKey(""); setPreviewError(null);
    api.printPreview({ ...opts(), requestId: "" }).then((p) => {
      if (!alive) return;
      if (twoUp === null && p.twoUpOffer && !p.twoUp) { setTwoUp(true); return; }   // a 2×6 strip on 4×6 paper: show the two-up sheet
      setPreview(p); setPreviewKey(selectionKey);
    }).catch((e) => { if (alive) { setPreview(null); setPreviewError(toAppError(e)); } });
    return () => { alive = false; };
  }, [mediaId, templateId, printer, paper, orientation, fit, copies, twoUp, caps]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { setTwoUp(settings.printTwoUp === "off" ? false : null); }, [mediaId, paper, settings.printTwoUp]);

  const p = pr?.printers.find((x) => x.name === printer);
  const unavailable = !p || p.status !== "ready";
  const currentPreview = previewKey === selectionKey ? preview : null;
  return (
    <div className="print-panel">
      <div className="note warn"><div>{pr?.note && language === "en" ? pr.note : t("spoolerNotice")}</div></div>
      <ErrorNote error={error} onClose={clear} />
      <div className="print-grid">
        <div className="form">
          <label>{t("printer")}
            <select value={printer} onChange={(e) => setPrinter(e.target.value)}>
              <option value="">{t("printChoose")}</option>
              {pr?.printers.map((x) => <option key={x.name} value={x.name}>{x.name}{x.default ? ` (${t("printDefaultSuffix")})` : ""} — {printerStatusLabel(x.status, language)}</option>)}
            </select>
          </label>
          {p && unavailable && <div className="note err"><div><strong>{tf("printStatus", { name: p.name, status: printerStatusLabel(p.status, language) })}</strong> {p.detail}</div></div>}
          {pr && printer && !p && <div className="note err"><div>{t("printPrinterMissing")}</div></div>}
          <label>{t("printPaper")}
            <select value={paper} onChange={(e) => setPaper(e.target.value)} disabled={!caps}>
              {caps?.papers.map((x) => <option key={`${x.id}-${x.name}`} value={x.name}>{paperLabel(x)}</option>)}
            </select>
          </label>
          <label>{t("printOrientation")}
            <select value={orientation} onChange={(e) => setOrientation(e.target.value)}>
              <option value="auto">{t("printAutoOrientation")}</option>
              {(caps?.orientations ?? ["portrait", "landscape"]).map((o) => <option key={o} value={o}>{o === "portrait" ? t("printPortrait") : o === "landscape" ? t("printLandscape") : o}</option>)}
            </select>
          </label>
          <label>{t("printPlacement")}
            <select value={fit} onChange={(e) => setFit(e.target.value)}>
              <option value="fit">{t("printFit")}</option>
              <option value="fill">{t("printFill")}</option>
              <option value="actual">{t("printActual")}</option>
            </select>
          </label>
          {(preview?.twoUpOffer || twoUp) && (
            <label className="check"><input type="checkbox" checked={!!twoUp} onChange={(e) => setTwoUp(e.target.checked)} /> {t("printTwoStrips")}</label>
          )}
          <label>{t("printCopies")}
            <input type="number" min={1} max={caps?.maxCopies ?? 10} value={copies} onChange={(e) => setCopies(Math.max(1, Number(e.target.value) || 1))} />
          </label>
        </div>
        <div className="print-preview">
          {currentPreview ? (
            <>
              <img src={currentPreview.image} alt={t("printPreviewAlt")} />
              <p className="muted small">
                {tf("printSummary", { copies: copies === 1 ? t("printCopyOne") : tf("printCopyMany", { count: copies }), sheet: currentPreview.paperMm.map(n => n.toFixed(0)).join("×"), printable: currentPreview.printableMm.map(n => n.toFixed(0)).join("×"), image: currentPreview.imageInches.map(n => n.toFixed(2)).join("×"), scale: currentPreview.scalePct.toFixed(0) })}
                {currentPreview.rotated ? ` · ${t("printRotated")}` : ""} {currentPreview.croppedPct > 0.5 ? `· ${tf("printCropped", { percent: currentPreview.croppedPct.toFixed(0) })}` : ""}
              </p>
              <p className="muted small">{tf("printWhiteSpace", { margins: describeMargins(currentPreview.marginMm, language) })}</p>
              {currentPreview.unprintableMm && <p className="muted small">{tf("printUnprintable", { margins: describeMargins(currentPreview.unprintableMm, language) })}</p>}
              {currentPreview.assumedGeometry && <p className="muted small"><Badge kind="warn">{t("printEstimated")}</Badge> {t("printGeometryMissing")}</p>}
              {currentPreview.warning && <div className="note warn"><div><LocalizedDiagnostic text={currentPreview.warning} summary="reviewPrintWarning" /></div></div>}
              <p className="muted small">{t("printPreviewHelp")}</p>
            </>
          ) : previewError ? <ErrorNote error={previewError} /> : <p className="muted">{printer ? t("printPreparing") : t("printChoosePreview")}</p>}
        </div>
      </div>
      <div className="row">
        <span className="muted small">{t("printJobHint")}</span>
        <button className="primary big" disabled={busy || !printer || !paper || unavailable || !currentPreview} onClick={() => run(async () => {
          const j = await api.printMedia(opts());
          requestId.current = newRequestId();                      // the next click is a new action
          toast(tf("printJobStatus", { id: j.id, status: t(({ submitting: "jobStatusSubmitting", queued: "jobStatusQueued", printing: "jobStatusPrinting", completed: "jobStatusCompleted", removed: "jobStatusRemoved", failed: "jobStatusFailed", unknown: "jobStatusUnknown" } as const)[j.status]) })); onPrinted?.(j);
        })}>{busy ? t("sending") : copies > 1 ? tf("printSubmitMany", { count: copies }) : t("print")}</button>
      </div>
    </div>
  );
}
