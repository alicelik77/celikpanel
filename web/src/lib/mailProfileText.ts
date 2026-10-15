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
