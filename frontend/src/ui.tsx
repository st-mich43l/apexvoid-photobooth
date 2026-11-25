import { useEffect, useRef, useState, type ReactNode } from "react";
import { useApp } from "./state";
import { AppError, toAppError } from "./api";
import type { Support } from "./types";

export function Badge({ kind, children }: { kind: "ok" | "warn" | "err" | "info" | "sim"; children: ReactNode }) {
  return <span className={`badge ${kind}`}>{children}</span>;
}

export function SupportBadge({ v }: { v: Support }) {
  if (v === "supported") return <Badge kind="ok">supported</Badge>;
  if (v === "unsupported") return <Badge kind="err">unsupported</Badge>;
  return <Badge kind="warn">unverified</Badge>;
}

/** Message + action, as produced by app.UserError. */
export function ErrorNote({ error, onClose }: { error: AppError | string | null | undefined; onClose?: () => void }) {
  if (!error) return null;
  const e = typeof error === "string" ? new AppError(error) : error;
  return (
    <div className="note err" role="alert">
      <div><strong>{e.message}</strong>{e.action && <div className="action">{e.action}</div>}</div>
      {onClose && <button className="ghost" onClick={onClose} aria-label="Dismiss">✕</button>}
    </div>
  );
}

/** Runs an async action, tracking busy + error. */
export function useAction() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<AppError | null>(null);
  const run = async <T,>(f: () => Promise<T>): Promise<T | undefined> => {
    setBusy(true); setError(null);
    try { return await f(); } catch (e) { setError(toAppError(e)); return undefined; } finally { setBusy(false); }
  };
  return { busy, error, run, clear: () => setError(null) };
}

/** The simulated camera must never look like a real one. */
export function SimulatedRibbon() {
  const { camera } = useApp();
  if (!camera.device?.simulated) return null;
  return <div className="ribbon" role="alert">SIMULATED CAMERA — images are test patterns, not real photographs</div>;
}

/** Live view: an <img> on the loopback MJPEG stream, re-opened after drops. */
export function LivePreview({ className = "", fit = "contain", objectPosition = "50% 50%" }: { className?: string; fit?: "contain" | "cover"; objectPosition?: string }) {
  const { camera, previewUrl, settings } = useApp();
  const [failed, setFailed] = useState(0);
  const timer = useRef<number | undefined>(undefined);
  useEffect(() => () => window.clearTimeout(timer.current), []);
  const on = camera.liveView && camera.state !== "disconnected" && camera.state !== "error";
  useEffect(() => setFailed(0), [previewUrl]);
  if (!on) {
    return (
      <div className={`preview empty ${className}`}>
        <div>
          {camera.reconnecting ? "Camera disconnected — reconnecting…" : camera.device ? "Live view is off" : "No camera connected"}
        </div>
      </div>
    );
  }
  return (
    <div className={`preview ${settings?.previewMirror ? "mirrored" : ""} ${className}`}>
      <img
        key={`${previewUrl}-${failed}`}
        src={previewUrl}
        alt="Live camera preview"
        style={{ objectFit: fit, objectPosition }}
        onError={() => { timer.current = window.setTimeout(() => setFailed((n) => n + 1), 800); }}
      />
    </div>
  );
}

const FOCUSABLE = 'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

/** A dialog that behaves like one: focus moves inside, Tab stays inside, Escape closes, and focus returns to where it was. */
export function Modal({ title, onClose, children, wide }: { title: string; onClose: () => void; children: ReactNode; wide?: boolean }) {
  const box = useRef<HTMLDivElement>(null);
  const closeRef = useRef(onClose);
  closeRef.current = onClose;
  useEffect(() => {
    const before = document.activeElement as HTMLElement | null;
    const node = box.current;
    // an element inside that asked for focus (autoFocus, the safe default button) keeps it; otherwise the first control
    if (node && !node.contains(document.activeElement)) (node.querySelector<HTMLElement>("footer button, " + FOCUSABLE) ?? node).focus();
    const f = (e: KeyboardEvent) => {
      if (e.key === "Escape") { e.stopPropagation(); closeRef.current(); return; }
      if (e.key !== "Tab" || !node) return;
      const items = Array.from(node.querySelectorAll<HTMLElement>(FOCUSABLE));
      if (items.length === 0) { e.preventDefault(); return; }
      const first = items[0], last = items[items.length - 1], cur = document.activeElement;
      if (e.shiftKey && (cur === first || !node.contains(cur))) { e.preventDefault(); last.focus(); }
      else if (!e.shiftKey && (cur === last || !node.contains(cur))) { e.preventDefault(); first.focus(); }
    };
    window.addEventListener("keydown", f);
    return () => { window.removeEventListener("keydown", f); if (before && document.contains(before)) before.focus(); };
  }, []);
  return (
    <div className="modal-back" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div ref={box} className={`modal ${wide ? "wide" : ""}`} role="dialog" aria-modal="true" aria-label={title} tabIndex={-1}>
        <header><h2>{title}</h2><button className="ghost" onClick={onClose} aria-label="Close">✕</button></header>
        {children}
      </div>
    </div>
  );
}

export function Confirm({ title, body, confirmLabel, danger, onYes, onNo }:
  { title: string; body: ReactNode; confirmLabel: string; danger?: boolean; onYes: () => void; onNo: () => void }) {
  return (
    <Modal title={title} onClose={onNo}>
      <div className="modal-body">{body}</div>
      <footer>
        <button onClick={onNo}>Cancel</button>
        <button className={danger ? "danger" : "primary"} onClick={onYes} autoFocus>{confirmLabel}</button>
      </footer>
    </Modal>
  );
}
