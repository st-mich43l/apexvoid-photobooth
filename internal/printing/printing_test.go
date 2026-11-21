package printing

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fake struct {
	submits int
	status  string
	failing error
}

func (f *fake) Name() string { return "fake" }
func (f *fake) List(context.Context) ([]Printer, error) {
	return []Printer{{Name: "P1", Status: "ready"}}, nil
}
func (f *fake) Submit(context.Context, string, string, Options) (string, error) {
	if f.failing != nil {
		return "", f.failing
	}
	f.submits++
	return "n" + string(rune('0'+f.submits)), nil
}
func (f *fake) Status(context.Context, string) (string, string, error) { return f.status, "", nil }

func TestValidatePaper(t *testing.T) {
	ok := []struct {
		w, h  float64
		paper string
	}{{2, 6, "4x6"}, {4, 6, "4x6"}, {6, 4, "4x6"}, {4, 6, "5x7"}}
	for _, c := range ok {
		if err := ValidatePaper(c.w, c.h, c.paper); err != nil {
			t.Errorf("%v: %v", c, err)
		}
	}
	if ValidatePaper(5, 7, "4x6") == nil {
		t.Error("5x7 on 4x6 accepted")
	}
	if err := ValidatePaper(4, 6, "Bogus"); err == nil || !strings.Contains(err.Error(), "unknown paper") {
		t.Errorf("%v", err)
	}
}

func TestPrintValidatesBeforeSubmitting(t *testing.T) {
	f := &fake{status: JobCompleted}
	s := NewSpooler(f)
	ctx := context.Background()
	if _, err := s.Print(ctx, "P1", "a.jpg", "m", 5, 7, Options{Paper: "4x6", Copies: 1}); err == nil || f.submits != 0 {
		t.Fatal("wrong paper must not reach the printer")
	}
	if _, err := s.Print(ctx, "", "a.jpg", "m", 4, 6, Options{Paper: "4x6", Copies: 1}); err == nil {
		t.Fatal("no printer accepted")
	}
	if _, err := s.Print(ctx, "P1", "a.jpg", "m", 4, 6, Options{Paper: "4x6", Copies: 99}); err == nil {
		t.Fatal("99 copies accepted")
	}
	j, err := s.Print(ctx, "P1", "a.jpg", "m", 4, 6, Options{Paper: "4x6", Copies: 1})
	if err != nil || j.Status != JobQueued || f.submits != 1 {
		t.Fatalf("%v %+v", err, j)
	}
}

func TestReprintIsControlled(t *testing.T) {
	f := &fake{status: JobPrinting}
	s := NewSpooler(f)
	ctx := context.Background()
	j, _ := s.Print(ctx, "P1", "a.jpg", "m", 4, 6, Options{Paper: "4x6", Copies: 1})
	if _, err := s.Reprint(ctx, j.ID); err == nil {
		t.Fatal("reprint while printing accepted")
	}
	f.status = JobCompleted
	for i := 0; i < 3; i++ {
		if _, err := s.Reprint(ctx, j.ID); err != nil {
			t.Fatalf("reprint %d: %v", i+1, err)
		}
	}
	if _, err := s.Reprint(ctx, j.ID); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("limit not enforced: %v", err)
	}
	if _, err := s.Reprint(ctx, "P9999"); err == nil {
		t.Fatal("unknown job")
	}
}

func TestDriverFailureLeavesNoJob(t *testing.T) {
	f := &fake{failing: errors.New("spooler down")}
	s := NewSpooler(f)
	if _, err := s.Print(context.Background(), "P1", "a.jpg", "m", 4, 6, Options{Paper: "4x6", Copies: 1}); err == nil || len(s.Jobs()) != 0 {
		t.Fatal("failed submission must not create a job")
	}
}

func TestWindowsSubmitIsHonestlyNotImplemented(t *testing.T) {
	_, err := windowsDriver{}.Submit(context.Background(), "p", "f", Options{})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatal(err)
	}
}

func TestParsers(t *testing.T) {
	ps, err := parseWindowsPrinters(`[{"Name":"Canon SELPHY CP1500","Default":true,"PrinterStatus":0},{"Name":"PDF","Default":false,"PrinterStatus":7}]`)
	if err != nil || len(ps) != 2 || ps[0].Status != "ready" || !ps[0].Default || ps[1].Status != "offline" {
		t.Fatalf("%v %+v", err, ps)
	}
	one, _ := parseWindowsPrinters(`{"Name":"Solo","Default":false,"PrinterStatus":"Normal"}`)
	if len(one) != 1 || one[0].Status != "ready" {
		t.Fatalf("%+v", one)
	}
	l := parseLpstat("printer HP_1 is idle.  enabled since Mon\nprinter DNP_2 disabled since Mon\nsystem default destination: DNP_2\n")
	if len(l) != 2 || !l[1].Default || l[1].Status != "offline" || l[0].Status != "ready" {
		t.Fatalf("%+v", l)
	}
}
