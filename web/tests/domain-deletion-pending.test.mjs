import assert from 'node:assert/strict';
import test from 'node:test';
import { domainDeletionBlocksRetry, domainDeletionDetailKey, domainDeletionQuiet, domainDeletionReasonAutomatic, domainDeletionReasonKey, readDomainDeletionPending, readDomainDeletionOutcome, readDomainDeletionStage, readSavedDomainDeletion, readSavedDomainDeletionStatus, readSavedDomainDeletionState } from '../src/lib/domainDeletionPending.ts';
import { en as enShellHalf } from '../src/i18n/en.ts';
import { enErrorsScreens as enErrorsPart } from '../src/i18n/screens/errors/en.ts';
// The coded refusals only screens show are the screen part "errors" since v0.1.0-alpha.98.
import { enDnsPublicationScreens as enDnsPublicationPart } from '../src/i18n/screens/dnsPublication/en.ts';
const en = { ...enShellHalf, ...enErrorsPart, ...enDnsPublicationPart };
import { tr as trShellHalf } from '../src/i18n/tr.ts';
import { trErrorsScreens as trErrorsPart } from '../src/i18n/screens/errors/tr.ts';
import { trDnsPublicationScreens as trDnsPublicationPart } from '../src/i18n/screens/dnsPublication/tr.ts';
const tr = { ...trShellHalf, ...trErrorsPart, ...trDnsPublicationPart };
import { enScreens } from './locale-catalogue.mjs';
import { trScreens } from './locale-catalogue.mjs';

function response(status, body) {
  return new Response(JSON.stringify(body), {
    status, headers: { 'content-type': 'application/json' },
  });
}

test('202 exact DNS deletion pending returns only a reviewed reason', async () => {
  const reason = await readDomainDeletionPending(response(202, {
    status: 'deletion_pending',
    stage: 'dns_cleanup',
    reason: 'dns_peer_native_unknown',
    message: 'untrusted peer detail',
  }));
  assert.equal(reason, 'dns_peer_native_unknown');
});

test('generic, malformed and unrelated deletion results have no reason', async () => {
  for (const [status, body] of [
    [202, { status: 'deletion_pending', stage: 'dns_cleanup' }],
    [202, { status: 'deletion_pending', stage: 'dns_cleanup', reason: 'private peer output' }],
    [202, { status: 'deletion_pending', stage: 'site_cleanup', reason: 'dns_peer_native_unknown' }],
    [200, { status: 'deleted', reason: 'dns_peer_native_unknown' }],
    [202, null],
  ]) {
    assert.equal(await readDomainDeletionPending(response(status, body)), '');
  }
});

test('deletion UI classifies 202 as pending, only 200/204 as success', async () => {
  for (const status of [200, 204]) {
    const response = status === 204
      ? new Response(null, { status })
      : responseWithStatus(status);
    assert.deepEqual(await readDomainDeletionOutcome(response), { state: 'succeeded', reason: '' });
  }
  assert.deepEqual(await readDomainDeletionOutcome(response(202, {
    status: 'deletion_pending', stage: 'dns_cleanup',
    reason: 'dns_peer_native_unknown',
  })), { state: 'pending', reason: 'dns_peer_native_unknown' });
  for (const status of [201, 206, 409, 500]) {
    assert.deepEqual(await readDomainDeletionOutcome(response(status, {})), { state: 'error', reason: '' });
  }
});

function responseWithStatus(status) {
  return response(status, {});
}

test('every reviewed pending reason has readable EN/TR copy', () => {
  for (const reason of [
    'dns_peer_enrollment_required',
    'dns_peer_enrollment_changed',
    'dns_peer_inspection_unknown',
    'dns_peer_native_unknown',
    'dns_peer_journal_unknown',
    'dns_peer_owner_edit_unknown',
    'dns_peer_catalog_transfer_refused',
    'dns_peer_proof_internal',
    'dns_peer_proof_timeout',
    'bind_rndc_unavailable',
  ]) {
    const key = `err.DNS_PUBLICATION_FAILED.${reason}`;
    assert.match(en[key], /same publication/);
    assert.match(tr[key], /ayn\u0131 yay\u0131n\u0131/);
    assert.doesNotMatch(tr[key], /\?/);
  }
  for (const key of ['domains.deletionPending', 'domains.deletionWaiting', 'domains.retryDeletion', 'domains.checkDeletionStatus']) {
    assert.ok(enScreens[key]);
    assert.ok(trScreens[key]);
    assert.doesNotMatch(trScreens[key], /\?/);
  }
});

test('reload restores only a verified saved deletion marker and reviewed reason', async () => {
  assert.equal(await readSavedDomainDeletionStatus(response(200, {
    status: 'deletion_pending', stage: 'dns_cleanup',
    reason: 'dns_peer_journal_unknown',
  })), 'dns_peer_journal_unknown');
  assert.equal(await readSavedDomainDeletionStatus(response(200, {
    status: 'deletion_pending', stage: 'unknown', reason: 'dns_peer_journal_unknown',
  })), '');
  assert.equal(await readSavedDomainDeletionStatus(response(200, {
    status: 'deletion_pending', stage: 'dns_cleanup', reason: 'private peer output',
  })), '');
  assert.equal(await readSavedDomainDeletionStatus(response(200, {
    status: 'unknown', stage: 'unknown', reason: 'dns_peer_journal_unknown',
  })), '');
  assert.equal(await readSavedDomainDeletionStatus(new Response(null, { status: 204 })), null);
  assert.equal(await readSavedDomainDeletionStatus(response(404, {})), null);
  assert.equal(await readSavedDomainDeletionStatus(response(200, {
    status: 'completed', reason: 'dns_peer_journal_unknown',
  })), null);
});

test('rndc guidance names the reason, the owner, the commands and the retry action', async () => {
  assert.equal(await readDomainDeletionPending(response(202, {
    status: 'deletion_pending', stage: 'dns_cleanup', reason: 'bind_rndc_unavailable',
  })), 'bind_rndc_unavailable');
  const en_ = en['err.DNS_PUBLICATION_FAILED.bind_rndc_unavailable'];
  const tr_ = tr['err.DNS_PUBLICATION_FAILED.bind_rndc_unavailable'];
  for (const text of [en_, tr_]) {
    assert.match(text, /rndc key|rndc anahtar/);
    assert.match(text, /sudo rndc-confgen -a/);
    assert.match(text, /sudo systemctl restart named/);
    assert.match(text, /\/etc\/bind\/rndc\.key/);
    assert.doesNotMatch(text, /secret/i);
  }
  assert.ok(en_.includes(`“${enScreens['domains.retryDeletion']}”`));
  assert.ok(tr_.includes(`“${trScreens['domains.retryDeletion']}”`));
  assert.match(en_, /server owner/);
  assert.match(tr_, /sunucunun sahibi/);
});

test('a held Agent names the hold, the owner, the restart and the retry (2026-10-31)', async () => {
  assert.equal(await readDomainDeletionPending(response(202, {
    status: 'deletion_pending', stage: 'dns_cleanup', reason: 'agent_mutations_held',
  })), 'agent_mutations_held');
  assert.deepEqual(await readSavedDomainDeletionState(response(200, {
    status: 'deletion_pending', stage: 'dns_cleanup', reason: 'agent_mutations_held',
  })), { reason: 'agent_mutations_held', detail: '' });
  const en_ = en['err.DNS_PUBLICATION_FAILED.agent_mutations_held'];
  const tr_ = tr['err.DNS_PUBLICATION_FAILED.agent_mutations_held'];
  for (const text of [en_, tr_]) {
    assert.match(text, /sudo systemctl restart celikpanel-agent/);
    assert.match(text, /journalctl -u celikpanel-agent/);
    assert.doesNotMatch(text, /secret|ledger_ambiguous/i);
  }
  assert.ok(en_.includes(`“${enScreens['domains.retryDeletion']}”`));
  assert.ok(tr_.includes(`“${trScreens['domains.retryDeletion']}”`));
  assert.match(en_, /server owner/);
  assert.match(tr_, /sunucu sahibi/);
});

test('refused local catalog transfer names the secondary owner, the statement, both cases and the retry', async () => {
  assert.equal(await readDomainDeletionPending(response(202, {
    status: 'deletion_pending', stage: 'dns_cleanup', reason: 'dns_peer_catalog_transfer_refused',
  })), 'dns_peer_catalog_transfer_refused');
  const en_ = en['err.DNS_PUBLICATION_FAILED.dns_peer_catalog_transfer_refused'];
  const tr_ = tr['err.DNS_PUBLICATION_FAILED.dns_peer_catalog_transfer_refused'];
  for (const text of [en_, tr_]) {
    assert.match(text, /allow-transfer/);
    assert.match(text, /127\.0\.0\.1/);
    assert.match(text, /::1/);
    assert.match(text, /CelikPanel/);
  }
  assert.match(en_, /backup DNS server's owner/);
  assert.match(tr_, /yedek DNS sunucusunun sahibi/);
  assert.ok(en_.includes(`“${enScreens['domains.retryDeletion']}”`));
  assert.ok(tr_.includes(`“${trScreens['domains.retryDeletion']}”`));
});

test('only a reviewed inspector detail of an incomplete inspection is kept, with EN/TR copy', async () => {
  const details = ['inspector_policy', 'named_unavailable', 'listeners_unverified', 'catalog_unverified',
    'catalog_transfer_failed', 'catalog_malformed', 'observation_expired', 'config_unreviewed'];
  for (const detail of details) {
    assert.deepEqual(await readDomainDeletionOutcome(response(202, {
      status: 'deletion_pending', stage: 'dns_cleanup', reason: 'dns_peer_inspection_unknown', detail,
    })), { state: 'pending', reason: 'dns_peer_inspection_unknown', detail });
    assert.deepEqual(await readSavedDomainDeletionState(response(200, {
      status: 'deletion_pending', stage: 'dns_cleanup', reason: 'dns_peer_inspection_unknown', detail,
    })), { reason: 'dns_peer_inspection_unknown', detail });
    const key = `domains.peerInspectorDetail.${detail}`;
    if (detail === 'listeners_unverified') {
      // 6 Nov 2026: usually an older check tool, replaced by the backup's
      // own update; a waiting state unless someone changed the tool by hand.
      assert.match(enScreens[key], /check tool.*Nobody needs to do anything.*changed by hand/);
      assert.match(trScreens[key], /denetim aracı.*Kimsenin bir şey yapması gerekmiyor.*elle değiştirildiyse/);
      continue;
    }
    assert.match(enScreens[key], /secondary's inspector reported/);
    assert.match(trScreens[key], /denetleyicisi/);
    assert.doesNotMatch(trScreens[key], /\?/);
  }
  for (const [reason, detail] of [
    ['dns_peer_inspection_unknown', 'raw peer stderr'],
    ['dns_peer_inspection_unknown', 'catalog_transfer_refused'],
    ['dns_peer_native_unknown', 'named_unavailable'],
  ]) {
    assert.deepEqual(await readDomainDeletionOutcome(response(202, {
      status: 'deletion_pending', stage: 'dns_cleanup', reason, detail,
    })), { state: 'pending', reason });
    assert.equal((await readSavedDomainDeletionState(response(200, {
      status: 'deletion_pending', stage: 'dns_cleanup', reason, detail,
    }))).detail, '');
  }
});

test('the PowerDNS unreviewed-configuration detail names both recognised configurations', () => {
  const key = 'domains.peerInspectorDetail.config_unreviewed';
  assert.equal(enScreens[key],
    "The secondary's inspector reported that PowerDNS is not running with a configuration it recognises (the panel's own or the documented panel-free one).");
  assert.match(trScreens[key], /PowerDNS/);
  assert.match(trScreens[key], /panelin kendi yapılandırması/);
  assert.match(trScreens[key], /panelsiz yapılandırma/);
});

test('an internal proof failure names no owner change, the owner, the log command and the retry', async () => {
  assert.deepEqual(await readDomainDeletionOutcome(response(202, {
    status: 'deletion_pending', stage: 'dns_cleanup', reason: 'dns_peer_proof_internal',
  })), { state: 'pending', reason: 'dns_peer_proof_internal' });
  assert.equal(await readSavedDomainDeletionStatus(response(200, {
    status: 'deletion_pending', stage: 'dns_cleanup', reason: 'dns_peer_proof_internal',
  })), 'dns_peer_proof_internal');
  const en_ = en['err.DNS_PUBLICATION_FAILED.dns_peer_proof_internal'];
  const tr_ = tr['err.DNS_PUBLICATION_FAILED.dns_peer_proof_internal'];
  for (const text of [en_, tr_]) {
    assert.ok(text.includes('sudo journalctl -u celikpanel-agent | grep peer'));
    assert.match(text, /CelikPanel Agent/);
    assert.doesNotMatch(text, /secret|password|parola/i);
  }
  assert.match(en_, /could not run its own check of the backup DNS server/);
  assert.match(en_, /No change by either server's owner was found/);
  assert.match(en_, /server owner/);
  assert.match(en_, /Retrying does not help until/);
  assert.match(en_, /DNS answers are unaffected/);
  assert.doesNotMatch(en_, /evidence changed|reconcile/);
  assert.match(tr_, /kendi denetimiyle kontrol edemedi/);
  assert.match(tr_, /değişiklik bulunmadı/);
  assert.match(tr_, /sunucunun sahibi/);
  assert.match(tr_, /DNS yanıtları etkilenmez/);
  assert.doesNotMatch(tr_, /kanıt değişti|uzlaştır/);
  assert.ok(en_.includes(`“${enScreens['domains.retryDeletion']}”`));
  assert.ok(tr_.includes(`“${trScreens['domains.retryDeletion']}”`));
});

test('a proof timeout says nothing changed, who retries now and what to check if it repeats', async () => {
  assert.deepEqual(await readDomainDeletionOutcome(response(202, {
    status: 'deletion_pending', stage: 'dns_cleanup', reason: 'dns_peer_proof_timeout',
  })), { state: 'pending', reason: 'dns_peer_proof_timeout' });
  assert.equal(await readSavedDomainDeletionStatus(response(200, {
    status: 'deletion_pending', stage: 'dns_cleanup', reason: 'dns_peer_proof_timeout',
  })), 'dns_peer_proof_timeout');
  const en_ = en['err.DNS_PUBLICATION_FAILED.dns_peer_proof_timeout'];
  const tr_ = tr['err.DNS_PUBLICATION_FAILED.dns_peer_proof_timeout'];
  assert.match(en_, /did not finish within its time/);
  assert.match(en_, /Nothing was changed on either server/);
  assert.match(en_, /DNS answers are unaffected/);
  assert.match(en_, /retry now/);
  assert.match(en_, /SSH port/);
  assert.match(en_, /overloaded/);
  assert.doesNotMatch(en_, /evidence changed|reconcile|Retrying does not help/);
  assert.match(tr_, /sürede bitmedi/);
  assert.match(tr_, /Hiçbir sunucuda bir şey değiştirilmedi/);
  assert.match(tr_, /DNS yanıtları etkilenmez/);
  assert.match(tr_, /SSH bağlantı noktasına/);
  assert.match(tr_, /aşırı yüklü/);
  assert.doesNotMatch(tr_, /uzlaştır/);
  for (const text of [en_, tr_]) {
    assert.doesNotMatch(text, /secret|password|parola/i);
  }
  assert.ok(en_.includes(`“${enScreens['domains.retryDeletion']}”`));
  assert.ok(tr_.includes(`“${trScreens['domains.retryDeletion']}”`));
});

test('a challenge that cannot be reconciled waits for the automatic re-check, then names the Agent log and why a retry is safe', () => {
  const en_ = en['err.DNS_PUBLICATION_FAILED.dns_peer_journal_unknown'];
  const tr_ = tr['err.DNS_PUBLICATION_FAILED.dns_peer_journal_unknown'];
  assert.match(en_, /could not write or match its single-use check challenge/);
  assert.match(tr_, /yazamadı ya da eşleştiremedi/);
  // D-024: while the Panel re-checks by itself it is a waiting state; the
  // administrator's log step comes only after that, for a lasting result.
  assert.match(en_, /Nobody needs to do anything now/);
  assert.match(tr_, /kimsenin bir şey yapması gerekmiyor/);
  for (const text of [en_, tr_]) {
    assert.ok(text.includes('sudo journalctl -u celikpanel-agent | grep peer'));
  }
  assert.ok(en_.indexOf('Nobody needs to do anything now') < en_.indexOf('journalctl'));
  assert.ok(tr_.indexOf('kimsenin bir şey yapması gerekmiyor') < tr_.indexOf('journalctl'));
  assert.match(en_, /retry is safe because each challenge can be used only once/);
  assert.match(tr_, /yalnız bir kez kullanılabildiği/);
  assert.match(tr_, /yeniden denemek güvenlidir/);
  assert.ok(en_.includes(`“${enScreens['domains.retryDeletion']}”`));
  assert.ok(tr_.includes(`“${trScreens['domains.retryDeletion']}”`));
});

test('owner-edit guidance keeps its meaning and names the serial comparison', () => {
  const en_ = en['err.DNS_PUBLICATION_FAILED.dns_peer_owner_edit_unknown'];
  const tr_ = tr['err.DNS_PUBLICATION_FAILED.dns_peer_owner_edit_unknown'];
  assert.match(en_, /evidence changed during verification/);
  assert.match(en_, /compare the catalog zone and zone serials on both servers/);
  assert.match(tr_, /seri numaralarını karşılaştırıp/);
  assert.ok(en_.includes(`“${enScreens['domains.retryDeletion']}”`));
  assert.ok(tr_.includes(`“${trScreens['domains.retryDeletion']}”`));
});

test('owner-edit pending names the check that differed through its own sentence', async () => {
  const checks = ['operation_attempt', 'engine_state', 'active_engine', 'native_binding', 'deletion_receipt',
    'producer_catalog', 'catalog_probe', 'authority', 'transfer_observed', 'zone_answered'];
  const reason = 'dns_peer_owner_edit_unknown';
  for (const detail of checks) {
    assert.deepEqual(await readDomainDeletionOutcome(response(202, {
      status: 'deletion_pending', stage: 'dns_cleanup', reason, detail,
    })), { state: 'pending', reason, detail });
    assert.deepEqual(await readSavedDomainDeletionState(response(200, {
      status: 'deletion_pending', stage: 'dns_cleanup', reason, detail,
    })), { reason, detail });
    const key = domainDeletionDetailKey(reason, detail);
    assert.equal(key, `domains.peerOwnerEditDetail.${detail}`);
    assert.match(enScreens[key], /^What differed: /);
    assert.match(trScreens[key], /^Farklı olan: /);
    assert.doesNotMatch(trScreens[key], /\?/);
    assert.doesNotMatch(enScreens[key] + trScreens[key], /secret|password|parola|\d{6,}/i);
  }
  // A token is accepted only under the reason it refines.
  for (const [r, detail] of [
    ['dns_peer_owner_edit_unknown', 'named_unavailable'],
    ['dns_peer_owner_edit_unknown', 'serial 1790745288'],
    ['dns_peer_inspection_unknown', 'producer_catalog'],
    ['dns_peer_native_unknown', 'producer_catalog'],
  ]) {
    assert.deepEqual(await readDomainDeletionOutcome(response(202, {
      status: 'deletion_pending', stage: 'dns_cleanup', reason: r, detail,
    })), { state: 'pending', reason: r });
    assert.equal(domainDeletionDetailKey(r, detail), null);
  }
  assert.equal(domainDeletionDetailKey('dns_peer_inspection_unknown', 'named_unavailable'),
    'domains.peerInspectorDetail.named_unavailable');
  // The reason text and its retry of the same publication are unchanged.
  assert.match(en['err.DNS_PUBLICATION_FAILED.dns_peer_owner_edit_unknown'], /same publication/);
  assert.match(tr['err.DNS_PUBLICATION_FAILED.dns_peer_owner_edit_unknown'], /aynı yayını/);
});

// alpha.114 lab run 2, E-new-4: a joined server's deletion and the typed
// states that replaced "unknown" each carry their own reason and sentence.
test('a joined server deletion and the former unknown states read as typed reasons with their own words', async () => {
  const own = {
    remote_dns_removal_confirming: ['dns_cleanup', true],
    remote_dns_main_unreachable: ['dns_cleanup', true],
    remote_dns_removal_refused: ['dns_cleanup', false],
    remote_dns_removed_finishing: ['dns_removed', true],
    deletion_incomplete: ['incomplete', false],
    deletion_state_changing: ['dns_cleanup', true],
    deletion_marker_inconsistent: ['marker_check', false],
    deletion_already_running: ['dns_cleanup', true],
  };
  for (const [reason, [stage, automatic]] of Object.entries(own)) {
    assert.deepEqual(await readSavedDomainDeletionState(response(200, {
      status: 'deletion_pending', stage, reason, message: 'server text',
    })), { reason, detail: '' }, reason);
    assert.equal(domainDeletionReasonAutomatic(reason), automatic, reason);
    const key = domainDeletionReasonKey(reason);
    assert.equal(key, `domains.deletionReason.${reason}`);
    assert.ok(enScreens[key] && trScreens[key], key);
    assert.notEqual(enScreens[key], trScreens[key]);
  }
  // The joined server's 202 names the main server's answer.
  assert.deepEqual(await readDomainDeletionOutcome(response(202, {
    status: 'deletion_pending', stage: 'dns_cleanup', reason: 'remote_dns_removal_confirming',
  })), { state: 'pending', reason: 'remote_dns_removal_confirming' });
  // A repeated press while the domain is already being deleted (6 Nov 2026).
  assert.deepEqual(await readDomainDeletionOutcome(response(202, {
    status: 'deletion_pending', stage: 'dns_cleanup', reason: 'deletion_already_running',
  })), { state: 'pending', reason: 'deletion_already_running' });
  assert.match(enScreens['domains.deletionReason.deletion_already_running'], /^This domain is already being deleted\. Nobody needs to do anything/);
  assert.match(trScreens['domains.deletionReason.deletion_already_running'], /zaten siliniyor.*Kimsenin bir şey yapması gerekmiyor/);
  // Waiting states say nobody acts; the others name who acts.
  // Owner report 6 Nov 2026: one plain sentence, nobody acts.
  assert.match(enScreens['domains.deletionReason.remote_dns_removal_confirming'], /being deleted.*Nothing to do; this page updates by itself\.$/);
  assert.match(trScreens['domains.deletionReason.remote_dns_removal_confirming'], /siliniyor.*Yapmanız gereken bir şey yok/);
  assert.match(enScreens['domains.deletionReason.remote_dns_removal_refused'], /The server administrator/);
  assert.match(enScreens['domains.deletionReason.deletion_incomplete'], /Delete the domain again/);
  assert.match(trScreens['domains.deletionReason.deletion_incomplete'], /yeniden silin/);
  // An unreviewed reason is still not shown.
  assert.deepEqual(await readSavedDomainDeletionState(response(200, {
    status: 'deletion_pending', stage: 'incomplete', reason: 'raw_reason',
  })), { reason: '', detail: '' });
});

// Lab run 6: a deletion that finished between the list's answer and the
// status read answers 404. That is "gone", never "unknown".
test('a deleted domain status read is gone, a failed read stays unknown', async () => {
  const saved = globalThis.fetch;
  try {
    for (const [status, state] of [[404, 'gone'], [204, 'known'], [500, 'unknown'], [403, 'unknown']]) {
      globalThis.fetch = async () => new Response(null, { status });
      assert.equal((await readSavedDomainDeletion(7)).state, state, String(status));
    }
  } finally {
    globalThis.fetch = saved;
  }
});

// Owner report, 6 Nov 2026: delete pressed again and again while the backup DNS
// server confirmed the removal. While CelikPanel carries a deletion on by
// itself the domain is "being deleted" and no second delete is sent; the one
// reason whose own sentence asks for "delete again" keeps its button. The
// reasons whose whole sentence is "nothing to do" are said in one line.
test('a deletion CelikPanel carries on by itself blocks a second delete; the waiting ones are one line', () => {
  for (const reason of ['deletion_running', 'dns_peer_catching_up', 'remote_dns_removal_confirming', 'remote_dns_main_unreachable', 'deletion_state_changing']) {
    assert.equal(domainDeletionBlocksRetry(reason), true, reason);
  }
  for (const reason of ['remote_dns_removed_finishing', 'deletion_incomplete', 'remote_dns_removal_refused', 'deletion_marker_inconsistent', 'dns_removal_confirmed', '']) {
    assert.equal(domainDeletionBlocksRetry(reason), false, reason);
  }
  assert.deepEqual(['deletion_running', 'remote_dns_removal_confirming', 'deletion_state_changing'].filter(domainDeletionQuiet).length, 3);
  // One that names who acts after a while is never shortened.
  assert.equal(domainDeletionQuiet('remote_dns_main_unreachable'), false);
  assert.equal(domainDeletionQuiet('remote_dns_removed_finishing'), false);
  assert.match(enScreens['domains.beingDeletedLine'], /^\{name\} is being deleted\. Nothing to do; this page updates by itself\.$/);
  assert.match(trScreens['domains.beingDeletedLine'], /^\{name\} siliniyor\./);
});

// alpha.126 (owner's order): while this server's request runs, the status
// read names the step it is at now; the window shows that step. An unknown
// or missing step falls back to "running"; other answers keep their stage.
test('a running deletion names its live step; anything unreviewed is ignored', async () => {
  for (const step of ['mail_tls_cleanup', 'mail_runtime_cleanup', 'site_cleanup', 'dns_cleanup', 'certificate_cleanup', 'ledger_delete']) {
    const answer = response(200, { status: 'deletion_pending', stage: 'running', reason: 'deletion_running', step });
    assert.equal(await readDomainDeletionStage(answer), step);
    // The step never changes what the saved state means.
    assert.deepEqual(await readSavedDomainDeletionState(answer), { reason: 'deletion_running', detail: '' });
  }
  assert.equal(await readDomainDeletionStage(response(200, { status: 'deletion_pending', stage: 'running', reason: 'deletion_running' })), 'running');
  assert.equal(await readDomainDeletionStage(response(200, { status: 'deletion_pending', stage: 'running', reason: 'deletion_running', step: 'rm -rf' })), 'running');
  // `step` counts only while the request runs.
  assert.equal(await readDomainDeletionStage(response(200, { status: 'failed', stage: 'site_cleanup', step: 'ledger_delete' })), 'site_cleanup');
});
