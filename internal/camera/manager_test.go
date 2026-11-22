package camera_test

import (
	"context"
	"testing"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
	"github.com/st-mich43l/apexvoid-photobooth/internal/camera/mock"
)

func connected(t *testing.T) (*camera.Manager, *mock.Driver) {
	t.Helper()
	d := mock.NewDriver()
	d.Width, d.Height, d.CaptureDelay = 160, 120, time.Millisecond
	m := camera.NewManager(d)
	m.ReconnectInterval = 10 * time.Millisecond
	devs, err := m.Discover(context.Background())
	if err != nil || len(devs) != 1 {
		t.Fatalf("discover: %v %v", devs, err)
	}
	if err := m.Connect(context.Background(), devs[0]); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Shutdown() })
	return m, d
}

func waitFor(t *testing.T, what string, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if f() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout: %s", what)
}

func TestOperationsWithoutCameraReturnActionableError(t *testing.T) {
	m := camera.NewManager(mock.NewDriver())
	if _, err := m.Capture(context.Background()); camera.CodeOf(err) != camera.CodeNotConnected || camera.ActionOf(err) == "" {
		t.Fatalf("err=%v", err)
	}
	if err := m.StartLiveView(context.Background()); camera.CodeOf(err) != camera.CodeNotConnected {
		t.Fatalf("err=%v", err)
	}
}

func TestUnplugReconnectsAndRestoresLiveView(t *testing.T) {
	m, d := connected(t)
	if err := m.StartLiveView(context.Background()); err != nil {
		t.Fatal(err)
	}
	first := d.Last()
	d.SetPresent(false)
	waitFor(t, "disconnect detected", func() bool { return m.Status().Reconnecting })
	st := m.Status()
	if st.LastError == "" || st.LastAction == "" || st.Device == nil {
		t.Fatalf("status must explain the disconnect: %+v", st)
	}
	if !first.Released() {
		t.Fatal("lost camera handles must be released")
	}
	if _, err := m.Capture(context.Background()); camera.CodeOf(err) != camera.CodeNotConnected {
		t.Fatalf("capture while lost: %v", err)
	}
	d.SetPresent(true)
	waitFor(t, "reconnected", func() bool { s := m.Status(); return !s.Reconnecting && s.State != camera.StateDisconnected })
	waitFor(t, "live view restored", func() bool { return m.Status().LiveView })
	ctx, c := context.WithTimeout(context.Background(), 3*time.Second)
	defer c()
	if _, err := m.WaitFrame(ctx, 0); err != nil {
		t.Fatalf("no frames after reconnect: %v", err)
	}
	if _, err := m.Capture(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSecondCaptureRejectedWhileFirstRuns(t *testing.T) {
	m, d := connected(t)
	d.CaptureDelay = 300 * time.Millisecond
	errc := make(chan error, 1)
	go func() { _, err := m.Capture(context.Background()); errc <- err }()
	time.Sleep(50 * time.Millisecond)
	if _, err := m.Capture(context.Background()); camera.CodeOf(err) != camera.CodeDeviceBusy {
		t.Fatalf("want busy, got %v", err)
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
}

func TestShutdownReleasesAndIsIdempotent(t *testing.T) {
	m, d := connected(t)
	_ = m.StartLiveView(context.Background())
	cam := d.Last()
	if err := m.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if !cam.Released() || m.Status().State != camera.StateDisconnected {
		t.Fatal("camera still held after Shutdown")
	}
	if err := m.Shutdown(); err != nil {
		t.Fatal(err)
	}
}

func TestExplicitDisconnectStopsReconnecting(t *testing.T) {
	m, d := connected(t)
	d.SetPresent(false)
	waitFor(t, "lost", func() bool { return m.Status().Reconnecting })
	_ = m.Disconnect()
	d.SetPresent(true)
	time.Sleep(100 * time.Millisecond)
	if m.Status().State != camera.StateDisconnected {
		t.Fatal("manager reconnected after the operator disconnected")
	}
}

func TestMarkDuplicatesAcrossBackends(t *testing.T) {
	devs := []camera.DeviceInfo{
		{ID: "canon-edsdk:usb1#0", Name: "Canon EOS R50", Driver: "canon-edsdk", Vendor: "Canon", Kind: camera.KindDSLR},
		{ID: "uvc:key-r50", Name: "Canon EOS R50", Driver: "uvc", Vendor: "Canon", USBID: "04a9:32ea", Kind: camera.KindCanonUVC},
		{ID: "uvc:key-web", Name: "Integrated Webcam", Driver: "uvc", USBID: "0bda:5830", Kind: camera.KindWebcam},
		{ID: "other:web", Name: "Integrated Webcam", Driver: "other-backend", USBID: "0bda:5830", Kind: camera.KindWebcam},
		{ID: "uvc:eos-webcam", Name: "EOS Webcam Utility", Driver: "uvc", Vendor: "Canon", Kind: camera.KindVirtual},
	}
	camera.MarkDuplicates(devs)
	if devs[1].DuplicateOf != "canon-edsdk:usb1#0" || devs[1].Note == "" {
		t.Fatalf("Canon via EDSDK and the same body as UVC must not both be openable: %+v", devs[1])
	}
	if devs[2].DuplicateOf != "" || devs[3].DuplicateOf != "uvc:key-web" {
		t.Fatalf("the same USB id reached through two backends must be flagged once: %q %q", devs[2].DuplicateOf, devs[3].DuplicateOf)
	}
	// a Windows Hello module: RGB and infrared streams share name and USB id inside one backend → two sources
	hello := []camera.DeviceInfo{
		{ID: "uvc:rgb", Name: "USB2.0 HD IR UVC WebCam", Driver: "uvc", USBID: "13d3:56cb", Kind: camera.KindWebcam},
		{ID: "uvc:ir", Name: "USB2.0 HD IR UVC WebCam", Driver: "uvc", USBID: "13d3:56cb", Kind: camera.KindWebcam},
	}
	camera.MarkDuplicates(hello)
	if hello[0].DuplicateOf != "" || hello[1].DuplicateOf != "" {
		t.Fatalf("RGB and IR streams of one USB device must both stay selectable: %+v", hello)
	}
	if devs[4].DuplicateOf != "" {
		t.Fatal("a virtual camera is a different source, not a duplicate")
	}
	if devs[0].DuplicateOf != "" {
		t.Fatal("the EDSDK entry stays the primary one")
	}
	// two Canon bodies via EDSDK: ambiguous → nothing is auto-flagged
	two := []camera.DeviceInfo{
		{ID: "e1", Driver: "canon-edsdk", Vendor: "Canon"}, {ID: "e2", Driver: "canon-edsdk", Vendor: "Canon"},
		{ID: "u1", Driver: "uvc", Vendor: "Canon", Kind: camera.KindCanonUVC},
	}
	camera.MarkDuplicates(two)
	if two[2].DuplicateOf != "" {
		t.Fatal("ambiguous Canon match must not guess")
	}
}

func TestConnectRefusesDuplicateAndBusyDevice(t *testing.T) {
	m, d := connected(t)
	devs, _ := m.Discover(context.Background())
	dup := devs[0]
	dup.DuplicateOf = "other"
	if err := m.Connect(context.Background(), dup); camera.CodeOf(err) != camera.CodeDuplicate {
		t.Fatalf("%v", err)
	}
	d.CaptureDelay = 300 * time.Millisecond
	errc := make(chan error, 1)
	go func() { _, err := m.Capture(context.Background()); errc <- err }()
	time.Sleep(60 * time.Millisecond)
	if err := m.Connect(context.Background(), devs[0]); camera.CodeOf(err) != camera.CodeBusyOperation {
		t.Fatalf("switching cameras during a capture must be refused: %v", err)
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	if err := m.Connect(context.Background(), devs[0]); err != nil {
		t.Fatalf("switching is fine once idle: %v", err)
	}
}

func TestPreferredMatching(t *testing.T) {
	devs := []camera.DeviceInfo{
		{ID: "uvc:a", Name: "USB Camera", Driver: "uvc", USBID: "1:1"},
		{ID: "uvc:b", Name: "USB Camera", Driver: "uvc", USBID: "1:1", DuplicateOf: "uvc:a"},
		{ID: "uvc:c", Name: "Integrated Webcam", Driver: "uvc", USBID: "2:2"},
		{ID: "mock-0", Name: "Sim", Driver: "mock"},
	}
	cases := []struct {
		p    camera.Preferred
		want string
	}{
		{camera.Preferred{ID: "uvc:c"}, "uvc:c"},
		{camera.Preferred{ID: "uvc:old", Driver: "uvc", USBID: "2:2", Name: "Integrated Webcam"}, "uvc:c"}, // port changed → same USB id
		{camera.Preferred{ID: "uvc:old", Driver: "uvc", Name: "Integrated Webcam"}, "uvc:c"},               // unique name
		{camera.Preferred{ID: "uvc:b"}, ""},                                                                // duplicates never match
		{camera.Preferred{ID: "gone", Driver: "uvc", Name: "USB Camera"}, "uvc:a"},                         // unique after dedupe
		{camera.Preferred{}, ""},
	}
	for i, c := range cases {
		got, ok := c.p.Match(devs)
		if (c.want == "") == ok || (ok && got.ID != c.want) {
			t.Errorf("case %d: got %q ok=%v want %q", i, got.ID, ok, c.want)
		}
	}
}

func TestPreferredMatchNeverGuessesBetweenLookAlikes(t *testing.T) {
	a := camera.DeviceInfo{ID: "canon-edsdk:A", Name: "Canon EOS R50", Driver: "canon-edsdk", USBID: "04a9:32b3"}
	b := camera.DeviceInfo{ID: "canon-edsdk:B", Name: "Canon EOS R50", Driver: "canon-edsdk", USBID: "04a9:32b3"}
	pref := camera.Preferred{ID: "canon-edsdk:gone", Name: a.Name, Driver: a.Driver, USBID: a.USBID}
	// the remembered body is gone and two identical bodies remain: no guess
	if _, ok := pref.Match([]camera.DeviceInfo{a, b}); ok {
		t.Fatal("two identical bodies share name and USB id; neither may be chosen")
	}
	// exactly one candidate: found again, e.g. after moving to another USB port
	if got, ok := pref.Match([]camera.DeviceInfo{b}); !ok || got.ID != b.ID {
		t.Fatalf("%v %v", got, ok)
	}
	// the exact id always wins
	pref.ID = a.ID
	if got, ok := pref.Match([]camera.DeviceInfo{b, a}); !ok || got.ID != a.ID {
		t.Fatalf("%v %v", got, ok)
	}
	// look-alikes were present when it was lost: only the identical id may return
	pref = camera.Preferred{ID: "canon-edsdk:gone", Name: a.Name, Driver: a.Driver, USBID: a.USBID, ExactOnly: true}
	if _, ok := pref.Match([]camera.DeviceInfo{b}); ok {
		t.Fatal("ExactOnly must not fall back to the name or USB id")
	}
	pref.ID = b.ID
	if _, ok := pref.Match([]camera.DeviceInfo{b}); !ok {
		t.Fatal("ExactOnly still matches the same id")
	}
}

func TestStatusVersionOnlyGrows(t *testing.T) {
	m := camera.NewManager()
	last := m.Status().Version
	for i := 0; i < 5; i++ {
		v := m.Status().Version
		if v <= last {
			t.Fatalf("version %d after %d: the UI could not tell a stale snapshot from a new one", v, last)
		}
		last = v
	}
}
