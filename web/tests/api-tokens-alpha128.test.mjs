// alpha.128 (G15): the API tokens card on My account and the remote API
// switch in Settings. Mounted with the real shared reader, the real buttons
// and the real confirmation layer; what is sent is counted.
//
// Uzak API: Hesabım'daki API jetonları kartı ve Ayarlar'daki anahtar; gerçek
// okuyucu ve düğmelerle bağlanır, gönderilenler sayılır.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { enApiTokens } from '../src/i18n/apiTokens/en.ts';
import { trApiTokens } from '../src/i18n/apiTokens/tr.ts';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const placeholders = (text) => [...text.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();

test('every API token word exists in English and Turkish with the same values', () => {
  assert.deepEqual(Object.keys(trApiTokens).sort(), Object.keys(enApiTokens).sort());
  for (const [key, english] of Object.entries(enApiTokens)) {
    assert.ok(english.trim(), `${key} is empty`);
    assert.ok(trApiTokens[key].trim(), `${key} has no Turkish`);
    assert.notEqual(trApiTokens[key], english, `${key} is not translated`);
    assert.deepEqual(placeholders(trApiTokens[key]), placeholders(english), `${key}: different values`);
  }
});

test('the card and the switch load in their own chunk, never on the boot path', () => {
  const account = read('../src/components/AccountPage.tsx');
  const settings = read('../src/components/Settings.tsx');
  assert.match(account, /const ApiTokensCard = lazy\(\(\) => import\('\.\/ApiTokens'\)/);
  assert.match(settings, /const RemoteApiSwitch = lazy\(\(\) => import\('\.\/ApiTokens'\)/);
  assert.doesNotMatch(account, /from '\.\/ApiTokens'/);
  assert.doesNotMatch(settings, /from '\.\/ApiTokens'/);
  // Team members never see the card.
  assert.match(account, /\(role === 'admin' \|\| role === 'reseller' \|\| role === 'customer'\) && \(\s*<Suspense fallback=\{null\}><ApiTokensCard \/><\/Suspense>/);
});

test('making a token is a guarded request: a lost answer is asked again with the same identity', async () => {
  const identity = await import(compileSource('lib/requestIdentity.ts', () => { throw new Error('no imports'); }));
  assert.equal(identity.isReaskRoute('POST', '/api/v1/account/api-tokens'), true);
  assert.equal(identity.isReaskRoute('DELETE', '/api/v1/account/api-tokens/4'), false);
});

const fill = (text, vars = {}) => Object.entries(vars).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
globalThis.a128Role = 'customer';
const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null, Check = () => null, Copy = () => null, KeyRound = () => null, Plug = () => null, Trash2 = () => null;
  export const Loader2 = () => null, RotateCw = () => null, X = () => null, Info = () => null;
  export const useI18n = () => ({ t: (key) => key, locale: 'en' });
  export const useAuth = () => ({ role: globalThis.a128Role });
  export const Link = ({ to, children, ...rest }) => React.createElement('a', { href: to, ...rest }, children);
  export const useNavigate = () => () => true;
`);
const wordsURL = compileSource('i18n/apiTokens.ts', (specifier) => (
  specifier.endsWith('/en') ? compileSource('i18n/apiTokens/en.ts', () => '') : compileSource('i18n/apiTokens/tr.ts', () => '')
));
const shared = sharedLayer(stub, { '/i18n/apiTokens': wordsURL });
const { ApiTokensCard, RemoteApiSwitch, decodeApiTokens, refusalKey } = await import(shared.compile('components/ApiTokens.tsx'));
const say = (key, vars) => fill(enApiTokens[key], vars);

const originalFetch = globalThis.fetch;
let tree, calls;
function serve(table, changes = {}) {
  calls = [];
  globalThis.document = { addEventListener() {}, removeEventListener() {} };
  globalThis.fetch = (url, options = {}) => {
    const method = options.method || 'GET';
    calls.push({ method, url, body: options.body });
    if (method !== 'GET') {
      const change = changes[method + ' ' + url];
      return Promise.resolve().then(() => (change ? change(options) : Response.json({ error: 'no such change' }, { status: 404 })));
    }
    const answer = table[url];
    if (!answer) return Promise.resolve(Response.json({ error: 'no such route' }, { status: 404 }));
    return Promise.resolve().then(() => answer());
  };
}
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
async function mount(Component) {
  await act(async () => {
    tree = Renderer.create(React.createElement(Component));
    await settled();
    await settled();
  });
}
async function cleanup() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  delete globalThis.document;
  delete globalThis.confirm;
}
const flat = (node) => (node === null ? '' : typeof node === 'string' ? node : Array.isArray(node) ? node.map(flat).join('') : (node.children || []).map(flat).join(''));
const text = () => flat(tree.toJSON());
const instanceText = (node) => (typeof node === 'string' ? node : (node.children || []).map(instanceText).join(''));
const buttonsWith = (label) => tree.root.findAll((node) => node.type === 'button' && instanceText(node).includes(label));
const act2 = async (fn) => {
  await act(async () => {
    await fn();
    await settled();
    await settled();
  });
};
const reads = (path) => calls.filter((call) => call.method === 'GET' && call.url === path).length;

const listAnswer = (over = {}) => ({
  server_enabled: true, can_create: true, max_tokens: 20, lifetimes: [7, 30, 90, 365],
  tokens: [
    { id: 4, name: 'backup script', prefix: 'cpk_abcdefghijkl', scope: 'manage', state: 'active', created_at: 1762400000, expires_at: 1770000000, last_used_at: 1762450000, last_used_ip: '198.51.100.7' },
    { id: 3, name: 'old', prefix: 'cpk_mnopqrstuvwx', scope: 'read', state: 'ended', created_at: 1762000000, expires_at: 1769000000 },
  ],
  ...over,
});

test('decoding: an answer without its fields is not an empty list', () => {
  assert.throws(() => decodeApiTokens({ tokens: [] }));
  assert.throws(() => decodeApiTokens({ server_enabled: true, can_create: true, tokens: [{ id: 'x' }] }));
  const state = decodeApiTokens(listAnswer());
  assert.equal(state.tokens[0].scope, 'manage');
  assert.equal(state.tokens[1].state, 'ended');
  assert.equal(refusalKey(decodeApiTokens(listAnswer({ server_enabled: false, can_create: false, refused_by: 'server' })), true), 'apitok.off.admin');
  assert.equal(refusalKey(decodeApiTokens(listAnswer({ server_enabled: false, can_create: false, refused_by: 'server' })), false), 'apitok.off.other');
  assert.equal(refusalKey(decodeApiTokens(listAnswer({ can_create: false, refused_by: 'subscription' })), false), 'apitok.refused.plan');
  assert.equal(refusalKey(decodeApiTokens(listAnswer({ can_create: false, refused_by: 'impersonation' })), false), 'apitok.refused.impersonation');
  assert.equal(refusalKey(decodeApiTokens(listAnswer()), false), null);
});

test('the card lists the tokens and makes one: the token is shown once, the list reads again', async () => {
  let made = false;
  const token = 'cpk_zzzzzzzzzzzz_' + 'A'.repeat(43);
  serve({ '/api/v1/account/api-tokens': () => Response.json(made ? listAnswer({ tokens: [...listAnswer().tokens, { id: 5, name: 'ci', prefix: 'cpk_zzzzzzzzzzzz', scope: 'read', state: 'active', created_at: 1, expires_at: 2 }] }) : listAnswer()) },
    { 'POST /api/v1/account/api-tokens': (options) => { made = true; assert.deepEqual(JSON.parse(options.body), { name: 'ci', scope: 'read', expires_days: 90 }); return Response.json({ token, item: {} }, { status: 201 }); } });
  try {
    await mount(ApiTokensCard);
    const body = text();
    assert.ok(body.includes('backup script') && body.includes('cpk_abcdefghijkl…'), body);
    assert.ok(body.includes(say('apitok.state.ended')), body);
    assert.ok(body.includes('198.51.100.7'), body);
    assert.ok(!body.includes(token));
    // An empty name is refused here, nothing is sent.
    const form = tree.root.findByType('form');
    await act2(() => form.props.onSubmit({ preventDefault() {} }));
    assert.ok(text().includes(say('apitok.nameRequired')));
    assert.equal(calls.filter((call) => call.method === 'POST').length, 0);
    const input = tree.root.findByType('input');
    await act2(() => input.props.onChange({ target: { value: '  ci ' } }));
    await act2(() => tree.root.findByType('form').props.onSubmit({ preventDefault() {} }));
    assert.equal(calls.filter((call) => call.method === 'POST').length, 1);
    assert.ok(text().includes(token), 'the new token is not shown');
    assert.ok(text().includes(say('apitok.made.title')));
    assert.ok(reads('/api/v1/account/api-tokens') >= 2, 'the list did not read again');
    // "I have saved it" hides the token; it is never shown again.
    await act2(() => buttonsWith(say('apitok.made.done'))[0].props.onClick({}));
    assert.ok(!text().includes(token));
    assert.ok(text().includes('cpk_zzzzzzzzzzzz…'));
  } finally {
    await cleanup();
  }
});

test('removing asks first, sends one DELETE, says it is done and reads the list again', async () => {
  let removed = false;
  serve({ '/api/v1/account/api-tokens': () => Response.json(removed ? listAnswer({ tokens: [listAnswer().tokens[1]] }) : listAnswer()) },
    { 'DELETE /api/v1/account/api-tokens/4': () => { removed = true; return new Response(null, { status: 204 }); } });
  try {
    await mount(ApiTokensCard);
    globalThis.operationEnds = [];
    globalThis.confirm = () => false;
    await act2(() => buttonsWith(say('apitok.remove'))[0].props.onClick({}));
    assert.equal(calls.filter((call) => call.method === 'DELETE').length, 0, 'removed without a yes');
    globalThis.confirm = (message, request) => {
      assert.equal(request.tone, 'danger');
      assert.equal(request.title, say('apitok.remove.title', { name: 'backup script' }));
      return true;
    };
    await act2(() => buttonsWith(say('apitok.remove'))[0].props.onClick({}));
    assert.equal(calls.filter((call) => call.method === 'DELETE').length, 1);
    assert.deepEqual(globalThis.operationEnds.at(-1), { state: 'done', message: say('apitok.remove.done') });
    assert.ok(!text().includes('backup script'), 'the list did not refresh by itself');
  } finally {
    await cleanup();
  }
});

test('when no token can be made the card says why and who acts, and offers no form', async () => {
  for (const [answer, role, expected, link] of [
    [listAnswer({ server_enabled: false, can_create: false, refused_by: 'server' }), 'admin', say('apitok.off.admin'), true],
    [listAnswer({ server_enabled: false, can_create: false, refused_by: 'server' }), 'customer', say('apitok.off.other'), false],
    [listAnswer({ can_create: false, refused_by: 'subscription', grantor_kind: 'provider', grantor_name: 'acme' }), 'customer',
      say('apitok.refused.plan', { grantor: say('apitok.grantor.provider', { name: 'acme' }) }), false],
  ]) {
    globalThis.a128Role = role;
    serve({ '/api/v1/account/api-tokens': () => Response.json(answer) });
    try {
      await mount(ApiTokensCard);
      assert.ok(text().includes(expected), text());
      assert.equal(tree.root.findAll((node) => node.type === 'form').length, 0, 'a form was offered');
      assert.equal(tree.root.findAll((node) => node.type === 'a' && node.props.href === '/settings?section=panel#remote-api').length, link ? 1 : 0);
      // The tokens are still listed so they can be removed.
      assert.ok(text().includes('backup script'));
    } finally {
      await cleanup();
    }
  }
  globalThis.a128Role = 'customer';
});

test('a create whose answer did not arrive is unknown, not failed, and the list is read again', async () => {
  serve({ '/api/v1/account/api-tokens': () => Response.json(listAnswer()) },
    { 'POST /api/v1/account/api-tokens': () => { throw new TypeError('network'); } });
  try {
    await mount(ApiTokensCard);
    const before = reads('/api/v1/account/api-tokens');
    await act2(() => tree.root.findByType('input').props.onChange({ target: { value: 'x' } }));
    await act2(() => tree.root.findByType('form').props.onSubmit({ preventDefault() {} }));
    assert.ok(text().includes(say('apitok.unknown')), text());
    assert.ok(reads('/api/v1/account/api-tokens') > before);
  } finally {
    await cleanup();
  }
});

test('the switch: turning on asks first and says what it opens; the state reads again', async () => {
  let enabled = false;
  serve({ '/api/v1/panel/remote-api': () => Response.json({ enabled, working_tokens: enabled ? 3 : 0 }) },
    { 'PUT /api/v1/panel/remote-api': (options) => { enabled = JSON.parse(options.body).enabled; return Response.json({ enabled, working_tokens: 3 }); } });
  try {
    await mount(RemoteApiSwitch);
    assert.ok(text().includes(say('apitok.switch.off')));
    globalThis.operationEnds = [];
    globalThis.confirm = (message, request) => {
      assert.equal(message, say('apitok.switch.onMessage'));
      assert.equal(request.tone, 'default');
      return true;
    };
    await act2(() => buttonsWith(say('apitok.switch.turnOn'))[0].props.onClick({}));
    assert.equal(calls.filter((call) => call.method === 'PUT').length, 1);
    assert.ok(text().includes(say('apitok.switch.on', { n: 3 })), text());
    globalThis.confirm = (message, request) => {
      assert.equal(message, say('apitok.switch.offMessage'));
      assert.equal(request.tone, 'danger');
      return true;
    };
    await act2(() => buttonsWith(say('apitok.switch.turnOff'))[0].props.onClick({}));
    assert.ok(text().includes(say('apitok.switch.off')));
  } finally {
    await cleanup();
  }
});
