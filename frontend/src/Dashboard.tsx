import {useEffect, useState} from 'react';
import {ListSuites} from '../wailsjs/go/gui/App';
import {store} from '../wailsjs/go/models';
import {timeAgo} from './form';

const SLOTS = 30;

interface Props {
    runningId: number; // suite currently running, 0 if none
    version: number;   // bumps when a run finishes, to reload
    onCreate: () => void;
    onOpen: (id: number, tab?: 'config' | 'history' | 'trends') => void;
    onRun: (id: number) => void;
}

export default function Dashboard({runningId, version, onCreate, onOpen, onRun}: Props) {
    const [suites, setSuites] = useState<store.SuiteSummary[] | null>(null);
    const [error, setError] = useState('');
    const [filter, setFilter] = useState('');
    const [sort, setSort] = useState<'lastRun' | 'name'>('lastRun');

    useEffect(() => {
        ListSuites().then(setSuites).catch(e => setError(String(e)));
    }, [version]);

    const lastRunAt = (s: store.SuiteSummary) => s.recent?.[0]?.startedAt ?? '';
    const shown = (suites ?? [])
        .filter(s => s.name.toLowerCase().includes(filter.trim().toLowerCase()))
        .sort((a, b) => sort === 'name' ? a.name.localeCompare(b.name) : lastRunAt(b).localeCompare(lastRunAt(a)));

    return (
        <section>
            <div className="page-head">
                <h2>Performance suites {suites && <span className="count">({suites.length})</span>}</h2>
            </div>
            <div className="toolbar">
                <label className="inline">Sort by
                    <select value={sort} onChange={e => setSort(e.target.value as 'lastRun' | 'name')}>
                        <option value="lastRun">Last run</option>
                        <option value="name">Name</option>
                    </select>
                </label>
                <span className="spacer"/>
                <label className="inline">Filter
                    <input type="search" placeholder="ex: Checkout API" value={filter} onChange={e => setFilter(e.target.value)}/>
                </label>
            </div>

            {error && <div className="error" role="alert">{error}</div>}

            {suites?.length === 0 && (
                <div className="card empty">
                    <h3>No performance suites yet</h3>
                    <p className="muted">A suite is a saved load test you can re-run to track performance over time.</p>
                    <button className="primary" onClick={onCreate}>Create your first suite</button>
                </div>
            )}
            {suites && suites.length > 0 && shown.length === 0 && <p className="muted">No suites match “{filter}”.</p>}

            <div className="suite-grid">
                {shown.map(s => <SuiteCard key={s.id} suite={s} running={runningId === s.id} busy={runningId !== 0}
                                           onOpen={onOpen} onRun={onRun}/>)}
            </div>
        </section>
    );
}

function SuiteCard({suite: s, running, busy, onOpen, onRun}: {
    suite: store.SuiteSummary; running: boolean; busy: boolean;
    onOpen: Props['onOpen']; onRun: Props['onRun'];
}) {
    const runs = s.recent ?? []; // newest first
    const last = runs[0], prev = runs[1];
    const oldest = runs.at(-1);
    const strip = [...runs].reverse(); // oldest → newest, right-aligned
    const successRate = last && last.total > 0 ? (last.total - last.failed) / last.total * 100 : null;
    const delta = last && prev ? last.avgMs - prev.avgMs : null;

    return (
        <article className={`suite-card${last && !last.ok ? ' failed' : ''}`}>
            <div className="suite-card-body">
                <button className="link suite-name" onClick={() => onOpen(s.id)}>{s.name}</button>
                <p className="suite-target" title={s.url}><span className="method-tag">{s.method}</span> {s.url}</p>

                <div className="strip-labels">
                    <span>{oldest ? <>Oldest <span className="muted">({timeAgo(oldest.startedAt)})</span></> : 'No runs'}</span>
                    <span>Most recent</span>
                </div>
                <div className="strip" role="img"
                     aria-label={`Last ${runs.length} runs: ${runs.filter(r => r.ok).length} passed, ${runs.filter(r => !r.ok).length} failed`}>
                    {Array.from({length: SLOTS - strip.length}, (_, i) => <i key={`e${i}`}/>)}
                    {strip.map(r => <i key={r.id} className={r.ok ? 'ok' : 'bad'}
                                       title={`${r.ok ? 'Passed' : 'Failed'} – ${new Date(r.startedAt).toLocaleString()}`}/>)}
                </div>

                <p className="suite-status">
                    {running ? <span className="running">● Running…</span>
                        : !last ? <span className="muted">This suite has not been run.</span>
                            : last.ok ? <><span className="good">✓ Passed</span> — {timeAgo(last.startedAt)}</>
                                : <><span className="bad">✗ Failed</span> — {timeAgo(last.startedAt)}</>}
                </p>
            </div>
            <div className="suite-card-foot">
                <div className="suite-actions">
                    <button className="run" disabled={busy} onClick={() => onRun(s.id)}
                            title={busy && !running ? 'Another test is running' : undefined}>▶ Run</button>
                    <button className="link" onClick={() => onOpen(s.id, 'config')}>✎ Edit</button>
                </div>
                <div className="metric">
                    <strong>{successRate === null ? '--' : `${successRate.toFixed(successRate === 100 ? 0 : 2)}%`}</strong>
                    <span>Success rate</span>
                </div>
                <div className="metric">
                    <strong>{last ? `${last.avgMs.toFixed(1)}ms` : '--'}</strong>
                    <span>Avg. response time</span>
                    <em>{delta === null ? '--' : `${delta > 0 ? '+' : ''}${delta.toFixed(0)}ms`}</em>
                </div>
            </div>
        </article>
    );
}
