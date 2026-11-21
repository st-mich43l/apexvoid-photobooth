import { backendLabel, kindLabel } from "./sourceInfo";
import { useApp } from "./state";

/** The actual device, backend and stream mode in use — shown while shooting so the operator never has to guess. */
export function SourceChip() {
  const { camera } = useApp();
  const d = camera.device;
  if (!d) return null;
  const m = camera.mode;
  const kind = d.driver === "uvc" ? "Frame grab from a video stream" : d.driver === "canon-edsdk" ? "Camera original (full resolution)" : "Simulated";
  return (
    <div className="source-chip" title={`${kindLabel(d.kind)} · ${backendLabel(d.driver)}`}>
      <b>{d.name}</b>
      <span>{d.driver === "canon-edsdk" ? "Canon SDK" : d.driver === "uvc" ? "Webcam/UVC" : "Mock"}</span>
      {m && m.width > 0 && <span>{m.width}×{m.height}{m.fpsMax ? ` @ ${Math.round(m.fpsMax)} fps` : ""}</span>}
      <span>{kind}</span>
    </div>
  );
}
