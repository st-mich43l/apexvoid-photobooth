package app

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
	"github.com/st-mich43l/apexvoid-photobooth/internal/templates"
)

func pngFrameB64(w, h int, windows ...image.Rectangle) string {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 200, 30, 90, 255 // magenta artwork
	}
	for _, r := range windows {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				img.SetNRGBA(x, y, color.NRGBA{})
			}
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

func stripWindows() []image.Rectangle {
	return []image.Rectangle{image.Rect(45, 45, 555, 405), image.Rect(45, 435, 555, 795), image.Rect(45, 825, 555, 1185), image.Rect(45, 1215, 555, 1575)}
}

func TestImportingAPNGFrameCreatesAUsableFrameInOneStep(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	imp, err := s.ImportPNGFrame(pngFrameB64(600, 1800, stripWindows()...), "/home/me/Pictures/Wedding Frame.png")
	if err != nil {
		t.Fatal(err)
	}
	d := imp.Draft
	if imp.Mode != "windows" || imp.Windows != 4 || d.Name != "Wedding Frame" || d.WidthIn != 2 || d.HeightIn != 6 || d.DPI != 300 || d.Photos() != 4 || d.Overlay == "" || !d.Enabled {
		t.Fatalf("%+v", imp)
	}
	if s.tpl.Get(d.ID) != nil {
		t.Fatal("nothing is saved until the operator saves the draft")
	}
	for i, sl := range d.Slots { // numbered top to bottom
		if sl.Photo != i || sl.Y < float64(i)*1.2 {
			t.Fatalf("slot %d: %+v", i, sl)
		}
	}
	v, err := s.SaveFrame(d)
	if err != nil {
		t.Fatal(err)
	}
	// the frame renders: photos show through the windows, the artwork covers the rest
	photos := []image.Image{media.Solid(100, 100, color.NRGBA{0, 255, 0, 255}), media.Solid(100, 100, color.NRGBA{0, 255, 0, 255}), media.Solid(100, 100, color.NRGBA{0, 255, 0, 255}), media.Solid(100, 100, color.NRGBA{0, 255, 0, 255})}
	out, err := s.tpl.Compose(*s.tpl.Get(v.ID), photos, rcZero())
	if err != nil {
		t.Fatal(err)
	}
	for i, cy := range []int{225, 615, 1005, 1395} {
		if c := out.NRGBAAt(300, cy); c.G < 250 || c.R > 5 {
			t.Fatalf("window %d must show the photo, got %v", i+1, c)
		}
	}
	for _, p := range [][2]int{{20, 20}, {300, 420}, {300, 1700}} {
		if c := out.NRGBAAt(p[0], p[1]); c.R < 190 || c.G > 40 {
			t.Fatalf("artwork must cover %v, got %v", p, c)
		}
	}
	// and guests can use it straight away
	shootWith(t, s, v.ID, "", "")
	done := finish(t, s)
	if m, _ := s.store.Media(done.ResultID); m.FrameID != v.ID || m.Width != 600 || m.Height != 1800 {
		t.Fatalf("%+v", m)
	}
}

func TestAPictureWithoutWindowsBecomesTheBackgroundWithStandardSlots(t *testing.T) {
	s, _, _ := newTestApp(t)
	for name, in := range map[string]string{"opaque": pngFrameB64(600, 1800), "specks": pngFrameB64(600, 1800, image.Rect(10, 10, 20, 20))} {
		imp, err := s.ImportPNGFrame(in, name+".png")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if imp.Mode != "background" || imp.Draft.BackgroundImage == "" || imp.Draft.Overlay != "" || imp.Draft.Photos() != 4 || len(imp.Notes) < 2 {
			t.Fatalf("%s: %+v", name, imp)
		}
		if _, err := s.SaveFrame(imp.Draft); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	// other canvas sizes get one large slot inside a margin
	imp, err := s.ImportPNGFrame(pngFrameB64(1800, 1200), "landscape")
	if err != nil || imp.Draft.WidthIn != 6 || imp.Draft.HeightIn != 4 || len(imp.Draft.Slots) != 1 {
		t.Fatalf("%v %+v", err, imp.Draft.Geometry)
	}
}

func TestPNGFrameNotesWarnAboutSizeAndStandardSizes(t *testing.T) {
	s, _, _ := newTestApp(t)
	imp, err := s.ImportPNGFrame(pngFrameB64(300, 900, image.Rect(20, 20, 280, 440)), "small")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(imp.Notes, " ")
	if imp.Draft.DPI != 150 && imp.Draft.DPI != 300 || !strings.Contains(joined, "2×6") {
		t.Fatalf("%+v", imp)
	}
	big, err := s.ImportPNGFrame(pngFrameB64(1200, 3600, image.Rect(90, 90, 1110, 810)), "hi-res")
	if err != nil || big.Draft.DPI != 600 || big.Draft.WidthIn != 2 {
		t.Fatalf("a 1200x3600 picture is a 2x6 strip at 600 dpi: %v %+v", err, big.Draft.Geometry)
	}
}

func TestPNGFrameImportRefusesBadFilesAndIsBlockedAfterMigration(t *testing.T) {
	s, _, _ := newTestApp(t)
	svg := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(`<svg onload="alert(1)"/>`))
	for name, in := range map[string]string{"svg": svg, "garbage": "!!!", "empty": ""} {
		_, err := s.ImportPNGFrame(in, "x.png")
		if ue, ok := err.(*UserError); !ok || ue.Msg == "" {
			t.Errorf("%s: %v", name, err)
		}
	}
	s.restartRequired.Store(true)
	if _, err := s.ImportPNGFrame(pngFrameB64(600, 1800, stripWindows()...), "x"); err == nil {
		t.Fatal("blocked while a restart is required")
	}
}

func TestTooManyWindowsFallBackToTheBackgroundMode(t *testing.T) {
	s, _, _ := newTestApp(t)
	var ws []image.Rectangle
	for r := 0; r < 5; r++ {
		for c := 0; c < 3; c++ { // 15 windows
			ws = append(ws, image.Rect(30+c*190, 30+r*350, 30+c*190+170, 30+r*350+320))
		}
	}
	imp, err := s.ImportPNGFrame(pngFrameB64(600, 1800, ws...), "many")
	if err != nil || imp.Mode != "background" || !strings.Contains(strings.Join(imp.Notes, " "), "at most 12") {
		t.Fatalf("%v %+v", err, imp.Notes)
	}
}

func rcZero() templates.RenderContext { return templates.RenderContext{} }
