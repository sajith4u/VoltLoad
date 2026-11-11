
# VoltLoad

**Put Your Systems Under Pressure.**

A local-first HTTP API load testing tool. It runs as a desktop app (built with [Wails](https://wails.io)) or headless from the terminal, and both use the same Go engine. No server, database or internet connection is required apart from the target you are testing.

## Features (MVP)

- **Performance suites**: saved, re-runnable load tests, created through a step-by-step flow (details → request → workload → thresholds → review)
- **Dashboard**: one card per suite with a pass/fail strip of its recent runs, last status, success rate, average response time and a Run button
- **Trend charts** per suite across runs: average response time, hits/sec, p95 latency, error rate, total requests, duration
- **Virtual user engine**: every virtual user runs concurrently as its own user, with a private connection, its own cookie session, optional random think time between requests, and its own statistics in the results
- **PDF report** with the full details of a run: summary, response-time percentiles, thresholds, timeline charts, test configuration, status codes, errors and per-user statistics. Open it from a run with "View PDF report", or use `--out report.pdf` in the CLI
- Request setup: method, URL, headers, body, timeout
- Workloads: constant virtual users, or ramping **stages**, with Smoke / Load / Stress / Spike presets
- **Thresholds** such as `p95 < 500` or `errorRate < 1` mark a run PASS or FAIL. The CLI exits with code 2 on FAIL, so CI jobs can gate on it
- **Live logs** while a suite runs: stage changes, sampled requests per virtual user with status and response time, errors, and the final verdict, with filters for requests, errors and engine events. In the CLI: `--logs`
- **Backend logs**: the same log is printed to the terminal running the app (so `wails dev` shows it) and appended to `~/.local/share/voltload/logs/voltload.log` (`%APPDATA%\VoltLoad\logs\voltload.log` on Windows), each line tagged with the suite
- Live run view (updates every 500 ms) with charts for requests/sec, latency and active VUs, plus a Stop button
- Run history per suite: view, delete, compare two runs, export PDF / HTML / CSV / JSON reports
- **YAML scenarios** shared by the desktop app (import/export) and the CLI. See [examples/smoke.yaml](examples/smoke.yaml)

History is stored in `~/.local/share/voltload/database/voltload.db` on Linux and `%APPDATA%\VoltLoad\database\voltload.db` on Windows.

## Tech Stack

**Backend:** Go 1.26+

**Frontend:** React + TypeScript + Vite, embedded in the binary by Wails

**Storage:** SQLite (`modernc.org/sqlite`, pure Go)

## Prerequisites

- Go 1.26 or newer (Wails v2.17 requires it)
- Node.js and npm
- Wails CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1`
  (installs to `$(go env GOPATH)/bin`, so make sure that directory is on your `PATH`)
- Linux only: `sudo apt install libgtk-3-dev libwebkit2gtk-4.0-dev`

Run `wails doctor` to check your setup.

## Run Locally

Clone the project

```bash
  git clone https://github.com/sajith4u/gohammer.git
  cd gohammer
```

Desktop app, with live reload

```bash
  wails dev
```

UI preview in a browser (no GTK/WebKit needed). It shows sample data and simulated runs, and sends no real requests:

```bash
  cd frontend && npm install && npm run dev   # then open the printed http://localhost:5173 URL
```

Desktop app, production build (output in `build/bin/`)

```bash
  wails build
  ./build/bin/voltload
```

CLI only (no GUI dependencies needed)

```bash
  go build -o voltload .
  ./voltload run --url https://example.com --vus 10 --duration 30
  ./voltload run --url https://api.local/items -X POST \
      -H "Content-Type: application/json" -d '{"a":1}' --requests 1000 --json

  # scenario file; flags override it. Exit code 2 if a threshold fails.
  ./voltload validate examples/smoke.yaml
  ./voltload run examples/smoke.yaml --threshold "p95<500" --out report.html
  ./voltload run examples/smoke.yaml --think-min 200 --think-max 800 --out report.pdf
  ./voltload run examples/smoke.yaml --logs      # live log instead of the progress line
  ./voltload report result.json -o report.pdf    # convert a saved JSON result
  ./voltload version
```

Running the binary with no arguments opens the desktop window. `voltload <command> --help` lists all flags. Press Ctrl+C to stop a CLI run early; the results collected so far are still printed.

Tests

```bash
  go test ./...
```

## License


[![MIT License](https://img.shields.io/badge/License-MIT-green.svg)](https://choosealicense.com/licenses/mit/)

