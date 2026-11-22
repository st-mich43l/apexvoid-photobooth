package edsdk

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"
)

var errWorkerClosed = errors.New("EDSDK worker stopped")

// stuckAfter is how long one SDK call may run before diagnostics call it stuck.
var stuckAfter = 20 * time.Second

type job struct {
	op  string
	run func()
}

// Health is an observable snapshot of the SDK thread.
type Health struct {
	Running   bool   `json:"running"`
	Busy      bool   `json:"busy"`
	Op        string `json:"op,omitempty"` // the SDK operation in progress
	RunningMs int64  `json:"runningMs"`
	Stuck     bool   `json:"stuck"`     // one call exceeded stuckAfter
	Abandoned uint64 `json:"abandoned"` // calls whose caller gave up (cancelled) while they ran
	Completed uint64 `json:"completed"`
}

// worker owns the SDK thread: it locks one goroutine to one OS thread, calls
// Init on it, then serves jobs and pumps SDK events/messages between them.
// EDSDK is not thread-safe and delivers callbacks on the initialising thread,
// so exactly one native operation runs at a time (jobs are never overlapped).
type worker struct {
	a    api
	jobs chan job
	quit chan struct{}
	done chan struct{}
	once sync.Once

	mu        sync.Mutex
	op        string
	since     time.Time
	completed uint64
	abandoned uint64
}

func startWorker(a api) (*worker, error) {
	w := &worker{a: a, jobs: make(chan job), quit: make(chan struct{}), done: make(chan struct{})}
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(w.done)
		if err := a.Init(); err != nil {
			// Init cleans up after itself; Terminate is only for a successful Init.
			ready <- err
			return
		}
		ready <- nil
		defer a.Terminate()
		t := time.NewTicker(25 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case j := <-w.jobs:
				w.begin(j.op)
				j.run()
				w.end()
				a.PumpEvents()
			case <-t.C:
				a.PumpEvents()
			case <-w.quit:
				return
			}
		}
	}()
	if err := <-ready; err != nil {
		return nil, err
	}
	return w, nil
}

func (w *worker) begin(op string) {
	w.mu.Lock()
	w.op, w.since = op, time.Now()
	w.mu.Unlock()
}

func (w *worker) end() {
	w.mu.Lock()
	w.op, w.since = "", time.Time{}
	w.completed++
	w.mu.Unlock()
}

// Health reports what the SDK thread is doing right now.
func (w *worker) Health() Health {
	w.mu.Lock()
	defer w.mu.Unlock()
	h := Health{Running: true, Completed: w.completed, Abandoned: w.abandoned}
	select {
	case <-w.done:
		h.Running = false
	default:
	}
	if w.op != "" {
		h.Busy, h.Op = true, w.op
		run := time.Since(w.since)
		h.RunningMs = run.Milliseconds()
		h.Stuck = run > stuckAfter
	}
	return h
}

func (w *worker) noteAbandoned() {
	w.mu.Lock()
	w.abandoned++
	w.mu.Unlock()
}

// call runs fn on the SDK thread and returns its result. The result travels
// through a buffered channel, never through variables shared with the caller:
// if ctx ends while fn is still running, fn finishes on its own, the SDK
// resources it touches stay owned by the SDK thread, and nothing races with
// the caller. A job that was never accepted never runs.
func call[T any](ctx context.Context, w *worker, op string, fn func() (T, error)) (T, error) {
	type res struct {
		v   T
		err error
	}
	var zero T
	ch := make(chan res, 1)
	j := job{op: op, run: func() { v, err := fn(); ch <- res{v, err} }}
	select {
	case w.jobs <- j:
	case <-w.done:
		return zero, errWorkerClosed
	case <-ctx.Done():
		return zero, ctx.Err()
	}
	select {
	case r := <-ch:
		return r.v, r.err
	case <-ctx.Done():
		w.noteAbandoned()
		return zero, ctx.Err()
	}
}

// do runs fn on the SDK thread and waits for it (or ctx).
func (w *worker) do(ctx context.Context, fn func() error) error {
	_, err := call(ctx, w, "", func() (struct{}, error) { return struct{}{}, fn() })
	return err
}

// doOp is do with a name that shows up in Health while the call runs.
func (w *worker) doOp(ctx context.Context, op string, fn func() error) error {
	_, err := call(ctx, w, op, func() (struct{}, error) { return struct{}{}, fn() })
	return err
}

// detached returns a context that ignores the caller's cancellation but is
// bounded by d. It is used for native work that must run to completion once
// started (finishing a transfer, closing a session) so a cancelled caller can
// never leave a native reference behind.
func detached(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), d)
}

// stop terminates the SDK (EdsTerminateSDK) and waits for the thread to exit,
// but never longer than within. A stuck SDK call cannot be interrupted; in that
// case stop reports which operation was blocking so shutdown stays bounded and
// the cause is visible.
func (w *worker) stop(within time.Duration) error {
	w.once.Do(func() { close(w.quit) })
	select {
	case <-w.done:
		return nil
	case <-time.After(within):
		h := w.Health()
		op := h.Op
		if op == "" {
			op = "an SDK call"
		}
		return fmt.Errorf("Canon EDSDK did not shut down within %s (blocked in %s)", within, op)
	}
}
