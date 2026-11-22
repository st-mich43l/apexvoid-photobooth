package templates

import (
	"image"
	"image/color"
	"testing"
)

// frameWithWindows builds a w×h picture: opaque pink with the given transparent rectangles.
func frameWithWindows(w, h int, windows ...image.Rectangle) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 244, 150, 190, 255
	}
	for _, r := range windows {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				img.SetNRGBA(x, y, color.NRGBA{})
			}
		}
	}
	return img
}

func near2(a, b PxRect, tol int) bool {
	d := func(x, y int) bool { return x-y <= tol && y-x <= tol }
	return d(a.X, b.X) && d(a.Y, b.Y) && d(a.W, b.W) && d(a.H, b.H)
}

func TestDetectsTheFourWindowsOfAStripInReadingOrder(t *testing.T) {
	wins := []image.Rectangle{image.Rect(45, 45, 555, 405), image.Rect(45, 435, 555, 795), image.Rect(45, 825, 555, 1185), image.Rect(45, 1215, 555, 1575)}
	got := DetectWindows(frameWithWindows(600, 1800, wins...))
	if len(got) != 4 {
		t.Fatalf("%v", got)
	}
	for i, w := range wins {
		if !near2(got[i], PxRect{w.Min.X, w.Min.Y, w.Dx(), w.Dy()}, 2) {
			t.Errorf("window %d: %+v, want %v", i+1, got[i], w)
		}
	}
}

func TestWindowsAreNumberedRowByRowAndFoundAtHighResolution(t *testing.T) {
	// a 2×3 grid at 1200×1800, listed out of order on purpose
	var rs []image.Rectangle
	for _, c := range [][2]int{{650, 1200}, {50, 50}, {650, 50}, {50, 650}, {50, 1200}, {650, 650}} {
		rs = append(rs, image.Rect(c[0], c[1], c[0]+500, c[1]+500))
	}
	got := DetectWindows(frameWithWindows(1200, 1800, rs...))
	want := [][2]int{{50, 50}, {650, 50}, {50, 650}, {650, 650}, {50, 1200}, {650, 1200}}
	if len(got) != 6 {
		t.Fatalf("%v", got)
	}
	for i, w := range want {
		if abs(got[i].X-w[0]) > 2 || abs(got[i].Y-w[1]) > 2 {
			t.Errorf("slot %d at %+v, want %v", i+1, got[i], w)
		}
	}
	// 2400×3600 (600 dpi) is analysed on a coarser grid but stays accurate
	big := DetectWindows(frameWithWindows(2400, 3600, image.Rect(100, 100, 1100, 1500)))
	if len(big) != 1 || !near2(big[0], PxRect{100, 100, 1000, 1400}, 4) {
		t.Fatalf("%v", big)
	}
}

func abs(i int) int {
	if i < 0 {
		return -i
	}
	return i
}

func TestRoundedWindowsStillCount(t *testing.T) {
	img := frameWithWindows(600, 1800)
	cx, cy, r := 300, 500, 250
	for y := cy - r; y < cy+r; y++ {
		for x := cx - r; x < cx+r; x++ {
			if (x-cx)*(x-cx)+(y-cy)*(y-cy) < r*r { // a circular hole: ~78 % of its bounding box
				img.SetNRGBA(x, y, color.NRGBA{})
			}
		}
	}
	got := DetectWindows(img)
	if len(got) != 1 || !near2(got[0], PxRect{50, 250, 500, 500}, 3) {
		t.Fatalf("%v", got)
	}
}

func TestSpecksThinLinesOpaquePicturesAndFullyTransparentOnesGiveNoWindows(t *testing.T) {
	if w := DetectWindows(frameWithWindows(600, 1800)); len(w) != 0 {
		t.Fatalf("an opaque picture has no windows: %v", w)
	}
	if w := DetectWindows(frameWithWindows(600, 1800, image.Rect(10, 10, 20, 20), image.Rect(0, 900, 600, 904))); len(w) != 0 {
		t.Fatalf("specks and hairlines are not photo windows: %v", w)
	}
	if w := DetectWindows(frameWithWindows(600, 1800, image.Rect(0, 0, 600, 1800))); len(w) != 0 {
		t.Fatalf("a picture that is transparent everywhere has no window: %v", w)
	}
	// a fully transparent surround with a decorative border: the surround is not a window
	edge := frameWithWindows(600, 1800, image.Rect(0, 0, 600, 1800))
	for y := 100; y < 1700; y++ {
		for x := 100; x < 500; x++ {
			if y < 130 || y > 1670 || x < 130 || x > 470 {
				edge.SetNRGBA(x, y, color.NRGBA{200, 0, 0, 255})
			}
		}
	}
	for _, w := range DetectWindows(edge) {
		if w.W > 560 && w.H > 1700 {
			t.Fatalf("the transparent outside must not become a window: %+v", w)
		}
	}
	// a semi-transparent veil (alpha 200) is artwork, not a window
	veil := frameWithWindows(600, 1800)
	for i := 3; i < len(veil.Pix); i += 4 {
		veil.Pix[i] = 200
	}
	if w := DetectWindows(veil); len(w) != 0 {
		t.Fatalf("%v", w)
	}
}

func TestGuessCanvasMatchesStandardPrintSizesAtEveryResolution(t *testing.T) {
	for _, c := range []struct {
		w, h   int
		wi, hi float64
		dpi    int
		std    bool
	}{
		{600, 1800, 2, 6, 300, true}, {1200, 3600, 2, 6, 600, true}, {1200, 1800, 4, 6, 300, true}, {1800, 1200, 6, 4, 300, true},
		{1500, 2100, 5, 7, 300, true}, {2400, 3600, 4, 6, 600, true}, {1195, 1805, 4, 6, 300, true}, {400, 1200, 2, 6, 200, true},
		{1000, 1000, 3.33, 3.33, 300, false},
	} {
		wi, hi, dpi, std := GuessCanvas(c.w, c.h)
		if wi != c.wi || hi != c.hi || dpi != c.dpi || std != c.std {
			t.Errorf("%dx%d → %g×%g @%d std=%v, want %g×%g @%d std=%v", c.w, c.h, wi, hi, dpi, std, c.wi, c.hi, c.dpi, c.std)
		}
	}
	if _, _, dpi, _ := GuessCanvas(9000, 12000); dpi != 600 {
		t.Errorf("a very large picture needs 600 dpi to stay within the canvas limit, got %d", dpi)
	}
}

func TestSlotsGrowByABleedAndStayInsideTheCanvas(t *testing.T) {
	s := SlotsFromWindows([]PxRect{{0, 0, 600, 300}, {45, 450, 510, 360}}, 600, 1800, 2, 6)
	if s[0].X != 0 || s[0].Y != 0 || s[0].W != 2 || s[0].Photo != 0 || s[1].Photo != 1 {
		t.Fatalf("%+v", s)
	}
	if s[1].X > 0.15 || s[1].X < 0.13 || s[1].W < 1.7 {
		t.Fatalf("%+v", s[1])
	}
	f := Template{ID: "x", Geometry: Geometry{WidthIn: 2, HeightIn: 6, DPI: 300, Slots: s}}
	if err := f.Validate(); err != nil {
		t.Fatalf("slots from a picture must validate: %v", err)
	}
}
