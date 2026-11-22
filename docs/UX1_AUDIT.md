# UX-1 — guest experience and session lifecycle: audit and what changed

Scope reviewed: `Booth.tsx`, `ShootFlow.tsx`, `FrameSelection.tsx`, `ExitDialog.tsx`, `safeExit.ts`, `usePrepareMode.tsx`,
`state.tsx`, `ui.tsx`, `pages/Dashboard.tsx`, `Studio.tsx`, `internal/capture/capture.go`, `internal/app/shoot_api.go`,
`internal/app/sources_api.go` (+ the Settings/Printer/Video/Frames pages that save settings).
Nothing here was run in WebView2 or with real hardware — see the manual checklist at the end of `docs/HARDWARE_TESTS.md` (section G).

## Findings → fixes
| # | Finding (where) | Consequence | Fix |
|---|---|---|---|
| 1 | A 90 s timer in `Booth.tsx` set the screen to Welcome on **every** non-Welcome screen | a guest looking at their REVIEW photos, or a countdown/shutter/save in progress, was thrown out; the unmount cleanup in `ShootFlow` then cancelled the session | state-aware inactivity: only the frame choice, the live preview before the first shot and an already-saved result are ever reset by a timer; never COUNTDOWN/CAPTURING/REVIEW/SAVING or a camera reconnection |
| 2 | **Home** called `setScreen("welcome")` directly | same as 1, on one accidental tap, no explanation | Home is ignored while a photo is being taken/saved, asks first (and says what happens to the photos) in COUNTDOWN/REVIEW, leaves directly only when nothing is at stake |
| 3 | Finish called `onDone()` (screen → Welcome) **and** `resetShooting()`, and the unmount cleanup called `cancelShooting`/`resetShooting` again; a new session mounted at once and ran its own cleanup and `startPreview` | up to four overlapping resets racing the next preview start (the "stuck on All done / camera not opening" family) | one serialised, idempotent reset (`sessionGate` in the UI, `Engine.Recover` + a transition lock in Go); the next session waits for it; failure is visible and retryable; a hung reset times out (25 s) |
| 4 | `Engine.Reset` / `StartPreview` / `Cancel` were not serialised | a Reset racing a preview start left the engine at IDLE with live view **on** (reproduced by a new test) | `tr` mutex around those transitions; `Recover` waits for a save in flight and never cuts it off |
| 5 | Cancel and "Start over" abandoned a session with one tap | accidental loss of the strip (originals were kept) | confirmation dialog with the safe default ("Keep going") |
| 6 | `usePrepareMode.prepare` was not single-flight; a second call overwrote the pending promise | a frame-choice that never resolved (button stuck busy) / two overlapping camera switches | one preparation at a time; a second tap gets the first one's answer |
| 7 | `confirmFrame` could run twice on a fast double tap | same | single-flight guard |
| 8 | Booth "ready" meant only "a camera object exists" | guests could start on an unwritable library, a missing preferred camera, a full disk | backend **BoothReadiness** (required vs optional), enforced again server-side in `StartShooting` |
| 9 | Camera status events could arrive out of order and overwrite a newer one | stale "connected/reconnecting" | `Status.Version`; the UI drops older snapshots |
| 10 | Four pages saved the **whole** settings object they had loaded earlier | a stale copy rolled back newer values (camera chosen on another page, a preselected frame, …) | `PatchSettings` merges named keys into the backend's current copy; the Settings page sends only what it changed and takes over background updates |
| 11 | Dashboard read the printer list once; wording implied more than is known | a printer switched off later still showed ready; "ready" read as "prints work" | printers re-read every 10 s; text says what Windows reports and that paper output is not checked; no printer = digital only (not a problem) |
| 12 | In portrait the sidebar hid the **Open Booth Mode** button | no launch from most pages in a portrait kiosk | icon-only launch button stays visible in portrait |
| 13 | `Modal` had no focus management | keyboard/touch-keyboard users could tab behind a dialog; focus was lost on close | focus moves in, Tab is kept inside, Escape closes (once), focus returns to the opener; the Esc shortcut of Booth ignores an open dialog |
| 14 | Guests saw raw errors ("EDSDK…", error text) and a dead-end ERROR screen | confusing and not recoverable by a guest | friendly text, "your photos are safe", one large "Try again"; operators still see the real error in Studio |

## Not changed (on purpose)
Frame-selection-before-camera, no Video in Booth, the exit mechanisms (button, Esc, 3 s corner, PIN), `safeExit` order, Canon/UVC drivers,
printing, 300/600 dpi rendering, custom frames, the gallery. The 90 s idle limit on the frame screen is unchanged.

## Known limits
* A shutter in flight cannot be interrupted safely, so Home/Exit wait for it (Exit up to 15 s, Home until it ends).
* The 56 px touch target and portrait/landscape layout are CSS; jsdom cannot prove them. Section G lists what to look at on the real display.
* Readiness says what the software can observe. It cannot know that paper is in the printer or that a lens cap is off.
