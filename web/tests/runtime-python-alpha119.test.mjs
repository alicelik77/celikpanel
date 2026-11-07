import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import ts from 'typescript';
import { dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { enRuntimeAppsScreens as en } from '../src/i18n/screens/runtimeApps/en.ts';
import { trRuntimeAppsScreens as tr } from '../src/i18n/screens/runtimeApps/tr.ts';

// alpha.119, Python apps (W5). Every requirements refusal code the server
// can name has words; a refused line is shown with its file and line in the
// person's language; the Python form sends module:callable, the server and
// the workers; a server without its own Python sends the person to the
// administrator before anything else; the administrator's source-build choice
// changes the summary and the digest that is sent.
//
// Python uygulamaları: her gereksinim reddinin sözcükleri var; reddedilen satır
// dosya ve satır numarasıyla gösterilir; Python'u olmayan sunucuda önce
// yönetici yönlendirilir; kaynaktan derleme seçimi özeti ve özeti değiştirir.

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const moduleURL = (source) => 'data:text/javascript;base64,' + Buffer.from(source).toString('base64');
const lib = await import(moduleURL(ts.transpileModule(read('../src/lib/runtimeApps.ts'), {
  compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 },
}).outputText));
const both = (key) => {
  assert.ok(en[key], `no English words for ${key}`);
  assert.ok(tr[key], `no Turkish words for ${key}`);
};

test('every requirements refusal and Python state has words, and the codes are the server’s', () => {
  const go = read('../../internal/siteruntime/requirements.go');
  const block = go.slice(go.indexOf('const ('), go.indexOf('// ReqRefusalCodes'));
  const fromGo = [...block.matchAll(/Req\w+\s*=\s*"([a-z_]+)"/g)].map((m) => m[1]).sort();
  assert.equal(fromGo.length, 15, fromGo.join(','));
  assert.deepEqual([...lib.RUNTIME_REQ_CODES].sort(), fromGo);
  for (const code of lib.RUNTIME_REQ_CODES) both(`runtime.req.${code}`);
  for (const code of ['runtime_missing', 'src_missing', 'venv_missing', 'requirements_refused', 'wheel_unavailable', 'build_failed', 'pypi_unreachable']) {
    for (const suffix of ['', '.next', '.resume']) both(`runtime.state.${code}${suffix}`);
  }
  assert.equal(lib.RUNTIME_STATE_GUIDANCE.pypi_unreachable.class, 'unknown', 'an unreachable PyPI is not a verdict on the app');
  assert.equal(lib.RUNTIME_STATE_GUIDANCE.runtime_missing.who, 'administrator');
  for (const code of ['pypi_packages', 'pypi_third_party', 'interpreter_exec', 'source_builds']) both(`runtime.risk.${code}`);
  both('runtime.op.step.build');
  // The risk summary names what CelikPanel checks and what it does not.
  assert.match(en['runtime.risk.pypi_third_party'], /hash/);
  assert.match(tr['runtime.risk.pypi_third_party'], /özet/);
});

test('a refused line is read back from the server’s words; the form rules match the server’s', () => {
  assert.deepEqual(lib.parseRequirementsRefusal('requirements.txt:4 extra_index_url: --extra-index-url https://***@x/simple'),
    { file: 'requirements.txt', line: 4, code: 'extra_index_url', text: '--extra-index-url https://***@x/simple' });
  assert.equal(lib.parseRequirementsRefusal('pip stopped with an error'), null);
  assert.equal(lib.parseRequirementsRefusal('a.txt:1 made_up: x'), null);
  for (const good of ['main:app', 'mysite.wsgi:application', '_a.b:c']) assert.ok(lib.callableValid(good), good);
  for (const bad of ['main', 'main:app()', 'a b:c', 'main:$X', '1a:b', 'a-b:c']) assert.ok(!lib.callableValid(bad), bad);
  assert.equal(lib.maxWorkers(64), 1);
  assert.equal(lib.maxWorkers(512), 4);
  assert.equal(lib.maxWorkers(65536), 8);
  // The Go side has the same rule.
  assert.match(read('../../internal/siteruntime/python.go'), /callablePattern = regexp\.MustCompile\(`\^\[A-Za-z_\]\[A-Za-z0-9_\]\{0,63\}\(\\\.\[A-Za-z_\]\[A-Za-z0-9_\]\{0,63\}\)\{0,7\}:\[A-Za-z_\]\[A-Za-z0-9_\]\{0,63\}\$`\)/);
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
  export const RuntimeAppRisk = (props) => React.createElement('div', { 'data-risk': JSON.stringify(props.risk) });
  let n = 0;
  export const newOperationID = () => 'op-test-' + String(++n).padStart(4, '0');
`);
const shared = sharedLayer(stub);
const own = { '/lib/runtimeApps': shared.compile('lib/runtimeApps.ts') };
const { DomainRuntimeApps } = await import(shared.compile('components/DomainRuntimeApps.tsx', own));
const { AdminRuntimeApps } = await import(shared.compile('components/AdminRuntimeApps.tsx', own));

const risk = { can_do: ['any_code'], blocked: ['mail_ports'], network: ['https_only'], remains: ['pypi_third_party'] };
const riskSource = { ...risk, remains: ['pypi_third_party', 'source_builds'] };
const limits = { memory_mb: 512, cpu_percent: 100, tasks_max: 256 };
const programKind = { kind: 'program', implemented: true, offering_id: 'runtime:program', offered: false, refused_by: 'administrator', permission_granted: true, held: false, risk, limits, accepted_level: 'full', accepted_gaps: [] };
const pythonKind = (python) => ({ kind: 'python', implemented: true, offering_id: 'component:python:3.13', offered: true, permission_granted: true, held: false, risk, limits,
  accepted_level: 'full', accepted_gaps: [], python });
const domainView = (python, app = null, operation = null) => ({
  domain: 'example.com', suspended: false, has_site: true, project_type: 'php', certificate: true, disk_full: false,
  sandbox: { known: true, level: 'full', gaps: [] }, kinds: [programKind, pythonKind(python)], app, operation, port_range: [30000, 34999],
});

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
const change = async (id, value) => act(async () => {
  tree.root.find((n) => n.props.id === id).props.onChange({ target: { value } });
  await settle();
});

test('the offered Python kind is chosen first and its form sends module:callable, the server and the workers', async () => {
  await mount(React.createElement(DomainRuntimeApps, { domainId: 1, domainName: 'example.com' }),
    (url, init) => (init.method === 'POST' ? { id: 3, port: 30003 } : domainView({ minor: '3.13', ready: true, max_workers: 4, source_builds: false })));
  try {
    const shown = text();
    assert.ok(shown.includes('runtime.python.create') && shown.includes('runtime.upload.python.step2'), shown);
    assert.ok(shown.includes('runtime.python.wheelsOnly'), 'wheels only is said before the form');
    await change('runtime-create-entry', 'main:create_app()');
    assert.equal(button('runtime.create.button').props.disabled, true, 'a factory call is not a callable');
    await change('runtime-create-entry', 'main:app');
    await change('runtime-create-server', 'asgi');
    await change('runtime-create-workers', '5');
    assert.equal(button('runtime.create.button').props.disabled, true, 'more workers than the memory allows');
    await change('runtime-create-workers', '3');
    const form = tree.root.findAll((n) => n.type === 'form').at(-1);
    await act(async () => { form.props.onSubmit({ preventDefault() {} }); await settle(); });
    const post = requests.find((r) => r.method === 'POST');
    assert.equal(post.url, '/api/v1/domains/1/runtime-apps/create');
    assert.deepEqual(post.body, { kind: 'python', entry: 'main:app', server: 'asgi', workers: 3, health_path: '/' });
  } finally { await unmount(); }
});

test('a server without its own Python sends the person to the administrator before anything else', async () => {
  await mount(React.createElement(DomainRuntimeApps, { domainId: 1, domainName: 'example.com' }),
    () => domainView({ minor: '3.11', ready: false, detail: 'Python’s venv module is not installed', max_workers: 4, source_builds: false }));
  try {
    const shown = text();
    assert.ok(shown.includes('runtime.python.missing') && shown.includes('venv module'), shown);
    assert.ok(!tree.root.findAll((n) => n.type === 'form').length, 'no form while Python is missing');
  } finally { await unmount(); }
});

test('a refused requirements line is shown with its file and line; the build step is named while it runs', async () => {
  const app = { id: 3, kind: 'python', entry: 'main:app', args: [], health_path: '/', port: 30003, state: 'failed', deployed: false, url: 'https://example.com/',
    folder: '/x/app', upload_dir: 'app/src', unit: 'celikrt-3.service', env: [], python: { server: 'asgi', callable: 'main:app', workers: 2 }, states: [], created_at: 'x' };
  const failed = { id: 'op-1', app_id: 3, kind: 'deploy', state: 'failed', step: 'validate', class: 'prerequisite', reason: 'requirements_refused',
    message: 'requirements.txt:2 vcs: git+https://github.com/x/y.git', created_at: 'x', updated_at: 'x' };
  await mount(React.createElement(DomainRuntimeApps, { domainId: 1, domainName: 'example.com' }),
    () => domainView({ minor: '3.13', ready: true, max_workers: 4, source_builds: false }, app, failed));
  try {
    const shown = text();
    assert.ok(shown.includes('runtime.req.line{"file":"requirements.txt","line":2,"why":"runtime.req.vcs"}'), shown);
    assert.ok(shown.includes('runtime.state.requirements_refused.next'), 'the guidance of the state');
    assert.ok(shown.includes('runtime.python.app{"callable":"main:app"}') && shown.includes('runtime.python.server.asgi'), shown);
    assert.ok(!shown.includes('runtime.args'), 'a Python app has no argument list');
  } finally { await unmount(); }
  const running = { id: 'op-2', app_id: 3, kind: 'deploy', state: 'running', step: 'build', created_at: 'x', updated_at: 'x' };
  await mount(React.createElement(DomainRuntimeApps, { domainId: 1, domainName: 'example.com' }),
    () => domainView({ minor: '3.13', ready: true, max_workers: 4, source_builds: false }, app, running));
  try {
    assert.ok(text().includes('runtime.op.step.build'), text());
  } finally { await unmount(); }
});

test('the administrator’s source-build choice changes the summary and the digest sent', async () => {
  const admin = {
    kinds: [{ kind: 'python', offering_id: 'component:python:3.13', implemented: true, risk, risk_digest: 'plain', offered: false, offer_stale: false, held: false,
      limits, accepted_level: 'full', accepted_gaps: [], level_gap: false, apps: 0, running: 0,
      python: { minor: '3.13', ready: true, max_workers: 4, source_builds: false }, source_builds: false,
      risk_source_builds: riskSource, risk_digest_source_builds: 'withsource' }],
    sandbox: { known: true, level: 'full', gaps: [] }, port_range: [30000, 34999],
  };
  await mount(React.createElement(AdminRuntimeApps), () => admin);
  try {
    const shownRisk = () => JSON.parse(tree.root.find((n) => n.props['data-risk'] !== undefined).props['data-risk']);
    assert.deepEqual(shownRisk(), risk);
    const boxes = () => tree.root.findAll((n) => n.type === 'input' && n.props.type === 'checkbox');
    assert.equal(boxes().length, 2, 'source builds and the risk confirmation');
    await act(async () => { boxes()[1].props.onChange({ target: { checked: true } }); await settle(); });
    // Choosing source builds asks to read the summary again.
    await act(async () => { boxes()[0].props.onChange({ target: { checked: true } }); await settle(); });
    assert.deepEqual(shownRisk(), riskSource);
    assert.equal(button('runtime.admin.offer').props.disabled, true);
    await act(async () => { boxes()[1].props.onChange({ target: { checked: true } }); await settle(); });
    await act(async () => { button('runtime.admin.offer').props.onClick(); await settle(); });
    const post = requests.find((r) => r.method === 'POST');
    assert.equal(post.url, '/api/v1/admin/runtime-apps/python/offer');
    assert.equal(post.body.risk_digest, 'withsource');
    assert.equal(post.body.source_builds, true);
  } finally { await unmount(); }
  // Not installed: nothing can be offered, and the page says what to install.
  admin.kinds[0].python = { minor: '3.11', ready: false, detail: 'the venv module is not installed', packages: ['python3', 'python3-venv'], max_workers: 4, source_builds: false };
  await mount(React.createElement(AdminRuntimeApps), () => admin);
  try {
    assert.ok(text().includes('runtime.admin.python.missing') && text().includes('python3 python3-venv'), text());
    assert.equal(button('runtime.admin.offer'), undefined);
  } finally { await unmount(); }
});
