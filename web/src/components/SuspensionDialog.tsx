import { useState } from 'react';
import { Ban, Power, PowerOff } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { readApiError, type ApiError } from '../lib/apiError';
import { invalidate, lastKnown, useRefreshEvery, useRemote } from '../lib/remote';
import {
    accountSuspensionURL,
    availableActions,
    causeKeys,
    decodeAccountSuspension,
    decodeSubscriptionAsAccount,
    operationRunning,
    subscriptionSuspensionURL,
    type SubscriptionSuspensionView,
    type SuspensionOperation,
    type SuspensionPreview,
} from '../lib/suspension';
import { showToast } from './Toast';
import { Button, Dialog, ErrorBanner, RemoteGate, Spinner, inputClass } from './ui';

// Suspend or resume the services of an account or of one subscription (owner,
// 2 Nov 2026). The dialogue reads what the change would touch and says it
// BEFORE the person confirms; a suspension needs a reason. The work runs on the
// server in the background: this window then reads its progress every two
// seconds and shows, first, why it is where it is, who acts, what to do and how
// it continues (D-024), and only then the per-service list. Reading never
// starts anything; a lost answer is never sent a second time.
//
// Bir hesabın ya da aboneliğin hizmetlerini askıya alma / yeniden açma. Pencere
// onaydan ÖNCE neyin değişeceğini okuyup söyler; askıya almak neden ister. İş
// sunucuda arka planda yürür; pencere ilerlemeyi iki saniyede bir okur ve önce
// nedeni, kimin işlem yapacağını, ne yapılacağını ve nasıl devam edileceğini
// (D-024), sonra hizmet listesini gösterir. Okuma hiçbir şey başlatmaz.
export type SuspensionTarget = { kind: 'user' | 'subscription'; id: number; name: string };

const REASON_MAX = 500;
const POLL_MS = 2000;
type Choice = 'suspend' | 'resume';

export function SuspensionDialog({ target, onClose }: { target: SuspensionTarget; onClose: () => void }) {
    const { t } = useI18n();
    const user = target.kind === 'user';
    const handle = useRemote(user ? accountSuspensionURL(target.id) : subscriptionSuspensionURL(target.id), user ? decodeAccountSuspension : decodeSubscriptionAsAccount);
    const [picked, setPicked] = useState<Choice | null>(null);
    const [reason, setReason] = useState('');
    const [sending, setSending] = useState(false);
    const [refusal, setRefusal] = useState<ApiError | null>(null);

    const view = lastKnown(handle.remote)?.value ?? null;
    // The server's operation is still working: ask how far it is, every two seconds.
    useRefreshEvery(handle, POLL_MS, view?.subscriptions.some(operationRunning) === true);

    // An action is offered on a state that was just read, never on an earlier or unread one.
    const known = handle.remote.state === 'known' && !handle.reading;
    const actions = view ? availableActions(view) : { suspend: false, resume: false };
    const choice: Choice | null = picked && actions[picked] ? picked
        : actions.suspend && !actions.resume ? 'suspend'
            : actions.resume && !actions.suspend ? 'resume'
                : null;
    const reasonMissing = choice === 'suspend' && reason.trim() === '';

    // One request: the answer to a change that did not arrive is not known, and
    // nothing is sent again; the state is read so the person can look.
    // Bir istek: gelmeyen yanıt bilinmez, yeniden gönderilmez; durum okunur.
    const send = async (url: string, body: Record<string, string>, started: TranslationKey) => {
        setSending(true);
        setRefusal(null);
        try {
            const res = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
            if (!res.ok) {
                setRefusal(await readApiError(res));
            } else {
                showToast('success', t(started));
                setReason('');
                setPicked(null);
            }
        } catch {
            showToast('error', t('common.resultUnknown'));
        } finally {
            setSending(false);
            invalidate('users', 'suspension');
        }
    };

    const confirm = () => {
        if (!choice || !view) return;
        const body: Record<string, string> = choice === 'suspend' ? { action: 'suspend', reason: reason.trim() } : { action: 'resume' };
        void send(user ? accountSuspensionURL(target.id) : subscriptionSuspensionURL(target.id), body, choice === 'suspend' ? 'susp.started.suspend' : 'susp.started.resume');
    };
    const retry = (subscription: SubscriptionSuspensionView) => {
        void send(subscriptionSuspensionURL(subscription.subscription_id), { action: 'retry' }, 'susp.retried');
    };

    return (
        <Dialog
            id="suspension"
            title={t('susp.title', { name: target.name })}
            description={t('susp.intro')}
            icon={Ban}
            width="xl"
            busy={sending}
            onDismiss={onClose}
            footerLead={reasonMissing ? <p id="suspension-reason-missing" className="text-sm text-fg" data-blocked-reason="">{t('susp.reasonMissing')}</p> : undefined}
            actions={
                <>
                    <Button onClick={onClose} disabled={sending}>{t('common.close')}</Button>
                    {choice && (
                        <Button
                            variant={choice === 'suspend' ? 'danger' : 'primary'}
                            icon={choice === 'suspend' ? PowerOff : Power}
                            loading={sending}
                            disabled={sending || !known || reasonMissing}
                            aria-describedby={reasonMissing ? 'suspension-reason-missing' : undefined}
                            onClick={confirm}
                            data-suspension-confirm={choice}
                        >
                            {t(choice === 'suspend' ? 'susp.choose.suspend' : 'susp.choose.resume')}
                        </Button>
                    )}
                </>
            }
        >
            <RemoteGate remote={handle.remote} checking={t('susp.checking')} failed={t('susp.unknown')} onRetry={() => void handle.retry()} busy={handle.reading}>
                {({ value }) => (
                    <div className="space-y-4 text-sm leading-6 text-fg">
                        <ErrorBanner error={refusal} sentence="susp.failed" />
                        {value.role === 'reseller' && <p className="text-fg-muted" data-suspension-reseller="">{t('susp.resellerScope')}</p>}
                        {value.subscriptions.length === 0 && <p className="text-fg-muted">{t('susp.noSubscriptions')}</p>}
                        {value.subscriptions.map((subscription) => (
                            <SubscriptionBlock key={subscription.subscription_id} subscription={subscription} choice={choice} busy={sending || !known} onRetry={retry} />
                        ))}
                        {actions.suspend && actions.resume && (
                            <div role="group" aria-label={t('susp.choose')} className="flex flex-wrap gap-2">
                                {(['suspend', 'resume'] as const).map((which) => (
                                    <Button key={which} variant={choice === which ? 'primary' : 'secondary'} aria-pressed={choice === which} onClick={() => setPicked(which)}>
                                        {t(which === 'suspend' ? 'susp.choose.suspend' : 'susp.choose.resume')}
                                    </Button>
                                ))}
                            </div>
                        )}
                        {choice === 'suspend' && (
                            <label className="block">
                                <span className="mb-1 block text-xs text-fg-muted">{t('susp.reasonField')}</span>
                                <textarea
                                    value={reason}
                                    maxLength={REASON_MAX}
                                    rows={3}
                                    onChange={(event) => setReason(event.target.value)}
                                    className={inputClass}
                                    aria-describedby={reasonMissing ? 'suspension-reason-missing' : undefined}
                                />
                                <span className="mt-1 block text-xs text-fg-subtle">{t('susp.reasonHint', { max: REASON_MAX })}</span>
                            </label>
                        )}
                    </div>
                )}
            </RemoteGate>
        </Dialog>
    );
}

// One subscription: where it stands, then the guidance of its operation (when
// there is one), then what the chosen action would touch.
function SubscriptionBlock({ subscription, choice, busy, onRetry }: {
    subscription: SubscriptionSuspensionView;
    choice: Choice | null;
    busy: boolean;
    onRetry: (subscription: SubscriptionSuspensionView) => void;
}) {
    const { t, locale } = useI18n();
    const operation = subscription.operation ?? null;
    const since = subscription.suspended_at ? new Date(subscription.suspended_at) : null;
    const sinceText = since && !Number.isNaN(since.getTime()) ? since.toLocaleDateString(locale) : '';
    const causes = causeKeys(subscription.causes);
    const showPreview = choice !== null && subscription.preview.action === choice && !operationRunning(subscription)
        && (choice === 'suspend' ? subscription.can_suspend : subscription.can_resume);
    return (
        <section className="rounded-lg border border-border p-3" data-suspension-subscription={subscription.subscription_id}>
            <div className="flex flex-wrap items-center gap-2">
                <h4 className="text-base font-semibold">{subscription.subscription_name}</h4>
                <span className={`rounded-md px-2 py-0.5 text-xs font-medium ${subscription.status === 'active' ? 'bg-success/10 text-success' : 'bg-warning/15 text-warning'}`}>
                    {t(`susp.status.${subscription.status}` as TranslationKey)}
                </span>
            </div>
            {subscription.status !== 'active' && (
                <p className="mt-1 text-fg-muted">
                    {causes.map((cause) => t(`susp.cause.${cause}` as TranslationKey, { name: subscription.causes.reseller ?? '' })).join(' ')}
                    {subscription.reason && <> {t('susp.reasonGiven', { reason: subscription.reason })}</>}
                    {sinceText && <> {t('susp.since', { date: sinceText })}</>}
                </p>
            )}
            {subscription.resume_blocked && <p className="mt-1" data-suspension-blocked="">{t('susp.resumeBlocked', { why: subscription.resume_blocked })}</p>}
            {operation && <OperationGuidance operation={operation} busy={busy} onRetry={() => onRetry(subscription)} />}
            {showPreview && <Preview preview={subscription.preview} />}
        </section>
    );
}

const resultTone = {
    in_progress: 'border-border bg-surface-2/50',
    done: 'border-success/30 bg-success/10',
    verified_failure: 'border-danger/30 bg-danger/10',
    prerequisite: 'border-warning-mark/50 bg-warning-mark/15',
    unknown: 'border-warning-mark/50 bg-warning-mark/15',
} as const;

// D-024: the current reason, who must act, the concrete next action and how
// work resumes come first; a verified failure, an unmet prerequisite and an
// unknown result are three different things and are named as such.
// D-024: önce mevcut neden, kimin işlem yapacağı, somut sonraki eylem ve nasıl
// devam edileceği; doğrulanmış hata, eksik önkoşul ve bilinmeyen sonuç ayrıdır.
function OperationGuidance({ operation, busy, onRetry }: { operation: SuspensionOperation; busy: boolean; onRetry: () => void }) {
    const { t } = useI18n();
    const { guidance } = operation;
    const result = guidance.result in resultTone ? guidance.result : 'unknown';
    return (
        <div
            className={`mt-3 rounded-lg border p-3 ${resultTone[result]}`}
            role={operation.state === 'failed' ? 'alert' : 'status'}
            data-suspension-operation={operation.state}
            data-suspension-result={result}
        >
            <p className="flex items-center gap-2 font-semibold">
                {operation.state === 'running' && <Spinner />}
                {t(`susp.result.${result}` as TranslationKey)}
                <span className="text-xs font-normal text-fg-muted">
                    {t(operation.kind === 'resume' ? 'susp.op.resume' : 'susp.op.suspend')}
                    {operation.attempts > 1 && <> · {t('susp.op.attempts', { n: operation.attempts })}</>}
                </span>
            </p>
            <dl className="mt-2 space-y-1">
                {guidance.reason && <GuidanceLine label={t('susp.guidance.reason')} text={guidance.reason} />}
                <GuidanceLine label={t('susp.guidance.who')} text={t(`susp.who.${guidance.who_acts}` as TranslationKey)} />
                {guidance.next_action && <GuidanceLine label={t('susp.guidance.next')} text={guidance.next_action} />}
                {guidance.resumes && <GuidanceLine label={t('susp.guidance.resumes')} text={guidance.resumes} />}
            </dl>
            {operation.state === 'running' && <p className="mt-2 text-xs text-fg-muted">{t('susp.polling')}</p>}
            {operation.state === 'failed' && (
                <div className="mt-3">
                    <Button variant="primary" disabled={busy} onClick={onRetry} data-suspension-retry="">{t('susp.retry')}</Button>
                </div>
            )}
            {operation.effects.length > 0 && (
                <details className="mt-3" open={operation.state !== 'completed'}>
                    <summary className="cursor-pointer text-xs font-semibold text-fg-muted">{t('susp.effects', { n: operation.effects.length })}</summary>
                    <ul className="mt-1 space-y-0.5 text-xs">
                        {operation.effects.map((effect, index) => (
                            <li key={`${effect.kind}:${effect.target}:${index}`} className="break-words">
                                <span className="font-medium">{t(`susp.effect.kind.${effect.kind}` as TranslationKey)}</span>
                                {effect.target && <> {effect.target}</>}
                                {': '}
                                <span className={effect.state === 'failed' ? 'text-danger' : effect.state === 'manual' ? 'text-warning' : 'text-fg-muted'}>
                                    {t(`susp.effect.state.${effect.state}` as TranslationKey)}
                                </span>
                                {effect.detail && <span className="text-fg-muted"> ({effect.detail})</span>}
                            </li>
                        ))}
                    </ul>
                </details>
            )}
        </div>
    );
}

function GuidanceLine({ label, text }: { label: string; text: string }) {
    return (
        <div>
            <dt className="inline font-semibold">{label} </dt>
            <dd className="inline break-words">{text}</dd>
        </div>
    );
}

// What the action would touch, said before it is confirmed.
function Preview({ preview }: { preview: SuspensionPreview }) {
    const { t } = useI18n();
    const rows: Array<[TranslationKey, string]> = [
        ['susp.preview.sites', preview.sites.join(', ')],
        ['susp.preview.mailDomains', preview.mail_domains.join(', ')],
        ['susp.preview.mailboxes', preview.mailboxes > 0 ? String(preview.mailboxes) : ''],
        ['susp.preview.databaseUsers', preview.database_users.join(', ')],
        ['susp.preview.vpnPeers', preview.vpn_peers > 0 ? String(preview.vpn_peers) : ''],
        ['susp.preview.siteAccounts', preview.site_accounts.join(', ')],
    ];
    const shown = rows.filter(([, value]) => value !== '');
    return (
        <div className="mt-3" data-suspension-preview={preview.action}>
            <p className="font-medium">{t(preview.action === 'suspend' ? 'susp.preview.suspendHead' : 'susp.preview.resumeHead')}</p>
            {shown.length === 0 ? (
                <p className="text-fg-muted">{t('susp.preview.nothing')}</p>
            ) : (
                <dl className="mt-1 space-y-1">
                    {shown.map(([label, value]) => <GuidanceLine key={label} label={t(label)} text={value} />)}
                </dl>
            )}
            {preview.notes.length > 0 && (
                <ul className="mt-2 list-disc space-y-0.5 pl-5 text-fg-muted">
                    {preview.notes.map((note) => <li key={note}>{note}</li>)}
                </ul>
            )}
        </div>
    );
}
