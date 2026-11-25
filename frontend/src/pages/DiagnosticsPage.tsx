import { useEffect, useState } from "react";
import { api } from "../api";
import { Icon } from "../icons";
import { PageHeader, StateBlock, StatusPill } from "../operator/parts";
import type { View } from "../operator/status";
import { useApp } from "../state";
import { useT } from "../state";
import { ErrorNote, useAction } from "../ui";
import type { Check, Diagnostics } from "../types";

/** Each check answers: what is wrong, does it stop guests, what to do, can I retry. */
const viewOfCheck = (c: Check): View => (c.state === "problem" ? "setup" : c.state === "warn" ? "warning" : c.state === "info" ? "info" : "ready");
const pillLabel = (c: Check, t: ReturnType<typeof useT>["t"]) => (c.state === "problem" ? (c.blocksGuests ? t("stopsGuests") : t("problem")) : c.state === "warn" ? t("warning") : c.state === "info" ? t("notNeeded") : t("ok"));

export function DiagnosticsPage() {
  const { toast } = useApp();
  const { t, tf } = useT();
  const [d, setD] = useState<Diagnostics | null>(null);
  const { busy, error, run } = useAction();
  const sum = useAction();
  const bundle = useAction();
  const [summary, setSummary] = useState("");
  const load = () => run(async () => setD(await api.diagnostics()));
  useEffect(() => { load(); }, []); // eslint-disable-line react-hooks/exhaustive-deps

  const copy = () => sum.run(async () => {
    const text = await api.diagnosticSummary();
    setSummary(text);
    try { await navigator.clipboard.writeText(text); toast(t("supportSummaryCopied")); } catch { toast(t("copyClipboardUnavailable"), "err"); }
  });

  const exportBundle = () => bundle.run(async () => {
    const folder = await api.pickFolder();
    if (!folder) return;
    const path = await api.exportSupportBundle(folder);
    toast(tf("supportBundleSaved", { path }));
  });

  const blocking = d?.checks.filter((c) => c.blocksGuests) ?? [];
  const order = (c: Check) => (c.blocksGuests ? 0 : c.state === "problem" ? 1 : c.state === "warn" ? 2 : c.state === "info" ? 4 : 3);
  const checks = [...(d?.checks ?? [])].sort((a, b) => order(a) - order(b));
  return (
    <div className="page">
      <PageHeader title={t("diagnostics")} sub={t("diagnosticSubtitle")}>
        <button onClick={exportBundle} disabled={bundle.busy}>{t("exportSupportBundle")}</button>
        <button onClick={copy} disabled={sum.busy}><Icon name="upload" size={16} /> {t("copySupportSummary")}</button>
        <button onClick={load} disabled={busy}>{busy ? t("checking") : t("runAgain")}</button>
      </PageHeader>
      <ErrorNote error={error ?? sum.error ?? bundle.error} />
      {!d && !error && <StateBlock kind="loading">{t("checkingComputer")}</StateBlock>}
      {d && (<>
        <div className={`note ${blocking.length ? "err" : "info"}`} role="status"><div>{blocking.length ? tf(blocking.length === 1 ? "stopsGuestOne" : "stopsGuestsCount", { count: blocking.length }) : t("nothingStopsGuests")} {t("notNeededSetupHint")}</div></div>
        <section className="card"><h3>{t("checkLabel")} <span className="muted small">{d.platform}</span></h3>
          <ul className="diag-list">
            {checks.map((c, i) => (
              <li key={i} className={`diag ${c.state}`}>
                <StatusPill view={viewOfCheck(c)} label={pillLabel(c, t)} />
                <div><strong>{c.name}</strong><span className="muted"> {c.detail}</span>
                  {c.state !== "ok" && c.action && <div className="action">{c.action}{c.state === "problem" || c.state === "warn" ? " It is safe to run the check again." : ""}</div>}</div>
              </li>))}
          </ul>
        </section>
        <section className="card"><h3>{t("canonVerificationTitle")}</h3>
          <ul>{d.movieFindings.map((m, i) => <li key={i}>{m}</li>)}</ul>
          <p className="muted small">{t("verificationLevels")}</p>
        </section>
        <section className="card"><h3>{t("privacyOffline")}</h3><p>{d.network}</p><p className="muted small">{t("logsLabel")} {d.logDir}</p>
          <p className="muted small">{t("supportBundlePrivacy")}</p></section>
        {summary && <section className="card"><h3>{t("supportSummary")}</h3><textarea className="summary" readOnly value={summary} rows={14} aria-label={t("supportSummary")} onFocus={(e) => e.currentTarget.select()} /></section>}
      </>)}
    </div>
  );
}
