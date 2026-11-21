# Install, run, test

## Run (Windows 10/11 x64)
1. Download the CI artifact `ApexVoidPhotobooth-windows-x64-portable` and unzip anywhere.
2. Requires the **WebView2 runtime** (already present on Windows 11 and updated Windows 10; otherwise install the
   Evergreen runtime from Microsoft).
3. Double-click `ApexVoidPhotobooth.exe`. No server, Docker, database or network is needed.
4. For webcams/video: put `ffmpeg.exe` next to the app (or use the `…-with-ffmpeg` artifact, an LGPL build).
5. For the Canon R50: follow docs/CAMERAS.md (SDK DLLs are not included).

Data: `%LOCALAPPDATA%\ApexVoidPhotobooth` (database, sessions, templates, logs, trash); settings in
`%APPDATA%\ApexVoidPhotobooth\settings.json`. Change the media folder in Settings (restart to apply).
The exe is **unsigned**: SmartScreen may warn on first run.

Without a camera: Settings → enable **Simulated camera** to try the workflow (everything is labelled SIMULATED).

## Build
```
cd frontend && npm ci && npm run build          # writes cmd/photobooth/frontend_dist
go test ./...                                    # (the cmd package needs WebKit on Linux; use $(go list ./... | grep -v /cmd/))
GOOS=windows GOARCH=amd64 go build -ldflags "-H windowsgui -s -w" -o ApexVoidPhotobooth.exe ./cmd/photobooth
```
The CI additionally embeds icon/manifest/version resources with go-winres.

Offline self-test of the whole stack (no window): `ApexVoidPhotobooth.exe --selftest report.json`.

## Tests
Go: unit, adapter contract (mock, UVC with synthetic source, Canon with a fake SDK), state machine, persistence,
crash recovery, disconnect/reconnect, repeat sessions, loopback server. Frontend: typecheck, unit tests, build.
Hardware-only checks: docs/HARDWARE_TESTS.md.
