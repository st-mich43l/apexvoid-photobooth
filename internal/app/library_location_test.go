package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/capture"
	"github.com/st-mich43l/apexvoid-photobooth/internal/settings"
)

func startAt(t *testing.T, cfg string, res func(*settings.Manager) settings.Resolution) *Service {
	t.Helper()
	s, err := New(Options{ConfigPath: cfg, ResolveLibrary: res, Tick: 5 * time.Millisecond,
		DisableDriver: map[string]bool{"canon-edsdk": true, "uvc": true}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Shutdown() })
	return s
}

func TestWindowsStyleMigrationEndToEnd(t *testing.T) {
	base := t.TempDir()
	legacy := filepath.Join(base, "Local", "ApexVoidPhotobooth")
	docs := filepath.Join(base, "Tài liệu", "apexvoid-studio")
	cfg := filepath.Join(base, "cfg", "settings.json")

	// 1. an existing user with a library in the old place
	sm, _ := settings.Load(cfg)
	st := sm.Get()
	st.MediaDir, st.EnableMock, st.CountdownSeconds = legacy, true, 1
	_ = sm.Save(st)
	s1 := startAt(t, cfg, nil)
	connectMock(t, s1)
	_ = s1.StartPreview()
	_ = s1.StartShooting(frameFor(1), "", "")
	waitShoot(t, s1, capture.Review)
	_ = s1.AcceptPhotos()
	waitShoot(t, s1, capture.Completed)
	_ = s1.Shutdown()

	// 2. new version starts: default points to Documents, legacy data exists → keeps legacy and offers migration
	st.MediaDir = ""
	_ = sm.Save(st)
	pending := func(*settings.Manager) settings.Resolution {
		return settings.Resolution{Path: legacy, Source: settings.SourceLegacy, DefaultPath: docs, LegacyPath: legacy, MigrationPending: true}
	}
	s2 := startAt(t, cfg, pending)
	ls := s2.LibraryStatus()
	if !ls.MigrationPending || ls.Path != legacy || ls.DefaultPath != docs {
		t.Fatalf("%+v", ls)
	}
	if s2.LibrarySummary().Photos != 1 {
		t.Fatal("existing media must stay visible before migration")
	}
	connectMock(t, s2)
	_ = s2.StartPreview()
	_ = s2.StartShooting(frameFor(1), "", "")
	waitShoot(t, s2, capture.Review)
	if _, err := s2.MigrateLibrary(); err == nil {
		t.Fatal("must refuse to migrate during a capture")
	}
	_ = s2.CancelShooting()
	_ = s2.ResetShooting()

	rep, err := s2.MigrateLibrary()
	if err != nil || !rep.Verified || !rep.RestartRequired || !rep.SourceKept {
		t.Fatalf("%v %+v", err, rep)
	}
	if _, err := os.Stat(filepath.Join(legacy, settings.MigratedMarker)); err != nil {
		t.Fatal("legacy marker missing")
	}
	// writes are blocked until restart so nothing lands in the old copy only
	for name, err := range map[string]error{"shoot": s2.StartShooting(frameFor(1), "", ""), "trash": s2.TrashMedia("x"), "record": s2.StartRecording()} {
		ue, ok := err.(*UserError)
		if !ok || !strings.Contains(ue.Action, "reopen") {
			t.Fatalf("%s should be blocked with a restart instruction, got %v", name, err)
		}
	}
	_ = s2.Shutdown()

	// 3. after restart the real resolution picks the new library, which has the media
	done := func(*settings.Manager) settings.Resolution {
		return settings.Resolution{Path: docs, Source: settings.SourceDocuments, DefaultPath: docs, LegacyPath: legacy}
	}
	s3 := startAt(t, cfg, done)
	if s3.LibrarySummary().Photos < 1 || s3.LibraryStatus().Path != docs {
		t.Fatalf("%+v %+v", s3.LibrarySummary(), s3.LibraryStatus())
	}
	ms, _ := s3.ListMedia("", 10, 0)
	for _, m := range ms {
		p, err := s3.store.FilePath(m.ID, "original")
		if err != nil || !strings.HasPrefix(p, docs) {
			t.Fatalf("%s %v", p, err)
		}
	}
}

func TestKeepLegacyLibraryBecomesCustom(t *testing.T) {
	base := t.TempDir()
	legacy := filepath.Join(base, "old")
	cfg := filepath.Join(base, "s.json")
	s := startAt(t, cfg, func(*settings.Manager) settings.Resolution {
		return settings.Resolution{Path: legacy, Source: settings.SourceLegacy, DefaultPath: filepath.Join(base, "new"), LegacyPath: legacy, MigrationPending: true}
	})
	if err := s.KeepLegacyLibrary(); err != nil {
		t.Fatal(err)
	}
	if s.settings.Get().MediaDir != legacy || s.LibraryStatus().MigrationPending {
		t.Fatal("keeping the old library must persist it as an explicit choice")
	}
}

func TestFallbackWarningReachesTheUI(t *testing.T) {
	base := t.TempDir()
	s := startAt(t, filepath.Join(base, "s.json"), func(*settings.Manager) settings.Resolution {
		return settings.Resolution{Path: filepath.Join(base, "fb"), Source: settings.SourceFallback, Warning: "The Documents folder is unavailable (x)."}
	})
	if b := s.Bootstrap(); !strings.Contains(b.Location.Warning, "Documents folder is unavailable") || b.Location.Source != "fallback" {
		t.Fatalf("%+v", b.Location)
	}
}

func TestKioskPINLifecycleAndLockout(t *testing.T) {
	s, _, _ := newTestApp(t)
	if s.GetSettings().KioskPinSet {
		t.Fatal("PIN must be off by default")
	}
	if err := s.VerifyKioskPin(""); err != nil {
		t.Fatal("with protection off any exit is allowed")
	}
	if err := s.SetKioskPin("", "12"); err == nil {
		t.Fatal("short PIN accepted")
	}
	if err := s.SetKioskPin("", "2468"); err != nil {
		t.Fatal(err)
	}
	pub := s.GetSettings()
	if !pub.KioskPinSet || pub.KioskPin != "" || s.Bootstrap().Settings.KioskPin != "" {
		t.Fatal("the PIN record must never reach the UI")
	}
	// saving settings from the UI must not wipe or replace the PIN
	n := pub
	n.KioskPin, n.CountdownSeconds = "forged", 4
	if _, err := s.SaveSettings(n); err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyKioskPin("2468"); err != nil {
		t.Fatalf("PIN lost after SaveSettings: %v", err)
	}
	if err := s.SetKioskPin("0000", ""); err == nil {
		t.Fatal("clearing without the current PIN accepted")
	}
	// one failure was already counted by the refused SetKioskPin("0000", "") above
	for i := 0; i < 3; i++ {
		if err := s.VerifyKioskPin("1111"); err == nil || !strings.Contains(err.Error(), "Wrong PIN") {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if err := s.VerifyKioskPin("1111"); err == nil || !strings.Contains(err.Error(), "Locked") {
		t.Fatalf("5th failure must lock: %v", err)
	}
	if err := s.VerifyKioskPin("2468"); err == nil || !strings.Contains(err.Error(), "Too many") {
		t.Fatalf("even the right PIN is refused while locked: %v", err)
	}
	s.pinMu.Lock()
	s.pinLockedUntil = time.Time{}
	s.pinMu.Unlock()
	if err := s.SetKioskPin("2468", ""); err != nil {
		t.Fatal(err)
	}
	if s.GetSettings().KioskPinSet {
		t.Fatal("PIN not cleared")
	}
}

func TestStructuredPINVerificationPreservesLockout(t *testing.T) {
	s, _, _ := newTestApp(t)
	if err := s.SetKioskPin("", "2468"); err != nil {
		t.Fatal(err)
	}
	if result := s.VerifyKioskPinDetailed("0000"); result.OK || result.Code != "pin_wrong" {
		t.Fatalf("wrong PIN: %+v", result)
	}
	for i := 0; i < 3; i++ {
		s.VerifyKioskPinDetailed("0000")
	}
	if result := s.VerifyKioskPinDetailed("0000"); result.OK || result.Code != "pin_locked" || result.RetrySeconds != 30 {
		t.Fatalf("lockout: %+v", result)
	}
	if result := s.VerifyKioskPinDetailed("2468"); result.OK || result.Code != "pin_locked" || result.RetrySeconds < 1 {
		t.Fatalf("locked correct PIN: %+v", result)
	}
}

func TestStructuredPINSettingRetainsLegacyBehavior(t *testing.T) {
	s, _, _ := newTestApp(t)
	if r := s.SetKioskPinDetailed("", "12"); r.OK || r.Code != "pin_setting_failed" || r.Diagnostic == "" {
		t.Fatalf("invalid PIN: %+v", r)
	}
	if r := s.SetKioskPinDetailed("", "2468"); !r.OK {
		t.Fatalf("set PIN: %+v", r)
	}
	if r := s.SetKioskPinDetailed("0000", "1357"); r.OK || r.Code != "pin_wrong" {
		t.Fatalf("wrong current PIN: %+v", r)
	}
	if err := s.VerifyKioskPin("2468"); err != nil {
		t.Fatalf("old PIN was replaced: %v", err)
	}
}
