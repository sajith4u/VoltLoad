package loader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Config describes one HTTP load test. It is also the YAML scenario format shared by the CLI and desktop app,
// and the JSON tags match the frontend bindings.
type Config struct {
	Name        string            `json:"name" yaml:"name,omitempty"`
	Method      string            `json:"method" yaml:"method,omitempty"`
	URL         string            `json:"url" yaml:"url"`
	Headers     map[string]string `json:"headers" yaml:"headers,omitempty"`
	Body        string            `json:"body" yaml:"body,omitempty"`
	VUs         int               `json:"vus" yaml:"vus,omitempty"`                 // concurrent virtual users; with stages, set to the peak
	DurationSec int               `json:"durationSec" yaml:"durationSec,omitempty"` // stop after this long (0 = no limit); with stages, set to their total
	Requests    int               `json:"requests" yaml:"requests,omitempty"`       // stop after this many requests (0 = no limit)
	TimeoutMs   int               `json:"timeoutMs" yaml:"timeoutMs,omitempty"`     // per-request timeout (0 = 30s)
	ThinkMinMs  int               `json:"thinkMinMs" yaml:"thinkMinMs,omitempty"`   // each VU pauses a random time between
	ThinkMaxMs  int               `json:"thinkMaxMs" yaml:"thinkMaxMs,omitempty"`   // ThinkMinMs and ThinkMaxMs after every request (0 = none)
	Stages      []Stage           `json:"stages" yaml:"stages,omitempty"`           // optional ramp profile; replaces constant VUs
	Thresholds  []Threshold       `json:"thresholds" yaml:"thresholds,omitempty"`   // pass/fail criteria checked at the end
}

// Stage linearly ramps from the previous stage's VUs (0 before the first) to VUs over DurationSec.
type Stage struct {
	DurationSec int `json:"durationSec" yaml:"durationSec"`
	VUs         int `json:"vus" yaml:"vus"`
}

// Threshold is a pass/fail rule such as {p95 < 500}. Latency metrics are in ms, errorRate in percent.
type Threshold struct {
	Metric string  `json:"metric" yaml:"metric"` // avg, p50, p90, p95, p99, max, errorRate, rps
	Op     string  `json:"op" yaml:"op"`         // <, <=, >, >=
	Value  float64 `json:"value" yaml:"value"`
}

type CheckResult struct {
	Threshold Threshold `json:"threshold"`
	Actual    float64   `json:"actual"`
	Passed    bool      `json:"passed"`
}

// Snapshot is the live progress sent while a test runs.
type Snapshot struct {
	ElapsedSec   float64 `json:"elapsedSec"`
	Total        int64   `json:"total"`
	Success      int64   `json:"success"`
	Failed       int64   `json:"failed"`
	RPS          float64 `json:"rps"` // over the last tick
	AvgLatencyMs float64 `json:"avgLatencyMs"`
	ActiveVUs    int64   `json:"activeVUs"`
}

// VUStat is one virtual user's share of the run.
type VUStat struct {
	ID       int     `json:"id"` // 1-based
	Requests int64   `json:"requests"`
	Failed   int64   `json:"failed"`
	AvgMs    float64 `json:"avgMs"`
}

// LogEntry is one line of the live run log. Request lines carry Status/LatencyMs; engine lines have VU 0.
type LogEntry struct {
	ElapsedSec float64 `json:"elapsedSec"`
	Level      string  `json:"level"` // info, ok, warn, error
	Kind       string  `json:"kind"`  // system or request
	VU         int     `json:"vu"`    // 1-based virtual user, 0 for the engine itself
	Message    string  `json:"message"`
	Status     int     `json:"status"`    // HTTP status of a request line, 0 if it never got a response
	LatencyMs  float64 `json:"latencyMs"` // of a request line
}

// Line formats the entry as one plain-text line, e.g. "  1.02s  OK     VU 3     GET /  200  15.5ms".
func (e LogEntry) Line() string {
	who := "engine"
	if e.VU > 0 {
		who = fmt.Sprintf("VU %d", e.VU)
	}
	detail := ""
	if e.Kind == "request" && e.Status > 0 {
		detail = fmt.Sprintf("  %d  %.1fms", e.Status, e.LatencyMs)
	}
	return fmt.Sprintf("%7.2fs  %-5s  %-7s  %s%s", e.ElapsedSec, strings.ToUpper(e.Level), who, e.Message, detail)
}

// Request lines are sampled so a fast test can't flood the UI: at most this many per tick, per kind.
const logOKPerTick, logErrPerTick = 6, 6

type Latency struct {
	Min float64 `json:"min"`
	Avg float64 `json:"avg"`
	P50 float64 `json:"p50"`
	P90 float64 `json:"p90"`
	P95 float64 `json:"p95"`
	P99 float64 `json:"p99"`
	Max float64 `json:"max"`
}

// Result is the final outcome of a test; latencies are in milliseconds.
type Result struct {
	Config      Config         `json:"config"`
	StartedAt   time.Time      `json:"startedAt"`
	DurationSec float64        `json:"durationSec"`
	Total       int64          `json:"total"`
	Success     int64          `json:"success"`
	Failed      int64          `json:"failed"`
	RPS         float64        `json:"rps"`
	LatencyMs   Latency        `json:"latencyMs"`
	StatusCodes map[int]int64  `json:"statusCodes"`
	Errors      map[string]int `json:"errors"`
	Stopped     bool           `json:"stopped"`  // cancelled by the user before its limit
	VUs         []VUStat       `json:"vus"`      // what each virtual user did, by VU number
	Timeline    []Snapshot     `json:"timeline"` // progress samples over the run, downsampled to at most ~600 points
	Checks      []CheckResult  `json:"checks"`
	Passed      bool           `json:"passed"` // all thresholds passed (true when there are none)
}

var methods = map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true, "HEAD": true, "OPTIONS": true}

// Validate normalises cfg and rejects anything the engine can't run.
func (c *Config) Validate() error {
	c.Method = strings.ToUpper(strings.TrimSpace(c.Method))
	if c.Method == "" {
		c.Method = "GET"
	}
	if !methods[c.Method] {
		return fmt.Errorf("unsupported method %q", c.Method)
	}
	u, err := url.Parse(strings.TrimSpace(c.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("url must be an absolute http:// or https:// URL")
	}
	c.URL = u.String()
	if len(c.Stages) > 0 {
		c.VUs, c.DurationSec = 0, 0
		for i, st := range c.Stages {
			if st.DurationSec < 1 || st.VUs < 0 || st.VUs > 10000 {
				return fmt.Errorf("stage %d: duration must be >= 1s and VUs between 0 and 10000", i+1)
			}
			c.VUs = max(c.VUs, st.VUs)
			c.DurationSec += st.DurationSec
		}
	}
	if c.VUs < 1 || c.VUs > 10000 {
		return errors.New("virtual users must be between 1 and 10000")
	}
	if c.DurationSec < 0 || c.Requests < 0 || c.TimeoutMs < 0 {
		return errors.New("duration, requests and timeout cannot be negative")
	}
	if c.ThinkMinMs < 0 || c.ThinkMaxMs < c.ThinkMinMs {
		return errors.New("think time: min cannot be negative and max cannot be below min")
	}
	for i, t := range c.Thresholds {
		if _, ok := metricOf[t.Metric]; !ok {
			return fmt.Errorf("threshold %d: unknown metric %q (use avg, p50, p90, p95, p99, max, errorRate, rps)", i+1, t.Metric)
		}
		if _, ok := ops[t.Op]; !ok {
			return fmt.Errorf("threshold %d: unknown operator %q (use <, <=, >, >=)", i+1, t.Op)
		}
	}
	if c.DurationSec == 0 && c.Requests == 0 {
		return errors.New("set a duration, a request count, or both")
	}
	if c.TimeoutMs == 0 {
		c.TimeoutMs = 30000
	}
	return nil
}

type collector struct {
	mu        sync.Mutex
	latencies []float64 // shortcut: keeps every sample in memory (~8 bytes each); switch to a histogram for multi-million-request runs
	codes     map[int]int64
	errs      map[string]int
	success   int64
	failed    int64
	sumMs     float64
	vus       []VUStat // index = VU number - 1; AvgMs holds the latency sum until the run ends
}

func (c *collector) add(vu int, ms float64, code int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.latencies = append(c.latencies, ms)
	c.sumMs += ms
	c.vus[vu].Requests++
	c.vus[vu].AvgMs += ms
	if err != nil || code >= 400 {
		c.vus[vu].Failed++
	}
	switch {
	case err != nil:
		c.failed++
		if len(c.errs) < 50 || c.errs[err.Error()] > 0 { // cap distinct error strings
			c.errs[err.Error()]++
		}
	case code >= 400:
		c.failed++
		c.codes[code]++
	default:
		c.success++
		c.codes[code]++
	}
}

func (c *collector) snapshot() (total, success, failed int64, avg float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	total = c.success + c.failed
	if total > 0 {
		avg = c.sumMs / float64(total)
	}
	return total, c.success, c.failed, avg
}

// Run executes the test until its duration or request budget is used up, or ctx is cancelled.
// onTick (may be nil) is called every tick with live progress.
func Run(ctx context.Context, cfg Config, tick time.Duration, onTick func(Snapshot)) (Result, error) {
	return RunWithLog(ctx, cfg, tick, onTick, nil)
}

// RunWithLog is Run plus a live log: onLog (may be nil) receives engine events and a sample of requests.
// It is called from many goroutines at once.
func RunWithLog(ctx context.Context, cfg Config, tick time.Duration, onTick func(Snapshot), onLog func(LogEntry)) (Result, error) {
	if err := cfg.Validate(); err != nil {
		return Result{}, err
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if cfg.DurationSec > 0 {
		runCtx, cancel = context.WithTimeout(runCtx, time.Duration(cfg.DurationSec)*time.Second)
		defer cancel()
	}

	col := &collector{codes: map[int]int64{}, errs: map[string]int{}, vus: make([]VUStat, cfg.VUs)}
	for i := range col.vus {
		col.vus[i].ID = i + 1
	}
	var budget atomic.Int64
	budget.Store(int64(cfg.Requests))
	start := time.Now()

	var okLeft, errLeft atomic.Int64 // request log lines still allowed this tick
	okLeft.Store(logOKPerTick)
	errLeft.Store(logErrPerTick)
	emit := func(e LogEntry) {
		if onLog != nil {
			e.ElapsedSec = time.Since(start).Seconds()
			onLog(e)
		}
	}
	say := func(level, format string, args ...any) {
		emit(LogEntry{Level: level, Kind: "system", Message: fmt.Sprintf(format, args...)})
	}
	target := cfg.Method + " " + requestPath(cfg.URL)
	logRequest := func(vu int, ms float64, code int, err error) {
		if onLog == nil {
			return
		}
		e := LogEntry{Kind: "request", VU: vu + 1, Status: code, LatencyMs: ms, Level: "ok", Message: target}
		left := &okLeft
		if err != nil {
			e.Level, e.Message, left = "error", target+" failed: "+err.Error(), &errLeft
		} else if code >= 400 {
			e.Level, left = "error", &errLeft
		}
		if left.Add(-1) >= 0 {
			emit(e)
		}
	}
	say("info", "Starting %s", cfg.Method+" "+cfg.URL)
	if len(cfg.Stages) == 0 {
		say("info", "%d virtual users started, each with its own connection and session", cfg.VUs)
	}

	// VU i sends requests only while i < active; stages move active up and down over time.
	var active atomic.Int64
	active.Store(int64(cfg.VUs))
	if len(cfg.Stages) > 0 {
		active.Store(0)
		go func() {
			t := time.NewTicker(100 * time.Millisecond)
			defer t.Stop()
			stage := -1
			for {
				elapsed := time.Since(start)
				if i, from := stageAt(cfg.Stages, elapsed); i != stage && i < len(cfg.Stages) {
					stage = i
					st := cfg.Stages[i]
					verb := fmt.Sprintf("ramping %d → %d virtual users over", from, st.VUs)
					if from == st.VUs {
						verb = fmt.Sprintf("holding %d virtual users for", st.VUs)
					}
					say("info", "Stage %d/%d: %s %ds", i+1, len(cfg.Stages), verb, st.DurationSec)
				}
				active.Store(int64(stageVUs(cfg.Stages, elapsed)))
				select {
				case <-runCtx.Done():
					return
				case <-t.C:
				}
			}
		}()
	}

	var wg sync.WaitGroup
	for i := 0; i < cfg.VUs; i++ {
		wg.Add(1)
		go func(i int64) {
			defer wg.Done()
			// Each VU is its own user: a private connection and cookie jar, so sessions and
			// keep-alive connections are never shared between VUs. Goroutines, not OS threads:
			// the Go scheduler spreads them over all CPU cores and they cost ~KBs, not MBs.
			jar, _ := cookiejar.New(nil) // never errors with nil options
			client := &http.Client{
				Timeout:   time.Duration(cfg.TimeoutMs) * time.Millisecond,
				Jar:       jar,
				Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, MaxIdleConnsPerHost: 1},
			}
			defer client.CloseIdleConnections()
			for runCtx.Err() == nil {
				if i >= active.Load() {
					// shortcut: idle VUs poll every 50ms; fine up to the 10k VU cap, use a cond/broadcast if that grows
					select {
					case <-runCtx.Done():
					case <-time.After(50 * time.Millisecond):
					}
					continue
				}
				if cfg.Requests > 0 && budget.Add(-1) < 0 {
					return
				}
				doRequest(runCtx, client, cfg, col, int(i), logRequest)
				if cfg.ThinkMaxMs > 0 {
					think := cfg.ThinkMinMs + rand.IntN(cfg.ThinkMaxMs-cfg.ThinkMinMs+1)
					select {
					case <-runCtx.Done():
					case <-time.After(time.Duration(think) * time.Millisecond):
					}
				}
			}
		}(int64(i))
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	if tick <= 0 {
		tick = 500 * time.Millisecond
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	var lastTotal int64
	lastTime := start
	var timeline []Snapshot
	stride, ticks := 1, 0 // keep every stride-th tick; stride doubles whenever the timeline fills up
loop:
	for {
		select {
		case <-done:
			break loop
		case now := <-ticker.C:
			total, s, f, avg := col.snapshot()
			snap := Snapshot{
				ElapsedSec: now.Sub(start).Seconds(), Total: total, Success: s, Failed: f,
				RPS: float64(total-lastTotal) / now.Sub(lastTime).Seconds(), AvgLatencyMs: avg,
				ActiveVUs: active.Load(),
			}
			lastTotal, lastTime = total, now
			okLeft.Store(logOKPerTick)
			errLeft.Store(logErrPerTick)
			if ticks++; ticks%stride == 0 {
				if timeline = append(timeline, snap); len(timeline) >= 600 {
					for i := 0; i < len(timeline)/2; i++ {
						timeline[i] = timeline[2*i+1]
					}
					timeline, stride = timeline[:len(timeline)/2], stride*2
				}
			}
			if onTick != nil {
				onTick(snap)
			}
		}
	}

	elapsed := time.Since(start).Seconds()
	for i := range col.vus {
		if n := col.vus[i].Requests; n > 0 {
			col.vus[i].AvgMs /= float64(n)
		}
	}
	res := Result{
		VUs: col.vus, Timeline: timeline,
		Config: cfg, StartedAt: start, DurationSec: elapsed,
		Success: col.success, Failed: col.failed, Total: col.success + col.failed,
		LatencyMs: summarize(col.latencies), StatusCodes: col.codes, Errors: col.errs,
		Stopped: ctx.Err() != nil,
	}
	if elapsed > 0 {
		res.RPS = float64(res.Total) / elapsed
	}
	res.Checks, res.Passed = evaluate(cfg.Thresholds, res)

	if res.Stopped {
		say("warn", "Stopped before the planned end")
	}
	level := "ok"
	if res.Failed > 0 {
		level = "warn"
	}
	say(level, "Finished: %d requests in %.1fs · %.1f req/s · %d failed", res.Total, elapsed, res.RPS, res.Failed)
	for _, c := range res.Checks {
		verdict, lvl := "PASS", "ok"
		if !c.Passed {
			verdict, lvl = "FAIL", "error"
		}
		say(lvl, "Threshold %s %s %g: %s (actual %.2f)", c.Threshold.Metric, c.Threshold.Op, c.Threshold.Value, verdict, c.Actual)
	}
	return res, nil
}

// requestPath is the path and query of rawURL, for compact log lines.
func requestPath(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.RequestURI() == "" {
		return "/"
	}
	return u.RequestURI()
}

// stageAt returns the index of the stage active at elapsed time t and the VU count it starts from.
// The index equals len(stages) once every stage is over.
func stageAt(stages []Stage, t time.Duration) (index, from int) {
	for i, st := range stages {
		d := time.Duration(st.DurationSec) * time.Second
		if t < d {
			return i, from
		}
		t -= d
		from = st.VUs
	}
	return len(stages), from
}

// stageVUs is the target VU count at elapsed time t, interpolating linearly within each stage.
func stageVUs(stages []Stage, t time.Duration) int {
	from := 0
	for _, st := range stages {
		d := time.Duration(st.DurationSec) * time.Second
		if t < d {
			return from + int(math.Round(float64(st.VUs-from)*float64(t)/float64(d)))
		}
		t -= d
		from = st.VUs
	}
	return from
}

var metricOf = map[string]func(Result) float64{
	"avg": func(r Result) float64 { return r.LatencyMs.Avg },
	"p50": func(r Result) float64 { return r.LatencyMs.P50 },
	"p90": func(r Result) float64 { return r.LatencyMs.P90 },
	"p95": func(r Result) float64 { return r.LatencyMs.P95 },
	"p99": func(r Result) float64 { return r.LatencyMs.P99 },
	"max": func(r Result) float64 { return r.LatencyMs.Max },
	"rps": func(r Result) float64 { return r.RPS },
	"errorRate": func(r Result) float64 {
		if r.Total == 0 {
			return 0
		}
		return float64(r.Failed) / float64(r.Total) * 100
	},
}

var ops = map[string]func(a, b float64) bool{
	"<":  func(a, b float64) bool { return a < b },
	"<=": func(a, b float64) bool { return a <= b },
	">":  func(a, b float64) bool { return a > b },
	">=": func(a, b float64) bool { return a >= b },
}

func evaluate(ts []Threshold, r Result) ([]CheckResult, bool) {
	out := make([]CheckResult, 0, len(ts))
	passed := true
	for _, t := range ts {
		actual := metricOf[t.Metric](r)
		ok := ops[t.Op](actual, t.Value)
		passed = passed && ok
		out = append(out, CheckResult{Threshold: t, Actual: actual, Passed: ok})
	}
	return out, passed
}

func doRequest(ctx context.Context, client *http.Client, cfg Config, col *collector, vu int, log func(vu int, ms float64, code int, err error)) {
	var body io.Reader
	if cfg.Body != "" {
		body = strings.NewReader(cfg.Body)
	}
	req, err := http.NewRequestWithContext(ctx, cfg.Method, cfg.URL, body)
	if err != nil {
		col.add(vu, 0, 0, err)
		log(vu, 0, 0, err)
		return
	}
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	t := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return // aborted because the test ended, not a target failure
		}
		ms := msSince(t)
		col.add(vu, ms, 0, err)
		log(vu, ms, 0, err)
		return
	}
	io.Copy(io.Discard, resp.Body) // read the full body so latency includes it and the connection is reused
	resp.Body.Close()
	ms := msSince(t)
	col.add(vu, ms, resp.StatusCode, nil)
	log(vu, ms, resp.StatusCode, nil)
}

func msSince(t time.Time) float64 { return float64(time.Since(t).Microseconds()) / 1000 }

func summarize(ms []float64) Latency {
	if len(ms) == 0 {
		return Latency{}
	}
	sort.Float64s(ms)
	var sum float64
	for _, v := range ms {
		sum += v
	}
	p := func(q float64) float64 { return ms[int(q*float64(len(ms)-1))] }
	return Latency{Min: ms[0], Avg: sum / float64(len(ms)), P50: p(0.50), P90: p(0.90), P95: p(0.95), P99: p(0.99), Max: ms[len(ms)-1]}
}
