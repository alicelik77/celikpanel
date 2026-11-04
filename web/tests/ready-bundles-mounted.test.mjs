// The ready bundles strip, mounted (alpha.94, D-034). The Components page is
// drawn with the real shared layer, the real strip and the real bundle
// decoder, from a map of what the Panel answers, in three states: offered,
// installed and verified, installed but not verified. A database or web
// bundle is then installed through a stand-in for the operation tracker that
// records every component install it is asked to start: the second install is
// not asked for before the first operation has ended with a fresh scan, and a
// failure stops the sequence.
// Hazır paket şeridi, bağlanmış hâlde: üç durum ve sıralı kurulum. İkinci
// kurulum, ilk işlem taze taramayla bitmeden istenmez; hata diziyi durdurur.
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
import { enServerScreens } from './locale-catalogue.mjs';

// The page speaks English from the real catalogue, so a bundle is named by
// its words and never by a fallback.
const words = { ...en, ...enScreens, ...enServerScreens };
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
  export const useLocation = () => ({ hash: '', pathname: '/components', search: globalThis.bundleSearch ?? '' });
  export const useNavigationBlocker = () => {};
  export const Link = (props) => React.createElement('a', null, props.children);
  export const showToast = (...args) => globalThis.bundleToasts.push(args);
  export const PageHeader = () => null;
  export const ServiceActionNotice = () => null, isServiceActionOutcome = () => false;
  export const FirewallNoSSHAcknowledgement = () => null, readFirewallSSHReason = () => null, FirewallSSHReasonLine = () => null;
  export const useAuth = () => ({ role: 'admin', user: { username: 'admin', role: 'admin' } });
  export const useServerSetup = () => null;
  // alpha.114: the in-place mail dialog has its own mounted tests (batch3).
  export const MailSetupDialog = () => null;
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

// --- What the Panel answers -------------------------------------------------------
const component = (id, category, installed, extra = {}) => ({
  id, name: id, description: `${id} on this server`, icon: '', category,
  status: installed ? 'active (running)' : 'not_installed', is_installed: installed, versions: installed ? ['1.0'] : [], ...extra,
});
const profile = (id) => ({ id, name: id, description: `${id} profile`, status: 'available', available: true, verified: false, latest_attempt_status: 'none', services: ['postfix'] });
// The name codes the Panel sends; the database bundles carry their tool in
// theirs since alpha.101.
const nameCodes = { 'db.postgresql': 'db_postgresql_phppgadmin', 'db.mariadb': 'db_mariadb_phpmyadmin' };
const bundle = (id, category, components, extra = {}) => ({
  id, category, name_code: nameCodes[id] ?? id.replace(/\./g, '_'), components, status: 'available', action: 'install',
  action_route: 'components', addons: [], missing_components: components, ...extra,
});
function payload({ nginx = false, phpfpm = false, bundles }) {
  return {
    scanned_at: new Date().toISOString(),
    dns_identity_ready: true,
    mail_hostname: { current: 'server1', hostname: '', source: '', current_usable: true, will_set_hostname: false },
    profiles: [profile('core-mail'), profile('webmail'), profile('protected-mail')],
    services: [
      component('nginx', 'web', nginx),
      component('php-fpm', 'web', phpfpm, { kind: 'runtime' }),
      component('mariadb', 'database', true),
      component('phpmyadmin', 'database', true, { kind: 'tool', status: 'installed' }),
      component('postfix', 'email', false),
      component('rspamd', 'email', false),
    ],
    bundles,
  };
}
const webBundle = (extra) => bundle('web.nginx_php', 'web', ['nginx', 'php-fpm'], extra);
// MariaDB + phpMyAdmin (alpha.101): the database with its browser tool and the
// web server and PHP-FPM the tool runs on.
const MARIADB_BUNDLE = ['mariadb', 'nginx', 'php-fpm', 'phpmyadmin'];
const dbBundle = (extra) => bundle('db.mariadb', 'database', MARIADB_BUNDLE, { status: 'installed_verified', action: 'none', missing_components: [], ...extra });
// Frankfurt-shaped package versions (sample data, Debian 13).
const versionOf = (full, short, pkg) => ({ versions: [], package_version: full, package_version_short: short, package_versions: [{ package: pkg, version: full }], package_version_read_at: '2026-10-22T08:00:00Z' });
const withVersions = (answer) => ({ ...answer, services: answer.services.map((item) => ({
  nginx: { ...item, ...versionOf('1.26.3-3+deb13u1', '1.26.3', 'nginx') },
  'php-fpm': { ...item, versions: ['8.4'] },
  mariadb: { ...item, ...versionOf('1:11.8.3-0+deb13u1', '11.8.3', 'mariadb-server') },
  phpmyadmin: { ...item, ...versionOf('4:5.2.2-really5.2.1-1', '5.2.1', 'phpmyadmin') },
}[item.id] ?? item)) });
const partsOf = () => Object.fromEntries(tree.root.findAll((node) => node.props && node.props['data-bundle-part-state'] !== undefined)
  .map((node) => [node.props['data-bundle-part-state'] === undefined ? '' : label(node.children.find((child) => child.type === 'span' && child.props.className === 'shrink-0 text-fg')), node]));

const originalFetch = globalThis.fetch;
// What /host-mutation-readiness answers; a test may close it.
let readiness = { ready: true };
const originalStart = globalThis.bundleTracker.state.startInstall;
const restoreTracker = () => { globalThis.bundleTracker.state.startInstall = originalStart; };
let tree;
let requests;
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
// alpha.118: a test may answer more routes (method and path) and read every
// full address asked for, query included.
let extraRoutes = {};
let urls = [];
function serve(managed) {
  requests = [];
  urls = [];
  globalThis.fetch = async (input, init = {}) => {
    const path = String(input).split('?')[0];
    requests.push(`${init.method || 'GET'} ${path}`);
    urls.push({ method: init.method || 'GET', url: String(input), body: init.body });
    const extra = extraRoutes[`${init.method || 'GET'} ${path}`];
    if (extra) return answer(...extra);
    if (path === '/api/v1/managed-services' || path === '/api/v1/managed-services/scan') return answer(managed);
    if (path === '/api/v1/host-mutation-readiness') return answer(readiness);
    if (path === '/api/v1/firewall') return answer({ enabled: true, engine_available: true, tcp_ports: [22, 80, 443], udp_ports: [], ssh_ports: [22], persistence_state: 'ready' });
    return answer({ error: 'no route' }, 404);
  };
}
async function settle() { await act(async () => { await new Promise((resolve) => setTimeout(resolve, 15)); }); }
async function mount(managed) {
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
  delete globalThis.document;
}
const label = (node) => { const walk = (n) => (typeof n === 'string' ? n : (n.children || []).map(walk).join('')); return walk(node); };
const strip = (category) => tree.root.find((node) => node.props && node.props['data-bundle-strip'] === category);
const stripText = (category) => label(strip(category));
const bundleNode = (id) => tree.root.findAll((node) => node.props && node.props['data-bundle'] === id)[0];
const buttonsIn = (node) => node.findAll((child) => child.type === 'button');
async function press(node) { await act(async () => { await node.props.onClick(); }); await settle(); }
async function pressLabel(text) {
  const node = tree.root.findAll((child) => child.type === 'button' && label(child).includes(text))[0];
  assert.ok(node, `no button ${text}`);
  await press(node);
}

// --- The chips ----------------------------------------------------------------------
const chips = (category) => strip(category).findAll((node) => node.props && node.props['data-bundle-state'] !== undefined);
const dialogText = () => {
  const dialogs = tree.root.findAll((node) => node.props && node.props.role === 'dialog');
  return dialogs.map(label).join(' | ');
};
// A bundle part's component name, without what its row says after it (its
// version since alpha.105).
const dotted = (text) => text.replace(/\s*·\s*/g, ' · ');
const partName = (node) => dotted(label(node)).replace(/(Will be installed|Already installed|Not needed|[0-9][0-9., ]* · already installed|The distribution’s version · recommended|…)(Change)?$/, '');
const installedState = () => tree.root.findAll((node) => node.props && node.props['data-bundle-installed-state'] !== undefined)[0]?.props['data-bundle-installed-state'];

test('an offered bundle is an outlined chip that says Install; pressing it opens the confirmation, and nothing is sent', async () => {
  await mount(payload({ bundles: [webBundle(), dbBundle()] }));
  try {
    const chip = bundleNode('web.nginx_php');
    assert.equal(chip.type, 'button');
    assert.equal(chip.props['data-bundle-state'], 'available');
    assert.ok(stripText('web').includes(say('services.bundles.label')), stripText('web'));
    assert.equal(label(chip), `Nginx + PHP-FPM·${say('services.bundles.chip.install')}`);
    assert.doesNotMatch(chip.props.className, /bg-primary\b/, 'the strip has no filled button');
    // No permanent hint under the strip.
    assert.ok(!stripText('web').includes('A ready bundle installs'), stripText('web'));
    await press(chip);
    assert.ok(dialogText().includes(say('services.bundles.dialog.title', { name: say('services.bundles.name.web_nginx_php') })), dialogText());
    assert.equal(requests.filter((line) => !line.startsWith('GET ') && !line.startsWith('POST /api/v1/managed-services/scan')).length, 0, requests.join(', '));
    assert.equal(globalThis.bundleTracker.calls.length, 0);
  } finally { await unmount(); }
});

// alpha.101 (owner's screens on Frankfurt): each part says its row's state
// and version, not "Already installed"; there is no Verify button and no
// "Verify reads the components again" sentence; Close stays.
test('an installed and verified component bundle is a check-mark chip; its dialogue says each part\'s state and version, with Close only', async () => {
  await mount(withVersions(payload({ nginx: true, phpfpm: true, bundles: [webBundle({ status: 'installed_verified', action: 'none', missing_components: [] }), dbBundle()] })));
  try {
    const chip = bundleNode('db.mariadb');
    assert.equal(chip.props['data-bundle-state'], 'verified');
    assert.equal(label(chip), 'MariaDB + phpMyAdmin');
    assert.equal(chip.props['aria-label'], 'MariaDB + phpMyAdmin, installed and verified');
    // No Repair link on the strip.
    assert.ok(!stripText('database').includes(say('services.bundles.repairEllipsis')) && !stripText('database').includes('Repair'), stripText('database'));
    await press(chip);
    assert.equal(installedState(), 'verified');
    assert.ok(dialogText().includes(say('services.bundles.installed.verified', { name: 'MariaDB + phpMyAdmin' })), dialogText());
    // Described, not told to install (alpha.95 review).
    assert.ok(dialogText().includes(say('services.bundles.about.db_mariadb_phpmyadmin')), dialogText());
    assert.ok(!dialogText().includes(say('services.bundles.description.db_mariadb_phpmyadmin')), dialogText());
    const parts = partsOf();
    const said = Object.fromEntries(Object.entries(parts).map(([name, node]) => [name, label(node).slice(name.length)]));
    assert.deepEqual(said, {
      [nameOf('mariadb')]: `${say('services.running')} · 11.8.3`,
      [nameOf('nginx')]: `${say('services.running')} · 1.26.3`,
      [nameOf('php-fpm')]: `${say('services.running')} · 8.4`,
      [nameOf('phpmyadmin')]: `${say('services.installedLabel')} · 5.2.1`,
    });
    assert.ok(!dialogText().includes('Already installed'), dialogText());
    // The full version as the package database printed it is the part's tooltip.
    const versionCell = parts[nameOf('phpmyadmin')].find((node) => node.props && node.props.title === '4:5.2.2-really5.2.1-1');
    assert.match(versionCell.props.className, /\bmin-w-0\b/);
    const shortVersion = versionCell.find((node) => node.type === 'span' && typeof node.props.className === 'string' && /\btruncate\b/.test(node.props.className));
    assert.equal(label(shortVersion), '5.2.1');
    assert.match(shortVersion.props.className, /font-mono/);
    // The same word its row says, from the same data.
    for (const id of MARIADB_BUNDLE) {
      let row = tree.root.findAll((node) => node.props && node.props.id === id && node.props.category !== undefined && node.props.installed !== undefined)[0];
      while (row && row.type !== 'li') row = row.parent;
      if (!row) continue;
      const word = said[nameOf(id)].split(' · ')[0];
      assert.ok(label(row).includes(word), `${id}: the row does not say ${word}`);
    }
    const buttons = tree.root.findAll((node) => node.props && node.props.role === 'dialog')[0].findAll((node) => node.type === 'button').map(label);
    assert.deepEqual(buttons.filter((text) => text !== ''), [say('common.close')], buttons.join(', '));
    assert.ok(!dialogText().includes('Verify'), dialogText());
    assert.equal(tree.root.findAll((node) => node.props && node.props['data-bundle-installed-how'] !== undefined).length, 0, 'a verified component bundle needs no how-to sentence');
  } finally { await unmount(); }
});

const mail = (id, profileId, extra) => ({ ...bundle(id, 'email', ['postfix'], extra), action_route: 'mail_profile', profile_id: profileId });
const withPostfix = (answer) => ({ ...answer, services: answer.services.map((item) => (item.id === 'postfix' ? { ...item, is_installed: true, status: 'active (running)' } : item)) });

test('an installed mail bundle opens a dialogue with Repair…, never Verify; one not verified says so with a grey dot and its reason under the strip', async () => {
  await mount(withPostfix(payload({ bundles: [mail('mail.core', 'core-mail', { status: 'installed_verified', action: 'repair', missing_components: [] })] })));
  try {
    const chip = bundleNode('mail.core');
    assert.equal(label(chip), 'Core Mail');
    await press(chip);
    const buttons = tree.root.findAll((node) => node.type === 'button').map(label);
    assert.ok(buttons.includes(say('services.bundles.repairEllipsis')), buttons.join(', '));
    assert.ok(!buttons.includes('Verify'), 'a mail bundle offers Verify');
    assert.ok(dialogText().includes(say('services.bundles.installed.repairHow')), dialogText());
  } finally { await unmount(); }

  await mount(withPostfix(payload({ bundles: [mail('mail.core', 'core-mail', { status: 'installed_unverified', action: 'repair', missing_components: [], warning_code: 'repair_to_verify' })] })));
  try {
    const chip = bundleNode('mail.core');
    assert.equal(chip.props['data-bundle-state'], 'unverified');
    assert.equal(label(chip), `Core Mail·${say('services.bundles.chip.notVerified')}`);
    const dot = chip.find((node) => node.props && node.props['data-tone'] !== undefined);
    assert.equal(dot.props['data-tone'], 'unknown');
    // The Panel's reason is said again under the chips, calmly.
    const reason = strip('email').find((node) => node.props && node.props['data-bundle-reason'] === 'mail.core');
    assert.equal(label(reason), say('services.mailProfiles.code.repair_to_verify'));
    assert.match(reason.props.className, /text-fg-muted/);
  } finally { await unmount(); }
});

// An installed component bundle with a stopped part: the part says Stopped in
// the row's red, the dialogue points to the page's Rescan in words, and there
// is no button of its own. An older Panel's "verify" action draws no button
// either.
test('a component bundle with a stopped part says which part and points to Rescan; nothing is sent', async () => {
  for (const action of ['none', 'verify']) {
    const stopped = withVersions(payload({ nginx: true, phpfpm: true, bundles: [webBundle({ status: 'installed_verified', action: 'none', missing_components: [] }), dbBundle({ status: 'installed_unverified', action })] }));
    stopped.services = stopped.services.map((item) => (item.id === 'mariadb' ? { ...item, status: 'inactive (dead)' } : item));
    await mount(stopped);
    try {
      assert.equal(bundleNode('db.mariadb').props['data-bundle-state'], 'unverified');
      await press(bundleNode('db.mariadb'));
      assert.equal(installedState(), 'unverified');
      const parts = partsOf();
      const maria = parts[nameOf('mariadb')];
      assert.equal(maria.props['data-bundle-part-state'], 'stopped');
      assert.ok(label(maria).endsWith(`${say('services.stopped')} · 11.8.3`), label(maria));
      // Only the state word is red; the version stays muted (review of alpha.101).
      const word = maria.find((node) => node.props && node.props['data-bundle-part-word'] !== undefined);
      const version = maria.find((node) => node.props && node.props['data-bundle-part-version'] !== undefined);
      assert.equal(label(word), say('services.stopped'));
      assert.match(word.props.className, /\btext-danger\b/, 'a stopped part is not drawn as the row draws it');
      assert.equal(label(version), '11.8.3');
      assert.doesNotMatch(version.props.className, /text-danger/, 'the version of a stopped part is drawn red');
      assert.match(version.props.className, /\btext-fg-muted\b/);
      let up = version.parent;
      while (up && up !== maria) {
        assert.ok(!(typeof up.props?.className === 'string' && /text-danger/.test(up.props.className)), 'the version inherits the red of a stopped part');
        up = up.parent;
      }
      assert.equal(parts[nameOf('nginx')].props['data-bundle-part-state'], 'running');
      assert.ok(dialogText().includes(say('services.bundles.installed.unverifiedHow', { rescan: say('services.rescan') })), dialogText());
      const buttons = tree.root.findAll((node) => node.props && node.props.role === 'dialog')[0].findAll((node) => node.type === 'button').map(label);
      assert.deepEqual(buttons.filter((text) => text !== ''), [say('common.close')], `${action}: ${buttons.join(', ')}`);
      assert.equal(requests.filter((line) => line.startsWith('POST ')).length, 0, requests.join(', '));
    } finally { await unmount(); }
  }
});

test('under Apache the MariaDB bundle says what it installs, what is installed and what is not needed, and never calls Nginx installed', async () => {
  const apache = payload({ bundles: [
    bundle('db.mariadb', 'database', MARIADB_BUNDLE, { missing_components: ['phpmyadmin'] }),
  ], phpfpm: true });
  apache.services = apache.services.map((item) => (item.id === 'phpmyadmin' ? { ...item, is_installed: false, status: 'not_installed', versions: [] } : item));
  apache.services.push(component('apache', 'web', true));
  await mount(apache);
  try {
    // alpha.122: MariaDB and PHP-FPM are here, Nginx is not needed: the chip names what it adds.
    assert.equal(label(bundleNode('db.mariadb')), `MariaDB + phpMyAdmin·${say('services.bundles.chip.addNamed', { names: nameOf('phpmyadmin') })}`);
    await press(bundleNode('db.mariadb'));
    const parts = Object.fromEntries(tree.root.findAll((node) => node.props && node.props['data-bundle-part'] !== undefined)
      .map((node) => [partName(node), node.props['data-bundle-part']]));
    assert.deepEqual(parts, { [nameOf('mariadb')]: 'installed', [nameOf('nginx')]: 'notNeeded', [nameOf('php-fpm')]: 'installed', [nameOf('phpmyadmin')]: 'install' });
    assert.ok(tree.root.findAll((node) => node.type === 'button' && label(node) === say('services.bundles.dialog.confirm', { n: 1 })).length === 1);
    // The install of phpMyAdmin fails: the stopped chip counts Nginx, which
    // Apache makes "not needed", as present, so "3 of 4" can still complete.
    await pressLabel(say('services.bundles.dialog.confirm', { n: 1 }));
    assert.deepEqual(globalThis.bundleTracker.calls.map((call) => call.serviceId), ['phpmyadmin']);
    globalThis.bundleTracker.set({ operation: null, locked: false, failure: { message: 'phpmyadmin failed', code: 'service_install_failed', component: 'phpmyadmin' }, catalogSnapshot: apache });
    for (let i = 0; i < 4; i += 1) await settle();
    const chip = tree.root.findAll((node) => node.props && node.props['data-bundle-status'] === 'stopped')[0];
    assert.ok(chip, 'the sequence did not stop');
    assert.ok(label(chip).includes(say('services.bundles.chip.progress', { n: 3, total: 4 })), label(chip));
  } finally { await unmount(); }
});

// A part the scan has never observed is "not checked yet", never "not needed".
test('an installed bundle\'s part that was never observed says "not checked yet"', async () => {
  const answer = withVersions(payload({ nginx: true, phpfpm: true, bundles: [dbBundle()] }));
  answer.services = answer.services.map((item) => (item.id === 'phpmyadmin' ? { ...item, is_installed: null, status: 'unknown' } : item));
  await mount(answer);
  try {
    await press(bundleNode('db.mariadb'));
    const part = partsOf()[nameOf('phpmyadmin')];
    assert.equal(part.props['data-bundle-part-state'], 'notChecked');
    assert.equal(part.props['data-bundle-part'], 'notChecked');
    assert.ok(label(part).endsWith(say('services.notChecked')), label(part));
    assert.ok(!label(part).includes(say('services.bundles.dialog.notNeeded')), label(part));
  } finally { await unmount(); }
});

test('a server with nothing installed says so once and still draws the strips', async () => {
  const empty = payload({ bundles: [webBundle(), bundle('db.mariadb', 'database', MARIADB_BUNDLE)] });
  empty.services = empty.services.map((item) => ({ ...item, is_installed: false, status: 'not_installed', versions: [] }));
  await mount(empty);
  try {
    assert.ok(label(tree.root).includes(say('services.noneInstalledYet')), 'the empty view no longer says nothing is installed');
    assert.ok(bundleNode('web.nginx_php') && bundleNode('db.mariadb'), 'the strips are not drawn');
    // alpha.122: every component is still listed, as not installed.
    assert.ok(rowOf('nginx') && rowOf('mariadb'), 'the not-installed rows are listed');
  } finally { await unmount(); }
});

// --- The owner's two servers ----------------------------------------------------------
// Payloads shaped like the owner's screens (2026-10-17, and alpha.100 on
// 2026-10-22): the same registry on both, different states. Boston:
// PostgreSQL and Nginx, no mail, no MariaDB, no PHP-FPM, no phpPgAdmin.
// Frankfurt: everything installed and verified. Since alpha.101 a database
// bundle is the database with its browser tool.
const registry = () => [
  { ...bundle('mail.core', 'email', ['postfix', 'dovecot'], { addons: ['mail.protected'] }), action_route: 'mail_profile', profile_id: 'core-mail' },
  { ...bundle('mail.webmail', 'email', ['postfix', 'dovecot', 'nginx', 'php-fpm', 'roundcube'], { addons: ['mail.protected'] }), action_route: 'mail_profile', profile_id: 'webmail' },
  { ...bundle('mail.protected', 'email', ['postfix', 'dovecot', 'rspamd']), action_route: 'mail_profile', profile_id: 'protected-mail' },
  bundle('db.postgresql', 'database', ['postgresql', 'nginx', 'php-fpm', 'phppgadmin']),
  bundle('db.mariadb', 'database', MARIADB_BUNDLE),
  bundle('web.nginx_php', 'web', ['nginx', 'php-fpm']),
];
const verifiedBundle = (item) => ({ ...item, status: 'installed_verified', action: item.action_route === 'mail_profile' ? 'repair' : 'none', missing_components: [] });
function server(name) {
  const installed = name === 'frankfurt'
    ? new Set(['postfix', 'dovecot', 'rspamd', 'nginx', 'php-fpm', 'roundcube', 'postgresql', 'mariadb', 'phppgadmin', 'phpmyadmin'])
    : new Set(['nginx', 'postgresql']);
  const services = [
    ['postfix', 'email'], ['dovecot', 'email'], ['rspamd', 'email'], ['roundcube', 'email', { kind: 'tool' }],
    ['nginx', 'web'], ['php-fpm', 'web', { kind: 'runtime' }],
    ['postgresql', 'database'], ['mariadb', 'database'], ['phppgadmin', 'database', { kind: 'tool', status: 'installed' }], ['phpmyadmin', 'database', { kind: 'tool', status: 'installed' }],
  ].map(([id, category, extra]) => component(id, category, installed.has(id), installed.has(id) ? extra : { ...extra, status: 'not_installed' }));
  // As the Panel derives them (TestBundlesOnABostonShapedServer,
  // TestBundlesOnAnArchShapedServer).
  const bundles = registry().map((item) => {
    if (name === 'frankfurt') return verifiedBundle(item);
    switch (item.id) {
    case 'db.postgresql': return name === 'arch'
      ? { ...item, status: 'blocked', action: 'none', missing_components: ['php-fpm', 'phppgadmin'], blocked_code: 'component_not_installable', blocked_vars: { component: 'phpPgAdmin', kind: 'package', platform: 'Arch Linux (pacman)' } }
      : { ...item, missing_components: ['php-fpm', 'phppgadmin'] };
    case 'db.mariadb': return { ...item, missing_components: ['mariadb', 'php-fpm', 'phpmyadmin'] };
    case 'web.nginx_php': return { ...item, missing_components: ['php-fpm'] };
    default: return item;
    }
  });
  return { ...payload({ bundles }), services };
}

test('Boston and Frankfurt draw the same chips in the same order; PostgreSQL + phpPgAdmin is offered on Boston, Core Mail is included on Frankfurt', async () => {
  const seen = {};
  for (const name of ['boston', 'frankfurt']) {
    await mount(server(name));
    try {
      seen[name] = Object.fromEntries(['email', 'database', 'web'].map((category) => [category, chips(category).map((node) => [node.props['data-bundle'], node.props['data-bundle-state'], label(node)])]));
      // No Repair link and no "installed and verified" sentence on any strip.
      for (const category of ['email', 'database', 'web']) {
        assert.ok(!stripText(category).includes('Repair'), stripText(category));
        assert.ok(!stripText(category).includes('installed and verified'), stripText(category));
      }
      // The Dashboard's count is the number of chips that install.
      const installable = ['email', 'database', 'web'].flatMap(chips).filter((node) => ['available', 'partial'].includes(node.props['data-bundle-state'])).length;
      assert.equal(installable, name === 'boston' ? 6 : 0, name);
    } finally { await unmount(); }
  }
  for (const category of ['email', 'database', 'web']) {
    assert.deepEqual(seen.boston[category].map(([id]) => id), seen.frankfurt[category].map(([id]) => id), category);
  }
  assert.deepEqual(seen.boston.database, [
    // alpha.122 (owner's Boston screens): PostgreSQL and Nginx are here, so
    // each database chip names what it adds instead of "Install".
    ['db.postgresql', 'available', `PostgreSQL + phpPgAdmin·${say('services.bundles.chip.addNamed', { names: `${nameOf('php-fpm')}, ${nameOf('phppgadmin')}` })}`],
    ['db.mariadb', 'available', `MariaDB + phpMyAdmin·${say('services.bundles.chip.addNamed', { names: `${nameOf('mariadb')}, ${nameOf('php-fpm')}, ${nameOf('phpmyadmin')}` })}`],
  ]);
  assert.deepEqual(seen.frankfurt.database, [
    ['db.postgresql', 'verified', 'PostgreSQL + phpPgAdmin'],
    ['db.mariadb', 'verified', 'MariaDB + phpMyAdmin'],
  ]);
  assert.deepEqual(seen.frankfurt.email, [
    ['mail.core', 'included', `Core Mail·${say('services.bundles.chip.included')}`],
    ['mail.webmail', 'verified', 'Webmail'],
    ['mail.protected', 'verified', 'Spam-Protected Mail'],
  ]);
  // The included chip's dialogue names the bundle that contains it and offers no Repair of its own.
  await mount(server('frankfurt'));
  try {
    await press(bundleNode('mail.core'));
    assert.equal(installedState(), 'included');
    assert.ok(dialogText().includes(say('services.bundles.installed.included', { name: 'Core Mail', other: 'Webmail' })), dialogText());
    assert.ok(!tree.root.findAll((node) => node.type === 'button').map(label).includes(say('services.bundles.repairEllipsis')));
  } finally { await unmount(); }
  // Frankfurt's database bundle: installed and verified, each part with its state.
  await mount(server('frankfurt'));
  try {
    await press(bundleNode('db.postgresql'));
    assert.equal(installedState(), 'verified');
    const states = Object.fromEntries(Object.entries(partsOf()).map(([name, node]) => [name, node.props['data-bundle-part-state']]));
    assert.deepEqual(states, { [nameOf('postgresql')]: 'running', [nameOf('nginx')]: 'running', [nameOf('php-fpm')]: 'running', [nameOf('phppgadmin')]: 'installed' });
  } finally { await unmount(); }
});

// Boston (owner's screen on alpha.100): PostgreSQL and Nginx are kept,
// PHP-FPM and phpPgAdmin are installed, in that order, one at a time.
test('on Boston, PostgreSQL + phpPgAdmin keeps PostgreSQL and Nginx and installs only PHP-FPM, then phpPgAdmin', async () => {
  const tracker = globalThis.bundleTracker;
  await mount(server('boston'));
  try {
    await press(bundleNode('db.postgresql'));
    assert.ok(dialogText().includes(say('services.bundles.dialog.title', { name: 'PostgreSQL + phpPgAdmin' })), dialogText());
    const parts = Object.fromEntries(tree.root.findAll((node) => node.props && node.props['data-bundle-part'] !== undefined)
      .map((node) => [partName(node), node.props['data-bundle-part']]));
    assert.deepEqual(parts, { [nameOf('postgresql')]: 'installed', [nameOf('nginx')]: 'installed', [nameOf('php-fpm')]: 'install', [nameOf('phppgadmin')]: 'install' });
    await pressLabel(say('services.bundles.dialog.confirm', { n: 2 }));
    assert.deepEqual(tracker.calls.map((call) => call.serviceId), ['php-fpm']);
    const after = server('boston');
    after.services = after.services.map((item) => (item.id === 'php-fpm' ? { ...item, is_installed: true, status: 'active (running)' } : item));
    tracker.set({ operation: null, locked: false, catalogSnapshot: after });
    for (let i = 0; i < 3; i += 1) await settle();
    assert.deepEqual(tracker.calls.map((call) => call.serviceId), ['php-fpm', 'phppgadmin']);
  } finally { await unmount(); }
});

// Arch: phpPgAdmin has no package there; the whole bundle is blocked, its chip
// names the part plainly, and a press says the full reason and installs nothing.
test('on Arch, PostgreSQL + phpPgAdmin is blocked whole and says phpPgAdmin is not available on this system', async () => {
  await mount(server('arch'));
  try {
    const blocked = bundleNode('db.postgresql');
    assert.equal(blocked.props['data-bundle-state'], 'blocked');
    assert.equal(label(blocked), `PostgreSQL + phpPgAdmin·${say('services.bundles.short.component_not_installable_named', { component: 'phpPgAdmin' })}`);
    assert.equal(blocked.type, 'button');
    assert.equal(blocked.props['aria-expanded'], false);
    await press(blocked);
    const reason = tree.root.find((node) => node.props && node.props['data-bundle-blocked-reason'] === 'db.postgresql');
    assert.equal(reason.props.id, blocked.props['aria-controls']);
    assert.ok(label(reason).includes(say('services.mailProfiles.code.component_not_installable.platform', { component: 'phpPgAdmin', platform: 'Arch Linux (pacman)' })), label(reason));
    assert.equal(bundleNode('db.postgresql').props['aria-expanded'], true);
    assert.equal(globalThis.bundleTracker.calls.length, 0);
    assert.equal(tree.root.findAll((node) => node.props && node.props.role === 'dialog').length, 0, 'a blocked chip opened a dialogue');
    assert.equal(bundleNode('db.mariadb').props['data-bundle-state'], 'available');
  } finally { await unmount(); }
});

// --- Roundcube leftovers ------------------------------------------------------------
const withRoundcube = (extra = {}) => {
  const answer = payload({ bundles: [webBundle(), dbBundle()] });
  answer.services.push(component('roundcube', 'email', false, { kind: 'tool' }));
  return { ...answer, ...extra };
};
const leftoversNote = () => tree.root.findAll((node) => node.props && node.props['data-webmail-leftovers'] !== undefined)[0];
const leftoversButton = () => tree.root.findAll((node) => node.type === 'button' && node.props['data-webmail-cleanup'] !== undefined)[0];

test('Roundcube leftovers: without the Panel\'s record of an unfinished install or removal the row says nothing and offers nothing', async () => {
  await mount(withRoundcube());
  try {
    assert.equal(leftoversNote(), undefined);
    assert.equal(leftoversButton(), undefined);
  } finally { await unmount(); }
  // alpha.96: the Panel folds the webmail attempt into its record (the newer
  // of the Roundcube entry and the webmail attempt decides). A failed attempt
  // that a later clean removal followed arrives without a record: nothing.
  const failed = withRoundcube();
  failed.profiles = failed.profiles.map((item) => (item.id === 'webmail' ? { ...item, latest_attempt_status: 'failed', latest_attempt_error: 'roundcube download failed' } : item));
  await mount(failed);
  try {
    assert.equal(leftoversNote(), undefined, 'a failed attempt alone still offers the cleanup');
    assert.equal(leftoversButton(), undefined);
  } finally { await unmount(); }
  // The Panel's record of that failed attempt is the offer.
  await mount({ ...failed, webmail_cleanup: 'install_failed' });
  try {
    assert.equal(label(leftoversNote()), say('services.cleanupWebmailRowNote.install'));
    assert.ok(leftoversButton());
  } finally { await unmount(); }
  // Roundcube's own page opens this page with ?cleanup=roundcube: the dialogue
  // opens once, whatever the row offers, and the address loses ?cleanup with
  // a replace so a reload does not open it again.
  globalThis.bundleSearch = '?cleanup=roundcube';
  globalThis.bundleNavigated = [];
  globalThis.bundleNavigateOptions = [];
  await mount(withRoundcube());
  try {
    assert.ok(tree.root.findAll((node) => node.props && node.props['data-webmail-cleanup-list'] !== undefined).length === 1, 'the cleanup link did not open the dialogue');
    assert.equal(leftoversButton(), undefined);
    assert.deepEqual(globalThis.bundleNavigated, ['/components']);
    assert.deepEqual(globalThis.bundleNavigateOptions, [{ replace: true }]);
  } finally { await unmount(); delete globalThis.bundleSearch; delete globalThis.bundleNavigated; delete globalThis.bundleNavigateOptions; }
});

test('Roundcube leftovers: with the Panel\'s record, a muted line on the row, a neutral button, and a confirmation that lists what is removed and what is not', async () => {
  await mount(withRoundcube({ webmail_cleanup: 'removal_incomplete' }));
  try {
    const note = leftoversNote();
    assert.equal(note.props['data-webmail-leftovers'], 'removal');
    assert.equal(label(note), say('services.cleanupWebmailRowNote.removal'));
    assert.match(note.props.className, /text-fg-muted/);
    const button = leftoversButton();
    assert.equal(label(button), say('services.cleanupWebmailRow'));
    assert.doesNotMatch(button.props.className, /danger/);
    await press(button);
    const list = tree.root.find((node) => node.props && node.props['data-webmail-cleanup-list'] !== undefined);
    const listed = label(list);
    for (const path of ['/var/lib/celikpanel-webmail', '/etc/nginx/conf.d/celikpanel-webmail.conf', '/run/celikpanel-webmail.sock', '/var/lib/.celikpanel-webmail.stage']) {
      assert.ok(listed.includes(path), listed);
    }
    assert.ok(dialogText().includes(say('services.cleanupWebmailUntouched')), dialogText());
    const confirm = tree.root.findAll((node) => node.type === 'button' && label(node) === say('services.cleanupWebmail'))[0];
    assert.ok(confirm, 'no confirm button');
    assert.match(confirm.props.className, /danger/, 'the danger style stays on the confirmation');
    assert.equal(requests.filter((line) => line.startsWith('POST /api/v1/service/uninstall')).length, 0, 'opening the dialogue removed something');
  } finally { await unmount(); }
});

// --- The sequence -------------------------------------------------------------------
async function startWebBundle(n = 2) {
  await press(bundleNode('web.nginx_php'));
  await pressLabel(say('services.bundles.dialog.confirm', { n }));
}
const fresh = (extra) => payload(extra);
const running = () => tree.root.findAll((node) => node.props && node.props['data-bundle-running'] !== undefined).length;
const stoppedChip = () => tree.root.findAll((node) => node.props && node.props['data-bundle-status'] === 'stopped')[0];
const stoppedReason = () => tree.root.findAll((node) => node.props && node.props['data-bundle-stopped-reason'] !== undefined)[0];
const ours = (serviceId, n) => ({ id: 'op' + n, service_id: serviceId, kind: 'service_install', status: 'running' });

test('a web bundle installs its components one at a time: the second only after the first operation, its own, ended with a fresh scan', async () => {
  const tracker = globalThis.bundleTracker;
  await mount(payload({ bundles: [webBundle(), dbBundle()] }));
  try {
    await startWebBundle();
    assert.deepEqual(tracker.calls.map((call) => call.serviceId), ['nginx'], 'the first install');
    assert.ok(stripText('web').includes(say('services.bundles.step', { n: 1, total: 2, component: nameOf('nginx') })), stripText('web'));
    assert.equal(running(), 1);
    // The other chips wait while a bundle installs.
    assert.equal(bundleNode('db.mariadb').props.disabled, true);
    for (let i = 0; i < 4; i += 1) await settle();
    tracker.set({ operation: { ...tracker.state.operation, phase: 'installing' } });
    await settle();
    assert.equal(tracker.calls.length, 1, 'the second install was asked for while the first was running');
    tracker.set({ operation: null, locked: true });
    await settle();
    assert.equal(tracker.calls.length, 1, 'asked for before the tracker released the host');
    tracker.set({ locked: false, catalogSnapshot: fresh({ nginx: true, bundles: [webBundle({ missing_components: ['php-fpm'] }), dbBundle()] }) });
    for (let i = 0; i < 3; i += 1) await settle();
    assert.deepEqual(tracker.calls.map((call) => call.serviceId), ['nginx', 'php-fpm']);
    assert.ok(stripText('web').includes(say('services.bundles.step', { n: 2, total: 2, component: nameOf('php-fpm') })), stripText('web'));
    tracker.set({ operation: null, locked: false, catalogSnapshot: fresh({ nginx: true, phpfpm: true, bundles: [webBundle({ status: 'installed_verified', action: 'none', missing_components: [] }), dbBundle()] }) });
    for (let i = 0; i < 3; i += 1) await settle();
    assert.equal(tracker.calls.length, 2);
    assert.equal(running(), 0);
    assert.equal(bundleNode('web.nginx_php').props['data-bundle-status'], 'installed_verified');
    assert.ok(tracker.calls.every((call) => call.operationKind === undefined && call.package === undefined));
  } finally { await unmount(); }
});

test('while the gate is read the chip says it checks the server; "installing" only once the install was accepted', async () => {
  const tracker = globalThis.bundleTracker;
  await mount(payload({ bundles: [webBundle(), dbBundle()] }));
  // The readiness read waits until the test lets it answer; the install is
  // accepted only when the test says so.
  let release;
  const held = new Promise((resolve) => { release = resolve; });
  const plain = globalThis.fetch;
  globalThis.fetch = async (input, init) => {
    if (String(input).split('?')[0] === '/api/v1/host-mutation-readiness') await held;
    return plain(input, init);
  };
  let accept;
  tracker.state.startInstall = (request) => new Promise((resolve) => {
    tracker.calls.push(request);
    accept = () => { tracker.set({ operation: ours(request.serviceId, tracker.calls.length), locked: true }); resolve(true); };
  });
  try {
    await press(bundleNode('web.nginx_php'));
    await act(async () => {
      tree.root.findAll((node) => node.type === 'button' && label(node) === say('services.bundles.dialog.confirm', { n: 2 }))[0].props.onClick();
    });
    await settle();
    const checking = say('services.bundles.checkingBefore', { component: nameOf('nginx') });
    const installing = say('services.bundles.step', { n: 1, total: 2, component: nameOf('nginx') });
    assert.ok(stripText('web').includes(checking), stripText('web'));
    assert.ok(!stripText('web').includes(installing), 'it said installing before anything was sent');
    assert.equal(tracker.calls.length, 0);
    await act(async () => { release(); });
    await settle();
    assert.equal(tracker.calls.length, 1);
    assert.ok(stripText('web').includes(checking) && !stripText('web').includes(installing), 'it said installing before the server accepted');
    await act(async () => { accept(); });
    await settle();
    assert.ok(stripText('web').includes(installing), stripText('web'));
  } finally { await unmount(); restoreTracker(); }
});

test('an operation that ends without a fresh scan from the tracker sends nothing more, even with the host released', async () => {
  const tracker = globalThis.bundleTracker;
  await mount(payload({ bundles: [webBundle(), dbBundle()] }));
  try {
    await startWebBundle();
    assert.equal(tracker.calls.length, 1);
    // The operation ended and the host is released, but no fresh scan came.
    tracker.set({ operation: null, locked: false });
    for (let i = 0; i < 4; i += 1) await settle();
    assert.equal(tracker.calls.length, 1, 'the next step was sent without a fresh scan');
    assert.equal(running(), 1, 'the strip stopped saying it waits');
  } finally { await unmount(); }
});

test('a failed step stops the sequence with Continue on its chip and the reason under the strip; a reload offers Install, which installs only what is missing', async () => {
  const tracker = globalThis.bundleTracker;
  await mount(payload({ bundles: [webBundle(), dbBundle()] }));
  try {
    await startWebBundle();
    assert.equal(tracker.calls.length, 1);
    tracker.set({ operation: null, locked: false, failure: { message: 'nginx did not start', code: 'service_start_failed', component: 'nginx' },
      catalogSnapshot: fresh({ bundles: [webBundle(), dbBundle()] }) });
    for (let i = 0; i < 4; i += 1) await settle();
    assert.equal(tracker.calls.length, 1, 'the next component was installed after a failure');
    assert.equal(running(), 0);
    const chip = stoppedChip();
    assert.equal(chip.props['data-bundle-stop'], 'failed');
    assert.ok(label(chip).includes(say('services.bundles.continue')), label(chip));
    assert.ok(label(stoppedReason()).includes(say('services.bundles.stopped.failed', { component: nameOf('nginx') })), label(stoppedReason()));
  } finally { await unmount(); }

  // After a reload: Nginx is there, PHP-FPM is not. The bundle is offered;
  // nothing continues by itself, and Install installs only PHP-FPM.
  await mount(payload({ nginx: true, bundles: [webBundle({ missing_components: ['php-fpm'] }), dbBundle()] }));
  try {
    for (let i = 0; i < 3; i += 1) await settle();
    assert.equal(bundleNode('web.nginx_php').props['data-bundle-status'], 'available');
    assert.equal(tracker.calls.length, 0, 'the page continued by itself');
    await startWebBundle(1);
    assert.deepEqual(tracker.calls.map((call) => call.serviceId), ['php-fpm']);
  } finally { await unmount(); }
});

test('an operation that is not this step\'s (another screen\'s, adopted on a busy host) is never taken for the step', async () => {
  const tracker = globalThis.bundleTracker;
  await mount(payload({ bundles: [webBundle(), dbBundle()] }));
  try {
    // The tracker adopts someone else's running operation instead of ours.
    tracker.state.startInstall = async (request) => {
      tracker.calls.push(request);
      tracker.set({ operation: { id: 'other', service_id: 'mariadb', kind: 'service_install', status: 'running' }, locked: true });
      return true;
    };
    await startWebBundle();
    await settle();
    let chip = stoppedChip();
    assert.ok(chip, 'the strip still says it installs Nginx');
    assert.equal(chip.props['data-bundle-stop'], 'foreign');
    assert.ok(label(stoppedReason()).includes(say('services.bundles.stopped.foreign')), label(stoppedReason()));
    // That operation ends with a fresh scan: nothing more is sent.
    tracker.set({ operation: null, locked: false, catalogSnapshot: fresh({ nginx: true, bundles: [webBundle({ missing_components: ['php-fpm'] }), dbBundle()] }) });
    for (let i = 0; i < 3; i += 1) await settle();
    assert.equal(tracker.calls.length, 1, 'the next step was sent on another operation\'s end');
    chip = stoppedChip();
    assert.ok(label(chip).includes(say('services.bundles.continue')), 'no Continue');
  } finally { await unmount(); restoreTracker(); }
});

test('the change gate is read again before each step: a host that is not ready stops the sequence with Continue', async () => {
  const tracker = globalThis.bundleTracker;
  await mount(payload({ bundles: [webBundle(), dbBundle()] }));
  try {
    await startWebBundle();
    assert.deepEqual(tracker.calls.map((call) => call.serviceId), ['nginx']);
    tracker.set({ operation: { ...ours('nginx', 1) } });
    await settle();
    // While Nginx installs, the host stops accepting changes.
    readiness = { ready: false, code: 'HOST_MUTATION_BUSY', reason: 'package_manager_active' };
    tracker.set({ operation: null, locked: false, catalogSnapshot: fresh({ nginx: true, bundles: [webBundle({ missing_components: ['php-fpm'] }), dbBundle()] }) });
    for (let i = 0; i < 4; i += 1) await settle();
    assert.equal(tracker.calls.length, 1, 'the next step was sent through a closed gate');
    assert.equal(stoppedChip().props['data-bundle-stop'], 'gated');
    assert.ok(label(stoppedReason()).includes(say('services.bundles.stopped.gated', { reason: say('services.bundles.gatedReadiness') })), label(stoppedReason()));
    assert.ok(requests.filter((entry) => entry === 'GET /api/v1/host-mutation-readiness').length >= 2, 'the gate was not read again');
  } finally { await unmount(); readiness = { ready: true }; }
});

// --- alpha.105: versions in the bundle dialogue, what else it installs ---------------
// Owner decision 2026-10-24: recommend, warn, never impose.
function serveVersions(managed, { preview, repo = {}, candidate = {} }) {
  serve(managed);
  const plain = globalThis.fetch;
  const previews = [];
  globalThis.fetch = async (input, init = {}) => {
    const url = String(input);
    const [path, query = ''] = url.split('?');
    const params = new URLSearchParams(query);
    if (path === '/api/v1/service/candidate') return answer({ version: candidate[params.get('id')] ?? '' });
    if (path === '/api/v1/repo') return answer(repo[params.get('service_id')] ?? { available: false, enabled: false });
    if (path === '/api/v1/service/install-preview') {
      previews.push(params.getAll('step'));
      return answer(preview(params.getAll('step')));
    }
    return plain(input, init);
  };
  return previews;
}
const SURY = { available: true, enabled: true, name: 'Sury PHP (packages.sury.org)', packages: ['php8.3-fpm', 'php8.5-fpm'] };
const known = (notable = [], refusals = []) => ({ state: 'known', installs: [], notable, refusals, removes: [], requested: [] });
async function mountMariaDBBundle(options) {
  const managed = server('boston');
  const previews = serveVersions(managed, options);
  globalThis.bundleTracker.calls.length = 0;
  globalThis.bundleTracker.set({ operation: null, failure: null, catalogSnapshot: null, locked: false });
  await act(async () => { tree = Renderer.create(React.createElement(ServiceList, {})); });
  for (let i = 0; i < 3; i += 1) await settle();
  await press(bundleNode('db.mariadb'));
  for (let i = 0; i < 2; i += 1) await settle();
  return previews;
}
const confirmButton = (n) => tree.root.findAll((node) => node.type === 'button' && label(node) === say('services.bundles.dialog.confirm', { n }))[0];
const versionText = (id) => dotted(label(tree.root.find((node) => node.props && node.props['data-bundle-part'] !== undefined
  && label(node).startsWith(nameOf(id)))));

test('the bundle dialogue says the version each part brings, marks the distribution’s as recommended, and Change lists the component’s own versions', async () => {
  globalThis.bundleToasts = [];
  globalThis.document = { addEventListener() {}, removeEventListener() {}, getElementById: () => null, activeElement: null, body: { style: {} } };
  const previews = await mountMariaDBBundle({ preview: () => known(), repo: { 'php-fpm': SURY }, candidate: { 'php-fpm': '2:8.4+96', mariadb: '1:11.8.3-1', phpmyadmin: '4:5.2.2-really+dfsg-1+deb13u1' } });
  try {
    assert.ok(versionText('php-fpm').includes(say('services.bundles.version.recommended', { version: '8.4' })), versionText('php-fpm'));
    assert.ok(versionText('phpmyadmin').includes(say('services.bundles.version.recommended', { version: '5.2.2' })), versionText('phpmyadmin'));
    // PostgreSQL is installed on Boston and is not part of this bundle; Nginx is.
    assert.ok(versionText('nginx').includes('1.0 · already installed'), versionText('nginx'));
    // Only PHP-FPM offers other versions here.
    const changes = tree.root.findAll((node) => node.props && node.props['data-bundle-version-change'] !== undefined);
    assert.deepEqual(changes.map((node) => node.props['data-bundle-version-change']), ['php-fpm']);
    assert.ok(label(tree.root.find((node) => node.props && node.props['data-install-preview'] === 'clean')).includes(say('services.installPreview.nothingElse')));
    await press(changes[0]);
    const options = tree.root.findAll((node) => node.type === 'input' && node.props['data-bundle-version-option'] !== undefined);
    assert.deepEqual(options.map((node) => node.props['data-bundle-version-option']), ['default', 'php8.3-fpm', 'php8.5-fpm']);
    await act(async () => { options[1].props.onChange(); });
    for (let i = 0; i < 2; i += 1) await settle();
    assert.ok(versionText('php-fpm').includes(say('services.bundles.version.vendor', { version: '8.3', repo: SURY.name })), versionText('php-fpm'));
    assert.ok(label(tree.root.find((node) => node.props && node.props['data-bundle-tool-php'] === '8.3')).includes(say('services.bundles.version.runsOn', { version: '8.3' })));
    // The simulation is asked for the bundle's steps together, with the choice.
    assert.deepEqual(previews.at(-1), ['mariadb', 'php-fpm:php8.3-fpm', 'phpmyadmin::8.3']);
    // A distribution tool on another PHP is a warning, not a refusal.
    const warning = tree.root.findAll((node) => node.props && node.props['data-bundle-warning'] !== undefined);
    assert.equal(warning.length, 1);
    assert.ok(label(warning[0]).includes(say('services.bundles.warn.distroTool', { tool: 'phpMyAdmin', default: '8.4', version: '8.3' })), label(warning[0]));
    assert.equal(confirmButton(3).props.disabled, false);
    await press(confirmButton(3));
    const first = globalThis.bundleTracker.calls[0];
    assert.equal(first.serviceId, 'mariadb');
    assert.equal(first.checkInstalls, true);
    assert.deepEqual(first.confirmInstalls, []);
  } finally { await unmount(); }
});

test('notable installs are named with their reason and hold Install until the owner confirms them; the confirmation goes with every step', async () => {
  globalThis.bundleToasts = [];
  globalThis.document = { addEventListener() {}, removeEventListener() {}, getElementById: () => null, activeElement: null, body: { style: {} } };
  const tracker = globalThis.bundleTracker;
  await mountMariaDBBundle({
    preview: () => known([
      { package: 'apache2', reason: 'apache', component: 'apache' },
      { package: 'libapache2-mod-php8.5', reason: 'apache', component: 'apache' },
      { package: 'php8.5-common', reason: 'new_php_version', version: '8.5' },
    ]),
    candidate: { 'php-fpm': '2:8.4+96' },
  });
  try {
    const notes = tree.root.find((node) => node.props && node.props['data-install-preview'] === 'notable');
    // alpha.109: grouped, one package a line; packages sharing a reason under it.
    assert.ok(label(notes).includes('The Apache web server:apache2libapache2-mod-php8.5'), label(notes));
    assert.ok(label(notes).includes(`php8.5-common: ${say('services.notable.why.new_php_version', { version: '8.5' })}`), label(notes));
    assert.equal(confirmButton(3).props.disabled, true, 'Install was offered before the owner confirmed');
    const box = tree.root.find((node) => node.type === 'input' && node.props['data-install-preview-confirm'] !== undefined);
    await act(async () => { box.props.onChange({ target: { checked: true } }); });
    await settle();
    assert.equal(confirmButton(3).props.disabled, false);
    await press(confirmButton(3));
    assert.deepEqual(tracker.calls[0].confirmInstalls, ['apache2', 'libapache2-mod-php8.5', 'php8.5-common']);
    tracker.set({ operation: null, locked: false, catalogSnapshot: (() => {
      const after = server('boston');
      after.services = after.services.map((item) => (item.id === 'mariadb' ? { ...item, is_installed: true, status: 'active (running)' } : item));
      return after;
    })() });
    for (let i = 0; i < 3; i += 1) await settle();
    assert.deepEqual(tracker.calls.map((call) => call.serviceId), ['mariadb', 'php-fpm']);
    assert.deepEqual(tracker.calls[1].confirmInstalls, ['apache2', 'libapache2-mod-php8.5', 'php8.5-common']);
  } finally { await unmount(); }
});

test('a chosen PHP version that cannot serve the tool is refused with what it lacks; a read that failed says so and does not hold Install', async () => {
  globalThis.bundleToasts = [];
  globalThis.document = { addEventListener() {}, removeEventListener() {}, getElementById: () => null, activeElement: null, body: { style: {} } };
  let failing = false;
  await mountMariaDBBundle({
    preview: (steps) => (failing ? { state: 'unknown', installs: [], notable: [], refusals: [], removes: [], requested: [] }
      : steps.includes('phpmyadmin::8.5') ? known([], [{ id: 'phpmyadmin', code: 'php_extensions_unavailable', version: '8.5', packages: ['php8.5-mysql'] }]) : known()),
    repo: { 'php-fpm': SURY },
    candidate: { 'php-fpm': '2:8.4+96' },
  });
  try {
    await press(tree.root.find((node) => node.props && node.props['data-bundle-version-change'] === 'php-fpm'));
    const options = tree.root.findAll((node) => node.type === 'input' && node.props['data-bundle-version-option'] !== undefined);
    await act(async () => { options[2].props.onChange(); });
    for (let i = 0; i < 2; i += 1) await settle();
    const refusal = tree.root.find((node) => node.props && node.props['data-bundle-refusal'] === 'php_extensions_unavailable');
    assert.ok(label(refusal).includes(say('services.bundles.refusal.php_extensions_unavailable', { tool: 'phpMyAdmin', version: '8.5', packages: 'php8.5-mysql' })), label(refusal));
    assert.equal(confirmButton(3).props.disabled, true);
    // Back to the recommended version, with the read failing this time.
    failing = true;
    await act(async () => { options[0].props.onChange(); });
    for (let i = 0; i < 2; i += 1) await settle();
    assert.ok(dialogText().includes(say('services.installPreview.unknown', { why: '' })), dialogText());
    assert.equal(confirmButton(3).props.disabled, false);
  } finally { await unmount(); }
});

// --- alpha.118: the owner's Boston screens (Ubuntu 24.04, alpha.117) ------------------
// The installed view (first since alpha.100) hid the PostgreSQL row, and the
// bundle (PostgreSQL alone until alpha.101) became "PostgreSQL + phpPgAdmin":
// the bundle was the only database offer. Then PostgreSQL was removed and
// phpPgAdmin stayed "Installed" without it.
// Kurulu görünüm PostgreSQL satırını gizliyordu; tek teklif paketti.
const PG_BUNDLE = ['postgresql', 'nginx', 'php-fpm', 'phppgadmin'];
const pgBundle = (extra) => bundle('db.postgresql', 'database', PG_BUNDLE, { missing_components: ['postgresql', 'phppgadmin'], ...extra });
function databaseServer({ postgresql = false, phppgadmin = false, needs } = {}) {
  const answer = payload({ nginx: true, phpfpm: true, bundles: [pgBundle(), dbBundle()] });
  answer.services.push(
    component('postgresql', 'database', postgresql, { name: 'PostgreSQL' }),
    component('phppgadmin', 'database', phppgadmin, phppgadmin ? { kind: 'tool', status: 'installed', ...(needs ? { needs_missing: needs } : {}) } : { kind: 'tool', requires_missing: ['postgresql'] }),
  );
  return answer;
}
const pgImpact = {
  service_id: 'postgresql', removal: 'packages', has_units: true, setup_required: false, package_family: 'apt', packages: ['postgresql'],
  all_versions: true, firewall_ports: [], mail_filter_rewired: false, database_tools_reconfigured: false,
  required_by: ['phppgadmin'], companion_tools: ['phppgadmin'], bundles: [], hosted: null, simulation: null,
};
const rowOf = (id) => tree.root.findAll((node) => node.type === 'li' && node.props['data-component-row'] === ''
  && node.findAll((child) => child.type === 'a' && label(child) === nameOf(id)).length > 0)[0];
const posts = () => urls.filter((call) => call.method === 'POST' && call.url === '/api/v1/service/uninstall').map((call) => JSON.parse(call.body).service_id);

// alpha.120 (owner's Boston screens, alpha.119): "why is the PostgreSQL server
// component not like MariaDB? it looks like a side thing". alpha.122 (owner,
// Boston, alpha.121: "why can't phpPgAdmin and phpMyAdmin be installed
// separately?"): one list. Every offered component has its own row in its
// category, installed ones first; one not installed reads Not installed with
// Install, or says in one line why it cannot be installed here.
// Tek liste: her bileşenin kendi satırı; kurulu olmayan Kur ya da nedenini söyler.
const installIn = (row) => row.findAll((node) => node.type === 'button' && label(node) === say('services.install'))[0];
const rowNames = () => tree.root.findAll((node) => node.type === 'li' && node.props['data-component-row'] === '')
  .map((row) => label(row.findAll((child) => child.type === 'a')[0]));
const sentPosts = () => urls.filter((call) => call.method === 'POST' && !call.url.startsWith('/api/v1/managed-services/scan')).length;

test('PostgreSQL that is not installed has its own row like MariaDB: Not installed and Install, which opens its own install dialog with its versions', async () => {
  await mount(databaseServer());
  try {
    const row = rowOf('postgresql');
    assert.ok(row, 'the list shows the PostgreSQL row');
    assert.ok(label(row).includes(say('services.notInstalled')), label(row));
    assert.ok(rowOf('mariadb'), 'MariaDB keeps its row');
    assert.equal(tree.root.findAll((node) => node.props && node.props['data-bundle-alone-install'] !== undefined).length, 0, 'no "install one on its own" link');
    assert.ok(bundleNode('db.postgresql'), 'the ready bundles stay');
    // phpPgAdmin has its row too, and says what it needs first.
    assert.ok(label(rowOf('phppgadmin')).includes(say('services.requiresLabel', { names: nameOf('postgresql') })), label(rowOf('phppgadmin')));
    assert.equal(installIn(rowOf('phppgadmin')), undefined, 'no Install while PostgreSQL is missing');
    await press(installIn(row));
    assert.ok(dialogText().includes(say('services.installTitle', { name: nameOf('postgresql') })), dialogText());
    // The version choice is the install dialog's own: it asks for PostgreSQL's repository and candidate.
    assert.ok(urls.some((call) => call.url === '/api/v1/repo?service_id=postgresql'), urls.map((call) => call.url).join(', '));
    assert.ok(urls.some((call) => call.url === '/api/v1/service/candidate?id=postgresql'));
    assert.equal(sentPosts(), 0, 'opening the dialog sends nothing');
  } finally { await unmount(); }
});

// The owner's question itself: both database servers are installed, so both
// browser tools are offered on their own rows.
const bothDatabases = () => {
  const answer = databaseServer({ postgresql: true });
  answer.bundles = [pgBundle({ missing_components: ['phppgadmin'] }), dbBundle({ status: 'available', action: 'install', missing_components: ['phpmyadmin'] })];
  answer.services = answer.services.map((item) => (item.id === 'phppgadmin' ? { ...item, requires_missing: undefined }
    : item.id === 'phpmyadmin' ? component('phpmyadmin', 'database', false, { kind: 'tool' }) : item));
  return answer;
};

test('alpha.122: with PostgreSQL and MariaDB installed, phpPgAdmin and phpMyAdmin each have a Not installed row with Install, after the installed rows', async () => {
  await mount(bothDatabases());
  try {
    for (const id of ['phppgadmin', 'phpmyadmin']) {
      const row = rowOf(id);
      assert.ok(row, `${id} has its own row`);
      assert.ok(label(row).includes(say('services.notInstalled')), label(row));
      assert.ok(installIn(row), `${id} offers Install`);
    }
    // Installed first within the category.
    const names = [nameOf('mariadb'), nameOf('postgresql'), nameOf('phpmyadmin'), nameOf('phppgadmin')];
    const order = rowNames().filter((name) => names.includes(name));
    assert.deepEqual(order.slice(0, 2).sort(), [nameOf('mariadb'), nameOf('postgresql')].sort(), order.join(', '));
    // phpPgAdmin's Install opens its own dialog; nothing is sent.
    await press(installIn(rowOf('phppgadmin')));
    assert.ok(dialogText().includes(say('services.installTitle', { name: nameOf('phppgadmin') })), dialogText());
    assert.equal(sentPosts(), 0);
  } finally { await unmount(); }
});

test('alpha.122: a bundle some of whose parts are here names what it adds; one with nothing here says Install', async () => {
  await mount(bothDatabases());
  try {
    assert.equal(label(bundleNode('db.postgresql')), `PostgreSQL + phpPgAdmin·${say('services.bundles.chip.addNamed', { names: nameOf('phppgadmin') })}`);
    assert.equal(label(bundleNode('db.mariadb')), `MariaDB + phpMyAdmin·${say('services.bundles.chip.addNamed', { names: nameOf('phpmyadmin') })}`);
    assert.equal(bundleNode('db.postgresql').findAll((node) => node.props && node.props['data-bundle-adds'] === 'phppgadmin').length, 1);
  } finally { await unmount(); }
  await mount(payload({ bundles: [webBundle()] }));
  try {
    assert.equal(label(bundleNode('web.nginx_php')), `Nginx + PHP-FPM·${say('services.bundles.chip.install')}`);
  } finally { await unmount(); }
  // Both languages have the words; Turkish puts no suffix on a name.
  assert.equal(words['services.bundles.chip.addNamed'], 'Add {names}');
  assert.match(source('i18n/screens/components/tr.ts'), /'services\.bundles\.chip\.addNamed': 'Ekle: \{names\}',/);
});

test('alpha.122: Fail2ban, ClamAV and vsftpd are listed as not installed with Install; the count says how many are installed', async () => {
  const answer = databaseServer();
  answer.services.push(component('fail2ban', 'security', false), component('clamav', 'security', false), component('vsftpd', 'ftp', false));
  await mount(answer);
  try {
    for (const id of ['fail2ban', 'clamav', 'vsftpd']) {
      assert.ok(rowOf(id), `${id} has a row`);
      assert.ok(label(rowOf(id)).includes(say('services.notInstalled')), label(rowOf(id)));
      assert.ok(installIn(rowOf(id)), `${id} offers Install`);
    }
    const count = tree.root.findAll((node) => node.props && node.props['data-component-count'] !== undefined)[0];
    const installed = answer.services.filter((item) => item.is_installed === true).length;
    assert.equal(label(count), say('services.countInstalled', { total: answer.services.length, installed }));
  } finally { await unmount(); }
});

test('a component another one excludes or the host does not offer keeps its row and says why in one line, without Install', async () => {
  const answer = databaseServer();
  answer.bundles = [pgBundle({ status: 'blocked', action: 'none', blocked_code: 'component_not_installable', blocked_vars: { component: 'phpPgAdmin' } }), dbBundle()];
  answer.services.push(component('apache', 'web', false, { name: 'Apache', conflict_with: 'Nginx' }));
  await mount(answer);
  try {
    assert.ok(installIn(rowOf('postgresql')), 'PostgreSQL offers Install on its row');
    const apache = rowOf('apache');
    assert.ok(apache, 'Apache beside Nginx keeps its row');
    assert.ok(label(apache).includes(say('services.conflictWith', { name: 'Nginx' })), label(apache));
    assert.equal(apache.findAll((node) => node.props && node.props['data-row-conflict'] === 'Nginx').length, 1);
    assert.equal(installIn(apache), undefined, 'no Install beside Nginx');
  } finally { await unmount(); }
  const notOffered = databaseServer();
  notOffered.services = notOffered.services.map((item) => (item.id === 'postgresql' ? { ...item, not_offered: true, not_offered_reason: 'not on this system' } : item));
  await mount(notOffered);
  try {
    assert.ok(label(rowOf('postgresql')).includes(say('services.notOffered')), label(rowOf('postgresql')));
    assert.equal(installIn(rowOf('postgresql')), undefined);
  } finally { await unmount(); }
});

test('alpha.122: a mail engine that is not installed points to the mail setup; Exim beside Postfix says the conflict', async () => {
  const answer = server('boston');
  answer.services.push(component('exim', 'email', false, { name: 'Exim', conflict_with: 'Postfix' }));
  await mount(answer);
  try {
    for (const id of ['postfix', 'dovecot', 'rspamd', 'roundcube']) {
      const row = rowOf(id);
      assert.ok(row, `${id} has a row`);
      assert.equal(row.findAll((node) => node.props && node.props['data-row-mail-setup'] !== undefined).length, 1, `${id} points to the mail setup`);
      assert.equal(installIn(row), undefined, `${id} does not install one part of a mail setup alone`);
    }
    assert.ok(label(rowOf('exim')).includes(say('services.conflictWith', { name: 'Postfix' })), label(rowOf('exim')));
    // PHP-FPM is in Webmail too, but it is not a mail engine: its row keeps Install.
    assert.equal(rowOf('php-fpm').findAll((node) => node.props && node.props['data-row-mail-setup'] !== undefined).length, 0);
    assert.ok(installIn(rowOf('php-fpm')), 'PHP-FPM keeps Install');
  } finally { await unmount(); }
});

test('alpha.122: ?component=<id>&install= opens that component\'s install dialog, sends nothing and leaves the address; a blocked one says why', async () => {
  globalThis.bundleSearch = '?component=phppgadmin&install=';
  globalThis.bundleNavigated = [];
  globalThis.bundleNavigateOptions = [];
  await mount(bothDatabases());
  try {
    assert.ok(dialogText().includes(say('services.installTitle', { name: nameOf('phppgadmin') })), dialogText());
    assert.equal(sentPosts(), 0, 'nothing starts before the owner confirms');
    assert.deepEqual(globalThis.bundleNavigated, ['/components?component=phppgadmin']);
    assert.deepEqual(globalThis.bundleNavigateOptions, [{ replace: true }]);
    assert.ok(rowOf('phppgadmin'), 'the search shows the linked row');
  } finally { await unmount(); }
  globalThis.bundleSearch = '?component=apache&install=';
  globalThis.bundleNavigated = [];
  const blocked = databaseServer();
  blocked.services.push(component('apache', 'web', false, { name: 'Apache', conflict_with: 'Nginx' }));
  await mount(blocked);
  try {
    assert.equal(tree.root.findAll((node) => node.props && node.props.role === 'dialog').length, 0, 'no dialog for a blocked component');
    assert.deepEqual(globalThis.bundleToasts.map(([, text]) => text), [say('services.installLinkBlocked', { name: nameOf('apache') })]);
    assert.equal(sentPosts(), 0);
  } finally { await unmount(); delete globalThis.bundleSearch; delete globalThis.bundleNavigated; delete globalThis.bundleNavigateOptions; }
});

test('no security emergency section in the list; an open emergency puts a short red badge on its row that links to it', async () => {
  await mount(databaseServer());
  try {
    assert.equal(tree.root.findAll((node) => node.props && node.props['data-security-emergency'] !== undefined).length, 0, 'no section in the list');
    assert.ok(!label(rowOf('mariadb')).includes(say('emg.rowBadge')), 'no badge while nothing is open');
    assert.ok(urls.some((call) => call.method === 'GET' && call.url === '/api/v1/security-emergencies'), 'the list is read');
  } finally { await unmount(); }
  const open = { emergency: { id: 'e1', component: 'mariadb', version: '', name: 'MariaDB', reason: 'CVE-2026-0003', stage: 'withdrawn', withdrawn: true },
    guidance: { result: '', reason: '', who_acts: 'administrator', next_action: '', resumes: '' }, operation: null,
    preview: { sites: [], customers: [], resellers: [], offered_by: [], notes: [] },
    notices: { total: 0, seen: 0, mail_sent: 0, mail_pending: 0, mail_failed: 0, no_address: 0, mail_not_available: 0 }, confirm_name: 'MariaDB', alternatives: [], can: {} };
  extraRoutes = { 'GET /api/v1/security-emergencies': [{ emergencies: [open] }] };
  await mount(databaseServer());
  try {
    assert.ok(label(rowOf('mariadb')).includes(say('emg.rowBadge')), label(rowOf('mariadb')));
    assert.ok(!label(rowOf('postgresql')).includes(say('emg.rowBadge')), 'only the component whose emergency is open');
    assert.equal(tree.root.findAll((node) => node.props && node.props['data-security-emergency'] !== undefined).length, 0);
    assert.equal(urls.filter((call) => call.method === 'POST' && !call.url.startsWith('/api/v1/managed-services/scan')).length, 0, 'reading sends nothing');
  } finally { await unmount(); extraRoutes = {}; }
  const list = source('components/ServiceList.tsx');
  assert.match(list, /<Link to=\{emergencyPath\(s\.id\)\}[^>]*data-row-emergency=\{s\.id\}>/);
  assert.match(source('lib/securityEmergency.ts'), /return `\/components\/\$\{serviceId\}#\$\{EMERGENCY_ANCHOR\}`;/);
});

test("Python, the distribution's own runtime, shows its package version and its one version", async () => {
  const answer = databaseServer();
  answer.services.push(component('python', 'web', true, {
    kind: 'runtime', status: 'installed', versions: [], packages: ['python3', 'python3-venv'],
    instances: [{ version: '3.12', path: '/usr/bin/python3.12', managed: false }],
    package_version: '3.12.3-0ubuntu2', package_version_short: '3.12.3', package_version_read_at: '2026-11-04T08:00:00Z',
    package_versions: [{ package: 'python3', version: '3.12.3-0ubuntu2' }, { package: 'python3-venv', version: '3.12.3-0ubuntu2' }],
  }));
  await mount(answer);
  try {
    const row = rowOf('python');
    assert.ok(label(row).includes('3.12.3'), label(row));
    const toggle = row.findAll((node) => node.type === 'button' && label(node).startsWith(say('services.versionsToggle')))[0];
    assert.equal(label(toggle), say('services.versionsToggle') + '1', 'Versions counts the one Python found, not 0');
  } finally { await unmount(); }
});

test('an installed phpPgAdmin without PostgreSQL says it cannot work and offers install PostgreSQL or remove phpPgAdmin; nothing is sent', async () => {
  await mount(databaseServer({ phppgadmin: true, needs: ['postgresql'] }));
  try {
    const notice = tree.root.find((node) => node.props && node.props['data-tool-needs'] === 'postgresql');
    assert.ok(label(notice).includes(say('services.toolNeeds', { name: nameOf('phppgadmin'), needs: nameOf('postgresql') })), label(notice));
    assert.match(notice.props.className, /\brelative\b/, 'the actions sit above the row link');
    await press(notice.find((node) => node.props['data-tool-needs-install'] === 'postgresql'));
    assert.ok(dialogText().includes(say('services.installTitle', { name: nameOf('postgresql') })), dialogText());
    assert.equal(posts().length, 0);
  } finally { await unmount(); }
  extraRoutes = { 'GET /api/v1/service/removal-impact': [{ ...pgImpact, service_id: 'phppgadmin', packages: ['phppgadmin'], all_versions: false, required_by: [], companion_tools: [], database_tools_reconfigured: true }] };
  await mount(databaseServer({ phppgadmin: true, needs: ['postgresql'] }));
  try {
    await press(tree.root.find((node) => node.props && node.props['data-tool-needs-remove'] === ''));
    assert.ok(dialogText().includes(say('services.uninstallTitle', { name: nameOf('phppgadmin') })), dialogText());
    assert.ok(urls.some((call) => call.url === '/api/v1/service/removal-impact?service_id=phppgadmin'));
    assert.equal(posts().length, 0, 'the removal waits for the confirmation');
  } finally { await unmount(); extraRoutes = {}; }
  // A tool with everything it needs says nothing.
  await mount(databaseServer({ postgresql: true, phppgadmin: true }));
  try {
    assert.equal(tree.root.findAll((node) => node.props && node.props['data-tool-needs'] !== undefined).length, 0);
  } finally { await unmount(); }
});

async function removePostgreSQL({ keepTool }) {
  extraRoutes = {
    'GET /api/v1/service/removal-impact': [pgImpact],
    'POST /api/v1/service/uninstall': [{ success: true }],
  };
  await mount(databaseServer({ postgresql: true, phppgadmin: true }));
  await press(rowOf('postgresql').find((node) => node.type === 'button' && node.props.title === say('services.uninstall')));
  await settle();
  const choice = tree.root.find((node) => node.props && node.props['data-removal-companions'] === 'phppgadmin');
  assert.ok(label(choice).includes(say('services.removal.alsoTools', { name: nameOf('postgresql'), tools: nameOf('phppgadmin') })), label(choice));
  const box = choice.find((node) => node.type === 'input');
  assert.equal(box.props.checked, true, 'ticked by default');
  assert.equal(posts().length, 0);
  if (keepTool) await act(async () => { box.props.onChange({ target: { checked: false } }); });
  const dialog = tree.root.findAll((node) => node.props && node.props.role === 'dialog')[0];
  await press(dialog.findAll((node) => node.type === 'button' && label(node) === say('services.uninstall'))[0]);
  await settle();
  return posts();
}

test('removing PostgreSQL offers phpPgAdmin, ticked, and removes it after PostgreSQL', async () => {
  try {
    assert.deepEqual(await removePostgreSQL({ keepTool: false }), ['postgresql', 'phppgadmin']);
  } finally { await unmount(); extraRoutes = {}; }
});

test('removing PostgreSQL with phpPgAdmin unticked keeps phpPgAdmin', async () => {
  try {
    assert.deepEqual(await removePostgreSQL({ keepTool: true }), ['postgresql']);
  } finally { await unmount(); extraRoutes = {}; }
});
