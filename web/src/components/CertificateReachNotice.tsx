import { AlertTriangle, RefreshCw } from 'lucide-react';
import { useI18n } from '../i18n';
import type { CertificateReachGate } from '../lib/certificateNames';
import { Button } from './ui';

// Why "Get certificate" is off, or what could not be checked, where the person
// looks (owner rule 2026-10-31, and 2 Nov 2026: "If the domain is not on the
// server, the certificate cannot be obtained either"). `top` is the full
// guidance at the top of the SSL/TLS screen: what is wrong, who acts and what
// to do, and how it resumes. `inline` is the short reason beside the disabled
// button. Both carry the same two actions: check again (the screen updates
// itself, no refresh) and, when only mail.<domain> fails, request without it.
// Nothing here starts a request.
//
// "Sertifika al" düğmesinin neden kapalı olduğu ya da neyin kontrol edilemediği,
// kişinin baktığı yerde. `top` ekranın üstünde tam yönlendirme, `inline` kapalı
// düğmenin yanında kısa nedendir. Hiçbiri istek başlatmaz.
export function CertificateReachNotice({
    gate,
    variant,
    domainName,
    onRecheck,
    rechecking,
    onLeaveOutMail,
    onRequestAnyway,
    requestAnywayDisabled = false,
    className = '',
}: {
    gate: CertificateReachGate;
    variant: 'top' | 'inline';
    domainName: string;
    onRecheck: () => void;
    rechecking: boolean;
    onLeaveOutMail: () => void;
    /** The owner states the names are behind a proxy such as Cloudflare: send the request anyway. */
    onRequestAnyway: () => void;
    requestAnywayDisabled?: boolean;
    className?: string;
}) {
    const { t } = useI18n();
    if (gate.kind === 'open') return null;
    if (gate.kind === 'checking') {
        return variant === 'inline' ? (
            <p role="status" className={`text-sm text-fg-muted ${className}`} data-cert-reach="checking">
                {t('ssl.reach.checking', { name: domainName })}
            </p>
        ) : null;
    }

    const blocked = gate.kind === 'blocked';
    const mailName = `mail.${domainName.replace(/\.$/, '').toLowerCase()}`;
    const joined = gate.names.join(', ');
    const tone = blocked
        ? 'border-warning-mark/50 bg-warning-mark/20'
        : 'border-border bg-surface-muted';

    const actions = (
        <div className="mt-3 flex flex-wrap gap-2">
            <Button
                type="button"
                variant="secondary"
                icon={RefreshCw}
                onClick={onRecheck}
                disabled={rechecking}
                loading={rechecking}
            >
                {t(rechecking ? 'ssl.reach.rechecking' : 'ssl.reach.recheck')}
            </Button>
            {blocked && gate.mailOnly && (
                <Button type="button" variant="secondary" onClick={onLeaveOutMail}>
                    {t('ssl.reach.leaveOutMail', { mail: mailName })}
                </Button>
            )}
            {blocked && (
                <Button type="button" variant="secondary" onClick={onRequestAnyway} disabled={requestAnywayDisabled}>
                    {t('ssl.reach.requestAnyway')}
                </Button>
            )}
        </div>
    );

    if (variant === 'inline') {
        return (
            <div id="ssl-reach-reason" role="status" className={className} data-cert-reach={gate.kind} data-blocked-reason="">
                <p className="text-sm text-fg">
                    {blocked
                        ? t(gate.mailOnly ? 'ssl.reach.inline.mailOnly' : 'ssl.reach.inline.blocked', { mail: mailName })
                        : t('ssl.reach.inline.unknown')}
                </p>
                {actions}
            </div>
        );
    }

    let reason: string;
    let action = '';
    if (!blocked) {
        reason = t('ssl.reach.unknown', { names: joined });
    } else if (gate.mailOnly) {
        reason = t('ssl.reach.mailOnly.reason', { mail: gate.names[0] ?? mailName });
        action = t('ssl.reach.mailOnly.action', { mail: gate.names[0] ?? mailName, ip: gate.serverIp });
    } else {
        const nsNow = gate.domainFails && gate.liveNameservers.length > 0
            ? ` ${t('ssl.reach.nsNow', { live: gate.liveNameservers.join(', ') })}`
            : '';
        reason = `${t(gate.names.length === 1 ? 'ssl.reach.reason.one' : 'ssl.reach.reason.many', { names: joined })}${nsNow}.`;
        action = gate.domainFails && gate.nameservers.length > 0
            ? t('ssl.reach.action.ns', { ns: gate.nameservers.join(', '), ip: gate.serverIp })
            : t('ssl.reach.action.record', { names: joined, ip: gate.serverIp });
    }

    return (
        <div
            role="status"
            className={`rounded-lg border px-3 py-3 text-sm text-fg ${tone} ${className}`}
            data-cert-reach-top={gate.kind}
        >
            <p className="font-medium">{t(blocked ? 'ssl.reach.blockedTitle' : 'ssl.reach.unknownTitle')}</p>
            <p className="mt-1 flex items-start gap-2">
                <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
                <span className="break-words">{reason}</span>
            </p>
            {action && <p className="mt-2 break-words">{action}</p>}
            {blocked && <p className="mt-2 text-xs text-fg-subtle">{t('ssl.reach.resume')}</p>}
            {blocked && <p className="mt-1 text-xs text-fg-subtle">{t('ssl.reach.proxyHint')}</p>}
            {actions}
        </div>
    );
}
