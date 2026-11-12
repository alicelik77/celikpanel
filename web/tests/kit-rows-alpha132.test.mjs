// alpha.132 shared kit, mounted and as contracts: a list row's actions as
// words with one "More" menu (keyboard, screen reader, placement), the kinds
// and states in words (never a raw code), the one guidance block, and a
// refusal whose only words are the server's own shown under a closed
// "Details" (P3, P5, D-024).
//
// alpha.132 ortak parçaları: satır işlemleri ve "Diğer" menüsü, tür ve durum
// sözcükleri, tek yol gösterme kutusu, sunucunun ham metni "Ayrıntı" altında.
import assert from 'node:assert/strict';
import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { kitModules } from './fixtures/kit.mjs';
import { apiErrorURL, dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { enScreens, trScreens } from './locale-catalogue.mjs';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const words = enScreens;
const fill = (text, vars) => String(text).replace(/\{(\w+)\}/g, (all, name) => vars?.[name] ?? all);
const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null;
  const words = ${JSON.stringify(words)};
  export const useI18n = () => ({ t: (key, vars) => words[key] === undefined ? key : String(words[key]).replace(/\\{(\\w+)\\}/g, (all, name) => vars?.[name] ?? all), locale: 'en' });
  export const useNavigate = () => () => {};
`);
const shared = sharedLayer(stub);
const ui = await import(shared.uiURL);
const kit = kitModules(stub, shared.uiURL);
const { RowActions } = await import(kit.rowActions);
const { Guidance } = await import(kit.guidance);
const { StateLabel } = await import(kit.stateLabel);
const wordsLib = await import(kit.words);
const apiError = await import(apiErrorURL);
const t = (key, vars) => (words[key] === undefined ? key : fill(words[key], vars));
const tr = (key, vars) => (trScreens[key] === undefined ? key : fill(trScreens[key], vars));

const textOf = (node) => node === null || node === undefined || typeof node === 'boolean' ? ''
  : typeof node === 'string' || typeof node === 'number' ? String(node)
    : Array.isArray(node) ? node.map(textOf).join('') : textOf(node.children);

// --- Kinds and states in words ---------------------------------------------------------------

test('every kind the server stores has a word in both languages; an unknown kind is never printed raw to a customer', () => {
  // The agent's own list of the kinds a site can have (cmd/agent/vhost_rpc.go).
  const agent = read('../../cmd/agent/vhost_rpc.go');
  const kinds = /case ("php"(?:, "[a-z_]+")+):/.exec(agent)[1].split(', ').map((item) => item.slice(1, -1));
  assert.deepEqual(kinds, ['php', 'static', 'node', 'proxy', 'forwarding', 'service_app', 'runtime_app']);
  for (const code of [...kinds, 'dnsonly']) {
    for (const say of [t, tr]) {
      const word = wordsLib.siteKindWord(say, code);
      assert.ok(word && !word.includes('kind.') && word !== code, `${code} has no word`);
    }
  }
  assert.equal(wordsLib.siteKindWord(tr, 'static'), 'Statik site');
  assert.equal(wordsLib.siteKindWord(tr, 'proxy'), 'Başka sunucudan gösterilen site');
  assert.equal(wordsLib.siteKindWord(tr, undefined), 'PHP sitesi', 'a missing kind is PHP, as the server reads it');
  assert.equal(wordsLib.siteKindWord(tr, 'runtime_app', { runtimeKind: 'python' }), 'Python uygulaması');
  assert.equal(wordsLib.siteKindWord(tr, 'runtime_app', { runtimeKind: 'program' }), 'Program');
  assert.equal(wordsLib.siteKindWord(tr, 'runtime_app'), 'Program ya da Python uygulaması');
  assert.equal(wordsLib.siteKindWord(tr, 'quantum_site'), 'Başka bir tür');
  assert.equal(wordsLib.siteKindWord(tr, 'quantum_site', { admin: true }), 'Başka bir tür (quantum_site)');
});

test('every domain state the schema allows has a word and a tone; an unknown one is a neutral word', () => {
  const schema = read('../../internal/db/migrations/001_full_schema.sql');
  assert.match(schema, /status TEXT DEFAULT 'active' CHECK\(status IN \('active', 'suspended', 'pending'\)\)/);
  assert.equal(wordsLib.stateWord(tr, 'active'), 'Aktif');
  assert.equal(wordsLib.stateWord(tr, 'suspended'), 'Askıya alındı');
  assert.equal(wordsLib.stateWord(tr, 'pending'), 'Silinmeyi bekliyor');
  assert.equal(wordsLib.stateWord(tr, 'frozen'), 'Durumu bilinmiyor');
  assert.equal(wordsLib.stateWord(t, 'frozen', { admin: true }), 'State not known (frozen)');
  assert.deepEqual(['active', 'suspended', 'pending', 'frozen'].map(wordsLib.stateTone), ['success', 'danger', 'unknown', 'unknown']);
  assert.equal(wordsLib.whoWord(tr, 'administrator'), 'Sunucu yöneticisi');
  const label = Renderer.create(React.createElement(StateLabel, { status: 'suspended' }));
  assert.equal(textOf(label.toJSON()), 'Suspended');
  assert.equal(label.root.findByProps({ 'data-tone': 'danger' }).type, 'span');
});

test('the kit speaks both languages with the same placeholders', () => {
  const keys = Object.keys(enScreens).filter((key) => /^(kind|state|kit)\./.test(key));
  assert.ok(keys.length >= 26, `only ${keys.length} kit keys`);
  for (const key of keys) {
    assert.ok(trScreens[key], `${key} has no Turkish`);
    assert.deepEqual(trScreens[key].match(/\{\w+\}/g) ?? [], enScreens[key].match(/\{\w+\}/g) ?? [], key);
  }
  assert.equal(trScreens['problem.details'], 'Ayrıntı');
});

// --- Row actions ------------------------------------------------------------------------------

// react-test-renderer has no DOM: each host node gets a small stand-in that
// can take the focus and has a box, and the document keeps its listeners.
let focused = null;
let listeners = [];
let windowListeners = [];
const nodeMock = (element) => ({
  focus() { focused = element.props['data-row-action'] ?? element.props['data-row-more'] ?? element.type; },
  getBoundingClientRect: () => ({ top: globalThis.anchorTop ?? 100, bottom: (globalThis.anchorTop ?? 100) + 32, left: 900, right: 1000 }),
  offsetWidth: 200,
  offsetHeight: 160,
  contains: () => false,
});
function stage() {
  focused = null;
  listeners = [];
  windowListeners = [];
  globalThis.window = {
    innerHeight: 900, innerWidth: 1440,
    addEventListener: (type, listener) => windowListeners.push([type, listener]),
    removeEventListener: (type, listener) => { windowListeners = windowListeners.filter((item) => item[1] !== listener); },
  };
  globalThis.document = {
    get activeElement() { return null; },
    addEventListener: (type, listener) => listeners.push([type, listener]),
    removeEventListener: (type, listener) => { listeners = listeners.filter((item) => item[1] !== listener); },
    body: {},
  };
}
function unstage() {
  delete globalThis.window;
  delete globalThis.document;
  delete globalThis.anchorTop;
}

const ran = [];
const rowProps = () => ({
  name: 'kahvedukkani.dr.test',
  actions: [
    { id: 'manage', label: 'Manage', onSelect: () => ran.push('manage') },
    { id: 'visit', label: 'Open site', href: 'https://kahvedukkani.dr.test' },
  ],
  more: [
    { id: 'delete', label: 'Delete', danger: true, onSelect: () => ran.push('delete') },
    { id: 'copy', label: 'Copy', onSelect: () => ran.push('copy') },
    { id: 'export', label: 'Export', disabled: true, disabledReason: 'Nothing to export yet', onSelect: () => ran.push('export') },
  ],
});
const find = (tree, predicate) => tree.root.findAll((node) => typeof node.type === 'string' && predicate(node));
const moreButton = (tree) => find(tree, (node) => node.props['data-row-more'] !== undefined)[0];
const menu = (tree) => find(tree, (node) => node.props.role === 'menu')[0];
const items = (tree) => find(tree, (node) => node.props.role === 'menuitem');

test('a row shows at most two actions as words and the rest in a More menu, Delete last in red after a line', async () => {
  stage();
  ran.length = 0;
  let tree;
  try {
    await act(async () => { tree = Renderer.create(React.createElement(RowActions, rowProps()), { createNodeMock: nodeMock }); });
    const visible = find(tree, (node) => node.props['data-row-action'] !== undefined);
    assert.deepEqual(visible.map((node) => [node.type, textOf(node)]), [['button', 'Manage'], ['a', 'Open site']], 'two word actions, the link a real link');
    assert.equal(visible[1].props.target, '_blank');
    const more = moreButton(tree);
    assert.equal(textOf(more), 'More');
    assert.equal(more.props['aria-label'], 'More: kahvedukkani.dr.test', 'the menu names its row and keeps its visible word');
    assert.equal(more.props['aria-haspopup'], 'menu');
    assert.equal(more.props['aria-expanded'], false);
    assert.equal(menu(tree), undefined, 'the menu is closed until asked');

    await act(async () => { more.props.onClick({}); });
    assert.equal(moreButton(tree).props['aria-expanded'], true);
    const list = menu(tree);
    assert.equal(moreButton(tree).props['aria-controls'], list.props.id);
    assert.deepEqual(items(tree).map((node) => node.props['data-row-action']), ['copy', 'export', 'delete'], 'the dangerous item is not last');
    const children = list.children.filter((node) => typeof node !== 'string');
    assert.equal(children.at(-2).props.role, 'separator', 'no line before Delete');
    assert.match(items(tree).at(-1).props.className, /text-danger/);
    assert.equal(focused, 'copy', 'the first item does not take the focus');
    assert.equal(list.props.style.visibility, 'visible');
    assert.deepEqual([list.props.style.top, list.props.style.left], [136, 800], 'not placed under the button, on its right edge');

    // A disabled item stays readable with its reason and does nothing.
    const off = items(tree)[1];
    assert.equal(off.props['aria-disabled'], true);
    assert.ok(textOf(off).includes('Nothing to export yet'));
    await act(async () => { off.props.onClick(); });
    assert.deepEqual(ran, []);
    assert.ok(menu(tree), 'a disabled item closed the menu');

    await act(async () => { items(tree).at(-1).props.onClick(); });
    assert.deepEqual(ran, ['delete']);
    assert.equal(menu(tree), undefined, 'choosing an item leaves the menu open');
    assert.equal(focused, 'kahvedukkani.dr.test', 'the focus does not go back to More');
    assert.deepEqual(listeners, [], 'the closed menu still listens');
  } finally {
    await act(async () => tree.unmount());
    unstage();
  }
});

test('the More menu works from the keyboard and closes on Escape, Tab, a click outside, scroll and resize', async () => {
  stage();
  let tree;
  const key = (name) => ({ key: name, preventDefault() {} });
  try {
    await act(async () => { tree = Renderer.create(React.createElement(RowActions, rowProps()), { createNodeMock: nodeMock }); });
    // ArrowUp on the button opens the menu at its last item.
    await act(async () => { moreButton(tree).props.onKeyDown(key('ArrowUp')); });
    assert.equal(focused, 'delete');
    await act(async () => { menu(tree).props.onKeyDown(key('Escape')); });
    assert.equal(menu(tree), undefined);
    assert.equal(focused, 'kahvedukkani.dr.test', 'Escape does not give the focus back');

    await act(async () => { moreButton(tree).props.onKeyDown(key('ArrowDown')); });
    assert.equal(focused, 'copy');
    await act(async () => { menu(tree).props.onKeyDown(key('Tab')); });
    assert.equal(menu(tree), undefined, 'Tab leaves the menu open');

    for (const [where, type] of [['document', 'pointerdown'], ['document', 'scroll'], ['window', 'resize']]) {
      await act(async () => { moreButton(tree).props.onClick({}); });
      assert.ok(menu(tree));
      const pool = where === 'document' ? listeners : windowListeners;
      const entry = pool.find((item) => item[0] === type);
      assert.ok(entry, `no ${type} listener`);
      await act(async () => { entry[1]({ target: {} }); });
      assert.equal(menu(tree), undefined, `${type} leaves the menu open`);
    }
  } finally {
    await act(async () => tree.unmount());
    unstage();
  }
});

test('near the page bottom the menu opens upward, and a row without rare actions has no More', async () => {
  stage();
  globalThis.anchorTop = 820;
  let tree;
  try {
    await act(async () => { tree = Renderer.create(React.createElement(RowActions, rowProps()), { createNodeMock: nodeMock }); });
    await act(async () => { moreButton(tree).props.onClick({}); });
    assert.equal(menu(tree).props.style.top, 820 - 4 - 160, 'the menu runs off the bottom of the window');
    await act(async () => tree.update(React.createElement(RowActions, { ...rowProps(), more: [] })));
    assert.equal(moreButton(tree), undefined);
  } finally {
    await act(async () => tree.unmount());
    unstage();
  }
});

// --- The guidance block -----------------------------------------------------------------------

test('the guidance block says Why, Who acts, Next step and Then in that order, with its one action', async () => {
  let tree;
  await act(async () => {
    tree = Renderer.create(React.createElement(Guidance, {
      tone: 'waiting', title: 'shop.example is not pointing here yet', why: 'The name points elsewhere.', who: 'You, at your registrar.',
      next: 'Change the A record.', then: 'This page checks by itself.', action: React.createElement('button', null, 'Show what to write'),
      'data-case': 'kept',
    }));
  });
  const root = tree.root.find((node) => node.type === 'div' && node.props['data-case'] === 'kept');
  assert.equal(root.props['data-guidance'], 'waiting');
  assert.equal(root.props.role, undefined, 'the whole block is a live region');
  const live = tree.root.findByProps({ 'data-guidance-live': 'status' });
  assert.equal(live.props.role, 'status');
  assert.equal(textOf(live), 'shop.example is not pointing here yet');
  const labels = tree.root.findAllByType('dt').map(textOf);
  assert.deepEqual(labels, ['Why', 'Who acts', 'Next step', 'Then']);
  assert.ok(textOf(tree.toJSON()).includes('Show what to write'));
  await act(async () => tree.update(React.createElement(Guidance, { tone: 'failed', title: 'The scan failed', who: wordsLib.whoWord(t, 'you') })));
  assert.equal(tree.root.findByProps({ 'data-guidance-live': 'alert' }).props.role, 'alert', 'a verified failure is not an alert');
  assert.deepEqual(tree.root.findAllByType('dt').map(textOf), ['Who acts'], 'an empty row is drawn');
  await act(async () => tree.update(React.createElement(Guidance, { tone: 'neutral', title: 'Lead', live: 'off' })));
  assert.equal(tree.root.findByProps({ 'data-guidance-live': 'off' }).props.role, undefined);
  await act(async () => tree.unmount());
  // Turkish labels as drawn (X16).
  assert.deepEqual(['why', 'who', 'next', 'then'].map((part) => trScreens[`kit.guidance.${part}`]), ['Neden', 'Kim yapar', 'Sonraki adım', 'Sonra']);
});

// --- A refusal in the server's own words ------------------------------------------------------

async function banner(error, { admin = false, sentence } = {}) {
  let tree;
  await act(async () => {
    tree = Renderer.create(React.createElement(ui.ServerDetailAccess.Provider, { value: admin }, React.createElement(ui.ErrorBanner, { error, sentence })));
  });
  const details = tree.root.findAll((node) => node.type === 'details');
  const result = { text: textOf(tree.toJSON()), main: textOf(tree.root.findAllByType('span')[0]), details: details.length ? textOf(details[0]) : null };
  await act(async () => tree.unmount());
  return result;
}

test('a refusal whose only words are the server\'s is said plainly, with them under a closed Details', async () => {
  const plain = await banner({ message: 'quota reached for this plan' }, { sentence: 'domains.deleteFailed' });
  assert.equal(plain.main, t('domains.deleteFailed'), 'the server text is still inline');
  assert.ok(plain.details.includes('quota reached for this plan'), 'the server text is lost');
  const generic = await banner({ message: 'upstream answered 502' });
  assert.equal(generic.main, t('kit.detail.generic'));
  assert.ok(generic.details.includes('upstream answered 502'));
  // A customer does not read server internals; they are told who can.
  for (const message of ['open /etc/nginx/sites-enabled/a.conf: permission denied', 'celikpanel-agent.service failed', 'run sudo systemctl restart nginx', 'try `apt install x`']) {
    const customer = await banner({ message }, { sentence: 'domains.deleteFailed' });
    assert.equal(customer.details, null, `a customer reads: ${message}`);
    assert.ok(customer.text.includes(t('kit.detail.adminOnly')));
    const admin = await banner({ message }, { sentence: 'domains.deleteFailed', admin: true });
    assert.ok(admin.details.includes(message), `the administrator lost: ${message}`);
    assert.ok(!admin.text.includes(t('kit.detail.adminOnly')));
  }
  // A coded refusal with words of its own says them; the code alone is under Details.
  const coded = await banner({ message: 'raw english', code: 'SITE_HAS_NO_PHP', vars: { domain: 'a.test' } }, { sentence: 'domains.deleteFailed' });
  assert.equal(coded.main, t('err.SITE_HAS_NO_PHP', { domain: 'a.test' }));
  assert.equal(coded.details, `${t('problem.details')}SITE_HAS_NO_PHP`);
  assert.ok(!coded.text.includes('raw english'));
  // An address is not a path.
  assert.equal(apiError.serverTextForEveryone('could not reach https://example.com/health'), true);
  assert.equal(apiError.serverTextForEveryone('disk full on /var'), false);
  // The one-string form keeps the reason beside the sentence (toasts, progress windows).
  assert.equal(apiError.refusalText({ message: 'quota reached' }, t, 'users.createFailed'), `${t('users.createFailed')}. The server said: quota reached`);
});

// --- No leftovers -----------------------------------------------------------------------------

test('the replaced pieces are gone: icon-only row actions, the old guidance block and its copies', () => {
  const src = new URL('../src/', import.meta.url);
  const files = [];
  const walk = (dir) => {
    for (const name of readdirSync(dir)) {
      const path = new URL(name, dir);
      if (statSync(path).isDirectory()) walk(new URL(name + '/', dir));
      else if (/\.(ts|tsx)$/.test(name)) files.push([path.pathname.slice(src.pathname.length), readFileSync(path, 'utf8')]);
    }
  };
  walk(src);
  assert.equal(existsSync(new URL('components/SiteToolGuidance.tsx', src)), false);
  for (const [file, text] of files) {
    assert.doesNotMatch(text, /\bIconAction\b|SiteToolGuidance|SetupLead\b|wpt\.guide\.|wpt\.who\./, file);
    // The hand-copied why/who/next/then list lives on only in the domain page's
    // attention box, which the domain-page lane converts (alpha.132).
    if (file !== 'components/DomainDetail.tsx' && !file.startsWith('i18n/')) assert.doesNotMatch(text, /setup\.v2\.(why|who|next|then)'/, file);
  }
  const domains = read('../src/components/Domains.tsx');
  assert.doesNotMatch(domains, /^\s*\{d\.project_type \|\| 'php'\}\s*$|: d\.status\}/m, 'the list prints a raw code');
  assert.match(domains, /siteKindWord\(t, d\.project_type/);
  assert.match(domains, /<StateLabel status=\{d\.status\}/);
});
