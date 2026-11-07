import { Layers } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { RemoteGate, UsageBar } from './ui';
import { useRemote } from '../lib/remote';
import { MY_SUBSCRIPTION_URL, decodeMyPlan, usagePercent, type MyPlan, type MyPlanSubscription } from '../lib/myPlan';
import { overLimitText, stateName } from '../lib/rights';
import { ResellerPoolNotices } from './RightsNotices';
import { formatBytes } from '../lib/usage';

// "My plan" (alpha.114): the signed-in customer's or reseller's plan, its
// limits and the live usage the quota checks use — the same numbers a
// refusal is based on — and whom to ask for a larger plan. The reason and the
// next step come first; the meters follow. Traffic (alpha.118, D-044) is the
// websites' bytes this calendar month; a number that is not known says why.
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

const TRAFFIC_REASONS: Record<string, TranslationKey> = {
    not_collected_yet: 'myPlan.trafficReason.first',
    collection_failed: 'myPlan.trafficReason.failed',
};

/** This month's traffic: the meter, or why the number is not known. */
function TrafficLine({ sub, interval }: { sub: MyPlanSubscription; interval: number }) {
    const { t } = useI18n();
    const u = sub.usage;
    const measure = u.traffic_measure;
    if (!measure || measure.state === 'none') return null;
    if (measure.state === 'unknown') {
        const reason = TRAFFIC_REASONS[measure.reason ?? ''] ?? 'myPlan.trafficReason.site';
        return (
            <p className="text-sm text-fg-muted sm:col-span-2" data-my-plan-traffic="unknown">
                <span className="font-medium text-fg">{t('myPlan.traffic')}: </span>{t('myPlan.trafficUnknown', { reason: t(reason, { minutes: interval }) })}
            </p>
        );
    }
    const used = u.traffic_used_bytes ?? 0;
    const limit = u.traffic_limit_bytes ?? 0;
    return (
        <div data-my-plan-traffic="measuring">
            <Meter label={t('myPlan.traffic')} used={used} limit={limit} usedText={formatBytes(used)} limitText={formatBytes(limit)} />
        </div>
    );
}

/** The disk in parts, when it was measured, and the countdown when full. */
function DiskFacts({ sub }: { sub: MyPlanSubscription }) {
    const { t } = useI18n();
    const disk = sub.usage.disk;
    if (!disk) return null;
    return (
        <div className="text-xs text-fg-subtle sm:col-span-2" data-my-plan-disk={disk.state}>
            {disk.state === 'not_measured_yet' && <p>{t('myPlan.diskNotMeasured')}</p>}
            {disk.state === 'unknown' && <p>{t('myPlan.diskUnknown', { detail: disk.detail ?? '' })}</p>}
            {(disk.state === 'measured' || disk.state === 'partial') && (
                <p>{t('myPlan.diskParts', { web: formatBytes(disk.web_bytes), mail: formatBytes(disk.mail_bytes), db: formatBytes(disk.db_bytes) })}</p>
            )}
            {disk.state === 'partial' && disk.detail && <p>{t('myPlan.diskPartial', { detail: disk.detail })}</p>}
        </div>
    );
}

/** Where closing uploads on the server stands: pending, failed, or kept by the owner. */
function UploadNotes({ disk }: { disk: NonNullable<MyPlanSubscription['usage']['disk']> }) {
    const { t } = useI18n();
    const note = disk.uploads_note ?? '';
    const lines: string[] = [];
    if (note.startsWith('error:')) lines.push(t('myPlan.uploadsError', { detail: note.slice('error:'.length) }));
    else if (disk.uploads_applied === false) lines.push(t('myPlan.uploadsPending'));
    const codes = note.startsWith('error:') ? [] : note.split(',');
    if (codes.includes('ssh_open')) lines.push(t('myPlan.uploadsSSH'));
    if (codes.includes('ftp_kept')) lines.push(t('myPlan.uploadsFTPKept'));
    if (codes.includes('php_kept')) lines.push(t('myPlan.uploadsPHPKept'));
    if (lines.length === 0) return null;
    return <ul className="mt-1 list-disc pl-5 text-xs" data-my-plan-upload-notes="">{lines.map((line) => <li key={line}>{line}</li>)}</ul>;
}

function SubscriptionBlock({ sub, interval }: { sub: MyPlanSubscription; interval: number }) {
    const { t } = useI18n();
    const u = sub.usage;
    const mb = 1024 * 1024;
    const suspendOn = u.disk?.suspend_on ? new Date(u.disk.suspend_on).toLocaleDateString() : '';
    return (
        <div className="rounded-lg border border-border p-4" data-my-plan-subscription={sub.id}>
            {/* The state by its name (D-042 P3): Unlimited, Plan: <name>, or Custom. */}
            <p className="text-base font-semibold text-fg" data-my-plan-state={sub.rights ?? ''}>
                {sub.rights ? stateName(sub.rights, sub.plan?.name, t) : sub.plan ? sub.plan.name : t('myPlan.noPlanName')}
            </p>
            {sub.rights && <p className="mt-0.5 text-xs text-fg-subtle">{t(sub.rights === 'unlimited' ? 'rights.my.unlimited' : sub.rights === 'plan' ? 'rights.my.plan' : 'rights.my.custom')}</p>}
            {u.traffic_limited && (
                <div className="mt-2 rounded-md bg-danger/10 p-3 text-sm text-danger" data-my-plan-traffic-limited="">
                    <p className="font-semibold">{t('myPlan.trafficLimitedTitle')}</p>
                    <p className="mt-1">{t('myPlan.trafficLimited')}</p>
                </div>
            )}
            {u.disk?.uploads_blocked && (
                <div className="mt-2 rounded-md bg-danger/10 p-3 text-sm text-danger" data-my-plan-disk-full="">
                    <p className="font-semibold">{t('myPlan.diskFullTitle')}</p>
                    <p className="mt-1">{t(suspendOn ? 'myPlan.diskFull' : 'myPlan.diskFullNoDate', { date: suspendOn })}</p>
                    <UploadNotes disk={u.disk} />
                </div>
            )}
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
                <DiskFacts sub={sub} />
                <TrafficLine sub={sub} interval={interval} />
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
                            {/* alpha.128 (G18): the usage notices are in the one Notifications list
                                above this card (Dashboard and My account), with the account notices.
                                Kullanım bildirimleri artık bu kartın üstündeki tek Bildirimler listesinde. */}
                            {/* A reseller's pool comes first: it is what they hand out to customers.
                                A plan of their own is only needed to host their own sites.
                                Bayinin havuzu once gelir; kendi plani yalniz kendi siteleri icindir. */}
                            {plan.pool && <PoolBlock pool={plan.pool} rights={plan.resellerRights} />}
                            {plan.subscriptions.length === 0 ? (
                                <p className="text-sm text-fg-muted" data-my-plan-none={plan.pool ? 'reseller' : 'account'}>{t(plan.pool ? 'myPlan.noneReseller' : 'myPlan.none')}</p>
                            ) : (
                                plan.subscriptions.map((sub) => <SubscriptionBlock key={sub.id} sub={sub} interval={plan.trafficIntervalMinutes} />)
                            )}
                            {plan.bandwidthMeasured && <p className="text-xs text-fg-subtle" data-my-plan-traffic-interval="">{t('myPlan.trafficInterval', { minutes: plan.trafficIntervalMinutes })}</p>}
                            {contact && <p className="text-sm text-fg-muted">{contact}</p>}
                        </div>
                    );
                }}
            </RemoteGate>
        </section>
    );
}

function PoolBlock({ pool, rights }: { pool: NonNullable<MyPlan['pool']>; rights?: MyPlan['resellerRights'] }) {
    const { t } = useI18n();
    const a = pool.allocated;
    const l = pool.limits;
    // alpha.117 (K2): the pool limits what the customers really use; what is
    // handed out is information only.
    // Havuz müşterilerin gerçekten kullandığını sınırlar; dağıtılan yalnız bilgidir.
    if (pool.used) {
        const u = pool.used;
        const diskMB = Math.round(u.disk_used_bytes / (1024 * 1024));
        return (
            <div className="rounded-lg border border-border p-4" data-my-plan-pool="used">
                <p className="text-base font-semibold text-fg">{t('myPlan.pool')}</p>
                {rights && <p className="mt-0.5 text-sm text-fg-muted" data-my-reseller-rights={rights.state}>{stateName(rights.state, rights.plan_name, t, true)}</p>}
                <p className="mt-1 text-xs text-fg-subtle">{l ? t('rights.my.poolUsedHint') : t('myPlan.poolNone')}</p>
                <div className="mt-3"><ResellerPoolNotices /></div>
                {pool.over_limit.length > 0 && (
                    <ul className="mt-2 list-disc pl-5 text-sm text-danger">{pool.over_limit.map((line) => <li key={line}>{overLimitText(line, t)}</li>)}</ul>
                )}
                <div className="mt-3 grid gap-3 sm:grid-cols-2">
                    <Meter label={t('myPlan.customers')} used={u.customers} limit={l?.max_customers ?? 0} usedText={String(u.customers)} limitText={String(l?.max_customers ?? 0)} />
                    <Meter label={t('plans.col.domains')} used={u.domains} limit={l?.max_domains ?? 0} usedText={String(u.domains)} limitText={String(l?.max_domains ?? 0)} />
                    <Meter label={t('plans.col.databases')} used={u.databases} limit={l?.max_databases ?? 0} usedText={String(u.databases)} limitText={String(l?.max_databases ?? 0)} />
                    <Meter label={t('plans.col.mail')} used={u.mailboxes} limit={l?.max_email_accounts ?? 0} usedText={String(u.mailboxes)} limitText={String(l?.max_email_accounts ?? 0)} />
                    <Meter label={t('plans.col.disk')} used={diskMB} limit={l?.disk_quota_mb ?? 0} usedText={fmtMB(diskMB)} limitText={fmtMB(l?.disk_quota_mb ?? 0)} />
                    <Meter
                        label={t('myPlan.traffic')}
                        used={u.traffic_month_bytes ?? 0}
                        limit={(l?.traffic_quota_mb ?? 0) * 1024 * 1024}
                        usedText={formatBytes(u.traffic_month_bytes ?? 0)}
                        limitText={fmtMB(l?.traffic_quota_mb ?? 0)}
                    />
                </div>
                <p className="mt-3 text-xs text-fg-subtle" data-my-plan-allocated="">
                    {t('rights.my.allocated', { customers: a.customers, domains: a.unlimited_domains ? t('rights.unlimitedShort') : String(a.domains) })}
                </p>
            </div>
        );
    }
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
