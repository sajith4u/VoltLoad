import {FormEvent, useState} from 'react';
import {LoadScenario, ParseScenario, SaveSuite} from '../wailsjs/go/gui/App';
import {isDesktop, pickTextFile} from './bridge';
import {Form, emptyForm, fromConfig, toSuite} from './form';
import {DetailsFields, RequestFields, ThresholdFields, WorkloadFields, workloadSummary} from './SuiteFields';

const STEPS = ['Details', 'Request', 'Workload', 'Thresholds', 'Review'];

/** Step-by-step flow for creating a performance suite. Each step validates before moving on. */
export default function CreateSuite({onCreated, onCancel}: { onCreated: (id: number) => void; onCancel: () => void }) {
    const [form, setForm] = useState<Form>(emptyForm);
    const [step, setStep] = useState(0);
    const [error, setError] = useState('');
    const [saving, setSaving] = useState(false);
    const set = <K extends keyof Form>(k: K, v: Form[K]) => setForm(f => ({...f, [k]: v}));
    const last = step === STEPS.length - 1;

    async function next(e: FormEvent) {
        e.preventDefault(); // the browser has already run the step's field validation
        setError('');
        if (step === 2 && form.mode === 'constant' && form.durationSec === 0 && form.requests === 0) {
            setError('Set a duration, a max request count, or both – otherwise the test would never end.');
            return;
        }
        if (!last) return setStep(step + 1);
        setSaving(true);
        try {
            onCreated(await SaveSuite(toSuite(form)));
        } catch (err) {
            setError(String(err));
            setSaving(false);
        }
    }

    async function importYaml() {
        setError('');
        try {
            let c;
            if (isDesktop) {
                c = await LoadScenario();
            } else {
                const text = await pickTextFile('.yaml,.yml');
                c = text === null ? null : await ParseScenario(text);
            }
            if (c) setForm(f => fromConfig(c, f));
        } catch (err) {
            setError(String(err));
        }
    }

    const fields = {form, set};
    return (
        <section className="narrow">
            <div className="page-head">
                <h2>Create performance suite</h2>
                <button type="button" onClick={importYaml}>Import from YAML…</button>
            </div>

            <ol className="stepper">
                {STEPS.map((name, i) => (
                    <li key={name} className={i === step ? 'current' : i < step ? 'done' : ''} aria-current={i === step ? 'step' : undefined}>
                        <button type="button" disabled={i > step} onClick={() => setStep(i)}>
                            <span className="step-num">{i < step ? '✓' : i + 1}</span>{name}
                        </button>
                    </li>
                ))}
            </ol>

            <form className="card" onSubmit={next}>
                <h3>{['Name your suite', 'What should be called?', 'How much load?', 'When does a run pass?', 'Review and create'][step]}</h3>
                <p className="muted step-hint">{[
                    'A suite is a saved test you can run again and again. Its run history and trends are tracked together.',
                    'The HTTP request every virtual user sends in a loop.',
                    'Constant load, or stages that ramp virtual users up and down. Presets fill in common shapes.',
                    'Optional pass/fail rules checked at the end of every run. You can add them later.',
                    'Check the details. You can change everything later from the suite’s Configuration tab.',
                ][step]}</p>

                {step === 0 && <DetailsFields {...fields}/>}
                {step === 1 && <RequestFields {...fields}/>}
                {step === 2 && <WorkloadFields {...fields}/>}
                {step === 3 && <ThresholdFields {...fields}/>}
                {last && (
                    <dl className="review">
                        <dt>Name</dt><dd>{form.name}</dd>
                        {form.description && <><dt>Description</dt><dd>{form.description}</dd></>}
                        <dt>Request</dt><dd><span className="method-tag">{form.method}</span> {form.url}</dd>
                        <dt>Workload</dt><dd>{workloadSummary(form)}</dd>
                        <dt>Thresholds</dt>
                        <dd>{form.thresholds.length
                            ? form.thresholds.map(t => `${t.metric} ${t.op} ${t.value}`).join(', ')
                            : 'None – runs fail only if requests fail'}</dd>
                    </dl>
                )}

                {error && <div className="error" role="alert">{error}</div>}

                <div className="wizard-foot">
                    <button type="button" onClick={onCancel}>Cancel</button>
                    <span className="spacer"/>
                    {step > 0 && <button type="button" onClick={() => { setError(''); setStep(step - 1); }}>Back</button>}
                    <button type="submit" className="primary" disabled={saving}>{last ? 'Create suite' : 'Next'}</button>
                </div>
            </form>
        </section>
    );
}
