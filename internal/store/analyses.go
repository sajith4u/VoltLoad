package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"voltload/internal/webperf"
)

// AnalysisSummary is one row of the frontend-analysis history.
type AnalysisSummary struct {
	ID        int64   `json:"id"`
	URL       string  `json:"url"`
	StartedAt string  `json:"startedAt"`
	LCPMs     float64 `json:"lcpMs"`
	LoadMs    float64 `json:"loadMs"`
	Issues    int     `json:"issues"` // critical + warning recommendations
	React     bool    `json:"react"`
}

func (s *Store) SaveAnalysis(a webperf.Analysis) (int64, error) {
	b, err := json.Marshal(a)
	if err != nil {
		return 0, err
	}
	issues := 0
	for _, r := range a.Recommendations {
		if r.Severity != "info" {
			issues++
		}
	}
	res, err := s.db.Exec(`INSERT INTO web_analyses (url, started_at, lcp_ms, load_ms, issues, react, result_json) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.URL, time.Now().UTC(), a.First.LCPMs, a.First.LoadMs, issues, a.React.Detected, string(b))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListAnalyses(limit int) ([]AnalysisSummary, error) {
	rows, err := s.db.Query(`SELECT id, url, started_at, lcp_ms, load_ms, issues, react FROM web_analyses ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AnalysisSummary{}
	for rows.Next() {
		var a AnalysisSummary
		var started sql.NullTime
		if err := rows.Scan(&a.ID, &a.URL, &started, &a.LCPMs, &a.LoadMs, &a.Issues, &a.React); err != nil {
			return nil, err
		}
		a.StartedAt = started.Time.Format(timeFmt)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetAnalysis(id int64) (webperf.Analysis, error) {
	var a webperf.Analysis
	var raw string
	if err := s.db.QueryRow(`SELECT result_json FROM web_analyses WHERE id = ?`, id).Scan(&raw); err != nil {
		return a, fmt.Errorf("analysis %d: %w", id, err)
	}
	err := json.Unmarshal([]byte(raw), &a)
	a.ID = id
	return a, err
}

func (s *Store) DeleteAnalysis(id int64) error {
	_, err := s.db.Exec(`DELETE FROM web_analyses WHERE id = ?`, id)
	return err
}
