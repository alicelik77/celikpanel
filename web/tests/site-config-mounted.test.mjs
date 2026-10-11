import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';

// A site configuration file the owner changed is kept and named (D-031,
// 2026-10-10). The domain's Configuration file page is mounted with the real
// shared layer (remote state, lost answers, the error banner) and a fetch that
// answers as the Panel does. It must:
//   - say "checking" and "could not check" as such, with no choice enabled;
//   - name each state the classifier reports, with the owner's choices only
//     where they apply;
//   - bind "keep mine" and "take CelikPanel's" to the digests it was shown,
//     ask once before replacing, and send each choice once;
//   - when a choice's answer is lost, send nothing again and read the state.

const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null;
  export const FileCode2 = () => null;
  export const FilePen = () => null;
  export const FolderPlus = () => null;
  export const Info = () => null;
  export const RefreshCw = () => null;
  // A refusal's sentence is "translated" (marked) so the error banner shows
  // the catalogue's words for its code, as in the application.
  const i18n = { t: (key, vars) => (key.startsWith('err.') ? 'T:' + key : vars ? key + JSON.stringify(vars) : key), locale: 'en' };
  export const useI18n = () => i18n;
  export const useNavigate = () => () => true;
  export const showToast = () => {};
`);
const shared = sharedLayer(stub);
const { DomainSiteConfig, SiteConfigNotice, decodeSiteConfig } = await import(shared.compile('components/DomainSiteConfig.tsx'));
globalThis.window ??= globalThis;

const URL_BASE = '/api/v1/domains/4/site-config';
const FILE = 'c'.repeat(64);
const RENDER = 'b'.repeat(64);
const view = (state, more = {}) => ({
  domain_id: 4, domain: 'example.com', kind: 'nginx_vhost',
  path: '/etc/nginx/sites-available/example.com.conf', state,
  include_dir: '/etc/nginx/celikpanel-sites.d/example.com',
  file_sha256: FILE, render_sha256: RENDER,
  actions: ['owner_edited', 'foreign', 'unknown_origin'].includes(state) ? ['keep', 'take', 'merge'] : state === 'absent' ? ['recreate']
    : ['unreadable', 'unwritable'].includes(state) ? ['recheck'] : [],
  ...more,
});
const edited = view('owner_edited', {
  pending_path: '/etc/nginx/sites-available/example.com.conf.celikpanel-pending',
  diff: '--- a\n+++ b\n@@ -1,2 +1,2 @@\n-    index owner.html;\n+    index index.php;\n',
});

const originalFetch = globalThis.fetch;
let tree;
let requests;
let withheld;
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
const HANG = () => new Promise((resolve) => { withheld.push(resolve); });
const DROP = () => { throw new TypeError('network'); };
function serve(routes) {
  requests = [];
  globalThis.fetch = async (input, init = {}) => {
    const path = String(input).split('?')[0];
    const method = init.method || 'GET';
    requests.push({ method, path, body: init.body ? JSON.parse(init.body) : undefined });
    const reply = routes[`${method} ${path}`];
    if (typeof reply === 'function') return reply(init);
    if (reply === undefined) return answer({ error: 'no route' }, 404);
    return answer(reply);
  };
}
async function settle() { await act(async () => { await new Promise((resolve) => setTimeout(resolve, 15)); }); }
async function mount(element) {
  withheld = [];
  await act(async () => { tree = Renderer.create(element); });
  await settle();
}
async function unmount() {
  for (const resolve of withheld) resolve(answer({}, 500));
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
}
const text = () => {
  const walk = (node) => (typeof node === 'string' ? node : !node ? '' : Array.isArray(node) ? node.map(walk).join(' ') : walk(node.children));
  return walk(tree.toJSON());
};
const buttons = () => tree.root.findAll((node) => node.type === 'button');
const label = (node) => {
  const walk = (n) => (typeof n === 'string' ? n : (n.children || []).map(walk).join(''));
  return walk(node).trim();
};
const enabled = (name) => buttons().some((node) => !node.props.disabled && label(node).includes(name));
const press = async (name) => {
  const node = buttons().find((item) => label(item) === name || label(item).endsWith(name));
  assert.ok(node, `no button ${name}: ${buttons().map(label).join(' | ')}`);
  assert.ok(!node.props.disabled, `${name} is disabled`);
  await act(async () => { await node.props.onClick({ preventDefault() {} }); });
  await settle();
};
const posts = () => requests.filter((request) => request.method === 'POST');
const page = () => React.createElement(DomainSiteConfig, { domainId: 4, domainName: 'example.com' });
const CHOICES = ['siteConfig.take', 'siteConfig.keep', 'siteConfig.recreate'];

test('while the state is read: checking, and no choice is offered', async () => {
  serve({ [`GET ${URL_BASE}`]: HANG });
  await mount(page());
  assert.ok(text().includes('siteConfig.checking'));
  for (const name of CHOICES) assert.equal(enabled(name), false, name);
  await unmount();
});

for (const [how, reply] of [
  ['dropped', DROP],
  ['refused', () => answer({ error: 'x', code: 'SITE_CONFIG_NOT_READ' }, 502)],
  ['not the contract', () => answer({ state: 'edited' })],
]) {
  test(`a read that is ${how}: could not check, never a state, no choice`, async () => {
    serve({ [`GET ${URL_BASE}`]: reply });
    await mount(page());
    assert.ok(text().includes('siteConfig.unknownRead'), text());
    for (const key of ['siteConfig.ownerEdited.title', 'siteConfig.missing.title', 'siteConfig.managed.title']) {
      assert.ok(!text().includes(key), key);
    }
    for (const name of CHOICES) assert.equal(enabled(name), false, name);
    await unmount();
  });
}

test('each state the classifier reports is named, with its choices only where they apply', async () => {
  const cases = [
    [view('managed_unchanged'), 'siteConfig.managed.title', []],
    [view('managed_unchanged', { adopted_from: 'v0.1.0-alpha.82' }), 'siteConfig.adopted.start{"release":"v0.1.0-alpha.82"}', []],
    // The creation-time text: the suffix is not shown, the sentence says so.
    [view('managed_unchanged', { adopted_from: 'v0.1.0-alpha.81 (creation)' }), 'siteConfig.adopted.creation{"release":"v0.1.0-alpha.81"}', []],
    [edited, 'siteConfig.ownerEdited.title', ['siteConfig.take', 'siteConfig.keep', 'siteConfig.merge']],
    [view('foreign', { diff: '--- a\n+++ b\n' }), 'siteConfig.foreign.title', ['siteConfig.take', 'siteConfig.keep']],
    [view('unknown_origin', { diff: '--- a\n+++ b\n' }), 'siteConfig.unknownOrigin.body', ['siteConfig.take', 'siteConfig.keep']],
    [view('absent', { file_sha256: undefined }), 'siteConfig.missing.body', ['siteConfig.recreate']],
    [view('unreadable', { reason: 'symlink' }), 'siteConfig.unreadable.symlink', []],
    [view('unreadable', { reason: 'write_refused' }), 'siteConfig.unreadable.write_refused', []],
    [view('unknown', { reason: 'agent_does_not_report' }), 'siteConfig.unknownState.body', []],
  ];
  for (const [answerBody, sentence, choices] of cases) {
    serve({ [`GET ${URL_BASE}`]: answerBody });
    await mount(page());
    assert.ok(text().includes(sentence), `${answerBody.state}: ${sentence}\n${text()}`);
    for (const name of CHOICES) assert.equal(enabled(name), choices.includes(name), `${answerBody.state}: ${name}`);
    if (answerBody.state === 'managed_unchanged') {
      assert.ok(text().includes('siteConfig.include{"dir":"/etc/nginx/celikpanel-sites.d/example.com"}'));
    }
    await unmount();
  }
});

test('the difference is shown as the server computed it', async () => {
  serve({ [`GET ${URL_BASE}`]: edited });
  await mount(page());
  const diff = tree.root.findAll((node) => node.props?.['data-site-config-diff'] !== undefined);
  assert.equal(diff.length, 1);
  assert.ok(text().includes('-    index owner.html;') && text().includes('+    index index.php;'));
  await unmount();
});

test('keep mine sends the file digest it was shown, once, and says what was recorded', async () => {
  let reads = 0;
  serve({
    [`GET ${URL_BASE}`]: () => {
      reads += 1;
      return answer(reads === 1 ? edited : { ...edited, decision: { kind: 'keep_mine', decided_at: '2026-10-10T12:00:00Z', current: true } });
    },
    [`POST ${URL_BASE}/keep`]: () => answer({ ...edited, decision: { kind: 'keep_mine', decided_at: '2026-10-10T12:00:00Z', current: true } }),
  });
  await mount(page());
  await press('siteConfig.keep');
  assert.equal(posts().length, 1);
  assert.deepEqual(posts()[0].body, { file_sha256: FILE, render_sha256: RENDER });
  assert.ok(text().includes('siteConfig.done.keep'));
  assert.ok(text().includes('siteConfig.keptByChoice'), 'the kept-by-choice state is not shown after the re-read');
  assert.equal(enabled('siteConfig.keep'), false, 'keep stays offered for a file already kept');
  assert.equal(enabled('siteConfig.take'), true);
  await unmount();
});

test('take CelikPanel’s asks once in place, then sends both digests once', async () => {
  serve({
    [`GET ${URL_BASE}`]: edited,
    [`POST ${URL_BASE}/take`]: () => answer({ ...view('managed_unchanged'), outcome: 'taken', backup_path: '/etc/nginx/sites-available/example.com.conf.celikpanel-backup-20261010T120000Z' }),
  });
  await mount(page());
  await press('siteConfig.take');
  assert.equal(posts().length, 0, 'the first press replaced the file');
  assert.ok(text().includes('siteConfig.take.confirmText{"path":"/etc/nginx/sites-available/example.com.conf"}'));
  await press('siteConfig.take.confirm');
  assert.equal(posts().length, 1);
  assert.equal(posts()[0].path, `${URL_BASE}/take`);
  assert.deepEqual(posts()[0].body, { file_sha256: FILE, render_sha256: RENDER });
  assert.ok(text().includes('siteConfig.done.take{"backup":"/etc/nginx/sites-available/example.com.conf.celikpanel-backup-20261010T120000Z"}'));
  await unmount();
});

test('a choice refused because the file changed says so and reads the file again', async () => {
  serve({
    [`GET ${URL_BASE}`]: edited,
    [`POST ${URL_BASE}/keep`]: () => answer({ error: 'changed', code: 'SITE_CONFIG_CHANGED' }, 409),
  });
  await mount(page());
  const before = requests.filter((request) => request.method === 'GET').length;
  await press('siteConfig.keep');
  assert.ok(text().includes('T:err.SITE_CONFIG_CHANGED'), text());
  assert.ok(requests.filter((request) => request.method === 'GET').length > before, 'the file was not read again');
  await unmount();
});

test('a choice whose answer is lost is not sent again; the state is read and named', async () => {
  serve({ [`GET ${URL_BASE}`]: view('absent', { file_sha256: undefined }), [`POST ${URL_BASE}/recreate`]: DROP });
  await mount(page());
  await press('siteConfig.recreate');
  assert.equal(posts().length, 1, 'the change was sent a second time');
  assert.ok(tree.root.findAll((node) => node.props?.['data-result-unknown'] !== undefined).length === 1, 'no result-unknown notice');
  assert.ok(requests.filter((request) => request.method === 'GET').length >= 2, 'the state was not read again');
  await unmount();
});

test('merge by hand does nothing on the server and names both files', async () => {
  serve({ [`GET ${URL_BASE}`]: edited });
  await mount(page());
  await press('siteConfig.merge');
  assert.equal(posts().length, 0);
  assert.ok(text().includes('siteConfig.merge.hint{"path":"/etc/nginx/sites-available/example.com.conf","pending":"/etc/nginx/sites-available/example.com.conf.celikpanel-pending"}'));
  await unmount();
});

test('the notice above a domain names a kept, missing or unreadable file and nothing else', async () => {
  for (const [state, key] of [
    ['owner_edited', 'siteConfig.notice.kept'], ['foreign', 'siteConfig.notice.kept'], ['unknown_origin', 'siteConfig.notice.kept'],
    ['missing', 'siteConfig.notice.missing'], ['unreadable', 'siteConfig.notice.unreadable'],
  ]) {
    let opened = 0;
    await mount(React.createElement(SiteConfigNotice, { state, onOpen: () => { opened += 1; } }));
    assert.ok(text().includes(key), state);
    await press('siteConfig.notice.open');
    assert.equal(opened, 1);
    await act(async () => tree.unmount());
  }
  for (const state of ['managed_unchanged', 'unknown']) {
    await mount(React.createElement(SiteConfigNotice, { state, onOpen() {} }));
    assert.equal(tree.toJSON(), null, state);
    await act(async () => tree.unmount());
  }
  tree = undefined;
});

// D-031 step 1b: the certificate part of a kept file.
const managed = { managed_dir: '/etc/nginx/celikpanel-managed.d/example.com', managed_include: 'include /etc/nginx/celikpanel-managed.d/example.com/*.conf;' };
const certificateReady = {
  ...edited, ...managed, validation: 'ready', pending_reason: 'certificate',
  decision: { kind: 'keep_mine', decided_at: '2026-10-10T12:00:00Z', current: true },
  certificate: { cert_path: '/certs/new/fullchain.pem', key_path: '/certs/new/privkey.pem', served_expires_at: '2026-10-22T00:00:00Z', served_days_left: 11, referenced: false },
};

test('a new certificate the kept file does not use is named, with its lines and the days left on the one in use', async () => {
  serve({ [`GET ${URL_BASE}`]: certificateReady });
  await mount(page());
  assert.ok(text().includes('siteConfig.certificate.ready.title'), text());
  assert.ok(text().includes('siteConfig.certificate.ready.body'));
  assert.ok(text().includes('ssl_certificate /certs/new/fullchain.pem;') && text().includes('ssl_certificate_key /certs/new/privkey.pem;'));
  assert.ok(text().includes('siteConfig.certificate.servedDays'));
  assert.ok(text().includes('"days":11'));
  // The owner's choices stay: take, or keep (once the file changed).
  assert.equal(enabled('siteConfig.take'), true);
  await unmount();
  serve({ [`GET ${URL_BASE}`]: { ...certificateReady, decision: undefined, certificate: { ...certificateReady.certificate, referenced: true } } });
  await mount(page());
  assert.ok(text().includes('siteConfig.certificate.ready.referenced'), text());
  assert.ok(!text().includes('ssl_certificate /certs/new/fullchain.pem;'));
  assert.equal(enabled('siteConfig.keep'), true);
  await unmount();
});

test('a certificate the kept file does not let validate names the line to add and the schedule', async () => {
  for (const validation of ['include_missing', 'names_missing', 'challenge_kept', 'challenge_failed', 'ready']) {
    serve({ [`GET ${URL_BASE}`]: {
      ...view('unknown_origin', { diff: '--- a\n+++ b\n' }), ...managed, validation, pending_reason: 'certificate_validation',
      certificate: { served_expires_at: '2026-10-19T00:00:00Z', served_days_left: 8, referenced: false },
    } });
    await mount(page());
    assert.ok(text().includes('siteConfig.certificate.validation.title'), text());
    assert.ok(text().includes(`siteConfig.certificate.validation.${validation}`), validation);
    assert.equal(text().includes('include /etc/nginx/celikpanel-managed.d/example.com/*.conf;'), validation === 'include_missing', validation);
    assert.ok(text().includes('siteConfig.certificate.schedule'));
    await unmount();
  }
});

// Second round of step 1b: the readiness is measured by a probe. Its answer is
// named (first name, HTTP status); an unanswered probe is unknown, not "not
// ready"; a read whose probe found the file ready ended the wait and says the
// certificate can be requested again, and asks the domain page to read the list.
test('the probe’s answer is named; unknown is not "not ready"; a read that ended the wait says so once', async () => {
  const waiting = {
    ...view('unknown_origin', { diff: '--- a\n+++ b\n' }), ...managed, pending_reason: 'certificate_validation',
    certificate: { served_expires_at: '2026-10-19T00:00:00Z', served_days_left: 8, referenced: false },
  };
  serve({ [`GET ${URL_BASE}`]: { ...waiting, validation: 'include_missing', validation_name: 'www.example.com', validation_status: 301 } });
  await mount(page());
  assert.ok(text().includes('siteConfig.certificate.validation.probe') && text().includes('"name":"www.example.com"') && text().includes('"status":301'), text());
  await unmount();
  serve({ [`GET ${URL_BASE}`]: { ...waiting, validation: 'unknown' } });
  await mount(page());
  assert.ok(text().includes('siteConfig.certificate.validation.unknownTitle') && text().includes('siteConfig.certificate.validation.unknown{'), text());
  assert.ok(!text().includes('siteConfig.certificate.validation.title'), 'not "cannot be validated"');
  assert.ok(!text().includes('siteConfig.certificate.validation.include_missing') && !text().includes('siteConfig.certificate.validation.probe'));
  await unmount();
  let ended = 0;
  serve({ [`GET ${URL_BASE}`]: { ...view('unknown_origin', { diff: '--- a\n+++ b\n' }), ...managed, validation: 'ready', resolved_reason: 'certificate_validation' } });
  await mount(React.createElement(DomainSiteConfig, { domainId: 4, domainName: 'example.com', onReasonEnded: () => { ended += 1; } }));
  assert.ok(text().includes('siteConfig.certificate.validation.readyTitle') && text().includes('siteConfig.certificate.validation.ready'), text());
  assert.ok(!text().includes('siteConfig.certificate.validation.title') && !text().includes('siteConfig.certificate.schedule'), text());
  assert.equal(ended, 1, 'the domains list is read again once');
  await unmount();
});

test('an unreadable file names who acts and the next step for its reason', async () => {
  for (const [reason, next, detail] of [
    ['symlink', 'siteConfig.unreadable.next.symlink', false],
    ['permission', 'siteConfig.unreadable.next.permission', false],
    ['write_refused', 'siteConfig.unreadable.next.other', true],
  ]) {
    serve({ [`GET ${URL_BASE}`]: view('unreadable', { reason, detail: 'rename: operation not permitted' }) });
    await mount(page());
    assert.ok(text().includes(next), reason);
    assert.equal(text().includes('siteConfig.unreadable.next.detail'), detail, reason);
    assert.ok(text().includes('siteConfig.unreadable.meanwhile'));
    await unmount();
  }
});

// set10 scenario g: a locked file. The Panel answers "unwritable" with the
// system error and the protection it measured; the page names them, who
// decides and the next step with the command for that protection, shows what
// CelikPanel wanted to write, and offers "check again", which only reads.
test('a locked file is named unwritable, with the system error, the next step, the difference and check again', async () => {
  const locked = view('unwritable', {
    reason: 'write_refused', system_error: 'EPERM', protection: 'immutable',
    diff: '--- a\n+++ b\n@@ -1 +1 @@\n-old\n+new\n',
  });
  let reads = 0;
  serve({ [`GET ${URL_BASE}`]: () => { reads += 1; return answer(locked); } });
  await mount(page());
  assert.ok(text().includes('siteConfig.unwritable.title'), text());
  assert.ok(text().includes('siteConfig.unwritable.body{"error":"EPERM"}'), text());
  assert.ok(text().includes('siteConfig.unwritable.protection.immutable'));
  assert.ok(text().includes('siteConfig.unwritable.next.flag{"command":"sudo chattr -i /etc/nginx/sites-available/example.com.conf"}'), text());
  assert.ok(text().includes('siteConfig.unwritable.resume') && text().includes('siteConfig.unwritable.diff'));
  assert.ok(text().includes('+new') && !text().includes('siteConfig.unreadable.title'));
  for (const name of CHOICES) assert.equal(enabled(name), false, name);
  await press('siteConfig.recheck');
  assert.equal(posts().length, 0, 'check again changed something');
  assert.equal(reads, 2, 'check again did not read the state again');
  await unmount();
  for (const [more, next] of [
    [{ protection: 'append_only' }, 'siteConfig.unwritable.next.flag{"command":"sudo chattr -a /etc/nginx/sites-available/example.com.conf"}'],
    [{ protection: 'directory_immutable' }, 'siteConfig.unwritable.next.flag{"command":"sudo chattr -i /etc/nginx/sites-available"}'],
    [{ protection: 'read_only_filesystem', system_error: 'EROFS' }, 'siteConfig.unwritable.next.readOnly'],
    [{ protection: 'none', system_error: undefined }, 'siteConfig.unwritable.next.none'],
    [{ protection: undefined, system_error: undefined }, 'siteConfig.unwritable.next.unknown'],
  ]) {
    serve({ [`GET ${URL_BASE}`]: { ...locked, ...more } });
    await mount(page());
    assert.ok(text().includes(next), `${JSON.stringify(more)}\n${text()}`);
    if ('system_error' in more && !more.system_error) assert.ok(text().includes('siteConfig.unwritable.bodyNoError'));
    await unmount();
  }
});

// The protection was removed and "check again" measured none on the same
// bytes: CelikPanel's own text whose last change is held. The page says it is
// written at the next render and offers to write it now, once, bound to the
// digests it showed.
test('a removed protection offers to write CelikPanel’s text now, once, and says it was written', async () => {
  const held = view('managed_unchanged', {
    reason: 'protection_removed', protection: 'none', actions: ['apply'],
    diff: '--- a\n+++ b\n@@ -1 +1 @@\n-old\n+new\n',
  });
  let reads = 0;
  serve({
    [`GET ${URL_BASE}`]: () => { reads += 1; return answer(reads === 1 ? held : view('managed_unchanged')); },
    [`POST ${URL_BASE}/apply`]: () => answer({ ...view('managed_unchanged'), outcome: 'written' }),
  });
  await mount(page());
  assert.ok(text().includes('siteConfig.held.title') && text().includes('siteConfig.held.body'), text());
  assert.ok(text().includes('siteConfig.held.auto') && text().includes('siteConfig.apply.hint'));
  assert.ok(text().includes('siteConfig.held.diff') && text().includes('+new'));
  assert.ok(!text().includes('siteConfig.unwritable.title'), 'still named unwritable');
  for (const name of CHOICES) assert.equal(enabled(name), false, name);
  await press('siteConfig.apply');
  assert.equal(posts().length, 1);
  assert.equal(posts()[0].path, `${URL_BASE}/apply`);
  assert.deepEqual(posts()[0].body, { file_sha256: FILE, render_sha256: RENDER });
  assert.ok(text().includes('siteConfig.done.apply'), text());
  assert.ok(text().includes('siteConfig.managed.title') && !enabled('siteConfig.apply'));
  await unmount();
});

test('writing the held text refused because the file is protected again names the refusal and reads again', async () => {
  const held = view('managed_unchanged', { reason: 'protection_removed', protection: 'none', actions: ['apply'] });
  serve({
    [`GET ${URL_BASE}`]: held,
    [`POST ${URL_BASE}/apply`]: () => answer({ error: 'x', code: 'SITE_CONFIG_UNWRITABLE', reason: 'write_refused' }, 409),
  });
  await mount(page());
  await press('siteConfig.apply');
  assert.equal(posts().length, 1);
  assert.ok(text().includes('err.SITE_CONFIG_UNWRITABLE'), text());
  assert.ok(!text().includes('siteConfig.done.apply'));
  await unmount();
});

// A refused "take" on the owner's edited file: the page, the row and the
// badge say unwritable (one precedence); the page names both facts, the next
// step for the protection, check again, and the owner's three choices.
test('an edited file whose take was refused names both facts and keeps the choices', async () => {
  const both = view('unwritable', {
    file_state: 'owner_edited', reason: 'write_refused', system_error: 'EPERM', protection: 'immutable',
    actions: ['keep', 'take', 'merge', 'recheck'],
    pending_path: '/etc/nginx/sites-available/example.com.conf.celikpanel-pending',
    diff: '--- a\n+++ b\n@@ -1 +1 @@\n-    index owner.html;\n+    index index.php;\n',
  });
  serve({
    [`GET ${URL_BASE}`]: both,
    [`POST ${URL_BASE}/take`]: () => answer({ error: 'x', code: 'SITE_CONFIG_UNWRITABLE', reason: 'write_refused' }, 409),
  });
  await mount(page());
  assert.ok(text().includes('siteConfig.editedUnwritable.title'), text());
  assert.ok(text().includes('siteConfig.editedUnwritable.body'));
  assert.ok(text().includes('siteConfig.editedUnwritable.refused{"error":"EPERM"}'), text());
  assert.ok(text().includes('siteConfig.unwritable.protection.immutable'));
  assert.ok(text().includes('siteConfig.editedUnwritable.next.flag{"command":"sudo chattr -i /etc/nginx/sites-available/example.com.conf"}'), text());
  assert.ok(text().includes('siteConfig.unwritable.resume') && text().includes('siteConfig.diff.title'));
  assert.ok(!text().includes('siteConfig.unwritable.title') && !text().includes('siteConfig.unwritable.diff'));
  assert.equal(enabled('siteConfig.take'), true);
  assert.equal(enabled('siteConfig.keep'), true);
  assert.equal(enabled('siteConfig.recheck'), true);
  await press('siteConfig.take');
  await press('siteConfig.take.confirm');
  assert.equal(posts().length, 1);
  assert.ok(text().includes('err.SITE_CONFIG_UNWRITABLE'), text());
  await unmount();
});

test('the notice above a domain names a file that cannot be replaced', async () => {
  await mount(React.createElement(SiteConfigNotice, { state: 'unwritable', onOpen() {} }));
  assert.ok(text().includes('siteConfig.notice.unwritable'));
  assert.equal(tree.toJSON().props['data-site-config-notice'], 'unwritable');
  await act(async () => tree.unmount());
  tree = undefined;
});

test('the two captions of the difference are drawn in the page language', async () => {
  serve({ [`GET ${URL_BASE}`]: { ...edited, diff: "--- /etc/nginx/sites-available/example.com.conf (on this server)\n+++ CelikPanel's text\n@@ -1 +1 @@\n-a\n+b\n" } });
  await mount(page());
  assert.ok(text().includes('--- /etc/nginx/sites-available/example.com.conf siteConfig.diff.here'), text());
  assert.ok(text().includes('+++ siteConfig.diff.panel'));
  assert.ok(!text().includes('(on this server)') && !text().includes("CelikPanel's text"));
  await unmount();
});

test('the notice above a domain honours keep mine, and a waiting certificate outranks it', async () => {
  for (const [props, key, kind] of [
    [{ state: 'owner_edited', keptByChoice: true }, 'siteConfig.notice.keptByChoice', 'kept_by_choice'],
    [{ state: 'owner_edited', keptByChoice: true, pendingReason: 'certificate' }, 'siteConfig.notice.certificate', 'certificate'],
    [{ state: 'unknown_origin', pendingReason: 'certificate_validation' }, 'siteConfig.notice.certificateValidation', 'certificate_validation'],
  ]) {
    await mount(React.createElement(SiteConfigNotice, { ...props, onOpen() {} }));
    assert.ok(text().includes(key), key);
    assert.ok(!text().includes('siteConfig.notice.kept '), key);
    assert.equal(tree.toJSON().props['data-site-config-notice-kind'], kind);
    await act(async () => tree.unmount());
  }
  tree = undefined;
});

test('an answer that is not the contract is refused by the decoder', () => {
  assert.throws(() => decodeSiteConfig({ state: 'edited', actions: [] }));
  assert.throws(() => decodeSiteConfig({ state: 'owner_edited' }));
  assert.equal(decodeSiteConfig(edited).state, 'owner_edited');
});

// Every key the page and the refusals use exists in English and in Turkish.
test('every siteConfig and SITE_CONFIG key is in both languages', () => {
  const read = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
  const keys = (source, prefix) => new Set([...source.matchAll(new RegExp(`'(${prefix}[^']*)':`, 'g'))].map((match) => match[1]));
  const screenEN = keys(read('i18n/screens/server/en.ts'), 'siteConfig\\.');
  const screenTR = keys(read('i18n/screens/server/tr.ts'), 'siteConfig\\.');
  const errEN = keys(read('i18n/en.ts'), 'err\\.SITE_CONFIG_');
  const errTR = keys(read('i18n/tr.ts'), 'err\\.SITE_CONFIG_');
  assert.deepEqual([...screenTR].sort(), [...screenEN].sort());
  assert.deepEqual([...errTR].sort(), [...errEN].sort());
  assert.equal(errEN.size, 9);
  const component = read('components/DomainSiteConfig.tsx') + read('components/Domains.tsx') + read('components/DomainDetail.tsx');
  for (const match of component.matchAll(/'(siteConfig\.[a-zA-Z_.]+)'/g)) assert.ok(screenEN.has(match[1]), match[1]);
  for (const reason of ['symlink', 'not_regular', 'permission', 'too_large', 'read_failed', 'write_refused']) {
    assert.ok(screenEN.has(`siteConfig.unreadable.${reason}`), reason);
  }
  // The Go refusal codes are the ones the catalogue carries.
  const go = readFileSync(new URL('../../cmd/panel/site_config.go', import.meta.url), 'utf8');
  for (const match of go.matchAll(/= "(SITE_CONFIG_[A-Z_]+)"/g)) assert.ok(errEN.has(`err.${match[1]}`), match[1]);
});
