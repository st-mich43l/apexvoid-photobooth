import { describe, expect, it, vi } from "vitest";
const h = vi.hoisted(() => ({ call: vi.fn() }));
vi.mock("@wailsio/runtime", () => ({ Call: { ByName: h.call }, Events: { On: vi.fn() } }));
import { api, toAppError } from "./api";
describe("structured PIN errors", () => {
  it("retains the stable code and retry duration from the Wails result", async () => {
    h.call.mockResolvedValueOnce({ ok: false, code: "pin_locked", retrySeconds: 18 });
    try { await api.verifyKioskPin("0000"); throw new Error("expected failure"); }
    catch (error) { const e = toAppError(error); expect(e.code).toBe("pin_locked"); expect(e.params?.seconds).toBe(18); }
    expect(h.call).toHaveBeenCalledWith(expect.stringContaining("VerifyKioskPinDetailed"), "0000");
  });
  it("preserves the legacy diagnostic message and action", () => {
    const error = toAppError(new Error("Camera failed\nCheck the USB cable"));
    expect(error.message).toBe("Camera failed"); expect(error.action).toBe("Check the USB cable");
  });
});
it("categorizes errors by the invoked operation while retaining diagnostics", async () => {
  h.call.mockRejectedValueOnce(new Error("EDSDK 0x81\nReconnect USB"));
  try { await api.connectCamera("canon:1"); throw new Error("expected failure"); }
  catch (error) { const e = toAppError(error); expect(e.code).toBe("camera_error"); expect(e.message).toBe("EDSDK 0x81"); expect(e.action).toBe("Reconnect USB"); }
});
