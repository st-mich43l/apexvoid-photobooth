package templates

import (
	"image"
	"math"
	"sort"
)

// PxRect is a rectangle in image pixels.
type PxRect struct{ X, Y, W, H int }

// DetectWindows finds the transparent "windows" of a frame picture: connected areas that
// are (mostly) transparent and roughly rectangular. They are returned in reading order
// (top to bottom, left to right). Nothing is returned for an opaque picture, and the big
// transparent surround of a picture that is transparent everywhere is ignored.
func DetectWindows(img image.Image) []PxRect {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 8 || h < 8 {
		return nil
	}
	f := max(1, (max(w, h)+1199)/1200) // work on a grid of at most ~1200 cells per side
	cw, ch := (w+f-1)/f, (h+f-1)/f
	clear := make([]bool, cw*ch)
	transparentAny := false
	for cy := 0; cy < ch; cy++ {
		for cx := 0; cx < cw; cx++ {
			n, total := 0, 0
			for y := cy * f; y < min((cy+1)*f, h); y++ {
				for x := cx * f; x < min((cx+1)*f, w); x++ {
					total++
					if alphaAt(img, b.Min.X+x, b.Min.Y+y) < 128 {
						n++
					}
				}
			}
			if n*2 >= total {
				clear[cy*cw+cx] = true
				transparentAny = true
			}
		}
	}
	if !transparentAny {
		return nil
	}
	type comp struct{ x0, y0, x1, y1, cells int }
	seen := make([]bool, cw*ch)
	var comps []comp
	for start := range clear {
		if !clear[start] || seen[start] {
			continue
		}
		c := comp{x0: cw, y0: ch, x1: -1, y1: -1}
		stack := []int{start}
		seen[start] = true
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			x, y := p%cw, p/cw
			c.cells++
			c.x0, c.y0, c.x1, c.y1 = min(c.x0, x), min(c.y0, y), max(c.x1, x), max(c.y1, y)
			for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nx, ny := x+d[0], y+d[1]
				if nx < 0 || ny < 0 || nx >= cw || ny >= ch {
					continue
				}
				if q := ny*cw + nx; clear[q] && !seen[q] {
					seen[q] = true
					stack = append(stack, q)
				}
			}
		}
		comps = append(comps, c)
	}
	var wins []PxRect
	for _, c := range comps {
		bw, bh := c.x1-c.x0+1, c.y1-c.y0+1
		boxCells := bw * bh
		switch {
		case float64(c.cells) < 0.01*float64(cw*ch): // specks and thin lines
			continue
		case float64(c.cells) < 0.6*float64(boxCells): // not rectangular enough to be a photo window
			continue
		case bw >= cw*95/100 && bh >= ch*95/100: // the whole picture is transparent: no window
			continue
		}
		wins = append(wins, PxRect{X: c.x0 * f, Y: c.y0 * f, W: min(bw*f, w-c.x0*f), H: min(bh*f, h-c.y0*f)})
	}
	return readingOrder(wins)
}

// readingOrder sorts rectangles into rows (by vertical overlap) and then left to right.
func readingOrder(r []PxRect) []PxRect {
	sort.Slice(r, func(i, j int) bool { return r[i].Y+r[i].H/2 < r[j].Y+r[j].H/2 })
	var rows [][]PxRect
	for _, x := range r {
		cy := x.Y + x.H/2
		placed := false
		for i := range rows {
			ref := rows[i][0]
			if cy >= ref.Y && cy <= ref.Y+ref.H {
				rows[i] = append(rows[i], x)
				placed = true
				break
			}
		}
		if !placed {
			rows = append(rows, []PxRect{x})
		}
	}
	var out []PxRect
	for _, row := range rows {
		sort.Slice(row, func(i, j int) bool { return row[i].X < row[j].X })
		out = append(out, row...)
	}
	return out
}

func alphaAt(img image.Image, x, y int) uint8 {
	switch m := img.(type) {
	case *image.NRGBA:
		return m.Pix[m.PixOffset(x, y)+3]
	case *image.RGBA:
		return m.Pix[m.PixOffset(x, y)+3]
	}
	_, _, _, a := img.At(x, y).RGBA()
	return uint8(a >> 8)
}

// standardSizes are the print formats a frame picture is matched to (short side, long side in inches).
var standardSizes = [][2]float64{{2, 6}, {4, 6}, {5, 7}, {6, 8}, {8, 10}, {3.5, 5}}

// GuessCanvas decides the physical size and DPI for a picture of w×h pixels. A picture that
// matches a standard print size at 300, 600, 200 or 150 dpi (within 3 %) gets exactly that
// size (a 600×1800 picture is a 2×6 strip at 300 dpi, 1200×3600 is the same strip at 600 dpi).
// Anything else is taken at 300 dpi (600 when that would be larger than the maximum canvas).
func GuessCanvas(w, h int) (widthIn, heightIn float64, dpi int, standard bool) {
	short, long := min(w, h), max(w, h)
	for _, d := range []int{300, 600, 200, 150} {
		for _, s := range standardSizes {
			if math.Abs(float64(short)/float64(d)-s[0])/s[0] < 0.03 && math.Abs(float64(long)/float64(d)-s[1])/s[1] < 0.03 {
				if w <= h {
					return s[0], s[1], d, true
				}
				return s[1], s[0], d, true
			}
		}
	}
	dpi = 300
	if float64(w)/300 > 24 || float64(h)/300 > 36 {
		dpi = 600
	}
	return math.Round(float64(w)/float64(dpi)*100) / 100, math.Round(float64(h)/float64(dpi)*100) / 100, dpi, false
}

// SlotsFromWindows turns pixel windows into photo slots on a canvas of widthIn×heightIn. Each
// slot is grown by a hair (0.01 in) so the photo reaches under the overlay's edge and no seam
// of background shows between photo and artwork.
func SlotsFromWindows(wins []PxRect, imgW, imgH int, widthIn, heightIn float64) []Slot {
	sx, sy := widthIn/float64(imgW), heightIn/float64(imgH)
	const bleed = 0.01
	r3 := func(v float64) float64 { return math.Round(v*1000) / 1000 }
	var out []Slot
	for i, r := range wins {
		x0, y0 := math.Max(0, float64(r.X)*sx-bleed), math.Max(0, float64(r.Y)*sy-bleed)
		x1, y1 := math.Min(widthIn, float64(r.X+r.W)*sx+bleed), math.Min(heightIn, float64(r.Y+r.H)*sy+bleed)
		out = append(out, Slot{X: r3(x0), Y: r3(y0), W: r3(x1 - x0), H: r3(y1 - y0), Photo: i})
	}
	return out
}
