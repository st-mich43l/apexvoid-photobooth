# UX7 localization audit

Status: **in progress**. This first change establishes independent language preferences; it does not claim complete localization or Windows/hardware acceptance.

## Baseline

The work starts at `4e2c63e1ad3e5b6c5312343ce41c1c7340761461`, the latest `master` at audit time. There were no later commits. PR #41 introduced the language setting and release work; #46–#48 extended translated operator, device, gallery and frame-builder screens. PR #50 fixed frame-slot preview cropping, #51 produced the single portable artifact, #52 reduced the sample catalog and #53 introduced floating navigation. These changes remain in the base.

The existing dictionary uses typed English keys, a Vietnamese record, named interpolation and an English fallback. Settings use JSON and the service's targeted `PatchSettings`; capture and booth share state and an idempotent session gate. English is the legacy operator default. Guest screens previously followed that same setting. UX6 records English from runtime device and validation messages, and does not claim Windows display or physical-device acceptance.

Baseline frontend tests: 318/319 passed on one full run; Gallery's asynchronous thumbnail test failed and is tracked as intermittent. A previous run on the same source passed 319/319. Baseline Go internal packages were cached; Wails' Linux desktop target could not build in this environment because pkg-config and the GTK dependencies are absent. Go integration tests require loopback access.

## Decisions and implemented scope

- The existing typed dictionary remains authoritative. Tests check language-key parity, duplicate keys across English domains, nonempty values and interpolation parity; named placeholders also accept camelCase and underscores.
- Operator selection is applied without remounting the app and persisted as a `language` patch. Failed writes revert the selection. Settings edits remain independent.
- `guestLanguage` is a validated persisted default, defaulting to English for old configurations. The welcome selection lives only in Booth's React state and is discarded when returning to welcome after a session. The selector is unavailable during the capture flow. A scoped locale supplies guest frame selection, capture, printing and dialogs without modifying the operator preference.
- Category names are translated for presentation only. User data and stored category identifiers are unchanged. Gallery and printer dates use explicit `en-US` or `vi-VN` formatting. The Canon SDK settings description now reflects trusted portable packaging.
- Wails v3 `application/bindings.go` constructs call errors from `errors.Join(errorOutputs...).Error()`. The existing `UserError` sends newline-separated message/action text; returning a Go struct alone will not produce a structured JS error. No incompatible error serialization change has been made in this pass.

## Open work before UX7 can be called complete

- Audit and translate all remaining application-owned strings, particularly Studio photo flow, `ui`, frame builder metadata, notifications, fullscreen and mode-preparation messages, diagnostics and backend readiness and validation text. Guest exit progress, PIN errors, print and recovery details still contain English. Some current host hints contain technical backend text and require a safe structured replacement.
- Implement and test stable error and recovery codes across the Wails boundary without matching arbitrary English messages; preserve diagnostic details separately. Safe-exit now distinguishes window-restoration failures by type. This first pass avoids message matching for guest print failures and conservatively treats an exception as an unknown submission.
- Complete locale-aware number formatting and audit singular/plural usage. Existing parity checks do not prove translation quality or that every screen uses the dictionary.
- Verify guest cancellation, operational guards, Settings synchronization and simultaneous settings events beyond the cases currently tested. Validate keyboard, focus, diacritics, long labels and scroll at all requested display sizes and Windows scaling factors.
- Browser-harness captures of Studio and Welcome were made in both languages at 1920×1080, 1080×1920 and 1366×768; no document-level horizontal overflow was observed. These captures use the harness camera stand-in and do not cover every screen or Windows display scaling. The trusted Canon portable artifact is built only on master with repository secrets; PR CI cannot exercise that job. Record actual CI results separately and do not interpret headless CI as physical acceptance.
- Complete target Windows/WebView2 and real camera/printer/touchscreen tests in `UX6_WINDOWS_ACCEPTANCE.md`; third-party device names and diagnostics may remain in their original language.
