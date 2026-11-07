import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { setupExecutionGuidance, setupCheckLine } from '../src/lib/serverSetupGuidance.ts';
import { enScreens, trScreens } from './locale-catalogue.mjs';

// alpha.114 lab run 1, a server joined by code: the joined server's waits
// for the main DNS server name it (E1), the final DNS check of a later run
// says why it waits and who acts, and an overdue wait says the owner acts
// now (E8); the joined server's mail step no longer claims to write the main
// domain's records (S9).
// Katilan sunucu: bekleme ana DNS sunucusunu adlandirir, DNS denetimi
// nedenini soyler, ana alan adinin kayitlari ilk sunucuda kalir.

const source = path => readFileSync(new URL(path, import.meta.url), 'utf8');
const context = (extra = {}) => ({ dns_mode: 'local', dns_role: 'secondary', dns_engine: 'pdns', local_nameserver: 'ns2.c114.test', local_ip: '192.0.3.161', peer_nameserver: 'ns1.c114.test', peer_ip: '192.0.3.160', panel_domain: 'boston.c114.test', mail_hostname: 'mail.boston.c114.test', dns_hosting_management: 'panel', dns_publisher_host: 'frankfurt.c114.test', ...extra });
const keys = guide => guide.messages.map(item => item.key);
const placeholders = text => [...String(text).matchAll(/\{(\w+)\}/g)].map(match => match[1]).sort().join(',');

test('a wait for the main DNS server names it; only a verified refusal asks for a new code', () => {
    for (const code of ['server_setup_dns_publisher_peer_busy', 'server_setup_dns_publisher_peer_license', 'server_setup_dns_publisher_peer_unexplained', 'server_setup_dns_publisher_unconfirmed']) {
        const guide = setupExecutionGuidance({ status: code.endsWith('unconfirmed') ? 'failed' : 'waiting', phase: 'dns_publisher', steps: [{ id: '10-dns_publisher', kind: 'dns_publisher', target: 'https://frankfurt.c114.test:1923', status: 'running' }], error: { code, message: '' }, context: context() });
        const line = guide.messages.find(item => item.key === `setup.v5.wait.${code}`);
        assert.ok(line, code);
        assert.deepEqual(line.values, { server: 'frankfurt.c114.test' });
        assert.match(enScreens[`setup.v5.wait.${code}`], /\{server\}/);
        assert.match(trScreens[`setup.v5.wait.${code}`], /\{server\}/);
        assert.equal(/new join code/.test(enScreens[`setup.v5.wait.${code}`]), code.endsWith('unconfirmed'), code);
    }
});

test('the final DNS check says its reason, who acts and the next action', () => {
    const reasons = {
        dns_publisher_missing: ['setup.check.dns.publisherMissing', { primary: 'frankfurt.c114.test' }],
        dns_publisher_unconfirmed: ['setup.check.dns.publisherUnconfirmed', { primary: 'frankfurt.c114.test' }],
        dns_backup_not_serving: ['setup.check.dns.backupNotServing', { ns1: 'ns1.c114.test', ns2: 'ns2.c114.test' }],
        dns_nameservers_not_public: ['setup.check.dns.nameserversNotPublic', { ns1: 'ns1.c114.test', ns2: 'ns2.c114.test' }],
        dns_setup_changed: ['setup.check.dns.setupChanged', {}],
        dns_engine_not_ready: ['setup.check.dns.engineNotReady', {}],
        // alpha.129 lab: the DNS software stopped; the line names it and the command.
        dns_engine_stopped: ['setup.check.dns.engineStopped', { engine: 'BIND', unit: 'named' }],
        dns_pair_not_ready: ['setup.check.dns.pairNotReady', {}],
    };
    const vars = { primary: 'frankfurt.c114.test', ns1: 'ns1.c114.test', ns2: 'ns2.c114.test', local_ip: '192.0.3.161', peer_ip: '192.0.3.160', engine: 'BIND', unit: 'named' };
    for (const [reason, [key, values]] of Object.entries(reasons)) {
        const line = setupCheckLine({ id: 'dns', state: reason === 'dns_publisher_unconfirmed' ? 'unknown' : 'action_required', code: 'dns_required', reason, vars });
        assert.equal(line.text?.key, key, reason);
        assert.deepEqual(line.text.values, values);
        assert.ok(enScreens[key] && trScreens[key], key);
        assert.equal(placeholders(enScreens[key]), placeholders(trScreens[key]), key);
    }
    // An unexpected reason or a name that is not a host name keeps the general sentence.
    assert.equal(setupCheckLine({ id: 'dns', state: 'action_required', code: 'dns_required', reason: 'other' }).text, undefined);
    assert.equal(setupCheckLine({ id: 'dns', state: 'action_required', code: 'dns_required', reason: 'dns_publisher_missing', vars: { primary: '<b>' } }).text, undefined);
});

test('a final check waiting past its bound is said as the owner acting now', () => {
    const checks = [{ id: 'dns', state: 'action_required', code: 'dns_required', reason: 'dns_publisher_missing', vars: { primary: 'frankfurt.c114.test' } }];
    const run = extra => ({ status: 'waiting', phase: 'verification', steps: [{ id: '06-verify', kind: 'verify', target: 'custom', status: 'pending' }], checks, context: context(), ...extra });
    const waiting = setupExecutionGuidance(run());
    assert.equal(waiting.title, 'setup.guide.waitTitle');
    assert.ok(keys(waiting).includes('setup.guide.verificationResume'));
    assert.equal(waiting.checks[0].text.key, 'setup.check.dns.publisherMissing');
    const overdue = setupExecutionGuidance(run({ error: { code: 'server_setup_verification_overdue', message: '' } }));
    assert.equal(overdue.title, 'setup.guide.failedTitle');
    assert.ok(keys(overdue).includes('setup.guide.verificationOverdue'));
    assert.ok(!keys(overdue).includes('setup.guide.verificationResume'));
    assert.ok(enScreens['setup.guide.verificationOverdue'] && trScreens['setup.guide.verificationOverdue']);
});

test('the in-place mail dialog shows each open check and a check-again action', () => {
    const dialog = source('../src/components/MailSetupDialog.tsx');
    assert.match(dialog, /guidance\.checks && guidance\.checksAt === index/);
    assert.match(dialog, /data-mail-setup-checks/);
    assert.match(dialog, /setup\.reload\('manual'\)/);
    assert.match(dialog, /server_setup_verification_overdue/);
    assert.ok(enScreens['svc.mailSetup.checkAgain'] && trScreens['svc.mailSetup.checkAgain']);
});

test('a joined server\'s mail step keeps the main domain\'s records on the first server', () => {
    const steps = source('../src/components/ServerSetupWizardSteps.tsx');
    const joinedBranch = steps.slice(steps.indexOf('if (joined) return'), steps.indexOf('data-setup-mail=""'));
    assert.match(joinedBranch, /setup\.v3\.mail\.joinedMain/);
    assert.doesNotMatch(joinedBranch, /helpBackup|MX<\/span>|SPF<\/span>|DMARC<\/span>/);
    assert.match(source('../src/components/ServerSetup.tsx'), /joined=\{joinedLink && firstKeepsMainMail \?/);
    for (const key of ['setup.v3.mail.joinedTitle', 'setup.v3.mail.joinedMain', 'setup.v3.mail.joinedOwn']) {
        assert.ok(enScreens[key] && trScreens[key], key);
        assert.equal(placeholders(enScreens[key]), placeholders(trScreens[key]), key);
        assert.doesNotMatch(enScreens[key], /over the setup link/);
    }
    assert.match(enScreens['setup.v3.mail.joinedMain'], /does not change/);
});
