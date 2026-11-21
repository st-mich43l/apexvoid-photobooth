import { useState } from "react";
import { api } from "../api";
import { useApp } from "../state";
import { Confirm, ErrorNote, useAction } from "../ui";
import type { Report, Settings as S } from "../types";

export function SettingsPage() {
  const { settings, setSettings, toast, bumpLibrary } = useApp();
  const [draft, setDraft] = useState<S>(settings);
  const [confirmTrash, setConfirmTrash] = useState(false);
  const [report, setReport] = useState<Report | null>(null);
  const save = useAction();
  const other = useAction();
  const dirty = JSON.stringify(draft) !== JSON.stringify(settings);
  const set = <K extends keyof S>(k: K, v: S[K]) => setDraft((d) => ({ ...d, [k]: v }));

  return (
    <div className="page">
      <h1>Settings</h1>
      <ErrorNote error={save.error ?? other.error} />
      <section className="card"><h3>Photo sessions</h3>
        <div className="form">
          <label>Countdown (seconds)<input type="number" min={1} max={10} value={draft.countdownSeconds} onChange={(e) => set("countdownSeconds", Number(e.target.value))} /></label>
          <label>Photographs per session
            <select value={draft.shots} onChange={(e) => set("shots", Number(e.target.value))}>{[1, 2, 4, 6].map((n) => <option key={n}>{n}</option>)}</select>
          </label>
          <label className="check"><input type="checkbox" checked={draft.autoReconnect} onChange={(e) => set("autoReconnect", e.target.checked)} /> Reconnect the camera automatically after a USB drop</label>
        </div>
      </section>

      <section className="card"><h3>Storage</h3>
        <div className="form">
          <label>Media folder (empty = default)
            <div className="row">
              <input value={draft.mediaDir} placeholder="Default application data folder" onChange={(e) => set("mediaDir", e.target.value)} />
              <button onClick={() => other.run(async () => { const p = await api.pickFolder(); if (p) set("mediaDir", p); })}>Browse…</button>
            </div>
          </label>
        </div>
        <p className="muted small">Changing the folder takes effect after restarting the application; existing media is not moved automatically.</p>
        <div className="row">
          <button onClick={() => other.run(async () => { setReport(await api.verifyLibrary(true)); bumpLibrary(); })}>Verify library</button>
          <button className="danger" onClick={() => setConfirmTrash(true)}>Empty trash…</button>
        </div>
        {report && <p className="muted small">Checked {report.checked}. Missing: {report.missing.length}. Corrupt: {report.corrupt.length}. Recovered: {report.recovered.length}. Interrupted sessions: {report.interrupted.length}.</p>}
      </section>

      <section className="card"><h3>External components</h3>
        <div className="form">
          <label>FFmpeg path (empty = next to the app, then PATH)<input value={draft.ffmpegPath} onChange={(e) => set("ffmpegPath", e.target.value)} placeholder="C:\ffmpeg\bin\ffmpeg.exe" /></label>
          <label>Canon EDSDK folder (empty = next to the app / ./sdk/canon)<input value={draft.canonSdkDir} onChange={(e) => set("canonSdkDir", e.target.value)} placeholder="C:\Canon\EDSDK_64\Dll" /></label>
        </div>
        <p className="muted small">The Canon SDK is proprietary and is not bundled. See the installation guide.</p>
      </section>

      <section className="card"><h3>Development</h3>
        <label className="check"><input type="checkbox" checked={draft.enableMock} onChange={(e) => set("enableMock", e.target.checked)} /> Show the SIMULATED camera (test patterns — never real photographs)</label>
      </section>

      <div className="savebar">
        <button disabled={!dirty} onClick={() => setDraft(settings)}>Discard</button>
        <button className="primary" disabled={!dirty || save.busy} onClick={() => save.run(async () => {
          const r = await api.saveSettings(draft); setSettings(draft); toast(r.restartRequired ? "Saved — restart the app to use the new media folder" : "Settings saved");
        })}>Save settings</button>
      </div>
      {confirmTrash && <Confirm title="Empty trash?" danger confirmLabel="Delete permanently" onNo={() => setConfirmTrash(false)}
        body={<p>All items in the trash are erased from disk. This cannot be undone.</p>}
        onYes={() => other.run(async () => { const n = await api.emptyTrash(); setConfirmTrash(false); toast(`${n} item(s) deleted permanently`); })} />}
    </div>
  );
}
