// Batch 13 (2026-10-15): the update's own restart (owner report 2026-10-15,
// Frankfurt, a real update alpha.84 -> alpha.85: a warning-marked "not known"
// on the update page and the same sentence in the corner notice at T+02:59
// while the panel restarted as planned). D-033: calm progress inside the
// expected window, "not known" only after it; one notice, not two.
//
//   u86restart   the update page while the status read loses the connection
//                after a read said "running": the card's calm restart line
//                with T+ and the ID, no corner notice; then the same page past
//                the window; then another page (Domains) with the corner
//                notice in the restart
//
// The window is not shortened in the product. Past it is reached with a
// test-side clock control on the open page: Date.now is moved forward by a
// fixed amount after the calm state was photographed (`clockAhead`), recorded
// with the state. The mock on 127.0.0.1 answers everything; the status read is
// made to drop its connection with the mock's `drop` override.
//
// Screenshots also go to BROWSER_INSPECT_SHOTS (default: the run's out dir) as
// <state>-<lang>-<theme>-<width>.png.
// On ucuncu grup: guncellemenin kendi yeniden baslamasi.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';

const UPDATE_ID = 'f142f0a883f393df25bda7927dfb2c70';
const TARGET = { version: 'v0.1.0-alpha.85', commit: '36c33c4531ee6c1306d324db7a666403176f3bf7', sequence: '85', os: 'linux', arch: 'amd64', archive_sha256: 'cb6424b7'.padEnd(64, '0'), archive_size: '66146417' };
const followed = createdAgo => ({ 'celikpanel.system-update-operation.v1': JSON.stringify({ state_version: 1, phase: 'active', marker: { marker_version: 1, request_id: UPDATE_ID, current_version: 'v0.1.0-alpha.84', current_commit: 'f0157e6c8f1614cff176821172f8a9067493e625', created_at: Date.now() - createdAgo, target: TARGET } }) });

// What the owner sees: the card's line, its icon and colour, the corner notice.
const facts = page => page.evaluate(() => {
    const visible = node => node && node.getClientRects().length > 0 && getComputedStyle(node).visibility !== 'hidden';
    const line = document.querySelector('[data-update-phase]');
    const corner = document.querySelector('[aria-labelledby="system-update-background-title"]');
    const dialog = document.querySelector('[aria-labelledby="system-update-operation-title"]');
    const warning = Array.from(document.querySelectorAll('.text-warning')).filter(visible)
        .map(node => (node.closest('[data-update-phase], aside, [role="dialog"]')?.getAttribute('data-update-phase')) || node.closest('aside, [role="dialog"]')?.tagName || 'page');
    return {
        phase: line?.getAttribute('data-update-phase') ?? null,
        line: visible(line) ? line.innerText.split('\n').filter(Boolean) : null,
        lineSpinner: !!line?.querySelector('.animate-spin, [class*="animate-spin"]'),
        lineWarning: !!line?.querySelector('.text-warning'),
        corner: visible(corner) ? corner.innerText.split('\n').filter(Boolean) : null,
        cornerWarning: !!corner?.querySelector('.text-warning'),
        dialog: visible(dialog) ? dialog.innerText.split('\n').filter(Boolean) : null,
        warningMarks: warning,
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
    };
});

export default function batch13(scenarios, { base, locale, theme, out, viewport, ctl, reset, newPage, closePage, shot, into, waitFor, pause }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const snap = async (page, state, note = {}) => {
        await shot(page, `u86-${state}`, { ...(await facts(page)), ...note });
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `u86-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };

    scenarios.u86restart = async () => {
        await reset();
        await ctl({ clearAll: true });
        // Past the two-minute navigation lock: the update page is usable, as on
        // the owner's screen at T+02:59.
        await ctl({ update: { request_id: UPDATE_ID, status: 'running', target: TARGET } });
        let page = await newPage(followed(170_000));
        await page.goto(`${base}/settings?section=updates`, { waitUntil: 'domcontentloaded' });
        await waitFor(page, () => document.querySelector('[data-update-phase]')?.getAttribute('data-update-phase') === 'applying', 20000).catch(() => {});
        // The panel restarts: the status read loses its connection.
        await ctl({ override: { '/api/v1/panel/update/status': { drop: true } } });
        await waitFor(page, () => document.querySelector('[data-update-phase]')?.getAttribute('data-update-phase') === 'restarting', 20000).catch(() => {});
        await pause(1200);
        await into(page, '[data-update-phase]');
        await snap(page, 'restart-calm');

        // Past the window: the same open page, its clock moved forward.
        await page.evaluate(() => { const real = Date.now.bind(Date); Date.now = () => real() + 7 * 60_000; });
        await waitFor(page, () => document.querySelector('[data-update-phase]')?.getAttribute('data-update-phase') === 'unknown', 10000).catch(() => {});
        await pause(600);
        await into(page, '[data-update-phase]');
        await snap(page, 'restart-overdue', { clockAhead: '7 min (test-side Date.now)' });
        await closePage(page);

        // Another page: the corner notice is the one notice, calm in the restart.
        await reset();
        await ctl({ clearAll: true });
        await ctl({ update: { request_id: UPDATE_ID, status: 'running', target: TARGET } });
        page = await newPage(followed(170_000));
        await page.goto(`${base}/domains`, { waitUntil: 'domcontentloaded' });
        await waitFor(page, () => !!document.querySelector('[aria-labelledby="system-update-background-title"]'), 20000).catch(() => {});
        await pause(2000);
        await ctl({ override: { '/api/v1/panel/update/status': { drop: true } } });
        await waitFor(page, () => /restart|yeniden başl/i.test(document.querySelector('[aria-labelledby="system-update-background-title"]')?.innerText || ''), 20000).catch(() => {});
        await pause(1200);
        await snap(page, 'restart-corner');
        await closePage(page);
        await ctl({ clearAll: true });
    };
}
