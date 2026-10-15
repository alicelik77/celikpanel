import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import ts from 'typescript';
import { en } from '../src/i18n/en.ts';
import { tr } from '../src/i18n/tr.ts';

// The component operation lock after a lost connection (owner rule 2026-10-14,
// audit row 16). The real ComponentOperationProvider and the real overlay are
// mounted together; the Panel's replies are scripted. What is held:
//
//   - while the connection is lost the lock shows motion beside "Reconnecting"
//     and says that the page asks again by itself and nothing needs pressing;
//   - after 60 s without a usable answer the motion stops and an honest notice
//     says that the result is not known, that it is not a failure, and how to
//     check the panel on the server; "Reload page" stays;
//   - the reads go on every 3 s after the notice, the lock stays, and nothing
//     but GET is sent; an answer ends the reconnect and its notice.
//
// Kopan baglantida bilesen islemi kilidi: hareket ve "bir sey yapmaniz
// gerekmez" cumlesi; 60 sn sonra durust bildirim; okumalar 3 sn'de bir surer.
const require = createRequire(import.meta.url);
const reactURL = pathToFileURL(require.resolve('react')).href;
const dataModule = source => 'data:text/javascript;base64,' + Buffer.from(source).toString('base64');
const read = path => readFileSync(new URL(path, import.meta.url), 'utf8');
const compile = text => ts.transpileModule(text, { compilerOptions: { jsx: ts.JsxEmit.React, module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 } }).outputText;

const stub = dataModule(`import React from '${reactURL}';
 export const useI18n = () => ({ t: (key, vars = {}) => {
   let text = globalThis.reconnectCatalog[key] ?? key;
   for (const [name, value] of Object.entries(vars)) text = text.replaceAll('{' + name + '}', String(value));
   return text;
 } });
 export const readApiError = async response => ({ message: 'error ' + response.status });
 export const useNavigationBlocker = () => {};
 export const showToast = () => {};
 export const createPortal = node => node;
 export const refreshRemote = () => {};
 export const Button = props => React.createElement('button', props);
 export const ErrorBanner = props => React.createElement('error-banner', props);
 export const KeepWords = props => props.text;
 export const AlertTriangle = () => React.createElement('warning-icon');
 export const Loader2 = props => React.createElement('spinner-icon', props);
 export const WifiOff = () => React.createElement('offline-icon');
 export const X = () => null;
`);
const providerSource = read('../src/components/ComponentOperation.tsx');
const decoderURL = dataModule(compile(providerSource.slice(providerSource.indexOf('export function operationError('), providerSource.indexOf('function restoredOperation('))));
const guidanceURL = dataModule(compile(read('../src/lib/componentOperationGuidance.ts')));
const handoverURL = dataModule(compile(read('../src/lib/panelHandover.ts')));
const overlayURL = dataModule(`import React from '${reactURL}';\n` + compile(read('../src/components/OperationOverlay.tsx'))
  .replace(/from ['"]([^'"]+)['"]/g, (_, name) => `from '${name === 'react' ? reactURL : name.endsWith('/ComponentOperation') ? decoderURL : name.endsWith('/componentOperationGuidance') ? guidanceURL : stub}'`));
const providerURL = dataModule(`import React from '${reactURL}';\n` + compile(providerSource)
  .replace(/from ['"]([^'"]+)['"]/g, (_, name) => `from '${name === 'react' ? reactURL : name.endsWith('/panelHandover') ? handoverURL : stub}'`)
  .replace(/import\(['"]([^'"]+)['"]\)/g, () => `import('${overlayURL}')`));
const { ComponentOperationProvider } = await import(providerURL);

const ID = 'a'.repeat(32), REQUEST = 'b'.repeat(32);
const KEYS = ['celikpanel.components.operation-id', 'celikpanel.components.operation-label', 'celikpanel.components.operation-recovery'];
const STATUS = `GET /api/v1/service/operation?id=${ID}`;
const running = () => Response.json({ operation: { id: ID, request_id: REQUEST, kind: 'service_install', service_id: 'nginx', status: 'running', phase: 'installing', started_at: '2026-10-14T10:00:00Z' } });
const refused = () => { throw new TypeError('connection refused'); };

let tree, timers, calls, routes, storage, reloads;
const realTimeout = setTimeout;
globalThis.HTMLElement = class {};
const root = { attributes: new Map(), hasAttribute(name) { return this.attributes.has(name); }, getAttribute(name) { return this.attributes.get(name) ?? null; }, setAttribute(name, value) { this.attributes.set(name, value); }, removeAttribute(name) { this.attributes.delete(name); } };
globalThis.document = Object.assign(new EventTarget(), { visibilityState: 'visible', activeElement: null, body: {}, getElementById: id => (id === 'root' ? root : null) });
globalThis.window = Object.assign(new EventTarget(), {
  // Every product timer is fired by hand, so a retry and the bound are observable.
  setTimeout: (fn, ms) => { const timer = { fn, ms, live: true }; timers.push(timer); return timer; },
  clearTimeout: timer => { if (timer) timer.live = false; },
  location: { reload: () => { reloads++; } },
});
globalThis.sessionStorage = { getItem: key => (storage.has(key) ? storage.get(key) : null), setItem: (key, value) => { storage.set(key, String(value)); }, removeItem: key => { storage.delete(key); } };
const originalFetch = globalThis.fetch;

const settle = async () => { for (let round = 0; round < 8; round++) await act(async () => { await new Promise(resolve => realTimeout(resolve, 0)); }); };
const pending = () => timers.filter(timer => timer.live).map(timer => timer.ms);
// Fires only the live timers of one delay: the 3 s reads and the 60 s bound are separate.
const fire = async ms => { const due = timers.filter(timer => timer.live && timer.ms === ms); for (const timer of due) timer.live = false; await act(async () => { for (const timer of due) timer.fn(); }); await settle(); return due.length; };
const textOf = node => node == null ? '' : typeof node === 'string' ? node : Array.isArray(node) ? node.map(textOf).join(' ') : textOf(node.children);
const dialog = () => tree.root.findAll(node => node.props && node.props.role === 'dialog')[0] ?? null;
const reconnectSpinners = () => tree.root.findAll(node => node.type === 'spinner-icon' && node.props['data-operation-reconnecting'] !== undefined);
const reloadButton = c => tree.root.findAll(node => node.type === 'button' && node.props.children === c['common.reloadPage'])[0] ?? null;

async function open() {
  timers = []; calls = []; reloads = 0; root.attributes.clear();
  routes = { [STATUS]: [running] };
  storage = new Map([
    [KEYS[0], ID], [KEYS[1], 'Nginx'],
    [KEYS[2], JSON.stringify({ version: 3, operation_kind: 'service_install', request_id: REQUEST, service_id: 'nginx', label: 'Nginx', created_at: Date.parse('2026-10-14T10:00:00Z') })],
  ]);
  globalThis.fetch = async (url, init = {}) => {
    const key = `${init.method || 'GET'} ${url}`;
    calls.push({ key, method: init.method || 'GET' });
    const replies = routes[key];
    if (!replies) throw new Error(`unexpected request ${key}`);
    return replies[0]();
  };
  await act(async () => { tree = Renderer.create(React.createElement(ComponentOperationProvider, null, null)); });
  await settle();
}
async function close() { if (tree) await act(async () => tree.unmount()); tree = undefined; globalThis.fetch = originalFetch; }

for (const [locale, c] of Object.entries({ en, tr })) {
  test(`${locale}: a lost connection reconnects with motion, says nothing needs pressing, and is bounded while the reads go on`, async () => {
    globalThis.reconnectCatalog = c;
    await open();
    try {
      assert.ok(dialog(), 'the lock is up while the operation runs');
      assert.equal(reconnectSpinners().length, 0, 'no reconnect while the server answers');
      // The connection drops: the next read is refused.
      routes[STATUS] = [refused];
      assert.equal(await fire(1500), 1);
      let page = textOf(tree.toJSON());
      assert.ok(page.includes(c['services.operation.reconnecting']), 'the status names the reconnect');
      assert.ok(page.includes(c['services.operation.reconnectingAuto']), 'the sentence says the page asks again by itself');
      assert.equal(reconnectSpinners().length, 1, 'the reconnect carries motion');
      // Inline before the first word, so it stays with the sentence when a phone
      // wraps it (browser inspection 2026-10-14: a flex item sat apart at the edge).
      assert.match(reconnectSpinners()[0].props.className || '', /(^| )inline-block( |$)/);
      assert.ok(!/(^| )flex( |$)/.test(reconnectSpinners()[0].parent.props.className || ''), 'the sentence is not a flex row that parts the spinner from its text');
      assert.ok(pending().includes(3000), 'the next read is scheduled');
      assert.equal(pending().filter(ms => ms === 60000).length, 1, 'the reconnect is bounded');
      // Further refused reads do not restart the bound.
      for (let round = 0; round < 3; round++) assert.equal(await fire(3000), 1);
      assert.equal(timers.filter(timer => timer.ms === 60000).length, 1, 'the bound is set once per reconnect');
      assert.equal(await fire(60000), 1);
      page = textOf(tree.toJSON());
      assert.ok(page.includes(c['services.operation.reconnectSlow']), 'an honest unknown result after a minute');
      assert.ok(!page.includes(c['services.operation.reconnectingAuto']));
      assert.equal(reconnectSpinners().length, 0, 'no motion left running past the bound');
      assert.ok(dialog(), 'the lock stays: an operation may still be running');
      assert.equal(root.hasAttribute('inert'), true, 'the page under the lock stays inert');
      const before = calls.length;
      assert.deepEqual(pending(), [3000], 'reads continue after the notice');
      assert.equal(await fire(3000), 1);
      assert.equal(await fire(3000), 1);
      assert.equal(calls.length, before + 2, 'one read every 3 s after the notice');
      assert.ok(textOf(tree.toJSON()).includes(c['services.operation.reconnectSlow']), 'the notice stays while no answer arrives');
      const reload = reloadButton(c);
      assert.ok(reload, 'reload is offered');
      reload.props.onClick();
      assert.equal(reloads, 1);
      // The server answers again: the reconnect and its notice end by themselves.
      routes[STATUS] = [running];
      assert.equal(await fire(3000), 1);
      page = textOf(tree.toJSON());
      assert.ok(!page.includes(c['services.operation.reconnectSlow']) && !page.includes(c['services.operation.reconnecting']));
      assert.equal(reconnectSpinners().length, 0);
      assert.ok(calls.every(call => call.method === 'GET'), 'reconnecting only reads');
      assert.ok(calls.every(call => call.key === STATUS), 'only the operation is read');
    } finally { await close(); }
  });
}

test('the reconnect wording says who acts and how it resumes in both languages', () => {
  for (const c of [en, tr]) {
    assert.ok(c['services.operation.reconnectingAuto']);
    assert.match(c['services.operation.reconnectSlow'], /sudo systemctl status celikpanel-panel/);
    assert.match(c['services.operation.reconnectSlow'], /sudo systemctl start celikpanel-panel/);
  }
  assert.match(en['services.operation.reconnectingAuto'], /by itself; you do not need to press anything/);
  assert.match(tr['services.operation.reconnectingAuto'], /kendiliğinden yeniden sorar; hiçbir düğmeye basmanız gerekmez/);
  assert.match(en['services.operation.reconnectSlow'], /not known yet\. This does not mean it failed: the server keeps the operation's record/);
  assert.match(tr['services.operation.reconnectSlow'], /henüz bilinmiyor\. Bu, işlemin başarısız olduğu anlamına gelmez: sunucu işlemin kaydını tutar/);
});
