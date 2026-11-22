import type { ReactNode } from "react";
import { Icon } from "../icons";
import { Badge } from "../ui";
import { VIEW_LABEL, VIEW_TONE, type View } from "./status";

/** The same pill for the same state on every Studio page. */
export function StatusPill({ view, label }: { view: View; label?: string }) {
  return <Badge kind={VIEW_TONE[view]}>{label ?? VIEW_LABEL[view]}</Badge>;
}

/** One header for every Studio page: title, an optional one-line purpose, and the page's actions on the right. */
export function PageHeader({ title, sub, children }: { title: string; sub?: string; children?: ReactNode }) {
  return (
    <header className="page-head">
      <div><h1>{title}</h1>{sub && <p className="muted">{sub}</p>}</div>
      {children && <div className="page-actions">{children}</div>}
    </header>
  );
}

/** Loading / error / empty, the same way everywhere. */
export function StateBlock({ kind, children, action }: { kind: "loading" | "error" | "empty"; children: ReactNode; action?: ReactNode }) {
  return (
    <div className={`state-block ${kind}`} role={kind === "error" ? "alert" : "status"}>
      {kind === "loading" ? <span className="spinner" aria-hidden="true" /> : <Icon name={kind === "error" ? "diagnostics" : "photo"} size={22} />}
      <span>{children}</span>{action}
    </div>
  );
}

/** A Prepare Booth section: icon, title, state pill, body, and the one action that fixes it. */
export function PrepCard({ id, icon, title, view, pillLabel, children, actions }:
  { id: string; icon: string; title: string; view: View; pillLabel?: string; children: ReactNode; actions?: ReactNode }) {
  return (
    <section className={`prep-card v-${view}`} aria-labelledby={`prep-${id}`} data-view={view}>
      <header><span className="prep-icon"><Icon name={icon} size={20} /></span><h2 id={`prep-${id}`}>{title}</h2><StatusPill view={view} label={pillLabel} /></header>
      <div className="prep-body">{children}</div>
      {actions && <footer>{actions}</footer>}
    </section>
  );
}

/** A label/value row inside a PrepCard. */
export function Fact({ label, children }: { label: string; children: ReactNode }) {
  return <div className="fact"><dt>{label}</dt><dd>{children}</dd></div>;
}
