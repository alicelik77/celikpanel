// Batch 21 (2026-10-17): the Panel's stored update check of the alpha.96
// candidate (D-035): the update card that already knows the last result, the
// sidebar link, the Dashboard's one quiet row and the Settings item. Against the
// owner's Frankfurt screen of alpha.95 (two amber boxes, a green line, the same
// "A signed update is available." twice) the craft floor is: one status line,
// at most one neutral notice, ONE filled button, no amber/red/green on an
// available update, a failed automatic check shown only on the card, Turkish
// complete, no raw keys, no overflow at 390 px, 44 px targets on a phone.
//
//   y21current    Settings -> CelikPanel updates, stored result "current"
//                 (automatic, checked 2 hours ago), automatic checks on
//   y21available  the same card with v0.1.0-alpha.96 available (found 2 hours
//                 ago), the server ready: one status line, "Before you update",
//                 the readiness box, ONE filled button, collapsed Technical
//                 details; the sidebar link visible (on a phone with the menu
//                 opened)
//   y21stopped    available, and an earlier attempt to that version stopped
//                 before changing anything (failed, recovery_runtime_preflight_
//                 failed): ONE neutral notice, no amber
//   y21failed     the stored result is a failed automatic check
//                 (release_unverified), no release found: the calm "Last
//                 automatic check..." line and the next check time on the card;
//                 the Dashboard and the sidebar say nothing about updates
//   y21off        automatic checks switched off, stored result current: the card
//                 says so, the switch is unchecked
//   y21dashboard  the Dashboard with an update available: the quiet row at the
//                 top with "Review and update" and "Remind me later"; then after
//                 pressing "Remind me later": the row is gone, the sidebar link
//                 stays
//
// The build the mock runs is v0.1.0-alpha.95 (commit 5f3cb1a2...), the found
// release v0.1.0-alpha.96 (made-up commit and SHA-256), expected restart 90 s,
// checked 2 hours ago, next check in 10 hours. Every answer comes from the
// mock on 127.0.0.1 (mock.mjs and mock-batch21.mjs); nothing else is
// contacted, and nothing is started or downloaded.
// Screenshots also go to BROWSER_INSPECT_SHOTS (default: the run's out dir) as
// <state>-<lang>-<theme>-<width>.png.
// On yirmi birinci grup: alpha.96 saklanan guncelleme kontrolu.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { batch21State } from './mock-batch21.mjs';

const STOPPED = { request_id: 'c'.repeat(32), phase: 'failed', failure_code: 'recovery_runtime_preflight_failed', finished_at: '2026-10-17T09:15:42Z' };
const ENGLISH = ['Update available', 'You are on the latest', 'Before you update', 'The server is ready', 'Check for updates', 'Technical details', 'Remind me later', 'Review and update', 'Automatic checks', 'Last automatic check', 'next check', 'Update to', 'checked', 'ago', 'has stopped', 'stopped'];

const facts = (page, locale) => page.evaluate((english, locale) => {
    const visible = (node) => node.getClientRects().length > 0 && getComputedStyle(node).visibility !== 'hidden';
    const main = document.querySelector('main');
    const text = main ? main.innerText : '';
    const triple = (c) => (c.match(/\d+(\.\d+)?/g) || []).slice(0, 3).join(',');
    const rgbOf = (css) => { const probe = document.createElement('span'); probe.style.color = css; document.body.appendChild(probe); const c = getComputedStyle(probe).color; probe.remove(); return triple(c); };
    const tokens = { amber: rgbOf('rgb(var(--warning))'), amberMark: rgbOf('rgb(var(--warning-mark))'), red: rgbOf('rgb(var(--danger))'), green: rgbOf('rgb(var(--success))'), primary: rgbOf('rgb(var(--primary))') };
    const label = (node) => `${node.tagName.toLowerCase()}${node.getAttribute('data-message-tone') ? '[tone]' : ''}: ${(node.innerText || node.getAttribute('aria-label') || '').trim().replace(/\s+/g, ' ').slice(0, 50)}`;
    const found = { amber: [], red: [], green: [] };
    const scope = Array.from(document.querySelectorAll('main *, [data-update-available], [data-update-available] *'));
    for (const node of scope) {
        if (!visible(node)) continue;
        const style = getComputedStyle(node);
        const props = [style.color, style.backgroundColor, style.borderTopColor, style.borderLeftColor, style.fill, style.stroke];
        const own = (node.children.length === 0 && (node.innerText || '').trim()) || node.tagName === 'svg' || node.tagName === 'path' || style.backgroundColor !== 'rgba(0, 0, 0, 0)' || parseFloat(style.borderTopWidth) > 0;
        if (!own) continue;
        const set = new Set(props.filter((c) => c && c !== 'none').map(triple));
        for (const [name, key] of [['amber', 'amber'], ['amber', 'amberMark'], ['red', 'red'], ['green', 'green']]) if (set.has(tokens[key]) && found[name].length < 12) found[name].push(label(node));
    }
    const small = Array.from(document.querySelectorAll('main button, main a, main summary, main input, [data-update-available]')).filter(visible).map((node) => ({ node, r: node.getBoundingClientRect() }))
        .filter(({ node, r }) => window.innerWidth <= 639 && Math.max(r.height, node.closest('label')?.getBoundingClientRect().height || 0) < 43.5 && getComputedStyle(node, '::after').position !== 'absolute')
        .map(({ node, r }) => `${(node.innerText || node.getAttribute('aria-label') || node.type || '').trim().slice(0, 40)} ${Math.round(r.height)}px`);
    const rect = (node) => { if (!node) return null; const r = node.getBoundingClientRect(); return { x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.width), h: Math.round(r.height) }; };
    const row = document.querySelector('[data-dashboard-update]');
    const link = Array.from(document.querySelectorAll('[data-update-available]')).find(visible);
    const sw = document.querySelector('[data-update-auto] input');
    return {
        lang: document.documentElement.lang,
        innerWidth: window.innerWidth,
        // Elements drawn past the window's right edge (a phone is laid out at 390 px; a wider layout window means something is wider).
        pastRight: Array.from(document.querySelectorAll('body *')).filter((n) => { const r = n.getBoundingClientRect(); return visible(n) && r.width > 0 && r.right > Math.min(window.innerWidth, 392) + 1 && r.right > 392; }).slice(0, 10).map((n) => `${n.tagName.toLowerCase()}.${String(n.className?.baseVal ?? n.className).slice(0, 60)} right=${Math.round(n.getBoundingClientRect().right)} w=${Math.round(n.getBoundingClientRect().width)}: ${(n.innerText || '').trim().replace(/s+/g, ' ').slice(0, 30)}`),
        mainText: text.replace(/\n{2,}/g, '\n').slice(0, 1800),
        rawKeys: text.split('\n').filter((line) => /panelUpdate\.|settings\.section|services\.mutationReadiness|recovery\.reason\./.test(line)).slice(0, 8),
        englishLeaks: locale === 'tr' ? english.filter((item) => text.includes(item)) : [],
        filledButtons: Array.from(document.querySelectorAll('main button')).filter((node) => visible(node) && triple(getComputedStyle(node).backgroundColor) === tokens.primary).map((node) => node.innerText.trim().slice(0, 40)),
        amberUsed: found.amber, redUsed: found.red, greenUsed: found.green,
        smallTargets: small.slice(0, 15),
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
        mainOverflowX: main ? main.scrollWidth > main.clientWidth + 1 : false,
        technicalOpen: document.querySelector('[data-update-technical]')?.open ?? null,
        previousAttempt: document.querySelector('[data-previous-attempt]')?.getAttribute('data-previous-attempt') ?? null,
        switch: sw ? { checked: sw.checked, disabled: sw.disabled, rect: rect(sw) } : null,
        sidebarLink: link ? { text: link.innerText.trim(), rect: rect(link), colour: getComputedStyle(link).color } : null,
        sidebarLinkCount: document.querySelectorAll('[data-update-available]').length,
        dashboardRow: row ? { text: row.innerText.replace(/\s+/g, ' ').trim(), rect: rect(row), mainRect: rect(main), buttons: Array.from(row.querySelectorAll('a, button')).map((n) => ({ text: n.innerText.trim(), rect: rect(n) })) } : null,
        settingsItems: Array.from(document.querySelectorAll('[role="tab"], nav a, nav button')).filter(visible).map((n) => n.innerText.replace(/\s+/g, ' ').trim()).filter((t) => /update|güncelleme/i.test(t)).slice(0, 4),
    };
}, ENGLISH, locale);

export default function batch21(scenarios, { base, locale, theme, out, viewport, vp, ctl, reset, newPage, closePage, shot, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const phone = vp === 'phone';
    const snap = async (page, state, note = {}) => {
        await shot(page, `y21-${state}`, { ...(await facts(page, locale)), ...note });
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `y21-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    // The content as tall as <main>, so the whole card is in the picture.
    const whole = async (page, state, note = {}) => {
        const height = await page.evaluate(() => { const m = document.querySelector('main'); return m ? m.scrollHeight + (window.innerHeight - m.clientHeight) : document.body.scrollHeight; });
        await page.setViewport({ ...viewport, height: Math.min(Math.max(height, viewport.height), 6000) });
        await pause(500);
        await snap(page, state, { ...note, wholeHeight: height });
        await page.setViewport(viewport);
        await pause(300);
    };
    // On a phone the sidebar is behind the menu button; on a wide screen it is there already.
    const sidebar = async (page, state, note = {}) => {
        if (phone) {
            // The button is named 'Menu' in English and 'Menü' in Turkish.
            await page.click('button[aria-label="Menu"], button[aria-label="Menü"]').catch(() => {});
            await pause(700);
        }
        await snap(page, state, note);
        if (phone) {
            await page.keyboard.press('Escape');
            await pause(400);
        }
    };
    const clean = () => ctl({ clearAll: true, b12: null, b16: null, b19: null, b20: null, b21: null, managedScan: null, componentOperation: null, domains: [], infrastructureZones: [] });
    const open = async (path, waitSelector) => {
        const page = await newPage();
        await page.goto(`${base}${path}`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page);
        if (waitSelector) await waitFor(page, (sel) => !!document.querySelector(sel), 15000, waitSelector).catch(() => {});
        await pause(900);
        return page;
    };

    const card = (name, options, { waitSelector = '[data-update-auto]', menu = false } = {}) => async () => {
        await reset(); await clean();
        await ctl({ b21: batch21State(options) });
        const page = await open('/settings?section=updates', waitSelector);
        // The readiness read of an offered update comes by itself; give it a moment.
        if (options.target) await waitFor(page, () => { const b = document.querySelector('#panel-update-start-button'); return !!b && !b.disabled; }, 8000).catch(() => {});
        // The menu is photographed first, at the window's own size: a window made as tall as <main> for the whole-card picture leaves a phone's drawer that tall.
        if (menu) await sidebar(page, `${name}-sidebar`);
        await whole(page, name);
        await closePage(page);
        await clean();
    };

    scenarios.y21current = card('settings-current', { outcome: 'current', automatic: true });
    scenarios.y21available = card('settings-available', { outcome: 'available', target: true }, { waitSelector: '#panel-update-start-button', menu: true });
    scenarios.y21stopped = card('settings-stopped', { outcome: 'available', target: true, previous: STOPPED }, { waitSelector: '#panel-update-start-button' });
    scenarios.y21off = card('settings-off', { outcome: 'current', automatic: true, autoCheck: false });

    scenarios.y21failed = async () => {
        await reset(); await clean();
        await ctl({ b21: batch21State({ outcome: 'failed', reason: 'release_unverified', automatic: true, target: false }) });
        const page = await open('/settings?section=updates', '[data-update-auto]');
        await whole(page, 'settings-failed');
        await closePage(page);
        // The Dashboard and the sidebar say nothing about updates.
        const dash = await open('/', null);
        await sidebar(dash, 'dashboard-failed-sidebar');
        await whole(dash, 'dashboard-failed');
        await closePage(dash);
        await clean();
    };

    scenarios.y21dashboard = async () => {
        await reset(); await clean();
        await ctl({ b21: batch21State({ outcome: 'available', target: true }) });
        const page = await open('/', '[data-dashboard-update]');
        await whole(page, 'dashboard-available');
        await snap(page, 'dashboard-available-top');
        if (phone) await sidebar(page, 'dashboard-available-sidebar');
        await page.evaluate(() => document.querySelector('[data-update-later]')?.click());
        await pause(700);
        await snap(page, 'dashboard-later-top');
        await sidebar(page, 'dashboard-later-sidebar');
        await closePage(page);
        await clean();
    };
}
