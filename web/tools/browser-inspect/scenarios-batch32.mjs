// Batch 32 (2026-10-26): the v0.1.0-alpha.109 candidate (Frankfurt's webmail
// update, the webmail link, install confirmations people can read).
//
//   y32confirm   phpMyAdmin's install dialog on Ubuntu: the grouped
//                confirmation (packages a vendor repository replaces, the
//                additional packages under their reasons, the footer)
//   y32webmail   Components: the Roundcube row after Frankfurt's failed and
//                rolled-back update (Open webmail held back); then its Manage page
//
// Every answer comes from the mock on 127.0.0.1 through overrides (SAMPLE
// data shaped as the Panel answers); nothing is installed, moved or contacted.
// Otuz ikinci grup: alpha.109 onayı ve web postası satırı; hepsi sahte.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';

const now = () => new Date().toISOString().replace(/\.\d{3}Z$/, 'Z');
const svc = (id, name, category, installed, extra = {}) => ({
    id, name, description: '', icon: '', category, status: installed ? 'active (running)' : 'not_installed', is_installed: installed,
    versions: [], packages: [id], repair_available: true, ...extra,
});
const SERVICES = [
    svc('nginx', 'Nginx', 'web', true, { kind: 'service', unit: 'nginx', version: '1.24.0' }),
    svc('php-fpm', 'PHP-FPM', 'web', true, { kind: 'runtime', versions: ['8.3'], instances: [{ version: '8.3', unit: 'php8.3-fpm', managed: true, status: 'active (running)' }] }),
    svc('mariadb', 'MariaDB', 'database', false, { kind: 'service', packages: ['mariadb-server'] }),
    svc('phpmyadmin', 'phpMyAdmin', 'database', false, { kind: 'tool', status: 'not_installed' }),
    svc('postfix', 'Postfix', 'email', true, { kind: 'service', version: '3.8.6' }),
    svc('dovecot', 'Dovecot', 'email', true, { kind: 'service', packages: ['dovecot-imapd'], version: '2.3.21' }),
    svc('roundcube', 'Roundcube', 'email', true, { kind: 'tool', status: 'installed', packages: [], version: '1.6.15', php_binding: { version: '8.4', socket_ready: true, version_installed: true } }),
];
const SCAN = () => ({
    scanned_at: now(), scan_age_seconds: 0, dns_identity_ready: true,
    mail_hostname: { current: 'mail.example.net', hostname: 'mail.example.net', source: 'saved', current_usable: true, will_set_hostname: false },
    services: SERVICES, bundles: [],
    profiles: ['core-mail', 'webmail', 'protected-mail'].map((id) => ({ id, name: id, description: `${id} profile`, status: 'available', available: true, verified: false, latest_attempt_status: 'none', services: ['postfix'] })),
});
const NOTABLE = [
    { package: 'php-common', reason: 'vendor_shared', component: 'sury', from: '2:93ubuntu2', to: '2:101+0~20261021.78+ubuntu24.04~1.gbp53f25f' },
    { package: 'php8.5-common', reason: 'new_php_version', version: '8.5' },
    { package: 'php8.5-mbstring', reason: 'new_php_version', version: '8.5' },
    { package: 'php8.5-mysql', reason: 'new_php_version', version: '8.5' },
    { package: 'apache2', reason: 'apache' },
    { package: 'mariadb-server', reason: 'database_server' },
];
const B28 = { candidates: { phpmyadmin: '4:5.2.1+dfsg-3' }, repos: {}, preview: { state: 'known', installs: [], notable: NOTABLE, refusals: [], removes: [], requested: [], distribution: 'ubuntu' }, previewFor: {} };
const REASON = "webmail did not answer on 1.6.20 (PHP-FPM 8.4 could not open CelikPanel's PHP check for Webmail (Roundcube) (HTTP 404: 404 Not Found)); the previous 1.6.15 tree was put back byte for byte and answers again";
const FAILED = {
    id: 'c'.repeat(32), request_id: 'd'.repeat(32), kind: 'webmail_update', service_id: 'roundcube', package_name: '1.6.20', status: 'failed', phase: 'updating',
    started_at: '2026-10-25T09:58:00Z', finished_at: '2026-10-25T10:00:00Z',
    result: { success: false, outcome: 'rolled_back', previous_answers: true, rolled_back: true, code: 'PROOF_FAILED', from: '1.6.15', to: '1.6.20', reason: REASON },
    error: { code: 'webmail_update_failed', message: 'Webmail was not updated.' },
};
const ROUNDCUBE = { state: 'read', check: { installed: true, version: '1.6.15', line: '1.6', latest_on_line: '1.6.20', latest_overall: '1.7.5', update_available: true, checked_at: '2026-10-25T10:00:00Z' }, last_update: FAILED, webmail_ready: true };

const facts = (page) => page.evaluate(() => {
    const visible = (node) => node.getClientRects().length > 0 && getComputedStyle(node).visibility !== 'hidden';
    const root = document.querySelector('[role="dialog"]') || document.querySelector('main') || document.body;
    const raw = /\b(life|services|common|mail)\.[a-zA-Z][a-zA-Z0-9_.]*\b|\{[a-zA-Z]+\}/;
    return {
        dialog: !!document.querySelector('[role="dialog"]'),
        text: root.innerText.split('\n').map((l) => l.trim()).filter(Boolean).slice(0, 60),
        rawKeys: root.innerText.split('\n').filter((l) => raw.test(l)).slice(0, 6),
        buttons: Array.from(root.querySelectorAll('button')).filter(visible).map((b) => `${b.innerText.trim()}${b.disabled ? ' [disabled]' : ''}`).slice(-6),
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
        dialogOverflowX: (() => { const d = document.querySelector('[role="dialog"]'); return d ? d.scrollWidth > d.clientWidth + 1 : false; })(),
    };
});

export default function batch32(scenarios, { base, locale, theme, out, viewport, ctl, reset, newPage, closePage, shot, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const snap = async (page, state) => {
        await shot(page, `y32-${state}`, await facts(page));
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `y32-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    const clean = () => ctl({ clearAll: true, b28: null, managedScan: null, componentOperation: null, domains: [], services: [] });
    const open = async (path, ready, extra = {}, storage = {}, b28 = null) => {
        await reset(); await clean();
        await ctl({ ...(b28 ? { b28 } : {}), override: {
            '/api/v1/managed-services': { status: 200, body: SCAN() },
            '/api/v1/managed-services/scan': { status: 200, body: SCAN() },
            '/api/v1/service/roundcube/update': { status: 200, body: ROUNDCUBE, method: 'GET' },
            ...extra,
        } });
        const page = await newPage(storage);
        await page.goto(`${base}${path}`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page);
        await waitFor(page, ready, 10000).catch(() => {});
        await pause(600);
        return page;
    };

    scenarios.y32confirm = async () => {
        const page = await open('/services/phpmyadmin', () => !!document.querySelector('main button'), {}, {}, B28);
        await page.evaluate(() => {
            const button = Array.from(document.querySelectorAll('main button')).find((b) => /Install| kur$/.test(b.innerText.trim()));
            button?.click();
        });
        await waitFor(page, () => !!document.querySelector('[data-install-preview="notable"]'), 8000).catch(() => {});
        await pause(500);
        await snap(page, 'confirm-grouped');
        await page.evaluate(() => { const d = document.querySelector('[role="dialog"] [data-install-preview="notable"]'); d?.scrollIntoView({ block: 'end' }); });
        await pause(300);
        await snap(page, 'confirm-grouped-end');
        await closePage(page);
        await clean();
    };

    scenarios.y32webmail = async () => {
        const page = await open('/services', () => !!document.querySelector('[data-row-webmail-update]'), {}, { 'celikpanel.components.view': 'installed' });
        await page.evaluate(() => document.querySelector('[data-row-webmail-update]')?.scrollIntoView({ block: 'center' }));
        await pause(300);
        await snap(page, 'webmail-row');
        await closePage(page);
        const manage = await open('/services/roundcube', () => !!document.querySelector('[data-webmail-update]'));
        await manage.evaluate(() => document.querySelector('[data-webmail-update]')?.scrollIntoView({ block: 'start' }));
        await pause(300);
        await snap(manage, 'webmail-card');
        await closePage(manage);
        await clean();
    };
}
