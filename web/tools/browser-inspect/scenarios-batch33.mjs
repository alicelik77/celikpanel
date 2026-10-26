// Batch 33 (2026-10-26): the v0.1.0-alpha.110 candidate (webmail on its own
// address).
//
//   y33address   Roundcube's Manage page: the Webmail address card without an
//                address (suggestion), a check with no record yet (the record,
//                the zone hosted here), a check that is ready and its set-up
//                confirmation, then an enabled and proved address
//
// Every answer comes from the mock on 127.0.0.1 through overrides (SAMPLE
// data shaped as the Panel answers); nothing is installed, moved or contacted.
// Otuz üçüncü grup: web postası adresi kartı; hepsi sahte.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';

const now = () => new Date().toISOString().replace(/\.\d{3}Z$/, 'Z');
const svc = (id, name, category, installed, extra = {}) => ({
    id, name, description: '', icon: '', category, status: installed ? 'active (running)' : 'not_installed', is_installed: installed,
    versions: [], packages: [id], repair_available: true, ...extra,
});
const SERVICES = [
    svc('nginx', 'Nginx', 'web', true, { kind: 'service', unit: 'nginx', version: '1.26.3' }),
    svc('php-fpm', 'PHP-FPM', 'web', true, { kind: 'runtime', versions: ['8.4'], instances: [{ version: '8.4', unit: 'php8.4-fpm', managed: true, status: 'active (running)' }] }),
    svc('postfix', 'Postfix', 'email', true, { kind: 'service', version: '3.10.1' }),
    svc('dovecot', 'Dovecot', 'email', true, { kind: 'service', packages: ['dovecot-imapd'], version: '2.4.1' }),
    svc('roundcube', 'Roundcube', 'email', true, { kind: 'tool', status: 'installed', packages: [], version: '1.6.20', php_binding: { version: '8.4', socket_ready: true, version_installed: true } }),
];
const SCAN = () => ({
    scanned_at: now(), scan_age_seconds: 0, dns_identity_ready: true,
    mail_hostname: { current: 'mail.celikhost.com', hostname: 'mail.celikhost.com', source: 'saved', current_usable: true, will_set_hostname: false },
    services: SERVICES, bundles: [],
    profiles: ['core-mail', 'webmail', 'protected-mail'].map((id) => ({ id, name: id, description: `${id} profile`, status: 'available', available: true, verified: false, latest_attempt_status: 'none', services: ['postfix'] })),
});
const HOST = 'mail.celikhost.com';
const ROUNDCUBE = (url) => ({ state: 'read', check: { installed: true, version: '1.6.20', line: '1.6', latest_on_line: '1.6.20', update_available: false, checked_at: '2026-10-26T10:00:00Z' }, last_update: null, webmail_ready: !!url, webmail_url: url });
const NONE = { address: null, suggestion: HOST, expected: ['72.62.38.15'], status: { present: false, managed: false }, last_operation: null };
const CHECK = (state) => ({
    check: {
        host: HOST, state, expected: ['72.62.38.15'], firewall: { state: 'open' }, current: '',
        ...(state === 'not_found' ? {
            code: 'no_record', ready: false, message: `Public DNS has no address record for ${HOST} yet.`,
            records: [{ type: 'A', name: 'mail', content: '72.62.38.15', ttl: 3600, text: `${HOST}. 3600 IN A 72.62.38.15` }],
            zone: { name: 'celikhost.com', records_url: '/api/v1/dns/infrastructure-zones/celikhost.com/records' },
            owner_action: { who: 'administrator', next: 'Add the record below to the zone celikhost.com hosted here (it is shown before it is added), then press Check.' },
        } : { ready: true, answered: ['72.62.38.15'], message: `Public DNS answers 72.62.38.15 for ${HOST}: this server.` }),
    },
});
const ENABLED = {
    address: { host: HOST, url: `https://${HOST}/`, enabled_at: '2026-10-26T10:05:00Z' }, suggestion: HOST, expected: ['72.62.38.15'],
    status: { present: true, managed: true, proved: true, certificate_expires: '2027-01-24T10:04:00Z', hosts: [HOST] },
    last_operation: { id: 'e'.repeat(32), request_id: 'f'.repeat(32), kind: 'webmail_address', service_id: 'roundcube', package_name: HOST, status: 'succeeded', phase: 'enabling', started_at: '2026-10-26T10:04:00Z', finished_at: '2026-10-26T10:05:00Z', result: { success: true, outcome: 'enabled', host: HOST, url: `https://${HOST}/`, certificate_expires: '2027-01-24T10:04:00Z', roundcube_settings: 'applied' }, error: null },
};

const facts = (page) => page.evaluate(() => {
    const visible = (node) => node.getClientRects().length > 0 && getComputedStyle(node).visibility !== 'hidden';
    const root = document.querySelector('[role="dialog"]') || document.querySelector('[data-webmail-address]') || document.querySelector('main') || document.body;
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

export default function batch33(scenarios, { base, locale, theme, out, viewport, ctl, reset, newPage, closePage, shot, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const snap = async (page, state) => {
        await shot(page, `y33-${state}`, await facts(page));
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `y33-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    const clean = () => ctl({ clearAll: true, b28: null, managedScan: null, componentOperation: null, domains: [], services: [] });
    const open = async (address, check, url) => {
        await reset(); await clean();
        await ctl({ override: {
            '/api/v1/managed-services': { status: 200, body: SCAN() },
            '/api/v1/managed-services/scan': { status: 200, body: SCAN() },
            '/api/v1/service/roundcube/update': { status: 200, body: ROUNDCUBE(url), method: 'GET' },
            '/api/v1/service/roundcube/address': { status: 200, body: address, method: 'GET' },
            ...(check ? { '/api/v1/service/roundcube/address/check': { status: 200, body: check, method: 'POST' } } : {}),
        } });
        const page = await newPage({});
        await page.goto(`${base}/services/roundcube`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page);
        await waitFor(page, () => !!document.querySelector('[data-webmail-address]'), 10000).catch(() => {});
        await pause(700);
        await page.evaluate(() => document.querySelector('[data-webmail-address]')?.scrollIntoView({ block: 'start' }));
        await pause(300);
        return page;
    };
    const check = async (page) => {
        await page.evaluate(() => document.querySelector('[data-webmail-check]')?.click());
        await pause(900);
    };

    scenarios.y33address = async () => {
        let page = await open(NONE, CHECK('not_found'), '');
        await snap(page, 'address-none');
        await check(page);
        await snap(page, 'address-no-record');
        await closePage(page);

        page = await open(NONE, CHECK('ready'), '');
        await check(page);
        await snap(page, 'address-ready');
        await page.evaluate(() => document.querySelector('[data-webmail-setup-offer]')?.click());
        await waitFor(page, () => !!document.querySelector('[role="dialog"]'), 5000).catch(() => {});
        await pause(400);
        await snap(page, 'address-setup-confirm');
        await closePage(page);

        page = await open(ENABLED, null, `https://${HOST}/`);
        await snap(page, 'address-enabled');
        await page.evaluate(() => document.querySelector('[data-webmail-remove-offer]')?.click());
        await waitFor(page, () => !!document.querySelector('[role="dialog"]'), 5000).catch(() => {});
        await pause(400);
        await snap(page, 'address-remove-confirm');
        await closePage(page);
        await clean();
    };
}
