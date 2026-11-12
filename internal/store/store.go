// Package store keeps test history in an embedded SQLite database.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"voltload/internal/loader"

	_ "modernc.org/sqlite" // pure Go, so no cgo/toolchain needed for cross-platform builds
)

// migrations[i] upgrades the schema from version i to i+1. Only append.
var migrations = []string{
	`CREATE TABLE runs (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		name        TEXT NOT NULL,
		method      TEXT NOT NULL,
		url         TEXT NOT NULL,
		started_at  TIMESTAMP NOT NULL,
		total       INTEGER NOT NULL,
		failed      INTEGER NOT NULL,
		rps         REAL NOT NULL,
		p95_ms      REAL NOT NULL,
		result_json TEXT NOT NULL
	)`,
	// NULL = run had no thresholds
	`ALTER TABLE runs ADD COLUMN passed INTEGER`,
	`CREATE TABLE suites (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		name        TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		config_json TEXT NOT NULL,
		created_at  TIMESTAMP NOT NULL,
		updated_at  TIMESTAMP NOT NULL
	)`,
	// NULL = run made before suites existed, or from the CLI
	`ALTER TABLE runs ADD COLUMN suite_id INTEGER`,
	`ALTER TABLE runs ADD COLUMN avg_ms REAL NOT NULL DEFAULT 0`,
	`ALTER TABLE runs ADD COLUMN duration_sec REAL NOT NULL DEFAULT 0`,
	`CREATE INDEX runs_suite ON runs (suite_id, id)`,
	// the first-run account is admin / admin (bcrypt); the user changes it from the app
	`CREATE TABLE users (username TEXT PRIMARY KEY, pass_hash TEXT NOT NULL);
	INSERT INTO users VALUES ('admin', '$2a$10$YEMiGs9WhEvsHI1mjrMOTuLyNZgdo.GEgvPzNqJeYL6GsrDo.eEQm')`,
	// frontend analyses (internal/webperf); summary columns for the list, the full result as JSON
	`CREATE TABLE web_analyses (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		url         TEXT NOT NULL,
		started_at  TIMESTAMP NOT NULL,
		lcp_ms      REAL NOT NULL,
		load_ms     REAL NOT NULL,
		issues      INTEGER NOT NULL,
		react       INTEGER NOT NULL,
		result_json TEXT NOT NULL
	)`,
}

type Store struct{ db *sql.DB }

// RunSummary is one row of the history list.
type RunSummary struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Method      string  `json:"method"`
	URL         string  `json:"url"`
	StartedAt   string  `json:"startedAt"`
	Total       int64   `json:"total"`
	Failed      int64   `json:"failed"`
	RPS         float64 `json:"rps"`
	P95Ms       float64 `json:"p95Ms"`
	Passed      *bool   `json:"passed"` // nil when the run had no thresholds
	AvgMs       float64 `json:"avgMs"`
	DurationSec float64 `json:"durationSec"`
	OK          bool    `json:"ok"` // thresholds passed, or with no thresholds, no failed requests
}

// Suite is a saved, re-runnable test definition. Its name is also the config's name.
type Suite struct {
	ID          int64         `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Config      loader.Config `json:"config"`
	UpdatedAt   string        `json:"updatedAt"`
}

// SuiteSummary is one dashboard card: the suite plus its most recent runs, newest first.
type SuiteSummary struct {
	ID          int64        `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Method      string       `json:"method"`
	URL         string       `json:"url"`
	UpdatedAt   string       `json:"updatedAt"`
	Recent      []RunSummary `json:"recent"`
}

const timeFmt = "2006-01-02T15:04:05Z07:00"

// SaveSuite validates and stores the suite, creating it when ID is 0. Returns its ID.
func (s *Store) SaveSuite(su Suite) (int64, error) {
	su.Name = strings.TrimSpace(su.Name)
	if su.Name == "" {
		return 0, errors.New("suite name is required")
	}
	su.Config.Name = su.Name
	if err := su.Config.Validate(); err != nil {
		return 0, err
	}
	b, err := json.Marshal(su.Config)
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	if su.ID == 0 {
		res, err := s.db.Exec(`INSERT INTO suites (name, description, config_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			su.Name, su.Description, string(b), now, now)
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	res, err := s.db.Exec(`UPDATE suites SET name = ?, description = ?, config_json = ?, updated_at = ? WHERE id = ?`,
		su.Name, su.Description, string(b), now, su.ID)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, fmt.Errorf("suite %d not found", su.ID)
	}
	return su.ID, nil
}

func (s *Store) GetSuite(id int64) (Suite, error) {
	su := Suite{ID: id}
	var raw string
	var updated sql.NullTime
	err := s.db.QueryRow(`SELECT name, description, config_json, updated_at FROM suites WHERE id = ?`, id).
		Scan(&su.Name, &su.Description, &raw, &updated)
	if err != nil {
		return su, fmt.Errorf("suite %d: %w", id, err)
	}
	su.UpdatedAt = updated.Time.Format(timeFmt)
	return su, json.Unmarshal([]byte(raw), &su.Config)
}

// ListSuites returns every suite, most recently updated first, each with up to recent runs.
func (s *Store) ListSuites(recent int) ([]SuiteSummary, error) {
	rows, err := s.db.Query(`SELECT id, name, description, config_json, updated_at FROM suites ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	out := []SuiteSummary{}
	for rows.Next() {
		var su SuiteSummary
		var raw string
		var updated sql.NullTime
		var cfg loader.Config
		if err := rows.Scan(&su.ID, &su.Name, &su.Description, &raw, &updated); err != nil {
			rows.Close()
			return nil, err
		}
		json.Unmarshal([]byte(raw), &cfg) // a corrupt config only blanks the card's method/url
		su.Method, su.URL, su.UpdatedAt = cfg.Method, cfg.URL, updated.Time.Format(timeFmt)
		out = append(out, su)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Recent, err = s.ListRuns(out[i].ID, recent); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// DeleteSuite removes the suite and all of its runs.
func (s *Store) DeleteSuite(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM runs WHERE suite_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM suites WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// DataDir is the per-user app data directory: ~/.local/share/voltload on Linux, %APPDATA%\VoltLoad on Windows.
func DataDir() (string, error) {
	if runtime.GOOS == "linux" {
		if d := os.Getenv("XDG_DATA_HOME"); d != "" {
			return filepath.Join(d, "voltload"), nil
		}
		home, err := os.UserHomeDir()
		return filepath.Join(home, ".local", "share", "voltload"), err
	}
	d, err := os.UserConfigDir()
	return filepath.Join(d, "VoltLoad"), err
}

// Open creates the database file if needed and applies pending migrations.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // single writer; avoids SQLITE_BUSY between our own connections
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func migrate(db *sql.DB) error {
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	if v > len(migrations) {
		return fmt.Errorf("database schema v%d is newer than this app (v%d)", v, len(migrations))
	}
	for ; v < len(migrations); v++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[v]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", v+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, v+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

// SaveRun stores a finished run. suiteID 0 means the run belongs to no suite.
func (s *Store) SaveRun(suiteID int64, r loader.Result) (int64, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return 0, err
	}
	var passed *bool
	if len(r.Checks) > 0 {
		passed = &r.Passed
	}
	var suite *int64
	if suiteID != 0 {
		suite = &suiteID
	}
	res, err := s.db.Exec(`INSERT INTO runs (suite_id, name, method, url, started_at, total, failed, rps, p95_ms, avg_ms, duration_sec, passed, result_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		suite, r.Config.Name, r.Config.Method, r.Config.URL, r.StartedAt.UTC(), r.Total, r.Failed, r.RPS,
		r.LatencyMs.P95, r.LatencyMs.Avg, r.DurationSec, passed, string(b))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListRuns returns a suite's runs, newest first.
func (s *Store) ListRuns(suiteID int64, limit int) ([]RunSummary, error) {
	rows, err := s.db.Query(`SELECT id, name, method, url, started_at, total, failed, rps, p95_ms, avg_ms, duration_sec, passed
		FROM runs WHERE suite_id = ? ORDER BY id DESC LIMIT ?`, suiteID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RunSummary{}
	for rows.Next() {
		var r RunSummary
		var started sql.NullTime
		var passed sql.NullBool
		if err := rows.Scan(&r.ID, &r.Name, &r.Method, &r.URL, &started, &r.Total, &r.Failed, &r.RPS, &r.P95Ms, &r.AvgMs, &r.DurationSec, &passed); err != nil {
			return nil, err
		}
		r.OK = r.Failed == 0
		if passed.Valid {
			r.Passed = &passed.Bool
			r.OK = passed.Bool
		}
		r.StartedAt = started.Time.Format(timeFmt)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetRun(id int64) (loader.Result, error) {
	var raw string
	var r loader.Result
	if err := s.db.QueryRow(`SELECT result_json FROM runs WHERE id = ?`, id).Scan(&raw); err != nil {
		return r, err
	}
	return r, json.Unmarshal([]byte(raw), &r)
}

func (s *Store) DeleteRun(id int64) error {
	_, err := s.db.Exec(`DELETE FROM runs WHERE id = ?`, id)
	return err
}
