import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { databaseNameLimitsURL } from './fixtures/shared-layer.mjs';
import { enScreens, trScreens } from './locale-catalogue.mjs';

// alpha.134 fix round: the field and the server judge a new database's names
// by one rule. The browser half reads the same cases as the panel and the
// Agent (internal/services/testdata/database-name-limits.json), so a name the
// field lets through is one the server creates, and a name the server refuses
// is stopped in the field with the same number.
// Alan ve sunucu yeni veritabanı adlarını tek kurala göre yargılar.

const rule = await import(databaseNameLimitsURL);
const cases = JSON.parse(readFileSync(new URL('../../internal/services/testdata/database-name-limits.json', import.meta.url), 'utf8'));
const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');

test('the browser counts exactly as the panel and the Agent (shared cases)', () => {
  assert.ok(cases.lengths.length >= 10);
  for (const c of cases.lengths) {
    assert.deepEqual(rule.databaseNamesOver(c.engine, c.database, c.users), { by: c.by, underscores: c.underscores }, c.note);
  }
  assert.ok(cases.characters.length >= 8);
  for (const c of cases.characters) assert.equal(rule.databaseNameCharactersOK(c.name), c.valid, JSON.stringify(c));
});

test('the form shows the server\'s own name and the refusal beside the field, in both languages', () => {
  const form = read('../src/components/DomainDatabaseManager.tsx');
  // The prefix is the server's answer, never a guess from the domain.
  assert.match(form, /listed\?\.value\.namePrefix/);
  assert.doesNotMatch(form, /domainName\.replace\(/);
  assert.match(form, /databaseNamesOver\(dbType, fullName, \[fullName \+ '_user'\]\)/);
  assert.match(read('../src/lib/domainDatabases.ts'), /payload\.name_prefix/);
  assert.match(read('../../cmd/panel/domain_database_handlers.go'), /"name_prefix": sanitizeName\(domain\.Name\) \+ "_"/);
  for (const [name, catalogue] of [['en', enScreens], ['tr', trScreens]]) {
    for (const key of ['ddb.tooLong', 'ddb.tooLongUnderscores']) assert.match(catalogue[key] ?? '', /\{over\}/, `${name} ${key}`);
    assert.equal(typeof catalogue['ddb.nameCharacters'], 'string', `${name} ddb.nameCharacters`);
    assert.match(catalogue['err.DATABASE_NAME_TOO_LONG.length'] ?? '', /\{name\}[\s\S]*\{over\}/, `${name} plain refusal`);
    assert.doesNotMatch(catalogue['err.DATABASE_NAME_TOO_LONG.length'], /underscore|alt çizgi/, `${name} plain refusal speaks of underscores`);
    for (const key of ['err.DATABASE_CREATE_LEFT_OVER', 'err.DATABASE_CREATE_LEFT_OVER.administrator']) {
      assert.match(catalogue[key] ?? '', /\{name\}[\s\S]*\{left\}/, `${name} ${key}`);
    }
    assert.match(catalogue['err.DB_COPY_FAILED.kept'] ?? '', /\{name\}/, `${name} copy kept`);
  }
  assert.notEqual(enScreens['err.DATABASE_CREATE_LEFT_OVER'], enScreens['err.DATABASE_CREATE_LEFT_OVER.administrator']);
});

// Integration: "1 characters" read wrong - the English length sentences need
// no plural form. A catalogue sentence identical to the server's message reads
// as "the server's own words" (refusalParts) and the screen would show a
// generic sentence with the real one under Details: the server words it
// differently.
test('the English length sentences read right for one and for many, and differ from the server\'s words', () => {
  const keys = ['ddb.tooLong', 'ddb.tooLongUnderscores', 'err.DATABASE_NAME_TOO_LONG', 'err.DATABASE_NAME_TOO_LONG.length'];
  for (const key of keys) {
    assert.match(enScreens[key], /too long by \{over\}/, key);
    assert.doesNotMatch(enScreens[key], /\{over\} characters/, key);
  }
  assert.doesNotMatch(read('../src/i18n/dataTools/en.ts'), /\{over\} characters/);
  const go = read('../../cmd/panel/database_names.go');
  assert.doesNotMatch(go, /characters shorter|too long by/);
  assert.match(go, /over the engine's length limit/);
  // The left-over refusal, both readers: the part after {left} is not the
  // server's sentence word for word.
  for (const key of ['err.DATABASE_CREATE_LEFT_OVER', 'err.DATABASE_CREATE_LEFT_OVER.administrator']) {
    const tail = enScreens[key].split('{left}')[1];
    assert.ok(tail && tail.length > 40, key);
    assert.ok(!go.includes(tail), `${key} equals the server's message`);
  }
});
