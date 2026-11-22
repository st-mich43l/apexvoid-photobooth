import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import type { CanonStatus } from "./types";

const S = vi.hoisted(() => ({ st: {} as unknown, camera: { state: "disconnected", reconnecting: false, liveView: false } }));
vi.mock("./api", () => ({ api: { canonStatus: async () => S.st } }));
vi.mock("./state", () => ({ useApp: () => ({ camera: S.camera }) }));

import { CanonPanel } from "./CanonPanel";

afterEach(cleanup);

const base = (): CanonStatus => ({
  platform: "windows/amd64", sdk: { kind: "", path: "C:\\sdk\\EDSDK.dll", machine: "x64" }, detected: [], conflicts: [],
  guide: ["Set the camera's USB connection app to Photo Import / Remote Control", "Use JPEG or RAW+JPEG"],
  verification: "Automated tests use a simulated Canon SDK; neither proves that a real EOS R50 works.",
  items: [],
});

describe("CanonPanel", () => {
  it("shows a missing SDK as a problem with what to do, and never as working", async () => {
    S.st = { ...base(), sdk: { kind: "not_found", problem: "EDSDK.dll not found" }, recommendation: "Copy the 64-bit EDSDK.dll next to the application.", items: [
      { key: "sdk", label: "Canon SDK files", state: "problem", detail: "EDSDK.dll not found", action: "Copy the 64-bit EDSDK.dll next to the application." },
      { key: "liveview", label: "Live view", state: "not_tried", detail: "Connect the camera first." },
    ] };
    render(<CanonPanel />);
    await waitFor(() => expect(screen.getByText("Canon SDK files")).toBeTruthy());
    expect(screen.getByText(/Next step:/).parentElement?.textContent).toMatch(/Copy the 64-bit EDSDK.dll/);
    const rows = screen.getAllByRole("row");
    expect(rows[0].getAttribute("data-state")).toBe("problem");
    expect(rows[0].textContent).toMatch(/problem/);
    expect(rows[1].textContent).toMatch(/not tried yet/);
    expect(screen.queryByText("working")).toBeNull();
  });

  it("calls a capability 'working' only when the backend says it was observed, and states what is unverified", async () => {
    S.st = { ...base(), items: [
      { key: "liveview", label: "Live view", state: "ok", detail: "Frames were received in this session." },
      { key: "still", label: "Still capture (shutter)", state: "not_tried", detail: "Not tried yet: take a test photo." },
    ], active: { name: "Canon EOS R50", interface: "USB (Canon EDSDK)", state: "liveview", liveView: true,
      capabilities: { liveView: "supported", stillCapture: "unverified", originalTransfer: "unverified", movieRecord: "unverified", movieTransfer: "unverified", streamedVideo: "unsupported" },
      counters: { captures: 3, lateTransfers: 1, invalidTransfers: 0, lastFile: "IMG_0003.JPG" } } };
    render(<CanonPanel />);
    await waitFor(() => expect(screen.getByText("Live view")).toBeTruthy());
    expect(screen.getAllByText("working")).toHaveLength(1);
    expect(screen.getAllByText("not tried yet")).toHaveLength(1);
    expect(screen.getByText(/3 photo\(s\) received, 1 late transfer\(s\) ignored; last file IMG_0003.JPG/)).toBeTruthy();
    expect(screen.getByText(/neither proves that a real EOS R50 works/)).toBeTruthy();
  });

  it("includes the Windows setup guide", async () => {
    S.st = { ...base(), items: [] };
    render(<CanonPanel />);
    await waitFor(() => expect(screen.getByText(/Setup guide for the Canon EOS R50/)).toBeTruthy());
    expect(screen.getByText(/Photo Import \/ Remote Control/)).toBeTruthy();
  });
});
