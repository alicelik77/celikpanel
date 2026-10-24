// alpha.107 (D-038 §17): the operating-system release card, mounted, in
// English and Turkish: a tested release says so with its patch date; after an
// OS release upgrade, every change is said with its next action and the
// record is replaced only when the administrator marks it reviewed.
// İşletim sistemi kartı, bağlanmış olarak.
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
  export const AlertTriangle = () => null, CheckCircle2 = () => null, Info = () => null, XCircle = () => null, X = () => null, Loader2 = () => null, Server = () => null, RefreshCw = () => null;
  const fill = (text, vars) => Object.entries(vars || {}).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
  export const useI18n = () => ({ t: (key, vars) => (globalThis.cardWords?.[key] !== undefined ? fill(globalThis.cardWords[key], vars) : key), locale: globalThis.cardLocale ?? 'en' });
  export const useNavigate = () => () => true;
`);
const shared = sharedLayer(stub);
const { OSReleaseCard } = await import(shared.compile('components/OSReleaseCard.tsx'));

const releases = [{ name: 'Debian 13 (trixie)', level: 'tested', patched_until: '2030-06-30' }, { name: 'Arch Linux (rolling)', level: 'development' }];
const same = { state: 'read', checked: '2026-10-24', releases, changed: false, changes: [],
  current: { id: 'debian', version_id: '13', codename: 'trixie', pretty_name: 'Debian GNU/Linux 13 (trixie)', level: 'tested', name: 'Debian 13 (trixie)', patched_until: '2030-06-30' },
  recorded: { pretty_name: 'Debian GNU/Linux 13 (trixie)', codename: 'trixie', version_id: '13', recorded_at: '2026-10-24T10:00:00Z' } };
const upgraded = { ...same, changed: true,
  current: { id: 'debian', version_id: '14', codename: 'forky', pretty_name: 'Debian GNU/Linux 14 (forky)', level: 'not_listed' },
  changes: [{ kind: 'repo_suite', subject: 'sury', from: 'trixie', to: 'forky' }, { kind: 'pg_cluster_new', subject: '18/main' }, { kind: 'mariadb_version', from: '11.8', to: '12.1' }] };

const originalFetch = globalThis.fetch;
let tree;
let requests = [];
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
async function settle() { await act(async () => { await new Promise((resolve) => setTimeout(resolve, 10)); }); }
async function mount(reads, { words = en, locale = 'en', post } = {}) {
  globalThis.cardWords = words;
  globalThis.cardLocale = locale;
  requests = [];
  let n = 0;
  globalThis.fetch = async (input, init = {}) => {
    const url = String(input);
    requests.push({ url, method: init.method ?? 'GET' });
    if (url === '/api/v1/server/os-release/reviewed' && init.method === 'POST') return post();
    if (url === '/api/v1/server/os-release') return answer(reads[Math.min(n++, reads.length - 1)]);
    return answer({ error: 'no route' }, 404);
  };
  await act(async () => { tree = Renderer.create(React.createElement(OSReleaseCard)); });
  await settle();
}
async function unmount() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
}
const text = () => { const walk = (n) => (typeof n === 'string' ? n : Array.isArray(n) ? n.map(walk).join('') : (n?.children || []).map(walk).join('')); return walk(tree.toJSON()); };
const attr = (name) => tree.root.findAll((node) => typeof node.type === 'string' && node.props && node.props[name] !== undefined);

test('a tested release says so, with its patch date and the owner’s step', async () => {
  await mount([same]);
  try {
    assert.equal(attr('data-os-level')[0].props['data-os-level'], 'tested');
    assert.ok(text().includes(fill(en['life.os.level.tested'], { name: 'Debian 13 (trixie)' })));
    assert.ok(text().includes(en['life.os.ownerStep']));
    assert.equal(attr('data-os-review').length, 0, 'nothing to review');
    assert.equal(attr('data-os-release-entry').length, 2);
  } finally { await unmount(); }
});

test('after an OS upgrade each change has its next action, and reviewing replaces the record', async () => {
  await mount([upgraded, same], { post: () => answer({ state: 'read' }) });
  try {
    assert.equal(attr('data-os-changed')[0].props['data-os-changed'], 'yes');
    assert.ok(text().includes(fill(en['life.os.level.not_listed'], { pretty: 'Debian GNU/Linux 14 (forky)' })));
    assert.ok(text().includes(fill(en['life.os.change.repo_suite'], { subject: 'sury', from: 'trixie', to: 'forky' })));
    assert.ok(text().includes(fill(en['life.os.change.pg_cluster_new'], { subject: '18/main' })));
    assert.equal(requests.filter((request) => request.method === 'POST').length, 0, 'nothing is recorded before the click');
    await act(async () => { attr('data-os-review')[0].props.onClick(); });
    await settle();
    assert.equal(requests.filter((request) => request.method === 'POST').length, 1);
    assert.equal(attr('data-os-changed')[0].props['data-os-changed'], 'no');
  } finally { await unmount(); }
});

test('in Turkish too, and a review that was not recorded says so', async () => {
  await mount([upgraded], { words: tr, locale: 'tr', post: () => answer({ error: 'the operating-system release could not be read, so nothing was recorded; try again' }, 503) });
  try {
    assert.ok(text().includes(fill(tr['life.os.change.mariadb_version'], { from: '11.8', to: '12.1' })));
    await act(async () => { attr('data-os-review')[0].props.onClick(); });
    await settle();
    assert.ok(text().includes(fill(tr['life.os.reviewFailed'], { reason: 'the operating-system release could not be read, so nothing was recorded; try again' })));
  } finally { await unmount(); }
});
