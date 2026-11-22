import { usePatchSettings } from "./usePatchSettings";
import { useApp } from "../state";
import { ErrorNote } from "../ui";

/** The three settings operators change most. Each change is a PatchSettings of just that key. */
export function QuickSettings() {
  const { settings } = useApp();
  const { patch, busy, error, clear } = usePatchSettings();
  const cd = settings.countdownSeconds;
  return (
    <section className="card quick" aria-label="Quick settings">
      <h3>Quick settings</h3>
      <ErrorNote error={error} onClose={clear} />
      <div className="quick-row">
        <div className="stepper" role="group" aria-label="Countdown">
          <span className="lbl">Countdown</span>
          <button aria-label="Shorter countdown" disabled={busy || cd <= 1} onClick={() => patch({ countdownSeconds: cd - 1 }, `Countdown ${cd - 1} s`)}>−</button>
          <output aria-live="polite">{cd} s</output>
          <button aria-label="Longer countdown" disabled={busy || cd >= 10} onClick={() => patch({ countdownSeconds: cd + 1 }, `Countdown ${cd + 1} s`)}>+</button>
        </div>
        <label className="check"><input type="checkbox" checked={!!settings.previewMirror} disabled={busy} onChange={(e) => patch({ previewMirror: e.target.checked }, e.target.checked ? "Preview mirrored" : "Preview not mirrored")} /> Mirror the live preview <span className="muted small">(saved photos are never mirrored)</span></label>
        <label className="inline">Print resolution
          <select aria-label="Output DPI" value={settings.outputDpi || 300} disabled={busy} onChange={(e) => patch({ outputDpi: Number(e.target.value) }, "Resolution saved")}>
            <option value={300}>300 dpi (standard)</option><option value={600}>600 dpi (high, slower)</option>
          </select>
        </label>
      </div>
    </section>
  );
}
