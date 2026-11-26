import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, act } from "@testing-library/react";
import { MediaImage } from "./MediaImage";

afterEach(() => { cleanup(); vi.useRealTimers(); });

describe("MediaImage", () => {
  it("reports ready only after a decoded image has nonzero dimensions", () => {
    const status = vi.fn();
    const { container } = render(<MediaImage src="/photo" alt="photo" onStatus={status} />);
    const image = container.querySelector("img")!;
    Object.defineProperties(image, { naturalWidth: { configurable: true, value: 1600 }, naturalHeight: { configurable: true, value: 1200 } });
    fireEvent.load(image);
    expect(container.querySelector(".media-image")?.getAttribute("data-state")).toBe("ready");
    expect(status).toHaveBeenLastCalledWith(true);
  });

  it("retries failed requests twice, then exposes a manual retry", () => {
    vi.useFakeTimers();
    const { container } = render(<MediaImage src="/photo" alt="photo" />);
    fireEvent.error(container.querySelector("img")!);
    act(() => { vi.advanceTimersByTime(500); });
    expect(container.querySelector("img")?.getAttribute("src")).toContain("retry=1");
    fireEvent.error(container.querySelector("img")!);
    act(() => { vi.advanceTimersByTime(1400); });
    expect(container.querySelector("img")?.getAttribute("src")).toContain("retry=2");
    fireEvent.error(container.querySelector("img")!);
    expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(container.querySelector("img")?.getAttribute("src")).toContain("retry=3");
  });

  it("cancels a pending request timeout when unmounted", () => {
    vi.useFakeTimers();
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const { unmount } = render(<MediaImage src="/pending" alt="photo" />);
    unmount();
    act(() => { vi.advanceTimersByTime(11000); });
    expect(warn).not.toHaveBeenCalled();
    warn.mockRestore();
  });
});
