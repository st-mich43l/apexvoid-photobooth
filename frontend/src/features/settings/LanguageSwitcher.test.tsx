import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { Settings } from "../../shared/types";
const h = vi.hoisted(() => ({ value: {} as Record<string, unknown>, patch: vi.fn() }));
vi.mock("../../app/state", () => ({ useApp: () => h.value }));
vi.mock("../../shared/api", () => ({ api: { patchSettings: h.patch } }));
import { LanguageSwitcher } from "./LanguageSwitcher";
function Harness() {
  const [settings, setSettings] = useState({ language: "en", countdownSeconds: 7 } as Settings);
  const [count, setCount] = useState(0);
  h.value = { settings, setSettings };
  return <><LanguageSwitcher /><button onClick={() => setCount(count + 1)}>State {count}</button><span>{settings.countdownSeconds}</span></>;
}
afterEach(() => { cleanup(); h.patch.mockReset(); });
it("switches immediately and patches only the language without remounting", async () => {
  let finish!: () => void;
  h.patch.mockReturnValue(new Promise<void>(resolve => { finish = resolve; }));
  render(<Harness />);
  fireEvent.click(screen.getByText("State 0"));
  fireEvent.change(screen.getByLabelText("Interface language"), { target: { value: "vi" } });
  expect(screen.getByText("State 1")).toBeTruthy();
  expect((screen.getByLabelText("Ngôn ngữ giao diện") as HTMLSelectElement).value).toBe("vi");
  expect(h.patch).toHaveBeenCalledWith({ language: "vi" });
  finish();
  await waitFor(() => expect((screen.getByLabelText("Ngôn ngữ giao diện") as HTMLSelectElement).disabled).toBe(false));
});
it("restores the previous language and reports a failed save", async () => {
  h.patch.mockRejectedValue(new Error("disk unavailable"));
  render(<Harness />);
  fireEvent.change(screen.getByLabelText("Interface language"), { target: { value: "vi" } });
  await screen.findByRole("alert");
  expect((screen.getByLabelText("Interface language") as HTMLSelectElement).value).toBe("en");
  expect(screen.getByText("7")).toBeTruthy();
});
