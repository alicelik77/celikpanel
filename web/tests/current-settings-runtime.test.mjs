import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import ts from 'typescript';
import { englishCatalogue, turkishCatalogue } from './locale-catalogue.mjs';
import { replaceLazyImport, sharedLayer } from './fixtures/shared-layer.mjs';

// The rule these three screens follow (8 Oct 2026): nothing is shown as a
// setting, an empty list or an editable form until the server's current state
// is known, and no save is built from defaults. Before it, one failed read on
// a healthy server left the mail policy, a domain's backup schedule and a
// domain's scheduled tasks one click away from being replaced.

const require = createRequire(import.meta.url);
const dataModule = (text) => 'data:text/javascript;base64,' + Buffer.from(text).toString('base64');
const reactURL = pathToFileURL(require.resolve('react')).href;
const compilerOptions = { jsx: ts.JsxEmit.React, module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 };
const source = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');

const icons = ['Mail', 'Activity', 'Trash2', 'RefreshCw', 'RotateCw', 'Archive', 'Download', 'RotateCcw', 'HardDrive',
  'Database', 'Clock', 'Info', 'Plus', 'Edit2', 'Play', 'Pause', 'Save', 'X', 'AlertTriangle', 'UploadCloud'];
const stub = dataModule(`
  import React from '${reactURL}';
  ${icons.map((name) => `export const ${name} = () => null;`).join('\n')}
  export const useI18n = () => ({ t: (key) => key });
  export const useNavigate = () => () => true;
  // alpha.127: the administrator's install link; others see the sentence it carries.
  export const AdminInstallLink = (props) => props.fallback ?? null;
  export const showToast = (...args) => globalThis.currentTest.toasts.push(args);
  export const ServiceShell = (props) => React.createElement('main', null, props.children);
  // alpha.124: the passwordless mail notice on the Postfix page reads its own address; not under test here.
  export const PasswordlessMailNotice = () => null;
  // alpha.128: the outgoing mail sections on the Postfix page read their own addresses; not under test here.
  export const OutgoingMailSettings = () => null;
  export const MailPlanLimits = () => null;
  // alpha.130: the mail relay and mail program set-up read their own addresses; not under test here.
  export const MailRelaySettings = () => null;
  export const MailServerOptions = () => null;
  export const MailAutoconfigNotice = () => null;
`);
// Since 9 Oct 2026 these three screens stand on the shared remote-state layer,
// so they are mounted with the real one: lib/remote.ts for the read and the
// real Checking and CouldNotCheck of components/ui.tsx for what is drawn.
const shared = sharedLayer(stub);
const currentURL = shared.compile('components/CurrentSettings.tsx');
const load = async (name) => (await import(shared.compile(`components/${name}.tsx`, { '/CurrentSettings': currentURL })))[name];
const PostfixManagement = await load('PostfixManagement');
// The remote copies section (alpha.118) has its own test; here it is a stand-in.
const remoteSectionStub = dataModule('export const DomainRemoteBackups = () => null;');
const DomainBackupManager = (await import(replaceLazyImport(
  shared.compile('components/DomainBackupManager.tsx', { '/CurrentSettings': currentURL }), './DomainRemoteBackups', remoteSectionStub,
))).DomainBackupManager;
const DomainCronManager = await load('DomainCronManager');

const originalFetch = globalThis.fetch;
let tree, calls;
// routes: [method, url fragment, answer]; the first match answers. An answer
// is a function (called per request), 'pending' or 'offline'.
function fixture(routes) {
  calls = [];
  globalThis.currentTest = { toasts: [] };
  globalThis.confirm = () => true;
  globalThis.fetch = (url, options = {}) => {
    const method = options.method || 'GET';
    const body = options.body ? JSON.parse(options.body) : undefined;
    calls.push({ method, url, body });
    const route = routes.find(([m, fragment]) => m === method && url.includes(fragment));
    if (!route) return Promise.resolve(Response.json([]));
    if (route[2] === 'pending') return new Promise(() => {});
    if (route[2] === 'offline') return Promise.reject(new TypeError('fetch failed'));
    return Promise.resolve(route[2]({ method, url, body }));
  };
}
const refusal = (status, code, error = 'server sentence') => () => Response.json({ error, code }, { status });
// The screens start their reads and reloads without awaiting them, so every
// step also waits one turn of the event loop for those answers to land.
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
async function mount(Component, props = {}) {
  await act(async () => {
    tree = Renderer.create(React.createElement(Component, props));
    await settled();
  });
}
async function cleanup() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  delete globalThis.currentTest;
}
const text = () => JSON.stringify(tree.toJSON());
const buttons = (label) => tree.root.findAllByType('button').filter((node) => [].concat(node.props.children).includes(label));
const button = (label) => buttons(label)[0];
const press = async (node) => {
  await act(async () => {
    await node.props.onClick({});
    await settled();
  });
};
const click = (label) => press(button(label));
// alpha.132: the mail policy is saved with the shared save bar (components/kit/save).
const saveBar = () => tree.root.findAll((node) => node.type === 'button' && node.props['data-save'] !== undefined)[0];
const policySave = () => press(saveBar());
const writes = () => calls.filter((call) => call.method !== 'GET');
const scenario = (name, routes, Component, props, body) => test(name, async () => {
  fixture(routes);
  try {
    await mount(Component, props);
    await body();
  } finally {
    await cleanup();
  }
});

// --- Server mail policy ---

const stockPolicy = { message_size_mb: 9, dnsbl_zones: [], outbound_rate_limit: 0, version: 'mp1-a' };
const policyInputs = () => tree.root.findAllByType('input');

for (const [name, answer] of [
  ['still loading', 'pending'],
  ['refused by the server', refusal(502, 'CURRENT_SETTINGS_UNREADABLE')],
  ['unreachable', 'offline'],
  ['an old Agent answer without a body', () => new Response('', { status: 500 })],
]) {
  scenario(`mail policy ${name}: no form, no defaults, no Save`, [['GET', '/mail/policy', answer]], PostfixManagement, { onBack() {} }, async () => {
    assert.equal(policyInputs().length, 0, 'no policy field may be shown before the policy is known');
    assert.equal(saveBar(), undefined);
    assert.doesNotMatch(text(), /mailpolicy\.(dnsbl|rate|maxSize)"/);
    if (answer === 'pending') {
      assert.match(text(), /current\.checking/);
      assert.doesNotMatch(text(), /mailpolicy\.unreadable/);
    } else {
      assert.match(text(), /mailpolicy\.unreadable/);
      assert.equal(buttons('common.retry').length, 1);
    }
    assert.equal(writes().length, 0);
  });
}

test('mail policy: Retry reads again and only then shows the server values', async () => {
  let attempt = 0;
  fixture([['GET', '/mail/policy', () => (attempt++ === 0
    ? Response.json({ error: 'x', code: 'CURRENT_SETTINGS_UNREADABLE' }, { status: 502 })
    : Response.json({ ...stockPolicy, message_size_mb: 40, dnsbl_zones: ['zen.spamhaus.org'], outbound_rate_limit: 12 }))]]);
  try {
    await mount(PostfixManagement, { onBack() {} });
    await click('common.retry');
    assert.doesNotMatch(text(), /mailpolicy\.unreadable/);
    const values = policyInputs().map((node) => node.props.value ?? node.props.checked);
    assert.deepEqual(values, [40, 'zen.spamhaus.org', true, true, 12]);
  } finally {
    await cleanup();
  }
});

scenario('mail policy: nothing is sent before Save; a save carries the version and the loaded values it did not change',
  [['GET', '/mail/policy', () => Response.json(stockPolicy)],
    ['PUT', '/mail/policy', ({ body }) => Response.json({ success: true, policy: { ...stockPolicy, ...body, version: 'mp1-b' } })]],
  PostfixManagement, { onBack() {} }, async () => {
    assert.equal(saveBar(), undefined, 'Save is shown with nothing changed');
    await act(async () => policyInputs()[0].props.onChange({ target: { value: '12' } }));
    assert.equal(writes().length, 0, 'typing saved');
    assert.equal(tree.root.findAll((node) => typeof node.type === 'string' && node.props['data-changed-mark'] !== undefined).length, 1, 'the changed field is not marked');
    await policySave();
    assert.deepEqual(writes()[0].body, { message_size_mb: 12, dnsbl_zones: [], outbound_rate_limit: 0, version: 'mp1-a' });
    assert.equal(saveBar(), undefined, 'the saved change is still unsaved');
    await act(async () => policyInputs()[0].props.onChange({ target: { value: '14' } }));
    await policySave();
    assert.equal(writes()[1].body.version, 'mp1-b', 'the next save uses the version the server answered');
  });

scenario('mail policy: Discard gives the changes up and sends nothing',
  [['GET', '/mail/policy', () => Response.json(stockPolicy)]],
  PostfixManagement, { onBack() {} }, async () => {
    await act(async () => policyInputs()[0].props.onChange({ target: { value: '12' } }));
    await press(tree.root.findAll((node) => node.type === 'button' && node.props['data-save-discard'] !== undefined)[0]);
    assert.equal(policyInputs()[0].props.value, 9);
    assert.equal(saveBar(), undefined);
    assert.equal(writes().length, 0);
  });

for (const code of ['SETTINGS_CHANGED', 'SETTINGS_VERSION_REQUIRED']) {
  test(`mail policy: ${code} shows the reload guidance and keeps what was typed`, async () => {
    let served = stockPolicy;
    fixture([['GET', '/mail/policy', () => Response.json(served)], ['PUT', '/mail/policy', refusal(409, code)]]);
    try {
      await mount(PostfixManagement, { onBack() {} });
      await act(async () => policyInputs()[0].props.onChange({ target: { value: '50' } }));
      await policySave();
      assert.match(text(), /mailpolicy\.stale/);
      assert.equal(policyInputs()[0].props.value, 50, 'the typed size stays on screen');
      assert.equal(saveBar().props.disabled, true, 'a stale form cannot be saved again');
      assert.deepEqual(globalThis.currentTest.toasts, [], 'the guidance is on screen, not a passing toast');
      served = { ...stockPolicy, message_size_mb: 20, version: 'mp1-c' };
      await click('current.reload');
      assert.doesNotMatch(text(), /mailpolicy\.stale/);
      // alpha.132: the typed value is kept over the newer policy and the
      // server's value is said beside it, to take or to overwrite.
      assert.equal(policyInputs()[0].props.value, 50);
      assert.equal(tree.root.findAll((node) => typeof node.type === 'string' && node.props['data-server-moved'] !== undefined).length, 1);
      assert.equal(saveBar().props.disabled, false);
    } finally {
      await cleanup();
    }
  });
}

scenario('mail policy: restrictions CelikPanel will not rewrite withdraw the DNSBL controls and are never sent changed',
  [['GET', '/mail/policy', () => Response.json({ ...stockPolicy, dnsbl_zones: ['bl.example.org'], dnsbl_locked: 'no_baseline' })],
    ['PUT', '/mail/policy', ({ body }) => Response.json({ success: true, policy: { ...body, dnsbl_locked: 'no_baseline', version: 'mp1-b' } })]],
  PostfixManagement, { onBack() {} }, async () => {
    assert.match(text(), /mailpolicy\.dnsblLocked\.other/, 'the reason is on screen (the stub has no words for the reason)');
    assert.match(text(), /mailpolicy\.dnsblLockedAction/);
    assert.doesNotMatch(text(), /mailpolicy\.zones|mailpolicy\.dnsbl"/);
    await act(async () => policyInputs()[0].props.onChange({ target: { value: '12' } }));
    await policySave();
    assert.deepEqual(writes()[0].body.dnsbl_zones, ['bl.example.org']);
  });

scenario('mail policy: another refusal is the error toast and the form stays',
  [['GET', '/mail/policy', () => Response.json(stockPolicy)], ['PUT', '/mail/policy', refusal(400, 'MAIL_POLICY_INVALID', 'zone sentence')]],
  PostfixManagement, { onBack() {} }, async () => {
    await act(async () => policyInputs()[0].props.onChange({ target: { value: '12' } }));
    await policySave();
    assert.deepEqual(globalThis.currentTest.toasts, [['error', 'zone sentence']]);
    assert.doesNotMatch(text(), /mailpolicy\.stale/);
    assert.equal(policyInputs()[0].props.value, 12, 'the refused change stays');
    assert.equal(saveBar().props.disabled, false);
  });

// --- Automatic backup schedule ---

const domain = { domainId: 7, domainName: 'example.com' };
const weeklyFull = { enabled: true, frequency: 'weekly', backup_type: 'full', retention: 30, last_status: 'success', last_run: '2026-10-07T03:00:00Z', version: 'bs1-a' };
const scheduleFields = () => [...tree.root.findAllByType('select'), ...tree.root.findAllByType('input')]
  .filter((node) => node.props.id !== 'backup-database' && node.props.type !== 'checkbox');
const emptyLists = [['GET', '/backups/schedule', null], ['GET', '/backups', () => Response.json({ backups: [] })], ['GET', '/databases', () => Response.json({ databases: [] })]];
const withSchedule = (answer, ...more) => [...more, ['GET', '/backups/schedule', answer], ...emptyLists.slice(1)];

for (const [name, answer] of [
  ['still loading', 'pending'],
  ['refused by the server', refusal(500, 'INTERNAL')],
  ['unreachable', 'offline'],
]) {
  scenario(`backup schedule ${name}: no "off / daily / files / 7" form and no Save`, withSchedule(answer), DomainBackupManager, domain, async () => {
    assert.equal(scheduleFields().length, 0, 'no schedule field may be shown before the schedule is known');
    assert.equal(saveButtons().length, 0);
    assert.doesNotMatch(text(), /backup\.auto\.on"/);
    if (answer === 'pending') assert.match(text(), /current\.checking/);
    else {
      assert.match(text(), /backup\.auto\.unknown/);
      assert.equal(buttons('common.retry').length, 1);
    }
    assert.equal(writes().length, 0);
  });
}

// alpha.132: the schedule is a draft; only Save in the bar writes it.
const saveButtons = () => tree.root.findAll((node) => node.type === 'button' && node.props['data-save'] !== undefined);
const saveBar = () => saveButtons()[0];
const discard = () => tree.root.findAll((node) => node.type === 'button' && node.props['data-save-discard'] !== undefined)[0];
const switchBox = () => tree.root.findAllByType('input').find((node) => node.props.type === 'checkbox');
const change = async (node, value) => act(async () => node.props.onChange({ target: { value, checked: value } }));

scenario('backup schedule: a known schedule fills the form; nothing is written until Save, which sends the change with its version',
  withSchedule(() => Response.json(weeklyFull),
    ['PUT', '/backups/schedule', () => Response.json({ success: true, version: 'bs1-b' })],
    ['DELETE', '/backups/schedule', () => Response.json({ success: true, version: 'bs1-none' })]),
  DomainBackupManager, domain, async () => {
    assert.deepEqual(scheduleFields().map((node) => node.props.value), ['weekly', 'full', 30]);
    assert.equal(switchBox().props.checked, true);
    assert.equal(saveButtons().length, 0, 'Save is offered with nothing changed');
    await change(scheduleFields()[2], '14');
    assert.equal(writes().length, 0, 'a changed field was written before Save');
    assert.match(text(), /data-changed-mark/);
    await press(saveBar());
    assert.deepEqual(writes()[0].body, { frequency: 'weekly', backup_type: 'full', retention: 14, version: 'bs1-a' });
    assert.equal(saveButtons().length, 0, 'the bar stays after a save');
    // Turning it off is a change too, sent by Save with the version the save answered.
    await change(switchBox(), false);
    assert.equal(scheduleFields().length, 0, 'the choices of a schedule turned off are still shown');
    await press(saveBar());
    assert.match(writes()[1].url, /\/backups\/schedule\?version=bs1-b$/, 'turn off carries the version the save answered');
    assert.equal(switchBox().props.checked, false);
    assert.equal(writes().length, 2);
  });

scenario('backup schedule: Discard puts the server’s schedule back and sends nothing',
  withSchedule(() => Response.json(weeklyFull)),
  DomainBackupManager, domain, async () => {
    await change(scheduleFields()[0], 'daily');
    await change(scheduleFields()[2], '3');
    assert.match(text(), /2 changes not saved/);
    await press(discard());
    assert.deepEqual(scheduleFields().map((node) => node.props.value), ['weekly', 'full', 30]);
    assert.equal(saveButtons().length, 0);
    assert.equal(writes().length, 0);
  });

scenario('backup schedule: no schedule yet is a known state; turning it on and saving sends the first choices with its version',
  withSchedule(() => Response.json({ enabled: false, version: 'bs1-none' }),
    ['PUT', '/backups/schedule', () => Response.json({ success: true, version: 'bs1-b' })]),
  DomainBackupManager, domain, async () => {
    assert.equal(switchBox().props.checked, false);
    await change(switchBox(), true);
    await press(saveBar());
    assert.deepEqual(writes()[0].body, { frequency: 'daily', backup_type: 'files', retention: 7, version: 'bs1-none' });
  });

scenario('backup schedule: a save whose answer was lost keeps the change, holds Save and sends nothing twice',
  withSchedule(() => Response.json(weeklyFull), ['PUT', '/backups/schedule', 'offline']),
  DomainBackupManager, domain, async () => {
    await change(scheduleFields()[2], '14');
    await press(saveBar());
    assert.equal(writes().length, 1);
    assert.equal(saveBar().props.disabled, true, 'Save is offered again before the schedule was read again');
    assert.equal(scheduleFields()[2].props.value, 14);
    await press(saveBar());
    assert.equal(writes().length, 1, 'a lost save was sent a second time');
    // Checking reads the schedule again; it does not hold the change, so Save is offered again.
    await press(tree.root.findAll((node) => node.type === 'button' && node.props['data-save-check'] !== undefined)[0]);
    assert.equal(scheduleFields()[2].props.value, 14, 'the change was dropped although it was not saved');
    assert.equal(saveBar().props.disabled, false);
    assert.equal(writes().length, 1);
  });

test('backup schedule: a stale save shows the reload guidance and keeps the chosen values', async () => {
  let served = weeklyFull;
  fixture(withSchedule(() => Response.json(served), ['PUT', '/backups/schedule', refusal(409, 'SETTINGS_CHANGED')]));
  try {
    await mount(DomainBackupManager, domain);
    await change(scheduleFields()[2], '14');
    await press(saveBar());
    assert.match(text(), /backup\.auto\.stale/);
    assert.deepEqual(scheduleFields().map((node) => node.props.value), ['weekly', 'full', 14]);
    assert.equal(saveBar().props.disabled, true);
    assert.deepEqual(globalThis.currentTest.toasts, []);
    // The schedule read again: the untouched fields follow it, the changed one
    // keeps the person's value and says what the server holds now.
    served = { ...weeklyFull, frequency: 'daily', retention: 5, version: 'bs1-c' };
    await click('current.reload');
    assert.doesNotMatch(text(), /backup\.auto\.stale/);
    assert.deepEqual(scheduleFields().map((node) => node.props.value), ['daily', 'full', 14]);
    assert.match(text(), /data-server-moved/);
    assert.equal(saveBar().props.disabled, false);
  } finally {
    await cleanup();
  }
});

// --- Scheduled tasks ---

const job = { id: '0000abcd', schedule: '0 3 * * *', command: '/usr/bin/true', enabled: true, comment: '' };
const cronInputs = () => tree.root.findAllByType('input');

for (const [name, answer] of [
  ['still loading', 'pending'],
  ['unreadable on the server', refusal(502, 'CURRENT_SETTINGS_UNREADABLE')],
  ['an unclassified failure', refusal(500, 'INTERNAL')],
  ['unreachable', 'offline'],
]) {
  scenario(`scheduled tasks ${name}: no "No scheduled tasks" and no Add`, [['GET', '/cron', answer]], DomainCronManager, domain, async () => {
    assert.doesNotMatch(text(), /cron\.empty/);
    assert.equal(buttons('cron.add').length, 0);
    assert.equal(cronInputs().length, 0);
    if (answer === 'pending') assert.match(text(), /current\.checking/);
    else {
      assert.match(text(), /cron\.unknown/);
      assert.equal(buttons('common.retry').length, 1);
    }
    assert.equal(writes().length, 0);
  });
}

scenario('scheduled tasks: cron missing keeps its own guidance, not the could-not-load notice',
  [['GET', '/cron', refusal(409, 'CRON_NOT_INSTALLED', 'install cron sentence')]], DomainCronManager, domain, async () => {
    assert.match(text(), /install cron sentence/);
    assert.doesNotMatch(text(), /cron\.unknown|cron\.empty/);
    assert.equal(buttons('cron.add').length, 0);
  });

scenario('scheduled tasks: a read list with no tasks is the empty state and offers Add',
  [['GET', '/cron', () => Response.json({ jobs: [], version: 'ct1-a' })]], DomainCronManager, domain, async () => {
    assert.match(text(), /cron\.empty/);
    assert.equal(buttons('cron.add').length, 1);
  });

test('scheduled tasks: every change carries the list version; a stale one keeps the typed task', async () => {
  let version = 'ct1-a';
  let refuse = true;
  fixture([
    ['GET', '/cron', () => Response.json({ jobs: [job], version })],
    ['POST', '/cron', (request) => (refuse ? refusal(409, 'SETTINGS_CHANGED')() : Response.json({ success: true }))],
    ['PUT', '/cron', () => Response.json({ success: true })],
    ['DELETE', '/cron', () => Response.json({ success: true })],
  ]);
  try {
    await mount(DomainCronManager, domain);
    await click('cron.add');
    await act(async () => cronInputs()[1].props.onChange({ target: { value: '/usr/local/bin/report' } }));
    await click('cron.save');
    assert.deepEqual(writes()[0].body, { schedule: '0 * * * *', command: '/usr/local/bin/report', comment: '', version: 'ct1-a' });
    assert.match(text(), /cron\.stale/);
    assert.equal(cronInputs()[1].props.value, '/usr/local/bin/report', 'the typed command stays in the form');
    assert.equal(button('cron.save').props.disabled, true);
    assert.deepEqual(globalThis.currentTest.toasts, []);

    version = 'ct1-b';
    refuse = false;
    await click('cron.reload');
    assert.doesNotMatch(text(), /cron\.stale/);
    assert.equal(cronInputs()[1].props.value, '/usr/local/bin/report', 'reloading the list keeps the form');
    await click('cron.save');
    assert.equal(writes()[1].body.version, 'ct1-b');

    const iconButton = (title) => tree.root.findAllByType('button').find((node) => node.props.title === title);
    await press(iconButton('cron.disable'));
    assert.deepEqual(writes()[2].body, { ...job, enabled: false, version: 'ct1-b' });
    await press(iconButton('cron.delete'));
    assert.match(writes()[3].url, /\/cron\?id=0000abcd&version=ct1-b$/);
  } finally {
    await cleanup();
  }
});

scenario('scheduled tasks: a duplicate is refused with the server sentence and the form stays',
  [['GET', '/cron', () => Response.json({ jobs: [job], version: 'ct1-a' })],
    ['POST', '/cron', refusal(409, 'CRON_JOB_DUPLICATE', 'already exists sentence')]], DomainCronManager, domain, async () => {
    await click('cron.add');
    await act(async () => cronInputs()[1].props.onChange({ target: { value: '/usr/bin/true' } }));
    await click('cron.save');
    assert.deepEqual(globalThis.currentTest.toasts, [['error', 'already exists sentence']]);
    assert.doesNotMatch(text(), /cron\.stale/);
    assert.equal(cronInputs()[1].props.value, '/usr/bin/true');
  });

// --- Copy ---

const newKeys = [
  'current.checking', 'current.reload',
  'mailpolicy.unknown', 'mailpolicy.stale', 'mailpolicy.dnsblLockedAction',
  'mailpolicy.dnsblLocked.variable', 'mailpolicy.dnsblLocked.malformed', 'mailpolicy.dnsblLocked.no_baseline',
  'mailpolicy.dnsblLocked.terminal', 'mailpolicy.dnsblLocked.other',
  'backup.auto.unknown', 'backup.auto.stale',
  'cron.unknown', 'cron.stale', 'cron.reload',
  'err.CURRENT_SETTINGS_UNREADABLE', 'err.CRON_JOB_DUPLICATE', 'err.MAIL_POLICY_INVALID', 'err.MAIL_POLICY_INVALID.dnsbl_zone',
];
const value = (catalogue, key) => {
  const match = catalogue.match(new RegExp(`'${key.replace(/\./g, '\\.')}':\\s*'((?:[^'\\\\]|\\\\.)*)',\\n`));
  return match ? match[1] : '';
};

test('the three screens have their reading, could-not-load and stale texts in both languages', () => {
  for (const key of newKeys) {
    assert.ok(value(englishCatalogue, key), 'missing EN text ' + key);
    assert.ok(value(turkishCatalogue, key), 'missing TR text ' + key);
    assert.notEqual(value(englishCatalogue, key), value(turkishCatalogue, key), key + ' is not translated');
  }
  // A could-not-load text says nothing was changed and how to go on; a stale
  // text says nothing was saved and that what was typed is kept.
  for (const key of ['mailpolicy.unknown', 'backup.auto.unknown', 'cron.unknown']) {
    assert.match(value(englishCatalogue, key), /could not be (read|loaded).*Nothing was changed.*Try again/);
    assert.match(value(turkishCatalogue, key), /(okunamadı|yüklenemedi).*Hiçbir şey değiştirilmedi.*Tekrar deneyin/);
  }
  for (const key of ['mailpolicy.stale', 'backup.auto.stale', 'cron.stale']) {
    assert.match(value(englishCatalogue, key), /changed on the server after this (page|list) loaded, so nothing was (saved|changed)/);
    assert.match(value(turkishCatalogue, key), /yüklendikten sonra sunucuda değişti; bu yüzden hiçbir şey (kaydedilmedi|değiştirilmedi)/);
  }
  // The owner's own commands and setting names are the same in both languages.
  for (const literal of ['/etc/postfix/main.cf', 'smtpd_recipient_restrictions', 'reject_rbl_client', 'sudo systemctl reload postfix']) {
    assert.ok(value(englishCatalogue, 'mailpolicy.dnsblLockedAction').includes(literal), 'EN lacks ' + literal);
    assert.ok(value(turkishCatalogue, 'mailpolicy.dnsblLockedAction').includes(literal), 'TR lacks ' + literal);
  }
});

test('the three screens read through the shared current-state helper and never seed a form from defaults', () => {
  const postfix = source('../src/components/PostfixManagement.tsx');
  const backup = source('../src/components/DomainBackupManager.tsx');
  const cron = source('../src/components/DomainCronManager.tsx');
  for (const [name, text] of [['mail policy', postfix], ['backup schedule', backup], ['scheduled tasks', cron]]) {
    assert.match(text, /readCurrent</, name + ' must read its state through readCurrent');
    assert.match(text, /isStaleWrite\(error\)/, name + ' must tell a stale write from other refusals');
    assert.match(text, /<CurrentGate /, name + ' must gate its form or list on the known state');
  }
  // The old loaders: a failed read fell through to the defaults.
  assert.doesNotMatch(postfix, /fetch\('\/api\/v1\/mail\/policy'\)\s*\.then/);
  assert.doesNotMatch(postfix, /if \(!loaded\) return null/);
  assert.doesNotMatch(postfix, /useState\(25\)/);
  // Every write sends the version it was read at.
  assert.match(postfix, /version: policy\.version/);
  assert.match(backup, /version: schedule\.version/);
  assert.match(backup, /\?version=\$\{encodeURIComponent\(schedule\.version\)\}/);
  assert.match(cron, /\{ id: editingJob\.id, schedule, command, enabled, comment, version \}/);
  assert.match(cron, /\{ schedule, command, comment, version \}/);
  assert.match(cron, /enabled: !job\.enabled, version \}/);
  assert.match(cron, /&version=\$\{encodeURIComponent\(version\)\}/);
});
