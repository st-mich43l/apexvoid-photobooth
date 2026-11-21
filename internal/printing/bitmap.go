package printing

import (
	"image"
	"image/color"
	"image/draw"

	xdraw "golang.org/x/image/draw"

	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
)

// Rotate90 turns the image 90° clockwise (print drivers want the pixels the right way round).
func Rotate90(src image.Image) image.Image { return media.Rotate(src, 90) }

// ToBGRA converts to the top-down 32-bit BGRA layout of a Windows DIB
// (BI_RGB, negative height). Transparent pixels are composited over white.
func ToBGRA(src image.Image) (pix []byte, w, h int) {
	b := src.Bounds()
	w, h = b.Dx(), b.Dy()
	white := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(white, white.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(white, white.Bounds(), src, b.Min, draw.Over)
	pix = make([]byte, w*h*4)
	for i := 0; i < w*h; i++ {
		pix[i*4+0] = white.Pix[i*4+2] // B
		pix[i*4+1] = white.Pix[i*4+1] // G
		pix[i*4+2] = white.Pix[i*4+0] // R
		pix[i*4+3] = 0
	}
	return pix, w, h
}

// Preview draws the sheet (white), the unprintable border (grey), and the
// image exactly where the plan puts it, at previewW pixels wide.
func Preview(img image.Image, g PageGeometry, pl PagePlan, previewW int) image.Image {
	scale := float64(previewW) / float64(g.PhysW)
	ph := int(float64(g.PhysH) * scale)
	out := image.NewNRGBA(image.Rect(0, 0, previewW, ph))
	draw.Draw(out, out.Bounds(), image.NewUniform(color.NRGBA{208, 204, 220, 255}), image.Point{}, draw.Src) // non-printable
	pr := image.Rect(int(float64(g.OffX)*scale), int(float64(g.OffY)*scale), int(float64(g.OffX+g.PrintW)*scale), int(float64(g.OffY+g.PrintH)*scale))
	draw.Draw(out, pr, image.NewUniform(color.White), image.Point{}, draw.Src)
	src := img
	if pl.Rotated {
		src = Rotate90(img)
	}
	d := image.Rect(int(float64(pl.Dest.Min.X+g.OffX)*scale), int(float64(pl.Dest.Min.Y+g.OffY)*scale),
		int(float64(pl.Dest.Max.X+g.OffX)*scale), int(float64(pl.Dest.Max.Y+g.OffY)*scale))
	if d.Dx() > 0 && d.Dy() > 0 {
		xdraw.CatmullRom.Scale(out, d, src, pl.Src, xdraw.Src, nil)
	}
	return out
}
