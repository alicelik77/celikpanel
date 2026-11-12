import { useEffect, useState } from 'react';
import { CalendarClock } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import { apiErrorText, readApiError, type ApiError } from '../lib/apiError';
import { invalidate, readRemote } from '../lib/remote';
import { answerWasLost } from '../lib/requestIdentity';
import { confirmAction } from '../lib/confirmAction';
import { Button, Dialog, inputClass } from './ui';
import { useSaveRun } from './kit/save/saveRun';
import { useLeaveGuard } from './kit/save/leaveGuard';
import { useSaveWords } from './kit/save/words';

// Out-of-office reply for one mailbox (alpha.114). Dovecot sends the reply
// itself (Sieve), so it keeps working when the panel is stopped. The dialog
// reads the stored setting and the server's reading of the mailbox: a mail
// filter script the mailbox owner made is named first and never replaced.
// Posta kutusu için otomatik yanıt. Yanıtı Dovecot gönderir; kutu sahibinin
// kendi filtre betiği önce söylenir ve asla değiştirilmez.

interface Setting {
    enabled: boolean;
    subject: string;
    message: string;
    start_date?: string;
    end_date?: string;
    interval_days: number;
    addresses?: string[];
}

interface ServerState {
    applied: boolean;
    script: 'none' | 'managed' | 'owner' | string;
    sieve_available: boolean;
    refusal?: string;
    detail?: string;
    warning?: string;
}

interface View {
    autoresponder: Setting;
    stored: boolean;
    state?: ServerState;
    state_error?: string;
}

type Load = { state: 'loading' } | { state: 'failed' } | { state: 'known'; view: View };

// The dialog's fields, as the setting fills them.
// Pencerenin alanları, ayarın doldurduğu biçimde.
interface Form {
    enabled: boolean;
    subject: string;
    message: string;
    start: string;
    end: string;
    days: number;
    addresses: string;
}

const addressList = (text: string) => text.split(/[\s,;]+/).filter(Boolean);

function formOf(s: Setting, defaultSubject: string): Form {
    return {
        enabled: Boolean(s.enabled),
        subject: s.subject || defaultSubject,
        message: s.message || '',
        start: s.start_date || '',
        end: s.end_date || '',
        days: s.interval_days || 1,
        addresses: (s.addresses || []).join(', '),
    };
}

// The same setting, whatever spacing separates the addresses.
// Adresler nasıl ayrılırsa ayrılsın aynı ayar.
const sameSetting = (a: Form, b: Form) =>
    JSON.stringify({ ...a, addresses: addressList(a.addresses) }) === JSON.stringify({ ...b, addresses: addressList(b.addresses) });

function decodeView(raw: unknown): View {
    const view = raw as View | null;
    if (!view || typeof view !== 'object' || !view.autoresponder || typeof view.autoresponder !== 'object' || typeof view.stored !== 'boolean') {
        throw new Error('autoresponder answer outside the contract');
    }
    return view;
}

// alpha.132: Save is on only when something changed and the reply can be
// saved; what is missing is named beside it. Discard closes without saving;
// closing any other way with changes asks first, and so does leaving the
// page. A save whose answer was lost is not sent again: Save stays off until
// the reply is read again, which says whether it was saved.
// Kaydet yalnız bir şey değiştiyse ve yanıt kaydedilebilirse açıktır; eksik
// olan yanında yazar. Vazgeç kaydetmeden kapatır; başka yoldan kapatmak ve
// sayfadan çıkmak sorar. Yanıtı yiten kayıt yinelenmez.
export function MailAutoresponderDialog({
    domainId,
    accountId,
    address,
    onClose,
}: {
    domainId: number;
    accountId: number;
    address: string;
    onClose: () => void;
}) {
    const { t } = useI18n();
    const say = useSaveWords();
    const url = `/api/v1/domains/${domainId}/mail/autoresponder?id=${accountId}`;
    const [load, setLoad] = useState<Load>({ state: 'loading' });
    const [attempt, setAttempt] = useState(0);
    // The setting as read, and the form over it.
    // Okunan ayar ve üzerindeki form.
    const [initial, setInitial] = useState<Form | null>(null);
    const [form, setForm] = useState<Form | null>(null);
    const [refusal, setRefusal] = useState<ApiError | null>(null);
    // What a save whose answer was lost sent; Save stays off until a read decides.
    // Yanıtı yiten kaydın gönderdiği; bir okuma karar verene dek Kaydet kapalıdır.
    const [lost, setLost] = useState<Form | null>(null);
    const [checking, setChecking] = useState(false);
    const runner = useSaveRun();
    const saving = runner.saving;

    useEffect(() => {
        const controller = new AbortController();
        setLoad({ state: 'loading' });
        // A refused, failed or malformed answer is unknown, never a default.
        void readRemote(url, decodeView, undefined, { signal: controller.signal })
            .then((read) => {
                if (controller.signal.aborted) return;
                if (read.state !== 'known') {
                    setLoad({ state: 'failed' });
                    return;
                }
                const next = formOf(read.value.autoresponder, t('mail.autoreply.defaultSubject'));
                setInitial(next);
                setForm(next);
                setLoad({ state: 'known', view: read.value });
            });
        return () => controller.abort();
    }, [url, attempt, t]);

    const changed = !!form && !!initial && !sameSetting(form, initial);
    useLeaveGuard(changed ? 'some' : 0);
    const set = <K extends keyof Form>(field: K, value: Form[K]) => setForm((current) => current && { ...current, [field]: value });

    const datesValid = !form || !form.start || !form.end || form.end >= form.start;
    const daysValid = !!form && Number.isInteger(form.days) && form.days >= 1 && form.days <= 30;
    const messageValid = !form || !form.enabled || form.message.trim().length > 0;
    const ownerScript = load.state === 'known' && load.view.state?.script === 'owner';
    // What stops Save although something changed, beside the button.
    // Bir şey değiştiği hâlde Kaydet'i durduran, düğmenin yanında.
    const missing = !messageValid ? t('mail.autoreply.messageRequired')
        : !datesValid ? t('mail.autoreply.datesInvalid')
            : !daysValid ? t('mail.autoreply.intervalInvalid') : '';
    const canSave = load.state === 'known' && !saving && changed && !missing && !ownerScript && !lost;

    const save = () => {
        if (!canSave || !form) return;
        const sent = form;
        void runner.run(async () => {
            setRefusal(null);
            let res: Response;
            try {
                res = await fetch(`/api/v1/domains/${domainId}/mail/autoresponder`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        id: accountId,
                        enabled: sent.enabled,
                        subject: sent.subject,
                        message: sent.message,
                        start_date: sent.start,
                        end_date: sent.end,
                        interval_days: sent.days,
                        addresses: addressList(sent.addresses),
                    }),
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
                const apiError = await readApiError(res);
                if (apiError.code === 'MAIL_AUTORESPONDER_REFUSED') {
                    // Nothing changed on the server; the reason says who acts.
                    setRefusal(apiError);
                } else {
                    showToast('error', apiErrorText(apiError, t, 'mail.autoreply.saveFailed'));
                }
                return 'refused';
            }
            showToast('success', sent.enabled ? t('mail.autoreply.savedOn') : t('mail.autoreply.savedOff'));
            // The server's warning, when the answer carries one.
            // Yanıt taşıyorsa sunucunun uyarısı.
            let warning: unknown;
            try {
                warning = ((await res.json()) as { state?: ServerState }).state?.warning;
            } catch {
                warning = undefined;
            }
            if (typeof warning === 'string' && warning) showToast('info', warning);
            invalidate('mail');
            onClose();
            return 'saved';
        });
    };

    // After a lost answer: the reply read again says whether it was saved.
    // What was typed stays; a reply that is not the one sent becomes what the
    // form is measured against.
    // Yanıt yittikten sonra: yeniden okunan yanıt kaydedilip kaydedilmediğini söyler.
    const check = async () => {
        if (!lost) return;
        setChecking(true);
        const read = await readRemote(url, decodeView);
        setChecking(false);
        if (read.state !== 'known') return;
        const now = formOf(read.value.autoresponder, t('mail.autoreply.defaultSubject'));
        setLost(null);
        runner.clear();
        if (sameSetting(now, lost)) {
            showToast('success', lost.enabled ? t('mail.autoreply.savedOn') : t('mail.autoreply.savedOff'));
            invalidate('mail');
            onClose();
            return;
        }
        setInitial(now);
        setLoad({ state: 'known', view: read.value });
    };

    // Closing with unsaved changes asks; Discard is the answer given in advance.
    // Kaydedilmemiş değişiklikle kapatmak sorar; Vazgeç önceden verilmiş yanıttır.
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

    const state = load.state === 'known' ? load.view.state : undefined;
    const busy = saving || checking;
    return (
        <Dialog
            id="mail-autoreply"
            icon={CalendarClock}
            busy={busy}
            onDismiss={() => void dismiss()}
            noValidate
            width="lg"
            onSubmit={(event) => {
                event.preventDefault();
                save();
            }}
            title={t('mail.autoreply.title')}
            description={
                <>
                    {t('mail.autoreply.account')}: <strong className="break-all font-medium text-fg">{address}</strong>
                </>
            }
            footerLead={lost ? (
                <div className="flex flex-wrap items-center gap-x-3 gap-y-2 text-sm text-fg" aria-live="polite">
                    <p className="min-w-0 max-w-[75ch] flex-1">{say('unknown')}</p>
                    <Button type="button" onClick={() => void check()} loading={checking}>{t('common.checkAgain')}</Button>
                </div>
            ) : load.state === 'known' && !ownerScript ? (
                <p data-form-reason="" className={`text-sm ${changed && missing ? 'text-danger' : 'text-fg-muted'}`} aria-live="polite">
                    {!changed ? t('domain.unsaved.none') : missing || say('pendingSome')}
                </p>
            ) : undefined}
            actions={
                <>
                    <Button type="button" onClick={onClose} disabled={busy} data-form-discard="">
                        {say('discard')}
                    </Button>
                    <Button type="submit" variant="primary" disabled={!canSave} loading={saving} data-form-save="">
                        {saving ? say('saving') : say('save')}
                    </Button>
                </>
            }
        >
            {load.state === 'loading' && <p className="text-sm text-fg-muted">{t('mail.autoreply.loading')}</p>}
            {load.state === 'failed' && (
                <div className="space-y-2 text-sm">
                    <p className="text-danger">{t('mail.autoreply.loadFailed')}</p>
                    <Button type="button" onClick={() => setAttempt((n) => n + 1)}>
                        {t('common.retry')}
                    </Button>
                </div>
            )}
            {load.state === 'known' && form && (
                <div className="space-y-3">
                    {refusal && (
                        <p role="alert" className="rounded-md border border-danger/40 bg-danger/10 px-3 py-2 text-sm text-danger">
                            {apiErrorText(refusal, t, 'mail.autoreply.saveFailed')}
                        </p>
                    )}
                    {ownerScript && (
                        <p role="status" className="rounded-md border border-warning-mark/50 bg-warning-mark/20 px-3 py-2 text-sm text-fg">
                            {t('mail.autoreply.ownerScript')}
                        </p>
                    )}
                    {load.view.state_error && <p className="text-xs text-fg-muted">{t('mail.autoreply.stateUnknown')}</p>}
                    {state && !state.sieve_available && !ownerScript && (
                        <p className="text-xs text-fg-muted">{t('mail.autoreply.installsSieve')}</p>
                    )}
                    <label className="flex items-center gap-2 text-sm">
                        <input type="checkbox" checked={form.enabled} onChange={(e) => set('enabled', e.target.checked)} disabled={busy || ownerScript} />
                        {t('mail.autoreply.enabled')}
                    </label>
                    <label className="block">
                        <span className="mb-1 block text-xs text-fg-muted">{t('mail.autoreply.subject')}</span>
                        <input value={form.subject} maxLength={200} onChange={(e) => set('subject', e.target.value)} disabled={busy} className={inputClass} />
                    </label>
                    <label className="block">
                        <span className="mb-1 block text-xs text-fg-muted">{t('mail.autoreply.message')}</span>
                        <textarea
                            value={form.message}
                            rows={6}
                            maxLength={8000}
                            onChange={(e) => set('message', e.target.value)}
                            disabled={busy}
                            aria-invalid={!messageValid}
                            className={inputClass}
                        />
                    </label>
                    <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                        <label className="block">
                            <span className="mb-1 block text-xs text-fg-muted">{t('mail.autoreply.start')}</span>
                            <input type="date" value={form.start} onChange={(e) => set('start', e.target.value)} disabled={busy} className={inputClass} />
                        </label>
                        <label className="block">
                            <span className="mb-1 block text-xs text-fg-muted">{t('mail.autoreply.end')}</span>
                            <input type="date" value={form.end} onChange={(e) => set('end', e.target.value)} disabled={busy} aria-invalid={!datesValid} className={inputClass} />
                        </label>
                        <label className="block">
                            <span className="mb-1 block text-xs text-fg-muted">{t('mail.autoreply.interval')}</span>
                            <input
                                type="number"
                                min={1}
                                max={30}
                                value={form.days}
                                onChange={(e) => set('days', Number(e.target.value))}
                                disabled={busy}
                                aria-invalid={!daysValid}
                                className={inputClass}
                            />
                        </label>
                    </div>
                    <label className="block">
                        <span className="mb-1 block text-xs text-fg-muted">{t('mail.autoreply.addresses')}</span>
                        <input value={form.addresses} onChange={(e) => set('addresses', e.target.value)} disabled={busy} className={inputClass} />
                    </label>
                    <p className="text-xs text-fg-subtle">{t('mail.autoreply.help')}</p>
                </div>
            )}
        </Dialog>
    );
}

export default MailAutoresponderDialog;
