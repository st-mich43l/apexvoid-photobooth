# Printing (Windows spooler + GDI)

## Pipeline
```
captured images → selected layout → rendered print-ready image → printer driver (GDI) → Windows spooler
```
* A composed strip / 2-up sheet / earlier print is used **as the rendered file it already is** (its pixel size is checked
  against the chosen layout).
* A **single photo is never printed as the raw original**: it is rendered through a one-photo layout (4×6 or 5×7,
  portrait/landscape chosen to match the photo, the *edited* version when the photo was edited) and saved as an
  export (`kind: print`). Originals are never touched.
* Reprints reuse the same rendered file. Originals and rendered exports are both kept.

## Windows implementation (`internal/printing/winspool_windows.go`)
Plain syscalls to `winspool.drv` / `gdi32.dll`; no custom or kernel driver, no raw JPEG sent to the printer.
* `EnumPrintersW` (level 2) → printers, status/attributes (offline, paper out, jam, door open …), `GetDefaultPrinterW`.
* `DeviceCapabilitiesW` → the printer's own paper sizes (`DC_PAPERS/PAPERNAMES/PAPERSIZE`), orientations, copies,
  colour, resolutions.
* `DocumentPropertiesW` → the driver's DEVMODE, to which paper (driver id, or custom size), orientation are applied and
  which the driver validates.
* `CreateDCW("WINSPOOL")` + `GetDeviceCaps` → **real page geometry**: physical sheet, printable area, unprintable offsets,
  DPI. The image is placed by `PlanPage` (fit / fill / true size, automatic 90° turn, centring on the *sheet*).
* `StartDoc` (returns the **native spooler job id**) → `StartPage` → `StretchDIBits` (32-bit top-down DIB, HALFTONE) →
  `EndPage` per copy → `EndDoc`. Several copies are pages of one document (more reliable than `dmCopies`).
* `GetJobW` (level 2) → job status flags → `queued / printing / completed / removed / failed`.
  A job that disappears from the queue is **"removed"**, a job the spooler marks PRINTED/COMPLETE is **"completed"**;
  both texts say that this is *not proof that paper came out*.

## Safety
* **No duplicates:** each user action carries a request id (the same id never prints twice); an identical print inside
  15 s is refused (`Reprint` is the deliberate way to print again); simultaneous identical submissions are serialised.
* **Reprint** needs an explicit confirmation, is refused while the job is still in the queue, is bounded to 3 per original
  and writes a new history row linked to the original.
* **History in SQLite** (`print_jobs`). A job is recorded *before* it is sent; after a crash it shows as **unknown**
  ("check the Windows print queue") — never as printed. Jobs still in the spooler after a restart are tracked again.
* An offline / errored printer is refused before anything is sent, with the printer's own explanation.
* **Test page:** 10 mm grid, 25 mm corner squares, paper size — prints at true size so margins can be measured.

## Formats
4×6 in photo · 2×6 in strip · two 2×6 strips on one 4×6 sheet (cut apart) · 5×7 in · any paper the driver reports
(including custom sizes via `DMPAPER_USER`). Printable area ≠ physical paper: the preview shows the printer's
unprintable border and the exact image position, scale and margins.

## What is verified
| | status |
|---|---|
| Placement maths, preview, spooler-state mapping, job lifecycle, duplicate protection, restart recovery | unit tests (CI) |
| Struct layouts passed to the Windows API (`PRINTER_INFO_2W`, `JOB_INFO_2W`, `DOCINFOW`, `BITMAPINFOHEADER`) | asserted in a Windows test on the CI runner |
| Printer enumeration / capabilities / geometry on a real Windows spooler | CI Windows runner (software printers only) |
| Real submission through StartDoc/StretchDIBits/EndDoc to a **file-port** printer (Microsoft Print To PDF / XPS) and job tracking to "left the queue" | CI step *Windows printing through the real spooler* — see the run log; **if the runner has no such driver this is reported as unverified** |
| **Physical paper output on a real printer (4×6 dye-sub, inkjet, laser)** | **NOT verified** — hardware checklist D1–D8 |
| Print DPI/colour fidelity, borderless behaviour of specific drivers | **NOT verified** |

`ApexVoidPhotobooth.exe --print-selftest report.json "<printer>" ["<paper>"]` runs the sequence headlessly
(without a printer name it only enumerates) and writes a JSON report with every spooler state seen.

## UX-4 operator checks

The Printer page saves the default paper, orientation, placement, copy count and strip two-up choice. Select a printer and a paper that it currently reports; review the preview's scale, crop and grey unprintable region before printing. A calibration page is sent only after confirmation; marking that one page inspected does not confirm later delivery. The guest result allows one print action and directs additional copies to the operator's confirmed reprint flow.

Reprints retain the stored image DPI and require the original file, job, printer and paper to remain available. Legacy jobs without stored DPI use a valid JPEG density when present; otherwise recompose the image. If submission is **unknown**, check the Windows queue and output tray; do not submit another copy until the uncertainty is resolved. The application never treats a job leaving the queue as proof of physical delivery. See [Hardware tests](HARDWARE_TESTS.md) for physical acceptance checks.

Physical paper delivery, margins and endurance must be recorded on the target equipment using [Hardware tests](HARDWARE_TESTS.md). A spooler/file-port test does not demonstrate paper delivery. Investigate an unknown print job before any explicit reprint; never assume it failed or silently send it again.
