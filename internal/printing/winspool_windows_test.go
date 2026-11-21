//go:build windows

package printing

import (
	"context"
	"testing"
	"unsafe"
)

// The structures are handed to the Windows API as raw memory: their layout
// must match the SDK (x64) exactly.
func TestWindowsStructLayouts(t *testing.T) {
	checks := []struct {
		name      string
		got, want uintptr
	}{
		{"PRINTER_INFO_2W size", unsafe.Sizeof(printerInfo2{}), 136},
		{"PRINTER_INFO_2W.Status offset", unsafe.Offsetof(printerInfo2{}.Status), 124},
		{"PRINTER_INFO_2W.Attributes offset", unsafe.Offsetof(printerInfo2{}.Attributes), 104},
		{"JOB_INFO_2W size", unsafe.Sizeof(jobInfo2{}), 160},
		{"JOB_INFO_2W.Status offset", unsafe.Offsetof(jobInfo2{}.Status), 104},
		{"DOCINFOW size", unsafe.Sizeof(docInfo{}), 40},
		{"BITMAPINFOHEADER size", unsafe.Sizeof(bitmapInfoHeader{}), 40},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

// Runs against the machine's real spooler. A runner/PC may have no printers:
// the point is that enumeration and capability queries work and never crash.
func TestEnumeratePrintersAgainstTheRealSpooler(t *testing.T) {
	ctx := context.Background()
	d := winDriver{}
	ps, err := d.List(ctx)
	if err != nil {
		t.Fatalf("EnumPrinters: %v", err)
	}
	t.Logf("%d printers installed", len(ps))
	for _, p := range ps {
		t.Logf("printer %q default=%v status=%s port=%q driver=%q", p.Name, p.Default, p.Status, p.Port, p.Driver)
		c, err := d.Capabilities(ctx, p.Name)
		if err != nil {
			t.Logf("  capabilities: %v", err)
			continue
		}
		t.Logf("  %d papers, orientations=%v, maxCopies=%d, color=%v, resolutions=%v", len(c.Papers), c.Orientations, c.MaxCopies, c.Color, c.Resolutions)
		if len(c.Papers) > 0 {
			if g, err := d.Geometry(ctx, p.Name, c.Papers[0], Portrait); err != nil {
				t.Logf("  geometry: %v", err)
			} else {
				t.Logf("  geometry for %s: %+v", c.Papers[0].Name, g)
				if g.PhysW <= 0 || g.PhysH <= 0 || g.PrintW > g.PhysW {
					t.Errorf("implausible geometry %+v", g)
				}
			}
		}
	}
	// a job id that does not exist is "removed from the queue", not an error
	if len(ps) > 0 {
		st, err := d.Status(ctx, ps[0].Name, "2147483000")
		if err != nil || st.Status != StatusRemoved {
			t.Errorf("unknown job: %v %+v", err, st)
		}
	}
}
