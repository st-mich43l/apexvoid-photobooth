package app

import (
	"bytes"
	"context"
	"image"
	"os"
	"strings"
	"testing"

	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
	"github.com/st-mich43l/apexvoid-photobooth/internal/printing"
	"github.com/st-mich43l/apexvoid-photobooth/internal/templates"
)

func decodeJPEG(t *testing.T, b []byte) image.Image {
	t.Helper()
	img, err := media.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// samePixels compares a region of a with a region of b. The sheet is JPEG-encoded again, so a tiny
// codec difference is allowed; any re-render, resample or shift would be far larger.
func samePixels(a, b image.Image, ar, br image.Rectangle) bool {
	var sum, n float64
	for y := 0; y < ar.Dy(); y += 7 {
		for x := 0; x < ar.Dx(); x += 5 {
			r1, g1, b1, _ := a.At(ar.Min.X+x, ar.Min.Y+y).RGBA()
			r2, g2, b2, _ := b.At(br.Min.X+x, br.Min.Y+y).RGBA()
			for _, d := range []int{int(r1>>8) - int(r2>>8), int(g1>>8) - int(g2>>8), int(b1>>8) - int(b2>>8)} {
				if d < 0 {
					d = -d
				}
				sum += float64(d)
				n++
			}
		}
	}
	return sum/n < 1.5
}

func TestTwoStripsOnOne4x6SheetAreCopiedPixelForPixelFromTheApprovedComposition(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	shootWith(t, s, "rose-pink.strip4", "coral", "")
	done := finish(t, s)
	fp := printerOf(t, s)
	o := opts("two1", done.ResultID, "")
	o.Paper, o.TwoUp = "4x6 in", true
	// the preview shows the sheet BEFORE anything is sent
	pv, err := s.PrintPreview(o)
	if err != nil || !pv.TwoUp || !pv.TwoUpOffer || pv.ImageInches[0] < 3.99 || pv.ImageInches[0] > 4.01 || pv.ImageInches[1] < 5.99 || pv.ImageInches[1] > 6.01 {
		t.Fatalf("%v %+v", err, pv)
	}
	if fp.n() != 0 {
		t.Fatal("a preview must not print")
	}
	j, err := s.PrintMedia(o)
	if err != nil {
		t.Fatal(err)
	}
	if fp.n() != 1 || j.RenderedMediaID == "" {
		t.Fatalf("%+v", j)
	}
	sent := decodeJPEG(t, fp.files[0])
	if sent.Bounds().Dx() != 1200 || sent.Bounds().Dy() != 1800 {
		t.Fatalf("sent %v", sent.Bounds())
	}
	strip, _ := s.store.FilePath(done.ResultID, "original")
	data, err := os.ReadFile(strip)
	if err != nil {
		t.Fatal(err)
	}
	orig := decodeJPEG(t, data)
	if !samePixels(sent, orig, image.Rect(0, 0, 600, 1800), orig.Bounds()) || !samePixels(sent, orig, image.Rect(600, 0, 1200, 1800), orig.Bounds()) {
		t.Fatal("both halves must be exactly the approved strip (no re-render, no resampling)")
	}
	if _, x, _, ok := media.JPEGDensity(fp.files[0]); !ok || x != 300 {
		t.Fatal("the sheet must carry 300 DPI")
	}
	// the stored print says which frame made it
	rm, _ := s.store.Media(j.RenderedMediaID)
	if rm.Kind != "print" || rm.FrameID != "rose-pink.strip4" || rm.VariantID != "coral" || rm.Width != 1200 {
		t.Fatalf("%+v", rm)
	}
	// the finished strip is untouched
	if m, _ := s.store.Media(done.ResultID); m.Width != 600 {
		t.Fatal("the display strip must stay a 2x6 strip")
	}
}

func TestTwoUpIsOfferedOnlyWhenItFitsAndRefusedForOtherImages(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	shootWith(t, s, "plain.strip4", "", "")
	done := finish(t, s)
	o := opts("", done.ResultID, "")
	o.Paper = "4x6 in"
	if pv, err := s.PrintPreview(o); err != nil || !pv.TwoUpOffer || pv.TwoUp {
		t.Fatalf("a 2x6 strip on 4x6 paper offers two-up but does not apply it unasked: %v %+v", err, pv)
	}
	// a 4x6 sheet frame is not a strip
	_ = s.ResetShooting()
	sn := shootWith(t, s, "plain.sheet4", "", "")
	d2 := finish(t, s)
	_ = sn
	o2 := opts("", d2.ResultID, "")
	o2.Paper, o2.TwoUp = "4x6 in", true
	if _, err := s.PrintPreview(o2); err == nil || !strings.Contains(err.Error(), "narrow strips") {
		t.Fatalf("two-up of a 4x6 sheet must be refused: %v", err)
	}
	o2.TwoUp = false
	if pv, err := s.PrintPreview(o2); err != nil || pv.TwoUpOffer {
		t.Fatalf("no offer for a sheet: %v %+v", err, pv)
	}
}

func TestReprintUsesTheFinalizedFileNotAnEditedFrame(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	d, _ := s.NewFrameDraft("plain.strip4", "Reprint me")
	d.Enabled = true
	v, err := s.SaveFrame(d)
	if err != nil {
		t.Fatal(err)
	}
	shootWith(t, s, v.ID, "", "")
	done := finish(t, s)
	fp := printerOf(t, s)
	j, err := s.PrintMedia(func() PrintOptions { o := opts("r1", done.ResultID, ""); o.Paper = "4x6 in"; return o }())
	if err != nil {
		t.Fatal(err)
	}
	first := append([]byte(nil), fp.files[0]...)
	// the operator redesigns and then deletes the frame
	d2, _ := s.GetFrame(v.ID)
	d2.Background = "#ff0000"
	if _, err := s.SaveFrame(d2); err != nil {
		t.Fatal(err)
	}
	_ = s.DeleteFrame(v.ID)
	fp.setState(printing.MapSpoolerJob(0, false)) // the job left the queue
	s.prn.Poll(context.Background())
	if _, err := s.Reprint(j.ID, true); err != nil {
		t.Fatalf("reprint: %v", err)
	}
	if fp.n() != 2 || !bytes.Equal(first, fp.files[1]) {
		t.Fatal("a reprint must send exactly the file of the original job")
	}
}

func TestPrintResultUsesTheOperatorsPrinterAndTwoUpAutomatically(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	if _, err := s.PrintResult("x", "id1"); err == nil || !strings.Contains(err.Error(), "not set up") {
		t.Fatalf("no printer chosen: %v", err)
	}
	shootWith(t, s, "plain.strip4", "", "")
	done := finish(t, s)
	cfg := s.settings.Get()
	cfg.PrinterName, cfg.PrintPaper = "Lab", "4x6 in"
	_ = s.settings.Save(cfg)
	fp := printerOf(t, s)
	if _, err := s.PrintResult(done.ResultID, "tap1"); err != nil {
		t.Fatal(err)
	}
	if img := decodeJPEG(t, fp.files[0]); img.Bounds().Dx() != 1200 {
		t.Fatalf("a 2x6 strip on 4x6 paper is printed two-up automatically, got %v", img.Bounds())
	}
	// the same tap never prints twice
	if _, err := s.PrintResult(done.ResultID, "tap1"); err != nil || fp.n() != 1 {
		t.Fatalf("%v n=%d", err, fp.n())
	}
	_ = templates.DefaultFrameID
}
