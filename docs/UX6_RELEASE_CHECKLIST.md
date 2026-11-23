# UX-6 release checklist

**Production readiness: BLOCKED.** PASS requires recorded evidence; missing evidence is UNVERIFIED, not PASS.

- [ ] P0 findings in `UX6_AUDIT.md` resolved and reviewed.
- [ ] Full regression and Windows CI checks pass on the candidate commit; reproducibility investigated and dependency hashes verified.
- [ ] Windows 10/11 WebView2 desktop and touchscreen/scaling journey passes on target machines.
- [ ] Physical R50, webcam and printer acceptance and endurance results recorded in `UX6_HARDWARE_RESULTS.md`.
- [ ] Startup/shutdown, exclusive ownership, filesystem matrix, backup/restore and interruption/power-loss recovery qualified.
- [ ] English and Vietnamese interface and Vietnamese frame captions verified on target displays.
- [ ] Offline WebView2 setup, portable and installer upgrades/uninstall tested without deleting media or settings.
- [ ] Offline diagnostics privacy reviewed and support recovery instructions verified.
- [ ] Trusted signing identity in place and signature verified; unsigned builds remain labeled test builds.
- [ ] Candidate version, source commit, artifact manifest, full checksums, dependency notices, changelog and release notes recorded.
