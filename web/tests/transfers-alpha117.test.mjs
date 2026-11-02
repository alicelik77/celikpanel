// Subscription transfer (alpha.117, D-042 section 8): the decoders and item
// names of src/lib/transfers.ts, the words for every refusal code the Panel
// writes, and where the screens mount the transfer.
//
// Abonelik devri: çözücüler, öğe adları, her ret kodunun sözcükleri ve
// ekranların devri nereye yerleştirdiği.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';
import { englishScreens, turkishScreens } from './locale-catalogue.mjs';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const moduleURL = (source) => 'data:text/javascript;base64,' + Buffer.from(source).toString('base64');
const compile = (path) => ts.transpileModule(read(path), { compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 } }).outputText;
const nameURL = moduleURL(compile('../src/lib/componentName.ts'));
const errorURL = moduleURL(compile('../src/lib/apiError.ts'));
const lib = await import(moduleURL(compile('../src/lib/transfers.ts')
  .replace("from './componentName'", `from '${nameURL}'`)
  .replace("from './apiError'", `from '${errorURL}'`)));

const preview = {
  subscription_id: 7, subscription_name: 'shop', from: { id: 1, username: 'ali' }, to: { id: 2, username: 'ayse' },
  from_reseller: null, to_reseller: { id: 3, username: 'bayi' }, same_tree: false,
  rights_from: 'unlimited', rights_to: 'unlimited', plan_converted: false,
  capped: ['permission:ssh'], moves: { domains: 1 }, host_changes: 'none',
};

test('a preview answer is decoded only when it is the contract', () => {
  const answer = lib.decodePreviewAnswer({ preview, can_transfer: true });
  assert.equal(answer.canTransfer, true);
  assert.deepEqual(answer.preview.capped, ['permission:ssh']);
  assert.deepEqual(answer.preview.grants_removed, [], 'a missing list is empty, not undefined');
  const refused = lib.decodePreviewAnswer({ preview, can_transfer: false, refusal: { code: 'TRANSFER_POOL_EXCEEDED', message: 'x', details: ['domains'] } });
  assert.equal(refused.canTransfer, false);
  assert.equal(refused.refusal.code, 'TRANSFER_POOL_EXCEEDED');
  assert.throws(() => lib.decodePreviewAnswer({ preview: { subscription_id: 7 } }));
  assert.throws(() => lib.decodeTransferList({ requests: 'no' }));
  assert.deepEqual(lib.decodeTransferList({ requests: [], unseen: { received: 1 }, waiting: 2 }).unseen, { received: 1, decided: 0 });
  assert.throws(() => lib.decodeReceivers({}));
});

test('a refused confirmed call keeps its fresh preview, and a body that is not the contract has none', async () => {
  const withPreview = new Response(JSON.stringify({ error: 'no', code: 'TRANSFER_STALE', preview }), { status: 409 });
  const answer = await lib.readTransferRefusal(withPreview);
  assert.equal(answer.error.code, 'TRANSFER_STALE');
  assert.equal(answer.preview.subscription_id, 7);
  const broken = await lib.readTransferRefusal(new Response('not json', { status: 500 }));
  assert.equal(broken.preview, null);
});

test('rights items are named, never shown as ids when there are words', () => {
  const t = (key, vars = {}) => {
    const words = { 'transfer.permission.ssh': 'SSH (command line)', 'transfer.item.acme': 'Certificates from {provider}' };
    let text = words[key] ?? key;
    for (const [name, value] of Object.entries(vars)) text = text.replaceAll(`{${name}}`, String(value));
    return text;
  };
  assert.equal(lib.rightsItemLabel('permission:ssh', t), 'SSH (command line)');
  assert.equal(lib.rightsItemLabel('component:php-fpm:8.3', t), 'PHP 8.3');
  assert.equal(lib.rightsItemLabel('component:node:22', t), 'Node.js 22');
  assert.equal(lib.rightsItemLabel('integration:acme:letsencrypt', t), 'Certificates from letsencrypt');
});

test('every transfer refusal code the Panel writes has English and Turkish words', () => {
  const go = read('../../cmd/panel/transfer.go');
  const codes = [...go.matchAll(/errCodeTransfer\w+\s*=\s*"([A-Z_]+)"/g)].map((m) => m[1]);
  assert.ok(codes.length >= 12, `read ${codes.length} codes`);
  for (const code of codes) {
    assert.match(englishScreens, new RegExp(`'err\\.${code}':`), `no English words for ${code}`);
    assert.match(turkishScreens, new RegExp(`'err\\.${code}':`), `no Turkish words for ${code}`);
  }
  for (const permission of [...read('../../cmd/panel/rights_model.go').matchAll(/\bright[A-Z]\w*\s*=\s*"([a-z_]+)"/g)].map((m) => m[1])) {
    assert.match(englishScreens, new RegExp(`'transfer\\.permission\\.${permission}':`), `no words for permission ${permission}`);
  }
});

test('the accounts page offers the transfer and the requests; My account offers a customer the request', () => {
  const users = read('../src/components/UsersPage.tsx');
  assert.match(users, /<TransferRowAction user=\{u\}/);
  assert.match(users, /<TransferRequestsList \/>/);
  assert.match(read('../src/components/AccountPage.tsx'), /role === 'customer' && <TransferRequestCard \/>/);
  const screen = read('../src/components/SubscriptionTransfer.tsx');
  // The confirmed call carries the reviewed owner and one request id.
  assert.match(screen, /from_user_id: preview\.from\.id, request_id: pending\.current\.id, confirm: true/);
});
