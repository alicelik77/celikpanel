import { Layers } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { RemoteGate, UsageBar } from './ui';
import { useRemote } from '../lib/remote';
import { MY_SUBSCRIPTION_URL, decodeMyPlan, usagePercent, type MyPlan, type MyPlanSubscription } from '../lib/myPlan';

// "My plan" (alpha.114): the signed-in customer's or reseller's plan, its
// limits and the live usage the quota checks use — the same numbers a
// refusal is based on — and whom to ask for a larger plan. The reason and the
// next step come first; the meters follow. Traffic is not measured yet and
// says so instead of showing a limit.
//
// "Planım": müşterinin ya da bayinin planı, sınırları ve kota denetimlerinin
// kullandığı canlı kullanım; daha büyük plan için kime başvurulacağı.

function fmtMB(mb: number): string {
    return mb >= 1024 ? `${(mb / 1024).toFixed(mb % 1024 === 0 ? 0 : 1)} GB` : `${mb} MB`;
}

function Meter({ label, used, limit, usedText, limitText }: { label: string; used: number; limit: number; usedText: string; limitText: string }) {
    const { t } = useI18n();
    const pct = usagePercent(used, limit);
    return (
        <div>
            <div className="flex items-baseline justify-between gap-2 text-sm">
                <span className="text-fg-muted">{label}</span>
                <span className="font-medium text-fg">
                    {pct === null ? t('myPlan.usedUnlimited', { used: usedText }) : t('myPlan.usedOf', { used: usedText, limit: limitText })}
                </span>
            </div>
            {pct !== null && <div className="mt-1"><UsageBar percent={pct} /></div>}
            {pct !== null && pct >= 80 && <p className="mt-0.5 text-xs text-danger">{used >= limit ? t('myPlan.full') : t('myPlan.nearlyFull')}</p>}
        </div>
    );
}

function SubscriptionBlock({ sub }: { sub: MyPlanSubscription }) {
    const { t } = useI18n();
    const u = sub.usage;
    const mb = 1024 * 1024;
    return (
        <div className="rounded-lg border border-border p-4" data-my-plan-subscription={sub.id}>
            <p className="text-base font-semibold text-fg">{sub.plan ? sub.plan.name : t('myPlan.noPlanName')}</p>
            {sub.over_limit.length > 0 && (
                <div className="mt-2 rounded-md bg-danger/10 p-3 text-sm text-danger">
                    <p className="font-semibold">{t('myPlan.over')}</p>
                    <ul className="mt-1 list-disc pl-5">{sub.over_limit.map((line) => <li key={line}>{line}</li>)}</ul>
                </div>
            )}
            <div className="mt-3 grid gap-3 sm:grid-cols-2">
                <Meter label={t('plans.col.domains')} used={u.domains} limit={u.domains_limit} usedText={String(u.domains)} limitText={String(u.domains_limit)} />
                <Meter label={t('plans.col.databases')} used={u.databases} limit={u.databases_limit} usedText={String(u.databases)} limitText={String(u.databases_limit)} />
                <Meter label={t('plans.col.mail')} used={u.mail_accounts} limit={u.mail_limit} usedText={String(u.mail_accounts)} limitText={String(u.mail_limit)} />
                <Meter
                    label={t('plans.col.disk')}
                    used={u.disk_used_bytes}
                    limit={u.disk_limit_bytes}
                    usedText={fmtMB(Math.round(u.disk_used_bytes / mb))}
                    limitText={fmtMB(Math.round(u.disk_limit_bytes / mb))}
                />
            </div>
        </div>
    );
}

function contactText(plan: MyPlan, t: (key: TranslationKey, vars?: Record<string, string | number>) => string): string | null {
    const c = plan.contact;
    if (c.kind === 'none') return null;
    const who = [c.name, c.email ? `(${c.email})` : ''].filter(Boolean).join(' ');
    return c.kind === 'reseller' ? t('myPlan.contactReseller', { who }) : t('myPlan.contactAdmin', { who });
}

export function MyPlanCard() {
    const { t } = useI18n();
    const read = useRemote(MY_SUBSCRIPTION_URL, decodeMyPlan);
    return (
        <section id="plan" className="scroll-mt-24 rounded-xl border border-border bg-surface p-5" data-account-section="plan">
            <div className="mb-3 flex items-center gap-2">
                <Layers className="h-4 w-4 text-primary" aria-hidden="true" />
                <h2 className="text-base font-semibold text-fg">{t('myPlan.title')}</h2>
            </div>
            <RemoteGate
                remote={read.remote}
                checking={t('myPlan.checking')}
                failed={t('myPlan.unknown')}
                onRetry={() => void read.retry()}
                busy={read.reading}
            >
                {(shown) => {
                    const plan = shown.value;
                    const contact = contactText(plan, t);
                    return (
                        <div className="grid gap-3">
                            {plan.subscriptions.length === 0 ? (
                                <p className="text-sm text-fg-muted">{t('myPlan.none')}</p>
                            ) : (
                                plan.subscriptions.map((sub) => <SubscriptionBlock key={sub.id} sub={sub} />)
                            )}
                            {plan.pool && <PoolBlock pool={plan.pool} />}
                            {!plan.bandwidthMeasured && <p className="text-xs text-fg-subtle">{t('myPlan.trafficNotMeasured')}</p>}
                            {contact && <p className="text-sm text-fg-muted">{contact}</p>}
                        </div>
                    );
                }}
            </RemoteGate>
        </section>
    );
}

function PoolBlock({ pool }: { pool: NonNullable<MyPlan['pool']> }) {
    const { t } = useI18n();
    const a = pool.allocated;
    const l = pool.limits;
    return (
        <div className="rounded-lg border border-border p-4" data-my-plan-pool="">
            <p className="text-base font-semibold text-fg">{t('myPlan.pool')}</p>
            <p className="mt-1 text-xs text-fg-subtle">{l ? t('myPlan.poolHint') : t('myPlan.poolNone')}</p>
            {pool.over_limit.length > 0 && (
                <ul className="mt-2 list-disc pl-5 text-sm text-danger">{pool.over_limit.map((line) => <li key={line}>{line}</li>)}</ul>
            )}
            <div className="mt-3 grid gap-3 sm:grid-cols-2">
                <Meter label={t('myPlan.customers')} used={a.customers} limit={l?.max_customers ?? 0} usedText={String(a.customers)} limitText={String(l?.max_customers ?? 0)} />
                <Meter label={t('plans.col.domains')} used={a.unlimited_domains ? Infinity : a.domains} limit={l?.max_domains ?? 0} usedText={a.unlimited_domains ? '∞' : String(a.domains)} limitText={String(l?.max_domains ?? 0)} />
                <Meter label={t('plans.col.databases')} used={a.unlimited_databases ? Infinity : a.databases} limit={l?.max_databases ?? 0} usedText={a.unlimited_databases ? '∞' : String(a.databases)} limitText={String(l?.max_databases ?? 0)} />
                <Meter label={t('plans.col.mail')} used={a.unlimited_mailboxes ? Infinity : a.mailboxes} limit={l?.max_email_accounts ?? 0} usedText={a.unlimited_mailboxes ? '∞' : String(a.mailboxes)} limitText={String(l?.max_email_accounts ?? 0)} />
                <Meter label={t('plans.col.disk')} used={a.unlimited_disk ? Infinity : a.disk_mb} limit={l?.disk_quota_mb ?? 0} usedText={a.unlimited_disk ? '∞' : fmtMB(a.disk_mb)} limitText={fmtMB(l?.disk_quota_mb ?? 0)} />
            </div>
        </div>
    );
}
