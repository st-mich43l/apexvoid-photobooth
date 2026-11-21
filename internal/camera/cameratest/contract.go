// Package cameratest holds the adapter contract: every camera driver
// (mock, UVC, Canon EDSDK) must pass RunContract. Hardware drivers run it
// only when the matching environment variable is set.
package cameratest

import (
	"bytes"
	"context"
	"image/jpeg"
	"testing"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
)

// Options tunes the contract for a driver.
type Options struct {
	// WantSimulated is true only for the mock driver: captures must be flagged.
	WantSimulated bool
	Timeout       time.Duration
}

// RunContract exercises the behaviour every Camera must have.
func RunContract(t *testing.T, drv camera.Driver, o Options) {
	t.Helper()
	if o.Timeout == 0 {
		o.Timeout = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), o.Timeout)
	defer cancel()

	if err := drv.Available(); err != nil {
		t.Skipf("driver %s unavailable: %v", drv.Name(), err)
	}
	devs, err := drv.Discover(ctx)
	if err != nil || len(devs) == 0 {
		t.Skipf("driver %s: no devices (%v)", drv.Name(), err)
	}
	for _, d := range devs {
		if d.Driver != drv.Name() || d.ID == "" || d.Name == "" {
			t.Fatalf("malformed DeviceInfo %+v", d)
		}
		if d.Kind == "" {
			t.Fatalf("device %q has no Kind: the Camera Sources screen cannot label it", d.Name)
		}
		if d.Simulated != o.WantSimulated {
			t.Fatalf("Simulated=%v, want %v", d.Simulated, o.WantSimulated)
		}
	}

	t.Run("operations before connect fail with actionable error", func(t *testing.T) {
		cam, err := drv.Open(devs[0])
		if err != nil {
			t.Fatal(err)
		}
		defer cam.Disconnect()
		if _, err := cam.Capture(ctx); err == nil {
			t.Fatal("Capture before Connect must fail")
		}
	})

	cam, err := drv.Open(devs[0])
	if err != nil {
		t.Fatal(err)
	}
	defer cam.Disconnect()
	if err := cam.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	caps := cam.Capabilities()
	if caps.StillCapture == "" || caps.LiveView == "" {
		t.Fatalf("capabilities must be explicit: %+v", caps)
	}

	t.Run("live view delivers advancing JPEG frames", func(t *testing.T) {
		if caps.LiveView == camera.Unsupported {
			t.Skip("live view not supported")
		}
		if err := cam.StartLiveView(ctx); err != nil {
			t.Fatal(err)
		}
		var seq uint64
		for i := 0; i < 3; i++ {
			fctx, c := context.WithTimeout(ctx, 10*time.Second)
			f, err := cam.WaitFrame(fctx, seq)
			c()
			if err != nil {
				t.Fatalf("frame %d: %v", i, err)
			}
			if f.Seq <= seq {
				t.Fatalf("sequence did not advance: %d <= %d", f.Seq, seq)
			}
			if _, err := jpeg.Decode(bytes.NewReader(f.JPEG)); err != nil {
				t.Fatalf("frame is not a JPEG: %v", err)
			}
			seq = f.Seq
		}
		if err := cam.StopLiveView(); err != nil {
			t.Fatal(err)
		}
		if err := cam.StopLiveView(); err != nil {
			t.Fatalf("StopLiveView must be idempotent: %v", err)
		}
	})

	t.Run("capture returns a decodable original", func(t *testing.T) {
		if caps.StillCapture == camera.Unsupported {
			t.Skip("still capture not supported")
		}
		// live view must be on for webcam-style drivers; harmless for others
		_ = cam.StartLiveView(ctx)
		p, err := cam.Capture(ctx)
		if err != nil {
			t.Fatalf("Capture: %v", err)
		}
		if p.Simulated != o.WantSimulated {
			t.Fatalf("Photo.Simulated=%v want %v", p.Simulated, o.WantSimulated)
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(p.Data))
		if err != nil || cfg.Width < 16 {
			t.Fatalf("original is not a JPEG: %v %+v", err, cfg)
		}
	})

	t.Run("cancelled capture returns promptly", func(t *testing.T) {
		if caps.StillCapture == camera.Unsupported {
			t.Skip()
		}
		cctx, c := context.WithCancel(ctx)
		c()
		done := make(chan struct{})
		go func() { _, _ = cam.Capture(cctx); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Capture ignored context cancellation")
		}
	})

	t.Run("disconnect is idempotent and closes events", func(t *testing.T) {
		if err := cam.Disconnect(); err != nil {
			t.Fatal(err)
		}
		if err := cam.Disconnect(); err != nil {
			t.Fatalf("second Disconnect: %v", err)
		}
		select {
		case _, ok := <-cam.Events():
			if ok {
				// drain any queued event
				for range cam.Events() {
				}
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Events channel not closed after Disconnect")
		}
		if cam.State() != camera.StateDisconnected {
			t.Fatalf("state after Disconnect: %s", cam.State())
		}
		if _, err := cam.Capture(ctx); err == nil {
			t.Fatal("Capture after Disconnect must fail")
		}
	})
}
