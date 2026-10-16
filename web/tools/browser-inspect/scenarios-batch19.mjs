// Batch 19 (2026-10-17): the ready bundles and the Components colour
// corrections of alpha.94 (branch feat/alpha94-ready-bundles; D-034),
// photographed against the owner's mockup (design94/mockup.html) and the
// craft floor: one filled button per page, no amber on a healthy server, a
// stopped service in red with its word, Turkish complete, no raw codes, no
// overflow at 390 px, 44 px targets on a phone, a wait that moves (D-033).
//
//   y19boston     Components on a server with nothing installed but its DNS
//                 engine: every category's strip offers its bundles
//   y19frankfurt  Components with webmail verified (one line, spam protection
//                 as the one add-on), MariaDB installed (phpMyAdmin offered),
//                 the firewall active (one line) and, as mock data, a stopped
//                 Fail2ban (red dot and "Stopped")
//   y19running    a database bundle being installed: step 2/3 (PHP-FPM) while
//                 the first component's operation has ended
//   y19dashboard  the Dashboard's Ready bundles card on the Frankfurt mock
//
// Every answer comes from the mock on 127.0.0.1 (mock.mjs and
// mock-batch19.mjs); nothing else is contacted, and the "installs" are the
// mock's own operations.
// Screenshots also go to BROWSER_INSPECT_SHOTS (default: the run's out dir) as
// <state>-<lang>-<theme>-<width>.png.
// On dokuzuncu grup: hazır paketler ve Bileşenler renk düzeltmeleri.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { batch19State } from './mock-batch19.mjs';

const TOKENS = /services\.bundles|dashboard\.bundles|firewall\.compact|mail_core|db_mariadb|web_nginx_php|\{[a-z]+\}/;
const ENGLISH = ['Ready bundles', 'installed and verified', 'not verified', 'Add spam protection', 'Add phpMyAdmin', 'Open in Components', 'Firewall on', 'Settings', 'Stopped', 'installing'];
const facts = (page, locale) => page.evaluate((tokens, english, locale) => {
    const visible = (node) => node.getClientRects().length > 0 && getComputedStyle(node).visibility !== 'hidden';
    const main = document.querySelector('main');
    const text = main ? main.innerText : '';
    const re = new RegExp(tokens);
    const primaryBg = (() => { const probe = document.createElement('span'); probe.className = 'bg-primary'; document.body.appendChild(probe); const colour = getComputedStyle(probe).backgroundColor; probe.remove(); return colour; })();
    const warning = (() => { const probe = document.createElement('span'); probe.className = 'text-warning'; document.body.appendChild(probe); const colour = getComputedStyle(probe).color; probe.remove(); return colour; })();
    const small = Array.from(document.querySelectorAll('main button, main a')).filter(visible).map((node) => ({ node, r: node.getBoundingClientRect() }))
        .filter(({ r }) => window.innerWidth <= 639 && r.height < 43.5).map(({ node, r }) => `${(node.innerText || node.getAttribute('aria-label') || node.title || '').trim().slice(0, 40)} ${Math.round(r.height)}px`);
    return {
        lang: document.documentElement.lang,
        strips: Array.from(document.querySelectorAll('[data-bundle-strip]')).map((node) => `${node.getAttribute('data-bundle-strip')}: ${node.innerText.replace(/\s+/g, ' ').trim()}`),
        rawTokens: text.split('\n').filter((line) => re.test(line)).map((line) => line.slice(0, 160)),
        englishLeaks: locale === 'tr' ? english.filter((item) => text.includes(item)) : [],
        filledButtons: Array.from(document.querySelectorAll('main button')).filter((node) => visible(node) && getComputedStyle(node).backgroundColor === primaryBg).map((node) => node.innerText.trim().slice(0, 40)),
        amberText: Array.from(document.querySelectorAll('main *')).filter((node) => visible(node) && node.children.length === 0 && getComputedStyle(node).color === warning && (node.innerText || '').trim()).map((node) => node.innerText.trim().slice(0, 60)),
        smallTargets: small.slice(0, 15),
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
        mainOverflowX: main ? main.scrollWidth > main.clientWidth + 1 : false,
    };
}, TOKENS.source, ENGLISH, locale);

export default function batch19(scenarios, { base, locale, theme, out, viewport, ctl, reset, newPage, closePage, shot, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const snap = async (page, state, note = {}) => {
        await shot(page, `y19-${state}`, { ...(await facts(page, locale)), ...note });
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `y19-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    const whole = async (page, state, note = {}) => {
        const height = await page.evaluate(() => { const m = document.querySelector('main'); return m ? m.scrollHeight + (window.innerHeight - m.clientHeight) : document.body.scrollHeight; });
        await page.setViewport({ ...viewport, height: Math.min(Math.max(height, viewport.height), 6000) });
        await pause(500);
        await snap(page, state, { ...note, wholeHeight: height });
        await page.setViewport(viewport);
        await pause(300);
    };
    const clean = () => ctl({ clearAll: true, b19: null, b12: null, b16: null, managedScan: null, componentOperation: null });
    // The catalogue view, every category open: the strips are drawn in it.
    const open = async (path) => {
        const page = await newPage({ 'celikpanel.components.view': 'catalog' });
        await page.goto(`${base}${path}`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page); await pause(700);
        return page;
    };

    scenarios.y19boston = async () => {
        await reset(); await clean();
        await ctl({ b19: batch19State('boston') });
        const page = await open('/services');
        await waitFor(page, () => document.querySelectorAll('[data-bundle-strip]').length >= 3, 15000).catch(() => {});
        await whole(page, 'components-boston');
        await closePage(page);
        await clean();
    };

    scenarios.y19frankfurt = async () => {
        await reset(); await clean();
        await ctl({ b19: batch19State('frankfurt') });
        const page = await open('/services');
        await waitFor(page, () => !!document.querySelector('[data-bundle="mail.webmail"]'), 15000).catch(() => {});
        await whole(page, 'components-frankfurt');
        // One offer opened: what it installs, Install… outlined.
        await page.evaluate(() => document.querySelector('button[data-bundle="db.mariadb.admin"]')?.click());
        await pause(400);
        await page.evaluate(() => document.querySelector('[data-bundle-strip="database"]')?.scrollIntoView({ block: 'center' }));
        await pause(300);
        await snap(page, 'components-frankfurt-offer');
        await closePage(page);
        await clean();
    };

    scenarios.y19running = async () => {
        await reset(); await clean();
        await ctl({ b19: batch19State('running', ['php-fpm']) });
        const page = await open('/services');
        await waitFor(page, () => !!document.querySelector('button[data-bundle="db.mariadb.admin"]'), 15000).catch(() => {});
        await page.evaluate(() => document.querySelector('button[data-bundle="db.mariadb.admin"]')?.click());
        await pause(300);
        await page.evaluate(() => Array.from(document.querySelectorAll('[data-bundle-strip="database"] button')).find((node) => node.innerText.includes('…'))?.click());
        await pause(500);
        await page.evaluate(() => { const dialog = document.querySelector('[role="dialog"]'); const buttons = dialog ? Array.from(dialog.querySelectorAll('button')) : []; buttons[buttons.length - 1]?.click(); });
        await waitFor(page, () => /2\/3/.test(document.querySelector('[data-bundle-running]')?.innerText || ''), 20000).catch(() => {});
        await pause(900);
        await snap(page, 'components-bundle-running');
        await closePage(page);
        await clean();
    };

    scenarios.y19dashboard = async () => {
        await reset(); await clean();
        await ctl({ b19: batch19State('frankfurt') });
        const page = await open('/');
        await waitFor(page, () => !!document.querySelector('[data-dashboard-bundles] li'), 15000).catch(() => {});
        await page.evaluate(() => document.querySelector('[data-dashboard-bundles]')?.scrollIntoView({ block: 'center' }));
        await pause(400);
        await snap(page, 'dashboard-bundles-card');
        await closePage(page);
        await clean();
    };
}
