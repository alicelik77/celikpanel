import { readApiError, type ApiError } from './apiError';
import { unansweredCause } from './lostAnswer';

// The certificate that "Enable SSL" asks for when a site is added.
//
// Creating a site never issues a certificate (the web server starts without
// one, because none exists yet). When the person ticked SSL, Add domain sends
// ONE request right after the site exists, to the same route the SSL/TLS page
// uses for its "Request certificate" button: /api/v1/domains/<id>/ssl/letsencrypt.
// The same server code answers, so the names (the domain and its www name),
// the account contact address, the validation, the failure kinds and the
// activation are the ones the SSL page shows.
//
// This is a single request per click on "Create domain". The route carries a
// request identity (D-029): the page's fetch layer adds it, a lost answer is
// asked for once more under the same identity and never run twice. Nothing
// here polls or retries; the outcome is what the one request came to.
// Contract: docs/OPERATION-GUIDANCE.md, D-024.
//
// Site eklerken "SSL etkinleştir"in istediği sertifika. Site oluşturmak
// sertifika çıkarmaz. Kişi SSL'i işaretlediyse, site oluşunca SSL/TLS sayfasının
// "Sertifika iste" düğmesinin kullandığı rotaya TEK istek gider; aynı sunucu
// kodu yanıtlar. İstek kimlik taşır (D-029): yeniden çalıştırılmaz.

export type SiteCertificateOutcome =
    /** Issued and served. */
    | { kind: 'issued' }
    /** Issued and stored; the site's configuration file is the owner's and does not use it yet. */
    | { kind: 'waitingForOwner' }
    /** Issued and stored; the web server could not start using it. Retry activation on the SSL page. */
    | { kind: 'activationPending' }
    /** Active on the website; mail TLS still has to follow. Retry activation on the SSL page. */
    | { kind: 'dependentsPending' }
    /** The server answered and refused, or the request ended in a failure it can name. Nothing was issued. */
    | { kind: 'failed'; error: ApiError }
    /** No answer of its own arrived: whether a certificate was issued is not known. */
    | { kind: 'unknown' };

const certificateRoute = (domainId: number) => `/api/v1/domains/${domainId}/ssl/letsencrypt`;

/**
 * Asks once for the free certificate of a site that was just created. The
 * contact address is left out on purpose: the server uses the signed-in
 * account's own address and refuses with a pointer to My account when it has
 * none. Never throws.
 */
export async function requestSiteCertificate(domainId: number): Promise<SiteCertificateOutcome> {
    const url = certificateRoute(domainId);
    const init: RequestInit = {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        // The SSL page's defaults: Let's Encrypt, renewed automatically, the
        // mail name not included. SSL was ticked when the site was added, so
        // once this first certificate is active http:// redirects to
        // https:// (the owner can turn that off on the SSL/TLS page).
        body: JSON.stringify({ auto_renew: true, include_mail: false, provider: 'letsencrypt', reissue: false, redirect_http: true }),
    };
    let res: Response | null = null;
    try {
        res = await fetch(url, init);
    } catch {
        res = null;
    }
    // A dropped connection, a gateway's page, or the Panel's own word that the
    // result is not known yet: neither a success nor a failure.
    if (await unansweredCause('POST', url, res)) return { kind: 'unknown' };
    if (res === null) return { kind: 'unknown' };
    if (!res.ok) {
        const error = await readApiError(res);
        if (error.code === 'ssl_activation_pending') return { kind: 'activationPending' };
        if (error.code === 'ssl_dependents_pending') return { kind: 'dependentsPending' };
        return { kind: 'failed', error };
    }
    try {
        const body: unknown = await res.json();
        const status = body && typeof body === 'object' ? (body as { status?: unknown }).status : undefined;
        return status === 'waiting_for_owner' ? { kind: 'waitingForOwner' } : { kind: 'issued' };
    } catch {
        // The request went through and its answer cannot be read: not known.
        return { kind: 'unknown' };
    }
}

/** certbot ran and did not fulfil the request (cmd/panel/certificate_issue_failure.go). */
export const isIssueFailure = (error: ApiError): boolean => error.code === 'CERTIFICATE_ISSUE_FAILED';

/** The sentence for each kind of failure; the same sentences the SSL page shows. */
export const ISSUE_FAILURE_KEYS = {
    authority_unreachable: 'ssl.issueFailure.authority_unreachable',
    validation: 'ssl.issueFailure.validation',
    rate_limited: 'ssl.issueFailure.rate_limited',
    timeout: 'ssl.issueFailure.timeout',
    tool: 'ssl.issueFailure.tool',
    server_clock: 'ssl.issueFailure.server_clock',
} as const;
