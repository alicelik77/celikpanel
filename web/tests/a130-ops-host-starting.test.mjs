// alpha.130 lab: a change sent while the server is still booting is said in
// both languages (HOST_STARTING), never as a bare server error.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

test('HOST_STARTING has its words in English and Turkish', () => {
  for (const lang of ['en', 'tr']) {
    const words = readFileSync(new URL(`../src/i18n/screens/errors/${lang}.ts`, import.meta.url), 'utf8');
    const line = words.split('\n').find((l) => l.includes("'err.HOST_STARTING'"));
    assert.ok(line, lang);
    assert.match(line, /systemctl is-system-running/);
  }
  const go = readFileSync(new URL('../../cmd/panel/httperr.go', import.meta.url), 'utf8');
  assert.match(go, /errCodeHostStarting = "HOST_STARTING"/);
});
