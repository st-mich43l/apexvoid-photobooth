import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { DOUBLE_TAP_MS, reconcileSelection, selectGridShape, slotsToSubmit, toggleSelection, usePhotoSelection } from "./photoSelection";
import type { Shot } from "../../shared/types";

const shots = (n: number, prefix = "m"): Shot[] => Array.from({ length: n }, (_, i) => ({ slot: i, mediaId: `${prefix}${i}`, simulated: false }));
const V = (n: number) => new Set(shots(n).map((s) => s.mediaId));

describe("selection rules (pure)", () => {
  it("selects, unselects, and never exceeds the required count or repeats a photo", () => {
    let r = toggleSelection([], "m3", 2, V(8)); expect(r.ids).toEqual(["m3"]);
    r = toggleSelection(r.ids, "m1", 2, V(8)); expect(r.ids).toEqual(["m3", "m1"]);
    r = toggleSelection(r.ids, "m5", 2, V(8)); expect(r).toMatchObject({ ids: ["m3", "m1"], changed: false, reason: "full" });
    r = toggleSelection(r.ids, "m3", 2, V(8)); expect(r.ids).toEqual(["m1"]);
    expect(toggleSelection(["m1"], "ghost", 2, V(8))).toMatchObject({ changed: false, reason: "unknown" });
  });
  it("reconcile drops unknown and duplicate ids and trims to the required count, keeping the first picks", () => {
    expect(reconcileSelection(["m1", "gone", "m1", "m2", "m3"], 2, V(8))).toEqual(["m1", "m2"]);
    expect(reconcileSelection(["m1"], 0, V(8))).toEqual([]);
  });
  it("submits capture slots in selection order, only when exactly the required count is valid", () => {
    const c = shots(8);
    expect(slotsToSubmit(["m1", "m3", "m0", "m7", "m4", "m6"], c, 6)).toEqual([1, 3, 0, 7, 4, 6]);
    expect(slotsToSubmit(["m1", "m3"], c, 6)).toEqual([]);
    expect(slotsToSubmit(["m1", "nope"], c, 2)).toEqual([]);
    expect(slotsToSubmit(["m1", "m1"], c, 2)).toEqual([]);
  });
  it("lays eight candidates out 4×2 on landscape and 2×4 on portrait, and other counts sensibly", () => {
    expect(selectGridShape(8, false)).toEqual({ cols: 4, rows: 2 });
    expect(selectGridShape(8, true)).toEqual({ cols: 2, rows: 4 });
    expect(selectGridShape(6, false)).toEqual({ cols: 3, rows: 2 });
    expect(selectGridShape(4, false)).toEqual({ cols: 4, rows: 1 });
    expect(selectGridShape(12, false)).toEqual({ cols: 4, rows: 3 });
    expect(selectGridShape(12, true)).toEqual({ cols: 3, rows: 4 });
    expect(selectGridShape(1, true)).toEqual({ cols: 1, rows: 1 });
    for (let n = 1; n <= 12; n++) for (const p of [true, false]) { const g = selectGridShape(n, p); expect(g.cols * g.rows).toBeGreaterThanOrEqual(n); }
  });
});

describe("usePhotoSelection", () => {
  let t = 0;
  const clock = () => t;
  const tap = (r: { current: ReturnType<typeof usePhotoSelection> }, id: string) => { t += DOUBLE_TAP_MS + 50; act(() => r.current.toggle(id)); };
  const setup = (sid = "S1", cands = shots(8), required = 6) => renderHook((p: { sid: string; cands: Shot[]; required: number }) => usePhotoSelection(p.sid, p.cands, p.required, clock), { initialProps: { sid, cands, required } });

  it("starts with nothing selected and Continue unavailable", () => {
    const { result } = setup();
    expect(result.current.selectedIds).toEqual([]);
    expect(result.current).toMatchObject({ count: 0, required: 6, remaining: 6, complete: false, slots: [] });
  });

  it("numbers photos in the order they were tapped, and renumbers when one is unselected", () => {
    const { result } = setup();
    for (const id of ["m1", "m3", "m0"]) tap(result, id);
    expect([result.current.orderOf("m1"), result.current.orderOf("m3"), result.current.orderOf("m0"), result.current.orderOf("m5")]).toEqual([1, 2, 3, 0]);
    tap(result, "m3");
    expect([result.current.orderOf("m1"), result.current.orderOf("m0")]).toEqual([1, 2]);
  });

  it("completes at exactly the required count, refuses more with a notice, and gives the backend the capture slots in order", () => {
    const { result } = setup();
    for (const id of ["m1", "m3", "m0", "m7", "m4"]) tap(result, id);
    expect(result.current).toMatchObject({ complete: false, remaining: 1 });
    tap(result, "m6");
    expect(result.current).toMatchObject({ complete: true, remaining: 0, slots: [1, 3, 0, 7, 4, 6] });
    tap(result, "m2");
    expect(result.current.notice).toBe("full");
    expect(result.current.selectedIds).toHaveLength(6);
    tap(result, "m1");
    expect(result.current.notice).toBeNull();
    expect(result.current.complete).toBe(false);
  });

  it("the example from the spec: eight shots, a six-photo frame, 2, 4, 1, 7, 5, 8", () => {
    const { result } = setup();
    for (const n of [2, 4, 1, 7, 5, 8]) tap(result, `m${n - 1}`);
    expect(result.current.slots).toEqual([1, 3, 0, 6, 4, 7]);
  });

  it("an accidental double tap does not select-then-unselect", () => {
    const { result } = setup();
    act(() => result.current.toggle("m2"));
    t += 100;
    act(() => result.current.toggle("m2"));
    expect(result.current.selectedIds).toEqual(["m2"]);
    t += DOUBLE_TAP_MS + 10;
    act(() => result.current.toggle("m2"));
    expect(result.current.selectedIds).toEqual([]);
    // two different photos in quick succession are both taps
    t += 1000; act(() => result.current.toggle("m1")); t += 50; act(() => result.current.toggle("m2"));
    expect(result.current.selectedIds).toEqual(["m1", "m2"]);
  });

  it("is scoped to the session: a new session starts empty", () => {
    const { result, rerender } = setup();
    tap(result, "m1"); tap(result, "m2");
    expect(result.current.count).toBe(2);
    rerender({ sid: "S2", cands: shots(8, "n"), required: 6 });
    expect(result.current.selectedIds).toEqual([]);
    rerender({ sid: "S1", cands: shots(8), required: 6 });
    expect(result.current.selectedIds).toEqual([]); // not resurrected either
  });

  it("a frame with fewer photos keeps the first valid picks; a frame with more keeps them all and asks for the rest", () => {
    const { result, rerender } = setup();
    for (const id of ["m4", "m1", "m6", "m2"]) tap(result, id);
    rerender({ sid: "S1", cands: shots(8), required: 3 });
    expect(result.current.selectedIds).toEqual(["m4", "m1", "m6"]);
    expect(result.current.complete).toBe(true);
    rerender({ sid: "S1", cands: shots(8), required: 6 });
    expect(result.current.selectedIds).toEqual(["m4", "m1", "m6"]);
    expect(result.current).toMatchObject({ complete: false, remaining: 3 });
  });

  it("a retaken photo (new media id) silently leaves the selection instead of being submitted stale", () => {
    const { result, rerender } = setup("S1", shots(8), 2);
    tap(result, "m1"); tap(result, "m2");
    rerender({ sid: "S1", cands: shots(8).map((c) => (c.slot === 1 ? { ...c, mediaId: "m1-retaken" } : c)), required: 2 });
    expect(result.current.selectedIds).toEqual(["m2"]);
    expect(result.current.complete).toBe(false);
    expect(result.current.slots).toEqual([]);
  });

  it("submits once: Continue pressed again while sending, or after success, sends nothing", async () => {
    const { result } = setup("S1", shots(4), 2);
    tap(result, "m3"); tap(result, "m0");
    const sent: number[][] = [];
    let release!: (ok: boolean) => void;
    const send = (s: number[]) => { sent.push(s); return new Promise<boolean>((r) => { release = r; }); };
    let p1!: Promise<void>;
    act(() => { p1 = result.current.submit(send); void result.current.submit(send); void result.current.submit(send); });
    expect(sent).toEqual([[3, 0]]);
    await act(async () => { release(true); await p1; });
    await act(async () => { await result.current.submit(send); });
    expect(sent).toHaveLength(1);
  });

  it("a failed send may be retried", async () => {
    const { result } = setup("S1", shots(4), 2);
    tap(result, "m1"); tap(result, "m2");
    const sent: number[][] = [];
    await act(async () => { await result.current.submit(async (s) => { sent.push(s); return false; }); });
    await act(async () => { await result.current.submit(async (s) => { sent.push(s); return true; }); });
    expect(sent).toEqual([[1, 2], [1, 2]]);
  });

  it("does not submit an incomplete selection", async () => {
    const { result } = setup("S1", shots(4), 2);
    tap(result, "m1");
    let called = false;
    await act(async () => { await result.current.submit(async () => { called = true; return true; }); });
    expect(called).toBe(false);
  });
});
