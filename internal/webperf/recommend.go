package webperf

import (
	"fmt"
	"net/url"
	"sort"
)

type Recommendation struct {
	Severity string   `json:"severity"` // critical, warning, info
	Title    string   `json:"title"`
	Detail   string   `json:"detail"` // what to do
	Items    []string `json:"items"`  // the assets or calls it is about, worst first
}

const maxItems = 8

func ms(v float64) string {
	if v >= 1000 {
		return fmt.Sprintf("%.2f s", v/1000)
	}
	return fmt.Sprintf("%.0f ms", v)
}

func size(b int64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(b)/(1<<10))
	}
	return fmt.Sprintf("%d B", b)
}

// short trims a URL to its path for display, keeping the host for third-party files.
func short(raw string, page string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	s := u.Path
	if u.RawQuery != "" {
		s += "?" + u.RawQuery
	}
	if siteOf(raw) != siteOf(page) {
		s = u.Host + s
	}
	if len(s) > 90 {
		s = s[:87] + "…"
	}
	return s
}

// bytesOf is what a file cost to download: the transfer size, or the body size when the browser
// hides the transfer size (cross-origin without Timing-Allow-Origin shows neither).
func bytesOf(a Asset) int64 { return max(a.TransferBytes, a.EncodedBytes) }

func recommend(a Analysis) []Recommendation {
	var out []Recommendation
	add := func(sev, title, detail string, items []string) {
		if len(items) > maxItems {
			items = append(items[:maxItems:maxItems], fmt.Sprintf("…and %d more", len(items)-maxItems))
		}
		out = append(out, Recommendation{Severity: sev, Title: title, Detail: detail, Items: items})
	}
	f := a.First
	page := a.FinalURL
	itemsOf := func(as []Asset, line func(Asset) string) []string {
		var s []string
		for _, x := range as {
			s = append(s, line(x))
		}
		return s
	}
	pick := func(keep func(Asset) bool, worst func(Asset) float64) []Asset {
		var s []Asset
		for _, x := range a.Assets {
			if keep(x) {
				s = append(s, x)
			}
		}
		sort.SliceStable(s, func(i, j int) bool { return worst(s[i]) > worst(s[j]) })
		return s
	}

	// --- the page itself
	if f.Status >= 400 {
		add("critical", fmt.Sprintf("The page returned HTTP %d", f.Status),
			"The measurements below describe an error page. Check the URL and that the site is up and doesn't need a login.", nil)
	}
	if a.React.DevBuild {
		add("critical", "React development build in use",
			"Development builds are several times slower and much larger: they run extra checks and warnings on every render. "+
				"Build with NODE_ENV=production (npm run build) and serve the build output, not the dev server.", nil)
	}
	if f.TTFBMs > 800 {
		add(sev(f.TTFBMs, 1800), "Slow server response ("+ms(f.TTFBMs)+" to first byte)",
			"Nothing can render until the HTML arrives. Serve the HTML from a CDN or edge cache, cache rendered pages on the server, "+
				"pre-render static routes, and check slow middleware, database calls or redirects before the response.", nil)
	}
	if f.LCPMs > 2500 {
		d := "The main content appears late. Preload the hero image or font (<link rel=preload>), don't lazy-load above-the-fold images, " +
			"and reduce the JavaScript that must run before the first render."
		if a.React.Detected && (a.React.Framework == "" || a.React.Framework == "Vite dev server") {
			d += " This looks like a client-rendered React app: the screen stays empty until the bundle downloads, parses and runs. " +
				"Server-side rendering or static generation (Next.js, Remix / React Router framework mode) or pre-rendering shows content before the JavaScript runs."
		}
		add(sev(f.LCPMs, 4000), "Largest content appears after "+ms(f.LCPMs), d, nil)
	} else if f.FCPMs > 1800 {
		add(sev(f.FCPMs, 3000), "First paint after "+ms(f.FCPMs),
			"Users see a blank screen until then. Cut render-blocking CSS and JavaScript, inline critical CSS, and show a server-rendered shell or skeleton.", nil)
	}
	if f.TBTMs > 200 {
		add(sev(f.TBTMs, 600), "Main thread blocked for "+ms(f.TBTMs)+" by "+count(f.LongTasks, "long task", "long tasks"),
			"The page can't respond to clicks or typing during long tasks. Split the bundle (React.lazy + Suspense, dynamic import()), "+
				"avoid heavy work during render (useMemo, move parsing and sorting to a Web Worker), render long lists with virtualization, "+
				"and load third-party scripts with defer or after interaction.", nil)
	}
	if f.CLS > 0.1 {
		add(sev(f.CLS, 0.25), fmt.Sprintf("Layout shifts while loading (CLS %.2f)", f.CLS),
			"Content jumps as the page loads. Give images and videos width/height (or aspect-ratio), reserve space for content that loads "+
				"from APIs (skeletons with fixed size), and avoid inserting banners above existing content.", nil)
	}

	// --- JavaScript
	scripts := pick(func(x Asset) bool { return x.Kind == "script" }, func(x Asset) float64 { return float64(max(bytesOf(x), x.DecodedBytes)) })
	var jsSent, jsParsed int64
	for _, s := range scripts {
		jsSent += bytesOf(s)
		jsParsed += s.DecodedBytes
	}
	if jsSent > 400<<10 || jsParsed > 1<<20 {
		sevJS := "warning"
		if jsSent > 1<<20 {
			sevJS = "critical"
		}
		add(sevJS, fmt.Sprintf("Large JavaScript payload: %s downloaded, %s to parse", size(jsSent), size(jsParsed)),
			"Every byte of JavaScript must be downloaded, parsed and executed before React can render. Split code by route with React.lazy "+
				"and dynamic import(), find what's large with a bundle analyzer (rollup-plugin-visualizer, webpack-bundle-analyzer, source-map-explorer), "+
				"replace heavy libraries (moment → date-fns/dayjs, lodash → per-function imports) and make sure tree-shaking works.",
			itemsOf(scripts, func(x Asset) string {
				return fmt.Sprintf("%s  (%s, %s unpacked)", short(x.URL, page), size(bytesOf(x)), size(x.DecodedBytes))
			}))
	}

	// --- compression
	uncompressed := pick(func(x Asset) bool {
		text := x.Kind == "document" || x.Kind == "script" || x.Kind == "stylesheet" || x.Kind == "api"
		return text && x.EncodedBytes > 2<<10 && x.EncodedBytes == x.DecodedBytes
	}, func(x Asset) float64 { return float64(x.DecodedBytes) })
	if len(uncompressed) > 0 {
		add("warning", count(len(uncompressed), "text response", "text responses")+" sent without compression",
			"Enable Brotli or gzip on the server or CDN for HTML, JavaScript, CSS and JSON. Text usually shrinks 70–90%.",
			itemsOf(uncompressed, func(x Asset) string { return fmt.Sprintf("%s  (%s)", short(x.URL, page), size(x.DecodedBytes)) }))
	}

	// --- images
	bigImages := pick(func(x Asset) bool { return x.Kind == "image" && bytesOf(x) > 200<<10 }, func(x Asset) float64 { return float64(bytesOf(x)) })
	if len(bigImages) > 0 {
		add("warning", count(len(bigImages), "large image", "large images"),
			"Serve WebP or AVIF, resize images to the size they are displayed (srcset/sizes), and add loading=\"lazy\" to images below the fold.",
			itemsOf(bigImages, func(x Asset) string { return fmt.Sprintf("%s  (%s)", short(x.URL, page), size(bytesOf(x))) }))
	}

	// --- API calls
	apis := pick(func(x Asset) bool { return x.Kind == "api" }, func(x Asset) float64 { return x.DurationMs })
	var slow []Asset
	worst := 0.0
	for _, x := range apis {
		if x.DurationMs > 1000 {
			slow = append(slow, x)
			worst = max(worst, x.DurationMs)
		}
	}
	if len(slow) > 0 {
		add(sev(worst, 3000), count(len(slow), "slow API call", "slow API calls")+" (slowest "+ms(worst)+")",
			"The UI waits on these responses. Speed up the endpoints (indexes, caching, smaller payloads, pagination), start requests in parallel "+
				"instead of one after another, cache responses on the client (TanStack Query, SWR), and show skeletons while loading.",
			itemsOf(slow, func(x Asset) string {
				return fmt.Sprintf("%s  %s (waiting %s)", ms(x.DurationMs), short(x.URL, page), ms(x.WaitMs))
			}))
	}
	if chain := waterfall(apis, page); len(chain) > 0 {
		add("warning", "API calls run one after another",
			"These calls start only when an earlier one finishes. In React this often comes from fetching in a child's useEffect after the parent's data loads. "+
				"Start independent requests together (Promise.all, route loaders, prefetching in the parent), or combine them into one endpoint.",
			chain)
	}
	if len(apis) > 15 {
		add("info", fmt.Sprintf("%d API calls during page load", len(apis)),
			"Many small calls add latency and server load. Batch them, add an endpoint shaped for this page (backend-for-frontend), or defer data that isn't on screen yet.", nil)
	}
	failed := pick(func(x Asset) bool { return x.Status >= 400 }, func(x Asset) float64 { return x.StartMs })
	if len(failed) > 0 {
		add("warning", count(len(failed), "request", "requests")+" failed",
			"Failed requests waste time and often mean missing data or a broken feature. Fix or remove them.",
			itemsOf(failed, func(x Asset) string { return fmt.Sprintf("HTTP %d  %s", x.Status, short(x.URL, page)) }))
	}

	// --- caching
	notCached := pick(func(x Asset) bool {
		static := x.Kind == "script" || x.Kind == "stylesheet" || x.Kind == "font" || x.Kind == "image"
		return static && (x.Repeat == "downloaded" || x.Repeat == "revalidated")
	}, func(x Asset) float64 { return float64(bytesOf(x)) })
	if len(notCached) > 0 {
		add("warning", count(len(notCached), "static file", "static files")+" not cached by the browser",
			"On the repeat visit these were downloaded again or re-checked with the server. Build tools put a content hash in file names "+
				"(main.3f9a1c.js), so serve those with Cache-Control: public, max-age=31536000, immutable, and keep only index.html short-lived.",
			itemsOf(notCached, func(x Asset) string { return fmt.Sprintf("%s  (%s on repeat)", short(x.URL, page), x.Repeat) }))
	}

	// --- request count, blocking resources, protocol, third parties, DOM
	blocking := pick(func(x Asset) bool { return x.RenderBlocking }, func(x Asset) float64 { return x.DurationMs })
	if len(blocking) > 2 {
		add("info", count(len(blocking), "render-blocking file", "render-blocking files"),
			"The browser won't paint until these load. Add defer or type=module to scripts, inline the critical CSS, and load the rest asynchronously.",
			itemsOf(blocking, func(x Asset) string { return fmt.Sprintf("%s  %s", ms(x.DurationMs), short(x.URL, page)) }))
	}
	http1 := pick(func(x Asset) bool { return !x.ThirdParty && x.Protocol == "http/1.1" }, func(x Asset) float64 { return x.StartMs })
	if len(http1) > 3 {
		add("info", fmt.Sprintf("%d of your files served over HTTP/1.1", len(http1)),
			"HTTP/1.1 fetches only a few files at a time per host. Enable HTTP/2 or HTTP/3 on the server or CDN.", nil)
	}
	if f.Requests > 100 {
		add("info", fmt.Sprintf("%d requests on first load", f.Requests),
			"Fewer, larger bundles and sprites (or icon fonts/SVG sprites) load faster than many small files.", nil)
	}
	if domains := thirdParties(a.Assets); len(domains) >= 5 {
		add("info", fmt.Sprintf("Files from %d third-party domains", len(domains)),
			"Each new domain costs a DNS lookup and connection, and third-party scripts compete with your app for the main thread. "+
				"Remove unused tags, load analytics and widgets after the page is interactive, and add <link rel=preconnect> for the ones you need early.",
			domains)
	}
	fonts := pick(func(x Asset) bool { return x.Kind == "font" }, func(x Asset) float64 { return float64(bytesOf(x)) })
	if len(fonts) > 4 {
		add("info", fmt.Sprintf("%d font files", len(fonts)),
			"Use fewer weights or a variable font, serve WOFF2, preload the one used above the fold, and set font-display: swap.", nil)
	}
	if f.DOMNodes > 1500 {
		add("info", fmt.Sprintf("Large DOM: %d elements", f.DOMNodes),
			"Big DOMs make React renders, style recalculation and layout slower. Virtualize long lists and tables (TanStack Virtual, react-window) and paginate.", nil)
	}
	if !a.Settled {
		add("info", "The page kept making requests",
			"It was still loading after 30 seconds, so it was measured as it was then. Polling or analytics beacons can cause this; "+
				"if it is unintended, check for request loops (for example a useEffect with a dependency that changes on every render).", nil)
	}

	rank := map[string]int{"critical": 0, "warning": 1, "info": 2}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].Severity] < rank[out[j].Severity] })
	return out
}

// count says "1 request" or "3 requests".
func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func sev(v, critical float64) string {
	if v > critical {
		return "critical"
	}
	return "warning"
}

// waterfall finds API calls that started within 100ms after another API call finished, a sign they
// wait for each other.
func waterfall(apis []Asset, page string) []string {
	var out []string
	for _, b := range apis {
		for _, a := range apis {
			end := a.StartMs + a.DurationMs
			if a.URL != b.URL && b.StartMs >= end && b.StartMs-end < 100 {
				out = append(out, fmt.Sprintf("%s → %s", short(a.URL, page), short(b.URL, page)))
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

func thirdParties(as []Asset) []string {
	count := map[string]int{}
	for _, x := range as {
		if x.ThirdParty {
			if u, err := url.Parse(x.URL); err == nil {
				count[u.Host]++
			}
		}
	}
	var hosts []string
	for h := range count {
		hosts = append(hosts, h)
	}
	sort.Slice(hosts, func(i, j int) bool {
		return count[hosts[i]] > count[hosts[j]] || count[hosts[i]] == count[hosts[j]] && hosts[i] < hosts[j]
	})
	for i, h := range hosts {
		hosts[i] = fmt.Sprintf("%s  (%d requests)", h, count[h])
	}
	return hosts
}
