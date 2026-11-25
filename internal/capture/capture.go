// Package capture implements the photo-shooting state machine:
//
//	IDLE -> PREVIEW -> COUNTDOWN -> CAPTURING -> (COUNTDOWN ...) -> REVIEW -> SAVING -> COMPLETED
//
// with explicit ERROR and CANCELLED states. Every transition is validated
// under one lock; an invalid request returns ErrInvalidTransition and changes
// nothing, so session data and the camera are never left in a half state.
package capture

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
	"github.com/st-mich43l/apexvoid-photobooth/internal/sessions"
)

// State of the shooting engine.
type State string

const (
	Idle      State = "IDLE"
	Preview   State = "PREVIEW"
	Countdown State = "COUNTDOWN"
	Capturing State = "CAPTURING"
	Review    State = "REVIEW"
	Saving    State = "SAVING"
	Completed State = "COMPLETED"
	Error     State = "ERROR"
	Cancelled State = "CANCELLED"
)

// ErrInvalidTransition is returned for a request not allowed in the current state.
var ErrInvalidTransition = errors.New("invalid state transition")

// Camera is what the engine needs from the camera subsystem (camera.Manager).
type Camera interface {
	StartLiveView(ctx context.Context) error
	StopLiveView() error
	Capture(ctx context.Context) (camera.Photo, error)
}

// Store is what the engine needs from the session repository.
type Store interface {
	Create(mode string, shots int, templateID string, simulated bool) (sessions.Session, error)
	AddPhoto(sessionID string, in sessions.PhotoInput) (sessions.Media, error)
	DiscardMedia(id string) error
	SetStatus(id, status, note string) error
}

// Config of one shooting session.
type Config struct {
	Shots       int
	FramePhotos int
	Countdown   int // seconds
	TemplateID  string
	// Frame selection, carried to Options.BeginSession untouched (the engine does not interpret it).
	FrameID, VariantID, Caption string
}

// Shot is one accepted-or-reviewable photo.
type Shot struct {
	Slot      int    `json:"slot"`
	MediaID   string `json:"mediaId"`
	Simulated bool   `json:"simulated"`
}

// Snapshot is the externally visible state.
type Snapshot struct {
	State       State  `json:"state"`
	Version     uint64 `json:"version"`
	SessionID   string `json:"sessionId"`
	Shots       int    `json:"shots"`     // planned
	Slot        int    `json:"slot"`      // slot being shot (0-based)
	Remaining   int    `json:"remaining"` // countdown seconds left
	Taken       []Shot `json:"taken"`
	Error       string `json:"error,omitempty"`
	ErrorAction string `json:"errorAction,omitempty"`
	Warning     string `json:"warning,omitempty"` // non-fatal, e.g. a failed retake (old photo kept)
	WarningCode string `json:"warningCode,omitempty"`
	Simulated   bool   `json:"simulated"`
	ResultID    string `json:"resultId,omitempty"` // composed strip, if the finalizer produced one
}

// Options wires the engine.
type Options struct {
	Camera Camera
	Store  Store
	// Tick is the length of one countdown second (default 1s; tests use ms).
	Tick time.Duration
	// IsSimulated tells whether the active camera is the mock one.
	IsSimulated func() bool
	// Finalize runs in SAVING (compose strip, ...). It may return a result media id.
	Finalize func(sessionID string) (resultID string, err error)
	// BeginSession runs once the session exists, before the first countdown. It
	// freezes the chosen frame for the session. An error aborts the session
	// (nothing is shot) and is shown to the operator.
	BeginSession func(sessionID string, cfg Config) error
	// KeepSelectedPhotos keeps the selected captures for the frame and removes
	// every other candidate photo from the session/library.
	KeepSelectedPhotos func(sessionID string, selectedSlots []int) error
	// OnChange receives every new snapshot, in order.
	OnChange func(Snapshot)
	// CaptureRetries is how many times a failed capture is retried (not for disconnects).
	CaptureRetries int
	// ReconnectWait is how long a session waits for a camera that was lost
	// during a shot (default 60 s; negative = do not wait, fail at once). The
	// session, its frame and the photos already taken are kept meanwhile.
	ReconnectWait time.Duration
}

// Engine is the state machine. Create with New.
type Engine struct {
	o    Options
	tr   sync.Mutex // serialises session transitions that touch the camera: StartPreview, Cancel, Reset, Recover
	emit sync.Mutex // orders OnChange calls
	mu   sync.Mutex
	s    Snapshot
	gen  uint64 // bumped on cancel/reset; stale workers discard their results
	stop context.CancelFunc
	done chan struct{}
	cfg  Config
}

// New creates an engine in IDLE.
func New(o Options) *Engine {
	if o.Tick <= 0 {
		o.Tick = time.Second
	}
	if o.IsSimulated == nil {
		o.IsSimulated = func() bool { return false }
	}
	return &Engine{o: o, s: Snapshot{State: Idle, Taken: []Shot{}}}
}

// Snapshot returns the current state.
func (e *Engine) Snapshot() Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	return clone(e.s)
}

func clone(s Snapshot) Snapshot {
	s.Taken = append([]Shot{}, s.Taken...)
	return s
}

// update applies f under the lock, bumps the version and publishes.
func (e *Engine) update(f func(s *Snapshot)) Snapshot {
	e.emit.Lock()
	defer e.emit.Unlock()
	e.mu.Lock()
	// Version only ever grows. Callers replace the whole snapshot (*s = Snapshot{...}), which would
	// reset it to zero; the UI drops any update older than the newest one it has seen, so a reset
	// version made it ignore every event of the next session.
	v := e.s.Version
	f(&e.s)
	e.s.Version = v + 1
	snap := clone(e.s)
	e.mu.Unlock()
	if e.o.OnChange != nil {
		e.o.OnChange(snap)
	}
	return snap
}

func invalid(from State, action string) error {
	return fmt.Errorf("%w: cannot %s while %s", ErrInvalidTransition, action, from)
}

// StartPreview turns live view on: IDLE -> PREVIEW.
func (e *Engine) StartPreview(ctx context.Context) error {
	e.tr.Lock()
	defer e.tr.Unlock()
	e.mu.Lock()
	if e.s.State != Idle {
		st := e.s.State
		e.mu.Unlock()
		return invalid(st, "start preview")
	}
	e.mu.Unlock()
	if err := e.o.Camera.StartLiveView(ctx); err != nil {
		e.fail(err, "")
		return err
	}
	e.mu.Lock()
	if e.s.State != Idle { // raced with Shutdown/Reset
		e.mu.Unlock()
		return nil
	}
	e.mu.Unlock()
	e.update(func(s *Snapshot) {
		s.State = Preview
		s.Error, s.ErrorAction, s.Warning, s.WarningCode = "", "", "", ""
	})
	return nil
}

// Start begins a session: PREVIEW -> COUNTDOWN.
func (e *Engine) Start(cfg Config) error {
	if cfg.Shots < 1 || cfg.Shots > 12 || cfg.FramePhotos < 0 || cfg.FramePhotos > cfg.Shots || cfg.Countdown < 0 || cfg.Countdown > 30 {
		return fmt.Errorf("invalid shooting configuration %+v", cfg)
	}
	e.mu.Lock()
	if e.s.State != Preview {
		st := e.s.State
		e.mu.Unlock()
		return invalid(st, "start")
	}
	e.mu.Unlock()
	sess, err := e.o.Store.Create("photo", cfg.Shots, cfg.TemplateID, e.o.IsSimulated())
	if err != nil {
		e.fail(fmt.Errorf("could not create session: %w", err), "Check that the media folder is writable and has free space.")
		return err
	}
	if e.o.BeginSession != nil {
		if err := e.o.BeginSession(sess.ID, cfg); err != nil {
			_ = e.o.Store.SetStatus(sess.ID, sessions.StatusFailed, "frame: "+err.Error())
			return err
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.mu.Lock()
	if e.s.State != Preview { // lost a race with Cancel/Reset
		e.mu.Unlock()
		cancel()
		_ = e.o.Store.SetStatus(sess.ID, sessions.StatusCancelled, "")
		return invalid(e.Snapshot().State, "start")
	}
	e.gen++
	gen := e.gen
	e.stop, e.done, e.cfg = cancel, make(chan struct{}), cfg
	done := e.done
	e.mu.Unlock()
	e.update(func(s *Snapshot) {
		*s = Snapshot{State: Countdown, SessionID: sess.ID, Shots: cfg.Shots, Slot: 0, Remaining: cfg.Countdown, Taken: []Shot{}, Simulated: sess.Simulated}
	})
	go func() {
		defer close(done)
		e.runShots(ctx, gen, sess.ID, 0, cfg.Shots, "", cfg.FramePhotos > 0)
	}()
	return nil
}

// runShots takes slots [from, from+count). replace is the media id being
// replaced by a retake ("" for a first take).
func (e *Engine) runShots(ctx context.Context, gen uint64, sessionID string, from, count int, replace string, candidate bool) {
	resumes := 0
	for slot := from; slot < from+count; slot++ {
		if !e.countdown(ctx, gen, slot) {
			return
		}
		e.update(func(s *Snapshot) {
			if e.gen == gen {
				s.State, s.Slot, s.Remaining = Capturing, slot, 0
			}
		})
		photo, err := e.captureWithRetry(ctx, gen)
		if ctx.Err() != nil || !e.current(gen) {
			return
		}
		if err != nil && isLost(err) && e.o.ReconnectWait >= 0 && resumes < maxResumes {
			// The USB link dropped during the shot. Nothing is lost: the session, the frame
			// snapshot and the photos already taken stay as they are; wait for the manager to
			// reconnect, then repeat this slot from its countdown.
			resumes++
			if e.awaitCamera(ctx, gen) {
				slot-- // same slot again (the loop increments it)
				continue
			}
			if ctx.Err() != nil || !e.current(gen) {
				return
			}
			err = camera.Errf(camera.CodeDisconnected, "Check the USB cable and that the camera is on, then start again. The photos already taken were kept.", err,
				"the camera did not come back")
		}
		if err != nil {
			if replace != "" { // failed retake: keep the old photo, back to review
				e.update(func(s *Snapshot) {
					s.State = Review
					s.WarningCode = "retake_failed"
					s.Warning = "Retake failed: " + err.Error() + " The previous photo was kept."
				})
				return
			}
			e.failSession(sessionID, err)
			return
		}
		m, err := e.o.Store.AddPhoto(sessionID, sessions.PhotoInput{Slot: slot, Data: photo.Data, Simulated: photo.Simulated, Source: photo.Source, Candidate: candidate})
		if m.ID == "" { // nothing was stored
			e.failSession(sessionID, fmt.Errorf("could not save photo: %w", err))
			return
		}
		// err != nil with m.ID set only means thumbnails failed; the original is safe.
		if replace != "" {
			_ = e.o.Store.DiscardMedia(replace)
		}
		e.update(func(s *Snapshot) {
			if e.gen != gen {
				return
			}
			shot := Shot{Slot: slot, MediaID: m.ID, Simulated: m.Simulated}
			replaced := false
			for i := range s.Taken {
				if s.Taken[i].Slot == slot {
					s.Taken[i], replaced = shot, true
				}
			}
			if !replaced {
				s.Taken = append(s.Taken, shot)
			}
			if photo.Simulated {
				s.Simulated = true
			}
		})
	}
	e.update(func(s *Snapshot) {
		if e.gen == gen {
			s.State, s.Remaining, s.Warning, s.WarningCode = Review, 0, "", ""
		}
	})
}

func (e *Engine) current(gen uint64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.gen == gen
}

func (e *Engine) countdown(ctx context.Context, gen uint64, slot int) bool {
	for r := e.cfg.Countdown; r >= 1; r-- {
		e.update(func(s *Snapshot) {
			if e.gen == gen {
				s.State, s.Slot, s.Remaining = Countdown, slot, r
			}
		})
		select {
		case <-ctx.Done():
			return false
		case <-time.After(e.o.Tick):
		}
	}
	return ctx.Err() == nil
}

// maxResumes bounds how often one run of shots may wait for a lost camera.
const maxResumes = 5

func isLost(err error) bool {
	switch camera.CodeOf(err) {
	case camera.CodeDisconnected, camera.CodeNotConnected:
		return true
	}
	return false
}

// awaitCamera waits until the camera answers again. It reports true when the
// camera is back and false when the wait ended (timeout, cancel, session gone).
func (e *Engine) awaitCamera(ctx context.Context, gen uint64) bool {
	wait := e.o.ReconnectWait
	if wait == 0 {
		wait = 60 * time.Second
	}
	poll := e.o.Tick / 2
	if poll < 20*time.Millisecond {
		poll = 20 * time.Millisecond
	}
	if poll > time.Second {
		poll = time.Second
	}
	e.update(func(s *Snapshot) {
		if e.gen == gen {
			s.State, s.Remaining = Capturing, 0
			s.WarningCode = "camera_reconnecting"
			s.Warning = "The camera was disconnected. Waiting for it to reconnect - your photos and frame are kept."
		}
	})
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return false
		case <-time.After(poll):
		}
		if !e.current(gen) {
			return false
		}
		pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := e.o.Camera.StartLiveView(pctx) // idempotent when it is already running
		cancel()
		if err == nil {
			e.update(func(s *Snapshot) {
				if e.gen == gen {
					s.Warning, s.WarningCode = "", ""
				}
			})
			return true
		}
	}
}

func (e *Engine) captureWithRetry(ctx context.Context, gen uint64) (camera.Photo, error) {
	var photo camera.Photo
	var err error
	for attempt := 0; attempt <= e.o.CaptureRetries; attempt++ {
		photo, err = e.o.Camera.Capture(ctx)
		if err == nil || ctx.Err() != nil {
			return photo, err
		}
		switch camera.CodeOf(err) {
		case camera.CodeCaptureFailed, camera.CodeTimeout:
			select {
			case <-ctx.Done():
				return photo, ctx.Err()
			case <-time.After(e.o.Tick / 2):
			}
			continue
		}
		break
	}
	return photo, err
}

// Retake re-shoots one slot: REVIEW -> COUNTDOWN -> CAPTURING -> REVIEW.
func (e *Engine) Retake(slot int) error {
	e.mu.Lock()
	if e.s.State != Review {
		st := e.s.State
		e.mu.Unlock()
		return invalid(st, "retake")
	}
	var old string
	for _, t := range e.s.Taken {
		if t.Slot == slot {
			old = t.MediaID
		}
	}
	if old == "" {
		e.mu.Unlock()
		return fmt.Errorf("%w: no photo in slot %d", ErrInvalidTransition, slot)
	}
	sid, cfg := e.s.SessionID, e.cfg
	ctx, cancel := context.WithCancel(context.Background())
	e.gen++
	gen := e.gen
	e.stop, e.done = cancel, make(chan struct{})
	done := e.done
	e.mu.Unlock()
	e.update(func(s *Snapshot) {
		s.State, s.Slot, s.Remaining, s.Warning, s.WarningCode = Countdown, slot, cfg.Countdown, "", ""
	})
	go func() {
		defer close(done)
		e.runShots(ctx, gen, sid, slot, 1, old, cfg.FramePhotos > 0)
	}()
	return nil
}

// Accept finalises the session: REVIEW -> SAVING -> COMPLETED.
func (e *Engine) Accept() error {
	e.mu.Lock()
	framePhotos := e.cfg.FramePhotos
	e.mu.Unlock()
	selected := make([]int, framePhotos)
	for i := range selected {
		selected[i] = i
	}
	return e.AcceptSelected(selected)
}

// AcceptSelected validates the chosen captures, moves them to the front of
// the session's photo order, then finalizes the frame.
func (e *Engine) AcceptSelected(selectedSlots []int) error {
	e.mu.Lock()
	if e.s.State != Review {
		st := e.s.State
		e.mu.Unlock()
		return invalid(st, "accept")
	}
	sid := e.s.SessionID
	framePhotos := e.cfg.FramePhotos
	if framePhotos > 0 {
		if len(selectedSlots) != framePhotos {
			e.mu.Unlock()
			return fmt.Errorf("select exactly %d photos for the frame", framePhotos)
		}
		taken := make(map[int]bool, len(e.s.Taken))
		for _, shot := range e.s.Taken {
			taken[shot.Slot] = true
		}
		seen := make(map[int]bool, len(selectedSlots))
		for _, slot := range selectedSlots {
			if !taken[slot] || seen[slot] {
				e.mu.Unlock()
				return fmt.Errorf("selected photos must be unique captures from this session")
			}
			seen[slot] = true
		}
	}
	shots := append([]Shot(nil), e.s.Taken...)
	e.gen++
	gen := e.gen
	e.mu.Unlock()
	e.update(func(s *Snapshot) { s.State = Saving })
	if e.o.KeepSelectedPhotos != nil && framePhotos > 0 {
		if err := e.o.KeepSelectedPhotos(sid, selectedSlots); err != nil {
			e.update(func(s *Snapshot) { s.State = Review })
			return err
		}
	}
	if !e.current(gen) {
		return nil
	}
	if framePhotos > 0 {
		bySlot := make(map[int]Shot, len(shots))
		for _, shot := range shots {
			bySlot[shot.Slot] = shot
		}
		ordered := make([]Shot, 0, len(selectedSlots))
		for i, slot := range selectedSlots {
			shot := bySlot[slot]
			shot.Slot = i
			ordered = append(ordered, shot)
		}
		e.update(func(s *Snapshot) { s.Taken = ordered })
	}
	var result string
	var ferr error
	if e.o.Finalize != nil {
		result, ferr = e.o.Finalize(sid)
	}
	if !e.current(gen) {
		return nil
	}
	if ferr != nil {
		// photos are already safe on disk; finish the session and warn
		_ = e.o.Store.SetStatus(sid, sessions.StatusCompleted, "finalize: "+ferr.Error())
		e.update(func(s *Snapshot) {
			s.State = Completed
			s.WarningCode = "composition_failed"
			s.Warning = "Photos saved, but the strip could not be created: " + ferr.Error()
		})
		return nil
	}
	if err := e.o.Store.SetStatus(sid, sessions.StatusCompleted, ""); err != nil {
		e.failSession(sid, fmt.Errorf("could not finish session: %w", err))
		return err
	}
	e.update(func(s *Snapshot) { s.State = Completed; s.ResultID = result })
	return nil
}

// Cancel aborts COUNTDOWN/CAPTURING/REVIEW: -> CANCELLED. Photos already
// stored stay on disk (session marked cancelled).
func (e *Engine) Cancel() error {
	e.tr.Lock()
	defer e.tr.Unlock()
	return e.cancelLocked()
}

func (e *Engine) cancelLocked() error {
	e.mu.Lock()
	switch e.s.State {
	case Countdown, Capturing, Review:
	default:
		st := e.s.State
		e.mu.Unlock()
		return invalid(st, "cancel")
	}
	sid := e.s.SessionID
	stop, done := e.stop, e.done
	e.gen++
	e.mu.Unlock()
	if stop != nil {
		stop()
	}
	waitFor(done, 10*time.Second)
	_ = e.o.Store.SetStatus(sid, sessions.StatusCancelled, "")
	e.update(func(s *Snapshot) { s.State, s.Remaining = Cancelled, 0 })
	return nil
}

// Reset returns to IDLE from PREVIEW, COMPLETED, CANCELLED or ERROR, and
// releases live view so the camera is not left busy.
func (e *Engine) Reset() error {
	e.tr.Lock()
	defer e.tr.Unlock()
	return e.resetLocked()
}

func (e *Engine) resetLocked() error {
	e.mu.Lock()
	switch e.s.State {
	case Idle, Preview, Completed, Cancelled, Error:
	default:
		st := e.s.State
		e.mu.Unlock()
		return invalid(st, "reset")
	}
	e.gen++
	e.mu.Unlock()
	_ = e.o.Camera.StopLiveView()
	e.update(func(s *Snapshot) { *s = Snapshot{State: Idle, Taken: []Shot{}} })
	return nil
}

// Recover returns the engine to IDLE from whatever state it is in, and is safe to
// call repeatedly and concurrently: it is the one operation a screen needs when
// it is done with a session (Finish, Home, an error screen, a stale leftover).
//
//   - COUNTDOWN / CAPTURING / REVIEW: the session is cancelled first (originals
//     already taken stay in the library, the session is marked cancelled);
//   - SAVING: it waits (up to wait) for the save to finish — an in-flight save is never cut off;
//   - PREVIEW / COMPLETED / CANCELLED / ERROR: live view is released;
//   - IDLE: nothing to do.
//
// It returns an error only if the engine could not be brought back to IDLE.
func (e *Engine) Recover(wait time.Duration) error {
	e.tr.Lock()
	defer e.tr.Unlock()
	deadline := time.Now().Add(wait)
	for {
		switch st := e.Snapshot().State; st {
		case Idle:
			return nil
		case Countdown, Capturing, Review:
			if err := e.cancelLocked(); err != nil {
				return err
			}
		case Saving:
			if time.Now().After(deadline) {
				return fmt.Errorf("the photos are still being saved")
			}
			time.Sleep(20 * time.Millisecond)
		default: // Preview, Completed, Cancelled, Error
			return e.resetLocked()
		}
	}
}

// Shutdown stops everything (application exit).
func (e *Engine) Shutdown() {
	e.mu.Lock()
	sid, st := e.s.SessionID, e.s.State
	stop, done := e.stop, e.done
	e.gen++
	e.mu.Unlock()
	if stop != nil {
		stop()
	}
	waitFor(done, 10*time.Second)
	switch st {
	case Countdown, Capturing, Review, Saving:
		_ = e.o.Store.SetStatus(sid, sessions.StatusInterrupted, "application closed")
	}
	_ = e.o.Camera.StopLiveView()
}

func (e *Engine) fail(err error, action string) {
	if action == "" {
		action = camera.ActionOf(err)
	}
	e.update(func(s *Snapshot) {
		s.State, s.Remaining = Error, 0
		s.Error, s.ErrorAction = err.Error(), action
	})
}

func (e *Engine) failSession(sessionID string, err error) {
	_ = e.o.Store.SetStatus(sessionID, sessions.StatusFailed, err.Error())
	e.fail(err, "")
}

func waitFor(ch chan struct{}, d time.Duration) {
	if ch == nil {
		return
	}
	select {
	case <-ch:
	case <-time.After(d):
	}
}
