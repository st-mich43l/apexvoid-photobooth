package app

import (
	"strings"
	"testing"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/capture"
)

func item(r BoothReadiness, key string) ReadinessItem {
	for _, it := range r.Items {
		if it.Key == key {
			return it
		}
	}
	return ReadinessItem{}
}

func TestReadinessWithoutACameraBlocksGuestsWithAnAction(t *testing.T) {
	s, _, _ := newTestApp(t)
	r := s.BoothReadiness()
	if r.CanStart || r.Status != ReadyStatusSetup || r.Headline != "Setup required" {
		t.Fatalf("%+v", r)
	}
	cam := item(r, "camera")
	if !cam.Required || cam.State != ItemProblem || cam.Action == "" {
		t.Fatalf("%+v", cam)
	}
	if r.Guest == "" || strings.Contains(strings.ToLower(r.Guest), "error") || strings.Contains(r.Guest, "EDSDK") {
		t.Fatalf("the guest sentence must be calm and non-technical: %q", r.Guest)
	}
	// the API itself refuses a guest session even if a screen still believed the booth was ready
	if err := s.StartShooting("plain.strip4", "", ""); err == nil {
		t.Fatal("a session must not start without a camera")
	} else if ue, ok := err.(*UserError); !ok || ue.Action == "" {
		t.Fatalf("%v", err)
	}
}

func TestReadinessReadyWithCameraAndNoPrinter(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	r := s.BoothReadiness()
	if !r.CanStart {
		t.Fatalf("%+v", r)
	}
	if p := item(r, "printer"); p.Required || p.State != ItemInfo || !strings.Contains(p.Detail, "digital") {
		t.Fatalf("a missing printer must not block digital-only sessions: %+v", p)
	}
	if r.Status != ReadyStatusDemo {
		t.Fatalf("%+v", r)
	}
	if err := s.StartPreview(); err != nil {
		t.Fatal(err)
	}
	if err := s.StartShooting("plain.strip4", "", ""); err != nil {
		t.Fatalf("a ready booth must start: %v", err)
	}
	_ = s.RecoverShooting()
}

func TestReadinessPrinterProblemsAreOptionalAndNeverClaimPaper(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	if _, err := s.PatchSettings(map[string]any{"printerName": "Ghost Printer"}); err != nil {
		t.Fatal(err)
	}
	r := s.BoothReadiness()
	p := item(r, "printer")
	if p.Required || p.State != ItemWarn || !r.CanStart {
		t.Fatalf("an unavailable printer informs but never blocks: %+v / %+v", p, r)
	}
	if r.Status != ReadyStatusDemo {
		t.Fatalf("%+v", r)
	}
	if !strings.Contains(p.Action, "digitally") {
		t.Fatalf("the operator must be told photos are still saved: %+v", p)
	}
	// the fake printer reports the queue state only: the wording must not claim a print
	for _, it := range r.Items {
		if strings.Contains(strings.ToLower(it.Detail), "printed successfully") || strings.Contains(strings.ToLower(it.Detail), "paper was") {
			t.Fatalf("readiness must not claim physical output: %+v", it)
		}
	}
}

func TestReadinessLibraryUnwritableBlocksGuests(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	s.writeProbe.fn = func(string) (bool, string) { return false, "access is denied" }
	r := s.BoothReadiness()
	st := item(r, "storage")
	if r.CanStart || st.State != ItemProblem || !st.Required || !strings.Contains(st.Detail, "access is denied") || st.Action == "" {
		t.Fatalf("%+v / %+v", r, st)
	}
	if err := s.StartPreview(); err != nil {
		t.Fatal(err)
	}
	err := s.StartShooting("plain.strip4", "", "")
	if err == nil || !strings.Contains(err.Error(), "Cannot write") {
		t.Fatalf("starting must be refused: %v", err)
	}
	_ = s.RecoverShooting()
}

func TestReadinessPreferredCameraMissingIsNeverSilentlySubstituted(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	if _, err := s.PatchSettings(map[string]any{"photoCamera": map[string]any{"id": "uvc:gone", "name": "Studio Webcam", "driver": "uvc", "usbId": ""}}); err != nil {
		t.Fatal(err)
	}
	r := s.BoothReadiness()
	c := item(r, "camera")
	if r.CanStart || c.State != ItemProblem || !strings.Contains(c.Detail, "Studio Webcam") || c.Action == "" {
		t.Fatalf("%+v / %+v", r, c)
	}
}

func TestReadinessNoFramesAndSessionInProgress(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	for _, f := range s.ListFrames(false).Frames {
		_ = s.tpl.SetEnabled(f.ID, false)
	}
	if r := s.BoothReadiness(); r.CanStart || item(r, "frames").State != ItemProblem {
		t.Fatalf("%+v", r)
	}
	_ = s.tpl.SetEnabled("plain.strip4", true)
	if err := s.StartPreview(); err != nil {
		t.Fatal(err)
	}
	if err := s.StartShooting("plain.strip4", "", ""); err != nil {
		t.Fatal(err)
	}
	r := s.BoothReadiness()
	if r.CanStart || r.Status != ReadyStatusAttention || item(r, "session").State != ItemProblem {
		t.Fatalf("a second guest session must not start on top of the first: %+v", r)
	}
	_ = s.RecoverShooting()
	if s.ShootState().State != capture.Idle {
		t.Fatalf("%+v", s.ShootState())
	}
}

func TestReadinessReconnectingIsItsOwnState(t *testing.T) {
	s, _, _ := newTestApp(t)
	devs, _ := s.ListCameras()
	var id string
	for _, d := range devs {
		if d.Simulated {
			id = d.ID
		}
	}
	if err := s.ConnectCamera(id); err != nil {
		t.Fatal(err)
	}
	s.mgr.ReconnectInterval = time.Hour // stay in "reconnecting"
	s.mockDriver().SetPresent(false)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !s.mgr.Status().Reconnecting {
		time.Sleep(10 * time.Millisecond)
	}
	r := s.BoothReadiness()
	if r.Status != ReadyStatusReconnecting || r.CanStart || r.Headline != "Reconnecting" {
		t.Fatalf("%+v", r)
	}
	if !strings.Contains(r.Guest, "moment") {
		t.Fatalf("guest text: %q", r.Guest)
	}
}

func TestPatchSettingsNeverOverwritesNewerValuesWithAStaleCopy(t *testing.T) {
	s, _, _ := newTestApp(t)
	stale := s.GetSettings()                                                                      // a screen loaded the settings…
	if _, err := s.PatchSettings(map[string]any{"countdownSeconds": 7, "shots": 6}); err != nil { // …another page changed them
		t.Fatal(err)
	}
	// the stale screen now saves ONE change; it must not roll the others back
	if _, err := s.PatchSettings(map[string]any{"printerName": "P1"}); err != nil {
		t.Fatal(err)
	}
	cur := s.GetSettings()
	if cur.CountdownSeconds != 7 || cur.Shots != 6 || cur.PrinterName != "P1" {
		t.Fatalf("%+v (stale %+v)", cur, stale)
	}
	// the PIN is not writable through a patch
	if _, err := s.PatchSettings(map[string]any{"kioskPin": "evil", "kioskPinSet": true}); err != nil {
		t.Fatal(err)
	}
	if s.settings.Get().KioskPin != "" {
		t.Fatal("the PIN must only change through SetKioskPin")
	}
	// invalid values are still refused
	if _, err := s.PatchSettings(map[string]any{"countdownSeconds": 99}); err == nil {
		t.Fatal("validation must still apply")
	}
	if s.GetSettings().CountdownSeconds != 7 {
		t.Fatal("a refused patch must change nothing")
	}
}

func TestRecoverShootingIsIdempotent(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	_ = s.StartPreview()
	_ = s.StartShooting("plain.strip4", "", "")
	for i := 0; i < 3; i++ {
		if err := s.RecoverShooting(); err != nil {
			t.Fatal(err)
		}
	}
	if st := s.ShootState().State; st != capture.Idle {
		t.Fatal(st)
	}
}
