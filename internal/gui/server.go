package gui

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"voltload/reports"
)

// The browser mode of the app: the same App methods the desktop window binds, served over HTTP.
//
//	POST /api/call/{Method}          JSON array of arguments -> JSON result, or {"error": "..."}
//	GET  /api/events                 Server-Sent Events: the same events the desktop app emits
//	GET  /api/runs/{id}/report.{fmt} a run's report (pdf, html, csv, json); ?download=1 to save it
//	GET  /...                        the embedded frontend
//
// It only listens on loopback. Because it can generate load against any URL, it also refuses requests
// a web page on another site could make: every request must name a loopback Host (blocks DNS
// rebinding) and calls must be application/json (a cross-site page can't send that without a CORS
// preflight, which this server never approves).

// desktopMethods open native dialogs; the browser frontend uses the HTTP report routes and its own
// file upload/download instead.
var desktopMethods = map[string]bool{"ExportRun": true, "OpenRunPDF": true, "SaveScenario": true, "LoadScenario": true}

const maxBody = 1 << 20

// sessions are the signed-in browsers: a random token in an HttpOnly cookie maps to a username.
// shortcut: in memory, so a server restart signs everyone out and there's no expiry or login rate
// limit; the server is loopback-only and bcrypt keeps guessing slow. Add both before exposing it further.
type sessions struct {
	mu sync.Mutex
	m  map[string]string
}

const cookieName = "voltload_session"

func (s *sessions) user(r *http.Request) string {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[c.Value]
}

func (s *sessions) start(w http.ResponseWriter, user string) {
	tok := make([]byte, 32)
	rand.Read(tok)
	t := hex.EncodeToString(tok)
	s.mu.Lock()
	s.m[t] = user
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: t, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

func (s *sessions) end(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.mu.Lock()
		delete(s.m, c.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

// requireLogin wraps the streaming and report routes (calls check inside call, since Login itself is one).
func (s *sessions) requireLogin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.user(r) == "" {
			http.Error(w, "sign in required", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// broker fans events out to every connected browser tab.
type broker struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func (b *broker) emit(event string, data any) {
	payload, err := json.Marshal(data)
	if err != nil {
		return
	}
	msg := []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event, payload))
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- msg:
		default: // shortcut: a tab that can't keep up loses events (logs are sampled anyway); add per-tab backpressure if this shows up
		}
	}
}

func (b *broker) subscribe() (chan []byte, func()) {
	ch := make(chan []byte, 1024)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}
}

// Serve runs the app as a local web server on addr until ctx is cancelled. A running test is stopped
// and its result saved before Serve returns. ready (may be nil) receives the bound URL.
func Serve(ctx context.Context, assets fs.FS, addr, dataDir string, ready func(url string)) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("address %q: %w", addr, err)
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("refusing to listen on %s: the server has no authentication, so it only listens on loopback (127.0.0.1, ::1 or localhost)", host)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	b := &broker{subs: map[chan []byte]struct{}{}}
	app := &App{emit: b.emit}
	app.open(dataDir)
	defer app.close()

	srv := &http.Server{Handler: handler(app, b, assets), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx) // event streams are closed by ctx below, so this doesn't wait on them
	}()
	srv.BaseContext = func(net.Listener) context.Context { return ctx }

	url := "http://" + ln.Addr().String()
	app.logf("VoltLoad is serving on %s (data: %s)", url, dataDir)
	if ready != nil {
		ready(url)
	}
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func handler(app *App, b *broker, assets fs.FS) http.Handler {
	mux := http.NewServeMux()
	sess := &sessions{m: map[string]string{}}
	mux.HandleFunc("POST /api/call/{method}", func(w http.ResponseWriter, r *http.Request) { call(app, sess, w, r) })
	mux.HandleFunc("GET /api/events", sess.requireLogin(func(w http.ResponseWriter, r *http.Request) { events(b, w, r) }))
	mux.HandleFunc("GET /api/runs/{id}/{file}", sess.requireLogin(func(w http.ResponseWriter, r *http.Request) { report(app, w, r) }))
	mux.Handle("GET /", http.FileServerFS(assets))
	return guard(mux)
}

// guard rejects requests that did not come from a page served by this loopback server.
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loopbackHost(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && !loopbackHost(strings.TrimPrefix(strings.TrimPrefix(o, "http://"), "https://")) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func loopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

var errorType = reflect.TypeFor[error]()

// call invokes an exported App method by name with JSON-decoded arguments, like the Wails bridge.
func call(app *App, sess *sessions, w http.ResponseWriter, r *http.Request) {
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		fail(w, http.StatusUnsupportedMediaType, "calls must be application/json")
		return
	}
	name := r.PathValue("method")
	m := reflect.ValueOf(app).MethodByName(name)
	if !m.IsValid() || desktopMethods[name] {
		fail(w, http.StatusNotFound, "unknown method "+name)
		return
	}
	// sign-in state lives in the cookie, so these three are answered here instead of by App
	user := sess.user(r)
	switch name {
	case "Session":
		writeJSON(w, http.StatusOK, map[string]any{"result": user})
		return
	case "Logout":
		sess.end(w, r)
		writeJSON(w, http.StatusOK, map[string]any{"result": nil})
		return
	case "Login": // App.Login checks the password below
	default:
		if user == "" {
			fail(w, http.StatusUnauthorized, "sign in required")
			return
		}
	}
	var raw []json.RawMessage
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&raw); err != nil {
		fail(w, http.StatusBadRequest, "arguments must be a JSON array: "+err.Error())
		return
	}
	mt := m.Type()
	if len(raw) != mt.NumIn() {
		fail(w, http.StatusBadRequest, fmt.Sprintf("%s takes %d arguments, got %d", name, mt.NumIn(), len(raw)))
		return
	}
	args := make([]reflect.Value, len(raw))
	for i, a := range raw {
		v := reflect.New(mt.In(i))
		if err := json.Unmarshal(a, v.Interface()); err != nil {
			fail(w, http.StatusBadRequest, fmt.Sprintf("argument %d: %v", i+1, err))
			return
		}
		args[i] = v.Elem()
	}
	var result any
	for _, out := range m.Call(args) {
		if out.Type() == errorType {
			if !out.IsNil() {
				fail(w, http.StatusUnprocessableEntity, out.Interface().(error).Error())
				return
			}
			continue
		}
		result = out.Interface()
	}
	if name == "Login" {
		sess.start(w, args[0].String())
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

func events(b *broker, w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ch, unsubscribe := b.subscribe()
	defer unsubscribe()
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	keepAlive := time.NewTicker(20 * time.Second)
	defer keepAlive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			w.Write(msg)
			flusher.Flush()
		case <-keepAlive.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// report serves /api/runs/{id}/report.{format}. PDFs open inline (the browser's viewer) unless ?download=1.
func report(app *App, w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	format, ok := strings.CutPrefix(r.PathValue("file"), "report.")
	if err != nil || !ok || reports.FormatFromPath("x."+format) != format {
		http.NotFound(w, r)
		return
	}
	res, err := app.GetRun(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	disposition := "inline"
	if r.URL.Query().Get("download") == "1" {
		disposition = "attachment"
	}
	types := map[string]string{"pdf": "application/pdf", "html": "text/html; charset=utf-8", "csv": "text/csv; charset=utf-8", "json": "application/json"}
	w.Header().Set("Content-Type", types[format])
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=\"voltload-run-%d.%s\"", disposition, id, format))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if err := reports.Write(w, format, res); err != nil {
		log.Printf("report run %d as %s: %v", id, format, err)
	}
}
