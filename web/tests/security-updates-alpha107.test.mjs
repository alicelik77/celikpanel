// alpha.107 (D-038 §4, §9): Security → Security updates, mounted, in English
// and Turkish. On: last run and what is waiting. Off: a warning with the
// administrator's own command, never a switch. An uncovered vendor
// repository and the drop-in that adds it, shown in full and confirmed with
// its digest. Components outside the OS's updates. A failed read is
// unknown, never "off".
// Güvenlik güncellemeleri kartı, bağlanmış olarak.
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
  export const AlertTriangle = () => null, CheckCircle2 = () => null, Info = () => null, XCircle = () => null, ShieldCheck = () => null, X = () => null, Loader2 = () => null;
  const fill = (text, vars) => Object.entries(vars || {}).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
  export const useI18n = () => ({ t: (key, vars) => (globalThis.cardWords?.[key] !== undefined ? fill(globalThis.cardWords[key], vars) : key), locale: 'en' });
  export const useNavigate = () => () => true;
  export const showToast = () => {};
  export const componentName = (s) => (typeof s === 'string' ? s : s.name);
`);
const shared = sharedLayer(stub);
const lifeLineURL = shared.compile('components/LifeLine.tsx');
const { SecurityUpdatesCard, decodeOSUpdates, enableCommand } = await import(shared.compile('components/SecurityUpdatesCard.tsx', { '/LifeLine': lifeLineURL }));

const originalFetch = globalThis.fetch;
let tree;
let requests = [];
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
async function settle() { await act(async () => { await new Promise((resolve) => setTimeout(resolve, 10)); }); }
const proposed = '// Written by CelikPanel\nUnattended-Upgrade::Origins-Pattern {\n\t"origin=deb.sury.org";\n};\n';
const read = (extra = {}) => ({
  state: 'read', package_family: 'apt',
  automatic: { state: 'on', tool: 'unattended-upgrades' },
  last_run: '2026-10-24T06:12:01Z', last_run_source: 'log',
  pending: { state: 'known', total: 3, security: 2, security_names: ['libssl3t64', 'openssl'], lists_at: '2026-10-24T05:00:00Z' },
  vendors: [{ repo_id: 'sury', name: 'Sury PHP (packages.sury.org)', origin: 'deb.sury.org', covered: false, confined: true, packages: 14, names: ['php8.5-fpm', 'php8.5-cli'] }],
  outside: [{ id: 'roundcube', versions: ['1.6.15'] }, { id: 'node', versions: ['22.14.0'] }],
  drop_in: { path: '/etc/apt/apt.conf.d/52celikpanel-vendor-origins', state: 'absent', proposed, proposed_sha256: 'a'.repeat(64) },
  ...extra,
});
async function mount(reads, { words = en, post } = {}) {
  globalThis.cardWords = words;
  globalThis.document = { addEventListener() {}, removeEventListener() {}, getElementById: () => null, activeElement: null, body: { style: {} } };
  requests = [];
  let n = 0;
  globalThis.fetch = async (input, init = {}) => {
    const url = String(input);
    requests.push({ url, method: init.method ?? 'GET', body: init.body });
    if (url === '/api/v1/security/os-updates/origins') return post(init);
    if (url === '/api/v1/security/os-updates') {
      const next = reads[Math.min(n++, reads.length - 1)];
      return next === 'fail' ? answer({ error: 'no' }, 502) : answer(next);
    }
    return answer({ error: 'no route' }, 404);
  };
  await act(async () => { tree = Renderer.create(React.createElement(SecurityUpdatesCard)); });
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

test('on: the tool, the last run and the waiting security updates are said', async () => {
  await mount([read()]);
  try {
    assert.equal(attr('data-os-updates')[0].props['data-os-updates'], 'on');
    const all = text();
    assert.ok(all.includes(fill(en['life.updates.auto.on'], { tool: 'unattended-upgrades' })));
    assert.ok(all.includes('libssl3t64, openssl'));
    assert.ok(all.includes('Webmail (Roundcube) 1.6.15'));
    assert.ok(all.includes('Node.js 22.14.0'));
    assert.equal(attr('data-os-updates-vendor')[0].props['data-covered'], 'no');
  } finally { await unmount(); }
});

test('off is a warning with the administrator’s own command, and nothing switches it', async () => {
  await mount([read({ automatic: { state: 'off', tool: 'unattended-upgrades', reason: 'config_off' } })]);
  try {
    const all = text();
    assert.ok(all.includes('sudo dpkg-reconfigure -plow unattended-upgrades'));
    assert.ok(all.includes('never turns them off'));
    assert.equal(tree.root.findAll((node) => node.props?.['data-tone'] === 'warn' && node.props?.['data-life-line'] === 'automatic').length, 1);
  } finally { await unmount(); }
  assert.equal(enableCommand('apt', 'timer_disabled'), 'sudo systemctl enable --now apt-daily-upgrade.timer');
  assert.ok(enableCommand('dnf').includes('dnf-automatic'));
});

test('the drop-in is shown in full and written only with the digest that was shown', async () => {
  await mount([read(), read({ drop_in: { path: '/etc/apt/apt.conf.d/52celikpanel-vendor-origins', state: 'present', origins: ['deb.sury.org'] }, vendors: [{ repo_id: 'sury', name: 'Sury', covered: true, confined: true, packages: 14 }] })], {
    post: () => answer({ written: true, effective: true, origins: ['deb.sury.org'] }),
  });
  try {
    await act(async () => { attr('data-os-updates-offer')[0].props.onClick(); });
    assert.equal(attr('data-os-updates-text')[0].props.children, proposed);
    await act(async () => { attr('data-os-updates-confirm')[0].props.onClick(); });
    await settle();
    const post = requests.find((request) => request.method === 'POST');
    assert.deepEqual(JSON.parse(post.body), { expected_sha256: 'a'.repeat(64) });
    assert.ok(text().includes(fill(en['life.updates.dropIn.done'], { origins: 'deb.sury.org' })));
    assert.equal(attr('data-os-updates-vendor')[0].props['data-covered'], 'yes');
  } finally { await unmount(); }
});

test('Arch says what is true, in Turkish too', async () => {
  await mount([read({ package_family: 'pacman', automatic: { state: 'not_applicable' }, pending: { state: 'unknown', total: 0, security: 0 }, vendors: [], drop_in: undefined, last_run: '2026-10-20T08:00:01Z' })], { words: tr });
  try {
    const all = text();
    assert.ok(all.includes(tr['life.updates.auto.not_applicable']));
    assert.ok(all.includes('Son tam sistem yükseltmesi'));
  } finally { await unmount(); }
});

test('a failed read is unknown, never off', async () => {
  await mount(['fail']);
  try {
    assert.ok(text().includes(en['life.updates.unknown']));
    assert.equal(attr('data-os-updates').length, 0);
  } finally { await unmount(); }
  assert.throws(() => decodeOSUpdates({ state: 'unknown', vendors: [], outside: [] }));
});
