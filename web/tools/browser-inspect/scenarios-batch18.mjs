// Batch 18 (2026-10-16): the second server as a full peer, part 2 (branch
// feat/alpha91-second-server-part2; the entry "Second server as a full peer,
// part 2" of docs/OPERATION-GUIDANCE.md), photographed for the craft floor,
// D-024 and D-033: Turkish complete, no raw ids, no overflow at 390 px, dark
// legible, no hosting action on a DNS-only entry, no instruction to act when
// nothing needs to be done.
//
//   y18domains   Domains with one website and the infrastructure zone as a
//                DNS-only row; then the zone list unreadable (one line, the
//                domains stay)
//   y18zone      the infrastructure zone's page: what it is, who keeps which
//                record, then the records (setup's, the other server's, the
//                owner's)
//   y18plan      the primary's setup plan with the other server's mail host
//                name beside its panel host name
//   y18setup     the secondary's setup waiting for public DNS: the primary
//                created the record (nothing to do); refused (another record
//                uses the name); no DNS connection (add by hand)
//   y18mail      the Components mail box on a secondary, with and without an
//                authorized DNS connection to the primary
//
// Every answer comes from the mock on 127.0.0.1 (mock.mjs, mock-batch16.mjs
// and overrides); nothing else is contacted and nothing is started.
// Screenshots also go to BROWSER_INSPECT_SHOTS (default: the run's out dir) as
// <state>-<lang>-<theme>-<width>.png.
// On sekizinci grup: ikinci sunucu tam es, ikinci bolum.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';

const ZONE = 'celikhost.com';
const BASE = `/api/v1/dns/infrastructure-zones/${ZONE}`;
const REQ_ID = 'd'.repeat(32);
const PLAN_ID = 'f'.repeat(32);
const DOMAIN = { id: 1, domain_name: 'shop.example.org', status: 'active', created_at: '2026-09-01T00:00:00Z', project_type: 'php', php_version: '8.3', ssl_enabled: true, disk_usage: 24_500_000, bandwidth: 180_000_000 };
const RECORDS = [
    { id: 1, domain_id: 4, name: ZONE, type: 'SOA', content: 'ns1.celikhost.com hostmaster.celikhost.com 2026101601 3600 600 604800 300', ttl: 3600, disabled: false },
    { id: 2, domain_id: 4, name: ZONE, type: 'NS', content: 'ns1.celikhost.com', ttl: 3600, disabled: false },
    { id: 3, domain_id: 4, name: ZONE, type: 'NS', content: 'ns2.celikhost.com', ttl: 3600, disabled: false },
    { id: 4, domain_id: 4, name: 'frankfurt.celikhost.com', type: 'A', content: '203.0.113.10', ttl: 300, disabled: false, managed_by: 'setup' },
    { id: 5, domain_id: 4, name: 'boston.celikhost.com', type: 'A', content: '198.51.100.20', ttl: 300, disabled: false, managed_by: 'setup' },
    { id: 6, domain_id: 4, name: 'ns1.celikhost.com', type: 'A', content: '203.0.113.10', ttl: 300, disabled: false, managed_by: 'setup' },
    { id: 7, domain_id: 4, name: 'ns2.celikhost.com', type: 'A', content: '198.51.100.20', ttl: 300, disabled: false, managed_by: 'setup' },
    { id: 8, domain_id: 4, name: 'mail.boston.celikhost.com', type: 'A', content: '198.51.100.20', ttl: 300, disabled: false, managed_by: 'peer' },
    { id: 9, domain_id: 4, name: ZONE, type: 'TXT', content: '"google-site-verification=Zx81kq"', ttl: 3600, disabled: false },
];
const ZONES = [{ zone: ZONE, kind: 'infrastructure', records: RECORDS.length, peer_records: 1 }];
const zoneServed = { [`${BASE}/zone`]: { status: 200, body: { id: 4, name: ZONE, type: 'MASTER' } }, [`${BASE}/records`]: { status: 200, body: { records: RECORDS } } };

const SECONDARY = { dns_mode: 'local', dns_role: 'secondary', dns_engine: 'bind', local_nameserver: 'ns2.celikhost.com', local_ip: '198.51.100.20', peer_nameserver: 'ns1.celikhost.com', peer_ip: '203.0.113.10', panel_domain: 'boston.celikhost.com', mail_hostname: 'mail.boston.celikhost.com', dns_hosting_management: 'panel' };
const STEPS = (status) => [
    { id: '01-dns', kind: 'dns', target: 'local', status: 'succeeded' },
    { id: '02-access_dns', kind: 'access_dns', target: 'boston.celikhost.com', qualifier: '198.51.100.20', status: 'succeeded' },
    { id: '03-dns_publisher', kind: 'dns_publisher', target: 'https://frankfurt.celikhost.com:2083', status: 'succeeded' },
    { id: '04-mail_profile', kind: 'mail_profile', target: 'core-mail', status: 'succeeded' },
    { id: '05-access_dns', kind: 'access_dns', target: 'mail.boston.celikhost.com', qualifier: '198.51.100.20', status },
    { id: '06-mail_certificate', kind: 'mail_certificate', target: 'mail.boston.celikhost.com', status: 'pending' },
    { id: '07-verify', kind: 'verify', target: 'web_mail', status: 'pending' },
];
const waiting = (peer) => ({ request_id: 'adopt', statuses: [], extra: { status: 'waiting', phase: 'access_dns', steps: STEPS('running'), error: { code: 'server_setup_access_dns_required', message: 'Public DNS must resolve the name.' }, context: { ...SECONDARY, peer_host_record: peer } } });
const outcome = (status, extra = {}) => ({ status, name: 'mail.boston.celikhost.com', type: 'A', content: '198.51.100.20', ...extra });

const GUIDANCE = { reason: 'dns_secondary', record_type: 'A', primary_nameserver: 'ns1.celikhost.com', primary_ip: '203.0.113.10', record_name: 'mail.boston.celikhost.com', record_value: '198.51.100.20' };
const component = (id, name, category, installed, kind = 'service') => ({ id, name, description: `${name} on this server`, icon: '', category, kind, versions: [], config_files: [], packages: [id], ports: [], is_installed: installed, status: installed ? 'active (running)' : '', ...(installed && kind === 'service' ? { unit: id } : {}) });
const SERVICES = [component('nginx', 'Nginx', 'web', true), component('postfix', 'Postfix', 'email', false), component('dovecot', 'Dovecot', 'email', false), component('bind', 'BIND', 'dns', true)];
const profile = (id, name, services) => ({ id, name, description: `${name} for this server`, status: 'available', available: true, verified: false, latest_attempt_status: 'none', services });
const scan = (guidance) => ({ scanned_at: new Date().toISOString(), scan_age_seconds: 0, dns_identity_ready: false, services: SERVICES, profiles: [profile('core-mail', 'Core mail', ['postfix', 'dovecot'])], mail_dns_guidance: guidance });

const DRAFT = { purpose: 'web', panel_domain: 'frankfurt.celikhost.com', mail_hostname: '', dns_mode: 'local', remote_dns_connection_id: '', dns_engine: 'bind', dns_role: 'primary', ns1: 'ns1.celikhost.com', ns2: 'ns2.celikhost.com', local_ip: '203.0.113.10', peer_ip: '198.51.100.20', peer_ns: 'ns2.celikhost.com', node_version: '', database: 'mariadb', infrastructure_dns: { zone: ZONE, peer_panel_domain: 'boston.celikhost.com', peer_mail_hostname: 'mail.boston.celikhost.com' } };
const CATALOG = { version: 1, inventory_state: 'ready', presets: { web: ['nginx', 'php-fpm', 'mariadb', 'certbot'], web_mail: [], application: [], dns: [], custom: [] }, required_components: ['certbot'], components: [] };

// What the floor asks of a state, measured on the page.
const ENGLISH = ['Infrastructure zone', 'DNS only', 'DNS records only', 'Manage DNS records', 'Managed by server setup', 'Requested by the other server', 'Other server', 'mail host name', 'Nothing more is needed', 'did not create', 'no authorized DNS connection', 'Back to Domains', 'Open server setup', 'could not', 'Record to create'];
const TOKENS = /domains\.infrastructure|dns\.managed|dns\.requested|setup\.guide|setup\.infrastructure|services\.mailProfiles|REMOTE_DNS|INFRASTRUCTURE_DNS|managed_by|peer_host_record|\{[a-z]+\}/;
const facts = (page, locale) => page.evaluate((english, tokens, locale) => {
    const visible = node => node.getClientRects().length > 0 && getComputedStyle(node).visibility !== 'hidden';
    const text = [document.querySelector('main'), document.querySelector('[data-shell-page-header-target]')].filter(Boolean).map(node => node.innerText || '').join('\n');
    const words = new Set(text.split(/[^\p{L}]+/u));
    const re = new RegExp(tokens);
    const row = document.querySelector('[data-infrastructure-zone]');
    return {
        lang: document.documentElement.lang,
        englishLeaks: locale === 'tr' ? english.filter(item => (item.includes(' ') ? text.includes(item) : words.has(item))) : [],
        rawTokens: text.split('\n').filter(line => re.test(line)).map(line => line.slice(0, 160)),
        infrastructureRow: row ? row.innerText.replace(/\s+/g, ' ').trim() : null,
        infrastructureRowButtons: row ? Array.from(row.querySelectorAll('button, a')).map(node => node.title || node.innerText.trim()) : null,
        managedLabels: Array.from(document.querySelectorAll('[data-managed-by]')).filter(visible).map(node => `${node.getAttribute('data-managed-by')}: ${node.innerText.trim()}`),
        disabledDeletes: Array.from(document.querySelectorAll('main button[aria-label]')).filter(node => node.disabled && visible(node)).length,
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
        mainOverflowX: (m => (m ? m.scrollWidth > m.clientWidth + 1 : false))(document.querySelector('main')),
    };
}, ENGLISH, TOKENS.source, locale);

export default function batch18(scenarios, { base, locale, theme, out, viewport, ctl, reset, newPage, closePage, shot, into, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const snap = async (page, state, note = {}) => {
        await shot(page, `y18-${state}`, { ...(await facts(page, locale)), ...note });
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `y18-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    const whole = async (page, state, note = {}) => {
        const height = await page.evaluate(() => { const m = document.querySelector('main'); return m ? m.scrollHeight + (window.innerHeight - m.clientHeight) : document.body.scrollHeight; });
        await page.setViewport({ ...viewport, height: Math.min(Math.max(height, viewport.height), 6000) });
        await pause(500);
        await snap(page, state, { ...note, wholeHeight: height });
        await page.setViewport(viewport);
        await pause(300);
    };
    const clean = () => ctl({ clearAll: true, b12: null, b16: null, managedScan: null, execution: null, setupStatus: 'ready', domains: [], infrastructureZones: [] });
    const open = async (path, storage = {}) => {
        const page = await newPage(storage);
        await page.goto(`${base}${path}`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page); await pause(700);
        return page;
    };

    scenarios.y18domains = async () => {
        await reset(); await clean();
        await ctl({ domains: [DOMAIN], infrastructureZones: ZONES });
        let page = await open('/domains');
        await waitFor(page, () => !!document.querySelector('[data-infrastructure-zone]'), 15000).catch(() => {});
        await snap(page, 'domains');
        await closePage(page);
        await ctl({ override: { '/api/v1/dns/infrastructure-zones': { status: 503, body: { error: 'unavailable', code: 'AGENT_UNREACHABLE' } } } });
        page = await open('/domains');
        await pause(800);
        await snap(page, 'domains-unread');
        await closePage(page);
        await clean();
    };

    scenarios.y18zone = async () => {
        await reset(); await clean();
        await ctl({ infrastructureZones: ZONES, override: zoneServed });
        const page = await open(`/domains/infrastructure/${ZONE}`);
        await waitFor(page, () => !!document.querySelector('[data-managed-by]'), 15000).catch(() => {});
        await whole(page, 'zone');
        await closePage(page);
        await clean();
    };

    scenarios.y18plan = async () => {
        await reset(); await clean();
        await ctl({ b16: { status: 'draft', guidance: 'guided', draft: structuredClone(DRAFT), revision: 7, extension: false, catalog: CATALOG }, setupStatus: 'draft' });
        const page = await open('/setup');
        const field = '#setup-peer_mail_hostname';
        if (!(await page.$(field))) {
            await page.$eval('main form button[type="submit"]', node => node.click()).catch(() => {});
            await pause(1200);
        }
        await waitFor(page, (selector) => !!document.querySelector(selector), 10000, field).catch(() => {});
        await into(page, field);
        await snap(page, 'plan-field');
        await closePage(page);
        await clean();
    };

    scenarios.y18setup = async () => {
        // The mock's execution carries its own plan id; the marker is adopted by request id (as in batch 16's x16check).
        const marker = { 'celikpanel.setup.start.admin': JSON.stringify({ request_id: REQ_ID, plan_id: 'a'.repeat(32), panel_domain: 'boston.celikhost.com' }) };
        for (const [state, peer] of [
            ['setup-applied', { channel: true, outcome: outcome('applied', { result: 'created', zone: ZONE }) }],
            ['setup-refused', { channel: true, outcome: outcome('refused', { code: 'REMOTE_DNS_HOST_RECORD_CONFLICT' }) }],
            ['setup-nochannel', { channel: false, outcome: outcome('none') }],
        ]) {
            await reset(); await clean();
            await ctl({ setupStatus: 'waiting', execution: waiting(peer) });
            const page = await newPage(marker);
            await page.goto(`${base}/setup`, { waitUntil: 'networkidle0' });
            await page.waitForSelector('#setup-progress-title', { timeout: 20000 }).catch(() => {});
            await quiet(page); await pause(900);
            await whole(page, state);
            await closePage(page);
        }
        await clean();
    };

    scenarios.y18mail = async () => {
        for (const [state, channel] of [['mail-channel', true], ['mail-nochannel', false]]) {
            await reset(); await clean();
            await ctl({ b16: { status: 'ready', guidance: 'guided', draft: structuredClone(DRAFT), revision: 7, extension: false, catalog: CATALOG }, managedScan: scan({ ...GUIDANCE, peer_record_channel: channel }), b12: { scan: 'stored' } });
            const page = await open('/services');
            await waitFor(page, () => !!document.querySelector('[data-mail-dns-secondary]'), 15000).catch(() => {});
            await into(page, '[data-mail-dns-secondary]');
            await snap(page, state);
            await closePage(page);
        }
        await clean();
    };
}
