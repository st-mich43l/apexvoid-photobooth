# ApexVoid Photobooth — market review and architecture decisions

Status: Phase 1 (2026-11). This replaces the previous direction (an ERP/CRM/booking
web service). The product is camera-operated desktop software.

## 1. Competitor review (what the vendors' own pages say)

Sources: the vendors' public pages as read on 2026-11-21 (home/feature pages only, not
manuals). Anything a page did not state is marked *not stated* — it does **not** mean the
product lacks it. No vendor code, assets or SDK binaries were used.

| | LumaBooth for Windows (dslrBooth) | Darkroom Booth | Breeze Booth |
|---|---|---|---|
| Platforms | Windows (separate Apple product) | Windows | Booth app is Apple (iPad/iPhone/Mac); Windows only for the event editor/hub (Windows booth = a separate product) |
| Cameras | "Canon, Nikon, Sony, or webcam", GoPro | tethered "digital cameras, webcams and GoPro" (wired or wireless) | Canon DSLR & mirrorless, built-in iOS cameras; "Live View Support" |
| Live view | *not stated* | *not stated* | listed |
| Capture modes | photos, video, GIF, slow-mo, 360, boomerang | photos, video (incl. slow-mo), GIF/boomerang, 3-shot + pick best, retakes | photos, GIF (slideshow/burst/boomerang), video with auto-editing |
| Strips / layouts | built-in print layout designer | 75+ templates for print and screen | drag-and-drop layout editor, custom layouts |
| Filters / effects | effects and filters, green screen | green screen with overlays/video | colour/B&W filters, custom LUTs, mirror effects, local + cloud AI background removal |
| Printing | print from the app | prints, re-print options | any Windows-compatible printer; auto-print, watch-folder print, duplex, multiple printers, pay-per-print |
| Touch / kiosk | *not stated* | touchscreen, keyboard, USB button, voice; coin-op / bill acceptors | touch screens, button control, touch-screen designer, auto-start, password-protected exit |
| Offline | *not stated*; promotes cloud galleries | cloud upload features; not stated as offline-only | "can be configured to run 100% offline" |
| Pricing (as stated) | free trial; paid terms not on page | one-time licence + yearly maintenance | subscription per device |

Take-aways that shaped Phase 1:

* The table stakes are: reliable tethered capture with live view, a countdown flow with retakes,
  layout/strip composition at exact print size, a guest-facing touch UI, and a separate operator UI.
* Offline operation is a selling point (Breeze states it explicitly) — we run with **no server,
  no cloud, no network**; the only socket is a token-protected loopback server inside the app.
* A password-protected exit is standard in kiosk software. Phase 1 has a hidden 3-second
  corner hold; an operator PIN is a later milestone (as scoped).
* GIF/boomerang, filters/effects, green screen, cloud sharing and print queues are common but
  **not** in Phase 1 (see the roadmap below).

## 2. Canon EDSDK (what is and is not verified)

Canon's SDK page (usa.canon.com/support/sdk) returned HTTP 403 to automated fetching, so
**its licence and redistribution terms were not read**. Treat them as unverified; the project
therefore never commits or ships SDK files (`.gitignore` blocks them; the user copies the DLLs from
their own Canon download — see `docs/CAMERAS.md`).

Evidence gathered from other public pages (secondary, not authoritative):

* Canon firmware notes for the EOS R50 (v1.1.0 / v1.3.0 entries) mention support for the
  "Software Development Assistance Kit (EDSDK/CCAPI)". Which EDSDK *release* first lists the R50 was
  **not confirmed**; the EDSDK release note entry seen (13.18.10) names the R7 and R10. → Hardware
  test step 1 checks the installed SDK's own release notes.
* Canon support/community replies say the R50 can be used as a USB webcam in its "streaming"
  (UVC/UAC) mode without Canon's webcam utility. Not verified by us on hardware.
* EDSDK exposes movie-related properties for some bodies. Whether **movie start/stop** and **movie file
  transfer** work on the R50 with the installed SDK is **unknown**: no hardware or SDK was available.

Decision: the Canon adapter reports `movieRecord` and `movieTransfer` as **unverified** and does not
implement them. Video is implemented through the independent UVC path (the R50 in streaming mode is
recorded on the PC by FFmpeg). "Camera-native movie recording" and "frames streamed to the PC" are different
things and are reported separately in the capability model (`movieRecord` vs `streamedVideo`).

## 3. Technology decisions

| Choice | Why | Compatibility / distribution |
|---|---|---|
| **Wails 3.1.0** (stable, tagged 2026-11-01) + Go 1.26 | Go backend, web UI, small native exe | Windows needs the Microsoft Edge **WebView2** runtime (ships with Windows 11 and current Windows 10 updates; Evergreen installer otherwise). Pure Go on Windows (no CGO, cross-compiles). macOS/Linux builds need WebKit (future targets). |
| React 19 + TypeScript 5.9 + Vite 8 | as requested | built into `cmd/photobooth/frontend_dist` and embedded in the exe |
| **modernc.org/sqlite** | SQLite without CGO → trivial Windows cross-build, no DLL | pure Go, BSD-licensed |
| Canon **EDSDK.dll** via `LoadLibraryEx` + syscalls (no cgo, no headers) | official SDK, no compiler toolchain for Canon headers needed | 64-bit DLL required (checked by PE header); proprietary, **not bundled** |
| **FFmpeg** as a subprocess for UVC/webcam + video | DirectShow capture, H.264/AAC encode, MP4 in one maintained tool. Evaluated: DirectShow (chosen via FFmpeg's `dshow`), Media Foundation (used only as an *encoder* `h264_mf` for LGPL builds; direct MF capture would need a CGO/COM binding we cannot test here) | an `ffmpeg.exe` is located next to the app, in `ffmpeg\`, via Settings, or on PATH. Running it as a separate process keeps GPL/LGPL obligations out of our binary. `libx264` needs a GPL build; LGPL builds use `h264_mf` / hardware encoders. |
| OS print system | OS-native printing | discovery implemented (PowerShell / CUPS); submission only CUPS; **Windows submission not implemented** |
| Loopback HTTP server (127.0.0.1, random port, per-run token) | WebView2's asset interception cannot stream an endless MJPEG response nor serve large videos with Range efficiently | binds loopback only, token required, files addressed by DB id never by path; no external network |

Why not a different stack: nothing in camera/video capture forced us off Wails/Go/React.

## 4. Module map

```
cmd/photobooth        Wails shell (thin) + --selftest
internal/app          application service bound to the UI (no Wails import → headless-testable)
internal/camera       Camera/Driver interfaces, capability model, Manager (reconnect, single capture lock)
  camera/edsdk        Canon EDSDK adapter (api boundary + Windows DLL binding + SDK thread)
  camera/uvc          UVC/webcam adapter (FFmpeg)
  camera/mock         simulated camera (flagged SIMULATED everywhere)
  camera/cameratest   adapter contract test every driver must pass
internal/capture      photo-shooting state machine
internal/video        FFmpeg discovery/preview/recording pipeline, probe, thumbnails
internal/sessions     session lifecycle + media repository (originals never overwritten)
internal/storage      layout, SQLite, atomic writes
internal/media        EXIF orientation, thumbnails, non-destructive edits
internal/templates    frames: layout geometry + style, the single renderer, snapshots (docs/FRAMES.md)
internal/printing     printer discovery, paper validation, job tracking, bounded reprint
internal/settings     JSON settings
frontend/             React UI: Studio (operator) and Booth (guest)
```

Windows-specific code is confined to `camera/edsdk/dll_windows.go`, `video/proc_windows.go`,
`printing/proc_windows.go`, `app/diskfree_windows.go`; everything else is portable and tested on Linux.

## 5. Key design points

* **Camera abstraction & capability model** — `Support` is `supported | unsupported | unverified`. Canon
  capabilities start `unverified` and are promoted only by a real success on the connected camera.
* **EDSDK threading** — one goroutine locked to one OS thread initialises the SDK (COM STA), runs every
  call as a job, and pumps `EdsGetEvent` + the Windows message queue so callbacks fire. UI calls never run SDK
  code directly, so the UI cannot freeze. A fake SDK implementing the same `api` interface tests the
  full adapter logic (and asserts the single-thread rule).
* **State machine** — `IDLE → PREVIEW → COUNTDOWN → CAPTURING → (COUNTDOWN…) → REVIEW → SAVING → COMPLETED`,
  plus `ERROR`/`CANCELLED`. Invalid requests return `ErrInvalidTransition` and change nothing. A failed retake
  keeps the previous photo (warning, back to REVIEW). One automatic retry for transient capture errors.
* **Data safety** — each original is written (temp + fsync + rename, never overwriting) *before* its DB row;
  startup `Verify` marks crashed sessions `interrupted`, flags missing files and re-registers orphan originals
  and orphan MP4s. Deletes go to a recoverable trash; edits are recipes + separate renders.
* **Video** — one FFmpeg process owns the device (preview JPEGs on stdout + optional MP4). Starting/stopping a
  recording restarts it (≈1 s preview gap). MP4 is *fragmented* with a 1 s GOP so a crash loses ≤ ~1–2 s
  (verified by killing FFmpeg in a test).
* **Simulated camera** — hidden unless enabled in Settings; ribbon on every screen; photos carry
  `simulated=1` in the database and are labelled in the gallery.

## 6. Out of Phase 1 (roadmap)

Operator PIN, GIF/boomerang, filters/effects/green screen, template designer UI, Windows print submission,
print queue UX, cloud/QR sharing (optional, off by default), macOS/Linux packaging, Canon movie probing on hardware,
auto-update, code signing.
