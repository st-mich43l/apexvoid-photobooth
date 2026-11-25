import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { AppError } from "./api";
import { GuestLocale } from "./locale";
import { ErrorNote } from "./ui";
afterEach(cleanup);
it("localizes a coded error and keeps technical details available", () => {
  render(<GuestLocale language="vi"><ErrorNote error={new AppError("EDSDK 0x81", "Reconnect USB", "camera_error")} /></GuestLocale>);
  expect(screen.getByText(/Không thể hoàn tất thao tác máy ảnh/)).toBeTruthy();
  expect(screen.getByText("EDSDK 0x81")).toBeTruthy();
  expect(screen.getByText("Reconnect USB")).toBeTruthy();
  expect(screen.getByText("Chi tiết kỹ thuật (tiếng Anh)")).toBeTruthy();
});
