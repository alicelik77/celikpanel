// The owner's password rule (6 Nov 2026), the browser half. Every new password
// the panel sets has at least 6 characters (or its field's stricter minimum),
// an uppercase letter, a lowercase letter and a special character (anything
// that is not a letter or a digit). The server checks the same rule in
// internal/passwordpolicy/policy.go and refuses with PASSWORD_WEAK; both
// halves are tested against internal/passwordpolicy/testdata, so they cannot
// drift. Sign-in and "current password" fields are never checked.
//
// Sahibin parola kurali (6 Kas 2026), tarayici yarisi. Sunucu ayni kurali
// internal/passwordpolicy/policy.go icinde denetler; iki yari ayni test
// verisini okur.

export type PasswordRule = 'invalid_text' | 'too_short' | 'too_long' | 'needs_upper' | 'needs_lower' | 'needs_special';

export interface PasswordWindow {
    /** Characters (code points); never below the owner's 6. */
    min: number;
    /** UTF-8 bytes, what the service receives; 0 means no maximum. */
    max: number;
}

export const basePasswordMin = 6;

// The same table as passwordpolicy.Fields (and testdata/fields.json).
export const passwordFields = {
    account: { min: 8, max: 1024 },
    mailbox: { min: 8, max: 1024 },
    database_user: { min: 12, max: 128 },
    file_access: { min: 8, max: 128 },
    site_protect: { min: 8, max: 72 },
    service_app_owner: { min: 12, max: 128 },
} as const satisfies Record<string, PasswordWindow>;

export type PasswordField = keyof typeof passwordFields;

export function passwordWindow(field: PasswordField | PasswordWindow): PasswordWindow {
    const w = typeof field === 'string' ? passwordFields[field] : field;
    return { min: Math.max(basePasswordMin, w.min), max: w.max };
}

const upper = /\p{Lu}/u;
const lower = /\p{Ll}/u;
const digit = /\p{Nd}/u;
// A control character, U+FFFD (what the server reads for text that was not
// valid UTF-8) or a lone surrogate (which arrives as U+FFFD).
const invalid = /[\p{Cc}\uFFFD]|\p{Cs}/u;
const special = /[^\p{L}\p{Nd}\p{Cc}\p{Cs}\uFFFD]/u;

/** The rules a password breaks, in the server's order; empty when it meets them. */
export function passwordProblems(password: string, field: PasswordField | PasswordWindow): PasswordRule[] {
    const { min, max } = passwordWindow(field);
    const out: PasswordRule[] = [];
    if (invalid.test(password)) out.push('invalid_text');
    if ([...password].length < min) out.push('too_short');
    if (max > 0 && new TextEncoder().encode(password).byteLength > max) out.push('too_long');
    if (!upper.test(password)) out.push('needs_upper');
    if (!lower.test(password)) out.push('needs_lower');
    if (!special.test(password)) out.push('needs_special');
    return out;
}

export function passwordAcceptable(password: string, field: PasswordField | PasswordWindow): boolean {
    return passwordProblems(password, field).length === 0;
}

export type PasswordStrength = 'tooWeak' | 'acceptable' | 'strong' | 'veryStrong';

/** Below the rule: too weak (refused). Meets it: acceptable; 12 or more with
 * a digit: strong; 16 or more with all four kinds: very strong. */
export function passwordStrength(password: string, field: PasswordField | PasswordWindow): PasswordStrength {
    if (!passwordAcceptable(password, field)) return 'tooWeak';
    const length = [...password].length;
    if (!digit.test(password) || length < 12) return 'acceptable';
    return length >= 16 ? 'veryStrong' : 'strong';
}

// Letters without look-alikes, digits without 0/1, and the three symbols no
// shell, URL, connection string or config file reads specially.
const letters = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz';
const digits = '23456789';
const symbols = '-_.';
const alphabet = letters + digits + symbols;

function draw(set: string): string {
    // Rejection sampling keeps every character equally likely.
    const limit = Math.floor(0x100000000 / set.length) * set.length;
    const buf = new Uint32Array(1);
    for (;;) {
        crypto.getRandomValues(buf);
        if (buf[0] < limit) return set[buf[0] % set.length];
    }
}

/**
 * A password from the browser's cryptographic generator (not a plain pseudo-random one):
 * 20 characters by default, always meets the rule and has digits too (so it
 * reads "Very strong"), and never starts or ends with a symbol.
 */
export function generatePassword(length = 20): string {
    const n = Math.max(16, length);
    for (;;) {
        let out = draw(letters + digits);
        for (let i = 1; i < n - 1; i++) out += draw(alphabet);
        out += draw(letters + digits);
        if (passwordStrength(out, { min: n, max: 0 }) === 'veryStrong') return out;
    }
}
