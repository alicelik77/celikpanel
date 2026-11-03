import { useState } from 'react';
import { Layers, Lock, Pencil, Plus, Save, Trash2, X } from 'lucide-react';
import { useI18n } from '../i18n';
import type { TranslationKey } from '../i18n/en';
import { showToast } from './Toast';
import { Button, Checking, CouldNotCheck, ErrorBanner, KnownEmpty, RemoteGate, inputClass } from './ui';
import { BlockedReason } from './BlockingProblems';
import type { ApiError } from '../lib/apiError';
import { refusalText, refusalToast } from '../lib/apiError';
import { countText, invalidate, mapRemote, useRemote } from '../lib/remote';
import {
    NEW_PLAN, OFFERED_URL, PERMISSION_IDS, RESELLER_PLANS_URL, editedFromResellerPlan, groupSoftware, limitWords, planBody, rightsSummary,
    selectionOf, sendRights, softwareGroupName, softwareName,
    type EditedPlan, type PlanKind, type ResellerPlan, type RightsPreview, type RightsSelection, type RightsView,
} from '../lib/rights';
import { RightsPreviewBox, RightsSetEditor } from './RightsSetEditor';
import { readableRefusal, useGrantorCatalog } from './AccountRights';

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

/** The editor of one plan, with its narrowing preview. */
export function PlanEditor({ kind, initial, grantor, grantorIsReseller, onClose }: {
    kind: PlanKind;
    initial: EditedPlan;
    grantor: RightsView;
    grantorIsReseller: boolean;
    onClose: () => void;
}) {
    const { t } = useI18n();
    const isNew = initial.id === 0;
    const [plan, setPlan] = useState(initial);
    const [selection, setSelection] = useState<RightsSelection>(() =>
        selectionOf(initial.rights ?? { all_permissions: true, all_software: true, permissions: [], software: [] }, grantor));
    // An existing plan's set is sent only once it was changed here: an
    // untouched "everything offered" stays as it is.
    const [touched, setTouched] = useState(isNew);
    const [busy, setBusy] = useState(false);
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

    const save = async (confirm: boolean) => {
        setBusy(true);
        setRefusal(null);
        try {
            const overUsage = confirm && (shown?.preview.over_limit.length ?? 0) > 0;
            const result = await sendRights(url, isNew ? 'POST' : 'PUT', planBody(kind, plan, chosen, { narrowing: confirm, overUsage }));
            if (result.kind === 'lost') {
                showToast('error', t('common.resultUnknown'));
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

    return (
        <section className="mb-4 rounded-xl border border-border bg-surface-2/50 p-4" data-plan-editor={kind}>
            <label className="block max-w-md">
                <span className="mb-1 block text-xs text-fg-muted">{t('plans.form.name')}</span>
                <input id={`${idPrefix}-name`} value={plan.name} onChange={(e) => setPlan({ ...plan, name: e.target.value })} className={inputClass} autoFocus />
            </label>
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
                />
            </div>
            {!isNew && !touched && initial.rights?.all_permissions && initial.rights.all_software && (
                <p className="mt-3 text-xs text-fg-muted">{t('rights.plan.everythingKept')}</p>
            )}
            {shown && <div className="mt-4"><RightsPreviewBox id={`${idPrefix}-preview`} preview={shown.preview} sentence={shown.sentence} /></div>}
            <ErrorBanner className="mt-3" error={readableRefusal(refusal, t)} sentence="plans.saveFailed" />
            <div className="mt-4 flex flex-wrap items-center justify-end gap-2">
                {nameMissing && <BlockedReason id={`${idPrefix}-reason`} target={`${idPrefix}-name`} text={t('acct.plan.missing')} className="mr-auto" />}
                <Button type="button" icon={X} onClick={onClose}>{t('common.cancel')}</Button>
                {shown ? (
                    <>
                        <Button type="button" onClick={() => setPending(null)} disabled={busy}>{t('rights.back')}</Button>
                        <Button type="button" variant="primary" onClick={() => void save(true)} loading={busy} data-plan-confirm="">
                            {t(shown.preview.over_limit.length > 0 && kind === 'customer' ? 'plans.saveAnyway' : 'rights.plan.confirm')}
                        </Button>
                    </>
                ) : (
                    <Button type="button" variant="primary" icon={Save} onClick={() => void save(false)} loading={busy} disabled={nameMissing}
                        aria-describedby={nameMissing ? `${idPrefix}-reason` : undefined}>
                        {t('plans.save')}
                    </Button>
                )}
            </div>
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
        if (!confirm(t('rights.plan.deleteConfirm', { name: plan.name }))) return;
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
    };

    return (
        <div data-reseller-plans="">
            <p className="mb-3 max-w-[75ch] text-xs text-fg-subtle">{t('rights.resellerPlans.hint')}</p>
            <BuiltinPlanRow kind="reseller" />
            <div className="mb-3 flex items-center justify-between">
                <span className="text-xs text-fg-subtle">{countText(mapRemote(rows, (list) => list.length)) === '1' ? t('common.itemsTotalOne') : t('common.itemsTotal', { n: countText(mapRemote(rows, (list) => list.length)) })}</span>
                <Button variant="primary" icon={Plus} disabled={!known} onClick={() => setEditing({ ...NEW_PLAN, numbers: { ...NEW_PLAN.numbers, max_customers: 10 } })}>{t('plans.add')}</Button>
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
                                                <IconButton disabled={!known} title={t('plans.edit')} onClick={() => setEditing(editedFromResellerPlan(plan))}><Pencil className="h-4 w-4" /></IconButton>
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
    return id.startsWith('permission:') ? t(`rights.perm.${id.slice('permission:'.length)}` as TranslationKey) : softwareName(id);
}

/**
 * What the administrator offers on this server: every permission and every
 * installed piece of software, each shown or hidden for resellers and
 * customers. Hiding changes nothing that runs; the item can no longer be
 * chosen. The Unlimited plans follow at once (P4).
 */
export function OfferedItemsTab() {
    const { t } = useI18n();
    const catalog = useGrantorCatalog();
    const [pending, setPending] = useState<string | null>(null);
    const [failure, setFailure] = useState<ApiError | null>(null);

    const change = async (item: string, offered: boolean) => {
        if (pending) return;
        setPending(item);
        setFailure(null);
        try {
            const result = await sendRights(OFFERED_URL, 'PUT', { offering_id: item, offered });
            if (result.kind === 'lost') showToast('error', t('common.resultUnknown'));
            if (result.kind === 'refused') setFailure(result.error);
            // Every chooser, plan and account screen follows the offer.
            invalidate('rights', 'components');
        } finally {
            setPending(null);
        }
    };

    const row = (item: string, offered: boolean) => (
        <li key={item} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2" data-offered-item={item} data-offered={offered ? 'yes' : 'no'}>
            <label className="flex min-w-0 flex-1 cursor-pointer items-center gap-2.5 max-sm:min-h-11">
                <input type="checkbox" checked={offered} disabled={pending !== null || catalog.reading} onChange={(e) => void change(item, e.target.checked)}
                    className="h-4 w-4 shrink-0 accent-primary" aria-label={t('rights.offered.toggle', { name: offeredLabel(item, t) })} />
                <span className="min-w-0 text-sm font-medium text-fg">{offeredLabel(item, t)}</span>
            </label>
            <span className={`text-xs ${offered ? 'text-fg-muted' : 'font-semibold text-warning'}`}>{t(offered ? 'rights.offered.yes' : 'rights.offered.no')}</span>
        </li>
    );

    return (
        <div data-offered-layer="">
            <p className="mb-3 max-w-[75ch] text-sm text-fg-muted">{t('rights.offered.intro')}</p>
            {catalog.remote.state === 'loading' && <Checking label={t('rights.catalogChecking')} />}
            {catalog.remote.state === 'unknown' && <CouldNotCheck text={t('rights.catalogUnknown')} onRetry={() => void catalog.retry()} busy={catalog.reading} />}
            {catalog.remote.state === 'known' && (() => {
                const view = catalog.remote.value;
                return (
                    <div className="grid gap-4 lg:grid-cols-2">
                        <section>
                            <h3 className="mb-2 text-sm font-semibold text-fg">{t('rights.part.permissions')}</h3>
                            <ul className="divide-y divide-border rounded-lg border border-border bg-surface">
                                {PERMISSION_IDS.map((id) => row(`permission:${id}`, view.permissions.find((item) => item.id === id)?.granted !== false))}
                            </ul>
                        </section>
                        <section>
                            <h3 className="mb-2 text-sm font-semibold text-fg">{t('rights.part.software')}</h3>
                            {view.installed_unknown && <p className="mb-2 text-xs text-warning">{t('rights.installedUnknown')}</p>}
                            {view.software.length === 0 ? <p className="text-sm text-fg-muted">{t('rights.softwareNone')}</p> : (
                                <div className="grid gap-3">
                                    {groupSoftware(view.software).map(({ group, items }) => {
                                        const heading = softwareGroupName(group);
                                        return (
                                            <div key={group}>
                                                <p className="mb-1 text-xs font-semibold text-fg-muted">{heading.product ?? t(heading.key === 'acme' ? 'rights.group.acme' : 'rights.group.extension')}</p>
                                                <ul className="divide-y divide-border rounded-lg border border-border bg-surface">{items.map((item) => row(item.id, item.granted))}</ul>
                                            </div>
                                        );
                                    })}
                                </div>
                            )}
                        </section>
                    </div>
                );
            })()}
            {pending && <Checking className="mt-2" label={t('rights.offered.saving', { name: offeredLabel(pending, t) })} />}
            <ErrorBanner className="mt-3" error={failure} sentence="rights.offered.failed" />
        </div>
    );
}

