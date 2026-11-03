import { useId } from 'react';
import { useI18n } from '../../i18n';
import type { GuideRole } from './types';

// The administrator reads the guide of another role ("view as"). Only the
// chapters change: the guide is public and the role is relevance, not access.
// Yönetici başka bir rolün kılavuzunu okur; yalnız bölümler değişir.
const roles: GuideRole[] = ['admin', 'reseller', 'customer'];

export function ViewAsSwitch({ value, onChange }: { value: GuideRole; onChange: (role: GuideRole) => void }) {
    const { t } = useI18n();
    const name = useId();
    return (
        <fieldset className="flex flex-wrap items-center gap-x-3 gap-y-1.5" data-guide-view-as="">
            <legend className="sr-only">{t('guide.viewAs')}</legend>
            <span aria-hidden="true" className="text-xs text-fg-muted">{t('guide.viewAs')}</span>
            <div className="inline-flex rounded-lg border border-border-strong p-0.5">
                {roles.map((role) => (
                    <label
                        key={role}
                        className={`cursor-pointer rounded-md px-2.5 py-1 text-xs font-medium transition-colors focus-within:ring-2 focus-within:ring-primary max-sm:min-h-11 max-sm:inline-flex max-sm:items-center ${
                            value === role ? 'bg-primary text-primary-fg' : 'text-fg-muted hover:bg-surface-2 hover:text-fg'
                        }`}
                    >
                        <input
                            type="radio"
                            name={name}
                            value={role}
                            checked={value === role}
                            onChange={() => onChange(role)}
                            className="sr-only"
                        />
                        {t(`role.${role}`)}
                    </label>
                ))}
            </div>
        </fieldset>
    );
}
