import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { apiErrorURL, compileSource, dataModule, reactURL, remoteURL, replaceLazyImport, sharedLayer } from './fixtures/shared-layer.mjs';

// alpha.133 (owner's real-system check of alpha.132): in a plan's limits,
// "Extra FTP/SFTP logins per subscription" set to 999999 showed its red line
// but Save stayed on, and the server refused. A limit outside its range, or
// above the level above, is now named beside Save, and Save stays off until it
// is corrected: in the plan form (service and reseller plans) and in an
// account's own Custom set, as the site's PHP limits do. A notice level is
// kept as typed (it was forced to 50-95 under the typing) and is held the same
// way. Mounted with the real limits part.
//
// Planın sınırlarında aralık dışındaki değer Kaydet'in yanında adıyla söylenir
// ve düzeltilene dek Kaydet kapalı kalır; hesaba özel kümede de.

const icons = ['AlertTriangle', 'Check', 'Info', 'X', 'Lock', 'Copy', 'Layers', 'Pencil', 'Plus', 'Trash2', 'Save', 'XCircle', 'Loader2'];
const stub = dataModule(`
  import React from '${reactURL}';
  ${icons.map((name) => `export const ${name} = () => null;`).join('\n')}
  export const useI18n = () => ({ locale: 'en', t: (key, vars) => (vars ? key + JSON.stringify(vars) : key) });
  export const useNavigate = () => () => true;
  export const showToast = () => {};
  export const unitText = (t, unit, value) => String(value), whoText = () => '';
`);
const shared = sharedLayer(stub);
const rightsURL = compileSource('lib/rights.ts', (specifier) => {
  if (specifier.endsWith('/apiError')) return apiErrorURL;
  throw new Error(`lib/rights.ts imports ${specifier}`);
});
const rights = await import(rightsURL);
const own = {
  '/lib/rights': rightsURL,
  '/lib/accounts': compileSource('lib/accounts.ts', (specifier) => (specifier.endsWith('/remote') ? remoteURL : shared.resolve(specifier))),
  '/lib/appCatalog': compileSource('lib/appCatalog.ts', (specifier) => shared.resolve(specifier)),
};
own['/BlockingProblems'] = shared.compile('components/BlockingProblems.tsx', own);
own['/RightsSetEditor'] = shared.compile('components/RightsSetEditor.tsx', own);
// The limits part is its own chunk; the fixture gives screens a quiet one. Here it is the real one.
const quietLimits = dataModule('export const PlanLimitsSection = () => null; export const CustomLimitsSection = () => null;');
const limitsURL = shared.compile('components/PlanLimitsSection.tsx', own);
own['/AccountRights'] = replaceLazyImport(shared.compile('components/AccountRights.tsx', own), quietLimits, limitsURL);
const { PlanEditor } = await import(replaceLazyImport(shared.compile('components/PlanRights.tsx', own), quietLimits, limitsURL));
const { RightsChangeForm } = await import(own['/AccountRights']);

const answer = (status, body) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
const settle = async () => { for (let i = 0; i < 8; i++) await act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)); }); };
const textOf = (node) => (node === null || node === undefined || typeof node === 'boolean' ? '' : typeof node === 'string' || typeof node === 'number' ? String(node) : Array.isArray(node) ? node.map(textOf).join(' ') : textOf(node.children));

// The administrator's level: at most 1000 extra logins, no ceiling above.
const limits = { scope: 'administrator', limits: [
  { key: 'ftp.max_logins', unit: 'count', ceiling: null, min: 0, max: 1000 },
  { key: 'php.memory_limit_mb', unit: 'mb', ceiling: 512, min: 32, max: 4096 },
] };

async function mount(element) {
  const calls = [];
  const original = globalThis.fetch;
  globalThis.fetch = async (url, init = {}) => {
    const call = { url: String(url), method: init.method ?? 'GET', body: init.body ? JSON.parse(init.body) : undefined };
    calls.push(call);
    if (call.url.startsWith('/api/v1/rights/setting-limits')) return answer(200, limits);
    return answer(200, { success: true });
  };
  let tree;
  await act(async () => { tree = Renderer.create(element); });
  await settle();
  const input = (id) => tree.root.find((node) => node.type === 'input' && node.props.id === id);
  return {
    tree, calls, input,
    writes: () => calls.filter((call) => call.method !== 'GET'),
    text: () => textOf(tree.toJSON()),
    save: () => tree.root.findAll((node) => node.type === 'button' && node.props['data-save'] !== undefined)[0],
    type: async (id, value) => { await act(async () => input(id).props.onChange({ target: { value } })); await settle(); },
    press: async (node) => { await act(async () => { await node.props.onClick({}); }); await settle(); },
    done: async () => { await act(async () => tree.unmount()); globalThis.fetch = original; },
  };
}

const permissions = rights.PERMISSION_IDS.map((id) => ({ id, granted: true }));
const grantor = { subscription_id: 0, state: 'unlimited', unrestricted: false, own: { all_permissions: true, all_software: true, permissions: [], software: [] }, permissions, software: [] };

test('a plan limit above its range is named beside Save, which stays off until it is corrected', async () => {
  const initial = rights.editedFromCustomerPlan({ id: 4, name: 'Basic', max_domains: 3, max_databases: 1, max_email_accounts: 2, disk_quota_mb: 100, bandwidth_quota_mb: 0, rights: { all_permissions: true, all_software: true, permissions: [], software: [] } });
  const view = await mount(React.createElement(PlanEditor, { kind: 'customer', initial, grantor, grantorIsReseller: false, onClose: () => {} }));
  try {
    const ftp = 'plan-customer-4-ceiling-ftp-max_logins';
    await view.type(ftp, '999999');
    assert.equal(view.input(ftp).props['aria-invalid'], true, 'the field is not marked');
    assert.ok(view.text().includes('limits.plan.outside{"name":"limits.key.ftp.max_logins"}'), `the field is not named beside Save: ${view.text()}`);
    assert.equal(view.save().props.disabled, true, 'Save is on with a value outside its range');
    assert.equal(view.writes().length, 0, 'typing sent something');
    // Above the level above (512 MB) is held the same way.
    await view.type(ftp, '20');
    await view.type('plan-customer-4-ceiling-php-memory_limit_mb', '1024');
    assert.ok(view.text().includes('limits.plan.outside{"name":"limits.key.php.memory_limit_mb"}'), view.text());
    assert.equal(view.save().props.disabled, true);
    await view.type('plan-customer-4-ceiling-php-memory_limit_mb', '256');
    assert.ok(!view.text().includes('limits.plan.outside'), 'the reason stayed after the value was corrected');
    assert.equal(view.save().props.disabled, false, 'Save stayed off after the value was corrected');
    await view.press(view.save());
    assert.equal(view.writes().length, 1);
    assert.deepEqual(view.writes()[0].body.ceilings, { 'ftp.max_logins': 20, 'php.memory_limit_mb': 256 });
  } finally {
    await view.done();
  }
});

test('a notice level keeps what is typed; outside 50-95 it holds Save the same way', async () => {
  const initial = rights.editedFromCustomerPlan({ id: 4, name: 'Basic', max_domains: 3, max_databases: 1, max_email_accounts: 2, disk_quota_mb: 100, bandwidth_quota_mb: 0, notify_percent_disk: 80, rights: { all_permissions: true, all_software: true, permissions: [], software: [] } });
  const view = await mount(React.createElement(PlanEditor, { kind: 'customer', initial, grantor, grantorIsReseller: false, onClose: () => {} }));
  try {
    const disk = 'plan-customer-4-notify_percent_disk';
    await view.type(disk, '8');
    assert.equal(view.input(disk).props.value, 8, 'the typed number was changed under the typing');
    assert.ok(view.text().includes('limits.plan.outside{"name":"limits.plan.notifyDisk"}'), view.text());
    assert.equal(view.save().props.disabled, true);
    await view.type(disk, '85');
    assert.equal(view.save().props.disabled, false);
    await view.press(view.save());
    assert.equal(view.writes()[0].body.notify_percent_disk, 85);
  } finally {
    await view.done();
  }
});

test('an account\'s own Custom set: a limit outside its range is named and the review button stays off', async () => {
  const current = { subscription_id: 9, state: 'custom', unrestricted: false, own: { all_permissions: true, all_software: true, permissions: [], software: [] }, permissions, software: [] };
  const numbers = { max_customers: 0, max_domains: 3, max_databases: 1, max_email_accounts: 2, disk_quota_mb: 100, traffic_quota_mb: 0 };
  const view = await mount(React.createElement(RightsChangeForm, {
    target: { kind: 'subscription', url: '/api/v1/subscriptions/9/rights', current, numbers },
    grantor, plans: [], grantorIsReseller: false, onDone: () => {},
  }));
  try {
    const review = () => view.tree.root.find((node) => node.type === 'button' && textOf(node).includes('rights.review'));
    assert.equal(review().props.disabled, false);
    const ftp = 'rights-subscription-9-ceiling-ftp-max_logins';
    await view.type(ftp, '999999');
    assert.ok(view.text().includes('limits.plan.outside{"name":"limits.key.ftp.max_logins"}'), view.text());
    const reason = view.tree.root.find((node) => node.type === 'p' && node.props['data-blocked-reason'] === ftp);
    assert.ok(reason, 'the reason does not lead to the field');
    assert.equal(review().props.disabled, true, 'the review button is on with a value outside its range');
    await view.type(ftp, '5');
    assert.equal(review().props.disabled, false);
    assert.equal(view.writes().length, 0, 'typing sent something');
  } finally {
    await view.done();
  }
});
