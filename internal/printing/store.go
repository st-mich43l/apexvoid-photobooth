package printing

import (
	"database/sql"
	"errors"
	"time"

	"github.com/st-mich43l/apexvoid-photobooth/internal/storage"
)

// Store persists print jobs.
type Store interface {
	Insert(j Job) error
	Update(j Job) error
	Get(id string) (Job, error)
	ByRequest(requestID string) (Job, bool, error)
	List(limit int) ([]Job, error)
	Active() ([]Job, error)
	CountReprints(root string) (int, error)
	HasPendingReprint(root string) (bool, error)
	HasPrint(mediaID string) (bool, error)
	// Recent finds a non-failed job for the same media/printer/paper/copies/kind created after since.
	Recent(j Job, since time.Time) (Job, bool, error)
}

// SQLStore keeps jobs in the library database (table print_jobs).
type SQLStore struct{ db *storage.DB }

// NewSQLStore wraps an open library database.
func NewSQLStore(db *storage.DB) *SQLStore { return &SQLStore{db: db} }

const jobCols = `id,request_id,parent_id,kind,media_id,rendered_media_id,session_id,template_id,file,printer,paper_name,paper_id,orientation,fit,copies,status,detail,native_id,attempt,created_at,updated_at,image_dpi`

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func (s *SQLStore) Insert(j Job) error {
	_, err := s.db.Exec(`INSERT INTO print_jobs(`+jobCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		j.ID, j.RequestID, j.ParentID, j.Kind, j.MediaID, j.RenderedMediaID, j.SessionID, j.TemplateID, j.File, j.Printer, j.PaperName, j.PaperID,
		j.Orientation, j.Fit, j.Copies, j.Status, j.Detail, j.NativeID, j.Attempt, ts(j.CreatedAt), ts(j.UpdatedAt), j.ImageDPI)
	return err
}

func (s *SQLStore) Update(j Job) error {
	res, err := s.db.Exec(`UPDATE print_jobs SET status=?, detail=?, native_id=?, rendered_media_id=?, updated_at=? WHERE id=?`,
		j.Status, j.Detail, j.NativeID, j.RenderedMediaID, ts(j.UpdatedAt), j.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

type rowScanner interface{ Scan(...any) error }

func scanJob(r rowScanner) (Job, error) {
	var j Job
	var c, u string
	err := r.Scan(&j.ID, &j.RequestID, &j.ParentID, &j.Kind, &j.MediaID, &j.RenderedMediaID, &j.SessionID, &j.TemplateID, &j.File, &j.Printer,
		&j.PaperName, &j.PaperID, &j.Orientation, &j.Fit, &j.Copies, &j.Status, &j.Detail, &j.NativeID, &j.Attempt, &c, &u, &j.ImageDPI)
	j.CreatedAt, _ = time.Parse(time.RFC3339Nano, c)
	j.UpdatedAt, _ = time.Parse(time.RFC3339Nano, u)
	return j, err
}

func (s *SQLStore) CountReprints(root string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM print_jobs WHERE parent_id=?`, root).Scan(&n)
	return n, err
}

func (s *SQLStore) HasPendingReprint(root string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM print_jobs WHERE parent_id=? AND status IN ('submitting','queued','printing','unknown')`, root).Scan(&n)
	return n > 0, err
}

func (s *SQLStore) HasPrint(mediaID string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM print_jobs WHERE media_id=? AND kind='print' AND (status<>'failed' OR native_id<>'')`, mediaID).Scan(&n)
	return n > 0, err
}

func (s *SQLStore) Get(id string) (Job, error) {
	return scanJob(s.db.QueryRow(`SELECT `+jobCols+` FROM print_jobs WHERE id=?`, id))
}

func (s *SQLStore) ByRequest(req string) (Job, bool, error) {
	if req == "" {
		return Job{}, false, nil
	}
	j, err := scanJob(s.db.QueryRow(`SELECT `+jobCols+` FROM print_jobs WHERE request_id=? ORDER BY created_at LIMIT 1`, req))
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, nil
	}
	return j, err == nil, err
}

func (s *SQLStore) query(q string, args ...any) ([]Job, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (s *SQLStore) List(limit int) ([]Job, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	return s.query(`SELECT `+jobCols+` FROM print_jobs ORDER BY created_at DESC LIMIT ?`, limit)
}

// ListSession returns a deterministic bounded page of a session's print history.
func (s *SQLStore) ListSession(id string, limit, offset int) ([]Job, error) {
	if id == "" || offset < 0 {
		return nil, errors.New("invalid session or offset")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.query(`SELECT `+jobCols+` FROM print_jobs WHERE session_id=? ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?`, id, limit, offset)
}

func (s *SQLStore) Active() ([]Job, error) {
	return s.query(`SELECT ` + jobCols + ` FROM print_jobs WHERE status IN ('submitting','queued','printing') ORDER BY created_at`)
}

func (s *SQLStore) Recent(j Job, since time.Time) (Job, bool, error) {
	out, err := s.query(`SELECT `+jobCols+` FROM print_jobs WHERE kind=? AND media_id=? AND printer=? AND paper_name=? AND copies=? AND parent_id='' AND status<>'failed' AND (status IN ('submitting','unknown') OR created_at>=?) ORDER BY created_at DESC LIMIT 1`,
		j.Kind, j.MediaID, j.Printer, j.PaperName, j.Copies, ts(since))
	if err != nil || len(out) == 0 {
		return Job{}, false, err
	}
	return out[0], true, nil
}
