import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { remoteURL } from './fixtures/shared-layer.mjs';

// The owner's rule of 31 Oct 2026: no change may need a page refresh. A change
// names what it changed (`invalidate('domains')`) and every read on screen that
// shows that thing reads again. These tests are about the mechanism itself:
// who is asked again, who is not, and that it never sends a change.
//
// Sahibin 31 Eki 2026 kuralı: hiçbir değişiklik sayfa yenilemesi istemez. Bu
// testler mekanizmanın kendisini sınar: kim yeniden okur, kim okumaz ve hiçbir
// değişiklik göndermediği.

const { invalidate, readKeyCovers, useInvalidation, useRemote } = await import(remoteURL);
const originalFetch = globalThis.fetch;
globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const decode = (raw) => raw;
const json = (body) => new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });
const settle = () => act(async () => { await new Promise((resolve) => setTimeout(resolve, 15)); });

let tree;
let requests;
async function mount(element) {
  await act(async () => { tree = Renderer.create(element); });
  await settle();
}
async function unmount() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
}
function install() {
  requests = [];
  globalThis.fetch = async (url, init) => {
    requests.push({ url: String(url), method: init?.method ?? 'GET' });
    return json({ n: requests.length });
  };
}
const Reader = ({ url }) => { useRemote(url, decode); return null; };
const count = (url) => requests.filter((request) => request.url === url).length;

test('invalidate: every mounted read a name covers reads again, once each', async () => {
  install();
  try {
    await mount(React.createElement(React.Fragment, null,
      React.createElement(Reader, { url: '/api/v1/domains' }),
      React.createElement(Reader, { url: '/api/v1/dns/infrastructure-zones' }),
      // Two readers of one address still share one request.
      React.createElement(Reader, { url: '/api/v1/dashboard' }),
      React.createElement(Reader, { url: '/api/v1/dashboard' })));
    assert.equal(requests.length, 3, 'three addresses, one request each');
    requests.length = 0;
    let asked;
    await act(async () => { asked = invalidate('domains'); });
    await settle();
    assert.deepEqual([...asked].sort(), ['/api/v1/dashboard', '/api/v1/dns/infrastructure-zones', '/api/v1/domains']);
    assert.equal(count('/api/v1/domains'), 1);
    assert.equal(count('/api/v1/dns/infrastructure-zones'), 1);
    assert.equal(count('/api/v1/dashboard'), 1, 'two readers of one address: one request');
  } finally { await unmount(); globalThis.fetch = originalFetch; }
});

test('invalidate: an address the name does not cover is not read again', async () => {
  install();
  try {
    await mount(React.createElement(React.Fragment, null,
      React.createElement(Reader, { url: '/api/v1/domains' }),
      React.createElement(Reader, { url: '/api/v1/offered-probe' }),
      React.createElement(Reader, { url: '/api/v1/domains/7/databases' })));
    requests.length = 0;
    await act(async () => { invalidate('components'); });
    await settle();
    assert.equal(requests.length, 0, 'domains, databases and an unknown address are not components');
    await act(async () => { invalidate('databases'); });
    await settle();
    assert.deepEqual(requests.map((request) => request.url), ['/api/v1/domains/7/databases']);
  } finally { await unmount(); globalThis.fetch = originalFetch; }
});

test('invalidate: the names cover the addresses the owner rule lists', () => {
  assert.ok(readKeyCovers('domains', '/api/v1/domains'));
  assert.ok(readKeyCovers('domains', '/api/v1/dns/infrastructure-zones'));
  assert.ok(readKeyCovers('dns', '/api/v1/domains/12/dns/records'));
  assert.ok(readKeyCovers('dns', '/api/v1/domains/12/dnssec'));
  assert.ok(readKeyCovers('components', '/api/v1/managed-services'));
  assert.ok(readKeyCovers('components', '/api/v1/service/postgresql/clusters'));
  assert.ok(readKeyCovers('components', '/api/v1/firewall'));
  assert.ok(readKeyCovers('components', '/api/v1/repo?service_id=php-fpm'));
  assert.ok(readKeyCovers('mail', '/api/v1/domains/12/mail/accounts'));
  assert.ok(readKeyCovers('mail', '/api/v1/dashboard'));
  assert.ok(readKeyCovers('databases', '/api/v1/database-servers/3/databases'));
  assert.ok(readKeyCovers('databases', '/api/v1/domains/12/databases'));
  assert.ok(!readKeyCovers('databases', '/api/v1/domains/12/mail/accounts'));
  assert.ok(!readKeyCovers('mail', '/api/v1/domains'));
});

test('useInvalidation: a screen with its own read is told, once per invalidation, only for its names', async () => {
  install();
  const calls = [];
  const Listener = () => { useInvalidation(['mail', 'dns'], () => calls.push('read')); return null; };
  try {
    await mount(React.createElement(Listener));
    await act(async () => { invalidate('databases'); });
    assert.deepEqual(calls, [], 'another name does not reach it');
    await act(async () => { invalidate('mail'); });
    assert.deepEqual(calls, ['read']);
    // Two of its names in one call are one telling.
    await act(async () => { invalidate('mail', 'dns'); });
    assert.deepEqual(calls, ['read', 'read']);
    assert.equal(requests.length, 0, 'telling a screen sends no request of its own');
  } finally { await unmount(); globalThis.fetch = originalFetch; }
});

test('useInvalidation: the latest callback is the one called', async () => {
  install();
  const seen = [];
  const Listener = ({ label }) => { useInvalidation(['team'], () => seen.push(label)); return null; };
  try {
    await mount(React.createElement(Listener, { label: 'first' }));
    await act(async () => { tree.update(React.createElement(Listener, { label: 'second' })); });
    await act(async () => { invalidate('team'); });
    assert.deepEqual(seen, ['second']);
  } finally { await unmount(); globalThis.fetch = originalFetch; }
});

test('after unmount nothing is requested and nobody is told', async () => {
  install();
  const calls = [];
  const Both = () => { useRemote('/api/v1/domains', decode); useInvalidation(['domains'], () => calls.push('read')); return null; };
  await mount(React.createElement(Both));
  await unmount();
  requests.length = 0;
  try {
    let asked;
    await act(async () => { asked = invalidate('domains', 'dns', 'components', 'mail', 'databases', 'users', 'plans', 'team'); });
    await settle();
    assert.deepEqual(asked, []);
    assert.equal(requests.length, 0);
    assert.deepEqual(calls, []);
  } finally { globalThis.fetch = originalFetch; }
});

test('invalidate only reads: no request it makes is anything but GET', async () => {
  install();
  try {
    await mount(React.createElement(React.Fragment, null,
      ['/api/v1/domains', '/api/v1/dns/overview', '/api/v1/managed-services', '/api/v1/domains/4/mail/accounts',
        '/api/v1/database-servers', '/api/v1/users', '/api/v1/plans', '/api/v1/team-members']
        .map((url) => React.createElement(Reader, { key: url, url }))));
    requests.length = 0;
    await act(async () => { invalidate('domains', 'dns', 'components', 'mail', 'databases', 'users', 'plans', 'team'); });
    await settle();
    assert.ok(requests.length >= 8, 'every mounted read was asked again');
    assert.ok(requests.every((request) => request.method === 'GET'), JSON.stringify(requests.filter((request) => request.method !== 'GET')));
  } finally { await unmount(); globalThis.fetch = originalFetch; }
});
