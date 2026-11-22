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

## Not in this stage

The Studio frame **builder** (custom artwork import, drag-and-drop slots), printing the frame snapshot as a two-up
4×6 sheet and golden-image regression files are staged as F3.2 / F3.3.
