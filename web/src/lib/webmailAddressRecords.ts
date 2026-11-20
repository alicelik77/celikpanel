// "Add this record" where the panel safely can (alpha.135, owner 2026-11-14:
// "beside Copy there should be Add"). The webmail address check names the
// record a missing name needs; where it can be added depends on where the
// name's DNS lives:
//
// - a zone this panel hosts: its own record editor, one POST per record;
// - the paired main DNS server's zone (check.peerZone): one POST asks this
//   server to ask that one over the authorized DNS connection; the answer
//   says per record what became of it (cmd/panel/webmail_address_dns_record.go);
// - anywhere else: the record to copy only (the card says where DNS lives).
//
// Both adds run inside the panel's own confirmation, which turns into the
// locked progress window with real steps, and end as done, a verified
// refusal (who acts, what next), a busy main server (nothing applied), or a
// result that is not known (D-024). A zone record whose answer was lost is
// never sent again by itself. A request to the main server that arrived but
// was not answered in time is asked again ONCE after REASK_AFTER_MS (alpha.139:
// the main server answers "present" or "busy" and never writes twice); after
// that, and after busy, a person decides ("Ask again" opens the confirmation
// once more with a new request identity). After an add, the card waits for
// public DNS with read-only checks at a slow, bounded pace (RECHECK_*).
//
// Kayıt, adın DNS'inin yaşadığı yere eklenir: bu paneldeki bölge ya da eşli ana
// DNS sunucusu (yetkili bağlantıyla). Onay penceresi adımları gösterir; sonuç
// tamam, gerekçeli ret, meşgul ya da bilinmeyen sonuçtur. Zamanında yanıt
// gelmeyen istek bir kez, beklemeden sonra yeniden sorulur; sonra karar kişinindir.
// Ardından genel DNS yalnız okunarak, sınırlı aralıklarla beklenir.
import type { TranslationKey } from '../i18n/en';
import { apiErrorText, readApiError } from './apiError';
import { confirmAction, type OperationEnd, type OperationReport, type OperationStep } from './confirmAction';
import { newRequestId } from './requestIdentity';
import { decodeWebmailDNSRecordAnswer, WEBMAIL_ADDRESS_DNS_RECORD_URL, type WebmailAddressCheck, type WebmailDNSRecordAnswer } from './webmailAddress';

type Translate = (key: TranslationKey, vars?: Record<string, string | number>) => string;

/** The card reads public DNS this often after an add, for at most RECHECK_FOR_MS. */
export const RECHECK_EVERY_MS = 20_000;
/** Five minutes: a resolver keeps a "no such name" answer for up to the zone's 300-second negative TTL. */
export const RECHECK_FOR_MS = 5 * 60_000;
/**
 * alpha.139: the pause before the one automatic re-ask after "no answer in
 * time". The main server waits at most 3 s for its locks and may publish for
 * up to 30 s, while this server stops waiting for its answer after 10 s; 25 s
 * later that publication has ended, so the re-ask reads "present" (or "busy"
 * if it still runs). It never writes the record twice.
 */
export const REASK_AFTER_MS = 25_000;

/** What a finished add leaves for the card: where it went and an optional record that was not added. */
export interface RecordsAdded {
    host: string;
    /** The zone (added here) or the other server (asked). */
    zone: string;
    server: string;
    /** The AAAA record that was not added, with the sentence that says why. */
    optional: { text: string; why: string } | null;
}

const JSON_HEADERS = { 'Content-Type': 'application/json' };
const recordsText = (check: WebmailAddressCheck) => check.records.map((record) => record.text).join('\n');

const REFUSALS = ['outside_zone', 'zone_review', 'taken', 'address', 'invalid', 'not_ready', 'unauthorized', 'unsupported', 'license', 'unreachable'] as const;
type Refusal = (typeof REFUSALS)[number];
const isRefusal = (code: string): code is Refusal => (REFUSALS as readonly string[]).includes(code);
const REFUSAL_KEY: Record<Refusal, TranslationKey> = {
    outside_zone: 'dnsadd.refused.outside_zone',
    zone_review: 'dnsadd.refused.zone_review',
    taken: 'dnsadd.refused.taken',
    address: 'dnsadd.refused.address',
    invalid: 'dnsadd.refused.invalid',
    not_ready: 'dnsadd.refused.not_ready',
    unauthorized: 'dnsadd.refused.unauthorized',
    unsupported: 'dnsadd.refused.unsupported',
    license: 'dnsadd.refused.license',
    unreachable: 'dnsadd.refused.unreachable',
};

interface AddHooks {
    t: Translate;
    /** The add succeeded: the card starts waiting for public DNS (before the window is closed). */
    onAdded: (added: RecordsAdded) => void;
    /** The fix a withdrawn offer names: one read-only check. */
    check: () => void;
    /**
     * alpha.139: the request ended busy or not known; asking again is safe
     * (a new request identity: the main server answers "present" if it was
     * added and never writes twice). The card keeps the offer, and the end's
     * "Ask again" opens the confirmation once more.
     */
    askAgain?: () => void;
    onRetryable?: () => void;
}

/** Case 1: the zone is hosted here; its own record editor takes the records after a confirmation. */
export function confirmAddToZone(check: WebmailAddressCheck, hooks: AddHooks): Promise<boolean> {
    const zone = check.zone;
    if (!zone) return Promise.resolve(false);
    const { t } = hooks;
    return confirmAction({
        title: t('life.addr.zoneDialogTitle', { zone: zone.name }),
        message: `${t('life.addr.zoneDialogBody', { zone: zone.name })}\n\n${recordsText(check)}`,
        confirmLabel: t('life.addr.zoneConfirm'),
        progressTitle: t('life.addr.adding', { zone: zone.name }),
        action: async (report: OperationReport): Promise<OperationEnd> => {
            const since = Date.now();
            const steps = (write: OperationStep['state'], dns: OperationStep['state'], note?: string): OperationStep[] => [
                { id: 'write', label: t('dnsadd.step.writeZone', { zone: zone.name }), state: write, since, note },
                { id: 'dns', label: t('dnsadd.step.dns'), state: dns, note: dns === 'waiting' ? t('dnsadd.step.dnsNote') : undefined },
            ];
            report.steps(steps('running', 'pending'));
            for (const record of check.records) {
                let res: Response | null = null;
                try {
                    res = await fetch(zone.recordsURL, {
                        method: 'POST',
                        headers: JSON_HEADERS,
                        body: JSON.stringify({ name: record.name, type: record.type, content: record.content, ttl: record.ttl, prio: 0 }),
                    });
                } catch {
                    res = null;
                }
                if (res === null) {
                    // The answer was lost: the record may have been added. It is
                    // never sent again by itself; Check reads DNS.
                    report.steps(steps('waiting', 'pending', t('dnsadd.step.noAnswer')));
                    return { state: 'unknown', title: t('dnsadd.unknownTitle'), reason: t('life.addr.addLost', { record: record.text }), fix: { label: t('life.addr.check'), run: hooks.check } };
                }
                if (!res.ok) {
                    const reason = apiErrorText(await readApiError(res), t);
                    report.steps(steps('failed', 'pending'));
                    return { state: 'failed', title: t('dnsadd.notAddedTitle'), reason: t('life.addr.addFailed', { record: record.text, zone: zone.name, reason }) };
                }
            }
            report.steps(steps('done', 'waiting'));
            hooks.onAdded({ host: check.host, zone: zone.name, server: '', optional: null });
            return { state: 'done', title: t('dnsadd.doneTitle'), message: t('life.addr.added', { zone: zone.name }) };
        },
    });
}

/** Case 2: one confirmed request asks the paired main DNS server, over the authorized connection. */
export function confirmAskPeer(check: WebmailAddressCheck, hooks: AddHooks): Promise<boolean> {
    const peer = check.peerZone;
    if (!peer) return Promise.resolve(false);
    const { t } = hooks;
    const vars = { server: peer.server, zone: peer.zone };
    const hasAAAA = check.records.some((record) => record.type === 'AAAA');
    const body = t(peer.zone ? 'dnsadd.peer.body' : 'dnsadd.peer.bodyAnyZone', vars);
    const after = [t('dnsadd.peer.transfer'), ...(hasAAAA ? [t('dnsadd.peer.aaaa', vars)] : [])].join(' ');
    return confirmAction({
        title: t('dnsadd.peer.title', vars),
        message: `${body}\n\n${recordsText(check)}\n\n${after}`,
        kept: [t('dnsadd.peer.keptZone'), t('dnsadd.peer.keptOwner', vars)],
        confirmLabel: t('dnsadd.peer.confirm'),
        progressTitle: t('dnsadd.peer.progress', vars),
        action: async (report: OperationReport): Promise<OperationEnd> => {
            let since = Date.now();
            const steps = (ask: OperationStep['state'], write: OperationStep['state'], dns: OperationStep['state'], note?: string): OperationStep[] => [
                { id: 'ask', label: t('dnsadd.step.ask', vars), state: ask, since, note },
                { id: 'write', label: t('dnsadd.step.write', vars), state: write },
                { id: 'dns', label: t('dnsadd.step.dns'), state: dns, note: dns === 'waiting' ? t('dnsadd.step.dnsNote') : undefined },
            ];
            // Busy or not known: nothing to undo, asking again is safe (alpha.139).
            const retryable = (end: Exclude<OperationEnd, { state: 'done' }>): OperationEnd => {
                hooks.onRetryable?.();
                return hooks.askAgain ? { ...end, fix: { label: t('dnsadd.askAgain'), run: hooks.askAgain } } : end;
            };
            const unknown = (): OperationEnd => {
                report.steps(steps('waiting', 'pending', 'pending', t('dnsadd.step.noAnswer')));
                return retryable({ state: 'unknown', title: t('dnsadd.unknownTitle'), reason: t('dnsadd.peer.unknown', vars) });
            };
            const busy = (): OperationEnd => {
                // The answer came (busy): the first step is done and says it
                // answered, with no clock left running (alpha.139 lab). What
                // the answer means is said once, in the end below (no note).
                const answered = steps('done', 'pending', 'pending');
                answered[0] = { ...answered[0], label: t('dnsadd.step.answered', vars) };
                report.steps(answered);
                return retryable({ state: 'blocked', title: t('dnsadd.busyTitle'), subtitle: t('dnsadd.busySubtitle'), reason: t('dnsadd.peer.busy', vars) });
            };
            // One request with a new identity: the decoded answer, or how it ended.
            const ask = async (): Promise<WebmailDNSRecordAnswer | OperationEnd> => {
                let requestID: string;
                try {
                    requestID = newRequestId();
                } catch {
                    return unknown();
                }
                let res: Response | null = null;
                try {
                    res = await fetch(WEBMAIL_ADDRESS_DNS_RECORD_URL, { method: 'POST', headers: JSON_HEADERS, body: JSON.stringify({ host: check.host, request_id: requestID }) });
                } catch {
                    res = null;
                }
                if (res === null) return unknown();
                if (!res.ok) {
                    const error = await readApiError(res);
                    if (error.code === 'WEBMAIL_DNS_RECORD_NOT_OFFERED') {
                        report.steps(steps('failed', 'pending', 'pending'));
                        return { state: 'blocked', title: t('dnsadd.notAddedTitle'), reason: t('dnsadd.peer.notOffered', vars), fix: { label: t('life.addr.check'), run: hooks.check } };
                    }
                    return unknown();
                }
                let answer: WebmailDNSRecordAnswer | null = null;
                try {
                    answer = decodeWebmailDNSRecordAnswer(await res.json());
                } catch {
                    answer = null;
                }
                return answer ?? unknown();
            };
            report.steps(steps('running', 'pending', 'pending'));
            let answer = await ask();
            let again = false;
            if ('records' in answer && answer.records[0].outcome === 'unknown' && answer.records[0].code === 'no_answer') {
                // The request arrived and its answer did not come in time: the
                // main server may still be publishing. Once, after a visible
                // pause, the same record is asked for again with a new identity.
                since = Date.now();
                report.steps(steps('waiting', 'pending', 'pending', t('dnsadd.step.reask', vars)));
                await new Promise<void>((resolve) => { setTimeout(resolve, REASK_AFTER_MS); });
                since = Date.now();
                report.steps(steps('running', 'pending', 'pending'));
                answer = await ask();
                again = true;
            }
            if (!('records' in answer)) return answer;
            const a = answer.records[0];
            if (a.outcome === 'unknown') return unknown();
            if (a.outcome === 'busy') return busy();
            return peerEnd(answer, check, report, steps, again, hooks);
        },
    });
}

function peerEnd(
    answer: WebmailDNSRecordAnswer,
    check: WebmailAddressCheck,
    report: OperationReport,
    steps: (ask: OperationStep['state'], write: OperationStep['state'], dns: OperationStep['state'], note?: string) => OperationStep[],
    again: boolean,
    { t, onAdded }: AddHooks,
): OperationEnd {
    const vars = { server: answer.server };
    const [a, aaaa] = answer.records;
    if (a.outcome === 'refused' || a.outcome === 'not_sent') {
        report.steps(steps('done', 'failed', 'pending'));
        const code = isRefusal(a.code) ? a.code : 'invalid';
        return { state: 'blocked', title: t('dnsadd.notAddedTitle'), reason: t(REFUSAL_KEY[code], vars) };
    }
    // added, present or saved: the A record is there.
    // After the re-ask, "present" is the first request's record, now confirmed.
    const said: TranslationKey = a.outcome === 'present' ? (again ? 'dnsadd.peer.confirmed' : 'dnsadd.peer.present') : a.outcome === 'saved' ? 'dnsadd.peer.saved' : 'dnsadd.peer.added';
    const lines = [t(said, { ...vars, record: a.text })];
    let optional: RecordsAdded['optional'] = null;
    if (aaaa) {
        if (aaaa.outcome === 'added' || aaaa.outcome === 'present' || aaaa.outcome === 'saved') lines.push(t('dnsadd.peer.alsoAAAA'));
        else if (aaaa.outcome === 'unknown') lines.push(t('dnsadd.aaaa.unknown'));
        else {
            const why = t(aaaa.code === 'address' ? 'dnsadd.aaaa.address' : aaaa.code === 'unreachable' ? 'dnsadd.aaaa.unreachable' : 'dnsadd.aaaa.other', vars);
            optional = { text: aaaa.text, why };
            lines.push(`${why} ${aaaa.text}`);
        }
    }
    // The wait for public DNS is said once, under its step.
    report.steps(steps('done', 'done', 'waiting'));
    onAdded({ host: check.host, zone: answer.zone, server: answer.server, optional });
    return { state: 'done', title: t('dnsadd.doneTitle'), message: lines.join(' ') };
}
