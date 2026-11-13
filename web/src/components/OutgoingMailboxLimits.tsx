import { useMemo, useState } from 'react';
import { AlertTriangle, KeyRound, Send } from 'lucide-react';
import { useAuth } from '../auth/AuthContext';
import { useMailOutgoingWords } from '../i18n/mailOutgoing';
import { readApiError, apiErrorText } from '../lib/apiError';
import { invalidate, lastKnown, useRemote } from '../lib/remote';
import { localeTag } from '../lib/dateFormat';
import { answerWasLost } from '../lib/requestIdentity';
import { useNavigate } from '../router';
import { showToast } from './Toast';
import { Button, RemoteGate, inputClass } from './ui';
import { useDraft } from './kit/save/draft';
import { ChangedMark, SaveBar, ServerMovedNote, changedRowClass } from './kit/save/SaveBar';
import { useSaveWords } from './kit/save/words';

// A domain's mailboxes and their hourly sending limits (alpha.128, gap G1).
// The state of the limits comes first (off, not in force, being set up),
// with who acts; then each mailbox: its limit, what it sent this hour, and a
// security pause with its one way out (a new password). A reseller or
// customer chooses within the plan's ceiling; the administrator may go above.
//
// Bir alan adının posta kutuları ve saatlik gönderim sınırları: önce
// sınırların durumu ve kimin ne yapacağı, sonra her kutu: sınırı, bu saatki
// gönderimi ve güvenlik duraklatmasından tek çıkış (yeni parola).

interface Paused {
    paused_at: string;
    reason: 'spike' | 'refusals' | 'manual' | string;
    detail?: string;
}

interface Effective {
    limit: number;
    mode: 'default' | 'custom' | 'unlimited';
    custom?: number;
    default: number;
    ceiling: number;
    new_until?: string;
}

interface MailboxRow {
    id: number;
    address: string;
    effective: Effective;
    used_hour: number;
    sent_7d: number;
    refused_7d: number;
    paused?: Paused;
}

interface DomainOutgoing {
    enforced: boolean;
    state: 'off' | 'pending' | 'not_enforced' | 'on';
    reason?: string;
    domain_limit: number;
    domain_used_hour: number;
    subscription_limit: number;
    ceiling: number;
    default: number;
    may_exceed_ceiling: boolean;
    mailboxes: MailboxRow[];
}

// A mailbox's limit as one value of the draft: 'default', 'unlimited' or
// 'custom:<per hour>' (the number as typed while it is being typed).
// Bir kutunun sınırı taslağın tek değeri olarak.
type Mode = Effective['mode'];
const limitOf = (e: Effective) => (e.mode === 'custom' ? `custom:${e.custom ?? ''}` : e.mode);
const modeOf = (limit: string): Mode => (limit.startsWith('custom:') ? 'custom' : limit as Mode);
const perHourOf = (limit: string) => limit.slice('custom:'.length);

function decodeDomainOutgoing(raw: unknown): DomainOutgoing {
    const value = raw as Partial<DomainOutgoing> | null;
    if (!value || typeof value.state !== 'string' || !Array.isArray(value.mailboxes) || typeof value.ceiling !== 'number') {
        throw new Error('outgoing');
    }
    return value as DomainOutgoing;
}

export function OutgoingMailboxLimits({ domainId, readOnly, onChangePassword }: {
    domainId: number;
    readOnly?: boolean;
    /** Opens the mailbox's own change-password dialogue. */
    onChangePassword: (id: number, address: string) => void;
}) {
    const { t, locale } = useMailOutgoingWords();
    const { role } = useAuth();
    const navigate = useNavigate();
    const url = `/api/v1/domains/${domainId}/mail/outgoing`;
    const read = useRemote(url, decodeDomainOutgoing);
    // alpha.132: the mailboxes' limits are a draft over the list last read.
    // "Change limit" opens a mailbox's choice; nothing is sent until Save in
    // the bar; Discard puts the server's limits back; leaving with a change
    // asks first. Save sends each changed mailbox once, in order.
    // Kutuların sınırları son okunan listenin üzerinde bir taslaktır;
    // Kaydet'e dek hiçbir şey gönderilmez.
    const answer = lastKnown(read.remote)?.value;
    const known = useMemo(() => answer && Object.fromEntries(answer.mailboxes.map((row) => [String(row.id), limitOf(row.effective)])), [answer]);
    const draft = useDraft<Record<string, string>>(known);
    const say = useSaveWords();
    const [opened, setOpened] = useState<number[]>([]);
    const admin = role === 'admin';
    const limitText = (n: number) => (n > 0 ? String(n) : t('mout.dom.none'));
    const time = (iso: string) => new Date(iso).toLocaleString(locale, { dateStyle: 'short', timeStyle: 'short' });
    const modeWords = (view: DomainOutgoing, limit: string) => {
        const mode = modeOf(limit);
        return mode === 'unlimited' ? t('mout.dom.mode.unlimited')
            : mode === 'custom' ? `${t('mout.dom.mode.custom')}: ${perHourOf(limit)}`
                : view.default > 0 ? t('mout.dom.mode.default', { n: view.default }) : t('mout.dom.mode.defaultNone');
    };
    // The first changed mailbox whose hourly number is not one the plan allows.
    // Planın izin vermediği saatlik sayıyı taşıyan ilk değişmiş kutu.
    const invalid = (view: DomainOutgoing) => {
        const max = view.ceiling > 0 && !view.may_exceed_ceiling ? view.ceiling : 1000000;
        const row = view.mailboxes.find((item) => {
            const limit = draft.values?.[String(item.id)] ?? '';
            if (!draft.isChanged(String(item.id)) || modeOf(limit) !== 'custom') return false;
            const n = Number(perHourOf(limit));
            return !/^\d+$/.test(perHourOf(limit)) || n < 1 || n > max;
        });
        return row ? t('mout.dom.invalid', { address: row.address, max: max.toLocaleString(localeTag(locale)) }) : undefined;
    };

    const save = (view: DomainOutgoing) => void draft.save(async (changes) => {
        let applied = true;
        try {
            for (const [id, limit] of Object.entries(changes) as [string, string][]) {
                const mode = modeOf(limit);
                let res: Response;
                try {
                    res = await fetch(url, {
                        method: 'PUT',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ id: Number(id), mode, per_hour: mode === 'custom' ? Number(perHourOf(limit)) : 0 }),
                    });
                } catch {
                    return 'unknown';
                }
                if (answerWasLost(res)) return 'unknown';
                if (!res.ok) {
                    showToast('error', apiErrorText(await readApiError(res), t));
                    return 'refused';
                }
                // An answer without its details is saved but not known to be in force.
                // Ayrıntısız yanıt kaydedilmiştir ama uygulandığı bilinmez.
                try {
                    applied = applied && ((await res.json()) as { applied?: boolean }).applied === true;
                } catch {
                    applied = false;
                }
            }
        } finally {
            // What was sent before a refusal or a lost answer is read back too.
            // Bir ret ya da yiten yanıttan önce gönderilenler de geri okunur.
            invalidate('mail');
        }
        showToast('success', t(view.state === 'on' && applied ? 'mout.dom.saved' : 'mout.dom.savedPending'));
        setOpened([]);
        return 'saved';
    });
    const discard = () => {
        draft.reset();
        setOpened([]);
    };

    return (
        <section className="mt-5 rounded-xl border border-border bg-surface p-4" data-outgoing-limits>
            <h3 className="mb-2 flex items-center gap-2 text-sm font-semibold text-fg">
                <Send className="h-4 w-4 text-fg-subtle" aria-hidden="true" />
                {t('mout.dom.title')}
            </h3>
            <RemoteGate remote={read.remote} checking={t('mout.dom.reading')} failed={t('mout.dom.failed')} onRetry={() => void read.retry()} busy={read.reading}>
                {({ value: view, stale }) => (
                    <>
                        {view.state !== 'on' && (
                            <div role="status" data-outgoing-state={view.state} className="mb-3 flex flex-wrap items-start gap-2 rounded-lg border border-border bg-surface-2/50 p-3 text-sm text-fg">
                                <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
                                <p className="min-w-0 max-w-[75ch] flex-1">
                                    {view.state === 'off' ? t(admin ? 'mout.dom.offAdmin' : 'mout.dom.off')
                                        : view.state === 'pending' ? t('mout.dom.pending')
                                            : t(admin ? 'mout.dom.notEnforcedAdmin' : 'mout.dom.notEnforced')}
                                </p>
                                {admin && view.state !== 'pending' && (
                                    <Button type="button" onClick={() => navigate('/components/postfix')}>{t('mout.dom.openSettings')}</Button>
                                )}
                            </div>
                        )}
                        <p className="mb-3 text-xs text-fg-muted">
                            {t('mout.dom.domainUse', { used: view.domain_used_hour, limit: limitText(view.domain_limit) })}{' '}
                            {t('mout.dom.subscriptionUse', { limit: limitText(view.subscription_limit) })}
                        </p>
                        {view.mailboxes.length === 0 ? (
                            <p className="text-sm text-fg-muted">{t('mout.dom.empty')}</p>
                        ) : (
                            <ul className="divide-y divide-border">
                                {view.mailboxes.map((row) => {
                                    const e = row.effective;
                                    const id = String(row.id);
                                    const limit = draft.values?.[id] ?? limitOf(e);
                                    const modeText = e.mode === 'unlimited' ? t('mout.dom.mode.unlimited')
                                        : e.mode === 'custom' ? `${t('mout.dom.mode.custom')}: ${e.custom}`
                                            : e.default > 0 ? t('mout.dom.mode.default', { n: e.default }) : t('mout.dom.mode.defaultNone');
                                    return (
                                        <li key={row.id} className="py-3" data-outgoing-mailbox={row.address}>
                                            <div className="flex flex-wrap items-center justify-between gap-2">
                                                <span className="font-medium text-fg">{row.address}</span>
                                                <span className="text-xs text-fg-muted">
                                                    {t('mout.dom.used', { used: row.used_hour, sent: row.sent_7d })}
                                                    {row.refused_7d > 0 && <> · <span className="text-warning">{t('mout.dom.refused', { n: row.refused_7d })}</span></>}
                                                </span>
                                            </div>
                                            {row.paused && (
                                                <div role="alert" className="mt-2 flex flex-wrap items-start gap-2 rounded-lg border border-danger/40 bg-danger/10 p-3 text-sm text-fg">
                                                    <p className="min-w-0 max-w-[75ch] flex-1">
                                                        {row.paused.reason === 'manual'
                                                            ? t(admin || role === 'reseller' ? 'mout.dom.pausedManual.manager' : 'mout.dom.pausedManual.customer')
                                                            : t('mout.dom.paused', { reason: t(`mout.paused.reason.${row.paused.reason}` as never) })}
                                                        {row.paused.detail && <span className="mt-1 block text-xs text-fg-muted">{row.paused.detail}</span>}
                                                    </p>
                                                    {!readOnly && row.paused.reason !== 'manual' && (
                                                        <Button type="button" variant="primary" icon={KeyRound} onClick={() => onChangePassword(row.id, row.address)} data-outgoing-resume={row.address}>
                                                            {t('mout.dom.changePassword')}
                                                        </Button>
                                                    )}
                                                </div>
                                            )}
                                            {opened.includes(row.id) || draft.isChanged(id) ? (
                                                <div className={`-mx-2 mt-2 rounded-r px-2 py-2 ${draft.isChanged(id) ? changedRowClass : ''}`}>
                                                    <div className="flex flex-wrap items-center gap-2">
                                                        <select className={`${inputClass} field-md`} value={modeOf(limit)} disabled={draft.saving} aria-label={t('mout.mailbox')}
                                                            onChange={(event) => {
                                                                const mode = event.target.value as Mode;
                                                                draft.set(id, mode !== 'custom' ? mode : `custom:${e.custom ?? (e.default > 0 ? e.default : 100)}`);
                                                            }}>
                                                            <option value="default">{view.default > 0 ? t('mout.dom.mode.default', { n: view.default }) : t('mout.dom.mode.defaultNone')}</option>
                                                            <option value="custom">{t('mout.dom.mode.custom')}</option>
                                                            {(view.ceiling === 0 || view.may_exceed_ceiling) && <option value="unlimited">{t('mout.dom.mode.unlimited')}</option>}
                                                        </select>
                                                        {modeOf(limit) === 'custom' && (
                                                            <label className="flex items-center gap-2 text-xs text-fg-muted">
                                                                <span>
                                                                    <input type="number" min={1} max={view.ceiling > 0 && !view.may_exceed_ceiling ? view.ceiling : 1000000} value={perHourOf(limit)}
                                                                        disabled={draft.saving} onChange={(event) => draft.set(id, `custom:${event.target.value.trim()}`)}
                                                                        className={`${inputClass} field-sm`} aria-label={t('mout.perHour')} />
                                                                </span>
                                                                {t('mout.perHour')}
                                                            </label>
                                                        )}
                                                        {draft.isChanged(id) ? <ChangedMark from={modeWords(view, draft.base?.[id] ?? limit)} to={modeWords(view, limit)} />
                                                            : <button type="button" className="rounded px-1.5 py-0.5 text-xs text-primary hover:bg-surface-2" onClick={() => setOpened((ids) => ids.filter((item) => item !== row.id))}>{t('common.close')}</button>}
                                                    </div>
                                                    {view.ceiling > 0 && !view.may_exceed_ceiling && <p className="mt-1 text-xs text-fg-subtle">{t('mout.dom.ceiling', { n: view.ceiling })}</p>}
                                                    {draft.isMoved(id) && draft.base && (
                                                        <ServerMovedNote value={modeWords(view, draft.base[id])} onTake={() => draft.take(id)} />
                                                    )}
                                                </div>
                                            ) : (
                                                <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-fg-muted">
                                                    <span>{modeText}</span>
                                                    {e.new_until && <span>· {t('mout.dom.newUntil', { n: e.limit, time: time(e.new_until) })}</span>}
                                                    {!readOnly && !stale && (
                                                        <button type="button" className="rounded px-1.5 py-0.5 text-primary hover:bg-surface-2" onClick={() => setOpened((ids) => [...ids, row.id])}>
                                                            {t('mout.dom.change')}
                                                        </button>
                                                    )}
                                                </div>
                                            )}
                                        </li>
                                    );
                                })}
                            </ul>
                        )}
                        {!readOnly && (
                            <SaveBar
                                className="-mx-4 px-4 pb-0"
                                count={draft.count}
                                saving={draft.saving}
                                blockedBy={stale ? say('stale') : invalid(view)}
                                unknown={draft.outcome === 'unknown' ? { held: draft.held, checking: read.reading, onCheck: () => void read.retry() } : undefined}
                                onSave={() => save(view)}
                                onDiscard={discard}
                            />
                        )}
                    </>
                )}
            </RemoteGate>
        </section>
    );
}

export default OutgoingMailboxLimits;
