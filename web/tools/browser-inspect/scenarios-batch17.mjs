// Batch 17 (2026-10-16): the update drawn as one bar per step (owner design of
// 2026-10-16, after updating the second installed server from alpha.85 to
// alpha.87 and seeing a warning and a corner notice while nothing was wrong).
//
//   steps88       an update this browser started (its stored record 10 s old,
//                 inside the navigation lock): the download with measured
//                 bytes, then the Panel down (the restart step, time-based),
//                 then PANEL_STARTING (still the restart), then the new
//                 version answering ("verify the new version"), then
//                 succeeded ("Updated to" with the build id)
//   steps88card   past the lock (record 170 s old): the update card with the
//                 steps, then the status read dropped (the restart on the
//                 card), then the same page with its clock moved 7 minutes
//                 ahead (the honest overdue wording)
//   steps88corner another page (Domains) during the restart: the compact
//                 corner notice
//
// The plan is the one cmd/panel system_update_plan.go writes, given to the
// mock as the status answer. The windows are not shortened in the product;
// past the window is reached with a test-side clock on the open page, as in
// batch 13. The mock on 127.0.0.1 answers everything; nothing else is
// contacted.
//
// Screenshots also go to BROWSER_INSPECT_SHOTS (default: the run's out dir) as
// <state>-<lang>-<theme>-<width>.png.
// On yedinci grup: guncellemenin adim cubuklari.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';

const UPDATE_ID = '5b0e8c1f2a3d4e6f708192a3b4c5d6e7';
const TARGET = { version: 'v0.1.0-alpha.88', commit: '8b96ca5ce0e1f2a3b4c5d6e7f8091a2b3c4d5e6f', sequence: '88', os: 'linux', arch: 'amd64', archive_sha256: '5e3b2a19'.padEnd(64, '0'), archive_size: '66201344' };
const followed = createdAgo => ({ 'celikpanel.system-update-operation.v1': JSON.stringify({ state_version: 1, phase: 'active', marker: { marker_version: 1, request_id: UPDATE_ID, current_version: 'v0.1.0-alpha.87', current_commit: 'a8cddcb5400000000000000000000000000000ab', created_at: Date.now() - createdAgo, target: TARGET } }) });
const installed = { '/api/v1/panel/version': { status: 200, body: { version: TARGET.version, commit: TARGET.commit, agent_commit: TARGET.commit, agent_matches: true, hostname: 'server1', ipv4: '203.0.113.10' } } };
const IDS = ['download', 'verify', 'prepare', 'restart', 'confirm'];
const plan = (current, extra = {}) => ({
    steps: IDS.map((id, index) => ({
        id,
        state: index < IDS.indexOf(current) ? 'done' : index === IDS.indexOf(current) ? 'running' : 'pending',
        ...(extra[id] ?? {}),
    })),
    expected_restart_seconds: 120,
    restart_bound_seconds: 360,
});
const running = (current, extra) => ({ request_id: UPDATE_ID, status: 'running', target: TARGET, created_at: '2026-10-16T09:00:00Z', updated_at: '2026-10-16T09:00:01Z', plan: plan(current, extra) });

// What the owner sees: the dialog, the card's steps, the corner notice, which
// step runs and how its bar is drawn, and any alarming sentence.
const facts = page => page.evaluate(() => {
    const visible = node => node && node.getClientRects().length > 0 && getComputedStyle(node).visibility !== 'hidden';
    const lines = node => (visible(node) ? node.innerText.split('\n').filter(Boolean) : null);
    const dialog = document.querySelector('[aria-labelledby="system-update-operation-title"]');
    const card = document.querySelector('[data-update-steps-card]');
    const corner = document.querySelector('[aria-labelledby="system-update-background-title"]');
    const steps = Array.from(document.querySelectorAll('[data-update-step]')).filter(visible)
        .map(node => `${node.getAttribute('data-update-step')}=${node.getAttribute('data-step-state')}`);
    const bars = Array.from(document.querySelectorAll('[role="progressbar"]')).filter(visible).map(node => ({
        label: node.getAttribute('aria-label'),
        kind: node.getAttribute('data-step-progress'),
        fill: node.firstElementChild ? Math.round(node.firstElementChild.getBoundingClientRect().width / Math.max(1, node.getBoundingClientRect().width) * 100) : null,
    }));
    const text = document.body.innerText;
    return {
        dialog: lines(dialog),
        card: lines(card),
        corner: lines(corner),
        steps,
        bars,
        alarming: (text.match(/connection was interrupted|could not be read|not known|bağlantı kesildi|okunamadı|bilinmiyor/gi) || []),
        overdueText: /longer than an update normally needs|normalde gereken süreden daha uzun/i.test(text),
        restartNote: !!document.querySelector('[data-update-restart-note]'),
        warningMarks: Array.from(document.querySelectorAll('.text-warning')).filter(visible).length,
        navigationLock: !!document.querySelector('[inert][aria-busy="true"]'),
        sidebarVersion: (text.match(/v0\.1\.0-alpha\.\d+/g) || []).slice(-1)[0] ?? null,
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
    };
});

export default function batch17(scenarios, { base, locale, theme, out, viewport, ctl, reset, newPage, closePage, shot, into, waitFor, pause }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const snap = async (page, state, note = {}) => {
        await shot(page, `s88-${state}`, { ...(await facts(page)), ...note });
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `s88-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    const stepRuns = (page, id) => waitFor(page, (step) => !!document.querySelector(`[data-update-step="${step}"][data-step-state="running"]`), 25000, id).catch(() => {});

    scenarios.steps88 = async () => {
        await reset();
        await ctl({ clearAll: true });
        await ctl({ validity: 45, update: running('download', { download: { progress: 0.42, elapsed_seconds: 12 } }) });
        const page = await newPage(followed(10_000));
        await page.goto(`${base}/settings?section=updates`, { waitUntil: 'domcontentloaded' });
        await stepRuns(page, 'download');
        await pause(1500);
        await snap(page, 'download');
        // The installer verified the package; the updater checks the server.
        await ctl({ update: running('prepare') });
        await stepRuns(page, 'prepare');
        await pause(800);
        // The Panel stops: every API request loses its connection.
        await ctl({ mode: 'down' });
        const down = Date.now();
        await stepRuns(page, 'restart');
        await pause(Math.max(0, 25_000 - (Date.now() - down)));
        await snap(page, 'restart', { outageSeconds: Math.round((Date.now() - down) / 1000) });
        // The new Panel answers; its management is still starting.
        await ctl({ mode: 'starting' });
        await pause(9_000);
        await snap(page, 'starting', { outageSeconds: Math.round((Date.now() - down) / 1000) });
        // Ready, on the new version: the confirmation runs.
        await ctl({ mode: 'ok', update: { ...running('confirm'), phase: 'verifying' }, override: installed });
        await stepRuns(page, 'confirm');
        await pause(1200);
        await snap(page, 'confirm', { secondsSinceDown: Math.round((Date.now() - down) / 1000) });
        // Succeeded: "Updated to" with the build id, then the page's own reload.
        await ctl({ update: { ...running('confirm'), status: 'succeeded', plan: { ...plan('confirm'), steps: IDS.map(id => ({ id, state: 'done' })) } } });
        await waitFor(page, () => !!document.querySelector('[data-update-build]'), 15000).catch(() => {});
        await snap(page, 'updated');
        await closePage(page);
        await ctl({ clearAll: true });
    };

    scenarios.steps88card = async () => {
        await reset();
        await ctl({ clearAll: true });
        await ctl({ update: running('verify') });
        const page = await newPage(followed(170_000));
        await page.goto(`${base}/settings?section=updates`, { waitUntil: 'domcontentloaded' });
        await stepRuns(page, 'verify');
        await pause(1200);
        await into(page, '[data-update-steps-card]');
        await snap(page, 'card');
        await ctl({ override: { '/api/v1/panel/update/status': { drop: true } } });
        await stepRuns(page, 'restart');
        await pause(6000);
        await into(page, '[data-update-steps-card]');
        await snap(page, 'card-restart');
        await page.evaluate(() => { const real = Date.now.bind(Date); Date.now = () => real() + 7 * 60_000; });
        await waitFor(page, () => document.querySelector('[data-update-phase]')?.getAttribute('data-update-phase') === 'unknown', 10000).catch(() => {});
        await pause(800);
        await into(page, '[data-update-steps-card]');
        await snap(page, 'overdue', { clockAhead: '7 min (test-side Date.now)' });
        await closePage(page);
        await ctl({ clearAll: true });
    };

    scenarios.steps88corner = async () => {
        await reset();
        await ctl({ clearAll: true });
        await ctl({ update: running('download', { download: { progress: 0.6 } }) });
        const page = await newPage(followed(170_000));
        await page.goto(`${base}/domains`, { waitUntil: 'domcontentloaded' });
        await waitFor(page, () => !!document.querySelector('[aria-labelledby="system-update-background-title"] [data-update-step]'), 20000).catch(() => {});
        await pause(1500);
        await ctl({ override: { '/api/v1/panel/update/status': { drop: true } } });
        await waitFor(page, () => !!document.querySelector('[aria-labelledby="system-update-background-title"] [data-update-step="restart"]'), 20000).catch(() => {});
        await pause(5000);
        await snap(page, 'corner');
        await closePage(page);
        await ctl({ clearAll: true });
    };
}
