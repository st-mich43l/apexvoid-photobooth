package integration_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/st-mich43l/apexvoid-photobooth/internal/app"
)

func TestProbeReportsStepsHonestly(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "report.json")
	if code := app.ProbeCameras("test", []string{out, "--include-mock"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	var rep app.ProbeReport
	b, _ := os.ReadFile(out)
	if err := json.Unmarshal(b, &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Devices) != 1 {
		t.Fatalf("%s", b)
	}
	d := rep.Devices[0]
	if !d.Connected || !d.LiveView.OK || !d.Still.OK || !d.Released || d.LiveView.FPS < 2 {
		t.Fatalf("%+v", d)
	}
	if d.Video.Attempted {
		t.Fatal("the simulated camera cannot record: video must be reported as not attempted (unverified), not passed")
	}
	if _, err := os.Stat(d.Still.File); err != nil {
		t.Fatal("captured photo missing")
	}
}

func TestProbeWithNoCameraSaysEverythingIsUnverified(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	out := filepath.Join(t.TempDir(), "r.json")
	if code := app.ProbeCameras("test", []string{out}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	var rep app.ProbeReport
	b, _ := os.ReadFile(out)
	_ = json.Unmarshal(b, &rep)
	found := false
	for _, n := range rep.Notes {
		if len(n) > 0 {
			found = true
		}
	}
	if len(rep.Devices) != 0 || !found {
		t.Fatalf("%s", b)
	}
}
