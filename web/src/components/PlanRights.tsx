import { Suspense, lazy, useMemo, useState } from 'react';
import { Copy, Layers, Lock, Pencil, Plus, Trash2, X } from 'lucide-react';
import { useNavigate } from '../router';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { showToast } from './Toast';
import { Button, Checking, CouldNotCheck, ErrorBanner, KnownEmpty, RemoteGate, inputClass } from './ui';
import type { ApiError } from '../lib/apiError';
import { apiErrorActionLabel, refusalText, refusalToast } from '../lib/apiError';
import { countText, invalidate, lastKnown, mapRemote, useRemote } from '../lib/remote';
import {
    NEW_PLAN, RESELLER_PLANS_URL, applyOffers, editedFromResellerPlan, groupSoftware, itemLabel, limitWords, offeredInCatalog, offeredStateOf,
    permissionGroups, permissionIdsOf, permissionTopicKey, planBody, rightsSummary, selectionOf, sendRights, softwareGroupKey, softwareGroupName,
    type EditedPlan, type Numbers, type OffersApplied, type PlanKind, type ResellerPlan, type RightsItem, type RightsPreview, type RightsSelection, type RightsView,
} from '../lib/rights';
import { RightsPreviewBox, RightsSetEditor } from './RightsSetEditor';
import { readableRefusal, useGrantorCatalog } from './AccountRights';
import { confirmAction, followOperation, type OperationReport, type OperationStep } from '../lib/confirmAction';
import { useDraft } from './kit/save/draft';
import { ChangedMark, SaveBar, ServerMovedNote, changedRowClass } from './kit/save/SaveBar';
import { useSaveWords } from './kit/save/words';
import { useLeaveGuard } from './kit/save/leaveGuard';

// The offered applications (alpha.118) are their own chunk, fetched with the tab.
// alpha.128: the plan's limits of site settings, a chunk of its own.
const PlanLimitsSection = lazy(() => import('./PlanLimitsSection').then((module) => ({ default: module.PlanLimitsSection })));
const ServerSettingLimits = lazy(() => import('./ServerSettingLimits').then((module) => ({ default: module.ServerSettingLimits })));
const OfferedAppsSection = lazy(() => import('./OfferedAppsSection').then((module) => ({ default: module.OfferedAppsSection })));

// Plans are templates of a rights set (D-042 6.4-6.5): quantities,
// permissions and software. Editing a plan shows first what it does to the
// accounts bound to it - how many, which items go, where a removed item is in
// use ("keeps running, cannot be chosen again") and who falls below a new
// limit - and saves only after that is confirmed. The built-in Unlimited
// plans are fixed rows: they take everything newly offered, so they cannot be
// narrowed; a narrower plan is a new plan.
//
// Planlar bir hak kümesinin şablonudur. Plan düzenlemesi önce bağlı hesaplara
// ne yaptığını gösterir, onaydan sonra kaydeder. Yerleşik Sınırsız planlar
// sabit satırlardır.

type Say = (key: TranslationKey, vars?: Record<string, string | number>) => string;

const sameItems = (a: string[], b: string[]) => a.length === b.length && [...a].sort().join('\n') === [...b].sort().join('\n');

/**
 * What differs from the plan as it was opened, one entry per changed field
 * (alpha.132): the name, each quantity, the permissions, the software, each
 * limit of site settings and the over-limit choices. Exported for the tests.
 */
export function planChanges(kind: PlanKind, plan: EditedPlan, initial: EditedPlan, selection: RightsSelection, initialSelection: RightsSelection): string[] {
    const changed: string[] = [];
    if (plan.name !== initial.name) changed.push('name');
    for (const key of Object.keys(plan.numbers) as (keyof Numbers)[]) {
        if ((kind === 'reseller' || key !== 'max_customers') && plan.numbers[key] !== initial.numbers[key]) changed.push(key);
    }
    if (!sameItems(selection.permissions, initialSelection.permissions)) changed.push('permissions');
    if (!sameItems(selection.software, initialSelection.software)) changed.push('software');
    const ceilings = plan.ceilings ?? {};
    const before = initial.ceilings ?? {};
    for (const key of new Set([...Object.keys(ceilings), ...Object.keys(before)])) if (ceilings[key] !== before[key]) changed.push(`ceiling:${key}`);
    if (kind === 'customer') {
        if ((plan.overuse_policy || 'close') !== (initial.overuse_policy || 'close')) changed.push('overuse_policy');
        if ((plan.notify_percent_disk ?? 80) !== (initial.notify_percent_disk ?? 80)) changed.push('notify_percent_disk');
        if ((plan.notify_percent_traffic ?? 80) !== (initial.notify_percent_traffic ?? 80)) changed.push('notify_percent_traffic');
    } else if ((plan.overselling || 'allowed') !== (initial.overselling || 'allowed')) changed.push('overselling');
    return changed;
}

/**
 * The editor of one plan, with its narrowing preview.
 * alpha.132 (owner, 8 Nov 2026): one way of saving. A changed field is
 * marked, the shared save bar counts the changes and Save is shown only
 * when there is one (a new plan: until it is created); Discard puts the
 * plan back as it was opened (a new plan: closes the form); closing or
 * leaving with unsaved changes asks first.
 */
export function PlanEditor({ kind, initial, grantor, grantorIsReseller, onClose }: {
    kind: PlanKind;
    initial: EditedPlan;
    grantor: RightsView;
    grantorIsReseller: boolean;
    onClose: () => void;
}) {
    const { t } = useI18n();
    const say = useSaveWords();
    const isNew = initial.id === 0;
    const [plan, setPlan] = useState(initial);
    const [initialSelection] = useState<RightsSelection>(() =>
        selectionOf(initial.rights ?? { all_permissions: true, all_software: true, permissions: [], software: [] }, grantor));
    const [selection, setSelection] = useState<RightsSelection>(initialSelection);
    // An existing plan's set is sent only once it was changed here: an
    // untouched "everything offered" stays as it is.
    const [touched, setTouched] = useState(isNew);
    const [busy, setBusy] = useState(false);
    // The answer of the last save did not arrive: Save waits for a check.
    const [lost, setLost] = useState(false);
    const [pending, setPending] = useState<{ preview: RightsPreview; sentence: string; body: string } | null>(null);
    const [refusal, setRefusal] = useState<ApiError | null>(null);
    const url = kind === 'reseller'
        ? (isNew ? RESELLER_PLANS_URL : `${RESELLER_PLANS_URL}/${plan.id}`)
        : (isNew ? '/api/v1/plans' : `/api/v1/plans/${plan.id}`);
    const chosen = touched ? selection : null;
    const bodyText = JSON.stringify(planBody(kind, plan, chosen));
    const shown = pending && pending.body === bodyText ? pending : null;
    const nameMissing = !plan.name.trim();
    const idPrefix = `plan-${kind}-${plan.id}`;
    const changes = planChanges(kind, plan, initial, selection, initialSelection);
    useLeaveGuard(changes.length);

    const discard = () => {
        setPlan(initial);
        setSelection(initialSelection);
        setTouched(isNew);
        setPending(null);
        setRefusal(null);
    };
    // Closing with unsaved changes asks, in the words of leaving a page.
    const close = async () => {
        if (changes.length > 0) {
            const leave = await confirmAction({
                title: say('leaveTitle'),
                tone: 'danger',
                lost: [changes.length === 1 ? say('leaveLostOne') : say('leaveLost', { n: changes.length })],
                kept: [say('leaveKept')],
                confirmLabel: say('leave'),
                cancelLabel: say('stay'),
            });
            if (!leave) return;
        }
        onClose();
    };

    const save = async (confirm: boolean) => {
        if (busy) return;
        setBusy(true);
        setRefusal(null);
        try {
            const overUsage = confirm && (shown?.preview.over_limit.length ?? 0) > 0;
            const result = await sendRights(url, isNew ? 'POST' : 'PUT', planBody(kind, plan, chosen, { narrowing: confirm, overUsage }));
            if (result.kind === 'lost') {
                // Not known whether it was saved: nothing is sent again
                // until the list has been read again.
                setLost(true);
                setPending(null);
                invalidate('plans', 'rights');
                return;
            }
            if (result.kind === 'refused') {
                const code = result.error.code;
                if (code === 'PLAN_NARROWING_CONFIRM' && result.preview) {
                    const sentence = t(kind === 'reseller' ? 'rights.plan.narrowingReseller' : 'rights.plan.narrowing', { n: result.preview.subscriptions ?? 0 });
                    setPending({ preview: result.preview, sentence, body: bodyText });
                    return;
                }
                if (code === 'PLAN_BELOW_USAGE') {
                    const preview: RightsPreview = {
                        from: '', to: '', leaves_plan: false, added: [], removed: [], in_use: [], outside_grantor: [], message: '',
                        over_limit: result.error.details ?? [],
                    };
                    setPending({ preview, sentence: t('plans.belowUsage'), body: bodyText });
                    return;
                }
                setRefusal(result.error);
                return;
            }
            setPending(null);
            const over = (result.body as { over_limit?: unknown } | null)?.over_limit;
            showToast('success', isNew ? t('plans.created') : Array.isArray(over) && over.length > 0 ? t('plans.updatedOver') : t('plans.updated'));
            invalidate('plans', 'rights', 'users');
            onClose();
        } finally {
            setBusy(false);
        }
    };

    const nameChanged = changes.includes('name');
    return (
        <section className="mb-4 rounded-xl border border-border bg-surface-2/50 p-4" data-plan-editor={kind}>
            <div className="flex flex-wrap items-start justify-between gap-3">
                <div className={`min-w-0 max-w-md flex-1 ${nameChanged ? `-mx-2 rounded-r px-2 ${changedRowClass}` : ''}`}>
                    <label className="block">
                        <span className="mb-1 block text-xs text-fg-muted">{t('plans.form.name')}{nameChanged && !isNew && <ChangedMark />}</span>
                        <input id={`${idPrefix}-name`} value={plan.name} onChange={(e) => setPlan({ ...plan, name: e.target.value })} className={inputClass} autoFocus />
                    </label>
                </div>
                <Button type="button" icon={X} onClick={() => void close()} disabled={busy} data-plan-close="">{t('common.close')}</Button>
            </div>
            <div className="mt-4">
                <RightsSetEditor
                    grantor={grantor}
                    selection={selection}
                    onSelection={(next) => { setSelection(next); setTouched(true); }}
                    numbers={plan.numbers}
                    onNumbers={(numbers) => setPlan({ ...plan, numbers })}
                    reseller={kind === 'reseller'}
                    grantorIsReseller={grantorIsReseller}
                    idPrefix={idPrefix}
                    base={isNew ? undefined : { numbers: initial.numbers, selection: initialSelection }}
                />
            </div>
            <Suspense fallback={<Checking label={t('limits.checking')} />}>
                <PlanLimitsSection kind={kind} plan={plan} onPlan={setPlan} idPrefix={idPrefix} base={isNew ? undefined : initial} />
            </Suspense>
            {!isNew && !touched && initial.rights?.all_permissions && initial.rights.all_software && (
                <p className="mt-3 text-xs text-fg-muted">{t('rights.plan.everythingKept')}</p>
            )}
            {shown && <div className="mt-4"><RightsPreviewBox id={`${idPrefix}-preview`} preview={shown.preview} sentence={shown.sentence} /></div>}
            <ErrorBanner className="mt-3" error={readableRefusal(refusal, t)} sentence="plans.saveFailed" />
            {shown ? (
                <div className="mt-4 flex flex-wrap items-center justify-end gap-2">
                    <Button type="button" onClick={() => setPending(null)} disabled={busy}>{t('rights.back')}</Button>
                    <Button type="button" variant="primary" onClick={() => void save(true)} loading={busy} data-plan-confirm="">
                        {t(shown.preview.over_limit.length > 0 && kind === 'customer' ? 'plans.saveAnyway' : 'rights.plan.confirm')}
                    </Button>
                </div>
            ) : (
                <SaveBar
                    className="-mx-4 px-4 pb-0"
                    // A new plan is created only by Save: the bar stays until then.
                    count={isNew ? 'some' : changes.length}
                    saving={busy}
                    blockedBy={nameMissing ? t('acct.plan.missing') : undefined}
                    unknown={lost ? { held: true, checking: false, onCheck: () => { invalidate('plans', 'rights'); setLost(false); } } : undefined}
                    onSave={() => void save(false)}
                    onDiscard={isNew ? onClose : discard}
                />
            )}
        </section>
    );
}

/** A built-in Unlimited plan: a fixed row, named by what it does. */
export function BuiltinPlanRow({ kind, ownField = false }: { kind: PlanKind; ownField?: boolean }) {
    const { t } = useI18n();
    return (
        <div className="mb-3 flex flex-wrap items-start gap-3 rounded-xl border border-border-strong bg-surface p-4" data-builtin-plan={kind}>
            <Lock className="mt-0.5 h-4 w-4 shrink-0 text-fg-muted" aria-hidden="true" />
            <div className="min-w-0 flex-1">
                <p className="text-base font-semibold text-fg">
                    {t(kind === 'reseller' ? 'rights.state.unlimitedReseller' : 'rights.state.unlimited')}
                    <span className="ml-2 rounded-md bg-surface-2 px-2 py-0.5 text-xs font-medium text-fg-muted">{t('rights.builtin')}</span>
                </p>
                <p className="mt-1 max-w-[75ch] text-sm text-fg-muted">{t(kind === 'reseller' ? 'rights.builtinHintReseller' : ownField ? 'rights.builtinHintOwnField' : 'rights.builtinHint')}</p>
            </div>
        </div>
    );
}

/** The editor wrapped with the grantor's catalogue it needs. */
export function PlanEditorGate({ kind, initial, grantorIsReseller, onClose }: { kind: PlanKind; initial: EditedPlan; grantorIsReseller: boolean; onClose: () => void }) {
    const { t } = useI18n();
    const grantor = useGrantorCatalog();
    return (
        <RemoteGate remote={grantor.remote} checking={t('rights.catalogChecking')} failed={t('rights.catalogUnknown')} onRetry={() => void grantor.retry()} busy={grantor.reading} className="mb-4">
            {(catalog) => <PlanEditor kind={kind} initial={initial} grantor={catalog.value} grantorIsReseller={grantorIsReseller} onClose={onClose} />}
        </RemoteGate>
    );
}

const decodeResellerPlansList = (raw: unknown) => {
    if (!raw || typeof raw !== 'object' || !('plans' in raw)) throw new Error('shape');
    return raw as { plans: ResellerPlan[] | null };
};

/** The administrator's reseller plans (D-042 5). */
export function ResellerPlansTab() {
    const { t } = useI18n();
    const plans = useRemote(RESELLER_PLANS_URL, decodeResellerPlansList);
    const [editing, setEditing] = useState<EditedPlan | null>(null);
    const [busy, setBusy] = useState(false);
    const known = plans.remote.state === 'known' && !plans.reading;
    const rows = mapRemote(plans.remote, (value) => value.plans ?? []);

    const remove = async (plan: ResellerPlan) => {
        await confirmAction({ message: t('rights.plan.deleteConfirm', { name: plan.name }), confirmLabel: t('common.delete'), tone: 'danger',
            action: async () => {
                setBusy(true);
                try {
                    const result = await sendRights(`${RESELLER_PLANS_URL}/${plan.id}`, 'DELETE');
                    if (result.kind === 'lost') {
                        showToast('error', t('common.resultUnknown'));
                    } else if (result.kind === 'refused') {
                        showToast('error', refusalText(result.error, t, 'plans.deleteFailed'), refusalToast(result.error, t));
                    } else {
                        showToast('success', t('plans.deleted'));
                    }
                    invalidate('rights', 'plans');
                } finally {
                    setBusy(false);
                }
            },
        });
    };

    return (
        <div data-reseller-plans="">
            <p className="mb-3 max-w-[75ch] text-xs text-fg-subtle">{t('rights.resellerPlans.hint')}</p>
            <BuiltinPlanRow kind="reseller" />
            <div className="mb-3 flex items-center justify-between">
                <span className="text-xs text-fg-subtle">{countText(mapRemote(rows, (list) => list.length)) === '1' ? t('common.itemsTotalOne') : t('common.itemsTotal', { n: countText(mapRemote(rows, (list) => list.length)) })}</span>
                {/* Hidden while a plan form is open (owner, 6 Nov 2026). */}
                {editing === null && <Button variant="primary" icon={Plus} disabled={!known} onClick={() => setEditing({ ...NEW_PLAN, numbers: { ...NEW_PLAN.numbers, max_customers: 10 } })} data-plan-add="">{t('plans.add')}</Button>}
            </div>
            {editing && <PlanEditorGate kind="reseller" initial={editing} grantorIsReseller={false} onClose={() => setEditing(null)} />}
            <RemoteGate remote={rows} checking={t('plans.checking')} failed={t('plans.unknown')} onRetry={() => void plans.retry()} busy={plans.reading} className="py-3">
                {(shown) => (shown.value.length === 0 ? (
                    <KnownEmpty of={shown} icon={Layers} title={t('rights.resellerPlans.empty')} hint={t('rights.resellerPlans.emptyHint')} />
                ) : (
                    <div className="overflow-x-auto rounded-xl border border-border-strong bg-surface">
                        <table className="w-full text-sm">
                            <thead>
                                <tr className="border-b border-border text-left text-xs font-semibold text-fg-muted">
                                    <th className="px-4 py-2.5">{t('plans.col.name')}</th>
                                    <th className="px-4 py-2.5">{t('rights.qty.customers')}</th>
                                    <th className="px-4 py-2.5">{t('plans.col.domains')}</th>
                                    <th className="px-4 py-2.5">{t('plans.col.databases')}</th>
                                    <th className="px-4 py-2.5">{t('plans.col.mail')}</th>
                                    <th className="px-4 py-2.5">{t('plans.col.disk')}</th>
                                    <th className="px-4 py-2.5">{t('rights.qty.traffic')}</th>
                                    <th className="px-4 py-2.5">{t('rights.col.rights')}</th>
                                    <th className="px-4 py-2.5">{t('rights.col.resellers')}</th>
                                    <th className="px-4 py-2.5" />
                                </tr>
                            </thead>
                            <tbody>
                                {shown.value.map((plan) => (
                                    <tr key={plan.id} className="border-b border-border last:border-0 hover:bg-surface-2/60">
                                        <td className="px-4 py-3 text-base font-medium text-fg">{plan.name}</td>
                                        <td className="px-4 py-3 text-fg-muted">{limitWords(plan.max_customers, t)}</td>
                                        <td className="px-4 py-3 text-fg-muted">{limitWords(plan.max_domains, t)}</td>
                                        <td className="px-4 py-3 text-fg-muted">{limitWords(plan.max_databases, t)}</td>
                                        <td className="px-4 py-3 text-fg-muted">{limitWords(plan.max_email_accounts, t)}</td>
                                        <td className="px-4 py-3 text-fg-muted">{limitWords(plan.disk_quota_mb, t, 'MB')}</td>
                                        <td className="px-4 py-3 text-fg-muted">{limitWords(plan.traffic_quota_mb ?? 0, t, 'MB')}</td>
                                        <td className="px-4 py-3 text-fg-muted">{rightsSummary(plan.rights, t)}</td>
                                        <td className="px-4 py-3 text-fg-muted">{plan.resellers ?? 0}</td>
                                        <td className="px-4 py-3">
                                            <div className="flex items-center justify-end gap-0.5">
                                                <IconButton disabled={!known || editing !== null} title={t('plans.edit')} onClick={() => setEditing(editedFromResellerPlan(plan))}><Pencil className="h-4 w-4" /></IconButton>
                                                <IconButton disabled={!known || editing !== null} title={t('limits.plan.copy')} onClick={() => setEditing({ ...editedFromResellerPlan(plan), id: 0, name: t('limits.plan.copyName', { name: plan.name }) })}><Copy className="h-4 w-4" /></IconButton>
                                                <IconButton danger disabled={!known || busy} title={t('users.delete')} onClick={() => void remove(plan)}><Trash2 className="h-4 w-4" /></IconButton>
                                            </div>
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                ))}
            </RemoteGate>
        </div>
    );
}

function IconButton({ children, title, onClick, danger, disabled }: { children: React.ReactNode; title: string; onClick: () => void; danger?: boolean; disabled?: boolean }) {
    return (
        <button type="button" title={title} aria-label={title} onClick={onClick} disabled={disabled}
            className={`rounded-md p-1.5 text-fg-subtle transition-colors hover:bg-surface-2 disabled:pointer-events-none disabled:opacity-40 max-sm:min-h-11 max-sm:min-w-11 ${danger ? 'hover:text-danger' : 'hover:text-fg'}`}>
            {children}
        </button>
    );
}

// ---- the administrator's offered layer (D-042 5, P1) -------------------------

/** One item of the offered layer in words. */
function offeredLabel(id: string, t: Say): string {
    return itemLabel(id, t);
}

/** Where a row's own screen is: Catalog management, a component's page or Settings. */
function RowLink({ to, label }: { to: string; label: string }) {
    const navigate = useNavigate();
    return (
        <button type="button" onClick={() => navigate(to)} className="text-xs font-semibold text-primary hover:underline max-sm:min-h-11" data-offered-link={to}>
            {label}
        </button>
    );
}

/**
 * What the applications' part (OfferedAppsSection.tsx, its own chunk) hands
 * the Offered tab's one save bar: its unsaved count and state, and its save,
 * discard and check. Stable functions; a new object only when a value changes.
 */
export interface OfferedPart {
    count: number;
    saving: boolean;
    /** Save is off until the list has been read again. */
    blocked: boolean;
    unknown: boolean;
    held: boolean;
    checking: boolean;
    /**
     * Asks every question its save needs (offering an application asks with
     * its risk) and sends nothing: true when there is none or the answer was
     * yes. The tab asks it before anything at all is sent.
     */
    ask: () => Promise<boolean>;
    /** Sends the changes without asking, each one a step in `report`; null when nothing was sent. */
    save: (report: OperationReport) => Promise<OffersApplied | null>;
    reset: () => void;
    check: () => void;
}

/** What a save of offers did not do, beside the list, with each refusal's own fix. */
function OffersReport({ report, className = '' }: { report: OffersApplied | null; className?: string }) {
    const { t } = useI18n();
    const navigate = useNavigate();
    if (!report || report.failed.length === 0) return null;
    return (
        <div className={`rounded-lg border border-danger/40 bg-danger/5 px-3 py-2 text-sm leading-6 text-fg ${className}`} role="alert" data-offered-report="">
            {report.saved.length > 0 && <p>{t('rights.offered.savedSome', { what: report.saved.join(', ') })}</p>}
            {report.failed.map((line) => (
                <p key={line.text} className="flex flex-wrap items-center gap-x-3 gap-y-1">
                    <span className="min-w-0 max-w-[75ch]">{line.text}</span>
                    {line.fix && <Button type="button" onClick={() => navigate(line.fix!.action!)}>{apiErrorActionLabel(line.fix, t)}</Button>}
                </p>
            ))}
        </div>
    );
}

/**
 * What the administrator offers on this server: every permission and every
 * installed piece of software, each shown or hidden for resellers and
 * customers. Hiding changes nothing that runs; the item can no longer be
 * chosen. The Unlimited plans follow at once (P4).
 *
 * alpha.130: a tick says what the server did. Every change is answered with
 * the item's state as the gates now read it (or the list is read again); a
 * state other than the one asked for is said beside the list, never a box
 * that quietly flips back. Items offered elsewhere (uploaded programs,
 * Python in Catalog management) are shown with their state and a way there,
 * not with a box that cannot change them. Permissions stand under the same
 * topics as on every rights screen, sorted in the reader's language.
 *
 * alpha.132 (owner, 8 Nov 2026): ticking no longer saves. The permissions,
 * the software and the applications are one list with one save bar under
 * it. The applications stay their own chunk with their own read and draft
 * (the catalogue module stays off the Users route) and hand the bar their
 * count and save (OfferedPart). The server's limits further down are a
 * separate setting with their own bar. Save sends one request per changed
 * item; what the server did not take is named with its reason and stays a
 * marked change; offering an application asks first, with its risk.
 */
export function OfferedItemsTab() {
    const { t, locale } = useI18n();
    const say = useSaveWords();
    const catalog = useGrantorCatalog();
    const rightsView = lastKnown(catalog.remote)?.value;
    const known = useMemo(() => {
        if (!rightsView) return null;
        const offers: Record<string, boolean> = {};
        for (const id of permissionIdsOf(rightsView)) offers[`permission:${id}`] = rightsView.permissions.find((item) => item.id === id)?.granted ?? false;
        for (const item of rightsView.software) if (!item.id.startsWith('app:') && !offeredInCatalog(item.id)) offers[item.id] = item.granted;
        return offers;
    }, [rightsView]);
    const draft = useDraft<Record<string, boolean>>(known);
    const [report, setReport] = useState<OffersApplied | null>(null);
    // The applications' part (its own chunk) once it has been fetched.
    const [apps, setApps] = useState<OfferedPart | null>(null);

    const saveItems = async (report: OperationReport) => {
        let result: OffersApplied | null = null;
        await draft.save(async (changes) => {
            result = await applyOffers(Object.entries(changes) as [string, boolean][], {
                t,
                report,
                nameOf: (id) => offeredLabel(id, t),
                reread: async () => {
                    // Every chooser, plan and account screen follows the offer.
                    invalidate('rights', 'components');
                    const reread = await catalog.retry();
                    return (id) => (reread.state === 'known' ? offeredStateOf(reread.value, id) : undefined);
                },
            });
            if (result.failed.length === 0) return 'saved';
            setReport(result);
            return result.lost ? 'unknown' : 'refused';
        });
        return result;
    };
    // One bar for the whole list. Every question the save needs is asked
    // first (offering an application asks with its risk); a "no" sends
    // nothing and keeps every change. Then the permissions and software and
    // the applications run in one progress window, a step per item.
    // Tek çubuk: kaydetmenin gerektirdiği her soru önce sorulur; "hayır"
    // hiçbir şey göndermez. Sonra hepsi tek ilerleme penceresinde çalışır.
    const save = async () => {
        const items = draft.count > 0;
        const part = apps && apps.count > 0 ? apps : null;
        if (!items && !part) return;
        if (part && !(await part.ask())) return;
        setReport(null);
        await followOperation({
            progressTitle: t('rights.offered.applying'),
            action: async (report) => {
                // Both lists end with their own read step; ids stay unique in the window.
                let first: OperationStep[] = [];
                let second: OperationStep[] = [];
                const show = () => report.steps([...first, ...second.map((step) => ({ ...step, id: `apps-${step.id}` }))]);
                const results: OffersApplied[] = [];
                if (items) {
                    const result = await saveItems({ steps: (steps) => { first = steps; show(); } });
                    if (result) results.push(result);
                }
                if (part) {
                    const result = await part.save({ steps: (steps) => { second = steps; show(); } });
                    if (result) results.push(result);
                }
                const failed = results.flatMap((result) => result.failed);
                if (failed.length === 0) return { state: 'done', title: t('rights.offered.savedTitle') };
                const lost = results.some((result) => result.lost);
                return {
                    state: lost ? 'unknown' : 'failed',
                    title: t(lost ? 'rights.offered.unknownTitle' : 'rights.offered.failedTitle'),
                    reason: `${failed.map((line) => line.text).join(' ')} ${t('rights.offered.next')}`,
                };
            },
        });
    };
    const itemsUnknown = draft.outcome === 'unknown';
    const unknown = itemsUnknown || apps?.unknown;

    const row = (item: string, state: RightsItem | undefined) => {
        const name = offeredLabel(item, t);
        const inCatalog = offeredInCatalog(item);
        const offered = (inCatalog ? undefined : draft.values?.[item]) ?? state?.granted ?? false;
        const changed = !inCatalog && draft.isChanged(item);
        // The word on the right says what the server has now; a changed row's
        // mark already says what Save will make of it.
        // Sağdaki sözcük sunucuda şu an olanı söyler; değişiklik işareti
        // Kaydet'in ne yapacağını zaten söyler.
        const live = (inCatalog ? undefined : draft.base?.[item]) ?? state?.granted ?? false;
        const withdrawn = state?.withdrawn === true;
        const component = item.startsWith('component:node:') ? 'node' : 'php-fpm';
        return (
            <li key={item} className={`flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2 ${changed ? changedRowClass : ''}`} data-offered-item={item} data-offered={offered ? 'yes' : 'no'}>
                {inCatalog ? (
                    <span className="min-w-0 flex-1 text-sm font-medium text-fg">{name}</span>
                ) : (
                    <label className="flex min-w-0 flex-1 cursor-pointer items-center gap-2.5 max-sm:min-h-11">
                        <input type="checkbox" checked={offered} disabled={draft.saving || catalog.remote.state !== 'known' || (withdrawn && !state?.granted)}
                            onChange={(e) => draft.set(item, e.target.checked)}
                            className="h-4 w-4 shrink-0 accent-primary" aria-label={t('rights.offered.toggle', { name })} />
                        <span className="min-w-0 text-sm font-medium text-fg">{name}{changed && <ChangedMark switched={offered} />}</span>
                    </label>
                )}
                <span className={`text-xs ${live ? 'text-fg-muted' : 'font-semibold text-warning'}`} data-offered-now={live ? 'yes' : 'no'}>{t(live ? 'rights.offered.yes' : 'rights.offered.no')}</span>
                {draft.isMoved(item) && draft.base && (
                    <div className="w-full"><ServerMovedNote value={t(draft.base[item] ? 'rights.offered.yes' : 'rights.offered.no')} onTake={() => draft.take(item)} /></div>
                )}
                {inCatalog && (
                    <span className="flex w-full flex-wrap items-center gap-x-2 text-xs text-fg-muted" data-offered-elsewhere={item}>
                        {t('rights.offered.inCatalog')} <RowLink to="/addons?view=catalog" label={t('rights.offered.openCatalog')} />
                    </span>
                )}
                {withdrawn && (
                    <span className="flex w-full flex-wrap items-center gap-x-2 text-xs text-warning" data-offered-withdrawn={item}>
                        {t('rights.offered.withdrawn')} <RowLink to={`/components/${component}`} label={t('rights.offered.openComponent')} />
                    </span>
                )}
                {state?.server_off && (
                    <span className="flex w-full flex-wrap items-center gap-x-2 text-xs text-warning" data-offered-server-off={item}>
                        {t('rights.offered.remoteApiOff')} <RowLink to="/settings?section=panel" label={t('rights.offered.turnOn')} />
                    </span>
                )}
            </li>
        );
    };

    return (
        <div data-offered-layer="">
            <p className="mb-3 max-w-[75ch] text-sm text-fg-muted">{t('rights.offered.intro')}</p>
            <div data-offered-draft="">
                {catalog.remote.state === 'loading' && <Checking label={t('rights.catalogChecking')} />}
                {catalog.remote.state === 'unknown' && <CouldNotCheck text={t('rights.catalogUnknown')} onRetry={() => void catalog.retry()} busy={catalog.reading} />}
                {rightsView && (
                    <div className="grid gap-4 lg:grid-cols-2">
                        <section>
                            <h3 className="mb-2 text-sm font-semibold text-fg">{t('rights.part.permissions')}</h3>
                            <div className="grid gap-3">
                                {permissionGroups(permissionIdsOf(rightsView), t, locale).map(({ topic, ids }) => (
                                    <div key={topic} data-offered-topic={topic}>
                                        <p className="mb-1 text-xs font-semibold text-fg-muted">{t(permissionTopicKey(topic))}</p>
                                        <ul className="divide-y divide-border rounded-lg border border-border bg-surface">
                                            {ids.map((id) => row(`permission:${id}`, rightsView.permissions.find((item) => item.id === id)))}
                                        </ul>
                                    </div>
                                ))}
                            </div>
                        </section>
                        <section>
                            <h3 className="mb-2 text-sm font-semibold text-fg">{t('rights.part.software')}</h3>
                            {rightsView.installed_unknown && <p className="mb-2 text-xs text-warning">{t('rights.installedUnknown')}</p>}
                            {rightsView.software.length === 0 ? <p className="text-sm text-fg-muted">{t('rights.softwareNone')}</p> : (
                                <div className="grid gap-3">
                                    {groupSoftware(rightsView.software.filter((item) => !item.id.startsWith('app:'))).map(({ group, items }) => {
                                        const heading = softwareGroupName(group);
                                        return (
                                            <div key={group}>
                                                <p className="mb-1 text-xs font-semibold text-fg-muted">{heading.product ?? t(softwareGroupKey(heading.key))}</p>
                                                <ul className="divide-y divide-border rounded-lg border border-border bg-surface">{items.map((item) => row(item.id, item))}</ul>
                                            </div>
                                        );
                                    })}
                                </div>
                            )}
                        </section>
                    </div>
                )}
                <OffersReport className="mt-4" report={report} />
                <Suspense fallback={<Checking className="mt-6" label={t('apps.checking')} />}>
                    <OfferedAppsSection onPart={setApps} />
                </Suspense>
                <SaveBar
                    count={draft.count + (apps?.count ?? 0)}
                    saving={draft.saving || !!apps?.saving}
                    blockedBy={(draft.count > 0 && catalog.remote.state !== 'known') || apps?.blocked ? say('stale') : undefined}
                    unknown={unknown
                        ? {
                            held: (itemsUnknown && draft.held) || !!apps?.held,
                            checking: catalog.reading || !!apps?.checking,
                            onCheck: () => { void catalog.retry(); apps?.check(); },
                        }
                        : undefined}
                    onSave={() => void save()}
                    onDiscard={() => { setReport(null); draft.reset(); apps?.reset(); }}
                />
            </div>
            {/* alpha.128: the server's limits of site settings, every key. */}
            <div className="mt-6"><Suspense fallback={null}><ServerSettingLimits /></Suspense></div>
        </div>
    );
}
