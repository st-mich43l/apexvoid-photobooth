import type { DeviceInfo, Mode, SourceKind } from "./types";

const KIND: Record<SourceKind, string> = {
  dslr: "Canon camera (Canon SDK)", "dslr-uvc": "Canon camera in USB streaming (UVC) mode", webcam: "Webcam",
  "capture-card": "Capture card", virtual: "Virtual (software) camera", simulated: "SIMULATED camera",
};
export const kindLabel = (k: SourceKind): string => KIND[k] ?? "Camera";

export const backendLabel = (driver: string): string =>
  driver === "canon-edsdk" ? "Canon EDSDK" : driver === "uvc" ? "Windows video device (FFmpeg/DirectShow)" : driver === "mock" ? "Mock (development)" : driver;

/** Video needs a streamed (UVC) source: the Canon SDK source and the simulated one are photo-only. */
export const canRecordVideo = (d: DeviceInfo): boolean => d.driver === "uvc" && !d.duplicateOf;

/** Distinct "1280×720" sizes, largest first. */
export function resolutions(modes?: Mode[]): string[] {
  const seen = new Set<string>();
  return [...(modes ?? [])]
    .sort((a, b) => b.width * b.height - a.width * a.height)
    .map((m) => `${m.width}×${m.height}`)
    .filter((s) => (seen.has(s) ? false : (seen.add(s), true)));
}

/** One select option per distinct size, using the best frame rate offered for it (capped at 30). */
export function modeOptions(modes?: Mode[]): Array<{ key: string; label: string; width: number; height: number; fps: number; format: string }> {
  const best = new Map<string, Mode>();
  for (const m of modes ?? []) {
    const k = `${m.width}x${m.height}`;
    const cur = best.get(k);
    if (!cur || (m.format === "mjpeg" && cur.format !== "mjpeg") || ((m.fpsMax || 30) > (cur.fpsMax || 30) && m.format === cur.format)) best.set(k, m);
  }
  return [...best.values()]
    .sort((a, b) => b.width * b.height - a.width * a.height)
    .map((m) => {
      const fps = Math.min(30, Math.round(m.fpsMax || 30));
      return { key: `${m.width}x${m.height}@${fps}`, label: `${m.width}×${m.height} @ ${fps} fps${m.format ? ` (${m.format})` : ""}`, width: m.width, height: m.height, fps, format: m.format };
    });
}

/** Device-specific guidance for an error, so the operator knows what to try. */
export function guidance(d: DeviceInfo | undefined, error?: string): string {
  if (!d || !error) return "";
  const e = error.toLowerCase();
  if (d.driver === "canon-edsdk") {
    if (/busy|owned|in use/.test(e)) return "Close EOS Utility, EOS Webcam Utility and Canon Camera Connect, then reconnect.";
    if (/sdk|dll/.test(e)) return "Copy the 64-bit EDSDK.dll and EdsImage.dll next to the app (see docs/CAMERAS.md).";
    return "Check the USB cable, that the camera is on and in a photo mode, and that nothing else controls it.";
  }
  if (d.driver === "uvc") {
    if (/ffmpeg/.test(e)) return "Place ffmpeg.exe next to the app or set its path in Settings.";
    if (/another program|in use|busy|could not be opened/.test(e)) return "Close other programs using the camera (Teams, Zoom, browser tabs, OBS), then try again.";
    if (d.kind === "dslr-uvc") return "Set the camera's USB mode to streaming/webcam, and close EOS Utility.";
    if (d.kind === "virtual") return "Start the program that provides this virtual camera first.";
    return "Check the USB connection and the Windows camera privacy settings (Settings → Privacy → Camera).";
  }
  return "";
}
