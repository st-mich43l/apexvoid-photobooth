// Package sessions manages the lifecycle of photo/video sessions and the
// media records inside them. Originals are written once and never replaced.
package sessions

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/media"
	"github.com/st-mich43l/apexvoid-photobooth/internal/storage"
)

// Status values of a session.
const (
	StatusActive      = "active"
	StatusCompleted   = "completed"
	StatusCancelled   = "cancelled"
	StatusFailed      = "failed"
	StatusInterrupted = "interrupted" // app/PC stopped mid-session; originals kept
)

// Media status values.
const (
	MediaActive    = "active"
	MediaDiscarded = "discarded" // replaced by a retake; file kept on disk
	MediaTrashed   = "trashed"
	MediaMissing   = "missing"
	MediaPartial   = "partial" // recovered, possibly truncated recording
)

// Session is one booth session.
type Session struct {
	ID           string `json:"id"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
	Status       string `json:"status"`
	Mode         string `json:"mode"`
	ShotsPlanned int    `json:"shotsPlanned"`
	TemplateID   string `json:"templateId"`
	Simulated    bool   `json:"simulated"`
	Note         string `json:"note"`
	MediaCount   int    `json:"mediaCount"`
	// The frame chosen for the session (the immutable snapshot is in FrameDir).
	FrameID   string `json:"frameId"`
	FrameName string `json:"frameName"`
	VariantID string `json:"variantId"`
	FrameRev  int    `json:"frameRev"`
	Caption   string `json:"caption"`
	FrameDir  string `json:"frameDir"` // library-relative snapshot folder
}

// FrameRef identifies the frame (and snapshot) a session or export used.
type FrameRef struct {
	ID, Name, Variant, Caption, Dir string
	Revision                        int
}

// Media is one stored file.
type Media struct {
	ID         string `json:"id"`
	SessionID  string `json:"sessionId"`
	Kind       string `json:"kind"` // photo | video | strip
	Slot       int    `json:"slot"`
	RelPath    string `json:"relPath"`
	ThumbRel   string `json:"thumbRel"`
	PreviewRel string `json:"previewRel"`
	EditedRel  string `json:"editedRel"`
	Edits      string `json:"edits"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	SizeBytes  int64  `json:"sizeBytes"`
	SHA256     string `json:"sha256"`
	DurationMs int64  `json:"durationMs"`
	Simulated  bool   `json:"simulated"`
	Source     string `json:"source"`
	Status     string `json:"status"`
	CapturedAt string `json:"capturedAt"`
	// the frame that produced a composed image ("" for photos and videos)
	FrameID   string `json:"frameId"`
	FrameName string `json:"frameName"`
	VariantID string `json:"variantId"`
	FrameRev  int    `json:"frameRev"`
	FrameDir  string `json:"frameDir"`
}

// Store is the session/media repository.
type Store struct {
	db  *storage.DB
	now func() time.Time
	mu  sync.Mutex // serialises writes that touch files and rows together
}

// New creates a Store over an open library database.
func New(db *storage.DB) *Store { return &Store{db: db, now: time.Now} }

// SetClock overrides the clock (tests).
func (s *Store) SetClock(f func() time.Time) { s.now = f }

func (s *Store) ts() string { return s.now().UTC().Format(time.RFC3339Nano) }

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failing is unrecoverable
	}
	return hex.EncodeToString(b)
}

// Create starts a session and its directory tree.
func (s *Store) Create(mode string, shots int, templateID string, simulated bool) (Session, error) {
	if mode != "photo" && mode != "video" {
		return Session{}, fmt.Errorf("invalid session mode %q", mode)
	}
	if shots < 1 || shots > 12 {
		return Session{}, fmt.Errorf("shots must be 1..12")
	}
	id := "S" + s.now().Format("20060102-150405") + "-" + randHex(3)
	if err := s.db.Layout.EnsureSession(id); err != nil {
		return Session{}, err
	}
	t := s.ts()
	sess := Session{ID: id, CreatedAt: t, UpdatedAt: t, Status: StatusActive, Mode: mode, ShotsPlanned: shots, TemplateID: templateID, Simulated: simulated}
	_, err := s.db.Exec(`INSERT INTO sessions(id,created_at,updated_at,status,mode,shots_planned,template_id,simulated) VALUES(?,?,?,?,?,?,?,?)`,
		id, t, t, sess.Status, mode, shots, templateID, b2i(simulated))
	if err != nil {
		return Session{}, err
	}
	return sess, nil
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// SetStatus updates a session's status and optional note.
func (s *Store) SetStatus(id, status, note string) error {
	res, err := s.db.Exec(`UPDATE sessions SET status=?, note=CASE WHEN ?='' THEN note ELSE ? END, updated_at=? WHERE id=?`, status, note, note, s.ts(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// PhotoInput is a captured original to store.
type PhotoInput struct {
	Slot      int
	Data      []byte
	Simulated bool
	Source    string
}

// AddPhoto stores the original (exclusive create), derives preview and
// thumbnail and records everything. The original is durable on disk before the
// row exists, so a crash can never lose a capture.
func (s *Store) AddPhoto(sessionID string, in PhotoInput) (Media, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.Session(sessionID); err != nil {
		return Media{}, err
	}
	w, h, err := media.Dimensions(in.Data)
	if err != nil {
		return Media{}, fmt.Errorf("camera returned data that is not a decodable image: %w", err)
	}
	img, err := media.Decode(in.Data)
	if err != nil {
		return Media{}, err
	}
	id := "M" + randHex(6)
	base := fmt.Sprintf("IMG_%02d_%s", in.Slot+1, id)
	l := s.db.Layout
	origAbs := filepath.Join(l.SessionDir(sessionID), "originals", base+".jpg")
	if err := storage.WriteNew(origAbs, in.Data); err != nil {
		return Media{}, err
	}
	m := Media{ID: id, SessionID: sessionID, Kind: "photo", Slot: in.Slot, Width: w, Height: h, SizeBytes: int64(len(in.Data)),
		Simulated: in.Simulated, Source: in.Source, Status: MediaActive, CapturedAt: s.ts()}
	sum := sha256.Sum256(in.Data)
	m.SHA256 = hex.EncodeToString(sum[:])
	if m.RelPath, err = l.Rel(origAbs); err != nil {
		return Media{}, err
	}
	// Derived files may fail without losing the capture: report but keep the original.
	prevAbs := filepath.Join(l.SessionDir(sessionID), "previews", base+".jpg")
	thumbAbs := filepath.Join(l.SessionDir(sessionID), "previews", base+"_thumb.jpg")
	derr := writeDerived(img, prevAbs, thumbAbs)
	if derr == nil {
		m.PreviewRel, _ = l.Rel(prevAbs)
		m.ThumbRel, _ = l.Rel(thumbAbs)
	}
	if err := s.insertMedia(m); err != nil {
		return Media{}, err
	}
	_, _ = s.db.Exec(`UPDATE sessions SET updated_at=? WHERE id=?`, s.ts(), sessionID)
	if derr != nil {
		return m, fmt.Errorf("original saved, but preview generation failed: %w", derr)
	}
	return m, nil
}

func writeDerived(img image.Image, prevAbs, thumbAbs string) error {
	prev, err := media.EncodeJPEG(media.Fit(img, 1600, 1600), 85)
	if err != nil {
		return err
	}
	th, err := media.EncodeJPEG(media.Fit(img, 420, 420), 80)
	if err != nil {
		return err
	}
	if err := storage.WriteReplace(prevAbs, prev); err != nil {
		return err
	}
	return storage.WriteReplace(thumbAbs, th)
}

const mediaCols = `id,session_id,kind,slot,rel_path,thumb_rel,preview_rel,edited_rel,edits_json,width,height,size_bytes,sha256,duration_ms,simulated,source,status,captured_at,frame_id,frame_name,variant_id,frame_rev,frame_dir`

func (s *Store) insertMedia(m Media) error {
	_, err := s.db.Exec(`INSERT INTO media(`+mediaCols+`,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.ID, m.SessionID, m.Kind, m.Slot, m.RelPath, m.ThumbRel, m.PreviewRel, m.EditedRel, m.Edits, m.Width, m.Height, m.SizeBytes,
		m.SHA256, m.DurationMs, b2i(m.Simulated), m.Source, m.Status, m.CapturedAt, m.FrameID, m.FrameName, m.VariantID, m.FrameRev, m.FrameDir, s.ts())
	return err
}

type scanner interface{ Scan(...any) error }

func scanMedia(r scanner) (Media, error) {
	var m Media
	var sim int
	err := r.Scan(&m.ID, &m.SessionID, &m.Kind, &m.Slot, &m.RelPath, &m.ThumbRel, &m.PreviewRel, &m.EditedRel, &m.Edits,
		&m.Width, &m.Height, &m.SizeBytes, &m.SHA256, &m.DurationMs, &sim, &m.Source, &m.Status, &m.CapturedAt,
		&m.FrameID, &m.FrameName, &m.VariantID, &m.FrameRev, &m.FrameDir)
	m.Simulated = sim != 0
	return m, err
}

// Session returns one session with its active media count.
func (s *Store) Session(id string) (Session, error) {
	row := s.db.QueryRow(sessionSelect+` WHERE s.id=?`, id)
	return scanSession(row)
}

const sessionSelect = `SELECT s.id,s.created_at,s.updated_at,s.status,s.mode,s.shots_planned,s.template_id,s.simulated,s.note,
	s.frame_id,s.frame_name,s.variant_id,s.frame_rev,s.caption,s.frame_dir,
	(SELECT COUNT(*) FROM media m WHERE m.session_id=s.id AND m.status IN ('active','partial')) FROM sessions s`

func scanSession(r scanner) (Session, error) {
	var x Session
	var sim int
	err := r.Scan(&x.ID, &x.CreatedAt, &x.UpdatedAt, &x.Status, &x.Mode, &x.ShotsPlanned, &x.TemplateID, &sim, &x.Note,
		&x.FrameID, &x.FrameName, &x.VariantID, &x.FrameRev, &x.Caption, &x.FrameDir, &x.MediaCount)
	x.Simulated = sim != 0
	return x, err
}

// List returns sessions, newest first.
func (s *Store) List(limit, offset int) ([]Session, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(sessionSelect+` WHERE s.status<>'trashed' ORDER BY s.created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		x, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// Media returns one media record (any status).
func (s *Store) Media(id string) (Media, error) {
	return scanMedia(s.db.QueryRow(`SELECT `+mediaCols+` FROM media WHERE id=?`, id))
}

// ListMedia returns visible media (active/partial/missing), newest first. An
// empty sessionID lists the whole library.
func (s *Store) ListMedia(sessionID string, limit, offset int) ([]Media, error) {
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	q := `SELECT ` + mediaCols + ` FROM media WHERE status IN ('active','partial','missing')`
	args := []any{}
	if sessionID != "" {
		q += ` AND session_id=?`
		args = append(args, sessionID)
	}
	q += ` ORDER BY captured_at DESC, slot ASC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Media
	for rows.Next() {
		m, err := scanMedia(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// DiscardMedia marks a photo as replaced by a retake. The file stays on disk
// (the capture flow never deletes originals).
func (s *Store) DiscardMedia(id string) error {
	_, err := s.db.Exec(`UPDATE media SET status=? WHERE id=? AND status='active'`, MediaDiscarded, id)
	return err
}

// VideoInput describes a finished (or recovered) recording that already sits
// in the session's videos directory.
type VideoInput struct {
	AbsPath    string
	DurationMs int64
	Width      int
	Height     int
	Partial    bool
	Source     string
	ThumbJPEG  []byte // optional poster frame
}

// NewVideoPath reserves a media id and path inside the session's videos dir.
func (s *Store) NewVideoPath(sessionID string) (id, abs string, err error) {
	if _, err = s.Session(sessionID); err != nil {
		return "", "", err
	}
	id = "M" + randHex(6)
	return id, filepath.Join(s.db.Layout.SessionDir(sessionID), "videos", "VID_"+id+".mp4"), nil
}

// AddVideo registers a recording written by the video engine.
func (s *Store) AddVideo(sessionID, id string, in VideoInput) (Media, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := os.Stat(in.AbsPath)
	if err != nil {
		return Media{}, err
	}
	l := s.db.Layout
	m := Media{ID: id, SessionID: sessionID, Kind: "video", Width: in.Width, Height: in.Height, SizeBytes: st.Size(), DurationMs: in.DurationMs,
		Source: in.Source, Status: MediaActive, CapturedAt: s.ts()}
	if in.Partial {
		m.Status = MediaPartial
	}
	if m.RelPath, err = l.Rel(in.AbsPath); err != nil {
		return Media{}, err
	}
	if len(in.ThumbJPEG) > 0 {
		abs := filepath.Join(l.SessionDir(sessionID), "previews", "VID_"+id+"_thumb.jpg")
		if err := storage.WriteReplace(abs, in.ThumbJPEG); err == nil {
			m.ThumbRel, _ = l.Rel(abs)
		}
	}
	if err := s.insertMedia(m); err != nil {
		return Media{}, err
	}
	_, _ = s.db.Exec(`UPDATE sessions SET updated_at=? WHERE id=?`, s.ts(), sessionID)
	return m, nil
}

// AddExport stores a generated image (e.g. a composed photo strip) under
// exports/ and records it as media of kind "strip".
func (s *Store) AddExport(sessionID, kind, name string, data []byte, w, h int) (Media, error) {
	return s.AddFrameExport(sessionID, kind, name, data, w, h, FrameRef{})
}

// SetFrame records the frame (and its snapshot folder) the session now uses.
func (s *Store) SetFrame(sessionID string, f FrameRef) error {
	_, err := s.db.Exec(`UPDATE sessions SET template_id=?,frame_id=?,frame_name=?,variant_id=?,frame_rev=?,caption=?,frame_dir=?,updated_at=? WHERE id=?`,
		f.ID, f.ID, f.Name, f.Variant, f.Revision, f.Caption, f.Dir, s.ts(), sessionID)
	return err
}

// AddFrameExport is AddExport that also records which frame produced the image.
func (s *Store) AddFrameExport(sessionID, kind, name string, data []byte, w, h int, f FrameRef) (Media, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := "M" + randHex(6)
	l := s.db.Layout
	abs := filepath.Join(l.SessionDir(sessionID), "exports", fmt.Sprintf("%s_%s.jpg", name, id))
	if err := storage.WriteNew(abs, data); err != nil {
		return Media{}, err
	}
	m := Media{ID: id, SessionID: sessionID, Kind: kind, Width: w, Height: h, SizeBytes: int64(len(data)), Status: MediaActive, CapturedAt: s.ts(), Source: "composed",
		FrameID: f.ID, FrameName: f.Name, VariantID: f.Variant, FrameRev: f.Revision, FrameDir: f.Dir}
	sum := sha256.Sum256(data)
	m.SHA256 = hex.EncodeToString(sum[:])
	m.RelPath, _ = l.Rel(abs)
	if img, err := media.Decode(data); err == nil {
		prev := filepath.Join(l.SessionDir(sessionID), "previews", fmt.Sprintf("%s_%s_thumb.jpg", name, id))
		if th, err := media.EncodeJPEG(media.Fit(img, 420, 420), 80); err == nil && storage.WriteReplace(prev, th) == nil {
			m.ThumbRel, _ = l.Rel(prev)
		}
	}
	if err := s.insertMedia(m); err != nil {
		return Media{}, err
	}
	return m, nil
}

// SaveEdits stores a non-destructive recipe and renders it to edited/. The
// original is only read. A zero recipe reverts to the original.
func (s *Store) SaveEdits(mediaID string, e media.Edits) (Media, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.Media(mediaID)
	if err != nil {
		return Media{}, err
	}
	if m.Kind == "video" {
		return Media{}, errors.New("videos cannot be edited")
	}
	if err := e.Validate(); err != nil {
		return Media{}, err
	}
	l := s.db.Layout
	origAbs, err := l.Abs(m.RelPath)
	if err != nil {
		return Media{}, err
	}
	data, err := os.ReadFile(origAbs)
	if err != nil {
		return Media{}, fmt.Errorf("read original: %w", err)
	}
	src, err := media.Decode(data)
	if err != nil {
		return Media{}, err
	}
	oldEdited := m.EditedRel
	base := strings.TrimSuffix(filepath.Base(origAbs), filepath.Ext(origAbs))
	if e.IsZero() {
		m.EditedRel, m.Edits = "", ""
		_ = writeThumbOnly(l, &m, src, filepath.Join(l.SessionDir(m.SessionID), "previews", base+"_thumb.jpg"))
	} else {
		out, err := media.Apply(src, e)
		if err != nil {
			return Media{}, err
		}
		enc, err := media.EncodeJPEG(out, 92)
		if err != nil {
			return Media{}, err
		}
		editedAbs := filepath.Join(l.SessionDir(m.SessionID), "edited", fmt.Sprintf("%s_e%s.jpg", base, randHex(3)))
		if err := storage.WriteNew(editedAbs, enc); err != nil {
			return Media{}, err
		}
		m.EditedRel, _ = l.Rel(editedAbs)
		j, _ := json.Marshal(e)
		m.Edits = string(j)
		_ = writeThumbOnly(l, &m, out, filepath.Join(l.SessionDir(m.SessionID), "previews", base+"_thumb.jpg"))
	}
	if _, err := s.db.Exec(`UPDATE media SET edited_rel=?, edits_json=?, thumb_rel=? WHERE id=?`, m.EditedRel, m.Edits, m.ThumbRel, m.ID); err != nil {
		return Media{}, err
	}
	if oldEdited != "" && oldEdited != m.EditedRel { // only derived files are ever replaced
		if p, err := l.Abs(oldEdited); err == nil {
			_ = os.Remove(p)
		}
	}
	return m, nil
}

func writeThumbOnly(l storage.Layout, m *Media, img image.Image, abs string) error {
	th, err := media.EncodeJPEG(media.Fit(img, 420, 420), 80)
	if err != nil {
		return err
	}
	if err := storage.WriteReplace(abs, th); err != nil {
		return err
	}
	m.ThumbRel, _ = l.Rel(abs)
	return nil
}

// FilePath resolves which file to serve for a media id: kind is one of
// "original", "edited", "preview", "basepreview", "thumb".
func (s *Store) FilePath(id, kind string) (string, error) {
	m, err := s.Media(id)
	if err != nil {
		return "", err
	}
	if m.Status == MediaTrashed {
		return "", os.ErrNotExist
	}
	rel := m.RelPath
	switch kind {
	case "original":
	case "edited":
		if m.EditedRel != "" {
			rel = m.EditedRel
		}
	case "basepreview": // the unedited preview: the editor draws its edits on top of this
		if m.PreviewRel != "" {
			rel = m.PreviewRel
		}
	case "preview":
		switch {
		case m.EditedRel != "":
			rel = m.EditedRel
		case m.PreviewRel != "":
			rel = m.PreviewRel
		}
	case "thumb":
		if m.ThumbRel == "" {
			return "", os.ErrNotExist
		}
		rel = m.ThumbRel
	default:
		return "", fmt.Errorf("unknown file kind %q", kind)
	}
	return s.db.Layout.Abs(rel)
}

// Export copies the original (or edited render) into destDir without ever
// overwriting: an existing name gets a numeric suffix. Returns the new path.
func (s *Store) Export(id, destDir string, edited bool) (string, error) {
	m, err := s.Media(id)
	if err != nil {
		return "", err
	}
	kind := "original"
	if edited && m.EditedRel != "" {
		kind = "edited"
	}
	src, err := s.FilePath(id, kind)
	if err != nil {
		return "", err
	}
	if destDir == "" {
		destDir = filepath.Join(s.db.Layout.SessionDir(m.SessionID), "exports")
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	name := filepath.Base(src)
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 0; i < 1000; i++ {
		dst := filepath.Join(destDir, stem+ext)
		if i > 0 {
			dst = filepath.Join(destDir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		}
		err := storage.WriteNewFrom(dst, func(w io.Writer) error {
			f, err := os.Open(src)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = io.Copy(w, f)
			return err
		})
		if errors.Is(err, storage.ErrExists) {
			continue
		}
		return dst, err
	}
	return "", errors.New("could not find a free export file name")
}

// Trash moves a media item's files to trash/ and hides it. It is recoverable
// with Restore until EmptyTrash runs. The caller (UI) is responsible for
// obtaining the operator's confirmation first.
func (s *Store) Trash(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.Media(id)
	if err != nil {
		return err
	}
	l := s.db.Layout
	for _, rel := range []string{m.RelPath, m.PreviewRel, m.ThumbRel, m.EditedRel} {
		if rel == "" {
			continue
		}
		abs, err := l.Abs(rel)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err != nil {
			continue
		}
		dst := filepath.Join(l.TrashDir(), m.ID, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.Rename(abs, dst); err != nil {
			return fmt.Errorf("move %s to trash: %w", rel, err)
		}
	}
	_, err = s.db.Exec(`UPDATE media SET status=? WHERE id=?`, MediaTrashed, id)
	return err
}

// Restore moves trashed files back.
func (s *Store) Restore(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.Media(id)
	if err != nil {
		return err
	}
	l := s.db.Layout
	for _, rel := range []string{m.RelPath, m.PreviewRel, m.ThumbRel, m.EditedRel} {
		if rel == "" {
			continue
		}
		abs, err := l.Abs(rel)
		if err != nil {
			continue
		}
		src := filepath.Join(l.TrashDir(), m.ID, filepath.FromSlash(rel))
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := os.Rename(src, abs); err != nil {
			return err
		}
	}
	_, err = s.db.Exec(`UPDATE media SET status=? WHERE id=?`, MediaActive, id)
	return err
}

// EmptyTrash permanently deletes trashed media files and rows.
func (s *Store) EmptyTrash() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT id FROM media WHERE status=?`, MediaTrashed)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if err := os.RemoveAll(filepath.Join(s.db.Layout.TrashDir(), id)); err != nil {
			return 0, err
		}
		if _, err := s.db.Exec(`DELETE FROM media WHERE id=?`, id); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

// TrashSession trashes every media item of a session.
func (s *Store) TrashSession(id string) error {
	ms, err := s.ListMedia(id, 2000, 0)
	if err != nil {
		return err
	}
	for _, m := range ms {
		if err := s.Trash(m.ID); err != nil {
			return err
		}
	}
	return nil
}

// Report summarises an integrity scan.
type Report struct {
	Checked     int      `json:"checked"`
	Missing     []string `json:"missing"`     // media ids whose original is gone
	Recovered   []string `json:"recovered"`   // media ids re-registered from orphan originals
	Interrupted []string `json:"interrupted"` // sessions left active by a crash
	Corrupt     []string `json:"corrupt"`     // media ids whose original no longer matches its checksum
}

// Verify is run at startup (and from Diagnostics): it marks sessions left
// active by a crash as interrupted, flags media whose file vanished, and
// re-registers orphan originals found on disk so no capture is ever lost
// because a database write did not complete. Checksums are verified when deep.
func (s *Store) Verify(deep bool) (Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var rep Report
	l := s.db.Layout
	rows, err := s.db.Query(`SELECT id FROM sessions WHERE status=?`, StatusActive)
	if err != nil {
		return rep, err
	}
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		rep.Interrupted = append(rep.Interrupted, id)
	}
	rows.Close()
	for _, id := range rep.Interrupted {
		_, _ = s.db.Exec(`UPDATE sessions SET status=?, updated_at=? WHERE id=?`, StatusInterrupted, s.ts(), id)
	}

	rows, err = s.db.Query(`SELECT ` + mediaCols + ` FROM media WHERE status IN ('active','partial','missing')`)
	if err != nil {
		return rep, err
	}
	var all []Media
	known := map[string]bool{}
	for rows.Next() {
		m, err := scanMedia(rows)
		if err != nil {
			rows.Close()
			return rep, err
		}
		all = append(all, m)
	}
	rows.Close()
	for _, m := range all {
		rep.Checked++
		abs, err := l.Abs(m.RelPath)
		known[filepath.ToSlash(m.RelPath)] = true
		if err != nil {
			continue
		}
		st, err := os.Stat(abs)
		switch {
		case err != nil:
			rep.Missing = append(rep.Missing, m.ID)
			_, _ = s.db.Exec(`UPDATE media SET status=? WHERE id=?`, MediaMissing, m.ID)
		case m.Status == MediaMissing:
			_, _ = s.db.Exec(`UPDATE media SET status=? WHERE id=?`, MediaActive, m.ID)
		}
		if err == nil && deep && m.SHA256 != "" && m.Kind == "photo" {
			if data, err := os.ReadFile(abs); err == nil {
				sum := sha256.Sum256(data)
				if hex.EncodeToString(sum[:]) != m.SHA256 || st.Size() != m.SizeBytes {
					rep.Corrupt = append(rep.Corrupt, m.ID)
				}
			}
		}
	}
	// orphan originals: written to disk but the row never landed
	sdirs, _ := os.ReadDir(l.SessionsDir())
	for _, sd := range sdirs {
		if !sd.IsDir() {
			continue
		}
		var exists int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id=?`, sd.Name()).Scan(&exists); err != nil || exists == 0 {
			continue
		}
		files, _ := os.ReadDir(filepath.Join(l.SessionsDir(), sd.Name(), "originals"))
		for _, f := range files {
			if f.IsDir() || strings.HasPrefix(f.Name(), ".tmp-") {
				continue
			}
			abs := filepath.Join(l.SessionsDir(), sd.Name(), "originals", f.Name())
			rel, _ := l.Rel(abs)
			if known[rel] {
				continue
			}
			var n int
			_ = s.db.QueryRow(`SELECT COUNT(*) FROM media WHERE rel_path=?`, rel).Scan(&n)
			if n > 0 {
				continue
			}
			data, err := os.ReadFile(abs)
			if err != nil {
				continue
			}
			w, h, err := media.Dimensions(data)
			if err != nil {
				continue
			}
			sum := sha256.Sum256(data)
			id := "M" + randHex(6)
			m := Media{ID: id, SessionID: sd.Name(), Kind: "photo", RelPath: rel, Width: w, Height: h, SizeBytes: int64(len(data)),
				SHA256: hex.EncodeToString(sum[:]), Source: "recovered from disk", Status: MediaActive, CapturedAt: s.ts()}
			if img, err := media.Decode(data); err == nil {
				base := strings.TrimSuffix(f.Name(), filepath.Ext(f.Name()))
				pa := filepath.Join(l.SessionsDir(), sd.Name(), "previews", base+".jpg")
				ta := filepath.Join(l.SessionsDir(), sd.Name(), "previews", base+"_thumb.jpg")
				if writeDerived(img, pa, ta) == nil {
					m.PreviewRel, _ = l.Rel(pa)
					m.ThumbRel, _ = l.Rel(ta)
				}
			}
			if s.insertMedia(m) == nil {
				rep.Recovered = append(rep.Recovered, id)
			}
		}
	}
	return rep, nil
}
