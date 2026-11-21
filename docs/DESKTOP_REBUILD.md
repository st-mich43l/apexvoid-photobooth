# Desktop Rebuild: Architecture Decision and Migration Gates

## Product boundary

ApexVoid Photobooth is a local workstation application. It must start, show its UI and support a demo session without network access, Docker, PostgreSQL, a browser, or ApexVoid Enterprise. Windows x64 is the first target; macOS and Linux must remain feasible without rewriting the core workflow.

**Explicitly excluded from the new application:** Enterprise app registration, signed manifest/enrollment, gateway assertions, platform RBAC, workspaces, ERP catalog, café POS, staff booking calendar and customer-service desk. The old repository retains history; we do not migrate this business domain into the desktop runtime.

## Decision (2026-11-21)

Use Wails v3.0.0 with Go 1.26+ and React/TypeScript/Vite. Wails supplies native windows and bundles the frontend inside the application executable. It uses Microsoft WebView2 on Windows, WebKit on macOS/Linux. A portable EXE still depends on the target OS's WebView2 Runtime. Prefer native Windows builds for the first hardware validation.

Alternatives considered:
- Electron: broad ecosystem and bundled Chromium, but larger distribution and additional native-camera integration concerns.
- Tauri v2: efficient, but introduces Rust alongside the project's Go expertise.

## Architecture target

```text
React kiosk UI / operator settings
      |
Wails typed service bridge
      |
Go application layer (state machine / sessions / events)
      |------------------------|----------------------|
CameraAdapter             PhotoPipeline         PrinterAdapter
  - mock (Phase 0)         - originals           - mock (Phase 0)
  - Canon EDSDK/Windows    - strip layouts       - Windows spooler
  - gPhoto2/Linux          - templates           - macOS/Linux
      |
local SQLite + per-session directories
```

Camera preview must not be confused with high-resolution shutter capture. Canon EOS R50 on USB should be validated first against the Canon EDSDK on Windows. Webcam/UVC and mock adapters are separate implementations. Do not bundle proprietary Canon binaries before reviewing the SDK license and distribution rights. Never fake a hardware-connected status.

## Implementation gates

**P0 — Windows portable smoke test**
- Generate a Wails desktop React project, replace the stock UI with a photobooth demo and remove any runtime dependence on Enterprise.
- Build Windows x64 EXE via local script and Windows GitHub Actions, publish workflow artifact.
- Test launch, kiosk-size UI, countdown, four simulated frames and reset.
- No camera, persistence or printing claims.

**P1 — Native camera spike**
- Obtain Canon EDSDK through the official developer process.
- Verify EOS R50 enumeration, USB ownership conflict (EOS Utility), live view, shutter trigger, full-resolution transfer and recoverable disconnect.
- Add a mock adapter and hardware integration tests; capture failures must not silently become demo captures.

**P2 — Offline photo workflow**
- Immutable session IDs; original file retention; local SQLite metadata; crash-safe filesystem writes.
- Four-shot sequence, retake policy, template composition, image previews and export.
- User data stored under an OS-appropriate app data directory, not the EXE folder.

**P3 — Printing and kiosk operations**
- Windows print queue abstraction, printer selection, media sizes, retries without accidental duplicate prints.
- Auto-start, fullscreen kiosk, safe operator settings, hardware diagnostics.
- Windows installer/signing after the portable workflow is stable.

**P4 — macOS and Linux**
- Separate camera/printer adapters per platform, CI builds, permission checks and on-device hardware validation.

## Acceptance and non-goals for P0

The portable EXE is a first boot-and-flow experiment. A demo capture means UI simulation only. GitHub Actions success only establishes a successful build, **not** successful camera capture or human validation on a Windows device. No ERP/CRM data migration or destructive deletion in P0.
