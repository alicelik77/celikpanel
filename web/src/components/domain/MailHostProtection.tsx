import { ShieldAlert, ShieldCheck, ShieldQuestion } from 'lucide-react';
import { useI18n } from '../../i18n';
import { useNavigate } from '../../router';
import { decodeCoverageLine, mailHostWords } from './coverageLines';

// The mail screen's sentence under the server name mail programs are told
// (alpha.134): whether a trusted certificate protects it, by which one and
// until when, or why not - the certificate screen's "mail programs" line in
// the same words. Where the fix is the domain's certificate, one link opens
// that screen; a server matter names the administrator in its sentence.
// Posta ekranında sunucu adının altındaki cümle: sertifika ekranındaki satırın aynısı.

const FIXED_ON_CERTIFICATE_SCREEN = new Set(['no_certificate', 'not_in_certificate', 'certificate_not_usable', 'mail_not_using']);

export function MailHostProtection({ host, protection, domainName }: { host: string; protection: unknown; domainName: string }) {
    const { t, locale } = useI18n();
    const line = decodeCoverageLine(protection);
    const navigate = useNavigate();
    const state = line?.state ?? 'unknown';
    const Icon = state === 'protected' ? ShieldCheck : state === 'not_protected' ? ShieldAlert : ShieldQuestion;
    const tone = state === 'protected' ? 'text-success' : state === 'not_protected' ? 'text-danger' : 'text-fg-subtle';
    const fixHere = state === 'not_protected' && FIXED_ON_CERTIFICATE_SCREEN.has(line?.reason ?? '');
    return (
        <p className="mt-3 flex flex-wrap items-start gap-x-2 gap-y-1 text-sm" data-mail-host-protection={state}>
            <Icon className={`mt-0.5 h-4 w-4 shrink-0 ${tone}`} aria-hidden="true" />
            <span className="min-w-0 flex-1">
                <span className="font-mono text-fg">{host}</span>
                <span className="text-fg-muted">: {mailHostWords(t, locale, line)}</span>
            </span>
            {fixHere && (
                <button
                    type="button"
                    className="text-sm font-medium text-primary underline underline-offset-4"
                    onClick={() => navigate(`/domains/${encodeURIComponent(domainName)}?section=security&part=certificate`)}
                >
                    {t('certcover.mailHost.fix')}
                </button>
            )}
        </p>
    );
}
