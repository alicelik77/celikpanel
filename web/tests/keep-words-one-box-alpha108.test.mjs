// alpha.108 (owner finding C6): Frankfurt's phpMyAdmin install refusal drew
// its sentence in three narrow columns ("Sury PHP deposu dağıtımın" /
// "php-common" / "paketlerini…") because KeepWords returned a fragment and the
// refusal's row is a flex row with an icon: every text run and every kept
// word became its own flex item. The sentence is now one box, so beside an
// icon it takes the row's remaining width at 390 and at 1440.
// Cümle tek kutudur; esnek satırda parçalar ayrı sütun olmaz.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const ui = readFileSync(new URL('../src/components/ui.tsx', import.meta.url), 'utf8');
const preview = readFileSync(new URL('../src/components/InstallPreviewNotes.tsx', import.meta.url), 'utf8');

function keepWordsBody() {
  const start = ui.indexOf('export function KeepWords(');
  assert.ok(start >= 0, 'KeepWords exists');
  const end = ui.indexOf('\n}\n', start);
  return ui.slice(start, end);
}

test('KeepWords renders one inline box, never a fragment of separate pieces', () => {
  const body = keepWordsBody();
  assert.match(body, /return \(\s*<span data-keep-words="">/);
  assert.doesNotMatch(body, /<>/, 'a fragment would split the sentence into flex items');
  // The kept words stay unbroken inside it.
  assert.match(body, /className="whitespace-nowrap"/);
});

test('the refusal that showed the columns puts the sentence beside its icon as one flex item', () => {
  // The flex row with an icon column, then exactly one KeepWords child.
  assert.match(preview, /<li key=\{group\.key\} className="flex items-start gap-2"[^>]*>\s*<AlertTriangle className="mt-1 h-4 w-4 shrink-0 text-danger" aria-hidden="true" \/>\s*<KeepWords/);
});
