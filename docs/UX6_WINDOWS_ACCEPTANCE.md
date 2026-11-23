# UX-6 Windows acceptance protocol

Status: **UNVERIFIED** until executed and recorded on each target kiosk. Record date, tester, app commit and SHA-256, Windows build, architecture, WebView2 version, display, scaling, camera/firmware and printer/driver. Do not include customer photos, file paths, usernames or device serial numbers in the repository. Use synthetic test subjects and disposable test libraries.

## Desktop and kiosk

Run on supported Windows 10 x64 and Windows 11 x64 in the actual Wails/WebView2 window at 1920×1080 landscape, 1080×1920 portrait and 1366×768, with 100%, 125%, 150% and 200% scaling where supported. For each, record PASS / FAIL / UNVERIFIED and screenshots without personal information.

Complete Welcome → Choose Frame → Preview → Countdown → Capture → Review → Retake → Result → Print → Finish; start a new session. Check clipped controls, touch targets, fullscreen, scroll, context menu, image drag and selection, Back/Home, operator PIN, operator exit via button/Escape/long press, on-screen keyboard occlusion, reconnect messages, responsiveness while saving, and accessibility of Windows recovery. Exercise repeated Booth enter/exit. Do not lock out the operator or Windows recovery to make the kiosk appear more secure.

## Camera: Canon EOS R50

Use a licensed SDK installation, USB data cable, reliable power and an empty test library. Run `ApexVoidPhotobooth.exe --canon-validate report.json --interactive` on the actual PC. Inspect and sanitize any report before sharing; do not upload customer imagery or private paths. Record SDK version, model, firmware, Windows build, driver status, capture duration, JPEG dimensions, EXIF retention, session counts and resource usage.

For JPEG and RAW+JPEG separately verify discovery, SDK initialization, connection, actual live view, shutter, full-resolution JPEG transfer, EXIF/dimensions, a four-photo session, retake, final frame, at least 20 consecutive physical sessions, USB reconnect, disconnect during countdown and capture, EOS Utility busy state, shutdown release, absent Canon at startup, and process-termination recovery. Try RAW-only and record the unsupported/error behavior. After a timeout, cancellation or failed transfer, stop the session and power-cycle and reconnect the camera before further shooting; verify delayed transfers cannot appear in a later slot. The SDK does not supply a proven universal capture correlation token; connection reset behavior and arbitrarily late events remain a physical acceptance risk.

## Webcam and recording

Test integrated and external USB webcams, multiple sources, source switching/release, selectable resolution and frame rate, live view, still frame grab, video, microphone, A/V sync, USB reconnect, shutdown while recording and partial-recording recovery. Verify the webcam works with no Canon SDK and the simulated camera is labeled. A webcam frame grab is not a full-resolution Canon still.

## Physical printer

Record printer/driver/paper and measured dimensions, margins, orientation, DPI, scaling and physical output. Test discovery and paper capabilities; offline, paused and error states; calibration; 2×6 on 4×6, two-up strips and full-sheet 4×6; portrait/landscape; 300/600 DPI; margins and crop; reprint DPI; double tap; unplug during submission; restart recovery; unknown-job investigation; digital-only fallback. Run at least 30 physical print jobs and count actual sheets. Windows spooler completion is not proof of paper delivery; never silently reprint an unknown job.

## Filesystem and recovery

With copies of synthetic libraries, test default and OneDrive-redirected Documents, custom paths, non-ASCII usernames, long paths, read-only and low-space volumes, NTFS and other intended filesystems, and removable/network paths if allowed. Test forced process termination during capture, ZIP export, trash, restore, migration, frame save and print submission; restart and check originals, metadata and journal state. Hard-link support and Windows share semantics must be demonstrated on each intended filesystem. Do not disconnect media or cut power on a customer's only copy.

## Endurance targets

Record start/end times, memory, handle count, CPU, failures and recovery observations for 100 simulated sessions, 20 physical Canon sessions, 30 physical prints, repeated device reconnects, an eight-hour idle soak, repeated Booth entry/exit and process/power interruption. These are proposed targets, not measured results. Record actual results in `UX6_HARDWARE_RESULTS.md`.
