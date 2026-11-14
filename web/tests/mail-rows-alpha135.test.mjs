// alpha.135 (owner, 13 and 14 Nov 2026, after alpha.133 on his servers): a
// domain's mailbox list.
//
//   - "webmail her emaile özgü link olsa, genel olmasa daha iyi olur. email
//     girili açacak": each row opens webmail with its own address filled in
//     (Roundcube 1.6 reads `_user` for its sign-in form); the general card is
//     gone when webmail works and stays only as the problem with its fix;
//   - "bu sınırlar nedir. çok fazla mı karmaşık ayar var acaba": no sending
//     limits card. While the limits are applied each row has one short line
//     and "Sending limit…" in its menu; while they are not, a customer,
//     reseller or team member sees nothing about them and the administrator
//     one line with its button; a domain or account limit is said only when it
//     matters; a stopped mailbox keeps its state and its way out.
//
// Mounted with the shipped list, webmail card, limit parts, kit and shared
// layer; the roles are administrator, reseller, customer and team member.
//
// Posta kutusu listesi: her satır web postasını kendi adresiyle açar;
// gönderim sınırlarının ayrı kartı yok; rollere göre ne görüldüğü.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import ts from 'typescript';
import { dataModule, mailSetupLibURL, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';

const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const icons = new Set(['AlertTriangle']);
for (const file of ['DomainMailManager', 'OutgoingMailboxLimits', 'WebmailAccess']) {
  for (const match of source(`components/${file}.tsx`).matchAll(/import\s*\{([^}]+)\}\s*from 'lucide-react'/g)) {
    for (const item of match[1].split(',')) if (item.trim()) icons.add(item.trim());
  }
}
const stub = dataModule(`
  import React from '${reactURL}';
  ${[...icons].map((name) => `export const ${name} = () => null;`).join('\n')}
  const i18n = { t: (key, vars) => (vars ? key + JSON.stringify(vars) : key), locale: 'en' };
  export const useI18n = () => i18n;
  export const useMailOutgoingWords = () => i18n;
  export const useNavigate = () => (to) => { globalThis.currentTest.navigated.push(to); return true; };
  export const useAuth = () => ({ role: globalThis.currentTest.role });
  export const Link = (props) => React.createElement('a', { href: props.to, 'data-webmail-action': props['data-webmail-action'] }, props.children);
  export const showToast = (...args) => globalThis.currentTest.toasts.push(args);
  export const MailAuthPanel = () => null;
  export const MailSettingsPanel = () => null;
  export const rblSentence = () => null;
  export const MailAutoresponderDialog = () => null;
  export const runBulk = async () => ({ done: [], refused: [], unknown: null, notStarted: [] });
`);
const shared = sharedLayer(stub);
const webmailURL = shared.compile('components/WebmailAccess.tsx');
const limitsURL = shared.compile('components/OutgoingMailboxLimits.tsx');
const none = 'data:text/javascript,export default () => null';
const managerText = source('components/DomainMailManager.tsx')
  .replace("import('./WebmailAccess')", `import('${webmailURL}')`)
  .replaceAll("import('./OutgoingMailboxLimits')", `import('${limitsURL}')`)
  .replace("import('./MailDeliveryLookup')", `import('${none}')`)
  .replace("import('./DomainMailOptions')", `import('${none}')`)
  .replace("import('./MailboxRecoveryDialog')", `import('${none}')`);
const compiled = ts.transpileModule(managerText, {
  compilerOptions: { jsx: ts.JsxEmit.React, module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 },
}).outputText.replace(/from ['"]([^'"]+)['"]/g, (_, specifier) => `from '${specifier === 'react' ? reactURL : shared.resolve(specifier)}'`);
const Inner = (await import(dataModule(`import React from '${reactURL}';\n${compiled}`))).DomainMailManager;
const DomainMailManager = (props) => React.createElement(React.Suspense, { fallback: null }, React.createElement(Inner, props));
const { webmailSignInURL } = await import(mailSetupLibURL);

// --- The server's answers ---------------------------------------------------
const protocol = (port, security) => ({ host: 'mail.example.com', port, security });
const setup = (overrides = {}) => ({
  mail_host: 'mail.example.com', imap: protocol(993, 'SSL/TLS'), pop3: protocol(995, 'SSL/TLS'), smtp: protocol(587, 'STARTTLS'),
  username_is_full_email: true, webmail_available: true, webmail_url: 'https://mail.example.com/', ...overrides,
});
const accounts = [{ id: 5, address: 'info@example.com', quota_mb: 1024 }, { id: 6, address: 'a+b@example.com', quota_mb: 512 }];
const mailbox = (id, address, effective = {}, extra = {}) => ({
  id, address, effective: { limit: 500, mode: 'default', default: 500, ceiling: 800, ...effective }, used_hour: 12, sent_7d: 40, refused_7d: 0, ...extra,
});
const outgoing = (overrides = {}) => ({
  enforced: true, state: 'on', domain_limit: 750, domain_used_hour: 12, subscription_limit: 1000, subscription_used_hour: 30,
  domain_refused_7d: 0, subscription_refused_7d: 0, ceiling: 800, default: 500, may_exceed_ceiling: false,
  mailboxes: [mailbox(5, 'info@example.com'), mailbox(6, 'a+b@example.com', { limit: 50, new_until: '2026-11-15T09:00:00Z' })],
  ...overrides,
});
const off = () => outgoing({ enforced: false, state: 'off' });

const originalFetch = globalThis.fetch;
let tree;
let calls;
function serve({ role = 'customer', webmail = setup(), limits = off() } = {}) {
  calls = [];
  globalThis.currentTest = { toasts: [], navigated: [], role };
  globalThis.document = { addEventListener() {}, removeEventListener() {} };
  globalThis.window = { innerHeight: 800, innerWidth: 1200, addEventListener() {}, removeEventListener() {} };
  const table = {
    '/api/v1/domains/1/mail/setup': webmail,
    '/api/v1/domains/1/mail/outgoing': limits,
    '/api/v1/domains/1/mail/accounts': { accounts },
    '/api/v1/domains/1/mail/forwardings': { forwardings: [] },
    '/api/v1/domains/1/mail/quota': { plugin_enabled: true, usages: [] },
  };
  globalThis.fetch = (url, options = {}) => {
    const method = options.method || 'GET';
    calls.push({ method, url });
    const answer = table[url.split('?')[0]];
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
const text = () => JSON.stringify(tree.toJSON());
const words = (node) => (typeof node === 'string' ? node : (node?.children ?? []).map(words).join(' '));
const attr = (name, value) => tree.root.findAll((node) => typeof node.type === 'string' && node.props[name] !== undefined && (value === undefined || node.props[name] === value));
const rowActions = (address) => attr('data-row-actions', address)[0];
const visible = (address) => rowActions(address).findAll((node) => typeof node.type === 'string' && node.props['data-row-action'] !== undefined).map((node) => node.props['data-row-action']);
const press = async (node) => {
  await act(async () => {
    await node.props.onClick({ stopPropagation() {}, preventDefault() {} });
    for (let i = 0; i < 3; i++) await settled();
  });
};
const menu = async (address) => {
  const more = attr('data-row-more', address);
  if (more.length === 0) return [];
  await press(more[0]);
  const items = attr('data-row-menu', address)[0].findAll((node) => node.props.role === 'menuitem').map((node) => node.props['data-row-action']);
  await press(more[0]);
  return items;
};
const outgoingWords = () => /mout\./.test(text()) || attr('data-outgoing-line').length + attr('data-outgoing-state').length + attr('data-outgoing-near').length > 0;

test('one mailbox’s webmail link fills in its address, encoded, and carries nothing else', () => {
  assert.equal(webmailSignInURL('https://mail.example.com/', 'info@example.com'), 'https://mail.example.com/?_user=info%40example.com');
  assert.equal(webmailSignInURL('https://mail.example.com/', 'a+b@example.com'), 'https://mail.example.com/?_user=a%2Bb%40example.com');
  assert.equal(webmailSignInURL('https://mail.example.com/', 'ö&x=1@example.com'), 'https://mail.example.com/?_user=%C3%B6%26x%3D1%40example.com');
  for (const url of [null, '', 'http://mail.example.com/', 'https://mail.example.com/webmail/', 'https://mail.example.com/?x=1', 'javascript:alert(1)']) {
    assert.equal(webmailSignInURL(url, 'info@example.com'), null, String(url));
  }
  assert.equal(webmailSignInURL('https://mail.example.com/', ' '), null);
  // Never a password, a token or a sign-in in the link.
  assert.doesNotMatch(source('lib/mailSetup.ts').slice(source('lib/mailSetup.ts').indexOf('export function webmailSignInURL')), /_pass|token|_action=login/);
});

for (const role of ['admin', 'reseller', 'customer', 'additional_user']) {
  scenario(`${role}: webmail works → no card; each row opens its own mailbox; Password… beside it, the rest in More`, async () => {
    serve({ role });
    await mount();
    assert.equal(attr('data-webmail-access').length, 0, 'the general webmail card is drawn although webmail works');
    for (const [address, encoded] of [['info@example.com', 'info%40example.com'], ['a+b@example.com', 'a%2Bb%40example.com']]) {
      const link = rowActions(address).findAll((node) => node.type === 'a' && node.props['data-row-action'] === 'webmail');
      assert.equal(link.length, 1, address);
      assert.equal(link[0].props.href, `https://mail.example.com/?_user=${encoded}`);
      assert.equal(link[0].props.target, '_blank');
      assert.equal(link[0].props.rel, 'noopener noreferrer');
      assert.deepEqual(visible(address), ['webmail', 'password'], address);
      assert.deepEqual(await menu(address), ['recovery', 'autoreply', 'empty', 'delete'], `${address}: no limit action while limits are off`);
    }
  });
}

scenario('team member who only reads: the webmail link and nothing else', async () => {
  serve({ role: 'additional_user' });
  await mount({ readOnly: true });
  assert.deepEqual(visible('info@example.com'), ['webmail']);
  assert.equal(attr('data-row-more').length, 0);
});

for (const role of ['admin', 'customer']) {
  scenario(`${role}: webmail without an address → the card is the problem with its fix, and no row offers webmail`, async () => {
    serve({ role, webmail: setup({ webmail_available: false, webmail_url: '', webmail_reason: 'no_address' }) });
    await mount();
    assert.equal(attr('data-webmail-access').length, 1);
    assert.equal(attr('data-row-action', 'webmail').length, 0, 'a row offers webmail that cannot open');
    assert.deepEqual(visible('info@example.com'), ['password']);
    const links = attr('data-webmail-action');
    if (role === 'admin') {
      assert.ok(text().includes('adminNext.webmail.no_address'));
      assert.deepEqual(links.map((node) => node.props.href), ['/components/roundcube']);
    } else {
      assert.ok(text().includes('mail.webmail.tenant.no_address'));
      assert.equal(links.length, 0, 'a customer is offered an administrator’s page');
    }
  });
}

for (const role of ['customer', 'reseller', 'additional_user']) {
  scenario(`${role}: limits not applied → nothing about sending limits at all`, async () => {
    for (const limits of [off(), outgoing({ enforced: false, state: 'not_enforced' }), outgoing({ enforced: false, state: 'pending' })]) {
      serve({ role, limits });
      await mount();
      assert.ok(!outgoingWords(), `${role} ${limits.state}: ${text()}`);
      assert.ok(!(await menu('info@example.com')).includes('limit'));
      await cleanup();
    }
  });
}

scenario('administrator: limits off → one line with its button, once per page, not per mailbox', async () => {
  serve({ role: 'admin' });
  await mount();
  const line = attr('data-outgoing-state', 'off');
  assert.equal(line.length, 1);
  assert.ok(text().includes('mout.dom.offAdmin'));
  assert.equal(attr('data-outgoing-line').length, 0);
  const button = line[0].findAll((node) => node.type === 'button');
  assert.equal(button.length, 1);
  await press(button[0]);
  assert.deepEqual(globalThis.currentTest.navigated, ['/components/postfix']);
});

for (const role of ['admin', 'reseller', 'customer']) {
  scenario(`${role}: limits applied → one short line on each row and Sending limit… in its menu; no problem line while far from a limit`, async () => {
    serve({ role, limits: outgoing() });
    await mount();
    const lines = attr('data-outgoing-line');
    assert.deepEqual(lines.map((node) => node.props['data-outgoing-line']), ['info@example.com', 'a+b@example.com']);
    assert.match(words(lines[0]), /mout\.row\.limit\{"n":500\}.*mout\.row\.hour\{"n":12\}/);
    assert.match(words(lines[1]), /mout\.row\.new\{"n":50,"time":/);
    assert.equal(attr('data-outgoing-near').length + attr('data-outgoing-state').length, 0);
    assert.ok(!text().includes('mout.dom.title'), 'the old card is back');
    assert.deepEqual(await menu('info@example.com'), ['recovery', 'autoreply', 'limit', 'empty', 'delete']);
    // Sending limit… opens the limit's own window.
    await press(attr('data-row-more', 'info@example.com')[0]);
    await press(attr('data-row-menu', 'info@example.com')[0].findAll((node) => node.props['data-row-action'] === 'limit')[0]);
    assert.ok(text().includes('mout.dlg.title'));
    assert.ok(attr('data-form-save').length === 1 && attr('data-form-save')[0].props.disabled, 'Save is on before a change');
  });
}

for (const role of ['admin', 'customer']) {
  scenario(`${role}: near a domain limit and refusals in 7 days → one problem line with what to do`, async () => {
    serve({ role, limits: outgoing({ domain_used_hour: 640, subscription_used_hour: 700, domain_refused_7d: 3 }) });
    await mount();
    const near = attr('data-outgoing-near');
    assert.equal(near.length, 1);
    const said = words(near[0]);
    assert.match(said, /mout\.near\.domain\{"used":640,"limit":750\}/);
    assert.match(said, /mout\.near\.domainRefused\{"n":3\}/);
    assert.doesNotMatch(said, /mout\.near\.account/, 'the account limit is named at 70%');
    const buttons = near[0].findAll((node) => node.type === 'button');
    if (role === 'admin') {
      assert.match(said, /mout\.near\.nextAdmin/);
      assert.equal(buttons.length, 1);
      await press(buttons[0]);
      assert.deepEqual(globalThis.currentTest.navigated, ['/components/postfix#mail-plan-limits']);
    } else {
      assert.match(said, /mout\.near\.next$/);
      assert.equal(buttons.length, 0, 'a customer is offered the administrator’s page');
    }
  });
}

scenario('a stopped mailbox keeps its state and its way out; a stop by hand has no password way out', async () => {
  serve({ role: 'customer', limits: outgoing({ mailboxes: [
    mailbox(5, 'info@example.com', {}, { paused: { paused_at: '2026-11-14T08:00:00Z', reason: 'spike' } }),
    mailbox(6, 'a+b@example.com', {}, { paused: { paused_at: '2026-11-14T08:00:00Z', reason: 'manual' } }),
  ] }) });
  await mount();
  const [spike, manual] = attr('data-outgoing-paused');
  assert.equal(spike.props.role, 'alert');
  assert.match(words(spike), /mout\.dom\.paused/);
  assert.match(words(manual), /mout\.dom\.pausedManual\.customer/);
  assert.equal(attr('data-outgoing-resume', 'a+b@example.com').length, 0);
  await press(attr('data-outgoing-resume', 'info@example.com')[0]);
  assert.ok(text().includes('mail.passwordDialog.title'), 'the way out does not open the password window');
});

test('the shipped texts: every new key in English and Turkish with the same values', async () => {
  const pairs = [
    [(await import('../src/i18n/mailOutgoing/en.ts')).enMailOutgoing, (await import('../src/i18n/mailOutgoing/tr.ts')).trMailOutgoing, /^mout\.(row|near|dlg)\./],
    [(await import('../src/i18n/screens/mailDatabases/en.ts')).enMailDatabasesScreens, (await import('../src/i18n/screens/mailDatabases/tr.ts')).trMailDatabasesScreens, /^mail\.(row\.|leftHeld)/],
  ];
  const vars = (value) => [...value.matchAll(/\{(\w+)\}/g)].map((match) => match[1]).sort().join(',');
  for (const [en, tr, pattern] of pairs) {
    const keys = Object.keys(en).filter((key) => pattern.test(key));
    assert.ok(keys.length >= 8, String(pattern));
    for (const key of keys) {
      assert.ok(tr[key] && tr[key] !== en[key], key);
      assert.equal(vars(tr[key]), vars(en[key]), key);
    }
  }
  // The old card's words are gone with it.
  const en = (await import('../src/i18n/mailOutgoing/en.ts')).enMailOutgoing;
  for (const key of ['mout.dom.title', 'mout.dom.off', 'mout.dom.domainUse', 'mout.dom.subscriptionUse', 'mout.dom.change', 'mout.dom.used']) assert.ok(!(key in en), key);
});
