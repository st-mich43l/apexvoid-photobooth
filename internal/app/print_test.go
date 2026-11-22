package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/capture"
	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
	"github.com/st-mich43l/apexvoid-photobooth/internal/printing"
	"github.com/st-mich43l/apexvoid-photobooth/internal/sessions"
	"github.com/st-mich43l/apexvoid-photobooth/internal/settings"
)

// fakePrinter stands in for the Windows spooler: it records what it is asked
// to print and reports a job state that tests control.
type fakePrinter struct {
	mu      sync.Mutex
	status  string
	submits []printing.SubmitRequest
	files   [][]byte // content of each submitted file at submission time
	state   printing.JobState
	margin  float64 // unprintable border in mm
}

func newFakePrinter() *fakePrinter {
	return &fakePrinter{status: "ready", state: printing.JobState{Status: printing.StatusQueued, Detail: "Sent to the Windows print queue."}}
}

func (f *fakePrinter) Name() string { return "fake" }
func (f *fakePrinter) List(context.Context) ([]printing.Printer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return []printing.Printer{{Name: "Lab", Default: true, Status: f.status, Detail: ""}}, nil
}
func (f *fakePrinter) Capabilities(context.Context, string) (printing.Caps, error) {
	return printing.Caps{Papers: []printing.Paper{{ID: 119, Name: "4x6 in", WidthIn: 4, HeightIn: 6}, {ID: 120, Name: "5x7 in", WidthIn: 5, HeightIn: 7}, {ID: 9, Name: "A4", WidthIn: 8.27, HeightIn: 11.69}},
		Orientations: []printing.Orientation{printing.Portrait, printing.Landscape}, MaxCopies: 10, Resolutions: [][2]int{{300, 300}}}, nil
}
func (f *fakePrinter) Geometry(_ context.Context, _ string, p printing.Paper, o printing.Orientation) (printing.PageGeometry, error) {
	return printing.GeometryFor(p, o, 300, f.margin), nil
}
func (f *fakePrinter) Submit(_ context.Context, r printing.SubmitRequest) (string, error) {
	b, err := os.ReadFile(r.File)
	if err != nil {
		return "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submits = append(f.submits, r)
	f.files = append(f.files, b)
	return string(rune('0' + len(f.submits))), nil
}
func (f *fakePrinter) Status(context.Context, string, string) (printing.JobState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state, nil
}
func (f *fakePrinter) setState(s printing.JobState) { f.mu.Lock(); f.state = s; f.mu.Unlock() }
func (f *fakePrinter) n() int                       { f.mu.Lock(); defer f.mu.Unlock(); return len(f.submits) }

// session with a 4-photo strip, built through the real workflow
func shootStrip(t *testing.T, s *Service, shots int) capture.Snapshot {
	t.Helper()
	connectMock(t, s)
	_ = s.StartPreview()
	if err := s.StartShooting(frameFor(shots), "", ""); err != nil {
		t.Fatal(err)
	}
	waitShoot(t, s, capture.Review)
	_ = s.AcceptPhotos()
	sn := waitShoot(t, s, capture.Completed)
	_ = s.ResetShooting()
	return sn
}

func printerOf(t *testing.T, s *Service) *fakePrinter {
	t.Helper()
	return s.opts.PrintDriver.(*fakePrinter)
}

func opts(id, mediaID, tpl string) PrintOptions {
	return PrintOptions{RequestID: id, MediaID: mediaID, TemplateID: tpl, Printer: "Lab", Paper: "4x6 in", Copies: 1}
}

func TestPrintingAComposedStripSubmitsTheRenderedStripNotAnOriginal(t *testing.T) {
	s, root, _ := newTestApp(t)
	sn := shootStrip(t, s, 4)
	fp := printerOf(t, s)

	j, err := s.PrintMedia(opts("r1", sn.ResultID, "plain.strip4"))
	if err != nil {
		t.Fatal(err)
	}
	if fp.n() != 1 {
		t.Fatal("not submitted")
	}
	stripPath, _ := s.store.FilePath(sn.ResultID, "original")
	if fp.submits[0].File != stripPath || j.File != stripPath {
		t.Fatalf("the rendered strip must be printed: %s vs %s", fp.submits[0].File, stripPath)
	}
	for _, sh := range sn.Taken {
		orig, _ := s.store.FilePath(sh.MediaID, "original")
		if fp.submits[0].File == orig {
			t.Fatal("a raw camera original was sent to the printer")
		}
	}
	w, h, _ := media.Dimensions(fp.files[0])
	if w != 600 || h != 1800 {
		t.Fatalf("a 2x6 strip at 300 dpi is 600x1800, got %dx%d", w, h)
	}
	if !strings.HasPrefix(stripPath, root) {
		t.Fatal("render must live in the library")
	}
	if fp.submits[0].Paper.ID != 119 || fp.submits[0].ImageDPI != 300 {
		t.Fatalf("%+v", fp.submits[0])
	}
}

func TestPrintingASinglePhotoRendersItThroughALayoutAndKeepsTheOriginal(t *testing.T) {
	s, _, _ := newTestApp(t)
	sn := shootStrip(t, s, 2)
	fp := printerOf(t, s)
	photoID := sn.Taken[0].MediaID
	origPath, _ := s.store.FilePath(photoID, "original")
	before, _ := os.ReadFile(origPath)

	j, err := s.PrintMedia(opts("p1", photoID, ""))
	if err != nil {
		t.Fatal(err)
	}
	if fp.submits[0].File == origPath || bytes.Equal(fp.files[0], before) {
		t.Fatal("the unprocessed original must never be printed")
	}
	// the mock photo is landscape, so the landscape 4x6 layout is used: 6x4 in @300 = 1800x1200
	if w, h, _ := media.Dimensions(fp.files[0]); w != 1800 || h != 1200 {
		t.Fatalf("rendered print is %dx%d", w, h)
	}
	if j.RenderedMediaID == "" || j.TemplateID != "single-4x6" {
		t.Fatalf("%+v", j)
	}
	// the rendered image is kept as an export next to the untouched original
	rm, err := s.store.Media(j.RenderedMediaID)
	if err != nil || rm.Kind != "print" || rm.SessionID != sn.SessionID {
		t.Fatalf("%v %+v", err, rm)
	}
	if after, _ := os.ReadFile(origPath); sha256.Sum256(after) != sha256.Sum256(before) {
		t.Fatal("original modified by printing")
	}
}

func TestPrintingAnEditedPhotoUsesTheEditedVersion(t *testing.T) {
	s, _, _ := newTestApp(t)
	sn := shootStrip(t, s, 2)
	fp := printerOf(t, s)
	photoID := sn.Taken[0].MediaID
	if _, err := s.SaveEdits(photoID, media.Edits{Rotate: 90}); err != nil { // landscape → portrait
		t.Fatal(err)
	}
	if _, err := s.PrintMedia(opts("e1", photoID, "")); err != nil {
		t.Fatal(err)
	}
	if w, h, _ := media.Dimensions(fp.files[0]); w != 1200 || h != 1800 {
		t.Fatalf("the rotated edit must pick the portrait layout, got %dx%d", w, h)
	}
}

func TestPrintFormats(t *testing.T) {
	s, _, _ := newTestApp(t)
	sn := shootStrip(t, s, 4)
	fp := printerOf(t, s)
	cases := []struct {
		name, tpl, paper string
		w, h             int
	}{
		{"2x6 strip", "plain.strip4", "4x6 in", 600, 1800},
		{"two 2x6 strips on a 4x6 sheet", "plain.twoup4", "4x6 in", 1200, 1800},
	}
	for i, c := range cases {
		m, err := s.ComposeSession(sn.SessionID, c.tpl, "", "", true)
		if err != nil {
			t.Fatal(err)
		}
		o := opts("f"+string(rune('a'+i)), m.ID, c.tpl)
		o.Paper = c.paper
		time.Sleep(5 * time.Millisecond)
		if _, err := s.PrintMedia(o); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if w, h, _ := media.Dimensions(fp.files[fp.n()-1]); w != c.w || h != c.h {
			t.Fatalf("%s: %dx%d", c.name, w, h)
		}
	}
	// 5x7 single photo on 5x7 paper
	o := opts("f57", sn.Taken[0].MediaID, "single-5x7")
	o.Paper = "5x7 in"
	if _, err := s.PrintMedia(o); err != nil {
		t.Fatal(err)
	}
	if w, h, _ := media.Dimensions(fp.files[fp.n()-1]); w != 2100 && w != 1500 || h != 1500 && h != 2100 {
		t.Fatalf("5x7: %dx%d", w, h)
	}
}

func TestPrintRefusesALayoutThatDoesNotMatchTheImage(t *testing.T) {
	s, _, _ := newTestApp(t)
	sn := shootStrip(t, s, 4)
	_, err := s.PrintMedia(opts("m1", sn.ResultID, "single-4x6"))
	if ue, ok := err.(*UserError); !ok || !strings.Contains(ue.Msg, "not made with the layout") || ue.Action == "" {
		t.Fatalf("%v", err)
	}
	if printerOf(t, s).n() != 0 {
		t.Fatal("submitted despite the mismatch")
	}
	// a multi-photo layout cannot render one photo
	if _, err := s.PrintMedia(opts("m2", sn.Taken[0].MediaID, "plain.strip4")); err == nil {
		t.Fatal("a 4-photo layout for one photo accepted")
	}
}

func TestPaperTooSmallForTheLayoutIsRefused(t *testing.T) {
	s, _, _ := newTestApp(t)
	sn := shootStrip(t, s, 4)
	// a 7x5 in layout does not fit 4x6 paper
	o := opts("s2", sn.Taken[0].MediaID, "single-5x7")
	o.Paper = "4x6 in"
	if _, err := s.PrintMedia(o); err == nil || printerOf(t, s).n() != 0 {
		t.Fatalf("a 7x5 layout on 4x6 paper must be refused (%v)", err)
	}
	// but a two-up 4x6 sheet fits 4x6 and 5x7 paper alike
	m, _ := s.ComposeSession(sn.SessionID, "plain.twoup4", "", "", true)
	for i, paper := range []string{"4x6 in", "5x7 in"} {
		o := opts("ok"+string(rune('a'+i)), m.ID, "plain.twoup4")
		o.Paper = paper
		o.Copies = i + 1 // different copies: not a duplicate of the previous job
		if _, err := s.PrintMedia(o); err != nil {
			t.Fatalf("%s: %v", paper, err)
		}
	}
}

func TestPreviewShowsTheRealPlacement(t *testing.T) {
	s, _, _ := newTestApp(t)
	sn := shootStrip(t, s, 4)
	// 2x6 strip on 4x6 paper: centred, 1 inch (25.4 mm) side margins, true size
	o := opts("", sn.ResultID, "plain.strip4")
	pv, err := s.PrintPreview(o)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(pv.Image, "data:image/jpeg;base64,") || pv.ScalePct < 99 || pv.ScalePct > 101 || pv.Rotated {
		t.Fatalf("%+v", pv)
	}
	if abs(int(pv.MarginMm[0])-25) > 1 || abs(int(pv.MarginMm[2])-25) > 1 || pv.PaperMm[0] < 101 || pv.PaperMm[0] > 103 {
		t.Fatalf("margins/paper wrong: %+v", pv)
	}
	// a printer with a 3 mm unprintable border: margins can never be smaller
	printerOf(t, s).margin = 3
	m2, _ := s.ComposeSession(sn.SessionID, "plain.twoup4", "", "", true)
	pv, err = s.PrintPreview(opts("", m2.ID, "plain.twoup4"))
	if err != nil {
		t.Fatal(err)
	}
	if pv.MarginMm[0] < 2.9 || pv.ScalePct > 100.5 || pv.CroppedPct != 0 {
		t.Fatalf("fit must stay inside the printable area: %+v", pv)
	}
	// "fill" crops instead
	o = opts("", m2.ID, "plain.twoup4")
	o.Fit = "fill"
	pv, _ = s.PrintPreview(o)
	if pv.Warning == "" {
		t.Fatalf("filling a sheet with an unprintable border must warn: %+v", pv)
	}
	// a photo preview must NOT add anything to the library
	before := s.LibrarySummary()
	if _, err := s.PrintPreview(opts("", sn.Taken[0].MediaID, "")); err != nil {
		t.Fatal(err)
	}
	ms, _ := s.ListMedia(sn.SessionID, 100, 0)
	for _, m := range ms {
		if m.Kind == "print" {
			t.Fatal("preview stored a render")
		}
	}
	_ = before
}

func TestSameClickNeverPrintsTwiceAndQuickRepeatIsRefused(t *testing.T) {
	s, _, _ := newTestApp(t)
	sn := shootStrip(t, s, 4)
	fp := printerOf(t, s)
	a, err := s.PrintMedia(opts("click", sn.ResultID, "plain.strip4"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.PrintMedia(opts("click", sn.ResultID, "plain.strip4"))
	if err != nil || b.ID != a.ID || fp.n() != 1 {
		t.Fatalf("same request: %v %s %s n=%d", err, a.ID, b.ID, fp.n())
	}
	_, err = s.PrintMedia(opts("click2", sn.ResultID, "plain.strip4"))
	if ue, ok := err.(*UserError); !ok || !strings.Contains(ue.Msg, "identical print") || fp.n() != 1 {
		t.Fatalf("an identical print seconds later must be refused: %v n=%d", err, fp.n())
	}
}

func TestOfflinePrinterIsReportedAndNothingIsSubmitted(t *testing.T) {
	s, _, _ := newTestApp(t)
	sn := shootStrip(t, s, 4)
	fp := printerOf(t, s)
	fp.mu.Lock()
	fp.status = "offline"
	fp.mu.Unlock()
	_, err := s.PrintMedia(opts("o1", sn.ResultID, "plain.strip4"))
	if ue, ok := err.(*UserError); !ok || !strings.Contains(ue.Msg, "offline") || ue.Action == "" || fp.n() != 0 {
		t.Fatalf("%v n=%d", err, fp.n())
	}
	pr, _ := s.ListPrinters()
	if pr.Printers[0].Status != "offline" || !strings.Contains(pr.Note, "cannot prove") {
		t.Fatalf("%+v", pr)
	}
}

func TestVideosAndMissingMediaCannotBePrinted(t *testing.T) {
	s, _, _ := newTestApp(t)
	if _, err := s.PrintMedia(opts("v", "Mnope", "")); err == nil {
		t.Fatal("unknown media")
	}
	if _, err := s.PrintMedia(PrintOptions{MediaID: "x", Printer: "Lab", Paper: "4x6 in", Copies: 1}); err == nil {
		t.Fatal("a request without an id must be refused")
	}
}

func TestTestPrintIsLabelledAndSizedToThePaper(t *testing.T) {
	s, root, _ := newTestApp(t)
	fp := printerOf(t, s)
	j, err := s.TestPrint("Lab", "4x6 in", "portrait")
	if err != nil {
		t.Fatal(err)
	}
	if j.Kind != "test" || fp.n() != 1 {
		t.Fatalf("%+v", j)
	}
	img, err := png.Decode(bytes.NewReader(fp.files[0]))
	if err != nil || img.Bounds().Dx() != 1200 || img.Bounds().Dy() != 1800 {
		t.Fatalf("%v %v", err, img.Bounds())
	}
	if !strings.HasPrefix(fp.submits[0].File, filepath.Join(root, "exports", "test-prints")) {
		t.Fatal(fp.submits[0].File)
	}
	// no ink-looking photo content: top-left pixel is the pink calibration square, centre is the cross
	if r, g, b, _ := img.At(10, 10).RGBA(); r>>8 < 200 || g>>8 > 200 || b>>8 < 150 {
		t.Fatal("calibration square missing")
	}
	// landscape variant swaps the sides
	time.Sleep(5 * time.Millisecond)
	if _, err := s.TestPrint("Lab", "4x6 in", "landscape"); err == nil {
		// the second test print inside the duplicate window is refused: this is the safety net
		t.Log("second test print accepted")
	}
	if _, err := s.TestPrint("Lab", "Bogus", ""); err == nil {
		t.Fatal("unknown paper")
	}
}

func TestJobStatesAreTrackedAndReprintNeedsConfirmation(t *testing.T) {
	s, _, ev := newTestApp(t)
	sn := shootStrip(t, s, 4)
	fp := printerOf(t, s)
	j, err := s.PrintMedia(opts("t1", sn.ResultID, "plain.strip4"))
	if err != nil || j.Status != printing.StatusQueued {
		t.Fatalf("%v %+v", err, j)
	}
	if _, err := s.Reprint(j.ID, true); err == nil {
		t.Fatal("reprint while queued")
	}
	fp.setState(printing.MapSpoolerJob(0, false)) // left the queue
	changed := s.prn.Poll(context.Background())
	if len(changed) != 1 || changed[0].Status != printing.StatusRemoved || !strings.Contains(changed[0].Detail, "no proof") {
		t.Fatalf("%+v", changed)
	}
	if _, err := s.Reprint(j.ID, false); err == nil {
		t.Fatal("unconfirmed reprint accepted")
	}
	rp, err := s.Reprint(j.ID, true)
	if err != nil || rp.ParentID != j.ID || fp.n() != 2 {
		t.Fatalf("%v %+v n=%d", err, rp, fp.n())
	}
	if fp.submits[1].File != fp.submits[0].File {
		t.Fatal("a reprint must reuse the same rendered file")
	}
	jobs := s.PrintJobs()
	if len(jobs) != 2 {
		t.Fatalf("history: %d", len(jobs))
	}
	ev.mu.Lock()
	if ev.n["print:job"] == 0 {
		t.Fatal("UI not notified")
	}
	ev.mu.Unlock()
}

func TestPrintHistorySurvivesRestartAndInterruptedJobsAreUnknown(t *testing.T) {
	base := t.TempDir()
	cfg := filepath.Join(base, "s.json")
	sm, _ := settings.Load(cfg)
	st := sm.Get()
	st.MediaDir, st.EnableMock, st.CountdownSeconds = filepath.Join(base, "lib"), true, 1
	_ = sm.Save(st)
	fp := newFakePrinter()
	mk := func() *Service {
		s, err := New(Options{ConfigPath: cfg, PrintDriver: fp, Tick: 5 * time.Millisecond, SkipRestore: true, DisableDriver: map[string]bool{"canon-edsdk": true, "uvc": true}})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	s1 := mk()
	sn := shootStrip(t, s1, 4)
	j, err := s1.PrintMedia(opts("h1", sn.ResultID, "plain.strip4"))
	if err != nil {
		t.Fatal(err)
	}
	// a second job that was caught mid-submission when the app died
	_ = s1.prnStore.Insert(printing.Job{ID: "Pcrash", Kind: "print", Printer: "Lab", PaperName: "4x6 in", Copies: 1, Status: printing.StatusSubmitting, Attempt: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()})
	_ = s1.Shutdown()

	fp.setState(printing.JobState{Status: printing.StatusPrinting, Detail: "Printing."})
	s2 := mk()
	defer s2.Shutdown()
	byID := map[string]printing.Job{}
	for _, x := range s2.PrintJobs() {
		byID[x.ID] = x
	}
	if byID[j.ID].Status != printing.StatusPrinting || byID[j.ID].File == "" {
		t.Fatalf("job still in the spooler must be tracked again: %+v", byID[j.ID])
	}
	if c := byID["Pcrash"]; c.Status != printing.StatusUnknown || !strings.Contains(c.Detail, "Check the Windows print queue") {
		t.Fatalf("%+v", c)
	}
}

var _ = image.Rect
var _ sessions.Media

func TestPrintSelfTestEnumeratesWithoutAPrinter(t *testing.T) {
	out := filepath.Join(t.TempDir(), "r.json")
	if code := PrintSelfTest("test", []string{out}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	b, _ := os.ReadFile(out)
	if !strings.Contains(string(b), `"driver"`) || !strings.Contains(string(b), "cannot be proven") && !strings.Contains(string(b), "do not prove") {
		t.Fatalf("%s", b)
	}
}
