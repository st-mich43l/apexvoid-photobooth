package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
	"github.com/st-mich43l/apexvoid-photobooth/internal/camera/edsdk"
	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
	"github.com/st-mich43l/apexvoid-photobooth/internal/settings"
	"github.com/st-mich43l/apexvoid-photobooth/internal/video"
)

// ProbeResult is the outcome of exercising one camera source.
type ProbeResult struct {
	Device       camera.DeviceInfo    `json:"device"`
	Connected    bool                 `json:"connected"`
	ConnectMs    int64                `json:"connectMs"`
	LiveView     ProbeStep            `json:"liveView"`
	Still        ProbeStep            `json:"still"`
	Video        ProbeStep            `json:"video"`
	Capabilities *camera.Capabilities `json:"capabilitiesAfter,omitempty"`
	Released     bool                 `json:"releasedCleanly"`
	Error        string               `json:"error,omitempty"`
}

// ProbeStep is one checked operation. Attempted=false means it was not run
// (and therefore must be reported as UNVERIFIED, never as passed).
type ProbeStep struct {
	Attempted bool    `json:"attempted"`
	OK        bool    `json:"ok"`
	Detail    string  `json:"detail,omitempty"`
	Error     string  `json:"error,omitempty"`
	Action    string  `json:"action,omitempty"`
	Ms        int64   `json:"ms,omitempty"`
	FPS       float64 `json:"fps,omitempty"`
	File      string  `json:"file,omitempty"`
}

// ProbeReport is written by `--probe-cameras` for hardware acceptance.
type ProbeReport struct {
	Version   string                `json:"version"`
	OS        string                `json:"os"`
	When      string                `json:"when"`
	OutputDir string                `json:"outputDir"`
	Drivers   []camera.DriverStatus `json:"drivers"`
	Conflicts []string              `json:"conflicts"`
	Devices   []ProbeResult         `json:"devices"`
	Notes     []string              `json:"notes"`
}

// ProbeCameras runs the hardware acceptance sequence against every real
// camera source and writes a JSON report plus the captured files:
//
//	photobooth.exe --probe-cameras [report.json] [--include-mock]
//
// It uses the real settings (SDK/FFmpeg paths) but a throw-away library, so it
// never touches the operator's media. Steps that cannot run are reported as
// not attempted, never as passed.
func ProbeCameras(version string, args []string) int {
	reportPath, includeMock := "probe-report.json", false
	for _, a := range args {
		if a == "--include-mock" {
			includeMock = true
		} else {
			reportPath = a
		}
	}
	stamp := time.Now().Format("20060102-150405")
	outDir := filepath.Join(filepath.Dir(reportPath), "probe-"+stamp)
	_ = os.MkdirAll(outDir, 0o755)
	tmp, err := os.MkdirTemp("", "apexvoid-probe-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	defer os.RemoveAll(tmp)
	cfg := settings.DefaultPath()
	if includeMock { // an isolated config with the simulated camera enabled
		cfg = filepath.Join(tmp, "settings.json")
		sm, _ := settings.Load(cfg)
		st := sm.Get()
		st.EnableMock = true
		_ = sm.Save(st)
	}
	svc, err := New(Options{Version: version, ConfigPath: cfg, SkipRestore: true,
		ResolveLibrary: func(*settings.Manager) settings.Resolution {
			return settings.Resolution{Path: filepath.Join(tmp, "lib"), Source: settings.SourceCustom}
		}})
	if err != nil {
		fmt.Fprintln(os.Stderr, "start:", err)
		return 2
	}
	defer svc.Shutdown()

	rep := ProbeReport{Version: version, OS: runtime.GOOS + "/" + runtime.GOARCH, When: time.Now().Format(time.RFC3339), OutputDir: outDir,
		Drivers: svc.mgr.Drivers(), Conflicts: edsdk.Conflicts()}
	devs, derr := svc.discoverFresh()
	if derr != nil {
		rep.Notes = append(rep.Notes, "discovery: "+derr.Error())
	}
	if len(devs) == 0 {
		rep.Notes = append(rep.Notes, "No camera source was found: every camera step is UNVERIFIED.")
	}
	for _, d := range devs {
		if d.DuplicateOf != "" {
			rep.Notes = append(rep.Notes, fmt.Sprintf("%s skipped: duplicate of %s", d.Name, d.DuplicateOf))
			continue
		}
		rep.Devices = append(rep.Devices, svc.probeOne(d, outDir))
	}
	b, _ := json.MarshalIndent(rep, "", "  ")
	if err := os.WriteFile(reportPath, b, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Println("probe report:", reportPath)
	return 0
}

func (s *Service) probeOne(d camera.DeviceInfo, outDir string) (r ProbeResult) {
	r.Device = d
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	t0 := time.Now()
	if err := s.mgr.Connect(ctx, d); err != nil {
		r.Error, r.LiveView.Action = err.Error(), camera.ActionOf(err)
		return r
	}
	r.Connected, r.ConnectMs = true, time.Since(t0).Milliseconds()
	defer func() {
		caps := s.mgr.Status().Capabilities
		r.Capabilities = caps
		r.Released = s.mgr.Disconnect() == nil && s.mgr.Status().Device == nil
	}()

	// live view: count frames for up to 3 s
	r.LiveView.Attempted = true
	t1 := time.Now()
	if err := s.mgr.StartLiveView(ctx); err != nil {
		r.LiveView.Error, r.LiveView.Action = err.Error(), camera.ActionOf(err)
	} else {
		var seq uint64
		frames := 0
		var first time.Duration
		end := time.Now().Add(3 * time.Second)
		for time.Now().Before(end) {
			fctx, c := context.WithTimeout(ctx, 4*time.Second)
			f, err := s.mgr.WaitFrame(fctx, seq)
			c()
			if err != nil {
				r.LiveView.Error = "no frames: " + err.Error()
				break
			}
			if frames == 0 {
				first = time.Since(t1)
				if cfg, _, err := image.DecodeConfig(bytesReader(f.JPEG)); err == nil {
					r.LiveView.Detail = fmt.Sprintf("%d×%d", cfg.Width, cfg.Height)
				}
			}
			seq = f.Seq
			frames++
		}
		if frames > 0 {
			r.LiveView.OK, r.LiveView.Ms = true, first.Milliseconds()
			r.LiveView.FPS = float64(frames) / time.Since(t1).Seconds()
		}
	}

	// still photo
	if r.LiveView.OK || d.Driver == edsdk.DriverName { // EDSDK can shoot without live view
		r.Still.Attempted = true
		t2 := time.Now()
		p, err := s.mgr.Capture(ctx)
		if err != nil {
			r.Still.Error, r.Still.Action = err.Error(), camera.ActionOf(err)
		} else if w, h, derr := media.Dimensions(p.Data); derr != nil {
			r.Still.Error = "the camera returned data that is not an image: " + derr.Error()
		} else {
			file := filepath.Join(outDir, fmt.Sprintf("%s-photo.jpg", safeName(d.Name)))
			_ = os.WriteFile(file, p.Data, 0o644)
			r.Still.OK, r.Still.Ms, r.Still.File = true, time.Since(t2).Milliseconds(), file
			r.Still.Detail = fmt.Sprintf("%d×%d, %d bytes, %s, simulated=%v", w, h, len(p.Data), p.Source, p.Simulated)
		}
	}

	// video (only sources that stream frames to the PC)
	if rc, ok := s.mgr.Camera().(video.Recordable); ok && r.LiveView.OK {
		r.Video.Attempted = true
		ff, ferr := findFFmpeg(s)
		enc, eerr := "", error(nil)
		if ferr == nil {
			enc, eerr = s.pickEncoder(ff)
		}
		switch {
		case ferr != nil:
			r.Video.Error = "FFmpeg not found"
		case eerr != nil:
			r.Video.Error = eerr.Error()
		default:
			file := filepath.Join(outDir, fmt.Sprintf("%s-video.mp4", safeName(d.Name)))
			audio := s.settings.Get().Video.AudioInput
			if err := rc.StartRecording(ctx, video.RecordOptions{Path: file, Encoder: enc, Audio: audio}); err != nil {
				r.Video.Error, r.Video.Action = err.Error(), camera.ActionOf(err)
			} else {
				time.Sleep(3 * time.Second)
				res, err := rc.StopRecording(ctx)
				if err != nil || res.Partial {
					r.Video.Error = fmt.Sprintf("stop: %v partial=%v %v", err, res.Partial, res.Err)
				} else if p, perr := video.ProbeFile(ctx, ff, file); perr != nil {
					r.Video.Error = perr.Error()
				} else {
					r.Video.OK, r.Video.File = true, file
					r.Video.Detail = fmt.Sprintf("%d×%d %s, %.1fs, audio=%v (microphone %q — judge lip-sync and sound by playing the file)", p.Width, p.Height, p.VCodec, p.Duration.Seconds(), p.HasAudio, audio)
				}
			}
		}
	}
	return r
}

func safeName(n string) string {
	out := []rune{}
	for _, c := range n {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			out = append(out, c)
		} else {
			out = append(out, '_')
		}
	}
	return string(out)
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
