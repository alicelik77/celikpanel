// alpha.110: webmail on its own address. Roundcube never runs on the panel's
// origin (a script in it could call the panel's API as the signed-in
// administrator), so every screen links to https://<host>/ and to nothing else.
// This file covers: the one strict link validator, decoders that fail closed,
// the address card mounted on the Roundcube Manage page, the confirmations
// (what will happen, in order; the certificate is kept; the old address goes
// only after the new one works), a DNS record added only after a confirmation,
// ONE POST per confirmed action with a request id and only reads afterwards,
// and the EN/TR words.
//
// alpha.110: web postası kendi adresinde. Her ekran yalnız https://<ad>/
// adresine bağlantı verir; çözücüler kapalı başarısız olur; kayıt ve kurulum
// yalnız onaydan sonra gider; her onay tek POST'tur.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, requestIdentityURL, roundcubeUpdateURL, sharedLayer, webmailAddressLibURL } from './fixtures/shared-layer.mjs';
import { enWebmailAddressScreens as enLifecycleScreens } from '../src/i18n/screens/webmailAddress/en.ts';
import { trWebmailAddressScreens as trLifecycleScreens } from '../src/i18n/screens/webmailAddress/tr.ts';

const read = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const lib = await import(webmailAddressLibURL);

// --- The one validator of a link ---------------------------------------------

test('isWebmailAddressURL accepts only https://<valid lower-case host name>/', () => {
  for (const good of ['https://mail.example.com/', 'https://webmail.celikhost.com/', 'https://a-b.c1.example.org/', 'https://xn--mail-fsa.example.com/']) {
    assert.equal(lib.isWebmailAddressURL(good), true, good);
  }
  for (const bad of [
    '/webmail/', 'http://mail.example.com/', 'https://mail.example.com', 'https://mail.example.com:8443/', 'https://user@mail.example.com/',
    'https://user:pw@mail.example.com/', 'https://mail.example.com/x', 'https://mail.example.com//', 'https://mail.example.com/?a=1', 'https://mail.example.com/#x',
    'javascript:alert(1)', 'data:text/html,x', '//mail.example.com/', 'https://Mail.Example.com/', 'https://mail/', 'https://203.0.113.5/', 'https://-a.example.com/',
    'https://a..example.com/', 'https://mail.example.com./', 'https:// mail.example.com/', '', null, undefined, 7, {}, ['https://mail.example.com/'],
  ]) {
    assert.equal(lib.isWebmailAddressURL(bad), false, String(bad));
  }
  assert.equal(lib.webmailLinkOrNull('https://mail.example.com/'), 'https://mail.example.com/');
  assert.equal(lib.webmailLinkOrNull('/webmail/'), null);
});

test('the decoders fail closed: anything malformed is unknown, never a link and never "no address"', () => {
  const good = { address: { host: 'mail.example.com', url: 'https://mail.example.com/', enabled_at: '2026-10-26T10:00:00Z' }, expected: ['203.0.113.5'], suggestion: 'mail.example.com', last_operation: null, status: { present: true, managed: true, proved: true, certificate_expires: '2027-01-24T00:00:00Z' } };
  const ok = lib.decodeWebmailAddress(good);
  assert.equal(ok.address.url, 'https://mail.example.com/');
  assert.equal(ok.status.proved, true);
  assert.equal(lib.decodeWebmailAddress({ address: null, expected: [] }).address, null);
  // The server leaves proved out when it did not answer: that is not proved.
  assert.equal(lib.decodeWebmailAddress({ ...good, status: { present: true, managed: true } }).status.proved, false);
  assert.equal(lib.decodeWebmailAddress({ ...good, status: undefined, status_unknown: true }).statusUnknown, true);
  assert.equal(lib.decodeWebmailAddress({ ...good, status: undefined }).statusUnknown, true, 'an address without its status is not "fine"');
  for (const bad of [
    null, 'x', [], {}, { expected: [] },
    { address: { host: 'mail.example.com', url: 'http://mail.example.com/' }, expected: [] },
    { address: { host: 'mail.example.com', url: 'https://other.example.com/' }, expected: [] },
    { address: { host: 'Mail', url: 'https://Mail/' }, expected: [] },
    { address: null, expected: 'x' },
    { address: null, expected: [], last_operation: { id: 'nope', status: 'running', kind: 'webmail_address' } },
  ]) {
    assert.throws(() => lib.decodeWebmailAddress(bad), undefined, JSON.stringify(bad));
  }
  const check = { host: 'mail.example.com', state: 'ready', ready: true, expected: ['203.0.113.5'], answered: ['203.0.113.5'], firewall: { state: 'open' } };
  assert.equal(lib.decodeWebmailAddressCheck({ check }).ready, true);
  for (const bad of [null, {}, { check: null }, { check: { ...check, state: 'weird' } }, { check: { ...check, ready: 'yes' } }, { check: { ...check, state: 'not_found', ready: true } }, { check: { ...check, expected: 'x' } }, { check: { ...check, records: [{ type: 'MX' }] } }]) {
    assert.equal(lib.decodeWebmailAddressCheck(bad), null, JSON.stringify(bad));
  }
  // A zone link that is not the panel's own record API is not followed.
  const withZone = (records_url) => lib.decodeWebmailAddressCheck({ check: { ...check, state: 'not_found', ready: false, zone: { name: 'example.com', records_url } } }).zone;
  assert.deepEqual(withZone('/api/v1/domains/7/dns/records'), { name: 'example.com', recordsURL: '/api/v1/domains/7/dns/records' });
  assert.deepEqual(withZone('/api/v1/dns/infrastructure-zones/celikhost.com/records')?.name, 'example.com');
  assert.equal(withZone('https://evil.example/api'), null);
  assert.equal(withZone('/api/v1/users'), null);
  const op = { id: 'a'.repeat(32), status: 'running', kind: 'webmail_address' };
  assert.ok(lib.decodeWebmailAddressOperation(op));
  for (const bad of [null, {}, { ...op, id: 'x' }, { ...op, status: 'weird' }, { ...op, kind: 'webmail_update' }, { ...op, kind: undefined }]) {
    assert.equal(lib.decodeWebmailAddressOperation(bad), null, JSON.stringify(bad));
  }
});

test('an operation result is read exactly: success only from a succeeded operation that says so', () => {
  const op = (extra) => ({ id: 'a'.repeat(32), kind: 'webmail_address', package_name: 'mail.example.com', status: 'succeeded', ...extra });
  assert.equal(lib.webmailAddressOutcome(op({ result: { success: true, outcome: 'enabled', host: 'mail.example.com', url: 'https://mail.example.com/' } })).outcome, 'enabled');
  assert.equal(lib.webmailAddressOutcome(op({ result: { outcome: 'enabled' } })).outcome, 'unknown', 'a success without success:true is not a success');
  assert.equal(lib.webmailAddressOutcome(op({ status: 'failed', result: { outcome: 'enabled', success: true } })).outcome, 'unknown');
  assert.equal(lib.webmailAddressOutcome(op({ status: 'failed', result: { outcome: 'not_enabled', code: 'NAME_IN_USE', reason: 'x' } })).code, 'NAME_IN_USE');
  assert.equal(lib.webmailAddressOutcome(op({ status: 'failed', result: { outcome: 'unknown' }, error: { code: 'webmail_address_interrupted' } })).outcome, 'interrupted');
  assert.equal(lib.webmailAddressOutcome(op({ status: 'failed', result: { success: false, outcome: 'enabled', url: 'http://x/' } })).url, '');
  assert.equal(lib.webmailAddressOutcome(op({ kind: 'webmail_address_remove', result: { success: true, outcome: 'removed', certificate_removed: true } })).certificateRemoved, true);
  assert.equal(lib.webmailAddressOutcome(op({ kind: 'webmail_address_remove', status: 'failed', result: { outcome: 'enabled' } })).outcome, 'unknown');
});

// --- The card, mounted --------------------------------------------------------

const en = { ...enLifecycleScreens, 'common.cancel': 'Cancel', 'common.retry': 'Try again', 'common.staleNotice': 'stale' };
const tr = { ...trLifecycleScreens, 'common.cancel': 'Vazgeç', 'common.retry': 'Tekrar dene', 'common.staleNotice': 'eski' };
const fill = (text, vars = {}) => Object.entries(vars).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
globalThis.cardWords = en;
const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null, CheckCircle2 = () => null, Info = () => null, XCircle = () => null, Globe = () => null, X = () => null, Loader2 = () => null;
  const fill = (text, vars) => Object.entries(vars || {}).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
  export const useI18n = () => ({ t: (key, vars) => (globalThis.cardWords?.[key] !== undefined ? fill(globalThis.cardWords[key], vars) : key), locale: 'en' });
  export const useNavigate = () => () => true;
  export const Link = (props) => React.createElement('a', { href: props.to, ...props }, props.children);
`);
const shared = sharedLayer(stub);
const textURL = shared.compile('lib/webmailAddressText.ts');
const flowURL = shared.compile('lib/webmailAddressFlow.ts', { '/requestIdentity': requestIdentityURL, '/roundcubeUpdate': roundcubeUpdateURL, '/webmailAddress': webmailAddressLibURL });
const { WebmailAddressCard } = await import(shared.compile('components/WebmailAddressCard.tsx', {
  '/LifeLine': shared.compile('components/LifeLine.tsx'),
  '/lib/webmailAddressText': textURL,
  '/lib/webmailAddressFlow': flowURL,
}));

const ADDRESS = '/api/v1/service/roundcube/address';
const originalFetch = globalThis.fetch;
let tree;
let requests = [];
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
async function settle(ms = 10) { await act(async () => { await new Promise((resolve) => setTimeout(resolve, ms)); }); }
const opID = 'c'.repeat(32);
const operation = (status, result = {}, extra = {}) => ({ id: opID, request_id: 'd'.repeat(32), kind: 'webmail_address', service_id: 'roundcube', package_name: 'mail.example.com', status, phase: 'enabling', started_at: '2026-10-26T10:00:00Z', result, ...extra });
const none = (extra = {}) => ({ address: null, suggestion: 'mail.example.com', expected: ['203.0.113.5'], last_operation: null, ...extra });
const enabled = (extra = {}, status = {}) => ({
  address: { host: 'mail.example.com', url: 'https://mail.example.com/', enabled_at: '2026-10-26T10:00:00Z' },
  suggestion: 'mail.example.com', expected: ['203.0.113.5'], last_operation: null,
  status: { present: true, managed: true, certificate_expires: '2027-01-24T00:00:00Z', proved: true, ...status }, ...extra,
});
const checkOf = (extra = {}) => ({
  host: 'mail.example.com', state: 'ready', message: 'Public DNS answers 203.0.113.5 for mail.example.com: this server.', expected: ['203.0.113.5'], answered: ['203.0.113.5'],
  firewall: { state: 'open' }, ready: true, ...extra,
});
const notFound = (extra = {}) => checkOf({
  state: 'not_found', code: 'no_record', ready: false, answered: [],
  records: [{ type: 'A', name: 'mail', content: '203.0.113.5', ttl: 3600, text: 'mail.example.com. 3600 IN A 203.0.113.5' }],
  owner_action: { who: 'administrator', next: 'Add the record' }, ...extra,
});

async function mount({ reads, checks = [], post, ops = [], records, words = en }) {
  globalThis.cardWords = words;
  globalThis.document = { addEventListener() {}, removeEventListener() {}, getElementById: () => null, activeElement: null, body: { style: {} } };
  requests = [];
  let readCount = 0;
  let checkCount = 0;
  globalThis.fetch = async (input, init = {}) => {
    const url = String(input);
    const method = init.method ?? 'GET';
    requests.push({ url, method, body: init.body });
    if (url === ADDRESS && method === 'POST') return post(init);
    if (url === ADDRESS) return answer(reads[Math.min(readCount++, reads.length - 1)]);
    if (url === ADDRESS + '/check') return answer({ check: checks[Math.min(checkCount++, checks.length - 1)] });
    if (url === ADDRESS + '/remove') return post(init);
    if (records && url === records.url) return records.answer(init);
    if (url.startsWith('/api/v1/service/operation?')) {
      const next = ops.length > 1 ? ops.shift() : ops[0];
      return next === undefined ? answer({ error: 'not found' }, 404) : answer({ operation: next });
    }
    return answer({ error: 'no route' }, 404);
  };
  await act(async () => { tree = Renderer.create(React.createElement(WebmailAddressCard)); });
  await settle();
}
async function unmount() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  delete globalThis.document;
}
const text = () => { const walk = (n) => (typeof n === 'string' ? n : Array.isArray(n) ? n.map(walk).join('') : (n?.children || []).map(walk).join('')); return walk(tree.toJSON()); };
const attr = (name) => tree.root.findAll((node) => node.props && node.props[name] !== undefined);
const posts = () => requests.filter((request) => request.method === 'POST');
async function typeHost(value) { await act(async () => { attr('data-webmail-host')[0].props.onChange({ target: { value } }); }); }
async function pressCheck() { await act(async () => { attr('data-webmail-address-form')[0].props.onSubmit({ preventDefault() {} }); }); await settle(); }
async function click(name) { await act(async () => { attr(name)[0].props.onClick(); }); await settle(); }

test('no address yet: the reason, the editable suggestion and nothing sent', async () => {
  await mount({ reads: [none()] });
  try {
    assert.ok(text().includes(en['life.addr.noneYet']));
    assert.ok(text().includes(en['life.addr.why']));
    assert.equal(attr('data-webmail-host')[0].props.value, 'mail.example.com', 'the host name is prefilled from the suggestion');
    assert.ok(text().includes('mail.example.com'));
    assert.equal(attr('data-webmail-address-link').length, 0);
    assert.equal(posts().length, 0, 'opening the card changes and checks nothing');
    await typeHost('webmail.celikhost.com');
    assert.equal(attr('data-webmail-host')[0].props.value, 'webmail.celikhost.com', 'it is editable');
  } finally { await unmount(); }
});

test('an unreadable address read is unknown, not "no address"', async () => {
  await mount({ reads: [{ address: 'nonsense' }] });
  try {
    assert.ok(text().includes(en['life.addr.unknown']));
    assert.ok(!text().includes(en['life.addr.noneYet']));
    assert.equal(attr('data-webmail-host').length, 0);
  } finally { await unmount(); }
});

test('Check only reads; a missing record is an unmet prerequisite shown with the exact record and who acts', async () => {
  await mount({ reads: [none()], checks: [notFound()] });
  try {
    await pressCheck();
    const check = posts().filter((request) => request.url === ADDRESS + '/check');
    assert.equal(check.length, 1);
    assert.deepEqual(JSON.parse(check[0].body), { host: 'mail.example.com' });
    assert.equal(posts().filter((request) => request.url === ADDRESS).length, 0, 'Check never starts the set-up');
    assert.ok(text().includes(en['life.addr.kind.unmet']));
    assert.ok(text().includes(fill(en['life.addr.msg.no_record'], { host: 'mail.example.com' })));
    assert.ok(text().includes(fill(en['life.addr.who'], { who: en['life.addr.who.administrator'] })));
    assert.ok(text().includes(en['life.addr.next'].replace('{next}', '') ) , 'the next action is said');
    assert.ok(text().includes(fill(en['life.addr.next.no_record.external'], {})));
    assert.equal(attr('data-webmail-records')[0].children.join(''), 'mail.example.com. 3600 IN A 203.0.113.5', 'the exact record, copyable');
    assert.ok(text().includes(en['life.addr.dnsNone']));
    assert.ok(text().includes(fill(en['life.addr.thisServer'], { expected: '203.0.113.5' })));
    assert.equal(attr('data-webmail-setup-offer').length, 0, 'not ready: no set-up button');
    assert.equal(attr('data-webmail-zone-offer').length, 0, 'the zone is not hosted here: no shortcut');
  } finally { await unmount(); }
});

test('the record is added to a zone this panel hosts only after a confirmation that lists it, then Check runs once', async () => {
  const zone = { name: 'example.com', records_url: '/api/v1/domains/7/dns/records' };
  await mount({ reads: [none()], checks: [notFound({ zone }), notFound({ zone })], records: { url: zone.records_url, answer: () => answer({ id: 1 }, 201) } });
  try {
    await pressCheck();
    const before = posts().filter((request) => request.url === zone.records_url).length;
    assert.equal(before, 0);
    await click('data-webmail-zone-offer');
    assert.equal(posts().filter((request) => request.url === zone.records_url).length, 0, 'opening the dialog adds nothing');
    assert.ok(text().includes(fill(en['life.addr.zoneDialogTitle'], { zone: 'example.com' })));
    assert.ok(text().includes('mail.example.com. 3600 IN A 203.0.113.5'), 'the dialog lists the record exactly');
    assert.ok(text().includes(en['life.addr.zoneDialogWait']), 'it says DNS needs a few minutes and Check can be pressed again');
    await click('data-webmail-zone-confirm');
    await settle(30);
    const added = posts().filter((request) => request.url === zone.records_url);
    assert.equal(added.length, 1);
    assert.deepEqual(JSON.parse(added[0].body), { name: 'mail', type: 'A', content: '203.0.113.5', ttl: 3600, prio: 0 });
    assert.equal(posts().filter((request) => request.url === ADDRESS + '/check').length, 2, 'one re-check after adding, never a loop');
    await settle(1700);
    assert.equal(posts().filter((request) => request.url === ADDRESS + '/check').length, 2, 'nothing polls the check');
    assert.ok(text().includes(fill(en['life.addr.added'], { zone: 'example.com' })));
  } finally { await unmount(); }
});

test('a refused record, and a lost answer for a record, say what is known and are never sent again', async () => {
  const zone = { name: 'example.com', records_url: '/api/v1/domains/7/dns/records' };
  await mount({ reads: [none()], checks: [notFound({ zone })], records: { url: zone.records_url, answer: () => answer({ error: 'record conflicts with a CNAME' }, 409) } });
  try {
    await pressCheck();
    await click('data-webmail-zone-offer');
    await click('data-webmail-zone-confirm');
    await settle(30);
    assert.equal(posts().filter((request) => request.url === zone.records_url).length, 1);
    assert.ok(text().includes('record conflicts with a CNAME'));
    assert.ok(text().includes(fill(en['life.addr.addFailed'], { record: 'mail.example.com. 3600 IN A 203.0.113.5', zone: 'example.com', reason: 'record conflicts with a CNAME' })));
  } finally { await unmount(); }
  await mount({ reads: [none()], checks: [notFound({ zone })], records: { url: zone.records_url, answer: () => { throw new TypeError('network'); } } });
  try {
    await pressCheck();
    await click('data-webmail-zone-offer');
    await click('data-webmail-zone-confirm');
    await settle(30);
    assert.equal(posts().filter((request) => request.url === zone.records_url).length, 1, 'not sent again');
    assert.ok(text().includes(fill(en['life.addr.addLost'], { record: 'mail.example.com. 3600 IN A 203.0.113.5' })));
  } finally { await unmount(); }
});

test('DNS pointing elsewhere, DNS that could not be asked and a closed firewall are three different things', async () => {
  await mount({ reads: [none()], checks: [checkOf({ state: 'elsewhere', code: 'points_elsewhere', ready: false, answered: ['198.51.100.9'], owner_action: { who: 'administrator', next: 'x' } })] });
  try {
    await pressCheck();
    assert.ok(text().includes(en['life.addr.kind.elsewhere']));
    assert.ok(text().includes(fill(en['life.addr.msg.points_elsewhere'], { answered: '198.51.100.9', host: 'mail.example.com', expected: '203.0.113.5' })));
    assert.ok(text().includes(fill(en['life.addr.dnsAnswered'], { answered: '198.51.100.9' })));
    assert.equal(attr('data-webmail-setup-offer').length, 0);
  } finally { await unmount(); }
  await mount({ reads: [none()], checks: [checkOf({ state: 'unknown', code: 'dns_unreachable', ready: false, answered: [], owner_action: { who: 'nobody yet', next: 'x' } })] });
  try {
    await pressCheck();
    assert.ok(text().includes(en['life.addr.kind.unknown']));
    assert.ok(text().includes(fill(en['life.addr.msg.dns_unreachable'], { host: 'mail.example.com' })));
    assert.ok(!text().includes(en['life.addr.dnsNone']), 'DNS was not asked, so it is not said to have answered nothing');
    assert.ok(text().includes(en['life.addr.next.dns_unreachable']));
    assert.equal(attr('data-webmail-setup-offer').length, 0);
  } finally { await unmount(); }
  await mount({ reads: [none()], checks: [checkOf({ ready: false, code: 'firewall_closed', firewall: { state: 'closed', closed: [80, 443] }, owner_action: { who: 'administrator', next: 'x' } })] });
  try {
    await pressCheck();
    assert.ok(text().includes(en['life.addr.kind.unmet']));
    assert.ok(text().includes(fill(en['life.addr.next.firewall_closed'], { ports: '80 / 443' })));
    assert.ok(text().includes('CelikPanel never opens ports by itself'));
    const link = attr('data-webmail-firewall-link')[0];
    assert.equal(link.props.to, '/security#firewall');
    assert.equal(attr('data-webmail-setup-offer').length, 0);
  } finally { await unmount(); }
});

test('Set up is offered only for a ready check; its confirmation says what happens in order; ONE POST with a request id; then only reads', async () => {
  await mount({
    reads: [none(), enabled()],
    checks: [checkOf({ current: 'old.example.com' })],
    post: () => answer({ operation: operation('queued') }, 202),
    ops: [operation('running'), operation('running'), operation('succeeded', { success: true, outcome: 'enabled', host: 'mail.example.com', url: 'https://mail.example.com/', certificate_expires: '2027-01-24T00:00:00Z', roundcube_settings: 'owner_edited', warning: 'ocsp stapling is off' })],
  });
  try {
    await pressCheck();
    assert.ok(text().includes(en['life.addr.kind.ready']));
    await click('data-webmail-setup-offer');
    assert.equal(posts().filter((request) => request.url === ADDRESS).length, 0, 'the offer button only opens the dialog');
    const dialog = text();
    for (const key of ['life.addr.setupIntro', 'life.addr.step.prove', 'life.addr.setupSafe']) assert.ok(dialog.includes(en[key]), key);
    assert.ok(dialog.includes(fill(en['life.addr.step.cert'], { host: 'mail.example.com' })));
    assert.ok(dialog.includes(fill(en['life.addr.step.nginx'], { host: 'mail.example.com' })));
    assert.ok(dialog.includes(fill(en['life.addr.step.replace'], { current: 'old.example.com' })), 'the old address goes only after the new one works');
    assert.ok(dialog.indexOf('Let’s Encrypt') < dialog.indexOf('web server configuration') && dialog.indexOf('web server configuration') < dialog.indexOf('Prove that webmail') && dialog.indexOf('Prove that webmail') < dialog.indexOf('Remove the old address'), 'in this order');
    assert.ok(dialog.includes('a certificate that was obtained is kept'));
    await click('data-webmail-setup-confirm');
    await settle(30);
    assert.ok(text().includes(fill(en['life.addr.working'], { host: 'mail.example.com' })), 'visible waiting says nothing needs to be done');
    assert.ok(en['life.addr.working'].includes('You don’t need to do anything'));
    await settle(1700);
    await settle(1700);
    await settle(1700);
    const sent = posts().filter((request) => request.url === ADDRESS);
    assert.equal(sent.length, 1, 'one POST for one confirmed action; polling never starts a second');
    const body = JSON.parse(sent[0].body);
    assert.equal(body.host, 'mail.example.com');
    assert.match(body.request_id, /^[a-f0-9]{32}$/);
    assert.ok(requests.some((request) => request.method === 'GET' && request.url === `/api/v1/service/operation?id=${opID}`));
    assert.ok(text().includes(fill(en['life.addr.done'], { url: 'https://mail.example.com/' })), text());
    assert.ok(text().includes(en['life.addr.settingsEdited']));
    assert.ok(text().includes('ocsp stapling is off'));
    assert.equal(attr('data-webmail-address-link')[0].props.href, 'https://mail.example.com/');
  } finally { await unmount(); }
});

test('a lost POST answer is looked up by its request id and never sent again', async () => {
  await mount({ reads: [none()], checks: [checkOf()], post: () => { throw new TypeError('network'); }, ops: [] });
  try {
    await pressCheck();
    await click('data-webmail-setup-offer');
    await click('data-webmail-setup-confirm');
    await settle(30);
    const sent = posts().filter((request) => request.url === ADDRESS);
    assert.equal(sent.length, 1);
    const id = JSON.parse(sent[0].body).request_id;
    assert.ok(requests.some((request) => request.url === `/api/v1/service/operation?request_id=${id}`));
    assert.ok(text().includes(en['life.addr.answerLost']));
    await settle(1700);
    assert.equal(posts().filter((request) => request.url === ADDRESS).length, 1);
  } finally { await unmount(); }
});

test('the server re-checks: a refusal shows its check and nothing is followed; a busy server says nothing was started', async () => {
  await mount({
    reads: [none()], checks: [checkOf()],
    post: () => answer({ error: 'not ready', code: 'WEBMAIL_ADDRESS_NOT_READY', check: notFound() }, 409),
  });
  try {
    await pressCheck();
    await click('data-webmail-setup-offer');
    await click('data-webmail-setup-confirm');
    await settle(30);
    assert.ok(text().includes(en['life.addr.notReadyAgain']));
    assert.ok(text().includes(en['life.addr.kind.unmet']), 'the server’s own check is shown');
    assert.equal(requests.filter((request) => request.url.startsWith('/api/v1/service/operation')).length, 0);
  } finally { await unmount(); }
  await mount({ reads: [none()], checks: [checkOf()], post: () => answer({ error: 'busy', code: 'service_operation_busy' }, 409) });
  try {
    await pressCheck();
    await click('data-webmail-setup-offer');
    await click('data-webmail-setup-confirm');
    await settle(30);
    assert.ok(text().includes(en['life.addr.busy']));
  } finally { await unmount(); }
});

test('after a reload the card re-attaches to a running set-up by reading it; it never POSTs', async () => {
  await mount({
    reads: [none({ last_operation: operation('running') }), enabled()],
    post: () => { throw new Error('no POST may be sent'); },
    ops: [operation('running'), operation('succeeded', { success: true, outcome: 'enabled', host: 'mail.example.com', url: 'https://mail.example.com/' })],
  });
  try {
    assert.ok(text().includes(fill(en['life.addr.working'], { host: 'mail.example.com' })));
    await settle(1700);
    await settle(1700);
    assert.equal(posts().length, 0);
    assert.ok(requests.some((request) => request.url === `/api/v1/service/operation?id=${opID}`));
    assert.ok(text().includes(fill(en['life.addr.done'], { url: 'https://mail.example.com/' })));
  } finally { await unmount(); }
});

test('failures are said by their cause, each with who acts and what next; a half-done rollback is a verified failure', async () => {
  const failed = (result) => none({ last_operation: operation('failed', { success: false, outcome: 'not_enabled', host: 'mail.example.com', ...result }, { error: { code: 'webmail_address_failed', message: 'Webmail’s address was not set up.' } }) });
  const cases = [
    ['CERTIFICATE_FAILED', 'port 80 is reachable from the internet'],
    ['NAME_IN_USE', '/etc/nginx/conf.d/other.conf'],
    ['CONFIG_OWNER_EDITED', 'restore the file or remove it'],
    ['PROOF_FAILED', 'the certificate is kept'],
  ];
  for (const [code, needle] of cases) {
    await mount({ reads: [failed({ code, reason: 'file /etc/nginx/conf.d/other.conf' })] });
    try {
      const said = fill(en['life.addr.fail.' + code], { host: 'mail.example.com', reason: 'file /etc/nginx/conf.d/other.conf' });
      assert.ok(text().includes(said), code + ': ' + text());
      assert.ok(said.includes(needle), code + ' mentions ' + needle);
      assert.ok(/Next:/.test(said), code + ' says the next action');
    } finally { await unmount(); }
  }
  await mount({ reads: [failed({ outcome: 'rollback_failed', code: 'ROLLBACK_FAILED', reason: 'nginx -t failed' })] });
  try {
    assert.ok(text().includes('sudo nginx -t'));
    assert.equal(attr('data-life-line').filter((node) => node.props['data-tone'] === 'bad').length > 0, true, 'a verified failure is drawn as one');
  } finally { await unmount(); }
  // An unknown result is not a failure and not a success.
  await mount({ reads: [none({ last_operation: operation('failed', { outcome: 'unknown' }, { error: { code: 'webmail_address_unknown', message: 'x' } }) })] });
  try {
    assert.ok(text().includes(fill(en['life.addr.unknownResult'], { host: 'mail.example.com' })));
    assert.ok(attr('data-life-line').every((node) => node.props['data-tone'] !== 'bad'));
  } finally { await unmount(); }
  // A failure for a name that is already the address is not said any more.
  await mount({ reads: [enabled({ last_operation: operation('failed', { outcome: 'not_enabled', code: 'PROOF_FAILED' }) })] });
  try {
    assert.ok(!text().includes('did not answer there'));
  } finally { await unmount(); }
});

test('an enabled address: the full URL as a safe link, sign-in words, certificate and proof; Remove says exactly what happens', async () => {
  await mount({ reads: [enabled()], post: () => answer({ operation: operation('queued', {}, { kind: 'webmail_address_remove' }) }, 202), ops: [operation('succeeded', { success: true, outcome: 'removed', host: 'mail.example.com', certificate_removed: false }, { kind: 'webmail_address_remove' })] });
  try {
    const link = attr('data-webmail-address-link')[0];
    assert.equal(link.props.href, 'https://mail.example.com/');
    assert.equal(link.props.target, '_blank');
    assert.equal(link.props.rel, 'noopener noreferrer');
    assert.ok(text().includes('https://mail.example.com/'));
    assert.ok(text().includes(en['life.addr.signIn']));
    assert.ok(en['life.addr.signIn'].includes('No panel account is needed'));
    assert.ok(text().includes('Certificate valid until'));
    assert.ok(text().includes(en['life.addr.proved']));
    assert.equal(attr('data-webmail-address-change').length > 0, true);
    await click('data-webmail-remove-offer');
    assert.equal(posts().length, 0, 'the offer only opens the dialog');
    const dialog = text();
    assert.ok(dialog.includes(fill(en['life.addr.removeBody'], { host: 'mail.example.com' })));
    assert.ok(en['life.addr.removeBody'].includes('stops answering webmail') && en['life.addr.removeBody'].includes('Mailboxes and messages are not touched') && en['life.addr.removeBody'].includes('links to it stop working'));
    assert.ok(dialog.includes(en['life.addr.removeCert']));
    assert.ok(dialog.includes('The certificate is kept unless you tick this'));
    await click('data-webmail-remove-confirm');
    await settle(1700);
    const sent = posts().filter((request) => request.url === ADDRESS + '/remove');
    assert.equal(sent.length, 1);
    const body = JSON.parse(sent[0].body);
    assert.equal(body.host, 'mail.example.com');
    assert.equal(body.remove_certificate, false, 'the certificate is kept unless the box is ticked');
    assert.match(body.request_id, /^[a-f0-9]{32}$/);
    assert.ok(text().includes(fill(en['life.addr.removed'], { host: 'mail.example.com' })));
    assert.ok(text().includes(en['life.addr.removedCertKept']));
  } finally { await unmount(); }
  await mount({ reads: [enabled()], post: () => answer({ operation: operation('running', {}, { kind: 'webmail_address_remove' }) }, 202), ops: [operation('running', {}, { kind: 'webmail_address_remove' })] });
  try {
    await click('data-webmail-remove-offer');
    await act(async () => { attr('data-webmail-remove-certificate')[0].props.onChange({ target: { checked: true } }); });
    await click('data-webmail-remove-confirm');
    await settle(30);
    assert.equal(JSON.parse(posts().find((request) => request.url === ADDRESS + '/remove').body).remove_certificate, true);
  } finally { await unmount(); }
});

test('an address the server cannot prove, cannot read or no longer has is said apart', async () => {
  await mount({ reads: [enabled({}, { proved: false, proof_error: 'HTTP 502' })] });
  try {
    assert.ok(text().includes(en['life.addr.notProved']));
    assert.ok(text().includes(fill(en['life.addr.notProvedReason'], { reason: 'HTTP 502' })));
  } finally { await unmount(); }
  await mount({ reads: [{ ...enabled(), status: undefined, status_unknown: true }] });
  try {
    assert.ok(text().includes(en['life.addr.statusUnknown']));
    assert.ok(!text().includes(en['life.addr.notProved']), 'unknown is not "not answering"');
    assert.equal(attr('data-webmail-address-link').length, 1, 'the address is still shown');
  } finally { await unmount(); }
  await mount({ reads: [enabled({}, { present: false, managed: false, proved: false })] });
  try {
    assert.ok(text().includes(en['life.addr.notPresent']));
    assert.equal(attr('data-webmail-address-link').length, 0, 'no link to an address the web server no longer answers');
    assert.equal(attr('data-webmail-host').length, 1, 'the form is open to set it up again');
  } finally { await unmount(); }
});

test('the same card works in Turkish', async () => {
  await mount({ reads: [none()], words: tr });
  try {
    assert.ok(text().includes(tr['life.addr.noneYet']));
    assert.ok(!text().includes('life.addr.'));
  } finally { await unmount(); }
});

// --- Source contracts -----------------------------------------------------------

const card = read('components/WebmailAddressCard.tsx');
const flowSource = read('lib/webmailAddressFlow.ts');

test('the card is mounted on the Roundcube Manage page, above the update card, for an installed Roundcube', () => {
  const detail = read('components/ComponentDetail.tsx');
  assert.ok(detail.includes("import { WebmailAddressCard } from './WebmailAddressCard';"));
  const condition = "{serviceId === 'roundcube' && record?.is_installed === true && (\n                <div className=\"mb-6\"><WebmailAddressCard /></div>";
  assert.ok(detail.includes(condition));
  assert.ok(detail.indexOf('<WebmailAddressCard />') < detail.indexOf('<RoundcubeUpdateCard />'));
});

test('the flow sends one POST per confirmed action, polls with reads only, and looks a lost answer up by request id', () => {
  assert.equal((flowSource.match(/method: 'POST'/g) ?? []).length, 1, 'exactly one place sends a change');
  assert.ok(flowSource.includes('if (sending.current || following) return;'));
  assert.ok(flowSource.includes('readWebmailUpdateOperation(`request_id=${id}`)'));
  assert.ok(flowSource.includes('readWebmailUpdateOperation(`id=${encodeURIComponent(id)}`)'));
  assert.ok(!/setInterval/.test(flowSource + card), 'no interval anywhere');
  assert.ok(!/setInterval|setTimeout\([^)]*runCheck/.test(card), 'the check is never run on a timer');
  // The follow loop is reads only.
  const loop = flowSource.slice(flowSource.indexOf('const follow'), flowSource.indexOf('// Re-attach'));
  assert.ok(!loop.includes("'POST'") && !loop.includes('fetch('));
  // The records are sent only from the confirmed action.
  assert.equal((card.match(/zone\.recordsURL/g) ?? []).length, 1);
  assert.ok(card.indexOf('const addRecords') < card.indexOf("data-webmail-zone-confirm"));
  assert.ok(card.includes("onClick={() => setConfirming('zone')}"), 'the zone button only opens the confirmation');
  assert.ok(card.includes("onClick={() => void addRecords(check)}"));
});

test('the confirmations say that the certificate is kept and the old address goes after the new one works', () => {
  assert.ok(enLifecycleScreens['life.addr.setupSafe'].includes('a certificate that was obtained is kept'));
  assert.ok(enLifecycleScreens['life.addr.step.replace'].includes('after the new one works'));
  assert.ok(enLifecycleScreens['life.addr.removeCertHint'].includes('The certificate is kept unless you tick this'));
  assert.ok(trLifecycleScreens['life.addr.setupSafe'].includes('sertifika korunur'));
  assert.ok(trLifecycleScreens['life.addr.step.replace'].includes('çalıştıktan sonra'));
  assert.ok(trLifecycleScreens['life.addr.removeCertHint'].includes('korunur'));
  assert.ok(card.includes('data-webmail-remove-certificate'));
  assert.ok(card.includes('remove_certificate: removeCertificate'));
  assert.ok(card.includes('useState(false)'), 'the certificate is not deleted by default');
});

test('every key the address screens use exists in English and Turkish, with the same values and no internal names', () => {
  const sources = [card, read('lib/webmailAddressText.ts'), read('lib/webmailAddressFlow.ts'), read('components/WebmailAccess.tsx')].join('\n');
  const keys = new Set([...sources.matchAll(/'((?:life\.addr|mail\.webmail)\.[A-Za-z_.]+)'/g)].map((m) => m[1]));
  for (const name of ['ready', 'firewall_closed', 'no_record', 'points_elsewhere', 'points_to_peer', 'dns_unreachable', 'server_address_unknown', 'invalid', 'panel_address']) keys.add('life.addr.msg.' + name);
  for (const name of ['ADDRESS_INVALID', 'ROUNDCUBE_NOT_INSTALLED', 'NGINX_UNAVAILABLE', 'PHP_UNAVAILABLE', 'CERTBOT_MISSING', 'NAME_IN_USE', 'CONFIG_OWNER_EDITED', 'NGINX_REJECTED', 'CERTIFICATE_FAILED', 'PROOF_FAILED', 'ROLLBACK_FAILED']) keys.add('life.addr.fail.' + name);
  assert.ok(keys.size > 80, `read ${keys.size} keys`);
  for (const key of [...keys].filter((k) => k.startsWith('life.addr.'))) {
    assert.ok(enLifecycleScreens[key], 'missing EN ' + key);
    assert.ok(trLifecycleScreens[key], 'missing TR ' + key);
    assert.notEqual(enLifecycleScreens[key], trLifecycleScreens[key], key + ' is not translated');
    const vars = (s) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort().join(',');
    assert.equal(vars(trLifecycleScreens[key]), vars(enLifecycleScreens[key]), key + ' uses different values');
    for (const words of [enLifecycleScreens[key], trLifecycleScreens[key]]) assert.doesNotMatch(words, /alpha\.?\d|D-0\d\d|\bB\d\b/, key + ' shows an internal name');
  }
  const mailEn = read('i18n/screens/mailDatabases/en.ts');
  const mailTr = read('i18n/screens/mailDatabases/tr.ts');
  for (const key of ['mail.webmail.signIn', 'mail.webmail.noAddressAdmin', 'mail.webmail.setAddress']) {
    assert.ok(mailEn.includes(`'${key}':`) && mailTr.includes(`'${key}':`), key);
  }
  assert.ok(!mailEn.includes('mail.webmail.heldBack') && !mailTr.includes('mail.webmail.heldBack'), 'the hold-back sentence is gone');
  for (const part of ['webmailAddress', 'mailDatabases', 'dashboard']) {
    for (const lang of ['en', 'tr']) {
      const source = read(`i18n/screens/${part}/${lang}.ts`);
      const opening = source.indexOf('{\n', source.indexOf('export const')) + 2;
      assert.ok(source.slice(opening).startsWith('    // ---- alpha.110 webmail address ----'), `${part}/${lang}: the alpha.110 block sits right after the opening line`);
    }
  }
});

test('nothing in the screens still links to the panel path or holds the links back', () => {
  for (const file of ['components/RoundcubeUpdateFlow.tsx', 'components/WebmailAccess.tsx', 'components/ServiceList.tsx', 'lib/roundcubeUpdate.ts', 'lib/mailSetup.ts']) {
    const source = read(file);
    assert.ok(!source.includes('WEBMAIL_LINK_OFFERED'), file);
    assert.ok(!source.includes('href="/webmail/"') && !source.includes("href='/webmail/'") && !source.includes("'/webmail/'"), file);
  }
  const flow = read('components/RoundcubeUpdateFlow.tsx');
  assert.ok(flow.includes('if (!isWebmailAddressURL(url)) return null;'));
  assert.ok(flow.includes('href={url}'));
  const row = read('components/ServiceList.tsx');
  assert.ok(row.includes('<OpenWebmailLink url={webmailUrl} />'));
});
