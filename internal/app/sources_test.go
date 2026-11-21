package app

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
	"github.com/st-mich43l/apexvoid-photobooth/internal/capture"
	"github.com/st-mich43l/apexvoid-photobooth/internal/settings"
)

// fakeSources is a driver named like the real one that lists several webcams.
type fakeSources struct {
	name string
	mu   sync.Mutex
	devs []camera.DeviceInfo
	open map[string]*fakeCam // currently open cameras by id
	hist []string            // order of opens/releases
}

func (f *fakeSources) Name() string     { return f.name }
func (f *fakeSources) Available() error { return nil }
func (f *fakeSources) set(d ...camera.DeviceInfo) {
	f.mu.Lock()
	f.devs = d
	f.mu.Unlock()
}
func (f *fakeSources) Discover(context.Context) ([]camera.DeviceInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]camera.DeviceInfo(nil), f.devs...), nil
}
func (f *fakeSources) Open(i camera.DeviceInfo) (camera.Camera, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := &fakeCam{drv: f, info: i, frames: camera.NewFrameBuffer(), events: make(chan camera.Event, 2)}
	if f.open == nil {
		f.open = map[string]*fakeCam{}
	}
	f.open[i.ID] = c
	f.hist = append(f.hist, "open:"+i.ID)
	return c, nil
}

type fakeCam struct {
	drv    *fakeSources
	info   camera.DeviceInfo
	frames *camera.FrameBuffer
	events chan camera.Event
	mu     sync.Mutex
	state  camera.State
	closed bool
	live   bool
}

func (c *fakeCam) Info() camera.DeviceInfo { return c.info }
func (c *fakeCam) Capabilities() camera.Capabilities {
	return camera.Capabilities{LiveView: camera.Supported, StillCapture: camera.Supported, StreamedVideo: camera.Supported}
}
func (c *fakeCam) State() camera.State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}
func (c *fakeCam) Connect(context.Context) error {
	c.mu.Lock()
	c.state = camera.StateConnected
	c.mu.Unlock()
	return nil
}
func (c *fakeCam) Disconnect() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed, c.state = true, camera.StateDisconnected
	c.mu.Unlock()
	c.drv.mu.Lock()
	c.drv.hist = append(c.drv.hist, "release:"+c.info.ID)
	c.drv.mu.Unlock()
	close(c.events)
	return nil
}
func (c *fakeCam) StartLiveView(context.Context) error {
	c.mu.Lock()
	c.live, c.state = true, camera.StateLiveView
	c.mu.Unlock()
	c.frames.Publish([]byte{0xff, 0xd8, 0xff, 0xd9})
	return nil
}
func (c *fakeCam) StopLiveView() error { c.mu.Lock(); c.live = false; c.mu.Unlock(); return nil }
func (c *fakeCam) WaitFrame(ctx context.Context, after uint64) (camera.Frame, error) {
	return c.frames.Wait(ctx, after)
}
func (c *fakeCam) Capture(context.Context) (camera.Photo, error) {
	return camera.Photo{}, camera.Errf(camera.CodeUnsupported, "", nil, "fake")
}
func (c *fakeCam) Events() <-chan camera.Event { return c.events }

func webcam(id, name, usb string) camera.DeviceInfo {
	return camera.DeviceInfo{ID: id, Name: name, Driver: "uvc", Kind: camera.KindWebcam, USBID: usb,
		Modes: []camera.Mode{{Width: 1280, Height: 720, FPSMax: 30, Format: "mjpeg"}, {Width: 640, Height: 480, FPSMax: 30, Format: "yuyv422"}}}
}

type srcEnv struct {
	s   *Service
	drv *fakeSources
	cfg string
}

func newSources(t *testing.T, restore bool) *srcEnv {
	t.Helper()
	base := t.TempDir()
	cfg := filepath.Join(base, "s.json")
	sm, _ := settings.Load(cfg)
	st := sm.Get()
	st.MediaDir, st.CountdownSeconds = filepath.Join(base, "lib"), 1
	_ = sm.Save(st)
	return startSources(t, cfg, &fakeSources{name: "uvc"}, restore)
}

func startSources(t *testing.T, cfg string, drv *fakeSources, restore bool) *srcEnv {
	t.Helper()
	if len(drv.devs) == 0 {
		drv.devs = []camera.DeviceInfo{webcam("uvc:a", "Integrated Webcam", "0bda:5830"), webcam("uvc:b", "Logitech C920", "046d:082d")}
	}
	s, err := New(Options{ConfigPath: cfg, ExtraDrivers: []camera.Driver{drv}, Tick: 5 * time.Millisecond, SkipRestore: !restore,
		DisableDriver: map[string]bool{"canon-edsdk": true, "uvc": true, "mock": true}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Shutdown() })
	return &srcEnv{s: s, drv: drv, cfg: cfg}
}

func (e *srcEnv) active() string {
	if st := e.s.CameraStatus(); st.Device != nil && !st.Reconnecting {
		return st.Device.ID
	}
	return ""
}

func TestCameraSourcesListsEveryDeviceWithDetails(t *testing.T) {
	e := newSources(t, false)
	e.drv.set(webcam("uvc:a", "Integrated Webcam", "0bda:5830"), webcam("uvc:b", "Integrated Webcam", "0bda:5830"),
		camera.DeviceInfo{ID: "uvc:v", Name: "OBS Virtual Camera", Driver: "uvc", Kind: camera.KindVirtual})
	src, err := e.s.ListCameraSources()
	if err != nil || len(src.Devices) != 3 {
		t.Fatalf("%v %+v", err, src)
	}
	if src.Devices[1].DuplicateOf != "" {
		t.Fatalf("two streams of one USB device through one backend are two sources: %+v", src.Devices[1])
	}
	if len(src.Devices[0].Modes) != 2 || src.Devices[0].Driver != "uvc" || src.Devices[0].Kind != camera.KindWebcam {
		t.Fatalf("backend, kind and supported resolutions must be listed: %+v", src.Devices[0])
	}
	// a genuine duplicate (same device through another backend) cannot be opened
	other := &fakeSources{name: "other", devs: []camera.DeviceInfo{{ID: "other:x", Name: "Integrated Webcam", Driver: "other", USBID: "0bda:5830", Kind: camera.KindWebcam}}}
	s2, err := New(Options{ConfigPath: filepath.Join(t.TempDir(), "s.json"), ExtraDrivers: []camera.Driver{e.drv, other}, SkipRestore: true,
		DisableDriver: map[string]bool{"canon-edsdk": true, "uvc": true, "mock": true}})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Shutdown()
	src2, _ := s2.ListCameraSources()
	var dupID string
	for _, d := range src2.Devices {
		if d.DuplicateOf != "" {
			dupID = d.ID
		}
	}
	if dupID == "" {
		t.Fatalf("cross-backend duplicate not flagged: %+v", src2.Devices)
	}
	if err := s2.ConnectCamera(dupID); err == nil {
		t.Fatal("a duplicate entry was connected")
	}
}

func TestSwitchingReleasesThePreviousDeviceFirst(t *testing.T) {
	e := newSources(t, false)
	if err := e.s.ConnectCamera("uvc:a"); err != nil {
		t.Fatal(err)
	}
	_ = e.s.StartLiveView()
	if err := e.s.ConnectCamera("uvc:b"); err != nil {
		t.Fatal(err)
	}
	if e.active() != "uvc:b" {
		t.Fatal("not switched")
	}
	h := e.drv.hist
	want := []string{"open:uvc:a", "release:uvc:a", "open:uvc:b"}
	if len(h) < 3 || h[0] != want[0] || h[1] != want[1] || h[2] != want[2] {
		t.Fatalf("the previous device must be released before the next one opens: %v", h)
	}
	if !e.s.CameraStatus().LiveView {
		t.Fatal("the preview that was running must be re-initialised after switching")
	}
}

func TestSwitchRefusedDuringCountdown(t *testing.T) {
	base := t.TempDir()
	cfg := filepath.Join(base, "s.json")
	sm, _ := settings.Load(cfg)
	st := sm.Get()
	st.MediaDir, st.CountdownSeconds = filepath.Join(base, "lib"), 10
	_ = sm.Save(st)
	drv := &fakeSources{name: "uvc"}
	s, err := New(Options{ConfigPath: cfg, ExtraDrivers: []camera.Driver{drv}, Tick: 200 * time.Millisecond, SkipRestore: true,
		DisableDriver: map[string]bool{"canon-edsdk": true, "uvc": true, "mock": true}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()
	drv.set(webcam("uvc:a", "A", "1:1"), webcam("uvc:b", "B", "2:2"))
	if err := s.ConnectCamera("uvc:a"); err != nil {
		t.Fatal(err)
	}
	_ = s.StartPreview()
	_ = s.StartShooting(1)
	waitShoot(t, s, capture.Countdown)
	err = s.ConnectCamera("uvc:b")
	if ue, ok := err.(*UserError); !ok || ue.Action == "" {
		t.Fatalf("switching during a session must be refused with guidance: %v", err)
	}
	if st := s.CameraStatus(); st.Device == nil || st.Device.ID != "uvc:a" {
		t.Fatal("the camera was swapped under a running session")
	}
}

func TestPreferredCameraSurvivesRestartAndIsRestored(t *testing.T) {
	e := newSources(t, false)
	if err := e.s.SetPreferredCamera(ModePhoto, "uvc:b"); err != nil {
		t.Fatal(err)
	}
	if e.s.GetSettings().PhotoCamera.Name != "Logitech C920" {
		t.Fatal("preference not saved")
	}
	cfg, drv := e.cfg, e.drv
	_ = e.s.Shutdown()

	e2 := startSources(t, cfg, drv, true) // restart with auto-restore
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && e2.active() == "" {
		time.Sleep(10 * time.Millisecond)
	}
	if e2.active() != "uvc:b" {
		t.Fatalf("preferred camera not restored after restart: %q", e2.active())
	}
}

func TestPreferredCameraIsFoundAgainWhenItsIdChanges(t *testing.T) {
	e := newSources(t, false)
	_ = e.s.SetPreferredCamera(ModePhoto, "uvc:b")
	// replugged into another port: new id, same USB id and name
	e.drv.set(webcam("uvc:a", "Integrated Webcam", "0bda:5830"), webcam("uvc:b-newport", "Logitech C920", "046d:082d"))
	e.s.RestorePreferred()
	if e.active() != "uvc:b-newport" {
		t.Fatalf("active %q", e.active())
	}
}

func TestMissingPreferredCameraNeverFallsBackSilently(t *testing.T) {
	e := newSources(t, false)
	_ = e.s.SetPreferredCamera(ModePhoto, "uvc:b")
	e.drv.set(webcam("uvc:a", "Integrated Webcam", "0bda:5830")) // C920 unplugged
	e.s.RestorePreferred()
	if e.active() != "" {
		t.Fatal("start-up connected a different camera without asking")
	}
	src, _ := e.s.ListCameraSources()
	if src.RestoreNote == "" || src.PhotoMatchID != "" {
		t.Fatalf("the operator must be told: %+v", src)
	}
	r, err := e.s.PrepareMode(ModePhoto, "")
	if err != nil || r.Ready || !r.NeedsFallback || r.Candidate == nil || r.Candidate.ID != "uvc:a" || r.Message == "" {
		t.Fatalf("%v %+v", err, r)
	}
	if e.active() != "" {
		t.Fatal("PrepareMode must not switch before approval")
	}
	r, err = e.s.PrepareMode(ModePhoto, "uvc:a")
	if err != nil || !r.Ready || e.active() != "uvc:a" {
		t.Fatalf("after approval: %v %+v active=%q", err, r, e.active())
	}
}

func TestPhotoAndVideoHaveSeparatePreferences(t *testing.T) {
	e := newSources(t, false)
	_ = e.s.SetPreferredCamera(ModePhoto, "uvc:a")
	_ = e.s.SetPreferredCamera(ModeVideo, "uvc:b")
	r, err := e.s.PrepareMode(ModePhoto, "")
	if err != nil || !r.Ready || e.active() != "uvc:a" {
		t.Fatalf("%v %+v", err, r)
	}
	r, err = e.s.PrepareMode(ModeVideo, "")
	if err != nil || !r.Ready || !r.Switched || e.active() != "uvc:b" {
		t.Fatalf("video mode must switch to its own camera: %v %+v", err, r)
	}
	r, _ = e.s.PrepareMode(ModeVideo, "")
	if !r.Ready || r.Switched {
		t.Fatalf("already correct: %+v", r)
	}
	r, _ = e.s.PrepareMode(ModePhoto, "")
	if e.active() != "uvc:a" || !r.Switched {
		t.Fatal("photo mode must switch back")
	}
	_ = e.s.ClearPreferredCamera(ModeVideo)
	if e.s.GetSettings().VideoCamera.ID != "" {
		t.Fatal("not cleared")
	}
}

func TestNoPreferenceKeepsConnectedCameraWithoutPrompting(t *testing.T) {
	e := newSources(t, false)
	_ = e.s.ConnectCamera("uvc:b")
	r, err := e.s.PrepareMode(ModePhoto, "")
	if err != nil || !r.Ready || r.NeedsFallback || e.active() != "uvc:b" {
		t.Fatalf("%v %+v", err, r)
	}
}

func TestVideoModeRejectsSourcesThatCannotRecord(t *testing.T) {
	base := t.TempDir()
	cfg := filepath.Join(base, "s.json")
	sm, _ := settings.Load(cfg)
	st := sm.Get()
	st.MediaDir, st.EnableMock = filepath.Join(base, "lib"), true
	_ = sm.Save(st)
	drv := &fakeSources{name: "uvc"}
	s, err := New(Options{ConfigPath: cfg, ExtraDrivers: []camera.Driver{drv}, SkipRestore: true, DisableDriver: map[string]bool{"canon-edsdk": true, "uvc": true}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()
	drv.set(webcam("uvc:a", "A", "1:1"))
	err = s.SetPreferredCamera(ModeVideo, "mock-0")
	if ue, ok := err.(*UserError); !ok || ue.Action == "" {
		t.Fatalf("the simulated/Canon-SDK source cannot be the video camera: %v", err)
	}
	if err := s.SetPreferredCamera(ModePhoto, "mock-0"); err != nil {
		t.Fatalf("it can be the photo camera: %v", err)
	}
	// with only photo-capable devices, video mode reports clearly
	drv.set()
	if _, err := s.PrepareMode(ModeVideo, ""); err == nil {
		t.Fatal("no video camera must be an error with guidance")
	}
}

func TestCameraModeIsRememberedPerDeviceAndAppliedByReconnecting(t *testing.T) {
	e := newSources(t, false)
	_ = e.s.ConnectCamera("uvc:a")
	opens := len(e.drv.hist)
	if err := e.s.SetCameraMode("uvc:a", settings.CameraMode{Width: 1280, Height: 720, FPS: 30, Format: "mjpeg"}); err != nil {
		t.Fatal(err)
	}
	if len(e.drv.hist) <= opens || e.active() != "uvc:a" {
		t.Fatalf("the active camera must be reconnected so the mode applies: %v", e.drv.hist)
	}
	w, h, fps := e.s.cameraPrefs("uvc:a")
	if w != 1280 || h != 720 || fps != 30 {
		t.Fatalf("%d %d %d", w, h, fps)
	}
	if w, _, _ := e.s.cameraPrefs("uvc:b"); w == 1280 && e.s.GetSettings().Video.Width != 1280 {
		t.Fatal("a mode must not leak to other devices")
	}
	if err := e.s.SetCameraMode("uvc:a", settings.CameraMode{Width: 10, Height: 10, FPS: 30}); err == nil {
		t.Fatal("invalid mode accepted")
	}
}
