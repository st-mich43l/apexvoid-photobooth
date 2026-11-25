import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({
  S: {} as Record<string, unknown>,
  api: { listPrinters: vi.fn(), printerCapabilities: vi.fn(), printJobs: vi.fn(), listMedia: vi.fn(), patchSettings: vi.fn(), testPrint: vi.fn(), reprint: vi.fn(), printPreview: vi.fn(), printMedia: vi.fn() },
}));
vi.mock("./state", async () => {
  const { translate, formatTranslation } = await import("./i18n");
  return { useApp: () => h.S, useT: () => ({ t: (key: Parameters<typeof translate>[1]) => translate("en", key), tf: (key: Parameters<typeof translate>[1], values: Record<string, string | number>) => formatTranslation("en", key, values) }) };
});
vi.mock("./api", () => ({ api: h.api, on: () => () => {}, toAppError: (e: unknown) => Object.assign(new Error(e instanceof Error ? e.message : String(e)), { action: "" }) }));
vi.mock("./PrintPanel", () => ({ PrintPanel: () => <div data-testid="panel" /> }));

import { PrinterPage } from "./pages/PrinterPage";

const caps = { papers: [{ id: 119, name: "4x6 in", widthIn: 4, heightIn: 6 }], orientations: ["portrait"], maxCopies: 5, color: true, resolutions: [[300, 300]], customPaper: false };
const jobBase = { requestId: "", parentId: "", kind: "print", mediaId: "Mabcdef123", renderedMediaId: "", sessionId: "S", templateId: "t", file: "f", printer: "Lab", paperName: "4x6 in", paperId: 119, orientation: "auto", fit: "fit", copies: 1, nativeId: "1", attempt: 1, createdAt: "2026-11-21T12:00:00Z", updatedAt: "2026-11-21T12:00:00Z" };

beforeEach(() => {
  h.S.settings = { printerName: "Lab", printPaper: "4x6 in" };
  h.S.setSettings = vi.fn(); h.S.toast = vi.fn(); h.S.mediaUrl = () => ""; h.S.libraryVersion = 0;
  for (const f of Object.values(h.api)) f.mockReset();
  h.api.listPrinters.mockResolvedValue({ printers: [{ name: "Lab", default: true, status: "ready" }, { name: "Spare", default: false, status: "offline", detail: "The printer is offline or unavailable." }], driver: "windows-spooler (GDI)", default: "Lab", note: "The spooler cannot prove that paper came out." });
  h.api.printerCapabilities.mockResolvedValue(caps);
  h.api.listMedia.mockResolvedValue([{ id: "Mstrip", kind: "strip", thumbRel: "x", editedRel: "" }]);
  h.api.printJobs.mockResolvedValue([
    { ...jobBase, id: "P3", status: "failed", detail: "Not printing: out of paper." },
    { ...jobBase, id: "P2", status: "removed", detail: "The job left the print queue. It was most likely printed, but the spooler gives no proof that paper came out." },
    { ...jobBase, id: "P1", status: "printing", detail: "Printing." },
    { ...jobBase, id: "PT", kind: "test", status: "completed", detail: "ok" },
  ]);
  h.api.patchSettings.mockResolvedValue({ restartRequired: false });
});
afterEach(cleanup);

describe("Printer page", () => {
  it("lists printers with availability, the default and the one used by the booth", async () => {
    render(<PrinterPage />);
    await screen.findByText("Spare");
    expect(screen.getByText("Windows default")).toBeTruthy();
    expect(screen.getByText("used by the booth")).toBeTruthy();
    expect(screen.getByText("offline")).toBeTruthy();
    expect(screen.getByText(/offline or unavailable/)).toBeTruthy();
    expect(screen.getByText(/cannot prove that paper came out/)).toBeTruthy();
  });

  it("shows job states honestly (a job that left the queue is not called 'printed')", async () => {
    render(<PrinterPage />);
    await screen.findByText("Left the queue");
    expect(screen.getByText("Failed")).toBeTruthy();
    expect(screen.getByText("Printing")).toBeTruthy();
    expect(screen.getByText("Test page")).toBeTruthy();
    expect(screen.getByText(/Printed \(spooler says so\)/)).toBeTruthy();
    expect(screen.queryByText("Printed")).toBeNull();
  });

  it("asks before printing a test page and only then sends it", async () => {
    h.api.testPrint.mockResolvedValue({ ...jobBase, id: "PN", kind: "test", status: "queued", detail: "" });
    render(<PrinterPage />);
    await screen.findByText("Spare");
    fireEvent.click(await screen.findByRole("button", { name: "Test print…" }));
    expect(h.api.testPrint).not.toHaveBeenCalled();
    fireEvent.click(await screen.findByRole("button", { name: "Print test page" }));
    await waitFor(() => expect(h.api.testPrint).toHaveBeenCalledWith("Lab", "4x6 in", "portrait"));
  });

  it("requires confirmation to reprint, never for a job still in the queue", async () => {
    h.api.reprint.mockResolvedValue({ ...jobBase, id: "PR", status: "queued", detail: "" });
    render(<PrinterPage />);
    await screen.findByText("Left the queue");
    const buttons = screen.getAllByRole("button", { name: "Reprint…" });
    expect(buttons).toHaveLength(2);                          // failed + removed; not the printing job, not the test page
    fireEvent.click(buttons[1]);
    expect(h.api.reprint).not.toHaveBeenCalled();
    expect(screen.getByText(/cannot tell whether the earlier print came out/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Reprint" }));
    await waitFor(() => expect(h.api.reprint).toHaveBeenCalledWith("P2", true));
  });

  it("makes a printer the booth printer and saves the paper default", async () => {
    render(<PrinterPage />);
    await screen.findByText("Spare");
    fireEvent.click(screen.getAllByRole("button", { name: "Use this printer" })[1] as HTMLButtonElement);
    await waitFor(() => expect(h.api.patchSettings).toHaveBeenCalledWith({ printerName: "Spare" }));
  });

  it("offers a preview/print panel for recent images", async () => {
    render(<PrinterPage />);
    expect(await screen.findByTestId("panel")).toBeTruthy();
  });
});
