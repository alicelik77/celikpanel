import { useEffect, useState } from 'react';
import { Shield, ShieldCheck, ShieldOff, Copy, Check, KeyRound, Mail } from 'lucide-react';
import { showToast } from './Toast';
import { useI18n } from '../i18n';
import { Button, RemoteGate, inputClass } from './ui';
import { PageHeader } from './PageHeader';
import { readApiError } from '../lib/apiError';
import { useRemote } from '../lib/remote';
import { ChangePasswordModal } from './ChangePasswordModal';
import { ChangeEmailModal } from './ChangeEmailModal';
import { useLocation } from '../router';

// My account: the signed-in person's own sign-in security, opened from the
// personal menu at the top right (D-037, owner 2026-10-22). It is what
// Settings called "Account security" until v0.1.0-alpha.98, moved as it was,
// with the password change beside it: personal things are looked for under
// one's own name, not in the server's settings. Every role that had the
// section has this page (admin, reseller, customer); /settings?section=account
// arrives here.
//
// Hesabım: oturum açan kişinin kendi giriş güvenliği; sağ üstteki kişisel
// menüden açılır (D-037). v0.1.0-alpha.98'e dek Ayarlar'ın "Hesap güvenliği"
// bölümüydü; olduğu gibi taşındı, yanına şifre değiştirme geldi. Bölümü olan
// her rol bu sayfayı görür; /settings?section=account buraya ulaşır.
export function AccountPage() {
    const { t } = useI18n();
    return (
        <div className="p-4 sm:p-6 md:p-8">
            <PageHeader title={t('nav.account')} subtitle={t('account.subtitle')} breadcrumb={[t('common.home'), t('nav.account')]} />
            <div className="grid max-w-3xl gap-5">
                <PasswordPanel />
                <EmailPanel />
                <TwoFactorPanel />
            </div>
        </div>
    );
}

// The password change the personal menu has always opened, in the same
// dialog; this page is its place now.
// Kişisel menünün her zaman açtığı şifre değiştirme, aynı pencerede.
function PasswordPanel() {
    const { t } = useI18n();
    const [open, setOpen] = useState(false);
    return (
        <section className="rounded-xl border border-border bg-surface p-5">
            <div className="mb-1 flex items-center gap-2">
                <KeyRound className="h-5 w-5 text-fg-subtle" aria-hidden="true" />
                <h2 className="text-base font-semibold text-fg">{t('account.password.title')}</h2>
            </div>
            <p className="mb-4 text-sm text-fg-muted">{t('account.password.desc')}</p>
            <Button variant="secondary" icon={KeyRound} className="max-sm:min-h-11" onClick={() => setOpen(true)}>
                {t('profile.changePassword')}
            </Button>
            {open && <ChangePasswordModal onClose={() => setOpen(false)} />}
        </section>
    );
}

const OWN_IDENTITY_URL = '/api/v1/auth/me';

// The signed-in person's own address, from the identity answer. An answer
// without the field is not "no address".
// Oturum açan kişinin kendi adresi; alanı taşımayan yanıt "adres yok" değildir.
function decodeOwnEmail(raw: unknown): string {
    if (!raw || typeof raw !== 'object' || typeof (raw as { email?: unknown }).email !== 'string') {
        throw new Error('shape');
    }
    return (raw as { email: string }).email;
}

// My email (v0.1.0-alpha.99, D-037 amendment): the address CelikPanel keeps
// for the person, which for an administrator is also the panel certificate's
// contact. /account#email is where the certificate's and the setup plan's
// contact refusal send the person. The address is read from the server; the
// change is offered only on a known answer.
// E-posta adresim: kişinin adresi; yönetici için panel sertifikasının iletişim
// adresidir. /account#email, sertifikanın ve kurulum planının iletişim reddinin
// gönderdiği yerdir. Değişiklik yalnız bilinen bir yanıt üzerinde sunulur.
function EmailPanel() {
    const { t } = useI18n();
    const location = useLocation();
    const identity = useRemote(OWN_IDENTITY_URL, decodeOwnEmail);
    const [open, setOpen] = useState(false);
    const known = identity.remote.state === 'known' ? identity.remote.value : null;

    useEffect(() => {
        if (location.hash !== '#email') return;
        document.getElementById('email')?.scrollIntoView?.({ block: 'start' });
    }, [location.hash]);

    return (
        <section id="email" className="scroll-mt-24 rounded-xl border border-border bg-surface p-5" data-account-section="email">
            <div className="mb-1 flex items-center gap-2">
                <Mail className="h-5 w-5 text-fg-subtle" aria-hidden="true" />
                <h2 className="text-base font-semibold text-fg">{t('account.email.title')}</h2>
            </div>
            <p className="mb-4 text-sm text-fg-muted">{t('account.email.desc')}</p>
            <div className="mb-4 min-h-[1.5rem]">
                <RemoteGate
                    remote={identity.remote}
                    checking={t('account.email.checking')}
                    failed={t('account.email.unknown')}
                    onRetry={() => void identity.retry()}
                    busy={identity.reading}
                >
                    {({ value }) => (
                        <p className="break-all text-sm font-medium text-fg" data-own-email="">
                            {value || t('account.email.none')}
                        </p>
                    )}
                </RemoteGate>
            </div>
            <Button
                variant="secondary"
                icon={Mail}
                className="max-sm:min-h-11"
                disabled={known === null || identity.reading}
                onClick={() => setOpen(true)}
            >
                {t('account.email.change')}
            </Button>
            {open && (
                <ChangeEmailModal
                    current={known}
                    onClose={() => setOpen(false)}
                    onChanged={() => void identity.retry()}
                    onUnknown={() => void identity.retry()}
                />
            )}
        </section>
    );
}

const TWO_FACTOR_STATUS_URL = '/api/v1/auth/2fa/status';

// Whether two-factor sign-in is on for the signed-in account. An answer
// without the field is not "off".
// Oturum açan hesapta iki faktörlü girişin açık olup olmadığı. Alanı
// taşımayan yanıt "kapalı" değildir.
interface TwoFactorStatus {
    enabled: boolean;
    // The recovery codes left and the size of the set (alpha.114); null when
    // the server did not say (an older server, or 2FA is off).
    recoveryRemaining: number | null;
    recoveryTotal: number | null;
}

function decodeTwoFactorStatus(raw: unknown): TwoFactorStatus {
    if (!raw || typeof raw !== 'object' || typeof (raw as { enabled?: unknown }).enabled !== 'boolean') {
        throw new Error('shape');
    }
    const body = raw as { enabled: boolean; recovery_remaining?: unknown; recovery_total?: unknown };
    return {
        enabled: body.enabled,
        recoveryRemaining: typeof body.recovery_remaining === 'number' ? body.recovery_remaining : null,
        recoveryTotal: typeof body.recovery_total === 'number' ? body.recovery_total : null,
    };
}

// The recovery codes the server has just made. They are in the answer once;
// they are read from nowhere else.
// Sunucunun az önce ürettiği kurtarma kodları; yanıtta bir kez bulunur.
async function readRecoveryCodes(res: Response): Promise<string[] | null> {
    try {
        const codes = ((await res.json()) as { recovery_codes?: unknown } | null)?.recovery_codes;
        if (!Array.isArray(codes) || codes.length === 0 || !codes.every((code) => typeof code === 'string')) return null;
        return codes as string[];
    } catch (error) {
        // The answer could not be read as the codes; the caller says so.
        console.warn('recovery codes answer could not be read', error);
        return null;
    }
}

function TwoFactorPanel() {
    const { t } = useI18n();
    const status = useRemote(TWO_FACTOR_STATUS_URL, decodeTwoFactorStatus);
    const [setup, setSetup] = useState<{ secret: string; uri: string } | null>(null);
    const [qr, setQr] = useState<string>('');

    // The otpauth:// URI as a QR code — scanning beats typing a 32-char
    // secret. Generated locally in the browser; the secret never leaves.
    // otpauth:// URI'si QR olarak — taramak 32 karakterlik anahtarı yazmayı
    // döver. Tarayıcıda yerel üretilir; anahtar dışarı çıkmaz.
    useEffect(() => {
        let cancelled = false;
        setQr('');
        if (setup?.uri) {
            void import('qrcode')
                .then(({ default: QRCode }) => QRCode.toDataURL(setup.uri, { width: 192, margin: 1 }))
                .then((url) => {
                    if (!cancelled) setQr(url);
                })
                .catch(() => {
                    // Without the picture the secret below can still be typed.
                    if (!cancelled) setQr('');
                });
        }

        return () => {
            cancelled = true;
        };
    }, [setup]);
    const [code, setCode] = useState('');
    const [setupPw, setSetupPw] = useState('');
    const [disablePw, setDisablePw] = useState('');
    const [disableCode, setDisableCode] = useState('');
    const [busy, setBusy] = useState(false);
    const [copied, setCopied] = useState(false);
    // The codes just made, shown until the person says they saved them.
    const [newCodes, setNewCodes] = useState<string[] | null>(null);
    const [regenPw, setRegenPw] = useState('');
    const [regenCode, setRegenCode] = useState('');
    const [codesCopied, setCodesCopied] = useState(false);

    // A change whose answer did not arrive is not known to have failed: the
    // state is read again instead of being assumed.
    // Yanıtı gelmeyen değişikliğin başarısız olduğu bilinmez: durum
    // varsayılmaz, yeniden okunur.
    const lostAnswer = (error: unknown) => {
        if (!(error instanceof TypeError)) return false;
        showToast('error', t('settings.2fa.resultUnknown'));
        void status.retry();
        return true;
    };

    const startSetup = async () => {
        setBusy(true);
        try {
            const r = await fetch('/api/v1/auth/2fa/setup', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ password: setupPw }),
            });
            if (!r.ok) throw new Error((await readApiError(r)).message);
            setSetup(await r.json());
        } catch (e) {
            if (!lostAnswer(e)) showToast('error', (e as Error).message || t('settings.2fa.reauthFailed'));
        } finally {
            setBusy(false);
        }
    };

    const enable = async () => {
        setBusy(true);
        try {
            const r = await fetch('/api/v1/auth/2fa/enable', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ password: setupPw, code: code.trim() }),
            });
            if (!r.ok) throw new Error((await readApiError(r)).message);
            showToast('success', t('settings.2fa.enabled'));
            const made = await readRecoveryCodes(r);
            if (made) setNewCodes(made);
            else showToast('warning', t('account.recovery.unreadable'));
            setSetup(null);
            setSetupPw('');
            setCode('');
            await status.retry();
        } catch (e) {
            if (!lostAnswer(e)) showToast('error', (e as Error).message || t('settings.2fa.badCode'));
        } finally {
            setBusy(false);
        }
    };

    const disable = async () => {
        setBusy(true);
        try {
            const r = await fetch('/api/v1/auth/2fa/disable', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ password: disablePw, code: disableCode.trim() }),
            });
            if (!r.ok) throw new Error();
            showToast('success', t('settings.2fa.disabled'));
            setNewCodes(null);
            setDisablePw('');
            setDisableCode('');
            await status.retry();
        } catch (e) {
            if (!lostAnswer(e)) showToast('error', t('settings.2fa.disableFailed'));
        } finally {
            setBusy(false);
        }
    };

    const regenerate = async () => {
        setBusy(true);
        try {
            const r = await fetch('/api/v1/auth/2fa/recovery/regenerate', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ password: regenPw, code: regenCode.trim() }),
            });
            if (!r.ok) throw new Error((await readApiError(r)).message);
            const made = await readRecoveryCodes(r);
            if (made) {
                setNewCodes(made);
                showToast('success', t('account.recovery.done'));
            } else {
                showToast('warning', t('account.recovery.unreadable'));
            }
            setRegenPw('');
            setRegenCode('');
            await status.retry();
        } catch (e) {
            if (!lostAnswer(e)) showToast('error', (e as Error).message || t('settings.2fa.reauthFailed'));
        } finally {
            setBusy(false);
        }
    };

    const copyCodes = () => {
        if (!newCodes) return;
        navigator.clipboard?.writeText(newCodes.join('\n')).then(() => {
            setCodesCopied(true);
            setTimeout(() => setCodesCopied(false), 1200);
        });
    };

    const downloadCodes = () => {
        if (!newCodes) return;
        const text = `${t('account.recovery.fileHeader')}\n\n${newCodes.join('\n')}\n`;
        const url = URL.createObjectURL(new Blob([text], { type: 'text/plain' }));
        const link = document.createElement('a');
        link.href = url;
        link.download = 'celikpanel-recovery-codes.txt';
        link.click();
        URL.revokeObjectURL(url);
    };

    const copySecret = () => {
        if (!setup) return;
        navigator.clipboard?.writeText(setup.secret).then(() => {
            setCopied(true);
            setTimeout(() => setCopied(false), 1200);
        });
    };

    // "On" and "off" are drawn only for an answer the server gave.
    const known = status.remote.state === 'known' ? status.remote.value.enabled : null;
    const remaining = status.remote.state === 'known' ? status.remote.value.recoveryRemaining : null;
    const total = status.remote.state === 'known' ? status.remote.value.recoveryTotal : null;
    // A change is offered only on the state as it is now, not while it is
    // being read again and not on an earlier answer.
    const locked = busy || status.reading || status.remote.state !== 'known';

    return (
        <section className="rounded-xl border border-border bg-surface p-5">
            <div className="mb-1 flex items-center gap-2">
                {known === true ? (
                    <ShieldCheck className="h-5 w-5 text-success" />
                ) : known === false ? (
                    <ShieldOff className="h-5 w-5 text-fg-subtle" />
                ) : (
                    <Shield className="h-5 w-5 text-fg-subtle" />
                )}
                <h2 className="text-base font-semibold text-fg">{t('settings.2fa.title')}</h2>
                {known === true && (
                    <span className="ml-auto rounded-md bg-success/10 px-2 py-0.5 text-xs font-medium text-success">
                        {t('settings.2fa.on')}
                    </span>
                )}
            </div>
            <p className="mb-4 text-sm text-fg-muted">{t('settings.2fa.desc')}</p>

            {/* The least height of the two forms, so the card is the same
                size while it is checking and when the answer arrives.
                İki formun en az yüksekliği; kart kontrol ederken ve yanıt
                gelince aynı boydadır. */}
            <div className="min-h-[11.25rem] sm:min-h-[10.5rem]">
                <RemoteGate
                    remote={status.remote}
                    checking={t('settings.2fa.checking')}
                    failed={t('settings.2fa.unknown')}
                    onRetry={() => void status.retry()}
                    busy={status.reading}
                >
                    {({ value: { enabled } }) => (enabled ? (
                        <div className="space-y-3">
                            {newCodes && (
                                <div className="space-y-3 rounded-lg border border-warning/40 bg-warning/10 p-3" role="region" aria-label={t('account.recovery.shownTitle')}>
                                    <p className="text-sm font-semibold text-fg">{t('account.recovery.shownTitle')}</p>
                                    <p className="text-sm text-fg-muted">{t('account.recovery.shownOnce')}</p>
                                    <ul className="grid grid-cols-1 gap-1 font-mono text-sm text-fg sm:grid-cols-2">
                                        {newCodes.map((code) => <li key={code}>{code}</li>)}
                                    </ul>
                                    <div className="flex flex-wrap gap-2">
                                        <Button variant="secondary" icon={codesCopied ? Check : Copy} onClick={copyCodes}>{t('account.recovery.copyAll')}</Button>
                                        <Button variant="secondary" onClick={downloadCodes}>{t('account.recovery.download')}</Button>
                                        <Button variant="primary" icon={Check} onClick={() => setNewCodes(null)}>{t('account.recovery.saved')}</Button>
                                    </div>
                                </div>
                            )}
                            <div className="space-y-2 border-b border-border pb-4">
                                <h3 className="text-sm font-semibold text-fg">{t('account.recovery.title')}</h3>
                                <p className="text-sm text-fg-muted">{t('account.recovery.desc')}</p>
                                {remaining === null || total === null ? (
                                    <p className="text-sm text-fg-subtle">{t('account.recovery.unknown')}</p>
                                ) : (
                                    <p className={`text-sm ${remaining <= 2 ? 'font-medium text-warning' : 'text-fg'}`}>
                                        {remaining === 0
                                            ? t('account.recovery.none')
                                            : remaining <= 2
                                                ? t('account.recovery.low', { remaining })
                                                : t('account.recovery.remaining', { remaining, total })}
                                    </p>
                                )}
                                <p className="text-sm text-fg-muted">{t('account.recovery.regenHint')}</p>
                                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                                    <label className="space-y-1 text-sm font-medium text-fg">
                                        <span>{t('login.password')}</span>
                                        <input
                                            type="password"
                                            value={regenPw}
                                            onChange={(e) => setRegenPw(e.target.value)}
                                            autoComplete="current-password"
                                            className={inputClass}
                                        />
                                    </label>
                                    <label className="space-y-1 text-sm font-medium text-fg">
                                        <span>{t('settings.2fa.enterCode')}</span>
                                        <input
                                            value={regenCode}
                                            onChange={(e) => setRegenCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                                            inputMode="numeric"
                                            autoComplete="one-time-code"
                                            placeholder="000000"
                                            className={`${inputClass} font-mono tracking-widest`}
                                        />
                                    </label>
                                </div>
                                <Button variant="secondary" icon={KeyRound} loading={busy} disabled={locked || !regenPw || regenCode.length < 6} onClick={regenerate}>
                                    {busy ? t('account.recovery.working') : t('account.recovery.regenerate')}
                                </Button>
                            </div>
                            <p className="text-sm text-fg-muted">{t('settings.2fa.disableHint')}</p>
                            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                                <label className="space-y-1 text-sm font-medium text-fg">
                                    <span>{t('login.password')}</span>
                                    <input
                                        type="password"
                                        value={disablePw}
                                        onChange={(e) => setDisablePw(e.target.value)}
                                        autoComplete="current-password"
                                        className={inputClass}
                                    />
                                </label>
                                <label className="space-y-1 text-sm font-medium text-fg">
                                    <span>{t('settings.2fa.enterCode')}</span>
                                    <input
                                        value={disableCode}
                                        onChange={(e) => setDisableCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                                        inputMode="numeric"
                                        autoComplete="one-time-code"
                                        placeholder="000000"
                                        className={`${inputClass} font-mono tracking-widest`}
                                    />
                                </label>
                            </div>
                            <Button variant="danger" icon={ShieldOff} disabled={locked || !disablePw || disableCode.length < 6} onClick={disable}>
                                {t('settings.2fa.disable')}
                            </Button>
                        </div>
                    ) : setup ? (
                        <div className="space-y-4">
                            <ol className="list-inside list-decimal space-y-1 text-sm text-fg-muted">
                                <li>{t('settings.2fa.step1')}</li>
                                <li>{t('settings.2fa.step2')}</li>
                            </ol>
                            {qr && (
                                <div className="flex justify-center">
                                    <img src={qr} alt="TOTP QR" className="rounded-lg border border-border bg-white p-1" width={192} height={192} />
                                </div>
                            )}
                            <div className="flex items-center gap-2 rounded-lg border border-border bg-surface-2/50 p-3">
                                <code className="min-w-0 flex-1 break-all font-mono text-sm text-fg">{setup.secret}</code>
                                <button onClick={copySecret} title={t('common.copy')} className="rounded-md p-1.5 text-fg-subtle hover:bg-surface-2 hover:text-fg">
                                    {copied ? <Check className="h-4 w-4 text-success" /> : <Copy className="h-4 w-4" />}
                                </button>
                            </div>
                            <a href={setup.uri} className="text-sm text-primary hover:underline">{t('settings.2fa.openApp')}</a>
                            <div>
                                <label className="mb-1.5 block text-sm font-medium text-fg-muted">{t('settings.2fa.enterCode')}</label>
                                <input value={code} onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))} placeholder="000000" className={`${inputClass} max-w-[12rem] font-mono text-lg tracking-[0.3em]`} />
                            </div>
                            <div className="flex gap-2">
                                <Button variant="primary" icon={ShieldCheck} disabled={locked || !setupPw || code.length < 6} onClick={enable}>
                                    {t('settings.2fa.verify')}
                                </Button>
                                <Button variant="secondary" onClick={() => { setSetup(null); setSetupPw(''); setCode(''); }}>
                                    {t('common.back')}
                                </Button>
                            </div>
                        </div>
                    ) : (
                        <div className="space-y-3">
                            <p className="text-sm text-fg-muted">{t('settings.2fa.reauthHint')}</p>
                            <label className="block max-w-md space-y-1 text-sm font-medium text-fg">
                                <span>{t('login.password')}</span>
                                <input
                                    type="password"
                                    value={setupPw}
                                    onChange={(e) => setSetupPw(e.target.value)}
                                    autoComplete="current-password"
                                    className={inputClass}
                                />
                            </label>
                            <Button variant="primary" icon={ShieldCheck} disabled={locked || !setupPw} onClick={startSetup}>
                                {t('settings.2fa.setup')}
                            </Button>
                        </div>
                    ))}
                </RemoteGate>
            </div>
        </section>
    );
}
