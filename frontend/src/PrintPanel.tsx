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
  const [orientation, setOrientation] = useState("auto");
  const [fit, setFit] = useState("fit");
  const [copies, setCopies] = useState(1);
  const [twoUp, setTwoUp] = useState<boolean | null>(null); // null = follow the offer for this paper
  const [preview, setPreview] = useState<PrintPreview | null>(null);
  const [previewError, setPreviewError] = useState("");
  const { busy, error, run, clear } = useAction();
  // one id per user action: a double tap or a retry can never print twice
  const requestId = useRef(newRequestId());

  useEffect(() => {
    api.listPrinters().then((r) => { setPr(r); setPrinter((p) => p || r.default || r.printers.find((x) => x.status === "ready")?.name || ""); }).catch(() => {});
  }, []);
  useEffect(() => {
    if (!printer) { setCaps(null); return; }
    api.printerCapabilities(printer).then((c) => { setCaps(c); setPaper((p) => pickPaper(c.papers, p)); }).catch(() => setCaps(null));
  }, [printer]);

  const opts = (): import("./types").PrintOptions => ({ requestId: requestId.current, mediaId, templateId: templateId ?? "", printer, paper, orientation, fit, copies, twoUp: twoUp ?? false });
  useEffect(() => {
    if (!printer || !paper) { setPreview(null); return; }
    let alive = true;
    setPreviewError("");
    api.printPreview({ ...opts(), requestId: "" }).then((p) => {
      if (!alive) return;
      if (twoUp === null && p.twoUpOffer && !p.twoUp) { setTwoUp(true); return; }   // a 2×6 strip on 4×6 paper: show the two-up sheet
      setPreview(p);
    }).catch((e) => { if (alive) { setPreview(null); setPreviewError(e.message + (e.action ? ` ${e.action}` : "")); } });
    return () => { alive = false; };
  }, [mediaId, templateId, printer, paper, orientation, fit, twoUp]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => { setTwoUp(null); }, [mediaId, paper]);

  const p = pr?.printers.find((x) => x.name === printer);
  const unavailable = p && p.status !== "ready";
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
          {unavailable && <div className="note err"><div><strong>{p!.name} is {p!.status}.</strong> {p!.detail}</div></div>}
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
          {preview ? (
            <>
              <img src={preview.image} alt="Print preview" />
              <p className="muted small">
                Sheet {preview.paperMm[0].toFixed(0)}×{preview.paperMm[1].toFixed(0)} mm (printable {preview.printableMm[0].toFixed(0)}×{preview.printableMm[1].toFixed(0)} mm) · image {preview.imageInches[0].toFixed(2)}×{preview.imageInches[1].toFixed(2)} in
                at {preview.scalePct.toFixed(0)}% {preview.rotated ? "· turned 90°" : ""} {preview.croppedPct > 0.5 ? `· ${preview.croppedPct.toFixed(0)}% cropped` : ""}
              </p>
              <p className="muted small">White space on the sheet: {describeMargins(preview.marginMm)}.</p>
              {preview.assumedGeometry && <p className="muted small"><Badge kind="warn">estimated</Badge> The driver's real printable area is not available here.</p>}
              {preview.warning && <div className="note warn"><div>{preview.warning}</div></div>}
              <p className="muted small">Grey = area this printer cannot print. The file that is printed is the finished layout, never the raw camera original.</p>
            </>
          ) : <p className="muted">{previewError || (printer ? "Preparing preview…" : "Choose a printer to see the preview.")}</p>}
        </div>
      </div>
      <div className="row">
        <span className="muted small">Print job id: unique per click, so a double tap prints once.</span>
        <button className="primary big" disabled={busy || !printer || !paper || !!unavailable || !preview} onClick={() => run(async () => {
          const j = await api.printMedia(opts());
          requestId.current = newRequestId();                      // the next click is a new action
          toast(`Print job ${j.id}: ${j.status}`); onPrinted?.(j);
        })}>{busy ? "Sending…" : `Print ${copies > 1 ? `${copies} copies` : ""}`}</button>
      </div>
    </div>
  );
}
