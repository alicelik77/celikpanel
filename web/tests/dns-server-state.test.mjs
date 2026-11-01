// Owner report 2026-10-31 (Frankfurt): the first server, PowerDNS main DNS
// server with Boston joined as its backup, answered celikhost.com publicly
// while the domain's DNS page said "NOT PUBLISHED: no active authoritative DNS
// engine". dns_server "" meant "not ready to publish a new change" (the backup
// had not yet confirmed the zone list). The explanation now follows
// dns_server_state; what is allowed does not change.
//
// dns_server "" artik durumuna gore aciklanir.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';
import { enScreens, trScreens } from './locale-catalogue.mjs';
import { remoteURL } from './fixtures/shared-layer.mjs';

const read = path => readFileSync(new URL(path, import.meta.url), 'utf8');
const dataURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`;
const compile = (path, replace = s => s) => replace(ts.transpileModule(read(path), { compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 } }).outputText);
const apiErrorURL = dataURL(compile('../src/lib/apiError.ts'));
const caps = await import(dataURL(compile('../src/lib/hostingCapabilities.ts', s => s.replace(/from ['"]\.\/remote['"]/, `from '${remoteURL}'`))));
const { apiErrorText } = await import(apiErrorURL);
const fill = (catalogue) => (key, vars) => catalogue[key] === undefined ? key : String(catalogue[key]).replace(/\{(\w+)\}/g, (all, name) => vars?.[name] ?? all);
const answer = (extra) => caps.decodeHostingCapabilities({ web_server: 'nginx', php_versions: [], dns_server: '', dns_identity_ready: true, mail_server: true, dns_management_mode: 'local', dns_management_ready: false, database_servers: [], db_tools: [], ...extra });

test('the capability answer carries why the local DNS takes no new change', () => {
    const value = answer({ dns_server_state: 'pair_pending', dns_server_engine: 'pdns', dns_server_peer: 'ns2.celikhost.com' });
    assert.equal(value.dns_server_state, 'pair_pending');
    assert.equal(value.dns_server_engine, 'pdns');
    assert.equal(value.dns_server_peer, 'ns2.celikhost.com');
    // An older server says nothing: nothing is claimed beyond what it said.
    assert.equal(answer({}).dns_server_state, '');
    assert.equal(answer({ dns_server_state: 'made_up' }).dns_server_state, '');
});

for (const [locale, catalogue] of [['en', enScreens], ['tr', trScreens]]) {
    const t = fill(catalogue);
    test(`${locale}: a main DNS server waiting for its backup says the records are served and who confirms`, () => {
        const explained = caps.dnsStateExplanation(answer({ dns_server_state: 'pair_pending', dns_server_engine: 'pdns', dns_server_peer: 'ns2.celikhost.com' }), t);
        assert.equal(explained.state, 'pair_pending');
        assert.equal(explained.served, true);
        const what = t(explained.what, explained.vars);
        assert.ok(what.includes('ns2.celikhost.com'), what);
        assert.ok(!what.includes(t('dns.notServed')));
        assert.notEqual(explained.what, 'dns.notServed');
        // The Add domain screens say the same, not "DNS is missing".
        const sentence = caps.dnsEngineBlockerText(answer({ dns_server_state: 'pair_pending', dns_server_peer: 'ns2.celikhost.com' }), t);
        assert.notEqual(sentence, t('domains.add.needsDns'));
        assert.ok(sentence.includes('ns2.celikhost.com'));
        // The refusal of a change says it too (cmd/panel writeDNSServerRequired).
        const refusal = apiErrorText({ message: 'raw', code: 'DNS_SERVER_REQUIRED', reason: 'pair_pending', vars: { engine: 'pdns', peer: 'ns2.celikhost.com' } }, t);
        assert.ok(refusal.includes('ns2.celikhost.com') && refusal !== t('err.DNS_SERVER_REQUIRED'), refusal);
    });
    test(`${locale}: every state has its own plain words; no engine keeps the "not served" sentence`, () => {
        for (const state of ['secondary', 'switching', 'port53_conflict', 'not_running', 'engine_conflict']) {
            const explained = caps.dnsStateExplanation(answer({ dns_server_state: state, dns_server_engine: 'bind', dns_server_peer: 'ns1.example.com' }), t);
            assert.equal(explained.state, state);
            for (const key of [explained.what, explained.todo, explained.title, `err.DNS_SERVER_REQUIRED.${state}`]) assert.ok(catalogue[key], `${key} in ${locale}`);
            assert.ok(!t(explained.what, explained.vars).includes('{'), `${state} fills its values`);
        }
        assert.ok(t(caps.dnsStateExplanation(answer({ dns_server_state: 'not_running', dns_server_engine: 'pdns' }), t).what, { engine: 'PowerDNS' }).includes('PowerDNS'));
        assert.equal(caps.dnsStateExplanation(answer({ dns_server_state: 'secondary' }), t).vars.peer, t('dnsstate.mainServer'));
        for (const state of ['none', '']) {
            const explained = caps.dnsStateExplanation(answer({ dns_server_state: state }), t);
            assert.equal(explained.state, 'none');
            assert.equal(explained.what, 'dns.notServed');
            assert.equal(caps.dnsEngineBlockerText(answer({ dns_server_state: state }), t), t('domains.add.needsDns'));
        }
        assert.equal(caps.dnsStateExplanation(answer({ dns_server: 'pdns', dns_server_state: 'ready' }), t), null);
    });
}

test('what is allowed does not change: dns_server alone still decides', () => {
    for (const state of ['pair_pending', 'secondary', 'none']) {
        assert.equal(caps.dnsBlocker(answer({ dns_server_state: state }), 'dnsonly'), 'engine');
        assert.equal(caps.localDNSReady(answer({ dns_server_state: state })), false);
    }
    // The DNS page draws the explanation in the shared problem pattern, with a way to the DNS page.
    const page = read('../src/components/DomainDNSManager.tsx');
    assert.match(page, /<BlockingProblems id="dns-not-ready"/);
    assert.match(page, /navigate\('\/dns-infrastructure'\)/);
    assert.doesNotMatch(page, /<span>\{t\('dns\.notServed'\)\}<\/span>/);
});
