import { useEffect, useState } from 'react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { Checking, CouldNotCheck, inputClass } from './ui';
import { useRemote } from '../lib/remote';
import type { EditedPlan, PlanKind } from '../lib/rights';
import { unitText } from '../lib/settingLimits';
import { ChangedMark, changedRowClass } from './kit/save/SaveBar';

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

/** A field of the limits whose value is outside what it may be: its input's id and its name. */
export interface OutsideField { id: string; name: string }

const NOTIFY_FIELDS = ['notify_percent_disk', 'notify_percent_traffic'] as const;
const notifyOutside = (value: number) => value < 50 || value > 95;

export function PlanLimitsSection({ kind, plan, onPlan, idPrefix, ceilingsOff = false, base, onOutside }: {
    kind: PlanKind; plan: EditedPlan; onPlan: (next: EditedPlan) => void; idPrefix: string; ceilingsOff?: boolean;
    /** alpha.132: the plan as it was opened; given, what differs from it is marked as an unsaved change. */
    base?: EditedPlan;
    /**
     * alpha.133: the first field whose value is outside its range or above
     * the level above (null: none). The form names it beside Save and keeps
     * Save off until it is corrected, as the site's PHP limits do.
     */
    onOutside?: (field: OutsideField | null) => void;
}) {
    const { t } = useI18n();
    const marked = (changed: boolean) => (base && changed ? `-mx-2 rounded-r px-2 ${changedRowClass}` : '');
    const grantor = useRemote(ceilingsOff ? null : '/api/v1/rights/setting-limits', decodeLimits);
    const ceilings = plan.ceilings ?? {};
    const setCeiling = (key: string, raw: string) => {
        const next = { ...ceilings };
        if (raw.trim() === '') delete next[key];
        else next[key] = Math.max(0, Math.floor(Number(raw)));
        onPlan({ ...plan, ceilings: next });
    };
    const policy = plan.overuse_policy || 'close';
    const overselling = plan.overselling || 'allowed';
    const ceilingId = (key: string) => `${idPrefix}-ceiling-${key.replace(/\W/g, '-')}`;
    const ceilingOutside = (limit: GrantorLimit, value: number | undefined) => value !== undefined && (value < limit.min || value > limit.max ||
        (limit.ceiling !== null && (limit.floor ? value < limit.ceiling : value > limit.ceiling)));
    // The bound the value passes, in words: the level above when it sets
    // one, else the key's own range ("at most 1000", never "at most no limit").
    const outsideWords = (limit: GrantorLimit, value: number) => {
        if (value < limit.min) return t('limits.php.belowMin', { min: unitText(t, limit.unit, limit.min) });
        if (value > limit.max) return t('limits.php.aboveLimit', { limit: unitText(t, limit.unit, limit.max) });
        const level = unitText(t, limit.unit, limit.ceiling ?? 0);
        return limit.floor ? t('limits.php.belowMin', { min: level }) : t('limits.php.aboveLimit', { limit: level });
    };
    const notifyName = (field: (typeof NOTIFY_FIELDS)[number]) => t(field === 'notify_percent_disk' ? 'limits.plan.notifyDisk' : 'limits.plan.notifyTraffic');
    // The first field out of range, in the order shown: the limits, then the notice levels.
    const known = !ceilingsOff && grantor.remote.state === 'known' ? grantor.remote.value.limits : [];
    const ceilingOut = known.find((limit) => ceilingOutside(limit, ceilings[limit.key]));
    const notifyOut = kind === 'customer' ? NOTIFY_FIELDS.find((field) => notifyOutside(plan[field] ?? 80)) : undefined;
    const outside: OutsideField | null = ceilingOut
        ? { id: ceilingId(ceilingOut.key), name: t(`limits.key.${ceilingOut.key}` as TranslationKey) }
        : notifyOut ? { id: `${idPrefix}-${notifyOut}`, name: notifyName(notifyOut) } : null;
    useEffect(() => { onOutside?.(outside); }, [outside?.id, outside?.name]); // eslint-disable-line react-hooks/exhaustive-deps
    // Gone with the form (a Custom set switched to a plan): nothing is outside any more.
    useEffect(() => () => onOutside?.(null), []); // eslint-disable-line react-hooks/exhaustive-deps
    return (
        <fieldset className="mt-4 rounded-lg border border-border p-3" data-plan-limits="">
            <legend className="px-1 text-sm font-semibold text-fg">{t(ceilingsOff ? 'limits.plan.overselling.title' : 'limits.plan.title')}</legend>
            {!ceilingsOff && grantor.remote.state === 'loading' && <Checking label={t('limits.checking')} />}
            {!ceilingsOff && grantor.remote.state === 'unknown' && <CouldNotCheck text={t('limits.unread')} onRetry={() => void grantor.retry()} busy={grantor.reading} />}
            {!ceilingsOff && grantor.remote.state === 'known' && (
                <>
                    <p className="mb-2 max-w-[75ch] text-xs text-fg-subtle">
                        {t('limits.plan.hint', { level: grantor.remote.value.scope === 'reseller' ? t('limits.plan.levelReseller') : t('limits.plan.levelServer') })}
                    </p>
                    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                        {grantor.remote.value.limits.map((limit) => {
                            const id = ceilingId(limit.key);
                            const above = limit.ceiling !== null ? unitText(t, limit.unit, limit.ceiling) : t('limits.plan.none');
                            const value = ceilings[limit.key];
                            const outside = ceilingOutside(limit, value);
                            return (
                                <label key={limit.key} htmlFor={id} className={`block ${marked(value !== (base?.ceilings ?? {})[limit.key])}`}>
                                    <span className="mb-1 block text-xs text-fg-muted">{t(`limits.key.${limit.key}` as TranslationKey)}{base && value !== (base.ceilings ?? {})[limit.key] && <ChangedMark />}</span>
                                    <input id={id} type="number" min={limit.min} max={limit.ceiling ?? limit.max} value={value ?? ''}
                                        placeholder={t('limits.plan.upTo', { value: above })} onChange={(e) => setCeiling(limit.key, e.target.value)}
                                        className={`${inputClass} field-sm`} aria-invalid={outside || undefined} />
                                    {outside && value !== undefined && <span className="mt-1 block text-xs text-danger">{outsideWords(limit, value)}</span>}
                                </label>
                            );
                        })}
                    </div>
                </>
            )}
            {kind === 'customer' ? (
                <div className="mt-4">
                    <p className="mb-1 text-xs font-semibold text-fg">{t('limits.plan.overuse.title')}{base && policy !== (base.overuse_policy || 'close') && <ChangedMark />}</p>
                    {(['close', 'notify'] as const).map((option) => (
                        <label key={option} className={`flex items-start gap-2 py-1 text-sm text-fg ${marked(policy === option && policy !== (base?.overuse_policy || 'close'))}`}>
                            <input type="radio" name={`${idPrefix}-overuse`} checked={policy === option} onChange={() => onPlan({ ...plan, overuse_policy: option })} className="mt-1" />
                            <span>{t(`limits.plan.overuse.${option}` as TranslationKey)}</span>
                        </label>
                    ))}
                    <div className="mt-2 grid grid-cols-1 gap-3 sm:grid-cols-2">
                        {NOTIFY_FIELDS.map((field) => {
                            // The number as typed (an empty field is 0): a value
                            // out of 50-95 is marked, keeps Save off and is never
                            // changed under the typing.
                            const value = plan[field] ?? 80;
                            const out = notifyOutside(value);
                            return (
                                <label key={field} htmlFor={`${idPrefix}-${field}`} className={`block ${marked(value !== (base?.[field] ?? 80))}`}>
                                    <span className="mb-1 block text-xs text-fg-muted">{notifyName(field)}{base && value !== (base[field] ?? 80) && <ChangedMark />}</span>
                                    <input id={`${idPrefix}-${field}`} type="number" min={50} max={95} value={value || ''} className={`${inputClass} field-xs`} aria-invalid={out || undefined}
                                        onChange={(e) => onPlan({ ...plan, [field]: Math.max(0, Math.floor(Number(e.target.value) || 0)) })} />
                                </label>
                            );
                        })}
                    </div>
                    <p className={`mt-1 text-xs ${NOTIFY_FIELDS.some((field) => notifyOutside(plan[field] ?? 80)) ? 'text-danger' : 'text-fg-subtle'}`}>{t('limits.plan.notifyRange')}</p>
                </div>
            ) : (
                <div className="mt-4">
                    <p className="mb-1 text-xs font-semibold text-fg">{t('limits.plan.overselling.title')}{base && overselling !== (base.overselling || 'allowed') && <ChangedMark />}</p>
                    {(['allowed', 'not_allowed'] as const).map((option) => (
                        <label key={option} className={`flex items-start gap-2 py-1 text-sm text-fg ${marked(overselling === option && overselling !== (base?.overselling || 'allowed'))}`}>
                            <input type="radio" name={`${idPrefix}-overselling`} checked={overselling === option} onChange={() => onPlan({ ...plan, overselling: option })} className="mt-1" />
                            <span>{t(`limits.plan.overselling.${option}` as TranslationKey)}</span>
                        </label>
                    ))}
                </div>
            )}
        </fieldset>
    );
}

function ownOf(kind: PlanKind) {
    return (raw: unknown): { ceilings: Record<string, number>; overselling?: string } => {
        const body = decodeLimits(raw) as { limits: (GrantorLimit & { set_by?: string })[]; overselling?: unknown };
        const ceilings: Record<string, number> = {};
        for (const limit of body.limits) {
            // A plan-bound subscription that becomes Custom starts from its
            // plan's limits (the server copies them too); a reseller's own
            // level is marked "reseller" (its reseller plan's or its own).
            const own = kind === 'customer' ? limit.set_by === 'subscription' || limit.set_by === 'plan' : limit.set_by === 'reseller';
            if (own && limit.ceiling !== null) ceilings[limit.key] = limit.ceiling;
        }
        return { ceilings, overselling: typeof body.overselling === 'string' ? body.overselling : undefined };
    };
}

/**
 * The limits of a Custom set (alpha.128): a Custom subscription's own
 * ceilings, overuse policy and notice levels, or a Custom reseller's own
 * ceilings and overselling choice, edited like a plan's and read from the
 * account's effective limits. `onChange` gets only what was touched, so an
 * untouched form sends nothing and keeps what is stored.
 */
export function CustomLimitsSection({ kind, subscriptionId, resellerId, idPrefix, onChange, onOutside }: {
    kind: PlanKind; subscriptionId?: number; resellerId?: number; idPrefix: string;
    onChange: (extras: Record<string, unknown> | null) => void;
    onOutside?: (field: OutsideField | null) => void;
}) {
    const url = kind === 'customer'
        ? (subscriptionId ? `/api/v1/rights/setting-limits?subscription_id=${subscriptionId}` : null)
        : (resellerId ? `/api/v1/rights/setting-limits?reseller_id=${resellerId}` : null);
    const own = useRemote(url, ownOf(kind));
    const [edited, setEdited] = useState<EditedPlan | null>(null);
    if (own.remote.state !== 'known') return null;
    const base: EditedPlan = { id: 0, name: '', numbers: { max_customers: 0, max_domains: 0, max_databases: 0, max_email_accounts: 0, disk_quota_mb: 0, traffic_quota_mb: 0 }, bandwidth: 0,
        ceilings: own.remote.value.ceilings, overselling: own.remote.value.overselling };
    const plan = edited ?? base;
    return (
        <PlanLimitsSection kind={kind} plan={plan} idPrefix={idPrefix} onOutside={onOutside} onPlan={(next) => {
            setEdited(next);
            const extras: Record<string, unknown> = { ceilings: next.ceilings ?? {} };
            if (kind === 'customer') {
                if (next.overuse_policy) extras.overuse_policy = next.overuse_policy;
                if (next.notify_percent_disk) extras.notify_percent_disk = next.notify_percent_disk;
                if (next.notify_percent_traffic) extras.notify_percent_traffic = next.notify_percent_traffic;
            } else if (next.overselling) {
                extras.overselling = next.overselling;
            }
            onChange(extras);
        }} />
    );
}
