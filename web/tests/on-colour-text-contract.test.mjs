import assert from 'node:assert/strict';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

// The "-fg" colours (text-primary-fg, text-success-fg, text-warning-fg,
// text-danger-fg) are the text colour ON that colour's own fill: white or
// near-black, chosen for the solid background. Used as a text colour on an
// ordinary surface they vanish (lab, 12 Nov 2026: a PHP value's over-limit
// message was white on the white card). This test reads every class string
// in web/src and fails when such a token is not on an element that also has
// that colour's solid fill (bg-danger, or bg-danger/80 and more opaque).
// For danger text on a normal surface the token is text-danger.
//
// "-fg" renkleri yalnız kendi dolgulu zemininde kullanılır; normal yüzeyde
// metin rengi olarak görünmez olur.

const src = fileURLToPath(new URL('../src/', import.meta.url));
const files = (dir) => readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    return statSync(path).isDirectory() ? files(path) : /\.(tsx|ts)$/.test(name) ? [path] : [];
});

// The string literal (quote, double quote or template) around position `at`.
function literalAround(text, at) {
    let start = at;
    while (start > 0 && !`'"\``.includes(text[start - 1])) start--;
    let end = at;
    while (end < text.length && !`'"\``.includes(text[end])) end++;
    return text.slice(start, end);
}

test('an on-colour text token is used only on its own solid fill', () => {
    const misuse = [];
    for (const path of files(src)) {
        const text = readFileSync(path, 'utf8');
        for (const match of text.matchAll(/\btext-(primary|success|warning|danger)-fg\b/g)) {
            const classes = literalAround(text, match.index);
            const fill = new RegExp(`(^|\\s|:)bg-${match[1]}(/(8\\d|9\\d|100))?(\\s|$)`);
            if (!fill.test(classes)) {
                const line = text.slice(0, match.index).split('\n').length;
                misuse.push(`${relative(src, path).split('\\').join('/')}:${line} ${match[0]} in "${classes.trim()}"`);
            }
        }
    }
    assert.deepEqual(misuse, [], `An "-fg" colour is text on its own fill only; use text-${'<colour>'} on a normal surface.\n${misuse.join('\n')}`);
});
