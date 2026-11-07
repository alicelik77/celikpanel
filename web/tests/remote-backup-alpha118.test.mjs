// alpha.118 (W4): remote backups. The screens read the Panel's guidance and
// say it in the reader's language (D-024); the server enforces every rule.
// These tests hold the pure decisions the screens make themselves.
//
// alpha.118: uzak yedekler. Ekranlar sunucunun yönlendirmesini okurun dilinde
// söyler; kuralları sunucu uygular. Bu testler ekranların kendi kararlarını tutar.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import {
  REMOTE_GUIDANCE_CODES, decodeSiteOverview, decodeTestResult, generatePassphrase, guidanceCodeKeys, guidanceTone,
  keepCountProblem, operationActive, progressPercent, refusalResult, refusalWho, remoteEligibleBackupName, serverTime,
} from '../src/lib/remoteBackup.ts';
import { isReaskRoute } from '../src/lib/requestIdentity.ts';
import { enRemoteBackupScreens as en } from '../src/i18n/screens/remoteBackup/en.ts';
import { trRemoteBackupScreens as tr } from '../src/i18n/screens/remoteBackup/tr.ts';
import { englishCatalogue, turkishCatalogue } from './locale-catalogue.mjs';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');

test('every remote backup code the server gives has a sentence and a next step in both languages', () => {
  for (const code of REMOTE_GUIDANCE_CODES) {
    const keys = guidanceCodeKeys(code);
    for (const [name, catalogue] of [['English', en], ['Turkish', tr]]) {
      assert.equal(typeof catalogue[keys.what], 'string', `${name} has no ${keys.what}`);
      assert.equal(typeof catalogue[keys.todo], 'string', `${name} has no ${keys.todo}`);
      assert.doesNotMatch(catalogue[keys.what] + catalogue[keys.todo], /[A-Z]{3,}_[A-Z_]+/, `${name} ${code} shows an internal name`);
    }
  }
  assert.equal(guidanceCodeKeys('SOMETHING_ELSE'), null);
  // Every code the Go side can put into a guidance or a coded refusal is covered.
  const go = ['remote_backup_handlers.go', 'remote_backup_worker.go', 'remote_backup_model.go']
    .map((file) => read(`../../cmd/panel/${file}`)).join('\n');
  const found = new Set();
  for (const pattern of [
    /remote(?:Prerequisite|VerifiedFailure)\("([A-Z_]+)"/g,
    /code: "([A-Z_]+)"/g,
    /writeCodedError\(w, [^,]+, "([A-Z_]+)"/g,
    /code(?: :)?= "([A-Z_]+)"/g,
    /case "([A-Z_]+)"(?:, "[A-Z_]+")*:/g,
  ]) for (const match of go.matchAll(pattern)) found.add(match[1]);
  found.delete('UNKNOWN_OPERATION'); // an internal impossibility, never a person's to act on
  for (const code of found) assert.ok(REMOTE_GUIDANCE_CODES.includes(code), `${code} has no guidance sentence`);
});

test('the phases, results and who-acts values have words in both languages', () => {
  const keys = [
    ...['waiting', 'backup', 'upload', 'verify', 'retention', 'download', 'import', 'restore', 'done'].map((p) => `rbak.phase.${p}`),
    ...['waiting', 'in_progress', 'retrying', 'done', 'prerequisite', 'verified_failure', 'unknown'].map((r) => `rbak.result.${r}`),
    ...['none', 'panel', 'destination_owner', 'administrator', 'account_owner', 'you'].map((w) => `rbak.who.${w}`),
    ...['administrator', 'reseller', 'subscription', 'role'].map((l) => `rbak.addRefused.${l}`),
    ...['connect', 'write', 'read', 'delete'].map((s) => `rbak.test.step.${s}`),
  ];
  for (const key of keys) {
    assert.equal(typeof en[key], 'string', `English has no ${key}`);
    assert.equal(typeof tr[key], 'string', `Turkish has no ${key}`);
  }
  // The phases the worker records.
  const worker = read('../../cmd/panel/remote_backup_worker.go');
  for (const match of worker.matchAll(/(?:newRemoteProgress\(ctx, op\.ID, |progress\.phase = |phase = ')"?([a-z]+)/g)) {
    assert.equal(typeof en[`rbak.phase.${match[1]}`], 'string', `phase ${match[1]} has no words`);
  }
});

test('the three failure classes are drawn apart; progress is one tone', () => {
  const tones = ['prerequisite', 'verified_failure', 'unknown'].map(guidanceTone);
  assert.equal(new Set(tones).size, 3);
  for (const result of ['waiting', 'in_progress', 'retrying']) assert.equal(guidanceTone(result), 'progress');
  assert.equal(guidanceTone('done'), 'done');
  assert.equal(guidanceTone('nonsense'), 'unknown');
  assert.notEqual(en['rbak.result.prerequisite'], en['rbak.result.verified_failure']);
  assert.notEqual(tr['rbak.result.verified_failure'], tr['rbak.result.unknown']);
  // A store refusal names its class in `reason`; anything else is an unmet prerequisite.
  assert.equal(refusalResult('verified_failure'), 'verified_failure');
  assert.equal(refusalResult('unknown'), 'unknown');
  assert.equal(refusalResult(undefined), 'prerequisite');
  assert.equal(refusalWho('REMOTE_SETTINGS'), 'you');
  assert.equal(refusalWho('REMOTE_SCHEDULE_ADMINISTRATOR'), 'administrator');
  assert.equal(refusalWho('REMOTE_CREDENTIALS'), 'destination_owner');
});

test('only generated backup names can be sent', () => {
  for (const name of [
    'files-20261103T101500.123456789Z-0123456789abcdef.cpbak',
    'full-20261103T101500.000000001Z-fedcba9876543210.cpbak',
    'database-12-20261103T101500.123456789Z-0123456789abcdef.cpbak',
  ]) assert.equal(remoteEligibleBackupName(name), true, name);
  for (const name of [
    'example.com-files-20260101.tar.gz', 'files-20261103T101500Z-0123456789abcdef.cpbak',
    'database-0-20261103T101500.123456789Z-0123456789abcdef.cpbak', 'full-20261103T101500.123456789Z-0123456789ABCDEF.cpbak',
    '../files-20261103T101500.123456789Z-0123456789abcdef.cpbak', '',
  ]) assert.equal(remoteEligibleBackupName(name), false, name);
});

test('keep counts follow the server: 0-90 daily, 0-104 weekly, at least one copy', () => {
  assert.equal(keepCountProblem(7, 4), null);
  assert.equal(keepCountProblem(0, 1), null);
  assert.equal(keepCountProblem(90, 104), null);
  assert.equal(keepCountProblem(0, 0), 'rbak.sched.keepNone');
  assert.equal(keepCountProblem(91, 0), 'rbak.sched.keepDailyRange');
  assert.equal(keepCountProblem(-1, 3), 'rbak.sched.keepDailyRange');
  assert.equal(keepCountProblem(1.5, 3), 'rbak.sched.keepDailyRange');
  assert.equal(keepCountProblem(3, 105), 'rbak.sched.keepWeeklyRange');
  for (const key of ['rbak.sched.keepNone', 'rbak.sched.keepDailyRange', 'rbak.sched.keepWeeklyRange']) {
    assert.equal(typeof tr[key], 'string');
  }
});

test('a generated passphrase is long, easy to write down and never repeats', () => {
  const made = new Set(Array.from({ length: 100 }, () => generatePassphrase()));
  assert.equal(made.size, 100);
  for (const passphrase of made) assert.match(passphrase, /^[a-hjkm-z2-9]{5}(-[a-hjkm-z2-9]{5}){5}$/);
});

test('the test answer is read as the server sends it, the host key with Go field names', () => {
  const answer = decodeTestResult({
    ok: false, steps: [{ step: 'connect', ok: false }],
    host_key: { Key: 'ssh-ed25519 AAAA', Fingerprint: 'SHA256:abc' },
    guidance: { result: 'prerequisite', code: 'REMOTE_HOST_KEY_CONFIRM', reason: 'x', who_acts: 'destination_owner', next_action: 'y', resumes: 'z' },
  });
  assert.deepEqual(answer.hostKey, { key: 'ssh-ed25519 AAAA', fingerprint: 'SHA256:abc' });
  assert.equal(answer.guidance.code, 'REMOTE_HOST_KEY_CONFIRM');
  assert.equal(decodeTestResult({ ok: true, steps: [], host_key: { fingerprint: 'SHA256:x' } }).hostKey.fingerprint, 'SHA256:x');
  assert.equal(decodeTestResult({ ok: true, steps: null }).hostKey, null);
  assert.throws(() => decodeTestResult({ steps: [] }));
});

test('a site overview that is not the contract is unknown, never empty', () => {
  for (const bad of [null, [], {}, { destinations: [], operations: [] }, { destinations: 'x', operations: [], can_add_destination: true }]) {
    assert.throws(() => decodeSiteOverview(bad), JSON.stringify(bad));
  }
  const site = decodeSiteOverview({
    destinations: null, policies: [], can_add_destination: false, add_refused_by: 'subscription',
    operations: [{ id: 'rb-1', kind: 'upload', state: 'running', phase: 'upload', bytes_done: 50, bytes_total: 200, guidance: { result: 'in_progress', who_acts: 'none' } }],
  });
  assert.deepEqual(site.destinations, []);
  assert.equal(site.addRefusedBy, 'subscription');
  assert.equal(operationActive(site.operations[0]), true);
  assert.equal(operationActive({ state: 'failed' }), false);
  assert.equal(progressPercent(50, 200), 25);
  assert.equal(progressPercent(5, 0), null);
  assert.equal(progressPercent(300, 200), 100);
});

test('server times are UTC; an empty or zero time is no time', () => {
  assert.equal(serverTime('2026-11-03 10:00:00').toISOString(), '2026-11-03T10:00:00.000Z');
  assert.equal(serverTime(''), null);
  assert.equal(serverTime('0001-01-01T00:00:00Z'), null);
});

test('sending and restoring are identified changes, asked again once when the answer is lost', () => {
  for (const url of ['/api/v1/domains/12/backups/remote/send', '/api/v1/domains/12/backups/remote/restore']) {
    assert.equal(isReaskRoute('POST', url), true, url);
    assert.equal(isReaskRoute('GET', url), false, url);
  }
  for (const url of ['/api/v1/domains/12/backups/remote', '/api/v1/domains/12/backups/remote/policy', '/api/v1/remote-backup/destinations']) {
    assert.equal(isReaskRoute('POST', url), false, url);
  }
  // The server guards the same two routes.
  const guard = read('../../cmd/panel/request_identity.go');
  assert.match(guard, /\/api\/v1\/domains\/\{id\}\/backups\/remote\/send/);
  assert.match(guard, /\/api\/v1\/domains\/\{id\}\/backups\/remote\/restore/);
});

test('the new right follows ssh, as on the server, with its name in both languages', () => {
  // alpha.118 integration: the other alpha.118 rights (service_apps, then the
  // site tools that rights_site_tools.go appends) follow it in the same order;
  // alpha.119's uploaded programs (program_apps) come right after service_apps.
  const rights = read('../src/lib/rights.ts');
  assert.match(rights, /'file_access', 'ssh',\s*(?:\/\/[^\n]*\n\s*)*'remote_backup_destinations', 'service_apps', 'program_apps', 'python_apps', 'php_settings', 'php_version', 'nginx_directives',\s*'wordpress_tools', 'malware_scan',\s*(?:\/\/[^\n]*\n\s*|'[a-z_]+',\s*)*\] as const;/);
  const server = read('../../cmd/panel/rights_model.go');
  assert.match(server, /rightFileAccess, rightSSH,\s*rightRemoteBackupDestinations,\s*rightServiceApps,\s*rightProgramApps,\s*rightPythonApps,\s*rightPHPSettings,\s*rightPHPVersion,\s*rightNginxDirectives,\s*\}/);
  for (const catalogue of [englishCatalogue, turkishCatalogue]) {
    assert.match(catalogue, /'rights\.perm\.remote_backup_destinations':/);
    assert.match(catalogue, /'transfer\.permission\.remote_backup_destinations':/);
  }
});

test('secrets are typed into password fields and never echoed; the host key is never accepted by itself', () => {
  const editor = read('../src/components/RemoteBackupDestinations.tsx');
  for (const field of ['secret', 'password']) {
    assert.match(editor, new RegExp(`<PasswordInput id=\\{id\\('${field}'\\)\\}`), `${field} is not a password field`);
  }
  assert.match(editor, /<textarea id=\{id\('key'\)\}[\s\S]{0,300}autoComplete="off"/);
  // The fingerprint goes back only from the "matches" button.
  assert.equal([...editor.matchAll(/setConfirmedFingerprint\(fingerprint\)/g)].length, 1);
  assert.match(editor, /onClick=\{\(\) => onConfirm\(result\.hostKey!\.fingerprint\)\}/);
  // A passphrase is sent on creation only, and saving waits for "I have written it down".
  assert.match(editor, /\.\.\.\(creating && !forTest \? \{ passphrase \} : \{\}\)/);
  assert.match(editor, /!writtenDown \? t\('rbak\.form\.writtenDownNeeded'\)/);
});

test('the site section follows operations only by reading, and only while one runs', () => {
  const site = read('../src/components/DomainRemoteBackups.tsx');
  assert.match(site, /useRefreshEvery\(overview, FOLLOW_EVERY_MS, following\)/);
  assert.match(site, /const following = operations\.some\(operationActive\)/);
  const manager = read('../src/components/DomainBackupManager.tsx');
  assert.match(manager, /remoteEligibleBackupName\(backup\.name\)/);
  assert.match(manager, /const DomainRemoteBackups = lazy\(/);
});

// --- Mounted: the site section with the shipped shared layer ---------------------

const { default: React } = await import('react');
const { default: Renderer, act } = await import('react-test-renderer');
const { dataModule, reactURL, sharedLayer } = await import('./fixtures/shared-layer.mjs');

const iconNames = new Set(['AlertTriangle']);
for (const file of ['DomainRemoteBackups', 'RemoteBackupShared']) {
  for (const match of read(`../src/components/${file}.tsx`).matchAll(/import\s*\{([^}]+)\}\s*from 'lucide-react'/g)) {
    for (const item of match[1].split(',')) {
      const icon = item.replace(/\btype\b/, '').trim();
      if (icon && icon !== 'LucideIcon') iconNames.add(icon);
    }
  }
}
const stub = dataModule(`
  import React from '${reactURL}';
  ${[...iconNames].map((name) => `export const ${name} = () => null;`).join('\n')}
  // Every key "has words" here: [key] plus its values, so a fallback is never taken.
  const i18n = { t: (key, vars) => '[' + key + ']' + (vars ? JSON.stringify(vars) : ''), locale: 'en' };
  export const useI18n = () => i18n;
  export const useNavigate = () => () => true;
  export const useAuth = () => ({ role: globalThis.remoteRole ?? 'customer' });
  export const showToast = (...args) => globalThis.remoteToasts.push(args);
`);
const shared = sharedLayer(stub);
const sharedURL = shared.compile('components/RemoteBackupShared.tsx');
const { DomainRemoteBackups } = await import(shared.compile('components/DomainRemoteBackups.tsx', { '/RemoteBackupShared': sharedURL }));

const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
async function mountSite(overview, props = {}) {
  const calls = [];
  const originalFetch = globalThis.fetch;
  globalThis.remoteToasts = [];
  globalThis.fetch = (url, options = {}) => {
    calls.push({ method: options.method || 'GET', url: String(url) });
    if (String(url).endsWith('/backups/remote')) return Promise.resolve(Response.json(overview));
    return Promise.resolve(Response.json({ objects: [] }));
  };
  let tree;
  await act(async () => {
    tree = Renderer.create(React.createElement(DomainRemoteBackups, { domainId: 7, readOnly: false, sendName: null, onSendClosed() {}, ...props }));
    await settled();
  });
  const done = async () => {
    await act(async () => tree.unmount());
    globalThis.fetch = originalFetch;
  };
  return { tree, calls, text: () => JSON.stringify(tree.toJSON()), done };
}

const destination = { id: 3, name: 'Office NAS', kind: 'sftp', scope: 'own', manageable: true, has_secret: true, passphrase_check: 'AB12-CD34', offered_to_accounts: false, location: '' };

test('mounted: a running send shows its progress and phase; a failure shows its class, who acts and the next step', async () => {
  const site = await mountSite({
    destinations: [destination], policies: [], can_add_destination: true, add_refused_by: '',
    operations: [
      { id: 'rb-1', kind: 'upload', destination_id: 3, backup_name: 'files-x.cpbak', state: 'running', phase: 'upload', bytes_done: 25, bytes_total: 100,
        guidance: { result: 'in_progress', reason: 'Running: upload.', who_acts: 'none', next_action: '', resumes: '' } },
      { id: 'rb-2', kind: 'upload', destination_id: 3, state: 'failed', phase: 'upload',
        guidance: { result: 'prerequisite', code: 'REMOTE_PERMISSION', reason: 'access denied', who_acts: 'destination_owner', next_action: 'x', resumes: 'y' } },
    ],
  });
  try {
    const text = site.text();
    assert.match(text, /\[rbak\.running\.what\]\{\\"phase\\":\\"\[rbak\.phase\.upload\]\\"\}/);
    const bar = site.tree.root.findByProps({ role: 'progressbar' });
    assert.equal(bar.props['aria-valuenow'], 25);
    assert.match(text, /rbak\.result\.prerequisite/);
    assert.match(text, /rbak\.code\.REMOTE_PERMISSION\.what/);
    assert.match(text, /rbak\.code\.REMOTE_PERMISSION\.todo/);
    assert.match(text, /rbak\.who\.destination_owner/);
    assert.ok(site.calls.every((call) => call.method === 'GET'), 'reading the section changed something');
  } finally {
    await site.done();
  }
});

test('mounted: no usable destination says why and who can change it, and offers no send', async () => {
  const site = await mountSite({ destinations: [], policies: [], operations: [], can_add_destination: false, add_refused_by: 'subscription' });
  try {
    const text = site.text();
    assert.match(text, /rbak\.site\.noDestination/);
    assert.match(text, /rbak\.addRefused\.subscription/);
    assert.match(text, /rbak\.site\.askProvider/);
    assert.doesNotMatch(text, /rbak\.objects\.show/);
  } finally {
    await site.done();
  }
});

for (const match of read('../src/components/RemoteBackupDestinations.tsx').matchAll(/import\s*\{([^}]+)\}\s*from 'lucide-react'/g)) {
  for (const item of match[1].split(',')) if (item.trim()) iconNames.add(item.trim());
}
const destStub = dataModule(`
  import React from '${reactURL}';
  ${[...iconNames].map((name) => `export const ${name} = () => null;`).join('\n')}
  const i18n = { t: (key, vars) => '[' + key + ']' + (vars ? JSON.stringify(vars) : ''), locale: 'en' };
  export const useI18n = () => i18n;
  export const useNavigate = () => () => true;
  export const showToast = (...args) => globalThis.remoteToasts.push(args);
`);
const destShared = sharedLayer(destStub);
const { RemoteBackupDestinations } = await import(destShared.compile('components/RemoteBackupDestinations.tsx', {
  '/RemoteBackupShared': destShared.compile('components/RemoteBackupShared.tsx'),
}));

test('mounted: an SFTP key is sent back only after "The fingerprint matches"; no passphrase leaves with a test', async () => {
  globalThis.document ??= { addEventListener() {}, removeEventListener() {} };
  globalThis.remoteToasts = [];
  const originalFetch = globalThis.fetch;
  const posts = [];
  globalThis.fetch = (url, options = {}) => {
    if ((options.method || 'GET') === 'GET') {
      return Promise.resolve(Response.json({ destinations: [], can_add: true, add_refused_by: '', add_scope: 'own', min_passphrase: 12 }));
    }
    posts.push({ url: String(url), body: JSON.parse(options.body) });
    return Promise.resolve(Response.json({
      ok: false, steps: [{ step: 'connect', ok: false }],
      host_key: { Key: 'ssh-ed25519 AAAA', Fingerprint: 'SHA256:abc' },
      guidance: { result: 'prerequisite', code: 'REMOTE_HOST_KEY_CONFIRM', reason: 'confirm', who_acts: 'destination_owner', next_action: '', resumes: '' },
    }));
  };
  let tree;
  const act2 = (fn) => act(async () => { await fn(); await settled(); });
  const textOf = (node) => node.children.map((child) => (typeof child === 'string' ? child : textOf(child))).join('');
  const button = (label) => tree.root.findAllByType('button').find((node) => textOf(node).includes(label));
  const host = (id) => tree.root.findAll((node) => typeof node.type === 'string' && node.props.id === id)[0];
  const type = (id, value) => host(id).props.onChange({ target: { value, checked: value } });
  try {
    await act2(async () => { tree = Renderer.create(React.createElement(RemoteBackupDestinations, { admin: false })); });
    await act2(() => button('[rbak.add]').props.onClick());
    await act2(() => tree.root.findAll((node) => node.type === 'input' && node.props.value === 'sftp')[0].props.onChange());
    await act2(() => { type('rbak-form-name', 'Office'); type('rbak-form-host', 'backup.example.com'); type('rbak-form-user', 'u'); type('rbak-form-path', '/b'); type('rbak-form-password', 'secret-pw'); });
    assert.equal(host('rbak-form-password').props.type, 'password');
    assert.equal(host('rbak-form-passphrase').props.type, 'password');
    await act2(() => button('[rbak.test]').props.onClick());
    assert.equal(posts.length, 1);
    assert.equal(posts[0].body.host_key_fingerprint, undefined, 'a key was confirmed without a person');
    assert.equal(posts[0].body.passphrase, undefined, 'the passphrase left with a test');
    assert.match(JSON.stringify(tree.toJSON()), /SHA256:abc/);
    await act2(() => button('[rbak.hostKey.match]').props.onClick());
    assert.equal(posts.length, 2);
    assert.equal(posts[1].body.host_key_fingerprint, 'SHA256:abc');
  } finally {
    if (tree) await act(async () => tree.unmount());
    globalThis.fetch = originalFetch;
  }
});
