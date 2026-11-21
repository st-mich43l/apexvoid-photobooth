# Third-party software

## FFmpeg (single-file build only)
The CI artifact `ApexVoidPhotobooth-windows-x64` (the only one) contains an **unmodified FFmpeg n8.1.3 (LGPL build)** by the
BtbN FFmpeg-Builds project (`ffmpeg-n8.1.3-39-g660b5aa7db-win64-lgpl-8.1`), stored gzip-compressed inside the executable.

* It is **not linked** into the application. At first use it is unpacked to
  `%LOCALAPPDATA%\ApexVoidPhotobooth\bin\ffmpeg-<hash>.exe` and started as a **separate process**.
* You can replace it: put your own `ffmpeg.exe` next to the application, or set its path in Settings — both take
  precedence over the built-in copy. Delete the `bin` folder to remove the unpacked copy.
* Licence: GNU LGPL v2.1 or later (text in `FFMPEG-LICENSE.txt`, also unpacked next to the executable). This build has
  no GPL parts (no libx264); H.264 video uses the Windows Media Foundation encoder (`h264_mf`) or a hardware encoder.
* Source: FFmpeg source for this version is available from https://ffmpeg.org/ (tag n8.1, release 8.1.x) and the
  build scripts/configuration from https://github.com/BtbN/FFmpeg-Builds (release tag `autobuild-2026-11-21-12-37`).
  FFmpeg is a trademark of Fabrice Bellard.

A local `go build` without `-tags embedffmpeg` contains no third-party binary.

## Canon EDSDK
Not included and never committed or redistributed: copy the DLLs from your own Canon download (docs/CAMERAS.md).
