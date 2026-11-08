// alpha.130: a removed PostgreSQL version's cluster whose files stayed. One
// that holds only what initdb made says it goes by itself and offers nothing;
// one that may hold the owner's data says why it is kept and offers one
// "Archive and remove", asked in the product's own dialogue and followed
// through the server's steps; the end names the archive and the list is read
// again. With several live clusters the PostgreSQL page says whose files its
// editors open.
// Kaldırılmış PostgreSQL sürümünden kalan küme ve PostgreSQL sayfasında küme seçimi.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { enDatabaseClustersScreens } from '../src/i18n/screens/databaseClusters/en.ts';
import { trDatabaseClustersScreens } from '../src/i18n/screens/databaseClusters/tr.ts';
import { enPgLeftoversScreens } from '../src/i18n/screens/pgLeftovers/en.ts';
import { trPgLeftoversScreens } from '../src/i18n/screens/pgLeftovers/tr.ts';
import { enOpstepsScreens } from '../src/i18n/screens/opsteps/en.ts';

const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const en = { ...enDatabaseClustersScreens, ...enPgLeftoversScreens, ...enOpstepsScreens, 'common.cancel': 'Cancel', 'common.retry': 'Try again' };
const tr = { ...trDatabaseClustersScreens, ...trPgLeftoversScreens };
const fill = (text, vars = {}) => Object.entries(vars).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
globalThis.cardWords = en;
const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null, CheckCircle2 = () => null, Info = () => null, XCircle = () => null, X = () => null, Loader2 = () => null;
  export const Trash2 = () => null, ArrowRightLeft = () => null, Database = () => null, DownloadCloud = () => null, Layers = () => null, Network = () => null;
  export const Settings = () => null, ShieldCheck = () => null, FileCode = () => null;
  const fill = (text, vars) => Object.entries(vars || {}).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
  export const useI18n = () => ({ t: (key, vars) => (globalThis.cardWords?.[key] !== undefined ? fill(globalThis.cardWords[key], vars) : key), locale: 'en' });
  export const useNavigate = () => () => true;
  export const useComponentOperation = () => globalThis.pgOperation;
  // The PostgreSQL page's neighbours: only the editors matter here.
  export const ServiceShell = ({ children }) => React.createElement('div', null, children);
  export const RepoCard = () => null, PostgreSQLClustersCard = () => null, ComponentPanels = () => null, ConfigEditor = () => null;
  export const editorCardHeight = '';
  export const useComponentConfigFiles = () => ({ files: { state: 'known', value: globalThis.pgFiles }, retry() {}, reading: false });
  const editor = (kind) => ({ configPath }) => {
    React.useEffect(() => { (globalThis.pgMounts ??= []).push(kind + ':' + configPath); }, []);
    return React.createElement('section', { 'data-editor': kind, 'data-path': configPath });
  };
  export const PostgreSQLSettings = editor('settings');
  export const PostgreSQLAccessRules = editor('access');
`);
const shared = sharedLayer(stub);
const libURL = shared.compile('lib/pgClusters.ts');
const { decodePGClusters } = await import(libURL);
const { PostgreSQLClustersCard } = await import(shared.compile('components/PostgreSQLClusters.tsx', { '/lib/pgClusters': libURL }));
const { PostgreSQLManagement, pgClustersFromFiles } = await import(shared.compile('components/PostgreSQLManagement.tsx'));

const originalFetch = globalThis.fetch;
let tree;
let requests = [];
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
async function settle() { await act(async () => { await new Promise((resolve) => setTimeout(resolve, 15)); }); }
const live = { version: '18', name: 'main', port: 5433, configured_port: 5433, status: 'online', online: true, data_directory: '/var/lib/postgresql/18/main', config_path: '/etc/postgresql/18/main/postgresql.conf', databases: [], databases_known: true };
const removed = (leftover) => ({ version: '16', name: 'main', port: 5432, configured_port: 5432, status: 'down,binaries_missing', online: false, data_directory: '/var/lib/postgresql/16/main', config_path: '/etc/postgresql/16/main/postgresql.conf', databases: [], databases_known: false, databases_error: 'not running', ...(leftover ? { leftover } : {}) });
const clusters = (old) => ({ state: 'read', supported: true, next_port: 5434, used_ports: [5432, 5433], creates_cluster: true, repository_on: false, clusters: old ? [old, live] : [live], offers: [], records: {}, unplaced: [], mismatches: [], moves: [] });
const ARCHIVE = '/var/backups/celikpanel/postgresql-16-main-20261108T101500Z.tar.gz';

async function mount(reads, { words = en, post = () => answer({}) } = {}) {
  globalThis.cardWords = words;
  globalThis.document = { addEventListener() {}, removeEventListener() {}, getElementById: () => null, activeElement: null, body: { style: {} } };
  globalThis.window ??= globalThis;
  globalThis.operationSteps = [];
  globalThis.operationEnds = [];
  globalThis.confirmed = [];
  globalThis.confirm = (text, request) => { globalThis.confirmed.push(request); return true; };
  requests = [];
  globalThis.pgOperation = { locked: false, startInstall: async () => true };
  let n = 0;
  globalThis.fetch = async (input, init = {}) => {
    const url = String(input);
    requests.push({ url, method: init.method ?? 'GET', body: init.body, headers: init.headers ?? {} });
    if (init.method === 'POST') return post(url, init);
    if (url === '/api/v1/service/postgresql/clusters') return answer(reads[Math.min(n++, reads.length - 1)]);
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
  delete globalThis.confirm;
}
const text = () => { const walk = (n) => (typeof n === 'string' ? n : Array.isArray(n) ? n.map(walk).join('') : (n?.children || []).map(walk).join('')); return walk(tree.toJSON()); };
const attr = (name, value) => tree.root.findAll((node) => typeof node.type === 'string' && node.props && node.props[name] !== undefined && (value === undefined || node.props[name] === value));
const reads = () => requests.filter((request) => request.method === 'GET' && request.url === '/api/v1/service/postgresql/clusters').length;

test('a leftover is decoded strictly; a malformed one is left out', () => {
  const decoded = decodePGClusters(clusters(removed({ fresh: false, reason: 'it has a database made after initdb', data_bytes: 52_000_000 })));
  assert.deepEqual(decoded.clusters[0].leftover, { fresh: false, reason: 'it has a database made after initdb', data_bytes: 52_000_000 });
  assert.deepEqual(decodePGClusters(clusters(removed({ fresh: true, data_bytes: 0 }))).clusters[0].leftover, { fresh: true, data_bytes: 0 });
  for (const bad of [{ fresh: 'yes', data_bytes: 1 }, { fresh: true }, { fresh: true, data_bytes: -1 }, { fresh: false, data_bytes: 1, reason: 7 }, [true], 'fresh']) {
    assert.equal(decodePGClusters(clusters(removed(bad))).clusters[0].leftover, undefined, JSON.stringify(bad));
  }
  assert.equal('leftover' in decodePGClusters(clusters(removed())).clusters[0], false, 'an older answer has none');
});

test('a fresh leftover says it goes by itself and offers nothing', async () => {
  await mount([clusters(removed({ fresh: true, data_bytes: 40_000_000 }))]);
  try {
    const shown = text();
    assert.ok(shown.includes(fill(en['pgc.leftover.badge'], { version: '16' })));
    assert.ok(shown.includes(fill(en['pgc.leftover.fresh'], { version: '16', port: 5432 })));
    assert.equal(attr('data-pg-leftover-archive').length, 0, 'no button: there is nothing to do');
    assert.equal(attr('data-pg-port-offer').length, 1, 'Port is offered for the running cluster only');
    assert.equal(attr('data-pg-move-offer').length, 1, 'Move is offered for the running cluster only');
    assert.equal(attr('data-pg-removed').length, 0, 'the alpha.127 hint is not repeated');
  } finally { await unmount(); }
});

test('a kept leftover says why, and Archive and remove asks first, follows the server steps and names the archive', async () => {
  const after = clusters();
  await mount([clusters(removed({ fresh: false, reason: 'it has roles made after initdb (shop)', data_bytes: 9_000_000 })), after], {
    post: (url) => (url === '/api/v1/service/postgresql/clusters/leftover'
      ? answer({ removed: true, cluster: '16/main', archive: ARCHIVE, removed_paths: ['/etc/postgresql/16/main', '/var/lib/postgresql/16/main'] })
      : answer({}, 404)),
  });
  try {
    const shown = text();
    assert.ok(shown.includes(fill(en['pgc.leftover.kept'], { version: '16', port: 5432 })));
    assert.ok(shown.includes(fill(en['pgc.leftover.reason'], { reason: 'it has roles made after initdb (shop)' })));
    assert.equal(attr('data-pg-port-offer').length, 1);
    const before = reads();
    await act(async () => { attr('data-pg-leftover-archive')[0].props.onClick(); });
    await settle();
    const asked = globalThis.confirmed[0];
    assert.equal(asked.title, fill(en['pgc.leftover.confirmTitle'], { cluster: '16/main' }));
    assert.equal(asked.message, fill(en['pgc.leftover.confirmBody'], { version: '16', name: 'main', data: '/var/lib/postgresql/16/main', port: 5432 }));
    assert.equal(asked.tone, 'danger');
    const posts = requests.filter((request) => request.method === 'POST');
    assert.equal(posts.length, 1);
    assert.equal(posts[0].url, '/api/v1/service/postgresql/clusters/leftover');
    assert.deepEqual(JSON.parse(posts[0].body), { version: '16', name: 'main' });
    assert.ok(posts[0].headers['X-CelikPanel-Request-Id'], 'the window follows the server steps');
    assert.deepEqual(globalThis.operationSteps[0].map((step) => step.label), [
      fill(en['opstep.pgLeftover.check'], { cluster: '16/main' }), en['opstep.pgLeftover.archive'], en['opstep.common.records'],
    ]);
    const done = fill(en['pgc.leftover.done'], { cluster: '16/main', archive: ARCHIVE, port: 5432 });
    assert.deepEqual(globalThis.operationEnds[0], { state: 'done', message: done });
    assert.ok(text().includes(done), 'the card keeps the archive name');
    assert.ok(reads() > before, 'the list is read again by itself');
    assert.equal(attr('data-pg-leftover').length, 0, 'the removed cluster is gone from the list');
  } finally { await unmount(); }
});

test('a refusal says nothing was changed; a removal that stopped after the archive names the archive', async () => {
  const kept = clusters(removed({ fresh: false, reason: 'it has a database made after initdb', data_bytes: 1 }));
  await mount([kept], { words: tr, post: () => answer({ error: 'CelikPanel still records databases on cluster 16/main.', code: 'PG_LEFTOVER_HAS_RECORDS' }, 409) });
  try {
    await act(async () => { attr('data-pg-leftover-archive')[0].props.onClick(); });
    await settle();
    assert.deepEqual(globalThis.operationEnds[0], { state: 'blocked', reason: tr['err.PG_LEFTOVER_HAS_RECORDS'] });
    assert.ok(text().includes(tr['err.PG_LEFTOVER_HAS_RECORDS']));
  } finally { await unmount(); }
  await mount([kept], { post: () => answer({ error: 'cluster 16/main could not be archived (2.1 GB free, 3.0 GB needed); nothing was removed', code: 'PG_LEFTOVER_ARCHIVE_FAILED', changed: false }, 409) });
  try {
    await act(async () => { attr('data-pg-leftover-archive')[0].props.onClick(); });
    await settle();
    assert.deepEqual(globalThis.operationEnds[0], { state: 'blocked', reason: fill(en['pgc.leftover.unchanged'], { reason: 'cluster 16/main could not be archived (2.1 GB free, 3.0 GB needed); nothing was removed' }) });
  } finally { await unmount(); }
  const stopped = 'cluster 16/main was archived in ' + ARCHIVE + ', but removing it stopped: permission denied; remove the rest yourself or try again';
  await mount([kept], { post: () => answer({ error: stopped, code: 'PG_LEFTOVER_REMOVE_FAILED', changed: true, archive: ARCHIVE }, 500) });
  try {
    await act(async () => { attr('data-pg-leftover-archive')[0].props.onClick(); });
    await settle();
    assert.deepEqual(globalThis.operationEnds[0], { state: 'failed', reason: fill(en['pgc.leftover.partial'], { cluster: '16/main', archive: ARCHIVE, reason: stopped }) });
  } finally { await unmount(); }
});

test('an older answer without a description keeps the alpha.127 hint', async () => {
  await mount([clusters(removed())]);
  try {
    assert.equal(attr('data-pg-removed').length, 1);
    assert.equal(attr('data-pg-leftover').length, 0);
  } finally { await unmount(); }
});

test('the PostgreSQL page offers a cluster choice only with several clusters, and its editors follow it', async () => {
  const debian = ['/etc/postgresql/17/main/postgresql.conf', '/etc/postgresql/17/main/pg_hba.conf', '/etc/postgresql/18/main/postgresql.conf', '/etc/postgresql/18/main/pg_hba.conf', '/etc/postgresql/18/main/pg_ident.conf'];
  assert.deepEqual(pgClustersFromFiles(debian).map((cluster) => `${cluster.version}/${cluster.name}`), ['18/main', '17/main']);
  assert.deepEqual(pgClustersFromFiles(['/var/lib/postgres/data/postgresql.conf', '/var/lib/postgres/data/pg_hba.conf']), [], 'Arch has no clusters to choose from');
  const page = async (files) => {
    globalThis.cardWords = en;
    globalThis.pgFiles = files;
    globalThis.pgMounts = [];
    await act(async () => { tree = Renderer.create(React.createElement(PostgreSQLManagement, { onBack() {} })); });
  };
  for (const files of [['/etc/postgresql/18/main/postgresql.conf', '/etc/postgresql/18/main/pg_hba.conf'], ['/var/lib/postgres/data/postgresql.conf', '/var/lib/postgres/data/pg_hba.conf']]) {
    await page(files);
    try {
      assert.equal(attr('data-pg-cluster-pick').length, 0, 'one cluster: nothing extra');
      assert.equal(attr('data-editor', 'settings')[0].props['data-path'], files[0]);
    } finally { await act(async () => tree.unmount()); tree = undefined; }
  }
  await page(debian);
  try {
    assert.deepEqual(attr('data-pg-cluster-choice').map((node) => node.props['data-pg-cluster-choice']), ['18/main', '17/main']);
    assert.ok(text().includes(fill(en['pgc.pick.cluster'], { version: '18', name: 'main' })));
    assert.equal(attr('data-pg-cluster-choice', '18/main')[0].props['aria-pressed'], true, 'the highest version first');
    assert.equal(attr('data-editor', 'settings')[0].props['data-path'], '/etc/postgresql/18/main/postgresql.conf');
    await act(async () => { attr('data-pg-cluster-choice', '17/main')[0].props.onClick(); });
    assert.equal(attr('data-editor', 'settings')[0].props['data-path'], '/etc/postgresql/17/main/postgresql.conf');
    assert.deepEqual(globalThis.pgMounts, ['settings:/etc/postgresql/18/main/postgresql.conf', 'settings:/etc/postgresql/17/main/postgresql.conf'], 'the editor is mounted afresh for the other file');
    const tabs = tree.root.findAll((node) => node.type === 'button' && node.props['aria-pressed'] !== undefined && node.props['data-pg-cluster-choice'] === undefined);
    const accessTab = tabs[1];
    await act(async () => { accessTab.props.onClick(); });
    assert.equal(attr('data-editor', 'access')[0].props['data-path'], '/etc/postgresql/17/main/pg_hba.conf');
  } finally { await act(async () => tree.unmount()); tree = undefined; }
});

test('every new sentence is in both languages with the same placeholders, and the card uses only keys that exist', () => {
  const holes = (value) => [...value.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort().join(',');
  for (const key of Object.keys(enPgLeftoversScreens)) {
    assert.ok(trPgLeftoversScreens[key], `no Turkish for ${key}`);
    assert.equal(holes(trPgLeftoversScreens[key]), holes(enPgLeftoversScreens[key]), `${key}: different values`);
  }
  for (const file of ['components/PostgreSQLClusters.tsx', 'components/PostgreSQLManagement.tsx']) {
    for (const [, key] of source(file).matchAll(/'(pgc\.(?:leftover|pick)\.[A-Za-z.]+)'/g)) assert.ok(enPgLeftoversScreens[key], `${file}: no words for ${key}`);
  }
  assert.match(source('components/PostgreSQLClusters.tsx'), /flowSteps\(t, 'pgLeftover', \['check!', 'archive', 'records'\]/);
  assert.match(source('components/PostgreSQLManagement.tsx'), /<PostgreSQLSettings key=\{mainConf\}/);
});

// alpha.130 lab: one PostgreSQL version could not be removed at all. With two
// versions installed, each cluster row offers its version's removal; it asks
// first, sends the version's package, names the owner's files that were kept
// and reads the list again. With one version installed it is not offered (the
// Components list removes the whole component).
test('with two versions installed, one version is removed from its cluster row', async () => {
  const sixteen = { ...live, version: '16', port: 5432, data_directory: '/var/lib/postgresql/16/main', config_path: '/etc/postgresql/16/main/postgresql.conf' };
  const both = { ...clusters(), clusters: [sixteen, live] };
  await mount([both, clusters()], {
    post: (url) => (url === '/api/v1/service/uninstall'
      ? answer({ removed: true, success: true, owner_files_kept: ['/etc/systemd/system/postgresql@16-main.service.d/override.conf'] })
      : answer({}, 404)),
  });
  try {
    assert.equal(attr('data-pg-remove-major').length, 2, 'each version of two is offered');
    const before = reads();
    await act(async () => { attr('data-pg-remove-major', '16')[0].props.onClick(); });
    await settle();
    const asked = globalThis.confirmed[0];
    assert.equal(asked.title, fill(en['pgc.removeMajor.confirmTitle'], { version: '16' }));
    assert.equal(asked.tone, 'danger');
    const posts = requests.filter((request) => request.method === 'POST');
    assert.deepEqual(JSON.parse(posts[0].body), { service_id: 'postgresql', package: 'postgresql-16' });
    const done = fill(en['pgc.removeMajor.doneKept'], { version: '16', cluster: '16/main', port: 5432, files: '/etc/systemd/system/postgresql@16-main.service.d/override.conf' });
    assert.deepEqual(globalThis.operationEnds[0], { state: 'done', message: done });
    assert.ok(reads() > before, 'the list is read again by itself');
    assert.equal(attr('data-pg-remove-major').length, 0, 'one version left: no version removal is offered');
  } finally { await unmount(); }
});
