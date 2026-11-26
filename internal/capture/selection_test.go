package capture_test

import (
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/capture"
	"github.com/st-mich43l/apexvoid-photobooth/internal/sessions"
)

// selEnv is the engine wired like the application: the store's select / prune / restore hooks, and a finalizer the
// test controls (it plays the frame compositor).
func selEnv(t *testing.T, finalize func(string) (string, error)) *env {
	t.Helper()
	e := newEnv(t, 0)
	e.eng.Shutdown()
	e.eng = capture.New(capture.Options{
		Camera: e.mgr, Store: e.store, Tick: 5 * time.Millisecond, IsSimulated: func() bool { return true },
		SelectPhotos: e.store.SelectPhotos, PruneUnselected: e.store.PruneDiscardedPhotos, RestoreUnselected: e.store.RestoreDiscardedPhotos,
		Finalize: finalize,
	})
	return e
}

func (e *env) review(shots, framePhotos int) capture.Snapshot {
	e.t.Helper()
	e.preview()
	if err := e.eng.Start(capture.Config{Shots: shots, FramePhotos: framePhotos, Countdown: 1, TemplateID: "t"}); err != nil {
		e.t.Fatal(err)
	}
	return e.waitState(capture.Review)
}

func (e *env) photos(sid string) []sessions.Media {
	e.t.Helper()
	ms, err := e.store.ListMedia(sid, 100, 0)
	if err != nil {
		e.t.Fatal(err)
	}
	var out []sessions.Media
	for _, m := range ms {
		if m.Kind == "photo" {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slot < out[j].Slot }) // the compositor reads photos in slot order
	return out
}

func idsBySlot(s capture.Snapshot) map[int]string {
	m := map[int]string{}
	for _, sh := range s.Taken {
		m[sh.Slot] = sh.MediaID
	}
	return m
}

func TestAcceptSelectedKeepsGuestOrderAndPrunesTheRestAfterTheStripExists(t *testing.T) {
	var sawAtFinalize []string
	var e *env
	e = selEnv(t, func(sid string) (string, error) {
		for _, m := range e.photos(sid) {
			sawAtFinalize = append(sawAtFinalize, m.ID)
		}
		return "strip-1", nil
	})
	rv := e.review(8, 3)
	if len(rv.Taken) != 8 {
		t.Fatalf("eight candidates expected, got %d", len(rv.Taken))
	}
	by := idsBySlot(rv)
	if err := e.eng.AcceptSelected([]int{5, 1, 7}); err != nil {
		t.Fatal(err)
	}
	done := e.waitState(capture.Completed)
	want := []string{by[5], by[1], by[7]}
	if len(done.Taken) != 3 {
		t.Fatalf("taken %+v", done.Taken)
	}
	for i, sh := range done.Taken {
		if sh.Slot != i || sh.MediaID != want[i] {
			t.Fatalf("selection order lost: %+v want %v", done.Taken, want)
		}
	}
	// the compositor saw exactly the three, in slot order = selection order
	if len(sawAtFinalize) != 3 || sawAtFinalize[0] != want[0] || sawAtFinalize[1] != want[1] || sawAtFinalize[2] != want[2] {
		t.Fatalf("finalizer saw %v want %v", sawAtFinalize, want)
	}
	if done.ResultID != "strip-1" {
		t.Fatalf("%+v", done)
	}
	left := e.photos(done.SessionID)
	if len(left) != 3 {
		t.Fatalf("the unselected photos must be gone after success, %d remain", len(left))
	}
}

func TestAcceptSelectedRejectsBadSelectionsWithoutChangingAnything(t *testing.T) {
	e := selEnv(t, func(string) (string, error) { return "x", nil })
	rv := e.review(8, 3)
	for name, sel := range map[string][]int{
		"too few": {1, 2}, "too many": {1, 2, 3, 4}, "duplicate": {1, 1, 2}, "unrelated slot": {1, 2, 99}, "negative": {-1, 2, 3}, "empty": nil,
	} {
		if err := e.eng.AcceptSelected(sel); err == nil {
			t.Fatalf("%s must be refused", name)
		}
		if s := e.eng.Snapshot(); s.State != capture.Review || len(s.Taken) != 8 {
			t.Fatalf("%s changed the session: %+v", name, s)
		}
	}
	if n, err := e.store.SessionPhotoCount(rv.SessionID); err != nil || n != 8 {
		t.Fatalf("all eight candidates must still exist, %d do (%v)", n, err)
	}
	if err := e.eng.AcceptSelected([]int{0, 1, 2}); err != nil {
		t.Fatalf("a valid selection after refused ones must work: %v", err)
	}
}

func TestDoubleSubmitComposesOnce(t *testing.T) {
	var calls int32
	e := selEnv(t, func(string) (string, error) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(60 * time.Millisecond)
		return "strip", nil
	})
	e.review(8, 2)
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(i int) { defer wg.Done(); errs[i] = e.eng.AcceptSelected([]int{3, 4}) }(i)
	}
	wg.Wait()
	ok := 0
	for _, err := range errs {
		if err == nil {
			ok++
		} else if !errors.Is(err, capture.ErrInvalidTransition) {
			t.Fatalf("a duplicate submit must be an invalid transition, got %v", err)
		}
	}
	e.waitState(capture.Completed)
	if ok != 1 || atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("accepted %d times, composed %d times", ok, calls)
	}
}

func TestNoRecaptureOnceAccepted(t *testing.T) {
	release := make(chan struct{})
	e := selEnv(t, func(string) (string, error) { <-release; return "s", nil })
	e.review(4, 2)
	go func() { _ = e.eng.AcceptSelected([]int{0, 1}) }()
	e.waitState(capture.Saving)
	if err := e.eng.Retake(0); !errors.Is(err, capture.ErrInvalidTransition) {
		t.Fatalf("a retake while saving must be refused: %v", err)
	}
	close(release)
	e.waitState(capture.Completed)
}

func TestFailedCompositionRestoresEveryPhoto(t *testing.T) {
	e := selEnv(t, func(string) (string, error) { return "", errors.New("render failed") })
	rv := e.review(8, 3)
	by := idsBySlot(rv)
	if err := e.eng.AcceptSelected([]int{6, 0, 2}); err != nil {
		t.Fatal(err)
	}
	done := e.waitState(capture.Completed)
	if done.WarningCode != "composition_failed" || done.ResultID != "" {
		t.Fatalf("%+v", done)
	}
	ms := e.photos(done.SessionID)
	if len(ms) != 8 {
		t.Fatalf("a failed composition must not lose any of the eight photographs, %d remain", len(ms))
	}
	// the chosen three come first, in the guest's order, so a re-compose uses them
	for i, id := range []string{by[6], by[0], by[2]} {
		if ms[i].ID != id {
			t.Fatalf("selected photos must lead in selection order: %v", ms)
		}
	}
}

func TestRejectedSelectionBecauseStoreFailsReturnsToReview(t *testing.T) {
	e := newEnv(t, 0)
	e.eng.Shutdown()
	e.eng = capture.New(capture.Options{Camera: e.mgr, Store: e.store, Tick: 5 * time.Millisecond, IsSimulated: func() bool { return true },
		SelectPhotos: func(string, []int) error { return errors.New("disk full") }})
	e.review(4, 2)
	if err := e.eng.AcceptSelected([]int{0, 1}); err == nil {
		t.Fatal("must fail")
	}
	if s := e.eng.Snapshot(); s.State != capture.Review || len(s.Taken) != 4 {
		t.Fatalf("the guest must be able to try again: %+v", s)
	}
}
