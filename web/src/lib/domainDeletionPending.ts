// A 202 deletion response is a durable pending operation, never a success receipt.
const reviewedDNSPeerReasons = new Set([
    'dns_peer_enrollment_required',
    'dns_peer_enrollment_changed',
    'dns_peer_inspection_unknown',
    'dns_peer_native_unknown',
    'dns_peer_journal_unknown',
    // alpha.133: this server's check record is in a state the Agent cannot
    // sort out by itself; the server administrator acts.
    'dns_peer_journal_unresolved',
    'dns_peer_owner_edit_unknown',
    // The secondary's named refused the inspector's local catalog transfer.
    'dns_peer_catalog_transfer_refused',
    // The Agent could not run its own proof of the secondary; no owner
    // change was found. A retry waits for a fixed Agent.
    'dns_peer_proof_internal',
    // A step of the proof of the secondary ran out of its time before the
    // peer's answer was accepted. The zone is already removed on this server;
    // the Panel re-checks by itself (alpha.133: the detail names the side).
    'dns_peer_proof_timeout',
    // The backup DNS server has not caught up with this server yet; the
    // Panel re-checks the same publication by itself.
    'dns_peer_catching_up',
    // Local, not peer: this server's named could not be asked about zone
    // state (no usable rndc key). Same retry action, same pending deletion.
    'bind_rndc_unavailable',
    // The Agent holds every change after an operation it could not verify;
    // the owner restarts the Agent, then retries the same deletion.
    'agent_mutations_held',
]);

// What the secondary's inspector reported for an incomplete inspection. Only
// these tokens are shown, each through its own translated sentence; the server
// never forwards inspector or SSH output.
const reviewedInspectorDetails = new Set([
    'inspector_policy',
    'named_unavailable',
    'listeners_unverified',
    'catalog_unverified',
    'catalog_transfer_failed',
    'catalog_malformed',
    'observation_expired',
    // PowerDNS only: its configuration matches neither reviewed shape.
    'config_unreviewed',
]);

// Which check of this server's proof found different evidence when the
// deletion stays pending as dns_peer_owner_edit_unknown. Only these tokens are
// shown, each through its own translated sentence; no observed value is sent.
const reviewedOwnerEditChecks = new Set([
    'operation_attempt',
    'engine_state',
    'active_engine',
    'native_binding',
    'deletion_receipt',
    'producer_catalog',
    'catalog_probe',
    'authority',
    'transfer_observed',
    'zone_answered',
]);

// Which side of the proof ran out of time (alpha.133): this server's own
// checks, or the backup DNS server's answer.
const reviewedProofTimeoutSides = new Set(['local_check', 'peer_exchange']);

// The reviewed detail tokens of each reason that carries one.
const reviewedDetailsByReason: Record<string, Set<string>> = {
    dns_peer_inspection_unknown: reviewedInspectorDetails,
    dns_peer_owner_edit_unknown: reviewedOwnerEditChecks,
    dns_peer_proof_timeout: reviewedProofTimeoutSides,
    // alpha.133 round 5 review: the backup presented another SSH host key
    // (its own guide, "peerHostKeyChanged").
    dns_peer_enrollment_changed: new Set(['host_key']),
};

// The translation key of a reviewed detail sentence, or null.
export function domainDeletionDetailKey(reason: string, detail: string): string | null {
    if (!detail) return null;
    if (reason === 'dns_peer_inspection_unknown' && reviewedInspectorDetails.has(detail)) {
        return `domains.peerInspectorDetail.${detail}`;
    }
    if (reason === 'dns_peer_owner_edit_unknown' && reviewedOwnerEditChecks.has(detail)) {
        return `domains.peerOwnerEditDetail.${detail}`;
    }
    if (reason === 'dns_peer_proof_timeout' && reviewedProofTimeoutSides.has(detail)) {
        return `err.DNS_PUBLICATION_FAILED.dns_peer_proof_timeout.${detail}`;
    }
    return null;
}

// The reasons of this server's DNS step that the deletion guide says as why,
// who acts, next step and how it continues (alpha.133, screen part
// "deletionGuide"), by their key segment.
// Neden, kim yapar, sonraki adım ve nasıl sürer biçiminde söylenen nedenler.
// alpha.133 round 5: every DNS proof reason has its guide (lab w9: a backup
// that did not answer showed one jargon row with no who, next step or how it
// continues).
const guideReasons: Record<string, string> = {
    dns_peer_journal_unknown: 'journalUnknown',
    dns_peer_journal_unresolved: 'journalUnresolved',
    dns_peer_proof_timeout: 'proofTimeout',
    dns_peer_catching_up: 'catchingUp',
    dns_peer_enrollment_required: 'peerSetup',
    dns_peer_enrollment_changed: 'peerAccessChanged',
    dns_peer_inspection_unknown: 'peerRefused',
    dns_peer_native_unknown: 'peerNotRemoved',
    dns_peer_owner_edit_unknown: 'evidenceChanged',
    dns_peer_catalog_transfer_refused: 'transferRefused',
    dns_peer_proof_internal: 'internal',
};

/**
 * The deletion guide's key segment of a reason, or null when it has none.
 * The backup's exchange that got no answer (dns_peer_proof_timeout with the
 * detail peer_exchange) has its own: the backup did not answer.
 */
export function domainDeletionGuideReason(reason: string, detail = ''): string | null {
    if (reason === 'dns_peer_proof_timeout' && detail === 'peer_exchange') return 'peerNoAnswer';
    // The backup still transfers or answers for the deleted zone: the
    // backup's answer did not show the removal (alpha.133 round 5 review).
    if (reason === 'dns_peer_owner_edit_unknown' && backupSideOwnerEditChecks.has(detail)) return 'peerNotRemoved';
    if (reason === 'dns_peer_enrollment_changed' && detail === 'host_key') return 'peerHostKeyChanged';
    return Object.prototype.hasOwnProperty.call(guideReasons, reason) ? guideReasons[reason] : null;
}

// Owner-edit checks that only look at the backup DNS server (they run after
// the recovery's own pre-checks of this server); the others mean this
// server's DNS may be changed by hand.
const backupSideOwnerEditChecks = new Set(['transfer_observed', 'zone_answered']);

/**
 * Whether CelikPanel stops checking a deletion's DNS step by itself
 * (cmd/panel dnsPeerPendingRetriesAutomatically, one rule in
 * internal/transport DNSPeerPendingStopsAutomaticRechecks): a check record it
 * cannot sort out, or an owner edit of this server's DNS, which CelikPanel
 * never acts over on its own.
 */
export function domainDeletionStopsRechecks(reason: string, detail = ''): boolean {
    return reason === 'dns_peer_journal_unresolved' ||
        (reason === 'dns_peer_owner_edit_unknown' && !backupSideOwnerEditChecks.has(detail));
}

/**
 * Guides whose cause a person has to fix on one of the two servers. CelikPanel
 * still checks again by itself and the deletion finishes once it is fixed,
 * but "you do not need to do anything" would be untrue.
 */
const personGuides = new Set(['peerSetup', 'peerAccessChanged', 'peerHostKeyChanged', 'peerRefused', 'peerNotRemoved', 'evidenceChanged', 'transferRefused', 'internal']);

export function domainDeletionGuideNeedsPerson(guide: string): boolean {
    return personGuides.has(guide);
}

// What happens next to a deletion's DNS step, as the server says it
// (follow_up, alpha.133): CelikPanel re-checks it and that can finish it;
// the re-checks continue but the administrators were notified; the automatic
// checks ended; or a person must act first.
// Sonra ne olacağı: kendiliğinden, takıldı (yönetici bildirildi), durdu, kişi.
export type DomainDeletionFollowUp = 'automatic' | 'stuck' | 'stopped' | 'person';
const followUps = new Set<string>(['automatic', 'stuck', 'stopped', 'person']);

function reviewedFollowUp(pending: Record<string, unknown>): { followUp?: DomainDeletionFollowUp; nextCheckAt?: string } {
    const out: { followUp?: DomainDeletionFollowUp; nextCheckAt?: string } = {};
    if (typeof pending.follow_up === 'string' && followUps.has(pending.follow_up)) {
        out.followUp = pending.follow_up as DomainDeletionFollowUp;
    }
    if (typeof pending.next_check_at === 'string' && !Number.isNaN(Date.parse(pending.next_check_at))) {
        out.nextCheckAt = pending.next_check_at;
    }
    return out;
}

// A verified failure of a non-DNS deletion stage on this server, by stage.
// The deletion stays pending and the same retry action repeats it.
const reviewedStageFailures: Record<string, string> = {
    mail_runtime_cleanup: 'mail_runtime_cleanup_failed',
    // alpha.134: the sender-limit lists, the website (its error line names
    // the part still there) and the domain's databases.
    mail_lists_cleanup: 'mail_lists_cleanup_failed',
    site_cleanup: 'site_cleanup_failed',
    database_cleanup: 'database_cleanup_failed',
};

function reviewedStageFailure(pending: Record<string, unknown>): string {
    return typeof pending.stage === 'string' && Object.prototype.hasOwnProperty.call(reviewedStageFailures, pending.stage) &&
        pending.reason === reviewedStageFailures[pending.stage]
        ? pending.reason
        : '';
}

// A deletion whose DNS removal is complete (for example after CelikPanel's own
// re-check once the backup caught up) while its later stages have not run.
const dnsRemovedStage = 'dns_removed';
const dnsRemovedReason = 'dns_removal_confirmed';

// A deletion request still running on the server: what it saved so far is a
// moment of it, and a paired DNS step re-checks the backup by itself. Who
// must act is said only once the request has ended.
const runningStage = 'running';
const runningReason = 'deletion_running';

// The deletion's own reviewed reasons (alpha.114 lab run 2): a joined
// server's removal on the main DNS server, and the typed states that replaced
// an "unknown" answer. Each has its own sentence saying who acts.
const ownDeletionReasons = new Set([
    'remote_dns_removal_confirming',
    'remote_dns_main_unreachable',
    'remote_dns_removal_refused',
    'remote_dns_removed_finishing',
    'deletion_incomplete',
    'deletion_state_changing',
    'deletion_marker_inconsistent',
    // A delete press while this domain is already being deleted: nothing new
    // was started (6 Nov 2026).
    'deletion_already_running',
    // alpha.129: this server's removal waits for another domain's DNS change
    // (published by itself afterwards), or it failed (the owner's retry).
    'dns_removal_waiting',
    'dns_removal_failed',
    // The DNS removal is confirmed but finishing the deletion failed three
    // times in a row: the error line and the owner's retry are shown.
    'deletion_finish_failed',
]);

// Reasons whose next step CelikPanel takes by itself: the page reads again
// on its own while one is shown (no manual refresh).
const automaticDeletionReasons = new Set([
    'deletion_running',
    'dns_peer_catching_up',
    'remote_dns_removal_confirming',
    'remote_dns_main_unreachable',
    'remote_dns_removed_finishing',
    'deletion_state_changing',
    'deletion_already_running',
    'dns_removal_waiting',
]);

export function domainDeletionReasonAutomatic(reason: string): boolean {
    return automaticDeletionReasons.has(reason);
}

// While one of these is shown the domain is "being deleted": CelikPanel is
// carrying on by itself, so the page sends no second delete for it (owner
// report 6 Nov 2026: pressing delete again and again). The one automatic reason
// whose own sentence asks for "delete again" after a while
// (remote_dns_removed_finishing) keeps its delete button.
// Bunlardan biri gösterilirken alan adı "siliniyor"dur; sayfa ikinci bir silme
// göndermez.
export function domainDeletionBlocksRetry(reason: string): boolean {
    return automaticDeletionReasons.has(reason) && reason !== 'remote_dns_removed_finishing';
}

// Reasons whose whole sentence is "nobody needs to do anything": the page
// says it in one line ("<name> is being deleted. Nothing to do; this page
// updates by itself."). When the automatic checks end without confirmation the
// saved reason changes to one that names who must act, and that is shown.
// Cümlesi "kimsenin bir şey yapması gerekmez" olan nedenler tek satırla söylenir.
const quietDeletionReasons = new Set(['deletion_running', 'remote_dns_removal_confirming', 'deletion_state_changing', 'deletion_already_running']);

export function domainDeletionQuiet(reason: string): boolean {
    return quietDeletionReasons.has(reason);
}

function ownReason(pending: Record<string, unknown>): string {
    return typeof pending.reason === 'string' && ownDeletionReasons.has(pending.reason) ? pending.reason : '';
}

// The translation key for a reviewed reason, or null for the generic text.
export function domainDeletionReasonKey(reason: string): string | null {
    if (!reason) return null;
    if (ownDeletionReasons.has(reason)) return `domains.deletionReason.${reason}`;
    if (reason === dnsRemovedReason) return 'domains.deletionDNSRemoved';
    if (reason === runningReason) return 'domains.deletionRunning';
    if (Object.values(reviewedStageFailures).includes(reason)) return `err.DOMAIN_DELETION_FAILED.${reason}`;
    return `err.DNS_PUBLICATION_FAILED.${reason}`;
}

function reviewedDetail(pending: Record<string, unknown>, reason: string): string {
    const allowed = Object.prototype.hasOwnProperty.call(reviewedDetailsByReason, reason)
        ? reviewedDetailsByReason[reason]
        : undefined;
    return allowed !== undefined && typeof pending.detail === 'string' && allowed.has(pending.detail)
        ? pending.detail
        : '';
}

async function readPendingBody(response: Response): Promise<{ reason: string; detail: string; followUp?: DomainDeletionFollowUp; nextCheckAt?: string }> {
    if (response.status !== 202) return { reason: '', detail: '' };
    try {
        const body: unknown = await response.json();
        if (!body || typeof body !== 'object' || Array.isArray(body)) return { reason: '', detail: '' };
        const pending = body as Record<string, unknown>;
        if (pending.status === 'deletion_pending' && pending.stage !== 'dns_cleanup') {
            return { reason: reviewedStageFailure(pending), detail: '' };
        }
        if (pending.status !== 'deletion_pending' || pending.stage !== 'dns_cleanup') return { reason: '', detail: '' };
        if (ownReason(pending)) return { reason: ownReason(pending), detail: '' };
        const reason = typeof pending.reason === 'string' && reviewedDNSPeerReasons.has(pending.reason)
            ? pending.reason
            : '';
        return { reason, detail: reviewedDetail(pending, reason), ...(reason ? reviewedFollowUp(pending) : {}) };
    } catch {
        return { reason: '', detail: '' };
    }
}

export async function readDomainDeletionPending(response: Response): Promise<string> {
    return (await readPendingBody(response)).reason;
}


export async function readDomainDeletionOutcome(response: Response): Promise<{
    state: 'pending' | 'succeeded' | 'error';
    reason: string;
    detail?: string;
    followUp?: DomainDeletionFollowUp;
    nextCheckAt?: string;
}> {
    if (response.status === 202) {
        const { reason, detail, ...next } = await readPendingBody(response);
        return { state: 'pending', reason, ...(detail ? { detail } : {}), ...next };
    }
    if (response.status === 200 || response.status === 204) {
        return { state: 'succeeded', reason: '' };
    }
    return { state: 'error', reason: '' };
}


// What a completed deletion left of the domain's DKIM key (30 Oct 2026). The
// key CelikPanel made is removed; a key or signing-table line it did not make
// is kept and named, and a removal that failed is said. null: nothing to say.
// Silmenin DKIM anahtarından bıraktığı: CelikPanel'in yapmadığı korunur.
export async function readDomainDeletionDKIM(
    response: Response,
): Promise<{ keptKey: string; ownerLines: number; failed: boolean } | null> {
    try {
        const body: unknown = await response.json();
        if (!body || typeof body !== 'object' || Array.isArray(body)) return null;
        const dkim = (body as Record<string, unknown>).dkim;
        if (!dkim || typeof dkim !== 'object' || Array.isArray(dkim)) return null;
        const value = dkim as Record<string, unknown>;
        const keptKey = typeof value.kept_key === 'string' && value.kept_key.startsWith('/') ? value.kept_key : '';
        const ownerLines = Array.isArray(value.owner_table_lines) ? value.owner_table_lines.length : 0;
        const failed = typeof value.error === 'string' && value.error !== '';
        return keptKey || ownerLines || failed ? { keptKey, ownerLines, failed } : null;
    } catch {
        return null;
    }
}

// null means this GET did not verify an active deletion marker. An empty reason
// means the marker exists but the exact DNS/Agent reason remains unknown.
export async function readSavedDomainDeletionState(
    response: Response,
): Promise<SavedDomainDeletion | null> {
    if (response.status !== 200) return null;
    try {
        const body: unknown = await response.json();
        if (!body || typeof body !== 'object' || Array.isArray(body)) return null;
        const saved = savedDomainDeletionState(body as Record<string, unknown>);
        // alpha.134: the choice for the domain's databases this deletion was
        // accepted with, so its retry says it and repeats it.
        const databases = (body as Record<string, unknown>).databases;
        return saved && (databases === 'keep' || databases === 'delete') ? { ...saved, databases } : saved;
    } catch {
        return null;
    }
}

function savedDomainDeletionState(pending: Record<string, unknown>): SavedDomainDeletion | null {
    if (pending.status === 'unknown' && pending.stage === 'unknown') return { reason: '', detail: '' };
    // A recorded, verified stage failure: the marker exists and the same
    // deletion can be retried; an unreviewed failure shows the generic text.
    if (pending.status === 'failed') return { reason: reviewedStageFailure(pending), detail: '' };
    if (pending.status !== 'deletion_pending') return null;
    if (pending.stage === dnsRemovedStage && pending.reason === dnsRemovedReason) {
        return { reason: dnsRemovedReason, detail: '' };
    }
    if (pending.stage === runningStage && pending.reason === runningReason) {
        return { reason: runningReason, detail: '' };
    }
    if (ownReason(pending)) return { reason: ownReason(pending), detail: '' };
    if (pending.stage !== 'dns_cleanup') return { reason: '', detail: '' };
    const reason = typeof pending.reason === 'string' && reviewedDNSPeerReasons.has(pending.reason)
        ? pending.reason
        : '';
    return { reason, detail: reviewedDetail(pending, reason), ...(reason ? reviewedFollowUp(pending) : {}) };
}

/** A saved deletion marker's reviewed reason, its detail and what happens next. */
export interface SavedDomainDeletion {
    reason: string;
    detail: string;
    followUp?: DomainDeletionFollowUp;
    nextCheckAt?: string;
    /** alpha.134: the databases choice the deletion was accepted with (its retry repeats it). */
    databases?: 'keep' | 'delete';
}

// What one read of a domain's saved deletion marker established. `known` with
// `saved: null` is the server's own "no marker" (204), or a marker this screen
// has no reviewed words for. `unknown` is a read that failed, was refused or
// dropped: it is never "no deletion is waiting".
// Bir alan adının kayıtlı silme işaretinin tek okumasının saptadığı. `unknown`
// başarısız, reddedilmiş ya da kopmuş okumadır; "bekleyen silme yok" değildir.
// `gone` is the server's 404: the domain no longer exists, so its deletion
// finished between the list's answer and this read (lab run 6).
// `gone`, sunucunun 404'üdür: alan adı artık yok, silme bitmiştir.
export type SavedDomainDeletionRead =
    | { state: 'known'; saved: SavedDomainDeletion | null }
    | { state: 'gone' }
    | { state: 'unknown' };

// readSavedDomainDeletion only reads; it never retries or starts a deletion.
// readSavedDomainDeletion yalnız okur; silmeyi yeniden denemez ya da başlatmaz.
export async function readSavedDomainDeletion(domainId: number): Promise<SavedDomainDeletionRead> {
    const { stage: _stage, ...read } = await readSavedDomainDeletionWithStage(domainId);
    return read as SavedDomainDeletionRead;
}

// The stages a deletion reports (the step it is at, or where it stopped),
// as the server names them. Only these are shown; anything else is ''.
// Bir silmenin bildirdiği aşamalar; yalnız bunlar gösterilir.
const reviewedStages = new Set([
    'running', 'mail_tls_cleanup', 'mail_runtime_cleanup', 'mail_lists_cleanup', 'site_cleanup', 'database_cleanup', 'dns_cleanup',
    'certificate_cleanup', 'ledger_delete', 'dns_removed', 'incomplete', 'marker_check',
]);

/**
 * The reviewed stage of a deletion answer (202 or a status read), or ''.
 * While this server's request runs (stage `running`) a status read also
 * names the step it is at now (`step`, alpha.126): that step is returned, so
 * the progress window shows mail settings, website, DNS, certificates and
 * finishing as they happen.
 */
export async function readDomainDeletionStage(response: Response): Promise<string> {
    try {
        const body: unknown = await response.clone().json();
        const fields = body && typeof body === 'object' && !Array.isArray(body) ? (body as Record<string, unknown>) : {};
        const stage = fields.stage;
        if (stage === 'running' && typeof fields.step === 'string' && fields.step !== 'running' && reviewedStages.has(fields.step)) {
            return fields.step;
        }
        return typeof stage === 'string' && reviewedStages.has(stage) ? stage : '';
    } catch {
        return '';
    }
}

/** readSavedDomainDeletion with the stage the deletion is at (progress window). Reads only. */
export async function readSavedDomainDeletionWithStage(domainId: number): Promise<SavedDomainDeletionRead & { stage: string }> {
    try {
        const response = await fetch(`/api/v1/domains/${domainId}/deletion-status`);
        if (response.status === 204) return { state: 'known', saved: null, stage: '' };
        if (response.status === 404) return { state: 'gone', stage: '' };
        if (response.status !== 200) return { state: 'unknown', stage: '' };
        const stage = await readDomainDeletionStage(response);
        return { state: 'known', saved: await readSavedDomainDeletionState(response), stage };
    } catch {
        return { state: 'unknown', stage: '' };
    }
}

export async function readSavedDomainDeletionStatus(response: Response): Promise<string | null> {
    const state = await readSavedDomainDeletionState(response);
    return state === null ? null : state.reason;
}
