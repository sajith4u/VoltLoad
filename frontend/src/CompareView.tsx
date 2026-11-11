import {loader} from '../wailsjs/go/models';

type Row = [label: string, get: (r: loader.Result) => number, lowerIsBetter: boolean, digits?: number];

const ROWS: Row[] = [
    ['Requests', r => r.total, false, 0],
    ['Req/s', r => r.rps, false],
    ['Error rate %', r => r.total ? r.failed / r.total * 100 : 0, true],
    ['avg ms', r => r.latencyMs.avg, true],
    ['p50 ms', r => r.latencyMs.p50, true],
    ['p90 ms', r => r.latencyMs.p90, true],
    ['p95 ms', r => r.latencyMs.p95, true],
    ['p99 ms', r => r.latencyMs.p99, true],
    ['max ms', r => r.latencyMs.max, true],
];

const title = (r: loader.Result) => `${r.config.name || r.config.url} · ${new Date(r.startedAt).toLocaleString()}`;

/** Side-by-side metrics for a baseline (a) and a candidate (b), with the change from a to b. */
export default function CompareView({a, b, onClose}: { a: loader.Result; b: loader.Result; onClose: () => void }) {
    return (
        <div className="card">
            <div className="card-head">
                <h3>Comparison</h3>
                <button onClick={onClose}>Close</button>
            </div>
            <table>
                <thead>
                <tr><th>Metric</th><th>Baseline<br/><span className="muted">{title(a)}</span></th>
                    <th>Candidate<br/><span className="muted">{title(b)}</span></th><th>Change</th></tr>
                </thead>
                <tbody>
                {ROWS.map(([label, get, lowerIsBetter, digits = 1]) => {
                    const va = get(a), vb = get(b);
                    const pct = va === 0 ? (vb === 0 ? 0 : Infinity) : (vb - va) / va * 100;
                    const better = lowerIsBetter ? vb < va : vb > va;
                    const same = Math.abs(pct) < 1;
                    return (
                        <tr key={label}>
                            <td>{label}</td>
                            <td>{va.toFixed(digits)}</td>
                            <td>{vb.toFixed(digits)}</td>
                            <td className={same ? 'muted' : better ? 'good' : 'bad'}>
                                {same ? 'no change' : `${pct > 0 ? '+' : ''}${Number.isFinite(pct) ? pct.toFixed(1) : '∞'}% ${better ? '(better)' : '(worse)'}`}
                            </td>
                        </tr>
                    );
                })}
                </tbody>
            </table>
        </div>
    );
}
