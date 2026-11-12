package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"voltload/internal/loader"
	"voltload/internal/webperf"
)

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	in := loader.Result{
		Config:    loader.Config{Name: "smoke", Method: "GET", URL: "http://x", VUs: 2, Requests: 5},
		StartedAt: time.Now(), Total: 5, Failed: 1, RPS: 12.5,
		LatencyMs:   loader.Latency{P95: 42},
		StatusCodes: map[int]int64{200: 4, 503: 1},
		Checks:      []loader.CheckResult{{Passed: false}},
	}
	sid, err := s.SaveSuite(Suite{Name: " smoke ", Description: "d", Config: in.Config})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveSuite(Suite{Name: "bad", Config: loader.Config{URL: "nope"}}); err == nil {
		t.Fatal("invalid config must be rejected")
	}
	id, err := s.SaveRun(sid, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveRun(0, in); err != nil { // suite-less run must not show up under the suite
		t.Fatal(err)
	}
	su, err := s.GetSuite(sid)
	if err != nil || su.Name != "smoke" || su.Config.URL != "http://x" {
		t.Fatalf("suite = %+v, err = %v", su, err)
	}
	su.Name = "renamed"
	if _, err := s.SaveSuite(su); err != nil {
		t.Fatal(err)
	}
	cards, err := s.ListSuites(30)
	if err != nil || len(cards) != 1 || cards[0].Name != "renamed" || len(cards[0].Recent) != 1 || cards[0].Recent[0].OK {
		t.Fatalf("cards = %+v, err = %v", cards, err)
	}
	list, err := s.ListRuns(sid, 10)
	if err != nil || len(list) != 1 || list[0].ID != id || list[0].P95Ms != 42 || list[0].Name != "smoke" || list[0].Passed == nil || *list[0].Passed {
		t.Fatalf("list = %+v, err = %v", list, err)
	}
	got, err := s.GetRun(id)
	if err != nil || got.StatusCodes[503] != 1 || got.Config.VUs != 2 {
		t.Fatalf("get = %+v, err = %v", got, err)
	}
	s.Close()

	// reopening must not re-run migrations
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.DeleteRun(id); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListRuns(sid, 10); len(list) != 0 {
		t.Fatalf("expected empty after delete, got %d", len(list))
	}
	s.SaveRun(sid, in)
	if err := s.DeleteSuite(sid); err != nil {
		t.Fatal(err)
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM runs WHERE suite_id = ?`, sid).Scan(&n)
	if cards, _ := s.ListSuites(30); len(cards) != 0 || n != 0 {
		t.Fatalf("suite delete left %d cards, %d runs", len(cards), n)
	}
}

// A database created by schema v1 must upgrade in place and keep its rows.
func TestMigrateFromV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	saved := migrations
	migrations = migrations[:1]
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO runs (name, method, url, started_at, total, failed, rps, p95_ms, result_json)
		VALUES ('old', 'GET', 'http://x', CURRENT_TIMESTAMP, 1, 0, 1, 1, '{}')`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	migrations = saved

	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var name string
	var suite sql.NullInt64
	if err := s.db.QueryRow(`SELECT name, suite_id FROM runs`).Scan(&name, &suite); err != nil || name != "old" || suite.Valid {
		t.Fatalf("old row: name=%q suite=%v err=%v", name, suite, err)
	}
}

func TestAnalysesRoundTrip(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := webperf.Analysis{URL: "https://example.com/", React: webperf.ReactInfo{Detected: true}, First: webperf.Load{LCPMs: 2100, LoadMs: 3000},
		Recommendations: []webperf.Recommendation{{Severity: "critical"}, {Severity: "warning"}, {Severity: "info"}}}
	id, err := s.SaveAnalysis(a)
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.ListAnalyses(10)
	if err != nil || len(list) != 1 || list[0].Issues != 2 || !list[0].React || list[0].LCPMs != 2100 {
		t.Fatalf("list: %+v %v", list, err)
	}
	got, err := s.GetAnalysis(id)
	if err != nil || got.ID != id || got.URL != a.URL || len(got.Recommendations) != 3 {
		t.Fatalf("get: %+v %v", got, err)
	}
	if err := s.DeleteAnalysis(id); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListAnalyses(10); len(list) != 0 {
		t.Fatalf("not deleted: %+v", list)
	}
}
