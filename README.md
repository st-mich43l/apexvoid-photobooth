# ApexVoid Photobooth

Camera-operated photobooth desktop software for Windows (macOS/Linux later): connects to real
cameras, shows live view, shoots photos with a countdown, records video, manages sessions and a local media
library, composes photo strips at exact print size, and (later) prints.

It is **standalone**: no server, Docker, Enterprise/CRM/ERP integration, account system or external database.
Data lives in a local SQLite file and ordinary folders; the app makes no network connections.

**Stack:** Wails 3 + Go · React + TypeScript + Vite · SQLite (pure Go) · FFmpeg (external) · Canon EDSDK (external, not bundled).

## What works, and how it was verified

| Area | Status | Verified by |
|---|---|---|
| Camera abstraction, capability model, reconnect, single-capture lock | working | CI unit + contract tests |
| Simulated (mock) camera | dev tool, always labelled SIMULATED | CI |
| **Canon EOS R50 via EDSDK** (discover, live view, shutter, JPEG original download, USB-drop recovery, clean release) | **implemented, NOT hardware-tested** | adapter logic tested against a fake SDK in CI; the real DLL binding is only compiled (cross-vet), never executed |
| **UVC / webcam via FFmpeg** (preview, stills, MP4 recording, mic selection) | pipeline tested with a synthetic FFmpeg source; **real DirectShow device untested** | CI |
| Canon movie start/stop + movie file transfer via EDSDK | **not implemented, unverified** (see docs/CAMERAS.md) | — |
| Photo state machine (IDLE→PREVIEW→COUNTDOWN→CAPTURING→REVIEW→SAVING→COMPLETED, ERROR, CANCELLED) | working | CI incl. race detector, cancel, retake, disconnect, repeat sessions |
| Media library: sessions, originals never overwritten, thumbnails, gallery, edit (rotate/crop/brightness/contrast/saturation), export, trash/restore, crash recovery | working | CI |
| Layouts (2×6 strips, 4×6 sheets, single, custom JSON/overlay) at exact size and DPI | working | CI |
| Printing (Windows spooler + GDI): enumeration, capabilities, real page geometry, preview, submission, spooler job tracking, history, reprint | **implemented**; physical paper output **NOT verified** on any printer (see docs/PRINTING.md for what CI proves) | unit tests, Windows struct tests, CI file-port printer step |
| **Frame builder** (custom frames, drag/resize slots, transparent PNG overlay import, atomic storage) **300 / 600 dpi output** and **two-up 4×6 printing of 2×6 strips**, Booth *Print* | working; unit/UI/golden-image tests; the Windows CI job runs the whole frame flow | CI; **not run in WebView2, with a real camera or on paper** — [docs/FRAMES.md](docs/FRAMES.md), checklist F1–F9 in [docs/HARDWARE_TESTS.md](docs/HARDWARE_TESTS.md) |
| **Frame selection before capture**: 17 original designs × 9 layouts, colour options, optional caption, frame decides the photo count, per-session immutable snapshot, change design after capture without retaking | working with the simulated camera; unit/UI tests | CI; **not yet run inside WebView2 or with a real camera** — see [docs/FRAMES.md](docs/FRAMES.md) |
| Studio (operator) and Booth (guest) UI | working in simulated mode; screenshots checked in a browser harness | manual; **not yet run inside WebView2 on Windows** |
| Windows x64 single-file exe (FFmpeg built in) | built by CI as the only artifact; self-tests run on the Windows runner (start-up, built-in FFmpeg unpacks and finds an H.264 encoder, camera probe, printing to a file-port printer) | CI |

Hardware acceptance steps: [docs/HARDWARE_TESTS.md](docs/HARDWARE_TESTS.md). Camera setup and the EOS R50 video findings:
[docs/CAMERAS.md](docs/CAMERAS.md). Market review and decisions: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).
Install/build/test: [docs/INSTALL.md](docs/INSTALL.md).

## Quick start (no camera)
Run the app, Settings → enable **Simulated camera**, Camera → Connect, Photo → Start.

## Removed from the previous application
Enterprise integration, CRM/ERP SDK, roles/workspaces, guest care, booking, café POS/catalog, PostgreSQL,
Docker runtime, remote registration and the legacy web UI. Git history is preserved.
