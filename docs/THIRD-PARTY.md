# Third-party software

ApexVoid Photobooth itself is MIT-licensed (`LICENSE`). The components below are not covered by that licence; each
keeps its own terms.

## FFmpeg (single-file build only)
The single-file CI artifact `ApexVoidPhotobooth-windows-x64` contains an **unmodified FFmpeg n8.1.3 (LGPL build)** by the
BtbN FFmpeg-Builds project (`ffmpeg-n8.1.3-39-g660b5aa7db-win64-lgpl-8.1`), stored gzip-compressed inside the executable.

* It is **not linked** into the application. At first use it is unpacked to
  `%LOCALAPPDATA%\ApexVoidPhotobooth\bin\ffmpeg-<hash>.exe` and started as a **separate process**.
* You can replace it: put your own `ffmpeg.exe` next to the application, or set its path in Settings — both take
  precedence over the built-in copy. Delete the `bin` folder to remove the unpacked copy.
* Licence: GNU LGPL v3 (the embedded build’s `LICENSE.txt` is extracted alongside FFmpeg and included in the `--notices` output). This build has
  no GPL parts (no libx264); H.264 video uses the Windows Media Foundation encoder (`h264_mf`) or a hardware encoder.
* Source: FFmpeg source for this version is available from https://ffmpeg.org/ (tag n8.1, release 8.1.x) and the
  build scripts/configuration from https://github.com/BtbN/FFmpeg-Builds (release tag `autobuild-2026-11-21-12-37`).
  FFmpeg is a trademark of Fabrice Bellard.

A local `go build` without `-tags embedffmpeg` contains no third-party binary.

## Canon EDSDK
Trusted master CI builds one portable executable with the Canon EDSDK DLLs embedded. The EDSDK is proprietary to Canon and is redistributed with the application under Canon's authorization; it is not covered by the MIT License. The app extracts those DLLs to the user cache when the Canon driver is used. Pull-request jobs do not receive the SDK vault password or publish the portable artifact. See docs/CAMERAS.md.

## Noto Sans

The frame renderer embeds Noto Sans Regular as a fallback for text that its selected Go font cannot render, including Vietnamese characters. Copyright 2018 The Noto Project Authors; distributed under the SIL Open Font License 1.1. The font and full license are included in `internal/templates/fonts/` and the executable’s `--notices` output. Source: https://github.com/notofonts/noto-fonts/tree/main/hinted/ttf/NotoSans. Embedded file SHA-256: `b85c38ecea8a7cfb39c24e395a4007474fa5a4fc864f6ee33309eb4948d232d5`.
