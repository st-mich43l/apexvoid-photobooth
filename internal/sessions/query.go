package sessions

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SessionQuery filters the session catalogue. Cursor is opaque and represents
// the last (created_at,id) pair; new sessions cannot shift older pages.
type SessionQuery struct {
	Search    string `json:"search"`
	Mode      string `json:"mode"`
	Status    string `json:"status"`
	Simulated string `json:"simulated"` // all, real, simulated, test
	From      string `json:"from"`      // YYYY-MM-DD, inclusive UTC
	To        string `json:"to"`        // YYYY-MM-DD, inclusive UTC
	Cursor    string `json:"cursor"`
	Limit     int    `json:"limit"`
}
type SessionCard struct {
	Session
	ThumbnailID      string `json:"thumbnailId"`
	ThumbnailVersion string `json:"thumbnailVersion"`
	PhotoCount       int    `json:"photoCount"`
	HasStrip         bool   `json:"hasStrip"`
}
type SessionPage struct {
	Items      []SessionCard `json:"items"`
	NextCursor string        `json:"nextCursor"`
	HasMore    bool          `json:"hasMore"`
}
type sessionCursor struct {
	Created string `json:"t"`
	ID      string `json:"i"`
}

// Session timestamps are UTC RFC3339Nano, whose optional fraction does not
// sort chronologically as raw text. Normalize to nine fractional digits.
const sessionCreatedKey = `(substr(p.created_at,1,19)||'.'||substr(CASE WHEN substr(p.created_at,20,1)='.' THEN substr(p.created_at,21,length(p.created_at)-21) ELSE '' END||'000000000',1,9))`

func (s *Store) QuerySessions(f SessionQuery) (SessionPage, error) {
	page := SessionPage{Items: []SessionCard{}}
	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 40
	}
	q := `SELECT p.*, COALESCE((SELECT m.id FROM media m WHERE m.session_id=p.id AND m.status IN ('active','partial') AND m.thumb_rel<>'' ORDER BY CASE WHEN m.kind IN ('strip','print') THEN 0 ELSE 1 END, m.captured_at DESC,m.id DESC LIMIT 1),''), COALESCE((SELECT m.thumb_rel FROM media m WHERE m.session_id=p.id AND m.status IN ('active','partial') AND m.thumb_rel<>'' ORDER BY CASE WHEN m.kind IN ('strip','print') THEN 0 ELSE 1 END, m.captured_at DESC,m.id DESC LIMIT 1),''), (SELECT COUNT(*) FROM media m WHERE m.session_id=p.id AND m.kind='photo' AND m.status IN ('active','partial','missing')), EXISTS(SELECT 1 FROM media m WHERE m.session_id=p.id AND m.kind IN ('strip','print') AND m.status IN ('active','partial')) FROM (` + sessionSelect + `) p WHERE 1=1`
	if f.Status != "trashed" {
		q += ` AND p.status<>'trashed'`
	}
	args := []any{}
	if f.Search != "" {
		q += ` AND instr(lower(p.id),lower(?))>0`
		args = append(args, strings.TrimSpace(f.Search))
	}
	if f.Mode != "" {
		if f.Mode != "photo" && f.Mode != "video" {
			return page, fmt.Errorf("invalid session mode")
		}
		q += ` AND p.mode=?`
		args = append(args, f.Mode)
	}
	if f.Status != "" {
		switch f.Status {
		case StatusActive, StatusCompleted, StatusCancelled, StatusFailed, StatusInterrupted, "trashing", "trashed":
		default:
			return page, fmt.Errorf("invalid session status")
		}
		q += ` AND p.status=?`
		args = append(args, f.Status)
	}
	switch f.Simulated {
	case "", "all":
	case "real":
		q += ` AND p.simulated=0`
	case "simulated":
		q += ` AND p.simulated=1`
	case "test":
		q += ` AND p.note='Operator camera test'`
	default:
		return page, fmt.Errorf("invalid simulation filter")
	}
	if f.From != "" {
		d, err := time.Parse("2006-01-02", f.From)
		if err != nil {
			return page, fmt.Errorf("invalid start date")
		}
		q += ` AND p.created_at>=?`
		args = append(args, d.UTC().Format("2006-01-02"))
	}
	if f.To != "" {
		d, err := time.Parse("2006-01-02", f.To)
		if err != nil {
			return page, fmt.Errorf("invalid end date")
		}
		q += ` AND p.created_at<?`
		args = append(args, d.AddDate(0, 0, 1).UTC().Format("2006-01-02"))
	}
	if f.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(f.Cursor)
		var c sessionCursor
		if err != nil || json.Unmarshal(raw, &c) != nil || c.Created == "" || c.ID == "" {
			return page, fmt.Errorf("invalid session cursor")
		}
		timestamp, err := time.Parse(time.RFC3339Nano, c.Created)
		if err != nil {
			return page, fmt.Errorf("invalid session cursor time")
		}
		key := timestamp.UTC().Format("2006-01-02T15:04:05.000000000")
		q += ` AND (` + sessionCreatedKey + `<? OR (` + sessionCreatedKey + `=? AND p.id<?))`
		args = append(args, key, key, c.ID)
	}
	q += ` ORDER BY ` + sessionCreatedKey + ` DESC,p.id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var x SessionCard
		var sim, strip int
		err := rows.Scan(&x.ID, &x.CreatedAt, &x.UpdatedAt, &x.Status, &x.Mode, &x.ShotsPlanned, &x.TemplateID, &sim, &x.Note, &x.FrameID, &x.FrameName, &x.VariantID, &x.FrameRev, &x.Caption, &x.FrameDir, &x.MediaCount, &x.ThumbnailID, &x.ThumbnailVersion, &x.PhotoCount, &strip)
		if err != nil {
			return page, err
		}
		x.Simulated = sim != 0
		x.HasStrip = strip != 0
		page.Items = append(page.Items, x)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) > limit {
		page.HasMore = true
		page.Items = page.Items[:limit]
		last := page.Items[len(page.Items)-1]
		raw, _ := json.Marshal(sessionCursor{last.CreatedAt, last.ID})
		page.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return page, nil
}

type MediaPage struct {
	Items      []Media `json:"items"`
	NextOffset int     `json:"nextOffset"`
	HasMore    bool    `json:"hasMore"`
}

// QuerySessionMedia uses deterministic ordering and bounded offset pages. A
// session normally stops changing once complete; refresh after editing it.
func (s *Store) QuerySessionMedia(id string, limit, offset int) (MediaPage, error) {
	page := MediaPage{Items: []Media{}}
	if id == "" {
		return page, fmt.Errorf("session ID is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		return page, fmt.Errorf("invalid offset")
	}
	rows, err := s.db.Query(`SELECT `+mediaCols+` FROM media WHERE session_id=? AND status IN ('active','partial','missing') ORDER BY CASE WHEN kind='photo' THEN 0 ELSE 1 END, CASE WHEN kind='photo' THEN slot ELSE 0 END,captured_at,id LIMIT ? OFFSET ?`, id, limit+1, offset)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		m, err := scanMedia(rows)
		if err != nil {
			return page, err
		}
		page.Items = append(page.Items, m)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	if len(page.Items) > limit {
		page.HasMore = true
		page.Items = page.Items[:limit]
	}
	page.NextOffset = offset + len(page.Items)
	return page, nil
}
