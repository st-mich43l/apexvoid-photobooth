package edsdk

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
)

// These tests drive the real adapter against the fake SDK. They prove the
// driver logic (ownership, ordering, recovery); they say nothing about real
// Canon hardware, which is covered by docs/HARDWARE_TESTS.md.

const portA = `\\?\usb#vid_04a9&pid_32b3#a1b2c3#{guid}`
const portB = `\\?\usb#vid_04a9&pid_32b3#d4e5f6#{guid}`

func bg() context.Context { return context.Background() }

func waitFor(t *testing.T, what string, within time.Duration, cond func() bool) {
	t.Helper()
	end := time.Now().Add(within)
	for time.Now().Before(end) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func (f *fakeAPI) with(fn func()) { f.mu.Lock(); defer f.mu.Unlock(); fn() }

// leakedItems counts transfer items that were handed out but never released
// (the open camera handle itself is not counted).
func (f *fakeAPI) leakedItems() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, k := range f.live {
		if k == "item" {
			n++
		}
	}
	return n
}

func quiet(t *testing.T, f *fakeAPI) {
	t.Helper()
	f.with(func() {
		if len(f.violations) > 0 {
			t.Fatalf("SDK threading violations: %v", f.violations)
		}
	})
}

func TestInitFailureIsReportedAndCanBeRetried(t *testing.T) {
	f := newFake()
	f.initErr = errors.New("EdsInitializeSDK failed")
	d := driverWith(f, nil)
	_, err := d.Discover(bg())
	if camera.CodeOf(err) != camera.CodeSDKMissing || camera.ActionOf(err) == "" {
		t.Fatalf("init failure must be an actionable SDK error: %v", err)
	}
	if _, running := d.Health(); running {
		t.Fatal("a failed initialisation must not leave an SDK thread behind")
	}
	f.with(func() {
		if f.terminated != 0 {
			t.Fatal("EdsTerminateSDK must not run for an SDK that never initialised")
		}
	})
	f.with(func() { f.initErr = nil })
	devs, err := d.Discover(bg())
	if err != nil || len(devs) != 1 {
		t.Fatalf("retry after fixing the cause: %v %v", devs, err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	f.with(func() {
		if f.terminated != 1 {
			t.Fatalf("terminate count %d", f.terminated)
		}
	})
}

type stubDriver struct{ devs []camera.DeviceInfo }

func (s *stubDriver) Name() string     { return "stub" }
func (s *stubDriver) Available() error { return nil }
func (s *stubDriver) Discover(context.Context) ([]camera.DeviceInfo, error) {
	return s.devs, nil
}
func (s *stubDriver) Open(camera.DeviceInfo) (camera.Camera, error) { return nil, errors.New("n/a") }

func TestMissingSDKKeepsWebcamsWorking(t *testing.T) {
	// no SDK folder is configured and none exists next to the test binary
	d := NewDriver(func() string { return t.TempDir() })
	web := &stubDriver{devs: []camera.DeviceInfo{{ID: "uvc:1", Name: "USB Webcam", Driver: "stub", Kind: camera.KindWebcam}}}
	m := camera.NewManager(d, web)
	st := m.Drivers()
	if st[0].Available || st[0].Problem == "" {
		t.Fatalf("the Canon driver must report why it is unavailable: %+v", st[0])
	}
	devs, err := m.Discover(bg())
	if err != nil || len(devs) != 1 || devs[0].ID != "uvc:1" {
		t.Fatalf("webcams must keep working without the SDK: %v %v", devs, err)
	}
}

func TestStableIdentityAcrossReorderAndHotplug(t *testing.T) {
	f := newFake()
	f.cams = []rawCamera{{Name: "Canon EOS R50", Port: portA}, {Name: "Canon EOS R50", Port: portB}}
	d := driverWith(f, nil)
	defer d.Close()
	byPort := func() map[string]string {
		devs, err := d.Discover(bg())
		if err != nil || len(devs) != 2 {
			t.Fatalf("%v %v", devs, err)
		}
		m := map[string]string{}
		for _, x := range devs {
			if x.USBID != "04a9:32b3" || x.Interface == "" || x.Vendor != "Canon" || x.Kind != camera.KindDSLR {
				t.Fatalf("device metadata: %+v", x)
			}
			m[x.Port] = x.ID
		}
		return m
	}
	before := byPort()
	f.with(func() { f.order = []int{1, 0} }) // the SDK lists them the other way round now
	after := byPort()
	if before[portA] != after[portA] || before[portB] != after[portB] || before[portA] == before[portB] {
		t.Fatalf("ids must follow the device, not the list position: %v vs %v", before, after)
	}
	// connecting by id opens that physical body even though the order changed
	devs, _ := d.Discover(bg())
	var wantB camera.DeviceInfo
	for _, x := range devs {
		if x.Port == portB {
			wantB = x
		}
	}
	cam, err := d.Open(wantB)
	if err != nil {
		t.Fatal(err)
	}
	if err := cam.Connect(bg()); err != nil {
		t.Fatal(err)
	}
	f.with(func() {
		if f.openedPort != portB {
			t.Fatalf("opened %q, wanted %q", f.openedPort, portB)
		}
	})
	_ = cam.Disconnect()
	if f.leaked() != 0 {
		t.Fatalf("leaked %d", f.leaked())
	}
	// a third body appearing does not change the others
	f.with(func() {
		f.cams = append(f.cams, rawCamera{Name: "Canon EOS R50", Port: `\\?\usb#vid_04a9&pid_32b3#999#{g}`})
		f.order = nil
	})
	devs, _ = d.Discover(bg())
	got := map[string]string{}
	for _, x := range devs {
		got[x.Port] = x.ID
	}
	if got[portA] != before[portA] || got[portB] != before[portB] {
		t.Fatal("hot-plugging another camera changed existing ids")
	}
}

func TestIdenticalPortsGetDistinctIDs(t *testing.T) {
	ids := stableIDs([]rawCamera{{Name: "Canon EOS R50"}, {Name: "Canon EOS R50"}, {Name: "Canon EOS R7"}})
	if ids[0] == ids[1] || ids[0] == ids[2] || !strings.HasPrefix(ids[0], "canon-edsdk:name:") {
		t.Fatalf("%v", ids)
	}
	if usbIDOf(`\\?\usb#VID_04A9&PID_32B3#x`) != "04a9:32b3" || usbIDOf("COM1") != "" {
		t.Fatal("usb id parsing")
	}
}

func TestRepeatedConnectDisconnectLeaksNothing(t *testing.T) {
	f := newFake()
	d := driverWith(f, nil)
	defer d.Close()
	for i := 0; i < 25; i++ {
		devs, err := d.Discover(bg())
		if err != nil || len(devs) != 1 {
			t.Fatalf("%d: %v %v", i, devs, err)
		}
		cam, err := d.Open(devs[0])
		if err != nil {
			t.Fatal(err)
		}
		if err := cam.Connect(bg()); err != nil {
			t.Fatalf("%d: %v", i, err)
		}
		c := cam.(*Camera)
		c.FrameInterval = time.Millisecond
		if i%2 == 0 {
			if err := c.StartLiveView(bg()); err != nil {
				t.Fatal(err)
			}
		}
		if err := c.Disconnect(); err != nil {
			t.Fatalf("%d: %v", i, err)
		}
		if err := c.Disconnect(); err != nil { // idempotent
			t.Fatal(err)
		}
	}
	f.with(func() {
		if f.sessions != 0 || len(f.live) != 0 || f.opens != 25 {
			t.Fatalf("sessions=%d live=%d opens=%d", f.sessions, len(f.live), f.opens)
		}
		if f.onObject != nil || f.onState != nil {
			t.Fatal("event handlers must be unregistered on disconnect")
		}
	})
	quiet(t, f)
}

func TestLiveViewStartStopRestart(t *testing.T) {
	f := newFake()
	_, c := connect(t, f)
	for round := 0; round < 3; round++ {
		if err := c.StartLiveView(bg()); err != nil {
			t.Fatal(err)
		}
		if err := c.StartLiveView(bg()); err != nil { // idempotent
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(bg(), 2*time.Second)
		fr, err := c.WaitFrame(ctx, 0)
		cancel()
		if err != nil || !isJPEGStream(fr.JPEG) {
			t.Fatalf("round %d: %v", round, err)
		}
		f.with(func() {
			if !f.evfOn {
				t.Fatal("live view should be on")
			}
		})
		if c.State() != camera.StateLiveView {
			t.Fatalf("state %v", c.State())
		}
		_ = c.StopLiveView()
		f.with(func() {
			if f.evfOn {
				t.Fatal("live view must be off after Stop")
			}
		})
		if c.State() != camera.StateConnected {
			t.Fatalf("state %v", c.State())
		}
	}
	f.with(func() {
		if f.evfModeSet == 0 {
			t.Fatal("Evf_Mode should be requested (best effort) before the PC output is switched on")
		}
	})
	if c.Capabilities().LiveView != camera.Supported {
		t.Fatal("live view should be promoted after real frames")
	}
}

func TestLiveViewIsRearmedWhenTheBodyDropsIt(t *testing.T) {
	f := newFake()
	_, c := connect(t, f)
	if err := c.StartLiveView(bg()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(bg(), 2*time.Second)
	defer cancel()
	fr, err := c.WaitFrame(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	var arms int
	f.with(func() { f.evfOn = false; arms = f.evfArms }) // the camera turned the PC output off (e.g. after a shot)
	next, err := c.WaitFrame(ctx, fr.Seq)
	if err != nil || next.Seq <= fr.Seq {
		t.Fatalf("live view must come back on its own: %v", err)
	}
	f.with(func() {
		if f.evfArms <= arms {
			t.Fatal("the PC output was never requested again")
		}
	})
}

func TestLiveViewResumesAfterACapture(t *testing.T) {
	f := newFake()
	_, c := connect(t, f)
	_ = c.StartLiveView(bg())
	ctx, cancel := context.WithTimeout(bg(), 2*time.Second)
	defer cancel()
	if _, err := c.WaitFrame(ctx, 0); err != nil {
		t.Fatal(err)
	}
	var arms int
	f.with(func() { arms = f.evfArms })
	if _, err := c.Capture(bg()); err != nil {
		t.Fatal(err)
	}
	f.with(func() {
		if f.evfArms <= arms || !f.evfOn {
			t.Fatal("live view must be re-armed after a capture")
		}
	})
	if c.State() != camera.StateLiveView {
		t.Fatalf("state %v", c.State())
	}
}

func TestShutterErrorsAreActionable(t *testing.T) {
	cases := []struct {
		name string
		code uint32
		want string
	}{
		{"busy", errDeviceBusy, camera.CodeDeviceBusy},
		{"no card", errTakePictureNoCard, camera.CodeCaptureFailed},
		{"card protected", errTakePictureProtect, camera.CodeCaptureFailed},
		{"memory full", errDeviceMemoryFull, camera.CodeCaptureFailed},
		{"unsupported", errCommDeviceIncompat, camera.CodeUnsupported},
	}
	for _, tc := range cases {
		f := newFake()
		f.takeErrs = []error{&sdkError{Op: "x", Code: tc.code}}
		_, c := connect(t, f)
		_, err := c.Capture(bg())
		if camera.CodeOf(err) != tc.want || camera.ActionOf(err) == "" {
			t.Errorf("%s: %v / %q", tc.name, err, camera.ActionOf(err))
		}
		if _, err := c.Capture(bg()); err != nil {
			t.Errorf("%s: the camera must stay usable after a failed shot: %v", tc.name, err)
		}
	}
}

func TestIncompleteTransferIsRejectedAndTheNextShotWorks(t *testing.T) {
	f := newFake()
	_, c := connect(t, f)
	f.with(func() { f.truncate = true })
	_, err := c.Capture(bg())
	if camera.CodeOf(err) != camera.CodeCaptureFailed || !strings.Contains(err.Error(), "not received completely") {
		t.Fatalf("%v", err)
	}
	if c.Diagnostics().Invalid != 1 {
		t.Fatal("invalid transfer not counted")
	}
	if c.Capabilities().OriginalTransfer == camera.Supported {
		t.Fatal("a rejected transfer must not promote the capability")
	}
	f.with(func() { f.truncate = false })
	p, err := c.Capture(bg())
	if err != nil || p.Filename != "IMG_0002.JPG" {
		t.Fatalf("%v %+v", err, p)
	}
	if f.leakedItems() != 0 {
		t.Fatalf("leaked %d", f.leaked())
	}
}

func TestValidateJPEG(t *testing.T) {
	good := jpegBytes(40, 30, 3)
	if err := validateJPEG(good, uint64(len(good))); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		d    []byte
		size uint64
	}{
		"empty":     {nil, 0},
		"short":     {good[:len(good)/2], uint64(len(good))},
		"not jpeg":  {[]byte("hello world, definitely not an image"), 0},
		"truncated": {good[:len(good)-70], 0},
	} {
		if validateJPEG(tc.d, tc.size) == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestRAWAndJPEGInEitherOrder(t *testing.T) {
	for _, exts := range [][]string{{".CR3", ".JPG"}, {".JPG", ".CR3"}, {".JPG"}} {
		f := newFake()
		f.captured = exts
		_, c := connect(t, f)
		for shot := 1; shot <= 3; shot++ {
			p, err := c.Capture(bg())
			if err != nil || !strings.HasSuffix(p.Filename, ".JPG") {
				t.Fatalf("%v shot %d: %v %+v", exts, shot, err, p)
			}
		}
		time.Sleep(80 * time.Millisecond) // let any trailing companion arrive
		if f.leakedItems() != 0 {
			t.Fatalf("%v: leaked %d handles", exts, f.leaked())
		}
		f.with(func() {
			if f.downloaded != 3 {
				t.Fatalf("%v: downloaded %d", exts, f.downloaded)
			}
		})
	}
}

func TestTimeoutThenLateTransferIsNeverAttachedToTheNextShot(t *testing.T) {
	f := newFake()
	_, c := connect(t, f)
	c.CaptureTimeout, c.LateGrace = 60*time.Millisecond, 200*time.Millisecond
	f.with(func() { f.holdShots = true })
	_, err := c.Capture(bg())
	if camera.CodeOf(err) != camera.CodeTimeout {
		t.Fatalf("%v", err)
	}
	// the delayed photo of shot 1 turns up while the next capture is settling
	f.with(func() { f.holdShots = false })
	c.CaptureTimeout = time.Second
	res := make(chan camera.Photo, 1)
	errc := make(chan error, 1)
	go func() {
		p, err := c.Capture(bg())
		res <- p
		errc <- err
	}()
	time.Sleep(40 * time.Millisecond)
	f.releaseHeld()
	p, err := <-res, <-errc
	if err != nil {
		t.Fatal(err)
	}
	if p.Filename != "IMG_0002.JPG" {
		t.Fatalf("the late photo of shot 1 was attached to shot 2: %q", p.Filename)
	}
	if c.Diagnostics().LateTransfers < 2 {
		t.Fatalf("late transfers not counted: %+v", c.Diagnostics())
	}
	time.Sleep(50 * time.Millisecond)
	if f.leakedItems() != 0 {
		t.Fatalf("leaked %d", f.leaked())
	}
}

func TestLateArrivalWithoutACaptureIsDroppedAndReleased(t *testing.T) {
	f := newFake()
	_, c := connect(t, f)
	f.with(func() { f.holdShots = true })
	c.CaptureTimeout = 40 * time.Millisecond
	if _, err := c.Capture(bg()); camera.CodeOf(err) != camera.CodeTimeout {
		t.Fatal(err)
	}
	f.releaseHeld()
	waitFor(t, "late events dropped", time.Second, func() bool { return c.Diagnostics().LateTransfers >= 2 })
	if f.leakedItems() != 0 {
		t.Fatalf("leaked %d", f.leaked())
	}
}

func TestDisconnectDuringCaptureReturnsPromptly(t *testing.T) {
	f := newFake()
	_, c := connect(t, f)
	c.CaptureTimeout = 10 * time.Second
	f.with(func() { f.holdShots = true })
	errc := make(chan error, 1)
	go func() { _, err := c.Capture(bg()); errc <- err }()
	time.Sleep(60 * time.Millisecond)
	f.unplug()
	select {
	case err := <-errc:
		if camera.CodeOf(err) != camera.CodeDisconnected {
			t.Fatalf("%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("capture kept waiting although the camera was unplugged")
	}
	if err := c.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if f.leaked() != 0 {
		t.Fatalf("leaked %d", f.leaked())
	}
	quiet(t, f)
}

func TestCancelDuringCaptureReleasesTheLateItem(t *testing.T) {
	f := newFake()
	_, c := connect(t, f)
	c.CaptureTimeout = 5 * time.Second
	f.with(func() { f.holdShots = true })
	ctx, cancel := context.WithCancel(bg())
	errc := make(chan error, 1)
	go func() { _, err := c.Capture(ctx); errc <- err }()
	time.Sleep(40 * time.Millisecond)
	cancel()
	if err := <-errc; !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
	f.releaseHeld()
	waitFor(t, "late items dropped", time.Second, func() bool { return c.Diagnostics().LateTransfers >= 2 })
	time.Sleep(30 * time.Millisecond)
	if f.leakedItems() != 0 {
		t.Fatalf("a cancelled capture leaked %d native references", f.leaked())
	}
}

func TestCancelledConnectDoesNotLeakASession(t *testing.T) {
	f := newFake()
	f.openDelay = 150 * time.Millisecond
	d := driverWith(f, nil)
	defer d.Close()
	devs, _ := d.Discover(bg())
	cam, _ := d.Open(devs[0])
	ctx, cancel := context.WithTimeout(bg(), 30*time.Millisecond)
	defer cancel()
	err := cam.Connect(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("%v", err)
	}
	f.with(func() {
		if f.sessions != 0 {
			t.Fatalf("session left open: %d", f.sessions)
		}
	})
	if f.leaked() != 0 {
		t.Fatalf("leaked %d", f.leaked())
	}
	_ = cam.Disconnect()
}

func TestShutdownIsBoundedWhenTheSDKIsStuck(t *testing.T) {
	f := newFake()
	f.termBlock = make(chan struct{})
	defer close(f.termBlock)
	old := ShutdownTimeout
	ShutdownTimeout = 100 * time.Millisecond
	defer func() { ShutdownTimeout = old }()
	d := driverWith(f, nil)
	if _, err := d.Discover(bg()); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	err := d.Close()
	if err == nil || !strings.Contains(err.Error(), "did not shut down") {
		t.Fatalf("a stuck SDK must be reported, not waited on: %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("shutdown was not bounded")
	}
}

func TestWorkerHealthShowsAStuckCall(t *testing.T) {
	f := newFake()
	w, err := startWorker(f)
	if err != nil {
		t.Fatal(err)
	}
	old := stuckAfter
	stuckAfter = 30 * time.Millisecond
	defer func() { stuckAfter = old }()
	release := make(chan struct{})
	ctx, cancel := context.WithCancel(bg())
	errc := make(chan error, 1)
	go func() { errc <- w.doOp(ctx, "download photo", func() error { <-release; return nil }) }()
	waitFor(t, "call to start", time.Second, func() bool { return w.Health().Busy })
	time.Sleep(60 * time.Millisecond)
	h := w.Health()
	if !h.Busy || h.Op != "download photo" || !h.Stuck || h.RunningMs < 30 {
		t.Fatalf("%+v", h)
	}
	cancel() // the caller gives up; the call keeps running on the SDK thread
	if err := <-errc; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if w.Health().Abandoned != 1 {
		t.Fatalf("%+v", w.Health())
	}
	// a second operation cannot overlap the first: it is only accepted after it finished
	second := make(chan error, 1)
	go func() { second <- w.doOp(bg(), "second", func() error { return nil }) }()
	select {
	case <-second:
		t.Fatal("two native operations overlapped")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	if err := w.stop(time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentOperationsAreSerialised(t *testing.T) {
	f := newFake()
	d, c := connect(t, f)
	c.CaptureTimeout = 2 * time.Second
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := map[string]int{}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.Capture(bg())
			mu.Lock()
			if err == nil {
				results["ok"]++
			} else {
				results[camera.CodeOf(err)]++
			}
			mu.Unlock()
		}()
	}
	for i := 0; i < 3; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = d.Discover(bg()) }()
		go func() {
			defer wg.Done()
			_ = c.StartLiveView(bg())
			time.Sleep(5 * time.Millisecond)
			_ = c.StopLiveView()
		}()
	}
	wg.Wait()
	if results["ok"] < 1 || results["ok"]+results[camera.CodeBusyOperation] != 4 {
		t.Fatalf("overlapping captures must be refused, never mixed: %v", results)
	}
	quiet(t, f)
	_ = c.Disconnect()
	if f.leaked() != 0 {
		t.Fatalf("leaked %d", f.leaked())
	}
}

func TestDisconnectRacingTheStateCallbackNeverPanics(t *testing.T) {
	f := newFake()
	w, err := startWorker(f)
	if err != nil {
		t.Fatal(err)
	}
	defer w.stop(time.Second)
	for i := 0; i < 400; i++ {
		c := newCamera(w, f, camera.DeviceInfo{ID: "x"}, nil, nil)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); c.onState(evStateShutdown, 0) }()
		go func() { defer wg.Done(); _ = c.Disconnect() }()
		wg.Wait()
	}
}

// ---- manager level: recovery with the real Manager around the Canon driver ----

func newManaged(t *testing.T, f *fakeAPI) (*camera.Manager, *Driver) {
	t.Helper()
	d := driverWith(f, nil)
	if _, err := d.Discover(bg()); err != nil { // start the SDK so the driver reports itself ready
		t.Fatal(err)
	}
	m := camera.NewManager(d)
	m.ReconnectInterval = 10 * time.Millisecond
	t.Cleanup(func() { _ = m.Shutdown(); _ = d.Close() })
	return m, d
}

func TestUSBLossAndRecoveryThroughTheManager(t *testing.T) {
	f := newFake()
	m, _ := newManaged(t, f)
	devs, err := m.Discover(bg())
	if err != nil || len(devs) != 1 {
		t.Fatal(devs, err)
	}
	if err := m.Connect(bg(), devs[0]); err != nil {
		t.Fatal(err)
	}
	if err := m.StartLiveView(bg()); err != nil {
		t.Fatal(err)
	}
	f.unplug()
	waitFor(t, "reconnecting state", 3*time.Second, func() bool { return m.Status().Reconnecting })
	if _, err := m.Capture(bg()); camera.CodeOf(err) != camera.CodeNotConnected {
		t.Fatalf("while the camera is away: %v", err)
	}
	f.replug()
	waitFor(t, "live view restored after reconnect", 5*time.Second, func() bool {
		s := m.Status()
		return !s.Reconnecting && s.Device != nil && s.LiveView && s.State == camera.StateLiveView
	})
	p, err := m.Capture(bg())
	if err != nil || len(p.Data) == 0 {
		t.Fatalf("capture after recovery: %v", err)
	}
	if err := m.Shutdown(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "everything released", time.Second, func() bool { return f.leaked() == 0 })
	f.with(func() {
		if f.sessions != 0 || f.evfOn {
			t.Fatalf("camera left busy: sessions=%d evf=%v", f.sessions, f.evfOn)
		}
	})
	quiet(t, f)
}

func TestReconnectNeverSubstitutesALookAlike(t *testing.T) {
	f := newFake()
	f.cams = []rawCamera{{Name: "Canon EOS R50", Port: portA}, {Name: "Canon EOS R50", Port: portB}}
	m, _ := newManaged(t, f)
	devs, _ := m.Discover(bg())
	var a camera.DeviceInfo
	for _, d := range devs {
		if d.Port == portA {
			a = d
		}
	}
	if err := m.Connect(bg(), a); err != nil {
		t.Fatal(err)
	}
	// body A disappears; an identical body B is still attached
	f.with(func() { f.cams = []rawCamera{{Name: "Canon EOS R50", Port: portB}} })
	f.unplug()
	waitFor(t, "reconnecting", 3*time.Second, func() bool { return m.Status().Reconnecting })
	f.replug()
	time.Sleep(300 * time.Millisecond)
	if s := m.Status(); !s.Reconnecting || s.Device == nil {
		t.Fatalf("a different body must not be adopted: %+v", s)
	}
	f.with(func() {
		if f.opens != 1 {
			t.Fatalf("camera B was opened (%d opens)", f.opens)
		}
	})
	// the right body comes back
	f.with(func() {
		f.cams = []rawCamera{{Name: "Canon EOS R50", Port: portA}, {Name: "Canon EOS R50", Port: portB}}
	})
	waitFor(t, "body A re-adopted", 5*time.Second, func() bool { return !m.Status().Reconnecting && m.Status().Device != nil })
	f.with(func() {
		if f.openedPort != portA {
			t.Fatalf("reconnected to %q", f.openedPort)
		}
	})
}

func TestSingleBodyIsFoundAgainOnAnotherUSBPort(t *testing.T) {
	f := newFake()
	f.cams = []rawCamera{{Name: "Canon EOS R50", Port: portA}}
	m, _ := newManaged(t, f)
	devs, _ := m.Discover(bg())
	if err := m.Connect(bg(), devs[0]); err != nil {
		t.Fatal(err)
	}
	f.unplug()
	waitFor(t, "reconnecting", 3*time.Second, func() bool { return m.Status().Reconnecting })
	f.with(func() {
		f.cams = []rawCamera{{Name: "Canon EOS R50", Port: `\\?\usb#vid_04a9&pid_32b3#other-port#{g}`}}
	})
	f.replug()
	waitFor(t, "found on the new port", 5*time.Second, func() bool { return !m.Status().Reconnecting && m.Status().Device != nil })
}

func TestOperatorDisconnectDuringCaptureStopsTheWait(t *testing.T) {
	f := newFake()
	_, c := connect(t, f)
	c.CaptureTimeout = 10 * time.Second
	f.with(func() { f.holdShots = true })
	errc := make(chan error, 1)
	go func() { _, err := c.Capture(bg()); errc <- err }()
	time.Sleep(50 * time.Millisecond)
	if err := c.Disconnect(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errc:
		if camera.CodeOf(err) != camera.CodeDisconnected {
			t.Fatalf("%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a capture kept waiting after the camera was disconnected")
	}
	if f.leaked() != 0 {
		t.Fatalf("leaked %d", f.leaked())
	}
}
