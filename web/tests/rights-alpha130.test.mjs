// Rights screens, alpha.130 (owner, 8 Nov 2026).
//
// - Every rights screen shows the permissions under the same topics, in the
//   same order in every language, sorted inside a topic by the label in the
//   reader's language (Turkish rules in Turkish: ç ğ ı i ö ş ü).
// - The Offered tab never lets a tick flip back silently: a change the
//   server did not make is said; items offered elsewhere (Catalog
//   management) are shown with their state and a way there; a version
//   withdrawn for security and the remote API's server switch say so.
// - The hint beside an item that cannot be given names the right screen,
//   and an item the reseller above an account lacks says so.
// - The server's limits of site settings have one Save for the section.
//
// Hak ekranları, alpha.130: aynı konu başlıkları ve dile göre sıralama;
// Sunulanlar sekmesinde sessiz geri dönüş yok; bölüm başına tek Kaydet.
import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { apiErrorURL, compileSource, dataModule, reactURL, remoteURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { enScreens, trScreens } from './locale-catalogue.mjs';

const { en: enShell } = await import(new URL('../src/i18n/en.ts', import.meta.url).href);
const { tr: trShell } = await import(new URL('../src/i18n/tr.ts', import.meta.url).href);
const catalogues = { en: { ...enShell, ...enScreens }, tr: { ...trShell, ...trScreens } };
const fill = (text, vars) => String(text).replace(/\{(\w+)\}/g, (all, name) => (vars && vars[name] !== undefined ? String(vars[name]) : all));
const say = (locale) => (key, vars) => (catalogues[locale][key] !== undefined ? fill(catalogues[locale][key], vars) : key);

const rightsURL = compileSource('lib/rights.ts', (specifier) => {
  if (specifier.endsWith('/apiError')) return apiErrorURL;
  throw new Error(`lib/rights.ts imports ${specifier}`);
});
const rights = await import(rightsURL);

// ---- grouping and sorting ---------------------------------------------------

test('every permission has a topic; topics keep one order in both languages', () => {
  for (const id of rights.PERMISSION_IDS) assert.notEqual(rights.permissionTopic(id), 'other', id);
  const en = rights.permissionGroups(rights.PERMISSION_IDS, say('en'), 'en');
  const tr = rights.permissionGroups(rights.PERMISSION_IDS, say('tr'), 'tr');
  assert.deepEqual(en.map((g) => g.topic), ['dns', 'mail', 'databases', 'sites', 'files', 'apps', 'security', 'backups', 'account']);
  assert.deepEqual(tr.map((g) => g.topic), en.map((g) => g.topic), 'the same headings in every language');
  assert.equal(en.flatMap((g) => g.ids).length, rights.PERMISSION_IDS.length, 'every permission once');
  assert.deepEqual([...en.find((g) => g.topic === 'files').ids].sort(), ['file_access', 'file_manager', 'site_protect_redirects', 'ssh']);
  assert.deepEqual([...en.find((g) => g.topic === 'databases').ids].sort(), ['database_remote', 'databases']);
  // Inside a topic: the reader's language, its own rules.
  for (const [locale, groups] of [['en', en], ['tr', tr]]) {
    const collator = new Intl.Collator(locale, { sensitivity: 'base', numeric: true });
    for (const group of groups) {
      const labels = group.ids.map((id) => say(locale)(`rights.perm.${id}`));
      assert.deepEqual(labels, [...labels].sort((a, b) => collator.compare(a, b)), `${locale} ${group.topic}`);
    }
  }
  // Every topic heading exists in both languages.
  for (const topic of [...rights.PERMISSION_TOPICS, 'other']) {
    for (const locale of ['en', 'tr']) assert.ok(catalogues[locale][`rights.topic.${topic}`], `${locale} rights.topic.${topic}`);
  }
});

test('Turkish order: c ç, g ğ, ı i, o ö, s ş, u ü; switching language sorts again', () => {
  const labels = { a: 'Üç', b: 'Şu', c: 'Çatı', d: 'İnce', e: 'Ilık', f: 'Ödev', g: 'Ğa', h: 'Cam', i: 'Su', j: 'Oda', k: 'Uç', l: 'Gaz', m: 'Cıvata' };
  const t = (key) => labels[key.replace('rights.perm.', '')] ?? key;
  const order = (locale) => rights.permissionGroups(Object.keys(labels), t, locale)[0].ids.map((id) => labels[id]);
  assert.deepEqual(order('tr'), ['Cam', 'Cıvata', 'Çatı', 'Gaz', 'Ğa', 'Ilık', 'İnce', 'Oda', 'Ödev', 'Su', 'Şu', 'Uç', 'Üç']);
  assert.equal(rights.permissionGroups(Object.keys(labels), t, 'tr')[0].topic, 'other', 'an unknown permission stands under Other');
  // The same items in English sort by English rules (Ç beside C).
  assert.notDeepEqual(order('en'), order('tr'));
  assert.ok(order('en').indexOf('Çatı') < order('en').indexOf('Cıvata'));
});

test('the hint names the screen that offers the item', () => {
  const grantor = { permissions: [], software: [{ id: 'component:php-fpm:8.1', granted: false, withdrawn: true }] };
  assert.equal(rights.notOfferedKey('permission:database_remote', grantor), 'rights.notOffered');
  assert.equal(rights.notOfferedKey('component:php-fpm:8.4', grantor), 'rights.notOffered');
  assert.equal(rights.notOfferedKey('runtime:program', grantor), 'rights.notOfferedCatalog');
  assert.equal(rights.notOfferedKey('component:python:3.12', grantor), 'rights.notOfferedCatalog');
  assert.equal(rights.notOfferedKey('app:n8n', grantor), 'rights.notOfferedCatalog');
  assert.equal(rights.notOfferedKey('app:wordpress', grantor), 'rights.notOffered');
  assert.equal(rights.notOfferedKey('component:php-fpm:8.1', grantor), 'rights.notOfferedWithdrawn');
  assert.match(catalogues.en['rights.notOffered'], /Users, Offered/);
  assert.match(catalogues.tr['rights.notOffered'], /Kullanıcılar → Sunulanlar/);
  assert.doesNotMatch(catalogues.tr['rights.notOffered'], /Hesaplar/);
});

test('the answer of a change is read for the state it asked for', () => {
  assert.equal(rights.offeredStateIn({ item: { id: 'permission:database_remote', offered: false } }, 'permission:database_remote'), false);
  assert.equal(rights.offeredStateIn({ item: { id: 'permission:mail', offered: true } }, 'permission:database_remote'), undefined);
  assert.equal(rights.offeredStateIn({ hidden: {} }, 'permission:mail'), undefined);
  const view = { permissions: [{ id: 'mail', granted: true }], software: [{ id: 'component:mariadb', granted: false }] };
  assert.equal(rights.offeredStateOf(view, 'permission:mail'), true);
  assert.equal(rights.offeredStateOf(view, 'component:mariadb'), false);
  assert.equal(rights.offeredStateOf(view, 'component:postgresql'), undefined);
});

// ---- mounted ------------------------------------------------------------------

const icons = ['Save', 'X', 'AlertTriangle', 'XCircle', 'Check', 'Copy', 'Layers', 'Lock', 'Pencil', 'Plus', 'Trash2', 'Loader2'];
const stub = dataModule(`
  import React from '${reactURL}';
  ${icons.map((name) => `export const ${name} = () => null;`).join('\n')}
  const fill = ${fill.toString()};
  export const useI18n = () => ({ locale: globalThis.rightsFixture.locale, t: (key, vars) => globalThis.rightsFixture.catalogue[key] !== undefined ? fill(globalThis.rightsFixture.catalogue[key], vars) : key });
  export const useNavigate = () => (to) => globalThis.rightsFixture.navigated.push(to);
  export const showToast = (...args) => globalThis.rightsFixture.toasts.push(args);
`);
const shared = sharedLayer(stub);
const own = { '/lib/rights': rightsURL };
own['/lib/accounts'] = compileSource('lib/accounts.ts', (specifier) => (specifier.endsWith('/remote') ? remoteURL : shared.resolve(specifier)));
own['/BlockingProblems'] = shared.compile('components/BlockingProblems.tsx', own);
own['/RightsSetEditor'] = shared.compile('components/RightsSetEditor.tsx', own);
own['/AccountRights'] = shared.compile('components/AccountRights.tsx', own);
const { readableRefusal } = await import(own['/AccountRights']);
const { RightsSetEditor } = await import(own['/RightsSetEditor']);
const { OfferedItemsTab } = await import(shared.compile('components/PlanRights.tsx', own));
const { PolicyEditor } = await import(shared.compile('components/ServerSettingLimits.tsx', { '/lib/settingLimits': compileSource('lib/settingLimits.ts', (specifier) => shared.resolve(specifier)) }));

const textOf = (node) => (node === null || node === undefined || typeof node === 'boolean' ? '' : typeof node === 'string' || typeof node === 'number' ? String(node) : Array.isArray(node) ? node.map(textOf).join(' ') : textOf(node.children));
const answer = (status, body) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
const settle = async () => { for (let i = 0; i < 6; i++) await act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)); }); };

async function mount(element, server, locale = 'en') {
  const calls = [];
  globalThis.rightsFixture = { locale, catalogue: catalogues[locale], toasts: [], navigated: [] };
  const original = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    const call = { url: String(url), method: init.method ?? 'GET', body: init.body ? JSON.parse(init.body) : undefined };
    calls.push(call);
    return server(call, calls);
  };
  let tree;
  await act(async () => { tree = Renderer.create(element); });
  await settle();
  return {
    tree, calls,
    text: () => textOf(tree.toJSON()),
    done: async () => { await act(async () => tree.unmount()); globalThis.fetch = original; delete globalThis.rightsFixture; },
  };
}

const adminCatalog = (extra = {}) => ({
  scope: 'administrator',
  rights: {
    subscription_id: 0, state: 'unlimited', unrestricted: false,
    own: { all_permissions: true, all_software: true, permissions: [], software: [] },
    permissions: rights.PERMISSION_IDS.map((id) => ({
      id, granted: id !== 'database_remote',
      ...(id === 'remote_api' ? { server_off: true } : {}),
      ...(id === 'database_remote' ? { refused_by: 'administrator' } : {}),
    })),
    software: [
      { id: 'component:php-fpm:8.4', granted: true },
      { id: 'component:php-fpm:8.1', granted: false, refused_by: 'administrator', withdrawn: true },
      { id: 'runtime:program', granted: true },
      { id: 'component:python:3.12', granted: false, refused_by: 'administrator' },
      { id: 'component:mariadb', granted: true },
    ],
    ...extra,
  },
});

test('the Offered tab: a tick the server did not make is said, never a silent flip back', async () => {
  const view = await mount(React.createElement(OfferedItemsTab), ({ url, method }) => {
    if (url.startsWith('/api/v1/rights/catalog')) return answer(200, adminCatalog());
    if (method === 'PUT') return answer(200, { hidden: { php: [], node: [] }, hidden_offerings: [], offered_apps: [], item: { id: 'permission:database_remote', offered: false } });
    throw new Error(url);
  });
  try {
    const row = view.tree.root.findByProps({ 'data-offered-item': 'permission:database_remote' });
    assert.equal(row.props['data-offered'], 'no');
    await act(async () => { await row.findByType('input').props.onChange({ target: { checked: true } }); });
    await settle();
    const put = view.calls.find((call) => call.method === 'PUT');
    assert.deepEqual(put.body, { offering_id: 'permission:database_remote', offered: true });
    assert.ok(view.text().includes('Nothing changed: the server still has Remote access to databases hidden.'), view.text());
  } finally {
    await view.done();
  }
});

test('the Offered tab in Turkish: topics, items offered elsewhere, a withdrawn version, the remote API switch', async () => {
  const view = await mount(React.createElement(OfferedItemsTab), ({ url }) => {
    if (url.startsWith('/api/v1/rights/catalog')) return answer(200, adminCatalog());
    throw new Error(url);
  }, 'tr');
  try {
    const topics = view.tree.root.findAll((node) => node.props && node.props['data-offered-topic'] !== undefined && typeof node.type === 'string').map((node) => node.props['data-offered-topic']);
    assert.deepEqual(topics, ['dns', 'mail', 'databases', 'sites', 'files', 'apps', 'security', 'backups', 'account']);
    const text = view.text();
    for (const heading of ['Alan adları ve DNS', 'Dosyalar ve erişim', 'Hesap, ekip ve API']) assert.ok(text.includes(heading), heading);
    // Uploaded programs and Python: their state and the way to Catalog management, no box.
    for (const item of ['runtime:program', 'component:python:3.12']) {
      const row = view.tree.root.findByProps({ 'data-offered-item': item });
      assert.equal(row.findAllByType('input').length, 0, `${item} has no box that cannot change it`);
      assert.ok(textOf(row).includes(catalogues.tr['rights.offered.inCatalog']));
      await act(async () => row.findByProps({ 'data-offered-link': '/addons?view=catalog' }).props.onClick());
    }
    assert.deepEqual(globalThis.rightsFixture.navigated, ['/addons?view=catalog', '/addons?view=catalog']);
    const withdrawn = view.tree.root.findByProps({ 'data-offered-item': 'component:php-fpm:8.1' });
    assert.equal(withdrawn.findByType('input').props.disabled, true);
    assert.ok(textOf(withdrawn).includes(catalogues.tr['rights.offered.withdrawn']));
    const api = view.tree.root.findByProps({ 'data-offered-item': 'permission:remote_api' });
    assert.ok(textOf(api).includes(catalogues.tr['rights.offered.remoteApiOff']));
    assert.ok(api.findAll((node) => node.props && node.props['data-offered-link'] === '/settings?section=panel').length > 0);
  } finally {
    await view.done();
  }
});

test('a refused offer says why and where to fix it', async () => {
  const view = await mount(React.createElement(OfferedItemsTab), ({ url, method }) => {
    if (url.startsWith('/api/v1/rights/catalog')) return answer(200, adminCatalog());
    if (method === 'PUT') return answer(409, { error: 'x', code: 'VERSION_WITHDRAWN', reason: 'offer', action: '/components/php-fpm', vars: { name: 'PHP 8.4' } });
    throw new Error(url);
  });
  try {
    const row = view.tree.root.findByProps({ 'data-offered-item': 'component:php-fpm:8.4' });
    await act(async () => { await row.findByType('input').props.onChange({ target: { checked: false } }); });
    await settle();
    assert.ok(view.text().includes('Nothing was changed: PHP 8.4 is withdrawn for security on this server, so it cannot be offered.'), view.text());
    assert.ok(view.text().includes(catalogues.en['err.VERSION_WITHDRAWN.action']));
  } finally {
    await view.done();
  }
});

test('the rights editor: topics, and an item the reseller lacks says so', async () => {
  const grantor = adminCatalog().rights;
  const effective = {
    ...grantor, reseller_name: 'bayi1',
    permissions: grantor.permissions.map((item) => (item.id === 'ssh' ? { id: 'ssh', granted: false, refused_by: 'reseller' } : item)),
  };
  const selection = { permissions: ['ssh', 'mail'], software: [] };
  const element = React.createElement(RightsSetEditor, {
    grantor, selection, onSelection: () => {}, numbers: { ...rights.NO_LIMITS }, onNumbers: () => {}, idPrefix: 'x', effective,
  });
  const view = await mount(element, () => { throw new Error('no read'); }, 'tr');
  try {
    const ssh = view.tree.root.findByProps({ 'data-rights-permission': 'ssh' });
    assert.ok(textOf(ssh).includes('bayi1 adlı bayide bu hak yok'), textOf(ssh));
    assert.equal(view.tree.root.findAll((node) => node.props && node.props['data-rights-reseller-lacks'] === 'mail').length, 0);
    const db = view.tree.root.findByProps({ 'data-rights-permission': 'database_remote' });
    assert.ok(textOf(db).includes(catalogues.tr['rights.notOffered']));
    const program = view.tree.root.findAll((node) => node.props && node.props['data-rights-topic'] !== undefined && typeof node.type === 'string');
    assert.equal(program[0].props['data-rights-topic'], 'dns');
  } finally {
    await view.done();
  }
});

test('a refusal lists applications and programs by name', () => {
  const t = say('tr');
  const shown = readableRefusal({ message: 'x', details: ['app:wordpress', 'runtime:program', 'permission:ssh'] }, t);
  assert.deepEqual(shown.details, ['WordPress', t('rights.software.program'), t('rights.perm.ssh')]);
});

test('limits of site settings: one Save names the rows it saves; a row keeps its own error', async () => {
  const policy = [
    { key: 'php.memory_limit_mb', server_ceiling: 512, server_default: null, customer_editable: true, customer_choice: true },
    { key: 'cron.max_jobs', server_ceiling: null, server_default: null, customer_editable: true, customer_choice: false },
    { key: 'db.max_user_connections', server_ceiling: 50, server_default: null, customer_editable: true, customer_choice: false },
  ];
  const limits = [
    { key: 'php.memory_limit_mb', unit: 'mb', min: 16, max: 16384 },
    { key: 'cron.max_jobs', unit: 'count', min: 0, max: 1000 },
    { key: 'db.max_user_connections', unit: 'count', min: 1, max: 10000 },
  ];
  let reread = 0;
  const view = await mount(React.createElement(PolicyEditor, { policy, limits, observedAt: 1, onSaved: async () => { reread++; } }), ({ body }) => (
    body.key === 'cron.max_jobs' ? answer(400, { error: 'no', code: 'SETTING_UNKNOWN' }) : answer(200, { success: true })
  ));
  try {
    assert.equal(view.tree.root.findAll((node) => node.type === 'button' && node.props['data-limits-save'] !== undefined).length, 1, 'one Save');
    // Limit-only keys have no "customers may change it" box.
    assert.equal(view.tree.root.findByProps({ 'data-limit-row': 'cron.max_jobs' }).findAllByType('input').length, 1);
    assert.equal(view.tree.root.findByProps({ 'data-limit-row': 'php.memory_limit_mb' }).findAllByType('input').length, 2);
    const save = () => view.tree.root.findByProps({ 'data-limits-save': '' });
    assert.equal(save().props.disabled, true, 'nothing changed yet');
    const input = (key) => view.tree.root.findByProps({ 'data-limit-row': key }).findAllByType('input')[0];
    await act(async () => input('php.memory_limit_mb').props.onChange({ target: { value: '1024' } }));
    await act(async () => input('cron.max_jobs').props.onChange({ target: { value: '20' } }));
    assert.ok(view.text().includes('Will be saved: '), view.text());
    // An invalid value keeps Save off and says the range beside the row.
    await act(async () => input('db.max_user_connections').props.onChange({ target: { value: '0' } }));
    assert.equal(save().props.disabled, true);
    assert.ok(textOf(view.tree.root.findByProps({ 'data-limit-row': 'db.max_user_connections' })).includes('Between 1 and 10000.'));
    await act(async () => input('db.max_user_connections').props.onChange({ target: { value: '50' } }));
    await act(async () => { await save().props.onClick(); });
    await settle();
    const puts = view.calls.filter((call) => call.method === 'PUT');
    assert.deepEqual(puts.map((call) => call.body.key), ['php.memory_limit_mb', 'cron.max_jobs'], 'only the changed rows');
    assert.equal(puts[1].body.customer_editable, true, 'a limit-only key is never sent as fixed');
    assert.equal(reread, 1);
    const cron = view.tree.root.findByProps({ 'data-limit-row': 'cron.max_jobs' });
    assert.equal(cron.findAll((node) => node.props && node.props['data-limit-error'] === 'cron.max_jobs').length > 0, true, 'the error stays beside its row');
    assert.ok(globalThis.rightsFixture.toasts.some(([kind]) => kind === 'error'));
  } finally {
    await view.done();
  }
});
