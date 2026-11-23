import { translate } from "../i18n";
import { useEffect, useRef, useState } from "react";
import { api } from "../api";
import { useApp } from "../state";
import { Badge, Confirm, ErrorNote, useAction } from "../ui";
import { bytes } from "../format";
import { MigrationDialog } from "../MigrationDialog";
import type { Report, Settings as S } from "../types";

export function SettingsPage() {
  const { settings, setSettings, toast, bumpLibrary, location } = useApp();
  const t = (key: Parameters<typeof translate>[1]) => translate(settings?.language, key);
  const [migrate, setMigrate] = useState(false);
  const [pinCur, setPinCur] = useState("");
  const [pinNew, setPinNew] = useState("");
  const pin = useAction();
  const [draft, setDraft] = useState<S>(settings);
  // what the draft was based on: a background update (a camera chosen on another page, a preselected frame…) is taken over
  // for every setting this page has not touched, so saving can never roll those back with a stale copy
  const base = useRef<S>(settings);
  useEffect(() => {
    const old = base.current;
    setDraft((d) => {
      const next = { ...d } as Record<string, unknown>;
      for (const k of Object.keys(settings) as (keyof S)[]) {
        if (JSON.stringify(d[k]) === JSON.stringify(old[k])) next[k as string] = settings[k];
      }
      return next as unknown as S;
    });
    base.current = settings;
  }, [settings]);
  const [confirmTrash, setConfirmTrash] = useState(false);
  const [report, setReport] = useState<Report | null>(null);
  const save = useAction();
  const other = useAction();
  const dirty = JSON.stringify(draft) !== JSON.stringify(settings);
  const set = <K extends keyof S>(k: K, v: S[K]) => setDraft((d) => ({ ...d, [k]: v }));

  return (
    <div className="page">
      <h1>{translate(settings?.language, "settings")}</h1>
      <ErrorNote error={save.error ?? other.error} />
      <section className="card"><h3>Language / Ngôn ngữ</h3>
        <label>{t("interfaceLanguage")}<select value={draft.language ?? "en"} onChange={(e) => set("language", e.target.value as "en" | "vi")}>
          <option value="en">English</option><option value="vi">Tiếng Việt (preview — partial)</option>
        </select></label>
        <p className="muted small">{t("translationPreview")}</p>
      </section>
      <section className="card"><h3>{translate(settings?.language, "photoSessions")}</h3>
        <div className="form">
          <label>{t("countdownSeconds")}<input type="number" min={1} max={10} value={draft.countdownSeconds} onChange={(e) => set("countdownSeconds", Number(e.target.value))} /></label>
          <p className="muted small">{t("shotsFromFrame")}</p>
          <label>{t("outputResolution")}
            <select aria-label="Print resolution" value={draft.outputDpi || 300} onChange={(e) => set("outputDpi", Number(e.target.value))}>
              <option value={300}>300 dpi — standard (2×6 strip = 600×1800 px)</option>
              <option value={600}>600 dpi — high (2×6 strip = 1200×3600 px; larger files, slower)</option>
            </select>
          </label>
          <p className="muted small">{t("resolutionNext")}</p>
          <label className="check"><input type="checkbox" checked={!!draft.previewMirror} onChange={(e) => set("previewMirror", e.target.checked)} /> {t("mirrorPreview")}</label>
          <label className="check"><input type="checkbox" checked={draft.autoReconnect} onChange={(e) => set("autoReconnect", e.target.checked)} /> {t("autoReconnect")}</label>
        </div>
      </section>

      <section className="card"><h3>{translate(settings?.language, "mediaLibrary")}</h3>
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
          <button onClick={() => other.run(() => api.openLibraryFolder())}>{t("openLibrary")}</button>
          {location.migrationPending && <button className="primary" onClick={() => setMigrate(true)}>{t("moveDocuments")}</button>}
        </div>
      </section>

      <section className="card"><h3>{translate(settings?.language, "preferredCameras")}</h3>
        <p className="muted small">{t("preferencesHint")}</p>
        <table className="table"><tbody>
          <tr><td>{t("photoCamera")}</td><td>{settings.photoCamera.name || <span className="muted">{t("notSet")}</span>}</td>
            <td>{settings.photoCamera.name && <button className="ghost" onClick={() => other.run(async () => { await api.clearPreferredCamera("photo"); toast("Photo camera cleared"); })}>{t("clear")}</button>}</td></tr>
          <tr><td>{t("videoCamera")}</td><td>{settings.videoCamera.name || <span className="muted">{t("notSet")}</span>}</td>
            <td>{settings.videoCamera.name && <button className="ghost" onClick={() => other.run(async () => { await api.clearPreferredCamera("video"); toast("Video camera cleared"); })}>{t("clear")}</button>}</td></tr>
        </tbody></table>
      </section>

      <section className="card"><h3>{translate(settings?.language, "printingDefaults")}</h3>
        <table className="table"><tbody>
          <tr><td>{t("defaultPrinter")}</td><td>{settings.printerName || <span className="muted">Windows default printer</span>}</td></tr>
          <tr><td>{t("defaultPaper")}</td><td>{settings.printPaper || <span className="muted">{t("notSet")}</span>}</td></tr>
        </tbody></table>
        <p className="muted small">{t("printerPageHint")}</p>
      </section>

      <section className="card"><h3>{t("kioskPin")}</h3>
        <p className="muted small">{t("kioskPinHint")}</p>
        <ErrorNote error={pin.error} onClose={pin.clear} />
        <div className="form">
          {settings.kioskPinSet && <label>{t("currentPin")}<input type="password" inputMode="numeric" autoComplete="off" maxLength={8} value={pinCur} onChange={(e) => setPinCur(e.target.value.replace(/\D/g, ""))} /></label>}
          <label>{settings.kioskPinSet ? t("newPin") : "PIN"}<input type="password" inputMode="numeric" autoComplete="off" maxLength={8} value={pinNew} onChange={(e) => setPinNew(e.target.value.replace(/\D/g, ""))} /></label>
        </div>
        <div className="row">
          <button className="primary" disabled={pin.busy || pinNew.length < 4} onClick={() => pin.run(async () => { await api.setKioskPin(pinCur, pinNew); setPinCur(""); setPinNew(""); toast(settings.kioskPinSet ? "PIN changed" : "PIN set"); })}>{settings.kioskPinSet ? "Change PIN" : "Set PIN"}</button>
          {settings.kioskPinSet && <button className="danger" disabled={pin.busy || pinCur.length < 4} onClick={() => pin.run(async () => { await api.setKioskPin(pinCur, ""); setPinCur(""); toast("PIN removed"); })}>{t("removePin")}</button>}
        </div>
      </section>

      <section className="card"><h3>{translate(settings?.language, "storage")}</h3>
        <div className="form">
          <label>Custom media folder (empty = default: Documents\apexvoid-studio)
            <div className="row">
              <input value={draft.mediaDir} placeholder="Default application data folder" onChange={(e) => set("mediaDir", e.target.value)} />
              <button onClick={() => other.run(async () => { const p = await api.pickFolder(); if (p) set("mediaDir", p); })}>{t("browse")}</button>
            </div>
          </label>
        </div>
        <p className="muted small">{t("folderRestart")}</p>
        <div className="row">
          <button onClick={() => other.run(async () => { setReport(await api.verifyLibrary(true)); bumpLibrary(); })}>{t("verifyLibrary")}</button>
          <button className="danger" onClick={() => setConfirmTrash(true)}>{t("emptyTrash")}</button>
        </div>
        {report && <p className="muted small">Checked {report.checked}. Missing: {report.missing.length}. Corrupt: {report.corrupt.length}. Recovered: {report.recovered.length}. Interrupted sessions: {report.interrupted.length}.</p>}
      </section>

      <section className="card"><h3>{translate(settings?.language, "externalComponents")}</h3>
        <div className="form">
          <label>FFmpeg path (empty = next to the app, then PATH)<input value={draft.ffmpegPath} onChange={(e) => set("ffmpegPath", e.target.value)} placeholder="C:\ffmpeg\bin\ffmpeg.exe" /></label>
          <label>Canon EDSDK folder (empty = next to the app / ./sdk/canon)<input value={draft.canonSdkDir} onChange={(e) => set("canonSdkDir", e.target.value)} placeholder="C:\Canon\EDSDK_64\Dll" /></label>
        </div>
        <p className="muted small">The Canon SDK is proprietary and is not bundled. See the installation guide.</p>
      </section>

      <section className="card"><h3>{translate(settings?.language, "development")}</h3>
        <label className="check"><input type="checkbox" checked={draft.enableMock} onChange={(e) => set("enableMock", e.target.checked)} /> {t("simulatedCamera")}</label>
      </section>

      <div className="savebar">
        <button disabled={!dirty} onClick={() => setDraft(settings)}>{translate(settings?.language, "discard")}</button>
        <button className="primary" disabled={!dirty || save.busy} onClick={() => save.run(async () => {
          // only what this page changed is sent; the backend merges it into its current settings
          const changed: Record<string, unknown> = {};
          for (const k of Object.keys(draft) as (keyof S)[]) if (JSON.stringify(draft[k]) !== JSON.stringify(settings[k])) changed[k as string] = draft[k];
          const r = await api.patchSettings(changed as Partial<S>);
          if (r.settings) { setSettings(r.settings); setDraft(r.settings); }
          toast(r.restartRequired ? "Saved — restart the app to use the new media folder" : "Settings saved");
        })}>{translate(settings?.language, "saveSettings")}</button>
      </div>
      {migrate && <MigrationDialog onClose={() => setMigrate(false)} />}
      {confirmTrash && <Confirm title="Empty trash?" danger confirmLabel="Delete permanently" onNo={() => setConfirmTrash(false)}
        body={<p>All items in the trash are erased from disk. This cannot be undone.</p>}
        onYes={() => other.run(async () => { const n = await api.emptyTrash(); setConfirmTrash(false); toast(`${n} item(s) deleted permanently`); })} />}
    </div>
  );
}
