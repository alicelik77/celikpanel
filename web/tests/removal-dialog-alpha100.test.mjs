// The removal dialog and the first Components visit, mounted (alpha.100, owner
// decision 2026-10-22). The page is drawn with the real shared layer from what
// the Panel answers. The dialog reads GET /api/v1/service/removal-impact once
// when it opens and says, by name, what the removal affects on this server, what
// the uninstall path removes and what it keeps; a read that fails says so and the
// removal stays allowed. The first visit opens the installed view; only a
// remembered "catalog" opens the catalogue.
// Kaldırma penceresi ve Bileşenler'in ilk ziyareti, bağlanmış olarak.
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
const { ServiceList, decodeRemovalImpact, removalSentences } = page;
globalThis.window ??= globalThis;

// --- What the Panel answers ---------------------------------------------------------
const component = (id, category, installed, extra = {}) => ({
  id, name: id, description: `${id} on this server`, icon: '', category,
  status: installed ? 'active (running)' : 'not_installed', is_installed: installed, versions: [], packages: [id], ...extra,
});
const profile = (id) => ({ id, name: id, description: `${id} profile`, status: 'available', available: true, verified: false, latest_attempt_status: 'none', services: ['postfix'] });
const SERVER = [
  component('nginx', 'web', true),
  component('php-fpm', 'web', true, { kind: 'runtime', versions: ['8.4'] }),
  component('phpmyadmin', 'database', true, { kind: 'tool' }),
  component('mariadb', 'database', true, { packages: ['mariadb-server'] }),
  component('postgresql', 'database', true),
  component('postfix', 'email', true),
  component('rspamd', 'email', true),
  component('roundcube', 'email', true, { kind: 'tool', packages: [] }),
  component('certbot', 'security', true, { kind: 'tool' }),
  component('cron', 'system', true),
  component('redis', 'cache', false),
];
const payload = (services = SERVER) => ({
  scanned_at: new Date().toISOString(),
  dns_identity_ready: true,
  mail_hostname: { current: 'server1', hostname: '', source: '', current_usable: true, will_set_hostname: false },
  profiles: [profile('core-mail'), profile('webmail'), profile('protected-mail')],
  services,
  bundles: [],
});
const impact = (extra = {}) => ({
  service_id: 'x', removal: 'packages', package_family: 'apt', packages: [], all_versions: false, firewall_ports: [],
  mail_filter_rewired: false, database_tools_reconfigured: false, required_by: [], bundles: [], hosted: null, ...extra,
});
const sites = ['a.example (ali)', 'b.example (ali)', 'c.example (veli)', 'd.example (veli)', 'e.example (ali)', 'f.example (ali)', 'g.example (ali)'];
const IMPACTS = {
  nginx: impact({ packages: ['nginx'], firewall_ports: ['80/tcp', '443/tcp'], required_by: ['phpmyadmin', 'roundcube'],
    bundles: [{ id: 'web.nginx_php', name_code: 'web_nginx_php' }, { id: 'db.mariadb.admin', name_code: 'db_mariadb_admin' }],
    hosted: { kind: 'sites', count: 7, names: sites, blocks_removal: true } }),
  postgresql: impact({ packages: ['postgresql'], all_versions: true, bundles: [{ id: 'db.postgresql', name_code: 'db_postgresql' }],
    hosted: { kind: 'databases', count: 2, names: ['shop', 'blog'], blocks_removal: true } }),
  postfix: impact({ packages: ['postfix'], firewall_ports: ['25/tcp', '587/tcp'], required_by: ['rspamd', 'roundcube'],
    bundles: [{ id: 'mail.core', name_code: 'mail_core' }],
    hosted: { kind: 'mail_addresses', count: 1, names: ['info@a.example'], blocks_removal: true } }),
  rspamd: impact({ packages: ['rspamd'], mail_filter_rewired: true, hosted: { kind: 'mail_domains', count: 1, names: ['a.example'], blocks_removal: false } }),
  roundcube: impact({ removal: 'webmail_tree', hosted: { kind: 'mailboxes', count: 0, names: [], blocks_removal: false } }),
  certbot: impact({ packages: ['certbot'], package_family: 'pacman' }),
  cron: { __status: 409, body: { error: 'CelikPanel does not remove this server’s cron service.', code: 'NATIVE_CRON_REMOVAL_REFUSED' } },
};

const originalFetch = globalThis.fetch;
let tree;
let requests = [];
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
function serve(managed, impacts = IMPACTS) {
  requests = [];
  globalThis.fetch = async (input, init = {}) => {
    const url = String(input);
    const path = url.split('?')[0];
    requests.push({ path, url, method: init.method ?? 'GET' });
    if (path === '/api/v1/managed-services' || path === '/api/v1/managed-services/scan') return answer(managed);
    if (path === '/api/v1/host-mutation-readiness') return answer({ ready: true });
    if (path === '/api/v1/firewall') return answer({ enabled: true, engine_available: true, tcp_ports: [22, 80, 443], udp_ports: [], ssh_ports: [22], persistence_state: 'ready' });
    if (path === '/api/v1/service/removal-impact') {
      const id = new URLSearchParams(url.split('?')[1]).get('service_id');
      const found = typeof impacts === 'function' ? impacts(id) : impacts[id];
      if (found instanceof Promise) return found;
      if (found === undefined) return answer({ error: 'no' }, 500);
      if (found.__status) return answer(found.body, found.__status);
      return answer({ ...found, service_id: id });
    }
    return answer({ error: 'no route' }, 404);
  };
}
async function settle() { await act(async () => { await new Promise((resolve) => setTimeout(resolve, 15)); }); }
async function mount({ managed = payload(), impacts, catalogue = words, stored } = {}) {
  globalThis.bundleWords = catalogue;
  globalThis.bundleToasts = [];
  globalThis.document = { addEventListener() {}, removeEventListener() {}, getElementById: () => null, activeElement: null, body: { style: {} } };
  if (stored !== undefined) {
    globalThis.localStorage = { getItem: (key) => (key === 'celikpanel.components.view' ? stored : null), setItem() {}, removeItem() {} };
  }
  serve(managed, impacts);
  await act(async () => { tree = Renderer.create(React.createElement(ServiceList, {})); });
  for (let i = 0; i < 3; i += 1) await settle();
}
async function unmount() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  globalThis.bundleWords = words;
  delete globalThis.document;
  delete globalThis.localStorage;
}
const label = (node) => { const walk = (n) => (typeof n === 'string' ? n : (n.children || []).map(walk).join('')); return walk(node); };
const rowOf = (id) => {
  let node = tree.root.findAll((child) => child.props && child.props.id === id && child.props.category !== undefined && child.props.installed !== undefined)[0];
  while (node && node.type !== 'li') node = node.parent;
  return node;
};
const dialog = () => tree.root.findAll((node) => node.props && node.props['data-removal-impact'] !== undefined)[0];
const lines = () => Object.fromEntries(tree.root.findAll((node) => node.props && node.props['data-removal-line'] !== undefined)
  .map((node) => [node.props['data-removal-line'], label(node)]));
const sentenceKeys = () => tree.root.findAll((node) => node.props && node.props['data-removal-sentence'] !== undefined).map((node) => node.props['data-removal-sentence']);
const removeButton = () => tree.root.findAll((node) => node.type === 'button' && label(node).includes(say('services.uninstall')) && node.props.title === undefined)[0];
async function openRemoval(id) {
  const trash = rowOf(id).findAll((node) => node.type === 'button' && node.props.title === globalThis.bundleWords['services.uninstall'])[0];
  assert.ok(trash, `${id} has no remove button`);
  await act(async () => { trash.props.onClick(); });
  await settle();
}
const nameOf = (id) => words[`services.component.name.${id}`] ?? id;

// --- The dialog -------------------------------------------------------------------------
test('Nginx: sites by name with "+N more", the refusal they cause, who needs it, the bundles, the ports, and what is removed and kept', async () => {
  await mount();
  try {
    await openRemoval('nginx');
    assert.equal(dialog().props['data-removal-impact'], 'known');
    const said = lines();
    assert.equal(said.hosted, say('services.removal.hosted.sites', { name: 'Nginx', count: 7, names: sites.slice(0, 5).join(', ') + ' ' + say('services.removal.more', { n: 2 }) }));
    assert.equal(said.blocks, say('services.removal.blocks', { name: 'Nginx' }));
    assert.equal(said.required, say('services.removal.requiredBy', { name: 'Nginx', names: `${nameOf('phpmyadmin')}, ${nameOf('roundcube')}` }));
    assert.equal(said['bundle-web.nginx_php'], say('services.removal.bundle', { bundle: words['services.bundles.name.web_nginx_php'] }));
    assert.equal(said['bundle-db.mariadb.admin'], say('services.removal.bundle', { bundle: words['services.bundles.name.db_mariadb_admin'] }));
    assert.equal(said.ports, say('services.removal.ports', { ports: '80/tcp, 443/tcp' }));
    assert.deepEqual(sentenceKeys(), ['services.removal.removed.packages', 'services.removal.kept.packages']);
    assert.ok(label(tree.root).includes('apt-get purge --auto-remove'));
    // What the server loses comes first; red stays on the Remove button only.
    const sections = tree.root.findAll((node) => node.props && (node.props['data-removal-affected'] !== undefined || node.props['data-removal-removed'] !== undefined || node.props['data-removal-kept'] !== undefined))
      .map((node) => Object.keys(node.props).find((key) => key.startsWith('data-removal-')));
    assert.deepEqual(sections, ['data-removal-affected', 'data-removal-removed', 'data-removal-kept']);
    const blocks = tree.root.findAll((node) => node.props && node.props['data-removal-line'] === 'blocks')[0];
    assert.doesNotMatch(blocks.props.className ?? '', /danger/);
    assert.equal(tree.root.findAll((node) => node.props && node.props['data-removal-nothing'] !== undefined).length, 0);
    // One read, no change started by it.
    const reads = requests.filter((request) => request.path === '/api/v1/service/removal-impact');
    assert.equal(reads.length, 1);
    assert.equal(reads[0].method, 'GET');
    assert.equal(requests.filter((request) => request.method !== 'GET').length, 0, 'opening the dialog changed something');
    // The danger button stays outlined, and enabled: the Panel refuses on its own.
    const button = removeButton();
    assert.ok(button && !button.props.disabled);
    assert.match(button.props.className, /bg-surface text-danger/);
  } finally { await unmount(); }
});

test('PostgreSQL: databases by name, every version removed, and the database-file sentence says who decides', async () => {
  await mount();
  try {
    await openRemoval('postgresql');
    const said = lines();
    assert.equal(said.hosted, say('services.removal.hosted.databases', { name: 'PostgreSQL', count: 2, names: 'shop, blog' }));
    assert.equal(said['bundle-db.postgresql'], say('services.removal.bundle', { bundle: words['services.bundles.name.db_postgresql'] }));
    assert.deepEqual(sentenceKeys(), ['services.removal.removed.packages', 'services.removal.removed.allVersions',
      'services.removal.kept.packages', 'services.removal.kept.databaseFiles']);
  } finally { await unmount(); }
});

test('Postfix: mail addresses, the components that need it, and the mail bundle', async () => {
  await mount();
  try {
    await openRemoval('postfix');
    const said = lines();
    assert.equal(said.hosted, say('services.removal.hosted.mail_addresses', { count: 1, names: 'info@a.example' }));
    assert.equal(said.required, say('services.removal.requiredBy', { name: 'Postfix', names: `${nameOf('rspamd')}, ${nameOf('roundcube')}` }));
    assert.ok(said['bundle-mail.core']);
    assert.ok(said.blocks);
  } finally { await unmount(); }
});

test('Rspamd: mail domains whose mail is no longer checked, never a refusal, and Postfix is rewired', async () => {
  await mount();
  try {
    await openRemoval('rspamd');
    const said = lines();
    assert.equal(said.hosted, say('services.removal.hosted.mail_domains', { count: 1, names: 'a.example' }));
    assert.equal(said.blocks, undefined);
    assert.ok(sentenceKeys().includes('services.removal.removed.mailFilter'));
  } finally { await unmount(); }
});

test('certbot: nothing else depends on it; pacman names its own command', async () => {
  await mount();
  try {
    await openRemoval('certbot');
    assert.deepEqual(lines(), {});
    const nothing = tree.root.findAll((node) => node.props && node.props['data-removal-nothing'] !== undefined)[0];
    assert.equal(label(nothing), say('services.removal.nothing', { name: nameOf('certbot') }));
    assert.ok(label(tree.root).includes('pacman -Rns'));
    assert.ok(!label(tree.root).includes('apt-get'));
  } finally { await unmount(); }
});

test('Roundcube: the webmail tree, not packages; no mailbox yet is "nothing else depends on it"', async () => {
  await mount();
  try {
    await openRemoval('roundcube');
    assert.deepEqual(sentenceKeys(), ['services.removal.removed.webmailTree', 'services.removal.kept.webmailTree']);
    assert.ok(!label(tree.root).includes(say('services.removal.command')));
    assert.ok(tree.root.findAll((node) => node.props && node.props['data-removal-nothing'] !== undefined).length === 1);
  } finally { await unmount(); }
});

test('a component the Panel never removes answers with its own refusal, said as a refusal and not as "could not be read"', async () => {
  await mount();
  try {
    await openRemoval('cron');
    assert.equal(dialog().props['data-removal-impact'], 'refused');
    const text = label(tree.root);
    assert.ok(text.includes(say('err.NATIVE_CRON_REMOVAL_REFUSED')), text.slice(0, 400));
    assert.ok(!text.includes(say('services.removal.unknown', { name: nameOf('cron') })));
  } finally { await unmount(); }
});

test('a read that fails says it could not read, keeps removal allowed, and Retry only reads again', async () => {
  await mount({ impacts: {} });
  try {
    await openRemoval('nginx');
    assert.equal(dialog().props['data-removal-impact'], 'unknown');
    assert.ok(label(tree.root).includes(say('services.removal.unknown', { name: 'Nginx' })));
    assert.ok(removeButton() && !removeButton().props.disabled, 'a failed read blocked the removal');
    const retry = tree.root.findAll((node) => node.type === 'button' && label(node) === say('common.retry'))[0];
    await act(async () => { retry.props.onClick(); });
    await settle();
    assert.equal(requests.filter((request) => request.path === '/api/v1/service/removal-impact').length, 2);
    assert.equal(requests.filter((request) => request.method !== 'GET').length, 0);
  } finally { await unmount(); }
});

test('while the read runs the dialog shows that it reads, with removal still allowed; an answer outside the contract is unknown', async () => {
  await mount({ impacts: () => new Promise(() => {}) });
  try {
    await openRemoval('nginx');
    assert.equal(dialog().props['data-removal-impact'], 'checking');
    assert.ok(label(tree.root).includes(say('services.removal.checking', { name: 'Nginx' })));
    assert.ok(removeButton() && !removeButton().props.disabled);
  } finally { await unmount(); }
  await mount({ impacts: { nginx: { removal: 'everything' } } });
  try {
    await openRemoval('nginx');
    assert.equal(dialog().props['data-removal-impact'], 'unknown');
  } finally { await unmount(); }
});

test('the read is bounded and a late answer is not shown after the bound', () => {
  const list = source('components/ServiceList.tsx');
  assert.match(list, /export const REMOVAL_IMPACT_READ_BOUND_MS = 10_000;/);
  assert.match(list, /setTimeout\(\(\) => \{\s*controller\?\.abort\(\);\s*finish\(\{ state: 'unknown' \}\);\s*\}, REMOVAL_IMPACT_READ_BOUND_MS\)/);
  assert.match(list, /if \(settled\) return;/);
  assert.match(list, /fetch\(`\/api\/v1\/service\/removal-impact\?service_id=\$\{encodeURIComponent\(serviceId\)\}`, \{\s*cache: 'no-store'/);
});

test('Turkish: the dialog speaks Turkish with no raw key', async () => {
  await mount({ catalogue: trWords });
  try {
    await openRemoval('nginx');
    const text = label(tree.root);
    assert.ok(text.includes(trWords['services.removal.affectedTitle']));
    assert.ok(text.includes(fill(trWords['services.removal.more'], { n: 2 })));
    assert.ok(!/services\.removal\./.test(text), 'a key rendered as its name');
  } finally { await unmount(); }
});

test('the sentence chooser follows the uninstall path for each class', () => {
  const base = decodeRemovalImpact(impact({ packages: ['x'] }));
  assert.deepEqual(removalSentences(base), { removed: ['services.removal.removed.packages'], kept: ['services.removal.kept.packages'], command: 'apt-get purge --auto-remove' });
  assert.equal(removalSentences({ ...base, package_family: 'dnf' }).command, null, 'no command is claimed where the uninstall does not run');
  assert.deepEqual(removalSentences({ ...base, database_tools_reconfigured: true }).removed, ['services.removal.removed.packages', 'services.removal.removed.databaseTools']);
  assert.deepEqual(removalSentences({ ...base, hosted: { kind: 'databases', count: 0, names: [], blocks_removal: false } }).kept,
    ['services.removal.kept.packages', 'services.removal.kept.databaseFiles']);
  assert.deepEqual(removalSentences(decodeRemovalImpact(impact({ removal: 'webmail_tree' }))).removed, ['services.removal.removed.webmailTree']);
  for (const bad of [null, {}, impact({ hosted: { kind: 'planets', count: 1, names: [], blocks_removal: false } }), impact({ required_by: [1] }), impact({ hosted: { kind: 'sites', count: -1, names: [], blocks_removal: true } })]) {
    assert.equal(decodeRemovalImpact(bad), null);
  }
});

test('every removal text exists in English and Turkish in the components part, Turkish not English', () => {
  const keys = Object.keys(enComponents).filter((key) => key.startsWith('services.removal.'));
  assert.ok(keys.length >= 28);
  for (const key of keys) {
    assert.equal(typeof trComponents[key], 'string', key);
    if (!['services.removal.hosted.php_sites'].includes(key)) assert.notEqual(trComponents[key], enComponents[key], key);
  }
  for (const key of Object.keys(trComponents).filter((key) => key.startsWith('services.removal.'))) assert.ok(key in enComponents, key);
  // The texts say what the code does: no mockup sentence about kept
  // configuration files survives, and the kept text names who decides.
  assert.ok(!/configuration (files|backups) (stay|are not)/i.test(Object.values(enComponents).join('\n')));
  assert.match(enComponents['services.removal.removed.packages'], /configuration files/);
  assert.match(enComponents['services.removal.kept.packages'], /decided by your distribution/);
});

// --- The first visit ------------------------------------------------------------------
test('the first visit opens the installed view; a stored "catalog" opens the catalogue; an unknown value is a first visit', async () => {
  for (const [stored, shown] of [[undefined, false], [null, false], ['installed', false], ['catalog', true], ['something-else', false]]) {
    await mount({ stored });
    try {
      assert.equal(rowOf('redis') !== undefined, shown, `stored ${stored}: the not-installed row`);
      assert.ok(rowOf('nginx'), `stored ${stored}: an installed row`);
    } finally { await unmount(); }
  }
});
