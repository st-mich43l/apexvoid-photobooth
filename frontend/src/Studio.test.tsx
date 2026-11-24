import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({ S: {} as Record<string, unknown>, api: { boothReadiness: vi.fn() } }));
vi.mock("./state", async () => {
  const { translate, formatTranslation } = await import("./i18n");
  return {
    useApp: () => h.S,
    useT: () => ({
      t: (key: Parameters<typeof translate>[1]) => translate((h.S.settings as { language?: string } | undefined)?.language, key),
      tf: (key: Parameters<typeof formatTranslation>[1], values: Record<string, string | number>) => formatTranslation((h.S.settings as { language?: string } | undefined)?.language, key, values),
    }),
  };
});
vi.mock("./api", () => ({ api: h.api, toAppError: (e: unknown) => ({ message: String(e), action: "" }) }));
vi.mock("./pages/CameraPage", () => ({ CameraPage: () => <div data-testid="page-CameraPage" /> }));
vi.mock("./pages/Dashboard", () => ({ Dashboard: () => <div data-testid="page-Dashboard" /> }));
vi.mock("./pages/DiagnosticsPage", () => ({ DiagnosticsPage: () => <div data-testid="page-DiagnosticsPage" /> }));
vi.mock("./pages/Gallery", () => ({ Gallery: () => <div data-testid="page-Gallery" /> }));
vi.mock("./pages/PhotoPage", () => ({ PhotoPage: () => <div data-testid="page-PhotoPage" /> }));
vi.mock("./pages/Settings", () => ({ SettingsPage: () => <div data-testid="page-SettingsPage" /> }));
vi.mock("./pages/Frames", () => ({ Frames: () => <div data-testid="page-Frames" /> }));
vi.mock("./pages/PrinterPage", () => ({ PrinterPage: () => <div data-testid="page-PrinterPage" /> }));
vi.mock("./pages/VideoPage", () => ({ VideoPage: () => <div data-testid="page-VideoPage" /> }));
vi.mock("./operator/PreparePage", () => ({ PreparePage: () => <div data-testid="page-PreparePage" /> }));
vi.mock("./MigrationDialog", () => ({ MigrationDialog: () => null }));
import { NAV_MAIN, NAV_TOOLS, Studio } from "./Studio";

// the project has no @types/node; the tests run in node, so read the stylesheet from disk
// @ts-ignore
const { readFileSync } = (await import("node:fs")) as { readFileSync: (p: string, e: string) => string };
const css = readFileSync(`${(globalThis as unknown as { process: { cwd(): string } }).process.cwd()}/src/operator/operator.css`, "utf8");

beforeEach(() => {
  window.location.hash = "";
  h.api.boothReadiness.mockReset().mockResolvedValue({ canStart: true, items: [] });
  h.S = { camera: { device: { name: "C", simulated: false }, state: "connected", reconnecting: false }, boot: { version: "t" }, location: {}, refreshReadiness: vi.fn() };
});
afterEach(cleanup);

describe("Studio navigation", () => {
  it("daily operations first, everything else under Tools, and no page lost", () => {
    expect(NAV_MAIN.map(([id]) => id)).toEqual(["dashboard", "prepare", "camera", "printer", "frames", "gallery"]);
    expect(NAV_TOOLS.map(([id]) => id)).toEqual(["photo", "video", "settings", "diagnostics"]);
    render(<Studio onBooth={vi.fn()} />);
    const nav = screen.getByRole("navigation", { name: "Studio navigation" });
    expect(within(within(nav).getByRole("group", { name: "Daily" })).getAllByRole("button")).toHaveLength(6);
    expect(within(within(nav).getByRole("group", { name: "Tools" })).getAllByRole("button")).toHaveLength(4);
  });
  it("marks the current page and opens each page without a reload", () => {
    render(<Studio onBooth={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Dashboard" }).getAttribute("aria-current")).toBe("page");
    fireEvent.click(screen.getByRole("button", { name: "Prepare Booth" }));
    expect(screen.getByTestId("page-PreparePage")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Prepare Booth" }).getAttribute("aria-current")).toBe("page");
    expect(screen.getByRole("button", { name: "Dashboard" }).getAttribute("aria-current")).toBeNull();
    for (const [id, test] of [["Camera", "CameraPage"], ["Printer", "PrinterPage"], ["Frames", "Frames"], ["Gallery", "Gallery"], ["Photo", "PhotoPage"], ["Video", "VideoPage"], ["Settings", "SettingsPage"], ["Diagnostics", "DiagnosticsPage"]]) {
      fireEvent.click(screen.getByRole("button", { name: id })); expect(screen.getByTestId(`page-${test}`)).toBeTruthy();
    }
  });
  it("deep links such as #prepare open that page", () => {
    window.location.hash = "#prepare";
    render(<Studio onBooth={vi.fn()} />);
    expect(screen.getByTestId("page-PreparePage")).toBeTruthy();
  });
  it("Open Booth Mode in the navigation checks readiness first and launches once, however often it is clicked", async () => {
    const onBooth = vi.fn();
    render(<Studio onBooth={onBooth} />);
    const b = screen.getByRole("button", { name: "Open Booth Mode" });
    fireEvent.click(b); fireEvent.click(b); fireEvent.click(b);
    await waitFor(() => expect(onBooth).toHaveBeenCalledTimes(1));
    expect(h.api.boothReadiness).toHaveBeenCalledTimes(1);
  });
  it("does not launch when a required check fails", async () => {
    const onBooth = vi.fn();
    h.api.boothReadiness.mockResolvedValue({ canStart: false, items: [{ key: "camera", label: "Photo camera", required: true, state: "problem", detail: "No camera is connected.", action: "Connect one." }] });
    render(<Studio onBooth={onBooth} />);
    fireEvent.click(screen.getByRole("button", { name: "Open Booth Mode" }));
    expect((await screen.findByRole("dialog")).textContent).toMatch(/No camera is connected/);
    expect(onBooth).not.toHaveBeenCalled();
  });
});

describe("Studio layouts", () => {
  it("portrait (and narrow windows) get a labelled top bar instead of an icon-only rail", () => {
    const portrait = css.slice(css.indexOf("@media (orientation: portrait), (max-width: 760px)"));
    expect(portrait).toMatch(/\.studio \{ grid-template-columns: 1fr !important/);
    expect(portrait).toMatch(/\.studio nav \{[^}]*flex-direction: row/);
    expect(portrait).toMatch(/overflow-x: auto/);
    expect(portrait).toMatch(/\.studio nav \.booth-btn \{[^}]*display: inline-flex/); // Open Booth Mode stays reachable
    expect(portrait).not.toMatch(/font-size: 0/);                                         // labels are never hidden
  });
  it("Prepare Booth is two columns on wide screens and one in portrait", () => {
    expect(css).toMatch(/\.prep-grid \{ display: grid; grid-template-columns: repeat\(2, minmax\(0, 1fr\)\)/);
    expect(css).toMatch(/@media \(orientation: portrait\) \{ \.prep-grid \{ grid-template-columns: 1fr/);
  });
  it("controls stay comfortable to touch", () => {
    expect(css).toMatch(/\.stepper button \{ min-width: 44px; min-height: 44px/);
    expect(css).toMatch(/\.studio nav button \{[^}]*min-height: 56px/);
  });
});
