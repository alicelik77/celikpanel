// Batch 35 (2026-11-14): a certificate request that gave no new certificate,
// after the alpha.134 lab check.
//
//   y35certfail  Security › Certificate: "Cover the mail name too" pressed and
//                refused, in the guidance block, for the reader's role - the
//                lab's three cases (the authority does not know the account,
//                certbot ended without an established reason, the issued
//                certificate's chain is not trusted by this server) and the
//                authority refusing a name's proof; an administrator's
//                technical details open.
//   y35adddomain Add domain with SSL ticked and the certificate refused, for
//                a customer and for an administrator.
//
// Every answer comes from the mock on 127.0.0.1 through overrides (SAMPLE
// data shaped as the Panel answers); nothing is requested or contacted.
// Otuz beşinci grup: sertifika isteği başarısız, okuyanın rolüne göre; hepsi sahte.
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
const CONNECTION = { domain: 'example.com', server_ip: '192.0.2.4', nameservers: ['ns1.example.net', 'ns2.example.net'], live_nameservers: ['ns1.example.net', 'ns2.example.net'], live_ips: ['192.0.2.4'], status: 'delegated', ssl_ready: true, glue_needed: false, nameservers_usable: true, propagation_pending: false, checked_at: '2026-11-14T07:20:00Z', dns_management_mode: 'local' };
const SSL = {
    domain_id: 1, domain_name: 'example.com', has_certificate: true, managed_names: ['example.com', 'www.example.com'],
    settings: { force_https: true, hsts_enabled: false, hsts_max_age: 300 },
    certificate: {
        id: 4, type: 'letsencrypt', provider_id: 'letsencrypt', issuer: 'Let’s Encrypt', subject: 'example.com', issued_at: '2026-11-12T12:37:35Z', expires_at: '2027-02-10T12:37:35Z',
        days_until_expiry: 88, auto_renew: true, renewal_status: 'os_scheduled', status: 'active', dns_names: ['example.com', 'www.example.com', 'autoconfig.example.com', 'autodiscover.example.com'],
        activated: true, usable: true, trust_status: 'trusted', activation_pending: false, dependents_pending: false,
    },
};
const until = '2027-02-10T12:37:35Z';
const COVERAGE = {
    checked_at: '2026-11-14T07:21:52Z', mail_name: 'mail.example.com', server_ip: '192.0.2.4',
    lines: [
        { kind: 'mail_name', name: 'mail.example.com', state: 'not_protected', reason: 'not_in_certificate', action: 'cover_mail' },
        { kind: 'site', role: 'domain', name: 'example.com', state: 'protected', by: 'domain_certificate', until },
        { kind: 'site', role: 'www', name: 'www.example.com', state: 'protected', by: 'domain_certificate', until },
        { kind: 'mail_programs', name: 'mail.server1.example.net', state: 'protected', by: 'server_mail_certificate', until },
    ],
};
const failure = (status, reason, vars) => ({ method: 'POST', status, body: { error: 'No certificate was issued.', code: 'CERTIFICATE_ISSUE_FAILED', reason, vars: { domain: 'example.com', kept: 'previous', ...vars } } });
const RECORD = '/var/log/letsencrypt/letsencrypt.log';
const CASES = [
    ['account-customer', 'customer', failure(502, 'account_unknown', {})],
    ['account-admin', 'admin', failure(502, 'account_unknown', { detail: 'An unexpected error occurred: acme.messages.Error: urn:ietf:params:acme:error:accountDoesNotExist :: The request specified an account that does not exist', record: RECORD })],
    ['tool-customer', 'customer', failure(502, 'tool', {})],
    ['untrusted-customer', 'customer', failure(409, 'issued_check', { check: 'untrusted' })],
    ['untrusted-admin', 'admin', failure(409, 'issued_check', { check: 'untrusted', detail: 'x509: certificate signed by unknown authority' })],
    ['validation-customer', 'customer', failure(502, 'validation', {})],
];

const facts = (page) => page.evaluate(() => {
    const node = document.querySelector('[data-certificate-issue]');
    const raw = /\b(ssl|kit|certcover)\.[a-zA-Z][a-zA-Z0-9_.]*\b|\{[a-z_]+\}/;
    const text = node ? node.innerText : '';
    return {
        kind: node?.getAttribute('data-certificate-issue') ?? null,
        text: text.split('\n').map((l) => l.trim()).filter(Boolean),
        rawKeys: text.split('\n').filter((l) => raw.test(l)),
        serverFile: /\/var\/log|letsencrypt\.log/.test(text),
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
    };
});

export default function batch35(scenarios, { base, locale, theme, out, viewport, ctl, reset, drainLog, newPage, closePage, shot, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const snap = async (page, state, extra = {}) => {
        await shot(page, `y35-${state}`, { ...(await facts(page)), ...extra, requests: await drainLog() });
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `y35-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    const press = (page, labels) => page.evaluate((wanted) => {
        const button = Array.from(document.querySelectorAll('button')).find((node) => wanted.includes((node.innerText || '').trim()) && !node.disabled);
        button?.click();
        return Boolean(button);
    }, labels);

    scenarios.y35certfail = async () => {
        for (const [name, role, answer] of CASES) {
            await reset();
            await ctl({ clearAll: true, domains: [DOMAIN], capabilities: CAPABILITIES, connection: CONNECTION, ssl: SSL,
                override: { [ME]: { status: 200, body: USERS[role] }, '/api/v1/domains/1/ssl/coverage': { status: 200, body: COVERAGE }, '/api/v1/domains/1/ssl/letsencrypt': answer } });
            const page = await newPage({});
            await page.goto(`${base}/domains/example.com?section=security`, { waitUntil: 'domcontentloaded' });
            await page.waitForSelector('main', { timeout: 20000 });
            await quiet(page);
            await waitFor(page, () => Array.from(document.querySelectorAll('button')).some((node) => ['Cover the mail name too', 'Posta adını da kapsa'].includes((node.innerText || '').trim()) && !node.disabled), 20000).catch(() => {});
            await pause(400);
            const pressed = await press(page, ['Cover the mail name too', 'Posta adını da kapsa']);
            await pause(500);
            await press(page, ['Continue', 'Devam et']);
            await waitFor(page, () => !!document.querySelector('[data-certificate-issue]'), 20000).catch(() => {});
            await quiet(page);
            await pause(900);
            if (role === 'admin') await page.evaluate(() => document.querySelectorAll('[data-certificate-issue] details').forEach((d) => { d.open = true; }));
            await page.evaluate(() => document.querySelector('[data-certificate-issue]')?.scrollIntoView({ block: 'start' }));
            await pause(300);
            await snap(page, `certfail-${name}`, { role, pressed });
            await closePage(page);
        }
        await reset(); await ctl({ clearAll: true, domains: [], ssl: null });
    };

    // Add domain with SSL ticked: the site is created and the certificate is
    // refused; the dialog shows the same block, with how it goes on there.
    scenarios.y35adddomain = async () => {
        for (const [name, role, answer] of [['adddomain-customer', 'customer', failure(502, 'tool', { kept: 'none' })], ['adddomain-admin', 'admin', failure(502, 'tool', { kept: 'none', detail: 'An unexpected error occurred: the webroot plugin does not appear to be installed', record: RECORD })]]) {
            await reset();
            await ctl({ clearAll: true, domains: [], capabilities: CAPABILITIES, ssl: null,
                override: { [ME]: { status: 200, body: USERS[role] }, '/api/v1/domains/create': { method: 'POST', status: 200, body: { DomainID: 7, SiteID: 3, Domain: 'example.com', ProjectType: 'static' } }, '/api/v1/domains/7/ssl/letsencrypt': answer } });
            const page = await newPage({});
            await page.goto(`${base}/domains`, { waitUntil: 'domcontentloaded' });
            await page.waitForSelector('main', { timeout: 20000 });
            await quiet(page);
            await waitFor(page, () => Array.from(document.querySelectorAll('button')).some((node) => /^(Add domain|Alan adı ekle)$/.test((node.innerText || '').trim())), 20000).catch(() => {});
            await page.evaluate(() => Array.from(document.querySelectorAll('button')).find((node) => /^(Add domain|Alan adı ekle)$/.test((node.innerText || '').trim()))?.click());
            await waitFor(page, () => !!document.querySelector('input[placeholder="example.com"]'), 15000).catch(() => {});
            await pause(800);
            await page.type('input[placeholder="example.com"]', 'example.com');
            await page.evaluate(() => { const box = document.getElementById('ssl'); if (box && !box.checked) box.click(); });
            await pause(300);
            await page.evaluate(() => document.querySelector('form button[type="submit"]')?.click());
            await waitFor(page, () => !!document.querySelector('[data-certificate-issue]'), 20000).catch(() => {});
            await quiet(page);
            await pause(900);
            if (role === 'admin') await page.evaluate(() => document.querySelectorAll('[data-certificate-issue] details').forEach((d) => { d.open = true; }));
            await snap(page, name, { role });
            await closePage(page);
        }
        await reset(); await ctl({ clearAll: true, domains: [], ssl: null });
    };
}
