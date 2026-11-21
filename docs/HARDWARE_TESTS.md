# Hardware acceptance tests (require a Windows PC + camera)

CI cannot run these. Record the result of each step in the table at the bottom. Until a step is ticked by a
person with the hardware, treat it as **unverified**.

Prerequisites: Windows 10/11 x64, Canon EOS R50 + USB cable, Canon EDSDK x64 DLLs in place (docs/CAMERAS.md),
EOS Utility closed. Download the portable artifact from the CI run and unzip it.

## A. Canon EOS R50 via EDSDK
| # | Step | Expected |
|---|---|---|
| A1 | Confirm the SDK release notes list the R50. Studio → Diagnostics. | `canon-edsdk` driver "ready"; no conflicting programs. |
| A2 | Camera on, Photo mode, JPEG. Studio → Camera → Refresh. | "Canon EOS R50" listed (driver `canon-edsdk`). |
| A3 | Connect. | Connected; capabilities show live view / still capture **unverified**. |
| A4 | Start live view. | Live image in ~1–2 s, smooth; capability flips to **supported**. |
| A5 | Photo → Start (1 shot). | Countdown, shutter fires, review shows the photo; original is a full-resolution JPEG in `sessions\<id>\originals`; capability `originalTransfer` **supported**. |
| A6 | Compare the file with the card copy (same pixel size, EXIF present). | Matches. |
| A7 | 4-photo session, retake photo 2, accept. | Strip created; gallery shows originals + strip; no simulated badge. |
| A8 | Open EOS Utility first, then connect in the app. | Error `device_busy` naming EOS Utility, with an action. |
| A9 | During live view, unplug USB, wait 5 s, replug. | "reconnecting" banner; automatic reconnect; live view restored. |
| A10 | Unplug during the countdown of a session. | Session ends in ERROR with an action; photos taken so far are kept; Reset works. |
| A11 | Close the app (X) mid-live-view, then start EOS Utility. | EOS Utility connects immediately (camera not left busy); app log shows clean shutdown. |
| A12 | Record any struct/constant mismatch found vs `EDSDKTypes.h`. | None, or fix `types.go`. |
| A13 | (Investigation) Does movie start/stop / movie transfer work via EDSDK on the R50? | Document the finding; capability stays unverified until implemented and proven. |

## B. R50 (or webcam) via UVC
| # | Step | Expected |
|---|---|---|
| B1 | Set R50 USB mode to streaming; place `ffmpeg.exe` beside the app. Camera → Refresh. | Device listed under `uvc`. |
| B2 | Connect + live view. | Smooth preview; Video page lists modes and microphones. |
| B3 | Record 20 s with a microphone, stop. | MP4 in gallery, plays with sound; lip-sync correct (clap test); duration ≈ 20 s. |
| B4 | Record, unplug USB mid-recording. | Recording saved as **partial**, playable. |
| B5 | Kill the app process during recording; restart. | Recording recovered as partial. |

## C. Booth Mode
Portrait and landscape touchscreen: Photo and Video flows, large targets, hold top-left corner 3 s exits.

## D. Printing (not part of Phase 1 acceptance)
Discovery lists installed printers. Submission on Windows is not implemented.

## Results
| Step | Date | Tester | Camera/firmware/SDK | Pass/Fail | Notes |
|---|---|---|---|---|---|
