// The PHP version card of a PHP tool's Manage window, mounted (alpha.104,
// owner decision of 2026-10-23). Webmail (Roundcube), phpMyAdmin and
// phpPgAdmin each run on one installed PHP-FPM version; the card says which,
// says when that PHP is missing, offers only installed versions, moves the
// tool through the Panel with visible waiting and says the result: moved and
// answered, put back on the previous version with the reason, or not known.
// Bir PHP aracının Yönet penceresindeki PHP sürümü kartı, bağlanmış olarak.
import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { enComponentsScreens } from '../src/i18n/screens/components/en.ts';
import { trComponentsScreens } from '../src/i18n/screens/components/tr.ts';
import { enComponentPagesScreens } from '../src/i18n/screens/componentPages/en.ts';
import { trComponentPagesScreens } from '../src/i18n/screens/componentPages/tr.ts';
// The components and component-page parts (split in alpha.106), as one.
const en = { ...enComponentsScreens, ...enComponentPagesScreens };
const tr = { ...trComponentsScreens, ...trComponentPagesScreens };

const fill = (text, vars = {}) => Object.entries(vars).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
globalThis.cardWords = en;
const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null, Code2 = () => null;
  const fill = (text, vars) => Object.entries(vars || {}).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
  const i18n = { t: (key, vars) => (globalThis.cardWords?.[key] !== undefined ? fill(globalThis.cardWords[key], vars) : key), locale: 'en' };
  export const useI18n = () => i18n;
  export const useNavigate = () => () => true;
`);
const shared = sharedLayer(stub);
const { PHPToolVersion } = await import(shared.compile('components/PHPToolVersion.tsx', {
  '/lib/componentName': shared.compile('lib/componentName.ts'),
}));

const read = (tools, versions = ['8.6', '8.5', '8.4']) => ({
  state: 'read',
  tools,
  versions: versions.map((version) => ({ version, sites: 0, site_names: [], tools: [] })),
});
const tool = (extra) => ({ tool: 'phppgadmin', service_id: 'phppgadmin', installed: true, version: '8.4', ready: true, version_installed: true, ...extra });

const originalFetch = globalThis.fetch;
let tree;
let requests = [];
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
async function settle() { await act(async () => { await new Promise((resolve) => setTimeout(resolve, 10)); }); }
async function mount({ reads, bind, words = en }) {
  globalThis.cardWords = words;
  requests = [];
  let readCount = 0;
  globalThis.fetch = async (input, init = {}) => {
    const url = String(input);
    requests.push({ url, method: init.method ?? 'GET', body: init.body });
    if (url === '/api/v1/service/php-tools') {
      const next = reads[Math.min(readCount, reads.length - 1)];
      readCount += 1;
      return next === 'fail' ? answer({ error: 'no' }, 502) : answer(next);
    }
    if (url === '/api/v1/service/php-tools/bind') return bind(init);
    return answer({ error: 'no route' }, 404);
  };
  await act(async () => { tree = Renderer.create(React.createElement(PHPToolVersion, { serviceId: 'phppgadmin', tool: 'phppgadmin' })); });
  await settle();
}
async function unmount() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
}
const label = (node) => { const walk = (n) => (typeof n === 'string' ? n : (n.children || []).map(walk).join('')); return walk(node); };
const attr = (name) => tree.root.findAll((node) => node.props && node.props[name] !== undefined)[0];
const say = (key, vars) => fill(en[key], vars);

test('the current version, and only the other installed versions are offered', async () => {
  await mount({ reads: [read([tool({})])] });
  try {
    assert.equal(attr('data-php-tool-state').props['data-php-tool-state'], 'ready');
    assert.equal(label(attr('data-php-tool-state')), say('component.phpTool.current', { tool: 'phpPgAdmin', version: '8.4' }));
    assert.deepEqual(attr('data-php-tool-select').props.children.map((option) => option.props.value), ['8.6', '8.5']);
    assert.equal(requests.filter((request) => request.method !== 'GET').length, 0, 'reading the card changed something');
  } finally { await unmount(); }
});

test('a PHP that is gone or stopped is said, and the bound version is offered again only when it is not running', async () => {
  await mount({ reads: [read([tool({ ready: false, version_installed: false })], ['8.6', '8.5'])] });
  try {
    assert.equal(label(attr('data-php-tool-state')), say('component.phpTool.gone', { tool: 'phpPgAdmin', version: '8.4' }));
    assert.deepEqual(attr('data-php-tool-select').props.children.map((option) => option.props.value), ['8.6', '8.5']);
  } finally { await unmount(); }
  await mount({ reads: [read([tool({ ready: false })])] });
  try {
    assert.equal(label(attr('data-php-tool-state')), say('component.phpTool.stopped', { tool: 'phpPgAdmin', version: '8.4' }));
  } finally { await unmount(); }
});

test('moving: the request, visible waiting, then the result; a failed proof says it was put back', async () => {
  let release;
  const held = new Promise((resolve) => { release = resolve; });
  await mount({
    reads: [read([tool({})]), read([tool({ version: '8.6' })])],
    bind: async () => { await held; return answer({ bound: true, tool: 'phppgadmin', version: '8.6', previous: '8.4' }); },
  });
  try {
    const button = tree.root.findAll((node) => node.type === 'button' && label(node).includes(say('component.phpTool.apply', { version: '8.6' })))[0];
    await act(async () => { button.props.onClick(); });
    await settle();
    // While it runs the card says what happens and that nothing is needed.
    assert.ok(label(tree.root).includes(say('component.phpTool.movingWait', { tool: 'phpPgAdmin', version: '8.6' })));
    const sent = requests.find((request) => request.url === '/api/v1/service/php-tools/bind');
    assert.deepEqual(JSON.parse(sent.body), { tool: 'phppgadmin', version: '8.6' });
    await act(async () => { release(); });
    await settle();
    assert.equal(label(attr('data-php-tool-result')), say('component.phpTool.done', { tool: 'phpPgAdmin', version: '8.6' }));
    assert.equal(label(attr('data-php-tool-state')), say('component.phpTool.current', { tool: 'phpPgAdmin', version: '8.6' }));
  } finally { await unmount(); }

  await mount({
    reads: [read([tool({})])],
    bind: async () => answer({ error: 'phpPgAdmin did not work on PHP 8.6 (phpPgAdmin answered HTTP 502); it was put back on PHP 8.4', code: 'PHP_TOOL_PROBE_FAILED', restored: true, previous: '8.4',
      cause: { kind: 'http_status', version: '8.6', status: 502, detail: 'phpPgAdmin answered HTTP 502' } }, 409),
  });
  try {
    const button = tree.root.findAll((node) => node.type === 'button' && label(node).includes(say('component.phpTool.apply', { version: '8.6' })))[0];
    await act(async () => { button.props.onClick(); });
    await settle();
    assert.equal(attr('data-php-tool-result').props['data-php-tool-result'], 'restored');
    // The cause is said in the reader's language, from its facts.
    assert.equal(label(attr('data-php-tool-result')), say('component.phpTool.restored', {
      tool: 'phpPgAdmin', version: '8.6', previous: '8.4', reason: say('component.phpTool.cause.http_status', { status: 502 }),
    }));
  } finally { await unmount(); }

  // No structured cause (or one this screen does not know): quoted, not guessed.
  await mount({
    reads: [read([tool({})])],
    bind: async () => answer({ error: 'something new happened', code: 'PHP_TOOL_PROBE_FAILED', restored: true, previous: '8.4', cause: { kind: 'brand_new_kind', detail: 'something new happened' } }, 409),
  });
  try {
    const button = tree.root.findAll((node) => node.type === 'button' && label(node).includes(say('component.phpTool.apply', { version: '8.6' })))[0];
    await act(async () => { button.props.onClick(); });
    await settle();
    assert.equal(label(attr('data-php-tool-result')), say('component.phpTool.restored', {
      tool: 'phpPgAdmin', version: '8.6', previous: '8.4', reason: say('component.phpTool.cause.other', { detail: 'something new happened' }),
    }));
  } finally { await unmount(); }
});

test('Turkish: a missing-extensions cause is said in Turkish, not in the Agent’s English', async () => {
  await mount({
    words: tr,
    reads: [read([tool({})])],
    bind: async () => answer({ error: 'phpPgAdmin did not work on PHP 8.6 (PHP 8.6 lacks pgsql for phpPgAdmin); it was put back on PHP 8.4', code: 'PHP_TOOL_PROBE_FAILED', restored: true, previous: '8.4',
      cause: { kind: 'missing_extensions', version: '8.6', extensions: ['pgsql', 'mbstring'] } }, 409),
  });
  try {
    const button = tree.root.findAll((node) => node.type === 'button' && label(node).includes(fill(tr['component.phpTool.apply'], { version: '8.6' })))[0];
    await act(async () => { button.props.onClick(); });
    await settle();
    const text = label(attr('data-php-tool-result'));
    assert.ok(text.includes(fill(tr['component.phpTool.cause.missing_extensions'], { version: '8.6', extensions: 'pgsql, mbstring' })), text);
    assert.ok(!text.includes('lacks'), text);
  } finally { await unmount(); }
});

test('a tool that could not be put back says so with the server’s words', async () => {
  await mount({
    reads: [read([tool({})])],
    bind: async () => answer({ error: 'phpPgAdmin did not work on PHP 8.6 (down); the previous configuration was put back, but nginx rejected it (emerg)', code: 'PHP_TOOL_PROBE_FAILED',
      restored: false, mutation_applied: true, previous: '8.4', cause: { kind: 'no_answer', version: '8.6' } }, 409),
  });
  try {
    const button = tree.root.findAll((node) => node.type === 'button' && label(node).includes(say('component.phpTool.apply', { version: '8.6' })))[0];
    await act(async () => { button.props.onClick(); });
    await settle();
    assert.equal(attr('data-php-tool-result').props['data-php-tool-result'], 'failed');
    const text = label(attr('data-php-tool-result'));
    assert.ok(text.includes(say('component.phpTool.cause.no_answer', { version: '8.6' })), text);
    assert.ok(text.includes('the previous configuration was put back, but nginx rejected it'), text);
  } finally { await unmount(); }
});

test('extension packages missing on the bound version: not ready, and that version is offered again', async () => {
  await mount({ reads: [read([tool({ extensions_present: false })])] });
  try {
    assert.equal(attr('data-php-tool-state').props['data-php-tool-state'], 'extensions');
    assert.equal(label(attr('data-php-tool-state')), say('component.phpTool.missingExtensions', { tool: 'phpPgAdmin', version: '8.4' }));
    assert.deepEqual(attr('data-php-tool-select').props.children.map((option) => option.props.value), ['8.6', '8.5', '8.4']);
    assert.equal(attr('data-php-tool-select').props.value, '8.4', 'the version the sentence names is not the one chosen');
  } finally { await unmount(); }
});

test('a lost answer is read back and never sent again; a failed read says so with Retry', async () => {
  // No answer at all, and a gateway's own page in place of the Panel's
  // answer (502/504 HTML): both are "not known", never a failure.
  for (const bind of [
    async () => { throw new TypeError('network'); },
    async () => new Response('<html><body>502 Bad Gateway</body></html>', { status: 502, headers: { 'Content-Type': 'text/html' } }),
    async () => new Response('<html><body>504 Gateway Time-out</body></html>', { status: 504, headers: { 'Content-Type': 'text/html' } }),
  ]) {
    await mount({ reads: [read([tool({})])], bind });
    try {
      const button = tree.root.findAll((node) => node.type === 'button' && label(node).includes(say('component.phpTool.apply', { version: '8.6' })))[0];
      await act(async () => { button.props.onClick(); });
      await settle();
      // alpha.108: the card reads the state back instead of leaving it
      // unknown, and never sends the move again.
      assert.equal(attr('data-php-tool-result').props['data-php-tool-result'], 'reconciledRead');
      assert.equal(label(attr('data-php-tool-result')), say('component.phpTool.reconciledRead', { tool: 'phpPgAdmin', current: '8.4' }));
      assert.equal(requests.filter((request) => request.url === '/api/v1/service/php-tools/bind').length, 1);
    } finally { await unmount(); }
  }
  await mount({ reads: ['fail'] });
  try {
    assert.ok(label(tree.root).includes(say('component.phpTool.unknown', { tool: 'phpPgAdmin' })));
  } finally { await unmount(); }
});

test('Turkish: the card speaks Turkish with no raw key', async () => {
  await mount({ reads: [read([tool({})])], words: tr });
  try {
    const text = label(tree.root);
    assert.ok(text.includes(fill(tr['component.phpTool.current'], { tool: 'phpPgAdmin', version: '8.4' })));
    assert.ok(!/component\.phpTool\./.test(text));
  } finally { await unmount(); }
});
