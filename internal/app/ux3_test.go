package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
)

// UX-3: operator readiness and diagnostics, against the mock camera, fake webcam driver and fake printer.

func diagCheck(d Diagnostics, name string) Check {
	for _, c := range d.Checks {
		if c.Name == name {
			return c
		}
	}
	return Check{}
}

func TestSimulatedCameraIsDemoNeverVerifiedHardware(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	r := s.BoothReadiness()
	if !r.Demo || r.Status != ReadyStatusDemo || !r.CanStart {
		t.Fatalf("a simulated camera works but is demo/test mode: %+v", r)
	}
	if v := item(r, "camera").Verification; v != VerDemo {
		t.Fatalf("camera verification %q", v)
	}
	// even after a photograph was taken it is never "verified"
	_ = s.StartPreview()
	if err := s.StartShooting("plain.strip4", "", ""); err != nil {
		t.Fatal(err)
	}
	waitShoot(t, s, "REVIEW")
	_ = s.RecoverShooting()
	if v := item(s.BoothReadiness(), "capture").Verification; v == VerVerified {
		t.Fatalf("a simulated capture must not count as hardware verification")
	}
	if !strings.Contains(strings.ToLower(item(r, "camera").Detail), "simulated") {
		t.Fatalf("the operator must be told: %+v", item(r, "camera"))
	}
}

func TestWebcamOnlyIsReadyAndNeedsNoCanonSDK(t *testing.T) {
	e := newSources(t, false)
	if err := e.s.ConnectCamera("uvc:a"); err != nil {
		t.Fatal(err)
	}
	r := e.s.BoothReadiness()
	if r.Demo || !r.CanStart {
		t.Fatalf("a webcam is a real camera: %+v", r)
	}
	if v := item(r, "capture").Verification; v == VerDemo || v == "" {
		t.Fatalf("a real camera is verified or unverified, never demo: %q", v)
	}
	for _, it := range r.Items {
		if it.Group == "camera" && strings.Contains(strings.ToLower(it.Label+it.Detail), "canon") {
			t.Fatalf("Canon must not appear in a webcam-only readiness: %+v", it)
		}
	}
	d := e.s.RunDiagnostics()
	sdk := diagCheck(d, "Canon SDK files")
	if sdk.Name != "" && (sdk.BlocksGuests || sdk.State == "problem") {
		t.Fatalf("a missing Canon SDK must not make a webcam set-up unusable: %+v", sdk)
	}
	for _, c := range d.Checks {
		if c.BlocksGuests && c.Scope == "canon" {
			t.Fatalf("%+v", c)
		}
	}
}

func TestCanonDriverUnavailableMattersOnlyForCanonCameras(t *testing.T) {
	e := newSources(t, false)
	_ = e.s.ConnectCamera("uvc:a")
	for _, c := range e.s.RunDiagnostics().Checks {
		if c.Scope == "canon" && (c.State == "problem" || c.State == "warn") {
			t.Fatalf("webcam set-up: %+v", c)
		}
	}
	// a preferred Canon camera that is not connected makes the SDK a real problem
	_ = e.s.mgr.Disconnect()
	if _, err := e.s.PatchSettings(map[string]any{"photoCamera": map[string]any{"id": "canon:1", "name": "Canon EOS R50", "driver": "canon-edsdk"}}); err != nil {
		t.Fatal(err)
	}
	c := diagCheck(e.s.RunDiagnostics(), "Canon SDK files")
	if c.Name != "" && c.State == "ok" {
		return // SDK present on this machine
	}
	if c.State != "problem" || !c.BlocksGuests {
		t.Fatalf("with a Canon camera preferred the SDK is required: %+v", c)
	}
}

func TestPreferredCameraMissingShowsInReadiness(t *testing.T) {
	e := newSources(t, false)
	_ = e.s.SetPreferredCamera(ModePhoto, "uvc:b")
	e.drv.set(webcam("uvc:a", "Integrated Webcam", "0bda:5830"))
	e.s.RestorePreferred()
	r := e.s.BoothReadiness()
	c := item(r, "camera")
	if r.CanStart || c.State != ItemProblem || !strings.Contains(c.Detail, "Logitech C920") || c.Action == "" {
		t.Fatalf("%+v", r)
	}
}

func TestCameraRemovedWhilePreparingIsReportedNotSubstituted(t *testing.T) {
	e := newSources(t, false)
	_ = e.s.SetPreferredCamera(ModePhoto, "uvc:a")
	_ = e.s.ConnectCamera("uvc:a")
	e.drv.set() // unplugged between the readiness check and "Open Booth"
	r, err := e.s.PrepareMode(ModePhoto, "")
	if err == nil && r.Ready {
		t.Fatalf("must not claim readiness for a camera that is gone: %+v", r)
	}
	if e.active() == "uvc:b" {
		t.Fatal("silently switched")
	}
}

func TestPrinterStatesAreOptionalAndHonest(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	// digital only
	r := s.BoothReadiness()
	if r.Printing != PrintDigital || item(r, "printer").Required {
		t.Fatalf("%+v", r)
	}
	// configured and Windows says ready: still not proof of paper
	if _, err := s.PatchSettings(map[string]any{"printerName": "Lab", "printPaper": "4x6 in"}); err != nil {
		t.Fatal(err)
	}
	s.prnCache = printerCache{}
	r = s.BoothReadiness()
	p := item(r, "printer")
	if r.Printing != PrintUnverified || p.Verification != VerUnverified || !strings.Contains(p.Detail, "not checked") {
		t.Fatalf("%+v", p)
	}
	// configured but offline: guests can still take photos, the operator is told they stay digital
	printerOf(t, s).mu.Lock()
	printerOf(t, s).status = "offline"
	printerOf(t, s).mu.Unlock()
	s.prnCache = printerCache{}
	r = s.BoothReadiness()
	p = item(r, "printer")
	if !r.CanStart || r.Printing != PrintUnavailable || p.State != ItemWarn || p.Required || !strings.Contains(p.Action, "digitally") {
		t.Fatalf("%+v", r)
	}
}

func TestLowDiskThresholdsAreSharedByReadinessAndDiagnostics(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	old := freeSpace
	defer func() { freeSpace = old }()
	freeSpace = func(string) int64 { return 100 << 20 }
	r := s.BoothReadiness()
	if r.CanStart || item(r, "space").State != ItemProblem || !item(r, "space").Required {
		t.Fatalf("under 500 MB blocks: %+v", item(r, "space"))
	}
	if c := diagCheck(s.RunDiagnostics(), "Free disk space"); c.State != "problem" || !c.BlocksGuests {
		t.Fatalf("%+v", c)
	}
	freeSpace = func(string) int64 { return 1 << 30 }
	r = s.BoothReadiness()
	if !r.CanStart || item(r, "space").State != ItemWarn {
		t.Fatalf("under 2 GB warns without blocking: %+v", item(r, "space"))
	}
	if c := diagCheck(s.RunDiagnostics(), "Free disk space"); c.State != "warn" || c.BlocksGuests {
		t.Fatalf("%+v", c)
	}
}

func TestNoFramesBlocksAndIsInTheFramesGroup(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	for _, f := range s.ListFrames(false).Frames {
		if err := s.SetFrameEnabled(f.ID, false); err != nil {
			t.Fatal(err)
		}
	}
	r := s.BoothReadiness()
	f := item(r, "frames")
	if r.CanStart || f.State != ItemProblem || f.Group != "frames" {
		t.Fatalf("%+v", r)
	}
}

func TestSetupCompleteWithAWebcamIsReady(t *testing.T) {
	e := newSources(t, false)
	_ = e.s.ConnectCamera("uvc:a")
	for _, id := range []string{"plain.strip4"} {
		_ = e.s.tpl.SetEnabled(id, true)
	}
	r := e.s.BoothReadiness()
	if r.Status != ReadyStatusReady || !r.CanStart || r.Demo {
		t.Fatalf("%+v", r)
	}
	for _, it := range r.Items {
		if it.Group == "" {
			t.Fatalf("every line belongs to a Prepare Booth section: %+v", it)
		}
	}
}

func TestOperatorCameraTestSessionIsLabelledNotDeleted(t *testing.T) {
	s, _, _ := newTestApp(t)
	sn := shootStrip(t, s, 2)
	if err := s.LabelTestSession(sn.SessionID); err != nil {
		t.Fatal(err)
	}
	ss, err := s.store.Session(sn.SessionID)
	if err != nil || ss.Note != testNote || ss.MediaCount == 0 || ss.Status != "completed" {
		t.Fatalf("%v %+v", err, ss)
	}
	if err := s.LabelTestSession("nope"); err == nil {
		t.Fatal("unknown session")
	}
}

func TestDiagnosticSummaryContainsNothingSensitive(t *testing.T) {
	s, root, _ := newTestApp(t)
	connectMock(t, s)
	if err := s.SetKioskPin("", "482913"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PatchSettings(map[string]any{"printerName": "Reception HP LaserJet 4000", "ffmpegPath": filepath.Join(root, "tools", "ffmpeg.exe")}); err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	host, _ := os.Hostname()
	out := s.DiagnosticSummary()
	for _, secret := range []string{"482913", "Reception HP LaserJet", root, filepath.Base(root), s.settings.Path} {
		if secret != "" && strings.Contains(out, secret) {
			t.Fatalf("summary leaks %q:\n%s", secret, out)
		}
	}
	if len(home) > 3 && strings.Contains(out, home) {
		t.Fatalf("summary leaks the home folder")
	}
	if len(host) > 3 && strings.Contains(out, host) {
		t.Fatalf("summary leaks the computer name")
	}
	for _, want := range []string{"support summary", "Booth:", "SIMULATED", "Printer configured: true", "PIN protection: true", "Readiness:", "Diagnostics:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "<media folder>") {
		t.Fatalf("paths should be replaced by placeholders:\n%s", out)
	}
	_ = camera.KindWebcam
}
