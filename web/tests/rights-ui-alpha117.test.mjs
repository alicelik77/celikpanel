// Plans and rights screens (alpha.117, D-042, docs/PLANS-AND-RIGHTS.md 3-6, 9).
//
// The pure helpers are the shipped lib/rights.ts; the forms are mounted with
// the shipped ui.tsx, remote.ts and apiError.ts and the real English and
// Turkish catalogues, against a server that answers like the Go handlers do:
// a first save is answered with a preview (RIGHTS_LEAVES_PLAN,
// PLAN_NARROWING_CONFIRM), and only the confirmed second save changes
// anything. A refusal says why and who grants the right, in the reader's
// language. Menus and tabs follow what the server says is closed.
//
// Plan ve hak ekranları: saf yardımcılar gönderilen modüldür; formlar gerçek
// katalogla bağlanır. İlk kayda sunucu önizlemeyle yanıt verir; yalnız
// onaylanan ikinci kayıt değiştirir.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
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
const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');

const rightsURL = compileSource('lib/rights.ts', (specifier) => {
  if (specifier.endsWith('/apiError')) return apiErrorURL;
  throw new Error(`lib/rights.ts imports ${specifier}`);
});
const rights = await import(rightsURL);
const apiError = await import(apiErrorURL);
const remote = await import(remoteURL);

// ---- pure helpers -----------------------------------------------------------

test('names: software by product and version, permissions by plain label, states by name', () => {
  const t = say('en');
  assert.equal(rights.softwareName('component:php-fpm:8.3'), 'PHP 8.3');
  assert.equal(rights.softwareName('component:node:22'), 'Node.js 22');
  assert.equal(rights.softwareName('component:mariadb'), 'MariaDB');
  assert.equal(rights.softwareName('component:postgresql:16'), 'PostgreSQL 16');
  assert.equal(rights.softwareName('integration:acme:letsencrypt'), 'Let’s Encrypt');
  assert.equal(rights.itemLabel('permission:dns_edit', t), 'Edit DNS records');
  assert.equal(rights.itemLabel('permission:dns_edit', say('tr')), 'DNS kayıtlarını düzenleme');
  assert.equal(rights.stateName('unlimited', undefined, t), 'Unlimited');
  assert.equal(rights.stateName('unlimited', undefined, t, true), 'Unlimited reseller');
  assert.equal(rights.stateName('plan', 'Basic', t), 'Plan: Basic');
  assert.equal(rights.stateName('plan:4', 'Basic', say('tr')), 'Plan: Basic');
  assert.equal(rights.stateName('custom', undefined, say('tr')), 'Özel');
  // Grouped by component, newest version first, PHP before Node.js.
  const groups = rights.groupSoftware([{ id: 'component:node:20' }, { id: 'component:php-fpm:8.3' }, { id: 'component:php-fpm:8.10' }, { id: 'component:node:22' }, { id: 'integration:acme:zerossl' }]);
  assert.deepEqual(groups.map((g) => g.group), ['component:php-fpm', 'component:node', 'integration:acme']);
  assert.deepEqual(groups[0].items.map((i) => i.id), ['component:php-fpm:8.10', 'component:php-fpm:8.3']);
  assert.deepEqual(groups[1].items.map((i) => i.id), ['component:node:22', 'component:node:20']);
});

const grantor = {
  subscription_id: 0, state: 'unlimited', unrestricted: false,
  own: { all_permissions: true, all_software: true, permissions: [], software: [] },
  permissions: rights.PERMISSION_IDS.map((id) => ({ id, granted: id !== 'ssh', ...(id === 'ssh' ? { refused_by: 'reseller' } : {}) })),
  software: [
    { id: 'component:php-fpm:8.3', granted: true }, { id: 'component:php-fpm:8.5', granted: true },
    { id: 'component:node:22', granted: false, refused_by: 'reseller' },
  ],
};

test('a set: "everything offered" becomes what the grantor may give; only that is selectable', () => {
  const all = rights.selectionOf({ all_permissions: true, all_software: true, permissions: [], software: [] }, grantor);
  assert.ok(!all.permissions.includes('ssh'), 'a permission the grantor may not give is not selected');
  assert.deepEqual(all.software, ['component:php-fpm:8.3', 'component:php-fpm:8.5']);
  const payload = rights.payloadOf({ permissions: ['mail', 'dns_edit'], software: ['component:php-fpm:8.5', 'component:php-fpm:8.3', 'component:php-fpm:8.3'] });
  assert.deepEqual(payload, { all_permissions: false, all_software: false, permissions: ['dns_edit', 'mail'], software: ['component:php-fpm:8.3', 'component:php-fpm:8.5'] });
  assert.equal(rights.grantable(grantor, 'permissions', 'ssh'), false);
  assert.equal(rights.grantable(grantor, 'software', 'component:node:22'), false);
  assert.deepEqual(rights.outsideGrantor({ permissions: ['ssh', 'mail'], software: ['component:node:22'] }, grantor), ['permission:ssh', 'component:node:22']);
  assert.equal(rights.quantity('-3'), 0);
  assert.equal(rights.quantity('12'), 12);
  assert.equal(rights.quantity('x'), 0);
});

test('the account form sends Unlimited by name, a plan, or a Custom set; Create says what is missing', () => {
  assert.deepEqual(rights.newAccountRightsBody(rights.UNLIMITED_CHOICE, 'customer'), { state: 'unlimited' });
  assert.deepEqual(rights.newAccountRightsBody({ ...rights.UNLIMITED_CHOICE, state: 'plan', planID: 7 }, 'customer'), { state: 'plan', plan_id: 7 });
  const custom = rights.newAccountRightsBody({ state: 'custom', planID: 0, selection: { permissions: ['mail'], software: [] }, numbers: { ...rights.NO_LIMITS, max_domains: 3, max_customers: 9 } }, 'customer');
  assert.deepEqual(custom.quantities, { max_domains: 3, max_databases: 0, max_email_accounts: 0, disk_quota_mb: 0, traffic_quota_mb: 0 }, 'a customer has no customers count');
  const reseller = rights.newAccountRightsBody({ state: 'custom', planID: 0, selection: { permissions: [], software: [] }, numbers: { ...rights.NO_LIMITS, max_customers: 9 } }, 'reseller');
  assert.equal(reseller.quantities.max_customers, 9);
  assert.deepEqual(rights.newAccountRightsMissing({ ...rights.UNLIMITED_CHOICE, state: 'plan', planID: 0 }, say('en')), ['Choose a plan']);
  assert.deepEqual(rights.newAccountRightsMissing(rights.UNLIMITED_CHOICE, say('en')), []);
  // The plans an account may be given.
  const plans = [{ id: 1, name: 'Global' }, { id: 2, name: 'Mine', owner_id: 30 }, { id: 3, name: 'Other', owner_id: 31 }];
  assert.deepEqual(rights.assignablePlans(plans, { parent_id: 30 }, true).map((p) => p.id), [1, 2]);
  assert.deepEqual(rights.assignablePlans(plans, {}, true).map((p) => p.id), [1]);
  assert.deepEqual(rights.assignablePlans([plans[0], plans[1]], {}, false).map((p) => p.id), [1, 2]);
});

test('a plan is saved with its rights only when they were chosen; over-usage is confirmed only where the server takes it', () => {
  const edited = rights.editedFromCustomerPlan({ id: 4, name: 'Basic', max_domains: 5, max_databases: 1, max_email_accounts: 2, disk_quota_mb: 100, bandwidth_quota_mb: 0, rights: { all_permissions: true, all_software: true, permissions: [], software: [] } });
  assert.equal(rights.planBody('customer', edited, null).rights, undefined, 'an untouched "everything offered" stays');
  const body = rights.planBody('customer', edited, { permissions: ['mail'], software: [] }, { narrowing: true, overUsage: true });
  assert.deepEqual(body.rights.permissions, ['mail']);
  assert.equal(body.confirm_narrowing, true);
  assert.equal(body.accept_over_usage, true);
  assert.equal(body.max_customers, undefined);
  const reseller = rights.planBody('reseller', { ...edited, numbers: { ...edited.numbers, max_customers: 4 } }, null, { narrowing: true, overUsage: true });
  assert.equal(reseller.accept_over_usage, undefined, 'a reseller plan has no such field');
  assert.equal(reseller.max_customers, 4);
  assert.equal(reseller.bandwidth_quota_mb, undefined);
  // alpha.118 (D-044): a customer plan keeps its traffic in bandwidth_quota_mb,
  // a reseller plan in traffic_quota_mb.
  const traffic = rights.planBody('customer', { ...edited, numbers: { ...edited.numbers, traffic_quota_mb: 2048 } }, null);
  assert.equal(traffic.bandwidth_quota_mb, 2048);
  assert.equal(traffic.traffic_quota_mb, undefined);
  assert.equal(rights.planBody('reseller', { ...edited, numbers: { ...edited.numbers, traffic_quota_mb: 512 } }, null).traffic_quota_mb, 512);
  assert.equal(rights.editedFromCustomerPlan({ ...edited, max_domains: 1, max_databases: 1, max_email_accounts: 1, disk_quota_mb: 1, bandwidth_quota_mb: 300 }).numbers.traffic_quota_mb, 300);
});

test('usage lines the server writes read in the owner’s language', () => {
  const t = say('en');
  assert.equal(rights.overLimitText('domains: 5 in use, limit 3', t), 'Domains: 5 in use, the limit is 3');
  assert.equal(rights.overLimitText('bob (Basic, #7) - disk: 900 MB in use, limit 500 MB', t), 'bob (Basic, #7): Disk: 900 MB in use, the limit is 500 MB');
  assert.equal(rights.overLimitText('reseller #12 customers: 4 in use, pool 3', t), 'Reseller #12: Customers: 4 in use, the limit is 3');
  assert.equal(rights.overLimitText('something else', t), 'something else');
  assert.equal(rights.overLimitText('mailboxes: 9 in use, pool 8', say('tr')), 'Posta kutusu: 9 kullanımda, sınır 8');
  assert.equal(rights.inUseText({ item: 'component:php-fpm:8.5', count: 3, what: 'sites' }, t), 'PHP 8.5: 3 site(s)');
  assert.equal(rights.limitWords(0, t), 'Unlimited');
  assert.equal(rights.limitWords(2048, t, 'MB'), '2 GB');
  assert.equal(rights.rightsSummary(undefined, t), 'Everything offered');
  assert.equal(rights.rightsSummary({ all_permissions: false, all_software: true, permissions: ['mail', 'cron'], software: [] }, t), '2 permissions · all software');
});

test('decoders: an answer that is not the contract is unknown, never a default', () => {
  assert.throws(() => rights.decodeCatalog({}));
  assert.throws(() => rights.decodeUserRights({ role: 'admin' }));
  assert.throws(() => rights.decodeNotices({}));
  const view = rights.decodeCatalog({ scope: 'reseller', rights: { ...grantor, permissions: null } });
  assert.deepEqual(view.permissions, []);
  const customer = rights.decodeUserRights({ role: 'customer', subscriptions: [{ id: 5, name: 'Unlimited', state: 'unlimited' }] });
  assert.equal(customer.subscriptions[0].id, 5);
  const reseller = rights.decodeUserRights({ role: 'reseller', rights: grantor, pool: null, used: { customers: 2, domains: 1 }, over_limit: null });
  assert.equal(reseller.pool, null);
  assert.equal(reseller.used.customers, 2);
  assert.deepEqual(reseller.over_limit, []);
  assert.equal(rights.decodeNotices({ notices: null }).length, 0);
  assert.equal(rights.decodeResellerPlans({ plans: [{ id: 1, name: 'R', max_customers: 3, rights: grantor.own }], builtin: [{ key: 'unlimited', name: 'Unlimited reseller', kind: 'reseller' }] }).plans[0].max_customers, 3);
});

test('every read that shows rights is read again after a rights change', () => {
  for (const url of ['/api/v1/rights/catalog', '/api/v1/rights/notices', '/api/v1/subscriptions/5/rights', '/api/v1/users/7/rights',
    '/api/v1/reseller-plans', '/api/v1/plans', '/api/v1/my/subscription', '/api/v1/domains', '/api/v1/offered-versions', '/api/v1/hosting/capabilities']) {
    assert.ok(remote.readKeyCovers('rights', url), url);
  }
  assert.ok(!remote.readKeyCovers('rights', '/api/v1/domains/3/mail/accounts'));
});

// ---- refusals: one place, the reader's words ----------------------------------

test('a refused right says why and who grants it, in English and in Turkish', () => {
  const refusal = { message: 'x', code: 'RIGHT_NOT_GRANTED', reason: 'subscription', action: '/account', vars: { right: 'cron', name: 'Scheduled tasks', grantor: 'your provider (bob)' } };
  const en = apiError.apiErrorText(refusal, say('en'));
  assert.equal(en, 'Nothing was changed: Scheduled tasks is not in this subscription’s rights. Ask your provider (bob) to include it; it can be used as soon as it is included. What already runs keeps running.');
  const tr = apiError.apiErrorText(refusal, say('tr'));
  assert.equal(tr, 'Hiçbir şey değişmedi: Zamanlanmış görevler bu aboneliğin haklarında yok. Eklemesini hizmet sağlayıcınızdan (bob) isteyin; eklendiği anda kullanılabilir. Çalışan her şey çalışmaya devam eder.');
  assert.equal(apiError.apiErrorActionLabel(refusal, say('tr')), 'Planıma bak');
  const offered = apiError.apiErrorText({ message: 'x', code: 'NOT_OFFERED', reason: 'administrator', vars: { item: 'component:node:22', name: 'Node.js 22', grantor: 'the administrator' } }, say('tr'));
  assert.match(offered, /^Hiçbir şey değişmedi: Node\.js 22 bu sunucuda sunulmuyor\. .* sunmasını yöneticiden isteyin\./);
});

// ---- mounted forms ----------------------------------------------------------

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
const besideRemote = (specifier) => (specifier.endsWith('/remote') ? remoteURL : shared.resolve(specifier));
const accountsURL = compileSource('lib/accounts.ts', besideRemote);
const own = { '/lib/rights': rightsURL, '/lib/accounts': accountsURL };
own['/BlockingProblems'] = shared.compile('components/BlockingProblems.tsx', own);
own['/RightsSetEditor'] = shared.compile('components/RightsSetEditor.tsx', own);
own['/AccountRights'] = shared.compile('components/AccountRights.tsx', own);
const { RightsChangeForm, NewAccountRightsSection } = await import(own['/AccountRights']);
const { PlanEditor } = await import(shared.compile('components/PlanRights.tsx', own));

const textOf = (node) => (node === null || node === undefined || typeof node === 'boolean' ? '' : typeof node === 'string' || typeof node === 'number' ? String(node) : Array.isArray(node) ? node.map(textOf).join(' ') : textOf(node.children));
const answer = (status, body) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });

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
  return {
    tree, calls,
    text: () => textOf(tree.toJSON()),
    press: async (node) => { await act(async () => { await node.props.onClick(); }); },
    done: async () => { await act(async () => tree.unmount()); globalThis.fetch = original; delete globalThis.rightsFixture; },
  };
}

test('an account on a plan: one changed tick shows the exact "leaves the plan" sentence, and only the confirmation changes it', async () => {
  const current = { ...grantor, subscription_id: 5, state: 'plan', plan_id: 3, plan_name: 'Basic' };
  const preview = { from: 'plan:3', from_plan_name: 'Basic', to: 'custom', leaves_plan: true, added: [], removed: ['permission:dns_edit'], in_use: [], over_limit: [], outside_grantor: [], message: 'x' };
  let done = 0;
  const view = await mount(React.createElement(RightsChangeForm, {
    target: { kind: 'subscription', url: '/api/v1/subscriptions/5/rights', current, numbers: { ...rights.NO_LIMITS } },
    grantor, plans: [{ id: 3, name: 'Basic' }], grantorIsReseller: false, onDone: () => { done++; },
  }), ({ body }) => (body.confirm ? answer(200, { state: 'custom' }) : answer(409, { error: preview.message, code: 'RIGHTS_LEAVES_PLAN', preview })));
  try {
    const custom = view.tree.root.findByProps({ 'data-rights-choice': 'custom' }).findByType('input');
    await act(async () => custom.props.onChange());
    const dns = view.tree.root.findByProps({ 'data-rights-permission': 'dns_edit' }).findByType('input');
    assert.equal(dns.props.checked, true);
    await act(async () => dns.props.onChange({ target: { checked: false } }));
    // A permission the editor may not give stays visible, fixed, with the reason.
    const ssh = view.tree.root.findByProps({ 'data-rights-permission': 'ssh' });
    assert.equal(ssh.findByType('input').props.disabled, true);
    assert.ok(textOf(ssh).includes(catalogues.en['rights.notOffered']));

    await view.press(view.tree.root.findAll((n) => n.type === 'button' && textOf(n).includes('Review the change'))[0]);
    assert.equal(view.calls.length, 1);
    assert.equal(view.calls[0].method, 'PUT');
    assert.equal(view.calls[0].body.state, 'custom');
    assert.equal(view.calls[0].body.confirm, undefined, 'the first save asks; it does not confirm');
    assert.ok(!view.calls[0].body.rights.permissions.includes('dns_edit'));
    const page = view.text();
    assert.ok(page.includes('With this change the account leaves the "Basic" plan and becomes Custom. Later changes to the plan will not reach it. Continue?'), page);
    assert.ok(page.includes('Taken away:') && page.includes('Edit DNS records'));
    assert.equal(done, 0);

    await view.press(view.tree.root.findByProps({ 'data-rights-confirm': '' }));
    assert.equal(view.calls.length, 2);
    assert.equal(view.calls[1].body.confirm, true);
    assert.deepEqual(view.calls[1].body.rights, view.calls[0].body.rights, 'the confirmed change is the one previewed');
    assert.equal(done, 1);
    assert.ok(globalThis.rightsFixture.toasts.some(([kind, text]) => kind === 'success' && text === catalogues.en['rights.saved']));
  } finally {
    await view.done();
  }
});

test('Turkish: the same sentence in Turkish', async () => {
  const current = { ...grantor, subscription_id: 5, state: 'plan', plan_id: 3, plan_name: 'Temel' };
  const preview = { from: 'plan:3', from_plan_name: 'Temel', to: 'custom', leaves_plan: true, added: [], removed: [], in_use: [], over_limit: [], outside_grantor: [], message: 'x' };
  const view = await mount(React.createElement(RightsChangeForm, {
    target: { kind: 'subscription', url: '/api/v1/subscriptions/5/rights', current, numbers: { ...rights.NO_LIMITS } },
    grantor, plans: [], grantorIsReseller: true, onDone() {},
  }), () => answer(409, { code: 'RIGHTS_LEAVES_PLAN', preview }), 'tr');
  try {
    await act(async () => view.tree.root.findByProps({ 'data-rights-choice': 'custom' }).findByType('input').props.onChange());
    await view.press(view.tree.root.findAll((n) => n.type === 'button' && textOf(n).includes('Değişikliği gözden geçir'))[0]);
    assert.ok(view.text().includes('Bu değişiklikle hesap "Temel" planından çıkar ve Özel olur. Plandaki sonraki değişiklikler ona ulaşmaz. Devam edilsin mi?'));
  } finally {
    await view.done();
  }
});

test('choosing a plan with no plan to choose: the reason stands beside the disabled button, with a one-click way out', async () => {
  let created = 0;
  const current = { ...grantor, subscription_id: 5, state: 'unlimited' };
  const view = await mount(React.createElement(RightsChangeForm, {
    target: { kind: 'subscription', url: '/api/v1/subscriptions/5/rights', current, numbers: { ...rights.NO_LIMITS } },
    grantor, plans: [], grantorIsReseller: false, onDone() {}, onCreatePlan: () => { created++; },
  }), () => answer(500, {}));
  try {
    await act(async () => view.tree.root.findByProps({ 'data-rights-choice': 'plan' }).findByType('input').props.onChange());
    const review = view.tree.root.findAll((n) => n.type === 'button' && textOf(n).includes('Review the change'))[0];
    assert.equal(review.props.disabled, true);
    assert.ok(review.props['aria-describedby'], 'the disabled button names its reason');
    assert.ok(view.text().includes(catalogues.en['rights.noPlan']));
    await view.press(view.tree.root.findAll((n) => n.type === 'button' && textOf(n).includes('Create a plan'))[0]);
    assert.equal(created, 1);
    assert.equal(view.calls.length, 0, 'nothing was sent');
  } finally {
    await view.done();
  }
});

test('editing a plan: who follows it, what keeps running, who is over a new limit - then saved only on confirmation', async () => {
  const preview = {
    from: 'plan:4', to: 'plan:4', leaves_plan: false, added: [], removed: ['component:php-fpm:8.5'], subscriptions: 3,
    in_use: [{ item: 'component:php-fpm:8.5', count: 3, what: 'sites' }], over_limit: ['bob (Basic, #7) - domains: 5 in use, limit 3'], outside_grantor: [], message: 'x',
  };
  let closed = 0;
  const initial = rights.editedFromCustomerPlan({ id: 4, name: 'Basic', max_domains: 3, max_databases: 1, max_email_accounts: 2, disk_quota_mb: 100, bandwidth_quota_mb: 0, rights: { all_permissions: true, all_software: true, permissions: [], software: [] } });
  const view = await mount(React.createElement(PlanEditor, { kind: 'customer', initial, grantor, grantorIsReseller: false, onClose: () => { closed++; } }),
    ({ body }) => (body.confirm_narrowing ? answer(200, { success: true }) : answer(409, { code: 'PLAN_NARROWING_CONFIRM', preview })));
  try {
    const php85 = view.tree.root.findByProps({ 'data-rights-software': 'component:php-fpm:8.5' }).findByType('input');
    await act(async () => php85.props.onChange({ target: { checked: false } }));
    await view.press(view.tree.root.findAll((n) => n.type === 'button' && textOf(n).trim() === 'Save')[0]);
    assert.equal(view.calls[0].url, '/api/v1/plans/4');
    assert.deepEqual(view.calls[0].body.rights.software, ['component:php-fpm:8.3']);
    const page = view.text();
    assert.ok(page.includes('3 account(s) follow this plan. What is taken away keeps running where it is used, but cannot be chosen again. Save?'), page);
    assert.ok(page.includes('In use now (keeps running, cannot be chosen again):') && page.includes('PHP 8.5: 3 site(s)'));
    assert.ok(page.includes('bob (Basic, #7): Domains: 5 in use, the limit is 3'));
    const confirm = view.tree.root.findByProps({ 'data-plan-confirm': '' });
    assert.equal(textOf(confirm).trim(), catalogues.en['plans.saveAnyway']);
    await view.press(confirm);
    assert.equal(view.calls[1].body.confirm_narrowing, true);
    assert.equal(view.calls[1].body.accept_over_usage, true);
    assert.equal(closed, 1);
  } finally {
    await view.done();
  }
});

test('the account form: Unlimited is chosen by name; Custom starts from what the creator may give', async () => {
  let choice = rights.UNLIMITED_CHOICE;
  const render = () => React.createElement(NewAccountRightsSection, {
    role: 'customer', choice, onChoice: (next) => { choice = next; }, customerPlans: [{ id: 2, name: 'Basic' }], grantorIsReseller: true,
  });
  const view = await mount(render(), ({ url }) => (url === '/api/v1/rights/catalog' ? answer(200, { scope: 'reseller', rights: grantor }) : answer(404, {})));
  try {
    const unlimited = view.tree.root.findByProps({ 'data-new-rights': 'unlimited' });
    assert.equal(unlimited.findByType('input').props.checked, true);
    assert.ok(textOf(unlimited).includes('Unlimited') && textOf(unlimited).includes(catalogues.en['rights.unlimitedHintReseller']));
    await act(async () => view.tree.root.findByProps({ 'data-new-rights': 'custom' }).findByType('input').props.onChange());
    assert.equal(choice.state, 'custom');
    assert.ok(!choice.selection.permissions.includes('ssh'), 'nothing outside the creator’s own field');
    assert.deepEqual(choice.selection.software, ['component:php-fpm:8.3', 'component:php-fpm:8.5']);
    await act(async () => view.tree.update(render()));
    assert.ok(view.tree.root.findByProps({ 'data-rights-software': 'component:node:22' }).findByType('input').props.disabled);
    assert.ok(view.text().includes(catalogues.en['rights.notYours']));
  } finally {
    await view.done();
  }
});

// ---- menus and tabs ------------------------------------------------------------

test('menus and tabs follow what the server says is closed', async () => {
  const nav = await import(compileSource('nav.ts', () => dataModule('export const LayoutDashboard = 0, Globe = 0, Database = 0, Server = 0, Users = 0, Settings = 0, DownloadCloud = 0, ScrollText = 0, Package = 0, Shield = 0, Activity = 0, Network = 0, ShieldCheck = 0, Wrench = 0, UserCheck = 0, ServerCog = 0;')));
  const ids = (access) => nav.navItemsForRole('customer', access).map((item) => item.id);
  assert.ok(ids({ accountType: 'account', databases: true }).includes('databases'));
  assert.ok(!ids({ accountType: 'account', databases: false }).includes('databases'));
  assert.ok(ids({ accountType: 'account' }).includes('databases'), 'an older server that does not say keeps the page');
  assert.ok(nav.navItemsForRole('admin', { databases: false }).some((item) => item.id === 'databases'));
  assert.match(source('lib/api.ts'), /databases: rawFeatures\.databases !== false,/);
  // alpha.132: the rights decide in the registry of a domain's sections and
  // in the section files that hand them to their screens.
  const detail = ['domain/sections.ts', 'domain/sections/DnsSection.tsx', 'domain/sections/SecuritySection.tsx'].map((file) => source(`components/${file}`)).join('\n');
  for (const [permission, where] of [['mail', "v.view('mail') && v.open('mail')"], ['databases', "v.view('databases') && v.open('databases')"],
    ['cron', "v.view('cron') && v.open('cron')"], ['backups', "v.view('backups') && v.open('backups')"], ['file_manager', "v.open('file_manager')"],
    ['app_installer', "v.open('app_installer')"], ['site_protect_redirects', "v.open('site_protect_redirects')"], ['dns_edit', "readOnly={readOnly || !open('dns_edit')}"],
    ['dnssec', "dnssecHidden={!open('dnssec')}"], ['ssl_custom', "customOpen={open('ssl_custom')}"], ['ssl_acme', "acmeOpen={open('ssl_acme')}"],
    ['file_access', "(v.open('file_access') || v.open('ssh'))"]]) {
    assert.ok(detail.includes(where), `${permission}: ${where}`);
  }
});

test('the rights copy exists in both languages and names no internal codes', () => {
  const enKeys = Object.keys(enScreens).filter((key) => key.startsWith('rights.'));
  const trKeys = Object.keys(trScreens).filter((key) => key.startsWith('rights.'));
  assert.deepEqual([...enKeys].sort(), [...trKeys].sort());
  assert.ok(enKeys.length > 100);
  for (const key of enKeys) {
    for (const text of [enScreens[key], trScreens[key]]) assert.doesNotMatch(text, /D-0\d\d|\bP[1-7]\b|\bK[1-7]\b|offering id|permission:/, key);
  }
  for (const key of ['rights.grantor.administrator', 'rights.grantor.provider']) {
    assert.ok(enShell[key] && trShell[key], `${key} is in the shell: lib/apiError.ts reads it`);
  }
  for (const code of ['RIGHT_NOT_GRANTED', 'NOT_OFFERED', 'RIGHTS_OUTSIDE_GRANTOR', 'RIGHTS_LEAVES_PLAN', 'RIGHTS_CHANGE_CONFIRM', 'PLAN_NARROWING_CONFIRM', 'RESELLER_PLAN_IN_USE']) {
    assert.ok(enScreens[`err.${code}`] && trScreens[`err.${code}`], code);
  }
});
