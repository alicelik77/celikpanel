import { AlertTriangle, CheckCircle, HelpCircle, XCircle } from 'lucide-react';
import { useI18n } from '../../i18n';
import type { TranslationKey } from '../../i18n/en';
import { Checking, CouldNotCheck } from '../ui';
import { RowActions, type RowAction } from '../kit/rows/RowActions';
import type { Remote } from '../../lib/remote';
import { isDNSPrerequisite, lineKindWords, lineNextWords, lineWords, type Coverage, type CoverageAction, type CoverageLine } from './coverageLines';

// "Neler korunuyor" (alpha.134, docs/CERTIFICATE-COVERAGE.md): one list of
// everything of the domain that visitors and mail programs connect to - what
// it is, the name, and "Protected" (by which certificate, until when) or
// "Not protected" with one sentence and at most one action. It only shows
// what the server said; the actions are the certificate screen's own
// (handed in), so each runs in that screen's progress window.
//
// Alan adında ziyaretçilerin ve posta programlarının bağlandığı her şeyin tek
// listesi: ne olduğu, adı, korunuyor mu; korunmuyorsa tek cümle ve tek eylem.

export interface CoverageActions {
    /** Request the certificate again (with every name that points here). */
    reissue?: () => void;
    /** The same request, offered on the mail name ("Cover the mail name too"). */
    coverMail?: () => void;
    /** Open the upload form (an uploaded certificate is not requested again). */
    upload?: () => void;
    /** Mail starts serving the certificate that already covers the name. */
    useForMail?: () => void;
    /** Open the domain's DNS records, or its mail settings, on this page. */
    openDns?: () => void;
    openMail?: () => void;
    /** Something runs now: the actions are off. */
    busy?: boolean;
}

const STATE_ICON = { protected: CheckCircle, not_protected: XCircle, unknown: HelpCircle } as const;
const STATE_TONE = { protected: 'text-success', not_protected: 'text-danger', unknown: 'text-fg-subtle' } as const;

export function CertificateCoverage({ coverage, onRetry, rereading, actions }: {
    coverage: Remote<Coverage>;
    onRetry: () => void;
    rereading: boolean;
    actions: CoverageActions;
}) {
    const { t, locale } = useI18n();
    const action = (line: CoverageLine): RowAction | null => {
        const id: CoverageAction | undefined = line.action;
        const label = id ? t(`certcover.action.${id}` as TranslationKey) : '';
        const disabled = actions.busy === true;
        switch (id) {
            case 'reissue':
                return actions.reissue ? { id, label, onSelect: actions.reissue, disabled } : null;
            case 'cover_mail':
                return actions.coverMail ? { id, label, onSelect: actions.coverMail, disabled } : null;
            case 'upload':
                return actions.upload ? { id, label, onSelect: actions.upload, disabled } : null;
            case 'use_for_mail':
                return actions.useForMail ? { id, label, onSelect: actions.useForMail, disabled } : null;
            case 'dns':
                return actions.openDns ? { id, label, onSelect: actions.openDns } : null;
            case 'mail_setup':
                return actions.openMail ? { id, label, onSelect: actions.openMail } : null;
            case 'recheck':
                // Reads only: the public DNS is asked again.
                return { id, label, onSelect: onRetry, disabled: rereading };
            default:
                return null;
        }
    };

    return (
        <div className="mt-4 border-t border-border pt-4" data-certificate-coverage="">
            <p className="text-sm font-semibold text-fg">{t('certcover.title')}</p>
            <p className="mt-0.5 text-xs text-fg-muted">{t('certcover.hint')}</p>
            {coverage.state === 'loading' && <Checking label={t('certcover.checking')} className="mt-3" />}
            {coverage.state === 'unknown' && (
                <CouldNotCheck className="mt-3" text={t('certcover.unknown')} onRetry={onRetry} busy={rereading} />
            )}
            {coverage.state === 'known' && coverage.value.lines.length > 0 && (
                <ul className="mt-3 divide-y divide-border rounded-lg border border-border">
                    {coverage.value.lines.map((line) => {
                        // A name waiting for DNS is a prerequisite, not a failure:
                        // its badge says what it waits for, in the warning tone.
                        const waiting = isDNSPrerequisite(line);
                        const Icon = waiting ? AlertTriangle : STATE_ICON[line.state];
                        const fix = line.state === 'protected' ? null : action(line);
                        const next = lineNextWords(t, line, coverage.value.server_ip);
                        return (
                            <li
                                key={`${line.kind}:${line.name}`}
                                className="flex flex-wrap items-start justify-between gap-x-4 gap-y-2 px-3 py-2.5 text-sm"
                                data-coverage-line={line.kind}
                                data-coverage-state={line.state}
                            >
                                <span className="min-w-0 flex-1 basis-64">
                                    <span className="block text-xs text-fg-subtle">{lineKindWords(t, line)}</span>
                                    <span className="block break-all font-mono text-fg">{line.name || '—'}</span>
                                    <span className="mt-0.5 block text-xs text-fg-muted">{lineWords(t, locale, line)}</span>
                                    {next && <span className="mt-1 block text-xs text-fg" data-coverage-next="">{next}</span>}
                                </span>
                                <span className="flex shrink-0 flex-wrap items-center justify-end gap-3 max-sm:w-full max-sm:justify-start">
                                    <span className={`inline-flex items-center gap-1.5 font-medium ${waiting ? 'text-warning' : STATE_TONE[line.state]}`}>
                                        <Icon className="h-4 w-4" aria-hidden="true" />
                                        {t(`certcover.state.${waiting ? line.reason : line.state}` as TranslationKey)}
                                    </span>
                                    {fix && <RowActions name={line.name} actions={[fix]} />}
                                </span>
                            </li>
                        );
                    })}
                </ul>
            )}
        </div>
    );
}
