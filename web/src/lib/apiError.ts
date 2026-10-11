import type { TranslationKey } from '../i18n/en';

// The API error contract (B1, Jul 18): every error body is one JSON shape —
// {error, code?, action?}. `code` is a stable machine constant for a
// deliberate refusal; the UI prefers its localized text (`err.<CODE>`) over
// the server's English message. `action` is an in-panel path that fixes the
// refusal; its button label is `err.<CODE>.action`.
//
// API hata sözleşmesi (B1, 18 Tem): her hata gövdesi tek JSON biçimidir —
// {error, code?, action?}. `code` bilinçli bir reddin sabit makine
// sabitidir; UI, sunucunun İngilizce mesajı yerine yerelleştirilmiş metnini
// (`err.<CODE>`) tercih eder. `action` reti düzelten panel-içi yoldur;
// düğme etiketi `err.<CODE>.action` anahtarıdır.

export interface ApiError {
    message: string;
    code?: string;
    action?: string;
    // reason refines code. The server works out which of several things caused
    // a coded refusal and says so; a screen that has words for that reason uses
    // them, and one that does not falls back to the sentence for the code,
    // which is what every screen did before this existed.
    // reason, code'u inceltir. O gerekçe için sözü olan ekran onu kullanır;
    // olmayan, kodun cümlesine döner.
    reason?: string;
    // detail refines reason with one reviewed machine token: for a crontab that
    // could not be read, the cause the server verified itself (`cron_allow`,
    // `cron_deny`). A screen uses it only for the tokens it has words for.
    // The server's own printed line is not this; it travels in `vars.detail`.
    // detail, reason'ı gözden geçirilmiş tek bir makine belirteciyle inceltir.
    // Sunucunun kendi yazdığı satır bu değildir; o `vars.detail` içindedir.
    detail?: string;
    // These flags are proof-bearing outcome fields, not synonyms. In
    // particular, partial_success alone must never be treated as proof that a
    // host mutation happened; only mutation_applied === true carries that
    // meaning.
    partialSuccess?: boolean;
    mutationApplied?: boolean;
    // details: the refusal's evidence, one display line per item — e.g. the
    // sites that block removing a runtime version (B3d). Additive: absent on
    // older responses, safely ignored by older screens.
    // details: retin kanıtı, kalem başına bir görüntü satırı — örn. bir
    // runtime sürümünün kaldırılmasını engelleyen siteler (B3d). Eklemeli:
    // eski cevaplarda yok, eski ekranlar güvenle yok sayar.
    details?: string[];
    // vars: named values the localized sentence for code needs, e.g. the
    // blocking directory for HOSTING_ROOT_NOT_TRAVERSABLE. Additive; only
    // string values are kept.
    // vars: kodun yerel cümlesinin ihtiyaç duyduğu adlı değerler. Eklemeli.
    vars?: Record<string, string>;
    // restore: after a service refused CelikPanel's text for a site file,
    // whether putting the previous file back was confirmed ('confirmed') or
    // not ('unconfirmed'). Additive; an unconfirmed restore selects the
    // sentence `err.<CODE>[.<reason>].unconfirmed`.
    // restore: önceki dosyanın geri konduğu doğrulandı mı. Eklemeli.
    restore?: string;
    // service: after such a put-back, what happened to the service
    // ('reloaded', 'not_reloaded', 'not_running', 'reload_failed');
    // serviceReason refines not_reloaded ('config_refused_elsewhere',
    // 'check_unavailable'); serviceActive is whether it runs now ('true',
    // 'false', 'unknown'). Additive (set12 cell RP1); a confirmed restore
    // whose service was not reloaded or not running selects the sentence
    // `siteConfig.service.<file kind>.<service>[.<serviceReason>]` (screen half).
    service?: string;
    serviceReason?: string;
    serviceActive?: string;
}

function stringVars(value: unknown): Record<string, string> | undefined {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
    const vars: Record<string, string> = {};
    for (const [name, item] of Object.entries(value)) {
        if (typeof item === 'string') vars[name] = item;
    }
    return Object.keys(vars).length > 0 ? vars : undefined;
}

// readApiError tolerates all three generations of error bodies: the coded
// JSON envelope, legacy plain text, and an empty body. This is THE one way
// to read an error response — do not hand-roll res.text()/res.json().
// readApiError üç kuşak hata gövdesini de tolere eder: kodlu JSON zarf,
// eski düz metin ve boş gövde. Hata cevabı okumanın TEK yolu budur —
// elle res.text()/res.json() yazmayın.
export async function readApiError(res: Response): Promise<ApiError> {
    try {
        const text = (await res.text()).trim();
        if (!text) return { message: '' };
        try {
            const d = JSON.parse(text);
            if (d && typeof d === 'object' && ('error' in d || 'code' in d)) {
                return {
                    message: d.error || '',
                    code: d.code,
                    action: d.action,
                    reason: typeof d.reason === 'string' && d.reason ? d.reason : undefined,
                    detail: typeof d.detail === 'string' && d.detail ? d.detail : undefined,
                    partialSuccess: d.partial_success === true ? true : undefined,
                    mutationApplied: d.mutation_applied === true ? true : undefined,
                    details: Array.isArray(d.details)
                        ? d.details.filter((item: unknown): item is string => typeof item === 'string')
                        : undefined,
                    vars: stringVars(d.vars),
                    restore: typeof d.restore === 'string' && d.restore ? d.restore : undefined,
                    service: typeof d.service === 'string' && d.service ? d.service : undefined,
                    serviceReason: typeof d.service_reason === 'string' && d.service_reason ? d.service_reason : undefined,
                    serviceActive: typeof d.service_active === 'string' && d.service_active ? d.service_active : undefined,
                };
            }
        } catch {
            /* legacy plain text / eski düz metin */
        }
        return { message: text };
    } catch {
        return { message: '' };
    }
}

type T = (key: TranslationKey, vars?: Record<string, string | number>) => string;

// apiErrorText prefers the localized text of a coded refusal and falls back
// to the server message, then to a generic error.
// apiErrorText, kodlu reddin yerelleştirilmiş metnini tercih eder; sunucu
// mesajına, o da yoksa genel hataya düşer.
export function apiErrorText(e: ApiError, t: T, fallbackKey: TranslationKey = 'common.error'): string {
    if (e.code) {
        // The refined sentence first: a refusal that named which of several
        // causes it was deserves the words for that cause, not the words that
        // cover all of them. R-068 is the case this was written for - "another
        // server change or package-manager task" was one sentence for three
        // situations, one of which does not end by waiting.
        // Önce inceltilmiş cümle: hangi sebep olduğunu adlandıran bir ret, o
        // sebebin sözlerini hak eder; hepsini kapsayanları değil.
        // A restore that was not confirmed has its own sentence: the one for
        // the confirmed case would say the previous file is back.
        if (e.restore === 'unconfirmed') {
            const keys = [e.reason ? 'err.' + e.code + '.' + e.reason + '.unconfirmed' : '', 'err.' + e.code + '.unconfirmed'];
            for (const candidate of keys) {
                if (!candidate) continue;
                const named = t(candidate as TranslationKey, e.vars);
                if (named !== candidate) return named;
            }
            // No sentence for it: the server's own words say it.
            if (e.message) return e.message;
        }
        // The previous file is back and the service was deliberately not
        // reloaded (its check refused the configuration elsewhere) or was
        // not running: the sentence for the confirmed case would say it
        // keeps running with the file. A sentence that names the check's
        // line is used only when the answer carries it (administrators).
        // These sentences belong to the site-file screens (the screen half
        // of the catalogue, `siteConfig.service.<file kind>.<service>…`): such
        // a refusal is only answered on a screen, and while the screen half
        // is not loaded the server's own sentence stands.
        const fileKind = e.code === 'SITE_CONFIG_NGINX_REFUSED' ? 'nginx_vhost' : e.code === 'SITE_CONFIG_SERVICE_REFUSED' ? e.reason : undefined;
        if (fileKind && e.restore === 'confirmed' && (e.service === 'not_reloaded' || e.service === 'not_running')) {
            const prefix = 'siteConfig.service.' + fileKind + '.' + e.service;
            const keys = e.serviceReason ? [prefix + '.' + e.serviceReason, prefix] : [prefix];
            for (const candidate of keys) {
                const named = t(candidate as TranslationKey, e.vars);
                if (named !== candidate && !named.includes('{line}')) return named;
            }
            if (e.message) return e.message;
        }
        if (e.reason) {
            const refined = ('err.' + e.code + '.' + e.reason) as TranslationKey;
            const named = t(refined, e.vars);
            if (named !== refined) return named;
        }
        const key = ('err.' + e.code) as TranslationKey;
        const s = t(key, e.vars);
        if (s !== key) return s;
    }
    return e.message || t(fallbackKey);
}

// apiErrorActionLabel: the fix-it button's label for a coded refusal that
// carries an action path; '' when there is nothing to render.
// apiErrorActionLabel: action yolu taşıyan kodlu ret için düğme etiketi;
// çizilecek bir şey yoksa ''.
export function apiErrorActionLabel(e: ApiError, t: T): string {
    if (!e.code || !e.action) return '';
    const key = ('err.' + e.code + '.action') as TranslationKey;
    const s = t(key);
    return s !== key ? s : t('common.goFix');
}
