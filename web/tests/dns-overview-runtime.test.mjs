// alpha.115: the DNS page's overview, mounted with the shipped shared layer
// (lib/remote.ts, lib/lostAnswer.ts, ui.tsx) and the real EN/TR catalogue.
// It covers: the decoder failing closed; the sentence first, worded from its
// code with the servers named by the page ("This server (alpha)", never an
// invented name); a verified failure as an alert; an unreadable overview said
// as such; the provider home without DNS software; the join entry; Advanced
// mounted only when opened; and "Move mail to …" — a confirmation that states
// the consequence, one read of the list, ONE PUT of the one MX, nothing sent
// again after a lost answer, and no change at all when several MX exist.
//
// alpha.115: DNS sayfasinin ozeti; cumle once, sunucular sayfanin adiyla,
// dogrulanmis hata uyari olarak, posta tasima onaydan sonra tek PUT.
import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { enScreens, trScreens } from './locale-catalogue.mjs';

const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null, Loader2 = () => null;
  const fill = (text, vars) => Object.entries(vars || {}).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
  export const useI18n = () => ({ t: (key, vars) => (globalThis.dnsWords?.[key] !== undefined ? fill(globalThis.dnsWords[key], vars) : key), locale: 'en' });
  export const useNavigate = () => () => true;
  export const Link = (props) => React.createElement('a', { href: props.to, className: props.className }, props.children);
  export const SetupJoinCard = () => React.createElement('join-card');
`);
const shared = sharedLayer(stub);
const libURL = shared.compile('lib/dnsOverview.ts');
const lib = await import(libURL);
const { DNSOverview } = await import(shared.compile('components/DNSOverview.tsx', { '/lib/dnsOverview': libURL }));

const originalFetch = globalThis.fetch;
let tree;
let requests = [];
const json = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
async function settle(ms = 10) { await act(async () => { await new Promise((resolve) => setTimeout(resolve, ms)); }); }

const sentence = (code, state = 'ok', vars = { domain: 'example.test' }) => ({ state, code: `dns_overview_${code}`, reason: 'server reason', who: 'server who', next: 'server next', then: 'server then', vars });
const paired = (extra = {}) => ({
  domain: 'example.test', home: 'own', setup_ready: true, sentence: sentence('ok'),
  servers: [
    { who: 'this', role: 'main', short_name: 'alpha', panel_name: 'panel.example.test', software: 'bind', ipv4: '203.0.113.10', answers: 'ok', serial: 1 },
    { who: 'peer', role: 'backup', short_name: 'beta', panel_name: 'beta.example.test', software: 'bind', ipv4: '198.51.100.20', answers: 'ok' },
  ],
  names: [
    { name: 'panel.example.test', kind: 'panel', type: 'A', server: 'this', expected: '203.0.113.10', seen: ['203.0.113.10'], state: 'ok', where: 'zone' },
    { name: 'example.test', kind: 'nameservers', type: 'NS', expected: 'ns1.example.test, ns2.example.test', seen: ['ns1.example.test', 'ns2.example.test'], state: 'ok', where: 'registrar' },
  ],
  mail: [], mail_receiver: { server: 'none' },
  own_records_link: { where: 'here', path: '/domains/infrastructure/example.test', api: '/api/v1/dns/infrastructure-zones/example.test' },
  join: { state: 'bound' }, checked_at: '2026-10-28T10:00:00Z', next_check_after: 60, ...extra,
});

async function mount(overview, { words = enScreens, routes = {} } = {}) {
  globalThis.dnsWords = words;
  requests = [];
  globalThis.fetch = async (url, init = {}) => {
    requests.push({ url, method: init.method || 'GET', body: init.body });
    if (routes[url]) return routes[url](init);
    if (url === '/api/v1/dns/overview') return typeof overview === 'function' ? overview() : json(overview);
    return json({ error: 'unexpected' }, 404);
  };
  await act(async () => { tree = Renderer.create(React.createElement(DNSOverview, { advanced: React.createElement('advanced-screen') })); });
  await settle();
}
async function unmount() {
  if (tree) await act(async () => tree.unmount());
  tree = null;
  globalThis.fetch = originalFetch;
  delete globalThis.dnsWords;
}
const textOf = () => JSON.stringify(tree.toJSON());
const byProp = (prop, value) => tree.root.findAll((node) => node.props && node.props[prop] === value);
const button = (label) => tree.root.findAllByType('button').find((node) => JSON.stringify(node.props.children).includes(label));

test('the decoder fails closed: an answer outside the contract is unknown, never a guess', () => {
  assert.equal(lib.decodeDNSOverview(paired()).servers.length, 2);
  for (const bad of [
    null, {}, { ...paired(), home: 'cloud' }, { ...paired(), sentence: { ...sentence('ok'), state: 'fine' } },
    { ...paired(), servers: [...paired().servers, paired().servers[0]] },
    { ...paired(), names: [{ ...paired().names[0], state: 'perhaps' }] },
    { ...paired(), own_records_link: { where: 'here', path: 'https://evil.example/' } },
    { ...paired(), own_records_link: { where: 'here', path: '/domains/x', api: 'https://evil.example/api' } },
    { ...paired(), mx_move: { zone: 'example.test', to_host: 'mail.example.test', to: 'elsewhere' } },
    { ...paired(), join: { state: 'maybe' } },
  ]) assert.throws(() => lib.decodeDNSOverview(bad), undefined, JSON.stringify(bad)?.slice(0, 80));
});

test('every sentence code and every state the page can show has words in English and Turkish', () => {
  for (const code of lib.DNS_OVERVIEW_CODES) {
    for (const part of ['title', 'why', 'who', 'next', 'then']) {
      const key = `dnsov.s.${code}.${part}`;
      assert.ok(enScreens[key], key);
      assert.ok(trScreens[key] && trScreens[key] !== enScreens[key], `${key} (tr)`);
    }
  }
  for (const state of ['ok', 'missing', 'different', 'not_yet', 'unknown', 'kept', 'pending']) assert.ok(enScreens[`dnsov.state.${state}`] && trScreens[`dnsov.state.${state}`], state);
  for (const reason of ['setup_not_done', 'standalone_bind', 'standalone_pdns', 'external', 'remote', 'backup']) assert.ok(enScreens[`dnsov.join.${reason}`] && trScreens[`dnsov.join.${reason}`], reason);
  assert.equal(lib.dnsOverviewSentenceKey('dns_overview_ok'), 'dnsov.s.ok');
  assert.equal(lib.dnsOverviewSentenceKey('dns_overview_from_a_later_release'), null);
});

test('all good: the sentence first, both servers by the page\'s own names, the names with the internet\'s answer, Advanced closed', async () => {
  await mount(paired());
  try {
    const lead = byProp('data-dns-sentence', 'dns_overview_ok')[0];
    assert.equal(lead.props['data-state'], 'ok');
    assert.equal(lead.props.role, 'status');
    const text = textOf();
    assert.ok(text.includes('All good') && text.includes('This server (alpha)') && text.includes('Second server (beta)'));
    assert.ok(text.indexOf('All good') < text.indexOf('Your DNS servers'), 'the sentence comes before the servers');
    assert.ok(!/dnsov\./.test(text), 'no raw key on screen');
    assert.ok(!text.includes('Frankfurt') && !text.includes('Boston'), 'no invented server names');
    assert.equal(byProp('data-dns-name', 'panel.example.test')[0].props['data-state'], 'ok');
    assert.equal(tree.root.findAllByType('advanced-screen').length, 0, 'Advanced is not mounted until opened');
    assert.equal(tree.root.findAllByType('a').find((link) => link.props.href === '/domains/infrastructure/example.test') !== undefined, true);
    assert.ok(!text.includes('Move main DNS') && !text.includes('Ana DNS’i'), 'moving the main DNS is not part of this release');
    await act(async () => byProp('data-dns-advanced', '')[0].props.onToggle({ currentTarget: { open: true } }));
    assert.equal(tree.root.findAllByType('advanced-screen').length, 1);
    assert.ok(requests.every((request) => request.method === 'GET'));
  } finally { await unmount(); }
});

test('a verified failure is an alert in Turkish that names the backup as the page names it', async () => {
  const value = paired({ sentence: sentence('backup_not_serving', 'failed', { domain: 'example.test', backup: 'peer', main: 'this' }) });
  value.servers[1].answers = 'not_serving';
  await mount(value, { words: trScreens });
  try {
    const lead = byProp('data-dns-sentence', 'dns_overview_backup_not_serving')[0];
    assert.equal(lead.props.role, 'alert');
    const text = textOf();
    assert.ok(text.includes('İkinci sunucu (beta) yanıt veriyor ama example.test için değil'), text.slice(0, 400));
    assert.ok(text.includes('Bu sunucu (alpha) üzerinden'));
    assert.equal(byProp('data-dns-server', 'peer')[0].props['data-answers'], 'not_serving');
    assert.ok(!/dnsov\./.test(text));
  } finally { await unmount(); }
});

test('an overview that cannot be read is said, with nothing guessed; an unknown code keeps the server\'s sentence', async () => {
  await mount(() => json({ error: 'down' }, 503));
  try {
    assert.equal(byProp('data-dns-overview-state', 'unknown').length, 1);
    assert.equal(byProp('data-dns-server', 'this').length, 0);
  } finally { await unmount(); }
  await mount(paired({ sentence: { ...sentence('ok'), code: 'dns_overview_from_a_later_release', reason: 'A later sentence.' } }));
  try { assert.ok(textOf().includes('A later sentence.')); } finally { await unmount(); }
});

test('at an external provider: where the list is kept, no DNS software claimed, the records to set', async () => {
  await mount(paired({
    home: 'provider', sentence: sentence('provider_records', 'prerequisite', { domain: 'example.test', name: 'panel.example.test', type: 'A', value: '203.0.113.10' }),
    servers: [{ who: 'this', role: 'none', short_name: 'alpha', panel_name: 'panel.example.test', ipv4: '203.0.113.10', answers: 'not_applicable' }],
    names: [{ name: 'panel.example.test', kind: 'panel', type: 'A', server: 'this', expected: '203.0.113.10', seen: [], state: 'missing', where: 'provider' }],
    own_records_link: { where: 'provider' }, join: { state: 'unavailable', reason: 'external' },
  }));
  try {
    const text = textOf();
    assert.equal(byProp('data-dns-home', 'provider').length, 1);
    assert.ok(!text.includes('DNS software:'), 'no server claims DNS software');
    assert.ok(text.includes('Set panel.example.test (A) to 203.0.113.10'));
    assert.equal(byProp('data-dns-name', 'panel.example.test')[0].props['data-state'], 'missing');
    assert.equal(byProp('data-dns-join', 'external').length, 1);
  } finally { await unmount(); }
});

test('while setup waits for the second server the existing join card is offered; a running BIND says why there is none', async () => {
  await mount(paired({ join: { state: 'waiting' }, sentence: sentence('setup_running', 'waiting') }));
  try { assert.equal(tree.root.findAllByType('join-card').length, 1); } finally { await unmount(); }
  await mount(paired({ servers: [{ ...paired().servers[0], role: 'standalone' }], join: { state: 'unavailable', reason: 'standalone_bind' } }));
  try {
    assert.equal(tree.root.findAllByType('join-card').length, 0);
    assert.ok(textOf().includes('cannot gain a backup DNS server'));
    assert.ok(textOf().includes('answering · no backup'));
  } finally { await unmount(); }
});

const kept = () => paired({
  sentence: sentence('ok_mail_kept', 'ok', { domain: 'example.test', host: 'mail.example.test' }),
  mail: [
    { kind: 'mx', name: 'example.test', type: 'MX', expected: '10 mail.example.test', seen: ['10 mx.oldhost.example'], state: 'kept' },
    { kind: 'spf', name: 'example.test', type: 'TXT', expected: 'v=spf1 mx a:mail.example.test ~all', seen: ['v=spf1 mx a:mail.example.test ~all'], state: 'ok' },
  ],
  mail_receiver: { server: 'this', host: 'mail.example.test', short_name: 'alpha' },
  mx_move: { zone: 'example.test', to_host: 'mail.example.test', to: 'this' },
});
const RECORDS = '/api/v1/dns/infrastructure-zones/example.test/records';

test('Move mail: the consequence is confirmed in the page first, then the one MX is changed with ONE PUT', async () => {
  let puts = 0;
  await mount(kept(), { routes: { [RECORDS]: (init) => {
    if (init.method === 'PUT') { puts++; return json({ ok: true }); }
    return json({ records: [{ id: 7, name: 'example.test', type: 'MX', content: 'mx.oldhost.example', ttl: 3600, prio: 10 }, { id: 8, name: 'example.test', type: 'TXT', content: '"v=spf1"', ttl: 3600 }] });
  } } });
  try {
    await act(async () => button('Move mail to This server (alpha)').props.onClick());
    assert.equal(byProp('data-dns-mail-confirm', '').length, 1);
    assert.ok(textOf().includes('Create the mailboxes on This server (alpha) first'));
    assert.equal(requests.filter((request) => request.url === RECORDS).length, 0, 'nothing is read or sent before the confirmation');
    await act(async () => button('Move mail now').props.onClick());
    await settle();
    assert.equal(puts, 1);
    const put = requests.find((request) => request.method === 'PUT');
    assert.deepEqual(JSON.parse(put.body), { id: 7, content: 'mail.example.test', ttl: 3600, prio: 10, disabled: false });
    assert.equal(byProp('data-dns-mail-moved', '').length, 1);
  } finally { await unmount(); }
});

test('Move mail: several MX records are not chosen for the owner; a lost answer is never sent again', async () => {
  await mount(kept(), { routes: { [RECORDS]: () => json({ records: [{ id: 7, name: 'example.test', type: 'MX', content: 'mx1.oldhost.example', ttl: 3600, prio: 10 }, { id: 9, name: 'example.test', type: 'MX', content: 'mx2.oldhost.example', ttl: 3600, prio: 20 }] }) } });
  try {
    await act(async () => button('Move mail to').props.onClick());
    await act(async () => button('Move mail now').props.onClick());
    await settle();
    assert.equal(requests.filter((request) => request.method === 'PUT').length, 0);
    assert.ok(textOf().includes('has 2 MX records'));
  } finally { await unmount(); }
  await mount(kept(), { routes: { [RECORDS]: (init) => {
    if (init.method === 'PUT') throw new TypeError('connection dropped');
    return json({ records: [{ id: 7, name: 'example.test', type: 'MX', content: 'mx.oldhost.example', ttl: 3600, prio: 10 }] });
  } } });
  try {
    await act(async () => button('Move mail to').props.onClick());
    await act(async () => button('Move mail now').props.onClick());
    await settle(30);
    assert.equal(requests.filter((request) => request.method === 'PUT').length, 1, 'sent once');
    assert.ok(textOf().includes('it is not known whether the MX changed'));
    assert.ok(requests.filter((request) => request.url === '/api/v1/dns/overview').length >= 2, 'the overview is read again');
  } finally { await unmount(); }
});

test('the MX target helper only looks: none, many, already done, or the one record', () => {
  const mx = (id, content) => ({ id, name: 'example.test', type: 'MX', content, ttl: 300, prio: 10 });
  assert.deepEqual(lib.dnsOverviewMXTarget([], 'example.test', 'mail.example.test'), { none: 'none', count: 0 });
  assert.equal(lib.dnsOverviewMXTarget([mx(1, 'a.example'), mx(2, 'b.example')], 'example.test', 'mail.example.test').none, 'many');
  assert.equal(lib.dnsOverviewMXTarget([mx(1, 'mail.example.test')], 'example.test', 'mail.example.test').none, 'done');
  assert.equal(lib.dnsOverviewMXTarget([mx(1, 'old.example'), { ...mx(2, 'x.example'), name: 'sub.example.test' }], 'example.test', 'mail.example.test').record.id, 1);
});
