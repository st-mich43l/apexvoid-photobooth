import { LanguageSwitcher } from "../LanguageSwitcher";
import { formatTranslation, translate } from "../../../shared/i18n";
import { useEffect, useRef, useState } from "react";
import { api } from "../../../shared/api";
import { useApp } from "../../../app/state";
import { Badge, Confirm, ErrorNote, LocalizedDiagnostic, useAction } from "../../../shared/ui";
import { bytes } from "../../../shared/format";
import { MigrationDialog } from "../MigrationDialog";
import type { Report, Settings as S } from "../../../shared/types";

export function SettingsPage() {
  const { settings, setSettings, toast, bumpLibrary, location } = useApp();
  const t = (key: Parameters<typeof translate>[1]) => translate(settings?.language, key);
  const tf = (key: Parameters<typeof formatTranslation>[1], values: Record<string, string | number>) => formatTranslation(settings?.language, key, values);
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
      <section className="card"><h3>{t("languageSection")}</h3>
        <label>{t("interfaceLanguage")}<LanguageSwitcher /></label>
        <label>{t("guestLanguage")}<select value={draft.guestLanguage ?? "en"} onChange={(e) => set("guestLanguage", e.target.value as "en" | "vi")}>
          <option value="en">English — EN</option><option value="vi">Tiếng Việt — VI</option>
        </select></label>
        <p className="muted small">{t("translationPreview")}</p>
      </section>
      <section className="card"><h3>{t("photoSettings")}</h3>
        <div className="form">
          <label>{t("countdownSeconds")}<input type="number" min={1} max={10} value={draft.countdownSeconds} onChange={(e) => set("countdownSeconds", Number(e.target.value))} /></label>
          <label>{t("photosPerSession")}<input type="number" min={1} max={12} value={draft.shots} onChange={(e) => set("shots", Number(e.target.value))} /></label>
          <p className="muted small">{t("photosPerSessionHint")}</p>
          <label>{t("outputResolution")}
            <select aria-label={t("outputResolution")} value={draft.outputDpi || 300} onChange={(e) => set("outputDpi", Number(e.target.value))}>
              <option value={300}>{t("printDpiStandard")}</option>
              <option value={600}>{t("printDpiHigh")}</option>
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
          {location.source === "documents" && <Badge kind="ok">{t("documentsFolder")}</Badge>}
          {location.source === "custom" && <Badge kind="info">{t("customFolder")}</Badge>}
          {location.source === "legacy" && <Badge kind="warn">{t("legacyFolder")}</Badge>}
          {location.source === "fallback" && <Badge kind="err">{t("fallbackFolder")}</Badge>}
          {" "}{bytes(location.freeBytes, settings.language)} {t("freeBytes")}. {t("mediaContentsAndConfig")} <code>{location.configPath}</code>.
        </p>
        {location.warning && <div className="note warn"><div><LocalizedDiagnostic text={location.warning} summary="storageWarning" /></div></div>}
        <div className="row">
          <button onClick={() => other.run(() => api.openLibraryFolder())}>{t("openLibrary")}</button>
          {location.migrationPending && <button className="primary" onClick={() => setMigrate(true)}>{t("moveDocuments")}</button>}
        </div>
      </section>

      <section className="card"><h3>{translate(settings?.language, "preferredCameras")}</h3>
        <p className="muted small">{t("preferencesHint")}</p>
        <table className="table"><tbody>
          <tr><td>{t("photoCamera")}</td><td>{settings.photoCamera.name || <span className="muted">{t("notSet")}</span>}</td>
            <td>{settings.photoCamera.name && <button className="ghost" onClick={() => other.run(async () => { await api.clearPreferredCamera("photo"); toast(t("photoCameraCleared")); })}>{t("clear")}</button>}</td></tr>
          <tr><td>{t("videoCamera")}</td><td>{settings.videoCamera.name || <span className="muted">{t("notSet")}</span>}</td>
            <td>{settings.videoCamera.name && <button className="ghost" onClick={() => other.run(async () => { await api.clearPreferredCamera("video"); toast(t("videoCameraCleared")); })}>{t("clear")}</button>}</td></tr>
        </tbody></table>
      </section>

      <section className="card"><h3>{translate(settings?.language, "printingDefaults")}</h3>
        <table className="table"><tbody>
          <tr><td>{t("defaultPrinter")}</td><td>{settings.printerName || <span className="muted">{t("windowsDefaultPrinter")}</span>}</td></tr>
          <tr><td>{t("defaultPaper")}</td><td>{settings.printPaper || <span className="muted">{t("notSet")}</span>}</td></tr>
        </tbody></table>
        <p className="muted small">{t("printerPageHint")}</p>
      </section>

      <section className="card"><h3>{t("kioskPin")}</h3>
        <p className="muted small">{t("kioskPinHint")}</p>
        <ErrorNote error={pin.error} onClose={pin.clear} />
        <div className="form">
          {settings.kioskPinSet && <label>{t("currentPin")}<input type="password" inputMode="numeric" autoComplete="off" maxLength={8} value={pinCur} onChange={(e) => setPinCur(e.target.value.replace(/\D/g, ""))} /></label>}
          <label>{settings.kioskPinSet ? t("newPin") : t("pinLabel")}<input type="password" inputMode="numeric" autoComplete="off" maxLength={8} value={pinNew} onChange={(e) => setPinNew(e.target.value.replace(/\D/g, ""))} /></label>
        </div>
        <div className="row">
          <button className="primary" disabled={pin.busy || pinNew.length < 4} onClick={() => pin.run(async () => { await api.setKioskPin(pinCur, pinNew); setPinCur(""); setPinNew(""); toast(settings.kioskPinSet ? t("pinChanged") : t("pinSet")); })}>{settings.kioskPinSet ? t("changePin") : t("setPin")}</button>
          {settings.kioskPinSet && <button className="danger" disabled={pin.busy || pinCur.length < 4} onClick={() => pin.run(async () => { await api.setKioskPin(pinCur, ""); setPinCur(""); toast(t("pinRemoved")); })}>{t("removePin")}</button>}
        </div>
      </section>

      <section className="card"><h3>{translate(settings?.language, "storage")}</h3>
        <div className="form">
          <label>{t("customMediaFolder")}
            <div className="row">
              <input value={draft.mediaDir} placeholder={t("defaultAppDataFolder")} onChange={(e) => set("mediaDir", e.target.value)} />
              <button onClick={() => other.run(async () => { const p = await api.pickFolder(); if (p) set("mediaDir", p); })}>{t("browse")}</button>
            </div>
          </label>
        </div>
        <p className="muted small">{t("folderRestart")}</p>
        <div className="row">
          <button onClick={() => other.run(async () => { setReport(await api.verifyLibrary(true)); bumpLibrary(); })}>{t("verifyLibrary")}</button>
          <button className="danger" onClick={() => setConfirmTrash(true)}>{t("emptyTrash")}</button>
        </div>
        {report && <p className="muted small">{tf("verificationReport", { checked: report.checked, missing: report.missing.length, corrupt: report.corrupt.length, recovered: report.recovered.length, interrupted: report.interrupted.length })}</p>}
      </section>

      <section className="card"><h3>{translate(settings?.language, "externalComponents")}</h3>
        <div className="form">
          <label>{t("ffmpegPath")}<input value={draft.ffmpegPath} onChange={(e) => set("ffmpegPath", e.target.value)} placeholder="C:\ffmpeg\bin\ffmpeg.exe" /></label>
          <label>{t("canonSdkFolder")}<input value={draft.canonSdkDir} onChange={(e) => set("canonSdkDir", e.target.value)} placeholder="C:\Canon\EDSDK_64\Dll" /></label>
        </div>
        <p className="muted small">{t("canonSdkNotBundled")}</p>
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
          toast(r.restartRequired ? t("saveRestartFolder") : t("settingsSaved"));
        })}>{translate(settings?.language, "saveSettings")}</button>
      </div>
      {migrate && <MigrationDialog onClose={() => setMigrate(false)} />}
      {confirmTrash && <Confirm title={t("emptyTrashConfirmTitle")} danger confirmLabel={t("permanentlyDelete")} onNo={() => setConfirmTrash(false)}
        body={<p>{t("emptyTrashConfirmBody")}</p>}
        onYes={() => other.run(async () => { const n = await api.emptyTrash(); setConfirmTrash(false); toast(tf("permanentlyDeleted", { count: n })); })} />}
    </div>
  );
}
