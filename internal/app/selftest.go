package app

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/capture"
	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
	"github.com/st-mich43l/apexvoid-photobooth/internal/settings"
	"github.com/st-mich43l/apexvoid-photobooth/internal/templates"
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
	if err := svc.StartShootingWithCount("plain.single", "", "", 1); err != nil {
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
	// the frame flow end to end: choose a four-photo frame, shoot, compose, and make the two-up 4×6 print sheet
	ff, err := selfTestFrameFlow(svc, wait, evidenceDir(args), 300)
	report["frameFlow"] = ff
	if err != nil {
		return fail("frame flow", err)
	}
	cfg600 := svc.settings.Get()
	cfg600.OutputDPI = 600
	if err := svc.settings.Save(cfg600); err != nil {
		return fail("600 dpi setting", err)
	}
	ff600, err := selfTestFrameFlow(svc, wait, evidenceDir(args), 600)
	report["frameFlow600"] = ff600
	if err != nil {
		return fail("frame flow at 600 dpi", err)
	}
	if err := svc.Shutdown(); err != nil {
		return fail("shutdown", err)
	}
	report["ok"] = true
	return finish(0)
}

// evidenceDir is where the self-test leaves the images it produced: next to the report.
func evidenceDir(args []string) string {
	if len(args) > 0 {
		return filepath.Dir(args[0])
	}
	return ""
}

// selfTestFrameFlow runs frame selection → four photos → composition → two-up
// sheet through the real service, checks sizes and DPI, and (when dir is set)
// saves the strip and the sheet so CI can publish them as evidence.
func selfTestFrameFlow(svc *Service, wait func(capture.State) bool, dir string, wantDPI int) (map[string]any, error) {
	out := map[string]any{"ok": false}
	_ = svc.ResetShooting()
	if err := svc.SetFrameEnabled("rose-pink.strip4", true); err != nil { // decorated designs are off by default
		return out, err
	}
	if err := svc.StartPreview(); err != nil {
		return out, err
	}
	if err := svc.StartShootingWithCount("rose-pink.strip4", "coral", "", 4); err != nil {
		return out, err
	}
	if !wait(capture.Review) {
		return out, fmt.Errorf("review not reached: %s", svc.ShootState().State)
	}
	sn := svc.ShootState()
	if sn.Shots != 4 || len(sn.Taken) != 4 {
		return out, fmt.Errorf("the frame must decide four shots, got %d/%d", len(sn.Taken), sn.Shots)
	}
	if err := svc.AcceptPhotos(); err != nil {
		return out, err
	}
	if !wait(capture.Completed) {
		return out, fmt.Errorf("completion not reached: %s", svc.ShootState().State)
	}
	done := svc.ShootState()
	m, err := svc.store.Media(done.ResultID)
	if err != nil {
		return out, fmt.Errorf("no composed image: %v", err)
	}
	path, _ := svc.store.FilePath(m.ID, "original")
	data, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	img, err := media.Decode(data)
	if err != nil {
		return out, err
	}
	_, dpi, _, ok := media.JPEGDensity(data)
	out["stripPixels"], out["frame"], out["variant"], out["dpi"] = fmt.Sprintf("%dx%d", img.Bounds().Dx(), img.Bounds().Dy()), m.FrameID, m.VariantID, dpi
	wantW, wantH := 2*wantDPI, 6*wantDPI
	if img.Bounds().Dx() != wantW || img.Bounds().Dy() != wantH || !ok || dpi != wantDPI || m.FrameID != "rose-pink.strip4" || m.VariantID != "coral" {
		return out, fmt.Errorf("unexpected composition %v %d dpi %+v", img.Bounds(), dpi, m)
	}
	sess, _ := svc.store.Session(done.SessionID)
	if _, err := os.Stat(filepath.Join(svc.db.Layout.Root, filepath.FromSlash(sess.FrameDir), "frame.json")); err != nil {
		return out, fmt.Errorf("the frame snapshot was not stored: %v", err)
	}
	sheet := templates.TwoUpSheet(img, color.NRGBA{255, 255, 255, 255})
	enc, err := media.EncodeJPEG(sheet, 98)
	if err != nil {
		return out, err
	}
	enc = media.SetJPEGDensity(enc, dpi)
	out["twoUpPixels"] = fmt.Sprintf("%dx%d", sheet.Bounds().Dx(), sheet.Bounds().Dy())
	if sheet.Bounds().Dx() != 4*wantDPI || sheet.Bounds().Dy() != 6*wantDPI {
		return out, fmt.Errorf("two-up sheet is %v", sheet.Bounds())
	}
	if dir != "" {
		_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("selftest-frame-strip-2x6-%ddpi.jpg", wantDPI)), data, 0o644)
		_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("selftest-frame-twoup-4x6-%ddpi.jpg", wantDPI)), enc, 0o644)
	}
	out["ok"] = true
	return out, nil
}
