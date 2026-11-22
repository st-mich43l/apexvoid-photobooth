package app

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
	"github.com/st-mich43l/apexvoid-photobooth/internal/camera/edsdk"
	"github.com/st-mich43l/apexvoid-photobooth/internal/capture"
	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
	"github.com/st-mich43l/apexvoid-photobooth/internal/settings"
)

// Validation outcomes. UNVERIFIED means the step was not run (or needs a human
// who was not there): it is never reported as passed.
const (
	ValPass       = "PASS"
	ValFail       = "FAIL"
	ValUnverified = "UNVERIFIED"
)

// ValStep is one acceptance scenario of docs/HARDWARE_TESTS.md (Canon section).
type ValStep struct {
	N        int    `json:"n"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Action   string `json:"action,omitempty"`
	Evidence string `json:"evidence,omitempty"` // file to look at
	Review   string `json:"review,omitempty"`   // what a human must still confirm
	Ms       int64  `json:"ms,omitempty"`
}

// CanonValidationReport is written by `--canon-validate`.
type CanonValidationReport struct {
	Version   string              `json:"version"`
	OS        string              `json:"os"`
	When      string              `json:"when"`
	OutputDir string              `json:"outputDir"`
	SDK       edsdk.SDKReport     `json:"sdk"`
	Detected  []camera.DeviceInfo `json:"detected"`
	Conflicts []string            `json:"conflicts"`
	Steps     []ValStep           `json:"steps"`
	Summary   map[string]int      `json:"summary"`
	Notes     []string            `json:"notes"`
}

// CanonValidate runs the Canon EOS R50 acceptance scenarios on a real machine:
//
//	photobooth.exe --canon-validate [report.json] [--interactive] [--sessions N]
//
// It uses the real settings (SDK folder) but a throw-away library, so the
// operator's media is never touched or deleted. Without --interactive the steps
// that need a person (unplug the cable, start EOS Utility) are UNVERIFIED.
func CanonValidate(version string, args []string) int {
	reportPath, interactive, sessionsN := "canon-validation.json", false, 3
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--interactive":
			interactive = true
		case "--sessions":
			if i+1 < len(args) {
				fmt.Sscan(args[i+1], &sessionsN)
				i++
			}
		default:
			reportPath = args[i]
		}
	}
	stamp := time.Now().Format("20060102-150405")
	outDir := filepath.Join(filepath.Dir(reportPath), "canon-"+stamp)
	_ = os.MkdirAll(outDir, 0o755)
	tmp, err := os.MkdirTemp("", "apexvoid-canon-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	defer os.RemoveAll(tmp)
	svc, err := New(Options{Version: version, ConfigPath: settings.DefaultPath(), SkipRestore: true, Tick: 300 * time.Millisecond,
		ResolveLibrary: func(*settings.Manager) settings.Resolution {
			return settings.Resolution{Path: filepath.Join(tmp, "lib"), Source: settings.SourceCustom}
		}})
	if err != nil {
		fmt.Fprintln(os.Stderr, "start:", err)
		return 2
	}
	v := &canonValidator{svc: svc, out: outDir, interactive: interactive, in: bufio.NewReader(os.Stdin), needSDK: true, minLong: 1920,
		discover: svc.edsdk.Discover}
	rep := v.run(version, sessionsN)
	_ = svc.Shutdown()
	b, _ := json.MarshalIndent(rep, "", "  ")
	if err := os.WriteFile(reportPath, b, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	for _, s := range rep.Steps {
		fmt.Printf("%2d  %-10s %s", s.N, s.Status, s.Name)
		if s.Detail != "" {
			fmt.Printf(" — %s", s.Detail)
		}
		fmt.Println()
	}
	fmt.Printf("\nreport: %s\nevidence: %s\nPASS %d · FAIL %d · UNVERIFIED %d\n", reportPath, outDir, rep.Summary[ValPass], rep.Summary[ValFail], rep.Summary[ValUnverified])
	if rep.Summary[ValFail] > 0 {
		return 1
	}
	return 0
}

type canonValidator struct {
	svc         *Service
	out         string
	interactive bool
	in          *bufio.Reader
	rep         CanonValidationReport
	dev         camera.DeviceInfo
	photos      [][]byte
	connected   bool

	// seams for tests: the production values talk to the Canon driver
	discover func(ctx context.Context) ([]camera.DeviceInfo, error)
	needSDK  bool
	minLong  int // shortest long edge accepted as a full-resolution original
}

func (v *canonValidator) step(n int, name string, f func(*ValStep)) {
	s := ValStep{N: n, Name: name, Status: ValUnverified}
	t0 := time.Now()
	f(&s)
	s.Ms = time.Since(t0).Milliseconds()
	v.rep.Steps = append(v.rep.Steps, s)
}

func (s *ValStep) pass(detail string)             { s.Status, s.Detail = ValPass, detail }
func (s *ValStep) fail(detail, action string)     { s.Status, s.Detail, s.Action = ValFail, detail, action }
func (s *ValStep) skip(reason string)             { s.Status, s.Detail = ValUnverified, reason }
func (s *ValStep) failErr(prefix string, e error) { s.fail(prefix+": "+e.Error(), camera.ActionOf(e)) }

func (v *canonValidator) ask(prompt string) string {
	fmt.Print(prompt + " ")
	line, _ := v.in.ReadString('\n')
	return strings.TrimSpace(line)
}

func (v *canonValidator) wait(want capture.State, d time.Duration) bool {
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		if v.svc.ShootState().State == want {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func (v *canonValidator) run(version string, sessionsN int) CanonValidationReport {
	s := v.svc
	v.rep = CanonValidationReport{Version: version, OS: runtime.GOOS + "/" + runtime.GOARCH, When: time.Now().Format(time.RFC3339), OutputDir: v.out,
		SDK: edsdk.SDKStatus(s.settings.Get().CanonSDKDir), Conflicts: edsdk.Conflicts(), Detected: []camera.DeviceInfo{}}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	handlesBefore := processHandles()

	// 1. detected
	v.step(1, "Camera detected", func(st *ValStep) {
		if v.needSDK && !v.rep.SDK.OK() {
			st.fail(v.rep.SDK.Problem, v.rep.SDK.Action)
			return
		}
		if len(v.rep.Conflicts) > 0 {
			v.rep.Notes = append(v.rep.Notes, "programs that may own the camera are running: "+strings.Join(v.rep.Conflicts, ", "))
		}
		dctx, c := context.WithTimeout(ctx, 15*time.Second)
		devs, err := v.discover(dctx)
		c()
		if err != nil {
			st.failErr("discovery", err)
			return
		}
		v.rep.Detected = devs
		if len(devs) == 0 {
			st.fail("no Canon camera found", "Switch the camera on, check the USB cable and set the USB connection app to Photo Import / Remote Control.")
			return
		}
		v.dev = devs[0]
		st.pass(fmt.Sprintf("%s (%s, %s)", v.dev.Name, v.dev.Interface, v.dev.ID))
		if len(devs) > 1 {
			v.rep.Notes = append(v.rep.Notes, fmt.Sprintf("%d Canon bodies attached; the first one is used", len(devs)))
		}
	})

	// 2. connected
	v.step(2, "Camera connected", func(st *ValStep) {
		if v.dev.ID == "" {
			st.skip("no camera detected")
			return
		}
		if err := s.mgr.Connect(ctx, v.dev); err != nil {
			st.failErr("connect", err)
			return
		}
		v.connected = true
		st.pass("session open, photos sent to the PC")
	})

	// 3. live view
	v.step(3, "Live view displays the physical camera feed", func(st *ValStep) {
		if !v.connected {
			st.skip("camera not connected")
			return
		}
		if err := s.mgr.StartLiveView(ctx); err != nil {
			st.failErr("start live view", err)
			return
		}
		var seq uint64
		frames := 0
		t0 := time.Now()
		for time.Since(t0) < 5*time.Second {
			fctx, c := context.WithTimeout(ctx, 4*time.Second)
			f, err := s.mgr.WaitFrame(fctx, seq)
			c()
			if err != nil {
				break
			}
			seq = f.Seq
			frames++
			if frames == 1 || frames == 40 {
				p := filepath.Join(v.out, fmt.Sprintf("liveview-%d.jpg", frames))
				_ = os.WriteFile(p, f.JPEG, 0o644)
				st.Evidence = p
			}
		}
		if frames == 0 {
			st.fail("no frames arrived", "Check the camera is in a still-photo mode with the lens cap off.")
			return
		}
		fps := float64(frames) / time.Since(t0).Seconds()
		st.pass(fmt.Sprintf("%d frames in %.1fs (%.1f fps)", frames, time.Since(t0).Seconds(), fps))
		st.Review = "Open the evidence image and confirm it shows what the camera sees (not a placeholder)."
	})

	// 4-6. shutter, transfer, EXIF
	var first camera.Photo
	v.step(4, "Shutter command captures an actual photograph", func(st *ValStep) {
		if !v.connected {
			st.skip("camera not connected")
			return
		}
		p, err := s.mgr.Capture(ctx)
		if err != nil {
			st.failErr("capture", err)
			return
		}
		first = p
		v.photos = append(v.photos, p.Data)
		st.pass(fmt.Sprintf("%s, %d bytes", p.Filename, len(p.Data)))
		st.Review = "Confirm a real exposure was made (shutter sound / the image shows the scene)."
	})
	v.step(5, "Full-resolution JPEG transfers successfully", func(st *ValStep) {
		if len(first.Data) == 0 {
			st.skip("no photo was captured")
			return
		}
		w, h, err := media.Dimensions(first.Data)
		if err != nil {
			st.fail("not a decodable image: "+err.Error(), "Set image quality to JPEG or RAW+JPEG.")
			return
		}
		p := filepath.Join(v.out, "photo-1-"+first.Filename)
		if err := os.WriteFile(p, first.Data, 0o644); err != nil {
			st.fail(err.Error(), "")
			return
		}
		st.Evidence = p
		long := max(w, h)
		if long < v.minLong {
			st.fail(fmt.Sprintf("%d×%d is live-view size, not a full-resolution original", w, h), "Set the camera's JPEG size to L and check the photo is saved to the PC.")
			return
		}
		sum := sha256.Sum256(first.Data)
		onDisk, _ := os.ReadFile(p)
		same := bytes.Equal(onDisk, first.Data)
		st.pass(fmt.Sprintf("%d×%d, %d bytes, sha256 %s…, written unchanged=%v", w, h, len(first.Data), hex.EncodeToString(sum[:4]), same))
		if !same {
			st.fail("the saved file differs from the received bytes", "")
		}
		st.Review = "Compare the pixel size with the camera's JPEG size setting (R50 L = 6000×4000)."
	})
	v.step(6, "EXIF metadata is preserved", func(st *ValStep) {
		if len(first.Data) == 0 {
			st.skip("no photo was captured")
			return
		}
		mk := exifCanon(first.Data)
		if mk == "" {
			st.fail("no Canon EXIF block found in the original", "Photos must be the camera's own JPEG, not a re-encoded copy.")
			return
		}
		st.pass("EXIF present: " + mk)
	})

	// 7-9. the real session flow: four photos, a retake, a framed strip
	var taken []capture.Shot
	v.step(7, "Four-photo session completes", func(st *ValStep) {
		if !v.connected {
			st.skip("camera not connected")
			return
		}
		if err := s.StartPreview(); err != nil {
			st.failErr("preview", err)
			return
		}
		if err := s.StartShooting("plain.strip4", "", ""); err != nil {
			st.failErr("start shooting", err)
			return
		}
		if !v.wait(capture.Review, 4*time.Minute) {
			ss := s.ShootState()
			st.fail(fmt.Sprintf("stopped in %s: %s", ss.State, ss.Error), ss.ErrorAction)
			return
		}
		taken = append([]capture.Shot(nil), s.ShootState().Taken...)
		if len(taken) != 4 {
			st.fail(fmt.Sprintf("%d photos instead of 4", len(taken)), "")
			return
		}
		seen := map[string]bool{}
		for _, t := range taken {
			m, err := s.GetMedia(t.MediaID)
			if err != nil || m.SHA256 == "" || seen[m.SHA256] {
				st.fail("a photo is missing or repeated (late transfer attached to the wrong shot?)", "")
				return
			}
			seen[m.SHA256] = true
		}
		st.pass("4 distinct originals saved")
	})
	v.step(8, "One photo can be retaken", func(st *ValStep) {
		if len(taken) != 4 {
			st.skip("the four-photo session did not complete")
			return
		}
		if err := s.RetakePhoto(1); err != nil {
			st.failErr("retake", err)
			return
		}
		time.Sleep(200 * time.Millisecond)
		if !v.wait(capture.Review, 2*time.Minute) {
			st.fail("the retake did not finish: "+s.ShootState().Error, s.ShootState().ErrorAction)
			return
		}
		now := s.ShootState().Taken
		if len(now) != 4 {
			st.fail("the session lost photos", "")
			return
		}
		bySlot := map[int]string{}
		for _, t := range now {
			bySlot[t.Slot] = t.MediaID
		}
		for _, t := range taken {
			if t.Slot == 1 && bySlot[1] == t.MediaID {
				st.fail("the retaken slot still holds the old photo", "")
				return
			}
			if t.Slot != 1 && bySlot[t.Slot] != t.MediaID {
				st.fail(fmt.Sprintf("slot %d changed although only slot 1 was retaken", t.Slot+1), "")
				return
			}
		}
		st.pass("only slot 2 was replaced")
	})
	v.step(9, "Photo strip is produced using the selected frame", func(st *ValStep) {
		if len(taken) != 4 || s.ShootState().State != capture.Review {
			st.skip("no reviewable session")
			return
		}
		if err := s.AcceptPhotos(); err != nil {
			st.failErr("accept", err)
			return
		}
		if !v.wait(capture.Completed, time.Minute) {
			st.fail("saving did not finish: "+s.ShootState().Error, "")
			return
		}
		res := s.ShootState().ResultID
		if res == "" {
			st.fail("the session completed without a strip", "")
			return
		}
		m, err := s.GetMedia(res)
		if err != nil {
			st.failErr("strip", err)
			return
		}
		p := filepath.Join(s.db.Layout.Root, filepath.FromSlash(m.RelPath))
		if b, err := os.ReadFile(p); err == nil {
			out := filepath.Join(v.out, "strip.jpg")
			_ = os.WriteFile(out, b, 0o644)
			st.Evidence = out
		}
		st.pass(fmt.Sprintf("%d×%d strip with frame plain.strip4", m.Width, m.Height))
		st.Review = "Open the strip and check the four photos sit in the frame."
		_ = s.ResetShooting()
	})

	// 10. USB loss and recovery
	v.step(10, "USB disconnect and reconnect recover correctly", func(st *ValStep) {
		if !v.connected {
			st.skip("camera not connected")
			return
		}
		if !v.interactive {
			st.skip("needs a person to unplug the cable: run with --interactive")
			return
		}
		_ = s.StartPreview()
		v.ask("Unplug the camera's USB cable now, wait 3 seconds, then press Enter.")
		d0 := time.Now().Add(20 * time.Second)
		for time.Now().Before(d0) && !s.mgr.Status().Reconnecting {
			time.Sleep(100 * time.Millisecond)
		}
		if !s.mgr.Status().Reconnecting {
			st.fail("the application did not notice the unplugged camera", "")
			return
		}
		v.ask("Plug the cable back in, then press Enter.")
		d1 := time.Now().Add(60 * time.Second)
		for time.Now().Before(d1) {
			if cs := s.mgr.Status(); !cs.Reconnecting && cs.Device != nil {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if cs := s.mgr.Status(); cs.Reconnecting || cs.Device == nil {
			st.fail("the camera did not reconnect within 60 s", "Check the cable; unplug and replug the camera.")
			return
		}
		if _, err := s.mgr.Capture(ctx); err != nil {
			st.failErr("capture after reconnect", err)
			return
		}
		st.pass("loss detected, reconnected automatically, shutter works again")
	})

	// 12. repeated sessions: no leaks, camera never left busy
	v.step(12, fmt.Sprintf("%d repeated sessions leave no leak and the camera free", sessionsN), func(st *ValStep) {
		if !v.connected {
			st.skip("camera not connected")
			return
		}
		goBefore := runtime.NumGoroutine()
		for i := 0; i < sessionsN; i++ {
			_ = s.ResetShooting()
			if err := s.StartPreview(); err != nil {
				st.failErr(fmt.Sprintf("session %d preview", i+1), err)
				return
			}
			if err := s.StartShooting("plain.single", "", ""); err != nil {
				st.failErr(fmt.Sprintf("session %d", i+1), err)
				return
			}
			if !v.wait(capture.Review, 2*time.Minute) {
				st.fail(fmt.Sprintf("session %d stopped in %s: %s", i+1, s.ShootState().State, s.ShootState().Error), s.ShootState().ErrorAction)
				return
			}
			_ = s.AcceptPhotos()
			v.wait(capture.Completed, 30*time.Second)
		}
		_ = s.ResetShooting()
		time.Sleep(time.Second)
		growth := runtime.NumGoroutine() - goBefore
		hb := processHandles()
		detail := fmt.Sprintf("goroutines %+d", growth)
		if hb > 0 {
			detail += fmt.Sprintf(", OS handles %d → %d", handlesBefore, hb)
		}
		if growth > 8 || (hb > 0 && handlesBefore > 0 && hb-handlesBefore > 60) {
			st.fail("resources grew across sessions ("+detail+")", "Send this report to the developer.")
			return
		}
		st.pass(detail)
	})

	// 11. release the camera for other programs (last: it closes the SDK)
	v.step(11, "Closing ApexVoid releases the camera for EOS Utility", func(st *ValStep) {
		if !v.connected {
			st.skip("camera not connected")
			return
		}
		t0 := time.Now()
		if err := s.mgr.Shutdown(); err != nil {
			st.failErr("release camera", err)
			return
		}
		if err := s.edsdk.Close(); err != nil {
			st.fail("the SDK did not shut down: "+err.Error(), "Close and reopen ApexVoid; replug the camera.")
			return
		}
		detail := fmt.Sprintf("session closed and SDK terminated in %d ms", time.Since(t0).Milliseconds())
		if !v.interactive {
			st.skip(detail + "; whether EOS Utility can now use the camera needs a person: run with --interactive")
			return
		}
		a := strings.ToLower(v.ask("Start EOS Utility now. Does it detect the camera and open its remote shooting window? (y/n)"))
		if strings.HasPrefix(a, "y") {
			st.pass(detail + "; EOS Utility took the camera")
		} else {
			st.fail(detail+"; EOS Utility could not use the camera", "Some handle was kept open: send this report to the developer.")
		}
	})

	v.rep.Notes = append(v.rep.Notes,
		"PASS means the automated criterion held on this machine with this camera. Steps with a 'review' still need a person to look at the evidence.",
		"UNVERIFIED means the step was not run or needs a person: it is not a pass.",
		"This run used a throw-away library; no media of the operator was read, changed or deleted.")
	sort.Slice(v.rep.Steps, func(i, j int) bool { return v.rep.Steps[i].N < v.rep.Steps[j].N })
	v.rep.Summary = map[string]int{ValPass: 0, ValFail: 0, ValUnverified: 0}
	for _, st := range v.rep.Steps {
		v.rep.Summary[st.Status]++
	}
	return v.rep
}

// exifCanon returns the camera make/model found in the JPEG's EXIF block ("" when there is none).
func exifCanon(data []byte) string {
	head := data
	if len(head) > 1<<17 {
		head = head[:1<<17]
	}
	i := bytes.Index(head, []byte("Exif\x00\x00"))
	if i < 0 || i > 4096 {
		return ""
	}
	seg := head[i:]
	j := bytes.Index(seg, []byte("Canon"))
	if j < 0 {
		return ""
	}
	end := j
	for end < len(seg) && seg[end] >= 32 && seg[end] < 127 && end-j < 48 {
		end++
	}
	return strings.TrimSpace(string(seg[j:end]))
}
