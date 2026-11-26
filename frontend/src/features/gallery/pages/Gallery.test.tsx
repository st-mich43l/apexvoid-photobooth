import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
const h = vi.hoisted(() => ({ api: { querySessions: vi.fn(), querySessionMedia: vi.fn(), listSessionPrintJobs: vi.fn(), trashSession: vi.fn(), restoreSession: vi.fn(), pickFolder: vi.fn(), exportSession: vi.fn(), trashMedia: vi.fn(), restoreMedia: vi.fn(), exportMedia: vi.fn(), reprint: vi.fn(), listMedia: vi.fn(), composeSession: vi.fn() }, state: { mediaUrl: (id: string, kind = "thumb") => `/media/${id}/${kind}`, libraryVersion: 0, bumpLibrary: vi.fn(), toast: vi.fn() } }));
vi.mock("../../../shared/api", () => ({ api: h.api, toAppError: (e: unknown) => Object.assign(new Error(String(e)), { action: "" }) }));
vi.mock("../../../app/state", async () => {
  const { translate, formatTranslation } = await import("../../../shared/i18n");
  return { useApp: () => h.state, useT: () => ({ t: (key: Parameters<typeof translate>[1]) => translate("en", key), tf: (key: Parameters<typeof translate>[1], values: Record<string, string | number>) => formatTranslation("en", key, values) }) };
});
vi.mock("./Editor", () => ({ Editor: () => null }));
vi.mock("../../printer/PrintPanel", () => ({ PrintPanel: ({ mediaId }: { mediaId: string }) => <p>print panel for {mediaId}</p> }));
vi.mock("../../frames/FrameSelection", () => ({ FrameSelection: () => <p>frame picker</p> }));
import { Gallery } from "./Gallery";
const card = { id: "S001", createdAt: "2026-01-02T12:00:00Z", updatedAt: "2026-01-02T12:00:00Z", status: "completed", mode: "photo", shotsPlanned: 4, templateId: "", simulated: false, note: "", mediaCount: 1, frameId: "", frameName: "Classic", variantId: "", frameRev: 0, caption: "", frameDir: "", thumbnailId: "M1", thumbnailVersion: "thumb", photoCount: 1, hasStrip: false };
const item = { id: "M1", sessionId: "S001", kind: "photo", slot: 0, relPath: "", thumbRel: "thumb", previewRel: "", editedRel: "", edits: "", width: 100, height: 100, sizeBytes: 1, sha256: "", durationMs: 0, simulated: false, source: "camera", status: "active", capturedAt: "2026-01-02T12:00:00Z", frameId: "", frameName: "", variantId: "", frameRev: 0, frameDir: "" };
const strip = (id: string, w: number, h: number, frameName = "Lavender") => ({ ...item, id, kind: "strip", width: w, height: h, frameId: "lavender.sheet4", frameName });
const video = (id: string, extra = {}) => ({ ...item, id, kind: "video", width: 1920, height: 1080, durationMs: 24_500, ...extra });
const job = { id: "J1", requestId: "r", parentId: "", kind: "print", mediaId: "C1", renderedMediaId: "", sessionId: "S001", templateId: "", file: "", printer: "Lab", paperName: "4x6 in", paperId: 1, orientation: "portrait", fit: "fit", copies: 2, status: "completed", detail: "", nativeId: "1", attempt: 1, createdAt: "2026-01-02T12:05:00Z", updatedAt: "2026-01-02T12:05:00Z" };
beforeEach(() => { for (const f of Object.values(h.api)) f.mockReset(); h.state.toast.mockReset(); h.state.bumpLibrary.mockReset(); h.api.querySessions.mockResolvedValue({ items: [card], nextCursor: "next", hasMore: true }); h.api.querySessionMedia.mockResolvedValue({ items: [item], nextOffset: 1, hasMore: false }); h.api.listSessionPrintJobs.mockResolvedValue([]); });
afterEach(cleanup);
const openSession = async (session = card) => { h.api.querySessions.mockResolvedValue({ items: [session], nextCursor: "", hasMore: false }); render(<Gallery />); fireEvent.click(await screen.findByText(session.id)); await screen.findByRole("navigation", { name: "Breadcrumb" }); };
const region = (name: string) => screen.queryByRole("region", { name }) ?? screen.getByRole("complementary", { name });

describe("session gallery", () => {
  it("filters on the server and opens bounded session media", async () => {
    render(<Gallery />); await screen.findByText("S001");
    fireEvent.change(screen.getByLabelText("Mode"), { target: { value: "video" } });
    await waitFor(() => expect(h.api.querySessions).toHaveBeenLastCalledWith(expect.objectContaining({ mode: "video", limit: 40, cursor: "" })));
    fireEvent.click(await screen.findByText("S001")); await screen.findByRole("button", { name: /Open photo 1 from/ });
    expect(h.api.querySessionMedia).toHaveBeenCalledWith("S001", 50, 0);
  });
  it("passes the cursor for a second page", async () => {
    render(<Gallery />); await screen.findByText("S001");
    h.api.querySessions.mockResolvedValueOnce({ items: [{ ...card, id: "S000" }], nextCursor: "", hasMore: false });
    fireEvent.click(screen.getByRole("button", { name: "Next sessions" }));
    await screen.findByText("S000"); expect(h.api.querySessions).toHaveBeenLastCalledWith(expect.objectContaining({ cursor: "next" }));
  });
  it("navigates images with the keyboard and handles a missing thumbnail", async () => {
    h.api.querySessionMedia.mockResolvedValue({ items: [item, { ...item, id: "M2", slot: 1 }], nextOffset: 2, hasMore: false });
    render(<Gallery />); await screen.findByText("S001");
    const thumb = document.querySelector(".session-card img")!;
    fireEvent.error(thumb);
    expect(document.querySelector(".session-card .media-image")?.getAttribute("data-state")).toBe("retrying");
    fireEvent.click(screen.getByText("S001")); await screen.findAllByRole("button", { name: /Open photo/ });
    fireEvent.click(screen.getByRole("button", { name: /Open photo 1 from/ }));
    expect((screen.getByAltText("Selected photo") as HTMLImageElement).src).toContain("/media/M1");
    fireEvent.keyDown(window, { key: "ArrowRight" });
    await waitFor(() => expect((screen.getByAltText("Selected photo") as HTMLImageElement).src).toContain("/media/M2"));
  });
  it("discards stale search responses", async () => {
    let resolveOld!: (value: unknown) => void;
    h.api.querySessions.mockImplementation((q: { search: string }) => q.search ? Promise.resolve({ items: [{ ...card, id: "S-new" }], nextCursor: "", hasMore: false }) : new Promise((r) => { resolveOld = r; }));
    render(<Gallery />); await waitFor(() => expect(resolveOld).toBeDefined());
    fireEvent.change(screen.getByLabelText("Search session ID"), { target: { value: "new" } }); await screen.findByText("S-new");
    await act(async () => resolveOld({ items: [card], nextCursor: "", hasMore: false }));
    expect(screen.queryByText("S001")).toBeNull(); expect(screen.getByText("S-new")).toBeTruthy();
  });
  it("navigates from the last image to the next media page", async () => {
    const first = Array.from({ length: 50 }, (_, i) => ({ ...item, id: `M${i}`, slot: i }));
    h.api.querySessionMedia.mockImplementation((_id: string, _limit: number, offset: number) => Promise.resolve(offset === 0 ? { items: first, nextOffset: 50, hasMore: true } : { items: [{ ...item, id: "M50", slot: 50 }], nextOffset: 51, hasMore: false }));
    render(<Gallery />); fireEvent.click(await screen.findByText("S001"));
    const tiles = await screen.findAllByRole("button", { name: /Open photo/ }); fireEvent.click(tiles[49]);
    fireEvent.click(screen.getByRole("button", { name: /^Next$/ }));
    await waitFor(() => expect((screen.getByAltText("Selected photo") as HTMLImageElement).src).toContain("/media/M50"));
    expect(h.api.querySessionMedia).toHaveBeenLastCalledWith("S001", 50, 50);
  });
  it("pages session media with bounded queries", async () => {
    h.api.querySessionMedia.mockResolvedValue({ items: [item], nextOffset: 50, hasMore: true });
    await openSession();
    await screen.findByText("Items 1–1");
    fireEvent.click(screen.getByRole("button", { name: "Next media" }));
    await waitFor(() => expect(h.api.querySessionMedia).toHaveBeenLastCalledWith("S001", 50, 50));
  });
});

describe("session detail header", () => {
  it("shows a compact header with breadcrumb, localized status and the session ID in a secondary line", async () => {
    await openSession({ ...card, photoCount: 4 });
    const header = document.querySelector(".sd-header") as HTMLElement;
    expect(within(header).getByText("Completed")).toBeTruthy();
    expect(within(header).getByText("4 photos")).toBeTruthy();
    expect(within(header).queryByText("S001")).toBeNull(); // the long ID is not in the title area
    expect(document.querySelector(".sd-id code")?.textContent).toBe("S001");
    fireEvent.click(within(screen.getByRole("navigation", { name: "Breadcrumb" })).getByRole("button", { name: "Gallery" }));
    expect(await screen.findByLabelText("Session filters")).toBeTruthy();
  });
  it("keeps the trash action inside the overflow menu with its confirmation and undo", async () => {
    h.api.trashSession.mockResolvedValue(undefined); h.api.restoreSession.mockResolvedValue(undefined);
    await openSession();
    const actions = screen.getByRole("group", { name: "Session actions" });
    expect(within(actions).queryByRole("button", { name: /trash/i })).toBeNull();
    expect(screen.queryByRole("checkbox", { name: /edited/i })).toBeNull(); // no permanent export option in the header
    fireEvent.click(within(actions).getByRole("button", { name: "More session actions" }));
    const menu = screen.getByRole("menu", { name: "More session actions" });
    expect(within(menu).getAllByRole("menuitem").map((b) => b.textContent)).toEqual(["Copy session ID", "Move session to trash…"]);
    fireEvent.click(within(menu).getByRole("menuitem", { name: "Move session to trash…" }));
    expect(screen.queryByRole("menu")).toBeNull();
    fireEvent.click(within(screen.getByRole("dialog", { name: "Move session to trash?" })).getByRole("button", { name: "Move session to trash" }));
    await waitFor(() => expect(h.api.trashSession).toHaveBeenCalledWith("S001"));
    const undo = h.state.toast.mock.calls.find((c) => c[0] === "Session moved to trash")?.[2] as () => void;
    undo(); await waitFor(() => expect(h.api.restoreSession).toHaveBeenCalledWith("S001"));
  });
  it("closes the overflow menu with Escape and returns focus to its button", async () => {
    await openSession();
    const more = screen.getByRole("button", { name: "More session actions" });
    fireEvent.click(more);
    const first = screen.getByRole("menuitem", { name: "Copy session ID" });
    await waitFor(() => expect(document.activeElement).toBe(first));
    fireEvent.keyDown(first, { key: "ArrowDown" });
    expect(document.activeElement).toBe(screen.getByRole("menuitem", { name: "Move session to trash…" }));
    fireEvent.keyDown(document.activeElement!, { key: "Escape" });
    expect(screen.queryByRole("menu")).toBeNull(); expect(document.activeElement).toBe(more);
  });
  it("keeps action availability rules for active and trashed sessions", async () => {
    await openSession({ ...card, status: "active" });
    expect((screen.getByRole("button", { name: "Export session" }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "More session actions" }));
    expect((screen.getByRole("menuitem", { name: "Move session to trash…" }) as HTMLButtonElement).disabled).toBe(true);
    cleanup();
    h.api.restoreSession.mockResolvedValue(undefined);
    await openSession({ ...card, status: "trashed" });
    expect((screen.getByRole("button", { name: "Export session" }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "More session actions" }));
    expect(screen.queryByRole("menuitem", { name: /trash/ })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Restore session" }));
    await waitFor(() => expect(h.api.restoreSession).toHaveBeenCalledWith("S001"));
  });
  it("asks about edited photos inside the export interaction", async () => {
    h.api.pickFolder.mockResolvedValue("/chosen"); h.api.exportSession.mockResolvedValue("/chosen/S001.zip");
    await openSession();
    fireEvent.click(screen.getByRole("button", { name: "Export session" }));
    const dialog = screen.getByRole("dialog", { name: "Export session" });
    fireEvent.click(within(dialog).getByRole("button", { name: "Choose folder and export" }));
    await waitFor(() => expect(h.api.exportSession).toHaveBeenCalledWith("S001", "/chosen", true));
    fireEvent.click(screen.getByRole("button", { name: "Export session" }));
    fireEvent.click(within(screen.getByRole("dialog", { name: "Export session" })).getByRole("checkbox", { name: /Include edited photos/ }));
    fireEvent.click(screen.getByRole("button", { name: "Choose folder and export" }));
    await waitFor(() => expect(h.api.exportSession).toHaveBeenLastCalledWith("S001", "/chosen", false));
  });
  it("copies the session ID", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
    await openSession();
    fireEvent.click(screen.getByRole("button", { name: "Copy session ID" }));
    await waitFor(() => expect(h.state.toast).toHaveBeenCalledWith("Session ID copied", "ok"));
    expect(writeText).toHaveBeenCalledWith("S001");
  });
});

describe("session detail media", () => {
  it("classifies originals, compositions and videos into separate groups", async () => {
    h.api.querySessionMedia.mockResolvedValue({ items: [item, { ...item, id: "M2", slot: 1, editedRel: "e" }, strip("C1", 1200, 1800), video("V1")], nextOffset: 4, hasMore: false });
    await openSession({ ...card, hasStrip: true, photoCount: 2 });
    const originals = region("Original photos");
    expect(within(originals).getAllByRole("button", { name: /Open photo/ })).toHaveLength(2);
    expect(within(originals).getByText("edited")).toBeTruthy();
    expect(within(originals).queryByRole("button", { name: /composition/i })).toBeNull();
    expect(within(originals).getByRole("heading", { name: "Videos" })).toBeTruthy();
    expect(within(originals).getByRole("button", { name: "Play video 1, 00:24" })).toBeTruthy();
    const panel = region("Final composition");
    expect(within(panel).getByRole("button", { name: "Open composition 1 in the viewer" })).toBeTruthy();
    expect(within(panel).getByText("4×6 print")).toBeTruthy();
    expect(within(panel).getByText("1200×1800 px")).toBeTruthy();
  });
  it("sizes photo tiles from the photo's own dimensions", async () => {
    h.api.querySessionMedia.mockResolvedValue({ items: [{ ...item, width: 1600, height: 1067 }, { ...item, id: "M2", slot: 1, width: 1067, height: 1600 }], nextOffset: 2, hasMore: false });
    await openSession();
    const tiles = await screen.findAllByRole("button", { name: /Open photo/ });
    expect(Number(tiles[0].style.getPropertyValue("--ar"))).toBeCloseTo(1.5, 2);
    expect(Number(tiles[1].style.getPropertyValue("--ar"))).toBeCloseTo(0.667, 2);
  });
  it("shows the missing and partial indicators", async () => {
    h.api.querySessionMedia.mockResolvedValue({ items: [{ ...item, status: "missing" }, video("V1", { status: "partial" })], nextOffset: 2, hasMore: false });
    await openSession();
    expect(within(region("Original photos")).getAllByText("file missing").length).toBeGreaterThan(0);
    expect(within(region("Original photos")).getByText("partial recording")).toBeTruthy();
  });
  it("opens the composition in the viewer and prints from the panel", async () => {
    h.api.querySessionMedia.mockResolvedValue({ items: [item, strip("C1", 600, 1800)], nextOffset: 2, hasMore: false });
    await openSession({ ...card, hasStrip: true });
    const panel = region("Final composition");
    expect(within(panel).getByText("2×6 strip")).toBeTruthy();
    fireEvent.click(within(panel).getByRole("button", { name: "Print…" }));
    expect(screen.getByText("print panel for C1")).toBeTruthy();
    fireEvent.click(within(screen.getByRole("dialog", { name: "Print" })).getByRole("button", { name: "Close" }));
    fireEvent.click(within(panel).getByRole("button", { name: "Open composition 1 in the viewer" }));
    expect(screen.getByRole("dialog", { name: "Photo strip" })).toBeTruthy();
  });
  it("exports the composition and opens Change design", async () => {
    h.api.exportMedia.mockResolvedValue("/exports/C1.jpg"); h.api.listMedia.mockResolvedValue([item]);
    h.api.querySessionMedia.mockResolvedValue({ items: [item, strip("C1", 1200, 1800)], nextOffset: 2, hasMore: false });
    await openSession({ ...card, hasStrip: true });
    fireEvent.click(within(region("Final composition")).getByRole("button", { name: "Export image" }));
    await waitFor(() => expect(h.state.toast).toHaveBeenCalledWith("Image exported to /exports/C1.jpg"));
    expect(h.api.exportMedia).toHaveBeenCalledWith("C1", "", false);
    fireEvent.click(within(region("Final composition")).getByRole("button", { name: "Change design…" }));
    expect(await screen.findByText("frame picker")).toBeTruthy();
    expect(h.api.listMedia).toHaveBeenCalledWith("S001", 200, 0);
  });
  it("selects between multiple compositions, newest first", async () => {
    h.api.querySessionMedia.mockResolvedValue({ items: [item, strip("C1", 600, 1800, "Strip"), strip("C2", 1200, 1800, "Sheet")], nextOffset: 3, hasMore: false });
    await openSession({ ...card, hasStrip: true });
    const panel = region("Final composition");
    expect(within(panel).getByText("2 of 2")).toBeTruthy();
    const picker = within(panel).getByRole("group", { name: "Compositions" });
    const [first, second] = within(picker).getAllByRole("button");
    expect(second.getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(first);
    expect(first.getAttribute("aria-pressed")).toBe("true");
    expect(within(panel).getByText("1 of 2")).toBeTruthy();
    expect(within(panel).getByText("2×6 strip")).toBeTruthy();
  });
  it("shows a localized empty state when there is no composition", async () => {
    await openSession();
    const panel = region("Final composition");
    expect(within(panel).getByText("No final composition yet")).toBeTruthy();
    expect(within(panel).getByRole("button", { name: "Choose design…" })).toBeTruthy();
  });
  it("adapts the side panel to recordings in a video-only session", async () => {
    h.api.querySessionMedia.mockResolvedValue({ items: [video("V1"), video("V2", { durationMs: 8_000, status: "partial" })], nextOffset: 2, hasMore: false });
    await openSession({ ...card, mode: "video", photoCount: 0 });
    expect(screen.queryByRole("complementary", { name: "Final composition" })).toBeNull();
    expect(screen.getByRole("heading", { name: "Recordings" })).toBeTruthy();
    const panel = region("Recording");
    expect(within(panel).getByText("00:24")).toBeTruthy();
    h.api.exportMedia.mockResolvedValue("/exports/V1.mp4");
    fireEvent.click(within(panel).getByRole("button", { name: "Export video" }));
    await waitFor(() => expect(h.state.toast).toHaveBeenCalledWith("Video exported to /exports/V1.mp4"));
    fireEvent.click(screen.getByRole("button", { name: "Play video 2, 00:08" }));
    expect(screen.getByRole("dialog", { name: "Video" })).toBeTruthy();
  });
});

describe("session print history", () => {
  it("keeps loading, error and empty states distinct", async () => {
    let fail!: (e: Error) => void;
    h.api.listSessionPrintJobs.mockReturnValue(new Promise((_r, j) => { fail = j; }));
    await openSession();
    expect(screen.getByText("Loading print history…")).toBeTruthy();
    await act(async () => fail(new Error("db")));
    expect(screen.getByRole("alert").textContent).toBe("Print history could not be loaded.");
    cleanup();
    h.api.listSessionPrintJobs.mockResolvedValue([]);
    await openSession();
    expect(await screen.findByText("No print jobs for this session.")).toBeTruthy();
    expect(screen.getByText(/does not confirm the paper came out/)).toBeTruthy();
  });
  it("lists jobs with printer, paper, status and reprint", async () => {
    h.api.listSessionPrintJobs.mockResolvedValue([job, { ...job, id: "J2", parentId: "J1", attempt: 2, status: "queued", copies: 1 }]);
    h.api.reprint.mockResolvedValue({ ...job, id: "J3" });
    await openSession();
    const history = region("Print history");
    await within(history).findByText("2 jobs");
    expect(within(history).getAllByText("Lab")).toHaveLength(2);
    expect(within(history).getByText("2 copies")).toBeTruthy();
    expect(within(history).getByText("Printed (spooler says so)")).toBeTruthy();
    expect(within(history).getByText("reprint #1")).toBeTruthy();
    expect(within(history).getAllByRole("button", { name: "Reprint…" })).toHaveLength(1); // the queued job cannot be reprinted yet
    fireEvent.click(within(history).getByRole("button", { name: "Reprint…" }));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Reprint" }));
    await waitFor(() => expect(h.api.reprint).toHaveBeenCalledWith("J1", true));
  });
});
