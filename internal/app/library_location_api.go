package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/settings"
	"github.com/st-mich43l/apexvoid-photobooth/internal/storage"
)

// LibraryStatus describes the active media library location.
type LibraryStatus struct {
	Path             string `json:"path"`
	Source           string `json:"source"` // documents | legacy | custom | fallback | default
	DefaultPath      string `json:"defaultPath"`
	LegacyPath       string `json:"legacyPath"`
	MigrationPending bool   `json:"migrationPending"`
	RestartRequired  bool   `json:"restartRequired"`
	Warning          string `json:"warning"`
	ConfigPath       string `json:"configPath"`
	FreeBytes        int64  `json:"freeBytes"`
}

// LibraryStatus reports where media is stored and whether a migration is offered.
func (s *Service) LibraryStatus() LibraryStatus {
	r := s.libRes
	return LibraryStatus{Path: s.db.Layout.Root, Source: r.Source, DefaultPath: r.DefaultPath, LegacyPath: r.LegacyPath,
		MigrationPending: r.MigrationPending && !s.restartRequired.Load(), RestartRequired: s.restartRequired.Load(), Warning: r.Warning,
		ConfigPath: s.settings.Path, FreeBytes: freeBytes(s.db.Layout.Root)}
}

// MigrationReport is returned to the UI after a successful migration.
type MigrationReport struct {
	storage.MigrateResult
	RestartRequired bool `json:"restartRequired"`
}

// MigrateLibrary copies the existing library (the old %LOCALAPPDATA% folder)
// to the default Documents location after the operator agreed. The old folder
// is never modified or deleted; the app must be restarted to switch over, and
// until then writes are blocked so nothing lands in the old copy only.
func (s *Service) MigrateLibrary() (MigrationReport, error) {
	if !s.libRes.MigrationPending || s.restartRequired.Load() {
		return MigrationReport{}, newUserErr("There is nothing to migrate.", "")
	}
	if st := s.eng.Snapshot().State; st == "COUNTDOWN" || st == "CAPTURING" || st == "REVIEW" || st == "SAVING" || s.VideoState().State == "recording" {
		return MigrationReport{}, newUserErr("A capture is in progress.", "Finish or cancel it, then migrate.")
	}
	if !s.migrating.CompareAndSwap(false, true) {
		return MigrationReport{}, newUserErr("A migration is already running.", "")
	}
	defer s.migrating.Store(false)
	_ = s.eng.Reset() // stop live view/engine so nothing writes meanwhile

	src := s.db.Layout
	dst := storage.Layout{Root: s.libRes.DefaultPath}
	destOwner, err := storage.AcquireOwnership(dst.Root)
	if err != nil {
		return MigrationReport{}, newUserErr("The destination library is in use.", "Close the other instance and try again.")
	}
	defer destOwner.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()
	last := time.Now()
	res, err := storage.MigrateLibrary(ctx, src, dst, storage.MigrateOptions{
		SrcDB: s.db, FreeBytes: freeBytes,
		Progress: func(done, total int64) {
			if time.Since(last) > 250*time.Millisecond || done == total {
				last = time.Now()
				s.emit("migration:progress", map[string]int64{"done": done, "total": total})
			}
		},
	})
	if err != nil {
		s.log.Error("library migration failed", "from", src.Root, "to", dst.Root, "err", err)
		var ce *storage.ConflictError
		switch {
		case errors.As(err, &ce):
			return MigrationReport{}, newUserErr(err.Error(), "Move or rename that file in the new folder, or choose another library folder in Settings. Your old library is untouched.")
		case errors.Is(err, storage.ErrDestHasLibrary):
			return MigrationReport{}, newUserErr(err.Error(), "Nothing was copied. Your old library is untouched.")
		}
		return MigrationReport{}, newUserErr(err.Error(), "Your old library is untouched. Fix the problem and try again; a partial copy is resumed automatically.")
	}
	note := fmt.Sprintf("This library was copied to %s on %s.\nThis folder was left untouched as a backup; delete it only after checking the new library.\n", dst.Root, time.Now().Format(time.RFC3339))
	_ = os.WriteFile(filepath.Join(src.Root, settings.MigratedMarker), []byte(note), 0o644)
	s.restartRequired.Store(true)
	s.log.Info("library migrated", "from", src.Root, "to", dst.Root, "files", res.Files, "bytes", res.Bytes)
	s.emit("library:changed", s.LibraryStatus())
	return MigrationReport{MigrateResult: res, RestartRequired: true}, nil
}

// KeepLegacyLibrary makes the old folder an explicit custom library, so the
// migration prompt is not shown again.
func (s *Service) KeepLegacyLibrary() error {
	if !s.libRes.MigrationPending {
		return nil
	}
	cur := s.settings.Get()
	cur.MediaDir = s.libRes.LegacyPath
	if err := s.settings.Save(cur); err != nil {
		return newUserErr(err.Error(), "")
	}
	s.libRes.MigrationPending, s.libRes.Source = false, settings.SourceCustom
	s.emit("library:changed", s.LibraryStatus())
	return nil
}

// OpenLibraryFolder opens the media library in the OS file manager.
func (s *Service) OpenLibraryFolder() error {
	return userErr(openFolder(s.db.Layout.Root))
}

func openFolder(dir string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer.exe", filepath.FromSlash(dir)).Start()
	case "darwin":
		return exec.Command("open", dir).Start()
	default:
		return exec.Command("xdg-open", dir).Start()
	}
}
