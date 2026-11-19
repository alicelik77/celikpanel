// alpha.139 (owner, 19 Nov 2026: "email için genel olarak ayarlar tabı var
// ama her email için de bir ayar bilgisi göstersek fena olmaz"): each mailbox
// shows its own connection settings.
//
//   - "Connection settings…" is the first entry of a mailbox row's "More"
//     menu; the row gains no button (a list that only reads, which has no
//     menu, shows it beside "Webmail");
//   - it opens the panel's own window titled with the address: the user name,
//     IMAP, POP3 and SMTP from the Settings tab's own read, the row's webmail
//     link when there is one, each copyable, the password line, and "Copy all"
//     with a plain text in the page's language that never holds a password;
//   - an unknown read says so with Retry and shows no values; a domain whose
//     mail is not ready shows the tab's own line once above the values;
//   - the success notice after a mailbox is made offers the same window.
//
// Mounted with the shipped list, window, kit, shared layer and the tab's own
// problem lines; roles administrator, reseller, customer and team member.
//
// Her posta kutusu kendi bağlantı bilgilerini gösterir: Diğer menüsünün ilk
// girdisi, adresle başlıklı pencere, Tümünü kopyala (parolasız), okunamayan
// yanıt, hazır olmayan posta ve oluşturma sonucundaki düğme.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import ts from 'typescript';
import { dataModule, reactURL, sharedLayer, compileSource, dateFormatURL } from './fixtures/shared-layer.mjs';
import { enMailDatabasesScreens } from '../src/i18n/screens/mailDatabases/en.ts';
import { trMailDatabasesScreens } from '../src/i18n/screens/mailDatabases/tr.ts';
import { enMailboxConnection } from '../src/i18n/mailboxConnection/en.ts';
import { trMailboxConnection } from '../src/i18n/mailboxConnection/tr.ts';

const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const icons = new Set(['AlertTriangle']);
for (const file of ['DomainMailManager', 'OutgoingMailboxLimits', 'WebmailAccess', 'MailboxConnectionDialog', 'MailAutoconfigNotice', 'domain/MailHostProtection']) {
  for (const match of source(`components/${file}.tsx`).matchAll(/import\s*\{([^}]+)\}\s*from 'lucide-react'/g)) {
    for (const item of match[1].split(',')) if (item.trim()) icons.add(item.trim());
  }
}

// The words: a key with its values (what a screen asked for) unless a test
// reads the shipped catalogue in one language.
// Sözler: anahtar ve değerleri; bir test bir dilin gönderilen metnini okuyabilir.
const catalogue = {
  en: { ...enMailDatabasesScreens, ...enMailboxConnection },
  tr: { ...trMailDatabasesScreens, ...trMailboxConnection },
};
globalThis.wordsFor = (locale) => (locale
  ? { locale, t: (key, vars) => Object.entries(vars ?? {}).reduce((text, [name, value]) => text.split(`{${name}}`).join(String(value)), catalogue[locale][key] ?? key) }
  : { locale: 'en', t: (key, vars) => (vars ? key + JSON.stringify(vars) : key) });
const stub = dataModule(`
  import React from '${reactURL}';
  ${[...icons].map((name) => `export const ${name} = () => null;`).join('\n')}
  const words = () => globalThis.currentTest.words;
  export const useI18n = words;
  export const useMailOutgoingWords = words;
  export const useMailboxConnectionWords = words;
  export const useMailConfigWords = words;
  export const useNavigate = () => (to) => { globalThis.currentTest.navigated.push(to); return true; };
  export const useAuth = () => ({ role: globalThis.currentTest.role, user: { role: globalThis.currentTest.role } });
  export const Link = (props) => React.createElement('a', { href: props.to }, props.children);
  export const showToast = (...args) => globalThis.currentTest.toasts.push(args);
  export const MailAuthPanel = () => null;
  export const MailSettingsPanel = () => null;
  export const rblSentence = () => null;
  export const MailAutoresponderDialog = () => null;
  export const runBulk = async () => ({ done: [], refused: [], unknown: null, notStarted: [] });
`);
const shared = sharedLayer(stub);
const coverageURL = compileSource('components/domain/coverageLines.ts', (specifier) => (specifier.endsWith('/dateFormat') ? dateFormatURL : stub));
const hostLineURL = shared.compile('components/domain/MailHostProtection.tsx', { '/coverageLines': coverageURL });
const autoconfigURL = shared.compile('components/MailAutoconfigNotice.tsx');
const windowURL = shared.compile('components/MailboxConnectionDialog.tsx', {
  '/MailAutoconfigNotice': autoconfigURL, '/MailHostProtection': hostLineURL, '/coverageLines': coverageURL,
});
const none = 'data:text/javascript,export default () => null';
const managerText = source('components/DomainMailManager.tsx')
  .replace("import('./WebmailAccess')", `import('${shared.compile('components/WebmailAccess.tsx')}')`)
  .replaceAll("import('./OutgoingMailboxLimits')", `import('${shared.compile('components/OutgoingMailboxLimits.tsx')}')`)
  .replace("import('./MailDeliveryLookup')", `import('${none}')`)
  .replace("import('./DomainMailOptions')", `import('${none}')`)
  .replace("import('./MailboxRecoveryDialog')", `import('${none}')`)
  .replace("import('./MailboxConnectionDialog')", `import('${windowURL}')`);
assert.ok(managerText.includes(windowURL), 'the window is not loaded lazily by the mailbox list');
const compiled = ts.transpileModule(managerText, {
  compilerOptions: { jsx: ts.JsxEmit.React, module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 },
}).outputText.replace(/from ['"]([^'"]+)['"]/g, (_, specifier) => `from '${specifier === 'react' ? reactURL : shared.resolve(specifier)}'`);
const Inner = (await import(dataModule(`import React from '${reactURL}';\n${compiled}`))).DomainMailManager;
const DomainMailManager = (props) => React.createElement(React.Suspense, { fallback: null }, React.createElement(Inner, props));
const { connectionText, notReadyLine } = await import(windowURL);
const { decodeMailSetup } = await import(shared.resolve('../lib/mailSetup'));

// --- The server's answers ---------------------------------------------------
const protocol = (port, security) => ({ host: 'mail.example.com', port, security });
const setup = (overrides = {}) => ({
  mail_host: 'mail.example.com', imap: protocol(993, 'SSL/TLS'), pop3: protocol(995, 'SSL/TLS'), smtp: protocol(587, 'STARTTLS'),
  username_is_full_email: true, webmail_available: true, webmail_url: 'https://mail.example.com/', ...overrides,
});
const notHere = { state: 'unavailable', reason: 'mail_not_here', names: ['autoconfig.example.com', 'autodiscover.example.com'], records: [] };
const hostLine = (state, reason) => ({ kind: 'mail_programs', name: 'mail.example.com', state, ips: ['203.0.113.9'],
  ...(reason ? { reason } : { by: 'server_mail_certificate', until: '2027-02-01T00:00:00Z' }) });
const accounts = [{ id: 5, address: 'info@example.com', quota_mb: 1024 }];
const PASSWORD = 'Kahve-Dukkani-2026!';

const originalFetch = globalThis.fetch;
const originalNavigator = Object.getOwnPropertyDescriptor(globalThis, 'navigator');
let tree;
let calls;
let copied;
let table;
function serve({ role = 'customer', webmail = setup(), locale } = {}) {
  calls = [];
  copied = [];
  globalThis.currentTest = { toasts: [], navigated: [], role, words: globalThis.wordsFor(locale) };
  globalThis.document = { addEventListener() {}, removeEventListener() {}, body: {} };
  globalThis.window = { innerHeight: 800, innerWidth: 1200, addEventListener() {}, removeEventListener() {}, setTimeout: () => 0, clearTimeout() {} };
  Object.defineProperty(globalThis, 'navigator', { configurable: true, value: { clipboard: { writeText: async (text) => { copied.push(text); } } } });
  table = {
    '/api/v1/domains/1/mail/setup': webmail,
    '/api/v1/domains/1/mail/outgoing': { enforced: false, state: 'off', mailboxes: [] },
    '/api/v1/domains/1/mail/accounts': { accounts },
    '/api/v1/domains/1/mail/forwardings': { forwardings: [] },
    '/api/v1/domains/1/mail/quota': { plugin_enabled: true, usages: [] },
  };
  globalThis.fetch = (url, options = {}) => {
    const method = options.method || 'GET';
    calls.push({ method, url, body: options.body });
    if (method === 'POST' && url === '/api/v1/domains/1/mail/accounts') return Promise.resolve(Response.json({ success: true }));
    const answer = table[url.split('?')[0]];
    if (answer === 500) return Promise.resolve(Response.json({ error: 'the agent did not answer' }, { status: 500 }));
    if (method !== 'GET' || !answer) return Promise.resolve(Response.json({ error: 'no such route in this test' }, { status: 404 }));
    return Promise.resolve(Response.json(answer));
  };
}
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
async function mount(props = {}) {
  await act(async () => {
    tree = Renderer.create(React.createElement(DomainMailManager, { domainId: 1, domainName: 'example.com', ...props }));
    for (let i = 0; i < 5; i++) await settled();
  });
}
async function cleanup() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  if (originalNavigator) Object.defineProperty(globalThis, 'navigator', originalNavigator);
  delete globalThis.currentTest;
  delete globalThis.document;
  delete globalThis.window;
}
const scenario = (name, body) => test(name, async () => {
  try {
    await body();
  } finally {
    await cleanup();
  }
});
const words = (node) => (typeof node === 'string' ? node : (node?.children ?? []).map(words).join(' '));
const attr = (name, value, root = tree.root) => root.findAll((node) => typeof node.type === 'string' && node.props[name] !== undefined && (value === undefined || node.props[name] === value));
const press = async (node) => {
  await act(async () => {
    await node.props.onClick({ stopPropagation() {}, preventDefault() {} });
    for (let i = 0; i < 5; i++) await settled();
  });
};
const visible = (address) => attr('data-row-actions', address)[0].findAll((node) => typeof node.type === 'string' && node.props['data-row-action'] !== undefined && node.props.role !== 'menuitem').map((node) => node.props['data-row-action']);
const menu = async (address) => {
  await press(attr('data-row-more', address)[0]);
  const items = attr('data-row-menu', address)[0].findAll((node) => node.props.role === 'menuitem');
  return items;
};
const openWindow = async (address = 'info@example.com') => {
  const entry = (await menu(address)).find((node) => node.props['data-row-action'] === 'connection');
  await press(entry);
};
const dialog = () => tree.root.findAll((node) => node.props.role === 'dialog')[0];
const shownValues = () => attr('data-mailbox-copy', undefined, dialog()).map((node) => node.props['data-mailbox-copy']);

for (const role of ['admin', 'reseller', 'customer', 'additional_user']) {
  scenario(`${role}: "Connection settings…" is the menu's first entry and the row gains no button; it opens this mailbox's values`, async () => {
    serve({ role });
    await mount();
    assert.deepEqual(visible('info@example.com'), ['webmail', 'password'], 'the row has a new button');
    const items = await menu('info@example.com');
    assert.equal(items[0].props['data-row-action'], 'connection');
    assert.equal(items[0].props['aria-disabled'], undefined, 'reading the settings is never off');
    assert.match(words(items[0]), /mail\.row\.connection/);
    await press(items[0]);
    const window = dialog();
    assert.ok(window, 'the panel’s own window did not open');
    assert.match(words(window), /info@example\.com/);
    // The values: the address, each server name, the row's own webmail link.
    assert.deepEqual(shownValues(), [
      'info@example.com', 'mail.example.com', 'mail.example.com', 'mail.example.com', 'https://mail.example.com/?_user=info%40example.com',
    ]);
    for (const [name, port, security] of [['IMAP', '993', 'SSL/TLS'], ['POP3', '995', 'SSL/TLS'], ['SMTP', '587', 'STARTTLS']]) {
      const block = attr('data-mailbox-protocol', name, window)[0];
      const said = words(block);
      assert.ok(said.includes(port) && said.includes(security) && said.includes('mail.setup.server'), `${name}: ${said}`);
    }
    // The password line names "Password…" as the way to a new one; no value of it anywhere.
    assert.match(words(attr('data-mailbox-password-line', undefined, window)[0]), /mconn\.password$/);
    assert.equal(window.findAll((node) => node.type === 'input' || node.type === 'textarea').length, 0, 'the window asks for or holds a value');
    // The same read as the Settings tab: one GET of the setup address, nothing else asked.
    assert.ok(calls.every((call) => call.method === 'GET'));
    assert.deepEqual([...new Set(calls.map((call) => call.url))].sort(), Object.keys(table).sort());
    // One value copied; the line under the values says so.
    await press(attr('data-mailbox-copy', 'mail.example.com', window)[0]);
    assert.deepEqual(copied, ['mail.example.com']);
    assert.equal(attr('data-mailbox-copy-said', 'copied').length, 1);
  });
}

scenario('team member who only reads: the entry beside Webmail (no menu), the window, and who may set a password', async () => {
  serve({ role: 'additional_user' });
  await mount({ readOnly: true });
  assert.deepEqual(visible('info@example.com'), ['webmail', 'connection']);
  assert.equal(attr('data-row-more').length, 0);
  await press(attr('data-row-action', 'connection')[0]);
  assert.ok(dialog());
  assert.match(words(attr('data-mailbox-password-line')[0]), /mconn\.passwordReadOnly/);
  assert.equal(attr('data-row-action', 'password').length, 0);
});

scenario('without webmail the window has no webmail value and Copy all no webmail line', async () => {
  serve({ role: 'customer', webmail: setup({ webmail_available: false, webmail_url: '', webmail_reason: 'not_installed' }) });
  await mount();
  assert.deepEqual(visible('info@example.com'), ['password']);
  await openWindow();
  assert.deepEqual(shownValues(), ['info@example.com', 'mail.example.com', 'mail.example.com', 'mail.example.com']);
  await press(attr('data-mailbox-copy-all')[0]);
  assert.equal(copied.length, 1);
  assert.doesNotMatch(copied[0], /mail\.row\.webmail|https:/);
});

const expected = {
  en: [
    'Mail settings for info@example.com',
    '',
    'Username: info@example.com',
    'Password: the one you were given separately (not in this message)',
    '',
    'Incoming mail (IMAP)',
    '  Server: mail.example.com',
    '  Port: 993',
    '  Security: SSL/TLS',
    '',
    'Incoming mail, alternative (POP3)',
    '  Server: mail.example.com',
    '  Port: 995',
    '  Security: SSL/TLS',
    '',
    'Outgoing mail (SMTP)',
    '  Server: mail.example.com',
    '  Port: 587',
    '  Security: STARTTLS',
    '',
    'Webmail: https://mail.example.com/?_user=info%40example.com',
  ].join('\n'),
  tr: [
    'info@example.com için posta ayarları',
    '',
    'Kullanıcı adı: info@example.com',
    'Parola: size ayrıca iletilen parola (bu mesajda yok)',
    '',
    'Gelen posta (IMAP)',
    '  Sunucu: mail.example.com',
    '  Port: 993',
    '  Güvenlik: SSL/TLS',
    '',
    'Gelen posta, diğer seçenek (POP3)',
    '  Sunucu: mail.example.com',
    '  Port: 995',
    '  Güvenlik: SSL/TLS',
    '',
    'Giden posta (SMTP)',
    '  Sunucu: mail.example.com',
    '  Port: 587',
    '  Güvenlik: STARTTLS',
    '',
    'Web posta: https://mail.example.com/?_user=info%40example.com',
  ].join('\n'),
};

for (const locale of ['en', 'tr']) {
  scenario(`${locale}: a mailbox made here offers its settings in the result; Copy all is the page's language and never the password`, async () => {
    serve({ role: 'customer', locale });
    await mount();
    // Make a mailbox with a password, as the form does.
    await press(tree.root.findAll((node) => node.type === 'button' && words(node).includes(catalogue[locale]['mail.addAccount']))[0]);
    const inputs = tree.root.findAll((node) => node.type === 'input');
    await act(async () => { inputs.find((node) => node.props.placeholder === 'info').props.onChange({ target: { value: 'info' } }); });
    const passwordField = tree.root.findAll((node) => node.type === 'input' && (node.props.type === 'password' || node.props.autoComplete === 'new-password'))[0];
    await act(async () => { passwordField.props.onChange({ target: { value: PASSWORD } }); });
    await press(tree.root.findAll((node) => node.type === 'button' && words(node).trim().endsWith(catalogue[locale]['mail.create']))[0]);
    assert.ok(calls.some((call) => call.method === 'POST' && JSON.parse(call.body).password === PASSWORD), 'the mailbox was not made');
    // The result's one button opens the same window for the new address.
    const [kind, , extra] = globalThis.currentTest.toasts.at(-1);
    assert.equal(kind, 'success');
    assert.equal(extra.action.label, catalogue[locale]['mail.row.connectionShow']);
    assert.equal(extra.key, 'mail-created:info@example.com');
    assert.equal(dialog(), undefined);
    await act(async () => { extra.action.run(); for (let i = 0; i < 5; i++) await settled(); });
    assert.ok(dialog(), 'the result did not open the window');
    await press(attr('data-mailbox-copy-all')[0]);
    assert.deepEqual(copied, [expected[locale]]);
    assert.ok(!copied[0].includes(PASSWORD) && !JSON.stringify(tree.toJSON()).includes(PASSWORD), 'the password is in the window or its text');
    assert.match(words(attr('data-mailbox-copy-said', 'copiedAll')[0]), new RegExp(catalogue[locale]['mconn.copiedAll'].slice(0, 20)));
  });
}

scenario('the read could not be made: what could not be read, Retry, no values and nothing to copy; Retry reads again', async () => {
  serve({ role: 'customer' });
  table['/api/v1/domains/1/mail/setup'] = 500;
  await mount();
  await openWindow();
  const window = dialog();
  assert.match(words(window), /mail\.setup\.unknown/);
  assert.equal(attr('data-mailbox-connection', undefined, window).length, 0, 'values are drawn although the read failed');
  assert.equal(attr('data-mailbox-copy-all')[0].props.disabled, true);
  table['/api/v1/domains/1/mail/setup'] = setup();
  const retry = window.findAll((node) => node.type === 'button' && words(node).includes('common.retry'))[0];
  await press(retry);
  assert.equal(attr('data-mailbox-connection', 'info@example.com').length, 1);
  assert.equal(attr('data-mailbox-copy-all')[0].props.disabled, false);
});

scenario('mail not ready: the tab’s own line, once, above the values', async () => {
  const cases = [
    [setup({ autoconfig: notHere }), 'mail', /mcfg\.auto\.reason\.mail_not_here/],
    [setup({ autoconfig: { ...notHere, reason: 'mail_host_unknown' } }), 'mail', /mcfg\.auto\.reason\.mail_host_unknownCustomer/],
    [setup({ host_protection: hostLine('not_protected', 'points_elsewhere') }), 'host', /certcover\.reason\.points_elsewhere/],
    [setup({ host_protection: hostLine('not_protected', 'not_in_certificate') }), 'host', /certcover\.reason\.not_in_certificate/],
    // Both: the more basic one, once.
    [setup({ autoconfig: notHere, host_protection: hostLine('not_protected', 'no_address') }), 'mail', /mcfg\.auto\.reason\.mail_not_here/],
  ];
  for (const [answer, kind, sentence] of cases) {
    serve({ role: 'customer', webmail: answer });
    await mount();
    await openWindow();
    const lines = attr('data-mailbox-connection-problem');
    assert.equal(lines.length, 1, kind);
    assert.equal(lines[0].props['data-mailbox-connection-problem'], kind);
    assert.match(words(lines[0]), sentence);
    assert.deepEqual(shownValues().slice(0, 2), ['info@example.com', 'mail.example.com'], 'the values are still shown');
    await cleanup();
  }
  // Ready, protected, a subdomain or a partial set-up: no line (not a new check).
  for (const answer of [setup(), setup({ host_protection: hostLine('protected') }), setup({ autoconfig: { ...notHere, reason: 'subdomain' } }),
    setup({ autoconfig: { state: 'partial', reason: '', served: 'no', records: [] } }), setup({ host_protection: hostLine('unknown', 'not_checked') })]) {
    assert.equal(notReadyLine(decodeMailSetup(answer)), null, JSON.stringify(answer));
  }
});

test('the copy-all text holds every value of the read and no password field', () => {
  const t = globalThis.wordsFor('en').t;
  const text = connectionText(t, 'a+b@example.com', decodeMailSetup(setup()));
  assert.match(text, /Username: a\+b@example\.com/);
  assert.match(text, /_user=a%2Bb%40example\.com/);
  // The only password line says where the password comes from, never one.
  assert.deepEqual(text.split('\n').filter((line) => /password/i.test(line)), ['Password: the one you were given separately (not in this message)']);
});

test('the window’s words: every key in English and Turkish with the same values, and the window loads lazily', () => {
  const vars = (value) => [...value.matchAll(/\{(\w+)\}/g)].map((match) => match[1]).sort().join(',');
  for (const key of Object.keys(enMailboxConnection)) {
    assert.ok(trMailboxConnection[key] && trMailboxConnection[key] !== enMailboxConnection[key], key);
    assert.equal(vars(trMailboxConnection[key]), vars(enMailboxConnection[key]), key);
  }
  for (const key of ['mail.row.connection', 'mail.row.connectionShow']) assert.ok(enMailDatabasesScreens[key] && trMailDatabasesScreens[key], key);
  assert.equal(trMailDatabasesScreens['mail.row.connection'], 'Bağlantı bilgileri…');
  const manager = source('components/DomainMailManager.tsx');
  assert.match(manager, /const MailboxConnectionDialog = lazy\(\(\) => import\('\.\/MailboxConnectionDialog'\)\);/);
  assert.doesNotMatch(manager, /from '\.\/MailboxConnectionDialog'/);
  // The window has no request shape of its own: the Settings tab's reader.
  const window = source('components/MailboxConnectionDialog.tsx');
  assert.match(window, /useRemote\(mailSetupURL\(domainId\), decodeMailSetup\)/);
  assert.doesNotMatch(window, /fetch\(|['"`]\/api\/v1/);
});
