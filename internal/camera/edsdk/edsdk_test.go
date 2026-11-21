package edsdk

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
	"github.com/st-mich43l/apexvoid-photobooth/internal/camera/cameratest"
)

// fakeAPI is an in-memory stand-in for EDSDK.dll. It enforces the SDK's
// threading rule (every call from the one SDK goroutine) and delivers events
// only from PumpEvents, like the real SDK.
type fakeAPI struct {
	mu       sync.Mutex
	gid      string
	cam      ref
	next     ref
	live     map[ref]string // handle -> what it is
	pending  []func()
	onObject func(uint32, ref)
	onState  func(uint32, uint32)
	items    map[ref]fileItem
	evfOn    bool
	saveTo   uint32
	captured []string // file names produced per shot
	// fault injection
	openErr     error
	takeErrs    []error // consumed one per TakePicture
	noTakeCmd   bool    // TakePicture reports NOT_SUPPORTED
	shutterUsed int
	skipped     int
	downloaded  int
	closed      bool
	unplugged   bool
	violations  []string
	extends     int
}

func (f *fakeAPI) extendCount() int { f.mu.Lock(); defer f.mu.Unlock(); return f.extends }

type fileItem struct {
	name string
	data []byte
}

func newFake() *fakeAPI {
	return &fakeAPI{live: map[ref]string{}, items: map[ref]fileItem{}, next: 100, captured: []string{"IMG_0001.CR3", "IMG_0001.JPG"}}
}

func goid() string {
	var b [64]byte
	n := runtime.Stack(b[:], false)
	return strings.Fields(string(b[:n]))[1]
}

func (f *fakeAPI) enter(op string) func() {
	f.mu.Lock()
	id := goid()
	if f.gid == "" {
		f.gid = id
	} else if f.gid != id {
		f.violations = append(f.violations, fmt.Sprintf("%s called from goroutine %s, SDK thread is %s", op, id, f.gid))
	}
	return f.mu.Unlock
}

func (f *fakeAPI) newRef(kind string) ref { f.next++; f.live[f.next] = kind; return f.next }

func jpegBytes(w, h int, seed byte) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = seed + byte(i%7)
	}
	var b bytes.Buffer
	_ = jpeg.Encode(&b, img, nil)
	return b.Bytes()
}

func (f *fakeAPI) Init() error { defer f.enter("Init")(); return nil }
func (f *fakeAPI) Terminate()  { defer f.enter("Terminate")() }
func (f *fakeAPI) CameraList() ([]rawCamera, error) {
	defer f.enter("CameraList")()
	if f.unplugged {
		return nil, nil
	}
	return []rawCamera{{Ref: f.newRef("camera"), Name: "Canon EOS R50", Port: "USB-FAKE"}}, nil
}
func (f *fakeAPI) OpenSession(c ref) error {
	defer f.enter("OpenSession")()
	if f.openErr != nil {
		return f.openErr
	}
	f.cam = c
	return nil
}
func (f *fakeAPI) CloseSession(c ref) error {
	defer f.enter("CloseSession")()
	f.closed = true
	if f.unplugged {
		return &sdkError{Op: "EdsCloseSession", Code: errDeviceNotFound}
	}
	return nil
}
func (f *fakeAPI) Release(r ref) { defer f.enter("Release")(); delete(f.live, r) }
func (f *fakeAPI) SendCommand(c ref, cmd, param uint32) error {
	defer f.enter("SendCommand")()
	if f.unplugged {
		return &sdkError{Op: "EdsSendCommand", Code: errDeviceNotFound}
	}
	switch cmd {
	case cmdTakePicture:
		if f.noTakeCmd {
			return &sdkError{Op: "EdsSendCommand", Code: errNotSupported}
		}
		if len(f.takeErrs) > 0 {
			e := f.takeErrs[0]
			f.takeErrs = f.takeErrs[1:]
			if e != nil {
				return e
			}
		}
		f.shoot()
	case cmdExtendShutdownTimer:
		f.extends++
	case cmdPressShutter:
		if param == shutterCompletelyNonAF {
			f.shutterUsed++
			f.shoot()
		}
	}
	return nil
}
func (f *fakeAPI) shoot() {
	for _, n := range f.captured {
		n := n
		f.pending = append(f.pending, func() {
			it := f.newRef("item")
			f.items[it] = fileItem{name: n, data: jpegBytes(120, 80, 9)}
			if f.onObject != nil {
				f.onObject(evObjectDirItemRequestTransfer, it)
			}
		})
	}
}
func (f *fakeAPI) SetU32(r ref, prop, v uint32) error {
	defer f.enter("SetU32")()
	switch prop {
	case propSaveTo:
		f.saveTo = v
	case propEvfOutputDevice:
		f.evfOn = v&evfOutputPC != 0
	}
	return nil
}
func (f *fakeAPI) GetU32(r ref, prop uint32) (uint32, error) {
	defer f.enter("GetU32")()
	return 0, nil
}
func (f *fakeAPI) AnnounceHostSpace(ref) error { defer f.enter("AnnounceHostSpace")(); return nil }
func (f *fakeAPI) DownloadEvf(c ref) ([]byte, error) {
	defer f.enter("DownloadEvf")()
	if f.unplugged {
		return nil, &sdkError{Op: "EdsDownloadEvfImage", Code: errDeviceNotFound}
	}
	if !f.evfOn {
		return nil, &sdkError{Op: "EdsDownloadEvfImage", Code: errObjectNotReady}
	}
	return jpegBytes(64, 48, 1), nil
}
func (f *fakeAPI) DirItem(it ref) (string, uint64, error) {
	defer f.enter("DirItem")()
	i := f.items[it]
	return i.name, uint64(len(i.data)), nil
}
func (f *fakeAPI) Download(it ref, size uint64) ([]byte, error) {
	defer f.enter("Download")()
	f.downloaded++
	d := f.items[it].data
	delete(f.items, it)
	delete(f.live, it)
	return d, nil
}
func (f *fakeAPI) SkipItem(it ref) {
	defer f.enter("SkipItem")()
	f.skipped++
	delete(f.items, it)
	delete(f.live, it)
}
func (f *fakeAPI) SetHandlers(c ref, o func(uint32, ref), s func(uint32, uint32)) error {
	defer f.enter("SetHandlers")()
	f.onObject, f.onState = o, s
	return nil
}
func (f *fakeAPI) PumpEvents() {
	f.mu.Lock()
	if id := goid(); f.gid != "" && f.gid != id {
		f.violations = append(f.violations, "PumpEvents from wrong goroutine")
	}
	pend := f.pending
	f.pending = nil
	f.mu.Unlock()
	for _, p := range pend {
		p()
	}
}

// unplug schedules the state-event the real SDK sends on USB removal.
func (f *fakeAPI) unplug() {
	f.mu.Lock()
	f.unplugged = true
	f.pending = append(f.pending, func() {
		if f.onState != nil {
			f.onState(evStateShutdown, 0)
		}
	})
	f.mu.Unlock()
}

func (f *fakeAPI) leaked() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.live)
}

func driverWith(f *fakeAPI, find conflictFinder) *Driver {
	d := NewDriver(nil)
	d.newAPI = func(string) (api, error) { return f, nil }
	d.find = find
	return d
}

func connect(t *testing.T, f *fakeAPI) (*Driver, *Camera) {
	t.Helper()
	d := driverWith(f, nil)
	t.Cleanup(func() { _ = d.Close() })
	devs, err := d.Discover(context.Background())
	if err != nil || len(devs) != 1 {
		t.Fatalf("discover %v %v", devs, err)
	}
	cam, err := d.Open(devs[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := cam.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := cam.(*Camera)
	c.FrameInterval, c.CaptureTimeout = 2*time.Millisecond, 500*time.Millisecond
	t.Cleanup(func() { _ = c.Disconnect() })
	return d, c
}

func TestAdapterContractAgainstFakeSDK(t *testing.T) {
	f := newFake()
	d := driverWith(f, nil)
	defer d.Close()
	cameratest.RunContract(t, d, cameratest.Options{})
	if len(f.violations) > 0 {
		t.Fatalf("SDK threading violations: %v", f.violations)
	}
}

func TestPhotoIsJPEGAndRAWCompanionIsSkipped(t *testing.T) {
	f := newFake()
	_, c := connect(t, f)
	p, err := c.Capture(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if p.Filename != "IMG_0001.JPG" || p.Simulated {
		t.Fatalf("%+v", p)
	}
	if _, err := jpeg.Decode(bytes.NewReader(p.Data)); err != nil {
		t.Fatal(err)
	}
	if f.skipped != 1 || f.downloaded != 1 {
		t.Fatalf("RAW should be skipped and JPEG downloaded: skipped=%d downloaded=%d", f.skipped, f.downloaded)
	}
	if f.saveTo != saveToHost {
		t.Fatal("camera must be told to save to the host")
	}
	caps := c.Capabilities()
	if caps.StillCapture != camera.Supported || caps.OriginalTransfer != camera.Supported {
		t.Fatalf("capabilities not promoted after a real success: %+v", caps)
	}
	if caps.MovieRecord != camera.Unverified || caps.MovieTransfer != camera.Unverified {
		t.Fatalf("movie capabilities must stay unverified: %+v", caps)
	}
}

func TestRAWOnlyReportsActionableTimeout(t *testing.T) {
	f := newFake()
	f.captured = []string{"IMG_0001.CR3"}
	_, c := connect(t, f)
	_, err := c.Capture(context.Background())
	if camera.CodeOf(err) != camera.CodeTimeout || !strings.Contains(camera.ActionOf(err), "JPEG") {
		t.Fatalf("err=%v action=%q", err, camera.ActionOf(err))
	}
}

func TestNotReadyIsRetried(t *testing.T) {
	f := newFake()
	f.takeErrs = []error{&sdkError{Op: "x", Code: errObjectNotReady}, &sdkError{Op: "x", Code: errObjectNotReady}, nil}
	_, c := connect(t, f)
	if _, err := c.Capture(context.Background()); err != nil {
		t.Fatalf("should succeed after retries: %v", err)
	}
}

func TestShutterButtonFallback(t *testing.T) {
	f := newFake()
	f.noTakeCmd = true
	_, c := connect(t, f)
	if _, err := c.Capture(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.shutterUsed != 1 {
		t.Fatal("fallback not used")
	}
}

func TestAFFailureIsActionable(t *testing.T) {
	f := newFake()
	f.takeErrs = []error{&sdkError{Op: "x", Code: errTakePictureAFNG}}
	_, c := connect(t, f)
	_, err := c.Capture(context.Background())
	if camera.CodeOf(err) != camera.CodeCaptureFailed || !strings.Contains(strings.ToLower(camera.ActionOf(err)), "focus") {
		t.Fatalf("%v / %q", err, camera.ActionOf(err))
	}
}

func TestBusyNamesTheProgramHoldingTheCamera(t *testing.T) {
	f := newFake()
	f.openErr = &sdkError{Op: "EdsOpenSession", Code: errDeviceBusy}
	d := driverWith(f, func() []string { return []string{"explorer.exe", "EOS Utility 3.exe"} })
	defer d.Close()
	devs, _ := d.Discover(context.Background())
	cam, _ := d.Open(devs[0])
	err := cam.Connect(context.Background())
	if camera.CodeOf(err) != camera.CodeDeviceBusy || !strings.Contains(camera.ActionOf(err), "EOS Utility 3.exe") {
		t.Fatalf("%v / %q", err, camera.ActionOf(err))
	}
	if f.leaked() != 0 {
		t.Fatalf("handles leaked after failed connect: %d", f.leaked())
	}
}

func TestUnplugEmitsDisconnectAndReleasesEverything(t *testing.T) {
	f := newFake()
	d, c := connect(t, f)
	_ = d
	if err := c.StartLiveView(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := c.WaitFrame(ctx, 0); err != nil {
		t.Fatal(err)
	}
	f.unplug()
	select {
	case ev := <-c.Events():
		if ev.Kind != camera.EventDisconnected {
			t.Fatalf("%+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no disconnect event")
	}
	if _, err := c.Capture(context.Background()); camera.CodeOf(err) != camera.CodeDisconnected {
		t.Fatalf("capture after unplug: %v", err)
	}
	if err := c.Disconnect(); err != nil {
		t.Fatalf("Disconnect after unplug must succeed: %v", err)
	}
	if f.leaked() != 0 {
		t.Fatalf("handle leak: %d", f.leaked())
	}
	if len(f.violations) > 0 {
		t.Fatal(f.violations)
	}
}

func TestCleanShutdownLeavesNoHandlesAndLiveViewOff(t *testing.T) {
	f := newFake()
	_, c := connect(t, f)
	_ = c.StartLiveView(context.Background())
	if !f.evfOn {
		t.Fatal("live view should be on")
	}
	if err := c.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if f.evfOn || !f.closed {
		t.Fatalf("camera left busy: evf=%v closed=%v", f.evfOn, f.closed)
	}
	if f.leaked() != 0 {
		t.Fatalf("leaked %d handles", f.leaked())
	}
	if err := c.Disconnect(); err != nil {
		t.Fatal(err)
	}
}

func TestStructLayouts(t *testing.T) {
	if s := unsafe.Sizeof(DeviceInfoRaw{}); s != 520 {
		t.Errorf("EdsDeviceInfo size %d, want 520", s)
	}
	if s := unsafe.Sizeof(DirItemInfo{}); s != 288 {
		t.Errorf("EdsDirectoryItemInfo size %d, want 288 on x64", s)
	}
	if s := unsafe.Sizeof(Capacity{}); s != 12 {
		t.Errorf("EdsCapacity size %d, want 12", s)
	}
}

func TestErrorTranslationIsActionable(t *testing.T) {
	cases := map[uint32]string{
		errDeviceBusy: camera.CodeDeviceBusy, errCommDisconnected: camera.CodeDisconnected, errCommDeviceIncompat: camera.CodeUnsupported,
		errTakePictureNoCard: camera.CodeCaptureFailed, 0x12345: camera.CodeCaptureFailed,
	}
	for code, want := range cases {
		err := translate("op", &sdkError{Op: "op", Code: code}, nil)
		if camera.CodeOf(err) != want || camera.ActionOf(err) == "" {
			t.Errorf("0x%X -> %q action=%q", code, camera.CodeOf(err), camera.ActionOf(err))
		}
		if !errors.Is(err, err) || !strings.Contains(err.Error(), "op") {
			t.Errorf("message lost: %v", err)
		}
	}
	if got := matchOwners([]string{"chrome.exe", "eos utility 3.EXE"}); len(got) != 1 {
		t.Errorf("case-insensitive match failed: %v", got)
	}
}

// minimal PE header with the given machine type
func writePE(t *testing.T, path string, machine uint16) {
	t.Helper()
	b := make([]byte, 0x100)
	b[0], b[1] = 'M', 'Z'
	binary.LittleEndian.PutUint32(b[0x3C:], 0x80)
	copy(b[0x80:], "PE\x00\x00")
	binary.LittleEndian.PutUint16(b[0x84:], machine)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLocateSDK(t *testing.T) {
	dir := t.TempDir()
	if _, err := locateSDK("", dir); err == nil {
		t.Fatal("empty dir must report SDK missing")
	}
	sub := filepath.Join(dir, "sdk", "canon")
	_ = os.MkdirAll(sub, 0o755)
	writePE(t, filepath.Join(sub, "EDSDK.dll"), 0x014c) // 32-bit
	_ = os.WriteFile(filepath.Join(sub, "EdsImage.dll"), []byte("x"), 0o644)
	if _, err := locateSDK("", dir); err == nil || !strings.Contains(err.Error(), "64-bit") {
		t.Fatalf("32-bit DLL must be rejected with a clear reason: %v", err)
	}
	writePE(t, filepath.Join(sub, "EDSDK.dll"), 0x8664)
	p, err := locateSDK("", dir)
	if err != nil || filepath.Base(p) != "EDSDK.dll" {
		t.Fatalf("%v %v", p, err)
	}
	_ = os.Remove(filepath.Join(sub, "EdsImage.dll"))
	if _, err := locateSDK("", dir); err == nil || !strings.Contains(err.Error(), "EdsImage.dll") {
		t.Fatalf("missing EdsImage.dll: %v", err)
	}
}

var _ = strconv.Itoa

func TestKeepAliveExtendsThePowerOffTimerAndStopsOnDisconnect(t *testing.T) {
	f := newFake()
	_, c := connect(t, f)
	_ = c
	f.mu.Lock()
	f.extends = 0
	f.mu.Unlock()
	// the camera warns that it will shut down soon → immediate extension
	f.mu.Lock()
	f.pending = append(f.pending, func() { f.onState(evStateWillSoonShutDown, 0) })
	f.mu.Unlock()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && f.extendCount() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if f.extendCount() == 0 {
		t.Fatal("a 'will soon shut down' warning must extend the timer")
	}
	if err := c.Disconnect(); err != nil {
		t.Fatal(err)
	}
	n := f.extendCount()
	time.Sleep(100 * time.Millisecond)
	if f.extendCount() != n {
		t.Fatal("keep-alive kept running after Disconnect")
	}
}

func TestPeriodicKeepAlive(t *testing.T) {
	f := newFake()
	d := driverWith(f, nil)
	defer d.Close()
	devs, _ := d.Discover(context.Background())
	cam, _ := d.Open(devs[0])
	c := cam.(*Camera)
	c.KeepAliveInterval = 20 * time.Millisecond
	if err := c.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer c.Disconnect()
	time.Sleep(200 * time.Millisecond)
	if f.extendCount() < 3 {
		t.Fatalf("only %d keep-alive commands in 200 ms at a 20 ms interval", f.extendCount())
	}
	if len(f.violations) > 0 {
		t.Fatal(f.violations)
	}
}
