package gui

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"voltload/internal/loader"
)

func TestBackendLogWritesToFile(t *testing.T) {
	a := &App{dataDir: t.TempDir()}
	a.openLog()

	var wg sync.WaitGroup
	for vu := 1; vu <= 8; vu++ { // many VUs log at once; every line must arrive whole
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.logf("[suite 1 %q] %s", "Checkout", loader.LogEntry{ElapsedSec: 1.5, Level: "ok", Kind: "request", VU: vu, Message: "GET /items", Status: 200, LatencyMs: 12.3}.Line())
		}()
	}
	wg.Wait()
	a.logFile.Close()

	b, err := os.ReadFile(filepath.Join(a.dataDir, "logs", "voltload.log"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 8 {
		t.Fatalf("want 8 lines, got %d:\n%s", len(lines), b)
	}
	for _, l := range lines {
		if !strings.Contains(l, `[suite 1 "Checkout"]`) || !strings.Contains(l, "GET /items  200  12.3ms") {
			t.Fatalf("malformed line: %q", l)
		}
	}
}
