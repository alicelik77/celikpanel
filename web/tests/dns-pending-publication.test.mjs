import assert from 'node:assert/strict';
import test from 'node:test';
import { apiErrorText } from '../src/lib/apiError.ts';
import { decodeDNSEngineSnapshot, decodeDNSPendingPublications, pendingPublicationText } from '../src/lib/dnsEngineContract.ts';
import { domainDeletionReasonKey, readDomainDeletionOutcome, readSavedDomainDeletionState, readSavedDomainDeletionStatus } from '../src/lib/domainDeletionPending.ts';
import { enScreens, trScreens } from './locale-catalogue.mjs';
import { dnsEngineEn, dnsEngineText, dnsEngineTr } from '../src/i18n/dnsEngine.ts';
import { enErrorsScreens as en } from '../src/i18n/screens/errors/en.ts';
import { trErrorsScreens as tr } from '../src/i18n/screens/errors/tr.ts';

// 2026-10-30: a paired publication whose backup has not caught up is re-checked
// by CelikPanel itself. Owner text names the reason, says nobody needs to act,
// that it resumes by itself and what to check if it lasts; no internal codes.

const internal = /dns_peer|V3|lease|ledger|journal|RecoverDNSZone|DNS_PUBLICATION|request_id|\{\w+\}/;

function response(status, body) {
  return new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } });
}

test('the "backup has not caught up" pending reason has EN/TR guidance: reason, nobody acts, resumes by itself, what to check', () => {
  const key = 'err.DNS_PUBLICATION_FAILED.dns_peer_catching_up';
  assert.match(en[key], /has not caught up/);
  assert.match(en[key], /Nobody needs to do anything/);
  assert.match(en[key], /re-checks this same publication by itself/);
  assert.match(en[key], /TCP port 53/);
  assert.match(tr[key], /henüz yetişmedi/);
  assert.match(tr[key], /Kimsenin bir şey yapması gerekmiyor/);
  assert.match(tr[key], /aynı yayını kendiliğinden yeniden denetler/);
  assert.match(tr[key], /TCP 53/);
  for (const text of [en[key], tr[key]]) assert.doesNotMatch(text, internal);
});

test('proof timeout and challenge texts no longer say nothing retries by itself', () => {
  for (const reason of ['dns_peer_proof_timeout', 'dns_peer_journal_unknown']) {
    const key = `err.DNS_PUBLICATION_FAILED.${reason}`;
    assert.doesNotMatch(en[key], /Nothing retries by itself/);
    assert.doesNotMatch(tr[key], /Kendiliğinden yeniden denenmez/);
    assert.match(en[key], /re-checks this same publication by itself/);
    assert.match(tr[key], /aynı yayını kendiliğinden yeniden denetler/);
  }
  // Causes that need a person still say so.
  assert.match(en['err.DNS_PUBLICATION_FAILED.dns_peer_enrollment_required'], /Nothing retries by itself/);
  assert.match(en['err.DNS_PUBLICATION_FAILED.dns_peer_proof_internal'], /Nothing retries by itself/);
});

test('a held change names its cause and says it is published by itself afterwards', () => {
  const t = (catalog) => (key, vars) => {
    let text = catalog[key] ?? key;
    for (const [name, value] of Object.entries(vars ?? {})) text = text.split(`{${name}}`).join(String(value));
    return text;
  };
  for (const [catalog, nobody, itself] of [
    [en, /Nobody needs to do anything/, /publishes this one too/],
    [tr, /kimsenin bir şey yapması gerekmiyor/i, /bunu da yayımlar/],
  ]) {
    const caught = apiErrorText({ message: 'server text', code: 'DNS_PUBLICATION_HELD', reason: 'dns_peer_catching_up', vars: { zone: 'a.example' } }, t(catalog));
    assert.match(caught, nobody);
    assert.match(caught, itself);
    assert.doesNotMatch(caught, internal);
    for (const reason of ['dns_peer_proof_timeout', 'dns_peer_journal_unknown', 'pending_verification']) {
      const text = apiErrorText({ message: 'server text', code: 'DNS_PUBLICATION_HELD', reason }, t(catalog));
      assert.match(text, itself);
      assert.doesNotMatch(text, internal);
    }
    // A cause that needs a person keeps the general held text: no promise of
    // an automatic re-check of the earlier change, but this change is
    // published by itself once the earlier one finishes.
    const owner = apiErrorText({ message: 'server text', code: 'DNS_PUBLICATION_HELD', reason: 'dns_peer_enrollment_required' }, t(catalog));
    assert.equal(owner, catalog['err.DNS_PUBLICATION_HELD']);
    assert.doesNotMatch(owner, /save this change again|yeniden kaydedin/);
  }
});

test('a deletion pending because the backup has not caught up is a reviewed reason', async () => {
  assert.deepEqual(await readDomainDeletionOutcome(response(202, {
    status: 'deletion_pending', stage: 'dns_cleanup', reason: 'dns_peer_catching_up',
  })), { state: 'pending', reason: 'dns_peer_catching_up' });
  assert.equal(await readSavedDomainDeletionStatus(response(200, {
    status: 'deletion_pending', stage: 'dns_cleanup', reason: 'dns_peer_catching_up',
  })), 'dns_peer_catching_up');
});

test('a deletion whose DNS removal completed by itself names the one action left', async () => {
  const saved = await readSavedDomainDeletionState(response(200, {
    status: 'deletion_pending', stage: 'dns_removed', reason: 'dns_removal_confirmed',
  }));
  assert.deepEqual(saved, { reason: 'dns_removal_confirmed', detail: '' });
  assert.deepEqual(await readSavedDomainDeletionState(response(200, {
    status: 'deletion_pending', stage: 'dns_removed', reason: 'something else',
  })), { reason: '', detail: '' });
  const key = domainDeletionReasonKey('dns_removal_confirmed');
  assert.equal(key, 'domains.deletionDNSRemoved');
  assert.ok(enScreens[key].includes(`“${enScreens['domains.retryDeletion']}”`));
  assert.ok(trScreens[key].includes(`“${trScreens['domains.retryDeletion']}”`));
  for (const text of [enScreens[key], trScreens[key]]) assert.doesNotMatch(text, internal);
});

test('a deletion still running says it waits and re-checks, and names nobody who must act', async () => {
  const saved = await readSavedDomainDeletionState(response(200, {
    status: 'deletion_pending', stage: 'running', reason: 'deletion_running',
  }));
  assert.deepEqual(saved, { reason: 'deletion_running', detail: '' });
  assert.deepEqual(await readSavedDomainDeletionState(response(200, {
    status: 'deletion_pending', stage: 'running', reason: 'dns_peer_inspection_unknown',
  })), { reason: '', detail: '' });
  const key = domainDeletionReasonKey('deletion_running');
  assert.equal(key, 'domains.deletionRunning');
  assert.match(enScreens[key], /checked again automatically/);
  assert.match(enScreens[key], /Nobody needs to do anything/);
  assert.match(trScreens[key], /kendiliğinden yeniden denetlenir/);
  for (const text of [enScreens[key], trScreens[key]]) {
    assert.doesNotMatch(text, internal);
    assert.doesNotMatch(text, /SSH|must check/);
  }
});

function readySnapshot(overrides = {}) {
  return {
    revision: 4, engine_epoch: 2, active_engine: 'pdns', state: 'ready', topology: 'paired',
    pair_role: 'primary', pair_ready: false,
    dnssec_zone_count: 0, zone_count: 2, pending_zone_count: 1,
    engines: [
      { id: 'pdns', installed: true, running: true, managed: true, status: 'active' },
      { id: 'bind', installed: false, running: false, managed: false, status: 'available' },
    ],
    ...overrides,
  };
}

test('the DNS engine snapshot carries pending publications and drops malformed ones', () => {
  const pending = [
    { zone: 'a.example', reason: 'dns_peer_catching_up', automatic: true, next_check_at: '2026-10-30T10:00:15Z' },
    { zone: 'b.example', reason: 'dns_peer_enrollment_required', automatic: false },
    { zone: 'Bad Zone', automatic: true },
    { zone: 'c.example', reason: '<script>', automatic: true, next_check_at: 'soon' },
    { zone: 'd.example' },
  ];
  const decoded = decodeDNSEngineSnapshot(readySnapshot({ pending_publications: pending }));
  assert.ok(decoded);
  assert.deepEqual(decoded.pending_publications, [
    { zone: 'a.example', reason: 'dns_peer_catching_up', automatic: true, next_check_at: '2026-10-30T10:00:15Z' },
    { zone: 'b.example', reason: 'dns_peer_enrollment_required', automatic: false },
    { zone: 'c.example', automatic: true },
  ]);
  // An older Panel omits the field.
  const older = decodeDNSEngineSnapshot(readySnapshot());
  assert.ok(older);
  assert.equal(older.pending_publications, undefined);
  assert.equal(decodeDNSPendingPublications('x'), undefined);
});

test('the card line says why it waits, who acts and when CelikPanel checks again', () => {
  for (const [locale, catalog] of [['en', dnsEngineEn], ['tr', dnsEngineTr]]) {
    const text = (key, vars) => dnsEngineText(locale, key, vars);
    const caught = pendingPublicationText({ zone: 'a.example', reason: 'dns_peer_catching_up', automatic: true, next_check_at: '2026-10-30T10:00:15Z' }, text, locale);
    assert.ok(caught.startsWith('a.example: '));
    assert.match(caught, locale === 'en' ? /backup DNS server to catch up/ : /yetişmesi bekleniyor/);
    assert.match(caught, locale === 'en' ? /Nobody needs to do anything/ : /Kimsenin bir şey yapması gerekmiyor/);
    assert.match(caught, /\d{2}[:.]\d{2}/);
    const soon = pendingPublicationText({ zone: 'a.example', automatic: true }, text, locale);
    assert.equal(soon, dnsEngineText(locale, 'dnsEngine.pendingPublication.automaticSoon', { zone: 'a.example' }));
    const owner = pendingPublicationText({ zone: 'b.example', reason: 'dns_peer_enrollment_required', automatic: false }, text, locale);
    assert.equal(owner, dnsEngineText(locale, 'dnsEngine.pendingPublication.owner', { zone: 'b.example' }));
    for (const key of ['catchingUp', 'automatic', 'automaticSoon', 'owner', 'held']) {
      const value = catalog[`dnsEngine.pendingPublication.${key}`];
      assert.ok(value);
      assert.doesNotMatch(value.replace(/\{(zone|time)\}/g, ''), internal);
    }
  }
});
