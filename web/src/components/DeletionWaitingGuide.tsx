import { Guidance } from './kit/rows/Guidance';
import { Button } from './ui';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { domainDeletionDetailKey, domainDeletionGuideReason } from '../lib/domainDeletionPending';
import {
    DELETION_LOG_COMMAND, DELETION_RECORD_LIST_COMMAND, deletionGuideFollowUp, deletionGuideWords, type DeletionGuideInput,
} from '../lib/deletionGuide';

// A domain deletion waiting for its DNS step on Domains (alpha.133, D-024):
// what waits, why, who acts, the next step only when someone has one, how
// it continues, the one button this screen has for it, and the
// administrator's commands with the technical explanation behind a closed
// "Details" (administrators only). The words are lib/deletionGuide.ts's,
// shared with the deletion window.
//
// DNS adımını bekleyen silme: ne bekliyor, neden, kim yapar, sonraki adım,
// nasıl sürer; komut ve teknik açıklama kapalı "Ayrıntılar" altında.

export function DeletionWaitingGuide({ name, input, admin, onCheck }: {
    name: string;
    input: DeletionGuideInput;
    admin: boolean;
    onCheck: () => void;
}) {
    const { t, locale } = useI18n();
    const words = deletionGuideWords(input, admin, t, locale, 'banner');
    const guide = domainDeletionGuideReason(input.reason, input.detail);
    if (!words || !guide) return null;
    // What the backup's check tool reported, or which comparison differed:
    // technical, so only in the administrator's Details.
    const detailKey = domainDeletionDetailKey(input.reason, input.detail ?? '') as TranslationKey | null;
    // (The proof timeout's side is already part of its why.)
    const detailText = detailKey && input.reason !== 'dns_peer_proof_timeout' ? t(detailKey) : '';
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
                    {detailText && detailText !== detailKey && <p className="mt-1 break-words">{detailText}</p>}
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
