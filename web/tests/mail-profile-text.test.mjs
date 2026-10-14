// The mail profiles' names, descriptions and the server's profile sentences in
// the interface language (sixteenth record, alpha.84: English text on the
// Turkish Components page and Dashboard card). The sentences this page has
// words for are matched against the server's own source, so a server wording
// change shows up here instead of silently falling back to English.
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
const say = (locale) => (key) => catalogues[locale][key] ?? key;
const sentences = [...source.matchAll(/^    '([^']+)': 'services\.mailProfiles\.text\.(\w+)',$/gm)].map((m) => [m[1], 'services.mailProfiles.text.' + m[2]]);

test('every server sentence the page translates is written that way by the server', () => {
  assert.ok(sentences.length >= 7, `found ${sentences.length}`);
  for (const [sentence] of sentences) assert.ok(server.includes(`"${sentence}"`) || server.includes(`= "${sentence}"`), `the server no longer writes: ${sentence}`);
});

test('the profile text is said in Turkish on a Turkish page, and unknown text is kept as the server wrote it', () => {
  for (const [sentence, key] of sentences) {
    assert.ok(catalogues.en[key] && catalogues.tr[key], key);
    assert.notEqual(catalogues.tr[key], catalogues.en[key], `${key} is not translated`);
    assert.equal(lib.mailProfileSentence(sentence, say('tr')), catalogues.tr[key]);
  }
  assert.equal(lib.mailProfileSentence('Postfix: package unavailable', say('tr')), 'Postfix: package unavailable');
  const core = { id: 'core-mail', name: 'Core Mail', description: 'Postfix SMTP and Dovecot IMAP/POP3 with authenticated submission.' };
  assert.equal(lib.mailProfileName(core, say('tr')), 'Temel Posta');
  assert.equal(lib.mailProfileDescription(core, say('tr')), catalogues.tr['services.mailProfiles.description.coreMail']);
  assert.equal(lib.mailProfileDescription(core, say('en')), core.description, 'the English words are the server’s');
  for (const id of ['webmail', 'protected-mail']) assert.notEqual(lib.mailProfileDescription({ id, name: id }, say('tr')), '');
  const other = { id: 'future-mail', name: 'Future Mail', description: 'From a newer server.' };
  assert.equal(lib.mailProfileName(other, say('tr')), 'Future Mail');
  assert.equal(lib.mailProfileDescription(other, say('tr')), 'From a newer server.');
});

test('the English descriptions match the server’s compiled ones', () => {
  for (const [key, text] of [['coreMail', 'Postfix SMTP and Dovecot IMAP/POP3 with authenticated submission.'], ['webmail', 'Core mail plus Nginx, PHP-FPM and Roundcube webmail.'], ['protectedMail', 'Core mail with Rspamd spam filtering.']]) {
    assert.equal(catalogues.en[`services.mailProfiles.description.${key}`], text);
    assert.ok(server.includes(`Description: "${text}"`), text);
  }
});
