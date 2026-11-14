// alpha.135: "Add" beside "Copy" for webmail's address record, wherever the
// panel can add it safely (owner, 2026-11-14). Three cases by where the name's
// DNS lives: a zone this panel hosts (its own record editor), the paired main
// DNS server's zone (one confirmed request over the authorized connection), or
// elsewhere (Copy only, and the name servers public DNS names). Every add runs
// in the panel's own confirmation and progress; outcomes are kept apart (done,
// a refusal with who acts and what next, not known); a lost answer is never sent
// again; after an add the page reads public DNS by itself, slowly and for a
// bounded time, and never sends a change while it does.
//
// alpha.135: "Kopyala"nın yanında "Ekle". Üç durum; her ekleme panelin kendi
// onayında ve ilerlemesinde; sonuçlar ayrı; ardından genel DNS sınırlı ve
// yalnız okunarak beklenir.
import assert from 'node:assert/strict';
import { readdirSync, readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { apiErrorURL, dataModule, reactURL, requestIdentityURL, roundcubeUpdateURL, sharedLayer, webmailAddressLibURL } from './fixtures/shared-layer.mjs';
import { enWebmailAddressScreens as enScreens } from '../src/i18n/screens/webmailAddress/en.ts';
import { trWebmailAddressScreens as trScreens } from '../src/i18n/screens/webmailAddress/tr.ts';
import { enDnsAddScreens } from '../src/i18n/screens/dnsAdd/en.ts';
import { trDnsAddScreens } from '../src/i18n/screens/dnsAdd/tr.ts';

const read = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const lib = await import(webmailAddressLibURL);

const en = { ...enScreens, ...enDnsAddScreens, 'common.cancel': 'Cancel', 'common.retry': 'Try again', 'common.staleNotice': 'stale' };
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
// The panel's own confirmation, as the ConfirmHost answers it: the question is
// kept, and a confirmed action runs with a report that keeps its steps.
const confirmStub = dataModule(`
  export const confirmAction = async (request) => {
    globalThis.confirmLog.push(request);
    if (!globalThis.confirmYes) return false;
    const run = { steps: [] };
    globalThis.confirmRuns.push(run);
    run.end = request.action ? await request.action({ steps: (s) => { run.steps = s; } }, { option: false, typed: '' }) : undefined;
    return true;
  };
`);
const shared = sharedLayer(stub);
const recordsURL = shared.compile('lib/webmailAddressRecords.ts', { '/confirmAction': confirmStub, '/requestIdentity': requestIdentityURL, '/webmailAddress': webmailAddressLibURL, '/apiError': apiErrorURL });
const { WebmailAddressCard } = await import(shared.compile('components/WebmailAddressCard.tsx', {
  '/LifeLine': shared.compile('components/LifeLine.tsx'),
  '/lib/webmailAddressText': shared.compile('lib/webmailAddressText.ts'),
  '/lib/webmailAddressFlow': shared.compile('lib/webmailAddressFlow.ts', { '/requestIdentity': requestIdentityURL, '/roundcubeUpdate': roundcubeUpdateURL, '/webmailAddress': webmailAddressLibURL }),
  '/lib/webmailAddressRecords': recordsURL,
}));

const ADDRESS = '/api/v1/service/roundcube/address';
const DNS_RECORD = ADDRESS + '/dns-record';
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
const none = () => ({ address: null, suggestion: 'mail.boston.example.com', expected: ['203.0.113.5'], last_operation: null });
const A = 'mail.boston.example.com. 300 IN A 203.0.113.5';
const AAAA = 'mail.boston.example.com. 300 IN AAAA 2001:db8::5';
const notFound = (extra = {}) => ({
  host: 'mail.boston.example.com', state: 'not_found', code: 'no_record', ready: false, message: 'x', expected: ['203.0.113.5'], answered: [],
  firewall: { state: 'open' }, records: [{ type: 'A', name: 'mail.boston', content: '203.0.113.5', ttl: 300, text: A }],
  owner_action: { who: 'administrator', next: 'x' }, ...extra,
});
const peerZone = { server: 'frankfurt.example.com', zone: 'example.com' };
const ready = () => ({ host: 'mail.boston.example.com', state: 'ready', ready: true, message: 'ok', expected: ['203.0.113.5'], answered: ['203.0.113.5'], firewall: { state: 'open' } });
const recordAnswer = (records) => ({ record: { request_id: 'a'.repeat(32), host: 'mail.boston.example.com', server: 'frankfurt.example.com', zone: 'example.com', state: 'done', records } });

// Time: the card's 20-second pace and 5-minute bound run on a test clock.
const realSetTimeout = globalThis.setTimeout;
const realClearTimeout = globalThis.clearTimeout;
const realNow = Date.now;
let clock = 0;
let paced = [];

let tree;
let requests = [];
async function settle(ms = 10) { await act(async () => { await new Promise((resolve) => realSetTimeout(resolve, ms)); }); }
async function mount({ checks, dnsRecord, records }) {
  globalThis.cardWords = en;
  globalThis.confirmLog = [];
  globalThis.confirmRuns = [];
  globalThis.confirmYes = true;
  globalThis.document = { addEventListener() {}, removeEventListener() {}, getElementById: () => null, activeElement: null, body: { style: {} } };
  clock = 0;
  paced = [];
  Date.now = () => realNow() + clock;
  // The wait's 20-second timers are held and released one by one (tick).
  globalThis.setTimeout = (fn, ms, ...rest) => {
    if (ms !== 20_000) return realSetTimeout(fn, ms, ...rest);
    const id = { paced: true };
    paced.push({ id, fn });
    return id;
  };
  globalThis.clearTimeout = (id) => {
    if (id && id.paced) paced = paced.filter((entry) => entry.id !== id);
    else realClearTimeout(id);
  };
  requests = [];
  let checkCount = 0;
  globalThis.fetch = async (input, init = {}) => {
    const url = String(input);
    const method = init.method ?? 'GET';
    requests.push({ url, method, body: init.body });
    if (url === ADDRESS && method === 'GET') return answer(none());
    if (url === ADDRESS + '/check') return answer({ check: checks[Math.min(checkCount++, checks.length - 1)] });
    if (url === DNS_RECORD) return dnsRecord(init);
    if (records && url === records.url) return records.answer(init);
    return answer({ error: 'no route' }, 404);
  };
  await act(async () => { tree = Renderer.create(React.createElement(WebmailAddressCard)); });
  await settle();
}
async function unmount() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.setTimeout = realSetTimeout;
  globalThis.clearTimeout = realClearTimeout;
  Date.now = realNow;
  delete globalThis.document;
}
const text = () => { const walk = (n) => (typeof n === 'string' ? n : Array.isArray(n) ? n.map(walk).join('') : (n?.children || []).map(walk).join('')); return walk(tree.toJSON()); };
const attr = (name) => tree.root.findAll((node) => node.props && node.props[name] !== undefined);
const posts = (url) => requests.filter((request) => request.method === 'POST' && request.url === url);
async function pressCheck() { await act(async () => { attr('data-webmail-address-form')[0].props.onSubmit({ preventDefault() {} }); }); await settle(); }
async function click(name) { await act(async () => { attr(name)[0].props.onClick(); }); await settle(30); }
// Twenty seconds pass: the one held timer of the wait runs.
async function tick() {
  const next = paced.shift();
  if (!next) return false;
  clock += 20_000;
  await act(async () => { next.fn(); });
  await settle();
  return true;
}
const lastRun = () => globalThis.confirmRuns[globalThis.confirmRuns.length - 1];

test('the decoders fail closed for the peer, the name servers and the answer', () => {
  const base = { check: notFound() };
  const peer = (value) => lib.decodeWebmailAddressCheck({ check: { ...base.check, peer_zone: value } }).peerZone;
  assert.deepEqual(peer(peerZone), peerZone);
  assert.deepEqual(peer({ server: 'frankfurt.example.com' }), { server: 'frankfurt.example.com', zone: '' });
  for (const bad of [{ server: 'Frankfurt' }, { server: 'https://x.example.com/' }, { server: 'a.example.com', zone: 'x y' }, 'x']) assert.equal(peer(bad), null, JSON.stringify(bad));
  const managed = (value) => lib.decodeWebmailAddressCheck({ check: { ...base.check, managed_by: value } }).managedBy;
  assert.deepEqual(managed({ zone: 'example.org', nameservers: ['ns1.host.test'] }), { zone: 'example.org', nameservers: ['ns1.host.test'] });
  for (const bad of [{ zone: 'example.org', nameservers: [] }, { zone: 'example.org', nameservers: ['<b>'] }, { zone: 'x', nameservers: ['ns1.host.test'] }]) assert.equal(managed(bad), null);
  const decoded = lib.decodeWebmailDNSRecordAnswer(recordAnswer([{ type: 'A', text: A, outcome: 'added' }, { type: 'AAAA', text: AAAA, outcome: 'refused', code: 'address' }]));
  assert.equal(decoded.records[1].code, 'address');
  for (const bad of [null, {}, { record: {} }, recordAnswer([]), recordAnswer([{ type: 'A', text: A, outcome: 'done' }]), recordAnswer([{ type: 'AAAA', text: AAAA, outcome: 'added' }])]) {
    assert.equal(lib.decodeWebmailDNSRecordAnswer(bad), null, JSON.stringify(bad));
  }
});

test('the main DNS server holds the name: Add beside Copy, the panel’s own confirmation, ONE request, then a bounded read-only wait until DNS shows it', async () => {
  await mount({
    checks: [notFound({ peer_zone: peerZone }), notFound({ peer_zone: peerZone }), ready()],
    dnsRecord: () => answer(recordAnswer([{ type: 'A', text: A, outcome: 'added' }])),
  });
  try {
    await pressCheck();
    assert.ok(attr('data-webmail-peer-offer').length > 0, 'Add is offered');
    assert.equal(attr('data-webmail-zone-offer').length, 0);
    assert.ok(text().includes(fill(en['dnsadd.next.peer'], { server: 'frankfurt.example.com' })), 'the next action names the other server');
    assert.ok(text().includes(en['life.addr.copy']), 'Copy stays');
    // The offer only opens the confirmation; a cancelled one sends nothing.
    globalThis.confirmYes = false;
    await click('data-webmail-peer-offer');
    const question = globalThis.confirmLog[0];
    assert.equal(question.title, fill(en['dnsadd.peer.title'], { server: 'frankfurt.example.com' }));
    assert.ok(question.message.includes(fill(en['dnsadd.peer.body'], peerZone)) && question.message.includes(A) && question.message.includes(en['dnsadd.peer.transfer']));
    assert.deepEqual(question.kept, [en['dnsadd.peer.keptZone'], fill(en['dnsadd.peer.keptOwner'], { server: 'frankfurt.example.com' })]);
    assert.equal(posts(DNS_RECORD).length, 0, 'nothing is sent before the confirmation');
    assert.ok(attr('data-webmail-peer-offer').length > 0, 'a cancelled question keeps the offer');
    globalThis.confirmYes = true;
    await click('data-webmail-peer-offer');
    assert.equal(posts(DNS_RECORD).length, 1);
    const body = JSON.parse(posts(DNS_RECORD)[0].body);
    assert.equal(body.host, 'mail.boston.example.com');
    assert.match(body.request_id, /^[a-f0-9]{32}$/);
    const run = lastRun();
    assert.equal(run.end.state, 'done');
    assert.ok(run.end.message.includes(fill(en['dnsadd.peer.added'], { server: 'frankfurt.example.com', record: A })));
    assert.deepEqual(run.steps.map((step) => `${step.id}:${step.state}`), ['ask:done', 'write:done', 'dns:waiting']);
    assert.equal(run.steps[2].note, en['dnsadd.step.dnsNote']);
    // Visible waiting, no second offer, and only reads.
    assert.ok(text().includes(en['dnsadd.wait']));
    assert.equal(attr('data-webmail-peer-offer').length, 0, 'not offered again for the added name');
    assert.equal(posts(ADDRESS + '/check').length, 1, 'nothing is read before twenty seconds');
    await tick();
    assert.equal(posts(ADDRESS + '/check').length, 2, 'public DNS is read again by itself');
    assert.ok(text().includes(en['dnsadd.wait']), 'still not there: the wait goes on, the last answer stays on screen');
    assert.ok(attr('data-webmail-records').length > 0);
    await tick();
    assert.equal(posts(ADDRESS + '/check').length, 3);
    assert.ok(attr('data-webmail-setup-offer').length > 0, 'ready: the existing set-up button');
    assert.ok(!text().includes(en['dnsadd.wait']), 'the wait ends');
    assert.equal(await tick(), false, 'nothing more is scheduled');
    assert.equal(posts(DNS_RECORD).length, 1, 'polling never sends the request again');
  } finally { await unmount(); }
});

test('the wait is bounded: after five minutes it says so, offers Check and never adds again', async () => {
  await mount({ checks: [notFound({ peer_zone: peerZone })], dnsRecord: () => answer(recordAnswer([{ type: 'A', text: A, outcome: 'present' }])) });
  try {
    await pressCheck();
    await click('data-webmail-peer-offer');
    assert.ok(lastRun().end.message.includes(fill(en['dnsadd.peer.present'], { server: 'frankfurt.example.com', record: A })));
    let ticks = 0;
    while (ticks < 40 && await tick()) ticks++;
    assert.equal(ticks, 15, 'one read every 20 seconds for 5 minutes');
    assert.equal(posts(ADDRESS + '/check').length, 16, 'the check pressed, then the fifteen reads of the wait');
    assert.ok(text().includes(fill(en['dnsadd.waitOver.peer'], { server: 'frankfurt.example.com' })));
    assert.ok(!text().includes(en['dnsadd.wait']));
    assert.equal(attr('data-webmail-peer-offer').length, 0, 'not added again from here');
    assert.ok(attr('data-webmail-check').length > 0, 'Check stays');
    assert.equal(posts(DNS_RECORD).length, 1);
  } finally { await unmount(); }
});

test('each refusal says the reason, who acts and what next; Copy stays and nothing is read in a loop', async () => {
  for (const code of ['outside_zone', 'taken', 'address', 'not_ready', 'unauthorized', 'unsupported', 'license']) {
    await mount({ checks: [notFound({ peer_zone: peerZone })], dnsRecord: () => answer(recordAnswer([{ type: 'A', text: A, outcome: 'refused', code }])) });
    try {
      await pressCheck();
      await click('data-webmail-peer-offer');
      const end = lastRun().end;
      assert.equal(end.state, 'blocked', code);
      assert.equal(end.reason, fill(en['dnsadd.refused.' + code], { server: 'frankfurt.example.com' }), code);
      assert.ok(/Next:/.test(end.reason), code + ' says the next action');
      assert.equal(attr('data-webmail-peer-offer').length, 0, 'the refused answer is not offered again until Check');
      assert.ok(text().includes(en['life.addr.copy']), 'Copy stays');
      assert.equal(paced.length, 0, 'no wait is scheduled after a refusal');
      assert.equal(posts(ADDRESS + '/check').length, 1, 'no wait after a refusal');
      await pressCheck();
      assert.ok(attr('data-webmail-peer-offer').length > 0, 'a new Check offers it again');
    } finally { await unmount(); }
  }
  await mount({ checks: [notFound({ peer_zone: peerZone })], dnsRecord: () => answer(recordAnswer([{ type: 'A', text: A, outcome: 'not_sent', code: 'unreachable' }])) });
  try {
    await pressCheck();
    await click('data-webmail-peer-offer');
    assert.equal(lastRun().end.reason, fill(en['dnsadd.refused.unreachable'], { server: 'frankfurt.example.com' }));
  } finally { await unmount(); }
  await mount({ checks: [notFound({ peer_zone: peerZone })], dnsRecord: () => answer({ error: 'x', code: 'WEBMAIL_DNS_RECORD_NOT_OFFERED' }, 409) });
  try {
    await pressCheck();
    await click('data-webmail-peer-offer');
    const end = lastRun().end;
    assert.equal(end.state, 'blocked');
    assert.equal(end.reason, fill(en['dnsadd.peer.notOffered'], { server: 'frankfurt.example.com' }));
    assert.equal(end.fix.label, en['life.addr.check']);
  } finally { await unmount(); }
});

test('no answer is not known: said so, never sent again, Check is the next step', async () => {
  for (const dnsRecord of [() => { throw new TypeError('network'); }, () => answer(recordAnswer([{ type: 'A', text: A, outcome: 'unknown' }])), () => answer({ nonsense: true })]) {
    await mount({ checks: [notFound({ peer_zone: peerZone }), notFound({ peer_zone: peerZone })], dnsRecord });
    try {
      await pressCheck();
      await click('data-webmail-peer-offer');
      const end = lastRun().end;
      assert.equal(end.state, 'unknown');
      assert.equal(end.reason, fill(en['dnsadd.peer.unknown'], { server: 'frankfurt.example.com' }));
      assert.equal(lastRun().steps[0].note, en['dnsadd.step.noAnswer']);
      assert.equal(paced.length, 0, 'no wait is scheduled for an unknown result');
      assert.equal(posts(DNS_RECORD).length, 1, 'not sent again');
      assert.equal(posts(ADDRESS + '/check').length, 1, 'no wait for an unknown result');
      await act(async () => { end.fix.run(); });
      await settle();
      assert.equal(posts(ADDRESS + '/check').length, 2, 'Check reads DNS');
      assert.equal(posts(DNS_RECORD).length, 1);
    } finally { await unmount(); }
  }
});

test('an AAAA record the main server does not accept: the A record is added, the AAAA stays optional with Copy', async () => {
  const records = [{ type: 'A', name: 'mail.boston', content: '203.0.113.5', ttl: 300, text: A }, { type: 'AAAA', name: 'mail.boston', content: '2001:db8::5', ttl: 300, text: AAAA }];
  await mount({ checks: [notFound({ peer_zone: peerZone, records })], dnsRecord: () => answer(recordAnswer([{ type: 'A', text: A, outcome: 'added' }, { type: 'AAAA', text: AAAA, outcome: 'refused', code: 'address' }])) });
  try {
    await pressCheck();
    globalThis.confirmYes = false;
    await click('data-webmail-peer-offer');
    assert.ok(globalThis.confirmLog[0].message.includes(fill(en['dnsadd.peer.aaaa'], { server: 'frankfurt.example.com' })), 'the confirmation says when the AAAA record is added');
    assert.ok(globalThis.confirmLog[0].message.includes(AAAA));
    globalThis.confirmYes = true;
    await click('data-webmail-peer-offer');
    const why = fill(en['dnsadd.aaaa.address'], { server: 'frankfurt.example.com' });
    assert.equal(lastRun().end.state, 'done');
    assert.ok(lastRun().end.message.includes(why));
    const optional = attr('data-webmail-optional-record');
    assert.equal(optional.length, 1);
    assert.ok(text().includes(why) && text().includes(AAAA));
  } finally { await unmount(); }
  await mount({ checks: [notFound({ peer_zone: peerZone, records })], dnsRecord: () => answer(recordAnswer([{ type: 'A', text: A, outcome: 'added' }, { type: 'AAAA', text: AAAA, outcome: 'added' }])) });
  try {
    await pressCheck();
    await click('data-webmail-peer-offer');
    assert.ok(lastRun().end.message.includes(en['dnsadd.peer.alsoAAAA']));
    assert.equal(attr('data-webmail-optional-record').length, 0);
  } finally { await unmount(); }
});

test('the zone is hosted here: added through its own editor after the confirmation, then the same bounded wait', async () => {
  const zone = { name: 'example.com', records_url: '/api/v1/domains/7/dns/records' };
  await mount({ checks: [notFound({ zone }), notFound({ zone }), ready()], records: { url: zone.records_url, answer: () => answer({ id: 1 }, 201) }, dnsRecord: () => { throw new Error('the other server is never asked for a zone hosted here'); } });
  try {
    await pressCheck();
    assert.equal(attr('data-webmail-peer-offer').length, 0);
    globalThis.confirmYes = false;
    await click('data-webmail-zone-offer');
    assert.equal(globalThis.confirmLog[0].title, fill(en['life.addr.zoneDialogTitle'], { zone: 'example.com' }));
    assert.ok(globalThis.confirmLog[0].message.includes(A));
    assert.equal(posts(zone.records_url).length, 0);
    globalThis.confirmYes = true;
    await click('data-webmail-zone-offer');
    assert.deepEqual(JSON.parse(posts(zone.records_url)[0].body), { name: 'mail.boston', type: 'A', content: '203.0.113.5', ttl: 300, prio: 0 });
    assert.equal(lastRun().end.state, 'done');
    assert.deepEqual(lastRun().steps.map((step) => `${step.id}:${step.state}`), ['write:done', 'dns:waiting']);
    assert.ok(text().includes(en['dnsadd.wait']));
    await tick();
    await tick();
    assert.ok(attr('data-webmail-setup-offer').length > 0);
    assert.equal(posts(DNS_RECORD).length, 0);
  } finally { await unmount(); }
  await mount({ checks: [notFound({ zone })], records: { url: zone.records_url, answer: () => answer({ error: 'record conflicts with a CNAME' }, 409) } });
  try {
    await pressCheck();
    await click('data-webmail-zone-offer');
    assert.equal(lastRun().end.state, 'failed');
    assert.equal(lastRun().end.reason, fill(en['life.addr.addFailed'], { record: A, zone: 'example.com', reason: 'record conflicts with a CNAME' }));
  } finally { await unmount(); }
  await mount({ checks: [notFound({ zone })], records: { url: zone.records_url, answer: () => { throw new TypeError('network'); } } });
  try {
    await pressCheck();
    await click('data-webmail-zone-offer');
    assert.equal(lastRun().end.state, 'unknown');
    assert.equal(lastRun().end.reason, fill(en['life.addr.addLost'], { record: A }));
    assert.equal(paced.length, 0);
    assert.equal(posts(zone.records_url).length, 1, 'not sent again');
  } finally { await unmount(); }
});

test('elsewhere: Copy only, and where the name’s DNS is managed when public DNS names it', async () => {
  await mount({ checks: [notFound({ managed_by: { zone: 'example.org', nameservers: ['ns1.host.test', 'ns2.host.test'] }, records: [{ type: 'A', name: 'mail.boston.example.com.', content: '203.0.113.5', ttl: 3600, text: 'x' }] })], dnsRecord: () => { throw new Error('never'); } });
  try {
    await pressCheck();
    assert.equal(attr('data-webmail-peer-offer').length + attr('data-webmail-zone-offer').length, 0);
    assert.equal(attr('data-webmail-managed-by').length, 1);
    assert.ok(text().includes(fill(en['dnsadd.managedBy'], { zone: 'example.org', nameservers: 'ns1.host.test, ns2.host.test' })));
    assert.ok(text().includes(fill(en['life.addr.next.no_record.external'], {})));
  } finally { await unmount(); }
  // No connection, or the name servers could not be read: no offer and no guess.
  await mount({ checks: [notFound()] , dnsRecord: () => { throw new Error('never'); } });
  try {
    await pressCheck();
    assert.equal(attr('data-webmail-peer-offer').length + attr('data-webmail-managed-by').length, 0);
  } finally { await unmount(); }
});

test('only administrators reach this: the card is on the Components page and the request is under the administrator-only prefix', () => {
  assert.ok(lib.WEBMAIL_ADDRESS_DNS_RECORD_URL.startsWith('/api/v1/service/'), 'the server refuses /api/v1/service/ to anyone but an administrator');
  const detail = read('components/ComponentDetail.tsx');
  assert.ok(detail.includes("{serviceId === 'roundcube' && record?.is_installed === true && ("));
  const mounted = readdirSync(new URL('../src/', import.meta.url), { recursive: true })
    .filter((file) => /\.tsx$/.test(file) && read(file.replace(/\\/g, '/')).includes('<WebmailAddressCard'));
  assert.deepEqual(mounted.map((file) => file.replace(/\\/g, '/')), ['components/ComponentDetail.tsx'], 'the card is mounted nowhere else');
  const app = read('App.tsx');
  assert.ok(/ComponentDetail/.test(app));
});

test('the add flows send changes only from the confirmed action, and the card’s only timer is the bounded wait', () => {
  const records = read('lib/webmailAddressRecords.ts');
  const card = read('components/WebmailAddressCard.tsx');
  assert.equal((records.match(/method: 'POST'/g) ?? []).length, 2, 'one POST for the zone records, one for the request');
  for (const marker of ['zone.recordsURL', 'WEBMAIL_ADDRESS_DNS_RECORD_URL, {']) {
    const at = records.indexOf(marker);
    const action = records.lastIndexOf('action: async', at);
    assert.ok(at > 0 && action >= 0, marker + ' is sent inside a confirmed action');
  }
  assert.ok(!/fetch\(/.test(card.slice(card.indexOf('// After an add'), card.indexOf('const addHooks'))), 'the wait only calls the read-only check');
  assert.ok(!/setInterval/.test(card + records));
  assert.equal((card.match(/setTimeout\(/g) ?? []).length, 3, 'the copied mark and the two steps of the bounded wait');
  assert.ok(card.includes('RECHECK_EVERY_MS') && card.includes('RECHECK_FOR_MS'));
  assert.ok(records.includes('RECHECK_EVERY_MS = 20_000') && records.includes('RECHECK_FOR_MS = 5 * 60_000'));
  assert.ok(!card.includes('zoneDialogWait') && !enScreens['life.addr.zoneDialogWait'] && !trScreens['life.addr.zoneDialogWait'], 'the "checks once, never a loop" sentence is gone');
});

test('the words of the dnsAdd part: every key used exists in English and Turkish with the same values, none unused', () => {
  const keys = Object.keys(enDnsAddScreens);
  assert.ok(keys.length >= 40 && keys.every((key) => key.startsWith('dnsadd.')), `${keys.length} keys`);
  const vars = (s) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort().join(',');
  for (const key of keys) {
    assert.ok(trDnsAddScreens[key], 'missing TR ' + key);
    assert.notEqual(trDnsAddScreens[key], enDnsAddScreens[key], key);
    assert.equal(vars(trDnsAddScreens[key]), vars(enDnsAddScreens[key]), key);
    assert.doesNotMatch(trDnsAddScreens[key], /REMOTE_DNS|outside_zone|request_id|connection_id/, key + ' shows a raw code');
  }
  const sources = [read('lib/webmailAddressRecords.ts'), read('lib/webmailAddressText.ts'), read('components/WebmailAddressCard.tsx')].join('\n');
  const used = new Set([...sources.matchAll(/'(dnsadd\.[A-Za-z_.]+)'/g)].map((m) => m[1]));
  for (const key of used) assert.ok(enDnsAddScreens[key], 'missing ' + key);
  for (const key of keys) assert.ok(used.has(key), 'unused ' + key);
});
