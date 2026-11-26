# Booth capture geometry and photo choice (BOOTH-01)

Hotfix for two defects reported after physical testing with a Canon EOS R100.

## 1. Root cause: the framing guide

`GuestShoot` looked the guide up with

```ts
frame.photoSlots?.find((slot) => slot.photo === s.slot)
```

`s.slot` is the **capture index** (0..7 when eight candidates are taken); `slot.photo` is a **destination photo index of the
frame**. They are different things. A six-photo frame therefore had a guide for captures 1–6 and none for 7 and 8; a
three-photo frame lost it from capture 4. The guide was also a different shape for different captures whenever the frame's
windows differed, and it ignored the window's border inset, so even when it was found it was not what the compositor cuts.

The photo-choice screen had the same confusion: a "frame slot" had to be selected first, then a photo assigned to it, with
automatic initial picks, swapping, and per-photo retake buttons.

## 2. The fix

### Capture guide (`frontend/src/features/booth/captureGeometry.ts`)
* The guide is derived from the **frame's photo windows only** (shape, border inset, focus point) and the camera
  (live-picture shape, field of view, mirroring). It never receives a capture index, so it is identical in PREVIEW,
  COUNTDOWN and CAPTURING and for every candidate.
* Window = slot rounded to pixels, less the frame's photo border: the same arithmetic as `Template.photoWindow` in Go.
  The crop is "cover, cut around the focus point" (a zero focus is the centre): `coverCrop` repeats
  `templates.CoverCrop` exactly, `windowCrop` is the same crop as fractions for the screen.
* Candidates are interchangeable, so several windows of different shapes give the **area every window shows**
  (conservative common safe area, `mode: "common"`); identical windows give that window's crop (`mode: "single"`). Repeated
  photo references and mixed shapes need no special case. Windows that share nothing, or unknown geometry, show the whole
  live picture (the same for every capture) instead of a wrong part.
* The frame is recalculated when the guest changes it before shooting; the live picture's shape is read from the stream and
  frozen once shooting starts.
* The preview is flipped on screen when mirroring is on; the saved photo is not. The guide is flipped with it, so the
  area on the guide is the area in the saved photo (matters for off-centre focus).
* The original JPEG is never cropped or overwritten; the destination crop is applied by the compositor.

### Pinned against the Go compositor
`Template.SlotCrops` / `CoverCrop` in `internal/templates/render.go` are used by `compose` itself. `TestSlotCropGolden`
writes the crop of every photo window of representative frames (each layout, every design on the 4-photo strip, border,
off-centre focus, repeated photos) for five camera sizes (6000×4000, 4000×3000, 1920×1080, 1280×720, portrait) to
`internal/templates/testdata/slotcrops.json`; `TestSlotCropMatchesComposedPixels` proves the crop is what `Compose` really
draws. `captureGeometry.test.ts` reads the same file and requires the TypeScript result to be identical (integer crop) or
within a pixel (on-screen fractions). If the compositor changes, the Go test fails until the file is regenerated
(`UPDATE_GOLDEN=1 go test ./internal/templates -run SlotCropGolden`), and the TypeScript test then fails until the guide
code agrees. EXIF orientation is applied by `media.Decode` before the crop, so the crop is in displayed orientation.

### Choosing photos (`photoSelection.ts`, `guest/PhotoSelectGrid.tsx`)
* Every candidate is one large touch target: tap to select, tap to unselect, numbered badge, clear border. Eight shots are a
  4×2 grid on landscape and 2×4 on portrait (`selectGridShape` handles other counts). The review thumbnails are not cropped.
* Starts empty for each session; never more than the frame needs, never a duplicate; Continue only when exactly the
  required count is selected; selection order is frame order; unselecting renumbers; an accidental double tap on one
  photo (< 320 ms) is ignored; Continue submits once.
* Selections are keyed by the capture's media id, so a retaken (Studio only) or vanished photo drops out instead of being
  submitted stale. A frame change keeps the valid choices (trimmed to the new count) and asks for the rest.
* No frame-slot buttons, no per-photo assignment, no automatic initial selection, no retake and no Start over on the guest
  review (Home still asks first, as UX-1 requires). Studio uses the same grid and keeps retake as an operator-only action.
* Selecting never calls the camera or the session API.

### Backend acceptance
`AcceptSelected` validates the exact count and distinct captures of this session (unchanged), and now:
* checks and enters SAVING in **one step** under the engine lock (before, two quick submits could both pass the check and
  compose twice; the same race let a retake start while saving);
* records the choice in **one transaction** (`Store.SelectPhotos`): selected captures become photos 0..n-1 in selection
  order, the others are marked `discarded`; **nothing is deleted yet**;
* deletes the unselected photos (`PruneDiscardedPhotos`, repeatable) only **after** the finished image was saved;
* if composition fails, restores them (`RestoreDiscardedPhotos`) so no photograph is lost;
* a crash between choosing and deleting is recovered at start-up (`Verify` gives discarded photos of unfinished sessions back).
Before, unselected originals were permanently deleted *before* composition, one by one, outside the reordering transaction.
Changing the frame during review now also changes the number of photos to choose (`Engine.SetFramePhotos`); before, the
interface asked for the new count while the engine still demanded the old one. Any frame that needs no more photos than
were captured is accepted without the old "use the first N" step.

## 3. Canon live view versus the final photograph (hardware-dependent)
The driver is unchanged. What is known and what is not:
* The guide assumes the live picture and the full-resolution JPEG show **the same field of view**. Canon does not guarantee
  that: the EVF stream (typically ~1024×680 or 960×640, 3:2) can be cropped or have a different position than the sensor
  JPEG (6000×4000 on the EOS R100), and some bodies change it with the shooting mode.
* The mapping lives in one place: `previewFovFor(driver, model)` returns the part of the still frame the live view shows
  (default: all of it) and `resolveCaptureGuide` maps the crop through it. A measured body adds an entry to `KNOWN_FOV`
  in `captureGeometry.ts`; no CSS is adjusted per camera. `fovMismatch(liveAspect, stillAspect)` flags a visible shape
  difference.
* **Calibration (needs the camera):** film a printed grid or ruler, note what lies on the guide's edges, take a photo, and
  compare with the window in the finished strip (HARDWARE_TESTS.md H3, H10). If they differ, measure the EVF's area in still
  coordinates and add it. The EDSDK properties `kEdsPropID_Evf_ImagePosition` and `kEdsPropID_Evf_CoordinateSystem` can
  report this at runtime; wiring them through the driver was deliberately **not** done in this hotfix (it would touch the
  working EDSDK capture path).
* Orientation: if a body delivers its JPEG rotated by EXIF, the crop is already in displayed orientation; the live stream
  has no EXIF, so a rotated live picture would be visible immediately.

## 4. Verification status
| What | Status |
|---|---|
| Crop arithmetic identical to the Go compositor (golden file, 5 camera sizes, borders, focus, repeats) | CI-tested |
| Guide identical for all eight captures of 1-, 3-, 4- and 6-photo frames; frame change; mirroring; mixed windows | CI-tested (jsdom) |
| Selection rules, submission order and exactly-once, language switch, session reset, frame change | CI-tested |
| Backend validation, ordering, double submit, composition failure, crash recovery, frame change in review | CI-tested (Go, race detector on capture/sessions/templates) |
| Layout at 1920×1080, 1366×768, 1080×1920, EN and VI, 2×6 and 4×6 | browser-harness screenshots (`docs/img/booth01/`) with the **simulated** camera |
| **Canon EOS R100 live view versus JPEG field of view; real WebView2; touch; Windows scaling** | **UNVERIFIED — not tested by the project** |
