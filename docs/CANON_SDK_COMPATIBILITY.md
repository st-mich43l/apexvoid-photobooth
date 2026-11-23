# Canon EDSDK 13.21.10 compatibility assessment

Assessment on Linux, 2026-11-23. **Real Windows DLL loading and EOS R50 operation remain UNVERIFIED.** Static inspection and fake-SDK tests cannot establish hardware reliability or complete CAM-1.

## Package inspected

The locally supplied `EDSDK132110CD(13.21.10)` contains `Document/ReleaseNote.txt`, `Document/EDSDK_API_EN.pdf`, Windows headers in `Windows/EDSDK/Header`, x86 binaries and import library in `Windows/EDSDK/{Dll,Library}`, x64 binaries and import library in `Windows/EDSDK_64/{Dll,Library}`, and C++/C# samples in `Windows/sample`. Linux and macOS packages are also present. The release note identifies version 13.21.10, dated 08/03/2026. The API reference lists EOS R50 under supported camera control, distinct from R50V; it says the Windows SDK was checked on Windows 11, 32- and 64-bit. Windows 10 operation is not established by that statement.

`file` identifies both x64 DLLs as PE32+ AMD64 and both files in `EDSDK/Dll` as x86 PE32. Static inspection of x64 `EDSDK.dll` found all 25 exports used by this adapter. Its import table lists SETUPAPI, WS2_32, KERNEL32, USER32, ADVAPI32, ole32, SHELL32 and VERSION; `EdsImage.dll` imports KERNEL32 and ADVAPI32. This does not prove dynamically loaded dependencies are available or that the DLL can initialise on Windows. Keep Canon's complete x64 Dll folder together.

The package's readme includes third-party IJG notices; it is not evidence of permission to redistribute Canon's SDK. The Canon redistribution licence was not found in the inspected document files and must be reviewed separately. No SDK files are included in this repository or release by this change.

## Binding comparison

`EDSDK_HEADERS=<Windows/EDSDK/Header> go test ./internal/camera/edsdk -run Headers -v` passed against the supplied headers. The verifier covers the constants listed in `headers_test.go`, not every SDK declaration. The headers declare 8-byte packing, and the fields of `EdsDeviceInfo`, `EdsDirectoryItemInfo`, and `EdsCapacity` match the Go definitions; Go size tests check 520, 288 and 12 bytes respectively. Object and state callback argument order and 64-bit stream sizes were compared with the headers. This is a source-level comparison, not a Windows ABI or callback execution test.

The existing adapter serialises calls on a locked OS thread, pumps SDK events and Windows messages, uses host save destination and capacity, and handles asynchronous transfer events. Canon's sample uses a fresh memory stream and EVF reference per frame; the adapter now follows that lifecycle. COM apartment initialisation errors are now reported. These changes still require Windows and camera testing. SDK discovery now accepts the configured folder, `CANON_EDSDK_PATH`, and application-relative paths, in that order. A package root containing `Windows/EDSDK_64/Dll` is accepted.

## Remaining limitations and validation

* Camera serial identity, battery, shooting mode and capacity are not established by this assessment. Do not infer them from SDK load success.
* A transfer delayed beyond the finite grace period can still arrive during a later capture window; the SDK event provides no proven shot correlation in this implementation. Cancellation and interrupted transfers need particular hardware scrutiny. Do not claim guaranteed shot association.
* Actual camera discovery, session ownership, frame rate, shutter, JPEG size/EXIF, sequential captures, reconnect, resource usage and shutdown are **UNVERIFIED**. Follow `HARDWARE_TESTS.md` on Windows with the R50; preserve its report and evidence outside version control.
* Camera-native movie recording remains unimplemented and unverified. UVC video is separate.
