import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, dataModule, reactURL, requestIdentityURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { screenParts, screenPartExport } from './locale-catalogue.mjs';
import { en as enShell } from '../src/i18n/en.ts';
import { tr as trShell } from '../src/i18n/tr.ts';

// v0.1.0-alpha.99 (D-037 amendment): My account → My email, mounted with the
// real shared layer (lib/remote.ts, lib/apiError.ts, components/ui.tsx), the
// real request identity (D-029) and the real catalogue in both languages.
// The card reads the address from the server and offers the change only on a
// known answer; the dialogue sends one identity per press, visibly waits
// (D-033), says when the result is not known, and shows every refusal in its
// coded words (D-024). The panel certificate's and the setup plan's contact
// refusal lead to /account#email.
//
// Hesabım → E-posta adresim, gerçek paylaşılan katman, gerçek istek kimliği ve
// iki dildeki gerçek katalogla bağlanır.

const catalogue = { en: { ...enShell }, tr: { ...trShell } };
for (const part of screenParts) {
  for (const locale of ['en', 'tr']) {
    const module = await import(`../src/i18n/screens/${part}/${locale}.ts`);
    Object.assign(catalogue[locale], module[screenPartExport(part, locale)]);
  }
}

const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const icons = new Set(['AlertTriangle', 'XCircle']);
for (const file of ['components/AccountPage.tsx', 'components/ChangeEmailModal.tsx', 'components/ui.tsx']) {
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
  export const useLocation = () => ({ pathname: '/account', search: '', hash: globalThis.currentTest.hash ?? '' });
  export const Link = ({ to, children, ...props }) => React.createElement('a', { ...props, href: to }, children);
  export const showToast = (...args) => globalThis.currentTest.toasts.push(args);
  export const PageHeader = (props) => React.createElement('header', null, props.title, props.subtitle);
  export const ChangePasswordModal = () => null;
`);
const shared = sharedLayer(stub, { '/lib/requestIdentity': requestIdentityURL });
const modalURL = shared.compile('components/ChangeEmailModal.tsx');
const { OWN_EMAIL_WAIT_MS } = await import(modalURL);
const { AccountPage } = await import(shared.compile('components/AccountPage.tsx', { '/ChangeEmailModal': modalURL }));
const { ErrorBanner } = await import(shared.uiURL);

const settled = () => new Promise((resolve) => setImmediate(resolve));
const flush = async () => { for (let i = 0; i < 4; i += 1) await settled(); };
let tree;
let calls;

// `me` answers each read of /api/v1/auth/me in turn (the last one repeats);
// `change` answers the PUT.
function serve({ me = [() => Response.json({ email: 'owner@example.com' })], change, locale = 'en', hash = '' } = {}) {
  calls = [];
  const t = (key) => (key in catalogue[locale] ? catalogue[locale][key] : key);
  globalThis.currentTest = { toasts: [], navigated: [], hash, i18n: { t, locale, screensReady: true, screensFailed: false } };
  const scrolled = [];
  globalThis.currentTest.scrolled = scrolled;
  globalThis.document = {
    addEventListener() {}, removeEventListener() {},
    getElementById: (id) => ({ scrollIntoView: (options) => scrolled.push([id, options]) }),
  };
  globalThis.window = { location: { origin: 'http://panel.test', hostname: 'panel.test' } };
  globalThis.location = globalThis.window.location;
  let reads = 0;
  globalThis.fetch = (url, options = {}) => {
    const method = options.method || 'GET';
    calls.push({ method, url, options });
    if (method === 'GET' && url === '/api/v1/auth/me') {
      const answer = me[Math.min(reads, me.length - 1)];
      reads += 1;
      return Promise.resolve().then(() => answer());
    }
    if (method === 'GET' && url === '/api/v1/auth/2fa/status') return Promise.resolve(Response.json({ enabled: false }));
    if (method === 'PUT' && url === '/api/v1/auth/email') return Promise.resolve().then(() => change(options));
    return Promise.resolve(Response.json({ error: 'no such route in this test' }, { status: 404 }));
  };
}
const originalFetch = globalThis.fetch;
async function mount(Component = AccountPage, props = {}) {
  await act(async () => {
    tree = Renderer.create(React.createElement(Component, props));
    await flush();
  });
}
async function cleanup() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  delete globalThis.currentTest;
  delete globalThis.document;
  delete globalThis.window;
  delete globalThis.location;
}
const text = () => JSON.stringify(tree.toJSON());
const t = (key, locale = 'en') => catalogue[locale][key];
const labelOf = (node) => node.findAll((child) => typeof child === 'string' || true)
  .flatMap((child) => child.children ?? []).filter((part) => typeof part === 'string').join(' ');
const buttonNamed = (label) => tree.root.findAll((node) => node.type === 'button' && labelOf(node).includes(label));
const card = () => tree.root.find((node) => node.props.id === 'email');
const dialog = () => tree.root.findAll((node) => node.props.role === 'dialog');
const field = (name) => tree.root.find((node) => node.type === 'input' && node.props.name === name);
const type = async (name, value) => act(async () => { field(name).props.onChange({ target: { value } }); await flush(); });
const submit = async () => act(async () => {
  tree.root.find((node) => node.type === 'form').props.onSubmit({ preventDefault() {} });
  await flush();
});
const here = (key) => globalThis.currentTest.i18n.t(key);
const openDialog = async () => act(async () => { buttonNamed(here('account.email.change'))[0].props.onClick({}); await flush(); });
const puts = () => calls.filter((call) => call.method === 'PUT');

test('My email: the card says it is reading, then shows the address and offers the change only on a known answer', async () => {
  let release;
  serve({ me: [() => new Promise((resolve) => { release = () => resolve(Response.json({ email: 'owner@example.com' })); })] });
  try {
    await mount();
    assert.equal(card().props.className.includes('scroll-mt-24'), true);
    assert.ok(text().includes(t('account.email.checking')), 'the card does not say it is reading');
    assert.equal(buttonNamed(t('account.email.change'))[0].props.disabled, true, 'the change is offered before the address is known');
    await act(async () => { release(); await flush(); });
    assert.ok(text().includes('owner@example.com'));
    assert.equal(buttonNamed(t('account.email.change'))[0].props.disabled, false);
    // The card sits between Password and Two-step sign-in.
    const sections = tree.root.findAll((node) => node.type === 'section').map((node) => node.props.id ?? '');
    assert.deepEqual(sections.indexOf('email'), 1, 'the email card is not the second card');
  } finally { await cleanup(); }
});

test('My email: a failed read is "could not be read", never "no address", and nothing is offered', async () => {
  serve({ me: [() => Promise.reject(new TypeError('fetch failed')), () => Response.json({ email: 'owner@example.com' })] });
  try {
    await mount();
    assert.ok(text().includes(t('account.email.unknown')));
    assert.ok(!text().includes(t('account.email.none')));
    assert.equal(buttonNamed(t('account.email.change'))[0].props.disabled, true);
    // Retry on the card reads again; the known answer then offers the change.
    const retry = card().findAll((node) => node.type === 'button' && labelOf(node).includes(t('common.retry')));
    assert.equal(retry.length, 1, 'the card offers no Retry for its own read');
    await act(async () => { retry[0].props.onClick({}); await flush(); });
    assert.ok(text().includes('owner@example.com'));
    assert.equal(buttonNamed(t('account.email.change'))[0].props.disabled, false);
  } finally { await cleanup(); }
});

test('/account#email scrolls to the card', async () => {
  serve({ hash: '#email' });
  try {
    await mount();
    assert.deepEqual(globalThis.currentTest.scrolled[0], ['email', { block: 'start' }]);
  } finally { await cleanup(); }
});

test('The dialogue sends one identity per press, visibly waits with its controls held, then closes and the card shows the new address', async () => {
  let answer;
  serve({
    me: [() => Response.json({ email: 'owner@example.com' }), () => Response.json({ email: 'new@example.com' })],
    change: () => new Promise((resolve) => { answer = resolve; }),
  });
  try {
    await mount();
    await openDialog();
    assert.equal(dialog().length, 1);
    assert.ok(text().includes(t('account.email.dialogTitle')));
    assert.ok(text().includes('owner@example.com'), 'the dialogue does not say the current address');
    const save = () => buttonNamed(t('account.email.save'))[0];
    assert.equal(save().props.disabled, true, 'an empty form can be saved');
    await type('email', '  new@Example.com ');
    assert.equal(save().props.disabled, true, 'saving without the password is offered');
    await type('current-password', 'secret password');
    assert.equal(save().props.disabled, false);
    assert.equal(save().props.type, 'submit', 'Enter does not save');

    await submit();
    assert.equal(puts().length, 1);
    const sent = puts()[0].options;
    assert.match(new Headers(sent.headers).get('X-CelikPanel-Request-Id'), /^[0-9a-f]{32}$/);
    assert.deepEqual(JSON.parse(sent.body), { email: 'new@Example.com', current_password: 'secret password' });
    // Waiting (D-033): it says so, the save spins, nothing else can be pressed.
    assert.ok(text().includes(t('account.email.saving')));
    assert.equal(save().props['aria-busy'], true);
    assert.equal(save().props.disabled, true);
    assert.equal(field('email').props.disabled, true);
    assert.equal(field('current-password').props.disabled, true);
    assert.equal(buttonNamed(t('common.cancel'))[0].props.disabled, true);
    assert.equal(tree.root.find((node) => node.props.role === 'dialog').props['aria-busy'], true);
    // A second press while it saves sends nothing.
    await submit();
    assert.equal(puts().length, 1, 'a press while saving sent a second change');

    await act(async () => { answer(Response.json({ success: true })); await flush(); });
    assert.equal(dialog().length, 0, 'the dialogue stayed open after success');
    assert.deepEqual(globalThis.currentTest.toasts, [['success', t('account.email.changed')]]);
    assert.ok(text().includes('new@example.com'), 'the card did not read the new address');
  } finally { await cleanup(); }
});

const refusals = [
  ['account_email_invalid', 400],
  ['account_email_unchanged', 409],
  ['account_email_taken', 409],
  ['account_email_wrong_password', 403],
  ['account_email_too_many_attempts', 429],
  ['account_email_changed_elsewhere', 409],
  ['account_email_request_invalid', 400],
];

for (const locale of ['en', 'tr']) {
  test(`Every refusal is shown in the dialogue in its own words (${locale}), and the dialogue stays usable`, async () => {
    for (const [code, status] of refusals) {
      serve({ locale, change: () => Response.json({ error: 'server sentence', code }, { status }) });
      try {
        await mount();
        await openDialog();
        await type('email', 'other@example.com');
        await type('current-password', 'secret password');
        await submit();
        const alert = tree.root.find((node) => node.props.role === 'alert');
        assert.equal(alert.props['data-email-refusal'], code);
        const sentence = catalogue[locale][`err.${code}`];
        assert.ok(sentence, `no ${locale} words for ${code}`);
        assert.ok(JSON.stringify(alert.props.children ? tree.toJSON() : '').includes(sentence), `${code}: its words are not shown`);
        assert.ok(!text().includes('server sentence'), `${code}: the server's English sentence was shown instead`);
        assert.equal(buttonNamed(here('account.email.save'))[0].props.disabled, code === 'account_email_wrong_password', `${code}: the form is not usable again`);
        assert.equal(dialog().length, 1, `${code}: the dialogue closed`);
        assert.equal(globalThis.currentTest.toasts.length, 0, `${code}: a toast was shown`);
        // A wrong password is cleared; the address stays for the next try.
        assert.equal(field('current-password').props.value, code === 'account_email_wrong_password' ? '' : 'secret password', code);
        assert.equal(field('email').props.value, 'other@example.com');
        assert.equal(field('email').props.disabled, false);
      } finally { await cleanup(); }
    }
  });
}

test('A lost answer, the Panel\'s own "outcome unknown" and the bound all say the result is not known, and the card reads again', async (context) => {
  const cases = [
    ['a gateway page', () => new Response('<html>bad gateway</html>', { status: 502, headers: { 'Content-Type': 'text/html' } })],
    ['the Panel restarted', () => Response.json({ error: 'x', code: 'REQUEST_OUTCOME_UNKNOWN' }, { status: 409 })],
    ['still running', () => Response.json({ error: 'x', code: 'REQUEST_IN_PROGRESS' }, { status: 409 })],
    ['no connection', () => Promise.reject(new TypeError('fetch failed'))],
  ];
  for (const [name, change] of cases) {
    serve({ change });
    try {
      await mount();
      const readsBefore = calls.filter((call) => call.url === '/api/v1/auth/me').length;
      await openDialog();
      await type('email', 'other@example.com');
      await type('current-password', 'secret password');
      await submit();
      assert.ok(text().includes(t('account.email.outcomeUnknown')), `${name}: the unknown result is not said`);
      assert.equal(tree.root.findAll((node) => node.props.role === 'alert').length, 0, `${name}: called a failure`);
      assert.equal(dialog().length, 1);
      assert.ok(calls.filter((call) => call.url === '/api/v1/auth/me').length > readsBefore, `${name}: the address was not read again`);
      assert.equal(buttonNamed(t('account.email.save'))[0].props.disabled, false, `${name}: the dialogue stays held`);
    } finally { await cleanup(); }
  }

  // The bound (D-033): an answer that never comes is given up after the wait,
  // never waited on for ever.
  context.mock.timers.enable({ apis: ['setTimeout'] });
  serve({ change: (options) => new Promise((_resolve, reject) => {
    options.signal.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')));
  }) });
  try {
    await mount();
    await openDialog();
    await type('email', 'other@example.com');
    await type('current-password', 'secret password');
    await submit();
    assert.ok(text().includes(t('account.email.saving')));
    assert.equal(OWN_EMAIL_WAIT_MS, 20_000);
    await act(async () => { context.mock.timers.tick(OWN_EMAIL_WAIT_MS); await flush(); });
    assert.ok(text().includes(t('account.email.outcomeUnknown')), 'the bound did not end the wait');
    assert.ok(!text().includes(t('account.email.saving')));
  } finally {
    await cleanup();
    context.mock.timers.reset();
  }
});

test('The panel certificate\'s contact refusal leads to My account, in both languages', async () => {
  for (const locale of ['en', 'tr']) {
    serve({ locale });
    try {
      await mount(ErrorBanner, { error: { message: 'server sentence', code: 'panel_certificate_contact_email_invalid', action: '/account#email' } });
      assert.ok(text().includes(catalogue[locale]['err.panel_certificate_contact_email_invalid']));
      assert.ok(catalogue[locale]['err.panel_certificate_contact_email_invalid'].includes(locale === 'en' ? 'My account' : 'Hesabım'));
      const button = buttonNamed(catalogue[locale]['err.panel_certificate_contact_email_invalid.action'])[0];
      assert.ok(button, `${locale}: no button to My account`);
      await act(async () => { button.props.onClick(); await flush(); });
      assert.deepEqual(globalThis.currentTest.navigated, ['/account#email']);
    } finally { await cleanup(); }
  }
  // The setup plan's blocker carries the same link, and its words say where.
  const setup = source('components/ServerSetup.tsx');
  assert.ok(setup.includes(`code === 'panel_certificate_contact_email_invalid' && <>{' '}<Link to="/account#email"`));
  assert.ok(setup.includes(`{t('setup.blocker.contactAction')}`));
  assert.ok(catalogue.en['setup.blocker.contact'].includes('My account (top-right menu)'));
  assert.ok(catalogue.tr['setup.blocker.contact'].includes('Hesabım’da (sağ üst menü)'));
  // A refusal that carries an action stays on the panel certificate card as a banner.
  const settings = source('components/Settings.tsx');
  assert.ok(settings.includes('if (problem.code && problem.action) setRefusal(problem);'));
  assert.ok(settings.includes('<ErrorBanner className="mb-4" error={refusal} />'));
});

test('A lost answer on the email change is asked again once under the same identity (D-029)', async () => {
  const { isReaskRoute } = await import(requestIdentityURL);
  assert.equal(isReaskRoute('PUT', '/api/v1/auth/email'), true);
  assert.equal(isReaskRoute('POST', '/api/v1/auth/email'), false);
  assert.equal(isReaskRoute('PUT', '/api/v1/auth/password'), false);
});

// After an unknown result the next Save of the same address carries the same
// identity, so a change the first arrival did make is answered from it and
// never runs twice; a definite answer, or another address, starts a new one.
test('After an unknown result, Save again sends the same identity until a definite answer', async () => {
  const answers = [
    () => Promise.reject(new TypeError('fetch failed')),
    () => Promise.reject(new TypeError('fetch failed')),
    () => Response.json({ error: 'x', code: 'account_email_taken' }, { status: 409 }),
    () => Promise.reject(new TypeError('fetch failed')),
    () => Promise.reject(new TypeError('fetch failed')),
  ];
  let sent = 0;
  serve({ change: () => answers[sent++]() });
  try {
    await mount();
    await openDialog();
    await type('email', 'other@example.com');
    await type('current-password', 'secret password');
    await submit();
    await submit();
    await submit();
    await submit();
    await type('email', 'third@example.com');
    await submit();
    const ids = puts().map((call) => new Headers(call.options.headers).get('X-CelikPanel-Request-Id'));
    assert.equal(ids.length, 5);
    assert.equal(ids[1], ids[0], 'the Save after an unknown result did not carry the same identity');
    assert.equal(ids[2], ids[0], 'the identity changed before a definite answer');
    assert.notEqual(ids[3], ids[2], 'a definite refusal did not end the identity');
    assert.notEqual(ids[4], ids[3], 'another address kept the identity of the first');
  } finally { await cleanup(); }
});

// REQUEST_IN_PROGRESS keeps the identity (the first arrival will answer it);
// REQUEST_OUTCOME_UNKNOWN is final on the Panel for that identity, so the next
// Save is a new action with a new identity and never loops on "not known".
test('After the Panel\'s REQUEST_OUTCOME_UNKNOWN, the next Save carries a new identity', async () => {
  const answers = [
    () => Response.json({ error: 'x', code: 'REQUEST_IN_PROGRESS' }, { status: 409 }),
    () => Response.json({ error: 'x', code: 'REQUEST_OUTCOME_UNKNOWN' }, { status: 409 }),
    () => Response.json({ success: true }),
  ];
  let sent = 0;
  serve({ change: () => answers[sent++]() });
  try {
    await mount();
    await openDialog();
    await type('email', 'other@example.com');
    await type('current-password', 'secret password');
    await submit();
    assert.ok(text().includes(t('account.email.outcomeUnknown')));
    await submit();
    assert.ok(text().includes(t('account.email.outcomeUnknown')));
    await submit();
    const ids = puts().map((call) => new Headers(call.options.headers).get('X-CelikPanel-Request-Id'));
    assert.equal(ids.length, 3);
    assert.equal(ids[1], ids[0], 'REQUEST_IN_PROGRESS did not keep the identity');
    assert.notEqual(ids[2], ids[1], 'REQUEST_OUTCOME_UNKNOWN kept an identity the Panel will never run again');
    assert.equal(dialog().length, 0, 'the new action did not complete');
  } finally { await cleanup(); }
});
