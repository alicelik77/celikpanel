// PostgreSQL databases every account of the server can enter (security lane
// S1b, 20 Nov 2026): the PostgreSQL page lists what CelikPanel left open with
// what it found and what closing changes beside the one button that closes
// it (the panel's own question, never the browser's), and what it closed with
// when, by whom, the undo and the plain statements of the undo. Problems now
// has the line and its words in both languages.
// Her hesabın girebildiği eski PostgreSQL veritabanları: bulunan, kapatma ve
// geri alma bir arada; Sorunlar listesinde satırı. Düzeltme turu: izleme,
// başarısız kapatma, yeniden açılanlar, Yeniden oku.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, dataModule, reactURL } from './fixtures/shared-layer.mjs';
import { enPgPublicScreens } from '../src/i18n/screens/pgPublic/en.ts';
import { trPgPublicScreens } from '../src/i18n/screens/pgPublic/tr.ts';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');

const view = {
    checked_at: '2026-11-20T10:00:00Z',
    open: [{
        database_id: 7, name: 'shop', port: 5432, state: 'open', owner: 'shop_owner', acl: '',
        found: ['other_roles', 'not-a-reason'], other_roles: ['stranger'], roles: ['shop_owner', 'stranger'],
        grants: [{ role: 'reader' }, { role: 'writer', temporary: true }],
    }],
    closed: [{
        database_id: 8, name: 'blog', port: 5433, state: 'closed', by: 'panel', at: '2026-11-20T09:00:00Z',
        undo_statements: ['GRANT CONNECT, TEMPORARY ON DATABASE "blog" TO PUBLIC;', 'REVOKE CONNECT ON DATABASE "blog" FROM "reader";'],
    }],
    unknown: [],
};

globalThis.__pgpub = { asked: [], sent: [] };
const stub = dataModule(`
  import React from '${reactURL}';
  const s = globalThis.__pgpub;
  export const useI18n = () => ({ t: (key, vars) => (vars ? key + JSON.stringify(vars) : key), locale: 'en' });
  export const ShieldCheck = () => null;
  export function Card(props) { return React.createElement('section', { title: props.title }, props.children); }
  export function Button(props) { return React.createElement('button', { onClick: props.onClick, disabled: props.disabled }, props.children); }
  export function RemoteGate(props) { return props.children({ value: props.remote.value, stale: false }); }
  export function Guidance(props) {
    return React.createElement('div', { 'data-guidance': props.tone }, ['title', 'why', 'who', 'next', 'then'].map((row) => React.createElement('p', { key: row }, row + '=' + props[row])), props.action);
  }
  export const useRemote = (url, decode) => ({ remote: { state: 'known', value: decode(s.view) }, reading: false, retry: async () => { s.reread = (s.reread || 0) + 1; } });
  export const useRefreshEvery = () => {};
  export const useInvalidation = () => {};
  export const readApiError = async (res) => ({ message: 'refused', code: res.headers.get('x-code') || undefined });
  export const apiErrorText = (e) => e.message;
  export async function confirmAction(request) { s.asked.push(request); s.end = await request.action(); return true; }
`);
const blockURL = compileSource('components/PostgreSQLPublicAccessBlock.tsx', () => stub);
const { PostgreSQLPublicAccessBlock, decodePGPublic } = await import(blockURL);

test('an answer of another shape is not a list; unknown reasons are left out', () => {
    assert.throws(() => decodePGPublic({ open: [] }));
    const decoded = decodePGPublic(view);
    assert.deepEqual(decoded.open[0].found, ['other_roles']);
    assert.deepEqual(decoded.closed[0].grants, []);
});

test('an open database: what was found, what closing gives, the one confirmed close', async () => {
    globalThis.__pgpub.view = view;
    const calls = [];
    globalThis.fetch = async (url, init) => { calls.push({ url, body: JSON.parse(init.body) }); return new Response('{"status":"closed"}', { status: 200 }); };
    let tree;
    await act(async () => { tree = Renderer.create(React.createElement(PostgreSQLPublicAccessBlock)); });
    const text = JSON.stringify(tree.toJSON());
    assert.match(text, /pgpub\.found\.other_roles\{\\"roles\\":\\"stranger\\"\}/);
    assert.match(text, /pgpub\.gives\{\\"users\\":\\"reader, pgpub\.user\.write/);
    assert.match(text, /pgpub\.closed\.panel/);
    assert.match(text, /GRANT CONNECT, TEMPORARY ON DATABASE \\"blog\\" TO PUBLIC;/);
    const close = tree.root.findAll((node) => node.type === 'button' && JSON.stringify(node.props.children) === '"pgpub.close"');
    assert.equal(close.length, 1);
    await act(async () => {
        close[0].props.onClick();
        await new Promise((resolve) => setTimeout(resolve, 20));
    });
    const asked = globalThis.__pgpub.asked.at(-1);
    assert.match(asked.title, /pgpub\.confirm\.title/);
    assert.ok(asked.lost.some((line) => line.includes('stranger')), 'the accounts that lose their way in are named before the close');
    assert.deepEqual(calls, [{ url: '/api/v1/admin/postgresql/public-access', body: { action: 'close', database_id: 7, acl: '', roles: ['shop_owner', 'stranger'] } }]);
    assert.equal(globalThis.__pgpub.end.state, 'done');
    assert.ok(globalThis.__pgpub.reread >= 1, 'the list reads again by itself');
    await act(async () => tree.unmount());
});

// alpha.143 fix round: the watch with its dates, a failed close with its
// reason, a result not known, what every account can do, the sessions named
// before a close, a database an administrator opened again, Read again.
test('watched, failed, unknown and opened-again databases say what holds and who acts', async () => {
    globalThis.__pgpub.view = {
        checked_at: '2026-11-22T10:00:00Z',
        open: [
            { database_id: 1, name: 'watched', port: 5432, public_connect: true, public_temporary: true, found: ['observing'], watch_since: '2026-11-22T01:00:00Z', closes_at: '2026-11-23T01:00:00Z' },
            { database_id: 2, name: 'failed', port: 5432, public_connect: true, found: ['close_failed', 'missing_users'], failed_at: '2026-11-22T02:00:00Z', failure: 'role gone does not exist', missing: ['gone'] },
            { database_id: 3, name: 'used', port: 5432, public_temporary: true, found: ['unknown_result', 'other_sessions', 'replication_roles'], sessions: ['reporting', 'subscriber'], sessions_now: ['reporting'], replication: ['subscriber'], roles: [] },
        ],
        closed: [],
        reopened: [{ database_id: 4, name: 'again', port: 5432, state: 'reopened', by: 'administrator', at: '2026-11-21T12:00:00Z', public_connect: true, public_temporary: true }],
        unknown: [],
    };
    const calls = [];
    globalThis.fetch = async (url, init) => {
        calls.push(url);
        if (init?.method === 'POST') return new Response('{}', { status: 502, headers: { 'x-code': 'POSTGRESQL_PUBLIC_UNKNOWN' } });
        return new Response('{}', { status: 200 });
    };
    let tree;
    await act(async () => { tree = Renderer.create(React.createElement(PostgreSQLPublicAccessBlock)); });
    const text = JSON.stringify(tree.toJSON());
    assert.match(text, /pgpub\.found\.observing\{\\"since\\":\\"[^\\"]+\\",\\"until\\":\\"[^\\"]+\\"\}/, 'a watched database says since when and until when');
    assert.match(text, /pgpub\.found\.close_failed\{[^}]*role gone does not exist/);
    assert.match(text, /pgpub\.found\.missing_users\{\\"roles\\":\\"gone\\"\}/);
    assert.match(text, /pgpub\.found\.unknown_result/);
    assert.match(text, /pgpub\.found\.replication_roles\{\\"roles\\":\\"subscriber\\"\}/);
    assert.match(text, /pgpub\.reach\.both/);
    assert.match(text, /pgpub\.reach\.connect/);
    assert.match(text, /pgpub\.reach\.temporary/);
    assert.match(text, /pgpub\.reopened\.title/);
    assert.match(text, /pgpub\.reopened\.at/);
    assert.match(text, /pgpub\.readAt/);
    // The sessions at the last reading are named in the confirmation; an
    // unknown answer is said as unknown.
    const closes = tree.root.findAll((node) => node.type === 'button' && JSON.stringify(node.props.children) === '"pgpub.close"');
    assert.equal(closes.length, 4, 'every open and opened-again database has its Close');
    await act(async () => {
        closes[2].props.onClick();
        await new Promise((resolve) => setTimeout(resolve, 20));
    });
    const asked = globalThis.__pgpub.asked.at(-1);
    assert.ok(asked.lost.some((line) => line.startsWith('pgpub.confirm.sessions') && line.includes('reporting')), 'the sessions now are named before the close');
    assert.equal(globalThis.__pgpub.end.state, 'unknown');
    // Read again asks for one scan.
    const reread = tree.root.findAll((node) => node.type === 'button' && JSON.stringify(node.props.children) === '"pgpub.reread"');
    await act(async () => {
        reread[0].props.onClick();
        await new Promise((resolve) => setTimeout(resolve, 20));
    });
    assert.ok(calls.includes('/api/v1/admin/postgresql/public-access?fresh=1'));
    await act(async () => tree.unmount());
});

test('nothing open, closed, opened again or unread: the block is not drawn', async () => {
    globalThis.__pgpub.view = { open: [], closed: [], reopened: [], unknown: [] };
    let tree;
    await act(async () => { tree = Renderer.create(React.createElement(PostgreSQLPublicAccessBlock)); });
    assert.equal(tree.toJSON(), null);
    await act(async () => tree.unmount());
});

test('no browser dialogue; the words exist in both languages; Problems now has the line', () => {
    const source = read('../src/components/PostgreSQLPublicAccessBlock.tsx');
    assert.doesNotMatch(source, /window\.(confirm|alert)|\balert\(/);
    assert.deepEqual(Object.keys(trPgPublicScreens).sort(), Object.keys(enPgPublicScreens).sort());
    const problems = read('../src/components/ProblemsNow.tsx');
    assert.match(problems, /'postgresql_databases_open'/);
    for (const lang of ['en', 'tr']) {
        // The Problems-now lines have their own words part since alpha.142.
        const problemsNow = read(`../src/i18n/screens/problemsNow/${lang}.ts`);
        for (const part of ['title', 'body', 'action']) assert.match(problemsNow, new RegExp(`'pnow\\.postgresql_databases_open\\.${part}'`));
    }
    assert.match(read('../src/components/PostgreSQLManagement.tsx'), /PostgreSQLPublicAccessBlock/);
});
