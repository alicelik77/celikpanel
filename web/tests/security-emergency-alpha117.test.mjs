import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { enEmergencyScreens } from '../src/i18n/screens/emergency/en.ts';
import { trEmergencyScreens } from '../src/i18n/screens/emergency/tr.ts';

// The security emergency in the interface (D-043, alpha.117): the section
// shows each emergency's guidance before its steps; a step's confirmation
// lists who is affected and stays closed until the reason, the exact name
// and the password are there; reading progress never sends anything; a
// customer's notice switches a site through its ordinary version change.
const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const icons = new Set(['AlertTriangle', 'XCircle']);
for (const name of ['ui.tsx', 'SecurityEmergency.tsx', 'SecurityNoticeBanner.tsx']) {
  for (const match of source(`components/${name}`).matchAll(/import\s*\{([^}]+)\}\s*from 'lucide-react'/g)) {
    for (const item of match[1].split(',')) {
      const icon = item.replace(/\btype\b/, '').trim().split(/\s+as\s+/)[0];
      if (icon && icon !== 'LucideIcon') icons.add(icon);
    }
  }
}
const stub = dataModule(`
  import React from '${reactURL}';
  ${[...icons].map((name) => `export const ${name} = () => null;`).join('\n')}
  export const useI18n = () => ({ t: (key, vars) => key + (vars ? JSON.stringify(vars) : ''), locale: 'en' });
  export const useNavigate = () => () => {};
  export const useLocation = () => ({ pathname: '/components/php-fpm', search: '', hash: '' });
  export const useAuth = () => ({ role: globalThis.emergencyTest?.role ?? 'customer' });
  export const showToast = () => {};
`);
const shared = sharedLayer(stub);
const { SecurityEmergencySection } = await import(shared.compile('components/SecurityEmergency.tsx'));
const { SecurityNoticeBanner, switchRequest } = await import(shared.compile('components/SecurityNoticeBanner.tsx'));

const detail = (extra = {}) => ({
  emergency: { id: 'e1', component: 'php', version: '8.5', name: 'PHP 8.5', reason: 'CVE-2026-0001', stage: 'withdrawn', withdrawn: true, recommended: '8.4' },
  guidance: { result: 'PHP 8.5 is withdrawn for security.', reason: 'CVE-2026-0001', who_acts: 'administrator', next_action: 'Stop it if the risk remains.', resumes: 'Nothing waits.' },
  operation: null,
  preview: {
    sites: [{ domain_id: 10, domain: 'shop.example.com', customer: 'ada', reseller: 'res', maintenance: true }],
    customers: [{ id: 7, name: 'ada', sites: 1 }], resellers: [{ id: 3, name: 'res', sites: 1 }], offered_by: ['Plan: Basic'], notes: ['Nothing is deleted.'],
  },
  notices: { total: 0, seen: 0, mail_sent: 0, mail_pending: 0, mail_failed: 0, no_address: 0, mail_not_available: 0 },
  confirm_name: 'PHP 8.5', alternatives: ['8.4'], can: { stop: true, reenable: true, cancel: true },
  ...extra,
});

const originalFetch = globalThis.fetch;
let tree, calls;
function serve(answers) {
  calls = [];
  globalThis.emergencyTest = { role: answers.role };
  globalThis.document = { addEventListener() {}, removeEventListener() {}, getElementById: () => null };
  globalThis.fetch = (url, options = {}) => {
    const method = options.method || 'GET';
    calls.push({ method, url: String(url), body: options.body });
    const answer = answers[`${method} ${url}`];
    if (answer === undefined) return Promise.resolve(Response.json({ error: 'unexpected' }, { status: 404 }));
    return Promise.resolve(Response.json(answer.body, { status: answer.status ?? 200 }));
  };
}
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
async function mount(element) {
  await act(async () => { tree = Renderer.create(element); await settled(); await settled(); });
}
async function cleanup() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined; globalThis.fetch = originalFetch; delete globalThis.emergencyTest; delete globalThis.document;
}
const text = () => JSON.stringify(tree.toJSON());
const byData = (name, value) => tree.root.findAll((node) => node.props && node.props[name] === value);
const click = async (node) => { await act(async () => { node.props.onClick(); await settled(); await settled(); }); };
const type = async (node, value) => { await act(async () => { node.props.onChange({ target: { value } }); }); };

test('the emergency copy exists in both languages', () => {
  assert.deepEqual(Object.keys(trEmergencyScreens).sort(), Object.keys(enEmergencyScreens).sort());
  for (const code of ['EMERGENCY_NAME_MISMATCH', 'EMERGENCY_WRONG_PASSWORD', 'EMERGENCY_IMPERSONATION_REFUSED', 'VERSION_WITHDRAWN', 'SECURITY_EMERGENCY_MAINTENANCE']) {
    assert.ok(enEmergencyScreens[`err.${code}`] && trEmergencyScreens[`err.${code}`], code);
  }
  assert.match(enEmergencyScreens['emg.intro'], /No customer data is ever deleted/);
});

test('a closed section only reads; opened, it shows guidance before the steps', async () => {
  serve({ 'GET /api/v1/security-emergencies': { body: { emergencies: [detail()] } }, 'GET /api/v1/security-emergencies/e1': { body: detail() } });
  try {
    await mount(React.createElement(SecurityEmergencySection, { component: 'php', name: 'PHP', versions: ['8.4', '8.5'] }));
    // alpha.118: closed, it reads the list (one GET) to know whether an
    // emergency is open, and reads nothing else.
    assert.deepEqual(calls.map((call) => `${call.method} ${call.url}`), ['GET /api/v1/security-emergencies'], 'a closed section only reads the list');
    await click(tree.root.findAll((node) => node.type === 'button')[0]);
    const page = text();
    assert.ok(page.indexOf('PHP 8.5 is withdrawn for security.') < page.indexOf('emg.step.stop'), 'guidance comes before the steps');
    assert.ok(page.includes('Stop it if the risk remains.'));
    assert.ok(calls.every((call) => call.method === 'GET'), 'reading never sends a change');
    // Only the version without an emergency can be withdrawn again.
    const version = byData('data-emergency-version', '')[0];
    assert.deepEqual(version.props.children.map((option) => option.props.value), ['8.4']);
  } finally { await cleanup(); }
});

test('a stop is confirmed only with the reason, the exact name and the password', async () => {
  serve({
    'GET /api/v1/security-emergencies': { body: { emergencies: [detail()] } },
    'GET /api/v1/security-emergencies/e1': { body: detail() },
    'POST /api/v1/security-emergencies/e1/stop': { status: 202, body: detail({ operation: { id: 'o1', kind: 'stop', state: 'running', done: 0, total: 2, effects: [] } }) },
  });
  try {
    await mount(React.createElement(SecurityEmergencySection, { component: 'php', name: 'PHP', versions: ['8.4', '8.5'] }));
    await click(tree.root.findAll((node) => node.type === 'button')[0]);
    await click(byData('data-emergency-step', 'stop')[0]);
    const page = text();
    for (const affected of ['shop.example.com', 'ada', 'res', 'Plan: Basic']) assert.ok(page.includes(affected), affected);
    const confirm = () => byData('data-emergency-confirm', 'stop')[0];
    assert.equal(confirm().props.disabled, true);
    await type(byData('data-emergency-confirm-name', '')[0], 'PHP 8.4');
    await type(byData('data-emergency-confirm-password', '')[0], 'secret');
    assert.equal(confirm().props.disabled, true, 'a wrong name keeps it closed');
    await type(byData('data-emergency-confirm-name', '')[0], 'PHP 8.5');
    assert.equal(confirm().props.disabled, false);
    const form = tree.root.findAll((node) => node.type === 'form')[0];
    await act(async () => { form.props.onSubmit({ preventDefault() {} }); await settled(); await settled(); });
    const sent = calls.filter((call) => call.method === 'POST');
    assert.equal(sent.length, 1);
    assert.deepEqual(JSON.parse(sent[0].body), { reason: 'CVE-2026-0001', confirm_name: 'PHP 8.5', password: 'secret' });
  } finally { await cleanup(); }
});

test("a customer's notice switches the site through its ordinary version change", async () => {
  const notice = {
    id: 4, kind: 'emergency', audience: 'customer',
    body: { component: 'php', version: '8.5', name: 'PHP 8.5', reason: 'CVE-2026-0001', recommended: '8.4', sites: [{ domain_id: 10, domain: 'shop.example.com', maintenance: true }] },
    actions: [{ domain_id: 10, domain: 'shop.example.com', kind: 'php', to: '8.4' }],
  };
  serve({
    'GET /api/v1/security-notices': { body: { notices: [notice] } },
    'POST /api/v1/domains/10/php': { body: { status: 'success' } },
    'POST /api/v1/security-notices/4/seen': { body: { seen: true } },
  });
  try {
    await mount(React.createElement(SecurityNoticeBanner));
    const page = text();
    for (const words of ['notice.emg.title', 'CVE-2026-0001', 'shop.example.com', 'notice.emg.nothingLost']) assert.ok(page.includes(words), words);
    await click(byData('data-security-switch', 10)[0]);
    const posted = calls.find((call) => call.method === 'POST');
    assert.equal(posted.url, '/api/v1/domains/10/php');
    assert.deepEqual(JSON.parse(posted.body), { php_version: '8.4' });
  } finally { await cleanup(); }
  assert.deepEqual(switchRequest({ domain_id: 3, domain: 'a', kind: 'node', to: '22.12.0' }), { url: '/api/v1/domains/3/app/node-version', body: { version: '22.12.0' } });
});

test('a moved notice says where each site went and offers no switch', async () => {
  const notice = {
    id: 5, kind: 'moved', audience: 'reseller',
    body: { component: 'node', version: '22.11.0', name: 'Node.js 22.11.0', reason: 'CVE-2026-0002', sites: [{ domain_id: 11, domain: 'app.example.com', customer: 'ada', maintenance: false, moved_to: '22.12.0' }] },
    actions: [],
  };
  serve({ 'GET /api/v1/security-notices': { body: { notices: [notice] } }, 'POST /api/v1/security-notices/5/seen': { body: { seen: true } } });
  try {
    await mount(React.createElement(SecurityNoticeBanner));
    const page = text();
    assert.equal(byData('data-security-notice', 'moved').length, 1);
    for (const words of ['notice.emg.movedTitle', 'notice.emg.movedSite', '22.12.0', 'app.example.com', '(ada)', 'CVE-2026-0002', 'notice.emg.movedTodo']) assert.ok(page.includes(words), words);
    assert.equal(byData('data-security-switch', 11).length, 0);
    assert.ok(calls.every((call) => call.method === 'GET'), 'reading never sends a change');
  } finally { await cleanup(); }
  assert.ok(enEmergencyScreens['err.SECURITY_EMERGENCY_WITHDRAWN'] && trEmergencyScreens['err.SECURITY_EMERGENCY_WITHDRAWN']);
});

test('an administrator gets no notice read', async () => {
  serve({ role: 'admin' });
  try {
    await mount(React.createElement(SecurityNoticeBanner));
    assert.equal(calls.length, 0);
    assert.equal(tree.toJSON(), null);
  } finally { await cleanup(); }
});
