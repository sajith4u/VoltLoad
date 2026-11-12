package webperf

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// browser is a headless Chrome with a fresh profile (so the first visit has an empty cache), driven
// over the DevTools protocol. It is a minimal client: one page, one request at a time, events ignored.
type browser struct {
	cmd     *exec.Cmd
	profile string
	conn    *websocket.Conn
	id      int64
	version string
}

// FindChrome returns the path of an installed Chrome, Chromium or Edge. VOLTLOAD_CHROME overrides it.
func FindChrome() (string, error) {
	if p := os.Getenv("VOLTLOAD_CHROME"); p != "" {
		return p, nil
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge", "msedge", "chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	paths := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		filepath.Join(os.Getenv("LOCALAPPDATA"), `Google\Chrome\Application\chrome.exe`),
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", errors.New("no Chrome, Chromium or Edge found: install one, or set VOLTLOAD_CHROME to its path")
}

func launch(ctx context.Context) (*browser, error) {
	bin, err := FindChrome()
	if err != nil {
		return nil, err
	}
	profile, err := os.MkdirTemp("", "voltload-chrome-")
	if err != nil {
		return nil, err
	}
	args := []string{"--headless=new", "--disable-gpu", "--no-first-run", "--no-default-browser-check",
		"--disable-extensions", "--disable-background-networking", "--mute-audio",
		"--remote-debugging-port=0", "--user-data-dir=" + profile, "--window-size=1350,940", "about:blank"}
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		args = append(args, "--no-sandbox") // Chrome refuses to run as root with its sandbox
	}
	b := &browser{cmd: exec.CommandContext(ctx, bin, args...), profile: profile}
	stderr, err := b.cmd.StderrPipe()
	if err != nil {
		b.close()
		return nil, err
	}
	if err := b.cmd.Start(); err != nil {
		b.close()
		return nil, fmt.Errorf("start %s: %w", bin, err)
	}

	// Chrome prints "DevTools listening on ws://127.0.0.1:PORT/devtools/browser/…" once it is ready.
	found := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			if _, ws, ok := strings.Cut(sc.Text(), "DevTools listening on "); ok {
				found <- ws
				break
			}
		}
		io.Copy(io.Discard, stderr) // keep draining so Chrome never blocks on a full pipe
	}()
	var wsURL string
	select {
	case wsURL = <-found:
	case <-time.After(20 * time.Second):
		b.close()
		return nil, errors.New("Chrome did not start within 20s")
	case <-ctx.Done():
		b.close()
		return nil, ctx.Err()
	}
	u, err := url.Parse(wsURL)
	if err != nil {
		b.close()
		return nil, err
	}

	var targets []struct{ Type, WebSocketDebuggerURL string }
	var ver struct{ Browser string }
	if err := getJSON(ctx, "http://"+u.Host+"/json/list", &targets); err != nil {
		b.close()
		return nil, err
	}
	getJSON(ctx, "http://"+u.Host+"/json/version", &ver)
	b.version = ver.Browser
	for _, t := range targets {
		if t.Type == "page" {
			b.conn, _, err = websocket.DefaultDialer.DialContext(ctx, t.WebSocketDebuggerURL, nil)
			if err != nil {
				b.close()
				return nil, err
			}
			return b, nil
		}
	}
	b.close()
	return nil, errors.New("Chrome opened no page")
}

func getJSON(ctx context.Context, u string, out any) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

func (b *browser) close() {
	if b.conn != nil {
		b.conn.Close()
	}
	if b.cmd.Process != nil {
		b.cmd.Process.Kill()
		b.cmd.Wait()
	}
	os.RemoveAll(b.profile)
}

// call sends one DevTools command and waits for its reply, skipping any events in between.
func (b *browser) call(ctx context.Context, method string, params any, out any) error {
	b.id++
	id := b.id
	if dl, ok := ctx.Deadline(); ok {
		b.conn.SetReadDeadline(dl)
		b.conn.SetWriteDeadline(dl)
	}
	if err := b.conn.WriteJSON(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return err
	}
	for {
		var m struct {
			ID     int64
			Result json.RawMessage
			Error  *struct{ Message string }
		}
		if err := b.conn.ReadJSON(&m); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("%s: %w", method, err)
		}
		if m.ID != id {
			continue
		}
		if m.Error != nil {
			return fmt.Errorf("%s: %s", method, m.Error.Message)
		}
		if out != nil {
			return json.Unmarshal(m.Result, out)
		}
		return nil
	}
}

// eval runs a JS expression in the page (awaiting promises) and decodes its JSON value into out.
func (b *browser) eval(ctx context.Context, expr string, out any) error {
	var r struct {
		Result           struct{ Value json.RawMessage }
		ExceptionDetails *struct {
			Text      string
			Exception *struct{ Description string }
		}
	}
	if err := b.call(ctx, "Runtime.evaluate", map[string]any{"expression": expr, "returnByValue": true, "awaitPromise": true}, &r); err != nil {
		return err
	}
	if e := r.ExceptionDetails; e != nil {
		if e.Exception != nil {
			return errors.New(e.Exception.Description)
		}
		return errors.New(e.Text)
	}
	if out == nil || len(r.Result.Value) == 0 {
		return nil
	}
	return json.Unmarshal(r.Result.Value, out)
}

func (b *browser) navigate(ctx context.Context, u string) error {
	var r struct{ ErrorText string }
	if err := b.call(ctx, "Page.navigate", map[string]any{"url": u}, &r); err != nil {
		return err
	}
	if r.ErrorText != "" {
		return fmt.Errorf("could not load %s: %s", u, r.ErrorText)
	}
	return nil
}
