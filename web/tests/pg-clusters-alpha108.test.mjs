// alpha.108: PostgreSQL majors side by side, mounted, in English and Turkish.
// Every cluster is shown with CelikPanel's records on it; another major is
// offered with the port its cluster would get and installs through the
// install preview and the component operation; a port change is shown first
// (the line, who connects, the restart) and sent with the file's SHA-256; a
// move's switch is its own confirmed step after the proof.
// PostgreSQL ana sürümleri yan yana, bağlanmış olarak.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { enDatabaseClustersScreens } from '../src/i18n/screens/databaseClusters/en.ts';
import { trDatabaseClustersScreens } from '../src/i18n/screens/databaseClusters/tr.ts';

const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const common = { 'common.cancel': 'Cancel', 'common.retry': 'Try again' };
const en = { ...enDatabaseClustersScreens, ...common };
const tr = { ...trDatabaseClustersScreens, 'common.cancel': 'Vazgeç', 'common.retry': 'Tekrar dene' };
const fill = (text, vars = {}) => Object.entries(vars).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
globalThis.cardWords = en;
const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null, CheckCircle2 = () => null, Info = () => null, XCircle = () => null, X = () => null, Loader2 = () => null;
  export const ArrowRightLeft = () => null, Database = () => null, DownloadCloud = () => null, Layers = () => null, Network = () => null;
  const fill = (text, vars) => Object.entries(vars || {}).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
  export const useI18n = () => ({ t: (key, vars) => (globalThis.cardWords?.[key] !== undefined ? fill(globalThis.cardWords[key], vars) : key), locale: 'en' });
  export const useNavigate = () => () => true;
  export const useComponentOperation = () => globalThis.pgOperation;
`);
const shared = sharedLayer(stub);
const { PostgreSQLClustersCard } = await import(shared.compile('components/PostgreSQLClusters.tsx', { '/lib/pgClusters': shared.compile('lib/pgClusters.ts') }));

const originalFetch = globalThis.fetch;
let tree;
let requests = [];
let installs = [];
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
async function settle() { await act(async () => { await new Promise((resolve) => setTimeout(resolve, 15)); }); }
const proof = { equal: true, tables_total: 2, tables: [{ schema: 'public', table: 'orders', source: 10, target: 10 }, { schema: 'public', table: 'users', source: 3, target: 3 }], mismatches: 0, source_version: '17', target_version: '18', roles_created: ['shop_user'], roles_kept: [] };
const clusters = (extra = {}) => ({
  state: 'read', supported: true, next_port: 5434, used_ports: [5432, 5433], creates_cluster: true, repository_on: true,
  clusters: [
    { version: '17', name: 'main', port: 5432, configured_port: 5432, status: 'online', online: true, data_directory: '/var/lib/postgresql/17/main', config_path: '/etc/postgresql/17/main/postgresql.conf', databases: ['shop'], databases_known: true },
    { version: '18', name: 'main', port: 5433, configured_port: 5433, status: 'online', online: true, data_directory: '/var/lib/postgresql/18/main', config_path: '/etc/postgresql/18/main/postgresql.conf', databases: [], databases_known: true },
  ],
  offers: [
    { major: '16', package: 'postgresql-16', candidate: '16.10-1.pgdg13+1', origin: 'pgdg', needs_repository: false, port: 5434 },
    { major: '15', package: 'postgresql-15', needs_repository: true, port: 5434 },
  ],
  records: { '17/main': [{ database_id: 3, name: 'shop', domain: 'shop.example.test', subscription_id: 1, server_id: 1, host: 'localhost', port: 5432, users: ['shop_user'] }], '18/main': [] },
  unplaced: [], mismatches: [], moves: [], ...extra,
});
const portPreview = { state: 'read', records: [{ database_id: 3, name: 'shop', domain: 'shop.example.test', subscription_id: 1, server_id: 1, host: 'localhost', port: 5432, users: ['shop_user'] }], preview: {
  config_path: '/etc/postgresql/17/main/postgresql.conf', config_sha256: 'a'.repeat(64), line_before: 'port = 5432', line_after: 'port = 5440', from: 5432, to: 5440,
  sessions: [{ database: 'shop', user: 'shop_user', client: 'local', application: 'psql', count: 2 }], sessions_known: true, phppgadmin_port: 5432,
} };
async function mount(reads, { words = en, post = () => answer({}) } = {}) {
  globalThis.cardWords = words;
  globalThis.document = { addEventListener() {}, removeEventListener() {}, getElementById: () => null, activeElement: null, body: { style: {} } };
  requests = [];
  installs = [];
  globalThis.pgOperation = { locked: false, startInstall: async (request) => { installs.push(request); return true; } };
  let n = 0;
  globalThis.fetch = async (input, init = {}) => {
    const url = String(input);
    requests.push({ url, method: init.method ?? 'GET', body: init.body });
    if (init.method === 'POST') return post(url, init);
    if (url === '/api/v1/service/postgresql/clusters') return answer(reads[Math.min(n++, reads.length - 1)]);
    if (url.startsWith('/api/v1/service/postgresql/clusters/port-preview')) return answer(portPreview);
    if (url.startsWith('/api/v1/service/install-preview')) return answer({ state: 'known', installs: ['postgresql-16'], notable: [], refusals: [] });
    return answer({ error: 'no route' }, 404);
  };
  await act(async () => { tree = Renderer.create(React.createElement(PostgreSQLClustersCard)); });
  await settle();
}
async function unmount() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  delete globalThis.document;
}
const text = () => { const walk = (n) => (typeof n === 'string' ? n : Array.isArray(n) ? n.map(walk).join('') : (n?.children || []).map(walk).join('')); return walk(tree.toJSON()); };
const attr = (name, value) => tree.root.findAll((node) => typeof node.type === 'string' && node.props && node.props[name] !== undefined && (value === undefined || node.props[name] === value));

test('every cluster is shown with its port, databases and CelikPanel’s records; another major says its port', async () => {
  await mount([clusters()]);
  try {
    const all = text();
    assert.ok(all.includes(fill(en['pgc.cluster.name'], { version: '17', cluster: '17/main' })));
    assert.ok(all.includes(fill(en['pgc.cluster.record'], { name: 'shop', domain: 'shop.example.test', host: 'localhost', port: 5432, users: 'shop_user' })));
    assert.ok(all.includes(fill(en['pgc.offers.pgdg'], { major: '16', candidate: '16.10-1.pgdg13+1', port: 5434 })));
    assert.ok(all.includes(fill(en['pgc.offers.needsRepo'], { major: '15' })));
    assert.equal(attr('data-pg-install-offer').length, 1, 'only an offered major has an Install button');
    await act(async () => { attr('data-pg-install-offer', '16')[0].props.onClick(); });
    await settle();
    assert.ok(text().includes(fill(en['pgc.install.port'], { major: '16', port: 5434, used: '5432, 5433' })));
    assert.ok(text().includes(en['pgc.firewall']));
    assert.ok(requests.some((request) => request.url.startsWith('/api/v1/service/install-preview')), 'the install preview is read');
    await act(async () => { attr('data-pg-install-confirm')[0].props.onClick(); });
    await settle();
    assert.equal(installs.length, 1);
    assert.deepEqual({ ...installs[0] }, { serviceId: 'postgresql', name: 'PostgreSQL 16', package: 'postgresql-16', checkInstalls: true, confirmInstalls: [] });
  } finally { await unmount(); }
});

test('a port change is shown first and sent with the file it was shown over', async () => {
  await mount([clusters()], { post: (url) => (url === '/api/v1/service/postgresql/clusters/port' ? answer({ changed: true, proved: true, from: 5432, to: 5440, records: 1 }) : answer({}, 404)) });
  try {
    await act(async () => { attr('data-pg-port-offer')[0].props.onClick(); });
    assert.equal(attr('data-pg-port-confirm')[0].props.disabled, true, 'nothing can be sent before the plan is read');
    await act(async () => { attr('data-pg-port-input')[0].props.onChange({ target: { value: '5440' } }); });
    await act(async () => { attr('data-pg-port-preview')[0].props.onClick(); });
    await settle();
    const shown = text();
    assert.ok(shown.includes(fill(en['pgc.port.line'], { path: '/etc/postgresql/17/main/postgresql.conf', before: 'port = 5432', after: 'port = 5440' })));
    assert.ok(shown.includes(fill(en['pgc.port.restart'], { cluster: '17/main' })));
    assert.ok(shown.includes(fill(en['pgc.port.records'], { list: 'shop (localhost:5432)', to: 5440 })));
    assert.ok(shown.includes(fill(en['pgc.port.tool'], { port: 5432 })));
    await act(async () => { attr('data-pg-port-confirm')[0].props.onClick(); });
    await settle();
    const post = requests.find((request) => request.method === 'POST');
    assert.deepEqual(JSON.parse(post.body), { version: '17', name: 'main', port: 5440, expected_sha256: 'a'.repeat(64) });
    assert.ok(text().includes(fill(en['pgc.port.done'], { cluster: '17/main', from: 5432, to: 5440, records: 1 })));
  } finally { await unmount(); }
});

test('a port that does not answer is said as put back, in Turkish', async () => {
  await mount([clusters()], { words: tr, post: () => answer({ error: 'cluster 17/main did not answer on port 5440', rolled_back: true, restored: true }, 409) });
  try {
    await act(async () => { attr('data-pg-port-offer')[0].props.onClick(); });
    await act(async () => { attr('data-pg-port-input')[0].props.onChange({ target: { value: '5440' } }); });
    await act(async () => { attr('data-pg-port-preview')[0].props.onClick(); });
    await settle();
    await act(async () => { attr('data-pg-port-confirm')[0].props.onClick(); });
    await settle();
    assert.ok(text().includes(fill(tr['pgc.port.rolledBack'], { cluster: '17/main', from: 5432, to: 5440, reason: 'cluster 17/main did not answer on port 5440' })));
  } finally { await unmount(); }
});

test('a copied move shows its proof and switches only as its own step', async () => {
  const move = { id: 7, database_id: 3, database: 'shop', source_cluster: '17/main', target_cluster: '18/main', source_port: 5432, target_port: 5433, state: 'copied', proof, created_at: '2026-10-25 10:00:00', updated_at: '2026-10-25 10:00:00' };
  await mount([clusters({ moves: [move] }), clusters({ moves: [{ ...move, state: 'switched' }] })], { post: (url) => (url === '/api/v1/service/postgresql/moves/7/switch' ? answer({ state: 'switched', port: 5433 }) : answer({}, 404)) });
  try {
    assert.ok(text().includes(fill(en['pgc.proof.equal'], { tables: 2, from: '17', to: '18' })));
    assert.deepEqual(attr('data-pg-move-action').map((node) => node.props['data-pg-move-action']), ['switch', 'drop-copy']);
    await act(async () => { attr('data-pg-move-action', 'switch')[0].props.onClick(); });
    assert.ok(text().includes(fill(en['pgc.moves.dialog.switch.body'], { database: 'shop', from: '17/main (5432)', to: '18/main (5433)', port: 5433 })));
    assert.equal(requests.filter((request) => request.method === 'POST').length, 0, 'opening the dialogue sends nothing');
    await act(async () => { attr('data-pg-move-action-confirm', 'switch')[0].props.onClick(); });
    await settle();
    assert.equal(requests.filter((request) => request.method === 'POST').length, 1);
    assert.ok(text().includes(fill(en['pgc.moves.done.switch'], { database: 'shop', from: '17/main (5432)', to: '18/main (5433)', port: 5433 })));
    assert.deepEqual(attr('data-pg-move-action').map((node) => node.props['data-pg-move-action']), ['switch-back', 'drop-source']);
  } finally { await unmount(); }
});

test('a server without postgresql-common and a failed read say so', async () => {
  await mount([{ ...clusters(), supported: false, clusters: [], offers: [] }]);
  try {
    assert.ok(text().includes(en['pgc.unsupported']));
  } finally { await unmount(); }
  await mount([{ state: 'unknown' }]);
  try {
    assert.ok(text().includes(en['pgc.unknown']));
    assert.equal(attr('data-pg-port-offer').length, 0);
  } finally { await unmount(); }
});

test('the catalogue part is complete in both languages and every key the card uses exists', () => {
  const card = source('components/PostgreSQLClusters.tsx');
  const used = new Set([...card.matchAll(/'(pgc\.[A-Za-z.]+)'/g)].map((m) => m[1]));
  for (const state of ['copied', 'switched', 'switched_back', 'source_dropped', 'copy_dropped']) used.add(`pgc.moves.state.${state}`);
  for (const state of ['copied', 'switched', 'switched_back']) used.add(`pgc.moves.next.${state}`);
  for (const action of ['switch', 'switch-back', 'drop-source', 'drop-copy']) {
    for (const key of [`pgc.moves.action.${action}`, `pgc.moves.done.${action}`, `pgc.moves.dialog.${action}.title`, `pgc.moves.dialog.${action}.body`]) used.add(key);
  }
  for (const key of used) {
    assert.ok(en[key], `no English for ${key}`);
    assert.ok(tr[key], `no Turkish for ${key}`);
  }
  for (const key of Object.keys(enDatabaseClustersScreens)) {
    assert.ok(trDatabaseClustersScreens[key], `no Turkish for ${key}`);
    const placeholders = (text) => [...text.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort().join(',');
    assert.equal(placeholders(trDatabaseClustersScreens[key]), placeholders(enDatabaseClustersScreens[key]), `${key}: different values`);
  }
});

test('the PostgreSQL page shows the card, MariaDB says plainly why it has none, a removal names the databases', () => {
  assert.match(source('components/PostgreSQLManagement.tsx'), /<PostgreSQLClustersCard \/>/);
  assert.match(source('components/MariaDBManagement.tsx'), /t\('pgc\.mariadb\.noSideBySide'\)/);
  const list = source('components/ServiceList.tsx');
  assert.match(list, /t\('pgc\.removal\.holds'/);
  assert.match(list, /t\('pgc\.removal\.unknown'/);
  // The install goes through the component operation with the preview's confirmations.
  const card = source('components/PostgreSQLClusters.tsx');
  assert.match(card, /checkInstalls: true,\s*confirmInstalls: confirmed/);
  assert.match(card, /installPreviewURL\(\[\{ id: 'postgresql', package: offer\.package \}\]\)/);
});
