package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/capture"
	"github.com/st-mich43l/apexvoid-photobooth/internal/settings"
)

// SelfTest starts the real application core (SQLite, filesystem, loopback
// server, simulated camera) without opening a window, runs one photo session
// and exits. CI runs it on Windows to prove start-up works offline:
//
//	photobooth.exe --selftest [report.json]
//
// It uses a throw-away folder and never touches the user's settings or media.
func SelfTest(version string, args []string) int {
	report := map[string]any{"ok": false}
	finish := func(code int) int {
		if len(args) > 0 {
			b, _ := json.MarshalIndent(report, "", "  ")
			_ = os.WriteFile(args[0], b, 0o644)
		}
		return code
	}
	fail := func(step string, err error) int {
		report["failedStep"], report["error"] = step, err.Error()
		return finish(1)
	}
	dir, err := os.MkdirTemp("", "apexvoid-selftest-*")
	if err != nil {
		return fail("tempdir", err)
	}
	defer os.RemoveAll(dir)
	cfg := filepath.Join(dir, "settings.json")
	sm, _ := settings.Load(cfg)
	st := sm.Get()
	st.MediaDir, st.EnableMock, st.CountdownSeconds, st.Shots = filepath.Join(dir, "lib"), true, 1, 1
	if err := sm.Save(st); err != nil {
		return fail("settings", err)
	}
	svc, err := New(Options{Version: version, ConfigPath: cfg, Tick: 20 * time.Millisecond})
	if err != nil {
		return fail("start", err)
	}
	defer svc.Shutdown()
	devs, err := svc.ListCameras()
	var sim string
	for _, d := range devs {
		if d.Simulated {
			sim = d.ID
		}
	}
	if err != nil || sim == "" {
		return fail("discover", fmt.Errorf("simulated camera not found: %v", err))
	}
	if err := svc.ConnectCamera(sim); err != nil {
		return fail("connect", err)
	}
	if err := svc.StartPreview(); err != nil {
		return fail("preview", err)
	}
	if err := svc.StartShooting(1); err != nil {
		return fail("shoot", err)
	}
	wait := func(want capture.State) bool {
		for i := 0; i < 500 && svc.ShootState().State != want; i++ {
			time.Sleep(20 * time.Millisecond)
		}
		return svc.ShootState().State == want
	}
	if !wait(capture.Review) {
		return fail("review", fmt.Errorf("stuck in %s", svc.ShootState().State))
	}
	if err := svc.AcceptPhotos(); err != nil {
		return fail("accept", err)
	}
	if !wait(capture.Completed) {
		return fail("complete", fmt.Errorf("stuck in %s", svc.ShootState().State))
	}
	ls := svc.LibrarySummary()
	diag := svc.RunDiagnostics()
	report["version"], report["library"], report["checks"], report["network"] = version, ls, diag.Checks, diag.Network
	if ls.Photos != 1 {
		return fail("library", fmt.Errorf("expected 1 photo, got %d", ls.Photos))
	}
	if err := svc.Shutdown(); err != nil {
		return fail("shutdown", err)
	}
	report["ok"] = true
	return finish(0)
}
