import { AlertTriangle, CheckCircle2, Info } from 'lucide-react';
import { useAuth } from '../auth/AuthContext';
import { useMailConfigWords } from '../i18n/mailConfig';
import type { TranslationKey } from '../i18n/en';
import { apiErrorText, readApiError } from '../lib/apiError';
import { confirmAction } from '../lib/confirmAction';
import type { MailAutoconfig } from '../lib/mailSetup';
import { mailSetupURL } from '../lib/mailSetup';
import { refreshRemote } from '../lib/remote';
import { flowSteps, sendWithSteps } from '../lib/requestProgress';
import { Button } from './ui';

// Whether mail programs set themselves up for this domain (alpha.130): the
// state first, then what is missing with who acts, and the one press that
// finishes what this server can finish (the site file and missing DNS
// records). Existing DNS records are never replaced. The manual settings
// below it stay the fallback.
//
// Posta programlarının kendini ayarlayıp ayarlamadığı: önce durum, sonra
// eksikler ve kimin yapacağı; bu sunucunun bitirebileceği tek düğmeyle biter.

interface Props {
    domainId: number;
    autoconfig: MailAutoconfig;
    readOnly?: boolean;
}

const pair = (names: string[]) => names.join(' · ');

export function MailAutoconfigNotice({ domainId, autoconfig, readOnly = false }: Props) {
    const { t } = useMailConfigWords();
    const { user } = useAuth();
    const isAdmin = user?.role === 'admin';
    const names = pair(autoconfig.names);
    const missing = autoconfig.records.filter((r) => r.state === 'missing');
    const different = autoconfig.records.filter((r) => r.state === 'different');

    if (autoconfig.state === 'unavailable') {
        const reason = autoconfig.reason === 'mail_host_unknown' && !isAdmin ? 'mail_host_unknownCustomer' : autoconfig.reason;
        const known = ['subdomain', 'mail_not_here', 'name_taken', 'no_site', 'mail_host_unknown', 'mail_host_unknownCustomer', 'suspended'].includes(reason);
        return (
            <p className="mb-4 flex items-start gap-2 text-sm text-fg-muted" data-mail-autoconfig="unavailable">
                <Info className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
                <span className="max-w-[75ch]">{t(`mcfg.auto.reason.${known ? reason : 'unknown'}` as TranslationKey, { names })}</span>
            </p>
        );
    }

    const fix = async () => {
        await confirmAction({
            title: t('mcfg.auto.title'),
            message: t('mcfg.auto.confirm', { names: pair(missing.map((r) => r.name)) || names }),
            confirmLabel: t('mcfg.auto.fix'),
            progressTitle: t('mcfg.auto.progress'),
            action: async (report) => {
                try {
                    const res = await sendWithSteps(report, flowSteps(t, 'mailAutoconfig', ['check!', 'site', 'dns']), t, 'mailAutoconfig',
                        (headers) => fetch(`/api/v1/domains/${domainId}/mail/autoconfig`, { method: 'POST', headers }));
                    if (!res.ok) {
                        const error = await readApiError(res);
                        if (error.code === 'MAIL_AUTOCONFIG_SITE_FILE_KEPT') return { state: 'blocked', reason: t('mcfg.auto.siteKept') };
                        return { state: 'failed', reason: t('mcfg.auto.failed', { reason: apiErrorText(error, t) }) };
                    }
                    const answer = await res.json() as { dns?: string; different?: string[] };
                    if (answer.different && answer.different.length > 0) return { state: 'done', message: t('mcfg.auto.doneDifferent', { names: pair(answer.different) }) };
                    if (answer.dns === 'not_here' || answer.dns === 'no_zone') return { state: 'done', message: t('mcfg.auto.doneDnsElsewhere') };
                    return { state: 'done', message: t('mcfg.auto.done') };
                } catch {
                    return { state: 'unknown', reason: t('common.resultUnknown') };
                } finally {
                    refreshRemote(mailSetupURL(domainId));
                }
            },
        });
    };

    const problems: string[] = [];
    if (autoconfig.served === 'no') problems.push(t('mcfg.auto.notServed', { names }));
    if (autoconfig.served === 'unknown') problems.push(t('mcfg.auto.unknown'));
    if (autoconfig.served === 'yes' && autoconfig.secure === 'no') problems.push(t('mcfg.auto.notSecure', { names }));
    if (autoconfig.served === 'yes' && autoconfig.secure === 'unknown') problems.push(t('mcfg.auto.secureUnknown', { names }));
    if (autoconfig.dns === 'local' && missing.length > 0) problems.push(t('mcfg.auto.recordsMissing', { names: pair(missing.map((r) => r.name)) }));
    for (const record of different) problems.push(t('mcfg.auto.recordDifferent', { name: record.name, type: record.type, content: record.content }));

    const on = autoconfig.state === 'on';
    const surface = on ? 'border-success/40 bg-success/10' : 'border-warning-mark/50 bg-warning-mark/15';
    const Icon = on ? CheckCircle2 : AlertTriangle;
    const elsewhere = autoconfig.dns !== 'local' && autoconfig.records.length > 0;
    return (
        <div role="status" data-mail-autoconfig={autoconfig.state} className={`mb-4 rounded-lg border p-3 text-sm text-fg ${surface}`}>
            <div className="flex flex-wrap items-start gap-2">
                <Icon className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
                <div className="min-w-0 max-w-[75ch] flex-1">
                    <p className="font-medium">{t('mcfg.auto.title')}</p>
                    <p className="mt-0.5">{on ? t('mcfg.auto.on') : autoconfig.state === 'off' ? t('mcfg.auto.off') : t('mcfg.auto.partial')}</p>
                    {!on && problems.length > 0 && (
                        <ul className="mt-1 list-disc space-y-0.5 pl-5 text-fg-muted">
                            {problems.map((text) => <li key={text}>{text}</li>)}
                        </ul>
                    )}
                    {!on && elsewhere && (
                        <>
                            <p className="mt-2 text-fg-muted">{t('mcfg.auto.recordsElsewhere')}</p>
                            <ul className="mt-1 space-y-0.5 font-mono text-xs">
                                {autoconfig.records.map((r) => (
                                    <li key={r.name} className="break-all">{r.name} {r.type} {r.type === 'SRV' ? `${r.prio ?? 0} ` : ''}{r.content}</li>
                                ))}
                            </ul>
                        </>
                    )}
                </div>
                {!on && autoconfig.can_fix && (readOnly
                    ? <span className="text-xs text-fg-muted">{t('mcfg.auto.fixReadOnly')}</span>
                    : <Button type="button" variant="primary" onClick={() => void fix()}>{t('mcfg.auto.fix')}</Button>)}
            </div>
        </div>
    );
}

export default MailAutoconfigNotice;
