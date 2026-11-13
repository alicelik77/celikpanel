// alpha.133: the left menu's Databases page follows the one visibility rule
// (drawings V15–V17). An administrator reads every database with its owner,
// a reseller their customers', a customer theirs grouped by domain. Mounted
// with the shipped shared layer, kit and decoder; the server is a stand-in
// fetch. Passwords and contents are never shown: the decoder refuses an
// answer that carries a field named like a secret.
//
// alpha.133: Sol menüdeki Veritabanları sayfası tek görünürlük kuralına uyar.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, dataModule, reactURL, replaceLazyImport, sharedLayer } from './fixtures/shared-layer.mjs';
import { enScreens, trScreens } from './locale-catalogue.mjs';

const icons = ['AlertTriangle', 'ArrowDown', 'ArrowRight', 'ArrowUp', 'Database', 'Globe', 'KeyRound', 'Plus', 'SearchX', 'Server', 'Trash2', 'X'];
const stub = dataModule(`
  import React from '${reactURL}';
  ${icons.map((name) => `export const ${name} = () => null;`).join('\n')}
  const say = (key, vars) => (vars ? key + JSON.stringify(vars) : key);
  export const useI18n = () => ({ t: say, locale: 'tr' });
  export const useNavigate = () => (path) => { globalThis.page.navigated.push(path); };
  export const Link = (props) => React.createElement('a', { href: props.to }, props.children);
  export const useAuth = () => ({ role: globalThis.page.role });
  export const showToast = (...args) => globalThis.page.toasts.push(args);
  export const PageHeader = (props) => React.createElement('header', null, React.createElement('h1', null, props.title), React.createElement('p', { 'data-subtitle': '' }, props.subtitle), props.actions);
  export const EngineMark = (props) => React.createElement('i', { 'data-engine-mark': props.engine });
`);
// The new-password form is the shared one (its own tests); here it shows what it was given.
const passwordForm = dataModule(`
  import React from '${reactURL}';
  export const DatabaseUserPassword = (props) => { globalThis.page.passwordForms.push(props); return React.createElement('form', { 'data-password-form': props.user }, props.user); };
`);
const shared = sharedLayer(stub);
const decoderURL = compileSource('lib/visibleDatabases.ts', () => stub);
const pageURL = replaceLazyImport(shared.compile('components/databases/DatabasesPage.tsx', {
  '/lib/visibleDatabases': decoderURL,
  '/auth/domainAccess': compileSource('auth/domainAccess.ts', () => stub),
  '/lib/databaseDeleteWords': compileSource('lib/databaseDeleteWords.ts', () => stub),
}), '../DatabaseUserPassword', passwordForm);
const { DatabasesPage } = await import(pageURL);
const { decodeVisibleDatabases, VISIBLE_DATABASES_URL } = await import(decoderURL);
const { typedNameMatches } = await import(shared.resolve('/ConfirmDetails'));

// --- The server's answers ----------------------------------------------------------------------
const ownerIds = new Map();
const owner = (username, extra = {}) => {
  if (!ownerIds.has(username)) ownerIds.set(username, ownerIds.size + 1);
  return { id: ownerIds.get(username), username, role: 'customer', parent: null, self: false, ...extra };
};
const db = (id, name, who, domain, extra = {}) => ({
  id, name, engine: 'mariadb', owner: who, domain: domain ? { id: id * 10, name: domain } : null,
  size_bytes: 14_900_000, created_at: '2026-11-11T10:00:00Z', users: [{ id: id * 100, username: `${name}_user` }], ...extra,
});
const ADMIN_LIST = [
  db(1, 'atolye_dr_test_shop', owner('atolye'), 'atolye.dr.test', { size_bytes: 190_840_832 }),
  db(2, 'atolye_dr_test_stok', owner('atolye'), 'atolye.dr.test', { engine: 'postgresql' }),
  db(3, 'kahvedukkani_dr_test_magaza', owner('musteri1', { parent: 'bayi1' }), 'kahvedukkani.dr.test', {
    users: [{ id: 31, username: 'kahve_app' }, { id: 32, username: 'kahve_rapor' }],
  }),
  db(4, 'musteri1_arsiv', owner('musteri1', { parent: 'bayi1' }), null, { size_bytes: null }),
  db(5, 'bayi1_deneme', owner('bayi1', { role: 'reseller' }), 'bayi1.dr.test'),
  db(6, 'wiki_dr_test_icerik', owner('yonetici', { role: 'admin', self: true }), 'wiki.dr.test', { users: [], size_bytes: 1_048_576 }),
];
const answer = (databases, extra = {}) => ({ databases, engines: [{ engine: 'mariadb', installed: true, running: true }, { engine: 'postgresql', installed: true, running: true }], may_create: true, ...extra });
const DOMAINS = [
  { id: 1, domain_name: 'zeta.dr.test', status: 'active', created_at: '' },
  { id: 2, domain_name: 'alfa.dr.test', status: 'active', created_at: '' },
  { id: 3, domain_name: 'closed.dr.test', status: 'active', created_at: '', rights_closed: ['databases'] },
  { id: 4, domain_name: 'leaving.dr.test', status: 'pending', created_at: '' },
];

// --- Mounting --------------------------------------------------------------------------------
let tree;
let calls;
const settled = async () => { for (let i = 0; i < 4; i++) await new Promise((resolve) => setTimeout(resolve, 0)); };
function serve(role, routes, changes = {}) {
  calls = [];
  globalThis.page = { role, navigated: [], toasts: [], passwordForms: [], asked: [] };
  globalThis.document = { addEventListener() {}, removeEventListener() {}, visibilityState: 'visible' };
  globalThis.window = { location: { origin: 'http://panel.test' } };
  globalThis.confirm = (text, request) => { globalThis.page.asked.push(request); return globalThis.page.answer ?? false; };
  globalThis.fetch = async (url, options = {}) => {
    const method = options.method || 'GET';
    calls.push({ method, url });
    const route = method === 'GET' ? routes[url] : changes[url];
    if (!route) return Response.json(method === 'GET' ? { error: 'no such route' } : { success: true }, { status: method === 'GET' ? 404 : 200 });
    return route();
  };
}
const json = (body, status = 200) => () => Response.json(body, { status });
async function mount(role, routes, changes) {
  serve(role, routes, changes);
  await act(async () => { tree = Renderer.create(React.createElement(DatabasesPage)); await settled(); });
}
async function cleanup() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  delete globalThis.page;
  delete globalThis.confirm;
}
const textOf = (node) => (node === null || node === undefined || typeof node === 'boolean' ? ''
  : typeof node === 'string' || typeof node === 'number' ? String(node)
    : Array.isArray(node) ? node.map(textOf).join(' ') : textOf(node.children));
const text = () => textOf(tree.toJSON());
const has = (key) => text().includes(key);
const rows = () => tree.root.findAll((node) => typeof node.type === 'string' && node.props['data-database-row'] !== undefined).map((node) => node.props['data-database-row']);
const button = (where) => tree.root.find((node) => node.type === 'button' && Object.entries(where).every(([key, value]) => node.props[key] === value));
const buttonsWith = (label) => tree.root.findAll((node) => node.type === 'button' && textOf(node).includes(label));
const press = async (node) => { await act(async () => { await node.props.onClick?.({}); await settled(); }); };
const openMenu = async (name) => press(button({ 'data-row-more': name }));
const menuItem = (id) => button({ role: 'menuitem', 'data-row-action': id });

// --- Administrator (V17) -----------------------------------------------------------------------

test('administrator: every database in one list, by owner then name, with the owner, domain, type and size', async () => {
  try {
    await mount('admin', { [VISIBLE_DATABASES_URL]: json(answer(ADMIN_LIST)) });
    assert.deepEqual(rows(), ['atolye_dr_test_shop', 'atolye_dr_test_stok', 'bayi1_deneme', 'kahvedukkani_dr_test_magaza', 'musteri1_arsiv', 'wiki_dr_test_icerik']);
    for (const column of ['databases.col.name', 'databases.col.owner', 'databases.col.domain', 'databases.col.kind', 'databases.col.size']) assert.ok(has(column), column);
    // Under the owner: "you", the reseller of a reseller's customer, a reseller.
    assert.ok(has('databases.owner.you'));
    assert.ok(has('databases.owner.reseller{"name":"bayi1"}'));
    assert.ok(has('databases.owner.isReseller'));
    assert.ok(has('databases.unbound'), 'a database bound to no domain says so');
    assert.ok(has('MariaDB') && has('PostgreSQL'), 'the type is said in words');
    assert.ok(has('182 MB') && has('14 MB'), 'sizes go through the size word');
    assert.ok(has('databases.sizeNotKnown'), 'a size not known is not drawn as 0');
    // 6 databases, 5 accounts; no "N items" counter.
    const subtitle = textOf(tree.root.findByProps({ 'data-subtitle': '' }));
    assert.match(subtitle, /databases\.scope\.admin · databases\.count\.databases\{"n":6\}, databases\.count\.accounts\{"n":4\}/);
    assert.ok(!has('common.itemsTotal'));
    // Nothing of the server's own machinery is on this page.
    for (const gone of ['databases.account.', 'databases.tab.', 'databases.default', 'dbremote.']) assert.ok(!has(gone), gone);
  } finally {
    await cleanup();
  }
});

test('administrator: a click on a column heading sorts by it, a second click the other way', async () => {
  try {
    await mount('admin', { [VISIBLE_DATABASES_URL]: json(answer(ADMIN_LIST)) });
    assert.equal(button({ 'data-sort': 'owner' }).parent.props['aria-sort'], 'ascending');
    await press(button({ 'data-sort': 'name' }));
    assert.deepEqual(rows().slice(0, 2), ['atolye_dr_test_shop', 'atolye_dr_test_stok']);
    assert.equal(rows().at(-1), 'wiki_dr_test_icerik');
    assert.equal(button({ 'data-sort': 'name' }).parent.props['aria-sort'], 'ascending');
    await press(button({ 'data-sort': 'name' }));
    assert.equal(rows()[0], 'wiki_dr_test_icerik');
    assert.equal(button({ 'data-sort': 'name' }).parent.props['aria-sort'], 'descending');
    await press(button({ 'data-sort': 'size' }));
    assert.equal(rows()[0], 'wiki_dr_test_icerik', 'smallest first');
    assert.equal(rows().at(-1), 'musteri1_arsiv', 'a size not known goes last');
    await press(button({ 'data-sort': 'domain' }));
    assert.equal(rows().at(-1), 'musteri1_arsiv', 'a database bound to no domain goes last');
  } finally {
    await cleanup();
  }
});

test('one search over name, owner and domain; no match says so and clears', async () => {
  try {
    await mount('admin', { [VISIBLE_DATABASES_URL]: json(answer(ADMIN_LIST)) });
    const search = () => tree.root.find((node) => node.type === 'input' && node.props.type === 'search');
    assert.equal(search().props.placeholder, 'databases.search');
    await act(async () => search().props.onChange({ target: { value: 'BAYI1' } }));
    assert.deepEqual(rows(), ['bayi1_deneme', 'kahvedukkani_dr_test_magaza', 'musteri1_arsiv'], 'the owner and the reseller are searched');
    await act(async () => search().props.onChange({ target: { value: 'wiki.dr' } }));
    assert.deepEqual(rows(), ['wiki_dr_test_icerik'], 'the domain is searched');
    await act(async () => search().props.onChange({ target: { value: 'kargo' } }));
    assert.deepEqual(rows(), []);
    assert.ok(has('databases.noMatch') && has('databases.noMatchHint{"q":"kargo"}'));
    assert.ok(!has('databases.empty.admin'), 'no match is not "no databases"');
    await press(button({ 'data-database-clear-search': '' }));
    assert.equal(rows().length, 6);
    assert.equal(search().props.value, '');
  } finally {
    await cleanup();
  }
});

test('"Open" and the name take the person to the owner\'s domain page at its Databases section; a database bound to no domain has no "Open"', async () => {
  try {
    await mount('admin', { [VISIBLE_DATABASES_URL]: json(answer(ADMIN_LIST)) });
    const open = tree.root.findAll((node) => node.type === 'button' && node.props['data-row-action'] === 'open');
    assert.equal(open.length, 5, 'one "Open" per database bound to a domain');
    const row = (name) => tree.root.find((node) => node.props['data-database-row'] === name);
    assert.equal(row('musteri1_arsiv').findAll((node) => node.type === 'button' && node.props['data-row-action'] === 'open').length, 0);
    await press(row('kahvedukkani_dr_test_magaza').find((node) => node.type === 'button' && node.props['data-row-action'] === 'open'));
    await press(row('atolye_dr_test_shop').findAll((node) => node.type === 'button')[0]);
    assert.deepEqual(globalThis.page.navigated, ['/domains/kahvedukkani.dr.test?section=databases', '/domains/atolye.dr.test?section=databases']);
  } finally {
    await cleanup();
  }
});

test('delete asks in the panel with what is lost, what stays and the typed name, then sends one DELETE and reads the list again', async () => {
  try {
    // The server says kahve_rapor stays (another database uses it).
    const list = ADMIN_LIST.map((item) => (item.id === 3 ? { ...item, users: [{ id: 31, username: 'kahve_app' }, { id: 32, username: 'kahve_rapor', stays: 'shared' }] } : item));
    await mount('admin', { [VISIBLE_DATABASES_URL]: json(answer(list)) }, { '/api/v1/databases/3': json({ status: 'deleted' }) });
    await openMenu('kahvedukkani_dr_test_magaza');
    // Declined: nothing is sent.
    globalThis.page.answer = false;
    await press(menuItem('delete'));
    assert.equal(calls.filter((call) => call.method === 'DELETE').length, 0);
    const [request] = globalThis.page.asked;
    assert.equal(request.tone, 'danger');
    assert.equal(request.title, 'databases.delete.title{"name":"kahvedukkani_dr_test_magaza"}');
    assert.equal(request.typedName.value, 'kahvedukkani_dr_test_magaza');
    assert.ok(!request.typedName.ignoreCase, 'the database name is typed exactly');
    assert.equal(typedNameMatches('kahvedukkani_dr_test_maga', request.typedName), false);
    assert.equal(typedNameMatches('Kahvedukkani_dr_test_magaza', request.typedName), false);
    assert.equal(typedNameMatches('kahvedukkani_dr_test_magaza', request.typedName), true);
    // alpha.133 round 5: the users only this database uses are removed with
    // it; one another database also uses stays (the server decides the same).
    assert.deepEqual(request.lost, ['databases.delete.lostDataSized{"name":"kahvedukkani_dr_test_magaza","size":"14 MB"}', 'databases.delete.lostUsers{"users":"kahve_app"}']);
    assert.deepEqual(request.kept, ['databases.delete.keptDomain{"domain":"kahvedukkani.dr.test"}', 'databases.delete.keptShared{"users":"kahve_rapor","name":"kahvedukkani_dr_test_magaza"}', 'databases.delete.keptOthers']);
    // Confirmed: one DELETE, the list is read again, the toast says it.
    globalThis.page.answer = true;
    const before = calls.filter((call) => call.url === VISIBLE_DATABASES_URL).length;
    await openMenu('kahvedukkani_dr_test_magaza');
    await press(menuItem('delete'));
    assert.deepEqual(calls.filter((call) => call.method === 'DELETE').map((call) => call.url), ['/api/v1/databases/3']);
    assert.ok(calls.filter((call) => call.url === VISIBLE_DATABASES_URL).length > before, 'the list is not read again');
    assert.deepEqual(globalThis.page.toasts, [['success', 'databases.dbDeleted']]);
  } finally {
    await cleanup();
  }
});

test('an engine or CelikPanel account is said as staying; accounts the server kept for their own rights are said after the delete', async () => {
  try {
    const list = ADMIN_LIST.map((item) => (item.id === 3 ? { ...item, users: [{ id: 31, username: 'kahve_app' }, { id: 32, username: 'root', stays: 'account' }] } : item));
    await mount('admin', { [VISIBLE_DATABASES_URL]: json(answer(list)) },
      { '/api/v1/databases/3': json({ status: 'deleted', users_removed: [], users_kept: ['root'], users_kept_on_engine: ['kahve_app'] }) });
    globalThis.page.answer = true;
    await openMenu('kahvedukkani_dr_test_magaza');
    await press(menuItem('delete'));
    const [request] = globalThis.page.asked;
    assert.deepEqual(request.lost.slice(1), ['databases.delete.lostUsers{"users":"kahve_app"}']);
    assert.ok(request.kept.includes('databases.delete.keptAccount{"users":"root","name":"kahvedukkani_dr_test_magaza"}'));
    assert.deepEqual(globalThis.page.toasts, [['success', 'databases.deletedNamed{"name":"kahvedukkani_dr_test_magaza"} databases.keptNote.otherRights{"name":"kahvedukkani_dr_test_magaza","users":"kahve_app"}']]);
  } finally {
    await cleanup();
  }
});

test('a server reached with its own account: the question says its users stay; a list that could not say offers no delete', async () => {
  try {
    const list = ADMIN_LIST.map((item) => (item.id === 3 ? { ...item, users: [{ id: 31, username: 'kahve_app', stays: 'server' }] }
      : item.id === 5 ? { ...item, users: [{ id: 51, username: 'bayi_user', stays: 'unknown' }] } : item));
    await mount('admin', { [VISIBLE_DATABASES_URL]: json(answer(list)) },
      { '/api/v1/databases/3': json({ status: 'deleted', users_removed: [], users_kept: [], users_kept_on_server: ['kahve_app'] }) });
    globalThis.page.answer = true;
    await openMenu('kahvedukkani_dr_test_magaza');
    await press(menuItem('delete'));
    const [request] = globalThis.page.asked;
    assert.equal(request.lost.length, 1, 'only the data is lost');
    assert.ok(request.kept.includes('databases.delete.keptServer{"users":"kahve_app","name":"kahvedukkani_dr_test_magaza"}'));
    assert.deepEqual(globalThis.page.toasts, [['success', 'databases.deletedNamed{"name":"kahvedukkani_dr_test_magaza"} databases.keptNote.server{"name":"kahvedukkani_dr_test_magaza","users":"kahve_app"}']]);
    // A database whose users' fate is unknown: no delete offered, and why.
    await openMenu('bayi1_deneme');
    const item = menuItem('delete');
    assert.equal(item.props['aria-disabled'], true);
    assert.ok(has('databases.delete.usersUnknown'), 'the reason is not said');
    assert.equal(calls.filter((call) => call.method === 'DELETE' && call.url.endsWith('/5')).length, 0);
  } finally {
    await cleanup();
  }
});

test('a deletion that left something says per group what is still to do, in the reader\u2019s words', async () => {
  try {
    await mount('admin', { [VISIBLE_DATABASES_URL]: json(answer(ADMIN_LIST)) }, {
      '/api/v1/databases/3': () => Response.json({ error: 'raw english', code: 'DATABASE_DELETE_USERS_LEFT', reason: 'mixed', partial_success: true, mutation_applied: true,
        vars: { name: 'kahvedukkani_dr_test_magaza', users: 'kahve_app, kahve_rapor', pending: 'kahve_app', rights: 'kahve_rapor' } }, { status: 409 }),
    });
    globalThis.page.answer = true;
    await openMenu('kahvedukkani_dr_test_magaza');
    await press(menuItem('delete'));
    assert.ok(has('err.DATABASE_DELETE_USERS_LEFT.mixed{"name":"kahvedukkani_dr_test_magaza","users":"kahve_app, kahve_rapor","pending":"kahve_app","rights":"kahve_rapor"}'), text());
    assert.ok(!has('raw english'));
  } finally {
    await cleanup();
  }
});

test('after a delete the row leaves the list by itself; another open tab drops it at its next half-minute read', async () => {
  const intervals = [];
  const realSetInterval = globalThis.setInterval;
  const realClearInterval = globalThis.clearInterval;
  globalThis.setInterval = (fn, ms) => { intervals.push({ fn, ms }); return intervals.length; };
  globalThis.clearInterval = () => {};
  try {
    let current = ADMIN_LIST;
    const routes = { [VISIBLE_DATABASES_URL]: () => Response.json(answer(current)) };
    // This tab deletes: the list is read again at once, no reload.
    await mount('admin', routes, { '/api/v1/databases/3': () => { current = current.filter((item) => item.id !== 3); return Response.json({ status: 'deleted' }); } });
    assert.ok(rows().includes('kahvedukkani_dr_test_magaza'));
    globalThis.page.answer = true;
    await openMenu('kahvedukkani_dr_test_magaza');
    await press(menuItem('delete'));
    assert.ok(!rows().includes('kahvedukkani_dr_test_magaza'), 'the deleted row is still drawn');
    assert.equal(rows().length, 5);
    await cleanup();
    // Another tab (its own page) learns of a delete made elsewhere at its
    // next half-minute read while it is visible; nothing is pressed there.
    intervals.length = 0;
    await mount('admin', routes);
    assert.ok(rows().includes('bayi1_deneme'));
    current = current.filter((item) => item.id !== 5);
    const tick = intervals.find((entry) => entry.ms === 30_000);
    assert.ok(tick, 'no half-minute read is set');
    await act(async () => { tick.fn(); await settled(); });
    assert.ok(!rows().includes('bayi1_deneme'), 'the other tab still draws a deleted database');
    assert.equal(calls.filter((call) => call.method !== 'GET').length, 0, 'a read changed something');
  } finally {
    globalThis.setInterval = realSetInterval;
    globalThis.clearInterval = realClearInterval;
    await cleanup();
  }
});

test('a refused delete says the reason in the reader’s words: the administrator gets the fix and its page, a customer is told to ask the administrator', async () => {
  const refusal = (extra) => () => Response.json({
    error: 'This MariaDB server is running and answering, but it refused the credential CelikPanel holds for it.',
    code: 'DATABASE_ENGINE_CREDENTIAL_REFUSED', vars: { engine: 'MariaDB' }, ...extra,
  }, { status: 409 });
  try {
    await mount('admin', { [VISIBLE_DATABASES_URL]: json(answer(ADMIN_LIST)) },
      { '/api/v1/databases/3': refusal({ reason: 'administrator', action: '/components/mariadb' }) });
    globalThis.page.answer = true;
    await openMenu('kahvedukkani_dr_test_magaza');
    await press(menuItem('delete'));
    assert.ok(has('err.DATABASE_ENGINE_CREDENTIAL_REFUSED.administrator{"engine":"MariaDB"}'), text());
    assert.ok(!has('refused the credential'), 'the server’s English sentence is drawn');
    assert.ok(rows().includes('kahvedukkani_dr_test_magaza'), 'a refused delete removed the row');
    await press(buttonsWith('common.goFix')[0]);
    assert.deepEqual(globalThis.page.navigated, ['/components/mariadb']);
    await cleanup();

    await mount('customer', { [VISIBLE_DATABASES_URL]: json(answer([db(3, 'kahvedukkani_dr_test_magaza', owner('musteri1', { self: true }), 'kahvedukkani.dr.test')])) },
      { '/api/v1/databases/3': refusal({}) });
    globalThis.page.answer = true;
    await openMenu('kahvedukkani_dr_test_magaza');
    await press(menuItem('delete'));
    assert.ok(has('err.DATABASE_ENGINE_CREDENTIAL_REFUSED{"engine":"MariaDB"}'), text());
    assert.equal(buttonsWith('common.goFix').length, 0, 'a customer is sent to a page they do not have');
    // Both languages carry both sentences, with the same placeholders.
    for (const key of ['err.DATABASE_ENGINE_CREDENTIAL_REFUSED', 'err.DATABASE_ENGINE_CREDENTIAL_REFUSED.administrator', 'err.DATABASE_ENGINE_UNREACHABLE', 'err.DATABASE_ENGINE_UNREACHABLE.administrator',
      'err.DATABASE_DELETE_FAILED', 'err.DATABASE_DELETE_UNCONFIRMED', 'err.DATABASE_DELETE_USERS_LEFT', 'err.DATABASE_DELETE_USERS_LEFT.unknown', 'err.DATABASE_DELETE_USERS_LEFT.rights', 'err.DATABASE_DELETE_USERS_LEFT.mixed', 'err.DATABASE_DELETE_USERS_LEFT.exists',
      'err.DATABASE_DELETE_RECORDS_LEFT', 'err.DATABASE_ENGINE_BUSY', 'databases.delete.lostUsers', 'databases.delete.keptShared', 'databases.delete.keptAccount', 'databases.delete.keptServer',
      'databases.delete.usersUnknown', 'databases.deletedNamed', 'databases.keptNote.otherRights', 'databases.keptNote.unchecked', 'databases.keptNote.server']) {
      assert.ok(enScreens[key] && trScreens[key], key);
      const holes = (value) => [...value.matchAll(/\{(\w+)\}/g)].map((match) => match[1]).sort().join(',');
      assert.equal(holes(trScreens[key]), holes(enScreens[key]), key);
      assert.doesNotMatch(trScreens[key], /CREDENTIAL|REFUSED|credential/, key);
    }
  } finally {
    await cleanup();
  }
});

test('a new password asks which user when there are several, uses the shared form, and never shows a password', async () => {
  try {
    await mount('admin', { [VISIBLE_DATABASES_URL]: json(answer(ADMIN_LIST)) });
    // No user: the item says why it cannot be used.
    await openMenu('wiki_dr_test_icerik');
    assert.equal(menuItem('password').props['aria-disabled'], true);
    assert.ok(has('databases.noUsers'));
    await openMenu('wiki_dr_test_icerik');
    await openMenu('kahvedukkani_dr_test_magaza');
    await press(menuItem('password'));
    assert.ok(has('databases.password.which'), 'several users: which one is asked');
    assert.equal(globalThis.page.passwordForms.length, 0, 'the form opened before a user was chosen');
    const choice = tree.root.findAll((node) => node.type === 'input' && node.props.type === 'radio');
    assert.deepEqual(choice.map((node) => node.props.value), [31, 32]);
    await act(async () => { choice[1].props.onChange(); await settled(); });
    const form = globalThis.page.passwordForms.at(-1);
    assert.equal(form.url, '/api/v1/database-users/32/password');
    assert.equal(form.user, 'kahve_rapor');
    assert.deepEqual(Object.keys(form).sort(), ['onClose', 'onSettled', 'url', 'user'], 'the form is given no password');
    // One user: the form opens for that user at once.
    await act(async () => form.onClose());
    await openMenu('atolye_dr_test_shop');
    await press(menuItem('password'));
    assert.ok(!has('databases.password.which'));
    assert.equal(globalThis.page.passwordForms.at(-1).user, 'atolye_dr_test_shop_user');
    assert.doesNotMatch(text(), /password":|secret/i);
  } finally {
    await cleanup();
  }
});

test('an engine that is installed but stopped is told to the administrator with the way to Components', async () => {
  try {
    await mount('admin', { [VISIBLE_DATABASES_URL]: json(answer(ADMIN_LIST, { engines: [{ engine: 'mariadb', installed: true, running: false }, { engine: 'postgresql', installed: false, running: false }] })) });
    assert.ok(has('databases.stopped.title{"engine":"MariaDB"}'));
    assert.ok(!has('databases.stopped.title{"engine":"PostgreSQL"}'), 'an engine that is not installed is not "stopped"');
    await press(buttonsWith('databases.openComponents')[0]);
    assert.deepEqual(globalThis.page.navigated, ['/components']);
    assert.ok(!has('databases.noEngine.title'), 'one engine is installed');
  } finally {
    await cleanup();
  }
});

// --- Reseller (V16) and customer (V15) ---------------------------------------------------------

test('reseller: the same list for their customers, no reseller line under their own customers, and nothing of the administrator\'s', async () => {
  const mine = ADMIN_LIST.filter((item) => item.owner.username === 'musteri1').map((item) => ({ ...item, owner: { ...item.owner } }));
  try {
    await mount('reseller', { [VISIBLE_DATABASES_URL]: json(answer(mine, { may_create: false, engines: [{ engine: 'mariadb', installed: true, running: false }] })) });
    assert.deepEqual(rows(), ['kahvedukkani_dr_test_magaza', 'musteri1_arsiv']);
    assert.ok(has('databases.col.owner'));
    assert.ok(!has('databases.owner.reseller'), 'a reseller is not told that their customer is theirs');
    assert.ok(has('databases.resellerNoAccount'), 'a reseller without an account of their own is told why nothing is added in their name');
    assert.equal(buttonsWith('databases.addDatabase').length, 0);
    for (const adminOnly of ['databases.stopped.', 'databases.noEngine.', 'databases.openComponents']) assert.ok(!has(adminOnly), adminOnly);
    const subtitle = textOf(tree.root.findByProps({ 'data-subtitle': '' }));
    assert.match(subtitle, /databases\.scope\.reseller · databases\.count\.databases\{"n":2\}, databases\.count\.customersOne/);
  } finally {
    await cleanup();
  }
});

test('customer: their databases grouped by domain, those bound to none last, no owner column', async () => {
  const self = owner('musteri1', { self: true });
  const list = [
    db(3, 'kahvedukkani_dr_test_rapor', self, 'kahvedukkani.dr.test', { engine: 'postgresql' }),
    db(4, 'musteri1_arsiv', self, null),
    db(5, 'randevu_dr_test_takvim', self, 'randevu.dr.test'),
    db(6, 'kahvedukkani_dr_test_magaza', self, 'kahvedukkani.dr.test'),
  ];
  try {
    await mount('customer', { [VISIBLE_DATABASES_URL]: json(answer(list)) });
    const groups = tree.root.findAll((node) => node.type === 'section').map((node) => node.props['data-database-group']);
    assert.deepEqual(groups, ['kahvedukkani.dr.test', 'randevu.dr.test', 'unbound']);
    assert.deepEqual(rows(), ['kahvedukkani_dr_test_magaza', 'kahvedukkani_dr_test_rapor', 'randevu_dr_test_takvim', 'musteri1_arsiv']);
    assert.ok(has('databases.group.unbound'));
    assert.ok(!has('databases.col.owner') && !has('databases.owner.'), 'a customer reads no owner');
    assert.equal(tree.root.findAll((node) => node.type === 'input' && node.props.type === 'search').length, 0);
    assert.deepEqual(tree.root.findAll((node) => node.props['data-engine-mark']).map((node) => node.props['data-engine-mark']), ['mariadb', 'postgresql', 'mariadb', 'mariadb']);
    const subtitle = textOf(tree.root.findByProps({ 'data-subtitle': '' }));
    assert.match(subtitle, /^databases\.scope\.customer · databases\.count\.databases\{"n":4\}$/);
    // The same row actions.
    assert.equal(tree.root.findAll((node) => node.type === 'button' && node.props['data-row-action'] === 'open').length, 3);
    assert.equal(tree.root.findAll((node) => node.type === 'button' && node.props['data-row-more'] !== undefined).length, 4);
  } finally {
    await cleanup();
  }
});

test('"Add database" asks for which domain, lists only the domains that can take one, and opens that domain\'s Databases section', async () => {
  try {
    await mount('customer', { [VISIBLE_DATABASES_URL]: json(answer([])), '/api/v1/domains': json(DOMAINS) });
    assert.ok(has('databases.empty.customer') && has('databases.empty.customerHint'));
    const add = buttonsWith('databases.addDatabase');
    assert.equal(add.length, 1, '"Add database" stands once, in the header');
    assert.equal(textOf(tree.root.findByProps({ 'data-subtitle': '' })), 'databases.scope.customer', 'an empty list carries no count');
    await press(add[0]);
    assert.ok(has('databases.pick.title'));
    const offered = tree.root.findAll((node) => node.type === 'button' && node.props['data-database-pick'] !== undefined).map((node) => node.props['data-database-pick']);
    assert.deepEqual(offered, ['alfa.dr.test', 'zeta.dr.test'], 'a domain whose plan closes databases or that is being deleted is offered');
    await press(button({ 'data-database-pick': 'zeta.dr.test' }));
    // The section opens with its own "add a database" form (DomainDatabaseManager reads ?create=1 once).
    assert.deepEqual(globalThis.page.navigated, ['/domains/zeta.dr.test?section=databases&create=1']);
    assert.ok(!has('databases.pick.title'), 'the question stays open');
    assert.equal(calls.filter((call) => call.method !== 'GET').length, 0, 'choosing a domain changes nothing');
  } finally {
    await cleanup();
  }
});

test('a team member is offered only the domains where they may manage databases', async () => {
  const domainAccess = readFileSync(new URL('../src/auth/domainAccess.ts', import.meta.url), 'utf8');
  const capabilities = /DOMAIN_CAPABILITIES = \[([^\]]+)\]/.exec(domainAccess)[1].match(/'(\w+)'/g).map((item) => item.slice(1, -1));
  const grant = (mode) => Object.fromEntries(capabilities.map((name) => [name, name === 'databases' ? mode : 'none']));
  try {
    await mount('additional_user', { [VISIBLE_DATABASES_URL]: json(answer([])), '/api/v1/domains': json([
      { id: 1, domain_name: 'view.dr.test', status: 'active', access: grant('view') },
      { id: 2, domain_name: 'manage.dr.test', status: 'active', access: grant('manage') },
    ]) });
    await press(buttonsWith('databases.addDatabase')[0]);
    assert.deepEqual(tree.root.findAll((node) => node.type === 'button' && node.props['data-database-pick'] !== undefined).map((node) => node.props['data-database-pick']), ['manage.dr.test']);
  } finally {
    await cleanup();
  }
});

// --- The server's real answer in alpha.133 -------------------------------------------------------

// dblist-be answers size_bytes null for every row (sizes are read one at a
// time by the Agent), created_at "" for a record without a time, engines null
// when the Agent could not be asked, and an empty list for a team member.
const real = (row) => ({ ...row, size_bytes: null });
test('no size anywhere: the size column, its sort and its phone line are absent; an unknown date is not drawn', async () => {
  try {
    await mount('admin', { [VISIBLE_DATABASES_URL]: json(answer(ADMIN_LIST.map(real))) });
    assert.equal(rows().length, 6);
    assert.ok(!has('databases.col.size') && !has('databases.sizeNotKnown'), 'a column of dashes is drawn');
    assert.equal(tree.root.findAll((node) => node.type === 'button' && node.props['data-sort'] === 'size').length, 0);
    assert.equal(tree.root.findAll((node) => node.props['data-label'] === 'databases.col.size').length, 0);
    await openMenu('kahvedukkani_dr_test_magaza');
    globalThis.page.answer = false;
    await press(menuItem('delete'));
    assert.deepEqual(globalThis.page.asked[0].lost[0], 'databases.delete.lostData{"name":"kahvedukkani_dr_test_magaza"}');
  } finally {
    await cleanup();
  }
  const self = owner('musteri1', { self: true });
  try {
    await mount('customer', { [VISIBLE_DATABASES_URL]: json(answer([
      real(db(3, 'kahvedukkani_dr_test_rapor', self, 'kahvedukkani.dr.test', { created_at: '' })),
      real(db(4, 'kahvedukkani_dr_test_magaza', self, 'kahvedukkani.dr.test')),
    ])) });
    assert.ok(!has('databases.sizeNotKnown'), 'every card says the size is not known');
    assert.equal(text().split('databases.added').length - 1, 1, 'a record without a time says when it was added');
  } finally {
    await cleanup();
  }
});

test('engines not known right now: the administrator reads "could not be read", nobody reads "no database service"', async () => {
  try {
    await mount('admin', { [VISIBLE_DATABASES_URL]: json(answer([], { engines: null })) });
    assert.ok(tree.root.findAll((node) => node.props['data-database-guidance'] === 'engines-unknown').length > 0);
    assert.ok(!has('databases.noEngine') && has('databases.empty.adminHint'));
    assert.equal(buttonsWith('databases.addDatabase').length, 1, '"Add database" waits for nothing the server could not say');
  } finally {
    await cleanup();
  }
  for (const role of ['reseller', 'customer']) {
    try {
      await mount(role, { [VISIBLE_DATABASES_URL]: json(answer([], { engines: null })) });
      assert.ok(!has('databases.noEngine') && !has('databases.enginesUnknown'), role);
    } finally {
      await cleanup();
    }
  }
});

test('a team member: an empty list says where their databases are, never "none" and never "no service"', async () => {
  try {
    await mount('additional_user', { [VISIBLE_DATABASES_URL]: json({ databases: [], engines: [], may_create: false }), '/api/v1/domains': json([]) });
    assert.ok(has('databases.empty.team') && has('databases.empty.teamHint'));
    assert.ok(!has('databases.noEngine') && !has('databases.empty.customer'));
  } finally {
    await cleanup();
  }
});

test('the administrator’s list says, in one quiet line, that outside connections are set on Security; nobody else’s does', async () => {
  try {
    await mount('admin', { [VISIBLE_DATABASES_URL]: json(answer(ADMIN_LIST.map(real))) });
    const link = tree.root.findAll((node) => node.type === 'a' && node.props.href === '/security#outside-connections');
    assert.equal(link.length, 1);
    assert.equal(textOf(link[0]), 'databases.outsideLink');
  } finally {
    await cleanup();
  }
  for (const role of ['reseller', 'customer']) {
    try {
      await mount(role, { [VISIBLE_DATABASES_URL]: json(answer(ADMIN_LIST.map(real))) });
      assert.ok(!has('databases.outsideLink'), role);
    } finally {
      await cleanup();
    }
  }
});

// --- States --------------------------------------------------------------------------------------

test('each role\'s really-empty list, the provider\'s missing service, and nothing claimed before the answer', async () => {
  const cases = [
    ['admin', answer([]), 'databases.empty.admin'],
    ['reseller', answer([], { may_create: false }), 'databases.empty.reseller'],
    ['customer', answer([], { may_create: false }), 'databases.empty.customerClosed'],
    ['customer', answer([], { engines: [] }), 'databases.noEngine.tenant'],
    ['reseller', answer([], { engines: [{ engine: 'mariadb', installed: false, running: false }] }), 'databases.noEngine.tenant'],
  ];
  for (const [role, body, expected] of cases) {
    try {
      await mount(role, { [VISIBLE_DATABASES_URL]: json(body) });
      assert.ok(has(expected), `${role}: ${expected}`);
      if (expected === 'databases.empty.reseller') assert.ok(!has('databases.noEngine'), 'a reseller is told "no engine" while engines exist');
    } finally {
      await cleanup();
    }
  }
  // Being read: the checking line only.
  try {
    await mount('admin', { [VISIBLE_DATABASES_URL]: () => new Promise(() => {}) });
    assert.ok(has('databases.checking'));
    for (const claim of ['databases.empty.', 'databases.noEngine', 'databases.addDatabase']) assert.ok(!has(claim), claim);
  } finally {
    await cleanup();
  }
});

test('a list that could not be read says so, changes nothing and reads again on request', async () => {
  let fail = true;
  try {
    await mount('admin', { [VISIBLE_DATABASES_URL]: () => (fail ? Promise.reject(new TypeError('offline')) : Response.json(answer(ADMIN_LIST))) });
    assert.ok(has('databases.unknown'));
    assert.ok(!has('databases.empty.admin') && !has('databases.addDatabase'));
    fail = false;
    await press(buttonsWith('common.retry')[0]);
    assert.equal(rows().length, 6);
  } finally {
    await cleanup();
  }
});

test('the list reads itself again on an interval while the tab is visible, and only reads', () => {
  const source = readFileSync(new URL('../src/components/databases/DatabasesPage.tsx', import.meta.url), 'utf8');
  assert.match(source, /useRefreshEvery\(list, REFRESH_MS, useTabVisible\(\)\);/);
  assert.match(source, /const REFRESH_MS = 30_000;/);
  assert.doesNotMatch(source, /window\.(confirm|alert|prompt)/);
  assert.doesNotMatch(source, /common\.refresh|RefreshCw/, 'no manual refresh button');
});

// --- The decoder ---------------------------------------------------------------------------------

test('the decoder takes the contract and refuses an answer with a secret-looking field rather than drawing it', async () => {
  const good = answer(ADMIN_LIST);
  const decoded = decodeVisibleDatabases(good);
  assert.equal(decoded.databases.length, 6);
  assert.equal(decoded.mayCreate, true);
  assert.deepEqual(decoded.databases[3].domain, null);
  assert.equal(decoded.databases[3].sizeBytes, null);
  assert.equal(decoded.databases[2].owner.parent, 'bayi1');
  for (const field of ['password', 'secret', 'db_password', 'connection_string', 'token']) {
    const bad = structuredClone(good);
    bad.databases[0].users[0][field] = 'hunter2';
    assert.throws(() => decodeVisibleDatabases(bad), /secret-looking/, field);
  }
  const top = { ...structuredClone(good), admin_password: 'x' };
  assert.throws(() => decodeVisibleDatabases(top), /secret-looking/);
  assert.equal(decodeVisibleDatabases({ ...good, engines: null }).engines, null, 'engines not known is kept as not known');
  assert.equal(decodeVisibleDatabases({ ...good, databases: [{ ...good.databases[0], created_at: '' }] }).databases[0].createdAt, null);
  for (const broken of [null, [], { databases: [] }, { ...good, may_create: 'yes' }, { ...good, engines: 'none' }, { ...good, databases: [{ ...good.databases[0], owner: null }] }]) {
    assert.throws(() => decodeVisibleDatabases(broken), /not the contract/);
  }
  // On the page: such an answer is "could not be read", and the secret is nowhere.
  const leaking = structuredClone(good);
  leaking.databases[0].password = 'hunter2';
  try {
    await mount('admin', { [VISIBLE_DATABASES_URL]: json(leaking) });
    assert.ok(has('databases.unknown'));
    assert.ok(!has('hunter2'));
  } finally {
    await cleanup();
  }
});

// --- Words ---------------------------------------------------------------------------------------

test('every text of the page is in both languages with the same placeholders, and the could-not-read ones say nothing was changed', () => {
  const source = readFileSync(new URL('../src/components/databases/DatabasesPage.tsx', import.meta.url), 'utf8');
  const keys = [...new Set([...source.matchAll(/'((?:databases)\.[a-zA-Z0-9_.]+)'/g)].map((match) => match[1]))];
  assert.ok(keys.length > 50, `${keys.length} keys`);
  for (const key of keys) {
    assert.ok(enScreens[key], `${key} has no English`);
    assert.ok(trScreens[key], `${key} has no Turkish`);
    assert.deepEqual((trScreens[key].match(/\{\w+\}/g) ?? []).sort(), (enScreens[key].match(/\{\w+\}/g) ?? []).sort(), key);
  }
  for (const key of ['databases.unknown', 'databases.pick.unknown']) {
    assert.match(enScreens[key], /Nothing was changed/);
    assert.match(trScreens[key], /Hiçbir şey değiştirilmedi/);
  }
  assert.equal(trScreens['databases.empty.admin'], 'Bu sunucuda henüz veritabanı yok.');
  assert.equal(trScreens['databases.empty.reseller'], 'Müşterilerinizin henüz veritabanı yok.');
  assert.equal(trScreens['databases.noEngine.tenant'], 'Sağlayıcınız bu sunucuda veritabanı sunmuyor.');
  assert.equal(trScreens['databases.pick.title'], 'Hangi alan adı için?');
  assert.equal(trScreens['databases.addDatabase'], 'Veritabanı ekle');
  // The removed page's words are gone with it.
  for (const gone of ['databases.subtitle', 'databases.tab.users', 'databases.default', 'databases.noServers', 'databases.passwordNotShown', 'dbm.createDatabase']) {
    assert.equal(enScreens[gone], undefined, gone);
    assert.equal(trScreens[gone], undefined, gone);
  }
});

test('the domain’s Databases section opens its "add a database" form for ?create=1, once, and takes the parameter out of the address', () => {
  const manager = readFileSync(new URL('../src/components/DomainDatabaseManager.tsx', import.meta.url), 'utf8');
  assert.match(manager, /useState\(\(\) => !readOnly && new URLSearchParams\(search\)\.get\('create'\) === '1'\)/);
  assert.match(manager, /params\.delete\('create'\);[\s\S]{0,120}replaceCurrentRouterURL\(`\$\{pathname\}\$\{query \? `\?\$\{query\}` : ''\}\$\{hash\}`\)/);
});
