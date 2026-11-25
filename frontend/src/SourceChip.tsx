import { backendLabel, kindLabel } from "./sourceInfo";
import { useLocale } from "./locale";
import { translate } from "./i18n";
import { useApp } from "./state";

/** The actual device, backend and stream mode in use — shown while shooting so the operator never has to guess. */
export function SourceChip() {
  const { camera, settings } = useApp();
  const language = useLocale(settings?.language);
  const d = camera.device;
  if (!d) return null;
  const m = camera.mode;
  const kind = translate(language, d.driver === "uvc" ? "sourceFrameGrab" : d.driver === "canon-edsdk" ? "sourceOriginal" : "sourceSim");
  return (
    <div className="source-chip" title={`${kindLabel(d.kind, language)} · ${backendLabel(d.driver, language)}`}>
      <b>{d.name}</b>
      <span>{d.driver === "canon-edsdk" ? "Canon SDK" : d.driver === "uvc" ? "Webcam/UVC" : "Mock"}</span>
      {m && m.width > 0 && <span>{m.width}×{m.height}{m.fpsMax ? ` @ ${Math.round(m.fpsMax)} fps` : ""}</span>}
      <span>{kind}</span>
    </div>
  );
}
