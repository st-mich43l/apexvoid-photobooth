// Package uvc is the webcam / UVC adapter. It drives any camera that shows up
// as an operating-system video device (USB webcams, capture cards, and a
// Canon EOS body in USB "streaming"/UVC mode) through FFmpeg: DirectShow on
// Windows, V4L2 on Linux. Stills are frames grabbed from the stream, not
// camera originals, and the capability model says so.
package uvc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
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
	// Prefs returns the requested resolution/frame rate for a device id.
	Prefs func(deviceID string) Prefs

	mu      sync.Mutex
	devices map[string]video.Device
	infos   map[string]camera.DeviceInfo

	native Enumerator // operating-system enumeration without FFmpeg (nil = none)

	// test hooks
	listFn   func(ctx context.Context, ffmpeg string, modes bool) (video.Devices, error)
	override []string
}

// NewDriver creates the driver. Both funcs may be nil.
func NewDriver(ffmpegPath func() string, prefs func(deviceID string) Prefs) *Driver {
	return &Driver{FFmpegPath: ffmpegPath, Prefs: prefs, devices: map[string]video.Device{}, infos: map[string]camera.DeviceInfo{}, native: nativeEnumerator}
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

// Available reports whether devices can be listed: natively (Windows) or via FFmpeg.
func (d *Driver) Available() error {
	if d.native != nil {
		return nil
	}
	_, err := d.ffmpeg()
	return err
}

// Discover lists video sources. On Windows the operating system is asked first
// (no FFmpeg needed to *see* a webcam); when FFmpeg is present it adds the
// supported resolutions and the exact device names it needs.
func (d *Driver) Discover(ctx context.Context) ([]camera.DeviceInfo, error) {
	var native []NativeDevice
	var nerr error
	if d.native != nil {
		native, nerr = d.native(ctx)
	}
	var ff []video.Device
	ffNote := ""
	if path, err := d.ffmpeg(); err == nil {
		list := video.ListDevices
		if d.listFn != nil {
			list = d.listFn
		}
		devs, lerr := list(ctx, path, true)
		if lerr != nil && len(native) == 0 {
			return nil, camera.Errf(camera.CodeNoDevice, "Check that the camera is connected.", lerr, "could not list video devices")
		}
		ff = devs.Video
	} else {
		ffNote = "FFmpeg is not installed: this source is listed, but preview, photos and recording need FFmpeg (see docs/CAMERAS.md)."
		if d.native == nil {
			return nil, err
		}
	}
	if nerr != nil && len(ff) == 0 && len(native) == 0 {
		return nil, camera.Errf(camera.CodeNoDevice, "Check that the camera is connected.", nerr, "could not list video devices")
	}
	merged := mergeDevices(native, ff)
	ids := stableIDs(merged)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.devices, d.infos = map[string]video.Device{}, map[string]camera.DeviceInfo{}
	out := make([]camera.DeviceInfo, 0, len(merged))
	for i, v := range merged {
		kind, vendor := Classify(v.Name, v.USBID, v.USBID == "" && d.native != nil && v.Key == "")
		info := camera.DeviceInfo{ID: ids[i], Name: v.Name, Driver: DriverName, Port: v.ID, Kind: kind, Vendor: vendor, USBID: v.USBID, Modes: v.Modes}
		if ffNote != "" {
			info.Note = ffNote
		}
		if v.USBID != "" && info.Note == "" {
			n := 0
			for _, o := range merged {
				if o.USBID == v.USBID {
					n++
				}
			}
			if n > 1 {
				info.Note = "This USB device exposes several video streams (for example a Windows Hello camera has an RGB and an infrared stream). Infrared streams do not give a normal picture: try the other entry."
			}
		}
		d.devices[ids[i]] = v
		d.infos[ids[i]] = info
		out = append(out, info)
	}
	return out, nil
}

// Open creates a Camera for a discovered device.
func (d *Driver) Open(info camera.DeviceInfo) (camera.Camera, error) {
	ff, _ := d.ffmpeg() // may be empty: the device is still listed; preview then reports "FFmpeg missing"
	d.mu.Lock()
	dev, ok := d.devices[info.ID]
	d.mu.Unlock()
	if !ok {
		return nil, camera.Errf(camera.CodeNoDevice, "Press Refresh in the camera list.", nil, "video device %q is no longer present", info.Name)
	}
	var p Prefs
	if d.Prefs != nil {
		p = d.Prefs(info.ID)
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
	sizeSeen   bool
}

func (c *Camera) Info() camera.DeviceInfo { return c.info }

func (c *Camera) Capabilities() camera.Capabilities {
	notes := []string{
		"Still photos are frame grabs from the video stream at the stream resolution — not full-resolution camera originals.",
		"Video is encoded on the PC by FFmpeg from the streamed frames.",
	}
	if c.info.Kind == camera.KindCanonUVC || c.info.Vendor == "Canon" {
		notes = append(notes, "Canon body in USB streaming (UVC) mode: use the Canon EDSDK source (photo mode) for full-resolution originals.")
	}
	if c.info.Kind == camera.KindVirtual {
		notes = append(notes, "Software (virtual) camera: it only works while its own program is running and may hold the real camera exclusively.")
	}
	if c.ffmpeg == "" && c.override == nil {
		notes = append(notes, "FFmpeg was not found: preview, photos and recording are unavailable until ffmpeg.exe is placed next to the app or set in Settings.")
		return camera.Capabilities{LiveView: camera.Unsupported, StillCapture: camera.Unsupported, OriginalTransfer: camera.Unsupported,
			MovieRecord: camera.Unsupported, MovieTransfer: camera.Unsupported, StreamedVideo: camera.Unsupported, Notes: notes}
	}
	return camera.Capabilities{
		LiveView: camera.Supported, StillCapture: camera.Supported, OriginalTransfer: camera.Unsupported,
		MovieRecord: camera.Unsupported, MovieTransfer: camera.Unsupported, StreamedVideo: camera.Supported, Notes: notes,
	}
}

// ActiveMode reports the mode the device is streaming (or will stream) in.
func (c *Camera) ActiveMode() (camera.Mode, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mode.Width > 0 {
		return c.mode, true
	}
	if c.prefs.Width > 0 {
		return camera.Mode{Width: c.prefs.Width, Height: c.prefs.Height, FPSMax: float64(c.prefs.FPS)}, true
	}
	return camera.Mode{}, false
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
		m.FPSMax = float64(src.FPS)
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
	if c.ffmpeg == "" && c.override == nil {
		c.mu.Unlock()
		return camera.Errf(camera.CodeSDKMissing, "Place ffmpeg.exe next to the application (or set its path in Settings), then connect again. See docs/CAMERAS.md.", nil, "FFmpeg is required for the camera preview")
	}
	c.sizeSeen = false
	src := c.source()
	c.mu.Unlock()

	var s *video.Stream
	s = video.NewStream(video.StreamOptions{
		FFmpeg: c.ffmpeg, Source: src,
		OnFrame: func(b []byte) {
			c.noteFrameSize(b)
			c.frames.Publish(b)
		},
		OnLost: func(err error, detail string) { c.lost(s, err, detail) },
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
		return camera.Errf(camera.CodeDeviceBusy, "Close other programs using the camera (Teams, Zoom, OBS, browsers) and retry. If the device lists several streams (e.g. RGB and infrared), try the other entry. For a Canon body, set USB streaming mode. Every attempt is written to the log (Settings → Media library → Open Library Folder → logs).", err, "could not start the camera preview")
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

// noteFrameSize records the real stream size from the first frame of a stream
// (the open may have fallen back to the device's default mode).
func (c *Camera) noteFrameSize(jpegData []byte) {
	c.mu.Lock()
	seen := c.sizeSeen
	c.mu.Unlock()
	if seen {
		return
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(jpegData))
	if err != nil {
		return
	}
	c.mu.Lock()
	c.sizeSeen = true
	if c.mode.Width != cfg.Width || c.mode.Height != cfg.Height {
		c.mode.Width, c.mode.Height = cfg.Width, cfg.Height
	}
	c.mu.Unlock()
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
	src := "UVC frame grab (not a camera original)"
	if m, ok := c.ActiveMode(); ok {
		src = fmt.Sprintf("UVC frame grab %d×%d (not a camera original)", m.Width, m.Height)
	}
	return camera.Photo{Data: f.JPEG, Source: src}, nil
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

// streamForTest exposes the running stream to tests (to simulate an unplug).
func (c *Camera) streamForTest() *video.Stream {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stream
}
