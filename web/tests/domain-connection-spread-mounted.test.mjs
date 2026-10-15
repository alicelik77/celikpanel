import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { sharedLayer } from './fixtures/shared-layer.mjs';
import { en } from '../src/i18n/en.ts';
import { tr } from '../src/i18n/tr.ts';
import { enScreens } from '../src/i18n/screens/en.ts';
import { trScreens } from '../src/i18n/screens/tr.ts';

// A domain's "DNS update is spreading" (owner rule 2026-10-14, audit row 35).
// The card is mounted with the real shared remote-state layer; only fetch, the
// timers and the tab's visibility are scripted. What is held:
//
//   - while the change spreads and the tab is visible the card checks again by
//     itself every 30 s with the same GET as "Check again", says so, and says
//     when it last checked (the server's stamp of the answer);
//   - after 60 automatic checks (30 minutes of a visible page) it stops and
//     says plainly that the change was still spreading at the last check, that
//     this is not a failure, and what the owner can do;
//   - a hidden tab checks nothing; a change that has stopped spreading is not
//     checked again; nothing but GET is sent.
//
// Alan adinin "DNS degisikligi yayiliyor" durumu: kart 30 sn'de bir yalniz GET
// ile kendisi kontrol eder, 30 dakika sonra durur ve bunu soyler; gizli sekmede
// kontrol etmez.
const require = createRequire(import.meta.url);
const reactURL = pathToFileURL(require.resolve('react')).href;
const url = text => 'data:text/javascript;base64,' + Buffer.from(text).toString('base64');
const stub = url(`
import React from '${reactURL}';
export const useI18n = () => ({ t: (key, vars = {}) => {
    let text = globalThis.spreadCatalog[key] ?? key;
    for (const [name, value] of Object.entries(vars)) text = text.replaceAll('{' + name + '}', String(value));
    return text;
}, locale: globalThis.spreadLocale });
export const useNavigate = () => () => true;
export const showToast = () => {};
export const StatusDot = () => null, HelpButton = () => null;
export const Globe = () => null, ShieldCheck = () => null, Copy = () => null, AlertTriangle = () => null, RefreshCw = () => null, Check = () => null;
`);
const shared = sharedLayer(stub);
const { DomainConnection, DOMAIN_SPREAD_RECHECK_MS, DOMAIN_SPREAD_AUTO_CHECKS } = await import(shared.compile('components/DomainConnection.tsx'));

let timers = [];
globalThis.document = Object.assign(new EventTarget(), { visibilityState: 'visible' });
globalThis.window = {
    setTimeout: (fn, ms) => { const timer = { fn, ms, live: true }; timers.push(timer); return timer; },
    clearTimeout: timer => { if (timer && typeof timer === 'object') timer.live = false; },
};
const originalFetch = globalThis.fetch;
const PATH = '/api/v1/domains/1/connection';
const FIRST = Date.parse('2026-10-14T10:00:00Z');
let tree, calls, answers, spreadingAnswers;
const answer = (index, spreading) => ({
    domain: 'example.com', server_ip: '192.0.2.4', nameservers: ['ns1.example.net', 'ns2.example.net'],
    live_nameservers: ['ns1.example.net', 'ns2.example.net'], live_ips: ['192.0.2.4'], status: 'delegated', ssl_ready: true,
    propagation_pending: spreading,
    resolver_observations: spreading ? [
        { resolver: '1.1.1.1', nameservers: ['ns1.example.net'], ips: ['192.0.2.4'], status: 'delegated', ssl_ready: true },
        { resolver: '8.8.8.8', nameservers: ['ns.old.example'], ips: ['198.51.100.7'], status: 'elsewhere', ssl_ready: false },
    ] : [],
    glue_needed: false, nameservers_usable: true, dns_management_mode: 'local',
    // The server's own stamp of each answer, 30 s apart.
    checked_at: new Date(FIRST + index * 30_000).toISOString(),
});
const settle = async () => { for (let round = 0; round < 6; round++) await act(async () => { await new Promise(resolve => setImmediate(resolve)); }); };
const pollTimers = () => timers.filter(timer => timer.live && timer.ms === DOMAIN_SPREAD_RECHECK_MS);
const firePoll = async () => { const due = pollTimers(); for (const timer of due) timer.live = false; await act(async () => { for (const timer of due) timer.fn(); }); await settle(); return due.length; };
const textOf = node => node == null ? '' : typeof node === 'string' ? node : Array.isArray(node) ? node.map(textOf).join('') : textOf(node.children);
const page = () => textOf(tree.toJSON());
const timeOf = (locale, index) => new Date(FIRST + index * 30_000).toLocaleTimeString(locale === 'tr' ? 'tr-TR' : 'en-US');

async function mount(locale, catalog, stillSpreading = () => true) {
    globalThis.spreadCatalog = catalog;
    globalThis.spreadLocale = locale;
    globalThis.document.visibilityState = 'visible';
    timers = []; calls = []; answers = 0;
    globalThis.fetch = async (path, options = {}) => {
        calls.push({ path: String(path), method: options.method || 'GET' });
        if (String(path) !== PATH) throw new Error('unexpected request ' + path);
        const index = answers++;
        return Response.json(answer(index, stillSpreading(index)));
    };
    await act(async () => { tree = Renderer.create(React.createElement(DomainConnection, { domainId: 1, domainName: 'example.com' })); });
    await settle();
}
async function unmount() { if (tree) await act(async () => tree.unmount()); tree = null; globalThis.fetch = originalFetch; }

for (const [locale, shell, screens] of [['en', en, enScreens], ['tr', tr, trScreens]]) {
    const c = { ...shell, ...screens };
    test(`${locale}: a spreading change is checked again every 30 s by itself, with GET only, and the checks stop after 30 minutes`, async () => {
        try {
            await mount(locale, c);
            assert.ok(page().includes(c['conn.status.propagating']));
            assert.ok(page().includes(c['conn.propagation.auto']), 'the card says it checks again by itself');
            assert.ok(page().includes(c['conn.propagation.lastChecked'].replace('{time}', timeOf(locale, 0))), 'the time of the last answer, from the server\'s stamp');
            assert.equal(pollTimers().length, 1, 'the next check is scheduled');
            assert.equal(await firePoll(), 1);
            assert.equal(calls.length, 2, 'one automatic check');
            assert.ok(page().includes(c['conn.propagation.lastChecked'].replace('{time}', timeOf(locale, 1))), 'the time follows the latest answer');
            for (let check = 2; check <= DOMAIN_SPREAD_AUTO_CHECKS; check++) assert.equal(await firePoll(), 1, `automatic check ${check}`);
            assert.equal(calls.length, 1 + DOMAIN_SPREAD_AUTO_CHECKS, 'the bound: 60 automatic checks, 30 minutes of a visible page');
            assert.equal(DOMAIN_SPREAD_AUTO_CHECKS * DOMAIN_SPREAD_RECHECK_MS, 30 * 60_000);
            assert.equal(pollTimers().length, 0, 'nothing is scheduled after the bound');
            assert.equal(await firePoll(), 0);
            assert.equal(calls.length, 1 + DOMAIN_SPREAD_AUTO_CHECKS);
            const stopped = c['conn.propagation.autoStopped'].replace('{time}', timeOf(locale, DOMAIN_SPREAD_AUTO_CHECKS));
            assert.ok(page().includes(stopped), 'the stop is said plainly, with the time of the last check');
            assert.ok(!page().includes(c['conn.propagation.auto']), 'the card no longer says it checks by itself');
            // "Check again" still checks now, and does not restart the automatic checks.
            const button = tree.root.findAll(node => node.type === 'button' && textOf(node.children).includes(c['conn.recheck']))[0];
            await act(async () => { button.props.onClick(); });
            await settle();
            assert.equal(calls.length, 2 + DOMAIN_SPREAD_AUTO_CHECKS);
            assert.ok(page().includes(c['conn.propagation.autoStopped'].replace('{time}', timeOf(locale, DOMAIN_SPREAD_AUTO_CHECKS + 1))));
            assert.equal(pollTimers().length, 0);
            assert.ok(calls.every(call => call.method === 'GET' && call.path === PATH), 'only the GET that "Check again" sends');
        } finally { await unmount(); }
    });
}

test('a hidden tab checks nothing, and the checks resume when it is shown', async () => {
    try {
        await mount('en', { ...en, ...enScreens });
        assert.equal(pollTimers().length, 1);
        globalThis.document.visibilityState = 'hidden';
        await act(async () => { globalThis.document.dispatchEvent(new Event('visibilitychange')); });
        assert.equal(pollTimers().length, 0, 'hidden: nothing is scheduled');
        assert.equal(await firePoll(), 0);
        assert.equal(calls.length, 1, 'no check while hidden');
        globalThis.document.visibilityState = 'visible';
        await act(async () => { globalThis.document.dispatchEvent(new Event('visibilitychange')); });
        // Shown again: one check at once, not after the delay (2026-10-15).
        assert.equal(calls.length, 2, 'shown again: the card checks at once');
        assert.equal(pollTimers().length, 1, 'then the next check is scheduled');
        assert.equal(await firePoll(), 1);
        assert.equal(calls.length, 3);
        assert.ok(calls.every(call => call.method === 'GET'));
    } finally { await unmount(); }
});

test('a change that has stopped spreading is not checked again, and nothing is said about checking', async () => {
    const c = { ...en, ...enScreens };
    try {
        await mount('en', c, index => index === 0);
        assert.equal(await firePoll(), 1);
        assert.equal(calls.length, 2);
        assert.ok(!page().includes(c['conn.status.propagating']));
        assert.ok(!page().includes(c['conn.propagation.auto']) && !page().includes(c['conn.propagation.lastChecked'].split('{time}')[0]));
        assert.equal(pollTimers().length, 0, 'connected: nothing is scheduled');
        assert.equal(await firePoll(), 0);
    } finally { await unmount(); }
    try {
        await mount('en', c, () => false);
        assert.equal(pollTimers().length, 0, 'a domain that is not spreading is never checked by itself');
    } finally { await unmount(); }
});

test('the spreading wording says who acts and how it resumes, in Turkish too', () => {
    for (const c of [enScreens, trScreens]) {
        for (const key of ['conn.propagation.auto', 'conn.propagation.lastChecked', 'conn.propagation.autoStopped']) assert.ok(c[key], key);
        assert.ok(c['conn.propagation.lastChecked'].includes('{time}'));
        assert.ok(c['conn.propagation.autoStopped'].includes('{time}'));
        assert.ok(c['conn.propagation.auto'].includes(c['conn.recheck']), 'names the button by its label');
        assert.ok(c['conn.propagation.autoStopped'].includes(c['conn.recheck']));
    }
    assert.match(trScreens['conn.propagation.auto'], /birkaç dakikadan birkaç saate kadar sürebilir/);
    assert.match(trScreens['conn.propagation.auto'], /yarım dakikada bir kendiliğinden yeniden kontrol eder; sizin bir şey yapmanız gerekmez/);
    assert.match(trScreens['conn.propagation.autoStopped'], /hâlâ yayılıyordu/);
    assert.match(trScreens['conn.propagation.autoStopped'], /Bu bir hata değildir/);
    assert.match(enScreens['conn.propagation.autoStopped'], /This is not a failure/);
});
