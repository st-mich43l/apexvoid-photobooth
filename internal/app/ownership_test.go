package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/settings"
	"github.com/st-mich43l/apexvoid-photobooth/internal/storage"
)

func TestLibraryOwnershipBeforeDatabaseOpen(t *testing.T) {
	root := t.TempDir()
	resolve := func(*settings.Manager) settings.Resolution { return settings.Resolution{Path: root} }
	opts := Options{ConfigPath: filepath.Join(t.TempDir(), "settings.json"), ResolveLibrary: resolve, SkipRestore: true}
	first, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := New(opts); !errors.Is(err, storage.ErrLibraryOwned) {
		if second != nil {
			_ = second.Shutdown()
		}
		t.Fatalf("expected exclusive ownership error: %v", err)
	}
	if err := first.Shutdown(); err != nil {
		t.Fatal(err)
	}
	second, err := New(opts)
	if err != nil {
		t.Fatalf("ownership not released: %v", err)
	}
	_ = second.Shutdown()
}

// TestLargeLibraryStartup exercises the current synchronous safety scan. The
// fixture uses tiny synthetic files and is not a Windows hardware benchmark.
func TestLargeLibraryStartup(t *testing.T) {
	root := t.TempDir()
	layout := storage.Layout{Root: root}
	db, err := storage.Open(layout)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	stamp := "2026-01-01T00:00:00Z"
	for i := 0; i < 2001; i++ {
		id := fmt.Sprintf("S%05d", i)
		if err = layout.EnsureSession(id); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(layout.SessionDir(id), "originals", "synthetic.jpg")
		if err = os.WriteFile(path, []byte("synthetic"), 0600); err != nil {
			t.Fatal(err)
		}
		rel, err := layout.Rel(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(`INSERT INTO sessions(id,created_at,updated_at,status,mode) VALUES(?,?,?,'completed','photo')`, id, stamp, stamp); err != nil {
			t.Fatal(err)
		}
		for j := 0; j < 5; j++ {
			if _, err = tx.Exec(`INSERT INTO media(id,session_id,kind,rel_path,captured_at,created_at,status) VALUES(?,?,'photo',?,?,?,'active')`, fmt.Sprintf("M%05d-%d", i, j), id, rel, stamp, stamp); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	svc, err := New(Options{ConfigPath: filepath.Join(t.TempDir(), "settings.json"), ResolveLibrary: func(*settings.Manager) settings.Resolution { return settings.Resolution{Path: root} }, SkipRestore: true})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Shutdown()
	if svc.library.Checked != 10005 {
		t.Fatalf("checked %d media", svc.library.Checked)
	}
	t.Logf("startup: %s, 2001 sessions, 10005 records, synthetic files", elapsed)
}

func TestStartupStopsOnUnrecoverableTrashJournal(t *testing.T) {
	root := t.TempDir()
	opts := Options{ConfigPath: filepath.Join(t.TempDir(), "settings.json"), ResolveLibrary: func(*settings.Manager) settings.Resolution { return settings.Resolution{Path: root} }, SkipRestore: true}
	svc, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.db.Exec(`INSERT INTO sessions(id,created_at,updated_at,status,mode) VALUES('missing','2026-01-01','2026-01-01','trashing','photo'); INSERT INTO media(id,session_id,kind,rel_path,captured_at,created_at,status) VALUES('m','missing','photo','sessions/missing/originals/a.jpg','2026-01-01','2026-01-01','active'); INSERT INTO session_trash_ops(session_id,previous_status,state) VALUES('missing','completed','trashing')`); err != nil {
		t.Fatal(err)
	}
	if err = svc.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if second, err := New(opts); err == nil {
		_ = second.Shutdown()
		t.Fatal("startup must not expose an unrecovered library")
	}
	// The failed startup must release ownership so an operator can repair it.
	owner, err := storage.AcquireOwnership(root)
	if err != nil {
		t.Fatal(err)
	}
	_ = owner.Close()
}
