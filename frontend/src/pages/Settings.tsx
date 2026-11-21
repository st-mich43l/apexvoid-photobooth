import { useState } from "react";
import { api } from "../api";
import { useApp } from "../state";
import { Badge, Confirm, ErrorNote, useAction } from "../ui";
import { bytes } from "../format";
import { MigrationDialog } from "../MigrationDialog";
import type { Report, Settings as S } from "../types";

export function SettingsPage() {
  const { settings, setSettings, toast, bumpLibrary, location } = useApp();
  const [migrate, setMigrate] = useState(false);
  const [pinCur, setPinCur] = useState("");
  const [pinNew, setPinNew] = useState("");
  const pin = useAction();
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

      <section className="card"><h3>Media library</h3>
        <p className="big-line path">{location.path}</p>
        <p className="muted small">
          {location.source === "documents" && <Badge kind="ok">Documents\apexvoid-studio</Badge>}
          {location.source === "custom" && <Badge kind="info">custom folder</Badge>}
          {location.source === "legacy" && <Badge kind="warn">old folder — not migrated yet</Badge>}
          {location.source === "fallback" && <Badge kind="err">fallback folder</Badge>}
          {" "}{bytes(location.freeBytes)} free. Photos, videos, thumbnails, edits, strips, exports, templates, trash and logs all live here; settings are kept in <code>{location.configPath}</code>.
        </p>
        {location.warning && <div className="note warn"><div>{location.warning}</div></div>}
        <div className="row">
          <button onClick={() => other.run(() => api.openLibraryFolder())}>Open Library Folder</button>
          {location.migrationPending && <button className="primary" onClick={() => setMigrate(true)}>Move library to Documents…</button>}
        </div>
      </section>

      <section className="card"><h3>Preferred cameras</h3>
        <p className="muted small">Restored when the app starts. If a preferred camera is missing the app asks before using another one. Choose them on the Camera page.</p>
        <table className="table"><tbody>
          <tr><td>Photo camera</td><td>{settings.photoCamera.name || <span className="muted">not set</span>}</td>
            <td>{settings.photoCamera.name && <button className="ghost" onClick={() => other.run(async () => { await api.clearPreferredCamera("photo"); toast("Photo camera cleared"); })}>Clear</button>}</td></tr>
          <tr><td>Video camera</td><td>{settings.videoCamera.name || <span className="muted">not set</span>}</td>
            <td>{settings.videoCamera.name && <button className="ghost" onClick={() => other.run(async () => { await api.clearPreferredCamera("video"); toast("Video camera cleared"); })}>Clear</button>}</td></tr>
        </tbody></table>
      </section>

      <section className="card"><h3>Kiosk exit PIN</h3>
        <p className="muted small">When set, leaving Booth Mode requires this PIN (4–8 digits). Five wrong attempts lock the PIN prompt for 30 seconds. Leave empty to protect nothing.</p>
        <ErrorNote error={pin.error} onClose={pin.clear} />
        <div className="form">
          {settings.kioskPinSet && <label>Current PIN<input type="password" inputMode="numeric" autoComplete="off" maxLength={8} value={pinCur} onChange={(e) => setPinCur(e.target.value.replace(/\D/g, ""))} /></label>}
          <label>{settings.kioskPinSet ? "New PIN" : "PIN"}<input type="password" inputMode="numeric" autoComplete="off" maxLength={8} value={pinNew} onChange={(e) => setPinNew(e.target.value.replace(/\D/g, ""))} /></label>
        </div>
        <div className="row">
          <button className="primary" disabled={pin.busy || pinNew.length < 4} onClick={() => pin.run(async () => { await api.setKioskPin(pinCur, pinNew); setPinCur(""); setPinNew(""); toast(settings.kioskPinSet ? "PIN changed" : "PIN set"); })}>{settings.kioskPinSet ? "Change PIN" : "Set PIN"}</button>
          {settings.kioskPinSet && <button className="danger" disabled={pin.busy || pinCur.length < 4} onClick={() => pin.run(async () => { await api.setKioskPin(pinCur, ""); setPinCur(""); toast("PIN removed"); })}>Remove PIN</button>}
        </div>
      </section>

      <section className="card"><h3>Storage</h3>
        <div className="form">
          <label>Custom media folder (empty = default: Documents\apexvoid-studio)
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
      {migrate && <MigrationDialog onClose={() => setMigrate(false)} />}
      {confirmTrash && <Confirm title="Empty trash?" danger confirmLabel="Delete permanently" onNo={() => setConfirmTrash(false)}
        body={<p>All items in the trash are erased from disk. This cannot be undone.</p>}
        onYes={() => other.run(async () => { const n = await api.emptyTrash(); setConfirmTrash(false); toast(`${n} item(s) deleted permanently`); })} />}
    </div>
  );
}
