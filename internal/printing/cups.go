package printing

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// cupsDriver prints through CUPS (Linux/macOS). It is a development and
// future-platform path; the paper list is generic because CUPS capabilities are
// not queried.
type cupsDriver struct{}

func (cupsDriver) Name() string { return "cups" }

func runCmd(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	hideWindow(cmd)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("%s: %v %s", name, err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

var reLpstat = regexp.MustCompile(`^printer (\S+) (is idle|now printing|disabled)`)

func (cupsDriver) List(ctx context.Context) ([]Printer, error) {
	out, err := runCmd(ctx, "lpstat", "-p", "-d")
	if err != nil && out == "" {
		return nil, nil // CUPS missing or no printers
	}
	return parseLpstat(out), nil
}

func parseLpstat(out string) []Printer {
	var ps []Printer
	def := ""
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "system default destination:") {
			def = strings.TrimSpace(strings.TrimPrefix(l, "system default destination:"))
		}
		if m := reLpstat.FindStringSubmatch(l); m != nil {
			st := "ready"
			if m[2] == "disabled" {
				st = "offline"
			}
			ps = append(ps, Printer{Name: m[1], Status: st})
		}
	}
	for i := range ps {
		ps[i].Default = ps[i].Name == def
	}
	return ps
}

func (cupsDriver) Capabilities(context.Context, string) (Caps, error) {
	return Caps{Papers: KnownPapers, Orientations: []Orientation{Portrait, Landscape}, MaxCopies: 10, Color: true, Resolutions: [][2]int{{300, 300}},
		Note: "Generic paper list: CUPS capabilities are not queried."}, nil
}

func (cupsDriver) Geometry(_ context.Context, _ string, p Paper, o Orientation) (PageGeometry, error) {
	return GeometryFor(p, o, 300, 0), nil
}

var reJobID = regexp.MustCompile(`request id is (\S+)`)

func (cupsDriver) Submit(ctx context.Context, r SubmitRequest) (string, error) {
	out, err := runCmd(ctx, "lp", "-d", r.Printer, "-n", fmt.Sprint(r.Copies), "-o", "media="+r.Paper.Name, "-o", "fit-to-page=false", r.File)
	if err != nil {
		return "", err
	}
	if m := reJobID.FindStringSubmatch(out); m != nil {
		return m[1], nil
	}
	return "", fmt.Errorf("lp did not report a job id: %q", strings.TrimSpace(out))
}

func (cupsDriver) Status(ctx context.Context, _ string, id string) (JobState, error) {
	out, _ := runCmd(ctx, "lpstat", "-W", "not-completed", "-o")
	if strings.Contains(out, id) {
		return JobState{Status: StatusPrinting, Detail: "In the CUPS queue."}, nil
	}
	return MapSpoolerJob(0, false), nil
}
