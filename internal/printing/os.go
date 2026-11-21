package printing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// NewOSDriver returns the driver for this platform: PowerShell discovery on
// Windows (submission not implemented), CUPS elsewhere.
func NewOSDriver() Driver {
	if runtime.GOOS == "windows" {
		return windowsDriver{}
	}
	return cupsDriver{}
}

func runCmd(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
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

// --- Windows: discovery through PowerShell Get-Printer ---

type windowsDriver struct{}

func (windowsDriver) Name() string { return "windows-spooler" }

func (windowsDriver) List(ctx context.Context) ([]Printer, error) {
	out, err := runCmd(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		"Get-Printer | Select-Object Name,Default,PrinterStatus | ConvertTo-Json -Compress")
	if err != nil {
		return nil, err
	}
	return parseWindowsPrinters(out)
}

func parseWindowsPrinters(out string) ([]Printer, error) {
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	type rec struct {
		Name          string
		Default       bool
		PrinterStatus any
	}
	var recs []rec
	if strings.HasPrefix(out, "[") {
		if err := json.Unmarshal([]byte(out), &recs); err != nil {
			return nil, err
		}
	} else {
		var r rec
		if err := json.Unmarshal([]byte(out), &r); err != nil {
			return nil, err
		}
		recs = []rec{r}
	}
	var ps []Printer
	for _, r := range recs {
		st := "unknown"
		switch v := r.PrinterStatus.(type) {
		case float64:
			if v == 0 {
				st = "ready"
			} else if v == 7 {
				st = "offline"
			} else {
				st = "error"
			}
		case string:
			st = strings.ToLower(v)
			if st == "normal" {
				st = "ready"
			}
		}
		ps = append(ps, Printer{Name: r.Name, Default: r.Default, Status: st})
	}
	return ps, nil
}

func (windowsDriver) Submit(context.Context, string, string, Options) (string, error) {
	return "", fmt.Errorf("%w (Windows submission is planned for the printing milestone and requires validation on a real printer)", ErrNotImplemented)
}

func (windowsDriver) Status(context.Context, string) (string, string, error) {
	return "", "", ErrNotImplemented
}

// --- CUPS (Linux/macOS) ---

type cupsDriver struct{}

func (cupsDriver) Name() string { return "cups" }

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

var reJobID = regexp.MustCompile(`request id is (\S+)`)

func (cupsDriver) Submit(ctx context.Context, printer, file string, o Options) (string, error) {
	out, err := runCmd(ctx, "lp", "-d", printer, "-n", fmt.Sprint(o.Copies), "-o", "media="+o.Paper, "-o", "fit-to-page=false", file)
	if err != nil {
		return "", err
	}
	if m := reJobID.FindStringSubmatch(out); m != nil {
		return m[1], nil
	}
	return "", fmt.Errorf("lp did not report a job id: %q", strings.TrimSpace(out))
}

func (cupsDriver) Status(ctx context.Context, id string) (string, string, error) {
	out, _ := runCmd(ctx, "lpstat", "-W", "not-completed", "-o")
	if strings.Contains(out, id) {
		return JobPrinting, "", nil
	}
	return JobCompleted, "no longer in the CUPS queue (completion is not proof of paper output)", nil
}
