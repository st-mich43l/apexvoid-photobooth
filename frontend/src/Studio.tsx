import { useCallback, useEffect, useState } from "react";
import { Window } from "@wailsio/runtime";
import { enterFullscreen, leaveFullscreen } from "./fullscreen";
import { useApp, useT } from "./state";
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
  const { camera, boot, location } = useApp();
  const { t } = useT();
  const [migrateOpen, setMigrateOpen] = useState(true);
  const [fullscreen, setFullscreen] = useState(false);
  const [fullscreenBusy, setFullscreenBusy] = useState(false);
  const [fullscreenError, setFullscreenError] = useState(false);
  useEffect(() => {
    let mounted = true;
    Window.IsFullscreen().then(value => { if (mounted) setFullscreen(value); }).catch(() => {});
    return () => { mounted = false; };
  }, []);
  const toggleFullscreen = async () => {
    if (fullscreenBusy) return;
    setFullscreenBusy(true);
    setFullscreenError(false);
    try {
      const current = await Window.IsFullscreen();
      if (current) await leaveFullscreen(); else await enterFullscreen();
      setFullscreen(!current);
    } catch {
      setFullscreenError(true);
    } finally {
      setFullscreenBusy(false);
    }
  };
  // #camera, #gallery … deep-link straight to a page (handy for support and screenshots)
  const [page, setPage] = useState(() => (NAV.some(([id]) => id === window.location.hash.slice(1).split("/")[0]) ? window.location.hash.slice(1).split("/")[0] : "dashboard"));
  const ok = camera.device && camera.state !== "error" && !camera.reconnecting;
  const go = useCallback((id: string) => { setPage(id); try { window.history.replaceState(null, "", `#${id}`); } catch { /* not available in some web views */ } }, []);
  // every way into Booth Mode checks readiness first (one launch at a time, failing items listed, no camera is switched)
  const booth = useOpenBooth(onBooth, go);
  const navButton = ([id, label]: [string, string]) => (
    <button key={id} className={page === id ? "active" : ""} onClick={() => go(id)} aria-current={page === id ? "page" : undefined}>
      <Icon name={id} /><span className="nav-label">{t(id as Parameters<typeof t>[0]) || label}</span>
    </button>
  );

  return (
    <div className="studio studio-shell">
      <main>
        <header className="topbar">
          <div className="brand"><span className="logo"><Icon name="camera" size={22} /></span><div><b>ApexVoid</b><small>Photobooth Studio</small></div></div>
          <div className="topbar-actions">
            <span className="app-version">{boot.version && (/^\d/.test(boot.version) ? `v${boot.version}` : boot.version)}</span>
            <span className="camera-status">{ok ? <Badge kind={camera.device!.simulated ? "sim" : "ok"}>{camera.device!.simulated ? t("demoCameraBadge") : camera.device!.name}</Badge>
              : camera.reconnecting ? <Badge kind="warn">{t("reconnectingBadge")}</Badge> : <Badge kind="err">{t("noCameraBadge")}</Badge>}</span>
            <button className="fullscreen-btn" onClick={toggleFullscreen} disabled={fullscreenBusy} aria-label={t(fullscreen ? "exitFullscreen" : "enterFullscreen")} title={t(fullscreen ? "exitFullscreen" : "enterFullscreen")} aria-pressed={fullscreen}><Icon name={fullscreen ? "fullscreenExit" : "fullscreen"} size={19} /></button>
            <button className="booth-action" onClick={booth.open} disabled={booth.busy} aria-label={t("boothOpenLabel")}><Icon name="play" size={16} />{t("boothOpenLabel")}</button>
          </div>
        </header>
        {fullscreenError && <div className="note warn banner" role="alert">{t("fullscreenFailed")}</div>}
        {location.warning && <div className="note warn banner" role="alert"><div>{location.warning}</div></div>}
        {location.restartRequired && <div className="note warn banner" role="alert"><div><strong>{t("restartRequiredBanner")}</strong> {t("libraryCopiedTo")} {location.defaultPath}. {t("closeReopenCapturePaused")}</div></div>}
        {location.migrationPending && !migrateOpen && <div className="note warn banner"><div>{t("libraryStillOld")}</div><button onClick={() => setMigrateOpen(true)}>{t("moveToDocuments")}</button></div>}
        {page === "dashboard" && <Dashboard go={go} />}
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
      <nav className="bottom-nav" aria-label={t("studioNav")}>
        <div className="nav-group" role="group" aria-label={t("dailyNav")}>{NAV_MAIN.map(navButton)}</div>
        <div className="nav-group tools" role="group" aria-label={t("toolsNav")}>{NAV_TOOLS.map(navButton)}</div>
      </nav>
    {booth.dialog}
    {location.migrationPending && migrateOpen && <MigrationDialog onClose={() => setMigrateOpen(false)} />}
    </div>
  );
}
