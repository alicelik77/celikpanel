import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';

import { refusalActor } from '../src/lib/apiError.ts';
import { operationEndLine } from '../src/lib/confirmAction.ts';
import { enProblemsScreens } from '../src/i18n/screens/problems/en.ts';
import { trProblemsScreens } from '../src/i18n/screens/problems/tr.ts';
import { enDataTools as enData } from '../src/i18n/dataTools/en.ts';
import { trDataTools as trData } from '../src/i18n/dataTools/tr.ts';
import { dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';

// alpha.149 lab W1: the progress window said "Waiting for someone to act" over
// a reason that names the server's administrator. One rule (lib/apiError.ts
// refusalActor): the heading names the administrator for a refusal about an
// existing account whose reason names them, read by the administrator; making
// a new account, and every other viewer, keep the general heading (review of
// PR #374, F6).
// Neden yöneticiyi adlandırıyorsa ve hesap mevcutsa yöneticiye başlık da
// adlandırır; yeni hesapta ve diğer rollerde genel başlık kalır.
const naming = ['privileged', 'member', 'unreadable', 'unsupported_engine', 'owns_objects'];
const refused = (reason) => ({ message: '', code: 'DATABASE_ACCOUNT_REFUSED', ...(reason ? { reason } : {}) });

test('the rule: an existing account, a reason that names the administrator, the administrator reading', () => {
    for (const reason of naming) {
        assert.equal(refusalActor(refused(reason), 'admin', 'existing'), 'administrator', reason);
        // The reason's own words name the administrator in both languages.
        assert.match(enData[`err.DATABASE_ACCOUNT_REFUSED.${reason}`], /server's administrator/, reason);
        assert.match(trData[`err.DATABASE_ACCOUNT_REFUSED.${reason}`], /[Ss]unucunun yöneticisi/, reason);
        // A new account: the person's next step may be another name.
        assert.equal(refusalActor(refused(reason), 'admin', 'new'), undefined, reason);
        // Everyone else reads the same sentence, which does not tell them to
        // ask their provider: their heading stays the general one.
        for (const role of ['reseller', 'customer', 'additional_user', undefined]) {
            assert.equal(refusalActor(refused(reason), role, 'existing'), undefined, `${reason} ${role}`);
        }
    }
    // The general sentence (an unknown reason) names them too.
    assert.equal(refusalActor(refused(), 'admin', 'existing'), 'administrator');
    assert.match(enData['err.DATABASE_ACCOUNT_REFUSED'], /server's administrator/);
    // Sentences that name no one keep the general heading.
    for (const reason of ['absent', 'system_database']) {
        assert.equal(refusalActor(refused(reason), 'admin', 'existing'), undefined, reason);
        assert.doesNotMatch(enData[`err.DATABASE_ACCOUNT_REFUSED.${reason}`], /administrator/, reason);
        assert.doesNotMatch(trData[`err.DATABASE_ACCOUNT_REFUSED.${reason}`], /yönetici/, reason);
    }
    assert.equal(refusalActor({ message: 'x', code: 'OTHER' }, 'admin', 'existing'), undefined);
    assert.equal(refusalActor(null, 'admin', 'existing'), undefined);
});

test('the heading names the administrator in English and Turkish; the general one stays', () => {
    assert.equal(enProblemsScreens['operation.blocked.administrator'], "Waiting for the server's administrator to act");
    assert.equal(trProblemsScreens['operation.blocked.administrator'], 'Sunucunun yöneticisinin bir şey yapması bekleniyor');
    assert.equal(enProblemsScreens['operation.blocked'], 'Waiting for someone to act');
    assert.equal(trProblemsScreens['operation.blocked'], 'Birinin bir şey yapması bekleniyor');
    const say = (key) => enProblemsScreens[key];
    assert.equal(operationEndLine({ state: 'blocked', reason: 'r', actor: 'administrator' }, say), "Waiting for the server's administrator to act");
    assert.equal(operationEndLine({ state: 'blocked', reason: 'r' }, say), 'Waiting for someone to act');
    // Only a blocked ending names the actor; a failure keeps its own word.
    assert.equal(operationEndLine({ state: 'failed', reason: 'r', actor: 'administrator' }, say), 'Not completed');
    assert.equal(operationEndLine({ state: 'unknown', reason: 'r', actor: 'administrator' }, say), 'The result is not known');
});

// The flows that show the refusal, mounted: adding, removing and a new
// password on a domain database's users, as the administrator and as a
// customer. The ending each flow gives the window is read as it is.
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
  export const useAuth = () => ({ role: globalThis.viewerRole });
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
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
const words = (node) => (typeof node === 'string' ? node : (node?.children ?? []).map(words).join(' '));

// The end of one flow, the server refusing it with `reason`.
async function endOf(flow, role, reason) {
    const originalFetch = globalThis.fetch;
    globalThis.viewerRole = role;
    globalThis.operationEnds = [];
    globalThis.fetch = (url) => (String(url).startsWith('/api/v1/operation-progress/')
        ? Promise.resolve(new Response('{}', { status: 404 }))
        : Promise.resolve(Response.json({ error: 'refused', code: 'DATABASE_ACCOUNT_REFUSED', reason, vars: { user: 'shop_db_reader' } }, { status: 409 })));
    globalThis.confirm = () => true;
    let tree;
    try {
        await act(async () => {
            tree = Renderer.create(React.createElement(DatabaseUsers, { domainId: 4, databaseId: 9, databaseName: 'shop_db', users, readOnly: false }));
            await settled();
        });
        const button = (key) => tree.root.findAll((node) => node.type === 'button' && (words(node).includes(key) || String(node.props['aria-label'] ?? '').includes(key)))[0];
        const input = (match) => tree.root.findAll((node) => node.type === 'input' && match(node.props))[0];
        if (flow === 'add') {
            await act(async () => button('dbu.add').props.onClick());
            await act(async () => input((props) => props.placeholder === 'reader').props.onChange({ target: { value: 'report' } }));
            await act(async () => input((props) => props.autoComplete === 'new-password').props.onChange({ target: { value: STRONG } }));
            await act(async () => { await button('dbu.addStart').props.onClick(); await settled(); });
        } else if (flow === 'remove') {
            await act(async () => { await button('dbu.removeUser').props.onClick(); await settled(); });
        } else {
            await act(async () => button('dbu.newPasswordFor').props.onClick());
            await act(async () => input((props) => props.autoComplete === 'new-password').props.onChange({ target: { value: STRONG } }));
            await act(async () => { await button('dbu.change').props.onClick(); await settled(); });
        }
        const end = globalThis.operationEnds.at(-1);
        assert.ok(end, `${flow}: no ending`);
        return end;
    } finally {
        if (tree) await act(async () => tree.unmount());
        globalThis.fetch = originalFetch;
        delete globalThis.confirm;
    }
}

test('removing a user and its new password name the administrator for the administrator only; adding never', async () => {
    for (const flow of ['remove', 'password']) {
        const admin = await endOf(flow, 'admin', 'owns_objects');
        assert.equal(admin.state, 'blocked', flow);
        assert.equal(admin.actor, 'administrator', flow);
        assert.equal(operationEndLine(admin, (key) => enProblemsScreens[key]), "Waiting for the server's administrator to act", flow);
        // The reason is the refusal's own sentence, unchanged.
        assert.ok(admin.reason.startsWith('err.DATABASE_ACCOUNT_REFUSED.owns_objects'), admin.reason);
        for (const role of ['customer', 'reseller']) {
            const other = await endOf(flow, role, 'owns_objects');
            assert.equal(other.state, 'blocked', `${flow} ${role}`);
            assert.equal(other.actor, undefined, `${flow} ${role}`);
            assert.equal(other.reason, admin.reason, `${flow} ${role}: the same words as today`);
            assert.equal(operationEndLine(other, (key) => enProblemsScreens[key]), 'Waiting for someone to act', `${flow} ${role}`);
        }
        // A reason that names no one keeps the general heading.
        assert.equal((await endOf(flow, 'admin', 'absent')).actor, undefined, flow);
    }
    // A new account: the general heading, for the administrator too.
    const add = await endOf('add', 'admin', 'privileged');
    assert.equal(add.state, 'blocked');
    assert.equal(add.actor, undefined);
});
