import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { enScreens, trScreens } from './locale-catalogue.mjs';
import { dataModule, reactURL, replaceLazyImport, requestProgressURL, sharedLayer } from './fixtures/shared-layer.mjs';

// What a site backup holds (docs/SITE-BACKUP-CONTENTS-DESIGN.md), on the
// Backups screen: said before a backup is made, with sizes and what is left
// out; a large backup asks first; the server's backup disk without room is a
// problem with who acts; each backup says what it holds and whether it lacks
// the program; a restore says it stops and starts the program, what is lost
// and kept, and what happened to the program afterwards. Reading what a
// backup holds never stops a backup from being made.

const icons = ['Archive', 'RefreshCw', 'Download', 'Trash2', 'RotateCcw', 'HardDrive', 'Database', 'Clock', 'Info', 'UploadCloud',
  'AlertTriangle', 'Save', 'X'];
const stub = dataModule(`
  import React from '${reactURL}';
  ${icons.map((name) => `export const ${name} = () => null;`).join('\n')}
  const i18n = { t: (key, vars) => (vars ? key + JSON.stringify(vars) : key), locale: 'en' };
  export const useI18n = () => i18n;
  export const useNavigate = () => () => true;
  export const showToast = (...args) => globalThis.currentTest.toasts.push(args);
`);
const shared = sharedLayer(stub);
const currentURL = shared.compile('components/CurrentSettings.tsx');
const remoteSectionStub = dataModule('export const DomainRemoteBackups = () => null;');
const DomainBackupManager = (await import(replaceLazyImport(
  shared.compile('components/DomainBackupManager.tsx', { '/CurrentSettings': currentURL }), './DomainRemoteBackups', remoteSectionStub,
))).DomainBackupManager;
const { backupRestoreSteps, progressSteps } = await import(requestProgressURL);

const D = '/api/v1/domains/1';
const GiB = 1024 * 1024 * 1024;
const MiB = 1024 * 1024;
const programContents = {
  site_bytes: 12 * MiB, site_files: 40, areas: [{ name: 'app', bytes: 1.1 * GiB, files: 3 }],
  left_out: [{ path: 'app/.pip-cache', reason: 'cache', bytes: 230 * MiB }], left_out_bytes: 230 * MiB,
  backup_free_bytes: 50 * GiB, complete: true, program: { kind: 'program', deployed: true },
};
const oldBackup = { name: 'files-old.cpbak', size: 4096, type: 'files', origin: 'manual', legacy: false, restorable: true, created_at: '2026-11-01T10:00:00Z', format: 2 };
const newBackup = {
  name: 'files-new.cpbak', size: 900 * MiB, type: 'files', origin: 'manual', legacy: false, restorable: true, created_at: '2026-11-12T10:00:00Z',
  format: 3, areas: [{ name: 'app', bytes: 1.1 * GiB, files: 3 }], left_out: [{ path: 'app/data/sock', reason: 'special' }],
};

let tree;
let requests;
let asked;
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
function serve(routes) {
  requests = [];
  asked = [];
  globalThis.currentTest = { toasts: [] };
  globalThis.confirm = (message, request) => { asked.push({ message, request }); return globalThis.confirmAnswer ?? true; };
  globalThis.fetch = async (input, init = {}) => {
    const path = String(input).split('?')[0];
    const method = init.method || 'GET';
    requests.push(`${method} ${path}`);
    const reply = routes[`${method} ${path}`];
    if (typeof reply === 'function') return reply(init);
    if (reply === undefined) return answer({ error: 'no route' }, 404);
    return answer(reply);
  };
}
const base = (more = {}) => ({
  [`GET ${D}/backups/contents`]: programContents,
  [`GET ${D}/backups`]: { backups: [newBackup, oldBackup] },
  [`GET ${D}/databases`]: { databases: [{ id: 3, name: 'bioindex_net_kapsora', type: 'mariadb', user: 'b', created_at: '' }] },
  [`GET ${D}/backups/schedule`]: { enabled: false, version: 'v1' },
  ...more,
});
const settle = () => act(async () => { await new Promise((resolve) => setTimeout(resolve, 20)); });
async function mount() {
  await act(async () => { tree = Renderer.create(React.createElement(DomainBackupManager, { domainId: 1, domainName: 'example.com' })); });
  await settle();
}
async function unmount() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  delete globalThis.confirmAnswer;
}
const text = () => {
  const walk = (node) => (typeof node === 'string' ? node : !node ? '' : Array.isArray(node) ? node.map(walk).join(' ') : walk(node.children));
  return walk(tree.toJSON());
};
const label = (node) => {
  const walk = (n) => (typeof n === 'string' ? n : (n.children || []).map(walk).join(''));
  return `${walk(node)} ${node.props['aria-label'] ?? ''}`.trim();
};
const buttonNamed = (name, index = 0) => tree.root.findAll((node) => node.type === 'button' && label(node).includes(name))[index];
const press = async (node) => {
  assert.ok(node, 'no such button');
  await act(async () => { await node.props.onClick({ preventDefault() {} }); });
  await settle();
};

test('what a backup of this site holds is said before one is made, with sizes and what is left out', async () => {
  serve(base());
  await mount();
  try {
    const shown = text();
    for (const phrase of ['bkc.title', 'bkc.site{"size":"12 MB"}', 'bkc.area.program{"size":"1.10 GB"}',
      'bkc.databases{"names":"bioindex_net_kapsora"}', 'bkc.leftTitle', 'bkc.left.cache{"size":"230 MB"}', 'bkc.secrets', 'bkc.large{"size":"1.11 GB"}']) {
      assert.ok(shown.includes(phrase), `${phrase} is not said: ${shown.slice(0, 600)}`);
    }
    assert.ok(!shown.includes('bkc.space.title'), 'room on the backup disk is reported missing while there is enough');
  } finally { await unmount(); }
});

test('a backup disk without room for the backup is a problem with who acts and what to do', async () => {
  serve(base({ [`GET ${D}/backups/contents`]: { ...programContents, backup_free_bytes: 1 * GiB } }));
  await mount();
  try {
    const shown = text();
    assert.ok(shown.includes('bkc.space.title') && shown.includes('bkc.space.next') && shown.includes('kit.guidance.who.administrator'), shown.slice(0, 600));
  } finally { await unmount(); }
});

test('a large files backup asks first, naming its size; declined, nothing is sent', async () => {
  serve(base());
  globalThis.confirmAnswer = false;
  await mount();
  try {
    await press(buttonNamed('backup.files'));
    assert.equal(asked.length, 1, 'a large backup was made without asking');
    assert.ok(asked[0].message.includes('bkc.confirmLarge{"size":"1.11 GB"}'), asked[0].message);
    assert.ok(!requests.includes(`POST ${D}/backups`), 'a declined backup was sent');
  } finally { await unmount(); }
});

test('a small backup is made without a question', async () => {
  serve(base({ [`GET ${D}/backups/contents`]: { ...programContents, areas: [{ name: 'app', bytes: 2 * MiB, files: 1 }] },
    [`POST ${D}/backups`]: { success: true } }));
  await mount();
  try {
    await press(buttonNamed('backup.files'));
    assert.equal(asked.length, 0, 'a small backup asked first');
    assert.ok(requests.includes(`POST ${D}/backups`));
  } finally { await unmount(); }
});

test('reading what a backup holds failed: said as such, retried by a read, and backups can still be made', async () => {
  serve(base({ [`GET ${D}/backups/contents`]: () => answer({ error: 'agent unavailable', code: 'INTERNAL' }, 502) }));
  await mount();
  try {
    assert.ok(text().includes('bkc.unknown'));
    assert.ok(!buttonNamed('backup.files').props.disabled, 'a failed read stopped a backup from being made');
    requests = [];
    await press(buttonNamed('common.retry'));
    assert.ok(requests.length > 0 && requests.every((line) => line.startsWith('GET ')), requests.join(', '));
  } finally { await unmount(); }
});

test('each backup says what it holds, what it left out, and whether it lacks the program', async () => {
  serve(base());
  await mount();
  try {
    const shown = text();
    assert.ok(shown.includes('bkc.row.holds{"list":"bkc.area.program{\\"size\\":\\"1.10 GB\\"}"}'), shown);
    assert.ok(shown.includes('bkc.row.left{"list":"bkc.left.special{\\"count\\":1}"}'), shown);
    assert.equal(shown.split('bkc.row.noProgramOld').length - 1, 1, 'the older backup is not labelled as lacking the program');
  } finally { await unmount(); }
});

test('restoring a backup with the program says it stops and starts it, what is lost and kept, and what happened', async () => {
  serve(base({ [`POST ${D}/backups/restore`]: { success: true, program: { state: 'not_started', reason: 'restart_loop' }, restored: ['app'] } }));
  await mount();
  try {
    await press(buttonNamed('backup.restore', 0));
    const { message, request } = asked[0];
    assert.ok(message.includes('bkc.restore.stopStart'), message);
    assert.deepEqual(request.lost, ['bkc.restore.programLost']);
    assert.deepEqual(request.kept, ['bkc.restore.secretsKept']);
    const shown = text();
    assert.ok(shown.includes('bkc.result.notStarted') && shown.includes('bkc.why.restart_loop') && shown.includes('bkc.next.programs'), shown.slice(0, 800));
  } finally { await unmount(); }
});

test('restoring an older backup says the program stays as it is', async () => {
  serve(base({ [`POST ${D}/backups/restore`]: { success: true } }));
  await mount();
  try {
    await press(buttonNamed('backup.restore', 1));
    const { message, request } = asked[0];
    assert.ok(!message.includes('bkc.restore.stopStart'), message);
    assert.equal(request.lost, undefined);
    assert.deepEqual(request.kept, ['bkc.restore.programKept']);
  } finally { await unmount(); }
});

test('a program started again after the restore is said by the toast; nothing stays on screen', async () => {
  serve(base({ [`POST ${D}/backups/restore`]: { success: true, program: { state: 'started' } } }));
  await mount();
  try {
    await press(buttonNamed('backup.restore', 0));
    assert.ok(globalThis.currentTest.toasts.some(([tone, words]) => tone === 'success' && words === 'bkc.result.started'), JSON.stringify(globalThis.currentTest.toasts));
    assert.ok(!text().includes('bkc.result.'), 'a started program left a block on screen');
  } finally { await unmount(); }
});

test('a failed restore that left the program stopped says so under the refusal', async () => {
  serve(base({ [`POST ${D}/backups/restore`]: () => answer({ error: 'x', code: 'BACKUP_RESTORE_ROLLBACK_FAILED', vars: { program: 'left_stopped', program_reason: 'files_unknown' } }, 500) }));
  await mount();
  try {
    await press(buttonNamed('backup.restore', 0));
    const shown = text();
    assert.ok(shown.includes('bkc.result.failedLeftStopped') && shown.includes('bkc.why.files_unknown') && shown.includes('bkc.next.startLater'), shown.slice(0, 800));
  } finally { await unmount(); }
});

// alpha.134 lab: another site's backup answered "internal error". Its refusal
// now comes with its own code; the screen keeps it above the list, with no
// program block (the program was never touched), and the list stays usable.
test('restoring another site\'s backup says so above the list, and the list stays', async () => {
  const sentence = 'This backup was made of another site, so it is not restored here; nothing was restored and the site is unchanged.';
  serve(base({ [`POST ${D}/backups/restore`]: () => answer({ error: sentence, code: 'BACKUP_RESTORE_OTHER_SITE' }, 409) }));
  await mount();
  try {
    await press(buttonNamed('backup.restore', 0));
    const shown = text();
    assert.ok(shown.includes(sentence), shown.slice(0, 800));
    assert.ok(!shown.includes('bkc.result.') && !/internal/i.test(shown), shown.slice(0, 800));
    assert.ok(buttonNamed('backup.restore', 0), 'the list is gone');
  } finally { await unmount(); }
});

test('the restore window shows stopping and starting the program only when the server plans them', () => {
  const t = (key) => key;
  const plain = progressSteps(backupRestoreSteps(t), t, undefined, undefined, 0, 'running').map((step) => step.id);
  assert.deepEqual(plain, ['check', 'inspect', 'restore', 'finish']);
  const withProgram = progressSteps(backupRestoreSteps(t), t, ['check', 'inspect', 'program_stop', 'restore', 'program_start', 'finish'], 'program_start', 0, 'running');
  assert.deepEqual(withProgram.map((step) => step.id), ['check', 'inspect', 'program_stop', 'restore', 'program_start', 'finish']);
  assert.equal(withProgram[4].state, 'running');
});

test('the new words are in English and Turkish, in plain Turkish', () => {
  const english = enScreens;
  const turkish = trScreens;
  const refusals = ['err.BACKUP_RESTORE_NOT_FOUND', 'err.BACKUP_RESTORE_OTHER_SITE', 'err.BACKUP_RESTORE_NO_SPACE', 'err.BACKUP_RESTORE_SITE_ACCOUNT'];
  const keys = Object.keys(english).filter((key) => key.startsWith('bkc.') || key.startsWith('err.BACKUP_RESTORE_PROGRAM_') || key.startsWith('opstep.restore.program') || refusals.includes(key));
  assert.ok(keys.length > 40, `only ${keys.length} keys`);
  for (const key of refusals) {
    assert.ok(keys.includes(key), `${key} is missing`);
    // D-024: nothing was changed, and what to do (or who acts) is said.
    assert.match(english[key], /nothing was restored/, key);
    assert.match(turkish[key], /hiçbir şey geri yüklenmedi/i, key);
    assert.doesNotMatch(`${english[key]} ${turkish[key]}`, /log|günlü|lstat|manifest/i, `${key} sends the reader to the server`);
  }
  // A catalogue sentence identical to the server's message reads as "the
  // server's own words" (refusalParts) and the screen would show a generic
  // sentence instead: each differs from cmd/panel's English message.
  const go = readFileSync(new URL('../../cmd/panel/backup_restore_refusals.go', import.meta.url), 'utf8');
  for (const [key, constant] of [['err.BACKUP_RESTORE_NOT_FOUND', 'errCodeRestoreNotFound'], ['err.BACKUP_RESTORE_OTHER_SITE', 'errCodeRestoreOtherSite'],
    ['err.BACKUP_RESTORE_NO_SPACE', 'errCodeRestoreNoSpace'], ['err.BACKUP_RESTORE_SITE_ACCOUNT', 'errCodeRestoreSiteAccount']]) {
    const message = go.match(new RegExp(`${constant},\\s*"([^"]+)"`))?.[1];
    assert.ok(message, `${constant} has no message`);
    assert.notEqual(english[key], message, `${key} equals the server's message`);
  }
  for (const key of keys) assert.ok(typeof turkish[key] === 'string' && turkish[key].length > 0, `${key} has no Turkish`);
  assert.equal(turkish['bkc.row.noProgramOld'], 'Bu yedek programı içermiyor (eski biçim)');
  assert.equal(turkish['bkc.area.program'], 'Program dosyaları ve verisi ({size})');
  for (const key of keys) assert.doesNotMatch(turkish[key], /runtime_app|celikrt|\.service|app\/bin|payload/, `${key} carries a code`);
});
