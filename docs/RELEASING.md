# Releasing

Official Windows releases are published by this branch's CI to persistent storage on the apexvoid VPS and offered at
**https://photobooth.apexvoid.net/download/**. The website reads the release manifest at runtime, so publishing needs
no website change. Storage, publisher and operator procedures (verify, retry, rollback, withdraw, restore) are in
ansible-library: `docs/photobooth-release-distribution.md`.

Only `master` publishes. `community` and every other branch, pull requests and forks never do: the release jobs are
limited to `refs/heads/master`, the `photobooth-release` environment only accepts `master`, and pull requests never
receive secrets.

## What makes a run publish

A push to `master` (or a manual run of CI on `master`) publishes when **all** of these hold:

1. every validation job passed: `go`, `frontend`, `windows`, `canon-portable`, `canon-windows-smoke`;
2. `release/publication.json` has `"enabled": true` and every gate required by its `variant` is approved (below);
3. `docs/releases/v<version>.md` exists — the approved release notes are the release intent;
4. the version is not published yet and is newer than every retained stable version. A stale run is skipped by CI
   and rejected again by the VPS publisher. A push of an already published version is skipped. A **manual** run of
   the highest published version is an explicit retry: identical files are a no-op, a different file fails with a
   conflict. Expired versions remain tombstoned privately and cannot be republished.

Otherwise the `Release decision` job succeeds with the reason (and the open gates) in its job summary, and nothing
is published.

## Version

`frontend/package.json` is the single version authority (SemVer, no build metadata). `build/winres/winres.json` must
match it — CI's `go` job fails otherwise (`python3 scripts/release/release.py check-version`). The public version is
`<version>`, the tag `v<version>`, the file `ApexVoidPhotobooth-v<version>-windows-x64.exe`. A version with a
pre-release part (`1.0.0-rc.1`) is not published in the public catalog. Older release builds remain available as CI
artifacts for development and testing. Stable public releases are strictly increasing; intentional rollback is an
operator action limited to retained versions.

## Publication gates (`release/publication.json`)

| Gate                  | Required for       | Decision                                                                    |
| --------------------- | ------------------ | --------------------------------------------------------------------------- |
| `applicationLicense`  | every release      | approved 2026-11-25 — MIT License (`LICENSE`, shipped as `LICENSE.txt`)       |
| `ffmpegLgpl`          | every release      | approved 2026-11-25 — LGPL text in the notices, source pointers in `SOURCE.md` |
| `canonRedistribution` | `variant: "canon"` | approved 2026-11-25 — owner confirmed Canon's authorization                   |
| `codeSigning`         | —                  | `signed: false` — no certificate; the site says "not code-signed"           |

`variant` chooses the executable: `standard` is the `windows` job's build (FFmpeg built in, **no Canon SDK**; Canon
cameras need the SDK configured separately), `canon` is the `canon-portable` build with the SDK embedded, validated by
`canon-windows-smoke`. Setting `enabled: true` while a required gate is unapproved fails CI; claiming
`signed: true` always fails (this pipeline cannot sign). Only the owner approves a gate, with the decision written in
its `note`.

## Release a version

1. On a branch: bump `frontend/package.json` and `build/winres/winres.json`, add `docs/releases/v<version>.md`
   (what changed, known limitations, the hardware status — no claims beyond the evidence).
2. Open a pull request into `master`. Its CI validates everything but never publishes.
3. Merge. The master run builds and validates once, then `publish`:
   - takes the exact executable the Windows jobs validated (no rebuild) and checks its SHA-256 against the build;
   - assembles `RELEASE-NOTES.md`, `SHA256SUMS`, `THIRD-PARTY-NOTICES.txt` (docs/THIRD-PARTY.md plus the
     executable's own `--notices`), `SOURCE.md`, `LICENSE.txt`, and `release.json`; validates them with the publisher's validator;
   - runs ansible-library's `publish-photobooth-release.yml` (pinned commit, pinned VPS host key);
   - downloads the file from the public URL and checks it against `SHA256SUMS`.
4. The job summary lists the version, SHA-256, size and links.

**Retry** a failed publication: Actions → the failed run → **Re-run failed jobs**. It reuses that run's artifacts
(kept 30 days). **Never** rebuild a published version: bump the version instead.

## Verify a published release

```powershell
Get-FileHash .\ApexVoidPhotobooth-v<version>-windows-x64.exe -Algorithm SHA256
```

Compare with `SHA256SUMS` on the download page. The executable is not code-signed; SmartScreen may warn on first run.
