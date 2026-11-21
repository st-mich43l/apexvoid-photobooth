// Package mock is a development/test camera. Everything it produces is
// marked Simulated and must never be presented as a real photograph.
package mock

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"sync"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
)

const DriverName = "mock"

// Driver exposes one simulated camera and lets tests inject faults.
type Driver struct {
	mu      sync.Mutex
	present bool
	last    *Camera
	// CaptureDelay simulates shutter + transfer time.
	CaptureDelay time.Duration
	// Width/Height of the simulated "original".
	Width, Height int
}

// NewDriver returns a driver with one plugged-in simulated camera.
func NewDriver() *Driver {
	return &Driver{present: true, CaptureDelay: 150 * time.Millisecond, Width: 1920, Height: 1280}
}

func (d *Driver) Name() string     { return DriverName }
func (d *Driver) Available() error { return nil }

// SetPresent simulates plugging/unplugging the USB cable. Unplugging notifies
// the open camera.
func (d *Driver) SetPresent(p bool) {
	d.mu.Lock()
	d.present = p
	c := d.last
	d.mu.Unlock()
	if !p && c != nil {
		c.Unplug()
	}
}

// Last returns the most recently opened camera (nil if none).
func (d *Driver) Last() *Camera {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.last
}

func info() camera.DeviceInfo {
	return camera.DeviceInfo{ID: "mock-0", Name: "SIMULATED camera (mock)", Driver: DriverName, Simulated: true, Kind: camera.KindSimulated}
}

func (d *Driver) Discover(context.Context) ([]camera.DeviceInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.present {
		return nil, nil
	}
	return []camera.DeviceInfo{info()}, nil
}

func (d *Driver) Open(i camera.DeviceInfo) (camera.Camera, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.present {
		return nil, camera.Errf(camera.CodeNoDevice, "Plug the camera in.", nil, "simulated camera is unplugged")
	}
	c := &Camera{drv: d, frames: camera.NewFrameBuffer(), events: make(chan camera.Event, 4), state: camera.StateDisconnected}
	d.last = c
	return c, nil
}

// Camera is the simulated device.
type Camera struct {
	drv    *Driver
	frames *camera.FrameBuffer
	events chan camera.Event

	mu       sync.Mutex
	state    camera.State
	closed   bool
	stopLive context.CancelFunc
	shots    int
	failNext error
}

func (c *Camera) Info() camera.DeviceInfo { return info() }
func (c *Camera) Capabilities() camera.Capabilities {
	return camera.Capabilities{
		LiveView: camera.Supported, StillCapture: camera.Supported, OriginalTransfer: camera.Supported,
		MovieRecord: camera.Unsupported, MovieTransfer: camera.Unsupported, StreamedVideo: camera.Unsupported,
		Notes: []string{"SIMULATED device: images are generated test patterns, not photographs."},
	}
}
func (c *Camera) State() camera.State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}
func (c *Camera) Events() <-chan camera.Event { return c.events }

func (c *Camera) Connect(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return camera.Errf(camera.CodeNotConnected, "", nil, "camera was released")
	}
	c.state = camera.StateConnected
	return nil
}

// FailNextCapture makes the next Capture return err.
func (c *Camera) FailNextCapture(err error) { c.mu.Lock(); c.failNext = err; c.mu.Unlock() }

// Shots returns how many captures succeeded.
func (c *Camera) Shots() int { c.mu.Lock(); defer c.mu.Unlock(); return c.shots }

// Released reports whether Disconnect ran.
func (c *Camera) Released() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.closed }

// Unplug simulates a USB disconnect.
func (c *Camera) Unplug() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.state = camera.StateError
	c.mu.Unlock()
	select {
	case c.events <- camera.Event{Kind: camera.EventDisconnected, Err: fmt.Errorf("simulated USB unplug")}:
	default:
	}
}

func (c *Camera) Disconnect() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.state = camera.StateDisconnected
	stop := c.stopLive
	c.stopLive = nil
	c.mu.Unlock()
	if stop != nil {
		stop()
	}
	close(c.events)
	return nil
}

func (c *Camera) StartLiveView(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.state == camera.StateError || c.state == camera.StateDisconnected {
		return camera.Errf(camera.CodeNotConnected, "Reconnect the camera.", nil, "camera is not connected")
	}
	if c.stopLive != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.stopLive = cancel
	c.state = camera.StateLiveView
	go func() {
		t := time.NewTicker(66 * time.Millisecond)
		defer t.Stop()
		n := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				n++
				c.frames.Publish(render(640, 427, fmt.Sprintf("LIVE VIEW %d", n), n))
			}
		}
	}()
	return nil
}

func (c *Camera) StopLiveView() error {
	c.mu.Lock()
	stop := c.stopLive
	c.stopLive = nil
	if c.state == camera.StateLiveView {
		c.state = camera.StateConnected
	}
	c.mu.Unlock()
	if stop != nil {
		stop()
	}
	return nil
}

func (c *Camera) WaitFrame(ctx context.Context, after uint64) (camera.Frame, error) {
	return c.frames.Wait(ctx, after)
}

func (c *Camera) Capture(ctx context.Context) (camera.Photo, error) {
	c.mu.Lock()
	if c.closed || c.state == camera.StateError || c.state == camera.StateDisconnected {
		c.mu.Unlock()
		return camera.Photo{}, camera.Errf(camera.CodeDisconnected, "Reconnect the camera.", nil, "camera is not connected")
	}
	if err := c.failNext; err != nil {
		c.failNext = nil
		c.mu.Unlock()
		return camera.Photo{}, err
	}
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		return camera.Photo{}, ctx.Err()
	case <-time.After(c.drv.CaptureDelay):
	}
	c.mu.Lock()
	c.shots++
	n := c.shots
	c.mu.Unlock()
	w, h := c.drv.Width, c.drv.Height
	return camera.Photo{
		Data: render(w, h, fmt.Sprintf("SIMULATED CAPTURE %d", n), n*7), Filename: fmt.Sprintf("SIM_%04d.JPG", n),
		Simulated: true, Source: "mock (simulated)",
	}, nil
}

// render draws a gradient test pattern with a SIMULATED banner.
func render(w, h int, label string, seed int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Pix[(y*w+x)*4+0] = uint8((x*255/w + seed*3) % 256)
			img.Pix[(y*w+x)*4+1] = uint8(y * 160 / h)
			img.Pix[(y*w+x)*4+2] = uint8(200 - x*120/w)
			img.Pix[(y*w+x)*4+3] = 255
		}
	}
	scale := w / 320
	if scale < 1 {
		scale = 1
	}
	bar := 14 * scale
	draw.Draw(img, image.Rect(0, 0, w, bar), image.NewUniform(color.RGBA{20, 10, 40, 255}), image.Point{}, draw.Src)
	// basicfont has one fixed size: draw into a small strip, then enlarge by pixel replication.
	small := image.NewRGBA(image.Rect(0, 0, w/scale, 14))
	draw.Draw(small, small.Bounds(), image.NewUniform(color.RGBA{20, 10, 40, 255}), image.Point{}, draw.Src)
	d := &font.Drawer{Dst: small, Src: image.NewUniform(color.RGBA{255, 160, 200, 255}), Face: basicfont.Face7x13, Dot: fixed.P(6, 11)}
	d.DrawString("SIMULATED - NOT A REAL PHOTO  " + label)
	for y := 0; y < bar; y++ {
		for x := 0; x < w; x++ {
			sp := small.PixOffset(x/scale, y/scale)
			dp := img.PixOffset(x, y)
			copy(img.Pix[dp:dp+4], small.Pix[sp:sp+4])
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
	return buf.Bytes()
}
