# UX-4 printing audit

The existing path is capture → composition/export → print-ready media → the Go page placement engine → GDI/Windows spooler → persisted job and queue polling → explicit reprint. Preview and final submission both use the same Go placement engine; the spooler cannot establish that paper was delivered.

## Findings and changes

* Reprinting checked the file after submitting, used a fixed 300 DPI, and counted only the newest 500 jobs. Jobs now persist image DPI; older JPEGs can supply a valid JFIF density. Reprints preflight the original record, readable image header, installed and available printer, and current paper capabilities. An indexed database count enforces the limit across all history. Legacy files without trustworthy DPI are refused rather than guessed.
* Submission is persisted before calling the native driver. If the spooler may have accepted a job but a later native call or database update fails, the result is unknown, the request ID remains reserved, and identical new requests are blocked. A GDI error after StartDoc carries the native job ID. A failed queued job must disappear from the queue before reprinting. Unknown reprints require operator investigation and are not automatically repeated.
* A paused Windows queue was reported ready. It now reports paused, with a prompt to resume the queue. Offline, paper and driver errors remain visible.
* Printer capability and preview requests could arrive out of order. The UI discards obsolete capability responses, clears invalid paper and old previews, and only enables Print for the currently displayed selection. The Go preview reports unprintable margins as well as image margins and crop/scale.
* Booth guests can send the finished result once. Additional copies require an operator reprint with confirmation and the normal limit. A pending guest action cannot be sent again by a double tap. Operator defaults cover printer, paper, orientation, placement, copies and optional two-up strips; incompatible paper is not silently selected on the Printer page.
* The calibration page remains a deliberate confirmed action. An operator can mark the particular test page as inspected; this is a temporary UI acknowledgement, not a claim about later prints.

## Verification and limits

Automated tests use fake drivers and synthetic media; see the PR for command results. Screenshots in `docs/img/ux4` use a simulated camera and fake printer. Physical output, native GDI behavior and Windows file-port printing remain **UNVERIFIED** until the Windows CI or a person runs them. The code does not automatically restart the spooler, clear jobs, or claim physical delivery.

The request lock is process-local; simultaneous independent application processes sharing one library are not covered by a database uniqueness constraint. An unknown job is deliberately blocked rather than resolved automatically; the operator must investigate the queue, and a future explicit reconciliation workflow is still needed. The existing two-up export constructs a sheet and encodes it as JPEG; its pixel placement tests do not prove a particular printer's trim or margins. Some driver calls can block inside the operating system despite a cancelled context. These limitations require hardware and operational validation before calling the workflow production verified.

## Physical printer acceptance — all UNVERIFIED

Use an installed 4×6 printer, correct media, an operator, and a throw-away session. Record printer/driver, paper and settings; do not include personal photos in the repository.

1. Print the calibration page intentionally. Inspect orientation, 25 mm squares, grid, margins, centering, scale and whether any claimed borderless mode actually reaches the edge.
2. Print 2×6 strips two-up on 4×6 at 300 DPI and 600 DPI. Measure both strips and the sheet, check order, centering and cropping, and compare with the preview.
3. Print a 4×6 composition in portrait and landscape, and a supported custom or 5×7 sheet. Verify the selected paper and unprintable margins.
4. Disconnect and reconnect the printer, pause/resume the queue, and test paper-out and jam status. Confirm the UI gives a safe recovery path and existing photos and jobs remain intact.
5. Double-tap Print; confirm a single job. Request a second copy from the operator, verify confirmation and limits, and check that an unknown job is not resent.
6. Perform at least 30 repeated print operations, including a restart and a reprint, recording errors, memory behavior and spooler status. Manually verify each physical page.
