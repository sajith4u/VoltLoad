package reports

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"voltload/internal/loader"
)

func sample() loader.Result {
	return loader.Result{
		Config:    loader.Config{Name: "<script>x</script>", Method: "GET", URL: "http://x", VUs: 3},
		StartedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Total: 10, Success: 9, Failed: 1, RPS: 5,
		LatencyMs:   loader.Latency{P95: 12.345},
		StatusCodes: map[int]int64{503: 1, 200: 9},
		Checks:      []loader.CheckResult{{Threshold: loader.Threshold{Metric: "p95", Op: "<", Value: 10}, Actual: 12.345}},
	}
}

func TestFormats(t *testing.T) {
	var b bytes.Buffer
	if err := Write(&b, "html", sample()); err != nil {
		t.Fatal(err)
	}
	html := b.String()
	if strings.Contains(html, "<script>x") || !strings.Contains(html, "Thresholds: FAIL") || !strings.Contains(html, "HTTP 503") {
		t.Fatal("html report missing content or not escaped")
	}

	b.Reset()
	if err := Write(&b, "csv", sample()); err != nil {
		t.Fatal(err)
	}
	csv := b.String()
	if !strings.Contains(csv, "latency_p95_ms,12.35") || strings.Index(csv, "status_200") > strings.Index(csv, "status_503") {
		t.Fatalf("csv:\n%s", csv)
	}

	b.Reset()
	full := sample()
	full.Config.Stages = []loader.Stage{{DurationSec: 5, VUs: 3}}
	full.Config.Headers = map[string]string{"Authorization": "Bearer secret-token"}
	full.Errors = map[string]int{"dial tcp: connection refused – ünïcode": 1}
	for i := 0; i < 120; i++ { // enough rows to force a page break
		full.VUs = append(full.VUs, loader.VUStat{ID: i + 1, Requests: 3, AvgMs: 10})
		full.Timeline = append(full.Timeline, loader.Snapshot{ElapsedSec: float64(i), RPS: float64(i % 7), AvgLatencyMs: 12, ActiveVUs: 3})
	}
	if err := Write(&b, "pdf", full); err != nil {
		t.Fatal(err)
	}
	if pdf := b.Bytes(); !bytes.HasPrefix(pdf, []byte("%PDF-")) || bytes.Count(pdf, []byte("/Type /Page\n")) < 2 || bytes.Contains(pdf, []byte("secret-token")) {
		t.Fatalf("pdf: %d bytes, want a multi-page PDF that does not leak header values", len(pdf))
	}

	path := filepath.Join(t.TempDir(), "r.json")
	if err := WriteFile(path, sample()); err != nil {
		t.Fatal(err)
	}
	if r, err := ReadJSON(path); err != nil || r.Total != 10 || r.StatusCodes[503] != 1 {
		t.Fatalf("json round trip: %+v %v", r, err)
	}
	if WriteFile(filepath.Join(t.TempDir(), "r.txt"), sample()) == nil {
		t.Fatal("unknown extension should fail")
	}
}
