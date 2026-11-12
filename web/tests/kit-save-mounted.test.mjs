import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';

// The shared saving parts of alpha.132 (components/kit/save), mounted: the
// draft over the server's last answer, the save bar, and the question before
// leaving unsaved changes, through the real router. The confirmation is the
// fixture's host, which answers through globalThis.confirm(text, request).
// Ortak kaydetme parçaları bağlı hâlde: taslak, kaydet çubuğu ve kaydedilmemiş
// değişiklikten çıkarken sorulan soru, gerçek yönlendiriciyle.

const stub = dataModule(`
  export const useI18n = () => ({ t: (key) => key, locale: 'en' });
  export const useNavigate = () => () => {};
  export const AlertTriangle = () => null;
  export const Save = () => null;
  export const showToast = (type, message) => { (globalThis.toasts ??= []).push([type, message]); };
`);
const shared = sharedLayer(stub);
const { useDraft } = await import(shared.resolve('/draft'));
const { SaveBar, ChangedMark } = await import(shared.resolve('/SaveBar'));
const { confirmLeaveIfDirty, useLeaveGuard } = await import(shared.resolve('/leaveGuard'));
const { askToLeave } = await import(shared.resolve('/leaveAsker'));
const { saveWords } = await import(shared.resolve('/words'));
const { PasswordLevelCard } = await import(shared.compile('components/ServerSettingLimits.tsx', { '/lib/settingLimits': compileSource('lib/settingLimits.ts', (specifier) => shared.resolve(specifier)) }));
const router = await import(compileSource('router.tsx', (specifier) => (
  specifier === './router-core' ? compileSource('router-core.ts', () => stub)
    : specifier === './router-history' ? compileSource('router-history.ts', () => stub)
      : specifier.endsWith('/leaveAsker') ? shared.resolve('/leaveAsker')
        : stub
)));

const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
let asked = [];
const answering = (answer) => {
  asked = [];
  globalThis.confirm = (text, request) => { asked.push(request); return answer; };
};

// A probe: the draft and the bar as a screen uses them.
let draft;
function Probe({ known, send }) {
  draft = useDraft(known);
  return React.createElement(SaveBar, {
    count: draft.count,
    saving: draft.saving,
    onSave: () => void draft.save(send ?? (async () => 'saved')),
    onDiscard: draft.reset,
    unknown: draft.outcome === 'unknown' ? { held: draft.held, checking: false, onCheck() {} } : undefined,
  });
}
async function mounted(element, run) {
  let tree;
  await act(async () => { tree = Renderer.create(element); });
  try { await run(tree); } finally { await act(async () => tree.unmount()); }
}
const text = (tree) => JSON.stringify(tree.toJSON() ?? '');
const button = (tree, data) => tree.root.findAll((node) => node.type === 'button' && node.props[data] !== undefined)[0];

test('a draft counts what differs from the server, Discard drops it, and setting a value back is no change', async () => {
  const server = { a: 1, b: 'x' };
  await mounted(React.createElement(Probe, { known: server }), async (tree) => {
    assert.equal(tree.toJSON(), null, 'the bar is shown with nothing changed');
    await act(async () => { draft.set('a', 2); });
    assert.equal(draft.count, 1);
    assert.deepEqual(draft.values, { a: 2, b: 'x' });
    assert.ok(text(tree).includes('1 change not saved. Nothing changes until you press Save.'));
    await act(async () => { draft.set('b', 'y'); });
    assert.ok(text(tree).includes('2 changes not saved.'));
    await act(async () => { draft.set('b', 'x'); });
    assert.equal(draft.count, 1, 'a value set back to the server’s is still counted');
    await act(async () => { button(tree, 'data-save-discard').props.onClick(); });
    assert.equal(draft.count, 0);
    assert.deepEqual(draft.values, server);
  });
});

test('a newer server answer keeps the person’s edit, flags a field the server changed too, and drops an edit it now holds', async () => {
  let known = { a: 1, b: 1, c: 1 };
  const element = () => React.createElement(Probe, { known });
  await mounted(element(), async (tree) => {
    await act(async () => { draft.set('a', 2); draft.set('b', 5); });
    // The server changed a (also edited), b now holds the edit, c moved alone.
    known = { a: 3, b: 5, c: 9 };
    await act(async () => { tree.update(element()); });
    await act(async () => { await settled(); });
    assert.deepEqual(draft.values, { a: 2, b: 5, c: 9 }, 'an edit was overwritten, or an untouched field did not follow the server');
    assert.deepEqual(draft.changed, ['a']);
    assert.equal(draft.isMoved('a'), true, 'the server’s change under the edit is not flagged');
    assert.equal(draft.base.a, 3);
    // b, once held by the server, is no edit any more: a later server change shows.
    known = { a: 3, b: 7, c: 9 };
    await act(async () => { tree.update(element()); });
    assert.equal(draft.values.b, 7, 'an edit the server held came back');
    await act(async () => { draft.take('a'); });
    assert.equal(draft.count, 0);
  });
});

test('a saved draft shows what was sent until the next answer; a refused one keeps the edits', async () => {
  let reply = 'refused';
  const sent = [];
  const known = { level: 'owner' };
  await mounted(React.createElement(Probe, { known, send: async (changes) => { sent.push(changes); return reply; } }), async (tree) => {
    await act(async () => { draft.set('level', 'strong'); });
    await act(async () => { button(tree, 'data-save').props.onClick(); await settled(); });
    assert.deepEqual(sent, [{ level: 'strong' }]);
    assert.equal(draft.count, 1, 'a refused save dropped the edit');
    reply = 'saved';
    await act(async () => { button(tree, 'data-save').props.onClick(); await settled(); });
    assert.equal(draft.count, 0);
    assert.equal(draft.values.level, 'strong', 'the saved value is not shown until the server is read again');
    assert.equal(tree.toJSON(), null);
  });
});

test('a save whose answer was lost keeps the draft, says so, and holds Save until the server is read again; nothing runs twice', async () => {
  let known = { on: true };
  let finish;
  let runs = 0;
  const send = () => { runs++; return new Promise((resolve) => { finish = resolve; }); };
  const element = () => React.createElement(Probe, { known, send });
  await mounted(element(), async (tree) => {
    await act(async () => { draft.set('on', false); });
    const save = button(tree, 'data-save').props.onClick;
    await act(async () => { save(); save(); await settled(); });
    assert.equal(runs, 1, 'a second press sent a second save');
    assert.equal(button(tree, 'data-save').props.disabled, true, 'Save is not locked while saving');
    await act(async () => { finish('unknown'); await settled(); });
    assert.equal(draft.count, 1, 'the draft was dropped although the result is not known');
    assert.ok(text(tree).includes(saveWords.en.unknown.slice(0, 40)), 'the bar does not say the result is unknown');
    assert.equal(button(tree, 'data-save').props.disabled, true, 'Save is offered before the server was read again');
    assert.ok(button(tree, 'data-save-check'), 'there is no way to check again');
    // The server is read again and does not hold the change: Save is offered again.
    known = { on: true };
    await act(async () => { tree.update(element()); await settled(); });
    assert.equal(draft.held, false);
    assert.equal(button(tree, 'data-save').props.disabled, false);
    // Read again holding it: nothing is left to save and the notice goes.
    known = { on: false };
    await act(async () => { tree.update(element()); await settled(); });
    assert.equal(draft.count, 0);
    assert.equal(draft.outcome, null);
    assert.equal(tree.toJSON(), null);
  });
});

test('the changed mark says Changed, or the switch’s two states in words', async () => {
  await mounted(React.createElement('div', null, React.createElement(ChangedMark), React.createElement(ChangedMark, { switched: true })), async (tree) => {
    assert.ok(text(tree).includes('"Changed"'));
    assert.ok(text(tree).includes('Changed: off → on'));
  });
});

test('the saving words exist in both languages', () => {
  assert.deepEqual(Object.keys(saveWords.tr).sort(), Object.keys(saveWords.en).sort());
  for (const [key, value] of Object.entries(saveWords.tr)) {
    assert.ok(value.trim(), `${key} is empty in Turkish`);
    if (!['on', 'off'].includes(key)) assert.notEqual(value, saveWords.en[key], `${key} is not translated`);
  }
  assert.equal(saveWords.tr.save, 'Kaydet');
  assert.equal(saveWords.tr.discard, 'Vazgeç');
  assert.equal(saveWords.tr.leave, 'Kaydetmeden çık');
  assert.equal(saveWords.tr.stay, 'Sayfada kal');
  assert.match(saveWords.tr.pending, /^\{n\} değişiklik kaydedilmedi\. Kaydet’e basana dek hiçbir şey değişmez\.$/);
});

function Dirty({ n }) {
  useLeaveGuard(n);
  return null;
}

test('leaving asks only while something is unsaved, in the panel’s own dialog, and "leave" releases it', async () => {
  answering(true);
  assert.equal(await confirmLeaveIfDirty(), true);
  assert.equal(asked.length, 0, 'it asked with nothing unsaved');
  await mounted(React.createElement(Dirty, { n: 2 }), async () => {
    answering(false);
    assert.equal(await confirmLeaveIfDirty(), false);
    assert.equal(asked.length, 1);
    const [request] = asked;
    assert.equal(request.confirmLabel, 'Leave without saving');
    assert.equal(request.cancelLabel, 'Stay on this page');
    assert.deepEqual(request.lost, ['2 changes on this page that were not saved']);
    assert.ok(request.kept.length === 1 && request.tone === 'danger' && !request.action);
    answering(true);
    assert.equal(await confirmLeaveIfDirty(), true);
    assert.equal(askToLeave(), null, 'the changes given up are asked about again');
  });
  assert.equal(askToLeave(), null);
});

// --- The router ----------------------------------------------------------------------

// A browser history: entries, a cursor, pushState/replaceState and an
// asynchronous go() that raises popstate, as a browser does.
function fakeBrowser(start) {
  const entries = [{ url: start, state: null }];
  let at = 0;
  const listeners = [];
  const location = {};
  const sync = () => {
    const url = new URL(entries[at].url, 'https://panel.test');
    Object.assign(location, { pathname: url.pathname, search: url.search, hash: url.hash, href: url.href, origin: url.origin });
  };
  sync();
  const history = {
    get state() { return entries[at].state; },
    pushState(state, _title, url) { entries.splice(at + 1); entries.push({ url, state }); at++; sync(); },
    replaceState(state, _title, url) { entries[at] = { url: url ?? entries[at].url, state }; sync(); },
    go(delta) {
      setTimeout(() => {
        at = Math.min(entries.length - 1, Math.max(0, at + delta));
        sync();
        for (const listener of listeners) listener({});
      }, 0);
    },
  };
  globalThis.window = {
    location, history,
    addEventListener: (type, listener) => { if (type === 'popstate') listeners.push(listener); },
    removeEventListener: (type, listener) => { if (type === 'popstate') listeners.splice(listeners.indexOf(listener), 1); },
  };
  return { url: () => entries[at].url };
}

let navigate;
let shownPath;
function Page({ dirty, blocked }) {
  navigate = router.useNavigate();
  shownPath = router.useLocation().pathname;
  useLeaveGuard(dirty ? 1 : 0);
  router.useNavigationBlocker(blocked);
  return null;
}
const app = (props) => React.createElement(router.BrowserRouter, null, React.createElement(Page, props));
const wait = async () => { await act(async () => { for (let i = 0; i < 5; i++) await settled(); }); };

test('a link with unsaved changes asks; "stay" stays, "leave" goes', async () => {
  const browser = fakeBrowser('/domains/1');
  const blocked = { current: false };
  await mounted(app({ dirty: true, blocked }), async () => {
    answering(false);
    let went;
    await act(async () => { went = navigate('/domains'); });
    await wait();
    assert.equal(went, false);
    assert.equal(asked.length, 1, 'leaving unsaved changes did not ask');
    assert.equal(browser.url(), '/domains/1');
    answering(true);
    await act(async () => { navigate('/domains'); });
    await wait();
    assert.equal(browser.url(), '/domains');
    assert.equal(shownPath, '/domains');
  });
});

test('Back with unsaved changes returns to the page, asks, and goes back only on "leave"', async () => {
  const browser = fakeBrowser('/domains');
  const blocked = { current: false };
  await mounted(app({ dirty: false, blocked }), async (tree) => {
    await act(async () => { navigate('/domains/1'); });
    await act(async () => { tree.update(app({ dirty: true, blocked })); });
    answering(false);
    window.history.go(-1);
    await wait();
    assert.equal(asked.length, 1, 'Back did not ask');
    assert.equal(browser.url(), '/domains/1', 'Back left the unsaved page although the person stayed');
    assert.equal(shownPath, '/domains/1');
    answering(true);
    window.history.go(-1);
    await wait();
    assert.equal(asked.length, 1);
    assert.equal(browser.url(), '/domains');
    assert.equal(shownPath, '/domains');
  });
});

test('a running operation still refuses without asking, and a page with nothing unsaved goes at once', async () => {
  const browser = fakeBrowser('/system');
  const blocked = { current: true };
  await mounted(app({ dirty: true, blocked }), async (tree) => {
    answering(true);
    let went;
    await act(async () => { went = navigate('/domains'); });
    await wait();
    assert.equal(went, false);
    assert.equal(asked.length, 0, 'a running operation’s blocker asked instead of refusing');
    assert.equal(browser.url(), '/system');
    blocked.current = false;
    await act(async () => { tree.update(app({ dirty: false, blocked })); });
    await act(async () => { went = navigate('/domains'); });
    assert.equal(went, true);
    assert.equal(asked.length, 0);
    assert.equal(browser.url(), '/domains');
  });
});

// --- A conversion: the password level (alpha.132) ------------------------------------

test('the password level is saved by Save, not by choosing it', async () => {
  let level = 'owner';
  const calls = [];
  globalThis.fetch = async (url, init = {}) => {
    calls.push(`${init.method ?? 'GET'} ${url}`);
    if (init.method === 'PUT') level = JSON.parse(init.body).level;
    return new Response(JSON.stringify({ level }), { status: 200, headers: { 'Content-Type': 'application/json' } });
  };
  await mounted(React.createElement(PasswordLevelCard), async (tree) => {
    await act(async () => { await settled(); });
    const radio = (value) => tree.root.findAll((node) => node.type === 'input' && node.props.type === 'radio')[['owner', 'strong', 'very_strong'].indexOf(value)];
    assert.equal(radio('owner').props.checked, true);
    await act(async () => { radio('strong').props.onChange(); });
    assert.deepEqual(calls.filter((call) => !call.startsWith('GET')), [], 'choosing a level saved it');
    assert.equal(radio('strong').props.checked, true);
    assert.equal(tree.root.findAll((node) => node.props['data-changed-mark'] !== undefined).length, 1, 'the choice is not marked');
    await act(async () => { button(tree, 'data-save').props.onClick(); await settled(); await settled(); });
    assert.deepEqual(calls.filter((call) => !call.startsWith('GET')), ['PUT /api/v1/rights/password-level']);
    assert.equal(radio('strong').props.checked, true);
    assert.equal(button(tree, 'data-save'), undefined, 'the save bar stayed after the level was saved');
  });
  delete globalThis.fetch;
});
