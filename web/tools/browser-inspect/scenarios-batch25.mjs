// Batch 25 (2026-10-22): the v0.1.0-alpha.100 candidate (D-037 amendment).
// The removal dialog says what a removal affects on this server, by name, and
// what it removes and keeps; the first Components visit opens the installed
// view; Server setup has its own page under Server, directly under Components.
//
//   y25firstvisit  /services in a fresh browser (no remembered view: the
//                  installed view), then with a remembered "catalog"
//   y25removal     the removal dialog for Nginx, PostgreSQL, Postfix,
//                  certbot and the firewall engine (Frankfurt-shaped server), Nginx while the read is
//                  held open, and Nginx when the read fails
//   y25setup       /server-setup as administrator, the rail with its new item,
//                  /settings?section=setup (replaced by /server-setup),
//                  /settings with three sections, and the Dashboard's purpose
//                  card with its link
//
// Every answer comes from the mock on 127.0.0.1 (mock.mjs, mock-batch20.mjs);
// /api/v1/service/removal-impact is a constant set through the mock's
// overrides (SAMPLE data, not measured), so no new mock file is needed.
// Nothing is installed, removed or contacted. Screenshots also go to
// BROWSER_INSPECT_SHOTS (default: the run's out dir) as
// <state>-<lang>-<theme>-<width>.png.
// Yirmi besinci grup: alpha.100 kaldirma penceresi, ilk ziyaret, Sunucu kurulumu.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { batch20State } from './mock-batch20.mjs';
import { batch21State } from './mock-batch21.mjs';

const IMPACT = '/api/v1/service/removal-impact';
const base0 = { removal: 'packages', has_units: true, setup_required: false, package_family: 'apt', packages: [], all_versions: false, firewall_ports: [], mail_filter_rewired: false, database_tools_reconfigured: false, required_by: [], bundles: [], hosted: null };
// Sample answers, shaped as cmd/panel/service_removal_impact.go answers.
const IMPACTS = {
    nginx: { ...base0, service_id: 'nginx', packages: ['nginx'], firewall_ports: ['80/tcp', '443/tcp'], required_by: ['phpmyadmin', 'phppgadmin', 'roundcube'],
        bundles: [{ id: 'mail.webmail', name_code: 'mail_webmail' }, { id: 'db.postgresql.admin', name_code: 'db_postgresql_admin' }, { id: 'db.mariadb.admin', name_code: 'db_mariadb_admin' }, { id: 'web.nginx_php', name_code: 'web_nginx_php' }],
        hosted: { kind: 'sites', count: 8, names: ['blog.example.com (ali)', 'shop.example.com (ali)', 'docs.example.net (veli)', 'api.example.net (veli)', 'www.example.org (ayse)', 'old.example.org (ayse)', 'mail.example.com (ali)', 'test.example.com (ali)'], blocks_removal: true } },
    postgresql: { ...base0, service_id: 'postgresql', packages: ['postgresql'], all_versions: true, required_by: ['phppgadmin'],
        bundles: [{ id: 'db.postgresql', name_code: 'db_postgresql' }], hosted: { kind: 'databases', count: 0, names: [], blocks_removal: false } },
    postfix: { ...base0, service_id: 'postfix', packages: ['postfix'], firewall_ports: ['25/tcp', '465/tcp', '587/tcp'], required_by: ['rspamd', 'roundcube'],
        bundles: [{ id: 'mail.core', name_code: 'mail_core' }, { id: 'mail.webmail', name_code: 'mail_webmail' }, { id: 'mail.protected', name_code: 'mail_protected' }],
        hosted: { kind: 'mail_addresses', count: 3, names: ['info@example.com', 'sales@example.com', 'hello@example.net'], blocks_removal: true } },
    certbot: { ...base0, service_id: 'certbot', packages: ['certbot'], has_units: false, setup_required: true,
        hosted: { kind: 'certificates', count: 4, names: ['panel.example.com', 'mail.example.com', 'blog.example.com', 'shop.example.com'], blocks_removal: false } },
    nftables: { ...base0, service_id: 'nftables', packages: ['nftables'], has_units: false, setup_required: true,
        hosted: { kind: 'firewall', count: 0, names: [], blocks_removal: false, firewall: { known: true, enabled: true, open_ports: 4 } } },
};
const NAMES = { nginx: 'Nginx', postgresql: 'PostgreSQL', postfix: 'Postfix', certbot: 'certbot', nftables: 'nftables' };
const RAW = /\b(services|nav|setup|settings|dashboard|common|err)\.[a-zA-Z][a-zA-Z0-9_.]*\b|\{[a-zA-Z]+\}/;
const ENGLISH = ['What is removed', 'Affected on this server', 'What stays', 'Nothing else on this server', 'more', 'Server setup', 'Uninstall', 'Cancel', 'Retry', 'could not be read', 'Reading what'];

const facts = (page, locale) => page.evaluate((rawSource, english, locale) => {
    const visible = (node) => node.getClientRects().length > 0 && getComputedStyle(node).visibility !== 'hidden';
    const rect = (node) => { if (!node) return null; const r = node.getBoundingClientRect(); return { x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.width), h: Math.round(r.height) }; };
    const body = document.body.innerText || '';
    const lines = body.split('\n').map((l) => l.trim()).filter(Boolean);
    const re = new RegExp(rawSource);
    const dialog = document.querySelector('[role="dialog"]');
    const rail = document.querySelector('aside nav, nav.flex-1');
    const small = Array.from(document.querySelectorAll('[role="dialog"] button, main button, nav a')).filter(visible)
        .map((node) => ({ node, r: node.getBoundingClientRect() }))
        .filter(({ r }) => window.innerWidth <= 639 && r.height < 43.5)
        .map(({ node, r }) => `${(node.innerText || node.getAttribute('aria-label') || '').trim().slice(0, 40)} ${Math.round(r.height)}px`);
    return {
        lang: document.documentElement.lang,
        location: { path: location.pathname, search: location.search, historyLength: history.length },
        rawKeys: lines.filter((line) => re.test(line)).slice(0, 8),
        englishLeaks: locale === 'tr' ? english.filter((item) => new RegExp(`\\b${item}\\b`).test(body)) : [],
        view: document.querySelector('[aria-pressed="true"]')?.innerText ?? null,
        rows: Array.from(document.querySelectorAll('main li')).filter(visible).length,
        dialog: dialog ? {
            rect: rect(dialog),
            state: dialog.querySelector('[data-removal-impact]')?.getAttribute('data-removal-impact') ?? null,
            lines: Array.from(dialog.querySelectorAll('[data-removal-line]')).map((n) => `${n.getAttribute('data-removal-line')}: ${n.innerText.replace(/\s+/g, ' ').trim()}`),
            sentences: Array.from(dialog.querySelectorAll('[data-removal-sentence]')).map((n) => n.getAttribute('data-removal-sentence')),
            nothing: !!dialog.querySelector('[data-removal-nothing]'),
            command: dialog.querySelector('[data-removal-command]')?.innerText ?? null,
            scrolls: dialog.scrollHeight > dialog.clientHeight + 1,
            overflowX: dialog.scrollWidth > dialog.clientWidth + 1,
            removeButton: Array.from(dialog.querySelectorAll('button')).filter(visible).map((b) => `${b.innerText.trim()}${b.disabled ? ' [disabled]' : ''} ${getComputedStyle(b).color}/${getComputedStyle(b).borderColor}`).slice(-2),
        } : null,
        rail: rail ? Array.from(rail.querySelectorAll('a')).filter(visible).map((a) => `${a.innerText.replace(/\s+/g, ' ').trim()} -> ${a.getAttribute('href')}${a.getAttribute('aria-current') ? ' [current]' : ''}`) : [],
        tabs: Array.from(document.querySelectorAll('[role="tab"]')).filter(visible).map((n) => n.innerText.replace(/\s+/g, ' ').trim().slice(0, 60)),
        h1: Array.from(document.querySelectorAll('h1')).filter(visible).map((n) => n.innerText.trim()),
        purposeLinks: Array.from(document.querySelectorAll('main a')).filter((a) => visible(a) && (a.getAttribute('href') || '').includes('server-setup')).map((a) => `${a.innerText.replace(/\s+/g, ' ').trim()} -> ${a.getAttribute('href')}`),
        smallTargets: small.slice(0, 12),
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
    };
}, RAW.source, ENGLISH, locale);

export default function batch25(scenarios, { base, locale, theme, out, viewport, vp, ctl, reset, newPage, closePage, shot, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const phone = vp === 'phone';
    const snap = async (page, state, note = {}) => {
        await shot(page, `y25-${state}`, { ...(await facts(page, locale)), ...note });
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `y25-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    const whole = async (page, state, note = {}) => {
        const height = await page.evaluate(() => { const m = document.querySelector('main'); return m ? m.scrollHeight + (window.innerHeight - m.clientHeight) : document.body.scrollHeight; });
        await page.setViewport({ ...viewport, height: Math.min(Math.max(height, viewport.height), 6000) });
        await pause(500);
        await snap(page, state, { ...note, wholeHeight: height });
        await page.setViewport(viewport);
        await pause(300);
    };
    const clean = () => ctl({ clearAll: true, b3: null, b12: null, b16: null, b19: null, b20: null, b21: null, b22: null, managedScan: null, componentOperation: null, domains: [], infrastructureZones: [], twoFactor: false, services: [] });
    const open = async (path, ready, storage = {}) => {
        const page = await newPage(storage);
        await page.goto(`${base}${path}`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page);
        if (ready) await waitFor(page, ready, 15000).catch(() => {});
        await pause(900);
        return page;
    };
    const rowsReady = () => document.querySelectorAll('main li').length >= 4;
    const openRemoval = async (page, name) => {
        const clicked = await page.evaluate((name) => {
            const buttons = Array.from(document.querySelectorAll('main li button[title]')).filter((b) => b.getAttribute('title') === b.getAttribute('aria-label'));
            for (const button of buttons) {
                const li = button.closest('li');
                const [first = '', second = ''] = (li?.innerText || '').split('\n');
                // The firewall engine's name is translated; its description names nftables.
                if ((first.includes(name) || (name === 'nftables' && second.includes(name))) &&/Uninstall|Kaldır/.test(button.getAttribute('title'))) { button.click(); return true; }
            }
            return false;
        }, name);
        await pause(300);
        return clicked;
    };

    scenarios.y25firstvisit = async () => {
        await reset(); await clean();
        await ctl({ b20: batch20State('frankfurt', {}), b21: batch21State({ outcome: 'current', automatic: true }) });
        let page = await open('/services', rowsReady);
        await whole(page, 'components-first-visit');
        await closePage(page);
        page = await open('/services', rowsReady, { 'celikpanel.components.view': 'catalog' });
        await whole(page, 'components-stored-catalog');
        await closePage(page);
        await clean();
    };

    scenarios.y25removal = async () => {
        for (const id of ['nginx', 'postgresql', 'postfix', 'certbot', 'nftables']) {
            await reset(); await clean();
            await ctl({ b20: batch20State('frankfurt', {}), override: { [IMPACT]: { status: 200, body: IMPACTS[id] } } });
            const page = await open('/services', rowsReady);
            const clicked = await openRemoval(page, NAMES[id]);
            await waitFor(page, () => !!document.querySelector('[data-removal-impact="known"]'), 8000).catch(() => {});
            await pause(400);
            await snap(page, `removal-${id}`, { clicked });
            await closePage(page);
        }
        await reset(); await clean();
        await ctl({ b20: batch20State('frankfurt', {}), override: { [IMPACT]: { hang: true } } });
        let page = await open('/services', rowsReady);
        await openRemoval(page, 'Nginx');
        await pause(800);
        await snap(page, 'removal-nginx-checking');
        await closePage(page);
        await reset(); await clean();
        await ctl({ b20: batch20State('frankfurt', {}), override: { [IMPACT]: { status: 503, body: { error: 'state unverified', code: 'SERVICE_STATE_UNVERIFIED' } } } });
        page = await open('/services', rowsReady);
        await openRemoval(page, 'Nginx');
        await waitFor(page, () => !!document.querySelector('[data-removal-impact="unknown"]'), 8000).catch(() => {});
        await pause(400);
        await snap(page, 'removal-nginx-unknown');
        await closePage(page);
        await clean();
    };

    scenarios.y25setup = async () => {
        await reset(); await clean();
        await ctl({ b20: batch20State('frankfurt', {}), b21: batch21State({ outcome: 'current', automatic: true }) });
        let page = await open('/server-setup', () => !!document.querySelector('main h1'));
        await whole(page, 'server-setup-page');
        if (phone) {
            await page.click('button[aria-label="Menu"], button[aria-label="Menü"]').catch(() => {});
            await pause(700);
        }
        await snap(page, 'server-setup-sidebar');
        await closePage(page);
        page = await open('/settings?section=setup', () => location.pathname !== '/settings');
        await pause(600);
        await snap(page, 'settings-section-setup-redirect');
        await closePage(page);
        page = await open('/settings', () => document.querySelectorAll('[role="tab"]').length > 0);
        await snap(page, 'settings-three-sections');
        await closePage(page);
        page = await open('/', () => document.querySelectorAll('main a[href="/server-setup"]').length > 0);
        await whole(page, 'dashboard-purpose-card');
        await closePage(page);
        await clean();
    };
}
