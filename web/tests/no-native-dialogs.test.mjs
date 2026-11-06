import assert from 'node:assert/strict';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

// Owner report, 6 Nov 2026: deleting a domain showed the browser's own
// dialogue ("boston.celikhost.com:1923 says — Delete …? OK / Cancel"). "I don't
// want messages like this. If a message is needed, we must show it ourselves."
// Every question now goes through lib/confirmAction.ts and the ConfirmHost at
// the application root; every notice through showToast or the page itself.
// This file keeps the browser's confirm, alert and prompt out of web/src.
//
// Sahip, 6 Kas 2026: tarayıcının kendi penceresi istenmiyor. Her soru
// lib/confirmAction.ts ve kökteki ConfirmHost üzerinden sorulur. Bu dosya
// tarayıcının confirm, alert ve prompt pencerelerini web/src dışında tutar.

const src = fileURLToPath(new URL('../src/', import.meta.url));
const native = ['confirm', 'alert', 'prompt'];

function sources(dir) {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) return sources(path);
    return /\.(tsx?|mjs|js)$/.test(name) ? [path] : [];
  });
}

// Comments are not calls.
const code = (text) => text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|[^:'"`])\/\/.*$/gm, '$1');

test('no screen opens the browser\'s own confirm, alert or prompt dialogue', () => {
  const found = [];
  for (const file of sources(src)) {
    const text = code(readFileSync(file, 'utf8'));
    for (const name of native) {
      // window.confirm(…), globalThis.alert(…), self.prompt(…)
      for (const m of text.matchAll(new RegExp(`\\b(?:window|globalThis|self)\\s*\\.\\s*${name}\\s*\\(`, 'g'))) {
        found.push(`${relative(src, file)}: ${m[0]}`);
      }
      // A bare call is the browser's unless the file declares its own binding
      // of that name (a dialogue's own "confirm" handler).
      const local = new RegExp(`(?:function\\s+${name}\\b|(?:const|let|var)\\s+${name}\\s*=)`).test(text);
      if (local) continue;
      for (const m of text.matchAll(new RegExp(`(?<![\\w.$])${name}\\s*\\(`, 'g'))) {
        found.push(`${relative(src, file)}: ${m[0]}`);
      }
    }
  }
  assert.deepEqual(found, [], `use confirmAction (lib/confirmAction.ts) or showToast instead: ${found.join(', ')}`);
});

test('the guard itself sees a native call', () => {
  const sample = code("const ok = window.confirm('x'); if (!confirm('y')) alert('z'); // confirm('in a comment')");
  assert.equal([...sample.matchAll(/\b(?:window|globalThis|self)\s*\.\s*confirm\s*\(/g)].length, 1);
  assert.equal([...sample.matchAll(/(?<![\w.$])confirm\s*\(/g)].length, 1);
  assert.equal([...sample.matchAll(/(?<![\w.$])alert\s*\(/g)].length, 1);
});

test('the confirmation is mounted once at the root and asks one question at a time', () => {
  const main = readFileSync(join(src, 'main.tsx'), 'utf8');
  assert.match(main, /<ConfirmHost \/>/);
  const host = readFileSync(join(src, 'components/ConfirmHost.tsx'), 'utf8');
  // A second request while one is open is answered "no" at once.
  assert.match(host, /if \(openRef\.current\) \{\s*resolve\(false\);/);
  // The shared dialogue shape, its busy state and the shell's own words.
  assert.match(host, /<Dialog[\s\S]*busy=\{busy\}/);
  assert.match(host, /t\('common\.cancel'\)/);
  assert.match(host, /t\('common\.continue'\)/);
});
