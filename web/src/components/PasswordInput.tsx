import { useId, useState, type InputHTMLAttributes } from 'react';
import { Check, Copy, Eye, EyeOff, Wand2 } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import {
    generatePassword,
    passwordProblems,
    passwordStrength,
    passwordWindow,
    type PasswordField,
    type PasswordStrength,
    type PasswordWindow,
} from '../lib/passwordPolicy';
import { inputClass } from './ui';

// The one password field of the panel (owner, 6 Nov 2026). Every password
// input is this component (tests/password-input-guard.test.mjs keeps it so):
// the eye shows and hides the text everywhere. A NEW password (`field` set)
// also gets the generator, the copy button, the owner's rule before anything
// is typed, a strength meter and, while the rule is not met, the rules still
// missing - the form disables its submit with passwordAcceptable(). Sign-in,
// "current password" and existing secrets (tokens, keys, a provider's
// password) pass no `field`: show/hide only.
//
// Panelin tek parola alanı. Yeni parolada üretme, kopyalama, kural, güç
// göstergesi ve eksik kurallar; mevcut sırlarda yalnız göster/gizle.

type NativeProps = Omit<InputHTMLAttributes<HTMLInputElement>, 'type' | 'value' | 'onChange' | 'defaultValue' | 'className'>;

export interface PasswordInputProps extends NativeProps {
    value: string;
    onChange: (value: string) => void;
    /** A new password: its field's window turns on the rule and the meter. */
    field?: PasswordField | PasswordWindow;
    /** The generator; on by default for a new password. */
    generate?: boolean;
    /** Called with a generated password (to fill a confirmation field too). */
    onGenerated?: (password: string) => void;
    /** Extra classes for the input (width, font). */
    className?: string;
}

const levels: PasswordStrength[] = ['tooWeak', 'acceptable', 'strong', 'veryStrong'];
const barTone: Record<PasswordStrength, string> = {
    tooWeak: 'bg-danger',
    acceptable: 'bg-warning',
    strong: 'bg-success',
    veryStrong: 'bg-success',
};
const textTone: Record<PasswordStrength, string> = {
    tooWeak: 'text-danger',
    acceptable: 'text-warning',
    strong: 'text-success',
    veryStrong: 'text-success',
};
const iconButton =
    'flex h-7 w-7 items-center justify-center rounded-md text-fg-subtle transition-colors hover:text-fg focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary disabled:opacity-40';

export function PasswordInput({
    value,
    onChange,
    field,
    generate,
    onGenerated,
    className = '',
    id,
    autoComplete,
    disabled,
    readOnly,
    'aria-describedby': describedBy,
    ...rest
}: PasswordInputProps) {
    const { t } = useI18n();
    const ownId = useId();
    const inputId = id ?? ownId;
    const ruleId = inputId + '-rule';
    const [visible, setVisible] = useState(false);
    const [copied, setCopied] = useState<'yes' | 'failed' | null>(null);
    const [generated, setGenerated] = useState(false);
    const isNew = field !== undefined;
    const canGenerate = (generate ?? isNew) && !readOnly;
    const canCopy = (generate ?? isNew) && value !== '';
    const locked = disabled || readOnly;

    const make = () => {
        const password = generatePassword();
        onChange(password);
        onGenerated?.(password);
        setVisible(true);
        setGenerated(true);
        setCopied(null);
    };
    const copy = async () => {
        try {
            await navigator.clipboard.writeText(value);
            setCopied('yes');
        } catch {
            setCopied('failed');
        }
    };
    const buttons = 1 + (canGenerate ? 1 : 0) + (canCopy ? 1 : 0);
    const padding = buttons === 3 ? 'pr-24' : buttons === 2 ? 'pr-16' : 'pr-9';

    return (
        <div className="min-w-0">
            <div className="relative">
                <input
                    {...rest}
                    id={inputId}
                    type={visible ? 'text' : 'password'}
                    value={value}
                    onChange={(e) => {
                        onChange(e.target.value);
                        setGenerated(false);
                        setCopied(null);
                    }}
                    autoComplete={autoComplete ?? (isNew ? 'new-password' : 'off')}
                    autoCapitalize="off"
                    spellCheck={false}
                    disabled={disabled}
                    readOnly={readOnly}
                    aria-describedby={[describedBy, isNew ? ruleId : ''].filter(Boolean).join(' ') || undefined}
                    className={`${inputClass} ${padding} ${className}`}
                />
                <div className="absolute inset-y-0 right-1 flex items-center gap-0.5">
                    {canGenerate && (
                        <button type="button" className={iconButton} onClick={make} disabled={locked} aria-label={t('pw.generate')} title={t('pw.generate')}>
                            <Wand2 className="h-4 w-4" aria-hidden="true" />
                        </button>
                    )}
                    {canCopy && (
                        <button type="button" className={iconButton} onClick={() => void copy()} disabled={disabled} aria-label={t(copied === 'yes' ? 'pw.copied' : 'pw.copy')} title={t(copied === 'yes' ? 'pw.copied' : 'pw.copy')}>
                            {copied === 'yes' ? <Check className="h-4 w-4 text-success" aria-hidden="true" /> : <Copy className="h-4 w-4" aria-hidden="true" />}
                        </button>
                    )}
                    <button
                        type="button"
                        className={iconButton}
                        onClick={() => setVisible((v) => !v)}
                        disabled={disabled}
                        aria-pressed={visible}
                        aria-controls={inputId}
                        aria-label={t(visible ? 'pw.hide' : 'pw.show')}
                        title={t(visible ? 'pw.hide' : 'pw.show')}
                    >
                        {visible ? <EyeOff className="h-4 w-4" aria-hidden="true" /> : <Eye className="h-4 w-4" aria-hidden="true" />}
                    </button>
                </div>
            </div>
            {isNew && <PasswordMeter id={ruleId} value={value} field={field} />}
            <p className="sr-only" aria-live="polite">{copied === 'yes' ? t('pw.copied') : ''}</p>
            {copied === 'failed' && <p className="mt-1 text-xs text-danger">{t('pw.copyFailed')}</p>}
            {generated && copied !== 'failed' && <p className="mt-1 text-xs text-fg-muted">{t('pw.generated')}</p>}
        </div>
    );
}

// The rule before anything is typed; then the meter, and while the rule is
// not met the rules still missing (the form's submit stays disabled).
function PasswordMeter({ id, value, field }: { id: string; value: string; field: PasswordField | PasswordWindow }) {
    const { t } = useI18n();
    const { min, max } = passwordWindow(field);
    if (value === '') {
        return <p id={id} className="mt-1 text-xs text-fg-subtle">{t('pw.rule', { min })}</p>;
    }
    const level = passwordStrength(value, field);
    const rank = levels.indexOf(level);
    const missing = passwordProblems(value, field).map((rule) => t(('pw.need.' + rule) as TranslationKey, { min, max }));
    return (
        <div id={id} className="mt-1.5 space-y-1">
            <div className="flex items-center gap-2">
                <div className="flex flex-1 gap-1" aria-hidden="true">
                    {levels.map((l, i) => (
                        <span key={l} className={`h-1 flex-1 rounded-full ${i <= rank ? barTone[level] : 'bg-border'}`} />
                    ))}
                </div>
                <span className={`shrink-0 text-xs font-medium ${textTone[level]}`}>{t('pw.strength', { level: t(('pw.level.' + level) as TranslationKey) })}</span>
            </div>
            {missing.length > 0 && <p className="text-xs text-danger">{t('pw.missing', { list: missing.join(', ') })}</p>}
        </div>
    );
}
