// Batch 30 (2026-10-24): the v0.1.0-alpha.107 candidate (D-038: versions,
// use and security of every component).
//
//   y30security  Security: the operating system's updates card, Sury PHP not
//                covered by automatic updates, the drop-in offered; its dialog
//   y30restarts  Maintenance: services on replaced code and a waiting reboot;
//                the restart question for nginx
//   y30os        Server setup: the operating-system card after a Debian 13 to
//                14 upgrade, each change with its next action
//   y30node      Components: the Node.js drawer with a newer patch, a site on
//                the old one, and the move question
//   y30php       PHP-FPM page: the vendor repository card (confined, its key
//                expiring, apt's error)
//   y30webmail   Roundcube page: the update card and its question
//
// Every answer comes from the mock on 127.0.0.1 through overrides (SAMPLE
// data shaped as the Panel answers); nothing is installed, moved or contacted.
// Screenshots also go to BROWSER_INSPECT_SHOTS as <state>-<lang>-<theme>-<width>.png.
// Otuzuncu grup: alpha.107 kartları ve pencereleri; hepsi sahte.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';

const now = () => new Date().toISOString().replace(/\.\d{3}Z$/, 'Z');
const svc = (id, name, category, extra = {}) => ({
    id, name, description: '', icon: '', category, status: 'active (running)', is_installed: true,
    versions: [], packages: [id], repair_available: true, ...extra,
});
const SERVICES = [
    svc('nginx', 'Nginx', 'web', { kind: 'service', unit: 'nginx' }),
    svc('php-fpm', 'PHP-FPM', 'web', { kind: 'runtime', versions: ['8.5', '8.4'], instances: [
        { version: '8.5', unit: 'php8.5-fpm', managed: true, status: 'active (running)' },
        { version: '8.4', unit: 'php8.4-fpm', managed: true, status: 'active (running)' }] }),
    svc('node', 'Node.js', 'web', { kind: 'runtime', packages: [], versions: ['22.14.0'], instances: [
        { version: '22.14.0', managed: true, status: 'installed' }, { version: '22.23.4', managed: true, status: 'installed' }] }),
    svc('postgresql', 'PostgreSQL', 'database', { kind: 'service' }),
    svc('postfix', 'Postfix', 'email', { kind: 'service' }),
    svc('roundcube', 'Roundcube', 'email', { kind: 'tool', status: 'installed', packages: [], php_binding: { version: '8.4', socket_ready: true, version_installed: true } }),
];
const SCAN = () => ({
    scanned_at: now(), scan_age_seconds: 0, dns_identity_ready: true,
    mail_hostname: { current: 'mail.example.net', hostname: 'mail.example.net', source: 'saved', current_usable: true, will_set_hostname: false },
    services: SERVICES, bundles: [],
    profiles: ['core-mail', 'webmail', 'protected-mail'].map((id) => ({ id, name: id, description: `${id} profile`, status: 'available', available: true, verified: false, latest_attempt_status: 'none', services: ['postfix'] })),
});
const PROPOSED = 'Unattended-Upgrade::Origins-Pattern {\n        "origin=deb.sury.org";\n};\n';
const OS_UPDATES = {
    state: 'read', package_family: 'apt',
    automatic: { state: 'on', tool: 'unattended-upgrades' },
    last_run: '2026-10-24T06:12:01Z', last_run_source: 'log',
    pending: { state: 'known', total: 3, security: 2, security_names: ['libssl3t64', 'openssl'], lists_at: '2026-10-24T05:00:00Z' },
    vendors: [{ repo_id: 'sury', name: 'Sury PHP (packages.sury.org)', origin: 'deb.sury.org', covered: false, confined: true, packages: 14, names: ['php8.5-fpm', 'php8.5-cli'] }],
    outside: [{ id: 'roundcube', versions: ['1.6.15'] }, { id: 'node', versions: ['22.14.0'] }],
    drop_in: { path: '/etc/apt/apt.conf.d/52celikpanel-vendor-origins', state: 'absent', proposed: PROPOSED, proposed_sha256: 'a'.repeat(64) },
    needrestart_auto_restart: false,
};
const RESTARTS = { state: 'read', needs: {
    source: 'needrestart', services_known: true, reboot_known: true, reboot_required: true, reboot_reason: 'reboot_required_file',
    reboot_packages: ['linux-image-6.12.57+deb13-amd64'], running_kernel: '6.12.48+deb13-amd64', expected_kernel: '6.12.57+deb13-amd64',
    services: [
        { unit: 'nginx.service', component: 'nginx', interruption: 'web' },
        { unit: 'php8.4-fpm.service', component: 'php-fpm', interruption: 'php' },
        { unit: 'named.service', component: 'bind', interruption: 'dns' },
        { unit: 'cron.service', interruption: 'other' },
    ],
} };
const OS_RELEASE = {
    state: 'read', checked: '2026-10-24', changed: true,
    current: { id: 'debian', version_id: '14', codename: 'forky', pretty_name: 'Debian GNU/Linux 14 (forky)', level: 'not_listed' },
    recorded: { pretty_name: 'Debian GNU/Linux 13 (trixie)', codename: 'trixie', version_id: '13', recorded_at: '2026-10-24T10:00:00Z' },
    releases: [
        { name: 'Debian 13 (trixie)', level: 'tested', patched_until: '2030-06-30' },
        { name: 'Ubuntu 24.04 LTS (noble)', level: 'tested', patched_until: '2029-05-31' },
        { name: 'Debian 12 (bookworm)', level: 'accepted', patched_until: '2028-06-30' },
        { name: 'Ubuntu 22.04 LTS (jammy)', level: 'accepted', patched_until: '2027-05-31' },
        { name: 'Arch Linux (rolling)', level: 'development' },
        { name: 'Red Hat Enterprise Linux 9 and AlmaLinux / Rocky Linux 9', level: 'preview' },
    ],
    changes: [
        { kind: 'repo_suite', subject: 'sury', from: 'trixie', to: 'forky' },
        { kind: 'pg_cluster_new', subject: '18/main' },
        { kind: 'php_added', subject: '8.5' },
        { kind: 'mariadb_version', from: '11.8', to: '12.1' },
    ],
};
const NODE_PATCHES = { state: 'read', patches: [{ installed: '22.14.0', line: '22', newest: '22.23.4', newer: true, sites: [{ domain_id: 7, name: 'app.example.net' }] }] };
const REPO = {
    available: true, enabled: true, id: 'sury', name: 'Sury PHP (packages.sury.org)',
    confinement: { state: 'confined', pin_path: '/etc/apt/preferences.d/celikpanel-sury.pref', kept: ['php8.4-fpm', 'php8.4-cli'], kept_total: 2, written_at: '2026-10-24T10:00:00Z', written_by: 'panel_upgrade' },
    key: { fingerprint: 'B97B0AFCAA1A47F044F244A07FCC7D46ACCC4CF8', state: 'expiring', expires_at: '2026-11-30T00:00:00Z' },
    lists: { state: 'fresh', date: 'Fri, 23 Oct 2026 10:00:00 UTC', origin: 'deb.sury.org' },
    apt_errors: ["W: GPG error: https://packages.sury.org/php trixie InRelease: The following signatures will be invalid soon"],
    apt_errors_at: '2026-10-24T09:00:00Z',
    enabled_by: 'admin', enabled_at: '2026-09-01 10:00:00',
};
const ROUNDCUBE = { state: 'read', check: { installed: true, version: '1.6.15', line: '1.6', latest_on_line: '1.6.20', latest_overall: '1.7.5', update_available: true, checked_at: '2026-10-24T10:00:00Z' } };

const facts = (page) => page.evaluate(() => {
    const visible = (node) => node.getClientRects().length > 0 && getComputedStyle(node).visibility !== 'hidden';
    const root = document.querySelector('[role="dialog"]') || document.querySelector('main') || document.body;
    const raw = /\b(life|services|common|setup)\.[a-zA-Z][a-zA-Z0-9_.]*\b|\{[a-zA-Z]+\}/;
    return {
        dialog: !!document.querySelector('[role="dialog"]'),
        text: root.innerText.split('\n').map((l) => l.trim()).filter(Boolean).slice(0, 60),
        rawKeys: root.innerText.split('\n').filter((l) => raw.test(l)).slice(0, 6),
        buttons: Array.from(root.querySelectorAll('button')).filter(visible).map((b) => `${b.innerText.trim()}${b.disabled ? ' [disabled]' : ''}`).slice(-6),
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
    };
});

export default function batch30(scenarios, { base, locale, theme, out, viewport, ctl, reset, newPage, closePage, shot, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const snap = async (page, state) => {
        await shot(page, `y30-${state}`, await facts(page));
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `y30-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    const clean = () => ctl({ clearAll: true, b28: null, managedScan: null, componentOperation: null, domains: [], services: [] });
    const answers = () => ctl({ override: {
        '/api/v1/managed-services': { status: 200, body: SCAN() },
        '/api/v1/managed-services/scan': { status: 200, body: SCAN() },
        '/api/v1/security/os-updates': { status: 200, body: OS_UPDATES, method: 'GET' },
        '/api/v1/maintenance/restarts': { status: 200, body: RESTARTS, method: 'GET' },
        '/api/v1/server/os-release': { status: 200, body: OS_RELEASE, method: 'GET' },
        '/api/v1/runtimes/node/patches': { status: 200, body: NODE_PATCHES, method: 'GET' },
        '/api/v1/repo': { status: 200, body: REPO, method: 'GET' },
        '/api/v1/service/roundcube/update': { status: 200, body: ROUNDCUBE, method: 'GET' },
    } });
    const open = async (path, ready, storage = {}) => {
        await reset(); await clean(); await answers();
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

    scenarios.y30security = async () => {
        const page = await open('/security', () => !!document.querySelector('[data-security-section="updates"] [data-life-line]'));
        await scrollTo(page, '[data-security-section="updates"]');
        await pause(300);
        await snap(page, 'security-updates');
        const opened = await page.evaluate(() => {
            const button = Array.from(document.querySelectorAll('[data-security-section="updates"] button')).find((b) => /drop-in|dosya|Write|Yaz|Add|Ekle|origins|kaynak/i.test(b.innerText));
            button?.click();
            return !!button;
        });
        if (opened) {
            await waitFor(page, () => !!document.querySelector('[role="dialog"]'), 5000).catch(() => {});
            await pause(300);
            await snap(page, 'security-dropin-dialog');
        }
        await closePage(page);
    };

    scenarios.y30restarts = async () => {
        const page = await open('/maintenance', () => !!document.querySelector('[data-maintenance-section="restarts"] [data-life-line]'));
        await scrollTo(page, '[data-maintenance-section="restarts"]');
        await pause(300);
        await snap(page, 'restarts');
        const opened = await page.evaluate(() => {
            const button = Array.from(document.querySelectorAll('[data-maintenance-section="restarts"] button')).find((b) => /Restart…|Yeniden başlat…/.test(b.innerText));
            button?.click();
            return !!button;
        });
        if (opened) {
            await waitFor(page, () => !!document.querySelector('[role="dialog"]'), 5000).catch(() => {});
            await pause(300);
            await snap(page, 'restart-dialog');
        }
        await closePage(page);
    };

    scenarios.y30os = async () => {
        const page = await open('/server-setup', () => !!document.querySelector('[data-server-os-release] [data-os-changes]'));
        await scrollTo(page, '[data-server-os-release]');
        await page.evaluate(() => { const details = document.querySelector('[data-server-os-release] details'); if (details) details.open = true; });
        await pause(300);
        await snap(page, 'os-release-changed');
        await closePage(page);
    };

    scenarios.y30node = async () => {
        const page = await open('/services', () => !!document.querySelector('main li'), { 'celikpanel.components.view': 'installed' });
        await page.evaluate(() => {
            const row = Array.from(document.querySelectorAll('main li')).find((li) => /Node\.js/.test(li.innerText));
            const button = row && Array.from(row.querySelectorAll('button')).find((b) => /Versions|Sürümler/.test(b.innerText));
            button?.click();
        });
        await waitFor(page, () => !!document.querySelector('[data-node-patches]'), 8000).catch(() => {});
        await scrollTo(page, '[data-node-patches]');
        await pause(300);
        await snap(page, 'node-patches');
        if (await click(page, '[data-node-move]')) {
            await waitFor(page, () => !!document.querySelector('[role="dialog"]'), 5000).catch(() => {});
            await pause(300);
            await snap(page, 'node-move-dialog');
        }
        await closePage(page);
    };

    scenarios.y30php = async () => {
        const page = await open('/services/php-fpm', () => !!document.querySelector('[data-life-line="key-expiring"]'));
        await scrollTo(page, '[data-life-line="key-expiring"]');
        await pause(300);
        await snap(page, 'php-repo-card');
        await closePage(page);
    };

    scenarios.y30webmail = async () => {
        await reset(); await clean(); await answers();
        // Roundcube has no vendor repository: its card draws nothing.
        await ctl({ override: { '/api/v1/repo': { status: 200, body: { available: false, enabled: false }, method: 'GET' } } });
        const page = await newPage({});
        await page.goto(`${base}/services/roundcube`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page);
        await waitFor(page, () => !!document.querySelector('main button'), 10000).catch(() => {});
        await pause(800);
        await page.evaluate(() => Array.from(document.querySelectorAll('main button')).find((b) => /1\.6\.20/.test(b.innerText))?.scrollIntoView({ block: 'center' }));
        await pause(800);
        await snap(page, 'webmail-update');
        const opened = await page.evaluate(() => {
            const button = Array.from(document.querySelectorAll('main button')).find((b) => /1\.6\.20/.test(b.innerText));
            button?.scrollIntoView({ block: 'center' });
            button?.click();
            return !!button;
        });
        if (opened) {
            await waitFor(page, () => !!document.querySelector('[role="dialog"]'), 5000).catch(() => {});
            await pause(300);
            await snap(page, 'webmail-update-dialog');
        }
        await closePage(page);
    };
}
