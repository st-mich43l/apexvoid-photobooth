import { useState } from "react";
import { useApp } from "./state";
import { Badge } from "./ui";
import { MigrationDialog } from "./MigrationDialog";
import { CameraPage } from "./pages/CameraPage";
import { Dashboard } from "./pages/Dashboard";
import { DiagnosticsPage } from "./pages/DiagnosticsPage";
import { Gallery } from "./pages/Gallery";
import { PhotoPage } from "./pages/PhotoPage";
import { SettingsPage } from "./pages/Settings";
import { Templates } from "./pages/Templates";
import { PrinterPage } from "./pages/PrinterPage";
import { VideoPage } from "./pages/VideoPage";

const NAV: Array<[string, string, string]> = [
  ["dashboard", "Dashboard", "◧"], ["camera", "Camera", "◉"], ["photo", "Photo", "▣"], ["video", "Video", "▶"],
  ["gallery", "Gallery", "▦"], ["templates", "Layouts", "▤"], ["printer", "Printer", "⎙"], ["settings", "Settings", "⚙"], ["diagnostics", "Diagnostics", "✚"],
];

/** Studio Mode — the operator interface. */
export function Studio({ onBooth }: { onBooth: () => void }) {
  const { camera, boot, location } = useApp();
  const [migrateOpen, setMigrateOpen] = useState(true);
  // #camera, #gallery … deep-link straight to a page (handy for support and screenshots)
  const [page, setPage] = useState(() => (NAV.some(([id]) => id === window.location.hash.slice(1)) ? window.location.hash.slice(1) : "dashboard"));
  const ok = camera.device && camera.state !== "error" && !camera.reconnecting;

  return (
    <div className="studio">
      <nav aria-label="Studio navigation">
        <div className="brand"><span className="logo">◉</span><div><b>ApexVoid</b><small>Photobooth Studio</small></div></div>
        {NAV.map(([id, label, icon]) => (
          <button key={id} className={page === id ? "active" : ""} onClick={() => setPage(id)} aria-current={page === id ? "page" : undefined}>
            <span aria-hidden>{icon}</span>{label}
          </button>
        ))}
        <div className="spacer" />
        <button className="booth-btn" onClick={onBooth}>▶ Open Booth Mode</button>
        <small className="ver">v{boot.version}</small>
      </nav>
      <main>
        <header className="topbar">
          <span>{ok ? <Badge kind={camera.device!.simulated ? "sim" : "ok"}>{camera.device!.simulated ? "SIMULATED camera" : camera.device!.name}</Badge>
            : camera.reconnecting ? <Badge kind="warn">Camera reconnecting…</Badge> : <Badge kind="err">No camera</Badge>}</span>
        </header>
        {location.warning && <div className="note warn banner" role="alert"><div>{location.warning}</div></div>}
        {location.restartRequired && <div className="note warn banner" role="alert"><div><strong>Restart required.</strong> The library was copied to {location.defaultPath}. Close and reopen the app; capturing is paused until then.</div></div>}
        {location.migrationPending && !migrateOpen && <div className="note warn banner"><div>Your library is still in the old folder.</div><button onClick={() => setMigrateOpen(true)}>Move to Documents…</button></div>}
        {page === "dashboard" && <Dashboard go={setPage} />}
        {page === "camera" && <CameraPage />}
        {page === "photo" && <PhotoPage />}
        {page === "video" && <VideoPage />}
        {page === "gallery" && <Gallery />}
        {page === "templates" && <Templates />}
        {page === "printer" && <PrinterPage />}
        {page === "settings" && <SettingsPage />}
        {page === "diagnostics" && <DiagnosticsPage />}
      </main>
    {location.migrationPending && migrateOpen && <MigrationDialog onClose={() => setMigrateOpen(false)} />}
    </div>
  );
}
