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
import { enRemovalRuleScreens } from '../src/i18n/screens/removalRule/en.ts';
import { trRemovalRuleScreens } from '../src/i18n/screens/removalRule/tr.ts';

// alpha.123 (D-045): the server-side remove-together operation in the global
// component operation lock. The real ComponentOperationProvider and overlay are
// mounted; the Panel's replies are scripted. Held:
//   - a page opened while the operation runs (a reload, or after a panel
//     restart) re-attaches to it, by the tab's marker or by the active read;
//   - the lock says the removal's own step, who acts (nobody) and how it
//     resumes, never the install wording and never a lost connection;
//   - nothing but GET is sent: a browser marker never starts a removal.
// Birlikte kaldırma işlemi genel kilitte: yeniden bağlanır, adımını söyler.

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
 export const useAuth = () => ({ role: globalThis.operationRole ?? 'admin' });
 export const showToast = () => {};
 export const createPortal = node => node;
 export const invalidate = () => [];
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


const RUNNING = { id: ID, request_id: REQUEST, kind: 'service_remove_together', service_id: 'postgresql', package_name: 'phppgadmin', status: 'running', phase: 'removing:phppgadmin', started_at: '2026-11-04T10:00:00Z' };
const ACTIVE = 'GET /api/v1/service/operation?active=1';

async function mount(session, replies) {
  timers = []; calls = []; reloads = 0; root.attributes.clear();
  routes = replies;
  storage = new Map(session);
  globalThis.fetch = async (url, init = {}) => {
    const key = `${init.method || 'GET'} ${url}`;
    calls.push({ key, method: init.method || 'GET' });
    const reply = routes[key];
    if (!reply) throw new Error(`unexpected request ${key}`);
    return reply[0]();
  };
  await act(async () => { tree = Renderer.create(React.createElement(ComponentOperationProvider, null, null)); });
  await settle();
}
async function close() { if (tree) await act(async () => tree.unmount()); tree = undefined; globalThis.fetch = originalFetch; }

function assertRemovalLock(c) {
  assert.ok(dialog(), 'the lock is up while the removal runs');
  const page = textOf(tree.toJSON());
  assert.ok(page.includes(c['services.operation.removeTitle'].replace('{name}', 'postgresql')), page);
  assert.ok(page.includes(c['services.operation.phase.removing'].replace('{name}', 'phppgadmin')), 'the step it is on');
  assert.ok(page.includes(c['services.operation.removeHint'].replace('{name}', 'postgresql').replace('{together}', 'phppgadmin')), 'who acts and how it resumes');
  assert.ok(!page.includes(c['services.operation.reconnecting']), 'not a lost connection');
  assert.ok(!page.includes(c['services.operation.title'].replace('{name}', 'postgresql (phppgadmin)')), 'not the install wording');
  assert.ok(calls.every(call => call.method === 'GET'), 'following only reads');
  const marker = JSON.parse(storage.get(KEYS[2]));
  assert.equal(marker.operation_kind, 'service_remove_together');
  assert.equal(marker.package_name, 'phppgadmin');
}

for (const [locale, c] of Object.entries({ en: { ...en, ...enRemovalRuleScreens }, tr: { ...tr, ...trRemovalRuleScreens } })) {
  test(`${locale}: after a panel restart a new page re-attaches to the running removal by the active read`, async () => {
    globalThis.reconnectCatalog = c;
    await mount([], { [ACTIVE]: [() => Response.json({ operation: RUNNING })], [STATUS]: [() => Response.json({ operation: RUNNING })] });
    try {
      assertRemovalLock(c);
      assert.ok(calls.some(call => call.key === ACTIVE), 'the active operation was read');
    } finally { await close(); }
  });

  test(`${locale}: a reloaded tab re-attaches by its own marker`, async () => {
    globalThis.reconnectCatalog = c;
    await mount([
      [KEYS[0], ID], [KEYS[1], 'postgresql (phppgadmin)'],
      [KEYS[2], JSON.stringify({ version: 3, operation_kind: 'service_remove_together', request_id: REQUEST, service_id: 'postgresql', label: 'postgresql (phppgadmin)', package_name: 'phppgadmin', created_at: Date.parse('2026-11-04T10:00:00Z') })],
    ], { [STATUS]: [() => Response.json({ operation: RUNNING })], [ACTIVE]: [() => Response.json({ operation: RUNNING })] });
    try {
      assertRemovalLock(c);
    } finally { await close(); }
  });
}

test('the remove-together marker is a valid recovery marker and never carries a runtime version', async () => {
  const { createOperationRecoveryMarker, decodeOperationRecoveryMarker } = await import(providerURL);
  const marker = createOperationRecoveryMarker({ serviceId: 'postgresql', name: 'PostgreSQL', operationKind: 'service_remove_together', package: 'phppgadmin' }, 1, REQUEST);
  assert.equal(marker?.operation_kind, 'service_remove_together');
  assert.deepEqual(decodeOperationRecoveryMarker(JSON.stringify(marker)), marker);
  assert.equal(createOperationRecoveryMarker({ serviceId: 'postgresql', name: 'PostgreSQL', operationKind: 'service_remove_together', version: '1.0.0' }, 1, REQUEST), null);
});
