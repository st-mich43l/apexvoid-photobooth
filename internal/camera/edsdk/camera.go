package edsdk

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
)

// capItem is a transfer request the camera raised inside a capture window.
type capItem struct {
	item ref
	seq  uint64
}

// Diag is a snapshot of driver-level counters for diagnostics.
type Diag struct {
	Health        Health `json:"sdk"`
	Captures      uint64 `json:"captures"`         // photos delivered
	LateTransfers uint64 `json:"lateTransfers"`    // transfer requests dropped because no capture was waiting for them
	Invalid       uint64 `json:"invalidTransfers"` // downloads rejected as incomplete / not a JPEG
	LastFile      string `json:"lastFile,omitempty"`
}

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

	// events: every send and the final close happen under evMu so a state
	// callback racing Disconnect can never send on a closed channel.
	evMu     sync.Mutex
	events   chan camera.Event
	evClosed bool

	items    chan capItem  // DirItemRequestTransfer notifications inside a capture window
	gone     chan struct{} // closed once when the camera is lost or disconnected
	goneOnce sync.Once

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

	// capture association: only transfer requests that arrive while a capture
	// window is open are candidates; SDK events have no proven shot identifier.
	// An uncertain capture blocks later captures on this camera object.
	capBusy           bool
	winOpen           bool
	winSeq            uint64
	transferUncertain bool // failed/cancelled shot may still deliver a JPEG
	lastFile          string
	nCaptures         uint64
	nLate             uint64
	nInvalid          uint64

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
		frames: camera.NewFrameBuffer(), events: make(chan camera.Event, 4), items: make(chan capItem, 16), gone: make(chan struct{}),
		state: camera.StateDisconnected, FrameInterval: 33 * time.Millisecond, CaptureTimeout: 25 * time.Second,
		KeepAliveInterval: 30 * time.Second, kick: make(chan struct{}, 1),
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

// Diagnostics returns driver counters and the SDK thread health.
func (c *Camera) Diagnostics() Diag {
	c.mu.Lock()
	d := Diag{Captures: c.nCaptures, LateTransfers: c.nLate, Invalid: c.nInvalid, LastFile: c.lastFile}
	c.mu.Unlock()
	d.Health = c.w.Health()
	return d
}

func (c *Camera) setCap(f func(*camera.Capabilities)) { c.mu.Lock(); f(&c.caps); c.mu.Unlock() }

func (c *Camera) signalGone() { c.goneOnce.Do(func() { close(c.gone) }) }

// sendEvent delivers ev unless the channel is already closed. It never blocks.
func (c *Camera) sendEvent(ev camera.Event) {
	c.evMu.Lock()
	defer c.evMu.Unlock()
	if c.evClosed {
		return
	}
	select {
	case c.events <- ev:
	default:
	}
}

func (c *Camera) closeEvents() {
	c.evMu.Lock()
	if !c.evClosed {
		c.evClosed = true
		close(c.events)
	}
	c.evMu.Unlock()
}

// emitLost reports a disconnect once and wakes anything waiting on the camera.
func (c *Camera) emitLost(cause error) {
	c.mu.Lock()
	if c.closed || c.notified {
		c.mu.Unlock()
		return
	}
	c.notified = true
	c.state = camera.StateError
	c.mu.Unlock()
	c.signalGone()
	c.sendEvent(camera.Event{Kind: camera.EventDisconnected, Err: cause})
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

// closeNative closes a session and releases its handle. It runs to completion
// (bounded) even if the caller was cancelled, and never leaves handlers behind.
func (c *Camera) closeNative(ctx context.Context, r ref) error {
	dctx, cancel := detached(ctx, 8*time.Second)
	defer cancel()
	_, err := call(dctx, c.w, "close session", func() (struct{}, error) {
		c.a.ClearHandlers(r)
		cerr := c.a.CloseSession(r) // a vanished camera makes this fail; the handle is still released
		c.a.Release(r)
		return struct{}{}, cerr
	})
	return err
}

func (c *Camera) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.closed || c.opened {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	// The native open runs to completion even if the caller cancels, so a
	// half-opened session can never be abandoned; cancellation is honoured
	// afterwards by closing what was opened.
	dctx, cancel := detached(ctx, 20*time.Second)
	defer cancel()
	found, err := call(dctx, c.w, "open session", func() (ref, error) {
		cams, err := c.a.CameraList()
		if err != nil {
			return 0, err
		}
		match := matchCamera(cams, c.info.ID)
		for i := range cams {
			if match < 0 || i != match {
				c.a.Release(cams[i].Ref)
			}
		}
		if match < 0 {
			return 0, &sdkError{Op: "find camera", Code: errDeviceNotFound}
		}
		r := cams[match].Ref
		fail := func(err error) (ref, error) {
			c.a.ClearHandlers(r)
			_ = c.a.CloseSession(r)
			c.a.Release(r)
			return 0, err
		}
		if err := c.a.OpenSession(r); err != nil {
			c.a.Release(r)
			return 0, err
		}
		// Send photos to the PC, not only to the card, and announce free space.
		if err := c.a.SetU32(r, propSaveTo, saveToHost); err != nil {
			return fail(err)
		}
		_ = c.a.AnnounceHostSpace(r) // required on some bodies, rejected on others: best effort
		if err := c.a.SetHandlers(r, c.onObject, c.onState); err != nil {
			return fail(err)
		}
		return r, nil
	})
	if err != nil {
		return c.fail("open camera session", err)
	}
	if ctx.Err() != nil {
		_ = c.closeNative(ctx, found)
		return ctx.Err()
	}
	c.mu.Lock()
	c.cam, c.opened, c.state, c.notified = found, true, camera.StateConnected, false
	kctx, kcancel := context.WithCancel(context.Background())
	c.stopKeep, c.keepDone = kcancel, make(chan struct{})
	done := c.keepDone
	c.mu.Unlock()
	go c.keepAlive(kctx, found, done)
	return nil
}

// matchCamera finds info.ID in the SDK list using the same order-independent
// identity as discovery.
func matchCamera(cams []rawCamera, id string) int {
	ids := stableIDs(cams)
	for i := range cams {
		if ids[i] == id {
			return i
		}
	}
	return -1
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
		_ = c.w.doOp(cctx, "extend shutdown timer", func() error { return c.a.SendCommand(r, cmdExtendShutdownTimer, 0) })
		cancel()
	}
}

// onObject runs on the SDK thread during PumpEvents. It must never block and
// it owns every object reference it is handed: each one is either queued for
// the capture that is waiting for it or released here.
func (c *Camera) onObject(event uint32, item ref) {
	if item == 0 {
		return
	}
	if event != evObjectDirItemRequestTransfer {
		// "Object without matching event fire must be released" (Canon samples).
		c.a.Release(item)
		return
	}
	c.mu.Lock()
	open, seq := c.winOpen, c.winSeq
	if !open {
		c.nLate++
	}
	c.mu.Unlock()
	if !open {
		c.a.SkipItem(item) // late arrival or RAW companion: acknowledge and drop
		return
	}
	select {
	case c.items <- capItem{item: item, seq: seq}:
	default: // queue full: drop rather than block the SDK thread
		c.a.SkipItem(item)
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

// armLiveView switches the live-view output to the PC. Mirrorless bodies such
// as the R50 may drop it after a capture, so it is also used to re-arm.
func (c *Camera) armLiveView(ctx context.Context, r ref) error {
	cur, err := call(ctx, c.w, "arm live view", func() (uint32, error) {
		if mode, merr := c.a.GetU32(r, propEvfMode); merr == nil && mode == 0 {
			_ = c.a.SetU32(r, propEvfMode, 1) // DSLRs need it; bodies that always run live view reject it
		}
		cur, gerr := c.a.GetU32(r, propEvfOutputDevice)
		if gerr != nil {
			cur = 0 // some bodies refuse the read until live view is on
		}
		return cur, c.a.SetU32(r, propEvfOutputDevice, cur|evfOutputPC)
	})
	if err == nil {
		c.mu.Lock()
		if cur&evfOutputPC == 0 { // remember what the camera had so Stop restores it
			c.lastEvf = cur
		}
		c.mu.Unlock()
	}
	return err
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
	if err := c.armLiveView(ctx, r); err != nil {
		if code, ok := sdkCode(err); ok && (code == errNotSupported || code == errUnimplemented) {
			c.setCap(func(k *camera.Capabilities) { k.LiveView = camera.Unsupported })
		}
		return c.fail("start live view", err)
	}
	lctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	c.mu.Lock()
	if c.liveStop != nil || c.closed { // raced with another start or Disconnect
		c.mu.Unlock()
		cancel()
		return nil
	}
	c.liveStop, c.liveDone, c.state = cancel, done, camera.StateLiveView
	c.mu.Unlock()
	go c.liveLoop(lctx, r, done)
	return nil
}

func isJPEGStream(b []byte) bool { return len(b) > 4 && b[0] == 0xFF && b[1] == 0xD8 }

func (c *Camera) liveLoop(ctx context.Context, r ref, done chan struct{}) {
	defer close(done)
	failures := 0
	for ctx.Err() == nil {
		jpg, err := call(ctx, c.w, "download live-view frame", func() ([]byte, error) { return c.a.DownloadEvf(r) })
		switch {
		case err == nil && isJPEGStream(jpg):
			failures = 0
			c.frames.Publish(jpg)
			c.setCap(func(k *camera.Capabilities) { k.LiveView = camera.Supported })
		case ctx.Err() != nil:
			return
		default:
			code, _ := sdkCode(err)
			switch {
			case err == nil || code == errObjectNotReady: // live view warming up, or dropped after a shot
				failures++
			case isDisconnect(code):
				c.emitLost(err)
				return
			default:
				failures += 5
			}
			// A body can silently turn the PC output off (after a shot, on a mode change):
			// ask for it again now and then instead of giving up.
			if failures > 0 && failures%20 == 0 {
				rctx, rc := context.WithTimeout(ctx, 3*time.Second)
				_ = c.armLiveView(rctx, r)
				rc()
			}
			if failures > 150 { // ~5 s without a frame
				c.mu.Lock()
				c.state = camera.StateConnected
				c.liveStop = nil
				c.mu.Unlock()
				c.sendEvent(camera.Event{Kind: camera.EventError, Err: translate("live view", err, c.find)})
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
	r, opened, closed, last := c.cam, c.opened, c.closed, c.lastEvf
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
		_ = c.w.doOp(ctx, "stop live view", func() error { return c.a.SetU32(r, propEvfOutputDevice, last&^evfOutputPC) })
	}
	return nil
}

func (c *Camera) WaitFrame(ctx context.Context, after uint64) (camera.Frame, error) {
	return c.frames.Wait(ctx, after)
}

// openWindow starts a capture window: from now on transfer requests belong to
// this capture. Anything queued earlier is stale and is acknowledged away.
func (c *Camera) openWindow() uint64 {
	c.drainItems()
	c.mu.Lock()
	c.winSeq++
	c.winOpen = true
	seq := c.winSeq
	c.mu.Unlock()
	return seq
}

// closeWindow ends the capture window and releases every item still queued.
func (c *Camera) closeWindow() {
	c.mu.Lock()
	c.winOpen = false
	c.mu.Unlock()
	c.drainItems()
}

// drainItems acknowledges and releases every queued transfer request. Items
// already pulled off the queue are owned by the caller, so this only ever
// touches references nobody else holds.
func (c *Camera) drainItems() {
	for {
		select {
		case it := <-c.items:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = c.w.doOp(ctx, "skip transfer", func() error { c.a.SkipItem(it.item); return nil })
			cancel()
		default:
			return
		}
	}
}

func lostErr() error {
	return camera.Errf(camera.CodeDisconnected, "Check the USB cable and that the camera is on. The app reconnects automatically.", nil, "the camera was disconnected during the capture")
}

// Capture triggers the shutter and returns the JPEG original transferred to
// the PC. RAW companions of a RAW+JPEG shot are discarded; a RAW-only setting
// is reported as an error because nothing viewable would arrive. A capture
// succeeds only once a complete JPEG has been received.
func (c *Camera) Capture(ctx context.Context) (camera.Photo, error) {
	r, err := c.ref()
	if err != nil {
		return camera.Photo{}, err
	}
	if err := ctx.Err(); err != nil {
		return camera.Photo{}, err
	}
	c.mu.Lock()
	if c.transferUncertain {
		c.mu.Unlock()
		return camera.Photo{}, camera.Errf(camera.CodeCaptureFailed, "Stop the session, turn the camera off and on, then reconnect it before trying again.", nil, "the previous transfer was not confirmed; a late photo cannot safely be assigned")
	}
	if c.capBusy {
		c.mu.Unlock()
		return camera.Photo{}, camera.Errf(camera.CodeBusyOperation, "Wait for the current capture to finish.", nil, "a capture is already in progress on this camera")
	}
	c.capBusy = true
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.capBusy = false; c.mu.Unlock() }()

	c.openWindow()
	defer c.closeWindow()

	// 1. release the shutter (retry while the camera is still focusing/busy)
	var shotErr error
	for attempt := 0; attempt < 6; attempt++ {
		_, shotErr = call(ctx, c.w, "release shutter", func() (struct{}, error) {
			err := c.a.SendCommand(r, cmdTakePicture, 0)
			if code, ok := sdkCode(err); ok && (code == errNotSupported || code == errUnimplemented) {
				// some bodies only accept the shutter-button command
				if err2 := c.a.SendCommand(r, cmdPressShutter, shutterCompletelyNonAF); err2 != nil {
					return struct{}{}, err2
				}
				return struct{}{}, c.a.SendCommand(r, cmdPressShutter, shutterOff)
			}
			return struct{}{}, err
		})
		if code, ok := sdkCode(shotErr); ok && code == errObjectNotReady {
			select {
			case <-ctx.Done():
				return camera.Photo{}, ctx.Err()
			case <-c.gone:
				return camera.Photo{}, lostErr()
			case <-time.After(300 * time.Millisecond):
			}
			continue
		}
		break
	}
	if shotErr != nil {
		// Even an SDK error may follow a shutter action; do not assume that
		// the camera cannot still deliver a transfer for this attempt.
		c.mu.Lock()
		c.transferUncertain = true
		c.mu.Unlock()
		if ctx.Err() != nil {
			// The native shutter call may finish after its caller gives up.
			return camera.Photo{}, ctx.Err()
		}
		return camera.Photo{}, uncertainTransferError(c.fail("take picture", shotErr))
	}

	// An unsuccessful wait after the shutter leaves a transfer uncorrelated.
	// Do not open another capture window on this camera object until an operator
	// has power-cycled and reconnected the camera.
	confirmed := false
	defer func() {
		if !confirmed {
			c.mu.Lock()
			c.transferUncertain = true
			c.mu.Unlock()
		}
	}()
	// 2. wait for the camera to offer the file, download and check the JPEG
	deadline := time.NewTimer(c.CaptureTimeout)
	defer deadline.Stop()
	sawRAW := false
	for {
		select {
		case <-ctx.Done():
			return camera.Photo{}, ctx.Err()
		case <-c.gone:
			return camera.Photo{}, lostErr()
		case <-deadline.C:
			msg := "the camera did not deliver the photo in time"
			act := "Stop the session, power-cycle the camera, check Photo mode and SaveTo-PC, then reconnect before retrying."
			if sawRAW {
				msg, act = "the camera only delivered a RAW file", "Set image quality to JPEG or RAW+JPEG, power-cycle and reconnect the camera before retrying."
			}
			return camera.Photo{}, camera.Errf(camera.CodeTimeout, act, nil, "%s", msg)
		case it := <-c.items:
			photo, done, err := c.receive(ctx, it)
			if err != nil {
				return camera.Photo{}, uncertainTransferError(err)
			}
			if !done { // RAW companion or a duplicate: keep waiting for the JPEG
				sawRAW = sawRAW || photo.Filename != ""
				continue
			}
			c.setCap(func(k *camera.Capabilities) { k.StillCapture, k.OriginalTransfer = camera.Supported, camera.Supported })
			c.afterCapture(ctx, r)
			confirmed = true
			return photo, nil
		}
	}
}

// Preserve the original error code while making the required reset explicit.
func uncertainTransferError(err error) error {
	var ce *camera.Error
	if errors.As(err, &ce) {
		copy := *ce
		copy.Action = strings.TrimSpace(ce.Action + " Stop the session, power-cycle and reconnect the camera before retrying.")
		return &copy
	}
	return camera.Errf(camera.CodeCaptureFailed, "Stop the session, power-cycle and reconnect the camera before retrying.", err, "camera transfer was not confirmed")
}

// receive handles one queued transfer request. The item is always completed
// (downloaded or skipped) even if ctx has been cancelled, so no native
// reference is left behind. done=false means "not the photo": the returned
// Photo.Filename then names a skipped RAW companion.
func (c *Camera) receive(ctx context.Context, it capItem) (camera.Photo, bool, error) {
	dctx, cancel := detached(ctx, 30*time.Second)
	defer cancel()
	type got struct {
		name string
		data []byte
		raw  bool
		dup  bool
		size uint64
	}
	g, err := call(dctx, c.w, "download photo", func() (got, error) {
		n, size, err := c.a.DirItem(it.item)
		if err != nil {
			c.a.SkipItem(it.item)
			return got{}, err
		}
		c.mu.Lock()
		dup := n == c.lastFile
		c.mu.Unlock()
		if !isJPEG(n) || dup {
			c.a.SkipItem(it.item) // RAW companion / repeated delivery: acknowledge and ignore
			return got{name: n, raw: !isJPEG(n), dup: dup}, nil
		}
		data, err := c.a.Download(it.item, size)
		return got{name: n, data: data, size: size}, err
	})
	if err != nil {
		return camera.Photo{}, false, c.fail("download photo", err)
	}
	if g.raw {
		return camera.Photo{Filename: g.name}, false, nil
	}
	if g.dup {
		return camera.Photo{}, false, nil
	}
	if verr := validateJPEG(g.data, g.size); verr != nil {
		c.mu.Lock()
		c.nInvalid++
		c.mu.Unlock()
		return camera.Photo{}, false, camera.Errf(camera.CodeCaptureFailed, "The transfer was incomplete. Stop the session, power-cycle and reconnect the camera before retrying.", verr, "the photo %s was not received completely", g.name)
	}
	c.mu.Lock()
	c.lastFile = g.name
	c.nCaptures++
	c.mu.Unlock()
	return camera.Photo{Data: g.data, Filename: g.name, Source: "Canon EDSDK original (" + c.info.Name + ")"}, true, nil
}

// afterCapture puts live view back after a shot if it was running: some bodies
// drop the PC output while exposing.
func (c *Camera) afterCapture(ctx context.Context, r ref) {
	c.mu.Lock()
	live := c.liveStop != nil
	c.mu.Unlock()
	if !live {
		return
	}
	dctx, cancel := detached(ctx, 3*time.Second)
	defer cancel()
	_ = c.armLiveView(dctx, r)
}

// validateJPEG checks that the transferred bytes are a complete JPEG: the size
// the camera announced, the start-of-image marker and the end-of-image marker.
// The original bytes are never modified.
func validateJPEG(data []byte, size uint64) error {
	if len(data) == 0 {
		return errors.New("no data")
	}
	if size > 0 && uint64(len(data)) != size {
		return fmt.Errorf("received %d of %d bytes", len(data), size)
	}
	if !isJPEGStream(data) {
		return errors.New("not a JPEG (no start-of-image marker)")
	}
	tail := data
	if len(tail) > 64 {
		tail = tail[len(tail)-64:]
	}
	if !bytes.Contains(tail, []byte{0xFF, 0xD9}) {
		return errors.New("JPEG is truncated (no end-of-image marker)")
	}
	return nil
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
	c.signalGone() // a capture waiting for the camera stops at once
	var err error
	if opened {
		err = c.closeNative(context.Background(), r)
		if errors.Is(err, errWorkerClosed) {
			err = nil
		}
		if code, ok := sdkCode(err); ok && isDisconnect(code) {
			err = nil
		}
	}
	c.drainItemsQuiet()
	c.closeEvents()
	if err != nil {
		return fmt.Errorf("closing camera session: %w", err)
	}
	return nil
}

// drainItemsQuiet releases queued items on shutdown when the SDK thread may be gone.
func (c *Camera) drainItemsQuiet() {
	for {
		select {
		case it := <-c.items:
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = c.w.doOp(ctx, "skip transfer", func() error { c.a.SkipItem(it.item); return nil })
			cancel()
		default:
			return
		}
	}
}
