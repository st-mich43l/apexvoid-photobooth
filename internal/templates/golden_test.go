package templates

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
)

// Golden-image regression tests: a few important frames are rendered with the
// generated sample portraits at 100 DPI and compared with the PNG in
// testdata/golden. Regenerate deliberately with UPDATE_GOLDEN=1 go test -run Golden
// and review the pictures before committing them.
var goldenDate = time.Date(2026, 11, 22, 9, 30, 0, 0, time.UTC)

type goldenCase struct {
	name, frame, variant, caption string
}

func renderGolden(t *testing.T, l *Library, c goldenCase) *image.NRGBA {
	t.Helper()
	f := l.Get(c.frame)
	if f == nil {
		t.Fatalf("frame %s", c.frame)
	}
	r, err := f.WithVariant(c.variant)
	if err != nil {
		t.Fatal(err)
	}
	r.DPI = 100
	img, err := Compose(r, SamplePhotos(r.Photos()), l.Assets(c.frame), RenderContext{Caption: c.caption, Date: goldenDate})
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func checkGolden(t *testing.T, name string, img *image.NRGBA) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name+".png")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(f, img); err != nil {
			t.Fatal(err)
		}
		return
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("golden image %s is missing (run with UPDATE_GOLDEN=1 once and review it): %v", name, err)
	}
	defer f.Close()
	want, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if want.Bounds() != img.Bounds() {
		t.Fatalf("%s: size %v, golden %v", name, img.Bounds(), want.Bounds())
	}
	var sum, big, n float64
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			c := img.NRGBAAt(x, y)
			wr, wg, wb, _ := want.At(x, y).RGBA()
			for _, d := range []int{int(c.R) - int(wr>>8), int(c.G) - int(wg>>8), int(c.B) - int(wb>>8)} {
				if d < 0 {
					d = -d
				}
				sum += float64(d)
				if d > 24 {
					big++
				}
				n++
			}
		}
	}
	if mean, frac := sum/n, big/n; mean > 0.5 || frac > 0.001 {
		out := filepath.Join(os.TempDir(), "golden-actual-"+name+".png")
		if f, err := os.Create(out); err == nil {
			_ = png.Encode(f, img)
			f.Close()
		}
		t.Fatalf("%s differs from the golden image: mean %.2f/255, %.3f%% of samples off by >24 (actual written to %s)", name, mean, frac*100, out)
	}
}

func TestGoldenFrames(t *testing.T) {
	l := Load(t.TempDir())
	for _, c := range []goldenCase{
		{"rose-pink-strip4-coral", "rose-pink.strip4", "coral", ""},
		{"film-strip-twoup4", "film-strip.twoup4", "", ""},
		{"polaroid-p57", "polaroid.p57", "peach", "Mai & Nam"},
		{"birthday-sheet6", "birthday.sheet6", "", "Happy 30th!"},
		{"couple-strip3", "couple.strip3", "wine", ""},
		{"vintage-camera-strip4", "vintage-camera.strip4", "", ""},
		{"monochrome-film-strip4", "monochrome-film.strip4", "", ""},
		{"plain-single", "plain.single", "black", ""},
	} {
		t.Run(c.name, func(t *testing.T) { checkGolden(t, c.name, renderGolden(t, l, c)) })
	}
}

func TestGoldenCustomFrameWithTransparentOverlayAndImageLayer(t *testing.T) {
	l := Load(t.TempDir())
	ov := image.NewNRGBA(image.Rect(0, 0, 200, 600)) // a deterministic "window" overlay: translucent pink wash with clear photo windows
	for y := 0; y < 600; y++ {
		for x := 0; x < 200; x++ {
			a := uint8(0)
			if x < 12 || x > 187 || y%150 < 12 {
				a = 200
			}
			ov.SetNRGBA(x, y, color.NRGBA{255, 120, 170, a})
		}
	}
	info, err := l.Staging().Import(pngBytes(ov))
	if err != nil {
		t.Fatal(err)
	}
	logo, _ := l.Staging().Import(pngBytes(media.Solid(40, 40, color.NRGBA{30, 60, 200, 255})))
	d, _ := l.NewDraft("plain.strip4", "Golden custom")
	d.Background, d.Overlay, d.Caption = "#1d1530", info.Name, true
	d.Layers = []Layer{
		{ID: "logo", Kind: LayerImage, Asset: logo.Name, X: 0.8, Y: 5.35, W: 0.4, H: 0.4},
		{ID: "t", Kind: LayerText, X: 0.1, Y: 5.75, W: 1.8, H: 0.2, Text: "{caption|date}", Font: "italic", SizeIn: 0.12, Color: "#f4a6c0"},
	}
	saved, err := l.SaveCustom(d, l.Staging().FS())
	if err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "custom-overlay", renderGolden(t, l, goldenCase{"", saved.ID, "", "Mai & Nam"}))
}

func TestTwoUpSheetCopiesPixelsExactly(t *testing.T) {
	strip, err := Compose(*Load(t.TempDir()).Get("rose-pink.strip4"), SamplePhotos(4), nil, RenderContext{Date: goldenDate})
	if err != nil {
		t.Fatal(err)
	}
	sheet := TwoUpSheet(strip, color.NRGBA{255, 255, 255, 255})
	if sheet.Bounds().Dx() != 1200 || sheet.Bounds().Dy() != 1800 {
		t.Fatalf("%v", sheet.Bounds())
	}
	for _, off := range []int{0, 600} {
		for y := 0; y < 1800; y += 3 {
			for x := 0; x < 600; x += 3 {
				if sheet.NRGBAAt(off+x, y) != strip.NRGBAAt(x, y) {
					t.Fatalf("pixel (%d,%d) of half %d differs", x, y, off)
				}
			}
		}
	}
	if !StripLike(600, 1800) || StripLike(1200, 1800) || StripLike(1500, 2100) || StripLike(1800, 1200) {
		t.Fatal("only narrow strips are doubled")
	}
}

func TestHighDPIOutputIsExactAndCarriesItsDensity(t *testing.T) {
	f := Load(t.TempDir()).Get("rose-pink.strip4")
	f.DPI = 600
	data, w, h, err := Render(*f, SamplePhotos(4), nil, RenderContext{Date: goldenDate})
	if err != nil || w != 1200 || h != 3600 {
		t.Fatalf("%v %d×%d", err, w, h)
	}
	if u, x, y, ok := media.JPEGDensity(data); !ok || u != 1 || x != 600 || y != 600 {
		t.Fatalf("density %v %v %v", u, x, y)
	}
	// the same frame at 150 DPI keeps its proportions
	f.DPI = 150
	_, w2, h2, _ := Render(*f, SamplePhotos(4), nil, RenderContext{})
	if w2 != 300 || h2 != 900 {
		t.Fatalf("%d×%d", w2, h2)
	}
}

func TestEveryLayoutAt600DPIIsExactAndMatchesThe300DPIRender(t *testing.T) {
	want := map[string][2]int{"strip4": {1200, 3600}, "strip3": {1200, 3600}, "strip2": {1200, 3600}, "twoup4": {2400, 3600}, "sheet6": {2400, 3600}, "sheet4": {2400, 3600}, "sheet2": {2400, 3600}, "p57": {3000, 4200}, "single": {2400, 3600}}
	l := Load(t.TempDir())
	for lid, px := range want {
		f := *l.Get("birthday." + lid) // birthday has patterns, text and rounded photos: the hardest case
		f.DPI = 600
		hi, err := Compose(f, SamplePhotos(f.Photos()), nil, RenderContext{Caption: "Hello", Date: goldenDate})
		if err != nil {
			t.Fatalf("%s: %v", lid, err)
		}
		if hi.Bounds().Dx() != px[0] || hi.Bounds().Dy() != px[1] {
			t.Fatalf("%s at 600 dpi is %v, want %dx%d", lid, hi.Bounds(), px[0], px[1])
		}
		f.DPI = 300
		lo, err := Compose(f, SamplePhotos(f.Photos()), nil, RenderContext{Caption: "Hello", Date: goldenDate})
		if err != nil {
			t.Fatal(err)
		}
		down := image.NewNRGBA(lo.Bounds())
		scaleDown(down, hi)
		if d := meanAbsDiff(lo, down); d > 6 {
			t.Errorf("%s: the 600 dpi render differs from the 300 dpi one (mean %.1f/255): layout is not resolution independent", lid, d)
		}
	}
}

func TestSnapshotResolutionOverrideAppliesToBuiltInsOnly(t *testing.T) {
	l := Load(t.TempDir())
	snap, err := l.TakeSnapshot(filepath.Join(t.TempDir(), "001"), "rose-pink.strip4", "", "", time.Now(), 600)
	if err != nil || snap.Frame.DPI != 600 {
		t.Fatalf("%v %+v", err, snap.Frame.DPI)
	}
	d, _ := l.NewDraft("plain.strip4", "Mine")
	saved, err := l.SaveCustom(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	snap2, err := l.TakeSnapshot(filepath.Join(t.TempDir(), "001"), saved.ID, "", "", time.Now(), 600)
	if err != nil || snap2.Frame.DPI != 300 {
		t.Fatalf("a custom frame keeps its own dpi: %v %d", err, snap2.Frame.DPI)
	}
	if _, err := l.TakeSnapshot(filepath.Join(t.TempDir(), "001"), "rose-pink.strip4", "", "", time.Now(), 1200); err == nil {
		t.Fatal("1200 dpi exceeds the supported range")
	}
}

func TestSixHundredDPICanvasLimits(t *testing.T) {
	f := *Load(t.TempDir()).Get("plain.p57")
	f.DPI = 600
	if err := f.Validate(); err != nil {
		t.Fatalf("5x7 at 600 dpi (12.6 MP) must be allowed: %v", err)
	}
	f.WidthIn, f.HeightIn = 24, 36 // 14400×21600 px = 311 MP
	if f.Validate() == nil {
		t.Fatal("a canvas over 80 megapixels must be refused even at 600 dpi")
	}
}
