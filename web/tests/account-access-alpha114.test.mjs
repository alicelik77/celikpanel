import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';

// Account access (alpha.114, P6): the link pages read their token only from the
// fragment, only in the shape the server makes, and the inviter's copy of a
// link is only ever a path on this panel.
// Hesap erişimi: jeton yalnız parça kısmından ve sunucunun ürettiği biçimde okunur.

const source = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const moduleURL = (text) => 'data:text/javascript;base64,' + Buffer.from(text).toString('base64');
const lib = await import(moduleURL(ts.transpileModule(source('../src/lib/accountAccess.ts'), {
  compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 },
}).outputText));

const token = 'A'.repeat(42) + 'Q';

test('a link page is one of three exact paths', () => {
  assert.equal(lib.accountLinkPurpose('/account-link/reset'), 'password_reset');
  assert.equal(lib.accountLinkPurpose('/account-link/invitation/'), 'invitation');
  assert.equal(lib.accountLinkPurpose('/account-link/verify'), 'email_verify');
  assert.equal(lib.accountLinkPurpose('/account-link/other'), null);
  assert.equal(lib.accountLinkPurpose('/account'), null);
});

test('the token is read only from #t= in the server’s shape', () => {
  assert.equal(lib.accountLinkToken('#t=' + token), token);
  assert.equal(lib.accountLinkToken('#t=' + token + 'x'), null);
  assert.equal(lib.accountLinkToken('#t=short'), null);
  assert.equal(lib.accountLinkToken('?t=' + token), null);
  assert.equal(lib.accountLinkToken('#t=' + token.slice(1) + '/'), null);
});

test('an invitation copy is only a path on this panel', () => {
  const copied = lib.decodeInvitationOutcome({ id: 4, delivery: 'link', reason: 'no_mail_server', link_path: '/account-link/invitation#t=' + token });
  assert.equal(lib.invitationLink(copied, 'https://panel.example.com:1923'), 'https://panel.example.com:1923/account-link/invitation#t=' + token);
  const foreign = lib.decodeInvitationOutcome({ delivery: 'link', link_path: 'https://evil.example/account-link/invitation#t=' + token });
  assert.equal(foreign.link_path, undefined);
  assert.equal(lib.invitationLink(foreign, 'https://panel.example.com'), '');
  assert.equal(lib.decodeInvitationOutcome({ delivery: 'mailed' }), null);
  assert.equal(lib.decodeInvitationOutcome({ delivery: 'sent' }).delivery, 'sent');
});

test('the mail status and the own confirmation state are decoded strictly', () => {
  assert.deepEqual(lib.decodeAccountMailStatus({ available: false, reason: 'no_mail_server' }), { available: false, reason: 'no_mail_server' });
  assert.throws(() => lib.decodeAccountMailStatus({ reason: 'x' }));
  assert.throws(() => lib.decodeOwnEmailVerification({ email_set: true, mail: { available: true } }));
  assert.deepEqual(lib.decodePendingInvitations({ invitations: null }), []);
  assert.throws(() => lib.decodePendingInvitations({}));
});

test('the link page clears the fragment and sends a confirmation once', () => {
  const page = source('../src/components/AccountLinkPage.tsx');
  assert.match(page, /window\.history\.replaceState\(window\.history\.state, '', window\.location\.pathname\)/);
  assert.match(page, /if \(!purpose \|\| !token \|\| sent\.current\) return;\s*sent\.current = true;/);
  // The session gate is not drawn over a link page.
  assert.match(source('../src/App.tsx'), /<AccountLinkSwitch><AuthGate \/><\/AccountLinkSwitch>/);
});
