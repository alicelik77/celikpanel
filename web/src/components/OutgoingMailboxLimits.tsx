import { useState, type ReactNode } from 'react';
import { AlertTriangle, Info, KeyRound, Send } from 'lucide-react';
import { useAuth } from '../auth/AuthContext';
import { useMailOutgoingWords } from '../i18n/mailOutgoing';
import type { TranslationKey } from '../i18n/en';
import { readApiError, apiErrorText } from '../lib/apiError';
import { confirmAction } from '../lib/confirmAction';
import { localeTag } from '../lib/dateFormat';
import { invalidate, lastKnown, readRemote, useRemote } from '../lib/remote';
import { answerWasLost } from '../lib/requestIdentity';
import { decodeDomainOutgoing, domainOutgoingURL, limitsApplied, type DomainOutgoing, type OutgoingEffective, type OutgoingMailbox } from '../lib/mailOutgoing';
import { useNavigate } from '../router';
import { showToast } from './Toast';
import { Button, Dialog, inputClass } from './ui';
import { useSaveRun } from './kit/save/saveRun';
import { useLeaveGuard } from './kit/save/leaveGuard';
import { ChangedMark } from './kit/save/SaveBar';
import { useSaveWords } from './kit/save/words';

// A domain's hourly sending limits on its mail screen (alpha.128, gap G1;
// alpha.135, owner 13 Nov 2026: "bu sınırlar nedir. çok fazla mı karmaşık
// ayar var acaba"). No card of their own any more:
//
//   - while the limits are really applied on this server, each mailbox row
//     carries one short line (its limit, or the new-mailbox limit and until
//     when, and what it sent this hour), and "Sending limit…" in the row's
//     menu opens the limit's dialog (explicit save, within the plan's ceiling);
//   - while they are not applied, a customer, reseller or team member sees
//     nothing about them; the administrator sees one line with its button,
//     once per page;
//   - the domain's and the hosting account's limits are named only when they
//     matter (80% of this hour used, or something refused in 7 days), as one
//     problem line with what to do;
//   - a mailbox whose sending was stopped keeps its prominent state and its
//     way out (a new password).
//
// Bir alan adının gönderim sınırları: ayrı kart yok. Sınırlar uygulanırken
// her kutu satırında tek kısa satır ve satırın menüsünde "Gönderim sınırı…";
// uygulanmazken müşteri hiçbir şey görmez, yönetici sayfada bir kez tek satır
// ve düğmesini görür; alan adı ve hesap sınırı yalnız önemli olunca söylenir.

type Translate = (key: TranslationKey, values?: Record<string, string | number>) => string;

const useOutgoing = (domainId: number) => useRemote(domainOutgoingURL(domainId), decodeDomainOutgoing);
const shortTime = (iso: string, locale: string) => new Date(iso).toLocaleString(localeTag(locale), { dateStyle: 'short', timeStyle: 'short' });

/** A mailbox's limit as words: the limit in force now (a new mailbox's, with until when), or none. */
function limitWords(t: Translate, effective: OutgoingEffective, locale: string): string {
    if (effective.limit <= 0) return t('mout.row.none');
    if (effective.new_until) return t('mout.row.new', { n: effective.limit, time: shortTime(effective.new_until, locale) });
    return t('mout.row.limit', { n: effective.limit });
}

function rowWords(t: Translate, row: OutgoingMailbox, locale: string): string {
    return `${limitWords(t, row.effective, locale)} · ${t('mout.row.hour', { n: row.used_hour })}`;
}

function Line({ tone, attr, children, action }: { tone: 'warn' | 'info'; attr: Record<string, string>; children: ReactNode; action?: ReactNode }) {
    return (
        <div role="status" {...attr} className="mb-3 flex flex-wrap items-center gap-x-3 gap-y-2 rounded-lg border border-border bg-surface-2/50 p-3 text-sm text-fg">
            <span className="flex min-w-0 max-w-[75ch] items-start gap-2">
                {tone === 'warn'
                    ? <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
                    : <Info className="mt-0.5 h-4 w-4 shrink-0 text-fg-subtle" aria-hidden="true" />}
                <span>{children}</span>
            </span>
            {action}
        </div>
    );
}

/**
 * The one line above the mailbox list: for the administrator, that the
 * limits are not applied (with the button to the mail server settings); for
 * everyone, a domain or account limit that matters now. Nothing otherwise.
 */
export function OutgoingNotice({ domainId }: { domainId: number }) {
    const { t } = useMailOutgoingWords();
    const { role } = useAuth();
    const navigate = useNavigate();
    const view = lastKnown(useOutgoing(domainId).remote)?.value;
    const admin = role === 'admin';
    if (!view) return null;
    const settings = <Button type="button" onClick={() => navigate('/components/postfix')}>{t('mout.dom.openSettings')}</Button>;
    if (!limitsApplied(view)) {
        if (!admin) return null;
        return (
            <Line tone="info" attr={{ 'data-outgoing-state': view.state }} action={view.state === 'pending' ? undefined : settings}>
                {t(view.state === 'off' ? 'mout.dom.offAdmin' : view.state === 'pending' ? 'mout.dom.pending' : 'mout.dom.notEnforcedAdmin')}
            </Line>
        );
    }
    const facts = nearLimitFacts(t, view);
    if (facts.length === 0) return null;
    return (
        <Line tone="warn" attr={{ 'data-outgoing-near': '' }}
            action={admin ? <Button type="button" onClick={() => navigate('/components/postfix#mail-plan-limits')}>{t('mout.near.openPlans')}</Button> : undefined}>
            {facts.join(' ')} {t(admin ? 'mout.near.nextAdmin' : 'mout.near.next')}
        </Line>
    );
}

/** What of the domain's and the account's limits matters now: 80% of this hour, or refusals in 7 days. */
export function nearLimitFacts(t: Translate, view: DomainOutgoing): string[] {
    const facts: string[] = [];
    const near = (used: number, limit: number) => limit > 0 && used * 5 >= limit * 4;
    if (near(view.domain_used_hour, view.domain_limit)) {
        facts.push(t(view.domain_used_hour >= view.domain_limit ? 'mout.near.domainFull' : 'mout.near.domain', { used: view.domain_used_hour, limit: view.domain_limit }));
    }
    if (near(view.subscription_used_hour, view.subscription_limit)) {
        facts.push(t(view.subscription_used_hour >= view.subscription_limit ? 'mout.near.accountFull' : 'mout.near.account', { used: view.subscription_used_hour, limit: view.subscription_limit }));
    }
    if (view.domain_refused_7d > 0) facts.push(t('mout.near.domainRefused', { n: view.domain_refused_7d }));
    if (view.subscription_refused_7d > 0) facts.push(t('mout.near.accountRefused', { n: view.subscription_refused_7d }));
    return facts;
}

/** A mailbox row's line while the limits are applied: its limit and this hour. Nothing otherwise. */
export function OutgoingRowLine({ domainId, accountId }: { domainId: number; accountId: number }) {
    const { t, locale } = useMailOutgoingWords();
    const view = lastKnown(useOutgoing(domainId).remote)?.value;
    const row = limitsApplied(view) ? view?.mailboxes.find((item) => item.id === accountId) : undefined;
    if (!row) return null;
    return (
        <span className="mt-0.5 block text-xs text-fg-muted" data-outgoing-line={row.address}>
            {rowWords(t, row, locale)}
            {row.refused_7d > 0 && <> · <span className="text-warning">{t('mout.row.refused', { n: row.refused_7d })}</span></>}
        </span>
    );
}

/**
 * A mailbox whose sending was stopped (while the limits are applied): a row of
 * the list's whole width under the mailbox, with its one way out (a new
 * password) unless it was stopped by hand.
 */
export function OutgoingPausedRow({ domainId, accountId, columns, readOnly, onChangePassword }: {
    domainId: number;
    accountId: number;
    columns: number;
    readOnly?: boolean;
    /** Opens the mailbox's own change-password dialogue. */
    onChangePassword: () => void;
}) {
    const { t } = useMailOutgoingWords();
    const { role } = useAuth();
    const view = lastKnown(useOutgoing(domainId).remote)?.value;
    const row = limitsApplied(view) ? view?.mailboxes.find((item) => item.id === accountId) : undefined;
    if (!row?.paused) return null;
    const manager = role === 'admin' || role === 'reseller';
    return (
        <tr className="border-b border-border last:border-0 max-sm:block">
            <td colSpan={columns} className="px-4 pb-3 pt-0 max-sm:block">
                <div role="alert" className="flex flex-wrap items-center gap-x-3 gap-y-2 rounded-lg border border-danger/40 bg-danger/10 p-3 text-sm text-fg" data-outgoing-paused={row.address}>
                    <p className="min-w-0 max-w-[75ch]">
                        {row.paused.reason === 'manual'
                            ? t(manager ? 'mout.dom.pausedManual.manager' : 'mout.dom.pausedManual.customer')
                            : t('mout.dom.paused', { reason: t(`mout.paused.reason.${row.paused.reason}` as TranslationKey) })}
                        {row.paused.detail && <span className="mt-1 block text-xs text-fg-muted">{row.paused.detail}</span>}
                    </p>
                    {!readOnly && row.paused.reason !== 'manual' && (
                        <Button type="button" variant="primary" icon={KeyRound} onClick={onChangePassword} data-outgoing-resume={row.address}>
                            {t('mout.dom.changePassword')}
                        </Button>
                    )}
                </div>
            </td>
        </tr>
    );
}

// A mailbox's limit as one value: 'default', 'unlimited' or 'custom:<per hour>'
// (the number as typed while it is being typed).
// Bir kutunun sınırı tek değer olarak.
type Mode = OutgoingEffective['mode'];
const limitOf = (e: OutgoingEffective) => (e.mode === 'custom' ? `custom:${e.custom ?? ''}` : e.mode);
const modeOf = (limit: string): Mode => (limit.startsWith('custom:') ? 'custom' : limit as Mode);
const perHourOf = (limit: string) => limit.slice('custom:'.length);

/**
 * "Sending limit…" of one mailbox: the choice within the plan's ceiling
 * (the administrator may go above it). Nothing is sent before Save; Discard
 * or closing with a change asks first; a lost answer is checked by reading
 * the limits again, never by sending twice.
 */
export function OutgoingLimitDialog({ domainId, accountId, address, onClose }: {
    domainId: number;
    accountId: number;
    address: string;
    onClose: () => void;
}) {
    const { t, locale } = useMailOutgoingWords();
    const say = useSaveWords();
    const url = domainOutgoingURL(domainId);
    const read = useOutgoing(domainId);
    const view = lastKnown(read.remote)?.value;
    const row = view?.mailboxes.find((item) => item.id === accountId) ?? null;
    const base = row ? limitOf(row.effective) : null;
    const [typed, setTyped] = useState<string | null>(null);
    const [lost, setLost] = useState<string | null>(null);
    const [checking, setChecking] = useState(false);
    const runner = useSaveRun();
    const value = typed ?? base ?? 'default';
    const changed = base !== null && typed !== null && typed !== base;
    useLeaveGuard(changed ? 'some' : 0);

    const max = view && view.ceiling > 0 && !view.may_exceed_ceiling ? view.ceiling : 1000000;
    const perHour = perHourOf(value);
    const invalid = modeOf(value) === 'custom' && (!/^\d+$/.test(perHour) || Number(perHour) < 1 || Number(perHour) > max)
        ? t('mout.dom.invalid', { max: max.toLocaleString(localeTag(locale)) }) : '';
    const canSave = !!row && changed && !invalid && !runner.saving && !lost && !checking;
    const modeWords = (limit: string) => {
        const mode = modeOf(limit);
        return mode === 'unlimited' ? t('mout.dom.mode.unlimited')
            : mode === 'custom' ? `${t('mout.dom.mode.custom')}: ${perHourOf(limit)}`
                : view && view.default > 0 ? t('mout.dom.mode.default', { n: view.default }) : t('mout.dom.mode.defaultNone');
    };

    const save = () => {
        if (!canSave) return;
        const sent = value;
        void runner.run(async () => {
            const mode = modeOf(sent);
            let res: Response;
            try {
                res = await fetch(url, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ id: accountId, mode, per_hour: mode === 'custom' ? Number(perHourOf(sent)) : 0 }),
                });
            } catch {
                setLost(sent);
                return 'unknown';
            }
            if (answerWasLost(res)) {
                setLost(sent);
                return 'unknown';
            }
            if (!res.ok) {
                showToast('error', apiErrorText(await readApiError(res), t));
                return 'refused';
            }
            // An answer without its details is saved but not known to be in force.
            let applied = false;
            try {
                applied = ((await res.json()) as { applied?: boolean }).applied === true;
            } catch {
                applied = false;
            }
            showToast('success', t(applied ? 'mout.dom.saved' : 'mout.dom.savedPending'));
            invalidate('mail');
            onClose();
            return 'saved';
        });
    };

    // After a lost answer the limits are read again: what was sent is either
    // there (saved) or not (the choice stays to be saved again).
    // Yanıt yitince sınırlar yeniden okunur.
    const check = async () => {
        if (!lost) return;
        setChecking(true);
        const again = await readRemote(url, decodeDomainOutgoing);
        setChecking(false);
        if (again.state !== 'known') return;
        const now = again.value.mailboxes.find((item) => item.id === accountId);
        setLost(null);
        runner.clear();
        invalidate('mail');
        if (now && limitOf(now.effective) === lost) {
            showToast('success', t('mout.dom.savedPending'));
            onClose();
        }
    };

    const dismiss = async () => {
        if (changed && !(await confirmAction({
            title: t('domain.unsaved.title'),
            lost: [t('domain.unsaved.lost')],
            kept: [say('leaveKept')],
            confirmLabel: t('domain.unsaved.close'),
            cancelLabel: t('domain.unsaved.back'),
            tone: 'danger',
        }))) return;
        onClose();
    };

    const busy = runner.saving || checking;
    return (
        <Dialog
            id="mail-outgoing-limit"
            icon={Send}
            width="sm"
            busy={busy}
            onDismiss={() => void dismiss()}
            noValidate
            onSubmit={(event) => {
                event.preventDefault();
                save();
            }}
            title={t('mout.dlg.title')}
            description={<>{t('mout.report.mailbox')}: <strong className="break-all font-medium text-fg">{address}</strong></>}
            footerLead={lost ? (
                <div className="flex flex-wrap items-center gap-x-3 gap-y-2 text-sm text-fg" aria-live="polite">
                    <p className="min-w-0 max-w-[75ch] flex-1">{say('unknown')}</p>
                    <Button type="button" onClick={() => void check()} loading={checking}>{t('common.checkAgain')}</Button>
                </div>
            ) : row ? (
                <p data-form-reason="" className={`text-sm ${changed && invalid ? 'text-danger' : 'text-fg-muted'}`} aria-live="polite">
                    {!changed ? t('domain.unsaved.none') : invalid || say('pendingSome')}
                </p>
            ) : undefined}
            actions={
                <>
                    <Button type="button" onClick={onClose} disabled={busy} data-form-discard="">{say('discard')}</Button>
                    <Button type="submit" variant="primary" disabled={!canSave} loading={runner.saving} data-form-save="">
                        {runner.saving ? say('saving') : say('save')}
                    </Button>
                </>
            }
        >
            {!view ? (
                <p className="text-sm text-fg-muted">{read.remote.state === 'loading' ? t('mout.reading') : t('mout.failed')}</p>
            ) : !row ? (
                <p className="text-sm text-fg-muted">{t('mout.dlg.gone')}</p>
            ) : (
                <div className="space-y-3">
                    <p className="text-sm text-fg-muted" data-outgoing-now="">{t('mout.dlg.now', { line: rowWords(t, row, locale) })}</p>
                    <label className="block">
                        <span className="mb-1 block text-xs text-fg-muted">{t('mout.dlg.label')}</span>
                        <span className="flex flex-wrap items-center gap-2">
                            <select className={`${inputClass} field-md`} value={modeOf(value)} disabled={busy}
                                onChange={(event) => {
                                    const mode = event.target.value as Mode;
                                    const e = row.effective;
                                    setTyped(mode !== 'custom' ? mode : `custom:${e.custom ?? (e.default > 0 ? e.default : 100)}`);
                                }}>
                                <option value="default">{view.default > 0 ? t('mout.dom.mode.default', { n: view.default }) : t('mout.dom.mode.defaultNone')}</option>
                                <option value="custom">{t('mout.dom.mode.custom')}</option>
                                {(view.ceiling === 0 || view.may_exceed_ceiling) && <option value="unlimited">{t('mout.dom.mode.unlimited')}</option>}
                            </select>
                            {modeOf(value) === 'custom' && (
                                <span className="flex items-center gap-2 text-xs text-fg-muted">
                                    <input type="number" min={1} max={max} value={perHour} disabled={busy}
                                        onChange={(event) => setTyped(`custom:${event.target.value.trim()}`)}
                                        className={`${inputClass} field-sm`} aria-label={t('mout.perHour')} />
                                    {t('mout.perHour')}
                                </span>
                            )}
                        </span>
                    </label>
                    {changed && base && <p><ChangedMark from={modeWords(base)} to={modeWords(value)} /></p>}
                    {view.ceiling > 0 && !view.may_exceed_ceiling && <p className="text-xs text-fg-subtle">{t('mout.dom.ceiling', { n: view.ceiling })}</p>}
                </div>
            )}
        </Dialog>
    );
}
