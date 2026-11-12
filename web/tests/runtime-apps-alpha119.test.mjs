import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import ts from 'typescript';
import { dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { enRuntimeAppsScreens as en } from '../src/i18n/screens/runtimeApps/en.ts';
import { trRuntimeAppsScreens as tr } from '../src/i18n/screens/runtimeApps/tr.ts';
import { enServiceAppsScreens } from '../src/i18n/screens/serviceApps/en.ts';
import { trServiceAppsScreens } from '../src/i18n/screens/serviceApps/tr.ts';

// alpha.119, uploaded programs (W4). Every state the server names has its
// reason, who acts, the next action and how work resumes in both languages;
// every failure reason, risk code and refusal has words; a secret value is
// never on screen; only the first read and the person's own Refresh ask the
// program's health path; the tab, the screen part and its budget are wired.
//
// Yüklenen programlar: her durumun nedeni, kimin yapacağı, sonraki adımı ve
// nasıl süreceği iki dilde; gizli değer ekranda yok; sağlık yolu yalnız ilk
// okumada ve Yenile'de sorulur.

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const moduleURL = (source) => 'data:text/javascript;base64,' + Buffer.from(source).toString('base64');
const lib = await import(moduleURL(ts.transpileModule(read('../src/lib/runtimeApps.ts'), {
  compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 },
}).outputText));
const placeholders = (text) => [...text.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();
const both = (key) => {
  assert.ok(en[key], `no English words for ${key}`);
  assert.ok(tr[key], `no Turkish words for ${key}`);
};

test('the part has the same keys and the same values in both languages', () => {
  assert.deepEqual(Object.keys(tr).sort(), Object.keys(en).sort());
  for (const key of Object.keys(en)) {
    assert.ok(key.startsWith('runtime.') || key.startsWith('err.RUNTIME_APP.') || key === 'audit.resource.runtime' || key.startsWith('dashboard.audit.action.runtime.'), `${key} is outside the part's prefixes`);
    assert.deepEqual(placeholders(tr[key]), placeholders(en[key]), `${key}: the two languages use different values`);
  }
  assert.equal(en['runtime.tab'], 'Programs');
  assert.equal(tr['runtime.tab'], 'Programlar');
});

test('every state code has its reason, who acts, the next action and how work resumes', () => {
  // The server's own list (runtimeAppStates and the entry codes) is the one the screen words.
  const handlers = read('../../cmd/panel/runtime_apps_handlers.go');
  const fn = handlers.slice(handlers.indexOf('func runtimeAppStates('), handlers.indexOf('func (p *Panel) writeRuntimeAppsView'));
  const fromGo = new Set([...fn.matchAll(/add\([^,]+,\s*"([a-z_]+)"\)/g)].map((m) => m[1]));
  const entry = read('../../internal/siteruntime/entry.go');
  for (const m of entry.matchAll(/Entry\w+\s*=\s*"([a-z_]+)"/g)) if (m[1] !== 'ok' && m[1] !== 'entry_invalid') fromGo.add(m[1]);
  // alpha.119 W5: the Python kind's states (siteruntime.PythonStateCodes).
  const python = read('../../internal/siteruntime/python.go');
  const pyBlock = python.slice(python.indexOf('const ('), python.indexOf('// PythonStateCodes'));
  for (const m of pyBlock.matchAll(/\w+\s*=\s*"([a-z_]+)"/g)) fromGo.add(m[1]);
  assert.ok(fromGo.size >= 20, `read ${[...fromGo].join(',')}`);
  assert.deepEqual([...fromGo].sort(), [...lib.RUNTIME_STATE_CODES].sort());
  for (const code of lib.RUNTIME_STATE_CODES) {
    for (const suffix of ['', '.next', '.resume']) both(`runtime.state.${code}${suffix}`);
    const guide = lib.RUNTIME_STATE_GUIDANCE[code];
    assert.ok(['verified_failure', 'prerequisite', 'unknown'].includes(guide.class), code);
    both(`runtime.who.${guide.who}`);
    both(`runtime.class.${guide.class}`);
  }
  // A result nobody could check is unknown, never a failure.
  assert.equal(lib.RUNTIME_STATE_GUIDANCE.agent_unreachable.class, 'unknown');
});

test('every failure reason, step, risk code and refusal has words in both languages', () => {
  for (const reason of lib.RUNTIME_OPERATION_REASONS) both(`runtime.reason.${reason}`);
  for (const step of lib.RUNTIME_STEPS) both(`runtime.op.step.${step}`);
  for (const cls of ['verified_failure', 'prerequisite', 'unknown']) both(`runtime.op.failed.${cls}`);
  for (const codes of Object.values(lib.RUNTIME_RISK_CODES)) for (const code of codes) both(`runtime.risk.${code}`);
  for (const reason of lib.RUNTIME_REFUSAL_REASONS) both(`err.RUNTIME_APP.${reason}`);
  // Every reason the Panel writes for a failed operation is a state or has its own words.
  const run = read('../../cmd/panel/runtime_apps_run.go');
  const named = new Set([...run.matchAll(/failRuntimeAppOperation\([^,]+,[^,]+,[^,]+,\s*"([a-z_]+)"/g)].map((m) => m[1]));
  // A deploy whose new version did not come up (failStarted, alpha.119 W5).
  for (const m of run.matchAll(/failStarted\((?:true|false),\s*"([a-z_]+)"/g)) named.add(m[1]);
  assert.ok(named.size >= 8, [...named].join(','));
  for (const reason of named) assert.ok(lib.RUNTIME_STATE_GUIDANCE[reason] || en[`runtime.reason.${reason}`], `no words for ${reason}`);
  // Every RUNTIME_APP refusal the Panel writes has words.
  const handlers = ['runtime_apps_handlers.go', 'runtime_apps_admin.go'].map((f) => read(`../../cmd/panel/${f}`)).join('\n');
  for (const m of handlers.matchAll(/writeRuntimeAppRefusal\(w,\s*http\.Status\w+,\s*"([a-z_]+)"/g)) both(`err.RUNTIME_APP.${m[1]}`);
  // The sandbox gaps reuse the words of the AI apps part.
  for (const gap of ['systemd_old', 'systemd_unknown', 'bpf_missing', 'cgroup_v2_missing', 'nft_cgroup_missing']) {
    assert.ok(enServiceAppsScreens[`svcapps.sandbox.gap.${gap}`] && trServiceAppsScreens[`svcapps.sandbox.gap.${gap}`], gap);
  }
  // What is not covered is said on screen.
  assert.match(en['runtime.notCovered'], /2525/);
  assert.match(en['runtime.notCovered'], /traffic/);
});

test('the audit words cover the twelve actions and the resource', () => {
  const go = read('../../cmd/panel/audit_actions.go');
  const names = [...go.matchAll(/^\t"(runtime\.(?!node)[a-z_]+)",$/gm)].map((m) => m[1]);
  assert.equal(names.length, 12, names.join(','));
  for (const name of names) both(`dashboard.audit.action.${name}`);
  both('audit.resource.runtime');
});

test('the tab, the screen part and its budget are wired', () => {
  const detail = read('../src/components/DomainDetail.tsx');
  assert.match(detail, /!isTeamMember && !isDnsOnly && \(open\('program_apps'\) \|\| open\('python_apps'\) \|\| projectType === 'runtime_app'\) \? \[\{ id: 'programs', labelKey: 'runtime\.tab', icon: Cpu/);
  assert.match(detail, /const DomainRuntimeApps = lazy\(\(\) => import\('\.\/DomainRuntimeApps'\)/);
  assert.doesNotMatch(detail, /^import \{[^}]*DomainRuntimeApps[^}]*\} from/m);
  const addons = read('../src/components/AddonsPage.tsx');
  assert.match(addons, /const AdminRuntimeApps = lazy\(\(\) => import\('\.\/AdminRuntimeApps'\)/);
  const index = read('../src/i18n/screenParts.ts');
  assert.match(index, /import\('\.\/screens\/runtimeApps\/en'\)/);
  assert.match(index, /import\('\.\/screens\/runtimeApps\/tr'\)/);
  assert.match(read('../src/i18n/en.ts'), /\| RuntimeAppsScreenKey/);
  assert.match(read('./locale-catalogue.mjs'), /'runtimeApps'/);
  assert.match(read('../scripts/check-bundle-budget.mjs'), /^ {2}runtimeApps: kib\(/m);
  assert.match(read('../src/lib/rights.ts'), /'service_apps', 'program_apps'/);
});

test('polling only reads, without the health check, and only while an operation runs', () => {
  const source = read('../src/components/DomainRuntimeApps.tsx');
  assert.match(source, /const poll: RemoteHandle<unknown> = \{ remote, reading, retry: \(\) => load\(false\) \};/);
  assert.match(source, /useRefreshEvery\(poll, 4000, running\);/);
  assert.equal((source.match(/load\(true\)/g) ?? []).length, 3, 'first read, Refresh and the retry of a failed read');
  assert.match(source, /operation_id: deployID/);
  assert.match(source, /if \(res\) setDeployID\(newOperationID\(\)\);/);
});

// ---- mounted ----------------------------------------------------------------

const sources = ['DomainRuntimeApps.tsx', 'AdminRuntimeApps.tsx'].map((f) => read(`../src/components/${f}`)).join('\n');
const icons = new Set(['AlertTriangle']);
for (const match of sources.matchAll(/import\s*\{([^}]+)\}\s*from 'lucide-react'/g)) {
  for (const item of match[1].split(',')) if (item.trim()) icons.add(item.trim());
}
const stub = dataModule(`
  import React from '${reactURL}';
  ${[...icons].map((name) => `export const ${name} = () => null;`).join('\n')}
  const i18n = { t: (key, vars) => (vars ? key + JSON.stringify(vars) : key), locale: 'en' };
  export const useI18n = () => i18n;
  export const useNavigate = () => () => true;
  // alpha.127: the administrator's install link; others see the sentence it carries.
  export const AdminInstallLink = (props) => props.fallback ?? null;
  export const showToast = () => {};
  export const RuntimeAppRisk = () => null;
  let n = 0;
  export const newOperationID = () => 'op-test-' + String(++n).padStart(4, '0');
`);
const shared = sharedLayer(stub);
const own = { '/lib/runtimeApps': shared.compile('lib/runtimeApps.ts') };
const { DomainRuntimeApps } = await import(shared.compile('components/DomainRuntimeApps.tsx', own));
const { AdminRuntimeApps } = await import(shared.compile('components/AdminRuntimeApps.tsx', own));

const SECRET = 'TOPSECRET-VALUE-1234';
const risk = { can_do: ['any_code'], blocked: ['mail_ports'], network: ['https_only'], remains: ['mail_apis'] };
const limits = { memory_mb: 512, cpu_percent: 100, tasks_max: 256 };
const view = {
  domain: 'example.com', suspended: false, has_site: true, project_type: 'runtime_app', certificate: true, disk_full: false,
  sandbox: { known: true, level: 'full', gaps: [] },
  kinds: [{ kind: 'program', implemented: true, offering_id: 'runtime:program', offered: true, permission_granted: true, held: false, risk, limits, accepted_level: 'full', accepted_gaps: [] }],
  app: {
    id: 7, kind: 'program', entry: 'server', args: ['--listen=127.0.0.1:{port}'], health_path: '/healthz', port: 30001, state: 'running', deployed: true,
    url: 'https://example.com/', folder: '/var/www/vhosts/example.com/app', upload_dir: 'app/bin', unit: 'celikrt-7.service',
    // A secret must never come with a value; if it ever did, the screen still must not show it.
    env: [{ key: 'API_TOKEN', value: SECRET, secret: true }, { key: 'MODE', value: 'prod', secret: false }],
    status: { unit_file: 'x', unit: { present: true, active: true, enabled: true }, serving: true, health: { checked: true, status: 500, ok: false } },
    states: ['health_failing'], created_at: '2026-11-03 10:00:00',
  },
  operation: null, port_range: [30000, 34999],
};

let tree;
const requests = [];
const originalFetch = globalThis.fetch;
const settle = () => new Promise((resolve) => setTimeout(resolve, 15));
const label = (node) => (typeof node === 'string' ? node : (node.children || []).map(label).join(''));
const text = () => {
  const walk = (node) => (typeof node === 'string' ? node : !node ? '' : Array.isArray(node) ? node.map(walk).join(' ') : walk(node.children));
  return walk(tree.toJSON());
};
const button = (key) => tree.root.findAll((node) => node.type === 'button').find((b) => label(b).includes(key));
async function mount(element, answer) {
  requests.length = 0;
  globalThis.document = { addEventListener() {}, removeEventListener() {} };
  globalThis.fetch = async (input, init = {}) => {
    requests.push({ method: init.method || 'GET', url: String(input), body: init.body ? JSON.parse(init.body) : undefined });
    return new Response(JSON.stringify(answer(String(input), init)), { status: 200, headers: { 'Content-Type': 'application/json' } });
  };
  await act(async () => { tree = Renderer.create(element); });
  await act(async () => { await settle(); });
}
async function unmount() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  delete globalThis.document;
}

test('the states come first with their guidance; the health check is asked only on the first read and on Refresh', async () => {
  await mount(React.createElement(DomainRuntimeApps, { domainId: 1, domainName: 'example.com' }), (url) => (url.includes('/control') ? { state: 'stopped' } : view));
  try {
    const gets = () => requests.filter((r) => r.method === 'GET').map((r) => r.url);
    assert.deepEqual(gets(), ['/api/v1/domains/1/runtime-apps?health=1']);
    const shown = text();
    assert.ok(shown.includes('runtime.state.health_failing') && shown.includes('runtime.state.health_failing.next') && shown.includes('runtime.state.health_failing.resume'), shown);
    assert.ok(shown.indexOf('runtime.state.health_failing') < shown.indexOf('runtime.app.entry'), 'the state is above the program');
    await act(async () => { button('runtime.stop').props.onClick(); await settle(); });
    assert.deepEqual(requests.filter((r) => r.method === 'POST').map((r) => [r.url, r.body]), [['/api/v1/domains/1/runtime-apps/control', { action: 'stop' }]]);
    assert.equal(gets().at(-1), '/api/v1/domains/1/runtime-apps', 'a read after a change does not ask the health path');
    await act(async () => { button('runtime.refresh').props.onClick(); await settle(); });
    assert.equal(gets().at(-1), '/api/v1/domains/1/runtime-apps?health=1');
  } finally { await unmount(); }
});

test('a secret value is never on screen and a kept secret is sent without a value', async () => {
  await mount(React.createElement(DomainRuntimeApps, { domainId: 1, domainName: 'example.com' }), (url, init) => (init.method === 'PUT' ? { keys: ['API_TOKEN', 'MODE'] } : view));
  try {
    await act(async () => { button('runtime.env').props.onClick(); await settle(); });
    assert.ok(text().includes('runtime.env.secretSet'), text());
    assert.ok(!JSON.stringify(tree.toJSON()).includes(SECRET), 'the secret value reached the screen');
    const secretInput = tree.root.findAll((n) => n.type === 'input' && n.props.type === 'password');
    assert.equal(secretInput.length, 1);
    assert.equal(secretInput[0].props.value, '');
    const form = tree.root.findAll((n) => n.type === 'form' && typeof n.props.onSubmit === 'function').at(-1);
    // alpha.132: Save is offered only for a change.
    await act(async () => { form.props.onSubmit({ preventDefault() {} }); await settle(); });
    assert.equal(requests.filter((r) => r.method === 'PUT').length, 0, 'an unchanged list was sent');
    const mode = tree.root.find((n) => n.type === 'input' && n.props.value === 'prod');
    await act(async () => { mode.props.onChange({ target: { value: 'staging' } }); await settle(); });
    assert.ok(tree.root.findAll((n) => n.props['data-changed-mark'] !== undefined).length === 1, 'the changed row is not marked');
    await act(async () => { form.props.onSubmit({ preventDefault() {} }); await settle(); });
    const put = requests.find((r) => r.method === 'PUT');
    assert.equal(put.url, '/api/v1/domains/1/runtime-apps/env');
    assert.deepEqual(put.body, { entries: [{ key: 'API_TOKEN', secret: true }, { key: 'MODE', value: 'staging', secret: false }] });
  } finally { await unmount(); }
});

test('the administrator offers on a reduced server only after accepting every named gap', async () => {
  const admin = {
    kinds: [{ kind: 'program', offering_id: 'runtime:program', implemented: true, risk, risk_digest: 'd1', offered: false, offer_stale: false, held: false,
      limits, accepted_level: 'full', accepted_gaps: [], level_gap: false, apps: 0, running: 0 }],
    sandbox: { known: true, level: 'reduced', gaps: ['bpf_missing', 'nft_cgroup_missing'] }, port_range: [30000, 34999],
  };
  await mount(React.createElement(AdminRuntimeApps), () => admin);
  try {
    const offer = () => button('runtime.admin.offer');
    const boxes = () => tree.root.findAll((n) => n.type === 'input' && n.props.type === 'checkbox');
    const tick = async (box) => act(async () => { box.props.onChange({ target: { checked: true } }); await settle(); });
    assert.equal(boxes().length, 3);
    assert.equal(offer().props.disabled, true);
    await tick(boxes()[2]);
    assert.equal(offer().props.disabled, true, 'reading is not enough on a reduced server');
    await tick(boxes()[0]);
    assert.equal(offer().props.disabled, true, 'one gap is not every gap');
    await tick(boxes()[1]);
    assert.equal(offer().props.disabled, false);
    await act(async () => { offer().props.onClick(); await settle(); });
    const post = requests.find((r) => r.method === 'POST');
    assert.equal(post.url, '/api/v1/admin/runtime-apps/program/offer');
    assert.deepEqual(post.body, { risk_digest: 'd1', memory_mb: 512, cpu_percent: 100, tasks_max: 256, accepted_gaps: ['bpf_missing', 'nft_cgroup_missing'] });
  } finally { await unmount(); }
});
