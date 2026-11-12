import {FormEvent, useEffect, useState} from 'react';
import {AnalyzeURL, DeleteAnalysis, GetAnalysis, ListAnalyses} from '../wailsjs/go/gui/App';
import {store, webperf} from '../wailsjs/go/models';
import {timeAgo} from './form';

// ---------- formatting ----------
const ms = (v: number) => v >= 1000 ? `${(v / 1000).toFixed(2)} s` : `${Math.round(v)} ms`;
const bytes = (b: number) => b >= 1 << 20 ? `${(b / (1 << 20)).toFixed(1)} MB` : b >= 1024 ? `${Math.round(b / 1024)} KB` : `${b} B`;
const sent = (a: webperf.Asset) => Math.max(a.transferBytes, a.encodedBytes);
const errText = (err: unknown) => String(err instanceof Error ? err.message : err).replace(/^Error: /, '');

/** Path (and query) of a URL, with the host when it isn't the page's own. */
function shortUrl(raw: string, page: string) {
    try {
        const u = new URL(raw), p = new URL(page);
        const s = u.pathname + u.search;
        return u.host === p.host ? s : u.host + s;
    } catch {
        return raw;
    }
}

const RATING: Record<string, string> = {'good': 'Good', 'needs-improvement': 'Needs improvement', 'poor': 'Poor', 'n/a': 'Not measured'};
const SEVERITY: Record<string, string> = {critical: 'Critical', warning: 'Warning', info: 'Tip'};
const KINDS = ['document', 'script', 'stylesheet', 'image', 'font', 'api', 'other'];
const KIND_LABEL: Record<string, string> = {document: 'HTML', script: 'JavaScript', stylesheet: 'CSS', image: 'Images', font: 'Fonts', api: 'API calls', other: 'Other'};

/** Analyses how a web frontend (React or not) loads: vitals, assets, API calls, caching, recommendations. */
export default function FrontendPage() {
    const [url, setUrl] = useState('');
    const [history, setHistory] = useState<store.AnalysisSummary[] | null>(null);
    const [result, setResult] = useState<webperf.Analysis | null>(null);
    const [busy, setBusy] = useState(false);
    const [elapsed, setElapsed] = useState(0);
    const [error, setError] = useState('');

    const reload = () => ListAnalyses().then(setHistory).catch(err => setError(errText(err)));
    useEffect(() => { reload(); }, []);
    useEffect(() => {
        if (!busy) return;
        const start = Date.now();
        const t = setInterval(() => setElapsed(Math.round((Date.now() - start) / 1000)), 1000);
        return () => { clearInterval(t); setElapsed(0); };
    }, [busy]);

    async function analyze(e: FormEvent) {
        e.preventDefault();
        setBusy(true);
        setError('');
        try {
            setResult(await AnalyzeURL(url));
            reload();
        } catch (err) {
            setError(errText(err));
        } finally {
            setBusy(false);
        }
    }

    async function open(id: number) {
        setError('');
        try { setResult(await GetAnalysis(id)); } catch (err) { setError(errText(err)); }
    }

    async function remove(id: number) {
        if (!confirm('Delete this analysis?')) return;
        await DeleteAnalysis(id).catch(err => setError(errText(err)));
        if (result?.id === id) setResult(null);
        reload();
    }

    return (
        <section>
            <div className="page-head">
                <div>
                    <h2>Frontend analysis</h2>
                    <p className="muted">Load a web page in a real browser and find what makes it slow: page timings, assets, API calls and caching, with React-specific advice.</p>
                </div>
            </div>
            <form className="card analyze-form" onSubmit={analyze}>
                <label className="grow">Page URL
                    <input type="text" inputMode="url" placeholder="https://app.example.com/dashboard" value={url}
                           onChange={e => setUrl(e.target.value)} required disabled={busy}/>
                </label>
                <button className="primary" disabled={busy}>{busy ? 'Analysing…' : 'Analyse'}</button>
                <p className="muted analyze-hint">
                    {busy ? <><span className="pulse"/> Loading the page twice in headless Chrome (empty cache, then a repeat visit)… {elapsed}s</>
                        : 'Opens the page in headless Chrome twice: once with an empty cache, then as a returning visitor. Pages behind a login show the login page.'}
                </p>
            </form>
            {error && <div className="error" role="alert">{error}</div>}

            <div className="fa-layout">
                <aside className="card fa-history">
                    <h3>History</h3>
                    {history?.length === 0 && <p className="muted">No analyses yet.</p>}
                    <ul>
                        {history?.map(h => (
                            <li key={h.id} className={result?.id === h.id ? 'active' : ''}>
                                <button className="fa-history-item" onClick={() => open(h.id)}>
                                    <span className="fa-url">{h.url.replace(/^https?:\/\//, '')}</span>
                                    <span className="muted">{timeAgo(h.startedAt)} · load {ms(h.loadMs)} · {h.issues ? `${h.issues} issue${h.issues > 1 ? 's' : ''}` : 'no issues'}</span>
                                </button>
                                <button className="link danger" onClick={() => remove(h.id)} aria-label={`Delete analysis of ${h.url}`}>✕</button>
                            </li>
                        ))}
                    </ul>
                </aside>
                <div className="fa-main">
                    {result ? <AnalysisView a={result}/> : (
                        <div className="card empty">
                            <h3>No analysis selected</h3>
                            <p className="muted">Enter a URL above, or pick a past analysis.</p>
                        </div>
                    )}
                </div>
            </div>
        </section>
    );
}

function AnalysisView({a}: { a: webperf.Analysis }) {
    const page = a.finalUrl || a.url;
    const apis = a.assets.filter(x => x.kind === 'api');
    const r = a.react;
    return (
        <>
            <div className="card">
                <div className="card-head">
                    <div style={{minWidth: 0}}>
                        <h3 className="fa-url">{page}</h3>
                        <p className="muted">{new Date(a.startedAt).toLocaleString()} · {a.browser}</p>
                    </div>
                </div>
                <div className="chips">
                    {r.detected
                        ? <span className="chip">React {r.version || ''}{r.version && (r.devBuild ? ' · development build' : ' · production build')}</span>
                        : <span className="chip">React not detected</span>}
                    {r.framework && <span className="chip">{r.framework}</span>}
                    {r.detected && !r.framework && <span className="chip">Client-side rendered</span>}
                    {!a.settled && <span className="chip warn">Still loading after 30 s</span>}
                </div>
                <div className="vitals">
                    {a.vitals.map(v => (
                        <div key={v.key} className={`vital r-${v.rating}`}>
                            <span className="muted">{v.name}</span>
                            <b>{v.rating === 'n/a' ? '–' : v.unit === 'ms' ? ms(v.value) : v.value.toFixed(3)}</b>
                            <span className="rating"><i/>{RATING[v.rating]}</span>
                        </div>
                    ))}
                </div>
            </div>

            <div className="card">
                <h3>Recommendations <span className="count">({a.recommendations?.length ?? 0})</span></h3>
                {!a.recommendations?.length && <p className="good">No issues found. This page loads well.</p>}
                <ul className="recs">
                    {a.recommendations?.map((rec, i) => (
                        <li key={i} className={rec.severity}>
                            <span className={`sev ${rec.severity}`}>{SEVERITY[rec.severity]}</span>
                            <div>
                                <b>{rec.title}</b>
                                <p>{rec.detail}</p>
                                {rec.items?.length > 0 && <ul className="rec-items">{rec.items.map((it, j) => <li key={j}>{it}</li>)}</ul>}
                            </div>
                        </li>
                    ))}
                </ul>
            </div>

            <div className="fa-grid">
                <div className="card">
                    <h3>First visit vs repeat visit</h3>
                    <p className="muted">Repeat visit: same browser, with what it cached the first time.</p>
                    <table>
                        <thead><tr><th/><th className="num">First</th><th className="num">Repeat</th></tr></thead>
                        <tbody>
                        {([['Page load', 'loadMs', ms], ['Largest contentful paint', 'lcpMs', ms], ['First contentful paint', 'fcpMs', ms],
                            ['Time to first byte', 'ttfbMs', ms], ['DOM content loaded', 'domContentLoadedMs', ms],
                            ['Requests', 'requests', String], ['Transferred', 'transferBytes', bytes]] as const).map(([label, key, f]) => (
                            <tr key={key}><td>{label}</td><td className="num">{f(a.first[key])}</td><td className="num">{f(a.repeat[key])}</td></tr>
                        ))}
                        </tbody>
                    </table>
                </div>
                <ByKind assets={a.assets}/>
            </div>

            <div className="card">
                <h3>API calls <span className="count">({apis.length})</span></h3>
                {apis.length === 0
                    ? <p className="muted">No fetch or XHR calls during page load.</p>
                    : <Waterfall assets={apis} page={page} slowMs={1000}/>}
            </div>

            <details className="card">
                <summary><h3>All requests <span className="count">({a.assets.length})</span></h3></summary>
                <Waterfall assets={a.assets} page={page} showKind/>
            </details>
        </>
    );
}

function ByKind({assets}: { assets: webperf.Asset[] }) {
    const rows = KINDS.map(k => {
        const xs = assets.filter(x => x.kind === k);
        return {k, n: xs.length, sent: xs.reduce((s, x) => s + sent(x), 0), unpacked: xs.reduce((s, x) => s + x.decodedBytes, 0)};
    }).filter(r => r.n > 0);
    const top = Math.max(1, ...rows.map(r => r.sent));
    return (
        <div className="card">
            <h3>Where the bytes go</h3>
            <p className="muted">First visit. Cross-origin files that hide their size count as 0.</p>
            <div className="table-scroll"><table>
                <thead><tr><th>Type</th><th className="num">Requests</th><th className="num">Downloaded</th><th className="num">Unpacked</th><th/></tr></thead>
                <tbody>
                {rows.map(r => (
                    <tr key={r.k}>
                        <td>{KIND_LABEL[r.k]}</td><td className="num">{r.n}</td>
                        <td className="num">{bytes(r.sent)}</td><td className="num">{bytes(r.unpacked)}</td>
                        <td className="meter-cell"><span className="meter" style={{width: `${r.sent / top * 100}%`}}/></td>
                    </tr>
                ))}
                </tbody>
            </table></div>
        </div>
    );
}

/** A request table with a timeline bar per row: light part = waiting for the server, dark part = downloading. */
function Waterfall({assets, page, showKind, slowMs}: { assets: webperf.Asset[]; page: string; showKind?: boolean; slowMs?: number }) {
    const end = Math.max(1, ...assets.map(x => x.startMs + x.durationMs));
    return (
        <div className="table-scroll">
            <table className="waterfall">
                <thead>
                <tr>
                    <th>URL</th>{showKind && <th>Type</th>}<th className="num">Status</th><th className="num">Start</th>
                    <th className="num">Duration</th><th className="num">Size</th><th>Repeat visit</th><th className="wf-col">Timeline (0 – {ms(end)})</th>
                </tr>
                </thead>
                <tbody>
                {assets.map((x, i) => (
                    <tr key={i}>
                        <td className="wf-url" title={x.url}>{shortUrl(x.url, page)}{x.renderBlocking && <span className="badge">blocking</span>}</td>
                        {showKind && <td>{KIND_LABEL[x.kind]}</td>}
                        <td className={'num' + (x.status >= 400 ? ' bad' : '')}>{x.status || '–'}</td>
                        <td className="num">{ms(x.startMs)}</td>
                        <td className={'num' + (slowMs && x.durationMs > slowMs ? ' bad' : '')}>{ms(x.durationMs)}</td>
                        <td className="num">{sent(x) ? bytes(sent(x)) : '–'}</td>
                        <td className={x.repeat === 'downloaded' || x.repeat === 'revalidated' ? 'warn-text' : 'muted'}>{x.repeat || '–'}</td>
                        <td className="wf-col">
                            <div className="wf-track" title={`starts ${ms(x.startMs)}, waits ${ms(x.waitMs)}, total ${ms(x.durationMs)}`}>
                                <span className="wf-bar" style={{left: `${x.startMs / end * 100}%`, width: `max(2px, ${x.durationMs / end * 100}%)`}}>
                                    <span className="wf-wait" style={{width: `${x.durationMs ? Math.min(100, x.waitMs / x.durationMs * 100) : 0}%`}}/>
                                </span>
                            </div>
                        </td>
                    </tr>
                ))}
                </tbody>
            </table>
        </div>
    );
}
