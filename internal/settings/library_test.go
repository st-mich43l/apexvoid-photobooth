package settings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mkLibrary(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "database"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "database", "photobooth.db"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

type fixture struct {
	docs, local string
	e           env
	m           *Manager
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	// non-ASCII user name and a redirected (OneDrive-style) Documents folder
	f := &fixture{docs: filepath.Join(root, "OneDrive – Công ty", "Tài liệu"), local: filepath.Join(root, "Users", "Nguyễn Văn", "AppData", "Local")}
	_ = os.MkdirAll(f.docs, 0o755)
	_ = os.MkdirAll(f.local, 0o755)
	f.e = env{goos: "windows", documents: func() (string, error) { return f.docs, nil }, localAppData: func() string { return f.local },
		dataDirOther: func() string { return "/unused" }}
	f.m, _ = Load(filepath.Join(root, "settings.json"))
	return f
}

func TestFreshWindowsInstallUsesDocumentsLibrary(t *testing.T) {
	f := newFixture(t)
	r := f.m.resolve(f.e)
	want := filepath.Join(f.docs, "apexvoid-studio")
	if r.Path != want || r.Source != SourceDocuments || r.MigrationPending || r.Warning != "" {
		t.Fatalf("%+v", r)
	}
}

func TestLegacyLibraryIsKeptAndMigrationOffered(t *testing.T) {
	f := newFixture(t)
	legacy := filepath.Join(f.local, "ApexVoidPhotobooth")
	mkLibrary(t, legacy)
	r := f.m.resolve(f.e)
	if r.Path != legacy || r.Source != SourceLegacy || !r.MigrationPending || r.DefaultPath != filepath.Join(f.docs, "apexvoid-studio") {
		t.Fatalf("existing user data must stay in use until migrated: %+v", r)
	}
}

func TestNewLibraryWinsOverLegacyWhenBothHaveData(t *testing.T) {
	f := newFixture(t)
	mkLibrary(t, filepath.Join(f.local, "ApexVoidPhotobooth"))
	mkLibrary(t, filepath.Join(f.docs, "apexvoid-studio"))
	r := f.m.resolve(f.e)
	if r.Source != SourceDocuments || r.MigrationPending {
		t.Fatalf("must not auto-migrate over existing data: %+v", r)
	}
}

func TestMigratedLegacyIsNotOfferedAgain(t *testing.T) {
	f := newFixture(t)
	legacy := filepath.Join(f.local, "ApexVoidPhotobooth")
	mkLibrary(t, legacy)
	_ = os.WriteFile(filepath.Join(legacy, MigratedMarker), []byte("done"), 0o644)
	if r := f.m.resolve(f.e); r.MigrationPending || r.Source != SourceDocuments {
		t.Fatalf("%+v", r)
	}
}

func TestCustomMediaDirIsNeverTouched(t *testing.T) {
	f := newFixture(t)
	mkLibrary(t, filepath.Join(f.local, "ApexVoidPhotobooth"))
	s := f.m.Get()
	s.MediaDir = filepath.Join(t.TempDir(), "my library")
	if err := f.m.Save(s); err != nil {
		t.Fatal(err)
	}
	r := f.m.resolve(f.e)
	if r.Path != s.MediaDir || r.Source != SourceCustom || r.MigrationPending {
		t.Fatalf("%+v", r)
	}
	if _, err := os.Stat(filepath.Join(f.docs, "apexvoid-studio")); err == nil {
		t.Fatal("a custom library must not create the Documents folder")
	}
}

func TestDocumentsUnavailableFallsBackWithClearWarning(t *testing.T) {
	f := newFixture(t)
	f.e.documents = func() (string, error) { return "", errors.New("known folder not found") }
	r := f.m.resolve(f.e)
	if r.Source != SourceFallback || !strings.Contains(r.Warning, "Documents folder is unavailable") || r.Path != filepath.Join(f.local, "ApexVoidPhotobooth") {
		t.Fatalf("%+v", r)
	}
}

func TestResolutionDoesNotWriteBeforeLibraryOwnership(t *testing.T) {
	f := newFixture(t)
	// "Documents" is a file, so apexvoid-studio cannot be created below it
	file := filepath.Join(t.TempDir(), "Documents")
	_ = os.WriteFile(file, []byte("x"), 0o644)
	f.e.documents = func() (string, error) { return file, nil }
	r := f.m.resolve(f.e)
	if r.Source != SourceDocuments || r.Path != filepath.Join(file, LibraryFolderName) {
		t.Fatalf("resolution should be read-only and defer access checks: %+v", r)
	}
	if _, err := os.Stat(r.Path); !os.IsNotExist(err) {
		t.Fatalf("resolution touched the library before ownership: %v", err)
	}
	fallback := FallbackLibrary(r, errors.New("not a directory"))
	if fallback.Source != SourceFallback || fallback.Warning == "" || fallback.Path != filepath.Join(f.local, AppDirName) {
		t.Fatalf("fallback resolution: %+v", fallback)
	}
}

func TestNonWindowsKeepsPlatformDefault(t *testing.T) {
	f := newFixture(t)
	f.e.goos = "linux"
	f.e.dataDirOther = func() string { return "/home/u/.local/share/ApexVoidPhotobooth" }
	if r := f.m.resolve(f.e); r.Path != "/home/u/.local/share/ApexVoidPhotobooth" || r.Source != SourceDefault {
		t.Fatalf("%+v", r)
	}
}

func TestPINHashing(t *testing.T) {
	rec, err := HashPIN("4821")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rec, "4821") || !CheckPIN(rec, "4821") || CheckPIN(rec, "4822") || CheckPIN("garbage", "4821") {
		t.Fatal("PIN verification broken")
	}
	rec2, _ := HashPIN("4821")
	if rec == rec2 {
		t.Fatal("salt must differ between hashes")
	}
	for _, bad := range []string{"", "123", "123456789", "12a4", "١٢٣٤"} {
		if ValidatePIN(bad) == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}
