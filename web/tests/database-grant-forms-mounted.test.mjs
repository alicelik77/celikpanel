import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import Renderer, { act } from 'react-test-renderer';
import React from 'react';
import { compileSource, dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';

// alpha.134: database rights by exact name on the MariaDB page. The block
// only reads where the server's own rewrite stands (it never starts work):
// one guidance block per MariaDB with the reason, who acts, the next step and
// how it resumes; the narrowed rights that also reached other databases; the
// rights left as they are, each with why it matters and its statement under
// a closed Details. Administrators only. Mounted with the shipped kit, remote
// layer and route-scoped words; the stand-in catalogue answers keys.
// alpha.134: veritabanı hakları tam adla; bağlı hâlde sınanır.

const stub = dataModule(`
  import React from '${reactURL}';
  export const ShieldCheck = () => null;
  export const useI18n = () => ({ t: (key, vars) => (vars ? key + JSON.stringify(vars) : key), locale: 'xx' });
  export const useAuth = () => ({ role: globalThis.currentRole });
`);
const shared = sharedLayer(stub);
const routeCopy = (name) => compileSource(`i18n/${name}.ts`, (specifier) => (
  specifier === './index' ? stub : compileSource(`i18n/${specifier.slice(2)}.ts`, () => stub)
));
const own = { '/i18n/dbServer': routeCopy('dbServer'), '/auth/AuthContext': stub };
const { DatabaseGrantFormsBlock, engineGuidance, DATABASE_GRANT_FORMS_URL } = await import(shared.compile('components/DatabaseGrantFormsBlock.tsx', own));
const { enDbServer } = await import(compileSource('i18n/dbServer/en.ts', () => stub));
const { trDbServer } = await import(compileSource('i18n/dbServer/tr.ts', () => stub));

const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
let calls = [];
function serve(view, status = 200) {
  calls = [];
  globalThis.fetch = async (input, init = {}) => {
    calls.push({ url: String(input), method: (init.method ?? 'GET').toUpperCase() });
    return new Response(JSON.stringify(view), { status, headers: { 'Content-Type': 'application/json' } });
  };
}
globalThis.window ??= {
  addEventListener() {}, removeEventListener() {},
  setInterval: (...args) => setInterval(...args), clearInterval: (id) => clearInterval(id),
  setTimeout: (...args) => setTimeout(...args), clearTimeout: (id) => clearTimeout(id),
};
globalThis.document ??= { addEventListener() {}, removeEventListener() {}, visibilityState: 'visible' };
let tree;
async function mount() {
  await act(async () => { tree = Renderer.create(React.createElement(DatabaseGrantFormsBlock)); });
  await act(async () => { await settled(); await settled(); });
}
const text = () => JSON.stringify(tree.toJSON() ?? '');
const withoutDetails = (node) => {
  if (!node || typeof node !== 'object') return node;
  if (Array.isArray(node)) return node.map(withoutDetails);
  if (node.type === 'details') return null;
  return { ...node, children: node.children ? node.children.map(withoutDetails) : node.children };
};

const engine = (state, reason = '', over = {}) => ({ key: 'mariadb@this-machine:3306', name: 'local', local: true, state, reason, version: '11.8.6-MariaDB',
  checked_at: '2026-11-13 20:42:15', narrowed: 2, left: 1, waiting: 0, ...over });
const view = (engines, narrowed = [], left = []) => ({ engines, narrowed, left });

test('the words exist in English and Turkish, the same keys, every engine state and reason has its title', () => {
  assert.deepEqual(Object.keys(trDbServer).sort(), Object.keys(enDbServer).sort());
  const t = (key, vars) => (enDbServer[key] ?? `MISSING:${key}`) + (vars ? JSON.stringify(vars) : '');
  const go = readFileSync(new URL('../../cmd/panel/database_grant_forms.go', import.meta.url), 'utf8');
  const reasons = [...go.matchAll(/grantFormsReason\w+\s*=\s*"([a-z_]+)"/g)].map((m) => m[1]);
  assert.ok(reasons.length >= 10, `the panel names ${reasons.length} reasons`);
  for (const state of ['waiting', 'blocked']) {
    for (const reason of reasons) {
      const g = engineGuidance(t, engine(state, reason));
      for (const part of [g.title, g.why, g.next, g.then]) assert.doesNotMatch(part ?? '', /MISSING/, `${state}/${reason}: ${JSON.stringify(g)}`);
    }
  }
  assert.equal(engineGuidance(t, engine('done')).tone, 'success');
  assert.equal(engineGuidance(t, engine('running')).tone, 'progress');
  assert.equal(engineGuidance(t, engine('list_only', 'not_mariadb', { version: '8.0.36' })).tone, 'neutral');
  // Waiting for a verified update needs nobody; a refused account needs the reader.
  assert.ok(!engineGuidance(t, engine('waiting', 'update_verifying')).next);
  assert.ok(engineGuidance(t, engine('blocked', 'credential_refused')).next);
  for (const value of Object.values(trDbServer)) assert.doesNotMatch(value, /\b(the|and|is not)\b/, `English left in Turkish: ${value}`);
  // A root-only command is written with sudo.
  for (const value of [...Object.values(enDbServer), ...Object.values(trDbServer)]) assert.doesNotMatch(value, /(?<!sudo )systemctl restart/);
});

test('nobody but an administrator reads or sees it', async () => {
  for (const role of ['customer', 'reseller']) {
    globalThis.currentRole = role;
    serve(view([engine('done')]));
    await mount();
    assert.equal(tree.toJSON(), null);
    assert.equal(calls.length, 0);
    await act(async () => tree.unmount());
  }
});

test('an administrator reads its state only, sees the guidance, the narrowed and the left rights; the statement is under Details', async () => {
  globalThis.currentRole = 'admin';
  const narrowed = [{ engine: 'mariadb@this-machine:3306', account: 's5__x_user@localhost', pattern: 's5__x', database: 's5__x', kind: 'ours', reached: ['s51_x'] }];
  const left = [{ engine: 'mariadb@this-machine:3306', account: 'hand@%', pattern: 'shop_a', kind: 'not_ours', reason: 'not_provable', reached: ['shop1a'],
    statement: "GRANT SELECT ON `shop\\_a`.* TO 'hand'@'%';" }];
  serve(view([engine('blocked', 'credential_refused')], narrowed, left));
  await mount();
  assert.deepEqual(calls.map((c) => `${c.method} ${c.url}`), [`GET ${DATABASE_GRANT_FORMS_URL}`], 'the block only reads');
  const all = text();
  assert.match(all, /dbgf\.state\.credential_refused/);
  assert.match(all, /dbgf\.state\.credential_refused\.next/);
  assert.match(all, /dbgf\.narrowed\.row.*s51_x/);
  assert.match(all, /dbgf\.left\.reason\.not_provable/);
  assert.match(all, /dbgf\.left\.reachesNow.*shop1a/);
  const inline = JSON.stringify(withoutDetails(tree.toJSON()));
  assert.doesNotMatch(inline, /GRANT SELECT/, 'the statement is shown only under Details');
  assert.match(all, /GRANT SELECT/);
  await act(async () => tree.unmount());
});

test('a view that could not be read says so with one retry, never "nothing to do"', async () => {
  globalThis.currentRole = 'admin';
  serve({ error: 'x' }, 500);
  await mount();
  const all = text();
  assert.match(all, /dbgf\.unread/);
  assert.doesNotMatch(all, /dbgf\.state\.done/);
  await act(async () => tree.unmount());
});
