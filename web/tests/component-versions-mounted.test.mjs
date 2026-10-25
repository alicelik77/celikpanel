// The installed package versions on the Components page, mounted (alpha.97,
// D-036). The page is drawn with the real shared layer and the real version
// reader from what the Panel answers: without a known package version there
// is no desktop version column (alpha.95's line under the description stays
// for the runtimes); with one, the column comes back, a row shows the short
// upstream form in mono with the full version and its source in the tooltip,
// packages that differ say "several versions", and an unknown or absent
// component says nothing, never a dash.
// Bileşenler sayfasında kurulu paket sürümleri: sütun yalnız bir sürüm
// bilindiğinde döner; kısa biçim satırda, tam biçim ipucunda; bilinmeyen susar.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { en as enShellHalf } from '../src/i18n/en.ts';
import { enErrorsScreens as enErrorsPart } from '../src/i18n/screens/errors/en.ts';
// The coded refusals only screens show are the screen part "errors" since v0.1.0-alpha.98.
const en = { ...enShellHalf, ...enErrorsPart };
import { enScreens } from './locale-catalogue.mjs';
import { trScreens } from './locale-catalogue.mjs';
import { tr as trShellHalf } from '../src/i18n/tr.ts';
import { trErrorsScreens as trErrorsPart } from '../src/i18n/screens/errors/tr.ts';
const tr = { ...trShellHalf, ...trErrorsPart };
import { enComponentsScreens } from '../src/i18n/screens/components/en.ts';
import { trComponentsScreens } from '../src/i18n/screens/components/tr.ts';
import { enComponentPagesScreens } from '../src/i18n/screens/componentPages/en.ts';
import { trComponentPagesScreens } from '../src/i18n/screens/componentPages/tr.ts';
// The components and component-page parts (split in alpha.106), as one.
const enComponents = { ...enComponentsScreens, ...enComponentPagesScreens };
const trComponents = { ...trComponentsScreens, ...trComponentPagesScreens };

// The page speaks English from the real catalogue, so a bundle is named by
// its words and never by a fallback.
const words = { ...en, ...enScreens };
const trWords = { ...tr, ...trScreens };
globalThis.bundleWords = words;
const say = (key, vars = {}) => Object.entries(vars).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), words[key] ?? key);
const nameOf = (id) => words[`services.component.name.${id}`] ?? id;

const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const icons = new Set(['AlertTriangle']);
for (const path of ['components/ServiceList.tsx', 'components/FirewallPanel.tsx', 'components/ReadyBundleStrip.tsx', 'components/LifeLine.tsx', 'lib/componentIcons.tsx']) {
  for (const match of source(path).matchAll(/import\s*\{([^}]+)\}\s*from 'lucide-react'/g)) {
    for (const item of match[1].split(',')) {
      const icon = item.replace(/\btype\b/, '').trim().split(/\s+as\s+/)[0];
      if (icon && icon !== 'LucideIcon') icons.add(icon);
    }
  }
}

const stub = dataModule(`
  import React from '${reactURL}';
  ${[...icons].map((name) => `export const ${name} = () => null;`).join('\n')}
  const fill = (text, vars) => Object.entries(vars || {}).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
  const i18n = { t: (key, vars) => (globalThis.bundleWords?.[key] !== undefined ? fill(globalThis.bundleWords[key], vars) : vars ? key + JSON.stringify(vars) : key), locale: 'en' };
  export const useI18n = () => i18n;
  export const useNavigate = () => (path, options) => { globalThis.bundleNavigated?.push(path); globalThis.bundleNavigateOptions?.push(options ?? null); return true; };
  export const useLocation = () => ({ hash: '', pathname: '/services', search: globalThis.bundleSearch ?? '' });
  export const useNavigationBlocker = () => {};
  export const Link = (props) => React.createElement('a', null, props.children);
  export const showToast = (...args) => globalThis.bundleToasts.push(args);
  export const PageHeader = () => null;
  export const ServiceActionNotice = () => null, isServiceActionOutcome = () => false;
  export const FirewallNoSSHAcknowledgement = () => null, readFirewallSSHReason = () => null, FirewallSSHReasonLine = () => null;
  export const useAuth = () => ({ role: 'admin', user: { username: 'admin', role: 'admin' } });
  export const useServerSetup = () => null;
  export const createPortal = (node) => node;
  export const scanRefusedBySetup = () => false;
`);
const shared = sharedLayer(stub);
const lib = (path, more = {}) => shared.compile(path, more);
const operationURL = lib('components/ComponentOperation.tsx');
// The operation tracker's stand-in: the page reads it as it reads the real
// one, and every install it is asked to start is recorded. The test moves the
// operation on, as the real tracker does after its poll and its fresh scan.
const trackerURL = dataModule(`
  import React from '${reactURL}';
  export * from '${operationURL}';
  const store = globalThis.bundleTracker = {
    listeners: new Set(),
    calls: [],
    state: null,
    set(next) { store.state = { ...store.state, ...next }; for (const listener of store.listeners) listener(); },
  };
  store.state = {
    operation: null, failure: null, catalogSnapshot: null, locked: false, checking: false,
    startInstall: async (request) => {
      store.calls.push(request);
      store.set({ operation: { id: 'op' + store.calls.length, service_id: request.serviceId, kind: 'service_install', status: 'running' }, locked: true });
      return true;
    },
  };
  export const useComponentOperation = () => React.useSyncExternalStore(
    (listener) => { store.listeners.add(listener); return () => store.listeners.delete(listener); },
    () => store.state,
  );
`);
const pieces = {
  '/lib/mailProfileText': lib('lib/mailProfileText.ts'),
  '/lib/componentName': lib('lib/componentName.ts'),
  '/lib/readyBundles': lib('lib/readyBundles.ts', { '/mailProfileText': lib('lib/mailProfileText.ts') }),
  '/lib/componentIcons': lib('lib/componentIcons.tsx'),
};
const { ServiceList } = await import(lib('components/ServiceList.tsx', {
  '/ComponentOperation': trackerURL,
  '/lib/serverSetup': lib('lib/serverSetup.ts'),
  '/WebmailRepairButton': lib('components/WebmailRepairButton.tsx'),
  '/ReadyBundleStrip': lib('components/ReadyBundleStrip.tsx', pieces),
  ...pieces,
}));
globalThis.window ??= globalThis;


// --- What the Panel answers (alpha.97) -----------------------------------------------
const component = (id, category, installed, extra = {}) => ({
  id, name: id, description: `${id} on this server`, icon: '', category,
  status: installed ? 'active (running)' : 'not_installed', is_installed: installed, versions: [], ...extra,
});
// Frankfurt-shaped (Debian 13) package versions, sample data.
const nginxVersion = { package_version: '1.26.3-3+deb13u1', package_version_short: '1.26.3', package_versions: [{ package: 'nginx', version: '1.26.3-3+deb13u1' }], package_version_read_at: '2026-10-21T08:00:00Z' };
const mariadbVersion = { package_version: '1:11.8.3-0+deb13u1', package_version_short: '11.8.3', package_versions: [{ package: 'mariadb-server', version: '1:11.8.3-0+deb13u1' }], package_version_read_at: '2026-10-21T08:00:00Z' };
const spamVersions = { package_versions_differ: true, package_versions: [{ package: 'spamassassin', version: '4.0.1-1' }, { package: 'spamass-milter', version: '0.4.0-2' }], package_version_read_at: '2026-10-21T08:00:00Z' };
const profile = (id) => ({ id, name: id, description: `${id} profile`, status: 'available', available: true, verified: false, latest_attempt_status: 'none', services: ['postfix'] });
function payload(services) {
  const all = services.some((item) => item.id === 'postfix') ? services : [...services, component('postfix', 'email', false)];
  return {
    scanned_at: new Date().toISOString(),
    dns_identity_ready: true,
    mail_hostname: { current: 'server1', hostname: '', source: '', current_usable: true, will_set_hostname: false },
    profiles: [profile('core-mail'), profile('webmail'), profile('protected-mail')],
    services: all,
    bundles: [],
  };
}

const originalFetch = globalThis.fetch;
let tree;
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
function serve(managed) {
  globalThis.fetch = async (input) => {
    const path = String(input).split('?')[0];
    if (path === '/api/v1/managed-services' || path === '/api/v1/managed-services/scan') return answer(managed);
    if (path === '/api/v1/host-mutation-readiness') return answer({ ready: true });
    if (path === '/api/v1/firewall') return answer({ enabled: true, engine_available: true, tcp_ports: [22, 80, 443], udp_ports: [], ssh_ports: [22], persistence_state: 'ready' });
    return answer({ error: 'no route' }, 404);
  };
}
async function settle() { await act(async () => { await new Promise((resolve) => setTimeout(resolve, 15)); }); }
async function mount(managed, catalogue = words) {
  globalThis.bundleWords = catalogue;
  globalThis.bundleToasts = [];
  globalThis.document = { addEventListener() {}, removeEventListener() {}, getElementById: () => null, activeElement: null, body: { style: {} } };
  globalThis.bundleTracker.calls.length = 0;
  globalThis.bundleTracker.set({ operation: null, failure: null, catalogSnapshot: null, locked: false });
  serve(managed);
  await act(async () => { tree = Renderer.create(React.createElement(ServiceList, {})); });
  for (let i = 0; i < 3; i += 1) await settle();
}
async function unmount() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  globalThis.bundleWords = words;
  delete globalThis.document;
}
const label = (node) => { const walk = (n) => (typeof n === 'string' ? n : (n.children || []).map(walk).join('')); return walk(node); };
// A row is found by its icon (RowIcon carries the component id), then its <li>.
const rowOf = (id) => {
  let node = tree.root.findAll((child) => child.props && child.props.id === id && child.props.category !== undefined && child.props.installed !== undefined)[0];
  while (node && node.type !== 'li') node = node.parent;
  return node;
};
const versionsIn = (node) => node.findAll((child) => child.props && child.props['data-row-version'] !== undefined);
const columns = () => tree.root.findAll((node) => node.props && node.props['data-version-column'] !== undefined);

test('without a known package version there is no version column; a runtime keeps its versions under the description', async () => {
  await mount(payload([
    component('nginx', 'web', true),
    component('php-fpm', 'web', true, { kind: 'runtime', versions: ['8.4', '8.3'] }),
    component('certbot', 'security', true, { kind: 'tool' }),
  ]));
  try {
    assert.equal(columns().length, 0, 'no column of empty cells');
    assert.match(rowOf('nginx').props.className, /xl:grid-cols-\[minmax\(0,1fr\)_8rem_17rem\]/);
    assert.deepEqual(versionsIn(rowOf('nginx')), [], 'an unknown version says nothing');
    assert.deepEqual(versionsIn(rowOf('certbot')), []);
    assert.ok(!label(rowOf('nginx')).includes('—'), 'never a dash');
    const php = versionsIn(rowOf('php-fpm'));
    assert.equal(php.length, 1);
    assert.equal(label(php[0]), '8.4, 8.3');
  } finally { await unmount(); }
});

test('a known package version brings the column back: the short form in the row, the full one in its tooltip; unknown and absent rows stay empty', async () => {
  // The catalogue view, remembered: an absent row is drawn there (alpha.100:
  // the first visit opens the installed view).
  globalThis.localStorage = { getItem: (key) => (key === 'celikpanel.components.view' ? 'catalog' : null), setItem() {}, removeItem() {} };
  await mount(payload([
    component('nginx', 'web', true, nginxVersion),
    component('mariadb', 'database', true, mariadbVersion),
    component('spamassassin', 'email', true, spamVersions),
    component('certbot', 'security', true, { kind: 'tool' }),
    component('postfix', 'email', false, nginxVersion),
    component('php-fpm', 'web', true, { kind: 'runtime', versions: ['8.4', '8.3'] }),
  ]));
  try {
    assert.equal(columns().length, 6, 'one column cell per row');
    assert.match(rowOf('nginx').props.className, /xl:grid-cols-\[minmax\(0,1fr\)_7\.5rem_8rem_17rem\]/);
    const nginx = versionsIn(rowOf('nginx'));
    // The phone line under the description and the desktop cell.
    assert.equal(nginx.length, 2);
    for (const node of nginx) {
      assert.equal(node.props['data-row-version'], '1.26.3-3+deb13u1');
      assert.match(node.props.className, /font-mono/);
      assert.equal(node.props.title, `1.26.3-3+deb13u1 · ${say('services.versionFromPackages')}`);
      assert.ok(label(node).startsWith('1.26.3'), label(node));
    }
    assert.match(nginx[0].props.className, /xl:hidden/);
    // The epoch and the Debian revision stay in the full form only.
    const mariadb = versionsIn(rowOf('mariadb'))[0];
    assert.ok(label(mariadb).startsWith('11.8.3'), label(mariadb));
    assert.equal(mariadb.props['data-row-version'], '1:11.8.3-0+deb13u1');
    // Packages that differ say so; no single version is picked.
    const spam = versionsIn(rowOf('spamassassin'));
    assert.ok(spam.length > 0);
    for (const node of spam) {
      assert.equal(label(node), say('services.versionSeveral'));
      assert.ok(!label(node).includes('4.0.1'));
    }
    assert.deepEqual(versionsIn(rowOf('certbot')), [], 'a component with no version read says nothing');
    assert.deepEqual(versionsIn(rowOf('postfix')), [], 'an absent component says nothing, whatever the payload carries');
    // The runtime keeps its own versions, as chips in the column.
    const php = versionsIn(rowOf('php-fpm'));
    assert.ok(php.some((node) => label(node) === '8.48.3' || label(node) === '8.4, 8.3'), php.map(label).join(' | '));
    for (const id of ['nginx', 'mariadb', 'spamassassin', 'certbot', 'postfix']) {
      assert.ok(!label(rowOf(id)).includes('—'), `${id}: never a dash`);
    }
  } finally { await unmount(); delete globalThis.localStorage; }
});

// alpha.101: Frankfurt's phpMyAdmin read "5.2.2-really…". The Panel now sends
// the real version after "really" as the short form; the cell never clips
// silently: it truncates with an ellipsis and the full text is its tooltip.
test('a Debian "really" version reads as its real version, and no version cell clips silently', async () => {
  const really = { package_version: '4:5.2.2-really5.2.1-1', package_version_short: '5.2.1', package_versions: [{ package: 'phpmyadmin', version: '4:5.2.2-really5.2.1-1' }], package_version_read_at: '2026-10-22T08:00:00Z' };
  await mount(payload([
    component('phpmyadmin', 'database', true, { kind: 'tool', status: 'installed', ...really }),
    component('nginx', 'web', true, nginxVersion),
    component('node', 'web', true, { kind: 'runtime', status: 'installed', versions: ['22.11.0'] }),
  ]));
  try {
    const cells = versionsIn(rowOf('phpmyadmin'));
    assert.equal(cells.length, 2);
    for (const node of cells) {
      assert.ok(label(node).startsWith('5.2.1'), label(node));
      assert.ok(!label(node).startsWith('5.2.2-really'), label(node));
      assert.match(node.props.className, /\btruncate\b/);
      assert.equal(node.props.title, `4:5.2.2-really5.2.1-1 · ${say('services.versionFromPackages')}`);
    }
    for (const node of versionsIn(rowOf('node'))) {
      assert.match(node.props.className, /\btruncate\b/);
      assert.equal(node.props.title, '22.11.0');
    }
    const source = readFileSync(new URL('../src/components/ServiceList.tsx', import.meta.url), 'utf8');
    assert.ok(source.includes('className="hidden min-w-0 xl:block" data-version-column=""'), 'the column cell may shrink, so truncate can draw its ellipsis');
  } finally { await unmount(); }
});

test('a kept reading says so in its tooltip; a version that is not the contract is not shown', async () => {
  await mount(payload([
    component('nginx', 'web', true, { ...nginxVersion, package_version_kept: true }),
    component('mariadb', 'database', true, { ...mariadbVersion, package_version: '11.8.3; rm -rf /' }),
    component('redis', 'cache', true, { ...nginxVersion, package_versions: [{ package: 'redis-server', version: '8.0.2-1' }] }),
  ]));
  try {
    const nginx = versionsIn(rowOf('nginx'))[0];
    assert.equal(nginx.props.title, `1.26.3-3+deb13u1 · ${say('services.versionFromPackagesKept')}`);
    assert.deepEqual(versionsIn(rowOf('mariadb')), [], 'odd characters are unknown, never shown');
    assert.deepEqual(versionsIn(rowOf('redis')), [], 'a package version that disagrees with the version is unknown');
  } finally { await unmount(); }
});

test('Turkish: the row and its tooltip are Turkish, with no raw key', async () => {
  await mount(payload([
    component('nginx', 'web', true, nginxVersion),
    component('spamassassin', 'email', true, spamVersions),
  ]), trWords);
  try {
    const nginx = versionsIn(rowOf('nginx'))[0];
    assert.equal(nginx.props.title, `1.26.3-3+deb13u1 · ${trWords['services.versionFromPackages']}`);
    assert.equal(label(versionsIn(rowOf('spamassassin'))[0]), trWords['services.versionSeveral']);
    assert.ok(!JSON.stringify(tree.toJSON()).includes('services.version'), 'a key rendered as its name');
  } finally { await unmount(); }
});

test('the new version texts exist in English and Turkish, in the components part, Turkish not English', () => {
  for (const key of ['services.versionSeveral', 'services.versionFromPackages', 'services.versionFromPackagesKept',
    'component.packageVersion', 'component.packageVersionSource', 'component.packageVersionKept', 'component.packageVersionSeveral']) {
    assert.equal(typeof enComponents[key], 'string', key);
    assert.equal(typeof trComponents[key], 'string', key);
    assert.notEqual(trComponents[key], enComponents[key], key);
  }
});
