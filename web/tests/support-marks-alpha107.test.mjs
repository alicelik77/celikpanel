// alpha.107 (D-038 §11, §5, §14): versions are marked from the dated support
// table, mounted on the site's own PHP page (what the customer sees): each
// offered version carries its mark, the chosen one says until when and from
// whom, and choosing one without security fixes asks its own question and
// tells the server it was confirmed. A version the table does not know is
// "not known".
// Sürümler tarihli destek tablosundan işaretlenir; müşterinin kendi sayfası.
import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, sharedLayer, supportTableURL } from './fixtures/shared-layer.mjs';
import { enLifecycleScreens } from '../src/i18n/screens/lifecycle/en.ts';

const en = { ...enLifecycleScreens, 'php.version': 'PHP version', 'php.apply': 'Apply', 'php.changeConfirm': 'Change PHP from {from} to {to}?', 'php.changed': 'Changed to {version}' };
const fill = (text, vars = {}) => Object.entries(vars).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
globalThis.cardWords = en;
const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null, Save = () => null, X = () => null, Loader2 = () => null;
  const fill = (text, vars) => Object.entries(vars || {}).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
  export const useI18n = () => ({ t: (key, vars) => (globalThis.cardWords?.[key] !== undefined ? fill(globalThis.cardWords[key], vars) : key), locale: 'en' });
  export const useNavigate = () => () => true;
  export const showToast = (...args) => { (globalThis.toasts ??= []).push(args); };
`);
const shared = sharedLayer(stub);
const { DomainPHPSettings } = await import(shared.compile('components/DomainPHPSettings.tsx'));
const support = await import(supportTableURL);

const table = {
  checked: '2026-10-24', sources: {}, distro: 'debian-13', distro_name: 'Debian 13 (trixie)',
  statuses: {
    php: {
      '8.4': { status: 'supported', until: '2030-06-30', provider: 'distribution' },
      '8.2': { status: 'ends_soon', until: '2026-12-31', provider: 'upstream' },
      '8.1': { status: 'unsupported', until: '2025-12-31', provider: 'upstream' },
    },
  },
};
const originalFetch = globalThis.fetch;
const originalConfirm = globalThis.confirm;
let tree;
let requests = [];
let asked = [];
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
async function settle() { await act(async () => { await new Promise((resolve) => setTimeout(resolve, 10)); }); }
async function mount() {
  requests = [];
  asked = [];
  globalThis.confirm = (text) => { asked.push(text); return true; };
  globalThis.fetch = async (input, init = {}) => {
    const url = String(input);
    requests.push({ url, method: init.method ?? 'GET', body: init.body });
    if (url === '/api/v1/support-table') return answer(table);
    if (url === '/api/v1/hosting/capabilities') return answer({ web_server: 'nginx', php_versions: ['8.4', '8.2', '8.1'], dns_server: '', dns_identity_ready: false, mail_server: false, database_servers: [], db_tools: [] });
    if (url === '/api/v1/domains/7/php' && init.method === 'POST') return answer({ status: 'success' });
    if (url === '/api/v1/domains/7/php') return answer({ php_version: '8.4', pool_name: 'example', pool_config: null, available_versions: [] });
    return answer({ error: 'no route' }, 404);
  };
  await act(async () => {
    tree = Renderer.create(React.createElement(DomainPHPSettings, { domainId: 7, domainName: 'example.test', currentVersion: '8.4', onVersionChange: () => {} }));
  });
  await settle();
}
async function unmount() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  globalThis.confirm = originalConfirm;
}
const label = (node) => { const walk = (n) => (typeof n === 'string' ? n : (n.children || []).map(walk).join('')); return walk(node); };

test('each offered PHP version carries its mark, and the chosen one says until when', async () => {
  await mount();
  try {
    const options = tree.root.findAllByType('option').map(label);
    assert.deepEqual(options, ['PHP 8.4', `PHP 8.2 — ${fill(en['life.support.badge.ends_soon'], { until: '2026-12-31' })}`, `PHP 8.1 — ${en['life.support.badge.unsupported']}`]);
    const line = tree.root.findAll((node) => node.props?.['data-support-line'] !== undefined)[0];
    assert.equal(line.props['data-support-line'], 'supported');
    assert.ok(label(line).includes('Debian 13 (trixie)'));
  } finally { await unmount(); }
});

test('choosing a version without security fixes asks its own question and tells the server', async () => {
  await mount();
  try {
    const select = tree.root.findByType('select');
    await act(async () => { select.props.onChange({ target: { value: '8.1' } }); });
    const line = tree.root.findAll((node) => node.props?.['data-support-line'] !== undefined)[0];
    assert.equal(line.props['data-support-line'], 'unsupported');
    const apply = tree.root.findAll((node) => node.type === 'button' && label(node).includes('Apply'))[0];
    await act(async () => { await apply.props.onClick(); });
    await settle();
    assert.equal(asked.length, 1);
    assert.ok(asked[0].includes('no longer receives security fixes'));
    const post = requests.find((request) => request.method === 'POST');
    assert.deepEqual(JSON.parse(post.body), { php_version: '8.1', confirm_unsupported: true });
  } finally { await unmount(); }
});

test('the table decides; a version it does not know is not known', () => {
  assert.equal(support.supportFor(table, 'php', '8.6').status, 'unknown');
  assert.equal(support.supportFor(table, 'php', '8.1.34').status, 'unsupported');
  assert.equal(support.supportFor(null, 'php', '8.4').status, 'unknown');
  assert.equal(support.supportLine('node', 'v22.14.0'), '22');
  assert.throws(() => support.decodeSupportTable({ checked: 'x', statuses: { php: { '8.4': { status: 'maybe' } } } }));
});
