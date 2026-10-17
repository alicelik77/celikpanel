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
import { en } from '../src/i18n/en.ts';
import { enScreens } from '../src/i18n/screens/en.ts';
import { enServerScreens } from '../src/i18n/screens/server/en.ts';

// The page speaks English from the real catalogue, so a bundle is named by
// its words and never by a fallback.
const words = { ...en, ...enScreens, ...enServerScreens };
globalThis.bundleWords = words;
const say = (key, vars = {}) => Object.entries(vars).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), words[key] ?? key);
const nameOf = (id) => words[`services.component.name.${id}`] ?? id;

const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const icons = new Set(['AlertTriangle']);
for (const path of ['components/ServiceList.tsx', 'components/ReadyBundleStrip.tsx', 'lib/componentIcons.tsx']) {
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
  export const useNavigate = () => (path) => { globalThis.bundleNavigated?.push(path); return true; };
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

// --- What the Panel answers -------------------------------------------------------
const component = (id, category, installed, extra = {}) => ({
  id, name: id, description: `${id} on this server`, icon: '', category,
  status: installed ? 'active (running)' : 'not_installed', is_installed: installed, versions: installed ? ['1.0'] : [], ...extra,
});
const profile = (id) => ({ id, name: id, description: `${id} profile`, status: 'available', available: true, verified: false, latest_attempt_status: 'none', services: ['postfix'] });
const bundle = (id, category, components, extra = {}) => ({
  id, category, name_code: id.replace(/\./g, '_'), components, status: 'available', action: 'install',
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
      component('postfix', 'email', false),
      component('rspamd', 'email', false),
    ],
    bundles,
  };
}
const webBundle = (extra) => bundle('web.nginx_php', 'web', ['nginx', 'php-fpm'], extra);
const dbBundle = (extra) => bundle('db.mariadb', 'database', ['mariadb'], { status: 'installed_verified', action: 'none', missing_components: [], ...extra });

const originalFetch = globalThis.fetch;
// What /host-mutation-readiness answers; a test may close it.
let readiness = { ready: true };
const originalStart = globalThis.bundleTracker.state.startInstall;
const restoreTracker = () => { globalThis.bundleTracker.state.startInstall = originalStart; };
let tree;
let requests;
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
function serve(managed) {
  requests = [];
  globalThis.fetch = async (input, init = {}) => {
    const path = String(input).split('?')[0];
    requests.push(`${init.method || 'GET'} ${path}`);
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

test('an installed and verified component bundle is a check-mark chip; its dialogue says so and offers Verify, never Repair', async () => {
  await mount(payload({ nginx: true, phpfpm: true, bundles: [webBundle({ status: 'installed_verified', action: 'none', missing_components: [] }), dbBundle()] }));
  try {
    const chip = bundleNode('db.mariadb');
    assert.equal(chip.props['data-bundle-state'], 'verified');
    assert.equal(label(chip), 'MariaDB');
    assert.equal(chip.props['aria-label'], 'MariaDB bundle, installed and verified');
    // No Repair link on the strip.
    assert.ok(!stripText('database').includes(say('services.bundles.repairEllipsis')) && !stripText('database').includes('Repair'), stripText('database'));
    await press(chip);
    assert.equal(installedState(), 'verified');
    assert.ok(dialogText().includes(say('services.bundles.installed.verified', { name: 'MariaDB bundle' })), dialogText());
    // Described, not told to install (alpha.95 review).
    assert.ok(dialogText().includes(say('services.bundles.about.db_mariadb')), dialogText());
    assert.ok(!dialogText().includes(say('services.bundles.description.db_mariadb')), dialogText());
    const buttons = tree.root.findAll((node) => node.type === 'button' && node.props && node.props.role !== 'dialog').map(label);
    assert.ok(buttons.includes(say('services.bundles.verify')), buttons.join(', '));
    assert.ok(!buttons.includes(say('services.bundles.repairEllipsis')), 'a component bundle offers Repair');
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
    assert.ok(!buttons.includes(say('services.bundles.verify')), 'a mail bundle offers Verify');
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

test('Verify of a component bundle only reads: usable while the host holds changes for another reason, held with its reason while an operation runs', async () => {
  const stopped = payload({ bundles: [webBundle(), dbBundle({ status: 'installed_unverified', action: 'verify' })] });
  stopped.services = stopped.services.map((item) => (item.id === 'mariadb' ? { ...item, status: 'inactive (dead)' } : item));
  readiness = { ready: false, code: 'HOST_MUTATION_BUSY', reason: 'package_manager_active' };
  await mount(stopped);
  try {
    assert.equal(bundleNode('db.mariadb').props['data-bundle-state'], 'unverified');
    await press(bundleNode('db.mariadb'));
    const verify = tree.root.findAll((node) => node.type === 'button' && label(node) === say('services.bundles.verify'))[0];
    assert.ok(verify && !verify.props.disabled, 'Verify is off while changes are held');
    const before = requests.filter((line) => line === 'POST /api/v1/managed-services/scan').length;
    await press(verify);
    assert.equal(requests.filter((line) => line === 'POST /api/v1/managed-services/scan').length, before + 1, 'Verify did not read again');
    // An install is held: the offer is off.
    assert.equal(bundleNode('web.nginx_php').props.disabled, true);
  } finally { await unmount(); readiness = { ready: true }; }

  // While a component operation runs, a rescan would be refused as busy:
  // Verify is not offered, and says why (alpha.94 review item 2).
  await mount(stopped);
  try {
    globalThis.bundleTracker.set({ operation: ours('nginx', 9), locked: true });
    await settle();
    await press(bundleNode('db.mariadb'));
    const verify = tree.root.findAll((node) => node.type === 'button' && label(node) === say('services.bundles.verify'))[0];
    assert.equal(verify.props.disabled, true);
    assert.equal(verify.props.title, say('services.bundles.verifyBusy'));
    assert.ok(dialogText().includes(say('services.bundles.verifyBusy')), dialogText());
    const before = requests.filter((line) => line === 'POST /api/v1/managed-services/scan').length;
    await act(async () => { await verify.props.onClick?.(); });
    assert.equal(requests.filter((line) => line === 'POST /api/v1/managed-services/scan').length, before, 'a held Verify read again');
  } finally { await unmount(); }
});

test('under Apache the add-on says what it installs, what is installed and what is not needed, and never calls Nginx installed', async () => {
  const apache = payload({ bundles: [
    dbBundle({ addons: ['db.mariadb.admin'] }),
    bundle('db.mariadb.admin', 'database', ['nginx', 'php-fpm', 'phpmyadmin'], { parent: 'db.mariadb', missing_components: ['phpmyadmin'] }),
  ], phpfpm: true });
  apache.services.push(component('apache', 'web', true), component('phpmyadmin', 'database', false, { kind: 'tool' }));
  await mount(apache);
  try {
    assert.equal(label(bundleNode('db.mariadb.admin')), `phpMyAdmin·${say('services.bundles.chip.add')}`);
    await press(bundleNode('db.mariadb.admin'));
    const parts = Object.fromEntries(tree.root.findAll((node) => node.props && node.props['data-bundle-part'] !== undefined)
      .map((node) => [label(node).replace(/(Will be installed|Already installed|Not needed)$/, ''), node.props['data-bundle-part']]));
    assert.deepEqual(parts, { [nameOf('nginx')]: 'notNeeded', [nameOf('php-fpm')]: 'installed', [nameOf('phpmyadmin')]: 'install' });
    assert.ok(tree.root.findAll((node) => node.type === 'button' && label(node) === say('services.bundles.dialog.confirm', { n: 1 })).length === 1);
  } finally { await unmount(); }
});

test('the installed view of a server with nothing installed still draws the strips', async () => {
  globalThis.localStorage = { getItem: (key) => (key === 'celikpanel.components.view' ? 'installed' : null), setItem() {}, removeItem() {} };
  const empty = payload({ bundles: [webBundle(), bundle('db.mariadb', 'database', ['mariadb'])] });
  empty.services = empty.services.map((item) => ({ ...item, is_installed: false, status: 'not_installed', versions: [] }));
  await mount(empty);
  try {
    assert.ok(label(tree.root).includes(say('services.noneInstalledYet')), 'the empty view no longer says nothing is installed');
    assert.ok(bundleNode('web.nginx_php') && bundleNode('db.mariadb'), 'the strips are not drawn in the installed view');
  } finally { await unmount(); delete globalThis.localStorage; }
});

// --- The owner's two servers ----------------------------------------------------------
// Payloads shaped like the owner's screens of 2026-10-17: the same registry on
// both, different states. Boston: PostgreSQL and Nginx, no mail, no MariaDB,
// no PHP-FPM. Frankfurt: everything installed and verified.
const registry = () => [
  { ...bundle('mail.core', 'email', ['postfix', 'dovecot'], { addons: ['mail.protected'] }), action_route: 'mail_profile', profile_id: 'core-mail' },
  { ...bundle('mail.webmail', 'email', ['postfix', 'dovecot', 'nginx', 'php-fpm', 'roundcube'], { addons: ['mail.protected'] }), action_route: 'mail_profile', profile_id: 'webmail' },
  { ...bundle('mail.protected', 'email', ['postfix', 'dovecot', 'rspamd']), action_route: 'mail_profile', profile_id: 'protected-mail' },
  bundle('db.postgresql', 'database', ['postgresql'], { addons: ['db.postgresql.admin'] }),
  bundle('db.mariadb', 'database', ['mariadb'], { addons: ['db.mariadb.admin'] }),
  bundle('db.postgresql.admin', 'database', ['nginx', 'php-fpm', 'phppgadmin'], { parent: 'db.postgresql' }),
  bundle('db.mariadb.admin', 'database', ['nginx', 'php-fpm', 'phpmyadmin'], { parent: 'db.mariadb' }),
  bundle('web.nginx_php', 'web', ['nginx', 'php-fpm']),
];
const verifiedBundle = (item) => ({ ...item, status: 'installed_verified', action: item.action_route === 'mail_profile' ? 'repair' : 'none', missing_components: [] });
function server(name) {
  const installed = name === 'boston'
    ? new Set(['nginx', 'postgresql'])
    : new Set(['postfix', 'dovecot', 'rspamd', 'nginx', 'php-fpm', 'roundcube', 'postgresql', 'mariadb', 'phppgadmin', 'phpmyadmin']);
  const services = [
    ['postfix', 'email'], ['dovecot', 'email'], ['rspamd', 'email'], ['roundcube', 'email', { kind: 'tool' }],
    ['nginx', 'web'], ['php-fpm', 'web', { kind: 'runtime' }],
    ['postgresql', 'database'], ['mariadb', 'database'], ['phppgadmin', 'database', { kind: 'tool' }], ['phpmyadmin', 'database', { kind: 'tool' }],
  ].map(([id, category, extra]) => component(id, category, installed.has(id), extra));
  const bundles = registry().map((item) => {
    if (name === 'frankfurt') return verifiedBundle(item);
    switch (item.id) {
    case 'db.postgresql': return verifiedBundle(item);
    case 'db.postgresql.admin': return { ...item, missing_components: ['php-fpm', 'phppgadmin'] };
    case 'db.mariadb.admin': return { ...item, status: 'blocked', action: 'none', missing_components: ['php-fpm', 'phpmyadmin'], blocked_code: 'component_requires', blocked_vars: { component: 'phpMyAdmin', required: 'MariaDB' } };
    case 'web.nginx_php': return { ...item, missing_components: ['php-fpm'] };
    default: return item;
    }
  });
  return { ...payload({ bundles }), services };
}

test('Boston and Frankfurt draw the same chips in the same order; MariaDB is offered on Boston, Core Mail is included on Frankfurt', async () => {
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
    ['db.postgresql', 'verified', 'PostgreSQL'],
    ['db.mariadb', 'available', `MariaDB·${say('services.bundles.chip.install')}`],
    ['db.postgresql.admin', 'available', `phpPgAdmin·${say('services.bundles.chip.add')}`],
    ['db.mariadb.admin', 'blocked', 'phpMyAdmin·needs MariaDB'],
  ]);
  assert.deepEqual(seen.frankfurt.email, [
    ['mail.core', 'included', `Core Mail·${say('services.bundles.chip.included')}`],
    ['mail.webmail', 'verified', 'Webmail'],
    ['mail.protected', 'verified', 'Spam-Protected Mail'],
  ]);
  // A blocked chip says its full reason on press (keyboard and touch), and installs nothing.
  await mount(server('boston'));
  try {
    const blocked = bundleNode('db.mariadb.admin');
    assert.equal(blocked.type, 'button');
    assert.equal(blocked.props['aria-expanded'], false);
    await press(blocked);
    const reason = tree.root.find((node) => node.props && node.props['data-bundle-blocked-reason'] === 'db.mariadb.admin');
    assert.equal(reason.props.id, blocked.props['aria-controls']);
    assert.ok(label(reason).includes(say('services.mailProfiles.code.component_requires', { component: 'phpMyAdmin', required: 'MariaDB' })), label(reason));
    assert.equal(bundleNode('db.mariadb.admin').props['aria-expanded'], true);
    assert.equal(globalThis.bundleTracker.calls.length, 0);
    assert.equal(tree.root.findAll((node) => node.props && node.props.role === 'dialog').length, 0, 'a blocked chip opened a dialogue');
  } finally { await unmount(); }
  // The included chip's dialogue names the bundle that contains it and offers no Repair of its own.
  await mount(server('frankfurt'));
  try {
    await press(bundleNode('mail.core'));
    assert.equal(installedState(), 'included');
    assert.ok(dialogText().includes(say('services.bundles.installed.included', { name: 'Core Mail', other: 'Webmail' })), dialogText());
    assert.ok(!tree.root.findAll((node) => node.type === 'button').map(label).includes(say('services.bundles.repairEllipsis')));
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
  // A failed webmail attempt is such a record: an install did not finish.
  const failed = withRoundcube();
  failed.profiles = failed.profiles.map((item) => (item.id === 'webmail' ? { ...item, latest_attempt_status: 'failed', latest_attempt_error: 'roundcube download failed' } : item));
  await mount(failed);
  try {
    assert.equal(label(leftoversNote()), say('services.cleanupWebmailRowNote.install'));
    assert.ok(leftoversButton());
  } finally { await unmount(); }
  // Roundcube's own page opens this page with ?cleanup=roundcube: the dialogue
  // opens once, whatever the row offers.
  globalThis.bundleSearch = '?cleanup=roundcube';
  await mount(withRoundcube());
  try {
    assert.ok(tree.root.findAll((node) => node.props && node.props['data-webmail-cleanup-list'] !== undefined).length === 1, 'the cleanup link did not open the dialogue');
    assert.equal(leftoversButton(), undefined);
  } finally { await unmount(); delete globalThis.bundleSearch; }
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
