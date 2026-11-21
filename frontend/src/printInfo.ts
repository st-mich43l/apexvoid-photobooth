import type { JobStatus, Paper } from "./types";

export const jobLabel: Record<JobStatus, string> = {
  submitting: "Sending…", queued: "Queued", printing: "Printing", completed: "Printed (spooler says so)",
  removed: "Left the queue", failed: "Failed", unknown: "Unknown",
};
export const jobKind = (s: JobStatus): "ok" | "warn" | "err" | "info" =>
  s === "completed" ? "ok" : s === "removed" ? "info" : s === "failed" ? "err" : s === "unknown" ? "warn" : "info";
export const jobActive = (s: JobStatus): boolean => s === "submitting" || s === "queued" || s === "printing";

/** A new id per user action: the backend never prints the same id twice. */
export const newRequestId = (): string =>
  typeof crypto !== "undefined" && "randomUUID" in crypto ? crypto.randomUUID() : `r${Date.now()}${Math.random().toString(16).slice(2)}`;

export const paperLabel = (p: Paper): string => `${p.name} (${p.widthIn.toFixed(2)}×${p.heightIn.toFixed(2)} in)`;

/** Picks the printer's own paper whose size matches (either orientation), preferring the saved default. */
export function pickPaper(papers: Paper[], saved: string, wantIn?: [number, number]): string {
  if (saved && papers.some((p) => p.name === saved)) return saved;
  if (wantIn) {
    const m = papers.find((p) => (Math.abs(p.widthIn - wantIn[0]) < 0.1 && Math.abs(p.heightIn - wantIn[1]) < 0.1) || (Math.abs(p.widthIn - wantIn[1]) < 0.1 && Math.abs(p.heightIn - wantIn[0]) < 0.1));
    if (m) return m.name;
  }
  return papers[0]?.name ?? "";
}

export function describeMargins(m: [number, number, number, number]): string {
  const f = (n: number) => `${Math.max(0, n).toFixed(1)} mm`;
  return `left ${f(m[0])}, top ${f(m[1])}, right ${f(m[2])}, bottom ${f(m[3])}`;
}
