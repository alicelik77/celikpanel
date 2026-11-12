// What a domain's certificate protects (alpha.134, docs/CERTIFICATE-COVERAGE.md):
// the answer of GET /api/v1/domains/{id}/ssl/coverage, its one decoder, and
// the words of a line. The certificate screen's list and the mail screen's
// sentence under the server name both say a line through lineWords, so the
// same state never reads two ways. No React here.
//
// Alan adının sertifikasının neyi koruduğu: yanıt, çözücüsü ve bir satırın
// sözleri. Sertifika ekranının listesi ve posta ekranının cümlesi aynı yerden konuşur.
import type { TranslationKey } from '../../i18n/en';
import { formatDate } from '../../lib/dateFormat';

export type CoverageKind = 'site' | 'webmail' | 'mail_programs' | 'mail_name' | 'mail_setup';
export type CoverageState = 'protected' | 'not_protected' | 'unknown';
export type CoverageBy = 'domain_certificate' | 'server_mail_certificate' | 'webmail_certificate';
export type CoverageAction = 'reissue' | 'cover_mail' | 'upload' | 'use_for_mail' | 'dns' | 'mail_setup';

export interface CoverageLine {
    kind: CoverageKind;
    role?: 'domain' | 'www' | 'alias';
    name: string;
    state: CoverageState;
    by?: CoverageBy;
    until?: string;
    reason?: string;
    ips: string[];
    action?: CoverageAction;
}

export interface Coverage {
    lines: CoverageLine[];
    mail_name: string;
}

export const coverageURL = (domainId: number) => `/api/v1/domains/${domainId}/ssl/coverage`;

const KINDS: readonly string[] = ['site', 'webmail', 'mail_programs', 'mail_name', 'mail_setup'];
const STATES: readonly string[] = ['protected', 'not_protected', 'unknown'];
const BY: readonly string[] = ['domain_certificate', 'server_mail_certificate', 'webmail_certificate'];
const ACTIONS: readonly string[] = ['reissue', 'cover_mail', 'upload', 'use_for_mail', 'dns', 'mail_setup'];
const REASONS: readonly string[] = [
    'no_certificate', 'not_in_certificate', 'points_elsewhere', 'no_address', 'certificate_not_usable',
    'mail_not_using', 'server_certificate_not_ready', 'mail_name_unknown', 'webmail_no_certificate', 'not_checked',
];

/**
 * One line, or null when it is not the contract. A line that is "protected"
 * without a known certificate, or "not protected" without a known reason, is
 * not believed: it reads as not known.
 */
export function decodeCoverageLine(raw: unknown): CoverageLine | null {
    const value = raw as Record<string, unknown> | null;
    if (!value || typeof value !== 'object' || typeof value.name !== 'string' ||
        typeof value.kind !== 'string' || !KINDS.includes(value.kind) ||
        typeof value.state !== 'string' || !STATES.includes(value.state)) return null;
    const line: CoverageLine = {
        kind: value.kind as CoverageKind,
        name: value.name,
        state: value.state as CoverageState,
        ips: Array.isArray(value.ips) ? value.ips.filter((ip): ip is string => typeof ip === 'string') : [],
    };
    if (value.role === 'domain' || value.role === 'www' || value.role === 'alias') line.role = value.role;
    if (typeof value.by === 'string' && BY.includes(value.by)) line.by = value.by as CoverageBy;
    if (typeof value.until === 'string' && Number.isFinite(new Date(value.until).getTime())) line.until = value.until;
    if (typeof value.reason === 'string' && REASONS.includes(value.reason)) line.reason = value.reason;
    if (typeof value.action === 'string' && ACTIONS.includes(value.action)) line.action = value.action as CoverageAction;
    if ((line.state === 'protected' && (!line.by || !line.until)) || (line.state === 'not_protected' && !line.reason)) {
        return { ...line, state: 'unknown', reason: 'not_checked', by: undefined, until: undefined, action: undefined };
    }
    return line;
}

export function decodeCoverage(raw: unknown): Coverage {
    const body = raw as Record<string, unknown> | null;
    if (!body || typeof body !== 'object' || !Array.isArray(body.lines) || typeof body.mail_name !== 'string') throw new Error('coverage');
    return {
        lines: body.lines.map(decodeCoverageLine).filter((line): line is CoverageLine => line !== null),
        mail_name: body.mail_name,
    };
}

type Say = (key: TranslationKey, values?: Record<string, string | number>) => string;

/** What the line is ("Web site", "Mail programs (the name they are told)"). */
export function lineKindWords(t: Say, line: CoverageLine): string {
    if (line.kind === 'site') return t(`certcover.kind.${line.role ?? 'alias'}` as TranslationKey);
    return t(`certcover.kind.${line.kind}` as TranslationKey);
}

/** The line's sentence: by which certificate until when, or the reason. */
export function lineWords(t: Say, locale: 'tr' | 'en', line: CoverageLine): string {
    if (line.state === 'protected' && line.by && line.until) {
        return t(`certcover.by.${line.by}` as TranslationKey, { date: formatDate(new Date(line.until), locale) });
    }
    return t(`certcover.reason.${line.reason ?? 'not_checked'}` as TranslationKey, { ips: line.ips.join(', ') });
}

/**
 * The mail screen's sentence under the server name: the same line, said as
 * "A trusted certificate protects this name (…, until …)" or its reason.
 */
export function mailHostWords(t: Say, locale: 'tr' | 'en', line: CoverageLine | null): string {
    if (!line || line.state === 'unknown') return t('certcover.mailHost.unknown');
    if (line.state === 'protected' && line.by && line.until && line.by !== 'webmail_certificate') {
        return t(`certcover.mailHost.protected.${line.by}` as TranslationKey, { date: formatDate(new Date(line.until), locale) });
    }
    return lineWords(t, locale, line);
}
