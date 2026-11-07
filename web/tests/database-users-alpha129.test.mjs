// The users of one domain database (alpha.129): each with its access and a
// new password; more users with read-only or read-and-write access, added and
// removed in the progress window with the server's steps. The user made with
// the database is never offered for removal. A new password is sent once and
// never shown back; the last line says what happens to the sites that use
// the old one.
// Bir veritabanının kullanıcıları: erişim, yeni parola, ek kullanıcı ekleme
// ve kaldırma; veritabanıyla oluşturulan kullanıcı kaldırılmaz.
import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';

const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null;
  export const useNavigate = () => () => true;
  export const KeyRound = () => null;
  export const Trash2 = () => null;
  export const UserPlus = () => null;
  export const Plus = () => null;
  export const UserRound = () => null;
  const words = { t: (key, vars) => (vars ? key + JSON.stringify(vars) : key), locale: 'en' };
  export const useI18n = () => words;
  export const useDataToolsWords = () => words;
  export const showToast = () => {};
`);
const shared = sharedLayer(stub);
const passwordURL = shared.compile('components/DatabaseUserPassword.tsx');
const { DatabaseUsers } = await import(shared.compile('components/DatabaseUsers.tsx', { '/DatabaseUserPassword': passwordURL }));

const users = [
    { id: 11, username: 'shop_db_user', access: 'owner' },
    { id: 12, username: 'shop_db_reader', access: 'read' },
];
const STRONG = 'Str0ng-Passw0rd!';
const originalFetch = globalThis.fetch;
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
let tree;
let calls;
let answer;
async function mount(props = {}) {
    calls = [];
    globalThis.operationSteps = [];
    globalThis.operationEnds = [];
    globalThis.fetch = (url, options = {}) => {
        calls.push({ url: String(url), method: options.method || 'GET', body: options.body });
        if (String(url).startsWith('/api/v1/operation-progress/')) return Promise.resolve(new Response('{}', { status: 404 }));
        return Promise.resolve(answer());
    };
    await act(async () => {
        tree = Renderer.create(React.createElement(DatabaseUsers, { domainId: 4, databaseId: 9, databaseName: 'shop_db', users, readOnly: false, ...props }));
        await settled();
    });
}
async function cleanup() {
    if (tree) await act(async () => tree.unmount());
    tree = undefined;
    globalThis.fetch = originalFetch;
    delete globalThis.confirm;
}
const words = (node) => (typeof node === 'string' ? node : (node?.children ?? []).map(words).join(' '));
const button = (key) => tree.root.findAll((node) => node.type === 'button' && (words(node).includes(key) || String(node.props['aria-label'] ?? '').includes(key)))[0];
const changes = () => calls.filter((call) => call.method !== 'GET');

test('each user shows its access; the user made with the database cannot be removed, an added one can', async () => {
    answer = () => Response.json({});
    await mount();
    try {
        const rows = tree.root.findAll((node) => node.type === 'li');
        assert.equal(rows.length, 2);
        assert.ok(words(rows[0]).includes('dbu.accessOwner'));
        assert.ok(words(rows[1]).includes('dbu.accessRead'));
        const removeLabels = tree.root.findAll((node) => node.type === 'button' && String(node.props['aria-label'] ?? '').startsWith('dbu.removeUser'));
        assert.equal(removeLabels.length, 1);
        assert.ok(String(removeLabels[0].props['aria-label']).includes('shop_db_reader'));
        // Read only: nothing to press.
        await act(async () => tree.update(React.createElement(DatabaseUsers, { domainId: 4, databaseId: 9, databaseName: 'shop_db', users, readOnly: true })));
        assert.equal(tree.root.findAll((node) => node.type === 'button').length, 0);
    } finally {
        await cleanup();
    }
});

test('a new read-only user: the button waits for a valid name and a strong password, then the server makes it with its steps', async () => {
    answer = () => Response.json({ id: 13, username: 'shop_db_report', access: 'read' });
    await mount();
    try {
        await act(async () => button('dbu.add').props.onClick());
        const start = () => button('dbu.addStart');
        assert.equal(start().props.disabled, true);
        const nameInput = tree.root.findAll((node) => node.type === 'input' && node.props.placeholder === 'reader')[0];
        await act(async () => nameInput.props.onChange({ target: { value: 'user' } }));
        const passwordInput = tree.root.findAll((node) => node.type === 'input' && node.props.autoComplete === 'new-password')[0];
        await act(async () => passwordInput.props.onChange({ target: { value: STRONG } }));
        // "user" is the name of the user made with the database.
        assert.equal(start().props.disabled, true);
        await act(async () => nameInput.props.onChange({ target: { value: 'Report' } }));
        assert.equal(start().props.disabled, false, 'typed in capitals, kept in lowercase');
        await act(async () => {
            await start().props.onClick();
            await settled();
        });
        const sent = changes();
        assert.equal(sent.length, 1);
        assert.equal(sent[0].url, '/api/v1/domains/4/databases/9/users');
        assert.equal(sent[0].method, 'POST');
        assert.deepEqual(JSON.parse(sent[0].body), { name: 'report', access: 'read', password: STRONG });
        const end = globalThis.operationEnds.at(-1);
        assert.equal(end.state, 'done');
        assert.ok(end.message.startsWith('dbu.addedRead') && end.message.includes('shop_db_report'), end.message);
        assert.deepEqual(globalThis.operationSteps.at(-1).map((step) => step.id), ['check', 'create', 'records']);
        assert.ok(!JSON.stringify(tree.toJSON()).includes(STRONG), 'the password is not left on the page');
    } finally {
        await cleanup();
    }
});

test('removing an added user asks first, says the database stays, and sends one DELETE', async () => {
    answer = () => Response.json({ status: 'removed' });
    await mount();
    try {
        let asked = '';
        globalThis.confirm = (message) => { asked = message; return true; };
        await act(async () => {
            await button('dbu.removeUser').props.onClick();
            await settled();
        });
        assert.ok(asked.startsWith('dbu.removeConfirm') && asked.includes('shop_db_reader'), asked);
        assert.deepEqual(changes().map((call) => `${call.method} ${call.url}`), ['DELETE /api/v1/domains/4/databases/9/users/12']);
        assert.equal(globalThis.operationEnds.at(-1).state, 'done');
    } finally {
        await cleanup();
    }
});

test('a new password goes to the user, is not shown back, and the end names the application that got it', async () => {
    answer = () => Response.json({ success: true, sessions_ended: true, app_domain: 'shop.example', app_kind: 'wordpress', app_updated: true });
    await mount();
    try {
        await act(async () => button('dbu.newPasswordFor').props.onClick());
        const passwordInput = tree.root.findAll((node) => node.type === 'input' && node.props.autoComplete === 'new-password')[0];
        await act(async () => passwordInput.props.onChange({ target: { value: STRONG } }));
        await act(async () => {
            await button('dbu.change').props.onClick();
            await settled();
        });
        const sent = changes();
        assert.equal(sent.length, 1);
        assert.equal(sent[0].url, '/api/v1/domains/4/databases/9/users/11/password');
        assert.deepEqual(JSON.parse(sent[0].body), { password: STRONG });
        const end = globalThis.operationEnds.at(-1);
        assert.ok(end.message.startsWith('dbu.doneApp') && end.message.includes('WordPress') && end.message.includes('shop.example'), end.message);
        assert.equal(tree.root.findAll((node) => node.props['data-database-user-password-form']).length, 0, 'the form closed');
    } finally {
        await cleanup();
    }
});

test('a new password with no known application says the sites must be given it by hand', async () => {
    answer = () => Response.json({ success: true, sessions_ended: true });
    await mount();
    try {
        await act(async () => button('dbu.newPasswordFor').props.onClick());
        const passwordInput = tree.root.findAll((node) => node.type === 'input' && node.props.autoComplete === 'new-password')[0];
        await act(async () => passwordInput.props.onChange({ target: { value: STRONG } }));
        await act(async () => {
            await button('dbu.change').props.onClick();
            await settled();
        });
        assert.ok(globalThis.operationEnds.at(-1).message.startsWith('dbu.done{'), globalThis.operationEnds.at(-1).message);
    } finally {
        await cleanup();
    }
});
