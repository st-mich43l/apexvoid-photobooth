import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { Shot } from "../../shared/types";

/**
 * The one photo-selection model for the review screen (Booth and Studio).
 *
 * The guest taps photographs to choose them. The frame needs exactly `required` of them; the order of the taps is the
 * order they take in the frame. There is no frame-slot assignment, no automatic selection and no replacement:
 *  - a tap on an unselected photo selects it (if there is room), a tap on a selected one unselects it;
 *  - a photo is selected at most once, and never beyond `required`;
 *  - selections are identified by the capture's media id (a retake replaces the id, so a stale choice drops out);
 *  - the selection belongs to one session and starts empty for every new session.
 * Nothing here talks to the camera.
 */

export interface SelectionResult { ids: string[]; changed: boolean; reason?: "full" | "unknown" }

/** Tap on `id`. Pure: the same inputs always give the same selection. */
export function toggleSelection(ids: readonly string[], id: string, required: number, valid: ReadonlySet<string>): SelectionResult {
  if (!valid.has(id)) return { ids: [...ids], changed: false, reason: "unknown" };
  if (ids.includes(id)) return { ids: ids.filter((x) => x !== id), changed: true };
  if (ids.length >= required) return { ids: [...ids], changed: false, reason: "full" };
  return { ids: [...ids, id], changed: true };
}

/** Keeps only valid, distinct ids, in order, and never more than `required` (a frame with fewer photos keeps the first picks). */
export function reconcileSelection(ids: readonly string[], required: number, valid: ReadonlySet<string>): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const id of ids) {
    if (!valid.has(id) || seen.has(id)) continue;
    seen.add(id);
    if (out.length < Math.max(0, required)) out.push(id);
  }
  return out;
}

/** The capture slots to send to the backend, in selection order. Empty unless exactly `required` valid photos are selected. */
export function slotsToSubmit(ids: readonly string[], candidates: readonly Shot[], required: number): number[] {
  const bySlot = new Map(candidates.map((c) => [c.mediaId, c.slot]));
  const slots = ids.map((id) => bySlot.get(id));
  if (required < 1 || ids.length !== required || slots.some((s) => s === undefined) || new Set(slots).size !== slots.length) return [];
  return slots as number[];
}

/** Rows and columns of the review grid for `n` candidates. Eight candidates are 4×2 on landscape and 2×4 on portrait. */
export function selectGridShape(n: number, portrait: boolean): { cols: number; rows: number } {
  const count = Math.max(1, n);
  let cols: number;
  if (portrait) cols = count <= 2 ? 1 : count <= 8 ? 2 : 3;
  else cols = count <= 4 ? count : count <= 6 ? 3 : 4;
  return { cols, rows: Math.ceil(count / cols) };
}

/** Taps on the same photo closer together than this are one accidental double tap, not select-then-unselect. */
export const DOUBLE_TAP_MS = 320;

export interface PhotoSelection {
  selectedIds: readonly string[];
  /** 1-based position in the selection, 0 when not selected. */
  orderOf: (id: string) => number;
  toggle: (id: string) => void;
  count: number;
  required: number;
  remaining: number;
  complete: boolean;
  /** "full" right after a tap that was refused because the frame already has all its photos. */
  notice: "full" | null;
  /** Capture slots in selection order; empty until the selection is complete. */
  slots: number[];
  /** Sends the selection once. A second call (double tap on Continue) while one is in flight or after success does nothing. */
  submit: (send: (slots: number[]) => Promise<boolean>) => Promise<void>;
  submitting: boolean;
}

/** Selection state for one review. `sessionId` scopes it: a new session starts with nothing selected. */
export function usePhotoSelection(sessionId: string, candidates: readonly Shot[], required: number, now: () => number = Date.now): PhotoSelection {
  const [state, setState] = useState<{ sessionId: string; ids: string[] }>({ sessionId, ids: [] });
  const [notice, setNotice] = useState<"full" | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const submitted = useRef<string>("");
  const lastTap = useRef<{ id: string; at: number } | null>(null);

  const valid = useMemo(() => new Set(candidates.map((c) => c.mediaId)), [candidates]);
  const raw = state.sessionId === sessionId ? state.ids : [];
  const selectedIds = useMemo(() => reconcileSelection(raw, required, valid), [raw, required, valid]);

  // a new session, a retaken photo or a frame with another photo count: keep what is still valid, drop the rest
  useEffect(() => {
    setState((cur) => {
      const base = cur.sessionId === sessionId ? cur.ids : [];
      const next = reconcileSelection(base, required, valid);
      if (cur.sessionId === sessionId && next.length === cur.ids.length && next.every((id, i) => id === cur.ids[i])) return cur;
      return { sessionId, ids: next };
    });
    setNotice(null);
  }, [sessionId, required, valid]);
  useEffect(() => { submitted.current = ""; lastTap.current = null; setSubmitting(false); }, [sessionId]);

  const toggle = useCallback((id: string) => {
    const t = now();
    const prev = lastTap.current;
    lastTap.current = { id, at: t };
    if (prev && prev.id === id && t - prev.at < DOUBLE_TAP_MS) return; // accidental double tap
    const r = toggleSelection(selectedIds, id, required, valid);
    if (r.reason === "full") { setNotice("full"); return; }
    setNotice(null);
    if (r.changed) setState({ sessionId, ids: r.ids });
  }, [selectedIds, required, valid, sessionId, now]);

  const slots = useMemo(() => slotsToSubmit(selectedIds, candidates, required), [selectedIds, candidates, required]);
  const complete = slots.length > 0;

  const submit = useCallback(async (send: (slots: number[]) => Promise<boolean>) => {
    if (!complete || submitted.current === sessionId) return; // nothing valid to send, or already sent for this session
    submitted.current = sessionId;
    setSubmitting(true);
    let ok = false;
    try { ok = await send(slots); } finally {
      setSubmitting(false);
      if (!ok) submitted.current = ""; // a failed send may be tried again; a successful one is final for this session
    }
  }, [complete, slots, sessionId]);

  return {
    selectedIds, orderOf: (id) => selectedIds.indexOf(id) + 1, toggle, count: selectedIds.length, required, remaining: Math.max(0, required - selectedIds.length),
    complete, notice, slots, submit, submitting,
  };
}
