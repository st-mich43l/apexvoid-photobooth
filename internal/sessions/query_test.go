package sessions

import (
	"fmt"
	"testing"
	"time"
)

func TestLargeSessionCataloguePaginationAndFilters(t *testing.T) {
	s, db := open(t, t.TempDir())
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 2001; i++ {
		id := fmt.Sprintf("S%05d", i)
		created := start.Add(time.Duration(i) * time.Minute).Format(time.RFC3339Nano)
		mode := "photo"
		if i%2 == 0 {
			mode = "video"
		}
		sim := i % 2
		if _, err = tx.Exec(`INSERT INTO sessions(id,created_at,updated_at,status,mode,simulated) VALUES(?,?,?,'completed',?,?)`, id, created, created, mode, sim); err != nil {
			t.Fatal(err)
		}
		for j := 0; j < 5; j++ {
			mid := fmt.Sprintf("M%05d-%d", i, j)
			kind, status, edited := "photo", "active", ""
			if j == 1 {
				status = "partial"
			}
			if j == 2 {
				status = "missing"
			}
			if j == 3 {
				edited = "synthetic/edited.jpg"
			}
			if j == 4 {
				kind = "strip"
				if i%2 != 0 {
					kind = "video"
				}
			}
			if _, err = tx.Exec(`INSERT INTO media(id,session_id,kind,rel_path,captured_at,created_at,status,edited_rel,source,frame_name) VALUES(?,?,?,?,?,?,?,?,?,?)`, mid, id, kind, "synthetic/"+mid, created, created, status, edited, fmt.Sprintf("camera-%d", i%3), fmt.Sprintf("frame-%d", i%4)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	first, err := s.QuerySessions(SessionQuery{Limit: 40})
	if err != nil || len(first.Items) != 40 || !first.HasMore {
		t.Fatalf("first page %v %+v", err, first)
	}
	if first.Items[0].ID != "S02000" || first.Items[0].PhotoCount != 4 || !first.Items[0].HasStrip {
		t.Fatalf("summary %+v", first.Items[0])
	}
	// Insertion ahead of the cursor does not duplicate or shift older pages.
	if _, err = db.Exec(`INSERT INTO sessions(id,created_at,updated_at,status,mode) VALUES('Snew','2027-01-01T00:00:00Z','2027-01-01T00:00:00Z','completed','photo')`); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	page := first
	total := 0
	for {
		for _, x := range page.Items {
			if seen[x.ID] {
				t.Fatalf("duplicate %s", x.ID)
			}
			seen[x.ID] = true
			total++
		}
		if !page.HasMore {
			break
		}
		page, err = s.QuerySessions(SessionQuery{Limit: 40, Cursor: page.NextCursor})
		if err != nil {
			t.Fatal(err)
		}
	}
	if total != 2001 || seen["Snew"] {
		t.Fatalf("cursor count %d", total)
	}
	if _, err = db.Exec(`UPDATE sessions SET note='Operator camera test' WHERE id='S00001'`); err != nil {
		t.Fatal(err)
	}
	tests, err := s.QuerySessions(SessionQuery{Simulated: "test"})
	if err != nil || len(tests.Items) != 1 || tests.Items[0].ID != "S00001" {
		t.Fatalf("test filter %+v %v", tests, err)
	}
	filtered, err := s.QuerySessions(SessionQuery{Search: "S00001", Mode: "photo", Simulated: "simulated", From: "2026-01-01", To: "2026-01-01"})
	if err != nil || len(filtered.Items) != 1 || filtered.Items[0].ID != "S00001" {
		t.Fatalf("filter %+v %v", filtered, err)
	}
	none, err := s.QuerySessions(SessionQuery{Search: "S00001", Mode: "video"})
	if err != nil || len(none.Items) != 0 {
		t.Fatalf("mode %+v %v", none, err)
	}
	mp, err := s.QuerySessionMedia("S00001", 2, 0)
	if err != nil || !mp.HasMore || len(mp.Items) != 2 {
		t.Fatalf("media page %+v %v", mp, err)
	}
	mp2, err := s.QuerySessionMedia("S00001", 2, mp.NextOffset)
	if err != nil || mp.Items[1].ID == mp2.Items[0].ID {
		t.Fatalf("media boundary %+v %v", mp2, err)
	}
}

func TestSessionCursorChronologicalFractionAndSameTimestamp(t *testing.T) {
	s, db := open(t, t.TempDir())
	for _, x := range []struct{ id, created string }{{"whole", "2026-01-01T00:00:00Z"}, {"fraction", "2026-01-01T00:00:00.1Z"}, {"fraction2", "2026-01-01T00:00:00.1Z"}} {
		if _, err := db.Exec(`INSERT INTO sessions(id,created_at,updated_at,status,mode) VALUES(?,?,?,'completed','photo')`, x.id, x.created, x.created); err != nil {
			t.Fatal(err)
		}
	}
	var ids []string
	cursor := ""
	for {
		p, err := s.QuerySessions(SessionQuery{Limit: 1, Cursor: cursor, From: "2026-01-01", To: "2026-01-01"})
		if err != nil {
			t.Fatal(err)
		}
		for _, x := range p.Items {
			ids = append(ids, x.ID)
		}
		if !p.HasMore {
			break
		}
		cursor = p.NextCursor
	}
	if fmt.Sprint(ids) != "[fraction2 fraction whole]" {
		t.Fatalf("order: %v", ids)
	}
}
