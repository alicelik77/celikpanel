// alpha.133: a blocklist that refused the question (Spamhaus answers
// 127.255.255.254 to a question that comes through a public DNS resolver)
// was shown to the owner as "this server's address is on a spam blocklist".
// Mounted with the real shared reader and the real copy: a refusal is said as
// "could not check" with its reason and next step, never as listed and never
// as a warning about the address; customers and resellers get the short
// version without the server's details; a listing names its list.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, dataModule, reactURL, sharedLayer, webmailAddressLibURL } from './fixtures/shared-layer.mjs';
import { enMailDatabasesScreens } from '../src/i18n/screens/mailDatabases/en.ts';
import { trMailDatabasesScreens } from '../src/i18n/screens/mailDatabases/tr.ts';
import { enDomainOps } from '../src/i18n/domainOps/en.ts';
import { trDomainOps } from '../src/i18n/domainOps/tr.ts';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const say = (key, vars) => {
  let text = enMailDatabasesScreens[key];
  assert.ok(text, `missing copy ${key}`);
  for (const [name, value] of Object.entries(vars ?? {})) text = text.replaceAll(`{${name}}`, String(value));
  return text;
};

test('Blocklist copy: every new line exists in both languages and carries its list names', () => {
  const keys = Object.keys(enMailDatabasesScreens).filter((key) => key.startsWith('mail.rbl.'));
  assert.deepEqual(keys.sort(), Object.keys(trMailDatabasesScreens).filter((key) => key.startsWith('mail.rbl.')).sort());
  for (const key of keys.filter((k) => k.startsWith('mail.rbl.say.') || /mail\.rbl\.short\.(listed|policy)$/.test(k))) {
    assert.match(enMailDatabasesScreens[key], /\{lists\}/, key);
    assert.match(trMailDatabasesScreens[key], /\{lists\}/, key);
  }
  // The short version for customers and resellers names no server detail.
  for (const key of ['mail.rbl.short.listed', 'mail.rbl.short.policy', 'mail.rbl.short.notChecked']) {
    for (const text of [enMailDatabasesScreens[key], trMailDatabasesScreens[key]]) {
      assert.doesNotMatch(text, /1\.1\.1\.1|8\.8\.8\.8|resolver|çözümleyici|DNS/, key);
    }
  }
  assert.equal(enMailDatabasesScreens['mail.rblFailed'], undefined, 'the unused key stays removed');
  for (const key of ['dhealth.mail.listed', 'dhealth.mail.listed_policy']) {
    assert.match(enDomainOps[key], /\{items\}/);
    assert.match(trDomainOps[key], /\{items\}/);
  }
});

test('The deliverability card says the blocklist line through the one sentence, never as a raw code', () => {
  const card = read('../src/components/DomainMailManager.tsx');
  assert.match(card, /rblSentence\(t, \{ status: rbl\.status, code: rbl\.detail, items: rbl\.items \}, admin\)/);
  assert.match(card, /c\.detail && c\.id !== 'rbl'/);
  assert.match(card, /<DeliverabilityCard domainId=\{domainId\} admin=\{role === 'admin'\} \/>/);
  assert.match(card, /<MailSettingsPanel [^>]*admin=\{role === 'admin'\}/);
});

const icons = new Set();
for (const match of read('../src/components/MailSettingsPanel.tsx').matchAll(/import\s*\{([^}]+)\}\s*from 'lucide-react'/g)) {
  for (const item of match[1].split(',')) if (item.trim()) icons.add(item.trim());
}
icons.add('AlertTriangle');
globalThis.a133Say = (key, vars) => (enMailDatabasesScreens[key] ? say(key, vars) : key);
const stub = dataModule(`
  import React from '${reactURL}';
  ${[...icons].map((name) => `export const ${name} = () => null;`).join('\n')}
  export const useI18n = () => ({ t: globalThis.a133Say, locale: 'en' });
  export const useNavigate = () => () => true;
  export const Link = (props) => React.createElement('a', { href: props.to }, props.children);
  export const showToast = () => {};
  export const MailAutoconfigNotice = () => null;
  export const MailHostProtection = () => null;
  export const createPortal = () => null;
  export const useNavigationBlocker = () => {};
`);
const shared = sharedLayer(stub);
const mailSetupURL = compileSource('lib/mailSetup.ts', (specifier) => (specifier.endsWith('/webmailAddress') ? webmailAddressLibURL : stub));
const currentURL = shared.compile('components/CurrentSettings.tsx');
const { MailSettingsPanel, rblSentence } = await import(shared.compile('components/MailSettingsPanel.tsx', {
  '/lib/mailSetup': mailSetupURL, '/CurrentSettings': currentURL,
}));

const protocol = (port, security) => ({ host: 'mail.example.com', port, security });
const originalFetch = globalThis.fetch;
let tree, calls;
function serve(rbl) {
  calls = [];
  globalThis.document = { addEventListener() {}, removeEventListener() {}, visibilityState: 'visible' };
  const table = {
    '/api/v1/domains/1/mail/setup': () => Response.json({
      mail_host: 'mail.example.com', imap: protocol(993, 'SSL/TLS'), pop3: protocol(995, 'SSL/TLS'), smtp: protocol(587, 'STARTTLS'),
      username_is_full_email: true, webmail_available: false, webmail_url: '',
    }),
    '/api/v1/domains/1/mail/catch-all': () => Response.json({ enabled: false, destination: '', version: 'ca1' }),
    '/api/v1/domains/1/mail/rbl': () => Response.json(rbl),
  };
  globalThis.fetch = (url, options = {}) => {
    calls.push({ method: options.method || 'GET', url });
    const answer = table[url.split('?')[0]];
    if (!answer) return Promise.resolve(Response.json({ error: 'no such route' }, { status: 404 }));
    return Promise.resolve().then(() => answer());
  };
}
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
async function checkNow(rbl, admin) {
  serve(rbl);
  await act(async () => {
    tree = Renderer.create(React.createElement(MailSettingsPanel, { domainId: 1, domainName: 'example.com', admin }));
    await settled();
  });
  const textOf = (n) => n.children.map((c) => (typeof c === 'string' ? c : textOf(c))).join('');
  const button = tree.root.findAll((node) => node.type === 'button').find((node) => textOf(node).includes(say('mail.rbl.check')));
  assert.ok(button, 'Check now');
  await act(async () => {
    button.props.onClick();
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
const text = () => JSON.stringify(tree.toJSON());
const node = (attribute) => tree.root.findAll((n) => typeof n.type === 'string' && n.props[attribute] !== undefined);

const list = (zone, name, outcome, extra = {}) => ({ zone, name, listed: outcome === 'listed', outcome, ...extra });
const refused = {
  ip: '203.0.113.7',
  results: [
    list('zen.spamhaus.org', 'Spamhaus', 'not_checked', { reason: 'public_resolver' }),
    list('bl.spamcop.net', 'SpamCop', 'not_listed'),
    list('b.barracudacentral.org', 'Barracuda', 'not_listed'),
  ],
  summary: { status: 'unknown', code: 'public_resolver', items: ['Spamhaus'] },
};

test('Check now: a refused question is "could not check" with the way out, never listed', async () => {
  try {
    await checkNow(refused, true);
    const page = text();
    assert.ok(!page.includes(say('mail.rbl.onList')), 'a refusal drawn as listed');
    assert.ok(!page.includes(say('mail.rbl.listed', { ip: '203.0.113.7', n: 1 })));
    assert.ok(!page.includes(say('mail.rbl.clean', { ip: '203.0.113.7' })), 'an incomplete check drawn as clean');
    assert.ok(page.includes(say('mail.rbl.incomplete', { ip: '203.0.113.7', n: 1 })));
    assert.ok(page.includes(say('mail.rbl.say.public_resolver', { lists: 'Spamhaus' })));
    assert.deepEqual(node('data-rbl-outcome').map((n) => n.props['data-rbl-outcome']), ['not_checked', 'not_listed', 'not_listed']);
    assert.equal(node('data-rbl-summary')[0].props['data-rbl-summary'], 'unknown');
    // Not drawn as a warning or a danger about the address.
    assert.doesNotMatch(page, /border-danger|bg-warning|role":"alert/);
    assert.ok(calls.every((call) => call.method === 'GET'), 'only reads');
  } finally {
    await cleanup();
  }
});

test('Check now: a customer gets the short version without server details', async () => {
  try {
    await checkNow(refused, false);
    const page = text();
    assert.ok(page.includes(say('mail.rbl.short.notChecked')));
    assert.ok(!page.includes('1.1.1.1'));
    assert.ok(!page.includes(say('mail.rbl.say.public_resolver', { lists: 'Spamhaus' })));
  } finally {
    await cleanup();
  }
});

test('Check now: a listing names its list; a policy entry says what it is', async () => {
  try {
    await checkNow({
      ip: '203.0.113.7',
      results: [
        list('zen.spamhaus.org', 'Spamhaus', 'listed', { sublists: ['pbl'], policy: true }),
        list('bl.spamcop.net', 'SpamCop', 'not_listed'),
        list('b.barracudacentral.org', 'Barracuda', 'not_checked', { reason: 'lookup_failed' }),
      ],
      summary: { status: 'fail', code: 'policy', items: ['Spamhaus (PBL)'] },
    }, true);
    const page = text();
    assert.ok(page.includes(say('mail.rbl.listed', { ip: '203.0.113.7', n: 1 })));
    assert.ok(page.includes(say('mail.rbl.say.policy', { lists: 'Spamhaus (PBL)' })));
    assert.ok(page.includes(' (PBL)'), 'the listing row names its part');
  } finally {
    await cleanup();
  }
});

test('Check now: an answer without the summary is not guessed at', async () => {
  try {
    await checkNow({ ip: '203.0.113.7', results: [{ zone: 'zen.spamhaus.org', listed: true }] }, true);
    const page = text();
    assert.ok(page.includes(say('mail.rbl.unknown')));
    assert.ok(!page.includes(say('mail.rbl.onList')));
  } finally {
    await cleanup();
  }
});

test('One sentence per answer for the card and the check', () => {
  const t = say;
  assert.equal(rblSentence(t, { status: 'ok', code: 'clean' }, true), null);
  assert.equal(rblSentence(t, { status: 'fail', code: 'listed', items: ['SpamCop', 'Barracuda'] }, true), say('mail.rbl.say.listed', { lists: 'SpamCop, Barracuda' }));
  assert.equal(rblSentence(t, { status: 'fail', code: 'listed', items: ['SpamCop'] }, false), say('mail.rbl.short.listed', { lists: 'SpamCop' }));
  assert.equal(rblSentence(t, { status: 'fail', code: 'policy', items: ['Spamhaus (PBL)'] }, false), say('mail.rbl.short.policy', { lists: 'Spamhaus (PBL)' }));
  for (const code of ['public_resolver', 'too_many_queries', 'unknown_answer', 'lookup_failed', 'older_agent']) {
    assert.equal(rblSentence(t, { status: 'unknown', code, items: ['Spamhaus'] }, true), say(`mail.rbl.say.${code}`, { lists: 'Spamhaus' }));
    assert.equal(rblSentence(t, { status: 'unknown', code, items: ['Spamhaus'] }, false), say('mail.rbl.short.notChecked'));
  }
  // The blocklist check itself could not run: said as that.
  assert.equal(rblSentence(t, { status: 'unknown' }, true), say('mail.rbl.unknown'));
});

test('Domain health: a listing names its lists as written; record names stay in capitals', async () => {
  const words = (key, vars) => {
    let text = enDomainOps[key];
    for (const [name, value] of Object.entries(vars ?? {})) text = text.replaceAll(`{${name}}`, String(value));
    return text;
  };
  globalThis.a133Words = words;
  const stripStub = dataModule(`
    import React from '${reactURL}';
    export const Activity = () => null, AlertTriangle = () => null;
    export const useI18n = () => ({ t: globalThis.a133Words, locale: 'en' });
    export const useNavigate = () => () => true;
    export const Link = (props) => React.createElement('a', { href: props.to }, props.children);
  `);
  const enURL = compileSource('i18n/domainOps/en.ts', () => stripStub);
  const trURL = compileSource('i18n/domainOps/tr.ts', () => stripStub);
  const wordsURL = compileSource('i18n/domainOps.ts', (specifier) => (
    specifier.endsWith('/en') ? enURL : specifier.endsWith('/tr') ? trURL : stripStub
  ));
  const connectionURL = dataModule('export function decodeConnection(raw) { return raw; }');
  const strip = sharedLayer(stripStub, { '/i18n/domainOps': wordsURL, '/DomainConnection': connectionURL });
  const { healthSentence } = await import(strip.compile('components/DomainHealthStrip.tsx'));
  assert.equal(
    healthSentence({ id: 'mail', state: 'problem', code: 'listed_policy', items: ['Spamhaus (PBL)'] }, 'administrator', words),
    words('dhealth.mail.listed_policy', { items: 'Spamhaus (PBL)' }),
  );
  assert.equal(
    healthSentence({ id: 'mail', state: 'problem', code: 'listed', items: ['SpamCop', 'Barracuda'] }, 'administrator', words),
    words('dhealth.mail.listed', { items: 'SpamCop, Barracuda' }),
  );
  assert.equal(
    healthSentence({ id: 'mail', state: 'problem', code: 'auth_missing', items: ['dkim'] }, 'administrator', words),
    words('dhealth.mail.auth_missing', { items: 'DKIM' }),
  );
});
