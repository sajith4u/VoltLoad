import {useEffect, useState} from 'react';
import {RunSuite, RunningSuite, StopTest} from '../wailsjs/go/gui/App';
import {EventsOn} from '../wailsjs/runtime/runtime';
import {DoneEvent, EventDone, EventLog, EventProgress, LogEntry, Snapshot} from './events';
import Dashboard from './Dashboard';
import CreateSuite from './CreateSuite';
import SuitePage, {Tab} from './SuitePage';
import {Login, ChangePasswordDialog} from './Account';
import Icon from './Icons';
import FrontendPage from './FrontendPage';
import {Logout, Session} from '../wailsjs/go/gui/App';
import {UNAUTH, connectEvents} from './bridge';

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

type View = { page: 'dashboard' } | { page: 'create' } | { page: 'frontend' } | { page: 'suite'; id: number; tab: Tab };

/** Signed out → the login page; signed in → the app. */
export default function App() {
    const [user, setUser] = useState<string | null>(null); // null while checking for an existing session
    const [notice, setNotice] = useState('');

    useEffect(() => {
        const out = () => { connectEvents(false); setUser(''); };
        window.addEventListener(UNAUTH, out);
        Session().then(u => { if (u) connectEvents(true); setUser(u); })
            .catch(err => { setNotice(String(err instanceof Error ? err.message : err)); setUser(''); });
        return () => window.removeEventListener(UNAUTH, out);
    }, []);

    if (user === null) return null;
    if (!user) return <Login notice={notice} onLogin={u => { connectEvents(true); setNotice(''); setUser(u); }}/>;
    return <Shell user={user} onLogout={() => { Logout().catch(() => {}); connectEvents(false); setUser(''); }}/>;
}

function Shell({user, onLogout}: { user: string; onLogout: () => void }) {
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
    const [changing, setChanging] = useState(false);
    const section = view.page === 'suite' ? 'dashboard' : view.page; // a suite page belongs to Performance
    const nav = (page: 'dashboard' | 'create' | 'frontend', label: string, icon: 'grid' | 'plus' | 'gauge') => (
        <button className={'nav-item' + (section === page ? ' active' : '')} aria-current={section === page ? 'page' : undefined}
                onClick={() => setView({page})}>
            <Icon name={icon}/> {label}
        </button>
    );
    return (
        <div className="shell">
            <aside className="sidebar">
                <button className="brand" onClick={() => setView({page: 'dashboard'})}><span className="logo"><Icon name="bolt"/></span> VoltLoad</button>
                <nav aria-label="Main">
                    {nav('dashboard', 'Performance', 'grid')}
                    {nav('create', 'Create suite', 'plus')}
                    {nav('frontend', 'Frontend analysis', 'gauge')}
                    {runningId !== 0 && (
                        <button className="nav-item running" onClick={() => setView({page: 'suite', id: runningId, tab: 'live'})}>
                            <Icon name="activity"/> Test running <span className="pulse"/>
                        </button>
                    )}
                </nav>
                <span className="spacer"/>
                <div className="account">
                    <div className="who"><span className="avatar">{user[0]?.toUpperCase()}</span><b>{user}</b></div>
                    <button className="nav-item" onClick={() => setChanging(true)}><Icon name="key"/> Change password</button>
                    <button className="nav-item" onClick={onLogout}><Icon name="logout"/> Sign out</button>
                </div>
            </aside>
            {changing && <ChangePasswordDialog user={user} onClose={() => setChanging(false)}/>}

            <main>
                {error && <div className="error" role="alert">{error}</div>}
                {view.page === 'dashboard' && (
                    <Dashboard runningId={runningId} version={version} onRun={startRun}
                               onCreate={() => setView({page: 'create'})}
                               onOpen={(id, tab = 'history') => setView({page: 'suite', id, tab})}/>
                )}
                {view.page === 'frontend' && <FrontendPage/>}
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
