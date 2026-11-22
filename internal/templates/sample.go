package templates

import (
	"image"
	"image/color"
	"image/draw"
	"io/fs"
	"math"
	"sync"

	"golang.org/x/image/vector"
)

var samplePalettes = [][3]color.NRGBA{
	{{255, 214, 224, 255}, {244, 160, 190, 255}, {120, 80, 100, 255}},
	{{214, 224, 255, 255}, {150, 170, 235, 255}, {70, 80, 120, 255}},
	{{255, 233, 190, 255}, {246, 190, 120, 255}, {110, 80, 60, 255}},
	{{204, 240, 224, 255}, {130, 200, 170, 255}, {60, 100, 90, 255}},
	{{232, 214, 250, 255}, {190, 160, 235, 255}, {90, 70, 120, 255}},
	{{250, 220, 205, 255}, {236, 170, 150, 255}, {120, 70, 60, 255}},
}

var (
	sampleOnce  sync.Once
	sampleCache []image.Image
)

// SamplePhotos returns n generated stand-in portraits (a head and shoulders on
// a soft gradient). They are obviously illustrations, never camera output, and
// are used only for frame previews. The images are shared: do not modify them.
func SamplePhotos(n int) []image.Image {
	sampleOnce.Do(func() {
		for i := range samplePalettes {
			sampleCache = append(sampleCache, samplePortrait(i))
		}
	})
	out := make([]image.Image, n)
	for i := range out {
		out[i] = sampleCache[i%len(sampleCache)]
	}
	return out
}

func samplePortrait(i int) image.Image {
	const w, h = 600, 600 // drawn at 600 and scaled by the coordinates below (units of 1/1200)
	p := samplePalettes[i%len(samplePalettes)]
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ { // vertical gradient
		f := float64(y) / h
		c := color.NRGBA{
			R: uint8(float64(p[0].R)*(1-f) + float64(p[1].R)*f),
			G: uint8(float64(p[0].G)*(1-f) + float64(p[1].G)*f),
			B: uint8(float64(p[0].B)*(1-f) + float64(p[1].B)*f), A: 255}
		draw.Draw(img, image.Rect(0, y, w, y+1), image.NewUniform(c), image.Point{}, draw.Src)
	}
	shift := float32(i%3-1) * 20
	body := color.NRGBA{p[2].R, p[2].G, p[2].B, 255}
	skin := color.NRGBA{250, 224, 205, 255}
	blob := func(c color.NRGBA, x, y, bw, bh float32, round bool) {
		r := image.Rect(int(x), int(y), int(x+bw)+1, int(y+bh)+1)
		fill(img, r, c, func(z *vector.Rasterizer, ox, oy float32) {
			if round {
				ellipsePath(z, ox, oy, bw, bh, false)
			} else {
				rrectPath(z, ox, oy, bw, bh, 25, false)
			}
		})
	}
	blob(body, 125+shift, 390, 350, 350, true)
	blob(skin, 260+shift, 320, 80, 95, false)
	blob(body, 197+shift, 125, 205, 235, true)
	blob(skin, 210+shift, 165, 180, 205, true)
	return img
}

// Preview renders the frame with sample photos about widthPx pixels wide. It
// uses exactly the same renderer as the final output, only at a lower DPI.
func Preview(t Template, assets fs.FS, rc RenderContext, widthPx int) (*image.NRGBA, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}
	t.DPI = max(20, int(math.Round(float64(widthPx)/t.WidthIn)))
	return compose(t, SamplePhotos(t.Photos()), assets, rc)
}
