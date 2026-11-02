// "Get certificate" is honest about the domain's DNS (owner, 2 Nov 2026: "If the
// domain is not on the server, the certificate cannot be obtained either").
// The SSL/TLS screen reads the Overview's live check plus where each name of the
// request resolves; a known negative turns the button off and says, where the
// person looks, what is wrong and what to do; an unknown answer allows the
// attempt with a warning; Check again reads again without a page refresh.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import ts from 'typescript';
import { enScreens, trScreens } from './locale-catalogue.mjs';

const require = createRequire(import.meta.url);
const reactURL = pathToFileURL(require.resolve('react')).href;
const dataURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`;
const read = path => readFileSync(new URL(path, import.meta.url), 'utf8');
const compile = path => ts.transpileModule(read(path), { compilerOptions: { jsx: ts.JsxEmit.React, module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 } }).outputText;
const fill = (catalogue, key, vars) => String(catalogue[key]).replace(/\{(\w+)\}/g, (all, name) => vars?.[name] ?? all);
const textOf = node => node === null || node === undefined || typeof node === 'boolean' ? '' : typeof node === 'string' || typeof node === 'number' ? String(node) : Array.isArray(node) ? node.map(textOf).join('') : textOf(node.children);

const { certificateReachGate, decodeCertificateReach } = await import(dataURL(compile('../src/lib/certificateNames.ts')));

const reach = (checks, extra = {}) => decodeCertificateReach({
    server_ip: '72.62.38.15',
    nameservers: ['ns1.celikhost.com', 'ns2.celikhost.com'],
    live_nameservers: ['ns1.polohosting.com', 'ns2.polohosting.com'],
    certificate_names: checks,
    ...extra,
});
const site = (name, state) => ({ name, kind: 'site', state, ips: [] });
const mail = (name, state) => ({ name, kind: 'mail', state, ips: [] });
const DOMAIN = 'alicelik.com.tr';

test('the gate blocks only on a known negative and never on unknown', () => {
    const here = reach([site(DOMAIN, 'here'), site(`www.${DOMAIN}`, 'here'), mail(`mail.${DOMAIN}`, 'here')]);
    assert.equal(certificateReachGate(here, false, DOMAIN, true).kind, 'open');
    assert.equal(certificateReachGate(null, false, DOMAIN, false).kind, 'checking');

    const elsewhere = reach([site(DOMAIN, 'elsewhere'), site(`www.${DOMAIN}`, 'elsewhere'), mail(`mail.${DOMAIN}`, 'unknown')]);
    const blocked = certificateReachGate(elsewhere, false, DOMAIN, false);
    assert.equal(blocked.kind, 'blocked');
    assert.deepEqual(blocked.names, [DOMAIN, `www.${DOMAIN}`]);
    assert.equal(blocked.domainFails, true);
    assert.equal(blocked.mailOnly, false);
    assert.equal(blocked.serverIp, '72.62.38.15');
    assert.deepEqual(blocked.liveNameservers, ['ns1.polohosting.com', 'ns2.polohosting.com']);

    // A lookup that failed (a name, or the whole read) is unknown: the attempt is allowed.
    const unknown = certificateReachGate(reach([site(DOMAIN, 'unknown'), site(`www.${DOMAIN}`, 'here')]), false, DOMAIN, false);
    assert.deepEqual([unknown.kind, unknown.names], ['unknown', [DOMAIN]]);
    assert.equal(certificateReachGate(null, true, DOMAIN, false).kind, 'unknown');
    // A read that names no check is not "everything points here".
    assert.equal(certificateReachGate(reach([]), false, DOMAIN, false).kind, 'unknown');
    assert.throws(() => decodeCertificateReach({ server_ip: '1.2.3.4' }));
    assert.throws(() => decodeCertificateReach({ server_ip: '1.2.3.4', certificate_names: [{ name: 'x', kind: 'site', state: 'maybe' }] }));
});

test('only the mail name failing is offered without it, and counts only when mail is included', () => {
    const mailFails = reach([site(DOMAIN, 'here'), site(`www.${DOMAIN}`, 'here'), mail(`mail.${DOMAIN}`, 'elsewhere')]);
    assert.equal(certificateReachGate(mailFails, false, DOMAIN, false).kind, 'open');
    const withMail = certificateReachGate(mailFails, false, DOMAIN, true);
    assert.equal(withMail.kind, 'blocked');
    assert.equal(withMail.mailOnly, true);
    assert.deepEqual(withMail.names, [`mail.${DOMAIN}`]);
    // The nameservers already are this server's: they are not "the ones now".
    const delegated = reach([site(DOMAIN, 'elsewhere')], { live_nameservers: ['NS2.celikhost.com.', 'ns1.celikhost.com'] });
    assert.deepEqual(certificateReachGate(delegated, false, DOMAIN, false).liveNameservers, []);
});

async function mountNotice(gate, actions) {
    const lucide = dataURL('export const AlertTriangle = () => null, RefreshCw = () => null;');
    const stub = dataURL(`import React from '${reactURL}';
export const useI18n = () => ({ t: globalThis.__reachT });
export const Button = ({ loading, icon, variant, children, ...props }) => React.createElement('button', props, children);`);
    const source = compile('../src/components/CertificateReachNotice.tsx')
        .replace(/from ['"]([^'"]+)['"]/g, (_, from) => `from '${from === 'react' ? reactURL : from === 'lucide-react' ? lucide : stub}'`);
    const { CertificateReachNotice } = await import(dataURL(`import React from '${reactURL}';\n${source}`));
    return props => Renderer.create(React.createElement(CertificateReachNotice, { domainName: DOMAIN, rechecking: false, ...actions, ...props, gate }));
}

for (const [locale, catalogue] of [['en', enScreens], ['tr', trScreens]]) {
    test(`the notice names the problem, who acts, the fix and how it resumes (${locale})`, async () => {
        globalThis.__reachT = (key, vars) => fill(catalogue, key, vars);
        const calls = [];
        const blocked = certificateReachGate(
            reach([site(DOMAIN, 'elsewhere'), site(`www.${DOMAIN}`, 'elsewhere')]), false, DOMAIN, false);
        const make = await mountNotice(blocked, { onRecheck: () => calls.push('recheck'), onLeaveOutMail: () => calls.push('mail') });

        let top;
        await act(async () => { top = make({ variant: 'top' }); });
        const text = textOf(top.toJSON());
        // The real names and address of this server, not examples.
        for (const part of [DOMAIN, `www.${DOMAIN}`, 'ns1.polohosting.com, ns2.polohosting.com', 'ns1.celikhost.com, ns2.celikhost.com', '72.62.38.15']) {
            assert.ok(text.includes(part), `${locale}: the guidance names ${part}: ${text}`);
        }
        assert.ok(text.includes(catalogue['ssl.reach.blockedTitle']));
        assert.ok(text.includes(catalogue['ssl.reach.resume']));
        const button = top.root.findAll(node => node.type === 'button' && textOf(node) === catalogue['ssl.reach.recheck'])[0];
        await act(async () => button.props.onClick());
        assert.deepEqual(calls, ['recheck']);

        let inline;
        await act(async () => { inline = make({ variant: 'inline' }); });
        assert.equal(inline.root.findByProps({ id: 'ssl-reach-reason' }).props.role, 'status');
        assert.ok(textOf(inline.toJSON()).includes(catalogue['ssl.reach.inline.blocked']));
        await act(async () => { top.unmount(); inline.unmount(); });
    });

    test(`mail-only and unknown notices say what they are (${locale})`, async () => {
        globalThis.__reachT = (key, vars) => fill(catalogue, key, vars);
        const calls = [];
        const mailOnly = certificateReachGate(
            reach([site(DOMAIN, 'here'), mail(`mail.${DOMAIN}`, 'elsewhere')]), false, DOMAIN, true);
        const makeMail = await mountNotice(mailOnly, { onRecheck: () => {}, onLeaveOutMail: () => calls.push('mail') });
        let view;
        await act(async () => { view = makeMail({ variant: 'top' }); });
        const leave = view.root.findAll(node => node.type === 'button' && textOf(node) === fill(catalogue, 'ssl.reach.leaveOutMail', { mail: `mail.${DOMAIN}` }))[0];
        assert.ok(leave, `${locale}: the request without the mail name is offered`);
        await act(async () => leave.props.onClick());
        assert.deepEqual(calls, ['mail']);
        await act(async () => view.unmount());

        const unknown = certificateReachGate(null, true, DOMAIN, false);
        const makeUnknown = await mountNotice(unknown, { onRecheck: () => {}, onLeaveOutMail: () => {} });
        await act(async () => { view = makeUnknown({ variant: 'top' }); });
        const text = textOf(view.toJSON());
        assert.ok(text.includes(catalogue['ssl.reach.unknownTitle']) && text.includes(DOMAIN));
        assert.ok(!text.includes(catalogue['ssl.reach.blockedTitle']), 'unknown is not called blocked');
        await act(async () => view.unmount());
    });
}

test('the SSL screen turns Get certificate off for a known negative, reads again by itself and never on unknown', () => {
    const source = read('../src/components/DomainSSLSettings.tsx');
    assert.match(source, /connection\?names=certificate/);
    assert.match(source, /reachHolds = gate\.kind === 'blocked' \|\| gate\.kind === 'checking'/);
    assert.match(source, /disabled=\{locked \|\| providersRead\.remote\.state !== 'known' \|\| issuing \|\| !email \|\| replacementBlockedByMailState \|\| reachHolds\}/);
    assert.match(source, /reachHolds\) return;/, 'the handler does not start a request either');
    assert.match(source, /onRecheck=\{\(\) => void reach\.retry\(\)\}/);
    assert.match(source, /code === 'CERTIFICATE_DOMAIN_NOT_POINTED'\) void reach\.retry\(\)/);
    assert.doesNotMatch(source, /gate\.kind === 'unknown'[^\n]*disabled/);
});

test('the texts exist in both languages with the same placeholders, and the server refusal has words', () => {
    const keys = Object.keys(enScreens).filter(key => key.startsWith('ssl.reach.') || key.startsWith('err.CERTIFICATE_DOMAIN_NOT_POINTED'));
    assert.ok(keys.length >= 20, `found ${keys.length} keys`);
    const holes = text => [...String(text).matchAll(/\{(\w+)\}/g)].map(match => match[1]).sort().join(',');
    for (const key of keys) {
        assert.ok(trScreens[key], `${key} in Turkish`);
        assert.equal(holes(trScreens[key]), holes(enScreens[key]), `${key} has the same placeholders in both languages`);
    }
    assert.ok(enScreens['err.CERTIFICATE_DOMAIN_NOT_POINTED.mail_name'] && trScreens['err.CERTIFICATE_DOMAIN_NOT_POINTED.mail_name']);
    // The server sends exactly these variables with the refusal.
    const server = readFileSync(new URL('../../cmd/panel/certificate_names_check.go', import.meta.url), 'utf8');
    assert.match(server, /CERTIFICATE_DOMAIN_NOT_POINTED/);
    for (const name of ['names', 'ip']) assert.match(server, new RegExp(`"${name}": `));
});
