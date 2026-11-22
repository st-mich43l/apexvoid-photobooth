# Cameras: adapters, setup and known limits

## Capability model

Every connected camera reports, at runtime (Studio → Camera):

| capability | meaning |
|---|---|
| liveView / stillCapture | preview stream / shutter |
| originalTransfer | the *camera's own* full-resolution file reaches the PC |
| movieRecord / movieTransfer | camera-native movie start/stop and movie file transfer |
| streamedVideo | the PC records frames the camera streams (UVC) |

Values: `supported`, `unsupported`, `unverified` (API exists, has not succeeded on this camera yet).

## Canon EDSDK adapter (EOS R50, Windows 10/11 x64)

### Install the SDK (you must do this; it is not bundled)
1. Download the EDSDK from Canon (developer registration required; read Canon's licence — **its
   redistribution terms were not verified by this project**, so the DLLs are never committed or shipped).
2. Check the SDK release notes list **EOS R50** (support was not confirmed by us; the notes we could see named
   R7/R10 for 13.18.10).
3. Copy the **64-bit** `EDSDK.dll`, `EdsImage.dll` (and the rest of the `Dll` folder) into one of:
   `<app folder>\`, `<app folder>\sdk\canon\`, or any folder set in Settings → *Canon EDSDK folder*.
   The app verifies the DLL is 64-bit and that `EdsImage.dll` sits beside it.

### Camera settings
* Mode dial on a **still-photo** mode (not Movie), lens attached, battery charged / AC adapter.
* Image quality **JPEG** or **RAW+JPEG** (RAW-only delivers nothing viewable; the app reports it).
* Disable auto power-off during events. USB cable straight into the PC (no hub).
* Quit **EOS Utility, EOS Webcam Utility, Canon Camera Connect, Windows Photos/Camera import** — only one
  program can own the camera. If another one does, connect fails with `device_busy` and the app names the running
  process (Diagnostics lists them). EOS Utility is *not* required.

### How it works
Photos are sent straight to the PC (`SaveTo = Host`, `EdsSetCapacity`). Shutter: `TakePicture`, falling back to the
shutter-button command if the body reports "not supported". The JPEG arrives via the object event
`DirItemRequestTransfer`; a RAW companion is acknowledged and discarded. Live view: `Evf_Mode` (best effort) and
`Evf_OutputDevice |= PC`, then `EdsDownloadEvfImage` at up to ~30 fps (the real rate depends on the body and USB link
and is **not** promised). USB loss: state event `Shutdown` or `DEVICE_NOT_FOUND`/`COMM_DISCONNECTED` → the manager
releases handles and reconnects automatically (restoring live view).

### Reliability design (Phase C)
* **One SDK thread.** Every EDSDK call and the callback pump run on one locked OS thread, one call at a time. Results
  travel back through channels, never through variables shared with a caller that may have given up.
* **Capture windows.** A transfer request belongs to a photograph only if it arrives while that capture is waiting
  for it. Anything else — a RAW companion that arrives after the JPEG, a photo that arrives after a timeout, a repeated
  file name — is acknowledged and dropped (and counted), so a **late transfer can never be attached to another shot**.
  After a timeout the next capture waits a short grace period (`LateGrace`, 6 s) with its window closed before it
  fires the shutter. *Residual risk:* a transfer delayed by more than the grace period *and* arriving inside a later
  capture window cannot be told apart by the SDK; this has not been observed and cannot be tested without hardware.
* **A capture succeeds only when a complete original has been received**: the announced size, the JPEG start and end
  markers, then the application's own decode check before it is written (atomically, unchanged) to the session. The
  shutter command returning success proves nothing.
* **No leaked native references.** Every transfer item is downloaded or skipped even if the caller cancelled; a
  half-opened session is closed when the caller cancels `Connect`; unknown object events are released; handlers are
  unregistered on disconnect (the Windows build routes callbacks by camera reference, so events of a closed camera never
  reach another one).
* **Bounded and observable shutdown.** `Close` waits at most 10 s for the SDK thread and then reports which call was
  blocking. Diagnostics show the SDK call in progress and flag one running longer than 20 s as stuck.
* **Disconnects wake waiting captures** at once; a disconnect racing a state callback can no longer panic.
* **Stable identity.** The camera id comes from the USB port path the SDK reports (never from list position), so
  reordering or hot-plugging another body does not change it. Reconnect accepts the same id, or the same model on the
  same USB id / a unique name when there were no look-alikes — **it never adopts another body** (two identical R50s are
  never guessed between). The camera serial number is *not* read (it needs an open session).
* **Mid-session recovery.** If the cable drops during a shot, the session, the frame snapshot and the photos already
  taken are kept; the engine waits (60 s) for the manager to reconnect, then repeats that slot from its countdown.
  A failed retake keeps the previous photo.
* **Static SDK check.** Before loading anything the app reads `EDSDK.dll`'s PE headers: architecture (x64), imported
  DLLs that cannot be found, and the 25 entry points it calls. Missing, wrong-architecture, damaged, incomplete and
  incompatible installs are reported separately, with what to do. If the SDK is unusable only the Canon source is
  unavailable; webcams keep working.
* **Mirror preview** (Settings): flips the displayed live view only; originals are never mirrored.

### Diagnostics and readiness
Studio → Camera → *Canon camera setup* lists, in order: SDK files, SDK initialised, other programs owning the camera,
camera detected, camera connected, live view, still capture, original transfer and the last error (classified as SDK
missing / busy / unsupported / disconnected / timeout / capture failed), a *Next step*, and the R50 Windows setup
guide. A line says **working** only if it succeeded on the connected camera in this session; otherwise it says
**not tried yet** or **problem**. Per-session counters (photos received, late transfers ignored, incomplete transfers
rejected) are shown too.

### Validation levels (what each kind of evidence means)
| level | meaning |
|---|---|
| Automated test passed | the logic behaves as intended against a **simulated** EDSDK (`go test ./internal/camera/edsdk`, also under `-race`) |
| Windows build verified | the application compiles for Windows x64 and its self-tests run on a CI runner **without a camera or Canon SDK** |
| Real hardware verified | a person ran `photobooth.exe --canon-validate --interactive` (or docs/HARDWARE_TESTS.md) with a real EOS R50 and recorded the result |
| Not tested | everything else |

`photobooth.exe --canon-validate [report.json] [--interactive] [--sessions N]` runs the 12 acceptance scenarios against
the connected camera with a throw-away library (the operator's media is never read or deleted) and writes a JSON report
and the evidence files. Each scenario is `PASS`, `FAIL` or `UNVERIFIED`; steps that need a person (unplug the cable,
start EOS Utility) are `UNVERIFIED` unless `--interactive` is given and answered. The script's own logic is tested against the simulated camera; that
says nothing about a real body.

### Video on the R50 — findings
| question | answer |
|---|---|
| Does EDSDK expose movie start/stop for the R50? | **Not tested.** Reported `unverified`; not implemented. |
| Does EDSDK transfer movie files for the R50? | **Not tested.** Reported `unverified`; not implemented. |
| Supported alternative implemented | R50 in USB **streaming (UVC)** mode → UVC adapter → FFmpeg records on the PC. Reported as `streamedVideo` (this is *not* camera-native recording). |

The UI never claims EDSDK recording works: starting a recording on the Canon adapter shows the limitation and the
UVC alternative.

> Constant values and struct layouts in `internal/camera/edsdk/types.go` were written from Canon's public
> headers and **must be compared with `EDSDKTypes.h` / `EDSDKErrors.h` of the SDK version you install**
> (`EDSDK_HEADERS=<folder> go test ./internal/camera/edsdk -run Headers -v`). The headers are proprietary and not in
> this repository, so CI cannot do it. Struct *sizes* are unit-tested. The audit that preceded Phase C is in
> docs/CANON_AUDIT.md.

## Camera sources (Phase 2)

Studio → Camera lists every source: name, kind, backend, USB id, supported resolutions and status. Backends:

| Source | Backend | Notes |
|---|---|---|
| Canon EOS R50 (photo) | Canon EDSDK | full-resolution originals; photo only |
| Built-in laptop camera, external USB webcam | Windows video device (FFmpeg/DirectShow) | frame grabs + video; "built-in vs USB" cannot be told apart reliably, both are labelled *Webcam* |
| USB capture card / HDMI grabber | same | labelled by name heuristics; capture where Windows exposes it as a video device |
| Virtual camera (OBS, EOS Webcam Utility, …) | same | needs its own program running; may hold the real camera exclusively |
| Canon body in USB streaming mode | same | labelled *Canon camera in UVC mode*; hidden as a **duplicate** while the same body is listed through the Canon SDK |
| Simulated | mock | development only |

* **Enumeration without FFmpeg (Windows):** Plug-and-Play devices of class *Camera*/*Image* via SetupAPI plus the
  DirectShow software-camera registry give names, USB ids and stable instance keys. FFmpeg, when present, adds the
  supported resolutions and the exact DirectShow names. *Media Foundation* was evaluated and **not** used: its enumeration
  and capture need COM vtable calls from Go that cannot be exercised without Windows hardware; the SetupAPI/registry path
  uses plain syscalls. Preview, photos and recording still need FFmpeg.
* **Stable ids** come from the USB instance key (`usb#vid_xxxx&pid_xxxx#instance`), not list order, so a replug is
  recognised. Identical names get distinct ids. A device moved to another USB port is re-found by USB id / unique name.
* **Duplicates** (same USB id reached twice; one Canon via the SDK + one Canon-vendor UVC device) are flagged and
  cannot be opened; only one camera is ever active.
* **Preferred cameras** are stored separately for Photo and Video and restored at start-up. If one is missing nothing is
  substituted silently: the operator is shown the candidate and must approve (PIN if set). Video accepts only UVC sources.
* **Switching** is refused during a session/recording/capture; otherwise: stop preview → release the old device and its
  FFmpeg process → connect → restart the preview if it was running. Resolution/fps is remembered per source.
* A webcam photo is described as a **frame grab at the stream resolution**, never as a camera original.

`photobooth.exe --probe-cameras [report.json] [--include-mock]` exercises every source (connect, live-view fps, still,
3 s recording, release) with a throw-away library and writes a JSON report plus the captured files. Steps that could not
run are `attempted:false` = **unverified**, never "passed".

### Canon hardening in this phase
Auto power-off is prevented (keep-alive every 30 s and on the camera's "will soon shut down" event). To verify the
constants against your real SDK: `EDSDK_HEADERS=<folder with EDSDKTypes.h/EDSDKErrors.h> go test ./internal/camera/edsdk -run Headers -v`
(skipped in CI because the headers are proprietary). Nothing else changed in the unverified-capability policy: movie
recording through EDSDK is still **not implemented and unverified**.

## UVC / webcam adapter

Works with any OS video device: USB webcams, capture cards, and the R50 in streaming mode.

* Needs **FFmpeg** (`ffmpeg.exe` beside the app, in `ffmpeg\`, Settings path, or PATH).
* Windows uses DirectShow (`dshow`); Linux V4L2 is implemented; macOS parse-only/untested.
* Stills are frames grabbed from the stream at the stream resolution (not camera originals).
* Mode choice prefers camera-compressed MJPEG at HD+ (raw YUV at 1080p is limited to a few fps on USB 2).
* H.264 encoder: `libx264` (GPL builds) → `h264_mf` (Media Foundation, LGPL builds) → NVENC/QSV/AMF → OpenH264.
* Microphone: choose in Studio → Video (`dshow` audio device). "No audio" records video only.
* Exclusive access: a device cannot be opened twice. If another app has it, the error says so.

What is tested in CI (Linux/Windows runners, **no camera**): FFmpeg command construction, device-list parsing
(sample DirectShow/V4L2/AVFoundation output), preview framing, recording + stop + probe, max-duration, crash
recovery, and the shared camera contract using a synthetic FFmpeg source. What is **not** tested: opening a real
DirectShow device, real microphone sync, a real R50.
