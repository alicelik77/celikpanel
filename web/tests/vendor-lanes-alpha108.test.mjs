// alpha.108 (D-038 amendment "two lanes"; owner findings C1, C5, C8, C12):
// the vendor lane is shown first and written only as confirmed; a lost or
// refused answer reads the plan again and never resends; every install path
// shows the preview; the key line says apt's refusal only when apt said it;
// the security updates card offers the completing action; the tools stay on
// the distribution's PHP; tested marks are information.
// Satıcı yolu, bağlanmış olarak ve kaynak sözleşmeleriyle.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, sharedLayer, testedVersionsURL } from './fixtures/shared-layer.mjs';
import { enVendorLanesScreens } from '../src/i18n/screens/vendorLanes/en.ts';
import { trVendorLanesScreens } from '../src/i18n/screens/vendorLanes/tr.ts';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const en = { ...enVendorLanesScreens, 'common.cancel': 'Cancel', 'common.retry': 'Try again' };
const tr = { ...trVendorLanesScreens, 'common.cancel': 'Vazgeç', 'common.retry': 'Tekrar dene' };
const fill = (text, vars = {}) => Object.entries(vars).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
globalThis.laneWords = en;
const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null, CheckCircle2 = () => null, Info = () => null, XCircle = () => null, X = () => null, Loader2 = () => null;
  const fill = (text, vars) => Object.entries(vars || {}).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
  export const useI18n = () => ({ t: (key, vars) => (globalThis.laneWords?.[key] !== undefined ? fill(globalThis.laneWords[key], vars) : key), locale: 'en' });
  export const useNavigate = () => () => true;
  export const showToast = (type, text) => { (globalThis.laneToasts ??= []).push([type, text]); };
`);
const shared = sharedLayer(stub);
const { VendorLaneDialog } = await import(shared.compile('components/VendorLaneDialog.tsx'));
const { testedMark } = await import(testedVersionsURL);

const sha = (c) => c.repeat(64);
const freshPlan = {
  repo_id: 'sury', name: 'Sury PHP (packages.sury.org)', origin: 'deb.sury.org', enabled: false, ready: true, sha256: sha('a'),
  steps: [
    { step: 'repository', state: 'todo', path: '/etc/apt/sources.list.d/celikpanel-sury.list', text: 'deb https://packages.sury.org/php/ trixie main\n', sha256: sha('b') },
    { step: 'confinement', state: 'todo', path: '/etc/apt/preferences.d/celikpanel-sury.pref', text: 'Package: *\nPin: origin "packages.sury.org"\nPin-Priority: 100\n', sha256: sha('c') },
    { step: 'updates', state: 'todo', path: '/etc/apt/apt.conf.d/52celikpanel-vendor-origins', text: 'Unattended-Upgrade::Origins-Pattern {\n\t"origin=deb.sury.org";\n};\n', sha256: sha('d') },
  ],
};
const blockedPlan = { ...freshPlan, ready: false, sha256: sha('e'), steps: [freshPlan.steps[0], freshPlan.steps[1], { step: 'updates', state: 'blocked', reason: 'automatic_off', path: '/etc/apt/apt.conf.d/52celikpanel-vendor-origins' }] };

const originalFetch = globalThis.fetch;
let tree;
let requests;
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
const settle = async () => { await act(async () => { await new Promise((resolve) => setTimeout(resolve, 10)); }); };
// The rendered words, joined as a reader sees them (kept words are spans).
const text = () => {
  const out = [];
  const walk = (node) => {
    if (typeof node === 'string') out.push(node);
    else if (Array.isArray(node)) node.forEach(walk);
    else if (node?.children) node.children.forEach(walk);
  };
  walk(tree.toJSON());
  return out.join('');
};
async function mount(plans, { words = en, post, serviceId = 'php-fpm' } = {}) {
  globalThis.laneWords = words;
  globalThis.laneToasts = [];
  globalThis.document = { addEventListener() {}, removeEventListener() {}, getElementById: () => null, activeElement: null, body: { style: {} } };
  requests = [];
  let n = 0;
  globalThis.fetch = async (input, init = {}) => {
    const url = String(input);
    requests.push({ url, method: init.method ?? 'GET', body: init.body });
    if (url === '/api/v1/repo/lane' && init.method === 'POST') return post();
    if (url.startsWith('/api/v1/repo/lane?service_id=')) return answer({ state: 'read', plan: plans[Math.min(n++, plans.length - 1)] });
    return answer({ error: 'no route' }, 404);
  };
  const done = [];
  await act(async () => { tree = Renderer.create(React.createElement(VendorLaneDialog, { serviceId, component: 'PHP-FPM', onClose: () => {}, onDone: () => done.push(true) })); });
  await settle();
  return done;
}
async function unmount() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  delete globalThis.document;
}
const confirmButton = () => tree.root.find((node) => node.props['data-vendor-lane-confirm'] !== undefined && typeof node.props.onClick === 'function');

for (const [locale, words] of [['en', en], ['tr', tr]]) {
  test(`${locale}: a fresh vendor repository shows its three steps, their files, texts and digests, and sends only the confirmed plan`, async () => {
    const done = await mount([freshPlan], { words, post: () => answer({ written: true, completed: ['repository', 'confinement', 'updates'], effective: true }) });
    const shown = text();
    for (const step of freshPlan.steps) {
      assert.ok(shown.includes(step.path), step.path);
      assert.ok(shown.includes(step.sha256), step.sha256);
    }
    assert.ok(shown.includes(fill(words['lane.step.updates.todo'], { origin: 'deb.sury.org' })));
    assert.ok(shown.includes(fill(words['lane.afterTools'], {})));
    assert.equal(confirmButton().props.disabled, false);
    await act(async () => { confirmButton().props.onClick(); });
    await settle();
    const post = requests.find((request) => request.method === 'POST');
    assert.deepEqual(JSON.parse(post.body), { service_id: 'php-fpm', expected_sha256: sha('a') });
    assert.deepEqual(done, [true]);
    assert.equal(globalThis.laneToasts[0][0], 'success');
    await unmount();
  });

  test(`${locale}: a blocked step says why and what the administrator does, and nothing can be confirmed`, async () => {
    await mount([blockedPlan], { words, post: () => answer({}, 500), serviceId: 'postgresql' });
    assert.ok(text().includes('sudo dpkg-reconfigure -plow unattended-upgrades'));
    assert.ok(text().includes(words['lane.notReady.blocked']));
    assert.equal(confirmButton().props.disabled, true);
    await unmount();
  });

  test(`${locale}: a changed plan or a lost answer reads the plan again and sends nothing a second time`, async () => {
    await mount([freshPlan, freshPlan], { words, post: () => answer({ error: ['what would be written is not what was confirmed'], code: 'LANE_PLAN_CHANGED' }, 409) });
    await act(async () => { confirmButton().props.onClick(); });
    await settle();
    assert.ok(text().includes(words['lane.code.LANE_PLAN_CHANGED']));
    assert.equal(requests.filter((request) => request.method === 'POST').length, 1);
    assert.equal(requests.filter((request) => request.method === 'GET').length, 2);
    await unmount();
    await mount([freshPlan, freshPlan], { words, post: () => { throw new TypeError('network'); } });
    await act(async () => { confirmButton().props.onClick(); });
    await settle();
    assert.ok(text().includes(fill(words['lane.code.LANE_RESULT_UNKNOWN'], { reason: '', completed: '' })));
    assert.equal(requests.filter((request) => request.method === 'POST').length, 1);
    await unmount();
  });
}

test('every lane reason and refusal code has its words in both languages', () => {
  const dialog = read('../src/components/VendorLaneDialog.tsx');
  const lib = read('../src/lib/vendorLane.ts');
  for (const reason of ['automatic_off', 'automatic_not_installed', 'drop_in_edited', 'origin_mismatch', 'apt_config_unreadable', 'lists_unreadable', 'unknown']) {
    assert.ok(en[`lane.blocked.${reason}`] && tr[`lane.blocked.${reason}`], reason);
  }
  for (const code of lib.match(/'LANE_[A-Z_]+'/g).map((item) => item.slice(1, -1))) {
    assert.ok(en[`lane.code.${code}`] && tr[`lane.code.${code}`], code);
  }
  assert.match(dialog, /void handle\.retry\(\)/);
});

test('every install path shows the preview: the version list and repair carry what was confirmed (C1)', () => {
  const list = read('../src/components/ServiceList.tsx');
  const dialog = list.slice(list.indexOf('function ComponentActionConfirmationDialog'), list.indexOf('function MailProfileInstallDialog'));
  assert.match(dialog, /installPreviewURL\(\[\{ id: action\.service\.id, package: action\.pkg \}\]\)/);
  assert.match(dialog, /<InstallPreviewNotes/);
  assert.match(dialog, /disabled=\{confirmDisabled \|\| \(previewURL !== null && gate\.hold\)\}/);
  const confirm = list.slice(list.indexOf("        case 'install-package':"), list.indexOf("        case 'install-node':"));
  assert.match(confirm, /checkInstalls: true,\s*confirmInstalls: confirmed/);
  assert.doesNotMatch(read('../src/i18n/en.ts'), /Open the install again in Components/);
});

test('a vendor lane is never enabled alone from the version list or the install dialogue', () => {
  const list = read('../src/components/ServiceList.tsx');
  assert.equal((list.match(/<VendorLaneDialog/g) ?? []).length, 2);
  assert.match(list, /repoAction && repo && !repo\.lane && \(/);
  assert.match(list, /repoAction && repo && !\(repoAction === 'enable' && repo\.lane\) && \(/);
  assert.match(list, /<LockedVendorVersions service=\{service\}/);
  assert.match(list, /result\.provider === 'upstream'/);
  assert.match(list, /data-no-longer-updated=/);
});

test('the key line says apt refuses only when apt said so, and keeps its words apart (C5, C12)', () => {
  const card = read('../src/components/RepoCard.tsx');
  assert.match(card, /aptReportsExpiredKey\(info\.apt_errors\) \? t\('life\.repo\.key\.expired'/);
  assert.match(card, /data-key-separator="">·<\/span>/);
  assert.match(card, /data-lane-complete=""/);
  assert.match(card, /data-repo-refresh-key=""/);
});

test('the security updates card offers the completing action for an uncovered vendor (C8)', () => {
  const card = read('../src/components/SecurityUpdatesCard.tsx');
  assert.match(card, /\(!vendor\.covered \|\| !vendor\.confined\) && LANE_SERVICE\[vendor\.repo_id\]/);
  assert.match(card, /<VendorLaneDialog/);
});

test('the panel tools stay on the distribution’s PHP by default (A3)', () => {
  const tool = read('../src/components/PHPToolVersion.tsx');
  assert.match(tool, /const preferred = options\.find\(\(version\) => !vendorVersion\(version\)\)/);
  assert.match(tool, /data-php-tool-vendor=/);
});

test('tested marks are information read from the server, never a block (D-038 §15)', () => {
  const table = { checked: '2026-10-24', sources: {}, statuses: {}, tested_release: 'v0.1.0-alpha.108', tested: { php: ['8.4', '8.5'] } };
  assert.equal(testedMark(table, 'php', '8.4'), 'tested');
  assert.equal(testedMark(table, 'php', '8.4.13'), 'tested');
  assert.equal(testedMark(table, 'php', '8.3'), 'not_tested');
  assert.equal(testedMark({ ...table, tested_release: undefined }, 'php', '8.4'), null);
  assert.equal(testedMark(null, 'php', '8.4'), null);
});

test('a component on the default lane says so on its own page and offers the informed choice', () => {
  const card = read('../src/components/RepoCard.tsx');
  assert.match(card, /remote\.value\.available && !remote\.value\.enabled && remote\.value\.lane/);
  assert.match(card, /data-lane-default=""/);
  assert.match(card, /data-lane-enable=""/);
  for (const key of ['lane.default.title', 'lane.default.body', 'lane.default.left', 'lane.default.enable']) assert.ok(en[key] && tr[key], key);
});

test('an owner’s own source gets no second source: the dialog says so and only completes (review)', () => {
  const dialog = read('../src/components/VendorLaneDialog.tsx');
  assert.match(dialog, /ownerSourceLane\(value\.plan\) \? 'lane\.intro\.owner'/);
  assert.match(dialog, /const completing = Boolean\(plan && \(plan\.enabled \|\| ownerSourceLane\(plan\)\)\)/);
  for (const key of ['lane.intro.owner', 'lane.owner.owner_source', 'lane.blocked.duplicate_source']) assert.ok(en[key] && tr[key], key);
  assert.match(en['lane.blocked.automatic_off'], /Or add the repository yourself: CelikPanel then shows it as yours and not patched automatically/);
  assert.match(en['lane.blocked.automatic_off'], /asks one question/);
});
