package edsdk

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"time"
)

var errWorkerClosed = errors.New("EDSDK worker stopped")

type job struct {
	fn  func() error
	res chan error
}

// worker owns the SDK thread: it locks one goroutine to one OS thread, calls
// Init on it, then serves jobs and pumps SDK events/messages between them.
// EDSDK is not thread-safe and delivers callbacks on the initialising thread.
type worker struct {
	a    api
	jobs chan job
	quit chan struct{}
	done chan struct{}
	once sync.Once
}

func startWorker(a api) (*worker, error) {
	w := &worker{a: a, jobs: make(chan job), quit: make(chan struct{}), done: make(chan struct{})}
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(w.done)
		if err := a.Init(); err != nil {
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
				j.res <- j.fn()
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

// do runs fn on the SDK thread and waits for it (or ctx).
func (w *worker) do(ctx context.Context, fn func() error) error {
	j := job{fn: fn, res: make(chan error, 1)}
	select {
	case w.jobs <- j:
	case <-w.done:
		return errWorkerClosed
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-j.res:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// stop terminates the SDK (EdsTerminateSDK) and waits for the thread to exit.
func (w *worker) stop() {
	w.once.Do(func() { close(w.quit) })
	<-w.done
}
