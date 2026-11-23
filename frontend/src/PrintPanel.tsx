import { useEffect, useRef, useState } from "react";
import { api } from "./api";
import { describeMargins, newRequestId, paperLabel, pickPaper } from "./printInfo";
import { useApp } from "./state";
import { Badge, ErrorNote, useAction } from "./ui";
import type { PrintJob, PrinterCaps, PrintersResult, PrintPreview } from "./types";

/** Choose printer/paper/orientation/fit/copies, see the real placement, print. Shared by the gallery and the Printer page. */
export function PrintPanel({ mediaId, templateId, onPrinted }: { mediaId: string; templateId?: string; onPrinted?: (j: PrintJob) => void }) {
  const { settings, toast } = useApp();
  const [pr, setPr] = useState<PrintersResult | null>(null);
  const [caps, setCaps] = useState<PrinterCaps | null>(null);
  const [printer, setPrinter] = useState(settings.printerName);
  const [paper, setPaper] = useState(settings.printPaper);
  const [orientation, setOrientation] = useState(settings.printOrientation || "auto");
  const [fit, setFit] = useState(settings.printFit || "fit");
  const [copies, setCopies] = useState(settings.printCopies || 1);
  const [twoUp, setTwoUp] = useState<boolean | null>(settings.printTwoUp === "off" ? false : null); // null = follow the offer for this paper
  const [preview, setPreview] = useState<PrintPreview | null>(null);
  const [previewError, setPreviewError] = useState("");
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

  const opts = (): import("./types").PrintOptions => ({ requestId: requestId.current, mediaId, templateId: templateId ?? "", printer, paper, orientation, fit, copies, twoUp: twoUp ?? false });
  const selectionKey = JSON.stringify([mediaId, templateId, printer, paper, orientation, fit, copies, twoUp]);
  useEffect(() => {
    if (!printer || !paper || !caps || !caps.papers.some((p) => p.name === paper)) { setPreview(null); setPreviewKey(""); return; }
    let alive = true;
    setPreview(null); setPreviewKey(""); setPreviewError("");
    api.printPreview({ ...opts(), requestId: "" }).then((p) => {
      if (!alive) return;
      if (twoUp === null && p.twoUpOffer && !p.twoUp) { setTwoUp(true); return; }   // a 2×6 strip on 4×6 paper: show the two-up sheet
      setPreview(p); setPreviewKey(selectionKey);
    }).catch((e) => { if (alive) { setPreview(null); setPreviewError(e.message + (e.action ? ` ${e.action}` : "")); } });
    return () => { alive = false; };
  }, [mediaId, templateId, printer, paper, orientation, fit, copies, twoUp, caps]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { setTwoUp(settings.printTwoUp === "off" ? false : null); }, [mediaId, paper, settings.printTwoUp]);

  const p = pr?.printers.find((x) => x.name === printer);
  const unavailable = !p || p.status !== "ready";
  const currentPreview = previewKey === selectionKey ? preview : null;
  return (
    <div className="print-panel">
      <div className="note warn"><div>{pr?.note ?? "Printing goes through the Windows spooler. A job leaving the queue does not prove that paper came out."}</div></div>
      <ErrorNote error={error} onClose={clear} />
      <div className="print-grid">
        <div className="form">
          <label>Printer
            <select value={printer} onChange={(e) => setPrinter(e.target.value)}>
              <option value="">Choose…</option>
              {pr?.printers.map((x) => <option key={x.name} value={x.name}>{x.name}{x.default ? " (default)" : ""} — {x.status}</option>)}
            </select>
          </label>
          {p && unavailable && <div className="note err"><div><strong>{p.name} is {p.status}.</strong> {p.detail}</div></div>}
          {pr && printer && !p && <div className="note err"><div>The selected printer is no longer installed. Choose an available printer.</div></div>}
          <label>Paper
            <select value={paper} onChange={(e) => setPaper(e.target.value)} disabled={!caps}>
              {caps?.papers.map((x) => <option key={`${x.id}-${x.name}`} value={x.name}>{paperLabel(x)}</option>)}
            </select>
          </label>
          <label>Orientation
            <select value={orientation} onChange={(e) => setOrientation(e.target.value)}>
              <option value="auto">Automatic (follow the image)</option>
              {(caps?.orientations ?? ["portrait", "landscape"]).map((o) => <option key={o} value={o}>{o[0].toUpperCase() + o.slice(1)}</option>)}
            </select>
          </label>
          <label>Placement
            <select value={fit} onChange={(e) => setFit(e.target.value)}>
              <option value="fit">Fit inside the printable area (nothing cut off)</option>
              <option value="fill">Fill the whole sheet (edges may be cropped)</option>
              <option value="actual">True size, centred</option>
            </select>
          </label>
          {(preview?.twoUpOffer || twoUp) && (
            <label className="check"><input type="checkbox" checked={!!twoUp} onChange={(e) => setTwoUp(e.target.checked)} /> Two strips side by side on this sheet (cut it in half)</label>
          )}
          <label>Copies
            <input type="number" min={1} max={caps?.maxCopies ?? 10} value={copies} onChange={(e) => setCopies(Math.max(1, Number(e.target.value) || 1))} />
          </label>
        </div>
        <div className="print-preview">
          {currentPreview ? (
            <>
              <img src={currentPreview.image} alt="Print preview" />
              <p className="muted small">
                {copies} {copies === 1 ? "copy" : "copies"} · Sheet {currentPreview.paperMm[0].toFixed(0)}×{currentPreview.paperMm[1].toFixed(0)} mm (printable {currentPreview.printableMm[0].toFixed(0)}×{currentPreview.printableMm[1].toFixed(0)} mm) · image {currentPreview.imageInches[0].toFixed(2)}×{currentPreview.imageInches[1].toFixed(2)} in
                at {currentPreview.scalePct.toFixed(0)}% {currentPreview.rotated ? "· turned 90°" : ""} {currentPreview.croppedPct > 0.5 ? `· ${currentPreview.croppedPct.toFixed(0)}% cropped` : ""}
              </p>
              <p className="muted small">White space on the sheet: {describeMargins(currentPreview.marginMm)}.</p>
              {currentPreview.unprintableMm && <p className="muted small">Unprintable margins: {describeMargins(currentPreview.unprintableMm)}.</p>}
              {currentPreview.assumedGeometry && <p className="muted small"><Badge kind="warn">estimated</Badge> The driver's real printable area is not available here.</p>}
              {currentPreview.warning && <div className="note warn"><div>{currentPreview.warning}</div></div>}
              <p className="muted small">Grey = area this printer cannot print. The file that is printed is the finished layout, never the raw camera original.</p>
            </>
          ) : <p className="muted">{previewError || (printer ? "Preparing preview…" : "Choose a printer to see the preview.")}</p>}
        </div>
      </div>
      <div className="row">
        <span className="muted small">Print job id: unique per click, so a double tap prints once.</span>
        <button className="primary big" disabled={busy || !printer || !paper || unavailable || !currentPreview} onClick={() => run(async () => {
          const j = await api.printMedia(opts());
          requestId.current = newRequestId();                      // the next click is a new action
          toast(`Print job ${j.id}: ${j.status}`); onPrinted?.(j);
        })}>{busy ? "Sending…" : `Print ${copies > 1 ? `${copies} copies` : ""}`}</button>
      </div>
    </div>
  );
}
