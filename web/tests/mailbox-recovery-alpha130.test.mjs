import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

// Mailbox recovery (alpha.130; docs/MAILBOX-RECOVERY-REVIEW.md): the public
// pages read their token only from the fragment and drop it at once, the new
// password follows the mailbox rule with PasswordInput, every refusal code the
// server sends has words in both languages, and nothing joins the boot path.
// Posta kutusu kurtarma: jeton yalnız parça kısmından okunur, parola kuralı
// uygulanır, her ret kodunun iki dilde metni vardır, önyükleme büyümez.

const source = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const page = source('../src/components/MailboxRecoveryPage.tsx');
const dialog = source('../src/components/MailboxRecoveryDialog.tsx');
const en = source('../src/i18n/mailboxRecovery/en.ts');
const tr = source('../src/i18n/mailboxRecovery/tr.ts');
const goHandlers = source('../../cmd/panel/mailbox_recovery_handlers.go');

const keysOf = (text) => [...text.matchAll(/^\s+'([a-z0-9_.]+)':/gm)].map((match) => match[1]).sort();

test('the pages read the token only from the fragment and remove it at once', () => {
  assert.match(page, /accountLinkToken\(window\.location\.hash\)/);
  assert.match(page, /window\.history\.replaceState\(window\.history\.state, '', window\.location\.pathname\)/);
  // The confirmation is sent once per page load, never on a re-render.
  assert.match(page, /sent\.current = true/);
});

test('the three pages are exact paths under the account-link prefix', () => {
  for (const path of ['/account-link/mailbox-recovery', '/account-link/mailbox-confirm', '/account-link/mailbox-reset']) {
    assert.ok(page.includes(`'${path}'`), path);
  }
  for (const path of ['mailboxRecoveryConfirmPage = "/account-link/mailbox-confirm"', 'mailboxRecoveryResetPage   = "/account-link/mailbox-reset"', 'mailboxRecoveryRequestPage = "/account-link/mailbox-recovery"']) {
    assert.ok(source('../../cmd/panel/mailbox_recovery.go').includes(path), path);
  }
});

test('the new mailbox password follows the mailbox rule with PasswordInput', () => {
  assert.match(page, /<PasswordInput field="mailbox"/);
  assert.match(page, /passwordAcceptable\(password, 'mailbox'\)/);
  assert.doesNotMatch(page, /type="password"/);
});

test('no browser dialog; removal asks in the panel\'s own window', () => {
  for (const text of [page, dialog]) {
    assert.doesNotMatch(text, /window\.(confirm|alert|prompt)\(/);
  }
  assert.match(dialog, /confirmAction\(/);
});

test('both languages carry the same keys and every server refusal has words', () => {
  assert.deepEqual(keysOf(tr), keysOf(en));
  const codes = [...goHandlers.matchAll(/errCodeMailboxRecovery\w+\s+=\s+"([a-z_]+)"/g)].map((match) => match[1]);
  assert.ok(codes.length >= 8, 'the Go codes were read');
  for (const code of codes) {
    assert.ok(en.includes(`'err.${code}'`), `en err.${code}`);
    assert.ok(tr.includes(`'err.${code}'`), `tr err.${code}`);
  }
});

test('nothing joins the boot path: both screens are their own chunks', () => {
  assert.doesNotMatch(source('../src/App.tsx'), /MailboxRecovery/);
  assert.match(source('../src/components/AccountLinkPage.tsx'), /lazy\(\(\) => import\('\.\/MailboxRecoveryPage'\)\)/);
  assert.match(source('../src/components/DomainMailManager.tsx'), /lazy\(\(\) => import\('\.\/MailboxRecoveryDialog'\)\)/);
  assert.doesNotMatch(source('../src/i18n/en.ts'), /mailboxRecovery/);
});

test('a stop set by hand is not offered a password change and says who reopens it', () => {
  const limits = source('../src/components/OutgoingMailboxLimits.tsx');
  assert.match(limits, /!readOnly && row\.paused\.reason !== 'manual' &&/);
  assert.match(limits, /'mout\.dom\.pausedManual\.manager' : 'mout\.dom\.pausedManual\.customer'/);
  const manager = source('../src/components/DomainMailManager.tsx');
  assert.ok(manager.includes('const stillPaused = /"sending_paused":'), 'the password answer is read for a stop that stays');
  for (const lang of ['en', 'tr']) {
    const words = source(`../src/i18n/mailOutgoing/${lang}.ts`) + source(`../src/i18n/screens/mailWork/${lang}.ts`);
    for (const key of ['mout.dom.pausedManual.manager', 'mout.dom.pausedManual.customer', 'mout.paused.nextManual', 'mail.sendingStillPaused.manager', 'mail.sendingStillPaused.customer']) {
      assert.ok(words.includes(`'${key}'`), `${lang} ${key}`);
    }
  }
});
