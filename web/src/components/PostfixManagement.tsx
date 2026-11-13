import { useEffect, useMemo, useState } from 'react';
import { Mail, Activity, Trash2, RefreshCw, RotateCw, AlertTriangle } from 'lucide-react';
import { ServiceShell } from './ServiceShell';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import { Button, ErrorBanner, RemoteGate, StatusDot, inputClass } from './ui';
import type { TranslationKey } from '../i18n/en';
import { apiErrorText, readApiError, type ApiError } from '../lib/apiError';
import { decodeList, useRemote } from '../lib/remote';
import { CurrentGate, StaleNotice, isStaleWrite, readCurrent, type Current } from './CurrentSettings';
import { PasswordlessMailNotice } from './PasswordlessMailNotice';
import { OutgoingMailSettings } from './OutgoingMailSettings';
import { MailRelaySettings } from './MailRelaySettings';
import { MailServerOptions } from './MailServerOptions';
import { MailPlanLimits } from './MailPlanLimits';
import { confirmAction } from '../lib/confirmAction';
import { answerWasLost } from '../lib/requestIdentity';
import { useDraft } from './kit/save/draft';
import { ChangedMark, SaveBar, ServerMovedNote, changedRowClass } from './kit/save/SaveBar';
import { useSaveWords } from './kit/save/words';

interface PostfixManagementProps {
    onBack: () => void;
}

interface PostfixQueueItem {
    id: string;
    size: string;
    sender: string;
    arrival: string;
    status: string;
}

const decodeQueue = (raw: unknown) => decodeList<PostfixQueueItem>(raw);

// The causes of an unreadable queue the server verifies itself. Any other
// answer gets the sentence that names no cause.
// Sunucunun kendisinin doğruladığı okunamayan-kuyruk nedenleri.
const queueCauses: Record<string, TranslationKey> = {
    postfix_config: 'postfix.queue.unreadable.postfix_config',
};

// What a save that was answered 200 came to, by the server's `applied`.
// 200 ile yanıtlanan kaydın sonucu, sunucunun `applied` değerine göre.
const savedSentences: Record<string, TranslationKey> = {
    not_running: 'mailpolicy.saved.notRunning',
    unchanged: 'mailpolicy.saved.unchanged',
    unchanged_reloaded: 'mailpolicy.saved.unchangedReloaded',
};

// The policy a failed save carries beside its error, or undefined. A body that
// is not JSON carries none; the caller then reads the policy again.
// Başarısız bir kaydın hatasının yanında taşıdığı politika; yoksa undefined.
function writtenPolicy(raw: string): MailPolicy | undefined {
    try {
        const body: unknown = JSON.parse(raw);
        const policy = body && typeof body === 'object' ? (body as { policy?: MailPolicy }).policy : undefined;
        return policy && typeof policy.version === 'string' && policy.version ? policy : undefined;
    } catch {
        return undefined;
    }
}

// The mail queue of this server.
//
// The queue is read from Postfix and is one of three things (9 Oct 2026): being
// read, could not be read, or known. "The mail queue is empty" and the counts
// are said only for a queue the server answered; a read that failed used to
// say "empty" too. Flush and delete act on the queue that is shown, so they
// are off until it is known, and their answer is read instead of assumed.
//
// Bu sunucunun posta kuyruğu. Kuyruk Postfix'ten okunur ve üç şeyden biridir:
// okunuyor, okunamadı ya da biliniyor. "Posta kuyruğu boş" ve sayılar yalnız
// sunucunun yanıtladığı bir kuyruk için söylenir.
export function PostfixManagement({ onBack }: PostfixManagementProps) {
    const { t } = useI18n();
    const [activeTab, setActiveTab] = useState<'queue' | 'logs'>('queue');
    const queue = useRemote('/api/v1/postfix/queue', decodeQueue);
    const [acting, setActing] = useState(false);
    const known = queue.remote.state === 'known' ? queue.remote.value : null;

    const queueAction = async (action: string, id?: string) => {
        if (!known) return;
        const msg =
            action === 'flush' ? t('postfix.confirmFlush') : action === 'delete_all' ? t('postfix.confirmDeleteAll') : t('postfix.confirmDelete', { id: id ?? '' });
        await confirmAction({ message: msg, tone: 'danger',
            action: async () => {
                setActing(true);
                try {
                    const res = await fetch('/api/v1/postfix/queue', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ action, id }),
                    });
                    // The action is done only when the server says so.
                    // Eylem, ancak sunucu söylediğinde yapılmıştır.
                    if (!res.ok) showToast('error', apiErrorText(await readApiError(res), t, 'postfix.actionFailed'));
                    else showToast('success', t('postfix.done'));
                } catch {
                    showToast('error', t('postfix.actionFailed'));
                } finally {
                    setActing(false);
                    void queue.retry();
                }
            },
        });
    };

    const count = (status: string) => (known ?? []).filter((item) => item.status === status).length;

    // Why the queue could not be read (10 Oct 2026): the sentence for a cause
    // the server verified, else one that names no cause, and the line Postfix
    // itself printed. It used to say "check that Postfix is running", which a
    // stopped Postfix does not cause and was not the cause when measured.
    // Kuyruğun neden okunamadığı: sunucunun doğruladığı nedenin cümlesi, yoksa
    // neden adlandırmayan cümle ve Postfix'in kendi yazdığı satır.
    const queueUnreadable = () => {
        const error = queue.remote.state === 'unknown' ? queue.remote.reason : null;
        if (error?.code !== 'MAIL_QUEUE_UNREADABLE') return t('postfix.queue.unreadable');
        const sentence = t(queueCauses[error.reason ?? ''] ?? 'postfix.queue.unreadable');
        const detail = error.vars?.detail;
        return detail ? `${sentence} ${t('postfix.queue.said', { detail })}` : sentence;
    };

    return (
        <ServiceShell serviceId="postfix" name="Postfix" icon={Mail} onBack={onBack}>
            <PasswordlessMailNotice />
            <div className="mb-4 flex items-center gap-1 border-b border-border">
                <Tab active={activeTab === 'queue'} onClick={() => setActiveTab('queue')} icon={Mail} label={t('postfix.tab.queue')} />
                <Tab active={activeTab === 'logs'} onClick={() => setActiveTab('logs')} icon={Activity} label={t('postfix.tab.logs')} />
            </div>

            {activeTab === 'logs' ? (
                <div className="rounded-xl border border-border bg-surface p-10 text-center text-fg-muted">
                    <Activity className="mx-auto mb-3 h-10 w-10 opacity-40" aria-hidden="true" />
                    <p>{t('postfix.logsSoon')}</p>
                </div>
            ) : (
                <div aria-busy={queue.reading || acting}>
                    {/* The counts are the queue's own: they exist once it has
                        been read, in the place they keep.
                        Sayılar kuyruğun kendisinindir; okunduğunda vardır. */}
                    {known && (
                        <div className="mb-4 grid grid-cols-2 gap-3 lg:grid-cols-4">
                            <Stat labelKey="postfix.active" value={count('active')} />
                            <Stat labelKey="postfix.deferred" value={count('deferred')} accent={count('deferred') > 0} />
                            <Stat labelKey="postfix.hold" value={count('hold')} />
                            <Stat labelKey="postfix.corrupt" value={count('corrupt')} danger={count('corrupt') > 0} />
                        </div>
                    )}

                    <div className="mb-3 flex flex-wrap items-center justify-end gap-2">
                        <Button variant="secondary" icon={RefreshCw} loading={queue.reading && !!known} onClick={() => void queue.retry()}>
                            {t('postfix.refresh')}
                        </Button>
                        <Button variant="secondary" icon={RotateCw} onClick={() => queueAction('flush')} disabled={!known || acting}>
                            {t('postfix.flush')}
                        </Button>
                        <Button variant="danger" icon={Trash2} onClick={() => queueAction('delete_all')} disabled={!known || known.length === 0 || acting}>
                            {t('postfix.deleteAll')}
                        </Button>
                    </div>

                    <RemoteGate
                        remote={queue.remote}
                        checking={t('postfix.queue.checking')}
                        failed={queueUnreadable()}
                        onRetry={() => void queue.retry()}
                        busy={queue.reading}
                        className="min-h-[2.75rem]"
                    >
                        {({ value: items, stale }) => (items.length === 0 ? (
                            <div className="rounded-xl border border-border bg-surface p-12 text-center">
                                <Mail className="mx-auto mb-3 h-10 w-10 text-fg-subtle" aria-hidden="true" />
                                <p className="text-fg-muted">{t('postfix.empty')}</p>
                            </div>
                        ) : (
                            <div className="overflow-x-auto rounded-xl border border-border-strong bg-surface">
                                <table className="w-full text-sm">
                                    <thead>
                                        <tr className="border-b border-border text-left text-xs font-semibold text-fg-muted">
                                            <th className="px-4 py-2.5">ID</th>
                                            <th className="px-4 py-2.5">{t('postfix.col.sender')}</th>
                                            <th className="px-4 py-2.5">{t('postfix.col.size')}</th>
                                            <th className="px-4 py-2.5">{t('postfix.col.arrival')}</th>
                                            <th className="px-4 py-2.5">{t('domains.col.status')}</th>
                                            <th className="px-4 py-2.5" />
                                        </tr>
                                    </thead>
                                    <tbody>
                                        {items.map((item) => (
                                            <tr key={item.id} className="border-b border-border last:border-0 hover:bg-surface-2/60">
                                                <td className="px-4 py-2.5 font-mono text-fg">{item.id}</td>
                                                <td className="break-all px-4 py-2.5 text-fg-muted">{item.sender}</td>
                                                <td className="whitespace-nowrap px-4 py-2.5 text-fg-muted">{item.size}</td>
                                                <td className="whitespace-nowrap px-4 py-2.5 text-fg-muted">{item.arrival}</td>
                                                <td className="px-4 py-2.5">
                                                    <span className="inline-flex items-center gap-1.5 text-fg-muted">
                                                        <StatusDot ok={item.status === 'active'} />
                                                        {item.status}
                                                    </span>
                                                </td>
                                                <td className="px-4 py-2.5 text-right">
                                                    <button
                                                        onClick={() => queueAction('delete_id', item.id)}
                                                        disabled={stale || acting}
                                                        aria-label={t('postfix.deleteMessage', { id: item.id })}
                                                        title={t('postfix.deleteMessage', { id: item.id })}
                                                        className="rounded-md p-1.5 text-fg-subtle transition-colors hover:bg-surface-2 hover:text-danger disabled:pointer-events-none disabled:opacity-50"
                                                    >
                                                        <Trash2 className="h-4 w-4" aria-hidden="true" />
                                                    </button>
                                                </td>
                                            </tr>
                                        ))}
                                    </tbody>
                                </table>
                            </div>
                        ))}
                    </RemoteGate>
                </div>
            )}
            <OutgoingMailSettings />
            <MailRelaySettings />
            <MailServerOptions />
            <MailPlanLimits />
            <MailPolicySection />
        </ServiceShell>
    );
}

function Tab({ active, onClick, icon: Icon, label }: { active: boolean; onClick: () => void; icon: typeof Mail; label: string }) {
    return (
        <button
            onClick={onClick}
            className={`-mb-px flex items-center gap-2 border-b-2 px-3 py-2.5 text-sm font-medium transition-colors ${
                active ? 'border-primary text-primary' : 'border-transparent text-fg-muted hover:text-fg'
            }`}
        >
            <Icon className="h-4 w-4" />
            {label}
        </button>
    );
}

function Stat({ labelKey, value, accent, danger }: { labelKey: TranslationKey; value: number; accent?: boolean; danger?: boolean }) {
    const { t } = useI18n();
    const color = danger ? 'text-danger' : accent ? 'text-warning' : 'text-fg';
    return (
        <div className="rounded-xl border border-border bg-surface p-4">
            <p className="text-xs font-medium uppercase tracking-wide text-fg-subtle">{t(labelKey)}</p>
            <p className={`mt-1 text-2xl font-bold ${color}`}>{value}</p>
        </div>
    );
}


// Server-wide mail policy (Plesk "server-wide mail settings" core): the
// message size limit and DNSBL protection for incoming mail. Admin-only.
// Sunucu geneli posta politikası: mesaj boyutu sınırı ve gelen posta için
// DNSBL koruması. Yalnız yönetici.
interface MailPolicy {
    message_size_mb: number;
    dnsbl_zones: string[] | null;
    outbound_rate_limit: number;
    /** alpha.128 (G30): connections one sending server may hold at once; absent from an older Agent. */
    connection_limit?: number;
    // The Postfix values this policy was read from; every save carries it back.
    // Bu politikanın okunduğu Postfix değerleri; her kayıt onu geri taşır.
    version: string;
    // Set when CelikPanel will not rewrite the recipient restrictions it found.
    // CelikPanel bulduğu alıcı kısıtlarını yeniden yazmayacaksa dolar.
    dnsbl_locked?: string;
}

// The form's values, read from the server's policy. The rate and the zones
// keep a starting value while their switch is off.
// Formun değerleri, sunucunun politikasından okunur.
interface PolicyForm {
    sizeMB: number;
    dnsblOn: boolean;
    zones: string;
    // Outbound rate limit: messages per minute per sending client. 0 = off.
    // Guards against becoming a spam source (a compromised customer account).
    // Giden hız sınırı: gönderen istemci başına dakikadaki mesaj. 0 = kapalı.
    rateOn: boolean;
    rate: number;
    connections: number | null;
}

function formOf(policy: MailPolicy): PolicyForm {
    const z = policy.dnsbl_zones || [];
    return {
        sizeMB: policy.message_size_mb,
        dnsblOn: z.length > 0,
        zones: z.length > 0 ? z.join(', ') : 'zen.spamhaus.org',
        rateOn: policy.outbound_rate_limit > 0,
        rate: policy.outbound_rate_limit > 0 ? policy.outbound_rate_limit : 30,
        connections: typeof policy.connection_limit === 'number' ? policy.connection_limit : null,
    };
}

function MailPolicySection() {
    const { t } = useI18n();
    const say = useSaveWords();
    // The form holds nothing until the server's policy has been read: a form
    // filled with defaults was one Save away from replacing the real one.
    // Sunucunun politikası okunana kadar form hiçbir şey tutmaz.
    const [current, setCurrent] = useState<Current<MailPolicy>>({ state: 'loading' });
    const [stale, setStale] = useState(false);
    // The policy was written to main.cf and Postfix was not seen to take it: a
    // verified failure after a change (9 Oct 2026), or, when the server could
    // not establish what Postfix did, an unknown outcome (10 Oct 2026). It
    // stays on screen above the saved values until a save succeeds or the
    // policy is read again.
    // Politika main.cf'e yazıldı ve Postfix'in onu aldığı görülmedi: bir
    // değişiklik sonrası doğrulanmış hata ya da bilinmeyen sonuç. Ekranda,
    // kaydedilen değerlerin üstünde kalır.
    const [notReloaded, setNotReloaded] = useState<ApiError | null>(null);
    const policy = current.state === 'known' ? current.value : null;
    // alpha.132: the form is a draft over the policy last read; a changed
    // field is marked and only Save writes (owner, 8 Nov 2026).
    const known = useMemo(() => (policy ? formOf(policy) : null), [policy]);
    const draft = useDraft(known);
    const form = draft.values;

    const show = (next: MailPolicy) => {
        setStale(false);
        setCurrent({ state: 'known', value: next });
    };

    const load = async () => {
        setNotReloaded(null);
        setCurrent({ state: 'loading' });
        const next = await readCurrent<MailPolicy>('/api/v1/mail/policy');
        if (next.state === 'known') show(next.value);
        else setCurrent(next);
    };

    useEffect(() => {
        void load();
    }, []);

    const dnsblLocked = policy?.dnsbl_locked || '';

    const save = () => void draft.save(async (_changes, values) => {
        if (!policy) return 'refused';
        let r: Response;
        try {
            r = await fetch('/api/v1/mail/policy', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    message_size_mb: values.sizeMB,
                    dnsbl_zones: dnsblLocked
                        ? policy.dnsbl_zones || []
                        : values.dnsblOn ? values.zones.split(',').map((z) => z.trim()).filter(Boolean) : [],
                    outbound_rate_limit: values.rateOn ? values.rate : 0,
                    // Sent only when the server said its value; otherwise it is left.
                    connection_limit: values.connections ?? undefined,
                    version: policy.version,
                }),
            });
        } catch {
            return 'unknown';
        }
        if (answerWasLost(r)) return 'unknown';
        if (!r.ok) {
            // A stale save keeps what was typed on screen and says how to
            // go on; nothing was written.
            // Eskimiş kayıt, yazılanı ekranda tutar ve nasıl sürüleceğini söyler.
            // The body is read once more beside the error: after a write
            // that Postfix did not take, it also carries the policy that
            // is in main.cf now.
            // Gövde hatanın yanında bir kez daha okunur: Postfix'in
            // almadığı bir yazıdan sonra main.cf'teki politikayı da taşır.
            const raw = await r.text();
            const written = writtenPolicy(raw);
            const error = await readApiError(new Response(raw, { status: r.status }));
            if (isStaleWrite(error)) setStale(true);
            else if (error.code === 'MAIL_POLICY_NOT_RELOADED' || error.code === 'MAIL_POLICY_RELOAD_UNKNOWN') {
                // main.cf holds the new values: show them, with the version
                // the next save carries, and say what is known about
                // Postfix. Only an answer without them is read again.
                // main.cf yeni değerleri tutuyor: göster ve Postfix
                // hakkında bilineni söyle.
                if (written) show(written);
                else await load();
                setNotReloaded(error);
            } else showToast('error', apiErrorText(error, t));
            return 'refused';
        }
        setNotReloaded(null);
        let answer: { policy?: MailPolicy; applied?: string };
        try {
            answer = (await r.json()) as { policy?: MailPolicy; applied?: string };
        } catch {
            // Accepted, but the answer could not be read: read the policy again.
            await load();
            return 'unknown';
        }
        // "Applied" is said only when the server verified the reload. A
        // stopped Postfix was left stopped, and an unchanged policy wrote
        // nothing: each says so.
        // "Uygulandı", yalnız sunucu yeniden yüklemeyi doğruladığında
        // söylenir.
        showToast('success', t(savedSentences[answer.applied ?? ''] ?? 'mailpolicy.saved'));
        if (answer.policy?.version) show(answer.policy);
        else await load();
        return 'saved';
    });

    const lockedCause = () => {
        const key = `mailpolicy.dnsblLocked.${dnsblLocked}` as TranslationKey;
        const text = t(key);
        return text === key ? t('mailpolicy.dnsblLocked.other') : text;
    };

    // A changed field: its row is marked, and a value the server changed
    // meanwhile is said under it.
    const row = (field: keyof PolicyForm) => (draft.isChanged(field) ? `-mx-2 rounded-r px-2 ${changedRowClass}` : '');
    const mark = (field: keyof PolicyForm) => draft.isChanged(field) && <ChangedMark />;
    const moved = (field: keyof PolicyForm, words: (value: PolicyForm) => string) => draft.isMoved(field) && draft.base && (
        <ServerMovedNote value={words(draft.base)} onTake={() => draft.take(field)} />
    );
    const onOff = (on: boolean) => say(on ? 'on' : 'off');

    return (
        <section className="mt-5 rounded-xl border border-border bg-surface p-5" aria-busy={current.state === 'loading' || draft.saving}>
            <h3 className="mb-1 text-sm font-semibold text-fg">{t('mailpolicy.title')}</h3>
            <p className="mb-4 text-sm text-fg-muted">{t('mailpolicy.desc')}</p>
            <CurrentGate state={current.state} unknownKey="mailpolicy.unreadable" onRetry={load} />
            {policy && form && (
                <>
                    {notReloaded && (
                        <div role="alert" className="mb-4" data-policy-outcome={notReloaded.code === 'MAIL_POLICY_RELOAD_UNKNOWN' ? 'unknown' : 'not-reloaded'}>
                            {/* A reload Postfix was verified not to have taken
                                is a failure after a change: the failure
                                surface. An outcome that could not be
                                established is not one, and its own sentence
                                says so; it is drawn on the attention surface,
                                like every other unknown result (10 Oct 2026:
                                both were red).
                                Postfix'in almadığı doğrulanan yeniden yükleme,
                                değişiklik sonrası bir hatadır: hata yüzeyi.
                                Belirlenemeyen sonuç hata değildir; kendi cümlesi
                                de bunu söyler ve dikkat yüzeyinde çizilir. */}
                            {notReloaded.code === 'MAIL_POLICY_RELOAD_UNKNOWN' ? (
                                <div className="flex items-start gap-2 rounded-lg border border-warning-mark/50 bg-warning-mark/20 p-3 text-sm leading-relaxed text-fg">
                                    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
                                    <p className="min-w-0 max-w-[75ch] break-words">{apiErrorText(notReloaded, t)}</p>
                                </div>
                            ) : (
                                <ErrorBanner error={notReloaded} />
                            )}
                            {notReloaded.vars?.detail && (
                                <p className="mt-1.5 max-w-[75ch] break-words text-xs text-fg-muted">
                                    {t(notReloaded.reason === 'check' || notReloaded.reason === 'reload' ? 'mailpolicy.postfixSaid' : 'mailpolicy.observed')}{' '}
                                    <span className="font-mono text-fg">{notReloaded.vars.detail}</span>
                                </p>
                            )}
                        </div>
                    )}
                    {stale && <StaleNotice textKey="mailpolicy.stale" onReload={load} />}
                    <fieldset disabled={draft.saving} className="min-w-0">
                        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                            <div className={row('sizeMB')}>
                                <label className="block text-sm">
                                    <span className="mb-1 block text-xs text-fg-muted">{t('mailpolicy.maxSize')}{mark('sizeMB')}</span>
                                    <input type="number" min={1} max={200} value={form.sizeMB}
                                        onChange={(e) => draft.set('sizeMB', Math.max(1, Math.min(200, parseInt(e.target.value) || 25)))}
                                        className={`${inputClass} field-xs`} />
                                </label>
                                {moved('sizeMB', (value) => String(value.sizeMB))}
                            </div>
                            {!dnsblLocked && (
                                <div className={row('zones')}>
                                    <label className="block text-sm">
                                        <span className="mb-1 block text-xs text-fg-muted">{t('mailpolicy.zones')}{mark('zones')}</span>
                                        <input value={form.zones} onChange={(e) => draft.set('zones', e.target.value)}
                                            disabled={!form.dnsblOn} className={`${inputClass} field-xl`} placeholder="zen.spamhaus.org" />
                                    </label>
                                    {moved('zones', (value) => value.zones)}
                                </div>
                            )}
                        </div>
                        {dnsblLocked ? (
                            // The DNSBL entries on this server are the owner's own
                            // arrangement: say why they are not offered and what
                            // changes them, instead of showing them as "off".
                            // Bu sunucudaki DNSBL girdileri sahibin kendi düzenidir.
                            <div role="note" className="mt-3 max-w-[75ch] space-y-1 break-words rounded-lg border border-border bg-surface-2/50 p-3 text-sm leading-relaxed text-fg-muted">
                                <p className="text-fg">{lockedCause()}</p>
                                <p>{t('mailpolicy.dnsblLockedAction')}</p>
                            </div>
                        ) : (
                            <div className={`mt-3 ${row('dnsblOn')}`}>
                                <label className="flex min-h-[1.75rem] items-center gap-2 text-sm text-fg">
                                    <input type="checkbox" checked={form.dnsblOn} className="h-4 w-4" onChange={(e) => {
                                        draft.set('dnsblOn', e.target.checked);
                                        // Turned off: the zones are not sent, so they are no change either.
                                        if (!e.target.checked && draft.base) draft.set('zones', draft.base.zones);
                                    }} />
                                    {t('mailpolicy.dnsbl')}
                                    {draft.isChanged('dnsblOn') && <ChangedMark switched={form.dnsblOn} />}
                                </label>
                                <p className="mt-1 text-xs text-fg-subtle">{t('mailpolicy.dnsblHint')}</p>
                                {moved('dnsblOn', (value) => onOff(value.dnsblOn))}
                            </div>
                        )}

                        <div className={`mt-4 ${row('rateOn')}`}>
                            <label className="flex min-h-[1.75rem] items-center gap-2 text-sm text-fg">
                                <input type="checkbox" checked={form.rateOn} className="h-4 w-4" onChange={(e) => {
                                    draft.set('rateOn', e.target.checked);
                                    if (!e.target.checked && draft.base) draft.set('rate', draft.base.rate);
                                }} />
                                {t('mailpolicy.rate')}
                                {draft.isChanged('rateOn') && <ChangedMark switched={form.rateOn} />}
                            </label>
                            <p className="mt-1 text-xs text-fg-subtle">{t('mailpolicy.rateHint')}</p>
                            {moved('rateOn', (value) => onOff(value.rateOn))}
                        </div>
                        {form.rateOn && (
                            <div className={`mt-2 max-w-xs ${row('rate')}`}>
                                <label className="block text-sm">
                                    <span className="mb-1 block text-xs text-fg-muted">{t('mailpolicy.rateLabel')}{mark('rate')}</span>
                                    <input type="number" min={1} max={10000} value={form.rate}
                                        onChange={(e) => draft.set('rate', Math.max(1, Math.min(10000, parseInt(e.target.value) || 30)))}
                                        className={`${inputClass} field-xs`} />
                                </label>
                                {moved('rate', (value) => String(value.rate))}
                            </div>
                        )}

                        {form.connections !== null && (
                            <div className={`mt-4 max-w-xs ${row('connections')}`}>
                                <label className="block text-sm">
                                    <span className="mb-1 block text-xs text-fg-muted">{t('mailpolicy.connections')}{mark('connections')}</span>
                                    <input type="number" min={0} max={1000} value={form.connections}
                                        onChange={(e) => draft.set('connections', Math.max(0, Math.min(1000, parseInt(e.target.value) || 0)))}
                                        className={`${inputClass} field-xs`} />
                                    <span className="mt-1 block text-xs text-fg-subtle">{t('mailpolicy.connectionsHint')}</span>
                                </label>
                                {moved('connections', (value) => String(value.connections ?? ''))}
                            </div>
                        )}
                    </fieldset>
                    <SaveBar
                        className="-mx-5 px-5 pb-0"
                        count={draft.count}
                        saving={draft.saving}
                        blockedBy={stale ? say('stale') : undefined}
                        unknown={draft.outcome === 'unknown' ? { held: draft.held, checking: current.state === 'loading', onCheck: () => void load() } : undefined}
                        onSave={save}
                        onDiscard={draft.reset}
                    />
                </>
            )}
        </section>
    );
}
