package app

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/camera"
	"github.com/st-mich43l/apexvoid-photobooth/internal/settings"
)

func TestCanonValidateWithoutHardwareNeverReportsAPass(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir) // the real settings and library are never touched
	t.Setenv("APPDATA", cfgDir)
	t.Setenv("HOME", cfgDir)
	report := filepath.Join(t.TempDir(), "canon.json")
	code := CanonValidate("test", []string{report})
	if code != 1 {
		t.Fatalf("a run that could not find the SDK/camera must fail, got exit code %d", code)
	}
	b, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	var rep CanonValidationReport
	if err := json.Unmarshal(b, &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Steps) != 12 {
		t.Fatalf("all 12 acceptance scenarios must be listed, got %d", len(rep.Steps))
	}
	nums := map[int]bool{}
	for _, s := range rep.Steps {
		nums[s.N] = true
		if s.Status == ValPass {
			t.Fatalf("step %d (%s) passed without any camera", s.N, s.Name)
		}
		if s.N > 1 && s.Status != ValUnverified {
			t.Fatalf("step %d should be UNVERIFIED when no camera is detected: %+v", s.N, s)
		}
	}
	for n := 1; n <= 12; n++ {
		if !nums[n] {
			t.Fatalf("scenario %d missing", n)
		}
	}
	first := rep.Steps[0]
	if first.Status != ValFail || first.Action == "" {
		t.Fatalf("detection must fail with an action: %+v", first)
	}
	if rep.Summary[ValPass] != 0 || rep.Summary[ValFail] != 1 || rep.Summary[ValUnverified] != 11 {
		t.Fatalf("summary %v", rep.Summary)
	}
	if len(rep.Notes) == 0 {
		t.Fatal("the report must explain PASS / UNVERIFIED")
	}
}

func TestExifCanonFindsTheCameraModelInTheOriginalBytes(t *testing.T) {
	jpg := append([]byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x40}, []byte("Exif\x00\x00II*\x00\x08\x00\x00\x00....Canon\x00Canon EOS R50\x00....")...)
	if got := exifCanon(jpg); got != "Canon" {
		t.Fatalf("%q", got) // stops at the first NUL: the make
	}
	if exifCanon([]byte{0xFF, 0xD8, 0xFF, 0xD9}) != "" {
		t.Fatal("no EXIF must report nothing")
	}
	if exifCanon(append([]byte{0xFF, 0xD8}, []byte("Exif\x00\x00 Nikon D850")...)) != "" {
		t.Fatal("a non-Canon block must not be reported as Canon")
	}
}

// The acceptance script itself is exercised against the simulated camera so its
// logic (session, retake, strip, leak check) is known to work before it is run
// on real hardware. This says nothing about a real EOS R50.
func TestValidationScriptLogicAgainstTheSimulatedCamera(t *testing.T) {
	root := t.TempDir()
	cfg := filepath.Join(root, "settings.json")
	sm, _ := settings.Load(cfg)
	st := sm.Get()
	st.EnableMock, st.CountdownSeconds = true, 1
	if err := sm.Save(st); err != nil {
		t.Fatal(err)
	}
	svc, err := New(Options{Version: "test", ConfigPath: cfg, SkipRestore: true, Tick: 5 * time.Millisecond,
		DisableDriver: map[string]bool{"canon-edsdk": true, "uvc": true},
		ResolveLibrary: func(*settings.Manager) settings.Resolution {
			return settings.Resolution{Path: filepath.Join(root, "lib"), Source: settings.SourceCustom}
		}})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Shutdown()
	v := &canonValidator{svc: svc, out: t.TempDir(), in: bufio.NewReader(strings.NewReader("")), minLong: 0,
		discover: func(ctx context.Context) ([]camera.DeviceInfo, error) {
			devs, err := svc.mgr.Discover(ctx)
			var sim []camera.DeviceInfo
			for _, d := range devs {
				if d.Simulated {
					sim = append(sim, d)
				}
			}
			return sim, err
		}}
	rep := v.run("test", 2)
	byN := map[int]ValStep{}
	for _, s := range rep.Steps {
		byN[s.N] = s
	}
	for _, n := range []int{1, 2, 3, 4, 5, 7, 8, 9, 12} {
		if byN[n].Status != ValPass {
			t.Errorf("step %d %q: %s %s %s", n, byN[n].Name, byN[n].Status, byN[n].Detail, byN[n].Action)
		}
	}
	if byN[6].Status != ValFail { // the simulated camera writes no Canon EXIF: the check must notice
		t.Errorf("EXIF step should fail for a camera without Canon EXIF: %+v", byN[6])
	}
	for _, n := range []int{10, 11} { // need a person
		if byN[n].Status != ValUnverified {
			t.Errorf("step %d must be UNVERIFIED without --interactive: %+v", n, byN[n])
		}
	}
	for i := 1; i < len(rep.Steps); i++ {
		if rep.Steps[i].N < rep.Steps[i-1].N {
			t.Fatal("steps must be listed in order")
		}
	}
}
