import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { confirmActionURL, dataModule, domainDeletionQuestionURL, reactURL, replaceLazyImport, sharedLayer } from './fixtures/shared-layer.mjs';

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
const question = await import(domainDeletionQuestionURL);

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

// blog_user is shared with a database of another domain (it stays, losing
// only its rights on blog_db); shop_user goes with shop_db.
const databases = [
  { id: 7, name: 'blog_db', users: [{ id: 70, username: 'blog_user', access: 'owner', stays: 'shared' }] },
  { id: 8, name: 'shop_db', users: [{ id: 80, username: 'shop_user', access: 'owner' }] },
];

test('the databases are kept unless the box is ticked; ticked, each database and its users are said as the Databases page says them', async () => {
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
      // The choice comes first, under the heading and above the lists; the
      // sentence under it says what happens and changes with it (alpha.135).
      const json = text(tree);
      assert.ok(json.indexOf('domains.delete.databasesOption(n=2)') < json.indexOf('domains.delete.lost.site'), 'the choice is not above its consequences');
      assert.ok(json.includes('domains.delete.databasesStay(n=2)') && !json.includes('domains.delete.databasesGo'), 'the unticked box does not say the databases stay');
      const box = input(tree, 'data-confirm-option');
      assert.ok(box, 'no choice for the databases');
      assert.equal(box.props.checked, false, 'deleting the databases is chosen by default');
      assert.ok(text(tree).includes('domains.delete.databasesOption(n=2)'));
      assert.equal(lines(tree, 'data-confirm-lost').length, 4);
      assert.equal(lines(tree, 'data-confirm-kept').length, 3);
      assert.ok(!text(tree).includes('databases.delete.lostData'), 'a database is listed as lost before the box is ticked');
      if (tick) {
        await act(async () => { box.props.onChange({ target: { checked: true } }); });
        assert.ok(text(tree).includes('domains.delete.databasesGo(n=2)') && !text(tree).includes('domains.delete.databasesStay'), 'the ticked box still says the databases stay')
        // Each database's data, the users that go with it, and the user that stays.
        assert.equal(lines(tree, 'data-confirm-lost').length, 7);
        for (const line of ['databases.delete.lostData(name=blog_db)', 'databases.delete.lostData(name=shop_db)', 'databases.delete.lostUsers(users=shop_user)']) {
          assert.ok(text(tree).includes(line), `the ticked box does not say ${line}`);
        }
        assert.equal(lines(tree, 'data-confirm-kept').length, 4);
        assert.ok(text(tree).includes('databases.delete.keptShared(users=blog_user,name=blog_db)'), 'the shared user is not said to stay');
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

test('no choice without databases, for unread databases, for users that cannot be described, or on a retry', () => {
  const none = question.domainDeleteQuestion(t, []);
  assert.equal(none.option, undefined);
  assert.equal(none.message, 'domains.deleteWhat');
  assert.ok(!none.kept.some((line) => line.includes('databases')));
  const unread = question.domainDeleteQuestion(t, null);
  assert.equal(unread.option, undefined, 'a choice was offered for databases that could not be read');
  assert.equal(unread.kept[0], 'domains.delete.kept.databasesUnread');
  // The Databases page offers no deletion whose users it cannot describe.
  const unknown = question.domainDeleteQuestion(t, [{ id: 7, name: 'blog_db', users: [{ id: 70, username: 'blog_user', access: 'owner', stays: 'unknown' }] }]);
  assert.equal(unknown.option, undefined, 'a deletion was offered whose users could not be read');
  assert.equal(unknown.kept[0], 'domains.delete.kept.databasesUsersUnknown(n=1)');
  // A deletion that stopped repeats its accepted choice, says it and offers none.
  for (const accepted of ['keep', 'delete']) {
    const retry = question.domainDeleteQuestion(t, databases, accepted);
    assert.equal(retry.option, undefined, `a retry (${accepted}) offered a choice`);
    assert.equal(retry.message, `domains.deleteWhat domains.delete.${accepted === 'delete' ? 'retryDelete' : 'retryKeep'}`);
  }
  const bulk = question.domainBulkDeleteLists(t);
  assert.equal(bulk.kept[0], 'domains.delete.kept.databasesUnbound');
});

const date = (iso) => `D(${iso.slice(0, 10)})`;

test('the result: rows of what stayed with size and date, folders only under the administrator’s Details', async () => {
  const response = Response.json({
    status: 'deleted',
    kept: {
      choice: 'delete',
      deleted_databases: [
        { id: 8, name: 'shop_db', engine: 'mariadb', users_removed: ['shop_user'] },
        { id: 9, name: 'old_db', engine: 'mariadb', users_kept_on_engine: ['old_user'] },
      ],
      mail: { path: '/var/mail/vhosts/.celikpanel-quarantine/domain-4-0123456789abcdef', bytes: 2048, remove_after: '2026-12-14T10:00:00Z' },
      backups: { path: '/var/backups/celikpanel/subscriptions/1/domains/4', files: 3, bytes: 4096, remove_after: '2026-12-14T10:00:00Z' },
    },
  });
  const kept = await question.readDomainDeletionKept(response);
  assert.equal(kept.choice, 'delete');
  assert.deepEqual(kept.deleted.map((item) => item.name), ['shop_db', 'old_db']);
  // The body is still there for the other readers.
  assert.equal((await response.json()).status, 'deleted');
  assert.equal(question.domainDeletionDatabasesDone(t, kept), 'domains.step.done.databasesDeleted(n=2)');
  const admin = question.domainDeletionEnd(t, kept, true, size, date);
  assert.equal(admin.message, 'domains.deleted.databasesDeleted(names=shop_db, old_db). databases.keptNote.otherRights(name=old_db,users=old_user).');
  assert.deepEqual(admin.kept, { title: 'domains.deleted.keptTitle', rows: [
    'domains.deleted.row.mail(size=2048B,date=D(2026-12-14))',
    'domains.deleted.row.backups(n=3,size=4096B,date=D(2026-12-14))',
    'domains.deleted.row.holdingAdmin(size=6144B)',
  ] });
  assert.deepEqual(admin.details.lines, [
    'domains.deleted.detail.mail(path=/var/mail/vhosts/.celikpanel-quarantine/domain-4-0123456789abcdef)',
    'domains.deleted.detail.backups(path=/var/backups/celikpanel/subscriptions/1/domains/4)',
  ]);
  for (const visible of [admin.message, ...admin.kept.rows]) {
    assert.ok(!visible.includes('/var/') && !visible.includes('sudo'), `a folder or a command in the visible text: ${visible}`);
  }
  // A customer: no folder anywhere (the server sends none), whom to ask.
  const customerKept = { ...kept, mail: { ...kept.mail, path: '' }, backups: { ...kept.backups, path: '' } };
  const customer = question.domainDeletionEnd(t, customerKept, false, size, date);
  assert.equal(customer.details, undefined, 'a customer has the administrator’s Details');
  assert.equal(customer.kept.rows.at(-1), 'domains.deleted.row.askProvider(date=D(2026-12-14))');
  // Databases kept: their row says where they are; the step says how many.
  const keptOnes = { choice: 'keep', databases: ['blog_db', 'shop_db'], deleted: [], databaseFailure: null, mail: null, backups: null };
  assert.equal(question.domainDeletionDatabasesDone(t, keptOnes), 'domains.step.done.databasesKept(n=2)');
  assert.deepEqual(question.domainDeletionEnd(t, keptOnes, true, size, date).kept.rows, ['domains.deleted.row.databases(names=blog_db, shop_db)']);
  assert.equal(question.domainDeletionDatabasesDone(t, null), 'domains.step.done.noDatabases');
  // Nothing said, nothing shown; a held item without its date is not read.
  assert.equal(await question.readDomainDeletionKept(Response.json({ status: 'deleted' })), null);
  assert.equal(await question.readDomainDeletionKept(Response.json({ kept: { mail: { bytes: 1 } } })), null);
});

test('a database deletion that did not finish is said with the Databases page sentence', async () => {
  const kept = await question.readDomainDeletionKept(Response.json({
    status: 'deletion_pending', stage: 'database_cleanup', reason: 'database_cleanup_failed',
    kept: {
      choice: 'delete',
      database_failure: {
        name: 'blog_db', status: 409,
        answer: { error: 'The database blog_db was deleted, but ...', code: 'DATABASE_DELETE_USERS_LEFT', reason: 'rights', partial_success: true, mutation_applied: true, vars: { name: 'blog_db', users: 'blog_user', pending: '', rights: 'blog_user' } },
      },
    },
  }, { status: 202 }));
  assert.equal(kept.databaseFailure.code, 'DATABASE_DELETE_USERS_LEFT');
  assert.equal(kept.databaseFailure.reason, 'rights');
  assert.equal(kept.databaseFailure.mutationApplied, true);
  const sentence = question.domainDeletionDatabaseFailureText(t, kept);
  assert.ok(sentence.includes('err.DATABASE_DELETE_USERS_LEFT'), `not the Databases page sentence: ${sentence}`);
  assert.equal(question.domainDeletionDatabaseFailureText(t, null), '');
});

test('the window’s end: the heading says deleted, the rows of what stayed, the closed Details', async () => {
  await mounted(async (tree) => {
    const kept = { choice: 'keep', databases: ['blog_db'], deleted: [], databaseFailure: null,
      mail: { bytes: 2048, files: 0, removeAfter: '2026-12-14T10:00:00Z', path: '/var/mail/vhosts/.celikpanel-quarantine/domain-4-0123456789abcdef' }, backups: null };
    await act(async () => {
      void confirmAction({
        title: 'domains.deleteTitle',
        ...question.domainDeleteQuestion(t, []),
        tone: 'danger',
        action: async () => ({ state: 'done', title: 'domains.deleted', ...question.domainDeletionEnd(t, kept, true, size, date) }),
      });
      await settled();
    });
    await act(async () => { button(tree, 'confirm-action-confirm').props.onClick({}); await settled(); });
    const rows = tree.root.findAll((node) => node.props['data-operation-kept'] !== undefined)[0];
    assert.ok(rows, 'the rows of what stayed are not shown');
    const shown = text(tree);
    assert.ok(shown.includes('domains.deleted.keptTitle') && shown.includes('domains.deleted.row.databases(names=blog_db)') && shown.includes('domains.deleted.row.mail'));
    const details = tree.root.findAll((node) => node.type === 'details' && node.props['data-operation-details'] !== undefined)[0];
    assert.ok(details && !details.props.open, 'the folders are not under a closed Details');
    assert.ok(text(tree).includes('domains.deleted'), 'the heading does not say deleted');
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
      return Response.json({ databases: [{ id: 2, name: 'shop_db', users: [] }, { id: 1, name: 'blog_db', users: [] }] });
    };
    assert.deepEqual((await question.readDomainDatabasesForDeletion('/api/v1', 4)).map((item) => item.name), ['blog_db', 'shop_db']);
  } finally {
    globalThis.fetch = saved;
  }
});
