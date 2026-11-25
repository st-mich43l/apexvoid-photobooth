import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { explainFit, filterFrames, fitFor, photoCounts } from "../../shared/frameInfo";
import type { Frame } from "../../shared/types";

const h = vi.hoisted(() => ({ api: { listFrames: vi.fn() }, S: { settings: { templateId: "" }, frameUrl: (id: string, v: string, w: number, c = "") => `/f/${id}/${v || "-"}/${w}/${c}` } }));
vi.mock("../../app/state", () => ({ useApp: () => h.S }));
vi.mock("../../shared/api", () => ({ api: h.api, toAppError: (e: unknown) => ({ message: e instanceof Error ? e.message : String(e), action: "" }) }));
import { FrameSelection } from "./FrameSelection";

const f = (id: string, photos: number, over: Partial<Frame> = {}): Frame => ({
  id, name: id, kind: "strip", category: "Classic", order: 1, revision: 1, enabled: true, widthIn: 2, heightIn: 6, dpi: 300, slots: [], background: "#fff",
  photos, format: "2×6 strip", pixelW: 600, pixelH: 1800, ...over,
});
const FRAMES = [f("four", 4, { variants: [{ id: "x", name: "Rose" }, { id: "y", name: "Blue" }] }), f("three", 3), f("six", 6, { category: "Special", format: "4×6 sheet" }), f("cap", 4, { captionEnabled: true })];
beforeEach(() => { h.api.listFrames.mockResolvedValue({ frames: FRAMES, categories: ["Classic", "Special"], errors: [], dir: "", default: "four" }); });
afterEach(cleanup);

describe("frame helpers", () => {
  it("filters by category and photo count", () => {
    expect(filterFrames(FRAMES, { category: "Special", photos: 0 }).map((x) => x.id)).toEqual(["six"]);
    expect(filterFrames(FRAMES, { category: "All", photos: 4 }).map((x) => x.id)).toEqual(["four", "cap"]);
    expect(photoCounts(FRAMES)).toEqual([3, 4, 6]);
  });
  it("classifies a frame against photos already taken", () => {
    expect(fitFor(FRAMES[0], 4).kind).toBe("ok");
    expect(fitFor(FRAMES[0], undefined).kind).toBe("ok");
    expect(fitFor(FRAMES[1], 4)).toEqual({ kind: "fewer", need: 3, have: 4 });
    expect(fitFor(FRAMES[2], 4)).toEqual({ kind: "more", need: 6, have: 4 });
    expect(explainFit(fitFor(FRAMES[2], 4), "Six")).toMatch(/needs 6 photos.*you took 4/);
    expect(explainFit(fitFor(FRAMES[1], 4), "Three")).toMatch(/uses 3 photos.*first 3/);
  });
});

describe("FrameSelection", () => {
  const open = async (props: Partial<React.ComponentProps<typeof FrameSelection>> = {}) => {
    const onConfirm = vi.fn(), onBack = vi.fn();
    render(<FrameSelection variant="booth" onConfirm={onConfirm} onBack={onBack} {...props} />);
    await screen.findByRole("radio", { name: /four/ });
    return { onConfirm, onBack };
  };

  it("preselects the default frame and confirms it with no colour change", async () => {
    const { onConfirm } = await open();
    expect(screen.getByRole("radio", { name: /four/ }).getAttribute("aria-checked")).toBe("true");
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    expect(onConfirm).toHaveBeenCalledWith(expect.objectContaining({ id: "four", variantId: "", photos: 4 }), { useFirst: false, restart: false });
  });

  it("a colour choice changes the preview and is part of the confirmed choice", async () => {
    const { onConfirm } = await open();
    fireEvent.click(screen.getByRole("radio", { name: "Blue" }));
    expect((screen.getByAltText("four preview") as HTMLImageElement).getAttribute("src")).toContain("/f/four/y/720");
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    expect(onConfirm.mock.calls[0][0]).toMatchObject({ id: "four", variantId: "y" });
  });

  it("switching frame resets the colour; caption only for frames that allow it", async () => {
    const { onConfirm } = await open();
    expect(screen.queryByLabelText("Caption")).toBeNull();
    fireEvent.click(screen.getByRole("radio", { name: /cap/ }));
    fireEvent.change(screen.getByLabelText("Caption"), { target: { value: " Mai & Nam " } });
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    expect(onConfirm.mock.calls[0][0]).toMatchObject({ id: "cap", caption: "Mai & Nam" });
  });

  it("Back is available and nothing is confirmed", async () => {
    const { onBack, onConfirm } = await open();
    fireEvent.click(screen.getByRole("button", { name: /Back/ }));
    expect(onBack).toHaveBeenCalled();
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("after capture: a frame for fewer photos asks to use the first N, never silently", async () => {
    const { onConfirm } = await open({ takenPhotos: 4 });
    fireEvent.click(screen.getByRole("radio", { name: /three/ }));
    expect(screen.getByText(/uses 3 photos, but you took 4/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Continue" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Use the first 3 photos" }));
    expect(onConfirm).toHaveBeenCalledWith(expect.objectContaining({ id: "three" }), { useFirst: true, restart: false });
  });

  it("after capture: a frame needing more photos offers to start over", async () => {
    const { onConfirm } = await open({ takenPhotos: 4 });
    fireEvent.click(screen.getByRole("radio", { name: /six/ }));
    expect(screen.getByText(/needs 6 photos, but you took 4/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Start over with this frame" }));
    expect(onConfirm).toHaveBeenCalledWith(expect.objectContaining({ id: "six" }), { useFirst: false, restart: true });
  });

  it("shows a loading state, then an error with retry", async () => {
    h.api.listFrames.mockReturnValueOnce(new Promise(() => {}));
    render(<FrameSelection variant="booth" onConfirm={() => {}} onBack={() => {}} />);
    expect(screen.getByRole("status").textContent).toMatch(/Loading/);
    cleanup();
    h.api.listFrames.mockRejectedValueOnce(new Error("boom"));
    render(<FrameSelection variant="booth" onConfirm={() => {}} onBack={() => {}} />);
    await waitFor(() => expect(screen.getByRole("alert").textContent).toMatch(/could not be loaded/));
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    await screen.findByRole("radio", { name: /four/ });
  });

  it("the Studio variant can omit Back", async () => {
    render(<FrameSelection variant="studio" onConfirm={() => {}} />);
    await screen.findByRole("radio", { name: /four/ });
    expect(screen.queryByRole("button", { name: /Back/ })).toBeNull();
  });

  describe("guest gallery (UX-2)", () => {
    it("shows the layout, a photo count and the colour choices on each card", async () => {
      await open();
      const card = screen.getByRole("radio", { name: /four/ });
      expect(card.textContent).toMatch(/2×6/); expect(card.textContent).toMatch(/4 photos/);
      expect(card.querySelectorAll(".g-colordots i")).toHaveLength(2);
      expect(card.querySelector("img")?.getAttribute("loading")).toBe("lazy");
    });
    it("keeps the selection visible: details stay, and the chosen card is scrolled into view after filtering", async () => {
      const spy = vi.fn(); Element.prototype.scrollIntoView = spy;
      await open();
      fireEvent.click(screen.getByRole("button", { name: "Special" }));
      expect(screen.getByLabelText("Selected frame").textContent).toMatch(/four/); // filtered out of the grid, still summarised
      fireEvent.click(screen.getByRole("button", { name: "Classic" }));
      expect(spy).toHaveBeenCalled();
    });
    it("names the selected colour and labels the swatches", async () => {
      await open();
      expect(screen.getByRole("radio", { name: "Rose" }).getAttribute("aria-checked")).toBe("true");
      fireEvent.click(screen.getByRole("radio", { name: "Blue" }));
      expect(document.querySelector(".g-swatch-name")?.textContent).toBe("Blue");
    });
    it("typing a caption switches to the keyboard-friendly layout and back", async () => {
      await open();
      fireEvent.click(screen.getByRole("radio", { name: /cap/ }));
      const root = document.querySelector(".g-frames")!;
      fireEvent.focus(screen.getByLabelText("Caption"));
      expect(root.getAttribute("data-typing")).toBe("true");
      fireEvent.keyDown(screen.getByLabelText("Caption"), { key: "Enter" });
    });
    it("Back stays available while frames load and when they fail", async () => {
      h.api.listFrames.mockReturnValue(new Promise(() => {}));
      const onBack = vi.fn();
      render(<FrameSelection variant="booth" onConfirm={() => {}} onBack={onBack} backLabel="Home" />);
      fireEvent.click(screen.getByRole("button", { name: /Home/ }));
      expect(onBack).toHaveBeenCalled();
    });
    it("Continue is the single primary action and is disabled while busy", async () => {
      await open({ busy: true });
      expect((screen.getByRole("button", { name: /Please wait/ }) as HTMLButtonElement).disabled).toBe(true);
      expect(document.querySelectorAll(".g-primary")).toHaveLength(1);
    });
  });
});
