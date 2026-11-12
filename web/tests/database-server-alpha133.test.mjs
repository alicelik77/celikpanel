import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';

// alpha.133: the server's own database matters leave the Databases page.
// Security › Open to the outside holds the switch of connections from outside
// (saved only with Save, its consequences asked before anything is sent) and
// each service's readiness in plain words; MariaDB's and PostgreSQL's pages
// hold the account CelikPanel connects with, behind a "More" menu. Mounted
// with the shipped kit, remote layer, confirmation and route-scoped words; the
// stand-in catalogue answers keys, so the words are checked by key.
// alpha.133: sunucunun kendi veritabanı işleri Güvenlik ve bileşen sayfalarına
// taşındı; bağlı hâlde, gönderilen parçalarla sınanır.

const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const repo = (path) => readFileSync(new URL('../../' + path, import.meta.url), 'utf8');

const stub = dataModule(`
  import React from '${reactURL}';
  ${['Globe', 'KeyRound', 'Eye', 'RefreshCw', 'Trash2', 'Copy', 'Check', 'Plus', 'AlertTriangle'].map((name) => `export const ${name} = () => null;`).join('\n')}
  // A locale the route-scoped copies do not have: every word is its key.
  export const useI18n = () => ({ t: (key) => key, locale: 'xx' });
  export const useAuth = () => ({ role: globalThis.currentRole });
  export const useNavigate = () => (path) => { globalThis.navigated.push(path); };
  export const useLocation = () => ({ pathname: '/security', search: '', hash: '' });
  export const showToast = (...args) => { globalThis.toasts.push(args); };
`);
const shared = sharedLayer(stub);
const routeCopy = (name) => compileSource(`i18n/${name}.ts`, (specifier) => (
  specifier === './index' ? stub : compileSource(`i18n/${specifier.slice(2)}.ts`, () => stub)
));
const own = { '/i18n/dbServer': routeCopy('dbServer'), '/i18n/dbRemote': routeCopy('dbRemote'), '/auth/AuthContext': stub, '/router': stub, '/Toast': stub };
const panelModule = await import(shared.compile('components/DatabaseRemoteServerPanel.tsx', own));
const { DatabaseRemoteServerPanel, EngineAccountBlock, PROBLEM_FIX } = panelModule;
const { DatabaseRemoteAccess } = await import(shared.compile('components/DatabaseRemoteAccess.tsx', { ...own, '/i18n': stub }));
const { enDbServer } = await import(compileSource('i18n/dbServer/en.ts', () => stub));
const { trDbServer } = await import(compileSource('i18n/dbServer/tr.ts', () => stub));

// The codes of the transport contract.
const contract = repo('internal/transport/database_remote_contracts.go');
const CODES = [...contract.matchAll(/\bDatabaseRemote\w+\s*=\s*"([A-Z][A-Z0-9_]+)"/g)].map((m) => m[1]);

const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
let calls = [];
let routes = {};
function serve(table) {
  calls = [];
  routes = table;
  globalThis.navigated = [];
  globalThis.toasts = [];
  globalThis.operationEnds = [];
  globalThis.fetch = async (input, init = {}) => {
    const url = String(input);
    const method = (init.method ?? 'GET').toUpperCase();
    const body = init.body ? JSON.parse(init.body) : undefined;
    calls.push({ url, method, body });
    const route = routes[`${method} ${url}`];
    if (!route) return new Response('{}', { status: 404, headers: { 'Content-Type': 'application/json' } });
    return route({ url, method, body });
  };
}
const json = (value, status = 200) => () => new Response(JSON.stringify(value), { status, headers: { 'Content-Type': 'application/json' } });
let asked = [];
const answering = (answer) => {
  asked = [];
  globalThis.confirm = (text, request) => { asked.push(request); return answer; };
};

// react-test-renderer has no DOM: the "More" menu gets a document and a
// window that keep nothing, and each host node a small box.
globalThis.document ??= { activeElement: null, addEventListener() {}, removeEventListener() {}, body: {}, getElementById: () => null };
globalThis.window ??= {
  innerHeight: 900, innerWidth: 1440, addEventListener() {}, removeEventListener() {},
  setInterval: (...args) => setInterval(...args), clearInterval: (id) => clearInterval(id),
  setTimeout: (...args) => setTimeout(...args), clearTimeout: (id) => clearTimeout(id),
};
const nodeMock = () => ({ focus() {}, getBoundingClientRect: () => ({ top: 100, bottom: 132, left: 900, right: 1000 }), offsetWidth: 200, offsetHeight: 160, contains: () => false });

let tree;
async function mount(element) {
  await act(async () => { tree = Renderer.create(element, { createNodeMock: nodeMock }); });
  await act(async () => { await settled(); await settled(); });
}
async function unmount() {
  await act(async () => tree.unmount());
}
const text = () => JSON.stringify(tree.toJSON() ?? '');
// The page as read without opening any "Details": every <details> left out.
const withoutDetails = (node) => {
  if (!node || typeof node !== 'object') return node;
  if (Array.isArray(node)) return node.map(withoutDetails);
  if (node.type === 'details') return null;
  return { ...node, children: node.children ? node.children.map(withoutDetails) : node.children };
};
const inlineText = () => JSON.stringify(withoutDetails(tree.toJSON()));
const press = async (node) => { await act(async () => { node.props.onClick(); await settled(); await settled(); }); };
const byData = (name, value) => tree.root.findAll((node) => typeof node.type === 'string' && node.props[name] !== undefined && (value === undefined || node.props[name] === value));

const engine = (name, port, readiness = [], extra = {}) => ({ engine: name, port, rules: 0, state: { known: true, state: 'off', readiness, problems: [], ...extra } });
const serverView = (over = {}) => ({ offered: false, rules: 0, engines: [], mariadb_skip_name_resolve: false, panel_firewall_note: '', ...over });

test('every problem code of the transport contract has its words in English and Turkish, and its button', () => {
  assert.ok(CODES.length >= 20, `the contract file names ${CODES.length} codes`);
  for (const code of CODES) {
    for (const part of ['why', 'next']) {
      const key = `dbsv.problem.${code}.${part}`;
      assert.ok(enDbServer[key], `${key} has no English text`);
      assert.ok(trDbServer[key], `${key} has no Turkish text`);
    }
    assert.ok(code in PROBLEM_FIX, `${code} has no button decision`);
  }
  assert.deepEqual(Object.keys(PROBLEM_FIX).filter((code) => !CODES.includes(code)), [], 'a button decision for a code the contract does not have');
  assert.deepEqual(Object.keys(trDbServer).sort(), Object.keys(enDbServer).sort());
  // The account's old file is gone with the old Databases page (no leftovers).
  assert.ok(!existsSync(new URL('../src/components/DatabaseAccountStrip.tsx', import.meta.url)), 'DatabaseAccountStrip.tsx is back');
  for (const value of Object.values(trDbServer)) assert.doesNotMatch(value, /\b(the|and|is not)\b/, `English left in Turkish: ${value}`);
});

test('non-administrators render none of the server matters and ask nothing', async () => {
  for (const role of ['customer', 'reseller']) {
    globalThis.currentRole = role;
    serve({});
    await mount(React.createElement(DatabaseRemoteServerPanel));
    assert.equal(tree.toJSON(), null);
    await unmount();
    await mount(React.createElement(EngineAccountBlock, { engine: 'mariadb' }));
    assert.equal(tree.toJSON(), null);
    await unmount();
    assert.deepEqual(calls, [], `${role}: a read was sent`);
  }
});

test('every known code says why, who and the next step with its button; the server sentence is only under Details', async () => {
  globalThis.currentRole = 'admin';
  const blocking = CODES.filter((code) => code !== 'PG_FAIL2BAN');
  const readiness = blocking.map((code) => ({ Code: code, Detail: `raw sentence of ${code}` }));
  serve({ 'GET /api/v1/database-remote/server': json(serverView({ offered: true, engines: [engine('mysql', 3306, readiness)] })) });
  try {
    await mount(React.createElement(DatabaseRemoteServerPanel));
    for (const code of blocking) {
      assert.equal(byData('data-dbsv-problem', code).length, 1, `${code} has no guidance block`);
    }
    const page = text();
    const inline = inlineText();
    for (const code of blocking) {
      assert.ok(page.includes(`dbsv.problem.${code}.why`), `${code}: no reason`);
      assert.ok(page.includes(`dbsv.problem.${code}.next`), `${code}: no next step`);
      assert.ok(page.includes(`raw sentence of ${code}`), `${code}: the detail is not under Details`);
      assert.ok(!inline.includes(`raw sentence of ${code}`), `${code}: the server sentence is shown inline`);
    }
    assert.ok(page.includes('kit.guidance.who.you'));
    assert.equal(byData('data-dbsv-state', 'notReady').length, 1);
    // One button per block at most, the right one per code.
    const fixes = new Set(byData('data-dbsv-fix').map((node) => node.props['data-dbsv-fix']));
    for (const fix of ['components', 'apply', 'nameLookup']) assert.ok(fixes.has(fix), `no ${fix} button`);
    await press(byData('data-dbsv-fix', 'components')[0]);
    assert.deepEqual(globalThis.navigated, ['/components']);
    assert.equal(calls.filter((call) => call.method !== 'GET').length, 0, 'a guidance button sent a change without asking');
  } finally {
    await unmount();
  }
});

test('Fail2ban missing gets its own words and leads to its install; an unknown code is said neutrally with its details', async () => {
  globalThis.currentRole = 'admin';
  serve({ 'GET /api/v1/database-remote/server': json(serverView({ engines: [
    engine('mysql', 3306, [{ Code: 'NO_SLOWDOWN', Detail: 'Fail2ban is not installed; install it in Components' }, { Code: 'SOMETHING_NEW', Detail: 'a sentence nobody wrote words for' }]),
    engine('postgresql', 5432, [{ Code: 'PG_FAIL2BAN', Detail: 'Fail2ban is not installed; install it in Components to also ban addresses' }], { fail2ban: 'unavailable' }),
  ] })) });
  try {
    await mount(React.createElement(DatabaseRemoteServerPanel));
    const page = text();
    assert.ok(page.includes('dbsv.problem.NO_SLOWDOWN.f2bMissing.why'));
    assert.ok(page.includes('dbsv.problem.other.why') && page.includes('SOMETHING_NEW'));
    assert.ok(!inlineText().includes('a sentence nobody wrote words for') && !inlineText().includes('Fail2ban is not installed'));
    // PostgreSQL's extra layer never holds it back: ready, with a quiet note.
    assert.equal(byData('data-dbsv-state', 'ready').length, 1);
    assert.ok(page.includes('dbsv.note.pgFail2banMissing') && page.includes('dbsv.encrypted'));
    await press(byData('data-dbsv-fix', 'fail2ban')[0]);
    assert.deepEqual(globalThis.navigated, ['/components?component=fail2ban&install=']);
  } finally {
    await unmount();
  }
});

test('the switch sends nothing until Save; turning it off lists what stops before anything is sent', async () => {
  globalThis.currentRole = 'admin';
  let offered = true;
  serve({
    'GET /api/v1/database-remote/server': () => json(serverView({ offered, rules: 3, engines: [engine('mysql', 3306)] }))(),
    'PUT /api/v1/database-remote/server': ({ body }) => { offered = body.offered; return json({ ...serverView({ offered, rules: 3 }), changed: true })(); },
  });
  try {
    await mount(React.createElement(DatabaseRemoteServerPanel));
    const toggle = () => tree.root.find((node) => node.type === 'input' && node.props.role === 'switch');
    assert.equal(toggle().props.checked, true);
    assert.equal(byData('data-save-bar').length, 0);
    await act(async () => { toggle().props.onChange({ currentTarget: { checked: false } }); });
    assert.equal(toggle().props.checked, false);
    assert.equal(byData('data-changed-mark').length, 1, 'the changed row is not marked');
    assert.equal(byData('data-save-bar').length, 1);
    assert.equal(calls.filter((call) => call.method === 'PUT').length, 0, 'clicking the switch sent a change');

    // "No" sends nothing and keeps the change.
    answering(false);
    await press(byData('data-save')[0]);
    assert.equal(asked.length, 1);
    assert.equal(asked[0].title, 'dbsv.offTitle');
    assert.ok(asked[0].lost.includes('dbsv.offLost'), 'turning off does not say that the listed addresses stop');
    assert.ok(asked[0].kept.includes('dbsv.offKept'));
    assert.equal(asked[0].tone, 'danger');
    assert.equal(calls.filter((call) => call.method === 'PUT').length, 0);
    assert.equal(toggle().props.checked, false, 'the change was dropped after "no"');

    answering(true);
    await press(byData('data-save')[0]);
    await act(async () => { await settled(); await settled(); });
    const puts = calls.filter((call) => call.method === 'PUT');
    assert.deepEqual(puts.map((call) => call.body), [{ offered: false }]);
    assert.equal(byData('data-save-bar').length, 0, 'the bar stayed after saving');
    assert.equal(toggle().props.checked, false);

    // Turning it on asks too, without a list of losses.
    await act(async () => { toggle().props.onChange({ currentTarget: { checked: true } }); });
    answering(true);
    await press(byData('data-save')[0]);
    assert.equal(asked[0].title, 'dbsv.onTitle');
    assert.equal(asked[0].lost, undefined);
    assert.deepEqual(calls.filter((call) => call.method === 'PUT').map((call) => call.body), [{ offered: false }, { offered: true }]);
  } finally {
    await unmount();
  }
});

// --- The account CelikPanel connects with -----------------------------------

const servers = (account) => [
  { id: 1, type_name: 'MariaDB', name: 'MariaDB', port: 3306, admin_username: account, is_local: true },
  { id: 2, type_name: 'PostgreSQL', name: 'PostgreSQL', port: 5432, admin_username: 'celikpanel_admin', is_local: true },
  { id: 3, type_name: 'MariaDB', name: 'elsewhere', port: 3306, admin_username: 'remote_admin', is_local: false },
];
const menuItem = (id) => tree.root.findAll((node) => node.type === 'button' && node.props.role === 'menuitem' && node.props['data-row-action'] === id)[0];
const openMenu = async (name) => {
  if (menuItem('show')) return;
  await press(tree.root.find((node) => node.type === 'button' && node.props['data-row-more'] === name));
};
const textOf = (node) => (typeof node === 'string' ? node : (node.children ?? []).map(textOf).join(''));

test('the account block shows only on its own engine’s page, only when that engine is here', async () => {
  globalThis.currentRole = 'admin';
  serve({ 'GET /api/v1/database-servers': json(servers('celikpanel')) });
  try {
    await mount(React.createElement(EngineAccountBlock, { engine: 'mariadb' }));
    assert.equal(byData('data-engine-account-row').length, 1, 'the other machine’s server or PostgreSQL is listed on MariaDB’s page');
    assert.ok(text().includes('dbacc.title') && text().includes('celikpanel'));
  } finally {
    await unmount();
  }
  serve({ 'GET /api/v1/database-servers': json(servers('celikpanel').filter((s) => s.id !== 2)) });
  try {
    await mount(React.createElement(EngineAccountBlock, { engine: 'postgresql' }));
    assert.equal(tree.toJSON(), null, 'a block for an engine that is not installed');
  } finally {
    await unmount();
  }
});

test('the three actions keep today’s requests; the password is shown once and gone after closing', async () => {
  globalThis.currentRole = 'admin';
  let account = 'celikpanel';
  serve({
    'GET /api/v1/database-servers': () => json(servers(account))(),
    'GET /api/v1/database-servers/1/admin-account': json({ password: 'S3cret-once' }),
    'POST /api/v1/database-servers/1/admin-account': json({ success: true }),
    'DELETE /api/v1/database-servers/1/admin-account': () => { account = ''; return json({ success: true })(); },
  });
  try {
    await mount(React.createElement(EngineAccountBlock, { engine: 'mariadb' }));
    await openMenu('celikpanel');
    assert.deepEqual(tree.root.findAll((node) => node.props.role === 'menuitem').map((node) => node.props['data-row-action']), ['show', 'rotate', 'remove']);
    await press(menuItem('show'));
    assert.ok(text().includes('S3cret-once'));
    const close = tree.root.findAll((node) => node.type === 'button' && textOf(node) === 'common.close')[0];
    await press(close);
    assert.ok(!text().includes('S3cret-once'), 'the password stays on the page after closing');

    answering(false);
    await openMenu('celikpanel');
    await press(menuItem('rotate'));
    assert.equal(asked[0].title, 'dbacc.rotateTitle');
    assert.equal(calls.filter((call) => call.method === 'POST').length, 0, 'a new password was given without asking');
    answering(true);
    await openMenu('celikpanel');
    await press(menuItem('rotate'));
    await act(async () => { await settled(); await settled(); });
    assert.equal(calls.filter((call) => call.method === 'POST' && call.url === '/api/v1/database-servers/1/admin-account').length, 1);

    answering(true);
    await openMenu('celikpanel');
    await press(menuItem('remove'));
    await act(async () => { await settled(); await settled(); });
    assert.equal(asked[0].tone, 'danger');
    assert.deepEqual(asked[0].lost, ['dbacc.removeLostAdd', 'dbacc.removeLostUsers']);
    assert.ok(asked[0].kept.includes('dbacc.removeKeptData') && asked[0].kept.includes('dbacc.removeKeptSites'));
    assert.equal(calls.filter((call) => call.method === 'DELETE').length, 1);
    assert.equal(byData('data-engine-account-row').length, 0, 'the row of the removed account is still shown');
    assert.equal(byData('data-engine-account-missing').length, 1);
  } finally {
    await unmount();
  }
});

test('while the server list is being read again the account actions are off', async () => {
  globalThis.currentRole = 'admin';
  let release = null;
  let hang = false;
  serve({
    'GET /api/v1/database-servers': () => (hang ? new Promise((resolve) => { release = () => resolve(json(servers('celikpanel'))()); }) : json(servers('celikpanel'))()),
    'POST /api/v1/database-servers/1/admin-account': json({ success: true }),
  });
  try {
    await mount(React.createElement(EngineAccountBlock, { engine: 'mariadb' }));
    hang = true;
    answering(true);
    await openMenu('celikpanel');
    await press(menuItem('rotate'));
    await openMenu('celikpanel');
    assert.deepEqual(['show', 'rotate', 'remove'].map((id) => menuItem(id).props['aria-disabled']), [true, true, true], 'actions offered on a row being read again');
    await act(async () => { release(); await settled(); await settled(); });
    await openMenu('celikpanel');
    assert.deepEqual(['show', 'rotate', 'remove'].map((id) => Boolean(menuItem(id).props['aria-disabled'])), [false, false, false]);
  } finally {
    await unmount();
  }
});

// --- The domain page's remote access leads to the new place -----------------

test('a domain’s remote access tells the administrator where the switch is and switches nothing itself', async () => {
  globalThis.currentRole = 'admin';
  serve({ 'GET /api/v1/database-remote/databases/7': json({ engine: 'mysql', access: { may_edit: false, reason: 'not_offered', grantor: '' }, rules: [], users: [], client: {}, max_addresses_user: 10, your_address: '' }) });
  try {
    await mount(React.createElement(DatabaseRemoteAccess, { db: { id: 7, name: 'shop', type: 'mysql' } }));
    await act(async () => { tree.root.find((node) => node.type === 'details').props.onToggle({ target: { open: true } }); await settled(); await settled(); });
    assert.ok(text().includes('dbr.off.admin'));
    await press(byData('data-db-remote-open-switch')[0]);
    assert.deepEqual(globalThis.navigated, ['/security#outside-connections']);
    assert.equal(calls.filter((call) => call.method !== 'GET').length, 0);
  } finally {
    await unmount();
  }
  for (const lang of ['en', 'tr']) {
    const words = source(`i18n/dbRemote/${lang}.ts`);
    assert.match(words, lang === 'tr' ? /'dbr\.off\.admin': '[^']*Güvenlik › Dışarıya açık olanlar/ : /'dbr\.off\.admin': '[^']*Security › Open to the outside/);
    assert.doesNotMatch(words, /'dbrs\.|'dbr\.turnOn'|'dbr\.turnOnConfirm'/, 'the old server block’s words are left over');
  }
});

test('the address field is sized for an address, with its choices beside it', () => {
  const card = source('components/DatabaseRemoteAccess.tsx');
  const row = card.slice(card.lastIndexOf('<div', card.indexOf('data-db-remote-add-row')), card.indexOf("{t('dbr.add')}"));
  assert.match(row, /sm:w-96/, 'the address field grows with the card again');
  assert.doesNotMatch(row, /flex-1/);
  assert.match(row, /flex flex-col gap-2 sm:flex-row/, 'on a phone the controls do not stack');
});

test('the Security page loads the card lazily, next to Fail2ban; the component pages load the account block lazily', () => {
  const page = source('components/SecurityPage.tsx');
  assert.match(page, /lazy\(\(\) => import\('\.\/DatabaseRemoteServerPanel'\)/);
  assert.ok(page.indexOf('<Fail2banStatus />') < page.indexOf('<OutsideConnectionsCard />'));
  assert.ok(page.indexOf('<OutsideConnectionsCard />') < page.indexOf('<EmergencyArea />'));
  for (const [file, engine] of [['MariaDBManagement', 'mariadb'], ['PostgreSQLManagement', 'postgresql']]) {
    const text = source(`components/${file}.tsx`);
    assert.match(text, /lazy\(\(\) => import\('\.\/DatabaseRemoteServerPanel'\)\.then\(\(module\) => \(\{ default: module\.EngineAccountBlock \}\)\)\)/);
    assert.match(text, new RegExp(`<EngineAccountBlock engine="${engine}" />`));
  }
});

// The MariaDB page no longer recommends listening on every address for
// outside connections: that opens the server to the world, and outside
// connections are managed on Security › Open to the outside.
// MariaDB sayfası dışarıdan bağlantı için bind-address = 0.0.0.0 önermez.
test('the MariaDB page gives no tip to listen on every address', () => {
  assert.doesNotMatch(source('components/MariaDBManagement.tsx'), /mariadb\.tip\.bind/);
  for (const file of ['i18n/screens/componentPages/en.ts', 'i18n/screens/componentPages/tr.ts']) {
    assert.doesNotMatch(source(file), /mariadb\.tip\.bind|bind-address\s*=\s*0\.0\.0\.0/, file);
  }
});
