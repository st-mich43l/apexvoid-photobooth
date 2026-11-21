package uvc

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera/cameratest"
	"github.com/st-mich43l/apexvoid-photobooth/internal/video"
)

func syntheticDriver(t *testing.T) *Driver {
	t.Helper()
	if _, err := video.Locate(""); err != nil {
		t.Skip("ffmpeg not installed")
	}
	d := NewDriver(nil, func() Prefs { return Prefs{Width: 640, Height: 360, FPS: 30} })
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
