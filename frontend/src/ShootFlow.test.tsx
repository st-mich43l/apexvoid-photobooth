import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Frame, FrameChoice, ShootSnapshot } from "./types";

const h = vi.hoisted(() => ({
  api: { startPreview: vi.fn(), startShooting: vi.fn(), cancelShooting: vi.fn(), resetShooting: vi.fn(), retake: vi.fn(), accept: vi.fn(), listFrames: vi.fn(), setSessionFrame: vi.fn(), composeSession: vi.fn(), printResult: vi.fn() },
  S: {} as Record<string, unknown>,
}));
vi.mock("./state", () => ({ useApp: () => h.S }));
vi.mock("./api", () => ({ api: h.api, toAppError: (e: unknown) => Object.assign(new Error(e instanceof Error ? e.message : String(e)), { action: (e as { action?: string })?.action ?? "" }) }));
vi.mock("./SourceChip", () => ({ SourceChip: () => null }));
vi.mock("./PrintPanel", () => ({ PrintPanel: () => <div data-testid="print-panel" /> }));
vi.mock("./ui", async (orig) => ({ ...(await orig<typeof import("./ui")>()), LivePreview: () => <div data-testid="live" />, SimulatedRibbon: () => null }));
import { ShootFlow } from "./ShootFlow";

const FRAME: FrameChoice = { id: "four", name: "Four", variantId: "", caption: "", photos: 4, format: "2×6 strip" };
const snap = (state: ShootSnapshot["state"], o: Partial<ShootSnapshot> = {}): ShootSnapshot =>
  ({ state, version: 1, sessionId: "S1", shots: 4, slot: 0, remaining: 0, taken: [], simulated: false, ...o });
const taken = (n: number) => Array.from({ length: n }, (_, i) => ({ slot: i, mediaId: `m${i}`, simulated: false }));
const fr = (id: string, photos: number): Frame => ({ id, name: id, kind: "strip", category: "Classic", order: 1, revision: 1, enabled: true, widthIn: 2, heightIn: 6, dpi: 300, slots: [], background: "#fff", photos, format: "2×6 strip", pixelW: 600, pixelH: 1800 });

beforeEach(() => {
  h.S = { shoot: snap("PREVIEW"), camera: { device: { id: "c", name: "C" }, state: "connected", reconnecting: false, liveView: true }, settings: { templateId: "", printerName: "Lab" },
    mediaUrl: (id: string) => `/m/${id}`, frameUrl: (id: string) => `/f/${id}`, toast: vi.fn() };
  for (const k of Object.keys(h.api)) (h.api as Record<string, ReturnType<typeof vi.fn>>)[k].mockReset().mockResolvedValue(undefined);
  h.api.listFrames.mockResolvedValue({ frames: [fr("four", 4), fr("three", 3), fr("six", 6), fr("other4", 4)], categories: ["Classic"], errors: [], dir: "", default: "four" });
});
afterEach(cleanup);

describe("ShootFlow with a pre-selected frame", () => {
  it("starts the session with the chosen frame and no separate photo-count control", () => {
    render(<ShootFlow variant="booth" frame={{ ...FRAME, variantId: "rose", caption: "Hi" }} />);
    expect(screen.queryByRole("radiogroup", { name: "Number of photographs" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /TAP TO START/ }));
    expect(h.api.startShooting).toHaveBeenCalledWith("four", "rose", "Hi");
  });

  it("shows the selected frame beside the live view and the photo count on the button", () => {
    render(<ShootFlow variant="booth" frame={FRAME} />);
    expect(screen.getByLabelText("Frame: Four")).toBeTruthy();
    expect(screen.getByRole("button", { name: /4 photos/ })).toBeTruthy();
    expect(screen.getByTestId("live")).toBeTruthy();
  });

  it("before shooting the guest can go back to choose another frame", () => {
    const onChangeFrame = vi.fn();
    render(<ShootFlow variant="booth" frame={FRAME} onChangeFrame={onChangeFrame} />);
    fireEvent.click(screen.getByRole("button", { name: "Change frame" }));
    expect(onChangeFrame).toHaveBeenCalledWith(FRAME);
    expect(h.api.startShooting).not.toHaveBeenCalled();
  });

  describe("changing the design after capture", () => {
    beforeEach(() => { h.S.shoot = snap("REVIEW", { taken: taken(4) }); });
    const open = async () => {
      render(<ShootFlow variant="booth" frame={FRAME} />);
      fireEvent.click(screen.getByRole("button", { name: "Change design" }));
      await screen.findByRole("radio", { name: /other4/ });
    };

    it("same photo count: switches the design of the session without any retake", async () => {
      await open();
      fireEvent.click(screen.getByRole("radio", { name: /other4/ }));
      fireEvent.click(screen.getByRole("button", { name: "Use this design" }));
      await waitFor(() => expect(h.api.setSessionFrame).toHaveBeenCalledWith("S1", "other4", "", "", false));
      expect(h.api.retake).not.toHaveBeenCalled();
      expect(h.api.cancelShooting).not.toHaveBeenCalled();
    });

    it("a frame for fewer photos needs an explicit 'use the first N'", async () => {
      await open();
      fireEvent.click(screen.getByRole("radio", { name: /three/ }));
      fireEvent.click(screen.getByRole("button", { name: "Use the first 3 photos" }));
      await waitFor(() => expect(h.api.setSessionFrame).toHaveBeenCalledWith("S1", "three", "", "", true));
    });

    it("a frame needing more photos restarts the session with that frame", async () => {
      await open();
      fireEvent.click(screen.getByRole("radio", { name: /six/ }));
      fireEvent.click(screen.getByRole("button", { name: "Start over with this frame" }));
      await waitFor(() => expect(h.api.resetShooting).toHaveBeenCalled());
      expect(h.api.cancelShooting).toHaveBeenCalled();
      expect(h.api.setSessionFrame).not.toHaveBeenCalled();
    });

    it("'Keep this design' closes the picker and changes nothing", async () => {
      await open();
      fireEvent.click(screen.getByRole("button", { name: /Keep this design/ }));
      expect(screen.queryByRole("dialog", { name: "Change design" })).toBeNull();
      expect(h.api.setSessionFrame).not.toHaveBeenCalled();
    });
  });

  it("after saving, a new design makes a second image from the same photos", async () => {
    h.S.shoot = snap("COMPLETED", { taken: taken(4), resultId: "R1" });
    h.api.composeSession.mockResolvedValue({ id: "R2" });
    render(<ShootFlow variant="booth" frame={FRAME} />);
    expect((screen.getByAltText("Your photo strip") as HTMLImageElement).src).toContain("/m/R1");
    fireEvent.click(screen.getByRole("button", { name: "Change design" }));
    await screen.findByRole("radio", { name: /other4/ });
    fireEvent.click(screen.getByRole("radio", { name: /other4/ }));
    fireEvent.click(screen.getByRole("button", { name: "Use this design" }));
    await waitFor(() => expect(h.api.composeSession).toHaveBeenCalledWith("S1", "other4", "", "", false));
    await waitFor(() => expect((screen.getByAltText("Your photo strip") as HTMLImageElement).src).toContain("/m/R2"));
  });

  describe("printing the result", () => {
    beforeEach(() => { h.S.shoot = snap("COMPLETED", { taken: taken(4), resultId: "R1" }); });

    it("Booth: one tap prints the finished image with the operator's printer and says it was only sent", async () => {
      h.api.printResult.mockResolvedValue({ id: "P1", status: "queued", detail: "Sent to the Windows print queue." });
      render(<ShootFlow variant="booth" frame={FRAME} />);
      fireEvent.click(screen.getByRole("button", { name: /Print/ }));
      await waitFor(() => expect(h.api.printResult).toHaveBeenCalledWith("R1", expect.any(String)));
      await waitFor(() => expect(screen.getByRole("status").textContent).toMatch(/Sent to the printer/));
      expect(screen.getByRole("button", { name: "Print another copy" })).toBeTruthy();
    });

    it("Booth: a printing problem is shown with what to do, not swallowed", async () => {
      h.api.printResult.mockRejectedValue(Object.assign(new Error("Printing is not set up."), { action: "Ask the host to choose a printer in Studio → Printer." }));
      render(<ShootFlow variant="booth" frame={FRAME} />);
      fireEvent.click(screen.getByRole("button", { name: /Print/ }));
      await waitFor(() => expect(screen.getByRole("alert").textContent).toMatch(/Ask the host/));
    });

    it("Booth: no print button when the operator has not set up a printer", () => {
      (h.S.settings as { printerName: string }).printerName = "";
      render(<ShootFlow variant="booth" frame={FRAME} />);
      expect(screen.queryByRole("button", { name: /Print/ })).toBeNull();
    });

    it("Studio: opens the full print dialog (placement preview) instead of printing blindly", () => {
      render(<ShootFlow variant="studio" frame={FRAME} />);
      fireEvent.click(screen.getByRole("button", { name: "Print…" }));
      expect(screen.getByTestId("print-panel")).toBeTruthy();
      expect(h.api.printResult).not.toHaveBeenCalled();
    });
  });
});
