import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, confirmActionURL, dataModule, reactURL, replaceLazyImport, sharedLayer } from './fixtures/shared-layer.mjs';

// alpha.134: deleting a domain asks about its databases (kept unless the
// box is ticked, the box names them), asks for the name, and the result says
// what stayed and what can be done with it later. Mounted in the real
// ConfirmHost with the shipped question (lib/domainDeletionQuestion.ts).
// Alan adı silme sorusu: veritabanları seçimi, yazılan ad ve kalanlar.

const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null;
  export const HelpCircle = () => null;
  export const CheckCircle2 = () => null;
  export const Clock = () => null;
  export const Loader2 = () => null;
  export const XCircle = () => null;
  export const useI18n = () => ({ t: (key) => key, locale: 'en' });
  export const useNavigate = () => () => {};
  export const captureToasts = () => () => {};
  export const showToast = () => {};
`);
const shared = sharedLayer(stub);
const progressURL = shared.compile('components/OperationProgress.tsx');
let hostURL = replaceLazyImport(shared.compile('components/ConfirmHost.tsx'), './OperationProgress', progressURL);
hostURL = replaceLazyImport(hostURL, './kit/save/ConfirmDetails', shared.resolve('/ConfirmDetails'));
const { ConfirmHost } = await import(hostURL);
const { confirmAction } = await import(confirmActionURL);
const question = await import(compileSource('lib/domainDeletionQuestion.ts', () => stub));

globalThis.window = { setInterval, clearInterval, setTimeout, clearTimeout };
globalThis.document = { activeElement: null, getElementById: () => null, addEventListener() {}, removeEventListener() {} };
const settled = async () => { for (let i = 0; i < 5; i++) await new Promise((resolve) => setTimeout(resolve, 0)); };

// The words with their values, so a test sees what a sentence names.
const t = (key, vars) => (vars ? `${key}(${Object.entries(vars).map(([k, v]) => `${k}=${v}`).join(',')})` : key);
const size = (bytes) => `${bytes}B`;

async function mounted(run) {
  let tree;
  await act(async () => { tree = Renderer.create(React.createElement(ConfirmHost)); });
  try { await run(tree); } finally { await act(async () => tree.unmount()); }
}
const button = (tree, id) => tree.root.findAll((node) => node.type === 'button' && node.props.id === id)[0];
const input = (tree, data) => tree.root.findAll((node) => node.type === 'input' && node.props[data] !== undefined)[0];
const lines = (tree, data) => tree.root.findAll((node) => node.type === 'li' && node.props[data] !== undefined);
const text = (tree) => JSON.stringify(tree.toJSON());

const databases = [{ id: 7, name: 'blog_db' }, { id: 8, name: 'shop_db' }];

test('the databases are kept unless the box naming them is ticked; the name is typed either way', async () => {
  for (const tick of [false, true]) {
    await mounted(async (tree) => {
      let answered;
      let result;
      await act(async () => {
        result = confirmAction({
          title: 'domains.deleteTitle',
          ...question.domainDeleteQuestion(t, databases),
          typedName: { value: 'example.com', label: 'type it', ignoreCase: true },
          tone: 'danger',
          // No answer of its own: the window closes when it ends.
          action: async (_report, answer) => { answered = answer; },
        });
        await settled();
      });
      // Kept unless ticked: the message says so; the box is off at first.
      assert.ok(text(tree).includes('domains.delete.databasesStay(n=2)'), 'the question does not say the databases stay');
      const box = input(tree, 'data-confirm-option');
      assert.ok(box, 'no choice for the databases');
      assert.equal(box.props.checked, false, 'deleting the databases is chosen by default');
      assert.ok(text(tree).includes('domains.delete.databasesOption(n=2)'));
      assert.equal(lines(tree, 'data-confirm-lost').length, 4);
      assert.ok(!text(tree).includes('blog_db, shop_db'), 'the databases are listed as lost before the box is ticked');
      if (tick) {
        await act(async () => { box.props.onChange({ target: { checked: true } }); });
        assert.equal(lines(tree, 'data-confirm-lost').length, 5);
        assert.ok(text(tree).includes('domains.delete.lost.databases(names=blog_db, shop_db)'), 'the ticked box does not name the databases it deletes');
      }
      // The name before Delete turns on.
      assert.equal(button(tree, 'confirm-action-confirm').props.disabled, true);
      await act(async () => { input(tree, 'data-confirm-typed').props.onChange({ target: { value: 'Example.com' } }); });
      assert.equal(button(tree, 'confirm-action-confirm').props.disabled, false);
      await act(async () => { button(tree, 'confirm-action-confirm').props.onClick({}); await settled(); });
      assert.equal(answered.option, tick);
      assert.equal(await result, true);
    });
  }
});

test('without databases there is no choice; unread databases are said to stay', () => {
  const none = question.domainDeleteQuestion(t, []);
  assert.equal(none.option, undefined);
  assert.equal(none.message, 'domains.deleteWhat');
  assert.ok(!none.kept.some((line) => line.includes('databases')));
  const unread = question.domainDeleteQuestion(t, null);
  assert.equal(unread.option, undefined, 'a choice was offered for databases that could not be read');
  assert.equal(unread.kept[0], 'domains.delete.kept.databasesUnread');
  const bulk = question.domainBulkDeleteLists(t);
  assert.equal(bulk.kept[0], 'domains.delete.kept.databasesUnbound');
});

test('the result says what stayed: folders to the administrator, whom to ask to a customer', async () => {
  const response = Response.json({
    status: 'deleted',
    kept: {
      databases: [{ id: 7, name: 'blog_db', engine: 'mariadb' }],
      deleted_databases: [{ id: 8, name: 'shop_db', engine: 'mariadb' }],
      mail: { path: '/var/mail/vhosts/.celikpanel-quarantine/domain-4-ab', bytes: 2048 },
      backups: { path: '/var/backups/celikpanel/subscriptions/1/domains/4', files: 3, bytes: 4096 },
    },
  });
  const kept = await question.readDomainDeletionKept(response);
  assert.deepEqual(kept.databases, ['blog_db']);
  assert.deepEqual(kept.deleted, ['shop_db']);
  // The body is still there for the other readers.
  assert.equal((await response.json()).status, 'deleted');
  const admin = question.domainDeletionKeptSentences(t, kept, true, size);
  assert.deepEqual(admin, [
    'domains.deleted.databasesDeleted(names=shop_db)',
    'domains.deleted.databasesKept(names=blog_db)',
    'domains.deleted.mailKeptAdmin(size=2048B,path=/var/mail/vhosts/.celikpanel-quarantine/domain-4-ab)',
    'domains.deleted.backupsKeptAdmin(n=3,size=4096B,path=/var/backups/celikpanel/subscriptions/1/domains/4)',
  ]);
  const customer = question.domainDeletionKeptSentences(t, kept, false, size);
  assert.ok(customer.every((sentence) => !sentence.includes('/var/')), 'a customer is shown a server folder');
  assert.ok(customer.includes('domains.deleted.mailKept(size=2048B)'));
  // Nothing said, nothing shown; a malformed answer is not read as kept.
  assert.equal(await question.readDomainDeletionKept(Response.json({ status: 'deleted' })), null);
  assert.equal(await question.readDomainDeletionKept(Response.json({ kept: { mail: { path: 'relative', bytes: 1 } } })), null);
  // A size that could not be read is said as such.
  const unknownSize = question.domainDeletionKeptSentences(t, { databases: [], deleted: [], mail: { path: '/m', bytes: -1 }, backups: null }, true, size);
  assert.deepEqual(unknownSize, ['domains.deleted.mailKeptAdmin(size=domains.deleted.sizeUnknown,path=/m)']);
});

test('the result is shown at the end of the deletion window', async () => {
  await mounted(async (tree) => {
    await act(async () => {
      void confirmAction({
        title: 'domains.deleteTitle',
        ...question.domainDeleteQuestion(t, []),
        tone: 'danger',
        action: async () => ({ state: 'done', message: ['domains.deleted', ...question.domainDeletionKeptSentences(t, { databases: ['blog_db'], deleted: [], mail: null, backups: null }, true, size)].join(' ') }),
      });
      await settled();
    });
    await act(async () => { button(tree, 'confirm-action-confirm').props.onClick({}); await settled(); });
    const end = tree.root.findAll((node) => node.props['data-operation-end'] === 'done')[0];
    assert.ok(end, 'the end of the window is not shown');
    assert.ok(JSON.stringify(end.children).includes('domains.deleted.databasesKept(names=blog_db)'), 'the end does not say which databases stayed');
  });
});

test('a database list that is not the contract is not offered as a choice', async () => {
  const saved = globalThis.fetch;
  try {
    globalThis.fetch = async () => Response.json({ databases: [{ id: 'x', name: 3 }] });
    assert.equal(await question.readDomainDatabasesForDeletion('/api/v1', 4), null);
    globalThis.fetch = async () => Response.json({ error: 'no' }, { status: 403 });
    assert.equal(await question.readDomainDatabasesForDeletion('/api/v1', 4), null);
    globalThis.fetch = async (url) => {
      assert.equal(url, '/api/v1/domains/4/databases');
      return Response.json({ databases: [{ id: 2, name: 'shop_db' }, { id: 1, name: 'blog_db' }] });
    };
    assert.deepEqual(await question.readDomainDatabasesForDeletion('/api/v1', 4), [{ id: 1, name: 'blog_db' }, { id: 2, name: 'shop_db' }]);
  } finally {
    globalThis.fetch = saved;
  }
});
