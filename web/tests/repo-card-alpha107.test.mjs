// alpha.107 (D-038 §12, §1/§7): an enabled vendor repository's card, mounted,
// in English and Turkish: confined or not (Confine), kept packages, an
// edited rule or the owner's own rule, who enabled it and when, its signing
// key and expiry, apt's errors, and the moving default-version packages with
// the owner's optional command. And the install preview: a vendor
// replacement of a distribution package holds Install whatever is ticked, an
// unasked upgrade is named with its versions.
// Etkin üretici deposunun kartı, bağlanmış olarak; ve üretici değişiminin
// her durumda kurulumu tuttuğu önizleme.
import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, installPreviewLibURL, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { enLifecycleScreens } from '../src/i18n/screens/lifecycle/en.ts';
import { trLifecycleScreens } from '../src/i18n/screens/lifecycle/tr.ts';
import { enComponentsScreens } from '../src/i18n/screens/components/en.ts';
import { trComponentsScreens } from '../src/i18n/screens/components/tr.ts';

const en = { ...enComponentsScreens, ...enLifecycleScreens };
const tr = { ...trComponentsScreens, ...trLifecycleScreens };
const fill = (text, vars = {}) => Object.entries(vars).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
globalThis.cardWords = en;
const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null, CheckCircle2 = () => null, Info = () => null, Package = () => null, XCircle = () => null, X = () => null, Loader2 = () => null;
  const fill = (text, vars) => Object.entries(vars || {}).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
  export const useI18n = () => ({ t: (key, vars) => (globalThis.cardWords?.[key] !== undefined ? fill(globalThis.cardWords[key], vars) : key), locale: 'en' });
  export const useNavigate = () => () => true;
  export const showToast = () => {};
`);
const shared = sharedLayer(stub);
const lifeLineURL = shared.compile('components/LifeLine.tsx');
const { RepoCard, groupedFingerprint, repoDate } = await import(shared.compile('components/RepoCard.tsx', {
  '/lib/componentName': shared.compile('lib/componentName.ts'),
  '/LifeLine': lifeLineURL,
  '/RepoConfineDialog': shared.compile('components/RepoConfineDialog.tsx', { '/LifeLine': lifeLineURL }),
}));
const preview = await import(installPreviewLibURL);
const { installPreviewGate } = await import(shared.compile('components/InstallPreviewNotes.tsx'));

const originalFetch = globalThis.fetch;
let tree;
let requests = [];
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
async function settle() { await act(async () => { await new Promise((resolve) => setTimeout(resolve, 10)); }); }
const sury = (extra = {}) => ({
  available: true, enabled: true, id: 'sury', name: 'Sury PHP (packages.sury.org)',
  confinement: { state: 'confined', pin_path: '/etc/apt/preferences.d/celikpanel-sury.pref', kept: ['php8.4-fpm'], kept_total: 1, written_at: '2026-10-24T10:00:00Z', written_by: 'panel_upgrade' },
  key: { fingerprint: '15058500A0235D97F5D10063B188E2B695BD4743', state: 'valid', expires_at: '2027-02-01T00:00:00Z' },
  lists: { state: 'fresh', date: 'Fri, 23 Oct 2026 10:00:00 UTC', origin: 'deb.sury.org' },
  enabled_by: 'admin', enabled_at: '2026-09-01 10:00:00',
  ...extra,
});
const plan = { state: 'read', preview: {
  state: 'not_written', pin_path: '/etc/apt/preferences.d/celikpanel-sury.pref', text: 'Package: * Pin: origin packages.sury.org Pin-Priority: 100',
  sha256: 'ab'.repeat(32), kept: ['php8.5-fpm'], upgrades_now: 3, upgrades_confined: 2, still_from_vendor: ['php8.5-fpm'],
  changes: [{ package: 'php-common', from: '2:96', before: '2:102+sury', before_vendor: true }],
} };
async function mount(reads, { words = en, confine, plans = [plan] } = {}) {
  globalThis.cardWords = words;
  globalThis.document = { addEventListener() {}, removeEventListener() {}, getElementById: () => null, activeElement: null, body: { style: {} } };
  let p = 0;
  requests = [];
  let n = 0;
  globalThis.fetch = async (input, init = {}) => {
    const url = String(input);
    requests.push({ url, method: init.method ?? 'GET', body: init.body });
    if (url === '/api/v1/repo' && init.method === 'POST') return confine ? confine(init) : answer({ error: 'no' }, 409);
    if (url.startsWith('/api/v1/repo/confine-preview?service_id=')) return answer(plans[Math.min(p++, plans.length - 1)]);
    if (url.startsWith('/api/v1/repo?service_id=')) return answer(reads[Math.min(n++, reads.length - 1)]);
    return answer({ error: 'no route' }, 404);
  };
  await act(async () => { tree = Renderer.create(React.createElement(RepoCard, { serviceId: 'php-fpm' })); });
  await settle();
}
async function unmount() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  delete globalThis.document;
}
const text = () => { const walk = (n) => (typeof n === 'string' ? n : (n?.children || []).map(walk).join('')); return walk(tree.toJSON()); };
const attr = (name) => tree.root.findAll((node) => node.props && node.props[name] !== undefined);

test('a confined repository says so first, with what keeps following it and who enabled it', async () => {
  await mount([sury()]);
  try {
    assert.equal(attr('data-repo-state')[0].props['data-repo-state'], 'confined');
    const all = text();
    assert.ok(all.includes(fill(en['life.repo.state.confined'], { component: 'PHP-FPM' })), all);
    assert.ok(all.includes(fill(en['life.repo.kept'], { count: 1, names: 'php8.4-fpm' })));
    assert.ok(all.includes('admin'));
    assert.ok(all.includes('1505 8500 A023 5D97'));
    assert.equal(attr('data-repo-confine').length, 0, 'a confined repository offers no Confine');
  } finally { await unmount(); }
});

test('confining is shown first: the plan, the exact text and its SHA-256, then written as confirmed', async () => {
  const notWritten = sury({ confinement: { state: 'not_written' } });
  await mount([notWritten, sury({ confinement: { state: 'confined', written_by: 'owner', written_at: '2026-10-24T12:00:00Z' } })], { confine: () => answer(sury()) });
  try {
    assert.equal(attr('data-repo-state')[0].props['data-repo-state'], 'not_written');
    await act(async () => { attr('data-repo-confine')[0].props.onClick(); });
    await settle();
    assert.equal(requests.filter((request) => request.method === 'POST').length, 0, 'nothing is written before the confirmation');
    const all = text();
    assert.ok(all.includes(fill(en['life.repo.plan.change.noLongerReplaced'], { package: 'php-common', from: '2:96', before: '2:102+sury' })), all);
    assert.ok(all.includes(fill(en['life.repo.plan.still'], { names: 'php8.5-fpm' })));
    assert.ok(all.includes('ab'.repeat(32)));
    await act(async () => { attr('data-repo-confine-confirm')[0].props.onClick(); });
    await settle();
    const post = requests.find((request) => request.method === 'POST');
    assert.deepEqual(JSON.parse(post.body), { service_id: 'php-fpm', action: 'confine', expected_sha256: 'ab'.repeat(32) });
    assert.equal(attr('data-repo-state')[0].props['data-repo-state'], 'confined');
  } finally { await unmount(); }
});

test('a rule the owner deleted is said and offered again only on request, in Turkish too', async () => {
  await mount([sury({ confinement: { state: 'removed_by_owner', pin_path: '/etc/apt/preferences.d/celikpanel-sury.pref' } })], {
    words: tr, confine: () => answer({ error: 'the plan changed', code: 'REPO_CONFINE_PLAN_CHANGED' }, 409),
  });
  try {
    assert.ok(text().includes(fill(tr['life.repo.state.removed_by_owner'], { path: '/etc/apt/preferences.d/celikpanel-sury.pref' })));
    assert.ok(text().includes(tr['life.repo.confineAgain']));
    await act(async () => { attr('data-repo-confine')[0].props.onClick(); });
    await settle();
    await act(async () => { attr('data-repo-confine-confirm')[0].props.onClick(); });
    await settle();
    assert.ok(text().includes(tr['life.repo.plan.changed']));
  } finally { await unmount(); }
});

test('an edited rule and the owner’s own rule are kept and named; nothing is offered', async () => {
  await mount([sury({ confinement: { state: 'owner_rule', owner_files: ['/etc/apt/preferences.d/php-from-sury.pref'] } })]);
  try {
    assert.ok(text().includes('/etc/apt/preferences.d/php-from-sury.pref'));
    assert.equal(attr('data-repo-confine').length, 0);
  } finally { await unmount(); }
  await mount([sury({ confinement: { state: 'edited', pin_path: '/etc/apt/preferences.d/celikpanel-sury.pref' } })]);
  try {
    assert.equal(attr('data-repo-state')[0].props['data-repo-state'], 'edited');
    assert.ok(text().includes('celikpanel-sury.pref'));
  } finally { await unmount(); }
});

test('an expiring key, an expired list and apt’s errors are said with what to do', async () => {
  await mount([sury({
    key: { fingerprint: 'B97B0AFCAA1A47F044F244A07FCC7D46ACCC4CF8', state: 'expiring', expires_at: '2026-11-30T00:00:00Z' },
    lists: { state: 'expired', valid_until: 'Sat, 01 Nov 2026 00:00:00 UTC' },
    apt_errors: ["E: The repository 'https://packages.sury.org/php trixie InRelease' is not signed."],
    apt_errors_at: '2026-10-24T09:00:00Z',
  })]);
  try {
    assert.equal(attr('data-life-line').filter((node) => node.props['data-life-line'] === 'key-expiring').length, 1);
    assert.equal(attr('data-life-line').filter((node) => node.props['data-life-line'] === 'lists-expired').length, 1);
    assert.ok(text().includes('is not signed'));
    assert.equal(attr('data-repo-apt-errors').length, 1);
  } finally { await unmount(); }
});

test('the moving default-version packages are reported with the owner’s optional command, in Turkish too', async () => {
  await mount([sury({ moving_packages: [{ package: 'php-fpm', exact: 'php8.4-fpm' }] })], { words: tr });
  try {
    const all = text();
    assert.ok(all.includes(fill(tr['life.repo.moving.item'], { package: 'php-fpm', exact: 'php8.4-fpm' })));
    assert.ok(all.includes('sudo apt-mark manual php8.4-fpm && sudo apt-get remove php-fpm'));
    assert.ok(all.includes(tr['life.repo.moving.confined']));
    assert.ok(all.includes(fill(tr['life.repo.title'], { name: 'Sury PHP (packages.sury.org)' })));
  } finally { await unmount(); }
});

test('a component without an enabled repository draws nothing', async () => {
  await mount([{ available: true, enabled: false, id: 'sury' }]);
  try {
    assert.equal(tree.toJSON(), null);
  } finally { await unmount(); }
});

test('the date and the fingerprint are said as they are read', () => {
  assert.equal(groupedFingerprint('b97b0afc'), 'B97B 0AFC');
  assert.notEqual(repoDate('2026-09-01 10:00:00', 'en'), '2026-09-01 10:00:00');
  assert.equal(repoDate('not a date', 'en'), 'not a date');
});

test('a vendor replacement holds Install whatever is ticked; an upgrade says its versions', () => {
  const value = preview.decodeInstallPreview({
    state: 'known', installs: ['libpq5', 'libssl3'], removes: [], requested: [], refusals: [],
    notable: [
      { package: 'libpq5', reason: 'vendor_replaces_distribution', component: 'pgdg', from: '17.6-0+deb13u1', to: '18.0-1.pgdg13+1' },
      { package: 'libssl3', reason: 'upgrade', from: '3.5.1-1', to: '3.5.1-1+deb13u1' },
    ],
  });
  assert.equal(value.notable[0].reason, 'vendor_replaces_distribution');
  assert.equal(preview.hasVendorReplacement(value.notable), true);
  const handle = { remote: { state: 'known', value, observedAt: 0 }, reading: false };
  assert.deepEqual(installPreviewGate(handle, true), { hold: true, confirmed: [] });
  const upgradeOnly = { ...value, notable: [value.notable[1]] };
  assert.deepEqual(installPreviewGate({ ...handle, remote: { ...handle.remote, value: upgradeOnly } }, true), { hold: false, confirmed: ['libssl3'] });
  const groups = preview.notableGroups(upgradeOnly.notable);
  assert.deepEqual(groups[0].packages, ['libssl3 3.5.1-1 → 3.5.1-1+deb13u1']);
});

test('the vendor’s shared package is a named, confirmable item with its versions, never a refusal', () => {
  const notable = [{ package: 'php-common', reason: 'vendor_shared', from: '2:96', to: '2:101+sury', component: 'sury' }];
  assert.equal(preview.hasVendorReplacement(notable), false);
  const groups = preview.notableGroups(notable);
  assert.equal(groups[0].reason, 'vendor_shared');
  assert.deepEqual(groups[0].packages, ['php-common 2:96 → 2:101+sury']);
  assert.ok(en['services.installPreview.replaced'] && tr['services.installPreview.replaced']);
});
