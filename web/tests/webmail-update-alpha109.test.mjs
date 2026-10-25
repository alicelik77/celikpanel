// alpha.109 (Frankfurt): the webmail update is a durable server operation.
// After a reload the card re-attaches to a running update by reading it;
// the outcome of the last update stays said (failed and put back, with the
// cause) until the server runs that release; a lost POST answer is looked up
// by its request id, never sent again; the Open webmail link is offered only
// while webmail answers.
// Web postası güncellemesi kalıcı bir işlemdir; yeniden bağlanma, sonuç ve
// web postası bağlantısı.
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
  export const AlertTriangle = () => null, CheckCircle2 = () => null, Info = () => null, XCircle = () => null, Mail = () => null, X = () => null, Loader2 = () => null;
  const fill = (text, vars) => Object.entries(vars || {}).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
  export const useI18n = () => ({ t: (key, vars) => (globalThis.cardWords?.[key] !== undefined ? fill(globalThis.cardWords[key], vars) : key), locale: 'en' });
  export const useNavigate = () => () => true;
`);
const shared = sharedLayer(stub);
const { RoundcubeUpdateCard } = await import(shared.compile('components/RoundcubeUpdateCard.tsx', { '/LifeLine': shared.compile('components/LifeLine.tsx') }));

const originalFetch = globalThis.fetch;
let tree;
let requests = [];
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
async function settle(ms = 10) { await act(async () => { await new Promise((resolve) => setTimeout(resolve, ms)); }); }
const opID = 'c'.repeat(32);
const operation = (status, result = {}, extra = {}) => ({ id: opID, request_id: 'd'.repeat(32), kind: 'webmail_update', service_id: 'roundcube', package_name: '1.6.20', status, phase: 'updating', started_at: '2026-10-25T10:00:00Z', result, ...extra });
const check = (extra = {}, top = {}) => ({ state: 'read', check: { installed: true, version: '1.6.15', line: '1.6', latest_on_line: '1.6.20', update_available: true, ...extra }, last_update: null, webmail_ready: false, ...top });
async function mount(reads, { words = en, post, ops = [] } = {}) {
  globalThis.cardWords = words;
  globalThis.document = { addEventListener() {}, removeEventListener() {}, getElementById: () => null, activeElement: null, body: { style: {} } };
  requests = [];
  let n = 0;
  globalThis.fetch = async (input, init = {}) => {
    const url = String(input);
    requests.push({ url, method: init.method ?? 'GET', body: init.body });
    if (url === '/api/v1/service/roundcube/update' && init.method === 'POST') return post(init);
    if (url === '/api/v1/service/roundcube/update') return answer(reads[Math.min(n++, reads.length - 1)]);
    if (url.startsWith('/api/v1/service/operation?')) {
      const next = ops.length > 1 ? ops.shift() : ops[0];
      return next === undefined ? answer({ error: 'not found' }, 404) : answer({ operation: next });
    }
    return answer({ error: 'no route' }, 404);
  };
  await act(async () => { tree = Renderer.create(React.createElement(RoundcubeUpdateCard)); });
  await settle();
}
async function unmount() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  delete globalThis.document;
}
const text = () => { const walk = (n) => (typeof n === 'string' ? n : Array.isArray(n) ? n.map(walk).join('') : (n?.children || []).map(walk).join('')); return walk(tree.toJSON()); };
const attr = (name) => tree.root.findAll((node) => node.props && node.props[name] !== undefined);

test('after a reload the card re-attaches to a running update and says its outcome', async () => {
  await mount([check({}, { last_update: operation('running') }), check({ version: '1.6.20', update_available: false })], {
    post: () => { throw new Error('no POST may be sent'); },
    ops: [operation('running'), operation('succeeded', { success: true, outcome: 'updated', from: '1.6.15', to: '1.6.20', backup: '/var/lib/x/roundcube-1.6.15.tar.gz' })],
  });
  try {
    assert.ok(text().includes(en['life.webmail.updating']));
    await settle(1700);
    await settle(1700);
    assert.equal(requests.filter((request) => request.method === 'POST').length, 0);
    assert.ok(requests.some((request) => request.url === `/api/v1/service/operation?id=${opID}`));
    assert.ok(text().includes('roundcube-1.6.15.tar.gz'), text());
  } finally { await unmount(); }
});

test('the last failed update stays said with its cause, in Turkish too', async () => {
  const reason = "webmail did not answer on 1.6.20 (PHP-FPM 8.4 could not open CelikPanel's PHP check for Webmail (Roundcube) (HTTP 404: 404 Not Found)); the previous 1.6.15 tree was put back byte for byte and answers again";
  const failed = operation('failed', { outcome: 'rolled_back', previous_answers: true, from: '1.6.15', to: '1.6.20', reason }, { error: { code: 'webmail_update_failed', message: 'Webmail was not updated.' } });
  await mount([check({}, { last_update: failed })], { words: tr });
  try {
    assert.ok(text().includes(fill(tr['life.webmail.rolledBack'], { from: '1.6.15', to: '1.6.20', reason })), text());
    assert.ok(attr('data-webmail-update-offer').length >= 1, 'Update is offered again beside the outcome');
  } finally { await unmount(); }
  // Once the server runs that release, the old failure is not said.
  await mount([check({ version: '1.6.20', update_available: false }, { last_update: failed })]);
  try {
    assert.ok(!text().includes('put back'));
  } finally { await unmount(); }
});

test('an owner-edited configuration and an unknown result each say who acts and what next', async () => {
  const edited = operation('failed', { outcome: 'not_changed', code: 'CONFIG_OWNER_EDITED', to: '1.6.20', reason: '/etc/nginx/conf.d/celikpanel-webmail.conf was changed outside CelikPanel' });
  await mount([check({}, { last_update: edited })]);
  try {
    assert.ok(text().includes('Who acts: you or the server administrator'));
    assert.ok(text().includes('celikpanel-webmail.conf'));
  } finally { await unmount(); }
  await mount([check({}, { last_update: operation('failed', { outcome: 'unknown', to: '1.6.20' }) })]);
  try {
    assert.ok(text().includes(fill(en['life.webmail.outcomeUnknown'], { to: '1.6.20', reason: '' }).trim()));
  } finally { await unmount(); }
});

test('a lost POST answer is looked up by its request id and never sent again', async () => {
  await mount([check()], {
    post: () => { throw new TypeError('network'); },
    ops: [],
  });
  try {
    await act(async () => { attr('data-webmail-update-offer')[0].props.onClick(); });
    await act(async () => { attr('data-webmail-update-confirm')[0].props.onClick(); });
    await settle();
    const posts = requests.filter((request) => request.method === 'POST');
    assert.equal(posts.length, 1);
    const id = JSON.parse(posts[0].body).request_id;
    assert.ok(requests.some((request) => request.url === `/api/v1/service/operation?request_id=${id}`));
    assert.ok(text().includes(en['life.webmail.answerLost']));
  } finally { await unmount(); }
});

test('the Open webmail link is held back while webmail is only on the panel origin', async () => {
  // alpha.109 review: the panel's script policy stays on /webmail/, so the
  // interface does not start there; the link waits for webmail's own address.
  await mount([check({}, { webmail_ready: true })]);
  try {
    assert.equal(attr('data-open-webmail-link').length, 0);
  } finally { await unmount(); }
  await mount([check({}, { webmail_ready: false })]);
  try {
    assert.equal(attr('data-open-webmail-link').length, 0);
  } finally { await unmount(); }
});

test('a release check that could not be read still says the last update', async () => {
  const failed = operation('failed', { outcome: 'rolled_back', previous_answers: true, from: '1.6.15', to: '1.6.20', reason: 'webmail did not answer on 1.6.20' });
  await mount([{ state: 'unknown', last_update: failed, webmail_ready: false }]);
  try {
    assert.ok(text().includes(en['life.webmail.unknown']));
    assert.ok(text().includes(fill(en['life.webmail.rolledBack'], { from: '1.6.15', to: '1.6.20', reason: 'webmail did not answer on 1.6.20' })), text());
  } finally { await unmount(); }
});
