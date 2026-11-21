package app

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
	"github.com/st-mich43l/apexvoid-photobooth/internal/camera/edsdk"
	"github.com/st-mich43l/apexvoid-photobooth/internal/video"
)

// Check is one diagnostics line.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
	Action string `json:"action,omitempty"`
}

// Diagnostics is the hardware/software report shown in Studio > Diagnostics.
type Diagnostics struct {
	Platform  string                `json:"platform"`
	Checks    []Check               `json:"checks"`
	Drivers   []camera.DriverStatus `json:"drivers"`
	Conflicts []string              `json:"conflicts"`
	Library   LibrarySummary        `json:"library"`
	LogDir    string                `json:"logDir"`
	Network   string                `json:"network"`
	Movie     []string              `json:"movieFindings"`
}

// RunDiagnostics inspects the machine. Nothing here uses the network.
func (s *Service) RunDiagnostics() Diagnostics {
	d := Diagnostics{Platform: runtime.GOOS + "/" + runtime.GOARCH, Drivers: s.mgr.Drivers(), Library: s.librarySummary(), LogDir: s.db.Layout.LogsDir(),
		Network: "not used: the application makes no network connections (the preview server listens on loopback only)", Conflicts: []string{}}
	ok := func(name string, good bool, detail, action string) {
		d.Checks = append(d.Checks, Check{Name: name, OK: good, Detail: detail, Action: action})
	}
	ok("Media folder writable", s.db.Layout.Ensure() == nil, s.db.Layout.Root, "Choose another media folder in Settings.")
	free := freeBytes(s.db.Layout.Root)
	ok("Free disk space", free > 2<<30, humanBytes(free), "Free at least 2 GB before an event.")
	for _, dr := range d.Drivers {
		if dr.Name == "mock" && !dr.Available {
			continue
		}
		ok("Camera driver: "+dr.Name, dr.Available, orDash(dr.Problem, "ready"), "")
	}
	if ff, err := findFFmpeg(s); err != nil {
		ok("FFmpeg", false, err.Error(), "Needed for webcams and video. Place ffmpeg.exe next to the app.")
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		enc, eerr := video.PickEncoder(ctx, ff)
		cancel()
		detail := ff
		if video.IsBuiltIn(ff) {
			detail = "built into this application (LGPL build, unpacked to " + ff + ")"
		}
		ok("FFmpeg", true, detail, "")
		ok("H.264 encoder", eerr == nil, orDash(enc, "none found"), "Use an FFmpeg build with libx264 or h264_mf.")
	}
	if runtime.GOOS == "windows" {
		d.Conflicts = edsdk.Conflicts()
		if len(d.Conflicts) > 0 {
			ok("Programs that can own the Canon camera", false, "running: "+join(d.Conflicts), "Close them before connecting the camera.")
		} else {
			ok("Programs that can own the Canon camera", true, "none running", "")
		}
	}
	d.Movie = []string{
		"Canon EOS R50 movie start/stop through EDSDK: NOT TESTED (no hardware/SDK in CI) — reported as 'unverified', not implemented.",
		"Movie file transfer through EDSDK: NOT TESTED — reported as 'unverified', not implemented.",
		"Implemented alternative: the R50 in USB streaming (UVC) mode is recorded by FFmpeg on the PC (separate from camera-native recording).",
	}
	return d
}

func orDash(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func join(a []string) string { return strings.Join(a, ", ") }

func humanBytes(n int64) string {
	if n < 0 {
		return "unknown"
	}
	return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
}

// SaveResult tells the UI whether a restart is needed: changing the media
// folder takes effect after a restart (the library is opened once at startup).
type SaveResult struct {
	RestartRequired bool `json:"restartRequired"`
}
