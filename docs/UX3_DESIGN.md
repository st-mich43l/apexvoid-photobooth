# UX-3 — Operator setup and daily booth readiness

Scope: Studio (operator) UI plus small, additive backend changes. Guest UX-2 visuals and UX-1 session safety are untouched.
Before-state: [UX3_AUDIT.md](UX3_AUDIT.md).

## Daily workflow
Launch → **Prepare Booth** (camera, printer, frames, storage on one screen) → optional tests → **Open Booth Mode**.
Navigation: **Daily** = Dashboard · Prepare Booth · Camera · Printer · Frames · Gallery; **Tools** = Photo · Video · Settings · Diagnostics.
Every page that existed is still reachable. In portrait (and narrow windows) the nav is a labelled, scrollable top bar with Open Booth Mode first.

## One readiness engine
`BoothReadiness()` is still the only source of truth. It gained, additively:

| Field | Meaning |
|---|---|
| `items[].group` | camera · storage · frames · printer · session — the Prepare Booth section |
| `items[].verification` | `verified` (it worked on this machine this run) · `unverified` (present, unproven) · `demo` (simulated) |
| `demo` / status `demo` | a SIMULATED camera is the photo camera: sessions work, but this is not hardware verification |
| `printing` | `digital_only` · `unverified` (Windows says ready; paper never proven) · `unavailable` |

The UI maps lines to six states (`operator/status.ts`): **Ready · Setup required · Warning · Unavailable · Not verified · Demo / test mode**.
Required + problem = Setup required (blocks guests). Optional lines (printer, extra disk warning) never block. Canon SDK is not a readiness
line at all, so a webcam-only set-up is never blocked by it. Thresholds are the existing 500 MB block / 2 GB warn, now shared with diagnostics.

## Prepare Booth
* **Camera** — selected camera, connection, type/driver, preferred camera, preview, still-capture verification, reconnection; actions: Camera setup, Test camera.
* **Printer** — Digital only (a normal state, not an error) or printer + Windows status + paper + test print; wording never says paper was delivered.
* **Frames** — count, preselected frame with a preview from the Go renderer, Manage frames.
* **Storage** — folder, free space, writable, interrupted sessions, Library settings, Open media folder.
* **Quick settings** — countdown, mirror preview, output DPI, each a `PatchSettings` of exactly one key.
* **Open Booth Mode** — re-reads readiness, launches once however often it is clicked, otherwise lists the failing required items with a fix button.
  It never switches camera and never bypasses the per-session `startCheck` or `sessionGate`.

## Camera, Printer, Diagnostics
* **Camera** primary view: photo-camera list (Connect/Disconnect/Use as Photo camera), big live preview, optional **Camera test**. Under *Advanced*: Video camera
  (separate preference), stream resolution, drivers, Canon EDSDK diagnostics panel (unchanged), capabilities. Controls lock while a session is active (the backend refuses too).
* **Camera test** is explicit: it says the shutter will fire, then uses the normal preview → shoot → review → accept lifecycle with a real frame, labels the session
  **"Operator camera test"** (`LabelTestSession`, a note only) and keeps it. Failure shows the engine's reason and what to do; recovery goes through `sessionGate`.
  A simulated camera's result says "Demo test finished … does not verify real hardware". Webcam results mention frame grabs.
* **Printer**: *Digital only* vs *Print with a printer*; Windows status shown as "ready (not verified)"; test print needs a confirmation and is never automatic; history, reprint
  protection and placement/calibration (under *Advanced*) are unchanged. The Windows default printer is no longer shown as "in use" when none is chosen.
* **Diagnostics**: each check has `state` (ok/problem/warn/info), `blocksGuests`, `scope`. Canon SDK/driver and FFmpeg are *Not needed* unless a Canon / webcam camera is the photo
  camera (connected or preferred). **Copy support summary** returns status and counts only.

## Sanitized support summary
Contains: version, OS/arch, readiness lines, diagnostic lines, camera model/backend, counts, whether a printer / PIN exists.
Never contains: the PIN, secrets, photos or file names, folder paths (replaced by `<media folder>`, `<home>`, `<user profile>`…), printer name, computer or user names.
Covered by `TestDiagnosticSummaryContainsNothingSensitive`.

## Stability
Camera-list refreshes are latest-wins (`useLatest`); a refresh in progress is not started twice; readiness polling is unchanged (cached status, 8 s printer cache,
3 s write probe) and no new timers were added; camera switching stays behind `guardOperation()`; the single active-camera ownership model is untouched.

## Verification status
| Check | Status |
|---|---|
| Go tests (`./internal/app` incl. new UX-3 readiness/diagnostics/summary tests, `./internal/capture`) | pass |
| Go race tests `./internal/app/... ./internal/capture/... ./internal/camera/... ./internal/sessions/...` | see PR description |
| Frontend tests (297), `tsc --noEmit`, `vite build` | pass |
| `GOOS=windows go build ./cmd/photobooth` | compiles (cross-compile; not run) |
| Screenshots in `docs/img/ux3/` from the real Go service + headless Chromium | taken and reviewed |
| "Webcam" screenshots | **harness stand-in** (the mock camera presented as a USB webcam) — layout only, not a webcam test |
| **WebView2, Windows spooler, real printer, Canon EOS R50 / EDSDK, real UVC webcam, touch input** | **UNVERIFIED** |

Reproduce screenshots: see the header of `frontend/harness/operator-shots.mjs`.

## UNVERIFIED — to be run on Windows (WebView2, touch)
1. Fresh install, no camera: Prepare Booth shows *Setup required* with Camera setup one tap away; Open Booth Mode lists the camera and launches nothing.
2. UVC webcam only, no Canon SDK: Camera *Not verified* until a photo; Diagnostics shows Canon items as *Not needed*; guest photo works; Prepare shows *Ready*.
3. Canon EOS R50 with SDK: connect, live view, **Run camera test** fires the shutter once and the Gallery shows "Operator camera test"; EDSDK validation (`--canon-validate`) still passes.
4. Preferred camera unplugged: Prepare/Open Booth name it; nothing else is chosen automatically.
5. Unplug during the camera test: failure message, recovery, no stuck session, photos kept.
6. Printer: Digital only → no Print for guests; choose a printer → Windows status shown; **Test print…** confirm → paper appears; printer powered off → *Unavailable*, guests still save digitally.
7. Disk < 2 GB shows a warning, < 500 MB blocks Open Booth Mode.
8. Portrait tablet: top nav scrolls, labels readable, Prepare Booth is one column, no horizontal scroll, buttons ≥ 44 px.
9. Copy support summary pastes into a text file with no PIN, printer name, user name or paths.
10. Leave the Studio open 8 h: no growth in CPU while idle (readiness polling stays cheap).
