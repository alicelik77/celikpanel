import assert from 'node:assert/strict';
import test from 'node:test';
import { readFileSync } from 'node:fs';

import { apiErrorText } from '../src/lib/apiError.ts';
import { enDataTools } from '../src/i18n/dataTools/en.ts';
import { trDataTools } from '../src/i18n/dataTools/tr.ts';

// alpha.149, second review of PR #370 finding 16: the Agent's refusal of a
// database account travels as its code (DATABASE_ACCOUNT_REFUSED, reason =
// the refusal, vars.user = the account it names) and the screens that can
// show it word it in the person's language. The words are the data tools'
// route-scoped copy (loaded with the Databases screens), never the shell.

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const reasons = ['privileged', 'member', 'absent', 'unreadable', 'system_database', 'unsupported_engine'];

const say = (words) => (key, vars) => {
  let text = words[key] ?? key;
  for (const [name, value] of Object.entries(vars ?? {})) text = text.split(`{${name}}`).join(String(value));
  return text;
};

test('every refusal reason has its words in English and Turkish', () => {
  for (const words of [enDataTools, trDataTools]) {
    assert.ok(words['err.DATABASE_ACCOUNT_REFUSED']);
    for (const reason of reasons) assert.ok(words[`err.DATABASE_ACCOUNT_REFUSED.${reason}`], reason);
  }
});

test('a Turkish screen says the refusal in Turkish, naming the refused account', () => {
  const problem = { message: 'The database user app_owner holds administrator rights ...', code: 'DATABASE_ACCOUNT_REFUSED', reason: 'privileged', vars: { user: 'app_owner' } };
  const tr = apiErrorText(problem, say(trDataTools));
  assert.match(tr, /^app_owner veritabanı kullanıcısının/);
  assert.match(tr, /Hiçbir şey değiştirilmedi/);
  const en = apiErrorText(problem, say(enDataTools));
  assert.match(en, /The database user app_owner holds administrator rights/);
  const mysql = apiErrorText({ ...problem, reason: 'unsupported_engine' }, say(trDataTools));
  assert.match(mysql, /MariaDB yerine MySQL çalışıyor/);
  // A reason this release has no words for falls back to the code's sentence.
  assert.equal(apiErrorText({ ...problem, reason: 'later' }, say(trDataTools)), trDataTools['err.DATABASE_ACCOUNT_REFUSED']);
});

test('the words load with the Databases screens, not on the boot path', () => {
  assert.doesNotMatch(read('../src/i18n/en.ts'), /DATABASE_ACCOUNT_REFUSED/);
  assert.doesNotMatch(read('../src/i18n/tr.ts'), /DATABASE_ACCOUNT_REFUSED/);
  for (const screen of ['../src/components/DatabaseUsers.tsx', '../src/components/DatabaseUserPassword.tsx']) {
    const source = read(screen);
    assert.match(source, /useDataToolsWords/);
    assert.match(source, /refusalText\(/);
  }
});
