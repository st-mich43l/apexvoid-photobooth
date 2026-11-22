package app

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
	"github.com/st-mich43l/apexvoid-photobooth/internal/capture"
	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
	"github.com/st-mich43l/apexvoid-photobooth/internal/settings"
)

type events struct {
	mu sync.Mutex
	n  map[string]int
}

func (e *events) emit(name string, _ any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.n == nil {
		e.n = map[string]int{}
	}
	e.n[name]++
}

// newTestApp starts the application exactly as production does, against a
// temp media folder, with the simulated camera enabled and no network access
// required (the test never dials out).
func newTestApp(t *testing.T) (*Service, string, *events) {
	t.Helper()
	root := t.TempDir()
	cfg := filepath.Join(t.TempDir(), "settings.json")
	sm, _ := settings.Load(cfg)
	st := sm.Get()
	st.MediaDir, st.EnableMock, st.CountdownSeconds, st.Shots = root, true, 1, 2
	if err := sm.Save(st); err != nil {
		t.Fatal(err)
	}
	ev := &events{}
	s, err := New(Options{Version: "test", ConfigPath: cfg, Emit: ev.emit, Tick: 5 * time.Millisecond,
		DisableDriver: map[string]bool{"canon-edsdk": true, "uvc": true}, PrintDriver: newFakePrinter()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Shutdown() })
	return s, root, ev
}

func connectMock(t *testing.T, s *Service) {
	t.Helper()
	devs, err := s.ListCameras()
	if err != nil || len(devs) != 1 || !devs[0].Simulated {
		t.Fatalf("devices %v %v", devs, err)
	}
	if err := s.ConnectCamera(devs[0].ID); err != nil {
		t.Fatal(err)
	}
}

func waitShoot(t *testing.T, s *Service, want capture.State) capture.Snapshot {
	t.Helper()
	for i := 0; i < 12000; i++ { // 60 s: the race detector makes full-size image work slow
		if sn := s.ShootState(); sn.State == want {
			return sn
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s: %+v", want, s.ShootState())
	return capture.Snapshot{}
}

func TestStartsOfflineAndCreatesLibraryLayout(t *testing.T) {
	// no network is needed or used: the only listener is loopback
	s, root, _ := newTestApp(t)
	for _, d := range []string{"database", "sessions", "templates", "logs"} {
		if st, err := os.Stat(filepath.Join(root, d)); err != nil || !st.IsDir() {
			t.Fatalf("missing %s", d)
		}
	}
	b := s.Bootstrap()
	if !strings.HasPrefix(b.MediaBase, "http://127.0.0.1:") || b.MediaToken == "" || b.DataDir != root {
		t.Fatalf("%+v", b)
	}
	d := s.RunDiagnostics()
	if !strings.Contains(d.Network, "not used") || len(d.Movie) == 0 {
		t.Fatalf("%+v", d)
	}
}

func TestSimulatedCameraIsHiddenUntilEnabled(t *testing.T) {
	root := t.TempDir()
	cfg := filepath.Join(t.TempDir(), "s.json")
	sm, _ := settings.Load(cfg)
	st := sm.Get()
	st.MediaDir = root
	_ = sm.Save(st)
	s, err := New(Options{ConfigPath: cfg, DisableDriver: map[string]bool{"canon-edsdk": true, "uvc": true}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()
	devs, _ := s.ListCameras()
	if len(devs) != 0 {
		t.Fatalf("mock camera visible by default: %v", devs)
	}
}

func TestFullPhotoWorkflowWithSimulatedCamera(t *testing.T) {
	s, root, ev := newTestApp(t)
	connectMock(t, s)
	if err := s.StartPreview(); err != nil {
		t.Fatal(err)
	}
	if err := s.StartShooting("no-such-frame", "", ""); err == nil {
		t.Fatal("an unknown frame must be rejected")
	}
	if err := s.StartShooting("white-minimal.strip3", "", ""); err == nil {
		t.Fatal("a frame the operator has not enabled must be rejected")
	}
	if err := s.StartShooting(frameFor(2), "", ""); err != nil {
		t.Fatal(err)
	}
	sn := waitShoot(t, s, capture.Review)
	if len(sn.Taken) != 2 || !sn.Simulated {
		t.Fatalf("%+v", sn)
	}
	if err := s.RetakePhoto(0); err != nil {
		t.Fatal(err)
	}
	sn = waitShoot(t, s, capture.Review)
	if err := s.AcceptPhotos(); err != nil {
		t.Fatal(err)
	}
	sn = waitShoot(t, s, capture.Completed)
	if sn.ResultID == "" || sn.Warning != "" {
		t.Fatalf("strip not composed: %+v", sn)
	}

	// library: originals + strip, all flagged correctly
	ms, err := s.ListMedia(sn.SessionID, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	var photos, strips int
	for _, m := range ms {
		switch m.Kind {
		case "photo":
			photos++
			if !m.Simulated {
				t.Fatal("simulated photo not flagged")
			}
		case "strip":
			strips++
			if m.Width != 600 || m.Height != 1800 {
				t.Fatalf("2x6 strip at 300 dpi must be 600x1800, got %dx%d", m.Width, m.Height)
			}
		}
	}
	if photos != 2 || strips != 1 {
		t.Fatalf("photos=%d strips=%d", photos, strips)
	}
	ls := s.LibrarySummary()
	if ls.Sessions != 1 || ls.Photos != 2 {
		t.Fatalf("%+v", ls)
	}
	ev.mu.Lock()
	if ev.n["capture:state"] == 0 || ev.n["camera:status"] == 0 {
		t.Fatalf("UI events not emitted: %v", ev.n)
	}
	ev.mu.Unlock()

	// edit is non-destructive
	var first = ms[0]
	for _, m := range ms {
		if m.Kind == "photo" {
			first = m
			break
		}
	}
	origPath := filepath.Join(root, filepath.FromSlash(first.RelPath))
	before, _ := os.ReadFile(origPath)
	em, err := s.SaveEdits(first.ID, media.Edits{Rotate: 90, Saturation: -0.5})
	if err != nil || em.EditedRel == "" {
		t.Fatalf("%v %+v", err, em)
	}
	if after, _ := os.ReadFile(origPath); string(after) != string(before) {
		t.Fatal("original changed by edit")
	}

	// export, trash (recoverable), restore
	dest := t.TempDir()
	p, err := s.ExportMedia(first.ID, dest, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
	if err := s.TrashMedia(first.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RestoreMedia(first.ID); err != nil {
		t.Fatal(err)
	}

	// the shooting engine and camera can be reused
	_ = s.ResetShooting()
	if err := s.StartPreview(); err != nil {
		t.Fatal(err)
	}
	if err := s.StartShooting(frameFor(1), "", ""); err != nil {
		t.Fatal(err)
	}
	waitShoot(t, s, capture.Review)
	if err := s.AcceptPhotos(); err != nil {
		t.Fatal(err)
	}
	waitShoot(t, s, capture.Completed)
	if s.LibrarySummary().Sessions != 2 {
		t.Fatal("second session missing")
	}
}

func TestVideoWithoutUVCCameraGivesActionableLimitation(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s) // the simulated camera cannot record
	err := s.StartRecording()
	if err == nil {
		t.Fatal("recording must not silently succeed")
	}
	if ue, ok := err.(*UserError); !ok || ue.Action == "" {
		t.Fatalf("want actionable error, got %v", err)
	}
	if s.VideoState().State != "error" {
		t.Fatalf("state %+v", s.VideoState())
	}
}

func TestSettingsPersistAndFolderChangeNeedsRestart(t *testing.T) {
	s, _, _ := newTestApp(t)
	n := s.GetSettings()
	n.CountdownSeconds = 5
	res, err := s.SaveSettings(n)
	if err != nil || res.RestartRequired {
		t.Fatalf("%v %+v", err, res)
	}
	n.MediaDir = filepath.Join(t.TempDir(), "other")
	if res, _ := s.SaveSettings(n); !res.RestartRequired {
		t.Fatal("changing the media folder must require a restart")
	}
	n.Shots = 5
	if _, err := s.SaveSettings(n); err == nil {
		t.Fatal("invalid settings accepted")
	}
}

func TestShutdownReleasesCameraAndIsIdempotent(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	_ = s.StartPreview()
	if err := s.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if st := s.CameraStatus(); st.State != camera.StateDisconnected || st.Device != nil {
		t.Fatalf("camera still held: %+v", st)
	}
	if err := s.Shutdown(); err != nil {
		t.Fatal(err)
	}
}

func TestInterruptedSessionIsRecoveredOnNextStart(t *testing.T) {
	root := t.TempDir()
	cfg := filepath.Join(t.TempDir(), "s.json")
	sm, _ := settings.Load(cfg)
	st := sm.Get()
	st.MediaDir, st.EnableMock, st.CountdownSeconds = root, true, 1
	_ = sm.Save(st)
	mk := func() *Service {
		s, err := New(Options{ConfigPath: cfg, Tick: 5 * time.Millisecond, DisableDriver: map[string]bool{"canon-edsdk": true, "uvc": true}})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	s1 := mk()
	connectMock(t, s1)
	_ = s1.StartPreview()
	_ = s1.StartShooting(frameFor(1), "", "")
	waitShoot(t, s1, capture.Review)
	// simulate a crash: close the DB without Shutdown's session bookkeeping
	s1.httpd.Close()
	_ = s1.db.Close()
	_ = s1.logFile.Close() // a dead process holds no file handles (Windows cannot delete open files)

	s2 := mk()
	defer s2.Shutdown()
	ls := s2.LibrarySummary()
	if len(ls.Interrupted) != 1 || ls.Photos != 1 {
		t.Fatalf("crash recovery: %+v", ls)
	}
}

func TestLoopbackServerRequiresTokenAndServesMediaWithRange(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	_ = s.StartPreview()
	_ = s.StartShooting(frameFor(1), "", "")
	sn := waitShoot(t, s, capture.Review)
	base, tok := s.httpd.base, s.httpd.token

	get := func(path string, hdr map[string]string) *http.Response {
		req, _ := http.NewRequest("GET", base+path, nil)
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := get("/media/"+sn.Taken[0].MediaID+"/thumb", nil); r.StatusCode != 403 {
		t.Fatalf("no token must be refused, got %d", r.StatusCode)
	}
	if r := get("/media/"+sn.Taken[0].MediaID+"/thumb?t=wrong", nil); r.StatusCode != 403 {
		t.Fatalf("bad token accepted: %d", r.StatusCode)
	}
	r := get("/media/"+sn.Taken[0].MediaID+"/original?t="+tok, map[string]string{"Range": "bytes=0-99"})
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if r.StatusCode != 206 || len(body) != 100 || r.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("range: %d len=%d ct=%s", r.StatusCode, len(body), r.Header.Get("Content-Type"))
	}
	if r := get("/media/../../etc/passwd?t="+tok, nil); r.StatusCode == 200 {
		t.Fatal("path traversal served")
	}
	if r := get("/media/nonexistent/original?t="+tok, nil); r.StatusCode != 404 {
		t.Fatalf("unknown id: %d", r.StatusCode)
	}

	// MJPEG preview streams frames
	_ = s.ResetShooting()
	_ = s.StartPreview()
	resp := get("/preview.mjpg?t="+tok, nil)
	defer resp.Body.Close()
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "multipart/x-mixed-replace") {
		t.Fatalf("content-type %q", resp.Header.Get("Content-Type"))
	}
	buf := make([]byte, 4096)
	if n, _ := io.ReadFull(resp.Body, buf); n < 1000 || !strings.Contains(string(buf[:200]), "image/jpeg") {
		t.Fatalf("no preview data (%d bytes)", n)
	}
}

func TestTemplatesAndPrinting(t *testing.T) {
	s, _, _ := newTestApp(t)
	tr := s.ListTemplates()
	if len(tr.Templates) < 5 {
		t.Fatalf("%+v", tr)
	}
	prev, err := s.PreviewTemplate("rose-pink.strip4")
	if err != nil || !strings.HasPrefix(prev, "data:image/jpeg;base64,") {
		t.Fatalf("%v", err)
	}
	connectMock(t, s)
	_ = s.StartPreview()
	_ = s.StartShooting(frameFor(4), "", "")
	waitShoot(t, s, capture.Review)
	_ = s.AcceptPhotos()
	waitShoot(t, s, capture.Completed)
}

func TestComposeSessionWithDifferentLayout(t *testing.T) {
	s, _, _ := newTestApp(t)
	connectMock(t, s)
	_ = s.StartPreview()
	_ = s.StartShooting(frameFor(4), "", "")
	sn := waitShoot(t, s, capture.Review)
	_ = s.AcceptPhotos()
	waitShoot(t, s, capture.Completed)
	m, err := s.ComposeSession(sn.SessionID, "plain.twoup4", "", "", true)
	if err != nil || m.Width != 1200 || m.Height != 1800 {
		t.Fatalf("%v %+v", err, m)
	}
	if _, err := s.ComposeSession(sn.SessionID, "plain.single", "", "", true); err != nil {
		t.Fatal(err) // fewer slots than photos is fine
	}
	if _, err := s.ComposeSession(sn.SessionID, "plain.sheet6", "", "", true); err == nil {
		t.Fatal("6-photo layout with 4 photos must fail")
	}
}

func TestSelfTestRunsWholeStackOffline(t *testing.T) {
	out := filepath.Join(t.TempDir(), "report.json")
	if code := SelfTest("test", []string{out}); code != 0 {
		b, _ := os.ReadFile(out)
		t.Fatalf("selftest exit %d: %s", code, b)
	}
	b, _ := os.ReadFile(out)
	if !strings.Contains(string(b), `"ok": true`) {
		t.Fatalf("%s", b)
	}
}

// frameFor is the always-enabled Plain frame that takes n photos.
func frameFor(n int) string {
	return map[int]string{1: "plain.single", 2: "plain.strip2", 3: "plain.strip3", 4: "plain.strip4", 6: "plain.sheet6"}[n]
}
