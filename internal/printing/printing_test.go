package printing

import (
	"context"
	"errors"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/storage"
)

// ---- placement: real print geometry ----------------------------------------

// A 4x6 dye-sub printer: borderless, 300 dpi.
func borderless4x6() PageGeometry {
	return PageGeometry{PhysW: 1200, PhysH: 1800, PrintW: 1200, PrintH: 1800, DPIX: 300, DPIY: 300}
}

// An office inkjet on 4x6 paper: 3 mm unprintable border, 600 dpi.
func inkjet4x6() PageGeometry {
	m := int(math.Round(3.0 / 25.4 * 600))
	return PageGeometry{PhysW: 2400, PhysH: 3600, PrintW: 2400 - 2*m, PrintH: 3600 - 2*m, OffX: m, OffY: m, DPIX: 600, DPIY: 600}
}

func TestExactSizePhotoOnBorderlessPaperFillsTheSheet(t *testing.T) {
	pl := PlanPage(1200, 1800, 300, borderless4x6(), FitPrintable)
	if pl.Dest.Dx() != 1200 || pl.Dest.Dy() != 1800 || pl.Rotated || pl.ScalePct < 99.9 || pl.ScalePct > 100.1 {
		t.Fatalf("%+v", pl)
	}
	for _, m := range pl.MarginMM {
		if math.Abs(m) > 0.1 {
			t.Fatalf("no margins expected: %v", pl.MarginMM)
		}
	}
}

func TestInkjetKeepsTheWholeImageInsideThePrintableArea(t *testing.T) {
	g := inkjet4x6()
	pl := PlanPage(1200, 1800, 300, g, FitPrintable)
	if pl.Dest.Min.X < 0 || pl.Dest.Max.X > g.PrintW || pl.Dest.Min.Y < 0 || pl.Dest.Max.Y > g.PrintH {
		t.Fatalf("image leaves the printable area: %v", pl.Dest)
	}
	if pl.Src.Dx() != 1200 || pl.CroppedPct != 0 {
		t.Fatalf("fit must never crop: %+v", pl)
	}
	if pl.MarginMM[0] < 2.9 || math.Abs(pl.MarginMM[0]-pl.MarginMM[2]) > 0.3 || math.Abs(pl.MarginMM[1]-pl.MarginMM[3]) > 0.3 {
		t.Fatalf("margins must be at least the 3 mm hardware border and symmetric: %v", pl.MarginMM)
	}
}

func TestFillCoversTheSheetAndCropsTheOverflow(t *testing.T) {
	// a 3:2 landscape photo (1800x1200) on portrait 4x6 → turned and cropped
	pl := PlanPage(1800, 1200, 300, borderless4x6(), FitFill)
	if !pl.Rotated {
		t.Fatal("a landscape image on a portrait sheet must be turned")
	}
	if pl.Dest.Dx() != 1200 || pl.Dest.Dy() != 1800 {
		t.Fatalf("%v", pl.Dest)
	}
	// a 4:3 image on 3:2 paper loses a strip
	pl = PlanPage(1600, 1200, 300, PageGeometry{PhysW: 1800, PhysH: 1200, PrintW: 1800, PrintH: 1200, DPIX: 300, DPIY: 300}, FitFill)
	if pl.CroppedPct < 10 || pl.CroppedPct > 14 {
		t.Fatalf("expected ~11%% cropped, got %.1f", pl.CroppedPct)
	}
	if pl.Src.Dx() != 1600 || pl.Src.Dy() >= 1200 {
		t.Fatalf("crop must remove height: %v", pl.Src)
	}
}

func TestFillOnAPrinterThatCannotReachTheEdgesWarns(t *testing.T) {
	pl := PlanPage(1200, 1800, 300, PageGeometry{PhysW: 1200, PhysH: 1800, PrintW: 1100, PrintH: 1700, OffX: 50, OffY: 50, DPIX: 300, DPIY: 300}, FitFill)
	if pl.Warning == "" || pl.Dest.Dx() != 1100 {
		t.Fatalf("%+v", pl)
	}
	// the part of the picture that is printed lines up with the part of the sheet that is printable
	if pl.Src.Min.X <= 0 || pl.Src.Dx() >= 1200 {
		t.Fatalf("source must be trimmed in step with the destination: %v", pl.Src)
	}
}

func TestActualSizeIsCentredOnTheSheetAtItsOwnDPI(t *testing.T) {
	// a 2x6 strip (600x1800 @300) on 4x6 paper printed at 600 dpi → 1200x3600 device px, centred
	g := PageGeometry{PhysW: 2400, PhysH: 3600, PrintW: 2400, PrintH: 3600, DPIX: 600, DPIY: 600}
	pl := PlanPage(600, 1800, 300, g, FitActual)
	if pl.Dest.Dx() != 1200 || pl.Dest.Dy() != 3600 || pl.Dest.Min.X != 600 || pl.ScalePct < 99 || pl.ScalePct > 101 {
		t.Fatalf("%+v", pl)
	}
	// larger than the printable area: scaled down with a warning
	pl = PlanPage(1800, 2400, 300, inkjet4x6(), FitActual)
	if pl.Warning == "" {
		t.Fatal("expected a scaled-down warning")
	}
}

func TestTwoUpSheetAndStripFormats(t *testing.T) {
	// two 2x6 strips on one 4x6 sheet are a single 1200x1800 image: 1:1 on 4x6
	if pl := PlanPage(1200, 1800, 300, borderless4x6(), FitPrintable); pl.ScalePct < 99.9 {
		t.Fatalf("%+v", pl)
	}
	// a single 2x6 strip on 4x6 paper at "fit" is centred with 1-inch side margins
	pl := PlanPage(600, 1800, 300, borderless4x6(), FitPrintable)
	if pl.Dest.Dx() != 600 || math.Abs(pl.MarginMM[0]-25.4) > 0.3 || math.Abs(pl.MarginMM[2]-25.4) > 0.3 {
		t.Fatalf("%+v", pl)
	}
	// 5x7 image on 5x7 paper
	g57 := PageGeometry{PhysW: 1500, PhysH: 2100, PrintW: 1500, PrintH: 2100, DPIX: 300, DPIY: 300}
	if pl := PlanPage(1500, 2100, 300, g57, FitPrintable); pl.Dest.Dx() != 1500 {
		t.Fatalf("%+v", pl)
	}
}

func TestGeometryForCustomMargin(t *testing.T) {
	g := GeometryFor(Paper{Name: "4x6", WidthIn: 4, HeightIn: 6}, Landscape, 300, 3)
	if g.PhysW != 1800 || g.PhysH != 1200 || g.OffX != 35 || g.PrintW != 1730 || !g.Assumed {
		t.Fatalf("%+v", g)
	}
}

func TestPreviewDrawsTheImageWhereThePlanPutsIt(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 600, 1800))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 255, 0, 0, 255
	}
	g := borderless4x6()
	pl := PlanPage(600, 1800, 300, g, FitPrintable)
	pv := Preview(img, g, pl, 400).(*image.NRGBA)
	if pv.Bounds().Dx() != 400 || pv.Bounds().Dy() != 600 {
		t.Fatalf("%v", pv.Bounds())
	}
	if c := pv.NRGBAAt(200, 300); c.R < 250 || c.G > 5 { // centre: the red strip
		t.Fatalf("centre %v", c)
	}
	if c := pv.NRGBAAt(20, 300); c.R < 250 || c.G < 250 { // left margin: white paper
		t.Fatalf("margin %v", c)
	}
	// non-printable border is visible on a printer with margins
	g2 := inkjet4x6()
	pv2 := Preview(img, g2, PlanPage(600, 1800, 300, g2, FitPrintable), 400).(*image.NRGBA)
	if c := pv2.NRGBAAt(1, 1); c.R > 230 || c.B < 200 || c == (color.NRGBA{255, 255, 255, 255}) {
		t.Fatalf("unprintable border should be tinted: %v", c)
	}
}

func TestToBGRAIsTopDownBGRAOverWhite(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{10, 20, 30, 255})
	img.SetNRGBA(1, 0, color.NRGBA{0, 0, 0, 0}) // transparent → white
	pix, w, h := ToBGRA(img)
	if w != 2 || h != 2 || len(pix) != 16 {
		t.Fatal(w, h, len(pix))
	}
	if pix[0] != 30 || pix[1] != 20 || pix[2] != 10 {
		t.Fatalf("not BGR: %v", pix[:4])
	}
	if pix[4] != 255 || pix[5] != 255 || pix[6] != 255 {
		t.Fatalf("transparent must be white: %v", pix[4:8])
	}
}

// ---- status mapping -------------------------------------------------------

func TestSpoolerStatesAreHonest(t *testing.T) {
	cases := []struct {
		flags   uint32
		present bool
		want    string
	}{
		{JobSpooling, true, StatusQueued}, {JobPaused, true, StatusQueued}, {0, true, StatusQueued},
		{JobPrinting, true, StatusPrinting}, {JobPrinting | JobSpooling, true, StatusPrinting},
		{JobPrinted, true, StatusCompleted}, {JobComplete, true, StatusCompleted},
		{JobError, true, StatusFailed}, {JobOffline | JobPrinting, true, StatusFailed}, {JobPaperOut, true, StatusFailed},
		{JobUserIntervention, true, StatusFailed}, {JobDeleted, true, StatusFailed}, {JobDeleting, true, StatusFailed},
		{0, false, StatusRemoved},
	}
	for _, c := range cases {
		if got := MapSpoolerJob(c.flags, c.present); got.Status != c.want {
			t.Errorf("flags=0x%X present=%v: %s want %s", c.flags, c.present, got.Status, c.want)
		}
	}
	rm := MapSpoolerJob(0, false)
	if !strings.Contains(rm.Detail, "no proof") {
		t.Fatalf("removal must not claim delivery: %q", rm.Detail)
	}
	if d := MapSpoolerJob(JobPrinted, true).Detail; !strings.Contains(d, "not proof") {
		t.Fatalf("completed must not claim paper: %q", d)
	}
	if d := MapSpoolerJob(JobPaperOut, true).Detail; !strings.Contains(d, "out of paper") {
		t.Fatal(d)
	}
}

func TestPrinterAvailabilityMapping(t *testing.T) {
	if s, _ := MapPrinterStatus(0, 0); s != "ready" {
		t.Fatal(s)
	}
	if s, d := MapPrinterStatus(PrinterOffline, 0); s != "offline" || d == "" {
		t.Fatal(s, d)
	}
	if s, _ := MapPrinterStatus(0, PrinterAttrWorkOffline); s != "offline" {
		t.Fatal("'use printer offline' attribute must be offline")
	}
	if s, d := MapPrinterStatus(PrinterPaperOut|PrinterDoorOpen, 0); s != "error" || !strings.Contains(d, "out of paper") || !strings.Contains(d, "door open") {
		t.Fatal(s, d)
	}
}

func TestPaperValidation(t *testing.T) {
	two := []Paper{{ID: 119, Name: "4x6 in", WidthIn: 4, HeightIn: 6}, {ID: 9, Name: "A4", WidthIn: 8.27, HeightIn: 11.69}}
	if err := ValidateAgainst(two, 2, 6, "4x6 in"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAgainst(two, 6, 4, "4x6 in"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAgainst(two, 5, 7, "4x6 in"); err == nil {
		t.Fatal("5x7 on 4x6 accepted")
	}
	if err := ValidateAgainst(two, 4, 6, "Bogus"); err == nil || !strings.Contains(err.Error(), "unknown paper") {
		t.Fatal(err)
	}
	if !SameAspect(4, 6, 6, 4, 0.01) || SameAspect(4, 6, 5, 7, 0.01) {
		t.Fatal("aspect comparison")
	}
}

// ---- manager: lifecycle, duplicates, persistence ---------------------------

type fakeDriver struct {
	mu        sync.Mutex
	printers  []Printer
	submits   []SubmitRequest
	state     JobState
	failNext  error
	statusErr error
	seq       int
}

func newFake() *fakeDriver {
	return &fakeDriver{printers: []Printer{{Name: "Lab", Default: true, Status: "ready"}}, state: JobState{Status: StatusQueued, Detail: "Sent to the Windows print queue."}}
}
func (f *fakeDriver) Name() string { return "fake" }
func (f *fakeDriver) List(context.Context) ([]Printer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Printer(nil), f.printers...), nil
}
func (f *fakeDriver) Capabilities(context.Context, string) (Caps, error) {
	return Caps{Papers: KnownPapers, MaxCopies: 10}, nil
}
func (f *fakeDriver) Geometry(_ context.Context, _ string, p Paper, o Orientation) (PageGeometry, error) {
	return GeometryFor(p, o, 300, 0), nil
}
func (f *fakeDriver) Submit(_ context.Context, r SubmitRequest) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext != nil {
		e := f.failNext
		f.failNext = nil
		return "", e
	}
	f.seq++
	f.submits = append(f.submits, r)
	return string(rune('0' + f.seq)), nil
}
func (f *fakeDriver) Status(context.Context, string, string) (JobState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state, f.statusErr
}
func (f *fakeDriver) setState(s JobState) { f.mu.Lock(); f.state = s; f.mu.Unlock() }
func (f *fakeDriver) count() int          { f.mu.Lock(); defer f.mu.Unlock(); return len(f.submits) }

type env struct {
	m   *Manager
	drv *fakeDriver
	db  *storage.DB
	now time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db, err := storage.Open(storage.Layout{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	e := &env{drv: newFake(), db: db, now: time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)}
	e.m = NewManager(e.drv, NewSQLStore(db))
	e.m.SetClock(func() time.Time { return e.now })
	return e
}

func req(id string) PrintRequest {
	return PrintRequest{RequestID: id, MediaID: "M1", SessionID: "S1", File: "/tmp/strip.jpg", WidthIn: 2, HeightIn: 6, ImageDPI: 300,
		Printer: "Lab", Paper: Paper{Name: "4x6", WidthIn: 4, HeightIn: 6}, Copies: 1}
}

func TestPrintLifecycleAndPersistence(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	j, err := e.m.Print(ctx, req("r1"))
	if err != nil || j.Status != StatusQueued || j.NativeID == "" {
		t.Fatalf("%v %+v", err, j)
	}
	e.drv.setState(JobState{Status: StatusPrinting, Detail: "Printing."})
	if got, _ := e.m.Refresh(ctx, j.ID); got.Status != StatusPrinting {
		t.Fatalf("%+v", got)
	}
	e.drv.setState(MapSpoolerJob(0, false))
	got, _ := e.m.Refresh(ctx, j.ID)
	if got.Status != StatusRemoved {
		t.Fatalf("%+v", got)
	}
	// terminal states are not re-polled
	e.drv.setState(JobState{Status: StatusPrinting})
	if got, _ := e.m.Refresh(ctx, j.ID); got.Status != StatusRemoved {
		t.Fatal("terminal state changed")
	}
	jobs, _ := e.m.Jobs(10)
	if len(jobs) != 1 || jobs[0].ID != j.ID || jobs[0].File != "/tmp/strip.jpg" || jobs[0].PaperName != "4x6" {
		t.Fatalf("history not persisted: %+v", jobs)
	}
}

func TestSameRequestNeverPrintsTwice(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a, err := e.m.Print(ctx, req("click-1"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.m.Print(ctx, req("click-1")) // double-tap / retry of the same UI action
	if err != nil || b.ID != a.ID || e.drv.count() != 1 {
		t.Fatalf("idempotency: %v %s %s submits=%d", err, a.ID, b.ID, e.drv.count())
	}
}

func TestIdenticalPrintInsideWindowIsRefusedThenAllowed(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if _, err := e.m.Print(ctx, req("a")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.m.Print(ctx, req("b")); !errors.Is(err, ErrDuplicate) || e.drv.count() != 1 {
		t.Fatalf("a second identical print 1 s later must be refused: %v submits=%d", err, e.drv.count())
	}
	r := req("c")
	r.Copies = 2 // a different request is not a duplicate
	if _, err := e.m.Print(ctx, r); err != nil {
		t.Fatalf("%v", err)
	}
	e.now = e.now.Add(time.Minute)
	if _, err := e.m.Print(ctx, req("d")); err != nil {
		t.Fatalf("after the window the same print is allowed: %v", err)
	}
}

func TestConcurrentClicksSubmitOnce(t *testing.T) {
	e := newEnv(t)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := e.m.Print(context.Background(), req(string(rune('a'+i))))
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	if e.drv.count() != 1 {
		t.Fatalf("8 simultaneous identical prints produced %d jobs", e.drv.count())
	}
}

func TestOfflinePrinterAndBadInputAreRefusedBeforeSubmission(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.drv.printers[0].Status, e.drv.printers[0].Detail = "offline", "The printer is offline"
	if _, err := e.m.Print(ctx, req("x")); err == nil || !strings.Contains(err.Error(), "offline") || e.drv.count() != 0 {
		t.Fatalf("%v", err)
	}
	e.drv.printers[0].Status = "ready"
	r := req("y")
	r.WidthIn, r.HeightIn = 5, 7
	if _, err := e.m.Print(ctx, r); err == nil || e.drv.count() != 0 {
		t.Fatal("5x7 layout on 4x6 paper must be refused")
	}
	r = req("z")
	r.Copies = 99
	if _, err := e.m.Print(ctx, r); err == nil {
		t.Fatal("99 copies")
	}
	r = req("n")
	r.Printer = "Ghost"
	if _, err := e.m.Print(ctx, r); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatal(err)
	}
	r = req("f")
	r.File = ""
	if _, err := e.m.Print(ctx, r); err == nil {
		t.Fatal("nothing to print")
	}
}

func TestSubmissionFailureIsRecordedAndCanBeRetried(t *testing.T) {
	e := newEnv(t)
	e.drv.failNext = errors.New("spooler service is not running")
	j, err := e.m.Print(context.Background(), req("a"))
	if err == nil || j.Status != StatusFailed || !strings.Contains(j.Detail, "spooler service") {
		t.Fatalf("%v %+v", err, j)
	}
	// the failed attempt does not block a retry with a new request
	e.now = e.now.Add(time.Second)
	if _, err := e.m.Print(context.Background(), req("b")); err != nil {
		t.Fatalf("retry after failure: %v", err)
	}
}

func TestReprintNeedsConfirmationAndIsBounded(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	j, _ := e.m.Print(ctx, req("a"))
	if _, err := e.m.Reprint(ctx, j.ID, true); err == nil || !strings.Contains(err.Error(), "still in the print queue") {
		t.Fatalf("reprint while queued: %v", err)
	}
	e.drv.setState(MapSpoolerJob(JobPrinted, true))
	if _, err := e.m.Reprint(ctx, j.ID, false); !errors.Is(err, ErrNeedsConfirm) || e.drv.count() != 1 {
		t.Fatalf("unconfirmed reprint: %v", err)
	}
	for i := 1; i <= 3; i++ {
		rp, err := e.m.Reprint(ctx, j.ID, true)
		if err != nil || rp.ParentID != j.ID || rp.Attempt != i+1 {
			t.Fatalf("reprint %d: %v %+v", i, err, rp)
		}
		e.drv.setState(MapSpoolerJob(JobPrinted, true))
		e.m.Refresh(ctx, rp.ID)
	}
	if _, err := e.m.Reprint(ctx, j.ID, true); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("limit: %v", err)
	}
	hist, _ := e.m.Jobs(20)
	if len(hist) != 4 {
		t.Fatalf("reprint history: %d", len(hist))
	}
	if _, err := e.m.Reprint(ctx, "nope", true); err == nil {
		t.Fatal("unknown job")
	}
}

func TestRestartRecovery(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	// a job that was in the queue when the app stopped
	a, _ := e.m.Print(ctx, req("a"))
	// a job caught between "insert" and "submit" (crash)
	e.now = e.now.Add(time.Minute)
	stuck := Job{ID: "Pstuck", Kind: "print", Printer: "Lab", PaperName: "4x6", Copies: 1, Status: StatusSubmitting, Attempt: 1, CreatedAt: e.now, UpdatedAt: e.now}
	_ = NewSQLStore(e.db).Insert(stuck)

	e.drv.setState(JobState{Status: StatusPrinting, Detail: "Printing."})
	e2 := NewManager(e.drv, NewSQLStore(e.db)) // "restart": a fresh manager over the same database
	changed := e2.Recover(ctx)
	if len(changed) != 2 {
		t.Fatalf("%+v", changed)
	}
	got, _ := e2.Jobs(10)
	byID := map[string]Job{}
	for _, j := range got {
		byID[j.ID] = j
	}
	if byID[a.ID].Status != StatusPrinting {
		t.Fatalf("a job still in the spooler is tracked again: %+v", byID[a.ID])
	}
	if s := byID["Pstuck"]; s.Status != StatusUnknown || !strings.Contains(s.Detail, "Check the Windows print queue") {
		t.Fatalf("an interrupted submission must be 'unknown', never completed: %+v", s)
	}
	// job vanished from the queue while closed → removed, with a caution
	e.drv.setState(MapSpoolerJob(0, false))
	b, _ := e.m.Print(ctx, func() PrintRequest { r := req("b"); r.Copies = 2; return r }())
	e3 := NewManager(e.drv, NewSQLStore(e.db))
	e3.Recover(ctx)
	if j, _ := e3.Refresh(ctx, b.ID); j.Status != StatusRemoved || !strings.Contains(j.Detail, "cannot be confirmed") {
		t.Fatalf("%+v", j)
	}
	// unreadable queue → unknown
	e.drv.statusErr = errors.New("spooler unavailable")
	c, _ := e.m.Print(ctx, func() PrintRequest { r := req("c"); r.Copies = 3; return r }())
	_ = c
	e4 := NewManager(e.drv, NewSQLStore(e.db))
	e4.Recover(ctx)
	all, _ := e4.Jobs(20)
	for _, j := range all {
		if j.Printer == "Lab" && j.Copies == 3 && j.Status != StatusUnknown {
			t.Fatalf("%+v", j)
		}
	}
}

func TestPollReportsOnlyChanges(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, _ = e.m.Print(ctx, req("a"))
	if ch := e.m.Poll(ctx); len(ch) != 0 {
		t.Fatalf("nothing changed yet: %+v", ch)
	}
	e.drv.setState(JobState{Status: StatusPrinting, Detail: "Printing."})
	if ch := e.m.Poll(ctx); len(ch) != 1 || ch[0].Status != StatusPrinting {
		t.Fatalf("%+v", ch)
	}
	if ch := e.m.Poll(ctx); len(ch) != 0 {
		t.Fatal("unchanged job reported again")
	}
}

func TestFailedJobThatLeavesTheQueueStaysFailed(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	j, _ := e.m.Print(ctx, req("a"))
	e.drv.setState(MapSpoolerJob(JobPaperOut, true))
	if got, _ := e.m.Refresh(ctx, j.ID); got.Status != StatusFailed {
		t.Fatalf("%+v", got)
	}
	// Refresh does not poll terminal jobs, so a later "removed" can never mask the failure
	e.drv.setState(MapSpoolerJob(0, false))
	if got, _ := e.m.Refresh(ctx, j.ID); got.Status != StatusFailed {
		t.Fatalf("failure was masked: %+v", got)
	}
}

func TestDatabaseUpgradesFromVersion1(t *testing.T) {
	root := t.TempDir()
	db, _ := storage.Open(storage.Layout{Root: root})
	_, _ = db.Exec(`DROP TABLE print_jobs`)
	_, _ = db.Exec(`PRAGMA user_version=1`)
	db.Close()
	db2, err := storage.Open(storage.Layout{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	if err := NewSQLStore(db2).Insert(Job{ID: "P1", Printer: "x", Status: StatusQueued, CreatedAt: time.Now(), UpdatedAt: time.Now(), Copies: 1, Attempt: 1}); err != nil {
		t.Fatalf("print_jobs missing after upgrade: %v", err)
	}
}

func TestParseLpstat(t *testing.T) {
	l := parseLpstat("printer HP_1 is idle.  enabled since Mon\nprinter DNP_2 disabled since Mon\nsystem default destination: DNP_2\n")
	if len(l) != 2 || !l[1].Default || l[1].Status != "offline" || l[0].Status != "ready" {
		t.Fatalf("%+v", l)
	}
}

var _ = filepath.Join
var _ = os.Remove
