import {useEffect, useState} from 'react';
import {RunSuite, RunningSuite, StopTest} from '../wailsjs/go/gui/App';
import {EventsOn} from '../wailsjs/runtime/runtime';
import {DoneEvent, EventDone, EventLog, EventProgress, LogEntry, Snapshot} from './events';
import Dashboard from './Dashboard';
import CreateSuite from './CreateSuite';
import SuitePage, {Tab} from './SuitePage';

/** The one test that is running (or last ran) in this session. Only one test runs at a time. */
export interface RunState {
    suiteId: number;
    running: boolean;
    samples: Snapshot[]; // shortcut: keeps every 500ms sample (~7k/hour); downsample if multi-hour runs become common
    logs: LogEntry[]; // newest last, capped at MAX_LOGS
    done: DoneEvent | null;
    error: string;
}

const MAX_LOGS = 1000;
const newRun = (suiteId: number): RunState => ({suiteId, running: true, samples: [], logs: [], done: null, error: ''});

type View = { page: 'dashboard' } | { page: 'create' } | { page: 'suite'; id: number; tab: Tab };

export default function App() {
    const [view, setView] = useState<View>({page: 'dashboard'});
    const [run, setRun] = useState<RunState | null>(null);
    const [version, setVersion] = useState(0); // bump to make lists reload
    const [error, setError] = useState('');

    useEffect(() => {
        const offProgress = EventsOn(EventProgress, (s: Snapshot) =>
            setRun(r => r && {...r, samples: [...r.samples, s]}));
        const offLog = EventsOn(EventLog, (e: LogEntry) =>
            setRun(r => r && {...r, logs: [...r.logs, e].slice(-MAX_LOGS)}));
        const offDone = EventsOn(EventDone, (e: DoneEvent) => {
            setRun(r => ({suiteId: e.suiteId, samples: r?.samples ?? [], logs: r?.logs ?? [], running: false, done: e, error: e.error}));
            setVersion(v => v + 1);
        });
        // a test may already be running if the window was reloaded
        RunningSuite().then(id => { if (id) setRun(r => r ?? newRun(id)); })
            .catch(() => {});
        return () => { offProgress(); offLog(); offDone(); };
    }, []);

    async function startRun(id: number) {
        setError('');
        try {
            setRun(newRun(id)); // before starting, so the first log lines are not lost
            await RunSuite(id);
            setView({page: 'suite', id, tab: 'live'});
        } catch (err) {
            setRun(null);
            setError(String(err));
        }
    }

    const runningId = run?.running ? run.suiteId : 0;
    return (
        <div className="app">
            <header className="topbar">
                <button className="brand" onClick={() => setView({page: 'dashboard'})}>VoltLoad</button>
                <nav>
                    <button className={view.page !== 'create' ? 'active' : ''} onClick={() => setView({page: 'dashboard'})}>Performance</button>
                </nav>
                <span className="spacer"/>
                {runningId !== 0 && (
                    <button className="running-chip" onClick={() => setView({page: 'suite', id: runningId, tab: 'live'})}>
                        <span className="pulse"/> Test running – view
                    </button>
                )}
                <button className="primary" onClick={() => setView({page: 'create'})}>+ Create suite</button>
            </header>


            <main>
                {error && <div className="error" role="alert">{error}</div>}
                {view.page === 'dashboard' && (
                    <Dashboard runningId={runningId} version={version} onRun={startRun}
                               onCreate={() => setView({page: 'create'})}
                               onOpen={(id, tab = 'history') => setView({page: 'suite', id, tab})}/>
                )}
                {view.page === 'create' && (
                    <CreateSuite onCancel={() => setView({page: 'dashboard'})}
                                 onCreated={id => { setVersion(v => v + 1); setView({page: 'suite', id, tab: 'config'}); }}/>
                )}
                {view.page === 'suite' && (
                    <SuitePage key={view.id} id={view.id} tab={view.tab} run={run} version={version}
                               setTab={tab => setView({...view, tab})}
                               onRun={startRun} onStop={() => StopTest()}
                               onBack={() => setView({page: 'dashboard'})}
                               onChanged={() => setVersion(v => v + 1)}/>
                )}
            </main>
        </div>
    );
}
