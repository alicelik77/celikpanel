import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { compileSource, requestIdentityURL } from './fixtures/shared-layer.mjs';
import { enScreens as englishScreens, trScreens as turkishScreens } from './locale-catalogue.mjs';

// alpha.127 (Boston, 6 Nov 2026): PostgreSQL 16 removed with its data kept on
// 5432, PostgreSQL 18 on 5433. The phpPgAdmin card says which server it names
// against what runs, a kept cluster says it is kept, and a refused PostgreSQL
// change says what is wrong in both languages.

const read = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const phpToolsURL = compileSource('lib/phpTools.ts', (specifier) => {
  if (specifier.endsWith('/requestIdentity')) return requestIdentityURL;
  throw new Error('unexpected import ' + specifier);
});
const { decodePHPTools } = await import(phpToolsURL);

const tool = (extra) => ({ tool: 'phppgadmin', service_id: 'phppgadmin', installed: true, version: '8.3', ready: true, version_installed: true, ...extra });

test('phpPgAdmin servers are decoded, and a malformed list is not claimed', () => {
  const servers = { state: 'differs', owner: 'owner', config_path: '/etc/phppgadmin/config.inc.php',
    configured: [{ port: 5432, online: false, version: '16' }], running: [{ port: 5433, online: true, version: '18' }] };
  const read1 = decodePHPTools({ state: 'read', tools: [tool({ pg_servers: servers })], versions: [] });
  assert.equal(read1.state, 'read');
  assert.deepEqual(read1.tools[0].pg_servers, { ...servers, configured: [{ port: 5432, online: false, version: '16' }] });
  const bad = decodePHPTools({ state: 'read', tools: [tool({ pg_servers: { ...servers, state: 'whatever' } })], versions: [] });
  assert.equal(bad.tools[0].pg_servers, null);
  const older = decodePHPTools({ state: 'read', tools: [tool({})], versions: [] });
  assert.equal(older.tools[0].pg_servers, null);
});

test('every new sentence is in both languages, with the same placeholders', () => {
  const keys = ['pga.server', 'pga.serverPort', 'pga.servers.aligned', 'pga.servers.differs', 'pga.servers.owner', 'pga.servers.none',
    'pgc.cluster.removed', 'pgc.cluster.removedHint', 'err.PG_CLUSTER_UNAVAILABLE', 'err.PG_CLUSTER_UNAVAILABLE.removed',
    'err.PG_CLUSTER_UNAVAILABLE.stopped', 'err.PG_CLUSTER_UNAVAILABLE.none', 'err.PG_CLUSTER_UNAVAILABLE.major_mismatch',
    'err.PG_CLUSTER_UNAVAILABLE.unplaced', 'err.PG_CLUSTER_UNAVAILABLE.action'];
  const holes = (text) => [...text.matchAll(/\{([a-z_]+)\}/g)].map((m) => m[1]).sort().join(',');
  for (const key of keys) {
    assert.ok(englishScreens[key] && turkishScreens[key], `${key} in both languages`);
    assert.equal(holes(englishScreens[key]), holes(turkishScreens[key]), `${key} placeholders`);
  }
});

test('a removed version\'s kept cluster says so, and is not offered a start', () => {
  const source = read('components/PostgreSQLClusters.tsx');
  assert.match(source, /status\.includes\('binaries_missing'\)/);
  assert.match(source, /removed\s*\?\s*<p[^>]*data-pg-removed/);
  assert.match(read('components/PHPToolVersion.tsx'), /<PGToolServersLine servers=\{state\.pg_servers\} \/>/);
});
