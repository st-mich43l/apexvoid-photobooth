package sessions

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestTrashSessionOver2000AndRestore(t *testing.T) {
	s, db := open(t, t.TempDir())
	sess, err := s.Create("photo", 1, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.TrashSession(sess.ID); err == nil {
		t.Fatal("active session must not be trashed")
	}
	if err = s.SetStatus(sess.ID, StatusCompleted, ""); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(db.Layout.SessionDir(sess.ID), "originals")
	if err = os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2001; i++ {
		id := fmt.Sprintf("M%04d", i)
		p := filepath.Join(dir, id+".jpg")
		if err = os.WriteFile(p, []byte("synthetic"), 0600); err != nil {
			t.Fatal(err)
		}
		rel, _ := db.Layout.Rel(p)
		if _, err = tx.Exec(`INSERT INTO media(id,session_id,kind,rel_path,captured_at,created_at) VALUES(?,?,'photo',?,'t','t')`, id, sess.ID, rel); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO print_jobs(id,session_id,printer,status,created_at,updated_at) VALUES('job',?,'printer','completed','t','t')`, sess.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.TrashSession(sess.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = db.QueryRow(`SELECT COUNT(*) FROM media WHERE session_id=? AND status='trashed'`, sess.ID).Scan(&n); err != nil || n != 2001 {
		t.Fatalf("trashed %d %v", n, err)
	}
	if err = s.TrashSession(sess.ID); err != nil {
		t.Fatalf("repeat: %v", err)
	}
	if err = s.RestoreSession(sess.ID); err != nil {
		t.Fatal(err)
	}
	var jobs int
	if err = db.QueryRow(`SELECT COUNT(*) FROM print_jobs WHERE session_id=?`, sess.ID).Scan(&jobs); err != nil || jobs != 1 {
		t.Fatalf("print history changed: %d %v", jobs, err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM media WHERE session_id=? AND status='active'`, sess.ID).Scan(&n); err != nil || n != 2001 {
		t.Fatalf("restored %d %v", n, err)
	}
}

func TestTrashCrashRecoveryAndRollback(t *testing.T) {
	s, db := open(t, t.TempDir())
	sess, _ := s.Create("photo", 1, "", false)
	m, err := s.AddPhoto(sess.ID, PhotoInput{Data: jpegBytes(t, 10, 10, color.NRGBA{R: 1, G: 2, B: 3, A: 255})})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetStatus(sess.ID, StatusCompleted, ""); err != nil {
		t.Fatal(err)
	}
	original, _ := db.Layout.Abs(m.RelPath)
	target := filepath.Join(db.Layout.TrashDir(), m.ID, filepath.FromSlash(m.RelPath))
	if _, err = db.Exec(`INSERT INTO session_trash_ops(session_id,previous_status,state) VALUES(?,?,'trashing')`, sess.ID, StatusCompleted); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE sessions SET status='trashing' WHERE id=?`, sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO media_trash_ops(media_id,previous_status,state,session_owner) VALUES(?,?,'trashing',?)`, m.ID, MediaActive, sess.ID); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(original, target); err != nil {
		t.Fatal(err)
	}
	// A restart can finish an interrupted multi-file move.
	if _, err = s.Verify(false); err != nil {
		t.Fatal(err)
	}
	if err = s.RestoreSession(sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(original); err != nil {
		t.Fatal(err)
	}
}

func TestTrashSessionPartialFailureCanRestore(t *testing.T) {
	s, db := open(t, t.TempDir())
	sess, _ := s.Create("photo", 1, "", false)
	m, err := s.AddPhoto(sess.ID, PhotoInput{Data: jpegBytes(t, 10, 10, color.NRGBA{R: 2, G: 3, B: 4, A: 255})})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetStatus(sess.ID, StatusCompleted, ""); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(db.Layout.TrashDir(), m.ID, "sessions", sess.ID, "previews")
	if err = os.MkdirAll(filepath.Dir(blocker), 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(blocker, []byte("block"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = s.TrashSession(sess.ID); err == nil {
		t.Fatal("partial move should fail visibly")
	}
	orig, _ := db.Layout.Abs(m.RelPath)
	trash := filepath.Join(db.Layout.TrashDir(), m.ID, filepath.FromSlash(m.RelPath))
	if _, err = os.Stat(trash); err != nil {
		t.Fatalf("original must remain in trash: %v", err)
	}
	if err = os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	if err = s.RestoreSession(sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(orig); err != nil {
		t.Fatalf("original not restored: %v", err)
	}
	sess2, err := s.Session(sess.ID)
	if err != nil || sess2.Status != StatusCompleted {
		t.Fatalf("session status %+v %v", sess2, err)
	}
}

func TestTrashRecoveryFromInterruptedHardLink(t *testing.T) {
	s, db := open(t, t.TempDir())
	sess, _ := s.Create("photo", 1, "", false)
	m, err := s.AddPhoto(sess.ID, PhotoInput{Data: jpegBytes(t, 10, 10, color.NRGBA{R: 9, A: 255})})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetStatus(sess.ID, StatusCompleted, ""); err != nil {
		t.Fatal(err)
	}
	original, _ := db.Layout.Abs(m.RelPath)
	target := filepath.Join(db.Layout.TrashDir(), m.ID, filepath.FromSlash(m.RelPath))
	if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.Link(original, target); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO media_trash_ops(media_id,previous_status,state) VALUES(?,?,'trashing')`, m.ID, MediaActive); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Verify(false); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(original); !os.IsNotExist(err) {
		t.Fatalf("original link not removed: %v", err)
	}
	if err = s.Restore(m.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(original); err != nil {
		t.Fatal(err)
	}
}

func TestTrashRefusesToOverwriteDifferentExistingFile(t *testing.T) {
	s, db := open(t, t.TempDir())
	sess, _ := s.Create("photo", 1, "", false)
	m, err := s.AddPhoto(sess.ID, PhotoInput{Data: jpegBytes(t, 10, 10, color.NRGBA{R: 4, A: 255})})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetStatus(sess.ID, StatusCompleted, ""); err != nil {
		t.Fatal(err)
	}
	original, _ := db.Layout.Abs(m.RelPath)
	target := filepath.Join(db.Layout.TrashDir(), m.ID, filepath.FromSlash(m.RelPath))
	if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(target, []byte("keep existing"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = s.Trash(m.ID); err == nil {
		t.Fatal("conflict should fail")
	}
	b, err := os.ReadFile(target)
	if err != nil || string(b) != "keep existing" {
		t.Fatalf("existing file changed: %v", err)
	}
	if _, err = os.Stat(original); err != nil {
		t.Fatalf("original lost: %v", err)
	}
	if err = os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err = s.Restore(m.ID); err != nil {
		t.Fatal(err)
	}
}

func TestTrashPreflightFailureLeavesSessionUntouched(t *testing.T) {
	s, db := open(t, t.TempDir())
	sess, err := s.Create("photo", 1, "", false)
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.AddPhoto(sess.ID, PhotoInput{Data: jpegBytes(t, 10, 10, color.NRGBA{R: 1, G: 2, B: 3, A: 255})})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetStatus(sess.ID, StatusCompleted, ""); err != nil {
		t.Fatal(err)
	}
	original, err := db.Layout.Abs(m.RelPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(db.Layout.TrashDir()); err != nil {
		t.Fatal(err)
	}
	if err = s.TrashSession(sess.ID); err == nil {
		t.Fatal("expected preflight error")
	}
	if _, err = os.Stat(original); err != nil {
		t.Fatalf("original changed: %v", err)
	}
	current, err := s.Session(sess.ID)
	if err != nil || current.Status != StatusCompleted {
		t.Fatalf("session changed: %+v %v", current, err)
	}
	var n int
	if err = db.QueryRow(`SELECT COUNT(*) FROM session_trash_ops`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("journal changed: %d %v", n, err)
	}
}
