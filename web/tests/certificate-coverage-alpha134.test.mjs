import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { englishScreens, turkishScreens } from './locale-catalogue.mjs';

// What a domain's certificate protects (alpha.134, docs/CERTIFICATE-COVERAGE.md).
// Mounted with the shipped shared layer: the list "What is protected" says
// each line from the server's answer, its one action runs the certificate
// screen's own request in the progress window, the request form includes the
// mail name by default only where it points here, and the mail screen says
// the same line under the server name.
// Sertifikanın neyi koruduğu: liste, tek eylem, varsayılan posta adı ve posta ekranı cümlesi.

const icons = new Set(['AlertTriangle', 'XCircle', 'CheckCircle', 'HelpCircle', 'ShieldAlert', 'ShieldCheck', 'ShieldQuestion']);
for (const name of ['DomainSSLSettings', 'domain/CertificateCoverage', 'domain/MailHostProtection']) {
  const text = readFileSync(new URL(`../src/components/${name}.tsx`, import.meta.url), 'utf8');
  for (const match of text.matchAll(/import\s*\{([^}]+)\}\s*from 'lucide-react'/g)) {
    for (const item of match[1].split(',')) {
      const icon = item.replace(/\btype\b/, '').trim().split(/\s+as\s+/)[0];
      if (icon) icons.add(icon);
    }
  }
}
const stub = dataModule(`
  import React from '${reactURL}';
  ${[...icons].map((name) => `export const ${name} = () => null;`).join('\n')}
  const t = (key, values) => (values ? key + JSON.stringify(values) : key);
  const i18n = { t, locale: 'en' };
  export const useI18n = () => i18n;
  export const useNavigate = () => (to) => { (globalThis.navigations ??= []).push(to); };
  export const showToast = (...args) => { (globalThis.toasts ??= []).push(args); };
  export const CertificateIssueNotice = () => null;
  export const isCertificateIssueFailure = () => false;
  export const CertificateReachNotice = () => null;
  let n = 0;
  export const newOperationID = () => 'op-' + (++n);
`);
const shared = sharedLayer(stub);
const own = {
  '/lib/certificateNames': compileSource('lib/certificateNames.ts', () => stub),
  '/lib/sslTier': compileSource('lib/sslTier.ts', () => stub),
  '/lib/sslRenewal': compileSource('lib/sslRenewal.ts', () => stub),
  '/lib/hstsRemaining': compileSource('lib/hstsRemaining.ts', () => stub),
};
own['/coverageLines'] = compileSource('components/domain/coverageLines.ts', (specifier) => (specifier.endsWith('/dateFormat') ? shared.resolve('/lib/dateFormat') : stub));
own['/CertificateCoverage'] = shared.compile('components/domain/CertificateCoverage.tsx', own);
const { DomainSSLSettings } = await import(shared.compile('components/DomainSSLSettings.tsx', own));
const { MailHostProtection } = await import(shared.compile('components/domain/MailHostProtection.tsx', own));
const lines = await import(own['/coverageLines']);

// --- The harness ------------------------------------------------------------------
let tree;
let sent = [];
let reads = [];
const originalFetch = globalThis.fetch;
const settle = async () => { await act(async () => { for (let i = 0; i < 4; i++) await new Promise((resolve) => setTimeout(resolve, 5)); }); };
const json = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
function serve(routes) {
  sent = [];
  reads = [];
  globalThis.toasts = [];
  globalThis.navigations = [];
  globalThis.operationSteps = [];
  globalThis.confirm = () => true;
  globalThis.fetch = async (input, init = {}) => {
    const method = init.method || 'GET';
    const path = String(input).split('?')[0];
    if (method === 'GET') reads.push(path);
    else sent.push({ method, path, body: typeof init.body === 'string' ? JSON.parse(init.body) : undefined });
    const route = routes[`${method} ${path}`] ?? routes[path];
    if (!route) return json({ error: 'not served' }, 404);
    const answer = await route(init);
    return answer instanceof Response ? answer : json(answer);
  };
}
async function mount(element) {
  globalThis.document ??= { addEventListener() {}, removeEventListener() {} };
  await act(async () => { tree = Renderer.create(element); });
  await settle();
}
async function unmount() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
}
const words = (node) => (typeof node === 'string' ? node : !node ? '' : Array.isArray(node) ? node.map(words).join(' ') : words(node.children));
const text = () => words(tree.toJSON());
const nodeText = (node) => (typeof node === 'string' ? node : (node.children || []).map(nodeText).join(''));
const button = (key) => tree.root.findAll((node) => node.type === 'button').find((node) => nodeText(node).includes(key));
const press = async (node) => { assert.ok(node, 'no such button'); assert.ok(!node.props.disabled, 'the button is off'); await act(async () => { node.props.onClick({ preventDefault() {} }); }); await settle(); };
const lineOf = (kind, name) => tree.root.findAll((node) => node.type === 'li' && node.props['data-coverage-line'] === kind)
  .find((node) => nodeText(node).includes(name));

// --- The server -------------------------------------------------------------------
const certificate = {
  id: 1, type: 'letsencrypt', provider_id: 'letsencrypt', issuer: "Let's Encrypt", subject: 'example.com', issued_at: '2026-11-01T00:00:00Z', expires_at: '2027-01-30T00:00:00Z',
  days_until_expiry: 60, auto_renew: true, renewal_status: 'ok', status: 'valid', dns_names: ['example.com', 'www.example.com', 'autoconfig.example.com', 'autodiscover.example.com'],
  activated: true, usable: true, trust_status: 'trusted', activation_pending: false, dependents_pending: false,
};
const protectedLine = (kind, name, by = 'domain_certificate', extra = {}) => ({ kind, name, state: 'protected', by, until: '2027-01-30T00:00:00Z', ...extra });
// The owner's screen of 12 Nov: the site is covered, mail programs are told
// the server's name, and mail.example.com points here but is not in the certificate.
const ownersCase = {
  mail_name: 'mail.example.com',
  lines: [
    protectedLine('site', 'example.com', 'domain_certificate', { role: 'domain' }),
    protectedLine('site', 'www.example.com', 'domain_certificate', { role: 'www' }),
    protectedLine('mail_programs', 'mail.server.example', 'server_mail_certificate'),
    { kind: 'mail_name', name: 'mail.example.com', state: 'not_protected', reason: 'not_in_certificate', action: 'cover_mail' },
    protectedLine('mail_setup', 'autoconfig.example.com'),
    protectedLine('mail_setup', 'autodiscover.example.com'),
  ],
};
function sslServer({ hasCertificate = true, coverage = ownersCase, issue = () => ({ status: 'success', mail_name: 'mail.example.com', mail_served: true }), reach } = {}) {
  const state = { secure_mail: false, coverage };
  serve({
    'GET /api/v1/domains/1/ssl': () => ({
      domain_id: 1, domain_name: 'example.com', has_certificate: hasCertificate, managed_names: ['example.com', 'www.example.com'],
      ...(hasCertificate ? { certificate } : {}),
      settings: { force_https: false, hsts_enabled: false, hsts_max_age: 300 },
    }),
    'GET /api/v1/domains/1/ssl/mail': () => ({ secure_mail: state.secure_mail }),
    'GET /api/v1/domains/1/ssl/coverage': () => (state.coverage instanceof Response ? state.coverage : state.coverage),
    'GET /api/v1/ssl/providers': () => ({ providers: [{ id: 'letsencrypt', name: "Let's Encrypt", note: '', needs_eab: false }] }),
    'GET /api/v1/domains/1/connection': () => reach ?? json({ error: 'not served' }, 404),
    'POST /api/v1/domains/1/ssl/letsencrypt': (init) => issue(JSON.parse(init.body)),
    'PUT /api/v1/domains/1/ssl/mail': (init) => { state.secure_mail = JSON.parse(init.body).secure_mail; return { success: true }; },
  });
  return state;
}
const ssl = (props = {}) => React.createElement(DomainSSLSettings, { domainId: 1, domainName: 'example.com', mailAvailable: true, ...props });

// --- The list ---------------------------------------------------------------------
test('the list says every line from the server, with its certificate and end, or its reason', async () => {
  sslServer();
  await mount(ssl());
  try {
    const page = text();
    assert.ok(page.includes('certcover.title'), 'the list is not on the certificate screen');
    assert.equal(tree.root.findAll((node) => node.type === 'li' && node.props['data-coverage-line'] !== undefined).length, 6);
    const mailPrograms = nodeText(lineOf('mail_programs', 'mail.server.example'));
    assert.ok(mailPrograms.includes('certcover.kind.mail_programs'));
    assert.ok(mailPrograms.includes('certcover.by.server_mail_certificate{"date":'), 'the server certificate and its end are not said');
    assert.ok(mailPrograms.includes('certcover.state.protected'));
    const mailName = nodeText(lineOf('mail_name', 'mail.example.com'));
    assert.ok(mailName.includes('certcover.state.not_protected'));
    assert.ok(mailName.includes('certcover.reason.not_in_certificate'));
    assert.ok(mailName.includes('certcover.action.cover_mail'), 'the one action is not offered');
    // A protected line has no action.
    assert.ok(!nodeText(lineOf('site', 'www.example.com')).includes('certcover.action.'));
  } finally { await unmount(); }
});

test('names that are not protected come first, then the protected ones in the server\'s order; one list, nothing folded', async () => {
  const unknownLine = { kind: 'webmail', name: 'webmail.server.example', state: 'unknown', reason: 'not_checked' };
  const waitingLine = { kind: 'site', role: 'alias', name: 'shop.example.com', state: 'not_protected', reason: 'points_elsewhere', ips: ['198.51.100.7'], action: 'recheck' };
  // The server's order puts the gap fourth, the unknown line third and the
  // waiting alias last: on screen the problems lead, in that same order.
  sslServer({ coverage: { ...ownersCase, server_ip: '203.0.113.10', lines: [...ownersCase.lines.slice(0, 2), unknownLine, ...ownersCase.lines.slice(2), waitingLine] } });
  await mount(ssl());
  try {
    const shown = tree.root.findAll((node) => node.type === 'li' && node.props['data-coverage-line'] !== undefined)
      .map((node) => [node.props['data-coverage-state'], nodeText(node.find((part) => part.type === 'span' && String(part.props.className).includes('font-mono')))]);
    assert.deepEqual(shown, [
      ['unknown', 'webmail.server.example'],
      ['not_protected', 'mail.example.com'],
      ['not_protected', 'shop.example.com'],
      ['protected', 'example.com'],
      ['protected', 'www.example.com'],
      ['protected', 'mail.server.example'],
      ['protected', 'autoconfig.example.com'],
      ['protected', 'autodiscover.example.com'],
    ]);
    // The first line's one action is on screen without opening anything.
    assert.ok(button('certcover.action.cover_mail'), 'the gap\'s action is not shown');
    assert.equal(tree.root.findAll((node) => node.type === 'details' || node.props['aria-expanded'] !== undefined).length, 0, 'something in the list is folded');
  } finally { await unmount(); }
  // The helper keeps each group's order and changes nothing it is given.
  const given = ownersCase.lines.map((line) => lines.decodeCoverageLine(line));
  const ordered = lines.problemsFirst(given);
  assert.deepEqual(ordered.map((line) => line.name), ['mail.example.com', 'example.com', 'www.example.com', 'mail.server.example', 'autoconfig.example.com', 'autodiscover.example.com']);
  assert.deepEqual(given.map((line) => line.name), ownersCase.lines.map((line) => line.name));
});

test('the list could not be read: said as not known, never as unprotected, with a retry', async () => {
  const state = sslServer();
  state.coverage = json({ error: 'agent did not answer' }, 503);
  await mount(ssl());
  try {
    assert.ok(text().includes('certcover.unknown'));
    assert.ok(!text().includes('certcover.state.not_protected'));
  } finally { await unmount(); }
});

test('"Cover the mail name too": one request again in the progress window; the list is read again', async () => {
  const state = sslServer({ issue: (body) => { state.coverage = { ...ownersCase, lines: ownersCase.lines.map((line) => (line.kind === 'mail_name' ? protectedLine('mail_name', line.name) : line)) }; return { status: 'success', mail_name: 'mail.example.com', mail_served: true, body }; } });
  await mount(ssl());
  try {
    const before = reads.filter((path) => path.endsWith('/ssl/coverage')).length;
    await press(button('certcover.action.cover_mail'));
    assert.equal(sent.length, 1);
    assert.equal(sent[0].path, '/api/v1/domains/1/ssl/letsencrypt');
    // Same authority, same renewal choice, the account's own address, and
    // the mail name asked for by name: the server refuses before any
    // authority is asked if it no longer points here.
    assert.deepEqual(sent[0].body, { email: '', auto_renew: true, include_mail: true, provider: 'letsencrypt', eab_key_id: '', eab_hmac_key: '', reissue: true });
    assert.ok(globalThis.operationSteps.length > 0, 'the request did not run in the progress window');
    assert.ok(globalThis.toasts.some(([tone, message]) => tone === 'success' && message.startsWith('certcover.done.covered')));
    assert.ok(reads.filter((path) => path.endsWith('/ssl/coverage')).length > before, 'the list was not read again');
    assert.ok(nodeText(lineOf('mail_name', 'mail.example.com')).includes('certcover.state.protected'));
  } finally { await unmount(); }
});

test('mail could not take the new name: the certificate is in, mail is as it was, and the person is told', async () => {
  sslServer({ issue: () => ({ status: 'success', mail_name: 'mail.example.com', mail_served: false, mail_note: 'mail_not_switched' }) });
  await mount(ssl());
  try {
    await press(button('certcover.action.cover_mail'));
    assert.ok(!globalThis.toasts.some(([, message]) => message.startsWith('certcover.done.covered')), 'mail was said to use it');
    assert.ok(globalThis.toasts.some(([tone, message]) => tone === 'warning' && message.startsWith('certcover.note.mail_not_switched')));
  } finally { await unmount(); }
});

test('a refused request again changes nothing on screen and says why', async () => {
  sslServer({ issue: () => json({ error: 'the authority refused', code: 'CERTIFICATE_ISSUE_REFUSED' }, 409) });
  await mount(ssl());
  try {
    await press(button('certcover.action.cover_mail'));
    assert.ok(!globalThis.toasts.some(([tone]) => tone === 'success'));
    assert.ok(text().includes('ssl.reissueFailed'), 'the refusal is not said beside the certificate');
    assert.ok(nodeText(lineOf('mail_name', 'mail.example.com')).includes('certcover.state.not_protected'));
  } finally { await unmount(); }
});

test('"Cover the mail name too" for a name that moved away: refused before any authority, and the list is read again', async () => {
  const state = sslServer({ issue: () => {
    state.coverage = { ...ownersCase, server_ip: '192.0.2.10', lines: ownersCase.lines.map((line) => (line.kind === 'mail_name'
      ? { kind: 'mail_name', name: line.name, state: 'not_protected', reason: 'points_elsewhere', ips: ['198.51.100.7'], action: 'recheck' } : line)) };
    return json({ error: 'mail.example.com does not point to this server yet', code: 'CERTIFICATE_DOMAIN_NOT_POINTED', reason: 'mail_name', vars: { names: 'mail.example.com', ip: '192.0.2.10' } }, 409);
  } });
  await mount(ssl());
  try {
    const before = reads.filter((path) => path.endsWith('/ssl/coverage')).length;
    await press(button('certcover.action.cover_mail'));
    assert.equal(sent.length, 1);
    assert.ok(!globalThis.toasts.some(([tone]) => tone === 'success'));
    assert.ok(reads.filter((path) => path.endsWith('/ssl/coverage')).length > before, 'the list was not read again');
    const line = nodeText(lineOf('mail_name', 'mail.example.com'));
    assert.ok(line.includes('certcover.state.points_elsewhere'), 'the line does not say it waits for DNS');
    assert.ok(line.includes('certcover.action.recheck'));
  } finally { await unmount(); }
});

test('"Use it for mail too" asks mail to serve the certificate that already covers the name', async () => {
  sslServer({ coverage: { mail_name: 'mail.example.com', lines: [{ kind: 'mail_name', name: 'mail.example.com', state: 'not_protected', reason: 'mail_not_using', action: 'use_for_mail' }] } });
  await mount(ssl());
  try {
    await press(button('certcover.action.use_for_mail'));
    assert.deepEqual(sent.map((call) => `${call.method} ${call.path}`), ['PUT /api/v1/domains/1/ssl/mail']);
    assert.deepEqual(sent[0].body, { secure_mail: true });
    assert.ok(globalThis.toasts.some(([tone, message]) => tone === 'success' && message.startsWith('certcover.done.useForMail')));
  } finally { await unmount(); }
});

test('a name that points elsewhere opens the DNS records on this page; a view-only member gets no request', async () => {
  sslServer({ coverage: { mail_name: 'mail.example.com', server_ip: '192.0.2.10', lines: [
    { kind: 'mail_name', name: 'mail.example.com', state: 'not_protected', reason: 'points_elsewhere', ips: ['198.51.100.7'], action: 'dns' },
    { kind: 'site', role: 'alias', name: 'shop.example.net', state: 'not_protected', reason: 'not_in_certificate', action: 'reissue' },
  ] } });
  await mount(ssl({ readOnly: true }));
  try {
    const mailLine = nodeText(lineOf('mail_name', 'mail.example.com'));
    assert.ok(mailLine.includes('certcover.reason.points_elsewhere{"ips":"198.51.100.7"}'));
    // An unmet prerequisite: its own badge, never "Not protected", and the
    // next step names this server's records and the address.
    assert.ok(mailLine.includes('certcover.state.points_elsewhere'));
    assert.ok(!mailLine.includes('certcover.state.not_protected'));
    assert.ok(mailLine.includes('certcover.next.mail{"ip":"192.0.2.10","where":"certcover.where.here"}'), mailLine);
    await press(button('certcover.action.dns'));
    assert.deepEqual(globalThis.navigations, ['/domains/example.com?section=dns&part=records']);
    assert.equal(button('certcover.action.reissue'), undefined, 'a view-only member is offered a request');
    assert.equal(sent.length, 0);
  } finally { await unmount(); }
});

test('a name whose DNS is run elsewhere: who changes what, and "Check again" only reads', async () => {
  sslServer({ coverage: { mail_name: 'mail.example.com', server_ip: '', lines: [
    { kind: 'site', role: 'alias', name: 'shop.example.com', state: 'not_protected', reason: 'no_address', action: 'recheck' },
  ] } });
  await mount(ssl());
  try {
    const line = nodeText(lineOf('site', 'shop.example.com'));
    assert.ok(line.includes('certcover.state.no_address'));
    assert.ok(line.includes('certcover.next.site{"ip":"certcover.ip.unknown","where":"certcover.where.elsewhere"}'), line);
    const before = reads.filter((path) => path.endsWith('/ssl/coverage')).length;
    await press(button('certcover.action.recheck'));
    assert.equal(sent.length, 0, 'checking again sent a change');
    assert.ok(reads.filter((path) => path.endsWith('/ssl/coverage')).length > before, 'the public DNS was not asked again');
  } finally { await unmount(); }
});

// --- The request form ----------------------------------------------------------------
const reachWithMail = (state, ips = []) => ({
  server_ip: '192.0.2.10', nameservers: [], live_nameservers: [],
  certificate_names: [
    { name: 'example.com', kind: 'site', state: 'here', ips: ['192.0.2.10'] },
    { name: 'www.example.com', kind: 'site', state: 'here', ips: ['192.0.2.10'] },
    { name: 'mail.example.com', kind: 'mail', state, ips },
  ],
});
const mailTick = () => tree.root.findAll((node) => node.type === 'label' && node.props['data-ssl-mail-name'] !== undefined)[0]
  ?.findAll((node) => node.type === 'input')[0];
const typeEmail = async () => {
  const input = tree.root.findAll((node) => node.type === 'input' && node.props.type === 'email')[0];
  await act(async () => { input.props.onChange({ target: { value: 'admin@example.com' } }); });
};

test('the request form: the mail name pointing here is ticked by default; leaving it out is sent as such', async () => {
  sslServer({ hasCertificate: false, coverage: { mail_name: 'mail.example.com', lines: [] }, reach: reachWithMail('here', ['192.0.2.10']) });
  await mount(ssl());
  try {
    assert.equal(mailTick().props.checked, true, 'the mail name is not included by default');
    await typeEmail();
    await act(async () => { mailTick().props.onChange({ target: { checked: false } }); });
    await press(button('ssl.issue'));
    assert.equal(sent[0].body.exclude_mail, true);
    assert.equal('include_mail' in sent[0].body, false);
  } finally { await unmount(); }
});

test('the request form: a mail name that points elsewhere is not offered, with where it points', async () => {
  sslServer({ hasCertificate: false, coverage: { mail_name: 'mail.example.com', lines: [] }, reach: reachWithMail('elsewhere', ['198.51.100.7']) });
  await mount(ssl());
  try {
    assert.equal(mailTick(), undefined);
    assert.ok(text().includes('certcover.form.mailElsewhere{"name":"mail.example.com","ips":"198.51.100.7"}'));
    await typeEmail();
    await press(button('ssl.issue'));
    assert.equal('exclude_mail' in sent[0].body, false, 'the server decides; nothing to leave out');
  } finally { await unmount(); }
});

// --- The decoder and the shared words ---------------------------------------------------
test('a line is believed only when it is the contract', () => {
  assert.equal(lines.decodeCoverageLine({ kind: 'site', name: 'a', state: 'protected', by: 'domain_certificate' }).state, 'unknown', 'protected without an end');
  assert.equal(lines.decodeCoverageLine({ kind: 'site', name: 'a', state: 'not_protected' }).state, 'unknown', 'not protected without a reason');
  assert.equal(lines.decodeCoverageLine({ kind: 'site', name: 'a', state: 'not_protected', reason: 'something_new' }).state, 'unknown');
  assert.equal(lines.decodeCoverageLine({ kind: 'other', name: 'a', state: 'protected' }), null);
  assert.throws(() => lines.decodeCoverage({ lines: 'x' }));
});

test('the mail screen says the list\'s line under the server name, with the same certificate and end', async () => {
  const t = (key, values) => (values ? key + JSON.stringify(values) : key);
  serve({});
  await mount(React.createElement(MailHostProtection, { host: 'mail.server.example', domainName: 'example.com', protection: protectedLine('mail_programs', 'mail.server.example', 'server_mail_certificate') }));
  try {
    assert.ok(text().startsWith('mail.server.example') || text().includes('mail.server.example'));
    assert.ok(text().includes('certcover.mailHost.protected.server_mail_certificate{"date":'));
    assert.equal(button('certcover.mailHost.fix'), undefined);
  } finally { await unmount(); }
  serve({});
  await mount(React.createElement(MailHostProtection, { host: 'mail.example.com', domainName: 'example.com', protection: { kind: 'mail_programs', name: 'mail.example.com', state: 'not_protected', reason: 'certificate_not_usable' } }));
  try {
    assert.ok(text().includes('certcover.reason.certificate_not_usable'));
    await press(button('certcover.mailHost.fix'));
    assert.deepEqual(globalThis.navigations, ['/domains/example.com?section=security&part=certificate']);
  } finally { await unmount(); }
  assert.equal(lines.mailHostWords(t, 'en', null), 'certcover.mailHost.unknown');
});

test('every word of the list exists in English and Turkish, without jargon in Turkish', () => {
  const keys = [...readFileSync(new URL('../src/i18n/screens/certCoverage/en.ts', import.meta.url), 'utf8').matchAll(/'((?:certcover|opstep)\.[^']+)':/g)].map((match) => match[1]);
  assert.ok(keys.length > 40);
  for (const key of keys) {
    assert.ok(englishScreens.includes(`'${key}':`), key);
    assert.ok(turkishScreens.includes(`'${key}':`), key);
  }
  const turkish = readFileSync(new URL('../src/i18n/screens/certCoverage/tr.ts', import.meta.url), 'utf8');
  for (const word of ['SAN', 'SNI', 'ACME', 'http-01', 'TLS']) assert.ok(!new RegExp(`\\b${word}\\b`).test(turkish), `Turkish copy says ${word}`);
});
