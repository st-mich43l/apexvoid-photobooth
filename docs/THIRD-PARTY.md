# Third-party software

## FFmpeg (single-file build only)
The standard CI artifact `ApexVoidPhotobooth-windows-x64` contains an **unmodified FFmpeg n8.1.3 (LGPL build)** by the
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
The standard build does not include Canon DLLs. CI may create a one-day intermediate artifact solely to test SDK extraction and native initialization on a camera-free Windows runner; it is not a release download. The applicable Canon agreement has not been reviewed for redistribution permission, so public Canon-enabled distribution is blocked. The ordinary build requires an independently authorized SDK installation. See docs/CAMERAS.md and `UX6_RELEASE_PLAN.md`.

## Noto Sans

The frame renderer embeds Noto Sans Regular as a fallback for text that its selected Go font cannot render, including Vietnamese characters. Copyright 2018 The Noto Project Authors; distributed under the SIL Open Font License 1.1. The font and full license are included in `internal/templates/fonts/`. Source: https://github.com/notofonts/noto-fonts/tree/main/hinted/ttf/NotoSans. Embedded file SHA-256: `b85c38ecea8a7cfb39c24e395a4007474fa5a4fc864f6ee33309eb4948d232d5`.
