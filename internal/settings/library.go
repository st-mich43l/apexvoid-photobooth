package settings

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// LibraryFolderName is the Windows default library folder inside Documents.
const LibraryFolderName = "apexvoid-studio"

// MigratedMarker is written into a legacy library after a successful
// migration so it is not offered again. The legacy files are never deleted.
const MigratedMarker = "MIGRATED_TO.txt"

// Where the active library came from.
const (
	SourceCustom    = "custom"    // explicitly configured folder (never touched)
	SourceDocuments = "documents" // <Documents>\apexvoid-studio
	SourceLegacy    = "legacy"    // %LOCALAPPDATA%\ApexVoidPhotobooth, awaiting migration
	SourceFallback  = "fallback"  // Documents unusable → legacy/default location, with a warning
	SourceDefault   = "default"   // non-Windows default
)

// Resolution is the outcome of choosing the media library location.
type Resolution struct {
	Path             string `json:"path"`
	Source           string `json:"source"`
	DefaultPath      string `json:"defaultPath"`      // where the library belongs by default
	LegacyPath       string `json:"legacyPath"`       // old Windows location ("" elsewhere)
	MigrationPending bool   `json:"migrationPending"` // a legacy library exists and the new location has none
	Warning          string `json:"warning,omitempty"`
}

// env isolates everything platform specific so resolution is testable.
type env struct {
	goos         string
	documents    func() (string, error) // real Documents folder (SHGetKnownFolderPath on Windows)
	localAppData func() string
	dataDirOther func() string // default on non-Windows
}

func realEnv() env {
	return env{goos: runtime.GOOS, documents: documentsDir, localAppData: func() string { return os.Getenv("LOCALAPPDATA") }, dataDirOther: defaultDataDirOther}
}

// HasLibrary reports whether dir contains a media library database.
func HasLibrary(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, "database", "photobooth.db"))
	return err == nil && !st.IsDir()
}

func isMigrated(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, MigratedMarker))
	return err == nil
}

func writable(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	p := filepath.Join(dir, ".write-test")
	if err := os.WriteFile(p, []byte("ok"), 0o644); err != nil {
		return err
	}
	return os.Remove(p)
}

// resolve implements the decision table:
//
//	custom folder configured                      → use it, never migrate
//	new library already has data                  → use it
//	legacy library exists, not yet migrated       → keep using legacy, offer migration
//	otherwise                                     → new (Documents) library
//	Documents unavailable / read-only             → legacy/default location + warning
func (m *Manager) resolve(e env) Resolution {
	s := m.Get()
	if s.MediaDir != "" {
		return Resolution{Path: s.MediaDir, Source: SourceCustom, DefaultPath: s.MediaDir}
	}
	if e.goos != "windows" {
		p := e.dataDirOther()
		return Resolution{Path: p, Source: SourceDefault, DefaultPath: p}
	}
	legacy := ""
	if la := e.localAppData(); la != "" {
		legacy = filepath.Join(la, AppDirName)
	}
	docs, err := e.documents()
	var def string
	if err == nil {
		def = filepath.Join(docs, LibraryFolderName)
		if werr := writable(def); werr != nil {
			err = fmt.Errorf("%s is not writable: %w", def, werr)
		}
	}
	if err != nil {
		fb := legacy
		if fb == "" {
			fb = filepath.Join(os.TempDir(), AppDirName)
		}
		return Resolution{Path: fb, Source: SourceFallback, DefaultPath: def, LegacyPath: legacy,
			Warning: "The Documents folder is unavailable (" + err.Error() + "). Using " + fb + " instead. Fix the folder or choose another library folder in Settings."}
	}
	r := Resolution{Path: def, Source: SourceDocuments, DefaultPath: def, LegacyPath: legacy}
	if HasLibrary(def) {
		return r
	}
	if legacy != "" && HasLibrary(legacy) && !isMigrated(legacy) {
		r.Path, r.Source, r.MigrationPending = legacy, SourceLegacy, true
	}
	return r
}

// ResolveLibrary chooses the library location (see resolve).
func (m *Manager) ResolveLibrary() Resolution { return m.resolve(realEnv()) }

// DataDir returns the effective media library root.
func (m *Manager) DataDir() string { return m.ResolveLibrary().Path }

var errNotWindows = errors.New("Documents folder lookup is only implemented on Windows")
