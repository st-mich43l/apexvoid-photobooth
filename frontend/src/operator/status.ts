import type { BoothReadiness, ReadinessGroup, ReadinessItem } from "../types";

/** What the operator sees. One vocabulary everywhere in Studio (Dashboard, Prepare Booth, Camera, Printer, Diagnostics). */
export type View = "ready" | "setup" | "warning" | "unavailable" | "unverified" | "demo" | "info";

export const VIEW_LABEL: Record<View, string> = {
  ready: "Ready", setup: "Setup required", warning: "Warning", unavailable: "Unavailable", unverified: "Not verified", demo: "Demo / test mode", info: "Info",
};
export const VIEW_TONE: Record<View, "ok" | "err" | "warn" | "info" | "sim"> = {
  ready: "ok", setup: "err", warning: "warn", unavailable: "warn", unverified: "info", demo: "sim", info: "info",
};
/** Higher = more urgent; a section shows its most urgent line. */
const RANK: Record<View, number> = { setup: 6, warning: 5, unavailable: 4, demo: 3, unverified: 2, ready: 1, info: 0 };

/**
 * How one readiness line is shown:
 *  required + problem   → Setup required (blocks guests)
 *  optional + warn      → Unavailable for printing, Warning for everything else (never blocks)
 *  ok + demo            → Demo / test mode  (a SIMULATED camera is never "Ready")
 *  ok + unverified      → Not verified      (present, but nothing has proved it works yet)
 */
export function viewOf(it: ReadinessItem): View {
  if (it.state === "problem") return it.required ? "setup" : "warning";
  if (it.state === "warn") return it.key === "printer" ? "unavailable" : "warning";
  if (it.state === "info") return "info";
  if (it.verification === "demo") return "demo";
  if (it.verification === "unverified") return "unverified";
  return "ready";
}

export const itemsOf = (r: BoothReadiness | null | undefined, group: ReadinessGroup): ReadinessItem[] => (r?.items ?? []).filter((i) => i.group === group);

/** The most urgent view of a section. A section with no lines yet (no answer) is "info". */
export function groupView(items: ReadinessItem[]): View {
  let best: View = "info";
  for (const it of items) { const v = viewOf(it); if (RANK[v] > RANK[best]) best = v; }
  return best;
}

/** Required lines that currently stop guests from taking photos. */
export const blockers = (r: BoothReadiness | null | undefined): ReadinessItem[] => (r?.items ?? []).filter((i) => i.required && i.state === "problem");

/** Which Studio page fixes a readiness line. */
export const FIX: Record<string, [page: string, label: string]> = {
  camera: ["camera", "Camera setup"], capture: ["camera", "Camera setup"],
  storage: ["settings", "Library settings"], library: ["settings", "Library settings"], space: ["settings", "Library settings"],
  frames: ["frames", "Manage frames"], printer: ["printer", "Printer setup"], session: ["photo", "Open the session"], interrupted: ["gallery", "Open Gallery"],
};
