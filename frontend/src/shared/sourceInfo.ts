import { translate } from "./i18n";
import type { DeviceInfo, Mode, SourceKind } from "./types";

const KIND = { dslr: "sourceCanon", "dslr-uvc": "sourceCanonUvc", webcam: "sourceWebcam", "capture-card": "sourceCaptureCard", virtual: "sourceVirtual", simulated: "sourceSimulated" } as const;
export const kindLabel = (k: SourceKind, language?: string): string => translate(language, KIND[k] ?? "sourceCamera");
export const backendLabel = (driver: string, language?: string): string =>
  driver === "canon-edsdk" ? "Canon EDSDK" : driver === "uvc" ? translate(language, "backendUvc") : driver === "mock" ? translate(language, "backendMock") : driver;

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
export function guidance(d: DeviceInfo | undefined, error?: string, language?: string): string {
  if (!d || !error) return "";
  if (d.driver === "canon-edsdk") return translate(language, "sourceCanonHelp");
  if (d.driver === "uvc") return translate(language, d.kind === "dslr-uvc" ? "sourceStreamHelp" : d.kind === "virtual" ? "sourceVirtualHelp" : "sourceUvcHelp");
  return "";
}
