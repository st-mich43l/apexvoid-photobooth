# UX-2 — Premium Korean-style guest experience

Scope: the **guest (Booth Mode) interface only**. No backend/capture/state-machine changes. UX-1's session-safety
logic (`sessionGate`, `Booth.tsx` idle rules, `LeaveDialog`, `usePrepareMode`, operator exit) is untouched; only the
markup and styling it drives changed.

## Design language
- **Tokens, not hex codes** — `frontend/src/guest/guest.css` defines semantic `--g-*` tokens on `.booth-root`
  (`--g-bg #120D23`, `--g-elev #241A40`, `--g-elev-2 #35245A`, `--g-primary #9275E8`, `--g-rose #E8A0C3`, `--g-text #F7F2FA`, …).
  `guest.test.tsx` checks the text/background pairs against WCAG AA (4.5:1).
- **One primary action per screen**, always bottom-right, rose→lilac gradient with dark text. Back/Home is always bottom-left in
  the same place (frames: in the action bar; photo screens: the Home button). Secondary actions are outlined; disabled controls are
  flat and dashed so they cannot be mistaken for enabled ones.
- **Journey header**: brand + `Frame · Shoot · Review · Done` indicator (informational, never a button). The operator Exit button
  keeps the top-right corner (z-index above the change-design overlay); the top-left long-press corner is unchanged.
- Rounded, low-noise surfaces; no glows; motion is `transform`/`opacity` only.
- Guest and operator stay distinguishable: all guest CSS is scoped under `.booth-root`; Studio styles are untouched.

## Screens
| | Landscape | Portrait |
|---|---|---|
| **A Welcome** | brand, "Say cheese!", subtitle, large *Take photos*; floating strip artwork (aria-hidden, `pointer-events:none`) | same, artwork above/below |
| **B Frames** | gallery (left, scrolls on its own) + selected preview/colours/caption (right); Back / Continue bar | gallery above a compact selected summary; same bar. Caption focus hides the gallery so the on-screen keyboard cannot cover the field |
| **C Preview** | camera stage dominates; frame thumbnail + *Change frame* in a side column; "Get ready!" + *Start shooting · N photos* | stage takes the height, frame row above it |
| **D Countdown** | big number in a dark disc (readable on bright/dark scenes), "Photo 2 of 4" pill, "Smile!" + 320 ms white flash on `CAPTURING` | same |
| **E Review** | photos in capture order, each tile numbered with its own *Retake photo N*; frame summary + *Change design* / *Start over*; *Looks good* | 2-column tiles |
| **F Result** | finished image large (any aspect), "All done!", *Print* (only if a printer is configured) + *Change design*, *Finish* | image above, tools below |

Filters: categories + photo-count chips; cards show layout (2×6 / 4×6 / 5×7), a dot per photo, "N photos" and colour dots;
selected card has a rose ring + check and is scrolled into view after filtering. A fade + "More frames" pill signals more content.

## Behaviour that must not change (verified by tests)
- Countdown number, photo progress and flash are rendered **from the Go snapshot**; CSS never decides when the shutter fires.
- Overlays are `pointer-events:none`; reduced motion turns every animation/transition off.
- The preview `<img>` is the same single `LivePreview` (one MJPEG connection), `object-fit: contain`, never stretched; mirror stays CSS-only.
  The stage is not remounted between PREVIEW → COUNTDOWN → CAPTURING.
- Review tiles take the photo's real aspect ratio after load (container-query sizing) — no crop, no stretch.
- Retake/Accept/Finish/Print keep their existing guards (`busy`, `finishing`, one request id per print tap). Print says
  "Sent to the printer…", never "printed".
- Frame lists, `fitFor`/explain/`FrameDecision`, previews from the Go renderer (`frameUrl`) are unchanged; frame count is never hard-coded.

## Performance notes
- Gallery images: `loading="lazy"`, `decoding="async"`; Go renders previews at 420 px (gallery) / 720 px (landscape detail) / 320 px (portrait summary).
- No `backdrop-filter` anywhere over/near the live preview (the old booth pill had one; removed). Shadows are static.
- Animations: opacity/transform only; decorative loops (strip float, twinkle) exist only on Welcome.
- Production bundle: 408 kB JS (122 kB gzip) vs 364 kB before; no runtime asset fetches, no web fonts (system stack), works offline.
- **Not measured**: frame-rate on a real kiosk PC / WebView2. Only headless Chromium on a dev VM was available.

## Accessibility notes
Real `<button>`s and radio groups (frames, colours) with names; 4 px focus ring; step indicator exposes `aria-current`;
progress is a `progressbar`; overlays `aria-hidden` / `aria-live`; targets ≥ 48 px (primary ≥ 80 px at kiosk sizes).

## Verification status
| What | Status |
|---|---|
| Frontend unit/component tests (255) incl. all UX-1 session/safety suites | pass |
| `tsc --noEmit`, `vite build` | pass |
| Go tests `./internal/app ./internal/capture` | pass |
| `GOOS=windows go build ./cmd/photobooth` (cross-compile from Linux) | compiles; the binary was not run |
| Real flow (simulated camera, real Go renderer, fake printer) in headless Chromium at 1920×1080, 1080×1920, 1366×768, 768×1024, 1536×864 (=1080p@125 %), 1280×720 (=1080p@150 %) | screenshots in `docs/img/ux2/`, each reviewed; script reports no horizontal overflow / clipped buttons |
| **WebView2 / Windows / touchscreen / OSK / Canon EDSDK / UVC / real printer** | **NOT tested** — use the checklist below |

Reproduce screenshots: `cd frontend && npx vite build -c vite.harness.config.ts && cd .. && go run -tags uxharness ./cmd/uxharness -addr 127.0.0.1:5199 -dist frontend/dist-harness &`
then `cd frontend && NODE_PATH=$(npm root -g) node harness/shots.mjs ../docs/img/ux2`. The harness (build tag `uxharness`, not in product builds)
runs the real `app.Service` headless behind HTTP; the frontend is built with a shim replacing `@wailsio/runtime`.
Screenshots show a **simulated** camera (test patterns), so photos are not real faces.

## Known limitations
- At 150 % Windows scaling on 1080p (1280×720 CSS px) the frame gallery shows about one row; scrolling + the "More frames" cue cover it.
- Portrait caption entry relies on the layout switch (`data-typing`); whether WebView2 resizes the viewport for the Windows OSK is unverified.
- The live stage letterboxes non-matching camera aspect ratios with the dark stage colour (by design; no cropping).
- Retake still goes through the engine's existing flow (countdown → capture of that slot); the old photo is replaced by the engine only after the new one is saved (unchanged UX-1/Go behaviour).

## Manual WebView2 acceptance checklist (Windows touchscreen)
1. Launch, enter Booth Mode: fullscreen, Welcome fully visible with no scrolling; *Take photos* reacts instantly to touch.
2. Unplug camera → Welcome shows the "ask the host" message and a visibly disabled button; replug → enabled.
3. Frames: scroll with a finger (no accidental selections), filter chips, colour swatches, caption with the **on-screen keyboard** — field stays visible, Continue reachable.
4. Preview with Canon EDSDK, a UVC webcam and the simulated camera: image never stretched; mirror on/off only flips the display, saved original unchanged.
5. Countdown: number readable on bright and dark scenes; shutter fires on the Go state, not the animation; flash does not delay capture.
6. Pull the USB cable mid-session: calm "Reconnecting" card; photos kept; recovery works.
7. Review: each Retake replaces only its photo; Change design (same count / fewer / more photos) explains and confirms; Start over asks first.
8. Result: 2×6, 4×6 and a custom layout all fit; Print with a real printer → "Sent to the printer…"; double-tap prints once; digital-only shows no Print.
9. Finish returns to Welcome; immediate *Take photos* works; leave on REVIEW for > 2 min → never auto-resets.
10. Exit Booth button, Esc, 3 s top-left hold and PIN all still work, including while *Change design* is open.
11. Repeat at 100 %, 125 %, 150 % scaling and in portrait (rotated display): no clipped controls, no horizontal scroll, dialogs fit.
12. Run 30 sessions back-to-back; watch for dropped frames in the live preview and memory growth.
