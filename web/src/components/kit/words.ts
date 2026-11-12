import type { TranslationKey } from '../../i18n/en';
import type { StatusDotTone } from '../ui';

// Codes in words (alpha.132, principle P5 "no raw codes, ever"): what a site
// is and what state a domain is in, said the same way on every screen. A code
// this client does not know is never printed to a customer or a reseller: they
// read a neutral word, and only an administrator also sees the code in
// brackets, so a newer server's kind can still be recognised.
// Kodlar sözcükle: sitenin türü ve alan adının durumu her ekranda aynı
// sözcükle söylenir. Bilinmeyen kod müşteriye yazılmaz; yalnız yönetici
// nötr sözcüğün yanında kodu da görür.

type Translate = (key: TranslationKey, values?: Record<string, string | number>) => string;

// Every project_type the server stores (cmd/panel validates php, static, node,
// proxy, forwarding, service_app and runtime_app; dnsonly is a domain without
// a website). A missing value is PHP, as the server reads it
// (COALESCE(project_type, 'php')).
const KIND_KEYS: Record<string, TranslationKey> = {
    php: 'kind.php',
    static: 'kind.static',
    node: 'kind.node',
    proxy: 'kind.proxy',
    forwarding: 'kind.forwarding',
    service_app: 'kind.service',
    dnsonly: 'kind.dnsonly',
};

// The two kinds of an uploaded runtime application (runtime_apps.kind).
const RUNTIME_KEYS: Record<string, TranslationKey> = {
    program: 'kind.program',
    python: 'kind.python',
};

export interface WordOptions {
    /** The viewer is an administrator: an unknown code is shown in brackets. */
    admin?: boolean;
}

const unknownWord = (t: Translate, key: TranslationKey, code: string, admin?: boolean) =>
    admin && code ? `${t(key)} (${code})` : t(key);

/**
 * The kind of a site in words: "PHP site", "Static site", "DNS only" …
 * `runtimeKind` ('program' | 'python') names a runtime_app precisely when the
 * screen knows it; without it the word covers both.
 */
export function siteKindWord(t: Translate, projectType: string | null | undefined, options: WordOptions & { runtimeKind?: string } = {}): string {
    const code = projectType || 'php';
    if (code === 'runtime_app') {
        const precise = options.runtimeKind ? RUNTIME_KEYS[options.runtimeKind] : undefined;
        return t(precise ?? 'kind.runtime');
    }
    const key = KIND_KEYS[code];
    return key ? t(key) : unknownWord(t, 'kind.unknown', code, options.admin);
}

// Every domains.status the schema allows (active, suspended, pending; pending
// is a deletion that has not finished).
const STATE_KEYS: Record<string, TranslationKey> = {
    active: 'state.active',
    suspended: 'state.suspended',
    pending: 'state.pending',
};

const STATE_TONES: Record<string, StatusDotTone> = {
    active: 'success',
    suspended: 'danger',
    pending: 'unknown',
};

/** The state of a domain in words: "Active", "Suspended", "Deletion pending". */
export function stateWord(t: Translate, status: string | null | undefined, options: WordOptions = {}): string {
    const code = status ?? '';
    const key = STATE_KEYS[code];
    return key ? t(key) : unknownWord(t, 'state.unknown', code, options.admin);
}

/** The dot's tone that goes with stateWord (StatusDot / StateLabel). */
export function stateTone(status: string | null | undefined): StatusDotTone {
    return STATE_TONES[status ?? ''] ?? 'unknown';
}

/** Who acts, for the guidance block's "Who acts" row, when it is one of the three usual answers. */
export function whoWord(t: Translate, who: 'you' | 'administrator' | 'nobody'): string {
    return t(`kit.guidance.who.${who}`);
}
