import { useEffect, useRef, useState } from 'react';
import { ArrowRightLeft, Check, Send, X } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { useAuth } from '../auth/AuthContext';
import { Button, Dialog, ErrorBanner, RemoteGate, inputClass } from './ui';
import { readApiError, type ApiError } from '../lib/apiError';
import { newRequestId } from '../lib/requestIdentity';
import { invalidate, useRemote } from '../lib/remote';
import { SUBSCRIPTIONS_URL, decodeSubscriptions } from '../lib/subscriptions';
import { MY_SUBSCRIPTION_URL, decodeMyPlan } from '../lib/myPlan';
import {
    TRANSFER_REQUESTS_URL, decodePreviewAnswer, decodeReceivers, decodeTransferList, readTransferRefusal, rightsItemLabel,
    transferRequestActionURL, transferRequestURL, transferURL,
    type TransferPreview, type TransferPreviewAnswer, type TransferRow,
} from '../lib/transfers';

// Subscription transfer (alpha.117, D-042 section 8). The screen shows the
// server's preview and sends the confirmed call; every rule is the server's.
// A confirmed call keeps one request id until its answer arrives, so pressing
// again after a lost answer never transfers twice.
//
// Abonelik devri: ekran sunucunun önizlemesini gösterir ve onaylı çağrıyı
// gönderir; her kural sunucunundur. Kaybolan yanıttan sonra yeniden basmak
// iki kez devretmez.

type Translate = (key: TranslationKey, vars?: Record<string, string | number>) => string;

// The rights state in words: Unlimited, Custom or the plan's name.
function stateText(state: string, plan: string | undefined, t: Translate): string {
    if (state === 'unlimited') return t('transfer.state.unlimited');
    if (state.startsWith('plan:')) return t('transfer.state.plan', { plan: plan ?? state });
    return t('transfer.state.custom');
}

// After a transfer every list that shows owners, usage or requests reads again.
function refreshAfterTransfer() {
    invalidate('transfers');
}

export function TransferPreviewView({ preview }: { preview: TransferPreview }) {
    const { t } = useI18n();
    const m = preview.moves;
    const reseller = preview.to_reseller?.username ?? '';
    return (
        <div className="space-y-3 text-sm text-fg" data-transfer-preview={preview.subscription_id}>
            <p className="font-medium">{t('transfer.summary', { name: preview.subscription_name, from: preview.from.username, to: preview.to.username })}</p>
            <p className="text-fg-muted">{t('transfer.hostNone')}</p>
            <p className="text-fg-muted">
                {t('transfer.moves', { domains: m.domains, sites: m.sites, mailboxes: m.mailboxes, databases: m.databases, certificates: m.certificates, vpn: m.vpn_devices })}
            </p>
            {preview.plan_converted ? (
                <p className="rounded-md bg-warning/10 p-3" data-transfer-plan-converted="">{t('transfer.planConverted', { plan: preview.rights_from_plan_name ?? '' })}</p>
            ) : (
                <p className="text-fg-muted">{t('transfer.rightsStay', { state: stateText(preview.rights_to, preview.rights_to_plan_name, t) })}</p>
            )}
            {preview.capped.length > 0 && (
                <div className="rounded-md bg-warning/10 p-3" data-transfer-capped="">
                    <p>{t('transfer.capped', { reseller })}</p>
                    <ul className="mt-1 list-disc pl-5">
                        {preview.capped.map((item) => <li key={item}>{rightsItemLabel(item, t)}</li>)}
                    </ul>
                    {preview.capped_in_use.length > 0 && (
                        <ul className="mt-1 list-disc pl-5 text-fg-muted">
                            {preview.capped_in_use.map((line) => (
                                <li key={line.item}>{t('transfer.cappedInUse', { item: rightsItemLabel(line.item, t), count: line.count })}</li>
                            ))}
                        </ul>
                    )}
                </div>
            )}
            {preview.widened.length > 0 && (
                <div>
                    <p className="text-fg-muted">{t('transfer.widened')}</p>
                    <ul className="mt-1 list-disc pl-5">{preview.widened.map((item) => <li key={item}>{rightsItemLabel(item, t)}</li>)}</ul>
                </div>
            )}
            {preview.missing.length > 0 && (
                <ul className="list-disc pl-5 text-danger" data-transfer-missing="">
                    {preview.missing.map((item) => <li key={item}>{rightsItemLabel(item, t)}</li>)}
                </ul>
            )}
            {preview.pool.length > 0 && (
                <div>
                    <p className="text-fg-muted">{t('transfer.pool', { reseller })}</p>
                    <ul className="mt-1 list-disc pl-5">
                        {preview.pool.map((line) => (
                            <li key={line.kind} className={line.fits ? '' : 'text-danger'}>
                                {t('transfer.poolLine', { kind: t(`transfer.pool.${line.kind}` as TranslationKey), used: line.used, adds: line.adds, limit: line.limit })}
                            </li>
                        ))}
                    </ul>
                </div>
            )}
            {preview.grants_removed.length > 0 && (
                <div className="rounded-md bg-warning/10 p-3" data-transfer-grants="">
                    <p>{t('transfer.grants', { owner: preview.from.username })}</p>
                    <ul className="mt-1 list-disc pl-5">
                        {preview.grants_removed.map((line) => (
                            <li key={line.user_id}>{t('transfer.grantLine', { user: line.username, n: line.subscription_grants + line.domain_grants })}</li>
                        ))}
                    </ul>
                </div>
            )}
            <p className="text-fg-muted" data-transfer-credentials="">{t('transfer.credentials')}</p>
            {m.pending_vpn_devices > 0 && <p className="text-fg-muted">{t('transfer.pendingVpn', { n: m.pending_vpn_devices })}</p>}
            {preview.from_left_without_subscription && <p className="text-fg-muted">{t('transfer.fromLeft', { owner: preview.from.username })}</p>}
            {preview.receiver_subscriptions_after > 1 && (
                <p className="text-fg-muted">{t('transfer.receiverMany', { owner: preview.to.username, n: preview.receiver_subscriptions_after })}</p>
            )}
            {preview.installed_unknown && <p className="text-fg-subtle">{t('transfer.installedUnknown')}</p>}
        </div>
    );
}

// TransferDialog: choose the subscription (unless a request names it) and
// the new owner, review, then confirm. With a request it approves it.
export function TransferDialog({ owner, request, onClose }: {
    owner: { id: number; username: string };
    request?: TransferRow;
    onClose: () => void;
}) {
    const { t } = useI18n();
    const subs = useRemote(request ? null : SUBSCRIPTIONS_URL, decodeSubscriptions);
    const owned = subs.remote.state === 'known' ? subs.remote.value.filter((s) => s.owner === owner.username) : [];
    const [chosenSub, setChosenSub] = useState(0);
    const subID = request ? request.subscription_id : chosenSub || owned[0]?.id || 0;
    const receivers = useRemote(subID > 0 ? transferURL(subID, '/receivers') : null, decodeReceivers);
    const [toID, setToID] = useState(0);
    const [answer, setAnswer] = useState<TransferPreviewAnswer | null>(null);
    const [refusal, setRefusal] = useState<ApiError | null>(null);
    const [busy, setBusy] = useState(false);
    const [unknown, setUnknown] = useState(false);
    // One request id per reviewed transfer, kept until its answer arrives.
    const pending = useRef<{ key: string; id: string } | null>(null);

    // A request's receiver, preselected when the customer named one.
    useEffect(() => {
        if (!request || toID !== 0 || receivers.remote.state !== 'known') return;
        const named = receivers.remote.value.find((r) => r.username === request.requested_to);
        if (named) setToID(named.id);
    }, [request, receivers.remote, toID]);

    const reset = () => {
        setAnswer(null);
        setRefusal(null);
        setUnknown(false);
    };

    const review = async () => {
        if (subID <= 0 || toID <= 0) return;
        setBusy(true);
        reset();
        try {
            const url = request ? transferRequestActionURL(request.id, 'preview') : transferURL(subID, '/preview');
            const res = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ to_user_id: toID }) });
            if (!res.ok) {
                setRefusal(await readApiError(res));
                return;
            }
            setAnswer(decodePreviewAnswer(await res.json()));
        } catch {
            setRefusal({ message: t('transfer.previewUnknown') });
        } finally {
            setBusy(false);
        }
    };

    const confirm = async () => {
        if (!answer?.canTransfer) return;
        const preview = answer.preview;
        const key = `${subID}:${preview.from.id}:${preview.to.id}`;
        if (pending.current?.key !== key) pending.current = { key, id: newRequestId() };
        setBusy(true);
        setRefusal(null);
        setUnknown(false);
        try {
            const url = request ? transferRequestActionURL(request.id, 'approve') : transferURL(subID);
            const res = await fetch(url, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ to_user_id: preview.to.id, from_user_id: preview.from.id, request_id: pending.current.id, confirm: true }),
            });
            if (res.ok) {
                pending.current = null;
                showToast('success', t('transfer.done', { name: preview.subscription_name, owner: preview.to.username }));
                refreshAfterTransfer();
                onClose();
                return;
            }
            pending.current = null;
            const refused = await readTransferRefusal(res);
            if (refused.preview) setAnswer({ preview: refused.preview, canTransfer: false, refusal: null });
            setRefusal(refused.error);
        } catch {
            // Not known whether it happened: the same id is sent again.
            setUnknown(true);
            refreshAfterTransfer();
        } finally {
            setBusy(false);
        }
    };

    const blocked = answer && !answer.canTransfer && answer.refusal;
    return (
        <Dialog
            id="subscription-transfer"
            icon={ArrowRightLeft}
            width="lg"
            title={t('transfer.title')}
            description={t('transfer.desc')}
            busy={busy}
            onDismiss={onClose}
            actions={
                <>
                    <Button type="button" onClick={onClose} disabled={busy}>{t('common.cancel')}</Button>
                    {answer?.canTransfer ? (
                        <Button type="button" variant="primary" icon={ArrowRightLeft} loading={busy} onClick={() => void confirm()}>
                            {t('transfer.confirm')}
                        </Button>
                    ) : (
                        <Button type="button" variant="primary" loading={busy} disabled={subID <= 0 || toID <= 0} onClick={() => void review()}>
                            {t('transfer.review')}
                        </Button>
                    )}
                </>
            }
        >
            <div className="space-y-4">
                {unknown && <p className="rounded-md bg-warning/10 p-3 text-sm text-fg" role="status">{t('transfer.resultUnknown')}</p>}
                {blocked && (
                    <div className="rounded-md border border-danger/30 bg-danger/10 p-3 text-sm text-danger" data-transfer-blocked={blocked.code}>
                        <p className="font-semibold">{t('transfer.blocked')}</p>
                        <p className="mt-1">{t(`err.${blocked.code}` as TranslationKey)}</p>
                    </div>
                )}
                <ErrorBanner error={refusal} sentence="transfer.previewUnknown" />
                {request ? (
                    <p className="text-sm text-fg">{t('transfer.requests.line', { customer: request.from.username, name: request.subscription_name })}</p>
                ) : (
                    <RemoteGate remote={subs.remote} checking={t('transfer.subscriptionsChecking')} failed={t('transfer.subscriptionsUnknown')} onRetry={() => void subs.retry()} busy={subs.reading}>
                        {() => (owned.length === 0 ? (
                            <p className="text-sm text-fg-muted">{t('transfer.noSubscription', { owner: owner.username })}</p>
                        ) : (
                            <label className="block">
                                <span className="mb-1 block text-xs text-fg-muted">{t('transfer.subscription')}</span>
                                <select className={inputClass} value={subID} onChange={(e) => { setChosenSub(Number(e.target.value)); setToID(0); reset(); }}>
                                    {owned.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
                                </select>
                            </label>
                        ))}
                    </RemoteGate>
                )}
                {request?.requested_to && <p className="text-sm text-fg-muted">{t('transfer.requestedTo', { name: request.requested_to })}</p>}
                {subID > 0 && (
                    <RemoteGate remote={receivers.remote} checking={t('transfer.receiversChecking')} failed={t('transfer.receiversUnknown')} onRetry={() => void receivers.retry()} busy={receivers.reading}>
                        {(shown) => (shown.value.length === 0 ? (
                            <p className="text-sm text-fg-muted">{t('transfer.receiversNone')}</p>
                        ) : (
                            <label className="block">
                                <span className="mb-1 block text-xs text-fg-muted">{t('transfer.receiver')}</span>
                                <select className={inputClass} value={toID} onChange={(e) => { setToID(Number(e.target.value)); reset(); }} data-transfer-receiver="">
                                    <option value={0}>{t('transfer.receiverChoose')}</option>
                                    {shown.value.map((r) => (
                                        <option key={r.id} value={r.id}>{r.reseller ? `${r.username} (${r.reseller})` : r.username}</option>
                                    ))}
                                </select>
                            </label>
                        ))}
                    </RemoteGate>
                )}
                {answer && <TransferPreviewView preview={answer.preview} />}
            </div>
        </Dialog>
    );
}

// TransferRowAction: the Transfer button on an account's row.
export function TransferRowAction({ user, disabled }: { user: { id: number; username: string; role: string }; disabled?: boolean }) {
    const { t } = useI18n();
    const [open, setOpen] = useState(false);
    if (user.role !== 'customer' && user.role !== 'reseller') return null;
    return (
        <>
            <button
                type="button"
                disabled={disabled}
                title={t('transfer.action')}
                aria-label={t('transfer.action')}
                onClick={() => setOpen(true)}
                className="rounded-md p-1.5 text-fg-muted transition-colors hover:bg-surface-2 hover:text-fg disabled:pointer-events-none disabled:opacity-60 max-sm:min-h-11 max-sm:min-w-11"
            >
                <ArrowRightLeft className="h-4 w-4" />
            </button>
            {open && <TransferDialog owner={user} onClose={() => setOpen(false)} />}
        </>
    );
}

// What the signed-in account has not seen yet: a subscription it received,
// or the decision on its own request.
function OutcomeNotices({ rows }: { rows: TransferRow[] }) {
    const { t } = useI18n();
    const [hidden, setHidden] = useState<number[]>([]);
    const seen = async (id: number) => {
        setHidden((list) => [...list, id]);
        try {
            await fetch(transferRequestActionURL(id, 'seen'), { method: 'POST' });
        } finally {
            invalidate('transfers');
        }
    };
    const shown = rows.filter((row) => !hidden.includes(row.id) && (
        (row.status === 'completed' && row.to_you && !row.receiver_seen) ||
        (row.from_you && row.is_request && !row.to_you && !row.requester_seen && (row.status === 'rejected' || row.status === 'completed'))
    ));
    if (shown.length === 0) return null;
    return (
        <ul className="space-y-2">
            {shown.map((row) => (
                <li key={row.id} className="flex flex-wrap items-center gap-3 rounded-md bg-primary/10 p-3 text-sm text-fg" data-transfer-notice={row.id}>
                    <span className="min-w-0 flex-1">
                        {row.to_you
                            ? t('transfer.received', { name: row.subscription_name, from: row.from.username })
                            : row.status === 'completed'
                                ? t('transfer.decided.completed', { name: row.subscription_name, to: row.to?.username ?? '' })
                                : t('transfer.decided.rejected', { name: row.subscription_name })}
                        {row.status === 'rejected' && row.decision_reason && <span className="block text-fg-muted">{t('transfer.decided.reason', { reason: row.decision_reason })}</span>}
                    </span>
                    <Button type="button" icon={Check} onClick={() => void seen(row.id)}>{t('transfer.seen')}</Button>
                </li>
            ))}
        </ul>
    );
}

// TransferRequestsList: the requests the administrator or a reseller
// decides, beside the accounts.
export function TransferRequestsList() {
    const { t } = useI18n();
    const read = useRemote(TRANSFER_REQUESTS_URL, decodeTransferList);
    const [approving, setApproving] = useState<TransferRow | null>(null);
    const [rejecting, setRejecting] = useState<TransferRow | null>(null);
    const [reason, setReason] = useState('');
    const [busy, setBusy] = useState(false);
    const [refusal, setRefusal] = useState<ApiError | null>(null);
    const rejectID = useRef<{ row: number; id: string } | null>(null);

    const reject = async () => {
        if (!rejecting) return;
        if (rejectID.current?.row !== rejecting.id) rejectID.current = { row: rejecting.id, id: newRequestId() };
        setBusy(true);
        setRefusal(null);
        try {
            const res = await fetch(transferRequestActionURL(rejecting.id, 'reject'), {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ request_id: rejectID.current.id, reason: reason.trim() }),
            });
            if (!res.ok) {
                setRefusal(await readApiError(res));
                return;
            }
            rejectID.current = null;
            showToast('success', t('transfer.requests.rejected'));
            setRejecting(null);
            setReason('');
        } catch {
            showToast('error', t('common.resultUnknown'));
        } finally {
            setBusy(false);
            invalidate('transfers');
        }
    };

    if (read.remote.state === 'known' && read.remote.value.rows.length === 0) return null;
    return (
        <section className="mt-6 rounded-xl border border-border bg-surface p-5" data-transfer-requests="">
            <h2 className="text-base font-semibold text-fg">{t('transfer.requests.title')}</h2>
            <p className="mt-1 text-sm text-fg-muted">{t('transfer.requests.desc')}</p>
            <div className="mt-3">
                <RemoteGate remote={read.remote} checking={t('transfer.requests.checking')} failed={t('transfer.requests.unknown')} onRetry={() => void read.retry()} busy={read.reading}>
                    {(shown) => (
                        <div className="space-y-3">
                            <OutcomeNotices rows={shown.value.rows} />
                            <ul className="space-y-2">
                                {shown.value.rows.filter((row) => row.status === 'requested').map((row) => {
                                    const mine = row.can_decide === true;
                                    return (
                                        <li key={row.id} className="rounded-md border border-border p-3 text-sm" data-transfer-request={row.id}>
                                            <p className="font-medium text-fg">{t('transfer.requests.line', { customer: row.from.username, name: row.subscription_name })}</p>
                                            {row.requested_to && <p className="text-fg-muted">{t('transfer.requests.to', { name: row.requested_to })}</p>}
                                            {row.note && <p className="text-fg-muted">{t('transfer.requests.note', { note: row.note })}</p>}
                                            {mine ? (
                                                <div className="mt-2 flex flex-wrap gap-2">
                                                    <Button type="button" variant="primary" icon={ArrowRightLeft} onClick={() => setApproving(row)}>{t('transfer.requests.approve')}</Button>
                                                    <Button type="button" icon={X} onClick={() => { setRejecting(row); setReason(''); setRefusal(null); }}>{t('transfer.requests.reject')}</Button>
                                                </div>
                                            ) : (
                                                <p className="mt-1 text-fg-subtle">{t('transfer.requests.adminDecides')}</p>
                                            )}
                                        </li>
                                    );
                                })}
                            </ul>
                        </div>
                    )}
                </RemoteGate>
            </div>
            {approving && (
                <TransferDialog owner={approving.from} request={approving} onClose={() => { setApproving(null); invalidate('transfers'); }} />
            )}
            {rejecting && (
                <Dialog
                    id="transfer-reject"
                    icon={X}
                    tone="danger"
                    width="sm"
                    title={t('transfer.requests.rejectConfirm')}
                    description={t('transfer.requests.line', { customer: rejecting.from.username, name: rejecting.subscription_name })}
                    busy={busy}
                    onDismiss={() => setRejecting(null)}
                    actions={
                        <>
                            <Button type="button" onClick={() => setRejecting(null)} disabled={busy}>{t('common.cancel')}</Button>
                            <Button type="button" variant="danger" icon={X} loading={busy} onClick={() => void reject()}>{t('transfer.requests.rejectConfirm')}</Button>
                        </>
                    }
                >
                    <div className="space-y-3">
                        <ErrorBanner error={refusal} sentence="transfer.requests.unknown" />
                        <label className="block">
                            <span className="mb-1 block text-xs text-fg-muted">{t('transfer.requests.rejectReason')}</span>
                            <input className={inputClass} value={reason} maxLength={500} onChange={(e) => setReason(e.target.value)} />
                        </label>
                    </div>
                </Dialog>
            )}
        </section>
    );
}

// TransferRequestCard: a customer asks for one of its subscriptions to be
// transferred, sees the open request and the decision.
export function TransferRequestCard() {
    const { t } = useI18n();
    const { role, user } = useAuth();
    const plan = useRemote(role === 'customer' ? MY_SUBSCRIPTION_URL : null, decodeMyPlan);
    const read = useRemote(role === 'customer' ? TRANSFER_REQUESTS_URL : null, decodeTransferList);
    const [askFor, setAskFor] = useState(0);
    const [to, setTo] = useState('');
    const [note, setNote] = useState('');
    const [busy, setBusy] = useState(false);
    const [refusal, setRefusal] = useState<ApiError | null>(null);
    const [unknown, setUnknown] = useState(false);
    const pending = useRef<{ key: string; id: string } | null>(null);
    if (role !== 'customer' || user?.account_type !== 'account') return null;
    const rows = read.remote.state === 'known' ? read.remote.value.rows : [];
    const open = (subID: number) => rows.find((row) => row.subscription_id === subID && row.status === 'requested' && row.from_you);

    const send = async () => {
        const key = `${askFor}:${to.trim()}:${note.trim()}`;
        if (pending.current?.key !== key) pending.current = { key, id: newRequestId() };
        setBusy(true);
        setRefusal(null);
        setUnknown(false);
        try {
            const res = await fetch(transferRequestURL(askFor), {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ request_id: pending.current.id, to_username: to.trim(), note: note.trim() }),
            });
            if (!res.ok) {
                pending.current = null;
                setRefusal(await readApiError(res));
                return;
            }
            pending.current = null;
            showToast('success', t('transfer.ask.sent'));
            setAskFor(0);
            setTo('');
            setNote('');
        } catch {
            setUnknown(true);
        } finally {
            setBusy(false);
            invalidate('transfers');
        }
    };

    const withdraw = async (row: TransferRow) => {
        try {
            const res = await fetch(transferRequestActionURL(row.id, 'cancel'), { method: 'POST' });
            if (res.ok) showToast('success', t('transfer.ask.withdrawn'));
            else setRefusal(await readApiError(res));
        } catch {
            showToast('error', t('common.resultUnknown'));
        } finally {
            invalidate('transfers');
        }
    };

    return (
        <section className="rounded-xl border border-border bg-surface p-5" data-transfer-ask="">
            <div className="mb-2 flex items-center gap-2">
                <ArrowRightLeft className="h-4 w-4 text-primary" aria-hidden="true" />
                <h2 className="text-base font-semibold text-fg">{t('transfer.ask.title')}</h2>
            </div>
            <p className="text-sm text-fg-muted">{t('transfer.ask.desc')}</p>
            <div className="mt-3 space-y-3">
                <RemoteGate remote={read.remote} checking={t('transfer.requests.checking')} failed={t('transfer.ask.unknown')} onRetry={() => void read.retry()} busy={read.reading}>
                    {(shown) => <OutcomeNotices rows={shown.value.rows} />}
                </RemoteGate>
                {unknown && <p className="rounded-md bg-warning/10 p-3 text-sm text-fg" role="status">{t('transfer.ask.resultUnknown')}</p>}
                <ErrorBanner error={refusal} sentence="transfer.ask.unknown" />
                <RemoteGate remote={plan.remote} checking={t('transfer.subscriptionsChecking')} failed={t('transfer.subscriptionsUnknown')} onRetry={() => void plan.retry()} busy={plan.reading}>
                    {(shown) => (
                        <ul className="space-y-2">
                            {shown.value.subscriptions.map((sub) => {
                                const waiting = open(sub.id);
                                return (
                                    <li key={sub.id} className="rounded-md border border-border p-3 text-sm" data-transfer-ask-subscription={sub.id}>
                                        <p className="font-medium text-fg">{sub.name}</p>
                                        {waiting ? (
                                            <div className="mt-1 flex flex-wrap items-center gap-3">
                                                <span className="min-w-0 flex-1 text-fg-muted">{t('transfer.ask.waiting', { date: waiting.created_at })}</span>
                                                <Button type="button" icon={X} onClick={() => void withdraw(waiting)}>{t('transfer.ask.withdraw')}</Button>
                                            </div>
                                        ) : askFor === sub.id ? (
                                            <div className="mt-2 grid gap-2 sm:grid-cols-2">
                                                <label>
                                                    <span className="mb-1 block text-xs text-fg-muted">{t('transfer.ask.to')}</span>
                                                    <input className={inputClass} value={to} maxLength={100} onChange={(e) => setTo(e.target.value)} />
                                                </label>
                                                <label>
                                                    <span className="mb-1 block text-xs text-fg-muted">{t('transfer.ask.note')}</span>
                                                    <input className={inputClass} value={note} maxLength={500} onChange={(e) => setNote(e.target.value)} />
                                                </label>
                                                <div className="flex gap-2 sm:col-span-2 sm:justify-end">
                                                    <Button type="button" onClick={() => setAskFor(0)} disabled={busy}>{t('common.cancel')}</Button>
                                                    <Button type="button" variant="primary" icon={Send} loading={busy} disabled={read.remote.state !== 'known'} onClick={() => void send()}>{t('transfer.ask.send')}</Button>
                                                </div>
                                            </div>
                                        ) : (
                                            <div className="mt-2">
                                                <Button type="button" icon={ArrowRightLeft} disabled={read.remote.state !== 'known'} onClick={() => { setAskFor(sub.id); setRefusal(null); setUnknown(false); }}>{t('transfer.ask.open')}</Button>
                                            </div>
                                        )}
                                    </li>
                                );
                            })}
                        </ul>
                    )}
                </RemoteGate>
            </div>
        </section>
    );
}
