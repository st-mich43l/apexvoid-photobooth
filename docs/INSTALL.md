# Install, run, test

## Run (Windows 10/11 x64)
1. Download `ApexVoidPhotobooth-windows-x64` from a **successful master CI run** and unzip it. The artifact contains one unsigned file, `ApexVoidPhotobooth.exe`, with the pinned FFmpeg and Canon EDSDK DLLs embedded.
2. Requires the **Microsoft Edge WebView2 Evergreen Runtime**. ApexVoid checks its Windows registration before starting the window and shows a recovery message when it is missing. Obtain it from [Microsoft's WebView2 download page](https://developer.microsoft.com/microsoft-edge/webview2/). For offline kiosks, download the x64 Evergreen Standalone Installer on an internet-connected computer, transfer it to the kiosk, and run it with operator knowledge. Microsoft describes [offline deployment and detection](https://learn.microsoft.com/microsoft-edge/webview2/concepts/distribution). No prerequisite is downloaded silently. Registration alone cannot guarantee that a damaged runtime starts successfully; actual WebView2 acceptance is still required.
3. Double-click `ApexVoidPhotobooth.exe`. No server, Docker, database or network is needed.
4. Webcams and video work out of the box: FFmpeg is unpacked from the exe on first use (see docs/THIRD-PARTY.md). To use your own FFmpeg, put `ffmpeg.exe` next to the app or set its path in Settings; that copy always wins over the built-in one.
5. For the Canon R50: connect the camera and follow [the camera guide](CAMERAS.md). The embedded SDK is extracted automatically; no manual DLL installation is needed for the CI portable build.

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

Export embedded third-party notices to a new file: `ApexVoidPhotobooth.exe --notices notices.txt`.

Offline self-test of the whole stack (no window): `ApexVoidPhotobooth.exe --selftest report.json`.

## Tests
Go: unit, adapter contract (mock, UVC with synthetic source, Canon with a fake SDK), state machine, persistence,
crash recovery, disconnect/reconnect, repeat sessions, loopback server. Frontend: typecheck, unit tests, build.
Hardware-only checks: docs/HARDWARE_TESTS.md.

### Local portable build with Canon EDSDK

A Windows x64 build can embed the vaulted Canon 13.21.10 DLLs and unpack them automatically into the user's cache when the Canon driver is used. An SDK configured in Settings, `CANON_EDSDK_PATH`, or beside the app takes precedence. The built executable contains extractable Canon binaries; confirm Canon's licence permits its intended distribution. This does not verify camera operation.

On a trusted build machine with Go, Node, Ansible Vault, and the `ansible-library` checkout containing the encrypted files:

```sh
cd frontend && npm ci && npm run build && cd ..
./scripts/build-canon-portable.sh /path/to/ansible-library /secure/path/vault-password-file
```

The password file contains only the vault password. The script verifies the DLL hashes, creates `dist/ApexVoidPhotobooth-canon.exe`, and removes the temporary plaintext SDK files. The executable embeds FFmpeg too if the CI-prepared `internal/video/embedded/ffmpeg.exe.gz` and `FFMPEG-LICENSE.txt` are present; otherwise video needs a separate FFmpeg. This local cross-build does not add the CI Windows resources. Trusted master CI delivers one executable with both Canon DLLs and FFmpeg embedded. It validates the exact uploaded executable on a camera-free Windows runner; use the artifact only after the complete workflow succeeds. Pull-request jobs do not receive the vault password or publish the portable artifact. Camera operation still requires hardware validation.

### Gallery, export and trash

Gallery pages sessions and their media on demand; search, date (UTC), mode, status, simulated and operator-test filters are applied locally in SQLite. Open a session to export a new ZIP or move it to recoverable trash. The Trashed status filter provides Restore; permanent deletion remains an explicit Empty Trash action in Settings. Active sessions cannot be trashed. See [UX5_AUDIT.md](UX5_AUDIT.md) for recovery behavior and filesystem limitations.

### Offline support bundle

In Studio → Diagnostics, choose **Export support bundle** and select a local folder. The ZIP contains an allowlisted readiness report and counts of recognized log event categories, not raw logs, media, settings, folder paths, device names, PINs or secrets. It makes no network connection. Review any separately collected validation reports before sharing them; they are not included automatically. Current application logs rotate at approximately 1 MiB per file with four backups; local logs may still contain private paths or device names, so do not send them without review. Older daily logs are retained for up to 30 days and are not part of the ZIP.

### Back up and restore the complete library

1. Finish the active session and close ApexVoid. Confirm it has exited on every computer that can access the library. The SQLite database may have WAL files while open, so copying just the database is not a complete backup.
2. Copy the **entire** media-library folder shown in Studio → Settings, including `database`, `sessions`, `templates`, `trash`, and any SQLite WAL/SHM files. Copy `%APPDATA%\ApexVoidPhotobooth\settings.json` separately if operator preferences are needed. Store the backup in a protected location with enough free space.
3. To restore, keep the original and backup untouched; copy the complete backup to a new local folder. Start ApexVoid, select that folder as the custom library in Settings, then close and reopen the application. Check session counts, sample originals and compositions, and run Verify Library before relying on the restored copy.
4. Do not overwrite a live library or permanently delete the old copy until the restored library has been checked. Media and settings may contain private information; secure backup access. These instructions still require target Windows/filesystem validation.
