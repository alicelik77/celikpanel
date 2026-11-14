// The webmail card says why there is no webmail, who acts and the one next
// step (alpha.127; owner on Boston, 6 Nov 2026: "Webmail is not available on
// this server yet" said none of these). Mounted with the real shared layer and
// the real setup decoder: the administrator gets the action that fixes it (the
// Webmail setup's own install window, Roundcube's address, or Roundcube's page);
// a customer is told what is missing and that the provider acts, never an
// action of the administrator's.
// Web postası kartı: neden yok, kim yapar, tek sonraki adım. Yönetici eylemi
// alır; müşteri sağlayıcısına sorar.
import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, dataModule, reactURL, sharedLayer, webmailAddressLibURL } from './fixtures/shared-layer.mjs';
import { enMailDatabasesScreens as en } from '../src/i18n/screens/mailDatabases/en.ts';
import { trMailDatabasesScreens as tr } from '../src/i18n/screens/mailDatabases/tr.ts';
import { enAdminNextScreens as adminEn } from '../src/i18n/screens/adminNext/en.ts';
import { trAdminNextScreens as adminTr } from '../src/i18n/screens/adminNext/tr.ts';

const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null;
  const i18n = { t: (key) => key, locale: 'en' };
  export const useI18n = () => i18n;
  export const useNavigate = () => () => true;
  export const useAuth = () => ({ role: globalThis.webmailRole });
  export const Link = (props) => React.createElement('a', { href: props.to, 'data-webmail-action': props['data-webmail-action'] }, props.children);
`);
const shared = sharedLayer(stub);
const mailSetupURL = compileSource('lib/mailSetup.ts', (specifier) => (specifier.endsWith('/webmailAddress') ? webmailAddressLibURL : stub));
const WebmailAccess = (await import(shared.compile('components/WebmailAccess.tsx', { '/lib/mailSetup': mailSetupURL }))).default;

const protocol = (port, security) => ({ host: 'mail.example.com', port, security });
const setup = (overrides) => ({
  mail_host: 'mail.example.com', imap: protocol(993, 'SSL/TLS'), pop3: protocol(995, 'SSL/TLS'), smtp: protocol(587, 'STARTTLS'),
  username_is_full_email: true, webmail_available: false, webmail_url: '', ...overrides,
});

const originalFetch = globalThis.fetch;
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
async function render(role, body) {
  globalThis.webmailRole = role;
  const sent = [];
  globalThis.fetch = (url, options = {}) => {
    sent.push(options.method || 'GET');
    return Promise.resolve(Response.json(body));
  };
  let tree;
  await act(async () => {
    tree = Renderer.create(React.createElement(WebmailAccess, { domainId: 7 }));
    await settled();
    await settled();
  });
  const out = { text: JSON.stringify(tree.toJSON()), links: tree.root.findAll((node) => node.type === 'a').map((node) => node.props.href), sent };
  await act(async () => tree.unmount());
  globalThis.fetch = originalFetch;
  return out;
}

const ADMIN = {
  not_installed: ['adminNext.webmail.not_installed', 'adminNext.webmail.install', '/components?bundle=mail.webmail'],
  no_address: ['adminNext.webmail.no_address', 'mail.webmail.setAddress', '/components/roundcube'],
  not_answering: ['adminNext.webmail.not_answering', 'adminNext.webmail.openRoundcube', '/components/roundcube'],
};

test('the administrator sees the reason and the one action that fixes it', async () => {
  for (const [reason, [sentence, label, href]] of Object.entries(ADMIN)) {
    const shown = await render('admin', setup({ webmail_reason: reason }));
    assert.ok(shown.text.includes(sentence), `${reason}: ${shown.text}`);
    assert.ok(shown.text.includes(label), reason);
    assert.deepEqual(shown.links, [href], reason);
    assert.ok(!shown.text.includes('mail.webmail.tenant.'), `${reason}: the administrator is not told to ask`);
    // Reading the card never starts anything.
    assert.deepEqual(shown.sent, ['GET'], reason);
  }
});

test('a customer or reseller is told what is missing and that the provider acts, with no administrator action', async () => {
  for (const role of ['customer', 'reseller']) {
    for (const reason of ['not_installed', 'no_address', 'not_answering']) {
      const shown = await render(role, setup({ webmail_reason: reason }));
      assert.ok(shown.text.includes('mail.webmail.unavailable'), `${role} ${reason}`);
      assert.ok(shown.text.includes(`mail.webmail.tenant.${reason}`), `${role} ${reason}`);
      assert.deepEqual(shown.links, [], `${role} ${reason}: no link to the administrator's pages`);
    }
  }
});

test('an older Panel that names no reason still says who acts', async () => {
  const customer = await render('customer', setup({}));
  assert.ok(customer.text.includes('mail.webmail.tenant.other'));
  const admin = await render('admin', setup({ webmail_reason: 'something_new' }));
  assert.ok(admin.text.includes('mail.webmail.noAddressAdmin'));
  assert.deepEqual(admin.links, ['/components/roundcube']);
});

// alpha.135: each mailbox row opens webmail itself, so the card is only the
// problem with its fix and is absent when webmail works.
test('available webmail draws no card at all, for any role', async () => {
  for (const role of ['admin', 'reseller', 'customer', 'additional_user']) {
    const shown = await render(role, setup({ webmail_available: true, webmail_url: 'https://mail.example.com/' }));
    assert.equal(shown.text, 'null', `${role}: ${shown.text}`);
    assert.deepEqual(shown.links, [], role);
  }
});

test('every new sentence is in English and Turkish; the customer sentences name the provider, the administrator ones do not ask', () => {
  const keys = Object.keys(en).filter((key) => key.startsWith('mail.webmail.tenant.'));
  const adminKeys = Object.keys(adminEn).filter((key) => key.startsWith('adminNext.webmail.'));
  assert.equal(keys.length, 4);
  assert.equal(adminKeys.length, 5);
  for (const key of keys) assert.ok(tr[key] && tr[key] !== en[key], key);
  for (const key of adminKeys) assert.ok(adminTr[key] && adminTr[key] !== adminEn[key], key);
  for (const reason of ['not_installed', 'no_address', 'not_answering', 'other']) {
    assert.match(en[`mail.webmail.tenant.${reason}`], /Your provider/);
    assert.match(tr[`mail.webmail.tenant.${reason}`], /sağlayıcınız/i);
  }
  for (const reason of ['not_installed', 'no_address', 'not_answering']) {
    assert.doesNotMatch(adminEn[`adminNext.webmail.${reason}`], /provider|ask/i);
  }
});

test('“Install webmail” opens the Webmail setup’s own install window, once, and starts nothing by itself', async () => {
  const { readFileSync } = await import('node:fs');
  const list = readFileSync(new URL('../src/components/ServiceList.tsx', import.meta.url), 'utf8');
  const link = list.slice(list.indexOf('const bundleLinkRef'), list.indexOf('const confirmComponentAction'));
  assert.match(link, /params\.get\('bundle'\)/);
  // The same dialogue its chip opens; a setup that cannot start only opens its strip and says why.
  assert.match(link, /if \(bundle\.action_route === 'mail_profile'\) openMailBundle\(bundle\);\s*else openComponentBundle\(bundle\);/);
  assert.match(link, /bundle\.action_route !== 'mail_profile' \|\| dnsIdentityReady/);
  assert.match(link, /!mutationControlsDisabled && !componentOperation\.locked/);
  assert.match(link, /services\.installLinkBlocked/);
  // Used once and removed from the address.
  assert.match(link, /bundleLinkRef\.current = true;/);
  assert.match(link, /params\.delete\('bundle'\)/);
  assert.doesNotMatch(link, /fetch\(|startInstall|bundleSequence\.begin/);
});
