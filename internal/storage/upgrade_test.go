package storage

import "testing"

// A library made by the previous release (schema v2: no frame columns) must
// open, keep every row and gain the frame columns with empty values.
func TestDatabaseUpgradesFromVersion2KeepingSessionsAndMedia(t *testing.T) {
	root := t.TempDir()
	db, err := Open(Layout{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{"frame_id", "frame_name", "variant_id", "frame_rev", "caption", "frame_dir"} {
		if _, err := db.Exec(`ALTER TABLE sessions DROP COLUMN ` + c); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []string{"frame_id", "frame_name", "variant_id", "frame_rev", "frame_dir"} {
		if _, err := db.Exec(`ALTER TABLE media DROP COLUMN ` + c); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`DROP INDEX print_jobs_parent`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP INDEX print_jobs_media`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE print_jobs DROP COLUMN image_dpi`); err != nil {
		t.Fatal(err)
	}
	mustExec := func(q string) {
		t.Helper()
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	mustExec(`INSERT INTO sessions(id,created_at,updated_at,status,mode,shots_planned,template_id) VALUES('S1','t','t','completed','photo',4,'strip-2x6-4')`)
	mustExec(`INSERT INTO media(id,session_id,kind,rel_path,captured_at,created_at) VALUES('M1','S1','strip','sessions/S1/exports/a.jpg','t','t')`)
	mustExec(`PRAGMA user_version=2`)
	_ = db.Close()

	db2, err := Open(Layout{Root: root})
	if err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}
	defer db2.Close()
	var tpl, frame, dir string
	if err := db2.QueryRow(`SELECT template_id,frame_id,frame_dir FROM sessions WHERE id='S1'`).Scan(&tpl, &frame, &dir); err != nil || tpl != "strip-2x6-4" || frame != "" || dir != "" {
		t.Fatalf("session after upgrade: %v %q %q %q", err, tpl, frame, dir)
	}
	var rev int
	if err := db2.QueryRow(`SELECT frame_rev FROM media WHERE id='M1'`).Scan(&rev); err != nil || rev != 0 {
		t.Fatalf("media after upgrade: %v %d", err, rev)
	}
	var v int
	_ = db2.QueryRow(`PRAGMA user_version`).Scan(&v)
	if v != len(migrations) {
		t.Fatalf("schema version %d, want %d", v, len(migrations))
	}
}
