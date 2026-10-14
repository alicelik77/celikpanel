// Batch 12 (2026-10-14): the waiting states changed on the alpha.85 branch
// (docs/OPERATION-GUIDANCE.md, the two entries of 2026-10-14), photographed
// for the owner's rule: a screen that makes the owner wait either visibly
// waits (motion, interfering controls off) or says plainly that nothing needs
// to be done, and it never says "Waiting for you" when the owner's part is done.
//
//   w85setup     the final setup step waiting for reverse DNS; "Finishing
//                setup" after a passing check; its 60 s unknown-result notice
//   w85reconnect setup reconnecting after a lost read; its 60 s notice
//   w85components the Components page: the firewall banner refused with
//                AGENT_UNREACHABLE, the readiness pause, a component page's
//                install dialog while the server is busy; the mail profile
//                cards with typed codes and with an older server's text only
//   w85dashboard "Needs attention" naming the Agent; the system services and
//                mail stack cards checking, known, could not check, not
//                checked recently, held by a change
//   w85overlay   the component operation lock after a lost connection; its
//                60 s notice
//   w85update    the panel update card while the server is busy
//   w85spread    a domain's DNS change spreading, and the automatic checks
//                stopped after 30 minutes
//   w85ssl       a certificate request running; the HSTS retirement dated by
//                the server's time left (server clock 7 minutes behind)
//   w85sidebar   the sidebar build id (the menu opened on a phone)
//
// Screenshots also go to BROWSER_INSPECT_SHOTS (default: the run's out dir) as
// <state>-<lang>-<theme>-<width>.png.
//
// The 60 s bounds and the 30 s spreading cadence are not shortened in the
// product. Where a state lies behind one of them, the page under test gets a
// test-side clock control before it loads: window.setTimeout called with
// exactly that delay waits a shorter one instead (`fastTimers`). Nothing else
// changes, and the shortening is recorded with each state.
//
// Like the batches before: the mock on 127.0.0.1 answers everything; nothing
// else is contacted, nothing is started.
// On ikinci grup: alpha.85 dalindaki bekleme durumlari.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';

const HOST = 'panel.example.com';
const REQ_ID = 'd'.repeat(32);
const PLAN_ID = 'a'.repeat(32);
const MARKER = { 'celikpanel.setup.start.admin': JSON.stringify({ request_id: REQ_ID, plan_id: PLAN_ID, panel_domain: HOST }) };
const CONTEXT = { dns_mode: 'external', dns_role: '', dns_engine: 'bind', local_nameserver: '', local_ip: '', peer_nameserver: '', peer_ip: '', panel_domain: HOST, mail_hostname: 'mail.example.com', dns_hosting_management: '' };
const PTR = { id: 'mail_identity', state: 'action_required', code: 'mail_identity_required', reason: 'reverse_dns_mismatch', vars: { hostname: 'mail.example.com', ip: '203.0.113.10', ptr: 'static.10.113.0.203.provider.example' } };
const DONE = ['succeeded', 'succeeded', 'succeeded', 'succeeded', 'succeeded', 'succeeded', 'pending'];
const READY_CHECKS = [{ id: 'mail_identity', state: 'ready', code: 'ready' }, { id: 'panel_https', state: 'ready', code: 'ready' }];

const CAPABILITIES = { dns_server: 'bind', dns_identity_ready: true, dns_management_mode: 'local', dns_management_ready: true, web_server: 'nginx', php_versions: ['8.3'], mail_server: true, database_servers: ['mariadb'], db_tools: [] };
const DOMAIN = { id: 1, domain_name: 'example.com', status: 'active', project_type: 'php', php_version: '8.3', ssl_enabled: true, created_at: '2026-09-01T10:00:00Z', disk_usage: 48234496, bandwidth: 0 };
const CONNECTION = { domain: 'example.com', server_ip: '192.0.2.4', nameservers: ['ns1.example.net', 'ns2.example.net'], live_nameservers: ['ns1.example.net', 'ns2.example.net'], live_ips: ['192.0.2.4'], status: 'delegated', ssl_ready: true, glue_needed: false, nameservers_usable: true, propagation_pending: false, checked_at: '2026-10-14T09:00:00Z', dns_management_mode: 'local' };
const SPREADING = {
    ...CONNECTION, propagation_pending: true, checked_at: '2026-10-14T09:41:07Z',
    resolver_observations: [
        { resolver: '1.1.1.1', nameservers: ['ns1.example.net', 'ns2.example.net'], ips: ['192.0.2.4'], status: 'delegated', ssl_ready: true },
        { resolver: '8.8.8.8', nameservers: ['dns1.registrar-servers.example', 'dns2.registrar-servers.example'], ips: ['198.51.100.7'], status: 'elsewhere', ssl_ready: false },
    ],
};
const CERTIFICATE = {
    id: 4, type: 'letsencrypt', provider_id: 'letsencrypt', issuer: 'R11', subject: 'example.com', issued_at: '2026-10-09T08:00:00Z', expires_at: '2027-01-07T08:00:00Z',
    days_until_expiry: 85, auto_renew: true, renewal_status: 'ok', status: 'active', dns_names: ['example.com', 'www.example.com'],
    activated: true, usable: true, trust_status: 'trusted', activation_pending: false, dependents_pending: false,
};

const component = (id, name, category, extra = {}) => ({ id, name, description: `${name} on this server`, icon: '', category, kind: 'service', unit: id, versions: [], status: 'active (running)', is_installed: true, config_files: [], packages: [id], ports: [], ...extra });
const absent = (id, name, category, extra = {}) => component(id, name, category, { status: '', is_installed: false, unit: undefined, ...extra });
const SERVICES = [
    component('nginx', 'Nginx', 'web'),
    component('postfix', 'Postfix', 'mail'), component('dovecot', 'Dovecot', 'mail'),
    absent('roundcube', 'Roundcube', 'mail', { kind: 'tool' }),
    absent('rspamd', 'Rspamd', 'mail'),
    absent('redis', 'Redis', 'cache', { packages: ['redis-server'] }),
];
const profile = (id, name, services, extra = {}) => ({ id, name, description: `${name} for this server`, status: 'available', available: true, verified: false, latest_attempt_status: 'none', services, ...extra });
// Typed codes beside the server's English text (cmd/panel/mail_profiles.go, 2026-10-14).
const TYPED = [
    profile('core-mail', 'Core mail', ['postfix', 'dovecot'], {
        status: 'blocked', available: false,
        blocked_reason: 'Postfix conflicts with the installed Exim.', blocked_code: 'component_conflict', blocked_vars: { component: 'Postfix', conflicting: 'Exim' },
    }),
    profile('webmail', 'Webmail', ['roundcube'], {
        status: 'partial', latest_attempt_status: 'failed', latest_attempt_error: 'roundcube: php8.3-fpm socket /run/php/roundcube.sock not found',
        warning: 'Roundcube is installed, but its Unix-socket runtime is not ready.', warning_code: 'roundcube_not_ready',
    }),
    profile('protected-mail', 'Protected mail', ['rspamd'], {
        warning: 'Some components are already present. Installing this profile will reconcile the full plan without removing completed services.', warning_code: 'partly_present',
    }),
];
// An older Panel: the same answer without the codes.
const OLDER = TYPED.map(({ blocked_code: _b, blocked_vars: _v, warning_code: _w, ...rest }) => rest);
const scan = (profiles, ageSeconds = 0) => ({
    scanned_at: new Date(Date.now() - ageSeconds * 1000).toISOString(), scan_age_seconds: ageSeconds, dns_identity_ready: true,
    mail_hostname: { current: 'server1.example.com', hostname: '', source: '', current_usable: true, will_set_hostname: false },
    services: SERVICES, profiles,
});
const BUSY = { ready: false, code: 'HOST_MUTATION_BUSY', reason: 'package_manager_active' };
const AGENT_DOWN = { status: 503, body: { error: 'the CelikPanel Agent is not reachable', code: 'AGENT_UNREACHABLE' } };
const TARGET = { version: 'v0.1.0-alpha.85', commit: 'b'.repeat(40), sequence: '85', os: 'linux', arch: 'amd64', archive_sha256: 'c'.repeat(64), archive_size: '65870672' };
const CHECK = { supported: true, available: true, current_version: 'v0.1.0-alpha.84', current_commit: 'a'.repeat(40), target: TARGET };
const VERSION = { status: 200, body: { version: 'v0.1.0-alpha.84', commit: 'f0157e6c8f16a2b3c4d5e6f708192a3b4c5d6e7f', agent_commit: 'f0157e6c8f16a2b3c4d5e6f708192a3b4c5d6e7f', agent_matches: true, hostname: 'server1', ipv4: '203.0.113.10' } };

// What the owner's rule asks of each state, measured on the page.
const facts = page => page.evaluate(() => {
    const visible = node => node.getClientRects().length > 0 && getComputedStyle(node).visibility !== 'hidden';
    const spinners = Array.from(document.querySelectorAll('.animate-spin, [class*="animate-spin"]')).filter(visible).length;
    const buttons = Array.from(document.querySelectorAll('main button, [role="dialog"] button')).filter(visible)
        .map(node => `${(node.innerText || node.getAttribute('aria-label') || '').trim().replace(/\s+/g, ' ').slice(0, 40)}${node.disabled ? ' (disabled)' : ''}`).filter(Boolean);
    return { spinners, buttons, pageOverflowX: document.documentElement.scrollWidth > window.innerWidth };
});
const fastTimers = (page, map) => page.evaluateOnNewDocument(shorter => {
    const original = window.setTimeout;
    window.setTimeout = function (fn, ms, ...rest) {
        const key = String(ms);
        return original.call(window, fn, Object.prototype.hasOwnProperty.call(shorter, key) ? shorter[key] : ms, ...rest);
    };
}, map);

export default function batch12(scenarios, { base, vp, locale, theme, out, viewport, ctl, reset, newPage, closePage, shot, into, clickByText, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const snap = async (page, state, note = {}) => {
        await shot(page, `w85-${state}`, { ...(await facts(page)), ...note });
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `w85-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    const centre = (page, pattern) => page.evaluate(source => {
        const re = new RegExp(source, 'i');
        const node = Array.from(document.querySelectorAll('main p, main span, main h2, main h3, [role="dialog"] p, [role="dialog"] div'))
            .filter(item => item.getClientRects().length > 0 && re.test(item.innerText || '')).sort((a, b) => a.innerText.length - b.innerText.length)[0];
        node?.scrollIntoView({ block: 'center' });
        return !!node;
    }, pattern.source);
    const clean = () => ctl({ clearAll: true, b12: null, managedScan: null });

    scenarios.w85setup = async () => {
        await reset(); await clean();
        await ctl({ setupStatus: 'waiting', execution: { request_id: REQ_ID, statuses: DONE, extra: { status: 'waiting', phase: 'verification', context: CONTEXT, checks: [PTR] } }, b12: { manualChecks: READY_CHECKS } });
        const page = await newPage(MARKER);
        await fastTimers(page, { 60000: 6000 });
        await page.goto(`${base}/setup`, { waitUntil: 'networkidle0' });
        await page.waitForSelector('#setup-progress-title', { timeout: 20000 });
        await pause(600);
        await into(page, '#setup-progress-title');
        await snap(page, 'setup-rdns');
        await into(page, '[data-step-state="waitingReverseDNS"]');
        await snap(page, 'setup-rdns-step');
        await clickByText(page, ['Check requirements again', 'Gereksinimleri tekrar kontrol et']);
        await waitFor(page, () => !!document.querySelector('[data-setup-finishing]'), 15000).catch(() => {});
        await pause(400);
        await into(page, '[data-setup-finishing]');
        await snap(page, 'setup-finishing', { clock: '60000 ms shown as 6000 ms' });
        await into(page, '[data-step-state="finishing"]');
        await snap(page, 'setup-finishing-step', { clock: '60000 ms shown as 6000 ms' });
        await waitFor(page, () => /systemctl restart celikpanel-panel/.test(document.querySelector('main')?.innerText || ''), 15000).catch(() => {});
        await pause(400);
        await centre(page, /systemctl restart celikpanel-panel/);
        await snap(page, 'setup-finishing-slow', { clock: '60000 ms shown as 6000 ms' });
        await closePage(page);
        await clean();
    };

    scenarios.w85reconnect = async () => {
        await reset(); await clean();
        await ctl({ setupStatus: 'running', execution: { request_id: REQ_ID, statuses: ['succeeded', 'running'], extra: { phase: 'service-php-fpm' } } });
        const page = await newPage(MARKER);
        await fastTimers(page, { 60000: 6000 });
        await page.goto(`${base}/setup`, { waitUntil: 'networkidle0' });
        await page.waitForSelector('#setup-progress-title', { timeout: 20000 });
        await pause(500);
        await ctl({ override: { '/api/v1/setup/operation': { drop: true } } });
        await waitFor(page, () => !!document.querySelector('[data-setup-reconnecting]'), 15000).catch(() => {});
        await pause(400);
        await into(page, '#setup-progress-title');
        await snap(page, 'setup-reconnecting', { clock: '60000 ms shown as 6000 ms' });
        await waitFor(page, () => !!document.querySelector('[data-setup-reconnect-slow]'), 15000).catch(() => {});
        await pause(400);
        await into(page, '#setup-progress-title');
        await snap(page, 'setup-reconnect-slow', { clock: '60000 ms shown as 6000 ms' });
        await closePage(page);
        await clean();
    };

    scenarios.w85components = async () => {
        // The firewall banner refused because the Agent is not reachable.
        await reset(); await clean();
        await ctl({ managedScan: scan(TYPED), b12: { scan: 'stored' }, override: { '/api/v1/firewall': AGENT_DOWN } });
        let page = await newPage();
        await page.goto(`${base}/services`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page); await pause(800);
        await centre(page, /celikpanel-agent/);
        await snap(page, 'comp-firewall-agent');
        await closePage(page);

        // The readiness pause, lifted by the page itself.
        await ctl({ clearAll: true });
        await ctl({ override: { '/api/v1/host-mutation-readiness': { status: 200, body: BUSY } } });
        page = await newPage();
        await page.goto(`${base}/services`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page); await pause(800);
        await centre(page, /lifts this pause by itself|beklemeyi kendiliğinden kaldırır/);
        await snap(page, 'comp-readiness-pause');
        // The mail profile cards with typed codes.
        await into(page, '#mail-stacks');
        await snap(page, 'comp-mail-typed');
        await closePage(page);

        // The same cards from an older Panel: text only.
        await ctl({ clearAll: true, managedScan: scan(OLDER) });
        page = await newPage();
        await page.goto(`${base}/services`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page); await pause(800);
        await into(page, '#mail-stacks');
        await snap(page, 'comp-mail-older');
        await closePage(page);

        // A component page's install dialog while the server is busy.
        await ctl({ managedScan: scan(TYPED), override: { '/api/v1/host-mutation-readiness': { status: 200, body: BUSY } } });
        page = await newPage();
        await page.goto(`${base}/services/redis`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page); await pause(600);
        await clickByText(page, ['Install Redis', 'Redis kur'], 'main button');
        await waitFor(page, () => /lifts this pause by itself|beklemeyi kendiliğinden kaldırır/.test(document.querySelector('[role="dialog"]')?.innerText || ''), 15000).catch(() => {});
        await pause(400);
        await snap(page, 'comp-install-readiness');
        await closePage(page);
        await clean();
    };

    scenarios.w85dashboard = async () => {
        const open = async () => {
            const page = await newPage();
            await page.goto(`${base}/`, { waitUntil: 'domcontentloaded' });
            await page.waitForSelector('main', { timeout: 20000 });
            await pause(1500);
            return page;
        };
        const both = async (page, state) => {
            await page.evaluate(() => document.getElementById('dashboard-mail-stacks-heading')?.closest('section')?.scrollIntoView({ block: 'center' }));
            await pause(300);
            await snap(page, `dash-${state}-mail`);
            await centre(page, /^(System services|Sistem servisleri)$/);
            await snap(page, `dash-${state}-services`);
        };
        await reset(); await clean();
        await ctl({ domains: [DOMAIN], managedScan: scan(TYPED), b12: { scan: 'stored' }, override: { '/api/v1/firewall': AGENT_DOWN } });
        let page = await open();
        await quiet(page);
        await centre(page, /celikpanel-agent/);
        await snap(page, 'dash-attention-agent');
        await closePage(page);

        for (const [state, b12, wait] of [
            ['checking', { scan: 'stored', scanDelay: 60000 }, 2500],
            ['result', { scan: 'stored' }, 0],
            ['failed', { scan: 'fail' }, 0],
            ['stale', { scan: 'stale' }, 0],
            ['held', { scan: 'held' }, 0],
        ]) {
            await ctl({ clearAll: true, managedScan: scan(TYPED, state === 'result' ? 0 : 1200), b12 });
            page = await open();
            if (wait) await pause(wait); else await quiet(page);
            await both(page, state);
            await closePage(page);
        }
        await clean();
    };

    scenarios.w85overlay = async () => {
        await reset(); await clean();
        await ctl({ componentOperation: { id: 'c'.repeat(32), request_id: 'e'.repeat(32), kind: 'service_install', service_id: 'nginx', package_name: 'nginx', status: 'running', phase: 'package_install', started_at: new Date().toISOString() } });
        const page = await newPage();
        await fastTimers(page, { 60000: 8000 });
        await page.goto(`${base}/domains`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('[aria-labelledby="component-operation-title"]', { timeout: 20000 }).catch(() => {});
        await pause(800);
        await ctl({ override: { '/api/v1/service/operation': { drop: true } } });
        await waitFor(page, () => !!document.querySelector('[data-operation-reconnecting]'), 20000).catch(() => {});
        await pause(500);
        await snap(page, 'overlay-reconnecting', { clock: '60000 ms shown as 8000 ms' });
        await waitFor(page, () => !!document.querySelector('[data-operation-reconnect-slow]'), 20000).catch(() => {});
        await pause(500);
        await snap(page, 'overlay-reconnect-slow', { clock: '60000 ms shown as 8000 ms' });
        await closePage(page);
        await ctl({ componentOperation: null });
        await clean();
    };

    scenarios.w85update = async () => {
        await reset(); await clean();
        await ctl({ override: { '/api/v1/panel/update/check': { method: 'GET', status: 200, body: CHECK }, '/api/v1/host-mutation-readiness': { status: 200, body: BUSY } } });
        const page = await newPage();
        await page.goto(`${base}/settings?section=updates`, { waitUntil: 'domcontentloaded' });
        await waitFor(page, () => Array.from(document.querySelectorAll('main button')).some(node => ['Check for updates', 'Güncellemeyi kontrol et'].includes((node.innerText || '').trim()) && !node.disabled), 20000).catch(() => {});
        await clickByText(page, ['Check for updates', 'Güncellemeyi kontrol et'], 'main button');
        await waitFor(page, () => !!document.querySelector('[data-readiness-auto-resume]'), 20000).catch(() => {});
        await pause(600);
        await into(page, '[data-readiness-auto-resume]');
        await snap(page, 'update-busy');
        await closePage(page);
        await clean();
    };

    scenarios.w85spread = async () => {
        await reset(); await clean();
        await ctl({ domains: [DOMAIN], capabilities: CAPABILITIES, connection: SPREADING });
        let page = await newPage();
        await page.goto(`${base}/domains/example.com`, { waitUntil: 'domcontentloaded' });
        await waitFor(page, () => !!document.querySelector('[data-conn-spread-auto]'), 20000).catch(() => {});
        await pause(500);
        await into(page, '[data-conn-spread-auto]');
        await snap(page, 'conn-spreading');
        await closePage(page);

        page = await newPage();
        await fastTimers(page, { 30000: 40 });
        await page.goto(`${base}/domains/example.com`, { waitUntil: 'domcontentloaded' });
        await waitFor(page, () => document.querySelector('[data-conn-spread-auto]')?.getAttribute('data-conn-spread-auto') === 'stopped', 60000).catch(() => {});
        await pause(500);
        await into(page, '[data-conn-spread-auto]');
        await snap(page, 'conn-spread-stopped', { clock: '30000 ms shown as 40 ms (60 checks)' });
        await closePage(page);
        await clean();
    };

    scenarios.w85ssl = async () => {
        const openSSL = async () => {
            const page = await newPage();
            page.on('dialog', dialog => dialog.accept());
            await page.goto(`${base}/domains/example.com`, { waitUntil: 'domcontentloaded' });
            await waitFor(page, () => Array.from(document.querySelectorAll('main button')).some(node => ['Hosting', 'Barındırma'].includes((node.innerText || '').trim())), 15000);
            await quiet(page);
            await clickByText(page, ['Hosting', 'Barındırma'], 'main button'); await pause(300);
            await clickByText(page, ['SSL/TLS'], 'main button'); await pause(900);
            await quiet(page);
            return page;
        };
        await reset(); await clean();
        await ctl({ domains: [DOMAIN], capabilities: CAPABILITIES, connection: CONNECTION, ssl: null, override: { '/api/v1/domains/1/ssl/letsencrypt': { method: 'POST', delay: 60000, status: 200, body: { success: true } } } });
        let page = await openSSL();
        const email = await page.$('main input[type="email"]');
        if (email && !(await email.evaluate(node => node.value))) await email.type('owner@example.com');
        await clickByText(page, ['Get certificate', 'Sertifika al'], 'main button');
        await pause(900);
        await centre(page, /^(Issuing…|Alınıyor…)$/);
        await snap(page, 'ssl-issuing');
        await closePage(page);

        // The server's clock is 7 minutes behind: its stamp alone would read
        // 1 h 23 min left; the time left it states is 1 h 30 min.
        const skew = 7 * 60_000;
        await ctl({ clearAll: true, ssl: { domain_id: 1, domain_name: 'example.com', has_certificate: true, managed_names: ['example.com', 'www.example.com'], certificate: CERTIFICATE, settings: { force_https: true, hsts_enabled: false, hsts_max_age: 300, hsts_retire_after: new Date(Date.now() - skew + 90 * 60_000).toISOString(), hsts_retire_remaining_seconds: 5400 } } });
        page = await openSSL();
        await centre(page, /HSTS (retirement is active|emeklilik)/);
        await snap(page, 'ssl-hsts-countdown', { serverClock: '7 minutes behind' });
        await closePage(page);
        await ctl({ ssl: null });
        await clean();
    };

    scenarios.w85sidebar = async () => {
        await reset(); await clean();
        await ctl({ domains: [DOMAIN], override: { '/api/v1/panel/version': VERSION } });
        const page = await newPage();
        await page.goto(`${base}/domains`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page); await pause(500);
        if (vp === 'phone') {
            await page.click('button[aria-label="Menu"]').catch(() => {});
            await pause(700);
        }
        const stamp = await page.evaluate(() => Array.from(document.querySelectorAll('[data-build-stamp]')).filter(node => node.getClientRects().length > 0)
            .map(node => ({ text: node.innerText, lines: node.getClientRects().length, rect: (r => ({ x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.width), h: Math.round(r.height) }))(node.getBoundingClientRect()) })));
        await snap(page, 'sidebar-build', { stamp });
        await closePage(page);
        await clean();
    };
}
