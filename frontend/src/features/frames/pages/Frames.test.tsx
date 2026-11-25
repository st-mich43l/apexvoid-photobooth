import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Frame } from "../../../shared/types";

const h = vi.hoisted(() => ({
  api: { listFrames: vi.fn(), importPngFrame: vi.fn(), newFrameDraft: vi.fn(), getFrame: vi.fn(), deleteFrame: vi.fn(), setFrameEnabled: vi.fn(), setFrameCatalogue: vi.fn(), saveSettings: vi.fn() },
  S: {} as Record<string, unknown>,
  builder: vi.fn(),
}));
vi.mock("../../../app/state", async () => {
  const { translate, formatTranslation } = await import("../../../shared/i18n");
  return { useApp: () => h.S, useT: () => ({ t: (key: Parameters<typeof translate>[1]) => translate("en", key), tf: (key: Parameters<typeof translate>[1], values: Record<string, string | number>) => formatTranslation("en", key, values) }) };
});
vi.mock("../../../shared/api", () => ({ api: h.api, AppError: Error, toAppError: (e: unknown) => Object.assign(new Error(e instanceof Error ? e.message : String(e)), { action: (e as { action?: string })?.action ?? "" }) }));
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
  it("shows a small sample set while keeping all layouts available in the builder", async () => {
    const layouts = ["strip4", "strip3", "strip2", "twoup4", "sheet6", "sheet4", "sheet2", "p57", "single"];
    const designs = ["plain", "rose-pink", "film-strip", ...Array.from({ length: 14 }, (_, i) => `extra${i}`)];
    const samples = new Set(["plain.strip4", "plain.strip3", "plain.strip2", "plain.single", "plain.sheet6", "rose-pink.strip4", "film-strip.strip4"]);
    const frames = designs.flatMap((design) => layouts.map((layout) => ({ ...fr(`${design}.${layout}`, design, 4), category: design === "extra0" ? "Extra" : "Plain", enabled: design === "plain" && samples.has(`${design}.${layout}`) })));
    h.api.listFrames.mockResolvedValue({ frames, categories: ["Plain", "Extra"], errors: [], dir: "", default: "plain.strip4" });
    render(<Frames />);
    await screen.findByText("plain.strip4");
    expect(document.querySelectorAll(".grid.templates .tpl")).toHaveLength(7);
    expect(screen.getByText("rose-pink.strip4")).toBeTruthy();
    expect(screen.getByText("film-strip.strip4")).toBeTruthy();
    expect(screen.queryByText("extra0.strip4")).toBeNull();
    expect(within(screen.getAllByRole("combobox", { name: "Layout" })[0]).getAllByRole("option")).toHaveLength(9);
    fireEvent.click(screen.getByRole("checkbox", { name: "Show all built-in designs" }));
    expect(document.querySelectorAll(".grid.templates .tpl")).toHaveLength(153);
    expect(screen.getByText("extra0.strip4")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Extra" }));
    expect(document.querySelectorAll(".grid.templates .tpl")).toHaveLength(9);
    fireEvent.click(screen.getByRole("checkbox", { name: "Show all built-in designs" }));
    expect(document.querySelectorAll(".grid.templates .tpl")).toHaveLength(7);
  });

  it("keeps custom, offered, favorite and preselected frames visible", async () => {
    const frames = [
      fr("plain.strip4", "plain", 4),
      { ...fr("previously-offered", "old", 4), enabled: true },
      { ...fr("favorite", "old", 4), enabled: false, favorite: true },
      { ...fr("my-custom", "custom", 4), enabled: false, builtin: false, editable: true },
      { ...fr("preselected", "old", 4), enabled: false },
      { ...fr("unused", "old", 4), enabled: false },
    ];
    h.S.settings = { templateId: "preselected" };
    h.api.listFrames.mockResolvedValue({ frames, categories: ["Plain"], errors: [], dir: "", default: "preselected" });
    render(<Frames />);
    await screen.findByText("previously-offered");
    for (const name of ["favorite", "my-custom", "preselected"]) expect(screen.getByText(name)).toBeTruthy();
    expect(screen.queryByText("unused")).toBeNull();
  });

  it("reorders adjacent visible frames without dropping hidden catalogue entries", async () => {
    const frames = [fr("plain.strip4", "plain", 4), { ...fr("hidden-built-in", "old", 4), enabled: false }, { ...fr("rose-pink.strip4", "rose-pink", 4), enabled: false }];
    h.api.listFrames.mockResolvedValue({ frames, categories: ["Plain"], errors: [], dir: "", default: "plain.strip4" });
    h.api.setFrameCatalogue.mockResolvedValue(undefined);
    render(<Frames />); await screen.findByText("rose-pink.strip4");
    fireEvent.click(screen.getByRole("button", { name: "Move rose-pink.strip4 earlier" }));
    await waitFor(() => expect(h.api.setFrameCatalogue).toHaveBeenCalledWith(["rose-pink.strip4", "hidden-built-in", "plain.strip4"], {}));
  });

  it("filters frames by name and persists ordering", async () => {
    h.api.listFrames.mockResolvedValue({ frames: [fr("plain.strip4", "plain", 4), fr("rose", "rose", 3)], categories: ["Plain"], errors: [], dir: "", default: "plain.strip4" });
    h.api.setFrameCatalogue.mockResolvedValue(undefined);
    render(<Frames />); await screen.findByText("rose");
    fireEvent.click(screen.getByRole("button", { name: "Move rose earlier" }));
    await waitFor(() => expect(h.api.setFrameCatalogue).toHaveBeenCalledWith(["rose", "plain.strip4"], {}));
    fireEvent.change(screen.getByLabelText("Search frames"), { target: { value: "rose" } });
    expect(screen.queryByText("plain.strip4")).toBeNull(); expect(screen.getByText("rose")).toBeTruthy();
  });
  it("warns before hiding the last offered frame", async () => {
    render(<Frames />); await screen.findByText("plain.strip4");
    fireEvent.click(screen.getByRole("checkbox", { name: /Offer plain.strip4/ }));
    expect(screen.getByText(/No frames will be available/)).toBeTruthy();
    expect(h.api.setFrameEnabled).not.toHaveBeenCalled();
  });
  it("reports partial bulk failures and reloads the catalogue", async () => {
    h.api.listFrames.mockResolvedValue({ frames: [fr("plain.strip4", "plain", 4), fr("rose", "rose", 3), { ...fr("other", "other", 2), category: "Other" }], categories: ["Plain", "Other"], errors: [], dir: "", default: "plain.strip4" });
    h.api.setFrameEnabled.mockImplementation((id: string) => id === "rose" ? Promise.reject(new Error("fail")) : Promise.resolve());
    render(<Frames />); await screen.findByText("rose");
    fireEvent.click(screen.getByRole("button", { name: "Plain" }));
    fireEvent.click(screen.getByRole("button", { name: "Hide all shown" }));
    await waitFor(() => expect(screen.getByRole("alert").textContent).toMatch(/Updated 1 frames; failed: rose/i));
    expect(h.api.listFrames.mock.calls.length).toBeGreaterThan(1);
  });

});
