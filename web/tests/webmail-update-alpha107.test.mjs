// alpha.107 (D-038 §9): webmail (Roundcube) updates, mounted, in English and
// Turkish: the newest release on the installed line is offered, updated
// only after confirmation with visible waiting, and the result is said: done
// with the backup, or put back exactly as it was.
// Web postası güncellemeleri, bağlanmış olarak.
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
async function settle() { await act(async () => { await new Promise((resolve) => setTimeout(resolve, 10)); }); }
const check = (extra = {}) => ({ state: 'read', check: { installed: true, version: '1.6.15', line: '1.6', latest_on_line: '1.6.20', latest_overall: '1.7.5', update_available: true, checked_at: '2026-10-24T10:00:00Z', ...extra } });
const opID = 'a'.repeat(32);
const operation = (status, result = {}, extra = {}) => ({ id: opID, request_id: 'b'.repeat(32), kind: 'webmail_update', service_id: 'roundcube', package_name: '1.6.20', status, phase: status === 'running' ? 'updating' : 'completed', started_at: '2026-10-25T10:00:00Z', result, ...extra });
async function mount(reads, { words = en, post, ops = [], component = RoundcubeUpdateCard } = {}) {
  globalThis.cardWords = words;
  globalThis.document = { addEventListener() {}, removeEventListener() {}, getElementById: () => null, activeElement: null, body: { style: {} } };
  requests = [];
  let n = 0;
  globalThis.fetch = async (input, init = {}) => {
    const url = String(input);
    requests.push({ url, method: init.method ?? 'GET', body: init.body });
    if (url === '/api/v1/service/roundcube/update' && init.method === 'POST') return post(init);
    if (url === '/api/v1/service/roundcube/update') return answer(reads[Math.min(n++, reads.length - 1)]);
    if (url.startsWith('/api/v1/service/operation?')) return answer({ operation: ops.length > 1 ? ops.shift() : ops[0] ?? null });
    return answer({ error: 'no route' }, 404);
  };
  await act(async () => { tree = Renderer.create(React.createElement(component)); });
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

test('a newer release on the line is offered and updated after confirmation', async () => {
  await mount([check(), check({ version: '1.6.20', update_available: false })], {
    post: () => answer({ operation: operation('queued') }, 202),
    ops: [operation('succeeded', { success: true, outcome: 'updated', from: '1.6.15', to: '1.6.20', backup: '/var/lib/celikpanel-agent-private/roundcube-backups/roundcube-1.6.15.tar.gz' })],
  });
  try {
    assert.equal(attr('data-webmail-state')[0].props['data-webmail-state'], 'available');
    assert.ok(text().includes(fill(en['life.webmail.otherLine'], { overall: '1.7.5' })));
    await act(async () => { attr('data-webmail-update-offer')[0].props.onClick(); });
    await act(async () => { attr('data-webmail-update-confirm')[0].props.onClick(); });
    await settle();
    const post = requests.find((request) => request.method === 'POST');
    const sent = JSON.parse(post.body);
    assert.equal(sent.version, '1.6.20');
    assert.match(sent.request_id, /^[a-f0-9]{32}$/);
    assert.equal(requests.filter((request) => request.method === 'POST').length, 1, 'polling never sends a second update');
    assert.ok(text().includes('roundcube-1.6.15.tar.gz'));
    assert.equal(attr('data-webmail-state')[0].props['data-webmail-state'], 'current');
  } finally { await unmount(); }
});

test('a failed update says the previous files were put back, in Turkish too', async () => {
  await mount([check()], {
    words: tr,
    post: () => answer({ operation: operation('running') }, 202),
    ops: [operation('failed', { success: false, outcome: 'rolled_back', previous_answers: true, code: 'PROOF_FAILED', rolled_back: true, from: '1.6.15', to: '1.6.20', reason: 'webmail did not answer on 1.6.20' }, { error: { code: 'webmail_update_failed', message: 'Webmail was not updated.' } })],
  });
  try {
    await act(async () => { attr('data-webmail-update-offer')[0].props.onClick(); });
    await act(async () => { attr('data-webmail-update-confirm')[0].props.onClick(); });
    await settle();
    assert.ok(text().includes(fill(tr['life.webmail.rolledBack'], { from: '1.6.15', to: '1.6.20', reason: 'webmail did not answer on 1.6.20' })));
  } finally { await unmount(); }
});

test('a release on the installed line says it is current; a failed read is unknown', async () => {
  await mount([check({ version: '1.6.20', update_available: false })]);
  try {
    assert.ok(text().includes(fill(en['life.webmail.current'], { version: '1.6.20', line: '1.6' })));
    assert.equal(attr('data-webmail-update-offer').length, 0);
  } finally { await unmount(); }
  await mount([{ state: 'unknown' }]);
  try {
    assert.ok(text().includes(en['life.webmail.unknown']));
  } finally { await unmount(); }
});
