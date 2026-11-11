package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"voltload/internal/loader"
	"voltload/reports"

	"github.com/spf13/cobra"
)

// ExitThresholdsFailed is returned when the run completed but a threshold failed, so CI can fail the build.
const ExitThresholdsFailed = 2

var (
	runFlags      loader.Config
	runHeaders    []string
	runThresholds []string
	runJSON       bool
	runLogs       bool
	runOut        string
)

var runCmd = &cobra.Command{
	Use:   "run [scenario.yaml]",
	Short: "Run a load test headless (no GUI)",
	Long: `Run a load test from flags, a YAML scenario file, or both (flags override the file).
Exits with code 2 if any threshold fails.`,
	Example: `  voltload run --url https://example.com --vus 10 --duration 30
  voltload run --url https://api.local/items -X POST -H "Content-Type: application/json" -d '{"a":1}' --requests 1000
  voltload run ./tests/api.yaml --vus 50 --threshold "p95<500" --threshold "errorRate<1" --out report.html`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := buildConfig(cmd, args)
		if err != nil {
			return err
		}

		// Ctrl+C stops the test early but still prints what was collected.
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()

		var onTick func(loader.Snapshot)
		if !runJSON {
			onTick = func(s loader.Snapshot) {
				fmt.Fprintf(os.Stderr, "\r%6.1fs  vus %d  total %d  ok %d  failed %d  %.1f req/s   ",
					s.ElapsedSec, s.ActiveVUs, s.Total, s.Success, s.Failed, s.RPS)
			}
		}
		var onLog func(loader.LogEntry)
		if runLogs && !runJSON {
			onTick = nil // the log replaces the one-line progress display
			onLog = printLog
		}
		r, err := loader.RunWithLog(ctx, cfg, time.Second, onTick, onLog)
		if err != nil {
			return err
		}
		if runJSON {
			err = reports.Write(os.Stdout, "json", r)
		} else {
			fmt.Fprintln(os.Stderr)
			printResult(r)
		}
		if err != nil {
			return err
		}
		if runOut != "" {
			if err := reports.WriteFile(runOut, r); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "report written to", runOut)
		}
		if !r.Passed {
			os.Exit(ExitThresholdsFailed)
		}
		return nil
	},
}

var logMu sync.Mutex // log lines arrive from many virtual users at once

func printLog(e loader.LogEntry) {
	logMu.Lock()
	defer logMu.Unlock()
	fmt.Fprintln(os.Stderr, e.Line())
}

// buildConfig loads the optional scenario file, then applies any flags the user set explicitly.
func buildConfig(cmd *cobra.Command, args []string) (loader.Config, error) {
	var cfg loader.Config
	if len(args) == 1 {
		var err error
		if cfg, err = loader.LoadScenario(args[0]); err != nil {
			return cfg, fmt.Errorf("%s: %w", args[0], err)
		}
	}
	f := cmd.Flags()
	set := func(name string, apply func()) {
		if f.Changed(name) {
			apply()
		}
	}
	set("url", func() { cfg.URL = runFlags.URL })
	set("method", func() { cfg.Method = runFlags.Method })
	set("data", func() { cfg.Body = runFlags.Body })
	set("timeout", func() { cfg.TimeoutMs = runFlags.TimeoutMs })
	set("requests", func() { cfg.Requests = runFlags.Requests })
	set("think-min", func() { cfg.ThinkMinMs = runFlags.ThinkMinMs })
	set("think-max", func() { cfg.ThinkMaxMs = runFlags.ThinkMaxMs })
	// --vus/--duration switch a staged scenario back to a constant load
	set("vus", func() { cfg.VUs, cfg.Stages = runFlags.VUs, nil })
	set("duration", func() { cfg.DurationSec, cfg.Stages = runFlags.DurationSec, nil })
	if len(args) == 0 {
		if cfg.VUs == 0 && len(cfg.Stages) == 0 {
			cfg.VUs = 10
		}
		if cfg.DurationSec == 0 && cfg.Requests == 0 {
			cfg.Requests = 100
		}
	}
	if len(runHeaders) > 0 && cfg.Headers == nil {
		cfg.Headers = map[string]string{}
	}
	for _, h := range runHeaders {
		k, v, ok := strings.Cut(h, ":")
		if !ok {
			return cfg, fmt.Errorf("header %q must look like 'Name: value'", h)
		}
		cfg.Headers[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	for _, s := range runThresholds {
		t, err := parseThreshold(s)
		if err != nil {
			return cfg, err
		}
		cfg.Thresholds = append(cfg.Thresholds, t)
	}
	if runOut != "" && reports.FormatFromPath(runOut) == "" {
		return cfg, fmt.Errorf("--out %q: use a .json, .csv, .html or .pdf file", runOut)
	}
	return cfg, cfg.Validate()
}

var thresholdRe = regexp.MustCompile(`^\s*(\w+)\s*(<=|>=|<|>)\s*([0-9.]+)\s*$`)

// parseThreshold turns "p95<500" into a Threshold; the engine validates the metric name.
func parseThreshold(s string) (loader.Threshold, error) {
	m := thresholdRe.FindStringSubmatch(s)
	if m == nil {
		return loader.Threshold{}, fmt.Errorf("threshold %q must look like 'p95<500'", s)
	}
	v, err := strconv.ParseFloat(m[3], 64)
	if err != nil {
		return loader.Threshold{}, fmt.Errorf("threshold %q: %w", s, err)
	}
	return loader.Threshold{Metric: m[1], Op: m[2], Value: v}, nil
}

func printResult(r loader.Result) {
	l := r.LatencyMs
	stopped := ""
	if r.Stopped {
		stopped = "  (stopped)"
	}
	fmt.Printf("%s %s  vus=%d  %.2fs%s\n", r.Config.Method, r.Config.URL, r.Config.VUs, r.DurationSec, stopped)
	fmt.Printf("requests: %d  ok: %d  failed: %d  rps: %.2f\n", r.Total, r.Success, r.Failed, r.RPS)
	fmt.Printf("latency ms: min %.1f  avg %.1f  p50 %.1f  p90 %.1f  p95 %.1f  p99 %.1f  max %.1f\n",
		l.Min, l.Avg, l.P50, l.P90, l.P95, l.P99, l.Max)
	for _, c := range reports.StatusCodes(r) {
		fmt.Printf("  HTTP %d: %d\n", c.Code, c.Count)
	}
	for e, n := range r.Errors {
		fmt.Printf("  error x%d: %s\n", n, e)
	}
	for _, c := range r.Checks {
		status := "PASS"
		if !c.Passed {
			status = "FAIL"
		}
		fmt.Printf("  threshold %s %s %g: %s (actual %.2f)\n", c.Threshold.Metric, c.Threshold.Op, c.Threshold.Value, status, c.Actual)
	}
}

func init() {
	f := runCmd.Flags()
	f.StringVar(&runFlags.URL, "url", "", "target URL (required without a scenario file)")
	f.StringVarP(&runFlags.Method, "method", "X", "GET", "HTTP method")
	f.StringArrayVarP(&runHeaders, "header", "H", nil, "request header 'Name: value' (repeatable)")
	f.StringVarP(&runFlags.Body, "data", "d", "", "request body")
	f.IntVar(&runFlags.VUs, "vus", 10, "concurrent virtual users")
	f.IntVar(&runFlags.DurationSec, "duration", 0, "test duration in seconds")
	f.IntVar(&runFlags.Requests, "requests", 0, "total requests to send (default 100 if no duration)")
	f.IntVar(&runFlags.TimeoutMs, "timeout", 30000, "per-request timeout in ms")
	f.IntVar(&runFlags.ThinkMinMs, "think-min", 0, "minimum pause in ms each virtual user takes between requests")
	f.IntVar(&runFlags.ThinkMaxMs, "think-max", 0, "maximum pause in ms each virtual user takes between requests")
	f.StringArrayVar(&runThresholds, "threshold", nil, "pass/fail rule like 'p95<500' or 'errorRate<1' (repeatable)")
	f.BoolVar(&runJSON, "json", false, "print the full result as JSON")
	f.BoolVar(&runLogs, "logs", false, "print a live log (engine events and a sample of requests) instead of the progress line")
	f.StringVarP(&runOut, "out", "o", "", "also write a report; format from extension (.json, .csv, .html, .pdf)")
	rootCmd.AddCommand(runCmd)
}
