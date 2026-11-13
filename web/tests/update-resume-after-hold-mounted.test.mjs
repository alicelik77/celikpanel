// alpha.133 lab, server B (13 Nov 2026): the update window stayed on step 2
// for more than ten minutes after the update had finished. The access gate
// held the page while the Panel restarted (its decision ran out before the
// tracking had seen the restart), which pauses the update tracking; when the
// Panel answered again the tracking never read the update's status again.
// The browser had updated this server once before, so the followed record
// carried that earlier update's reload, already done in this tab. The resume
// picked that old request instead of the followed one, its guard could never
// settle (the navigation lock had been released by the pause), and nothing
// woke the status reads again.
//
// The real update provider and navigation lease are mounted with in-memory
// stand-ins for storage, locks and router only. The first case fails against
// 455b31173.
//
// Erisim kapisi guncelleme sirasinda sayfayi tuttuktan sonra izleme yeniden
// baslar; sekmede zaten yapilmis eski yeniden yukleme izlenen istegin yerine
// gecmez.
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import ts from 'typescript';
import { en } from '../src/i18n/en.ts';
import { enScreens } from './locale-catalogue.mjs';

const require = createRequire(import.meta.url);
const moduleURL = source => 'data:text/javascript;base64,' + Buffer.from(source).toString('base64');
function compileURL(relativePath, replacements = []) {
    let compiled = ts.transpileModule(readFileSync(new URL(relativePath, import.meta.url), 'utf8'), {
        compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020, jsx: ts.JsxEmit.ReactJSX },
    }).outputText;
    for (const [pattern, replacement] of replacements) compiled = compiled.replace(pattern, replacement);
    return moduleURL(compiled);
}
const from = (path, url) => [new RegExp(`from ['"]${path.replace(/[.*+?^${}()|[\]\\/]/g, '\\$&')}['"]`, 'g'), `from '${url}'`];
const reactURL = pathToFileURL(require.resolve('react')).href;
const jsxRuntimeURL = pathToFileURL(require.resolve('react/jsx-runtime')).href;

class MemoryStorage {
    constructor() { this.map = new Map(); }
    get length() { return this.map.size; }
    key(index) { return [...this.map.keys()][index] ?? null; }
    getItem(key) { return this.map.has(key) ? this.map.get(key) : null; }
    setItem(key, value) { this.map.set(key, String(value)); }
    removeItem(key) { this.map.delete(key); }
    clear() { this.map.clear(); }
}
const noopEvents = { addEventListener() {}, removeEventListener() {} };
globalThis.localStorage = new MemoryStorage();
globalThis.sessionStorage = new MemoryStorage();
globalThis.indexedDB = {};
globalThis.HTMLElement = class {};
globalThis.document = Object.assign(new EventTarget(), { visibilityState: 'visible', activeElement: null, body: { style: { overflow: '' } }, documentElement: null, getElementById: () => null, querySelector: () => null });
const replaced = [];
globalThis.window = Object.assign(globalThis.window ?? {}, noopEvents, {
    setTimeout: (...args) => setTimeout(...args),
    clearTimeout: id => clearTimeout(id),
    setInterval: (...args) => setInterval(...args),
    clearInterval: id => clearInterval(id),
    location: { href: 'https://panel.example:1923/settings?section=updates', replace: url => { replaced.push(url); } },
});

const MARKER_KEY = 'celikpanel.system-update-operation.v1';
const RELOADED_KEY = 'celikpanel.system-update-reloaded.v1';
const canonical = { snapshot: null };
globalThis.__canonical = canonical;
const leaseURL = moduleURL(`export * from '${compileURL('../src/lib/systemUpdateLease.ts')}';
export function transactCanonicalRecord(options, mutate) {
    const store = globalThis.__canonical;
    const current = store.snapshot ?? { canonical_version: 1, revision: 0, writer_id: options.ownerID, record: null };
    const decision = mutate(current.record, current);
    if (!decision) return Promise.resolve({ kind: 'failed' });
    if (decision.kind === 'write') {
        store.snapshot = { canonical_version: 1, revision: current.revision + 1, writer_id: options.ownerID, record: decision.record };
        if (decision.record) options.mirrorStorage?.setItem(options.mirrorKey, options.codec.encode(decision.record));
        else options.mirrorStorage?.removeItem(options.mirrorKey);
    } else {
        store.snapshot = current;
    }
    return Promise.resolve({ kind: 'committed', snapshot: store.snapshot, result: decision.result });
}`);
const authURL = compileURL('../src/lib/systemUpdateAuthSignal.ts');
const trackingURL = compileURL('../src/lib/systemUpdateTracking.ts');
const recoveryURL = compileURL('../src/lib/recoveryObservation.ts');
const failureURL = compileURL('../src/lib/systemUpdateFailure.ts');
const watchdogURL = compileURL('../src/lib/systemUpdateWatchdog.ts');
const outcomeURL = compileURL('../src/lib/systemUpdateOutcome.ts', [from('./recoveryObservation', recoveryURL), from('./systemUpdateFailure', failureURL)]);
const iconsURL = moduleURL(`import React from '${reactURL}';
const icon = name => props => React.createElement('i', { ...props, 'data-icon': name });
export const AlertTriangle = icon('AlertTriangle'), CheckCircle2 = icon('CheckCircle2'), DownloadCloud = icon('DownloadCloud'),
    Info = icon('Info'), Loader2 = icon('Loader2'), RefreshCw = icon('RefreshCw'), WifiOff = icon('WifiOff'), XCircle = icon('XCircle');`);
const strings = { ...en, ...enScreens };
// One translation function for the whole run, as the real I18nProvider gives
// its consumers while it does not re-render itself: a new function per render
// would re-run the provider's effects and hide a tracking that stopped.
const i18nURL = moduleURL(`const fill = (text, vars) => String(text).replace(/\\{(\\w+)\\}/g, (all, name) => vars && vars[name] !== undefined ? String(vars[name]) : all);
const t = (key, vars) => fill(globalThis.__resumeStrings[key] ?? key, vars);
const value = { t, locale: 'en', screensReady: true, screensFailed: false };
export function useI18n() { return value; }`);
globalThis.__resumeStrings = strings;
const operationURL = compileURL('../src/components/SystemUpdateOperation.tsx', [
    from('react/jsx-runtime', jsxRuntimeURL),
    from('react', reactURL),
    from('react-dom', moduleURL('export function flushSync(fn) { return fn(); }')),
    from('lucide-react', iconsURL),
    from('./ui', moduleURL(`import React from '${reactURL}'; export function Button({ children, ...props }) { return React.createElement('button', props, children); }`)),
    from('../i18n', i18nURL),
    from('../router', moduleURL('export function replaceCurrentRouterURL() { return true; } export function useNavigationBlocker() {}')),
    from('../lib/apiError', moduleURL(`export async function readApiError() { return {}; } export function apiErrorText() { return 'api-error'; }`)),
    from('../lib/recoveryShell', moduleURL('export async function prepareRecoveryShell() {}')),
    from('../lib/systemUpdateFailure', failureURL),
    from('../lib/systemUpdateOutcome', outcomeURL),
    from('../lib/recoveryObservation', recoveryURL),
    from('../lib/systemUpdateLease', leaseURL),
    from('../lib/systemUpdateAuthSignal', authURL),
    from('../lib/systemUpdateWatchdog', watchdogURL),
    // The real navigation lease: the pause releases the lock that was shown.
    from('../lib/useSystemUpdateNavigationLease', compileURL('../src/lib/useSystemUpdateNavigationLease.ts', [from('react', reactURL), from('./systemUpdateWatchdog', watchdogURL)])),
    from('../lib/systemUpdateRestartSignal', compileURL('../src/lib/systemUpdateRestartSignal.ts', [from('react', reactURL), from('./systemUpdateTracking', trackingURL)])),
    from('../lib/systemUpdateTracking', trackingURL),
    from('../lib/systemUpdateSteps', compileURL('../src/lib/systemUpdateSteps.ts')),
    from('./SystemUpdateSteps', moduleURL('export function SystemUpdateSteps() { return null; } export function systemUpdateStepsAnnouncement() { return ""; }')),
    from('./BuildCommit', moduleURL('export function BuildCommit() { return null; }')),
]);
const { SystemUpdateOperationProvider, useSystemUpdateOperation } = await import(operationURL);
const { publishSystemUpdateAuthentication } = await import(authURL);
function Probe() { globalThis.__context = useSystemUpdateOperation(); return null; }

const target = version => ({ version, commit: (version.endsWith('133') ? 'c' : 'd').repeat(40), sequence: version.split('.').pop(), os: 'linux', arch: 'amd64', archive_sha256: 'e'.repeat(64), archive_size: '123456' });
const NEW_ID = '59ac41ac4fad8d677e990617cb82d2d2';
const OLD_ID = '0b1d2e3f405162738495a6b7c8d9e0f1';
// The update this browser made on this server before (its reload done in this tab).
const OLD = { marker_version: 1, request_id: OLD_ID, current_version: 'v0.1.0-alpha.131', current_commit: 'a'.repeat(40), target: target('v0.1.0-alpha.132'), created_at: Date.now() - 86_400_000 };
const NEW = { marker_version: 1, request_id: NEW_ID, current_version: 'v0.1.0-alpha.132', current_commit: 'b'.repeat(40), target: target('v0.1.0-alpha.133'), created_at: Date.now() - 5_000 };
const fingerprint = marker => JSON.stringify({ state_version: 1, phase: 'active', marker });

const settle = async (ms) => { const end = Date.now() + ms; while (Date.now() < end) await act(async () => { await new Promise(resolve => setTimeout(resolve, 20)); }); };

async function mountFollowing({ carriedReload, authenticated = true, created = NEW.created_at }) {
    NEW.created_at = created;
    localStorage.clear();
    sessionStorage.clear();
    replaced.length = 0;
    const record = { state_version: 1, phase: 'active', marker: NEW, ...(carriedReload ? { required_reload: OLD } : {}) };
    localStorage.setItem(MARKER_KEY, JSON.stringify(record));
    if (carriedReload) sessionStorage.setItem(RELOADED_KEY, fingerprint(OLD));
    canonical.snapshot = { canonical_version: 1, revision: 1, writer_id: 'f'.repeat(32), record };
    const server = { status: 'running', reads: [] };
    globalThis.fetch = async (url, options) => {
        server.reads.push([String(url), options?.method ?? 'GET']);
        if (String(url) === `/api/v1/panel/update/status?request_id=${NEW_ID}`) {
            return { ok: true, status: 200, json: async () => ({ found: true, request_id: NEW_ID, status: server.status, target: NEW.target }) };
        }
        return { ok: false, status: 404, json: async () => ({}) };
    };
    publishSystemUpdateAuthentication(authenticated);
    // Host nodes as a browser gives them, so the update dialogue can take focus.
    const createNodeMock = () => ({ isConnected: true, inert: false, focus() {}, setAttribute() {}, removeAttribute() {} });
    let renderer;
    await act(async () => {
        renderer = TestRenderer.create(React.createElement(SystemUpdateOperationProvider, null, React.createElement('main', null, 'page'), React.createElement(Probe)), { createNodeMock });
    });
    await settle(300);
    return { renderer, server };
}
const statusReads = server => server.reads.filter(([url]) => url.startsWith('/api/v1/panel/update/status?')).length;

for (const carriedReload of [true, false]) {
    test(`a hold during the update's restart pauses the tracking, and access returning resumes it (${carriedReload ? 'after an earlier update from this browser' : 'first update from this browser'})`, async () => {
        const { renderer, server } = await mountFollowing({ carriedReload, created: Date.now() - 5_000 });
        try {
            assert.ok(statusReads(server) >= 1, 'the followed update is read');
            assert.equal(globalThis.__context.tracking?.requestID, NEW_ID);
            // The access gate holds the page (App publishes false), the Panel restarts.
            await act(async () => { publishSystemUpdateAuthentication(false); });
            await settle(2_000);
            const whilePaused = statusReads(server);
            await settle(1_800);
            assert.equal(statusReads(server), whilePaused, 'nothing is read while the tracking is paused');
            // The new Panel answers: the update finished.
            server.status = 'succeeded';
            await act(async () => { publishSystemUpdateAuthentication(true); });
            await settle(3_500);
            assert.ok(statusReads(server) > whilePaused, 'the followed update is read again once access returns');
            assert.equal(canonical.snapshot.record?.phase, 'terminal');
            assert.equal(canonical.snapshot.record?.outcome, 'succeeded');
            assert.equal(canonical.snapshot.record?.marker.request_id, NEW_ID, 'the followed request, never the earlier one');
            assert.equal(replaced.length, 1, 'the page reloads once into the new version');
            assert.ok(replaced[0].includes(`_cp_update=${NEW_ID}`));
            assert.ok(server.reads.every(([, method]) => method === 'GET'), 'resuming reads only; nothing is started');
        } finally { await act(async () => renderer.unmount()); }
    });

    // The lab's second view: the page opened again ten minutes later read
    // "Reading the state of the update…" (the session and licence reads answer
    // after the browser's own record is ready, so the tracking starts paused).
    test(`a page opened after the restart reads the followed update once access is confirmed (${carriedReload ? 'after an earlier update from this browser' : 'first update from this browser'})`, async () => {
        const { renderer, server } = await mountFollowing({ carriedReload, authenticated: false, created: Date.now() - 10 * 60_000 });
        try {
            assert.equal(statusReads(server), 0, 'nothing is read before access is confirmed');
            assert.equal(globalThis.__context.tracking?.phase, 'reading');
            server.status = 'succeeded';
            await act(async () => { publishSystemUpdateAuthentication(true); });
            await settle(3_500);
            assert.ok(statusReads(server) >= 1, 'the followed update is read once access is confirmed');
            assert.equal(canonical.snapshot.record?.outcome, 'succeeded');
            assert.equal(canonical.snapshot.record?.marker.request_id, NEW_ID);
            assert.equal(replaced.length, 1);
            assert.ok(server.reads.every(([, method]) => method === 'GET'));
        } finally { await act(async () => renderer.unmount()); }
    });
}
