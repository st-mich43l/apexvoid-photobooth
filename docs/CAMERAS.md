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
`DirItemRequestTransfer`; a RAW companion is acknowledged and discarded. Live view: `Evf_OutputDevice |= PC`
and `EdsDownloadEvfImage` at ~30 fps. USB loss: state event `Shutdown` or `DEVICE_NOT_FOUND`/`COMM_DISCONNECTED`
→ the manager releases handles and reconnects automatically (restoring live view).

### Video on the R50 — findings
| question | answer |
|---|---|
| Does EDSDK expose movie start/stop for the R50? | **Not tested.** Reported `unverified`; not implemented. |
| Does EDSDK transfer movie files for the R50? | **Not tested.** Reported `unverified`; not implemented. |
| Supported alternative implemented | R50 in USB **streaming (UVC)** mode → UVC adapter → FFmpeg records on the PC. Reported as `streamedVideo` (this is *not* camera-native recording). |

The UI never claims EDSDK recording works: starting a recording on the Canon adapter shows the limitation and the
UVC alternative.

> Constant values and struct layouts in `internal/camera/edsdk/types.go` were written from Canon's public
> headers from memory and **must be compared with `EDSDKTypes.h` / `EDSDKErrors.h` of your SDK version** during
> hardware validation (HARDWARE_TESTS.md step 2). Struct *sizes* are unit-tested.

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
