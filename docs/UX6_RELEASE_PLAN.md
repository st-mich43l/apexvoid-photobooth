# Windows release plan

Status: **not production-ready**. Physical hardware acceptance, WebView2 deployment, signing and the remaining UX-6 implementation are release blockers. Do not label CI artifacts as production releases.

The standard Windows CI artifact contains the application with pinned FFmpeg, not Canon binaries. It is an unsigned test build; `BUILD-MANIFEST.json` records commit, version, UTC build time, architecture, FFmpeg version and executable SHA-256, and `SHA256SUMS.txt` lists packaged file checksums. Check the hash on the target machine before testing. These records identify a build; they do not prove bit-for-bit reproducibility or publisher identity.

Trusted master CI also produces a separate Canon-enabled artifact containing extractable EDSDK DLLs. Pull-request workflows do not receive the vault password. The normal portable artifact uses an SDK installed separately.

An eventual signed release must use a trusted code-signing identity held in an approved signing service or protected CI secret; never commit a certificate private key or password. Verify signatures with Windows `Get-AuthenticodeSignature` and publish verified checksums over a trusted channel. Do not disable SmartScreen or other Windows security to distribute unsigned executables.

Before release, complete the hardware and WebView2 matrices in `UX6_WINDOWS_ACCEPTANCE.md`, run endurance and crash-recovery scenarios, qualify offline WebView2 installation, establish reproducible clean builds, review third-party licenses, test backup/restore and upgrade on copies, and publish versioned release notes and checksums. A minimal per-user installer definition is included; its offline prerequisites, upgrade and non-destructive uninstall behavior still require qualification on target machines. Portable distribution remains available.
