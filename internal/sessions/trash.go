package sessions

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// checkTrashLinks verifies the common library-to-trash path before recording a
// new operation. Per-file failures (including mount points and permissions) are
// still journalled and recoverable; no file is overwritten.
func (s *Store) checkTrashLinks() error {
	source, err := os.CreateTemp(s.db.Layout.SessionsDir(), ".trash-link-probe-")
	if err != nil {
		return fmt.Errorf("cannot test trash support: %w", err)
	}
	name := source.Name()
	defer os.Remove(name)
	if err = source.Close(); err != nil {
		return err
	}
	target, err := os.CreateTemp(s.db.Layout.TrashDir(), ".trash-link-probe-")
	if err != nil {
		return fmt.Errorf("cannot test trash support: %w", err)
	}
	dest := target.Name()
	if err = target.Close(); err != nil {
		os.Remove(dest)
		return err
	}
	if err = os.Remove(dest); err != nil {
		return err
	}
	defer os.Remove(dest)
	if err = os.Link(name, dest); err != nil {
		return fmt.Errorf("trash is unavailable: this library must support same-volume hard links: %w", err)
	}
	return nil
}

// Call with s.mu held when a write can race a session trash operation.
func (s *Store) checkWritableSession(id string) error {
	var status string
	if err := s.db.QueryRow(`SELECT status FROM sessions WHERE id=?`, id).Scan(&status); err != nil {
		return err
	}
	if status == "trashing" || status == "trashed" {
		return errors.New("session is being moved to trash or has been trashed")
	}
	return nil
}

func safeLocalID(id string) bool {
	return id != "" && id != "." && id != ".." && id == filepath.Base(id) && !strings.ContainsAny(id, `/\:*?"<>|`)
}

// moveTrashFiles is idempotent: an exclusive hard link followed by unlink
// never replaces an existing file. If interrupted between those operations,
// recovery recognizes the two links as the same file and completes the move.
func (s *Store) moveTrashFiles(m Media, restore bool) error {
	if !safeLocalID(m.ID) || !safeLocalID(m.SessionID) {
		return errors.New("invalid media or session ID")
	}
	sessionRoot, err := filepath.EvalSymlinks(s.db.Layout.SessionDir(m.SessionID))
	if err != nil {
		return err
	}
	for i, rel := range []string{m.RelPath, m.PreviewRel, m.ThumbRel, m.EditedRel} {
		if rel == "" {
			continue
		}
		original, err := s.db.Layout.Abs(rel)
		if err != nil {
			return err
		}
		parent, err := filepath.EvalSymlinks(filepath.Dir(original))
		if err != nil {
			return err
		}
		within, err := filepath.Rel(sessionRoot, parent)
		if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
			return fmt.Errorf("media path is outside its session: %s", rel)
		}
		trashed := filepath.Join(s.db.Layout.TrashDir(), m.ID, filepath.FromSlash(rel))
		from, to := original, trashed
		if restore {
			from, to = trashed, original
		}
		source, se := os.Lstat(from)
		destination, de := os.Lstat(to)
		if se == nil && de == nil {
			if os.SameFile(source, destination) {
				if err := os.Remove(from); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("both copies exist for %s; resolve before continuing", rel)
		}
		if se != nil && !os.IsNotExist(se) {
			return se
		}
		if de != nil && !os.IsNotExist(de) {
			return de
		}
		if os.IsNotExist(se) {
			if os.IsNotExist(de) && i == 0 && m.Status != MediaMissing {
				return fmt.Errorf("original file is missing: %s", rel)
			}
			continue
		}
		if !source.Mode().IsRegular() {
			return fmt.Errorf("media path is not a regular file: %s", rel)
		}
		if err := os.MkdirAll(filepath.Dir(to), 0755); err != nil {
			return err
		}
		if err := os.Link(from, to); err != nil {
			return fmt.Errorf("safely link %s (the library filesystem must support hard links): %w", rel, err)
		}
		if err := os.Remove(from); err != nil {
			return fmt.Errorf("finish moving %s: %w", rel, err)
		}
	}
	return nil
}

func (s *Store) trashLocked(id, owner string) error {
	m, err := s.Media(id)
	if err != nil {
		return err
	}
	if m.Status == MediaTrashed {
		return nil
	}
	var existing string
	if err := s.db.QueryRow(`SELECT state FROM media_trash_ops WHERE media_id=?`, id).Scan(&existing); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if existing == "restoring" {
		return errors.New("media restoration is in progress; finish it before moving to trash")
	}
	if _, err = s.db.Exec(`INSERT OR IGNORE INTO media_trash_ops(media_id,previous_status,state,session_owner) VALUES(?,?,'trashing',?)`, id, m.Status, owner); err != nil {
		return err
	}
	if err = s.moveTrashFiles(m, false); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE media SET status=? WHERE id=?`, MediaTrashed, id); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE media_trash_ops SET state='trashed' WHERE media_id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Trash(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.Media(id)
	if err != nil {
		return err
	}
	sess, err := s.Session(m.SessionID)
	if err != nil {
		return err
	}
	if sess.Status == StatusActive {
		return errors.New("an active session's media cannot be moved to trash")
	}
	if err := s.checkTrashLinks(); err != nil {
		return err
	}
	return s.trashLocked(id, "")
}

func (s *Store) restoreLocked(id string) error {
	m, err := s.Media(id)
	if err != nil {
		return err
	}
	before := "active"
	var state string
	err = s.db.QueryRow(`SELECT previous_status,state FROM media_trash_ops WHERE media_id=?`, id).Scan(&before, &state)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if m.Status != MediaTrashed && state != "trashing" && state != "restoring" {
		return nil
	}
	if _, err = s.db.Exec(`INSERT INTO media_trash_ops(media_id,previous_status,state) VALUES(?,?,'restoring') ON CONFLICT(media_id) DO UPDATE SET state='restoring'`, id, before); err != nil {
		return err
	}
	// Use the previous status to distinguish a known missing original.
	m.Status = before
	if err = s.moveTrashFiles(m, true); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE media SET status=? WHERE id=?`, before, id); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM media_trash_ops WHERE media_id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Restore(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.restoreLocked(id)
}

func (s *Store) TrashSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, err := s.Session(id)
	if err != nil {
		return err
	}
	if sess.Status == StatusActive {
		return errors.New("an active session cannot be moved to trash")
	}
	if sess.Status == "trashed" {
		return nil
	}
	var existing string
	if err := s.db.QueryRow(`SELECT state FROM session_trash_ops WHERE session_id=?`, id).Scan(&existing); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if existing == "restoring" {
		return errors.New("session restoration is in progress; finish it before moving to trash")
	}
	if existing == "" {
		if err := s.checkTrashLinks(); err != nil {
			return err
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT OR IGNORE INTO session_trash_ops(session_id,previous_status,state) VALUES(?,?,'trashing')`, id, sess.Status); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE sessions SET status='trashing' WHERE id=?`, id); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return s.processSessionTrashLocked(id, false)
}
func (s *Store) RestoreSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var state string
	if err := s.db.QueryRow(`SELECT state FROM session_trash_ops WHERE session_id=?`, id).Scan(&state); err != nil {
		return err
	}
	if _, err := s.db.Exec(`UPDATE session_trash_ops SET state='restoring' WHERE session_id=?`, id); err != nil {
		return err
	}
	return s.processSessionTrashLocked(id, true)
}
func (s *Store) processSessionTrashLocked(id string, restore bool) error {
	for {
		q := `SELECT id FROM media WHERE session_id=? AND status<>'trashed' LIMIT 200`
		if restore {
			q = `SELECT media_id FROM media_trash_ops WHERE session_owner=? AND state IN ('trashing','trashed','restoring') LIMIT 200`
		}
		rows, err := s.db.Query(q, id)
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var mid string
			if err := rows.Scan(&mid); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, mid)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			break
		}
		for _, mid := range ids {
			if restore {
				err = s.restoreLocked(mid)
			} else {
				err = s.trashLocked(mid, id)
			}
			if err != nil {
				return fmt.Errorf("session %s, media %s: %w", id, mid, err)
			}
		}
	}
	if restore {
		var previous string
		if err := s.db.QueryRow(`SELECT previous_status FROM session_trash_ops WHERE session_id=?`, id).Scan(&previous); err != nil {
			return err
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err = tx.Exec(`UPDATE sessions SET status=? WHERE id=?`, previous, id); err != nil {
			return err
		}
		if _, err = tx.Exec(`DELETE FROM session_trash_ops WHERE session_id=?`, id); err != nil {
			return err
		}
		return tx.Commit()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE sessions SET status='trashed' WHERE id=?`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE session_trash_ops SET state='trashed' WHERE session_id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) recoverTrashLocked() error {
	rows, err := s.db.Query(`SELECT session_id,state FROM session_trash_ops WHERE state IN ('trashing','restoring')`)
	if err != nil {
		return err
	}
	type op struct{ id, state string }
	var ops []op
	for rows.Next() {
		var o op
		if err := rows.Scan(&o.id, &o.state); err != nil {
			rows.Close()
			return err
		}
		ops = append(ops, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, o := range ops {
		if err = s.processSessionTrashLocked(o.id, o.state == "restoring"); err != nil {
			return err
		}
	}
	rows, err = s.db.Query(`SELECT media_id,state FROM media_trash_ops WHERE state IN ('trashing','restoring')`)
	if err != nil {
		return err
	}
	ops = nil
	for rows.Next() {
		var o op
		if err := rows.Scan(&o.id, &o.state); err != nil {
			rows.Close()
			return err
		}
		ops = append(ops, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, o := range ops {
		if o.state == "restoring" {
			err = s.restoreLocked(o.id)
		} else {
			err = s.trashLocked(o.id, "")
		}
		if err != nil {
			return err
		}
	}
	return nil
}
