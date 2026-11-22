/** Undo / redo over immutable snapshots. Rapid edits with the same key (typing, dragging a slider) merge into one step. */
export interface History<T> { past: T[]; present: T; future: T[]; key?: string; at?: number }

export const MAX_HISTORY = 100;
const MERGE_MS = 700;

export const newHistory = <T,>(present: T): History<T> => ({ past: [], present, future: [] });
export const canUndo = <T,>(h: History<T>) => h.past.length > 0;
export const canRedo = <T,>(h: History<T>) => h.future.length > 0;

export function push<T>(h: History<T>, next: T, key?: string, now = Date.now()): History<T> {
  if (Object.is(next, h.present)) return h;
  const merge = key !== undefined && h.key === key && h.at !== undefined && now - h.at < MERGE_MS;
  const past = merge ? h.past : [...h.past, h.present].slice(-MAX_HISTORY);
  return { past, present: next, future: [], key, at: now };
}
export function undo<T>(h: History<T>): History<T> {
  if (!h.past.length) return h;
  return { past: h.past.slice(0, -1), present: h.past[h.past.length - 1], future: [h.present, ...h.future] };
}
export function redo<T>(h: History<T>): History<T> {
  if (!h.future.length) return h;
  return { past: [...h.past, h.present], present: h.future[0], future: h.future.slice(1) };
}
