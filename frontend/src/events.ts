import {loader} from '../wailsjs/go/models';

// Mirrors loader.Snapshot and gui.DoneEvent; Wails only generates types for bound method signatures.
export interface Snapshot {
    elapsedSec: number;
    total: number;
    success: number;
    failed: number;
    rps: number;
    avgLatencyMs: number;
    activeVUs: number;
}

export interface DoneEvent {
    suiteId: number;
    runId: number;
    result: loader.Result;
    error: string;
}

/** Mirrors loader.LogEntry. */
export interface LogEntry {
    elapsedSec: number;
    level: 'info' | 'ok' | 'warn' | 'error';
    kind: 'system' | 'request';
    vu: number; // 0 = the engine itself
    message: string;
    status: number;
    latencyMs: number;
}

export const EventLog = 'test:log';
export const EventProgress = 'test:progress';
export const EventDone = 'test:done';
