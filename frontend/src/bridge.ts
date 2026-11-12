// Connects the UI to the Go backend. Inside the desktop window Wails provides window.go and
// window.runtime. In a browser (`voltload serve`, or the Vite dev server proxying /api to it) this
// installs the same two objects backed by HTTP calls and a Server-Sent Events stream, so the
// generated wailsjs bindings work unchanged against the real backend.
const w = window as any;

export const isDesktop = !!(w.go?.gui?.App && w.runtime);

/** Shown when the browser can't reach the backend. */
export const OFFLINE = 'Cannot reach the VoltLoad backend. Start it with `voltload serve` and reload this page.';

async function call(method: string, args: unknown[]) {
    let resp: Response;
    try {
        resp = await fetch(`/api/call/${method}`, {
            method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(args),
        });
    } catch {
        throw new Error(OFFLINE);
    }
    const body = await resp.json().catch(() => null);
    if (!body) throw new Error(resp.status === 404 || resp.status >= 500 ? OFFLINE : `HTTP ${resp.status}`);
    if (body.error) throw new Error(body.error);
    return body.result;
}

if (!isDesktop) {
    const handlers: Record<string, Set<(data: unknown) => void>> = {};
    const source = new EventSource('/api/events'); // reconnects on its own if the backend restarts
    const listen = (name: string) => source.addEventListener(name, e =>
        handlers[name]?.forEach(cb => cb(JSON.parse((e as MessageEvent).data))));

    w.go = {gui: {App: new Proxy({}, {get: (_, method: string) => (...args: unknown[]) => call(method, args)})}};
    w.runtime = {
        EventsOnMultiple(name: string, cb: (data: unknown) => void) {
            if (!handlers[name]) { handlers[name] = new Set(); listen(name); }
            handlers[name].add(cb);
            return () => handlers[name].delete(cb);
        },
    };
}

/** Saves text as a file through the browser's download flow. */
export function downloadText(text: string, filename: string, type = 'text/plain') {
    const url = URL.createObjectURL(new Blob([text], {type}));
    const a = Object.assign(document.createElement('a'), {href: url, download: filename});
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
}

/** Asks the user for a file and resolves with its text, or null if they cancel. */
export function pickTextFile(accept: string): Promise<string | null> {
    return new Promise(resolve => {
        const input = Object.assign(document.createElement('input'), {type: 'file', accept});
        input.onchange = () => input.files?.[0] ? input.files[0].text().then(resolve) : resolve(null);
        input.oncancel = () => resolve(null);
        input.click();
    });
}
