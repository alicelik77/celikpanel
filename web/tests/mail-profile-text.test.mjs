// The mail profiles' names, descriptions and their blocked and warning
// sentences in the interface language (sixteenth record, alpha.84: English text
// on the Turkish Components page and Dashboard card). Since 2026-10-14 the
// Panel types each sentence (blocked_code with blocked_vars, warning_code). The
// list of codes is read from the Panel's own source (mailProfileTextCodes,
// which a Go test keeps complete), so a code the Panel can send without words
// here fails this test instead of falling back to English.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';
import { en } from '../src/i18n/en.ts';
import { tr } from '../src/i18n/tr.ts';
import { enScreens } from '../src/i18n/screens/en.ts';
import { trScreens } from '../src/i18n/screens/tr.ts';
import { enServerScreens } from '../src/i18n/screens/server/en.ts';
import { trServerScreens } from '../src/i18n/screens/server/tr.ts';

const source = readFileSync(new URL('../src/lib/mailProfileText.ts', import.meta.url), 'utf8');
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 } }).outputText;
const lib = await import('data:text/javascript;base64,' + Buffer.from(compiled).toString('base64'));
const server = readFileSync(new URL('../../cmd/panel/mail_profiles.go', import.meta.url), 'utf8');
const catalogues = { en: { ...en, ...enScreens, ...enServerScreens }, tr: { ...tr, ...trScreens, ...trServerScreens } };
// The page's own t: the catalogue's sentence with its values, or the key.
const say = (locale) => (key, vars = {}) => {
  let text = catalogues[locale][key] ?? key;
  for (const [name, value] of Object.entries(vars)) text = text.replaceAll(`{${name}}`, String(value));
  return text;
};

// mailProfileTextCodes in cmd/panel/mail_profiles.go, resolved to its values.
const values = Object.fromEntries([...server.matchAll(/^\t(mailProfile\w+)\s+mailProfileTextCode = "([a-z_]+)"$/gm)].map((m) => [m[1], m[2]]));
const listBody = /var mailProfileTextCodes = \[\]mailProfileTextCode\{([\s\S]*?)\n\}/.exec(server)?.[1] ?? '';
const codes = [...listBody.matchAll(/\b(mailProfile\w+),/g)].map((m) => values[m[1]]);
const placeholders = (text) => [...text.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();

test('every code the Panel can send has an English and a Turkish sentence with the same values', () => {
  assert.ok(codes.length >= 12, `read ${codes.length} codes from the Panel`);
  assert.ok(codes.every(Boolean), `a listed code has no value: ${JSON.stringify(codes)}`);
  for (const code of codes) {
    const key = `services.mailProfiles.code.${code}`;
    assert.ok(catalogues.en[key], `no English sentence for ${code}`);
    assert.ok(catalogues.tr[key], `no Turkish sentence for ${code}`);
    assert.notEqual(catalogues.tr[key], catalogues.en[key], `${code} is not translated`);
    assert.deepEqual(placeholders(catalogues.tr[key]), placeholders(catalogues.en[key]), `${code}: the two sentences use different values`);
    for (const locale of ['en', 'tr']) assert.doesNotMatch(catalogues[locale][key], /scan|tara/i, `${locale} ${code} asks for a scan (D-032)`);
  }
  // The sentence matching that came before the codes is gone.
  assert.doesNotMatch(source, /serverSentences|mailProfileSentence/);
  for (const locale of ['en', 'tr']) assert.equal(Object.keys(catalogues[locale]).filter((key) => key.startsWith('services.mailProfiles.text.')).length, 0);
});

test('a code is said in the page language; its text is the fallback only when there is no usable code', () => {
  const conflict = { id: 'core-mail', name: 'Core Mail', blocked_reason: 'Postfix conflicts with installed Exim.', blocked_code: 'component_conflict', blocked_vars: { component: 'Postfix', conflicting: 'Exim' } };
  assert.equal(lib.mailProfileBlocked(conflict, say('tr')), 'Postfix, kurulu Exim ile çakışıyor.');
  assert.equal(lib.mailProfileBlocked(conflict, say('en')), 'Postfix conflicts with the installed Exim.');
  // An older Panel: no code, its text.
  assert.equal(lib.mailProfileBlocked({ ...conflict, blocked_code: undefined }, say('tr')), conflict.blocked_reason);
  // A code this page has no words for, a value missing, or not the shape: the text.
  assert.equal(lib.mailProfileBlocked({ ...conflict, blocked_code: 'from_a_newer_panel' }, say('tr')), conflict.blocked_reason);
  assert.equal(lib.mailProfileBlocked({ ...conflict, blocked_vars: { component: 'Postfix' } }, say('tr')), conflict.blocked_reason);
  assert.equal(lib.mailProfileBlocked({ ...conflict, blocked_code: 7 }, say('tr')), conflict.blocked_reason);
  assert.equal(lib.mailProfileBlocked({ ...conflict, blocked_vars: { component: 'Postfix', conflicting: 3 } }, say('tr')), conflict.blocked_reason);
  const partial = { id: 'core-mail', name: 'Core Mail', warning: 'Some components are already present…', warning_code: 'partly_present' };
  assert.equal(lib.mailProfileWarning(partial, say('tr')), catalogues.tr['services.mailProfiles.code.partly_present']);
  assert.equal(lib.mailProfileWarning({ ...partial, warning_code: undefined }, say('tr')), partial.warning);
  assert.equal(lib.mailProfileWarning({ id: 'core-mail', name: 'x' }, say('tr')), '');
});

test('the last attempt’s error keeps the server’s words behind a translated lead-in', () => {
  const failed = { id: 'core-mail', name: 'Core Mail', latest_attempt_error: 'submission verification failed' };
  assert.equal(lib.mailProfileAttemptError(failed, say('tr')), 'Son deneme şunu bildirdi: submission verification failed');
  assert.equal(lib.mailProfileAttemptError(failed, say('en')), 'The last attempt reported: submission verification failed');
  assert.equal(lib.mailProfileAttemptError({ id: 'core-mail', name: 'x' }, say('tr')), '');
});

test('names and descriptions come from the catalogue by profile id', () => {
  const core = { id: 'core-mail', name: 'Core Mail', description: 'Postfix SMTP and Dovecot IMAP/POP3 with authenticated submission.' };
  assert.equal(lib.mailProfileName(core, say('tr')), 'Temel Posta');
  assert.equal(lib.mailProfileDescription(core, say('tr')), catalogues.tr['services.mailProfiles.description.coreMail']);
  const other = { id: 'future-mail', name: 'Future Mail', description: 'From a newer server.' };
  assert.equal(lib.mailProfileName(other, say('tr')), 'Future Mail');
  assert.equal(lib.mailProfileDescription(other, say('tr')), 'From a newer server.');
  for (const [key, text] of [['coreMail', 'Postfix SMTP and Dovecot IMAP/POP3 with authenticated submission.'], ['webmail', 'Core mail plus Nginx, PHP-FPM and Roundcube webmail.'], ['protectedMail', 'Core mail with Rspamd spam filtering.']]) {
    assert.equal(catalogues.en[`services.mailProfiles.description.${key}`], text);
    assert.ok(server.includes(`Description: "${text}"`), text);
  }
});

test('a component that cannot be installed names the platform when the Panel sends it, in both languages', () => {
  const blocked = { id: 'core-mail', name: 'Core Mail', blocked_reason: 'Dovecot: the Arch Linux Dovecot package layout is not supported yet', blocked_code: 'component_not_installable', blocked_vars: { component: 'Dovecot', kind: 'distribution', platform: 'Arch Linux (pacman)' } };
  assert.equal(lib.mailProfileBlocked(blocked, say('en')), 'Dovecot cannot be installed automatically on this server’s platform: Arch Linux (pacman).');
  assert.equal(lib.mailProfileBlocked(blocked, say('tr')), 'Dovecot bu sunucunun platformunda otomatik olarak kurulamıyor: Arch Linux (pacman).');
  // An unverified platform, or an older Panel: no platform, the shorter sentence.
  const unnamed = { ...blocked, blocked_vars: { component: 'Dovecot', kind: 'distribution' } };
  assert.equal(lib.mailProfileBlocked(unnamed, say('tr')), catalogues.tr['services.mailProfiles.code.component_not_installable'].replace('{component}', 'Dovecot'));
  assert.equal(lib.mailProfileBlocked({ ...blocked, blocked_vars: { component: 'Dovecot', platform: '  ' } }, say('en')), 'Dovecot cannot be installed automatically on this server.');
  const key = 'services.mailProfiles.code.component_not_installable.platform';
  assert.deepEqual(placeholders(catalogues.tr[key]), placeholders(catalogues.en[key]));
  assert.deepEqual(placeholders(catalogues.en[key]), ['component', 'platform']);
});

test('a role in a requirement is named in the page language, never as its raw token', () => {
  // The roles are the catalogue's own: every ConflictGroup value (a requirement
  // names a role by it), read from internal/core ManagedServices.
  const catalogue = readFileSync(new URL('../../internal/core/managed_services.go', import.meta.url), 'utf8');
  const roles = [...new Set([...catalogue.matchAll(/^\t\tConflictGroup:\s+"([a-z-]+)"/gm)].map((m) => m[1]))];
  assert.ok(roles.length >= 6 && roles.includes('web-server'), `read ${roles.length} roles from the catalogue`);
  for (const role of roles) {
    const key = `services.mailProfiles.role.${role}`;
    assert.ok(catalogues.en[key], `no English name for the role ${role}`);
    assert.ok(catalogues.tr[key], `no Turkish name for the role ${role}`);
    assert.notEqual(catalogues.tr[key], catalogues.en[key], `${role} is not translated`);
  }
  const roleKeys = (locale) => Object.keys(catalogues[locale]).filter((key) => key.startsWith('services.mailProfiles.role.'));
  for (const locale of ['en', 'tr']) assert.equal(roleKeys(locale).length, roles.length, `${locale} names a role the catalogue does not have`);

  const roleRequired = { id: 'webmail', name: 'Webmail', blocked_reason: 'Roundcube requires web-server outside this profile.', blocked_code: 'component_requires', blocked_vars: { component: 'Roundcube', required: 'web-server' } };
  assert.equal(lib.mailProfileBlocked(roleRequired, say('en')), 'Roundcube requires web server, which is not part of this profile.');
  assert.equal(lib.mailProfileBlocked(roleRequired, say('tr')), 'Roundcube için bu profilin parçası olmayan web sunucusu gerekiyor.');
  // A role beside component names: the names stay, the roles are named.
  const mixed = { ...roleRequired, blocked_vars: { component: 'phpMyAdmin', required: 'MariaDB, web-server, PHP-FPM' } };
  assert.equal(lib.mailProfileBlocked(mixed, say('tr')), 'phpMyAdmin için bu profilin parçası olmayan MariaDB, web sunucusu, PHP-FPM gerekiyor.');
  // A role this page has no name for (a newer Panel): words, not the token.
  const newer = { ...roleRequired, blocked_vars: { component: 'Roundcube', required: 'queue-server' } };
  assert.equal(lib.mailProfileBlocked(newer, say('en')), 'Roundcube requires queue server, which is not part of this profile.');
  for (const locale of ['en', 'tr']) assert.doesNotMatch(lib.mailProfileBlocked(roleRequired, say(locale)), /web-server/);
});
