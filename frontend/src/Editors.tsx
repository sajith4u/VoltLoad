import {Stage, Threshold} from './form';

export function StagesEditor({stages, onChange}: { stages: Stage[]; onChange: (s: Stage[]) => void }) {
    const update = (i: number, patch: Partial<Stage>) => onChange(stages.map((s, j) => j === i ? {...s, ...patch} : s));
    const total = stages.reduce((n, s) => n + (s.durationSec || 0), 0);
    return (
        <div className="editor">
            <table>
                <thead>
                <tr><th>#</th><th>Duration (s)</th><th>Target VUs</th><th/></tr>
                </thead>
                <tbody>
                {stages.map((s, i) => (
                    <tr key={i}>
                        <td>{i + 1}</td>
                        <td><input type="number" min={1} required aria-label={`Stage ${i + 1} duration`} value={s.durationSec}
                                   onChange={e => update(i, {durationSec: e.target.valueAsNumber || 0})}/></td>
                        <td><input type="number" min={0} max={10000} required aria-label={`Stage ${i + 1} target VUs`} value={s.vus}
                                   onChange={e => update(i, {vus: e.target.valueAsNumber || 0})}/></td>
                        <td><button type="button" className="link danger" disabled={stages.length === 1}
                                    onClick={() => onChange(stages.filter((_, j) => j !== i))}>Remove</button></td>
                    </tr>
                ))}
                </tbody>
            </table>
            <div className="editor-foot">
                <button type="button" onClick={() => onChange([...stages, {durationSec: 30, vus: stages.at(-1)?.vus ?? 10}])}>
                    Add stage
                </button>
                <span className="muted">Each stage ramps linearly from the previous target. Total {total}s.</span>
            </div>
        </div>
    );
}

const METRICS: [string, string][] = [
    ['p95', 'p95 latency (ms)'], ['p99', 'p99 latency (ms)'], ['p90', 'p90 latency (ms)'], ['p50', 'p50 latency (ms)'],
    ['avg', 'avg latency (ms)'], ['max', 'max latency (ms)'], ['errorRate', 'error rate (%)'], ['rps', 'requests/sec'],
];

export function ThresholdsEditor({items, onChange}: { items: Threshold[]; onChange: (t: Threshold[]) => void }) {
    const update = (i: number, patch: Partial<Threshold>) => onChange(items.map((t, j) => j === i ? {...t, ...patch} : t));
    return (
        <div className="editor">
            {items.map((t, i) => (
                <div className="threshold" key={i}>
                    <select aria-label="Metric" value={t.metric} onChange={e => update(i, {metric: e.target.value})}>
                        {METRICS.map(([v, label]) => <option key={v} value={v}>{label}</option>)}
                    </select>
                    <select aria-label="Operator" value={t.op} onChange={e => update(i, {op: e.target.value})}>
                        {['<', '<=', '>', '>='].map(o => <option key={o}>{o}</option>)}
                    </select>
                    <input type="number" step="any" required aria-label="Limit" value={t.value}
                           onChange={e => update(i, {value: e.target.valueAsNumber || 0})}/>
                    <button type="button" className="link danger" onClick={() => onChange(items.filter((_, j) => j !== i))}>Remove</button>
                </div>
            ))}
            <div className="editor-foot">
                <button type="button" onClick={() => onChange([...items, {metric: 'p95', op: '<', value: 500}])}>Add threshold</button>
                <span className="muted">The run is marked FAIL if any rule is not met.</span>
            </div>
        </div>
    );
}
