package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTripAndDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s", "settings.json")
	m, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if g := m.Get(); g.CountdownSeconds != 3 || g.Shots != 4 {
		t.Fatalf("defaults %+v", g)
	}
	s := m.Get()
	s.CountdownSeconds, s.Shots = 5, 2
	if err := m.Save(s); err != nil {
		t.Fatal(err)
	}
	m2, _ := Load(p)
	if g := m2.Get(); g.CountdownSeconds != 5 || g.Shots != 2 {
		t.Fatalf("not persisted: %+v", g)
	}
}

func TestValidation(t *testing.T) {
	m, _ := Load(filepath.Join(t.TempDir(), "s.json"))
	bad := []func(*Settings){
		func(s *Settings) { s.CountdownSeconds = 0 },
		func(s *Settings) { s.Shots = 3 },
		func(s *Settings) { s.MediaDir = "relative/dir" },
		func(s *Settings) { s.Video.FPS = 0 },
	}
	for i, f := range bad {
		s := m.Get()
		f(&s)
		if err := m.Save(s); err == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
}

func TestCorruptFileFallsBackToDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	_ = os.WriteFile(p, []byte("{not json"), 0o644)
	m, err := Load(p)
	if err != nil || m.Get().Shots != 4 {
		t.Fatalf("corrupt file should yield defaults: %v", err)
	}
	if _, err := os.Stat(p + ".corrupt"); err != nil {
		t.Fatal("corrupt file not preserved")
	}
}
