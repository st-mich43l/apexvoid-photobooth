package app

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
	"github.com/st-mich43l/apexvoid-photobooth/internal/camera/edsdk"
	"github.com/st-mich43l/apexvoid-photobooth/internal/settings"
	"github.com/st-mich43l/apexvoid-photobooth/internal/video"
)

// Check is one diagnostics line.
type Check struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	OK     bool   `json:"ok"` // false only for a real problem or warning; informational lines are OK
	Detail string `json:"detail"`
	Action string `json:"action,omitempty"`
	// State: ok, problem (something is wrong), warn (worth fixing), info (does not apply to the current set-up, e.g. an unused camera driver).
	State string `json:"state"`
	// BlocksGuests: this check, as it stands, stops guests from taking photos.
	BlocksGuests bool `json:"blocksGuests"`
	// Scope: what the check concerns (storage, camera, canon, webcam, video, system).
	Scope string `json:"scope"`
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

// RunDiagnostics inspects the machine. Nothing here uses the network. Checks are source-aware:
// a Canon SDK problem is only a problem when a Canon camera is the photo camera; for a webcam set-up
// it is informational, and a missing FFmpeg only matters when a webcam (or video) is in use.
func (s *Service) RunDiagnostics() Diagnostics {
	d := Diagnostics{Platform: runtime.GOOS + "/" + runtime.GOARCH, Drivers: s.mgr.Drivers(), Library: s.librarySummary(), LogDir: s.db.Layout.LogsDir(),
		Network: "not used: the application makes no network connections (the preview server listens on loopback only)", Conflicts: []string{}}
	add := func(scope, code, name, state, detail, action string, blocks bool) {
		d.Checks = append(d.Checks, Check{Code: code, Name: name, OK: state == "ok" || state == "info", State: state, Detail: detail, Action: action, BlocksGuests: blocks, Scope: scope})
	}
	cfg := s.settings.Get()
	cs := s.mgr.Status()
	pref := s.pref(ModePhoto)
	uses := func(driver string) bool {
		return (cs.Device != nil && cs.Device.Driver == driver) || (cs.Device == nil && pref.Driver == driver)
	}
	canon, webcam := uses("canon-edsdk"), uses("uvc")

	if err := s.db.Layout.Ensure(); err == nil {
		add("storage", "media_writable", "Media folder writable", "ok", s.db.Layout.Root, "", false)
	} else {
		add("storage", "media_writable", "Media folder writable", "problem", s.db.Layout.Root, "Choose another media folder in Settings.", true)
	}
	switch free := freeSpace(s.db.Layout.Root); {
	case free < 0:
		add("storage", "disk_space", "Free disk space", "info", "unknown", "", false)
	case free < minFreeBytes:
		add("storage", "disk_space", "Free disk space", "problem", humanBytes(free)+" free — too little to save photos safely.", "Free some disk space or choose another media folder.", true)
	case free < warnFreeBytes:
		add("storage", "disk_space", "Free disk space", "warn", humanBytes(free)+" free.", "Free at least 2 GB before an event.", false)
	default:
		add("storage", "disk_space", "Free disk space", "ok", humanBytes(free), "", false)
	}
	for _, dr := range d.Drivers {
		switch {
		case dr.Name == "mock":
			if dr.Available && cfg.EnableMock {
				add("camera", "sim_camera", "Simulated camera", "info", "Enabled (demo/test only — never a real camera check).", "", false)
			}
		case dr.Available:
			add(driverScope(dr.Name), "camera_driver", "Camera driver: "+dr.Name, "ok", "ready", "", false)
		case uses(dr.Name):
			add(driverScope(dr.Name), "camera_driver", "Camera driver: "+dr.Name, "problem", orDash(dr.Problem, "unavailable"), "Fix this, or choose a different photo camera on the Camera page.", true)
		default:
			add(driverScope(dr.Name), "camera_driver", "Camera driver: "+dr.Name, "info", "Not available, and not needed for your current camera. "+dr.Problem, "", false)
		}
	}
	if ff, err := findFFmpeg(s); err != nil {
		state, act := "info", "Only needed for webcams and video recording."
		if webcam {
			state, act = "problem", "Webcam photos need ffmpeg.exe next to the app, or its path in Settings."
		}
		add("webcam", "ffmpeg", "FFmpeg", state, err.Error(), act, state == "problem")
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		enc, eerr := video.PickEncoder(ctx, ff)
		cancel()
		detail := ff
		if video.IsBuiltIn(ff) {
			detail = "built into this application (LGPL build, unpacked to " + ff + ")"
		}
		add("webcam", "ffmpeg", "FFmpeg", "ok", detail, "", false)
		if eerr == nil {
			add("video", "encoder", "H.264 encoder", "ok", enc, "", false)
		} else {
			add("video", "encoder", "H.264 encoder", "info", "none found — only video recording is affected, not photos.", "Use an FFmpeg build with libx264 or h264_mf to record video.", false)
		}
	}
	if sdk := edsdk.SDKStatus(cfg.CanonSDKDir); sdk.OK() {
		add("canon", "canon_sdk", "Canon SDK files", "ok", sdk.Path+" ("+sdk.Machine+")", "", false)
	} else if canon {
		add("canon", "canon_sdk", "Canon SDK files", "problem", edsdkKind(sdk.Kind)+": "+sdk.Problem, sdk.Action, true)
	} else {
		add("canon", "canon_sdk", "Canon SDK files", "info", "Not installed — only needed for Canon cameras. Webcams work without it. ("+edsdkKind(sdk.Kind)+")", sdk.Action, false)
	}
	if runtime.GOOS == "windows" {
		d.Conflicts = edsdk.Conflicts()
		switch {
		case len(d.Conflicts) > 0 && canon:
			add("canon", "canon_conflicts", "Programs that can own the Canon camera", "warn", "running: "+join(d.Conflicts), "Close them before connecting the camera.", false)
		case len(d.Conflicts) > 0:
			add("canon", "canon_conflicts", "Programs that can own the Canon camera", "info", "running: "+join(d.Conflicts)+" (only matters for Canon cameras)", "", false)
		default:
			add("canon", "canon_conflicts", "Programs that can own the Canon camera", "ok", "none running", "", false)
		}
	}
	d.Movie = []string{
		"Canon EOS R50 movie start/stop through EDSDK: NOT TESTED (no hardware/SDK in CI) — reported as 'unverified', not implemented.",
		"Movie file transfer through EDSDK: NOT TESTED — reported as 'unverified', not implemented.",
		"Implemented alternative: the R50 in USB streaming (UVC) mode is recorded by FFmpeg on the PC (separate from camera-native recording).",
	}
	return d
}

func driverScope(name string) string {
	switch name {
	case "canon-edsdk":
		return "canon"
	case "uvc":
		return "webcam"
	}
	return "camera"
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
	// Settings is the full result of a PatchSettings (the UI shows what the backend now holds).
	Settings *settings.Settings `json:"settings,omitempty"`
}

// edsdkKind names the kind of SDK problem so missing, incompatible and
// incomplete installs are told apart.
func edsdkKind(k string) string {
	switch k {
	case edsdk.SDKNotFound:
		return "missing"
	case edsdk.SDKWrongArch:
		return "wrong architecture"
	case edsdk.SDKInvalid:
		return "invalid DLL"
	case edsdk.SDKMissingDependency:
		return "incomplete (a dependent DLL is missing)"
	case edsdk.SDKMissingExport:
		return "incompatible version"
	case edsdk.SDKPlatform:
		return "not available on this platform"
	}
	return k
}
