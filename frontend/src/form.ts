import {loader, store} from '../wailsjs/go/models';

export type Stage = { durationSec: number; vus: number };
export type Threshold = { metric: string; op: string; value: number };

/** Editable shape of a suite; headers are text so they can be typed one per line. */
export interface Form {
    id: number;
    name: string;
    description: string;
    method: string;
    url: string;
    headers: string;
    body: string;
    mode: 'constant' | 'stages';
    vus: number;
    durationSec: number;
    stages: Stage[];
    requests: number;
    timeoutMs: number;
    thinkMinMs: number;
    thinkMaxMs: number;
    thresholds: Threshold[];
}

export const PRESETS: Record<string, { hint: string; stages: Stage[] }> = {
    Smoke: {hint: '1 VU for 30s – does it work at all?', stages: [{durationSec: 30, vus: 1}]},
    Load: {hint: 'Ramp to 20 VUs, hold 5 min, ramp down', stages: [
        {durationSec: 60, vus: 20}, {durationSec: 300, vus: 20}, {durationSec: 60, vus: 0}]},
    Stress: {hint: 'Step up to 100 VUs to find the breaking point', stages: [
        {durationSec: 60, vus: 50}, {durationSec: 120, vus: 50}, {durationSec: 60, vus: 100},
        {durationSec: 120, vus: 100}, {durationSec: 60, vus: 0}]},
    Spike: {hint: 'Sudden jump from 5 to 100 VUs and back', stages: [
        {durationSec: 10, vus: 5}, {durationSec: 30, vus: 5}, {durationSec: 10, vus: 100},
        {durationSec: 30, vus: 100}, {durationSec: 10, vus: 5}, {durationSec: 30, vus: 5}]},
};

export const emptyForm: Form = {
    id: 0, name: '', description: '', method: 'GET', url: 'https://', headers: '', body: '',
    mode: 'constant', vus: 10, durationSec: 30, stages: PRESETS.Load.stages, requests: 0, timeoutMs: 30000,
    thinkMinMs: 0, thinkMaxMs: 0, thresholds: [],
};

function parseHeaders(text: string): Record<string, string> {
    const out: Record<string, string> = {};
    for (const line of text.split('\n')) {
        const i = line.indexOf(':');
        if (i > 0) out[line.slice(0, i).trim()] = line.slice(i + 1).trim();
    }
    return out;
}

export function toConfig(f: Form): loader.Config {
    return loader.Config.createFrom({
        name: f.name.trim(), method: f.method, url: f.url, body: f.body, headers: parseHeaders(f.headers),
        vus: f.mode === 'constant' ? f.vus : 0,
        durationSec: f.mode === 'constant' ? f.durationSec : 0,
        stages: f.mode === 'stages' ? f.stages : [],
        requests: f.requests, timeoutMs: f.timeoutMs, thresholds: f.thresholds,
        thinkMinMs: f.thinkMinMs, thinkMaxMs: f.thinkMaxMs,
    });
}

export function toSuite(f: Form): store.Suite {
    return store.Suite.createFrom({id: f.id, name: f.name.trim(), description: f.description.trim(), config: toConfig(f)});
}

export function fromConfig(c: loader.Config, base: Form = emptyForm): Form {
    const staged = (c.stages?.length ?? 0) > 0;
    return {
        ...base,
        name: c.name || base.name, method: c.method || 'GET', url: c.url, body: c.body ?? '',
        headers: Object.entries(c.headers ?? {}).map(([k, v]) => `${k}: ${v}`).join('\n'),
        mode: staged ? 'stages' : 'constant',
        vus: staged ? emptyForm.vus : c.vus || emptyForm.vus,
        durationSec: staged ? emptyForm.durationSec : c.durationSec ?? 0,
        stages: staged ? c.stages : emptyForm.stages,
        requests: c.requests ?? 0, timeoutMs: c.timeoutMs || emptyForm.timeoutMs, thresholds: c.thresholds ?? [],
        thinkMinMs: c.thinkMinMs ?? 0, thinkMaxMs: c.thinkMaxMs ?? 0,
    };
}

export function fromSuite(s: store.Suite): Form {
    return {...fromConfig(s.config), id: s.id, name: s.name, description: s.description ?? ''};
}

/** "30s", "5m", "1m 30s" */
export function fmtDuration(sec: number): string {
    if (sec < 60) return `${Math.round(sec)}s`;
    const m = Math.floor(sec / 60), s = Math.round(sec % 60);
    return s ? `${m}m ${s}s` : `${m}m`;
}

/** "5h ago", "2 days ago" */
export function timeAgo(iso: string): string {
    const sec = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000);
    if (sec < 60) return 'just now';
    if (sec < 3600) return `${Math.floor(sec / 60)} min ago`;
    if (sec < 86400) return `${Math.floor(sec / 3600)}h ago`;
    const d = Math.floor(sec / 86400);
    return `${d} day${d > 1 ? 's' : ''} ago`;
}
