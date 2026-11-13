import { Guidance, type GuidanceTone } from './kit/rows/Guidance';
import { Button } from './ui';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { formatTime } from '../lib/dateFormat';
import { domainDeletionGuideReason, type DomainDeletionFollowUp } from '../lib/domainDeletionPending';

// A domain deletion waiting for its DNS step (alpha.133, D-024): what waits,
// why in one sentence, who acts, the next step only when someone has one,
// how it continues, the one button this screen has for it, and the
// administrator's command and the technical explanation behind a closed
// "Details". "You do not need to do anything" is said only while CelikPanel
// re-checks by itself and that can finish the deletion (follow_up
// "automatic"); otherwise the administrator is named with the one action.
// The words are the "deletionGuide" screen part's, also used by the
// deletion window (deletionGuideText) and the server's notification.
//
// DNS adımını bekleyen silme: ne bekliyor, neden, kim yapar, sonraki adım,
// nasıl sürer; komut ve teknik açıklama kapalı "Ayrıntılar" altında.

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

interface GuideWords {
    tone: GuidanceTone;
    why: string;
    who: string;
    next?: string;
    then: string;
}

/** The guide's sentences for one reason, or null when the reason has no guide. */
export function deletionGuideWords(
    input: DeletionGuideInput,
    admin: boolean,
    t: (key: TranslationKey, vars?: Record<string, string | number>) => string,
    locale: string,
    now: number = Date.now(),
): GuideWords | null {
    const guide = domainDeletionGuideReason(input.reason);
    if (!guide) return null;
    const k = (suffix: string, vars?: Record<string, string | number>) => t(`domains.deletionGuide.${suffix}` as TranslationKey, vars);
    const side = input.reason === 'dns_peer_proof_timeout' && input.detail
        ? t(`err.DNS_PUBLICATION_FAILED.dns_peer_proof_timeout.${input.detail}` as TranslationKey)
        : '';
    const why = side && !side.startsWith('err.') ? `${k(`${guide}.why`)} ${side}` : k(`${guide}.why`);
    const administrator = admin ? k('who.youAdministrator') : k('who.administrator');
    switch (deletionGuideFollowUp(input.reason, input.followUp)) {
        case 'person':
            return {
                tone: 'blocked', why, who: administrator,
                next: admin ? k(`${guide === 'journalUnresolved' ? 'journalUnresolved' : 'stuck'}.next`) : k('journalUnresolved.nextOther'),
                then: k('then.person'),
            };
        case 'stopped':
            return { tone: 'blocked', why, who: administrator, next: admin ? k('stopped.next') : k('stuck.nextOther'), then: k('then.stopped') };
        case 'stuck':
            return { tone: 'blocked', why, who: administrator, next: admin ? k('stuck.next') : k('stuck.nextOther'), then: k('then.stuck') };
        default: {
            const at = input.nextCheckAt ? Date.parse(input.nextCheckAt) : Number.NaN;
            const then = Number.isNaN(at)
                ? k('then.soon')
                : at <= now
                    ? k('then.now')
                    : k('then.at', { time: formatTime(new Date(at), locale) });
            return { tone: 'waiting', why, who: k('who.nobody'), then };
        }
    }
}

/** The deletion window's one paragraph: why, who acts, next step, how it continues. */
export function deletionGuideText(words: GuideWords): string {
    return [words.why, words.who, words.next, words.then].filter(Boolean).join(' ');
}

export function DeletionWaitingGuide({ name, input, admin, onCheck }: {
    name: string;
    input: DeletionGuideInput;
    admin: boolean;
    onCheck: () => void;
}) {
    const { t, locale } = useI18n();
    const words = deletionGuideWords(input, admin, t, locale);
    const guide = domainDeletionGuideReason(input.reason);
    if (!words || !guide) return null;
    return (
        <Guidance
            className="mb-4"
            tone={words.tone}
            title={t('domains.deletionWaiting', { name })}
            why={words.why}
            who={words.who}
            next={words.next}
            then={words.then}
            action={<Button variant="secondary" onClick={onCheck}>{t('domains.checkDeletionStatus')}</Button>}
            data-domain-deletion-guide={name}
            data-follow-up={deletionGuideFollowUp(input.reason, input.followUp)}
        >
            {admin && (
                <details className="text-xs text-fg-muted" data-deletion-guide-details="">
                    <summary className="cursor-pointer">{t(`domains.deletionGuide.details` as TranslationKey)}</summary>
                    <p className="mt-1 break-words">{t(`domains.deletionGuide.${guide}.details` as TranslationKey)}</p>
                    <code className="mt-1 block break-words">{DELETION_LOG_COMMAND}</code>
                    {input.reason === 'dns_peer_journal_unresolved' && (
                        <code className="mt-1 block break-words">{DELETION_RECORD_LIST_COMMAND}</code>
                    )}
                </details>
            )}
        </Guidance>
    );
}
