package edsdk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
)

// Camera is one Canon body. Capabilities start "unverified" and are promoted
// to "supported" (or demoted to "unsupported") only by what actually happened
// on the connected camera during this run.
type Camera struct {
	w    *worker
	a    api
	info camera.DeviceInfo
	find conflictFinder
	drv  *Driver

	frames *camera.FrameBuffer
	events chan camera.Event
	items  chan ref // DirItemRequestTransfer notifications

	mu       sync.Mutex
	cam      ref
	opened   bool
	closed   bool
	state    camera.State
	caps     camera.Capabilities
	liveStop context.CancelFunc
	liveDone chan struct{}
	lastEvf  uint32
	notified bool

	// FrameInterval paces live-view downloads (default ~30 fps).
	FrameInterval time.Duration
	// CaptureTimeout bounds the wait for the photo to arrive after the shutter.
	CaptureTimeout time.Duration
	// KeepAliveInterval is how often the camera's auto power-off timer is
	// extended while connected (default 30 s). The camera also announces
	// "will soon shut down", which triggers an immediate extension.
	KeepAliveInterval time.Duration

	kick     chan struct{}
	stopKeep context.CancelFunc
	keepDone chan struct{}
}

func newCamera(w *worker, a api, info camera.DeviceInfo, find conflictFinder, d *Driver) *Camera {
	return &Camera{
		w: w, a: a, info: info, find: find, drv: d,
		frames: camera.NewFrameBuffer(), events: make(chan camera.Event, 4), items: make(chan ref, 16),
		state: camera.StateDisconnected, FrameInterval: 33 * time.Millisecond, CaptureTimeout: 25 * time.Second, KeepAliveInterval: 30 * time.Second, kick: make(chan struct{}, 1),
		caps: camera.Capabilities{
			LiveView: camera.Unverified, StillCapture: camera.Unverified, OriginalTransfer: camera.Unverified,
			MovieRecord: camera.Unverified, MovieTransfer: camera.Unverified, StreamedVideo: camera.Unsupported,
			Notes: []string{
				"Capabilities marked unverified are promoted to supported only after they succeed on this camera.",
				"Camera-native movie recording and movie file transfer through EDSDK are NOT implemented or tested; use the camera in USB streaming (UVC) mode with the UVC adapter for video.",
			},
		},
	}
}

func (c *Camera) Info() camera.DeviceInfo { return c.info }

func (c *Camera) Capabilities() camera.Capabilities {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := c.caps
	cp.Notes = append([]string(nil), c.caps.Notes...)
	return cp
}

func (c *Camera) State() camera.State         { c.mu.Lock(); defer c.mu.Unlock(); return c.state }
func (c *Camera) Events() <-chan camera.Event { return c.events }

func (c *Camera) setCap(f func(*camera.Capabilities)) { c.mu.Lock(); f(&c.caps); c.mu.Unlock() }

// emitLost reports a disconnect once.
func (c *Camera) emitLost(cause error) {
	c.mu.Lock()
	if c.closed || c.notified {
		c.mu.Unlock()
		return
	}
	c.notified = true
	c.state = camera.StateError
	c.mu.Unlock()
	select {
	case c.events <- camera.Event{Kind: camera.EventDisconnected, Err: cause}:
	default:
	}
}

// fail translates an SDK error and reports a disconnect if that is what it was.
func (c *Camera) fail(op string, err error) error {
	if err == nil {
		return nil
	}
	if code, ok := sdkCode(err); ok && isDisconnect(code) {
		c.emitLost(err)
	}
	return translate(op, err, c.find)
}

func (c *Camera) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.closed || c.opened {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	var found rawCamera
	err := c.w.do(ctx, func() error {
		cams, err := c.a.CameraList()
		if err != nil {
			return err
		}
		var match *rawCamera
		for i := range cams {
			if deviceID(cams[i], i) == c.info.ID {
				match = &cams[i]
			}
		}
		for i := range cams {
			if match == nil || cams[i].Ref != match.Ref {
				c.a.Release(cams[i].Ref)
			}
		}
		if match == nil {
			return &sdkError{Op: "find camera", Code: errDeviceNotFound}
		}
		found = *match
		if err := c.a.OpenSession(found.Ref); err != nil {
			c.a.Release(found.Ref)
			return err
		}
		// Send photos to the PC, not only to the card, and announce free space.
		if err := c.a.SetU32(found.Ref, propSaveTo, saveToHost); err != nil {
			_ = c.a.CloseSession(found.Ref)
			c.a.Release(found.Ref)
			return err
		}
		_ = c.a.AnnounceHostSpace(found.Ref) // required on some bodies, rejected on others: best effort
		err = c.a.SetHandlers(found.Ref, c.onObject, c.onState)
		if err != nil {
			_ = c.a.CloseSession(found.Ref)
			c.a.Release(found.Ref)
		}
		return err
	})
	if err != nil {
		return c.fail("open camera session", err)
	}
	c.mu.Lock()
	c.cam, c.opened, c.state, c.notified = found.Ref, true, camera.StateConnected, false
	kctx, cancel := context.WithCancel(context.Background())
	c.stopKeep, c.keepDone = cancel, make(chan struct{})
	done := c.keepDone
	c.mu.Unlock()
	go c.keepAlive(kctx, found.Ref, done)
	return nil
}

// keepAlive stops the body from powering itself off during an event: the
// auto power-off timer is extended periodically and whenever the camera warns
// that it will shut down soon. Failures are ignored here; a vanished camera is
// detected by the state event and by the live-view/capture calls.
func (c *Camera) keepAlive(ctx context.Context, r ref, done chan struct{}) {
	defer close(done)
	iv := c.KeepAliveInterval
	if iv <= 0 {
		iv = 30 * time.Second
	}
	t := time.NewTicker(iv)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-c.kick:
		}
		cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		_ = c.w.do(cctx, func() error { return c.a.SendCommand(r, cmdExtendShutdownTimer, 0) })
		cancel()
	}
}

// onObject runs on the SDK thread during PumpEvents.
func (c *Camera) onObject(event uint32, item ref) {
	if event == evObjectDirItemRequestTransfer {
		select {
		case c.items <- item:
		default: // queue full: drop (and let the camera time out) rather than block the SDK thread
			c.a.SkipItem(item)
		}
	}
}

func (c *Camera) onState(event, data uint32) {
	switch event {
	case evStateShutdown:
		c.emitLost(errors.New("camera reported shutdown / USB disconnect"))
	case evStateWillSoonShutDown:
		select { // never block the SDK thread
		case c.kick <- struct{}{}:
		default:
		}
	}
}

func (c *Camera) ref() (ref, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || !c.opened {
		return 0, camera.Errf(camera.CodeNotConnected, "Connect the camera first.", nil, "camera is not connected")
	}
	if c.state == camera.StateError {
		return 0, camera.Errf(camera.CodeDisconnected, "Check the USB cable. The app reconnects automatically.", nil, "camera is disconnected")
	}
	return c.cam, nil
}

func (c *Camera) StartLiveView(ctx context.Context) error {
	r, err := c.ref()
	if err != nil {
		return err
	}
	c.mu.Lock()
	if c.liveStop != nil {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()
	err = c.w.do(ctx, func() error {
		cur, err := c.a.GetU32(r, propEvfOutputDevice)
		if err != nil {
			cur = 0 // some bodies refuse the read until live view is on
		}
		c.lastEvf = cur
		return c.a.SetU32(r, propEvfOutputDevice, cur|evfOutputPC)
	})
	if err != nil {
		if code, ok := sdkCode(err); ok && (code == errNotSupported || code == errUnimplemented) {
			c.setCap(func(k *camera.Capabilities) { k.LiveView = camera.Unsupported })
		}
		return c.fail("start live view", err)
	}
	lctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	c.mu.Lock()
	c.liveStop, c.liveDone, c.state = cancel, done, camera.StateLiveView
	c.mu.Unlock()
	go c.liveLoop(lctx, r, done)
	return nil
}

func (c *Camera) liveLoop(ctx context.Context, r ref, done chan struct{}) {
	defer close(done)
	failures := 0
	for ctx.Err() == nil {
		var jpg []byte
		err := c.w.do(ctx, func() (e error) { jpg, e = c.a.DownloadEvf(r); return })
		switch {
		case err == nil && len(jpg) > 0:
			failures = 0
			c.frames.Publish(jpg)
			c.setCap(func(k *camera.Capabilities) { k.LiveView = camera.Supported })
		case ctx.Err() != nil:
			return
		default:
			code, _ := sdkCode(err)
			if code == errObjectNotReady || err == nil { // live view still warming up
				failures++
			} else if isDisconnect(code) {
				c.emitLost(err)
				return
			} else {
				failures += 5
			}
			if failures > 150 { // ~5 s without a frame
				c.mu.Lock()
				c.state = camera.StateConnected
				c.liveStop = nil
				c.mu.Unlock()
				select {
				case c.events <- camera.Event{Kind: camera.EventError, Err: translate("live view", err, c.find)}:
				default:
				}
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(c.FrameInterval):
		}
	}
}

func (c *Camera) StopLiveView() error {
	c.mu.Lock()
	stop, done := c.liveStop, c.liveDone
	c.liveStop, c.liveDone = nil, nil
	r, opened, closed := c.cam, c.opened, c.closed
	if c.state == camera.StateLiveView {
		c.state = camera.StateConnected
	}
	c.mu.Unlock()
	if stop == nil {
		return nil
	}
	stop()
	<-done
	c.frames.Reset()
	if opened && !closed {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = c.w.do(ctx, func() error { return c.a.SetU32(r, propEvfOutputDevice, c.lastEvf&^evfOutputPC) })
	}
	return nil
}

func (c *Camera) WaitFrame(ctx context.Context, after uint64) (camera.Frame, error) {
	return c.frames.Wait(ctx, after)
}

func (c *Camera) drainItems() {
	for {
		select {
		case it := <-c.items:
			_ = c.w.do(context.Background(), func() error { c.a.SkipItem(it); return nil })
		default:
			return
		}
	}
}

// Capture triggers the shutter and returns the JPEG original transferred to
// the PC. RAW companions of a RAW+JPEG shot are discarded; a RAW-only setting
// is reported as an error because nothing viewable would arrive.
func (c *Camera) Capture(ctx context.Context) (camera.Photo, error) {
	r, err := c.ref()
	if err != nil {
		return camera.Photo{}, err
	}
	if err := ctx.Err(); err != nil {
		return camera.Photo{}, err
	}
	c.drainItems()

	// 1. release the shutter (retry while the camera is still focusing/busy)
	var shotErr error
	for attempt := 0; attempt < 6; attempt++ {
		shotErr = c.w.do(ctx, func() error {
			err := c.a.SendCommand(r, cmdTakePicture, 0)
			if code, ok := sdkCode(err); ok && (code == errNotSupported || code == errUnimplemented) {
				// some bodies only accept the shutter-button command
				if err2 := c.a.SendCommand(r, cmdPressShutter, shutterCompletelyNonAF); err2 != nil {
					return err2
				}
				return c.a.SendCommand(r, cmdPressShutter, shutterOff)
			}
			return err
		})
		if code, ok := sdkCode(shotErr); ok && code == errObjectNotReady {
			select {
			case <-ctx.Done():
				return camera.Photo{}, ctx.Err()
			case <-time.After(300 * time.Millisecond):
			}
			continue
		}
		break
	}
	if shotErr != nil {
		if ctx.Err() != nil {
			return camera.Photo{}, ctx.Err()
		}
		return camera.Photo{}, c.fail("take picture", shotErr)
	}

	// 2. wait for the camera to offer the file, download the JPEG
	deadline := time.NewTimer(c.CaptureTimeout)
	defer deadline.Stop()
	sawRAW := false
	for {
		select {
		case <-ctx.Done():
			return camera.Photo{}, ctx.Err()
		case <-deadline.C:
			msg := "the camera did not deliver the photo in time"
			act := "Check that the camera is in Photo mode with a card or SaveTo-PC enabled, then retry."
			if sawRAW {
				msg, act = "the camera only delivered a RAW file", "Set the camera's image quality to JPEG or RAW+JPEG."
			}
			return camera.Photo{}, camera.Errf(camera.CodeTimeout, act, nil, "%s", msg)
		case it := <-c.items:
			var name string
			var data []byte
			err := c.w.do(ctx, func() error {
				n, size, err := c.a.DirItem(it)
				if err != nil {
					c.a.SkipItem(it)
					return err
				}
				name = n
				if !isJPEG(n) {
					c.a.SkipItem(it) // RAW companion: acknowledge and ignore
					return nil
				}
				data, err = c.a.Download(it, size)
				return err
			})
			if err != nil {
				return camera.Photo{}, c.fail("download photo", err)
			}
			if data == nil {
				sawRAW = true
				continue
			}
			c.setCap(func(k *camera.Capabilities) {
				k.StillCapture, k.OriginalTransfer = camera.Supported, camera.Supported
			})
			return camera.Photo{Data: data, Filename: name, Source: "Canon EDSDK original (" + c.info.Name + ")"}, nil
		}
	}
}

func isJPEG(name string) bool {
	n := strings.ToLower(name)
	return strings.HasSuffix(n, ".jpg") || strings.HasSuffix(n, ".jpeg")
}

// Disconnect stops live view, closes the SDK session and releases the handle.
// Safe to call repeatedly and when the camera is already gone.
func (c *Camera) Disconnect() error {
	c.mu.Lock()
	stopKeep, keepDone := c.stopKeep, c.keepDone
	c.stopKeep, c.keepDone = nil, nil
	c.mu.Unlock()
	if stopKeep != nil {
		stopKeep()
		<-keepDone
	}
	_ = c.StopLiveView()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	r, opened := c.cam, c.opened
	c.opened = false
	c.state = camera.StateDisconnected
	c.mu.Unlock()
	var err error
	if opened {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		err = c.w.do(ctx, func() error {
			// a vanished camera makes CloseSession fail; the handle is still released
			cerr := c.a.CloseSession(r)
			c.a.Release(r)
			return cerr
		})
		if errors.Is(err, errWorkerClosed) {
			err = nil
		}
		if code, ok := sdkCode(err); ok && isDisconnect(code) {
			err = nil
		}
	}
	c.drainItemsQuiet()
	close(c.events)
	if err != nil {
		return fmt.Errorf("closing camera session: %w", err)
	}
	return nil
}

func (c *Camera) drainItemsQuiet() {
	for {
		select {
		case <-c.items:
		default:
			return
		}
	}
}
