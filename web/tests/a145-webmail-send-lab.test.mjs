// alpha.145 (alpha.144 lab): webmail could not send (D1); an alias's zone
// notice asked to make the change again while its record waited to be
// retried by itself (O1); the window of a failed PostgreSQL close kept its
// in-progress title above "Not completed" (O2).
// Web postası gönderemiyordu; takma ad kaydı beklerken bölge uyarısı
// değişikliği yeniden yapmayı istiyordu; başarısız kapatma penceresinin
// başlığı sürmekte olan işi söylüyordu.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import ts from 'typescript';
import { enScreens, trScreens } from './locale-catalogue.mjs';
import { compileSource, dataModule, reactURL, remoteURL } from './fixtures/shared-layer.mjs';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const dataURL = (text) => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`;
const compile = (path, replace = (s) => s) => replace(ts.transpileModule(read(path), { compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 } }).outputText);
const fill = (catalogue) => (key, vars) => catalogue[key] === undefined ? key : String(catalogue[key]).replace(/\{(\w+)\}/g, (all, name) => vars?.[name] ?? all);

test('D1: Problems now has a line for webmail that cannot send, worded with or without CelikPanel’s text beside the file', () => {
    const source = read('../src/components/ProblemsNow.tsx');
    assert.match(source, /'webmail_cannot_send',\n\];/, 'a kind this screen knows');
    const body = /export function problemBodyKey[\s\S]*?\n}\n/.exec(source)?.[0] ?? '';
    const problemBodyKey = new Function('MAIL_SIGNING_STATES', `return ${body.replace(/^export function problemBodyKey\(problem[^)]*\): TranslationKey/, 'function (problem)').replace(/ as TranslationKey/g, '')}`)([]);
    assert.equal(problemBodyKey({ kind: 'webmail_cannot_send', name: '/var/lib/celikpanel-webmail/config/config.inc.php.celikpanel-pending' }), 'pnow.webmail_cannot_send.body');
    assert.equal(problemBodyKey({ kind: 'webmail_cannot_send', name: '/var/lib/celikpanel-webmail/config/config.inc.php' }), 'pnow.webmail_cannot_send.body_no_text');
    for (const catalogue of [enScreens, trScreens]) {
        for (const key of ['title', 'body', 'body_no_text', 'action']) assert.ok(catalogue[`pnow.webmail_cannot_send.${key}`], key);
        assert.match(catalogue['pnow.webmail_cannot_send.body'], /\{other\}/);
        assert.match(catalogue['pnow.webmail_cannot_send.body_no_text'], /\{other\}.*tls:\/\/127\.0\.0\.1:587/);
        // The webmail profile says it too, never "installed and verified".
        assert.ok(catalogue['services.mailProfiles.code.webmail_cannot_send']);
    }
});

test('O1: while alias records wait, the zone’s notice says they are retried by themselves, not "make the change again"', async () => {
    const caps = await import(dataURL(compile('../src/lib/hostingCapabilities.ts', (s) => s.replace(/from ['"]\.\/remote['"]/, `from '${remoteURL}'`))));
    for (const state of ['pair_pending', 'switching', 'not_running']) {
        assert.equal(caps.heldAliasTodo(state, false), null, `${state}: nothing waits, the state's own words`);
        assert.equal(caps.heldAliasTodo(state, true), `dnsstate.${state}.todo_held`);
        for (const catalogue of [enScreens, trScreens]) {
            const held = catalogue[`dnsstate.${state}.todo_held`];
            assert.ok(held, `${state} held words`);
            assert.doesNotMatch(held, /make the change again|değişikliği yeniden yapın/);
            assert.match(held, /retried every hour|her saat yeniden denenir/);
        }
    }
    for (const state of ['secondary', 'port53_conflict', 'engine_conflict', 'none']) assert.equal(caps.heldAliasTodo(state, true), null);
    const manager = read('../src/components/DomainDNSManager.tsx');
    assert.match(manager, /aliasesWaiting: body\.aliases_waiting === true/);
    assert.match(manager, /todo: t\(heldAliasTodo\(dnsExplained\.state, zoneValue\?\.aliasesWaiting === true\) \?\? dnsExplained\.todo, dnsExplained\.vars\)/);
    // In the lab's words: the pair state with an alias waiting.
    const t = fill(enScreens);
    assert.match(t(caps.heldAliasTodo('pair_pending', true)), /published by themselves once it confirms/);
});

globalThis.__a145pg = { asked: [] };
const stub = dataModule(`
  import React from '${reactURL}';
  const s = globalThis.__a145pg;
  export const useI18n = () => ({ t: (key, vars) => (vars ? key + JSON.stringify(vars) : key), locale: 'en' });
  export const ShieldCheck = () => null;
  export function Card(props) { return React.createElement('section', { title: props.title }, props.children); }
  export function Button(props) { return React.createElement('button', { onClick: props.onClick, disabled: props.disabled }, props.children); }
  export function RemoteGate(props) { return props.children({ value: props.remote.value, stale: false }); }
  export function Guidance(props) { return React.createElement('div', null, props.action); }
  export const useRemote = (url, decode) => ({ remote: { state: 'known', value: decode(s.view) }, reading: false, retry: async () => {} });
  export const useRefreshEvery = () => {};
  export const useInvalidation = () => {};
  export const readApiError = async (res) => ({ message: 'refused', code: res.headers.get('x-code') || undefined });
  export const apiErrorText = (e) => e.message;
  export async function confirmAction(request) { s.asked.push(request); s.end = await request.action({ steps: () => {} }); return true; }
`);
const { PostgreSQLPublicAccessBlock, pgPublicEndTitle } = await import(compileSource('components/PostgreSQLPublicAccessBlock.tsx', () => stub));

test('O2: the window of a close or its undo ends with a title that follows the outcome', async () => {
    globalThis.__a145pg.view = {
        checked_at: '2026-11-22T10:00:00Z',
        open: [{ database_id: 7, name: 'shop', port: 5432, state: 'open', owner: 'shop_owner', acl: '', found: ['other_roles'], other_roles: ['stranger'], roles: ['shop_owner'] }],
        closed: [], unknown: [],
    };
    for (const [status, code, state, title] of [
        [200, '', 'done', 'pgpub.end.closed'],
        [409, 'POSTGRESQL_PUBLIC_FAILED', 'blocked', 'pgpub.end.notClosed'],
        [500, 'POSTGRESQL_PUBLIC_FAILED', 'failed', 'pgpub.end.notClosed'],
        [502, 'POSTGRESQL_PUBLIC_UNKNOWN', 'unknown', 'pgpub.end.unknown'],
    ]) {
        globalThis.fetch = async () => new Response('{}', { status, headers: code ? { 'x-code': code } : {} });
        let tree;
        await act(async () => { tree = Renderer.create(React.createElement(PostgreSQLPublicAccessBlock)); });
        const close = tree.root.findAll((node) => node.type === 'button' && JSON.stringify(node.props.children) === '"pgpub.close"');
        await act(async () => {
            close[0].props.onClick();
            await new Promise((resolve) => setTimeout(resolve, 20));
        });
        const end = globalThis.__a145pg.end;
        assert.equal(end.state, state, `HTTP ${status}`);
        assert.equal(end.title, `${title}{"name":"shop"}`, `HTTP ${status}: the title follows the outcome`);
        assert.match(globalThis.__a145pg.asked.at(-1).progressTitle, /^pgpub\.progress/, 'the running title is unchanged');
        await act(async () => tree.unmount());
    }
    assert.equal(pgPublicEndTitle('undo', 'done'), 'pgpub.undo.end.opened');
    assert.equal(pgPublicEndTitle('undo', 'failed'), 'pgpub.undo.end.notOpened');
    assert.equal(pgPublicEndTitle('undo', 'blocked'), 'pgpub.undo.end.notOpened');
    assert.equal(pgPublicEndTitle('undo', 'unknown'), 'pgpub.undo.end.unknown');
    for (const catalogue of [enScreens, trScreens]) {
        for (const key of ['pgpub.end.closed', 'pgpub.end.notClosed', 'pgpub.end.unknown', 'pgpub.undo.end.opened', 'pgpub.undo.end.notOpened', 'pgpub.undo.end.unknown']) {
            assert.match(catalogue[key] ?? '', /\{name\}/, key);
        }
    }
});
