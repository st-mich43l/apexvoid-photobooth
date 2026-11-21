import { useEffect, useState } from "react";
import { api } from "../api";
import { useApp } from "../state";
import { Badge, ErrorNote, useAction } from "../ui";
import type { TemplatesResult } from "../types";

export function Templates() {
  const { settings, setSettings, toast } = useApp();
  const [data, setData] = useState<TemplatesResult | null>(null);
  const [previews, setPreviews] = useState<Record<string, string>>({});
  const { error, run } = useAction();

  useEffect(() => {
    api.listTemplates().then(async (t) => {
      setData(t);
      for (const x of t.templates) api.previewTemplate(x.id).then((u) => setPreviews((p) => ({ ...p, [x.id]: u }))).catch(() => {});
    }).catch(() => {});
  }, []);

  const save = (patch: Partial<typeof settings>) => run(async () => {
    const n = { ...settings, ...patch }; await api.saveSettings(n); setSettings(n); toast("Saved");
  });

  return (
    <div className="page">
      <h1>Layouts</h1>
      <ErrorNote error={error} />
      <p className="muted">Layouts are rendered at their exact print size and DPI; printers and print jobs are on the Printer page. Previews below use colour blocks, not photographs. Custom layouts: put a <code>.json</code> file (and an optional overlay <code>.png</code>) in <code>{data?.dir}</code>.</p>
      {data?.errors.map((e, i) => <div key={i} className="note warn">{e}</div>)}
      <div className="grid templates">
        {data?.templates.map((t) => (
          <section key={t.id} className={`card tpl ${settings.templateId === t.id ? "selected" : ""}`}>
            <div className="tpl-preview">{previews[t.id] ? <img src={previews[t.id]} alt={`${t.name} preview`} /> : <div className="spinner" />}</div>
            <h3>{t.name}</h3>
            <p className="muted small">{t.widthIn}×{t.heightIn} in · {t.dpi} dpi · {Math.round(t.widthIn * t.dpi)}×{Math.round(t.heightIn * t.dpi)} px · {new Set(t.slots.map((s) => s.photo)).size} photos {t.kind === "custom" && <Badge kind="info">custom</Badge>}</p>
            <button className={settings.templateId === t.id ? "" : "primary"} disabled={settings.templateId === t.id} onClick={() => save({ templateId: t.id })}>
              {settings.templateId === t.id ? "Default layout" : "Use as default"}
            </button>
          </section>
        ))}
      </div>
    </div>
  );
}
