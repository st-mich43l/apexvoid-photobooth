package app

import (
	"os"
	"strings"
	"testing"

	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
)

func setDPI(t *testing.T, s *Service, dpi int) {
	t.Helper()
	cfg := s.settings.Get()
	cfg.OutputDPI = dpi
	if err := s.settings.Save(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestSixHundredDPIProducesExactPixelsDensityAndSnapshot(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	setDPI(t, s, 600)
	// the frame list tells the operator the real output size
	for _, f := range s.ListFrames(false).Frames {
		if f.ID == "rose-pink.strip4" && (f.DPI != 600 || f.PixelW != 1200 || f.PixelH != 3600) {
			t.Fatalf("%+v", f)
		}
	}
	shootWith(t, s, "rose-pink.strip4", "", "")
	done := finish(t, s)
	m, _ := s.store.Media(done.ResultID)
	if m.Width != 1200 || m.Height != 3600 {
		t.Fatalf("2x6 at 600 dpi is 1200x3600, got %dx%d", m.Width, m.Height)
	}
	p, _ := s.store.FilePath(m.ID, "original")
	data := mustReadFile(t, p)
	if u, x, y, ok := media.JPEGDensity(data); !ok || u != 1 || x != 600 || y != 600 {
		t.Fatalf("the JPEG must say 600 dpi: %v %v %v", u, x, y)
	}
	snap, _, err := readSessionSnapshot(s, done.SessionID)
	if err != nil || snap != 600 {
		t.Fatalf("the snapshot must keep the resolution the session used: %v %d", err, snap)
	}
	// changing the setting afterwards does not change that session
	setDPI(t, s, 300)
	id, err := s.finalizeStrip(done.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if m2, _ := s.store.Media(id); m2.Width != 1200 {
		t.Fatalf("an existing session keeps its 600 dpi, got %d px wide", m2.Width)
	}
	// and a new session uses the new setting
	_ = s.ResetShooting()
	shootWith(t, s, "rose-pink.strip4", "", "")
	d2 := finish(t, s)
	if m3, _ := s.store.Media(d2.ResultID); m3.Width != 600 {
		t.Fatalf("the next session is 300 dpi, got %d", m3.Width)
	}
}

func TestSettingAppliesToBuiltInFramesOnlyAndCustomFramesKeepTheirOwnDPI(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	d, _ := s.NewFrameDraft("plain.strip2", "Fixed 300")
	d.Enabled = true
	v, err := s.SaveFrame(d)
	if err != nil || v.DPI != 300 {
		t.Fatalf("%v %+v", err, v)
	}
	setDPI(t, s, 600)
	shootWith(t, s, v.ID, "", "")
	done := finish(t, s)
	if m, _ := s.store.Media(done.ResultID); m.Width != 600 {
		t.Fatalf("a custom frame keeps the DPI chosen in the builder, got %d px", m.Width)
	}
	// a custom frame saved at 600 dpi is 600 whatever the setting says
	setDPI(t, s, 300)
	d2, _ := s.GetFrame(v.ID)
	d2.DPI = 600
	if _, err := s.SaveFrame(d2); err != nil {
		t.Fatal(err)
	}
	_ = s.ResetShooting()
	shootWith(t, s, v.ID, "", "")
	d3 := finish(t, s)
	if m, _ := s.store.Media(d3.ResultID); m.Width != 1200 {
		t.Fatalf("custom 600 dpi frame: %d px", m.Width)
	}
}

func TestInvalidResolutionIsRejected(t *testing.T) {
	s, _, _ := newTestApp(t)
	cfg := s.settings.Get()
	for _, bad := range []int{150, 450, 1200, -1} {
		cfg.OutputDPI = bad
		if _, err := s.SaveSettings(cfg); err == nil {
			t.Fatalf("%d dpi accepted", bad)
		}
	}
	cfg.OutputDPI = 600
	if _, err := s.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestPrintingA600DPIStripKeepsTrueSizeOnA300AndA600DPIPrinter(t *testing.T) {
	s, _, _ := newTestApp(t)
	setDPI(t, s, 600)
	sn := shootStrip(t, s, 4)
	fp := printerOf(t, s)
	for _, printerDPI := range []int{300, 600} {
		fp.dpi = printerDPI
		o := opts("", sn.ResultID, "")
		o.Paper, o.Fit = "4x6 in", "actual"
		pv, err := s.PrintPreview(o)
		if err != nil {
			t.Fatal(err)
		}
		// 1200x3600 px at 600 dpi is 2x6 in: true size on the 4x6 sheet, 1 inch side margins, whatever the printer resolution
		if pv.ImageInches[0] < 1.99 || pv.ImageInches[0] > 2.01 || pv.ImageInches[1] < 5.99 || pv.ImageInches[1] > 6.01 || pv.ScalePct < 99 || pv.ScalePct > 101 {
			t.Fatalf("printer %d dpi: %+v", printerDPI, pv)
		}
		if abs(int(pv.MarginMm[0])-25) > 1 || abs(int(pv.MarginMm[2])-25) > 1 {
			t.Fatalf("printer %d dpi: margins %+v", printerDPI, pv.MarginMm)
		}
	}
}

func TestTwoUpAt600DPIIs2400x3600With600DPIMetadata(t *testing.T) {
	s, _, _ := newTestApp(t)
	setDPI(t, s, 600)
	sn := shootStrip(t, s, 4)
	fp := printerOf(t, s)
	o := opts("tu600", sn.ResultID, "")
	o.Paper, o.TwoUp = "4x6 in", true
	if _, err := s.PrintMedia(o); err != nil {
		t.Fatal(err)
	}
	img := decodeJPEG(t, fp.files[0])
	if img.Bounds().Dx() != 2400 || img.Bounds().Dy() != 3600 {
		t.Fatalf("two 2x6 strips at 600 dpi are 2400x3600, got %v", img.Bounds())
	}
	if _, x, _, ok := media.JPEGDensity(fp.files[0]); !ok || x != 600 {
		t.Fatalf("sheet density %d", x)
	}
	// it still fits the 4x6 paper (fit keeps it inside; scale is 100 % of true size)
	pv, err := s.PrintPreview(func() PrintOptions { p := opts("", sn.ResultID, ""); p.Paper, p.TwoUp = "4x6 in", true; return p }())
	if err != nil || pv.ImageInches[0] < 3.99 || pv.ImageInches[0] > 4.01 || !strings.Contains(pv.Image, "base64") {
		t.Fatalf("%v %+v", err, pv)
	}
}

func mustReadFile(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// readSessionSnapshot returns the DPI stored in the session's frame snapshot.
func readSessionSnapshot(s *Service, sessionID string) (int, string, error) {
	t, _, _, ref, err := s.sessionFrame(sessionID)
	return t.DPI, ref.Dir, err
}
