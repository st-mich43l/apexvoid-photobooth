import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useState } from "react";

vi.mock("../app/state", () => ({ useApp: () => ({ camera: {}, previewUrl: "", settings: {} }) }));
vi.mock("./api", () => ({ AppError: Error, toAppError: (e: unknown) => e }));
import { Modal } from "./ui";

afterEach(cleanup);

function Host({ onClose = () => {} }: { onClose?: () => void }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button onClick={() => setOpen(true)}>Open</button>
      {open && <Modal title="Sure?" onClose={() => { onClose(); setOpen(false); }}><div className="modal-body"><input aria-label="Name" /></div><footer><button>No</button><button>Yes</button></footer></Modal>}
    </>
  );
}

describe("Modal focus handling", () => {
  it("moves focus into the dialog, keeps Tab inside it, and gives focus back to the opener", () => {
    render(<Host />);
    const opener = screen.getByRole("button", { name: "Open" });
    opener.focus();
    fireEvent.click(opener);
    const dlg = screen.getByRole("dialog", { name: "Sure?" });
    expect(dlg.contains(document.activeElement)).toBe(true);
    const all = Array.from(dlg.querySelectorAll<HTMLElement>("button, input"));
    all[all.length - 1].focus();
    fireEvent.keyDown(window, { key: "Tab" });                         // past the last control: wraps to the first
    expect(document.activeElement).toBe(all[0]);
    fireEvent.keyDown(window, { key: "Tab", shiftKey: true });         // before the first: wraps to the last
    expect(document.activeElement).toBe(all[all.length - 1]);
    fireEvent.keyDown(window, { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(opener);
  });

  it("Escape closes once", () => {
    const onClose = vi.fn();
    render(<Host onClose={onClose} />);
    fireEvent.click(screen.getByRole("button", { name: "Open" }));
    fireEvent.keyDown(window, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("an element that asked for focus (a safe default) keeps it", () => {
    render(<Modal title="T" onClose={() => {}}><div className="modal-body"><button>Other</button><button autoFocus>Safe</button></div></Modal>);
    expect((document.activeElement as HTMLElement).textContent).toBe("Safe");
  });
});
