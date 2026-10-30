import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';

// Automatic DKIM on the Mail authentication card, and a missing file's
// download in the file manager (30 Oct 2026, last mail gap before alpha.113).
//
// The card shows what the automatic DKIM setup is doing, or why it stopped,
// with who acts and the one way it resumes (D-024). Reading the card never
// starts work; "Try again" sends exactly one request. A domain with external
// DNS is told that its mail is already signed and the record goes to its DNS
// provider; a different record kept in the zone is named. The file manager
// shows a missing file's refusal in its own words instead of opening a tab
// with the server's JSON answer.

const stub = dataModule(`
  import React from '${reactURL}';
  export const useI18n = () => ({ t: (key, vars) => (vars ? key + JSON.stringify(vars) : key), locale: 'en' });
  export const useNavigate = () => () => true;
  export const showToast = (...args) => globalThis.dkimToasts.push(args);
  ${['AlertTriangle', 'ShieldCheck', 'KeyRound', 'FileCheck2', 'Copy', 'Plus', 'Info', 'RefreshCw', 'Folder', 'File',
    'ChevronRight', 'ChevronUp', 'Trash2', 'Edit', 'Download', 'Upload', 'Save', 'X', 'FolderPlus', 'FilePlus']
    .map((name) => `export const ${name} = () => null;`).join('\n')}
`);
const shared = sharedLayer(stub);
const pick = async (name) => (await import(shared.compile(`components/${name}.tsx`)))[name];
const MailAuthPanel = await pick('MailAuthPanel');
const DomainFileManager = await pick('DomainFileManager');

const originalFetch = globalThis.fetch;
globalThis.window ??= globalThis;
const record = (status, extra = {}) => ({ name: 'celik._domainkey.site.example', recommended: status === 'no_key' ? '' : 'v=DKIM1; k=rsa; p=AAA', zone_value: '', dns_value: '', resolved: true, status, ...extra });
const auth = (extra = {}) => ({
  domain: 'site.example', zone_exists: true, dns_management_mode: 'local', dkim_selector: 'celik', signing_installed: true,
  spf: record('ok'), dkim: record('pending'), dmarc: record('ok'), dkim_signed: true, ...extra,
});

let tree;
let calls;
function serve(answers) {
  calls = [];
  globalThis.dkimToasts = [];
  globalThis.fetch = async (path, options = {}) => {
    calls.push({ path: String(path), method: options.method ?? 'GET' });
    const key = `${options.method ?? 'GET'} ${path}`;
    const answer = answers[key] ?? answers[String(path)];
    if (typeof answer === 'function') return answer();
    if (answer === undefined) return new Response('{}', { status: 404 });
    return Response.json(answer);
  };
}
async function mount(element) {
  await act(async () => { tree = Renderer.create(element); });
  await act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)); });
}
async function cleanup() {
  if (tree) await act(async () => tree.unmount());
  tree = null;
  globalThis.fetch = originalFetch;
}
const text = () => JSON.stringify(tree.toJSON());
const textOf = (node) => (typeof node === 'string' ? node : (node.children ?? []).map(textOf).join(''));
const buttons = (label) => tree.root.findAll((node) => node.type === 'button' && textOf(node).includes(label));

const AUTH = '/api/v1/domains/1/mail/auth';

test('a stopped automatic setup names its reason, shows the server line and offers one retry', async () => {
  serve({ [AUTH]: auth({ dkim: record('no_key'), automatic: { state: 'failed', reason: 'signing_failed', error_line: 'opendkim restart: Job failed', trigger: 'domain_created', recorded_at: '2026-10-30T10:00:00Z' } }),
    [`POST ${AUTH}/automatic`]: () => Response.json({ state: 'pending', queued: true }, { status: 202 }) });
  try {
    await mount(React.createElement(MailAuthPanel, { domainId: 1 }));
    assert.ok(text().includes('mailauth.auto.signingFailed'), text());
    assert.ok(text().includes('opendkim restart: Job failed'));
    // The manual key button is not offered beside the automatic retry.
    assert.equal(buttons('mailauth.generateKey').length, 0);
    const retry = buttons('mailauth.auto.retry');
    assert.equal(retry.length, 1);
    const readsBefore = calls.filter((call) => call.method === 'GET').length;
    assert.equal(calls.filter((call) => call.method === 'POST').length, 0, 'reading the card started work');
    await act(async () => { await retry[0].props.onClick(); });
    const posts = calls.filter((call) => call.method === 'POST');
    assert.deepEqual(posts.map((call) => call.path), [`${AUTH}/automatic`]);
    assert.ok(calls.filter((call) => call.method === 'GET').length > readsBefore, 'the card was not read again after the retry');
    assert.ok(globalThis.dkimToasts.some(([tone, message]) => tone === 'success' && message === 'mailauth.auto.queued'));
  } finally {
    await cleanup();
  }
});

test('while the setup runs the card says nothing is to be done and offers no change', async () => {
  serve({ [AUTH]: auth({ dkim: record('no_key'), automatic: { state: 'pending', trigger: 'first_mailbox', recorded_at: '2026-10-30T10:00:00Z' } }) });
  try {
    await mount(React.createElement(MailAuthPanel, { domainId: 1 }));
    assert.ok(text().includes('mailauth.auto.pending'));
    assert.equal(buttons('mailauth.auto.retry').length, 0);
    assert.equal(buttons('mailauth.generateKey').length, 0);
  } finally {
    await cleanup();
  }
});

test('each stopped reason has its own sentence', async () => {
  for (const [automatic, key] of [
    [{ state: 'failed', reason: 'key_failed' }, 'mailauth.auto.keyFailed'],
    [{ state: 'failed', reason: 'dns_publish_failed' }, 'mailauth.auto.dnsFailed'],
    [{ state: 'blocked', reason: 'dns_not_ready' }, 'mailauth.auto.dnsNotReady'],
    [{ state: 'unknown', reason: 'signing_unknown' }, 'mailauth.auto.unknown'],
    [{ state: 'waiting', reason: 'host_busy' }, 'mailauth.auto.waiting'],
  ]) {
    serve({ [AUTH]: auth({ automatic: { ...automatic, trigger: 'retry', recorded_at: '2026-10-30T10:00:00Z' } }) });
    try {
      await mount(React.createElement(MailAuthPanel, { domainId: 1 }));
      assert.ok(text().includes(key), `${JSON.stringify(automatic)} -> ${key}`);
      assert.equal(buttons('mailauth.auto.retry').length, automatic.state === 'waiting' ? 0 : 1, key);
    } finally {
      await cleanup();
    }
  }
});

test('external DNS: signing already works and the record goes to the provider', async () => {
  serve({ [AUTH]: auth({ dns_management_mode: 'external', zone_exists: false, dkim: record('missing') }) });
  try {
    await mount(React.createElement(MailAuthPanel, { domainId: 1 }));
    assert.ok(text().includes('mailauth.externalDKIM'));
    assert.ok(text().includes('v=DKIM1; k=rsa; p=AAA'), 'the record to copy is shown');
    assert.equal(buttons('mailauth.apply').length, 0, 'external DNS offers no local publication');
  } finally {
    await cleanup();
  }
});

test('a different record kept in the zone and a key outside the signing table are named', async () => {
  serve({ [AUTH]: auth({ dkim: record('missing'), dkim_owner_record: true, dkim_signed: false }) });
  try {
    await mount(React.createElement(MailAuthPanel, { domainId: 1 }));
    assert.ok(text().includes('mailauth.ownerRecord') && text().includes('celik._domainkey.site.example'));
    assert.ok(text().includes('mailauth.notSigned'));
    assert.equal(buttons('mailauth.auto.retry').length, 1);
  } finally {
    await cleanup();
  }
});

test('downloading a file that no longer exists shows the guidance and reads the folder again', async () => {
  const FILES = '/api/v1/domains/1/files?path=%2F';
  const DOWNLOAD = '/api/v1/domains/1/files/download?path=%2Fgone.txt';
  let opened = 0;
  const previousOpen = globalThis.window.open;
  globalThis.window.open = () => { opened++; return null; };
  serve({
    [FILES]: { files: [{ name: 'gone.txt', path: '/gone.txt', is_dir: false, size: 3, permissions: '0644', mod_time: '2026-10-30T10:00:00Z' }] },
    [DOWNLOAD]: () => Response.json({ error: 'this path does not exist', code: 'FILE_NOT_FOUND' }, { status: 404 }),
  });
  try {
    await mount(React.createElement(DomainFileManager, { domainId: 1, domainName: 'site.example' }));
    const download = tree.root.findAll((node) => typeof node.type === 'string' && typeof node.props?.onClick === 'function' && node.props.title === 'files.download');
    assert.equal(download.length, 1, 'download control not found');
    const listReads = calls.filter((call) => call.path === FILES).length;
    await act(async () => { await download[0].props.onClick({ stopPropagation() {}, preventDefault() {} }); });
    await act(async () => { await new Promise((resolve) => setTimeout(resolve, 0)); });
    assert.equal(opened, 0, 'a tab was opened with the server answer');
    assert.ok(calls.some((call) => call.path === DOWNLOAD && call.method === 'GET'));
    assert.ok(globalThis.dkimToasts.some(([tone, message]) => tone === 'error' && message === 'files.notFound'), JSON.stringify(globalThis.dkimToasts));
    assert.ok(calls.filter((call) => call.path === FILES).length > listReads, 'the folder was not read again');
  } finally {
    globalThis.window.open = previousOpen;
    await cleanup();
  }
});
