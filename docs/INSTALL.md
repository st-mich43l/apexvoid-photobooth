# Install, run, test

## Run (Windows 10/11 x64)
1. Download the CI artifact `ApexVoidPhotobooth-windows-x64-portable` and unzip anywhere.
2. Requires the **WebView2 runtime** (already present on Windows 11 and updated Windows 10; otherwise install the
   Evergreen runtime from Microsoft).
3. Double-click `ApexVoidPhotobooth.exe`. No server, Docker, database or network is needed.
4. For webcams/video: put `ffmpeg.exe` next to the app (or use the `…-with-ffmpeg` artifact, an LGPL build).
5. For the Canon R50: follow docs/CAMERAS.md (SDK DLLs are not included).

Data: the media library is `<your Documents folder>\apexvoid-studio` (resolved with the Windows Known Folder API, so
OneDrive-redirected Documents and non-ASCII user names work): SQLite database, originals, previews/thumbnails, edits,
strips, exports, videos, templates, trash and logs. Settings stay in `%APPDATA%\ApexVoidPhotobooth\settings.json`.
Settings → Media library shows the active path and has **Open Library Folder**. A custom folder (Settings) is respected
and never migrated.

**Upgrading from the first version** (library in `%LOCALAPPDATA%\ApexVoidPhotobooth`): the app keeps using the old
folder and asks whether to copy it to Documents. The copy is verified, the old folder is left untouched as a backup
(marked `MIGRATED_TO.txt`), and capturing is paused until you restart the app. An interrupted copy resumes. If Documents
is unavailable or read-only, the app uses the old/fallback folder and shows a warning.

**Leaving Booth Mode:** the visible **Exit Booth** button (top-right), the **Esc** key, or holding the top-left corner
for 3 s — each opens "Exit Booth Mode and return to Studio?". A running recording is stopped and saved first; an
unfinished photo session is closed with its captured originals kept. Optional operator PIN: Settings → Kiosk exit PIN.
The exe is **unsigned**: SmartScreen may warn on first run.

Without a camera: Settings → enable **Simulated camera** to try the workflow (everything is labelled SIMULATED).

## Build
```
cd frontend && npm ci && npm run build          # writes cmd/photobooth/frontend_dist
go test ./internal/...                           # the cmd package needs WebKit to build on Linux
GOOS=windows GOARCH=amd64 go build -ldflags "-H windowsgui -s -w" -o ApexVoidPhotobooth.exe ./cmd/photobooth
```
The CI additionally embeds icon/manifest/version resources with go-winres.

Offline self-test of the whole stack (no window): `ApexVoidPhotobooth.exe --selftest report.json`.

## Tests
Go: unit, adapter contract (mock, UVC with synthetic source, Canon with a fake SDK), state machine, persistence,
crash recovery, disconnect/reconnect, repeat sessions, loopback server. Frontend: typecheck, unit tests, build.
Hardware-only checks: docs/HARDWARE_TESTS.md.
