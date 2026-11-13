import assert from 'node:assert/strict';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

// Owner, 6 Nov 2026: "Every place we ask for a password must have an option to
// generate a strong password automatically, and a show/hide option." Every
// password input is components/PasswordInput.tsx. This file fails when a
// screen draws its own <input type="password"> (or a type expression that
// can be 'password') outside it. The exceptions below are deliberate and
// counted; a new one needs a reason here.
//
// Sahip, 6 Kas 2026: her parola alanı PasswordInput'tur. Bu dosya, onun
// dışında yeni bir parola girdisi çıkarsa başarısız olur.

const src = fileURLToPath(new URL('../src/', import.meta.url));

const allowed = {
  // The one password input.
  'components/PasswordInput.tsx': 1,
  // The sign-in form is drawn before the screen catalogue loads (shell copy)
  // and already has its own show/hide; a sign-in password is never generated.
  'components/Login.tsx': 1,
  // A license key, not a password: its own show/hide beside the key.
  'components/LicensePanel.tsx': 1,
  // The backup encryption passphrase: two fields under one show/hide, its own
  // cryptographic generator (lib/remoteBackup.ts generatePassphrase) and the
  // "I wrote it down" confirmation; the server's passphrase rule is its own.
  'components/RemoteBackupDestinations.tsx': 2,
};

function sources(dir) {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) return sources(path);
    return /\.tsx?$/.test(name) ? [path] : [];
  });
}

const code = (text) => text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|[^:'"`])\/\/.*$/gm, '$1');
// type="password", type='password', type={'password'}, type={x ? 'text' : 'password'}
const passwordType = /\btype=(?:["']password["']|\{[^}]*["'`]password["'`][^}]*\})/g;

test('every password input is PasswordInput, except the counted exceptions', () => {
  const found = {};
  for (const file of sources(src)) {
    const count = (code(readFileSync(file, 'utf8')).match(passwordType) ?? []).length;
    if (count > 0) found[relative(src, file).split('\\').join('/')] = count;
  }
  assert.deepEqual(found, allowed, 'a password field must be <PasswordInput> (components/PasswordInput.tsx)');
});

test('the guard sees each way of writing a password type', () => {
  const sample = `<input type="password" /><input type='password' /><input type={show ? 'text' : 'password'} /><input type="text" />`;
  assert.equal(sample.match(passwordType).length, 3);
});

test('a new password field turns on the rule; sign-in and current-password fields do not', () => {
  for (const file of sources(src)) {
    const text = code(readFileSync(file, 'utf8'));
    for (const m of text.matchAll(/<PasswordInput\b([^>]|=>)*?\/>/gs)) {
      const tag = m[0];
      const where = relative(src, file);
      if (/autoComplete=["']current-password["']/.test(tag)) {
        assert.doesNotMatch(tag, /\bfield=/, `${where}: a current password gets no rule or generator`);
      }
    }
  }
  // The forms that set a password the panel keeps or the server applies.
  const rule = {
    'components/DomainMailManager.tsx': ['field="mailbox"'],
    'components/DomainDatabaseManager.tsx': ['field="database_user"'],
    'components/DomainFileAccess.tsx': ['field="file_access"'],
    'components/DomainSiteFeatures.tsx': ['field="site_protect"'],
    'components/DomainServiceApps.tsx': ['field="service_app_owner"'],
    'components/UsersPage.tsx': ['field="account"'],
    'components/TeamMembersPage.tsx': ["field='account'"],
    'components/AccountLinkPage.tsx': ['field="account"'],
    'components/ChangePasswordModal.tsx': ['field="account"'],
  };
  for (const [file, needles] of Object.entries(rule)) {
    const text = readFileSync(join(src, file), 'utf8');
    for (const needle of needles) assert.ok(text.includes(needle), `${file} sets a new password with ${needle}`);
    assert.match(text, /passwordAcceptable\(/, `${file} keeps its submit off until the rule is met`);
  }
});
