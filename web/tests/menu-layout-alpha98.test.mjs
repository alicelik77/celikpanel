// v0.1.0-alpha.98 (D-037, owner 2026-10-21/22): "her işlemin yeri doğru
// olmalı ... herkes aradığını kolayca bulsun." Every page is where its name
// says it is. These tests pin the approved layout: the rail per role, the
// personal menu, Settings' remaining sections, the replace-redirect of every
// old /settings?section= address, and that no in-product link (web or a Go
// guidance action) points at a route or a Settings section that is not there.
//
// Her sayfa adının söylediği yerdedir. Bu testler onaylı düzeni sabitler:
// role göre ray, kişisel menü, Ayarlar'ın kalan bölümleri, eski adreslerin
// yönlendirmesi ve hiçbir bağlantının olmayan bir yere gitmemesi.
import assert from 'node:assert/strict';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { createRequire } from 'node:module';
import { join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import test from 'node:test';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import ts from 'typescript';
import { remoteURL } from './fixtures/shared-layer.mjs';
import { en } from '../src/i18n/en.ts';
import { tr } from '../src/i18n/tr.ts';
import { englishCatalogue, turkishCatalogue } from './locale-catalogue.mjs';

globalThis.document ??= Object.assign(new EventTarget(), { visibilityState: 'visible' });
globalThis.window ??= { setTimeout: (fn, ms) => globalThis.setTimeout(fn, ms), clearTimeout: (id) => globalThis.clearTimeout(id) };

const require = createRequire(import.meta.url);
const reactURL = pathToFileURL(require.resolve('react')).href;
const jsxRuntimeURL = pathToFileURL(require.resolve('react/jsx-runtime')).href;
const moduleURL = (source) => 'data:text/javascript;base64,' + Buffer.from(source).toString('base64');
const read = (relative) => readFileSync(new URL(relative, import.meta.url), 'utf8');
function compileURL(relativePath, imports = {}) {
    const compiled = ts.transpileModule(read(relativePath), {
        compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020, jsx: ts.JsxEmit.ReactJSX },
    }).outputText;
    return moduleURL(compiled.replace(/from ['"]([^'"]+)['"]/g, (_match, path) => {
        const target = path === 'react' ? reactURL : path === 'react/jsx-runtime' ? jsxRuntimeURL : imports[path];
        assert.ok(target, `unmapped import in ${relativePath}: ${path}`);
        return `from '${target}'`;
    }));
}

const iconStub = moduleURL(`
    import React from '${reactURL}';
    const Icon = (name) => (props) => React.createElement('i', { ...props, 'data-icon': name });
    ${['LayoutDashboard', 'Globe', 'Database', 'Server', 'Users', 'Settings', 'DownloadCloud', 'ScrollText', 'Package', 'Shield',
        'Activity', 'Network', 'ShieldCheck', 'Wrench', 'LogOut', 'ChevronDown', 'Menu', 'KeyRound', 'UserCheck']
        .map((name) => `export const ${name} = Icon('${name}');`).join('\n')}
`);
const navURL = compileURL('../src/nav.ts', { 'lucide-react': iconStub });
const nav = await import(navURL);

const ids = (items) => items.map((item) => item.id);
const rail = (role, access) => nav.navItemsForRole(role, access).filter((item) => item.group !== 'personal');
const byGroup = (role, access) => Object.fromEntries(nav.navGroups.map((group) => [group.id, ids(rail(role, access).filter((item) => item.group === group.id))]));

// --- The rail, per role -------------------------------------------------------------

test('the administrator rail is the approved layout, in order', () => {
    assert.deepEqual(byGroup('admin'), {
        main: ['dashboard'],
        hosting: ['domains', 'databases', 'users', 'addons', 'vpn'],
        server: ['services', 'dns', 'monitoring', 'security', 'maintenance', 'import', 'audit', 'settings'],
    });
    const labels = Object.fromEntries(nav.navItems.map((item) => [item.id, item.labelKey]));
    assert.equal(labels.dns, 'nav.dnsPeers');
    assert.equal(labels.security, 'nav.security');
    assert.equal(labels.maintenance, 'nav.maintenance');
    assert.equal(labels.account, 'nav.account');
    const paths = Object.fromEntries(nav.navItems.map((item) => [item.id, item.path]));
    assert.deepEqual([paths.dns, paths.security, paths.maintenance, paths.account], ['/dns-infrastructure', '/security', '/maintenance', '/account']);
    // The personal group is never drawn in the rail.
    assert.ok(!nav.navGroups.some((group) => group.id === 'personal'));
});

test('resellers, customers and additional users see what they saw before, without an empty Settings', () => {
    assert.deepEqual(ids(rail('reseller')), ['dashboard', 'domains', 'databases', 'users', 'addons', 'vpn']);
    assert.deepEqual(ids(rail('customer')), ['dashboard', 'domains', 'databases', 'vpn']);
    assert.deepEqual(ids(rail('customer', { accountType: 'account', teamMembers: true })), ['dashboard', 'domains', 'databases', 'users', 'vpn']);
    assert.deepEqual(ids(rail('additional_user')), ['dashboard', 'domains']);
    for (const role of ['reseller', 'customer', 'additional_user']) {
        assert.ok(!ids(rail(role)).includes('settings'), `${role} must not see an empty Settings`);
    }
});

test('each moved page keeps the roles it had inside Settings', () => {
    for (const role of ['admin', 'reseller', 'customer']) assert.equal(nav.canAccessPath(role, '/account'), true, role);
    assert.equal(nav.canAccessPath('additional_user', '/account'), false);
    for (const path of ['/security', '/maintenance', '/dns-infrastructure', '/settings']) {
        assert.equal(nav.canAccessPath('admin', path), true, path);
        for (const role of ['reseller', 'customer', 'additional_user']) assert.equal(nav.canAccessPath(role, path), false, `${role} ${path}`);
    }
});

// --- The old addresses --------------------------------------------------------------

test('every old Settings section address is replaced by its new page', () => {
    const moved = { account: '/account', security: '/security', dns: '/dns-infrastructure', 'system-databases': '/maintenance' };
    for (const [section, target] of Object.entries(moved)) {
        assert.equal(nav.settingsRedirect(`?section=${section}`, 'admin'), target, section);
    }
    for (const section of ['panel', 'license', 'updates', 'setup', 'nonsense', '']) {
        assert.equal(nav.settingsRedirect(section ? `?section=${section}` : '', 'admin'), null, section);
    }
    // A reseller or a customer had only their account in Settings.
    for (const role of ['reseller', 'customer']) {
        for (const search of ['', '?section=account', '?section=updates', '?section=dns']) {
            assert.equal(nav.settingsRedirect(search, role), '/account', `${role} ${search}`);
        }
    }
    // An additional user never had Settings; the route guard still sends them home.
    assert.equal(nav.settingsRedirect('?section=account', 'additional_user'), null);

    const app = read('../src/App.tsx');
    const route = app.slice(app.indexOf('function SettingsRoute()'), app.indexOf('function AppRoutes()'));
    assert.match(route, /const target = settingsRedirect\(location\.search, role\);/);
    assert.match(route, /if \(target\) return <Navigate to=\{target\} replace \/>;/);
    assert.match(app, /<Route path="\/settings" element=\{<SettingsRoute \/>\} \/>/);
});

// --- Every in-product link resolves -------------------------------------------------

const srcDir = fileURLToPath(new URL('../src/', import.meta.url));
const goDir = fileURLToPath(new URL('../../cmd/panel/', import.meta.url));
function walk(dir, keep) {
    return readdirSync(dir).flatMap((name) => {
        const full = join(dir, name);
        return statSync(full).isDirectory() ? walk(full, keep) : keep(full) ? [full] : [];
    });
}
const appSource = read('../src/App.tsx');
const routes = [...appSource.matchAll(/<Route path="([^"]+)"/g)].map((match) => match[1]).filter((path) => path !== '*');
const settingsSource = read('../src/components/Settings.tsx');
const settingsSections = settingsSource.match(/type SettingsSectionID = ([^;]+);/)[1].split('|').map((part) => part.trim().replace(/'/g, ''));

// Addresses the application answers before its routes: the sign-in and the
// license activation screens (App.tsx, LicenseOnboarding).
const shellAddresses = new Set(['/login', '/activate']);
function routeExists(pathname) {
    if (shellAddresses.has(pathname)) return true;
    const segments = pathname.replace(/\/$/, '').split('/');
    return routes.some((route) => {
        const parts = route.replace(/\/$/, '').split('/');
        return parts.length === segments.length && parts.every((part, index) => part.startsWith(':') || part === segments[index]);
    });
}
function checkLink(link, where) {
    const [pathAndQuery] = link.split('#');
    const [pathname, query = ''] = pathAndQuery.split('?');
    assert.ok(routeExists(pathname), `${where}: ${link} is not a route`);
    if (pathname === '/settings') {
        const section = new URLSearchParams(query).get('section');
        if (section !== null) assert.ok(settingsSections.includes(section), `${where}: ${link} names a Settings section that is not there`);
    }
}

test('every in-product link in the web sources resolves to a route and an existing Settings section', () => {
    const files = walk(srcDir, (file) => /\.(tsx?|mjs)$/.test(file));
    const found = [];
    const patterns = [
        /\bto=["'](\/[^"'{}\s]*)["']/g,
        /\bto=\{["'`](\/[^"'`{}\s]*)["'`]\}/g,
        /\bto:\s*["'](\/[^"']*)["']/g,
        /\bnavigate\(["'](\/[^"'`]*)["']/g,
        /["'](\/settings\?section=[^"']*)["']/g,
        /["'](\/(?:account|security|maintenance|dns-infrastructure)(?:[?#][^"']*)?)["']/g,
    ];
    for (const file of files) {
        const text = readFileSync(file, 'utf8');
        for (const pattern of patterns) {
            for (const match of text.matchAll(pattern)) {
                if (match[1].includes('${')) continue;
                found.push([match[1], file.slice(srcDir.length)]);
            }
        }
    }
    // The checks found the links that moved, so the scan is looking where they are.
    const links = new Set(found.map(([link]) => link));
    for (const expected of ['/dns-infrastructure', '/account', '/security', '/security#firewall', '/settings?section=updates']) {
        assert.ok(links.has(expected), `the scan did not see ${expected}`);
    }
    for (const [link, where] of found) checkLink(link, where);
});

test('every Go guidance action that names a panel page resolves too', () => {
    const files = walk(goDir, (file) => file.endsWith('.go') && !file.endsWith('_test.go'));
    let checked = 0;
    for (const file of files) {
        const text = readFileSync(file, 'utf8');
        for (const match of text.matchAll(/"(\/(?:settings|account|security|maintenance|dns-infrastructure)(?:\?section=[a-z-]+)?)"/g)) {
            checkLink(match[1], file.slice(goDir.length));
            checked += 1;
        }
    }
    assert.ok(checked >= 10, `expected the DNS guidance actions, found ${checked}`);
});

test('the recovery shell knows every new address', () => {
    const worker = read('../scripts/recovery-worker.template.js');
    for (const path of ['/account', '/security', '/maintenance', '/dns-infrastructure', '/settings']) {
        assert.ok(worker.includes(`'${path}'`), `recovery worker misses ${path}`);
    }
});

test('the four new labels exist in English and Turkish', () => {
    const want = {
        'nav.dnsPeers': ['DNS and peer servers', 'DNS ve eş sunucular'],
        'nav.security': ['Security', 'Güvenlik'],
        'nav.maintenance': ['Maintenance', 'Bakım'],
        'nav.account': ['My account', 'Hesabım'],
    };
    for (const [key, [english, turkish]] of Object.entries(want)) {
        assert.equal(en[key], english, key);
        assert.equal(tr[key], turkish, key);
    }
    for (const key of ['securityPage.subtitle', 'dnsPeers.subtitle', 'account.subtitle', 'account.password.title']) {
        assert.ok(englishCatalogue.includes(`'${key}'`), key);
        assert.ok(turkishCatalogue.includes(`'${key}'`), key);
    }
});

test('each new page has a header that says what it is for and moved content, never an empty page', () => {
    const pages = {
        SecurityPage: ['securityPage.subtitle', '<FirewallBar', 'place="security"', '<Fail2banStatus />', '<SecurityAuditCard />', 'id="firewall"'],
        // The card's own heading and subtitle say what the page is for (alpha.98 review: one subtitle, not two).
        MaintenancePage: ['<SystemSQLiteManager />'],
        DNSPeersPage: ['dnsPeers.subtitle', '<DNSServerSettings />'],
        AccountPage: ['account.subtitle', '<PasswordPanel />', '<TwoFactorPanel />', 'ChangePasswordModal'],
    };
    for (const [page, needles] of Object.entries(pages)) {
        const source = read(`../src/components/${page}.tsx`);
        assert.match(source, /<PageHeader title=\{t\('nav\.[a-zA-Z]+'\)\}/, page);
        for (const needle of needles) assert.ok(source.includes(needle), `${page} misses ${needle}`);
    }
    const app = read('../src/App.tsx');
    for (const [path, page] of [['/account', 'AccountPage'], ['/security', 'SecurityPage'], ['/maintenance', 'MaintenancePage'], ['/dns-infrastructure', 'DNSPeersPage']]) {
        assert.ok(app.includes(`<Route path="${path}" element={<PageWithLayout><${page} /></PageWithLayout>} />`), path);
    }
    // The setup gate keeps every page that was under /settings reachable.
    const setup = read('../src/lib/serverSetup.ts');
    for (const path of ['/settings', '/account', '/security', '/dns-infrastructure', '/maintenance']) assert.ok(setup.includes(`'${path}'`), path);
});

// --- The personal menu, mounted -----------------------------------------------------

const shellStub = moduleURL(`
    import React from '${reactURL}';
    export const BrandMark = () => null;
    export const BuildCommit = ({ commit }) => 'build ' + commit.slice(0, 12);
    export const ThemeSwitcher = () => null, SkinSwitcher = () => null;
    export const LanguageSwitcher = () => React.createElement('span', { 'data-language-switcher': '' });
    export const ChangePasswordModal = () => React.createElement('aside', { 'data-password-dialog': '' });
    export const useAuth = () => globalThis.menuFixture.auth;
    export const useI18n = () => ({ t: (key) => key, locale: 'en', screensReady: true, screensFailed: false });
    export const Link = ({ to, children, ...props }) => React.createElement('a', { ...props, href: to }, children);
    export const DesktopPageHeaderTargetContext = React.createContext(null);
    export const api = { getServices: async () => [], logout: async () => {} };
    export const publishComponentCensus = () => {}, useComponentCensus = () => null;
    export const availablePanelUpdate = () => null, wholeVersion = (text) => text;
    export const usePanelUpdateDiscovery = () => ({ status: 'idle', value: null, readAt: 0 });
`);
const layoutURL = compileURL('../src/components/Layout.tsx', {
    ...Object.fromEntries(['./BrandMark', './BuildCommit', '../lib/api', '../auth/AuthContext', '../i18n', '../lib/componentCensus',
        './ThemeSwitcher', './SkinSwitcher', './LanguageSwitcher', './ChangePasswordModal', './pageHeaderSlot', '../router',
        '../lib/panelUpdateDiscovery'].map((path) => [path, shellStub])),
    'lucide-react': iconStub,
    '../nav': navURL,
    '../lib/remote': remoteURL,
    '../lib/infrastructureZones': compileURL('../src/lib/infrastructureZones.ts'),
});
const { Layout } = await import(layoutURL);
const flush = async () => { for (let i = 0; i < 8; i += 1) await Promise.resolve(); };

async function withShell(role, inspect, access = {}) {
    const previousFetch = globalThis.fetch;
    const listeners = new Map();
    const previousAdd = document.addEventListener;
    const previousRemove = document.removeEventListener;
    document.addEventListener = (type, fn) => listeners.set(type, fn);
    document.removeEventListener = (type) => listeners.delete(type);
    globalThis.menuFixture = { auth: { role, user: { username: role, ...access }, logout() {} } };
    globalThis.fetch = async (input) => {
        const path = String(input);
        if (path === '/api/v1/panel/version') return Response.json({ version: 'v0.1.0-alpha.98', commit: 'a'.repeat(40), agent_commit: 'a'.repeat(40), agent_matches: true, hostname: 'h', ipv4: '192.0.2.1' });
        if (path === '/api/v1/domains') return Response.json([]);
        if (path === '/api/v1/dns/infrastructure-zones') return Response.json({ zones: [] });
        throw new Error('unexpected request: ' + path);
    };
    let renderer;
    try {
        await act(async () => {
            renderer = TestRenderer.create(React.createElement(Layout, { currentPage: 'dashboard', onPageChange: () => {} }, React.createElement('h1', null, 'Page')));
            await flush();
        });
        await inspect(renderer, listeners);
    } finally {
        if (renderer) await act(async () => renderer.unmount());
        globalThis.fetch = previousFetch;
        document.addEventListener = previousAdd;
        document.removeEventListener = previousRemove;
    }
}
const textOf = (node) => (typeof node === 'string' ? node : node.children.map(textOf).join(' '));
const menuButton = (renderer) => renderer.root.find((node) => node.type === 'button' && node.props['aria-expanded'] !== undefined);
const menuItems = (renderer) => renderer.root.find((node) => node.props.id === 'personal-menu')
    .findAll((node) => (node.type === 'a' || node.type === 'button'));
const railLabels = (renderer) => renderer.root.findAll((node) => node.type === 'button' && node.props.onClick && node.props['aria-expanded'] === undefined)
    .map((node) => node.findAll((child) => child.type === 'span' && typeof child.children[0] === 'string')[0]?.children[0]).filter(Boolean);

test('the personal menu opens with My account first, then Sign out, and closes on Escape', async () => {
    for (const role of ['admin', 'reseller', 'customer']) {
        await withShell(role, async (renderer, listeners) => {
            const button = menuButton(renderer);
            assert.equal(button.props['aria-expanded'], false);
            assert.equal(button.props['aria-controls'], undefined, 'aria-controls names the panel only while it exists');
            await act(async () => button.props.onClick());
            assert.equal(menuButton(renderer).props['aria-expanded'], true);
            assert.equal(menuButton(renderer).props['aria-controls'], 'personal-menu');
            const items = menuItems(renderer);
            assert.equal(items[0].type, 'a', role);
            assert.equal(items[0].props.href, '/account', role);
            assert.ok(textOf(items[0]).includes('nav.account'), role);
            assert.equal(items.at(-1).props['data-personal-menu-item'], 'logout', role);
            // The password change lives on My account now; the menu does not repeat it.
            assert.ok(!items.some((item) => textOf(item).includes('profile.changePassword')), role);
            await act(async () => listeners.get('keydown')({ key: 'Escape' }));
            assert.equal(renderer.root.findAll((node) => node.props.id === 'personal-menu').length, 0, role);
            // Focus leaving the menu (Tab past its last item) closes it too.
            await act(async () => menuButton(renderer).props.onClick());
            const container = menuButton(renderer).parent;
            const inside = { nodeType: 1 };
            await act(async () => container.props.onBlur({ currentTarget: { contains: (node) => node === inside }, relatedTarget: inside }));
            assert.equal(renderer.root.findAll((node) => node.props.id === 'personal-menu').length, 1, `${role}: focus moving inside keeps it open`);
            await act(async () => container.props.onBlur({ currentTarget: { contains: () => false }, relatedTarget: null }));
            assert.equal(renderer.root.findAll((node) => node.props.id === 'personal-menu').length, 0, `${role}: focus leaving closes it`);
        });
    }
});

test('an additional user, who has no My account, keeps the password change in the menu', async () => {
    await withShell('additional_user', async (renderer) => {
        await act(async () => menuButton(renderer).props.onClick());
        const items = menuItems(renderer);
        assert.ok(!items.some((item) => item.props.href === '/account'));
        assert.ok(items.some((item) => textOf(item).includes('profile.changePassword')));
    });
});

test('the drawn rail matches the approved layout, and only an administrator sees Settings', async () => {
    await withShell('admin', async (renderer) => {
        assert.deepEqual(railLabels(renderer), ['nav.dashboard', 'nav.domains', 'nav.databases', 'nav.users', 'nav.addons', 'nav.vpn',
            'nav.services', 'nav.dnsPeers', 'nav.monitoring', 'nav.security', 'nav.maintenance', 'nav.import', 'nav.audit', 'nav.settings']);
    });
    for (const role of ['reseller', 'customer']) {
        await withShell(role, async (renderer) => {
            const labels = railLabels(renderer);
            assert.ok(!labels.includes('nav.settings'), role);
            assert.ok(!labels.includes('nav.account'), `${role}: My account belongs to the personal menu`);
        });
    }
});

// --- Fail2ban on the Security page --------------------------------------------------

test('the Security page says Fail2ban\'s state only as the stored record says it', async () => {
    const source = read('../src/components/SecurityPage.tsx');
    const start = source.indexOf('type Fail2banState');
    const end = source.indexOf('function Fail2banStatus()');
    const running = read('../src/lib/serviceStatus.ts');
    const compiled = ts.transpileModule(running + String.fromCharCode(10) + source.slice(start, end), {
        compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 },
    }).outputText;
    const { fail2banState } = await import(moduleURL(compiled));
    const record = (fields) => [{ id: 'nginx', is_installed: true, status: 'running' }, { id: 'fail2ban', ...fields }];
    assert.equal(fail2banState(record({ is_installed: true, status: 'running' })), 'running');
    // The scan's own dialect is systemd's (browser batch 23 found the page
    // calling a running Fail2ban stopped before this was read like this).
    assert.equal(fail2banState(record({ is_installed: true, status: 'active (running)' })), 'running');
    assert.equal(fail2banState(record({ is_installed: true, status: 'inactive (dead)' })), 'notRunning');
    assert.equal(fail2banState(record({ is_installed: true, status: 'stopped' })), 'notRunning');
    assert.equal(fail2banState(record({ is_installed: true, status: 'failed' })), 'notRunning');
    assert.equal(fail2banState(record({ is_installed: true, status: 'unknown' })), 'installedUnknown');
    assert.equal(fail2banState(record({ is_installed: false, status: 'unknown' })), 'notInstalled');
    // A record nobody took is not "not installed".
    assert.equal(fail2banState(record({ is_installed: null, status: 'unknown' })), 'notChecked');
    // A known snapshot without a Fail2ban record: not installed (it is a known answer).
    assert.equal(fail2banState([]), 'notInstalled');
    assert.equal(fail2banState(record({ is_installed: true, status: 'active (exited)' })), 'notRunning');
    for (const state of ['running', 'installedUnknown', 'notRunning', 'notInstalled', 'notChecked', 'checking', 'unknown', 'manage', 'openComponents']) {
        assert.ok(englishCatalogue.includes(`'securityPage.f2b.${state}'`), state);
        assert.ok(turkishCatalogue.includes(`'securityPage.f2b.${state}'`), state);
    }
});
