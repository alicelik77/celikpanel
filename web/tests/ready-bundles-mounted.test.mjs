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
  export const useLocation = () => ({ hash: '', pathname: '/services' });
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

// --- Three states -------------------------------------------------------------------
test('an offered bundle is a quiet chip; choosing it says what it installs and offers Install… as an outlined button', async () => {
  await mount(payload({ bundles: [webBundle(), dbBundle()] }));
  try {
    const chip = bundleNode('web.nginx_php');
    assert.equal(chip.type, 'button');
    assert.equal(chip.props['data-bundle-status'], 'available');
    assert.equal(chip.props['aria-expanded'], false);
    assert.ok(stripText('web').includes(say('services.bundles.label')), stripText('web'));
    assert.ok(label(chip).includes('Nginx + PHP-FPM'), label(chip));
    assert.ok(stripText('web').includes(say('services.bundles.hint')));
    await press(chip);
    const text = stripText('web');
    assert.ok(text.includes(say('services.bundles.description.web_nginx_php')), text);
    const install = buttonsIn(strip('web')).find((node) => label(node).includes(say('services.bundles.installEllipsis')));
    assert.ok(install, 'no Install… in the strip');
    assert.doesNotMatch(install.props.className, /bg-primary\b/, 'the strip has no filled button');
    assert.equal(requests.filter((line) => !line.startsWith('GET ') && !line.startsWith('POST /api/v1/managed-services/scan')).length, 0, requests.join(', '));
    assert.equal(globalThis.bundleTracker.calls.length, 0);
  } finally { await unmount(); }
});

test('an installed and verified bundle collapses to one line with a green check and no button', async () => {
  await mount(payload({ nginx: true, phpfpm: true, bundles: [webBundle({ status: 'installed_verified', action: 'none', missing_components: [] }), dbBundle()] }));
  try {
    const line = bundleNode('db.mariadb');
    assert.equal(line.props['data-bundle-status'], 'installed_verified');
    assert.ok(label(line).includes('MariaDB bundle installed and verified'), label(line));
    assert.equal(buttonsIn(line).length, 0, 'a verified database bundle has nothing to press');
  } finally { await unmount(); }
});

const mail = (id, profileId, extra) => ({ ...bundle(id, 'email', ['postfix'], extra), action_route: 'mail_profile', profile_id: profileId });
const withPostfix = (answer) => ({ ...answer, services: answer.services.map((item) => (item.id === 'postfix' ? { ...item, is_installed: true, status: 'active (running)' } : item)) });

test('an installed mail bundle offers Repair (a text link when verified, a button when not); never "Verify"', async () => {
  await mount(withPostfix(payload({ bundles: [
    mail('mail.core', 'core-mail', { status: 'installed_verified', action: 'repair', missing_components: [], addons: ['mail.protected'] }),
    { ...mail('mail.protected', 'protected-mail', { status: 'partial', action: 'continue', missing_components: ['rspamd'] }), components: ['postfix', 'rspamd'] },
  ] })));
  try {
    const line = bundleNode('mail.core');
    // Named as the product names the mail profile.
    assert.ok(label(line).includes('Core Mail installed and verified'), label(line));
    const repair = buttonsIn(line).find((node) => label(node) === say('services.bundles.repair'));
    assert.ok(repair, label(line));
    assert.match(repair.props.className, /underline/, 'Repair is a text link');
    assert.ok(label(bundleNode('mail.protected')).includes('Add spam protection'));
  } finally { await unmount(); }

  const unproven = withPostfix(payload({ bundles: [mail('mail.core', 'core-mail', { status: 'installed_unverified', action: 'repair', missing_components: [], warning_code: 'repair_to_verify' })] }));
  await mount(unproven);
  try {
    const line = bundleNode('mail.core');
    assert.ok(label(line).includes('Core Mail: installed, not verified'), label(line));
    assert.ok(buttonsIn(line).some((node) => label(node) === say('services.bundles.repair')), 'no Repair');
    assert.ok(!buttonsIn(line).some((node) => label(node) === say('services.bundles.verify')), 'a mail bundle offers Verify');
    // The Panel's reason is said again, calmly.
    const reason = line.find((node) => node.props && node.props['data-bundle-reason'] === 'mail.core');
    assert.equal(label(reason), say('services.mailProfiles.code.repair_to_verify'));
    assert.match(reason.props.className, /text-fg-muted/);
    const dot = line.find((node) => node.props && node.props['data-tone'] !== undefined);
    assert.equal(dot.props['data-tone'], 'unknown');
  } finally { await unmount(); }
});

test('Verify of a component bundle only reads: it stays usable while the page holds its changes', async () => {
  const stopped = payload({ bundles: [webBundle(), dbBundle({ status: 'installed_unverified', action: 'verify' })] });
  stopped.services = stopped.services.map((item) => (item.id === 'mariadb' ? { ...item, status: 'inactive (dead)' } : item));
  readiness = { ready: false, code: 'HOST_MUTATION_BUSY', reason: 'package_manager_active' };
  await mount(stopped);
  try {
    const verify = buttonsIn(bundleNode('db.mariadb')).find((node) => label(node) === say('services.bundles.verify'));
    assert.ok(verify && !verify.props.disabled, 'Verify is off while changes are held');
    const before = requests.filter((line) => line === 'POST /api/v1/managed-services/scan').length;
    await press(verify);
    assert.equal(requests.filter((line) => line === 'POST /api/v1/managed-services/scan').length, before + 1, 'Verify did not read again');
    // An install is held: the offer's Install… is off.
    await press(bundleNode('web.nginx_php'));
    const install = buttonsIn(strip('web')).find((node) => label(node).includes(say('services.bundles.installEllipsis')));
    assert.equal(install.props.disabled, true);
  } finally { await unmount(); readiness = { ready: true }; }
});

test('under Apache the add-on says what it installs, what is installed and what is not needed, and never calls Nginx installed', async () => {
  const apache = payload({ bundles: [
    dbBundle({ addons: ['db.mariadb.admin'] }),
    bundle('db.mariadb.admin', 'database', ['nginx', 'php-fpm', 'phpmyadmin'], { parent: 'db.mariadb', missing_components: ['phpmyadmin'] }),
  ], phpfpm: true });
  apache.services.push(component('apache', 'web', true), component('phpmyadmin', 'database', false, { kind: 'tool' }));
  await mount(apache);
  try {
    await press(bundleNode('db.mariadb.admin'));
    const detail = stripText('database');
    assert.ok(detail.includes(say('services.bundles.skips', { components: nameOf('php-fpm') })), detail);
    assert.ok(detail.includes(say('services.bundles.notNeeded', { components: nameOf('nginx') })), detail);
    await pressLabel(say('services.bundles.installEllipsis'));
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

// --- The sequence -------------------------------------------------------------------
async function startWebBundle() {
  await press(bundleNode('web.nginx_php'));
  await pressLabel(say('services.bundles.installEllipsis'));
  await pressLabel(say('services.bundles.dialog.confirm', { n: 2 }));
}
const fresh = (extra) => payload(extra);
const running = () => tree.root.findAll((node) => node.props && node.props['data-bundle-running'] !== undefined).length;
const stoppedLine = () => tree.root.findAll((node) => node.props && node.props['data-bundle-status'] === 'stopped')[0];
const ours = (serviceId, n) => ({ id: 'op' + n, service_id: serviceId, kind: 'service_install', status: 'running' });

test('a web bundle installs its components one at a time: the second only after the first operation, its own, ended with a fresh scan', async () => {
  const tracker = globalThis.bundleTracker;
  await mount(payload({ bundles: [webBundle(), dbBundle()] }));
  try {
    await startWebBundle();
    assert.deepEqual(tracker.calls.map((call) => call.serviceId), ['nginx'], 'the first install');
    assert.ok(stripText('web').includes(say('services.bundles.step', { n: 1, total: 2, component: nameOf('nginx') })), stripText('web'));
    assert.equal(running(), 1);
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

test('a failed step stops the sequence with Continue; a reload offers Install, which installs only what is missing', async () => {
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
    const line = stoppedLine();
    assert.equal(line.props['data-bundle-stop'], 'failed');
    assert.ok(label(line).includes(say('services.bundles.stopped.failed', { component: nameOf('nginx') })), label(line));
    assert.ok(buttonsIn(line).some((node) => label(node) === say('services.bundles.continue')));
  } finally { await unmount(); }

  // After a reload: Nginx is there, PHP-FPM is not. The bundle is offered;
  // nothing continues by itself, and Install installs only PHP-FPM.
  await mount(payload({ nginx: true, bundles: [webBundle({ missing_components: ['php-fpm'] }), dbBundle()] }));
  try {
    for (let i = 0; i < 3; i += 1) await settle();
    assert.equal(bundleNode('web.nginx_php').props['data-bundle-status'], 'available');
    assert.equal(tracker.calls.length, 0, 'the page continued by itself');
    await press(bundleNode('web.nginx_php'));
    await pressLabel(say('services.bundles.installEllipsis'));
    await pressLabel(say('services.bundles.dialog.confirm', { n: 1 }));
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
    let line = stoppedLine();
    assert.ok(line, 'the strip still says it installs Nginx');
    assert.equal(line.props['data-bundle-stop'], 'foreign');
    assert.ok(label(line).includes(say('services.bundles.stopped.foreign')), label(line));
    // That operation ends with a fresh scan: nothing more is sent.
    tracker.set({ operation: null, locked: false, catalogSnapshot: fresh({ nginx: true, bundles: [webBundle({ missing_components: ['php-fpm'] }), dbBundle()] }) });
    for (let i = 0; i < 3; i += 1) await settle();
    assert.equal(tracker.calls.length, 1, 'the next step was sent on another operation\'s end');
    line = stoppedLine();
    assert.ok(buttonsIn(line).some((node) => label(node) === say('services.bundles.continue')), 'no Continue');
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
    const line = stoppedLine();
    assert.equal(line.props['data-bundle-stop'], 'gated');
    assert.ok(label(line).includes(say('services.bundles.stopped.gated', { reason: say('services.bundles.gatedReadiness') })), label(line));
    assert.ok(requests.filter((entry) => entry === 'GET /api/v1/host-mutation-readiness').length >= 2, 'the gate was not read again');
  } finally { await unmount(); readiness = { ready: true }; }
});
