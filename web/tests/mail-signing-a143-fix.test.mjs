// Mail signing after the independent review of alpha.143 (21 Nov 2026).
// Problems now words each signing state with the server owner's concrete step
// (finding 10) and never shows a state it does not know as fine; an
// out-of-office reply refuses an address that is not the mailbox's own and
// says once which saved addresses CelikPanel removed (finding 1).
// İmza durumları kendi somut adımıyla; kutunun kendisine ait olmayan adres
// reddedilir, çıkarılan adresler bir kez söylenir.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enProblemsNowScreens } from '../src/i18n/screens/problemsNow/en.ts';
import { trProblemsNowScreens } from '../src/i18n/screens/problemsNow/tr.ts';
import { enMailDatabasesScreens } from '../src/i18n/screens/mailDatabases/en.ts';
import { trMailDatabasesScreens } from '../src/i18n/screens/mailDatabases/tr.ts';
import { enErrorsScreens } from '../src/i18n/screens/errors/en.ts';
import { trErrorsScreens } from '../src/i18n/screens/errors/tr.ts';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const placeholders = (text) => [...text.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();
const states = ['conf_kept', 'port25_kept', 'submit_open', 'unsupported', 'failed', 'unknown'];

test('every signing state has its own words with a step, in both languages', () => {
    for (const state of states) {
        const key = `pnow.mail_signing.${state}`;
        const en = enProblemsNowScreens[key];
        const tr = trProblemsNowScreens[key];
        assert.ok(en && tr, `${state} has no words`);
        assert.notEqual(en, tr, `${state} is not translated`);
        assert.deepEqual(placeholders(tr), placeholders(en), `${state}: different values`);
        assert.doesNotMatch(en, /\blog\b/i, `${state} sends the administrator to the log`);
    }
    assert.match(enProblemsNowScreens['pnow.mail_signing.submit_open'], /authorized_submit_users = root, vmail/);
    assert.match(enProblemsNowScreens['pnow.mail_signing.conf_kept'], /SenderMacro \{auth_authen\}/);
    // The refused login setting is never advised where OpenDKIM refuses it.
    assert.doesNotMatch(enProblemsNowScreens['pnow.mail_signing.unsupported'], /SenderMacro/);
    assert.equal(enProblemsNowScreens['pnow.mail_signing.body'], undefined, 'the old one-size body that pointed at the log is gone');
});

test('the list picks the state words and reads an unknown state as unknown', () => {
    const source = read('../src/components/ProblemsNow.tsx');
    assert.match(source, /t\(problemBodyKey\(problem\), vars\)/);
    const body = /export function problemBodyKey[\s\S]*?\n}\n/.exec(source)?.[0] ?? '';
    // eslint-disable-next-line no-new-func
    const problemBodyKey = new Function('MAIL_SIGNING_STATES', `return ${body.replace(/^export function problemBodyKey\(problem[^)]*\): TranslationKey/, 'function (problem)').replace(/ as TranslationKey/g, '')}`)(states);
    assert.equal(problemBodyKey({ kind: 'mail_signing', name: 'submit_open' }), 'pnow.mail_signing.submit_open');
    assert.equal(problemBodyKey({ kind: 'mail_signing', name: 'from_a_newer_panel' }), 'pnow.mail_signing.unknown');
    assert.equal(problemBodyKey({ kind: 'disk_full' }), 'pnow.disk_full.body');
});

test('an address that is not the mailbox’s own is refused with its name, and removed ones are said once', () => {
    for (const [errors, mail] of [[enErrorsScreens, enMailDatabasesScreens], [trErrorsScreens, trMailDatabasesScreens]]) {
        assert.deepEqual(placeholders(errors['err.MAIL_AUTORESPONDER_REFUSED.foreign_address']), ['addresses']);
        assert.deepEqual(placeholders(mail['mail.autoreply.removed']), ['addresses']);
    }
    const dialog = read('../src/components/MailAutoresponderDialog.tsx');
    assert.match(dialog, /removed_addresses/);
    assert.match(dialog, /t\('mail\.autoreply\.removed', \{ addresses:/);
});

// Second review of alpha.143 (22 Nov 2026): a main.cf filter list the panel
// cannot read has two states of its own (B4); an out-of-office reply that
// still answers from a foreign address is one Problems now line per mailbox,
// worded per step (B3), and its window says it was turned off or is pending.
// The words live in the mail screen's part (Problems now's part is full).
test('the variable filter list and a reply still answering from a foreign address have their own words', () => {
    const keys = [
        'pnow.mail_signing.port25_variable', 'pnow.mail_signing.port25_variable_own', 'pnow.mail_signing.port25_variable_own_title',
        'pnow.mail_autoresponder_foreign.title', 'pnow.mail_autoresponder_foreign.agent_unreachable', 'pnow.mail_autoresponder_foreign.agent_build',
        'pnow.mail_autoresponder_foreign.agent_failed', 'pnow.mail_autoresponder_foreign.action',
        'mail.autoreply.removedOff', 'mail.autoreply.foreignPending',
    ];
    for (const key of keys) {
        const en = enMailDatabasesScreens[key];
        const tr = trMailDatabasesScreens[key];
        assert.ok(en && tr, `${key} has no words`);
        assert.notEqual(en, tr, `${key} is not translated`);
        assert.deepEqual(placeholders(tr), placeholders(en), `${key}: different values`);
    }
    // Never the hand-changed entry's words, and the own-value state says it is still in force.
    assert.doesNotMatch(enMailDatabasesScreens['pnow.mail_signing.port25_variable_own'], /changed by hand/);
    assert.match(enMailDatabasesScreens['pnow.mail_signing.port25_variable_own'], /still in force/);
    const source = read('../src/components/ProblemsNow.tsx');
    assert.match(source, /'mail_autoresponder_foreign'/);
    assert.match(source, /t\(problemTitleKey\(problem\), vars\)/);
    const body = /export function problemBodyKey[\s\S]*?\n}\n/.exec(source)?.[0] ?? '';
    // eslint-disable-next-line no-new-func
    const problemBodyKey = new Function('MAIL_SIGNING_STATES', `return ${body.replace(/^export function problemBodyKey\(problem[^)]*\): TranslationKey/, 'function (problem)').replace(/ as TranslationKey/g, '')}`)([...states, 'port25_variable', 'port25_variable_own']);
    assert.equal(problemBodyKey({ kind: 'mail_signing', name: 'port25_variable_own' }), 'pnow.mail_signing.port25_variable_own');
    assert.equal(problemBodyKey({ kind: 'mail_autoresponder_foreign', subject: 'agent_build' }), 'pnow.mail_autoresponder_foreign.agent_build');
    assert.equal(problemBodyKey({ kind: 'mail_autoresponder_foreign', subject: 'from_a_newer_panel' }), 'pnow.mail_autoresponder_foreign.agent_failed');
    const dialog = read('../src/components/MailAutoresponderDialog.tsx');
    assert.match(dialog, /t\('mail\.autoreply\.removedOff'\)/);
    assert.match(dialog, /t\('mail\.autoreply\.foreignPending', \{ addresses:/);
});
