// One bar per step of the update (owner design 2026-10-16, after updating the
// second installed server from alpha.85 to alpha.87 and seeing the old
// "connection interrupted, it may be updating" warning and a corner notice
// while nothing was wrong). The server declares the steps
// (cmd/panel system_update_plan.go); the page draws done steps with a check,
// the current one with a bar, the next ones quiet; the restart is a planned
// step with a time-based bar and one calm sentence; the only alarm is the
// honest one after the 6-minute window. Every test below fails against the
// previous source (the modules and the card's steps did not exist).
//
// Guncellemenin her adimi icin bir cubuk; yeniden baslatma planli bir adimdir;
// tek uyari 6 dakikalik pencereden sonraki durust uyaridir.
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
import { enServerScreens } from '../src/i18n/screens/server/en.ts';
import { trServerScreens } from '../src/i18n/screens/server/tr.ts';
import { panelUpdateDiscoveryStubSource } from './panelUpdateDiscoveryStub.mjs';
import {
    SYSTEM_UPDATE_DEFAULT_RESTART_SECONDS,
    SYSTEM_UPDATE_STEP_IDS,
    admittedSystemUpdatePlan,
    decodeSystemUpdatePlan,
    systemUpdateCurrentStep,
    systemUpdateRestartProgress,
    systemUpdateStepsView,
} from '../src/lib/systemUpdateSteps.ts';
import {
    SYSTEM_UPDATE_READ_MEMORY,
    systemUpdateCornerNoticeRepeats,
    systemUpdateDisplayedPhase,
    systemUpdateFollowRead,
} from '../src/lib/systemUpdateTracking.ts';

globalThis.document ??= Object.assign(new EventTarget(), { visibilityState: 'visible' });
globalThis.window ??= { setTimeout: () => 0, clearTimeout: () => {} };

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
const catalogues = { en: { ...en, ...enServerScreens }, tr: { ...tr, ...trServerScreens } };
const fill = (text, vars) => String(text).replace(/\{(\w+)\}/g, (all, name) => vars?.[name] ?? all);
const iconsURL = moduleURL(`import React from '${reactURL}';
const icon = name => props => React.createElement('i', { ...props, 'data-icon': name });
export const AlertTriangle = icon('AlertTriangle'), CheckCircle2 = icon('CheckCircle2'), DownloadCloud = icon('DownloadCloud'), Loader2 = icon('Loader2'), RefreshCw = icon('RefreshCw'), Info = icon('Info');`);
const uiURL = moduleURL(`import React from '${reactURL}'; export function Button({ children, ...props }) { return React.createElement('button', props, children); }`);
const i18nURL = moduleURL(`const fill = (text, vars) => String(text).replace(/\\{(\\w+)\\}/g, (all, name) => vars && vars[name] !== undefined ? String(vars[name]) : all);
export function useI18n() { const c = globalThis.__stepsCatalogue; return { t: (key, vars) => fill(c.strings[key] ?? key, vars), locale: c.locale }; }`);
const apiErrorURL = moduleURL(`export async function readApiError() { return {}; } export function apiErrorText() { return 'api-error'; }`);
const operationURL = moduleURL(`
export function createSystemUpdateRequestID() { return 'mounted-request-id'; }
export function systemUpdateResponseHint() { return 'response-error'; }
export function useSystemUpdateOperation() { return globalThis.__stepsOperation; }
export function validUpdateTarget() { return true; }
export function validUpdateVersion() { return true; }`);
const admissionURL = moduleURL(`
export function unverifiedHostMutationReadiness() { return { ready: false, code: 'HOST_MUTATION_UNAVAILABLE', reason: 'state_unverified' }; }
export function fetchHostMutationReadiness() { return Promise.resolve({ ready: true }); }
export async function runHostMutationAdmission(read, onReady) { const r = await read(); if (r.ready) await onReady(); return r; }`);
const outcomeURL = compileURL('../src/lib/systemUpdateOutcome.ts', [
    [/from ['"]\.\/recoveryObservation['"]/g, `from '${compileURL('../src/lib/recoveryObservation.ts')}'`],
    [/from ['"]\.\/systemUpdateFailure['"]/g, `from '${compileURL('../src/lib/systemUpdateFailure.ts')}'`],
]);
const stepsComponentURL = compileURL('../src/components/SystemUpdateSteps.tsx', [
    [/from ['"]react\/jsx-runtime['"]/g, `from '${jsxRuntimeURL}'`],
    [/from ['"]lucide-react['"]/g, `from '${iconsURL}'`],
    [/from ['"]\.\.\/i18n['"]/g, `from '${i18nURL}'`],
]);
const { SystemUpdateSteps, systemUpdateStepsAnnouncement } = await import(stepsComponentURL);
const { PanelUpdateCard } = await import(compileURL('../src/components/PanelUpdateCard.tsx', [
    [/from ['"]\.\.\/lib\/systemUpdateOutcome['"]/g, `from '${outcomeURL}'`],
    [/from ['"]react['"]/g, `from '${reactURL}'`],
    [/from ['"]react\/jsx-runtime['"]/g, `from '${jsxRuntimeURL}'`],
    [/from ['"]lucide-react['"]/g, `from '${iconsURL}'`],
    [/from ['"]\.\.\/lib\/panelUpdateDiscovery['"]/g, `from '${moduleURL(panelUpdateDiscoveryStubSource())}'`],
    [/from ['"]\.\/ui['"]/g, `from '${uiURL}'`],
    [/from ['"]\.\.\/i18n['"]/g, `from '${i18nURL}'`],
    [/from ['"]\.\.\/lib\/apiError['"]/g, `from '${apiErrorURL}'`],
    [/from ['"]\.\.\/lib\/panelUpdateAdmission['"]/g, `from '${admissionURL}'`],
    [/from ['"]\.\/SystemUpdateOperation['"]/g, `from '${operationURL}'`],
    [/from ['"]\.\/SystemUpdateSteps['"]/g, `from '${stepsComponentURL}'`],
    [/from ['"]\.\/BuildCommit['"]/g, `from '${compileURL('../src/components/BuildCommit.tsx', [[/from ['"]react\/jsx-runtime['"]/g, `from '${jsxRuntimeURL}'`], [/from ['"]\.\.\/i18n['"]/g, `from '${i18nURL}'`]])}'`],
]));

const flush = async () => { for (let i = 0; i < 4; i += 1) await Promise.resolve(); };
function textOf(node) {
    if (node === null || node === undefined || typeof node === 'boolean') return '';
    if (typeof node === 'string' || typeof node === 'number') return String(node);
    if (Array.isArray(node)) return node.map(textOf).join(' ');
    return textOf(node.children);
}
const ALARM = /could not|not known|interrupted|okunamad|bilinmiyor|kesildi|kesintiye/i;
const REQUEST = 'f142f0a883f393df25bda7927dfb2c70';
const COMMIT = '0123456789abcdef0123456789abcdef01234567';
const START = Date.UTC(2026, 9, 16, 9, 0, 0);
const at = seconds => START + seconds * 1000;

const serverPlan = (states, extra = {}) => ({
    steps: SYSTEM_UPDATE_STEP_IDS.map((id, index) => ({ id, state: states[index], ...(extra[id] ?? {}) })),
    expected_restart_seconds: extra.expected ?? 120,
    restart_bound_seconds: 360,
});
const view = (input) => systemUpdateStepsView({ plan: null, phase: 'applying', overdue: false, lostSince: null, now: at(60), succeeded: false, ...input });
const states = v => v.steps.map(step => `${step.id}=${step.state}`);

test('the plan is decoded exactly: five known steps in order, a bounded progress and expected length', () => {
    const plan = decodeSystemUpdatePlan(serverPlan(['running', 'pending', 'pending', 'pending', 'pending'], { download: { progress: 0.5, elapsed_seconds: 30 } }));
    assert.deepEqual(plan.steps[0], { id: 'download', state: 'running', progress: 0.5 });
    assert.equal(plan.expectedRestartSeconds, 120);
    // Anything else keeps the page's previous wording.
    assert.equal(decodeSystemUpdatePlan(null), null);
    assert.equal(decodeSystemUpdatePlan({ steps: [] }), null);
    const reordered = serverPlan(['done', 'done', 'running', 'pending', 'pending']);
    [reordered.steps[0], reordered.steps[1]] = [reordered.steps[1], reordered.steps[0]];
    assert.equal(decodeSystemUpdatePlan(reordered), null);
    const unknownState = serverPlan(['done', 'skipped', 'running', 'pending', 'pending']);
    assert.equal(decodeSystemUpdatePlan(unknownState), null);
    const extra = serverPlan(['done', 'done', 'running', 'pending', 'pending']);
    extra.steps.push({ id: 'reboot', state: 'pending' });
    assert.equal(decodeSystemUpdatePlan(extra), null);
    // A progress out of range is dropped; an expected length out of range is the default.
    const odd = decodeSystemUpdatePlan(serverPlan(['running', 'pending', 'pending', 'pending', 'pending'], { download: { progress: 1.5 }, expected: 9999 }));
    assert.equal(odd.steps[0].progress, undefined);
    assert.equal(odd.expectedRestartSeconds, SYSTEM_UPDATE_DEFAULT_RESTART_SECONDS);
});

test('the start answer declares the plan; an unreadable body only means it comes with the first read', async () => {
    const admitted = await admittedSystemUpdatePlan({ json: async () => ({ accepted: true, plan: serverPlan(['running', 'pending', 'pending', 'pending', 'pending']) }) });
    assert.equal(admitted.steps[0].state, 'running');
    assert.equal(await admittedSystemUpdatePlan({ json: async () => { throw new Error('body'); } }), null);
    assert.equal(await admittedSystemUpdatePlan({ json: async () => ({ accepted: true }) }), null, 'an older server: no plan');
});

test('the restart bar is time-based, reaches 90 % at the expected length and never the end by itself', () => {
    assert.equal(systemUpdateRestartProgress(0, 90), 0);
    assert.equal(systemUpdateRestartProgress(45_000, 90), 0.45);
    assert.ok(Math.abs(systemUpdateRestartProgress(90_000, 90) - 0.9) < 1e-9);
    let previous = 0;
    for (let seconds = 0; seconds <= 360; seconds += 15) {
        const value = systemUpdateRestartProgress(seconds * 1000, 90);
        assert.ok(value >= previous && value < 1, `${seconds} s -> ${value}`);
        previous = value;
    }
    assert.ok(systemUpdateRestartProgress(10 * 360_000, 90) <= 0.99);
    assert.ok(systemUpdateRestartProgress(60_000, Number.NaN) > 0, 'a missing length falls back to the default');
});

test('the steps follow the plan; the download shows its measured bytes, an unmeasured step slides', () => {
    const plan = decodeSystemUpdatePlan(serverPlan(['running', 'pending', 'pending', 'pending', 'pending'], { download: { progress: 0.42 } }));
    const downloading = view({ plan });
    assert.deepEqual(states(downloading), ['download=running', 'verify=pending', 'prepare=pending', 'restart=pending', 'confirm=pending']);
    assert.equal(systemUpdateCurrentStep(downloading).progress, 0.42);
    const verifying = view({ plan: decodeSystemUpdatePlan(serverPlan(['done', 'running', 'pending', 'pending', 'pending'])) });
    assert.equal(systemUpdateCurrentStep(verifying).id, 'verify');
    assert.equal(systemUpdateCurrentStep(verifying).progress, null, 'indeterminate');
    // Negative control: without a plan, an applying read draws no steps (the
    // previous wording stays), and an unknown read never draws calm steps.
    assert.equal(view({ plan: null }), null);
    assert.equal(view({ plan, phase: 'unknown' }), null);
    assert.equal(view({ plan, phase: 'reading' }), null);
    // A failure keeps its own wording, not steps.
    assert.equal(view({ plan: decodeSystemUpdatePlan(serverPlan(['done', 'done', 'done', 'done', 'failed'])) }), null);
});

test('a lost Panel after the update was active is the restart step, with time-based progress and nothing alarming', () => {
    const plan = decodeSystemUpdatePlan(serverPlan(['done', 'done', 'running', 'pending', 'pending'], { expected: 120 }));
    let memory = systemUpdateFollowRead(SYSTEM_UPDATE_READ_MEMORY, { operation: { status: 'running' }, disconnected: false, at: at(20) }, START).memory;
    // A refused connection, then PANEL_STARTING (a 503 counts as a lost read in pollExact).
    const lost = systemUpdateFollowRead(memory, { operation: null, disconnected: true, at: at(30) }, START);
    memory = lost.memory;
    const starting = systemUpdateFollowRead(memory, { operation: null, disconnected: true, at: at(90) }, START);
    assert.equal(starting.phase, 'restarting');
    assert.equal(starting.memory.lostSince, at(30));
    const steps = view({ plan, phase: systemUpdateDisplayedPhase(starting.phase, starting.restartDeadline, at(90)), lostSince: starting.memory.lostSince, now: at(90) });
    assert.deepEqual(states(steps), ['download=done', 'verify=done', 'prepare=done', 'restart=running', 'confirm=pending']);
    const restart = systemUpdateCurrentStep(steps);
    assert.equal(restart.restarting, true);
    assert.equal(restart.still, false);
    assert.ok(Math.abs(restart.progress - 0.45) < 1e-9, '60 s of 120 s expected');
    // The owner's real update: still no answer at T+02:59, still calm.
    const late = view({ plan, phase: systemUpdateDisplayedPhase(starting.phase, starting.restartDeadline, at(179)), lostSince: at(30), now: at(179) });
    assert.equal(systemUpdateCurrentStep(late).id, 'restart');
    assert.ok(systemUpdateCurrentStep(late).progress > restart.progress);
    // A page opened during the restart, nothing read yet: the same calm step.
    const cold = systemUpdateFollowRead(SYSTEM_UPDATE_READ_MEMORY, { operation: null, disconnected: true, at: at(100) }, START);
    const coldSteps = view({ plan: null, phase: cold.phase, lostSince: cold.memory.lostSince, now: at(110) });
    assert.equal(systemUpdateCurrentStep(coldSteps).id, 'restart');
});

test('past the window the restart bar stops and the overdue wording takes over; never before', () => {
    const plan = decodeSystemUpdatePlan(serverPlan(['done', 'done', 'running', 'pending', 'pending']));
    const memory = systemUpdateFollowRead(SYSTEM_UPDATE_READ_MEMORY, { operation: { status: 'running' }, disconnected: false, at: at(20) }, START).memory;
    const lost = systemUpdateFollowRead(memory, { operation: null, disconnected: true, at: at(30) }, START);
    assert.equal(systemUpdateDisplayedPhase(lost.phase, lost.restartDeadline, at(359)), 'restarting');
    const displayed = systemUpdateDisplayedPhase(lost.phase, lost.restartDeadline, at(361));
    assert.equal(displayed, 'unknown');
    const overdue = view({ plan, phase: displayed, overdue: true, lostSince: at(30), now: at(361) });
    assert.equal(systemUpdateCurrentStep(overdue).still, true);
    assert.equal(systemUpdateCurrentStep(overdue).id, 'restart');
});

test('reconnection: the new Panel answers, the confirmation runs, then every step is done', () => {
    const plan = decodeSystemUpdatePlan(serverPlan(['done', 'done', 'done', 'done', 'running']));
    const memory = systemUpdateFollowRead({ lastKnownActive: true, lostSince: at(30) }, { operation: { status: 'running', phase: 'verifying' }, disconnected: false, at: at(95) }, START);
    assert.equal(memory.phase, 'verifying');
    const confirming = view({ plan, phase: memory.phase, lostSince: null, now: at(95) });
    assert.deepEqual(states(confirming), ['download=done', 'verify=done', 'prepare=done', 'restart=done', 'confirm=running']);
    // Even with an older plan from before the outage, the new build answering proves the rest.
    const older = view({ plan: decodeSystemUpdatePlan(serverPlan(['done', 'done', 'running', 'pending', 'pending'])), phase: 'verifying' });
    assert.equal(systemUpdateCurrentStep(older).id, 'confirm');
    const done = view({ plan: null, phase: 'unknown', succeeded: true });
    assert.equal(done.completed, true);
    assert.ok(done.steps.every(step => step.state === 'done'));
});

test('no toast on the update page: the corner notice repeats neither the steps nor the success, a failure keeps it', () => {
    assert.equal(systemUpdateCornerNoticeRepeats(1, true), true);
    assert.equal(systemUpdateCornerNoticeRepeats(0, true), false, 'another page: the corner notice shows the compact step');
    const tracker = readFileSync(new URL('../src/components/SystemUpdateOperation.tsx', import.meta.url), 'utf8');
    assert.match(tracker, /systemUpdateCornerNoticeRepeats\(inlineNotices, tracking !== null \|\| updated !== null\)/);
    // Frankfurt 2026-10-16: a success the server's recovery verified is the success path too.
    assert.match(tracker, /const updatedMarker = \(!failureGuidance \|\| recoveredSuccess\) && terminalKind === 'succeeded' \? exactMarker : null;/);
    assert.match(tracker, /<SystemUpdateSteps view=\{compactSteps\} compact \/>/);
    // The plan is kept through lost reads and taken from every answered read.
    assert.match(tracker, /const readPlan = outcome\.operation\?\.plan;\n\s+if \(readPlan\) setPlanState\(\{ requestID: exactMarker\.request_id, plan: readPlan \}\);/);
    // The gates' grace and the lock still need the server's own word (unchanged).
    assert.match(tracker, /const restartRequestID = followed && followedMemory\?\.lastKnownActive \? followed\.request_id : null;/);
    // A 5xx (PANEL_STARTING, PANEL_UPDATE_RESTARTING) is a lost read, so it is the restart.
    assert.match(tracker, /disconnected: response\.status >= 500,/);
});

test('beside the step bars the dialog says the lock in one short sentence; without a plan the bounded detail stays', () => {
    // Owner 2026-10-16: the two-minute/six-minute lock sentence was the longest,
    // most technical text next to the calm bars.
    assert.equal(en['panelUpdate.steps.locked'], 'Other pages stay locked while the update runs.');
    assert.equal(tr['panelUpdate.steps.locked'], 'Güncelleme sürerken diğer sayfalar kilitli kalır.');
    assert.doesNotMatch(en['panelUpdate.steps.locked'], /minute|restart/i);
    const tracker = readFileSync(new URL('../src/components/SystemUpdateOperation.tsx', import.meta.url), 'utf8');
    // The close sentence belongs only to a failure the owner reads (Frankfurt 2026-10-16).
    assert.match(tracker, /const acknowledgementKey = realFailure \? 'panelUpdate\.failureAcknowledgement'\n\s+: stepsView \? 'panelUpdate\.steps\.locked' : 'panelUpdate\.interactionLocked';/);
    assert.match(tracker, /\{t\(acknowledgementKey\)\}/);
    // Control: the bounded wording is kept for a page without a plan.
    assert.match(en['panelUpdate.interactionLocked'], /after two minutes/);
});

for (const locale of ['en', 'tr']) {
    const c = catalogues[locale];

    test(`${locale}: the step list draws checks, one bar for the current step and quiet next steps`, async () => {
        globalThis.__stepsCatalogue = { locale, strings: catalogues[locale] };
        const steps = view({ plan: decodeSystemUpdatePlan(serverPlan(['done', 'done', 'pending', 'pending', 'pending'].map((s, i) => (i === 2 ? 'running' : s)))) });
        let renderer;
        await act(async () => { renderer = TestRenderer.create(React.createElement(SystemUpdateSteps, { view: steps })); });
        try {
            const items = renderer.root.findAll(node => node.props && node.props['data-update-step'] !== undefined);
            assert.deepEqual(items.map(item => item.props['data-step-state']), ['done', 'done', 'running', 'pending', 'pending']);
            assert.equal(items[0].findAll(node => node.props?.['data-icon'] === 'CheckCircle2').length, 1);
            const bars = renderer.root.findAll(node => node.props?.role === 'progressbar');
            assert.equal(bars.length, 1, 'one bar, the current step');
            assert.equal(bars[0].props['aria-label'], c['panelUpdate.steps.prepare']);
            assert.equal(bars[0].props['data-step-progress'], 'indeterminate');
            assert.equal(bars[0].props['aria-valuenow'], undefined);
            const text = textOf(renderer.toJSON());
            for (const key of ['download', 'verify', 'prepare', 'restart', 'confirm']) assert.ok(text.includes(c[`panelUpdate.steps.${key}`]));
            assert.doesNotMatch(text, ALARM);
            assert.ok(!text.includes(c['panelUpdate.steps.restarting']), 'the restart sentence only on the restart step');
        } finally { await act(async () => renderer.unmount()); }
    });

    test(`${locale}: the download prints its measured value; the restart has its one calm sentence and no number`, async () => {
        globalThis.__stepsCatalogue = { locale, strings: catalogues[locale] };
        const downloading = view({ plan: decodeSystemUpdatePlan(serverPlan(['running', 'pending', 'pending', 'pending', 'pending'], { download: { progress: 0.5 } })) });
        let renderer;
        await act(async () => { renderer = TestRenderer.create(React.createElement(SystemUpdateSteps, { view: downloading })); });
        const bar = renderer.root.find(node => node.props?.role === 'progressbar');
        assert.equal(bar.props['aria-valuenow'], 50);
        assert.equal(bar.props['data-step-progress'], 'measured');
        assert.ok(textOf(renderer.toJSON()).includes(locale === 'tr' ? '%50' : '50%'));
        await act(async () => renderer.unmount());

        const restarting = view({ plan: null, phase: 'restarting', lostSince: at(30), now: at(75) });
        await act(async () => { renderer = TestRenderer.create(React.createElement(SystemUpdateSteps, { view: restarting })); });
        try {
            const restartBar = renderer.root.find(node => node.props?.role === 'progressbar');
            assert.equal(restartBar.props['data-step-progress'], 'time');
            assert.equal(restartBar.props['aria-valuenow'], undefined, 'an estimate is not given a number');
            assert.doesNotMatch(String(restartBar.props.className), /update-step-still/);
            const note = renderer.root.find(node => node.props?.['data-update-restart-note'] !== undefined);
            assert.equal(textOf(note), c['panelUpdate.steps.restarting']);
            assert.equal(en['panelUpdate.steps.restarting'], 'The panel is restarting; this step can take a few minutes. This page reconnects by itself.');
            const text = textOf(renderer.toJSON());
            assert.doesNotMatch(text, ALARM);
            assert.doesNotMatch(text, /%/, 'no figure for the time-based bar');
            assert.equal(systemUpdateStepsAnnouncement(restarting, (key, vars) => fill(c[key], vars), 'v0.1.0-alpha.88'),
                `${c['panelUpdate.steps.restart']}. ${c['panelUpdate.steps.restarting']}`);
        } finally { await act(async () => renderer.unmount()); }
    });

    test(`${locale}: the compact corner notice shows only the current step`, async () => {
        globalThis.__stepsCatalogue = { locale, strings: catalogues[locale] };
        const restarting = view({ plan: null, phase: 'restarting', lostSince: at(30), now: at(75) });
        let renderer;
        await act(async () => { renderer = TestRenderer.create(React.createElement(SystemUpdateSteps, { view: restarting, compact: true })); });
        try {
            const items = renderer.root.findAll(node => node.props && node.props['data-update-step'] !== undefined);
            assert.deepEqual(items.map(item => item.props['data-update-step']), ['restart']);
        } finally { await act(async () => renderer.unmount()); }
    });

    async function mountCard(operation) {
        globalThis.__stepsCatalogue = { locale, strings: catalogues[locale] };
        globalThis.__stepsOperation = { start: async () => ({ kind: 'accepted' }), ...operation };
        const originalFetch = globalThis.fetch;
        globalThis.fetch = async (path) => {
            if (String(path) === '/api/v1/panel/version') return { ok: true, json: async () => ({ version: 'v0.1.0-alpha.87', commit: 'f'.repeat(40) }) };
            throw new Error('unexpected fetch ' + path);
        };
        let renderer;
        await act(async () => { renderer = TestRenderer.create(React.createElement(PanelUpdateCard)); await flush(); });
        return { renderer, restore: () => { globalThis.fetch = originalFetch; } };
    }

    test(`${locale}: the update card draws the restart step calmly, keeps the lock, and is the one notice`, async () => {
        const registrations = { registered: 0 };
        const tracking = { version: 'v0.1.0-alpha.88', phase: 'restarting', elapsed: '01:15', requestID: REQUEST };
        const steps = view({ plan: null, phase: 'restarting', lostSince: at(30), now: at(75) });
        const { renderer, restore } = await mountCard({
            active: true, tracking, steps, updated: null,
            registerInlineNotice: () => { registrations.registered += 1; return () => {}; },
        });
        try {
            const box = renderer.root.find(node => node.props?.['data-update-steps-card'] !== undefined);
            const text = textOf(box);
            assert.ok(text.includes(fill(c['panelUpdate.steps.title'], { version: 'v0.1.0-alpha.88' })), text);
            assert.ok(text.includes(c['panelUpdate.steps.restarting']));
            assert.ok(text.includes(`T+01:15 · ID ${REQUEST}`));
            assert.doesNotMatch(text, ALARM);
            assert.equal(box.findAll(node => node.props?.['data-icon'] === 'AlertTriangle').length, 0);
            // The previous calm line is not drawn beside the steps.
            assert.equal(renderer.root.findAll(node => node.props?.['data-update-phase'] !== undefined).length, 0);
            const check = renderer.root.findAll(node => node.type === 'button' && textOf(node).includes(c['panelUpdate.check']));
            assert.equal(check[0].props.disabled, true, 'checking for updates stays locked, with no sentence about it');
            assert.equal(registrations.registered, 1);
        } finally { restore(); await act(async () => renderer.unmount()); }
    });

    test(`${locale}: past the window the card keeps the steps and adds the honest overdue wording`, async () => {
        const tracking = { version: 'v0.1.0-alpha.88', phase: 'unknown', overdue: true, elapsed: '06:01', requestID: REQUEST };
        const steps = view({ plan: null, phase: 'unknown', overdue: true, lostSince: at(30), now: at(361) });
        const { renderer, restore } = await mountCard({ active: true, tracking, steps, updated: null });
        try {
            const row = renderer.root.find(node => node.props?.['data-update-phase'] === 'unknown');
            assert.ok(textOf(row).includes(c['panelUpdate.tracking.restartOverdue']));
            assert.ok(textOf(row).includes(fill(c['panelUpdate.card.unknown'], { version: 'v0.1.0-alpha.88' })));
            // The motion stops: no bar near its end and no "can take a few minutes" beside the overdue wording.
            assert.equal(renderer.root.findAll(node => node.props?.role === 'progressbar').length, 0);
            assert.equal(renderer.root.findAll(node => node.props?.['data-update-restart-note'] !== undefined).length, 0);
            const restartStep = renderer.root.find(node => node.props?.['data-update-step'] === 'restart');
            assert.equal(restartStep.props['data-step-state'], 'running');
        } finally { restore(); await act(async () => renderer.unmount()); }
    });

    test(`${locale}: after the update the card says "Updated to" with the new build id`, async () => {
        const steps = view({ plan: null, phase: 'unknown', succeeded: true });
        const { renderer, restore } = await mountCard({ active: true, tracking: null, steps, updated: { version: 'v0.1.0-alpha.88', commit: COMMIT } });
        try {
            const box = renderer.root.find(node => node.props?.['data-update-steps-card'] !== undefined);
            const text = textOf(box);
            assert.ok(text.includes(fill(c['panelUpdate.steps.updated'], { version: 'v0.1.0-alpha.88' })), text);
            const build = box.find(node => node.props?.['data-update-build'] !== undefined);
            assert.ok(textOf(build).includes(COMMIT.slice(0, 12)));
            assert.equal(box.findAll(node => node.props?.role === 'progressbar').length, 0);
            assert.equal(box.findAll(node => node.props?.['data-step-state'] === 'done').length, 5);
        } finally { restore(); await act(async () => renderer.unmount()); }
    });

    // Frankfurt 2026-10-16 (owner): an update the server's recovery finished
    // showed only "You are on the latest version." on the card. The card treats
    // it as the success it is, with one quiet line; the build line copies as
    // it reads ("build 1743ac69edb0"), the full id stays for screen readers.
    test(`${locale}: an update finished by the server's recovery keeps "Updated to" on the card, with one quiet line`, async () => {
        const steps = view({ plan: null, phase: 'unknown', succeeded: true });
        const { renderer, restore } = await mountCard({ active: true, tracking: null, steps, updated: { version: 'v0.1.0-alpha.92', commit: COMMIT, repeated: true } });
        try {
            const box = renderer.root.find(node => node.props?.['data-update-steps-card'] !== undefined);
            assert.ok(textOf(box).includes(fill(c['panelUpdate.steps.updated'], { version: 'v0.1.0-alpha.92' })));
            assert.equal(box.findAll(node => node.props?.['data-step-state'] === 'done').length, 5);
            assert.equal(textOf(box.find(node => node.props?.['data-update-repeated'] !== undefined)), c['panelUpdate.steps.repeated']);
            const build = box.find(node => node.props?.['data-update-build'] !== undefined);
            const copied = (node) => {
                if (node === null || node === undefined || typeof node === 'boolean') return '';
                if (typeof node === 'string' || typeof node === 'number') return String(node);
                if (Array.isArray(node)) return node.map(copied).join('');
                if (String(node.props?.className ?? '').split(/\s+/).includes('select-none')) return '';
                return copied(node.children);
            };
            assert.equal(copied(build), `build ${COMMIT.slice(0, 12)}`, 'copied text is exactly what is seen');
            const hidden = build.find(node => node.props?.['data-build-full'] !== undefined);
            assert.ok(String(hidden.props.className).includes('sr-only') && String(hidden.props.className).includes('select-none'));
            assert.ok(textOf(hidden).startsWith(' ('), 'a copy that ignores user-select still reads with a space');
            assert.ok(textOf(hidden).includes(COMMIT));
        } finally { restore(); await act(async () => renderer.unmount()); }
    });

    test(`${locale}: control: an ordinary success has no repeated-step line`, async () => {
        const steps = view({ plan: null, phase: 'unknown', succeeded: true });
        const { renderer, restore } = await mountCard({ active: true, tracking: null, steps, updated: { version: 'v0.1.0-alpha.92', commit: COMMIT } });
        try {
            assert.equal(renderer.root.findAll(node => node.props?.['data-update-repeated'] !== undefined).length, 0);
        } finally { restore(); await act(async () => renderer.unmount()); }
    });

    test(`${locale}: control: a card without steps keeps the previous calm line`, async () => {
        const tracking = { version: 'v0.1.0-alpha.88', phase: 'restarting', elapsed: '01:15', requestID: REQUEST };
        const { renderer, restore } = await mountCard({ active: true, tracking });
        try {
            assert.equal(renderer.root.findAll(node => node.props?.['data-update-steps-card'] !== undefined).length, 0);
            const row = renderer.root.find(node => node.props?.['data-update-phase'] === 'restarting');
            assert.ok(textOf(row).includes(fill(c['panelUpdate.card.restarting'], { version: 'v0.1.0-alpha.88' })));
        } finally { restore(); await act(async () => renderer.unmount()); }
    });
}

test('every step string exists in English and Turkish, and the Turkish is not the English', () => {
    const keys = Object.keys(en).filter(key => key.startsWith('panelUpdate.steps.'));
    // 13 + retrying state, its two notes and the repeated-step line (2026-10-16).
    assert.equal(keys.length, 17);
    for (const key of keys) {
        assert.equal(typeof tr[key], 'string', key);
        assert.notEqual(tr[key], en[key], key);
    }
});
