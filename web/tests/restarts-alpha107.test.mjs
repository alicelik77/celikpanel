// alpha.107 (D-038 §10, §3): Maintenance → Restarts and reboot, mounted, in
// English and Turkish: a reboot that waits, the services that still run
// replaced code with what a restart interrupts, said before it starts; a
// component restarts through its own restart, anything else gets the
// administrator's command; a failed read is unknown, never "nothing".
// Yeniden başlatmalar kartı, bağlanmış olarak.
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
  export const AlertTriangle = () => null, CheckCircle2 = () => null, Info = () => null, XCircle = () => null, RotateCw = () => null, X = () => null, Loader2 = () => null;
  const fill = (text, vars) => Object.entries(vars || {}).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
  export const useI18n = () => ({ t: (key, vars) => (globalThis.cardWords?.[key] !== undefined ? fill(globalThis.cardWords[key], vars) : key), locale: 'en' });
  export const useNavigate = () => () => true;
`);
const shared = sharedLayer(stub);
const { RestartsCard } = await import(shared.compile('components/RestartsCard.tsx', { '/LifeLine': shared.compile('components/LifeLine.tsx') }));

const originalFetch = globalThis.fetch;
let tree;
let requests = [];
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
async function settle() { await act(async () => { await new Promise((resolve) => setTimeout(resolve, 10)); }); }
const needs = (extra = {}) => ({ state: 'read', needs: {
  source: 'needrestart', services_known: true, reboot_known: true, reboot_required: true, reboot_reason: 'reboot_required_file', reboot_packages: ['linux-image-6.12.57+deb13-amd64'],
  services: [
    { unit: 'nginx.service', component: 'nginx', interruption: 'web' },
    { unit: 'named.service', component: 'bind', interruption: 'dns' },
    { unit: 'cron.service', interruption: 'other' },
  ],
  ...extra,
} });
async function mount(reads, { words = en, post } = {}) {
  globalThis.cardWords = words;
  globalThis.document = { addEventListener() {}, removeEventListener() {}, getElementById: () => null, activeElement: null, body: { style: {} } };
  requests = [];
  let n = 0;
  globalThis.fetch = async (input, init = {}) => {
    const url = String(input);
    requests.push({ url, method: init.method ?? 'GET', body: init.body });
    if (url === '/api/v1/service/action') return post(init);
    if (url === '/api/v1/maintenance/restarts') {
      const next = reads[Math.min(n++, reads.length - 1)];
      return next === 'fail' ? answer({ error: 'no' }, 502) : answer(next);
    }
    return answer({ error: 'no route' }, 404);
  };
  await act(async () => { tree = Renderer.create(React.createElement(RestartsCard)); });
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

test('a waiting reboot and the services that still run replaced code are said with what a restart interrupts', async () => {
  await mount([needs(), needs({ services: [], reboot_required: false })], { post: () => answer({ ok: true }) });
  try {
    const all = text();
    assert.ok(all.includes('linux-image-6.12.57+deb13-amd64'));
    assert.ok(all.includes(en['life.restarts.interruption.web']));
    // A DNS engine and a unit outside the catalogue get the command, never a button.
    assert.deepEqual(attr('data-restart-offer').map((node) => node.props['data-restart-offer']), ['nginx.service']);
    assert.ok(all.includes('sudo systemctl restart named.service'));
    assert.ok(all.includes('sudo systemctl restart cron.service'));
    await act(async () => { attr('data-restart-offer')[0].props.onClick(); });
    assert.ok(text().includes(fill(en['life.restarts.confirmTitle'], { unit: 'nginx.service' })));
    await act(async () => { attr('data-restart-confirm')[0].props.onClick(); });
    await settle();
    const post = requests.find((request) => request.method === 'POST');
    assert.deepEqual(JSON.parse(post.body), { name: 'nginx.service', action: 'restart' });
    assert.ok(text().includes(fill(en['life.restarts.done'], { unit: 'nginx.service' })));
    assert.equal(attr('data-restarts')[0].props['data-restarts'], 0);
  } finally { await unmount(); }
});

test('nothing waiting is said with how it was checked, in Turkish too', async () => {
  await mount([needs({ services: [], reboot_required: false, source: 'maps', needrestart_auto_restart: true })], { words: tr });
  try {
    const all = text();
    assert.ok(all.includes(tr['life.restarts.none.maps']));
    assert.ok(all.includes(tr['life.restarts.reboot.none']));
    assert.ok(all.includes(tr['life.restarts.needrestartAuto']));
  } finally { await unmount(); }
});

test('a failed read is unknown, never nothing to restart', async () => {
  await mount(['fail']);
  try {
    assert.ok(text().includes(en['life.restarts.unknown']));
    assert.equal(attr('data-restarts').length, 0);
  } finally { await unmount(); }
});
