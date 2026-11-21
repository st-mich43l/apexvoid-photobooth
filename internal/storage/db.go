package storage

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite" // pure-Go driver: no CGO, cross-compiles to Windows
)

// DB is the SQLite metadata database.
type DB struct {
	*sql.DB
	Layout Layout
}

var migrations = []string{
	`CREATE TABLE sessions (
		id TEXT PRIMARY KEY,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		status TEXT NOT NULL,          -- active | completed | cancelled | failed | interrupted
		mode TEXT NOT NULL,            -- photo | video
		shots_planned INTEGER NOT NULL DEFAULT 1,
		template_id TEXT NOT NULL DEFAULT '',
		simulated INTEGER NOT NULL DEFAULT 0,
		note TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE media (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL REFERENCES sessions(id),
		kind TEXT NOT NULL,            -- photo | video | strip
		slot INTEGER NOT NULL DEFAULT 0,
		rel_path TEXT NOT NULL,
		thumb_rel TEXT NOT NULL DEFAULT '',
		preview_rel TEXT NOT NULL DEFAULT '',
		edited_rel TEXT NOT NULL DEFAULT '',
		edits_json TEXT NOT NULL DEFAULT '',
		width INTEGER NOT NULL DEFAULT 0,
		height INTEGER NOT NULL DEFAULT 0,
		size_bytes INTEGER NOT NULL DEFAULT 0,
		sha256 TEXT NOT NULL DEFAULT '',
		duration_ms INTEGER NOT NULL DEFAULT 0,
		simulated INTEGER NOT NULL DEFAULT 0,
		source TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'active',   -- active | discarded | trashed | missing | partial
		captured_at TEXT NOT NULL,
		created_at TEXT NOT NULL
	);
	CREATE INDEX media_session ON media(session_id, slot);
	CREATE INDEX media_status ON media(status, captured_at);`,
}

// Open opens (creating if needed) the library database and migrates it.
func Open(l Layout) (*DB, error) {
	if err := l.Ensure(); err != nil {
		return nil, err
	}
	dsn := "file:" + l.DatabasePath() + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	sdb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	sdb.SetMaxOpenConns(1)
	d := &DB{DB: sdb, Layout: l}
	if err := d.migrate(); err != nil {
		_ = sdb.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) migrate() error {
	var v int
	if err := d.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	if v > len(migrations) {
		return fmt.Errorf("database schema v%d is newer than this application (v%d)", v, len(migrations))
	}
	for i := v; i < len(migrations); i++ {
		tx, err := d.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version=%d`, i+1)); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Close checkpoints the WAL and closes the database.
func (d *DB) Close() error {
	_, _ = d.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	return d.DB.Close()
}

// Backup copies a consistent snapshot of the database to dst.
func (d *DB) Backup(dst string) error {
	_ = os.Remove(dst)
	_, err := d.Exec(`VACUUM INTO ?`, dst)
	return err
}
