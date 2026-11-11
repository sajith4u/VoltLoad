# CLAUDE.md

VoltLoad is a local-first HTTP API load testing tool. It runs as a Wails v2 desktop app (React + TypeScript UI) or as a headless Cobra CLI, and both use the same engine. `requirement.md` is the full product spec. The current code covers its MVP slice. The app was renamed from GoHammer, so the repo folder and GitHub URL still say `gohammer`. Module `voltload`, Go 1.26 (Wails v2.17.1 requires it).

## Commands

```bash
wails dev                    # desktop app with hot reload (runs binary with no args → GUI)
wails build                  # production desktop build → build/bin/voltload
wails generate module        # regenerate frontend/wailsjs after changing bound Go methods/types
go build -o voltload . && ./voltload run --url https://example.com --vus 10 --duration 10
./voltload validate examples/smoke.yaml && ./voltload run examples/smoke.yaml --out r.html
go vet ./... && go test -race ./...
cd frontend && npm install && npm run build   # tsc type-check + Vite → frontend/dist
```

On Linux the desktop build needs `libgtk-3-dev` and `libwebkit2gtk-4.0-dev`. Run `wails doctor` to check. A plain `go build` has no GUI, so running it with no arguments prints a "missing build tags" error. That's expected.

## Layout

- `main.go` embeds `frontend/dist` and passes it to `cmd.Execute(assets)`. `//go:embed` must sit next to `frontend/`.
- `cmd/`: Cobra commands. In `root.go`, running with no subcommand calls `gui.Run`, which opens the window. `run.go` defines `run [scenario.yaml]`, where flags override the file, but only the flags the user actually set (`Flags().Changed`). It also takes `--threshold "p95<500"` and `--out file.{html,csv,json}`, and exits with code 2 when a threshold fails. `tools.go` defines `version`, `validate` and `report`. The version is set with `-ldflags "-X voltload/cmd.Version=…"`.
- `internal/loader/` is the engine. `Config.Validate()` normalises and checks input. With stages, it sets `VUs` to the peak and `DurationSec` to the total. `Run(ctx, cfg, tick, onTick)` starts `VUs` goroutines. Each VU sends requests only while its index is below the active target, and with stages a controller recomputes that target every 100ms (`stageVUs`, a linear ramp starting from 0). Each VU is a goroutine acting as a distinct user: it has its own `http.Client` with a private `Transport` (one keep-alive connection) and cookie jar, an optional random think time (`ThinkMinMs`–`ThinkMaxMs`) after every request, and its own `VUStat` in the result. These are goroutines and not OS threads, and the Go scheduler spreads them across cores. Don't pin them with `LockOSThread`. The run ends when the duration expires, the request budget runs out or ctx is cancelled. It returns a `Result` with counts, RPS, latency percentiles, status codes, errors, per-VU stats (`VUs`), a `Timeline` of snapshots (downsampled to about 600 points at most) and threshold `Checks`/`Passed`. Requests cut off by cancellation don't count as failures.
- Live log: `RunWithLog` (`Run` is the same without a log) calls `onLog` with `LogEntry` values. Engine events (start, stage changes, finish, threshold verdicts) are always sent. Request lines are sampled at `logOKPerTick` successes plus `logErrPerTick` failures per tick, so a fast test can't flood the UI. `onLog` is called from many goroutines at once. `LogEntry.Line()` is the one plain-text format, used by the CLI `--logs` and by the app's backend log. Logs aren't stored with the run in the database.
- `internal/loader/scenario.go` handles YAML scenarios. `Config` itself is the format (yaml tags), and unknown keys are rejected.
- `reports/`: `Write`/`WriteFile` render a `Result` as JSON, CSV, a standalone HTML page (`html/template`, no external assets) or a full-detail PDF. `pdf.go` uses `github.com/go-pdf/fpdf`, which is pure Go, with the built-in Helvetica font, so all text must go through `d.tr()` (cp1252). The CLI and the desktop export both use it. Reports show header names only and never values, because values may be credentials.
- `internal/store/`: SQLite persistence using `modernc.org/sqlite`, which is pure Go with no cgo. `DataDir()` returns the OS app-data directory. Schema migrations are an append-only slice applied through `PRAGMA user_version`. A **suite** is a saved, re-runnable test: a name, a description and a `loader.Config` stored as JSON. A **run** belongs to a suite (`suite_id`; NULL for runs made before suites existed) and stores summary columns plus the full `Result` as JSON. `ListSuites` returns each suite with its 30 most recent runs for the dashboard cards. `RunSummary.OK` means thresholds passed, or with no thresholds, zero failed requests.
- `internal/gui/` is the Wails app. Exported `App` methods are bound to JS:
  - suites: `ListSuites`, `GetSuite`, `SaveSuite` (ID 0 creates), `DeleteSuite`
  - running: `RunSuite(id)`, `StopTest`, `RunningSuite`
  - runs: `ListRuns(suiteID)`, `GetRun`, `DeleteRun`, `ExportRun(id, format)`, `OpenRunPDF(id)`, which writes to `<data dir>/reports/` and opens the system PDF viewer
  - YAML: `SaveScenario`, `LoadScenario` (native file dialogs)

  The backend log (`openLog`/`logf` in `app.go`) writes each log line tagged `[suite <id> "<name>"]`, plus run start and saved/error lines, to stderr and to `<data dir>/logs/voltload.log`. It is append-only with no rotation. Add `shortcut:` handling if the file grows too large.

  Progress is sent as events: `test:progress` carries a `loader.Snapshot`, `test:log` carries a `loader.LogEntry`, and `test:done` carries a `DoneEvent`. Only one test runs at a time. On shutdown the app stops the running test and waits for its result to be saved.
- `frontend/src/` is the React UI, light theme only, with design tokens at the top of `style.css`:
  - `App.tsx`: top bar, page routing held in state (dashboard / create / suite), and the single app-wide `RunState` fed by the events.
  - `Dashboard.tsx`: the suite card grid. Each card shows a strip of the last 30 runs, the last status, Run/Edit, success rate and average response time with the change from the previous run.
  - `CreateSuite.tsx`: the 5-step create flow (Details → Request → Workload → Thresholds → Review).
  - `SuitePage.tsx`: side panel with Run/Stop, plus tabs for Configuration, History (with two-run compare), Trend Charts and the Live run.
  - `SuiteFields.tsx` and `Editors.tsx`: the form sections shared by the create flow and the Configuration tab.
  - `form.ts`: the editable `Form` model, its conversion to and from `loader.Config`/`store.Suite`, and the workload `PRESETS`.
  - `Chart.tsx`: a dependency-free SVG line/area chart. One series and one measure per chart, never a dual axis. Each metric has a fixed color.
  - `LogFeed.tsx`: the live log console on the Live run tab. It follows the newest line until the user scrolls up, and has filter chips. Its level classes are `lv-info`/`lv-ok`/`lv-warn`/`lv-error`. Don't use a bare `.error` class, because that is the alert-box style.
  - `ResultView.tsx` and `CompareView.tsx`: a single run's result and the comparison table.
  - `events.ts`: hand-written TS types for the event payloads, because Wails only generates types for method signatures. Keep it in sync with `loader.Snapshot`, `loader.LogEntry` and `gui.DoneEvent`.
  - `preview.ts`: when the page is opened outside the Wails window (for example the Vite URL in a browser), `window.go`/`window.runtime` don't exist. This file installs an in-memory stand-in with sample data and simulated runs, and the app shows a "Preview mode" banner. **Whenever you add a bound method, add it here too**, otherwise the browser preview breaks.
- `frontend/wailsjs/` is generated, so don't edit it. `frontend/dist/` is build output. Only `gitkeep` is committed, and `frontend/public/gitkeep` puts it back after each Vite build so `go:embed` still compiles on a clean checkout.
- `internal/ui/` (tview) and `internal/utils/` (viper config, file logger) are older code that nothing calls.

## Conventions

- Keep logic in `internal/loader` and `internal/store`. `cmd/` and `internal/gui` only adapt input and output.
- Whenever you add or change an exported `App` method or one of its types, run `wails generate module` and commit `frontend/wailsjs/`.
- To change the schema, append to `migrations` in `internal/store/store.go` and never edit an existing entry.
- New subcommands go in their own file under `cmd/` and register with `rootCmd.AddCommand` in `init()`.

## Not built yet (from requirement.md)

Projects and environments, arrival-rate (open model) workloads, multi-request scenarios, response assertions (body/status checks per request), backup/restore, retention, storing credentials in the OS keychain and leaving them out of exports, installers (NSIS/DEB/AppImage) with a release pipeline, and the VoltLoad rename.

## Known limits

- The engine keeps every latency sample in memory (about 8 bytes per request). Switch to a histogram before supporting multi-million-request runs (marked `shortcut:` in `loader.go`).
- Headers, including auth tokens, are stored in plain text in the history DB and in saved scenario files.
- After `wails generate module`, restart `wails dev`/Vite. Its file watcher loses the regenerated `wailsjs/` folder and keeps serving stale bindings ("does not provide an export named …").
- Requests still in flight when a run ends are dropped and not counted, so a run where nothing completed in time reports 0 requests with no error.
- Per-VU stats and the timeline are stored in each run's JSON, about 60 bytes per VU, so a 10k-VU run adds roughly 600 KB.
- The real GUI needs `wails dev`. Without WebKit, run `cd frontend && npm run dev` and open the Vite URL: the UI runs in preview mode (`preview.ts`) and can be clicked through or screenshotted in headless Chrome.
- A suite holds one request. Multi-request suites would need engine support first.
