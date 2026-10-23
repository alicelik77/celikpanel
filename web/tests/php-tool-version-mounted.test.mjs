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
import { enComponentsScreens as en } from '../src/i18n/screens/components/en.ts';
import { trComponentsScreens as tr } from '../src/i18n/screens/components/tr.ts';

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
    bind: async () => answer({ error: 'phpPgAdmin did not work on PHP 8.6 (phpPgAdmin answered HTTP 502); it was put back on PHP 8.4', code: 'PHP_TOOL_PROBE_FAILED', restored: true, previous: '8.4' }, 409),
  });
  try {
    const button = tree.root.findAll((node) => node.type === 'button' && label(node).includes(say('component.phpTool.apply', { version: '8.6' })))[0];
    await act(async () => { button.props.onClick(); });
    await settle();
    assert.equal(attr('data-php-tool-result').props['data-php-tool-result'], 'restored');
    assert.ok(label(attr('data-php-tool-result')).includes(say('component.phpTool.restored', {
      tool: 'phpPgAdmin', version: '8.6', previous: '8.4', reason: 'phpPgAdmin did not work on PHP 8.6 (phpPgAdmin answered HTTP 502); it was put back on PHP 8.4',
    })));
  } finally { await unmount(); }
});

test('a lost answer says the result is not known; a failed read says so with Retry', async () => {
  await mount({ reads: [read([tool({})])], bind: async () => { throw new TypeError('network'); } });
  try {
    const button = tree.root.findAll((node) => node.type === 'button' && label(node).includes(say('component.phpTool.apply', { version: '8.6' })))[0];
    await act(async () => { button.props.onClick(); });
    await settle();
    assert.equal(attr('data-php-tool-result').props['data-php-tool-result'], 'failed');
    assert.ok(label(attr('data-php-tool-result')).includes(en['component.phpTool.answerLost']));
  } finally { await unmount(); }
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
