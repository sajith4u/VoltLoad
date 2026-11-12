// Package webperf analyses how fast a web frontend (especially a React app) loads in a real browser:
// page timings and Core Web Vitals, every asset and API call, caching on a repeat visit, and the
// React build in use, then turns that into recommendations.
package webperf

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
)

// Analysis is the result of analysing one URL.
type Analysis struct {
	ID              int64            `json:"id"`
	URL             string           `json:"url"`
	FinalURL        string           `json:"finalUrl"` // after redirects
	StartedAt       string           `json:"startedAt"`
	Browser         string           `json:"browser"`
	React           ReactInfo        `json:"react"`
	Vitals          []Vital          `json:"vitals"` // first visit
	First           Load             `json:"first"`  // empty cache
	Repeat          Load             `json:"repeat"` // same browser, warm cache
	Assets          []Asset          `json:"assets"` // first visit, the document first, then by start time
	Recommendations []Recommendation `json:"recommendations"`
	Settled         bool             `json:"settled"` // false if the page was still making requests when measured
}

type ReactInfo struct {
	Detected  bool   `json:"detected"`
	Version   string `json:"version"`
	DevBuild  bool   `json:"devBuild"`
	Framework string `json:"framework"` // Next.js, Gatsby, Remix / React Router, Vite dev server, or ""
}

// Load is one visit's page-level numbers. Times are ms from navigation start.
type Load struct {
	Status             int     `json:"status"`
	TTFBMs             float64 `json:"ttfbMs"`
	FCPMs              float64 `json:"fcpMs"`
	LCPMs              float64 `json:"lcpMs"`
	DOMContentLoadedMs float64 `json:"domContentLoadedMs"`
	LoadMs             float64 `json:"loadMs"`
	TBTMs              float64 `json:"tbtMs"` // total blocking time: long-task time over 50ms each
	LongTasks          int     `json:"longTasks"`
	CLS                float64 `json:"cls"`
	Requests           int     `json:"requests"`
	TransferBytes      int64   `json:"transferBytes"`
	DecodedBytes       int64   `json:"decodedBytes"`
	DOMNodes           int     `json:"domNodes"`
}

// Asset is one request of the first visit. Sizes are 0 for cross-origin files that don't send
// Timing-Allow-Origin (the browser hides them).
type Asset struct {
	URL            string  `json:"url"`
	Kind           string  `json:"kind"` // document, script, stylesheet, image, font, api, other
	Protocol       string  `json:"protocol"`
	StartMs        float64 `json:"startMs"`
	DurationMs     float64 `json:"durationMs"`
	WaitMs         float64 `json:"waitMs"` // until the first response byte
	TransferBytes  int64   `json:"transferBytes"`
	EncodedBytes   int64   `json:"encodedBytes"` // body as sent (compressed)
	DecodedBytes   int64   `json:"decodedBytes"` // body after decompression
	Status         int     `json:"status"`
	RenderBlocking bool    `json:"renderBlocking"`
	ThirdParty     bool    `json:"thirdParty"`
	Repeat         string  `json:"repeat"` // on the repeat visit: cached, revalidated, downloaded, or "" if unknown
}

type Vital struct {
	Key    string  `json:"key"`
	Name   string  `json:"name"`
	Value  float64 `json:"value"`
	Unit   string  `json:"unit"` // ms or "" (CLS)
	Good   float64 `json:"good"` // at or below: good
	Poor   float64 `json:"poor"` // above: poor; between: needs improvement
	Rating string  `json:"rating"`
}

// Analyze loads rawURL twice in a fresh headless Chrome, first with an empty cache and then as a
// repeat visit, and reports what it measured.
func Analyze(ctx context.Context, rawURL string) (Analysis, error) {
	u, err := normalizeURL(rawURL)
	if err != nil {
		return Analysis{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	b, err := launch(ctx)
	if err != nil {
		return Analysis{}, err
	}
	defer b.close()
	if err := b.call(ctx, "Page.enable", nil, nil); err != nil { // without it the probe below isn't injected
		return Analysis{}, err
	}
	if err := b.call(ctx, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": probeJS}, nil); err != nil {
		return Analysis{}, err
	}

	a := Analysis{URL: u, StartedAt: time.Now().UTC().Format(time.RFC3339), Browser: b.version}
	first, settled, err := b.visit(ctx, u)
	if err != nil {
		return a, err
	}
	if err := b.navigate(ctx, "about:blank"); err != nil {
		return a, err
	}
	repeat, _, err := b.visit(ctx, u)
	if err != nil {
		return a, err
	}
	build(&a, first, repeat)
	a.Settled = settled
	a.Recommendations = recommend(a)
	return a, nil
}

func normalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("enter the URL of the page to analyse")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%q is not an http(s) URL", raw)
	}
	return u.String(), nil
}

// probeJS runs before any page script. It records metrics the page can only report as they happen,
// and stands in for the React DevTools hook: React registers each renderer with it, which tells us
// the React version and whether it is a development build.
const probeJS = `(() => {
  const vl = window.__voltload = {lcp: 0, cls: 0, longTasks: [], react: null};
  try { performance.setResourceTimingBufferSize(5000); } catch (e) {}
  const observe = (type, fn) => { try { new PerformanceObserver(l => l.getEntries().forEach(fn)).observe({type, buffered: true}); } catch (e) {} };
  observe('largest-contentful-paint', e => { vl.lcp = e.startTime; });
  observe('layout-shift', e => { if (!e.hadRecentInput) vl.cls += e.value; });
  observe('longtask', e => { vl.longTasks.push([e.startTime, e.duration]); });
  if (!window.__REACT_DEVTOOLS_GLOBAL_HOOK__) {
    let id = 0;
    window.__REACT_DEVTOOLS_GLOBAL_HOOK__ = {
      renderers: new Map(), supportsFiber: true, isDisabled: false,
      inject(r) { this.renderers.set(++id, r); vl.react = {version: r.version || '', dev: r.bundleType === 1}; return id; },
      checkDCE() {}, onCommitFiberRoot() {}, onCommitFiberUnmount() {}, onPostCommitFiberRoot() {},
    };
  }
})()`

// collectJS reads everything the page measured, once it has settled.
const collectJS = `(() => {
  const nav = performance.getEntriesByType('navigation')[0] || {};
  const paint = performance.getEntriesByName('first-contentful-paint')[0];
  const vl = window.__voltload || {};
  const isRoot = el => Object.keys(el).some(k => k.startsWith('__reactContainer$') || k.startsWith('__reactFiber$') || k === '_reactRootContainer');
  const near = document.body ? [...document.body.querySelectorAll(':scope > *, :scope > * > *')] : [];
  let framework = '';
  if (window.__NEXT_DATA__ || window.next || document.getElementById('__next')) framework = 'Next.js';
  else if (window.___gatsby || document.getElementById('___gatsby')) framework = 'Gatsby';
  else if (window.__remixContext || window.__reactRouterContext) framework = 'Remix / React Router';
  else if (document.querySelector('script[src*="/@vite/client"]')) framework = 'Vite dev server';
  return {
    url: location.href,
    nav: {ttfb: nav.responseStart || 0, end: nav.responseEnd || 0, dcl: nav.domContentLoadedEventEnd || 0, load: nav.loadEventEnd || 0,
          transfer: nav.transferSize || 0, encoded: nav.encodedBodySize || 0, decoded: nav.decodedBodySize || 0,
          status: nav.responseStatus || 0, protocol: nav.nextHopProtocol || ''},
    fcp: paint ? paint.startTime : 0, lcp: vl.lcp || 0, cls: vl.cls || 0, longTasks: vl.longTasks || [],
    domNodes: document.getElementsByTagName('*').length,
    react: vl.react || null, reactDom: !!document.querySelector('[data-reactroot]') || near.some(isRoot), framework,
    resources: performance.getEntriesByType('resource').map(e => ({
      url: e.name, initiator: e.initiatorType, contentType: e.contentType || '', protocol: e.nextHopProtocol,
      start: e.startTime, duration: e.duration, wait: e.responseStart > 0 ? e.responseStart - e.startTime : 0,
      transfer: e.transferSize, encoded: e.encodedBodySize, decoded: e.decodedBodySize,
      status: e.responseStatus || 0, blocking: e.renderBlockingStatus === 'blocking'})),
  };
})()`

type rawVisit struct {
	URL string
	Nav struct {
		TTFB, End, DCL, Load       float64
		Transfer, Encoded, Decoded int64
		Status                     int
		Protocol                   string
	}
	FCP, LCP, CLS float64
	LongTasks     [][2]float64
	DOMNodes      int
	React         *struct {
		Version string
		Dev     bool
	}
	ReactDOM  bool
	Framework string
	Resources []rawResource
}

type rawResource struct {
	URL, Initiator, ContentType, Protocol string
	Start, Duration, Wait                 float64
	Transfer, Encoded, Decoded            int64
	Status                                int
	Blocking                              bool
}

// visit loads u and returns its measurements once the page has stopped making requests for 2s (or
// after 30s, with settled false: some pages poll forever).
func (b *browser) visit(ctx context.Context, u string) (rawVisit, bool, error) {
	var v rawVisit
	if err := b.navigate(ctx, u); err != nil {
		return v, false, err
	}
	settled := false
	last, quietSince := -1, time.Now()
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); {
		select {
		case <-ctx.Done():
			return v, false, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
		var s struct {
			Href, Ready string
			N           int
		}
		// errors here are expected while the page navigates (its JS context is replaced); keep polling
		if b.eval(ctx, `({href: location.href, ready: document.readyState, n: performance.getEntriesByType('resource').length})`, &s) != nil ||
			s.Href == "about:blank" || s.Ready != "complete" {
			last = -1
			continue
		}
		if s.N != last {
			last, quietSince = s.N, time.Now()
			continue
		}
		if time.Since(quietSince) >= 2*time.Second {
			settled = true
			break
		}
	}
	err := b.eval(ctx, collectJS, &v)
	return v, settled, err
}

func build(a *Analysis, first, repeat rawVisit) {
	a.FinalURL = first.URL
	a.First = load(first)
	a.Repeat = load(repeat)
	a.React = ReactInfo{Detected: first.React != nil || first.ReactDOM, Framework: first.Framework}
	if first.React != nil {
		a.React.Version, a.React.DevBuild = first.React.Version, first.React.Dev
	}
	a.Vitals = vitals(a.First)

	host := siteOf(first.URL)
	onRepeat := map[string]rawResource{}
	for _, r := range repeat.Resources {
		onRepeat[r.URL] = r
	}
	a.Assets = append(a.Assets, Asset{
		URL: first.URL, Kind: "document", Protocol: first.Nav.Protocol, DurationMs: first.Nav.End, WaitMs: first.Nav.TTFB,
		TransferBytes: first.Nav.Transfer, EncodedBytes: first.Nav.Encoded, DecodedBytes: first.Nav.Decoded, Status: first.Nav.Status,
	})
	for _, r := range first.Resources {
		as := Asset{
			URL: r.URL, Kind: kind(r), Protocol: r.Protocol, StartMs: r.Start, DurationMs: r.Duration, WaitMs: r.Wait,
			TransferBytes: r.Transfer, EncodedBytes: r.Encoded, DecodedBytes: r.Decoded, Status: r.Status,
			RenderBlocking: r.Blocking, ThirdParty: siteOf(r.URL) != host,
		}
		if rr, ok := onRepeat[r.URL]; ok && rr.Decoded > 0 {
			switch {
			case rr.Transfer == 0:
				as.Repeat = "cached"
			case rr.Transfer < rr.Encoded: // only headers came over the network: a 304
				as.Repeat = "revalidated"
			default:
				as.Repeat = "downloaded"
			}
		}
		a.Assets = append(a.Assets, as)
	}
	rest := a.Assets[1:] // the document stays first
	sort.SliceStable(rest, func(i, j int) bool { return rest[i].StartMs < rest[j].StartMs })
}

func load(v rawVisit) Load {
	l := Load{
		Status: v.Nav.Status, TTFBMs: v.Nav.TTFB, FCPMs: v.FCP, LCPMs: v.LCP, DOMContentLoadedMs: v.Nav.DCL, LoadMs: v.Nav.Load,
		CLS: v.CLS, Requests: len(v.Resources) + 1, TransferBytes: v.Nav.Transfer, DecodedBytes: v.Nav.Decoded,
		LongTasks: len(v.LongTasks), DOMNodes: v.DOMNodes,
	}
	for _, t := range v.LongTasks {
		l.TBTMs += max(0, t[1]-50)
	}
	for _, r := range v.Resources {
		l.TransferBytes += r.Transfer
		l.DecodedBytes += r.Decoded
	}
	return l
}

var extKinds = map[string]string{
	".js": "script", ".mjs": "script", ".cjs": "script", ".css": "stylesheet",
	".png": "image", ".jpg": "image", ".jpeg": "image", ".gif": "image", ".webp": "image", ".avif": "image", ".svg": "image", ".ico": "image",
	".woff": "font", ".woff2": "font", ".ttf": "font", ".otf": "font", ".eot": "font",
}

func kind(r rawResource) string {
	switch r.Initiator {
	case "fetch", "xmlhttprequest", "beacon":
		return "api"
	}
	ct := r.ContentType
	switch {
	case strings.Contains(ct, "javascript"):
		return "script"
	case strings.Contains(ct, "css"):
		return "stylesheet"
	case strings.HasPrefix(ct, "image/"):
		return "image"
	case strings.HasPrefix(ct, "font/"):
		return "font"
	}
	if u, err := url.Parse(r.URL); err == nil {
		if k, ok := extKinds[strings.ToLower(path.Ext(u.Path))]; ok {
			return k
		}
	}
	switch r.Initiator {
	case "script":
		return "script"
	case "img":
		return "image"
	}
	return "other"
}

// siteOf returns the last two labels of the host, so cdn.example.com and www.example.com match.
// shortcut: wrong for two-part suffixes like example.co.uk (all of *.co.uk match); use the public
// suffix list (golang.org/x/net/publicsuffix) if third-party detection matters there.
func siteOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	labels := strings.Split(u.Hostname(), ".")
	if len(labels) > 2 {
		labels = labels[len(labels)-2:]
	}
	return strings.Join(labels, ".")
}

func vitals(l Load) []Vital {
	vs := []Vital{
		{Key: "ttfb", Name: "Time to first byte", Value: l.TTFBMs, Unit: "ms", Good: 800, Poor: 1800},
		{Key: "fcp", Name: "First contentful paint", Value: l.FCPMs, Unit: "ms", Good: 1800, Poor: 3000},
		{Key: "lcp", Name: "Largest contentful paint", Value: l.LCPMs, Unit: "ms", Good: 2500, Poor: 4000},
		{Key: "tbt", Name: "Total blocking time", Value: l.TBTMs, Unit: "ms", Good: 200, Poor: 600},
		{Key: "cls", Name: "Cumulative layout shift", Value: l.CLS, Good: 0.1, Poor: 0.25},
		{Key: "load", Name: "Page load", Value: l.LoadMs, Unit: "ms", Good: 3000, Poor: 6000},
	}
	for i, v := range vs {
		switch {
		case v.Value == 0 && (v.Key == "fcp" || v.Key == "lcp" || v.Key == "load"):
			vs[i].Rating = "n/a" // never happened (blank page) or not measurable
		case v.Value <= v.Good:
			vs[i].Rating = "good"
		case v.Value <= v.Poor:
			vs[i].Rating = "needs-improvement"
		default:
			vs[i].Rating = "poor"
		}
	}
	return vs
}
