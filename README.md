# ApexVoid Photobooth — Desktop

ApexVoid Photobooth is being rebuilt as an **offline-first, standalone desktop photobooth**. It is not an ApexVoid Enterprise / ERP / CRM application.

## Current milestone: Windows portable preview (Phase 0)

This branch contains a Windows-native bootstrapping proof of concept. The result is a Windows x64 `.exe` built with Wails 3, Go and React. The preview shows a touch-friendly photo-session flow using **simulated** shots.

**Not yet implemented:** Canon EOS R50 capture, real live view, photo file storage, image rendering, printing, payments, kiosk lockdown and hardware diagnostics. No camera integration is claimed by this preview.

### On a Windows 10/11 x64 development machine

Prerequisites: Go 1.26+, Node.js 22+, and Wails CLI v3.0.0.

```powershell
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0
$env:PATH += ";$env:USERPROFILE\go\bin"
wails3 doctor
.\scripts\bootstrap-desktop.ps1
```

The script generates the official Wails React starter under `desktop/apexvoidphotobooth/`, replaces it with the tracked ApexVoid standalone shell, and builds `desktop/apexvoidphotobooth/bin/*.exe`. It does **not** require Docker, PostgreSQL, Enterprise credentials, or an internet connection at runtime. The initial build requires internet access for dependencies. Windows WebView2 Runtime must be present (commonly preinstalled; check with `wails3 doctor`).

Run the resulting EXE directly. This is a **portable application**, not a fully self-contained OS runtime bundle. It can be copied to another Windows x64 machine with WebView2 installed.

### GitHub Actions

The `Windows portable preview` workflow builds the same EXE on `windows-latest` and uploads it as a downloadable workflow artifact. Check the job status before relying on the artifact.

### Architecture decision

- Desktop shell: Wails v3, Go, system webview
- UI: React + TypeScript + Vite
- Camera: adapter interface; Canon EDSDK driver first on Windows, platform adapters later
- Local persistence: SQLite + filesystem in a later phase
- Photo pipeline: full-resolution originals, layouts/templates, printer interface in later phases
- Offline-first: no Enterprise gateway or mandatory server
- Target platforms: Windows first, macOS and Linux next

### Old source code

The previous `cmd/`, `internal/`, `web/`, `db/`, `deploy/` and Docker files on this branch are **legacy Enterprise-era source**. They are deliberately not compiled or referenced by the desktop preview. They remain temporarily to preserve reviewability during the migration; they will be deleted from the new product once the standalone baseline is validated. Git history is preserved.

See [docs/DESKTOP_REBUILD.md](docs/DESKTOP_REBUILD.md) for the migration gates.
