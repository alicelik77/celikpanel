import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import ts from 'typescript';

// The panel update card while the server is busy (owner rule 2026-10-14,
// audit row 25). The card is mounted with the real readiness reader
// (panelUpdateAdmission.ts); only fetch, the timers and the tab's visibility
// are scripted. What is held:
//
//   - while the card says the server is busy it reads readiness again every
//     5 s by itself, with GET only, and says so under the busy text;
//   - a quiet read does not redraw the card into its "checking" look, and Retry
//     stays; a read that says ready returns the card to its normal state;
//   - the reads stop while the tab is hidden and when the card leaves the busy
//     state; nothing here starts or admits an update.
//
// Sunucu mesgulken guncelleme karti hazirligi 5 sn'de bir yalniz GET ile kendisi
// okur; gizli sekmede ve mesgul durumdan cikinca durur; guncelleme baslatmaz.
const require = createRequire(import.meta.url);
const moduleURL = source => 'data:text/javascript;base64,' + Buffer.from(source).toString('base64');
function compileURL(relativePath, replacements = []) {
    let compiled = ts.transpileModule(readFileSync(new URL(relativePath, import.meta.url), 'utf8'), {
        compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020, jsx: ts.JsxEmit.ReactJSX },
    }).outputText;
    for (const [pattern, replacement] of replacements) compiled = compiled.replace(pattern, replacement);
    return moduleURL(compiled);
}
const reactURL = pathToFileURL(require.resolve('react')).href;
const jsxRuntimeURL = pathToFileURL(require.resolve('react/jsx-runtime')).href;
const iconsURL = moduleURL(`import React from '${reactURL}';
    const Icon = (name) => (props) => React.createElement('i', { ...props, 'data-icon': name });
    export const AlertTriangle = Icon('alert'); export const DownloadCloud = Icon('download');
    export const Loader2 = Icon('spinner'); export const RefreshCw = Icon('refresh');`);
const uiURL = moduleURL(`import React from '${reactURL}';
    export function Button({ children, ...props }) { return React.createElement('button', props, children); }`);
const i18nURL = moduleURL(`export function useI18n() { return { t: (key, params) => params ? key + ' ' + JSON.stringify(params) : key, locale: 'en' }; }`);
const apiErrorURL = moduleURL(`export async function readApiError() { return {}; } export function apiErrorText() { return 'api-error'; }`);
const operationURL = moduleURL(`
    export function createSystemUpdateRequestID() { return 'mounted-request-id'; }
    export function systemUpdateResponseHint() { return 'response-error'; }
    export function useSystemUpdateOperation() { return globalThis.__pollOperation; }
    export function validUpdateTarget() { return true; }
    export function validUpdateVersion() { return true; }`);
const admissionURL = compileURL('../src/lib/panelUpdateAdmission.ts');
const recoveryObservationURL = compileURL('../src/lib/recoveryObservation.ts');
const systemUpdateFailureURL = compileURL('../src/lib/systemUpdateFailure.ts');
const outcomeURL = compileURL('../src/lib/systemUpdateOutcome.ts', [
    [/from ['"]\.\/recoveryObservation['"]/g, `from '${recoveryObservationURL}'`],
    [/from ['"]\.\/systemUpdateFailure['"]/g, `from '${systemUpdateFailureURL}'`],
]);
const panelURL = compileURL('../src/components/PanelUpdateCard.tsx', [
    [/from ['"]\.\.\/lib\/systemUpdateOutcome['"]/g, `from '${outcomeURL}'`],
    [/from ['"]react['"]/g, `from '${reactURL}'`],
    [/from ['"]react\/jsx-runtime['"]/g, `from '${jsxRuntimeURL}'`],
    [/from ['"]lucide-react['"]/g, `from '${iconsURL}'`],
    [/from ['"]\.\/ui['"]/g, `from '${uiURL}'`],
    [/from ['"]\.\.\/i18n['"]/g, `from '${i18nURL}'`],
    [/from ['"]\.\.\/lib\/apiError['"]/g, `from '${apiErrorURL}'`],
    [/from ['"]\.\.\/lib\/panelUpdateAdmission['"]/g, `from '${admissionURL}'`],
    [/from ['"]\.\/SystemUpdateOperation['"]/g, `from '${operationURL}'`],
    // The step bars (2026-10-16) are drawn only from the context's steps,
    // which this harness does not provide; tests/update-steps-mounted.test.mjs mounts them.
    [/from ['"]\.\/SystemUpdateSteps['"]/g, `from 'data:text/javascript,export function SystemUpdateSteps() { return null; } export function systemUpdateStepsAnnouncement() { return ""; }'`],
]);

let timers = [];
globalThis.document = Object.assign(new EventTarget(), { visibilityState: 'visible' });
globalThis.window = {
    setTimeout: (fn, ms) => { const timer = { fn, ms, live: true }; timers.push(timer); return timer; },
    clearTimeout: timer => { if (timer && typeof timer === 'object') timer.live = false; },
};
const { PanelUpdateCard, PANEL_UPDATE_READINESS_POLL_MS } = await import(panelURL);

const updateCheck = {
    supported: true, available: true, current_version: 'v0.1.0-alpha.84', current_commit: 'a'.repeat(40),
    target: { version: 'v0.1.0-alpha.85', commit: 'b'.repeat(40), sequence: '85', os: 'linux', arch: 'amd64', archive_sha256: 'c'.repeat(64), archive_size: '22500000' },
};
const BUSY = { ready: false, code: 'HOST_MUTATION_BUSY', reason: 'package_manager_active' };
const READY = { ready: true };
const READINESS = '/api/v1/host-mutation-readiness';

let calls, readiness, starts, renderer;
const settle = async () => { for (let round = 0; round < 6; round++) await act(async () => { await new Promise(resolve => setImmediate(resolve)); }); };
const pollTimers = () => timers.filter(timer => timer.live && timer.ms === PANEL_UPDATE_READINESS_POLL_MS);
const firePoll = async () => { const due = pollTimers(); for (const timer of due) timer.live = false; await act(async () => { for (const timer of due) timer.fn(); }); await settle(); return due.length; };
const reads = () => calls.filter(call => call.path === READINESS).length;
const page = () => JSON.stringify(renderer.toJSON());
const textIn = node => node.children.map(child => typeof child === 'string' ? child : textIn(child)).join('');
const buttonWith = text => renderer.root.findAllByType('button').find(node => textIn(node).includes(text)) ?? null;
const startButton = () => renderer.root.findByProps({ id: 'panel-update-start-button' });

async function mountBusy() {
    timers = []; calls = []; starts = 0; readiness = [() => Response.json(BUSY)];
    globalThis.document.visibilityState = 'visible';
    globalThis.__pollOperation = { active: false, start: async () => { starts += 1; return { kind: 'accepted' }; } };
    globalThis.fetch = async (input, init = {}) => {
        const path = String(input);
        calls.push({ path, method: init.method || 'GET' });
        if (path === '/api/v1/panel/version') return Response.json({ version: updateCheck.current_version, commit: updateCheck.current_commit });
        if (path === '/api/v1/panel/update/check') return Response.json(updateCheck);
        if (path === READINESS) return (readiness.length > 1 ? readiness.shift() : readiness[0])();
        throw new Error('unexpected fetch: ' + path);
    };
    await act(async () => { renderer = TestRenderer.create(React.createElement(PanelUpdateCard)); });
    await settle();
    await act(async () => { renderer.root.findAllByType('button')[0].props.onClick(); });
    await settle();
    assert.ok(page().includes("services.mutationReadiness.package_manager_active"), "the card says the server is busy");
}
async function unmount() { if (renderer) await act(async () => renderer.unmount()); renderer = null; }

test('while busy the card reads readiness again every 5 s by itself, quietly and with GET only, until a read says ready', async () => {
    const originalFetch = globalThis.fetch;
    try {
        await mountBusy();
        assert.ok(renderer.root.findAllByProps({ 'data-readiness-auto-resume': '' }).length === 1, 'the busy text says the page checks again by itself');
        assert.ok(page().includes('services.mutationReadiness.autoResume'));
        assert.ok(buttonWith('common.retry'), 'Retry stays');
        assert.equal(pollTimers().length, 1, 'one quiet read is scheduled');
        assert.equal(reads(), 1);

        // A quiet read in flight does not redraw the card into its checking look.
        let answer;
        readiness = [() => new Promise(resolve => { answer = resolve; })];
        assert.equal(await firePoll(), 1);
        assert.equal(reads(), 2);
        assert.ok(!page().includes('services.mutationReadiness.checking'), 'no checking look during a quiet read');
        assert.ok(page().includes('services.mutationReadiness.package_manager_active'), 'the busy text stays during a quiet read');
        assert.ok(buttonWith('common.retry'), 'Retry stays during a quiet read');
        assert.equal(pollTimers().length, 0, 'single-flight: nothing else is scheduled while a read is in flight');
        await act(async () => { answer(Response.json(BUSY)); });
        await settle();
        assert.ok(page().includes('services.mutationReadiness.package_manager_active'), 'still busy: the text stays');
        assert.equal(pollTimers().length, 1, 'the next read is scheduled after the answer');

        // A read that could not verify keeps what is shown.
        readiness = [() => Promise.reject(new TypeError('connection refused'))];
        assert.equal(await firePoll(), 1);
        assert.ok(page().includes('services.mutationReadiness.package_manager_active'));
        assert.ok(!page().includes('services.mutationReadiness.state_unverified'));

        readiness = [() => Response.json(READY)];
        assert.equal(await firePoll(), 1);
        assert.equal(reads(), 4);
        assert.ok(!page().includes('services.mutationReadiness.package_manager_active'), 'ready ends the busy text');
        assert.ok(!page().includes('services.mutationReadiness.autoResume'));
        assert.equal(startButton().props.disabled, false, 'the card is back in its normal state');
        assert.equal(pollTimers().length, 0, 'no read after ready');
        assert.equal(await firePoll(), 0);

        assert.ok(calls.every(call => call.method === 'GET'), 'only GET requests');
        assert.deepEqual([...new Set(calls.map(call => call.path))].sort(), ['/api/v1/host-mutation-readiness', '/api/v1/panel/update/check', '/api/v1/panel/version']);
        assert.equal(starts, 0, 'no update was started or admitted');
    } finally { await unmount(); globalThis.fetch = originalFetch; }
});

test('the quiet reads stop while the tab is hidden and resume when it is shown', async () => {
    const originalFetch = globalThis.fetch;
    try {
        await mountBusy();
        assert.equal(pollTimers().length, 1);
        globalThis.document.visibilityState = 'hidden';
        await act(async () => { globalThis.document.dispatchEvent(new Event('visibilitychange')); });
        assert.equal(pollTimers().length, 0, 'a hidden tab schedules nothing');
        assert.equal(await firePoll(), 0);
        assert.equal(reads(), 1, 'no read while hidden');
        globalThis.document.visibilityState = 'visible';
        await act(async () => { globalThis.document.dispatchEvent(new Event('visibilitychange')); });
        // Shown again: one read at once, not after the delay (2026-10-15).
        assert.equal(reads(), 2, 'shown again: the card reads at once');
        assert.equal(pollTimers().length, 1, 'then the next read is scheduled');
        assert.equal(await firePoll(), 1);
        assert.equal(reads(), 3);
        assert.ok(calls.every(call => call.method === 'GET'));
        assert.equal(starts, 0);
    } finally { await unmount(); globalThis.fetch = originalFetch; }
});

test('the quiet reads stop when the card leaves the busy state and when it unmounts', async () => {
    const originalFetch = globalThis.fetch;
    try {
        await mountBusy();
        assert.equal(pollTimers().length, 1);
        // The owner's Retry owns its read and shows the checking look; the quiet read stops.
        let answer;
        readiness = [() => new Promise(resolve => { answer = resolve; })];
        await act(async () => { buttonWith('common.retry').props.onClick(); });
        assert.ok(page().includes('services.mutationReadiness.checking'), 'the owner\'s Retry shows the checking look');
        assert.equal(pollTimers().length, 0, 'no quiet read while Retry reads');
        await act(async () => { answer(Response.json(READY)); });
        await settle();
        assert.equal(startButton().props.disabled, false);
        assert.equal(pollTimers().length, 0, 'not busy: nothing is scheduled');
        assert.equal(await firePoll(), 0);
        assert.equal(reads(), 2);
        await unmount();

        await mountBusy();
        assert.equal(pollTimers().length, 1);
        await unmount();
        assert.equal(pollTimers().length, 0, 'unmount clears the read');
        assert.equal(starts, 0);
    } finally { await unmount(); globalThis.fetch = originalFetch; }
});
