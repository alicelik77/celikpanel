// v0.1.0-alpha.100 (owner 2026-10-22): Server setup leaves Settings for its own
// page under Server, directly under Components, as "Server setup" / "Sunucu
// kurulumu". It shows what Settings → Server setup showed (the purpose, the
// state, the extend button and the wizard link); the Dashboard's "Server
// purpose" card opens it; /settings?section=setup is replaced by it; every
// in-product link points at it; Settings keeps three sections. Roles do not
// change: administrator only. The wizard itself stays at /setup.
//
// Sunucu kurulumu Ayarlar'dan çıkıp Sunucu altında, Bileşenler'in hemen
// altında kendi sayfası olur. Bu testler yerini, eski adresin yönlendirmesini,
// Pano kartının bağlantısını, Ayarlar'ın üç bölümünü ve rol korumasını sabitler.
import assert from 'node:assert/strict';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { createRequire } from 'node:module';
import { join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import test from 'node:test';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import ts from 'typescript';
import { en } from '../src/i18n/en.ts';
import { tr } from '../src/i18n/tr.ts';
import { englishCatalogue, turkishCatalogue } from './locale-catalogue.mjs';

const require = createRequire(import.meta.url);
const reactURL = pathToFileURL(require.resolve('react')).href;
const jsxRuntimeURL = pathToFileURL(require.resolve('react/jsx-runtime')).href;
const moduleURL = (source) => 'data:text/javascript;base64,' + Buffer.from(source).toString('base64');
const read = (relative) => readFileSync(new URL(relative, import.meta.url), 'utf8');
function compileURL(relativePath, imports = {}, fallback) {
    const compiled = ts.transpileModule(read(relativePath), {
        compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020, jsx: ts.JsxEmit.ReactJSX },
    }).outputText;
    return moduleURL(compiled.replace(/from ['"]([^'"]+)['"]/g, (_match, path) => {
        const target = path === 'react' ? reactURL : path === 'react/jsx-runtime' ? jsxRuntimeURL : imports[path] ?? fallback;
        assert.ok(target, `unmapped import in ${relativePath}: ${path}`);
        return `from '${target}'`;
    }));
}

const iconStub = moduleURL(`
    import React from '${reactURL}';
    const Icon = (name) => (props) => React.createElement('i', { ...props, 'data-icon': name });
    ${['LayoutDashboard', 'Globe', 'Database', 'Server', 'Users', 'Settings', 'DownloadCloud', 'ScrollText', 'Package', 'Shield',
        'Activity', 'Network', 'ShieldCheck', 'Wrench', 'UserCheck', 'ServerCog']
        .map((name) => `export const ${name} = Icon('${name}');`).join('\n')}
`);
const nav = await import(compileURL('../src/nav.ts', { 'lucide-react': iconStub }));
const setupLibURL = compileURL('../src/lib/serverSetup.ts');
const setupLib = await import(setupLibURL);

const ids = (items) => items.map((item) => item.id);
const rail = (role, access) => nav.navItemsForRole(role, access).filter((item) => item.group !== 'personal');

// --- Its place in the rail ----------------------------------------------------------

test('Server setup is its own Server item, directly under Components, for an administrator only', () => {
    const server = ids(rail('admin').filter((item) => item.group === 'server'));
    assert.deepEqual(server.slice(0, 2), ['services', 'serverSetup']);
    assert.equal(server.indexOf('serverSetup'), server.indexOf('services') + 1);
    const item = nav.navItems.find((entry) => entry.id === 'serverSetup');
    assert.equal(item.path, '/server-setup');
    assert.equal(item.labelKey, 'nav.serverSetup');
    assert.equal(item.group, 'server');
    assert.deepEqual(item.roles, ['admin']);
    assert.equal(item.icon({}).props['data-icon'], 'ServerCog');
    for (const role of ['reseller', 'customer', 'additional_user']) {
        assert.ok(!ids(nav.navItemsForRole(role)).includes('serverSetup'), role);
    }
    assert.ok(!ids(nav.navItemsForRole('customer', { accountType: 'account', teamMembers: true })).includes('serverSetup'));
});

test('the new address is guarded like the Settings section it was: administrator only', () => {
    assert.equal(nav.canAccessPath('admin', '/server-setup'), true);
    assert.equal(nav.canAccessPath('admin', '/server-setup/'), true);
    for (const role of ['reseller', 'customer', 'additional_user']) {
        assert.equal(nav.canAccessPath(role, '/server-setup'), false, role);
    }
    assert.equal(nav.canAccessPath('customer', '/server-setup', { accountType: 'account', teamMembers: true }), false);
    // It is not mistaken for Components (/services) by a prefix.
    const match = [...nav.navItems].sort((a, b) => b.path.length - a.path.length)
        .find((entry) => '/server-setup' === entry.path || '/server-setup'.startsWith(`${entry.path}/`));
    assert.equal(match.id, 'serverSetup');
});

test('the route answers with its page; the wizard keeps /setup', () => {
    const app = read('../src/App.tsx');
    assert.ok(app.includes(`const ServerSetupPage = lazyNamed(() => import('./components/ServerSetupPage'), 'ServerSetupPage');`),
        'the page is fetched with its route, not on the boot path');
    assert.ok(app.includes('<Route path="/server-setup" element={<PageWithLayout><ServerSetupPage /></PageWithLayout>} />'));
    assert.ok(app.includes('<Route path="/setup" element={<ServerSetup />} />'));
    assert.ok(!/^import \{[^}]*ServerSetupPage/m.test(app), 'the page must stay lazy');
});

test('/settings?section=setup is replaced by the new page; Settings keeps its three sections', () => {
    assert.equal(nav.movedSettingsSections.setup, '/server-setup');
    assert.equal(nav.settingsRedirect('?section=setup', 'admin'), '/server-setup');
    assert.equal(nav.settingsRedirect('?section=setup&from=guide', 'admin'), '/server-setup');
    for (const section of ['panel', 'license', 'updates']) {
        assert.equal(nav.settingsRedirect(`?section=${section}`, 'admin'), null, section);
    }
    // A reseller or a customer still goes to My account, an additional user nowhere.
    for (const role of ['reseller', 'customer']) assert.equal(nav.settingsRedirect('?section=setup', role), '/account', role);
    assert.equal(nav.settingsRedirect('?section=setup', 'additional_user'), null);

    // The redirect is a replace: Back does not return to the old address.
    const app = read('../src/App.tsx');
    const route = app.slice(app.indexOf('function SettingsRoute()'), app.indexOf('function AppRoutes()'));
    assert.match(route, /if \(target\) return <Navigate to=\{target\} replace \/>;/);

    const settings = read('../src/components/Settings.tsx');
    assert.match(settings, /type SettingsSectionID = 'panel' \| 'license' \| 'updates';/);
    const adminSections = settings.slice(settings.indexOf("const sections: SettingsSection[] = role === 'admin'"), settings.indexOf(': [];'));
    assert.deepEqual([...adminSections.matchAll(/id:\s*'([a-z-]+)'\s+as const/g)].map((match) => match[1]), ['panel', 'license', 'updates']);
    assert.ok(!settings.includes('ServerSetupSettings') && !settings.includes('settings-setup-panel'));
});

test('the setup gate and the recovery shell know the new address', () => {
    assert.equal(setupLib.isSetupRecoveryPath('/server-setup'), true);
    // A setup that needs the owner's choice does not take them away from the
    // page that offers it (it did not take them away from Settings either).
    const undecided = { version: 1, revision: 1, origin: 'fresh', status: 'new', required: true, guidance: 'undecided', draft: { purpose: 'web' }, checks: [] };
    assert.equal(setupLib.shouldOpenServerSetup(undecided, '/server-setup'), false);
    assert.equal(setupLib.shouldOpenServerSetup(undecided, '/domains'), true);
    const worker = read('../scripts/recovery-worker.template.js');
    for (const path of ['/server-setup', '/setup', '/settings']) assert.ok(worker.includes(`'${path}'`), path);
});

// --- Every link to the old section points at the new page ----------------------------

const srcDir = fileURLToPath(new URL('../src/', import.meta.url));
const goDir = fileURLToPath(new URL('../../cmd/panel/', import.meta.url));
function walk(dir, keep) {
    return readdirSync(dir).flatMap((name) => {
        const full = join(dir, name);
        return statSync(full).isDirectory() ? walk(full, keep) : keep(full) ? [full] : [];
    });
}

test('no in-product link names the old section; the zone page and the Panel\'s refusal open the new page', () => {
    for (const file of walk(srcDir, (path) => /\.(tsx?|mjs)$/.test(path))) {
        assert.ok(!readFileSync(file, 'utf8').includes('section=setup'), `${file.slice(srcDir.length)} still links /settings?section=setup`);
    }
    for (const file of walk(goDir, (path) => path.endsWith('.go') && !path.endsWith('_test.go'))) {
        assert.ok(!readFileSync(file, 'utf8').includes('section=setup'), `${file.slice(goDir.length)} still names /settings?section=setup`);
    }
    assert.ok(read('../src/components/InfrastructureZone.tsx').includes('<Link to="/server-setup"'));
    const refusal = read('../../cmd/panel/infrastructure_dns_zone.go');
    const writer = refusal.slice(refusal.indexOf('func writeInfrastructureDNSRecordManagedBySetup'));
    assert.match(writer.slice(0, writer.indexOf('\n}')), /"\/server-setup"\)/);
});

test('the new words exist in English and Turkish and no text sends the owner to Settings for setup', () => {
    assert.equal(en['nav.serverSetup'], 'Server setup');
    assert.equal(tr['nav.serverSetup'], 'Sunucu kurulumu');
    for (const key of ['setup.pageSubtitle', 'setup.dashboardOpenPage']) {
        assert.ok(englishCatalogue.includes(`"${key}"`), key);
        assert.ok(turkishCatalogue.includes(`"${key}"`), key);
    }
    assert.ok(!englishCatalogue.includes('Settings → Server setup'));
    assert.ok(!turkishCatalogue.includes('Ayarlar → Sunucu kurulumu'));
    assert.ok(englishCatalogue.includes('Server → Server setup'));
    assert.ok(turkishCatalogue.includes('Sunucu → Sunucu kurulumu'));
});

// --- Mounted: the page and the Dashboard card ----------------------------------------

const stub = moduleURL(`
    import React from '${reactURL}';
    export const useAuth = () => ({ role: globalThis.pageFixture.role, user: { username: 'owner' }, logout() {} });
    export const useI18n = () => ({ locale: 'en', t: (key) => key, screensReady: true, screensFailed: false });
    export const useLocation = () => ({ pathname: globalThis.pageFixture.path, search: '', hash: '' });
    export const Navigate = (props) => React.createElement('redirect', props);
    export const Link = ({ to, children, ...props }) => React.createElement('a', { ...props, href: to }, children);
    export const Layout = (props) => React.createElement('div', null, props.children);
    export const ChangePasswordModal = () => null;
    export const Button = (props) => React.createElement('button', props);
    export const Spinner = () => React.createElement('span', null, 'loading');
    export const PageHeader = (props) => React.createElement('header', { 'data-title': props.title, 'data-subtitle': props.subtitle, 'data-breadcrumb': (props.breadcrumb || []).join(' / ') });
    export const OSReleaseCard = () => null;
`);
const gateURL = compileURL('../src/components/ServerSetupGate.tsx', { '../lib/serverSetup': setupLibURL }, stub);
const choiceURL = compileURL('../src/components/ServerSetupChoice.tsx', { '../lib/serverSetup': setupLibURL, './ServerSetupGate': gateURL }, stub);
const pageURL = compileURL('../src/components/ServerSetupPage.tsx', { './ServerSetupChoice': choiceURL }, stub);
const { ServerSetupGate, ServerSetupDashboardNotice } = await import(gateURL);
const { ServerSetupPage } = await import(pageURL);

const draft = { purpose: 'web', panel_domain: 'panel.example.com', mail_hostname: '', dns_mode: 'external', remote_dns_connection_id: '', dns_engine: 'pdns', dns_role: 'primary',
    ns1: '', ns2: '', local_ip: '', peer_ip: '', peer_ns: '', node_version: '', database: 'mariadb' };
const snapshot = (overrides = {}) => ({ version: 1, revision: 3, origin: 'fresh', status: 'ready', required: false, guidance: 'guided', draft, checks: [], ...overrides });

async function mounted(path, body, child, inspect) {
    const previous = { fetch: globalThis.fetch, window: globalThis.window };
    globalThis.pageFixture = { role: 'admin', path };
    globalThis.window = { setTimeout, clearTimeout, location: { reload() {} } };
    const requests = [];
    globalThis.fetch = async (url, options) => {
        requests.push([String(url), options?.method ?? 'GET']);
        if (String(url) === '/api/v1/setup') return Response.json(body);
        throw new Error('unexpected request: ' + url);
    };
    let renderer;
    try {
        await act(async () => {
            renderer = TestRenderer.create(React.createElement(ServerSetupGate, null, React.createElement(child)));
            for (let i = 0; i < 8; i += 1) await Promise.resolve();
        });
        await inspect(renderer, requests);
    } finally {
        if (renderer) await act(async () => renderer.unmount());
        globalThis.fetch = previous.fetch;
        if (previous.window === undefined) delete globalThis.window; else globalThis.window = previous.window;
        delete globalThis.pageFixture;
    }
}
const textOf = (node) => (typeof node === 'string' ? node : node.children.map(textOf).join(' '));
const links = (renderer) => renderer.root.findAll((node) => node.type === 'a').map((node) => [node.props.href, textOf(node)]);

test('the page has a header saying what it is for and shows the purpose, the state and the wizard link', async () => {
    assert.ok(setupLib.decodeServerSetup(snapshot()), 'the fixture is a snapshot the Panel could send');
    await mounted('/server-setup', snapshot(), ServerSetupPage, async (renderer, requests) => {
        const header = renderer.root.findByType('header');
        assert.equal(header.props['data-title'], 'nav.serverSetup');
        assert.equal(header.props['data-subtitle'], 'setup.pageSubtitle');
        assert.equal(header.props['data-breadcrumb'], 'common.home / nav.serverSetup');
        const purpose = renderer.root.find((node) => node.props['data-setup-purpose'] !== undefined);
        assert.match(textOf(purpose), /setup\.dashboardReady\s*:\s+setup\.purpose\.web/);
        const shown = textOf(renderer.root.findByType('section'));
        assert.ok(shown.includes('setup.completeHelp') && shown.includes('setup.extend.help'), shown);
        assert.deepEqual(links(renderer), [['/setup', 'setup.extend.title']]);
        // Reading the page sends no write.
        assert.deepEqual(requests.filter(([, method]) => method !== 'GET'), []);
    });
    // A custom setup is named as one.
    await mounted('/server-setup', snapshot({ draft: { ...draft, customization: { components: ['nginx'] } } }), ServerSetupPage, async (renderer) => {
        assert.match(textOf(renderer.root.find((node) => node.props['data-setup-purpose'] !== undefined)), /setup\.purpose\.custom/);
    });
});

test('a setup still to be chosen stays on the page with the wizard link and the manual choice', async () => {
    const undecided = snapshot({ status: 'new', required: true, guidance: 'undecided' });
    await mounted('/server-setup', undecided, ServerSetupPage, async (renderer) => {
        assert.equal(renderer.root.findAllByType('redirect').length, 0, 'the gate took the owner away from Server setup');
        assert.deepEqual(links(renderer), [['/setup', 'setup.openWizard']]);
        assert.ok(renderer.root.findAllByType('button').some((button) => textOf(button).includes('setup.manual')));
        assert.equal(renderer.root.findAll((node) => node.props['data-setup-purpose'] !== undefined).length, 0);
    });
    // Control: elsewhere the same state still opens the wizard.
    await mounted('/domains', undecided, ServerSetupPage, async (renderer) => {
        assert.equal(renderer.root.findByType('redirect').props.to, '/setup');
    });
});

test('the Dashboard\'s Server purpose card opens Server setup; an unfinished setup still continues in the wizard', async () => {
    await mounted('/', snapshot(), ServerSetupDashboardNotice, async (renderer) => {
        assert.ok(textOf(renderer.root.findByType('section')).includes('setup.dashboardReady'));
        assert.deepEqual(links(renderer), [['/server-setup', 'setup.dashboardOpenPage']]);
        assert.equal(renderer.root.find((node) => node.type === 'a').props['data-setup-purpose-link'], '');
    });
    await mounted('/', snapshot({ status: 'draft', required: false }), ServerSetupDashboardNotice, async (renderer) => {
        assert.deepEqual(links(renderer), [['/setup', 'setup.resume']]);
    });
    const extension = snapshot({ status: 'draft', extension: true, extension_base: draft });
    await mounted('/', extension, ServerSetupDashboardNotice, async (renderer) => {
        assert.deepEqual(links(renderer).map(([href]) => href), ['/setup']);
    });
});
