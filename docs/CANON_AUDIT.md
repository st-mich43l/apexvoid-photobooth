# Canon EDSDK integration — audit (Phase C1) and what changed

Scope: `internal/camera/edsdk`, `internal/camera` (manager), `internal/capture`, the Camera/Diagnostics UI.
Nothing here was run against a real camera or the real Canon DLLs: the project has neither in CI.

## What existed (and was kept)
* A single `api` boundary to `EDSDK.dll` (`dll_windows.go`), loaded dynamically, with a fake for tests — kept.
* One locked OS thread that owns every SDK call and pumps `EdsGetEvent` + the Windows message queue — kept.
* Header-constant verifier (`EDSDK_HEADERS=…`), struct-size tests, process-conflict detection (EOS Utility etc.),
  keep-alive against auto power-off, `SaveTo=Host` + `EdsSetCapacity`, RAW companion skipping — kept.
* Manager with reconnect + live-view restore; capture engine with retry/retake; JPEG decode check and atomic write when a
  photo is stored — kept.
* Tested with a fake SDK only. Everything about the real R50 was, and remains, **unverified**.

## Defects found in the original implementation and their fixes
| # | Defect | Effect | Fix |
|---|---|---|---|
| 1 | `Disconnect` could close the events channel while a state callback was sending to it | `panic: send on closed channel` when unplugging during teardown | sends/close serialised; test hammers the race (fails without the fix) |
| 2 | A transfer item taken from the queue was never released if the caller's context ended before the job was accepted | leaked native `DirectoryItem` reference per cancelled capture | items are completed on a detached, bounded context; test with cancel during capture |
| 3 | Transfers were associated with a photograph by queue order only; `drainItems` ran once before the shutter; the engine retries after a timeout | a photo that arrived late could be attached to the **next** slot | capture windows (only items that arrive while a capture waits belong to it), grace after a timeout, duplicate-name guard, counters; tests (fail with the window forced open) |
| 4 | `Connect` cancelled after `EdsOpenSession` ran left the session open | camera stays busy for other programs | native open runs to completion and is closed if the caller cancelled; test |
| 5 | Camera id = `port#list-index` | reorder / hot-plug of another body changed identity | id from the USB port path; test with reordered and extra bodies (fails with the index id) |
| 6 | Reconnect fell back to "any camera with the same name" | with two identical bodies, an unrelated body could be adopted | same id, or same USB id / unique name only when there were no look-alikes; tests |
| 7 | Event handlers were one process-wide slot | events of an old camera reached the new one; refs never released | callbacks routed by camera reference, cleared on disconnect, unknown refs released |
| 8 | Object events other than "request transfer" carried references that were never released | handle leak | released in the callback |
| 9 | `EdsInitializeSDK` failure after `LoadLibrary` left the DLL loaded and COM initialised | resource leak, bad retry | `Init` undoes its work on failure; retry test |
| 10 | `Close` waited for the SDK thread forever | a stuck call hung application exit | bounded wait that names the blocking call; Health shows busy/stuck calls |
| 11 | A capture waited its full timeout (25 s) after the camera was unplugged or disconnected | guests stared at a frozen countdown | disconnect wakes the waiting capture; tests for unplug and for operator disconnect |
| 12 | Results of abandoned SDK calls were written into the caller's variables | data race / use-after-return | results travel through buffered channels (`-race` clean) |
| 13 | No frame validity check, no complete-transfer check at the driver | a truncated transfer could be stored | size + SOI/EOI check, then the existing decode check; test |
| 14 | `Evf_Mode` defined but never set; live view not re-armed after a shot | R50-class bodies may drop the PC output after exposing → frozen preview | best-effort `Evf_Mode`, re-arm after a capture and on a run of empty frames (**unverified on hardware**) |
| 15 | SDK check was only "x64 and EdsImage.dll exists" | missing dependency or incompatible build surfaced as a vague load error | static PE inspection: architecture, imports, 25 required exports, each reported separately |
| 16 | A cable drop during a shot failed the whole session | frame and photos lost | session waits for the reconnect and repeats the slot; failed retake keeps the old photo |
| 17 | No mirrored preview option | — | display-only mirror setting |

## Still missing / not done
* Real-hardware verification of every capability (see docs/HARDWARE_TESTS.md; all UNVERIFIED).
* Camera serial number as identity (needs an open session). Two identical bodies on the same port path are told apart only by occurrence.
* Camera-native movie recording / transfer through EDSDK (explicitly out of scope; UVC/FFmpeg video unchanged).
* RAW-only workflows (reported as unsupported); RAW files are discarded, not stored.
* The residual late-transfer case described in docs/CAMERAS.md.
* Constants/signatures were re-read against what is known of EDSDK 13.x but cannot be compared with the headers here.
