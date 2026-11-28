// alpha.150 lab D5 (28 Nov 2026): Update pressed while a site was being moved
// into its limits. The Panel refused at once (409
// PANEL_UPDATE_SITE_AUTO_MOVE_RUNNING) and started nothing, but the page kept
// the request as a failed update, locked the navigation for two minutes with
// "Checking the update's result" and then said the version it runs is not
// known and to start no other update.
//
// The real update provider is mounted (in-memory stand-ins for storage, locks
// and router only) and its start request is answered with exactly the JSON
// the Panel's real handler wrote for each refusal it gives before anything
// starts (web/tests/fixtures/real-path/panel-update-start-refusals.json, kept
// by cmd/panel/real_path_refusals_test.go from the Agent's real answers).
// Each refusal says its own words within the press, leaves nothing locked and
// no notice, reads no recovery record, and the next press starts the update.
//
// Hiçbir şey başlamadan reddedilen güncelleme: kendi sözleri, kilit yok,
// "sonuç bilinmiyor" yok; sonraki basış güncellemeyi başlatır.
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import ts from 'typescript';
import { en } from '../src/i18n/en.ts';
import { tr } from '../src/i18n/tr.ts';
import { enScreens, trScreens } from './locale-catalogue.mjs';

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
globalThis.window = Object.assign(globalThis.window ?? {}, noopEvents, {
    setTimeout: (...args) => setTimeout(...args),
    clearTimeout: id => clearTimeout(id),
    setInterval: (...args) => setInterval(...args),
    clearInterval: id => clearInterval(id),
    location: { href: 'https://panel.example:1923/settings?section=updates', replace() {} },
});

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
// The real refusal words (apiErrorText), as the page says them.
const apiErrorURL = compileURL('../src/lib/apiError.ts');
const outcomeURL = compileURL('../src/lib/systemUpdateOutcome.ts', [from('./recoveryObservation', recoveryURL), from('./systemUpdateFailure', failureURL)]);
const iconsURL = moduleURL(`import React from '${reactURL}';
const icon = name => props => React.createElement('i', { ...props, 'data-icon': name });
export const AlertTriangle = icon('AlertTriangle'), CheckCircle2 = icon('CheckCircle2'), DownloadCloud = icon('DownloadCloud'),
    Info = icon('Info'), Loader2 = icon('Loader2'), RefreshCw = icon('RefreshCw'), WifiOff = icon('WifiOff'), XCircle = icon('XCircle');`);
const catalogues = { en: { ...en, ...enScreens }, tr: { ...tr, ...trScreens } };
globalThis.__refusalStrings = catalogues.en;
const i18nURL = moduleURL(`const fill = (text, vars) => String(text).replace(/\\{(\\w+)\\}/g, (all, name) => vars && vars[name] !== undefined ? String(vars[name]) : all);
const t = (key, vars) => fill(globalThis.__refusalStrings[key] ?? key, vars);
const value = { t, locale: 'en', screensReady: true, screensFailed: false };
export function useI18n() { return value; }`);
const operationURL = compileURL('../src/components/SystemUpdateOperation.tsx', [
    from('react/jsx-runtime', jsxRuntimeURL),
    from('react', reactURL),
    from('react-dom', moduleURL('export function flushSync(fn) { return fn(); }')),
    from('lucide-react', iconsURL),
    from('./ui', moduleURL(`import React from '${reactURL}'; export function Button({ children, ...props }) { return React.createElement('button', props, children); }`)),
    from('../i18n', i18nURL),
    from('../router', moduleURL('export function replaceCurrentRouterURL() { return true; } export function useNavigationBlocker() {}')),
    from('../lib/apiError', apiErrorURL),
    from('../lib/recoveryShell', moduleURL('export async function prepareRecoveryShell() {}')),
    from('../lib/systemUpdateFailure', failureURL),
    from('../lib/systemUpdateOutcome', outcomeURL),
    from('../lib/recoveryObservation', recoveryURL),
    from('../lib/systemUpdateLease', leaseURL),
    from('../lib/systemUpdateAuthSignal', authURL),
    from('../lib/systemUpdateWatchdog', watchdogURL),
    from('../lib/useSystemUpdateNavigationLease', compileURL('../src/lib/useSystemUpdateNavigationLease.ts', [from('react', reactURL), from('./systemUpdateWatchdog', watchdogURL)])),
    from('../lib/systemUpdateRestartSignal', compileURL('../src/lib/systemUpdateRestartSignal.ts', [from('react', reactURL), from('./systemUpdateTracking', trackingURL)])),
    from('../lib/systemUpdateTracking', trackingURL),
    from('../lib/systemUpdateSteps', compileURL('../src/lib/systemUpdateSteps.ts')),
    from('./SystemUpdateSteps', moduleURL('export function SystemUpdateSteps() { return null; } export function systemUpdateStepsAnnouncement() { return ""; }')),
    from('./BuildCommit', moduleURL('export function BuildCommit() { return null; }')),
]);
const { SystemUpdateOperationProvider, useSystemUpdateOperation } = await import(operationURL);
const { publishSystemUpdateAuthentication } = await import(authURL);
const { apiErrorText } = await import(apiErrorURL);
function Probe() { globalThis.__context = useSystemUpdateOperation(); return null; }
const sayFrom = (catalogue) => (key, vars) => String(catalogue[key] ?? key).replace(/\{(\w+)\}/g, (all, name) => vars && vars[name] !== undefined ? String(vars[name]) : all);
const say = sayFrom(catalogues.en);
const sayTR = sayFrom(catalogues.tr);

// Exactly what the Panel's real handler answered (cmd/panel/real_path_refusals_test.go).
const refusals = JSON.parse(readFileSync(new URL('./fixtures/real-path/panel-update-start-refusals.json', import.meta.url), 'utf8'));
const settle = async (ms) => { const end = Date.now() + ms; while (Date.now() < end) await act(async () => { await new Promise(resolve => setTimeout(resolve, 20)); }); };
const target = { version: 'v0.1.0-alpha.163', commit: 'd'.repeat(40), sequence: '163', os: 'linux', arch: 'amd64', archive_sha256: 'e'.repeat(64), archive_size: '123456' };
let counter = 0;
const newMarker = () => ({ marker_version: 1, request_id: (++counter).toString(16).padStart(32, '0'), current_version: 'v0.1.0-alpha.162', current_commit: 'c'.repeat(40), target, created_at: Date.now() });

async function mount(answer) {
    localStorage.clear();
    sessionStorage.clear();
    canonical.snapshot = null;
    const server = { reads: [], answer };
    globalThis.fetch = async (url, options) => {
        server.reads.push([String(url), options?.method ?? 'GET']);
        if (String(url) === '/api/v1/panel/update/start') return server.answer();
        if (String(url).startsWith('/api/v1/panel/update/status?')) {
            return new Response(JSON.stringify({ found: true, request_id: server.lastID, status: 'running', target }), { status: 200 });
        }
        return new Response('{}', { status: 404 });
    };
    publishSystemUpdateAuthentication(true);
    const createNodeMock = () => ({ isConnected: true, inert: false, focus() {}, setAttribute() {}, removeAttribute() {} });
    let renderer;
    await act(async () => {
        renderer = TestRenderer.create(React.createElement(SystemUpdateOperationProvider, null, React.createElement('main', null, 'page'), React.createElement(Probe)), { createNodeMock });
    });
    await settle(200);
    return { renderer, server };
}

test('the Panel answers every refusal before anything starts at once, with its code (the kept real answers)', () => {
    assert.deepEqual(Object.keys(refusals).sort(), ['agent_site_move_running', 'busy', 'package_manager_busy', 'site_auto_move_running', 'site_move_running', 'start_refused', 'target_changed']);
    for (const [name, { status, body }] of Object.entries(refusals)) {
        assert.equal(status, 409, name);
        assert.match(body.code, /^(PANEL_UPDATE_[A-Z_]+|HOST_MUTATION_BUSY)$/, name);
    }
    assert.equal(refusals.site_auto_move_running.body.code, 'PANEL_UPDATE_SITE_AUTO_MOVE_RUNNING');
});

for (const [name, answer] of Object.entries(refusals)) {
    test(`a refused start says its own words and locks nothing (${name})`, async () => {
        const { renderer, server } = await mount(() => new Response(JSON.stringify(answer.body), { status: answer.status }));
        try {
            const marker = newMarker();
            const started = Date.now();
            let result;
            await act(async () => { result = await globalThis.__context.start(marker); });
            await settle(300);
            const words = apiErrorText({ message: answer.body.error, code: answer.body.code, reason: answer.body.reason }, say);
            assert.equal(result.kind, 'failed', 'the card gets the refusal');
            assert.equal(result.message, words, 'in the refusal’s own words');
            const trWords = apiErrorText({ message: answer.body.error, code: answer.body.code, reason: answer.body.reason }, sayTR);
            assert.notEqual(trWords, answer.body.error, 'the page’s own words in both languages, not the server’s English');
            assert.ok(Date.now() - started < 5_000, 'within seconds');
            assert.equal(canonical.snapshot.record, null, 'no update is recorded in this browser');
            assert.equal(globalThis.__context.active, false, 'nothing is locked: Start stays usable');
            assert.equal(globalThis.__context.tracking, null, 'no update is followed');
            assert.ok(!server.reads.some(([url]) => url.startsWith('/api/v1/recovery/status')), 'no recovery record is read for a refusal');
            assert.equal(server.reads.filter(([url]) => url === '/api/v1/panel/update/start').length, 1, 'one start request');
            // The move ended: the next press starts the update.
            server.answer = () => new Response(JSON.stringify({ accepted: true, request_id: server.lastID, status: 'queued', target }), { status: 202 });
            const next = newMarker();
            server.lastID = next.request_id;
            let accepted;
            await act(async () => { accepted = await globalThis.__context.start(next); });
            assert.equal(accepted.kind, 'accepted');
            assert.equal(canonical.snapshot.record?.phase, 'active');
            assert.equal(canonical.snapshot.record?.marker.request_id, next.request_id);
        } finally { await act(async () => renderer.unmount()); }
    });
}

test('the refusal’s words exist in both languages and say what the lab expected (D5)', () => {
    const code = refusals.site_auto_move_running.body.code;
    assert.match(catalogues.en[`err.${code}`], /being moved into its limits[\s\S]*was not started[\s\S]*within about 12 minutes[\s\S]*Start the update again then/);
    assert.match(catalogues.tr[`err.${code}`], /sınırlarının içine alınıyor[\s\S]*başlatılmadı[\s\S]*12 dakika[\s\S]*yeniden başlatın/);
    assert.match(refusals.site_auto_move_running.body.error, /within about 12 minutes/);
});

test('an unanswered start keeps the update followed (its result is not known)', async () => {
    const { renderer, server } = await mount(() => Promise.reject(new TypeError('network')));
    try {
        const marker = newMarker();
        server.lastID = marker.request_id;
        let result;
        await act(async () => { result = await globalThis.__context.start(marker); });
        assert.equal(result.kind, 'accepted', 'not said as a refusal');
        assert.equal(canonical.snapshot.record?.phase, 'active', 'the request stays followed');
        assert.equal(globalThis.__context.active, true);
    } finally { await act(async () => renderer.unmount()); }
});
