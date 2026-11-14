import { useEffect, useRef } from 'react';
import { useAuth } from '../auth/AuthContext';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import type { ApiError } from '../lib/apiError';
import { Guidance } from './kit/rows/Guidance';
import { whoWord } from './kit/words';
import { Button } from './ui';

// A certificate request that did not give the site a new certificate
// (11 Oct 2026; D-024, cmd/panel/certificate_issue_failure.go). The Panel
// answers CERTIFICATE_ISSUE_FAILED with the kind as `reason`: the authority
// could not be reached, refused the proof of a name, reached one of its
// limits, does not know this server's account, certbot ran out of time or
// ended without a reason that could be established, this server's clock does
// not accept the issued certificate, or the issued certificate did not pass
// the check before use (`vars.check`).
//
// alpha.134 lab: the sentence sent a customer to a log file on the server.
// Now it is the one guidance block: what happened (the current certificate
// stays in use), why, who acts for the reader's role, the next step and how
// it goes on. A customer or reseller is told what they can do themselves or
// to contact the server's administrator; certbot's own line and where certbot
// keeps its record are closed technical details for an administrator only
// (the Panel sends them to nobody else).
//
// Sertifika isteği siteye yeni sertifika vermedi. Ne oldu, neden, okuyanın
// rolüne göre kim yapar, sonraki adım ve sonra; teknik ayrıntı yalnız yöneticiye.
export function isCertificateIssueFailure(error: ApiError): boolean {
    return error.code === 'CERTIFICATE_ISSUE_FAILED';
}

type Who = 'you' | 'administrator' | 'nobody';

// For each kind: who acts and the next step. `admin` is the next step when
// it is the server administrator's; everyone else is then told to contact
// them (`ask`) or to try again later and contact them if it repeats (`later`).
const kinds: Record<string, { who: Who; next?: TranslationKey; admin?: TranslationKey; other?: TranslationKey }> = {
    validation: { who: 'you', next: 'ssl.issueFailure.next.validation' },
    rate_limited: { who: 'nobody', next: 'ssl.issueFailure.next.rate_limited' },
    authority_unreachable: { who: 'administrator', admin: 'ssl.issueFailure.next.authority_unreachable', other: 'ssl.issueFailure.next.ask' },
    account_unknown: { who: 'administrator', admin: 'ssl.issueFailure.next.account_unknown', other: 'ssl.issueFailure.next.ask' },
    timeout: { who: 'administrator', admin: 'ssl.issueFailure.next.record', other: 'ssl.issueFailure.next.later' },
    tool: { who: 'administrator', admin: 'ssl.issueFailure.next.record', other: 'ssl.issueFailure.next.later' },
    server_clock: { who: 'administrator', admin: 'ssl.issueFailure.next.server_clock', other: 'ssl.issueFailure.next.ask' },
    'issued_check.untrusted': { who: 'administrator', admin: 'ssl.issueFailure.next.untrusted', other: 'ssl.issueFailure.next.ask' },
    'issued_check.names': { who: 'administrator', admin: 'ssl.issueFailure.next.panelLog', other: 'ssl.issueFailure.next.later' },
    'issued_check.unusable': { who: 'administrator', admin: 'ssl.issueFailure.next.panelLog', other: 'ssl.issueFailure.next.later' },
};

const whys: Record<string, TranslationKey> = {
    validation: 'ssl.issueFailure.validation',
    rate_limited: 'ssl.issueFailure.rate_limited',
    authority_unreachable: 'ssl.issueFailure.authority_unreachable',
    account_unknown: 'ssl.issueFailure.account_unknown',
    timeout: 'ssl.issueFailure.timeout',
    tool: 'ssl.issueFailure.tool',
    server_clock: 'ssl.issueFailure.server_clock',
    'issued_check.untrusted': 'ssl.issueFailure.untrusted',
    'issued_check.names': 'ssl.issueFailure.names',
    'issued_check.unusable': 'ssl.issueFailure.unusable',
};

/** The kind a failure is said as: a reason this page has no words for is the general one. */
function issueFailureKind(failure: ApiError): string {
    const reason = failure.reason ?? '';
    const kind = reason === 'issued_check' ? `issued_check.${failure.vars?.check ?? 'unusable'}` : reason;
    return kinds[kind] ? kind : reason === 'issued_check' ? 'issued_check.unusable' : 'tool';
}

export function CertificateIssueNotice({
    failure,
    onClose,
    domain = '',
    then,
    className,
}: {
    failure: ApiError | null;
    /** Without it the block has no Close button (it is part of a result the page shows). */
    onClose?: () => void;
    /** The domain when the answer does not name it. */
    domain?: string;
    /** How it goes on, when the request is made somewhere else than here. */
    then?: string;
    className?: string;
}) {
    const { t } = useI18n();
    const { role } = useAuth();
    const box = useRef<HTMLDivElement>(null);
    useEffect(() => {
        if (failure) box.current?.scrollIntoView?.({ block: 'nearest' });
    }, [failure]);
    if (!failure) return null;
    const admin = role === 'admin';
    const kind = issueFailureKind(failure);
    const plan = kinds[kind];
    const vars = { ...failure.vars, domain: failure.vars?.domain || domain };
    const next = plan.next ?? (admin ? plan.admin : plan.other) ?? 'ssl.issueFailure.next.ask';
    // The server's administrator reading it is the one who acts; anyone else
    // acts first when the next step is theirs (asking again later), and is
    // otherwise told that the administrator does.
    const who: Who = plan.who !== 'administrator' ? plan.who : admin || next === 'ssl.issueFailure.next.later' ? 'you' : 'administrator';
    const detail = admin ? failure.vars?.detail : undefined;
    const record = admin ? failure.vars?.record : undefined;
    return (
        <div ref={box} data-certificate-issue={kind} className={className}>
            <Guidance
                tone="failed"
                title={t(failure.vars?.kept === 'previous' ? 'ssl.issueFailure.title.previous' : 'ssl.issueFailure.title.none', vars)}
                why={t(whys[kind], vars)}
                who={whoWord(t, who)}
                next={t(next, vars)}
                then={then ?? t('ssl.issueFailure.then')}
                action={onClose ? <Button type="button" onClick={onClose}>{t('common.close')}</Button> : undefined}
            >
                {(detail || record) && (
                    <details className="text-xs text-fg-muted">
                        <summary className="cursor-pointer">{t('common.technicalDetails')}</summary>
                        {detail && <p className="mt-1 break-words">{t('ssl.issueFailure.said', { detail })}</p>}
                        {record && <p className="mt-1 break-words">{t('ssl.issueFailure.record', { record })}</p>}
                    </details>
                )}
            </Guidance>
        </div>
    );
}
