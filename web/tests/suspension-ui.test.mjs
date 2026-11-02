import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, dataModule, reactURL, remoteURL, sharedLayer } from './fixtures/shared-layer.mjs';

// Subscription suspension in the interface (owner, 2 Nov 2026): the dialogue
// says what will change BEFORE it is confirmed and needs a reason to suspend;
// the work runs on the server and reading its progress never starts anything;
// a lost answer is not sent again; and the owner of a suspended subscription
// sees one plain screen that names who to contact.
const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const icons = new Set(['AlertTriangle', 'XCircle']);
for (const name of ['ui.tsx', 'SuspensionDialog.tsx', 'SuspendedScreen.tsx']) {
  for (const match of source(`components/${name}`).matchAll(/import\s*\{([^}]+)\}\s*from 'lucide-react'/g)) {
    for (const item of match[1].split(',')) {
      const icon = item.replace(/\btype\b/, '').trim().split(/\s+as\s+/)[0];
      if (icon && icon !== 'LucideIcon') icons.add(icon);
    }
  }
}
const stub = dataModule(`
  import React from '${reactURL}';
  ${[...icons].map((name) => `export const ${name} = () => null;`).join('\n')}
  const i18n = { t: (key, vars) => key + (vars ? JSON.stringify(vars) : ''), locale: 'en', screensReady: globalThis.suspensionTest?.screensReady !== false };
  export const useI18n = () => ({ ...i18n, screensReady: globalThis.suspensionTest?.screensReady !== false });
  export const useNavigate = () => () => {};
  export const showToast = (...args) => globalThis.suspensionTest.toasts.push(args);
  export const BrandMark = () => null, LanguageSwitcher = () => null, ThemeSwitcher = () => null;
`);
const shared = sharedLayer(stub);
const suspensionURL = compileSource('lib/suspension.ts', (specifier) => (specifier.endsWith('/remote') ? remoteURL : stub));
const own = { '/lib/suspension': suspensionURL };
const { SuspensionDialog } = await import(shared.compile('components/SuspensionDialog.tsx', own));
const { SuspendedScreen } = await import(shared.compile('components/SuspendedScreen.tsx', own));
const { decodeAccountSuspension, decodeDeletable } = await import(suspensionURL);
const apiURL = compileSource('lib/api.ts', () => stub);
const { parseCurrentUser, parseSuspended } = await import(apiURL);

const preview = (action, extra = {}) => ({
  action, sites: ['shop.example.com'], mail_domains: ['example.com'], mailboxes: 3, database_users: ['shop'], vpn_peers: 1,
  site_accounts: ['shop_ftp'], notes: ['Backups keep running.'], ...extra,
});
const subscription = (extra = {}) => ({
  subscription_id: 5, subscription_name: 'Basic', owner_id: 7, owner: 'ada', status: 'active', causes: { manual: false, expired: false },
  operation: null, preview: preview('suspend'), can_suspend: true, can_resume: false, ...extra,
});
const account = (subscriptions) => ({ user_id: 7, username: 'ada', role: 'customer', reseller_suspended: false, subscriptions });
const operation = (state, result, extra = {}) => ({
  id: 'op1', kind: 'suspend', trigger: 'manual', state, attempts: 1, created_at: '2026-11-02T10:00:00Z',
  guidance: { result, reason: 'The mail server did not answer.', who_acts: 'administrator', next_action: 'Start the mail server, then continue.', resumes: 'The same operation continues.' },
  effects: [{ kind: 'mail', target: 'example.com', state: 'failed', detail: 'timeout' }, { kind: 'web', target: 'shop.example.com', state: 'done' }],
  ...extra,
});

const originalFetch = globalThis.fetch;
let tree, calls, view;
function serve(initial) {
  calls = [];
  view = initial;
  globalThis.suspensionTest = { toasts: [] };
  globalThis.document = { addEventListener() {}, removeEventListener() {} };
  globalThis.fetch = (url, options = {}) => {
    const method = options.method || 'GET';
    calls.push({ method, url, body: options.body });
    if (method === 'POST') return Promise.resolve(Response.json(view, { status: 202 }));
    return Promise.resolve(Response.json(view));
  };
}
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
async function mount(element) {
  await act(async () => { tree = Renderer.create(element); await settled(); await settled(); });
}
async function cleanup() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined; globalThis.fetch = originalFetch; delete globalThis.suspensionTest; delete globalThis.document;
}
const text = () => JSON.stringify(tree.toJSON());
const buttons = () => tree.root.findAll((node) => node.type === 'button');
const confirmButton = () => buttons().find((node) => node.props['data-suspension-confirm']);
const posts = () => calls.filter((call) => call.method === 'POST');

test('the decoders treat anything but the contract as unknown, and a null list as none', () => {
  assert.throws(() => decodeAccountSuspension({ error: 'x' }));
  assert.throws(() => decodeAccountSuspension({ user_id: 1, subscriptions: [{ subscription_id: 1, status: 'bogus' }] }));
  assert.deepEqual(decodeAccountSuspension({ user_id: 1, subscriptions: null }).subscriptions, []);
  assert.throws(() => decodeDeletable({ subscriptions: 'x' }));
  assert.deepEqual(decodeDeletable({ subscriptions: null }), []);
  const [entry] = decodeDeletable({ subscriptions: [{ subscription_id: 3, owner_id: 7, owner: 'ada', causes: { manual: true }, contents: { domains: ['a.com'], sites: 1 }, delete_hint: 'Delete the domains first.' }] });
  assert.equal(entry.contents.mailboxes, 0);
  assert.deepEqual(entry.contents.domains, ['a.com']);
});

test('GET /auth/me: a suspended field keeps the pages away, even a malformed one', () => {
  const me = { username: 'ada', role: 'customer', effective_role: 'customer', account_type: 'account', impersonating: false, features: {} };
  assert.equal(parseCurrentUser(me).suspended, undefined);
  assert.ok(!('suspended' in parseCurrentUser(me)));
  const suspended = parseCurrentUser({ ...me, suspended: { kind: 'subscription', reason: 'Unpaid', since: '2026-11-01T00:00:00Z', contact: { kind: 'reseller', name: 'Acme', email: 'help@acme.test' } } }).suspended;
  assert.deepEqual(suspended.contact, { kind: 'reseller', name: 'Acme', email: 'help@acme.test' });
  assert.equal(suspended.reason, 'Unpaid');
  assert.deepEqual(parseSuspended('garbage'), { kind: 'subscription', reason: undefined, since: undefined, contact: { kind: 'none', name: undefined, email: undefined } });
});

test('the dialogue shows what will change before the confirm, and a reason is required', async () => {
  serve(account([subscription()]));
  try {
    await mount(React.createElement(SuspensionDialog, { target: { kind: 'user', id: 7, name: 'ada' }, onClose() {} }));
    const body = text();
    for (const word of ['shop.example.com', 'example.com', 'shop', 'shop_ftp', 'Backups keep running.', 'susp.preview.suspendHead']) assert.ok(body.includes(word), word);
    assert.ok(confirmButton().props.disabled, 'confirm is on without a reason');
    assert.ok(body.includes('susp.reasonMissing'), 'what keeps confirm off is not said beside it');
    assert.equal(posts().length, 0, 'something was sent before the confirm');
    const field = tree.root.findByType('textarea');
    await act(async () => { field.props.onChange({ target: { value: '  Unpaid invoice  ' } }); });
    assert.ok(!confirmButton().props.disabled);
    await act(async () => { confirmButton().props.onClick(); await settled(); await settled(); });
    assert.equal(posts().length, 1);
    assert.equal(posts()[0].url, '/api/v1/users/7/suspension');
    assert.deepEqual(JSON.parse(posts()[0].body), { action: 'suspend', reason: 'Unpaid invoice' });
  } finally {
    await cleanup();
  }
});

test('a resume needs no reason and a subscription is resumed by its own address', async () => {
  serve(account([subscription({ status: 'suspended', can_suspend: false, can_resume: true, preview: preview('resume') })]));
  try {
    await mount(React.createElement(SuspensionDialog, { target: { kind: 'user', id: 7, name: 'ada' }, onClose() {} }));
    assert.equal(tree.root.findAllByType('textarea').length, 0);
    assert.ok(text().includes('susp.preview.resumeHead'));
    assert.ok(!confirmButton().props.disabled);
  } finally {
    await cleanup();
  }
  serve({ ...subscription({ status: 'suspended', can_suspend: false, can_resume: true, preview: preview('resume') }) });
  try {
    await mount(React.createElement(SuspensionDialog, { target: { kind: 'subscription', id: 5, name: 'Basic' }, onClose() {} }));
    assert.equal(calls[0].url, '/api/v1/subscriptions/5/suspension');
    await act(async () => { confirmButton().props.onClick(); await settled(); await settled(); });
    assert.equal(posts()[0].url, '/api/v1/subscriptions/5/suspension');
    assert.deepEqual(JSON.parse(posts()[0].body), { action: 'resume' });
  } finally {
    await cleanup();
  }
});

test('a failed operation says reason, who acts, what to do and how it continues; retry continues the same operation', async () => {
  serve(account([subscription({ operation: operation('failed', 'verified_failure'), can_suspend: false })]));
  try {
    await mount(React.createElement(SuspensionDialog, { target: { kind: 'user', id: 7, name: 'ada' }, onClose() {} }));
    const body = text();
    for (const word of ['susp.result.verified_failure', 'The mail server did not answer.', 'susp.who.administrator', 'Start the mail server, then continue.', 'The same operation continues.']) assert.ok(body.includes(word), word);
    assert.ok(body.indexOf('The mail server did not answer.') < body.indexOf('susp.effect.kind.mail'), 'the effect list came before the guidance');
    const retry = buttons().find((node) => node.props['data-suspension-retry'] !== undefined);
    await act(async () => { retry.props.onClick(); await settled(); await settled(); });
    assert.equal(posts().length, 1);
    assert.equal(posts()[0].url, '/api/v1/subscriptions/5/suspension');
    assert.deepEqual(JSON.parse(posts()[0].body), { action: 'retry' });
  } finally {
    await cleanup();
  }
});

test('a prerequisite, an unknown result and a running operation are told apart; a read starts nothing', async () => {
  for (const [state, result] of [['failed', 'prerequisite'], ['failed', 'unknown'], ['running', 'in_progress']]) {
    serve(account([subscription({ operation: operation(state, result), can_suspend: false })]));
    try {
      await mount(React.createElement(SuspensionDialog, { target: { kind: 'user', id: 7, name: 'ada' }, onClose() {} }));
      assert.ok(tree.root.findAll((node) => node.props['data-suspension-result'] === result && typeof node.type === 'string').length > 0, result);
      assert.equal(posts().length, 0, `${result}: reading sent a change`);
    } finally {
      await cleanup();
    }
  }
});

test('a lost answer is not sent again', async () => {
  serve(account([subscription()]));
  try {
    await mount(React.createElement(SuspensionDialog, { target: { kind: 'user', id: 7, name: 'ada' }, onClose() {} }));
    const fetchBefore = globalThis.fetch;
    globalThis.fetch = (url, options = {}) => (options.method === 'POST' ? (calls.push({ method: 'POST', url }), Promise.reject(new TypeError('fetch failed'))) : fetchBefore(url, options));
    await act(async () => { tree.root.findByType('textarea').props.onChange({ target: { value: 'Unpaid' } }); });
    await act(async () => { confirmButton().props.onClick(); await settled(); await settled(); });
    assert.equal(posts().length, 1, 'the change was sent again');
    assert.ok(globalThis.suspensionTest.toasts.some(([kind, message]) => kind === 'error' && message.startsWith('common.resultUnknown')));
  } finally {
    await cleanup();
  }
});

test('the suspended owner sees one plain screen naming who to contact, and can sign out', async () => {
  globalThis.suspensionTest = { toasts: [] };
  const signedOut = [];
  globalThis.fetch = (url, options = {}) => { calls = [{ url, method: options.method }]; return Promise.resolve(new Response(null, { status: 204 })); };
  try {
    await mount(React.createElement(SuspendedScreen, {
      username: 'ada', onSignedOut: () => signedOut.push(true),
      suspended: { kind: 'subscription', reason: 'Unpaid invoice', since: '2026-11-01T00:00:00Z', contact: { kind: 'reseller', name: 'Acme Hosting', email: 'help@acme.test' } },
    }));
    const body = text();
    for (const word of ['suspended.title', 'Unpaid invoice', 'Acme Hosting', 'help@acme.test', 'suspended.resumes.text']) assert.ok(body.includes(word), word);
    const out = buttons().find((node) => [node.props.children].flat().includes('user.logout'));
    await act(async () => { out.props.onClick(); await settled(); await settled(); });
    assert.equal(calls[0].url, '/api/v1/auth/logout');
    assert.equal(calls[0].method, 'POST');
    assert.equal(signedOut.length, 1);
  } finally {
    await cleanup();
  }
});

test('with no contact recorded it still says what to do', async () => {
  globalThis.suspensionTest = { toasts: [] };
  try {
    await mount(React.createElement(SuspendedScreen, { username: 'ada', onSignedOut() {}, suspended: { kind: 'reseller', contact: { kind: 'none' } } }));
    const body = text();
    assert.ok(body.includes('suspended.titleReseller') && body.includes('suspended.noReason') && body.includes('suspended.next.none'));
  } finally {
    await cleanup();
  }
});

test('the app reads who the person is again when a call is refused as suspended', () => {
  const app = source('App.tsx');
  assert.match(app, /problem\.code === 'SUBSCRIPTION_SUSPENDED'/);
  assert.match(app, /shown\.suspended \?/);
});
