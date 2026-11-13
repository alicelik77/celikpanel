import assert from 'node:assert/strict';
import test from 'node:test';
import { readFileSync } from 'node:fs';
import { domainDeletionGuideReason, readSavedDomainDeletionState } from '../src/lib/domainDeletionPending.ts';
import { enDeletionGuideScreens as en } from '../src/i18n/screens/deletionGuide/en.ts';
import { trDeletionGuideScreens as tr } from '../src/i18n/screens/deletionGuide/tr.ts';
import { enScreens, trScreens } from './locale-catalogue.mjs';

// alpha.133 release blocker (lab of 12 Nov 2026): the banner of a deletion
// waiting for its DNS step said "nobody needs to do anything" in a state
// nothing repaired, in one grey block of seven lines with a command inline,
// and named a button that is not on that screen. The guide says why in one
// plain sentence, who acts, the next step only when there is one, how it
// continues, and keeps commands and the technical explanation for Details.

const source = (path) => readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8');
const reasons = ['journalUnknown', 'journalUnresolved', 'proofTimeout', 'catchingUp'];

test('every waiting reason has one plain why sentence in both languages', () => {
  for (const reason of reasons) {
    for (const [lang, words] of [['en', en], ['tr', tr]]) {
      const why = words[`domains.deletionGuide.${reason}.why`];
      assert.ok(why, `${lang} ${reason}`);
      assert.equal(why.split(/[.!?](\s|$)/).filter((part) => part && part.trim()).length, 1, `${lang} ${reason}: more than one sentence`);
      assert.doesNotMatch(why, /challenge|journal|nonce|proof|dns_peer|_unknown|journalctl|sudo|\//i, `${lang} ${reason}: jargon`);
      assert.ok(words[`domains.deletionGuide.${reason}.details`], `${lang} ${reason}: details`);
    }
  }
  assert.equal(tr['domains.deletionGuide.who.nobody'], 'Sizin bir şey yapmanız gerekmiyor.');
});

test('the guide names only the controls that exist, by their labels', () => {
  for (const [words, screens] of [[en, enScreens], [tr, trScreens]]) {
    const deleteControl = `${screens['kit.rows.more']} → ${screens['domains.action.delete']}`;
    for (const key of ['domains.deletionGuide.then.person', 'domains.deletionGuide.then.stopped']) {
      assert.ok(words[key].includes(deleteControl), `${key} does not name ${deleteControl}`);
    }
    for (const text of Object.values(words)) {
      assert.ok(!text.includes(screens['domains.retryDeletion']), `names a button that is not on the page: ${text}`);
    }
  }
  const banner = source('components/DeletionWaitingGuide.tsx');
  assert.ok(banner.includes("t('domains.checkDeletionStatus')"));
  assert.ok(!banner.includes('domains.retryDeletion'));
  // The commands are behind a closed Details, for an administrator only.
  assert.match(banner, /\{admin && \(\s*<details/);
  assert.ok(!/<details[^>]*\bopen\b/.test(banner));
});

test('"nobody needs to act" only while CelikPanel re-checks it by itself', () => {
  const banner = source('components/DeletionWaitingGuide.tsx');
  const automatic = banner.slice(banner.indexOf('default: {'));
  assert.ok(automatic.includes("k('who.nobody')"));
  assert.equal(banner.split("k('who.nobody')").length - 1, 1, 'who.nobody outside the automatic follow-up');
  assert.match(banner, /reason === 'dns_peer_journal_unresolved' \? 'person' : 'automatic'/);
});

test('the server notification uses the guide’s words for the unresolved check record', () => {
  const notice = readFileSync(new URL('../../cmd/panel/dns_pending_publication.go', import.meta.url), 'utf8').replaceAll('’', "'");
  const plain = (text) => text.replaceAll('’', "'");
  // The why sentence after "The domain is removed from DNS on this server, but ".
  const enWhy = plain(en['domains.deletionGuide.journalUnresolved.why']).split(', but ')[1].replace(/\.$/, '');
  const trWhy = plain(tr['domains.deletionGuide.journalUnresolved.why']).split('; ancak ')[1].replace(/\.$/, '');
  assert.ok(notice.includes(enWhy), enWhy);
  assert.ok(notice.includes(trWhy), trWhy);
  assert.ok(notice.includes('kendiniz silmeyin') && notice.includes('do not delete those files yourself'));
});

test('the deletion status carries what happens next, reviewed values only', async () => {
  const read = (body) => readSavedDomainDeletionState(new Response(JSON.stringify(body), { status: 200 }));
  assert.deepEqual(await read({ status: 'deletion_pending', stage: 'dns_cleanup', reason: 'dns_peer_journal_unknown',
    follow_up: 'automatic', next_check_at: '2026-11-13T09:05:00Z' }),
  { reason: 'dns_peer_journal_unknown', detail: '', followUp: 'automatic', nextCheckAt: '2026-11-13T09:05:00Z' });
  assert.deepEqual(await read({ status: 'deletion_pending', stage: 'dns_cleanup', reason: 'dns_peer_journal_unresolved', follow_up: 'person' }),
    { reason: 'dns_peer_journal_unresolved', detail: '', followUp: 'person' });
  assert.deepEqual(await read({ status: 'deletion_pending', stage: 'dns_cleanup', reason: 'dns_peer_journal_unknown',
    follow_up: 'whatever', next_check_at: 'soon' }), { reason: 'dns_peer_journal_unknown', detail: '' });
  for (const [reason, guide] of [['dns_peer_journal_unknown', 'journalUnknown'], ['dns_peer_journal_unresolved', 'journalUnresolved'],
    ['dns_peer_proof_timeout', 'proofTimeout'], ['dns_peer_catching_up', 'catchingUp'], ['dns_peer_native_unknown', null], ['toString', null]]) {
    assert.equal(domainDeletionGuideReason(reason), guide, reason);
  }
});
