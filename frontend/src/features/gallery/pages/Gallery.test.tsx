import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
const h = vi.hoisted(() => ({ api: { querySessions: vi.fn(), querySessionMedia: vi.fn(), listSessionPrintJobs: vi.fn(), trashSession: vi.fn(), restoreSession: vi.fn(), pickFolder: vi.fn(), exportSession: vi.fn(), trashMedia: vi.fn(), restoreMedia: vi.fn() }, state: { mediaUrl: (id: string) => `/media/${id}`, libraryVersion: 0, bumpLibrary: vi.fn(), toast: vi.fn() } }));
vi.mock("../../../shared/api", () => ({ api: h.api, toAppError: (e: unknown) => Object.assign(new Error(String(e)), { action: "" }) }));
vi.mock("../../../app/state", async () => {
  const { translate, formatTranslation } = await import("../../../shared/i18n");
  return { useApp: () => h.state, useT: () => ({ t: (key: Parameters<typeof translate>[1]) => translate("en", key), tf: (key: Parameters<typeof translate>[1], values: Record<string, string | number>) => formatTranslation("en", key, values) }) };
});
vi.mock("./Editor", () => ({ Editor: () => null }));
vi.mock("../../printer/PrintPanel", () => ({ PrintPanel: () => null }));
vi.mock("../../frames/FrameSelection", () => ({ FrameSelection: () => null }));
import { Gallery } from "./Gallery";
const card = { id: "S001", createdAt: "2026-01-02T12:00:00Z", updatedAt: "2026-01-02T12:00:00Z", status: "completed", mode: "photo", shotsPlanned: 4, templateId: "", simulated: false, note: "", mediaCount: 1, frameId: "", frameName: "Classic", variantId: "", frameRev: 0, caption: "", frameDir: "", thumbnailId: "M1", thumbnailVersion: "thumb", photoCount: 1, hasStrip: false };
const item = { id: "M1", sessionId: "S001", kind: "photo", slot: 0, relPath: "", thumbRel: "thumb", previewRel: "", editedRel: "", edits: "", width: 100, height: 100, sizeBytes: 1, sha256: "", durationMs: 0, simulated: false, source: "camera", status: "active", capturedAt: "2026-01-02T12:00:00Z", frameId: "", frameName: "", variantId: "", frameRev: 0, frameDir: "" };
beforeEach(() => { for (const f of Object.values(h.api)) f.mockReset(); h.api.querySessions.mockResolvedValue({ items: [card], nextCursor: "next", hasMore: true }); h.api.querySessionMedia.mockResolvedValue({ items: [item], nextOffset: 1, hasMore: false }); h.api.listSessionPrintJobs.mockResolvedValue([]); });
afterEach(cleanup);
describe("session gallery", () => {
 it("filters on the server and opens bounded session media", async () => {
  render(<Gallery />); await screen.findByText("S001");
  fireEvent.change(screen.getByLabelText("Mode"), { target: { value: "video" } });
  await waitFor(() => expect(h.api.querySessions).toHaveBeenLastCalledWith(expect.objectContaining({ mode: "video", limit: 40, cursor: "" })));
  fireEvent.click(await screen.findByText("S001")); await screen.findByRole("button", { name: /photo from/ });
  expect(h.api.querySessionMedia).toHaveBeenCalledWith("S001", 50, 0); expect(screen.getByText(/Original captures appear/)).toBeTruthy();
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
  fireEvent.click(screen.getByText("S001")); await screen.findAllByRole("button", { name: /photo from/ });
  fireEvent.click(screen.getAllByRole("button", { name: /photo from/ })[0]);
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
 it("exports the selected session to the chosen folder", async () => {
  h.api.pickFolder.mockResolvedValue("/chosen"); h.api.exportSession.mockResolvedValue("/chosen/S001.zip");
  render(<Gallery />); fireEvent.click(await screen.findByText("S001"));
  fireEvent.click(await screen.findByRole("button", { name: "Export session…" }));
  await waitFor(() => expect(h.api.exportSession).toHaveBeenCalledWith("S001", "/chosen", true));
 });

 it("navigates from the last image to the next media page", async () => {
  const first = Array.from({ length: 50 }, (_, i) => ({ ...item, id: `M${i}`, slot: i }));
  h.api.querySessionMedia.mockImplementation((_id: string, _limit: number, offset: number) => Promise.resolve(offset === 0 ? { items: first, nextOffset: 50, hasMore: true } : { items: [{ ...item, id: "M50", slot: 50 }], nextOffset: 51, hasMore: false }));
  render(<Gallery />); fireEvent.click(await screen.findByText("S001"));
  const tiles = await screen.findAllByRole("button", { name: /photo from/ }); fireEvent.click(tiles[49]);
  fireEvent.click(screen.getByRole("button", { name: /^Next$/ }));
  await waitFor(() => expect((screen.getByAltText("Selected photo") as HTMLImageElement).src).toContain("/media/M50"));
 });

});
