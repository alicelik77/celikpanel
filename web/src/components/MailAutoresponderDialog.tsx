import { useEffect, useState } from 'react';
import { CalendarClock } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import { apiErrorText, readApiError, type ApiError } from '../lib/apiError';
import { Button, Dialog, inputClass } from './ui';

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
    const [load, setLoad] = useState<Load>({ state: 'loading' });
    const [attempt, setAttempt] = useState(0);
    const [enabled, setEnabled] = useState(false);
    const [subject, setSubject] = useState('');
    const [message, setMessage] = useState('');
    const [start, setStart] = useState('');
    const [end, setEnd] = useState('');
    const [days, setDays] = useState(1);
    const [addresses, setAddresses] = useState('');
    const [saving, setSaving] = useState(false);
    const [refusal, setRefusal] = useState<ApiError | null>(null);

    useEffect(() => {
        const controller = new AbortController();
        setLoad({ state: 'loading' });
        fetch(`/api/v1/domains/${domainId}/mail/autoresponder?id=${accountId}`, { signal: controller.signal })
            .then(async (res) => {
                if (!res.ok) throw new Error(String(res.status));
                const view = (await res.json()) as View;
                const s = view.autoresponder;
                setEnabled(Boolean(s.enabled));
                setSubject(s.subject || t('mail.autoreply.defaultSubject'));
                setMessage(s.message || '');
                setStart(s.start_date || '');
                setEnd(s.end_date || '');
                setDays(s.interval_days || 1);
                setAddresses((s.addresses || []).join(', '));
                setLoad({ state: 'known', view });
            })
            .catch((error) => {
                if (error instanceof DOMException && error.name === 'AbortError') return;
                setLoad({ state: 'failed' });
            });
        return () => controller.abort();
    }, [domainId, accountId, attempt, t]);

    const datesValid = !start || !end || end >= start;
    const daysValid = Number.isInteger(days) && days >= 1 && days <= 30;
    const messageValid = !enabled || message.trim().length > 0;
    const ownerScript = load.state === 'known' && load.view.state?.script === 'owner';
    const canSave = load.state === 'known' && !saving && datesValid && daysValid && messageValid && !ownerScript;

    const save = async () => {
        if (!canSave) return;
        setSaving(true);
        setRefusal(null);
        try {
            const res = await fetch(`/api/v1/domains/${domainId}/mail/autoresponder`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    id: accountId,
                    enabled,
                    subject,
                    message,
                    start_date: start,
                    end_date: end,
                    interval_days: days,
                    addresses: addresses.split(/[\s,;]+/).filter(Boolean),
                }),
            });
            if (!res.ok) {
                const apiError = await readApiError(res);
                if (apiError.code === 'MAIL_AUTORESPONDER_REFUSED') {
                    // Nothing changed on the server; the reason says who acts.
                    setRefusal(apiError);
                    return;
                }
                showToast('error', apiErrorText(apiError, t, 'mail.autoreply.saveFailed'));
                return;
            }
            const body = (await res.json()) as { state?: ServerState };
            showToast('success', enabled ? t('mail.autoreply.savedOn') : t('mail.autoreply.savedOff'));
            if (body.state?.warning) showToast('info', body.state.warning);
            onClose();
        } catch {
            // The result is unknown: reading the dialog again shows the server's state.
            showToast('error', t('mail.autoreply.saveUnknown'));
        } finally {
            setSaving(false);
        }
    };

    const state = load.state === 'known' ? load.view.state : undefined;
    return (
        <Dialog
            id="mail-autoreply"
            icon={CalendarClock}
            busy={saving}
            onDismiss={onClose}
            noValidate
            width="lg"
            onSubmit={(event) => {
                event.preventDefault();
                void save();
            }}
            title={t('mail.autoreply.title')}
            description={
                <>
                    {t('mail.autoreply.account')}: <strong className="break-all font-medium text-fg">{address}</strong>
                </>
            }
            actions={
                <>
                    <Button type="button" onClick={onClose} disabled={saving}>
                        {t('common.cancel')}
                    </Button>
                    <Button type="submit" variant="primary" disabled={!canSave}>
                        {saving ? t('mail.autoreply.saving') : t('mail.autoreply.save')}
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
            {load.state === 'known' && (
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
                        <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} disabled={saving || ownerScript} />
                        {t('mail.autoreply.enabled')}
                    </label>
                    <label className="block">
                        <span className="mb-1 block text-xs text-fg-muted">{t('mail.autoreply.subject')}</span>
                        <input value={subject} maxLength={200} onChange={(e) => setSubject(e.target.value)} disabled={saving} className={inputClass} />
                    </label>
                    <label className="block">
                        <span className="mb-1 block text-xs text-fg-muted">{t('mail.autoreply.message')}</span>
                        <textarea
                            value={message}
                            rows={6}
                            maxLength={8000}
                            onChange={(e) => setMessage(e.target.value)}
                            disabled={saving}
                            aria-invalid={!messageValid}
                            className={inputClass}
                        />
                    </label>
                    <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                        <label className="block">
                            <span className="mb-1 block text-xs text-fg-muted">{t('mail.autoreply.start')}</span>
                            <input type="date" value={start} onChange={(e) => setStart(e.target.value)} disabled={saving} className={inputClass} />
                        </label>
                        <label className="block">
                            <span className="mb-1 block text-xs text-fg-muted">{t('mail.autoreply.end')}</span>
                            <input type="date" value={end} onChange={(e) => setEnd(e.target.value)} disabled={saving} aria-invalid={!datesValid} className={inputClass} />
                        </label>
                        <label className="block">
                            <span className="mb-1 block text-xs text-fg-muted">{t('mail.autoreply.interval')}</span>
                            <input
                                type="number"
                                min={1}
                                max={30}
                                value={days}
                                onChange={(e) => setDays(Number(e.target.value))}
                                disabled={saving}
                                aria-invalid={!daysValid}
                                className={inputClass}
                            />
                        </label>
                    </div>
                    {!datesValid && <p className="text-xs text-danger">{t('mail.autoreply.datesInvalid')}</p>}
                    <label className="block">
                        <span className="mb-1 block text-xs text-fg-muted">{t('mail.autoreply.addresses')}</span>
                        <input value={addresses} onChange={(e) => setAddresses(e.target.value)} disabled={saving} className={inputClass} />
                    </label>
                    <p className="text-xs text-fg-subtle">{t('mail.autoreply.help')}</p>
                </div>
            )}
        </Dialog>
    );
}

export default MailAutoresponderDialog;
