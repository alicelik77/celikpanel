import type { TranslationKey } from '../i18n/en';

// What a mail profile is called and what it says, in the interface language.
// The server sends its three compiled profiles with an English name and
// description, and its blocked and warning sentences as English text with a
// typed code beside them (blocked_code with blocked_vars, warning_code;
// cmd/panel/mail_profiles.go, 2026-10-14). A Turkish page showed the English
// text (sixteenth record, alpha.84). The page says a code in its own language;
// the server's text is shown only when no code came (an older Panel) or the
// code is one this page has no words for. The last attempt's error is the
// server's own words, introduced by a translated lead-in.
//
// Bir posta profilinin adı ve söyledikleri, arayüz dilinde. Sunucu engel ve
// uyarı cümlelerini İngilizce metin ve yanında türlü bir kodla gönderir; sayfa
// kodu kendi dilinde söyler, metni yalnız kod gelmediğinde ya da bilmediği bir
// kod olduğunda gösterir.

type Translate = (key: TranslationKey, vars?: Record<string, string | number>) => string;

const profileWords: Record<string, { name: TranslationKey; description: TranslationKey }> = {
    'core-mail': { name: 'dashboard.audit.profile.coreMail', description: 'services.mailProfiles.description.coreMail' },
    webmail: { name: 'dashboard.audit.profile.webmail', description: 'services.mailProfiles.description.webmail' },
    'protected-mail': { name: 'dashboard.audit.profile.protectedMail', description: 'services.mailProfiles.description.protectedMail' },
};

interface ProfileText {
    id: string;
    name: string;
    description?: string;
    blocked_reason?: string;
    blocked_code?: unknown;
    blocked_vars?: unknown;
    warning?: string;
    warning_code?: unknown;
    latest_attempt_error?: string;
}

export function mailProfileName(profile: ProfileText, t: Translate): string {
    const words = profileWords[profile.id];
    return words ? t(words.name) : profile.name;
}

export function mailProfileDescription(profile: ProfileText, t: Translate): string {
    const words = profileWords[profile.id];
    return words ? t(words.description) : profile.description ?? '';
}

// The sentence of a code, or null when there is no code or no words for it.
// Only string values are used; a missing value leaves its sentence unused.
// A code may have a fuller sentence for an optional value: component_not_installable
// names the platform when the Panel sends one (2026-10-15); without it (an
// unverified platform, an older Panel) the shorter sentence stands.
// Bir kodun isteğe bağlı bir değer için daha dolu bir cümlesi olabilir.
const fullerSentence: Record<string, { value: string; key: TranslationKey }> = {
    component_not_installable: { value: 'platform', key: 'services.mailProfiles.code.component_not_installable.platform' },
};
// A requirement may name a role instead of a component ("web-server": any
// installed web server satisfies it; cmd/panel mailProfileServiceName sends
// the token itself). The Panel joins what is missing with ", " in `required`.
// Each role token is said by its display name; a component name (already a
// product name) is left as it is; a role token this page has no name for
// (a newer Panel) is said as words, never as the raw token.
//
// Gereksinim bileşen yerine bir rol adı verebilir ("web-server"). Her rol adı
// görünen adıyla söylenir; bileşen adı olduğu gibi kalır; bilinmeyen rol adı
// sözcük olarak söylenir, ham belirteç olarak değil.
const requirementRoles = ['web-server', 'smtp-server', 'imap-server', 'spam-filter', 'dns-server', 'kv-store'] as const;
// More than one requirement is said as a list with "and" before the last one,
// and the sentence takes its plural form (browser batch 14, 2026-10-15:
// "requires Redis, key-value store, mail relay host, which is not part of
// this profile").
// Birden fazla gereksinim "ve" ile biten bir liste olarak söylenir.
export function requirementItems(required: string, t: Translate): string[] {
    return required.split(', ').filter(Boolean).map((token) => {
        if (!/^[a-z]+(-[a-z]+)+$/.test(token)) return token;
        if ((requirementRoles as readonly string[]).includes(token)) {
            return t(`services.mailProfiles.role.${token}` as TranslationKey);
        }
        return token.replace(/-/g, ' ');
    });
}
export function requirementText(required: string, t: Translate): string {
    const items = requirementItems(required, t);
    if (items.length < 2) return items.join('');
    return `${items.slice(0, -1).join(', ')} ${t('services.mailProfiles.requirementAnd')} ${items[items.length - 1]}`;
}

function codeSentence(code: unknown, vars: unknown, t: Translate): string | null {
    if (typeof code !== 'string' || !/^[a-z_]+$/.test(code)) return null;
    const values: Record<string, string> = {};
    if (vars && typeof vars === 'object') {
        for (const [name, value] of Object.entries(vars as Record<string, unknown>)) {
            if (typeof value === 'string') values[name] = value;
        }
    }
    const say = (key: TranslationKey): string | null => {
        const said = t(key, values);
        return said === key || /\{\w+\}/.test(said) ? null : said;
    };
    if (code === 'component_requires' && values.required) {
        const several = requirementItems(values.required, t).length > 1;
        values.required = requirementText(values.required, t);
        if (several) {
            const said = say('services.mailProfiles.code.component_requires.many');
            if (said) return said;
        }
    }
    const fuller = fullerSentence[code];
    if (fuller && values[fuller.value]?.trim()) {
        const said = say(fuller.key);
        if (said) return said;
    }
    return say(`services.mailProfiles.code.${code}` as TranslationKey);
}

export function mailProfileBlocked(profile: ProfileText, t: Translate): string {
    return codeSentence(profile.blocked_code, profile.blocked_vars, t) ?? profile.blocked_reason ?? '';
}

export function mailProfileWarning(profile: ProfileText, t: Translate): string {
    return codeSentence(profile.warning_code, undefined, t) ?? profile.warning ?? '';
}

export function mailProfileAttemptError(profile: ProfileText, t: Translate): string {
    const error = profile.latest_attempt_error?.trim();
    return error ? t('services.mailProfiles.attemptError', { error }) : '';
}
