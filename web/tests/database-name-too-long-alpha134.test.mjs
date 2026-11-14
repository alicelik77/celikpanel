import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enScreens, trScreens } from './locale-catalogue.mjs';
import { en as enShell } from '../src/i18n/en.ts';
import { tr as trShell } from '../src/i18n/tr.ts';

// alpha.134 second review: DATABASE_NAME_TOO_LONG reaches every screen that
// can create a database or give a user to one. Each reads it in the catalogue
// it renders refusals with, in both languages and with both of the server's
// values - never the English server sentence on a Turkish screen. Not in the
// shell (the boot path does not grow).
// Bu ret, onu alabilecek her ekranın okuduğu katalogdadır; kabukta değil.

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const key = 'err.DATABASE_NAME_TOO_LONG';

test('every caller that can receive DATABASE_NAME_TOO_LONG has its words in both languages', () => {
  for (const [name, catalogue] of [['screens (en)', enScreens], ['screens (tr)', trScreens]]) {
    assert.equal(typeof catalogue[key], 'string', `${name} has no ${key}`);
    assert.match(catalogue[key], /\{name\}/, name);
    assert.match(catalogue[key], /\{over\}/, name);
  }
  assert.notEqual(enScreens[key], trScreens[key]);
  assert.equal(key in enShell || key in trShell, false, 'the refusal is screen copy, not boot copy');
  // The database tools read their own namespace; it says the same.
  for (const locale of ['en', 'tr']) {
    const source = read(`../src/i18n/dataTools/${locale}.ts`);
    assert.match(source, /'err\.DATABASE_NAME_TOO_LONG': '[^']*\{name\}[^']*\{over\}/, `dataTools/${locale}`);
  }
  // The callers: each reads the server's refusal through readApiError and
  // renders it with the catalogue above (routes wait for the screen copy).
  for (const file of ['../src/components/DomainDatabaseManager.tsx', '../src/components/databases/DatabasesPage.tsx', '../src/components/DatabaseTools.tsx']) {
    assert.match(read(file), /readApiError/, file);
  }
  // The server sends both values with the code.
  const server = read('../../cmd/panel/database_names.go');
  assert.match(server, /errCodeDatabaseNameTooLong,[\s\S]*map\[string\]string\{"name": name, "over": over\}/);
});
