import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enScreens, trScreens } from './locale-catalogue.mjs';

// alpha.135: an English catalogue sentence that is word for word the panel's
// own message reads as "the server's own words" (refusalParts), and English
// readers get the generic sentence with the real one under Details (found in
// alpha.134). Every refusal code this release adds has its own sentence in
// both languages, and the English one is not the server's message, even with
// curly quotes made straight.
// alpha.135'in yeni ret kodlarının İngilizce cümlesi sunucunun iletisiyle aynı değildir.

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const go = [
  '../../cmd/panel/deletion_holding.go',
  '../../cmd/panel/deletion_holding_left.go',
  '../../cmd/panel/domain_delete_cleanup.go',
].map(read).join('\n');
const straight = (text) => text.replace(/[’‘]/g, "'").replace(/[“”]/g, '"');

test('the refusals alpha.135 adds are worded by the screen, not by the server', () => {
  const codes = ['DOMAIN_DELETION_PLAN_FIXED', 'HOLDING_LEFT_FAILED', 'HOLDING_LEFT_IN_USE', 'HOLDING_LEFT_UNKNOWN',
    'HOLDING_REMOVE_FAILED', 'HOLDING_REMOVE_UNKNOWN'];
  for (const code of codes) {
    assert.ok(go.includes(`"${code}"`) || go.includes('errCodeDomainDeletionPlanFixed'), `${code} is not the panel's`);
    const en = enScreens[`err.${code}`];
    assert.ok(en && trScreens[`err.${code}`], `${code} has no sentence in both languages`);
    for (const part of straight(en).split(/\{[a-z_]+\}/)) {
      if (part.trim().length < 20) continue;
      assert.ok(!straight(go).includes(part.trim()), `err.${code} is the server's message word for word: ${part.trim()}`);
    }
  }
});
