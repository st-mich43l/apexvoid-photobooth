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

## B2. Windows webcam (first real-device test — no Canon needed)
Run `ApexVoidPhotobooth.exe --probe-cameras report.json` from PowerShell first (`Start-Process -Wait`), then the UI steps.
| # | Step | Expected |
|---|---|---|
| W1 | Camera → Refresh with the integrated and an external webcam. | Both listed with backend, kind, USB id; resolutions listed when FFmpeg is present; listed (with an FFmpeg note) when it is not. |
| W2 | Connect each in turn; Start preview; change resolution/fps. | Preview runs; switching releases the previous camera (the camera LED of the old one turns off); mode applies. |
| W3 | Photo → 1 shot. | JPEG in `<Documents>\apexvoid-studio\sessions\<id>\originals`; gallery says "UVC frame grab W×H"; never "original". |
| W4 | Video → choose a microphone, record 20 s, clap once. | MP4 with sound; A/V in sync; plays in the gallery. |
| W5 | Pull the USB cable during preview, replug after 5 s. | Banner "reconnecting"; preview resumes on the same source. |
| W6 | Same during a recording. | Partial recording saved and playable. |
| W7 | Set a different Photo and Video camera; restart the app. | Preferences restored; a missing one produces a note and a confirmation, never a silent switch. |
| W8 | Plug a Canon R50 in streaming mode while the SDK source exists. | UVC entry marked duplicate, not connectable. |
| W9 | Close the app during preview/recording. | LEDs off; no `ffmpeg.exe` left in Task Manager. |
| W10 | `--probe-cameras` report. | `liveView`, `still`, `video` steps `ok:true`, `releasedCleanly:true`; any `attempted:false` stays unverified. |

## C. Booth Mode (must be run in the real Wails/WebView2 window — CI only tests the logic with jsdom)
| # | Step | Expected |
|---|---|---|
| C1 | Studio → Open Booth Mode. | Window goes fullscreen; **Exit Booth** visible top-right. |
| C2 | Click Exit Booth → "Stay in Booth". | Dialog closes, still fullscreen. |
| C3 | Press Esc. Press Esc again. | Dialog opens; second Esc closes it. |
| C4 | Hold the top-left corner 3 s (mouse and touch). | Same dialog (not a direct exit). |
| C5 | Exit Booth → "Exit Booth". | Normal desktop window restored (not fullscreen, title bar back); repeat enter/exit 10×. |
| C6 | Repeat C5 from the Photo screen mid-countdown, in REVIEW, and after unplugging the camera (error). | Exits; photos already taken are in the gallery; camera not left busy. |
| C7 | Repeat from the Video screen while recording. | Dialog says the recording will be stopped and saved; after exit the MP4 plays in the gallery. |
| C8 | Set a PIN (Settings), open Booth, exit with a wrong then the right PIN; 5 wrong PINs. | Wrong PIN refused; lock for 30 s after 5; right PIN exits. |
| C9 | Portrait and landscape touchscreen: Photo and Video flows, targets large enough. | OK. |

## C2. Documents library (Windows)
| # | Step | Expected |
|---|---|---|
| L1 | Fresh machine/user: start the app, shoot (simulated or real). | Files appear under `<Documents>\apexvoid-studio` (check the real Documents path, incl. OneDrive-redirected). |
| L2 | Settings → Media library. | Shows that path; **Open Library Folder** opens Explorer there. |
| L3 | Upgrade test: place a Phase-1 library in `%LOCALAPPDATA%\ApexVoidPhotobooth`, start the new build. | Existing media visible; migration prompt appears; nothing copied yet. |
| L4 | Accept the copy; watch progress; restart. | New library has all media, plays videos; old folder untouched with `MIGRATED_TO.txt`; capturing paused until restart. |
| L5 | Kill the app mid-copy; restart and accept again. | Resumes; no duplicates; verified. |
| L6 | User name with non-ASCII characters; Documents on OneDrive; read-only Documents. | Works / clear warning + fallback. |

## D. Printing (Windows; physical output must be judged by a person)
Prepare: install the printer driver, set the paper in the printer, Printer page → Refresh.
| # | Step | Expected |
|---|---|---|
| D1 | Printer page. | Installed printers listed with ready/offline/error, the Windows default marked; paper sizes of the selected printer listed. |
| D2 | Switch the printer off / unplug it / set "Use printer offline". | Shown as offline with an explanation; Print disabled. |
| D3 | Test print on 4×6. Measure the grid with a ruler. | Grid squares are 10 mm ±0.5; the 25 mm corner squares are complete unless the printer has an unprintable border (then the cut matches the border shown in the preview). |
| D4 | Print a 4-photo strip as **2×6 on 4×6** (fit) and as **two strips on 4×6**. | Correct size (2×6 in = 50.8×152.4 mm), centred, nothing cut off; the 2-up sheet cuts into two identical strips. |
| D5 | Print a single edited photo (rotate/brightness) on 4×6 and 5×7, fit and fill. | The **edited** picture is printed (not the original); orientation correct; "fill" crops as the preview showed. |
| D6 | Click Print twice quickly; click Print, wait 5 s, click again. | One job; the second says "identical print was just submitted"; Reprint needs confirmation. |
| D7 | Watch the queue while printing; pull the paper tray / unplug mid-job. | queued → printing → "Left the queue" (or **failed** with the reason, e.g. out of paper). |
| D8 | Close the app while a job is printing, reopen. | The job is tracked again (or "unknown" with instructions); history is still there. |
Record the printer model, driver, paper and result in the table below; any step not run stays **unverified**.

## Results
| Step | Date | Tester | Camera/firmware/SDK | Pass/Fail | Notes |
|---|---|---|---|---|---|
