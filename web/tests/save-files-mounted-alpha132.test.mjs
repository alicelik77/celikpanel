import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';

// One way of saving on a domain's file editor, automatic backups' neighbours
// and edit forms (alpha.132; owner, 8 Nov 2026: Save turns on only when
// something changed, nothing is saved without it, unsaved changes show, and
// they are not dropped unseen). Mounted with the shipped save parts
// (components/kit/save), the shipped confirmation and the real readers:
//
//   - the file editor: Save only for changed text; Discard; closing asks; a
//     file changed on the server meanwhile is never overwritten unseen; a
//     write whose answer was lost is not sent twice;
//   - a mailbox's sending limit: its own window with Save and Discard
//     (alpha.135; it was a draft with the save bar under the list);
//   - the out-of-office reply, a site's other login and a scheduled task:
//     Save only when something changed and the form can be saved, the reason
//     beside it; Discard; closing with changes asks.
//
// Dosya düzenleyicisi, gönderim sınırı ve düzenleme formları tek kaydetme
// düzeniyle, gerçek kaydetme parçaları ve gerçek onayla bağlı hâlde.

const icons = ['Folder', 'File', 'ChevronRight', 'ChevronUp', 'RefreshCw', 'Trash2', 'Edit', 'Download', 'Upload', 'X',
  'FolderPlus', 'FilePlus', 'AlertTriangle', 'KeyRound', 'Send', 'CalendarClock', 'FolderLock', 'Plus', 'UserRound',
  'Clock', 'Edit2', 'Play', 'Pause', 'Save', 'Info'];
const stub = dataModule(`
  import React from '${reactURL}';
  ${icons.map((name) => `export const ${name} = () => null;`).join('\n')}
  const i18n = { t: (key, vars) => (vars ? key + JSON.stringify(vars) : key), locale: 'en' };
  export const useI18n = () => i18n;
  export const useMailOutgoingWords = () => i18n;
  export const useNavigate = () => () => true;
  export const useAuth = () => ({ role: 'customer' });
  export const AdminInstallLink = () => null;
  export const showToast = (type, message) => { globalThis.toasts.push([type, message]); };
`);
const shared = sharedLayer(stub);
const currentURL = shared.compile('components/CurrentSettings.tsx');
const { DomainFileManager } = await import(shared.compile('components/DomainFileManager.tsx'));
const { OutgoingLimitDialog } = await import(shared.compile('components/OutgoingMailboxLimits.tsx'));
const { MailAutoresponderDialog } = await import(shared.compile('components/MailAutoresponderDialog.tsx'));
const { FTPLogins } = await import(shared.compile('components/FTPLogins.tsx'));
const { DomainCronManager } = await import(shared.compile('components/DomainCronManager.tsx', { '/CurrentSettings': currentURL }));
const { askToLeave } = await import(shared.resolve('/leaveAsker'));

const originalFetch = globalThis.fetch;
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
let tree;
let calls;
let asked;
// routes: [method, url fragment, answer(request) | 'offline']; the first match answers.
function serve(routes) {
  calls = [];
  asked = [];
  globalThis.toasts = [];
  globalThis.document = { addEventListener() {}, removeEventListener() {} };
  globalThis.confirm = () => { throw new Error('a question was asked that the test did not expect'); };
  globalThis.fetch = (url, options = {}) => {
    const method = options.method || 'GET';
    const body = options.body ? JSON.parse(options.body) : undefined;
    calls.push({ method, url, body });
    const route = routes.find(([m, fragment, , when]) => m === method && url.includes(fragment) && (!when || when(body)));
    if (!route) return Promise.reject(new TypeError('no route ' + method + ' ' + url));
    if (route[2] === 'offline') return Promise.reject(new TypeError('fetch failed'));
    return Promise.resolve(route[2]({ method, url, body }));
  };
}
const answering = (answer) => {
  globalThis.confirm = (text, request) => { asked.push(request); return answer; };
};
async function mount(element) {
  await act(async () => {
    tree = Renderer.create(element);
    await settled();
    await settled();
  });
}
async function cleanup() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  delete globalThis.document;
}
const scenario = (name, body) => test(name, async () => {
  try {
    await body();
  } finally {
    await cleanup();
  }
});
const text = () => JSON.stringify(tree.toJSON());
const words = (node) => (typeof node === 'string' ? node : (node?.children ?? []).map(words).join(' '));
const byData = (name) => tree.root.findAll((node) => typeof node.type === 'string' && node.props[name] !== undefined);
const one = (name) => byData(name)[0];
const buttonWith = (key) => tree.root.findAll((node) => node.type === 'button' && words(node).includes(key))[0];
const press = async (node, event = {}) => {
  await act(async () => {
    await node.props.onClick({ stopPropagation() {}, preventDefault() {}, ...event });
    await settled();
    await settled();
  });
};
const type = async (node, value) => act(async () => node.props.onChange({ target: { value, checked: value } }));
const writes = () => calls.filter((call) => call.method !== 'GET' && call.body?.action !== 'read');
// Whether leaving the page would ask now; the question is answered "stay"
// and not counted.
// Sayfadan çıkmak şimdi sorar mı; soru "kal" diye yanıtlanır, sayılmaz.
const unsaved = () => {
  const answer = globalThis.confirm;
  globalThis.confirm = () => false;
  try {
    return askToLeave() !== null;
  } finally {
    globalThis.confirm = answer;
  }
};

// --- The file editor ---

const FILES = '/api/v1/domains/1/files?path=';
const listing = { files: [{ name: 'index.php', path: '/index.php', is_dir: false, size: 20, permissions: '0644', mod_time: '2026-11-11T10:00:00Z' }] };
const fileRoutes = (server, write = () => Response.json({ success: true })) => [
  ['GET', FILES, () => Response.json(listing)],
  ['POST', FILES, () => Response.json({ path: '/index.php', content: server.text, size: server.text.length, is_binary: false }), (body) => body.action === 'read'],
  ['POST', FILES, (request) => write(request), (body) => body.action === 'write'],
];
async function openEditor(server, write) {
  serve(fileRoutes(server, write));
  await mount(React.createElement(DomainFileManager, { domainId: 1, domainName: 'site.example' }));
  await press(tree.root.findAll((node) => node.type === 'button' && node.props.title === 'files.edit')[0]);
  return tree.root.findByType('textarea');
}

scenario('file editor: Save appears only for changed text, sends exactly the text, and Discard restores it', async () => {
  const server = { text: '<?php echo 1;' };
  let editor = await openEditor(server);
  assert.equal(editor.props.value, '<?php echo 1;');
  assert.equal(byData('data-save').length, 0, 'Save is offered with nothing changed');
  assert.equal(byData('data-changed-mark').length, 0);
  await type(editor, '<?php echo 2;');
  assert.equal(writes().length, 0, 'typing wrote the file');
  assert.equal(byData('data-changed-mark').length, 1, 'the unsaved text is not marked');
  assert.ok(unsaved(), 'leaving the page with unsaved text does not ask');
  await press(one('data-save-discard'));
  editor = tree.root.findByType('textarea');
  assert.equal(editor.props.value, '<?php echo 1;');
  assert.equal(byData('data-save').length, 0);
  assert.ok(!unsaved());

  await type(editor, '<?php echo 3;');
  await press(one('data-save'));
  assert.deepEqual(writes().map((call) => call.body), [{ action: 'write', content: '<?php echo 3;' }]);
  assert.ok(calls.some((call) => call.body?.action === 'read' && calls.indexOf(call) > 1), 'the file was not read again before writing');
  assert.equal(byData('data-save').length, 0, 'the bar stays after the file was saved');
  assert.ok(globalThis.toasts.some(([kind, key]) => kind === 'success' && key === 'files.saved'));
  assert.equal(tree.root.findByType('textarea').props.value, '<?php echo 3;', 'the editor closed or lost the saved text');
});

scenario('file editor: a file changed on the server meanwhile is not overwritten; the person chooses', async () => {
  const server = { text: 'one' };
  const editor = await openEditor(server);
  await type(editor, 'mine');
  server.text = 'theirs';
  await press(one('data-save'));
  assert.equal(writes().length, 0, 'a file changed on the server was overwritten unseen');
  assert.equal(byData('data-file-moved').length, 1, 'the change on the server is not said');
  assert.equal(one('data-save').props.disabled, true, 'Save is offered before the person chose');
  assert.equal(tree.root.findByType('textarea').props.value, 'mine', 'what was typed was dropped');

  // Writing mine over it is asked, and runs in the progress window.
  answering(true);
  await press(buttonWith('files.overwrite'));
  assert.equal(asked.length, 1);
  assert.equal(typeof asked[0].action, 'function');
  assert.deepEqual(writes().map((call) => call.body), [{ action: 'write', content: 'mine' }]);
});

scenario('file editor: opening the server’s version over unsaved text asks, then shows it', async () => {
  const server = { text: 'one' };
  const editor = await openEditor(server);
  await type(editor, 'mine');
  server.text = 'theirs';
  await press(one('data-save'));
  answering(false);
  await press(buttonWith('files.takeServer'));
  assert.equal(tree.root.findByType('textarea').props.value, 'mine', 'the person said no and lost their text');
  answering(true);
  await press(buttonWith('files.takeServer'));
  assert.equal(tree.root.findByType('textarea').props.value, 'theirs');
  assert.equal(byData('data-file-moved').length, 0);
  assert.equal(byData('data-save').length, 0);
  assert.equal(writes().length, 0);
});

scenario('file editor: a refused write keeps the text and Save; a lost answer is not sent twice and a read decides', async () => {
  const server = { text: 'one' };
  let reply = () => Response.json({ error: 'gone', code: 'FILE_NOT_FOUND' }, { status: 404 });
  const editor = await openEditor(server, () => reply());
  await type(editor, 'two');
  await press(one('data-save'));
  assert.ok(globalThis.toasts.some(([kind, key]) => kind === 'error' && key === 'files.notFound'));
  assert.equal(tree.root.findByType('textarea').props.value, 'two');
  assert.equal(one('data-save').props.disabled, false, 'Save is not offered again after a refusal');

  reply = () => { throw new TypeError('fetch failed'); };
  await press(one('data-save'));
  assert.equal(writes().length, 2);
  assert.equal(one('data-save').props.disabled, true, 'Save is offered again before the file was read');
  await press(one('data-save'));
  assert.equal(writes().length, 2, 'a write whose answer was lost was sent a second time');
  // The write did arrive: the file read again holds the text.
  server.text = 'two';
  await press(one('data-save-check'));
  assert.equal(byData('data-save-bar').length, 0, 'the saved text is still said to be unsaved');
  assert.equal(writes().length, 2);
});

scenario('file editor: closing with unsaved text asks; staying keeps it, leaving closes the editor', async () => {
  const server = { text: 'one' };
  const editor = await openEditor(server);
  await type(editor, 'two');
  answering(false);
  await press(buttonWith('common.close'));
  assert.equal(asked.length, 1);
  assert.equal(asked[0].confirmLabel, 'domain.unsaved.close');
  assert.equal(tree.root.findByType('textarea').props.value, 'two');
  answering(true);
  await press(buttonWith('common.close'));
  assert.equal(tree.root.findAllByType('textarea').length, 0, 'the editor stayed open');
  assert.ok(!unsaved(), 'the closed editor still holds the page');
  assert.equal(writes().length, 0);
});

// --- A mailbox's sending limit ---

const OUT = '/api/v1/domains/3/mail/outgoing';
const mailbox = (id, address, effective) => ({ id, address, effective: { limit: 100, default: 100, ceiling: 500, ...effective }, used_hour: 0, sent_7d: 0, refused_7d: 0 });
const outgoing = {
  enforced: true, state: 'on', domain_limit: 0, domain_used_hour: 0, subscription_limit: 0, ceiling: 500, default: 100, may_exceed_ceiling: false,
  mailboxes: [mailbox(11, 'a@site.example', { mode: 'default' }), mailbox(12, 'b@site.example', { mode: 'custom', custom: 50 })],
};

const limitSave = () => one('data-form-save');
const limitReason = () => words(one('data-form-reason'));
const submitLimit = async () => {
  await act(async () => tree.root.findByType('form').props.onSubmit({ preventDefault() {} }));
  await act(settled);
};

scenario('sending limit: Save only after a change; a number over the plan is named; Save sends this mailbox once', async () => {
  let closed = 0;
  serve([['GET', OUT, () => Response.json(outgoing)], ['PUT', OUT, () => Response.json({ applied: true })]]);
  await mount(React.createElement(OutgoingLimitDialog, { domainId: 3, accountId: 12, address: 'b@site.example', onClose: () => { closed++; } }));
  const select = () => tree.root.findAllByType('select')[0];
  const number = () => tree.root.findAll((node) => node.type === 'input' && node.props.type === 'number')[0];
  assert.equal(select().props.value, 'custom');
  assert.equal(number().props.value, '50');
  assert.equal(limitSave().props.disabled, true);
  assert.equal(limitReason(), 'domain.unsaved.none');
  assert.match(text(), /mout\.dom\.ceiling\{\\"n\\":500\}/);
  await type(number(), '900');
  assert.equal(limitSave().props.disabled, true, 'a number over the plan’s ceiling can be saved');
  assert.match(limitReason(), /^mout\.dom\.invalid/);
  assert.ok(unsaved(), 'leaving with a change does not ask');
  await type(number(), '80');
  assert.equal(limitSave().props.disabled, false);
  assert.equal(byData('data-changed-mark').length, 1);
  assert.equal(writes().length, 0, 'something was sent before Save');
  await submitLimit();
  assert.deepEqual(writes().map((call) => call.body), [{ id: 12, mode: 'custom', per_hour: 80 }]);
  assert.ok(globalThis.toasts.some(([kind, key]) => kind === 'success' && key === 'mout.dom.saved'));
  assert.equal(closed, 1);
  // "No limit" is not offered above a plan's ceiling.
  assert.ok(!tree.root.findAllByType('option').some((node) => node.props.value === 'unlimited'));
});

scenario('sending limit: a refusal keeps the choice; a lost answer is not sent twice and a read that holds it closes; Discard closes unsaved', async () => {
  let closed = 0;
  let current = outgoing;
  let reply = () => Response.json({ error: 'no', code: 'OUTGOING_ABOVE_CEILING' }, { status: 403 });
  serve([['GET', OUT, () => Response.json(current)], ['PUT', OUT, () => reply()]]);
  await mount(React.createElement(OutgoingLimitDialog, { domainId: 3, accountId: 11, address: 'a@site.example', onClose: () => { closed++; } }));
  await type(tree.root.findAllByType('select')[0], 'custom');
  await submitLimit();
  assert.ok(globalThis.toasts.some(([kind]) => kind === 'error'));
  assert.equal(closed, 0);
  assert.equal(tree.root.findAllByType('select')[0].props.value, 'custom');
  assert.equal(limitSave().props.disabled, false);
  // No answer: Save holds; pressing again sends nothing.
  reply = () => { throw new TypeError('fetch failed'); };
  await submitLimit();
  assert.equal(writes().length, 2);
  assert.equal(limitSave().props.disabled, true, 'a lost save can be sent again');
  await submitLimit();
  assert.equal(writes().length, 2, 'a lost save was sent twice');
  // The limits read again hold it: saved, and the window closes.
  current = { ...outgoing, mailboxes: [mailbox(11, 'a@site.example', { mode: 'custom', custom: 100 }), outgoing.mailboxes[1]] };
  await press(buttonWith('common.checkAgain'));
  assert.equal(closed, 1);
  assert.equal(writes().length, 2);
  // Discard closes without a question and sends nothing.
  await type(tree.root.findAllByType('select')[0], 'default');
  await press(one('data-form-discard'));
  assert.equal(closed, 2);
  assert.equal(writes().length, 2);
});

// --- The out-of-office reply ---

const REPLY = '/api/v1/domains/4/mail/autoresponder';
const stored = { autoresponder: { enabled: false, subject: 'Away', message: '', interval_days: 1, addresses: [] }, stored: true, state: { applied: true, script: 'managed', sieve_available: true } };
const formSave = () => one('data-form-save');
const reason = () => words(one('data-form-reason'));

scenario('out-of-office reply: Save is off until a change; a missing message is named; Save sends the reply; Discard closes unsaved', async () => {
  let closed = 0;
  serve([['GET', REPLY, () => Response.json(stored)], ['PUT', REPLY, () => Response.json({ state: { applied: true } })]]);
  await mount(React.createElement(MailAutoresponderDialog, { domainId: 4, accountId: 9, address: 'a@site.example', onClose: () => { closed++; } }));
  assert.equal(formSave().props.disabled, true);
  assert.equal(reason(), 'domain.unsaved.none');
  const checkbox = tree.root.findAll((node) => node.type === 'input' && node.props.type === 'checkbox')[0];
  await act(async () => checkbox.props.onChange({ target: { checked: true } }));
  assert.equal(formSave().props.disabled, true, 'a reply without a message can be saved');
  assert.equal(reason(), 'mail.autoreply.messageRequired');
  assert.ok(unsaved());
  // Discard closes without a question and sends nothing.
  await press(one('data-form-discard'));
  assert.equal(closed, 1);
  assert.equal(writes().length, 0);

  await type(tree.root.findByType('textarea'), 'I am away.');
  assert.equal(formSave().props.disabled, false);
  await act(async () => tree.root.findByType('form').props.onSubmit({ preventDefault() {} }));
  await act(settled);
  assert.deepEqual(writes()[0].body, { id: 9, enabled: true, subject: 'Away', message: 'I am away.', start_date: '', end_date: '', interval_days: 1, addresses: [] });
  assert.equal(closed, 2);
});

scenario('out-of-office reply: closing with changes asks; a lost answer is not sent twice, and a read that holds it closes', async () => {
  let closed = 0;
  let current = stored;
  serve([['GET', REPLY, () => Response.json(current)], ['PUT', REPLY, 'offline']]);
  await mount(React.createElement(MailAutoresponderDialog, { domainId: 4, accountId: 9, address: 'a@site.example', onClose: () => { closed++; } }));
  const subject = () => tree.root.findAll((node) => node.type === 'input' && node.props.maxLength === 200)[0];
  await type(subject(), 'Back on Monday');
  // Escape or the backdrop: asked, and staying keeps the text.
  answering(false);
  await act(async () => { tree.root.findAll((node) => node.props.onMouseDown)[0].props.onMouseDown({ currentTarget: 1, target: 1 }); await settled(); });
  assert.equal(asked.length, 1);
  assert.equal(closed, 0);
  assert.equal(subject().props.value, 'Back on Monday');

  await act(async () => { tree.root.findByType('form').props.onSubmit({ preventDefault() {} }); await settled(); });
  assert.equal(writes().length, 1);
  assert.equal(formSave().props.disabled, true, 'Save is offered again before the reply was read');
  current = { ...stored, autoresponder: { ...stored.autoresponder, subject: 'Back on Monday' } };
  await press(buttonWith('common.checkAgain'));
  assert.equal(closed, 1, 'a reply the server holds is still said to be unsaved');
  assert.equal(writes().length, 1);
});

// --- A site's other login ---

const LOGINS = '/api/v1/domains/5/access/logins';
const loginsView = {
  domain: 'site.example', available: true, subscription_status: 'active', rights: { file_access: true }, owner: '', limit: { used: 1, ceiling: 10 },
  records: [{ login: 'site-design', folder: '/public_html' }],
  connection: { sftp_host: 'site.example', ftp_port: 21, sftp_port: 22, passive_min: 40000, passive_max: 40100 },
  state: {
    logins: [{ login: 'site-design', folder: '/public_html', account_ok: true, password: 'set', jail_active: true, unit_file: 'managed' }],
    server_logins: 1, sshd_present: true, sshd_drop_in: 'managed', ftp_installed: true, ftp_config: 'managed', warnings: [],
  },
};

scenario('other login: Save is off until a change, sends only what changed, Discard closes, closing with changes asks', async () => {
  serve([['GET', LOGINS, () => Response.json(loginsView)], ['PUT', LOGINS, () => Response.json({ warnings: [] })]]);
  await mount(React.createElement(FTPLogins, { domainId: 5, isAdmin: false }));
  await press(buttonWith('ftpLogins.change'));
  const folder = () => tree.root.findAll((node) => node.type === 'input' && node.props.maxLength === 512)[0];
  assert.equal(folder().props.value, 'public_html');
  assert.equal(formSave().props.disabled, true);
  assert.equal(reason(), 'domain.unsaved.none');
  await type(folder(), '');
  assert.equal(formSave().props.disabled, true, 'an empty folder can be saved');
  assert.equal(reason(), 'ftpLogins.folderMissing');
  await type(folder(), 'public_html/uploads');
  assert.equal(formSave().props.disabled, false);
  assert.ok(unsaved());
  // Closing the form with the change asks; staying keeps it.
  answering(false);
  await press(buttonWith('ftpLogins.change'));
  assert.equal(asked.length, 1);
  assert.equal(folder().props.value, 'public_html/uploads');
  // Save asks the change's own question and runs it in the progress window.
  answering(true);
  await press(formSave());
  assert.deepEqual(writes().map((call) => [call.method, call.body]), [['PUT', { folder: 'public_html/uploads' }]]);
  assert.equal(byData('data-form-save').length, 0, 'the form stayed open after the change was made');

  await press(buttonWith('ftpLogins.change'));
  await type(folder(), 'other');
  await press(one('data-form-discard'));
  assert.equal(byData('data-form-save').length, 0);
  assert.ok(!unsaved());
  assert.equal(writes().length, 1);
});

// --- A scheduled task ---

const CRON = '/api/v1/domains/6/cron';
const job = { id: '0000abcd', schedule: '0 3 * * *', command: '/usr/bin/true', enabled: true, comment: 'nightly' };

scenario('scheduled task: Save is off until a change, a missing command is named, Save keeps the task’s pause state, Discard closes', async () => {
  let listed = [job];
  serve([['GET', CRON, () => Response.json({ jobs: listed, version: 'ct1-a' })], ['PUT', CRON, () => Response.json({ success: true })]]);
  await mount(React.createElement(DomainCronManager, { domainId: 6, domainName: 'site.example' }));
  const edit = () => tree.root.findAll((node) => node.type === 'button' && node.props.title === 'cron.edit')[0];
  const command = () => tree.root.findAll((node) => node.type === 'input' && node.props.placeholder?.startsWith('/usr/bin/php'))[0];
  await press(edit());
  assert.equal(formSave().props.disabled, true);
  assert.equal(reason(), 'domain.unsaved.none');
  await type(command(), '  ');
  assert.equal(formSave().props.disabled, true);
  assert.equal(reason(), 'cron.commandRequired');
  await type(command(), '/usr/bin/php report.php');
  assert.equal(formSave().props.disabled, false);

  // Closing with the change asks; staying keeps it.
  answering(false);
  await press(tree.root.findAll((node) => node.type === 'button' && node.props.title === 'common.close')[0]);
  assert.equal(asked.length, 1);
  assert.equal(command().props.value, '/usr/bin/php report.php');

  // Paused from the list while the form is open: Save keeps it paused.
  listed = [{ ...job, enabled: false }];
  await press(tree.root.findAll((node) => node.type === 'button' && node.props.title === 'cron.disable')[0]);
  await press(formSave());
  const put = writes().filter((call) => call.body?.id === job.id && call.body.command === '/usr/bin/php report.php');
  assert.equal(put.length, 1);
  assert.equal(put[0].body.enabled, false, 'saving the form resumed a paused task');

  await press(edit());
  await type(command(), '/usr/bin/other');
  await press(one('data-form-discard'));
  assert.equal(byData('data-form-save').length, 0, 'Discard left the form open');
  assert.ok(!unsaved());
});
