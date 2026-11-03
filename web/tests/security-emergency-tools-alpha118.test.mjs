import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { enEmergencyScreens } from '../src/i18n/screens/emergency/en.ts';
import { trEmergencyScreens } from '../src/i18n/screens/emergency/tr.ts';

// alpha.118, the owner's Boston screens: under MariaDB the closed "Security
// emergency" section, red with a crossed-out shield, read as an error; a
// click on it opened MariaDB's page instead of the section, because the row's
// link stretches over the whole row and the section did not sit above it.
// While no emergency is open the section is now a quiet tool with a one-line
// hint; it turns red only when an emergency is open for its component; and it
// sits above the row link, so a click only opens or closes it.
// Acil durum bölümü, açık acil durum yokken sakin bir araçtır; tıklama yalnız açar/kapar.
const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const icons = new Set(['AlertTriangle', 'XCircle']);
for (const name of ['ui.tsx', 'SecurityEmergency.tsx']) {
  for (const match of source(`components/${name}`).matchAll(/import\s*\{([^}]+)\}\s*from 'lucide-react'/g)) {
    for (const item of match[1].split(',')) {
      const icon = item.replace(/\btype\b/, '').trim().split(/\s+as\s+/)[0];
      if (icon && icon !== 'LucideIcon') icons.add(icon);
    }
  }
}
const stub = dataModule(`
  import React from '${reactURL}';
  ${[...icons].map((name) => `export const ${name} = (props) => React.createElement('i', { 'data-icon': '${name}', className: props.className });`).join('\n')}
  export const useI18n = () => ({ t: (key, vars) => key + (vars ? JSON.stringify(vars) : ''), locale: 'en' });
  export const useNavigate = () => () => { globalThis.emergencyNavigated = true; };
  export const useAuth = () => ({ role: 'admin' });
  export const showToast = () => {};
`);
const shared = sharedLayer(stub);
const { SecurityEmergencySection } = await import(shared.compile('components/SecurityEmergency.tsx'));

const emergency = (component, stage = 'withdrawn') => ({
  emergency: { id: 'e-' + component, component, version: '', name: component === 'mariadb' ? 'MariaDB' : component, reason: 'CVE-2026-0003', stage, withdrawn: true },
  guidance: { result: 'Withdrawn.', reason: 'CVE-2026-0003', who_acts: 'administrator', next_action: 'Stop it if the risk remains.', resumes: '' },
  operation: null,
  preview: { sites: [], customers: [], resellers: [], offered_by: [], notes: [] },
  notices: { total: 0, seen: 0, mail_sent: 0, mail_pending: 0, mail_failed: 0, no_address: 0, mail_not_available: 0 },
  confirm_name: 'MariaDB', alternatives: [], can: { stop: true },
});

const originalFetch = globalThis.fetch;
let tree, calls;
function serve(list) {
  calls = [];
  globalThis.document = { addEventListener() {}, removeEventListener() {} };
  globalThis.fetch = (url, options = {}) => {
    calls.push(`${options.method || 'GET'} ${url}`);
    if (String(url) === '/api/v1/security-emergencies') return Promise.resolve(Response.json({ emergencies: list }));
    const one = list.find((item) => String(url) === `/api/v1/security-emergencies/${item.emergency.id}`);
    return Promise.resolve(one ? Response.json(one) : Response.json({ error: 'unexpected' }, { status: 404 }));
  };
}
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
async function mount(list) {
  serve(list);
  await act(async () => { tree = Renderer.create(React.createElement(SecurityEmergencySection, { component: 'mariadb', name: 'MariaDB', versions: [] })); await settled(); await settled(); });
}
async function cleanup() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined; globalThis.fetch = originalFetch; delete globalThis.document; delete globalThis.emergencyNavigated;
}
const text = () => JSON.stringify(tree.toJSON());
const section = () => tree.root.find((node) => node.props && node.props['data-security-emergency'] === 'mariadb');
const toggle = () => section().findAll((node) => node.type === 'button')[0];
const iconsShown = () => toggle().findAll((node) => node.props && node.props['data-icon'] !== undefined).map((node) => `${node.props['data-icon']} ${node.props.className}`);

test('the tools copy exists in both languages and says nothing is wrong', () => {
  for (const key of ['emg.toolsTitle', 'emg.toolsHint']) assert.ok(enEmergencyScreens[key] && trEmergencyScreens[key], key);
  assert.equal(enEmergencyScreens['emg.toolsTitle'], 'Security emergency tools');
  assert.equal(trEmergencyScreens['emg.toolsTitle'], 'Güvenlik acil durumu araçları');
  assert.match(enEmergencyScreens['emg.toolsHint'], /^Nothing is wrong\./);
  assert.match(trEmergencyScreens['emg.toolsHint'], /^Bir sorun yok\./);
});

test('with no emergency open the closed section is a quiet tool with its hint, and sits above the row link', async () => {
  // Another component's emergency does not turn this one red.
  await mount([emergency('php')]);
  try {
    assert.match(section().props.className, /(^|\s)relative(\s|$)/, 'relative: above the row link');
    assert.equal(section().props['data-emergency-open'], 0);
    assert.doesNotMatch(section().props.className, /danger/);
    assert.ok(text().includes('emg.toolsTitle'), text());
    assert.ok(tree.root.findAll((node) => node.type === 'p').some((node) => node.children.join('') === 'emg.toolsHint{"name":"MariaDB"}'), text());
    assert.ok(!text().includes('"emg.title"'), text());
    assert.deepEqual(iconsShown().filter((icon) => icon.startsWith('Shield')), ['Shield h-4 w-4 text-fg-muted']);
    assert.ok(calls.every((call) => call.startsWith('GET ')), 'reading never sends a change');
  } finally { await cleanup(); }
});

test('an open emergency for the component keeps the alarm: red, its name, no hint', async () => {
  await mount([emergency('mariadb')]);
  try {
    assert.equal(section().props['data-emergency-open'], 1);
    assert.match(section().props.className, /border-danger/);
    assert.ok(text().includes('emg.title'), text());
    assert.ok(!text().includes('emg.toolsHint'), text());
    assert.ok(text().includes('MariaDB'), text());
    assert.deepEqual(iconsShown().filter((icon) => icon.startsWith('Shield')), ['ShieldOff h-4 w-4 text-danger']);
  } finally { await cleanup(); }
});

test('a click on the title only opens and closes the section; it navigates nowhere and sends nothing', async () => {
  await mount([]);
  try {
    assert.equal(toggle().props['aria-expanded'], false);
    await act(async () => { toggle().props.onClick(); await settled(); });
    assert.equal(toggle().props['aria-expanded'], true);
    assert.ok(text().includes('emg.intro'), text());
    await act(async () => { toggle().props.onClick(); await settled(); });
    assert.equal(toggle().props['aria-expanded'], false);
    assert.equal(globalThis.emergencyNavigated, undefined);
    assert.ok(calls.every((call) => call.startsWith('GET ')));
  } finally { await cleanup(); }
});

test('the Components row draws the section under MariaDB and PostgreSQL, inside the row, and the strip is source-checked for the alone offer', () => {
  const list = source('components/ServiceList.tsx');
  // The row link stretches over the row; every control in the row sits above it.
  assert.match(list, /after:absolute after:inset-0/);
  assert.match(list, /<SecurityEmergencySection component=\{s\.id\} name=\{componentName\(s, t\)\} versions=\{\[\]\} \/>/);
  // The alone offer is given in the installed view only; the catalogue shows the rows themselves.
  assert.match(list, /onAlone=\{hideNotInstalled \? \(id\) => setInstallTarget\(/);
  // The mail dialog is fetched lazily (the route budget, alpha.118).
  assert.match(list, /const MailSetupDialog = lazy\(\(\) => import\('\.\/MailSetupDialog'\)/);
  assert.doesNotMatch(list, /import \{ MailSetupDialog \} from/);
});
