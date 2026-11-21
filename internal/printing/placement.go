package printing

import (
	"image"
	"math"
)

// GeometryFor estimates the page when the driver cannot be asked (CUPS, tests):
// the sheet at dpi, with an optional uniform unprintable margin.
func GeometryFor(p Paper, o Orientation, dpi int, marginMM float64) PageGeometry {
	w, h := p.WidthIn, p.HeightIn
	if o == Landscape && w < h || o == Portrait && w > h {
		w, h = h, w
	}
	pw, ph := int(math.Round(w*float64(dpi))), int(math.Round(h*float64(dpi)))
	m := int(math.Round(marginMM / 25.4 * float64(dpi)))
	return PageGeometry{PhysW: pw, PhysH: ph, PrintW: pw - 2*m, PrintH: ph - 2*m, OffX: m, OffY: m, DPIX: dpi, DPIY: dpi, Assumed: true}
}

// PagePlan says exactly where the image lands on the page.
type PagePlan struct {
	Rotated bool            `json:"rotated"` // the image is turned 90° to match the page
	Src     image.Rectangle `json:"-"`       // region of the (rotated) image that is drawn
	Dest    image.Rectangle `json:"-"`       // device pixels relative to the printable origin
	// scale actually applied relative to the image's own DPI (100 = true size)
	ScalePct   float64    `json:"scalePct"`
	MarginMM   [4]float64 `json:"marginMm"`   // sheet edge → image: left, top, right, bottom
	CroppedPct float64    `json:"croppedPct"` // part of the image that is cut off
	Warning    string     `json:"warning,omitempty"`
}

// PlanPage places an imgW×imgH image (rendered at imgDPI) on the page.
func PlanPage(imgW, imgH, imgDPI int, g PageGeometry, fit Fit) PagePlan {
	var pl PagePlan
	if imgDPI <= 0 {
		imgDPI = 300
	}
	// turn the image when its shape matches the page only after a 90° turn
	pageLandscape, imgLandscape := g.PhysW > g.PhysH, imgW > imgH
	if g.PhysW != g.PhysH && imgW != imgH && pageLandscape != imgLandscape {
		pl.Rotated = true
		imgW, imgH = imgH, imgW
	}
	printable := image.Rect(0, 0, g.PrintW, g.PrintH)
	full := image.Rect(0, 0, imgW, imgH)
	switch fit {
	case FitFill:
		// cover the whole sheet; crop the image to the sheet's aspect, centred
		s := math.Max(float64(g.PhysW)/float64(imgW), float64(g.PhysH)/float64(imgH))
		cw, ch := float64(g.PhysW)/s, float64(g.PhysH)/s
		x0, y0 := (float64(imgW)-cw)/2, (float64(imgH)-ch)/2
		src := image.Rect(int(math.Round(x0)), int(math.Round(y0)), int(math.Round(x0+cw)), int(math.Round(y0+ch))).Intersect(full)
		sheet := image.Rect(-g.OffX, -g.OffY, g.PhysW-g.OffX, g.PhysH-g.OffY)
		dest := sheet.Intersect(printable)
		// keep the src/dest correspondence when the printable area clips the sheet
		if dest != sheet {
			fx, fy := float64(src.Dx())/float64(sheet.Dx()), float64(src.Dy())/float64(sheet.Dy())
			src = image.Rect(src.Min.X+int(math.Round(float64(dest.Min.X-sheet.Min.X)*fx)), src.Min.Y+int(math.Round(float64(dest.Min.Y-sheet.Min.Y)*fy)),
				src.Min.X+int(math.Round(float64(dest.Max.X-sheet.Min.X)*fx)), src.Min.Y+int(math.Round(float64(dest.Max.Y-sheet.Min.Y)*fy))).Intersect(full)
			pl.Warning = "The printer cannot print to the sheet edge: the outer border is cut off."
		}
		pl.Src, pl.Dest = src, dest
		pl.CroppedPct = 100 * (1 - float64(src.Dx()*src.Dy())/float64(imgW*imgH))
	case FitActual:
		w := float64(imgW) * float64(g.DPIX) / float64(imgDPI)
		h := float64(imgH) * float64(g.DPIY) / float64(imgDPI)
		if w > float64(g.PrintW)+0.5 || h > float64(g.PrintH)+0.5 {
			pl.Warning = "At true size the image is larger than the printable area; it was scaled down to fit."
			pl.Src, pl.Dest = full, centered(fitInside(imgW, imgH, g.PrintW, g.PrintH), printable)
		} else {
			// centre on the SHEET (not the printable area) so margins are symmetric
			cx, cy := (g.PhysW-int(math.Round(w)))/2-g.OffX, (g.PhysH-int(math.Round(h)))/2-g.OffY
			pl.Src, pl.Dest = full, image.Rect(cx, cy, cx+int(math.Round(w)), cy+int(math.Round(h)))
		}
	default: // FitPrintable
		pl.Src, pl.Dest = full, centered(fitInside(imgW, imgH, g.PrintW, g.PrintH), printable)
	}
	if pl.Dest.Dx() > 0 && pl.Src.Dx() > 0 {
		pl.ScalePct = 100 * (float64(pl.Dest.Dx()) / float64(pl.Src.Dx())) / (float64(g.DPIX) / float64(imgDPI))
	}
	// margins on the sheet: dest is relative to the printable origin
	mm := func(px, dpi int) float64 { return float64(px) / float64(dpi) * 25.4 }
	left, top := pl.Dest.Min.X+g.OffX, pl.Dest.Min.Y+g.OffY
	right, bottom := g.PhysW-(pl.Dest.Max.X+g.OffX), g.PhysH-(pl.Dest.Max.Y+g.OffY)
	pl.MarginMM = [4]float64{mm(left, g.DPIX), mm(top, g.DPIY), mm(right, g.DPIX), mm(bottom, g.DPIY)}
	return pl
}

func fitInside(w, h, bw, bh int) image.Point {
	s := math.Min(float64(bw)/float64(w), float64(bh)/float64(h))
	return image.Pt(int(math.Round(float64(w)*s)), int(math.Round(float64(h)*s)))
}

func centered(sz image.Point, in image.Rectangle) image.Rectangle {
	x := in.Min.X + (in.Dx()-sz.X)/2
	y := in.Min.Y + (in.Dy()-sz.Y)/2
	return image.Rect(x, y, x+sz.X, y+sz.Y)
}
