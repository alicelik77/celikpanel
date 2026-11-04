// alpha.119 text fixes: product texts that disagreed with the real screens
// (FACTS items 3, 4, 6, 7, 13), in English and Turkish.
// alpha.119 metin duzeltmeleri: gercek ekranlarla celisen urun metinleri.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { en } from '../src/i18n/en.ts';
import { tr } from '../src/i18n/tr.ts';
import { enScreens, trScreens } from './locale-catalogue.mjs';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const all = { en: { ...en, ...enScreens }, tr: { ...tr, ...trScreens } };

test('plans hint: traffic is measured monthly for websites, not "not measured"', () => {
  assert.doesNotMatch(all.en['plans.hint'], /not measured/i);
  assert.match(all.en['plans.hint'], /counted/);
  assert.doesNotMatch(all.tr['plans.hint'], /ölçülmüyor/);
  assert.match(all.tr['plans.hint'], /sayılır/);
});

test('setup: the join pointer names the real option, in both languages', () => {
  assert.equal(all.en['setup.v6.kind.join'], 'Join an existing server');
  assert.ok(all.en['setup.v5.card.next'].includes(`“${all.en['setup.v6.kind.join']}”`));
  assert.ok(all.tr['setup.v5.card.next'].includes(`“${all.tr['setup.v6.kind.join']}”`));
  for (const locale of ['en', 'tr']) assert.ok(!/first server with a code|İlk sunucuya kodla/.test(all[locale]['setup.v5.card.next']));
});

test('update screen: the rollback outcome says which data comes back', () => {
  assert.match(all.en['panelUpdate.outcome.rolledBack'], /copy taken before the update/);
  assert.match(all.en['panelUpdate.outcome.rolledBack'], /not kept/);
  assert.match(all.tr['panelUpdate.outcome.rolledBack'], /güncelleme öncesi alınan kopyadan/);
  assert.match(all.tr['panelUpdate.outcome.rolledBack'], /korunmaz/);
  assert.match(read('../src/lib/systemUpdateOutcome.ts'), /key: 'panelUpdate\.outcome\.rolledBack', vars: versions/);
});

test('file manager: an unusable request has its own words, and the screen maps the code', () => {
  assert.match(read('../src/components/DomainFileManager.tsx'), /case 'FILE_REQUEST_INVALID': return t\('files\.requestInvalid'\)/);
  for (const locale of ['en', 'tr']) assert.ok(all[locale]['files.requestInvalid'].length > 40);
  assert.notEqual(all.en['files.requestInvalid'], all.tr['files.requestInvalid']);
});

test('docs: the built-in Unlimited plans are fixed rows (P4), not editable', () => {
  assert.match(read('../../docs/PLANS-AND-RIGHTS.md'), /cannot be deleted or edited/);
  assert.match(read('../../docs/PLANS-AND-RIGHTS.tr.md'), /silinemez ve düzenlenemez/);
  assert.doesNotMatch(read('../../docs/PLANS-AND-RIGHTS.md'), /administrator can change their limits and/);
});
