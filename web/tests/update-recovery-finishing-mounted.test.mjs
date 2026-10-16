// Frankfurt 2026-10-16 (alpha.89 -> alpha.90): the update's last step, turning
// automatic certificate renewal back on, did not confirm on the first try; the
// page showed a red cross with the raw updater line, and when the server's own
// recovery finished the update 16 s later it showed an old-style "Update
// verified" notice with a close button, a failure acknowledgement and no
// "Updated to" card. The real update provider is mounted here with in-memory
// stand-ins for the browser's storage, locks and router only:
//   - recovering (the step is repeated): calm "Finishing the update", four
//     steps done and the confirmation "Repeating automatically (attempt 1 of
//     3)", no red mark, no close button, the server's line behind "Details";
//   - recovering -> succeeded: "Updated to {version}", the build line copied as
//     it reads, five steps done, one quiet line, a Reload button; the browser's
//     record becomes a succeeded update so the update card keeps "Updated to";
//   - a real recovery_required (paused): red, with the close button.
// Each fails against the previous source (red cross and close button while
// recovering; no "Updated to" for a recovered update).
//
// Kurtarmanin kendisinin tamamladigi adim sakin gosterilir; kirmizi yalniz
// sahibin islem yapmasi gereken durumdadir.
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
export function useI18n() { const c = globalThis.__finishingCatalogue; return { t: (key, vars) => fill(c.strings[key] ?? key, vars), locale: c.locale, screensReady: true }; }`);
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
const RAW = 'reviewed updater failed: exit status 1: Certbot renewal scheduler state could not be restored';
const failedRecord = () => ({ state_version: 1, phase: 'terminal', marker, outcome: 'failed', message: RAW, reload_scheduled: false, completed_at: Date.now() - 5_000 });
const observation = (fields) => ({
    schema: 'celikpanel-recovery-status/v1', request_id: REQUEST, panel_state: 'ready', observation: 'known',
    observed_at: '2026-10-16T15:00:30Z', ...fields,
});
const RECOVERING = observation({ phase: 'recovering', reason: 'recovery_running', terminal_proof: 'none', previous_failure: 'update_failed', failure_code: 'update_finishing_scheduler' });
const SUCCEEDED = observation({ phase: 'succeeded', reason: 'update_verified', terminal_proof: 'update_verified', previous_failure: 'update_failed', failure_code: 'update_finishing_scheduler' });
const PAUSED = observation({ phase: 'recovery_required', reason: 'recovery_incomplete', terminal_proof: 'none', previous_failure: 'recovery_failed', automatic_recovery: 'paused_retry_limit', first_failure_code: 'update_finishing_scheduler' });

function textOf(node) {
    if (node === null || node === undefined || typeof node === 'boolean') return '';
    if (typeof node === 'string' || typeof node === 'number') return String(node);
    if (Array.isArray(node)) return node.map(textOf).join('');
    return textOf(node.children);
}
// What a copy of the node gives: user-select: none content is not copied.
function copiedText(node) {
    if (node === null || node === undefined || typeof node === 'boolean') return '';
    if (typeof node === 'string' || typeof node === 'number') return String(node);
    if (Array.isArray(node)) return node.map(copiedText).join('');
    if (String(node.props?.className ?? '').split(/\s+/).includes('select-none')) return '';
    return copiedText(node.children);
}
const settle = async (rounds = 12) => { for (let i = 0; i < rounds; i += 1) await act(async () => { await new Promise(resolve => setTimeout(resolve, 5)); }); };

async function mountWith(locale, answer) {
    globalThis.__finishingCatalogue = { locale, strings: locale === 'tr' ? tr : en };
    localStorage.clear();
    sessionStorage.clear();
    replaced.length = 0;
    const record = failedRecord();
    localStorage.setItem(MARKER_KEY, JSON.stringify(record));
    canonical.snapshot = { canonical_version: 1, revision: 1, writer_id: 'b'.repeat(32), record };
    const reads = [];
    globalThis.fetch = async (url, options) => {
        reads.push([String(url), options?.method ?? 'GET']);
        if (String(url) === `/api/v1/recovery/status?request_id=${REQUEST}`) {
            return { ok: true, status: 200, json: async () => answer };
        }
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
const buttonsOf = renderer => renderer.root.findAll(node => node.type === 'button').map(node => textOf(node));

for (const locale of ['en', 'tr']) {
    const c = locale === 'tr' ? tr : en;
    const fill = (key, vars) => String(c[key]).replace(/\{(\w+)\}/g, (all, name) => vars?.[name] ?? all);

    test(`${locale}: while recovery repeats the renewal step the page says "Finishing the update", calm, with the steps`, async () => {
        const { renderer, reads } = await mountWith(locale, RECOVERING);
        try {
            assert.ok(reads.some(([url]) => url.startsWith('/api/v1/recovery/status')), 'the recovery record was read');
            assert.ok(reads.every(([, method]) => method === 'GET'), 'reading never starts anything');
            assert.equal(titleOf(renderer), c['panelUpdate.finishing.title']);
            const dialog = dialogOf(renderer);
            assert.ok(textOf(dialog).includes(fill('panelUpdate.finishing.body', { version: 'v0.1.0-alpha.92' })));
            const mark = markOf(renderer);
            assert.equal(mark.props['data-update-mark'], 'progress');
            assert.equal(mark.findAll(node => node.props?.['data-icon'] === 'XCircle').length, 0, 'no red cross');
            assert.ok(!String(mark.props.className).includes('danger'));
            const steps = dialog.findAll(node => node.props?.['data-step-state'] !== undefined);
            assert.deepEqual(steps.map(node => `${node.props['data-update-step']}=${node.props['data-step-state']}`),
                ['download=done', 'verify=done', 'prepare=done', 'restart=done', 'confirm=retrying']);
            const note = dialog.find(node => node.props?.['data-update-retry-note'] !== undefined);
            assert.equal(textOf(note), fill('panelUpdate.steps.retrying', { attempt: '1', max: '3' }));
            // The card reads the same calm steps and no "Updated to" yet.
            assert.equal(globalThis.__context.steps.steps[4].state, 'retrying');
            assert.deepEqual(globalThis.__context.steps.steps[4].attempt, { current: 1, max: 3 });
            assert.equal(globalThis.__context.updated, null);
            // Nothing to close and no failure sentence while the server handles it.
            assert.ok(!buttonsOf(renderer).includes(c['panelUpdate.dismissFailure']));
            assert.ok(!textOf(dialog).includes(c['panelUpdate.failureAcknowledgement']));
            assert.ok(textOf(dialog).includes(c['panelUpdate.steps.locked']));
            // The server's raw line is behind "Details", never on the page itself.
            const details = dialog.find(node => node.props?.['data-update-details'] !== undefined);
            assert.equal(details.type, 'details');
            assert.ok(textOf(details).includes('Certbot renewal scheduler state could not be restored'));
            assert.equal(textOf(dialog).split('Certbot renewal scheduler state could not be restored').length, 2, 'the raw line appears only once, in Details');
        } finally { await act(async () => renderer.unmount()); }
    });

    test(`${locale}: a recovery that verified the update is "Updated to" with the steps, the build and a Reload button`, async () => {
        const { renderer } = await mountWith(locale, SUCCEEDED);
        try {
            assert.equal(titleOf(renderer), fill('panelUpdate.steps.updated', { version: 'v0.1.0-alpha.92' }));
            const dialog = dialogOf(renderer);
            assert.equal(markOf(renderer).props['data-update-mark'], 'success');
            const steps = dialog.findAll(node => node.props?.['data-step-state'] !== undefined);
            assert.equal(steps.length, 5);
            assert.ok(steps.every(node => node.props['data-step-state'] === 'done'));
            const build = dialog.find(node => node.props?.['data-update-build'] !== undefined);
            assert.equal(copiedText(build), 'build 1743ac69edb0', 'the build line copies as it reads');
            assert.ok(textOf(build).includes(TARGET_COMMIT), 'screen readers still get the full commit');
            assert.ok(String(build.props.title).includes(TARGET_COMMIT));
            assert.equal(textOf(dialog.find(node => node.props?.['data-update-repeated'] !== undefined)), c['panelUpdate.steps.repeated']);
            // The update card gets "Updated to" with the quiet line (owner, 2026-10-16).
            assert.deepEqual(globalThis.__context.updated, { version: 'v0.1.0-alpha.92', commit: TARGET_COMMIT, repeated: true });
            assert.equal(globalThis.__context.steps.completed, true);
            assert.ok(!buttonsOf(renderer).includes(c['panelUpdate.dismissFailure']), 'no failure close button');
            assert.ok(!textOf(dialog).includes(c['panelUpdate.failureAcknowledgement']));
            assert.ok(!textOf(dialog).includes('could not be restored'), 'no failure line for a verified update');
            // The browser's record is now the update's success, as a normal one.
            const stored = JSON.parse(localStorage.getItem(MARKER_KEY));
            assert.equal(stored.phase, 'terminal');
            assert.equal(stored.outcome, 'succeeded');
            assert.equal(stored.recovered, true);
            assert.equal(stored.required_reload.request_id, REQUEST);
            const reload = renderer.root.find(node => node.type === 'button' && node.props?.['data-update-reload'] !== undefined);
            assert.equal(textOf(reload), c['app.reload']);
            await act(async () => reload.props.onClick());
            assert.equal(replaced.length, 1);
            assert.ok(replaced[0].includes(`_cp_update=${REQUEST}`), replaced[0]);
        } finally { await act(async () => renderer.unmount()); }
    });

    test(`${locale}: control: a paused recovery (the owner must act) stays red with the close button`, async () => {
        const { renderer } = await mountWith(locale, PAUSED);
        try {
            assert.equal(markOf(renderer).props['data-update-mark'], 'danger');
            assert.equal(markOf(renderer).findAll(node => node.props?.['data-icon'] === 'XCircle').length, 1);
            assert.ok(buttonsOf(renderer).includes(c['panelUpdate.dismissFailure']));
            assert.equal(titleOf(renderer), c['recovery.automatic.pausedTitle']);
            assert.equal(dialogOf(renderer).findAll(node => node.props?.['data-step-state'] !== undefined).length, 0, 'no calm steps');
            assert.equal(JSON.parse(localStorage.getItem(MARKER_KEY)).outcome, 'failed', 'the failure is kept');
        } finally { await act(async () => renderer.unmount()); }
    });
}

test('every new string exists in English and Turkish and the Turkish is not the English', () => {
    for (const key of ['panelUpdate.finishing.title', 'panelUpdate.finishing.body', 'panelUpdate.steps.retrying',
        'panelUpdate.steps.retryingPlain', 'panelUpdate.steps.repeated', 'panelUpdate.steps.state.retrying',
        'panelUpdate.details', 'panelUpdate.outcome.checkingTitle', 'common.fullCommit',
        'recovery.failure.update_finishing_scheduler.pending', 'recovery.automatic.cause.update_finishing_scheduler',
        'recovery.reason.update_finishing_scheduler', 'panelUpdate.outcome.cause.update_finishing_scheduler']) {
        assert.equal(typeof en[key], 'string', key);
        assert.equal(typeof tr[key], 'string', key);
        assert.notEqual(tr[key], en[key], key);
    }
    assert.equal(en['panelUpdate.finishing.body'], '{version} is installed and running. One final step, turning automatic certificate renewal back on, did not confirm on the first try. The server is repeating it by itself. Nothing needs to be done; this page updates by itself.');
});

test('the sidebar build stamp uses the same copy-clean build line', () => {
    const layout = readFileSync(new URL('../src/components/Layout.tsx', import.meta.url), 'utf8');
    assert.match(layout, /<BuildCommit commit=\{v\.commit\} \/>/);
    assert.doesNotMatch(layout, /<span className="sr-only">\{v\.commit\}<\/span>/);
    for (const file of ['PanelUpdateCard.tsx', 'SystemUpdateOperation.tsx']) {
        const source = readFileSync(new URL(`../src/components/${file}`, import.meta.url), 'utf8');
        assert.doesNotMatch(source, /<span className="sr-only">\{updated\.commit\}<\/span>/, file);
        assert.match(source, /<BuildCommit commit=\{updated\.commit\} \/>/, file);
    }
});
