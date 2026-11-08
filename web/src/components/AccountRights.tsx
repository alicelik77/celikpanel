import { Suspense, lazy, useState } from 'react';
import { Save, X } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { showToast } from './Toast';
import { Button, ErrorBanner, RemoteGate, inputClass } from './ui';
import { BlockedReason } from './BlockingProblems';
import type { PanelUser } from '../lib/api';
import type { ApiError } from '../lib/apiError';
import { invalidate, useRemote } from '../lib/remote';
import { PLANS_URL, decodePlans } from '../lib/accounts';
import {
    NO_LIMITS, RESELLER_PLANS_URL, RIGHTS_CATALOG_URL, assignablePlans, decodeCatalog, decodeResellerPlans, decodeSubscriptionRights,
    decodeUserRights, itemLabel, limitWords, overLimitText, payloadOf, selectionOf, sendRights, stateName, subscriptionEndsAtURL, subscriptionRightsURL, userRightsURL,
    type NewAccountRights, type Numbers,
    type ResellerRights as ResellerRightsAnswer, type RightsPreview, type RightsSelection, type RightsState, type RightsView,
} from '../lib/rights';
import { RightsPreviewBox, RightsSetEditor } from './RightsSetEditor';
// alpha.128: a Custom set's limits of site settings, a chunk of its own.
const CustomLimitsSection = lazy(() => import('./PlanLimitsSection').then((module) => ({ default: module.CustomLimitsSection })));

// An account's rights (D-042 6.1-6.3): Unlimited, bound to a plan, or Custom,
// named on every screen. A change is shown first and then confirmed (P7): the
// server answers the first save with what it would do, and the person reads
// it - "the account leaves the plan", what is taken away and what keeps
// running - before the same change is sent again with the confirmation.
//
// Hesabın hakları: Sınırsız, bir plana bağlı ya da Özel. Değişiklik önce
// gösterilir, sonra onaylanır: sunucu ilk kayda ne yapacağıyla yanıt verir.

type Say = (key: TranslationKey, vars?: Record<string, string | number>) => string;

/** The grantor's catalogue, read once and shared by every rights screen. */
export function useGrantorCatalog() {
    return useRemote(RIGHTS_CATALOG_URL, decodeCatalog);
}

/** A refusal's evidence in words: rights items by name, usage lines in the reader's language. */
export function readableRefusal(error: ApiError | null, t: Say): ApiError | null {
    if (!error || !error.details) return error;
    return {
        ...error,
        details: error.details.map((line) => (/^(permission|component|integration|extension|app|runtime):/.test(line) ? itemLabel(line, t) : overLimitText(line, t))),
    };
}

/** The sentence above a preview: what this change is, in the owner's words. */
export function previewSentence(preview: RightsPreview, t: Say, reseller = false): string {
    if (preview.leaves_plan) return t(reseller ? 'rights.leavesPlanReseller' : 'rights.leavesPlan', { name: preview.from_plan_name ?? '' });
    if (preview.to.startsWith('plan:')) return t('rights.bindsPlan', { name: preview.to_plan_name ?? '' });
    if (preview.to === 'unlimited') return t(reseller ? 'rights.becomesUnlimitedReseller' : 'rights.becomesUnlimited');
    return t('rights.changeCustom');
}

interface ChangeTarget {
    kind: 'subscription' | 'reseller';
    url: string;
    current: RightsView;
    numbers: Numbers;
    /** alpha.128: the reseller whose Custom limits the form reads. */
    resellerId?: number;
}

/**
 * The form that changes one account's rights. `plans` are the plans the
 * person editing may give it; `onCreatePlan` opens the plans tab when there
 * is none.
 */
export function RightsChangeForm({ target, grantor, plans, grantorIsReseller, onDone, onCreatePlan }: {
    target: ChangeTarget;
    grantor: RightsView;
    plans: { id: number; name: string }[];
    grantorIsReseller: boolean;
    onDone: () => void;
    onCreatePlan?: () => void;
}) {
    const { t } = useI18n();
    const reseller = target.kind === 'reseller';
    const idPrefix = `rights-${target.kind}-${target.current.subscription_id || 'r'}`;
    const [mode, setMode] = useState<RightsState>(target.current.state);
    const [planID, setPlanID] = useState(target.current.plan_id ?? plans[0]?.id ?? 0);
    const [selection, setSelection] = useState<RightsSelection>(() => selectionOf(target.current.own, grantor));
    const [numbers, setNumbers] = useState<Numbers>(target.numbers);
    const [busy, setBusy] = useState(false);
    const [pending, setPending] = useState<{ preview: RightsPreview; body: string } | null>(null);
    const [refusal, setRefusal] = useState<ApiError | null>(null);
    // alpha.128: limits, overuse policy, overselling of a Custom set; only what was touched.
    const [extras, setExtras] = useState<Record<string, unknown> | null>(null);

    const body = (): Record<string, unknown> => {
        if (mode === 'unlimited') return { state: 'unlimited' };
        if (mode === 'plan') return { state: 'plan', plan_id: planID };
        const { max_customers, ...limits } = numbers;
        return { state: 'custom', rights: payloadOf(selection), quantities: reseller ? { ...limits, max_customers } : limits, ...(extras ?? {}) };
    };
    const bodyText = JSON.stringify(body());
    // A preview belongs to the exact values it was answered for.
    const shown = pending && pending.body === bodyText ? pending.preview : null;
    const blocked = mode === 'plan' && (plans.length === 0 || planID === 0);

    const save = async (confirm: boolean) => {
        setBusy(true);
        setRefusal(null);
        try {
            const sent = body();
            const accept = confirm && (shown?.over_limit.length ?? 0) > 0;
            const result = await sendRights(target.url, 'PUT', confirm ? { ...sent, confirm: true, ...(accept && !reseller ? { accept_over_usage: true } : {}) } : sent);
            if (result.kind === 'lost') {
                showToast('error', t('common.resultUnknown'));
                invalidate('rights', 'users');
                return;
            }
            if (result.kind === 'refused') {
                if (result.preview && (result.error.code === 'RIGHTS_LEAVES_PLAN' || result.error.code === 'RIGHTS_CHANGE_CONFIRM')) {
                    setPending({ preview: result.preview, body: JSON.stringify(sent) });
                    return;
                }
                setRefusal(result.error);
                return;
            }
            setPending(null);
            showToast('success', t('rights.saved'));
            invalidate('rights', 'users', 'plans');
            onDone();
        } finally {
            setBusy(false);
        }
    };

    const choice = (value: RightsState, label: string, hint: string) => (
        <label className={`flex cursor-pointer items-start gap-2.5 rounded-lg border p-3 max-sm:min-h-11 ${mode === value ? 'border-primary bg-primary/5' : 'border-border'}`} data-rights-choice={value}>
            <input type="radio" name={`${idPrefix}-mode`} className="mt-1 h-4 w-4 shrink-0 accent-primary" checked={mode === value} onChange={() => setMode(value)} />
            <span className="min-w-0">
                <span className="block text-sm font-medium text-fg">{label}</span>
                <span className="block text-xs text-fg-muted">{hint}</span>
            </span>
        </label>
    );

    return (
        <div className="grid gap-4" data-rights-form={target.kind}>
            <div className="grid gap-2 sm:grid-cols-3">
                {choice('unlimited', t(reseller ? 'rights.state.unlimitedReseller' : 'rights.state.unlimited'), t(grantorIsReseller ? 'rights.unlimitedHintReseller' : 'rights.unlimitedHint'))}
                {choice('plan', t('rights.choice.plan'), t('rights.choice.planHint'))}
                {choice('custom', t('rights.state.custom'), t('rights.choice.customHint'))}
            </div>

            {mode === 'plan' && (plans.length === 0 ? (
                <div className="flex flex-wrap items-center gap-2 text-sm text-fg" id={`${idPrefix}-noplan`}>
                    <span>{t(reseller ? 'rights.noResellerPlan' : 'rights.noPlan')}</span>
                    {onCreatePlan && <Button type="button" onClick={onCreatePlan}>{t('rights.createPlan')}</Button>}
                </div>
            ) : (
                <label className="block max-w-sm">
                    <span className="mb-1 block text-xs text-fg-muted">{t('rights.choice.plan')}</span>
                    <select id={`${idPrefix}-plan`} value={planID} onChange={(e) => setPlanID(Number(e.target.value))} className={inputClass}>
                        {planID === 0 && <option value={0}>{t('rights.choosePlan')}</option>}
                        {plans.map((plan) => <option key={plan.id} value={plan.id}>{plan.name}</option>)}
                    </select>
                </label>
            ))}

            {mode === 'custom' && (
                <RightsSetEditor
                    grantor={grantor}
                    selection={selection}
                    onSelection={setSelection}
                    numbers={numbers}
                    onNumbers={setNumbers}
                    reseller={reseller}
                    grantorIsReseller={grantorIsReseller}
                    idPrefix={idPrefix}
                    effective={target.kind === 'subscription' ? target.current : undefined}
                />
            )}
            {mode === 'custom' && (
                <Suspense fallback={null}>
                    <CustomLimitsSection kind={reseller ? 'reseller' : 'customer'} subscriptionId={target.current.subscription_id} resellerId={target.resellerId} idPrefix={idPrefix} onChange={setExtras} />
                </Suspense>
            )}

            {shown && <RightsPreviewBox id={`${idPrefix}-preview`} preview={shown} sentence={previewSentence(shown, t, reseller)} />}
            <ErrorBanner error={readableRefusal(refusal, t)} sentence="rights.saveFailed" />

            <div className="flex flex-wrap items-center justify-end gap-2">
                {blocked && (
                    <BlockedReason id={`${idPrefix}-reason`} target={plans.length === 0 ? `${idPrefix}-noplan` : `${idPrefix}-plan`} text={t(plans.length === 0 ? (reseller ? 'rights.noResellerPlan' : 'rights.noPlan') : 'rights.choosePlan')} className="mr-auto" />
                )}
                <Button type="button" icon={X} onClick={onDone}>{t('common.cancel')}</Button>
                {shown ? (
                    <>
                        <Button type="button" onClick={() => setPending(null)} disabled={busy}>{t('rights.back')}</Button>
                        <Button type="button" variant="primary" onClick={() => void save(true)} loading={busy} data-rights-confirm="">
                            {t(shown.over_limit.length > 0 && !reseller ? 'rights.confirmAnyway' : 'rights.confirm')}
                        </Button>
                    </>
                ) : (
                    <Button type="button" variant="primary" icon={Save} onClick={() => void save(false)} loading={busy} disabled={blocked}
                        aria-describedby={blocked ? `${idPrefix}-reason` : undefined}>
                        {t('rights.review')}
                    </Button>
                )}
            </div>
        </div>
    );
}

/** Bytes in a sentence: MB below one GB, GB with one decimal above (alpha.118). */
function usedBytes(bytes: number): string {
    const mb = bytes / (1024 * 1024);
    return mb >= 1024 ? `${(mb / 1024).toFixed(1)} GB` : `${Math.round(mb)} MB`;
}

/** The current state in one line: its name and its numbers. */
function CurrentState({ view, numbers, reseller, t }: { view: RightsView; numbers: Numbers; reseller: boolean; t: Say }) {
    const limits = [
        ...(reseller ? [`${t('rights.qty.customers')}: ${limitWords(numbers.max_customers, t)}`] : []),
        `${t('rights.qty.domains')}: ${limitWords(numbers.max_domains, t)}`,
        `${t('rights.qty.databases')}: ${limitWords(numbers.max_databases, t)}`,
        `${t('rights.qty.mailboxes')}: ${limitWords(numbers.max_email_accounts, t)}`,
        `${t('rights.qty.disk')}: ${limitWords(numbers.disk_quota_mb, t, 'MB')}`,
        `${t('rights.qty.traffic')}: ${limitWords(numbers.traffic_quota_mb, t, 'MB')}`,
    ];
    return (
        <div className="rounded-lg border border-border bg-surface p-3" data-rights-current={view.state}>
            <p className="text-xs text-fg-muted">{t('rights.now')}</p>
            <p className="text-base font-semibold text-fg">{stateName(view.state, view.plan_name, t, reseller)}</p>
            <p className="mt-1 text-xs text-fg-muted">{limits.join(' · ')}</p>
            {view.state === 'plan' && <p className="mt-1 text-xs text-fg-subtle">{t('rights.followsPlan')}</p>}
            {view.state === 'unlimited' && <p className="mt-1 text-xs text-fg-subtle">{t(reseller ? 'rights.unlimitedResellerFollows' : 'rights.unlimitedFollows')}</p>}
        </div>
    );
}

/**
 * The rights panel of one account in the Accounts list: a customer's
 * subscriptions, each Unlimited, on a plan or Custom; a reseller's own state
 * and pool (the administrator's).
 */
export function AccountRightsPanel({ user, isAdmin, onClose, onCreatePlan }: {
    user: PanelUser;
    isAdmin: boolean;
    onClose: () => void;
    onCreatePlan?: () => void;
}) {
    const { t } = useI18n();
    const read = useRemote(userRightsURL(user.id), decodeUserRights);
    const grantor = useGrantorCatalog();
    return (
        <section className="mb-4 rounded-xl border border-border bg-surface-2/50 p-4" data-account-rights={user.id} aria-label={t('rights.for', { name: user.username })}>
            <div className="mb-3 flex items-start justify-between gap-2">
                <div className="min-w-0">
                    <h3 className="text-sm font-semibold text-fg">{t('rights.for', { name: user.username })}</h3>
                    <p className="mt-0.5 text-xs text-fg-muted">{t(user.role === 'reseller' ? 'rights.introReseller' : 'rights.intro')}</p>
                </div>
                <Button type="button" icon={X} onClick={onClose} aria-label={t('common.close')}>{t('common.close')}</Button>
            </div>
            <RemoteGate remote={grantor.remote} checking={t('rights.catalogChecking')} failed={t('rights.catalogUnknown')} onRetry={() => void grantor.retry()} busy={grantor.reading}>
                {(catalog) => (
                    <RemoteGate remote={read.remote} checking={t('rights.checking')} failed={t('rights.unknown')} onRetry={() => void read.retry()} busy={read.reading}>
                        {(shown) => (shown.value.role === 'reseller' ? (
                            isAdmin ? (
                                <ResellerRightsBlock userId={user.id} answer={shown.value} grantor={catalog.value} onClose={onClose} onCreatePlan={onCreatePlan} />
                            ) : null
                        ) : (
                            <CustomerSubscriptions user={user} list={shown.value.subscriptions} grantor={catalog.value} isAdmin={isAdmin} onClose={onClose} onCreatePlan={onCreatePlan} />
                        ))}
                    </RemoteGate>
                )}
            </RemoteGate>
        </section>
    );
}

function ResellerRightsBlock({ userId, answer, grantor, onClose, onCreatePlan }: {
    userId: number;
    answer: ResellerRightsAnswer;
    grantor: RightsView;
    onClose: () => void;
    onCreatePlan?: () => void;
}) {
    const { t } = useI18n();
    const plans = useRemote(RESELLER_PLANS_URL, decodeResellerPlans);
    const numbers: Numbers = answer.pool ?? NO_LIMITS;
    const used = answer.used;
    return (
        <div className="grid gap-4">
            <CurrentState view={answer.rights} numbers={numbers} reseller t={t} />
            <p className="text-xs text-fg-muted" data-reseller-used="">
                {t('rights.poolUsed', {
                    customers: used.customers, domains: used.domains, databases: used.databases, mailboxes: used.mailboxes,
                    disk: usedBytes(used.disk_used_bytes),
                    traffic: usedBytes(used.traffic_month_bytes),
                })}
            </p>
            {answer.over_limit.length > 0 && (
                <ul className="list-disc pl-5 text-sm text-danger">{answer.over_limit.map((line) => <li key={line}>{overLimitText(line, t)}</li>)}</ul>
            )}
            <RemoteGate remote={plans.remote} checking={t('rights.plansChecking')} failed={t('rights.plansUnknown')} onRetry={() => void plans.retry()} busy={plans.reading}>
                {(list) => (
                    <RightsChangeForm
                        target={{ kind: 'reseller', url: userRightsURL(userId), current: answer.rights, numbers, resellerId: userId }}
                        grantor={grantor}
                        plans={list.value.plans}
                        grantorIsReseller={false}
                        onDone={onClose}
                        onCreatePlan={onCreatePlan}
                    />
                )}
            </RemoteGate>
        </div>
    );
}

function CustomerSubscriptions({ user, list, grantor, isAdmin, onClose, onCreatePlan }: {
    user: PanelUser;
    list: { id: number; name: string; state: string; plan_name?: string }[];
    grantor: RightsView;
    isAdmin: boolean;
    onClose: () => void;
    onCreatePlan?: () => void;
}) {
    const { t } = useI18n();
    const [chosen, setChosen] = useState(list[0]?.id ?? 0);
    if (list.length === 0) return <p className="text-sm text-fg-muted">{t('rights.noSubscription')}</p>;
    return (
        <div className="grid gap-3">
            {list.length > 1 && (
                <label className="block max-w-sm">
                    <span className="mb-1 block text-xs text-fg-muted">{t('rights.subscription')}</span>
                    <select value={chosen} onChange={(e) => setChosen(Number(e.target.value))} className={inputClass}>
                        {list.map((entry) => (
                            <option key={entry.id} value={entry.id}>{`${entry.name} · ${stateName(entry.state, entry.plan_name, t)}`}</option>
                        ))}
                    </select>
                </label>
            )}
            {chosen > 0 && <SubscriptionRights key={chosen} subscriptionId={chosen} user={user} grantor={grantor} isAdmin={isAdmin} onClose={onClose} onCreatePlan={onCreatePlan} />}
        </div>
    );
}

function SubscriptionRights({ subscriptionId, user, grantor, isAdmin, onClose, onCreatePlan }: {
    subscriptionId: number;
    user: PanelUser;
    grantor: RightsView;
    isAdmin: boolean;
    onClose: () => void;
    onCreatePlan?: () => void;
}) {
    const { t } = useI18n();
    const read = useRemote(subscriptionRightsURL(subscriptionId), decodeSubscriptionRights);
    const plans = useRemote(PLANS_URL, decodePlans);
    return (
        <RemoteGate remote={read.remote} checking={t('rights.checking')} failed={t('rights.unknown')} onRetry={() => void read.retry()} busy={read.reading}>
            {(shown) => {
                const view = shown.value;
                if (view.unrestricted) return <p className="text-sm text-fg-muted">{t('rights.adminWorkspace')}</p>;
                const numbers: Numbers = { ...view.limits, max_customers: 0 };
                return (
                    <div className="grid gap-4">
                        <CurrentState view={view} numbers={numbers} reseller={false} t={t} />
                        <ResellerLacks view={view} t={t} />
                        <EndDateField subscriptionId={subscriptionId} endsAt={view.ends_at} />
                        <RemoteGate remote={plans.remote} checking={t('rights.plansChecking')} failed={t('rights.plansUnknown')} onRetry={() => void plans.retry()} busy={plans.reading}>
                            {(list) => (
                                <RightsChangeForm
                                    target={{ kind: 'subscription', url: subscriptionRightsURL(subscriptionId), current: view, numbers }}
                                    grantor={grantor}
                                    plans={assignablePlans(list.value, user, isAdmin)}
                                    grantorIsReseller={!isAdmin}
                                    onDone={onClose}
                                    onCreatePlan={onCreatePlan}
                                />
                            )}
                        </RemoteGate>
                    </div>
                );
            }}
        </RemoteGate>
    );
}

/**
 * Items the account's own set names that its reseller does not have
 * (alpha.130): saved, but refused until the reseller has them. Shown with
 * the reseller's name, so the administrator knows where to add them.
 */
function ResellerLacks({ view, t }: { view: RightsView; t: Say }) {
    const named = (id: string, kind: 'permissions' | 'software') =>
        kind === 'permissions' ? view.own.permissions.includes(id) : view.own.software.includes(id);
    const lacking = [
        ...view.permissions.filter((item) => item.refused_by === 'reseller' && named(item.id, 'permissions')).map((item) => itemLabel('permission:' + item.id, t)),
        ...view.software.filter((item) => item.refused_by === 'reseller' && named(item.id, 'software')).map((item) => itemLabel(item.id, t)),
    ];
    if (lacking.length === 0) return null;
    return (
        <p className="rounded-lg border border-warning/40 bg-warning/10 p-3 text-sm text-fg" role="status" data-rights-reseller-lacks-list="">
            {t('rights.resellerLacksList', { name: view.reseller_name ?? '', items: lacking.join(', ') })}
        </p>
    );
}

// A subscription's end date (K7): empty means it never ends. When the date
// passes the subscription is suspended; a date already passed suspends it
// at once, which the field says before saving.
function EndDateField({ subscriptionId, endsAt }: { subscriptionId: number; endsAt: string | null }) {
    const { t } = useI18n();
    const stored = endsAt ? endsAt.slice(0, 10) : '';
    const [value, setValue] = useState(stored);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState<ApiError | null>(null);
    const today = new Date().toISOString().slice(0, 10);
    const save = async (next: string | null) => {
        setBusy(true);
        setError(null);
        const result = await sendRights(subscriptionEndsAtURL(subscriptionId), 'PUT', { ends_at: next });
        setBusy(false);
        if (result.kind === 'refused') {
            setError(result.error);
            return;
        }
        if (result.kind === 'lost') {
            // The change may or may not have been made: the reread says which.
            showToast('error', t('rights.endsAt.lost'));
        } else {
            showToast('success', next ? t('rights.endsAt.saved', { date: next }) : t('rights.endsAt.cleared'));
        }
        invalidate('rights', 'users', 'suspension');
    };
    return (
        <div className="rounded-lg border border-border p-3 text-sm" data-ends-at={subscriptionId}>
            <p className="font-medium">{t('rights.endsAt.title')}</p>
            <p className="text-fg-muted">{stored ? t('rights.endsAt.current', { date: stored }) : t('rights.endsAt.never')}</p>
            <div className="mt-2 flex flex-wrap items-center gap-2">
                <input type="date" className={inputClass} value={value} min="2000-01-01" onChange={(e) => setValue(e.target.value)} aria-label={t('rights.endsAt.title')} />
                <Button disabled={busy || value === '' || value === stored} onClick={() => void save(value)}>
                    <Save className="h-4 w-4" /> {t('rights.endsAt.save')}
                </Button>
                {stored && (
                    <Button variant="secondary" disabled={busy} onClick={() => void save(null)}>
                        <X className="h-4 w-4" /> {t('rights.endsAt.clear')}
                    </Button>
                )}
            </div>
            {value !== '' && value !== stored && value <= today && <p className="mt-1 text-warning">{t('rights.endsAt.pastWarning')}</p>}
            <p className="mt-1 text-xs text-fg-subtle">{t('rights.endsAt.hint')}</p>
            {error && <ErrorBanner error={error} />}
        </div>
    );
}

// ---- the Rights section of the account form (6.1) ----------------------------

/**
 * The Rights section of the account form: always visible, Unlimited chosen by
 * name, a plan, or a Custom set within the creator's own field.
 */
export function NewAccountRightsSection({ role, choice, onChoice, customerPlans, grantorIsReseller, onCreatePlan }: {
    role: string;
    choice: NewAccountRights;
    onChoice: (next: NewAccountRights) => void;
    customerPlans: { id: number; name: string }[];
    grantorIsReseller: boolean;
    onCreatePlan?: () => void;
}) {
    const { t } = useI18n();
    const reseller = role === 'reseller';
    const grantor = useGrantorCatalog();
    const resellerPlans = useRemote(reseller ? RESELLER_PLANS_URL : null, decodeResellerPlans);
    const plans = reseller ? (resellerPlans.remote.state === 'known' ? resellerPlans.remote.value.plans : []) : customerPlans;
    const set = (patch: Partial<NewAccountRights>) => onChoice({ ...choice, ...patch });
    const pick = (state: RightsState) => {
        if (state === 'custom' && choice.state !== 'custom' && grantor.remote.state === 'known') {
            // A Custom set starts from everything the creator may give.
            set({ state, selection: selectionOf({ all_permissions: true, all_software: true, permissions: [], software: [] }, grantor.remote.value) });
            return;
        }
        set({ state, planID: state === 'plan' ? (choice.planID || plans[0]?.id || 0) : choice.planID });
    };
    const option = (value: RightsState, label: string, hint: string) => (
        <label className={`flex cursor-pointer items-start gap-2.5 rounded-lg border p-3 max-sm:min-h-11 ${choice.state === value ? 'border-primary bg-primary/5' : 'border-border'}`} data-new-rights={value}>
            <input type="radio" name="new-account-rights" className="mt-1 h-4 w-4 shrink-0 accent-primary" checked={choice.state === value} onChange={() => pick(value)} />
            <span className="min-w-0">
                <span className="block text-sm font-medium text-fg">{label}</span>
                <span className="block text-xs text-fg-muted">{hint}</span>
            </span>
        </label>
    );
    return (
        <fieldset className="mt-4 min-w-0 border-t border-border pt-4" data-new-account-rights={role}>
            <legend className="sr-only">{t('rights.title')}</legend>
            <p className="text-sm font-semibold text-fg" aria-hidden="true">{t('rights.title')}</p>
            <p className="mt-0.5 text-xs text-fg-muted">{t(reseller ? 'rights.newIntroReseller' : 'rights.newIntro')}</p>
            <div className="mt-3 grid gap-2 sm:grid-cols-3">
                {option('unlimited', t(reseller ? 'rights.state.unlimitedReseller' : 'rights.state.unlimited'), t(grantorIsReseller ? 'rights.unlimitedHintReseller' : 'rights.unlimitedHint'))}
                {option('plan', t(reseller ? 'rights.choice.resellerPlan' : 'rights.choice.plan'), t('rights.choice.planHint'))}
                {option('custom', t('rights.state.custom'), t('rights.choice.customHint'))}
            </div>
            {choice.state === 'plan' && (reseller && resellerPlans.remote.state !== 'known' ? (
                <RemoteGate remote={resellerPlans.remote} checking={t('rights.plansChecking')} failed={t('rights.plansUnknown')} onRetry={() => void resellerPlans.retry()} busy={resellerPlans.reading} className="mt-3">
                    {() => null}
                </RemoteGate>
            ) : plans.length === 0 ? (
                <div className="mt-3 flex flex-wrap items-center gap-2 text-sm text-fg" id="new-account-noplan">
                    <span>{t(reseller ? 'rights.noResellerPlan' : 'rights.noPlan')}</span>
                    {onCreatePlan && <Button type="button" onClick={onCreatePlan}>{t('rights.createPlan')}</Button>}
                </div>
            ) : (
                <label className="mt-3 block max-w-sm">
                    <span className="mb-1 block text-xs text-fg-muted">{t(reseller ? 'rights.choice.resellerPlan' : 'rights.choice.plan')}</span>
                    <select value={choice.planID} onChange={(e) => set({ planID: Number(e.target.value) })} className={inputClass} data-new-rights-plan="">
                        {choice.planID === 0 && <option value={0}>{t('rights.choosePlan')}</option>}
                        {plans.map((plan) => <option key={plan.id} value={plan.id}>{plan.name}</option>)}
                    </select>
                </label>
            ))}
            {choice.state === 'custom' && (
                <RemoteGate remote={grantor.remote} checking={t('rights.catalogChecking')} failed={t('rights.catalogUnknown')} onRetry={() => void grantor.retry()} busy={grantor.reading} className="mt-3">
                    {(catalog) => (
                        <div className="mt-3">
                            <RightsSetEditor
                                grantor={catalog.value}
                                selection={choice.selection}
                                onSelection={(selection) => set({ selection })}
                                numbers={choice.numbers}
                                onNumbers={(numbers) => set({ numbers })}
                                reseller={reseller}
                                grantorIsReseller={grantorIsReseller}
                                idPrefix="new-account"
                            />
                        </div>
                    )}
                </RemoteGate>
            )}
        </fieldset>
    );
}
