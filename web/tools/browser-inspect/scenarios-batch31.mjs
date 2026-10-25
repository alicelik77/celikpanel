// Batch 31 (2026-10-25): the v0.1.0-alpha.108 candidate (D-038 amendment
// "two lanes", PostgreSQL majors side by side, the owner's alpha.107 findings).
//
//   y31lane      PHP-FPM page on the default lane: the card, then the vendor
//                lane dialog for a fresh Sury (three steps, files, digests)
//   y31complete  PHP-FPM page with Sury enabled but not confined nor covered
//                (Frankfurt): the missing steps, then the Complete dialog
//   y31locked    Components: the PHP-FPM versions drawer on the default lane,
//                locked versions and tested marks
//   y31security  Security: an uncovered vendor repository with its direct action
//   y31pg        PostgreSQL page: the clusters card with another major offered
//   y31rows      Components rows with the webmail Update button and the
//                sidebar collapsed (desktop) / the phone list
//
// Every answer comes from the mock on 127.0.0.1 through overrides (SAMPLE
// data shaped as the Panel answers); nothing is installed, moved or contacted.
// Otuz birinci grup: alpha.108 kartları ve pencereleri; hepsi sahte.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';

const now = () => new Date().toISOString().replace(/\.\d{3}Z$/, 'Z');
const svc = (id, name, category, extra = {}) => ({
    id, name, description: '', icon: '', category, status: 'active (running)', is_installed: true,
    versions: [], packages: [id], repair_available: true, ...extra,
});
const SERVICES = [
    svc('nginx', 'Nginx', 'web', { kind: 'service', unit: 'nginx', version: '1.26.3' }),
    svc('php-fpm', 'PHP-FPM', 'web', { kind: 'runtime', versions: ['8.4'], instances: [
        { version: '8.4', unit: 'php8.4-fpm', managed: true, status: 'active (running)' }] }),
    svc('postgresql', 'PostgreSQL', 'database', { kind: 'service', version: '17.6' }),
    svc('postfix', 'Postfix', 'email', { kind: 'service', version: '3.10.4' }),
    svc('roundcube', 'Roundcube', 'email', { kind: 'tool', status: 'installed', packages: [], version: '1.6.15', php_binding: { version: '8.4', socket_ready: true, version_installed: true } }),
];
const SCAN = () => ({
    scanned_at: now(), scan_age_seconds: 0, dns_identity_ready: true,
    mail_hostname: { current: 'mail.example.net', hostname: 'mail.example.net', source: 'saved', current_usable: true, will_set_hostname: false },
    services: SERVICES, bundles: [],
    profiles: ['core-mail', 'webmail', 'protected-mail'].map((id) => ({ id, name: id, description: `${id} profile`, status: 'available', available: true, verified: false, latest_attempt_status: 'none', services: ['postfix'] })),
});
const supported = (until, provider) => ({ status: 'supported', until, provider });
const SUPPORT = {
    checked: '2026-10-24', sources: {}, distro: 'debian-13', distro_name: 'Debian 13 (trixie)',
    statuses: { php: { '8.5': supported('2029-12-31', 'upstream'), '8.4': supported('2030-06-30', 'distribution'), '8.3': supported('2027-12-31', 'upstream'), '8.2': { status: 'ends_soon', until: '2026-12-31', provider: 'upstream' } },
        postgresql: { '18': supported('2030-11-14', 'upstream'), '17': supported('2030-06-30', 'distribution') }, mariadb: {}, node: {} },
    tested_release: 'v0.1.0-alpha.108', tested: { php: ['8.4', '8.5'], postgresql: ['17', '18'] },
};
const step = (name, state, extra = {}) => ({ step: name, state, ...extra });
const PIN = 'Explanation: Written by CelikPanel (D-038). The Sury PHP (packages.sury.org) repository was enabled for one component.\nExplanation: It provides a package only where the distribution has none and never replaces a distribution package.\nPackage: *\nPin: origin "packages.sury.org"\nPin-Priority: 100\n';
const DROPIN = '// Written by CelikPanel at the owner\'s confirmation (D-038).\nUnattended-Upgrade::Origins-Pattern {\n\t"origin=deb.sury.org";\n};\n';
const FRESH = { state: 'read', plan: { repo_id: 'sury', name: 'Sury PHP (packages.sury.org)', origin: 'deb.sury.org', enabled: false, ready: true, sha256: '3f'.repeat(32), steps: [
    step('repository', 'todo', { path: '/etc/apt/sources.list.d/celikpanel-sury.list', text: 'deb https://packages.sury.org/php/ trixie main\n# signed by the pinned OpenPGP key 15058500A0235D97F5D10063B188E2B695BD4743\n', sha256: '1a'.repeat(32) }),
    step('confinement', 'todo', { path: '/etc/apt/preferences.d/celikpanel-sury.pref', text: PIN, sha256: '2b'.repeat(32) }),
    step('updates', 'todo', { path: '/etc/apt/apt.conf.d/52celikpanel-vendor-origins', text: DROPIN, sha256: '4c'.repeat(32) }),
] } };
const COMPLETE = { state: 'read', plan: { ...FRESH.plan, enabled: true, sha256: '5d'.repeat(32), kept: ['php-common', 'php8.4-fpm', 'php8.5-fpm', 'php8.6-fpm'], steps: [
    step('repository', 'done', { path: '/etc/apt/sources.list.d/celikpanel-sury.list' }), FRESH.plan.steps[1], FRESH.plan.steps[2]] } };
// The review's owner source: the owner's own php.list, no CelikPanel source.
const OWNER = { state: 'read', plan: { ...FRESH.plan, sha256: '6e'.repeat(32), steps: [
    step('repository', 'owner', { reason: 'owner_source', path: '/etc/apt/sources.list.d/php.list', detail: '/etc/apt/sources.list.d/php.list' }), FRESH.plan.steps[1], FRESH.plan.steps[2]] } };
const REPO_OFF = { available: true, enabled: false, lane: true, origin: 'deb.sury.org', id: 'sury', name: 'Sury PHP (packages.sury.org)', detail: 'Debian/Ubuntu PHP repository — every maintained version, side by side.' };
const REPO_ON = { ...REPO_OFF, enabled: true, confinement: { state: 'not_written', pin_path: '/etc/apt/preferences.d/celikpanel-sury.pref' },
    key: { fingerprint: '15058500A0235D97F5D10063B188E2B695BD4743', state: 'valid', expires_at: '2028-02-15T13:14:55Z' },
    lists: { state: 'fresh', date: 'Sat, 25 Oct 2026 06:00:00 UTC', origin: 'deb.sury.org' }, enabled_by: 'admin', enabled_at: '2026-09-01 10:00:00', packages: ['php8.6-fpm', 'php8.5-fpm', 'php8.4-fpm'] };
const OS_UPDATES = {
    state: 'read', package_family: 'apt', automatic: { state: 'on', tool: 'unattended-upgrades' }, last_run: '2026-10-25T06:12:01Z', last_run_source: 'log',
    pending: { state: 'known', total: 0, security: 0, lists_at: '2026-10-25T05:00:00Z' },
    vendors: [{ repo_id: 'sury', name: 'Sury PHP (packages.sury.org)', origin: 'deb.sury.org', covered: false, confined: false, packages: 44, names: ['php8.4-fpm', 'php8.5-fpm', 'php8.6-fpm'] }],
    outside: [{ id: 'roundcube', versions: ['1.6.15'] }], drop_in: { path: '/etc/apt/apt.conf.d/52celikpanel-vendor-origins', state: 'absent' }, needrestart_auto_restart: false,
};
const CLUSTERS = { state: 'read', supported: true, next_port: 5433, used_ports: [5432], creates_cluster: true, repository_on: false, unplaced: [], mismatches: [], moves: [],
    clusters: [{ version: '17', name: 'main', port: 5432, configured_port: 5432, status: 'online', online: true, data_directory: '/var/lib/postgresql/17/main', config_path: '/etc/postgresql/17/main/postgresql.conf', databases: ['shop'], databases_known: true }],
    offers: [{ major: '18', package: 'postgresql-18', origin: '', needs_repository: true, port: 5433 }],
    records: { '17/main': [{ database_id: 3, name: 'shop', domain: 'shop.example.net', subscription_id: 1, server_id: 1, host: 'localhost', port: 5432, users: ['shop_user'] }] } };
const ROUNDCUBE = { state: 'read', check: { installed: true, version: '1.6.15', line: '1.6', latest_on_line: '1.6.20', latest_overall: '1.7.5', update_available: true, checked_at: '2026-10-25T10:00:00Z' } };

const facts = (page) => page.evaluate(() => {
    const visible = (node) => node.getClientRects().length > 0 && getComputedStyle(node).visibility !== 'hidden';
    const root = document.querySelector('[role="dialog"]') || document.querySelector('main') || document.body;
    const raw = /\b(lane|life|pgc|services|common|nav)\.[a-zA-Z][a-zA-Z0-9_.]*\b|\{[a-zA-Z]+\}/;
    return {
        dialog: !!document.querySelector('[role="dialog"]'),
        text: root.innerText.split('\n').map((l) => l.trim()).filter(Boolean).slice(0, 60),
        rawKeys: root.innerText.split('\n').filter((l) => raw.test(l)).slice(0, 6),
        buttons: Array.from(root.querySelectorAll('button')).filter(visible).map((b) => `${b.innerText.trim()}${b.disabled ? ' [disabled]' : ''}`).slice(-6),
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
    };
});

export default function batch31(scenarios, { base, locale, theme, out, viewport, ctl, reset, newPage, closePage, shot, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const snap = async (page, state) => {
        await shot(page, `y31-${state}`, await facts(page));
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `y31-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    const clean = () => ctl({ clearAll: true, b28: null, managedScan: null, componentOperation: null, domains: [], services: [] });
    const answers = (extra = {}) => ctl({ override: {
        '/api/v1/managed-services': { status: 200, body: SCAN() },
        '/api/v1/managed-services/scan': { status: 200, body: SCAN() },
        '/api/v1/support-table': { status: 200, body: SUPPORT, method: 'GET' },
        '/api/v1/security/os-updates': { status: 200, body: OS_UPDATES, method: 'GET' },
        '/api/v1/service/roundcube/update': { status: 200, body: ROUNDCUBE, method: 'GET' },
        '/api/v1/service/postgresql/clusters': { status: 200, body: CLUSTERS, method: 'GET' },
        ...extra,
    } });
    const open = async (path, ready, extra = {}, storage = {}) => {
        await reset(); await clean(); await answers(extra);
        const page = await newPage(storage);
        await page.goto(`${base}${path}`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page);
        await waitFor(page, ready, 10000).catch(() => {});
        await pause(500);
        return page;
    };
    const click = (page, selector) => page.evaluate((s) => { const node = document.querySelector(s); node?.scrollIntoView({ block: 'center' }); node?.click(); return !!node; }, selector);
    const scrollTo = (page, selector) => page.evaluate((s) => document.querySelector(s)?.scrollIntoView({ block: 'start' }), selector);
    const dialog = async (page, selector, state) => {
        if (await click(page, selector)) {
            await waitFor(page, () => !!document.querySelector('[data-vendor-lane]'), 8000).catch(() => {});
            await pause(400);
            await snap(page, state);
        }
    };

    scenarios.y31lane = async () => {
        const page = await open('/services/php-fpm', () => !!document.querySelector('[data-lane-default]'), {
            '/api/v1/repo': { status: 200, body: REPO_OFF, method: 'GET' },
            '/api/v1/repo/lane': { status: 200, body: FRESH, method: 'GET' },
        });
        await scrollTo(page, '[data-lane-default]');
        await pause(300);
        await snap(page, 'lane-default');
        await dialog(page, '[data-lane-enable]', 'lane-fresh-dialog');
        await closePage(page);
    };

    scenarios.y31owner = async () => {
        const page = await open('/services/php-fpm', () => !!document.querySelector('[data-lane-owner-source]') || !!document.querySelector('[data-life-line="lane-owner-source"]'), {
            '/api/v1/repo': { status: 200, body: REPO_OFF, method: 'GET' },
            '/api/v1/repo/lane': { status: 200, body: OWNER, method: 'GET' },
        });
        await scrollTo(page, '[data-lane-default]');
        await pause(300);
        await snap(page, 'lane-owner-card');
        await dialog(page, '[data-lane-enable]', 'lane-owner-dialog');
        await closePage(page);
    };

    scenarios.y31complete = async () => {
        const page = await open('/services/php-fpm', () => !!document.querySelector('[data-lane-missing]'), {
            '/api/v1/repo': { status: 200, body: REPO_ON, method: 'GET' },
            '/api/v1/repo/lane': { status: 200, body: COMPLETE, method: 'GET' },
        });
        await scrollTo(page, '[data-repo-card]');
        await pause(300);
        await snap(page, 'lane-missing');
        await dialog(page, '[data-lane-complete]', 'lane-complete-dialog');
        await closePage(page);
    };

    scenarios.y31locked = async () => {
        const page = await open('/services', () => !!document.querySelector('main li'), {
            '/api/v1/repo': { status: 200, body: REPO_OFF, method: 'GET' },
        }, { 'celikpanel.components.view': 'installed' });
        await page.evaluate(() => {
            const row = Array.from(document.querySelectorAll('main li')).find((li) => /PHP-FPM/.test(li.innerText));
            const button = row && Array.from(row.querySelectorAll('button')).find((b) => /Versions|Sürümler/.test(b.innerText));
            button?.click();
        });
        await waitFor(page, () => !!document.querySelector('[data-locked-versions]'), 8000).catch(() => {});
        await scrollTo(page, '[data-locked-versions]');
        await pause(300);
        await snap(page, 'versions-locked');
        await closePage(page);
    };

    scenarios.y31security = async () => {
        const page = await open('/security', () => !!document.querySelector('[data-os-updates-lane]'), {
            '/api/v1/repo/lane': { status: 200, body: COMPLETE, method: 'GET' },
        });
        await scrollTo(page, '[data-os-updates-vendor]');
        await pause(300);
        await snap(page, 'security-lane');
        await closePage(page);
    };

    scenarios.y31pg = async () => {
        const page = await open('/services/postgresql', () => !!document.querySelector('main section'), {
            '/api/v1/repo': { status: 200, body: { ...REPO_OFF, id: 'pgdg', origin: 'apt.postgresql.org', name: 'PostgreSQL Global Development Group (PGDG)' }, method: 'GET' },
        });
        await pause(800);
        await page.evaluate(() => document.querySelector('[data-pg-clusters]')?.scrollIntoView({ block: 'start' }));
        await pause(300);
        await snap(page, 'pg-clusters');
        await page.evaluate(() => document.querySelector('[data-repo-card]')?.scrollIntoView({ block: 'start' }));
        await pause(300);
        await snap(page, 'pg-clusters-lane');
        await closePage(page);
    };

    scenarios.y31rows = async () => {
        const storage = viewport.width > 600 ? { 'celikpanel.sidebar.collapsed': '1', 'celikpanel.components.view': 'installed' } : { 'celikpanel.components.view': 'installed' };
        const page = await open('/services', () => !!document.querySelector('main li'), {}, storage);
        await pause(800);
        await snap(page, 'components-rows');
        await page.evaluate(() => Array.from(document.querySelectorAll('main li')).find((li) => /Roundcube/.test(li.innerText))?.scrollIntoView({ block: 'center' }));
        await pause(300);
        await snap(page, 'components-roundcube');
        await closePage(page);
    };
}
