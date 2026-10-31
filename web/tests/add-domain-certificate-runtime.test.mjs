import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, requestIdentityURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { screenParts, screenPartExport } from './locale-catalogue.mjs';
import { en as enShell } from '../src/i18n/en.ts';
import { tr as trShell } from '../src/i18n/tr.ts';

// "Enable SSL" on Add domain used to do nothing: the site was recorded as
// having SSL while no certificate was ever requested. Now the dialog, once the
// site exists, sends ONE request to the route the SSL/TLS page uses and says
// what came of it (D-024): issued, failed with its reason and next step,
// waiting on the owner, or not known. The site stays created in every case,
// and nothing is requested twice - not by waiting, not by a lost answer.
//
// Mounted with the real shared layer (lib/remote.ts, lib/apiError.ts,
// lib/lostAnswer.ts, components/ui.tsx apart from the dialogue shell), the real
// lib/siteCertificateRequest.ts and the real catalogue in both languages.

const catalogue = { en: { ...enShell }, tr: { ...trShell } };
for (const part of screenParts) {
  for (const locale of ['en', 'tr']) {
    const module = await import(`../src/i18n/screens/${part}/${locale}.ts`);
    Object.assign(catalogue[locale], module[screenPartExport(part, locale)]);
  }
}

const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const icons = new Set(['AlertTriangle', 'XCircle']);
for (const file of ['components/AddDomainModal.tsx', 'components/ui.tsx']) {
  for (const match of source(file).matchAll(/import\s*\{([^}]+)\}\s*from 'lucide-react'/g)) {
    for (const item of match[1].split(',')) {
      const icon = item.replace(/\btype\b/, '').trim().split(/\s+as\s+/)[0];
      if (icon && icon !== 'LucideIcon') icons.add(icon);
    }
  }
}

const stub = dataModule(`
  import React from '${reactURL}';
  ${[...icons].map((name) => `export const ${name} = () => null;`).join('\n')}
  export const useI18n = () => globalThis.currentTest.i18n;
  export const useNavigate = () => (path) => { globalThis.currentTest.navigated.push(path); };
  export const useLocation = () => ({ pathname: '/domains', search: '', hash: '' });
  export const Link = ({ to, children, ...props }) => React.createElement('a', { ...props, href: to }, children);
  export const showToast = (...args) => globalThis.currentTest.toasts.push(args);
`);
const shared = sharedLayer(stub, { '/lib/requestIdentity': requestIdentityURL });
// The dialogue shell listens on `document`; this stand-in keeps its contract:
// a form with the body and the action row, busy and dismiss exposed.
const ui = dataModule(`
  import React from '${reactURL}';
  export * from '${shared.uiURL}';
  export const Dialog = (props) => React.createElement('form', { 'data-busy': props.busy ? 'yes' : 'no', 'data-dialog': props.id, onSubmit: props.onSubmit },
    React.createElement('h2', null, props.title), React.createElement('p', { 'data-description': 'yes' }, props.description), props.children, React.createElement('footer', null, props.actions));
`);
const { AddDomainModal } = await import(shared.compile('components/AddDomainModal.tsx', { '/ui': ui }));

const CAPABILITIES = {
  web_server: 'nginx', php_versions: [], dns_server: '', dns_identity_ready: false,
  dns_management_mode: 'external', dns_management_ready: true, mail_server: false, database_servers: [], db_tools: [],
};
const originalFetch = globalThis.fetch;
let tree;
let calls;
let succeeded;

function serve({ create, certificate, locale = 'en' }) {
  calls = [];
  succeeded = [];
  const t = (key, vars = {}) => {
    const text = key in catalogue[locale] ? catalogue[locale][key] : key;
    return text.replace(/\{(\w+)\}/g, (_, name) => String(vars[name] ?? `{${name}}`));
  };
  globalThis.currentTest = { toasts: [], navigated: [], i18n: { t, locale, screensReady: true, screensFailed: false } };
  globalThis.window = { location: { origin: 'http://panel.test', hostname: 'panel.test' } };
  globalThis.location = globalThis.window.location;
  globalThis.fetch = (url, options = {}) => {
    const method = options.method || 'GET';
    calls.push({ method, url, options });
    if (method === 'GET') return Promise.resolve(Response.json(CAPABILITIES));
    if (method === 'POST' && url === '/api/v1/domains/create') return Promise.resolve().then(() => create(options));
    if (method === 'POST' && /^\/api\/v1\/domains\/\d+\/ssl\/letsencrypt$/.test(url)) return Promise.resolve().then(() => certificate(options));
    return Promise.resolve(Response.json({ error: 'no such route in this test' }, { status: 404 }));
  };
}
const settled = () => new Promise((resolve) => setImmediate(resolve));
const flush = async () => { for (let i = 0; i < 6; i += 1) await settled(); };
async function mount() {
  await act(async () => {
    tree = Renderer.create(React.createElement(AddDomainModal, { onClose() {}, onSuccess: (outcome) => succeeded.push(outcome ?? null) }));
    await flush();
  });
}
async function cleanup() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  delete globalThis.currentTest;
  delete globalThis.window;
  delete globalThis.location;
}
const text = () => JSON.stringify(tree.toJSON());
const t = (key, locale = 'en') => catalogue[locale][key];
const labelOf = (node) => node.findAll(() => true).flatMap((child) => child.children ?? []).filter((part) => typeof part === 'string').join(' ');
const buttonNamed = (label) => tree.root.findAll((node) => node.type === 'button' && labelOf(node).includes(label));
const posts = (pattern) => calls.filter((call) => call.method === 'POST' && pattern.test(call.url));
async function fillAndSubmit({ ssl }) {
  await act(async () => {
    tree.root.find((node) => node.type === 'input' && node.props.placeholder === 'example.com').props.onChange({ target: { value: 'example.com' } });
    if (ssl) tree.root.find((node) => node.props.id === 'ssl').props.onChange({ target: { checked: true } });
    await flush();
  });
  await act(async () => {
    tree.root.find((node) => node.type === 'form').props.onSubmit({ preventDefault() {} });
    await flush();
  });
}
const created = () => Response.json({ DomainID: 7, SiteID: 3, Domain: 'example.com', ProjectType: 'static' });
const refusal = (status, body) => Response.json(body, { status });

test('SSL ticked: the site is created, then the certificate is requested once on the SSL page route', async () => {
  serve({ create: created, certificate: () => Response.json({ status: 'success', expires_at: '2027-01-01T00:00:00Z' }) });
  try {
    await mount();
    await fillAndSubmit({ ssl: true });
    const create = posts(/\/domains\/create$/);
    assert.equal(create.length, 1);
    assert.equal(JSON.parse(create[0].options.body).ssl_type, 'letsencrypt');
    const issue = posts(/ssl\/letsencrypt$/);
    assert.equal(issue.length, 1, 'exactly one certificate request');
    assert.equal(issue[0].url, '/api/v1/domains/7/ssl/letsencrypt');
    // The SSL page's defaults, and no address: the server uses the account's.
    assert.deepEqual(JSON.parse(issue[0].options.body), { auto_renew: true, include_mail: false, provider: 'letsencrypt', reissue: false, redirect_http: true });
    assert.equal(succeeded.length, 1);
    assert.ok(globalThis.currentTest.toasts.some(([tone, message]) => tone === 'success' && message === t('domains.add.sslIssued').replace('{name}', 'example.com')));
    assert.equal(globalThis.currentTest.navigated.length, 0);
  } finally { await cleanup(); }
});

test('SSL not ticked: no certificate request at all', async () => {
  serve({ create: created, certificate: () => { throw new Error('no certificate request expected'); } });
  try {
    await mount();
    await fillAndSubmit({ ssl: false });
    assert.equal(JSON.parse(posts(/\/domains\/create$/)[0].options.body).ssl_type, 'none');
    assert.equal(posts(/ssl\/letsencrypt$/).length, 0);
    assert.equal(succeeded.length, 1);
  } finally { await cleanup(); }
});

test('while the certificate is being requested the dialog says so, holds still, and cannot send Create again', async () => {
  let release;
  const gate = new Promise((resolve) => { release = resolve; });
  serve({ create: created, certificate: async () => { await gate; return Response.json({ status: 'success' }); } });
  try {
    await mount();
    await fillAndSubmit({ ssl: true });
    assert.ok(text().includes('Asking Let’s Encrypt for the certificate of example.com'));
    assert.ok(text().includes('You do not need to do anything'));
    assert.equal(tree.root.find((node) => node.type === 'form').props['data-busy'], 'yes');
    assert.equal(buttonNamed(t('domains.add.create')).length, 0, 'the Create button is gone');
    // The form handler is not reachable any more either.
    await act(async () => { tree.root.find((node) => node.type === 'form').props.onSubmit?.({ preventDefault() {} }); await flush(); });
    assert.equal(posts(/\/domains\/create$/).length, 1);
    assert.equal(posts(/ssl\/letsencrypt$/).length, 1);
    assert.equal(succeeded.length, 0);
    await act(async () => { release(); await flush(); });
    assert.equal(posts(/ssl\/letsencrypt$/).length, 1);
    assert.equal(succeeded.length, 1);
  } finally { await cleanup(); }
});

test('a verified issuance failure keeps the site, names the cause and the next step, and sends nothing again', async () => {
  serve({
    create: created,
    certificate: () => refusal(502, {
      error: 'certbot failed', code: 'CERTIFICATE_ISSUE_FAILED', reason: 'validation',
      vars: { domain: 'example.com', detail: 'Timeout during connect' },
    }),
  });
  try {
    await mount();
    await fillAndSubmit({ ssl: true });
    assert.equal(posts(/ssl\/letsencrypt$/).length, 1);
    // The site exists and the dialog says the certificate is what failed.
    assert.ok(text().includes('example.com was created, but its certificate was not issued'));
    assert.ok(text().includes('the certificate authority could not validate one of the names'));
    assert.ok(text().includes('Timeout during connect'));
    assert.ok(text().includes('request the certificate again under SSL/TLS'));
    // Nothing is closed or reported as done until the person acts.
    assert.equal(succeeded.length, 0);
    await act(async () => { await flush(); await flush(); });
    assert.equal(posts(/ssl\/letsencrypt$/).length, 1, 'waiting never requests again');
    assert.equal(posts(/\/domains\/create$/).length, 1);
    // Open SSL/TLS: the list is refreshed and the SSL page opens.
    await act(async () => { buttonNamed(t('domains.add.sslOpen'))[0].props.onClick(); await flush(); });
    assert.equal(succeeded.length, 1);
    assert.deepEqual(globalThis.currentTest.navigated, ['/domains/example.com?tab=ssl']);
  } finally { await cleanup(); }
});

test('every issuance failure kind has its sentence, in both languages', async () => {
  for (const locale of ['en', 'tr']) {
    for (const reason of ['authority_unreachable', 'validation', 'rate_limited', 'timeout', 'tool', 'something_new']) {
      serve({
        locale, create: created,
        certificate: () => refusal(502, { error: 'x', code: 'CERTIFICATE_ISSUE_FAILED', reason, vars: { domain: 'example.com' } }),
      });
      try {
        await mount();
        await fillAndSubmit({ ssl: true });
        const key = reason === 'something_new' ? 'tool' : reason;
        const sentence = t(`ssl.issueFailure.${key}`, locale).replace('{domain}', 'example.com');
        assert.ok(text().includes(JSON.stringify(sentence).slice(1, -1)), `${locale}/${reason}`);
      } finally { await cleanup(); }
    }
  }
});

test('a missing contact address is a refusal with the My account action; nothing was requested', async () => {
  serve({
    create: created,
    certificate: () => refusal(409, { error: 'no contact', code: 'ssl_contact_email_required', action: '/account#email' }),
  });
  try {
    await mount();
    await fillAndSubmit({ ssl: true });
    assert.ok(text().includes('example.com was created, but its certificate was not issued'));
    assert.ok(text().includes(JSON.stringify(t('err.ssl_contact_email_required')).slice(1, -1)));
    const action = buttonNamed(t('err.ssl_contact_email_required.action'));
    assert.equal(action.length, 1);
    assert.equal(posts(/ssl\/letsencrypt$/).length, 1);
  } finally { await cleanup(); }
});

test('issued but not yet served: the web server activation is pending, said with the next action', async () => {
  serve({ create: created, certificate: () => refusal(409, { error: 'x', code: 'ssl_activation_pending' }) });
  try {
    await mount();
    await fillAndSubmit({ ssl: true });
    assert.ok(text().includes('was issued, but the web server could not start using it yet'));
    assert.ok(text().includes('Retry activation'));
    assert.equal(succeeded.length, 0);
    assert.equal(posts(/ssl\/letsencrypt$/).length, 1);
  } finally { await cleanup(); }
});

test('issued and stored while the configuration file is the owner’s: waiting for the owner', async () => {
  serve({ create: created, certificate: () => Response.json({ status: 'waiting_for_owner', pending_reason: 'certificate' }) });
  try {
    await mount();
    await fillAndSubmit({ ssl: true });
    assert.ok(text().includes('the site’s configuration file is yours and does not use it yet'));
    assert.equal(succeeded.length, 0);
  } finally { await cleanup(); }
});

test('a lost answer is an unknown result: said as such, nothing is sent a second time', async () => {
  for (const lost of [
    () => { throw new TypeError('network'); },
    () => new Response('<html>bad gateway</html>', { status: 502, headers: { 'Content-Type': 'text/html' } }),
    () => refusal(409, { error: 'running', code: 'REQUEST_IN_PROGRESS' }),
    () => refusal(409, { error: 'restarted', code: 'REQUEST_OUTCOME_UNKNOWN' }),
  ]) {
    serve({ create: created, certificate: lost });
    try {
      await mount();
      await fillAndSubmit({ ssl: true });
      assert.ok(text().includes('it is not known whether a certificate was issued'));
      assert.ok(!text().includes('its certificate was not issued'), 'unknown is not reported as a failure');
      await act(async () => { await flush(); await flush(); });
      assert.equal(posts(/ssl\/letsencrypt$/).length, 1);
      assert.equal(posts(/\/domains\/create$/).length, 1);
      assert.equal(succeeded.length, 0);
    } finally { await cleanup(); }
  }
});

test('Done closes without opening the SSL page and still refreshes the list once', async () => {
  serve({ create: created, certificate: () => refusal(502, { error: 'x', code: 'CERTIFICATE_ISSUE_FAILED', reason: 'timeout' }) });
  try {
    await mount();
    await fillAndSubmit({ ssl: true });
    await act(async () => { buttonNamed(t('domains.add.sslDone'))[0].props.onClick(); await flush(); });
    assert.equal(succeeded.length, 1);
    assert.deepEqual(globalThis.currentTest.navigated, []);
  } finally { await cleanup(); }
});

test('DNS setup unfinished (partial success): the certificate is not requested and the person is told', async () => {
  serve({
    create: () => refusal(409, {
      error: 'dns', code: 'DNS_PUBLICATION_PENDING', partial_success: true, domain_id: 7, domain: 'example.com', zone_exists: false,
      action: '/domains/example.com?tab=dns',
    }),
    certificate: () => { throw new Error('no certificate request expected'); },
  });
  try {
    await mount();
    await fillAndSubmit({ ssl: true });
    assert.equal(posts(/ssl\/letsencrypt$/).length, 0);
    const messages = globalThis.currentTest.toasts.map(([, message]) => message);
    assert.ok(messages.includes(t('domains.add.sslNotRequested').replace('{name}', 'example.com')));
  } finally { await cleanup(); }
});

test('the checkbox text says what will happen, and every new sentence exists in English and Turkish', () => {
  const keys = Object.keys(catalogue.en).filter((key) => key.startsWith('domains.add.ssl'));
  assert.ok(keys.length >= 14);
  for (const key of [...keys, 'err.ssl_contact_email_required', 'err.ssl_contact_email_required.action']) {
    assert.ok(catalogue.tr[key], `tr ${key}`);
    assert.ok(catalogue.en[key], `en ${key}`);
  }
  assert.match(catalogue.en['domains.add.sslHint'], /right after the site is created/);
  assert.match(catalogue.en['domains.add.sslHint'], /request the certificate later/);
});
