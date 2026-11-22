import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Frame } from "../types";

const h = vi.hoisted(() => ({
  api: { listFrames: vi.fn(), importPngFrame: vi.fn(), newFrameDraft: vi.fn(), getFrame: vi.fn(), deleteFrame: vi.fn(), setFrameEnabled: vi.fn(), saveSettings: vi.fn() },
  S: {} as Record<string, unknown>,
  builder: vi.fn(),
}));
vi.mock("../state", () => ({ useApp: () => h.S }));
vi.mock("../api", () => ({ api: h.api, AppError: Error, toAppError: (e: unknown) => Object.assign(new Error(e instanceof Error ? e.message : String(e)), { action: (e as { action?: string })?.action ?? "" }) }));
vi.mock("../builder/FrameBuilder", () => ({ FrameBuilder: (p: Record<string, unknown>) => { h.builder(p); return <div data-testid="builder" />; } }));
import { Frames } from "./Frames";

const fr = (id: string, design: string, photos: number): Frame => ({ id, name: id, kind: "strip", category: "Plain", order: 1, revision: 1, enabled: true, builtin: true, design, widthIn: 2, heightIn: 6, dpi: 300, slots: [], background: "#fff", photos, format: "2×6 strip", pixelW: 600, pixelH: 1800 });

beforeEach(() => {
  h.S = { settings: { templateId: "plain.strip4" }, setSettings: vi.fn(), toast: vi.fn(), frameUrl: (id: string) => `/f/${id}` };
  h.builder.mockReset();
  for (const f of Object.values(h.api)) f.mockReset();
  h.api.listFrames.mockResolvedValue({ frames: [fr("plain.strip4", "plain", 4)], categories: ["Plain"], errors: [], dir: "", default: "plain.strip4" });
});
afterEach(cleanup);

const pick = async (file: File) => {
  await screen.findByRole("button", { name: "Import PNG as frame…" });
  await act(async () => { fireEvent.change(screen.getByLabelText("Import PNG as frame…"), { target: { files: [file] } }); await new Promise((r) => setTimeout(r, 30)); });
};

describe("Frames page: import a PNG as a frame", () => {
  it("offers the import next to Create frame", async () => {
    render(<Frames />);
    expect(await screen.findByRole("button", { name: "Create frame…" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Import PNG as frame…" })).toBeTruthy();
  });

  it("sends the picture and its file name, then opens the builder on the detected frame with the import notes", async () => {
    const draft = { id: "custom-wedding", name: "Wedding", slots: [{ x: 0, y: 0, w: 1, h: 1, photo: 0 }], widthIn: 2, heightIn: 6, dpi: 300 };
    const asset = { name: "img-1.png", format: "png", width: 600, height: 1800, hasAlpha: true, bytes: 10 };
    h.api.importPngFrame.mockResolvedValue({ draft, asset, windows: 4, mode: "windows", notes: ["Found 4 transparent windows"] });
    render(<Frames />);
    await pick(new File([new Uint8Array([137, 80, 78, 71])], "Wedding.png", { type: "image/png" }));
    await waitFor(() => expect(screen.getByTestId("builder")).toBeTruthy());
    expect(h.api.importPngFrame).toHaveBeenCalledWith(expect.stringMatching(/^data:image\/png;base64,/), "Wedding.png");
    expect(h.builder.mock.calls.at(-1)![0]).toMatchObject({ initial: draft, isNew: true, notes: ["Found 4 transparent windows"], knownAssets: [asset] });
  });

  it("shows why a file was refused and does not open the builder", async () => {
    h.api.importPngFrame.mockRejectedValue(Object.assign(new Error("The file is not a valid PNG or JPEG image"), { action: "Use a PNG or JPEG file." }));
    render(<Frames />);
    await pick(new File(["<svg/>"], "x.svg", { type: "image/svg+xml" }));
    await waitFor(() => expect(screen.getByRole("alert").textContent).toMatch(/not a valid PNG/));
    expect(screen.queryByTestId("builder")).toBeNull();
  });

  it("refuses a file over 15 MB without sending it", async () => {
    render(<Frames />);
    const big = new File([new Uint8Array(1)], "big.png", { type: "image/png" });
    Object.defineProperty(big, "size", { value: 16 * 1024 * 1024 });
    await pick(big);
    expect(h.api.importPngFrame).not.toHaveBeenCalled();
    expect((h.S.toast as ReturnType<typeof vi.fn>)).toHaveBeenCalledWith("That image is larger than 15 MB.", "err");
  });
});
