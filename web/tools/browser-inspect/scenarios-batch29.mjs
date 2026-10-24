// Batch 29 (2026-10-24): the v0.1.0-alpha.106 candidate (D-037 amendment:
// a removal says which version, and the dialog leads with the answer).
//
//   y29remove   a Frankfurt-shaped server (PHP-FPM 8.6, 8.5 and 8.4; phpPgAdmin
//               and Roundcube on 8.4, phpMyAdmin on 8.5): the PHP-FPM row's
//               remove button asks which version; "All versions" leads with
//               "can't be removed now", the causes once each and what to do,
//               Close beside a visibly unavailable "Can't remove"; PHP-FPM 8.4
//               chosen opens that version's own refusal
//
// Every answer comes from the mock on 127.0.0.1 through overrides (SAMPLE
// data shaped as the Panel answers); nothing is removed or contacted.
// Screenshots also go to BROWSER_INSPECT_SHOTS as <state>-<lang>-<theme>-<width>.png.
// Yirmi dokuzuncu grup: alpha.106 kaldırma penceresi; hepsi sahte.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';

const now = () => new Date().toISOString().replace(/\.\d{3}Z$/, 'Z');
const svc = (id, name, category, extra = {}) => ({
    id, name, description: '', icon: '', category, status: 'active (running)', is_installed: true,
    versions: [], packages: [id], repair_available: true, ...extra,
});
const instance = (version) => ({ version, unit: `php${version}-fpm`, managed: true, status: 'active (running)' });
const SERVICES = [
    svc('nginx', 'Nginx', 'web', { kind: 'service', unit: 'nginx' }),
    svc('php-fpm', 'PHP-FPM', 'web', { kind: 'runtime', versions: ['8.6', '8.5', '8.4'], instances: [instance('8.6'), instance('8.5'), instance('8.4')] }),
    svc('postgresql', 'PostgreSQL', 'database', { kind: 'service' }),
    svc('phppgadmin', 'phpPgAdmin', 'database', { kind: 'tool', status: 'installed', php_binding: { version: '8.4', socket_ready: true, version_installed: true } }),
    svc('phpmyadmin', 'phpMyAdmin', 'database', { kind: 'tool', status: 'installed', php_binding: { version: '8.5', socket_ready: true, version_installed: true } }),
    svc('postfix', 'Postfix', 'email', { kind: 'service' }),
    svc('roundcube', 'Roundcube', 'email', { kind: 'tool', status: 'installed', packages: [], php_binding: { version: '8.4', socket_ready: true, version_installed: true } }),
];
const SCAN = () => ({
    scanned_at: now(), scan_age_seconds: 0, dns_identity_ready: true,
    mail_hostname: { current: 'mail.example.net', hostname: 'mail.example.net', source: 'saved', current_usable: true, will_set_hostname: false },
    services: SERVICES, bundles: [],
    profiles: ['core-mail', 'webmail', 'protected-mail'].map((id) => ({ id, name: id, description: `${id} profile`, status: 'available', available: true, verified: false, latest_attempt_status: 'none', services: ['postfix'] })),
});
const PHP_TOOLS = {
    state: 'read',
    tools: [
        { tool: 'phpmyadmin', service_id: 'phpmyadmin', installed: true, version: '8.5', ready: true, version_installed: true },
        { tool: 'phppgadmin', service_id: 'phppgadmin', installed: true, version: '8.4', ready: true, version_installed: true },
        { tool: 'webmail', service_id: 'roundcube', installed: true, version: '8.4', ready: true, version_installed: true },
    ],
    versions: [
        { version: '8.6', sites: 0, site_names: [], tools: [] },
        { version: '8.5', sites: 1, site_names: ['shop.example (ali)'], tools: ['phpmyadmin'] },
        { version: '8.4', sites: 0, site_names: [], tools: ['phppgadmin', 'webmail'] },
    ],
};
const sim = (extra) => ({ state: 'checked', remove: [], collateral: [], bound_tools: [], default_runtime: false, refuses: false, ...extra });
const impact = (extra) => ({
    removal: 'packages', has_units: true, setup_required: false, package_family: 'apt', packages: ['php-fpm'], all_versions: false, firewall_ports: [],
    mail_filter_rewired: false, database_tools_reconfigured: false, required_by: [], bundles: [], hosted: null, ...extra,
});
// The owner's dialog on Frankfurt, 2026-10-24: the whole removal refused.
const WHOLE = impact({
    service_id: 'php-fpm', all_versions: true, required_by: ['phppgadmin', 'roundcube'],
    simulation: sim({ remove: Array.from({ length: 80 }, (_, i) => `php-package-${i + 1}`),
        collateral: [{ package: 'phppgadmin', reason: 'component', component: 'phppgadmin' }, { package: 'php-common', reason: 'would_install' }],
        bound_tools: ['phppgadmin', 'webmail'], refuses: true }),
});
const VERSION = impact({
    service_id: 'php-fpm', packages: ['php8.4-fpm'], package: 'php8.4-fpm', version: '8.4',
    hosted: { kind: 'php_sites', count: 0, names: [], blocks_removal: false },
    simulation: sim({ remove: ['php8.4-fpm', 'php8.4-cli', 'php8.4-common'], collateral: [
        { package: 'php-fpm', reason: 'component', component: 'php-fpm' },
        { package: 'php-pgsql', reason: 'reverse_dependency' },
    ], bound_tools: ['phppgadmin', 'webmail'], default_runtime: true, refuses: true }),
});

const facts = (page) => page.evaluate(() => {
    const visible = (node) => node.getClientRects().length > 0 && getComputedStyle(node).visibility !== 'hidden';
    const dialog = document.querySelector('[role="dialog"]');
    const raw = /\b(services|common)\.[a-zA-Z][a-zA-Z0-9_.]*\b|\{[a-zA-Z]+\}/;
    return dialog ? {
        text: dialog.innerText.split('\n').map((l) => l.trim()).filter(Boolean).slice(0, 40),
        rawKeys: dialog.innerText.split('\n').filter((l) => raw.test(l)).slice(0, 6),
        buttons: Array.from(dialog.querySelectorAll('button')).filter(visible).map((b) => `${b.innerText.trim()}${b.disabled ? ' [disabled]' : ''}`).slice(-3),
        overflowX: dialog.scrollWidth > dialog.clientWidth + 1,
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
    } : { dialog: null };
});

export default function batch29(scenarios, { base, locale, theme, out, viewport, ctl, reset, newPage, closePage, shot, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const snap = async (page, state) => {
        await shot(page, `y29-${state}`, await facts(page));
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `y29-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    const clean = () => ctl({ clearAll: true, b28: null, managedScan: null, componentOperation: null, domains: [], services: [] });
    const answers = (removal) => ctl({ override: {
        '/api/v1/managed-services': { status: 200, body: SCAN() },
        '/api/v1/managed-services/scan': { status: 200, body: SCAN() },
        '/api/v1/service/php-tools': { status: 200, body: PHP_TOOLS },
        '/api/v1/service/removal-impact': { status: 200, body: removal },
    } });
    const pressRemove = (page) => page.evaluate(() => {
        const row = Array.from(document.querySelectorAll('main li')).find((li) => /PHP-FPM/.test(li.innerText) && li.querySelector('button[title]'));
        const button = row && Array.from(row.querySelectorAll('button[title]')).find((b) => /Uninstall|Kaldır/.test(b.getAttribute('title') || ''));
        button?.click();
        return !!button;
    });

    scenarios.y29remove = async () => {
        await reset(); await clean();
        await answers(WHOLE);
        const page = await newPage({ 'celikpanel.components.view': 'installed' });
        await page.goto(`${base}/services`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page);
        await pause(800);
        await pressRemove(page);
        await waitFor(page, () => !!document.querySelector('[data-remove-which] [data-version-usage]'), 8000).catch(() => {});
        await pause(300);
        await snap(page, 'remove-which');
        await page.evaluate(() => document.querySelector('[data-remove-which-all]')?.click());
        await waitFor(page, () => !!document.querySelector('[data-removal-verdict]'), 8000).catch(() => {});
        await pause(400);
        await snap(page, 'remove-all-refused');
        await page.evaluate(() => document.querySelector('[data-removal-close]')?.click());
        await pause(300);
        await answers(VERSION);
        await pressRemove(page);
        await waitFor(page, () => !!document.querySelector('[data-remove-which-version="8.4"]'), 8000).catch(() => {});
        await page.evaluate(() => document.querySelector('[data-remove-which-version="8.4"]')?.click());
        await waitFor(page, () => !!document.querySelector('[data-removal-verdict]'), 8000).catch(() => {});
        await pause(400);
        await snap(page, 'remove-version-refused');
        await closePage(page);
        await clean();
    };
}
