import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { CameraStatus, FrameChoice, ShootSnapshot } from "../../../shared/types";

vi.mock("../ui", () => ({ LivePreview: (p: { fit?: string; objectPosition?: string }) => <div data-testid="live" data-fit={p.fit ?? "contain"} data-position={p.objectPosition ?? "50% 50%"} /> }));
import { GuestShoot, type GuestShootProps } from "./GuestShoot";
import { Welcome, type WelcomeProps } from "./Welcome";
import { StepIndicator } from "./parts";

afterEach(cleanup);
// the project has no @types/node; the tests run in node, so read the stylesheet from disk
// @ts-ignore
const { readFileSync } = (await import("node:fs")) as { readFileSync: (p: string, e: string) => string };
const css = readFileSync(`${(globalThis as unknown as { process: { cwd(): string } }).process.cwd()}/src/features/booth/guest/guest.css`, "utf8");

// ---- design tokens: every text/background pair the guest UI relies on must meet WCAG AA ----
const token = (name: string) => css.match(new RegExp(`--${name}:\\s*(#[0-9a-fA-F]{6})`))![1];
const lum = (hex: string) => {
  const c = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255).map((v) => (v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4));
  return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2];
};
const ratio = (a: string, b: string) => { const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p); return (x + 0.05) / (y + 0.05); };

describe("design tokens", () => {
  it.each([
    ["g-text", "g-bg"], ["g-text", "g-elev"], ["g-text", "g-elev-2"], ["g-muted", "g-bg"], ["g-muted", "g-elev"], ["g-muted", "g-surface"],
    ["g-on-accent", "g-rose"], ["g-on-accent", "g-primary"], ["g-warn", "g-bg"], ["g-err", "g-bg"],
  ])("%s on %s reaches 4.5:1", (fg, bg) => { expect(ratio(token(fg), token(bg))).toBeGreaterThanOrEqual(4.5); });
  it("white on the selected-chip purple reaches 4.5:1", () => { expect(ratio("#ffffff", token("g-primary-deep"))).toBeGreaterThanOrEqual(4.5); });
  it("keeps the ApexVoid palette", () => {
    expect(token("g-bg").toLowerCase()).toBe("#120d23"); expect(token("g-elev").toLowerCase()).toBe("#241a40");
    expect(token("g-primary").toLowerCase()).toBe("#9275e8"); expect(token("g-rose").toLowerCase()).toBe("#e8a0c3");
  });
});

describe("guest stylesheet guarantees", () => {
  it("decorative and animated overlays never take touches", () => {
    for (const sel of [".g-art", ".g-count", ".g-flash", ".g-smile", ".g-confetti", ".g-more"]) {
      const rule = css.split("\n").find((l) => l.startsWith(`${sel} {`)) ?? "";
      expect(rule, sel).toMatch(/pointer-events: none/);
    }
    expect(css).toMatch(/\.g-stage \.steps \{[^}]*pointer-events: none/);
  });
  it("switches all motion off for prefers-reduced-motion", () => {
    expect(css).toMatch(/@media \(prefers-reduced-motion: reduce\)[\s\S]*animation: none !important[\s\S]*transition: none !important/);
  });
  it("never blurs what is behind the live preview", () => {
    expect(css).not.toMatch(/backdrop-filter:\s*blur/);
  });
  it("has a visible keyboard focus ring and is scoped to the guest root", () => {
    expect(css).toMatch(/:focus-visible \{ outline: 4px solid var\(--g-focus\)/);
    for (const l of css.split("\n")) if (/^\.(g-|booth-root)/.test(l) === false && /^[.#a-z]/.test(l) && l.includes("{") && !l.startsWith("@") && !l.startsWith("  ") && !l.startsWith("/*")) throw new Error(`unscoped rule: ${l.slice(0, 60)}`);
  });
});

// ---- Screen A ----
const W = (o: Partial<WelcomeProps> = {}): WelcomeProps => ({ blocked: false, resetting: false, guestText: "", readiness: null, resetFailed: false, resetError: "", onStart: vi.fn(), onRetryReset: vi.fn(), ...o });
describe("Welcome", () => {
  it("ready: one primary action, branding, no Video option", () => {
    const p = W(); render(<Welcome {...p} />);
    expect(screen.getByText("Say cheese!")).toBeTruthy();
    expect(screen.getByText("Photobooth", { exact: false })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Take photos/ }));
    expect(p.onStart).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("button", { name: /video/i })).toBeNull();
    expect(screen.getAllByRole("button")).toHaveLength(1);
  });
  it("renders the welcome action in Vietnamese when requested", () => {
    render(<Welcome {...W({ language: "vi" })} />);
    expect(screen.getByText("Cười lên nào!")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Chụp ảnh" })).toBeTruthy();
  });
  it("blocked: the reason is on screen and the button is disabled", () => {
    const p = W({ blocked: true, guestText: "The photo booth is not ready yet. Please ask the host.", readiness: { status: "setup_required", canStart: false, headline: "Setup required", guest: "", items: [], at: "" } });
    render(<Welcome {...p} />);
    expect((screen.getByRole("button", { name: /Take photos/ }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByRole("status").textContent).toMatch(/ask the host/);
    expect(screen.getByText(/Host: Setup required/)).toBeTruthy();
  });
  it("resetting: shows a wait state and cannot be pressed", () => {
    render(<Welcome {...W({ resetting: true })} />);
    expect((screen.getByRole("button", { name: /One moment/ }) as HTMLButtonElement).disabled).toBe(true);
  });
  it("a failed tidy-up offers Try again", () => {
    const p = W({ resetFailed: true, resetError: "stuck" }); render(<Welcome {...p} />);
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(p.onRetryReset).toHaveBeenCalled();
    expect(screen.getByRole("alert").textContent).toMatch(/still tidying up/);
  });
  it("decorative artwork is hidden from assistive tech", () => {
    const { container } = render(<Welcome {...W()} />);
    expect(container.querySelector(".g-art")?.getAttribute("aria-hidden")).toBe("true");
  });
});

// ---- Screens C–F ----
const FRAME: FrameChoice = { id: "four", name: "Four", variantId: "", caption: "", photos: 4, format: "2×6 strip" };
const snap = (state: ShootSnapshot["state"], o: Partial<ShootSnapshot> = {}): ShootSnapshot => ({ state, version: 1, sessionId: "S", shots: 4, slot: 0, remaining: 0, taken: [], simulated: false, ...o });
const taken = (n: number) => Array.from({ length: n }, (_, i) => ({ slot: i, mediaId: `m${i}`, simulated: false }));
const cam = (o: Partial<CameraStatus> = {}): CameraStatus => ({ state: "liveview", liveView: true, reconnecting: false, drivers: [], device: { id: "c", name: "C", driver: "mock", simulated: true, kind: "simulated" }, ...o });
const G = (s: ShootSnapshot, o: Partial<GuestShootProps> = {}): GuestShootProps => ({
  s, camera: cam(), frame: FRAME, countdownSeconds: 3, busy: false, error: false, onClearError: vi.fn(), slow: false,
  mediaUrl: (id) => `/m/${id}`, frameUrl: (id) => `/f/${id}`, finishing: false, print: null,
  onStart: vi.fn(), onChangeFrame: vi.fn(), onRetake: vi.fn(), onAccept: vi.fn(), onCancel: vi.fn(), onChangeDesign: vi.fn(), onFinish: vi.fn(), onRetryPreview: vi.fn(), onReset: vi.fn(), ...o,
});

describe("live preview", () => {
  it("shows the camera, the chosen frame, the photo count and one Start action", () => {
    const p = G(snap("PREVIEW")); render(<GuestShoot {...p} />);
    expect(screen.getByTestId("live")).toBeTruthy();
    expect(screen.getByLabelText("Frame: Four")).toBeTruthy();
    expect(screen.getByText("Get ready!")).toBeTruthy();
    expect(screen.getByText(/3-second countdown/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Start shooting/ }));
    expect(p.onStart).toHaveBeenCalledTimes(1);
  });
  it("Start is disabled without live view", () => {
    render(<GuestShoot {...G(snap("PREVIEW"), { camera: cam({ liveView: false }) })} />);
    expect((screen.getByRole("button", { name: /Start shooting/ }) as HTMLButtonElement).disabled).toBe(true);
  });
  it("frames the upcoming shot to its frame slot and focus", () => {
    const frame = { ...FRAME, photoSlots: [
      { photo: 0, w: 0.4, h: 0.8, focusX: 0.25, focusY: 0.75 },
      { photo: 1, w: 0.8, h: 0.4, focusX: 0.6, focusY: 0.2 },
    ] };
    const { container, rerender } = render(<GuestShoot {...G(snap("PREVIEW"), { frame })} />);
    expect(container.querySelector(".g-shot-target")?.getAttribute("style")).toContain("--shot-ar: 0.5");
    expect(screen.getByTestId("live").getAttribute("data-fit")).toBe("cover");
    expect(screen.getByTestId("live").getAttribute("data-position")).toBe("25% 75%");
    rerender(<GuestShoot {...G(snap("COUNTDOWN", { slot: 1, remaining: 2 }), { frame })} />);
    expect(container.querySelector(".g-shot-target")?.getAttribute("style")).toContain("--shot-ar: 2");
    expect(screen.getByTestId("live").getAttribute("data-position")).toBe("60% 20%");
  });
  it("uses centered framing when a legacy frame has no slot geometry", () => {
    render(<GuestShoot {...G(snap("PREVIEW"))} />);
    expect(screen.getByTestId("live").getAttribute("data-fit")).toBe("contain");
    expect(document.querySelector(".g-shot-target")).toBeNull();
  });
  it("no camera: a plain message, never a blank screen", () => {
    render(<GuestShoot {...G(snap("PREVIEW"), { camera: cam({ device: undefined }) })} />);
    expect(screen.getByText(/camera is not ready/)).toBeTruthy();
  });
});

describe("countdown and capture feedback", () => {
  it("mirrors the engine: the number and the photo progress come from the snapshot", () => {
    const { rerender } = render(<GuestShoot {...G(snap("COUNTDOWN", { slot: 1, remaining: 2, taken: taken(1) }))} />);
    expect(screen.getByText("2")).toBeTruthy();
    expect(screen.getByText("Photo 2 of 4")).toBeTruthy();
    rerender(<GuestShoot {...G(snap("COUNTDOWN", { slot: 1, remaining: 1, taken: taken(1) }))} />);
    expect(screen.getByText("1")).toBeTruthy();
    expect(screen.queryByText("2")).toBeNull();
  });
  it("the shutter flash is decorative and only exists while the engine is CAPTURING", () => {
    const { container, rerender } = render(<GuestShoot {...G(snap("COUNTDOWN", { remaining: 1 }))} />);
    expect(container.querySelector(".g-flash")).toBeNull();
    rerender(<GuestShoot {...G(snap("CAPTURING"))} />);
    expect(container.querySelector(".g-flash")?.getAttribute("aria-hidden")).toBe("true");
    expect(screen.getByText("Smile!")).toBeTruthy();
    expect(screen.getByText(/Hold still/)).toBeTruthy();
  });
  it("a retake is named as one", () => {
    render(<GuestShoot {...G(snap("COUNTDOWN", { slot: 2, remaining: 3, taken: taken(4) }))} />);
    expect(screen.getByText("Retaking photo 3")).toBeTruthy();
  });
  it("Cancel asks first (handler only; the dialog is UX-1's)", () => {
    const p = G(snap("COUNTDOWN", { remaining: 3 })); render(<GuestShoot {...p} />);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(p.onCancel).toHaveBeenCalled();
  });
  it("camera loss keeps the calm message and blocks Start", () => {
    render(<GuestShoot {...G(snap("PREVIEW"), { camera: cam({ reconnecting: true }), slow: true })} />);
    expect(screen.getByRole("status").textContent).toMatch(/Reconnecting the camera.*photos are safe.*ask the host/);
    expect((screen.getByRole("button", { name: /Start shooting/ }) as HTMLButtonElement).disabled).toBe(true);
  });
});

describe("review", () => {
  it("renders review and retake actions in Vietnamese", () => {
    render(<GuestShoot {...G(snap("REVIEW", { taken: taken(1) }), { language: "vi" })} />);
    expect(screen.getByText("Bạn thấy ảnh thế nào?")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Chụp lại ảnh 1" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Ảnh đẹp rồi" })).toBeTruthy();
  });
  it("lists photos in capture order and each Retake names its photo", () => {
    const p = G(snap("REVIEW", { taken: [...taken(4)].reverse() })); render(<GuestShoot {...p} />);
    const imgs = screen.getAllByRole("img").filter((i) => /^Photo \d$/.test(i.getAttribute("alt") ?? "")).map((i) => i.getAttribute("alt"));
    expect(imgs).toEqual(["Photo 1", "Photo 2", "Photo 3", "Photo 4"]);
    fireEvent.click(screen.getByRole("button", { name: "Retake photo 3" }));
    expect(p.onRetake).toHaveBeenCalledWith(2);
    expect(p.onRetake).toHaveBeenCalledTimes(1);
  });
  it("Retake is disabled while busy or reconnecting (no double submit)", () => {
    render(<GuestShoot {...G(snap("REVIEW", { taken: taken(2), shots: 2 }), { busy: true })} />);
    for (const b of screen.getAllByRole("button", { name: /Retake photo/ })) expect((b as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole("button", { name: "Looks good" }) as HTMLButtonElement).disabled).toBe(true);
  });
  it("has one dominant action plus design and start-over, and shows the frame summary", () => {
    const p = G(snap("REVIEW", { taken: taken(4) })); render(<GuestShoot {...p} />);
    expect(screen.getByLabelText("Frame: Four")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Change design" })); expect(p.onChangeDesign).toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Start over" })); expect(p.onCancel).toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Looks good" })); expect(p.onAccept).toHaveBeenCalled();
    expect(document.querySelectorAll(".g-primary")).toHaveLength(1);
  });
  it("tile columns follow the photo count rather than assuming four", () => {
    const { container } = render(<GuestShoot {...G(snap("REVIEW", { shots: 6, taken: taken(6) }))} />);
    const st = (container.querySelector(".g-photos") as HTMLElement).style;
    expect(st.getPropertyValue("--cl")).toBe("3"); expect(st.getPropertyValue("--rl")).toBe("2"); expect(st.getPropertyValue("--cp")).toBe("2"); expect(st.getPropertyValue("--rp")).toBe("3");
  });
});

describe("result", () => {
  it("shows the finished image, a clear headline, Finish as the primary and Change design as secondary", () => {
    const p = G(snap("COMPLETED", { taken: taken(4), resultId: "R" }), { resultId: "R", print: <button>Print</button> }); render(<GuestShoot {...p} />);
    expect(screen.getByAltText("Your photo strip").getAttribute("src")).toBe("/m/R");
    expect(screen.getByText("All done!")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Finish" })); expect(p.onFinish).toHaveBeenCalled();
    expect(document.querySelector(".g-primary")?.textContent).toMatch(/Finish/);
    expect(screen.getByRole("button", { name: "Change design" })).toBeTruthy();
  });
  it("Finish is disabled once pressed", () => {
    render(<GuestShoot {...G(snap("COMPLETED"), { resultId: "R", finishing: true })} />);
    expect((screen.getByRole("button", { name: "Finish" }) as HTMLButtonElement).disabled).toBe(true);
  });
  it("digital-only sessions show no print control", () => {
    render(<GuestShoot {...G(snap("COMPLETED"), { resultId: "R" })} />);
    expect(screen.queryByRole("button", { name: /print/i })).toBeNull();
  });
  it("the celebration is decorative", () => {
    const { container } = render(<GuestShoot {...G(snap("COMPLETED"), { resultId: "R" })} />);
    expect(container.querySelector(".g-confetti")?.getAttribute("aria-hidden")).toBe("true");
  });
});

describe("step indicator", () => {
  it("announces the step and marks the current one", () => {
    render(<StepIndicator step={3} />);
    expect(screen.getByRole("list", { name: "Step 3 of 4: Review" })).toBeTruthy();
    expect(document.querySelectorAll("li.done")).toHaveLength(2);
    expect(document.querySelector("li.now")?.getAttribute("aria-current")).toBe("step");
  });
});

it("renders safe localized capture warnings without diagnostic paths", () => {
  render(<GuestShoot {...G(snap("COMPLETED", { warning: "disk failure C:\\private\\photos", warningCode: "composition_failed" }), { language: "vi" })} />);
  expect(screen.getByText(/Ảnh đã được lưu nhưng chưa thể tạo dải ảnh/)).toBeTruthy();
  expect(document.body.textContent).not.toContain("C:\\private");
});
