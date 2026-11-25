# UX-6 hardware results

No target kiosk hardware was available for this implementation. All scenarios below are **UNVERIFIED**, not failures and not passes. Use `UX6_WINDOWS_ACCEPTANCE.md` and attach sanitized evidence separately.

| Scenario | Status | Required evidence |
|---|---|---|
| Canon R50: JPEG, RAW+JPEG, RAW-only, retake, delayed transfer and USB recovery | UNVERIFIED | SDK/camera/firmware/Windows versions, dimensions, EXIF, durations, session counts, disconnect observations |
| UVC: integrated/external, switching, recording/audio and recovery | UNVERIFIED | device/driver, modes, sync observations, failure counts |
| Physical printing: sizing, 300/600 DPI, recovery and 30 jobs | UNVERIFIED | printer/driver, paper measurements, actual sheets and failure counts |
| Windows 10/11 WebView2 touchscreen journey and scaling | UNVERIFIED | Windows/WebView2 versions, resolution/scaling, screenshots and observations |
| Filesystems, low disk, interruption and power loss | UNVERIFIED | filesystem/storage configuration, recovery observations and intact-original counts |
| Eight-hour soak and repeat sessions | UNVERIFIED | duration, memory/handles/CPU, failures, recovery |

Software evidence: UX-5 CI run #274 exercised Windows GDI/spooler against a Microsoft file-port printer and produced a PDF. It did not print on physical paper or test an R50. The UX-6 runtime synthetic startup profile and lock tests are described in `UX6_AUDIT.md`. CI on a Windows runner is not a substitute for target kiosk acceptance.

## Simulated endurance (software only)

On the Linux development environment, `UX6_ENDURANCE=1 go test ./internal/capture -run '^TestSimulatedEndurance100Sessions$' -count=1 -v` completed 100 one-photo sessions in approximately 1.84 seconds with the mock camera (320×240 images and shortened countdown). Each session reached completed state and its synthetic original existed on disk. One run reported Go heap allocation of 419,120 bytes before and 464,304 bytes after forced GC, with four goroutines at the end. This is not a CPU or operating-system handle measurement, an eight-hour soak, a Windows benchmark, a four-photo endurance run, or physical camera/printer acceptance. CI repeats this software-only check.
