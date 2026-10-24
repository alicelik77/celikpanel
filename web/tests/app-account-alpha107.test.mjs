// alpha.107 (D-038 §6): which account a Node.js application runs as,
// mounted, in English and Turkish: an application made before runs as the
// shared web user and says so, with the move asked first; a move that did
// not work says it was put back.
// Uygulamanın hesabı, bağlanmış olarak.
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
const { AppAccountLine, IsolationFact } = await import(shared.compile('components/AppAccountLine.tsx', { '/LifeLine': shared.compile('components/LifeLine.tsx') }));

const originalFetch = globalThis.fetch;
let tree;
let requests = [];
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
async function settle() { await act(async () => { await new Promise((resolve) => setTimeout(resolve, 10)); }); }
async function mount(reads, { words = en, post } = {}) {
  globalThis.cardWords = words;
  globalThis.document = { addEventListener() {}, removeEventListener() {}, getElementById: () => null, activeElement: null, body: { style: {} } };
  requests = [];
  let n = 0;
  globalThis.fetch = async (input, init = {}) => {
    const url = String(input);
    requests.push({ url, method: init.method ?? 'GET' });
    if (url === '/api/v1/domains/4/app/account' && init.method === 'POST') return post();
    if (url === '/api/v1/domains/4/app/account') return answer(reads[Math.min(n++, reads.length - 1)]);
    return answer({ error: 'no route' }, 404);
  };
  await act(async () => { tree = Renderer.create(React.createElement(AppAccountLine, { domainId: 4 })); });
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
const web = { runs_as: 'web', site_user: 'app_example', web_user: 'www-data' };

test('an application on the shared web user says so and moves only after the question', async () => {
  await mount([web, { ...web, runs_as: 'site' }], { post: () => answer({ ...web, runs_as: 'site' }) });
  try {
    assert.equal(attr('data-app-account')[0].props['data-app-account'], 'web');
    assert.ok(text().includes(fill(en['life.app.web'], { user: 'www-data' })));
    await act(async () => { attr('data-app-account-offer')[0].props.onClick(); });
    assert.ok(text().includes(fill(en['life.app.dialogBody'], { user: 'app_example', web: 'www-data' })));
    assert.equal(requests.filter((request) => request.method === 'POST').length, 0, 'nothing is sent before the confirmation');
    await act(async () => { attr('data-app-account-confirm')[0].props.onClick(); });
    await settle();
    assert.ok(text().includes(fill(en['life.app.moved'], { user: 'app_example' })));
    assert.equal(attr('data-app-account')[0].props['data-app-account'], 'site');
  } finally { await unmount(); }
});

test('a move that did not work says it was put back, in Turkish too', async () => {
  await mount([web], { words: tr, post: () => answer({ error: 'connection refused', rolled_back: true }, 409) });
  try {
    await act(async () => { attr('data-app-account-offer')[0].props.onClick(); });
    await act(async () => { attr('data-app-account-confirm')[0].props.onClick(); });
    await settle();
    assert.ok(text().includes(fill(tr['life.app.rolledBack'], { reason: 'connection refused' })));
  } finally { await unmount(); }
});

test('shared isolation is said as a fact', async () => {
  await act(async () => { tree = Renderer.create(React.createElement(IsolationFact, { kind: 'mail' })); });
  try {
    assert.ok(text().includes('vmail'));
  } finally { await act(async () => tree.unmount()); tree = undefined; }
});
