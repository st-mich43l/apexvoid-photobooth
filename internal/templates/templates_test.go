package templates

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
)

func solid(w, h int, c color.NRGBA) image.Image { return media.Solid(w, h, c) }

func TestBuiltinsAreValidAndExactSize(t *testing.T) {
	want := map[string][2]int{"strip-2x6-4": {600, 1800}, "strip-4x6-2up": {1200, 1800}, "single-4x6": {1800, 1200}, "sheet-4x6-6": {1200, 1800}}
	for _, tpl := range Builtin() {
		if err := tpl.Validate(); err != nil {
			t.Fatalf("%s: %v", tpl.ID, err)
		}
		photos := make([]image.Image, tpl.Photos())
		for i := range photos {
			photos[i] = solid(900, 600, color.NRGBA{uint8(40 * i), 100, 200, 255})
		}
		data, w, h, err := Render(tpl, photos, "")
		if err != nil {
			t.Fatalf("%s: %v", tpl.ID, err)
		}
		if exp, ok := want[tpl.ID]; ok && (w != exp[0] || h != exp[1]) {
			t.Errorf("%s is %dx%d px, want %dx%d", tpl.ID, w, h, exp[0], exp[1])
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width != w || cfg.Height != h {
			t.Errorf("%s: encoded %v %v", tpl.ID, cfg, err)
		}
		if u, x, y, ok := media.JPEGDensity(data); !ok || u != 1 || x != 300 || y != 300 {
			t.Errorf("%s: DPI metadata missing: %v %v %v", tpl.ID, u, x, y)
		}
	}
}

func TestForShotsMatchesPhotoCount(t *testing.T) {
	lib := Load(t.TempDir())
	for _, n := range []int{1, 2, 4, 6} {
		tpl := lib.Get(ForShots(n))
		if tpl == nil || tpl.Photos() != n {
			t.Fatalf("shots=%d -> %v", n, tpl)
		}
	}
}

func TestTwoUpRepeatsPhotos(t *testing.T) {
	tpl := *Load(t.TempDir()).Get("strip-4x6-2up")
	if tpl.Photos() != 4 || len(tpl.Slots) != 8 {
		t.Fatalf("photos=%d slots=%d", tpl.Photos(), len(tpl.Slots))
	}
}

func TestComposePlacesPhotosAndBackground(t *testing.T) {
	tpl := Template{ID: "t", DPI: 100, WidthIn: 2, HeightIn: 2, Background: "#ff0000",
		Slots: []Slot{{X: 0.5, Y: 0.5, W: 1, H: 1, Photo: 0}}}
	out, err := Compose(tpl, []image.Image{solid(400, 100, color.NRGBA{0, 255, 0, 255})}, "")
	if err != nil {
		t.Fatal(err)
	}
	if c := out.NRGBAAt(10, 10); c.R != 255 || c.G != 0 {
		t.Fatalf("background %v", c)
	}
	if c := out.NRGBAAt(100, 100); c.G < 250 || c.R > 5 {
		t.Fatalf("photo not placed: %v", c)
	}
}

func TestCoverCropsInsteadOfStretching(t *testing.T) {
	// left half red, right half blue; a square slot must show only the centre crop of a wide photo
	src := image.NewNRGBA(image.Rect(0, 0, 400, 100))
	for x := 0; x < 400; x++ {
		for y := 0; y < 100; y++ {
			c := color.NRGBA{255, 0, 0, 255}
			if x >= 200 {
				c = color.NRGBA{0, 0, 255, 255}
			}
			src.SetNRGBA(x, y, c)
		}
	}
	dst := media.Solid(100, 100, color.NRGBA{})
	cover(dst, image.Rect(0, 0, 100, 100), src, 0.5, 0.5)
	if l, r := dst.NRGBAAt(5, 50), dst.NRGBAAt(95, 50); l.R < 250 || r.B < 250 {
		t.Fatalf("centre crop expected, got %v %v", l, r)
	}
}

func TestComposeRejectsBadInput(t *testing.T) {
	tpl := Builtin()[0]
	if _, err := Compose(tpl, nil, ""); err == nil {
		t.Fatal("too few photos accepted")
	}
	bad := tpl
	bad.Slots = []Slot{{X: 1.9, Y: 0, W: 1, H: 1}}
	if bad.Validate() == nil {
		t.Fatal("slot outside canvas accepted")
	}
	huge := tpl
	huge.WidthIn, huge.HeightIn, huge.DPI = 24, 36, 600
	if huge.Validate() == nil {
		t.Fatal("80MP+ canvas accepted")
	}
}

func TestUserTemplatesAndOverlay(t *testing.T) {
	dir := t.TempDir()
	ov := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	for i := 0; i < 10; i++ { // opaque white top row
		ov.SetNRGBA(i, 0, color.NRGBA{255, 255, 255, 255})
	}
	f, _ := os.Create(filepath.Join(dir, "frame.png"))
	_ = png.Encode(f, ov)
	f.Close()
	_ = os.WriteFile(filepath.Join(dir, "mine.json"), []byte(`{"id":"mine","name":"Mine","widthIn":2,"heightIn":2,"dpi":100,"background":"#000000","overlay":"frame.png","slots":[{"x":0.2,"y":0.2,"w":1.6,"h":1.6,"photo":0}]}`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "broken.json"), []byte(`{"id":"b","dpi":5}`), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "garbage.json"), []byte(`nope`), 0o644)
	lib := Load(dir)
	tpl := lib.Get("mine")
	if tpl == nil || len(lib.Errs) != 2 {
		t.Fatalf("tpl=%v errs=%v", tpl, lib.Errs)
	}
	out, err := lib.Compose(*tpl, []image.Image{solid(100, 100, color.NRGBA{0, 0, 255, 255})})
	if err != nil {
		t.Fatal(err)
	}
	if c := out.NRGBAAt(50, 0); c.R < 250 { // overlay row (scaled) is on top
		t.Fatalf("overlay missing: %v", c)
	}
	tpl.Overlay = "../../etc/passwd" // only the base name is ever used
	if _, err := lib.Compose(*tpl, []image.Image{solid(10, 10, color.NRGBA{})}); err == nil {
		t.Fatal("expected overlay error")
	}
}
