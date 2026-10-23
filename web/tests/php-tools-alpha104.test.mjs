// alpha.104, mounted: a removal never removes anything it did not name, and
// each PHP tool runs on a PHP version the owner chooses (owner decisions of
// 2026-10-23, after the Frankfurt removal of PHP-FPM 8.4 that also purged
// phpMyAdmin). The Components page and the tool card are drawn with the real
// shared layer from what the Panel answers: the version-removal dialog names
// the sites and tools on the version and what the package manager would also
// take, refuses before the button, offers to move the tools first and says
// exactly what happened when a move fails; the generic dialog names
// collateral; the versions list says who uses each version; a tool whose PHP
// is missing is not "Installed"; the tool's card moves it and says the result.
// alpha.104, bağlanmış: kaldırma adını söylemediğini kaldırmaz; her PHP aracı
// sahibin seçtiği PHP sürümünde çalışır.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { en as enShellHalf } from '../src/i18n/en.ts';
import { enErrorsScreens as enErrorsPart } from '../src/i18n/screens/errors/en.ts';
import { enScreens, trScreens } from './locale-catalogue.mjs';
import { tr as trShellHalf } from '../src/i18n/tr.ts';
import { trErrorsScreens as trErrorsPart } from '../src/i18n/screens/errors/tr.ts';
import { enComponentsScreens as enComponents } from '../src/i18n/screens/components/en.ts';
import { trComponentsScreens as trComponents } from '../src/i18n/screens/components/tr.ts';

const words = { ...enShellHalf, ...enErrorsPart, ...enScreens };
const trWords = { ...trShellHalf, ...trErrorsPart, ...trScreens };
globalThis.bundleWords = words;
const fill = (text, vars = {}) => Object.entries(vars).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
const say = (key, vars = {}) => fill(words[key] ?? key, vars);

const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const icons = new Set(['AlertTriangle']);
for (const path of ['components/ServiceList.tsx', 'components/FirewallPanel.tsx', 'components/ReadyBundleStrip.tsx', 'lib/componentIcons.tsx']) {
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
  const i18n = { t: (key, vars) => (globalThis.bundleWords?.[key] !== undefined ? fill(globalThis.bundleWords[key], vars) : key), locale: 'en' };
  export const useI18n = () => i18n;
  export const useNavigate = () => () => true;
  export const useLocation = () => ({ hash: '', pathname: '/services', search: '' });
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
const trackerURL = dataModule(`
  import React from '${reactURL}';
  export * from '${operationURL}';
  const store = globalThis.bundleTracker = { listeners: new Set(), calls: [], state: null,
    set(next) { store.state = { ...store.state, ...next }; for (const listener of store.listeners) listener(); } };
  store.state = { operation: null, failure: null, catalogSnapshot: null, locked: false, checking: false, startInstall: async () => true };
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
const page = await import(lib('components/ServiceList.tsx', {
  '/ComponentOperation': trackerURL,
  '/lib/serverSetup': lib('lib/serverSetup.ts'),
  '/WebmailRepairButton': lib('components/WebmailRepairButton.tsx'),
  '/ReadyBundleStrip': lib('components/ReadyBundleStrip.tsx', pieces),
  ...pieces,
}));
const { ServiceList } = page;
globalThis.window ??= globalThis;

// --- What the Panel answers (Frankfurt after alpha.104: PHP 8.6, 8.5 and 8.4) ------------
const component = (id, category, installed, extra = {}) => ({
  id, name: id, description: `${id} on this server`, icon: '', category,
  status: installed ? 'active (running)' : 'not_installed', is_installed: installed, versions: [], packages: [id], ...extra,
});
const instance = (version) => ({ version, unit: `php${version}-fpm`, managed: true, status: 'active (running)' });
const SERVER = [
  component('nginx', 'web', true),
  component('php-fpm', 'web', true, { kind: 'runtime', versions: ['8.6', '8.5', '8.4'], instances: [instance('8.6'), instance('8.5'), instance('8.4')] }),
  component('mariadb', 'database', true, { packages: ['mariadb-server'] }),
  component('postfix', 'email', true),
  component('phppgadmin', 'database', true, { kind: 'tool', status: 'installed', php_binding: { version: '8.4', socket_ready: false, version_installed: false } }),
  component('roundcube', 'email', true, { kind: 'tool', status: 'installed', packages: [], php_binding: { version: '8.4', socket_ready: true, version_installed: true } }),
  component('phpmyadmin', 'database', true, { kind: 'tool', status: 'installed', package_version: '4:5.2.2-really+dfsg-1+deb13u1', package_version_short: '5.2.2',
    package_versions: [{ package: 'phpmyadmin', version: '4:5.2.2-really+dfsg-1+deb13u1' }], package_version_read_at: '2026-10-23T08:00:00Z' }),
];
const payload = (services = SERVER) => ({
  scanned_at: new Date().toISOString(), dns_identity_ready: true,
  mail_hostname: { current: 'server1', hostname: '', source: '', current_usable: true, will_set_hostname: false },
  profiles: [profile('core-mail'), profile('webmail'), profile('protected-mail')], services, bundles: [],
});
function profile(id) {
  return { id, name: id, description: `${id} profile`, status: 'available', available: true, verified: false, latest_attempt_status: 'none', services: ['postfix'] };
}
const PHP_TOOLS = {
  state: 'read',
  tools: [
    { tool: 'phpmyadmin', service_id: 'phpmyadmin', installed: true, version: '8.5', ready: true, version_installed: true },
    { tool: 'phppgadmin', service_id: 'phppgadmin', installed: true, version: '8.4', ready: true, version_installed: true },
    { tool: 'webmail', service_id: 'roundcube', installed: true, version: '8.4', ready: true, version_installed: true },
  ],
  versions: [
    { version: '8.6', sites: 0, site_names: [], tools: [] },
    { version: '8.5', sites: 1, site_names: ['shop.example (ali)'], tools: ['phpmyadmin'] },
    { version: '8.4', sites: 2, site_names: ['a.example (ali)', 'b.example (veli)'], tools: ['phppgadmin', 'webmail'] },
  ],
};
const versionImpact = (simulation, hosted = { kind: 'php_sites', count: 0, names: [], blocks_removal: false }) => ({
  removal: 'packages', has_units: true, setup_required: false, package_family: 'apt', packages: ['php8.4-fpm'], all_versions: false, firewall_ports: [],
  mail_filter_rewired: false, database_tools_reconfigured: false, required_by: [], bundles: [], hosted, package: 'php8.4-fpm', version: '8.4', simulation,
});
const sim = (extra = {}) => ({ state: 'checked', remove: ['php8.4-fpm', 'php8.4-cli', 'php8.4-common'], collateral: [], bound_tools: [], default_runtime: false, refuses: false, ...extra });
// Frankfurt as it was on 2026-10-23: the removal would also take phpMyAdmin
// and the distribution's PHP packages; webmail and phpPgAdmin run on 8.4.
const FRANKFURT_SIM = sim({
  collateral: [
    { package: 'php-fpm', reason: 'component', component: 'php-fpm' },
    { package: 'phpmyadmin', reason: 'component', component: 'phpmyadmin' },
    { package: 'php-mysql', reason: 'reverse_dependency' },
    { package: 'php-mbstring', reason: 'reverse_dependency' },
  ],
  bound_tools: ['phppgadmin', 'webmail'], default_runtime: true, refuses: true,
});

const originalFetch = globalThis.fetch;
let tree;
let requests = [];
let routes = {};
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
function serve(managed, extra = {}) {
  requests = [];
  routes = extra;
  globalThis.fetch = async (input, init = {}) => {
    const url = String(input);
    const path = url.split('?')[0];
    const method = init.method ?? 'GET';
    requests.push({ path, url, method, body: init.body });
    if (routes[path]) return routes[path](url, init);
    if (path === '/api/v1/managed-services' || path === '/api/v1/managed-services/scan') return answer(managed);
    if (path === '/api/v1/host-mutation-readiness') return answer({ ready: true });
    if (path === '/api/v1/firewall') return answer({ enabled: true, engine_available: true, tcp_ports: [22, 80, 443], udp_ports: [], ssh_ports: [22], persistence_state: 'ready' });
    if (path === '/api/v1/repo') return answer({ available: false });
    if (path === '/api/v1/service/php-tools') return answer(PHP_TOOLS);
    return answer({ error: 'no route' }, 404);
  };
}
async function settle() { await act(async () => { await new Promise((resolve) => setTimeout(resolve, 15)); }); }
async function mount({ managed = payload(), extra = {}, catalogue = words } = {}) {
  globalThis.bundleWords = catalogue;
  globalThis.bundleToasts = [];
  globalThis.document = { addEventListener() {}, removeEventListener() {}, getElementById: () => null, activeElement: null, body: { style: {} } };
  serve(managed, extra);
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
const rowOf = (id) => {
  let node = tree.root.findAll((child) => child.props && child.props.id === id && child.props.category !== undefined && child.props.installed !== undefined)[0];
  while (node && node.type !== 'li') node = node.parent;
  return node;
};
const nameOf = (id) => globalThis.bundleWords[`services.component.name.${id}`] ?? id;
const simLines = () => Object.fromEntries(tree.root.findAll((node) => node.props && node.props['data-removal-sim'] !== undefined)
  .map((node) => [node.props['data-removal-sim'], label(node)]));
const buttonsLabelled = (text) => tree.root.findAll((node) => node.type === 'button' && label(node).includes(text));

async function openVersions() {
  const toggle = rowOf('php-fpm').findAll((node) => node.type === 'button' && label(node).includes(globalThis.bundleWords['services.versionsToggle']))[0];
  assert.ok(toggle, 'php-fpm has no versions toggle');
  await act(async () => { toggle.props.onClick(); });
  for (let i = 0; i < 2; i += 1) await settle();
}
async function openVersionRemoval(version) {
  await openVersions();
  const trash = tree.root.findAll((node) => node.type === 'button' && node.props.title === globalThis.bundleWords['services.removeVersion']);
  const rows = tree.root.findAll((node) => node.props && node.props['data-version-usage'] === version);
  let row = rows[0];
  while (row && row.type !== 'li') row = row.parent;
  const button = row ? row.findAll((node) => node.type === 'button' && node.props.title === globalThis.bundleWords['services.removeVersion'])[0] : trash[0];
  assert.ok(button, `PHP ${version} has no remove button`);
  await act(async () => { button.props.onClick(); });
  for (let i = 0; i < 3; i += 1) await settle();
}
const impactRoute = (body, status = 200) => ({ '/api/v1/service/removal-impact': () => answer(body, status) });

// --- The version-removal dialog ---------------------------------------------------
test('PHP 8.4 on Frankfurt: the tools on it, what the package manager would also take, the default, refused before the button', async () => {
  await mount({ extra: impactRoute(versionImpact(FRANKFURT_SIM)) });
  try {
    await openVersionRemoval('8.4');
    const read = requests.find((request) => request.path === '/api/v1/service/removal-impact');
    assert.match(read.url, /service_id=php-fpm&package=php8\.4-fpm/);
    const name = `${nameOf('php-fpm')} 8.4`;
    const said = simLines();
    assert.equal(said.tools, say('services.removal.sim.tools', { names: `${nameOf('phppgadmin')}, ${nameOf('roundcube')}`, version: '8.4' }));
    assert.equal(said.collateral, say('services.removal.sim.refused', { name }));
    assert.equal(said['component-phpmyadmin'], say('services.removal.sim.component', { packages: 'phpmyadmin', component: nameOf('phpmyadmin'), name }));
    assert.equal(said.default, say('services.removal.sim.default', { packages: 'php-fpm', name }));
    assert.equal(said.depends, say('services.removal.sim.depends', { packages: 'php-mysql, php-mbstring', name }));
    // Collateral present: no move is offered and Remove is not pressable.
    assert.equal(buttonsLabelled(say('services.version.moveButton', { version: '8.6' })).length, 0);
    const remove = buttonsLabelled(say('services.removeVersion')).find((node) => node.props.title === say('services.removal.refusedButton'));
    assert.ok(remove && remove.props.disabled, 'Remove is offered for a removal the Panel refuses');
    // Opening the dialog changed nothing.
    assert.equal(requests.filter((request) => request.method !== 'GET').length, 0);
  } finally { await unmount(); }
});

test('only tools on the version: move them to a chosen version, then remove; a failed move names what moved and what was put back', async () => {
  const uninstall = () => answer({
    error: 'Webmail did not work on PHP 8.5; it was put back on PHP 8.4', code: 'PHP_TOOL_MOVE_FAILED', tool: 'webmail', version: '8.5',
    previous: '8.4', restored: true, mutation_applied: true, moved: ['phppgadmin'], not_attempted: [],
  }, 409);
  await mount({ extra: { ...impactRoute(versionImpact(sim({ bound_tools: ['phppgadmin', 'webmail'], refuses: true }))), '/api/v1/service/uninstall': uninstall } });
  try {
    await openVersionRemoval('8.4');
    const select = tree.root.findAll((node) => node.type === 'select')[0];
    assert.deepEqual(select.props.children.map((option) => option.props.value), ['8.6', '8.5']);
    await act(async () => { select.props.onChange({ target: { value: '8.5' } }); });
    const move = buttonsLabelled(say('services.version.moveButton', { version: '8.5' }))[0];
    assert.ok(move && !move.props.disabled);
    await act(async () => { move.props.onClick(); });
    for (let i = 0; i < 3; i += 1) await settle();
    const sent = requests.find((request) => request.path === '/api/v1/service/uninstall');
    assert.deepEqual(JSON.parse(sent.body), { service_id: 'php-fpm', package: 'php8.4-fpm', move_tools_to: '8.5' });
    const note = tree.root.findAll((node) => node.props && node.props['data-version-move-failure'] === 'webmail')[0];
    assert.ok(note, 'the failed move is not explained');
    const text = label(note);
    assert.ok(text.includes(say('services.version.moveDone', { tools: nameOf('phppgadmin'), version: '8.5' })), text);
    // No structured cause in this answer: the server's words are quoted.
    assert.ok(text.includes(say('services.version.moveRestored', { tool: nameOf('roundcube'), version: '8.5', previous: '8.4', reason: say('component.phpTool.cause.other', { detail: 'Webmail did not work on PHP 8.5; it was put back on PHP 8.4' }) })), text);
    assert.ok(text.includes(say('services.version.moveKept', { version: '8.4' })), text);
  } finally { await unmount(); }
});

test('a successful move and removal says where the tools run now', async () => {
  await mount({ extra: {
    ...impactRoute(versionImpact(sim({ bound_tools: ['webmail'], refuses: true }))),
    '/api/v1/service/uninstall': () => answer({ removed: true, success: true, moved: ['webmail'], moved_to: '8.6' }),
  } });
  try {
    await openVersionRemoval('8.4');
    await act(async () => { buttonsLabelled(say('services.version.moveButton', { version: '8.6' }))[0].props.onClick(); });
    for (let i = 0; i < 4; i += 1) await settle();
    assert.ok(globalThis.bundleToasts.some(([kind, text]) => kind === 'success' && text === say('services.version.movedAndRemoved', { tools: nameOf('roundcube'), target: '8.6', version: '8.4' })),
      JSON.stringify(globalThis.bundleToasts));
  } finally { await unmount(); }
});

test('a read that fails says so and the removal stays allowed; sites on the version are named with the way to move them', async () => {
  await mount({ extra: impactRoute({ error: 'boom' }, 500) });
  try {
    await openVersionRemoval('8.4');
    assert.ok(label(tree.root).includes(say('services.removal.unknownVersion', { name: `${nameOf('php-fpm')} 8.4` })));
    const remove = buttonsLabelled(say('services.removeVersion')).find((node) => label(node) === say('services.removeVersion'));
    assert.ok(remove && !remove.props.disabled, 'a failed read held the removal');
  } finally { await unmount(); }
  await mount({ extra: impactRoute(versionImpact(sim(), { kind: 'php_sites', count: 2, names: ['a.example (ali)', 'b.example (veli)'], blocks_removal: true })) });
  try {
    await openVersionRemoval('8.4');
    const sites = tree.root.findAll((node) => node.props && node.props['data-removal-line'] === 'sites')[0];
    assert.ok(label(sites).includes(say('services.version.sitesOn', { count: 2, version: '8.4', names: 'a.example (ali), b.example (veli)' })));
    assert.ok(label(sites).includes(say('services.version.sitesLink')));
  } finally { await unmount(); }
});

test('the busy and unknown simulation say the removal is checked again when pressed', async () => {
  for (const state of ['busy', 'unknown']) {
    await mount({ extra: impactRoute(versionImpact(sim({ state, remove: [] }))) });
    try {
      await openVersionRemoval('8.6');
      assert.equal(simLines()[state], say(`services.removal.sim.${state}`));
    } finally { await unmount(); }
  }
});

// alpha.104 review: the move is offered only when the package manager
// answered; moves that happened before a refusal are said with it; "no other
// version" only when the versions were read.
test('tools on the version but no package manager answer: no move is offered', async () => {
  for (const state of ['busy', 'unknown']) {
    await mount({ extra: impactRoute(versionImpact(sim({ state, remove: [], bound_tools: ['webmail'], refuses: true }))) });
    try {
      await openVersionRemoval('8.4');
      assert.equal(buttonsLabelled(say('services.version.moveButton', { version: '8.6' })).length, 0, state);
      assert.equal(tree.root.findAll((node) => node.props && node.props['data-version-move'] !== undefined).length, 0, state);
    } finally { await unmount(); }
  }
});

test('the tools moved, then the removal was refused: both are said and everything is read again', async () => {
  await mount({ extra: {
    ...impactRoute(versionImpact(sim({ bound_tools: ['webmail'], refuses: true }))),
    '/api/v1/service/uninstall': () => answer({ error: 'the package manager’s answer changed', code: 'REMOVAL_PLAN_CHANGED', moved: ['webmail'], moved_to: '8.6' }, 409),
  } });
  try {
    await openVersionRemoval('8.4');
    const readsBefore = requests.filter((request) => request.path === '/api/v1/service/php-tools').length;
    await act(async () => { buttonsLabelled(say('services.version.moveButton', { version: '8.6' }))[0].props.onClick(); });
    for (let i = 0; i < 4; i += 1) await settle();
    const note = tree.root.findAll((node) => node.props && node.props['data-version-move-failure'] !== undefined)[0];
    assert.ok(note, 'the moves are not said');
    const text = label(note);
    assert.ok(text.includes(say('services.version.moveDone', { tools: nameOf('roundcube'), version: '8.6' })), text);
    assert.ok(text.includes(say('services.version.moveKept', { version: '8.4' })), text);
    assert.ok(label(tree.root).includes(say('err.REMOVAL_PLAN_CHANGED')), 'the refusal is not said');
    assert.ok(!label(tree.root).includes('Nothing was removed: the tools'), 'nothing claims no tool moved');
    assert.ok(requests.filter((request) => request.path === '/api/v1/service/php-tools').length > readsBefore, 'the tools were not read again');
  } finally { await unmount(); }
});

test('the tools moved and the removal answer was lost: not known, never "was not removed"', async () => {
  await mount({ extra: {
    ...impactRoute(versionImpact(sim({ bound_tools: ['webmail'], refuses: true }))),
    '/api/v1/service/uninstall': () => answer({ error: 'not known', code: 'REMOVAL_UNKNOWN_AFTER_MOVES', moved: ['webmail'], moved_to: '8.6' }, 502),
  } });
  try {
    await openVersionRemoval('8.4');
    await act(async () => { buttonsLabelled(say('services.version.moveButton', { version: '8.6' }))[0].props.onClick(); });
    for (let i = 0; i < 4; i += 1) await settle();
    const text = label(tree.root.findAll((node) => node.props && node.props['data-version-move-failure'] !== undefined)[0]);
    assert.ok(text.includes(say('services.version.removalUnknown', { version: '8.4' })), text);
    assert.ok(!text.includes(say('services.version.moveKept', { version: '8.4' })), text);
  } finally { await unmount(); }
});

test('installed versions that could not be read are not "no other version"', async () => {
  let fail = true;
  await mount({ extra: {
    ...impactRoute(versionImpact(sim({ bound_tools: ['webmail'], refuses: true }))),
    '/api/v1/service/php-tools': () => (fail ? answer({ error: 'agent' }, 502) : answer(PHP_TOOLS)),
  } });
  try {
    // Without the usage read the list cannot say which row is 8.4: the first
    // version's Remove opens (8.6), and the tools are said to run on it.
    await openVersionRemoval('8.4');
    const line = tree.root.findAll((node) => node.props && node.props['data-version-move'] === 'unknown')[0];
    assert.ok(line, 'the unread versions are not said');
    assert.ok(label(line).includes(say('services.version.versionsUnknown')));
    assert.ok(!label(tree.root).includes(say('services.version.moveNone')));
    fail = false;
    const retry = line.findAll((node) => node.type === 'button')[0];
    await act(async () => { retry.props.onClick(); });
    for (let i = 0; i < 3; i += 1) await settle();
    assert.ok(buttonsLabelled(say('services.version.moveButton', { version: '8.5' })).length > 0, 'Retry did not read the versions again');
  } finally { await unmount(); }
});

// --- Who uses each version; a tool whose PHP is missing -----------------------------
test('the versions list says who uses each version', async () => {
  await mount();
  try {
    await openVersions();
    const usage = Object.fromEntries(tree.root.findAll((node) => node.props && node.props['data-version-usage'] !== undefined).map((node) => [node.props['data-version-usage'], label(node)]));
    assert.equal(usage['8.4'], say('services.version.usedBy', { who: `${say('services.version.sites', { count: 2 })}, ${nameOf('phppgadmin')}, ${nameOf('roundcube')}` }));
    assert.equal(usage['8.5'], say('services.version.usedBy', { who: `${say('services.version.oneSite')}, ${nameOf('phpmyadmin')}` }));
    assert.equal(usage['8.6'], say('services.version.unused'));
  } finally { await unmount(); }
});

test('a PHP tool whose PHP is gone is not ready and says why; phpMyAdmin "really" version reads 5.2.2', async () => {
  await mount();
  try {
    const row = rowOf('phppgadmin');
    const state = row.findAll((node) => node.props && node.props['data-row-state'] !== undefined)[0];
    assert.equal(state.props['data-row-state'], 'notReady');
    assert.ok(label(state).includes(say('services.phpNotReady')));
    assert.ok(!label(state).includes(say('services.installedLabel')));
    const hint = row.findAll((node) => node.props && node.props['data-row-not-ready'] !== undefined)[0];
    assert.equal(label(hint), say('services.phpNotReadyGone', { version: '8.4' }));
    assert.equal(rowOf('roundcube').findAll((node) => node.props && node.props['data-row-state'] !== undefined)[0].props['data-row-state'], 'installed');
    const cells = rowOf('phpmyadmin').findAll((node) => node.props && node.props['data-row-version'] !== undefined);
    assert.ok(cells.length > 0);
    for (const cell of cells) {
      assert.ok(label(cell).startsWith('5.2.2'), label(cell));
      assert.ok(!label(cell).startsWith('5.2.2-really'), label(cell));
    }
  } finally { await unmount(); }
});

test('a PHP tool whose extension packages are missing on its version is not ready and says how to fix it', async () => {
  const services = SERVER.map((service) => service.id === 'roundcube'
    ? { ...service, php_binding: { version: '8.4', socket_ready: true, version_installed: true, extensions_present: false } } : service);
  await mount({ managed: payload(services) });
  try {
    const row = rowOf('roundcube');
    assert.equal(row.findAll((node) => node.props && node.props['data-row-state'] !== undefined)[0].props['data-row-state'], 'notReady');
    const hint = row.findAll((node) => node.props && node.props['data-row-not-ready'] !== undefined)[0];
    assert.equal(label(hint), say('services.phpNotReadyExtensions', { version: '8.4' }));
  } finally { await unmount(); }
});

// --- The generic removal dialog names collateral ---------------------------------------
test('removing a component names what the package manager would also take, and Remove is not offered', async () => {
  const mariadb = {
    removal: 'packages', has_units: true, setup_required: false, package_family: 'apt', packages: ['mariadb-server'], all_versions: false, firewall_ports: [],
    mail_filter_rewired: false, database_tools_reconfigured: false, required_by: ['phpmyadmin'], bundles: [], hosted: null,
    simulation: sim({ remove: ['mariadb-server'], collateral: [{ package: 'phpmyadmin', reason: 'component', component: 'phpmyadmin' }], refuses: true }),
  };
  await mount({ extra: impactRoute(mariadb) });
  try {
    const trash = rowOf('mariadb').findAll((node) => node.type === 'button' && node.props.title === say('services.uninstall'))[0];
    await act(async () => { trash.props.onClick(); });
    await settle();
    const said = simLines();
    assert.equal(said.collateral, say('services.removal.sim.refused', { name: nameOf('mariadb') }));
    assert.equal(said['component-phpmyadmin'], say('services.removal.sim.component', { packages: 'phpmyadmin', component: nameOf('phpmyadmin'), name: nameOf('mariadb') }));
    const remove = tree.root.findAll((node) => node.type === 'button' && label(node).includes(say('services.uninstall')) && node.props.title === say('services.removal.refusedButton'))[0];
    assert.ok(remove && remove.props.disabled);
  } finally { await unmount(); }
});

test('Turkish: the version dialog speaks Turkish with no raw key', async () => {
  await mount({ catalogue: trWords, extra: impactRoute(versionImpact(FRANKFURT_SIM)) });
  try {
    await openVersionRemoval('8.4');
    const text = label(tree.root);
    assert.ok(text.includes(trWords['services.removeVersionWarn']));
    assert.ok(!/services\.(removal|version)\./.test(text), 'a key rendered as its name');
  } finally { await unmount(); }
});

test('every alpha.104 text exists in English and Turkish, Turkish not English', () => {
  const keys = Object.keys(enComponents).filter((key) => /^(services\.removal\.sim\.|services\.version\.|component\.phpTool\.|services\.phpNotReady)/.test(key));
  assert.ok(keys.length >= 50, String(keys.length));
  for (const key of keys) {
    assert.equal(typeof trComponents[key], 'string', key);
    if (!['services.version.oneSite'].includes(key)) assert.notEqual(trComponents[key], enComponents[key], key);
  }
});

// alpha.104 review: another tool's PHP problem after an install is a warning
// naming that tool, decoded from the operation result; the install worked.
test('an install result naming another tool’s PHP problem is decoded for a warning, in both languages', async () => {
  const { decodePHPToolProblems } = await import(operationURL);
  assert.deepEqual(decodePHPToolProblems({ success: true, php_tool_problems: [
    { tool: 'phppgadmin', version: '8.4', kind: 'version_gone' },
    { tool: 'webmail', version: '8.5', kind: 'extensions_failed' },
    { tool: 'nginx', version: '8.5', kind: 'version_gone' },
    { tool: 'webmail', version: '8.5', kind: 'other' },
  ] }), [
    { tool: 'phppgadmin', version: '8.4', kind: 'version_gone' },
    { tool: 'webmail', version: '8.5', kind: 'extensions_failed' },
  ]);
  assert.deepEqual(decodePHPToolProblems({ success: true }), []);
  for (const key of ['services.phpToolProblem.versionGone', 'services.phpToolProblem.extensionsFailed']) {
    assert.equal(typeof enShellHalf[key], 'string', key);
    assert.equal(typeof trShellHalf[key], 'string', key);
    assert.notEqual(enShellHalf[key], trShellHalf[key], key);
  }
  assert.match(source('components/ComponentOperation.tsx'), /for \(const problem of decodePHPToolProblems\(next\.result\)\) \{\s*showToast\('warning'/);
});
