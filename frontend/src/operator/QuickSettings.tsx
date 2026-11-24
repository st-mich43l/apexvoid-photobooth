import { usePatchSettings } from "./usePatchSettings";
import { useApp, useT } from "../state";
import { ErrorNote } from "../ui";

/** The three settings operators change most. Each change is a PatchSettings of just that key. */
export function QuickSettings() {
  const { settings } = useApp();
  const { t, tf } = useT();
  const { patch, busy, error, clear } = usePatchSettings();
  const cd = settings.countdownSeconds;
  return (
    <section className="card quick" aria-label={t("quickSettings")}>
      <h3>{t("quickSettings")}</h3>
      <ErrorNote error={error} onClose={clear} />
      <div className="quick-row">
        <div className="stepper" role="group" aria-label={t("countdown")}>
          <span className="lbl">{t("countdown")}</span>
          <button aria-label={t("shorterCountdown")} disabled={busy || cd <= 1} onClick={() => patch({ countdownSeconds: cd - 1 }, tf("shorterCountdownSaved", { seconds: cd - 1 }))}>−</button>
          <output aria-live="polite">{cd} s</output>
          <button aria-label={t("longerCountdown")} disabled={busy || cd >= 10} onClick={() => patch({ countdownSeconds: cd + 1 }, tf("longerCountdownSaved", { seconds: cd + 1 }))}>+</button>
        </div>
        <label className="check"><input type="checkbox" checked={!!settings.previewMirror} disabled={busy} onChange={(e) => patch({ previewMirror: e.target.checked }, e.target.checked ? t("previewMirrored") : t("previewNotMirrored"))} /> {t("mirrorPreviewShort")} <span className="muted small">{t("savedPhotosNeverMirror")}</span></label>
        <label className="inline">{t("outputResolution")}
          <select aria-label={t("outputDPI")} value={settings.outputDpi || 300} disabled={busy} onChange={(e) => patch({ outputDpi: Number(e.target.value) }, t("resolutionSaved"))}>
            <option value={300}>{t("standard300")}</option><option value={600}>{t("high600")}</option>
          </select>
        </label>
      </div>
    </section>
  );
}
