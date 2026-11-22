# Hardware acceptance tests (require a Windows PC + camera)

CI cannot run these. Record the result of each step in the table at the bottom. Until a step is ticked by a
person with the hardware, treat it as **unverified**.

Prerequisites: Windows 10/11 x64, Canon EOS R50 + USB cable, Canon EDSDK x64 DLLs in place (docs/CAMERAS.md),
EOS Utility closed. Download the `ApexVoidPhotobooth-windows-x64` artifact from the CI run and unzip it.

## A. Canon EOS R50 via EDSDK (Phase C acceptance)
Run the automated script first, then the manual rows it cannot cover. **Nothing below has been run on real hardware
by the project: every result is UNVERIFIED until a person records it in the table at the end.**

```powershell
Start-Process -Wait .\ApexVoidPhotobooth.exe -ArgumentList '--canon-validate','C:\temp\canon.json','--interactive'
```
It uses a throw-away library (your media is not touched), prints one line per scenario and writes the JSON report and
the evidence files (live-view frames, the first photo, the strip). `PASS` = the automated criterion held;
`UNVERIFIED` = not run or needs a person; scenarios marked *review* still need you to look at the evidence.

| # | Scenario | How it is checked | Result |
|---|---|---|---|
| 1 | Camera detected | script step 1 (SDK folder check, then discovery); Studio → Camera → *Canon camera setup* | UNVERIFIED |
| 2 | Camera connected | step 2 | UNVERIFIED |
| 3 | Live view shows the physical camera feed | step 3 counts frames and saves two; **review** the image | UNVERIFIED |
| 4 | Shutter command captures an actual photograph | step 4; **review**: shutter sound, scene visible | UNVERIFIED |
| 5 | Full-resolution JPEG transfers | step 5: decodes, ≥ 1920 px long edge, written byte-identical; compare with the camera's JPEG size (R50 L = 6000×4000) | UNVERIFIED |
| 6 | EXIF and dimensions preserved | step 6 finds the Canon EXIF block in the original; compare with the card copy | UNVERIFIED |
| 7 | Four-photo session completes | step 7: 4 distinct originals (no late transfer attached to the wrong slot) | UNVERIFIED |
| 8 | One photo can be retaken | step 8: only slot 2 changes | UNVERIFIED |
| 9 | Strip produced with the selected frame | step 9; **review** `strip.jpg` | UNVERIFIED |
| 10 | USB disconnect and reconnect recover | step 10 (`--interactive`: unplug, replug); also unplug *during the countdown* of a session — the session must wait, then continue with the same frame and photos | UNVERIFIED |
| 11 | Closing ApexVoid releases the camera for EOS Utility | step 11 (`--interactive`: start EOS Utility, answer y/n) | UNVERIFIED |
| 12 | Repeated sessions: no leak, camera never busy | step 12 (`--sessions N`): goroutines and OS handle count stay flat | UNVERIFIED |

Manual rows (not in the script):
| # | Step | Expected | Result |
|---|---|---|---|
| A0 | Run `EDSDK_HEADERS=<Canon Header folder> go test ./internal/camera/edsdk -run Headers -v`; confirm the SDK release notes list the R50. | No constant mismatch; R50 listed. | UNVERIFIED |
| A1 | Open EOS Utility first, then connect in the app. | Error `device_busy` naming EOS Utility, with an action (the setup panel says "busy"). | UNVERIFIED |
| A2 | Remove `EdsImage.dll`, restart; then use the 32-bit DLL. | Setup panel says *incomplete* / *wrong architecture* with the action; webcams still work. | UNVERIFIED |
| A3 | Set image quality to RAW only. | Capture reports "only a RAW file"; RAW+JPEG works and the JPEG is used. | UNVERIFIED |
| A4 | Take 20 shots quickly (retakes included); watch for a wrong or repeated image. | Every slot holds the photo taken for it. | UNVERIFIED |
| A5 | Settings → *Mirror the live preview*. | Preview mirrored; the saved photo and strip are not. | UNVERIFIED |
| A6 | (Investigation, not implemented) Movie start/stop and movie transfer through EDSDK. | Document the finding; capability stays unverified. | UNVERIFIED |

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


## Frames (Phase 3) — to be done on the real Windows machine

| # | Step | Expected | Result |
|---|---|---|---|
| F1 | Booth → Photo | Frame selection appears **before** the camera starts (no live view, no shutter sound) | ☐ |
| F2 | Pick a 4-photo Korean frame, choose a colour, Continue | Camera prepares, live view shows with the frame thumbnail beside it, button says "4 photos" | ☐ |
| F3 | Shoot four real photos, retake one, accept | Strip is composed in the chosen frame; file is under `Documents\apexvoid-studio\sessions\<id>\exports` | ☐ |
| F4 | On the review/result screen choose *Change design* with a different 4-photo frame | New design appears without retaking; earlier image stays in the Gallery | ☐ |
| F5 | Choose a 3-photo frame after taking 4 | The app explains and offers "Use the first 3 photos" — it never switches silently | ☐ |
| F6 | Studio → Frames → Create frame, import a transparent PNG overlay (600×1800), move the 4 photo slots, Save, enable | Frame appears in Booth; overlay sits over the photos, transparent windows show them | ☐ |
| F7 | Print the strip on a 4×6 printer (Printer page / Booth *Print*) | Preview shows two strips on a 4×6 sheet; the paper has two identical strips, cuttable in half, correct size and margins | ☐ |
| F8 | Edit then delete the frame used for a finished session, reprint | The reprint is identical to the first print | ☐ |
| F9 | Touch the builder with a finger (drag a slot, resize with a handle) | Works; note any problem | ☐ |
| F10 | Settings → Print resolution 600 dpi, shoot, print the strip on the real printer (driver set to its highest quality) | File is 1200×3600 px; paper is 2×6 in with sharp detail; compare with a 300 dpi print | ☐ |
| F11 | Studio → Frames → *Import PNG as frame…* with your real frame PNG | Builder opens with a photo slot under each transparent window and a note saying how many were found; Save, offer, shoot: photos appear exactly in the windows | ☐ |
