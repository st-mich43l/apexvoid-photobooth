package templates

import (
	"image"
	"image/color"
	"image/draw"
)

// TwoUpSheet puts the finished strip twice, side by side, on a sheet twice as
// wide and as tall as the strip (two 2×6 strips make one 4×6 print that is cut
// in half). The strip is copied pixel for pixel — it is never re-rendered or
// resampled — so both halves are exactly the composition the guest approved.
func TwoUpSheet(strip image.Image, background color.NRGBA) *image.NRGBA {
	b := strip.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, 2*w, h))
	draw.Draw(out, out.Bounds(), image.NewUniform(background), image.Point{}, draw.Src)
	draw.Draw(out, image.Rect(0, 0, w, h), strip, b.Min, draw.Src)
	draw.Draw(out, image.Rect(w, 0, 2*w, h), strip, b.Min, draw.Src)
	return out
}

// StripLike reports whether an image of w×h pixels is a narrow strip (width at
// most 40 % of the height) that can be doubled side by side: 2×6 → 4×6.
func StripLike(w, h int) bool { return w > 0 && h > 0 && float64(2*w) <= float64(h)*0.8 }
