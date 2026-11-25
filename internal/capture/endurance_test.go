package capture_test

import (
	"context"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/capture"
	"github.com/st-mich43l/apexvoid-photobooth/internal/sessions"
)

// TestSimulatedEndurance100Sessions is explicitly simulated software
// qualification, never evidence for a camera, physical paper or kiosk soak.
func TestSimulatedEndurance100Sessions(t *testing.T) {
	if os.Getenv("UX6_ENDURANCE") != "1" {
		t.Skip("set UX6_ENDURANCE=1 to run simulated endurance")
	}
	e := newEnv(t, 0)
	start := time.Now()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for i := 0; i < 100; i++ {
		if err := e.eng.StartPreview(context.Background()); err != nil {
			t.Fatalf("session %d preview: %v", i, err)
		}
		if err := e.eng.Start(capture.Config{Shots: 1, Countdown: 1}); err != nil {
			t.Fatalf("session %d start: %v", i, err)
		}
		shot := e.waitState(capture.Review)
		if len(shot.Taken) != 1 || !shot.Simulated {
			t.Fatalf("session %d review %+v", i, shot)
		}
		if err := e.eng.Accept(); err != nil {
			t.Fatalf("session %d accept: %v", i, err)
		}
		result := e.waitState(capture.Completed)
		sess, err := e.store.Session(result.SessionID)
		if err != nil || sess.Status != sessions.StatusCompleted || sess.MediaCount != 1 {
			t.Fatalf("session %d: %+v %v", i, sess, err)
		}
		media, err := e.store.ListMedia(sess.ID, 10, 0)
		if err != nil || len(media) != 1 {
			t.Fatalf("session %d media: %v %+v", i, err, media)
		}
		path, err := e.db.Layout.Abs(media[0].RelPath)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = os.Stat(path); err != nil {
			t.Fatalf("session %d original: %v", i, err)
		}
		if err := e.eng.Reset(); err != nil {
			t.Fatalf("session %d reset: %v", i, err)
		}
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	t.Logf("SIMULATED only: 100 completed sessions; elapsed=%s; heap_before=%d heap_after=%d goroutines=%d", time.Since(start), before.HeapAlloc, after.HeapAlloc, runtime.NumGoroutine())
}
