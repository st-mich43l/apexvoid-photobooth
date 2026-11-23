package sessions

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var unsafeExportName = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func exportName(s string) string {
	s = unsafeExportName.ReplaceAllString(s, "_")
	s = strings.Trim(s, "._")
	if s == "" {
		return "item"
	}
	return s
}

// ExportSession streams one session into a new ZIP. It never modifies originals
// and removes an incomplete archive on any missing file or write failure.
func (s *Store) ExportSession(id, dest string, edited bool) (result string, err error) {
	sess, err := s.Session(id)
	if err != nil {
		return "", err
	}
	if sess.Status == StatusActive {
		return "", errors.New("finish the active session before exporting")
	}
	if sess.Status == "trashed" || sess.Status == "trashing" {
		return "", errors.New("restore the session before exporting")
	}
	if !safeLocalID(id) {
		return "", errors.New("invalid session ID")
	}
	sessionRoot, err := filepath.EvalSymlinks(s.db.Layout.SessionDir(id))
	if err != nil {
		return "", fmt.Errorf("resolve session directory: %w", err)
	}
	// Snapshot media metadata to a private temporary stream. The database is
	// released before copying image bytes so other sessions can keep capturing.
	snapshot, err := s.snapshotExportMedia(id)
	if err != nil {
		return "", err
	}
	defer func() { name := snapshot.Name(); _ = snapshot.Close(); _ = os.Remove(name) }()
	if dest == "" {
		dest = filepath.Join(s.db.Layout.Root, "exports")
	}
	if err = os.MkdirAll(dest, 0755); err != nil {
		return "", err
	}
	var f *os.File
	for i := 0; i < 10000; i++ {
		name := exportName(id)
		if i > 0 {
			name = fmt.Sprintf("%s-%d", name, i)
		}
		result = filepath.Join(dest, name+".zip")
		f, err = os.OpenFile(result, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if os.IsExist(err) {
			continue
		}
		break
	}
	if err != nil {
		return "", err
	}
	if f == nil {
		return "", errors.New("could not allocate export name")
	}
	outputPath := result
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(outputPath)
			result = ""
		}
	}()
	zw := zip.NewWriter(f)
	meta, err := zw.Create("session.json")
	if err != nil {
		return "", err
	}
	// No application settings, internal paths, credentials or unrelated sessions.
	if err = json.NewEncoder(meta).Encode(struct {
		ID, CreatedAt, Status, Mode, FrameName string
		Simulated                              bool
	}{sess.ID, sess.CreatedAt, sess.Status, sess.Mode, sess.FrameName, sess.Simulated}); err != nil {
		return "", err
	}
	decoder := json.NewDecoder(snapshot)
	for index := 0; ; index++ {
		var m Media
		if e := decoder.Decode(&m); errors.Is(e, io.EOF) {
			break
		} else if e != nil {
			return "", e
		}
		folder := "originals"
		if m.Kind == "strip" || m.Kind == "print" {
			folder = "compositions"
		} else if m.Kind == "video" {
			folder = "videos"
		}
		if m.RelPath == "" {
			return "", fmt.Errorf("media %s has no original file", m.ID)
		}
		for _, v := range []struct{ kind, rel string }{{folder, m.RelPath}, {"edited", func() string {
			if edited {
				return m.EditedRel
			}
			return ""
		}()}} {
			if v.rel == "" {
				continue
			}
			path, e := s.db.Layout.Abs(v.rel)
			if e != nil {
				return "", e
			}
			resolved, e := filepath.EvalSymlinks(path)
			if e != nil {
				return "", fmt.Errorf("export %s: %w", m.ID, e)
			}
			rel, e := filepath.Rel(sessionRoot, resolved)
			if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return "", fmt.Errorf("media %s is outside its session", m.ID)
			}
			in, e := os.Open(path)
			if e != nil {
				return "", fmt.Errorf("export %s: %w", m.ID, e)
			}
			name := v.kind + "/" + fmt.Sprintf("%08d_", index) + exportName(m.ID) + "_" + exportName(filepath.Base(path))
			out, e := zw.Create(name)
			if e == nil {
				_, e = io.Copy(out, in)
			}
			ce := in.Close()
			if e != nil {
				return "", e
			}
			if ce != nil {
				return "", ce
			}
		}
	}

	if err = zw.Close(); err != nil {
		return "", err
	}
	if err = f.Sync(); err != nil {
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	return result, nil
}

func (s *Store) snapshotExportMedia(id string) (*os.File, error) {
	tmp, err := os.CreateTemp("", "apexvoid-export-index-*.jsonl")
	if err != nil {
		return nil, err
	}
	cleanup := func() { name := tmp.Name(); _ = tmp.Close(); _ = os.Remove(name) }
	rows, err := s.db.Query(`SELECT `+mediaCols+` FROM media WHERE session_id=? AND status IN ('active','partial','missing','discarded') ORDER BY captured_at,id`, id)
	if err != nil {
		cleanup()
		return nil, err
	}
	enc := json.NewEncoder(tmp)
	for rows.Next() {
		m, e := scanMedia(rows)
		if e != nil {
			rows.Close()
			cleanup()
			return nil, e
		}
		if e = enc.Encode(m); e != nil {
			rows.Close()
			cleanup()
			return nil, e
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		cleanup()
		return nil, err
	}
	if _, err = tmp.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, err
	}
	return tmp, nil
}
