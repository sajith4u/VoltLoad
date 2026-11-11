package gui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"voltload/internal/loader"
	"voltload/internal/store"
	"voltload/reports"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Events emitted to the frontend.
const (
	EventProgress = "test:progress" // loader.Snapshot
	EventDone     = "test:done"     // DoneEvent
	EventLog      = "test:log"      // loader.LogEntry
)

// DoneEvent is sent once a test finishes, whether it succeeded, was stopped or failed.
type DoneEvent struct {
	SuiteID int64         `json:"suiteId"`
	RunID   int64         `json:"runId"` // 0 if the result could not be saved
	Result  loader.Result `json:"result"`
	Error   string        `json:"error"`
}

// App holds the methods exposed to the frontend (see frontend/wailsjs/go/gui/App.js). The desktop
// window binds them through Wails; `voltload serve` exposes the same methods over HTTP (server.go).
type App struct {
	ctx     context.Context // Wails runtime context; nil in serve mode, where the dialog methods are unavailable
	emit    func(event string, data any)
	store   *store.Store
	dataDir string
	logger  *log.Logger // backend log: terminal plus <data dir>/logs/voltload.log
	logFile *os.File

	mu      sync.Mutex
	cancel  context.CancelFunc // non-nil while a test runs
	running int64              // suite ID of the running test
	wg      sync.WaitGroup
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.emit = func(event string, data any) { runtime.EventsEmit(ctx, event, data) }
	dir, err := store.DataDir()
	if err != nil {
		runtime.LogErrorf(ctx, "find data directory: %v", err)
	}
	a.open(dir)
}

// open sets up logging and the database under dataDir. A database failure keeps the app usable for
// everything else; calls that need history report the error.
func (a *App) open(dataDir string) {
	a.dataDir = dataDir
	a.openLog()
	var err error
	if a.store, err = store.Open(filepath.Join(dataDir, "database", "voltload.db")); err != nil {
		a.store = nil
		a.logf("open database: %v", err)
	}
}

// openLog sends backend log lines to stderr (the terminal running the app, such as `wails dev`) and,
// when the data folder is writable, to a persistent log file. Logging never blocks the app starting.
func (a *App) openLog() {
	out := io.Writer(os.Stderr)
	dir := filepath.Join(a.dataDir, "logs")
	if err := os.MkdirAll(dir, 0o755); err == nil {
		if f, err := os.OpenFile(filepath.Join(dir, "voltload.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			a.logFile = f
			out = io.MultiWriter(os.Stderr, f)
		}
	}
	// log.Logger serialises writes, so lines from many virtual users never interleave
	a.logger = log.New(out, "", log.LstdFlags|log.Lmsgprefix)
}

func (a *App) logf(format string, args ...any) {
	if a.logger != nil {
		a.logger.Printf(format, args...)
	}
}

// shutdown stops any running test and waits for it so its result is saved before the DB closes.
func (a *App) shutdown(context.Context) { a.close() }

func (a *App) close() {
	a.StopTest()
	a.wg.Wait()
	if a.store != nil {
		a.store.Close()
	}
	if a.logFile != nil {
		a.logFile.Close()
	}
}

// RunSuite starts the saved suite in the background. Progress and the result arrive as events.
func (a *App) RunSuite(id int64) error {
	db, err := a.db()
	if err != nil {
		return err
	}
	su, err := db.GetSuite(id)
	if err != nil {
		return err
	}
	cfg := su.Config
	if err := cfg.Validate(); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		return errors.New("a test is already running")
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.running = id
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		prefix := fmt.Sprintf("[suite %d %q] ", id, su.Name)
		a.logf("%srun started", prefix)
		res, err := loader.RunWithLog(ctx, cfg, 500*time.Millisecond, func(s loader.Snapshot) {
			a.emit(EventProgress, s)
		}, func(e loader.LogEntry) {
			a.logf("%s%s", prefix, e.Line())
			a.emit(EventLog, e)
		})
		ev := DoneEvent{SuiteID: id, Result: res}
		if err != nil {
			ev.Error = err.Error()
		} else if ev.RunID, err = db.SaveRun(id, res); err != nil {
			ev.Error = "test finished but saving the result failed: " + err.Error()
		}
		if ev.Error != "" {
			a.logf("%srun error: %s", prefix, ev.Error)
		} else {
			a.logf("%srun saved as #%d", prefix, ev.RunID)
		}
		a.mu.Lock()
		a.cancel, a.running = nil, 0
		a.mu.Unlock()
		cancel()
		a.emit(EventDone, ev)
	}()
	return nil
}

// RunningSuite is the ID of the suite being run, or 0. Lets the UI recover its state after a reload.
func (a *App) RunningSuite() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running
}

// StopTest cancels the running test, if any. Results collected so far are still saved.
func (a *App) StopTest() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
	}
}

func (a *App) db() (*store.Store, error) {
	if a.store == nil {
		return nil, errors.New("history database is unavailable; check the app log")
	}
	return a.store, nil
}

func (a *App) ListSuites() ([]store.SuiteSummary, error) {
	s, err := a.db()
	if err != nil {
		return nil, err
	}
	return s.ListSuites(30)
}

func (a *App) GetSuite(id int64) (store.Suite, error) {
	s, err := a.db()
	if err != nil {
		return store.Suite{}, err
	}
	return s.GetSuite(id)
}

// SaveSuite creates the suite when its ID is 0, otherwise updates it. Returns the ID.
func (a *App) SaveSuite(su store.Suite) (int64, error) {
	s, err := a.db()
	if err != nil {
		return 0, err
	}
	return s.SaveSuite(su)
}

// DeleteSuite removes the suite and its run history.
func (a *App) DeleteSuite(id int64) error {
	s, err := a.db()
	if err != nil {
		return err
	}
	if a.RunningSuite() == id {
		return errors.New("stop the running test before deleting its suite")
	}
	return s.DeleteSuite(id)
}

func (a *App) ListRuns(suiteID int64) ([]store.RunSummary, error) {
	s, err := a.db()
	if err != nil {
		return nil, err
	}
	return s.ListRuns(suiteID, 200)
}

func (a *App) GetRun(id int64) (loader.Result, error) {
	s, err := a.db()
	if err != nil {
		return loader.Result{}, err
	}
	return s.GetRun(id)
}

func (a *App) DeleteRun(id int64) error {
	s, err := a.db()
	if err != nil {
		return err
	}
	return s.DeleteRun(id)
}

// ExportRun asks where to save and writes the run as a json, csv or html report. Returns the path, or "" if cancelled.
func (a *App) ExportRun(id int64, format string) (string, error) {
	if err := a.desktopOnly(); err != nil {
		return "", err
	}
	r, err := a.GetRun(id)
	if err != nil {
		return "", err
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: fmt.Sprintf("voltload-run-%d.%s", id, format),
		Filters:         []runtime.FileFilter{{DisplayName: strings.ToUpper(format), Pattern: "*." + format}},
	})
	if err != nil || path == "" {
		return "", err
	}
	if reports.FormatFromPath(path) != format {
		path += "." + format // some Linux dialogs don't append the extension
	}
	return path, reports.WriteFile(path, r)
}

// OpenRunPDF writes the run's full PDF report into the app's reports folder and opens it in the
// system PDF viewer. Returns the file path.
func (a *App) OpenRunPDF(id int64) (string, error) {
	if err := a.desktopOnly(); err != nil {
		return "", err
	}
	r, err := a.GetRun(id)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(a.dataDir, "reports")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("voltload-run-%d.pdf", id))
	if err := reports.WriteFile(path, r); err != nil {
		return "", err
	}
	runtime.BrowserOpenURL(a.ctx, "file://"+filepath.ToSlash(path))
	return path, nil
}

// ScenarioYAML returns cfg as a YAML scenario (the format `voltload run file.yaml` reads).
func (a *App) ScenarioYAML(cfg loader.Config) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", err
	}
	b, err := loader.MarshalScenario(cfg)
	return string(b), err
}

// ParseScenario validates YAML scenario text, such as an uploaded file, and returns its config.
func (a *App) ParseScenario(text string) (loader.Config, error) {
	return loader.ParseScenario([]byte(text))
}

// desktopOnly guards the methods that open native dialogs, which need the Wails window.
func (a *App) desktopOnly() error {
	if a.ctx == nil {
		return errors.New("this action needs the desktop app")
	}
	return nil
}

var yamlFilter = []runtime.FileFilter{{DisplayName: "Scenario (YAML)", Pattern: "*.yaml;*.yml"}}

// SaveScenario writes cfg as a YAML scenario (same format as `voltload run file.yaml`). Returns "" if cancelled.
func (a *App) SaveScenario(cfg loader.Config) (string, error) {
	if err := a.desktopOnly(); err != nil {
		return "", err
	}
	text, err := a.ScenarioYAML(cfg)
	if err != nil {
		return "", err
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{DefaultFilename: "scenario.yaml", Filters: yamlFilter})
	if err != nil || path == "" {
		return "", err
	}
	return path, os.WriteFile(path, []byte(text), 0o644)
}

// LoadScenario asks for a YAML scenario file and returns it validated. Returns a nil config if cancelled.
func (a *App) LoadScenario() (*loader.Config, error) {
	if err := a.desktopOnly(); err != nil {
		return nil, err
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Filters: yamlFilter})
	if err != nil || path == "" {
		return nil, err
	}
	c, err := loader.LoadScenario(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return &c, nil
}

// Run opens the desktop window. assets is the embedded frontend/dist.
func Run(assets fs.FS) error {
	app := &App{}
	return wails.Run(&options.App{
		Title:            "VoltLoad",
		Width:            1200,
		Height:           820,
		MinWidth:         900,
		MinHeight:        600,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 17, G: 24, B: 39, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
	})
}
