package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/printing"
	"github.com/st-mich43l/apexvoid-photobooth/internal/settings"
)

// PrintSelfTestReport documents one real submission through the OS print system.
type PrintSelfTestReport struct {
	Driver    string                 `json:"driver"`
	Printers  []printing.Printer     `json:"printers"`
	Printer   string                 `json:"printer"`
	Paper     *printing.Paper        `json:"paper,omitempty"`
	Caps      *printing.Caps         `json:"capabilities,omitempty"`
	Geometry  *printing.PageGeometry `json:"geometry,omitempty"`
	Job       *printing.Job          `json:"job,omitempty"`
	States    []string               `json:"states"`
	ElapsedMs int64                  `json:"elapsedMs"`
	OK        bool                   `json:"ok"`
	Error     string                 `json:"error,omitempty"`
	Note      string                 `json:"note"`
}

// PrintSelfTest lists printers and, when one is named, prints the calibration
// page to it through the real driver and watches the spooler until the job
// leaves the queue:
//
//	photobooth.exe --print-selftest report.json [printer name] [paper name]
//
// With no printer name it only enumerates (exit 0). It cannot prove paper came
// out of a physical printer; with a file-port printer (Microsoft Print to PDF)
// the output file is the evidence.
func PrintSelfTest(version string, args []string) int {
	report, printer, paper := "print-selftest.json", "", ""
	if len(args) > 0 {
		report = args[0]
	}
	if len(args) > 1 {
		printer = args[1]
	}
	if len(args) > 2 {
		paper = args[2]
	}
	rep := PrintSelfTestReport{States: []string{}, Note: "Spooler states do not prove that paper was delivered."}
	write := func(code int) int {
		b, _ := json.MarshalIndent(rep, "", "  ")
		_ = os.WriteFile(report, b, 0o644)
		fmt.Println("print self-test report:", report)
		return code
	}
	tmp, err := os.MkdirTemp("", "apexvoid-printtest-*")
	if err != nil {
		rep.Error = err.Error()
		return write(2)
	}
	defer os.RemoveAll(tmp)
	cfg := filepath.Join(tmp, "settings.json")
	svc, err := New(Options{Version: version, ConfigPath: cfg, SkipRestore: true,
		ResolveLibrary: func(*settings.Manager) settings.Resolution {
			return settings.Resolution{Path: filepath.Join(tmp, "lib"), Source: settings.SourceCustom}
		}})
	if err != nil {
		rep.Error = "start: " + err.Error()
		return write(2)
	}
	defer svc.Shutdown()

	ctx := context.Background()
	rep.Driver = svc.printerDriverName()
	ps, err := svc.prn.Driver().List(ctx)
	if err != nil {
		rep.Error = "list printers: " + err.Error()
		return write(1)
	}
	rep.Printers = ps
	if printer == "" {
		return write(0)
	}
	rep.Printer = printer
	caps, err := svc.prn.Driver().Capabilities(ctx, printer)
	if err != nil {
		rep.Error = "capabilities: " + err.Error()
		return write(1)
	}
	rep.Caps = &caps
	if len(caps.Papers) == 0 {
		rep.Error = "the printer reports no paper sizes"
		return write(1)
	}
	p := caps.Papers[0]
	{
		if q, ok := printing.FindPaper(caps.Papers, paper); ok {
			p = q
		} else if paper == "" {
			for _, c := range caps.Papers { // prefer a 4x6 sheet when offered
				if (c.WidthIn > 3.9 && c.WidthIn < 4.1 && c.HeightIn > 5.9 && c.HeightIn < 6.1) || (c.HeightIn > 3.9 && c.HeightIn < 4.1 && c.WidthIn > 5.9 && c.WidthIn < 6.1) {
					p = c
					break
				}
			}
		}
	}
	rep.Paper = &p
	if g, err := svc.prn.Driver().Geometry(ctx, printer, p, printing.Portrait); err == nil {
		rep.Geometry = &g
	} else {
		rep.Error = "geometry: " + err.Error()
		return write(1)
	}
	t0 := time.Now()
	job, err := svc.TestPrint(printer, p.Name, "portrait")
	if err != nil {
		rep.Error = "submit: " + err.Error()
		return write(1)
	}
	rep.Job = &job
	last := ""
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		j, rerr := svc.prn.Refresh(ctx, job.ID)
		if rerr != nil {
			rep.Error = "status: " + rerr.Error()
			return write(1)
		}
		if j.Status != last {
			last = j.Status
			rep.States = append(rep.States, fmt.Sprintf("%s (%s) +%dms", j.Status, j.Detail, time.Since(t0).Milliseconds()))
		}
		rep.Job = &j
		if printing.Terminal(j.Status) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	rep.ElapsedMs = time.Since(t0).Milliseconds()
	rep.OK = rep.Job != nil && (rep.Job.Status == printing.StatusRemoved || rep.Job.Status == printing.StatusCompleted)
	if !rep.OK && rep.Error == "" {
		rep.Error = "the job did not leave the queue cleanly (status " + rep.Job.Status + ")"
	}
	if rep.OK {
		return write(0)
	}
	return write(1)
}
