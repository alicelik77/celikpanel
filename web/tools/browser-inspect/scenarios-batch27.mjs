// Batch 27 (2026-10-23): the v0.1.0-alpha.104 candidate (D-037 amendment).
// A removal never removes anything it did not name; each PHP tool runs on a
// PHP version the owner chooses.
//
//   y27versions   the PHP-FPM versions list saying who uses each version; the
//                 version-removal dialog for PHP 8.4 on a Frankfurt-shaped
//                 server (tools on it, what the package manager would also
//                 take, the default PHP: refused before the button); only
//                 tools on it (move them, then remove); a failed move; the
//                 read held open
//   y27removal    the component removal dialog naming collateral (MariaDB,
//                 whose removal would take phpMyAdmin); a PHP tool whose PHP
//                 is gone reads "Not ready" on the Components page
//   y27tool       phpPgAdmin's Manage window with its PHP version card: the
//                 current version, a move in progress, a move put back
//   y27review     the review corrections: tools moved, then the removal
//                 refused; the installed versions not read; the package
//                 manager busy (no move offered); a tool whose extension
//                 packages are missing (row and card); a failed move's cause
//                 in the reader's language; the tool page's phone targets
//
// Every answer comes from the mock on 127.0.0.1 through its overrides (SAMPLE
// data shaped as the Panel answers, not measured). Nothing is installed,
// removed or contacted. Screenshots also go to BROWSER_INSPECT_SHOTS as
// <state>-<lang>-<theme>-<width>.png.
// Yirmi yedinci grup: alpha.104 kaldırma ve PHP araçları.
import { copyFile, mkdir } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { withSamplePackageVersions } from './sample-package-versions.mjs';

const now = () => new Date().toISOString().replace(/\.\d{3}Z$/, 'Z');
const svc = (id, name, category, installed, extra = {}) => ({
    id, name, description: '', icon: '', category, status: installed ? 'active (running)' : 'not_installed', is_installed: installed,
    versions: [], packages: [id], repair_available: true, ...extra,
});
const inst = (version) => ({ version, unit: `php${version}-fpm`, managed: true, status: 'active (running)' });
const tool = (id, name, category, extra = {}) => svc(id, name, category, true, { kind: 'tool', status: 'installed', ...extra });
const SERVICES = [
    svc('nginx', 'Nginx', 'web', true, { kind: 'service', unit: 'nginx' }),
    svc('php-fpm', 'PHP-FPM', 'web', true, { kind: 'runtime', versions: ['8.6', '8.5', '8.4'], instances: [inst('8.6'), inst('8.5'), inst('8.4')], packages: ['php-fpm'] }),
    svc('mariadb', 'MariaDB', 'database', true, { kind: 'service', unit: 'mariadb', packages: ['mariadb-server'] }),
    svc('postgresql', 'PostgreSQL', 'database', true, { kind: 'service', unit: 'postgresql' }),
    tool('phpmyadmin', 'phpMyAdmin', 'database', { php_binding: { tool: 'phpmyadmin', installed: true, version: '8.5', socket_ready: true, version_installed: true } }),
    tool('phppgadmin', 'phpPgAdmin', 'database', { php_binding: { tool: 'phppgadmin', installed: true, version: '8.3', socket_ready: false, version_installed: false } }),
    svc('postfix', 'Postfix', 'email', true, { kind: 'service', unit: 'postfix' }),
    svc('dovecot', 'Dovecot', 'email', true, { kind: 'service', unit: 'dovecot', packages: ['dovecot-imapd'] }),
    tool('roundcube', 'Roundcube', 'email', { packages: [], php_binding: { tool: 'webmail', installed: true, version: '8.4', socket_ready: true, version_installed: true } }),
].map((item) => withSamplePackageVersions(item, 'debian13'));
const SCAN = (services = SERVICES) => ({
    scanned_at: now(), scan_age_seconds: 0, dns_identity_ready: true,
    mail_hostname: { current: 'mail.example.net', hostname: 'mail.example.net', source: 'saved', current_usable: true, will_set_hostname: false },
    services, bundles: [],
    profiles: ['core-mail', 'webmail', 'protected-mail'].map((id) => ({ id, name: id, description: `${id} profile`, status: 'available', available: true, verified: false, latest_attempt_status: 'none', services: ['postfix'] })),
});
const PHP_TOOLS = {
    state: 'read',
    tools: [
        { tool: 'phpmyadmin', service_id: 'phpmyadmin', installed: true, version: '8.5', ready: true, version_installed: true },
        { tool: 'phppgadmin', service_id: 'phppgadmin', installed: true, version: '8.4', ready: true, version_installed: true },
        { tool: 'webmail', service_id: 'roundcube', installed: true, version: '8.4', ready: true, version_installed: true },
    ],
    versions: [
        { version: '8.6', sites: 0, site_names: [], tools: [] },
        { version: '8.5', sites: 1, site_names: ['shop.example.com (ali)'], tools: ['phpmyadmin'] },
        { version: '8.4', sites: 0, site_names: [], tools: ['phppgadmin', 'webmail'] },
    ],
};
const sim = (extra = {}) => ({ state: 'checked', remove: ['php8.4-fpm', 'php8.4-cli', 'php8.4-common', 'php8.4-opcache', 'php8.4-mysql', 'php8.4-pgsql', 'php8.4-readline'], collateral: [], bound_tools: [], default_runtime: false, refuses: false, ...extra });
const versionImpact = (simulation) => ({
    service_id: 'php-fpm', removal: 'packages', has_units: true, setup_required: false, package_family: 'apt', packages: ['php8.4-fpm'], all_versions: false, firewall_ports: [],
    mail_filter_rewired: false, database_tools_reconfigured: false, required_by: [], bundles: [], hosted: { kind: 'php_sites', count: 0, names: [], blocks_removal: false },
    package: 'php8.4-fpm', version: '8.4', simulation,
});
const FRANKFURT = sim({
    collateral: [
        { package: 'php-fpm', reason: 'component', component: 'php-fpm' },
        { package: 'phpmyadmin', reason: 'component', component: 'phpmyadmin' },
        { package: 'phppgadmin', reason: 'component', component: 'phppgadmin' },
        { package: 'php-mysql', reason: 'reverse_dependency' },
        { package: 'php-twig', reason: 'reverse_dependency' },
        { package: 'php-symfony-config', reason: 'reverse_dependency' },
    ],
    bound_tools: ['phppgadmin', 'webmail'], default_runtime: true, refuses: true,
});
const MARIADB = {
    service_id: 'mariadb', removal: 'packages', has_units: true, setup_required: false, package_family: 'apt', packages: ['mariadb-server'], all_versions: false, firewall_ports: [],
    mail_filter_rewired: false, database_tools_reconfigured: false, required_by: ['phpmyadmin'], bundles: [{ id: 'db.mariadb', name_code: 'db_mariadb_phpmyadmin' }],
    hosted: { kind: 'databases', count: 2, names: ['shop', 'blog'], blocks_removal: true },
    simulation: sim({ remove: ['mariadb-server', 'mariadb-client', 'galera-4'], collateral: [{ package: 'phpmyadmin', reason: 'component', component: 'phpmyadmin' }], refuses: true }),
};
const MOVE_FAILED = {
    error: 'Webmail (Roundcube) did not work on PHP 8.6 (PHP-FPM 8.6 lacks intl for Webmail (Roundcube)); it was put back on PHP 8.4', code: 'PHP_TOOL_MOVE_FAILED',
    detail: 'PHP-FPM 8.6 lacks intl for Webmail (Roundcube)',
    tool: 'webmail', version: '8.6', previous: '8.4', restored: true, mutation_applied: true, moved: ['phppgadmin'], not_attempted: [],
};
const RAW = /\b(services|component|nav|common|err)\.[a-zA-Z][a-zA-Z0-9_.]*\b|\{[a-zA-Z]+\}/;
const ENGLISH = ['Used by', 'Move them', 'then remove', 'Not ready', 'PHP version', 'Run on', 'would also remove', 'runs on PHP', 'Remove version', 'Cancel'];

const facts = (page, locale) => page.evaluate((rawSource, english, locale) => {
    const visible = (node) => node.getClientRects().length > 0 && getComputedStyle(node).visibility !== 'hidden';
    const body = document.body.innerText || '';
    const lines = body.split('\n').map((l) => l.trim()).filter(Boolean);
    const re = new RegExp(rawSource);
    const dialog = document.querySelector('[role="dialog"]');
    const small = Array.from(document.querySelectorAll('[role="dialog"] button, [role="dialog"] select, main button, main select')).filter(visible)
        .map((node) => ({ node, r: node.getBoundingClientRect() }))
        .filter(({ r }) => window.innerWidth <= 639 && r.height < 43.5)
        .map(({ node, r }) => `${(node.innerText || node.getAttribute('aria-label') || '').trim().slice(0, 40)} ${Math.round(r.height)}px`);
    return {
        rawKeys: lines.filter((line) => re.test(line)).slice(0, 8),
        englishLeaks: locale === 'tr' ? english.filter((item) => body.includes(item)) : [],
        usage: Array.from(document.querySelectorAll('[data-version-usage]')).map((n) => `${n.getAttribute('data-version-usage')}: ${n.innerText}`),
        rowStates: Array.from(document.querySelectorAll('[data-row-state]')).map((n) => n.getAttribute('data-row-state')),
        notReady: Array.from(document.querySelectorAll('[data-row-not-ready]')).map((n) => n.innerText),
        dialog: dialog ? {
            state: dialog.querySelector('[data-removal-impact]')?.getAttribute('data-removal-impact') ?? null,
            sim: Array.from(dialog.querySelectorAll('[data-removal-sim]')).map((n) => `${n.getAttribute('data-removal-sim')}: ${n.innerText.replace(/\s+/g, ' ').trim()}`),
            move: dialog.querySelector('[data-version-move]')?.innerText ?? null,
            failure: dialog.querySelector('[data-version-move-failure]')?.innerText ?? null,
            scrolls: dialog.scrollHeight > dialog.clientHeight + 1,
            overflowX: dialog.scrollWidth > dialog.clientWidth + 1,
            buttons: Array.from(dialog.querySelectorAll('button')).filter(visible).map((b) => `${b.innerText.trim()}${b.disabled ? ' [disabled]' : ''}`).slice(-3),
        } : null,
        card: document.querySelector('[data-php-tool]')?.innerText ?? null,
        smallTargets: small.slice(0, 12),
        pageOverflowX: document.documentElement.scrollWidth > window.innerWidth,
    };
}, RAW.source, ENGLISH, locale);

export default function batch27(scenarios, { base, locale, theme, out, viewport, ctl, reset, newPage, closePage, shot, waitFor, pause, quiet }) {
    const shots = resolve(process.env.BROWSER_INSPECT_SHOTS || out);
    const snap = async (page, state, note = {}) => {
        await shot(page, `y27-${state}`, { ...(await facts(page, locale)), ...note });
        await mkdir(shots, { recursive: true });
        await copyFile(join(out, `y27-${state}.png`), join(shots, `${state}-${locale}-${theme}-${viewport.width}.png`));
    };
    const whole = async (page, state, note = {}) => {
        const height = await page.evaluate(() => { const m = document.querySelector('main'); return m ? m.scrollHeight + (window.innerHeight - m.clientHeight) : document.body.scrollHeight; });
        await page.setViewport({ ...viewport, height: Math.min(Math.max(height, viewport.height), 6000) });
        await pause(500);
        await snap(page, state, { ...note, wholeHeight: height });
        await page.setViewport(viewport);
        await pause(300);
    };
    const clean = () => ctl({ clearAll: true, b3: null, b12: null, b16: null, b19: null, b20: null, b21: null, b22: null, b26: null, managedScan: null, componentOperation: null, domains: [], infrastructureZones: [], twoFactor: false, services: [] });
    const serve = (extra = {}) => ctl({ override: {
        '/api/v1/managed-services': { status: 200, body: SCAN() },
        '/api/v1/managed-services/scan': { status: 200, body: SCAN() },
        '/api/v1/service/php-tools': { status: 200, body: PHP_TOOLS },
        '/api/v1/repo': { status: 200, body: { available: false } },
        ...extra,
    } });
    const open = async (path, ready) => {
        const page = await newPage({ 'celikpanel.components.view': 'installed' });
        await page.goto(`${base}${path}`, { waitUntil: 'domcontentloaded' });
        await page.waitForSelector('main', { timeout: 20000 });
        await quiet(page);
        if (ready) await waitFor(page, ready, 15000).catch(() => {});
        await pause(900);
        return page;
    };
    const rowsReady = () => document.querySelectorAll('main li').length >= 4;
    const openVersions = (page) => page.evaluate(() => {
        for (const li of document.querySelectorAll('main li')) {
            if (!(li.innerText || '').startsWith('PHP-FPM')) continue;
            const toggle = Array.from(li.querySelectorAll('button')).find((b) => /versions|sürüm/i.test(b.innerText));
            if (toggle) { toggle.click(); return true; }
        }
        return false;
    });
    const openVersionRemoval = (page, version) => page.evaluate((version) => {
        const usage = document.querySelector(`[data-version-usage="${version}"]`);
        const li = usage?.closest('li');
        const trash = li ? Array.from(li.querySelectorAll('button[title]')).find((b) => /Remove version|Sürümü kaldır|kaldır/i.test(b.getAttribute('title'))) : null;
        if (trash) { trash.click(); return true; }
        return false;
    }, version);
    const openRemoval = (page, name) => page.evaluate((name) => {
        for (const li of document.querySelectorAll('main li')) {
            if (!(li.innerText || '').split('\n')[0].includes(name)) continue;
            const button = Array.from(li.querySelectorAll('button[title]')).find((b) => /Uninstall|Kaldır/.test(b.getAttribute('title')));
            if (button) { button.click(); return true; }
        }
        return false;
    }, name);

    scenarios.y27versions = async () => {
        await reset(); await clean(); await serve({ '/api/v1/service/removal-impact': { status: 200, body: versionImpact(FRANKFURT) } });
        let page = await open('/services', rowsReady);
        await openVersions(page); await pause(900);
        await whole(page, 'versions-usage');
        await openVersionRemoval(page, '8.4');
        await waitFor(page, () => !!document.querySelector('[data-removal-impact="known"]'), 8000).catch(() => {});
        await pause(400);
        await snap(page, 'version-removal-frankfurt');
        await closePage(page);

        await reset(); await clean();
        await serve({
            '/api/v1/service/removal-impact': { status: 200, body: versionImpact(sim({ bound_tools: ['phppgadmin', 'webmail'], refuses: true })) },
            '/api/v1/service/uninstall': { status: 409, body: MOVE_FAILED, delay: 2500 },
        });
        page = await open('/services', rowsReady);
        await openVersions(page); await pause(900);
        await openVersionRemoval(page, '8.4');
        await waitFor(page, () => !!document.querySelector('[data-removal-impact="known"]'), 8000).catch(() => {});
        await pause(400);
        await snap(page, 'version-removal-move');
        await page.evaluate(() => document.querySelector('[data-version-move-remove]')?.click());
        await pause(700);
        await snap(page, 'version-removal-moving');
        await waitFor(page, () => !!document.querySelector('[data-version-move-failure]'), 8000).catch(() => {});
        await pause(500);
        await snap(page, 'version-removal-move-failed');
        await closePage(page);

        await reset(); await clean(); await serve({ '/api/v1/service/removal-impact': { hang: true } });
        page = await open('/services', rowsReady);
        await openVersions(page); await pause(900);
        await openVersionRemoval(page, '8.4');
        await pause(800);
        await snap(page, 'version-removal-checking');
        await closePage(page);
        await clean();
    };

    scenarios.y27removal = async () => {
        await reset(); await clean(); await serve({ '/api/v1/service/removal-impact': { status: 200, body: MARIADB } });
        const page = await open('/services', rowsReady);
        await whole(page, 'components-not-ready');
        await openRemoval(page, 'MariaDB');
        await waitFor(page, () => !!document.querySelector('[data-removal-impact="known"]'), 8000).catch(() => {});
        await pause(400);
        await snap(page, 'removal-collateral');
        await closePage(page);
        await clean();
    };

    scenarios.y27tool = async () => {
        await reset(); await clean(); await serve();
        let page = await open('/services/phppgadmin', () => !!document.querySelector('[data-php-tool-state]'));
        await snap(page, 'tool-php-version');
        await closePage(page);

        await reset(); await clean(); await serve({ '/api/v1/service/php-tools/bind': { hang: true } });
        page = await open('/services/phppgadmin', () => !!document.querySelector('[data-php-tool-select]'));
        await page.evaluate(() => Array.from(document.querySelectorAll('[data-php-tool] button')).find((b) => /PHP 8\.6/.test(b.innerText))?.click());
        await pause(900);
        await snap(page, 'tool-php-version-moving');
        await closePage(page);

        await reset(); await clean();
        await serve({ '/api/v1/service/php-tools/bind': { status: 409, body: {
            error: 'phpPgAdmin did not work on PHP 8.6 (PHP-FPM 8.6 lacks pgsql for phpPgAdmin); it was put back on PHP 8.4', code: 'PHP_TOOL_PROBE_FAILED', detail: 'PHP-FPM 8.6 lacks pgsql for phpPgAdmin', restored: true, previous: '8.4',
        } } });
        page = await open('/services/phppgadmin', () => !!document.querySelector('[data-php-tool-select]'));
        await page.evaluate(() => Array.from(document.querySelectorAll('[data-php-tool] button')).find((b) => /PHP 8\.6/.test(b.innerText))?.click());
        await waitFor(page, () => !!document.querySelector('[data-php-tool-result]'), 8000).catch(() => {});
        await pause(500);
        await snap(page, 'tool-php-version-restored');
        await closePage(page);
        await clean();
    };

    // --- Review corrections (alpha.104 review) -------------------------------------
    scenarios.y27review = async () => {
        const toolsOn84 = { '/api/v1/service/removal-impact': { status: 200, body: versionImpact(sim({ bound_tools: ['phppgadmin', 'webmail'], refuses: true })) } };
        await reset(); await clean();
        await serve({ ...toolsOn84, '/api/v1/service/uninstall': { status: 409, body: {
            error: "the package manager's answer changed between the check and the removal; nothing was removed", code: 'REMOVAL_PLAN_CHANGED', moved: ['phppgadmin', 'webmail'], moved_to: '8.6',
        } } });
        let page = await open('/services', rowsReady);
        await openVersions(page); await pause(900);
        await openVersionRemoval(page, '8.4');
        await waitFor(page, () => !!document.querySelector('[data-version-move-remove]'), 8000).catch(() => {});
        await page.evaluate(() => document.querySelector('[data-version-move-remove]')?.click());
        await waitFor(page, () => !!document.querySelector('[data-version-move-failure]'), 8000).catch(() => {});
        await pause(600);
        await snap(page, 'review-moved-then-refused');
        await closePage(page);

        await reset(); await clean();
        await serve({ ...toolsOn84, '/api/v1/service/php-tools': { status: 502, body: { error: 'the Agent did not answer', code: 'AGENT_UNAVAILABLE' } } });
        page = await open('/services', rowsReady);
        await openVersions(page); await pause(900);
        // Without the usage read the rows have no usage marker: open 8.4's by its label.
        await page.evaluate(() => {
            for (const li of document.querySelectorAll('main li li, main li')) {
                if (!/\b8\.4\b/.test((li.innerText || '').split('\n')[0] || '')) continue;
                const trash = Array.from(li.querySelectorAll('button[title]')).find((b) => /Remove version|Sürümü kaldır|kaldır/i.test(b.getAttribute('title')));
                if (trash) { trash.click(); return; }
            }
        });
        await waitFor(page, () => !!document.querySelector('[data-removal-impact="known"]'), 8000).catch(() => {});
        await pause(500);
        await snap(page, 'review-versions-unknown');
        await closePage(page);

        await reset(); await clean();
        await serve({ '/api/v1/service/removal-impact': { status: 200, body: versionImpact(sim({ state: 'busy', remove: [], bound_tools: ['phppgadmin', 'webmail'], refuses: true })) } });
        page = await open('/services', rowsReady);
        await openVersions(page); await pause(900);
        await openVersionRemoval(page, '8.4');
        await waitFor(page, () => !!document.querySelector('[data-removal-impact="known"]'), 8000).catch(() => {});
        await pause(400);
        await snap(page, 'review-check-busy');
        await closePage(page);

        const lacking = SERVICES.map((item) => item.id === 'roundcube'
            ? { ...item, php_binding: { ...item.php_binding, extensions_present: false } } : item);
        await reset(); await clean();
        await serve({ '/api/v1/managed-services': { status: 200, body: SCAN(lacking) }, '/api/v1/managed-services/scan': { status: 200, body: SCAN(lacking) } });
        page = await open('/services', rowsReady);
        await whole(page, 'review-row-extensions');
        await closePage(page);

        const cardTools = { ...PHP_TOOLS, tools: PHP_TOOLS.tools.map((item) => item.tool === 'phppgadmin' ? { ...item, extensions_present: false } : item) };
        await reset(); await clean();
        await serve({ '/api/v1/service/php-tools': { status: 200, body: cardTools }, '/api/v1/service/php-tools/bind': { status: 409, body: {
            error: 'phpPgAdmin did not work on PHP 8.6 (PHP 8.6 lacks pgsql for phpPgAdmin); it was put back on PHP 8.4', code: 'PHP_TOOL_PROBE_FAILED',
            detail: 'PHP 8.6 lacks pgsql for phpPgAdmin', restored: true, previous: '8.4', cause: { kind: 'missing_extensions', version: '8.6', extensions: ['pgsql'] },
        } } });
        page = await open('/services/phppgadmin', () => !!document.querySelector('[data-php-tool-select]'));
        await whole(page, 'review-tool-extensions');
        await page.evaluate(() => Array.from(document.querySelectorAll('[data-php-tool] button')).find((b) => /PHP 8\.6/.test(b.innerText))?.click());
        await waitFor(page, () => !!document.querySelector('[data-php-tool-result]'), 8000).catch(() => {});
        await pause(500);
        await snap(page, 'review-tool-cause');
        await closePage(page);
        await clean();
    };
}
