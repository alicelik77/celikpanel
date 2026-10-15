// Batch 15 (2026-10-15): native record set17, defect D1. During a real
// update's restart, once the outage outlasted the access validity of the last
// good access read (42 to 60 s in the lab), the licence gate drew "The Panel is
// not answering; an update may be restarting it" over the update's calm
// restart, and the update's navigation lock did not come back after
// reconnection (deploy/e2e/release-recovery/evidence/set17-20261015, "Defect
// D1"; debian13/upd86-good-a/browser/page/state-05-viewport.png).
//
//   d1restart   an update this browser started (its stored record, 2 s old,
//               inside the two-minute navigation lock), the access answer
//               valid for 45 s, then the Panel down for 90 s (every API
//               request dropped), then the Panel back on the new version:
//               "verifying", then "succeeded" and the page's own reload
//
// The update is not started by a press here: the record is the one the page
// writes when it starts one (as batch 13). The restart window is not shortened
// in the product; the outage is real time. The mock on 127.0.0.1 answers
// everything; nothing else is contacted.
//
// Screenshots also go to BROWSER_INSPECT_SHOTS (default: the run's out dir) as
// <state>-<lang>-<theme>-<width>.png.
// On besinci grup: set17 D1, guncellemenin yeniden baslamasinda lisans kapisi.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';

const UPDATE_ID = '1669d2e8c9cd241bc0c93505605454f0';
const TARGET = { version: 'v0.1.0-alpha.86', commit: '313f3eea2c9b89251588648b09484cd34b7aecb1', sequence: '86', os: 'linux', arch: 'amd64', archive_sha256: '8c35464d'.padEnd(64, '0'), archive_size: '66170399' };
const followed = createdAgo => ({ 'celikpanel.system-update-operation.v1': JSON.stringify({ state_version: 1, phase: 'active', marker: { marker_version: 1, request_id: UPDATE_ID, current_version: 'v0.1.0-alpha.85', current_commit: '36c33c4531ee6c1306d324db7a666403176f3bf7', created_at: Date.now() - createdAgo, target: TARGET } }) });
const installed = { '/api/v1/panel/version': { status: 200, body: { version: TARGET.version, commit: TARGET.commit, agent_commit: TARGET.commit, agent_matches: true, hostname: 'server1', ipv4: '203.0.113.10' } } };

// What the owner sees: the update's dialog, the licence gate's dialog, whether
// the pages are held by the gate, and the version the sidebar names.
const facts = page => page.evaluate(() => {
    const visible = node => node && node.getClientRects().length > 0 && getComputedStyle(node).visibility !== 'hidden';
    const update = document.querySelector('[aria-labelledby="system-update-operation-title"]');
    const hold = document.getElementById('access-hold') || document.querySelector('[data-top-layer="hold"]');
    const notAnswering = /not answering|yanıt vermiyor/i.test(document.body.innerText);
    return {
        updateDialog: visible(update) ? update.innerText.split('\n').filter(Boolean) : null,
        updateSpinner: !!update?.querySelector('.animate-spin'),
        updateWarning: !!update?.querySelector('.text-warning'),
        accessHoldDialog: visible(hold) ? hold.innerText.split('\n').filter(Boolean) : null,
        accessHeldPages: document.querySelectorAll('[data-access-hold="blocked"]').length,
        notAnsweringText: notAnswering,
        connectionBanner: Array.from(document.querySelectorAll('[role="status"]')).filter(visible).map(node => node.innerText).filter(text => /connection|bağlantı/i.test(text)),
        sidebarVersion: (document.body.innerText.match(/v0\.1\.0-alpha\.\d+/g) || []).slice(-1)[0] ?? null,
        reloaded: location.search.includes('_cp_update'),
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
    };
});

export default function batch15(scenarios, { base, locale, theme, out, viewport, ctl, reset, newPage, closePage, shot, waitFor, pause }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const snap = async (page, state, note = {}) => {
        await shot(page, `d1-${state}`, { ...(await facts(page)), ...note });
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `d1-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };

    scenarios.d1restart = async () => {
        await reset();
        await ctl({ clearAll: true });
        await ctl({ validity: 45, update: { request_id: UPDATE_ID, status: 'running', target: TARGET } });
        const page = await newPage(followed(2_000));
        await page.goto(`${base}/settings?section=updates`, { waitUntil: 'domcontentloaded' });
        await waitFor(page, () => !!document.querySelector('[aria-labelledby="system-update-operation-title"]'), 20000).catch(() => {});
        await pause(2500);
        // The Panel restarts: every API request loses its connection for 90 s.
        await ctl({ mode: 'down' });
        const down = Date.now();
        await pause(60_000);
        await snap(page, 'outage-60s', { outageSeconds: Math.round((Date.now() - down) / 1000), accessValidity: '45 s' });
        await pause(Math.max(0, 88_000 - (Date.now() - down)));
        await snap(page, 'outage-88s', { outageSeconds: Math.round((Date.now() - down) / 1000) });
        await pause(Math.max(0, 90_000 - (Date.now() - down)));
        // The Panel answers again, already the new version: "verifying".
        await ctl({ mode: 'ok', update: { request_id: UPDATE_ID, status: 'running', target: TARGET, phase: 'verifying' }, override: installed });
        await waitFor(page, () => /alpha\.86/.test(document.querySelector('[aria-labelledby="system-update-operation-title"]')?.innerText || '')
            && !/restart|yeniden başl/i.test(document.querySelector('[aria-labelledby="system-update-operation-title"]')?.innerText || ''), 20000).catch(() => {});
        await pause(1500);
        await snap(page, 'reconnected', { secondsSinceDown: Math.round((Date.now() - down) / 1000) });
        // The update completes: the page reloads once on the new version.
        await ctl({ update: { request_id: UPDATE_ID, status: 'succeeded', target: TARGET } });
        await waitFor(page, () => location.search.includes('_cp_update'), 30000).catch(() => {});
        await pause(4000);
        await snap(page, 'new-version');
        await closePage(page);
        await ctl({ clearAll: true });
    };
}
