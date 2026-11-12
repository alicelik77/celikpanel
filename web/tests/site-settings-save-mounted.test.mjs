import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';

// One way of saving on a site's settings screens (alpha.132; owner, 8 Nov
// 2026): a setting is never saved by clicking it. Mounted with the shipped
// shared layer and the shipped saving parts (components/kit/save), each
// converted block is driven the way a person does: changing a setting sends
// nothing and marks it; Save sends exactly the changes; Discard puts the
// server's values back; a refusal keeps the change; an answer that was lost
// is said and sent nothing a second time.
// Bir sitenin ayar ekranlarında tek kaydetme yolu: ayar tıklanınca kaydedilmez.

const files = ['DomainSSLSettings', 'WordPressToolsPanel', 'DomainMalwareScan', 'DomainWebSettings', 'NodeSiteTools', 'DomainRuntimeApps', 'HostingTypePanel', 'DomainPHPSettings', 'SiteToolGuidance'];
const icons = new Set(['AlertTriangle', 'XCircle']);
for (const name of files) {
  const text = readFileSync(new URL(`../src/components/${name}.tsx`, import.meta.url), 'utf8');
  for (const match of text.matchAll(/import\s*\{([^}]+)\}\s*from 'lucide-react'/g)) {
    for (const item of match[1].split(',')) {
      const icon = item.replace(/\btype\b/, '').trim().split(/\s+as\s+/)[0];
      if (icon && icon !== 'LucideIcon') icons.add(icon);
    }
  }
}
const stub = dataModule(`
  import React from '${reactURL}';
  ${[...icons].map((name) => `export const ${name} = () => null;`).join('\n')}
  const t = (key, values) => (values ? key + JSON.stringify(values) : key);
  const i18n = { t, locale: 'en' };
  export const useI18n = () => i18n;
  export const useWebWords = () => i18n;
  export const useNavigate = () => () => {};
  export const showToast = (...args) => { (globalThis.toasts ??= []).push(args); };
  export const AdminInstallLink = () => null;
  export const CertificateIssueNotice = () => null;
  export const isCertificateIssueFailure = () => false;
  export const CertificateReachNotice = () => null;
  export const AppAccountLine = () => null;
  export const NodeSiteSandbox = () => null;
  export const OfferedVersionNote = () => null;
  export const RuntimeAppRisk = () => null;
  let n = 0;
  export const newOperationID = () => 'op-' + (++n);
`);
const shared = sharedLayer(stub);
const own = {
  '/lib/certificateNames': compileSource('lib/certificateNames.ts', () => stub),
  '/lib/sslTier': compileSource('lib/sslTier.ts', () => stub),
  '/lib/sslRenewal': compileSource('lib/sslRenewal.ts', () => stub),
  '/lib/hstsRemaining': compileSource('lib/hstsRemaining.ts', () => stub),
  '/lib/runtimeApps': shared.compile('lib/runtimeApps.ts'),
};
own['/SiteToolGuidance'] = shared.compile('components/SiteToolGuidance.tsx', own);
const load = async (name) => (await import(shared.compile(`components/${name}.tsx`, own)))[name];
const DomainSSLSettings = await load('DomainSSLSettings');
const WordPressToolsPanel = await load('WordPressToolsPanel');
const DomainMalwareScan = await load('DomainMalwareScan');
const DomainWebSettings = await load('DomainWebSettings');
const NodeSiteTools = await load('NodeSiteTools');
const DomainRuntimeApps = await load('DomainRuntimeApps');
const HostingTypePanel = await load('HostingTypePanel');
const DomainPHPSettings = await load('DomainPHPSettings');
const { askToLeave } = await import(shared.resolve('/leaveAsker'));
const { saveWords } = await import(shared.resolve('/words'));

// --- The harness ------------------------------------------------------------------
let tree;
let sent = [];
const originalFetch = globalThis.fetch;
const settle = async () => { await act(async () => { for (let i = 0; i < 4; i++) await new Promise((resolve) => setTimeout(resolve, 5)); }); };
const json = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
// routes: 'METHOD /path' or '/path' → (init) => a Response, a body, or a thrown drop.
function serve(routes) {
  sent = [];
  globalThis.toasts = [];
  globalThis.fetch = async (input, init = {}) => {
    const method = init.method || 'GET';
    const path = String(input).split('?')[0];
    if (method !== 'GET') sent.push({ method, path, body: typeof init.body === 'string' ? JSON.parse(init.body) : undefined });
    const route = routes[`${method} ${path}`] ?? routes[path];
    if (!route) return json({ error: 'not served' }, 404);
    const answer = await route(init);
    return answer instanceof Response ? answer : json(answer);
  };
}
async function mount(element) {
  globalThis.document ??= { addEventListener() {}, removeEventListener() {} };
  await act(async () => { tree = Renderer.create(element); });
  await settle();
}
async function unmount() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
}
const words = (node) => (typeof node === 'string' ? node : !node ? '' : Array.isArray(node) ? node.map(words).join(' ') : words(node.children));
const text = () => words(tree.toJSON());
const nodeText = (node) => (typeof node === 'string' ? node : (node.children || []).map(nodeText).join(''));
const marks = () => tree.root.findAll((node) => node.props['data-changed-mark'] !== undefined && typeof node.type === 'string').length;
const saveButton = () => tree.root.findAll((node) => node.type === 'button' && node.props['data-save'] !== undefined)[0];
const discardButton = () => tree.root.findAll((node) => node.type === 'button' && node.props['data-save-discard'] !== undefined)[0];
const button = (key) => tree.root.findAll((node) => node.type === 'button').find((node) => nodeText(node).includes(key));
// The checkbox of a labelled switch.
const switchOf = (key) => {
  const label = tree.root.findAll((node) => node.type === 'label').find((node) => nodeText(node).includes(key));
  assert.ok(label, `no switch ${key}`);
  return label.findAll((node) => node.type === 'input' && node.props.type === 'checkbox')[0];
};
const flip = async (input) => { await act(async () => { input.props.onChange({ target: { checked: !input.props.checked } }); }); };
const type = async (input, value) => { await act(async () => { input.props.onChange({ target: { value } }); }); };
const press = async (node) => { assert.ok(node, 'no such button'); assert.ok(!node.props.disabled, 'the button is off'); await act(async () => { node.props.onClick({ preventDefault() {} }); }); await settle(); };
const pending = (n) => (n === 1 ? saveWords.en.pendingOne : saveWords.en.pending.replace('{n}', String(n)));

// --- Certificate settings -----------------------------------------------------------
const certificate = {
  id: 1, type: 'letsencrypt', provider_id: 'letsencrypt', issuer: "Let's Encrypt", subject: 'example.com', issued_at: '2026-11-01T00:00:00Z', expires_at: '2027-01-30T00:00:00Z',
  days_until_expiry: 60, auto_renew: true, renewal_status: 'ok', status: 'valid', dns_names: ['example.com', 'www.example.com', 'mail.example.com'],
  activated: true, usable: true, trust_status: 'trusted', activation_pending: false, dependents_pending: false,
};
function sslServer({ settings = {}, refuse = {}, drop = {} } = {}) {
  const state = { auto_renew: true, force_https: false, hsts_enabled: false, hsts_max_age: 300, secure_mail: false, ...settings };
  const part = (name, apply) => async (init) => {
    if (drop[name]) throw new TypeError('connection dropped');
    if (refuse[name]) return json({ error: 'refused by the server', code: 'SSL_SETTINGS_REFUSED' }, 409);
    apply(JSON.parse(init.body));
    return { success: true };
  };
  serve({
    'GET /api/v1/domains/1/ssl': () => ({
      domain_id: 1, domain_name: 'example.com', has_certificate: true, managed_names: ['example.com', 'www.example.com'],
      certificate: { ...certificate, auto_renew: state.auto_renew },
      settings: { force_https: state.force_https, hsts_enabled: state.hsts_enabled, hsts_max_age: state.hsts_max_age },
    }),
    'GET /api/v1/domains/1/ssl/mail': () => ({ secure_mail: state.secure_mail }),
    'GET /api/v1/ssl/providers': () => ({ providers: [{ id: 'letsencrypt', name: "Let's Encrypt", note: '', needs_eab: false }] }),
    'PUT /api/v1/domains/1/ssl/renewal': part('renewal', (body) => { state.auto_renew = body.auto_renew; }),
    'POST /api/v1/domains/1/ssl/settings': part('https', (body) => { Object.assign(state, body); }),
    'PUT /api/v1/domains/1/ssl/mail': part('mail', (body) => { state.secure_mail = body.secure_mail; }),
  });
  return state;
}
const ssl = () => React.createElement(DomainSSLSettings, { domainId: 1, domainName: 'example.com', mailAvailable: true });

test('certificate settings: a switch sends nothing and is marked; Discard puts the server’s values back', async () => {
  sslServer();
  await mount(ssl());
  try {
    assert.equal(saveButton(), undefined, 'Save is offered with nothing changed');
    await flip(switchOf('ssl.forceHttps'));
    await flip(switchOf('ssl.autoRenewOn'));
    assert.deepEqual(sent, [], 'a switch sent something');
    assert.equal(marks(), 2);
    assert.ok(text().includes(pending(2)), text().slice(0, 300));
    assert.equal(switchOf('ssl.forceHttps').props.checked, true);
    await press(discardButton());
    assert.equal(switchOf('ssl.forceHttps').props.checked, false);
    assert.equal(switchOf('ssl.autoRenewOn').props.checked, true);
    assert.equal(saveButton(), undefined);
    assert.deepEqual(sent, []);
  } finally { await unmount(); }
});

test('certificate settings: turning HSTS on forces HTTPS and starts with the short trial; taking it back takes both back', async () => {
  sslServer({ settings: { hsts_max_age: 86400 } });
  await mount(ssl());
  try {
    await flip(switchOf('ssl.hsts'));
    assert.equal(switchOf('ssl.forceHttps').props.checked, true);
    assert.equal(switchOf('ssl.forceHttps').props.disabled, true, 'HTTPS can be turned off while HSTS is on');
    const duration = tree.root.findAll((node) => node.type === 'select').find((node) => node.props.value === 300);
    assert.ok(duration, 'HSTS does not start with the five-minute trial');
    await flip(switchOf('ssl.hsts'));
    assert.equal(switchOf('ssl.forceHttps').props.checked, false);
    assert.equal(saveButton(), undefined, 'something is still counted as changed');
    assert.deepEqual(sent, []);
  } finally { await unmount(); }
});

test('certificate settings: Save sends exactly the changed parts, in order, the mail certificate in the progress window', async () => {
  const state = sslServer();
  await mount(ssl());
  try {
    await flip(switchOf('ssl.autoRenewOn'));
    await flip(switchOf('ssl.forceHttps'));
    await flip(switchOf('ssl.secureMail'));
    globalThis.operationSteps = [];
    await press(saveButton());
    assert.deepEqual(sent.map((call) => `${call.method} ${call.path}`), [
      'PUT /api/v1/domains/1/ssl/renewal', 'POST /api/v1/domains/1/ssl/settings', 'PUT /api/v1/domains/1/ssl/mail',
    ]);
    assert.deepEqual(sent[0].body, { auto_renew: false });
    assert.deepEqual(sent[1].body, { force_https: true, hsts_enabled: false, hsts_max_age: 300 });
    assert.deepEqual(sent[2].body, { secure_mail: true });
    assert.ok(globalThis.operationSteps.length > 0, 'the mail certificate did not run in the progress window');
    assert.deepEqual(state, { auto_renew: false, force_https: true, hsts_enabled: false, hsts_max_age: 300, secure_mail: true });
    assert.ok(globalThis.toasts.some(([tone, message]) => tone === 'success' && message === 'ssl.settingsSaved'));
    assert.equal(saveButton(), undefined, 'the bar stayed after everything was saved');
    assert.equal(tree.root.findAll((node) => node.props['data-ssl-save-report'] !== undefined).length, 0);
  } finally { await unmount(); }
});

test('certificate settings: a refused part stops the rest; the page says what was saved, refused and not sent, and keeps the rest unsaved', async () => {
  sslServer({ refuse: { https: true } });
  await mount(ssl());
  try {
    await flip(switchOf('ssl.autoRenewOn'));
    await flip(switchOf('ssl.forceHttps'));
    await flip(switchOf('ssl.secureMail'));
    await press(saveButton());
    assert.deepEqual(sent.map((call) => call.path), ['/api/v1/domains/1/ssl/renewal', '/api/v1/domains/1/ssl/settings'], 'a part after the refused one was sent');
    const report = text();
    assert.ok(report.includes('ssl.save.saved{"parts":"ssl.save.part.renewal"}'), report.slice(0, 600));
    assert.ok(report.includes('ssl.save.refused{"part":"ssl.save.part.https"'), 'the refused part is not named');
    assert.ok(report.includes('ssl.save.notSent{"parts":"ssl.save.part.mail"}'), 'what was not sent is not named');
    assert.ok(report.includes('ssl.save.next'));
    // The renewal is saved (the server holds it); HTTPS and mail stay unsaved.
    assert.equal(switchOf('ssl.autoRenewOn').props.checked, false);
    assert.equal(marks(), 2);
    assert.ok(report.includes(pending(2)));
    // Save again sends only what is left.
    sent = [];
    await press(saveButton());
    assert.deepEqual(sent.map((call) => call.path), ['/api/v1/domains/1/ssl/settings'], 'the saved renewal was sent again');
  } finally { await unmount(); }
});

test('certificate settings: a lost answer is unknown, nothing is sent twice, and the rest is not sent', async () => {
  sslServer({ drop: { https: true } });
  await mount(ssl());
  try {
    await flip(switchOf('ssl.forceHttps'));
    await flip(switchOf('ssl.secureMail'));
    await press(saveButton());
    assert.deepEqual(sent.map((call) => call.path), ['/api/v1/domains/1/ssl/settings']);
    const shown = text();
    assert.ok(shown.includes('ssl.save.unknown{"part":"ssl.save.part.https"}'), shown.slice(0, 600));
    assert.ok(shown.includes('ssl.save.notSent{"parts":"ssl.save.part.mail"}'));
    assert.ok(!shown.includes('ssl.save.next'), 'a lost answer is told to fix a reason');
    assert.ok(shown.includes(saveWords.en.unknown.slice(0, 40)), 'the bar does not say the result is unknown');
    assert.ok(!globalThis.toasts.some(([tone]) => tone === 'success'), 'an unknown result was reported as saved');
    assert.equal(marks(), 2, 'the changes were dropped although the result is not known');
  } finally { await unmount(); }
});

// --- WordPress: automatic minor releases ---------------------------------------------
const wordpress = (auto) => ({
  project_type: 'php', auto_minor_core: auto, database_linked: true, operations: [],
  status: { detected: true, core_version: '6.8.3', plugins: [], themes: [], updates_checked: true },
});

test('WordPress automatic updates: ticking sends nothing; Save sends it; a refusal keeps it; Discard restores', async () => {
  let auto = false;
  let refuse = true;
  serve({
    'GET /api/v1/wordpress/1': () => wordpress(auto),
    'GET /api/v1/wordpress/1/clone-targets': () => ({ targets: [] }),
    'PUT /api/v1/wordpress/1/auto-update': (init) => {
      if (refuse) return json({ error: 'not allowed', code: 'WORDPRESS_TOOLS_NOT_OFFERED' }, 403);
      auto = JSON.parse(init.body).minor_core;
      return { operation_id: '' };
    },
  });
  await mount(React.createElement(WordPressToolsPanel, { domainId: 1 }));
  try {
    await flip(switchOf('wpt.autoMinor'));
    assert.deepEqual(sent, []);
    assert.equal(marks(), 1);
    await press(saveButton());
    assert.deepEqual(sent.map((call) => [call.method, call.path, call.body]), [['PUT', '/api/v1/wordpress/1/auto-update', { minor_core: true }]]);
    assert.ok(globalThis.toasts.some(([tone]) => tone === 'error'), 'the refusal is not said');
    assert.equal(switchOf('wpt.autoMinor').props.checked, true, 'a refused change was dropped');
    refuse = false;
    sent = [];
    await press(saveButton());
    assert.equal(sent.length, 1);
    assert.equal(auto, true);
    assert.equal(saveButton(), undefined);
    await flip(switchOf('wpt.autoMinor'));
    await press(discardButton());
    assert.equal(switchOf('wpt.autoMinor').props.checked, true);
  } finally { await unmount(); }
});

// --- The weekly malware scan ---------------------------------------------------------
test('weekly scan: ticking sends nothing; a lost answer holds Save until the schedule is read again', async () => {
  let enabled = false;
  let drop = true;
  serve({
    'GET /api/v1/malware/domains/1': () => ({ subscription_id: 4, is_admin: true, scanner: { state: 'ready' }, scans: [], findings: [] }),
    'GET /api/v1/malware/schedule': () => ({ enabled }),
    'PUT /api/v1/malware/schedule': (init) => {
      if (drop) throw new TypeError('connection dropped');
      enabled = JSON.parse(init.body).enabled;
      return { enabled };
    },
  });
  await mount(React.createElement(DomainMalwareScan, { domainId: 1 }));
  try {
    await flip(switchOf('mscan.schedule'));
    assert.deepEqual(sent, []);
    assert.equal(marks(), 1);
    await press(saveButton());
    assert.equal(sent.length, 1);
    assert.ok(text().includes(saveWords.en.unknown.slice(0, 40)), 'the lost answer is not said');
    assert.equal(saveButton().props.disabled, true, 'Save is offered before the schedule was read again');
    assert.equal(switchOf('mscan.schedule').props.checked, true, 'the change was dropped');
    drop = false;
    const check = tree.root.findAll((node) => node.type === 'button' && node.props['data-save-check'] !== undefined)[0];
    await press(check);
    assert.equal(sent.length, 1, 'checking again sent the change again');
    assert.equal(saveButton().props.disabled, false);
    await press(saveButton());
    assert.equal(enabled, true);
    assert.equal(saveButton(), undefined);
  } finally { await unmount(); }
});

// --- The web server settings ----------------------------------------------------------
const webView = (settings = {}) => ({
  project_type: 'php', index_applies: true, fields_apply: true, node_applies: false, page_cache_applies: false, default_index: 'index.php index.html',
  settings: { browser_cache_days: 0, ...settings }, cache_choices: [0, 1, 7, 30], error_codes: [404, 500],
  directives: { may_edit: true, set: false, text: '', max_size: 8192 },
});

test('web server settings: a change sends nothing; Save sends the settings; a form that cannot be saved says why; Discard restores', async () => {
  let saved = webView();
  serve({
    'GET /api/v1/domains/1/web-settings': () => saved,
    'POST /api/v1/domains/1/web-settings': (init) => { saved = webView(JSON.parse(init.body).settings); return { success: true }; },
  });
  await mount(React.createElement(DomainWebSettings, { domainId: 1, domainName: 'example.com' }));
  try {
    assert.equal(saveButton(), undefined);
    const cache = tree.root.findAll((node) => node.type === 'select').find((node) => node.props['aria-label'] === 'web.cache.title');
    await type(cache, '7');
    assert.deepEqual(sent, []);
    assert.equal(marks(), 1);
    // A header with a name and no value: Save is off and the bar says why.
    await press(button('web.headers.add'));
    const name = tree.root.findAll((node) => node.type === 'input' && node.props['aria-label'] === 'web.headers.name')[0];
    await type(name, 'X-Test');
    assert.ok(text().includes('web.problem.header'), 'the reason Save is off is not said');
    assert.equal(saveButton().props.disabled, true);
    await press(tree.root.findAll((node) => node.type === 'button' && node.props['aria-label'] === 'web.remove')[0]);
    assert.equal(saveButton().props.disabled, false);
    await press(saveButton());
    assert.deepEqual(sent, [{ method: 'POST', path: '/api/v1/domains/1/web-settings', body: { settings: { browser_cache_days: 7 } } }]);
    assert.equal(saveButton(), undefined, 'the bar stayed after the settings were saved');
    await type(tree.root.findAll((node) => node.type === 'select').find((node) => node.props['aria-label'] === 'web.cache.title'), '30');
    await press(discardButton());
    assert.equal(tree.root.findAll((node) => node.type === 'select').find((node) => node.props['aria-label'] === 'web.cache.title').props.value, 7);
  } finally { await unmount(); }
});

// --- A Node.js site's environment ------------------------------------------------------
test('Node.js variables: editing sends nothing; Save sends the list; Discard puts the saved list back', async () => {
  serve({
    'GET /api/v1/domains/1/app/env': () => ({ entries: [{ key: 'MODE', value: 'prod', secret: false }], min_secret: 16 }),
    'GET /api/v1/domains/1/app/packages': () => ({ phase: 'idle', scripts: [] }),
    'POST /api/v1/domains/1/app/env': () => ({ restarted: true }),
  });
  await mount(React.createElement(NodeSiteTools, { domainId: 1, domainName: 'example.com' }));
  try {
    assert.equal(saveButton(), undefined);
    const value = () => tree.root.findAll((node) => node.type === 'input' && node.props['aria-label'] === 'node.env.value')[0];
    await type(value(), 'staging');
    assert.deepEqual(sent, []);
    assert.ok(text().includes(saveWords.en.pendingSome));
    await press(discardButton());
    assert.equal(value().props.value, 'prod');
    await type(value(), 'staging');
    await press(saveButton());
    assert.deepEqual(sent, [{ method: 'POST', path: '/api/v1/domains/1/app/env', body: { entries: [{ key: 'MODE', value: 'staging', secret: false }] } }]);
  } finally { await unmount(); }
});

// --- The hosting type ----------------------------------------------------------------------
test('hosting type: picking a kind applies nothing, names the switch, asks before leaving, and Discard takes it back', async () => {
  serve({
    'GET /api/v1/domains/1/hosting': () => ({ project_type: 'php', php_version: '8.3' }),
    'GET /api/v1/hosting/capabilities': () => ({ web_server: 'nginx', php_versions: ['8.3'], dns_server: 'bind', mail_server: true, database_servers: [] }),
  });
  await mount(React.createElement(HostingTypePanel, { domainId: 1, domainName: 'example.com' }));
  try {
    assert.equal(saveButton(), undefined, 'an apply button is offered with nothing chosen');
    await press(button('hosting.type.static'));
    assert.deepEqual(sent, [], 'choosing a kind applied it');
    assert.ok(nodeText(saveButton()).includes('hosting.switchTo.static'), 'the button does not name the switch');
    assert.ok(text().includes('hosting.staged'));
    assert.equal(marks(), 1);
    const asked = [];
    globalThis.confirm = (_text, request) => { asked.push(request); return false; };
    assert.equal(await askToLeave(), false, 'leaving with a chosen kind did not ask');
    assert.equal(asked.length, 1);
    await press(discardButton());
    assert.equal(saveButton(), undefined);
    assert.equal(askToLeave(), null, 'leaving still asks after Discard');
  } finally { await unmount(); delete globalThis.confirm; }
});

// --- A program's settings -----------------------------------------------------------------
test('program settings: editing sends nothing; Save sends the settings; closing with a change asks', async () => {
  const app = {
    id: 7, kind: 'program', entry: 'server', args: ['--port={port}'], health_path: '/healthz', port: 30001, state: 'stopped', deployed: false,
    url: 'https://example.com/', folder: '/x/app', upload_dir: 'app/bin', unit: 'celikrt-7.service', env: [], states: [], created_at: 'x',
  };
  const view = {
    domain: 'example.com', suspended: false, has_site: true, project_type: 'runtime_app', certificate: true, disk_full: false,
    sandbox: { known: true, level: 'full', gaps: [] }, port_range: [30000, 34999], operation: null, app,
    kinds: [{ kind: 'program', implemented: true, offering_id: 'runtime:program', offered: true, permission_granted: true, held: false, risk: { can_do: [], blocked: [], network: [], remains: [] }, limits: { memory_mb: 512, cpu_percent: 100, tasks_max: 256 }, accepted_level: 'full', accepted_gaps: [] }],
  };
  serve({
    'GET /api/v1/domains/1/runtime-apps': () => view,
    'PUT /api/v1/domains/1/runtime-apps/settings': () => ({ success: true }),
  });
  await mount(React.createElement(DomainRuntimeApps, { domainId: 1, domainName: 'example.com' }));
  try {
    await press(button('runtime.settings.edit'));
    assert.equal(saveButton(), undefined);
    const health = () => tree.root.findAll((node) => node.type === 'input' && node.props.id === 'runtime-edit-health')[0];
    await type(health(), '/ready');
    assert.deepEqual(sent, []);
    assert.equal(marks(), 1);
    // Closing the form with a change asks; staying keeps it.
    const asked = [];
    globalThis.confirm = (_text, request) => { asked.push(request); return false; };
    await press(button('runtime.settings.edit'));
    assert.equal(asked.length, 1, 'closing the form with a change did not ask');
    assert.equal(health().props.value, '/ready');
    await press(saveButton());
    assert.deepEqual(sent, [{ method: 'PUT', path: '/api/v1/domains/1/runtime-apps/settings', body: { kind: 'program', entry: 'server', args: ['--port={port}'], health_path: '/ready' } }]);
  } finally { await unmount(); delete globalThis.confirm; }
});

// --- The PHP pool ------------------------------------------------------------------------------
test('PHP pool: editing sends nothing; an incomplete pool says why Save is off; Save sends the whole pool; Discard restores', async () => {
  const pool = { pm: 'ondemand', pm_max_children: 9, pm_start_servers: 3, pm_min_spare_servers: 2, pm_max_spare_servers: 4, user: 'site1', group: 'site1' };
  serve({
    'GET /api/v1/domains/1/php': () => ({ php_version: '8.3', pool_name: 'example_com', available_versions: ['8.3'], pool_config: pool }),
    'GET /api/v1/hosting/capabilities': () => ({ web_server: 'nginx', php_versions: ['8.3'], dns_server: 'bind', mail_server: true, database_servers: [] }),
    'POST /api/v1/domains/1/php/pool': () => ({ success: true }),
  });
  await mount(React.createElement(DomainPHPSettings, { domainId: 1, domainName: 'example.com', currentVersion: '8.3', onVersionChange() {}, isAdmin: true }));
  try {
    const field = (name) => tree.root.findAll((node) => node.props.id === `php-pool-${name}` && typeof node.type === 'string')[0];
    await type(field('pm_max_children'), '12');
    assert.deepEqual(sent, []);
    assert.equal(marks(), 1);
    await type(field('user'), '');
    assert.ok(text().includes('php.poolInvalid'), 'the reason Save is off is not said');
    assert.equal(saveButton().props.disabled, true);
    await type(field('user'), 'site1');
    await press(saveButton());
    assert.deepEqual(sent, [{ method: 'POST', path: '/api/v1/domains/1/php/pool', body: { version: '8.3', pool_config: { ...pool, pm_max_children: 12 } } }]);
    await type(field('pm_max_children'), '4');
    await press(discardButton());
    assert.notEqual(field('pm_max_children').props.value, '4', 'Discard did not put the saved value back');
  } finally { await unmount(); }
});
