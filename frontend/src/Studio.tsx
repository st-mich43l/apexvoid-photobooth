import { translate, type TranslationKey } from "./i18n";
import { useCallback, useState } from "react";
import { useApp } from "./state";
import { Badge } from "./ui";
import { MigrationDialog } from "./MigrationDialog";
import { CameraPage } from "./pages/CameraPage";
import { Dashboard } from "./pages/Dashboard";
import { DiagnosticsPage } from "./pages/DiagnosticsPage";
import { Gallery } from "./pages/Gallery";
import { PhotoPage } from "./pages/PhotoPage";
import { SettingsPage } from "./pages/Settings";
import { Frames } from "./pages/Frames";
import { Icon } from "./icons";
import { PrinterPage } from "./pages/PrinterPage";
import { VideoPage } from "./pages/VideoPage";
import { PreparePage } from "./operator/PreparePage";
import { useOpenBooth } from "./operator/useOpenBooth";

/** Daily operation first; the rest under Tools. Every page that existed before is still here. */
export const NAV_MAIN: Array<[string, string]> = [["dashboard", "Dashboard"], ["prepare", "Prepare Booth"], ["camera", "Camera"], ["printer", "Printer"], ["frames", "Frames"], ["gallery", "Gallery"]];
export const NAV_TOOLS: Array<[string, string]> = [["photo", "Photo"], ["video", "Video"], ["settings", "Settings"], ["diagnostics", "Diagnostics"]];
const NAV = [...NAV_MAIN, ...NAV_TOOLS];

/** Studio Mode — the operator interface. */
export function Studio({ onBooth }: { onBooth: () => void }) {
  const { camera, boot, location, settings } = useApp();
  const [migrateOpen, setMigrateOpen] = useState(true);
  // #camera, #gallery … deep-link straight to a page (handy for support and screenshots)
  const [page, setPage] = useState(() => (NAV.some(([id]) => id === window.location.hash.slice(1).split("/")[0]) ? window.location.hash.slice(1).split("/")[0] : "dashboard"));
  const ok = camera.device && camera.state !== "error" && !camera.reconnecting;
  const go = useCallback((id: string) => { setPage(id); try { window.history.replaceState(null, "", `#${id}`); } catch { /* not available in some web views */ } }, []);
  // every way into Booth Mode checks readiness first (one launch at a time, failing items listed, no camera is switched)
  const booth = useOpenBooth(onBooth, go);
  const navButton = ([id, label]: [string, string]) => (
    <button key={id} className={page === id ? "active" : ""} onClick={() => go(id)} aria-current={page === id ? "page" : undefined}>
      <Icon name={id} /><span className="nav-label">{translate(settings?.language, id as TranslationKey) || label}</span>
    </button>
  );

  return (
    <div className="studio">
      <nav aria-label="Studio navigation">
        <div className="brand"><span className="logo"><Icon name="camera" size={22} /></span><div><b>ApexVoid</b><small>Photobooth Studio</small></div></div>
        <div className="nav-group" role="group" aria-label="Daily">{NAV_MAIN.map(navButton)}</div>
        <div className="nav-group tools" role="group" aria-label="Tools"><small className="nav-head">Tools</small>{NAV_TOOLS.map(navButton)}</div>
        <div className="spacer" />
        <button className="booth-btn" onClick={booth.open} disabled={booth.busy} aria-label="Open Booth Mode" title="Open Booth Mode"><Icon name="play" size={16} /><span className="nav-label">Open Booth Mode</span></button>
        <small className="ver">v{boot.version}</small>
      </nav>
      <main>
        <header className="topbar">
          <span>{ok ? <Badge kind={camera.device!.simulated ? "sim" : "ok"}>{camera.device!.simulated ? "DEMO · SIMULATED camera" : camera.device!.name}</Badge>
            : camera.reconnecting ? <Badge kind="warn">Camera reconnecting…</Badge> : <Badge kind="err">No camera</Badge>}</span>
        </header>
        {location.warning && <div className="note warn banner" role="alert"><div>{location.warning}</div></div>}
        {location.restartRequired && <div className="note warn banner" role="alert"><div><strong>Restart required.</strong> The library was copied to {location.defaultPath}. Close and reopen the app; capturing is paused until then.</div></div>}
        {location.migrationPending && !migrateOpen && <div className="note warn banner"><div>Your library is still in the old folder.</div><button onClick={() => setMigrateOpen(true)}>Move to Documents…</button></div>}
        {page === "dashboard" && <Dashboard go={go} onBooth={booth.open} />}
        {page === "prepare" && <PreparePage go={go} onOpenBooth={booth.open} opening={booth.busy} />}
        {page === "camera" && <CameraPage />}
        {page === "photo" && <PhotoPage />}
        {page === "video" && <VideoPage />}
        {page === "gallery" && <Gallery />}
        {page === "frames" && <Frames />}
        {page === "printer" && <PrinterPage />}
        {page === "settings" && <SettingsPage />}
        {page === "diagnostics" && <DiagnosticsPage />}
      </main>
    {booth.dialog}
    {location.migrationPending && migrateOpen && <MigrationDialog onClose={() => setMigrateOpen(false)} />}
    </div>
  );
}
