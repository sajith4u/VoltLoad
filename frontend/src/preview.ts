// Browser preview: when the page is opened outside the Wails desktop window (e.g. the Vite dev URL),
// the Go bridge (window.go / window.runtime) does not exist. Install an in-memory stand-in with sample
// data so the UI still renders and can be clicked through. No real requests are sent in this mode.
const w = window as any;

export const isPreview = !w.go?.gui?.App || !w.runtime;

if (isPreview) {
    const subs: Record<string, ((d: any) => void)[]> = {};
    const emit = (n: string, d: any) => (subs[n] ?? []).forEach(cb => cb(d));
    w.runtime = {
        EventsOnMultiple: (n: string, cb: (d: any) => void) => {
            (subs[n] ??= []).push(cb);
            return () => { subs[n] = subs[n].filter(x => x !== cb); };
        },
    };

    const cfg = (name: string, url: string, extra: object = {}) => ({
        name, method: 'GET', url, headers: {}, body: '', vus: 20, durationSec: 60, requests: 0, timeoutMs: 30000, thinkMinMs: 0, thinkMaxMs: 0,
        stages: [], thresholds: [{metric: 'p95', op: '<', value: 400}], ...extra,
    });
    const now = Date.now();
    let nextId = 1;
    const suites: any[] = [
        {id: nextId++, name: 'Checkout API – load', description: 'Ramp to 20 VUs against the checkout endpoint',
            config: cfg('Checkout API – load', 'https://shop.example.test/api/checkout', {method: 'POST', body: '{"cart":42}',
                stages: [{durationSec: 20, vus: 20}, {durationSec: 30, vus: 20}, {durationSec: 10, vus: 0}]})},
        {id: nextId++, name: 'Catalog search', description: 'Constant 20 VUs for one minute',
            config: cfg('Catalog search', 'https://shop.example.test/api/search?q=shoes')},
        {id: nextId++, name: 'Health check smoke', description: '', config: cfg('Health check smoke', 'https://shop.example.test/health', {vus: 1, durationSec: 30, thresholds: []})},
        {id: nextId++, name: 'Login burst', description: 'Not run yet', config: cfg('Login burst', 'https://shop.example.test/api/login', {method: 'POST'})},
    ].map(s => ({...s, updatedAt: new Date(now - 86400e3 * 3).toISOString()}));

    const runs: any[] = []; // {id, suiteId, result}
    const makeResult = (suite: any, at: number, avg: number, failed: number, stopped = false) => {
        const total = 12000, c = suite.config;
        const r: any = {
            config: c, startedAt: new Date(at).toISOString(), durationSec: 60, total, success: total - failed, failed,
            rps: total / 60 * (300 / (avg + 100)), stopped,
            latencyMs: {min: avg * 0.3, avg, p50: avg * 0.85, p90: avg * 1.5, p95: avg * 1.8, p99: avg * 2.6, max: avg * 5},
            statusCodes: failed ? {200: total - failed, 503: failed} : {200: total}, errors: {},
        };
        r.vus = Array.from({length: Math.min(c.vus || 20, 20)}, (_, i) => ({id: i + 1, requests: Math.round(total / 20) + i, failed: 0, avgMs: avg + i}));
        r.timeline = Array.from({length: 60}, (_, i) => ({elapsedSec: i + 1, total: (i + 1) * 200, success: (i + 1) * 200, failed: 0,
            rps: Math.min(1, (i + 1) / 20) * r.rps + Math.sin(i / 3) * 8, avgLatencyMs: avg + Math.cos(i / 4) * 4, activeVUs: Math.round(Math.min(1, (i + 1) / 20) * (c.vus || 20))}));
        r.checks = (c.thresholds ?? []).map((t: any) => ({threshold: t, actual: r.latencyMs.p95, passed: r.latencyMs.p95 < t.value}));
        r.passed = r.checks.every((x: any) => x.passed);
        return r;
    };
    // seeded history: suite 1 healthy with one bad day, suite 2 degrading, suite 3 a few runs
    const seed = (suite: any, avgs: number[], failAt: number[] = []) => avgs.forEach((avg, i) =>
        runs.push({id: nextId++, suiteId: suite.id, result: makeResult(suite, now - (avgs.length - i) * 3600e3 * 5, avg, failAt.includes(i) ? 180 : 0)}));
    seed(suites[0], [120, 118, 131, 125, 122, 260, 119, 117, 124, 121, 116, 123], [5]);
    seed(suites[1], [150, 155, 149, 162, 171, 185, 204, 221, 236, 248], [8, 9]);
    seed(suites[2], [22, 24, 21]);

    const summary = (x: any) => {
        const r = x.result, passed = r.checks.length ? r.passed : null;
        return {id: x.id, name: r.config.name, method: r.config.method, url: r.config.url, startedAt: r.startedAt,
            total: r.total, failed: r.failed, rps: r.rps, p95Ms: r.latencyMs.p95, avgMs: r.latencyMs.avg,
            durationSec: r.durationSec, passed, ok: passed ?? r.failed === 0};
    };
    const runsOf = (id: number) => runs.filter(x => x.suiteId === id).sort((a, b) => b.id - a.id).map(summary);
    const find = (id: number) => {
        const s = suites.find(x => x.id === id);
        if (!s) throw new Error(`suite ${id} not found`);
        return s;
    };

    let running = 0, timer = 0, stopNow = () => {};
    w.go = {gui: {App: {
        ListSuites: async () => suites.map(s => ({id: s.id, name: s.name, description: s.description, method: s.config.method,
            url: s.config.url, updatedAt: s.updatedAt, recent: runsOf(s.id).slice(0, 30)})),
        GetSuite: async (id: number) => structuredClone(find(id)),
        SaveSuite: async (s: any) => {
            if (!s.name?.trim()) throw new Error('suite name is required');
            if (!/^https?:\/\/.+/.test(s.config.url)) throw new Error('url must be an absolute http:// or https:// URL');
            s = {...structuredClone(s), updatedAt: new Date().toISOString()};
            s.config.name = s.name;
            if (!s.id) { s.id = nextId++; suites.unshift(s); } else suites[suites.findIndex(x => x.id === s.id)] = s;
            return s.id;
        },
        DeleteSuite: async (id: number) => { suites.splice(suites.findIndex(x => x.id === id), 1); },
        ListRuns: async (id: number) => runsOf(id),
        GetRun: async (id: number) => runs.find(x => x.id === id).result,
        DeleteRun: async (id: number) => { runs.splice(runs.findIndex(x => x.id === id), 1); },
        RunningSuite: async () => running,
        RunSuite: async (id: number) => {
            if (running) throw new Error('a test is already running');
            const suite = find(id), started = Date.now(), length = 10; // simulated runs take 10s
            running = id;
            let total = 0;
            const path = suite.config.method + ' ' + new URL(suite.config.url).pathname;
            const log = (level: string, kind: string, vu: number, message: string, status = 0, latencyMs = 0) =>
                emit('test:log', {elapsedSec: (Date.now() - started) / 1000, level, kind, vu, message, status, latencyMs});
            setTimeout(() => {
                log('info', 'system', 0, `Starting ${suite.config.method} ${suite.config.url}`);
                log('info', 'system', 0, `Stage 1/2: ramping 0 → ${suite.config.vus || 20} virtual users over 3s`);
            });
            const finish = (stopped: boolean) => {
                clearInterval(timer);
                const result = makeResult(suite, started, 110 + Math.random() * 40, 0, stopped);
                if (stopped) log('warn', 'system', 0, 'Stopped before the planned end');
                log('ok', 'system', 0, `Finished: ${result.total} requests in 10.0s · ${result.rps.toFixed(1)} req/s · 0 failed`);
                result.checks.forEach((c: any) => log(c.passed ? 'ok' : 'error', 'system', 0,
                    `Threshold ${c.threshold.metric} ${c.threshold.op} ${c.threshold.value}: ${c.passed ? 'PASS' : 'FAIL'} (actual ${c.actual.toFixed(2)})`));
                const run = {id: nextId++, suiteId: id, result};
                runs.push(run);
                running = 0;
                emit('test:done', {suiteId: id, runId: run.id, result, error: ''});
            };
            stopNow = () => finish(true);
            timer = window.setInterval(() => {
                const t = (Date.now() - started) / 1000, vus = Math.round(Math.min(1, t / 3) * (suite.config.vus || 20));
                const rps = vus * 9 + Math.random() * 15;
                total += Math.round(rps / 2);
                emit('test:progress', {elapsedSec: t, total, success: total, failed: 0, rps,
                    avgLatencyMs: 90 + vus * 1.5 + Math.random() * 12, activeVUs: vus});
                if (Math.abs(t - 3) < 0.26) log('info', 'system', 0, `Stage 2/2: holding ${suite.config.vus || 20} virtual users for 7s`);
                for (let i = 0; i < 4; i++) log('ok', 'request', 1 + Math.floor(Math.random() * Math.max(vus, 1)), path, 200, 80 + Math.random() * 90);
                if (Math.random() < 0.18) log('error', 'request', 1 + Math.floor(Math.random() * Math.max(vus, 1)), path, 503, 300 + Math.random() * 200);
                if (t >= length) finish(false);
            }, 500);
        },
        StopTest: async () => { if (running) stopNow(); },
        OpenRunPDF: async () => { throw new Error('the PDF report needs the desktop app (preview mode)'); },
        ExportRun: async () => { throw new Error('Export needs the desktop app (preview mode)'); },
        SaveScenario: async () => { throw new Error('Saving files needs the desktop app (preview mode)'); },
        LoadScenario: async () => { throw new Error('Opening files needs the desktop app (preview mode)'); },
    }}};
}
