import {useState} from 'react';
import {ExportRun, OpenRunPDF} from '../wailsjs/go/gui/App';
import {isDesktop} from './bridge';
import Chart, {Point} from './Chart';
import {loader} from '../wailsjs/go/models';

export function Stat({label, value, bad}: { label: string; value: string | number; bad?: boolean }) {
    return (
        <div className={`stat${bad ? ' bad' : ''}`}>
            <span>{label}</span>
            <strong>{value}</strong>
        </div>
    );
}

/** Pass/fail pill: icon + word, so state is never conveyed by color alone. */
export function Verdict({passed}: { passed: boolean }) {
    return <span className={`verdict ${passed ? 'pass' : 'fail'}`}>{passed ? '✓ PASS' : '✗ FAIL'}</span>;
}

const VU_ROWS = 100;

export default function ResultView({result: r, runId, showTimeline}: { result: loader.Result; runId: number; showTimeline?: boolean }) {
    const [note, setNote] = useState('');
    const l = r.latencyMs;
    const errRate = r.total ? (r.failed / r.total) * 100 : 0;
    const codes = Object.entries(r.statusCodes ?? {}).sort(([a], [b]) => Number(a) - Number(b));
    const errors = Object.entries(r.errors ?? {});
    const checks = r.checks ?? [];

    const vus = r.vus ?? [];
    const timeline = showTimeline ? r.timeline ?? [] : [];
    const series = (get: (s: loader.Snapshot) => number): Point[] =>
        timeline.map(s => ({x: s.elapsedSec, y: get(s), label: `${s.elapsedSec.toFixed(1)}s`}));
    const xEnd = timeline.length ? `${timeline[timeline.length - 1].elapsedSec.toFixed(0)}s` : undefined;

    const reportURL = (format: string, download = false) => `/api/runs/${runId}/report.${format}${download ? '?download=1' : ''}`;

    async function viewPDF() {
        if (!isDesktop) {
            // the browser's own PDF viewer, in a new tab
            if (!window.open(reportURL('pdf'), '_blank', 'noopener')) setNote('Allow pop-ups for this page to open the PDF report.');
            return;
        }
        try {
            setNote(`PDF report opened: ${await OpenRunPDF(runId)}`);
        } catch (err) {
            setNote(`Could not open the PDF report: ${err}`);
        }
    }

    async function exportAs(format: string) {
        if (!isDesktop) {
            Object.assign(document.createElement('a'), {href: reportURL(format, true)}).click();
            return;
        }
        try {
            const path = await ExportRun(runId, format);
            setNote(path ? `Saved to ${path}` : '');
        } catch (err) {
            setNote(`Export failed: ${err}`);
        }
    }

    return (
        <div className="card">
            <div className="card-head">
                <h3>
                    {r.config.name || `${r.config.method} ${r.config.url}`}
                    {checks.length > 0 && <Verdict passed={r.passed}/>}
                    {r.stopped && <span className="badge">stopped</span>}
                </h3>
                {runId > 0 && (
                    <div className="buttons">
                        <button className="primary" onClick={viewPDF}>View PDF report</button>
                        <span className="muted">Export</span>
                        <button onClick={() => exportAs('pdf')}>PDF</button>
                        <button onClick={() => exportAs('html')}>HTML</button>
                        <button onClick={() => exportAs('csv')}>CSV</button>
                        <button onClick={() => exportAs('json')}>JSON</button>
                    </div>
                )}
            </div>
            <p className="muted">
                {r.config.method} {r.config.url} · peak {r.config.vus} VUs
                {(r.config.stages?.length ?? 0) > 0 && ` · ${r.config.stages.length} stages`}
                {' · '}{new Date(r.startedAt).toLocaleString()}
            </p>
            {note && <p className="muted">{note}</p>}

            <div className="stats">
                <Stat label="Requests" value={r.total}/>
                <Stat label="Success" value={r.success}/>
                <Stat label="Failed" value={`${r.failed} (${errRate.toFixed(1)}%)`} bad={r.failed > 0}/>
                <Stat label="Req/s" value={r.rps.toFixed(1)}/>
                <Stat label="Duration" value={`${r.durationSec.toFixed(1)} s`}/>
            </div>

            <h4>Latency (ms)</h4>
            <table>
                <thead>
                <tr><th>min</th><th>avg</th><th>p50</th><th>p90</th><th>p95</th><th>p99</th><th>max</th></tr>
                </thead>
                <tbody>
                <tr>{[l.min, l.avg, l.p50, l.p90, l.p95, l.p99, l.max].map((v, i) => <td key={i}>{v.toFixed(1)}</td>)}</tr>
                </tbody>
            </table>

            {checks.length > 0 && <>
                <h4>Thresholds</h4>
                <table>
                    <thead><tr><th>Rule</th><th>Actual</th><th>Result</th></tr></thead>
                    <tbody>
                    {checks.map((c, i) => (
                        <tr key={i}>
                            <td>{c.threshold.metric} {c.threshold.op} {c.threshold.value}</td>
                            <td>{c.actual.toFixed(2)}</td>
                            <td><Verdict passed={c.passed}/></td>
                        </tr>
                    ))}
                    </tbody>
                </table>
            </>}

            {timeline.length > 1 && <>
                <h4>Timeline</h4>
                <div className="charts">
                    <Chart title="Requests/sec" unit="req/s" color="#eb6834" xEnd={xEnd} points={series(s => s.rps)}/>
                    <Chart title="Avg latency" unit="ms" color="#2a78d6" xEnd={xEnd} points={series(s => s.avgLatencyMs)}/>
                    <Chart title="Active VUs" unit="VUs" color="#6250d6" xEnd={xEnd} points={series(s => s.activeVUs)}/>
                </div>
            </>}

            {codes.length > 0 && <>
                <h4>Status codes</h4>
                <table>
                    <tbody>
                    {codes.map(([c, n]) => <tr key={c} className={Number(c) >= 400 ? 'bad' : ''}><td>HTTP {c}</td><td>{n}</td></tr>)}
                    </tbody>
                </table>
            </>}

            {errors.length > 0 && <>
                <h4>Errors</h4>
                <table>
                    <tbody>
                    {errors.map(([e, n]) => <tr key={e} className="bad"><td>{e}</td><td>{n}</td></tr>)}
                    </tbody>
                </table>
            </>}
            {vus.length > 0 && (
                <details className="vus">
                    <summary>Virtual users ({vus.length}) – each ran as its own user with a separate connection and session</summary>
                    <table>
                        <thead><tr><th>User</th><th>Requests</th><th>Failed</th><th>Avg response (ms)</th></tr></thead>
                        <tbody>
                        {vus.slice(0, VU_ROWS).map(v => (
                            <tr key={v.id} className={v.failed > 0 ? 'bad' : ''}>
                                <td>VU {v.id}</td><td>{v.requests}</td><td>{v.failed}</td><td>{v.avgMs.toFixed(1)}</td>
                            </tr>
                        ))}
                        </tbody>
                    </table>
                    {vus.length > VU_ROWS && <p className="muted">Showing the first {VU_ROWS} of {vus.length}. The JSON export lists all of them.</p>}
                </details>
            )}
        </div>
    );
}
