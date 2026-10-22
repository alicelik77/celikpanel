import { useEffect, useRef, useState, type FormEvent } from 'react';
import { Mail } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import { readApiError, type ApiError } from '../lib/apiError';
import { REQUEST_ID_HEADER, answerWasLost, newRequestId } from '../lib/requestIdentity';
import { Button, Dialog, ErrorBanner, Spinner, inputClass } from './ui';

export const OWN_EMAIL_URL = '/api/v1/auth/email';

// How long the dialogue waits for the server's answer before it says that the
// result is not known yet (D-033: a wait has a bound). The server answers in
// well under a second; its own limit for this change is 30 seconds, and the
// identity below keeps a late arrival from changing anything twice.
// Diyaloğun sonucun henüz bilinmediğini söylemeden önce beklediği süre.
export const OWN_EMAIL_WAIT_MS = 20_000;

// The Panel's own word that the result is not known (D-029): it restarted
// while the change ran, or the first arrival is still running.
const OPEN_OUTCOMES = new Set(['REQUEST_OUTCOME_UNKNOWN', 'REQUEST_IN_PROGRESS']);

// The signed-in person changes their own email address (v0.1.0-alpha.99,
// D-037 amendment). The current password is asked, as for the password
// change; each press of Save is one action with one identity (D-029), which a
// lost answer is asked for again under. While it saves, the dialogue says so
// and holds its controls; after OWN_EMAIL_WAIT_MS it says that the result is
// not known, which is not a failure, and the page reads the address again.
// Every refusal is shown in the dialogue in the server's coded words (D-024).
//
// Oturum açan kişi kendi e-posta adresini değiştirir. Mevcut parola sorulur;
// her Kaydet basışı tek kimlikli tek eylemdir. Kaydederken diyalog bunu söyler
// ve denetimlerini tutar; süre dolunca sonucun bilinmediğini söyler (hata
// değildir) ve sayfa adresi yeniden okur. Her ret diyalogda kodlu sözleriyle
// gösterilir.
//
// After an unknown result (no answer, a gateway page, the bound, or the Panel
// saying the first arrival still runs) the next Save of the same address
// carries the SAME identity (security review of the alpha.99 candidate): if
// the first arrival did change the address, the Panel answers from it and
// nothing runs twice. A definite answer (a success or a refusal) ends that
// identity, and so does REQUEST_OUTCOME_UNKNOWN, which the Panel gives for
// that identity for 24 hours; a different address gets a new one.
// Sonucu bilinmeyen gönderiden sonra aynı adresin sonraki Kaydet'i AYNI kimliği
// taşır; kesin bir yanıt o kimliği bitirir.

export function ChangeEmailModal({
    current,
    onClose,
    onChanged,
    onUnknown,
}: {
    /** The address as last read, or null when it is not known. */
    current: string | null;
    onClose: () => void;
    /** The server changed the address (its answer carries no address; read it again). */
    onChanged?: () => void;
    /** No answer arrived in time: the caller reads the address again. */
    onUnknown?: () => void;
}) {
    const { t } = useI18n();
    const [address, setAddress] = useState('');
    const [password, setPassword] = useState('');
    const [saving, setSaving] = useState(false);
    const [refusal, setRefusal] = useState<ApiError | null>(null);
    const [unknown, setUnknown] = useState(false);
    const alive = useRef(true);
    // The identity of a send whose result is not known, and its address.
    const pending = useRef<{ id: string; address: string } | null>(null);
    useEffect(() => () => {
        alive.current = false;
    }, []);

    const ready = address.trim() !== '' && password !== '' && !saving;

    const outcomeUnknown = () => {
        if (!alive.current) return;
        setUnknown(true);
        onUnknown?.();
    };

    const submit = async (event?: FormEvent) => {
        event?.preventDefault();
        if (!ready) return;
        setSaving(true);
        setRefusal(null);
        setUnknown(false);
        const typed = address.trim();
        if (pending.current?.address !== typed) pending.current = { id: newRequestId(), address: typed };
        const id = pending.current.id;
        const controller = new AbortController();
        const bound = setTimeout(() => controller.abort(), OWN_EMAIL_WAIT_MS);
        try {
            const res = await fetch(OWN_EMAIL_URL, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json', [REQUEST_ID_HEADER]: id },
                body: JSON.stringify({ email: typed, current_password: password }),
                signal: controller.signal,
            });
            if (res.ok) {
                pending.current = null;
                showToast('success', t('account.email.changed'));
                onChanged?.();
                onClose();
                return;
            }
            if (answerWasLost(res)) {
                outcomeUnknown();
                return;
            }
            const problem = await readApiError(res);
            // REQUEST_IN_PROGRESS: the first arrival still runs; the same
            // identity will get its answer, so it is kept. REQUEST_OUTCOME_UNKNOWN
            // is final on the server (the Panel restarted while it ran; that
            // identity is answered so for 24 hours and never run again), so the
            // next Save is a new action with a new identity.
            // İlki hâlâ sürüyorsa kimlik korunur; sonuç bilinmiyorsa (kesin) bırakılır.
            if (problem.code === 'REQUEST_OUTCOME_UNKNOWN') pending.current = null;
            if (problem.code && OPEN_OUTCOMES.has(problem.code)) {
                outcomeUnknown();
                return;
            }
            pending.current = null;
            if (!alive.current) return;
            setRefusal(problem);
            if (problem.code === 'account_email_wrong_password') setPassword('');
        } catch {
            // The connection failed or the bound passed: whether the address
            // was saved is not known, and that is not a failure.
            outcomeUnknown();
        } finally {
            clearTimeout(bound);
            if (alive.current) setSaving(false);
        }
    };

    return (
        <Dialog
            id="change-email"
            icon={Mail}
            width="sm"
            title={t('account.email.dialogTitle')}
            busy={saving}
            onDismiss={onClose}
            onSubmit={(event) => void submit(event)}
            noValidate
            actions={
                <>
                    <Button type="button" onClick={onClose} disabled={saving}>{t('common.cancel')}</Button>
                    <Button type="submit" variant="primary" icon={Mail} loading={saving} disabled={!ready}>
                        {t('account.email.save')}
                    </Button>
                </>
            }
        >
            <div className="space-y-4">
                {current !== null && (
                    <p className="text-sm text-fg-muted">
                        <span>{t('account.email.current')}: </span>
                        <span className="break-all font-medium text-fg" data-current-email="">{current || t('account.email.none')}</span>
                    </p>
                )}
                <label className="block">
                    <span className="mb-1 block text-sm font-medium text-fg">{t('account.email.new')}</span>
                    <input
                        type="email"
                        name="email"
                        value={address}
                        onChange={(e) => setAddress(e.target.value)}
                        autoComplete="email"
                        inputMode="email"
                        spellCheck={false}
                        disabled={saving}
                        maxLength={254}
                        className={`${inputClass} max-sm:min-h-11`}
                        autoFocus
                    />
                </label>
                <label className="block">
                    <span className="mb-1 block text-sm font-medium text-fg">{t('account.email.password')}</span>
                    <input
                        type="password"
                        name="current-password"
                        value={password}
                        onChange={(e) => setPassword(e.target.value)}
                        autoComplete="current-password"
                        disabled={saving}
                        className={`${inputClass} max-sm:min-h-11`}
                    />
                    <span className="mt-1 block text-xs text-fg-muted">{t('account.email.passwordHint')}</span>
                </label>
                {saving && (
                    <p role="status" className="flex items-start gap-2 text-sm text-fg-muted" data-email-saving="">
                        <Spinner size="xs" tone="current" />
                        <span>{t('account.email.saving')}</span>
                    </p>
                )}
                {unknown && !saving && (
                    <p role="status" className="rounded-lg border border-warning-mark/60 bg-warning-mark/20 px-3 py-2.5 text-sm text-fg" data-email-unknown="">
                        {t('account.email.outcomeUnknown')}
                    </p>
                )}
                {refusal && !saving && (
                    <div role="alert" data-email-refusal={refusal.code ?? ''}>
                        <ErrorBanner error={refusal} />
                    </div>
                )}
            </div>
        </Dialog>
    );
}
