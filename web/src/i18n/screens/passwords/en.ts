// SCREEN catalogue part "passwords" (alpha.127): the shared password field
// (components/PasswordInput.tsx) - show/hide, generate, copy, the strength
// meter and the owner's password rule - and the server's PASSWORD_WEAK
// refusal, sentence for sentence the same as internal/passwordpolicy. Its own
// small part so no other part's budget carries it.
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// "passwords" ekran parçası: ortak parola alanı, güç göstergesi, sahibin
// parola kuralı ve sunucunun PASSWORD_WEAK reti.

export const enPasswordsScreens = {
    'pw.show': 'Show password',
    'pw.hide': 'Hide password',
    'pw.generate': 'Generate a strong password',
    'pw.copy': 'Copy',
    'pw.copied': 'Copied',
    'pw.copyFailed': 'Could not copy. Select the password and copy it yourself.',
    'pw.generated': 'A strong password is in the field. Copy it and keep it somewhere safe before you save.',
    'pw.rule': 'At least {min} characters, with an uppercase letter, a lowercase letter and a special character (for example - _ . !).',
    'pw.strength': 'Strength: {level}',
    'pw.level.tooWeak': 'Too weak',
    'pw.level.acceptable': 'Acceptable',
    'pw.level.strong': 'Strong',
    'pw.level.veryStrong': 'Very strong',
    'pw.missing': 'Still needed: {list}.',
    'pw.need.invalid_text': 'no line breaks, tabs or other control characters',
    'pw.need.too_short': 'at least {min} characters',
    'pw.need.too_long': 'at most {max} bytes',
    'pw.need.needs_upper': 'an uppercase letter',
    'pw.need.needs_lower': 'a lowercase letter',
    'pw.need.needs_special': 'a special character (for example - _ . !)',
    'pw.mismatch': 'The two passwords are not the same.',
    // The server's refusal: the same sentences as passwordpolicy.Message.
    'err.PASSWORD_WEAK': 'Password does not meet the rule. Nothing was changed.',
    'err.PASSWORD_WEAK.invalid_text': 'Password cannot contain line breaks, tabs or other control characters. Nothing was changed.',
    'err.PASSWORD_WEAK.too_short': 'Password must be at least {min} characters. Nothing was changed.',
    'err.PASSWORD_WEAK.too_long': 'Password can be at most {max} bytes. Nothing was changed.',
    'err.PASSWORD_WEAK.needs_upper': 'Password must contain an uppercase letter. Nothing was changed.',
    'err.PASSWORD_WEAK.needs_lower': 'Password must contain a lowercase letter. Nothing was changed.',
    'err.PASSWORD_WEAK.needs_special': 'Password must contain a special character (not a letter or digit, e.g. - _ . !). Nothing was changed.',
} as const;

export type PasswordsScreenKey = keyof typeof enPasswordsScreens;
