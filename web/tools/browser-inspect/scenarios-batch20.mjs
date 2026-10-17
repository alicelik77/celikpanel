// Batch 20 (2026-10-17): the one ready-bundle list of alpha.95 (commit
// bd6b7399c, branch fix/alpha95-bundle-consistency), photographed against the
// owner's own screens of the installed servers: every Components category shows
// the same bundles in the same order and only the state on each chip changes,
// the Dashboard card is one summary line, rows carry no column of dashes, and
// the craft floor holds: one filled button per page, no amber on a healthy
// server, Turkish complete, no raw codes, no overflow at 390 px, 44 px targets
// on a phone.
//
//   y20boston         Components on a Boston-like PowerDNS secondary (Nginx,
//                     Node.js, PostgreSQL, Fail2ban, certbot, firewall engine,
//                     cron installed; PHP-FPM, MariaDB, mail not): strips
//                     Email Install x3, Databases "PostgreSQL verified,
//                     MariaDB Install, phpPgAdmin Add, phpMyAdmin needs
//                     MariaDB", Web "Nginx + PHP-FPM Install"; Roundcube not
//                     installed (no leftovers note: Boston has no record of an
//                     unfinished Roundcube install or removal);
//                     then the Roundcube cleanup dialogue and the PostgreSQL
//                     installed dialogue
//   y20bostondns      the same server with its DNS identity not ready: the
//                     email strip says the DNS line once
//   y20frankfurt      everything installed and verified like the owner's real
//                     Frankfurt: Core Mail included, Webmail and Spam-Protected
//                     Mail verified, both databases with their admin tool,
//                     Nginx + PHP-FPM, firewall active; then the Webmail chip's
//                     dialogue (Repair...)
//   y20running        phpPgAdmin being added on the Boston mock: the moving chip
//                     (step 2 of 2) and every other chip disabled
//   y20dashboard-boston, y20dashboard-frankfurt
//                     the Dashboard: the Ready bundles card ("1 installed and
//                     verified · 6 can be installed" / "8 installed and
//                     verified"), the attention-none sentence and, on
//                     Frankfurt, the Domains tile counting the infrastructure
//                     zone while /api/v1/domains is empty
//
// Every answer comes from the mock on 127.0.0.1 (mock.mjs and
// mock-batch20.mjs); nothing else is contacted, and the "installs" are the
// mock's own operations.
// Screenshots also go to BROWSER_INSPECT_SHOTS (default: the run's out dir) as
// <state>-<lang>-<theme>-<width>.png.
// On yirminci grup: her sunucuda tek paket listesi.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { batch20State } from './mock-batch20.mjs';

const TOKENS = /services\.bundles|dashboard\.bundles|firewall\.compact|mail_core|db_mariadb|web_nginx_php|component_requires|dns_identity|partly_present|\{[a-z]+\}/;
const ENGLISH = ['Ready bundles', 'installed and verified', 'not verified', 'Add spam protection', 'Open in Components', 'Firewall on', 'Stopped', 'installing', 'can be installed', 'Install', 'needs ', 'Clean up', 'Continue', 'Verify', 'Repair', 'included', 'leftovers'];
const WORDS = ['PHP-FPM', 'phpMyAdmin', 'phpPgAdmin', 'PostgreSQL', 'MariaDB', 'Fail2ban', 'Node.js', 'Nginx'];
const ZONES = [{ zone: 'celikhost.com', kind: 'infrastructure', records: 9, peer_records: 1 }];

const facts = (page, locale) => page.evaluate((tokens, english, locale, words) => {
    const visible = (node) => node.getClientRects().length > 0 && getComputedStyle(node).visibility !== 'hidden';
    const main = document.querySelector('main');
    const text = main ? main.innerText : '';
    const re = new RegExp(tokens);
    const probeColour = (cls, prop) => { const probe = document.createElement('span'); probe.className = cls; document.body.appendChild(probe); const colour = getComputedStyle(probe)[prop]; probe.remove(); return colour; };
    const primaryBg = probeColour('bg-primary', 'backgroundColor');
    const warning = probeColour('text-warning', 'color');
    const danger = probeColour('text-danger', 'color');
    const small = Array.from(document.querySelectorAll('main button, main a')).filter(visible).map((node) => ({ node, r: node.getBoundingClientRect() }))
        // A row's name link is stretched over the whole row by its ::after (the target is the row).
        .filter(({ node, r }) => window.innerWidth <= 639 && r.height < 43.5 && getComputedStyle(node, '::after').position !== 'absolute').map(({ node, r }) => `${(node.innerText || node.getAttribute('aria-label') || node.title || '').trim().slice(0, 40)} ${Math.round(r.height)}px`);
    // A word the browser broke across two lines (a hyphen or a mid-word wrap).
    const broken = [];
    if (main) {
        const walker = document.createTreeWalker(main, NodeFilter.SHOW_TEXT);
        for (let node = walker.nextNode(); node; node = walker.nextNode()) {
            const parent = node.parentElement;
            if (!parent || !visible(parent)) continue;
            for (const word of words) {
                let from = 0;
                for (let at = node.data.indexOf(word, from); at >= 0; at = node.data.indexOf(word, from)) {
                    from = at + word.length;
                    const range = document.createRange();
                    range.setStart(node, at); range.setEnd(node, at + word.length);
                    const tops = Array.from(range.getClientRects()).map((r) => Math.round(r.top));
                    if (tops.length > 1 && Math.max(...tops) - Math.min(...tops) > 4) broken.push(`${word} in "${node.data.trim().slice(0, 50)}"`);
                }
            }
        }
    }
    const chips = Array.from(document.querySelectorAll('[data-bundle-strip] [data-bundle]'));
    return {
        lang: document.documentElement.lang,
        strips: Array.from(document.querySelectorAll('[data-bundle-strip]')).map((node) => `${node.getAttribute('data-bundle-strip')}: ${node.innerText.replace(/\s+/g, ' ').trim()}`),
        chipStates: chips.map((node) => `${node.getAttribute('data-bundle')}=${node.getAttribute('data-bundle-state')}${node.disabled ? ' (disabled)' : ''}`),
        chipsOutside: chips.filter((node) => node.getBoundingClientRect().right > window.innerWidth + 0.5).map((node) => node.getAttribute('data-bundle')),
        brokenWords: broken.slice(0, 12),
        rowVersions: Array.from(document.querySelectorAll('[data-row-version]')).map((node) => node.innerText.trim()),
        rawTokens: text.split('\n').filter((line) => re.test(line)).map((line) => line.slice(0, 160)),
        englishLeaks: locale === 'tr' ? english.filter((item) => text.includes(item)) : [],
        filledButtons: Array.from(document.querySelectorAll('main button')).filter((node) => visible(node) && getComputedStyle(node).backgroundColor === primaryBg).map((node) => node.innerText.trim().slice(0, 40)),
        amberText: Array.from(document.querySelectorAll('main *')).filter((node) => visible(node) && node.children.length === 0 && getComputedStyle(node).color === warning && (node.innerText || '').trim()).map((node) => node.innerText.trim().slice(0, 60)),
        redText: Array.from(document.querySelectorAll('main *')).filter((node) => visible(node) && node.children.length === 0 && getComputedStyle(node).color === danger && (node.innerText || '').trim()).map((node) => node.innerText.trim().slice(0, 60)),
        smallTargets: small.slice(0, 15),
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
        mainOverflowX: main ? main.scrollWidth > main.clientWidth + 1 : false,
        dashboardCard: document.querySelector('[data-dashboard-bundles]')?.innerText.replace(/\s+/g, ' ').trim() ?? null,
        domainsTile: Array.from(document.querySelectorAll('main a[href="/domains"]')).map((node) => node.innerText.replace(/\s+/g, ' ').trim()).slice(0, 3),
    };
}, TOKENS.source, ENGLISH, locale, WORDS);

export default function batch20(scenarios, { base, locale, theme, out, viewport, ctl, reset, newPage, closePage, shot, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const snap = async (page, state, note = {}) => {
        await shot(page, `y20-${state}`, { ...(await facts(page, locale)), ...note });
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `y20-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    const whole = async (page, state, note = {}) => {
        const height = await page.evaluate(() => { const m = document.querySelector('main'); return m ? m.scrollHeight + (window.innerHeight - m.clientHeight) : document.body.scrollHeight; });
        await page.setViewport({ ...viewport, height: Math.min(Math.max(height, viewport.height), 6000) });
        await pause(500);
        await snap(page, state, { ...note, wholeHeight: height });
        await page.setViewport(viewport);
        await pause(300);
    };
    const clean = () => ctl({ clearAll: true, b19: null, b20: null, b12: null, b16: null, managedScan: null, componentOperation: null, domains: [], infrastructureZones: [] });
    // The catalogue view, every category open: the strips are drawn in it.
    const open = async (path) => {
        const page = await newPage({ 'celikpanel.components.view': 'catalog' });
        await page.goto(`${base}${path}`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page); await pause(700);
        return page;
    };
    const click = (page, selector) => page.evaluate((sel) => { const node = document.querySelector(sel); node?.scrollIntoView({ block: 'center' }); node?.click(); return !!node; }, selector);
    const closeDialog = async (page) => {
        // The confirmation of a destructive action ignores Escape; its Cancel button closes it.
        await page.keyboard.press('Escape');
        await pause(300);
        await page.evaluate(() => Array.from(document.querySelectorAll('[role="dialog"] button')).find((node) => ['cancel', 'iptal', 'close', 'kapat'].includes(node.innerText.trim().toLocaleLowerCase('tr').replace('i̇', 'i')))?.click());
        await pause(300);
    };

    const components = async (name, server, options, extra) => {
        await reset(); await clean();
        await ctl({ b20: batch20State(server, options) });
        const page = await open('/services');
        await waitFor(page, () => document.querySelectorAll('[data-bundle-strip]').length >= 3, 15000).catch(() => {});
        await whole(page, name);
        if (extra) await extra(page);
        await closePage(page);
        await clean();
    };

    scenarios.y20boston = () => components('components-boston', 'boston', {}, async (page) => {
        // Boston has no record of an unfinished Roundcube install or removal,
        // so its row offers no cleanup (alpha.95 review); Roundcube's own page
        // reaches the dialogue through /services?cleanup=roundcube.
        await page.goto(`${base}/services?cleanup=roundcube`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('[data-webmail-cleanup-list]', { timeout: 20000 });
        await pause(500);
        await snap(page, 'components-boston-leftovers');
        await closeDialog(page);
        // An installed chip: the PostgreSQL dialogue (Verify).
        await click(page, 'button[data-bundle="db.postgresql"]');
        await pause(500);
        await snap(page, 'components-boston-installed-dialog');
        await closeDialog(page);
    });

    scenarios.y20bostondns = () => components('components-boston-dns', 'boston', { dnsReady: false });

    scenarios.y20frankfurt = () => components('components-frankfurt', 'frankfurt', {}, async (page) => {
        await click(page, 'button[data-bundle="mail.webmail"]');
        await pause(500);
        await snap(page, 'components-frankfurt-webmail-dialog');
        await closeDialog(page);
    });

    scenarios.y20running = async () => {
        await reset(); await clean();
        await ctl({ b20: batch20State('boston', { hold: ['phppgadmin'] }) });
        const page = await open('/services');
        await waitFor(page, () => !!document.querySelector('button[data-bundle="db.postgresql.admin"]'), 15000).catch(() => {});
        await click(page, 'button[data-bundle="db.postgresql.admin"]');
        await pause(600);
        await snap(page, 'components-bundle-plan');
        await page.evaluate(() => { const dialog = document.querySelector('[role="dialog"]'); const buttons = dialog ? Array.from(dialog.querySelectorAll('button')) : []; buttons[buttons.length - 1]?.click(); });
        await waitFor(page, () => /2\/2/.test(document.querySelector('[data-bundle-running]')?.innerText || ''), 25000).catch(() => {});
        await pause(900);
        await page.evaluate(() => document.querySelector('[data-bundle-strip="database"]')?.scrollIntoView({ block: 'center' }));
        await pause(300);
        await snap(page, 'components-bundle-running');
        await closePage(page);
        await clean();
    };

    const dashboard = (name, server, extra = {}) => async () => {
        await reset(); await clean();
        await ctl({ b20: batch20State(server), ...extra });
        const page = await open('/');
        await waitFor(page, () => !!document.querySelector('[data-dashboard-bundles] [data-bundles-summary]'), 15000).catch(() => {});
        await pause(600);
        await whole(page, name);
        await closePage(page);
        await clean();
    };
    scenarios['y20dashboard-boston'] = dashboard('dashboard-boston', 'boston');
    scenarios['y20dashboard-frankfurt'] = dashboard('dashboard-frankfurt', 'frankfurt', { domains: [], infrastructureZones: ZONES });
}
