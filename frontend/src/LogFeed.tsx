import {useEffect, useLayoutEffect, useRef, useState} from 'react';
import {LogEntry} from './events';

type Filter = 'all' | 'request' | 'error' | 'system';

const FILTERS: [Filter, string][] = [['all', 'All'], ['request', 'Requests'], ['error', 'Errors'], ['system', 'Engine']];
const ICON: Record<string, string> = {info: 'ℹ', ok: '✓', warn: '!', error: '✗'};

const matches = (e: LogEntry, f: Filter) =>
    f === 'all' || (f === 'error' ? e.level === 'error' || e.level === 'warn' : e.kind === f);

/** mm:ss.cc since the test started */
function clock(sec: number) {
    const m = Math.floor(sec / 60), s = sec - m * 60;
    return `${String(m).padStart(2, '0')}:${s.toFixed(2).padStart(5, '0')}`;
}

/** Live run log. Follows the newest line until the user scrolls up, then waits for "Jump to latest". */
export default function LogFeed({logs, running}: { logs: LogEntry[]; running: boolean }) {
    const [filter, setFilter] = useState<Filter>('all');
    const [follow, setFollow] = useState(true);
    const box = useRef<HTMLDivElement>(null);
    const shown = logs.filter(e => matches(e, filter));
    const errors = logs.filter(e => e.level === 'error').length;

    useLayoutEffect(() => {
        if (follow && box.current) box.current.scrollTop = box.current.scrollHeight;
    }, [shown.length, follow, filter]);
    useEffect(() => { if (running) setFollow(true); }, [running]);

    function onScroll() {
        const el = box.current!;
        setFollow(el.scrollHeight - el.scrollTop - el.clientHeight < 24);
    }

    return (
        <div className="card logs">
            <div className="card-head">
                <h3>
                    {running ? 'Live logs' : 'Run log'}
                    {running && <span className="pulse" aria-hidden="true"/>}
                    <span className="count">{logs.length} lines{errors > 0 && <> · <span className="bad">{errors} errors</span></>}</span>
                </h3>
                <div className="segmented" role="radiogroup" aria-label="Filter log lines">
                    {FILTERS.map(([f, label]) => (
                        <button key={f} type="button" role="radio" aria-checked={filter === f}
                                className={filter === f ? 'active' : ''} onClick={() => setFilter(f)}>{label}</button>
                    ))}
                </div>
            </div>

            <div className="log-wrap">
                <div className="log-box" ref={box} onScroll={onScroll} role="log" aria-live="off" tabIndex={0}
                     aria-label="Test run log">
                    {shown.length === 0 && (
                        <p className="log-empty">{logs.length === 0 ? 'Waiting for the first log lines…' : 'No lines match this filter.'}</p>
                    )}
                    {shown.map((e, i) => (
                        <div key={i} className={`log-line lv-${e.level}`}>
                            <span className="log-time">{clock(e.elapsedSec)}</span>
                            <span className={`log-level lv-${e.level}`}>{ICON[e.level] ?? '·'} {e.level.toUpperCase()}</span>
                            <span className={`log-who${e.vu ? '' : ' engine'}`}>{e.vu ? `VU ${e.vu}` : 'engine'}</span>
                            <span className="log-msg">{e.message}</span>
                            {e.kind === 'request' && e.status > 0 &&
                                <span className={`log-status ${e.status >= 400 ? 'bad' : 'good'}`}>{e.status}</span>}
                            {e.kind === 'request' && <span className="log-ms">{e.latencyMs.toFixed(1)} ms</span>}
                        </div>
                    ))}
                </div>
                {!follow && <button className="log-jump primary" onClick={() => setFollow(true)}>↓ Jump to latest</button>}
            </div>
            <p className="muted log-note">
                Engine events are all shown. Requests are sampled (a few per half second) so fast tests stay readable;
                failed requests are sampled separately so they are never crowded out.
            </p>
        </div>
    );
}
