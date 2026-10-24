// alpha.107 (D-038 §9): Node.js patch notices, mounted, in English and
// Turkish: a newer patch on the same line is said with the install asked
// first; once it is installed, a site still on the old patch moves only after
// its own question, and a move that did not answer says it was put back.
// Node.js yama bildirimleri, bağlanmış olarak.
import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { enLifecycleScreens } from '../src/i18n/screens/lifecycle/en.ts';
import { trLifecycleScreens } from '../src/i18n/screens/lifecycle/tr.ts';

const en = { ...enLifecycleScreens, 'common.cancel': 'Cancel', 'common.retry': 'Try again' };
const tr = { ...trLifecycleScreens, 'common.cancel': 'Vazgeç', 'common.retry': 'Tekrar dene' };
const fill = (text, vars = {}) => Object.entries(vars).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
globalThis.cardWords = en;
const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null, CheckCircle2 = () => null, Info = () => null, XCircle = () => null, X = () => null, Loader2 = () => null;
  const fill = (text, vars) => Object.entries(vars || {}).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
  export const useI18n = () => ({ t: (key, vars) => (globalThis.cardWords?.[key] !== undefined ? fill(globalThis.cardWords[key], vars) : key), locale: 'en' });
  export const useNavigate = () => () => true;
`);
const shared = sharedLayer(stub);
const { NodePatches } = await import(shared.compile('components/NodePatches.tsx'));

const originalFetch = globalThis.fetch;
let tree;
let requests = [];
let installs = [];
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
async function settle() { await act(async () => { await new Promise((resolve) => setTimeout(resolve, 10)); }); }
const patches = { state: 'read', patches: [{ installed: '22.14.0', line: '22', newest: '22.23.4', newer: true, sites: [{ domain_id: 7, name: 'app.example' }] }] };
async function mount(installed, { words = en, post } = {}) {
  globalThis.cardWords = words;
  globalThis.document = { addEventListener() {}, removeEventListener() {}, getElementById: () => null, activeElement: null, body: { style: {} } };
  requests = [];
  installs = [];
  globalThis.fetch = async (input, init = {}) => {
    const url = String(input);
    requests.push({ url, method: init.method ?? 'GET', body: init.body });
    if (url === '/api/v1/runtimes/node/patches') return answer(patches);
    if (url === '/api/v1/domains/7/app/node-version' && init.method === 'POST') return post();
    return answer({ error: 'no route' }, 404);
  };
  await act(async () => { tree = Renderer.create(React.createElement(NodePatches, { installed, onInstall: async (version) => { installs.push(version); } })); });
  await settle();
}
async function unmount() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  delete globalThis.document;
}
const text = () => { const walk = (n) => (typeof n === 'string' ? n : Array.isArray(n) ? n.map(walk).join('') : (n?.children || []).map(walk).join('')); return walk(tree.toJSON()); };
const attr = (name) => tree.root.findAll((node) => typeof node.type === 'string' && node.props && node.props[name] !== undefined);

test('a newer patch is said and installed only after the question', async () => {
  await mount(['22.14.0']);
  try {
    assert.ok(text().includes(fill(en['life.node.newer'], { installed: '22.14.0', newest: '22.23.4' })));
    assert.ok(text().includes(fill(en['life.node.installFirst'], { newest: '22.23.4' })));
    assert.equal(attr('data-node-move').length, 0, 'no move before the newer patch is installed');
    await act(async () => { attr('data-node-install-newer')[0].props.onClick(); });
    assert.ok(text().includes(fill(en['life.node.installBody'], { installed: '22.14.0', newest: '22.23.4' })));
    assert.deepEqual(installs, [], 'nothing is installed before the confirmation');
    await act(async () => { attr('data-node-confirm')[0].props.onClick(); });
    await settle();
    assert.deepEqual(installs, ['22.23.4']);
  } finally { await unmount(); }
});

test('a site moves only after its own question', async () => {
  await mount(['22.14.0', '22.23.4'], { post: () => answer({ moved: true, from: '22.14.0', to: '22.23.4' }) });
  try {
    await act(async () => { attr('data-node-move')[0].props.onClick(); });
    assert.ok(text().includes(fill(en['life.node.moveBody'], { site: 'app.example', installed: '22.14.0', newest: '22.23.4' })));
    assert.equal(requests.filter((request) => request.method === 'POST').length, 0);
    await act(async () => { attr('data-node-confirm')[0].props.onClick(); });
    await settle();
    const post = requests.find((request) => request.method === 'POST');
    assert.equal(JSON.parse(post.body).version, '22.23.4');
    assert.ok(text().includes(fill(en['life.node.moved'], { site: 'app.example', version: '22.23.4' })));
  } finally { await unmount(); }
});

test('a move that did not answer says it was put back, in Turkish too', async () => {
  await mount(['22.14.0', '22.23.4'], { words: tr, post: () => answer({ error: 'the application did not answer; it was put back on 22.14.0', rolled_back: true }, 409) });
  try {
    assert.ok(text().includes(fill(tr['life.node.newer'], { installed: '22.14.0', newest: '22.23.4' })));
    await act(async () => { attr('data-node-move')[0].props.onClick(); });
    await act(async () => { attr('data-node-confirm')[0].props.onClick(); });
    await settle();
    assert.ok(text().includes(fill(tr['life.node.moveFailed'], { site: 'app.example', reason: 'the application did not answer; it was put back on 22.14.0' })));
  } finally { await unmount(); }
});
