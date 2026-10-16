// The update's own restart is progress, not a problem (owner report
// 2026-10-15, Frankfurt, a real update alpha.84 -> alpha.85): while the panel
// restarted as the update's plan says it will, the update page showed a
// warning-marked "could not be read just now, so it is not known whether it has
// finished" and, at the same time, a corner notice saying the same thing with
// "T+02:59". D-033: expected waiting is calm progress; the "not known" wording
// and the warning mark come only after the expected window or on a verified
// failure (D-024 keeps the three apart). One notice, not two.
//
// Guncellemenin kendi yeniden baslamasi ilerlemedir; "bilinmiyor" ancak
// beklenen pencere asilinca ya da dogrulanmis hatada. Tek bildirim.
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
import {
    SYSTEM_UPDATE_EXPECTED_RESTART_MS,
    SYSTEM_UPDATE_READ_MEMORY,
    SYSTEM_UPDATE_RESTART_AFTER_LOSS_MS,
    systemUpdateCornerNoticeRepeats,
    systemUpdateDisplayedPhase,
    systemUpdateFollowRead,
    systemUpdateRestartDeadline,
    systemUpdateTrackingText,
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
export const AlertTriangle = icon('AlertTriangle'), CheckCircle2 = icon('CheckCircle2'), DownloadCloud = icon('DownloadCloud'), Loader2 = icon('Loader2'), RefreshCw = icon('RefreshCw');`);
const uiURL = moduleURL(`import React from '${reactURL}'; export function Button({ children, ...props }) { return React.createElement('button', props, children); }`);
const i18nURL = moduleURL(`const fill = (text, vars) => String(text).replace(/\\{(\\w+)\\}/g, (all, name) => vars && vars[name] !== undefined ? String(vars[name]) : all);
export function useI18n() { const c = globalThis.__restartCatalogue; return { t: (key, vars) => fill(c.strings[key] ?? key, vars), locale: c.locale }; }`);
const apiErrorURL = moduleURL(`export async function readApiError() { return {}; } export function apiErrorText() { return 'api-error'; }`);
const operationURL = moduleURL(`
export function createSystemUpdateRequestID() { return 'mounted-request-id'; }
export function systemUpdateResponseHint() { return 'response-error'; }
export function useSystemUpdateOperation() { return globalThis.__restartOperation; }
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
const { PanelUpdateCard } = await import(compileURL('../src/components/PanelUpdateCard.tsx', [
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
    [/from ['"]\.\/BuildCommit['"]/g, `from '${compileURL('../src/components/BuildCommit.tsx', [[/from ['"]react\/jsx-runtime['"]/g, `from '${jsxRuntimeURL}'`], [/from ['"]\.\.\/i18n['"]/g, `from '${i18nURL}'`]])}'`],
]));

const flush = async () => { for (let i = 0; i < 4; i += 1) await Promise.resolve(); };
function textOf(node) {
    if (node === null || node === undefined || typeof node === 'boolean') return '';
    if (typeof node === 'string' || typeof node === 'number') return String(node);
    if (Array.isArray(node)) return node.map(textOf).join(' ');
    return textOf(node.children);
}
const REQUEST = 'f142f0a883f393df25bda7927dfb2c70';
async function mountCard(locale, tracking, registrations = null) {
    globalThis.__restartCatalogue = { locale, strings: catalogues[locale] };
    globalThis.__restartOperation = {
        active: tracking !== null,
        start: async () => ({ kind: 'accepted' }),
        tracking,
        ...(registrations ? {
            registerInlineNotice: () => {
                registrations.registered += 1;
                return () => { registrations.released += 1; };
            },
        } : {}),
    };
    const originalFetch = globalThis.fetch;
    globalThis.fetch = async (path) => {
        if (String(path) === '/api/v1/panel/version') return { ok: true, json: async () => ({ version: 'v0.1.0-alpha.84', commit: 'f'.repeat(40) }) };
        throw new Error('unexpected fetch ' + path);
    };
    let renderer;
    await act(async () => { renderer = TestRenderer.create(React.createElement(PanelUpdateCard)); await flush(); });
    return { renderer, restore: () => { globalThis.fetch = originalFetch; } };
}
const row = renderer => renderer.root.find(node => node.props && node.props['data-update-phase'] !== undefined);
const icons = node => node.findAll(child => child.props && child.props['data-icon'] !== undefined).map(child => child.props['data-icon']);
const classes = node => node.findAll(child => typeof child.props?.className === 'string').map(child => child.props.className).join(' ');
const tTo = locale => (key, vars) => fill(catalogues[locale][key] ?? key, vars);

const START = Date.UTC(2026, 9, 15, 6, 40, 0);
const at = seconds => START + seconds * 1000;

test('the restart window: 6 minutes from the start, and at least 2 minutes from the first lost read', () => {
    assert.equal(SYSTEM_UPDATE_EXPECTED_RESTART_MS, 360_000);
    assert.equal(SYSTEM_UPDATE_RESTART_AFTER_LOSS_MS, 120_000);
    assert.equal(systemUpdateRestartDeadline(START, at(20)), at(360));
    assert.equal(systemUpdateRestartDeadline(START, at(300)), at(420), 'a late restart still gets 2 minutes');
    // An invalid start never creates an unbounded calm state.
    assert.equal(systemUpdateRestartDeadline(Number.NaN, at(20)), at(140));
});

test('a lost read after the server said the update is running is the restart; without that, only inside the window from the start', () => {
    let step = systemUpdateFollowRead(SYSTEM_UPDATE_READ_MEMORY, { operation: { status: 'running' }, disconnected: false, at: at(5) }, START);
    assert.equal(step.phase, 'applying');
    assert.deepEqual(step.memory, { lastKnownActive: true, lostSince: null });
    step = systemUpdateFollowRead(step.memory, { operation: null, disconnected: true, at: at(20) }, START);
    assert.equal(step.phase, 'restarting');
    assert.equal(step.restartDeadline, at(360));
    // The outage keeps its first moment.
    step = systemUpdateFollowRead(step.memory, { operation: null, disconnected: true, at: at(40) }, START);
    assert.equal(step.memory.lostSince, at(20));
    // A read that answers ends the outage.
    step = systemUpdateFollowRead(step.memory, { operation: { status: 'running', phase: 'verifying' }, disconnected: false, at: at(52) }, START);
    assert.equal(step.phase, 'verifying');
    assert.equal(step.memory.lostSince, null);
    // Queued counts as active: the worker may begin at any moment.
    const queued = systemUpdateFollowRead(SYSTEM_UPDATE_READ_MEMORY, { operation: { status: 'queued' }, disconnected: false, at: at(1) }, START);
    assert.equal(systemUpdateFollowRead(queued.memory, { operation: null, disconnected: true, at: at(3) }, START).phase, 'restarting');
    // Nothing read yet in this document (a page opened during the restart;
    // owner design 2026-10-16): inside the window counted from the start it
    // is the restart, bounded by that window only; after it, not known. The
    // server's word that the update is active is still not assumed.
    const cold = systemUpdateFollowRead(SYSTEM_UPDATE_READ_MEMORY, { operation: null, disconnected: true, at: at(3) }, START);
    assert.equal(cold.phase, 'restarting');
    assert.equal(cold.restartDeadline, at(360));
    assert.equal(cold.memory.lastKnownActive, false);
    const coldLate = systemUpdateFollowRead(SYSTEM_UPDATE_READ_MEMORY, { operation: null, disconnected: true, at: at(360) }, START);
    assert.equal(coldLate.phase, 'unknown');
    assert.equal(coldLate.restartDeadline, null);
    assert.equal(systemUpdateFollowRead(SYSTEM_UPDATE_READ_MEMORY, { operation: null, disconnected: true, at: at(3) }, Number.NaN).phase, 'unknown',
        'no valid start: never calm');
    // An answer that could not be verified is not a restart.
    assert.equal(systemUpdateFollowRead({ lastKnownActive: true, lostSince: null }, { operation: null, disconnected: false, at: at(3) }, START).phase, 'unknown');
});

test('the measured updates never reach "not known"; past the window it is said by itself', () => {
    // set16 Debian 13: start 22:55:07Z, API unanswered 22:55:19Z..22:55:51Z.
    const memory = systemUpdateFollowRead(SYSTEM_UPDATE_READ_MEMORY, { operation: { status: 'running' }, disconnected: false, at: at(12) }, START).memory;
    const lost = systemUpdateFollowRead(memory, { operation: null, disconnected: true, at: at(19) }, START);
    for (const second of [19, 30, 44]) assert.equal(systemUpdateDisplayedPhase(lost.phase, lost.restartDeadline, at(second)), 'restarting');
    // The owner's real update: still no answer at T+02:59.
    assert.equal(systemUpdateDisplayedPhase(lost.phase, lost.restartDeadline, at(179)), 'restarting');
    assert.equal(systemUpdateDisplayedPhase(lost.phase, lost.restartDeadline, at(359)), 'restarting');
    assert.equal(systemUpdateDisplayedPhase(lost.phase, lost.restartDeadline, at(360)), 'unknown');
    assert.equal(systemUpdateDisplayedPhase('restarting', null, at(10)), 'unknown', 'no deadline is never calm');
    assert.equal(systemUpdateDisplayedPhase('applying', null, at(999)), 'applying');
});

for (const locale of ['en', 'tr']) {
    const c = catalogues[locale];
    const t = tTo(locale);

    test(`${locale}: the restart is said once, calmly, with nothing to press`, () => {
        const notice = systemUpdateTrackingText({ version: 'v0.1.0-alpha.85', phase: 'restarting' }, c['panelUpdate.connectionLost'], t);
        assert.deepEqual(notice, { title: null, message: c['panelUpdate.tracking.restarting'], hint: null });
        assert.equal(en['panelUpdate.tracking.restarting'], 'The panel is restarting as part of the update. This page reconnects by itself; nothing needs to be pressed.');
        assert.doesNotMatch(notice.message, /not known|bilinmiyor|could not|okunamadı/);
    });

    test(`${locale}: past the window the notice says not known, not failed, and what the owner checks`, () => {
        const notice = systemUpdateTrackingText({ version: 'v0.1.0-alpha.85', phase: 'unknown', overdue: true }, c['panelUpdate.connectionLost'], t);
        assert.equal(notice.message, c['panelUpdate.tracking.unknown']);
        assert.equal(notice.hint, c['panelUpdate.tracking.restartOverdue']);
        assert.match(notice.hint, /sudo systemctl status celikpanel-panel/);
        assert.match(notice.hint, locale === 'en' ? /does not mean the update failed/ : /başarısız olduğu anlamına gelmez/);
    });

    test(`${locale}: the update card draws the restart as progress, carries T+ and the ID, and keeps the lock`, async () => {
        const registrations = { registered: 0, released: 0 };
        const tracking = { version: 'v0.1.0-alpha.85', phase: 'restarting', elapsed: '02:59', requestID: REQUEST };
        const { renderer, restore } = await mountCard(locale, tracking, registrations);
        try {
            const line = row(renderer);
            const text = textOf(line);
            assert.ok(text.includes(fill(c['panelUpdate.card.restarting'], { version: 'v0.1.0-alpha.85' })), text);
            assert.ok(!text.includes(fill(c['panelUpdate.card.unknown'], { version: 'v0.1.0-alpha.85' })));
            assert.deepEqual(icons(line), ['Loader2']);
            assert.doesNotMatch(classes(line), /warning/);
            assert.ok(text.includes(`T+02:59 · ID ${REQUEST}`), 'the counter and the ID are on the card');
            assert.equal(line.props.role, 'status');
            const check = renderer.root.findAll(node => node.type === 'button' && textOf(node).includes(c['panelUpdate.check']));
            assert.equal(check.length, 1);
            assert.equal(check[0].props.disabled, true, 'checking for updates stays locked');
            assert.equal(registrations.registered, 1, 'the card is the one notice while it is on screen');
        } finally { restore(); await act(async () => renderer.unmount()); }
        assert.equal(registrations.released, 1, 'leaving the page gives the corner notice back');
    });

    test(`${locale}: past the window the card shows the warning mark, not known, and what to check`, async () => {
        const tracking = { version: 'v0.1.0-alpha.85', phase: 'unknown', overdue: true, elapsed: '06:01', requestID: REQUEST };
        const { renderer, restore } = await mountCard(locale, tracking);
        try {
            const line = row(renderer);
            const text = textOf(line);
            assert.deepEqual(icons(line), ['AlertTriangle']);
            assert.match(classes(line), /text-warning/);
            assert.ok(text.includes(fill(c['panelUpdate.card.unknown'], { version: 'v0.1.0-alpha.85' })));
            assert.ok(text.includes(c['panelUpdate.tracking.restartOverdue']));
            assert.ok(!text.includes(fill(c['panelUpdate.card.restarting'], { version: 'v0.1.0-alpha.85' })));
            assert.ok(text.includes(`T+06:01 · ID ${REQUEST}`));
        } finally { restore(); await act(async () => renderer.unmount()); }
    });
}

test('one notice: the corner notice does not repeat the card, and a known outcome keeps it', () => {
    assert.equal(systemUpdateCornerNoticeRepeats(1, true), true);
    assert.equal(systemUpdateCornerNoticeRepeats(0, true), false, 'another page: the corner notice is the one');
    assert.equal(systemUpdateCornerNoticeRepeats(1, false), false, 'a failure or success: the card does not show it');
});

test('the provider: restart from the read, warning only off the restart, a failure stays a failure, one notice', () => {
    const tracker = readFileSync(new URL('../src/components/SystemUpdateOperation.tsx', import.meta.url), 'utf8');
    // The poll classifies every non-terminal read; a verified failure is
    // committed as terminal before it, so it is never shown as a restart.
    const failed = tracker.indexOf("if (outcome.kind === 'failed') {\n                const committed = await commitTerminal(exactMarker, 'failed', outcome.message);");
    const follow = tracker.indexOf('systemUpdateFollowRead(\n                readMemory(exactMarker.request_id),');
    assert.ok(failed > 0 && follow > failed);
    // The start POST accepted by the server makes the request known active.
    assert.match(tracker, /if \(response\.ok\) \{\n(?:.*\n){0,3}\s+readMemoryRef\.current = \{\n\s+requestID: exactMarker\.request_id,\n\s+memory: \{ \.\.\.readMemory\(exactMarker\.request_id\), lastKnownActive: true \},/);
    // What is drawn: the displayed phase, with the window applied each second.
    assert.match(tracker, /const displayedPhase = systemUpdateDisplayedPhase\(view\.phase, view\.restartDeadline, now\);/);
    assert.match(tracker, /phase: displayedPhase,/);
    // The dialog's warning icon (WifiOff) is not drawn during the restart.
    assert.match(tracker, /const disconnected = !pendingReload && !requiredReloadMarker && marker !== null && view\.disconnected\n\s+&& displayedPhase !== 'restarting';/);
    // The corner notice is not drawn while the card carries the same state.
    // Since 2026-10-16 the card also carries "Updated to", so a success is not repeated either.
    assert.match(tracker, /const inlineNoticeCarriesTracking = systemUpdateCornerNoticeRepeats\(inlineNotices, tracking !== null \|\| updated !== null\);/);
    assert.match(tracker, /const backgroundVisible = navigationLease\.released && !authPaused && !blocking && exactMarker !== null\n\s+&& !inlineNoticeCarriesTracking/);
    assert.match(tracker, /registerInlineNotice,\n\s+steps: stepsView,\n\s+updated,\n\s+\}\}>/);
    // The do-not-start-another-update lock is untouched: mutation stays occupied.
    assert.match(tracker, /const occupied = systemUpdateMutationLocked\(/);
});

// Native record set17, defect D1 (2026-10-15): the licence gate drew its own
// "not answering" dialog over the update's restart and, through the observation
// recovery it raised, ended the update's navigation lock. The provider now
// publishes the followed update to the gate (lib/systemUpdateRestartSignal.ts),
// and the gate keeps its last positive decision inside the same window.
const restartSignal = await import(compileURL('../src/lib/systemUpdateRestartSignal.ts', [
    [/from ['"]react['"]/, `from '${reactURL}'`],
    [/from ['"]\.\/systemUpdateTracking['"]/, `from '${compileURL('../src/lib/systemUpdateTracking.ts')}'`],
]));

test('set17 D1: the access grace ends with the update restart window, needs a lost read, and is bounded by the decision', () => {
    const { systemUpdateAccessGraceEnd: grace } = restartSignal;
    const start = 1_800_000_000_000;
    const signal = { requestID: 'a'.repeat(32), startedAt: start, lostSince: null };
    const until = start + 30_000;
    assert.equal(grace(null, until, start + 20_000), null, 'no followed update: nothing is kept');
    assert.equal(grace(signal, until, null), null, 'no lost read: the decision ends at its own deadline');
    // The update tracking's or the gate's lost read: the earlier one counts.
    assert.equal(grace({ ...signal, lostSince: start + 25_000 }, until, null), start + SYSTEM_UPDATE_EXPECTED_RESTART_MS);
    assert.equal(grace(signal, until, start + 25_000), start + SYSTEM_UPDATE_EXPECTED_RESTART_MS);
    assert.equal(grace({ ...signal, lostSince: start + 300_000 }, start + 400_000, start + 290_000),
        start + 290_000 + SYSTEM_UPDATE_RESTART_AFTER_LOSS_MS, 'a late restart: 2 minutes from the first lost read');
    // Never later than the decision's own end plus the 6-minute window.
    assert.equal(grace({ ...signal, lostSince: start + 600_000 }, start + 10_000, null), start + 10_000 + SYSTEM_UPDATE_EXPECTED_RESTART_MS);
    assert.equal(grace({ ...signal, lostSince: start + 25_000 }, 0, null), null, 'no positive decision: nothing to keep');
    assert.equal(grace({ ...signal, lostSince: Number.NaN }, until, null), null);
});

test('the session grace ends with the update restart window from the first PANEL_STARTING answer or lost read', () => {
    const { systemUpdateSessionGraceEnd: grace } = restartSignal;
    const start = 1_800_000_000_000;
    const signal = { requestID: 'a'.repeat(32), startedAt: start, lostSince: null };
    assert.equal(grace(null, start + 40_000), null, 'no followed update: the session is marked as before');
    assert.equal(grace(signal, start + 40_000), start + SYSTEM_UPDATE_EXPECTED_RESTART_MS, '6 minutes from the start');
    assert.equal(grace(signal, start + 300_000), start + 300_000 + SYSTEM_UPDATE_RESTART_AFTER_LOSS_MS, 'a late start: 2 minutes from the first answer');
    assert.equal(grace({ ...signal, lostSince: start + 290_000 }, start + 300_000), start + 290_000 + SYSTEM_UPDATE_RESTART_AFTER_LOSS_MS, 'the earlier lost read counts');
    assert.equal(grace(signal, Number.NaN), null);
});

test('set17 D1: the restart signal publishes only a change and is read by subscribers', () => {
    const seen = [];
    const unsubscribe = restartSignal.subscribeSystemUpdateRestart(() => seen.push(restartSignal.currentSystemUpdateRestart()));
    try {
        const value = { requestID: 'b'.repeat(32), startedAt: 1, lostSince: null };
        restartSignal.publishSystemUpdateRestart(value);
        restartSignal.publishSystemUpdateRestart({ ...value });
        restartSignal.publishSystemUpdateRestart({ ...value, lostSince: 5 });
        restartSignal.publishSystemUpdateRestart(null);
        restartSignal.publishSystemUpdateRestart(null);
        assert.deepEqual(seen, [value, { ...value, lostSince: 5 }, null]);
    } finally { unsubscribe(); restartSignal.publishSystemUpdateRestart(null); }
});

test('set17 D1: the provider publishes the followed update once the server said it is active, with its first lost read', () => {
    const tracker = readFileSync(new URL('../src/components/SystemUpdateOperation.tsx', import.meta.url), 'utf8');
    assert.match(tracker, /import \{ publishSystemUpdateRestart, systemUpdateNavigationRestart, useSystemUpdateRestart \} from '\.\.\/lib\/systemUpdateRestartSignal';/);
    assert.match(tracker, /const followedMemory = followed \? readMemory\(followed\.request_id\) : null;/);
    assert.match(tracker, /const restartRequestID = followed && followedMemory\?\.lastKnownActive \? followed\.request_id : null;/);
    assert.match(tracker, /const restartLostSince = followedMemory\?\.lostSince \?\? null;/);
    assert.match(tracker, /publishSystemUpdateRestart\(restartRequestID === null \|\| restartStartedAt === null \? null\n\s+: \{ requestID: restartRequestID, startedAt: restartStartedAt, lostSince: restartLostSince \}\);/);
    assert.match(tracker, /useEffect\(\(\) => \(\) => publishSystemUpdateRestart\(null\), \[\]\);/);
    // The gate reads it; App still pauses the update tracking on the gate's
    // observation recovery, which the gate no longer raises inside the window.
    const gate = readFileSync(new URL('../src/components/LicenseOnboarding.tsx', import.meta.url), 'utf8');
    assert.match(gate, /const published = useSystemUpdateRestart\(\);/);
    const app = readFileSync(new URL('../src/App.tsx', import.meta.url), 'utf8');
    assert.match(app, /publishSystemUpdateAuthentication\(state === 'ready' && !observationRecovery && user\?\.effective_role === 'admin'\);/);
});

// Native record set17b, finding F1 (2026-10-15): the lock ended by its
// two-minute lease at T+120 s in the middle of the restart. The provider now
// hands the lease the restart it publishes, for the exact request it follows;
// the lease's own behaviour is tested mounted in
// tests/system-update-navigation-lease-mounted.test.mjs.
test('set17b F1: the provider keeps the navigation lock through its own published restart', () => {
    const tracker = readFileSync(new URL('../src/components/SystemUpdateOperation.tsx', import.meta.url), 'utf8');
    assert.match(tracker, /const publishedRestart = useSystemUpdateRestart\(\);/);
    assert.match(tracker, /const navigationRestart = systemUpdateNavigationRestart\(\n\s+publishedRestart,\n\s+marker\?\.request_id \?\? null,\n\s+marker\?\.created_at \?\? null,\n\s+\);/);
    assert.match(tracker, /useSystemUpdateNavigationLease\(\n\s+modalIdentity,\n\s+exactMarker\?\.created_at \?\? null,\n\s+operationBlocks,\n\s+exactMarker === marker \? navigationRestart : null,\n\s+\);/);
    const { systemUpdateNavigationRestart: select } = restartSignal;
    const start = 1_800_000_000_000;
    const id = 'c'.repeat(32);
    assert.deepEqual(select({ requestID: id, startedAt: start, lostSince: start + 25_000 }, id, start),
        { lostSince: start + 25_000, until: systemUpdateRestartDeadline(start, start + 25_000) });
    assert.equal(select({ requestID: id, startedAt: start, lostSince: null }, id, start), null, 'reconnected: the restart no longer keeps the lock');
    // The lock line says the same bounds, in both languages.
    assert.match(en['panelUpdate.interactionLocked'], /after two minutes; if the panel is still restarting then, once it answers again, at the latest six minutes after the start/);
    assert.match(tr['panelUpdate.interactionLocked'], /İki dakika sonra kendiliğinden açılır; panel o sırada hâlâ yeniden başlıyorsa yeniden yanıt verdiğinde, en geç başlangıçtan altı dakika sonra açılır/);
});
