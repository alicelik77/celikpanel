// Batch 40 (2026-11-19): alpha.139, each mailbox shows its own connection
// settings.
//
//   y40conn   A domain's mailboxes as the administrator and as a customer:
//             the first mailbox's "More" menu open (Connection settings…
//             first), the window with that mailbox's values, and the window
//             in each state the read can carry: the told name without a
//             trusted certificate, mail not installed here, and no mail name
//
// Every answer comes from the mock on 127.0.0.1 through overrides (SAMPLE
// data shaped as the Panel answers); nothing is changed or contacted.
// Kırkıncı grup: posta kutusunun bağlantı bilgileri; hepsi sahte.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';

const ME = '/api/v1/auth/me';
const BASE_USER = { account_type: 'account', email: 'owner@example.com', impersonating: false, features: { team_members: false } };
const USERS = {
    admin: { ...BASE_USER, username: 'admin', role: 'admin', effective_role: 'admin' },
    customer: { ...BASE_USER, username: 'musteri1', role: 'customer', effective_role: 'customer' },
};
const CAPABILITIES = { dns_server: 'bind', dns_identity_ready: true, dns_management_mode: 'local', dns_management_ready: true, web_server: 'nginx', php_versions: ['8.4'], mail_server: true, database_servers: ['mariadb'], db_tools: [] };
const DOMAIN = { id: 1, domain_name: 'kahvedukkani.com.tr', status: 'active', project_type: 'php', php_version: '8.4', ssl_enabled: true, created_at: '2026-09-01T10:00:00Z', disk_usage: 3277, bandwidth: 0 };

const HOST = 'mail.kahvedukkani.com.tr';
const protocol = (port, security) => ({ host: HOST, port, security });
const READY = {
    mail_host: HOST, imap: protocol(993, 'SSL/TLS'), pop3: protocol(995, 'SSL/TLS'), smtp: { ...protocol(587, 'STARTTLS'), auth_required: true },
    username_is_full_email: true, webmail_available: true, webmail_url: `https://${HOST}/`,
    host_protection: { kind: 'mail_programs', name: HOST, state: 'protected', by: 'server_mail_certificate', until: '2027-02-01T00:00:00Z', ips: [] },
    autoconfig: { state: 'on', reason: '', names: ['autoconfig.kahvedukkani.com.tr', 'autodiscover.kahvedukkani.com.tr'], mail_host: HOST, served: 'yes', secure: 'yes', dns: 'local', records: [], can_fix: false },
};
// The states the read can carry (cmd/panel handleMailClientSetupWith): the
// told name without a trusted certificate; mail not installed here; and the
// server's mail name not set, whose host is the bare mail.<domain>.
const unavailable = (reason) => ({ ...READY, host_protection: undefined, autoconfig: { state: 'unavailable', reason, names: READY.autoconfig.names, served: 'unknown', secure: 'unknown', records: [] } });
const PROBLEMS = {
    certificate: { ...READY, host_protection: { kind: 'mail_programs', name: HOST, state: 'not_protected', reason: 'server_certificate_not_ready', ips: [] } },
    'not-here': unavailable('mail_not_here'),
    'name-unset': unavailable('mail_host_unknown'),
};
const ACCOUNTS = { accounts: [{ id: 5, address: 'siparis@kahvedukkani.com.tr', quota_mb: 2048 }, { id: 6, address: 'ayse.yilmaz@kahvedukkani.com.tr', quota_mb: 1024 }] };
const QUOTA = { plugin_enabled: true, usages: [
    { email: 'siparis@kahvedukkani.com.tr', used_kb: 350208, limit_kb: 2097152, available: true },
    { email: 'ayse.yilmaz@kahvedukkani.com.tr', used_kb: 100, limit_kb: 1048576, available: true },
] };

const facts = (page) => page.evaluate(() => {
    const dialog = document.querySelector('[role="dialog"]');
    const node = dialog || document.querySelector('[role="menu"]') || document.querySelector('main');
    const raw = /\b(mail\.(setup|row)|mconn|mcfg|certcover|kit)\.[a-zA-Z][a-zA-Z0-9_.]*\b|\{[a-z_]+\}/;
    const text = node ? node.innerText : '';
    const box = dialog?.getBoundingClientRect();
    return {
        text: text.split('\n').map((l) => l.trim()).filter(Boolean).slice(0, 80),
        rawKeys: text.split('\n').filter((l) => raw.test(l)),
        dialogWidth: box ? Math.round(box.width) : null,
        dialogHeight: box ? Math.round(box.height) : null,
        actionsVisible: dialog ? (() => { const r = dialog.querySelector('[data-dialog-actions]')?.getBoundingClientRect(); return !!r && r.bottom <= window.innerHeight; })() : null,
        rowButtons: Array.from(document.querySelector('[data-row-actions]')?.querySelectorAll('a, button') ?? []).map((b) => (b.innerText || '').trim()),
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
    };
});

export default function batch40(scenarios, { base, locale, theme, out, viewport, ctl, reset, drainLog, newPage, closePage, shot, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const snap = async (page, state, extra = {}) => {
        await shot(page, `y40-${state}`, { ...(await facts(page)), ...extra, requests: await drainLog() });
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `y40-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    const open = async (role, setup) => {
        await reset();
        await ctl({ clearAll: true, domains: [DOMAIN], capabilities: CAPABILITIES, override: {
            [ME]: { status: 200, body: USERS[role] },
            '/api/v1/domains/1/mail/accounts': { method: 'GET', status: 200, body: ACCOUNTS },
            '/api/v1/domains/1/mail/forwardings': { method: 'GET', status: 200, body: { forwardings: [] } },
            '/api/v1/domains/1/mail/quota': { method: 'GET', status: 200, body: QUOTA },
            '/api/v1/domains/1/mail/setup': { method: 'GET', status: 200, body: setup },
            '/api/v1/domains/1/mail/outgoing': { method: 'GET', status: 200, body: { enforced: false, state: 'off', mailboxes: [] } },
        } });
        const page = await newPage({});
        await page.goto(`${base}/domains/kahvedukkani.com.tr?section=mail`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page);
        await waitFor(page, () => !!document.querySelector('[data-row-more]'), 20000).catch(() => {});
        await pause(600);
        await page.evaluate(() => document.querySelector('[data-row-actions]')?.closest('table')?.scrollIntoView({ block: 'center' }));
        await pause(200);
        await page.evaluate(() => document.querySelector('[data-row-more]')?.click());
        await waitFor(page, () => !!document.querySelector('[role="menu"]'), 5000).catch(() => {});
        await pause(300);
        return page;
    };
    const openWindow = async (page) => {
        await page.evaluate(() => document.querySelector('[role="menuitem"][data-row-action="connection"]')?.click());
        await waitFor(page, () => !!document.querySelector('[data-mailbox-connection]'), 15000).catch(() => {});
        await pause(500);
    };

    scenarios.y40conn = async () => {
        for (const role of ['admin', 'customer']) {
            let page = await open(role, READY);
            await snap(page, `menu-${role}`, { role });
            await openWindow(page);
            await snap(page, `window-${role}`, { role });
            await page.evaluate(() => document.querySelector('[data-mailbox-copy-all]')?.click());
            await pause(300);
            await closePage(page);
            for (const [name, setup] of Object.entries(PROBLEMS)) {
                page = await open(role, setup);
                await openWindow(page);
                await snap(page, `window-${name}-${role}`, { role });
                await closePage(page);
            }
        }
        await reset(); await ctl({ clearAll: true, domains: [] });
    };
}
