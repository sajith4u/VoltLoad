package webperf

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A small "React app": a development React build registers with the devtools hook, then the page
// calls a slow API and, after it, a second one. The JS is uncompressed and has no cache headers.
func fakeReactSite() *httptest.Server {
	js := `__REACT_DEVTOOLS_GLOBAL_HOOK__.inject({version: '18.3.1', bundleType: 1});
fetch('/api/slow').then(r => r.json()).then(() => fetch('/api/next')).then(() => {
  document.getElementById('root').textContent = 'Dashboard loaded';
});
` + strings.Repeat("// padding so the bundle is large enough to be worth compressing\n", 80)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `<!doctype html><html><head><title>App</title><link rel="stylesheet" href="/app.css"></head>
<body><h1>Fake app</h1><div id="root"></div><script src="/app.js"></script></body></html>`)
	})
	mux.HandleFunc("/app.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		io.WriteString(w, "body { font-family: sans-serif }")
	})
	mux.HandleFunc("/app.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		io.WriteString(w, js)
	})
	mux.HandleFunc("/api/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	})
	mux.HandleFunc("/api/next", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"items":[]}`)
	})
	return httptest.NewServer(mux)
}

func TestAnalyzeRealBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("starts Chrome")
	}
	if _, err := FindChrome(); err != nil {
		t.Skip(err)
	}
	site := fakeReactSite()
	defer site.Close()

	a, err := Analyze(context.Background(), site.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !a.React.Detected || !a.React.DevBuild || a.React.Version != "18.3.1" {
		t.Errorf("react: %+v", a.React)
	}
	if !a.Settled || a.First.FCPMs <= 0 || a.First.LoadMs <= 0 || a.First.Status != 200 {
		t.Errorf("first visit: settled=%v %+v", a.Settled, a.First)
	}
	var slow *Asset
	for i, x := range a.Assets {
		if strings.HasSuffix(x.URL, "/api/slow") {
			slow = &a.Assets[i]
		}
	}
	if slow == nil || slow.Kind != "api" || slow.DurationMs < 1200 {
		t.Fatalf("slow API call not measured: %+v", slow)
	}
	if a.Assets[0].Kind != "document" {
		t.Errorf("first asset should be the document: %+v", a.Assets[0])
	}

	var titles []string
	for _, r := range a.Recommendations {
		titles = append(titles, r.Title)
	}
	all := strings.Join(titles, "\n")
	for _, want := range []string{"React development build", "slow API call", "one after another", "not cached", "without compression"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing recommendation %q; got:\n%s", want, all)
		}
	}
	if a.Recommendations[0].Severity != "critical" {
		t.Errorf("recommendations not ordered by severity: %+v", a.Recommendations[0])
	}
}

func TestAnalyzeRejectsBadURLs(t *testing.T) {
	for _, u := range []string{"", "ftp://x", "file:///etc/passwd", "javascript:alert(1)"} {
		if _, err := Analyze(context.Background(), u); err == nil {
			t.Errorf("%q accepted", u)
		}
	}
	if got, _ := normalizeURL(" example.com/app "); got != "https://example.com/app" {
		t.Errorf("normalize: %q", got)
	}
}

func TestVitalRatings(t *testing.T) {
	got := map[string]string{}
	for _, v := range vitals(Load{TTFBMs: 500, FCPMs: 2000, LCPMs: 5000, CLS: 0.05}) {
		got[v.Key] = v.Rating
	}
	want := map[string]string{"ttfb": "good", "fcp": "needs-improvement", "lcp": "poor", "cls": "good", "tbt": "good", "load": "n/a"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("ratings %v, want %v", got, want)
	}
}
