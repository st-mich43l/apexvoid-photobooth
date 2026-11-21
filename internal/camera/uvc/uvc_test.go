package uvc

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
	"github.com/st-mich43l/apexvoid-photobooth/internal/camera/cameratest"
	"github.com/st-mich43l/apexvoid-photobooth/internal/video"
)

func syntheticDriver(t *testing.T) *Driver {
	t.Helper()
	if _, err := video.Locate(""); err != nil {
		t.Skip("ffmpeg not installed")
	}
	d := NewDriver(nil, func(string) Prefs { return Prefs{Width: 640, Height: 360, FPS: 30} })
	d.native = nil
	d.listFn = func(context.Context, string, bool) (video.Devices, error) {
		return video.Devices{Video: []video.Device{{Name: "Synthetic test camera", ID: "synthetic", Kind: "video"}}}, nil
	}
	d.override = []string{"-re", "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30", "-re", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000"}
	return d
}

// The UVC adapter passes the same contract as every other driver, driven by
// a synthetic FFmpeg source (the device-open step is the only part not covered).
func TestContractWithSyntheticSource(t *testing.T) {
	cameratest.RunContract(t, syntheticDriver(t), cameratest.Options{})
}

func TestRecordThroughCamera(t *testing.T) {
	d := syntheticDriver(t)
	ctx := context.Background()
	devs, _ := d.Discover(ctx)
	cam, err := d.Open(devs[0])
	if err != nil {
		t.Fatal(err)
	}
	defer cam.Disconnect()
	_ = cam.Connect(ctx)
	rc := cam.(video.Recordable)
	enc, err := video.PickEncoder(ctx, mustFF(t))
	if err != nil {
		t.Skip(err)
	}
	if err := rc.StartRecording(ctx, video.RecordOptions{Path: filepath.Join(t.TempDir(), "a.mp4"), Encoder: enc}); err == nil {
		t.Fatal("recording without live view must fail")
	}
	if err := cam.StartLiveView(ctx); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "b.mp4")
	if err := rc.StartRecording(ctx, video.RecordOptions{Path: out, Encoder: enc, Audio: "x"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1500 * time.Millisecond)
	res, err := rc.StopRecording(ctx)
	if err != nil || res.Partial {
		t.Fatalf("%v %+v", err, res)
	}
	p, err := video.ProbeFile(ctx, mustFF(t), out)
	if err != nil || !p.HasVideo || !p.HasAudio {
		t.Fatalf("%v %+v", err, p)
	}
}

func mustFF(t *testing.T) string {
	p, err := video.Locate("")
	if err != nil {
		t.Skip("ffmpeg missing")
	}
	return p
}

func TestChooseMode(t *testing.T) {
	modes := []video.Mode{
		{Width: 1920, Height: 1080, FPSMax: 5, Format: "yuyv422"},
		{Width: 1920, Height: 1080, FPSMax: 30, Format: "mjpeg"},
		{Width: 1280, Height: 720, FPSMax: 30, Format: "mjpeg"},
		{Width: 640, Height: 480, FPSMax: 30, Format: "yuyv422"},
	}
	if m, _ := ChooseMode(modes, 1280, 720, 30); m.Width != 1280 || m.Format != "mjpeg" {
		t.Fatalf("720p30 request: %+v", m)
	}
	if m, _ := ChooseMode(modes, 1920, 1080, 30); m.Width != 1920 || m.Format != "mjpeg" {
		t.Fatalf("1080p30 should pick MJPEG not 5 fps raw: %+v", m)
	}
	if m, _ := ChooseMode(modes, 640, 480, 30); m.Width != 640 {
		t.Fatalf("%+v", m)
	}
	if _, ok := ChooseMode(nil, 1, 1, 1); ok {
		t.Fatal("empty modes")
	}
}

func TestUnavailableWithoutFFmpeg(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	d := NewDriver(func() string { return "/definitely/not/here" }, nil)
	if err := d.Available(); err == nil {
		t.Skip("an ffmpeg beside the test binary was found")
	}
}

func TestMergeNativeAndFFmpegLists(t *testing.T) {
	native := []NativeDevice{
		{Name: "Integrated Webcam", InstanceID: `USB\VID_0BDA&PID_5830&MI_00\6&1F&0&1`},
		{Name: "Canon EOS R50", InstanceID: `USB\VID_04A9&PID_32EA&MI_00\7&2B&0&3`},
		{Name: "OBS Virtual Camera", Virtual: true},
		{Name: "Unknown Cam", InstanceID: `USB\VID_1234&PID_0001\9&9&9&9`},
	}
	ff := []video.Device{
		{Name: "Integrated Webcam", ID: `@device_pnp_\\?\usb#vid_0bda&pid_5830&mi_00#6&1f&0&1#{x}\global`, Kind: "video",
			USBID: "0bda:5830", Key: "usb#vid_0bda&pid_5830#6&1f&0&1", Modes: []video.Mode{{Width: 1280, Height: 720, FPSMax: 30, Format: "mjpeg"}}},
		{Name: "Canon EOS R50", ID: `@device_pnp_\\?\usb#vid_04a9&pid_32ea&mi_00#7&2b&0&3#{x}\global`, Kind: "video",
			USBID: "04a9:32ea", Key: "usb#vid_04a9&pid_32ea#7&2b&0&3"},
		{Name: "OBS Virtual Camera", ID: "@device_sw_{860BB310}\\{A3FCE0F5}", Kind: "video"},
		{Name: "Only In DirectShow", ID: "@device_sw_{860BB310}\\{ZZ}", Kind: "video"},
	}
	m := mergeDevices(native, ff)
	if len(m) != 5 {
		t.Fatalf("want 4 native (matched) + 1 dshow-only, got %d: %+v", len(m), m)
	}
	if m[0].Name != "Integrated Webcam" || len(m[0].Modes) != 1 || !strings.HasPrefix(m[0].ID, "@device_pnp_") {
		t.Fatalf("native entry must adopt FFmpeg's exact name/modes: %+v", m[0])
	}
	if m[3].Name != "Unknown Cam" || m[3].USBID != "1234:0001" {
		t.Fatalf("native-only device lost: %+v", m[3])
	}
	// without FFmpeg only the native list remains, still with stable keys
	nf := mergeDevices(native, nil)
	if len(nf) != 4 || nf[1].USBID != "04a9:32ea" || nf[1].Key == "" {
		t.Fatalf("%+v", nf)
	}
}

func TestClassification(t *testing.T) {
	cases := []struct {
		name, usb, kind, vendor string
		soft                    bool
	}{
		{"Integrated Webcam", "0bda:5830", camera.KindWebcam, "", false},
		{"Logitech C920", "046d:082d", camera.KindWebcam, "", false},
		{"Canon EOS R50", "04a9:32ea", camera.KindCanonUVC, "Canon", false},
		{"EOS Webcam Utility Pro", "", camera.KindVirtual, "Canon", true},
		{"OBS Virtual Camera", "", camera.KindVirtual, "", true},
		{"Cam Link 4K", "0fd9:0066", camera.KindCaptureCard, "", false},
		{"USB Video", "534d:2109", camera.KindWebcam, "", false},
	}
	for _, c := range cases {
		k, v := Classify(c.name, c.usb, c.soft)
		if k != c.kind || v != c.vendor {
			t.Errorf("%s: got %s/%s want %s/%s", c.name, k, v, c.kind, c.vendor)
		}
	}
}

func TestStableIDsSurviveRefreshAndDuplicateNames(t *testing.T) {
	a := []video.Device{{Name: "USB Camera", Key: "usb#vid_1&pid_2#a"}, {Name: "USB Camera", Key: "usb#vid_1&pid_2#b"}, {Name: "Webcam"}, {Name: "Webcam"}}
	ids1 := stableIDs(a)
	// the OS lists them in a different order next time
	b := []video.Device{a[3], a[1], a[0], a[2]}
	ids2 := stableIDs(b)
	if ids1[0] != ids2[2] || ids1[1] != ids2[1] {
		t.Fatalf("keyed ids must not depend on list order: %v vs %v", ids1, ids2)
	}
	if ids1[0] == ids1[1] || ids1[2] == ids1[3] {
		t.Fatalf("duplicates must get distinct ids: %v", ids1)
	}
}

func TestDiscoverWorksWithoutFFmpegOnWindowsLikeNativeEnumeration(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	d := NewDriver(func() string { return "/no/ffmpeg" }, nil)
	d.native = func(context.Context) ([]NativeDevice, error) {
		return []NativeDevice{{Name: "Integrated Webcam", InstanceID: `USB\VID_0BDA&PID_5830&MI_00\6&1F&0&1`}}, nil
	}
	if _, err := video.Locate("/no/ffmpeg"); err == nil {
		t.Skip("an ffmpeg beside the test binary exists")
	}
	if err := d.Available(); err != nil {
		t.Fatalf("listing must not require FFmpeg: %v", err)
	}
	devs, err := d.Discover(context.Background())
	if err != nil || len(devs) != 1 || devs[0].Kind != camera.KindWebcam || !strings.Contains(devs[0].Note, "FFmpeg") {
		t.Fatalf("%v %+v", err, devs)
	}
	cam, err := d.Open(devs[0])
	if err != nil {
		t.Fatal(err)
	}
	defer cam.Disconnect()
	if err := cam.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	caps := cam.Capabilities()
	if caps.LiveView != camera.Unsupported || caps.StillCapture != camera.Unsupported {
		t.Fatalf("without FFmpeg preview/capture must be reported unsupported: %+v", caps)
	}
	err = cam.StartLiveView(context.Background())
	if camera.CodeOf(err) != camera.CodeSDKMissing || camera.ActionOf(err) == "" {
		t.Fatalf("want actionable FFmpeg error, got %v", err)
	}
}

func TestPhotoIsDescribedAsFrameGrab(t *testing.T) {
	d := syntheticDriver(t)
	ctx := context.Background()
	devs, _ := d.Discover(ctx)
	cam, _ := d.Open(devs[0])
	defer cam.Disconnect()
	_ = cam.Connect(ctx)
	_ = cam.StartLiveView(ctx)
	p, err := cam.Capture(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.Source, "frame grab") || strings.Contains(strings.ToLower(p.Source), "original transfer") {
		t.Fatalf("%q", p.Source)
	}
	if cam.Capabilities().OriginalTransfer != camera.Unsupported {
		t.Fatal("a webcam must never claim to deliver camera originals")
	}
}

// A webcam that stops delivering frames (USB unplug) is detected, the manager
// reconnects the same source by its stable id and the preview comes back.
func TestUnplugAndReplugRestoresPreviewThroughManager(t *testing.T) {
	d := syntheticDriver(t)
	m := camera.NewManager(d)
	m.ReconnectInterval = 50 * time.Millisecond
	ctx := context.Background()
	devs, err := m.Discover(ctx)
	if err != nil || len(devs) != 1 {
		t.Fatalf("%v %v", devs, err)
	}
	if err := m.Connect(ctx, devs[0]); err != nil {
		t.Fatal(err)
	}
	defer m.Shutdown()
	if err := m.StartLiveView(ctx); err != nil {
		t.Fatal(err)
	}
	fctx, c := context.WithTimeout(ctx, 10*time.Second)
	f, err := m.WaitFrame(fctx, 0)
	c()
	if err != nil {
		t.Fatal(err)
	}
	// simulate the unplug: the capture process dies
	m.Camera().(*Camera).streamForTest().Kill()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !m.Status().Reconnecting {
		time.Sleep(20 * time.Millisecond)
	}
	if st := m.Status(); !st.Reconnecting || st.LastError == "" || st.LastAction == "" {
		t.Fatalf("loss not reported with guidance: %+v", st)
	}
	deadline = time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) && (m.Status().Reconnecting || !m.Status().LiveView) {
		time.Sleep(50 * time.Millisecond)
	}
	if st := m.Status(); st.Reconnecting || !st.LiveView || st.Device == nil || st.Device.ID != devs[0].ID {
		t.Fatalf("not reconnected to the same source: %+v", st)
	}
	fctx, c = context.WithTimeout(ctx, 10*time.Second)
	defer c()
	if _, err := m.WaitFrame(fctx, f.Seq); err != nil {
		t.Fatalf("no frames after replug: %v", err)
	}
	if _, err := m.Capture(ctx); err != nil {
		t.Fatalf("capture after replug: %v", err)
	}
}
