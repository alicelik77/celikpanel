// CelikPanel's default HTTPS server, as the Nginx page shows it (alpha.113).
// The Agent reads its state from nginx's own configuration; reading never
// changes anything. "Set up now" / "Try again" is one POST with a request
// identity (D-029), sent only when the person presses it.
//
// CelikPanel'in varsayılan HTTPS sunucusu, Nginx sayfasında. Okuma hiçbir
// şeyi değiştirmez; "Şimdi kur" / "Yeniden dene" basıldığında tek POST'tur.

export const NGINX_DEFAULT_HTTPS_URL = '/api/v1/nginx/default-https';

export const DEFAULT_HTTPS_STATES = ['active', 'owner_default', 'owner_file', 'not_needed', 'pending', 'failed', 'unknown'] as const;
export type DefaultHTTPSState = (typeof DEFAULT_HTTPS_STATES)[number];

export interface DefaultHTTPSStatus {
    state: DefaultHTTPSState;
    mode: 'reject' | 'placeholder' | '';
    nginxVersion: string;
    path: string;
    ownerFile: string;
    detail: string;
    lastAt: string;
}

export interface DefaultHTTPSAnswer {
    outcome: string;
    status: DefaultHTTPSStatus;
}

const text = (value: unknown, field: string): string => {
    if (value === undefined) return '';
    if (typeof value !== 'string') throw new Error(field);
    return value;
};

// A state this page does not know is not drawn as one it does: it is unknown.
// Bu sayfanın bilmediği durum, bildiği biri gibi çizilmez.
export function decodeDefaultHTTPS(raw: unknown): DefaultHTTPSAnswer {
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) throw new Error('shape');
    const body = raw as { outcome?: unknown; status?: unknown };
    const status = body.status;
    if (!status || typeof status !== 'object' || Array.isArray(status)) throw new Error('status');
    const s = status as Record<string, unknown>;
    const state = text(s.state, 'state');
    if (!(DEFAULT_HTTPS_STATES as readonly string[]).includes(state)) throw new Error('state');
    const mode = text(s.mode, 'mode');
    if (mode !== '' && mode !== 'reject' && mode !== 'placeholder') throw new Error('mode');
    return {
        outcome: text(body.outcome, 'outcome'),
        status: {
            state: state as DefaultHTTPSState,
            mode,
            nginxVersion: text(s.nginx_version, 'nginx_version'),
            path: text(s.path, 'path'),
            ownerFile: text(s.owner_file, 'owner_file'),
            detail: text(s.detail, 'detail'),
            lastAt: text(s.last_at, 'last_at'),
        },
    };
}

/** The tone of the state's line: done, needs a person, or not known. */
export function defaultHTTPSTone(state: DefaultHTTPSState): 'ok' | 'info' | 'warn' | 'bad' | 'unknown' {
    switch (state) {
        case 'active':
            return 'ok';
        case 'not_needed':
        case 'owner_default':
        case 'owner_file':
            return 'info';
        case 'pending':
            return 'warn';
        case 'failed':
            return 'bad';
        default:
            return 'unknown';
    }
}

/** Only these states have a change to offer; every other state only reads. */
export function defaultHTTPSOffersSetup(state: DefaultHTTPSState): boolean {
    return state === 'pending' || state === 'failed';
}
