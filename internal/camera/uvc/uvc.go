// Package uvc is the webcam / UVC adapter. It drives any camera that shows up
// as an operating-system video device (USB webcams, capture cards, and a
// Canon EOS body in USB "streaming"/UVC mode) through FFmpeg: DirectShow on
// Windows, V4L2 on Linux. Stills are frames grabbed from the stream, not
// camera originals, and the capability model says so.
package uvc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
	"github.com/st-mich43l/apexvoid-photobooth/internal/video"
)

const DriverName = "uvc"

// Prefs are the requested capture parameters.
type Prefs struct {
	Width, Height, FPS int
}

// Driver discovers capture devices through FFmpeg.
type Driver struct {
	// FFmpegPath returns the configured ffmpeg path ("" = auto-locate).
	FFmpegPath func() string
	// Prefs returns the requested resolution/frame rate.
	Prefs func() Prefs

	mu      sync.Mutex
	devices map[string]video.Device

	// test hooks
	listFn   func(ctx context.Context, ffmpeg string, modes bool) (video.Devices, error)
	override []string
}

// NewDriver creates the driver. Both funcs may be nil.
func NewDriver(ffmpegPath func() string, prefs func() Prefs) *Driver {
	return &Driver{FFmpegPath: ffmpegPath, Prefs: prefs, devices: map[string]video.Device{}}
}

func (d *Driver) Name() string { return DriverName }

func (d *Driver) ffmpeg() (string, error) {
	cfg := ""
	if d.FFmpegPath != nil {
		cfg = d.FFmpegPath()
	}
	p, err := video.Locate(cfg)
	if err != nil {
		return "", camera.Errf(camera.CodeSDKMissing,
			"Place ffmpeg.exe next to the application (or set its path in Settings). See docs/CAMERAS.md.", err,
			"FFmpeg was not found, so webcams and video recording are unavailable")
	}
	return p, nil
}

// Available reports whether FFmpeg can be found.
func (d *Driver) Available() error { _, err := d.ffmpeg(); return err }

// Discover lists video capture devices.
func (d *Driver) Discover(ctx context.Context) ([]camera.DeviceInfo, error) {
	ff, err := d.ffmpeg()
	if err != nil {
		return nil, err
	}
	list := video.ListDevices
	if d.listFn != nil {
		list = d.listFn
	}
	devs, err := list(ctx, ff, true)
	if err != nil {
		return nil, camera.Errf(camera.CodeNoDevice, "Check that the camera is connected.", err, "could not list video devices")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.devices = map[string]video.Device{}
	var out []camera.DeviceInfo
	for _, v := range devs.Video {
		id := DriverName + ":" + v.ID
		d.devices[id] = v
		out = append(out, camera.DeviceInfo{ID: id, Name: v.Name, Driver: DriverName, Port: v.ID})
	}
	return out, nil
}

// Open creates a Camera for a discovered device.
func (d *Driver) Open(info camera.DeviceInfo) (camera.Camera, error) {
	ff, err := d.ffmpeg()
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	dev, ok := d.devices[info.ID]
	d.mu.Unlock()
	if !ok {
		return nil, camera.Errf(camera.CodeNoDevice, "Press Refresh in the camera list.", nil, "video device %q is no longer present", info.Name)
	}
	var p Prefs
	if d.Prefs != nil {
		p = d.Prefs()
	}
	return &Camera{info: info, dev: dev, ffmpeg: ff, prefs: p, override: d.override,
		frames: camera.NewFrameBuffer(), events: make(chan camera.Event, 4), state: camera.StateDisconnected}, nil
}

// ChooseMode picks the capture mode closest to the request: the best mode
// that does not exceed the requested size (or the smallest above it), preferring
// camera-compressed MJPEG at HD+ because raw YUV at 1080p is capped at a few fps
// on USB 2.0.
func ChooseMode(modes []video.Mode, w, h, fps int) (video.Mode, bool) {
	if len(modes) == 0 {
		return video.Mode{}, false
	}
	score := func(m video.Mode) float64 {
		s := 0.0
		area, want := float64(m.Width*m.Height), float64(w*h)
		if area > want {
			s += 1000 + (area-want)/1000
		} else {
			s += (want - area) / 1000
		}
		if fps > 0 && m.FPSMax > 0 && m.FPSMax < float64(fps) {
			s += 200 + (float64(fps)-m.FPSMax)*10
		}
		if m.Width >= 1280 && m.Format != "mjpeg" {
			s += 150
		}
		return s
	}
	best := modes[0]
	for _, m := range modes[1:] {
		if score(m) < score(best) {
			best = m
		}
	}
	return best, true
}

// Camera is one opened UVC device.
type Camera struct {
	info     camera.DeviceInfo
	dev      video.Device
	ffmpeg   string
	prefs    Prefs
	override []string

	frames *camera.FrameBuffer
	events chan camera.Event

	mu         sync.Mutex
	state      camera.State
	closed     bool
	stream     *video.Stream
	mode       video.Mode
	onRecEnd   func(video.RecordResult)
	liveStart  time.Time
	eventsDone bool
}

func (c *Camera) Info() camera.DeviceInfo { return c.info }

func (c *Camera) Capabilities() camera.Capabilities {
	notes := []string{
		"Still photos are frames grabbed from the video stream at the stream resolution, not camera originals.",
		"Video is encoded on the PC by FFmpeg from the streamed frames.",
	}
	if strings.Contains(strings.ToLower(c.dev.Name), "canon") || strings.Contains(strings.ToLower(c.dev.Name), "eos") {
		notes = append(notes, "Canon body in USB streaming (UVC) mode: use the Canon EDSDK adapter instead for full-resolution originals.")
	}
	return camera.Capabilities{
		LiveView: camera.Supported, StillCapture: camera.Supported, OriginalTransfer: camera.Unsupported,
		MovieRecord: camera.Unsupported, MovieTransfer: camera.Unsupported, StreamedVideo: camera.Supported, Notes: notes,
	}
}

func (c *Camera) State() camera.State         { c.mu.Lock(); defer c.mu.Unlock(); return c.state }
func (c *Camera) Events() <-chan camera.Event { return c.events }

// Mode returns the active capture mode (zero before live view).
func (c *Camera) Mode() video.Mode { c.mu.Lock(); defer c.mu.Unlock(); return c.mode }

func (c *Camera) Connect(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return camera.Errf(camera.CodeNotConnected, "", nil, "camera was released")
	}
	c.state = camera.StateConnected
	return nil
}

func (c *Camera) source() video.Source {
	w, h, fps := c.prefs.Width, c.prefs.Height, c.prefs.FPS
	if w == 0 {
		w, h = 1280, 720
	}
	if fps == 0 {
		fps = 30
	}
	src := video.Source{Video: c.dev, Width: w, Height: h, FPS: fps, InputOverride: c.override, AudioMap: "1:a"}
	if m, ok := ChooseMode(c.dev.Modes, w, h, fps); ok && c.override == nil {
		src.Width, src.Height, src.Format = m.Width, m.Height, m.Format
		if m.FPSMax > 0 && float64(src.FPS) > m.FPSMax {
			src.FPS = int(m.FPSMax)
		}
		c.mode = m
	}
	return src
}

func (c *Camera) StartLiveView(ctx context.Context) error {
	c.mu.Lock()
	if c.closed || c.state == camera.StateDisconnected || c.state == camera.StateError {
		c.mu.Unlock()
		return camera.Errf(camera.CodeNotConnected, "Connect the camera first.", nil, "camera is not connected")
	}
	if c.stream != nil {
		c.mu.Unlock()
		return nil
	}
	src := c.source()
	c.mu.Unlock()

	var s *video.Stream
	s = video.NewStream(video.StreamOptions{
		FFmpeg: c.ffmpeg, Source: src,
		OnFrame: func(b []byte) { c.frames.Publish(b) },
		OnLost:  func(err error, detail string) { c.lost(s, err, detail) },
		OnRecordEnd: func(r video.RecordResult) {
			c.mu.Lock()
			f := c.onRecEnd
			c.mu.Unlock()
			if f != nil {
				f(r)
			}
		},
	})
	if err := s.Start(ctx); err != nil {
		return camera.Errf(camera.CodeDeviceBusy, "Close other programs using the camera (EOS Utility, OBS, Zoom, browsers) and retry. For a Canon body, set USB streaming mode.", err, "could not start the camera preview")
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		s.Stop()
		return camera.Errf(camera.CodeNotConnected, "", nil, "camera was released")
	}
	c.stream, c.state, c.liveStart = s, camera.StateLiveView, time.Now()
	c.mu.Unlock()
	return nil
}

func (c *Camera) lost(s *video.Stream, err error, detail string) {
	c.mu.Lock()
	if c.stream != s || c.closed {
		c.mu.Unlock()
		return
	}
	c.stream, c.state = nil, camera.StateError
	c.mu.Unlock()
	c.frames.Reset()
	select {
	case c.events <- camera.Event{Kind: camera.EventDisconnected, Err: fmt.Errorf("camera stream ended: %s", detail)}:
	default:
	}
}

func (c *Camera) StopLiveView() error {
	c.mu.Lock()
	s := c.stream
	c.stream = nil
	if c.state == camera.StateLiveView {
		c.state = camera.StateConnected
	}
	c.mu.Unlock()
	if s != nil {
		s.Stop()
	}
	c.frames.Reset()
	return nil
}

func (c *Camera) WaitFrame(ctx context.Context, after uint64) (camera.Frame, error) {
	return c.frames.Wait(ctx, after)
}

// Capture returns the first frame that arrives after the call (never a stale
// frame from before the countdown ended).
func (c *Camera) Capture(ctx context.Context) (camera.Photo, error) {
	c.mu.Lock()
	st, s := c.state, c.stream
	c.mu.Unlock()
	if st == camera.StateDisconnected || st == camera.StateError || c.isClosed() {
		return camera.Photo{}, camera.Errf(camera.CodeDisconnected, "Reconnect the camera.", nil, "camera is not connected")
	}
	if s == nil {
		if err := c.StartLiveView(ctx); err != nil {
			return camera.Photo{}, err
		}
		select { // let auto-exposure / white balance settle
		case <-ctx.Done():
			return camera.Photo{}, ctx.Err()
		case <-time.After(700 * time.Millisecond):
		}
	}
	cur, _ := c.frames.Latest()
	fctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	f, err := c.frames.Wait(fctx, cur.Seq)
	if err != nil {
		if ctx.Err() != nil {
			return camera.Photo{}, ctx.Err()
		}
		return camera.Photo{}, camera.Errf(camera.CodeTimeout, "Check the camera connection and try again.", err, "no frame arrived from the camera")
	}
	return camera.Photo{Data: f.JPEG, Source: "UVC frame grab"}, nil
}

func (c *Camera) isClosed() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.closed }

func (c *Camera) Disconnect() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	s := c.stream
	c.stream = nil
	c.state = camera.StateDisconnected
	c.mu.Unlock()
	if s != nil {
		s.Stop() // finalises an active recording
	}
	close(c.events)
	return nil
}

// --- video.Recordable ---

var _ video.Recordable = (*Camera)(nil)

func (c *Camera) liveStream() (*video.Stream, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stream == nil {
		return nil, camera.Errf(camera.CodeLiveViewOff, "Start the preview before recording.", nil, "live view is not running")
	}
	return c.stream, nil
}

func (c *Camera) StartRecording(ctx context.Context, rec video.RecordOptions) error {
	s, err := c.liveStream()
	if err != nil {
		return err
	}
	if err := s.StartRecording(ctx, rec); err != nil {
		return camera.Errf(camera.CodeCaptureFailed, "Check the microphone selection and free disk space.", err, "could not start recording")
	}
	return nil
}

func (c *Camera) StopRecording(ctx context.Context) (video.RecordResult, error) {
	s, err := c.liveStream()
	if err != nil {
		return video.RecordResult{}, err
	}
	return s.StopRecording(ctx)
}

func (c *Camera) Recording() (bool, time.Time) {
	c.mu.Lock()
	s := c.stream
	c.mu.Unlock()
	if s == nil {
		return false, time.Time{}
	}
	return s.Recording()
}

func (c *Camera) SetOnRecordEnd(f func(video.RecordResult)) {
	c.mu.Lock()
	c.onRecEnd = f
	c.mu.Unlock()
}

var _ = errors.New
