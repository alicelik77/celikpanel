import { AlertTriangle } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';
import { useI18n } from '../i18n';
import { Button } from './ui';

// A change that was made, whose one-time result did not reach this page
// (D-029: `409 REQUEST_COMPLETED_RESULT_NOT_RETAINED`). The server keeps only
// that it was done: a VPN device's configuration and a database user's
// password are shown once and stored nowhere, so asking again cannot bring
// them back. That is not a failure (the device or the database exists) and is
// not drawn as one; it is something the person has to act on, so it stands in
// place on the attention surface with the sentence that says what to do, until
// they close it.
//
// Yapılmış, ama tek seferlik sonucu bu sayfaya ulaşmamış değişiklik (D-029).
// Sunucu yalnız yapıldığını saklar: VPN cihazının yapılandırması ve veritabanı
// kullanıcısının parolası bir kez gösterilir, hiçbir yerde saklanmaz. Bu bir
// hata değildir ve hata gibi çizilmez; kişinin yapması gereken bir şeydir, bu
// yüzden ne yapılacağını söyleyen cümleyle, kapatılana dek yerinde durur.
export function OnceOnlyNotice({
    text,
    onClose,
    className,
}: {
    /** What was made, that its result cannot be shown again, and what to do. */
    text: string | null;
    onClose: () => void;
    className?: string;
}) {
    const { t } = useI18n();
    const box = useRef<HTMLDivElement>(null);
    useEffect(() => {
        if (text) box.current?.scrollIntoView?.({ block: 'nearest' });
    }, [text]);
    if (!text) return null;
    return (
        <div
            ref={box}
            role="alert"
            data-once-only
            className={`flex items-start gap-2 rounded-lg border border-warning-mark/50 bg-warning-mark/20 p-3 text-sm leading-relaxed text-fg ${className ?? ''}`}
        >
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
            <div className="min-w-0">
                <p className="max-w-[75ch] break-words">{text}</p>
                <div className="mt-2">
                    <Button type="button" onClick={onClose}>{t('common.close')}</Button>
                </div>
            </div>
        </div>
    );
}

/** The answer that says a change was made and its one-time result is not kept. */
export function resultNotKept(problem: { code?: string; reason?: string }): boolean {
    return problem.code === 'REQUEST_COMPLETED_RESULT_NOT_RETAINED' && !problem.reason;
}

// A secret the server made up and answers once (a database user's password):
// shown in place, with Copy, and a plain note that it is not shown again,
// until the person closes it (owner feedback 2026-10-31: it was a toast that
// vanished in five seconds). It is never written to the console or a log,
// and leaves memory when the notice is closed.
// Sunucunun bir kez yanitladigi gizli bilgi: yerinde, Kopyala ile ve bir daha
// gosterilmeyecegi notuyla, kapatilana dek. Konsola ya da kayda yazilmaz.
export function ShownOnceSecret({
    title,
    rows,
    secret,
    onClose,
    className,
}: {
    title: string;
    /** Plain facts beside the secret: the user name, the database. */
    rows: { label: string; value: string }[];
    secret: { label: string; value: string };
    onClose: () => void;
    className?: string;
}) {
    const { t } = useI18n();
    const box = useRef<HTMLDivElement>(null);
    const [copied, setCopied] = useState<'yes' | 'no' | null>(null);
    useEffect(() => { box.current?.scrollIntoView?.({ block: 'nearest' }); }, []);
    const copy = async () => {
        try {
            await navigator.clipboard.writeText(secret.value);
            setCopied('yes');
        } catch {
            setCopied('no');
        }
    };
    return (
        <div ref={box} role="alert" data-shown-once="" className={`rounded-lg border border-warning-mark/50 bg-warning-mark/20 p-4 text-sm leading-relaxed text-fg ${className ?? ''}`}>
            <p className="font-semibold">{title}</p>
            <dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
                {rows.map(row => <div key={row.label} className="contents"><dt className="text-fg-muted">{row.label}</dt><dd className="break-all font-mono">{row.value}</dd></div>)}
                <dt className="text-fg-muted">{secret.label}</dt>
                <dd className="break-all font-mono" data-secret="">{secret.value}</dd>
            </dl>
            <p className="mt-2 font-medium">{t('secret.shownOnce')}</p>
            <div className="mt-3 flex flex-wrap items-center gap-2">
                <Button type="button" variant="primary" onClick={() => void copy()}>{t('secret.copy')}</Button>
                <Button type="button" onClick={onClose}>{t('secret.saved')}</Button>
                {copied && <span role="status" className="text-xs text-fg-muted">{t(copied === 'yes' ? 'secret.copied' : 'secret.copyFailed')}</span>}
            </div>
        </div>
    );
}
