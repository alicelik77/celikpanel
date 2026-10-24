// Batch 28 (2026-10-24): the v0.1.0-alpha.105 candidate (D-034 amendment:
// recommend, warn, never impose).
//
//   y28bundle   the PostgreSQL + phpPgAdmin bundle dialogue on a Boston-shaped
//               server (PostgreSQL and Nginx installed): each part's version,
//               the distribution's marked recommended, Change open on PHP-FPM;
//               PHP 8.5 chosen (a warning for the distribution tool); notable
//               installs named and Install held until confirmed; PHP 8.6
//               refused for lack of php8.6-pgsql
//
// Every answer comes from the mock on 127.0.0.1 (mock-batch28.mjs, SAMPLE
// data); nothing is installed or contacted. Screenshots also go to
// BROWSER_INSPECT_SHOTS as <state>-<lang>-<theme>-<width>.png.
// Yirmi sekizinci grup: alpha.105 paket penceresi.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { withSamplePackageVersions } from './sample-package-versions.mjs';

const now = () => new Date().toISOString().replace(/\.\d{3}Z$/, 'Z');
const svc = (id, name, category, installed, extra = {}) => ({
    id, name, description: '', icon: '', category, status: installed ? 'active (running)' : 'not_installed', is_installed: installed,
    versions: [], packages: [id], repair_available: true, ...extra,
});
const SERVICES = [
    withSamplePackageVersions(svc('nginx', 'Nginx', 'web', true, { kind: 'service', unit: 'nginx' }), 'debian13'),
    svc('php-fpm', 'PHP-FPM', 'web', false, { kind: 'runtime', packages: ['php-fpm'] }),
    withSamplePackageVersions(svc('postgresql', 'PostgreSQL', 'database', true, { kind: 'service', unit: 'postgresql' }), 'debian13'),
    svc('mariadb', 'MariaDB', 'database', false, { kind: 'service', packages: ['mariadb-server'] }),
    svc('phppgadmin', 'phpPgAdmin', 'database', false, { kind: 'tool', status: 'not_installed' }),
    svc('phpmyadmin', 'phpMyAdmin', 'database', false, { kind: 'tool', status: 'not_installed' }),
    svc('postfix', 'Postfix', 'email', false, { kind: 'service' }),
    svc('dovecot', 'Dovecot', 'email', false, { kind: 'service', packages: ['dovecot-imapd'] }),
    svc('rspamd', 'Rspamd', 'email', false, { kind: 'service' }),
    svc('roundcube', 'Roundcube', 'email', false, { kind: 'tool', packages: [] }),
];
const BUNDLES = [
    { id: 'db.postgresql', category: 'database', name_code: 'db_postgresql_phppgadmin', components: ['postgresql', 'nginx', 'php-fpm', 'phppgadmin'], status: 'available', action: 'install', action_route: 'components', addons: [], missing_components: ['php-fpm', 'phppgadmin'] },
    { id: 'db.mariadb', category: 'database', name_code: 'db_mariadb_phpmyadmin', components: ['mariadb', 'nginx', 'php-fpm', 'phpmyadmin'], status: 'available', action: 'install', action_route: 'components', addons: [], missing_components: ['mariadb', 'php-fpm', 'phpmyadmin'] },
];
const SCAN = () => ({
    scanned_at: now(), scan_age_seconds: 0, dns_identity_ready: true,
    mail_hostname: { current: 'mail.example.net', hostname: 'mail.example.net', source: 'saved', current_usable: true, will_set_hostname: false },
    services: SERVICES, bundles: BUNDLES,
    profiles: ['core-mail', 'webmail', 'protected-mail'].map((id) => ({ id, name: id, description: `${id} profile`, status: 'available', available: true, verified: false, latest_attempt_status: 'none', services: ['postfix'] })),
});
const known = (notable = [], refusals = []) => ({ state: 'known', installs: [], notable, refusals, removes: [], requested: [] });
const B28 = {
    candidates: { 'php-fpm': '2:8.4+101', phppgadmin: '7.14.7+dfsg-1' },
    repos: { 'php-fpm': { available: true, enabled: true, name: 'Sury PHP (packages.sury.org)', packages: ['php8.3-fpm', 'php8.5-fpm', 'php8.6-fpm'] } },
    preview: known(),
    previewFor: {
        'php-fpm:php8.5-fpm|phppgadmin::8.5': known([
            { package: 'php8.4-common', reason: 'new_php_version', version: '8.4' },
            { package: 'php8.4-mbstring', reason: 'new_php_version', version: '8.4' },
            { package: 'php8.4-pgsql', reason: 'new_php_version', version: '8.4' },
        ]),
        'php-fpm:php8.6-fpm|phppgadmin::8.6': known([], [{ id: 'phppgadmin', code: 'php_extensions_unavailable', version: '8.6', packages: ['php8.6-pgsql'] }]),
    },
};

const facts = (page) => page.evaluate(() => {
    const visible = (node) => node.getClientRects().length > 0 && getComputedStyle(node).visibility !== 'hidden';
    const dialog = document.querySelector('[role="dialog"]');
    const raw = /\b(services|common)\.[a-zA-Z][a-zA-Z0-9_.]*\b|\{[a-zA-Z]+\}/;
    const small = dialog ? Array.from(dialog.querySelectorAll('button, input, label')).filter(visible)
        .map((node) => ({ node, r: node.getBoundingClientRect() }))
        .filter(({ r }) => window.innerWidth <= 639 && r.height < 43.5 && r.height > 0)
        .map(({ node, r }) => `${(node.innerText || node.getAttribute('aria-label') || node.tagName).trim().slice(0, 40)} ${Math.round(r.height)}px`) : [];
    return dialog ? {
        text: dialog.innerText.split('\n').map((l) => l.trim()).filter(Boolean).slice(0, 40),
        rawKeys: dialog.innerText.split('\n').filter((l) => raw.test(l)).slice(0, 6),
        buttons: Array.from(dialog.querySelectorAll('button')).filter(visible).map((b) => `${b.innerText.trim()}${b.disabled ? ' [disabled]' : ''}`).slice(-3),
        scrolls: dialog.scrollHeight > dialog.clientHeight + 1,
        overflowX: dialog.scrollWidth > dialog.clientWidth + 1,
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
        smallTargets: small.slice(0, 10),
    } : { dialog: null };
});

export default function batch28(scenarios, { base, locale, theme, out, viewport, ctl, reset, newPage, closePage, shot, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const snap = async (page, state) => {
        await shot(page, `y28-${state}`, await facts(page));
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `y28-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    const clean = () => ctl({ clearAll: true, b3: null, b12: null, b16: null, b19: null, b20: null, b21: null, b22: null, b26: null, b28: null, managedScan: null, componentOperation: null, domains: [], infrastructureZones: [], twoFactor: false, services: [] });
    const choose = (page, option) => page.evaluate((option) => {
        const input = document.querySelector(`[data-bundle-version-option="${option}"]`);
        if (input) { input.click(); return true; }
        return false;
    }, option);

    scenarios.y28bundle = async () => {
        await reset(); await clean();
        await ctl({ b28: B28, override: {
            '/api/v1/managed-services': { status: 200, body: SCAN() },
            '/api/v1/managed-services/scan': { status: 200, body: SCAN() },
        } });
        const page = await newPage({ 'celikpanel.components.view': 'installed' });
        await page.goto(`${base}/services`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page);
        await waitFor(page, () => !!document.querySelector('[data-bundle="db.postgresql"]'), 15000).catch(() => {});
        await page.evaluate(() => document.querySelector('[data-bundle="db.postgresql"]')?.click());
        await waitFor(page, () => !!document.querySelector('[data-install-preview]'), 8000).catch(() => {});
        await pause(600);
        await snap(page, 'bundle-versions');
        await page.evaluate(() => document.querySelector('[data-bundle-version-change="php-fpm"]')?.click());
        await pause(300);
        await choose(page, 'php8.5-fpm');
        await waitFor(page, () => !!document.querySelector('[data-install-preview="notable"]'), 8000).catch(() => {});
        await pause(500);
        await snap(page, 'bundle-notable');
        await choose(page, 'php8.6-fpm');
        await waitFor(page, () => !!document.querySelector('[data-bundle-refusal]'), 8000).catch(() => {});
        await pause(500);
        await snap(page, 'bundle-refused');
        await closePage(page);
        await clean();
    };
}
