package gui

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"
)

// startServer runs Serve on a random loopback port with a temp data dir and returns its URL.
func startServer(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	urls := make(chan string, 1)
	done := make(chan error, 1)
	assets := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>VoltLoad</title>")}}
	go func() { done <- Serve(ctx, assets, "127.0.0.1:0", t.TempDir(), func(u string) { urls <- u }) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("serve: %v", err)
		}
	})
	select {
	case u := <-urls:
		return u
	case err := <-done:
		t.Fatalf("serve failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start")
	}
	return ""
}

// rpc calls an App method over HTTP and decodes the result into out (may be nil).
func rpc(t *testing.T, base, method string, out any, args ...any) (status int, errMsg string) {
	t.Helper()
	if args == nil {
		args = []any{}
	}
	body, _ := json.Marshal(args)
	resp, err := http.Post(base+"/api/call/"+method, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var env struct {
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&env)
	if out != nil && env.Error == "" {
		if err := json.Unmarshal(env.Result, out); err != nil {
			t.Fatalf("%s result: %v", method, err)
		}
	}
	return resp.StatusCode, env.Error
}

func TestServeEndToEnd(t *testing.T) {
	var hits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		io.WriteString(w, "ok")
	}))
	defer target.Close()
	base := startServer(t)

	// the UI is served
	if resp, err := http.Get(base + "/"); err != nil || resp.StatusCode != 200 {
		t.Fatalf("index: %v %v", resp, err)
	}

	// open the event stream before running, like the frontend does
	req, _ := http.NewRequest("GET", base+"/api/events", nil)
	stream, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	events := make(chan string, 10000)
	go func() {
		sc := bufio.NewScanner(stream.Body)
		for sc.Scan() {
			if name, ok := strings.CutPrefix(sc.Text(), "event: "); ok {
				events <- name
			}
		}
		close(events)
	}()

	// create a suite, then run it
	var id int64
	suite := map[string]any{"id": 0, "name": "E2E suite", "description": "", "config": map[string]any{
		"method": "GET", "url": target.URL + "/health", "vus": 3, "durationSec": 0, "requests": 30, "timeoutMs": 5000,
		"thresholds": []map[string]any{{"metric": "errorRate", "op": "<", "value": 1}},
	}}
	if _, e := rpc(t, base, "SaveSuite", &id, suite); e != "" || id == 0 {
		t.Fatalf("SaveSuite: id=%d err=%q", id, e)
	}
	if _, e := rpc(t, base, "RunSuite", nil, id); e != "" {
		t.Fatalf("RunSuite: %s", e)
	}

	seen := map[string]int{}
	timeout := time.After(15 * time.Second)
wait:
	for {
		select {
		case name, ok := <-events:
			if !ok {
				t.Fatal("event stream closed early")
			}
			seen[name]++
			if name == EventDone {
				break wait
			}
		case <-timeout:
			t.Fatalf("no %s event; saw %v", EventDone, seen)
		}
	}
	if seen[EventLog] == 0 {
		t.Fatalf("no log events streamed: %v", seen)
	}
	if hits.Load() != 30 {
		t.Fatalf("target got %d requests, want 30", hits.Load())
	}

	// the run is in history with a passing verdict, and its reports render
	var runs []struct {
		ID    int64 `json:"id"`
		Total int64 `json:"total"`
		OK    bool  `json:"ok"`
	}
	if _, e := rpc(t, base, "ListRuns", &runs, id); e != "" || len(runs) != 1 || runs[0].Total != 30 || !runs[0].OK {
		t.Fatalf("ListRuns: %+v %q", runs, e)
	}
	for format, prefix := range map[string]string{"pdf": "%PDF-", "json": "{", "csv": "metric,value", "html": "<!doctype html>"} {
		resp, err := http.Get(base + "/api/runs/" + itoa(runs[0].ID) + "/report." + format + "?download=1")
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !bytes.HasPrefix(b, []byte(prefix)) || !strings.HasPrefix(resp.Header.Get("Content-Disposition"), "attachment") {
			t.Fatalf("%s report: status %d, starts %q", format, resp.StatusCode, b[:min(len(b), 20)])
		}
	}

	// YAML round trip used by browser import/export
	var yamlText string
	var cfg map[string]any
	rpc(t, base, "ScenarioYAML", &yamlText, suite["config"])
	if _, e := rpc(t, base, "ParseScenario", &cfg, yamlText); e != "" || cfg["url"] != target.URL+"/health" {
		t.Fatalf("yaml round trip: %v %q\n%s", cfg, e, yamlText)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestServeRefusesUnsafeRequests(t *testing.T) {
	base := startServer(t)
	post := func(path, contentType, host, origin string) int {
		req, _ := http.NewRequest("POST", base+path, strings.NewReader("[]"))
		req.Header.Set("Content-Type", contentType)
		if host != "" {
			req.Host = host
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	cases := []struct {
		name                      string
		path, ctype, host, origin string
		want                      int
	}{
		{"ok", "/api/call/ListSuites", "application/json", "", "", 200},
		{"cross-site simple request", "/api/call/ListSuites", "text/plain", "", "", http.StatusUnsupportedMediaType},
		{"DNS rebinding host", "/api/call/ListSuites", "application/json", "evil.example:7878", "", http.StatusForbidden},
		{"foreign origin", "/api/call/ListSuites", "application/json", "", "https://evil.example", http.StatusForbidden},
		{"desktop-only method", "/api/call/OpenRunPDF", "application/json", "", "", http.StatusNotFound},
		{"unexported method", "/api/call/close", "application/json", "", "", http.StatusNotFound},
	}
	for _, c := range cases {
		if got := post(c.path, c.ctype, c.host, c.origin); got != c.want {
			t.Errorf("%s: status %d, want %d", c.name, got, c.want)
		}
	}
	if status, e := rpc(t, base, "GetSuite", nil, "not-a-number"); status != 400 || e == "" {
		t.Errorf("bad argument: status %d err %q", status, e)
	}
	if err := Serve(context.Background(), fstest.MapFS{}, "0.0.0.0:0", t.TempDir(), nil); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Errorf("non-loopback listen must be refused, got %v", err)
	}
}
