// Batch 16 (2026-10-16): the second server as a full peer, part 1 (branch
// feat/alpha87-second-server; the 2026-10-16 entry of
// docs/OPERATION-GUIDANCE.md), photographed for the craft floor, D-024 and
// D-033: Turkish complete, no raw ids, no overflow at 390 px, dark legible,
// interfering controls off while locked, no dead-end button, every waiting
// state moves or says that nothing needs to be done.
//
//   x16extend     the ready screen with "Extend this server's setup", a refused
//                 begin (a setup run in progress), the extension wizard:
//                 purpose prefilled, components with the installed ones kept,
//                 access with a new mail host name, the review's "what
//                 changes / to be installed / kept" blocks, "Stop extending"
//                 and the ready screen it returns to
//   x16notice     the Dashboard notice and Settings -> Server setup while an
//                 extension is in draft and while its run is active, and
//                 Settings for a completed setup; the setup page of that run
//   x16mail       the Components mail profile dialog on a prepared server (the
//                 setup route and its note), "Change" with the effects note,
//                 the request held and then refused as busy (the dialog stays
//                 open), the route that succeeds into setup's access step and
//                 its review; on an unprepared server the direct route with
//                 the self-signed note, and "Change" there
//   x16secondary  the DNS-secondary box with the primary named and the exact
//                 record, with no primary name and setup unfinished, and where
//                 its button leads
//   x16check      the setup final step: "Check requirements again" answered
//                 with the typed reverse DNS sentence (resolver named) and a
//                 check that was not read
//
// Every answer comes from the mock on 127.0.0.1 (mock.mjs, mock-batch16.mjs,
// mock-batch12.mjs and overrides); nothing else is contacted and nothing is
// started. Screenshots also go to BROWSER_INSPECT_SHOTS (default: the run's
// out dir) as <state>-<lang>-<theme>-<width>.png.
// On altinci grup: ikinci sunucu tam es, birinci bolum.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';

const HOST = 'panel.example.com';
const REQ_ID = 'd'.repeat(32);
const PLAN_ID = 'f'.repeat(32);
const EXEC_ID = 'b'.repeat(32);
const MARKER = { 'celikpanel.setup.start.admin': JSON.stringify({ request_id: REQ_ID, plan_id: PLAN_ID, panel_domain: HOST }) };

const row = (id, name, category, installed, dependencies = []) => ({ id, name, category, dependencies, conflicts: [], supported: true, installed });
const CATALOG = {
    version: 1, inventory_state: 'ready',
    presets: { web: ['nginx', 'php-fpm', 'mariadb', 'certbot'], web_mail: ['nginx', 'php-fpm', 'mariadb', 'certbot', 'postfix', 'dovecot'], application: ['nginx', 'certbot'], dns: [], custom: [] },
    required_components: ['certbot'],
    components: [
        row('nginx', 'Nginx', 'web', true), row('php-fpm', 'PHP-FPM', 'web', true), row('mariadb', 'MariaDB', 'database', true),
        row('postgresql', 'PostgreSQL', 'database', false), row('redis', 'Redis', 'cache', false),
        row('postfix', 'Postfix', 'email', false, ['dovecot']), row('dovecot', 'Dovecot', 'email', false, ['postfix']),
        row('rspamd', 'Rspamd', 'email', false, ['postfix', 'redis']), row('roundcube', 'Roundcube', 'email', false, ['postfix', 'dovecot', 'nginx', 'php-fpm']),
        row('certbot', 'Certbot', 'security', true),
    ],
};
const DRAFT = { purpose: 'web', panel_domain: HOST, mail_hostname: '', dns_mode: 'external', remote_dns_connection_id: '', dns_engine: 'bind', dns_role: 'primary', ns1: '', ns2: '', local_ip: '', peer_ip: '', peer_ns: '', node_version: '', database: 'mariadb' };
const b16 = (extra = {}) => ({ status: 'ready', guidance: 'guided', draft: structuredClone(DRAFT), revision: 7, extension: false, catalog: CATALOG, ...extra });
const EXTENDED = { ...DRAFT, purpose: 'web_mail', mail_hostname: 'mail.example.com', customization: { components: CATALOG.presets.web_mail } };

// The running extension, as GET /api/v1/setup/operation reports it.
const RUN_STEPS = [
    ['service', 'redis'], ['dns', 'external'], ['mail_profile', 'core-mail'], ['firewall', 'enable_and_persist'],
    ['access_dns', HOST], ['access_dns', 'mail.example.com'], ['mail_certificate', 'mail.example.com'], ['verify', 'web_mail'],
];
const runBody = statuses => ({
    id: EXEC_ID, request_id: REQ_ID, plan_id: PLAN_ID, status: 'running', phase: 'mail_profile',
    steps: RUN_STEPS.map(([kind, target], index) => ({ id: `${kind}-${index}`, kind, target, status: statuses[index] || 'pending' })),
    context: { dns_mode: 'external', dns_role: '', dns_engine: 'bind', local_nameserver: '', local_ip: '', peer_nameserver: '', peer_ip: '', panel_domain: HOST, mail_hostname: 'mail.example.com', dns_hosting_management: '' },
});

// The Components page's record (the whole contract of /managed-services).
const component = (id, name, category, installed, kind = 'service') => ({ id, name, description: `${name} on this server`, icon: '', category, kind, versions: [], config_files: [], packages: [id], ports: [], is_installed: installed, status: installed ? 'active (running)' : '', ...(installed && kind === 'service' ? { unit: id } : {}) });
const SERVICES = [
    component('nginx', 'Nginx', 'web', true), component('php-fpm', 'PHP-FPM', 'web', true, 'runtime'), component('mariadb', 'MariaDB', 'database', true),
    component('certbot', 'Certbot', 'security', true, 'tool'), component('postfix', 'Postfix', 'email', false), component('dovecot', 'Dovecot', 'email', false),
    component('rspamd', 'Rspamd', 'email', false), component('roundcube', 'Roundcube', 'email', false, 'tool'), component('bind', 'BIND', 'dns', true),
];
const profile = (id, name, services) => ({ id, name, description: `${name} for this server`, status: 'available', available: true, verified: false, latest_attempt_status: 'none', services });
const PROFILES = [profile('core-mail', 'Core mail', ['postfix', 'dovecot']), profile('webmail', 'Webmail', ['roundcube']), profile('protected-mail', 'Protected mail', ['rspamd'])];
const SAVED_NAME = { current: 'server1.example.com', hostname: 'mail.example.com', source: 'saved', current_usable: true, will_set_hostname: false };
const scan = (extra = {}) => ({ scanned_at: new Date().toISOString(), scan_age_seconds: 0, dns_identity_ready: true, mail_hostname: SAVED_NAME, services: SERVICES, profiles: PROFILES, ...extra });
const GUIDANCE = { reason: 'dns_secondary', record_type: 'A', primary_nameserver: 'ns1.example.com', primary_ip: '198.51.100.53', record_name: 'mail.example.com', record_value: '203.0.113.10' };

// The setup final step: the reverse DNS wait with its resolver, and a check
// the server could not read.
const CONTEXT = { dns_mode: 'external', dns_role: '', dns_engine: 'bind', local_nameserver: '', local_ip: '', peer_nameserver: '', peer_ip: '', panel_domain: HOST, mail_hostname: 'mail.example.com', dns_hosting_management: '' };
const DONE = ['succeeded', 'succeeded', 'succeeded', 'succeeded', 'succeeded', 'succeeded', 'pending'];
const PTR = { id: 'mail_identity', state: 'action_required', code: 'mail_identity_required', reason: 'reverse_dns_mismatch', vars: { hostname: 'mail.example.com', ip: '203.0.113.10', ptr: 'static.10.113.0.203.provider.example', resolver: '1.1.1.1' } };
const NOT_READ = { id: 'mail_delivery', state: 'unknown', code: 'mail_delivery_unknown' };

// What the floor asks of a state, measured on the page.
const ENGLISH = ['Extend this server', 'Extend setup', 'Stop extending', 'What changes', 'To be installed', 'Already installed', 'kept as they are', 'Continue extending', 'Extending this server', 'Review in server setup', 'Record to create', 'secondary DNS server', 'primary DNS server', 'self-signed', 'Keep mail', 'Keep mx', 'Change', 'could not be read', 'not read', 'Check requirements again', 'Customize selection', 'Installed', 'Keep existing installation', 'Server purpose', 'Mail server name', 'none'];
const TOKENS = /server_setup|setup\.extend|services\.mailProfiles|mail_profile|mail_dns|dns_secondary|core-mail|protected-mail|mail_identity|mail_delivery|reverse_dns|web_mail|enable_and_persist|\{[a-z]+\}/;
const facts = (page, locale) => page.evaluate((english, tokens, locale) => {
    const visible = node => node.getClientRects().length > 0 && getComputedStyle(node).visibility !== 'hidden';
    const text = [document.querySelector('main'), ...document.querySelectorAll('[role="dialog"]')].filter(Boolean).map(node => node.innerText || '').join('\n');
    const words = new Set(text.split(/[^\p{L}]+/u));
    const buttons = Array.from(document.querySelectorAll('main button, main a[href], [role="dialog"] button, [role="dialog"] input')).filter(visible)
        .map(node => `${node.tagName === 'INPUT' ? `input#${node.id}` : (node.innerText || node.getAttribute('aria-label') || node.title || '').trim().replace(/\s+/g, ' ').slice(0, 44)}${node.disabled || node.closest('fieldset:disabled') ? ' (disabled)' : ''}${node.getAttribute('aria-busy') === 'true' ? ' (busy)' : ''}`).filter(Boolean);
    const re = new RegExp(tokens);
    return {
        lang: document.documentElement.lang,
        englishLeaks: locale === 'tr' ? english.filter(item => (item.includes(' ') ? text.includes(item) : words.has(item))) : [],
        rawTokens: text.split('\n').filter(line => re.test(line)).map(line => line.slice(0, 160)),
        alerts: Array.from(document.querySelectorAll('main [role="alert"], [role="dialog"] [role="alert"]')).filter(visible).map(node => node.innerText.trim().slice(0, 200)),
        buttons: buttons.slice(0, 50),
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
        mainOverflowX: (m => (m ? m.scrollWidth > m.clientWidth + 1 : false))(document.querySelector('main')),
        dialogOverflowX: Array.from(document.querySelectorAll('[role="dialog"]')).some(node => node.scrollWidth > node.clientWidth + 1),
    };
}, ENGLISH, TOKENS.source, locale);

export default function batch16(scenarios, { base, locale, theme, out, viewport, ctl, reset, newPage, closePage, shot, into, clickByText, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const snap = async (page, state, note = {}) => {
        await shot(page, `x16-${state}`, { ...(await facts(page, locale)), ...note });
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `x16-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    // The whole of <main>: the viewport is made as tall as its content.
    const whole = async (page, state, note = {}) => {
        const height = await page.evaluate(() => { const m = document.querySelector('main'); return m ? m.scrollHeight + (window.innerHeight - m.clientHeight) : document.body.scrollHeight; });
        await page.setViewport({ ...viewport, height: Math.min(Math.max(height, viewport.height), 9000) });
        await pause(500);
        await snap(page, state, { ...note, wholeHeight: height });
        await page.setViewport(viewport);
        await pause(300);
    };
    const clean = () => ctl({ clearAll: true, b12: null, b16: null, managedScan: null, execution: null, setupStatus: 'ready' });
    const open = async (path, storage = {}) => {
        const page = await newPage(storage);
        await page.goto(`${base}${path}`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page); await pause(600);
        return page;
    };
    const click = async (page, selector) => { await page.waitForSelector(selector, { visible: true, timeout: 10000 }); await page.$eval(selector, node => node.scrollIntoView({ block: 'center' })); await pause(150); await page.click(selector); };
    const submit = page => click(page, 'main form button[type="submit"]');
    const type = async (page, selector, value) => { await page.waitForSelector(selector, { visible: true, timeout: 10000 }); await page.click(selector, { clickCount: 3 }); await page.keyboard.press('Backspace'); await page.type(selector, value); await pause(250); };
    // The first mail profile card's own button (Core mail): "Install" / "Kur".
    const profileButton = async page => {
        await page.evaluate(() => document.querySelector('#mail-stacks')?.scrollIntoView({ block: 'start' }));
        await pause(300);
        const handle = await page.evaluateHandle(() => Array.from(document.querySelectorAll('#mail-stacks button'))
            .find(item => item.getClientRects().length > 0 && /^(Install|Kur)$/.test((item.innerText || '').trim())) || null);
        const element = handle.asElement();
        if (!element) throw new Error('no Install button in the mail profile cards');
        await element.click();
        await page.waitForSelector('[role="dialog"]', { timeout: 10000 });
        await pause(500);
    };

    scenarios.x16extend = async () => {
        await reset(); await clean();
        await ctl({ b16: b16() });
        let page = await open('/setup');
        await waitFor(page, () => !!document.querySelector('[data-setup-extend]'), 15000).catch(() => {});
        await snap(page, 'ready');
        await into(page, '[data-setup-extend]');
        await snap(page, 'ready-extend');

        // A begin refused because a setup run holds the server.
        await ctl({ b16: b16({ extendRefuse: 'server_setup_busy' }) });
        await click(page, '[data-setup-extend] button');
        await waitFor(page, () => !!document.querySelector('[data-setup-extend] [role="alert"]'), 10000).catch(() => {});
        await pause(300);
        await into(page, '[data-setup-extend]');
        await snap(page, 'ready-refused-busy');
        await closePage(page);

        // The extension wizard.
        await ctl({ b16: b16({ extendDelay: 1500 }) });
        page = await open('/setup');
        await waitFor(page, () => !!document.querySelector('[data-setup-extend] button'), 15000).catch(() => {});
        await click(page, '[data-setup-extend] button');
        await pause(400);
        await snap(page, 'ready-beginning', { note: 'begin held 1.5 s' });
        await waitFor(page, () => !!document.querySelector('input[name="setup-purpose"]'), 15000).catch(() => {});
        await quiet(page); await pause(400);
        await snap(page, 'ext-purpose');
        await whole(page, 'ext-purpose-whole');
        await click(page, 'input[name="setup-purpose"][value="web_mail"]');
        await pause(300);
        await clickByText(page, ['Customize selection', 'Seçimi özelleştir']);
        await waitFor(page, () => !!document.querySelector('#setup-component-postfix'), 15000).catch(() => {});
        await pause(400);
        await snap(page, 'ext-components');
        await into(page, '#setup-component-postfix');
        await snap(page, 'ext-components-mail');
        await whole(page, 'ext-components-whole');
        await submit(page);
        await waitFor(page, () => !!document.querySelector('#setup-mail_hostname'), 15000).catch(() => {});
        await pause(400);
        await type(page, '#setup-mail_hostname', 'mail.example.com');
        await into(page, '#setup-mail_hostname');
        await snap(page, 'ext-access');
        await submit(page);
        await waitFor(page, () => !!document.querySelector('[data-setup-extension-review]'), 15000).catch(() => {});
        await pause(500);
        await into(page, '[data-setup-extension-review]');
        await snap(page, 'ext-review');
        await page.evaluate(() => { const details = document.querySelector('[data-setup-extension-review] details'); if (details) details.open = true; });
        await pause(300);
        await into(page, '[data-setup-extension-review] details');
        await snap(page, 'ext-review-kept');
        await whole(page, 'ext-review-whole');
        await into(page, '[data-setup-extend-stop]');
        await snap(page, 'ext-stop');
        await click(page, '[data-setup-extend-stop] button');
        await waitFor(page, () => !!document.querySelector('[data-setup-extend]'), 15000).catch(() => {});
        await pause(500);
        await snap(page, 'ext-stopped');
        await closePage(page);
        await clean();
    };

    scenarios.x16notice = async () => {
        await reset(); await clean();
        // Completed, not extending: Settings offers the extension.
        await ctl({ b16: b16() });
        let page = await open('/settings?section=setup');
        await waitFor(page, () => !!document.querySelector('#setup-settings-title'), 15000).catch(() => {});
        await into(page, '#setup-settings-title');
        await snap(page, 'settings-ready');
        await closePage(page);

        // An extension in draft.
        await ctl({ b16: b16({ status: 'draft', extension: true, base: structuredClone(DRAFT), draft: structuredClone(EXTENDED) }) });
        page = await open('/');
        await waitFor(page, () => !!document.querySelector('[data-setup-extension]'), 15000).catch(() => {});
        await into(page, '[data-setup-extension]');
        await snap(page, 'dash-draft');
        await closePage(page);
        page = await open('/settings?section=setup');
        await into(page, '#setup-settings-title');
        await snap(page, 'settings-draft');
        await closePage(page);

        // Its run is active.
        await ctl({ b16: b16({ status: 'running', extension: true, base: structuredClone(DRAFT), draft: structuredClone(EXTENDED) }), override: { '/api/v1/setup/operation': { method: 'GET', status: 200, body: runBody(['succeeded', 'succeeded', 'running']) } } });
        page = await open('/');
        await waitFor(page, () => !!document.querySelector('[data-setup-extension]'), 15000).catch(() => {});
        await into(page, '[data-setup-extension]');
        await snap(page, 'dash-running');
        await closePage(page);
        page = await open('/settings?section=setup');
        await into(page, '#setup-settings-title');
        await snap(page, 'settings-running');
        await closePage(page);
        page = await open('/setup', MARKER);
        await waitFor(page, () => !!document.querySelector('#setup-progress-title'), 15000).catch(() => {});
        await pause(600);
        await snap(page, 'ext-running');
        await closePage(page);
        await clean();
    };

    scenarios.x16mail = async () => {
        await reset(); await clean();
        // A prepared server: the dialog routes through setup.
        await ctl({ b16: b16({ draft: { ...structuredClone(DRAFT), mail_hostname: 'mail.example.com' } }), managedScan: scan(), b12: { scan: 'stored' } });
        let page = await open('/services');
        await waitFor(page, () => !!document.querySelector('#mail-stacks'), 15000).catch(() => {});
        await into(page, '#mail-stacks');
        await snap(page, 'mail-cards');
        await profileButton(page);
        await snap(page, 'mail-dialog-setup');
        await click(page, '[data-mail-hostname-change]');
        await type(page, '#mail-profile-hostname', 'mx.example.com');
        await into(page, '#mail-profile-hostname-note');
        await snap(page, 'mail-dialog-change');

        // Refused as busy: held for 2.5 s, then the reason in the open dialog.
        await ctl({ b16: b16({ draft: { ...structuredClone(DRAFT), mail_hostname: 'mail.example.com' }, extendRefuse: 'server_setup_busy', extendDelay: 2500 }) });
        await clickByText(page, ['Review in server setup', 'Sunucu kurulumunda incele']);
        await pause(500);
        await snap(page, 'mail-dialog-sending');
        await waitFor(page, () => !!document.querySelector('[data-mail-profile-route-error]'), 10000).catch(() => {});
        await pause(300);
        await page.evaluate(() => document.querySelector('[data-mail-profile-route-error]')?.scrollIntoView({ block: 'center' }));
        await snap(page, 'mail-dialog-refused');

        // Admitted: setup opens on its access step with the new name.
        await ctl({ b16: b16({ draft: { ...structuredClone(DRAFT), mail_hostname: 'mail.example.com' } }) });
        await clickByText(page, ['Review in server setup', 'Sunucu kurulumunda incele']);
        await waitFor(page, () => location.pathname === '/setup' && !!document.querySelector('#setup-mail_hostname'), 15000).catch(() => {});
        await quiet(page); await pause(500);
        await snap(page, 'mail-to-setup');
        await into(page, '#setup-mail_hostname');
        await snap(page, 'mail-to-setup-field');
        await submit(page);
        await waitFor(page, () => !!document.querySelector('[data-setup-extension-review]'), 15000).catch(() => {});
        await pause(500);
        await into(page, '[data-setup-extension-review]');
        await snap(page, 'mail-setup-review');
        await closePage(page);

        // A server without a finished setup: the direct route.
        await ctl({ b16: b16({ status: 'draft', guidance: 'manual' }), managedScan: scan() });
        page = await open('/services');
        await waitFor(page, () => !!document.querySelector('#mail-stacks'), 15000).catch(() => {});
        await profileButton(page);
        await snap(page, 'mail-dialog-direct');
        await click(page, '[data-mail-hostname-change]');
        await type(page, '#mail-profile-hostname', 'mx.example.com');
        await into(page, '#mail-profile-hostname-note');
        await snap(page, 'mail-dialog-direct-change');
        await closePage(page);
        await clean();
    };

    scenarios.x16secondary = async () => {
        await reset(); await clean();
        await ctl({ b16: b16(), managedScan: scan({ dns_identity_ready: false, mail_dns_guidance: GUIDANCE }), b12: { scan: 'stored' } });
        let page = await open('/services');
        await waitFor(page, () => !!document.querySelector('[data-mail-dns-secondary]'), 15000).catch(() => {});
        await into(page, '[data-mail-dns-secondary]');
        await snap(page, 'secondary-named');
        await page.evaluate(() => document.querySelector('#mail-stacks')?.scrollIntoView({ block: 'end' }));
        await pause(300);
        await snap(page, 'secondary-cards');
        await click(page, '[data-mail-dns-secondary] button');
        await waitFor(page, () => location.pathname === '/setup', 10000).catch(() => {});
        await quiet(page); await pause(500);
        await snap(page, 'secondary-to-setup');
        await closePage(page);

        await ctl({ b16: b16({ status: 'draft', guidance: 'manual' }), managedScan: scan({ dns_identity_ready: false, mail_dns_guidance: { ...GUIDANCE, primary_nameserver: '', primary_ip: '' } }) });
        page = await open('/services');
        await waitFor(page, () => !!document.querySelector('[data-mail-dns-secondary]'), 15000).catch(() => {});
        await into(page, '[data-mail-dns-secondary]');
        await snap(page, 'secondary-unnamed');
        await closePage(page);
        await clean();
    };

    scenarios.x16check = async () => {
        await reset(); await clean();
        await ctl({ setupStatus: 'waiting', execution: { request_id: REQ_ID, statuses: DONE, extra: { status: 'waiting', phase: 'verification', context: CONTEXT, checks: [PTR] } }, b12: { manualChecks: [PTR, NOT_READ, { id: 'panel_https', state: 'ready', code: 'ready' }] } });
        const page = await newPage({ 'celikpanel.setup.start.admin': JSON.stringify({ request_id: REQ_ID, plan_id: 'a'.repeat(32), panel_domain: HOST }) });
        await page.goto(`${base}/setup`, { waitUntil: 'networkidle0' });
        await page.waitForSelector('#setup-progress-title', { timeout: 20000 }).catch(() => {});
        await pause(600);
        // Where the owner is looking: the button, then the answer's box.
        const where = () => page.evaluate(() => {
            // The box under the button (the guidance above carries the same check).
            const box = document.querySelector('ul.list-disc > li[data-check="mail_identity"]')?.closest('.rounded-lg');
            const button = Array.from(document.querySelectorAll('main button')).find(node => /Check requirements again|Gereksinimleri tekrar kontrol et/.test(node.innerText));
            const top = node => (node ? Math.round(node.getBoundingClientRect().top) : null);
            return { scrollY: Math.round(window.scrollY), mainScroll: Math.round(document.querySelector('main')?.scrollTop || 0), buttonTop: top(button), boxTop: top(box), boxBottom: box ? Math.round(box.getBoundingClientRect().bottom) : null, viewport: window.innerHeight, focus: document.activeElement?.tagName || null };
        });
        await page.evaluate(() => Array.from(document.querySelectorAll('main button')).find(node => /Check requirements again|Gereksinimleri tekrar kontrol et/.test(node.innerText))?.scrollIntoView({ block: 'center' }));
        await pause(300);
        const before = await where();
        await clickByText(page, ['Check requirements again', 'Gereksinimleri tekrar kontrol et']);
        await waitFor(page, () => /port 25|25 numaralı/.test(document.querySelector('main')?.innerText || ''), 15000).catch(() => {});
        await pause(1500);
        const after = await where();
        await snap(page, 'check-typed', { where: { before, after } });
        await closePage(page);
        await clean();
    };
}
