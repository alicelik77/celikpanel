// alpha.97: an update that stopped in its read-only check says why, in the
// owner's language, and never the generic reason for a step it has words for.
//   - Frankfurt, 2026-10-17 (alpha.94 -> alpha.95): the stored line was
//     package_manager_busy (Debian's automatic package updates were running);
//     the dialog drew a generic failure. It is now a calm stop with the busy
//     package manager as its reason and when to start again.
//   - A recovery tool that could not be started or verified ('compatibility',
//     no step line) has its own words, with the server's first line in view.
//   - A refused-check step whose words live in the screen catalogue waits for
//     it ("Reading the reason…") instead of the generic reason; only when the
//     screen copy could not be loaded at all does the generic reason show.
// The real update provider is mounted with in-memory stand-ins for storage,
// locks and router only. Each case fails against alpha.96's source.
//
// Durdurulan güncelleme nedenini sahibin dilinde söyler; sözcüğü olan bir adım
// için genel neden gösterilmez.
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

// Browser stand-ins: storage, a location whose replace is recorded, events.
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
    location: { href: 'https://panel.example:2083/system/update', replace: url => { replaced.push(url); } },
});

const MARKER_KEY = 'celikpanel.system-update-operation.v1';
// The canonical record lives in IndexedDB in the browser; here it is memory,
// mirrored to localStorage exactly as the provider expects.
const canonical = { snapshot: null };
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
globalThis.__canonical = canonical;
const authURL = compileURL('../src/lib/systemUpdateAuthSignal.ts');
const trackingURL = compileURL('../src/lib/systemUpdateTracking.ts');
const recoveryURL = compileURL('../src/lib/recoveryObservation.ts');
const failureURL = compileURL('../src/lib/systemUpdateFailure.ts');
const outcomeURL = compileURL('../src/lib/systemUpdateOutcome.ts', [from('./recoveryObservation', recoveryURL), from('./systemUpdateFailure', failureURL)]);
const iconsURL = moduleURL(`import React from '${reactURL}';
const icon = name => props => React.createElement('i', { ...props, 'data-icon': name });
export const AlertTriangle = icon('AlertTriangle'), CheckCircle2 = icon('CheckCircle2'), DownloadCloud = icon('DownloadCloud'),
    Info = icon('Info'), Loader2 = icon('Loader2'), RefreshCw = icon('RefreshCw'), WifiOff = icon('WifiOff'), XCircle = icon('XCircle');`);
const i18nURL = moduleURL(`const fill = (text, vars) => String(text).replace(/\\{(\\w+)\\}/g, (all, name) => vars && vars[name] !== undefined ? String(vars[name]) : all);
export function useI18n() { const c = globalThis.__preflightCatalogue; const strings = c.screensReady ? c.all : c.shell; return { t: (key, vars) => fill(strings[key] ?? key, vars), locale: c.locale, screensReady: c.screensReady, screensFailed: c.screensFailed }; }`);
const stepsURL = compileURL('../src/components/SystemUpdateSteps.tsx', [
    from('react/jsx-runtime', jsxRuntimeURL), from('lucide-react', iconsURL), from('../i18n', i18nURL),
]);
const buildURL = compileURL('../src/components/BuildCommit.tsx', [from('react/jsx-runtime', jsxRuntimeURL), from('../i18n', i18nURL)]);
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
    from('../lib/systemUpdateWatchdog', compileURL('../src/lib/systemUpdateWatchdog.ts')),
    from('../lib/useSystemUpdateNavigationLease', moduleURL('export function useSystemUpdateNavigationLease(identity, createdAt, requested) { return { released: false, blocked: requested }; }')),
    from('../lib/systemUpdateRestartSignal', compileURL('../src/lib/systemUpdateRestartSignal.ts', [from('react', reactURL), from('./systemUpdateTracking', trackingURL)])),
    from('../lib/systemUpdateTracking', trackingURL),
    from('../lib/systemUpdateSteps', compileURL('../src/lib/systemUpdateSteps.ts')),
    from('./SystemUpdateSteps', stepsURL),
    from('./BuildCommit', buildURL),
]);
const { SystemUpdateOperationProvider, useSystemUpdateOperation } = await import(operationURL);
// What the update card reads from the provider.
function Probe() { globalThis.__context = useSystemUpdateOperation(); return null; }
const { publishSystemUpdateAuthentication } = await import(authURL);

const REQUEST = '13193201ba7a3d832bb64cc6d972f8a5';
const TARGET_COMMIT = '1743ac69edb0e5723dc60d992b53094dcc4b1431';
const marker = {
    marker_version: 1, request_id: REQUEST, current_version: 'v0.1.0-alpha.91', current_commit: 'a'.repeat(40),
    target: { version: 'v0.1.0-alpha.92', commit: TARGET_COMMIT, sequence: '92', os: 'linux', arch: 'amd64', archive_sha256: 'b'.repeat(64), archive_size: '123456' },
    created_at: Date.now() - 120_000,
};
const failedRecord = (message) => ({ state_version: 1, phase: 'terminal', marker, outcome: 'failed', message, reload_scheduled: false, completed_at: Date.now() - 5_000 });
const observation = (fields) => ({
    schema: 'celikpanel-recovery-status/v1', request_id: REQUEST, panel_state: 'ready', observation: 'known',
    observed_at: '2026-10-17T12:25:30Z', phase: 'failed', reason: 'update_failed', terminal_proof: 'none', previous_failure: 'update_failed', ...fields,
});
const BUSY = '!! CELIKPANEL_UPDATE_FAILURE code=package_manager_busy state=unchanged reason=the host package manager is active detail=';
const COMPATIBILITY = '!! CELIKPANEL_UPDATE_FAILURE code=recovery_runtime_preflight_failed state=unchanged reason=recovery runtime preflight step=compatibility: recovery launcher metadata is not trusted: /usr/libexec/celikpanel/recovery mode 0775, want 0755 detail=';
const REFUSED = '!! CELIKPANEL_UPDATE_FAILURE code=update_preflight_refused state=unchanged reason=update preflight step=agent_idle class=check_failed detail=agent package mutations are not idle';

function textOf(node) {
    if (node === null || node === undefined || typeof node === 'boolean') return '';
    if (typeof node === 'string' || typeof node === 'number') return String(node);
    if (Array.isArray(node)) return node.map(textOf).join('');
    return textOf(node.children);
}
const settle = async (rounds = 12) => { for (let i = 0; i < rounds; i += 1) await act(async () => { await new Promise(resolve => setTimeout(resolve, 5)); }); };

async function mountWith({ locale, message, answer, screensReady = true, screensFailed = false }) {
    const shell = locale === 'tr' ? tr : en;
    globalThis.__preflightCatalogue = { locale, shell, all: { ...shell, ...(locale === 'tr' ? trScreens : enScreens) }, screensReady, screensFailed };
    localStorage.clear();
    sessionStorage.clear();
    const record = failedRecord(message);
    localStorage.setItem(MARKER_KEY, JSON.stringify(record));
    canonical.snapshot = { canonical_version: 1, revision: 1, writer_id: 'b'.repeat(32), record };
    const reads = [];
    globalThis.fetch = async (url, options) => {
        reads.push([String(url), options?.method ?? 'GET']);
        if (String(url) === `/api/v1/recovery/status?request_id=${REQUEST}`) return { ok: true, status: 200, json: async () => answer };
        return { ok: false, status: 404, json: async () => ({}) };
    };
    publishSystemUpdateAuthentication(true);
    let renderer;
    await act(async () => {
        renderer = TestRenderer.create(React.createElement(SystemUpdateOperationProvider, null, React.createElement('main', null, 'page'), React.createElement(Probe)));
    });
    await settle();
    return { renderer, reads };
}
const dialogOf = renderer => renderer.root.find(node => node.props?.role === 'dialog' && node.props?.['aria-labelledby'] === 'system-update-operation-title');
const titleOf = renderer => textOf(renderer.root.find(node => node.props?.id === 'system-update-operation-title'));
const markOf = renderer => renderer.root.find(node => node.props?.['data-update-mark'] !== undefined);

for (const locale of ['en', 'tr']) {
    const c = { ...(locale === 'tr' ? tr : en), ...(locale === 'tr' ? trScreens : enScreens) };
    const fill = (key, vars) => String(c[key]).replace(/\{(\w+)\}/g, (all, name) => vars?.[name] ?? all);
    const versions = { target: 'v0.1.0-alpha.92', previous: 'v0.1.0-alpha.91' };

    test(`${locale}: a busy package manager is a calm stop with its own reason and when to start again`, async () => {
        const { renderer, reads } = await mountWith({ locale, message: BUSY, answer: observation({}) });
        try {
            assert.ok(reads.every(([, method]) => method === 'GET'), 'reading never starts anything');
            assert.equal(titleOf(renderer), c['panelUpdate.outcome.stoppedTitle']);
            assert.equal(markOf(renderer).props['data-update-mark'], 'neutral');
            const text = textOf(dialogOf(renderer));
            assert.ok(text.includes(fill('panelUpdate.outcome.stopped', versions)), text);
            assert.ok(text.includes(c['panelUpdate.outcome.packageManagerBusy']), text);
            assert.ok(text.includes(fill('panelUpdate.outcome.packageManagerBusyNext', versions)), text);
            for (const key of ['panelUpdate.outcome.preflightStep.generic', 'recovery.phase.failed', 'panelUpdate.failed']) {
                assert.ok(!text.includes(c[key]), `${key} must not be said for a busy package manager`);
            }
            assert.ok(!text.includes('CELIKPANEL_UPDATE_FAILURE'));
        } finally { await act(async () => renderer.unmount()); }
    });

    test(`${locale}: a recovery tool that could not be started or verified has its words and the server's line in view`, async () => {
        const { renderer } = await mountWith({ locale, message: COMPATIBILITY, answer: observation({ failure_code: 'recovery_runtime_preflight_failed' }) });
        try {
            const text = textOf(dialogOf(renderer));
            assert.ok(text.includes(c['panelUpdate.outcome.preflightStep.compatibility']), text);
            assert.ok(!text.includes(c['panelUpdate.outcome.preflightStep.generic']), 'not the generic reason');
            assert.ok(text.includes(fill('panelUpdate.outcome.serverMessage', { message: 'recovery launcher metadata is not trusted: /usr/libexec/celikpanel/recovery mode 0775, want 0755' })), text);
        } finally { await act(async () => renderer.unmount()); }
    });

    test(`${locale}: a refused-check step waits for its words, never the generic reason, until the screen copy fails`, async () => {
        const answer = observation({ failure_code: 'update_preflight_refused' });
        let mounted = await mountWith({ locale, message: REFUSED, answer, screensReady: false });
        try {
            const text = textOf(dialogOf(mounted.renderer));
            assert.ok(text.includes(c['panelUpdate.outcome.reasonLoading']), text);
            assert.ok(!text.includes(c['panelUpdate.outcome.preflightStep.generic']), 'the generic reason was shown while the words were loading');
            assert.ok(!text.includes('panelUpdate.outcome.preflightStep.agent_idle'), 'a key rendered as its name');
        } finally { await act(async () => mounted.renderer.unmount()); }
        mounted = await mountWith({ locale, message: REFUSED, answer });
        try {
            const text = textOf(dialogOf(mounted.renderer));
            assert.ok(text.includes(c['panelUpdate.outcome.preflightStep.agent_idle']), text);
            assert.ok(!text.includes(c['panelUpdate.outcome.reasonLoading']));
        } finally { await act(async () => mounted.renderer.unmount()); }
        mounted = await mountWith({ locale, message: REFUSED, answer, screensReady: false, screensFailed: true });
        try {
            const text = textOf(dialogOf(mounted.renderer));
            assert.ok(text.includes(c['panelUpdate.outcome.preflightStep.generic']), 'the boot-catalogue reason when the screen copy failed');
        } finally { await act(async () => mounted.renderer.unmount()); }
    });
}

test('the new update texts are in the boot catalogue in both languages, Turkish not English', () => {
    for (const key of ['panelUpdate.outcome.preflightStep.compatibility', 'panelUpdate.outcome.packageManagerBusy',
        'panelUpdate.outcome.packageManagerBusyNext', 'panelUpdate.outcome.reasonLoading']) {
        assert.equal(typeof en[key], 'string', key);
        assert.equal(typeof tr[key], 'string', key);
        assert.notEqual(tr[key], en[key], key);
    }
    assert.match(en['panelUpdate.outcome.packageManagerBusy'], /package manager was busy, for example with automatic security updates\. Nothing was changed\./);
    assert.match(tr['panelUpdate.outcome.packageManagerBusy'], /paket yöneticisi meşguldü/);
});
