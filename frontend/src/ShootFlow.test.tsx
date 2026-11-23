import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Frame, FrameChoice, ShootSnapshot } from "./types";

const h = vi.hoisted(() => ({
  api: { startPreview: vi.fn(), startShooting: vi.fn(), cancelShooting: vi.fn(), resetShooting: vi.fn(), recoverShooting: vi.fn(), retake: vi.fn(), accept: vi.fn(), listFrames: vi.fn(), setSessionFrame: vi.fn(), composeSession: vi.fn(), printResult: vi.fn() },
  S: {} as Record<string, unknown>,
}));
vi.mock("./state", () => ({ useApp: () => h.S }));
vi.mock("./api", () => ({ api: h.api, toAppError: (e: unknown) => Object.assign(new Error(e instanceof Error ? e.message : String(e)), { action: (e as { action?: string })?.action ?? "" }) }));
vi.mock("./SourceChip", () => ({ SourceChip: () => null }));
vi.mock("./PrintPanel", () => ({ PrintPanel: () => <div data-testid="print-panel" /> }));
vi.mock("./ui", async (orig) => ({ ...(await orig<typeof import("./ui")>()), LivePreview: () => <div data-testid="live" />, SimulatedRibbon: () => null }));
import { ShootFlow } from "./ShootFlow";
import { sessionGate } from "./sessionGate";

const FRAME: FrameChoice = { id: "four", name: "Four", variantId: "", caption: "", photos: 4, format: "2×6 strip" };
const snap = (state: ShootSnapshot["state"], o: Partial<ShootSnapshot> = {}): ShootSnapshot =>
  ({ state, version: 1, sessionId: "S1", shots: 4, slot: 0, remaining: 0, taken: [], simulated: false, ...o });
const taken = (n: number) => Array.from({ length: n }, (_, i) => ({ slot: i, mediaId: `m${i}`, simulated: false }));
const fr = (id: string, photos: number): Frame => ({ id, name: id, kind: "strip", category: "Classic", order: 1, revision: 1, enabled: true, widthIn: 2, heightIn: 6, dpi: 300, slots: [], background: "#fff", photos, format: "2×6 strip", pixelW: 600, pixelH: 1800 });

beforeEach(() => {
  sessionGate._forget();
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
    fireEvent.click(screen.getByRole("button", { name: /Start shooting/ }));
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
      h.api.recoverShooting.mockClear();   // (mounting directly in REVIEW counts as a leftover state; clear that call)
      fireEvent.click(screen.getByRole("radio", { name: /other4/ }));
      fireEvent.click(screen.getByRole("button", { name: "Use this design" }));
      await waitFor(() => expect(h.api.setSessionFrame).toHaveBeenCalledWith("S1", "other4", "", "", false));
      expect(h.api.retake).not.toHaveBeenCalled();
      expect(h.api.recoverShooting).not.toHaveBeenCalled();
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
      await waitFor(() => expect(h.api.recoverShooting).toHaveBeenCalled());
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
      expect((screen.getByRole("button", { name: "Sent to printer" }) as HTMLButtonElement).disabled).toBe(true);
      expect(screen.getByText(/another copy, please ask the operator/i)).toBeTruthy();
    });

    it("Booth: a double tap cannot submit twice", async () => {
      let finish!: (value: { id: string; status: string; detail: string }) => void;
      h.api.printResult.mockImplementation(() => new Promise((resolve) => { finish = resolve; }));
      render(<ShootFlow variant="booth" frame={FRAME} />);
      const button = screen.getByRole("button", { name: "Print" });
      fireEvent.click(button); fireEvent.click(button);
      expect(h.api.printResult).toHaveBeenCalledTimes(1);
      finish({ id: "P1", status: "queued", detail: "" });
      await waitFor(() => expect(screen.getByRole("button", { name: "Sent to printer" })).toBeTruthy());
    });

    it("Booth: a printing problem is shown with what to do, not swallowed", async () => {
      h.api.printResult.mockRejectedValue(Object.assign(new Error("Printing is not set up."), { action: "Ask the host to choose a printer in Studio → Printer." }));
      render(<ShootFlow variant="booth" frame={FRAME} />);
      fireEvent.click(screen.getByRole("button", { name: /Print/ }));
      await waitFor(() => expect(screen.getByRole("alert").textContent).toMatch(/ask the operator/i));
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

  describe("finishing and starting again", () => {
    it("Finish returns to the caller at once, without waiting for the camera to stop", async () => {
      h.S.shoot = snap("COMPLETED", { taken: taken(4), resultId: "R1" });
      h.api.recoverShooting.mockReturnValue(new Promise(() => {}));     // the camera takes forever to stop
      const onDone = vi.fn();
      render(<ShootFlow variant="booth" frame={FRAME} onDone={onDone} />);
      fireEvent.click(screen.getByRole("button", { name: "Finish" }));
      expect(onDone).toHaveBeenCalledTimes(1);
    });

    it("a reset that fails does not leave the guest on the result screen", () => {
      h.S.shoot = snap("COMPLETED", { taken: taken(4), resultId: "R1" });
      h.api.recoverShooting.mockRejectedValue(new Error("boom"));
      const onDone = vi.fn();
      render(<ShootFlow variant="booth" frame={FRAME} onDone={onDone} />);
      fireEvent.click(screen.getByRole("button", { name: "Finish" }));
      expect(onDone).toHaveBeenCalledTimes(1);
    });

    it("opening a new session while the old 'All done' state is still there clears it first", async () => {
      h.S.shoot = snap("COMPLETED", { taken: taken(4), resultId: "OLD" });
      render(<ShootFlow variant="booth" frame={FRAME} />);
      await waitFor(() => expect(h.api.recoverShooting).toHaveBeenCalledTimes(1)); // one coordinated reset, not a cancel followed by a reset
    });

    it("a normal start (idle or preview) does not reset anything", () => {
      h.S.shoot = snap("PREVIEW");
      render(<ShootFlow variant="booth" frame={FRAME} />);
      expect(h.api.recoverShooting).not.toHaveBeenCalled();
    });
  });
});

describe("UX-1: guest flow safety and clarity", () => {
  it("Cancel during the countdown asks first; 'Keep going' changes nothing; 'Start over' closes it through the one reset", async () => {
    const view = render(<ShootFlow variant="booth" frame={FRAME} />);      // opened in PREVIEW, the normal way in
    h.S.shoot = snap("COUNTDOWN", { remaining: 2, slot: 1, taken: taken(1) });
    view.rerender(<ShootFlow variant="booth" frame={FRAME} />);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(await screen.findByRole("dialog", { name: "Leave this photo session?" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Keep going" }));
    expect(h.api.recoverShooting).not.toHaveBeenCalled();
    expect(h.api.cancelShooting).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    fireEvent.click(await screen.findByRole("button", { name: "Start over" }));
    await waitFor(() => expect(h.api.recoverShooting).toHaveBeenCalledTimes(1));
  });

  it("'Start over' in review also asks, and says what happens to the photos", async () => {
    const view = render(<ShootFlow variant="booth" frame={FRAME} />);
    h.S.shoot = snap("REVIEW", { taken: taken(4) });
    view.rerender(<ShootFlow variant="booth" frame={FRAME} />);
    fireEvent.click(screen.getByRole("button", { name: "Start over" }));
    const dlg = await screen.findByRole("dialog", { name: "Leave this photo session?" });
    expect(dlg.textContent).toMatch(/4 photos/);
    expect(dlg.textContent).toMatch(/originals/);
    expect(h.api.recoverShooting).not.toHaveBeenCalled();
  });

  it("shows which photo is being taken: 'Photo 2 of 4' with one dot per photo", () => {
    h.S.shoot = snap("COUNTDOWN", { remaining: 3, slot: 1, taken: taken(1) });
    render(<ShootFlow variant="booth" frame={FRAME} />);
    const p = screen.getByRole("progressbar", { name: "Photo progress" });
    expect(p.textContent).toContain("Photo 2 of 4");
    expect(p.getAttribute("aria-valuenow")).toBe("2");
    expect(p.querySelectorAll("li")).toHaveLength(4);
    expect(p.querySelectorAll("li.done")).toHaveLength(1);
    expect(p.querySelectorAll("li.now")).toHaveLength(1);
    expect(screen.getByText("3")).toBeTruthy(); // the countdown number
  });

  it("the capture flash and countdown never take pointer events or block the controls", () => {
    h.S.shoot = snap("CAPTURING", { slot: 0 });
    render(<ShootFlow variant="booth" frame={FRAME} />);
    expect(screen.getByText("Smile!")).toBeTruthy();
    expect(screen.getByText(/Hold still/)).toBeTruthy();
  });

  it("rapid Finish taps call Finish once and disable the button", () => {
    h.S.shoot = snap("COMPLETED", { taken: taken(4), resultId: "R1" });
    const onDone = vi.fn();
    render(<ShootFlow variant="booth" frame={FRAME} onDone={onDone} />);
    const fin = screen.getByRole("button", { name: "Finish" }) as HTMLButtonElement;
    fireEvent.click(fin); fireEvent.click(fin); fireEvent.click(fin);
    expect(onDone).toHaveBeenCalledTimes(1);
    expect((screen.getByRole("button", { name: "Finish" }) as HTMLButtonElement).disabled).toBe(true);
  });

  describe("the camera is lost during a session", () => {
    beforeEach(() => { h.S.camera = { device: { id: "c", name: "C" }, state: "connected", reconnecting: true, liveView: false }; });

    it("guests get a calm reconnection card (no raw technical text) and their photos are said to be safe", () => {
      h.S.shoot = snap("COUNTDOWN", { remaining: 2, taken: taken(1), slot: 1 });
      render(<ShootFlow variant="booth" frame={FRAME} />);
      const card = screen.getByRole("status");
      expect(card.textContent).toMatch(/Reconnecting the camera/);
      expect(card.textContent).toMatch(/photos are safe/);
      expect(document.body.textContent).not.toMatch(/disconnected|USB|EDSDK|error/i);
    });

    it("cannot start a session or retake while reconnecting", () => {
      render(<ShootFlow variant="booth" frame={FRAME} />);
      expect((screen.getByRole("button", { name: /Start shooting/ }) as HTMLButtonElement).disabled).toBe(true);
      cleanup();
      h.S.shoot = snap("REVIEW", { taken: taken(2), shots: 2 });
      render(<ShootFlow variant="booth" frame={FRAME} />);
      for (const b of screen.getAllByRole("button", { name: /Retake/ })) expect((b as HTMLButtonElement).disabled).toBe(true);
    });

    it("Studio keeps the operator informed that photos already taken are kept", () => {
      render(<ShootFlow variant="studio" frame={FRAME} />);
      expect(screen.getByText(/Photos already taken are kept/)).toBeTruthy();
    });
  });

  it("a camera error leads to a recovery screen, not a dead end: friendly text and one big 'Try again'", async () => {
    h.S.shoot = snap("ERROR", { error: "EDSDK error 0x00008D01 raw", errorAction: "Check focus" });
    render(<ShootFlow variant="booth" frame={FRAME} />);
    expect(screen.getByText(/couldn’t take that photo/)).toBeTruthy();
    expect(screen.getByText(/other photos are safe/)).toBeTruthy();
    expect(document.body.textContent).not.toMatch(/EDSDK|0x00008D01/);
    h.api.recoverShooting.mockClear();
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    await waitFor(() => expect(h.api.recoverShooting).toHaveBeenCalledTimes(1));
  });

  it("Studio still shows the real error and its action", () => {
    h.S.shoot = snap("ERROR", { error: "Autofocus failed", errorAction: "Check focus" });
    render(<ShootFlow variant="studio" frame={FRAME} />);
    expect(screen.getByText("Autofocus failed")).toBeTruthy();
    expect(screen.getByText("Check focus")).toBeTruthy();
  });

  it("a failed action in Booth is explained kindly, without the raw message", async () => {
    h.api.startShooting.mockRejectedValue(Object.assign(new Error("Cannot write to C:\\Docs: access is denied"), { action: "Choose another media folder." }));
    render(<ShootFlow variant="booth" frame={FRAME} />);
    fireEvent.click(screen.getByRole("button", { name: /Start shooting/ }));
    await screen.findByText(/That didn’t work/);
    expect(document.body.textContent).not.toMatch(/access is denied/);
  });
});
