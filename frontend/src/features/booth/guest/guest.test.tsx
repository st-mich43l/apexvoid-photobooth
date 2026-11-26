import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { CameraStatus, FrameChoice, ShootSnapshot } from "../../../shared/types";

vi.mock("../../../shared/ui", () => ({ LivePreview: (p: { fit?: string; objectPosition?: string; mediaStyle?: object }) => <div data-testid="live" data-fit={p.fit ?? "contain"} data-position={p.objectPosition ?? "50% 50%"} data-media={p.mediaStyle ? JSON.stringify(p.mediaStyle) : ""} /> }));
import { GuestShoot, type GuestShootProps } from "./GuestShoot";
import { usePhotoSelection, DOUBLE_TAP_MS, type PhotoSelection } from "../photoSelection";
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
  selection: stubSelection(), onStart: vi.fn(), onChangeFrame: vi.fn(), onContinue: vi.fn(), onCancel: vi.fn(), onChangeDesign: vi.fn(), onFinish: vi.fn(), onRetryPreview: vi.fn(), onReset: vi.fn(), ...o,
});
const stubSelection = (): PhotoSelection => ({ selectedIds: [], orderOf: () => 0, toggle: vi.fn(), count: 0, required: 4, remaining: 4, complete: false, notice: null, slots: [], submit: vi.fn(), submitting: false });

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
  const slot = (photo: number, w: number, h: number, focusX?: number, focusY?: number) => ({ photo, w, h, focusX, focusY });
  const framed = (slots: ReturnType<typeof slot>[], o: Partial<FrameChoice> = {}): FrameChoice => ({ ...FRAME, photos: slots.length, photoSlots: slots, dpi: 300, borderIn: 0, ...o });
  const guideOf = (c: HTMLElement) => ({ box: c.querySelector(".g-shot-target")?.getAttribute("style"), media: screen.getByTestId("live").getAttribute("data-media") });

  it("frames the shot with the frame's photo WINDOW and keeps that guide before, during and after every one of eight captures", () => {
    for (const photos of [1, 3, 4, 6]) {
      const frame = framed(Array.from({ length: photos }, (_, i) => slot(i, 1.7, 1.2)));
      const { container, rerender, unmount } = render(<GuestShoot {...G(snap("PREVIEW", { shots: 8 }), { frame })} />);
      const first = guideOf(container);
      expect(first.box, `${photos}-photo frame`).toContain("--shot-ar");
      expect(first.media).toContain("objectFit");
      // eight candidates: the capture index runs past the frame's photo count and the guide must not move or vanish
      for (let capture = 0; capture < 8; capture++) {
        for (const state of ["COUNTDOWN", "CAPTURING"] as const) {
          rerender(<GuestShoot {...G(snap(state, { shots: 8, slot: capture, remaining: 2, taken: taken(capture) }), { frame })} />);
          expect(guideOf(container), `${photos}-photo frame, capture ${capture + 1}, ${state}`).toEqual(first);
        }
      }
      unmount();
    }
  });

  it("the guide is the window shape: a wide window is wide, not the shape of the 1:3 paper", () => {
    const { container } = render(<GuestShoot {...G(snap("PREVIEW"), { frame: framed([slot(0, 1.7, 1.2), slot(1, 1.7, 1.2)]) })} />);
    const ar = parseFloat(/--shot-ar:\s*([\d.]+)/.exec(container.querySelector(".g-shot-target")!.getAttribute("style")!)![1]);
    expect(ar).toBeCloseTo(1.7 / 1.2, 2);
  });

  it("changing the frame before shooting recalculates the guide", () => {
    const { container, rerender } = render(<GuestShoot {...G(snap("PREVIEW"), { frame: framed([slot(0, 1.7, 1.2)]) })} />);
    const wide = guideOf(container);
    rerender(<GuestShoot {...G(snap("PREVIEW"), { frame: framed([slot(0, 1.7, 2.6)]) })} />);
    expect(guideOf(container)).not.toEqual(wide);
  });

  it("an off-centre focus and a mirrored live view move the guide to the matching side", () => {
    const frame = framed([slot(0, 1, 1, 0.1, 0.5)]);
    const left = (c: HTMLElement) => parseFloat(JSON.parse(guideOf(c).media!).left);
    const a = render(<GuestShoot {...G(snap("PREVIEW"), { frame })} />);
    const plain = left(a.container);
    a.unmount();
    const b = render(<GuestShoot {...G(snap("PREVIEW"), { frame, mirrored: true })} />);
    expect(left(b.container)).not.toBeCloseTo(plain, 3); // flipped to the other side: the saved photo is not mirrored
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

describe("review: choose your favorites", () => {
  let now = 0;
  const clock = () => now;
  /** The real selection model wired to the real screen; `sent` records what Continue submits. */
  function Review(props: { snapshot: ShootSnapshot; frame?: FrameChoice; language?: string; sent: number[][]; extra?: Partial<GuestShootProps> }) {
    const f = props.frame ?? { ...FRAME, photos: 6 };
    const selection = usePhotoSelection(props.snapshot.sessionId, props.snapshot.taken, f.photos, clock);
    return <GuestShoot {...G(props.snapshot, { frame: f, language: props.language, selection, onContinue: () => { void selection.submit(async (slots) => { props.sent.push(slots); return true; }); }, ...props.extra })} />;
  }
  const eight = () => snap("REVIEW", { shots: 8, taken: taken(8) });
  const tile = (n: number) => screen.getAllByRole("button", { name: new RegExp(`^Photo ${n}\\.|^Photo ${n},`) })[0] as HTMLButtonElement;
  const tap = (n: number) => { now += DOUBLE_TAP_MS + 50; fireEvent.click(tile(n)); };
  const cont = () => screen.getByRole("button", { name: "Continue" }) as HTMLButtonElement;

  it("shows all eight candidates in capture order, with nothing selected, a count and a disabled Continue", () => {
    const sent: number[][] = [];
    const { container } = render(<Review snapshot={{ ...eight(), taken: [...taken(8)].reverse() }} sent={sent} />);
    expect(screen.getByRole("heading", { name: "Choose your favorites" })).toBeTruthy();
    expect(screen.getByText("Tap to select or unselect a photo")).toBeTruthy();
    expect(Array.from(container.querySelectorAll(".g-pick-tile")).map((b) => b.getAttribute("data-slot"))).toEqual(["0", "1", "2", "3", "4", "5", "6", "7"]);
    expect(container.querySelectorAll(".g-pick.on")).toHaveLength(0);
    expect(screen.getByText("0 of 6 selected")).toBeTruthy();
    expect(screen.getByText("Choose 6 more photos")).toBeTruthy();
    expect(cont().disabled).toBe(true);
    expect((container.querySelector(".g-pick-grid") as HTMLElement).style.getPropertyValue("--cl")).toBe("4");
    expect((container.querySelector(".g-pick-grid") as HTMLElement).style.getPropertyValue("--rl")).toBe("2");
  });

  it("a tap selects, a second tap unselects; numbers follow the order of the taps and close up when one is removed", () => {
    render(<Review snapshot={eight()} sent={[]} />);
    for (const n of [2, 4, 1]) tap(n);
    const badge = (n: number) => within(tile(n)).getByText(/^\d+$/).textContent;
    expect([badge(2), badge(4), badge(1)]).toEqual(["1", "2", "3"]);
    expect(tile(2).getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByText("3 of 6 selected")).toBeTruthy();
    tap(4);
    expect(tile(4).getAttribute("aria-pressed")).toBe("false");
    expect([badge(2), badge(1)]).toEqual(["1", "2"]);
    expect(screen.getByText("2 of 6 selected")).toBeTruthy();
  });

  it("never selects more than the frame needs, and says so", () => {
    render(<Review snapshot={eight()} sent={[]} />);
    for (const n of [1, 2, 3, 4, 5, 6]) tap(n);
    tap(7);
    expect(tile(7).getAttribute("aria-pressed")).toBe("false");
    expect(screen.getByText(/already chose 6/)).toBeTruthy();
    expect(screen.getByText("6 of 6 selected")).toBeTruthy();
    expect(cont().disabled).toBe(false);
  });

  it("the spec example: pick 2, 4, 1, 7, 5, 8 and Continue submits those capture slots in that order, once", () => {
    const sent: number[][] = [];
    render(<Review snapshot={eight()} sent={sent} />);
    for (const n of [2, 4, 1, 7, 5]) tap(n);
    expect(cont().disabled).toBe(true);
    expect(screen.getByText("Choose 1 more photo")).toBeTruthy();
    tap(8);
    expect(cont().disabled).toBe(false);
    fireEvent.click(cont()); fireEvent.click(cont()); fireEvent.click(cont());
    expect(sent).toEqual([[1, 3, 0, 6, 4, 7]]);
  });

  it("an accidental double tap on a photo does not select-then-unselect it", () => {
    render(<Review snapshot={eight()} sent={[]} />);
    now += 1000; fireEvent.click(tile(3)); now += 90; fireEvent.click(tile(3));
    expect(tile(3).getAttribute("aria-pressed")).toBe("true");
  });

  it("has no Retake, frame-slot assignment, automatic selection or Start over controls", () => {
    render(<Review snapshot={eight()} sent={[]} />);
    expect(screen.queryByRole("button", { name: /retake/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /frame photo|use for|selected for/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /start over/i })).toBeNull();
    expect(screen.queryByText(/retake|chụp lại/i)).toBeNull();
    // nothing is pre-selected
    expect(document.querySelectorAll('[aria-pressed="true"]')).toHaveLength(0);
  });

  it("selecting photos never calls anything but the selection model", () => {
    const p = { onStart: vi.fn(), onContinue: vi.fn(), onCancel: vi.fn(), onChangeDesign: vi.fn(), onRetryPreview: vi.fn(), onReset: vi.fn(), onFinish: vi.fn() };
    render(<Review snapshot={eight()} sent={[]} extra={p} />);
    for (const n of [1, 2, 3, 3, 4]) tap(n);
    for (const fn of Object.values(p)) expect(fn).not.toHaveBeenCalled();
  });

  it("Continue and the tiles are locked while a submit is in flight or the camera is reconnecting", () => {
    render(<Review snapshot={eight()} sent={[]} extra={{ busy: true }} />);
    expect(tile(1).disabled).toBe(true);
    expect(cont().disabled).toBe(true);
    cleanup();
    render(<Review snapshot={eight()} sent={[]} extra={{ camera: cam({ reconnecting: true }) }} />);
    expect(tile(1).disabled).toBe(true);
  });

  it("starts empty for a new session and keeps the choice when the language changes", () => {
    const sent: number[][] = [];
    const { rerender } = render(<Review snapshot={eight()} sent={sent} />);
    for (const n of [3, 5]) tap(n);
    rerender(<Review snapshot={eight()} sent={sent} language="vi" />);
    expect(screen.getByText("Đã chọn 2/6 ảnh")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Chọn ảnh yêu thích" })).toBeTruthy();
    expect(screen.getByText("Chạm vào ảnh để chọn hoặc bỏ chọn")).toBeTruthy();
    expect(screen.getByText("Chọn thêm 4 ảnh")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Tiếp tục" })).toBeTruthy();
    expect(document.querySelectorAll('[aria-pressed="true"]')).toHaveLength(2);
    rerender(<Review snapshot={{ ...eight(), sessionId: "NEXT" }} sent={sent} language="vi" />);
    expect(screen.getByText("Đã chọn 0/6 ảnh")).toBeTruthy();
  });

  it("changing the frame during review keeps the valid choices and asks for the new count", () => {
    const sent: number[][] = [];
    const six = { ...FRAME, photos: 6 }, four = { ...FRAME, photos: 4 };
    const { rerender } = render(<Review snapshot={eight()} sent={sent} frame={six} />);
    for (const n of [5, 2, 8]) tap(n);
    rerender(<Review snapshot={eight()} sent={sent} frame={four} />);
    expect(screen.getByText("3 of 4 selected")).toBeTruthy();
    expect(screen.getByText("Choose 1 more photo")).toBeTruthy();
    expect(cont().disabled).toBe(true);
    tap(1);
    expect(cont().disabled).toBe(false);
    fireEvent.click(cont());
    expect(sent).toEqual([[4, 1, 7, 0]]);
  });

  it("the frame summary and Change design stay available, and Continue is the only primary action", () => {
    const p = { onChangeDesign: vi.fn() };
    render(<Review snapshot={eight()} sent={[]} extra={p} />);
    expect(screen.getByLabelText("Frame: Four")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Change design" }));
    expect(p.onChangeDesign).toHaveBeenCalled();
    expect(document.querySelectorAll(".g-primary")).toHaveLength(1);
  });

  it("other candidate counts get a sensible grid (4 shots → one row of four; 12 → 4×3)", () => {
    const four = render(<Review snapshot={snap("REVIEW", { taken: taken(4) })} frame={{ ...FRAME, photos: 2 }} sent={[]} />);
    const g = four.container.querySelector(".g-pick-grid") as HTMLElement;
    expect([g.style.getPropertyValue("--cl"), g.style.getPropertyValue("--rl")]).toEqual(["4", "1"]);
    four.unmount();
    const twelve = render(<Review snapshot={snap("REVIEW", { taken: taken(12) })} frame={{ ...FRAME, photos: 4 }} sent={[]} />);
    const h = twelve.container.querySelector(".g-pick-grid") as HTMLElement;
    expect([h.style.getPropertyValue("--cl"), h.style.getPropertyValue("--rl"), h.style.getPropertyValue("--cp"), h.style.getPropertyValue("--rp")]).toEqual(["4", "3", "3", "4"]);
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
