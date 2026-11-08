import { useState } from 'react';
import { AlertTriangle, KeyRound, Send } from 'lucide-react';
import { useAuth } from '../auth/AuthContext';
import { useMailOutgoingWords } from '../i18n/mailOutgoing';
import { readApiError, apiErrorText } from '../lib/apiError';
import { invalidate, useRemote } from '../lib/remote';
import { useNavigate } from '../router';
import { showToast } from './Toast';
import { Button, RemoteGate, inputClass } from './ui';

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
    const [editing, setEditing] = useState<number | null>(null);
    const [mode, setMode] = useState<'default' | 'custom' | 'unlimited'>('default');
    const [perHour, setPerHour] = useState(100);
    const [busy, setBusy] = useState(false);
    const admin = role === 'admin';
    const limitText = (n: number) => (n > 0 ? String(n) : t('mout.dom.none'));
    const time = (iso: string) => new Date(iso).toLocaleString(locale, { dateStyle: 'short', timeStyle: 'short' });

    const save = async (row: MailboxRow, view: DomainOutgoing) => {
        if (busy) return;
        setBusy(true);
        try {
            const res = await fetch(url, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ id: row.id, mode, per_hour: mode === 'custom' ? perHour : 0 }),
            });
            if (!res.ok) {
                showToast('error', apiErrorText(await readApiError(res), t));
                return;
            }
            const answer = await res.json() as { applied?: boolean; unknown?: boolean };
            showToast('success', t(view.state === 'on' && answer.applied ? 'mout.dom.saved' : 'mout.dom.savedPending'));
            setEditing(null);
        } catch {
            showToast('error', t('common.resultUnknown'));
        } finally {
            setBusy(false);
            invalidate('mail');
        }
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
                                            {editing === row.id ? (
                                                <div className="mt-2 flex flex-wrap items-end gap-2">
                                                    <select className={`${inputClass} w-auto`} value={mode} onChange={(event) => setMode(event.target.value as typeof mode)} aria-label={t('mout.mailbox')}>
                                                        <option value="default">{view.default > 0 ? t('mout.dom.mode.default', { n: view.default }) : t('mout.dom.mode.defaultNone')}</option>
                                                        <option value="custom">{t('mout.dom.mode.custom')}</option>
                                                        {(view.ceiling === 0 || view.may_exceed_ceiling) && <option value="unlimited">{t('mout.dom.mode.unlimited')}</option>}
                                                    </select>
                                                    {mode === 'custom' && (
                                                        <label className="text-xs text-fg-muted">
                                                            <input type="number" min={1} max={view.ceiling > 0 && !view.may_exceed_ceiling ? view.ceiling : 1000000} value={perHour}
                                                                onChange={(event) => setPerHour(Math.max(1, parseInt(event.target.value, 10) || 1))}
                                                                className={`${inputClass} w-28`} aria-label={t('mout.perHour')} />
                                                            <span className="ml-1">{t('mout.perHour')}</span>
                                                        </label>
                                                    )}
                                                    <Button type="button" variant="primary" disabled={busy || stale} loading={busy} onClick={() => void save(row, view)}>{t('mout.dom.save')}</Button>
                                                    <Button type="button" disabled={busy} onClick={() => setEditing(null)}>{t('common.cancel')}</Button>
                                                    {view.ceiling > 0 && !view.may_exceed_ceiling && <p className="w-full text-xs text-fg-subtle">{t('mout.dom.ceiling', { n: view.ceiling })}</p>}
                                                </div>
                                            ) : (
                                                <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-fg-muted">
                                                    <span>{modeText}</span>
                                                    {e.new_until && <span>· {t('mout.dom.newUntil', { n: e.limit, time: time(e.new_until) })}</span>}
                                                    {!readOnly && !stale && (
                                                        <button type="button" className="rounded px-1.5 py-0.5 text-primary hover:bg-surface-2" onClick={() => {
                                                            setEditing(row.id);
                                                            setMode(e.mode);
                                                            setPerHour(e.custom ?? (e.default > 0 ? e.default : 100));
                                                        }}>{t('mail.editQuota')}</button>
                                                    )}
                                                </div>
                                            )}
                                        </li>
                                    );
                                })}
                            </ul>
                        )}
                    </>
                )}
            </RemoteGate>
        </section>
    );
}

export default OutgoingMailboxLimits;
