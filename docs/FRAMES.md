# Frames

A **frame** is what a guest chooses before any photo is taken. It is a *layout* (where the photos go) plus a
*design* (colours, borders, artwork, text). Choosing a frame also decides how many photos the session takes.

## Data model (`internal/templates`)

`Template` is the one model. Its JSON is flat and compatible with the earlier layout files; in Go it is split
into two embedded parts so one layout can carry many designs:

| Part | Fields | Purpose |
|---|---|---|
| `Geometry` | `widthIn`, `heightIn`, `dpi`, `slots[]` | physical size and photo slots (all inches from the top-left) |
| `Style` | `background`, `borderIn`, `borderColor`, `cornerIn`, `accent`, `textColor`, `backgroundImage`, `overlay`, `layers[]`, `variants[]` | how it looks |
| catalogue | `id`, `name`, `category`, `description`, `order`, `revision`, `enabled`, `builtin`, `legacy`, `captionEnabled`, `design`, `layoutId` | what the Booth lists |

* **Variants** are colour options of a design (they override background / border / accent / text colours).
* **Layers** are `rect`, `ellipse`, `shape` (heart, star, sparkle, flower, cloud, triangle, …), `pattern`
  (a shape repeated on a grid or scattered, e.g. confetti or film sprocket holes), `text` and `image`. A layer can sit
  *under* the photos. Text may contain `{caption}`, `{date}` or `{caption|date|literal}` (first non-empty).
* Everything is in inches, so a frame renders the same at 40 DPI (thumbnail) and 300 DPI (print).

## Built-in frames

17 original designs — Classic (White Minimal, Black Minimal, Clean Border, Soft Gray), Korean Pastel (Rose Pink,
Lavender, Cream Beige, Sky Blue), Retro (Film Strip, Vintage Camera, Polaroid Inspired, Monochrome Film), Special
(Floral, Birthday, Couple, Seasonal) and Plain (no decoration) — each with 3–4 colour options, generated onto 9 layouts:

| Layout | Photos | Output at 300 DPI |
|---|---:|---|
| `strip4` / `strip3` / `strip2` | 4 / 3 / 2 | 600×1800 (2×6) |
| `twoup4` | 4 (two identical strips) | 1200×1800 (4×6) |
| `sheet6` / `sheet4` / `sheet2` | 6 / 4 / 2 | 1200×1800 (4×6) |
| `p57` | 4 | 1500×2100 (5×7) |
| `single` | 1 | 1200×1800 (4×6 portrait) |

Frame ids are `<design>.<layout>` (for example `rose-pink.strip4`). All artwork is drawn by the renderer from vector
shapes and the embedded Go fonts (BSD licence): there is no third-party artwork. The earlier layouts (`strip-2x6-4`, …)
stay resolvable as hidden *legacy* frames so old sessions and settings still work.

By default the 4-photo 2×6 strip of every design and every Plain layout are offered in the Booth; the rest are
off until the operator switches them on (**Studio → Frames**). The switches are stored in `templates/frame-state.json`.

## One renderer

`templates.Compose` is the only composition code. Order: canvas colour → background artwork → layers marked *under*
→ photo slots (cover-crop around the focus point, border, rounded corners) → transparent overlay PNG → all other
layers in list order (decorations, images, text). The Frame-selection thumbnails, the large preview, the
session result, the gallery and the print job all come from this function; the UI never draws a frame itself. The
browser asks for `/frame/<id>/<variant>?w=<px>` (loopback + per-run token) and receives a JPEG rendered with
generated stand-in portraits — never camera images. `TestPreviewMatchesFinalRenderAtAnyDPI` checks that a low-DPI
preview equals the 300-DPI output scaled down.

Text uses the embedded Go fonts, which cover Latin scripts; characters outside them show the font's "missing glyph" box.

## Per-session frame and immutable snapshots

`StartShooting(frameID, variantID, caption)` validates the frame (exists, enabled, valid, 1–12 photos), takes the shot
count from it and — before the first shot — freezes it into the session:

```
sessions/<id>/frames/001/frame.json     the resolved frame (colour option applied), caption, date, revision
sessions/<id>/frames/001/assets/…       a copy of every image the frame uses
```

The session row records `frame_id`, `frame_name`, `variant_id`, `frame_rev`, `caption` and the snapshot folder; every
composed image records the frame that made it. Editing, disabling or deleting the frame later never changes how the
session renders (`TestSessionKeepsItsFrameWhenTheFrameIsLaterChangedOrDeleted`). Sessions from earlier releases have no
snapshot; they resolve their stored layout id. There is no shared "current template" state any more: concurrent
sessions, previews and prints each read their own snapshot.

### Changing the design after capture

`SetSessionFrame` / `ComposeSession` switch a session whose photos exist to another frame **without retaking**. The new
frame must use the number of photos that were taken. A frame that needs fewer is used only when the caller explicitly
says "use the first N"; a frame that needs more is refused with an explanation (the UI then offers "Start over with this
frame"). The original photos are never touched, the earlier compositions stay in the library, and a refused change
leaves the session's frame as it was.

A composition that cannot be made (damaged snapshot, disk error) never loses photos: the session completes with a
warning and the guest or operator can compose again.

## Schema

Database v3 adds the frame columns to `sessions` and `media` (empty for older rows; tested upgrade from v2).

## Frame Builder (Studio → Frames)

*Create frame…* (from any built-in layout), *Duplicate* (any frame), *Edit* / *Delete* (custom frames only).

* Three panes: add elements and layer list (left) · canvas (centre) · properties of the selected photo slot, layer or the
  frame (right). Undo / redo (Ctrl+Z / Ctrl+Y), duplicate (Ctrl+D), Delete, arrow-key nudging (Shift = 0.1 in), zoom, grid,
  snapping to a 0.05 in grid and to canvas / sibling edges and centres, layer order, locking, hiding, "behind the photos".
* The picture on the canvas is rendered by the Go renderer from the draft (`PreviewDraft`, debounced); the boxes and
  handles are drawn on top in percent of the canvas, so screen pixels convert to print inches exactly at any zoom
  (`builder/geometry.ts`, unit-tested).
* Editable: name, category, description, colours, photo border width/colour, corner radius, resolution, photo slots
  (position, size, photo number, crop focus), shapes, patterns, text (`{caption|date}`), images, background image,
  transparent overlay, colour options for guests, caption allowed or not, offered in Booth Mode.
* Storage: `templates/custom/<id>/{frame.json, preview.png, assets/}`. Saving writes a complete new folder and swaps it in
  (an interrupted swap is repaired on the next start); the revision increases with every save. Deleting moves the folder
  to `templates/custom/.deleted/`; sessions are unaffected because each owns its snapshot.
* Image import (PNG / JPEG): the bytes are decoded — the signature decides, not the file name or MIME type — and checked
  against 15 MB, 10 000 px per side and 40 megapixels (a "decompression bomb" header is refused before any pixels are
  allocated). The file is stored under a hash-based name with the detected extension, in the frame's own `assets/`;
  no path of the original is kept. SVG, GIF, HTML and executables are refused; frames cannot name URLs or paths. Alpha is
  preserved and reported; the builder warns when an overlay has no transparency, is stretched or is lower-resolution than the print.
* Imported but unsaved images wait in `templates/.staging/` and are purged after 24 hours.
* Guests never reach the builder: Booth Mode only lists enabled frames and starts sessions.

## Printing frames

* **What is printed is the finished composition**: `PrintMedia` / `PrintResult` read the stored result image (the file the
  guest approved). Nothing is re-rendered from the frame, so a frame that was edited or deleted afterwards cannot change
  a print; a **reprint** sends exactly the file of the original job (`TestReprintUsesTheFinalizedFileNotAnEditedFrame`).
  A finished strip is also checked against the pixel size of its own frame snapshot before printing.
* **Two-up for 2×6 strips.** The display strip (600×1800 px) is a different thing from the physical print. When the paper
  fits two strips side by side (4×6, 5×7) the Printer page and the Gallery offer *Two strips side by side on this sheet
  (cut it in half)* — on by default for a strip on 4×6 — and the preview shows the real 1200×1800 sheet before anything is
  sent. The two halves are the approved strip copied pixel for pixel (`templates.TwoUpSheet`; no resampling) at the same
  DPI (300). The sheet is stored as a `print` image that records the frame that made it. Only narrow strips qualify
  (width ≤ 40 % of height); a 4×6 sheet frame is refused. Designs that already contain two strips (`twoup4`) are a
  different, design-level choice and are printed as they are.
* **Booth**: after the photos, the result screen has *Print* (one tap, using the printer and paper the operator chose in
  Studio → Printer; hidden when none is set). A 2×6 strip on 4×6 paper is printed two-up automatically. The message says
  *sent to the printer* — a job in the queue is not proof that paper came out (see `docs/PRINTING.md`). The same tap never
  prints twice (request id + duplicate window).
* Dimensions, DPI metadata, cropping and margins come from the existing page planner; the preview is computed from the
  printer's real printable area when the driver reports it.

## Print resolution (300 / 600 DPI)

* **Settings → Photo sessions → Print resolution** chooses 300 dpi (standard) or 600 dpi (high) for the **built-in** frames.
  It is applied when a session starts and written into that session's snapshot, so changing it later never alters a finished
  session or a reprint. Custom frames keep the DPI set in the Frame Builder (150 / 200 / 300 / 600).
* Frames are described in inches, so every layout, border, pattern and caption scales exactly: at 600 dpi a 2×6 strip is
  1200×3600 px, a 4×6 sheet or two-up is 2400×3600, 5×7 is 3000×4200, and the JPEG carries 600 dpi in its header. The
  print planner uses the file's dpi and the printer's real resolution, so true-size placement is the same on a 300 and a
  600 dpi printer; two-up sheets of 600 dpi strips are 2400×3600 at 600 dpi.
* Cost: a 600 dpi render takes about a second for 5×7 on a development PC and files are about four times larger. The
  canvas limit (80 megapixels) and the 72–600 dpi range still apply.
* Tests: exact pixel size for all nine layouts, 600 dpi render ≈ 300 dpi render, density in the JPEG, snapshot keeps the
  session's resolution, custom frames unaffected by the setting, printing at 300 and 600 dpi printers, two-up at 600 dpi,
  and the Windows CI job runs the whole frame flow at both resolutions.
* **Not verified**: a physical printer at 600 dpi. Whether a driver really prints 600 dpi depends on the printer and its
  driver settings; the app only guarantees the file and the placement.

## Tests that guard rendering

`internal/templates/golden_test.go` renders eight built-in frames and a custom frame with a transparent overlay, an image
layer and a caption at 100 DPI and compares them with `testdata/golden/*.png` (mean difference < 0.5/255). Regenerate on
purpose with `UPDATE_GOLDEN=1 go test -run Golden ./internal/templates` and review the pictures. Other tests: preview ≈ 300-DPI
output, 600-DPI size and density, two-up pixel copy, snapshot independence, interrupted composition. The Windows CI job also runs
the whole frame flow (choose a frame → four photos → composition → two-up sheet) through the real service and publishes
`selftest-frame-strip-2x6.jpg` / `selftest-frame-twoup-4x6.jpg` in `ci-evidence/`.

## Limits

Latin-only fonts; no layer rotation; the builder has only been exercised in jsdom and a browser harness, never in WebView2.
