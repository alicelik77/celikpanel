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
      store.set({ operation: { id: 'op' + store.calls.length, service_id: request.serviceId, status: 'running' }, locked: true });
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
  '/lib/readyBundles': lib('lib/readyBundles.ts'),
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
let tree;
let requests;
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
function serve(managed) {
  requests = [];
  globalThis.fetch = async (input, init = {}) => {
    const path = String(input).split('?')[0];
    requests.push(`${init.method || 'GET'} ${path}`);
    if (path === '/api/v1/managed-services' || path === '/api/v1/managed-services/scan') return answer(managed);
    if (path === '/api/v1/host-mutation-readiness') return answer({ ready: true });
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
    // Nothing was sent by choosing.
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
    assert.ok(!stripText('database').includes('not verified'));
  } finally { await unmount(); }
});

test('an installed mail bundle that is verified offers Repair as a text link; one without its proof says so in grey and offers Verify', async () => {
  const mail = (id, profileId, extra) => ({ ...bundle(id, 'email', ['postfix'], extra), action_route: 'mail_profile', profile_id: profileId });
  const verified = payload({ bundles: [
    mail('mail.core', 'core-mail', { status: 'installed_verified', action: 'repair', missing_components: [], addons: ['mail.protected'] }),
    { ...mail('mail.protected', 'protected-mail', { status: 'partial', action: 'continue', missing_components: ['rspamd'] }), components: ['postfix', 'rspamd'] },
  ] });
  verified.services = verified.services.map((item) => (item.id === 'postfix' ? { ...item, is_installed: true, status: 'active (running)' } : item));
  await mount(verified);
  try {
    const line = bundleNode('mail.core');
    assert.ok(label(line).includes('Core mail installed and verified'), label(line));
    const repair = buttonsIn(line).find((node) => label(node) === say('services.bundles.repair'));
    assert.ok(repair, label(line));
    assert.match(repair.props.className, /underline/, 'Repair is a text link');
    assert.doesNotMatch(repair.props.className, /\bborder\b|bg-primary/);
    // The add-on follows as one "add …" offer.
    const addon = bundleNode('mail.protected');
    assert.ok(label(addon).includes('Add spam protection'), label(addon));
  } finally { await unmount(); }

  const unverified = payload({ bundles: [
    mail('mail.core', 'core-mail', { status: 'installed_unverified', action: 'verify', missing_components: [] }),
  ] });
  unverified.services = unverified.services.map((item) => (item.id === 'postfix' ? { ...item, is_installed: true, status: 'active (running)' } : item));
  await mount(unverified);
  try {
    const line = bundleNode('mail.core');
    assert.equal(line.props['data-bundle-status'], 'installed_unverified');
    assert.ok(label(line).includes('Core mail: installed, not verified'), label(line));
    const verify = buttonsIn(line).find((node) => label(node) === say('services.bundles.verify'));
    assert.ok(verify, 'no Verify');
    assert.doesNotMatch(verify.props.className, /bg-primary\b/);
    const dot = line.find((node) => node.props && node.props['data-tone'] !== undefined);
    assert.equal(dot.props['data-tone'], 'unknown', 'not verified is grey, not amber');
    assert.ok(!/text-warning|warning-mark/.test(JSON.stringify(line.findAll((node) => typeof node.props?.className === 'string').map((node) => node.props.className))), 'no amber on an unverified bundle');
  } finally { await unmount(); }
});

// --- The sequence -------------------------------------------------------------------
async function startWebBundle() {
  await press(bundleNode('web.nginx_php'));
  await pressLabel(say('services.bundles.installEllipsis'));
  await pressLabel(say('services.bundles.dialog.confirm', { n: 2 }));
}

test('a web bundle installs its components one at a time: the second is asked for only after the first operation ended with a fresh scan', async () => {
  const tracker = globalThis.bundleTracker;
  await mount(payload({ bundles: [webBundle(), dbBundle()] }));
  try {
    await startWebBundle();
    assert.deepEqual(tracker.calls.map((call) => call.serviceId), ['nginx'], 'the first install');
    assert.ok(stripText('web').includes(say('services.bundles.step', { n: 1, total: 2, component: nameOf('nginx') })), stripText('web'));
    assert.ok(tree.root.findAll((node) => node.props && node.props['data-bundle-running'] === 'web.nginx_php').length === 1);
    // The operation runs; the page re-renders and polls; nothing more is sent.
    for (let i = 0; i < 4; i += 1) await settle();
    tracker.set({ operation: { ...tracker.state.operation, phase: 'installing' } });
    await settle();
    assert.equal(tracker.calls.length, 1, 'the second install was asked for while the first was running');
    // The operation ends but the tracker has not given its fresh scan yet.
    tracker.set({ operation: null, locked: true });
    await settle();
    assert.equal(tracker.calls.length, 1, 'asked for before the tracker released the host');
    // The tracker's fresh scan arrives and the host is released.
    tracker.set({ locked: false, catalogSnapshot: payload({ nginx: true, bundles: [webBundle({ status: 'partial', action: 'continue', missing_components: ['php-fpm'] }), dbBundle()] }) });
    await settle();
    assert.deepEqual(tracker.calls.map((call) => call.serviceId), ['nginx', 'php-fpm']);
    assert.ok(stripText('web').includes(say('services.bundles.step', { n: 2, total: 2, component: nameOf('php-fpm') })), stripText('web'));
    // The last step ends: the run is over and nothing more is asked for.
    tracker.set({ operation: null, locked: false, catalogSnapshot: payload({ nginx: true, phpfpm: true, bundles: [webBundle({ status: 'installed_verified', action: 'none', missing_components: [] }), dbBundle()] }) });
    for (let i = 0; i < 3; i += 1) await settle();
    assert.equal(tracker.calls.length, 2);
    assert.equal(tree.root.findAll((node) => node.props && node.props['data-bundle-running'] !== undefined).length, 0);
    assert.equal(bundleNode('web.nginx_php').props['data-bundle-status'], 'installed_verified');
    // Each install is the ordinary component install, never a second address.
    assert.ok(tracker.calls.every((call) => call.operationKind === undefined && call.package === undefined));
  } finally { await unmount(); }
});

test('a failed step stops the sequence; the strip then reads Continue and a reload never continues by itself', async () => {
  const tracker = globalThis.bundleTracker;
  await mount(payload({ bundles: [webBundle(), dbBundle()] }));
  try {
    await startWebBundle();
    assert.equal(tracker.calls.length, 1);
    // The first operation fails: the tracker keeps its own guidance on screen.
    tracker.set({ operation: null, locked: false, failure: { message: 'nginx did not start', code: 'service_start_failed', component: 'nginx' },
      catalogSnapshot: payload({ bundles: [webBundle(), dbBundle()] }) });
    for (let i = 0; i < 4; i += 1) await settle();
    assert.equal(tracker.calls.length, 1, 'the next component was installed after a failure');
    assert.equal(tree.root.findAll((node) => node.props && node.props['data-bundle-running'] !== undefined).length, 0, 'the strip still says it is installing');
  } finally { await unmount(); }

  // Later, nginx is there and php-fpm is not: the strip says how far it got
  // and waits for the owner.
  await mount(payload({ nginx: true, bundles: [webBundle({ status: 'partial', action: 'continue', missing_components: ['php-fpm'] }), dbBundle()] }));
  try {
    for (let i = 0; i < 3; i += 1) await settle();
    const line = bundleNode('web.nginx_php');
    assert.equal(line.props['data-bundle-status'], 'partial');
    assert.ok(label(line).includes(say('services.bundles.partial', { name: 'Nginx + PHP-FPM', n: 1, total: 2 })), label(line));
    assert.equal(tracker.calls.length, 0, 'the page continued by itself');
    await pressLabel(say('services.bundles.continue'));
    await pressLabel(say('services.bundles.dialog.confirmContinue', { n: 1 }));
    assert.deepEqual(tracker.calls.map((call) => call.serviceId), ['php-fpm'], 'Continue installs only what is missing');
  } finally { await unmount(); }
});
