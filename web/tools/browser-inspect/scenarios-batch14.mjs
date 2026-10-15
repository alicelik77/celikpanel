// Batch 14 (2026-10-15): the smaller visible corrections of the alpha.86
// candidate ("Also in this candidate", items 1-13 of the 2026-10-15
// restart-notice entry in docs/OPERATION-GUIDANCE.md), photographed for the
// craft floor and D-033: Turkish complete, no raw ids or tokens, no overflow
// at 390 px, dark legible, locked controls off.
//
//   c86components  the Components page with the whole catalogue (27 ids) in
//                  the interface language, a search by a translated
//                  description, the install and uninstall dialogs, an action
//                  a component page whose record read is refused with
//                  SERVICE_STATE_UNVERIFIED, an action answered
//                  with SERVICE_STATE_REFRESH_FAILED (the toast and the
//                  "needs verification" banner with the controls it locks),
//                  and a stored record the server cannot verify on load
//   c86mail        the mail profile cards: component_not_installable with a
//                  platform, component_requires with a role token and with a
//                  mixed list (a component, a role, an unknown role)
//   c86dashboard   Recent activity with setup, mail, certificate, sign-in,
//                  repeated and unknown actions and the `system` actor; the
//                  stopped-service line with a translated component name
//   c86rdns        the setup final step's reverse DNS sentence: a resolver
//                  named, resolvers disagreeing (with and without a PTR), no
//                  PTR, and an older Agent (no resolver field)
//   c86store       the Store with the three component_state_* sentences
//   c86lang        document.documentElement.lang before and after TR<->EN
//
// Every answer comes from the mock on 127.0.0.1 (mock.mjs and its overrides);
// nothing else is contacted, nothing is started. Screenshots also go to
// BROWSER_INSPECT_SHOTS (default: the run's out dir) as
// <state>-<lang>-<theme>-<width>.png.
// On dorduncu grup: alpha.86 adayindaki kucuk gorunur duzeltmeler.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';

const HOST = 'panel.example.com';
const REQ_ID = 'd'.repeat(32);
const PLAN_ID = 'a'.repeat(32);
const MARKER = { 'celikpanel.setup.start.admin': JSON.stringify({ request_id: REQ_ID, plan_id: PLAN_ID, panel_domain: HOST }) };
const CONTEXT = { dns_mode: 'external', dns_role: '', dns_engine: 'bind', local_nameserver: '', local_ip: '', peer_nameserver: '', peer_ip: '', panel_domain: HOST, mail_hostname: 'mail.example.com', dns_hosting_management: '' };
const DONE = ['succeeded', 'succeeded', 'succeeded', 'succeeded', 'succeeded', 'succeeded', 'pending'];

// The catalogue as internal/core ManagedServices names it (2026-10-15): id,
// the server's English name and description, category, kind.
const CATALOGUE = [
    ['php-fpm', 'PHP-FPM', 'PHP FastCGI Process Manager', 'web', 'runtime'],
    ['node', 'Node.js', 'JavaScript Runtime', 'web', 'runtime'],
    ['nginx', 'Nginx', 'Reverse Proxy Server', 'web', 'service'],
    ['apache', 'Apache', 'Web Server', 'web', 'service'],
    ['postgresql', 'PostgreSQL', 'Database Server', 'database', 'service'],
    ['mariadb', 'MariaDB', 'Database Server', 'database', 'service'],
    ['phpmyadmin', 'phpMyAdmin', 'MariaDB/MySQL web admin tool', 'database', 'tool'],
    ['phppgadmin', 'phpPgAdmin', 'PostgreSQL web admin tool', 'database', 'tool'],
    ['postfix', 'Postfix', 'SMTP Server', 'email', 'service'],
    ['exim', 'Exim', 'SMTP Server', 'email', 'service'],
    ['rspamd', 'Rspamd', 'Spam Filter & DKIM', 'email', 'service'],
    ['dovecot', 'Dovecot', 'IMAP/POP3 Server', 'email', 'service'],
    ['roundcube', 'Roundcube', 'Webmail', 'email', 'tool'],
    ['spamassassin', 'SpamAssassin', 'Spam Filter', 'email', 'service'],
    ['wireguard', 'WireGuard VPN', 'Built-in VPN server', 'security', 'service'],
    ['certbot', 'Let\'s Encrypt client (certbot)', 'SSL certificate provider', 'security', 'tool'],
    ['fail2ban', 'Fail2ban', 'Intrusion Prevention', 'security', 'service'],
    ['nftables', 'Firewall engine', 'nftables — the panel firewall\'s packet-filtering engine', 'security', 'tool'],
    ['clamav', 'ClamAV', 'Antivirus / malware scanner', 'security', 'service'],
    ['bind', 'BIND', 'DNS Server', 'dns', 'service'],
    ['pdns', 'PowerDNS', 'DNS Server', 'dns', 'service'],
    ['vsftpd', 'vsftpd', 'FTP Server', 'ftp', 'service'],
    ['redis', 'Redis', 'Cache Server', 'cache', 'service'],
    ['valkey', 'Valkey', 'Cache Server', 'cache', 'service'],
    ['memcached', 'Memcached', 'Cache Server', 'cache', 'service'],
    ['netdata', 'Netdata', 'Real-time metrics agent', 'monitoring', 'service'],
    ['cron', 'Scheduled tasks (cron)', 'Native cron daemon that runs scheduled tasks', 'system', 'service'],
];
// Installed here: a small web and mail server; fail2ban and cron stopped.
const INSTALLED = { nginx: 'active (running)', mariadb: 'active (running)', postfix: 'active (running)', dovecot: 'active (running)', bind: 'active (running)', certbot: 'installed', nftables: 'installed', fail2ban: 'inactive (dead)', cron: 'failed' };
const SERVICES = CATALOGUE.map(([id, name, description, category, kind]) => ({
    id, name, description, icon: '', category, kind, versions: [], config_files: [], packages: [id], ports: [],
    ...(id in INSTALLED ? { is_installed: true, status: INSTALLED[id], unit: kind === 'service' ? id : undefined } : { is_installed: false, status: '' }),
}));
const profile = (id, name, services, extra = {}) => ({ id, name, description: `${name} for this server`, status: 'available', available: true, verified: false, latest_attempt_status: 'none', services, ...extra });
const PROFILES = [
    profile('core-mail', 'Core mail', ['postfix', 'dovecot'], {
        status: 'blocked', available: false,
        blocked_reason: 'Postfix cannot be installed automatically on this server.', blocked_code: 'component_not_installable', blocked_vars: { component: 'Postfix', platform: 'Debian/Ubuntu (apt)' },
    }),
    profile('webmail', 'Webmail', ['roundcube'], {
        status: 'blocked', available: false,
        blocked_reason: 'Roundcube requires web-server, which is not part of this profile.', blocked_code: 'component_requires', blocked_vars: { component: 'Roundcube', required: 'web-server' },
    }),
    profile('protected-mail', 'Protected mail', ['rspamd'], {
        status: 'blocked', available: false,
        blocked_reason: 'Rspamd requires Redis, kv-store, mail-relay-host, which is not part of this profile.', blocked_code: 'component_requires', blocked_vars: { component: 'Rspamd', required: 'Redis, kv-store, mail-relay-host' },
    }),
];
const scan = (profiles = PROFILES) => ({
    scanned_at: new Date().toISOString(), scan_age_seconds: 0, dns_identity_ready: true,
    mail_hostname: { current: 'server1.example.com', hostname: '', source: '', current_usable: true, will_set_hostname: false },
    services: SERVICES, profiles,
});

const minutesAgo = n => new Date(Date.now() - n * 60_000).toISOString();
const AUDIT = [
    { id: 12, username: 'admin', action: 'auth.login', ip_address: '198.51.100.23', created_at: minutesAgo(1) },
    { id: 11, username: 'admin', action: 'auth.login', ip_address: '198.51.100.23', created_at: minutesAgo(3) },
    { id: 10, username: 'admin', action: 'backup.offsite.verify_partial:example.com', created_at: minutesAgo(6) },
    { id: 9, username: 'system', action: 'panel.certificate:panel.example.com', created_at: minutesAgo(9) },
    { id: 8, username: 'admin', action: 'server_setup_mail_enrollment_requested', created_at: minutesAgo(12) },
    { id: 7, username: 'system', action: 'server.setup.child:certbot', created_at: minutesAgo(14) },
    { id: 6, username: 'system', action: 'server.setup.child:nftables — reconciled', created_at: minutesAgo(15) },
    { id: 5, username: 'admin', action: 'mail.profile.install:core-mail', created_at: minutesAgo(20) },
    { id: 4, username: 'admin', action: 'service.stop:cron', created_at: minutesAgo(25) },
];

// The reverse DNS check as the Panel passes it (vars.resolver, vars.resolvers).
const ptr = vars => ({ id: 'mail_identity', state: 'action_required', code: 'mail_identity_required', reason: 'reverse_dns_mismatch', vars: { hostname: 'mail.example.com', ip: '203.0.113.10', ...vars } });
const RDNS = [
    ['rdns-resolver', ptr({ ptr: 'static.10.113.0.203.provider.example', resolver: '1.1.1.1' })],
    ['rdns-disagree', ptr({ ptr: 'static.10.113.0.203.provider.example', resolver: '1.1.1.1', resolvers: 'disagree' })],
    ['rdns-none', ptr({ resolver: '8.8.8.8' })],
    ['rdns-none-disagree', ptr({ resolver: '8.8.8.8', resolvers: 'disagree' })],
    ['rdns-older-agent', ptr({ ptr: 'static.10.113.0.203.provider.example' })],
];

// The Store's three sentences, as cmd/panel/store_backend.go says them.
const STORE_TEXT = {
    component_state_unavailable: {
        en: 'CelikPanel has no usable record of this server\'s components yet. It checks them by itself while an administrator has the Dashboard, the Components page or a component\'s page open; open the Store again after that check.',
        tr: 'CelikPanel\'de bu sunucunun bileşenlerine ait kullanılabilir bir kayıt henüz yok. Bir yönetici Panel, Bileşenler sayfası ya da bir bileşenin sayfası açıkken bileşenleri kendiliğinden denetler; o denetimden sonra Mağaza\'yı yeniden açın.',
    },
    component_state_stale: {
        en: 'The record of this server\'s components is older than 15 minutes. CelikPanel checks them again by itself while an administrator has the Dashboard, the Components page or a component\'s page open; open the Store again after that check.',
        tr: 'Bu sunucunun bileşenlerine ait kayıt 15 dakikadan eski. CelikPanel, bir yönetici Panel, Bileşenler sayfası ya da bir bileşenin sayfası açıkken bileşenleri kendiliğinden yeniden denetler; o denetimden sonra Mağaza\'yı yeniden açın.',
    },
    component_state_invalid: {
        en: 'The stored record of this server\'s components could not be read. CelikPanel checks them again by itself while an administrator has the Dashboard, the Components page or a component\'s page open; open the Store again after that check.',
        tr: 'Bu sunucunun bileşenleri için saklanan kayıt okunamadı. CelikPanel, bir yönetici Panel, Bileşenler sayfası ya da bir bileşenin sayfası açıkken bileşenleri kendiliğinden yeniden denetler; o denetimden sonra Mağaza\'yı yeniden açın.',
    },
};
const storeItems = locale => Object.entries(STORE_TEXT).map(([code, text], index) => ({
    id: ['object-cache', 'webmail', 'malware-scan'][index],
    category: 'applications', vendor: 'CelikPanel',
    name: { en: ['Object cache', 'Webmail', 'Malware scanning'], tr: ['Nesne önbelleği', 'Webmail', 'Zararlı yazılım taraması'] }[locale][index],
    description: { en: ['A shared cache for sites.', 'Read mail in the browser.', 'Scan uploads for malware.'], tr: ['Siteler için ortak önbellek.', 'Postayı tarayıcıda okuyun.', 'Yüklemeleri zararlı yazılıma karşı tarayın.'] }[locale][index],
    state: 'unsupported', entitlement_state: 'included', blocker_reason: code, state_reason: text[locale],
    primary_action: { type: 'open_components', path: '/services', enabled: true }, metadata: { icon: '', tags: [] },
}));

// What the floor asks of a state, measured on the page.
const ENGLISH = ['PHP FastCGI Process Manager', 'JavaScript Runtime', 'Reverse Proxy Server', 'Database Server', 'web admin tool', 'SMTP Server', 'Spam Filter', 'IMAP/POP3 Server', 'Built-in VPN server', 'SSL certificate provider', 'Intrusion Prevention', 'Firewall engine', 'Antivirus', 'DNS Server', 'FTP Server', 'Cache Server', 'Real-time metrics agent', 'Scheduled tasks', 'Native cron daemon', 'Let\'s Encrypt client', 'signed in', 'service is stopped', 'which is not part of this profile', 'cannot be installed', 'public resolver', 'Public resolvers', 'Recent activity', 'needs verification', 'could not be verified', 'Rescan', 'checks the components', 'open the Store again', 'Needs attention'];
const TOKENS = /server\.setup|server_setup|auth\.login|panel\.certificate|mail\.profile|service\.stop|SERVICE_STATE|component_requires|component_not_installable|component_state_|web-server|kv-store|mail-relay-host|verify_partial|core-mail|\{[a-z]+\}/;
const facts = (page, locale) => page.evaluate((english, tokens, locale) => {
    const visible = node => node.getClientRects().length > 0 && getComputedStyle(node).visibility !== 'hidden';
    const text = [document.querySelector('main'), ...document.querySelectorAll('[role="dialog"], [aria-live]')].filter(Boolean).map(node => node.innerText || '').join('\n');
    const buttons = Array.from(document.querySelectorAll('main button, [role="dialog"] button')).filter(visible)
        .map(node => `${(node.innerText || node.getAttribute('aria-label') || node.title || '').trim().replace(/\s+/g, ' ').slice(0, 40)}${node.disabled || node.closest('fieldset:disabled') ? ' (disabled)' : ''}`).filter(Boolean);
    const re = new RegExp(tokens);
    return {
        lang: document.documentElement.lang,
        englishLeaks: locale === 'tr' ? english.filter(item => text.includes(item)) : [],
        rawTokens: text.split('\n').filter(line => re.test(line)).map(line => line.slice(0, 160)),
        toasts: Array.from(document.querySelectorAll('[aria-live]')).filter(visible).map(node => node.innerText.trim()).filter(Boolean),
        buttons: buttons.slice(0, 60),
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
        mainOverflowX: (m => (m ? m.scrollWidth > m.clientWidth + 1 : false))(document.querySelector('main')),
    };
}, ENGLISH, TOKENS.source, locale);

export default function batch14(scenarios, { base, vp, locale, theme, out, viewport, ctl, reset, newPage, closePage, shot, into, clickByText, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const target = name => join(shots, `${name}-${locale}-${theme}-${viewport.width}.png`);
    const snap = async (page, state, note = {}) => {
        await shot(page, `c86-${state}`, { ...(await facts(page, locale)), ...note });
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `c86-${state}.png`), target(state));
    };
    // The whole of <main> (it is the scroll container, not the document): the
    // viewport is made as tall as main's content for one screenshot.
    const whole = async (page, state, note = {}) => {
        const height = await page.evaluate(() => { const m = document.querySelector('main'); return m ? m.scrollHeight + (window.innerHeight - m.clientHeight) : document.body.scrollHeight; });
        await page.setViewport({ ...viewport, height: Math.min(Math.max(height, viewport.height), 12000) });
        await pause(500);
        await snap(page, state, { ...note, wholeHeight: height });
        await page.setViewport(viewport);
        await pause(300);
    };
    const centre = (page, pattern) => page.evaluate(source => {
        const re = new RegExp(source, 'i');
        const node = Array.from(document.querySelectorAll('main p, main span, main li, main h2, main h3, main div, [role="dialog"] p'))
            .filter(item => item.getClientRects().length > 0 && re.test(item.innerText || '')).sort((a, b) => a.innerText.length - b.innerText.length)[0];
        node?.scrollIntoView({ block: 'center' });
        return !!node;
    }, pattern.source);
    // The button in a component's row: from the row's visible name up to the
    // nearest ancestor that holds a matching button.
    const rowButton = async (page, name, pattern) => {
        const handle = await page.evaluateHandle((label, source) => {
            const re = new RegExp(source, 'i');
            const names = Array.from(document.querySelectorAll('main *')).filter(node => node.children.length === 0 && (node.textContent || '').trim() === label && node.getClientRects().length > 0);
            for (const start of names) {
                for (let node = start, depth = 0; node && depth < 9; node = node.parentElement, depth++) {
                    const button = Array.from(node.querySelectorAll('button')).find(item => item.getClientRects().length > 0 && re.test(`${item.innerText || ''} ${item.title || ''} ${item.getAttribute('aria-label') || ''}`));
                    if (button) return button;
                }
            }
            return null;
        }, name, pattern.source);
        const element = handle.asElement();
        if (!element) throw new Error(`no button ${pattern} in the row of ${name}`);
        await element.evaluate(node => node.scrollIntoView({ block: 'center' }));
        await pause(200);
        await element.click();
    };
    const confirmDialog = async page => {
        await page.waitForSelector('[role="dialog"]', { timeout: 10000 });
        await pause(300);
        await page.evaluate(() => { const buttons = Array.from(document.querySelector('[role="dialog"]').querySelectorAll('button')).filter(node => !node.disabled && node.getClientRects().length > 0); buttons[buttons.length - 1]?.click(); });
    };
    const openComponents = async (storage = {}) => {
        const page = await newPage(storage);
        await page.goto(`${base}/services`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await waitFor(page, () => (document.querySelector('main')?.innerText || '').includes('ClamAV'), 20000).catch(() => {});
        await quiet(page); await pause(600);
        return page;
    };
    const clean = () => ctl({ clearAll: true, b12: null, managedScan: null, subscriptions: [] });

    scenarios.c86components = async () => {
        await reset(); await clean();
        await ctl({ managedScan: scan(), b12: { scan: 'stored' } });
        let page = await openComponents();
        await snap(page, 'comp-catalogue-top');
        await whole(page, 'comp-catalogue-whole');

        // The search finds a component by its translated description.
        const search = await page.$('main input[type="search"], main input[placeholder]');
        if (search) {
            await search.type(locale === 'tr' ? 'saldırı' : 'intrusion');
            await pause(500);
            await snap(page, 'comp-search-description');
            await search.click({ clickCount: 3 }); await page.keyboard.press('Backspace'); await pause(300);
        }

        // The install dialog of a component that is not installed.
        await rowButton(page, 'ClamAV', /^\s*(Install|Kur)\b/);
        await waitFor(page, () => !!document.querySelector('[role="dialog"]'), 10000).catch(() => {});
        await quiet(page); await pause(500);
        await snap(page, 'comp-install-dialog');
        await page.keyboard.press('Escape'); await pause(400);

        // The uninstall dialog of an installed one.
        await rowButton(page, 'Fail2ban', /Uninstall|Kaldır/);
        await waitFor(page, () => !!document.querySelector('[role="dialog"]'), 10000).catch(() => {});
        await pause(500);
        await snap(page, 'comp-uninstall-dialog');
        await page.keyboard.press('Escape'); await pause(400);
        await closePage(page);

        // A component's own page while the stored record cannot be verified:
        // the Panel answers its read with 503 SERVICE_STATE_UNVERIFIED (the
        // one place it sends that code, cmd/panel/managed_service_handlers.go).
        await ctl({ override: { '/api/v1/managed-services': { method: 'GET', status: 503, body: { error: 'the stored component record could not be verified', code: 'SERVICE_STATE_UNVERIFIED' } } } });
        page = await newPage();
        await page.goto(`${base}/services/clamav`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page); await pause(800);
        await snap(page, 'comp-page-unverified');
        await closePage(page);

        // An action whose outcome and the state after it could not be verified.
        await ctl({ clearAll: true });
        await ctl({ override: { '/api/v1/service/action': { method: 'POST', status: 500, body: { error: 'service state refresh failed', code: 'SERVICE_STATE_REFRESH_FAILED' } } } });
        page = await openComponents();
        await rowButton(page, 'Nginx', /Restart|Yeniden başlat/);
        await confirmDialog(page);
        await waitFor(page, () => !!document.querySelector('main [role="alert"]'), 8000).catch(() => {});
        await pause(300);
        await page.evaluate(() => document.querySelector('main')?.scrollTo(0, 0));
        await pause(200);
        await snap(page, 'comp-refresh-failed');
        // The banner after the toast is gone, and the controls it locks.
        await pause(6500);
        await rowButton(page, 'Nginx', /Restart|Yeniden başlat/).catch(() => {});
        await pause(500);
        await snap(page, 'comp-refresh-failed-locked');
        await closePage(page);

        // A stored record the server cannot verify on load: the page checks by
        // itself (no toast); the check is held for 3 s to be seen.
        await ctl({ clearAll: true, b12: { scan: 'stored', scanDelay: 3000 } });
        await ctl({ override: { '/api/v1/managed-services': { method: 'GET', status: 503, body: { error: 'the stored component record could not be verified', code: 'SERVICE_STATE_UNVERIFIED' } } } });
        page = await newPage();
        await page.goto(`${base}/services`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await pause(1200);
        await snap(page, 'comp-stored-unverified-checking');
        await waitFor(page, () => (document.querySelector('main')?.innerText || '').includes('ClamAV'), 15000).catch(() => {});
        await quiet(page); await pause(400);
        await snap(page, 'comp-stored-unverified-checked');
        await closePage(page);
        await clean();
    };

    scenarios.c86mail = async () => {
        await reset(); await clean();
        await ctl({ managedScan: scan(), b12: { scan: 'stored' } });
        const page = await openComponents();
        await into(page, '#mail-stacks');
        await snap(page, 'mail-profiles');
        await page.evaluate(() => document.querySelector('#mail-stacks')?.scrollIntoView({ block: 'end' }));
        await pause(300);
        await snap(page, 'mail-profiles-end');
        await closePage(page);
        await clean();
    };

    scenarios.c86dashboard = async () => {
        await reset(); await clean();
        await ctl({ domains: [], managedScan: scan(), b12: { scan: 'stored' }, override: { '/api/v1/audit-logs': { method: 'GET', status: 200, body: { entries: AUDIT } } } });
        const page = await newPage();
        await page.goto(`${base}/`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page); await pause(1500);
        await centre(page, /^(Recent activity|Son etkinlik)$/);
        await page.evaluate(() => Array.from(document.querySelectorAll('main h2, main h3')).find(node => /^(Recent activity|Son etkinlik)$/.test(node.innerText.trim()))?.scrollIntoView({ block: 'start' }));
        await pause(300);
        await snap(page, 'dash-activity');
        await centre(page, /(service is stopped|servisi durdu)/);
        await snap(page, 'dash-stopped');
        await closePage(page);
        await clean();
    };

    scenarios.c86rdns = async () => {
        for (const [state, check] of RDNS) {
            await reset(); await clean();
            await ctl({ setupStatus: 'waiting', execution: { request_id: REQ_ID, statuses: DONE, extra: { status: 'waiting', phase: 'verification', context: CONTEXT, checks: [check] } } });
            const page = await newPage(MARKER);
            await page.goto(`${base}/setup`, { waitUntil: 'networkidle0' });
            await page.waitForSelector('#setup-progress-title', { timeout: 20000 }).catch(() => {});
            await pause(700);
            await centre(page, /(PTR)/);
            await snap(page, state);
            await closePage(page);
        }
        await clean();
    };

    scenarios.c86store = async () => {
        await reset(); await clean();
        await ctl({ subscriptions: [{ id: 1, name: 'example.com', owner: 'customer1' }], override: { '/api/v1/store': { method: 'GET', status: 200, body: { items: storeItems(locale) } } } });
        const page = await newPage();
        await page.goto(`${base}/addons`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page); await pause(800);
        await centre(page, /(Object cache|Nesne önbelleği)/);
        await snap(page, 'store-states');
        await whole(page, 'store-whole');
        await closePage(page);
        await clean();
    };

    scenarios.c86lang = async () => {
        await reset(); await clean();
        await ctl({ managedScan: scan(), b12: { scan: 'stored' } });
        const page = await openComponents();
        const other = locale === 'tr' ? 'en' : 'tr';
        const record = { start: await page.evaluate(() => document.documentElement.lang) };
        const switchTo = async to => {
            const direct = await page.evaluate(code => {
                const button = Array.from(document.querySelectorAll('[role="group"] button')).find(node => node.getClientRects().length > 0 && node.innerText.trim().toLowerCase() === code);
                if (button) { button.click(); return true; }
                return false;
            }, to);
            if (!direct) {
                // On a phone the switch is in the user menu.
                await page.evaluate(() => Array.from(document.querySelectorAll('header button')).find(node => /AD/.test(node.innerText || ''))?.click());
                await pause(400);
                await page.evaluate(code => Array.from(document.querySelectorAll('[role="group"] button')).find(node => node.getClientRects().length > 0 && node.innerText.trim().toLowerCase() === code)?.click(), to);
            }
            await waitFor(page, code => document.documentElement.lang === code, 10000, to).catch(() => {});
            await pause(500);
            return page.evaluate(() => ({ lang: document.documentElement.lang, heading: (document.querySelector('main h1, [data-shell-page-header-target] h1')?.innerText || '').trim() }));
        };
        record.afterSwitch = await switchTo(other);
        await snap(page, 'lang-switched', { langRecord: record });
        record.afterBack = await switchTo(locale);
        await snap(page, 'lang-back', { langRecord: record });
        console.log(`c86lang ${vp}-${theme}-${locale} ${JSON.stringify(record)}`);
        await closePage(page);
        await clean();
    };
}
