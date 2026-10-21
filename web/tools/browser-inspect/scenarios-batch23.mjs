// Batch 23 (2026-10-22): the menu layout of the alpha.98 candidate (D-037).
// The owner asked that every page be where its name says it is: the server's
// tools, its security and its maintenance under Server; Settings with four
// sections only; the person's own account in the personal menu. This batch
// photographs the rail for each role, each moved page, Settings, the personal
// menu, the old addresses that must still arrive, and the one calm firewall
// line of Components that now leads to /security#firewall.
//
//   y23sidebar      the rail for an administrator, a reseller and a customer (on
//                   a phone the drawer, opened with the Menu button)
//   y23security     /security: the healthy firewall (Boston-shaped components,
//                   Fail2ban in the scan's real dialect "active (running)"),
//                   Fail2ban stopped ("inactive (dead)"), the firewall OFF with
//                   Fail2ban not installed, /security#firewall at the window's
//                   own size
//   y23maintenance  /maintenance: the four system SQLite files
//   y23dns          /dns-infrastructure: today's DNS infrastructure content
//   y23account      /account for an administrator, a reseller, a customer
//                   (two-factor off; on for the customer)
//   y23settings     /settings (administrator) and its four sections
//   y23menu         the personal menu open: administrator and customer
//   y23redirects    the old addresses: /settings?section=account|security|dns|
//                   system-databases and /settings as a customer, with the
//                   final location and whether history grew
//   y23components   Components with the healthy firewall line (its Settings
//                   link must point to /security#firewall), the click, and the
//                   firewall OFF (its box stays on Components)
//
// Every answer comes from the mock on 127.0.0.1 (mock.mjs, mock-batch3.mjs,
// mock-batch20.mjs, mock-batch21.mjs); /api/v1/security/audit,
// /api/v1/system-databases and the three roles are constants set through the
// mock's overrides (as are the two empty lists of authorized DNS connections the
// DNS page reads), so no new mock file is needed. Nothing else is contacted.
// Screenshots also go to BROWSER_INSPECT_SHOTS (default: the run's out dir) as
// <state>-<lang>-<theme>-<width>.png.
// Yirmi ucuncu grup: alpha.98 menu duzeni.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { b3Defaults } from './mock-batch3.mjs';
import { batch20State } from './mock-batch20.mjs';
import { batch21State } from './mock-batch21.mjs';

const ME = '/api/v1/auth/me';
const AUDIT_URL = '/api/v1/security/audit';
const SYSDB_URL = '/api/v1/system-databases';
const BASE_USER = { account_type: 'account', email: 'owner@example.com', impersonating: false, features: { team_members: false } };
const USERS = {
    admin: { ...BASE_USER, username: 'admin', role: 'admin', effective_role: 'admin' },
    reseller: { ...BASE_USER, username: 'reseller1', role: 'reseller', effective_role: 'reseller' },
    customer: { ...BASE_USER, username: 'customer1', role: 'customer', effective_role: 'customer' },
};

// The audit answers exactly as the product's closed contract reads it
// (SecurityAuditCard.isSecurityAuditResponse). The agent never promotes the
// firewall persistence to PASS, so the realistic healthy answer has unknown /
// warning rows too; sample data, not measured.
const STAMP = '2026-10-22T08:00:00Z';
const AGENT_BASE = {
    contract_version: 1, capability: 'security_audit_v1', build_version: 'v0.1.0-alpha.98', build_commit: 'a'.repeat(40), generated_at: STAMP,
    ssh: { check: { status: 'unknown', code: 'ssh_policy_live_unverified' }, password_authentication: 'no', keyboard_interactive_authentication: 'no', permit_root_login: 'prohibit-password', pubkey_authentication: 'yes', hostbased_authentication: 'no', gssapi_authentication: 'no' },
    reboot: { check: { status: 'pass', code: 'reboot_not_required' }, required: false },
    signed_update: { check: { status: 'warning', code: 'signed_update_identity_unverified' }, enrolled: true, sequence: '98', version: 'v0.1.0-alpha.98', key_fingerprint: `sha256:${'b'.repeat(64)}` },
};
const TLS = {
    certificate: { status: 'pass', code: 'panel_tls_certificate_valid' }, self_signed: { status: 'warning', code: 'panel_tls_self_signed' },
    expiry: { status: 'pass', code: 'panel_tls_valid' }, key_match: { status: 'pass', code: 'panel_tls_key_match' }, is_self_signed: true, expires_at: '2027-10-01T00:00:00Z',
};
const AUDIT_HEALTHY = {
    contract_version: 1, generated_at: STAMP,
    agent: {
        ...AGENT_BASE,
        firewall: { engine: { status: 'pass', code: 'firewall_engine_available' }, default_drop: { status: 'pass', code: 'firewall_policy_drop' }, persistence: { status: 'unknown', code: 'firewall_persistence_unverified' }, tcp_allowlist: [22, 80, 443, 2083], udp_allowlist: [] },
        listeners: { check: { status: 'pass', code: 'listeners_match_allowlist' }, findings: [] },
    },
    tls: TLS,
};
const AUDIT_OFF = {
    ...AUDIT_HEALTHY,
    agent: {
        ...AGENT_BASE,
        firewall: { engine: { status: 'pass', code: 'firewall_engine_available' }, default_drop: { status: 'fail', code: 'firewall_disabled' }, persistence: { status: 'fail', code: 'firewall_persistence_missing' }, tcp_allowlist: [], udp_allowlist: [] },
        listeners: { check: { status: 'unknown', code: 'listener_state_ambiguous' }, findings: [] },
    },
};

const sqlite = (id, name, purpose, kind, extra = {}) => ({
    id, name, purpose, kind, mutable: id !== 'component-catalog', available: true, path_hint: `/var/lib/celikpanel/${id}.db`, size_bytes: 3_145_728, modified_at: '2026-10-22T07:12:00Z',
    journal_mode: 'wal', user_version: 0, schema_version: 41, status: 'ready', status_message: '', actions: ['check', 'snapshot', 'optimize'], ...extra,
});
const SYSTEM_DATABASES = {
    databases: [
        sqlite('panel', 'Panel database', 'Accounts, domains, settings.', 'control-plane'),
        sqlite('powerdns', 'PowerDNS database', 'DNS records.', 'dns', { available: false, status: 'missing', size_bytes: 0, modified_at: '', journal_mode: '', schema_version: undefined, path_hint: '/var/lib/powerdns/pdns.sqlite3', actions: ['check'] }),
        sqlite('roundcube', 'Roundcube database', 'Webmail data.', 'webmail', { available: false, status: 'missing', size_bytes: 0, modified_at: '', journal_mode: '', schema_version: undefined, path_hint: '/var/lib/roundcube/roundcube.db', actions: ['check'] }),
        sqlite('component-catalog', 'Component catalogue', 'What this panel knows about components.', 'catalog', { size_bytes: 262_144, schema_version: 7, actions: ['check', 'snapshot'] }),
    ],
};

// English words that must not appear on a Turkish page (whole-page scan, rail and header included).
const ENGLISH_PHRASES = ['Is this server safe', 'Manage Fail2ban', 'Open Components', 'Sign out', 'My account', 'Inspect and safely', 'Your own sign-in', 'Shared nameservers', 'Panel address and HTTPS', 'DNS and peer servers', 'Maintenance', 'Read-only', 'Two-factor', 'Retry', 'Refresh', 'Settings', 'Security', 'Server setup', 'Not checked', 'Change password', 'Password', 'Firewall', 'Dashboard', 'Hosting'];
const STOPWORDS = /\b(the|and|your|this|with|for|of|is|are|has|have|not|yet|from|you|can|will)\b/i;
const RAW = /\b(nav|securityPage|settings|account|systemDb|dnsPeers|maintenance|services|securityAudit|profile|user|common)\.[a-zA-Z][a-zA-Z0-9.]*\b|\{[a-zA-Z]+\}/;

const facts = (page, locale) => page.evaluate((english, stop, rawSource, locale) => {
    const visible = (node) => node.getClientRects().length > 0 && getComputedStyle(node).visibility !== 'hidden';
    const inViewX = (node) => { const r = node.getBoundingClientRect(); return r.right > 1 && r.left < window.innerWidth - 1 && r.width > 0; };
    const main = document.querySelector('main');
    const body = document.body.innerText || '';
    const text = main ? main.innerText : '';
    const triple = (c) => (c.match(/\d+(\.\d+)?/g) || []).slice(0, 3).join(',');
    const rgbOf = (css) => { const probe = document.createElement('span'); probe.style.color = css; document.body.appendChild(probe); const c = getComputedStyle(probe).color; probe.remove(); return triple(c); };
    const tokens = { amber: rgbOf('rgb(var(--warning))'), amberMark: rgbOf('rgb(var(--warning-mark))'), red: rgbOf('rgb(var(--danger))'), green: rgbOf('rgb(var(--success))'), primary: rgbOf('rgb(var(--primary))') };
    const label = (node) => `${node.tagName.toLowerCase()}: ${(node.innerText || node.getAttribute('aria-label') || '').trim().replace(/\s+/g, ' ').slice(0, 50)}`;
    const found = { amber: [], red: [], green: [] };
    const scope = Array.from(document.querySelectorAll('main *, header *, [data-personal-menu] *, nav *')).filter((n) => visible(n) && inViewX(n));
    for (const node of scope) {
        const style = getComputedStyle(node);
        const props = [style.color, style.backgroundColor, style.borderTopColor, style.borderLeftColor, style.fill, style.stroke];
        const own = (node.children.length === 0 && (node.innerText || '').trim()) || node.tagName === 'svg' || node.tagName === 'path' || style.backgroundColor !== 'rgba(0, 0, 0, 0)' || parseFloat(style.borderTopWidth) > 0;
        if (!own) continue;
        const set = new Set(props.filter((c) => c && c !== 'none').map(triple));
        for (const [name, key] of [['amber', 'amber'], ['amber', 'amberMark'], ['red', 'red'], ['green', 'green']]) if (set.has(tokens[key]) && found[name].length < 14) found[name].push(label(node));
    }
    const small = Array.from(document.querySelectorAll('main button, main a, main summary, main input, main [role="tab"], header button, header a, [data-personal-menu] a, [data-personal-menu] button, nav a, aside a, aside button')).filter((n) => visible(n) && inViewX(n)).map((node) => ({ node, r: node.getBoundingClientRect() }))
        .filter(({ node, r }) => window.innerWidth <= 639 && Math.max(r.height, node.closest('label')?.getBoundingClientRect().height || 0) < 43.5 && getComputedStyle(node, '::after').position !== 'absolute')
        .map(({ node, r }) => `${(node.innerText || node.getAttribute('aria-label') || node.type || '').trim().slice(0, 40)} ${Math.round(r.height)}px`);
    const rect = (node) => { if (!node) return null; const r = node.getBoundingClientRect(); return { x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.width), h: Math.round(r.height) }; };
    const rail = document.querySelector('aside nav, nav.flex-1');
    const railLinks = rail ? Array.from(rail.querySelectorAll('a, button')).filter(visible).map((a) => ({ text: a.innerText.replace(/\s+/g, ' ').trim(), href: a.getAttribute('href') || a.tagName.toLowerCase(), current: a.getAttribute('aria-current'), rect: rect(a) })) : [];
    const railHeadings = rail ? Array.from(rail.querySelectorAll('p')).filter(visible).map((p) => p.innerText.trim()) : [];
    const re = new RegExp(rawSource);
    const lines = body.split('\n').map((l) => l.trim()).filter(Boolean);
    const personal = document.querySelector('[data-personal-menu]');
    const h1 = Array.from(document.querySelectorAll('h1')).filter(visible).map((n) => n.innerText.trim());
    return {
        lang: document.documentElement.lang,
        innerWidth: window.innerWidth,
        location: { path: location.pathname, search: location.search, hash: location.hash, historyLength: history.length },
        h1,
        mainText: text.replace(/\n{2,}/g, '\n').slice(0, 2400),
        rail: { headings: railHeadings, links: railLinks.map((l) => `${l.text} -> ${l.href}${l.current ? ' [current]' : ''}`), onScreen: railLinks.length > 0 && railLinks.every((l) => l.rect.x >= -1) },
        railRects: railLinks.map((l) => l.rect).slice(0, 3),
        personalMenu: personal ? { items: Array.from(personal.querySelectorAll('a, button')).filter(visible).map((n) => `${n.tagName.toLowerCase()}: ${n.innerText.replace(/\s+/g, ' ').trim()}${n.getAttribute('href') ? ' -> ' + n.getAttribute('href') : ''}`), head: personal.firstElementChild?.innerText.replace(/\s+/g, ' ').trim(), rect: rect(personal) } : null,
        rawKeys: lines.filter((line) => re.test(line)).slice(0, 8),
        englishLeaks: locale === 'tr' ? english.filter((item) => body.includes(item)) : [],
        englishLines: locale === 'tr' ? lines.filter((line) => line.length > 12 && new RegExp(stop, 'i').test(line)).slice(0, 8) : [],
        filledButtons: Array.from(document.querySelectorAll('main button')).filter((n) => visible(n) && triple(getComputedStyle(n).backgroundColor) === tokens.primary).map((n) => n.innerText.trim().slice(0, 40)),
        amberUsed: found.amber, redUsed: found.red, greenUsed: found.green,
        smallTargets: small.slice(0, 15),
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
        mainOverflowX: main ? main.scrollWidth > main.clientWidth + 1 : false,
        pastRight: Array.from(document.querySelectorAll('body *')).filter((n) => { const r = n.getBoundingClientRect(); return visible(n) && r.width > 0 && r.right > window.innerWidth + 1 && n.closest('aside') === null; }).slice(0, 8).map((n) => `${n.tagName.toLowerCase()} right=${Math.round(n.getBoundingClientRect().right)}: ${(n.innerText || '').trim().replace(/\s+/g, ' ').slice(0, 30)}`),
        firewall: {
            compact: !!document.querySelector('[data-firewall-compact]'),
            settingsLink: document.querySelector('[data-firewall-settings-link]')?.getAttribute('href') ?? null,
            unread: !!document.querySelector('[data-firewall-unread]'),
            securityFirewallBox: !!document.querySelector('#firewall'),
        },
        security: {
            sections: Array.from(document.querySelectorAll('[data-security-section]')).map((n) => n.getAttribute('data-security-section')),
            fail2ban: document.querySelector('[data-fail2ban-state]')?.getAttribute('data-fail2ban-state') ?? null,
            firewallTop: rect(document.getElementById('firewall')),
        },
        settingsTabs: Array.from(document.querySelectorAll('[role="tab"]')).filter(visible).map((n) => n.innerText.replace(/\s+/g, ' ').trim().slice(0, 60)),
    };
}, ENGLISH_PHRASES, STOPWORDS.source, RAW.source, locale);

export default function batch23(scenarios, { base, locale, theme, out, viewport, vp, ctl, reset, newPage, closePage, shot, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const phone = vp === 'phone';
    const snap = async (page, state, note = {}) => {
        await shot(page, `y23-${state}`, { ...(await facts(page, locale)), ...note });
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `y23-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    // The content as tall as <main>, so the whole page is in the picture.
    const whole = async (page, state, note = {}) => {
        const height = await page.evaluate(() => { const m = document.querySelector('main'); return m ? m.scrollHeight + (window.innerHeight - m.clientHeight) : document.body.scrollHeight; });
        await page.setViewport({ ...viewport, height: Math.min(Math.max(height, viewport.height), 6000) });
        await pause(500);
        await snap(page, state, { ...note, wholeHeight: height });
        await page.setViewport(viewport);
        await pause(300);
    };
    // On a phone the rail is a drawer behind the Menu button; on a wide screen it is there already.
    const openDrawer = async (page) => {
        if (!phone) return;
        await page.click('button[aria-label="Menu"], button[aria-label="Menü"]').catch(() => {});
        await pause(700);
    };
    const closeDrawer = async (page) => {
        if (!phone) return;
        await page.keyboard.press('Escape');
        await pause(400);
    };
    const sidebar = async (page, state, note = {}) => {
        await openDrawer(page);
        await snap(page, state, note);
        await closeDrawer(page);
    };
    const clean = () => ctl({ clearAll: true, b3: null, b12: null, b16: null, b19: null, b20: null, b21: null, b22: null, managedScan: null, componentOperation: null, domains: [], infrastructureZones: [], twoFactor: false, services: [] });
    const as = (role) => ctl({ override: { [ME]: { status: 200, body: USERS[role] } } });
    const open = async (path, ready, ...args) => {
        const page = await newPage();
        await page.goto(`${base}${path}`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page);
        if (ready) await waitFor(page, ready, 15000, ...args).catch(() => {});
        await pause(900);
        return page;
    };
    const boston = (mutate) => {
        const state = batch20State('boston', {});
        if (mutate) state.services = state.services.map(mutate);
        return state;
    };
    const f2b = (status, installed = true) => (item) => (item.id === 'fail2ban' ? { ...item, is_installed: installed, status } : item);
    const firewallOff = () => ({ ...b3Defaults(), firewall: { enabled: false, engine_available: true, tcp_ports: [], udp_ports: [], ssh_ports: [22], persistence_state: 'ready' } });
    const mainHas = (source) => document.querySelector('main') !== null && new RegExp(source).test(document.querySelector('main').innerText || '');

    scenarios.y23sidebar = async () => {
        for (const role of ['admin', 'reseller', 'customer']) {
            await reset(); await clean();
            await as(role);
            if (role === 'admin') await ctl({ b21: batch21State({ outcome: 'current', automatic: true }), b20: boston(), override: { [AUDIT_URL]: { status: 200, body: AUDIT_HEALTHY } } });
            const page = await open(role === 'admin' ? '/security' : '/', null);
            await sidebar(page, `sidebar-${role}`);
            if (!phone) await whole(page, `sidebar-${role}-page`);
            await closePage(page);
            await clean();
        }
    };

    scenarios.y23security = async () => {
        const run = async (name, { audit, b3, b20, hash = '' }) => {
            await reset(); await clean();
            await ctl({ override: { [AUDIT_URL]: { status: 200, body: audit } }, b20, ...(b3 ? { b3 } : {}) });
            const page = await open(`/security${hash}`, mainHas, 'Fail2ban');
            await waitFor(page, () => !!document.querySelector('[data-security-section="audit"] dl') || !!document.querySelector('[role="alert"]'), 10000).catch(() => {});
            await whole(page, name);
            await closePage(page);
            await clean();
        };
        await run('security', { audit: AUDIT_HEALTHY, b20: boston() });
        await run('security-f2b-stopped', { audit: AUDIT_HEALTHY, b20: boston(f2b('inactive (dead)')) });
        await run('security-off', { audit: AUDIT_OFF, b20: boston(f2b('not_installed', false)), b3: firewallOff() });
        // /security#firewall at the window's own size: where the page lands.
        await reset(); await clean();
        await ctl({ override: { [AUDIT_URL]: { status: 200, body: AUDIT_HEALTHY } }, b20: boston() });
        const page = await open('/security#firewall', mainHas, 'Fail2ban');
        await snap(page, 'security-hash');
        await closePage(page);
        await clean();
    };

    scenarios.y23maintenance = async () => {
        await reset(); await clean();
        await ctl({ override: { [SYSDB_URL]: { status: 200, body: SYSTEM_DATABASES } } });
        const page = await open('/maintenance', () => document.querySelectorAll('main h3').length >= 2);
        await whole(page, 'maintenance');
        await closePage(page);
        await clean();
    };

    scenarios.y23dns = async () => {
        await reset(); await clean();
        // The two lists of authorized DNS connections the page reads: none yet (the mock has no such route otherwise, and a 404 reads as a failed check).
        await ctl({ override: { '/api/v1/dns/remote/connections': { status: 200, body: { connections: [] } }, '/api/v1/dns/remote/clients': { status: 200, body: { clients: [] } } } });
        const page = await open('/dns-infrastructure', null);
        await pause(800);
        await whole(page, 'dns-infrastructure');
        await closePage(page);
        await clean();
    };

    scenarios.y23account = async () => {
        for (const [role, twoFactor] of [['admin', false], ['reseller', false], ['customer', true]]) {
            await reset(); await clean();
            await as(role);
            await ctl({ twoFactor });
            const page = await open('/account', null);
            await pause(600);
            await whole(page, `account-${role}`);
            await closePage(page);
            await clean();
        }
    };

    scenarios.y23settings = async () => {
        await reset(); await clean();
        await ctl({ b21: batch21State({ outcome: 'current', automatic: true }) });
        const page = await open('/settings', () => document.querySelectorAll('[role="tab"]').length > 0);
        await whole(page, 'settings');
        for (const [id, name] of [['license', 'settings-license'], ['updates', 'settings-updates'], ['setup', 'settings-setup']]) {
            await page.click(`#settings-${id}-tab`).catch(() => {});
            await pause(900);
            await whole(page, name);
        }
        await closePage(page);
        await clean();
    };

    scenarios.y23menu = async () => {
        for (const role of ['admin', 'customer']) {
            await reset(); await clean();
            await as(role);
            const page = await open('/', null);
            await page.click('button[aria-controls="personal-menu"]').catch(() => {});
            await pause(600);
            await snap(page, `menu-${role}`);
            await closePage(page);
            await clean();
        }
    };

    scenarios.y23redirects = async () => {
        const cases = [
            ['admin', '/settings?section=account', 'redirect-account'],
            ['admin', '/settings?section=security', 'redirect-security'],
            ['admin', '/settings?section=dns', 'redirect-dns'],
            ['admin', '/settings?section=system-databases', 'redirect-system-databases'],
            ['customer', '/settings', 'redirect-customer-settings'],
            ['reseller', '/settings?section=updates', 'redirect-reseller-settings'],
        ];
        // Baseline: a plain page with no redirect, so "history did not grow" has something to be compared to.
        await reset(); await clean();
        await ctl({ override: { [AUDIT_URL]: { status: 200, body: AUDIT_HEALTHY }, [SYSDB_URL]: { status: 200, body: SYSTEM_DATABASES } }, b20: boston() });
        const baseline = await open('/domains', null);
        const baseHistory = await baseline.evaluate(() => history.length);
        await closePage(baseline);
        for (const [role, path, name] of cases) {
            await reset(); await clean();
            await as(role);
            await ctl({ override: { [AUDIT_URL]: { status: 200, body: AUDIT_HEALTHY }, [SYSDB_URL]: { status: 200, body: SYSTEM_DATABASES } }, b20: boston() });
            const page = await open(path, null);
            await pause(700);
            await snap(page, name, { requested: path, role, baselineHistoryLength: baseHistory });
            await closePage(page);
            await clean();
        }
    };

    scenarios.y23components = async () => {
        await reset(); await clean();
        await ctl({ b20: boston() });
        const page = await open('/services', () => document.querySelectorAll('main li').length >= 6);
        await whole(page, 'components-firewall-line');
        const href = await page.evaluate(() => document.querySelector('[data-firewall-settings-link]')?.getAttribute('href') ?? null);
        // Press the link: it must arrive at /security with the firewall box in view.
        if (href) {
            await ctl({ override: { [AUDIT_URL]: { status: 200, body: AUDIT_HEALTHY } } });
            await page.click('[data-firewall-settings-link]').catch(() => {});
            await pause(1500);
            await snap(page, 'components-firewall-line-followed', { hrefBeforeClick: href });
        }
        await closePage(page);
        await clean();
        await reset(); await clean();
        await ctl({ b20: boston(), b3: firewallOff() });
        const off = await open('/services', () => document.querySelectorAll('main li').length >= 6);
        await whole(off, 'components-firewall-off');
        await closePage(off);
        await clean();
    };
}
