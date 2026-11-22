# UX-3 operator workflow audit (before the change)

Audited on `master` at the UX-2 merge (`2a89ed0`): `Studio.tsx`, `pages/{Dashboard,CameraPage,PrinterPage,Settings,DiagnosticsPage,Frames}.tsx`,
`CanonPanel.tsx`, `state.tsx`, `api.ts`, and `internal/app/{readiness,diag,sources,settings,print}_api.go`.

## What an employee had to do
Start the app → read the Dashboard → open Camera (a long page with every control expanded) → open Printer → open Frames → open Settings
(for the media folder / countdown) → open Gallery (for interrupted sessions) → press "Open Booth Mode". Nothing told them the order.

## Findings
| # | Area | Observed behaviour | Effect |
|---|---|---|---|
| 1 | Readiness | `BoothReadiness()` treated a connected **simulated** camera as a normal, required "ok" line; headline "Ready for guests" | A demo camera looked like verified hardware |
| 2 | Diagnostics | Every unavailable driver / missing Canon SDK / missing FFmpeg was `ok:false` → red "problem", regardless of the camera in use | A valid webcam-only set-up looked broken |
| 3 | Diagnostics vs readiness | Diagnostics flagged free disk `< 2 GB` as a problem; readiness only blocks `< 500 MB` and warns `< 2 GB` | Two answers to the same question |
| 4 | Printer page | `printer = settings.printerName \|\| windowsDefault`: with *no* printer chosen the page showed the Windows default as "used by the booth", while the Booth hides Print when `printerName` is empty | UI said printing was on; guests got no Print button |
| 5 | Printer page | No explicit "Digital only" choice; test print lived in a secondary card | Optional printing was implicit |
| 6 | Camera page | One page: sources, Photo+Video buttons per device, backends, Canon panel, capabilities, resolution, all expanded; no test | Daily task (pick camera, look at picture) buried under advanced controls |
| 7 | Camera page | Device refresh on mount, button and every camera event had no ordering guard | A slow older scan could overwrite a newer list |
| 8 | Dashboard | Camera/Printer/Storage cards duplicate what the "Needs attention" list says; the fix-map for readiness keys lived inside the page | Duplicated mapping |
| 9 | Navigation | Flat list of 9 pages; in portrait an icon-only 76 px rail with labels hidden (`font-size:0`) | Unlabelled icons on a tablet |
| 10 | Open Booth | `onBooth` was called directly from three places (nav, Dashboard hero, …) with no fresh check; the guest screen gates each session later | Failure shown on the guest screen instead of to the operator |
| 11 | Settings | Countdown / mirror / DPI only reachable on the long Settings page (which already used `PatchSettings` correctly) | Slow for common changes |
| 12 | Support | No way to copy diagnostics for a support request | Operators screenshot pages |

## Already good (kept)
`PatchSettings` merge semantics, `guardOperation()` (no camera change during a session), `PrepareMode` (never switches camera silently),
printer list cache (8 s) and write-probe cache (3 s), test print behind a confirmation, reprint protection, spooler wording that never claims paper.

## Polling cost
Readiness is re-read every 10 s and on camera/settings change (150 ms debounce). It does **not** scan cameras: it reads the manager's
cached status; the printer list is cached for 8 s and the write probe for 3 s. UX-3 adds no timers; hardware lists are re-read only on page
open or an explicit "Refresh hardware status".
