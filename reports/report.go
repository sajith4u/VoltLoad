// Package reports renders a loader.Result as JSON, CSV, a standalone HTML page, or a full-detail PDF.
package reports

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"voltload/internal/loader"
)

var Formats = []string{"json", "csv", "html", "pdf"}

// FormatFromPath returns the format implied by a file extension, or "" if unknown.
func FormatFromPath(path string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if ext == "htm" {
		ext = "html"
	}
	for _, f := range Formats {
		if f == ext {
			return f
		}
	}
	return ""
}

func Write(w io.Writer, format string, r loader.Result) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	case "csv":
		return writeCSV(w, r)
	case "html":
		return page.Execute(w, r)
	case "pdf":
		return writePDF(w, r)
	}
	return fmt.Errorf("unknown report format %q (use json, csv, html or pdf)", format)
}

// WriteFile writes the report in the format implied by path's extension.
func WriteFile(path string, r loader.Result) error {
	format := FormatFromPath(path)
	if format == "" {
		return fmt.Errorf("can't tell report format from %q; use .json, .csv, .html or .pdf", path)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := Write(f, format, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// ReadJSON loads a result previously written as a JSON report.
func ReadJSON(path string) (loader.Result, error) {
	var r loader.Result
	b, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	return r, json.Unmarshal(b, &r)
}

func f2(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }

func writeCSV(w io.Writer, r loader.Result) error {
	l := r.LatencyMs
	rows := [][]string{
		{"metric", "value"},
		{"name", r.Config.Name},
		{"method", r.Config.Method},
		{"url", r.Config.URL},
		{"started_at", r.StartedAt.Format("2006-01-02T15:04:05Z07:00")},
		{"vus", strconv.Itoa(r.Config.VUs)},
		{"duration_sec", f2(r.DurationSec)},
		{"total", strconv.FormatInt(r.Total, 10)},
		{"success", strconv.FormatInt(r.Success, 10)},
		{"failed", strconv.FormatInt(r.Failed, 10)},
		{"rps", f2(r.RPS)},
		{"latency_min_ms", f2(l.Min)}, {"latency_avg_ms", f2(l.Avg)}, {"latency_p50_ms", f2(l.P50)},
		{"latency_p90_ms", f2(l.P90)}, {"latency_p95_ms", f2(l.P95)}, {"latency_p99_ms", f2(l.P99)},
		{"latency_max_ms", f2(l.Max)},
		{"thresholds_passed", strconv.FormatBool(r.Passed)},
	}
	for _, c := range StatusCodes(r) {
		rows = append(rows, []string{"status_" + strconv.Itoa(c.Code), strconv.FormatInt(c.Count, 10)})
	}
	for _, c := range r.Checks {
		t := c.Threshold
		rows = append(rows, []string{fmt.Sprintf("threshold %s %s %g", t.Metric, t.Op, t.Value), fmt.Sprintf("%s (actual %s)", passFail(c.Passed), f2(c.Actual))})
	}
	cw := csv.NewWriter(w)
	cw.WriteAll(rows)
	return cw.Error()
}

type CodeCount struct {
	Code  int
	Count int64
}

// StatusCodes returns the status code counts sorted by code.
func StatusCodes(r loader.Result) []CodeCount {
	out := make([]CodeCount, 0, len(r.StatusCodes))
	for c, n := range r.StatusCodes {
		out = append(out, CodeCount{c, n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

func passFail(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}

var page = template.Must(template.New("report").Funcs(template.FuncMap{
	"f2": f2, "codes": StatusCodes, "passFail": passFail,
	"errRate": func(r loader.Result) string {
		if r.Total == 0 {
			return "0.00"
		}
		return f2(float64(r.Failed) / float64(r.Total) * 100)
	},
	"date": date,
}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>VoltLoad report – {{.Config.Name}}</title>
<style>
:root{--bg:#fff;--fg:#111827;--muted:#6b7280;--line:#e5e7eb;--bad:#b91c1c;--good:#15803d;--tile:#f9fafb}
@media (prefers-color-scheme:dark){:root{--bg:#111827;--fg:#f3f4f6;--muted:#9ca3af;--line:#374151;--bad:#f87171;--good:#4ade80;--tile:#1f2937}}
body{margin:0 auto;max-width:960px;padding:24px 16px;background:var(--bg);color:var(--fg);font:14px/1.5 system-ui,sans-serif}
h1{margin:0}.muted{color:var(--muted)}
.tiles{display:grid;grid-template-columns:repeat(auto-fit,minmax(140px,1fr));gap:12px;margin:20px 0}
.tile{background:var(--tile);border-radius:6px;padding:12px}.tile span{display:block;color:var(--muted);font-size:12px}.tile b{font-size:20px}
table{width:100%;border-collapse:collapse;margin-bottom:20px;font-variant-numeric:tabular-nums}
th,td{text-align:left;padding:6px 8px;border-bottom:1px solid var(--line)}th{color:var(--muted);font-weight:500}
.bad{color:var(--bad)}.good{color:var(--good)}
</style></head><body>
<h1>{{if .Config.Name}}{{.Config.Name}}{{else}}{{.Config.Method}} {{.Config.URL}}{{end}}</h1>
<p class="muted">{{.Config.Method}} {{.Config.URL}} · {{.Config.VUs}} VUs · {{date .}}{{if .Stopped}} · stopped early{{end}}</p>
{{if .Checks}}<p><b class="{{if .Passed}}good{{else}}bad{{end}}">Thresholds: {{passFail .Passed}}</b></p>{{end}}
<div class="tiles">
<div class="tile"><span>Requests</span><b>{{.Total}}</b></div>
<div class="tile"><span>Success</span><b>{{.Success}}</b></div>
<div class="tile"><span>Failed</span><b{{if .Failed}} class="bad"{{end}}>{{.Failed}} ({{errRate .}}%)</b></div>
<div class="tile"><span>Req/s</span><b>{{f2 .RPS}}</b></div>
<div class="tile"><span>Duration</span><b>{{f2 .DurationSec}} s</b></div>
</div>
<h2>Latency (ms)</h2>
<table><tr><th>min</th><th>avg</th><th>p50</th><th>p90</th><th>p95</th><th>p99</th><th>max</th></tr>
<tr>{{with .LatencyMs}}<td>{{f2 .Min}}</td><td>{{f2 .Avg}}</td><td>{{f2 .P50}}</td><td>{{f2 .P90}}</td><td>{{f2 .P95}}</td><td>{{f2 .P99}}</td><td>{{f2 .Max}}</td>{{end}}</tr></table>
{{if .Checks}}<h2>Thresholds</h2><table><tr><th>Rule</th><th>Actual</th><th>Result</th></tr>
{{range .Checks}}<tr><td>{{.Threshold.Metric}} {{.Threshold.Op}} {{.Threshold.Value}}</td><td>{{f2 .Actual}}</td><td class="{{if .Passed}}good{{else}}bad{{end}}">{{passFail .Passed}}</td></tr>{{end}}</table>{{end}}
{{with codes .}}<h2>Status codes</h2><table>{{range .}}<tr{{if ge .Code 400}} class="bad"{{end}}><td>HTTP {{.Code}}</td><td>{{.Count}}</td></tr>{{end}}</table>{{end}}
{{if .Errors}}<h2>Errors</h2><table>{{range $e, $n := .Errors}}<tr class="bad"><td>{{$e}}</td><td>{{$n}}</td></tr>{{end}}</table>{{end}}
{{if .Config.Stages}}<h2>Stages</h2><table><tr><th>Duration (s)</th><th>Target VUs</th></tr>{{range .Config.Stages}}<tr><td>{{.DurationSec}}</td><td>{{.VUs}}</td></tr>{{end}}</table>{{end}}
<p class="muted">Generated by VoltLoad</p>
</body></html>
`))
