// Every date and time in the interface follows the selected interface language
// (owner report, 2026-11-08: the Turkish Components page showed
// "11/8/2026 5:20:51 AM" because the call named no locale and the browser's own
// locale won). This guard fails when source calls toLocaleString,
// toLocaleDateString or toLocaleTimeString with no argument at all, and holds
// the one formatter (src/lib/dateFormat.ts) to its contract.
//
// Arayüzdeki her tarih ve saat seçili arayüz dilini izler; dil vermeyen çağrı bu
// testte kalır.
import assert from 'node:assert/strict';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';
import ts from 'typescript';

const srcRoot = fileURLToPath(new URL('../src', import.meta.url));
const walk = (dir) => readdirSync(dir).flatMap((name) => {
  const path = join(dir, name);
  if (statSync(path).isDirectory()) return walk(path);
  return /\.(ts|tsx)$/.test(name) ? [path] : [];
});

test('no date is formatted without a locale', () => {
  const offenders = [];
  for (const file of walk(srcRoot)) {
    readFileSync(file, 'utf8').split('\n').forEach((line, index) => {
      if (/\.toLocale(?:Date|Time)?String\(\s*\)/.test(line)) offenders.push(`${file.slice(srcRoot.length + 1)}:${index + 1}: ${line.trim()}`);
    });
  }
  assert.deepEqual(offenders, [], 'pass the interface locale through src/lib/dateFormat.ts');
});

const compiled = ts.transpileModule(readFileSync(new URL('../src/lib/dateFormat.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 } }).outputText;
const { localeTag, formatDateTime, formatDate, formatTime } = await import('data:text/javascript;base64,' + Buffer.from(compiled).toString('base64'));

test('the formatter has no imports, so a boot file can take it for almost nothing', () => {
  assert.doesNotMatch(readFileSync(new URL('../src/lib/dateFormat.ts', import.meta.url), 'utf8'), /^\s*import\s/m);
});

test('localeTag maps the interface language', () => {
  assert.equal(localeTag('tr'), 'tr-TR');
  assert.equal(localeTag('en'), 'en-US');
  assert.equal(localeTag('de'), 'en-US');
});

test('the same moment reads Turkish in the Turkish interface and English in the English one', () => {
  const at = new Date(Date.UTC(2026, 10, 8, 5, 20, 51));
  const tr = formatDateTime(at, 'tr', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'UTC' });
  const en = formatDateTime(at, 'en', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'UTC' });
  assert.match(tr, /Kas/);
  assert.match(tr, /05:20/);
  assert.match(en, /Nov/);
  assert.match(en, /5:20/);
  assert.doesNotMatch(tr, /AM|PM/);
  assert.equal(formatDate(at, 'tr', { dateStyle: 'medium', timeZone: 'UTC' }), '8 Kas 2026');
  assert.equal(formatTime(at, 'tr', { hour: '2-digit', minute: '2-digit', timeZone: 'UTC' }), '05:20');
});

test('missing and invalid input is empty, never "Invalid Date"', () => {
  for (const value of [undefined, null, '', 'not a date', Number.NaN, new Date('x')]) {
    assert.equal(formatDateTime(value, 'tr'), '');
    assert.equal(formatDate(value, 'tr'), '');
    assert.equal(formatTime(value, 'tr'), '');
  }
});

test('input forms: ISO, SQLite text without a zone (UTC), epoch milliseconds and Date', () => {
  const options = { dateStyle: 'short', timeStyle: 'medium', timeZone: 'UTC' };
  const expected = formatDateTime(new Date(Date.UTC(2026, 10, 8, 5, 20, 51)), 'tr', options);
  assert.equal(formatDateTime('2026-11-08T05:20:51Z', 'tr', options), expected);
  assert.equal(formatDateTime('2026-11-08T07:20:51+02:00', 'tr', options), expected);
  assert.equal(formatDateTime('2026-11-08 05:20:51', 'tr', options), expected);
  assert.equal(formatDateTime(Date.UTC(2026, 10, 8, 5, 20, 51), 'tr', options), expected);
  assert.equal(formatDateTime(new Date(Date.UTC(2026, 10, 8, 5, 20, 51)), 'tr', options), expected);
});
