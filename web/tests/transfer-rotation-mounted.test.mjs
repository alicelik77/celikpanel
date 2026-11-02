// Transfer credential rotation, mounted (owner decision, 2 November 2026).
// The transfer dialog offers "Change all passwords and keys" checked, warns
// when it is unticked and sends the choice; the new owner sees a stopped
// change with its guidance first, the steps still to do and Retry, and what
// to set afterwards. The shared layer (remote, apiError, ui) is the shipped
// one; icons, words and toasts are stubs.
//
// Devirde parola ve anahtar yenileme, bağlanmış ekranlar.
import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { apiErrorURL, compileSource, dataModule, reactURL, remoteURL, requestIdentityURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { englishScreens, turkishScreens } from './locale-catalogue.mjs';

const icons = ['AlertTriangle', 'XCircle', 'ArrowRightLeft', 'Check', 'KeyRound', 'RotateCw', 'Send', 'X'];
const stub = dataModule(`
  import React from '${reactURL}';
  ${icons.map((name) => `export const ${name} = () => null;`).join('\n')}
  export const useI18n = () => ({ t: (key) => key, locale: 'en' });
  export const useNavigate = () => () => {};
  export const useAuth = () => ({ role: globalThis.currentTest.role, user: { account_type: 'account' } });
  export const showToast = (...args) => globalThis.currentTest.toasts.push(args);
`);
const componentNameURL = compileSource('lib/componentName.ts', () => stub);
const own = {
  '/lib/transfers': compileSource('lib/transfers.ts', (s) => (s.endsWith('/componentName') ? componentNameURL : s.endsWith('/apiError') ? apiErrorURL : stub)),
  '/lib/subscriptions': compileSource('lib/subscriptions.ts', (s) => (s.endsWith('/remote') ? remoteURL : stub)),
  '/lib/myPlan': compileSource('lib/myPlan.ts', () => stub),
  '/lib/requestIdentity': requestIdentityURL,
};
const shared = sharedLayer(stub, own);
const screen = await import(shared.compile('components/SubscriptionTransfer.tsx', own));

let tree;
let calls = [];
const originalFetch = globalThis.fetch;
function setup(role, answers, changes = {}) {
  calls = [];
  globalThis.currentTest = { toasts: [], role };
  // The dialog listens for Escape and keeps the focus (ui.tsx).
  globalThis.document = { addEventListener() {}, removeEventListener() {}, activeElement: null, body: {} };
  globalThis.fetch = (url, options = {}) => {
    const method = options.method || 'GET';
    calls.push({ method, url, body: options.body });
    const table = method === 'GET' ? answers : changes;
    const answer = table[String(url).split('?')[0]];
    if (!answer) return Promise.resolve(Response.json({ error: 'no such route in this test' }, { status: 404 }));
    return Promise.resolve().then(() => answer(options));
  };
}
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
async function mount(Component, props) {
  await act(async () => {
    tree = Renderer.create(React.createElement(Component, props));
    await settled();
    await settled();
  });
}
async function cleanup() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  delete globalThis.currentTest;
  delete globalThis.document;
}
const text = () => JSON.stringify(tree.toJSON());
const textOf = (node) => (typeof node === 'string' ? node : (node.children ?? []).map(textOf).join(' '));
const button = (key) => tree.root.findAll((node) => node.type === 'button').find((node) => textOf(node).includes(key));
const press = async (node) => {
  await act(async () => {
    await node.props.onClick({});
    await settled();
    await settled();
  });
};

const preview = {
  subscription_id: 7, subscription_name: 'shop', from: { id: 3, username: 'ali' }, to: { id: 2, username: 'ayse' },
  from_reseller: null, to_reseller: null, same_tree: true, rights_from: 'unlimited', rights_to: 'unlimited',
  plan_converted: false, moves: { domains: 1, pending_vpn_devices: 0 }, host_changes: 'credentials', rotate_credentials: true,
  rotation: {
    site_accounts: ['shop_example'], vpn_devices: ['phone'], mailboxes: ['box@shop.example'],
    database_users: [{ username: 'wp_user', app_domain: 'shop.example' }, { username: 'far', host: 'db.example.net' }],
    protected_users: [], legacy_ftp_accounts: 0, total: 5, after: ['mail_clients', 'file_access'], not_covered: ['app_admin_accounts'],
  },
};
const request = {
  id: 5, subscription_id: 7, subscription_name: 'shop', status: 'requested', from: { id: 3, username: 'ali' }, to: null,
  requested_to: 'ayse', note: '', approver: 'reseller', decision_reason: '', created_at: '', receiver_seen: false, requester_seen: false, is_request: true,
};

test('the transfer offers "Change all passwords and keys" checked, warns when unticked and sends the choice', async () => {
  setup('reseller', {
    '/api/v1/subscriptions/7/transfer/receivers': () => Response.json({ receivers: [{ id: 2, username: 'ayse', role: 'customer' }] }),
  }, {
    '/api/v1/transfer-requests/5/preview': () => Response.json({ preview, can_transfer: true }),
    '/api/v1/transfer-requests/5/approve': () => Response.json({ success: true }),
  });
  try {
    await mount(screen.TransferDialog, { owner: { id: 3, username: 'ali' }, request, onClose() {} });
    await press(button('transfer.review'));
    const box = tree.root.find((node) => node.type === 'input' && node.props['data-transfer-rotate'] !== undefined);
    assert.equal(box.props.checked, true, 'checked by default');
    for (const key of ['transfer.rotate.title', 'transfer.rotate.siteAccounts', 'transfer.rotate.dbApp', 'transfer.rotate.dbRemote',
      'transfer.rotate.after.mail_clients', 'transfer.rotate.notCovered.app_admin_accounts']) {
      assert.ok(text().includes(key), `the preview names ${key}`);
    }
    assert.ok(!text().includes('transfer.rotate.offWarning'));
    await act(async () => { box.props.onChange({ target: { checked: false } }); await settled(); });
    assert.ok(text().includes('transfer.rotate.offWarning'), 'unticking shows the warning');
    assert.ok(text().includes('transfer.credentials'), 'and says the old credentials keep working');
    await press(button('transfer.confirm'));
    const approve = calls.find((call) => call.url === '/api/v1/transfer-requests/5/approve');
    assert.equal(JSON.parse(approve.body).rotate_credentials, false);
  } finally {
    await cleanup();
  }
});

test('the new owner sees a stopped change with guidance first, what is still to do, and Retry', async () => {
  const rotation = {
    id: 'a'.repeat(32), state: 'failed', attempts: 1, created_at: '',
    guidance: { result: 'prerequisite', reason: 'site_account shop.example: the sshd drop-in was changed by hand', who_acts: 'approver_or_new_owner', next_action: 'x', resumes: 'y' },
    counts: { total: 4, done: 1, pending: 2, failed: 1, manual: 0, replaced: 0 },
    effects: [
      { kind: 'vpn', target: 'wireguard', state: 'done' },
      { kind: 'site_account', target: 'shop_example', state: 'failed', detail: 'the sshd drop-in was changed by hand' },
      { kind: 'mailbox', target: 'box@shop.example', state: 'pending' },
    ],
  };
  const row = {
    ...request, id: 9, status: 'completed', to: { id: 2, username: 'ayse' }, to_you: true, receiver_seen: true,
    rotation, result: { rotate_credentials: true, rotation: preview.rotation },
  };
  setup('customer', {
    '/api/v1/transfer-requests': () => Response.json({ requests: [row], unseen: { received: 0, decided: 0 }, waiting: 0 }),
    '/api/v1/my/subscription': () => Response.json({ error: 'not in this test' }, { status: 500 }),
  }, {
    '/api/v1/transfer-requests/9/rotation-retry': () => Response.json({ success: true, started: true }, { status: 202 }),
  });
  try {
    await mount(screen.TransferRequestCard, {});
    const shown = text();
    // Guidance before the list: what happened, who acts, what next.
    const order = ['transfer.rotation.result.prerequisite', 'transfer.rotation.label.who', 'transfer.rotation.label.next', 'transfer.rotation.stillToDo'];
    const at = order.map((key) => shown.indexOf(key));
    assert.ok(at.every((index) => index >= 0) && at.every((index, i) => i === 0 || index > at[i - 1]), `order ${at}`);
    assert.ok(shown.includes('the sshd drop-in was changed by hand'));
    assert.ok(shown.includes('transfer.rotation.setYours') && shown.includes('transfer.rotate.after.file_access'));
    assert.equal(button('transfer.seen'), undefined, 'a stopped change cannot be dismissed');
    await press(button('transfer.rotation.retry'));
    assert.ok(calls.some((call) => call.method === 'POST' && call.url === '/api/v1/transfer-requests/9/rotation-retry'));
  } finally {
    await cleanup();
  }
});

test('every word the rotation screens use exists in English and Turkish', () => {
  const keys = [
    'transfer.rotate.label', 'transfer.rotate.hint', 'transfer.rotate.offWarning', 'transfer.rotate.title', 'transfer.rotate.none',
    ...['mail_clients', 'database_apps', 'file_access', 'vpn_devices', 'protected_folders'].map((c) => `transfer.rotate.after.${c}`),
    ...['app_admin_accounts', 'own_app_secrets', 'legacy_ftp_accounts'].map((c) => `transfer.rotate.notCovered.${c}`),
    ...['in_progress', 'done', 'prerequisite', 'verified_failure', 'unknown'].map((c) => `transfer.rotation.result.${c}`),
    ...['none', 'panel', 'approver_or_new_owner', 'new_owner', 'administrator'].map((c) => `transfer.rotation.who.${c}`),
    ...['site_account', 'vpn', 'mailbox', 'database_user', 'protected_dir'].map((c) => `transfer.rotation.kind.${c}`),
    ...['pending', 'running', 'done', 'failed', 'manual', 'replaced'].map((c) => `transfer.rotation.state.${c}`),
  ];
  for (const key of keys) {
    assert.ok(englishScreens.includes(`'${key}':`), `no English words for ${key}`);
    assert.ok(turkishScreens.includes(`'${key}':`), `no Turkish words for ${key}`);
  }
});
