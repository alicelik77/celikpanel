// Batch 36 (2026-11-14): the alpha.135 candidate's screens that the release
// integration touched.
//
//   y36holding   Server › Maintenance › What deletions left: held mail and
//                backups (one failed removal), mail left in place, and the
//                line a list cut at the Agent's limit carries
//   y36mail      A domain's mailboxes while the sending limits are applied:
//                each row's limit line (a refusal count after "·"), the
//                quota beside it, Webmail / Password… / More (administrator
//                and customer)
//   y36deleted   Deleting a domain with its databases ticked, as the
//                administrator: the finished window with the Databases
//                page's own lines, alpha.134's rights facts included
//
// Every answer comes from the mock on 127.0.0.1 through overrides (SAMPLE
// data shaped as the Panel answers); nothing is deleted, moved or contacted.
// Otuz altıncı grup: alpha.135'in birleştirmede dokunulan ekranları; hepsi sahte.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';

const ME = '/api/v1/auth/me';
const BASE_USER = { account_type: 'account', email: 'owner@example.com', impersonating: false, features: { team_members: false } };
const USERS = {
    admin: { ...BASE_USER, username: 'admin', role: 'admin', effective_role: 'admin' },
    customer: { ...BASE_USER, username: 'musteri1', role: 'customer', effective_role: 'customer' },
};
const CAPABILITIES = { dns_server: 'bind', dns_identity_ready: true, dns_management_mode: 'local', dns_management_ready: true, web_server: 'nginx', php_versions: ['8.4'], mail_server: true, database_servers: ['mariadb'], db_tools: [] };
const DOMAIN = { id: 1, domain_name: 'example.com', status: 'active', project_type: 'php', php_version: '8.4', ssl_enabled: true, created_at: '2026-09-01T10:00:00Z', disk_usage: 3277, bandwidth: 0 };

const HOLDING = {
    items: [
        { id: 'mail.domain-2-6e7ceffb7e3cb3a1', kind: 'mail', domain: 'shop.example', domain_id: 2, bytes: 12582912, deleted_at: '2026-11-14T10:00:00Z', remove_after: '2026-12-14T10:00:00Z', adopted: false, path: '/var/mail/vhosts/.celikpanel-quarantine/domain-2-6e7ceffb7e3cb3a1', state: 'kept' },
        { id: 'mail.mailbox-9-0123456789abcdef', kind: 'mail', domain: 'example.com', domain_id: 1, mailbox: 'old@example.com', bytes: 2048, deleted_at: '2026-11-13T09:00:00Z', remove_after: '2026-12-13T09:00:00Z', adopted: false, path: '/var/mail/vhosts/.celikpanel-quarantine/mailbox-9-0123456789abcdef', state: 'kept' },
        { id: 'backups.s2-d2', kind: 'backups', domain: 'shop.example', domain_id: 2, bytes: 52428800, files: 3, deleted_at: '2026-11-14T10:00:00Z', remove_after: '2026-12-14T10:00:00Z', adopted: true, path: '/var/backups/celikpanel/subscriptions/2/domains/2', state: 'failed', failure: { kind: 'failed', line: 'remove: permission denied', at: '2026-11-14T11:00:00Z' }, next_try_at: '2026-11-14T12:00:00Z' },
    ],
    summary: { count: 3, bytes: 64913760, earliest_remove_after: '2026-12-13T09:00:00Z' },
    read_at: '2026-11-14T11:05:00Z',
    left: [
        { address: 'info@example.com', domain: 'example.com', domain_id: 1, bytes: 8192, files: 4, path: '/var/mail/vhosts/example.com/info' },
        { address: 'sales@example.com', domain: 'example.com', domain_id: 1, bytes: -1, path: '/var/mail/vhosts/example.com/sales' },
    ],
    left_more: true,
};

const protocol = (port, security) => ({ host: 'mail.example.com', port, security });
const SETUP = { mail_host: 'mail.example.com', imap: protocol(993, 'SSL/TLS'), pop3: protocol(995, 'SSL/TLS'), smtp: protocol(587, 'STARTTLS'), username_is_full_email: true, webmail_available: true, webmail_url: 'https://mail.example.com/' };
const ACCOUNTS = { accounts: [{ id: 5, address: 'info@example.com', quota_mb: 2048 }, { id: 6, address: 'siparis@example.com', quota_mb: 1024 }, { id: 7, address: 'ayse.yilmaz@example.com', quota_mb: 1024 }] };
const QUOTA = { plugin_enabled: true, usages: [
    { email: 'info@example.com', used_kb: 1024, limit_kb: 2097152, available: true },
    { email: 'siparis@example.com', used_kb: 350208, limit_kb: 1048576, available: true },
    { email: 'ayse.yilmaz@example.com', used_kb: 100, limit_kb: 1048576, available: true },
] };
const mailbox = (id, address, effective = {}, extra = {}) => ({ id, address, effective: { limit: 500, mode: 'default', default: 500, ceiling: 800, ...effective }, used_hour: 12, sent_7d: 40, refused_7d: 0, ...extra });
const OUTGOING = {
    enforced: true, state: 'on', domain_limit: 750, domain_used_hour: 18, subscription_limit: 1000, subscription_used_hour: 30,
    domain_refused_7d: 0, subscription_refused_7d: 0, ceiling: 800, default: 500, may_exceed_ceiling: false,
    mailboxes: [mailbox(5, 'info@example.com', {}, { refused_7d: 3 }), mailbox(6, 'siparis@example.com', {}, { used_hour: 6 }), mailbox(7, 'ayse.yilmaz@example.com', { limit: 50, new_until: '2026-11-15T09:00:00Z' }, { used_hour: 0 })],
};

const DELETED = {
    status: 'deleted',
    kept: {
        choice: 'delete',
        deleted_databases: [{ id: 7, name: 'shop_db', engine: 'mariadb', users_removed: ['shop_user'], users_kept: ['report_user'], users_kept_on_engine: ['report_user'],
            rights_removed_count: 2, rights_removed_from: ['shop_user@localhost', 'reporting'], rights_may_remain: false,
            rights_kept: [{ account: 'hand@%', pattern: 'shop_db', reaches: ['shop1db'] }] }],
        mail: { path: '/var/mail/vhosts/.celikpanel-quarantine/domain-1-0123456789abcdef', bytes: 12582912, remove_after: '2026-12-14T10:00:00Z' },
        backups: { path: '/var/backups/celikpanel/subscriptions/1/domains/1', files: 2, bytes: 4194304, remove_after: '2026-12-14T10:00:00Z' },
    },
};

const facts = (selector) => (page) => page.evaluate((wanted) => {
    const node = document.querySelector('[role="dialog"]') || document.querySelector(wanted) || document.querySelector('main');
    const raw = /\b(holding|mail|mout|domains|databases|kit)\.[a-zA-Z][a-zA-Z0-9_.]*\b|\{[a-z_]+\}/;
    const text = node ? node.innerText : '';
    // Where the browser broke a quota ("2048 MB") over two lines.
    const quotas = Array.from(document.querySelectorAll('td, span')).filter((cell) => /^\d+ MB$/.test((cell.innerText || '').trim()) && cell.getClientRects().length > 1).map((cell) => cell.innerText);
    return {
        text: text.split('\n').map((l) => l.trim()).filter(Boolean).slice(0, 80),
        rawKeys: text.split('\n').filter((l) => raw.test(l)),
        dotWithoutSpace: /·\S/.test(text),
        brokenQuotas: quotas,
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
    };
}, selector);

export default function batch36(scenarios, { base, locale, theme, out, viewport, ctl, reset, drainLog, newPage, closePage, shot, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const snap = async (page, state, selector, extra = {}) => {
        await shot(page, `y36-${state}`, { ...(await facts(selector)(page)), ...extra, requests: await drainLog() });
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `y36-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    const open = async (path, overrides, extra = {}) => {
        await reset();
        await ctl({ clearAll: true, domains: [DOMAIN], capabilities: CAPABILITIES, ...extra, override: overrides });
        const page = await newPage({});
        await page.goto(`${base}${path}`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page);
        return page;
    };

    scenarios.y36holding = async () => {
        const page = await open('/maintenance', { [ME]: { status: 200, body: USERS.admin }, '/api/v1/maintenance/deletion-holding': { method: 'GET', status: 200, body: HOLDING } });
        await waitFor(page, () => !!document.querySelector('[data-holding-list]'), 20000).catch(() => {});
        await pause(600);
        await page.evaluate(() => document.querySelectorAll('[data-holding-list] details').forEach((d) => { d.open = true; }));
        await page.evaluate(() => (document.querySelector('[data-holding-summary]') || document.querySelector('[data-holding-left-more]'))?.scrollIntoView({ block: 'start' }));
        await pause(300);
        await snap(page, 'holding', '[data-holding-list]');
        await closePage(page);
        await reset(); await ctl({ clearAll: true, domains: [] });
    };

    scenarios.y36mail = async () => {
        for (const role of ['admin', 'customer']) {
            const page = await open('/domains/example.com?section=mail', {
                [ME]: { status: 200, body: USERS[role] },
                '/api/v1/domains/1/mail/accounts': { method: 'GET', status: 200, body: ACCOUNTS },
                '/api/v1/domains/1/mail/forwardings': { method: 'GET', status: 200, body: { forwardings: [] } },
                '/api/v1/domains/1/mail/quota': { method: 'GET', status: 200, body: QUOTA },
                '/api/v1/domains/1/mail/setup': { method: 'GET', status: 200, body: SETUP },
                '/api/v1/domains/1/mail/outgoing': { method: 'GET', status: 200, body: OUTGOING },
            });
            await waitFor(page, () => !!document.querySelector('[data-outgoing-line]'), 20000).catch(() => {});
            await pause(700);
            await page.evaluate(() => document.querySelector('[data-outgoing-line]')?.closest('table, ul, section, div')?.scrollIntoView({ block: 'start' }));
            await pause(300);
            await snap(page, `mail-${role}`, '[data-outgoing-line]', { role });
            await closePage(page);
        }
        await reset(); await ctl({ clearAll: true, domains: [] });
    };

    scenarios.y36deleted = async () => {
        const page = await open('/domains', {
            [ME]: { status: 200, body: USERS.admin },
            '/api/v1/domains/1': { method: 'DELETE', status: 200, body: DELETED },
        }, { domainDatabases: { databases: [{ id: 7, name: 'shop_db', engine: 'MariaDB 10.11', users: [{ username: 'shop_user' }, { username: 'report_user', stays: 'shared' }] }], available_types: ['mysql'] } });
        await waitFor(page, () => !!document.querySelector('[data-row-more="example.com"]'), 20000).catch(() => {});
        await page.evaluate(() => document.querySelector('[data-row-more="example.com"]')?.click());
        await pause(400);
        await page.evaluate(() => document.querySelector('[role="menuitem"][data-row-action="delete"]')?.click());
        await waitFor(page, () => !!document.querySelector('[data-confirm-option]'), 15000).catch(() => {});
        await pause(500);
        await page.evaluate(() => document.querySelector('[data-confirm-option]')?.click());
        await page.type('[data-confirm-typed] input, input[data-confirm-typed], #confirm-action-typed', 'example.com').catch(() => {});
        await pause(400);
        await snap(page, 'delete-question', '[role="dialog"]');
        await page.evaluate(() => document.querySelector('[data-confirm-action]')?.click());
        await waitFor(page, () => /example\.com (deleted|silindi)/.test(document.querySelector('[role="dialog"]')?.innerText || ''), 20000).catch(() => {});
        await pause(900);
        await page.evaluate(() => document.querySelectorAll('[role="dialog"] details').forEach((d) => { d.open = true; }));
        await snap(page, 'delete-result', '[role="dialog"]');
        await closePage(page);
        await reset(); await ctl({ clearAll: true, domains: [], domainDatabases: { databases: [], available_types: ['mysql'] } });
    };
}
