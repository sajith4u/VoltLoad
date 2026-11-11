import {FormEvent, useEffect, useState} from 'react';
import {DeleteRun, DeleteSuite, GetRun, GetSuite, ListRuns, SaveScenario, SaveSuite} from '../wailsjs/go/gui/App';
import {loader, store} from '../wailsjs/go/models';
import {Form, fromSuite, timeAgo, toConfig, toSuite} from './form';
import {DetailsFields, RequestFields, ThresholdFields, WorkloadFields, workloadSummary} from './SuiteFields';
import {RunState} from './App';
import Chart, {Point} from './Chart';
import ResultView, {Stat, Verdict} from './ResultView';
import CompareView from './CompareView';
import LogFeed from './LogFeed';

export type Tab = 'config' | 'history' | 'trends' | 'live';

interface Props {
    id: number;
    tab: Tab;
    setTab: (t: Tab) => void;
    run: RunState | null; // the app-wide run, which may belong to another suite
    version: number;
    onRun: (id: number) => void;
    onStop: () => void;
    onBack: () => void;
    onChanged: () => void;
}

export default function SuitePage({id, tab, setTab, run, version, onRun, onStop, onBack, onChanged}: Props) {
    const [form, setForm] = useState<Form | null>(null);
    const [saved, setSaved] = useState<Form | null>(null);
    const [updatedAt, setUpdatedAt] = useState('');
    const [runs, setRuns] = useState<store.RunSummary[]>([]);
    const [error, setError] = useState('');

    const mine = run?.suiteId === id ? run : null;
    const running = !!mine?.running;
    const busy = !!run?.running;

    const loadSuite = () => GetSuite(id).then(s => {
        const f = fromSuite(s);
        setForm(f); setSaved(f); setUpdatedAt(s.updatedAt);
    }).catch(e => setError(String(e)));
    const loadRuns = () => ListRuns(id).then(setRuns).catch(e => setError(String(e)));

    useEffect(() => { loadSuite(); }, [id]);
    useEffect(() => { loadRuns(); }, [id, version]);

    if (!form || !saved) {
        return <section>{error ? <div className="error" role="alert">{error}</div> : <p className="muted">Loading…</p>}</section>;
    }
    const last = runs[0];
    const dirty = JSON.stringify(form) !== JSON.stringify(saved);
    const tabs: [Tab, string][] = [['config', 'Configuration'], ['history', 'History'], ['trends', 'Trend Charts']];
    if (mine) tabs.push(['live', running ? 'Live run' : 'Last run']);
    if (tab === 'live' && !mine) tab = 'history'; // the live tab only exists for this session's run

    return (
        <div className="suite-layout">
            <aside className="suite-side">
                <button className="link" onClick={onBack}>← All suites</button>
                <h2>{saved.name}</h2>
                {saved.description && <p className="muted">{saved.description}</p>}
                <p className="suite-target" title={saved.url}><span className="method-tag">{saved.method}</span> {saved.url}</p>
                <p className="muted">Updated {timeAgo(updatedAt)}</p>

                <div className="side-actions">
                    {running
                        ? <button className="danger-solid" onClick={onStop}>■ Stop test</button>
                        : <button className="run" disabled={busy || dirty} onClick={() => onRun(id)}
                                  title={dirty ? 'Save your changes first' : busy ? 'Another test is running' : undefined}>▶ Run test</button>}
                </div>
                {dirty && !running && <p className="muted">Save your changes to run.</p>}

                <dl className="side-facts">
                    <dt>Workload</dt><dd>{workloadSummary(saved)}</dd>
                    <dt>Thresholds</dt><dd>{saved.thresholds.length || 'None'}</dd>
                    <dt>Runs</dt><dd>{runs.length}</dd>
                    <dt>Last result</dt>
                    <dd>{!last ? 'Not run yet' : <><Verdict passed={last.ok}/> {timeAgo(last.startedAt)}</>}</dd>
                </dl>
            </aside>

            <section className="suite-main">
                <div className="tabs" role="tablist">
                    {tabs.map(([t, label]) => (
                        <button key={t} role="tab" aria-selected={tab === t} className={tab === t ? 'active' : ''}
                                onClick={() => setTab(t)}>{label}{t === 'live' && running && <span className="pulse"/>}</button>
                    ))}
                </div>

                {error && <div className="error" role="alert">{error}</div>}

                {tab === 'config' && (
                    <ConfigTab form={form} setForm={setForm} dirty={dirty} locked={running}
                               onSaved={() => { loadSuite(); onChanged(); }}
                               onDeleted={() => { onChanged(); onBack(); }}/>
                )}
                {tab === 'history' && <HistoryTab runs={runs} reload={() => { loadRuns(); onChanged(); }}/>}
                {tab === 'trends' && <TrendTab runs={runs}/>}
                {tab === 'live' && mine && <LiveTab run={mine} plannedSec={plannedSec(saved)} maxRequests={saved.requests}/>}
            </section>
        </div>
    );
}

const plannedSec = (f: Form) => f.mode === 'stages' ? f.stages.reduce((n, s) => n + s.durationSec, 0) : f.durationSec;

function ConfigTab({form, setForm, dirty, locked, onSaved, onDeleted}: {
    form: Form; setForm: (f: Form) => void; dirty: boolean; locked: boolean; onSaved: () => void; onDeleted: () => void;
}) {
    const [error, setError] = useState('');
    const [note, setNote] = useState('');
    const set = <K extends keyof Form>(k: K, v: Form[K]) => setForm({...form, [k]: v});
    const fields = {form, set};

    async function save(e: FormEvent) {
        e.preventDefault();
        setError(''); setNote('');
        try {
            await SaveSuite(toSuite(form));
            setNote('Saved');
            onSaved();
        } catch (err) {
            setError(String(err));
        }
    }

    async function exportYaml() {
        setError(''); setNote('');
        try {
            const path = await SaveScenario(toConfig(form));
            if (path) setNote(`Exported to ${path}`);
        } catch (err) {
            setError(String(err));
        }
    }

    async function remove() {
        if (!confirm(`Delete “${form.name}” and all of its run history? This cannot be undone.`)) return;
        try {
            await DeleteSuite(form.id);
            onDeleted();
        } catch (err) {
            setError(String(err));
        }
    }

    return (
        <form onSubmit={save}>
            <fieldset className="card" disabled={locked}><legend>Details</legend><DetailsFields {...fields}/></fieldset>
            <fieldset className="card" disabled={locked}><legend>Request</legend><RequestFields {...fields}/></fieldset>
            <fieldset className="card" disabled={locked}><legend>Workload</legend><WorkloadFields {...fields}/></fieldset>
            <fieldset className="card" disabled={locked}><legend>Thresholds</legend><ThresholdFields {...fields}/></fieldset>
            {error && <div className="error" role="alert">{error}</div>}
            <div className="form-foot">
                <button type="button" className="danger" disabled={locked} onClick={remove}>Delete suite</button>
                <span className="spacer"/>
                {note && <span className="muted">{note}</span>}
                <button type="button" onClick={exportYaml}>Export YAML…</button>
                <button type="submit" className="primary" disabled={!dirty || locked}>Save changes</button>
            </div>
        </form>
    );
}

function HistoryTab({runs, reload}: { runs: store.RunSummary[]; reload: () => void }) {
    const [selected, setSelected] = useState<{ id: number; result: loader.Result } | null>(null);
    const [picked, setPicked] = useState<number[]>([]); // up to 2 run ids
    const [compare, setCompare] = useState<[loader.Result, loader.Result] | null>(null);
    const [error, setError] = useState('');

    const guard = (fn: () => Promise<void>) => fn().catch(e => setError(String(e)));
    const open = (id: number) => guard(async () => { setCompare(null); setSelected({id, result: await GetRun(id)}); });
    const remove = (id: number) => guard(async () => {
        if (!confirm('Delete this run from history?')) return;
        await DeleteRun(id);
        if (selected?.id === id) setSelected(null);
        setPicked(p => p.filter(x => x !== id));
        reload();
    });
    const runCompare = () => guard(async () => {
        const [older, newer] = [...picked].sort((a, b) => a - b); // older run is the baseline
        setSelected(null);
        setCompare([await GetRun(older), await GetRun(newer)]);
    });
    const togglePick = (id: number) => setPicked(p => p.includes(id) ? p.filter(x => x !== id) : [...p, id].slice(-2));

    if (runs.length === 0) return <div className="card empty"><p className="muted">No runs yet. Press “Run test” to start one.</p></div>;
    return <>
        {error && <div className="error" role="alert">{error}</div>}
        <div className="card">
            <div className="card-head">
                <h3>Runs <span className="count">({runs.length})</span></h3>
                <div className="buttons">
                    <span className="muted">{picked.length}/2 selected</span>
                    <button onClick={runCompare} disabled={picked.length !== 2}>Compare</button>
                </div>
            </div>
            <table className="history">
                <thead>
                <tr><th/><th>Started</th><th>Result</th><th>Requests</th><th>Failed</th><th>Req/s</th><th>Avg ms</th><th>p95 ms</th><th/></tr>
                </thead>
                <tbody>
                {runs.map(r => (
                    <tr key={r.id} className={selected?.id === r.id ? 'selected' : ''}>
                        <td><input type="checkbox" checked={picked.includes(r.id)} onChange={() => togglePick(r.id)}
                                   aria-label={`Select run from ${new Date(r.startedAt).toLocaleString()} for comparison`}/></td>
                        <td><button className="link" onClick={() => open(r.id)}>{new Date(r.startedAt).toLocaleString()}</button></td>
                        <td><Verdict passed={r.ok}/></td>
                        <td>{r.total}</td>
                        <td className={r.failed > 0 ? 'bad' : ''}>{r.failed}</td>
                        <td>{r.rps.toFixed(1)}</td>
                        <td>{r.avgMs.toFixed(1)}</td>
                        <td>{r.p95Ms.toFixed(1)}</td>
                        <td><button className="link danger" onClick={() => remove(r.id)}>Delete</button></td>
                    </tr>
                ))}
                </tbody>
            </table>
        </div>
        {compare && <CompareView a={compare[0]} b={compare[1]} onClose={() => setCompare(null)}/>}
        {selected && <ResultView result={selected.result} runId={selected.id} showTimeline/>}
    </>;
}

// One fixed color per metric (validated categorical palette), so a metric looks the same everywhere.
const TRENDS: { title: string; unit: string; color: string; get: (r: store.RunSummary) => number }[] = [
    {title: 'Average response time', unit: 'ms', color: '#2a78d6', get: r => r.avgMs},
    {title: 'Average hits', unit: 'req/s', color: '#eb6834', get: r => r.rps},
    {title: 'p95 latency', unit: 'ms', color: '#1baf7a', get: r => r.p95Ms},
    {title: 'Error rate', unit: '%', color: '#eda100', get: r => r.total ? r.failed / r.total * 100 : 0},
    {title: 'Total requests', unit: '', color: '#e87ba4', get: r => r.total},
    {title: 'Test duration', unit: 's', color: '#008300', get: r => r.durationSec},
];

function TrendTab({runs}: { runs: store.RunSummary[] }) {
    const [recent, setRecent] = useState(25);
    const shown = runs.slice(0, recent).reverse(); // oldest → newest
    if (runs.length === 0) return <div className="card empty"><p className="muted">Trends appear after the first run.</p></div>;
    return (
        <div className="card">
            <div className="card-head">
                <h3>Trend Charts</h3>
                <label className="inline">Show recent
                    <select value={recent} onChange={e => setRecent(Number(e.target.value))}>
                        {[10, 25, 50, 100].map(n => <option key={n}>{n}</option>)}
                    </select>
                </label>
            </div>
            <p className="muted">Each point is one run, oldest on the left. Hover a chart to read a run’s value.</p>
            <div className="charts">
                {TRENDS.map(t => (
                    <Chart key={t.title} title={t.title} unit={t.unit} color={t.color} area dots={shown.length <= 30}
                           xEnd="latest run"
                           points={shown.map((r, i): Point => ({x: i, y: t.get(r), label: new Date(r.startedAt).toLocaleString()}))}/>
                ))}
            </div>
        </div>
    );
}

function LiveTab({run, plannedSec, maxRequests}: { run: RunState; plannedSec: number; maxRequests: number }) {
    const live = run.samples.at(-1);
    const progress = !live ? 0
        : plannedSec > 0 ? Math.min(100, live.elapsedSec / plannedSec * 100)
            : maxRequests > 0 ? Math.min(100, live.total / maxRequests * 100) : 0;
    const series = (get: (s: RunState['samples'][number]) => number): Point[] =>
        run.samples.map(s => ({x: s.elapsedSec, y: get(s), label: `${s.elapsedSec.toFixed(1)}s`}));
    const xEnd = live ? `${live.elapsedSec.toFixed(0)}s` : undefined;
    return <>
        {run.error && <div className="error" role="alert">{run.error}</div>}
        <div className="card">
            <h3>{run.running ? 'Live' : 'Timeline'}</h3>
            {run.running && <div className="progress" role="progressbar" aria-valuenow={Math.round(progress)}><div style={{width: `${progress}%`}}/></div>}
            <div className="stats">
                <Stat label="Elapsed" value={`${(live?.elapsedSec ?? 0).toFixed(1)} s`}/>
                <Stat label="Active VUs" value={live?.activeVUs ?? 0}/>
                <Stat label="Requests" value={live?.total ?? 0}/>
                <Stat label="Failed" value={live?.failed ?? 0} bad={(live?.failed ?? 0) > 0}/>
                <Stat label="Req/s" value={(live?.rps ?? 0).toFixed(1)}/>
                <Stat label="Avg latency" value={`${(live?.avgLatencyMs ?? 0).toFixed(1)} ms`}/>
            </div>
            <div className="charts">
                <Chart title="Requests/sec" unit="req/s" color="#eb6834" xEnd={xEnd} points={series(s => s.rps)}/>
                <Chart title="Avg latency" unit="ms" color="#2a78d6" xEnd={xEnd} points={series(s => s.avgLatencyMs)}/>
                <Chart title="Active VUs" unit="VUs" color="#6250d6" xEnd={xEnd} points={series(s => s.activeVUs)}/>
            </div>
        </div>
        <LogFeed logs={run.logs} running={run.running}/>
        {/* result is empty only if the engine rejected the config */}
        {run.done?.result.config?.url && <ResultView result={run.done.result} runId={run.done.runId}/>}
    </>;
}
