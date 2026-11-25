import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { FrameDef } from "../../../shared/types";

const h = vi.hoisted(() => ({ api: { previewDraft: vi.fn(), saveFrame: vi.fn(), importFrameImage: vi.fn(), newFrameDraft: vi.fn() } }));
vi.mock("../../../shared/api", () => ({
  api: h.api,
  AppError: class extends Error { action: string; constructor(m: string, a = "") { super(m); this.action = a; } },
  toAppError: (e: unknown) => Object.assign(new Error(e instanceof Error ? e.message : String(e)), { action: (e as { action?: string })?.action ?? "" }),
}));
vi.mock("../../../app/state", async () => {
  const { translate, formatTranslation } = await import("../../../shared/i18n");
  return { useApp: () => ({}), useT: () => ({ t: (key: Parameters<typeof translate>[1]) => translate("en", key), tf: (key: Parameters<typeof translate>[1], values: Record<string, string | number>) => formatTranslation("en", key, values) }) };
});
import { FrameBuilder } from "./FrameBuilder";

const DRAFT: FrameDef = {
  id: "custom-x", name: "X", kind: "custom", category: "Custom", order: 0, revision: 0, enabled: false, widthIn: 2, heightIn: 6, dpi: 300, background: "#ffffff",
  slots: [{ x: 0.15, y: 0.15, w: 1.7, h: 1.2, photo: 0 }, { x: 0.15, y: 1.45, w: 1.7, h: 1.2, photo: 1 }], layers: [],
};
const lastDraft = (): FrameDef => h.api.previewDraft.mock.calls.at(-1)![0];

beforeEach(() => {
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(() => ({ x: 0, y: 0, left: 0, top: 0, right: 200, bottom: 600, width: 200, height: 600, toJSON() {} }) as DOMRect);
  if (!window.PointerEvent) (window as unknown as { PointerEvent: typeof MouseEvent }).PointerEvent = MouseEvent as never;
  h.api.previewDraft.mockReset().mockResolvedValue("data:image/jpeg;base64,AAAA");
  h.api.saveFrame.mockReset().mockImplementation(async (f: FrameDef) => ({ ...f, revision: 1, photos: 2, format: "2×6 strip", pixelW: 600, pixelH: 1800 }));
  h.api.importFrameImage.mockReset();
});
afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const open = (props: Partial<React.ComponentProps<typeof FrameBuilder>> = {}) => {
  const onSaved = vi.fn(), onClose = vi.fn();
  render(<FrameBuilder initial={DRAFT} isNew onSaved={onSaved} onClose={onClose} {...props} />);
  return { onSaved, onClose };
};
const flush = () => act(async () => { await new Promise((r) => setTimeout(r, 220)); });

describe("Frame Builder", () => {
  it("shows the three panes and a live preview rendered by the backend", async () => {
    open();
    await flush();
    expect(screen.getByRole("complementary", { name: "Layers and elements" })).toBeTruthy();
    expect(screen.getByRole("complementary", { name: "Properties" })).toBeTruthy();
    expect((screen.getByAltText("Frame preview") as HTMLImageElement).src).toContain("data:image/jpeg");
    expect(h.api.previewDraft).toHaveBeenCalledWith(expect.objectContaining({ id: "custom-x" }), "", "", 560);
    expect(screen.getByTestId("fb-s0")).toBeTruthy(); expect(screen.getByTestId("fb-s1")).toBeTruthy();
  });

  it("dragging a photo slot moves it by exactly the dragged print distance and re-renders", async () => {
    open();
    await flush();
    fireEvent.click(screen.getByLabelText("Snap")); // snapping off for an exact distance
    const slot = screen.getByTestId("fb-s0");
    // the canvas is shown 200 px wide for 2 in → 100 px per inch
    fireEvent.pointerDown(slot, { clientX: 50, clientY: 50, button: 0 });
    fireEvent.pointerMove(window, { clientX: 60, clientY: 70 });
    fireEvent.pointerUp(window, { clientX: 60, clientY: 70 });
    await flush();
    expect(lastDraft().slots[0]).toMatchObject({ x: 0.25, y: 0.35, w: 1.7, h: 1.2 });
    expect(lastDraft().slots[1].y).toBe(1.45);                       // other slots untouched
  });

  it("resizing with a handle changes only that edge", async () => {
    open();
    await flush();
    fireEvent.click(screen.getByLabelText("Snap"));
    fireEvent.pointerDown(screen.getByTestId("fb-s0"), { clientX: 10, clientY: 10, button: 0 }); // select (no move)
    fireEvent.pointerUp(window);
    const handle = within(screen.getByTestId("fb-s0")).getAllByRole("generic", { hidden: true }).find((e) => e.getAttribute("data-handle") === "s")!;
    fireEvent.pointerDown(handle, { clientX: 100, clientY: 100, button: 0 });
    fireEvent.pointerMove(window, { clientX: 100, clientY: 90 });
    fireEvent.pointerUp(window);
    await flush();
    expect(lastDraft().slots[0]).toMatchObject({ x: 0.15, y: 0.15, w: 1.7 });
    expect(lastDraft().slots[0].h).toBeCloseTo(1.1, 3);
  });

  it("locked slots cannot be dragged", async () => {
    open({ initial: { ...DRAFT, slots: [{ ...DRAFT.slots[0], locked: true }, DRAFT.slots[1]] } });
    await flush();
    const before = h.api.previewDraft.mock.calls.length;
    fireEvent.pointerDown(screen.getByTestId("fb-s0"), { clientX: 50, clientY: 50, button: 0 });
    fireEvent.pointerMove(window, { clientX: 90, clientY: 90 });
    fireEvent.pointerUp(window);
    await flush();
    expect(h.api.previewDraft.mock.calls.length).toBe(before);
  });

  it("adds a text layer, edits it live, and undo/redo restore the earlier states", async () => {
    open();
    await flush();
    fireEvent.click(screen.getByRole("button", { name: "＋ Text" }));
    await flush();
    expect(within(screen.getByRole("list", { name: "Layers" })).getByText("Text")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Text"), { target: { value: "Hello {caption}" } });
    await flush();
    expect(lastDraft().layers![0]).toMatchObject({ kind: "text", text: "Hello {caption}" });
    fireEvent.click(screen.getByRole("button", { name: "Undo" }));         // the text edit
    await flush();
    expect(lastDraft().layers![0].text).toBe("{caption|date}");
    fireEvent.click(screen.getByRole("button", { name: "Undo" }));         // the layer itself
    await flush();
    expect(lastDraft().layers).toHaveLength(0);
    fireEvent.click(screen.getByRole("button", { name: "Redo" }));
    await flush();
    expect(lastDraft().layers).toHaveLength(1);
  });

  it("orders layers, hides under-photo layers and deletes with the keyboard", async () => {
    open();
    await flush();
    fireEvent.click(screen.getByRole("button", { name: "＋ Rectangle" }));
    fireEvent.click(screen.getByRole("button", { name: "＋ Ellipse" }));
    await flush();
    expect(lastDraft().layers!.map((l) => l.kind)).toEqual(["rect", "ellipse"]);
    fireEvent.click(screen.getByRole("button", { name: "Send backward" }));
    await flush();
    expect(lastDraft().layers!.map((l) => l.kind)).toEqual(["ellipse", "rect"]);
    fireEvent.keyDown(window, { key: "Delete" });
    await flush();
    expect(lastDraft().layers!.map((l) => l.kind)).toEqual(["rect"]);               // the selected ellipse was deleted
    fireEvent.keyDown(window, { key: "z", ctrlKey: true });
    await flush();
    expect(lastDraft().layers).toHaveLength(2);
  });

  it("arrow keys nudge the selected slot (Shift = 0.1 in) but not while typing", async () => {
    open();
    await flush();
    fireEvent.pointerDown(screen.getByTestId("fb-s0"), { clientX: 10, clientY: 10, button: 0 });
    fireEvent.pointerUp(window);
    fireEvent.keyDown(window, { key: "ArrowRight", shiftKey: true });
    await flush();
    expect(lastDraft().slots[0].x).toBeCloseTo(0.25, 6);
    fireEvent.keyDown(screen.getByLabelText("X"), { key: "ArrowRight" });   // typing in a field: no nudge
    await flush();
    expect(lastDraft().slots[0].x).toBeCloseTo(0.25, 6);
  });

  it("imports a transparent overlay through the backend and warns when it would cover the photos", async () => {
    h.api.importFrameImage.mockResolvedValue({ name: "img-1.png", format: "png", width: 300, height: 900, hasAlpha: false, bytes: 10 });
    open();
    await flush();
    const input = screen.getByLabelText("Import overlay…") as HTMLInputElement;
    const file = new File([new Uint8Array([137, 80, 78, 71])], "frame.png", { type: "image/png" });
    await act(async () => { fireEvent.change(input, { target: { files: [file] } }); await new Promise((r) => setTimeout(r, 50)); });
    expect(h.api.importFrameImage).toHaveBeenCalledWith(expect.stringMatching(/^data:image\/png;base64,/));
    await flush();
    expect(lastDraft().overlay).toBe("img-1.png");
    expect(screen.getByText(/no transparency/)).toBeTruthy();
    expect(screen.getByText(/smaller than the 600×1800/)).toBeTruthy();
  });

  it("shows the backend's reason when an import is refused and keeps the draft unchanged", async () => {
    h.api.importFrameImage.mockRejectedValue(Object.assign(new Error("The file is not a valid PNG or JPEG image (SVG and other formats are not accepted)"), { action: "Use a PNG or JPEG file." }));
    open();
    await flush();
    const input = screen.getByLabelText("Import overlay…") as HTMLInputElement;
    await act(async () => { fireEvent.change(input, { target: { files: [new File(["<svg/>"], "x.svg", { type: "image/svg+xml" })] } }); await new Promise((r) => setTimeout(r, 50)); });
    expect(screen.getByRole("alert").textContent).toMatch(/not a valid PNG or JPEG/);
    expect(lastDraft().overlay).toBeUndefined();
  });

  it("refuses a file over 15 MB without sending it", async () => {
    open();
    await flush();
    const big = new File([new Uint8Array(1)], "big.png", { type: "image/png" });
    Object.defineProperty(big, "size", { value: 16 * 1024 * 1024 });
    await act(async () => { fireEvent.change(screen.getByLabelText("Import overlay…"), { target: { files: [big] } }); });
    expect(h.api.importFrameImage).not.toHaveBeenCalled();
    expect(screen.getByRole("alert").textContent).toMatch(/larger than 15 MB/);
  });

  it("saves the edited draft, reports it and closes through onSaved", async () => {
    const { onSaved } = open();
    await flush();
    fireEvent.change(screen.getByLabelText("Frame name"), { target: { value: "Gold" } });
    fireEvent.click(screen.getByRole("checkbox", { name: /Offer in Booth Mode/ }));
    fireEvent.click(screen.getByRole("button", { name: "Save frame" }));
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    expect(h.api.saveFrame).toHaveBeenCalledWith(expect.objectContaining({ id: "custom-x", name: "Gold", enabled: true, slots: DRAFT.slots }));
  });

  it("shows a save error (for example a slot outside the canvas) and stays open", async () => {
    h.api.saveFrame.mockRejectedValueOnce(Object.assign(new Error("slot 2 lies outside the canvas"), { action: "Fix the highlighted problem and save again." }));
    const { onSaved } = open();
    await flush();
    fireEvent.click(screen.getByRole("button", { name: "Save frame" }));
    await waitFor(() => expect(screen.getByRole("alert").textContent).toMatch(/outside the canvas/));
    expect(onSaved).not.toHaveBeenCalled();
  });

  it("asks before discarding unsaved changes", async () => {
    const { onClose } = open();
    await flush();
    fireEvent.click(screen.getByRole("button", { name: "← Frames" }));
    expect(onClose).toHaveBeenCalledTimes(1);                                      // nothing changed: closes at once
    cleanup();
    const second = open();
    await flush();
    fireEvent.change(screen.getByLabelText("Frame name"), { target: { value: "Changed" } });
    fireEvent.click(screen.getByRole("button", { name: "← Frames" }));
    expect(second.onClose).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Discard" }));
    expect(second.onClose).toHaveBeenCalled();
  });

  it("colour options can be added and previewed", async () => {
    open();
    await flush();
    fireEvent.click(screen.getByRole("button", { name: "Add colour option" }));
    await flush();
    expect(lastDraft().variants!.map((v) => v.id)).toEqual(["default", "color-2"]);
    fireEvent.change(screen.getByLabelText("Preview colour"), { target: { value: "color-2" } });
    await flush();
    expect(h.api.previewDraft.mock.calls.at(-1)![1]).toBe("color-2");
  });

  it("Save as copy keeps the original and saves a disabled copy under a fresh id", async () => {
    h.api.newFrameDraft.mockResolvedValue({ ...DRAFT, id: "custom-x-copy", name: "X copy" });
    const { onSaved } = open({ isNew: false });
    await flush();
    fireEvent.click(screen.getByRole("button", { name: "Save as copy" }));
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    expect(h.api.saveFrame).toHaveBeenCalledWith(expect.objectContaining({ id: "custom-x-copy", name: "X copy", enabled: false, revision: 0 }));
  });
});

describe("Frame Builder opened from a PNG import", () => {
  it("shows what the import found, until dismissed, and knows the image it came with", async () => {
    const draft: FrameDef = { ...DRAFT, overlay: "img-1.png" };
    render(<FrameBuilder initial={draft} isNew notes={["Found 4 transparent windows: a photo slot was placed under each."]}
      knownAssets={[{ name: "img-1.png", format: "png", width: 600, height: 1800, hasAlpha: true, bytes: 10 }]} onSaved={() => {}} onClose={() => {}} />);
    await flush();
    expect(screen.getByRole("status", { name: "Import result" }).textContent).toMatch(/Found 4 transparent windows/);
    expect(screen.getByText(/600×1800 px · PNG · transparent/)).toBeTruthy();         // the overlay's details are shown, no warnings
    expect(screen.queryByText(/no transparency/)).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Dismiss import notes" }));
    expect(screen.queryByRole("status", { name: "Import result" })).toBeNull();
  });
});
