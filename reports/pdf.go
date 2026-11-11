package reports

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	"voltload/internal/loader"

	"github.com/go-pdf/fpdf"
)

const (
	pageW     = 210.0 // A4 portrait, mm
	margin    = 15.0
	bodyW     = pageW - 2*margin
	maxVURows = 50
)

type rgb [3]int

var (
	inkC    = rgb{29, 33, 48}
	mutedC  = rgb{102, 108, 125}
	lineC   = rgb{225, 228, 236}
	tileC   = rgb{247, 248, 251}
	accentC = rgb{75, 63, 209}
	goodC   = rgb{0, 99, 0}
	badC    = rgb{180, 42, 42}
	// same metric colors as the app's charts
	rpsC     = rgb{235, 104, 52}
	latencyC = rgb{42, 120, 214}
	vusC     = rgb{98, 80, 214}
)

// pdfDoc wraps fpdf with the few layout helpers the report needs.
type pdfDoc struct {
	*fpdf.Fpdf
	tr func(string) string // UTF-8 -> cp1252, the encoding of the built-in fonts
}

func (d *pdfDoc) text(c rgb)                      { d.SetTextColor(c[0], c[1], c[2]) }
func (d *pdfDoc) draw(c rgb)                      { d.SetDrawColor(c[0], c[1], c[2]) }
func (d *pdfDoc) fill(c rgb)                      { d.SetFillColor(c[0], c[1], c[2]) }
func (d *pdfDoc) font(style string, size float64) { d.SetFont("Helvetica", style, size) }

// room starts a new page unless h mm still fit on this one.
func (d *pdfDoc) room(h float64) {
	if _, pageH := d.GetPageSize(); d.GetY()+h > pageH-margin-8 {
		d.AddPage()
	}
}

func (d *pdfDoc) heading(s string) {
	d.room(24)
	d.Ln(5)
	d.font("B", 11)
	d.text(inkC)
	d.CellFormat(bodyW, 7, d.tr(s), "", 1, "L", false, 0, "")
	d.draw(lineC)
	d.Line(margin, d.GetY(), margin+bodyW, d.GetY())
	d.Ln(2)
}

// table draws a header row and rows; colors[i] (optional) sets the text color of row i.
func (d *pdfDoc) table(headers []string, widths []float64, rows [][]string, colors map[int]rgb) {
	header := func() {
		d.font("B", 8)
		d.text(mutedC)
		for i, h := range headers {
			d.CellFormat(widths[i], 6, d.tr(h), "B", 0, "L", false, 0, "")
		}
		d.Ln(-1)
	}
	header()
	for i, row := range rows {
		if _, pageH := d.GetPageSize(); d.GetY()+6 > pageH-margin-8 {
			d.AddPage()
			header()
		}
		d.font("", 9)
		d.text(inkC)
		if c, ok := colors[i]; ok {
			d.text(c)
		}
		for j, cell := range row {
			d.CellFormat(widths[j], 6, d.tr(clip(cell, int(widths[j]/1.75))), "B", 0, "L", false, 0, "")
		}
		d.Ln(-1)
	}
}

// kv draws "label   value" lines; long values wrap.
func (d *pdfDoc) kv(label, value string) {
	d.room(6)
	d.font("", 9)
	d.text(mutedC)
	d.CellFormat(38, 5.5, d.tr(label), "", 0, "L", false, 0, "")
	d.text(inkC)
	d.MultiCell(bodyW-38, 5.5, d.tr(value), "", "L", false)
}

func clip(s string, n int) string {
	r := []rune(s)
	if n < 4 || len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// chart draws one single-series line chart from a zero baseline.
func (d *pdfDoc) chart(title, unit string, c rgb, xs, ys []float64) {
	const h, padL, padB = 34.0, 14.0, 5.0
	d.room(h + 10)
	d.font("B", 8)
	d.text(inkC)
	d.fill(c)
	d.Rect(margin, d.GetY()+1.6, 2.4, 2.4, "F")
	d.SetX(margin + 4)
	d.CellFormat(bodyW-4, 5.5, d.tr(title+" ("+unit+")"), "", 1, "L", false, 0, "")

	top, left := d.GetY(), margin+padL
	w, plotH := bodyW-padL, h-padB
	yMax := niceMax(maxOf(ys))
	xMax := math.Max(xs[len(xs)-1], 1e-9)
	px := func(x float64) float64 { return left + x/xMax*w }
	py := func(y float64) float64 { return top + plotH - y/yMax*plotH }

	d.font("", 7)
	d.text(mutedC)
	d.draw(lineC)
	d.SetLineWidth(0.15)
	for _, t := range []float64{0, yMax / 2, yMax} {
		d.Line(left, py(t), left+w, py(t))
		d.SetXY(margin, py(t)-2)
		d.CellFormat(padL-1.5, 4, trimNum(t), "", 0, "R", false, 0, "")
	}
	d.SetXY(left, top+plotH+0.5)
	d.CellFormat(w, 4, "0s", "", 0, "L", false, 0, "")
	d.SetXY(left, top+plotH+0.5)
	d.CellFormat(w, 4, trimNum(xs[len(xs)-1])+"s", "", 0, "R", false, 0, "")

	d.draw(c)
	d.SetLineWidth(0.45)
	for i := 1; i < len(xs); i++ {
		d.Line(px(xs[i-1]), py(ys[i-1]), px(xs[i]), py(ys[i]))
	}
	d.SetLineWidth(0.2)
	d.SetXY(margin, top+h+2)
}

func maxOf(v []float64) float64 {
	m := 0.0
	for _, x := range v {
		m = math.Max(m, x)
	}
	return m
}

func niceMax(v float64) float64 {
	if v <= 0 {
		return 1
	}
	mag := math.Pow(10, math.Floor(math.Log10(v)))
	for _, m := range []float64{1, 2, 5, 10} {
		if m*mag >= v {
			return m * mag
		}
	}
	return 10 * mag
}

func trimNum(v float64) string { return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64) }

func writePDF(w io.Writer, r loader.Result) error {
	d := &pdfDoc{Fpdf: fpdf.New("P", "mm", "A4", "")}
	d.tr = d.UnicodeTranslatorFromDescriptor("")
	d.SetMargins(margin, margin, margin)
	d.SetAutoPageBreak(true, margin+8)
	d.SetTitle("VoltLoad report - "+r.Config.Name, true)
	d.SetCreator("VoltLoad", true)
	d.AliasNbPages("")
	d.SetFooterFunc(func() {
		d.SetY(-12)
		d.font("", 8)
		d.text(mutedC)
		d.CellFormat(bodyW/2, 5, d.tr("VoltLoad performance report · "+date(r)), "", 0, "L", false, 0, "")
		d.CellFormat(bodyW/2, 5, fmt.Sprintf("Page %d of {nb}", d.PageNo()), "", 0, "R", false, 0, "")
	})
	d.AddPage()

	// ---- title block ----
	d.font("B", 9)
	d.text(accentC)
	d.CellFormat(bodyW, 5, d.tr("VOLTLOAD  ·  PERFORMANCE TEST REPORT"), "", 1, "L", false, 0, "")
	d.font("B", 18)
	d.text(inkC)
	name := r.Config.Name
	if name == "" {
		name = r.Config.Method + " " + r.Config.URL
	}
	d.MultiCell(bodyW, 9, d.tr(name), "", "L", false)
	d.font("", 9)
	d.text(mutedC)
	meta := fmt.Sprintf("%s %s  ·  started %s", r.Config.Method, r.Config.URL, date(r))
	if r.Stopped {
		meta += "  ·  stopped early"
	}
	d.MultiCell(bodyW, 5, d.tr(meta), "", "L", false)
	if len(r.Checks) > 0 {
		d.Ln(1)
		d.font("B", 11)
		d.text(goodC)
		if !r.Passed {
			d.text(badC)
		}
		d.CellFormat(bodyW, 7, "Thresholds: "+passFail(r.Passed), "", 1, "L", false, 0, "")
	}
	d.Ln(2)

	// ---- summary tiles ----
	errRate := 0.0
	if r.Total > 0 {
		errRate = float64(r.Failed) / float64(r.Total) * 100
	}
	tiles := [][2]string{
		{"Requests", strconv.FormatInt(r.Total, 10)},
		{"Success", strconv.FormatInt(r.Success, 10)},
		{"Failed", fmt.Sprintf("%d (%s%%)", r.Failed, f2(errRate))},
		{"Requests/sec", f2(r.RPS)},
		{"Duration", f2(r.DurationSec) + " s"},
	}
	const gap = 3.0
	tw := (bodyW - gap*float64(len(tiles)-1)) / float64(len(tiles))
	y := d.GetY()
	for i, t := range tiles {
		x := margin + float64(i)*(tw+gap)
		d.fill(tileC)
		d.draw(lineC)
		d.Rect(x, y, tw, 16, "FD")
		d.SetXY(x+2, y+1.5)
		d.font("", 8)
		d.text(mutedC)
		d.CellFormat(tw-4, 5, t[0], "", 0, "L", false, 0, "")
		d.SetXY(x+2, y+7.5)
		d.font("B", 12)
		d.text(inkC)
		if i == 2 && r.Failed > 0 {
			d.text(badC)
		}
		d.CellFormat(tw-4, 6, t[1], "", 0, "L", false, 0, "")
	}
	d.SetXY(margin, y+18)

	// ---- latency ----
	d.heading("Response time (ms)")
	l := r.LatencyMs
	d.table([]string{"min", "avg", "p50", "p90", "p95", "p99", "max"}, evenWidths(7),
		[][]string{{f2(l.Min), f2(l.Avg), f2(l.P50), f2(l.P90), f2(l.P95), f2(l.P99), f2(l.Max)}}, nil)

	// ---- thresholds ----
	if len(r.Checks) > 0 {
		d.heading("Thresholds")
		rows, colors := [][]string{}, map[int]rgb{}
		for i, c := range r.Checks {
			rows = append(rows, []string{fmt.Sprintf("%s %s %s", c.Threshold.Metric, c.Threshold.Op, trimNum(c.Threshold.Value)), f2(c.Actual), passFail(c.Passed)})
			colors[i] = goodC
			if !c.Passed {
				colors[i] = badC
			}
		}
		d.table([]string{"Rule", "Actual", "Result"}, []float64{90, 45, 45}, rows, colors)
	}

	// ---- timeline charts ----
	if len(r.Timeline) > 1 {
		d.heading("Timeline")
		xs := make([]float64, len(r.Timeline))
		rps, lat, vus := make([]float64, len(xs)), make([]float64, len(xs)), make([]float64, len(xs))
		for i, s := range r.Timeline {
			xs[i], rps[i], lat[i], vus[i] = s.ElapsedSec, s.RPS, s.AvgLatencyMs, float64(s.ActiveVUs)
		}
		d.chart("Requests per second", "req/s", rpsC, xs, rps)
		d.chart("Average response time", "ms, cumulative", latencyC, xs, lat)
		d.chart("Active virtual users", "VUs", vusC, xs, vus)
	}

	// ---- configuration ----
	d.heading("Test configuration")
	c := r.Config
	d.kv("Request", c.Method+" "+c.URL)
	d.kv("Timeout", fmt.Sprintf("%d ms per request", c.TimeoutMs))
	if len(c.Headers) > 0 {
		names := make([]string, 0, len(c.Headers))
		for k := range c.Headers {
			names = append(names, k)
		}
		sort.Strings(names)
		// values can hold credentials, so reports list header names only
		d.kv("Headers", strings.Join(names, ", ")+" (values hidden)")
	}
	if c.Body != "" {
		d.kv("Body", fmt.Sprintf("%d bytes", len(c.Body)))
	}
	if len(c.Stages) > 0 {
		d.kv("Workload", fmt.Sprintf("%d stages, peak %d virtual users, %d s planned", len(c.Stages), c.VUs, c.DurationSec))
	} else {
		d.kv("Workload", fmt.Sprintf("%d constant virtual users, %s", c.VUs, limitText(c)))
	}
	if c.Requests > 0 {
		d.kv("Max requests", strconv.Itoa(c.Requests))
	}
	if c.ThinkMaxMs > 0 {
		d.kv("Think time", fmt.Sprintf("%d-%d ms between each user's requests", c.ThinkMinMs, c.ThinkMaxMs))
	} else {
		d.kv("Think time", "none (each user sends requests back to back)")
	}
	if len(c.Stages) > 0 {
		d.Ln(1)
		rows := [][]string{}
		from := 0
		for i, s := range c.Stages {
			rows = append(rows, []string{strconv.Itoa(i + 1), strconv.Itoa(s.DurationSec), fmt.Sprintf("%d -> %d", from, s.VUs)})
			from = s.VUs
		}
		d.table([]string{"Stage", "Duration (s)", "Virtual users"}, []float64{30, 50, 100}, rows, nil)
	}

	// ---- status codes & errors ----
	if codes := StatusCodes(r); len(codes) > 0 {
		d.heading("HTTP status codes")
		rows, colors := [][]string{}, map[int]rgb{}
		for i, sc := range codes {
			share := float64(sc.Count) / float64(r.Total) * 100
			rows = append(rows, []string{"HTTP " + strconv.Itoa(sc.Code), strconv.FormatInt(sc.Count, 10), f2(share) + "%"})
			if sc.Code >= 400 {
				colors[i] = badC
			}
		}
		d.table([]string{"Status", "Responses", "Share"}, []float64{60, 60, 60}, rows, colors)
	}
	if len(r.Errors) > 0 {
		d.heading("Errors")
		msgs := make([]string, 0, len(r.Errors))
		for e := range r.Errors {
			msgs = append(msgs, e)
		}
		sort.Slice(msgs, func(i, j int) bool { return r.Errors[msgs[i]] > r.Errors[msgs[j]] })
		rows, colors := [][]string{}, map[int]rgb{}
		for i, e := range msgs {
			rows = append(rows, []string{strconv.Itoa(r.Errors[e]), e})
			colors[i] = badC
		}
		d.table([]string{"Count", "Error"}, []float64{25, 155}, rows, colors)
	}

	// ---- virtual users ----
	if len(r.VUs) > 0 {
		d.heading("Virtual users")
		minReq, maxReq, idle := r.VUs[0].Requests, r.VUs[0].Requests, 0
		for _, v := range r.VUs {
			minReq, maxReq = min(minReq, v.Requests), max(maxReq, v.Requests)
			if v.Requests == 0 {
				idle++
			}
		}
		d.font("", 9)
		d.text(mutedC)
		d.MultiCell(bodyW, 5, d.tr(fmt.Sprintf(
			"%d virtual users ran as independent users, each with its own connection and cookie session. Requests per user: %d to %d. %d users sent no requests.",
			len(r.VUs), minReq, maxReq, idle)), "", "L", false)
		d.Ln(1)
		rows, colors := [][]string{}, map[int]rgb{}
		for i, v := range r.VUs {
			if i == maxVURows {
				break
			}
			rows = append(rows, []string{"VU " + strconv.Itoa(v.ID), strconv.FormatInt(v.Requests, 10), strconv.FormatInt(v.Failed, 10), f2(v.AvgMs)})
			if v.Failed > 0 {
				colors[i] = badC
			}
		}
		d.table([]string{"User", "Requests", "Failed", "Avg response (ms)"}, evenWidths(4), rows, colors)
		if len(r.VUs) > maxVURows {
			d.font("I", 8)
			d.text(mutedC)
			d.CellFormat(bodyW, 6, fmt.Sprintf("Showing the first %d of %d virtual users. The JSON report lists all of them.", maxVURows, len(r.VUs)), "", 1, "L", false, 0, "")
		}
	}

	return d.Output(w)
}

func evenWidths(n int) []float64 {
	w := make([]float64, n)
	for i := range w {
		w[i] = bodyW / float64(n)
	}
	return w
}

func limitText(c loader.Config) string {
	if c.DurationSec > 0 {
		return fmt.Sprintf("for %d s", c.DurationSec)
	}
	return "until the request limit"
}

func date(r loader.Result) string { return r.StartedAt.Format("2006-01-02 15:04:05 MST") }
