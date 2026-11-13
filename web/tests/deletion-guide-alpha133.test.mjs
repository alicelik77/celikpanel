import assert from 'node:assert/strict';
import test from 'node:test';
import { readFileSync } from 'node:fs';
import { domainDeletionBlocksRetry, domainDeletionGuideReason, readDomainDeletionOutcome, readSavedDomainDeletionState } from '../src/lib/domainDeletionPending.ts';
import { compileSource, dateFormatURL } from './fixtures/shared-layer.mjs';
import { enDeletionGuideScreens as en } from '../src/i18n/screens/deletionGuide/en.ts';
import { trDeletionGuideScreens as tr } from '../src/i18n/screens/deletionGuide/tr.ts';
import { enScreens, trScreens } from './locale-catalogue.mjs';

// alpha.133 release blocker (lab of 12 Nov 2026): the banner of a deletion
// waiting for its DNS step said "nobody needs to do anything" in a state
// nothing repaired, in one grey block of seven lines with a command inline,
// and named a button that is not on that screen. The guide says why in one
// plain sentence, who acts, the next step only when there is one, how it
// continues, and keeps commands and the technical explanation for Details.
// The review round added: the window and the bulk notes follow the server's
// follow_up, point to where the commands really are, and every text names
// only a control the reader has, enabled for that reason.

// lib/deletionGuide.ts with its two real neighbours (Node resolves no extensionless import).
const pendingURL = compileSource('lib/domainDeletionPending.ts', (specifier) => { throw new Error(specifier); });
const { deletionGuideText, deletionGuideWords } = await import(compileSource('lib/deletionGuide.ts', (specifier) => {
  if (specifier.endsWith('/dateFormat')) return dateFormatURL;
  if (specifier.endsWith('/domainDeletionPending')) return pendingURL;
  throw new Error(specifier);
}));
const source = (path) => readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8');
const reasons = ['journalUnknown', 'journalUnresolved', 'proofTimeout', 'catchingUp'];
const peerReasons = ['dns_peer_journal_unknown', 'dns_peer_journal_unresolved', 'dns_peer_proof_timeout', 'dns_peer_catching_up'];
const translate = (catalog) => (key, vars) => {
  let text = catalog[key] ?? key;
  for (const [name, value] of Object.entries(vars ?? {})) text = text.split(`{${name}}`).join(String(value));
  return text;
};

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

// S3 / W8 (alpha.133 review): every reason, follow-up, role and place. A text
// that names More → Delete… is read only by someone whose row offers it,
// enabled for that reason; a text that names Details only by an
// administrator (who has them, in the banner), and outside the banner it
// says where they are; no command or private path outside the banner's
// Details; "nobody needs to act" only while CelikPanel re-checks by itself.
test('each text names only controls its reader has, enabled for its reason', () => {
  const now = Date.parse('2026-11-13T09:00:00Z');
  for (const [screens, words, lang] of [[enScreens, en, 'en'], [trScreens, tr, 'tr']]) {
    const t = translate({ ...screens, ...words });
    const deleteControl = `${screens['kit.rows.more']} → ${screens['domains.action.delete']}`;
    const details = words['domains.deletionGuide.details'];
    const waitingNotice = lang === 'tr' ? 'bekleme kutusu' : 'waiting notice';
    for (const reason of peerReasons) {
      for (const followUp of [undefined, 'automatic', 'stuck', 'stopped', 'person']) {
        for (const admin of [true, false]) {
          for (const place of ['banner', 'window']) {
            const label = `${lang} ${reason} ${followUp} admin=${admin} ${place}`;
            const guide = deletionGuideWords({ reason, followUp, nextCheckAt: '2026-11-13T09:05:00Z' }, admin, t, lang, place, now);
            assert.ok(guide, label);
            const text = deletionGuideText(guide);
            assert.ok(!text.includes('domains.deletionGuide.'), `${label}: missing key in ${text}`);
            if (text.includes(deleteControl)) {
              assert.ok(admin, `${label}: sends a non-administrator to ${deleteControl}`);
              assert.ok(!domainDeletionBlocksRetry(reason), `${label}: names ${deleteControl}, which is disabled for this reason`);
            }
            if (text.includes(details)) {
              assert.ok(admin, `${label}: names ${details} to someone who has none`);
              if (place === 'window') assert.ok(text.includes(waitingNotice), `${label}: Details without where they are`);
            }
            assert.doesNotMatch(text, /sudo|\/var\/lib|journalctl/, `${label}: a command outside Details`);
            assert.equal(text.includes(words['domains.deletionGuide.who.nobody']), guide.automatic, `${label}: nobody-acts wording`);
            assert.ok(!text.includes(screens['domains.retryDeletion']), `${label}: names a button that does not exist`);
          }
        }
      }
    }
  }
});

test('the row offers Delete exactly where the server notification says so', () => {
  // cmd/panel dnsDeletionRowDeleteEnabled must agree with the list's own
  // rule (domainDeletionBlocksRetry) for every peer reason.
  const go = readFileSync(new URL('../../cmd/panel/dns_pending_publication.go', import.meta.url), 'utf8');
  const body = go.slice(go.indexOf('func dnsDeletionRowDeleteEnabled'));
  const disabled = body.slice(0, body.indexOf('return false')).match(/transport\.DNSPeerPending\w+/g) ?? [];
  const goNames = { DNSPeerPendingCatchingUp: 'dns_peer_catching_up' };
  const goDisabled = new Set(disabled.map((name) => goNames[name.replace('transport.', '')] ?? name));
  for (const reason of [...peerReasons, 'dns_peer_owner_edit_unknown', 'dns_peer_native_unknown', 'dns_peer_enrollment_required']) {
    assert.equal(goDisabled.has(reason), domainDeletionBlocksRetry(reason), reason);
  }
});

test('the window points to the banner’s Details and the banner keeps them for administrators', () => {
  const banner = source('components/DeletionWaitingGuide.tsx');
  assert.ok(banner.includes("t('domains.checkDeletionStatus')"));
  assert.ok(!banner.includes('domains.retryDeletion'));
  assert.match(banner, /\{admin && \(\s*<details/);
  assert.ok(!/<details[^>]*\bopen\b/.test(banner));
  const domains = source('components/Domains.tsx');
  // The window and the bulk notes use the window's words and pass what the
  // server said happens next.
  assert.match(domains, /pendingMessage\(result\.reason, result\.detail, result, 'window'\)/);
  assert.match(domains, /pendingEnd\(name, report, saved\.stage, reason, detail, stepSince, next\)/);
  assert.match(domains, /subtitle: t\('domains\.step\.rechecksAlone'\)/);
  assert.equal(domains.split("t('domains.step.rechecksAlone')").length - 1, 1);
});

test('the server notification uses the guide’s words for the unresolved check record', () => {
  const notice = readFileSync(new URL('../../cmd/panel/dns_pending_publication.go', import.meta.url), 'utf8').replaceAll('’', "'");
  const plain = (text) => text.replaceAll('’', "'");
  const enWhy = plain(en['domains.deletionGuide.journalUnresolved.why']).split(', but ')[1].replace(/\.$/, '');
  const trWhy = plain(tr['domains.deletionGuide.journalUnresolved.why']).split('; ancak ')[1].replace(/\.$/, '');
  assert.ok(notice.includes(enWhy), enWhy);
  assert.ok(notice.includes(trWhy), trWhy);
  assert.ok(notice.includes('kendiniz silmeyin') && notice.includes('do not delete those files yourself'));
});

test('the deletion status and the delete answer carry what happens next, reviewed values only', async () => {
  const read = (body) => readSavedDomainDeletionState(new Response(JSON.stringify(body), { status: 200 }));
  assert.deepEqual(await read({ status: 'deletion_pending', stage: 'dns_cleanup', reason: 'dns_peer_journal_unknown',
    follow_up: 'automatic', next_check_at: '2026-11-13T09:05:00Z' }),
  { reason: 'dns_peer_journal_unknown', detail: '', followUp: 'automatic', nextCheckAt: '2026-11-13T09:05:00Z' });
  assert.deepEqual(await read({ status: 'deletion_pending', stage: 'dns_cleanup', reason: 'dns_peer_journal_unresolved', follow_up: 'person' }),
    { reason: 'dns_peer_journal_unresolved', detail: '', followUp: 'person' });
  assert.deepEqual(await read({ status: 'deletion_pending', stage: 'dns_cleanup', reason: 'dns_peer_journal_unknown',
    follow_up: 'whatever', next_check_at: 'soon' }), { reason: 'dns_peer_journal_unknown', detail: '' });
  assert.deepEqual(await readDomainDeletionOutcome(new Response(JSON.stringify({ status: 'deletion_pending', stage: 'dns_cleanup',
    reason: 'dns_peer_proof_timeout', follow_up: 'stuck' }), { status: 202 })), { state: 'pending', reason: 'dns_peer_proof_timeout', followUp: 'stuck' });
  for (const [reason, guide] of [['dns_peer_journal_unknown', 'journalUnknown'], ['dns_peer_journal_unresolved', 'journalUnresolved'],
    ['dns_peer_proof_timeout', 'proofTimeout'], ['dns_peer_catching_up', 'catchingUp'], ['dns_peer_native_unknown', null], ['toString', null]]) {
    assert.equal(domainDeletionGuideReason(reason), guide, reason);
  }
});
