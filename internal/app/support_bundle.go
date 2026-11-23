package app

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// supportBundle deliberately uses an allowlist: raw logs, error text, device
// names, paths, settings, session IDs and validation reports are not exported.
type supportBundle struct {
	Version             string          `json:"version"`
	Platform            string          `json:"platform"`
	OSVersion           string          `json:"osVersion"`
	Generated           string          `json:"generatedUtc"`
	Readiness           string          `json:"readiness"`
	CanStart            bool            `json:"canStart"`
	Simulated           bool            `json:"simulated"`
	Drivers             map[string]bool `json:"drivers"`
	CheckCounts         map[string]int  `json:"checkCounts"`
	InterruptedSessions int             `json:"interruptedSessions"`
	HardwareAcceptance  string          `json:"hardwareAcceptance"`
}

var safeVersion = regexp.MustCompile(`^[A-Za-z0-9._+\-]{1,100}$`)

func (s *Service) supportData() supportBundle {
	version := s.opts.Version
	if !safeVersion.MatchString(version) {
		version = "unknown"
	}
	readiness := s.BoothReadiness()
	diagnostics := s.RunDiagnostics()
	out := supportBundle{Version: version, Platform: runtime.GOOS + "/" + runtime.GOARCH, OSVersion: platformVersion(), Generated: time.Now().UTC().Format(time.RFC3339), Readiness: readiness.Status, CanStart: readiness.CanStart, Simulated: readiness.Demo, Drivers: map[string]bool{}, CheckCounts: map[string]int{}, InterruptedSessions: len(diagnostics.Library.Interrupted), HardwareAcceptance: "UNVERIFIED: automated status is not physical device acceptance"}
	for _, d := range diagnostics.Drivers {
		switch d.Name {
		case "canon-edsdk", "uvc", "mock":
			out.Drivers[d.Name] = d.Available
		}
	}
	for _, c := range diagnostics.Checks {
		switch c.Scope {
		case "storage", "camera", "canon", "webcam", "video", "system":
			switch c.State {
			case "ok", "info", "warn", "problem":
				out.CheckCounts[c.Scope+":"+c.State]++
			}
		}
	}
	return out
}

// safeLogCategories emits only fixed category identifiers and counts. It
// never attempts to redact free-form messages, which may contain private data.
func safeLogCategories(dir string) map[string]int {
	out := map[string]int{}
	for i := 0; i <= 4; i++ {
		name := "app.log"
		if i > 0 {
			name = fmt.Sprintf("app.log.%d", i)
		}
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		scan := bufio.NewScanner(io.LimitReader(f, 1<<20))
		scan.Buffer(make([]byte, 4096), 1<<20)
		for scan.Scan() {
			line := scan.Text()
			switch {
			case strings.Contains(line, `msg="print failed"`):
				out["print_failure"]++
			case strings.Contains(line, `msg="camera disconnected"`):
				out["camera_disconnection"]++
			case strings.Contains(line, `msg="camera error"`):
				out["camera_error"]++
			case strings.Contains(line, `msg="capture failed"`):
				out["capture_failure"]++
			case strings.Contains(line, `msg="connect failed"`):
				out["camera_connection_failure"]++
			case strings.Contains(line, `msg="library recovery"`):
				out["library_recovery"]++
			case strings.Contains(line, `msg="library migration failed"`):
				out["storage_migration_failure"]++
			case strings.Contains(line, `msg="print jobs recovered after restart"`):
				out["print_recovery"]++
			}
		}
		_ = f.Close()
	}
	return out
}

// ExportSupportBundle writes a deliberately requested offline, sanitized ZIP.
// It refuses to replace an existing file and removes an incomplete archive.
func (s *Service) ExportSupportBundle(folder string) (path string, err error) {
	if folder == "" {
		return "", newUserErr("Choose a folder for the support bundle.", "")
	}
	path = filepath.Join(folder, "ApexVoid-support-"+time.Now().UTC().Format("20060102-150405")+".zip")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	outputPath := path
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(outputPath)
			path = ""
		}
	}()
	z := zip.NewWriter(f)
	write := func(name string, v any) error {
		w, e := z.Create(name)
		if e != nil {
			return e
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	if err = write("readiness.json", s.supportData()); err != nil {
		return "", err
	}
	if err = write("sanitized-log-categories.json", safeLogCategories(s.db.Layout.LogsDir())); err != nil {
		return "", err
	}
	if err = z.Close(); err != nil {
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	return path, nil
}
