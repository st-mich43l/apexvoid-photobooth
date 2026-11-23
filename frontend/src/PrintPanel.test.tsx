import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({
  S: {} as Record<string, unknown>,
  api: { listPrinters: vi.fn(), printerCapabilities: vi.fn(), printPreview: vi.fn(), printMedia: vi.fn() },
}));
vi.mock("./state", () => ({ useApp: () => h.S }));
vi.mock("./api", () => ({ api: h.api, toAppError: (e: unknown) => Object.assign(new Error(e instanceof Error ? e.message.split("\n")[0] : String(e)), { action: "" }) }));

import { PrintPanel } from "./PrintPanel";

const caps = { papers: [{ id: 119, name: "4x6 in", widthIn: 4, heightIn: 6 }, { id: 120, name: "5x7 in", widthIn: 5, heightIn: 7 }], orientations: ["portrait", "landscape"], maxCopies: 5, color: true, resolutions: null, customPaper: false };
const preview = { image: "data:image/jpeg;base64,AAAA", paperMm: [101.6, 152.4], printableMm: [95, 146], marginMm: [25.4, 0, 25.4, 0], scalePct: 100, croppedPct: 0, rotated: false, orientation: "portrait", warning: "", assumedGeometry: false, imageInches: [2, 6], templateId: "strip-2x6-4" };
const job = { id: "P1", status: "queued", detail: "", requestId: "x" };

beforeEach(() => {
  h.S.settings = { printerName: "", printPaper: "" };
  h.S.toast = vi.fn();
  for (const f of Object.values(h.api)) f.mockReset();
  h.api.listPrinters.mockResolvedValue({ printers: [{ name: "Lab", default: true, status: "ready" }, { name: "Old", default: false, status: "offline", detail: "The printer is offline." }], driver: "fake", default: "Lab", note: "The spooler cannot prove that paper came out." });
  h.api.printerCapabilities.mockResolvedValue(caps);
  h.api.printPreview.mockResolvedValue(preview);
  h.api.printMedia.mockResolvedValue(job);
});
afterEach(cleanup);

const printBtn = () => screen.getByRole("button", { name: /^Print/ }) as HTMLButtonElement;

describe("PrintPanel", () => {
  it("shows the printer's own paper sizes and the real placement preview", async () => {
    render(<PrintPanel mediaId="M1" />);
    await screen.findByAltText("Print preview");
    expect(screen.getByRole("option", { name: /4x6 in \(4\.00×6\.00 in\)/ })).toBeTruthy();
    expect(screen.getByText(/white space on the sheet: left 25\.4 mm/i)).toBeTruthy();
    expect(screen.getByText(/never the raw camera original/)).toBeTruthy();
    expect(screen.getByText(/cannot prove that paper came out/)).toBeTruthy();
  });

  it("sends one print with a unique request id, then a new id for the next action", async () => {
    render(<PrintPanel mediaId="M1" templateId="strip-2x6-4" />);
    await screen.findByAltText("Print preview");
    fireEvent.click(printBtn());
    await waitFor(() => expect(h.api.printMedia).toHaveBeenCalledTimes(1));
    const first = h.api.printMedia.mock.calls[0][0];
    expect(first).toMatchObject({ mediaId: "M1", templateId: "strip-2x6-4", printer: "Lab", paper: "4x6 in", copies: 1 });
    expect(first.requestId).toBeTruthy();
    await waitFor(() => expect(printBtn().disabled).toBe(false));
    fireEvent.click(printBtn());
    await waitFor(() => expect(h.api.printMedia).toHaveBeenCalledTimes(2));
    expect(h.api.printMedia.mock.calls[1][0].requestId).not.toBe(first.requestId);
  });

  it("blocks printing to an offline printer and says why", async () => {
    render(<PrintPanel mediaId="M1" />);
    await screen.findByAltText("Print preview");
    fireEvent.change(screen.getByLabelText("Printer"), { target: { value: "Old" } });
    await waitFor(() => expect(screen.getByText(/Old is offline/)).toBeTruthy());
    expect(printBtn().disabled).toBe(true);
    expect(h.api.printMedia).not.toHaveBeenCalled();
  });

  it("shows a duplicate-print refusal without losing the panel", async () => {
    h.api.printMedia.mockRejectedValueOnce(new Error("an identical print was just submitted (3s ago); use Reprint to print again"));
    render(<PrintPanel mediaId="M1" />);
    await screen.findByAltText("Print preview");
    fireEvent.click(printBtn());
    await waitFor(() => expect(screen.getByRole("alert").textContent).toMatch(/identical print/));
    expect(screen.getByAltText("Print preview")).toBeTruthy();
  });

  it("sends the chosen paper, orientation, placement and copies", async () => {
    render(<PrintPanel mediaId="M1" />);
    await screen.findByAltText("Print preview");
    fireEvent.change(screen.getByLabelText("Paper"), { target: { value: "5x7 in" } });
    fireEvent.change(screen.getByLabelText("Orientation"), { target: { value: "landscape" } });
    fireEvent.change(screen.getByLabelText("Placement"), { target: { value: "fill" } });
    fireEvent.change(screen.getByLabelText("Copies"), { target: { value: "3" } });
    await waitFor(() => expect(h.api.printPreview).toHaveBeenLastCalledWith(expect.objectContaining({ paper: "5x7 in", orientation: "landscape", fit: "fill" })));
    await screen.findByAltText("Print preview");
    fireEvent.click(printBtn());
    await waitFor(() => expect(h.api.printMedia).toHaveBeenCalledWith(expect.objectContaining({ paper: "5x7 in", orientation: "landscape", fit: "fill", copies: 3 })));
  });

  it("explains a failed preview and keeps Print disabled", async () => {
    h.api.printPreview.mockRejectedValue(Object.assign(new Error("This image was not made with the layout “Single photo”."), { action: "Choose the layout it was created with." }));
    render(<PrintPanel mediaId="M1" templateId="single-4x6" />);
    await screen.findByText(/not made with the layout/);
    expect(printBtn().disabled).toBe(true);
  });

  it("offers two strips on one 4x6 sheet, shows that sheet in the preview and sends the choice with the job", async () => {
    h.api.printPreview.mockImplementation(async (o: { twoUp: boolean }) => ({ ...preview, twoUpOffer: true, twoUp: o.twoUp, imageInches: o.twoUp ? [4, 6] : [2, 6] }));
    render(<PrintPanel mediaId="M1" />);
    await waitFor(() => expect(screen.getByText(/image 4\.00×6\.00 in/)).toBeTruthy());         // two-up is the default for a strip on 4x6
    const box = screen.getByRole("checkbox", { name: /Two strips side by side/ }) as HTMLInputElement;
    expect(box.checked).toBe(true);
    fireEvent.click(printBtn());
    await waitFor(() => expect(h.api.printMedia).toHaveBeenCalled());
    expect(h.api.printMedia.mock.calls[0][0].twoUp).toBe(true);
    fireEvent.click(box);                                                                          // the operator can still print a single strip
    await waitFor(() => expect(screen.getByText(/image 2\.00×6\.00 in/)).toBeTruthy());
    expect(h.api.printPreview.mock.calls.at(-1)![0].twoUp).toBe(false);
  });

  it("does not show the two-up option for images that are not strips", async () => {
    render(<PrintPanel mediaId="M1" />);
    await screen.findByAltText("Print preview");
    expect(screen.queryByRole("checkbox", { name: /Two strips/ })).toBeNull();
  });
  it("ignores old capabilities and blocks printing until the latest preview arrives", async () => {
    let resolveOld!: (v: typeof caps) => void;
    let resolvePreview!: (v: typeof preview) => void;
    h.api.listPrinters.mockResolvedValue({ printers: [{ name: "Lab", status: "ready", default: true }, { name: "New", status: "ready" }], default: "Lab" });
    h.api.printerCapabilities.mockImplementation((name: string) => name === "Lab" ? new Promise((r) => { resolveOld = r; }) : Promise.resolve({ ...caps, papers: [caps.papers[1]] }));
    h.api.printPreview.mockImplementation(() => new Promise((r) => { resolvePreview = r; }));
    render(<PrintPanel mediaId="M1" />);
    await waitFor(() => expect(resolveOld).toBeDefined());
    fireEvent.change(screen.getByLabelText("Printer"), { target: { value: "New" } });
    await waitFor(() => expect(screen.getByRole("option", { name: /5x7 in/ })).toBeTruthy());
    await act(async () => resolveOld(caps));
    expect(screen.queryByRole("option", { name: /4x6 in/ })).toBeNull();
    expect(printBtn().disabled).toBe(true);
    await waitFor(() => expect(resolvePreview).toBeDefined());
    await act(async () => resolvePreview(preview));
    await waitFor(() => expect(printBtn().disabled).toBe(false));
  });

});
