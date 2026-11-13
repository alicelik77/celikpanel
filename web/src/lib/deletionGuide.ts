import type { TranslationKey } from '../i18n/en';
import { formatTime } from './dateFormat';
import { domainDeletionBlocksRetry, domainDeletionGuideReason, type DomainDeletionFollowUp } from './domainDeletionPending';

// The words of a domain deletion waiting for its DNS step (alpha.133, D-024):
// why in one sentence, who acts, the next step only when someone has one,
// and how it continues. One source for the banner on Domains, the deletion
// window and the bulk-delete step notes (screen part "deletionGuide").
// "You do not need to do anything" is said only while CelikPanel re-checks
// by itself and that can finish the deletion (follow_up "automatic"). A
// text names a control only where the reader has it: the administrator's
// commands are in the banner's closed Details (never in the window, which
// points there), and only an administrator is sent to More → Delete….
//
// DNS adımını bekleyen silmenin sözcükleri: neden, kim yapar, sonraki adım,
// nasıl sürer; tek kaynak.

/** The administrator's log command for this deletion's DNS step. */
export const DELETION_LOG_COMMAND = 'sudo journalctl -u celikpanel-agent | grep peer';
/** The listing of the Agent's private folder (names and sizes only). */
export const DELETION_RECORD_LIST_COMMAND = 'sudo ls -la /var/lib/celikpanel-agent-private';

/** What happens next when the server did not say: a reason that needs a person, or CelikPanel's own re-check. */
export function deletionGuideFollowUp(reason: string, followUp?: DomainDeletionFollowUp): DomainDeletionFollowUp {
    if (followUp) return followUp;
    return reason === 'dns_peer_journal_unresolved' ? 'person' : 'automatic';
}

export interface DeletionGuideInput {
    reason: string;
    detail?: string;
    followUp?: DomainDeletionFollowUp;
    nextCheckAt?: string;
}

/** Where the words are shown: the banner has the closed Details; the window and the bulk notes do not. */
export type DeletionGuidePlace = 'banner' | 'window';

export interface DeletionGuideWords {
    tone: 'waiting' | 'blocked';
    automatic: boolean;
    why: string;
    who: string;
    next?: string;
    then: string;
}

type Translate = (key: TranslationKey, vars?: Record<string, string | number>) => string;

/** The guide's sentences for one reason, or null when the reason has no guide. */
export function deletionGuideWords(
    input: DeletionGuideInput,
    admin: boolean,
    t: Translate,
    locale: string,
    place: DeletionGuidePlace = 'banner',
    now: number = Date.now(),
): DeletionGuideWords | null {
    const guide = domainDeletionGuideReason(input.reason);
    if (!guide) return null;
    const k = (suffix: string, vars?: Record<string, string | number>) => t(`domains.deletionGuide.${suffix}` as TranslationKey, vars);
    const side = input.reason === 'dns_peer_proof_timeout' && input.detail
        ? t(`err.DNS_PUBLICATION_FAILED.dns_peer_proof_timeout.${input.detail}` as TranslationKey)
        : '';
    const why = side && !side.startsWith('err.') ? `${k(`${guide}.why`)} ${side}` : k(`${guide}.why`);
    const window = place === 'window';
    // The row offers no Delete while the list carries this reason on by
    // itself (domainDeletionBlocksRetry): its texts never send anyone there.
    const noRetry = domainDeletionBlocksRetry(input.reason);
    switch (deletionGuideFollowUp(input.reason, input.followUp)) {
        case 'person':
            return admin
                ? {
                    tone: 'blocked', automatic: false, why, who: k('who.youAdministrator'),
                    next: guide === 'journalUnresolved' ? k(window ? 'journalUnresolved.nextWindow' : 'journalUnresolved.next') : k('stuck.next'),
                    then: k(noRetry ? 'then.stoppedSupport' : 'then.person'),
                }
                : { tone: 'blocked', automatic: false, why, who: k('who.administrator'), next: k('journalUnresolved.nextOther'), then: k('then.personOther') };
        case 'stopped':
            return admin
                ? { tone: 'blocked', automatic: false, why, who: k('who.youAdministrator'), next: k(window ? 'stopped.nextWindow' : 'stopped.next'), then: k(noRetry ? 'then.stoppedSupport' : 'then.stopped') }
                : { tone: 'blocked', automatic: false, why, who: k('who.administrator'), next: k('stuck.nextOther'), then: k(noRetry ? 'then.stoppedSupport' : 'then.stoppedOther') };
        case 'stuck':
            return {
                tone: 'blocked', automatic: false, why,
                who: admin ? k('who.youAdministrator') : k('who.administrator'),
                next: admin ? k('stuck.next') : k('stuck.nextOther'),
                then: k('then.stuck'),
            };
        default: {
            const at = input.nextCheckAt ? Date.parse(input.nextCheckAt) : Number.NaN;
            const then = Number.isNaN(at)
                ? k('then.soon')
                : at <= now
                    ? k('then.now')
                    : k('then.at', { time: formatTime(new Date(at), locale) });
            return { tone: 'waiting', automatic: true, why, who: k('who.nobody'), then };
        }
    }
}

/**
 * The window's and the bulk notes' one paragraph: why, who acts, next step,
 * how it continues. With t, who acts and the next step carry the guidance
 * block's own labels ("Kim yapar:", "Sonraki adım:"), as the banner does.
 */
export function deletionGuideText(words: DeletionGuideWords, t?: Translate): string {
    const labelled = (key: TranslationKey, text?: string) => (text && t ? `${t(key)}: ${text}` : text);
    return [words.why, labelled('kit.guidance.who', words.who), labelled('kit.guidance.next', words.next), words.then].filter(Boolean).join(' ');
}
