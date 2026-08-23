// Package store records what sessions exist on this machine.
//
// tmux is the only registry cbx had before, so everything it knew about a
// session died with the tmux server. That is not hypothetical: restarting the
// server to pick up a new group took the master session's Remote Control URL
// with it, and nothing could recover it.
//
// SQLite rather than a JSON file because two callers overlap — the master
// Claude session and a person over SSH can both run cbx at once, and a
// read-modify-write over JSON is a lost update.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Session is one Claude Code session cbx started.
type Session struct {
	Name      string
	Dir       string
	Repo      string
	RCURL     string
	CreatedAt time.Time
	// Running is not stored — it is reconciled against tmux at read time,
	// because a row can outlive the process it describes.
	Running bool
}

type Store struct{ db *sql.DB }

// DefaultPath is where the database lives for the current user. State, not
// config: this is generated data cbx can rebuild, so it follows XDG_STATE_HOME.
func DefaultPath() string {
	if s := os.Getenv("XDG_STATE_HOME"); s != "" {
		return filepath.Join(s, "cbx", "sessions.db")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "cbx", "sessions.db")
	}
	return filepath.Join(home, ".local", "state", "cbx", "sessions.db")
}

// Open creates the database if it does not exist.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	// The pragmas go in the DSN, not an Exec. database/sql pools connections
	// and a PRAGMA set via Exec applies only to whichever connection served
	// it — the others still fail instantly on contention. busy_timeout makes a
	// blocked writer wait instead of returning SQLITE_BUSY; WAL lets readers
	// proceed during a write.
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS sessions (
		name       TEXT PRIMARY KEY,
		dir        TEXT NOT NULL,
		repo       TEXT NOT NULL DEFAULT '',
		rc_url     TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL
	)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Put records a session, replacing any earlier row with the same name. A name
// is reused when a session is killed and recreated, and the new row is the
// truth.
func (s *Store) Put(sess Session) error {
	if sess.Name == "" {
		return fmt.Errorf("session name is required")
	}
	created := sess.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	_, err := s.db.Exec(
		`INSERT INTO sessions (name, dir, repo, rc_url, created_at) VALUES (?,?,?,?,?)
		 ON CONFLICT(name) DO UPDATE SET dir=excluded.dir, repo=excluded.repo,
		   rc_url=excluded.rc_url, created_at=excluded.created_at`,
		sess.Name, sess.Dir, sess.Repo, sess.RCURL, created.Unix())
	if err != nil {
		return fmt.Errorf("record session %q: %w", sess.Name, err)
	}
	return nil
}

// Get returns a session by name. Absent is (nil, nil), not an error — callers
// routinely ask about a name that may not exist.
func (s *Store) Get(name string) (*Session, error) {
	var sess Session
	var created int64
	err := s.db.QueryRow(
		`SELECT name, dir, repo, rc_url, created_at FROM sessions WHERE name = ?`, name).
		Scan(&sess.Name, &sess.Dir, &sess.Repo, &sess.RCURL, &created)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read session %q: %w", name, err)
	}
	sess.CreatedAt = time.Unix(created, 0)
	return &sess, nil
}

// List returns every recorded session, oldest first.
func (s *Store) List() ([]Session, error) {
	rows, err := s.db.Query(`SELECT name, dir, repo, rc_url, created_at FROM sessions ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		var sess Session
		var created int64
		if err := rows.Scan(&sess.Name, &sess.Dir, &sess.Repo, &sess.RCURL, &created); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sess.CreatedAt = time.Unix(created, 0)
		out = append(out, sess)
	}
	return out, rows.Err()
}

// Delete forgets a session. Deleting one that was never recorded is not an
// error: the caller's intent is that it be gone.
func (s *Store) Delete(name string) error {
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE name = ?`, name); err != nil {
		return fmt.Errorf("delete session %q: %w", name, err)
	}
	return nil
}
