// alpha.108, the owner's Boston findings (Ubuntu 24.04, phpPgAdmin on the
// distribution's PHP 8.3), mounted: the Manage card says a failed proof's own
// cause and never "PHP-FPM 8.3 is not running" while 8.3 runs; Check again
// proves without moving; a lost answer is read back (bounded, visibly) and
// never sent again; the install toast says the proof's real cause.
// Boston bulguları, bağlanmış olarak: kart kanıtın kendi nedenini söyler.
import assert from 'node:assert/strict';
import test from 'node:test';
import { readFileSync } from 'node:fs';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { enComponentsScreens } from '../src/i18n/screens/components/en.ts';
import { trComponentsScreens } from '../src/i18n/screens/components/tr.ts';
import { enComponentPagesScreens } from '../src/i18n/screens/componentPages/en.ts';
import { trComponentPagesScreens } from '../src/i18n/screens/componentPages/tr.ts';
import { en as enShell } from '../src/i18n/en.ts';
import { tr as trShell } from '../src/i18n/tr.ts';

const en = { ...enComponentsScreens, ...enComponentPagesScreens, 'common.retry': enShell['common.retry'] };
const tr = { ...trComponentsScreens, ...trComponentPagesScreens, 'common.retry': trShell['common.retry'] };
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

const read = (tools, versions = ['8.3'], changing) => ({
  state: 'read', tools, versions: versions.map((version) => ({ version, sites: 0, site_names: [], tools: [] })), ...(changing ? { changing } : {}),
});
const unreadable = { kind: 'check_unreadable', version: '8.3', status: 404, detail: 'PHP-FPM 8.3 could not open CelikPanel’s PHP check for phpPgAdmin (HTTP 404: File not found.)' };
// Boston after alpha.107: 8.3 runs, its socket is there, the proof failed.
const boston = (extra) => ({
  tool: 'phppgadmin', service_id: 'phppgadmin', installed: true, version: '8.3', ready: false, version_installed: true, extensions_present: true,
  socket_ready: true, fpm_state: 'active', proof_failed: true, proof_cause: unreadable, ...extra,
});
const proved = (extra) => boston({ ready: true, proof_failed: false, proof_cause: undefined, ...extra });

const originalFetch = globalThis.fetch;
let tree;
let requests = [];
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
async function settle(ms = 10) { await act(async () => { await new Promise((resolve) => setTimeout(resolve, ms)); }); }
async function mount({ reads, bind, prove, words = en }) {
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
    if (url === '/api/v1/service/php-tools/prove') return prove(init);
    return answer({ error: 'no route' }, 404);
  };
  await act(async () => { tree = Renderer.create(React.createElement(PHPToolVersion, { serviceId: 'phppgadmin', tool: 'phppgadmin', reconcileDelayMs: 1 })); });
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
const causeText = (words, cause) => fill(words['component.phpTool.cause.' + cause.kind], { version: cause.version, status: cause.status })
  + (['no_answer', 'tool_error', 'not_tool_page'].includes(cause.kind) ? fill(words['component.phpTool.cause.said'], { detail: cause.detail }) : '');
const button = (text) => tree.root.findAll((node) => node.type === 'button' && label(node).includes(text))[0];
const mutations = () => requests.filter((request) => request.method !== 'GET');

test('Boston: a failed proof says its cause, not "not running", and offers Check again instead of the same version', async () => {
  await mount({ reads: [read([boston({})])] });
  try {
    assert.equal(attr('data-php-tool-state').props['data-php-tool-state'], 'proof');
    const text = label(attr('data-php-tool-state'));
    assert.equal(text, say('component.phpTool.proofFailed', { tool: 'phpPgAdmin', version: '8.3', reason: causeText(en, unreadable) }));
    assert.ok(!text.includes('not running'), text);
    assert.ok(attr('data-php-tool-check'), 'no Check again');
    // 8.3 is the only version and Check again covers it: nothing else is offered.
    assert.ok(attr('data-php-tool-options'));
    assert.equal(mutations().length, 0, 'reading the card changed something');
  } finally { await unmount(); }
});

test('"not running" only when the socket is missing or the unit is stopped or failed, read now', async () => {
  for (const extra of [{ socket_ready: false, fpm_state: 'inactive' }, { socket_ready: true, fpm_state: 'failed' }, { socket_ready: false, fpm_state: '' }]) {
    await mount({ reads: [read([boston(extra)])] });
    try {
      assert.equal(attr('data-php-tool-state').props['data-php-tool-state'], 'stopped', JSON.stringify(extra));
      assert.equal(label(attr('data-php-tool-state')), say('component.phpTool.stopped', { tool: 'phpPgAdmin', version: '8.3' }));
      assert.equal(tree.root.findAll((node) => node.props && node.props['data-php-tool-check'] !== undefined).length, 0);
    } finally { await unmount(); }
  }
});

test('Check again proves on the version it runs on and moves nothing', async () => {
  let release;
  const held = new Promise((resolve) => { release = resolve; });
  await mount({
    reads: [read([boston({})]), read([proved({})])],
    prove: async () => { await held; return answer({ proved: true, tool: 'phppgadmin', version: '8.3' }); },
  });
  try {
    await act(async () => { button(en['component.phpTool.checkAgain']).props.onClick(); });
    await settle();
    // Visible waiting while it runs.
    assert.ok(label(tree.root).includes(say('component.phpTool.checkWait', { tool: 'phpPgAdmin', version: '8.3' })));
    await act(async () => { release(); });
    await settle();
    assert.equal(label(attr('data-php-tool-result')), say('component.phpTool.checkPassed', { tool: 'phpPgAdmin', version: '8.3' }));
    assert.equal(label(attr('data-php-tool-state')), say('component.phpTool.current', { tool: 'phpPgAdmin', version: '8.3' }));
    assert.deepEqual(mutations().map((request) => [request.url, JSON.parse(request.body)]), [['/api/v1/service/php-tools/prove', { tool: 'phppgadmin' }]]);
  } finally { await unmount(); }
});

test('Check again that fails says the cause; one refused as busy says so and started nothing', async () => {
  await mount({ reads: [read([boston({})])], prove: async () => answer({ proved: false, tool: 'phppgadmin', version: '8.3', cause: unreadable }) });
  try {
    await act(async () => { button(en['component.phpTool.checkAgain']).props.onClick(); });
    await settle();
    assert.equal(attr('data-php-tool-result').props['data-php-tool-result'], 'checkFailed');
    assert.equal(label(attr('data-php-tool-result')), say('component.phpTool.checkFailed', { tool: 'phpPgAdmin', version: '8.3', reason: causeText(en, unreadable) }));
  } finally { await unmount(); }
  await mount({ reads: [read([boston({})])], prove: async () => answer({ error: 'another package operation is already in progress', code: 'service_operation_busy' }, 409) });
  try {
    await act(async () => { button(en['component.phpTool.checkAgain']).props.onClick(); });
    await settle();
    assert.equal(label(attr('data-php-tool-result')), en['component.phpTool.checkBusy']);
  } finally { await unmount(); }
});

test('a lost check is read back, waiting while the server still runs it; never asked again', async () => {
  await mount({
    reads: [read([boston({})]), read([boston({})], ['8.3'], { kind: 'php_tool_prove', tool: 'phppgadmin' }), read([proved({})])],
    prove: async () => { throw new TypeError('network'); },
  });
  try {
    await act(async () => { button(en['component.phpTool.checkAgain']).props.onClick(); });
    await settle(30);
    assert.equal(attr('data-php-tool-result').props['data-php-tool-result'], 'reconciledRead');
    assert.equal(label(attr('data-php-tool-result')), say('component.phpTool.reconciledRead', { tool: 'phpPgAdmin', current: '8.3' }));
    assert.equal(label(attr('data-php-tool-state')), say('component.phpTool.current', { tool: 'phpPgAdmin', version: '8.3' }));
    assert.equal(mutations().length, 1);
    assert.equal(requests.filter((request) => request.url === '/api/v1/service/php-tools').length, 3);
  } finally { await unmount(); }
});

test('a lost move whose state now shows the target ready says it completed; reads that keep failing give up with Retry', async () => {
  const two = (tool) => read([tool], ['8.4', '8.3']);
  await mount({
    reads: [two(proved({})), two(proved({ version: '8.4' }))],
    bind: async () => new Response('<html>504 Gateway Time-out</html>', { status: 504, headers: { 'Content-Type': 'text/html' } }),
  });
  try {
    await act(async () => { button(say('component.phpTool.apply', { version: '8.4' })).props.onClick(); });
    await settle(30);
    assert.equal(label(attr('data-php-tool-result')), say('component.phpTool.reconciledMoved', { tool: 'phpPgAdmin', version: '8.4' }));
    assert.equal(mutations().length, 1);
  } finally { await unmount(); }

  await mount({ reads: [two(proved({})), 'fail'], bind: async () => { throw new TypeError('network'); } });
  try {
    await act(async () => { button(say('component.phpTool.apply', { version: '8.4' })).props.onClick(); });
    await settle(1500);
    assert.equal(attr('data-php-tool-result').props['data-php-tool-result'], 'reconcileGaveUp');
    assert.ok(attr('data-php-tool-retry'), 'no Retry');
    assert.equal(mutations().length, 1, 'the move was sent again');
    assert.equal(requests.filter((request) => request.url === '/api/v1/service/php-tools').length, 1 + 24);
  } finally { await unmount(); }
});

test('"Use PHP 8.3" on 8.3 that only proved says nothing was moved, with the cause', async () => {
  // Extension packages missing: the same version is offered (that path installs them).
  await mount({
    reads: [read([proved({ extensions_present: false })])],
    bind: async () => answer({ error: 'phpPgAdmin is already set to PHP 8.3 but did not answer as itself there', code: 'PHP_TOOL_PROBE_FAILED', proof_only: true, previous: '8.3', cause: unreadable }, 409),
  });
  try {
    await act(async () => { button(say('component.phpTool.apply', { version: '8.3' })).props.onClick(); });
    await settle();
    assert.equal(label(attr('data-php-tool-result')), say('component.phpTool.proofOnlyFailed', { tool: 'phpPgAdmin', version: '8.3', reason: causeText(en, unreadable) }));
  } finally { await unmount(); }
});

test('Turkish: the Boston card speaks Turkish and does not say PHP-FPM is not running', async () => {
  await mount({ reads: [read([boston({})])], words: tr });
  try {
    const text = label(tree.root);
    assert.ok(text.includes(fill(tr['component.phpTool.proofFailed'], { tool: 'phpPgAdmin', version: '8.3', reason: causeText(tr, unreadable) })), text);
    assert.ok(!text.includes('çalışmadığı için'), text);
    assert.ok(text.includes(tr['component.phpTool.checkAgain']));
    assert.ok(!/component\.phpTool\./.test(text), text);
  } finally { await unmount(); }
});

test('the install toast says the proof’s own cause; the shell keeps its own small decoder', () => {
  const source = readFileSync(new URL('../src/components/ComponentOperation.tsx', import.meta.url), 'utf8');
  assert.match(source, /proof\.kind === 'check_unreadable'/);
  assert.match(source, /t\('services\.phpToolProof\.whySaid', \{ detail: proof\.detail \}\)/);
  assert.ok(!/from '\.\.\/lib\/phpTools'/.test(source), 'the shell module must not pull lib/phpTools');
  for (const words of [enShell, trShell]) {
    for (const key of ['services.phpToolProof.whyCheck', 'services.phpToolProof.whySaid']) assert.ok(words[key], key);
  }
  assert.ok(enShell['services.phpToolProof.notReady'].includes('Check again'));
  assert.ok(trShell['services.phpToolProof.notReady'].includes('Yeniden denetle'));
});
