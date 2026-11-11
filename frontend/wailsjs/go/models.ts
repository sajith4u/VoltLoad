export namespace loader {
	
	export class Threshold {
	    metric: string;
	    op: string;
	    value: number;
	
	    static createFrom(source: any = {}) {
	        return new Threshold(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.metric = source["metric"];
	        this.op = source["op"];
	        this.value = source["value"];
	    }
	}
	export class CheckResult {
	    threshold: Threshold;
	    actual: number;
	    passed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CheckResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.threshold = this.convertValues(source["threshold"], Threshold);
	        this.actual = source["actual"];
	        this.passed = source["passed"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Stage {
	    durationSec: number;
	    vus: number;
	
	    static createFrom(source: any = {}) {
	        return new Stage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.durationSec = source["durationSec"];
	        this.vus = source["vus"];
	    }
	}
	export class Config {
	    name: string;
	    method: string;
	    url: string;
	    headers: Record<string, string>;
	    body: string;
	    vus: number;
	    durationSec: number;
	    requests: number;
	    timeoutMs: number;
	    thinkMinMs: number;
	    thinkMaxMs: number;
	    stages: Stage[];
	    thresholds: Threshold[];
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.method = source["method"];
	        this.url = source["url"];
	        this.headers = source["headers"];
	        this.body = source["body"];
	        this.vus = source["vus"];
	        this.durationSec = source["durationSec"];
	        this.requests = source["requests"];
	        this.timeoutMs = source["timeoutMs"];
	        this.thinkMinMs = source["thinkMinMs"];
	        this.thinkMaxMs = source["thinkMaxMs"];
	        this.stages = this.convertValues(source["stages"], Stage);
	        this.thresholds = this.convertValues(source["thresholds"], Threshold);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Latency {
	    min: number;
	    avg: number;
	    p50: number;
	    p90: number;
	    p95: number;
	    p99: number;
	    max: number;
	
	    static createFrom(source: any = {}) {
	        return new Latency(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.min = source["min"];
	        this.avg = source["avg"];
	        this.p50 = source["p50"];
	        this.p90 = source["p90"];
	        this.p95 = source["p95"];
	        this.p99 = source["p99"];
	        this.max = source["max"];
	    }
	}
	export class Snapshot {
	    elapsedSec: number;
	    total: number;
	    success: number;
	    failed: number;
	    rps: number;
	    avgLatencyMs: number;
	    activeVUs: number;
	
	    static createFrom(source: any = {}) {
	        return new Snapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.elapsedSec = source["elapsedSec"];
	        this.total = source["total"];
	        this.success = source["success"];
	        this.failed = source["failed"];
	        this.rps = source["rps"];
	        this.avgLatencyMs = source["avgLatencyMs"];
	        this.activeVUs = source["activeVUs"];
	    }
	}
	export class VUStat {
	    id: number;
	    requests: number;
	    failed: number;
	    avgMs: number;
	
	    static createFrom(source: any = {}) {
	        return new VUStat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.requests = source["requests"];
	        this.failed = source["failed"];
	        this.avgMs = source["avgMs"];
	    }
	}
	export class Result {
	    config: Config;
	    // Go type: time
	    startedAt: any;
	    durationSec: number;
	    total: number;
	    success: number;
	    failed: number;
	    rps: number;
	    latencyMs: Latency;
	    statusCodes: Record<number, number>;
	    errors: Record<string, number>;
	    stopped: boolean;
	    vus: VUStat[];
	    timeline: Snapshot[];
	    checks: CheckResult[];
	    passed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.config = this.convertValues(source["config"], Config);
	        this.startedAt = this.convertValues(source["startedAt"], null);
	        this.durationSec = source["durationSec"];
	        this.total = source["total"];
	        this.success = source["success"];
	        this.failed = source["failed"];
	        this.rps = source["rps"];
	        this.latencyMs = this.convertValues(source["latencyMs"], Latency);
	        this.statusCodes = source["statusCodes"];
	        this.errors = source["errors"];
	        this.stopped = source["stopped"];
	        this.vus = this.convertValues(source["vus"], VUStat);
	        this.timeline = this.convertValues(source["timeline"], Snapshot);
	        this.checks = this.convertValues(source["checks"], CheckResult);
	        this.passed = source["passed"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	

}

export namespace store {
	
	export class RunSummary {
	    id: number;
	    name: string;
	    method: string;
	    url: string;
	    startedAt: string;
	    total: number;
	    failed: number;
	    rps: number;
	    p95Ms: number;
	    passed?: boolean;
	    avgMs: number;
	    durationSec: number;
	    ok: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RunSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.method = source["method"];
	        this.url = source["url"];
	        this.startedAt = source["startedAt"];
	        this.total = source["total"];
	        this.failed = source["failed"];
	        this.rps = source["rps"];
	        this.p95Ms = source["p95Ms"];
	        this.passed = source["passed"];
	        this.avgMs = source["avgMs"];
	        this.durationSec = source["durationSec"];
	        this.ok = source["ok"];
	    }
	}
	export class Suite {
	    id: number;
	    name: string;
	    description: string;
	    config: loader.Config;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Suite(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.config = this.convertValues(source["config"], loader.Config);
	        this.updatedAt = source["updatedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SuiteSummary {
	    id: number;
	    name: string;
	    description: string;
	    method: string;
	    url: string;
	    updatedAt: string;
	    recent: RunSummary[];
	
	    static createFrom(source: any = {}) {
	        return new SuiteSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.method = source["method"];
	        this.url = source["url"];
	        this.updatedAt = source["updatedAt"];
	        this.recent = this.convertValues(source["recent"], RunSummary);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

