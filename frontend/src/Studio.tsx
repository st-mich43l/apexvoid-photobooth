import { useState } from "react";
import { useApp } from "./state";
import { Badge } from "./ui";
import { CameraPage } from "./pages/CameraPage";
import { Dashboard } from "./pages/Dashboard";
import { DiagnosticsPage } from "./pages/DiagnosticsPage";
import { Gallery } from "./pages/Gallery";
import { PhotoPage } from "./pages/PhotoPage";
import { SettingsPage } from "./pages/Settings";
import { Templates } from "./pages/Templates";
import { VideoPage } from "./pages/VideoPage";

const NAV: Array<[string, string, string]> = [
  ["dashboard", "Dashboard", "◧"], ["camera", "Camera", "◉"], ["photo", "Photo", "▣"], ["video", "Video", "▶"],
  ["gallery", "Gallery", "▦"], ["templates", "Layouts", "▤"], ["settings", "Settings", "⚙"], ["diagnostics", "Diagnostics", "✚"],
];

/** Studio Mode — the operator interface. */
export function Studio({ onBooth }: { onBooth: () => void }) {
  const { camera, boot } = useApp();
  // #camera, #gallery … deep-link straight to a page (handy for support and screenshots)
  const [page, setPage] = useState(() => (NAV.some(([id]) => id === location.hash.slice(1)) ? location.hash.slice(1) : "dashboard"));
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
        {page === "dashboard" && <Dashboard go={setPage} />}
        {page === "camera" && <CameraPage />}
        {page === "photo" && <PhotoPage />}
        {page === "video" && <VideoPage />}
        {page === "gallery" && <Gallery />}
        {page === "templates" && <Templates />}
        {page === "settings" && <SettingsPage />}
        {page === "diagnostics" && <DiagnosticsPage />}
      </main>
    </div>
  );
}
