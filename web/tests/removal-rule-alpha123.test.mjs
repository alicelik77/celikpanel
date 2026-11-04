// alpha.123 (D-045): one dependency rule for every component, on screen. The
// removal window shows three groups - what must be moved or removed first
// (each with a link), what is removed together (ticked; unticked means the
// component cannot be removed) and what stops - and an install with unmet
// requirements offers "Install … first". Mounted, in English and Turkish.
// Tek bağımlılık kuralı ekranda, bağlanmış olarak.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { enRemovalRuleScreens } from '../src/i18n/screens/removalRule/en.ts';
import { trRemovalRuleScreens } from '../src/i18n/screens/removalRule/tr.ts';

const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const icons = new Set(['AlertTriangle']);
for (const path of ['components/ui.tsx', 'components/RemovalPlan.tsx']) {
  for (const match of source(path).matchAll(/import\s*\{([^}]+)\}\s*from 'lucide-react'/g)) {
    for (const item of match[1].split(',')) {
      const icon = item.replace(/\btype\b/, '').trim().split(/\s+as\s+/)[0];
      if (icon && icon !== 'LucideIcon') icons.add(icon);
    }
  }
}
globalThis.ruleWords = enRemovalRuleScreens;
const stub = dataModule(`
  import React from '${reactURL}';
  ${[...icons].map((name) => `export const ${name} = () => null;`).join('\n')}
  const fill = (text, vars) => Object.entries(vars || {}).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
  export const useI18n = () => ({ t: (key, vars) => (globalThis.ruleWords?.[key] !== undefined ? fill(globalThis.ruleWords[key], vars) : key), locale: 'en' });
  export const useNavigate = () => () => true;
  export const Link = (props) => React.createElement('a', { href: props.to }, props.children);
  export const createPortal = (node) => node;
`);
const shared = sharedLayer(stub);
const plans = await import(shared.compile('components/RemovalPlan.tsx'));
const ops = await import(shared.compile('lib/removalOperation.ts'));
const { RemovalPlanGroups, InstallNeedsDialog, decodeRemovalPlan, removalPlanRefused } = plans;

const textOf = (tree) => { const walk = (n) => (typeof n === 'string' ? n : Array.isArray(n) ? n.map(walk).join('') : (n?.children || []).map(walk).join('')); return walk(tree.toJSON()); };
const nameOf = (id) => ({ phppgadmin: 'phpPgAdmin', postgresql: 'PostgreSQL', node: 'Node.js', fail2ban: 'Fail2ban', mariadb: 'MariaDB' }[id] ?? id);

async function renderPlan(raw, props = {}, words = enRemovalRuleScreens) {
  globalThis.ruleWords = words;
  const plan = decodeRemovalPlan(raw);
  assert.ok(plan, 'the plan decodes');
  let tree;
  let kept = props.keepTogether ?? false;
  await act(async () => {
    tree = Renderer.create(React.createElement(RemovalPlanGroups, { plan, name: props.name ?? 'Nginx', nameOf, keepTogether: kept, onKeepTogether: (k) => { kept = k; }, busy: false, ...props }));
  });
  return { tree, plan, kept: () => kept, text: textOf(tree), find: (attr) => tree.root.findAll((n) => n.props?.[attr] !== undefined) };
}

const plan = (extra) => ({ blockers: [], together: [], stops: [], withdraws: [], withdraw_sets: 0, notes: [], ...extra });

test('blockers come first, each with the link to where it is moved or removed; no Remove', async () => {
  const raw = plan({ blockers: [
    { kind: 'component', component: 'node', count: 1, items: [{ name: 'Node.js', link: '/components' }] },
    { kind: 'php_sites', count: 12, items: [{ name: 'a.example (ali)', link: '/domains/a.example' }, { name: 'evil', link: 'https://elsewhere.example/' }] },
    { kind: 'apache_sites', count: 1, items: [{ name: 'old.example (veli)', link: '/domains/old.example' }] },
  ] });
  const view = await renderPlan(raw);
  assert.equal(view.find('data-rmrule-group').map((n) => n.props['data-rmrule-group']).join(','), 'first');
  assert.match(view.text, /Must be moved or removed first/);
  assert.match(view.text, /12 site\(s\) run on it\. Open each site and change its PHP version\./);
  assert.match(view.text, /and 10 more/);
  assert.match(view.text, /1 site\(s\) run on Apache/);
  const links = view.tree.root.findAll((n) => n.type === 'a').map((n) => n.props.href);
  assert.deepEqual(links, ['/components', '/domains/a.example', '/domains/old.example'], 'only links inside the panel');
  assert.equal(removalPlanRefused(view.plan), true);
  // With a way to open another removal, a component blocker is a button.
  let opened = '';
  const withButton = await renderPlan(raw, { onRemoveComponent: (id) => { opened = id; } });
  const button = withButton.tree.root.find((n) => n.props?.['data-rmrule-remove'] === 'node');
  await act(async () => button.props.onClick());
  assert.equal(opened, 'node');
});

test('what goes together is ticked; unticked, the removal is refused and says why', async () => {
  const raw = plan({ together: ['phppgadmin'], stops: [{ kind: 'database_tool', count: 0 }], withdraws: ['component:postgresql:17'], withdraw_sets: 2 });
  const view = await renderPlan(raw, { name: 'PostgreSQL' });
  const box = view.tree.root.find((n) => n.type === 'input');
  assert.equal(box.props.checked, true);
  assert.match(view.text, /phpPgAdmin Nothing else uses them, and they cannot work without PostgreSQL/);
  assert.match(view.text, /The web page for managing databases closes/);
  assert.match(view.text, /1 entr\(ies\) in 2 plan\(s\) or account\(s\) are withdrawn/);
  assert.equal(removalPlanRefused(view.plan, false), false);
  await act(async () => box.props.onChange({ target: { checked: false } }));
  assert.equal(view.kept(), true);
  assert.equal(removalPlanRefused(view.plan, true), true);
  const kept = await renderPlan(raw, { name: 'PostgreSQL', keepTogether: true });
  assert.equal(kept.find('data-rmrule-kept').length, 1);
  assert.match(kept.text, /PostgreSQL cannot be removed now/);
});

test('nothing requires it: a plain confirmation of what stops and what keeps working', async () => {
  const ftp = await renderPlan(plan({ stops: [{ kind: 'ftp', count: 4 }] }), { name: 'vsftpd' });
  assert.match(ftp.text, /4 hosting account\(s\) lose FTP\. SFTP keeps working\./);
  const none = await renderPlan(plan({}), { name: 'Netdata' });
  assert.match(none.text, /Nothing on this server requires it\./);
  const cache = await renderPlan(plan({ stops: [{ kind: 'unknown_use', count: 0 }] }), { name: 'Redis' }, trRemovalRuleScreens);
  assert.match(cache.text, /CelikPanel hangi sitelerin bunu kullandığını kaydetmez/);
  // The ClamAV schedule is a setting said in plain words.
  const clam = await renderPlan(plan({ together: ['setting:malware_schedule'], stops: [{ kind: 'malware_scans', count: 0 }] }), { name: 'ClamAV' });
  assert.match(clam.text, /the weekly scheduled malware scan \(turned off\)/);
  assert.equal(clam.find('data-removal-companions')[0].props['data-removal-companions'], '');
});

test('an unread Node.js unit list is a note, not a refusal', async () => {
  const view = await renderPlan(plan({ notes: ['runtime_units_unknown'] }), { name: 'Node.js 24.11.0' });
  assert.equal(view.find('data-rmrule-note').length, 1);
  assert.equal(removalPlanRefused(view.plan), false);
});

test('a plan that is not the contract is not shown', () => {
  assert.equal(decodeRemovalPlan({ blockers: 'x' }), null);
  assert.equal(decodeRemovalPlan(plan({ blockers: [{ kind: 'sites', count: -1, items: [] }] })), null);
  assert.equal(decodeRemovalPlan(null), null);
});

test('install: what is missing first, each with its Install button; a role lists who fills it', async () => {
  globalThis.ruleWords = enRemovalRuleScreens;
  const services = [
    { id: 'phpmyadmin', name: 'phpMyAdmin', is_installed: false },
    { id: 'mariadb', name: 'MariaDB', is_installed: false },
    { id: 'nginx', name: 'Nginx', is_installed: false, provides: ['web-server'] },
    { id: 'apache', name: 'Apache', is_installed: false, provides: ['web-server'], not_offered: true },
  ];
  let chosen = '';
  let tree;
  globalThis.document = { addEventListener() {}, removeEventListener() {}, getElementById: () => null, activeElement: null, body: { style: {} } };
  await act(async () => {
    tree = Renderer.create(React.createElement(InstallNeedsDialog, {
      service: services[0], missing: ['mariadb', 'web-server'], services,
      roleName: (token) => (token === 'web-server' ? 'web server' : null),
      onInstall: (id) => { chosen = id; }, onCancel: () => {},
    }));
  });
  const buttons = tree.root.findAll((n) => n.props?.['data-install-first'] !== undefined && n.type === 'button');
  assert.deepEqual(buttons.map((b) => b.props['data-install-first']), ['mariadb', 'nginx'], 'Apache cannot be installed here, so it is not offered');
  await act(async () => buttons[1].props.onClick());
  assert.equal(chosen, 'nginx');
  assert.match(textOf(tree), /phpMyAdmin needs other components first/);
  assert.match(textOf(tree), /Any one of these fills the role of web server/);
  await act(async () => tree.unmount());
  delete globalThis.document;
});

test('the server-side removal is followed by its id until it ends; a lost read is read again', async () => {
  const answers = [new Error('lost'), { operation: { id: 'b'.repeat(32), status: 'running', phase: 'removing:postgresql' } },
    { operation: { id: 'b'.repeat(32), status: 'succeeded', phase: 'done', result: { removed: ['phppgadmin', 'postgresql'] } } }];
  const seen = [];
  const original = globalThis.fetch;
  globalThis.fetch = async (url) => {
    seen.push(String(url));
    const next = answers.shift();
    if (next instanceof Error) throw next;
    return new Response(JSON.stringify(next), { status: 200 });
  };
  try {
    const phases = [];
    const done = await ops.followRemovalOperation({ operation: { id: 'b'.repeat(32), status: 'queued', phase: 'queued' } }, (op) => phases.push(op.phase), async () => {});
    assert.equal(done.status, 'succeeded');
    assert.deepEqual(done.removed, ['phppgadmin', 'postgresql']);
    assert.deepEqual(phases, ['queued', 'removing:postgresql', 'done']);
    assert.ok(seen.every((url) => url === `/api/v1/service/operation?id=${'b'.repeat(32)}`), 'only reads');
    const say = (key, vars) => (key === 'services.operation.phase.removing' ? `removing ${vars.name}` : key);
    assert.equal(ops.removalPhaseText(say, 'removing:postgresql', nameOf), 'removing PostgreSQL');
  } finally { globalThis.fetch = original; }
});

test('both languages carry every rule sentence with the same placeholders', () => {
  const holes = (text) => [...text.matchAll(/\{([a-z_]+)\}/g)].map((m) => m[1]).sort().join(',');
  for (const [key, text] of Object.entries(enRemovalRuleScreens)) {
    assert.ok(trRemovalRuleScreens[key], `${key} has no Turkish`);
    assert.equal(holes(trRemovalRuleScreens[key]), holes(text), key);
  }
});
