import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

// alpha.131, the owner's notes on the customer's "Connections from outside"
// card (8 Nov 2026): a new end is saved only with Apply, the row is laid out
// as address, state and a compact control, the customer's text for an
// owner's setting is calm, and the Databases list has no manual refresh.
const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const value = (catalogue, key) => {
  const match = catalogue.match(new RegExp(`'${key.replace(/\./g, '\\.')}':\\s*'((?:[^'\\\\]|\\\\.)*)'`));
  return match ? match[1] : undefined;
};

test('a new end for a listed address is saved only with Apply', () => {
  const card = source('components/DatabaseRemoteAccess.tsx');
  const select = card.slice(card.indexOf("aria-label={t('dbr.extendLabel'"), card.indexOf("{t('dbr.extendApply')}"));
  assert.ok(select.length > 0, 'the end selector and its Apply button');
  assert.doesNotMatch(select.slice(0, select.indexOf('</select>')), /extend\(/, 'picking an option sends nothing');
  assert.match(card, /onClick=\{\(\) => void extend\(rule, chosen\)\}/);
  assert.match(card, /onClick=\{\(\) => discardEnd\(rule\.id\)\}/);
  for (const name of ['en', 'tr']) {
    const words = source(`i18n/dbRemote/${name}.ts`);
    assert.ok(value(words, 'dbr.extendApply') && value(words, 'dbr.extendDiscard'), name);
  }
});

test('an address row keeps its state readable and stacks on a phone', () => {
  const card = source('components/DatabaseRemoteAccess.tsx');
  assert.match(card, /flex flex-col gap-2 px-3 py-2\.5 sm:flex-row/);
  assert.doesNotMatch(card, /inputClass\} w-auto shrink-0 text-xs/, 'the full-width select of alpha.130 is gone');
});

test('the customer reads a calm sentence when an owner setting blocks connections', () => {
  for (const name of ['en', 'tr']) {
    const words = source(`i18n/dbRemote/${name}.ts`);
    for (const key of ['dbr.problem.OWNER_LISTEN.other', 'dbr.problem.OWNER_HBA.other']) {
      const text = value(words, key);
      assert.ok(text, key);
      assert.doesNotMatch(text, /PostgreSQL|MariaDB|\{engine\}|by hand|elle/, `${name} ${key}`);
    }
  }
});

test('the Databases list refreshes itself; no manual refresh button', () => {
  const manager = source('components/DomainDatabaseManager.tsx');
  assert.doesNotMatch(manager, /RefreshCw/);
  assert.ok((manager.match(/invalidate\('databases'\)/g) ?? []).length >= 3);
});
