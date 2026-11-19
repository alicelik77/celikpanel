// Batch 37 (2026-11-19): the product screens of the public website
// (celikpanel.net, `site/`). Three pages of a well-kept example server, as an
// owner sees them: the Dashboard, Domains and Databases.
//
//   sitescreens  dashboard, domains, databases: one viewport each, written as
//                WebP (quality 82) beside the PNG record, into
//                BROWSER_INSPECT_SITE (default: <out>/site) as
//                product-<page>-<locale>-<theme>.png
//
// Every name, address and number is SAMPLE data (example.com, example.net,
// example.org, 192.0.2.0/24); the server is batch 20's sample "everything
// installed" server. Nothing is contacted but the mock on 127.0.0.1.
// Otuz yedinci grup: herkese açık sitenin ürün ekranları; hepsi örnek veri.
import { mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { batch20State } from './mock-batch20.mjs';

const MB = 1048576;
const day = (offset) => new Date(Date.UTC(2026, 10, 19 - offset, 9, 30)).toISOString();
const domain = (id, name, extra = {}) => ({
    id, domain_name: name, status: 'active', project_type: 'php', php_version: '8.4', ssl_enabled: true,
    created_at: day(40 - id * 3), disk_usage: 0, bandwidth: 0, ...extra,
});
const DOMAINS = [
    domain(1, 'example.com', { disk_usage: 1843 * MB, bandwidth: 412 * MB }),
    domain(2, 'shop.example.com', { disk_usage: 5120 * MB, bandwidth: 2390 * MB }),
    domain(3, 'example.net', { project_type: 'node', php_version: '', disk_usage: 612 * MB, bandwidth: 845 * MB }),
    domain(4, 'docs.example.org', { project_type: 'static', php_version: '', disk_usage: 94 * MB, bandwidth: 120 * MB }),
    domain(5, 'studio.example.org', { php_version: '8.3', disk_usage: 2310 * MB, bandwidth: 530 * MB }),
    domain(6, 'blog.example.net', { disk_usage: 760 * MB, bandwidth: 96 * MB }),
];
const owner = (id, username, role, parent = null) => ({ id, username, role, parent, self: role === 'admin' });
const ADMIN = owner(1, 'admin', 'admin');
const database = (id, name, engine, who, domainId, sizeMiB, users) => ({
    id, name, engine, owner: who, domain: domainId ? { id: domainId, name: DOMAINS[domainId - 1].domain_name } : null,
    size_bytes: Math.round(sizeMiB * 1048576), created_at: day(30 - id * 4), users: users.map((username, index) => ({ id: id * 10 + index, username })),
});
const DATABASES = {
    databases: [
        database(1, 'shop_main', 'mariadb', owner(3, 'shopteam', 'customer', 'agency'), 2, 46, ['shop_app']),
        database(2, 'example_wp', 'mariadb', owner(2, 'example', 'customer'), 1, 20.5, ['example_wp']),
        database(3, 'app_events', 'postgresql', owner(4, 'netapp', 'customer'), 3, 12.2, ['app_rw', 'app_ro']),
        database(4, 'studio_site', 'mariadb', owner(5, 'studio', 'customer', 'agency'), 5, 9, ['studio']),
        database(5, 'blog', 'mariadb', owner(2, 'example', 'customer'), 6, 5, ['blog']),
    ],
    engines: [{ engine: 'mariadb', installed: true, running: true }, { engine: 'postgresql', installed: true, running: true }],
    may_create: true,
};
const OS_UPDATES = { state: 'ready', checked_at: day(0), package_family: 'apt', pending: { state: 'known', total: 0, security: 0, lists_at: day(0) }, automatic: { state: 'on', tool: 'unattended-upgrades' } };
// One domain's overview: delegated here, a valid certificate, mail
// authentication complete, a recent backup, disk within its limit.
const CONNECTION = { domain: 'example.com', server_ip: '192.0.2.10', nameservers: ['ns1.example.net', 'ns2.example.net'], live_nameservers: ['ns1.example.net', 'ns2.example.net'], live_ips: ['192.0.2.10'], status: 'delegated', ssl_ready: true, glue_needed: false, nameservers_usable: true, propagation_pending: false, checked_at: day(0), dns_management_mode: 'local' };
const CERTIFICATE = {
    domain_id: 1, domain_name: 'example.com', has_certificate: true, managed_names: ['example.com', 'www.example.com'],
    settings: { force_https: true, hsts_enabled: false, hsts_max_age: 300 },
    certificate: {
        id: 4, type: 'letsencrypt', provider_id: 'letsencrypt', issuer: 'R11', subject: 'example.com', issued_at: day(12), expires_at: day(-78),
        days_until_expiry: 78, auto_renew: true, renewal_status: 'ok', status: 'active', dns_names: ['example.com', 'www.example.com'],
        activated: true, usable: true, trust_status: 'trusted', activation_pending: false, dependents_pending: false,
    },
};
const HEALTH = { contact: 'administrator', checks: [
    { id: 'certificate', state: 'ok', code: 'valid', days: 78, at: day(0) },
    { id: 'mail', state: 'ok', code: 'complete', at: day(0) },
    { id: 'backup', state: 'ok', code: 'recent', at: day(1) },
    { id: 'disk', state: 'ok', code: 'no_limit', used_bytes: 1843 * MB, at: day(0) },
] };
const VERSION = { version: 'v0.1.0-alpha.137', commit: '2b0fc4d', agent_commit: '2b0fc4d', agent_matches: true, hostname: 'server1', ipv4: '192.0.2.10' };

export default function batch37(scenarios, { base, locale, theme, out, viewport, ctl, reset, newPage, closePage, shot, waitFor, pause, quiet }) {
    const site = resolve(process.env.BROWSER_INSPECT_SITE || join(out, 'site'));
    const clean = () => ctl({ clearAll: true, b19: null, b20: null, b12: null, b16: null, managedScan: null, componentOperation: null, domains: [], infrastructureZones: [] });
    // The site shows the screens crisp: desktop at 1280×800 CSS pixels and
    // twice the density; the phone run keeps its own 390×844 at 2×.
    const sharp = viewport.width > 600 ? { width: 1280, height: 800, deviceScaleFactor: 2 } : viewport;
    const open = async (path) => {
        const page = await newPage();
        await page.setViewport(sharp);
        await page.goto(`${base}${path}`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page); await pause(1200);
        return page;
    };
    const capture = async (page, name) => {
        await shot(page, `y37-${name}`);
        await mkdir(site, { recursive: true });
        await page.screenshot({ path: join(site, `product-${name}-${locale}-${theme}.png`) });
    };
    scenarios.sitescreens = async () => {
        await reset(); await clean();
        await ctl({
            b20: batch20State('frankfurt', { extra: ['clamav'] }),
            domains: DOMAINS,
            dashboard: { databases: DATABASES.databases.length, mail_accounts: 14, expiring_certs: [] },
            override: {
                '/api/v1/notifications': { status: 200, body: { items: [] } },
                '/api/v1/security/os-updates/summary': { status: 200, body: OS_UPDATES },
                '/api/v1/databases/visible': { status: 200, body: DATABASES },
                '/api/v1/panel/version': { status: 200, body: VERSION },
                '/api/v1/domains/1/usage': { status: 200, body: { disk_usage: 1843 * MB, bandwidth: 412 * MB, traffic_state: 'measured' } },
                '/api/v1/domains/1/health': { status: 200, body: HEALTH },
            },
            connection: CONNECTION,
            ssl: CERTIFICATE,
        });
        for (const [name, path, ready] of [
            ['dashboard', '/', () => !!document.querySelector('main h1')],
            ['domains', '/domains', () => /example\.com/.test(document.querySelector('main')?.innerText || '')],
            ['domain', '/domains/example.com', () => /example\.com/.test(document.querySelector('main')?.innerText || '')],
            ['databases', '/databases', () => /shop_main/.test(document.querySelector('main')?.innerText || '')],
        ]) {
            const page = await open(path);
            await waitFor(page, ready, 15000).catch(() => {});
            await pause(800);
            await capture(page, name);
            await closePage(page);
        }
        await clean();
    };
}
