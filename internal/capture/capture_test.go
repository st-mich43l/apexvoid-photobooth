package capture_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
	"github.com/st-mich43l/apexvoid-photobooth/internal/camera/mock"
	"github.com/st-mich43l/apexvoid-photobooth/internal/capture"
	"github.com/st-mich43l/apexvoid-photobooth/internal/sessions"
	"github.com/st-mich43l/apexvoid-photobooth/internal/storage"
)

type env struct {
	t     *testing.T
	drv   *mock.Driver
	mgr   *camera.Manager
	store *sessions.Store
	db    *storage.DB
	eng   *capture.Engine
	mu    sync.Mutex
	seen  []capture.State
}

func newEnv(t *testing.T, retries int) *env {
	t.Helper()
	e := &env{t: t, drv: mock.NewDriver()}
	e.drv.Width, e.drv.Height, e.drv.CaptureDelay = 320, 240, 5*time.Millisecond
	e.mgr = camera.NewManager(e.drv)
	e.mgr.ReconnectInterval = 20 * time.Millisecond
	db, err := storage.Open(storage.Layout{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	e.db, e.store = db, sessions.New(db)
	e.eng = capture.New(capture.Options{
		Camera: e.mgr, Store: e.store, Tick: 5 * time.Millisecond, CaptureRetries: retries,
		IsSimulated: func() bool { return true },
		OnChange: func(s capture.Snapshot) {
			e.mu.Lock()
			if n := len(e.seen); n == 0 || e.seen[n-1] != s.State {
				e.seen = append(e.seen, s.State)
			}
			e.mu.Unlock()
		},
	})
	devs, _ := e.mgr.Discover(context.Background())
	if err := e.mgr.Connect(context.Background(), devs[0]); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.eng.Shutdown(); _ = e.mgr.Shutdown() })
	return e
}

func (e *env) waitState(want capture.State) capture.Snapshot {
	e.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if s := e.eng.Snapshot(); s.State == want {
			return s
		}
		time.Sleep(2 * time.Millisecond)
	}
	e.t.Fatalf("timeout waiting for %s, state=%+v", want, e.eng.Snapshot())
	return capture.Snapshot{}
}

func (e *env) preview() {
	e.t.Helper()
	if err := e.eng.StartPreview(context.Background()); err != nil {
		e.t.Fatal(err)
	}
}

func cfg(shots int) capture.Config {
	return capture.Config{Shots: shots, Countdown: 2, TemplateID: "t"}
}

func TestFullFlowFourShots(t *testing.T) {
	e := newEnv(t, 0)
	e.preview()
	if err := e.eng.Start(cfg(4)); err != nil {
		t.Fatal(err)
	}
	s := e.waitState(capture.Review)
	if len(s.Taken) != 4 {
		t.Fatalf("taken %d", len(s.Taken))
	}
	if err := e.eng.Accept(); err != nil {
		t.Fatal(err)
	}
	s = e.waitState(capture.Completed)
	sess, _ := e.store.Session(s.SessionID)
	if sess.Status != sessions.StatusCompleted || sess.MediaCount != 4 || !sess.Simulated {
		t.Fatalf("session %+v", sess)
	}
	ms, _ := e.store.ListMedia(s.SessionID, 0, 0)
	for _, m := range ms {
		p, _ := e.db.Layout.Abs(m.RelPath)
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("original missing: %v", err)
		}
		if !m.Simulated {
			t.Fatal("simulated capture not flagged in the library")
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	order := []capture.State{capture.Preview, capture.Countdown, capture.Capturing, capture.Review, capture.Saving, capture.Completed}
	i := 0
	for _, st := range e.seen {
		if i < len(order) && st == order[i] {
			i++
		}
	}
	if i != len(order) {
		t.Fatalf("states seen %v do not contain %v in order", e.seen, order)
	}
}

func TestInvalidTransitionsChangeNothing(t *testing.T) {
	e := newEnv(t, 0)
	check := func(name string, err error, want capture.State) {
		t.Helper()
		if !errors.Is(err, capture.ErrInvalidTransition) {
			t.Fatalf("%s: err=%v", name, err)
		}
		if got := e.eng.Snapshot().State; got != want {
			t.Fatalf("%s changed state to %s", name, got)
		}
	}
	check("start from idle", e.eng.Start(cfg(1)), capture.Idle)
	check("accept from idle", e.eng.Accept(), capture.Idle)
	check("retake from idle", e.eng.Retake(0), capture.Idle)
	check("cancel from idle", e.eng.Cancel(), capture.Idle)
	e.preview()
	check("accept from preview", e.eng.Accept(), capture.Preview)
	check("retake from preview", e.eng.Retake(0), capture.Preview)
	check("preview twice", e.eng.StartPreview(context.Background()), capture.Preview)
	if err := e.eng.Start(capture.Config{Shots: 0}); err == nil {
		t.Fatal("bad config accepted")
	}
	_ = e.eng.Start(cfg(1))
	check("start twice", e.eng.Start(cfg(1)), e.eng.Snapshot().State)
	e.waitState(capture.Review)
	check("reset in review", e.eng.Reset(), capture.Review)
	check("retake unknown slot", e.eng.Retake(5), capture.Review)
	if err := e.eng.Accept(); err != nil {
		t.Fatal(err)
	}
	check("accept twice", e.eng.Accept(), capture.Completed)
}

func TestCancelDuringCountdownLeavesCameraFree(t *testing.T) {
	e := newEnv(t, 0)
	e.preview()
	c := capture.Config{Shots: 2, Countdown: 5, TemplateID: "t"}
	_ = e.eng.Start(c)
	e.waitState(capture.Countdown)
	if err := e.eng.Cancel(); err != nil {
		t.Fatal(err)
	}
	s := e.eng.Snapshot()
	if s.State != capture.Cancelled {
		t.Fatalf("state %s", s.State)
	}
	if sess, _ := e.store.Session(s.SessionID); sess.Status != sessions.StatusCancelled || sess.MediaCount != 0 {
		t.Fatalf("session %+v", sess)
	}
	// camera must not be locked: a direct capture works
	if _, err := e.mgr.Capture(context.Background()); err != nil {
		t.Fatalf("camera left locked: %v", err)
	}
	if err := e.eng.Reset(); err != nil {
		t.Fatal(err)
	}
	e.preview()
}

func TestCancelDuringCapture(t *testing.T) {
	e := newEnv(t, 0)
	e.drv.CaptureDelay = 2 * time.Second
	e.preview()
	_ = e.eng.Start(capture.Config{Shots: 1, Countdown: 0, TemplateID: "t"})
	e.waitState(capture.Capturing)
	start := time.Now()
	if err := e.eng.Cancel(); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancel blocked on the camera")
	}
	if e.eng.Snapshot().State != capture.Cancelled {
		t.Fatal("not cancelled")
	}
	e.drv.CaptureDelay = time.Millisecond
	if _, err := e.mgr.Capture(context.Background()); err != nil {
		t.Fatalf("camera still locked: %v", err)
	}
}

func TestRetakeReplacesSlotAndKeepsOldFile(t *testing.T) {
	e := newEnv(t, 0)
	e.preview()
	_ = e.eng.Start(cfg(2))
	s := e.waitState(capture.Review)
	old := s.Taken[1].MediaID
	oldM, _ := e.store.Media(old)
	if err := e.eng.Retake(1); err != nil {
		t.Fatal(err)
	}
	s = e.waitState(capture.Review)
	if len(s.Taken) != 2 || s.Taken[1].MediaID == old {
		t.Fatalf("slot not replaced: %+v", s.Taken)
	}
	if m, _ := e.store.Media(old); m.Status != sessions.MediaDiscarded {
		t.Fatalf("old status %s", m.Status)
	}
	p, _ := e.db.Layout.Abs(oldM.RelPath)
	if _, err := os.Stat(p); err != nil {
		t.Fatal("old original deleted")
	}
	if ms, _ := e.store.ListMedia(s.SessionID, 0, 0); len(ms) != 2 {
		t.Fatalf("visible media %d", len(ms))
	}
}

func TestFailedRetakeKeepsPreviousPhoto(t *testing.T) {
	e := newEnv(t, 0)
	e.preview()
	_ = e.eng.Start(cfg(1))
	s := e.waitState(capture.Review)
	first := s.Taken[0].MediaID
	e.drv.Last().FailNextCapture(camera.Errf(camera.CodeCaptureFailed, "try again", nil, "autofocus failed"))
	if err := e.eng.Retake(0); err != nil {
		t.Fatal(err)
	}
	s = e.waitState(capture.Review)
	if s.Warning == "" || s.Taken[0].MediaID != first {
		t.Fatalf("expected warning and old photo kept: %+v", s)
	}
	if m, _ := e.store.Media(first); m.Status != sessions.MediaActive {
		t.Fatal("old photo no longer active")
	}
}

func TestCaptureRetrySucceeds(t *testing.T) {
	e := newEnv(t, 1)
	e.preview()
	e.drv.Last().FailNextCapture(camera.Errf(camera.CodeCaptureFailed, "", nil, "AF failed"))
	_ = e.eng.Start(cfg(1))
	if s := e.waitState(capture.Review); len(s.Taken) != 1 {
		t.Fatalf("%+v", s)
	}
}

func TestDisconnectMidSessionGoesToErrorAndRecovers(t *testing.T) {
	e := newEnv(t, 0)
	e.preview()
	c := capture.Config{Shots: 2, Countdown: 20, TemplateID: "t"}
	_ = e.eng.Start(c)
	e.waitState(capture.Countdown)
	e.drv.SetPresent(false) // USB unplug
	// during the long countdown nothing fails yet; make the capture happen now
	_ = e.eng.Cancel()
	if err := e.eng.Reset(); err != nil {
		t.Fatal(err)
	}
	if err := e.eng.StartPreview(context.Background()); err == nil {
		t.Fatal("preview must fail while the camera is gone")
	}
	s := e.eng.Snapshot()
	if s.State != capture.Error || s.Error == "" || s.ErrorAction == "" {
		t.Fatalf("want actionable ERROR, got %+v", s)
	}
	if err := e.eng.Reset(); err != nil {
		t.Fatal(err)
	}
	// plug back in; the manager reconnects by itself
	e.drv.SetPresent(true)
	deadline := time.Now().Add(5 * time.Second)
	// wait for the NEW camera: the old one is in StateError until the manager has processed the unplug
	for time.Now().Before(deadline) {
		if st := e.mgr.Status(); st.State == camera.StateConnected && !st.Reconnecting {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	e.preview()
	_ = e.eng.Start(cfg(1))
	e.waitState(capture.Review)
}

func TestCaptureErrorLeavesSessionFailedButPhotosKept(t *testing.T) {
	e := newEnv(t, 0)
	e.preview()
	_ = e.eng.Start(cfg(3))
	e.waitState(capture.Capturing)
	// fail the next shot (not a lost camera) after the first succeeded
	e.drv.Last().FailNextCapture(camera.Errf(camera.CodeCaptureFailed, "retry", nil, "autofocus failed"))
	s := e.waitState(capture.Error)
	sess, _ := e.store.Session(s.SessionID)
	if sess.Status != sessions.StatusFailed {
		t.Fatalf("session %+v", sess)
	}
	if len(s.Taken) == 0 {
		t.Fatalf("photos taken before the failure must be kept: %+v", s)
	}
	if err := e.eng.Reset(); err != nil {
		t.Fatal(err)
	}
}

func TestLostCameraDuringAShotResumesTheSameSessionAndFrame(t *testing.T) {
	var begun []string
	e := newEnv(t, 0)
	e.eng = capture.New(capture.Options{
		Camera: e.mgr, Store: e.store, Tick: 5 * time.Millisecond, CaptureRetries: 0, ReconnectWait: 5 * time.Second,
		IsSimulated:  func() bool { return true },
		BeginSession: func(id string, c capture.Config) error { begun = append(begun, id+"|"+c.FrameID); return nil },
	})
	e.preview()
	c := capture.Config{Shots: 3, Countdown: 1, TemplateID: "t", FrameID: "rose-pink.strip3"}
	if err := e.eng.Start(c); err != nil {
		t.Fatal(err)
	}
	// the first shot succeeds, then the camera drops during the second
	deadline := time.Now().Add(5 * time.Second)
	for len(e.eng.Snapshot().Taken) < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	e.drv.SetPresent(false)
	e.drv.Last().FailNextCapture(camera.Errf(camera.CodeDisconnected, "replug", nil, "gone"))
	deadline = time.Now().Add(5 * time.Second)
	var warned bool
	for time.Now().Before(deadline) {
		if s := e.eng.Snapshot(); s.Warning != "" && s.State == capture.Capturing {
			warned = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !warned {
		t.Fatalf("the operator/guest must be told the camera is being waited for: %+v", e.eng.Snapshot())
	}
	e.drv.SetPresent(true) // plugged back in; the manager reconnects by itself
	s := e.waitState(capture.Review)
	if len(s.Taken) != 3 || s.Warning != "" {
		t.Fatalf("the session must finish with all photos: %+v", s)
	}
	if len(begun) != 1 || begun[0] != s.SessionID+"|rose-pink.strip3" {
		t.Fatalf("the frame must be frozen once and kept: %v", begun)
	}
	sess, _ := e.store.Session(s.SessionID)
	if sess.Status == sessions.StatusFailed {
		t.Fatalf("session %+v", sess)
	}
}

func TestCameraThatNeverComesBackFailsTheSessionButKeepsThePhotos(t *testing.T) {
	e := newEnv(t, 0)
	e.eng = capture.New(capture.Options{
		Camera: e.mgr, Store: e.store, Tick: 5 * time.Millisecond, ReconnectWait: 150 * time.Millisecond,
		IsSimulated: func() bool { return true },
	})
	e.preview()
	_ = e.eng.Start(capture.Config{Shots: 3, Countdown: 1, TemplateID: "t"})
	deadline := time.Now().Add(5 * time.Second)
	for len(e.eng.Snapshot().Taken) < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	e.drv.SetPresent(false)
	e.drv.Last().FailNextCapture(camera.Errf(camera.CodeDisconnected, "replug", nil, "gone"))
	s := e.waitState(capture.Error)
	if s.ErrorAction == "" || len(s.Taken) == 0 {
		t.Fatalf("%+v", s)
	}
	sess, _ := e.store.Session(s.SessionID)
	if sess.Status != sessions.StatusFailed {
		t.Fatalf("session %+v", sess)
	}
}

func TestFailedRetakeAfterCameraLossKeepsThePreviousPhoto(t *testing.T) {
	e := newEnv(t, 0)
	e.eng = capture.New(capture.Options{
		Camera: e.mgr, Store: e.store, Tick: 5 * time.Millisecond, ReconnectWait: 100 * time.Millisecond,
		IsSimulated: func() bool { return true },
	})
	e.preview()
	_ = e.eng.Start(cfg(2))
	s := e.waitState(capture.Review)
	old := s.Taken[0].MediaID
	e.drv.SetPresent(false)
	e.drv.Last().FailNextCapture(camera.Errf(camera.CodeDisconnected, "replug", nil, "gone"))
	if err := e.eng.Retake(0); err != nil {
		t.Fatal(err)
	}
	s = e.waitState(capture.Review)
	if len(s.Taken) != 2 || s.Taken[0].MediaID != old || s.Warning == "" {
		t.Fatalf("the earlier valid photo must be kept: %+v", s)
	}
}

func TestRepeatedSessions(t *testing.T) {
	e := newEnv(t, 0)
	ids := map[string]bool{}
	for i := 0; i < 5; i++ {
		e.preview()
		if err := e.eng.Start(cfg(2)); err != nil {
			t.Fatal(err)
		}
		s := e.waitState(capture.Review)
		if ids[s.SessionID] {
			t.Fatal("duplicate session id")
		}
		ids[s.SessionID] = true
		_ = e.eng.Accept()
		e.waitState(capture.Completed)
		if err := e.eng.Reset(); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := e.store.List(10, 0)
	if len(list) != 5 {
		t.Fatalf("sessions %d", len(list))
	}
	if ms, _ := e.store.ListMedia("", 0, 0); len(ms) != 10 {
		t.Fatalf("media %d", len(ms))
	}
}

func TestShutdownReleasesCameraAndMarksInterrupted(t *testing.T) {
	e := newEnv(t, 0)
	e.preview()
	c := capture.Config{Shots: 1, Countdown: 30, TemplateID: "t"}
	_ = e.eng.Start(c)
	s := e.waitState(capture.Countdown)
	cam := e.drv.Last()
	e.eng.Shutdown()
	if err := e.mgr.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if !cam.Released() {
		t.Fatal("camera not released on shutdown")
	}
	if sess, _ := e.store.Session(s.SessionID); sess.Status != sessions.StatusInterrupted {
		t.Fatalf("session %+v", sess)
	}
}

// The UI drops every snapshot whose version is lower than the newest it has seen. The version
// must therefore never go back — in particular not when the engine is reset after a finished
// session, otherwise the next session's screens are ignored and the guest stays on "All done".
func TestSnapshotVersionNeverGoesBackAcrossSessionsAndResets(t *testing.T) {
	e := newEnv(t, 0)
	last := e.eng.Snapshot().Version
	check := func(step string) {
		t.Helper()
		v := e.eng.Snapshot().Version
		if v <= last {
			t.Fatalf("%s: version %d is not newer than %d", step, v, last)
		}
		last = v
	}
	for i := 0; i < 3; i++ {
		e.preview()
		check("preview")
		if err := e.eng.Start(cfg(2)); err != nil {
			t.Fatal(err)
		}
		e.waitState(capture.Review)
		check("review")
		_ = e.eng.Accept()
		e.waitState(capture.Completed)
		check("completed")
		if err := e.eng.Reset(); err != nil {
			t.Fatal(err)
		}
		check("reset")
	}
	// cancel and reset paths too
	e.preview()
	_ = e.eng.Start(cfg(2))
	e.waitState(capture.Review)
	_ = e.eng.Cancel()
	check("cancel")
	_ = e.eng.Reset()
	check("reset after cancel")
}
