import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { apiErrorURL, compileSource, dataModule, reactURL, remoteURL, replaceLazyImport, sharedLayer } from './fixtures/shared-layer.mjs';

// One way of saving on the administrator's and account screens (alpha.132,
// lane SAVE-SERVER; owner, 8 Nov 2026): the offered versions of a runtime,
// the Offered tab with its applications, the notification settings and the
// plan form, mounted. Nothing is sent when a setting changes; Save sends
// exactly the changes; Discard restores; a refusal, a partial failure and a
// lost answer each keep what was not saved as a marked change. (The PHP
// extensions, the mail policy, the server limits and the automatic update
// check are mounted in their screens' own tests.)
//
// Yönetici ve hesap ekranlarında tek kaydetme yolu, bağlı hâlde: değişiklik
// hiçbir şey göndermez; Kaydet yalnız değişenleri gönderir; Vazgeç geri alır.

const icons = ['AlertTriangle', 'Bell', 'Check', 'ExternalLink', 'Info', 'Mail', 'LayoutGrid', 'X', 'Lock', 'Copy', 'Layers', 'Pencil', 'Plus', 'Trash2', 'Save', 'XCircle', 'Loader2'];
const stub = dataModule(`
  import React from '${reactURL}';
  ${icons.map((name) => `export const ${name} = () => null;`).join('\n')}
  export const useI18n = () => ({ locale: 'en', t: (key, vars) => (vars ? key + JSON.stringify(vars) : key) });
  export const useNavigate = () => (to) => { globalThis.saveFixture.navigated.push(to); return true; };
  export const showToast = (...args) => globalThis.saveFixture.toasts.push(args);
  export const notificationsTranslation = () => undefined;
  export const Link = (props) => React.createElement('a', null, props.children);
  export const useLocation = () => ({ pathname: '/account', hash: '' });
  export const useAuth = () => ({ user: { impersonating: false } });
  export const usageNoticeText = () => '';
  export const unitText = () => '', whoText = () => '';
`);
const shared = sharedLayer(stub);
const rightsURL = compileSource('lib/rights.ts', (specifier) => {
  if (specifier.endsWith('/apiError')) return apiErrorURL;
  throw new Error(`lib/rights.ts imports ${specifier}`);
});
const rights = await import(rightsURL);
const usageURL = compileSource('lib/usage.ts', (specifier) => { throw new Error(`lib/usage.ts imports ${specifier}`); });
const own = {
  '/lib/rights': rightsURL,
  '/lib/accounts': compileSource('lib/accounts.ts', (specifier) => (specifier.endsWith('/remote') ? remoteURL : shared.resolve(specifier))),
  '/lib/appCatalog': compileSource('lib/appCatalog.ts', (specifier) => shared.resolve(specifier)),
  '/lib/notifications': compileSource('lib/notifications.ts', (specifier) => (specifier.endsWith('/usage') ? usageURL : shared.resolve(specifier))),
};
own['/BlockingProblems'] = shared.compile('components/BlockingProblems.tsx', own);
own['/RightsSetEditor'] = shared.compile('components/RightsSetEditor.tsx', own);
own['/AccountRights'] = shared.compile('components/AccountRights.tsx', own);
const { OfferedVersionsCard } = await import(shared.compile('components/OfferedVersionsCard.tsx', own));
// The Offered tab with its real applications part (the fixture quiets lazy parts).
const quietApps = dataModule('export const OfferedAppsSection = () => null;');
const { OfferedItemsTab, PlanEditor } = await import(replaceLazyImport(shared.compile('components/PlanRights.tsx', own), quietApps, shared.compile('components/OfferedAppsSection.tsx', own)));
const { NotificationPreferencesCard, NotificationSettingsCard } = await import(shared.compile('components/Notifications.tsx', own));

const answer = (status, body) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
const settle = async () => { for (let i = 0; i < 8; i++) await act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)); }); };
const textOf = (node) => (node === null || node === undefined || typeof node === 'boolean' ? '' : typeof node === 'string' || typeof node === 'number' ? String(node) : Array.isArray(node) ? node.map(textOf).join(' ') : textOf(node.children));

async function mount(element, server) {
  const calls = [];
  globalThis.saveFixture = { toasts: [], navigated: [] };
  const original = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    const call = { url: String(url), method: init.method ?? 'GET', body: init.body ? JSON.parse(init.body) : undefined };
    calls.push(call);
    return server(call, calls);
  };
  let tree;
  await act(async () => { tree = Renderer.create(element); });
  await settle();
  const all = (data) => tree.root.findAll((node) => typeof node.type === 'string' && node.props[data] !== undefined);
  return {
    tree, calls,
    writes: () => calls.filter((call) => call.method !== 'GET'),
    text: () => textOf(tree.toJSON()),
    marks: () => all('data-changed-mark').length,
    save: () => all('data-save').filter((node) => node.type === 'button')[0],
    discard: () => all('data-save-discard').filter((node) => node.type === 'button')[0],
    press: async (node) => { await act(async () => { await node.props.onClick({}); }); await settle(); },
    done: async () => { await act(async () => tree.unmount()); globalThis.fetch = original; delete globalThis.saveFixture; delete globalThis.confirm; delete globalThis.operationSteps; delete globalThis.operationEnds; },
  };
}
const tick = async (node, checked) => act(async () => node.props.onChange({ target: { checked, value: String(checked) }, currentTarget: { checked } }));
const type = async (node, value) => act(async () => node.props.onChange({ target: { value } }));

// ---- the offered versions of a runtime ------------------------------------------

test('offered versions: ticking sends nothing; Save sends each changed line once; a refused line stays marked, Discard gives it up', async () => {
  let hidden = ['8.2'];
  const view = await mount(React.createElement(OfferedVersionsCard, { component: 'php', installed: ['8.3.4', '8.2.20', '8.1.30'] }), ({ url, method, body }) => {
    if (url === '/api/v1/offered-versions' && method === 'PUT') {
      if (body.version === '8.2') return answer(409, { error: 'no', code: 'VERSION_WITHDRAWN' });
      hidden = body.offered ? hidden.filter((line) => line !== body.version) : [...hidden, body.version];
      return answer(200, { success: true });
    }
    if (url === '/api/v1/offered-versions') return answer(200, { hidden: { php: hidden, node: [] }, restricted: false });
    return answer(404, { error: 'none' });
  });
  try {
    const box = (line) => view.tree.root.findByProps({ 'data-offered-line': line }).findByType('input');
    assert.equal(view.save(), undefined, 'Save is shown with nothing changed');
    await tick(box('8.3'), false);
    await tick(box('8.2'), true);
    assert.equal(view.writes().length, 0, 'ticking a line saved it');
    assert.equal(view.marks(), 2);
    assert.ok(view.text().includes('2 changes not saved.'), view.text());
    await view.press(view.save());
    assert.deepEqual(view.writes().map((call) => call.body), [
      { component: 'php', version: '8.3', offered: false },
      { component: 'php', version: '8.2', offered: true },
    ], 'exactly the changed lines, one request each');
    const report = textOf(view.tree.root.findByProps({ 'data-offered-failure': '' }));
    assert.ok(report.includes('life.offer.savedSome{"what":"PHP 8.3"}') && report.includes('life.offer.notSaved{"what":"PHP 8.2"'), report);
    assert.equal(box('8.3').props.checked, false, 'the saved line shows what the server holds');
    assert.equal(box('8.2').props.checked, true, 'the refused line keeps the person’s choice');
    assert.equal(view.marks(), 1, 'only the refused line is still a change');
    await view.press(view.discard());
    assert.equal(box('8.2').props.checked, false);
    assert.equal(view.save(), undefined);
    assert.equal(view.writes().length, 2, 'Discard sent something');
  } finally {
    await view.done();
  }
});

test('offered versions: a lost answer says the result is not known and sends nothing twice', async () => {
  const view = await mount(React.createElement(OfferedVersionsCard, { component: 'php', installed: ['8.3.4'] }), ({ url, method }) => {
    if (method === 'PUT') throw new TypeError('fetch failed');
    if (url === '/api/v1/offered-versions') return answer(200, { hidden: { php: [], node: [] }, restricted: false });
    return answer(404, { error: 'none' });
  });
  try {
    await tick(view.tree.root.findByProps({ 'data-offered-line': '8.3' }).findByType('input'), false);
    await view.press(view.save());
    assert.equal(view.writes().length, 1);
    assert.ok(view.text().includes('it is not known whether this was saved'), view.text());
    assert.ok(view.text().includes('common.resultUnknown'));
    assert.equal(view.marks(), 1, 'the change whose answer was lost is still shown as a change');
  } finally {
    await view.done();
  }
});

// ---- the Offered tab: permissions, software and applications, one draft ----------

const permissions = rights.PERMISSION_IDS.map((id) => ({ id, granted: id !== 'ssh' }));
const catalog = { scope: 'administrator', rights: { subscription_id: 0, state: 'unlimited', unrestricted: false, own: { all_permissions: true, all_software: true, permissions: [], software: [] }, permissions, software: [{ id: 'component:mariadb', granted: true }] } };

test('the Offered tab: one bar for permissions and applications; offering an application asks with its risk; a refusal stays marked with its fix', async () => {
  let joomla = false;
  const view = await mount(React.createElement(OfferedItemsTab), ({ url, method, body }) => {
    if (url.startsWith('/api/v1/rights/catalog')) return answer(200, catalog);
    if (url === '/api/v1/apps') return answer(200, { apps: [{ id: 'joomla', name: 'Joomla', offered: joomla, risk: 'Extensions need care.' }] });
    if (method === 'PUT' && body.offering_id === 'permission:ssh') return answer(409, { error: 'no', code: 'VERSION_WITHDRAWN', action: '/components/php-fpm' });
    if (method === 'PUT') { joomla = body.offered; return answer(200, { item: { id: body.offering_id, offered: body.offered } }); }
    return answer(404, { error: 'none' });
  });
  try {
    const ssh = () => view.tree.root.findByProps({ 'data-offered-item': 'permission:ssh' }).findByType('input');
    const joomlaBox = () => view.tree.root.findByProps({ 'data-offered-app': 'joomla' }).findByType('input');
    await tick(ssh(), true);
    await tick(joomlaBox(), true);
    assert.equal(view.writes().length, 0, 'ticking saved');
    assert.equal(view.tree.root.findAll((node) => node.type === 'button' && node.props['data-save'] !== undefined).length, 1, 'one save bar for the whole list');
    assert.ok(view.text().includes('2 changes not saved.'));
    // Declined: nothing is sent and the changes stay.
    const asked = [];
    globalThis.confirm = (text) => { asked.push(text); return false; };
    await view.press(view.save());
    assert.equal(view.writes().length, 0);
    assert.ok(asked[0].includes('appOffer.confirm') && asked[0].includes('Extensions need care.'), asked[0]);
    assert.equal(view.marks(), 2);
    globalThis.confirm = () => true;
    globalThis.operationSteps = [];
    globalThis.operationEnds = [];
    await view.press(view.save());
    assert.deepEqual(view.writes().map((call) => call.body), [
      { offering_id: 'permission:ssh', offered: true },
      { offering_id: 'app:joomla', offered: true },
    ]);
    // Confirmed, the changes run in the progress window, one step each.
    assert.deepEqual(globalThis.operationSteps.at(-1).map((step) => [step.id, step.state]), [['permission:ssh', 'failed'], ['app:joomla', 'done'], ['read', 'done']]);
    assert.equal(globalThis.operationEnds.at(-1).state, 'failed');
    assert.ok(globalThis.operationEnds.at(-1).reason.includes('rights.offered.next'));
    const report = view.tree.root.findByProps({ 'data-offered-report': '' });
    assert.ok(textOf(report).includes('rights.offered.savedSome{"what":"Joomla"}'), textOf(report));
    assert.ok(textOf(report).includes('rights.offered.notSaved'));
    // The refusal's own fix is offered beside it.
    await view.press(report.findAllByType('button')[0]);
    assert.deepEqual(globalThis.saveFixture.navigated, ['/components/php-fpm']);
    assert.equal(ssh().props.checked, true, 'the refused item keeps the person’s choice');
    assert.equal(joomlaBox().props.checked, true);
    assert.equal(view.marks(), 1, 'only the refused item is still a change');
  } finally {
    await view.done();
  }
});

// ---- notification settings ----------------------------------------------------

test('my email choice: nothing is sent on change, a malformed address keeps Save off, a lost answer holds Save until checked', async () => {
  let lose = true;
  const view = await mount(React.createElement(NotificationPreferencesCard), ({ url, method }) => {
    if (url === '/api/v1/notification-preferences' && method === 'PUT') {
      if (lose) { lose = false; throw new TypeError('fetch failed'); }
      return answer(200, { success: true });
    }
    if (url === '/api/v1/notification-preferences') return answer(200, { mode: 'all', extra_email: '', own_email_set: true, mail_available: true });
    return answer(404, { error: 'none' });
  });
  try {
    const radio = (mode) => view.tree.root.findByProps({ 'data-notification-mode': mode }).findByType('input');
    const extra = () => view.tree.root.find((node) => node.type === 'input' && node.props['data-notification-extra'] !== undefined);
    await act(async () => radio('action').props.onChange());
    await type(extra(), 'desk@');
    assert.equal(view.writes().length, 0);
    assert.equal(view.marks(), 2);
    assert.equal(view.save().props.disabled, true, 'a malformed address can be saved');
    assert.ok(view.text().includes('notif.badEmail'));
    await type(extra(), ' desk@example.com ');
    assert.equal(view.save().props.disabled, false);
    await view.press(view.save());
    assert.deepEqual(view.writes()[0].body, { mode: 'action', extra_email: 'desk@example.com' });
    assert.ok(view.text().includes('it is not known whether this was saved'));
    assert.equal(view.save().props.disabled, true, 'Save after a lost answer, before the setting was read again');
    await view.press(view.tree.root.findAll((node) => node.type === 'button' && node.props['data-save-check'] !== undefined)[0]);
    assert.equal(view.save().props.disabled, false);
    await view.press(view.save());
    assert.equal(view.writes().length, 2);
  } finally {
    await view.done();
  }
});

test('server-wide notification settings: Save sends the form once; Discard restores it', async () => {
  const view = await mount(React.createElement(NotificationSettingsCard), ({ url, method }) => {
    if (method === 'PUT') return answer(200, { success: true });
    if (url === '/api/v1/panel/notification-settings') return answer(200, { expiry_days_before: 7, expiry_choices: [3, 7, 14], extra: { monitoring: '', account: '', security: '' } });
    return answer(404, { error: 'none' });
  });
  try {
    const select = () => view.tree.root.find((node) => node.type === 'select');
    const group = (name) => view.tree.root.find((node) => node.type === 'input' && node.props['data-notification-group'] === name);
    await act(async () => select().props.onChange({ target: { value: '14' } }));
    await type(group('security'), 'sec@example.com');
    assert.equal(view.writes().length, 0);
    assert.equal(view.marks(), 2);
    await view.press(view.discard());
    assert.equal(select().props.value, 7);
    assert.equal(group('security').props.value, '');
    assert.equal(view.save(), undefined);
    await type(group('monitoring'), ' ops@example.com');
    await view.press(view.save());
    assert.deepEqual(view.writes().map((call) => call.body), [{ expiry_days_before: 7, extra: { monitoring: 'ops@example.com', account: '', security: '' } }]);
  } finally {
    await view.done();
  }
});

// ---- the plan form --------------------------------------------------------------

test('editing a plan: Save appears only with a change, changes are marked, Discard restores, closing with changes asks', async () => {
  const grantor = catalog.rights;
  const initial = rights.editedFromCustomerPlan({ id: 4, name: 'Basic', max_domains: 3, max_databases: 1, max_email_accounts: 2, disk_quota_mb: 100, bandwidth_quota_mb: 0, rights: { all_permissions: true, all_software: true, permissions: [], software: [] } });
  let closed = 0;
  const view = await mount(React.createElement(PlanEditor, { kind: 'customer', initial, grantor, grantorIsReseller: false, onClose: () => { closed++; } }),
    () => answer(200, { success: true }));
  try {
    const name = () => view.tree.root.find((node) => node.type === 'input' && node.props.id === 'plan-customer-4-name');
    const domains = () => view.tree.root.find((node) => node.type === 'input' && node.props['data-rights-quantity'] === 'max_domains');
    assert.equal(view.save(), undefined, 'Save is shown with nothing changed');
    await type(name(), 'Basic plus');
    await type(domains(), '5');
    assert.equal(view.marks(), 2);
    assert.ok(view.text().includes('2 changes not saved.'));
    await view.press(view.discard());
    assert.equal(name().props.value, 'Basic');
    assert.equal(domains().props.value, 3);
    assert.equal(view.save(), undefined);
    await type(domains(), '5');
    // Closing with a change asks; staying keeps the form.
    const asked = [];
    globalThis.confirm = (text, request) => { asked.push(request); return false; };
    await view.press(view.tree.root.find((node) => node.type === 'button' && node.props['data-plan-close'] !== undefined));
    assert.equal(asked.length, 1);
    assert.equal(closed, 0);
    await view.press(view.save());
    assert.equal(view.writes().length, 1);
    assert.equal(view.writes()[0].body.max_domains, 5);
    assert.equal(view.writes()[0].body.rights, undefined, 'the rights set was sent although it was not touched');
    assert.equal(closed, 1);
  } finally {
    await view.done();
  }
});

test('the Users page asks before a tab with unsaved changes is left', () => {
  const source = readFileSync(new URL('../src/components/UsersPage.tsx', import.meta.url), 'utf8');
  assert.match(source, /const go = \(next: UsersTab\) => \{\n\s+if \(next !== tab\) void confirmLeaveIfDirty\(\)\.then/);
  assert.doesNotMatch(source, /onClick=\{\(\) => setTab\(/, 'a tab is switched without asking');
});
