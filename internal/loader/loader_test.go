package loader

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunRequestBudget(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if r.Method != "POST" || r.Header.Get("X-Test") != "1" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if b, _ := io.ReadAll(r.Body); string(b) != "hi" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if n%10 == 0 {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	r, err := Run(context.Background(), Config{
		Method: "post", URL: srv.URL, VUs: 7, Requests: 103, Body: "hi", Headers: map[string]string{"X-Test": "1"},
	}, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Total != 103 || hits.Load() != 103 {
		t.Fatalf("want exactly 103 requests, got result %d / server %d", r.Total, hits.Load())
	}
	if r.Failed != 10 || r.StatusCodes[500] != 10 || r.StatusCodes[200] != 93 {
		t.Fatalf("unexpected counts: failed=%d codes=%v", r.Failed, r.StatusCodes)
	}
	l := r.LatencyMs
	if !(l.Min <= l.P50 && l.P50 <= l.P95 && l.P95 <= l.P99 && l.P99 <= l.Max) || l.Max <= 0 {
		t.Fatalf("latency not ordered: %+v", l)
	}
	if r.Stopped {
		t.Fatal("finished run marked stopped")
	}
}

func TestRunDurationAndCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	ticks := 0
	start := time.Now()
	r, err := Run(ctx, Config{URL: srv.URL, VUs: 3, DurationSec: 30}, 50*time.Millisecond, func(Snapshot) { ticks++ })
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("cancel did not stop the run")
	}
	if !r.Stopped || r.Total == 0 || r.Failed != 0 || ticks == 0 {
		t.Fatalf("stopped=%v total=%d failed=%d ticks=%d", r.Stopped, r.Total, r.Failed, ticks)
	}
}

func TestValidate(t *testing.T) {
	bad := []Config{
		{URL: "example.com", VUs: 1, Requests: 1},
		{URL: "ftp://x", VUs: 1, Requests: 1},
		{URL: "http://x", VUs: 0, Requests: 1},
		{URL: "http://x", VUs: 1},
		{URL: "http://x", VUs: 1, Requests: 1, Method: "TRACE"},
		{URL: "http://x", Stages: []Stage{{DurationSec: 0, VUs: 5}}},
		{URL: "http://x", VUs: 1, Requests: 1, Thresholds: []Threshold{{Metric: "p42", Op: "<", Value: 1}}},
		{URL: "http://x", VUs: 1, Requests: 1, Thresholds: []Threshold{{Metric: "p95", Op: "==", Value: 1}}},
	}
	for _, c := range bad {
		if c.Validate() == nil {
			t.Errorf("expected error for %+v", c)
		}
	}
}

func TestStageVUs(t *testing.T) {
	st := []Stage{{DurationSec: 10, VUs: 100}, {DurationSec: 10, VUs: 100}, {DurationSec: 10, VUs: 0}}
	for _, c := range []struct {
		sec  float64
		want int
	}{{0, 0}, {5, 50}, {10, 100}, {15, 100}, {25, 50}, {30, 0}, {99, 0}} {
		if got := stageVUs(st, time.Duration(c.sec*float64(time.Second))); got != c.want {
			t.Errorf("t=%vs: got %d want %d", c.sec, got, c.want)
		}
	}
}

func TestRunStagesAndThresholds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	var peak int64
	r, err := Run(context.Background(), Config{
		URL:    srv.URL,
		Stages: []Stage{{DurationSec: 1, VUs: 4}, {DurationSec: 1, VUs: 0}},
		Thresholds: []Threshold{
			{Metric: "errorRate", Op: "<", Value: 1},
			{Metric: "p95", Op: "<", Value: 0}, // impossible, must fail
		},
	}, 50*time.Millisecond, func(s Snapshot) { peak = max(peak, s.ActiveVUs) })
	if err != nil {
		t.Fatal(err)
	}
	if r.Config.VUs != 4 || r.Config.DurationSec != 2 || peak < 3 || peak > 4 {
		t.Fatalf("vus=%d duration=%d peak=%d", r.Config.VUs, r.Config.DurationSec, peak)
	}
	if r.DurationSec < 1.9 || r.DurationSec > 3 {
		t.Fatalf("stages should run ~2s, ran %.2fs", r.DurationSec)
	}
	if r.Passed || len(r.Checks) != 2 || !r.Checks[0].Passed || r.Checks[1].Passed {
		t.Fatalf("checks = %+v passed=%v", r.Checks, r.Passed)
	}
}

func TestParseScenario(t *testing.T) {
	c, err := ParseScenario([]byte(`
name: smoke
url: http://localhost:8080/health
method: post
headers: {X-A: "1"}
stages:
  - {durationSec: 5, vus: 2}
thresholds:
  - {metric: p95, op: "<", value: 300}
`))
	if err != nil || c.Method != "POST" || c.VUs != 2 || c.DurationSec != 5 || c.Headers["X-A"] != "1" || len(c.Thresholds) != 1 {
		t.Fatalf("c=%+v err=%v", c, err)
	}
	if _, err := ParseScenario([]byte("url: http://x\nvus: 1\nrequests: 1\nduraton: 5\n")); err == nil {
		t.Fatal("unknown key should be rejected")
	}
	b, _ := MarshalScenario(c)
	if c2, err := ParseScenario(b); err != nil || c2.URL != c.URL || len(c2.Stages) != 1 {
		t.Fatalf("round trip: %+v %v", c2, err)
	}
}

// Each VU must be its own user: its own cookie session, its own stats, and think time between requests.
func TestVirtualUsersAreDistinct(t *testing.T) {
	var sessions atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("sid"); err != nil { // first request of a user: start a session
			http.SetCookie(w, &http.Cookie{Name: "sid", Value: "s" + strconv.FormatInt(sessions.Add(1), 10)})
		}
	}))
	defer srv.Close()

	start := time.Now()
	r, err := Run(context.Background(), Config{URL: srv.URL, VUs: 5, Requests: 40, ThinkMinMs: 20, ThinkMaxMs: 30}, 20*time.Millisecond, nil)
	if err != nil {
		t.Fatal(err)
	}
	if sessions.Load() != 5 {
		t.Fatalf("want one session per VU (5), server saw %d", sessions.Load())
	}
	var sum int64
	for i, v := range r.VUs {
		if v.ID != i+1 || v.Requests == 0 || v.AvgMs <= 0 {
			t.Fatalf("VU stat %+v", v)
		}
		sum += v.Requests
	}
	if len(r.VUs) != 5 || sum != 40 {
		t.Fatalf("per-VU requests sum to %d over %d VUs, want 40 over 5", sum, len(r.VUs))
	}
	// 40 requests over 5 VUs = 8 each, with >= 20ms think after each of the first 7
	if time.Since(start) < 140*time.Millisecond {
		t.Fatalf("think time not applied: run took %v", time.Since(start))
	}
	if len(r.Timeline) == 0 || r.Timeline[len(r.Timeline)-1].Total == 0 {
		t.Fatalf("timeline not recorded: %d points", len(r.Timeline))
	}
	if (&Config{URL: srv.URL, VUs: 1, Requests: 1, ThinkMinMs: 50, ThinkMaxMs: 10}).Validate() == nil {
		t.Fatal("think max below min must be rejected")
	}
}

func TestRunLog(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1)%2 == 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer srv.Close()

	var mu sync.Mutex
	var logs []LogEntry
	r, err := RunWithLog(context.Background(), Config{
		URL:        srv.URL + "/items?page=2",
		Stages:     []Stage{{DurationSec: 1, VUs: 3}, {DurationSec: 1, VUs: 3}},
		Thresholds: []Threshold{{Metric: "errorRate", Op: "<", Value: 1}},
	}, 100*time.Millisecond, nil, func(e LogEntry) { mu.Lock(); logs = append(logs, e); mu.Unlock() })
	if err != nil {
		t.Fatal(err)
	}
	var ok, bad, stages, requests int
	var sawFinish, sawThreshold bool
	for _, e := range logs {
		switch {
		case e.Kind == "request" && e.Status == 200 && e.Level == "ok" && e.VU >= 1 && e.Message == "GET /items?page=2":
			ok++
		case e.Kind == "request" && e.Status == 503 && e.Level == "error":
			bad++
		case strings.HasPrefix(e.Message, "Stage "):
			stages++
		case strings.HasPrefix(e.Message, "Finished: "):
			sawFinish = true
		case strings.HasPrefix(e.Message, "Threshold ") && e.Level == "error":
			sawThreshold = true
		}
		if e.Kind == "request" {
			requests++
		}
	}
	if ok == 0 || bad == 0 || stages != 2 || !sawFinish || !sawThreshold {
		t.Fatalf("ok=%d bad=%d stages=%d finish=%v threshold=%v\n%+v", ok, bad, stages, sawFinish, sawThreshold, logs)
	}
	// ~20 ticks x (6 ok + 6 error) is the ceiling; the run sent far more requests than that
	if int64(requests) >= r.Total || requests > 23*(logOKPerTick+logErrPerTick) {
		t.Fatalf("request log lines not sampled: %d lines for %d requests", requests, r.Total)
	}
}
