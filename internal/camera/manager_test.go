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
