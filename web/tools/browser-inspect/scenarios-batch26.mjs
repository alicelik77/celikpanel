// Batch 26 (2026-10-23): the update screen of the alpha.103 candidate, after
// two observations by the owner on installed servers running alpha.100.
//
//   y26stale    Frankfurt: the stored result names v0.1.0-alpha.101, found and
//               checked 13 hours ago; v0.1.0-alpha.102 has been published since.
//               Opening Settings -> CelikPanel updates checks again: first the
//               waiting line with the known offer kept and marked, Start held
//               (`settings-rechecking`), then the newest release
//               (`settings-newest`, and the sidebar link, on a phone with the
//               menu opened: `settings-newest-sidebar`)
//   y26stopped  Boston: the update to v0.1.0-alpha.102 stopped unchanged on a
//               busy package manager, and its notice is still open (seeded as
//               this browser's stored outcome, created 4 minutes and finished 2
//               minutes ago). The card shows the offer, the earlier attempt
//               with its cause and "still busy", Start waits for the notice to
//               be closed (`settings-stopped-busy`); after the package manager
//               is free and the card's own quiet re-read has run: "The package
//               manager has finished" (`settings-stopped-free`)
//
// The build the mock runs is v0.1.0-alpha.100; the releases are made up (see
// mock-batch26.mjs). Every answer comes from the mock on 127.0.0.1; nothing
// else is contacted, nothing is started or downloaded.
// Screenshots also go to BROWSER_INSPECT_SHOTS (default: the run's out dir) as
// <state>-<lang>-<theme>-<width>.png.
// Yirmi altinci grup: guncelleme ekrani acilinca yeniden denetim; durdurulan guncellemede dolu kart.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { B26_BUSY_SUMMARY, B26_CURRENT, B26_STOPPED_ID, B26_TARGETS, batch26State } from './mock-batch26.mjs';

const HOUR = 3600;
const ENGLISH = ['Update available', 'Checking celikpanel.net', 'Known before', 'Before you update', 'The server is ready', 'Check for updates', 'Technical details', 'Close the notice', 'package manager', 'stopped', 'Update to', 'ago', 'Close notification'];

const facts = (page, locale) => page.evaluate((english, locale) => {
    const visible = (node) => !!node && node.getClientRects().length > 0 && getComputedStyle(node).visibility !== 'hidden';
    const main = document.querySelector('main');
    const text = main ? main.innerText : '';
    const triple = (c) => (c.match(/\d+(\.\d+)?/g) || []).slice(0, 3).join(',');
    const rgbOf = (css) => { const probe = document.createElement('span'); probe.style.color = css; document.body.appendChild(probe); const c = getComputedStyle(probe).color; probe.remove(); return triple(c); };
    const primary = rgbOf('rgb(var(--primary))');
    const rect = (node) => { if (!visible(node)) return null; const r = node.getBoundingClientRect(); return { x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.width), h: Math.round(r.height) }; };
    const status = document.querySelector('[data-update-status]');
    const offer = document.querySelector('[data-update-offer]');
    const start = document.querySelector('#panel-update-start-button');
    const corner = document.querySelector('[aria-labelledby="system-update-background-title"]');
    const dialog = document.querySelector('[aria-labelledby="system-update-operation-title"]');
    const link = Array.from(document.querySelectorAll('[data-update-available]')).find(visible);
    const small = Array.from(document.querySelectorAll('main button, main a, main summary, main input')).filter(visible).map((node) => ({ node, r: node.getBoundingClientRect() }))
        .filter(({ node, r }) => window.innerWidth <= 639 && Math.max(r.height, node.closest('label')?.getBoundingClientRect().height || 0) < 43.5)
        .map(({ node, r }) => `${(node.innerText || node.getAttribute('aria-label') || node.type || '').trim().slice(0, 40)} ${Math.round(r.height)}px`);
    return {
        lang: document.documentElement.lang,
        mainText: text.replace(/\n{2,}/g, '\n').slice(0, 2400),
        rawKeys: text.split('\n').filter((line) => /panelUpdate\.|services\.mutationReadiness|recovery\.reason\./.test(line)).slice(0, 8),
        englishLeaks: locale === 'tr' ? english.filter((item) => text.includes(item)) : [],
        status: status ? { tone: status.getAttribute('data-update-status'), text: status.innerText.replace(/\s+/g, ' ').trim() } : null,
        offer: offer ? { rechecking: offer.hasAttribute('data-update-rechecking'), rect: rect(offer) } : null,
        start: start ? { text: start.innerText.trim(), disabled: start.disabled, rect: rect(start) } : null,
        filledButtons: Array.from(document.querySelectorAll('main button')).filter((node) => visible(node) && triple(getComputedStyle(node).backgroundColor) === primary).map((node) => node.innerText.trim().slice(0, 40)),
        previousAttempt: document.querySelector('[data-previous-attempt]')?.getAttribute('data-previous-attempt') ?? null,
        previousCause: document.querySelector('[data-previous-attempt-cause]')?.getAttribute('data-previous-attempt-cause') ?? null,
        packageManager: document.querySelector('[data-previous-attempt-package-manager]')?.getAttribute('data-previous-attempt-package-manager') ?? null,
        stoppedNoticeOpen: document.querySelector('[data-update-stopped-notice-open]')?.innerText ?? null,
        corner: visible(corner) ? { text: corner.innerText.replace(/\s+/g, ' ').trim().slice(0, 400), rect: rect(corner) } : null,
        dialog: visible(dialog) ? dialog.innerText.replace(/\s+/g, ' ').trim().slice(0, 400) : null,
        sidebarLink: link ? link.innerText.trim() : null,
        smallTargets: small.slice(0, 15),
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
        mainOverflowX: main ? main.scrollWidth > main.clientWidth + 1 : false,
    };
}, ENGLISH, locale);

export default function batch26(scenarios, { base, locale, theme, out, viewport, vp, ctl, reset, newPage, closePage, shot, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const phone = vp === 'phone';
    const snap = async (page, state, note = {}) => {
        await shot(page, `y26-${state}`, { ...(await facts(page, locale)), ...note });
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `y26-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    const whole = async (page, state, note = {}) => {
        const height = await page.evaluate(() => { const m = document.querySelector('main'); return m ? m.scrollHeight + (window.innerHeight - m.clientHeight) : document.body.scrollHeight; });
        await page.setViewport({ ...viewport, height: Math.min(Math.max(height, viewport.height), 6000) });
        await pause(500);
        await snap(page, state, { ...note, wholeHeight: height });
        await page.setViewport(viewport);
        await pause(300);
    };
    const sidebar = async (page, state) => {
        if (phone) {
            await page.click('button[aria-label="Menu"], button[aria-label="Menü"]').catch(() => {});
            await pause(700);
        }
        await snap(page, state);
        if (phone) { await page.keyboard.press('Escape'); await pause(400); }
    };
    const clean = () => ctl({ clearAll: true, b12: null, b16: null, b19: null, b20: null, b21: null, b26: null, managedScan: null, componentOperation: null, update: null, domains: [], infrastructureZones: [] });
    // `settle: false` does not wait for the network to be idle: the opening
    // check is on its way on purpose, and its wait is what is photographed.
    const open = async (path, storage = {}, settle = true) => {
        const page = await newPage(storage);
        await page.goto(`${base}${path}`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        if (settle) await quiet(page);
        return page;
    };

    scenarios.y26stale = async () => {
        await reset(); await clean();
        await ctl({ b26: batch26State({
            stored: { target: 'a101', checkedAge: 13 * HOUR, foundAge: 13 * HOUR, autoCheck: true },
            after: { target: 'a102', checkedAge: 0, foundAge: 0, autoCheck: true },
            refresh: 'checked', refreshDelay: 9000,
        }) });
        const page = await open('/settings?section=updates', {}, false);
        await waitFor(page, () => !!document.querySelector('[data-update-rechecking]'), 8000).catch(() => {});
        await pause(1200);
        await whole(page, 'settings-rechecking');
        await waitFor(page, () => { const b = document.querySelector('#panel-update-start-button'); return !!b && !b.disabled && !document.querySelector('[data-update-rechecking]'); }, 15000).catch(() => {});
        await pause(600);
        // The whole card first: a phone's menu, once opened, stays over it.
        await whole(page, 'settings-newest');
        await sidebar(page, 'settings-newest-sidebar');
        await closePage(page);
        await clean();
    };

    scenarios.y26stopped = async () => {
        await reset(); await clean();
        await ctl({ b26: batch26State({
            stored: { target: 'a102', checkedAge: 3 * HOUR, foundAge: 3 * HOUR, autoCheck: true },
            readiness: 'package_manager_active',
        }) });
        const now = Date.now();
        const record = {
            state_version: 1, phase: 'terminal',
            marker: { marker_version: 1, request_id: B26_STOPPED_ID, current_version: B26_CURRENT.version, current_commit: B26_CURRENT.commit, created_at: now - 240_000, target: B26_TARGETS.a102 },
            outcome: 'failed', message: B26_BUSY_SUMMARY, reload_scheduled: false, completed_at: now - 122_000,
        };
        const page = await open('/settings?section=updates', { 'celikpanel.system-update-operation.v1': JSON.stringify(record) });
        await waitFor(page, () => !!document.querySelector('[data-previous-attempt-package-manager]'), 12000).catch(() => {});
        await pause(900);
        await whole(page, 'settings-stopped-busy');
        // The package manager finishes; the card's own quiet re-read (every
        // 5 s while visible) says so. Nothing is started.
        await ctl({ b26: { ...batch26State({ stored: { target: 'a102', checkedAge: 3 * HOUR, foundAge: 3 * HOUR, autoCheck: true } }), readiness: 'ready' } });
        await waitFor(page, () => document.querySelector('[data-previous-attempt-package-manager]')?.getAttribute('data-previous-attempt-package-manager') === 'free', 15000).catch(() => {});
        await pause(600);
        await whole(page, 'settings-stopped-free');
        await closePage(page);
        await clean();
    };
}
