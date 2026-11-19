// Batch 39 (2026-11-19): a domain's programs in their own section (alpha.138).
//
//   y39programs  A program site (running program) as the administrator and as
//                a customer: the section row with Programs right after Site,
//                the Programs section, the Site section that is left, and the
//                Overview's tiles
//   y39service   An AI app site: the Programs section with its own card only
//   y39node      A Node.js site: no Programs in the row (neither can be added)
//   y39mixed     A domain that holds both: the AI app's site still shows the
//                program's card, to manage and remove it, and no form
//   y39held      A PHP site holding a created program, its right closed: the
//                section and the program's card with its removal
//   y39php       A PHP site: the Site section without the moved cards and the
//                Programs section with both create parts, as the
//                administrator and as a customer; a customer whose plan has no
//                program or app right (no Programs in the row); and the
//                address of alpha.132-137 (?section=site&part=programs)
//
// Every answer comes from the mock on 127.0.0.1 through overrides (SAMPLE
// data shaped as the Panel answers); nothing is changed or contacted.
// Otuz dokuzuncu grup: Programlar kendi bölümünde; hepsi sahte.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';

const ME = '/api/v1/auth/me';
const BASE_USER = { account_type: 'account', email: 'owner@example.com', impersonating: false, features: { team_members: false } };
const USERS = {
    admin: { ...BASE_USER, username: 'admin', role: 'admin', effective_role: 'admin' },
    customer: { ...BASE_USER, username: 'musteri1', role: 'customer', effective_role: 'customer' },
};
const CAPABILITIES = { dns_server: 'bind', dns_identity_ready: true, dns_management_mode: 'local', dns_management_ready: true, web_server: 'nginx', php_versions: ['8.4'], mail_server: true, database_servers: ['mariadb'], db_tools: [] };
const domain = (extra = {}) => ({ id: 1, domain_name: 'example.com', status: 'active', project_type: 'php', php_version: '8.4', ssl_enabled: true, created_at: '2026-09-01T10:00:00Z', disk_usage: 3277, bandwidth: 0, ...extra });

const risk = { can_do: ['any_code'], blocked: ['mail_ports'], network: ['https_only'], remains: ['mail_apis'] };
const limits = { memory_mb: 512, cpu_percent: 100, tasks_max: 256 };
const RUNTIME = (projectType, app) => ({
    domain: 'example.com', suspended: false, has_site: true, project_type: projectType, certificate: true, disk_full: false,
    sandbox: { known: true, level: 'full', gaps: [] },
    kinds: [{ kind: 'program', implemented: true, offering_id: 'runtime:program', offered: true, permission_granted: true, held: false, risk, limits, accepted_level: 'full', accepted_gaps: [] }],
    app, operation: null, port_range: [30000, 34999],
});
const RUNNING = {
    id: 7, kind: 'program', entry: 'server', args: ['--listen=127.0.0.1:{port}'], health_path: '/healthz', port: 30001, state: 'running', deployed: true,
    url: 'https://example.com/', folder: '/var/www/vhosts/example.com/app', upload_dir: 'app/bin', unit: 'celikrt-7.service',
    env: [{ key: 'MODE', value: 'prod', secret: false }],
    status: { unit_file: 'x', unit: { present: true, active: true, enabled: true }, serving: true, health: { checked: true, status: 200, ok: true } },
    states: [], created_at: '2026-11-03 10:00:00',
};
const N8N = {
    id: 'n8n', name: 'n8n', vendor: 'n8n GmbH', summary: 'Workflow automation', source: 'https://example.com/n8n', licence: 'Sustainable Use License', runtime: 'node', version: '1.70.0',
    customer_offerable: true, defaults: { memory_mb: 1024, cpu_percent: 100, tasks_max: 512, disk_mb: 2048 },
    risk: { can_do: [], disabled: [], network: [], remains: [] }, secrets: null, owner_login: true, available: true, held: false, offered: true,
};
const SERVICE = (projectType, install) => ({
    domain: 'example.com', suspended: false, has_site: true, project_type: projectType, certificate: true, permission_granted: true,
    availability_known: true, apps: [N8N], install, operation: null,
});
const INSTALLED = {
    id: 3, app_id: 'n8n', name: 'n8n', version: '1.70.0', target_version: '1.70.0', state: 'installed', url: 'https://example.com/', held: false,
    active: true, enabled: true, active_state: 'active', memory_mb: 310, serving: true, unit: 'celiksvc-3.service', folder: '/var/www/vhosts/example.com/n8n',
};

const facts = (page) => page.evaluate(() => {
    const row = document.querySelector('[data-domain-sections]');
    const tabs = row ? Array.from(row.querySelectorAll('[role="tab"]')) : [];
    const index = document.querySelector('[data-section-index]');
    const raw = /\b(domain|runtime|svcapps|hosting)\.[a-zA-Z][a-zA-Z0-9_.]*\b|\{[a-z_]+\}/;
    const text = document.querySelector('main')?.innerText || '';
    return {
        row: tabs.map((tab) => tab.innerText.trim()),
        selected: tabs.filter((tab) => tab.getAttribute('aria-selected') === 'true').map((tab) => tab.innerText.trim()),
        rowWraps: row ? new Set(tabs.map((tab) => Math.round(tab.getBoundingClientRect().top))).size > 1 : null,
        rowScrolls: row ? row.scrollWidth > row.clientWidth : null,
        rowSpare: row ? row.clientWidth - tabs.reduce((sum, tab) => sum + tab.getBoundingClientRect().width, 0) : null,
        index: index ? index.innerText.split('\n').map((l) => l.trim()).filter(Boolean) : [],
        parts: Array.from(document.querySelectorAll('[data-part]')).map((node) => node.getAttribute('data-part')),
        address: location.search,
        rawKeys: text.split('\n').filter((l) => raw.test(l)),
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
    };
});

export default function batch39(scenarios, { base, locale, theme, out, viewport, ctl, reset, drainLog, newPage, closePage, shot, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const snap = async (page, state, extra = {}) => {
        await shot(page, `y39-${state}`, { ...(await facts(page)), ...extra, requests: await drainLog() });
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `y39-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    const open = async (path, row, role, overrides) => {
        await reset();
        await ctl({ clearAll: true, domains: [row], capabilities: CAPABILITIES, override: { [ME]: { status: 200, body: USERS[role] }, ...overrides } });
        const page = await newPage({});
        await page.goto(`${base}${path}`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('[data-domain-sections]', { timeout: 20000 });
        await quiet(page);
        return page;
    };
    const top = (page) => page.evaluate(() => window.scrollTo(0, 0));
    const settle = async (page, selector) => {
        await waitFor(page, (wanted) => !!document.querySelector(wanted), 20000, selector).catch(() => {});
        await pause(800);
        await top(page);
        await pause(200);
    };
    const reads = (projectType, app, install) => ({
        '/api/v1/domains/1/runtime-apps': { method: 'GET', status: 200, body: RUNTIME(projectType, app) },
        '/api/v1/domains/1/service-apps': { method: 'GET', status: 200, body: SERVICE(projectType, install) },
    });

    scenarios.y39programs = async () => {
        for (const role of ['admin', 'customer']) {
            const row = domain({ project_type: 'runtime_app' });
            const page = await open('/domains/example.com?section=programs', row, role, reads('runtime_app', RUNNING, null));
            await settle(page, '[data-part="programs"]');
            await snap(page, `programs-${role}`, { role });
            await page.evaluate(() => document.querySelector('[data-section="site"]')?.click());
            await settle(page, '[data-part="type"]');
            await snap(page, `program-site-${role}`, { role });
            await page.evaluate(() => document.querySelector('[data-section="overview"]')?.click());
            await settle(page, '[data-overview-tile]');
            await page.evaluate(() => document.querySelector('[data-overview-tile]')?.scrollIntoView({ block: 'center' }));
            await pause(300);
            await snap(page, `program-overview-${role}`, { role });
            await closePage(page);
        }
        await reset(); await ctl({ clearAll: true, domains: [] });
    };

    scenarios.y39service = async () => {
        const page = await open('/domains/example.com?section=programs', domain({ project_type: 'service_app' }), 'admin', reads('service_app', null, INSTALLED));
        await settle(page, '[data-part="ai"]');
        await snap(page, 'service-admin', { role: 'admin' });
        await closePage(page);
        await reset(); await ctl({ clearAll: true, domains: [] });
    };

    // A domain that holds both (an AI app installed over a program before
    // alpha.138 refused that): the app's site still draws the program's card,
    // to manage and remove it, with no form in either card.
    scenarios.y39mixed = async () => {
        for (const role of ['admin', 'customer']) {
            const page = await open('/domains/example.com?section=programs', domain({ project_type: 'service_app', holds_program: true, holds_app: true }), role, reads('service_app', RUNNING, INSTALLED));
            await settle(page, '[data-part="programs"]');
            await snap(page, `mixed-${role}`, { role });
            await page.evaluate(() => document.querySelector('[data-part="programs"]')?.scrollIntoView({ block: 'start' }));
            await pause(300);
            await snap(page, `mixed-program-${role}`, { role });
            await closePage(page);
        }
        await reset(); await ctl({ clearAll: true, domains: [] });
    };

    // A PHP site holding a program that was created and never deployed, and
    // the plan's program rights closed since: the section and the program's
    // card are there, so the program can be removed (and the domain deleted).
    scenarios.y39held = async () => {
        const created = { ...RUNNING, state: 'created', deployed: false, status: undefined, states: ['not_offered'] };
        const page = await open('/domains/example.com?section=programs',
            domain({ rights_closed: ['program_apps', 'python_apps', 'service_apps'], holds_program: true }), 'customer', reads('php', created, null));
        await settle(page, '[data-part="programs"]');
        await snap(page, 'held-customer', { role: 'customer' });
        await closePage(page);
        await reset(); await ctl({ clearAll: true, domains: [] });
    };

    // alpha.138 lab: a PHP site with a created program and every right open.
    // The server refuses an AI app there (program_created), so the AI card
    // and its "Install" are not drawn; the program's card is.
    scenarios.y39created = async () => {
        const created = { ...RUNNING, state: 'created', deployed: false, status: undefined, states: [] };
        for (const role of ['admin', 'customer']) {
            const page = await open('/domains/example.com?section=programs', domain({ holds_program: true }), role, reads('php', created, null));
            await settle(page, '[data-part="programs"]');
            await snap(page, `created-${role}`, { role });
            await closePage(page);
        }
        await reset(); await ctl({ clearAll: true, domains: [] });
    };

    // alpha.138 lab: the section's "?" → "Help for this page" opens the
    // guide's domains chapter at its Programs part, not at its top.
    scenarios.y39guide = async () => {
        const page = await open('/domains/example.com?section=programs', domain(), 'admin', reads('php', null, null));
        await settle(page, '[data-part="programs"]');
        await page.evaluate(() => document.querySelector('[data-help-button]')?.click());
        await waitFor(page, () => !!document.querySelector('[data-help-menu] button'), 10000).catch(() => {});
        await page.evaluate(() => document.querySelector('[data-help-menu] button')?.click());
        await waitFor(page, () => !!document.getElementById('guide-domains-websites-programs'), 15000).catch(() => {});
        await pause(800);
        const place = await page.evaluate(() => {
            const heading = document.getElementById('guide-domains-websites-programs');
            const drawer = document.querySelector('[data-guide-drawer] [role="dialog"]');
            if (!heading || !drawer) return { found: false };
            const box = drawer.getBoundingClientRect();
            const top = heading.getBoundingClientRect().top - box.top;
            return { found: true, headingTopInDrawer: Math.round(top), drawerHeight: Math.round(box.height), heading: heading.innerText.trim() };
        });
        await snap(page, 'guide-admin', { role: 'admin', place });
        await closePage(page);
        await reset(); await ctl({ clearAll: true, domains: [] });
    };

    // Neither a program nor an AI app can be added to a Node.js site: no
    // Programs in the row, also when the old address asks for it.
    scenarios.y39node = async () => {
        const page = await open('/domains/example.com?section=programs', domain({ project_type: 'node', php_version: '' }), 'admin', reads('node', null, null));
        await settle(page, '[data-section-body]');
        await snap(page, 'node-admin', { role: 'admin' });
        await closePage(page);
        await reset(); await ctl({ clearAll: true, domains: [] });
    };

    scenarios.y39php = async () => {
        for (const role of ['admin', 'customer']) {
            const page = await open('/domains/example.com?section=site', domain(), role, reads('php', null, null));
            await settle(page, '[data-part="type"]');
            await snap(page, `php-site-${role}`, { role });
            await closePage(page);
        }
        // Both cards, each with its create part: a PHP site can take either.
        for (const role of ['admin', 'customer']) {
            const page = await open('/domains/example.com?section=programs', domain(), role, reads('php', null, null));
            await settle(page, '[data-part="ai"]');
            await snap(page, `php-programs-${role}`, { role });
            await page.evaluate(() => document.querySelector('[data-part="ai"]')?.scrollIntoView({ block: 'start' }));
            await pause(300);
            await snap(page, `php-programs-ai-${role}`, { role });
            await closePage(page);
        }
        const closed = await open('/domains/example.com?section=site', domain({ rights_closed: ['program_apps', 'python_apps', 'service_apps'] }), 'customer', {});
        await settle(closed, '[data-part="type"]');
        await snap(closed, 'php-site-no-rights-customer', { role: 'customer' });
        await closePage(closed);
        const old = await open('/domains/example.com?section=site&part=programs', domain(), 'customer', reads('php', null, null));
        await settle(old, '[data-part="ai"]');
        await snap(old, 'old-address-customer', { role: 'customer' });
        await closePage(old);
        await reset(); await ctl({ clearAll: true, domains: [] });
    };
}
