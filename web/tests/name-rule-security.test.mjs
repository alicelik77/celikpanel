// Security release (20 November 2026): the one name rule and the Node.js
// port rule. A customer's refusal says only the name it asked for, the same
// whoever holds it; the administrator's reasons, the port refusal and the
// three "Problems now" lines exist in English and Turkish with the same
// placeholders.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enNameRuleScreens as enErrorsScreens } from '../src/i18n/screens/nameRule/en.ts';
import { trNameRuleScreens as trErrorsScreens } from '../src/i18n/screens/nameRule/tr.ts';
import { enProblemsNowScreens } from '../src/i18n/screens/problemsNow/en.ts';
import { trProblemsNowScreens } from '../src/i18n/screens/problemsNow/tr.ts';
import { enTransfersScreens } from '../src/i18n/screens/transfers/en.ts';
import { trTransfersScreens } from '../src/i18n/screens/transfers/tr.ts';

const placeholders = (text) => [...text.matchAll(/\{([a-z]+)\}/g)].map((m) => m[1]).sort();

test('a customer is told only the name, the same whoever holds it', () => {
  for (const words of [enErrorsScreens, trErrorsScreens]) {
    assert.deepEqual(placeholders(words['err.HOSTNAME_NOT_AVAILABLE']), ['name']);
  }
});

test('every reason, the port refusal and the transfer refusal exist in both languages with the same placeholders', () => {
  const keys = ['own_name', 'other_account', 'server_name', 'remote_zone', 'account_taken']
    .map((reason) => `err.HOSTNAME_NOT_AVAILABLE.${reason}`)
    .concat(['err.HOSTNAME_NOT_AVAILABLE', 'err.APP_PORT_NOT_ALLOWED']);
  for (const key of keys) {
    assert.ok(enErrorsScreens[key] && trErrorsScreens[key], key);
    assert.deepEqual(placeholders(trErrorsScreens[key]), placeholders(enErrorsScreens[key]), key);
  }
  assert.deepEqual(placeholders(enErrorsScreens['err.APP_PORT_NOT_ALLOWED']), ['max', 'min']);
  assert.ok(enTransfersScreens['err.TRANSFER_NAME_OVERLAP'] && trTransfersScreens['err.TRANSFER_NAME_OVERLAP']);
});

test('Problems now has words for the names and ports an installation already holds', () => {
  const source = readFileSync(new URL('../src/components/ProblemsNow.tsx', import.meta.url), 'utf8');
  for (const kind of ['name_overlap', 'name_server_reserved', 'node_port_reserved']) {
    assert.match(source, new RegExp(`'${kind}'`));
    for (const part of ['title', 'body', 'action']) {
      const key = `pnow.${kind}.${part}`;
      assert.ok(enProblemsNowScreens[key] && trProblemsNowScreens[key], key);
      assert.deepEqual(placeholders(trProblemsNowScreens[key]), placeholders(enProblemsNowScreens[key]), key);
    }
  }
  assert.match(source, /other: problem\.name/);
  assert.ok(enProblemsNowScreens['pnow.source.names'] && trProblemsNowScreens['pnow.source.names']);
});
