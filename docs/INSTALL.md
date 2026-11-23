# Install, run, test

## Run (Windows 10/11 x64)
1. Download the CI artifact `ApexVoidPhotobooth-windows-x64` (the only artifact CI produces) and unzip anywhere. `ApexVoidPhotobooth.exe` is a single file with a pinned LGPL FFmpeg built in.
2. Requires the **WebView2 runtime** (already present on Windows 11 and updated Windows 10; otherwise install the
   Evergreen runtime from Microsoft).
3. Double-click `ApexVoidPhotobooth.exe`. No server, Docker, database or network is needed.
4. Webcams and video work out of the box: FFmpeg is unpacked from the exe on first use (see docs/THIRD-PARTY.md). To use your own FFmpeg, put `ffmpeg.exe` next to the app or set its path in Settings; that copy always wins over the built-in one.
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

### Local portable build with Canon EDSDK

A Windows x64 build can embed the vaulted Canon 13.21.10 DLLs and unpack them automatically into the user's cache when the Canon driver is used. An SDK configured in Settings, `CANON_EDSDK_PATH`, or beside the app takes precedence. The built executable contains extractable Canon binaries; confirm Canon's licence permits its intended distribution. This does not verify camera operation.

On a trusted build machine with Go, Node, Ansible Vault, and the `ansible-library` checkout containing the encrypted files:

```sh
cd frontend && npm ci && npm run build && cd ..
./scripts/build-canon-portable.sh /path/to/ansible-library /secure/path/vault-password-file
```

The password file contains only the vault password. The script verifies the DLL hashes, creates `dist/ApexVoidPhotobooth-canon.exe`, and removes the temporary plaintext SDK files. The executable embeds FFmpeg too if the CI-prepared `internal/video/embedded/ffmpeg.exe.gz` and `FFMPEG-LICENSE.txt` are present; otherwise video needs a separate FFmpeg. This local cross-build does not add the CI Windows resources. Trusted CI runs on `master` also produce `ApexVoidPhotobooth-windows-x64-canon`: configure the repository Actions secret `ANSIBLE_VAULT_PASSWORD` with the vault password. The job retrieves the encrypted SDK from `ansible-library` master, verifies the pinned DLL hashes, and tests SDK initialization on Windows. Pull-request jobs do not decrypt the SDK. GitHub Actions artifacts contain extractable Canon binaries; restrict distribution as required by the Canon licence. Camera operation still requires hardware validation.

### Gallery, export and trash

Gallery pages sessions and their media on demand; search, date (UTC), mode, status, simulated and operator-test filters are applied locally in SQLite. Open a session to export a new ZIP or move it to recoverable trash. The Trashed status filter provides Restore; permanent deletion remains an explicit Empty Trash action in Settings. Active sessions cannot be trashed. See [UX5_AUDIT.md](UX5_AUDIT.md) for recovery behavior and filesystem limitations.

### Offline support bundle

In Studio → Diagnostics, choose **Export support bundle** and select a local folder. The ZIP contains an allowlisted readiness report and counts of recognized log event categories, not raw logs, media, settings, folder paths, device names, PINs or secrets. It makes no network connection. Review any separately collected validation reports before sharing them; they are not included automatically. Current application logs rotate at approximately 1 MiB per file with four backups; local logs may still contain private paths or device names, so do not send them without review. Older daily logs are retained for up to 30 days and are not part of the ZIP.
