import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Checking, CouldNotCheck, inputClass } from './ui';
import { useRemote } from '../lib/remote';
import type { EditedPlan, PlanKind } from '../lib/rights';
import { unitText } from '../lib/settingLimits';

// The plan form's limits of site settings (alpha.128, docs/SITE-LIMITS.md):
// one number per key, empty = "up to the level above" (the server's limit for
// the administrator, the reseller's own for a reseller, read from
// GET /api/v1/rights/setting-limits). A customer plan also chooses what
// happens when a limit is passed ("Close" or "Notify only") and the notice
// levels; a reseller plan chooses whether it may hand out more than its pool.
//
// Plan formunun site ayarı sınırları: anahtar başına bir sayı, boş = üst
// düzeye kadar. Müşteri planı aşım kuralını, bayi planı fazla dağıtımı seçer.

export interface GrantorLimit {
    key: string;
    unit: string;
    floor?: boolean;
    ceiling: number | null;
    min: number;
    max: number;
}

function decodeLimits(raw: unknown): { scope: string; limits: GrantorLimit[] } {
    if (!raw || typeof raw !== 'object' || !Array.isArray((raw as { limits?: unknown }).limits)) throw new Error('shape');
    return raw as { scope: string; limits: GrantorLimit[] };
}

export function PlanLimitsSection({ kind, plan, onPlan, idPrefix }: { kind: PlanKind; plan: EditedPlan; onPlan: (next: EditedPlan) => void; idPrefix: string }) {
    const { t } = useI18n();
    const grantor = useRemote('/api/v1/rights/setting-limits', decodeLimits);
    const ceilings = plan.ceilings ?? {};
    const setCeiling = (key: string, raw: string) => {
        const next = { ...ceilings };
        if (raw.trim() === '') delete next[key];
        else next[key] = Math.max(0, Math.floor(Number(raw)));
        onPlan({ ...plan, ceilings: next });
    };
    const policy = plan.overuse_policy || 'close';
    const overselling = plan.overselling || 'allowed';
    return (
        <fieldset className="mt-4 rounded-lg border border-border p-3" data-plan-limits="">
            <legend className="px-1 text-sm font-semibold text-fg">{t('limits.plan.title')}</legend>
            {grantor.remote.state === 'loading' && <Checking label={t('limits.checking')} />}
            {grantor.remote.state === 'unknown' && <CouldNotCheck text={t('limits.unread')} onRetry={() => void grantor.retry()} busy={grantor.reading} />}
            {grantor.remote.state === 'known' && (
                <>
                    <p className="mb-2 max-w-[75ch] text-xs text-fg-subtle">
                        {t('limits.plan.hint', { level: grantor.remote.value.scope === 'reseller' ? t('limits.plan.levelReseller') : t('limits.plan.levelServer') })}
                    </p>
                    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                        {grantor.remote.value.limits.map((limit) => {
                            const id = `${idPrefix}-ceiling-${limit.key.replace(/\W/g, '-')}`;
                            const above = limit.ceiling !== null ? unitText(t, limit.unit, limit.ceiling) : t('limits.plan.none');
                            const value = ceilings[limit.key];
                            const outside = value !== undefined && (value < limit.min || value > limit.max ||
                                (limit.ceiling !== null && (limit.floor ? value < limit.ceiling : value > limit.ceiling)));
                            return (
                                <label key={limit.key} htmlFor={id} className="block">
                                    <span className="mb-1 block text-xs text-fg-muted">{t(`limits.key.${limit.key}` as TranslationKey)}</span>
                                    <input id={id} type="number" min={limit.min} max={limit.ceiling ?? limit.max} value={value ?? ''}
                                        placeholder={t('limits.plan.upTo', { value: above })} onChange={(e) => setCeiling(limit.key, e.target.value)}
                                        className={inputClass} aria-invalid={outside || undefined} />
                                    {outside && <span className="mt-1 block text-xs text-danger-fg">{limit.floor ? t('limits.php.belowMin', { min: above }) : t('limits.php.aboveLimit', { limit: above })}</span>}
                                </label>
                            );
                        })}
                    </div>
                </>
            )}
            {kind === 'customer' ? (
                <div className="mt-4">
                    <p className="mb-1 text-xs font-semibold text-fg">{t('limits.plan.overuse.title')}</p>
                    {(['close', 'notify'] as const).map((option) => (
                        <label key={option} className="flex items-start gap-2 py-1 text-sm text-fg">
                            <input type="radio" name={`${idPrefix}-overuse`} checked={policy === option} onChange={() => onPlan({ ...plan, overuse_policy: option })} className="mt-1" />
                            <span>{t(`limits.plan.overuse.${option}` as TranslationKey)}</span>
                        </label>
                    ))}
                    <div className="mt-2 grid grid-cols-1 gap-3 sm:grid-cols-2">
                        {(['notify_percent_disk', 'notify_percent_traffic'] as const).map((field) => (
                            <label key={field} className="block">
                                <span className="mb-1 block text-xs text-fg-muted">{t(field === 'notify_percent_disk' ? 'limits.plan.notifyDisk' : 'limits.plan.notifyTraffic')}</span>
                                <input type="number" min={50} max={95} value={plan[field] ?? 80} className={inputClass}
                                    onChange={(e) => onPlan({ ...plan, [field]: Math.min(95, Math.max(50, Number(e.target.value) || 80)) })} />
                            </label>
                        ))}
                    </div>
                    <p className="mt-1 text-xs text-fg-subtle">{t('limits.plan.notifyRange')}</p>
                </div>
            ) : (
                <div className="mt-4">
                    <p className="mb-1 text-xs font-semibold text-fg">{t('limits.plan.overselling.title')}</p>
                    {(['allowed', 'not_allowed'] as const).map((option) => (
                        <label key={option} className="flex items-start gap-2 py-1 text-sm text-fg">
                            <input type="radio" name={`${idPrefix}-overselling`} checked={overselling === option} onChange={() => onPlan({ ...plan, overselling: option })} className="mt-1" />
                            <span>{t(`limits.plan.overselling.${option}` as TranslationKey)}</span>
                        </label>
                    ))}
                </div>
            )}
        </fieldset>
    );
}
