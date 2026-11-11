import {Form, PRESETS, fmtDuration} from './form';
import {StagesEditor, ThresholdsEditor} from './Editors';

const METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS'];

export interface FieldsProps {
    form: Form;
    set: <K extends keyof Form>(k: K, v: Form[K]) => void;
}

export function DetailsFields({form, set}: FieldsProps) {
    return (
        <div className="fields">
            <label className="wide">Suite name
                <input required autoFocus value={form.name} placeholder="e.g. Checkout API – load"
                       onChange={e => set('name', e.target.value)}/>
            </label>
            <label className="wide">Description
                <textarea rows={2} value={form.description} placeholder="Optional: what this suite checks and why"
                          onChange={e => set('description', e.target.value)}/>
            </label>
        </div>
    );
}

export function RequestFields({form, set}: FieldsProps) {
    return (
        <div className="fields">
            <div className="row wide">
                <label className="method">Method
                    <select value={form.method} onChange={e => set('method', e.target.value)}>
                        {METHODS.map(m => <option key={m}>{m}</option>)}
                    </select>
                </label>
                <label className="grow">URL
                    <input type="url" required pattern="https?://.+" title="An http:// or https:// URL"
                           value={form.url} onChange={e => set('url', e.target.value)}/>
                </label>
            </div>
            <label className="half">Headers (one per line, Name: value)
                <textarea rows={4} value={form.headers} placeholder="Authorization: Bearer …"
                          onChange={e => set('headers', e.target.value)}/>
            </label>
            <label className="half">Body
                <textarea rows={4} value={form.body} onChange={e => set('body', e.target.value)}/>
            </label>
            <label>Timeout (ms)
                <input type="number" min={1} required value={form.timeoutMs}
                       onChange={e => set('timeoutMs', e.target.valueAsNumber || 0)}/>
            </label>
        </div>
    );
}

export function WorkloadFields({form, set}: FieldsProps) {
    return (
        <div className="fields">
            <div className="wide presets">
                <div className="segmented" role="radiogroup" aria-label="Load profile">
                    <button type="button" role="radio" aria-checked={form.mode === 'constant'}
                            className={form.mode === 'constant' ? 'active' : ''} onClick={() => set('mode', 'constant')}>Constant</button>
                    <button type="button" role="radio" aria-checked={form.mode === 'stages'}
                            className={form.mode === 'stages' ? 'active' : ''} onClick={() => set('mode', 'stages')}>Stages</button>
                </div>
                <span className="muted">Presets:</span>
                {Object.entries(PRESETS).map(([name, p]) => (
                    <button type="button" key={name} title={p.hint}
                            onClick={() => { set('mode', 'stages'); set('stages', p.stages); }}>{name}</button>
                ))}
            </div>
            {form.mode === 'constant' ? <>
                <label>Virtual users
                    <input type="number" min={1} max={10000} required value={form.vus}
                           onChange={e => set('vus', e.target.valueAsNumber || 0)}/>
                </label>
                <label>Duration (s, 0 = no limit)
                    <input type="number" min={0} required value={form.durationSec}
                           onChange={e => set('durationSec', e.target.valueAsNumber || 0)}/>
                </label>
            </> : <div className="wide"><StagesEditor stages={form.stages} onChange={s => set('stages', s)}/></div>}
            <label>Max requests (0 = none)
                <input type="number" min={0} required value={form.requests}
                       onChange={e => set('requests', e.target.valueAsNumber || 0)}/>
            </label>
            <div className="wide">
                <p className="field-group">Think time
                    <span className="muted"> – each virtual user pauses a random time in this range between its requests,
                        like a real person reading a page. 0 = send back to back.</span></p>
                <div className="row">
                    <label>Min (ms)
                        <input type="number" min={0} required value={form.thinkMinMs}
                               onChange={e => set('thinkMinMs', e.target.valueAsNumber || 0)}/>
                    </label>
                    <label>Max (ms)
                        <input type="number" min={form.thinkMinMs} required value={form.thinkMaxMs}
                               onChange={e => set('thinkMaxMs', e.target.valueAsNumber || 0)}/>
                    </label>
                </div>
            </div>
        </div>
    );
}

export function ThresholdFields({form, set}: FieldsProps) {
    return <ThresholdsEditor items={form.thresholds} onChange={t => set('thresholds', t)}/>;
}

/** One-line description of the workload, e.g. "20 VUs for 1m" or "3 stages, peak 20 VUs, 1m". */
export function workloadSummary(f: Form): string {
    const limit = f.requests > 0 ? `, max ${f.requests} requests` : '';
    if (f.mode === 'stages') {
        const total = f.stages.reduce((n, s) => n + s.durationSec, 0);
        return `${f.stages.length} stage${f.stages.length > 1 ? 's' : ''}, peak ${Math.max(...f.stages.map(s => s.vus))} VUs, ${fmtDuration(total)}${limit}`;
    }
    return `${f.vus} virtual user${f.vus > 1 ? 's' : ''}${f.durationSec > 0 ? ` for ${fmtDuration(f.durationSec)}` : ''}${limit}`;
}
