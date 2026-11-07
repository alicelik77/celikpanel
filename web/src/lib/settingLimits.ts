import type { TranslationKey } from '../i18n/en';

// Words for limits of site settings (alpha.128): a value with its unit and
// the level that set a limit. Shared by the site's PHP screen and the plan
// form, so neither chunk pulls in the other.
// Site ayarı sınırlarının sözcükleri: birimli değer ve sınırı koyan düzey.

type Say = (key: TranslationKey, vars?: Record<string, string | number>) => string;

export function unitText(t: Say, unit: string, n: number): string {
    if (unit === 'mb') return n >= 1024 && n % 1024 === 0 ? `${n / 1024} GB` : t('limits.unit.mb', { n });
    if (unit === 's') return t('limits.unit.s', { n });
    if (unit === 'min') return t('limits.unit.min', { n });
    return t('limits.unit.count', { n });
}

export function whoText(t: Say, setBy?: string, name?: string): string {
    switch (setBy) {
        case 'server': return t('limits.who.server');
        case 'reseller': return name ? t('limits.who.resellerNamed', { name }) : t('limits.who.reseller');
        case 'plan': return name ? t('limits.who.planNamed', { name }) : t('limits.who.plan');
        case 'subscription': return t('limits.who.subscription');
    }
    return '';
}


/**
 * Sends one change of a limit or a site's PHP values. A connection that
 * breaks answers null: the result is not known, and the caller says so and
 * reads again; it never sends a second time.
 */
export async function sendSettings(url: string, method: 'PUT' | 'POST', body: unknown, headers: Record<string, string> = {}): Promise<Response | null> {
    let res: Response | null;
    try {
        res = await fetch(url, { method, credentials: 'same-origin', headers: { 'Content-Type': 'application/json', ...headers }, body: JSON.stringify(body) });
    } catch {
        res = null;
    }
    return res;
}

/** The fields of a successful answer the screen reads ({} when it carries none). */
export async function answerBody(res: Response): Promise<{ nginx?: string }> {
    let body: unknown;
    try {
        body = await res.json();
    } catch {
        body = null;
    }
    return body && typeof body === 'object' ? (body as { nginx?: string }) : {};
}
